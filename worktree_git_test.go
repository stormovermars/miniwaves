package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// handBack is the USAGE.md "Git worktrees" hand-back, run in the worktree wt before the merge.
func handBack(t *testing.T, wt string) {
	t.Helper()
	errs := captureFile(t, &os.Stderr, func() { captureOutput(t, func() { runAll(t, []string{"list", "-A"}) }) })
	if strings.Contains(errs, "not synced") || strings.Contains(errs, "not on main") {
		t.Fatalf("hand-back step 1 warns: %q", errs)
	}
	gitW(t, wt, "add", "-A", dataFile, storeDir)
	gitW(t, wt, "commit", "-qm", "board: worktree record")
	on := strings.Fields(gitW(t, wt, "ls-tree", "-r", "--name-only", "main", "--", dataFile, storeDir))
	if len(on) > 0 {
		gitW(t, wt, append([]string{"checkout", "main", "--"}, on...)...)
	}
	for _, f := range strings.Fields(gitW(t, wt, "ls-files", dataFile, storeDir)) {
		found := false
		for _, o := range on {
			found = found || o == f
		}
		if !found {
			gitW(t, wt, "rm", "-q", f)
		}
	}
	gitW(t, wt, "commit", "-qm", "board: hand back to main")
}

// snapshotBoard is every file of main's live board, by name.
func snapshotBoard(t *testing.T, main string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	files, _ := filepath.Glob(filepath.Join(main, storeDir, "*.json"))
	for _, f := range append(files, filepath.Join(main, dataFile)) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[f] = b
	}
	return out
}

// BUG-019 tests 16 and 18: a tenant created in the worktree (a branch-only board file) and a dirty
// main board go through the hand-back; the merge then leaves every board file of main untouched and
// leaves no orphan.
func TestWorktreeHandBackMerge(t *testing.T) {
	e := wtBoard(t, true)
	chdirT(t, filepath.Join(e.wt, "api"))
	runAll(t, []string{"add", "task", "branch work", "-S", "ready"}, []string{"start", "TSK-001", "-s", "w1"},
		[]string{"done", "TSK-001", "ok", "-s", "w1"}, []string{"tenant", "add", "t2"}, []string{"add", "task", "y", "-t", "t2"})
	os.WriteFile(filepath.Join(e.wt, "api", "code.go"), []byte("package api\n"), 0o644)
	gitW(t, e.wt, "add", "api/code.go")
	gitW(t, e.wt, "commit", "-qm", "code")
	if !existsT(tenantPath(filepath.Join(e.wt, dataFile), "t2")) {
		t.Fatal("setup: t2 is not in the worktree board")
	}
	handBack(t, e.wt)
	chdirT(t, e.main)
	runAll(t, []string{"add", "task", "dirty main", "-t", "web"}) // main's board is dirty (uncommitted) at merge time
	before := snapshotBoard(t, e.main)
	gitW(t, e.main, "merge", "-q", "--no-edit", "b1")
	if after := snapshotBoard(t, e.main); len(after) != len(before) {
		t.Fatalf("the merge changed the board file set: %d -> %d", len(before), len(after))
	} else {
		for f, b := range before {
			if !bytes.Equal(after[f], b) {
				t.Fatalf("the merge touched %s", f)
			}
		}
	}
	if !existsT(filepath.Join(e.main, "api", "code.go")) {
		t.Fatal("the code did not merge")
	}
	if !hasTitle(itemsOf(t, e.board(), "t2"), "y") || itemsOf(t, e.board(), "api")[0].Status != "done" {
		t.Fatal("main lost the worktree's work")
	}
	// the branch's history keeps the worktree record (requirement 4)
	if log := gitW(t, e.main, "log", "--format=%s", "b1"); !strings.Contains(log, "board: worktree record") {
		t.Fatalf("branch history: %s", log)
	}
	if show := gitW(t, e.main, "show", "b1~1:.miniwaves/t2.json"); !strings.Contains(show, `"title":"y"`) {
		t.Fatalf("worktree record lacks t2: %s", show)
	}
}
