package main

import (
	r "github.com/dancannon/gorethink"
	"github.com/jaracil/ei"
)

func (nc *NexusConn) handleSyncReq(req *JsonRpcReq) {
	switch req.Method {
	case "sync.lock":
		lock, err := ei.N(req.Params).M("lock").String()
		if err != nil {
			req.Error(ErrInvalidParams, "lock", nil)
			return
		}
		tags := nc.getTags(lock)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		res, err := r.Table("locks").
			Insert(ei.M{"id": lock, "owner": nc.connId}).
			RunWrite(db, r.RunOpts{Durability: "hard"})
		if err != nil {
			if r.IsConflictErr(err) {
				req.Error(ErrLockNotOwned, "", nil)
			} else {
				req.Error(ErrInternal, "", nil)
			}
			return
		}
		if res.Inserted <= 0 {
			req.Error(ErrLockNotOwned, "", nil)
			return
		}
		req.Result(ei.M{"ok": true})
	case "sync.unlock":
		lock, err := ei.N(req.Params).M("lock").String()
		if err != nil {
			req.Error(ErrInvalidParams, "lock", nil)
			return
		}
		tags := nc.getTags(lock)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		res, err := r.Table("locks").
			GetAll(lock).
			Filter(r.Row.Field("owner").Eq(nc.connId)).
			Delete().
			RunWrite(db, r.RunOpts{Durability: "hard"})

		if err != nil {
			req.Error(ErrInternal, err.Error(), nil)
			return
		}
		if res.Deleted <= 0 {
			req.Error(ErrLockNotOwned, "", nil)
			return
		}
		req.Result(ei.M{"ok": true})
	default:
		req.Error(ErrMethodNotFound, "", nil)
	}
}
