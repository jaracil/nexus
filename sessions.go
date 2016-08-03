package main

import (
	"time"

	r "github.com/dancannon/gorethink"
	"github.com/jaracil/ei"
	. "github.com/jaracil/nexus/log"
)

type Session struct {
	Id     string `gorethink:"id"`
	Kick   bool   `gorethink:"kick"`
	Reload bool   `gorethink:"reload"`
}

type SessionFeed struct {
	Old *Session `gorethink:"old_val"`
	New *Session `gorethink:"new_val"`
}

func sessionTrack() {
	defer exit("sessions change-feed error")
	for retry := 0; retry < 10; retry++ {
		iter, err := r.Table("sessions").
			Between(nodeId, nodeId+"\uffff").
			Changes(r.ChangesOpts{IncludeInitial: true, Squash: false}).
			Pluck(map[string]interface{}{
				"new_val": []string{"id", "kick", "reload"},
				"old_val": []string{"id"}}).
			Run(db)
		if err != nil {
			Log.Errorf("Error opening sessionTrack iterator:%s", err.Error())
			time.Sleep(time.Second)
			continue
		}
		retry = 0
		for {
			sf := &SessionFeed{}
			if !iter.Next(sf) {
				Log.Errorf("Error processing sessionTrack feed: %s", iter.Err().Error())
				iter.Close()
				break
			}
			if sf.New == nil { // session closed
				sesNotify.Unregister(sf.Old.Id)
				continue
			}
			sesNotify.Notify(sf.New.Id, sf.New)
		}
	}
}

func (nc *NexusConn) handleSessionReq(req *JsonRpcReq) {
	switch req.Method {
	case "sys.session.list":
		prefix := ei.N(req.Params).M("prefix").Lower().StringZ()
		limit, err := ei.N(req.Params).M("limit").Int()
		if err != nil {
			limit = 100
		}
		skip, err := ei.N(req.Params).M("skip").Int()
		if err != nil {
			skip = 0
		}
		tags := nc.getTags(prefix)
		if !(ei.N(tags).M("@sys.session.list").BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		term := r.Table("sessions").
			Between(prefix, prefix+"\uffff", r.BetweenOpts{Index: "users"}).
			Group("user").
			Pluck("id", "nodeId", "remoteAddress", "creationTime", "protocol").
			Filter(r.Row.Field("protocol").Ne("internal"))

		if skip >= 0 {
			term = term.Skip(skip)
		}

		if limit >= 0 {
			term = term.Limit(limit)
		}

		cur, err := term.Ungroup().
			Map(func(row r.Term) interface{} {
				return ei.M{"user": row.Field("group"), "sessions": row.Field("reduction"), "n": row.Field("reduction").Count()}
			}).Run(db)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		var all []interface{}
		if err := cur.All(&all); err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		req.Result(all)

	case "sys.session.kick":
		fallthrough
	case "sys.session.reload":
		action := req.Method[12:]
		prefix := ei.N(req.Params).M("connId").StringZ()

		if len(prefix) < 16 {
			req.Error(ErrInvalidParams, "", nil)
			return
		}

		connuser, err := r.Table("sessions").
			Between(prefix, prefix+"\uffff").
			Pluck("user").
			Run(db)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		var userd interface{}
		if err := connuser.One(&userd); err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		user := ei.N(userd).M("user").Lower().StringZ()
		tags := nc.getTags(user)
		if !(ei.N(tags).M("@sys.session."+action).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		Log.Printf("Session [%s] is %sing session [%s] from user [%s]", nc.connId, action, prefix, user)

		res, err := r.Table("sessions").
			Between(prefix, prefix+"\uffff").
			Update(ei.M{action: true}).
			RunWrite(db)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		req.Result(ei.M{action + "ed": res.Replaced})

	default:
		req.Error(ErrMethodNotFound, "", nil)
	}
}
