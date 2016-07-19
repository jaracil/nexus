package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	r "github.com/dancannon/gorethink"
	"github.com/jaracil/ei"
	"github.com/jaracil/nxcli/nxcore"
)

type LoginResponse struct {
	User string                            `json:"user"`
	Tags map[string]map[string]interface{} `json:"tags"`
}

func (nc *NexusConn) handleSysReq(req *JsonRpcReq) {
	switch req.Method {
	case "sys.ping":
		req.Result("pong")

	case "sys.watchdog":
		wdt := ei.N(req.Params).Int64Z()
		if wdt < 10 {
			wdt = 10
		}
		atomic.StoreInt64(&nc.wdog, wdt)
		req.Result(ei.M{"ok": true, "watchdog": wdt})

	case "sys.login":
		var user string
		var tags map[string]map[string]interface{}

		// Auth
		method := ei.N(req.Params).M("method").StringZ()
		switch method {
		case "", "basic":
			var err int
			user, tags, err = nc.BasicAuth(req.Params)
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
			tags = loginResponse.Tags
		}

		ud, err := loadUserData(user)
		if err != ErrNoError {
			req.Error(err, "", nil)
			return
		}

		tags = maskTags(tags, ud.Tags)

		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&nc.user)), unsafe.Pointer(&UserData{User: ud.User, Tags: tags}))
		nc.updateSession()
		req.Result(ei.M{"ok": true, "user": nc.user.User})

	default:
		req.Error(ErrMethodNotFound, "", nil)
	}
}

// Return tags that are equal in both A and B
func maskTags(a map[string]map[string]interface{}, b map[string]map[string]interface{}) (m map[string]map[string]interface{}) {
	m = make(map[string]map[string]interface{})
	for prefix, tags := range a {
		if bprefix, ok := b[prefix]; ok {
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
	return ud, ErrNoError
}

func (nc *NexusConn) BasicAuth(params interface{}) (string, map[string]map[string]interface{}, int) {
	user, err := ei.N(params).M("user").String()
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
