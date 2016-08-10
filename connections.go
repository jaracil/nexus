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

	r "github.com/dancannon/gorethink"
	"github.com/jaracil/ei"
	. "github.com/jaracil/nexus/log"
	"github.com/jaracil/nxcli/nxcore"
	"github.com/jaracil/smartio"
	"golang.org/x/net/context"
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
	nc      *NexusConn
}

type JsonRpcRes struct {
	Jsonrpc string      `json:"jsonrpc"`
	Id      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *JsonRpcErr `json:"error,omitempty"`
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
	}
	nc.context, nc.cancelFun = context.WithCancel(mainContext)
	return nc
}

func NewInternalClient() *nxcore.NexusConn {
	client, server := net.Pipe()

	nc := NewNexusConn(server)
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
		},
	)
}

func (req *JsonRpcReq) Result(result interface{}) {
	req.nc.pushRes(
		&JsonRpcRes{
			Id:     req.Id,
			Result: result,
		},
	)
}

func (nc *NexusConn) pushRes(res *JsonRpcRes) (err error) {
	select {
	case nc.chRes <- res:
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
		Log.Warnf("Error on [%s] respWorker: %s", nc.connId, err)
		return
	}
	defer sesNotify.Unregister(nc.connId)
	for {
		select {
		case d := <-trackCh:

			switch res := d.(type) {

			case *Task:
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
					Log.Printf("Connection [%s] has been kicked!", nc.connId)
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
			Log.Debugf("Error on [%s] sendWorker: %s", nc.connId, err)
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
			Log.Debugf("[%s] connection marshal error: %s", nc.connId, err)
			break
		}
		buf = append(buf, byte('\r'), byte('\n'))
		n, err := nc.connTx.Write(buf)
		if err != nil || n != len(buf) {
			Log.Debugf("[%s] connection write error: %s", nc.connId, err)
			break
		}
	}
	Log.Debugf("Exit from [%s] sendWorker", nc.connId)
}

func (nc *NexusConn) recvWorker() {
	defer nc.close()
	dec := json.NewDecoder(nc.connRx)
	for dec.More() {
		req := &JsonRpcReq{nc: nc}
		nc.connRx.SetLimit(1024 * 1024 * 32)
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
			Log.Debugf("Error on [%s] recvWorker: %s", nc.connId, err)
			break
		}
	}
	Log.Debugf("Exit from [%s] recvWorker", nc.connId)
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
				Log.Warnf("Connection [%s] watch dog expired!", nc.connId)
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
				Log.Printf("Closing [%s] session", nc.connId)
			}
			dbClean(nc.connId)
		}
	}
}

func (nc *NexusConn) reload(fromSameSession bool) (bool, int) {
	if nc.user == nil {
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
		Log.Printf("Connection [%s] reloaded by other session", nc.connId)
	} else {
		Log.Printf("Connection [%s] reloaded by itself", nc.connId)
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
		Log.Errorf("Error updating session [%s]: %s", nc.connId, err)
		nc.close()
	}
}

var numconn int64

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
			Log.Debugf("Error on [%s] connection handler: %s", nc.connId, err)
			break
		}

		if (req.Jsonrpc != "2.0" && req.Jsonrpc != "") || req.Method == "" { //"jsonrpc":"2.0" is optional
			req.Error(ErrInvalidRequest, "", nil)
			continue
		}

		if (req.Method != "sys.ping" && nc.proto != "internal") || LogLevelIs(DebugLevel) {
			params, err := json.Marshal(req.Params)
			if err != nil {
				Log.Printf("[%s@%s] %s: %#v - id: %.0f", req.nc.connId, req.nc.conn.RemoteAddr(), req.Method, req.Params, req.Id)
			} else {
				Log.Printf("[%s@%s] %s: %s - id: %.0f", req.nc.connId, req.nc.conn.RemoteAddr(), req.Method, params, req.Id)
			}
		}

		go nc.handleReq(req)
	}
}
