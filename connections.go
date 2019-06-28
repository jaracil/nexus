package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/jaracil/ei"
	. "github.com/jaracil/nexus/log"
	"github.com/jaracil/smartio"
	"github.com/nayarsystems/nxgo/nxcore"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	r "gopkg.in/gorethink/gorethink.v5"
)

type JsonRpcErr struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type JsonRpcReq struct {
	Jsonrpc string      `json:"jsonrpc"`
	Id      interface{} `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`

	nc *NexusConn
}

type JsonRpcRes struct {
	Jsonrpc string      `json:"jsonrpc"`
	Id      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *JsonRpcErr `json:"error,omitempty"`

	req *JsonRpcReq
}

type NexusConn struct {
	conn      net.Conn
	proto     string
	connRx    *smartio.SmartReader
	connTx    *smartio.SmartWriter
	connId    string
	user      *UserData
	chRes     chan *JsonRpcRes
	chReq     chan *JsonRpcReq
	context   context.Context
	cancelFun context.CancelFunc
	wdog      int64
	closed    int32 //Atomic bool
	log       *logrus.Entry
}

func NewNexusConn(conn net.Conn) *NexusConn {
	nc := &NexusConn{
		conn:   conn,
		proto:  "unknown",
		connRx: smartio.NewSmartReader(conn),
		connTx: smartio.NewSmartWriter(conn),
		connId: nodeId + safeId(4),
		user:   Nobody,
		chRes:  make(chan *JsonRpcRes, 64),
		chReq:  make(chan *JsonRpcReq, 64),
		wdog:   90,
		log:    Log,
	}
	nc.context, nc.cancelFun = context.WithCancel(mainContext)
	return nc
}

func NewInternalClient() *nxcore.NexusConn {
	client, server := net.Pipe()

	nc := NewNexusConn(server)

	if len(opts.Verbose) < 2 {
		nc.log = LogDiscard()
	}

	nc.proto = "internal"
	nc.user = &UserData{
		User: "@internal",
		Tags: map[string]map[string]interface{}{
			".": map[string]interface{}{
				"@admin": true,
			},
		},
	}
	go nc.handle()

	return nxcore.NewNexusConn(client)
}

func (req *JsonRpcReq) Error(code int, message string, data interface{}) {
	if code < 0 {
		if message != "" {
			message = fmt.Sprintf("%s:[%s]", ErrStr[code], message)
		} else {
			message = ErrStr[code]
		}
	}
	req.nc.pushRes(
		&JsonRpcRes{
			Id:    req.Id,
			Error: &JsonRpcErr{Code: code, Message: message, Data: data},
			req:   req,
		},
	)
}

func (req *JsonRpcReq) Result(result interface{}) {
	req.nc.pushRes(
		&JsonRpcRes{
			Id:     req.Id,
			Result: result,
			req:    req,
		},
	)

}

func (nc *NexusConn) logRes(res *JsonRpcRes) {
	if res.req != nil {
		wf := nc.log.WithFields(logrus.Fields{
			"connid": nc.connId,
			"id":     res.Id,
			"type":   "response",
			"remote": nc.conn.RemoteAddr().String(),
			"proto":  nc.proto,
			"method": res.req.Method,
		})
		switch res.req.Method {
		// Do not log verbose actions
		case "pipe.read", "pipe.write", "sys.ping":
			if !LogLevelIs(DebugLevel) {
				return
			}
		}

		if res.Error != nil {
			wf.WithFields(logrus.Fields{
				"code":    res.Error.Code,
				"message": res.Error.Message,
				"data":    res.Error.Data,
			}).Info("<< error")

		} else {
			switch res.req.Method {

			// Do not log verbose results
			case "user.list", "task.list", "sys.session.list":

			default:
				wf = wf.WithFields(logrus.Fields{
					"result": res.Result,
				})
			}
			wf.Info("<< result")
		}
	}
}

