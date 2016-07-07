package test

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	nxcli "github.com/jaracil/nxcli"
	nexus "github.com/jaracil/nxcli/nxcore"
)

var RootSes *nexus.NexusConn

var NexusServer = "localhost:1717"
var BootstrapErr error = nil
var Suffix = ""

var UserA = "testa"
var UserB = "testb"
var UserC = "testc"
var UserD = "testd"

func TestMain(m *testing.M) {
	// Nexus server
	if ns := os.Getenv("NEXUS_SERVER"); ns != "" {
		NexusServer = ns
	}
	if de := strings.ToLower(os.Getenv("NEXUS_DEBUG")); de != "yes" && de != "y" && de != "true" && de != "enabled" {
		log.SetOutput(ioutil.Discard)
	}

	// Suffix
	rand.Seed(time.Now().UnixNano())
	Suffix = fmt.Sprintf("%04d", rand.Intn(9999))
	UserA = fmt.Sprintf("%s%s", UserA, Suffix)
	UserB = fmt.Sprintf("%s%s", UserB, Suffix)
	UserC = fmt.Sprintf("%s%s", UserC, Suffix)
	UserD = fmt.Sprintf("%s%s", UserD, Suffix)

	// Bootstrap
	bootstrap()

	// Run
	log.Println("Start tests")
	code := m.Run()
	log.Println("End tests")

	// Unbootstrap
	BootstrapErr = nil
	unbootstrap()
	if BootstrapErr != nil {
		log.Println(BootstrapErr)
	}

	// Exit
	os.Exit(code)
}

func bootstrap() {
	var err error

	log.Println("Bootstrapping...")

	// Login root session
	RootSes, err = login("root", "root")
	if err != nil {
		BootstrapErr = fmt.Errorf("Bootstrap: Logging in as root: %s", err.Error())
		return
	}

	log.Println("Created root session")

	// Create users and set tags
	for _, u := range []string{UserA, UserB, UserC, UserD} {
		_, err = RootSes.UserCreate(u, u)
		if err != nil {
			BootstrapErr = fmt.Errorf("Bootstrap: Creating %s user: %s", u, err.Error())
			return
		}
		_, err = RootSes.UserSetTags(u, ".", map[string]interface{}{"@admin": true})
		if err != nil {
			BootstrapErr = fmt.Errorf("Bootstrap: Setting admin tag to %s user: %s", u, err.Error())
			return
		}
		log.Printf("Created user %s\n", u)
	}

	log.Println("Bootstrap done")
}

func unbootstrap() {
	log.Println("Unbootstrapping...")

	// Delete users
	for _, u := range []string{UserA, UserB, UserC, UserD} {
		_, err := RootSes.UserDelete(u)
		if err != nil {
			BootstrapErr = fmt.Errorf("Unbootstrap: Deleting %s user: %s", u, err.Error())
			return
		}
		log.Printf("Deleted user %s\n", u)
	}

	// Stop root session
	RootSes.Close()

	log.Println("Unbootstrap done")
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
