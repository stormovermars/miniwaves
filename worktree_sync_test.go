package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// sameBoardFiles fails unless the worktree board's index and tenant files equal main's, byte for byte.
func sameBoardFiles(t *testing.T, main, wt string, slugs ...string) {
	t.Helper()
	files := []string{dataFile}
	for _, s := range slugs {
		files = append(files, filepath.Join(storeDir, s+".json"))
	}
	for _, f := range files {
		a, err1 := os.ReadFile(filepath.Join(main, f))
		b, err2 := os.ReadFile(filepath.Join(wt, f))
		if err1 != nil || err2 != nil || !bytes.Equal(a, b) {
			t.Fatalf("%s: worktree copy differs from main (%v %v)", f, err1, err2)
		}
	}
}

// eventsOf is every event of the tenant's items as "ID action" lines.
func eventsOf(t *testing.T, path, slug string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, it := range itemsOf(t, path, slug) {
		for _, e := range it.Events {
			out[it.ID+" "+e.At+" "+e.Action] = true
		}
	}
	return out
}

// BUG-019 tests 2 and 13: one worktree session takes two items in a row (WIP on main), and the
// worktree board then holds every event of both, the session and its claude_session.
func TestWorktreeTwoItemsInARow(t *testing.T) {
	e := wtBoard(t, true)
	runAll(t, []string{"configure", "wip.session=true"},
		[]string{"add", "task", "A", "-t", "api", "-S", "ready"}, []string{"add", "task", "B", "-t", "api", "-S", "ready"})
	chdirT(t, filepath.Join(e.wt, "api"))
	runAll(t, []string{"session", "start", "w1", "--claude-session", "cc-7"})
	for _, id := range []string{"TSK-001", "TSK-002"} {
		var got struct{ ID string }
		out := captureOutput(t, func() { runAll(t, []string{"next", "--claim", "-s", "w1", "-j"}) })
		if json.Unmarshal([]byte(out), &got) != nil || got.ID != id {
			t.Fatalf("claim: %q, want %s", out, id)
		}
		runAll(t, []string{"done", id, "ok", "-s", "w1"})
	}
	sameBoardFiles(t, e.main, e.wt, "api")
	wtb := filepath.Join(e.wt, dataFile)
	for ev := range eventsOf(t, e.board(), "api") {
		if !eventsOf(t, wtb, "api")[ev] {
			t.Fatalf("worktree board lacks %s", ev)
		}
	}
	b, _ := os.ReadFile(tenantPath(wtb, "api"))
	if !bytes.Contains(b, []byte(`"claude_session":"cc-7"`)) {
		t.Fatal("claude_session lost in the copy")
	}
}

// BUG-019 test 3: a live worktree session's activity moves Updated on main, so reap there spares it.
func TestWorktreeSessionNotReaped(t *testing.T) {
	e := wtBoard(t, true)
	runAll(t, []string{"add", "task", "A", "-t", "api", "-S", "ready"})
	chdirT(t, filepath.Join(e.wt, "api"))
	runAll(t, []string{"next", "--claim", "-s", "w1", "-j"})
	w := loadT(t, e.board())
	s, _ := sessionByID(w, "w1")
	s.Updated = time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339)
	if err := save(e.board(), w); err != nil {
		t.Fatal(err)
	}
	runAll(t, []string{"note", "TSK-001", "still on it", "-s", "w1"})
	chdirT(t, e.main)
	runAll(t, []string{"session", "reap", "-t", "api", "--older-than", "1h"})
	if it := itemsOf(t, e.board(), "api")[0]; it.Status != "doing" || it.Assignee != "w1" {
		t.Fatalf("an active worktree session was reaped: %s %s", it.Status, it.Assignee)
	}
}

// BUG-019 test 4: add, reject, decide and wave take their ids from main's counters in both worktrees.
func TestWorktreeIDsDistinct(t *testing.T) {
	e := wtBoard(t, true)
	wt2 := filepath.Join(e.root, "wt2")
	gitW(t, e.main, "worktree", "add", "-q", "-b", "b3", wt2)
	idOf := func(title string) string {
		for _, it := range itemsOf(t, e.board(), "api") {
			if it.Title == title {
				return it.ID
			}
		}
		return ""
	}
	for i, d := range []string{e.wt, wt2, e.wt, wt2} {
		chdirT(t, filepath.Join(d, "api"))
		story, task := fmt.Sprintf("s%d", i), fmt.Sprintf("x%d", i)
		runAll(t, []string{"add", "story", story, "-S", "ready"})
		runAll(t, []string{"add", "task", task, "-p", idOf(story), "-S", "ready"})
		runAll(t, []string{"start", idOf(task)}, []string{"done", idOf(task), "ok"}, []string{"reject", idOf(story), "redo", "--by", "sir"}, []string{"decide", "pick", "--by", "sir"},
			[]string{"wave", "create", fmt.Sprintf("w%d", i)})
	}
	seen := map[string]bool{}
	items := itemsOf(t, e.board(), "api")
	for _, it := range items {
		if seen[it.ID] {
			t.Fatalf("duplicate id %s", it.ID)
		}
		seen[it.ID] = true
	}
	if len(items) != 4*4+1 { // story, task, reject's fix task and wave per round; one Decisions chore
		t.Fatalf("%d items on main, want 17", len(items))
	}
}