func (nc *NexusConn) logReq(req *JsonRpcReq) {
	e := nc.log.WithFields(logrus.Fields{
		"connid": req.nc.connId,
		"id":     req.Id,
		"method": req.Method,
		"remote": req.nc.conn.RemoteAddr().String(),
		"proto":  nc.proto,
		"params": truncateJson(req.Params),
		"type":   "request",
	})

	// Fine tuning of logged fields
	if opts.IsProduction {
		switch req.Method {

		// Hide sensible parameters
		case "sys.login", "user.setPass":
			e = e.WithField("params", make(map[string]interface{}))

		// Do not log verbose actions
		case "pipe.read", "pipe.write", "sys.ping":
			if !LogLevelIs(DebugLevel) {
				return
			}
		}
	}

	e.Infof(">> %s", req.Method)
}

func (nc *NexusConn) pushRes(res *JsonRpcRes) (err error) {
	select {
	case nc.chRes <- res:
		nc.logRes(res)

	case <-nc.context.Done():
		err = errors.New("Context cancelled")
	}
	return
}

func (nc *NexusConn) pullRes() (res *JsonRpcRes, err error) {
	select {
	case res = <-nc.chRes:
	case <-nc.context.Done():
		err = errors.New("Context cancelled")
	}
	return
}

func (nc *NexusConn) pushReq(req *JsonRpcReq) (err error) {
	select {
	case nc.chReq <- req:
		nc.logReq(req)

	case <-nc.context.Done():
		err = errors.New("Context cancelled")
	}
	return
}

func (nc *NexusConn) pullReq() (req *JsonRpcReq, err error) {
	select {
	case req = <-nc.chReq:
	case <-nc.context.Done():
		err = errors.New("Context cancelled")
	}
	return
}

func getTags(ud *UserData, prefix string) (tags map[string]interface{}) {
	tags = map[string]interface{}{}
	if ud == nil || ud.Tags == nil {
		return
	}
	pfs := prefixes(prefix)
	for _, pf := range pfs {
		if tm, ok := ud.Tags[pf]; ok {
			for k, v := range tm {
				tags[k] = v
			}
		}
	}
	return
}

func (nc *NexusConn) getTags(prefix string) (tags map[string]interface{}) {
	return getTags(nc.user, prefix)
}

func (nc *NexusConn) handleReq(req *JsonRpcReq) {
	if req.Id == nil {
		return
	}
	switch {
	case strings.HasPrefix(req.Method, "sys."):
		switch {
		case strings.HasPrefix(req.Method, "sys.node."):
			nc.handleNodesReq(req)
		case strings.HasPrefix(req.Method, "sys.session."):
			nc.handleSessionReq(req)
		default:
			nc.handleSysReq(req)
		}
	case strings.HasPrefix(req.Method, "task."):
		nc.handleTaskReq(req)
	case strings.HasPrefix(req.Method, "pipe."):
		nc.handlePipeReq(req)
	case strings.HasPrefix(req.Method, "topic."):
		nc.handleTopicReq(req)
	case strings.HasPrefix(req.Method, "user."):
		nc.handleUserReq(req)
	case strings.HasPrefix(req.Method, "sync."):
		nc.handleSyncReq(req)

	default:
		req.Error(ErrMethodNotFound, "", nil)
	}
}

