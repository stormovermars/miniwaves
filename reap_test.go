package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// reapEnv: two sessions on gate-app, each owning one doing item.
func reapEnv(t *testing.T) string {
	path, _ := gateEnv(t)
	runAll(t,
		[]string{"add", "task", "A", "-S", "ready"},
		[]string{"add", "task", "B", "-S", "ready"},
		[]string{"start", "TSK-001", "-s", "old", "-a", "builder"},
		[]string{"start", "TSK-002", "-s", "fresh", "-a", "builder"})
	return path
}

func backdate(t *testing.T, path, session, item string, age time.Duration) {
	t.Helper()
	w := loadT(t, path)
	at := time.Now().UTC().Add(-age).Format(time.RFC3339)
	if s, err := sessionByID(w, session); err == nil {
		s.Updated = at
	}
	for i := range w.Tenants[0].Items {
		if it := &w.Tenants[0].Items[i]; it.ID == item {
			it.Updated = at
		}
	}
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
}

func TestSessionStopRefusesWithoutRelease(t *testing.T) {
	path := reapEnv(t)
	err := run([]string{"session", "stop", "old"})
	if err == nil || !strings.Contains(err.Error(), "still owns TSK-001") {
		t.Fatalf("%v", err)
	}
	if it := itemOf(t, path, "TSK-001"); it.Status != "doing" || it.Assignee != "old" {
		t.Fatalf("%+v", it)
	}
}

func TestSessionStopRelease(t *testing.T) {
	path := reapEnv(t)
	out := captureOutput(t, func() { runAll(t, []string{"session", "stop", "old", "--release"}) })
	if !strings.Contains(out, "released TSK-001") {
		t.Fatal(out)
	}
	it := itemOf(t, path, "TSK-001")
	if it.Status != "ready" || it.Assignee != "" || len(it.Notes) == 0 ||
		!strings.Contains(it.Notes[len(it.Notes)-1], "released: ") ||
		!strings.Contains(it.Notes[len(it.Notes)-1], "session old stopped with --release") {
		t.Fatalf("%+v", it)
	}
	s, _ := sessionByID(loadT(t, path), "old")
	if s.State != "stopped" || s.Item != "" {
		t.Fatalf("%+v", s)
	}
	if it := itemOf(t, path, "TSK-002"); it.Status != "doing" {
		t.Fatalf("other session touched: %+v", it)
	}
	// the released item can be claimed again
	runAll(t, []string{"start", "TSK-001", "-s", "fresh2"})
}

func TestReapStaleVsFresh(t *testing.T) {
	path := reapEnv(t)
	backdate(t, path, "old", "TSK-001", 3*time.Hour)
	out := captureOutput(t, func() { runAll(t, []string{"session", "reap"}) })
	if !strings.Contains(out, "reaped session old") || !strings.Contains(out, "released TSK-001") || strings.Contains(out, "fresh") {
		t.Fatal(out)
	}
	it := itemOf(t, path, "TSK-001")
	n := it.Notes[len(it.Notes)-1]
	if it.Status != "ready" || it.Assignee != "" || !strings.Contains(n, "reaped: ") || !strings.Contains(n, "no activity since ") || !strings.Contains(n, "(> 2h)") {
		t.Fatalf("%+v", it)
	}
	if it := itemOf(t, path, "TSK-002"); it.Status != "doing" || it.Assignee != "fresh" {
		t.Fatalf("%+v", it)
	}
	if s, _ := sessionByID(loadT(t, path), "fresh"); s.State != "working" {
		t.Fatalf("%+v", s)
	}
	// stopped sessions are not reaped again; custom threshold
	out = captureOutput(t, func() { runAll(t, []string{"session", "reap", "-A"}) })
	if !strings.Contains(out, "nothing to reap") {
		t.Fatal(out)
	}
	backdate(t, path, "fresh", "TSK-002", 90*time.Minute)
	out = captureOutput(t, func() { runAll(t, []string{"session", "reap", "--older-than", "1h", "-t", "gate-app"}) })
	if !strings.Contains(out, "reaped session fresh") || !strings.Contains(out, "(> 1h)") && !strings.Contains(out, "over 1h") {
		t.Fatal(out)
	}
}

func TestReapDryRunChangesNothing(t *testing.T) {
	path := reapEnv(t)
	backdate(t, path, "old", "TSK-001", 5*time.Hour)
	before := loadT(t, path)
	out := captureOutput(t, func() { runAll(t, []string{"session", "reap", "-n"}) })
	if !strings.Contains(out, "would reap session old") || !strings.Contains(out, "would release TSK-001") {
		t.Fatal(out)
	}
	after := loadT(t, path)
	b1, _ := json.Marshal(before.Tenants)
	b2, _ := json.Marshal(after.Tenants)
	s1, _ := json.Marshal(before.Sessions)
	s2, _ := json.Marshal(after.Sessions)
	if string(b1) != string(b2) || string(s1) != string(s2) {
		t.Fatal("dry-run changed the board")
	}
	out = captureOutput(t, func() { runAll(t, []string{"session", "reap", "--dry-run", "-j"}) })
	if !strings.Contains(out, `"dry_run": true`) && !strings.Contains(out, `"dry_run":true`) {
		t.Fatal(out)
	}
}

