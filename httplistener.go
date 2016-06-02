package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"

	"github.com/jaracil/ei"
	"golang.org/x/net/websocket"
)

type httpwsHandler struct{}

func (*httpwsHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {

	if req.TLS == nil {
		log.Printf("[WARN] Unencrypted connection from %s!!\n", req.RemoteAddr)
	}

	hcon := req.Header.Get("Connection")
	upgr := req.Header.Get("Upgrade")

	if hcon == "Upgrade" {
		if upgr == "websocket" {

			// WebSocket

			wsrv := &websocket.Server{}
			wsrv.Handler = func(ws *websocket.Conn) {
				log.Print("WebSocket connection from: ", req.RemoteAddr)
				NewNexusConn(ws).handle()
			}
			if wsrv.Header == nil {
				wsrv.Header = make(map[string][]string)
			}
			wsrv.Header["Access-Control-Allow-Origin"] = []string{"*"}

			wsrv.ServeHTTP(res, req)

		} else {
			log.Println("Connection dropped for requesting an upgrade to an unsupported protocol:", upgr)
			res.WriteHeader(400)
		}

	} else {

		// HTTP Bridge
		netCli, netSrv := net.Pipe()
		netCliBuf := bufio.NewReader(netCli)
		ns := NewNexusConn(netSrv)
		defer ns.clean()
		defer netCli.Close()
		go ns.handle()
		if user, pass, loginData := req.BasicAuth(); loginData {
			fmt.Fprintf(netCli, `{"jsonrpc":"2.0", "id":1, "method":"sys.login", "params":{"user":"%s", "pass":"%s"}}`, user, pass)
			resSlice, _, err := netCliBuf.ReadLine()
			if err != nil {
				res.WriteHeader(500)
				return
			}
			loginRes := ei.M{}
			if err := json.Unmarshal(resSlice, &loginRes); err != nil {
				res.WriteHeader(500)
				return
			}
			if ei.N(loginRes).M("id").IntZ() != 1 {
				res.WriteHeader(500)
				return
			}
			if !ei.N(loginRes).M("result").M("ok").BoolZ() {
				res.WriteHeader(200)
				res.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32010,"message":"Permission denied [login fail]"}}`))
				return
			}
		}
		if _, err := io.Copy(netCli, req.Body); err != nil {
			res.WriteHeader(500)
			return
		}
		if resSlice, _, err := netCliBuf.ReadLine(); err == nil {
			res.WriteHeader(200)
			res.Write([]byte(resSlice))
		} else {
			res.WriteHeader(500)
		}
	}
}

func httpListener(u *url.URL) {
	defer exit("http listener goroutine error")

	handler := http.Handler(&httpwsHandler{})

	log.Println("Listening HTTP  at:", u.Host)
	err := http.ListenAndServe(u.Host, handler)
	if err != nil {
		log.Println("HTTP listener error: " + err.Error())
		mainCancel()
		return
	}
}

func httpsListener(u *url.URL) {
	defer exit("https listener goroutine error")

	handler := http.Handler(&httpwsHandler{})

	log.Println("Listening HTTPS at:", u.Host)
	err := http.ListenAndServeTLS(u.Host, opts.SSL.Cert, opts.SSL.Key, handler)
	if err != nil {
		log.Println("HTTPS listener error: " + err.Error())
		mainCancel()
		return
	}
}