func (nc *NexusConn) respWorker() {
	defer nc.close()
	trackCh, err := sesNotify.Register(nc.connId, make(chan interface{}, 1024))
	if err != nil { // Duplicated session ???
		nc.log.WithFields(logrus.Fields{
			"connid": nc.connId,
			"error":  err,
		}).Warnf("Error on respWorker")
		return
	}
	defer sesNotify.Unregister(nc.connId)
	for {
		select {
		case d := <-trackCh:

			switch res := d.(type) {

			case *Task:

				if !strings.HasPrefix(res.Path, "@pull.") {

					i := nc.log.WithFields(logrus.Fields{
						"type":          "metric",
						"kind":          "taskCompleted",
						"connid":        nc.connId,
						"id":            res.LocalId,
						"taskid":        res.Id,
						"path":          res.Path,
						"method":        res.Method,
						"ttl":           res.Ttl,
						"targetSession": res.Tses,
					})

					if cT, ok := res.CreationTime.(time.Time); ok {
						if wT, ok := res.WorkingTime.(time.Time); ok {
							i = i.WithFields(logrus.Fields{
								"waitingTime": round(wT.Sub(cT).Seconds(), 8),
								"workingTime": round(time.Since(wT).Seconds(), 8),
							})
						} else {
							i = i.WithFields(logrus.Fields{
								"waitingTime": round(time.Now().Sub(cT).Seconds(), 8),
							})
						}
					}

					i.Info("Task completed")
				}

				if res.ErrCode != nil {
					nc.pushRes(
						&JsonRpcRes{
							Id:    res.LocalId,
							Error: &JsonRpcErr{Code: *res.ErrCode, Message: res.ErrStr, Data: res.ErrObj},
						},
					)
				} else {
					nc.pushRes(
						&JsonRpcRes{
							Id:     res.LocalId,
							Result: res.Result,
						},
					)
				}

			case *Session:
				if res.Reload {
					nc.reload(false)
				}
				if res.Kick {
					nc.log.WithFields(logrus.Fields{
						"connid": nc.connId,
					}).Printf("Connection kicked!", nc.connId)
					nc.close()
				}
			}

		case <-nc.context.Done():
			return
		}
	}
}

func (nc *NexusConn) sendWorker() {
	defer nc.close()
	var null *int
	for {
		res, err := nc.pullRes()
		if err != nil {
			nc.log.WithFields(logrus.Fields{
				"connid": nc.connId,
				"error":  err,
			}).Debugf("Error on sendWorker")
			break
		}
		if res.Id == nil {
			if res.Error == nil {
				continue //Skip notification responses
			}
			if res.Error.Code == ErrInvalidRequest || res.Error.Code == ErrParse {
				res.Id = null
			} else {
				continue
			}
		}
		res.Jsonrpc = "2.0"
		if res.Result == nil && res.Error == nil {
			res.Result = null
		}
		buf, err := json.Marshal(res)
		if err != nil {
			nc.log.WithFields(logrus.Fields{
				"connid": nc.connId,
				"error":  err,
			}).Debugf("Connection marshal error")
			break
		}
		buf = append(buf, byte('\r'), byte('\n'))
		n, err := nc.connTx.Write(buf)
		if err != nil || n != len(buf) {
			nc.log.WithFields(logrus.Fields{
				"connid": nc.connId,
				"error":  err,
			}).Debugf("Connection write error")
			break
		}
	}
	nc.log.WithFields(logrus.Fields{
		"connid": nc.connId,
	}).Debugf("Exit from sendWorker")
}

func (nc *NexusConn) recvWorker() {
	defer nc.close()
	dec := json.NewDecoder(nc.connRx)
	for dec.More() {
		req := &JsonRpcReq{nc: nc}
		nc.connRx.SetLimit(int64(opts.MaxMessageSize))
		err := dec.Decode(req)
		if err != nil {
			if _, ok := err.(*json.SyntaxError); ok {
				req.Error(ErrParse, "", nil)
				dec = json.NewDecoder(nc.connRx) // Refresh decoder
				continue
			}
			if _, ok := err.(*json.UnmarshalTypeError); ok {
				req.Error(ErrInvalidRequest, "", nil)
				continue
			}
			break
		}
		err = nc.pushReq(req)
		if err != nil {
			nc.log.WithFields(logrus.Fields{
				"connid": nc.connId,
				"error":  err,
			}).Debugf("Error on recvWorker")
			break
		}
	}
	nc.log.WithFields(logrus.Fields{
		"connid": nc.connId,
	}).Debugf("Exit from recvWorker")

}

