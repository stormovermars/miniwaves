package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// newsBoard builds a fixed two-tenant job board with hostile titles and rewrites every event stamp to a
// fixed, increasing time so the output is reproducible (and local time is UTC).
func newsBoard(t *testing.T) string {
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
	return path
}

func TestNewsGoldens(t *testing.T) {
	newsBoard(t)
	for _, f := range []string{"md", "html"} {
		got := ""
		got = captureOutput(t, func() { runAll(t, []string{"news", "--job", "tiny", "--format", f, "-o", "-"}) })
		got = strings.ReplaceAll(got, time.Now().Format("2006-01-02 15:04"), "<NOW>")
		checkGolden(t, "news-"+f, got)
		if strings.Contains(got, "<script>") || strings.Contains(got, "\r") {
			t.Fatalf("%s: raw <script> or CR in output", f)
		}
	}
}

func TestNewsDefaultPathAndUnchanged(t *testing.T) {
	path := newsBoard(t)
	root := filepath.Dir(path)
	out := captureOutput(t, func() { runAll(t, []string{"news"}) })
	file := filepath.Join(root, "docs", "news", "status.md")
	b, err := os.ReadFile(file)
	if err != nil || !strings.HasPrefix(out, "wrote ") {
		t.Fatalf("first run: %q %v", out, err)
	}
	if strings.Contains(string(b), "first run") == false {
		t.Fatalf("first run marker missing:\n%s", b)
	}
	if out = captureOutput(t, func() { runAll(t, []string{"news"}) }); !strings.HasPrefix(out, "unchanged ") {
		t.Fatalf("second run: %q", out)
	}
	// a board change writes again, and reports the moved events
	runAll(t, []string{"note", "TSK-002", "progress", "-t", "news-app"})
	if out = captureOutput(t, func() { runAll(t, []string{"news"}) }); !strings.HasPrefix(out, "wrote ") {
		t.Fatalf("after change: %q", out)
	}
	if b, _ = os.ReadFile(file); !strings.Contains(string(b), "moved since the last run") || !strings.Contains(string(b), "note: TSK-002") {
		t.Fatalf("change not shown:\n%s", b)
	}
	// deleting the page rewrites it even though the hash matches
	_ = os.Remove(file)
	if out = captureOutput(t, func() { runAll(t, []string{"news"}) }); !strings.HasPrefix(out, "wrote ") {
		t.Fatalf("after delete: %q", out)
	}
	// the hash ignores the stamp lines
	a := newsFill("x\x00stamp\x00 y\x00moved\x00", time.Now(), 1, true, true)
	if a != "x y" {
		t.Fatalf("hash form: %q", a)
	}
}

func TestNewsEscapingMD(t *testing.T) {
	if got := newsMD("a<b>&`c`|d\\e\nf"); got != "a&lt;b&gt;&amp;\\`c\\`\\|d\\\\e f" {
		t.Fatalf("escape: %q", got)
	}
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

func TestNewsMDCap(t *testing.T) {
	path := inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t, []string{"init", "Big"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"},
		[]string{"jobs", "enable", "big", "-t", "big"}, []string{"jobs", "set", "big", "summary=sum.txt"})
	long := strings.Repeat("word ", 18)
	w, _ := load(path)
	tn := &w.Tenants[0]
	for i := 0; i < 60; i++ {
		id := fmt.Sprintf("TSK-%03d", i+1)
		tn.Items = append(tn.Items, Item{ID: id, Kind: "task", Title: long + strconv.Itoa(i), Status: "doing", Created: "2026-10-22T09:00:00Z", Updated: "2026-10-22T09:00:00Z",
			Events: []Event{{At: "2026-10-22T09:00:00Z", Action: "ask", By: "s", Note: long}}})
	}
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("sum.txt", []byte(strings.Repeat("S", 30000)), 0644); err != nil {
		t.Fatal(err)
	}
	out := captureOutput(t, func() { runAll(t, []string{"news", "-o", "-"}) })
	if n := len([]rune(out)); n > 9500 {
		t.Fatalf("md is %d chars", n)
	}
	if strings.Contains(out, "## Last events") {
		t.Fatal("events were not trimmed first")
	}
	if lines := strings.Count(out, "\n"); lines > 300 {
		t.Fatalf("%d lines", lines)
	}
	// the summary is capped at 20 KB when read
	s, err := newsSummary(filepath.Dir(path), "sum.txt")
	if err != nil || len(s) > newsSummaryCap+40 || !strings.Contains(s, "[truncated at 20 KB]") {
		t.Fatalf("summary cap: %d %v", len(s), err)
	}
	// html keeps everything within the same list caps
	html := captureOutput(t, func() { runAll(t, []string{"news", "-o", "-", "--format", "html"}) })
	if !strings.Contains(html, "... 35 more") {
		t.Fatalf("html list cap missing")
	}
}

func TestNewsIsAWrite(t *testing.T) {
	if readOnly([]string{"news"}) || readOnly([]string{"news", "--job", "x"}) {
		t.Fatal("news must take the write lock")
	}
}
