package test

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	nxcli "github.com/jaracil/nxcli"
	nexus "github.com/jaracil/nxcli/nxcore"
)

var NexusServer = "localhost:1717"
var RootSes *nexus.NexusConn

var UserA = "testa"
var UserB = "testb"
var UserC = "testc"
var UserD = "testd"

var Prefix1 = "prefix1"
var Prefix2 = "prefix2"
var Prefix3 = "prefix3"
var Prefix4 = "prefix4"

var Suffix = ""

func TestMain(m *testing.M) {
	// Nexus server
	if ns := os.Getenv("NEXUS_SERVER"); ns != "" {
		NexusServer = ns
	}

	// Suffix
	rand.Seed(time.Now().UnixNano())
	Suffix = fmt.Sprintf("%04d", rand.Intn(9999))

	// Bootrap
	if err := bootstrap(); err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}

	// Run
	code := m.Run()

	// Unboostrap
	if err := unbootstrap(); err != nil {
		fmt.Print(err.Error())
	}

	// Exit
	os.Exit(code)
}

func bootstrap() error {
	var err error

	// Login root session
	RootSes, err = login("root", "root")
	if err != nil {
		return fmt.Errorf("bootstrap: logging in as root: %s", err.Error())
	}

	// Create users and set tags
	for _, u := range []string{UserA, UserB, UserC, UserD} {
		_, err = RootSes.UserCreate(u, u)
		if err != nil {
			return fmt.Errorf("bootstrap: creating %s user: %s", u, err.Error())
		}
		_, err = RootSes.UserSetTags(u, ".", map[string]interface{}{"@admin": true})
		if err != nil {
			return fmt.Errorf("bootstrap: setting admin tag to %s user: %s", u, err.Error())
		}
		_, err = RootSes.UserSetTags(u, Prefix1, map[string]interface{}{"@admin": true})
		if err != nil {
			return fmt.Errorf("bootstrap: setting admin tag to %s user: %s", u, err.Error())
		}
		_, err = RootSes.UserSetTags(u, Prefix2, map[string]interface{}{"@admin": true})
		if err != nil {
			return fmt.Errorf("bootstrap: setting admin tag to %s user: %s", u, err.Error())
		}
		_, err = RootSes.UserSetTags(u, Prefix3, map[string]interface{}{"@admin": true})
		if err != nil {
			return fmt.Errorf("bootstrap: setting admin tag to %s user: %s", u, err.Error())
		}
		_, err = RootSes.UserSetTags(u, Prefix4, map[string]interface{}{"@admin": true})
		if err != nil {
			return fmt.Errorf("bootstrap: setting admin tag to %s user: %s", u, err.Error())
		}
		//t.Logf("Created user %s\n", u)
	}
	return nil
}

func unbootstrap() error {
	// Delete users
	for _, u := range []string{UserA, UserB, UserC, UserD} {
		_, err := RootSes.UserDelete(u)
		if err != nil {
			return fmt.Errorf("Unbootstrap: deleting %s user: %s", u, err.Error())
		}
		//t.Logf("Deleted user %s\n", u)
	}

	// Stop root session
	RootSes.Close()
	return nil
}

func login(u string, p string) (*nexus.NexusConn, error) {
	ses, err := nxcli.Dial(NexusServer, nxcli.NewDialOptions())
	if err != nil {
		return nil, err
	}
	_, err = ses.Login(u, p)
	if err != nil {
		return nil, err
	}
	return ses, nil
}

func IsNexusErr(err error) bool {
	_, ok := err.(*nexus.JsonRpcErr)
	return ok
}

func IsNexusErrCode(err error, code int) bool {
	if nexusErr, ok := err.(*nexus.JsonRpcErr); ok {
		return nexusErr.Cod == code
	}
	return false
}
