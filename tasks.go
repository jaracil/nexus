package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/jaracil/ei"
	. "github.com/jaracil/nexus/log"
	"github.com/sirupsen/logrus"
	r "gopkg.in/gorethink/gorethink.v5"
)

type Task struct {
	Id           string      `gorethink:"id" json:"id"`
	Stat         string      `gorethink:"stat" json:"state""`
	Path         string      `gorethink:"path" json:"path"`
	Prio         int         `gorethink:"prio" json:"priority"`
	Ttl          int         `gorethink:"ttl" json:"ttl"`
	Detach       bool        `gorethink:"detach" json:"detached"`
	User         string      `gorethink:"user" json:"user"`
	Method       string      `gorethink:"method" json:"method"`
	Params       interface{} `gorethink:"params" json:"params"`
	LocalId      interface{} `gorethink:"localId" json:"-"`
	Tses         string      `gorethink:"tses" json:"targetSession"`
	Result       interface{} `gorethink:"result,omitempty" json:"result"`
	ErrCode      *int        `gorethink:"errCode,omitempty" json:"errCode"`
	ErrStr       string      `gorethink:"errStr,omitempty" json:"errString"`
	ErrObj       interface{} `gorethink:"errObj,omitempty" json:"errObject"`
	Tags         interface{} `gorethink:"tags,omitempty" json:"tags"`
	CreationTime interface{} `gorethink:"creationTime,omitempty" json:"creationTime"`
	WorkingTime  interface{} `gorethink:"workingTime,omitempty" json:"workingTime"`
	DeadLine     interface{} `gorethink:"deadLine,omitempty" json:"deadline"`
}

type TaskFeed struct {
	Old *Task `gorethink:"old_val"`
	New *Task `gorethink:"new_val"`
}

func taskPurge() {
	defer exit("purge goroutine error")
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			if isMasterNode() {
				wres, err := r.Table("tasks").
					Between(r.MinVal, r.Now(), r.BetweenOpts{Index: "deadLine"}).
					Update(r.Branch(r.Row.Field("stat").Ne("done"),
						ei.M{"stat": "done", "errCode": ErrTimeout, "errStr": ErrStr[ErrTimeout], "deadLine": r.Now().Add(600)},
						ei.M{}),
						r.UpdateOpts{ReturnChanges: true}).
					RunWrite(db, r.RunOpts{Durability: "soft"})
				if err == nil {
					for _, change := range wres.Changes {
						task := ei.N(change.OldValue)
						if path := task.M("path").StringZ(); !strings.HasPrefix(path, "@pull.") {
							hook("task", path+task.M("method").StringZ(), task.M("user").StringZ(), ei.M{
								"action":    "timeout",
								"id":        task.M("id").StringZ(),
								"timestamp": time.Now().UTC(),
							})
						}
					}
				}

				r.Table("tasks").
					Between(r.MinVal, r.Now(), r.BetweenOpts{Index: "deadLine"}).
					Filter(r.Row.Field("stat").Eq("done")).
					Delete().
					RunWrite(db, r.RunOpts{Durability: "soft"})
			}
		case <-mainContext.Done():
			return
		}
	}
}

func taskTrack() {
	defer exit("task change-feed error")
	for retry := 0; retry < 10; retry++ {
		iter, err := r.Table("tasks").
			Between(nodeId, nodeId+"\uffff").
			Changes(r.ChangesOpts{IncludeInitial: true, Squash: false}).
			Filter(r.Row.Field("new_val").Ne(nil)).
			Pluck(ei.M{"new_val": []string{
				"id",
				"stat",
				"localId",
				"detach",
				"user",
				"prio",
				"ttl",
				"path",
				"method",
				"result",
				"errCode",
				"errStr",
				"errObj",
				"tses",
				"creationTime",
				"workingTime"}}).
			Run(db)
		if err != nil {
			Log.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Errorln("Error opening taskTrack iterator")
			time.Sleep(time.Second)
			continue
		}
		retry = 0 //Reset retrys
		for {
			tf := &TaskFeed{}
			if !iter.Next(tf) {
				Log.WithFields(logrus.Fields{
					"error": iter.Err().Error(),
				}).Println("Error processing taskTrack feed")
				iter.Close()
				break
			}
			task := tf.New
			switch task.Stat {
			case "done":
				if !task.Detach {
					sesNotify.Notify(task.Id[0:16], task)
				}
				go deleteTask(task.Id)
			case "working":
				if strings.HasPrefix(task.Path, "@pull.") {
					go taskPull(task)
				}
			case "waiting":
				if !strings.HasPrefix(task.Path, "@pull.") {
					if task.Ttl <= 0 {
						go taskExpireTtl(task.Id)
					} else {
						go taskWakeup(task)
					}
				}
			}
		}
	}
}

