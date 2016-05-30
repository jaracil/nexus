package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"

	"golang.org/x/net/websocket"
)

func wsListener(u *url.URL) {
	defer exit("ws listener goroutine error")

	wsf := func(ws *websocket.Conn) {
		log.Print("WebSocket connection from: ", ws.RemoteAddr())
		NewNexusConn(ws).handle()
	}

	if u.Path == "" {
		u.Path = "/"
	}

	wsrv := &websocket.Server{}
	wsrv.Handler = wsf
	if wsrv.Header == nil {
		wsrv.Header = make(map[string][]string)
	}
	wsrv.Header["Access-Control-Allow-Origin"] = []string{"*"}
	http.Handle(u.Path, wsrv)

	log.Println("Listening WS  at:", fmt.Sprintf("%s%s", u.Host, u.Path))
	err := http.ListenAndServe(u.Host, nil)
	if err != nil {
		log.Println("Websocket listener error: " + err.Error())
		mainCancel()
		return
	}
}

func wssListener(u *url.URL) {
	defer exit("wsListener goroutine error")

	wsf := func(ws *websocket.Conn) {
		log.Print("Secure WebSocket connection from: ", ws.RemoteAddr())
		NewNexusConn(ws).handle()
	}

	wsrv := &websocket.Server{}
	wsrv.Handler = wsf

	log.Println("Listening WSS at:", fmt.Sprintf("%s%s", u.Host, u.Path))
	err := http.ListenAndServeTLS(u.Host, opts.SSL.Cert, opts.SSL.Key, wsrv)
	if err != nil {
		log.Println("Secure Websocket listener error: " + err.Error())
		mainCancel()
		return
	}
}
