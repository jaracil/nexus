package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"

	"github.com/jaracil/ei"
	. "github.com/jaracil/nexus/log"
	"github.com/sirupsen/logrus"
	"github.com/tylerb/graceful"
	"golang.org/x/net/context"
	"golang.org/x/net/websocket"
)

type httpwsHandler struct{}

func (*httpwsHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {

	if req.TLS == nil {
		Log.WithFields(logrus.Fields{
			"remote": req.RemoteAddr,
		}).Warn("Unencrypted connection!!")
	}

	if headerContains(req.Header["Connection"], "upgrade") {
		if headerContains(req.Header["Upgrade"], "websocket") {
			// WebSocket

			wsrv := &websocket.Server{}
			wsrv.Handler = func(ws *websocket.Conn) {
				if u, err := url.Parse(req.RemoteAddr); err != nil {
					ws.Config().Origin = &url.URL{Scheme: "http", Host: "0.0.0.0"}
				} else {
					ws.Config().Origin = u
				}
				Log.WithFields(logrus.Fields{
					"remote": ws.RemoteAddr().String(),
				}).Println("New WebSocket connection")

				nc := NewNexusConn(ws)
				if req.TLS != nil {
					nc.proto = "wss"
				} else {
					nc.proto = "ws"
				}

				nc.handle()
			}
			if wsrv.Header == nil {
				wsrv.Header = make(map[string][]string)
			}

			wsrv.Header["Access-Control-Allow-Origin"] = []string{"*"}

			wsrv.ServeHTTP(res, req)

		} else {
			Log.WithFields(logrus.Fields{
				"unsupported": req.Header["Upgrade"],
			}).Warn("Connection dropped for requesting an upgrade to an unsupported protocol")
			res.WriteHeader(http.StatusBadRequest)
		}

	} else {

		// HTTP Bridge
		netCli, netSrv := net.Pipe()
		netCliBuf := bufio.NewReaderSize(netCli, opts.MaxMessageSize)
		ns := NewNexusConn(netSrv)
		if req.TLS != nil {
			ns.proto = "https"
		} else {
			ns.proto = "http"
		}
		defer ns.close()
		defer netCli.Close()
		go ns.handle()
		if user, pass, loginData := req.BasicAuth(); loginData {
			fmt.Fprintf(netCli, `{"jsonrpc":"2.0", "id":1, "method":"sys.login", "params":{"user":"%s", "pass":"%s"}}`, user, pass)
			resSlice, _, err := netCliBuf.ReadLine()
			if err != nil {
				res.WriteHeader(http.StatusInternalServerError)
				res.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"Error reading login response [%s]"}}`, err.Error())))
				return
			}
			loginRes := ei.M{}
			if err := json.Unmarshal(resSlice, &loginRes); err != nil {
				res.WriteHeader(http.StatusInternalServerError)
				res.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"Error unmarshaling login response [%s]"}}`, err.Error())))
				return
			}
			if ei.N(loginRes).M("id").IntZ() != 1 {
				res.WriteHeader(http.StatusInternalServerError)
				res.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"Error on login response id"}}`)))
				return
			}
			if !ei.N(loginRes).M("result").M("ok").BoolZ() {
				res.Header().Set("Content-Type", "application/json")
				res.WriteHeader(http.StatusOK)
				res.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32010,"message":"Permission denied [login fail]"}}`))
				return
			}
		}
		if _, err := io.Copy(netCli, req.Body); err != nil {
			res.WriteHeader(http.StatusInternalServerError)
			res.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"Error sending body request [%s]"}}`, err.Error())))
			return
		}
		if resSlice, err := netCliBuf.ReadBytes('\n'); err == nil {
			res.Header().Set("Content-Type", "application/json")
			res.WriteHeader(http.StatusOK)
			res.Write(resSlice)
		} else {
			res.WriteHeader(http.StatusInternalServerError)
			res.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"Error reading request response [%s]"}}`, err.Error())))
		}
	}
}

func httpListener(u *url.URL, ctx context.Context) {
	defer Log.WithFields(logrus.Fields{
		"listener": u,
	}).Println("HTTP listener finished")
	server := graceful.Server{
		Server:  &http.Server{Addr: u.Host, Handler: http.Handler(&httpwsHandler{})},
		Timeout: 0,
	}

	go func() {
		select {
		case <-ctx.Done():
			server.Stop(0)
		}
	}()

	Log.WithFields(logrus.Fields{
		"address": u.String(),
	}).Println("New HTTP listener started")

	err := server.ListenAndServe()
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		Log.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Errorln("HTTP listener error")
		exit("http listener goroutine error")
		return
	}
}

func httpsListener(u *url.URL, ctx context.Context) {
	defer Log.WithFields(logrus.Fields{
		"listener": u,
	}).Println("HTTPS listener finished")

	server := graceful.Server{
		Server:  &http.Server{Addr: u.Host, Handler: http.Handler(&httpwsHandler{})},
		Timeout: 0,
	}

	go func() {
		select {
		case <-ctx.Done():
			server.Stop(0)
		}
	}()

	Log.WithFields(logrus.Fields{
		"address": u.String(),
	}).Println("New HTTPS listener started")

	err := server.ListenAndServeTLS(opts.SSL.Cert, opts.SSL.Key)
	if ctx.Err() != nil {
		return
	}

	if err != nil {
		Log.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Error("HTTPS listener error")
		exit("https listener goroutine error")
		return
	}
}

func healthCheckListener(u *url.URL, ctx context.Context) {
	defer Log.WithFields(logrus.Fields{
		"listener": u,
	}).Println("Health listener finished")

	server := graceful.Server{
		Server: &http.Server{Addr: u.Host, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
		})},
		Timeout: 0,
	}

	go func() {
		select {
		case <-ctx.Done():
			server.Stop(0)
		}
	}()

	Log.WithFields(logrus.Fields{
		"address": u.String(),
	}).Println("New health listener started")

	err := server.ListenAndServe()
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		Log.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Errorln("HealthCheck listener error")
		exit("healthCheck listener goroutine error")
		return
	}
}
