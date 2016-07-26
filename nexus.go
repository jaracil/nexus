package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/context"
)

var (
	nodeId        string
	mainContext   context.Context
	mainCancel    context.CancelFunc
	sesNotify     *Notifyer      = NewNotifyer()
	sigChan       chan os.Signal = make(chan os.Signal, 1)
	listenContext context.Context
	listenCancel  context.CancelFunc
)

func signalManager() {
	for s := range sigChan {
		switch s {
		case syscall.SIGINT:
			if listenContext.Err() == nil {
				log.Println("Stopping new connections")
				listenCancel()
				go func() {
					for numconn > 0 {
						time.Sleep(time.Second)
					}
					exit("there is no connection left")
				}()
			} else {
				exit("system INT signal")
			}
		case syscall.SIGTERM:
			exit("system TERM signal")
		case syscall.SIGKILL:
			exit("system KILL signal")
		case syscall.SIGUSR1:
			listenCancel()
		case syscall.SIGUSR2:
			if listenContext.Err() != nil {
				listen()
			}
		default:
		}
	}
}

func exit(cause string) {
	if mainContext.Err() == nil {
		println("Daemon exit, cause: ", cause)
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
		log.Fatal("Error opening rethinkdb connection:", err)
	}
	defer db.Close()

	go nodeTrack()
	go taskTrack()
	go pipeTrack()
	go sessionTrack()
	go taskPurge()

	listen()

	log.Printf("Start Daemon, Node ID:%s\r\n", nodeId)
	<-mainContext.Done()
	cleanNode(nodeId)
	for numconn > 0 {
		time.Sleep(time.Second)
	}
	log.Printf("Stop Daemon, Node ID:%s\r\n", nodeId)
}
