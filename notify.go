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

type Notifyer struct {
	sync.RWMutex
	m map[string]*nPoint
}

func NewNotifyer() *Notifyer {
	return &Notifyer{m: make(map[string]*nPoint)}
}

func (nt *Notifyer) Register(key string, ch chan interface{}) (chan interface{}, error) {
	nt.Lock()
	defer nt.Unlock()
	if _, ok := nt.m[key]; ok {
		return ch, errors.New("Key already exists")
	}
	nt.m[key] = &nPoint{ch: ch}
	return ch, nil
}

func (nt *Notifyer) Unregister(key string) error {
	nt.Lock()
	defer nt.Unlock()
	if _, ok := nt.m[key]; !ok {
		return errors.New("Key not exists")
	}
	close(nt.m[key].ch)
	delete(nt.m, key)
	return nil
}

func (nt *Notifyer) Notify(key string, d interface{}) error {
	nt.RLock()
	defer nt.RUnlock()
	np, ok := nt.m[key]
	if ok {
		select {
		case np.ch <- d:
		default:
			atomic.AddInt64(&np.drops, 1)
			return errors.New("Overflow")
		}
	}
	return errors.New("Key not exists")
}

func (nt *Notifyer) Channel(key string) (chan interface{}, error) {
	nt.RLock()
	defer nt.RUnlock()
	np, ok := nt.m[key]
	if ok {
		return np.ch, nil
	}
	return nil, errors.New("Key not exists")
}

func (nt *Notifyer) Drops(key string, reset bool) (int, error) {
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
	return 0, errors.New("Key not exists")
}

func (nt *Notifyer) Waiting(key string) (int, error) {
	nt.RLock()
	defer nt.RUnlock()
	np, ok := nt.m[key]
	if ok {
		return len(np.ch), nil
	}
	return 0, errors.New("Key not exists")
}

func (nt *Notifyer) Purge(key string, n int) (int, error) {
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
	return 0, errors.New("Key not exists")
}
