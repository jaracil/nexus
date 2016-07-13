package test

import (
	"time"
	"testing"
)

// TestSync
func TestSync(t *testing.T) {
	// Bootstrap
	if err := bootstrap(t); err != nil {
		t.Fatal(err)
	}
	
	// Login
	sesa, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("login with UserA: %s", err.Error())
	}
	sesb, err := login(UserB, UserB)
	if err != nil {
		t.Fatalf("login with UserB: %s", err.Error())
	}

	// Unlock not locked
	if done, err := sesa.Unlock(Prefix3); err != nil {
		t.Errorf("sync.unlock not locked: %s", err.Error())
	} else if done {
		t.Errorf("sync.unlock not locked: expecting not done")
	}

	// Lock
	if done, err := sesa.Lock(Prefix3); err != nil {
		t.Errorf("sync.lock: %s", err.Error())
	} else if !done {
		t.Errorf("sync.lock: expecting done")
	}
	
	// Relock
	if done, err := sesa.Lock(Prefix3); err != nil {
		t.Errorf("sync.lock: %s", err.Error())
	} else if done {
		t.Errorf("sync.lock: expecting not done")
	}
	
	// Fail to lock from another session
	if done, err := sesb.Lock(Prefix3); err != nil {
		t.Errorf("sync.lock: %s", err.Error())
	} else if done {
		t.Errorf("sync.lock: expecting not done")
	}
	
	// Unlock
	if done, err := sesa.Unlock(Prefix3); err != nil {
		t.Errorf("sync.unlock: %s", err.Error())
	} else if !done {
		t.Errorf("sync.unlock: expecting done")
	}

	// Lock
	if done, err := sesb.Lock(Prefix3); err != nil {
		t.Errorf("sync.lock: %s", err.Error())
	} else if !done {
		t.Errorf("sync.lock: expecting done")
	}

	// Close ses
	sesb.Close()
	<- sesb.GetContext().Done()
	time.Sleep(time.Second * 2)
	
	// Lock
	if done, err := sesa.Lock(Prefix3); err != nil {
		t.Errorf("sync.lock: %s", err.Error())
	} else if !done {
		t.Errorf("sync.lock: expecting done")
	}

	// Unbootstrap
	sesa.Close()
	if err := unbootstrap(t); err != nil {
		t.Fatal(err)
	}
}