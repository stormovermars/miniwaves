package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func userCfgPath(t *testing.T) string {
	t.Helper()
	inTempDir(t)
	p := filepath.Join(t.TempDir(), "cfg", "mw-user.json")
	t.Setenv("MW_USER_CONFIG", p)
	return p
}

func TestUserSetShow(t *testing.T) {
	p := userCfgPath(t) // no board needed: run it from an empty folder
	if out := captureOutput(t, func() { runAll(t, []string{"user", "show"}) }); !strings.Contains(out, "not created yet") || !strings.Contains(out, "alerts.on: (unset)") {
		t.Fatalf("fresh show:\n%s", out)
	}
	runAll(t, []string{"user", "set", "alerts.on=Ask, block,ask", "alerts.exec=[\"notify-send\",\"mw\"]", "fanfare.enabled=false", "fanfare.use=all"})
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	var v userView
	out := captureOutput(t, func() { runAll(t, []string{"user", "show", "-j"}) })
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !v.Exists || v.Path != p || strings.Join(v.AlertsOn, ",") != "ask,block" || strings.Join(v.AlertsExec, " ") != "notify-send mw" ||
		v.FanfareEnabled == nil || *v.FanfareEnabled || v.FanfareUse != "all" {
		t.Fatalf("show: %+v", v)
	}
	text := captureOutput(t, func() { runAll(t, []string{"user", "show"}) })
	for _, want := range []string{"alerts.on: ask,block", `alerts.exec: ["notify-send","mw"]`, "fanfare.enabled: false", "fanfare.use: all"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q:\n%s", want, text)
		}
	}
	// set merges; an empty value unsets
	runAll(t, []string{"user", "set", "fanfare.enabled=", "alerts.on="})
	out = captureOutput(t, func() { runAll(t, []string{"user", "show", "-j"}) })
	v = userView{}
	_ = json.Unmarshal([]byte(out), &v)
	if v.FanfareEnabled != nil || len(v.AlertsOn) != 0 || len(v.AlertsExec) != 2 || v.FanfareUse != "all" {
		t.Fatalf("after unset: %+v", v)
	}
}

func TestUserValidation(t *testing.T) {
	p := userCfgPath(t)
	for _, args := range [][]string{
		{"user"},
		{"user", "bogus"},
		{"user", "set"},
		{"user", "set", "nokey=1"},
		{"user", "set", "alerts.on"},
		{"user", "set", "alerts.on=Bad Name"},
		{"user", "set", "alerts.on=a;b"},
		{"user", "set", "alerts.exec=notify-send mw"},
		{"user", "set", `alerts.exec={"a":1}`},
		{"user", "set", `alerts.exec=[]`},
		{"user", "set", `alerts.exec=[""]`},
		{"user", "set", `alerts.exec=[1]`},
		{"user", "set", "fanfare.enabled=yes"},
		{"user", "set", "fanfare.enabled=1"},
		{"user", "set", "fanfare.use=../x"},
		{"user", "set", "fanfare.use=a b"},
		{"user", "show", "extra"},
	} {
		if err := run(args); err == nil {
			t.Errorf("%v: expected an error", args)
		}
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatal("a refused set must not create the file")
	}
	// one bad key stops the whole set
	if err := run([]string{"user", "set", "fanfare.use=ok", "fanfare.enabled=maybe"}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatal("partial write")
	}
}

func TestUserFileModeRefusedOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no mode check on Windows")
	}
	p := userCfgPath(t)
	runAll(t, []string{"user", "set", "fanfare.use=all"})
	for _, mode := range []os.FileMode{0o660, 0o666, 0o622, 0o602} {
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"user", "show"}, {"user", "set", "fanfare.use=x"}} {
			err := run(args)
			if err == nil || !strings.Contains(err.Error(), "writable by group or others") || !strings.Contains(err.Error(), "chmod 600") {
				t.Fatalf("mode %o %v: %v", mode, args, err)
			}
		}
	}
	for _, mode := range []os.FileMode{0o600, 0o644, 0o640, 0o400} {
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		if err := run([]string{"user", "show"}); err != nil {
			t.Fatalf("mode %o must be fine: %v", mode, err)
		}
	}
	// a malformed file is a clear error, not a panic
	_ = os.Chmod(p, 0o600)
	_ = os.WriteFile(p, []byte("{nope"), 0o600)
	if err := run([]string{"user", "show"}); err == nil || !strings.Contains(err.Error(), p) {
		t.Fatalf("malformed: %v", err)
	}
}

