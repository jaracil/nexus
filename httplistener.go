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
	"golang.org/x/net/websocket"
)

type httpwsHandler struct{}

func (*httpwsHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {

	if req.TLS == nil {
		warnln("[WARN] Unencrypted connection from %s!!\n", req.RemoteAddr)
	}

	if headerContains(req.Header["Connection"], "Upgrade") {
		if headerContains(req.Header["Upgrade"], "websocket") {

			// WebSocket

			wsrv := &websocket.Server{}
			wsrv.Handler = func(ws *websocket.Conn) {
				sysln("WebSocket connection from: ", req.RemoteAddr)
				NewNexusConn(ws).handle()
			}
			if wsrv.Header == nil {
				wsrv.Header = make(map[string][]string)
			}
			wsrv.Header["Access-Control-Allow-Origin"] = []string{"*"}

			wsrv.ServeHTTP(res, req)

		} else {
			warnf("Connection dropped for requesting an upgrade to an unsupported protocol: %v\n", req.Header["Upgrade"])
			res.WriteHeader(http.StatusBadRequest)
		}

	} else {

		// HTTP Bridge
		netCli, netSrv := net.Pipe()
		netCliBuf := bufio.NewReader(netCli)
		ns := NewNexusConn(netSrv)
		defer ns.close()
		defer netCli.Close()
		go ns.handle()
		if user, pass, loginData := req.BasicAuth(); loginData {
			fmt.Fprintf(netCli, `{"jsonrpc":"2.0", "id":1, "method":"sys.login", "params":{"user":"%s", "pass":"%s"}}`, user, pass)
			resSlice, _, err := netCliBuf.ReadLine()
			if err != nil {
				res.WriteHeader(http.StatusInternalServerError)
				return
			}
			loginRes := ei.M{}
			if err := json.Unmarshal(resSlice, &loginRes); err != nil {
				res.WriteHeader(http.StatusInternalServerError)
				return
			}
			if ei.N(loginRes).M("id").IntZ() != 1 {
				res.WriteHeader(http.StatusInternalServerError)
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
			return
		}
		if resSlice, _, err := netCliBuf.ReadLine(); err == nil {
			res.Header().Set("Content-Type", "application/json")
			res.WriteHeader(http.StatusOK)
			res.Write([]byte(resSlice))
		} else {
			res.WriteHeader(http.StatusInternalServerError)
		}
	}
}

func httpListener(u *url.URL) {
	defer exit("http listener goroutine error")

	handler := http.Handler(&httpwsHandler{})

	sysln("Listening HTTP:\t", u.Host)
	err := http.ListenAndServe(u.Host, handler)
	if err != nil {
		errf("HTTP listener error: %s", err.Error())
		mainCancel()
		return
	}
}

func httpsListener(u *url.URL) {
	defer exit("https listener goroutine error")

	handler := http.Handler(&httpwsHandler{})

	sysln("Listening HTTPS:\t", u.Host)
	err := http.ListenAndServeTLS(u.Host, opts.SSL.Cert, opts.SSL.Key, handler)
	if err != nil {
		errf("HTTPS listener error: %s", err.Error())
		mainCancel()
		return
	}
}
