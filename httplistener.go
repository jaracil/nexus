package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"

	"golang.org/x/net/websocket"
)

func httpws(res http.ResponseWriter, req *http.Request) {

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

		log.Println("Bridge not implemented. Dropping connection")
		res.WriteHeader(500)
		res.Write([]byte("HTTP Bridge not implemented\n"))
	}
}

func httpListener(u *url.URL) {
	defer exit("http listener goroutine error")

	if u.Path == "" {
		u.Path = "/"
	}

	log.Println("Listening HTTP  at:", fmt.Sprintf("%s%s", u.Host, u.Path))
	err := http.ListenAndServe(u.Host, nil)
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

	http.HandleFunc(u.Path, httpws)

	log.Println("Listening HTTPS at:", fmt.Sprintf("%s%s", u.Host, u.Path))
	err := http.ListenAndServeTLS(u.Host, opts.SSL.Cert, opts.SSL.Key, nil)
	if err != nil {
		log.Println("HTTPS listener error: " + err.Error())
		mainCancel()
		return
	}
}