func taskPull(task *Task) bool {
	prefix := task.Path
	if strings.HasPrefix(prefix, "@pull.") {
		prefix = prefix[6:]
	}
	for {
		wres, err := r.Table("tasks").
			OrderBy(r.OrderByOpts{Index: "pspc"}).
			Between(ei.S{prefix, "waiting", r.MinVal, r.MinVal}, ei.S{prefix, "waiting", r.MaxVal, r.MaxVal}, r.BetweenOpts{RightBound: "closed", Index: "pspc"}).
			Limit(1).
			Update(r.Branch(r.Row.Field("stat").Eq("waiting"),
				ei.M{"stat": "working", "tses": task.Id[0:16], "workingTime": r.Now()},
				ei.M{}),
				r.UpdateOpts{ReturnChanges: true}).
			RunWrite(db, r.RunOpts{Durability: "soft"})
		if err != nil {
			break
		}
		if wres.Replaced > 0 {
			newTask := ei.N(wres.Changes[0].NewValue)
			result := make(ei.M)
			result["taskid"] = newTask.M("id").StringZ()
			result["path"] = newTask.M("path").StringZ()
			result["method"] = newTask.M("method").StringZ()
			result["params"] = newTask.M("params").RawZ()
			result["tags"] = newTask.M("tags").MapStrZ()
			result["prio"] = -newTask.M("prio").IntZ()
			result["detach"] = newTask.M("detach").BoolZ()
			result["user"] = newTask.M("user").StringZ()
			pres, err := r.Table("tasks").
				Get(task.Id).
				Update(r.Branch(r.Row.Field("stat").Eq("working"),
					ei.M{"stat": "done", "result": result, "deadLine": r.Now().Add(600)},
					ei.M{})).
				RunWrite(db, r.RunOpts{Durability: "soft"})
			if err != nil || pres.Replaced != 1 {
				r.Table("tasks").
					Get(result["taskid"]).
					Update(ei.M{"stat": "waiting"}).
					RunWrite(db, r.RunOpts{Durability: "soft"})
				break
			}
			hook("task", newTask.M("path").StringZ()+newTask.M("method").StringZ(), newTask.M("user").StringZ(), ei.M{
				"action":    "pull",
				"id":        result["taskid"],
				"connid":    task.Id[0:16],
				"user":      task.User,
				"ttl":       newTask.M("ttl").IntZ(),
				"timestamp": time.Now().UTC(),
			})
			return true
		}
		if wres.Unchanged > 0 {
			continue
		}
		break
	}

	r.Table("tasks").
		Get(task.Id).
		Update(r.Branch(r.Row.Field("stat").Eq("working"),
			ei.M{"stat": "waiting"},
			ei.M{})).
		RunWrite(db, r.RunOpts{Durability: "soft"})

	// On the previous step where the pull transitions from working to waiting
	// there is a race condition where a push could enter and a single pull on that
	// path wouldnt be able to notice, and a deadlock would happen.
	// Here we check again for any task waiting that we could accept, and set ourselves
	// as working again to restart the loop on taskTrack()

	stuck, _ := r.Table("tasks").
		OrderBy(r.OrderByOpts{Index: "pspc"}).
		Between(ei.S{prefix, "waiting", r.MinVal, r.MinVal}, ei.S{prefix, "waiting", r.MaxVal, r.MaxVal}, r.BetweenOpts{RightBound: "closed", Index: "pspc"}).
		Limit(1).
		Run(db, r.RunOpts{Durability: "soft"})

	if !stuck.IsNil() {
		r.Table("tasks").
			Get(task.Id).
			Update(r.Branch(r.Row.Field("stat").Eq("waiting"),
				ei.M{"stat": "working"},
				ei.M{})).
			RunWrite(db, r.RunOpts{Durability: "soft"})
	}

	return false
}

