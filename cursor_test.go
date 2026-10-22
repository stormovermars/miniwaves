package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type sinceDoc struct {
	Events []logRow `json:"events"`
	Next   string   `json:"next"`
}

func logSinceJ(t *testing.T, since string, extra ...string) sinceDoc {
	t.Helper()
	var d sinceDoc
	args := append([]string{"log", "--since", since, "-j"}, extra...)
	out := captureOutput(t, func() {
		if err := run(args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	})
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if d.Events == nil {
		t.Fatalf("events must be an array: %s", out)
	}
	return d
}

func keysOf(d sinceDoc) []string {
	var k []string
	for _, e := range d.Events {
		k = append(k, e.Key)
	}
	return k
}

// addLateEvent appends an event with an explicit stamp, as if a git merge brought it in from another machine.
func addLateEvent(t *testing.T, path, tenant, item string, at time.Time, action, note string) {
	t.Helper()
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	tn, _ := tenantBySlug(w, tenant)
	it, err := itemByID(tn, item)
	if err != nil {
		t.Fatal(err)
	}
	it.Events = append(it.Events, Event{At: at.UTC().Format(time.RFC3339Nano), Action: action, By: "other-machine", Note: note})
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
}

func cursorBoard(t *testing.T) string {
	t.Helper()
	path := inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t,
		[]string{"init", "Cursor app"},
		[]string{"gate", "level", "trust", "--workspace", "--by", "t"},
		[]string{"add", "task", "One", "-S", "ready"},
		[]string{"add", "task", "Two", "-S", "ready"},
	)
	return path
}

func TestLogSinceStartAndResume(t *testing.T) {
	cursorBoard(t)
	d := logSinceJ(t, "start")
	// created + ready for both tasks, oldest first
	if len(d.Events) != 4 || d.Next == "" {
		t.Fatalf("start: %+v", d)
	}
	for i := 1; i < len(d.Events); i++ {
		if d.Events[i].At < d.Events[i-1].At {
			t.Fatalf("not oldest first: %+v", d.Events)
		}
	}
	if d.Events[0].Key != "cursor-app/TSK-001/0" {
		t.Fatalf("key: %q", d.Events[0].Key)
	}
	if e := logSinceJ(t, ""); len(e.Events) != 4 {
		t.Fatalf(`--since "" returns everything: %+v`, e)
	}
	again := logSinceJ(t, d.Next)
	if len(again.Events) != 0 {
		t.Fatalf("nothing new: %+v", again.Events)
	}
	runAll(t, []string{"note", "TSK-001", "hello"})
	n := logSinceJ(t, again.Next)
	if len(n.Events) != 1 || n.Events[0].Action != "note" || n.Events[0].Key != "cursor-app/TSK-001/2" {
		t.Fatalf("new event: %+v", n.Events)
	}
	if len(logSinceJ(t, n.Next).Events) != 0 {
		t.Fatal("delivered twice")
	}
}

func TestLogSinceLateGitMergedEventOnce(t *testing.T) {
	path := cursorBoard(t)
	d := logSinceJ(t, "start")
	// the late event is stamped before the cursor time (an hour before "now"), and arrives afterwards
	addLateEvent(t, path, "cursor-app", "TSK-002", time.Now().Add(-time.Hour), "note", "from the other clone")
	n := logSinceJ(t, d.Next)
	if len(n.Events) != 1 || n.Events[0].Note != "from the other clone" || n.Events[0].By != "other-machine" {
		t.Fatalf("late event must be returned: %+v", n.Events)
	}
	// exactly once: again from the new cursor and again from the old one's successor
	if m := logSinceJ(t, n.Next); len(m.Events) != 0 {
		t.Fatalf("late event returned twice: %+v", m.Events)
	}
	// a second late event and a fresh one in the same read
	addLateEvent(t, path, "cursor-app", "TSK-001", time.Now().Add(-90*time.Minute), "note", "late 2")
	runAll(t, []string{"note", "TSK-002", "fresh"})
	r := logSinceJ(t, n.Next)
	if len(r.Events) != 2 {
		t.Fatalf("late 2 + fresh: %+v", r.Events)
	}
	if m := logSinceJ(t, r.Next); len(m.Events) != 0 {
		t.Fatalf("returned twice: %+v", m.Events)
	}
	// an event older than the lookback is not returned (documented limit); a wider lookback does return it
	addLateEvent(t, path, "cursor-app", "TSK-001", time.Now().Add(-48*time.Hour), "note", "ancient")
	if m := logSinceJ(t, r.Next); len(m.Events) != 0 {
		t.Fatalf("outside the lookback: %+v", m.Events)
	}
	if m := logSinceJ(t, r.Next, "--lookback", "3d"); len(m.Events) != 1 || m.Events[0].Note != "ancient" {
		t.Fatalf("--lookback 3d: %+v", m.Events)
	}
	// a short lookback misses what a long one would see
	addLateEvent(t, path, "cursor-app", "TSK-002", time.Now().Add(-3*time.Hour), "note", "three hours")
	if m := logSinceJ(t, r.Next, "--lookback", "1h"); len(m.Events) != 0 {
		t.Fatalf("1h lookback: %+v", m.Events)
	}
}

