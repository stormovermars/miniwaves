package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecideUnderSession(t *testing.T) {
	path := needsBoard(t)
	runAll(t, []string{"decide", "Ship", "it", "--by", "Sir", "-s", "bot"})
	w, _ := load(path)
	ch, err := itemByID(&w.Tenants[0], "CHR-001")
	if err != nil {
		t.Fatal(err)
	}
	e := ch.Events[len(ch.Events)-1]
	if e.Action != "decide" || e.By != "bot" || e.Note != "Ship it (for Sir)" {
		t.Fatalf("session decision: %+v", e)
	}
	runAll(t, []string{"decide", "Alone", "-s", "bot"})
	w, _ = load(path)
	ch, _ = itemByID(&w.Tenants[0], "CHR-001")
	if e := ch.Events[len(ch.Events)-1]; e.By != "bot" || e.Note != "Alone" {
		t.Fatalf("session without --by: %+v", e)
	}
	// a human run is still attributed to the human
	runAll(t, []string{"decide", "Human", "--by", "Sir"})
	w, _ = load(path)
	ch, _ = itemByID(&w.Tenants[0], "CHR-001")
	if e := ch.Events[len(ch.Events)-1]; e.By != "Sir" {
		t.Fatalf("human: %+v", e)
	}
}

// ask/answer/decide on an unknown item fail and save nothing; jobs set with a bad key stores nothing.
func TestNegativesLeaveBoardUntouched(t *testing.T) {
	path := needsBoard(t)
	runAll(t, []string{"jobs", "enable", "j1", "-t", "needs-app"})
	snap := func() string {
		b, _ := os.ReadFile(path)
		tb, _ := os.ReadFile(tenantPath(path, "needs-app"))
		return string(b) + string(tb)
	}
	before := snap()
	for _, args := range [][]string{
		{"ask", "TSK-999", "q", "-s", "bot"}, {"answer", "TSK-999", "a", "--by", "Sir"}, {"decide", "x", "--by", "Sir", "-p", "TSK-999"},
		{"jobs", "set", "j1", "title=ok", "bogus=1"},
	} {
		if err := run(args); err == nil {
			t.Errorf("%v: expected an error", args)
		}
	}
	if snap() != before {
		t.Fatal("a failed command changed the board")
	}
	if !strings.Contains(before, "j1") {
		t.Fatal("setup")
	}
}

func TestFanfareStatusUserOverride(t *testing.T) {
	inTempDir(t)
	t.Setenv("MW_USER_CONFIG", filepath.Join(t.TempDir(), "mw-user.json"))
	runAll(t, []string{"init", "Ws"}, []string{"fanfare", "set", "enabled=true"}, []string{"user", "set", "fanfare.enabled=false"})
	var m map[string]any
	if err := json.Unmarshal([]byte(captureOutput(t, func() { runAll(t, []string{"fanfare", "-j"}) })), &m); err != nil {
		t.Fatal(err)
	}
	src, _ := m["source"].(map[string]any)
	if m["enabled"] != false || src["enabled"] != "user" || m["config"].(map[string]any)["enabled"] != true {
		t.Fatalf("status: %v", m)
	}
	if out := captureOutput(t, func() { runAll(t, []string{"fanfare"}) }); !strings.Contains(out, "fanfare: disabled (user; board: enabled)") {
		t.Fatalf("text:\n%s", out)
	}
}

func TestBoardJSONHasNeeds(t *testing.T) {
	needsBoard(t)
	runAll(t, []string{"ask", "TSK-001", "which?", "-s", "bot"})
	for _, args := range [][]string{{"board", "-j"}, {"board", "-A", "-j"}} {
		out := captureOutput(t, func() { runAll(t, args) })
		if !strings.Contains(out, `"needs": [`) || !strings.Contains(out, "which?") {
			t.Errorf("%v lacks needs: %s", args, out)
		}
	}
}

func buildDispatcherPrompt(path string, w *Workspace, name string) string {
	d, _ := jobPromptData(w, "")
	return buildDispatcherPromptJob(path, w, name, d)
}

func TestJobsEnableMovedAndUserUseValidation(t *testing.T) {
	inTempDir(t)
	t.Setenv("MW_USER_CONFIG", filepath.Join(t.TempDir(), "mw-user.json"))
	runAll(t, []string{"init", "Ws"}, []string{"jobs", "enable", "j1", "-t", "ws"})
	out := captureOutput(t, func() { runAll(t, []string{"jobs", "enable", "j2", "-t", "ws"}) })
	if !strings.Contains(out, "moved ws from job j1") {
		t.Fatalf("enable must say it moved the tenant:\n%s", out)
	}
	wantErr(t, "fanfare.use", "user", "set", "fanfare.use=Calm.x")
	runAll(t, []string{"user", "set", "fanfare.use=calm-x"})
}
