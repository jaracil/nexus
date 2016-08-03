package main

import (
	"net/url"

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
				go tcpListener(u, ctx)
			case "ssl":
				go sslListener(u, ctx)
			case "http":
				go httpListener(u, ctx)
			case "https":
				go httpsListener(u, ctx)
			case "health":
				go healthCheckListener(u, ctx)

			default:
				Log.Errorln("Unknown listener: ", u)
				mainCancel()
				return
			}

		} else {
			Log.Errorln("Couldn't parse listener:", v)
			mainCancel()
			return
		}
	}
}
