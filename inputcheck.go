package main

import (
	"regexp"
	"github.com/jaracil/ei"
	"errors"
)

const (
	_userRegexp = "^[a-zA-Z][a-zA-Z0-9-_.]*"
	_userMinLen = 3
	_userMaxLen = 500
	_passwordMinLen = 4
	_passwordMaxLen = 500
)

func checkRegexp(i ei.Ei, p ...interface{}) ei.Ei {
	s, err := i.String()
	if err != nil {
		return ei.N(err)
	}
	if len(p) < 1 {
		return ei.N(errors.New("regexp not provided"))
	}
	if match, err := regexp.MatchString(ei.N(p[0]).StringZ(), s); err != nil || !match {
		return ei.N(errors.New("regexp check failed"))
	}
	return i
}

func checkLen(i ei.Ei, p ...interface{}) ei.Ei {
	s, err := i.String()
	if err != nil {
		return ei.N(err)
	}
	if len(p) < 2 {
		return ei.N(errors.New("minlen and maxlen not provided"))
	}
	if minlen := ei.N(p[0]).IntZ(); minlen > 0 && len(s) < minlen {
		return ei.N(errors.New("minlen exceded"))
	}
	if maxlen := ei.N(p[1]).IntZ(); maxlen > 0 && len(s) > maxlen {
		return ei.N(errors.New("maxlen exceded"))
	}
	return i
}

