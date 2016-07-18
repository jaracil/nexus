package test

import (
	nexus "github.com/jaracil/nxcli/nxcore"
	"testing"
	"time"
)

func TestUserCreateFail(t *testing.T) {
	_, err := RootSes.UserCreate("root", "whatever")
	if err == nil {
		t.Errorf("user.create existing: expecting error")
	}
	_, err = RootSes.UserCreate("", "")
	if err == nil {
		t.Logf("user.create empty: expecting error: work needed on client input checking")
		RootSes.UserDelete("")
	}
}

func TestUserDeleteFail(t *testing.T) {
	_, err := RootSes.UserDelete("whatever")
	if err == nil {
		t.Errorf("user.delete unexisting: expecting error")
	}
}

func TestUserCreateDelete(t *testing.T) {
	_, err := RootSes.UserCreate("abcdef", "abcdef")
	if err != nil {
		t.Errorf("user.create: %s", err.Error())
	}
	_, err = RootSes.UserDelete("abcdef")
	if err != nil {
		t.Errorf("user.delete: %s", err.Error())
	}
}

func TestUserSetPass(t *testing.T) {
	_, err := RootSes.UserSetPass(UserA, "newpass")
	if err != nil {
		t.Errorf("user.setPass: %s", err.Error())
	}
	_, err = login(UserA, UserA)
	if !IsNexusErrCode(err, nexus.ErrPermissionDenied) {
		t.Errorf("user.login changed pass: expecting permission denied")
	}
	_, err = RootSes.UserSetPass(UserA, UserA)
	if err != nil {
		t.Errorf("user.setPass: %s", err.Error())
	}
	conn, err := login(UserA, UserA)
	if err != nil {
		t.Errorf("user.login changed pass: %s", err.Error())
	}
	conn.Close()
}

func TestUserTags(t *testing.T) {
	_, err := RootSes.UserSetTags(UserA, Prefix1, map[string]interface{}{
		"test":   1,
		"prueba": []string{"vaya", "vaya"},
		"otra":   map[string]interface{}{"a": 1, "b": 2},
		"yes":    true,
		"":       "",
	})
	if err != nil {
		t.Errorf("user.setTags: %s", err.Error())
	}

	sesA, err := login(UserA, UserA)
	if err != nil {
		t.Errorf("user.login: %s", err.Error())
	}

	_, _, err = sesA.ExecNoWait("task.push", map[string]interface{}{
		"method": Prefix1 + ".method",
		"params": "hello",
	})
	if err != nil {
		t.Errorf("task.push execNoWait: %s", err.Error())
	}

	task, err := RootSes.TaskPull(Prefix1, time.Second*30)
	if err != nil {
		t.Errorf("task.pull: expecting task: %s", err.Error())
	}
	if _, ok := task.Tags["test"].(float64); !ok {
		t.Errorf("task.tags missing test")
	}
	if _, ok := task.Tags["prueba"].([]interface{}); !ok {
		t.Errorf("task.tags missing prueba")
	}
	if _, ok := task.Tags["otra"].(map[string]interface{}); !ok {
		t.Errorf("task.tags missing otra")
	}
	if _, ok := task.Tags["yes"].(bool); !ok {
		t.Errorf("task.tags missing yes")
	}
	if _, ok := task.Tags[""].(string); !ok {
		t.Errorf("task.tags missing \"\"")
	}
	task.SendResult("ok")

	_, err = RootSes.UserDelTags(UserA, Prefix1, []string{"test", "otra"})
	if err != nil {
		t.Errorf("user.delTags: %s", err.Error())
	}

	//_, err = sesA.Login(UserA, UserA)
	//if err != nil {
	//	t.Errorf("user.relogin: %s", err.Error())
	//}
	//defer sesA.Close()

	_, err = sesA.Exec("sys.reload", nil)
	if err != nil {
		t.Errorf("sys.reload: %s", err.Error())
	}

	_, _, err = sesA.ExecNoWait("task.push", map[string]interface{}{
		"method": Prefix1 + ".method",
		"params": "hello",
	})
	if err != nil {
		t.Errorf("task.push execNoWait: %s", err.Error())
	}

	task, err = RootSes.TaskPull(Prefix1, time.Second*30)
	if err != nil {
		t.Errorf("task.pull: expecting task: %s", err.Error())
	}
	if _, ok := task.Tags["test"]; ok {
		t.Errorf("task.tags unexpected field test")
	}
	if _, ok := task.Tags["otra"]; ok {
		t.Errorf("task.tags unexepected field otra")
	}
	if _, ok := task.Tags["prueba"]; !ok {
		t.Errorf("task.tags missing field prueba")
	}
	task.SendResult("ok")

	if _, err = RootSes.UserSetTags("blablabla", Prefix1, map[string]interface{}{"x": "d"}); err == nil {
		t.Errorf("user.setTags unexisting: expecting error")
	}
}
