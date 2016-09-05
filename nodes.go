package main

import (
	"sync/atomic"
	"time"

	r "github.com/dancannon/gorethink"
	"github.com/jaracil/ei"
	. "github.com/jaracil/nexus/log"
	"github.com/shirou/gopsutil/load"
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
		"version":  Version,
	}
	_, err := r.Table("nodes").Insert(ndata).RunWrite(db)
	if err != nil {
		Log.Errorf("Error, can't insert on nodes table [%s]", err.Error())
		return
	}
	// WatchDog loop
	tick := time.NewTicker(time.Second * 3)
	defer tick.Stop()
	exit := false
	for !exit {
		select {
		case <-tick.C:
			info := ei.M{
				"deadline": r.Now().Add(10),
				"clients":  numconn,
			}
			if l, err := load.Avg(); err == nil {
				info["load"] = l
			}
			res, err := r.Table("nodes").
				Get(nodeId).
				Update(info, r.UpdateOpts{ReturnChanges: true}).
				RunWrite(db)
			if err != nil {
				Log.Errorf("Error, can't update on nodes table [%s]", err.Error())
				exit = true
				break
			}
			if res.Replaced == 0 {
				Log.Errorf("Error, zero records updated on nodes table. Deleted record?")
				exit = true
				break
			}
			newNodeData := ei.N(res.Changes[0].NewValue)
			if newNodeData.M("kill").BoolZ() {
				Log.Errorf("Ouch!, I've been killed")
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
						Log.Printf("Cleaning node [%s]", id)
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
							Log.Printf("I'm the master node now")
							setMasterNode(true)
						}
					} else {
						if isMasterNode() {
							Log.Printf("I'm NOT the master node anymore")
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
		Log.Errorf("Error cleaning node [%s]: %s", node, err)
	}
}

func (nc *NexusConn) handleNodesReq(req *JsonRpcReq) {
	switch req.Method {
	case "sys.node.list":
		limit, err := ei.N(req.Params).M("limit").Int()
		if err != nil {
			limit = 100
		}
		skip, err := ei.N(req.Params).M("skip").Int()
		if err != nil {
			skip = 0
		}
		tags := nc.getTags("sys.node")
		if !(ei.N(tags).M("@sys.node.list").BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		term := r.Table("nodes").Pluck("id", "clients", "load", "version")

		if skip >= 0 {
			term = term.Skip(skip)
		}

		if limit > 0 {
			term = term.Limit(limit)
		}

		cur, err := term.Run(db)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		var all []interface{}
		cur.All(&all)
		req.Result(all)
	default:
		req.Error(ErrMethodNotFound, "", nil)
	}
}
