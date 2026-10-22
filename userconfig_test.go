package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
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
	runAll(t, []string{"fanfare", "set", "dir=" + dir, "enabled=false"}, []string{"start", "TSK-001"}, []string{"done", "TSK-001", "ok"})
	plays(t, log, 0)
	// board off (an explicit enabled=false), user on: the board's off wins
	runAll(t, []string{"user", "set", "fanfare.enabled=true"}, []string{"done", "TSK-002", "ok"})
	runAll(t, []string{"done", "TSK-003", "ok"}, []string{"done", "TSK-004", "ok"})
	plays(t, log, 0)
	runAll(t, []string{"fanfare", "set", "enabled=true"})
	// board on, user on: plays (the finale was stamped off, so use a fresh item)
	runAll(t, []string{"add", "task", "E0", "--status", "ready"}, []string{"done", "TSK-005", "ok"})
	if l := plays(t, log, 1); !strings.Contains(l[0], "finale") {
		t.Fatalf("board on, user on: %v", l)
	}
	runAll(t, []string{"fanfare", "set", "enabled=false"})
	// board on, user off: silence again
	runAll(t, []string{"fanfare", "set", "enabled=true"}, []string{"user", "set", "fanfare.enabled=false"},
		[]string{"add", "task", "E", "--status", "ready"}, []string{"done", "TSK-006", "ok"})
	plays(t, log, 1)
	// board on, user unset: the board decides
	runAll(t, []string{"user", "set", "fanfare.enabled="}, []string{"add", "task", "F", "--status", "ready"}, []string{"done", "TSK-007", "ok"})
	plays(t, log, 2)
	// a broken user file never fails a command and falls back to the board
	_ = os.WriteFile(os.Getenv("MW_USER_CONFIG"), []byte("{broken"), 0o600)
	runAll(t, []string{"add", "task", "G", "--status", "ready"}, []string{"done", "TSK-008", "ok"})
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

func configureScript(t *testing.T, script string, fn func()) string {
	t.Helper()
	oldTTY, oldIn := stdinIsTerminal, configureIn
	stdinIsTerminal = func() bool { return true }
	configureIn = strings.NewReader(script)
	t.Cleanup(func() { stdinIsTerminal, configureIn = oldTTY, oldIn })
	return captureOutput(t, fn)
}

func TestConfigureInteractiveSaves(t *testing.T) {
	p := userCfgPath(t)
	// 17 prompts: pane(5) alerts(4) fanfare(2) status(2) discover(2); pane.style -> 5) kanban, zoom 2, fanfare.enabled 1) true, depth 6
	script := "\n5\n2\n\n\n" + // pane
		"\n\n\n\n" + // alerts
		"1\n\n" + // fanfare
		"\n\n" + // status
		"\n6\n" + // discover
		"s\n"
	out := configureScript(t, script, func() { runAll(t, []string{"configure"}) })
	if !strings.Contains(out, "pane.style: normal -> kanban") || !strings.Contains(out, "Save or Discard") {
		t.Fatalf("%s", out)
	}
	var c UserConfig
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	if c.Settings["pane.style"] != "kanban" || c.Settings["pane.zoom"] != "2" || c.Settings["discover.depth"] != "6" || c.Fanfare.Enabled == nil || !*c.Fanfare.Enabled {
		t.Fatalf("%s", b)
	}
	if len(c.Settings) != 3 {
		t.Fatalf("only changed keys are stored: %s", b)
	}
}

func TestConfigureDiscardKeepsFileIdentical(t *testing.T) {
	p := userCfgPath(t)
	runAll(t, []string{"user", "set", "pane.style=blocks", "alerts.on=ask"})
	before, _ := os.ReadFile(p)
	out := configureScript(t, "3\nbogus\n1\n"+strings.Repeat("\n", 20)+"d\n", func() { runAll(t, []string{"configure"}) })
	after, _ := os.ReadFile(p)
	if string(before) != string(after) || !strings.Contains(out, "discarded") {
		t.Fatalf("file changed or no discard:\n%s\n%s", after, out)
	}
	// EOF mid-walk also writes nothing
	configureScript(t, "2\n", func() { runAll(t, []string{"configure"}) })
	if again, _ := os.ReadFile(p); string(again) != string(before) {
		t.Fatal("EOF wrote")
	}
}

type failReader struct{ t *testing.T }

func (f failReader) Read([]byte) (int, error) { f.t.Fatal("stdin was read"); return 0, nil }

func TestConfigureNoTTYPrintsSettings(t *testing.T) {
	userCfgPath(t)
	oldTTY, oldIn := stdinIsTerminal, configureIn
	stdinIsTerminal, configureIn = func() bool { return false }, failReader{t}
	t.Cleanup(func() { stdinIsTerminal, configureIn = oldTTY, oldIn })
	out := captureOutput(t, func() { runAll(t, []string{"configure"}) })
	if !strings.Contains(out, "pane.refresh") || !strings.Contains(out, "(default)") || !strings.Contains(out, "hint:") {
		t.Fatalf("%s", out)
	}
}

