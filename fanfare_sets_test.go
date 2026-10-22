package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func setsWorkspace(t *testing.T) string {
	t.Helper()
	inTempDir(t)
	dir := fakeCollection(t)
	runAll(t, []string{"init", "Ws"}, []string{"fanfare", "set", "enabled=true", "dir=" + dir})
	return dir
}

func TestFanfareSetsAddDropUse(t *testing.T) {
	t.Setenv("MW_FANFARE_PLAYER", "/stub")
	setsWorkspace(t)
	runAll(t, []string{"fanfare", "sets", "add", "mine", "a", "b"}, []string{"fanfare", "sets", "add", "mine", "b", "c"})
	out := captureOutput(t, func() { runAll(t, []string{"fanfare", "sets", "-j"}) })
	want := `{"use":"all","sets":[{"name":"mine","active":false,"pairs":[{"name":"a","state":"playable"},{"name":"b","state":"playable"},{"name":"c","state":"playable"}]}]}` + "\n"
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
	runAll(t, []string{"fanfare", "use", "mine"})
	if txt := captureOutput(t, func() { runAll(t, []string{"fanfare", "sets"}) }); !strings.Contains(txt, "* mine: a (playable), b (playable), c (playable)") || !strings.Contains(txt, "use: mine") {
		t.Fatalf("%s", txt)
	}
	runAll(t, []string{"fanfare", "sets", "drop", "mine", "a"})
	wantErr(t, "not in set", "fanfare", "sets", "drop", "mine", "a")
	if out := captureOutput(t, func() { runAll(t, []string{"fanfare", "sets", "-j"}) }); strings.Contains(out, `"name":"a"`) {
		t.Fatalf("a not dropped: %s", out)
	}
	runAll(t, []string{"fanfare", "sets", "drop", "mine"}) // the active set: use resets
	out = captureOutput(t, func() { runAll(t, []string{"fanfare", "sets", "-j"}) })
	if out != `{"use":"all","sets":[]}`+"\n" {
		t.Fatalf("%s", out)
	}
	wantErr(t, "unknown set", "fanfare", "use", "mine")
	wantErr(t, "unknown set", "fanfare", "sets", "drop", "mine")
	runAll(t, []string{"fanfare", "use", "all"})
}

func TestFanfareSetsValidation(t *testing.T) {
	t.Setenv("MW_FANFARE_PLAYER", "/stub")
	setsWorkspace(t)
	for _, bad := range []string{"all", "Up", "_x", "", "has space", strings.Repeat("a", 33)} {
		wantErr(t, "set name", "fanfare", "sets", "add", bad, "a")
	}
	runAll(t, []string{"fanfare", "sets", "add", strings.Repeat("a", 32), "a"}, []string{"fanfare", "sets", "add", "0-ok", "a"})
	wantErr(t, "unknown pair", "fanfare", "sets", "add", "x", "nope")
	wantErr(t, "unknown pair", "fanfare", "sets", "add", "mine", "a", "nope")
	if out := captureOutput(t, func() { runAll(t, []string{"fanfare", "sets", "-j"}) }); strings.Contains(out, `"name":"mine"`) {
		t.Fatalf("a refused add left a set: %s", out)
	}
	wantErr(t, "usage", "fanfare", "sets", "add", "x")
	wantErr(t, "usage", "fanfare", "use")
}

func TestFanfarePickLimitedToSet(t *testing.T) {
	log := stubPlayer(t)
	setsWorkspace(t)
	runAll(t, []string{"fanfare", "sets", "add", "only-b", "b", "star-wars"}, []string{"fanfare", "use", "only-b"})
	for i := 0; i < 6; i++ {
		runAll(t, []string{"fanfare", "test"})
	}
	for _, l := range plays(t, log, 6) {
		if !strings.Contains(l, "/b/") {
			t.Fatalf("played outside the set: %s", l)
		}
	}
	// the picker alone: only b is ready inside the set
	w, _ := load(".miniwaves.json")
	c := fanfareCfg(w)
	p, err := pickPairFor(c, expandHome(c.Dir), func(n int) int {
		if n != 1 {
			t.Fatalf("candidates %d, want 1", n)
		}
		return 0
	})
	if err != nil || p.Name != "b" {
		t.Fatalf("%v %v", p, err)
	}
}

func TestFanfareSetFallbackNote(t *testing.T) {
	log := stubPlayer(t)
	setsWorkspace(t)
	runAll(t, []string{"fanfare", "sets", "add", "dud", "star-wars", "half"}, []string{"fanfare", "use", "dud"})
	var err error
	note := captureStderr(t, func() { err = run([]string{"fanfare", "test"}) })
	if err != nil {
		t.Fatal(err)
	}
	plays(t, log, 1)
	if strings.Count(note, "\n") != 1 || !strings.HasPrefix(note, "mw: fanfare:") || !strings.Contains(note, "dud") {
		t.Fatalf("note %q", note)
	}
	// no note without an active set
	runAll(t, []string{"fanfare", "use", "all"})
	if note := captureStderr(t, func() { err = run([]string{"fanfare", "test"}) }); note != "" || err != nil {
		t.Fatalf("%q %v", note, err)
	}
}

func TestFanfareStatusJSONSuperset(t *testing.T) {
	t.Setenv("MW_FANFARE_PLAYER", "/stub")
	dir := setsWorkspace(t)
	runAll(t, []string{"fanfare", "sets", "add", "mine", "a", "c"}, []string{"fanfare", "use", "mine"}, []string{"fanfare", "set", "volume=0.5"})
	out := captureOutput(t, func() { runAll(t, []string{"fanfare", "-j"}) })
	if !strings.HasPrefix(out, "{\n  \"") {
		t.Fatalf("not indented: %s", out)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"dir": `"` + dir + `"`, "ready": "3", "unplayable": "0", "incomplete": "1", "empty": "1", "manifest": "false",
		"enabled": "true", "use": `"mine"`, "sets": `{"mine":["a","c"]}`, "milestone_seconds": "15", "finale_seconds": "60", "volume": "0.5",
		"pairs": `[{"name":"a","playable":true},{"name":"b","playable":true},{"name":"c","playable":true},{"name":"half","playable":false},{"name":"star-wars","playable":false}]`,
	}
	for k, v := range want {
		var got bytes.Buffer
		if err := json.Compact(&got, m[k]); err != nil || got.String() != v {
			t.Fatalf("%s: got %s want %s", k, got.String(), v)
		}
	}
	var cfg FanfareConfig
	if err := json.Unmarshal(m["config"], &cfg); err != nil || !cfg.Enabled || cfg.Use != "mine" {
		t.Fatalf("config: %s %v", m["config"], err)
	}
	if txt := captureOutput(t, func() { runAll(t, []string{"fanfare"}) }); !strings.Contains(txt, "\nuse: mine\n") {
		t.Fatalf("%s", txt)
	}
}
