package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"

	"github.com/jaracil/nxcli/nxcore"

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

		user, pass, _ := req.BasicAuth()

		body := make([]byte, 4096)
		n, _ := req.Body.Read(body)
		body = body[:n]
		fmt.Printf("%s\n", body)
		var jsonreq nxcore.JsonRpcReq
		if err := json.Unmarshal(body[:n], &jsonreq); err != nil {
			log.Println("Malformed JSON on HTTP bridge:", err)
			return
		}

		A, B := net.Pipe()
		ns := NewNexusConn(B)
		go ns.handle()

		nc := nxcore.NewNexusConn(A)
		if _, err := nc.Login(user, pass); err == nil {

			jsonres := &nxcore.JsonRpcRes{Jsonrpc: "2.0", Id: jsonreq.Id}
			if r, e := nc.Exec(jsonreq.Method, jsonreq.Params); e == nil {
				jsonres.Result = r
			} else {
				jsonres.Error = e.(*nxcore.JsonRpcErr)
			}

			ret, _ := json.Marshal(jsonres)
			res.WriteHeader(200)
			res.Write(ret)

		} else {
			res.WriteHeader(401)
		}
		nc.Cancel()
		ns.clean()
	}
}

func httpListener(u *url.URL) {
	defer exit("http listener goroutine error")

	handler := http.Handler(&httpwsHandler{})

	log.Println("Listening HTTP  at:", fmt.Sprintf("%s%s", u.Host, u.Path))
	err := http.ListenAndServe(u.Host, handler)
	if err != nil {
		log.Println("HTTP listener error: " + err.Error())
		mainCancel()
		return
	}
}

func httpsListener(u *url.URL) {
	defer exit("https listener goroutine error")

	if u.Path == "" {
		u.Path = "/"
	}

	handler := http.Handler(&httpwsHandler{})

	log.Println("Listening HTTPS at:", fmt.Sprintf("%s%s", u.Host, u.Path))
	err := http.ListenAndServeTLS(u.Host, opts.SSL.Cert, opts.SSL.Key, handler)
	if err != nil {
		log.Println("HTTPS listener error: " + err.Error())
		mainCancel()
		return
	}
}
