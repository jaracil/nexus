package main

import (
	"strings"
	"time"

	"github.com/jaracil/ei"
	r "gopkg.in/gorethink/gorethink.v5"
)

type UserData struct {
	User      string                            `gorethink:"id,omitempty"`
	Pass      string                            `gorethink:"pass,omitempty"`
	Salt      string                            `gorethink:"salt,omitempty"`
	Tags      map[string]map[string]interface{} `gorethink:"tags,omitempty"`
	Templates []string                          `gorethink:"templates,omitempty"`
	CreatedAt time.Time                         `gorethink:"createdAt,omitempty"`

	// Limits
	Mask        map[string]map[string]interface{} `gorethink:"mask,omitempty"`
	MaxSessions int                               `gorethink:"maxsessions,omitempty"`
	Whitelist   []string                          `gorethink:"whitelist,omitempty"`
	Blacklist   []string                          `gorethink:"blacklist,omitempty"`
	Disabled    bool                              `gorethink:"disabled,omitempty"`
}

var Nobody *UserData = &UserData{User: "nobody", Tags: map[string]map[string]interface{}{}, MaxSessions: 100000}

const DEFAULT_MAX_SESSIONS = 50

func (nc *NexusConn) handleUserReq(req *JsonRpcReq) {
	switch req.Method {
	case "user.create":
		user, err := ei.N(req.Params).M("user").Lower().F(checkRegexp, _prefixRegexp).F(checkNotEmptyLabels).F(checkLen, _userMinLen, _userMaxLen).String()
		if err != nil {
			req.Error(ErrInvalidParams, "user", nil)
			return
		}
		pass, err := ei.N(req.Params).M("pass").F(checkLen, _passwordMinLen, _passwordMaxLen).String()
		if err != nil {
			req.Error(ErrInvalidParams, "pass", nil)
			return
		}
		tags := nc.getTags(user)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		ud := UserData{User: user, Salt: safeId(16), Tags: map[string]map[string]interface{}{}, Templates: []string{}, MaxSessions: DEFAULT_MAX_SESSIONS, Disabled: false, CreatedAt: time.Now()}
		ud.Pass, err = HashPass(pass, ud.Salt)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		_, err = r.Table("users").Insert(&ud).RunWrite(db, r.RunOpts{Durability: "hard"})
		if err != nil {
			if r.IsConflictErr(err) {
				req.Error(ErrUserExists, "", nil)
			} else {
				req.Error(ErrInternal, "", nil)
			}
			return
		}
		hook("user", user, nc.user.User, ei.M{
			"action": "create",
			"user":   user,
			"pass":   pass,
		})
		req.Result(map[string]interface{}{"ok": true})

	case "user.delete":
		user, err := ei.N(req.Params).M("user").Lower().String()
		if err != nil {
			req.Error(ErrInvalidParams, "user", nil)
			return
		}
		tags := nc.getTags(user)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		res, err := r.Table("users").Get(user).Delete().RunWrite(db, r.RunOpts{Durability: "hard"})
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if res.Deleted > 0 {
			hook("user", user, nc.user.User, ei.M{
				"action": "delete",
				"user":   user,
			})
			req.Result(map[string]interface{}{"ok": true})
		} else {
			req.Error(ErrInvalidUser, "", nil)
		}

	case "user.rename":
		user, err := ei.N(req.Params).M("user").Lower().String()
		if err != nil {
			req.Error(ErrInvalidParams, "user", nil)
			return
		}
		newUser, err := ei.N(req.Params).M("new").Lower().F(checkRegexp, _prefixRegexp).F(checkNotEmptyLabels).F(checkLen, _userMinLen, _userMaxLen).String()
		if err != nil {
			req.Error(ErrInvalidParams, "new", nil)
			return
		}
		oldUserTags := nc.getTags(user)
		if !(ei.N(oldUserTags).M("@"+req.Method).BoolZ() || ei.N(oldUserTags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		newUserTags := nc.getTags(newUser)
		if !(ei.N(newUserTags).M("@"+req.Method).BoolZ() || ei.N(newUserTags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		_, err = r.Table("users").Insert(map[string]interface{}{"id": newUser, "blockedBy": req.nc.connId, "renaming": "me"}).RunWrite(db, r.RunOpts{Durability: "hard"})
		if err != nil {
			if r.IsConflictErr(err) {
				req.Error(ErrUserExists, "", nil)
			} else {
				req.Error(ErrInternal, "", nil)
			}
			return
		}

		res, err := r.Table("users").Get(user).Update(map[string]interface{}{"blockedBy": req.nc.connId, "renaming": newUser}, r.UpdateOpts{ReturnChanges: true}).RunWrite(db, r.RunOpts{Durability: "hard"})
		if err != nil {
			r.Table("users").Get(newUser).Delete().RunWrite(db)
			req.Error(ErrInternal, "", nil)
			return
		}
		if res.Unchanged == 0 && res.Replaced == 0 {
			r.Table("users").Get(newUser).Delete().RunWrite(db)
			req.Error(ErrInvalidUser, "", nil)
			return
		}
		newUserData := ei.N(res.Changes[0].OldValue).MapStrZ()
		newUserData["id"] = newUser

		res, err = r.Table("users").Get(newUser).Replace(newUserData).RunWrite(db, r.RunOpts{Durability: "hard"})
		if err != nil || (res.Unchanged == 0 && res.Replaced == 0) {
			r.Table("users").Get(newUser).Delete().RunWrite(db)
			r.Table("users").Get(user).Replace(func(t r.Term) r.Term { return t.Without("blockedBy", "renaming") })
			req.Error(ErrInternal, "", nil)
			return
		}

		res, err = r.Table("users").Get(user).Delete().RunWrite(db, r.RunOpts{Durability: "hard"})
		if err != nil || res.Deleted == 0 {
			r.Table("users").Get(newUser).Delete().RunWrite(db)
			r.Table("users").Get(user).Replace(func(t r.Term) r.Term { return t.Without("blockedBy", "renaming") })
			req.Error(ErrInternal, "", nil)
			return
		}

		req.Result(map[string]interface{}{"ok": true})

	case "user.setTags":
		user, err := ei.N(req.Params).M("user").Lower().String()
		if err != nil {
			req.Error(ErrInvalidParams, "user", nil)
			return
		}
		prefix, err := ei.N(req.Params).M("prefix").Lower().String()
		if err != nil {
			req.Error(ErrInvalidParams, "prefix", nil)
			return
		}
		tgs, err := ei.N(req.Params).M("tags").MapStr()
		if err != nil {
			req.Error(ErrInvalidParams, "tags", nil)
			return
		}
		tags := nc.getTags(prefix)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		res, err := r.Table("users").Get(user).Update(map[string]interface{}{"tags": map[string]interface{}{prefix: tgs}}, r.UpdateOpts{ReturnChanges: true}).RunWrite(db, r.RunOpts{Durability: "hard"})
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if res.Unchanged == 0 && res.Replaced == 0 {
			req.Error(ErrInvalidUser, "", nil)
			return
		}
		if res.Replaced > 0 {
			hook("user", user, nc.user.User, ei.M{
				"action":  "setTags",
				"user":    user,
				"prefix":  prefix,
				"addTags": tgs,
				"tags":    ei.N(res.Changes[0].NewValue).M("tags").MapStrZ(),
			})
		}
		req.Result(map[string]interface{}{"ok": true})

	case "user.delTags":
		user, err := ei.N(req.Params).M("user").Lower().String()
		if err != nil {
			req.Error(ErrInvalidParams, "user", nil)
			return
		}
		prefix, err := ei.N(req.Params).M("prefix").Lower().String()
		if err != nil {
			req.Error(ErrInvalidParams, "prefix", nil)
			return
		}
		tgs, err := ei.N(req.Params).M("tags").Slice()
		if err != nil {
			req.Error(ErrInvalidParams, "tags", nil)
			return
		}
		tags := nc.getTags(prefix)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		res, err := r.Table("users").Get(user).Replace(func(source r.Term) r.Term {
			return r.Branch(
				source.HasFields("tags"),
				r.Branch(
					source.Field("tags").HasFields(prefix),
					r.Branch(
						source.Field("tags").Field(prefix).Without(tgs).Count().Ne(0),
						source.Merge(ei.M{"tags": ei.M{prefix: r.Literal(source.Field("tags").Field(prefix).Without(tgs))}}),
						source.Merge(ei.M{"tags": r.Literal(source.Field("tags").Without(prefix))}),
					),
					source.Merge(ei.M{}),
				),
				source.Merge(ei.M{"tags": r.Literal(ei.M{})}),
			)
		}, r.ReplaceOpts{ReturnChanges: true}).RunWrite(db, r.RunOpts{Durability: "hard"})
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if res.Unchanged == 0 && res.Replaced == 0 {
			req.Error(ErrInvalidUser, "", nil)
			return
		}
		if res.Replaced > 0 {
			hook("user", user, nc.user.User, ei.M{
				"action":  "delTags",
				"user":    user,
				"prefix":  prefix,
				"delTags": tgs,
				"tags":    ei.N(res.Changes[0].NewValue).M("tags").MapStrZ(),
			})
		}
		req.Result(map[string]interface{}{"ok": true})

	case "user.getTags":
		user, err := ei.N(req.Params).M("user").Lower().String()
		if err != nil {
			req.Error(ErrInvalidParams, "user", nil)
			return
		}

		tags := nc.getTags(user)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ() || user == nc.user.User) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		ud, errn := loadUserData(user)
		if errn != ErrNoError {
			req.Error(ErrInvalidParams, "", nil)
			return
		}

		req.Result(ei.M{"tags": ud.Tags})

	case "user.getEffectiveTags":
		user, err := ei.N(req.Params).M("user").Lower().String()
		if err != nil {
			req.Error(ErrInvalidParams, "user", nil)
			return
		}
		prefix, err := ei.N(req.Params).M("prefix").Lower().String()
		if err != nil {
			req.Error(ErrInvalidParams, "prefix", nil)
			return
		}

		tags := nc.getTags(user)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ() || user == nc.user.User) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		ud, errn := loadUserData(user)
		if errn != ErrNoError {
			req.Error(ErrInvalidParams, "", nil)
			return
		}

		req.Result(ei.M{"tags": getTags(ud, prefix)})

	case "user.setPass":
		user, err := ei.N(req.Params).M("user").Lower().String()
		if err != nil {
			req.Error(ErrInvalidParams, "user", nil)
			return
		}
		pass, err := ei.N(req.Params).M("pass").F(checkLen, _passwordMinLen, _passwordMaxLen).String()
		if err != nil {
			req.Error(ErrInvalidParams, "pass", nil)
			return
		}
		tags := nc.getTags(user)

		if !ei.N(tags).M("@admin").BoolZ() {
			if user == nc.user.User {
				if e, err := ei.N(tags).M("@" + req.Method).Bool(); err == nil && !e {
					req.Error(ErrPermissionDenied, "", nil)
					return
				}
			} else if !ei.N(tags).M("@" + req.Method).BoolZ() {
				req.Error(ErrPermissionDenied, "", nil)
				return
			}
		}

		salt := safeId(16)
		hp, err := HashPass(pass, salt)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		res, err := r.Table("users").Get(user).Update(map[string]interface{}{"salt": salt, "pass": hp}).RunWrite(db, r.RunOpts{Durability: "hard"})
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if res.Unchanged == 0 && res.Replaced == 0 {
			req.Error(ErrInvalidUser, "", nil)
			return
		}
		if res.Replaced > 0 {
			hook("user", user, nc.user.User, ei.M{
				"action": "setPass",
				"user":   user,
				"pass":   pass,
			})
		}
		req.Result(map[string]interface{}{"ok": true})

	case "user.list":
		prefix, depth, filter, limit, skip := getListParams(req.Params)

		tags := nc.getTags(prefix)
		if !(ei.N(tags).M("@user.list").BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		term := getListTerm("users", "", "id", prefix, depth, filter, limit, skip).
			Pluck("id", "tags", "templates", "whitelist", "blacklist", "maxsessions", "disabled", "createdAt")

		cur, err := term.Map(func(row r.Term) interface{} {
			return ei.M{
				"user":        row.Field("id"),
				"tags":        row.Field("tags").Default(ei.M{}),
				"templates":   row.Field("templates").Default(ei.S{}),
				"whitelist":   row.Field("whitelist").Default(ei.S{}),
				"blacklist":   row.Field("blacklist").Default(ei.S{}),
				"maxsessions": row.Field("maxsessions").Default(DEFAULT_MAX_SESSIONS),
				"disabled":    row.Field("disabled").Default(false),
				"createdAt":   row.Field("createdAt").Default(time.Time{}),
			}
		}).Run(db)
		if err != nil {
			req.Error(ErrInternal, err.Error(), nil)
			return
		}
		var all []interface{}
		if err := cur.All(&all); err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		req.Result(all)

	case "user.count":
		prefix := getPrefixParam(req.Params)
		filter := ei.N(req.Params).M("filter").StringZ()
		countSubprefixes := ei.N(req.Params).M("subprefixes").BoolZ()

		tags := nc.getTags(prefix)
		if !(ei.N(tags).M("@user.count").BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}

		term := getCountTerm("users", "", "id", prefix, filter, countSubprefixes)
		cur, err := term.Run(db)
		if err != nil {
			req.Error(ErrInternal, err.Error(), nil)
			return
		}
		var all []interface{}
		if err := cur.All(&all); err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if countSubprefixes {
			req.Result(all)
		} else if len(all) > 0 {
			req.Result(ei.M{"count": all[0]})
		} else {
			req.Result(ei.M{"count": 0})
		}

	case "user.addTemplate":
		param, err := ei.N(req.Params).M("template").String()
		if err != nil {
			req.Error(ErrInvalidParams, "template", nil)
			return
		}
		nc.userAddParam(req, param, "templates")

	case "user.delTemplate":
		param, err := ei.N(req.Params).M("template").String()
		if err != nil {
			req.Error(ErrInvalidParams, "template", nil)
			return
		}
		nc.userDelParam(req, param, "templates")

	case "user.addWhitelist":
		param, err := ei.N(req.Params).M("ip").String()
		if err != nil {
			req.Error(ErrInvalidParams, "ip", nil)
			return
		}
		nc.userAddParam(req, param, "whitelist")

	case "user.delWhitelist":
		param, err := ei.N(req.Params).M("ip").String()
		if err != nil {
			req.Error(ErrInvalidParams, "ip", nil)
			return
		}
		nc.userDelParam(req, param, "whitelist")

	case "user.addBlacklist":
		param, err := ei.N(req.Params).M("ip").String()
		if err != nil {
			req.Error(ErrInvalidParams, "ip", nil)
			return
		}
		nc.userAddParam(req, param, "blacklist")

	case "user.delBlacklist":
		param, err := ei.N(req.Params).M("ip").String()
		if err != nil {
			req.Error(ErrInvalidParams, "ip", nil)
			return
		}
		nc.userDelParam(req, param, "blacklist")

	case "user.setMaxSessions":
		param, err := ei.N(req.Params).M("maxsessions").Int()
		if err != nil {
			req.Error(ErrInvalidParams, "maxsessions", nil)
			return
		}
		nc.userSetParam(req, param, "maxsessions")

	case "user.setDisabled":
		param, err := ei.N(req.Params).M("disabled").Bool()
		if err != nil {
			req.Error(ErrInvalidParams, "disabled", nil)
			return
		}
		nc.userSetParam(req, param, "disabled")

	default:
		req.Error(ErrMethodNotFound, "", nil)
	}
}

func (nc *NexusConn) userAddParam(req *JsonRpcReq, param interface{}, field string) {
	nc.userChangeParam(req, param, field, "add")
}

func (nc *NexusConn) userDelParam(req *JsonRpcReq, param interface{}, field string) {
	nc.userChangeParam(req, param, field, "del")
}

func (nc *NexusConn) userSetParam(req *JsonRpcReq, param interface{}, field string) {
	nc.userChangeParam(req, param, field, "set")
}

func (nc *NexusConn) userChangeParam(req *JsonRpcReq, param interface{}, field, action string) {
	user, err := ei.N(req.Params).M("user").Lower().String()
	if err != nil {
		req.Error(ErrInvalidParams, "user", nil)
		return
	}

	userTags := ei.N(nc.getTags(user))
	if !(userTags.M("@"+req.Method).BoolZ() || userTags.M("@admin").BoolZ()) {
		req.Error(ErrPermissionDenied, "", nil)
		return
	}
	if field == "templates" {
		templateTags := ei.N(nc.getTags(ei.N(param).StringZ()))
		if !(templateTags.M("@user.useTemplate").BoolZ() || templateTags.M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
	}

	term := r.Table("users").Get(user)
	switch action {
	case "add":
		term = term.Update(map[string]interface{}{
			field: r.Row.Field(field).Default(ei.S{}).SetInsert(param),
		}, r.UpdateOpts{ReturnChanges: true})
	case "del":
		term = term.Update(map[string]interface{}{
			field: r.Row.Field(field).Default(ei.S{}).SetDifference([]interface{}{param}),
		}, r.UpdateOpts{ReturnChanges: true})
	case "set":
		term = term.Update(map[string]interface{}{field: param}, r.UpdateOpts{ReturnChanges: true})
	}
	res, err := term.RunWrite(db, r.RunOpts{Durability: "hard"})
	if err != nil {
		req.Error(ErrInternal, "", nil)
		return
	}
	if res.Unchanged == 0 && res.Replaced == 0 {
		req.Error(ErrInvalidUser, "", nil)
		return
	}
	if res.Replaced > 0 {
		hook("user", user, nc.user.User, ei.M{
			"action": strings.TrimPrefix(req.Method, "user."),
			action:   param,
			field:    ei.N(res.Changes[0].NewValue).M(field),
		})
	}
	req.Result(map[string]interface{}{"ok": true})
}
