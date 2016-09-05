package test

import (
	"testing"
	"time"
	"github.com/jaracil/nxcli/nxcore"
)

func TestSyncUnlockNotLocked(t *testing.T) {
	ses, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("login with UserA: %s", err.Error())
	}
	defer ses.Close()
	if _, err := ses.Unlock(Prefix3); err != nil && !IsNexusErrCode(err, nxcore.ErrLockNotOwned) {
		t.Errorf("sync.unlock not locked: %s", err.Error())
	} else if err == nil {
		t.Errorf("sync.unlock not locked: expecting not done")
	}
}

func TestSyncRelock(t *testing.T) {
	ses, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("login with UserA: %s", err.Error())
	}
	defer ses.Close()

	if _, err := ses.Lock(Prefix3); err != nil && !IsNexusErrCode(err, nxcore.ErrLockNotOwned) {
		t.Errorf("sync.lock: %s", err.Error())
	} else if err != nil {
		t.Errorf("sync.lock: expecting done")
	}
	if _, err := ses.Lock(Prefix3); err != nil && !IsNexusErrCode(err, nxcore.ErrLockNotOwned) {
		t.Errorf("sync.lock: %s", err.Error())
	} else if err == nil {
		t.Errorf("sync.lock: expecting not done")
	}
	ses.Unlock(Prefix3)
}

func TestSyncLockFail(t *testing.T) {
	sesa, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("login with UserA: %s", err.Error())
	}
	sesb, err := login(UserB, UserB)
	if err != nil {
		t.Fatalf("login with UserB: %s", err.Error())
	}
	defer sesa.Close()
	defer sesb.Close()

	// Lock
	if _, err := sesa.Lock(Prefix3); err != nil && !IsNexusErrCode(err, nxcore.ErrLockNotOwned) {
		t.Errorf("sync.lock: %s", err.Error())
	} else if err != nil {
		t.Errorf("sync.lock: expecting done")
	}

	time.Sleep(time.Millisecond * 100)

	// Fail to lock from another session
	if _, err := sesb.Lock(Prefix3); err != nil && !IsNexusErrCode(err, nxcore.ErrLockNotOwned) {
		t.Errorf("sync.lock: %s", err.Error())
	} else if err == nil {
		t.Errorf("sync.lock: expecting not done")
	}
	sesa.Unlock(Prefix3)

}

func TestSyncUnlockSesClose(t *testing.T) {
	sesa, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("login with UserA: %s", err.Error())
	}
	sesb, err := login(UserB, UserB)
	if err != nil {
		t.Fatalf("login with UserB: %s", err.Error())
	}
	defer sesa.Close()
	defer sesb.Close()

	// Lock
	if _, err := sesb.Lock(Prefix3); err != nil && !IsNexusErrCode(err, nxcore.ErrLockNotOwned) {
		t.Errorf("sync.lock: %s", err.Error())
	} else if err != nil {
		t.Errorf("sync.lock: expecting done")
	}

	// Close ses
	sesb.Close()
	<-sesb.GetContext().Done()
	time.Sleep(time.Second * 1)

	// Lock
	if _, err := sesa.Lock(Prefix3); err != nil && !IsNexusErrCode(err, nxcore.ErrLockNotOwned) {
		t.Errorf("sync.lock: %s", err.Error())
	} else if err != nil {
		t.Errorf("sync.lock: expecting done")
	}
	sesa.Unlock(Prefix3)
}
