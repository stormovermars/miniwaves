package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewsReadOnlyTable(t *testing.T) {
	for args, want := range map[string]bool{
		"news -o -": true, "news --out -": true, "news --format text": true, "news": false, "news -o x.md": false,
		"news --format text -o x.txt": false, "news --job j -o -": true,
	} {
		if got := readOnly(strings.Fields(args)); got != want {
			t.Errorf("%s: readOnly=%v", args, got)
		}
	}
}

func TestNewsStdoutNoFirstRun(t *testing.T) {
	newsBoard(t)
	os.WriteFile(".hash", []byte("1 x"), 0o644)
	out := captureOutput(t, func() { runAll(t, []string{"news", "--job", "tiny", "-o", "-"}) })
	if strings.Contains(out, "first run") || strings.Contains(out, "moved") {
		t.Fatalf("stdout headline has a moved segment:\n%s", out)
	}
}

func TestNewsJSONNeedsFile(t *testing.T) {
	newsBoard(t)
	wantErr(t, "-j needs a file target", "news", "--job", "tiny", "-o", "-", "-j")
	wantErr(t, "-j needs a file target", "news", "--job", "tiny", "--format", "text", "-j")
}

func TestNewsTwoJobsDefaults(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "Two"}, []string{"tenant", "add", "b", "B"},
		[]string{"jobs", "enable", "j1", "-t", "two"}, []string{"jobs", "enable", "j2", "-t", "b"})
	wantErr(t, "set news.out per job", "news")
	_ = path
	// text and stdout write no file, so they never hit the check
	captureOutput(t, func() { runAll(t, []string{"news", "--format", "text"}, []string{"news", "-o", "-"}) })
	runAll(t, []string{"jobs", "set", "j2", "news.out=docs/news/j2/"})
	captureOutput(t, func() { runAll(t, []string{"news"}) })
	if _, err := os.Stat(filepath.Join("docs", "news", "j2", "status.md")); err != nil {
		t.Fatal(err)
	}
}

func TestTextOutputsStripControls(t *testing.T) {
	newsBoard(t)
	bad := "q \x1b[31mRED\x1b[0m ‮"
	runAll(t, []string{"ask", "TSK-003", bad, "-t", "news-app", "-s", "x"})
	for _, args := range [][]string{{"board"}, {"review"}, {"log"}, {"log", "--since", "start"},
		{"news", "--job", "tiny", "-o", "-"}, {"news", "--job", "tiny", "--format", "html", "-o", "-"}, {"news", "--job", "tiny", "--format", "text"}} {
		out := captureOutput(t, func() { runAll(t, args) })
		if strings.ContainsAny(out, "\x1b‮") {
			t.Errorf("%v leaks a control character:\n%s", args, out)
		}
	}
}
