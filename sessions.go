package main

import (
	"log"
	"time"

	r "github.com/dancannon/gorethink"
	"github.com/jaracil/ei"
)

type Session struct {
	Id   string `gorethink:"id"`
	Kick bool   `gorethink:"kick"`
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
				"new_val": []string{"id", "kick"},
				"old_val": []string{"id"}}).
			Run(db)
		if err != nil {
			log.Printf("Error opening sessionTrack iterator:%s\n", err.Error())
			time.Sleep(time.Second)
			continue
		}
		retry = 0
		for {
			sf := &SessionFeed{}
			if !iter.Next(sf) {
				log.Printf("Error processing feed: %s\n", iter.Err().Error())
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
	case "sys.sessions.list":
		prefix := ei.N(req.Params).M("prefix").StringZ()

		tags := nc.getTags(prefix)
		if !(ei.N(tags).M("@sys.session.list").BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		cur, err := r.Table("sessions").
			Between(prefix, prefix+"\uffff", r.BetweenOpts{Index: "users"}).
			Group("user").
			Pluck("id", "nodeId", "remoteAddress", "creationTime", "protocol").
			Filter(r.Row.Field("protocol").Ne("internal")).
			Ungroup().
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

	case "sys.sessions.kick":
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
		user := ei.N(userd).M("user").StringZ()
		tags := nc.getTags(user)
		if !(ei.N(tags).M("@sys.session.kick").BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		log.Printf("Session [%s] is kicking the user [%s] from session [%s]\n", nc.connId, user, prefix)

		res, err := r.Table("sessions").
			Between(prefix, prefix+"\uffff").
			Update(ei.M{"kick": true}).
			RunWrite(db)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		req.Result(ei.M{"kicked": res.Replaced})

	default:
		req.Error(ErrMethodNotFound, "", nil)
	}
}
