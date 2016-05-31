package main

import (
	"log"
	"sync/atomic"
	"time"

	r "github.com/dancannon/gorethink"
	"github.com/jaracil/ei"
)

var masterNode = int32(0)

func isMasterNode() bool {
	return atomic.LoadInt32(&masterNode) != 0
}

func setMasterNode(master bool) {
	if master {
		atomic.StoreInt32(&masterNode, 1)
	} else {
		atomic.StoreInt32(&masterNode, 0)
	}
}

func nodeTrack() {
	defer exit("node tracker exit")

	// Insert node in node-tracking table
	ndata := ei.M{
		"id":       nodeId,
		"deadline": r.Now().Add(10),
		"kill":     false,
	}
	_, err := r.Table("nodes").Insert(ndata).RunWrite(db)
	if err != nil {
		log.Printf("Error, can't insert on nodes table [%s]", err.Error())
		return
	}
	// WatchDog loop
	tick := time.NewTicker(time.Second * 3)
	defer tick.Stop()
	exit := false
	for !exit {
		select {
		case <-tick.C:
			res, err := r.Table("nodes").
				Get(nodeId).
				Update(ei.M{"deadline": r.Now().Add(10)}, r.UpdateOpts{ReturnChanges: true}).
				RunWrite(db)
			if err != nil {
				log.Printf("Error, can't update on nodes table [%s]", err.Error())
				exit = true
				break
			}
			if res.Replaced == 0 {
				log.Printf("Error, cero records updated on nodes table. Record deleted?")
				exit = true
				break
			}
			newNodeData := ei.N(res.Changes[0].NewValue)
			if newNodeData.M("kill").BoolZ() {
				log.Printf("Ouch!, I'm killed")
				exit = true
				break
			}
			// Kill expired nodes
			r.Table("nodes").
				Filter(r.Row.Field("deadline").Lt(r.Now())).
				Filter(r.Row.Field("kill").Eq(false)).
				Update(ei.M{"kill": true}).
				RunWrite(db)

		case <-mainContext.Done():
			exit = true
		}
	}
	r.Table("nodes").
		Get(nodeId).
		Update(ei.M{"kill": true}).
		RunWrite(db)
}

func cleanNode(node string) {
	err := dbClean(node)
	if err == nil {
		r.Table("nodes").Get(node).Delete().RunWrite(db)
	}
}
