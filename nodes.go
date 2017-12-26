package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jaracil/ei"
	. "github.com/jaracil/nexus/log"
	"github.com/shirou/gopsutil/load"
	"github.com/sirupsen/logrus"
	r "gopkg.in/gorethink/gorethink.v3"
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
		"version":  Version.String(),
	}

	_, err := r.Table("nodes").Insert(ndata).RunWrite(db)
	if err != nil {
		Log.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Errorf("Error, can't insert on nodes table")
		return
	}

	var masterCtx context.Context
	var masterCancel context.CancelFunc

	// WatchDog loop
	tick := time.NewTicker(time.Second * 3)
	defer tick.Stop()
	exit := false
	last_deadline_update := time.Now()
	for !exit {
		select {
		case <-tick.C:
			info := ei.M{
				"deadline":  r.Now().Add(10),
				"clients":   numconn,
				"listening": listenContext.Err() == nil,
			}
			if l, err := load.Avg(); err == nil {
				info["load"] = l
			}
			res, err := r.Table("nodes").
				Get(nodeId).
				Update(info, r.UpdateOpts{ReturnChanges: true}).
				RunWrite(db)
			if err != nil {
				Log.WithFields(logrus.Fields{
					"error": err.Error(),
				}).Errorf("Error, can't update on nodes table")
				exit = true
				break
			}
			if res.Replaced == 0 {
				Log.Errorf("Error, zero records updated on nodes table. Deleted record?")
				exit = true
				break
			}

			newNodeData := ei.N(res.Changes[0].NewValue)
			oldNodeData := ei.N(res.Changes[0].OldValue)

			if newNodeData.M("kill").BoolZ() {
				Log.WithFields(logrus.Fields{
					"time of last deadline": last_deadline_update,
					"stored deadline":       oldNodeData.M("deadline").StringZ(),
				}).Errorf("Ouch!, I've been killed")
				exit = true
				break
			}
			last_deadline_update = time.Now()

			// Kill expired nodes
			res, err = r.Table("nodes").
				Filter(r.Row.Field("deadline").Lt(r.Now())).
				Filter(r.Row.Field("kill").Eq(false)).
				Update(ei.M{"kill": true, "killed_at": r.Now()}, r.UpdateOpts{ReturnChanges: true}).
				RunWrite(db, r.RunOpts{ReadMode: "majority"})
			if err == nil {
				for _, ch := range res.Changes {
					n := ei.N(ch.NewValue)
					Log.WithFields(logrus.Fields{
						"killed":   n.M("id").StringZ(),
						"now":      n.M("killed_at").TimeZ(),
						"deadline": n.M("deadline").TimeZ(),
					}).Printf("Killing node")
				}
			}
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
						Log.WithFields(logrus.Fields{
							"cleaned": id,
						}).Printf("Cleaning node")
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

							masterCtx, masterCancel = context.WithCancel(context.Background())
							go searchOrphaned(masterCtx)
							go searchUncompleted(masterCtx)
						}
					} else {
						if isMasterNode() {
							Log.Printf("I'm NOT the master node anymore")
							setMasterNode(false)
							masterCancel()
						}
					}
				}
			}

		case <-mainContext.Done():
			exit = true
		}
	}

	if masterCancel != nil {
		masterCancel()
	}

	r.Table("nodes").
		Get(nodeId).
		Update(ei.M{"kill": true}).
		RunWrite(db)
}

func searchOrphaned(ctx context.Context) {
	t := time.After(time.Second)
	for {
		select {
		case <-ctx.Done():
			return

		case <-t:
			t = time.After(time.Minute)
			nodes := make([]map[string]interface{}, 0)
			tcur, err := r.Table("nodes").Pluck("id").Run(db)
			if err != nil {
				Log.WithFields(logrus.Fields{
					"error": err,
				}).Errorf("Error listing nodes")
				return
			}
			tcur.All(&nodes)

			var nodesregexp string
			// (^node1|^node2|^node3)
			if len(nodes) > 0 {
				nodesregexp = "("
				for k, node := range nodes {
					nodesregexp = fmt.Sprintf("%s^%s", nodesregexp, node["id"])

					if k < len(nodes)-1 {
						nodesregexp = fmt.Sprintf("%s|", nodesregexp)
					} else {
						nodesregexp = fmt.Sprintf("%s)", nodesregexp)
					}
				}
			} else {
				Log.Errorf("Length of nodes list is 0... who am I??")
				return
			}

			searchOrphanedStuff(nodesregexp, "sessions", "nodeId")
			searchOrphanedStuff(nodesregexp, "tasks", "id")
			searchOrphanedStuff(nodesregexp, "pipes", "id")
			searchOrphanedStuff(nodesregexp, "locks", "owner")
		}
	}
}