func taskWakeup(task *Task) bool {
	for {
		wres, err := r.Table("tasks").
			Between(ei.S{"@pull." + task.Path, "waiting", r.MinVal, r.MinVal},
				ei.S{"@pull." + task.Path, "waiting", r.MaxVal, r.MaxVal},
				r.BetweenOpts{RightBound: "closed", Index: "pspc"}).
			Sample(1).
			Update(r.Branch(r.Row.Field("stat").Eq("waiting"),
				ei.M{"stat": "working", "workingTime": r.Now()},
				ei.M{})).
			RunWrite(db, r.RunOpts{Durability: "soft"})
		if err != nil {
			return false
		}
		if wres.Replaced > 0 {
			return true
		}
		if wres.Unchanged > 0 {
			continue
		}
		break
	}
	return false
}

func deleteTask(id string) {
	r.Table("tasks").Get(id).Delete().RunWrite(db, r.RunOpts{Durability: "soft"})
}

func taskExpireTtl(taskid string) {
	wres, err := r.Table("tasks").
		Get(taskid).
		Update(ei.M{"stat": "done", "errCode": ErrTtlExpired, "errStr": ErrStr[ErrTtlExpired], "deadLine": r.Now().Add(600)}, r.UpdateOpts{ReturnChanges: true}).
		RunWrite(db, r.RunOpts{Durability: "soft"})
	if err == nil {
		for _, change := range wres.Changes {
			task := ei.N(change.OldValue)
			hook("task", task.M("path").StringZ()+task.M("method").StringZ(), task.M("user").StringZ(), ei.M{
				"action":    "ttlExpired",
				"id":        task.M("id").StringZ(),
				"timestamp": time.Now().UTC(),
			})
		}
	}
}

