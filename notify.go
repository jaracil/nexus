package main

import (
	"errors"
	"sync"
	"sync/atomic"
)

type nPoint struct {
	ch    chan interface{}
	drops int64
}

type Notifier struct {
	sync.RWMutex
	m map[string]*nPoint
}

func NewNotifyer() *Notifier {
	return &Notifier{m: make(map[string]*nPoint)}
}

var ERROR_KEY_EXISTS = errors.New("Key already exists")
var ERROR_KEY_NOT_EXISTS = errors.New("Key doesn't exist")
var ERROR_OVERFLOW = errors.New("Overflow")

func (nt *Notifier) Register(key string, ch chan interface{}) (chan interface{}, error) {
	nt.Lock()
	defer nt.Unlock()
	if _, ok := nt.m[key]; ok {
		return ch, ERROR_KEY_EXISTS
	}
	nt.m[key] = &nPoint{ch: ch}
	return ch, nil
}

func (nt *Notifier) Unregister(key string) error {
	nt.Lock()
	defer nt.Unlock()
	if _, ok := nt.m[key]; !ok {
		return ERROR_KEY_NOT_EXISTS
	}
	close(nt.m[key].ch)
	delete(nt.m, key)
	return nil
}

func (nt *Notifier) Notify(key string, d interface{}) error {
	nt.RLock()
	defer nt.RUnlock()
	np, ok := nt.m[key]
	if ok {
		select {
		case np.ch <- d:
		default:
			atomic.AddInt64(&np.drops, 1)
			return ERROR_OVERFLOW
		}
	}
	return ERROR_KEY_NOT_EXISTS
}

func (nt *Notifier) Channel(key string) (chan interface{}, error) {
	nt.RLock()
	defer nt.RUnlock()
	np, ok := nt.m[key]
	if ok {
		return np.ch, nil
	}
	return nil, ERROR_KEY_NOT_EXISTS
}

func (nt *Notifier) Drops(key string, reset bool) (int, error) {
	nt.RLock()
	defer nt.RUnlock()
	np, ok := nt.m[key]
	if ok {
		var drops int64
		if reset {
			drops = atomic.SwapInt64(&np.drops, 0)
		} else {
			drops = atomic.LoadInt64(&np.drops)
		}
		return int(drops), nil
	}
	return 0, ERROR_KEY_NOT_EXISTS
}

func (nt *Notifier) Waiting(key string) (int, error) {
	nt.RLock()
	defer nt.RUnlock()
	np, ok := nt.m[key]
	if ok {
		return len(np.ch), nil
	}
	return 0, ERROR_KEY_NOT_EXISTS
}

func (nt *Notifier) Purge(key string, n int) (int, error) {
	nt.RLock()
	defer nt.RUnlock()
	purged := 0
	np, ok := nt.m[key]
	if ok {
		for {
			select {
			case <-np.ch:
				purged++
				n--
				if n == 0 {
					return purged, nil
				}
			default:
				return purged, nil
			}
		}
	}
	return 0, ERROR_KEY_NOT_EXISTS
}
