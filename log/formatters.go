package log

import (
	"encoding/json"
	"fmt"

	"github.com/Sirupsen/logrus"
)

// ProductionFormatter is a copy of the code from the JSONFormatter but slightly modified to always include some custom fields
type ProductionFormatter struct {
	// TimestampFormat sets the format used for marshaling timestamps.
	TimestampFormat string
	NodeID          string
}

func prefixFieldClashes(data logrus.Fields) {
	if t, ok := data["time"]; ok {
		data["fields.time"] = t
	}

	if m, ok := data["msg"]; ok {
		data["fields.msg"] = m
	}

	if l, ok := data["level"]; ok {
		data["fields.level"] = l
	}
}

func (f ProductionFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	data := make(logrus.Fields, len(entry.Data)+3)
	for k, v := range entry.Data {
		switch v := v.(type) {
		case error:
			// Otherwise errors are ignored by `encoding/json`
			// https://github.com/Sirupsen/logrus/issues/137
			data[k] = v.Error()
		default:
			data[k] = v
		}
	}
	prefixFieldClashes(data)

	timestampFormat := f.TimestampFormat
	if timestampFormat == "" {
		timestampFormat = logrus.DefaultTimestampFormat
	}

	data["time"] = entry.Time.Format(timestampFormat)
	data["msg"] = entry.Message
	data["level"] = entry.Level.String()
	data["node"] = f.NodeID

	serialized, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("Failed to marshal fields to JSON, %v", err)
	}
	return append(serialized, '\n'), nil
}
