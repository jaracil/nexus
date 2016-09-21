package main

import (
	"os"

	"github.com/jessevdk/go-flags"
)

var opts struct {
	Verbose      []bool         `short:"v" long:"verbose" description:"Show debug information. Set multiple times to increase verbosity"`
	Listeners    []string       `short:"l" long:"listen"  description:"Listen on (tcp|tcp+proxy|ssl|ssl+proxy|http|https)://addr:port" default:"tcp://0.0.0.0:1717"`
	IsProduction bool           `long:"production" description:"Enables Production mode"`
	Rethink      RethinkOptions `group:"RethinkDB Options"`
	SSL          SSLOptions     `group:"SSL Options"`
}

type RethinkOptions struct {
	Hosts      []string `short:"r" long:"rethinkdb" description:"RethinkDB host[:port]" default:"localhost:28015"`
	Database   string   `short:"d" long:"database" description:"RethinkDB database" default:"nexus"`
	MaxIdle    int      `long:"maxidle" description:"Max RethinkDB idle connections" default:"50"`
	MaxOpen    int      `long:"maxopen" description:"Max RethinkDB open connections" default:"200"`
	DefPipeLen int      `long:"defpipelen" description:"Default pipe length" default:"1000"`
	MaxPipeLen int      `long:"maxpipelen" description:"Max pipe length" default:"100000"`
}

type SSLOptions struct {
	Cert string `long:"sslCert" description:"SSL Certificate" default:"nexus.crt"`
	Key  string `long:"sslKey" description:"SSL Key" default:"nexus.key"`
}

func parseOptions() {
	_, err := flags.Parse(&opts)
	if err != nil {
		os.Exit(1)
	}
}
