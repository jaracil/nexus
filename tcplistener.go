package main

import (
	"crypto/tls"
	"net"
	"net/url"

	"github.com/armon/go-proxyproto"
	. "github.com/jaracil/nexus/log"
	"golang.org/x/net/context"
)

func tcpListener(u *url.URL, ctx context.Context, proxyed bool) {
	defer Log.Println("Listener", u, "finished")

	addr, err := net.ResolveTCPAddr("tcp", u.Host)
	if err != nil {
		Log.Errorln("Cannot resolve the tcp address: ", err)
		exit("tcpListener goroutine error")
		return
	}

	var listen net.Listener

	listen, err = net.ListenTCP("tcp", addr)
	if err != nil {
		Log.Println("Cannot open tcpListener:", err)
		exit("tcpListener goroutine error")
		return
	}

	if proxyed {
		listen = &proxyproto.Listener{Listener: listen}
	}

	Log.Println("Listening on", u)

	go func() {
		select {
		case <-ctx.Done():
			listen.Close()
		}
	}()

	for {
		conn, err := listen.Accept()

		if ctx.Err() == nil {
			if err != nil {
				Log.Errorln("Error accepting tcp socket:", err)
				exit("tcpListener goroutine error")
				return
			} else {
				Log.Warnf("Unencrypted connection from %s!", conn.RemoteAddr())
				Log.Printf("TCP connection from: %s", conn.RemoteAddr())
				nc := NewNexusConn(conn)
				nc.proto = "tcp"
				go nc.handle()
			}
		} else {
			return
		}
	}
}

func sslListener(u *url.URL, ctx context.Context, proxyed bool) {
	defer Log.Println("Listener", u, "finished")

	Log.Debugln("Loading SSL cert/key")
	cert, err := tls.LoadX509KeyPair(opts.SSL.Cert, opts.SSL.Key)
	if err != nil {
		Log.Errorln("Cannot load SSL cert/key:", err)
		exit("cannot load ssl cert/key")
		return
	}

	tlsConfig := &tls.Config{}
	tlsConfig.Certificates = []tls.Certificate{cert}

	var listen net.Listener

	if proxyed {
		addr, err := net.ResolveTCPAddr("tcp", u.Host)
		if err != nil {
			Log.Errorln("Cannot resolve the address: ", err)
			exit("ssl+proxy Listener goroutine error")
			return
		}

		l, err := net.ListenTCP("tcp", addr)
		if err != nil {
			Log.Println("Cannot open ssl+proxy Listener:", err)
			exit("ssl+proxy Listener goroutine error")
			return
		}

		proxyListen := &proxyproto.Listener{Listener: l}
		listen = tls.NewListener(proxyListen, tlsConfig)
	} else {
		listen, err = tls.Listen("tcp", u.Host, tlsConfig)
		if err != nil && ctx.Err() == nil {
			Log.Errorln("Cannot open sslListener:", err)
			exit("sslListener goroutine error")
			return
		}
	}

	Log.Println("Listening on", u)

	go func() {
		select {
		case <-ctx.Done():
			listen.Close()
		}
	}()

	for {

		conn, err := listen.Accept()

		if ctx.Err() == nil {
			if err != nil {
				Log.Errorln("Error accepting ssl socket:", err)
				exit("sslListener goroutine error")
				return
			} else {
				Log.Printf("SSL connection from: %s", conn.RemoteAddr())
				nc := NewNexusConn(conn)
				nc.proto = "ssl"
				go nc.handle()
			}
		} else {
			return
		}
	}
}
