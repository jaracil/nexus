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
			// Clean killed nodes after 10 seconds.
			cur, err := r.Table("nodes").
				Filter(r.Row.Field("deadline").Lt(r.Now().Add(-10))).
				Filter(r.Row.Field("kill").Eq(true)).
				Run(db)
			if err == nil {
				nodesKilled := ei.S{}
				err = cur.All(&nodesKilled)
				if err == nil {
					for _, n := range nodesKilled {
						id := ei.N(n).M("id").StringZ()
						cleanNode(id)
						log.Printf("Cleaning node [%s]", id)
					}
				}
			}
			// Check if this is the master node
			cur, err = r.Table("nodes").Min("id").Run(db)
			if err == nil {
				firstNode := ei.M{}
				err = cur.One(&firstNode)
				if err == nil {
					if ei.N(firstNode).M("id").StringZ() == nodeId {
						if !isMasterNode() {
							log.Printf("Now I'm master node")
							setMasterNode(true)
						}
					} else {
						if isMasterNode() {
							log.Printf("Now I'm not master node")
							setMasterNode(false)
						}
					}
				}
			}

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
	} else {
		log.Printf("Error cleaning node [%s]: %s", node, err)
	}
}
