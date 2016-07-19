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
		var mask map[string]map[string]interface{}

		// Auth
		method := ei.N(req.Params).M("method").StringZ()
		switch method {
		case "", "basic":
			var err int
			user, mask, err = nc.BasicAuth(req.Params)
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
		}

		ud, err := loadUserData(user)
		if err != ErrNoError {
			req.Error(err, "", nil)
			return
		}

		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&nc.user)), unsafe.Pointer(&UserData{User: ud.User, Mask: mask, Tags: maskTags(ud.Tags, mask)}))

		nc.updateSession()
		req.Result(ei.M{"ok": true, "user": nc.user.User, "connId": nc.connId})
	case "sys.reload":
		if done, errcode := nc.reload(true); !done {
			req.Error(errcode, "", nil)
		} else {
			req.Result(ei.M{"ok": true})
		}
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
