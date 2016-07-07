package main

const (
	ErrParse            = -32700
	ErrInvalidRequest   = -32600
	ErrMethodNotFound   = -32601
	ErrInvalidParams    = -32602
	ErrInternal         = -32603
	ErrTimeout          = -32000
	ErrCancel           = -32001
	ErrInvalidTask      = -32002
	ErrInvalidPipe      = -32003
	ErrInvalidUser      = -32004
	ErrUserExists       = -32005
	ErrPermissionDenied = -32010
	ErrTtlExpired       = -32011
)

var ErrStr = map[int]string{
	ErrParse:            "Parse error",
	ErrInvalidRequest:   "Invalid request",
	ErrMethodNotFound:   "Method not found",
	ErrInvalidParams:    "Invalid params",
	ErrInternal:         "Internal error",
	ErrTimeout:          "Timeout",
	ErrCancel:           "Cancel",
	ErrInvalidTask:      "Invalid task",
	ErrInvalidPipe:      "Invalid pipe",
	ErrInvalidUser:      "Invalid user",
	ErrUserExists:       "User already exists",
	ErrPermissionDenied: "Permission denied",
	ErrTtlExpired:       "TTL expired",
}
