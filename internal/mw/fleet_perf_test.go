package mw

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// A board the size of a real one: 4 tenants, 300 rows, 100 log entries per row. The targets are Sir's:
// fleet --changed unchanged < 2 s and one line, full fleet < 5 s, node beat < 1 s, merge-file < 2 s per file.
func perfBoard(t *testing.T) string {
	t.Helper()
	path := inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t, []string{"init", "Perf"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"},
		[]string{"tenant", "add", "b", "B"}, []string{"tenant", "add", "c", "C"}, []string{"tenant", "add", "d", "D"},
		[]string{"node", "set", "alpha"})
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-48 * time.Hour).UTC()
	for ti := range w.Tenants {
		tn := &w.Tenants[ti]
		for i := 1; i <= 75; i++ {
			it := Item{ID: fmt.Sprintf("TSK-%03d", i), Kind: "task", Title: fmt.Sprintf("row %d of %s with a title of ordinary length", i, tn.Slug),
				Status: []string{"ready", "doing", "blocked", "done", "ready"}[i%5], Created: base.Format(time.RFC3339), Updated: base.Format(time.RFC3339),
				Node: []string{"", "alpha", "beta", "any"}[i%4], Notes: []string{"a note", "another note"}}
			if it.Status == "doing" {
				it.StartedOn = []string{"alpha", "beta"}[i%2]
			}
			for e := 0; e < 100; e++ {
				it.Events = append(it.Events, Event{At: base.Add(time.Duration(e) * time.Minute).Format(time.RFC3339Nano), Action: "note", By: "s1", Note: fmt.Sprintf("progress entry %d on %s", e, it.ID)})
			}
			tn.Items = append(tn.Items, it)
			tn.Next["TSK"] = i
		}
	}
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	runAll(t, []string{"node", "beat"})
	return path
}

func timed(t *testing.T, name string, limit time.Duration, fn func()) time.Duration {
	t.Helper()
	start := time.Now()
	fn()
	d := time.Since(start)
	t.Logf("%-28s %v (limit %v)", name, d.Round(time.Millisecond), limit)
	if d > limit {
		t.Errorf("%s took %v, over %v", name, d, limit)
	}
	return d
}

func TestFleetPerformance(t *testing.T) {
	path := perfBoard(t)
	captureOutput(t, func() { runAll(t, []string{"fleet", "--changed"}) }) // primes the state
	var out string
	timed(t, "fleet --changed unchanged", 2*time.Second, func() {
		out = captureOutput(t, func() { runAll(t, []string{"fleet", "--changed"}) })
	})
	if strings.TrimSpace(out) != "no change" || strings.Count(out, "\n") != 1 {
		t.Fatalf("want exactly one line, got %q", out)
	}
	timed(t, "fleet (full)", 5*time.Second, func() {
		out = captureOutput(t, func() { runAll(t, []string{"fleet"}) })
	})
	if !strings.Contains(out, "alpha") {
		t.Fatalf("%s", out)
	}
	timed(t, "fleet -j", 5*time.Second, func() { captureOutput(t, func() { runAll(t, []string{"fleet", "-j"}) }) })
	timed(t, "node beat", time.Second, func() { captureOutput(t, func() { runAll(t, []string{"node", "beat"}) }) })

	// merge driver on the biggest file: each side appends 50 log entries to 100 rows and adds a row
	base, err := os.ReadFile(tenantPath(path, "b"))
	if err != nil {
		t.Fatal(err)
	}
	side := func(who string) []byte {
		var td tenantDoc
		if err := jsonUnmarshal(base, &td); err != nil {
			t.Fatal(err)
		}
		at := time.Now().UTC()
		for i := range td.Items {
			if i%3 == 0 {
				for e := 0; e < 50; e++ {
					td.Items[i].Events = append(td.Items[i].Events, Event{At: at.Add(time.Duration(e) * time.Second).Format(time.RFC3339Nano), Action: "note", By: who, Note: fmt.Sprintf("%s entry %d", who, e)})
				}
				td.Items[i].Updated = at.Format(time.RFC3339)
			}
		}
		td.Items = append(td.Items, Item{ID: "TSK-900", Kind: "task", Title: "added by " + who, Status: "ready", Created: at.Format(time.RFC3339), Updated: at.Format(time.RFC3339), Events: []Event{{At: at.Format(time.RFC3339Nano), Action: "created", By: who}}})
		return mustMarshal(td)
	}
	ours, theirs := side("ours"), side("theirs")
	t.Logf("merge input: base %d KB", len(base)/1024)
	var merged []byte
	timed(t, "merge-file (one tenant file)", 2*time.Second, func() {
		var err error
		if merged, err = mergeBoardFiles(base, ours, theirs); err != nil {
			t.Fatal(err)
		}
	})
	var td tenantDoc
	if err := jsonUnmarshal(merged, &td); err != nil || len(td.Items) != 76+1 {
		t.Fatalf("merged rows: %d %v", len(td.Items), err)
	}
	dir := t.TempDir()
	files := map[string][]byte{"base": base, "ours": ours, "theirs": theirs}
	for k, v := range files {
		if err := os.WriteFile(dir+"/"+k, v, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	timed(t, "mw merge-file (command)", 2*time.Second, func() {
		runAll(t, []string{"merge-file", dir + "/base", dir + "/ours", dir + "/theirs"})
	})
}
