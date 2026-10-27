package main

import (
	"strings"
	"testing"
)

// countsBoard: TSK-001 ask; 002 blocked, no dependency; 003 blocked on open 004; 004 ready; 005 blocked on
// done 006; 006 done; 007 ask + blocked, no dependency; 008 inbox draft.
func countsBoard(t *testing.T) *newsModel {
	t.Helper()
	path := inTempDir(t)
	runAll(t, []string{"init", "Counts"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"})
	for _, ti := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		runAll(t, []string{"add", "task", ti, "-S", "ready"})
	}
	runAll(t, []string{"add", "task", "draft"})
	runAll(t,
		[]string{"ask", "TSK-001", "which way?"},
		[]string{"block", "TSK-002", "waiting for Sir"},
		[]string{"block", "TSK-003", "needs d", "--on", "TSK-004"},
		[]string{"start", "TSK-006"}, []string{"done", "TSK-006", "ok", "--no-evidence", "x"},
		[]string{"block", "TSK-005", "was waiting for f", "--on", "TSK-004"},
		[]string{"ask", "TSK-007", "q"}, []string{"block", "TSK-007", "stuck"})
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	// a stale block on a done item (mw now refuses to create one): TSK-005 waits on the done TSK-006
	b5, _ := itemByID(&w.Tenants[0], "TSK-005")
	b5.Events[len(b5.Events)-1].On = "TSK-006"
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	return buildNewsModel(w, newsPart{title: w.Name, tenants: newsJobTenants(w, "")}, nil, newsWindow{full: true})
}

func TestCountsNeedsAndBlocked(t *testing.T) {
	m := countsBoard(t)
	c := m.Now.Counts
	// N: open ask (001, 007) + blocked without a dependency (002) + blocked on a done item (005)
	if c.Needs != 4 {
		t.Errorf("needs %d, want 4: %+v", c.Needs, m.Now.Needs)
	}
	// B: only 003, blocked on the open 004
	if c.Blocked != 1 || len(m.Now.Blocked) != 1 || m.Now.Blocked[0].ID != "TSK-003" {
		t.Errorf("blocked %d, want 1 (TSK-003): %+v", c.Blocked, m.Now.Blocked)
	}
	// no item twice
	seen := map[string]bool{}
	for _, l := range [][]newsItem{m.Now.Needs, m.Now.Blocked} {
		for _, it := range l {
			if seen[it.ID] {
				t.Errorf("%s is counted twice", it.ID)
			}
			seen[it.ID] = true
		}
	}
}

func TestCountsOpenAllExcludeInbox(t *testing.T) {
	c := countsBoard(t).Now.Counts
	if c.Draft != 1 || c.Total != 8 || c.Work != 7 || c.Done != 1 {
		t.Errorf("counts %+v: want 1 draft, total 8, work 7, done 1", c)
	}
	if got := formatStatusLine(c); got != "mw:N4B1:6/7" {
		t.Errorf("status line %q, want mw:N4B1:6/7", got)
	}
}

func TestBlockOnValidation(t *testing.T) {
	inTempDir(t)
	runAll(t, []string{"init", "On"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"}, []string{"add", "task", "a", "-S", "ready"})
	wantErr(t, "cannot wait for itself", "block", "TSK-001", "why", "--on", "TSK-001")
	wantErr(t, "--on", "block", "TSK-001", "why", "--on", "TSK-099")
}

func TestStatusExplainShowsSnippetAndRules(t *testing.T) {
	countsBoard(t)
	out := statusExplain(mustGetwd(), "")
	for _, want := range []string{`"statusLine":{"type":"command","command":"mw status --line --link"}`, "blocked with no dependency", "mw block --on ID", "inbox drafts left out", "counted once"} {
		if !strings.Contains(out, want) {
			t.Errorf("explain lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "mw:N4B1:6/7") {
		t.Errorf("explain line: %s", out)
	}
}
