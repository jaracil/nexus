package main

import (
	"fmt"
	"strings"

	"github.com/jaracil/ei"
	r "gopkg.in/gorethink/gorethink.v3"
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

func topicPublish(topic string, message interface{}) (int, error) {
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

		sent, err := topicPublish(topic, msg)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		req.Result(map[string]interface{}{"ok": true, "sent": sent})

	case "topic.list":
		prefix, depth, filter, limit, skip := getListParams(req.Params)

		tags := nc.getTags(prefix)
		if !(ei.N(tags).M("@sync.list").BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		term := r.Table("pipes").Between(prefix, prefix+"\uffff", r.BetweenOpts{Index: "subs"}).
			Distinct(r.DistinctOpts{Index: "id"}).Distinct().
			EqJoin(func(t r.Term) r.Term { return t }, r.Table("pipes")).Field("right").Field("subs")

		if prefix != "" || depth >= 0 || filter != "" {
			term = term.Map(func(t r.Term) r.Term {
				return t.Filter(func(t r.Term) r.Term {
					var filtTerm r.Term
					if prefix == "" {
						if depth < 0 {
							return t.Match(filter)
						} else if depth == 0 {
							return t.Match("^$")
						} else if depth == 1 {
							filtTerm = t.Match("^[^.]*$")
						} else {
							filtTerm = t.Match(fmt.Sprintf("^[^.]*([.][^.]*){0,%d}$", depth-1))
						}
					} else {
						if depth < 0 {
							filtTerm = t.Match(fmt.Sprintf("^%s([.][^.]*)*$", prefix))
						} else {
							filtTerm = t.Match(fmt.Sprintf("^%s([.][^.]*){0,%d}$", prefix, depth))
						}
					}
					if filter != "" {
						filtTerm = filtTerm.And(t.Match(filter))
					}
					return filtTerm
				})
			})
		}

		term = term.Reduce(func(left r.Term, right r.Term) r.Term { return left.Add(right) }).Default([]interface{}{}).Group(func(t r.Term) r.Term { return t }).Count().Ungroup().
			Map(func(t r.Term) r.Term { return r.Object("topic", t.Field("group"), "subscribers", t.Field("reduction")) })

		if skip >= 0 {
			term = term.Skip(skip)
		}
		if limit > 0 {
			term = term.Limit(limit)
		}

		cur, err := term.Run(db, r.RunOpts{})
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

	default:
		req.Error(ErrMethodNotFound, "", nil)

	}
}
