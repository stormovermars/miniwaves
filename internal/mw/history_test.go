package mw

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func histBoard(t *testing.T) string {
	t.Helper()
	path := inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t, []string{"init", "Hist"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"})
	return path
}

func TestHistoryAddQueryLast(t *testing.T) {
	path := histBoard(t)
	if err := run([]string{"history", "add", "push", "app", "abc123"}); err == nil || !strings.Contains(err.Error(), "mw node set") {
		t.Fatalf("add without a node: %v", err)
	}
	runAll(t, []string{"node", "set", "alpha"},
		[]string{"history", "add", "push", "app", "abc123", "--note", "first"},
		[]string{"history", "add", "release", "app", "v1.0.0", "--version", "1.0.0", "--target", "github", "--row", "tsk-001"})
	// a second node's file, written by hand (another computer)
	other := historyLine{At: "2099-01-01T00:00:00Z", Node: "beta", Kind: "upload", Repo: "lib", Ref: "v2", Version: "2.0"}
	b, _ := json.Marshal(other)
	if err := os.WriteFile(historyPath(path, "beta"), append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	var all []historyLine
	out := captureOutput(t, func() { runAll(t, []string{"history", "query", "-j"}) })
	if err := json.Unmarshal([]byte(out), &all); err != nil || len(all) != 3 || all[0].Node != "beta" || all[2].Kind != "push" && all[2].Kind != "release" {
		t.Fatalf("%v %s", err, out)
	}
	out = captureOutput(t, func() {
		runAll(t, []string{"history", "query", "--kind", "release", "--repo", "app", "--version", "1.0.0", "-j"})
	})
	all = nil
	if err := json.Unmarshal([]byte(out), &all); err != nil || len(all) != 1 || all[0].Row != "TSK-001" || all[0].Target != "github" {
		t.Fatalf("%v %s", err, out)
	}
	out = captureOutput(t, func() { runAll(t, []string{"history", "query", "--since", "2098-01-01", "--node", "beta", "-n", "5"}) })
	if !strings.Contains(out, "lib v2") || strings.Contains(out, "abc123") {
		t.Fatalf("%s", out)
	}
	out = captureOutput(t, func() { runAll(t, []string{"history", "last", "-j"}) })
	all = nil
	_ = json.Unmarshal([]byte(out), &all)
	if len(all) != 2 { // newest line per repo: app, lib
		t.Fatalf("last: %s", out)
	}
	out = captureOutput(t, func() { runAll(t, []string{"history", "last", "app"}) })
	if strings.Count(out, "\n") != 1 || !strings.Contains(out, "release app v1.0.0") {
		t.Fatalf("last app: %q", out)
	}
	// --changed
	first := captureOutput(t, func() { runAll(t, []string{"history", "query", "--changed"}) })
	if strings.TrimSpace(first) == "no change" {
		t.Fatal("first look said no change")
	}
	if out = strings.TrimSpace(captureOutput(t, func() { runAll(t, []string{"history", "query", "--changed"}) })); out != "no change" {
		t.Fatalf("%q", out)
	}
	runAll(t, []string{"history", "add", "publish", "app", "v1.0.0", "--target", "store"})
	if out = strings.TrimSpace(captureOutput(t, func() { runAll(t, []string{"history", "query", "--changed"}) })); out == "no change" {
		t.Fatal("a new line went unnoticed")
	}
}

func TestHistoryRefusesSecretsAndBadInput(t *testing.T) {
	histBoard(t)
	runAll(t, []string{"node", "set", "alpha"})
	for _, bad := range [][]string{
		{"history", "add", "push", "app", "abc", "--note", "token=abc123"},
		{"history", "add", "push", "app", "abc", "--note", "ghp_abcdefghijklmnopqrstuvwxyz0123456789"},
		{"history", "add", "push", "app", "abc", "--target", "password: hunter2"},
		{"history", "add", "ship", "app", "abc"},
		{"history", "add", "push", "app", "a b"},
		{"history", "add", "push", "app"},
		{"history", "query", "--since", "yesterday"},
	} {
		if err := run(bad); err == nil {
			t.Fatalf("%v accepted", bad)
		}
	}
}

func TestHistoryDoneWarnsOnReleaseRow(t *testing.T) {
	histBoard(t)
	runAll(t, []string{"node", "set", "alpha"}, []string{"add", "task", "Release 1.2", "-S", "ready"}, []string{"add", "task", "Fix a thing", "-S", "ready"})
	stderr := func(args ...string) string {
		return captureFile(t, &os.Stderr, func() { runAll(t, args) })
	}
	if s := stderr("done", "TSK-002", "fixed", "--no-evidence", "t"); strings.Contains(s, "ledger") {
		t.Fatalf("a plain row warned: %s", s)
	}
	if s := stderr("done", "TSK-001", "shipped", "--no-evidence", "t"); !strings.Contains(s, "no line for it") {
		t.Fatalf("no warning: %q", s)
	}
	runAll(t, []string{"reopen", "TSK-001"}, []string{"history", "add", "release", "app", "v1.2", "--row", "TSK-001"})
	if s := stderr("done", "TSK-001", "shipped", "--no-evidence", "t"); strings.Contains(s, "ledger") {
		t.Fatalf("warned although the ledger has the line: %s", s)
	}
}

func TestHistoryMergeUnion(t *testing.T) {
	a := `{"at":"2026-01-02T00:00:00Z","node":"a","kind":"push","repo":"r","ref":"1"}` + "\n"
	b := `{"at":"2026-01-01T00:00:00Z","node":"a","kind":"push","repo":"r","ref":"0"}` + "\n" + strings.TrimSpace(a) + "\n"
	got := string(mergeHistoryFiles([]byte(a), []byte(b)))
	if strings.Count(got, "\n") != 2 || !strings.HasPrefix(got, `{"at":"2026-01-01`) {
		t.Fatalf("%q", got)
	}
}