// BUG-019 test 10: a failed copy keeps the exit code, warns, and the next command heals the copy.
func TestWorktreeCopyFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod does not make a Windows folder read-only")
	}
	e := wtBoard(t, true)
	chdirT(t, filepath.Join(e.wt, "api"))
	runAll(t, []string{"add", "task", "first"})
	store := filepath.Join(e.wt, storeDir)
	os.Chmod(store, 0o500)
	t.Cleanup(func() { os.Chmod(store, 0o755) })
	code := -1
	errs := captureFile(t, &os.Stderr, func() { captureOutput(t, func() { code = mainExit([]string{"add", "task", "second"}, os.Stderr) }) })
	if code != 0 || !strings.Contains(errs, "mw: worktree board not synced:") || !hasTitle(itemsOf(t, e.board(), "api"), "second") {
		t.Fatalf("copy failure: exit %d, stderr %q", code, errs)
	}
	os.Chmod(store, 0o755)
	runAll(t, []string{"list"})
	sameBoardFiles(t, e.main, e.wt, "api")
}

// BUG-019 test 15: a worktree write main does not have (MW_WORKTREE=off, an older mw) is never
// overwritten; every command warns until it is handed to main.
func TestWorktreeUnsharedEventsGuard(t *testing.T) {
	e := wtBoard(t, true)
	chdirT(t, filepath.Join(e.wt, "api"))
	runAll(t, []string{"add", "task", "shared"})
	t.Setenv("MW_WORKTREE", "off")
	runAll(t, []string{"add", "task", "local only", "-t", "api"})
	t.Setenv("MW_WORKTREE", "")
	before, _ := os.ReadFile(tenantPath(filepath.Join(e.wt, dataFile), "api"))
	for _, args := range [][]string{{"add", "task", "after"}, {"list"}} {
		errs := captureFile(t, &os.Stderr, func() { captureOutput(t, func() { runAll(t, args) }) })
		if !strings.Contains(errs, "mw: worktree board has 1 events not on main (api); not synced\n") {
			t.Fatalf("%v: stderr %q", args, errs)
		}
	}
	after, _ := os.ReadFile(tenantPath(filepath.Join(e.wt, dataFile), "api"))
	if !bytes.Equal(before, after) || hasTitle(itemsOf(t, e.board(), "api"), "local only") {
		t.Fatal("the worktree-only event was overwritten or leaked to main")
	}
	if !hasTitle(itemsOf(t, e.board(), "api"), "after") {
		t.Fatal("the command itself must still run on main")
	}
}

// BUG-019 test 14: two processes in one worktree; afterwards the worktree board equals main's.
func TestWorktreeConcurrentSync(t *testing.T) {
	e := wtBoard(t, true)
	const n = 16
	for i := 0; i < n; i++ {
		runAll(t, []string{"add", "task", fmt.Sprintf("job %d", i), "-t", "api", "-S", "ready"})
	}
	bin := filepath.Join(t.TempDir(), "mw")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	_, thisFile, _, _ := runtime.Caller(0)
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = filepath.Dir(thisFile)
	build.Env = append(os.Environ(), "HOME="+origHome)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []string
	for p := 0; p < 2; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			for {
				c := exec.Command(bin, "next", "--claim", "-s", fmt.Sprintf("p%d", p), "-j")
				c.Dir = filepath.Join(e.wt, "api")
				out, err := c.Output()
				var m struct{ ID string }
				if err != nil || json.Unmarshal(out, &m) != nil || m.ID == "" {
					return
				}
				c = exec.Command(bin, "done", m.ID, "ok", "-s", fmt.Sprintf("p%d", p))
				c.Dir = filepath.Join(e.wt, "api")
				if out, err := c.CombinedOutput(); err != nil || len(bytes.TrimSpace(out)) == 0 || bytes.Contains(out, []byte("not synced")) {
					mu.Lock()
					errs = append(errs, fmt.Sprintf("%s: %v %s", m.ID, err, out))
					mu.Unlock()
				}
			}
		}(p)
	}
	wg.Wait()
	if len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}
	done := 0
	for _, it := range itemsOf(t, e.board(), "api") {
		if it.Status == "done" {
			done++
		}
	}
	if done != n {
		t.Fatalf("%d done on main, want %d", done, n)
	}
	sameBoardFiles(t, e.main, e.wt, "api")
}

// Review finding 2: a worktree write without events (a session start under MW_WORKTREE=off or an older
// mw) is not overwritten either, and the hand-back's mw list warns about it.
func TestWorktreeLocalChangeWithoutEvents(t *testing.T) {
	e := wtBoard(t, true)
	chdirT(t, filepath.Join(e.wt, "api"))
	runAll(t, []string{"add", "task", "shared"}) // the first sync records the copy
	t.Setenv("MW_WORKTREE", "off")
	runAll(t, []string{"session", "start", "solo", "-t", "api"})
	t.Setenv("MW_WORKTREE", "")
	wtf := tenantPath(filepath.Join(e.wt, dataFile), "api")
	before, _ := os.ReadFile(wtf)
	for _, args := range [][]string{{"add", "task", "after"}, {"list"}} {
		errs := captureFile(t, &os.Stderr, func() { captureOutput(t, func() { runAll(t, args) }) })
		if !strings.Contains(errs, "(api); not synced\n") {
			t.Fatalf("%v: stderr %q", args, errs)
		}
	}
	if after, _ := os.ReadFile(wtf); !bytes.Equal(before, after) || !bytes.Contains(after, []byte(`"id":"solo"`)) {
		t.Fatal("the worktree's session start was overwritten")
	}
}
