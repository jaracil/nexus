// Package log
package log

import "github.com/Sirupsen/logrus"

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
	Log.Formatter = customFormatter
	Log.Level = logrus.DebugLevel
}

// Sets log level to one of (debug, info, warn, error, fatal, panic)
func SetLogLevel(l uint8) {
	switch l {
	case DebugLevel:
		Log.Level = logrus.DebugLevel
	case InfoLevel:
		Log.Level = logrus.InfoLevel
	case WarnLevel:
		Log.Level = logrus.WarnLevel
	case ErrorLevel:
		Log.Level = logrus.ErrorLevel
	case FatalLevel:
		Log.Level = logrus.FatalLevel
	case PanicLevel:
		Log.Level = logrus.PanicLevel

	default:
		Log.Level = logrus.DebugLevel
	}
}

func GetLogLevel() uint8 {
	switch Log.Level {
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
