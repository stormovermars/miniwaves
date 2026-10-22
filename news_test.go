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

func newsTextOutput(t *testing.T, width string) string {
	t.Helper()
	args := []string{"news", "--job", "tiny", "--format", "text"}
	if width != "" {
		args = append(args, "--width", width)
	}
	return captureOutput(t, func() { runAll(t, args) })
}

func TestNewsTextGolden(t *testing.T) {
	newsBoard(t)
	t.Setenv("COLUMNS", "")
	got := newsTextOutput(t, "80")
	got = strings.ReplaceAll(got, time.Now().Format("2006-01-02 15:04"), "<NOW>")
	checkGolden(t, "news-text", got)
	for _, bad := range []string{"\x1b", "\r", "\\|", "&lt;", "&amp;"} {
		if strings.Contains(got, bad) {
			t.Fatalf("text output contains %q", bad)
		}
	}
	for _, l := range strings.Split(got, "\n") {
		if newsStrWidth(l) > 80 {
			t.Fatalf("line wider than 80: %q", l)
		}
	}
	if _, err := os.Stat("docs"); err == nil {
		t.Fatal("text wrote the news.out default")
	}
}

func TestNewsTextWidthClamp(t *testing.T) {
	for _, c := range []struct {
		flag, env string
		want      int
	}{
		{"", "", 80}, {"10", "", 40}, {"500", "", 160}, {"", "100", 80}, {"60", "100", 60}, {"", "abc", 80},
	} {
		t.Setenv("COLUMNS", c.env)
		if got, err := newsTextWidth(c.flag); err != nil || got != c.want {
			t.Fatalf("%+v: %d %v", c, got, err)
		}
	}
	if _, err := newsTextWidth("x"); err == nil {
		t.Fatal("bad width accepted")
	}
	newsBoard(t)
	for _, w := range []string{"10", "500"} {
		max := map[string]int{"10": 40, "500": 160}[w]
		for _, l := range strings.Split(newsTextOutput(t, w), "\n") {
			if n := newsStrWidth(l); n > max {
				t.Fatalf("width %s: %d cells: %q", w, n, l)
			}
		}
	}
}

func TestNewsTextControlStripped(t *testing.T) {
	if got := newsClean("a\x1b[31mred\x07\x00b‮c\tz\nq", false); got != "a[31mredbc z q" {
		t.Fatalf("clean: %q", got)
	}
	p := newsPart{title: "T\x1b]0;x\x07itle", summary: "s\x1b[2Jum\nline"}
	d := newsData{needs: []newsLine{{"A-1", "t", "evil \x1b[31m title", "d\x9bx"}}}
	out := strings.ReplaceAll(renderNewsText(p, d, 80), newsStampMarker, "now")
	for _, r := range out {
		if r != '\n' && (r < 0x20 || (r >= 0x7f && r <= 0x9f) || r == 0x202e) {
			t.Fatalf("control rune %U in output:\n%q", r, out)
		}
	}
}

func TestNewsTextWideRunes(t *testing.T) {
	if newsStrWidth("日本語") != 6 || newsStrWidth("abc") != 3 || newsStrWidth("é") != 1 {
		t.Fatal("rune widths")
	}
	if got := newsCut("日本語abc", 5); got != "日本" {
		t.Fatalf("cut: %q", got)
	}
	d := newsData{needs: []newsLine{{"A-1", "t", strings.Repeat("日本語", 40), ""}}}
	for _, l := range strings.Split(renderNewsText(newsPart{title: "日本語"}, d, 60), "\n") {
		if newsStrWidth(l) > 60 {
			t.Fatalf("wide line over 60 cells: %q", l)
		}
	}
}

func TestNewsTextTableAlignsSlash(t *testing.T) {
	d := newsData{miles: []newsMile{
		{"WAV-001", "a", "First", "1/2", "ready"}, {"WAV-002", "bb", "Second", "10/100", "doing"}, {"WAV-003", "c", "Third", "100/100", "done"},
	}}
	out := renderNewsText(newsPart{title: "J"}, d, 100)
	slash, bar := -1, -1
	n := 0
	for _, l := range strings.Split(out, "\n") {
		if !strings.HasPrefix(l, "│ WAV-") {
			continue
		}
		n++
		rs := []rune(l)
		si := -1
		for i, r := range rs {
			if r == '/' {
				si = i
			}
		}
		bi := strings.IndexAny(l, "█░")
		bi = len([]rune(l[:bi]))
		if slash == -1 {
			slash, bar = si, bi
		}
		if si != slash || bi != bar {
			t.Fatalf("misaligned (slash %d vs %d, bar %d vs %d): %q", si, slash, bi, bar, l)
		}
	}
	if n != 3 {
		t.Fatalf("rows: %d", n)
	}
}

func TestNewsTextTableNarrow(t *testing.T) {
	mk := func(tenants ...string) newsData {
		var d newsData
		for i, tn := range tenants {
			d.miles = append(d.miles, newsMile{fmt.Sprintf("WAV-%03d", i+1), tn, "A very long milestone title for narrow terminals", "3/4", "ready"})
		}
		return d
	}
	for _, w := range []int{40, 50, 60, 80} {
		for _, d := range []newsData{mk("one", "one"), mk("one", "two")} {
			out := renderNewsText(newsPart{title: "J"}, d, w)
			rows := 0
			for _, l := range strings.Split(out, "\n") {
				if newsStrWidth(l) > w {
					t.Fatalf("w=%d over: %q", w, l)
				}
				if strings.HasPrefix(l, "│ WAV-") || strings.HasPrefix(l, "│ MILE") {
					rows++
					if !strings.Contains(l, "ready") && !strings.Contains(l, "STATE") {
						t.Fatalf("w=%d state lost: %q", w, l)
					}
					if strings.Contains(l, "…") {
						t.Fatalf("ellipsis: %q", l)
					}
				}
			}
			if rows != 3 {
				t.Fatalf("w=%d rows %d:\n%s", w, rows, out)
			}
			if single := d.miles[0].Tenant == d.miles[1].Tenant; single == (strings.Count(strings.SplitN(out[strings.Index(out, "│ MILE"):], "\n", 2)[0], "│") == 6) {
				t.Fatalf("w=%d tenant column wrong (single=%v)", w, single)
			}
		}
	}
}

func TestNewsDescription(t *testing.T) {
	p := newsPart{title: "T", desc: "Hi <b>|x * _y_ \x1b[31m"}
	var d newsData
	md := renderNewsMD(p, d, 10, 1000)
	if !strings.Contains(md, "# T\n_Hi &lt;b&gt;\\|x \\* \\_y\\_ \x1b[31m_\n") {
		t.Fatalf("md: %q", md)
	}
	if !strings.Contains(renderNewsHTML([]newsPart{p}, []newsData{d}, "T"), "<em>Hi &lt;b&gt;|x * _y_ \x1b[31m</em>") {
		t.Fatal("html")
	}
	txt := renderNewsText(p, d, 80)
	if !strings.Contains(txt, "\nHi <b>|x * _y_ [31m\n") {
		t.Fatalf("text: %q", txt)
	}
	if strings.Contains(renderNewsMD(newsPart{title: "T"}, d, 10, 1000), "__") {
		t.Fatal("empty description rendered")
	}
}
