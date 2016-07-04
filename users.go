package main

import (
	r "github.com/dancannon/gorethink"
	"github.com/jaracil/ei"
)

type UserData struct {
	User string                            `gorethink:"id,omitempty"`
	Pass string                            `gorethink:"pass,omitempty"`
	Salt string                            `gorethink:"salt,omitempty"`
	Tags map[string]map[string]interface{} `gorethink:"tags,omitempty"`
}

var Nobody *UserData = &UserData{User: "nobody", Tags: map[string]map[string]interface{}{}}

func (nc *NexusConn) handleUserReq(req *JsonRpcReq) {
	switch req.Method {
	case "user.create":
		user, err := ei.N(req.Params).M("user").String()
		if err != nil {
			req.Error(ErrInvalidParams, "user", nil)
			return
		}
		pass, err := ei.N(req.Params).M("pass").String()
		if err != nil {
			req.Error(ErrInvalidParams, "pass", nil)
			return
		}
		tags := nc.getTags(user)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		ud := UserData{User: user, Salt: safeId(16), Tags: map[string]map[string]interface{}{}}
		ud.Pass, err = HashPass(pass, ud.Salt)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		_, err = r.Table("users").Insert(&ud).RunWrite(db)
		if err != nil {
			if r.IsConflictErr(err) {
				req.Error(ErrUserExists, "", nil)
			} else {
				req.Error(ErrInternal, "", nil)
			}
			return
		}
		req.Result(map[string]interface{}{"ok": true})
	case "user.delete":
		user, err := ei.N(req.Params).M("user").String()
		if err != nil {
			req.Error(ErrInvalidParams, "user", nil)
			return
		}
		tags := nc.getTags(user)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		res, err := r.Table("users").Get(user).Delete().RunWrite(db)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if res.Deleted > 0 {
			req.Result(map[string]interface{}{"ok": true})
		} else {
			req.Error(ErrInvalidUser, "", nil)
		}

	case "user.setTags":
		user, err := ei.N(req.Params).M("user").String()
		if err != nil {
			req.Error(ErrInvalidParams, "user", nil)
			return
		}
		prefix, err := ei.N(req.Params).M("prefix").String()
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
		res, err := r.Table("users").Get(user).Update(map[string]interface{}{"tags": map[string]interface{}{prefix: tgs}}).RunWrite(db)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if res.Unchanged == 0 && res.Replaced == 0 {
			req.Error(ErrInvalidUser, "", nil)
			return
		}
		req.Result(map[string]interface{}{"ok": true})
	case "user.delTags":
		user, err := ei.N(req.Params).M("user").String()
		if err != nil {
			req.Error(ErrInvalidParams, "user", nil)
			return
		}
		prefix, err := ei.N(req.Params).M("prefix").String()
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
		res, err := r.Table("users").Get(user).Update(map[string]interface{}{"tags": map[string]interface{}{prefix: r.Literal(r.Row.Field("tags").Field(prefix).Without(tgs))}}).RunWrite(db)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if res.Unchanged == 0 && res.Replaced == 0 {
			req.Error(ErrInvalidUser, "", nil)
			return
		}
		req.Result(map[string]interface{}{"ok": true})
	case "user.setPass":
		user, err := ei.N(req.Params).M("user").String()
		if err != nil {
			req.Error(ErrInvalidParams, "user", nil)
			return
		}
		pass, err := ei.N(req.Params).M("pass").String()
		if err != nil {
			req.Error(ErrInvalidParams, "pass", nil)
			return
		}
		tags := nc.getTags(user)
		if !(ei.N(tags).M("@"+req.Method).BoolZ() || ei.N(tags).M("@admin").BoolZ()) {
			req.Error(ErrPermissionDenied, "", nil)
			return
		}
		salt := safeId(16)
		hp, err := HashPass(pass, salt)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		res, err := r.Table("users").Get(user).Update(map[string]interface{}{"salt": salt, "pass": hp}).RunWrite(db)
		if err != nil {
			req.Error(ErrInternal, "", nil)
			return
		}
		if res.Unchanged == 0 && res.Replaced == 0 {
			req.Error(ErrInvalidUser, "", nil)
			return
		}
		req.Result(map[string]interface{}{"ok": true})
	default:
		req.Error(ErrMethodNotFound, "", nil)
	}
}
