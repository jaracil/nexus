package main

import (
	"crypto/tls"
	"net"
	"net/url"
)

func tcpListener(u *url.URL) {
	defer exit("tcpListener goroutine error")

	addr, err := net.ResolveTCPAddr("tcp", u.Host)
	if err != nil {
		errln("Cannot resolve the tcp address: ", err)
		mainCancel()
		return
	}

	listen, err := net.ListenTCP("tcp", addr)
	if err != nil {
		errln("Cannot open tcpListener: ", err)
		mainCancel()
		return
	}

	sysln("Listening TCP:\t", u.Host)

	for {
		conn, err := listen.Accept()
		if err != nil {
			errln("Error acceting tcp socket:", err)
			mainCancel()
			return
		}
		sysln("TCP connection from: ", conn.RemoteAddr())
		go NewNexusConn(conn).handle()
	}
}

func loadCerts(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(opts.SSL.Cert, opts.SSL.Key)
	if err != nil {
		errln("Cannot load SSL cert/key:", err)
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
		errln("Cannot start sslListener: ", err)
		mainCancel()
		return
	}

	sysln("Listening SSL:\t", u.Host)

	for {
		conn, err := listen.Accept()
		if err != nil {
			errln("Error acceting ssl socket: ", err)
			mainCancel()
			return
		}
		sysln("SSL connection from:", conn.RemoteAddr())
		go NewNexusConn(conn).handle()
	}
}
