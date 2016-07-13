package test

import (
	"time"
	"testing"
	nexus "github.com/jaracil/nxcli/nxcore"
)

// TestTask
func TestTask(t *testing.T) {
	// Bootstrap
	if err := bootstrap(t); err != nil {
		t.Fatal(err)
	}
	
	pushconn, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("sys.login userA: %s", err.Error())
	}

	pullconn, err := login(UserB, UserB)
	if err != nil {
		t.Fatalf("sys.login userB: %s", err.Error())
	}
	
	// Push timeout
	_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 2, &nexus.TaskOpts{})
	if !IsNexusErrCode(err, nexus.ErrTimeout) {
		t.Error("task.push without pull: expecting timeout")
	}
	
	// Pull timeout
	task, err := pullconn.TaskPull(Prefix4, time.Second * 2)
	if !IsNexusErrCode(err, nexus.ErrTimeout) {
		t.Errorf("task.pull: expecting timeout", err.Error())
	}

	// Task accept
	go func() {
		res, err := pushconn.TaskPush(Prefix4+".method", nil, time.Second * 20, &nexus.TaskOpts{})
		if err != nil {
			t.Errorf("task.push err: %s", err.Error())
		}
		if res != nil {
			t.Error("task.push: expecting nil res from accept")
		}
	}()
	task, err = pullconn.TaskPull(Prefix4, time.Second * 20)
	if err != nil {
		t.Errorf("task.pull: %s", err.Error())
	}
	_, err = task.Accept()
	if err != nil {
		t.Errorf("task.accept: %s", err.Error())
	}

	// Task reject
	go func() {
		_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 30, &nexus.TaskOpts{})
		if !IsNexusErrCode(err, 1) {
			t.Error("task.push err: expecting error code 1")
		}
	}()
	task, err = pullconn.TaskPull(Prefix4, time.Second * 20)
	if err != nil {
		t.Errorf("task.pull: %s", err.Error())
	}
	_, err = task.Reject()
	if err != nil {
		t.Errorf("task.reject: %s", err.Error())
	}
	task, err = pullconn.TaskPull(Prefix4, time.Second * 20)
	if err != nil {
		t.Errorf("task.pull: %s", err.Error())
	}
	_, err = task.SendError(1, "1", nil)
	if err != nil {
		t.Errorf("task.sendError: %s", err.Error())
	}
	_, err = task.SendError(2, "2", nil)
	if err == nil {
		t.Error("task.sendError: expecting an error")
	}

	// Task expire ttl
	go func() {
		_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 30, &nexus.TaskOpts{Ttl: 3})
		if !IsNexusErrCode(err, nexus.ErrTtlExpired) {
			t.Errorf("task.push err: expecting ErrTtlExpired: %s", err.Error())
		}
	}()
	for i := 0; i < 3; i++ {
		task, err := pullconn.TaskPull(Prefix4, time.Second * 6)
		if err != nil {
			t.Errorf("task.pull: %s", err.Error())
		}
		_, err = task.Reject()
		if err != nil {
			t.Errorf("task.reject: %s", err.Error())
		}
	}
	
	// Task prio
	go func() {
		_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 30, &nexus.TaskOpts{Priority: 5})
		if err != nil {
			t.Errorf("task.push: %s", err.Error())
		}
	}()
	go func() {
		_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 30, &nexus.TaskOpts{Priority: 10})
		if err != nil {
			t.Errorf("task.push: %s", err.Error())
		}
	}()
	go func() {
		_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 30, &nexus.TaskOpts{Priority: 20})
		if err != nil {
			t.Errorf("task.push: %s", err.Error())
		}
	}()
	go func() {
		_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 30, &nexus.TaskOpts{Priority: 15})
		if err != nil {
			t.Errorf("task.push: %s", err.Error())
		}
	}()
	go func() {
		_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 30, &nexus.TaskOpts{Priority: -500})
		if err != nil {
			t.Errorf("task.push: %s", err.Error())
		}
	}()
	time.Sleep(time.Second * 2)
	task, err = pullconn.TaskPull(Prefix4, time.Second * 6)
	if err != nil {
		t.Errorf("task.pull: %s", err.Error())
	}
	if task.Prio != 20 {
		t.Errorf("task.pull prio: expecting prio 20 got %d", task.Prio)
	}
	task.SendResult("ok")
	task, err = pullconn.TaskPull(Prefix4, time.Second * 6)
	if err != nil {
		t.Errorf("task.pull: %s", err.Error())
	}
	if task.Prio != 15 {
		t.Errorf("task.pull prio: expecting prio 15 got %d", task.Prio)
	}
	task.SendResult("ok")
	task, err = pullconn.TaskPull(Prefix4, time.Second * 6)
	if err != nil {
		t.Errorf("task.pull: %s", err.Error())
	}
	if task.Prio != 10 {
		t.Errorf("task.pull prio: expecting prio 10 got %d", task.Prio)
	}
	task.SendResult("ok")
	task, err = pullconn.TaskPull(Prefix4, time.Second * 6)
	if err != nil {
		t.Errorf("task.pull: %s", err.Error())
	}
	if task.Prio != 5 {
		t.Errorf("task.pull prio: expecting prio 5 got %d", task.Prio)
	}
	task.SendResult("ok")
	task, err = pullconn.TaskPull(Prefix4, time.Second * 6)
	if err != nil {
		t.Errorf("task.pull: %s", err.Error())
	}
	if task.Prio != -500 {
		t.Errorf("task.pull prio: expecting prio -500 got %d", task.Prio)
	}
	task.SendResult("ok")

	// Detach
	_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 20, &nexus.TaskOpts{Detach: true})
	if err != nil {
		t.Errorf("task.push err: %s", err.Error())
	}
	task, err = pullconn.TaskPull(Prefix4, time.Second * 20)
	if err != nil {
		t.Errorf("task.pull: %s", err.Error())
	}
	if !task.Detach {
		t.Errorf("task.pull: expecting a detached task")
	}
	_, err = task.Accept()
	if err != nil {
		t.Errorf("task.accept: %s", err.Error())
	}

	// Task cancel
	execId, _, err := pushconn.ExecNoWait("task.push", map[string]interface{}{
		"method": Prefix4+".method",
		"params": "hello",
	})
	if err != nil {
		t.Errorf("task.push execNoWait: %s", err.Error())
	}
	go func() {
		_, err = pullconn.TaskPull(Prefix4, time.Second * 20)
		if err != nil {
			if !IsNexusErrCode(err, nexus.ErrCancel) {
				t.Errorf("task.pull: expecting ErrCancel: %s", err.Error())
			}
		}
	}()
	time.Sleep(time.Second * 1)
	_, err = pushconn.Exec("task.cancel", map[string]interface{}{"id": execId})
	if err != nil {
		t.Errorf("task.cancel exec: %s", err.Error())
	}

	// Unbootstrap
	time.Sleep(time.Second*1)
	pushconn.Close()
	pullconn.Close()
	if err := unbootstrap(t); err != nil {
		t.Fatal(err)
	}
}