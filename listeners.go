package main

import (
	"log"
	"net/url"
)

func listen() {
	for _, v := range opts.Listeners {
		if u, err := url.Parse(v); err == nil {

			switch u.Scheme {
			case "tcp":
				go tcpListener(u)
			case "ssl":
				go sslListener(u)
			case "http":
				go httpListener(u)
			case "https":
				go httpsListener(u)

			default:
				log.Println("Unknown listener: ", u)
				mainCancel()
				return
			}

		} else {
			log.Println("Couldn't parse listener:", v)
			mainCancel()
			return
		}
	}
}
