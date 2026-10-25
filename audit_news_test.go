package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewsReadOnlyTable(t *testing.T) {
	for args, want := range map[string]bool{
		"news -o -": true, "news --out -": true, "news --format text": true, "news --format json": true, "news --job j --release": false, "news --format json --release": false, "news --release -o -": false, "news --format json -o x.json": false, "news": false, "news -o x.md": false,
		"news --format text -o x.txt": false, "news --job j -o -": true,
	} {
		if got := readOnly(strings.Fields(args)); got != want {
			t.Errorf("%s: readOnly=%v", args, got)
		}
	}
}

func TestNewsJSONNeedsFile(t *testing.T) {
	newsBoard(t)
	wantErr(t, "-j needs a file target", "news", "--job", "tiny", "-o", "-", "-j")
	wantErr(t, "-j needs a file target", "news", "--job", "tiny", "--format", "text", "-j")
}

func TestNewsTwoJobsDefaults(t *testing.T) {
	inTempDir(t)
	runAll(t, []string{"init", "Two"}, []string{"tenant", "add", "b", "B"},
		[]string{"jobs", "enable", "j1", "-t", "two"}, []string{"jobs", "enable", "j2", "-t", "b"})
	// each job has its own default folder: nothing is shared, nothing fails
	captureOutput(t, func() { runAll(t, []string{"news"}, []string{"board", "--md"}) })
	for _, j := range []string{"j1", "j2"} {
		for _, f := range []string{"status.md", "jobs.md"} {
			if _, err := os.Stat(filepath.Join("docs", "news", j, f)); err != nil {
				t.Fatal(err)
			}
		}
	}
	// two jobs pointed at one file give one combined document, not an error
	runAll(t, []string{"jobs", "set", "j1", "news.out=docs/shared/"}, []string{"jobs", "set", "j2", "news.out=docs/shared/"})
	captureOutput(t, func() { runAll(t, []string{"news"}, []string{"board", "--md"}) })
	for _, f := range []string{"status.md", "jobs.md"} {
		b, err := os.ReadFile(filepath.Join("docs", "shared", f))
		if err != nil {
			t.Fatal(err)
		}
		if f == "jobs.md" && (!strings.Contains(string(b), "## j1") && !strings.Contains(string(b), "## j2")) {
			t.Fatalf("combined jobs.md lacks the jobs:\n%s", b)
		}
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
