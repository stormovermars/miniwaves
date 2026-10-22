package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// newsTenantsBoard builds a job "big" of n tenants with perTenant items each and asks spread over the tenants
// (temp dir only). Slugs are long on purpose.
func newsTenantsBoard(t *testing.T, n, perTenant, asks int) string {
	t.Helper()
	path := inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t, []string{"init", "Many"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"}, []string{"jobs", "enable", "big", "-t", "many"})
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	proto := w.Tenants[0]
	w.Tenants = w.Tenants[:0]
	base := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	stamp := 0
	at := func() string { stamp++; return base.Add(time.Duration(stamp) * time.Second).Format(time.RFC3339) }
	statuses := []string{"done", "doing", "ready", "blocked", "inbox", "done", "ready", "doing", "done", "ready"}
	for i := 0; i < n; i++ {
		tn := proto
		tn.Slug = fmt.Sprintf("tenant-with-a-rather-long-slug-name-%04d", i)
		tn.Name = fmt.Sprintf("Tenant number %d with a long display name for the row", i)
		tn.Job = "big"
		tn.Next = map[string]int{"TSK": perTenant}
		tn.Items = nil
		for k := 0; k < perTenant; k++ {
			st := statuses[k%len(statuses)]
			it := Item{ID: fmt.Sprintf("TSK-%03d", k+1), Kind: "task", Title: fmt.Sprintf("Item %d of tenant %d %s", k, i, strings.Repeat("x", 80)), Status: st, Created: at(), Updated: at()}
			it.Events = append(it.Events, Event{At: at(), Action: "created"}, Event{At: at(), Action: st, By: "s"})
			if st == "done" {
				it.Judged = "ungated"
			}
			if k == 0 && i%(max(n/asks, 1)) == 0 && asks > 0 {
				it.Events = append(it.Events, Event{At: at(), Action: "ask", By: "s", Note: "question?"})
			}
			tn.Items = append(tn.Items, it)
		}
		w.Tenants = append(w.Tenants, tn)
	}
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	return path
}

func timedJSON(t *testing.T, bin string, extra ...string) (*newsModel, time.Duration, int) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "m.json")
	args := append([]string{"news", "--job", "big", "--format", "json", "-o", out}, extra...)
	start := time.Now()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "MW_SESSION=", "MW_TENANT=")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, b)
	}
	d := time.Since(start)
	b := mustRead(t, out)
	return decodeModel(t, b), d, len(b)
}

func buildMW(t *testing.T) string {
	bin := filepath.Join(os.TempDir(), fmt.Sprintf("mw-test-%d-%s", os.Getpid(), t.Name()))
	if out, err := srcCmd(bin).CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	t.Cleanup(func() { os.Remove(bin) })
	return bin
}

func TestNewsManyTenants(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("subprocess timing")
	}
	bin := buildMW(t) // before inTempDir moves HOME
	for _, c := range []struct{ tenants, items, asks int }{{100, 30, 20}, {1000, 10, 20}} {
		newsTenantsBoard(t, c.tenants, c.items, c.asks)
		m, d, size := timedJSON(t, bin)
		for r := 0; r < 4; r++ { // the first run is cold (page cache); keep the best
			if _, d2, _ := timedJSON(t, bin); d2 < d {
				d = d2
			}
		}
		t.Logf("%d tenants x %d items: %v, %d bytes", c.tenants, c.items, d, size)
		if size > newsModelMax || d > 1500*time.Millisecond {
			t.Errorf("%d tenants: %v, %d bytes", c.tenants, d, size)
		}
		if len(m.Now.Tenants) != c.tenants || m.Now.More["tenants"] != 0 || m.Now.Caps["tenants"] != 2000 {
			t.Fatalf("tenant rows: %d more %v", len(m.Now.Tenants), m.Now.More)
		}
		// the kanban/agile fields at scale
		if m.Now.Backlog.Count != c.tenants*c.items/10*4 || len(m.Now.Backlog.Items) != 50 || m.Now.More["backlog"] != c.tenants*c.items/10*4-50 || m.Now.Counts.Draft != c.tenants*c.items/10 {
			// per 10 items: 3 ready (+ the doing/blocked/done mix) and 1 draft; asks sit on item 0 (done)
			t.Errorf("backlog: count %d items %d more %d draft %d", m.Now.Backlog.Count, len(m.Now.Backlog.Items), m.Now.More["backlog"], m.Now.Counts.Draft)
		}
		if len(m.Now.RecentDone) != 10 || m.Now.Sprint != nil {
			t.Errorf("recent_done %d sprint %v", len(m.Now.RecentDone), m.Now.Sprint)
		}
		// asks from every tenant, in one run
		if m.Now.Counts.Needs != c.asks {
			t.Errorf("needs %d, want %d", m.Now.Counts.Needs, c.asks)
		}
		seen := map[string]bool{}
		for _, it := range m.Now.Needs {
			seen[it.Tenant] = true
		}
		if len(seen) != c.asks {
			t.Errorf("asks came from %d tenants", len(seen))
		}
		// sorted by attention: tenants with an ask first, then by last change (newest first), slug
		for i, r := range m.Now.Tenants {
			if i < c.asks && r.Counts.Needs != 1 {
				t.Fatalf("row %d: %+v", i, r)
			}
			if i > 0 {
				p := m.Now.Tenants[i-1]
				if p.Counts.Needs < r.Counts.Needs || p.Counts.Needs == r.Counts.Needs && p.Counts.Blocked < r.Counts.Blocked {
					t.Fatalf("not sorted by attention at %d", i)
				}
			}
			if r.Slug == "" || r.Counts.Total != c.items {
				t.Fatalf("row %d: %+v", i, r)
			}
		}
		// the row counts add up to the job counts
		sum := newsCounts{}
		for _, r := range m.Now.Tenants {
			sum.Needs += r.Counts.Needs
			sum.Blocked += r.Counts.Blocked
			sum.Doing += r.Counts.Doing
			sum.Ready += r.Counts.Ready
			sum.Done += r.Counts.Done
			sum.Total += r.Counts.Total
		}
		jc := m.Now.Counts
		if sum.Needs != jc.Needs || sum.Blocked != jc.Blocked || sum.Doing != jc.Doing || sum.Ready != jc.Ready || sum.Done != jc.Done || sum.Total != jc.Total {
			t.Errorf("rows %+v vs job %+v", sum, jc)
		}
		// --tenant: the same model for one tenant, the window job-wide
		one := m.Now.Tenants[0].Slug
		f, fd, _ := timedJSON(t, bin, "--tenant", one)
		t.Logf("--tenant: %v", fd)
		if len(f.Now.Tenants) != 1 || f.Now.Tenants[0].Slug != one || f.Release.At != m.Release.At || f.Now.Counts.Total != c.items {
			t.Errorf("filtered model: %+v", f.Now.Counts)
		}
		for _, it := range f.Now.Needs {
			if it.Tenant != one {
				t.Errorf("foreign item %+v", it)
			}
		}
		// bulletins with long tenant slugs stay within the limits
		for _, args := range [][]string{{"--format", "text"}, {"-o", "-"}, {"--format", "html", "-o", "-"}} {
			cmd := exec.Command(bin, append([]string{"news", "--job", "big"}, args...)...)
			cmd.Env = append(os.Environ(), "MW_SESSION=", "MW_TENANT=")
			b, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			out := string(b)
			if w, l := maxWidth(out); w > 70 {
				t.Errorf("%v: %d cells: %q", args, w, l)
			}
			if !strings.Contains(out, "<pre>") && len([]rune(out)) > bulletinLimit {
				t.Errorf("%v: %d characters", args, len([]rune(out)))
			}
		}
	}
}

