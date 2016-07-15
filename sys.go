package main

import (
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
		case "token":
			user, tags, err = nc.TokenAuth(req.Params)

		default:
			user, tags, err = nc.BasicAuth(req.Params)
		}

		if err != ErrNoError {
			req.Error(err, "", nil)
			return
		}

		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&nc.user)), unsafe.Pointer(&UserData{User: user, Tags: tags}))
		nc.updateSession()
		req.Result(ei.M{"ok": true, "user": nc.user.User})

	default:
		req.Error(ErrMethodNotFound, "", nil)
	}
}
