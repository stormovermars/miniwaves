package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReopenMessageAndShowFull(t *testing.T) {
	inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t, []string{"init", "App"}, []string{"add", "task", "T", "-S", "ready"})
	runAll(t, []string{"done", "TSK-001", "ok", "--no-evidence", "t"})
	long := strings.Repeat("rework because the header was wrong ", 6)
	runAll(t, []string{"reopen", "TSK-001", "-m", long})
	runAll(t, []string{"done", "TSK-001", "ok again", "--no-evidence", "t"}, []string{"reopen", "TSK-001"})
	out := captureOutput(t, func() { runAll(t, []string{"show", "TSK-001", "-j"}) })
	var d struct {
		Events []Event `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range d.Events {
		if e.Action == "reopen" {
			got = append(got, e.Note)
		}
	}
	if len(got) != 2 || got[0] != strings.TrimSpace(long) || got[1] != "" {
		t.Fatalf("reopen notes %q", got)
	}
	short := captureOutput(t, func() { runAll(t, []string{"show", "TSK-001"}) })
	if !strings.Contains(short, "reopen") || strings.Contains(short, strings.TrimSpace(long)) {
		t.Fatalf("plain show should cut the note:\n%s", short)
	}
	full := captureOutput(t, func() { runAll(t, []string{"show", "TSK-001", "--full"}) })
	if !strings.Contains(full, strings.TrimSpace(long)) {
		t.Fatalf("--full cut the note:\n%s", full)
	}
	// every event, not just the last five
	for i := 0; i < 4; i++ {
		runAll(t, []string{"ready", "TSK-001"}, []string{"done", "TSK-001", "x", "--no-evidence", "t"}, []string{"reopen", "TSK-001"})
	}
	if n := strings.Count(captureOutput(t, func() { runAll(t, []string{"show", "TSK-001", "--full"}) }), "reopen"); n < 6 {
		t.Fatalf("--full lists %d reopen events", n)
	}
	wantErr(t, "not valid here", "done", "TSK-001", "x", "-m", "no", "--no-evidence", "t")
}