func (nc *NexusConn) watchdog() {
	defer nc.close()
	tick := time.NewTicker(time.Second * 10)
	exit := false
	for !exit {
		select {
		case <-tick.C:
			now := time.Now().Unix()
			max := atomic.LoadInt64(&nc.wdog)
			if (now-nc.connRx.GetLast() > max) &&
				(now-nc.connTx.GetLast() > max) {
				exit = true
				nc.log.WithFields(logrus.Fields{
					"connid": nc.connId,
				}).Warnln("Connection watchdog expired!")
			}

			nc.updateSession()

		case <-nc.context.Done():
			exit = true
		}
	}
	tick.Stop()
}

func (nc *NexusConn) close() {
	if atomic.CompareAndSwapInt32(&nc.closed, 0, 1) {
		nc.cancelFun()
		nc.conn.Close()
		if mainContext.Err() == nil {
			if nc.proto != "internal" || LogLevelIs(DebugLevel) {
				nc.log.WithFields(logrus.Fields{
					"connid": nc.connId,
				}).Printf("Closing session")
			}
			dbClean(nc.connId)
		}
	}
}

func (nc *NexusConn) reload(fromSameSession bool) (bool, int) {
	if nc.user == nil || nc.user == Nobody {
		return false, ErrInvalidRequest
	}
	ud, err := loadUserData(nc.user.User)
	if err != ErrNoError {
		return false, ErrInternal
	}

	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&nc.user)), unsafe.Pointer(&UserData{
		User:        ud.User,
		Mask:        nc.user.Mask,
		Tags:        maskTags(ud.Tags, nc.user.Mask),
		MaxSessions: ud.MaxSessions,
		Whitelist:   ud.Whitelist,
		Blacklist:   ud.Blacklist,
	}))

	if !fromSameSession {
		wres, err := r.Table("sessions").
			Between(nc.connId, nc.connId+"\uffff").
			Update(ei.M{"reload": false}).
			RunWrite(db)
		if err != nil || wres.Replaced == 0 {
			return false, ErrInternal
		}
		nc.log.WithFields(logrus.Fields{
			"connid": nc.connId,
			"error":  err,
		}).Printf("Connection reloaded by other session")
	} else {
		nc.log.WithFields(logrus.Fields{
			"connid": nc.connId,
		}).Printf("Connection reloaded by itself")
	}
	return true, 0
}

func (nc *NexusConn) updateSession() {
	_, err := r.Table("sessions").
		Get(nc.connId).
		Replace(ei.M{
			"id":            nc.connId,
			"nodeId":        nodeId,
			"creationTime":  r.Row.Field("creationTime").Default(r.Now()),
			"lastSeen":      r.Now(),
			"remoteAddress": nc.conn.RemoteAddr().String(),
			"protocol":      nc.proto,
			"user":          nc.user.User,
		}).
		RunWrite(db)

	if err != nil {
		nc.log.WithFields(logrus.Fields{
			"connid": nc.connId,
			"error":  err,
		}).Errorln("Error updating session")
		nc.close()
	}
}

var numconn int64

type JsonRpcReqLog struct {
	ConnID string      `json:"connid"`
	Method string      `json:"method"`
	Params interface{} `json:"params"`
	Remote string      `json:"remoteAddr"`
	ID     interface{} `json:"id"`
}

func (nc *NexusConn) handle() {

	if nc.proto != "internal" {
		atomic.AddInt64(&numconn, 1)
		defer func() { atomic.AddInt64(&numconn, -1) }()
	}

	defer nc.close()
	go nc.respWorker()
	go nc.sendWorker()
	go nc.recvWorker()
	go nc.watchdog()

	nc.updateSession()

	for {
		req, err := nc.pullReq()
		if err != nil {
			nc.log.WithFields(logrus.Fields{
				"connid": nc.connId,
				"error":  err,
			}).Debug("Error on connection handler")
			break
		}

		if (req.Jsonrpc != "2.0" && req.Jsonrpc != "") || req.Method == "" { //"jsonrpc":"2.0" is optional
			req.Error(ErrInvalidRequest, "", nil)
			continue
		}

		go nc.handleReq(req)
	}
}
