package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	r "github.com/dancannon/gorethink"
	"github.com/jaracil/ei"
	. "github.com/jaracil/nexus/log"
)

type HookBans struct {
	*sync.RWMutex
	Map map[string]time.Time
}

type HookCache struct {
	*sync.Mutex
	Map map[string]*HookCacheItem
}

type HookCacheItem struct {
	List   []interface{}
	Expire time.Time
}

func (c *HookCache) Get(p string) []interface{} {
	c.Lock()
	if res, ok := c.Map[p]; ok {
		res.Expire = time.Now().Add(_hookCacheTime)
		c.Unlock()
		return res.List
	}
	c.Unlock()
	return nil
}

func (c *HookCache) Set(p string, list []interface{}) {
	c.Lock()
	c.Map[p] = &HookCacheItem{list, time.Now().Add(_hookCacheTime)}
	c.Unlock()
}

var _validHookTypes = []string{"task", "user"}

var hookBans = &HookBans{
	&sync.RWMutex{},
	map[string]time.Time{},
}
var _hookBanTime = time.Minute * 5

var hookCache = &HookCache{
	&sync.Mutex{},
	map[string]*HookCacheItem{},
}
var _hookCacheTime = time.Hour
var _hookCacheExpirePeriod = time.Minute * 30

func hookList(ty string, path string, user string) (res []interface{}) {
	p := fmt.Sprintf("%s|%s|%s", ty, path, user)
	if res = hookCache.Get(p); res != nil {
		return res
	}
	res = append(res, "hook.*", "hook."+ty+".*")
	for _, ps := range topicList(path) {
		res = append(res, fmt.Sprintf("hook.%s|%s", ty, ps))
		for _, us := range topicList(user) {
			res = append(res, fmt.Sprintf("hook.%s|%s|%s", ty, ps, us))
		}
	}
	hookCache.Set(p, res)
	return
}

func hookPublish(ty string, path string, user string, message interface{}) (int, error) {
	msg := ei.M{"topic": fmt.Sprintf("hook.%s|%s|%s", ty, path, user), "msg": message}
	hookTopics := hookList(ty, path, user)
	res, err := r.Table("pipes").
		GetAllByIndex("subs", hookTopics...).
		Update(map[string]interface{}{"msg": r.Literal(msg), "count": r.Row.Field("count").Add(1), "ismsg": true}).
		RunWrite(db, r.RunOpts{Durability: "soft"})
	return res.Replaced, err
}

func hook(ty string, path string, user string, data interface{}) {
	if hookIsBanned(ty, path, user) {
		return
	}
	switch ty {
	case "task", "user":
		n, _ := hookPublish(ty, path, user, data)
		if n == 0 {
			hookBan(ty, path, user)
		}
	}
}

func hookBan(ty string, path string, user string) {
	hookBans.Lock()
	hookBans.Map[ty+"|"+path+"|"+user] = time.Now().Add(_hookBanTime)
	hookBans.Unlock()
}

func hookUnban(ty string, path string, user string) {
	hookBans.Lock()
	typ, _ := normalizeHookPath(ty)
	if typ == "" { // All
		hookBans.Map = map[string]time.Time{}
	} else {
		pth, rec := normalizeHookPath(path)
		if pth == "" && rec { // All paths of one type
			for k, _ := range hookBans.Map {
				if strings.HasPrefix(k, ty+"|") {
					delete(hookBans.Map, k)
				}
			}
		} else if rec { // Some paths of one type
			for k, _ := range hookBans.Map {
				if strings.HasPrefix(k, ty+"|"+pth+".") || strings.HasPrefix(k, ty+"|"+pth+"|") {
					delete(hookBans.Map, k)
				}
			}
		} else {
			usr, rec := normalizeHookPath(user)
			if usr == "" && rec { // All users of one path
				for k, _ := range hookBans.Map {
					if strings.HasPrefix(k, ty+"|"+pth+"|") {
						delete(hookBans.Map, k)
					}
				}
			} else if rec { // Some users of one path
				for k, _ := range hookBans.Map {
					if strings.HasPrefix(k, ty+"|"+pth+"|"+usr+".") || k == ty+"|"+pth+"|"+usr {
						delete(hookBans.Map, k)
					}
				}
			} else { // One user of one path
				delete(hookBans.Map, ty+"|"+pth+"|"+usr)
			}
		}
	}
	hookBans.Unlock()
}

func normalizeHookPath(s string) (string, bool) {
	if s == "" || s == "*" {
		return "", true
	}
	recursive := strings.HasSuffix(s, ".*")
	return strings.TrimRight(s, "*."), recursive
}

func hookIsBanned(ty string, path string, user string) bool {
	hookBans.RLock()
	if t, ok := hookBans.Map[ty+"|"+path+"|"+user]; ok {
		if time.Since(t) <= 0 {
			hookBans.RUnlock()
			return true
		}
		hookBans.RUnlock()
		hookUnban(ty, path, user)
		return false
	}
	hookBans.RUnlock()
	return false
}

func hooksTrack() {
	go hookCacheExpire()
	defer exit("hooks topic-listen error")
	nic := NewInternalClient()
	defer nic.Close()
HookLoop:
	for retry := 0; retry < 10; retry++ {
		pipe, err := nic.PipeCreate()
		if err != nil {
			Log.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Errorln("Error creating pipe on hooks topic-listen")
			time.Sleep(time.Second)
			continue
		}
		_, err = nic.TopicSubscribe(pipe, "hook.listen")
		if err != nil {
			Log.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Errorln("Error subscribing to topic on hooks topic-listen")
			time.Sleep(time.Second)
			continue
		}
		retry = 0
		for {
			topicData, err := pipe.TopicRead(10, time.Minute)
			if err != nil {
				Log.WithFields(logrus.Fields{
					"error": err.Error(),
				}).Errorln("Error reading from pipe on hooks topic-listen")
				time.Sleep(time.Second)
				continue HookLoop
			}
			if topicData.Drops != 0 {
				Log.WithFields(logrus.Fields{
					"drops": topicData.Drops,
				}).Warnf("Got drops reading from pipe on hooks topic-listen", topicData.Drops)
			}
			for _, msg := range topicData.Msgs {
				m := ei.N(msg.Msg)
				ty := m.M("type").StringZ()
				path := m.M("path").StringZ()
				user := m.M("user").StringZ()
				hookUnban(ty, path, user)
			}
		}
	}
}

func hookCacheExpire() {
	for {
		time.Sleep(_hookCacheExpirePeriod)
		hookCache.Lock()
		now := time.Now()
		for key, ci := range hookCache.Map {
			if ci.Expire.Before(now) {
				delete(hookCache.Map, key)
			}
		}
		hookCache.Unlock()
	}
}
