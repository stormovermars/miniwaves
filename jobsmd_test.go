package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBoardMDRender(t *testing.T) {
	gp, err := filepath.Abs(filepath.Join("testdata", "golden", "board-md.md"))
	if err != nil {
		t.Fatal(err)
	}
	path := newsBoard(t)
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	p := newsPart{job: "tiny", title: "Tiny <job>", tenants: newsJobTenants(w, "tiny")}
	m := buildNewsModel(w, p, nil, newsWindow{full: true})
	got := "# status\n\n" + jobsMDSummary("tiny", m) + "\n\n" + renderJobsMD(jobsMDInput{id: "tiny", title: p.title, m: m, tenants: p.tenants, newsLink: "status.md"}, time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC))
	if os.Getenv("GOLDEN_UPDATE") == "1" {
		if err := os.WriteFile(gp, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(gp)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Errorf("board --md golden changed:\n--- want\n%s\n--- got\n%s", want, got)
	}
	for _, s := range []string{"### Needs you", "### In progress", "### Ready", "### Recently done", "### Milestones", "! tiny"} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q in:\n%s", s, got)
		}
	}
	if strings.Contains(got, "<script>") || strings.Contains(got, "<b>") {
		t.Errorf("raw HTML leaked:\n%s", got)
	}
}

func TestBoardMDWriteOnlyWhenChanged(t *testing.T) {
	path := newsBoard(t)
	root := filepath.Dir(path)
	target := filepath.Join(root, "docs", "news", "tiny", "jobs.md")
	out1 := captureOutput(t, func() { runAll(t, []string{"board", "--md"}) })
	if !strings.HasPrefix(out1, "wrote ") || !strings.Contains(out1, "jobs.md") {
		t.Fatalf("first run: %q", out1)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal(err)
	}
	out2 := captureOutput(t, func() { runAll(t, []string{"board", "--md"}) })
	if !strings.HasPrefix(out2, "unchanged ") {
		t.Fatalf("second run: %q", out2)
	}
	runAll(t, []string{"add", "task", "New thing", "-S", "ready", "-t", "doc"}, []string{"start", "TSK-002", "-s", "zed", "-t", "doc"})
	out3 := captureOutput(t, func() { runAll(t, []string{"board", "--md"}) })
	if !strings.HasPrefix(out3, "wrote ") {
		t.Fatalf("after change: %q", out3)
	}
	std := captureOutput(t, func() { runAll(t, []string{"board", "--md", "-o", "-"}) })
	if !strings.HasPrefix(std, "# ") || !strings.Contains(std, "New thing") {
		t.Fatalf("stdout form: %q", std)
	}
	if err := run([]string{"board", "--md", "--html"}); err == nil {
		t.Fatal("--md with --html should fail")
	}
	if err := run([]string{"board", "--md", "--job", "nope"}); err == nil {
		t.Fatal("unknown job should fail")
	}
}

func TestBoardMDNoJob(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "Plain"}, []string{"add", "task", "A", "-S", "ready"})
	captureOutput(t, func() { runAll(t, []string{"board", "--md"}) })
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "docs", "news", "jobs.md")); err != nil {
		t.Fatal(err)
	}
}

func TestMDWhen(t *testing.T) {
	now := time.Date(2026, 10, 25, 9, 0, 0, 0, time.Local)
	if got := mdWhen(time.Date(2026, 10, 25, 7, 5, 0, 0, time.Local), now); got != "07:05" {
		t.Errorf("today: %q", got)
	}
	if got := mdWhen(time.Date(2026, 10, 23, 12, 10, 0, 0, time.Local), now); got != "Oct 23 12:10" {
		t.Errorf("older: %q", got)
	}
}

func TestBoardMDWaiting(t *testing.T) {
	old := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = old })
	gp, err := filepath.Abs(filepath.Join("testdata", "golden", "board-md-waiting.md"))
	if err != nil {
		t.Fatal(err)
	}
	path := inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t,
		[]string{"init", "Wait app"},
		[]string{"jobs", "enable", "w", "-t", "wait-app"},
		[]string{"add", "task", "Base work", "-S", "ready", "-t", "wait-app"},
		[]string{"add", "task", "Needs base", "-S", "ready", "-t", "wait-app"},
		[]string{"add", "task", "Needs keys", "-S", "ready", "-t", "wait-app"},
		[]string{"start", "TSK-002", "-s", "bob", "-t", "wait-app"},
		[]string{"block", "TSK-002", "base first", "--on", "TSK-001", "-t", "wait-app"},
		[]string{"start", "TSK-003", "-s", "bob", "-t", "wait-app"},
		[]string{"block", "TSK-003", "need the keys", "-t", "wait-app"},
	)
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	for ti := range w.Tenants {
		for ii := range w.Tenants[ti].Items {
			for ei := range w.Tenants[ti].Items[ii].Events {
				at = at.Add(7 * time.Minute)
				w.Tenants[ti].Items[ii].Events[ei].At = at.Format(time.RFC3339)
			}
		}
	}
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	w, _ = load(path)
	p := newsPart{job: "w", title: "Wait", tenants: newsJobTenants(w, "w")}
	m := buildNewsModel(w, p, nil, newsWindow{full: true})
	got := renderJobsMD(jobsMDInput{id: "w", title: p.title, m: m, tenants: p.tenants}, time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC))
	if os.Getenv("GOLDEN_UPDATE") == "1" {
		if err := os.WriteFile(gp, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(gp)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Errorf("waiting golden changed:\n--- want\n%s\n--- got\n%s", want, got)
	}
	for _, s := range []string{"### Needs you", "### Waiting", "### Ready", "waits on TSK-001 (Base work)"} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q in:\n%s", s, got)
		}
	}
	if h := renderHTML(w); !strings.Contains(h, "waits on TSK-001 (Base work)") || !strings.Contains(h, "waiting (1)") || !strings.Contains(h, "needs you (1)") {
		t.Errorf("html lacks waiting groups:\n%s", h)
	}
}
