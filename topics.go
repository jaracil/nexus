package main

import (
	"strings"

	r "github.com/dancannon/gorethink"
	"github.com/jaracil/ei"
)

func topicList(s string) (res []interface{}) {
	s = strings.Trim(s, ". ")
	if s == "" {
		return []interface{}{".", ".*"}
	}
	res = append(res, s)
	chunks := strings.Split(s, ".")
	for n := len(chunks); n >= 0; n-- {
		res = append(res, strings.Join(chunks[0:n], ".")+".*")
	}
	return
}

func (nc *NexusConn) topicPublish(topic string, message interface{}) (int, error) {
	msg := ei.M{"topic": topic, "msg": message}
	res, err := r.Table("pipes").
		GetAllByIndex("subs", topicList(topic)...).
		Update(map[string]interface{}{"msg": r.Literal(msg), "count": r.Row.Field("count").Add(1), "ismsg": true}).
		RunWrite(db, r.RunOpts{Durability: "soft"})
	return res.Replaced, err
}

func (nc *NexusConn) handleTopicReq(req *JsonRpcReq) {
	switch req.Method {
	case "topic.sub":
		pipeid, err := ei.N(req.Params).M("pipeid").String()
		if err != nil {
			req.Error(ErrInvalidParams, "pipeid", nil)
			return
		}
		topic, err := ei.N(req.Params).M("topic").Lower().String()
		if err != nil {
			req.Error(ErrInvalidParams, "topic", nil)
			return
		}
		tags := nc.getTags(topic)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		res, err := r.Table("pipes").
			Get(pipeid).
			Update(map[string]interface{}{
				"subs":  r.Row.Field("subs").Default(ei.S{}).SetInsert(topic),
				"ismsg": false,
				"msg":   nil,
			}).
			RunWrite(db, r.RunOpts{Durability: "hard"})
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if res.Unchanged == 0 && res.Replaced == 0 {
			req.Error(ErrInvalidPipe, "", nil)
			return
		}
		req.Result(map[string]interface{}{"ok": true})
	case "topic.unsub":
		pipeid, err := ei.N(req.Params).M("pipeid").String()
		if err != nil {
			req.Error(ErrInvalidParams, "pipeid", nil)
			return
		}
		topic, err := ei.N(req.Params).M("topic").Lower().String()
		if err != nil {
			req.Error(ErrInvalidParams, "topic", nil)
			return
		}
		tags := nc.getTags(topic)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		res, err := r.Table("pipes").
			Get(pipeid).
			Update(map[string]interface{}{
				"subs":  r.Row.Field("subs").Default(ei.S{}).Difference(ei.S{topic}),
				"ismsg": false,
				"msg":   nil,
			}).
			RunWrite(db, r.RunOpts{Durability: "hard"})
		if err != nil {
			req.Error(ErrInternal, err.Error(), nil)
			return
		}
		if res.Unchanged == 0 && res.Replaced == 0 {
			req.Error(ErrInvalidPipe, "", nil)
			return
		}
		req.Result(map[string]interface{}{"ok": true})

	case "topic.pub":
		topic, err := ei.N(req.Params).M("topic").Lower().String()
		if err != nil {
			req.Error(ErrInvalidParams, "topic", nil)
			return
		}
		msg, err := ei.N(req.Params).M("msg").Raw()
		if err != nil {
			req.Error(ErrInvalidParams, "msg", nil)
			return
		}

		tags := nc.getTags(topic)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		sent, err := nc.topicPublish(topic, msg)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		req.Result(map[string]interface{}{"ok": true, "sent": sent})
	default:
		req.Error(ErrMethodNotFound, "", nil)

	}
}
