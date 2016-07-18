package main

import (
	"reflect"
	"sync/atomic"
	"unsafe"

	"github.com/jaracil/ei"
)

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
		var err int

		// Auth
		switch ei.N(req.Params).M("method").StringZ() {
		default:
			user, tags, err = nc.BasicAuth(req.Params)
		case "token":
			user, tags, err = nc.TokenAuth(req.Params)
		}

		if err != ErrNoError {
			req.Error(err, "", nil)
			return
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
