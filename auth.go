package main

import (
	"log"
	"strings"

	r "github.com/dancannon/gorethink"
	"github.com/jaracil/ei"
)

func loadUserData(user string) (*UserData, int) {
	ud := &UserData{}
	cur, err := r.Table("users").Get(strings.ToLower(user)).Run(db)
	if err != nil {
		return nil, ErrInternal
	}
	defer cur.Close()
	err = cur.One(ud)
	if err != nil {
		if err == r.ErrEmptyResult {
			return nil, ErrPermissionDenied
		}
		return nil, ErrInternal
	}
	return ud, ErrNoError
}

func (nc *NexusConn) BasicAuth(params interface{}) (string, map[string]map[string]interface{}, int) {
	user, err := ei.N(params).M("user").String()
	if err != nil {
		return "", nil, ErrInvalidParams
	}
	pass, err := ei.N(params).M("pass").String()
	if err != nil {
		return "", nil, ErrInvalidParams
	}
	var suser string
	split := strings.Split(user, ">")
	switch len(split) {
	case 1:
	case 2:
		if len(split[0]) > 0 && len(split[1]) > 0 {
			user = split[0]
			suser = split[1]
		} else {
			return "", nil, ErrInvalidParams

		}
	default:
		return "", nil, ErrInvalidParams
	}

	ud, rerr := loadUserData(user)
	if rerr != ErrNoError {
		return "", nil, rerr
	}
	dk, err := HashPass(pass, ud.Salt)
	if err != nil {
		return "", nil, ErrInternal
	}
	if ud.Pass != dk {
		return "", nil, ErrPermissionDenied
	}

	if suser != "" {
		tags := nc.getTags(suser)
		if !(ei.N(tags).M("@admin").BoolZ()) {
			return "", nil, ErrPermissionDenied
		}
		sud, err := loadUserData(user)
		if err != ErrNoError {
			return "", nil, ErrPermissionDenied
		}
		return sud.User, sud.Tags, ErrNoError

	}

	return ud.User, ud.Tags, ErrNoError
}

func (nc *NexusConn) TokenAuth(params interface{}) (string, map[string]map[string]interface{}, int) {
	token, err := ei.N(params).M("token").String()
	if err != nil {
		return "", nil, ErrInvalidParams
	}

	log.Printf("Checking if token [%s] is valid...", token)

	return "", nil, ErrPermissionDenied
}
