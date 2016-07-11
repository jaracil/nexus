package main

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/net/context"
)

var (
	nodeId      string
	mainContext context.Context
	mainCancel  context.CancelFunc
	sesNotify   *Notifier      = NewNotifyer()
	sigChan     chan os.Signal = make(chan os.Signal, 1)
)

func signalManager() {
	for s := range sigChan {
		switch s {
		case syscall.SIGINT:
			exit("system INT signal")
		case syscall.SIGTERM:
			exit("system TERM signal")
		case syscall.SIGKILL:
			exit("system KILL signal")
		default:
		}
	}
}

func exit(cause string) {
	if mainContext.Err() == nil {
		errln("Daemon exit. Cause: ", cause)
		mainCancel()
	}
}

func main() {
	parseOptions()
	nodeId = safeId(4)
	signal.Notify(sigChan)
	go signalManager()

	mainContext, mainCancel = context.WithCancel(context.Background())
	err := dbOpen()
	if err != nil {
		errfatalln("Error opening rethinkdb connection:", err)
	}
	defer db.Close()

	go nodeTrack()
	go taskTrack()
	go pipeTrack()
	go taskPurge()

	listen()

	sysf("Nexus node [%s] started", nodeId)
	<-mainContext.Done()
	cleanNode(nodeId)
	sysf("Nexus node [%s] stopped", nodeId)
}
