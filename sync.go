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
				req.Result(ei.M{"ok": false})
			} else {
				req.Error(ErrInternal, "", nil)
			}
			return
		}
		req.Result(ei.M{"ok": res.Inserted > 0})
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
		req.Result(ei.M{"ok": res.Deleted > 0})
	default:
		req.Error(ErrMethodNotFound, "", nil)
	}
}
