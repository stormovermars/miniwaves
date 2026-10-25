package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func fullJSON(t *testing.T, extra ...string) *newsModel {
	t.Helper()
	p := filepath.Join(t.TempDir(), "m.json")
	runAll(t, append([]string{"news", "--job", "big", "--format", "json", "-o", p}, extra...))
	return decodeModel(t, mustRead(t, p))
}

// newsOverCaps is a small board (well under 1 MB) with every capped list over its cap.
func newsOverCaps(t *testing.T) {
	t.Helper()
	path := inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t, []string{"init", "Over"}, []string{"gate", "level", "judged", "--workspace", "--by", "t"}, []string{"jobs", "enable", "big", "-t", "over"})
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	tn := &w.Tenants[0]
	n := 0
	stamp := func() string {
		n++
		return time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Second).Format(time.RFC3339)
	}
	add := func(prefix string, count int, kind, status string, evs ...string) {
		for i := 1; i <= count; i++ {
			it := Item{ID: fmt.Sprintf("%s-%04d", prefix, i), Kind: kind, Title: fmt.Sprintf("%s %d", prefix, i), Status: status, Judged: "ungated", Created: stamp(), Updated: stamp()}
			for _, a := range evs {
				it.Events = append(it.Events, Event{At: stamp(), Action: a, By: "s"})
			}
			tn.Items = append(tn.Items, it)
		}
	}
	add("WAV", 230, "wave", "ready")
	add("DON", 120, "task", "done", "created", "done", "accept")
	add("DNG", 60, "task", "doing", "start")
	add("RDY", 25, "task", "ready", "ready")
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
}

func TestNewsFullUncapped(t *testing.T) {
	newsOverCaps(t)
	m := fullJSON(t, "--full")
	if !m.Full || m.Truncated || m.TrimmedRows != 0 {
		t.Fatalf("flags: full=%v truncated=%v trimmed=%d", m.Full, m.Truncated, m.TrimmedRows)
	}
	for _, mm := range []map[string]int{m.Now.More, m.Happened.More, m.Next.More} {
		for k, n := range mm {
			if n != 0 {
				t.Errorf("more[%s]=%d", k, n)
			}
		}
	}
	if m.Now.Sprint != nil && m.Now.Sprint.More != 0 {
		t.Errorf("sprint more %d", m.Now.Sprint.More)
	}
	for name, got := range map[string][2]int{
		"doing": {len(m.Now.Doing), 60}, "done": {len(m.Happened.Done), 120}, "accepted": {len(m.Happened.Accepted), 120},
		"ready": {len(m.Next.Ready), 25}, "milestones": {len(m.Now.Milestones.Open), 230}, "next milestones": {len(m.Next.NextMilestones), 230},
		"backlog": {len(m.Now.Backlog.Items), 25},
	} {
		if got[0] != got[1] {
			t.Errorf("%s: %d rows, want %d", name, got[0], got[1])
		}
	}
	out := newsOut(t, "--job", "big", "--format", "text", "--full")
	if strings.Contains(out, "more (see full board)") || strings.Contains(out, "trimmed:") {
		t.Errorf("text has a more/trimmed line")
	}
	if n := strings.Count(out, "DNG-"); n != 60 {
		t.Errorf("text lists %d doing rows, want 60", n)
	}
	if n := strings.Count(out, "DON-"); n != 240 {
		t.Errorf("text lists %d done rows, want 240 (done + accepted)", n)
	}
	for _, f := range []string{"md", "html"} {
		o := newsOut(t, "--job", "big", "--format", f, "--full", "-o", "-")
		if strings.Contains(o, "more (see full board)") || !strings.Contains(o, "Coming up") {
			t.Errorf("%s: not uncapped", f)
		}
	}
	// without --full the caps are unchanged
	c := fullJSON(t)
	if c.Full || len(c.Now.Doing) != 50 || c.Now.More["doing"] != 10 || len(c.Happened.Done) != 100 || c.Happened.More["done"] != 20 || len(c.Next.Ready) != 10 || c.Next.Caps["ready"] != 10 {
		t.Errorf("capped model changed: full=%v doing=%d more=%d", c.Full, len(c.Now.Doing), c.Now.More["doing"])
	}
	if o := newsOut(t, "--job", "big", "--format", "text"); !strings.Contains(o, "more (see full board)") || strings.Count(o, "DNG-") >= 60 {
		t.Errorf("capped text changed")
	}
}

