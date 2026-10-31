package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func logRows(t *testing.T, args ...string) []logRow {
	t.Helper()
	out := captureOutput(t, func() { runAll(t, append([]string{"log", "-j"}, args...)) })
	var rows []logRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return rows
}

func TestLogActionGrep(t *testing.T) {
	needsBoard(t)
	t.Setenv("MW_SESSION", "")
	runAll(t,
		[]string{"decide", "Use CSV for export", "--by", "Sir"},
		[]string{"decide", "Keep the cache", "-p", "TSK-002", "--by", "Sir"},
		[]string{"note", "TSK-001", "csv is mentioned here too"},
	)
	if r := logRows(t, "--action", "decide"); len(r) != 2 {
		t.Fatalf("action decide: %+v", r)
	}
	r := logRows(t, "--action", "decide", "--grep", "CSV")
	if len(r) != 1 || r[0].Action != "decide" || !strings.Contains(r[0].Note, "CSV") {
		t.Fatalf("action+grep (case-insensitive): %+v", r)
	}
	if r := logRows(t, "--grep", "csv"); len(r) != 2 {
		t.Fatalf("grep alone covers notes: %+v", r)
	}
	if r := logRows(t, "--grep", "nothing-like-this"); len(r) != 0 {
		t.Fatalf("no match must print an empty array: %+v", r)
	}
	if err := run([]string{"log", "--grep", "x", "--follow"}); err == nil {
		t.Fatal("--grep is not for --follow")
	}
	if err := run([]string{"log", "--action", "decide", "--since", "start"}); err == nil {
		t.Fatal("--action is not for --since")
	}
}

func TestLogGrepArchived(t *testing.T) {
	needsBoard(t)
	runAll(t,
		[]string{"decide", "Ship the old format", "--by", "Sir"},
		[]string{"archive", "--days", "0"},
	)
	if r := logRows(t, "--action", "decide", "-A"); len(r) != 0 {
		t.Fatalf("without --grep the archive stays out: %+v", r)
	}
	for _, args := range [][]string{{"--grep", "old format"}, {"-A", "--grep", "old format", "--action", "decide"}} {
		if r := logRows(t, args...); len(r) != 1 || r[0].Action != "decide" {
			t.Fatalf("archived decision must be found with %v: %+v", args, r)
		}
	}
	// an archived tenant is searched too under -A
	runAll(t, []string{"tenant", "archive", "needs-app", "--force"})
	if r := logRows(t, "-A", "--grep", "old format"); len(r) != 1 {
		t.Fatalf("-A --grep reaches an archived tenant: %+v", r)
	}
}

func TestLogGrepBadArchiveWarns(t *testing.T) {
	path := needsBoard(t)
	w, _ := load(path)
	if err := os.WriteFile(archivePath(path, w.Tenants[0].Slug), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out string
	errText := captureFile(t, &os.Stderr, func() {
		out = captureOutput(t, func() { runAll(t, []string{"log", "--grep", "x"}) })
	})
	if !strings.Contains(errText, "archive") || strings.Count(strings.TrimSpace(errText), "\n") != 0 {
		t.Fatalf("want a one-line archive warning on stderr, got %q (stdout %q)", errText, out)
	}
}