func TestUserModeCheckSkippedWhenDisabled(t *testing.T) {
	p := userCfgPath(t)
	runAll(t, []string{"user", "set", "fanfare.use=all"})
	if runtime.GOOS == "windows" {
		return
	}
	_ = os.Chmod(p, 0o666)
	old := userModeCheck
	userModeCheck = false // what Windows does
	defer func() { userModeCheck = old }()
	if err := run([]string{"user", "show"}); err != nil {
		t.Fatalf("the check is off: %v", err)
	}
}

func TestUserConfigDirDefault(t *testing.T) {
	t.Setenv("MW_USER_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AppData", filepath.Join(home, "AppData"))
	p, err := userConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "mw-user.json" || filepath.Base(filepath.Dir(p)) != "miniwaves" {
		t.Fatalf("path %s", p)
	}
	d, _ := os.UserConfigDir()
	if filepath.Dir(filepath.Dir(p)) != d {
		t.Fatalf("path %s is not under UserConfigDir %s", p, d)
	}
}

func TestUserFanfareOverridesBoard(t *testing.T) {
	log := stubPlayer(t)
	inTempDir(t)
	t.Setenv("MW_USER_CONFIG", filepath.Join(t.TempDir(), "mw-user.json"))
	dir := fakeCollection(t)
	runAll(t, []string{"init", "Ws"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"},
		[]string{"add", "task", "A", "--status", "ready"}, []string{"add", "task", "B", "--status", "ready"},
		[]string{"add", "task", "C", "--status", "ready"}, []string{"add", "task", "D", "--status", "ready"})
	// board off, no user choice: silence
	runAll(t, []string{"fanfare", "set", "dir=" + dir}, []string{"start", "TSK-001"}, []string{"done", "TSK-001", "ok"})
	plays(t, log, 0)
	// board off, user on: the user wins
	runAll(t, []string{"user", "set", "fanfare.enabled=true"}, []string{"done", "TSK-002", "ok"})
	// TSK-003/4 are still open: no finale, no wave close, so finish the rest to trigger the finale
	runAll(t, []string{"done", "TSK-003", "ok"}, []string{"done", "TSK-004", "ok"})
	if l := plays(t, log, 1); !strings.Contains(l[0], "finale") {
		t.Fatalf("user on over board off: %v", l)
	}
	// board on, user off: silence again
	runAll(t, []string{"fanfare", "set", "enabled=true"}, []string{"user", "set", "fanfare.enabled=false"},
		[]string{"add", "task", "E", "--status", "ready"}, []string{"done", "TSK-005", "ok"})
	plays(t, log, 1)
	// board on, user unset: the board decides
	runAll(t, []string{"user", "set", "fanfare.enabled="}, []string{"add", "task", "F", "--status", "ready"}, []string{"done", "TSK-006", "ok"})
	plays(t, log, 2)
	// a broken user file never fails a command and falls back to the board
	_ = os.WriteFile(os.Getenv("MW_USER_CONFIG"), []byte("{broken"), 0o600)
	runAll(t, []string{"add", "task", "G", "--status", "ready"}, []string{"done", "TSK-007", "ok"})
	plays(t, log, 3)
}

func TestUserFanfareDefaults(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "mw-user.json")
	t.Setenv("MW_USER_CONFIG", cfg)
	t.Setenv("MW_NO_FANFARE", "")
	t.Setenv("MW_NO_PLUGINS", "")
	// fresh: both keys written
	if s := out(t, "user", "fanfare-defaults"); !strings.Contains(s, "on, set all") {
		t.Fatalf("out %q", s)
	}
	if s := out(t, "user", "show", "-j"); !strings.Contains(s, `"fanfare_enabled": true`) || !strings.Contains(s, `"fanfare_use": "all"`) {
		t.Fatalf("show %q", s)
	}
	// existing choices stay untouched
	out(t, "user", "set", "fanfare.enabled=false", "fanfare.use=calm")
	if s := out(t, "user", "fanfare-defaults"); !strings.Contains(s, "already set") {
		t.Fatalf("out %q", s)
	}
	if s := out(t, "user", "show", "-j"); !strings.Contains(s, `"fanfare_enabled": false`) || !strings.Contains(s, `"fanfare_use": "calm"`) {
		t.Fatalf("show %q", s)
	}
}

func TestUserFanfareDefaultsOptOut(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "mw-user.json")
	t.Setenv("MW_USER_CONFIG", cfg)
	for _, env := range []string{"MW_NO_FANFARE", "MW_NO_PLUGINS"} {
		t.Setenv("MW_NO_FANFARE", "")
		t.Setenv("MW_NO_PLUGINS", "")
		t.Setenv(env, "1")
		if s := out(t, "user", "fanfare-defaults"); !strings.Contains(s, "skipped") {
			t.Fatalf("%s: out %q", env, s)
		}
		if _, err := os.Stat(cfg); err == nil {
			t.Fatalf("%s: config written", env)
		}
	}
}
