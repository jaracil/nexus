package main

const (
	ErrParse            = -32700
	ErrInvalidRequest   = -32600
	ErrInternal         = -32603
	ErrInvalidParams    = -32602
	ErrMethodNotFound   = -32601
	ErrTtlExpired       = -32011
	ErrPermissionDenied = -32010
	ErrLockNotOwned     = -32006
	ErrUserExists       = -32005
	ErrInvalidUser      = -32004
	ErrInvalidPipe      = -32003
	ErrInvalidTask      = -32002
	ErrCancel           = -32001
	ErrTimeout          = -32000
	ErrNoError          = 0
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
