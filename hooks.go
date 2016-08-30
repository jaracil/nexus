package main

import (
	"fmt"
	"sync"
	"time"

	r "github.com/dancannon/gorethink"
	. "github.com/jaracil/nexus/log"
	"github.com/jaracil/ei"
)

var _hookBanTime = time.Minute * 5

type HookBans struct {
	*sync.RWMutex
	Task map[string]time.Time
}

var hookBans = &HookBans{
	&sync.RWMutex{},
	map[string]time.Time{},
}

var _validHookTypes = []string{"task"}

func hookList(ty string, path string, user string) (res []interface{}) {
	for _, ps := range topicList(path) {
		for _, us := range topicList(user) {
			res = append(res, fmt.Sprintf("%s|%s|%s", ty, ps, us))
		}
	}
	return
}

func hookPublish(ty string, path string, user string, message interface{}) (int, error) {
	msg := ei.M{"topic": fmt.Sprintf("%s|%s|%s", ty, path, user), "msg": message}
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
		case "task":
			n, _ := hookPublish(ty, path, user, data)
			if n == 0 {
				hookBan(ty, path, user)
			}
	}
}

func hookBan(ty string, path string, user string) {
	hookBans.Lock()
	switch ty {
		case "task":
			hookBans.Task[path+"|"+user] = time.Now().Add(_hookBanTime)
	}
	hookBans.Unlock()
}

func hookUnban(ty string, path string, user string) {
	hookBans.Lock()
	switch ty {
		case "task":
			delete(hookBans.Task, path+"|"+user)
	}
	hookBans.Unlock()
}

func hookIsBanned(ty string, path string, user string) bool {
	hookBans.RLock()
	var banMap map[string]time.Time
	switch ty {
		case "task":
			banMap = hookBans.Task
	}

	if t, ok := banMap[path+"|"+user]; ok {
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

func hooksTopicListen() {
	defer exit("hooks topic-listen error")
	nic := NewInternalClient()
	defer nic.Close()
HookLoop:
	for retry := 0; retry < 10; retry++ {
		pipe, err := nic.PipeCreate()
		if err != nil {
			Log.Errorln("Error creating pipe on hooks topic-listen:", err.Error())
			time.Sleep(time.Second)
			continue
		}
		_, err = nic.TopicSubscribe(pipe, "hook.listen")
		if err != nil {
			Log.Errorln("Error subscribing to topic on hooks topic-listen:", err.Error())
			time.Sleep(time.Second)
			continue
		}
		retry = 0
		for {
			topicData, err := pipe.TopicRead(10, time.Minute)
			if err != nil {
				Log.Errorln("Error reading from pipe on hooks topic-listen")
				time.Sleep(time.Second)
				continue HookLoop
			}
			if topicData.Drops != 0 {
				Log.Printf("Got %d drops reading from pipe on hooks topic-listen", topicData.Drops)
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