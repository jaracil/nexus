package main

import (
	"strings"
	"time"

	"github.com/jaracil/ei"
	. "github.com/jaracil/nexus/log"
	r "gopkg.in/gorethink/gorethink.v3"
)

var db *r.Session

func dbOpen() (err error) {
	db, err = r.Connect(r.ConnectOpts{
		Addresses: opts.Rethink.Hosts,
		Database:  opts.Rethink.Database,
		MaxIdle:   opts.Rethink.MaxIdle,
		MaxOpen:   opts.Rethink.MaxOpen,
	})
	if err != nil {
		return
	}
	err = dbBootstrap()
	if err != nil {
		db.Close()
	}
	return
}

func dbBootstrap() error {
	cur, err := r.DBList().Run(db)
	if err != nil {
		return err
	}
	dblist := make([]string, 0)
	err = cur.All(&dblist)
	cur.Close()
	if err != nil {
		return err
	}
	dbexists := false
	for _, x := range dblist {
		if x == opts.Rethink.Database {
			dbexists = true
			break
		}
	}
	if !dbexists {
		_, err := r.DBCreate(opts.Rethink.Database).RunWrite(db)
		if err != nil {
			return err
		}
	}
	cur, err = r.TableList().Run(db)
	if err != nil {
		return err
	}
	tablelist := make([]string, 0)
	err = cur.All(&tablelist)
	cur.Close()
	if err != nil {
		return err
	}
	if !inStrSlice(tablelist, "tasks") {
		Log.Println("Creating tasks table")
		_, err := r.TableCreate("tasks").RunWrite(db)
		if err != nil {
			return err
		}
	}
	if !inStrSlice(tablelist, "pipes") {
		Log.Println("Creating pipes table")
		_, err := r.TableCreate("pipes").RunWrite(db)
		if err != nil {
			return err
		}

	}
	if !inStrSlice(tablelist, "users") {
		Log.Println("Creating users table")
		_, err := r.TableCreate("users").RunWrite(db)
		if err != nil {
			return err
		}
		Log.Println("Creating root user")
		ud := UserData{User: "root", Salt: safeId(16), Tags: map[string]map[string]interface{}{".": {"@admin": true}}}
		ud.Pass, err = HashPass("root", ud.Salt)
		_, err = r.Table("users").Insert(&ud).RunWrite(db)
		if err != nil {
			return err
		}

	}
	if !inStrSlice(tablelist, "sessions") {
		Log.Println("Creating sessions table")
		_, err := r.TableCreate("sessions").RunWrite(db)
		if err != nil {
			return err
		}
	}
	cur, err = r.Table("sessions").IndexList().Run(db)
	sessionsIndexList := make([]string, 0)
	err = cur.All(&sessionsIndexList)
	cur.Close()
	if err != nil {
		return err
	}
	if !inStrSlice(sessionsIndexList, "users") {
		Log.Println("Creating users index on tasks sessions")
		_, err := r.Table("sessions").IndexCreateFunc("users", func(row r.Term) interface{} {
			return row.Field("user")
		}).RunWrite(db)
		if err != nil {
			return err
		}
	}
	if !inStrSlice(tablelist, "nodes") {
		Log.Println("Creating nodes table")
		_, err := r.TableCreate("nodes").RunWrite(db)
		if err != nil {
			return err
		}
	}
	if !inStrSlice(tablelist, "locks") {
		Log.Println("Creating locks table")
		_, err := r.TableCreate("locks").RunWrite(db)
		if err != nil {
			return err
		}
	}
	cur, err = r.Table("pipes").IndexList().Run(db)
	pipesIndexlist := make([]string, 0)
	err = cur.All(&pipesIndexlist)
	cur.Close()
	if err != nil {
		return err
	}
	if !inStrSlice(pipesIndexlist, "subs") {
		Log.Println("Creating subs index on pipes table")
		_, err := r.Table("pipes").IndexCreateFunc("subs", func(row r.Term) interface{} {
			return row.Field("subs")
		}, r.IndexCreateOpts{Multi: true}).RunWrite(db)
		if err != nil {
			return err
		}
	}
	cur, err = r.Table("tasks").IndexList().Run(db)
	tasksIndexlist := make([]string, 0)
	err = cur.All(&tasksIndexlist)
	cur.Close()
	if err != nil {
		return err
	}
	if !inStrSlice(tasksIndexlist, "pspc") {
		Log.Println("Creating pspc index on tasks table")
		_, err := r.Table("tasks").IndexCreateFunc("pspc", func(row r.Term) interface{} {
			return ei.S{row.Field("path"), row.Field("stat"), row.Field("prio"), row.Field("creationTime")}
		}).RunWrite(db)
		if err != nil {
			return err
		}
	}
	if !inStrSlice(tasksIndexlist, "deadLine") {
		Log.Println("Creating deadLine index on tasks table")
		_, err := r.Table("tasks").IndexCreateFunc("deadLine", func(row r.Term) interface{} {
			return row.Field("deadLine")
		}).RunWrite(db)
		if err != nil {
			return err
		}
	}
	if !inStrSlice(tasksIndexlist, "tses") {
		Log.Println("Creating tses index on tasks table")
		_, err := r.Table("tasks").IndexCreateFunc("tses", func(row r.Term) interface{} {
			return row.Field("tses")
		}).RunWrite(db)
		if err != nil {
			return err
		}
	}
	cur, err = r.Table("locks").IndexList().Run(db)
	locksIndexlist := make([]string, 0)
	err = cur.All(&locksIndexlist)
	cur.Close()
	if err != nil {
		return err
	}
	if !inStrSlice(locksIndexlist, "owner") {
		Log.Println("Creating owner index on locks table")
		_, err := r.Table("locks").IndexCreateFunc("owner", func(row r.Term) interface{} {
			return row.Field("owner")
		}).RunWrite(db)
		if err != nil {
			return err
		}
	}
	return nil
}

