package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/jaracil/ei"
	. "github.com/jaracil/nexus/log"
	"github.com/nayarsystems/nxgo/nxcore"
	"github.com/sirupsen/logrus"
	r "gopkg.in/gorethink/gorethink.v5"
)

type LoginResponse struct {
	User     string                            `json:"user"`
	Tags     map[string]map[string]interface{} `json:"tags"`
	Metadata map[string]interface{}            `json:"metadata"`
}

func (nc *NexusConn) handleSysReq(req *JsonRpcReq) {
	switch req.Method {
	case "sys.ping":
		req.Result(ei.M{"ok": true})

	case "sys.version":
		req.Result(ei.M{"version": Version.String()})

	case "sys.watchdog":
		wdt, err := ei.N(req.Params).M("watchdog").Lower().Int64()
		if err == nil {
			if wdt < 10 {
				wdt = 10
			}
			if wdt > 1800 {
				wdt = 1800
			}
			atomic.StoreInt64(&nc.wdog, wdt)
		}
		req.Result(ei.M{"watchdog": nc.wdog})

	case "sys.reload":
		if done, errcode := nc.reload(true); !done {
			req.Error(errcode, "", nil)
		} else {
			req.Result(ei.M{"ok": true})
		}

	case "sys.login":
		var user string
		var mask map[string]map[string]interface{}
		var metadata map[string]interface{}

		// Auth
		method := ei.N(req.Params).M("method").Lower().StringZ()
		switch method {
		case "", "basic":
			var err int
			user, _, err = nc.BasicAuth(req.Params)
			if err != ErrNoError {
				req.Error(err, "", nil)
				return
			}

		default:
			nic := NewInternalClient()
			defer nic.Close()
			ret, err := nic.TaskPush(fmt.Sprintf("sys.login.%s.login", method), req.Params, time.Second*10, &nxcore.TaskOpts{})
			if err != nil {
				req.Error(ErrPermissionDenied, err.Error(), nil)
				return
			}

			var loginResponse LoginResponse
			b, err := json.Marshal(ret)
			if err != nil {
				req.Error(ErrInternal, "", nil)
				return
			}

			if err := json.Unmarshal(b, &loginResponse); err != nil {
				req.Error(ErrInternal, "", nil)
				return
			}
			user = loginResponse.User
			mask = loginResponse.Tags
			metadata = loginResponse.Metadata
		}

		// Check user limits

		ud, err := loadUserData(user)
		if err != ErrNoError {
			req.Error(err, "", nil)
			return
		}

		if !nc.checkUserLimits(ud) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		// LOGGED IN!
		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&nc.user)), unsafe.Pointer(&UserData{
			User:        ud.User,
			Mask:        mask,
			Tags:        maskTags(ud.Tags, mask),
			MaxSessions: ud.MaxSessions,
			Whitelist:   ud.Whitelist,
			Blacklist:   ud.Blacklist,
		}))
		nc.updateSession()
		hook("user", ud.User, ud.User, ei.M{
			"action":      "login",
			"user":        nc.user.User,
			"mask":        nc.user.Mask,
			"tags":        nc.user.Tags,
			"maxSessions": nc.user.MaxSessions,
			"whitelist":   nc.user.Whitelist,
			"blacklist":   nc.user.Blacklist,
		})
		req.Result(ei.M{"ok": true, "user": nc.user.User, "connid": nc.connId, "tags": nc.user.Tags, "metadata": metadata})

	default:
		req.Error(ErrMethodNotFound, "", nil)
	}
}

// Return tags that are equal in both A and B
func maskTags(src map[string]map[string]interface{}, mask map[string]map[string]interface{}) (m map[string]map[string]interface{}) {
	m = make(map[string]map[string]interface{})

	if mask == nil {
		return src
	}

	for prefix, tags := range src {
		if bprefix, ok := mask[prefix]; ok {
			m[prefix] = make(map[string]interface{})
			for k, v := range tags {
				if vb, ok := bprefix[k]; ok && reflect.DeepEqual(v, vb) {
					m[prefix][k] = v
				}
			}
		}
	}
	return
}

func loadUserData(user string) (*UserData, int) {
	return loadUserDataWithTemplates(user, make(map[string]bool))
}

