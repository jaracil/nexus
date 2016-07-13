package test

import (
	"strings"
	"testing"
	"time"

	nxcli "github.com/jaracil/nxcli"
	nexus "github.com/jaracil/nxcli/nxcore"
)

// TestPing does a ping and waits for a pong
func TestPing(t *testing.T) {
	// Bootrap
	if err := bootstrap(t); err != nil {
		t.Fatal(err)
	}

	// Login success
	conn, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("login: %s", err.Error())
	}
	defer conn.Close()

	// Ping success
	err = conn.Ping(time.Second * 10)
	if err != nil {
		t.Fatalf("ping: %s", err.Error())
	}
	
	// Unbootstrap
	if err := unbootstrap(t); err != nil {
		t.Fatal(err)
	}
}

// TestLogin
func TestLogin(t *testing.T) {
	// Bootrap
	if err := bootstrap(t); err != nil {
		t.Fatal(err)
	}

	// New conn
	conn, err := nxcli.Dial(NexusServer, nxcli.NewDialOptions())
	if err != nil {
		t.Errorf("dial: %s", err.Error())
		return
	}
	defer conn.Close()

	// Ping success
	err = conn.Ping(time.Second * 10)
	if err != nil {
		t.Errorf("ping: %s", err.Error())
	}

	// Login fail
	_, err = conn.Login("", "")
	if !IsNexusErrCode(err, nexus.ErrPermissionDenied) {
		t.Errorf("login: expecting ErrPermissionDenied")
	}
	_, err = conn.Login(UserA, "abcd")
	if !IsNexusErrCode(err, nexus.ErrPermissionDenied) {
		t.Errorf("login: expecting ErrPermissionDenied")
	}

	// Relogin
	_, err = conn.Login(UserA, UserA)
	if err != nil {
		t.Errorf("login: %s", err.Error())
	}
	_, err = conn.Login(UserB, UserB)
	if err == nil {
		t.Errorf("relogin: expecting error")
	}

	// Login strings
	_, err = conn.Login(strings.ToUpper(UserA), UserA)
	if err != nil {
		t.Errorf("login upper: %s", err.Error())
	}
	_, err = conn.Login(" "+UserA, UserA)
	if !IsNexusErrCode(err, nexus.ErrPermissionDenied) {
		t.Errorf("login with prefix space: expecting ErrPermissionDenied", err.Error())
	}
	
	// Unbootstrap
	if err := unbootstrap(t); err != nil {
		t.Fatal(err)
	}
}
