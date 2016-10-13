package main

import (
	"net/url"

	"github.com/Sirupsen/logrus"
	. "github.com/jaracil/nexus/log"
	"golang.org/x/net/context"
)

func listen() {
	if listenContext != nil {
		listenCancel()
	}
	listenContext, listenCancel = context.WithCancel(mainContext)
	listeners(listenContext)
}

func listeners(ctx context.Context) {
	for _, v := range opts.Listeners {
		if u, err := url.Parse(v); err == nil {

			switch u.Scheme {
			case "tcp":
				go tcpListener(u, ctx, false)
			case "tcp+proxy":
				go tcpListener(u, ctx, true)
			case "ssl":
				go sslListener(u, ctx, false)
			case "ssl+proxy":
				go sslListener(u, ctx, true)
			case "http":
				go httpListener(u, ctx)
			case "https":
				go httpsListener(u, ctx)
			case "health":
				go healthCheckListener(u, ctx)

			default:
				Log.WithFields(logrus.Fields{
					"listener": u,
				}).Print("Unknown listener")
				mainCancel()
				return
			}

		} else {
			Log.WithFields(logrus.Fields{
				"listener": v,
			}).Print("Couldn't parse listener")
			mainCancel()
			return
		}
	}
}
