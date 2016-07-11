package main

import (
	"crypto/tls"
	"log"
	"net"
	"net/url"
)

func tcpListener(u *url.URL) {
	defer exit("tcpListener goroutine error")

	addr, err := net.ResolveTCPAddr("tcp", u.Host)
	if err != nil {
		log.Println("Cannot resolve the tcp address: ", err)
		mainCancel()
		return
	}

	listen, err := net.ListenTCP("tcp", addr)
	if err != nil {
		log.Println("Can not open tcpListener:", err)
		mainCancel()
		return
	}

	log.Println("Listening TCP   at:", u.Host)

	for {
		conn, err := listen.Accept()
		if err != nil {
			log.Print("Error acceting tcp socket:", err)
			mainCancel()
			return
		}
		log.Print("TCP connection from:", conn.RemoteAddr())
		go NewNexusConn(conn).handle()
	}
}

func loadCerts(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(opts.SSL.Cert, opts.SSL.Key)
	if err != nil {
		log.Println("Cannot load SSL cert/key:", err)
		mainCancel()
		return nil, err
	}
	return &cert, nil
}

func sslListener(u *url.URL) {
	defer exit("sslListener goroutine error")

	tlsConfig := &tls.Config{}
	tlsConfig.GetCertificate = loadCerts

	listen, err := tls.Listen("tcp", u.Host, tlsConfig)
	if err != nil {
		log.Print("Can not open sslListener:", err)
		mainCancel()
		return
	}

	log.Println("Listening SSL   at:", u.Host)

	// Server certs get loaded on first request, so we force one here to crash if certs are missing
	loadCerts(nil)

	for {
		conn, err := listen.Accept()
		if err != nil {
			log.Print("Error acceting ssl socket:", err)
			mainCancel()
			return
		}
		log.Println("SSL connection from:", conn.RemoteAddr())
		go NewNexusConn(conn).handle()
	}
}
