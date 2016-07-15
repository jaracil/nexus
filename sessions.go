package main

import (
	"log"
	"time"

	r "github.com/dancannon/gorethink"
)

type Session struct {
	Id   string `gorethink:"id"`
	Kick bool   `gorethink:"kick"`
}

type SessionFeed struct {
	Old *Session `gorethink:"old_val"`
	New *Session `gorethink:"new_val"`
}

func sessionTrack() {
	defer exit("sessions change-feed error")
	for retry := 0; retry < 10; retry++ {
		iter, err := r.Table("sessions").
			Between(nodeId, nodeId+"\uffff").
			Changes(r.ChangesOpts{IncludeInitial: true, Squash: false}).
			Pluck(map[string]interface{}{
				"new_val": []string{"id", "kick"},
				"old_val": []string{"id"}}).
			Run(db)
		if err != nil {
			log.Printf("Error opening sessionTrack iterator:%s\n", err.Error())
			time.Sleep(time.Second)
			continue
		}
		retry = 0
		for {
			sf := &SessionFeed{}
			if !iter.Next(sf) {
				log.Printf("Error processing feed: %s\n", iter.Err().Error())
				iter.Close()
				break
			}
			if sf.New == nil { // session closed
				sesNotify.Unregister(sf.Old.Id)
				continue
			}
			sesNotify.Notify(sf.New.Id, sf.New)
		}
	}
}
