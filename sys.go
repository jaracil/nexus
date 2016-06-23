package main

import (
	"strings"
	"sync/atomic"
	"unsafe"

	r "github.com/dancannon/gorethink"
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
		user, err := ei.N(req.Params).M("user").String()
		if err != nil {
			req.Error(ErrInvalidParams, "user", nil)
			return
		}
		pass, err := ei.N(req.Params).M("pass").String()
		if err != nil {
			req.Error(ErrInvalidParams, "pass", nil)
			return
		}
		ud := &UserData{}
		cur, err := r.Table("users").Get(strings.ToLower(user)).Run(db)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		defer cur.Close()
		err = cur.One(ud)
		if err != nil {
			if err == r.ErrEmptyResult {
				req.Error(ErrPermissionDenied, "", nil)
				return
			}
			req.Error(ErrInternal, "", nil)
			return
		}
		dk, err := HashPass(pass, ud.Salt)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if ud.Pass != dk {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&nc.user)), unsafe.Pointer(ud))
		req.Result(ei.M{"ok": true})
	default:
		req.Error(ErrMethodNotFound, "", nil)
	}
}
