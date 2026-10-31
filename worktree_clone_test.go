package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// cloneOf clones e.main to root/name on branch name and returns the clone root.
func cloneOf(t *testing.T, e wtEnv, name string) string {
	t.Helper()
	c := filepath.Join(e.root, name)
	gitW(t, e.root, "clone", "-q", e.main, c)
	gitW(t, c, "checkout", "-q", "-b", name)
	return c
}

// mwErr runs mw in-process and returns the exit code and what went to stderr.
func mwErr(t *testing.T, args ...string) (int, string) {
	t.Helper()
	code := -1
	errs := captureFile(t, &os.Stderr, func() { captureOutput(t, func() { code = mainExit(args, os.Stderr) }) })
	return code, errs
}

// MW_MAIN test 1: a clone with MW_MAIN writes through to main (claims visible there), keeps a copy,
// and the hand-back plus fetch and merge leave main's board untouched.
func TestCloneWritesThroughWithMWMain(t *testing.T) {
	e := wtBoard(t, true)
	runAll(t, []string{"add", "task", "A", "-t", "api", "-S", "ready"}, []string{"add", "task", "B", "-t", "api", "-S", "ready"})
	c := cloneOf(t, e, "c1")
	t.Setenv("MW_MAIN", e.main)
	chdirT(t, filepath.Join(c, "api"))
	runAll(t, []string{"next", "--claim", "-s", "c1w", "-j"})
	if it := itemsOf(t, e.board(), "api")[0]; it.Status != "doing" || it.Assignee != "c1w" {
		t.Fatalf("the clone's claim is not on main: %s %s", it.Status, it.Assignee)
	}
	runAll(t, []string{"done", "TSK-001", "ok", "-s", "c1w"})
	sameBoardFiles(t, e.main, c, "api")
	handBack(t, c)
	chdirT(t, e.main)
	t.Setenv("MW_MAIN", "")
	before := snapshotBoard(t, e.main)
	gitW(t, e.main, "fetch", "-q", c, "c1")
	gitW(t, e.main, "merge", "-q", "--no-edit", "FETCH_HEAD")
	after := snapshotBoard(t, e.main)
	for f, b := range before {
		if string(after[f]) != string(b) {
			t.Fatalf("the merge touched %s", f)
		}
	}
	if len(after) != len(before) || itemsOf(t, e.board(), "api")[0].Status != "done" {
		t.Fatal("main lost the clone's work or gained board files")
	}
}

// MW_MAIN tests 2 and 3: a clone without MW_MAIN, or with an invalid one, keeps its own board; an
// invalid MW_MAIN warns once and never fails the command.
func TestCloneWithoutOrInvalidMWMain(t *testing.T) {
	e := wtBoard(t, true)
	c := cloneOf(t, e, "c1")
	chdirT(t, filepath.Join(c, "web"))
	nb := t.TempDir()
	for _, env := range []string{"", "main", filepath.Join(e.root, "missing"), nb} {
		t.Setenv("MW_MAIN", env)
		title := "local " + filepath.Base(env)
		code, errs := mwErr(t, "add", "task", title, "-t", "web")
		if code != 0 || hasTitle(itemsOf(t, e.board(), "web"), title) || !hasTitle(itemsOf(t, filepath.Join(c, dataFile), "web"), title) {
			t.Fatalf("MW_MAIN=%q: exit %d, not the clone's own board (stderr %q)", env, code, errs)
		}
		if n := strings.Count(errs, "mw: MW_MAIN"); (env == "") != (n == 0) || n > 1 {
			t.Fatalf("MW_MAIN=%q: stderr %q", env, errs)
		}
	}
	t.Setenv("MW_MAIN", e.main)
	t.Setenv("MW_WORKTREE", "off")
	if code, errs := mwErr(t, "add", "task", "off", "-t", "web"); code != 0 || errs != "" || hasTitle(itemsOf(t, e.board(), "web"), "off") {
		t.Fatalf("MW_WORKTREE=off must disable MW_MAIN: %d %q", code, errs)
	}
	t.Setenv("MW_WORKTREE", "")
	chdirT(t, e.main) // MW_MAIN naming the current checkout: silent, as without it
	if code, errs := mwErr(t, "list"); code != 0 || strings.Contains(errs, "MW_MAIN") {
		t.Fatalf("in main itself: %d %q", code, errs)
	}
}

// MW_MAIN tests 4 and 5: in a real worktree commondir wins over a conflicting MW_MAIN (one warning);
// a symlinked MW_MAIN works and one naming the worktree's own main is silent.
func TestMWMainPrecedenceAndSymlink(t *testing.T) {
	e := wtBoard(t, true)
	c := cloneOf(t, e, "c1")
	chdirT(t, filepath.Join(e.wt, "api"))
	t.Setenv("MW_MAIN", c)
	code, errs := mwErr(t, "add", "task", "wt item")
	if code != 0 || errs != "mw: MW_MAIN ignored; worktree of "+e.main+"\n" || !hasTitle(itemsOf(t, e.board(), "api"), "wt item") {
		t.Fatalf("precedence: %d %q", code, errs)
	}
	if runtime.GOOS == "windows" {
		return
	}
	t.Setenv("MW_MAIN", filepath.Join(e.alias, "main"))
	if code, errs := mwErr(t, "add", "task", "wt item 2"); code != 0 || errs != "" {
		t.Fatalf("symlinked MW_MAIN naming the worktree's main: %d %q", code, errs)
	}
	chdirT(t, filepath.Join(c, "api"))
	if code, errs := mwErr(t, "add", "task", "clone item"); code != 0 || errs != "" || !hasTitle(itemsOf(t, e.board(), "api"), "clone item") {
		t.Fatalf("symlinked MW_MAIN from a clone: %d %q", code, errs)
	}
}