func TestReapJSONShape(t *testing.T) {
	path := reapEnv(t)
	backdate(t, path, "old", "TSK-001", 3*time.Hour)
	out := captureOutput(t, func() { runAll(t, []string{"session", "reap", "-A", "-j"}) })
	var got struct {
		Reaped []struct {
			Session string   `json:"session"`
			Tenant  string   `json:"tenant"`
			Items   []string `json:"items"`
		} `json:"reaped"`
		DryRun *bool `json:"dry_run"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if len(got.Reaped) != 1 || got.Reaped[0].Session != "old" || got.Reaped[0].Tenant != "gate-app" ||
		len(got.Reaped[0].Items) != 1 || got.Reaped[0].Items[0] != "TSK-001" || got.DryRun != nil {
		t.Fatalf("%+v", got)
	}
	out = captureOutput(t, func() { runAll(t, []string{"session", "reap", "-A", "-j"}) })
	if !strings.Contains(strings.Join(strings.Fields(out), ""), `"reaped":[]`) {
		t.Fatal(out)
	}
}

func TestBoardAgeAndReviewStale(t *testing.T) {
	path := reapEnv(t)
	backdate(t, path, "old", "TSK-001", 3*time.Hour)
	out := captureOutput(t, func() { runAll(t, []string{"board"}) })
	if !strings.Contains(out, "doing 3h") || !strings.Contains(out, "doing 0m") {
		t.Fatal(out)
	}
	out = captureOutput(t, func() { runAll(t, []string{"board", "-A"}) })
	if !strings.Contains(out, "doing 3h") {
		t.Fatal(out)
	}
	out = captureOutput(t, func() { runAll(t, []string{"review"}) })
	if !strings.Contains(out, "STALE (1)") || !strings.Contains(out, "TSK-001") || strings.Contains(out, "TSK-002") {
		t.Fatal(out)
	}
	out = captureOutput(t, func() { runAll(t, []string{"session", "list", "-v", "-j"}) })
	if !strings.Contains(out, `"age": "3h"`) && !strings.Contains(out, `"age":"3h"`) {
		t.Fatal(out)
	}
}

func TestNoteTouchesSessionLastSeen(t *testing.T) {
	path := reapEnv(t)
	backdate(t, path, "old", "TSK-001", 3*time.Hour)
	runAll(t, []string{"note", "TSK-001", "alive", "-s", "old"})
	s, _ := sessionByID(loadT(t, path), "old")
	if time.Since(parseTS(s.Updated)) > time.Minute {
		t.Fatalf("%+v", s)
	}
	out := captureOutput(t, func() { runAll(t, []string{"session", "reap"}) })
	if !strings.Contains(out, "nothing to reap") {
		t.Fatal(out)
	}
}

func TestReapExceptAndActor(t *testing.T) {
	path := reapEnv(t)
	backdate(t, path, "old", "TSK-001", 3*time.Hour)
	out := captureOutput(t, func() { runAll(t, []string{"session", "reap", "--except", "old,nobody"}) })
	if !strings.Contains(out, "nothing to reap") {
		t.Fatal(out)
	}
	if it := itemOf(t, path, "TSK-001"); it.Status != "doing" {
		t.Fatalf("excepted session reaped: %+v", it)
	}
	runAll(t, []string{"session", "reap", "-s", "janitor"})
	it := itemOf(t, path, "TSK-001")
	last := it.Events[len(it.Events)-1]
	if it.Status != "ready" || last.By != "janitor" || strings.Count(it.Notes[len(it.Notes)-1], "2026") != 2 {
		t.Fatalf("%+v", it)
	}
}

func TestParseAgeLimits(t *testing.T) {
	for _, bad := range []string{"1e9d", "NaNd", "Infd", "-1d", "0d", "366d", "9999h", "0s"} {
		if d, err := parseAge(bad); err == nil {
			t.Errorf("parseAge(%q) = %v, want error", bad, d)
		}
	}
	for _, good := range []string{"90m", "2h", "1d", "365d", "0.5d"} {
		if d, err := parseAge(good); err != nil || d <= 0 {
			t.Errorf("parseAge(%q) = %v, %v", good, d, err)
		}
	}
}

func TestWaveCommandsTouchSession(t *testing.T) {
	path := reapEnv(t)
	runAll(t, []string{"wave", "create", "W"})
	backdate(t, path, "old", "TSK-001", 3*time.Hour)
	runAll(t, []string{"wave", "add", "WAV-001", "TSK-002", "-s", "old"})
	s, _ := sessionByID(loadT(t, path), "old")
	if time.Since(parseTS(s.Updated)) > time.Minute {
		t.Fatalf("session not touched: %+v", s)
	}
}
