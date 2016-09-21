// Package log
package log

import (
	"io/ioutil"

	"github.com/Sirupsen/logrus"
)

// Singleton logrus logger object with custom format.
// Verbosity can be changed through SetLogLevel.
var Log *logrus.Entry
var Logger *logrus.Logger

var TimestampFormat = "2006/01/02 15:04:05.000000 -0700"

const (
	PanicLevel uint8 = iota
	FatalLevel
	ErrorLevel
	WarnLevel
	InfoLevel
	DebugLevel
)

func init() {
	Logger = logrus.New()
	customFormatter := &logrus.TextFormatter{DisableSorting: false}
	customFormatter.FullTimestamp = true
	customFormatter.TimestampFormat = TimestampFormat
	Logger.Formatter = customFormatter
	Logger.Level = logrus.DebugLevel
	Log = Logger.WithFields(logrus.Fields{
		"node": "not initialized",
	})
}

// Sets log level to one of (debug, info, warn, error, fatal, panic)
func SetLogLevel(l uint8) {
	switch l {
	case DebugLevel:
		Logger.Level = logrus.DebugLevel
	case InfoLevel:
		Logger.Level = logrus.InfoLevel
	case WarnLevel:
		Logger.Level = logrus.WarnLevel
	case ErrorLevel:
		Logger.Level = logrus.ErrorLevel
	case FatalLevel:
		Logger.Level = logrus.FatalLevel
	case PanicLevel:
		Logger.Level = logrus.PanicLevel

	default:
		Logger.Level = logrus.DebugLevel
	}
}

func GetLogLevel() uint8 {
	switch Logger.Level {
	case logrus.DebugLevel:
		return DebugLevel
	case logrus.InfoLevel:
		return InfoLevel
	case logrus.WarnLevel:
		return WarnLevel
	case logrus.ErrorLevel:
		return ErrorLevel
	case logrus.FatalLevel:
		return FatalLevel
	case logrus.PanicLevel:
		return PanicLevel

	default:
		return DebugLevel
	}
}

func LogLevelIs(l uint8) bool {
	return GetLogLevel() == l
}

func LogWithNode(node string) *logrus.Entry {
	return Logger.WithFields(logrus.Fields{
		"node": node,
		"type": "system",
	})
}

func LogDiscard() *logrus.Entry {
	Logger := logrus.New()
	Logger.Out = ioutil.Discard
	return Logger.WithFields(logrus.Fields{
		"node": "not initialized",
	})
}
