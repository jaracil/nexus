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
		var suser string
		split := strings.Split(user, ">")
		switch len(split) {
		case 1:
		case 2:
			if len(split[0]) > 0 && len(split[1]) > 0 {
				user = split[0]
				suser = split[1]
			} else {
				req.Error(ErrInvalidParams, "", nil)
				return
			}
		default:
			req.Error(ErrInvalidParams, "", nil)
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

		if suser != "" {
			tags := nc.getTags(suser)
			if !(ei.N(tags).M("@admin").BoolZ()) {
				req.Error(ErrPermissionDenied, "", nil)
				return
			}
			sud := &UserData{}
			scur, err := r.Table("users").Get(strings.ToLower(suser)).Run(db)
			if err != nil {
				req.Error(ErrInternal, "", nil)
				return
			}
			defer scur.Close()
			err = scur.One(sud)
			if err != nil {
				if err == r.ErrEmptyResult {
					req.Error(ErrPermissionDenied, "", nil)
					return
				}
				req.Error(ErrInternal, "", nil)
				return
			}
			atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&nc.user)), unsafe.Pointer(sud))
		}
		req.Result(ei.M{"ok": true, "user": nc.user.User})

	case "sys.stats":
		prefix, err := ei.N(req.Params).M("prefix").String()
		if err != nil {
			req.Error(ErrInvalidParams, "prefix", nil)
			return
		}
		tags := nc.getTags(prefix)
		if !(ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		cur, err := r.Table("tasks").Pluck("path").Run(db)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		pulls := make(map[string]int)
		pushs := make(map[string]int)
		var task Task
		for cur.Next(&task) {
			if strings.HasPrefix(task.Path, "@pull."+prefix) {
				p := strings.TrimPrefix(task.Path, "@pull.")
				pulls[p]++
			} else if strings.HasPrefix(task.Path, prefix) {
				pushs[task.Path]++
			}
		}
		ret := make(map[string]interface{})
		ret["pulls"] = pulls
		ret["pushes"] = pushs
		req.Result(ret)

	default:
		req.Error(ErrMethodNotFound, "", nil)
	}
}
