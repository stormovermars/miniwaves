package mw

import (
	"os"
	"strings"
	"testing"
)

func TestGlobalFlagsAnyPosition(t *testing.T) {
	inTempDir(t)
	runAll(t, []string{"init", "A"}, []string{"tenant", "add", "b"}, []string{"add", "task", "In b", "-t", "b"})
	a := captureOutput(t, func() { runAll(t, []string{"-t", "b", "list"}) })
	b := captureOutput(t, func() { runAll(t, []string{"list", "-t", "b"}) })
	if a != b || !strings.Contains(a, "In b") {
		t.Fatalf("leading -t differs:\n%s\n---\n%s", a, b)
	}
	if out := captureOutput(t, func() { runAll(t, []string{"-j", "-t", "b", "list"}) }); !strings.Contains(out, `"id"`) {
		t.Fatalf("leading -j: %s", out)
	}
	wantErr(t, "unknown command", "-t", "b", "nosuch")
	wantErr(t, "unknown command", "nosuch")
}

func TestVersionFlags(t *testing.T) {
	inTempDir(t)
	want := captureOutput(t, func() { runAll(t, []string{"version"}) })
	for _, f := range []string{"--version", "-V"} {
		if got := captureOutput(t, func() { runAll(t, []string{f}) }); got != want {
			t.Errorf("%s = %q, want %q", f, got, want)
		}
	}
}

func TestHelpWritesNothing(t *testing.T) {
	for _, args := range [][]string{{"init", "--help"}, {"dirs", "init", "--help"}, {"init", "-h"}, {"tenant", "add", "--help"}, {"add", "--help"}, {"--help"}, {"init", "--kit", "coordinated", "--help"}} {
		inTempDir(t)
		captureOutput(t, func() { runAll(t, args) })
		if ents, _ := os.ReadDir("."); len(ents) != 0 {
			t.Errorf("%v wrote %d file(s)", args, len(ents))
		}
	}
}

func TestQualifiedIDs(t *testing.T) {
	inTempDir(t)
	runAll(t, []string{"init", "A"}, []string{"tenant", "add", "b"}, []string{"add", "task", "In b", "-t", "b"})
	out := captureOutput(t, func() { runAll(t, []string{"show", "b:TSK-001"}) })
	if !strings.Contains(out, "In b") {
		t.Fatalf("qualified show:\n%s", out)
	}
	runAll(t, []string{"note", "b:TSK-001", "hello"})
	if out := captureOutput(t, func() { runAll(t, []string{"show", "TSK-001", "-t", "b"}) }); !strings.Contains(out, "hello") {
		t.Fatalf("qualified note missing:\n%s", out)
	}
}

func TestCheckStaleParents(t *testing.T) {
	inTempDir(t)
	runAll(t, []string{"init", "A"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"})
	runAll(t, []string{"add", "story", "S"}, []string{"add", "task", "C", "-p", "STR-001"}, []string{"start", "TSK-001", "-s", "x"}, []string{"ready", "STR-001"})
	out := captureOutput(t, func() { runAll(t, []string{"check"}) })
	if !strings.Contains(out, "STR-001") || !strings.Contains(out, "TSK-001") {
		t.Fatalf("stale parent not flagged:\n%s", out)
	}
}

func TestShowCitedEvidence(t *testing.T) {
	inTempDir(t)
	runAll(t, []string{"init", "A"}, []string{"add", "task", "T"}, []string{"note", "TSK-001", "commit a1b2c3d4 and go test ./... pass"})
	out := captureOutput(t, func() { runAll(t, []string{"show", "TSK-001"}) })
	if !strings.Contains(out, "notes cite 1 commits, 1 test runs") {
		t.Fatalf("cited evidence missing:\n%s", out)
	}
}

func TestTenantListOneLine(t *testing.T) {
	inTempDir(t)
	runAll(t, []string{"init", "A"}, []string{"tenant", "add", "b"}, []string{"tenant", "tag", "b", "+x", "+y"})
	out := captureOutput(t, func() { runAll(t, []string{"tenant", "list"}) })
	if n := len(strings.Split(strings.TrimSpace(out), "\n")); n != 2 || !strings.Contains(out, "  x,y") {
		t.Fatalf("want 2 lines:\n%s", out)
	}
}

func TestAmbiguousFolderWarns(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "A"}, []string{"tenant", "add", "b"})
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := range w.Tenants {
		w.Tenants[i].Path = mustGetwd()
	}
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	if e := captureStderr(t, func() { runAll(t, []string{"add", "task", "X"}) }); !strings.Contains(e, "maps to 2 tenants") {
		t.Fatalf("no warning on write:\n%s", e)
	}
	if e := captureStderr(t, func() { runAll(t, []string{"add", "task", "Y", "-t", "a"}) }); strings.Contains(e, "maps to") {
		t.Fatalf("warned despite -t:\n%s", e)
	}
	if e := captureStderr(t, func() { runAll(t, []string{"list"}) }); strings.Contains(e, "maps to") {
		t.Fatalf("warned on read:\n%s", e)
	}
}
