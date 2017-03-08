package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	. "github.com/jaracil/nexus/log"
	"github.com/rifflock/lfshook"
	"golang.org/x/net/context"
)

var (
	nodeId        string
	mainContext   context.Context
	mainCancel    context.CancelFunc
	sesNotify     *Notifier      = NewNotifier()
	sigChan       chan os.Signal = make(chan os.Signal, 1)
	listenContext context.Context
	listenCancel  context.CancelFunc
)

func signalManager() {
	for s := range sigChan {
		switch s {
		case syscall.SIGINT:
			if listenContext.Err() == nil {
				Log.Warnln("Stopping new connections")
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
		Log.WithFields(logrus.Fields{
			"cause": cause,
		}).Error("Daemon exit")
		mainCancel()
	}
}

func main() {
	parseOptions()
	nodeId = safeId(4)

	if len(opts.Verbose) > 0 {
		SetLogLevel(DebugLevel)
	} else {
		SetLogLevel(InfoLevel)
	}
	if opts.IsProduction {
		customFormatter := new(logrus.JSONFormatter)
		customFormatter.TimestampFormat = TimestampFormat
		Logger.Formatter = customFormatter
	}

	Log = GetLogger(nodeId, opts.Logs.AddSystemInfo)

	if opts.Logs.Path != "" {
		// Test if file will be accessible
		fd, err := os.OpenFile(opts.Logs.Path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			Log.WithFields(logrus.Fields{
				"error": err,
			}).Fatalln("Error opening logfile")
		}
		fd.Close()

		logfileFormatter := lfshook.NewHook(lfshook.PathMap{
			logrus.DebugLevel: opts.Logs.Path,
			logrus.InfoLevel:  opts.Logs.Path,
			logrus.WarnLevel:  opts.Logs.Path,
			logrus.ErrorLevel: opts.Logs.Path,
			logrus.FatalLevel: opts.Logs.Path,
			logrus.PanicLevel: opts.Logs.Path,
		})
		logfileFormatter.SetFormatter(Logger.Formatter)
		Logger.Hooks.Add(logfileFormatter)
	}

	signal.Notify(sigChan)
	go signalManager()

	mainContext, mainCancel = context.WithCancel(context.Background())
	err := dbOpen()
	if err != nil {
		Log.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("Error opening RethinkDB connection")
	}
	defer db.Close()

	go nodeTrack()
	go taskTrack()
	go pipeTrack()
	go sessionTrack()
	go taskPurge()
	go hooksTrack()

	listen()

	Log.Println("Nexus", Version.String(), "node started")

	<-mainContext.Done()
	cleanNode(nodeId)
	for numconn > 0 {
		time.Sleep(time.Second)
	}

	Log.Println("Nexus", Version.String(), "node stopped")
}