func TestNewsTenantFlagRules(t *testing.T) {
	newsBoard(t)
	wantErr(t, "--tenant is only for --format json", "news", "--job", "tiny", "-t", "doc")
	wantErr(t, "cannot be combined with --release", "news", "--job", "tiny", "--format", "json", "-t", "doc", "--release")
	wantErr(t, "not a visible tenant", "news", "--job", "tiny", "--format", "json", "-t", "nope")
	m := decodeModel(t, newsOut(t, "--job", "tiny", "--format", "json", "-t", "doc"))
	if len(m.Now.Tenants) != 1 || m.Now.Tenants[0].Slug != "doc" || m.Now.Counts.Needs != 1 || len(m.Now.Needs) != 1 || m.Now.Counts.Blocked != 0 {
		t.Fatalf("doc only: %+v %+v", m.Now.Tenants, m.Now.Counts)
	}
	all := decodeModel(t, newsOut(t, "--job", "tiny", "--format", "json"))
	if len(all.Now.Tenants) != 2 || all.Now.Tenants[0].Slug != "doc" { // doc has the open ask
		t.Fatalf("tenant rows: %+v", all.Now.Tenants)
	}
	if all.Release.At != m.Release.At {
		t.Fatal("the window moved with the filter")
	}
}

func TestNewsGuardTrimsTenantsLast(t *testing.T) {
	m := &newsModel{Version: 1, Now: newsNow{More: map[string]int{}}, Happened: newsHappened{More: map[string]int{}, Counts: map[string]int{}}}
	for i := 0; i < 2000; i++ {
		m.Now.Tenants = append(m.Now.Tenants, newsTenant{Slug: fmt.Sprintf("t%04d", i), Name: "n", LastChange: "2026-01-01T00:00:00Z"})
	}
	big := strings.Repeat("x", 190)
	for i := 0; i < 20000; i++ {
		m.Happened.Done = append(m.Happened.Done, newsItem{Tenant: "t", ID: "i", Title: big, Note: big, Since: "2026-01-01T00:00:00Z", By: "b"})
	}
	newsGuardSize(m)
	if len(m.Now.Tenants) != 2000 || m.Now.More["tenants"] != 0 || m.Happened.More["done"] == 0 {
		t.Fatalf("items should give way first: tenants %d", len(m.Now.Tenants))
	}
	// only when the items cannot shrink further are tenant rows dropped
	for i := range m.Now.Tenants {
		m.Now.Tenants[i].Name = strings.Repeat("n", 40)
		m.Now.Tenants[i].Slug = strings.Repeat("s", 40)
	}
	for i := 0; i < 8; i++ {
		m.Summary += big
	}
	big2 := &newsModel{Version: 1, Now: newsNow{More: map[string]int{}}, Happened: newsHappened{More: map[string]int{}, Counts: map[string]int{}}}
	for i := 0; i < 5000; i++ {
		big2.Now.Tenants = append(big2.Now.Tenants, newsTenant{Slug: strings.Repeat("s", 40), Name: strings.Repeat("n", 40), LastChange: "2026-01-01T00:00:00Z"})
	}
	newsGuardSize(big2)
	b, _ := newsModelJSON(big2)
	if len(b) > newsModelMax || big2.Now.More["tenants"] == 0 {
		t.Fatalf("tenant rows not trimmed: %d bytes, more %v", len(b), big2.Now.More)
	}
}
