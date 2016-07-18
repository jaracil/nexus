package test

import (
	"sync"
	"time"
	"testing"
	nexus "github.com/jaracil/nxcli/nxcore"
)

func TestTaskTimeout(t *testing.T) {
	pushconn, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("sys.login userA: %s", err.Error())
	}
	defer pushconn.Close()
	pullconn, err := login(UserB, UserB)
	if err != nil {
		t.Fatalf("sys.login userB: %s", err.Error())
	}
	defer pullconn.Close()
	_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 2, &nexus.TaskOpts{})
	if !IsNexusErrCode(err, nexus.ErrTimeout) {
		t.Error("task.push without pull: expecting timeout")
	}
	_, err = pullconn.TaskPull(Prefix4, time.Second * 2)
	if !IsNexusErrCode(err, nexus.ErrTimeout) {
		t.Errorf("task.pull: expecting timeout", err.Error())
	}
}

func TestTaskAccept(t *testing.T) {
	donech := make(chan bool, 0)
	pushconn, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("sys.login userA: %s", err.Error())
	}
	defer pushconn.Close()
	pullconn, err := login(UserB, UserB)
	if err != nil {
		t.Fatalf("sys.login userB: %s", err.Error())
	}
	defer pullconn.Close()
	go func() {
		res, err := pushconn.TaskPush(Prefix4+".method", nil, time.Second * 20, &nexus.TaskOpts{})
		if err != nil {
			t.Errorf("task.push err: %s", err.Error())
		}
		if res != nil {
			t.Error("task.push: expecting nil res from accept")
		}
		donech <- true
	}()
	task, err := pullconn.TaskPull(Prefix4, time.Second * 20)
	if err != nil {
		t.Errorf("task.pull: %s", err.Error())
	}
	_, err = task.Accept()
	if err != nil {
		t.Errorf("task.accept: %s", err.Error())
	}
	<- donech
}

func TestTaskReject(t *testing.T) {
	donech := make(chan bool, 0)
	pushconn, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("sys.login userA: %s", err.Error())
	}
	defer pushconn.Close()
	pullconn, err := login(UserB, UserB)
	if err != nil {
		t.Fatalf("sys.login userB: %s", err.Error())
	}
	defer pullconn.Close()

	go func() {
		_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 30, &nexus.TaskOpts{})
		if !IsNexusErrCode(err, 1) {
			t.Error("task.push err: expecting error code 1")
		}
		donech <- true
	}()
	task, err := pullconn.TaskPull(Prefix4, time.Second * 20)
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
	<- donech
}

func TestTaskExpireTTL(t *testing.T) {
	donech := make(chan bool, 0)
	pushconn, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("sys.login userA: %s", err.Error())
	}
	defer pushconn.Close()
	pullconn, err := login(UserB, UserB)
	if err != nil {
		t.Fatalf("sys.login userB: %s", err.Error())
	}
	defer pullconn.Close()

	go func() {
		_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 30, &nexus.TaskOpts{Ttl: 3})
		if !IsNexusErrCode(err, nexus.ErrTtlExpired) {
			t.Errorf("task.push err: expecting ErrTtlExpired: %s", err.Error())
		}
		donech <- true
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
	<- donech
}

func TestTaskPrio(t *testing.T) {
	wg := &sync.WaitGroup{}
	pushconn, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("sys.login userA: %s", err.Error())
	}
	defer pushconn.Close()
	pullconn, err := login(UserB, UserB)
	if err != nil {
		t.Fatalf("sys.login userB: %s", err.Error())
	}
	defer pullconn.Close()
	
	wg.Add(5)
	go func() {
		_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 30, &nexus.TaskOpts{Priority: 5})
		if err != nil {
			t.Errorf("task.push: %s", err.Error())
		}
		wg.Done()
	}()
	go func() {
		_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 30, &nexus.TaskOpts{Priority: 10})
		if err != nil {
			t.Errorf("task.push: %s", err.Error())
		}
		wg.Done()
	}()
	go func() {
		_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 30, &nexus.TaskOpts{Priority: 20})
		if err != nil {
			t.Errorf("task.push: %s", err.Error())
		}
		wg.Done()
	}()
	go func() {
		_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 30, &nexus.TaskOpts{Priority: 15})
		if err != nil {
			t.Errorf("task.push: %s", err.Error())
		}
		wg.Done()
	}()
	go func() {
		_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 30, &nexus.TaskOpts{Priority: -500})
		if err != nil {
			t.Errorf("task.push: %s", err.Error())
		}
		wg.Done()
	}()
	time.Sleep(time.Second * 2)
	task, err := pullconn.TaskPull(Prefix4, time.Second * 6)
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
	wg.Wait()
}

func TestTaskDetach(t *testing.T) {
	pushconn, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("sys.login userA: %s", err.Error())
	}
	defer pushconn.Close()
	pullconn, err := login(UserB, UserB)
	if err != nil {
		t.Fatalf("sys.login userB: %s", err.Error())
	}
	defer pullconn.Close()

	_, err = pushconn.TaskPush(Prefix4+".method", nil, time.Second * 20, &nexus.TaskOpts{Detach: true})
	if err != nil {
		t.Errorf("task.push err: %s", err.Error())
	}
	task, err := pullconn.TaskPull(Prefix4, time.Second * 20)
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
}

func TestTaskCancel(t *testing.T) {
	donech := make(chan bool, 0)
	pushconn, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("sys.login userA: %s", err.Error())
	}
	defer pushconn.Close()
	pullconn, err := login(UserB, UserB)
	if err != nil {
		t.Fatalf("sys.login userB: %s", err.Error())
	}
	defer pullconn.Close()

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
		donech <- true
	}()
	time.Sleep(time.Second * 1)
	_, err = pushconn.Exec("task.cancel", map[string]interface{}{"id": execId})
	if err != nil {
		t.Errorf("task.cancel exec: %s", err.Error())
	}
	<- donech
}