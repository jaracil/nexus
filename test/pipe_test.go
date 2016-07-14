package test

import (
	"time"
	"testing"
	nexus "github.com/jaracil/nxcli/nxcore"
)

func TestPipeUnexisting(t *testing.T) {
	conn, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("sys.login userA: %s", err.Error())
	}
	defer conn.Close()
	
	p, _ := conn.PipeOpen("whatever")
	_, err = p.Write("hello")
	if !IsNexusErrCode(err, nexus.ErrInvalidPipe) {
		t.Errorf("pipe.write unexisting: expecting ErrInvalidPipe")
	}
	_, err = p.Read(100, time.Second * 2)
	if !IsNexusErrCode(err, nexus.ErrInvalidPipe) {
		t.Errorf("pipe.read unexisting: expecting ErrInvalidPipe")
	}
	_, err = p.Close()
	if !IsNexusErrCode(err, nexus.ErrInvalidPipe) {
		t.Errorf("pipe.close unexisting: expecting ErrInvalidPipe")
	}
}

func TestPipeWriteReadClose(t *testing.T) {
	rconn, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("sys.login userA: %s", err.Error())
	}
	wconn, err := login(UserB, UserB)
	if err != nil {
		t.Fatalf("sys.login userB: %s", err.Error())
	}
	
	rpipe, err := rconn.PipeCreate()
	if err != nil {
		t.Fatalf("pipe.create: %s", err.Error())
	}
	wpipe, err := wconn.PipeOpen(rpipe.Id())
	if err != nil {
		t.Errorf("pipe.open: %s", err.Error())
	}
	if _, err = wpipe.Write(1); err != nil {
		t.Errorf("pipe.write: %s", err.Error())
	}
	if _, err = wpipe.Write(2); err != nil {
		t.Errorf("pipe.write: %s", err.Error())
	}
	if _, err = wpipe.Write(3); err != nil {
		t.Errorf("pipe.write: %s", err.Error())
	}
	if _, err = wpipe.Write(4); err != nil {
		t.Errorf("pipe.write: %s", err.Error())
	}
	if _, err = wpipe.Write(5); err != nil {
		t.Errorf("pipe.write: %s", err.Error())
	}
	pipeData, err := rpipe.Read(1, time.Second * 3)
	if err != nil {
		t.Errorf("pipe.read: %s", err.Error())
	}
	if len(pipeData.Msgs) != 1 {
		t.Errorf("pipe.read: expecting 1 message")
	}
	if pipeData.Waiting != 4 {
		t.Errorf("pipe.read: expecting 4 messages waiting")
	}
	pipeData, err = rpipe.Read(100, time.Second * 3)
	if err != nil {
		t.Errorf("pipe.read: %s", err.Error())
	}
	if len(pipeData.Msgs) != 4 {
		t.Errorf("pipe.read: expecting 4 messages")
	}
	if pipeData.Waiting != 0 {
		t.Errorf("pipe.read: expecting 0 messages waiting")
	}
	_, err = wpipe.Close()
	if err == nil {
		t.Errorf("pipe.close from writer: expecting error")
	}
	_, err = rpipe.Close()
	if err != nil {
		t.Errorf("pipe.close from reader: %s", err.Error())
	}
	if _, err = wpipe.Write(1); err == nil {
		t.Errorf("pipe.write on closed pipe: expecting error")
	}
	if _, err = rpipe.Read(1, time.Second); err == nil {
		t.Errorf("pipe.read on closed pipe: expecting error")
	}

	time.Sleep(time.Second*1)
	wconn.Close()
	rconn.Close()
}

func TestPipeOverflow(t *testing.T) {
	rconn, err := login(UserA, UserA)
	if err != nil {
		t.Fatalf("sys.login userA: %s", err.Error())
	}
	wconn, err := login(UserB, UserB)
	if err != nil {
		t.Fatalf("sys.login userB: %s", err.Error())
	}
	
	rpipe, err := rconn.PipeCreate(&nexus.PipeOpts{Length: 3})
	if err != nil {
		t.Errorf("pipe.create: %s", err.Error())
	}
	wpipe, err := wconn.PipeOpen(rpipe.Id())
	if err != nil {
		t.Errorf("pipe.open: %s", err.Error())
	}
	wpipe.Write(1)
	wpipe.Write(2)
	wpipe.Write(3)
	wpipe.Write(4)
	wpipe.Write(5)
	wpipe.Write(6)
	pipeData, err := rpipe.Read(100, time.Second * 2)
	if err != nil {
		t.Errorf("pipe.read: %s", err.Error())
	}
	if pipeData.Waiting != 0 {
		t.Errorf("pipe.read: expecting 0 messages waiting")
	}
	if pipeData.Drops != 3 {
		t.Errorf("pipe.read: expecting 3 messages dropped")
	}
	if len(pipeData.Msgs) != 3 {
		t.Errorf("pipe.read: expecting 3 messages")
	}
	_, err = rpipe.Close()
	if err != nil {
		t.Errorf("pipe.close: %s", err.Error())
	}

	time.Sleep(time.Second*1)
	wconn.Close()
	rconn.Close()
}