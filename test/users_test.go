package test

import (
	"log"
	"testing"
)

// TestUser
func TestUser(t *testing.T) {
	var err error
	log.Println("Starting TestPing")

	// Bootrap error
	if BootstrapErr != nil {
		t.Fatal(BootstrapErr)
	}

	_, err = RootSes.UserCreate("", "")
	if err == nil {
		t.Errorf("user.create empty: Expecting an error")
	}

	_, err = RootSes.UserCreate("root", "whatever")
	if err == nil {
		t.Errorf("user.create existing: Expecting an error")
	}

	_, err = RootSes.UserCreate("root", "whatever")
	if err == nil {
		t.Errorf("user.create existing: Expecting an error")
	}
}
