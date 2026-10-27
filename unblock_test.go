package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDoneUnblocksDependents(t *testing.T) {
	contractBoard(t)
	runAll(t, []string{"start", "BUG-001", "-s", "w1"}, []string{"block", "BUG-001", "waits", "--on", "TSK-001", "-s", "w1"})
	runAll(t, []string{"start", "TSK-001"}, []string{"done", "TSK-001", "ok"})
	w, _ := load(filepath.Join(mustGetwd(), dataFile))
	it, err := itemByID(&w.Tenants[0], "BUG-001")
	if err != nil || it.Status != "ready" {
		t.Fatalf("dependent not unblocked: %+v %v", it, err)
	}
	if e := it.Events[len(it.Events)-1]; e.Action != "ready" || e.Note != "unblocked: TSK-001 done" {
		t.Fatalf("event: %+v", e)
	}
	out := captureOutput(t, func() { runAll(t, []string{"status", "--line"}) })
	if strings.Contains(out, "N1") {
		t.Fatalf("needs-you count rose: %q", out)
	}
}

func blockedOn(t *testing.T, id string) string {
	t.Helper()
	w, _ := load(filepath.Join(mustGetwd(), dataFile))
	it, err := itemByID(&w.Tenants[0], id)
	if err != nil {
		t.Fatal(err)
	}
	return it.Status
}

func TestUnblockWaitsForAccept(t *testing.T) {
	contractBoard(t)
	runAll(t, []string{"gate", "judge", "on", "--by", "sir"}, []string{"start", "BUG-001", "-s", "w1"}, []string{"block", "BUG-001", "waits", "--on", "TSK-001", "-s", "w1"})
	runAll(t, []string{"start", "TSK-001"}, []string{"done", "TSK-001", "ok"})
	if s := blockedOn(t, "BUG-001"); s != "blocked" {
		t.Fatalf("judge on: done must not unblock, got %s", s)
	}
	runAll(t, []string{"accept", "TSK-001", "--by", "sir"})
	if s := blockedOn(t, "BUG-001"); s != "ready" {
		t.Fatalf("accept must unblock, got %s", s)
	}
}

func TestUnblockOnStoryAcceptAndWaveClose(t *testing.T) {
	contractBoard(t)
	runAll(t, []string{"gate", "judge", "on", "--by", "sir"}, []string{"gate", "evidence", "off", "--by", "sir"},
		[]string{"add", "story", "Story"}, []string{"add", "task", "Kid", "-p", "STR-001", "-S", "ready"}, []string{"wave", "create", "W"},
		[]string{"start", "BUG-001", "-s", "w1"}, []string{"block", "BUG-001", "waits", "--on", "STR-001", "-s", "w1"})
	runAll(t, []string{"start", "TSK-001"}, []string{"block", "TSK-001", "waits wave", "--on", "WAV-001"})
	runAll(t, []string{"start", "TSK-002"}, []string{"done", "TSK-002", "ok"})
	if s := blockedOn(t, "BUG-001"); s != "blocked" {
		t.Fatalf("open story must not unblock, got %s", s)
	}
	runAll(t, []string{"accept", "STR-001", "--by", "sir"})
	if s := blockedOn(t, "BUG-001"); s != "ready" {
		t.Fatalf("story accept must unblock, got %s", s)
	}
	if s := blockedOn(t, "TSK-001"); s != "blocked" {
		t.Fatalf("open wave must not unblock, got %s", s)
	}
	runAll(t, []string{"wave", "step", "WAV-001", "--na-rest", "t"}, []string{"wave", "close", "WAV-001", "--no-evidence", "t"})
	if s := blockedOn(t, "TSK-001"); s != "ready" {
		t.Fatalf("wave close must unblock, got %s", s)
	}
}

func TestBlockOnDoneRefused(t *testing.T) {
	contractBoard(t)
	runAll(t, []string{"start", "TSK-001"}, []string{"done", "TSK-001", "ok"}, []string{"start", "BUG-001", "-s", "w1"})
	wantErr(t, "already done; nothing to wait on", "block", "BUG-001", "x", "--on", "TSK-001", "-s", "w1")
}