func dbClean(prefix string) (err error) {
	// Delete all tasks from this prefix
	wres, err := r.Table("tasks").
		Between(prefix, prefix+"\uffff").
		Filter(r.Row.Field("detach").Not()).
		Delete(r.DeleteOpts{ReturnChanges: true}).
		RunWrite(db, r.RunOpts{Durability: "soft"})
	if err != nil {
		return
	}
	for _, change := range wres.Changes {
		task := ei.N(change.OldValue)
		if path := task.M("path").StringZ(); !strings.HasPrefix(path, "@pull.") {
			hook("task", path+task.M("method").StringZ(), task.M("user").StringZ(), ei.M{
				"action":    "pusherDisconnect",
				"id":        task.M("id").StringZ(),
				"timestamp": time.Now().UTC(),
			})
		}
	}

	// Recover all tasks whose target session is this prefix
	wres, err = r.Table("tasks").
		Between(prefix, prefix+"\uffff", r.BetweenOpts{Index: "tses"}).
		Update(r.Branch(r.Row.Field("stat").Eq("working"),
			map[string]interface{}{"stat": "waiting", "tses": nil, "ttl": r.Row.Field("ttl").Add(-1)},
			map[string]interface{}{}),
			r.UpdateOpts{ReturnChanges: true}).
		RunWrite(db, r.RunOpts{Durability: "soft"})
	if err != nil {
		return
	}
	for _, change := range wres.Changes {
		task := ei.N(change.OldValue)
		hook("task", task.M("path").StringZ()+task.M("method").StringZ(), task.M("user").StringZ(), ei.M{
			"action":    "pullerDisconnect",
			"id":        task.M("id").StringZ(),
			"timestamp": time.Now().UTC(),
		})
	}

	// Delete all pipes from this prefix
	_, err = r.Table("pipes").
		Between(prefix, prefix+"\uffff").
		Delete().
		RunWrite(db, r.RunOpts{Durability: "soft"})
	if err != nil {
		return
	}

	// Delete all locks from this prefix
	_, err = r.Table("locks").
		Between(prefix, prefix+"\uffff", r.BetweenOpts{Index: "owner"}).
		Delete().
		RunWrite(db, r.RunOpts{Durability: "soft"})
	if err != nil {
		return
	}

	// Delete all sessions from this node
	_, err = r.Table("sessions").
		Between(prefix, prefix+"\uffff").
		Delete().
		RunWrite(db, r.RunOpts{Durability: "soft"})

	return
}
