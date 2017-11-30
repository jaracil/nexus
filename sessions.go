package main

import (
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jaracil/ei"
	. "github.com/jaracil/nexus/log"
	r "gopkg.in/gorethink/gorethink.v3"
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
			Log.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Errorf("Error opening sessionTrack iterator")
			time.Sleep(time.Second)
			continue
		}
		retry = 0
		for {
			sf := &SessionFeed{}
			if !iter.Next(sf) {
				Log.WithFields(logrus.Fields{
					"error": iter.Err().Error(),
				}).Errorf("Error processing sessionTrack feed")
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
		prefix, depth, filter, limit, skip := getListParams(req.Params)

		tags := nc.getTags(prefix)
		if !(ei.N(tags).M("@sys.session.list").BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		term := getListTerm("sessions", "users", "user", prefix, depth, filter, limit, skip)

		term = term.
			Map(func(row r.Term) interface{} {
				return ei.M{"user": row.Field("user"),
					"connid":        row.Field("id"),
					"nodeid":        row.Field("nodeId"),
					"remoteAddress": row.Field("remoteAddress"),
					"creationTime":  row.Field("creationTime"),
					"protocol":      row.Field("protocol")}
			}).
			Group("user").
			Pluck("connid", "nodeid", "remoteAddress", "creationTime", "protocol").
			Filter(r.Row.Field("protocol").Ne("internal"))

		cur, err := term.Ungroup().
			Map(func(row r.Term) interface{} {
				return ei.M{"user": row.Field("group"),
					"sessions": row.Field("reduction"),
					"n":        row.Field("reduction").Count()}
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
		prefix := ei.N(req.Params).M("connid").StringZ()

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

		Log.WithFields(logrus.Fields{
			"connid":  nc.connId,
			"action":  action,
			"session": prefix,
			"by":      user,
		}).Printf("Session %s", action)

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