func loadUserDataWithTemplates(user string, loadedTemplates map[string]bool) (*UserData, int) {
	ud := &UserData{}
	cur, err := r.Table("users").Get(strings.ToLower(user)).Run(db)
	if err != nil {
		return nil, ErrInternal
	}
	defer cur.Close()
	err = cur.One(ud)
	if err != nil {
		if err == r.ErrEmptyResult {
			return nil, ErrPermissionDenied
		}
		return nil, ErrInternal
	}

	if ud.Tags == nil {
		ud.Tags = make(map[string]map[string]interface{})
	}

	if ud.Mask == nil {
		ud.Mask = make(map[string]map[string]interface{})
	}

	if ud.Blacklist == nil {
		ud.Blacklist = make([]string, 0)
	}

	if ud.Whitelist == nil {
		ud.Whitelist = make([]string, 0)
	}

	if ud.Templates == nil {
		ud.Templates = make([]string, 0)
	}

	grabTemplates(ud, loadedTemplates)
	return ud, ErrNoError
}

func grabTemplates(ud *UserData, loadedTemplates map[string]bool) {
	for _, template := range ud.Templates {
		if loadedTemplates[template] {
			continue
		}
		loadedTemplates[template] = true

		tud, err := loadUserDataWithTemplates(template, loadedTemplates)
		if err != ErrNoError {
			continue
		}

		mergeTags(tud, ud)
	}
}

// Bring from src tags that did not exist on dst
func mergeTags(src, dst *UserData) {
	for prefix, tags := range src.Tags {
		for tag, val := range tags {
			if _, ok := dst.Tags[prefix]; !ok {
				dst.Tags[prefix] = make(map[string]interface{})
			}
			if _, ok := dst.Tags[prefix][tag]; !ok {
				dst.Tags[prefix][tag] = val
			}
		}
	}
}

func (nc *NexusConn) checkUserLimits(ud *UserData) bool {
	if ud.Disabled {
		return false
	}

	nci := NewInternalClient()
	defer nci.Close()

	// Max Sessions opened?
	// soft limit because race condition checking sessions
	sessions, err := nci.SessionList(ud.User, -1, 0)
	seslen := 0
	for _, u := range sessions {
		if u.User == ud.User {
			seslen = len(u.Sessions)
			break
		}
	}
	if err != nil || (ud.MaxSessions > 0 && seslen+1 > ud.MaxSessions) {
		Log.WithFields(logrus.Fields{
			"user":        ud.User,
			"current":     seslen,
			"maxsessions": ud.MaxSessions,
		}).Warnf("User has too many sessions opened")
		return false
	}

	remoteaddr := nc.conn.RemoteAddr().String()

	// Whitelisted?
	for _, wr := range ud.Whitelist {
		if match, err := regexp.MatchString(wr, remoteaddr); err == nil && match {
			Log.WithFields(logrus.Fields{
				"user":      ud.User,
				"remote":    remoteaddr,
				"whitelist": wr,
			}).Warnf("User whitelisted", ud.User, remoteaddr, wr)
			return true
		}
	}

	// Blacklisted?
	for _, br := range ud.Blacklist {
		if match, err := regexp.MatchString(br, remoteaddr); err == nil && match {
			Log.WithFields(logrus.Fields{
				"user":      ud.User,
				"remote":    remoteaddr,
				"blacklist": br,
			}).Warnf("User blacklisted", ud.User, remoteaddr, br)
			return false
		}
	}

	return true
}

func (nc *NexusConn) BasicAuth(params interface{}) (string, map[string]map[string]interface{}, int) {
	user, err := ei.N(params).M("user").Lower().String()
	if err != nil {
		return "", nil, ErrInvalidParams
	}
	pass, err := ei.N(params).M("pass").String()
	if err != nil {
		return "", nil, ErrInvalidParams
	}
	var suser string
	split := strings.Split(user, ">")
	switch len(split) {
	case 1:
	case 2:
		if len(split[0]) > 0 && len(split[1]) > 0 {
			user = split[0]
			suser = split[1]
		} else {
			return "", nil, ErrInvalidParams

		}
	default:
		return "", nil, ErrInvalidParams
	}

	ud, rerr := loadUserData(user)
	if rerr != ErrNoError {
		return "", nil, rerr
	}
	dk, err := HashPass(pass, ud.Salt)
	if err != nil {
		return "", nil, ErrInternal
	}
	if ud.Pass != dk {
		return "", nil, ErrPermissionDenied
	}

	if suser != "" {
		tags := getTags(ud, suser)
		if !(ei.N(tags).M("@admin").BoolZ()) {
			return "", nil, ErrPermissionDenied
		}
		sud, err := loadUserData(suser)
		if err != ErrNoError {
			return "", nil, ErrPermissionDenied
		}
		return sud.User, sud.Tags, ErrNoError
	}

	return ud.User, ud.Tags, ErrNoError
}
