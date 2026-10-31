package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// untrackedLocks is what git status shows of the lock artifacts in checkout dir.
func untrackedLocks(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	for _, l := range strings.Split(gitW(t, dir, "status", "--porcelain", "--untracked-files=all"), "\n") {
		if strings.Contains(l, ".lock") || strings.HasSuffix(l, ".tmp") {
			out = append(out, l)
		}
	}
	return out
}

// Round-2 fix 1: lock artifacts stay out of git in a linked clone and in a worktree whose board sits
// below the repo root.
func TestLockArtifactsExcluded(t *testing.T) {
	e := wtBoard(t, true)
	c := cloneOf(t, e, "c1")
	chdirT(t, filepath.Join(c, "api"))
	runAll(t, []string{"link", e.main}, []string{"add", "task", "x", "-t", "api"})
	if l := untrackedLocks(t, c); len(l) > 0 {
		t.Fatalf("linked clone: %q", l)
	}
	sub := filepath.Join(e.main, "sub")
	os.MkdirAll(sub, 0o755)
	chdirT(t, sub)
	runAll(t, []string{"init", "subboard"}, []string{"tenant", "add", "s", "-d", "."})
	gitW(t, e.main, "add", "sub/"+dataFile, "sub/"+storeDir+"/s.json")
	gitW(t, e.main, "commit", "-qm", "sub board")
	wt := filepath.Join(e.root, "wt3")
	gitW(t, e.main, "worktree", "add", "-q", "-b", "b3", wt)
	chdirT(t, filepath.Join(wt, "sub"))
	runAll(t, []string{"add", "task", "y", "-t", "s"})
	if !hasTitle(itemsOf(t, filepath.Join(sub, dataFile), "s"), "y") {
		t.Fatal("the sub-dir board did not write through")
	}
	if l := untrackedLocks(t, wt); len(l) > 0 {
		t.Fatalf("worktree with a sub-dir board: %q", l)
	}
}

// Round-2 fix 2: a git checkout of the tenant file (only older state, no events or sessions of its
// own) heals on the next command instead of warning forever.
func TestWorktreeGuardHealsAfterCheckout(t *testing.T) {
	e := wtBoard(t, true)
	chdirT(t, filepath.Join(e.wt, "api"))
	runAll(t, []string{"add", "task", "a"}, []string{"session", "start", "s1", "-t", "api"})
	gitW(t, e.wt, "checkout", "HEAD", "--", storeDir+"/api.json")
	if code, errs := mwErr(t, "list", "-t", "api"); code != 0 || strings.Contains(errs, "not synced") {
		t.Fatalf("after git checkout: %d %q", code, errs)
	}
	sameBoardFiles(t, e.main, e.wt, "api")
	if code, errs := mwErr(t, "list", "-t", "api"); code != 0 || errs != "" {
		t.Fatalf("second run: %d %q", code, errs)
	}
}

// Round-2 fix 3: MW_MAIN in an unrelated repo (its board is not a copy of main's) keeps its own board.
func TestMWMainUnrelatedRepo(t *testing.T) {
	e := wtBoard(t, true)
	o := gitRepo(t, e.root, "other")
	os.MkdirAll(filepath.Join(o, "api"), 0o755)
	chdirT(t, o)
	runAll(t, []string{"init", "other"}, []string{"tenant", "add", "api", "-d", "api"})
	chdirT(t, filepath.Join(o, "api"))
	t.Setenv("MW_MAIN", e.main)
	code, errs := mwErr(t, "add", "task", "mine", "-t", "api")
	if code != 0 || errs != "mw: MW_MAIN ignored; "+o+" is not a clone of it\n" ||
		hasTitle(itemsOf(t, e.board(), "api"), "mine") || !hasTitle(itemsOf(t, filepath.Join(o, dataFile), "api"), "mine") {
		t.Fatalf("unrelated repo: %d %q", code, errs)
	}
}

// Round-2 fix 4: mw link works in a submodule and in a --separate-git-dir clone (MW_MAIN does too).
func TestLinkSubmoduleAndSeparateGitDir(t *testing.T) {
	e := wtBoard(t, true)
	host := gitRepo(t, e.root, "host")
	gitW(t, host, "submodule", "add", "-q", e.main, "sm")
	sep := filepath.Join(e.root, "sep")
	gitW(t, e.root, "clone", "-q", "--separate-git-dir", filepath.Join(e.root, "sep.git"), e.main, sep)
	for _, d := range []string{filepath.Join(host, "sm"), sep} {
		chdirT(t, filepath.Join(d, "web"))
		if code, _, se := mwRun(t, "link", e.main); code != 0 || strings.Contains(se, "worktree") {
			t.Fatalf("link in %s: %d %q", d, code, se)
		}
		if out := captureOutput(t, func() { runAll(t, []string{"link"}) }); out != e.main+"\n" {
			t.Fatalf("show in %s: %q", d, out)
		}
		title := "via " + filepath.Base(d)
		if code, errs := mwErr(t, "add", "task", title, "-t", "web"); code != 0 || !hasTitle(itemsOf(t, e.board(), "web"), title) {
			t.Fatalf("%s did not write through: %d %q", d, code, errs)
		}
		if existsT(filepath.Join(d, ".git", "mw-main")) {
			t.Fatal(".git is a file here")
		}
	}
}

// Round-2 fix 5: in a linked clone the folder alone picks main's tenant (no -t).
func TestLinkedCloneFolderTenant(t *testing.T) {
	e := wtBoard(t, true)
	c := cloneOf(t, e, "c1")
	chdirT(t, c)
	runAll(t, []string{"link", e.main})
	for _, s := range []string{"web", "api"} {
		chdirT(t, filepath.Join(c, s))
		runAll(t, []string{"add", "task", "in " + s})
		if !hasTitle(itemsOf(t, e.board(), s), "in "+s) {
			t.Fatalf("clone/%s did not land in main's %s tenant", s, s)
		}
	}
	if b, _ := os.ReadFile(tenantPath(filepath.Join(c, dataFile), "web")); !bytes.Contains(b, []byte("in web")) {
		t.Fatal("no copy in the clone")
	}
}
