package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// newsBoard builds a fixed two-tenant job board with hostile titles and rewrites every event stamp to a
// fixed, increasing time so the output is reproducible (and local time is UTC).
func newsBoard(t *testing.T) string {
	return newsBoardAt(t, time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC))
}

// newsBoardAt is newsBoard with its event stamps starting at base (a release test uses a past base, so
// every event lies before the release time).
func newsBoardAt(t *testing.T, base time.Time) string {
	t.Helper()
	old := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = old })
	path := inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	if err := os.WriteFile("summary.txt", []byte("Harness: 3 pass\n<script>alert(1)</script> | ``` fence\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runAll(t,
		[]string{"init", "News app"},
		[]string{"gate", "level", "trust", "--workspace", "--by", "t"},
		[]string{"tenant", "add", "doc", "Doc"},
		[]string{"jobs", "enable", "tiny", "-t", "news-app", "-t", "doc"},
		[]string{"jobs", "set", "tiny", "title=Tiny <job>", "news.url=https://example.com/news?a=1&b=2", "summary=summary.txt"},
		[]string{"wave", "create", "First | milestone", "-t", "news-app"},
		[]string{"add", "task", "Fix <script>alert(1)</script> now", "-S", "ready", "-t", "news-app"},
		[]string{"add", "task", "Pipe | in `title` & more", "-S", "ready", "-t", "news-app"},
		[]string{"add", "task", "Wait for keys", "-S", "ready", "-t", "news-app"},
		[]string{"add", "task", "Doc work", "-S", "ready", "-t", "doc"},
		[]string{"wave", "add", "WAV-001", "TSK-001", "TSK-002", "-t", "news-app"},
		[]string{"start", "TSK-001", "-s", "alice", "-t", "news-app"},
		[]string{"done", "TSK-001", "checked", "--no-evidence", "none", "-t", "news-app"},
		[]string{"start", "TSK-003", "-s", "bob", "-t", "news-app"},
		[]string{"block", "TSK-003", "needs <b>keys</b>\nsecond line", "-t", "news-app"},
		[]string{"start", "TSK-001", "-s", "carol", "-t", "doc"},
		[]string{"ask", "TSK-001", "Keep | or cut `X`?", "-s", "carol", "-t", "doc"},
		[]string{"decide", "Ship <it>", "--by", "sir", "-b", "cheap", "-t", "doc"},
	)
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	at := base
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
	return path
}

// nowMasked replaces the minute the run started in, and the next one, so a minute boundary cannot flake a golden.
func nowMasked(s string, before time.Time) string {
	s = strings.ReplaceAll(s, before.Format("2006-01-02 15:04"), "<NOW>")
	return strings.ReplaceAll(s, before.Add(time.Minute).Format("2006-01-02 15:04"), "<NOW>")
}

func TestNewsConfinement(t *testing.T) {
	path := newsBoard(t)
	root := filepath.Dir(path)
	w, _ := load(path)
	c := w.Jobs["tiny"]
	c.NewsOut = "../escape/"
	w.Jobs["tiny"] = c
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"news"})
	if err == nil || !strings.Contains(err.Error(), "news.out") {
		t.Fatalf("../ news.out: %v", err)
	}
	if _, e := os.Stat(filepath.Join(filepath.Dir(root), "escape")); e == nil {
		t.Fatal("escape dir was created")
	}
	c.NewsOut = "/abs/out/"
	w.Jobs["tiny"] = c
	_ = save(path, w)
	if err := run([]string{"news"}); err == nil {
		t.Fatal("absolute news.out accepted")
	}
	if runtime.GOOS != "windows" {
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
			t.Fatal(err)
		}
		c.NewsOut = "link/n/"
		w.Jobs["tiny"] = c
		_ = save(path, w)
		if err := run([]string{"news"}); err == nil || !strings.Contains(err.Error(), "leaves the workspace") {
			t.Fatalf("symlink news.out: %v", err)
		}
		if _, e := os.Stat(filepath.Join(outside, "n")); e == nil {
			t.Fatal("wrote through the symlink")
		}
		c.NewsOut = "docs/news/"
		c.Summary = "link/secret.txt"
		w.Jobs["tiny"] = c
		_ = save(path, w)
		if err := run([]string{"news"}); err == nil || !strings.Contains(err.Error(), "summary") {
			t.Fatalf("symlink summary: %v", err)
		}
	}
}

func TestNewsDashOElsewhere(t *testing.T) {
	path := newsBoard(t)
	w, _ := load(path)
	c := w.Jobs["tiny"]
	c.NewsOut = "../escape/" // a bad news.out does not matter when -o is given
	w.Jobs["tiny"] = c
	_ = save(path, w)
	elsewhere := filepath.Join(t.TempDir(), "sub", "n.html")
	if err := run([]string{"news", "-o", elsewhere, "--format", "html"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(elsewhere)
	if err != nil || !strings.Contains(string(b), "<!doctype html>") || strings.Contains(string(b), "<script>") {
		t.Fatalf("-o page: %v\n%s", err, b)
	}
}

func TestNewsIsAWrite(t *testing.T) {
	if readOnly([]string{"news"}) || readOnly([]string{"news", "--job", "x"}) {
		t.Fatal("news must take the write lock")
	}
}
