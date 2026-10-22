package main

import (
	"encoding/json"
	"regexp"
	"strconv"
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
	for i := 1; i < len(d.Events); i++ { // parsed times: RFC3339Nano drops trailing zeros, so strings do not sort
		a, _ := time.Parse(time.RFC3339Nano, d.Events[i-1].At)
		b, _ := time.Parse(time.RFC3339Nano, d.Events[i].At)
		if b.Before(a) {
			t.Fatalf("not oldest first: %+v", d.Events)
		}
	}
	if !regexp.MustCompile(`^cursor-app/TSK-00[12]/[0-9a-f]{12}$`).MatchString(d.Events[0].Key) {
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
	if len(n.Events) != 1 || n.Events[0].Action != "note" || !strings.HasPrefix(n.Events[0].Key, "cursor-app/TSK-001/") {
		t.Fatalf("new event: %+v", n.Events)
	}
	if len(logSinceJ(t, n.Next).Events) != 0 {
		t.Fatal("delivered twice")
	}
}

func TestLogSinceLateGitMergedEventOnce(t *testing.T) {
	path := cursorBoard(t)
	d := logSinceJ(t, "start")
	// an event from another clone, stamped after the cursor time, arrives with a merge: once
	addLateEvent(t, path, "cursor-app", "TSK-002", time.Now().Add(time.Minute), "note", "from the other clone")
	n := logSinceJ(t, d.Next)
	if len(n.Events) != 1 || n.Events[0].Note != "from the other clone" || n.Events[0].By != "other-machine" {
		t.Fatalf("merged event must be returned: %+v", n.Events)
	}
	if m := logSinceJ(t, n.Next); len(m.Events) != 0 {
		t.Fatalf("merged event returned twice: %+v", m.Events)
	}
	// two more events at one timestamp, delivered one per read (-n 1): neither lost nor repeated
	at := time.Now().Add(2 * time.Minute)
	addLateEvent(t, path, "cursor-app", "TSK-001", at, "note", "same stamp a")
	addLateEvent(t, path, "cursor-app", "TSK-002", at, "note", "same stamp b")
	a := logSinceJ(t, n.Next, "-n", "1")
	b := logSinceJ(t, a.Next, "-n", "1")
	if len(a.Events) != 1 || len(b.Events) != 1 || a.Events[0].Key == b.Events[0].Key {
		t.Fatalf("same-stamp events: %+v %+v", a.Events, b.Events)
	}
	if m := logSinceJ(t, b.Next); len(m.Events) != 0 {
		t.Fatalf("returned twice: %+v", m.Events)
	}
	// an event stamped older than the window before the cursor time is not returned; start returns it
	addLateEvent(t, path, "cursor-app", "TSK-001", time.Now().Add(-48*time.Hour), "note", "ancient")
	if m := logSinceJ(t, b.Next); len(m.Events) != 0 {
		t.Fatalf("before the window: %+v", m.Events)
	}
}

// A git merge that reorders an item's events (a foreign event lands at an old index) must neither lose the
// foreign event nor repeat any other: keys follow content, not position.
func TestLogSinceMergeReorder(t *testing.T) {
	path := cursorBoard(t)
	runAll(t, []string{"note", "TSK-001", "a"}, []string{"note", "TSK-001", "b"})
	d := logSinceJ(t, "start")
	w, _ := load(path)
	it, _ := itemByID(&w.Tenants[0], "TSK-001")
	ev := it.Events
	last := ev[len(ev)-1].At
	lt, _ := time.Parse(time.RFC3339Nano, last)
	foreign := Event{At: lt.Add(-time.Second).UTC().Format(time.RFC3339Nano), Action: "note", By: "other", Note: "foreign"} // stamped BEFORE the cursor time
	n := len(ev)
	// swap the last two and insert the foreign event at the old index of the second to last
	it.Events = append(append(append([]Event{}, ev[:n-2]...), foreign), ev[n-1], ev[n-2])
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	m := logSinceJ(t, d.Next)
	if len(m.Events) != 1 || m.Events[0].Note != "foreign" {
		t.Fatalf("want only the foreign event once: %+v", m.Events)
	}
	if x := logSinceJ(t, m.Next); len(x.Events) != 0 {
		t.Fatalf("repeat: %+v", x.Events)
	}
}

// A tenant that stamped before another tenant's save lands behind the cursor time: still delivered, once.
func TestLogSinceTwoTenantInterleave(t *testing.T) {
	path := cursorBoard(t)
	runAll(t, []string{"tenant", "add", "other", "Other"}, []string{"add", "task", "Z", "-t", "other"})
	d := logSinceJ(t, "start", "-A")
	now := time.Now()
	// Y stamped t2 and saved first; X stamped t1 < t2 and saved after Y was polled
	addLateEvent(t, path, "other", "TSK-001", now.Add(time.Minute), "note", "Y t2")
	m := logSinceJ(t, d.Next, "-A")
	if len(m.Events) != 1 || m.Events[0].Note != "Y t2" {
		t.Fatalf("Y: %+v", m.Events)
	}
	addLateEvent(t, path, "cursor-app", "TSK-001", now.Add(30*time.Second), "note", "X t1")
	x := logSinceJ(t, m.Next, "-A")
	if len(x.Events) != 1 || x.Events[0].Note != "X t1" {
		t.Fatalf("X must arrive once: %+v", x.Events)
	}
	if y := logSinceJ(t, x.Next, "-A"); len(y.Events) != 0 {
		t.Fatalf("repeat: %+v", y.Events)
	}
}

// Old-form cursors (seen = full tenant/item/H keys) keep matching: no duplicates.
func TestLogSinceOldFormSeenKeys(t *testing.T) {
	cursorBoard(t)
	d := logSinceJ(t, "start")
	var last time.Time
	for _, e := range d.Events {
		if ts, _ := time.Parse(time.RFC3339Nano, e.At); ts.After(last) {
			last = ts
		}
	}
	old := logCursor{T: last.UTC().Format(time.RFC3339Nano), Seen: keysOf(d)}
	if n := logSinceJ(t, encodeCursor(old)); len(n.Events) != 0 {
		t.Fatalf("old-form keys must still match: %+v", n.Events)
	}
}

func TestLogSinceFile(t *testing.T) {
	cursorBoard(t)
	f := t.TempDir() + "/cursor"
	read := func() sinceDoc {
		var d sinceDoc
		out := captureOutput(t, func() { runAll(t, []string{"log", "--since-file", f, "-j"}) })
		if err := json.Unmarshal([]byte(out), &d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	if d := read(); len(d.Events) != 4 {
		t.Fatalf("first: %+v", d)
	}
	if d := read(); len(d.Events) != 0 {
		t.Fatalf("second: %+v", d)
	}
	runAll(t, []string{"note", "TSK-001", "x"})
	if d := read(); len(d.Events) != 1 {
		t.Fatalf("third: %+v", d)
	}
	if err := run([]string{"log", "--since", "start", "--since-file", f}); err == nil {
		t.Fatal("both flags must error")
	}
}

// The cursor holds the hashes of the window only: bounded however many events the board has.
func TestLogSinceCursorSize(t *testing.T) {
	tn := &Tenant{Slug: "big"}
	base := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 100; i++ {
		it := Item{ID: "TSK-" + strconv.Itoa(i)}
		for j := 0; j < 100; j++ {
			it.Events = append(it.Events, Event{At: base.Add(time.Duration(i*100+j) * 10 * time.Minute).Format(time.RFC3339Nano), Action: "note", Note: strconv.Itoa(j)})
		}
		tn.Items = append(tn.Items, it)
	}
	rows, next, err := logSince([]*Tenant{tn}, "start", 0)
	if err != nil || len(rows) != 10000 {
		t.Fatalf("%d rows, %v", len(rows), err)
	}
	if len(next) >= 8192 { // ~144 events inside the 24h window, 16 hex chars each
		t.Fatalf("cursor is %d chars", len(next))
	}
	if rows, _, _ := logSince([]*Tenant{tn}, next, 0); len(rows) != 0 {
		t.Fatalf("replay: %d", len(rows))
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
	if c, err := decodeCursor(d.Next); err != nil || len(c.Seen) == 0 || len(c.Seen) > len(d.Events) {
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
	addLateEvent(t, path, "cursor-app", "TSK-002", time.Now().Add(time.Minute), "note", "late after archive")
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
		d.Events[i].Key = "<KEY>" // a content hash of the stamp
	}
	b, _ := json.MarshalIndent(d, "", "  ")
	checkGolden(t, "log-since-j", string(b)+"\n")
}
