package test

import (
	"strings"
	"testing"
	"time"

	nxcli "github.com/jaracil/nxcli"
	nexus "github.com/jaracil/nxcli/nxcore"
)

func TestPing(t *testing.T) {
	conn, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("login: %s", err.Error())
	}
	defer conn.Close()
	err = conn.Ping(time.Second * 10)
	if err != nil {
		t.Fatalf("ping: %s", err.Error())
	}
}

func TestLoginFail(t *testing.T) {
	conn, err := nxcli.Dial(NexusServer, nxcli.NewDialOptions())
	if err != nil {
		t.Errorf("dial: %s", err.Error())
		return
	}
	defer conn.Close()
	_, err = conn.Login("", "")
	if !IsNexusErrCode(err, nexus.ErrPermissionDenied) {
		t.Errorf("login: expecting ErrPermissionDenied")
	}
	_, err = conn.Login(UserA, "abcd")
	if !IsNexusErrCode(err, nexus.ErrPermissionDenied) {
		t.Errorf("login: expecting ErrPermissionDenied")
	}
}

func TestRelogin(t *testing.T) {
	conn, err := nxcli.Dial(NexusServer, nxcli.NewDialOptions())
	if err != nil {
		t.Errorf("dial: %s", err.Error())
		return
	}
	defer conn.Close()
	_, err = conn.Login(UserA, UserA)
	if err != nil {
		t.Errorf("login: %s", err.Error())
	}
	_, err = conn.Login(UserB, UserB)
	if err == nil {
		t.Logf("relogin: expecting error")
	}
}

func TestLoginWrongStrings(t *testing.T) {
	conn, err := nxcli.Dial(NexusServer, nxcli.NewDialOptions())
	if err != nil {
		t.Errorf("dial: %s", err.Error())
		return
	}
	_, err = conn.Login(strings.ToUpper(UserA), UserA)
	if err != nil {
		t.Errorf("login upper: %s", err.Error())
	}
	_, err = conn.Login(" "+UserA, UserA)
	if !IsNexusErrCode(err, nexus.ErrPermissionDenied) {
		t.Errorf("login with prefix space: expecting ErrPermissionDenied", err.Error())
	}
}