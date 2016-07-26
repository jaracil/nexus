package main

import (
	"crypto/tls"
	"log"
	"net"
	"net/url"

	"golang.org/x/net/context"
)

func tcpListener(u *url.URL, ctx context.Context) {
	defer log.Println("Listener", u, "finished")

	addr, err := net.ResolveTCPAddr("tcp", u.Host)
	if err != nil {
		log.Println("Cannot resolve the tcp address: ", err)
		exit("tcpListener goroutine error")
		return
	}

	listen, err := net.ListenTCP("tcp", addr)
	if err != nil {
		log.Println("Can not open tcpListener:", err)
		exit("tcpListener goroutine error")
		return
	}

	log.Println("Listening on", u)

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
				log.Print("Error acceting tcp socket:", err)
				exit("tcpListener goroutine error")
				return
			} else {
				log.Print("TCP connection from:", conn.RemoteAddr())
				nc := NewNexusConn(conn)
				nc.proto = "tcp"
				go nc.handle()
			}
		} else {
			return
		}

	}
}

func loadCerts(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(opts.SSL.Cert, opts.SSL.Key)
	if err != nil {
		log.Println("Cannot load SSL cert/key:", err)
		exit("cannot load ssl cert/key")
		return nil, err
	}
	return &cert, nil
}

func sslListener(u *url.URL, ctx context.Context) {
	defer log.Println("Listener", u, "finished")

	tlsConfig := &tls.Config{}
	tlsConfig.GetCertificate = loadCerts

	listen, err := tls.Listen("tcp", u.Host, tlsConfig)
	if err != nil && ctx.Err() == nil {
		log.Print("Can not open sslListener:", err)
		exit("sslListener goroutine error")
		return
	}

	log.Println("Listening on", u)

	// Server certs get loaded on first request, so we force one here to crash if certs are missing
	loadCerts(nil)

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
				log.Print("Error acceting ssl socket:", err)
				exit("sslListener goroutine error")
				return
			} else {
				log.Println("SSL connection from:", conn.RemoteAddr())
				nc := NewNexusConn(conn)
				nc.proto = "ssl"
				go nc.handle()
			}
		} else {
			return
		}
	}
}
