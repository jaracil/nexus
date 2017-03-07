package main

import (
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jaracil/ei"
	. "github.com/jaracil/nexus/log"
	r "gopkg.in/gorethink/gorethink.v3"
)

type Pipe struct {
	Id           string      `gorethink:"id"`
	Msg          interface{} `gorethink:"msg"`
	Count        int64       `gorethink:"count"`
	IsMsg        bool        `gorethink:"ismsg"`
	CreationTime interface{} `gorethink:"creationTime,omitempty"`
}

type PipeFeed struct {
	Old *Pipe `gorethink:"old_val"`
	New *Pipe `gorethink:"new_val"`
}

func pipeTrack() {
	defer exit("pipe change-feed error")
	for retry := 0; retry < 10; retry++ {
		iter, err := r.Table("pipes").
			Between(nodeId, nodeId+"\uffff").
			Changes(r.ChangesOpts{IncludeInitial: true, Squash: false}).
			Pluck(map[string]interface{}{
				"new_val": []string{"id", "msg", "count", "ismsg"},
				"old_val": []string{"id"}}).
			Run(db)
		if err != nil {
			Log.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Printf("Error opening pipeTrack iterator")
			time.Sleep(time.Second)
			continue
		}
		retry = 0
		for {
			pf := &PipeFeed{}
			if !iter.Next(pf) {
				Log.WithFields(logrus.Fields{
					"error": iter.Err().Error(),
				}).Printf("Error processing pipeTrack feed")
				iter.Close()
				break
			}
			if pf.New == nil { // Deleted pipe
				sesNotify.Unregister(pf.Old.Id)
				continue
			}
			if pf.New.IsMsg {
				sesNotify.Notify(pf.New.Id, pf.New)
			}
		}
	}
}

func (nc *NexusConn) handlePipeReq(req *JsonRpcReq) {
	switch req.Method {
	case "pipe.create":
		pipeid := nc.connId + safeId(10)
		length := ei.N(req.Params).M("len").IntZ()
		if length <= 0 {
			length = opts.Rethink.DefPipeLen
		}
		if length > opts.Rethink.MaxPipeLen {
			length = opts.Rethink.MaxPipeLen
		}
		_, err := sesNotify.Register(pipeid, make(chan interface{}, length))
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}

		pipe := &Pipe{
			Id:           pipeid,
			Msg:          nil,
			Count:        0,
			IsMsg:        false,
			CreationTime: r.Now(),
		}
		_, err = r.Table("pipes").Insert(pipe).RunWrite(db, r.RunOpts{Durability: "hard"})
		if err != nil {
			sesNotify.Unregister(pipeid)
			req.Error(ErrInternal, "", nil)
			return
		}
		req.Result(map[string]interface{}{"pipeid": pipeid})
	case "pipe.close":
		pipeid := ei.N(req.Params).M("pipeid").StringZ()
		if pipeid == "" || !strings.HasPrefix(pipeid, nc.connId) {
			req.Error(ErrInvalidPipe, "", nil)
			return
		}
		sesNotify.Unregister(pipeid)
		res, err := r.Table("pipes").Get(pipeid).Delete().RunWrite(db, r.RunOpts{Durability: "hard"})
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if res.Deleted > 0 {
			req.Result(map[string]interface{}{"ok": true})
		} else {
			req.Error(ErrInvalidPipe, "", nil)
		}
	case "pipe.write":
		pipeid := ei.N(req.Params).M("pipeid").StringZ()
		if pipeid == "" {
			req.Error(ErrInvalidPipe, "", nil)
			return
		}

		var msgs []interface{}
		var err error
		if !ei.N(req.Params).M("multi").BoolZ() {
			msgs = []interface{}{ei.N(req.Params).M("msg").RawZ()}
		} else {
			if msgs, err = ei.N(req.Params).M("msg").Slice(); err != nil {
				req.Error(ErrInvalidParams, "multi is true. msg should be an array", nil)
				return
			}
		}

		for _, msg := range msgs {
			res, err := r.Table("pipes").
				Get(pipeid).
				Update(map[string]interface{}{"msg": r.Literal(msg), "count": r.Row.Field("count").Add(1), "ismsg": true}).
				RunWrite(db, r.RunOpts{Durability: "soft"})
			if err != nil {
				req.Error(ErrInternal, "", nil)
				return
			}

			if res.Replaced <= 0 {
				req.Error(ErrInvalidPipe, "", nil)
				return
			}
		}
		req.Result(map[string]interface{}{"ok": true})

	case "pipe.read":
		pipeid := ei.N(req.Params).M("pipeid").StringZ()
		if pipeid == "" || !strings.HasPrefix(pipeid, nc.connId) {
			req.Error(ErrInvalidPipe, "", nil)
			return
		}
		max := ei.N(req.Params).M("max").IntZ()
		if max <= 0 {
			max = 10
		}
		ch, err := sesNotify.Channel(pipeid)
		if err != nil {
			req.Error(ErrInvalidPipe, "", nil)
			return
		}
		var toutCh <-chan time.Time
		timeout := ei.N(req.Params).M("timeout").Float64Z()
		if timeout > 0 {
			toutCh = time.After(time.Duration(timeout * float64(time.Second)))
		}
		messages := make([]interface{}, 0, 10)
		tout := false
		for (len(messages) == 0 || (len(messages) < max && len(ch) > 0)) && !tout {
			select {
			case m, ok := <-ch:
				if !ok {
					tout = true
					break
				}
				pipe := m.(*Pipe)
				messages = append(messages, map[string]interface{}{"msg": pipe.Msg, "count": pipe.Count})
			case <-toutCh:
				tout = true
			case <-nc.context.Done():
				req.Error(ErrCancel, "", nil)
				return
			}
		}
		drops, _ := sesNotify.Drops(pipeid, true)
		req.Result(map[string]interface{}{"msgs": messages, "waiting": len(ch), "drops": drops})
	default:
		req.Error(ErrMethodNotFound, "", nil)
	}
}
