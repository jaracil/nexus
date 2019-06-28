package main

import (
	"crypto/tls"
	"net"
	"net/url"

	"github.com/armon/go-proxyproto"
	. "github.com/jaracil/nexus/log"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
)

func tcpListener(u *url.URL, ctx context.Context, proxyed bool) {
	defer Log.WithFields(logrus.Fields{
		"listener": u,
	}).Warnln("TCP listener finished")

	addr, err := net.ResolveTCPAddr("tcp", u.Host)
	if err != nil {
		Log.WithFields(logrus.Fields{
			"error": err,
		}).Errorln("Cannot resolve the tcp address")
		exit("tcpListener goroutine error")
		return
	}

	var listen net.Listener

	listen, err = net.ListenTCP("tcp", addr)
	if err != nil {
		Log.WithFields(logrus.Fields{
			"error": err,
		}).Errorln("Cannot open tcpListener")
		exit("tcpListener goroutine error")
		return
	}

	if proxyed {
		listen = &proxyproto.Listener{Listener: listen}
	}

	Log.WithFields(logrus.Fields{
		"address": u.String(),
	}).Infoln("New TCP listener started")

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
				Log.WithFields(logrus.Fields{
					"error": err,
				}).Error("Error accepting tcp socket:", err)
				exit("tcpListener goroutine error")
				return
			} else {
				Log.WithFields(logrus.Fields{
					"address": conn.RemoteAddr().String(),
				}).Warn("Unencrypted connection!!")
				Log.WithFields(logrus.Fields{
					"address": conn.RemoteAddr().String(),
				}).Infoln("New TCP connection")

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
	defer Log.WithFields(logrus.Fields{
		"listener": u,
	}).Warnln("SSL listener finished")

	Log.Debugln("Loading SSL cert/key")
	cert, err := tls.LoadX509KeyPair(opts.SSL.Cert, opts.SSL.Key)
	if err != nil {
		Log.WithFields(logrus.Fields{
			"error": err,
		}).Errorln("Cannot load SSL cert/key")
		exit("cannot load ssl cert/key")
		return
	}

	tlsConfig := &tls.Config{}
	tlsConfig.Certificates = []tls.Certificate{cert}

	var listen net.Listener

	if proxyed {
		addr, err := net.ResolveTCPAddr("tcp", u.Host)
		if err != nil {
			Log.WithFields(logrus.Fields{
				"error": err,
			}).Errorln("Cannot resolve the address")
			exit("ssl+proxy Listener goroutine error")
			return
		}

		l, err := net.ListenTCP("tcp", addr)
		if err != nil {
			Log.WithFields(logrus.Fields{
				"error": err,
			}).Errorln("Cannot open ssl+proxy Listener")
			exit("ssl+proxy Listener goroutine error")
			return
		}

		proxyListen := &proxyproto.Listener{Listener: l}
		listen = tls.NewListener(proxyListen, tlsConfig)
	} else {
		listen, err = tls.Listen("tcp", u.Host, tlsConfig)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			Log.WithFields(logrus.Fields{
				"error": err,
			}).Errorln("Cannot open sslListener")
			exit("sslListener goroutine error")
			return
		}
	}

	Log.WithFields(logrus.Fields{
		"address": u.String(),
	}).Infoln("New SSL listener started")

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
				Log.WithFields(logrus.Fields{
					"error": err,
				}).Errorln("Error accepting ssl socket")
				exit("sslListener goroutine error")
				return
			} else {
				Log.WithFields(logrus.Fields{
					"remote": conn.RemoteAddr().String(),
				}).Infoln("New SSL connection")
				nc := NewNexusConn(conn)
				nc.proto = "ssl"
				go nc.handle()
			}
		} else {
			return
		}
	}
}