func (nc *NexusConn) handleTaskReq(req *JsonRpcReq) {
	switch req.Method {
	case "task.push":
		method, err := ei.N(req.Params).M("method").Lower().F(checkRegexp, _taskRegexp).F(checkNotEmptyLabels).String()
		if err != nil {
			req.Error(ErrInvalidParams, "method", nil)
			return
		}
		params, err := ei.N(req.Params).M("params").Raw()
		if err != nil {
			req.Error(ErrInvalidParams, "params", nil)
			return
		}
		prio := -ei.N(req.Params).M("prio").IntZ()
		ttl := ei.N(req.Params).M("ttl").IntZ()
		if ttl <= 0 {
			ttl = 5
		}
		detach := ei.N(req.Params).M("detach").BoolZ()
		tags := nc.getTags(method)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		path, met := getPathMethod(method)
		timeout := ei.N(req.Params).M("timeout").Float64Z()
		if timeout <= 0 {
			timeout = 60 * 60 * 24 * 10 // Ten days
		}
		task := &Task{
			Id:           nc.connId + safeId(10),
			Stat:         "waiting",
			Path:         path,
			Prio:         prio,
			Ttl:          ttl,
			Detach:       detach,
			Method:       met,
			Params:       params,
			Tags:         tags,
			User:         nc.user.User,
			LocalId:      req.Id,
			CreationTime: r.Now(),
			DeadLine:     r.Now().Add(timeout),
		}
		nc.log.WithFields(logrus.Fields{
			"connid": req.nc.connId,
			"id":     req.Id,
			"taskid": task.Id,
		}).Info("taskid generated")

		_, err = r.Table("tasks").Insert(task, r.InsertOpts{}).RunWrite(db, r.RunOpts{Durability: "soft"})
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		hook("task", task.Path+task.Method, task.User, ei.M{
			"action":       "push",
			"id":           task.Id,
			"connid":       nc.connId,
			"user":         nc.user.User,
			"tags":         nc.user.Tags,
			"path":         path,
			"method":       met,
			"params":       params,
			"detach":       detach,
			"ttl":          ttl,
			"prio":         prio,
			"creationTime": time.Now().UTC(),
			"timeout":      timeout,
		})
		if detach {
			req.Result(ei.M{"ok": true})
		}

	case "task.pull":
		if req.Id == nil {
			return
		}
		prefix := ei.N(req.Params).M("prefix").Lower().F(checkRegexp, _taskRegexp).F(checkNotEmptyLabels).StringZ()
		if prefix == "" {
			req.Error(ErrInvalidParams, "prefix", nil)
			return
		}
		if !strings.HasSuffix(prefix, ".") {
			prefix += "."
		}
		tags := nc.getTags(prefix)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		timeout := ei.N(req.Params).M("timeout").Float64Z()
		if timeout <= 0 {
			timeout = 60 * 60 * 24 * 10 // Ten days
		}
		task := &Task{
			Id:           nc.connId + safeId(10),
			Stat:         "working",
			Path:         "@pull." + prefix,
			Method:       "",
			Params:       nil,
			LocalId:      req.Id,
			CreationTime: r.Now(),
			DeadLine:     r.Now().Add(timeout),
			User:         nc.user.User,
		}
		_, err := r.Table("tasks").Insert(task).RunWrite(db, r.RunOpts{Durability: "soft"})
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}

	case "task.result":
		taskid := ei.N(req.Params).M("taskid").StringZ()
		result := ei.N(req.Params).M("result").RawZ()
		res, err := r.Table("tasks").
			Get(taskid).
			Update(ei.M{"stat": "done", "result": result, "deadLine": r.Now().Add(600)}, r.UpdateOpts{ReturnChanges: true}).
			RunWrite(db, r.RunOpts{Durability: "soft"})
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if res.Replaced > 0 {
			task := ei.N(res.Changes[0].OldValue)
			hook("task", task.M("path").StringZ()+task.M("method").StringZ(), task.M("user").StringZ(), ei.M{
				"action":    "result",
				"id":        taskid,
				"result":    result,
				"timestamp": time.Now().UTC(),
			})
			req.Result(ei.M{"ok": true})
		} else {
			req.Error(ErrInvalidTask, "", nil)
		}

	case "task.error":
		taskid := ei.N(req.Params).M("taskid").StringZ()
		code := ei.N(req.Params).M("code").IntZ()
		message := ei.N(req.Params).M("message").StringZ()
		data := ei.N(req.Params).M("data").RawZ()
		res, err := r.Table("tasks").
			Get(taskid).
			Update(ei.M{"stat": "done", "errCode": code, "errStr": message, "errObj": data, "deadLine": r.Now().Add(600)}, r.UpdateOpts{ReturnChanges: true}).
			RunWrite(db, r.RunOpts{Durability: "soft"})
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if res.Replaced > 0 {
			task := ei.N(res.Changes[0].OldValue)
			hook("task", task.M("path").StringZ()+task.M("method").StringZ(), task.M("user").StringZ(), ei.M{
				"action":    "error",
				"id":        taskid,
				"code":      code,
				"message":   message,
				"data":      data,
				"timestamp": time.Now().UTC(),
			})
			req.Result(ei.M{"ok": true})
		} else {
			req.Error(ErrInvalidTask, "", nil)
		}

	case "task.reject":
		taskid := ei.N(req.Params).M("taskid").StringZ()
		res, err := r.Table("tasks").
			Get(taskid).
			Update(ei.M{"stat": "waiting", "tses": nil, "ttl": r.Row.Field("ttl").Add(-1)}, r.UpdateOpts{ReturnChanges: true}).
			RunWrite(db, r.RunOpts{Durability: "soft"})
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if res.Replaced > 0 {
			task := ei.N(res.Changes[0].OldValue)
			hook("task", task.M("path").StringZ()+task.M("method").StringZ(), task.M("user").StringZ(), ei.M{
				"action":    "reject",
				"id":        taskid,
				"timestamp": time.Now().UTC(),
			})
			req.Result(ei.M{"ok": true})
		} else {
			req.Error(ErrInvalidTask, "", nil)
		}

	case "task.cancel":
		id := ei.N(req.Params).M("id").RawZ()
		wres, err := r.Table("tasks").
			Between(nc.connId, nc.connId+"\uffff").
			Filter(r.Row.Field("localId").Eq(id)).
			Update(r.Branch(r.Row.Field("stat").Ne("done"),
				ei.M{"stat": "done", "errCode": ErrCancel, "errStr": ErrStr[ErrCancel], "deadLine": r.Now().Add(600)},
				ei.M{}),
				r.UpdateOpts{ReturnChanges: true}).
			RunWrite(db, r.RunOpts{Durability: "soft"})

		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if wres.Replaced > 0 {
			task := ei.N(wres.Changes[0].NewValue)
			hook("task", task.M("path").StringZ()+task.M("method").StringZ(), task.M("user").StringZ(), ei.M{
				"action":    "cancel",
				"id":        task.M("taskid").StringZ(),
				"timestamp": time.Now().UTC(),
			})
			req.Result(ei.M{"ok": true})
		} else {
			req.Error(ErrInvalidTask, "", nil)
		}

	case "task.list":
		prefix, depth, filter, limit, skip := getListParams(req.Params)

		tags := nc.getTags(prefix)
		if !(ei.N(tags).M("@task.list").BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		var term r.Term
		if prefix == "" {
			if depth < 0 {
				term = r.Table("tasks")
			} else if depth == 0 {
				term = r.Table("tasks").GetAllByIndex("path", ".", "@pull.")
			} else {
				term = r.Table("tasks").Filter(r.Row.Field("path").Match(fmt.Sprintf("^(?:@pull[.])??(?:[^.]*[.]){0,%d}$", depth)))
			}
		} else {
			if depth != 0 {
				term = r.Table("tasks").Between(prefix+".", prefix+".\uffff", r.BetweenOpts{Index: "path"}).Union(r.Table("tasks").Between("@pull."+prefix+".", "@pull."+prefix+".\uffff", r.BetweenOpts{Index: "path"}))
			} else {
				term = r.Table("tasks").GetAllByIndex("path", prefix+".", "@pull."+prefix+".")
			}
			if depth > 0 {
				term = term.Filter(r.Row.Field("path").Match(fmt.Sprintf("^%s(?:[.][^.]*){0,%d}[.]$", prefix, depth)))
			}
		}
		if filter != "" {
			term = term.Filter(r.Row.Field("path").Match(filter))
		}
		if skip >= 0 {
			term = term.Skip(skip)
		}
		if limit > 0 {
			term = term.Limit(limit)
		}

		cur, err := term.Run(db)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		ret := make([]*Task, 0)
		if err := cur.All(&ret); err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}

		for _, task := range ret {
			task.Path = strings.TrimPrefix(task.Path, "@pull.")
			task.Params = truncateJson(task.Params)
			task.ErrObj = truncateJson(task.ErrObj)
		}

		req.Result(ret)

	case "task.count":
		prefix := getPrefixParam(req.Params)
		filter := ei.N(req.Params).M("filter").StringZ()
		countSubprefixes := ei.N(req.Params).M("subprefixes").BoolZ()

		tags := nc.getTags(prefix)
		if !(ei.N(tags).M("@task.count").BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		var pushTerm, pullTerm, term r.Term
		if countSubprefixes {
			if prefix == "" {
				pushTerm = r.Table("tasks")
				pullTerm = r.Table("tasks").Between("@pull.", "@pull.\uffff", r.BetweenOpts{Index: "path"})
				if filter != "" {
					pushTerm = pushTerm.Filter(r.Row.Field("path").Match(filter))
					pullTerm = pullTerm.Filter(r.Row.Field("path").Match(filter))
				}

				pushTerm = pushTerm.Group(r.Row.Field("path").Match("^([^@.][^.]*)[.](?:[^.]*[.])*$").Field("groups").Nth(0).Field("str")).Count().Ungroup().Filter(func(t r.Term) r.Term {
					return t.HasFields("group")
				}).Map(func(t r.Term) r.Term {
					return r.Object("prefix", t.Field("group"), "count", t.Field("reduction"))
				})

				pullTerm = pullTerm.Group(r.Row.Field("path").Match("^@pull[.]([^.]*)[.](?:[^.]*[.])*$").Field("groups").Nth(0).Field("str")).Count().Ungroup().Filter(func(t r.Term) r.Term {
					return t.HasFields("group")
				}).Map(func(t r.Term) r.Term {
					return r.Object("prefix", t.Field("group"), "count", t.Field("reduction"))
				})
			} else {
				pushTerm = r.Table("tasks").Between(prefix+".", prefix+".\uffff", r.BetweenOpts{Index: "path"})
				pullTerm = r.Table("tasks").Between("@pull."+prefix+".", "@pull."+prefix+".\uffff", r.BetweenOpts{Index: "path"})
				if filter != "" {
					pushTerm = pushTerm.Filter(r.Row.Field("path").Match(filter))
					pullTerm = pullTerm.Filter(r.Row.Field("path").Match(filter))
				}
				pushTerm = pushTerm.Group(r.Row.Field("path").Match(fmt.Sprintf("^(%s(?:[.][^.]*)?)[.](?:[^.]*[.])*$", prefix)).Field("groups").Nth(0).Field("str")).Count().Ungroup().Filter(func(t r.Term) r.Term {
					return t.HasFields("group")
				}).Map(func(t r.Term) r.Term {
					return r.Object("prefix", t.Field("group"), "count", t.Field("reduction"))
				})
				pullTerm = pullTerm.Group(r.Row.Field("path").Match(fmt.Sprintf("^@pull[.](%s(?:[.][^.]*)?)[.](?:[^.]*[.])*$", prefix)).Field("groups").Nth(0).Field("str")).Count().Ungroup().Filter(func(t r.Term) r.Term {
					return t.HasFields("group")
				}).Map(func(t r.Term) r.Term {
					return r.Object("prefix", t.Field("group"), "count", t.Field("reduction"))
				})
			}
			pushCur, err := pushTerm.Run(db)
			if err != nil {
				req.Error(ErrInternal, err.Error(), nil)
				return
			}
			var pushAll []interface{}
			if err := pushCur.All(&pushAll); err != nil {
				req.Error(ErrInternal, "", nil)
				return
			}
			pullCur, err := pullTerm.Run(db)
			if err != nil {
				req.Error(ErrInternal, err.Error(), nil)
				return
			}
			var pullAll []interface{}
			if err := pullCur.All(&pullAll); err != nil {
				req.Error(ErrInternal, "", nil)
				return
			}

			res := []interface{}{}
			countPulls := map[string]int{}
			for _, v := range pullAll {
				countPulls[ei.N(v).M("prefix").StringZ()] = ei.N(v).M("count").IntZ()
			}
			for _, v := range pushAll {
				p := ei.N(v).M("prefix").StringZ()
				if !strings.HasPrefix(p, "@pull.") {
					pullCount := countPulls[p]
					delete(countPulls, p)
					pushCount := ei.N(v).M("count").IntZ()
					res = append(res, ei.M{"prefix": p, "count": pushCount + pullCount, "pullCount": pullCount, "pushCount": pushCount})
				}
			}
			for p, v := range countPulls {
				res = append(res, ei.M{"prefix": p, "count": v, "pullCount": v, "pushCount": 0})
			}
			req.Result(res)

		} else {
			if prefix == "" {
				term = r.Table("tasks")
			} else {
				term = r.Table("tasks").Between(prefix+".", prefix+".\uffff", r.BetweenOpts{Index: "path"}).Union(r.Table("tasks").Between("@pull."+prefix+".", "@pull."+prefix+".\uffff", r.BetweenOpts{Index: "path"}))
			}
			if filter != "" {
				term = term.Filter(r.Row.Field("path").Match(filter))
			}
			term = term.Count()

			cur, err := term.Run(db)
			if err != nil {
				req.Error(ErrInternal, err.Error(), nil)
				return
			}
			var count int
			if err := cur.One(&count); err != nil {
				req.Error(ErrInternal, "", nil)
				return
			}

			if prefix == "" {
				term = r.Table("tasks").Between("@pull.", "@pull.\uffff", r.BetweenOpts{Index: "path"})
			} else {
				term = r.Table("tasks").Between("@pull."+prefix+".", "@pull."+prefix+".\uffff", r.BetweenOpts{Index: "path"})
			}
			if filter != "" {
				term = term.Filter(r.Row.Field("path").Match(filter))
			}
			term = term.Count()

			cur, err = term.Run(db)
			if err != nil {
				req.Error(ErrInternal, err.Error(), nil)
				return
			}
			var countPulls int
			if err := cur.One(&countPulls); err != nil {
				req.Error(ErrInternal, "", nil)
				return
			}

			countPushes := count - countPulls
			if countPushes < 0 {
				countPushes = 0
			}

			req.Result(ei.M{"count": count, "pullCount": countPulls, "pushCount": countPushes})
		}

	default:
		req.Error(ErrMethodNotFound, "", nil)
	}
}
