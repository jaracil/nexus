// Package log
package log

import (
	"strings"

	"github.com/Sirupsen/logrus"
)

// Singleton logrus logger object with custom format.
// Verbosity can be changed through SetLogLevel.
var Log *logrus.Logger

const (
	PanicLevel uint8 = iota
	FatalLevel
	ErrorLevel
	WarnLevel
	InfoLevel
	DebugLevel
)

func init() {
	Log = logrus.New()
	customFormatter := new(logrus.TextFormatter)
	customFormatter.FullTimestamp = true
	customFormatter.TimestampFormat = "2006/01/02 15:04:05"
	customFormatter.ForceColors = true
	Log.Formatter = customFormatter
	Log.Level = logrus.DebugLevel
}

// Sets log level to one of (debug, info, warn, error, fatal, panic)
func SetLogLevel(l string) error {
	switch strings.ToLower(l) {
	case "debug":
		Log.Level = logrus.DebugLevel
	case "info":
		Log.Level = logrus.InfoLevel
	case "warn":
		Log.Level = logrus.WarnLevel
	case "error":
		Log.Level = logrus.ErrorLevel
	case "fatal":
		Log.Level = logrus.FatalLevel
	case "panic":
		Log.Level = logrus.PanicLevel
	}
	return nil
}
