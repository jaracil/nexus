package main

import (
	r "github.com/dancannon/gorethink"
	"github.com/jaracil/ei"
)

var db *r.Session

func dbOpen() (err error) {
	db, err = r.Connect(r.ConnectOpts{
		Address:  opts.Rethink.Host,
		Database: opts.Rethink.Database,
		MaxIdle:  opts.Rethink.MaxIdle,
		MaxOpen:  opts.Rethink.MaxOpen,
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
		sysln("Creating tasks table")
		_, err := r.TableCreate("tasks").RunWrite(db)
		if err != nil {
			return err
		}
	}
	if !inStrSlice(tablelist, "pipes") {
		sysln("Creating pipes table")
		_, err := r.TableCreate("pipes").RunWrite(db)
		if err != nil {
			return err
		}

	}
	if !inStrSlice(tablelist, "users") {
		sysln("Creating users table")
		_, err := r.TableCreate("users").RunWrite(db)
		if err != nil {
			return err
		}
		sysln("Creating root user")
		ud := UserData{User: "root", Salt: safeId(16), Tags: map[string]map[string]interface{}{".": {"@admin": true}}}
		ud.Pass, err = HashPass("root", ud.Salt)
		_, err = r.Table("users").Insert(&ud).RunWrite(db)
		if err != nil {
			return err
		}

	}
	if !inStrSlice(tablelist, "nodes") {
		sysln("Creating nodes table")
		_, err := r.TableCreate("nodes").RunWrite(db)
		if err != nil {
			return err
		}
	}
	if !inStrSlice(tablelist, "locks") {
		sysln("Creating locks table")
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
		sysln("Creating subs index on pipes table")
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
		sysln("Creating pspc index on tasks table")
		_, err := r.Table("tasks").IndexCreateFunc("pspc", func(row r.Term) interface{} {
			return ei.S{row.Field("path"), row.Field("stat"), row.Field("prio"), row.Field("creationTime")}
		}).RunWrite(db)
		if err != nil {
			return err
		}
	}
	if !inStrSlice(tasksIndexlist, "deadLine") {
		sysln("Creating deadLine index on tasks table")
		_, err := r.Table("tasks").IndexCreateFunc("deadLine", func(row r.Term) interface{} {
			return row.Field("deadLine")
		}).RunWrite(db)
		if err != nil {
			return err
		}
	}
	if !inStrSlice(tasksIndexlist, "tses") {
		sysln("Creating tses index on tasks table")
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
		sysln("Creating owner index on locks table")
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
	_, err = r.Table("tasks").
		Between(prefix, prefix+"\uffff").
		Filter(r.Row.Field("detach").Not()).
		Delete().
		RunWrite(db, r.RunOpts{Durability: "soft"})
	if err != nil {
		return
	}
	// Recover all tasks whose target session is this prefix
	_, err = r.Table("tasks").
		Between(prefix, prefix+"\uffff", r.BetweenOpts{Index: "tses"}).
		Update(r.Branch(r.Row.Field("stat").Eq("working"),
			map[string]interface{}{"stat": "waiting", "tses": nil, "ttl": r.Row.Field("ttl").Add(-1)},
			map[string]interface{}{}),
			r.UpdateOpts{ReturnChanges: false}).
		RunWrite(db, r.RunOpts{Durability: "soft"})
	if err != nil {
		return
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
	return
}
