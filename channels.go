package main

import (
	r "github.com/dancannon/gorethink"
	"github.com/jaracil/ei"
)

func (nc *NexusConn) handleChanReq(req *JsonRpcReq) {
	switch req.Method {
	case "chan.sub":
		pipeid, err := ei.N(req.Params).M("pipeid").String()
		if err != nil {
			req.Error(ErrInvalidParams, "pipeid", nil)
			return
		}
		channel, err := ei.N(req.Params).M("chan").String()
		if err != nil {
			req.Error(ErrInvalidParams, "chan", nil)
			return
		}
		tags := nc.getTags(channel)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		res, err := r.Table("pipes").
			Get(pipeid).
			Update(map[string]interface{}{
				"subs":  r.Row.Field("subs").Default(ei.S{}).SetInsert(channel),
				"ismsg": false,
				"msg":   nil,
			}).
			RunWrite(db, r.RunOpts{Durability: "soft"})
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if res.Unchanged == 0 && res.Replaced == 0 {
			req.Error(ErrInvalidPipe, "", nil)
			return
		}
		req.Result(map[string]interface{}{"ok": true})
	case "chan.unsub":
		pipeid, err := ei.N(req.Params).M("pipeid").String()
		if err != nil {
			req.Error(ErrInvalidParams, "pipeid", nil)
			return
		}
		channel, err := ei.N(req.Params).M("chan").String()
		if err != nil {
			req.Error(ErrInvalidParams, "chan", nil)
			return
		}
		tags := nc.getTags(channel)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		res, err := r.Table("pipes").
			Get(pipeid).
			Update(map[string]interface{}{
				"subs":  r.Row.Field("subs").Default(ei.S{}).Difference(ei.S{channel}),
				"ismsg": false,
				"msg":   nil,
			}).
			RunWrite(db, r.RunOpts{Durability: "soft"})
		if err != nil {
			req.Error(ErrInternal, err.Error(), nil)
			return
		}
		if res.Unchanged == 0 && res.Replaced == 0 {
			req.Error(ErrInvalidPipe, "", nil)
			return
		}
		req.Result(map[string]interface{}{"ok": true})

	case "chan.pub":
		channel, err := ei.N(req.Params).M("chan").String()
		if err != nil {
			req.Error(ErrInvalidParams, "chan", nil)
			return
		}
		msg, err := ei.N(req.Params).M("msg").Raw()
		if err != nil {
			req.Error(ErrInvalidParams, "msg", nil)
			return
		}

		tags := nc.getTags(channel)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		res, err := r.Table("pipes").
			GetAllByIndex("subs", channel).
			Update(map[string]interface{}{"msg": msg, "count": r.Row.Field("count").Add(1), "ismsg": true}).
			RunWrite(db, r.RunOpts{Durability: "soft"})
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		req.Result(map[string]interface{}{"ok": true, "sent": res.Replaced})
	default:
		req.Error(ErrMethodNotFound, "", nil)

	}
}