// the stress board is over 1 MB when uncapped: the guard trims and the bulletin says so once.
func TestNewsFullStressTrimmed(t *testing.T) {
	newsStress(t)
	m := fullJSON(t, "--full")
	if !m.Truncated || m.TrimmedRows == 0 {
		t.Fatalf("expected a trim: truncated=%v trimmed=%d", m.Truncated, m.TrimmedRows)
	}
	out := newsOut(t, "--job", "big", "--format", "text", "--full")
	if n := strings.Count(out, "trimmed:"); n != 1 || !strings.Contains(out, "\ntrimmed: "+strconv.Itoa(m.TrimmedRows)+" rows\n") {
		t.Errorf("trimmed line: %d times, want one with %d rows", n, m.TrimmedRows)
	}
}

func TestNewsFullGuardAndRelease(t *testing.T) {
	m := &newsModel{Version: 1, Full: true, Now: newsNow{More: map[string]int{}}, Happened: newsHappened{More: map[string]int{}, Counts: map[string]int{}}}
	big := strings.Repeat("x", 190)
	for i := 0; i < 20000; i++ {
		m.Happened.Done = append(m.Happened.Done, newsItem{Tenant: "t", ID: strconv.Itoa(i), Title: big, Note: big, Since: "2026-01-01T00:00:00Z", By: "b"})
	}
	total := len(m.Happened.Done)
	newsGuardSize(m)
	b, _ := json.MarshalIndent(m, "", "  ")
	if len(b) > newsModelMax || !m.Truncated || m.TrimmedRows == 0 || m.TrimmedRows != total-len(m.Happened.Done) {
		t.Fatalf("guard: %d bytes truncated=%v trimmed=%d kept=%d", len(b), m.Truncated, m.TrimmedRows, len(m.Happened.Done))
	}
	m.Happened.Counts["done"] = total
	lines := bulletinLines(m, nil)
	want := "trimmed: " + strconv.Itoa(m.TrimmedRows) + " rows"
	n := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "trimmed:") {
			n++
			if l != want {
				t.Errorf("line %q, want %q", l, want)
			}
		}
	}
	if n != 1 {
		t.Errorf("%d trimmed lines", n)
	}
	// not full: the flag is absent from json
	if j, _ := json.Marshal(&newsModel{}); strings.Contains(string(j), "trimmed_rows") || strings.Contains(string(j), `"full"`) {
		t.Errorf("json carries full/trimmed_rows by default: %s", j)
	}
}

func TestNewsFullRefusedWithRelease(t *testing.T) {
	newsBoard(t)
	if err := run([]string{"news", "--job", "tiny", "--release", "--full"}); err == nil || strings.Count(err.Error(), "\n") != 0 || !strings.Contains(err.Error(), "--full cannot be combined with --release") {
		t.Fatalf("err = %v", err)
	}
}

func TestNewsFullNeverRewritesDraft(t *testing.T) {
	newsOverCaps(t)
	for _, f := range []string{"md", "html"} {
		err := run([]string{"news", "--job", "big", "--full", "--format", f})
		if err == nil || !strings.Contains(err.Error(), "--full would rewrite") {
			t.Fatalf("--full --format %s with the default target: %v", f, err)
		}
	}
	if err := run([]string{"news", "--job", "big", "--full"}); err == nil {
		t.Fatal("--full with the default format must be refused")
	}
	if m, _ := filepath.Glob(filepath.Join("docs", "news", "*", "status.*")); len(m) != 0 {
		t.Fatalf("draft written: %v", m)
	}
	// allowed: stdout, a file, json and text
	for _, args := range [][]string{
		{"news", "--job", "big", "--full", "-o", "-"},
		{"news", "--job", "big", "--full", "--format", "json"},
		{"news", "--job", "big", "--full", "--format", "text"},
		{"news", "--job", "big", "--full", "-o", filepath.Join(t.TempDir(), "x.md")},
	} {
		if err := run(args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
}