func searchOrphanedStuff(regex, what, field string) {

	orphaned, err := r.Table(what).Filter(func(ses r.Term) r.Term {
		return ses.Field(field).Match(regex).Not()
	}).Run(db)
	if err != nil {
		Log.WithFields(logrus.Fields{
			"error": err,
		}).Errorf("Error searching orphaned %s", what)
		return
	}

	orphans := make([]interface{}, 0)
	err = orphaned.All(&orphans)

	switch err {
	default:
		Log.WithFields(logrus.Fields{
			"error": err,
		}).Errorf("Error searching orphaned %s", what)

	case r.ErrEmptyResult:

	case nil:
		if len(orphans) <= 0 {
			return
		}

		o := make([]string, 0)
		for _, e := range orphans {
			if om, ok := e.(map[string]interface{}); ok {
				o = append(o, fmt.Sprintf("%s", om[field])[:8])
			}
		}

		Log.WithFields(logrus.Fields{
			"orphans": o,
		}).Warnf("Found %d orphaned %s", len(o), what)

		for _, s := range o {
			err := dbClean(s)

			if err != nil {
				Log.WithFields(logrus.Fields{
					"error": err,
					what:    s,
				}).Errorln("Error deleting orphaned %s", what)
			}
		}
	}
}

func searchUncompleted(ctx context.Context) {
	t := time.After(time.Second)
	for {
		select {
		case <-ctx.Done():
			return

		case <-t:
			t = time.After(time.Minute)
			searchUncompletedRenames()
		}
	}
}

func searchUncompletedRenames() {
	cur, err := r.Table("users").Between("", "\uffff", r.BetweenOpts{Index: "blockedBy"}).Run(db)
	if err != nil {
		if err != nil {
			Log.WithFields(logrus.Fields{
				"error": err,
			}).Errorf("Error searching uncompleted user renames")
			return
		}
	}
	uncompleted := make([]interface{}, 0)
	err = cur.All(&uncompleted)
	cur.Close()

	switch err {
	default:
		Log.WithFields(logrus.Fields{
			"error": err,
		}).Errorf("Error searching uncompleted user renames")

	case r.ErrEmptyResult:

	case nil:
		if len(uncompleted) <= 0 {
			return
		}

		for _, u := range uncompleted {
			user := ei.N(u).M("id").StringZ()
			blockedBy := ei.N(u).M("blockedBy").StringZ()
			renaming := ei.N(u).M("renaming").StringZ()
			if user != "" && blockedBy != "" {
				cur, err = r.Table("sessions").Get(blockedBy).Run(db)
				if err != nil {
					Log.WithFields(logrus.Fields{
						"error": err,
					}).Errorf("Error searching uncompleted user rename session for %s", user)
					cur.Close()
					continue
				}
				if cur.IsNil() {
					if renaming == "me" {
						r.Table("users").Get(user).Delete().RunWrite(db)
					} else {
						if renaming != "" {
							_, err = r.Table("users").Get(renaming).Delete().RunWrite(db)
							if err != nil {
								cur.Close()
								continue
							}
						}
						r.Table("users").Get(user).Replace(func(t r.Term) r.Term { return t.Without("blockedBy", "renaming") })
					}
				}
				cur.Close()
			}
		}
	}
}

func cleanNode(node string) {
	err := dbClean(node)
	if err == nil {
		r.Table("nodes").Get(node).Delete().RunWrite(db)
	} else {
		Log.WithFields(logrus.Fields{
			"node":  node,
			"error": err.Error(),
		}).Error("Error cleaning node")
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

		term := r.Table("nodes").Pluck("id", "clients", "load", "version", "listening")

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