func TestConfigureKeyValue(t *testing.T) {
	p := userCfgPath(t)
	for _, bad := range []string{"pane.style=fancy", "pane.zoom=3", "pane.refresh=2m", "alerts.kinds=ask,nope", "alerts.enabled=yes", "discover.depth=0", "status.detail=x", "pane.fit=90", "nokey=1"} {
		if err := run([]string{"configure", bad}); err == nil {
			t.Errorf("%s: expected an error", bad)
		}
	}
	if err := run([]string{"configure", "pane.style=blocks", "pane.zoom=9"}); err == nil {
		t.Fatal("one bad value must refuse all")
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatal("a refused configure wrote the file")
	}
	runAll(t, []string{"configure", "pane.style=Blocks", "pane.zoom=+1", "alerts.kinds=done, ask,done", "fanfare.use=all"})
	out := captureOutput(t, func() { runAll(t, []string{"configure", "--show"}) })
	for _, want := range []string{"pane.style", "blocks  (user)", "pane.zoom", "1  (user)", "ask,done  (user)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
}

func TestUserGetJSONSource(t *testing.T) {
	userCfgPath(t)
	runAll(t, []string{"user", "set", "pane.fit=fill", "fanfare.enabled=true"})
	var v struct {
		Keys []userGetKey `json:"keys"`
	}
	out := captureOutput(t, func() { runAll(t, []string{"user", "get", "-j"}) })
	if err := json.Unmarshal([]byte(out), &v); err != nil || len(v.Keys) != len(userKeys) {
		t.Fatalf("%v: %s", err, out)
	}
	by := map[string]userGetKey{}
	for _, k := range v.Keys {
		by[k.Key] = k
	}
	if k := by["pane.fit"]; k.Value != "fill" || k.Source != "user" || k.Default != "80" || k.Group != "pane" || len(k.Choices) != 2 {
		t.Fatalf("%+v", k)
	}
	if k := by["fanfare.enabled"]; k.Value != "true" || k.Source != "user" {
		t.Fatalf("%+v", k)
	}
	if k := by["pane.style"]; k.Value != "normal" || k.Source != "default" {
		t.Fatalf("%+v", k)
	}
	if !strings.Contains(out, `"choices":[]`) && !strings.Contains(out, `"choices": []`) {
		t.Fatalf("free-text choices must be an empty array: %s", out)
	}
	one := captureOutput(t, func() { runAll(t, []string{"user", "get", "pane.zoom", "-j"}) })
	if strings.Count(one, `"key"`) != 1 {
		t.Fatalf("%s", one)
	}
	wantErr(t, "unknown user key", "user", "get", "nope")
	if txt := captureOutput(t, func() { runAll(t, []string{"user", "get", "pane.fit"}) }); strings.TrimSpace(txt) != "pane.fit  fill  (user)" {
		t.Fatalf("%q", txt)
	}
}

func getKey(t *testing.T, key string) userGetKey {
	t.Helper()
	var v struct {
		Keys []userGetKey `json:"keys"`
	}
	o := captureOutput(t, func() { runAll(t, []string{"user", "get", key, "-j"}) })
	if err := json.Unmarshal([]byte(o), &v); err != nil || len(v.Keys) != 1 {
		t.Fatalf("%v: %s", err, o)
	}
	return v.Keys[0]
}

func TestUserGetFanfareSourceBoardOffWins(t *testing.T) {
	inTempDir(t)
	t.Setenv("MW_USER_CONFIG", filepath.Join(t.TempDir(), "mw-user.json"))
	// no board: default, then user
	if k := getKey(t, "fanfare.enabled"); k.Source != "default" || k.Value != "false" {
		t.Fatalf("no board, no user: %+v", k)
	}
	runAll(t, []string{"user", "set", "fanfare.enabled=true"})
	if k := getKey(t, "fanfare.enabled"); k.Source != "user" || k.Value != "true" {
		t.Fatalf("no board, user on: %+v", k)
	}
	// board with no fanfare setting + user true: on, source user
	runAll(t, []string{"init", "Ws"})
	if k := getKey(t, "fanfare.enabled"); k.Source != "user" || k.Value != "true" {
		t.Fatalf("board unset, user on: %+v", k)
	}
	w := boardWorkspace()
	if c, src := fanfareEffectiveSrc(w); !fanfareOn(c) || src["enabled"] != "user" {
		t.Fatalf("board unset effective: %v %v", fanfareOn(c), src)
	}
	// board false + user true: off, source board
	runAll(t, []string{"fanfare", "set", "enabled=false"})
	if k := getKey(t, "fanfare.enabled"); k.Source != "board" || k.Value != "false" {
		t.Fatalf("board off, user on: %+v", k)
	}
	if c, src := fanfareEffectiveSrc(boardWorkspace()); fanfareOn(c) || src["enabled"] != "board" {
		t.Fatalf("board off effective: %v %v", fanfareOn(c), src)
	}
	// board true + user true: on
	runAll(t, []string{"fanfare", "set", "enabled=true"})
	if c, _ := fanfareEffectiveSrc(boardWorkspace()); !fanfareOn(c) {
		t.Fatal("board on, user on must be on")
	}
}

func TestUserAlertKindsNoneAndNew(t *testing.T) {
	userCfgPath(t)
	runAll(t, []string{"user", "set", "alerts.kinds=none"})
	if k := getKey(t, "alerts.kinds"); k.Value != "" || k.Source != "user" {
		t.Fatalf("none: %+v", k)
	}
	runAll(t, []string{"user", "set", "alerts.kinds=new,done"})
	if k := getKey(t, "alerts.kinds"); k.Value != "done,new" {
		t.Fatalf("new,done: %+v", k)
	}
	runAll(t, []string{"user", "set", "alerts.kinds=all"})
	if k := getKey(t, "alerts.kinds"); !strings.Contains(","+k.Value+",", ",new,") {
		t.Fatalf("all lacks new: %+v", k)
	}
	if k := userKeyByName("alerts.kinds"); !strings.Contains(k.Default, "new") || !strings.Contains(k.allowed(), "none") {
		t.Fatalf("default %q allowed %q", k.Default, k.allowed())
	}
}

func TestUserConfigConcurrentSetsLoseNothing(t *testing.T) {
	p := userCfgPath(t)
	sets := [][2]string{{"pane.refresh", "5m"}, {"pane.style", "blocks"}, {"pane.zoom", "2"}, {"pane.fit", "fill"}, {"pane.open", "news"},
		{"alerts.enabled", "false"}, {"alerts.sound", "false"}, {"status.detail", "full"}, {"discover.depth", "6"}, {"alerts.message", "false"}}
	var wg sync.WaitGroup
	errs := make(chan error, len(sets))
	for _, kv := range sets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- updateUserConfig(p, func(c *UserConfig) (bool, error) {
				time.Sleep(5 * time.Millisecond) // widen the read-modify-write window
				return true, setUserKey(c, kv[0], kv[1])
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	c, _, _, err := readUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range sets {
		if c.Settings[kv[0]] != kv[1] {
			t.Errorf("%s = %q, want %q (lost update)", kv[0], c.Settings[kv[0]], kv[1])
		}
	}
}

func TestConfigureSaveKeepsConcurrentChange(t *testing.T) {
	p := userCfgPath(t)
	orig, _, _, _ := readUserConfig()
	nc := cloneUserConfig(orig)
	_ = setUserKey(&nc, "pane.zoom", "1")
	runAll(t, []string{"user", "set", "pane.style=kanban"}) // lands while the session is open
	if err := saveConfigureDiff(orig, nc, p); err != nil {
		t.Fatal(err)
	}
	c, _, _, _ := readUserConfig()
	if c.Settings["pane.zoom"] != "1" || c.Settings["pane.style"] != "kanban" {
		t.Fatalf("settings %v", c.Settings)
	}
}

func TestFanfareBoardEnabledTriState(t *testing.T) {
	inTempDir(t)
	t.Setenv("MW_USER_CONFIG", filepath.Join(t.TempDir(), "mw-user.json"))
	runAll(t, []string{"init", "Ws"})
	runAll(t, []string{"user", "set", "fanfare.enabled=true"})
	// `fanfare use all` / `fanfare set <other key>` on a fresh board must not decide enabled
	runAll(t, []string{"fanfare", "use", "all"})
	runAll(t, []string{"fanfare", "set", "volume=2"})
	if w := boardWorkspace(); w.Fanfare == nil || w.Fanfare.Enabled != nil {
		t.Fatalf("board enabled must stay unset: %+v", w.Fanfare)
	}
	if c, src := fanfareEffectiveSrc(boardWorkspace()); !fanfareOn(c) || src["enabled"] != "user" {
		t.Fatalf("fresh fanfare block, user on: %v %v", fanfareOn(c), src)
	}
	if k := getKey(t, "fanfare.enabled"); k.Source != "user" || k.Value != "true" {
		t.Fatalf("fresh fanfare block, user on: %+v", k)
	}
	// explicit board off beats user on
	runAll(t, []string{"fanfare", "set", "enabled=false"})
	if c, src := fanfareEffectiveSrc(boardWorkspace()); fanfareOn(c) || src["enabled"] != "board" {
		t.Fatalf("board off, user on: %v %v", fanfareOn(c), src)
	}
	// explicit board true, user false: off, source user
	runAll(t, []string{"fanfare", "set", "enabled=true"}, []string{"user", "set", "fanfare.enabled=false"})
	if c, src := fanfareEffectiveSrc(boardWorkspace()); fanfareOn(c) || src["enabled"] != "user" {
		t.Fatalf("board on, user off: %v %v", fanfareOn(c), src)
	}
}