func TestLogSinceLimitResumes(t *testing.T) {
	cursorBoard(t)
	first := logSinceJ(t, "start", "-n", "3")
	if len(first.Events) != 3 {
		t.Fatalf("-n 3: %+v", first.Events)
	}
	rest := logSinceJ(t, first.Next, "-n", "3")
	if len(rest.Events) != 1 {
		t.Fatalf("remaining: %+v", rest.Events)
	}
	seen := map[string]bool{}
	for _, k := range append(keysOf(first), keysOf(rest)...) {
		if seen[k] {
			t.Fatalf("duplicate %s", k)
		}
		seen[k] = true
	}
	if len(seen) != 4 {
		t.Fatalf("lost events: %v", seen)
	}
}

func TestLogSinceArchivePruning(t *testing.T) {
	path := cursorBoard(t)
	runAll(t, []string{"start", "TSK-001"}, []string{"done", "TSK-001", "ok", "--no-evidence", "x"})
	d := logSinceJ(t, "start")
	c, err := decodeCursor(d.Next)
	if err != nil || len(c.Seen) != len(d.Events) {
		t.Fatalf("cursor: %+v %v", c, err)
	}
	// archive TSK-001 (done, any age): its events leave the board
	runAll(t, []string{"archive", "--days", "0"})
	w, _ := load(path)
	if _, err := itemByID(&w.Tenants[0], "TSK-001"); err == nil {
		t.Fatal("TSK-001 should be archived")
	}
	n := logSinceJ(t, d.Next)
	if len(n.Events) != 0 {
		t.Fatalf("archiving must not replay or invent events: %+v", n.Events)
	}
	c2, _ := decodeCursor(n.Next)
	for _, k := range c2.Seen {
		if strings.Contains(k, "TSK-001") {
			t.Fatalf("pruned item still in the cursor: %v", c2.Seen)
		}
	}
	// the cursor keeps working: a new event and a late one on the surviving item are each returned once
	runAll(t, []string{"note", "TSK-002", "after archive"})
	addLateEvent(t, path, "cursor-app", "TSK-002", time.Now().Add(-time.Hour), "note", "late after archive")
	r := logSinceJ(t, n.Next)
	if len(r.Events) != 2 {
		t.Fatalf("after archive: %+v", r.Events)
	}
	if m := logSinceJ(t, r.Next); len(m.Events) != 0 {
		t.Fatalf("twice: %+v", m.Events)
	}
	// and an old cursor taken before the archive still works on the pruned board
	if m := logSinceJ(t, d.Next); len(m.Events) != 2 {
		t.Fatalf("old cursor on pruned board: %+v", m.Events)
	}
}

func TestLogSinceTextAllAndErrors(t *testing.T) {
	cursorBoard(t)
	runAll(t, []string{"tenant", "add", "other", "Other"}, []string{"add", "task", "Z", "-t", "other"})
	text := captureOutput(t, func() { runAll(t, []string{"log", "-A", "--since", "start"}) })
	lines := strings.Split(strings.TrimSpace(text), "\n")
	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, "next: ") || len(lines) != 6 { // 4 events + 1 for tenant other + next
		t.Fatalf("text:\n%s", text)
	}
	if !strings.Contains(text, "other") {
		t.Fatalf("-A spans tenants:\n%s", text)
	}
	cur := strings.TrimPrefix(last, "next: ")
	if out := captureOutput(t, func() { runAll(t, []string{"log", "-A", "--since", cur}) }); strings.Count(out, "\n") != 1 {
		t.Fatalf("resumed text should only hold the next line:\n%s", out)
	}
	for _, args := range [][]string{
		{"log", "--since", "!!notacursor"},
		{"log", "--since", "e30"}, // base64 of {} is fine; see below
		{"log", "--lookback", "1h"},
		{"log", "--since", "start", "--lookback", "0"},
		{"log", "--since", "start", "--lookback", "soon"},
		{"log", "--since", "start", "-n", "0"},
		{"log", "-A", "-t", "other", "--since", "start"},
	} {
		err := run(args)
		if args[2] == "e30" {
			if err != nil {
				t.Errorf("%v: an empty cursor object is the start: %v", args, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%v: expected an error", args)
		}
	}
	if !readOnly([]string{"log", "--since", "start"}) {
		t.Fatal("log is a read")
	}
}

func TestGoldenLogSince(t *testing.T) {
	cursorBoard(t)
	d := logSinceJ(t, "start")
	d.Next = "<CURSOR>"
	for i := range d.Events {
		d.Events[i].At = "<TS>"
	}
	b, _ := json.MarshalIndent(d, "", "  ")
	checkGolden(t, "log-since-j", string(b)+"\n")
}
