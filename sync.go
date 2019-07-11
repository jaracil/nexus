package main

import (
	"github.com/jaracil/ei"
	r "gopkg.in/rethinkdb/rethinkdb-go.v5"
)

func (nc *NexusConn) handleSyncReq(req *JsonRpcReq) {
	switch req.Method {
	case "sync.lock":
		lock, err := ei.N(req.Params).M("lock").Lower().F(checkRegexp, _prefixRegexp).F(checkNotEmptyLabels).String()
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
		lock, err := ei.N(req.Params).M("lock").Lower().F(checkRegexp, _prefixRegexp).F(checkNotEmptyLabels).String()
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

	case "sync.list":
		prefix, depth, filter, limit, skip := getListParams(req.Params)

		tags := nc.getTags(prefix)
		if !(ei.N(tags).M("@sync.list").BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		term := getListTerm("locks", "", "id", prefix, depth, filter, limit, skip).
			Pluck("id", "owner")

		cur, err := term.Run(db)
		defer cur.Close()
		if err != nil {
			req.Error(ErrInternal, err.Error(), nil)
			return
		}
		var all []interface{}
		if err := cur.All(&all); err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		req.Result(all)

	case "sync.count":
		prefix := getPrefixParam(req.Params)
		filter := ei.N(req.Params).M("filter").StringZ()
		countSubprefixes := ei.N(req.Params).M("subprefixes").BoolZ()

		tags := nc.getTags(prefix)
		if !(ei.N(tags).M("@sync.count").BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		term := getCountTerm("locks", "", "id", prefix, filter, countSubprefixes)
		cur, err := term.Run(db)
		defer cur.Close()
		if err != nil {
			req.Error(ErrInternal, err.Error(), nil)
			return
		}
		var all []interface{}
		if err := cur.All(&all); err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if countSubprefixes {
			req.Result(all)
		} else if len(all) > 0 {
			req.Result(ei.M{"count": all[0]})
		} else {
			req.Result(ei.M{"count": 0})
		}

	default:
		req.Error(ErrMethodNotFound, "", nil)
	}
}
