package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// wtEnv is a git repo with a board and two linked worktrees of it.
type wtEnv struct {
	root, alias, main, wt, nested string
}

func (e wtEnv) board() string { return filepath.Join(e.main, dataFile) }

func chdirT(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}

// wtBoard makes main/ (tenants demo at main, api at main/api stored through a symlinked alias of the
// root, web at main/web), commits the board when commitBoard, and adds the worktrees root/wt (outside)
// and main/.claude/worktrees/x (nested). The cwd ends in main.
func wtBoard(t *testing.T, commitBoard bool) wtEnv {
	t.Helper()
	gateEnv(t) // HOME, user config and env isolation; its own board is left behind in another temp dir
	t.Setenv("MW_WORKTREE", "")
	unsetEnvT(t, "MW_MAIN")
	e := wtEnv{root: tmpReal(t)}
	e.main = gitRepo(t, e.root, "main")
	e.alias = filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(e.root, e.alias); err != nil {
		t.Skip("no symlinks:", err)
	}
	for _, d := range []string{"api", "web"} {
		if err := os.MkdirAll(filepath.Join(e.main, d), 0o755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(e.main, d, "f"), []byte(d+"\n"), 0o644)
	}
	chdirT(t, e.main)
	runAll(t, []string{"init", "demo"}, []string{"gate", "level", "trust", "--workspace", "--by", "test"},
		[]string{"tenant", "add", "api", "-d", filepath.Join(e.alias, "main", "api")},
		[]string{"tenant", "add", "web", "-d", "web"})
	if commitBoard {
		gitW(t, e.main, "add", "-A")
	} else {
		gitW(t, e.main, "add", "api", "web")
	}
	gitW(t, e.main, "commit", "-qm", "board")
	e.wt = filepath.Join(e.root, "wt")
	e.nested = filepath.Join(e.main, ".claude", "worktrees", "x")
	gitW(t, e.main, "worktree", "add", "-q", "-b", "b1", e.wt)
	gitW(t, e.main, "worktree", "add", "-q", "-b", "b2", e.nested)
	return e
}

// itemsOf is the tenant's items on the board at path (nil when the board or tenant is missing).
func itemsOf(t *testing.T, path, slug string) []Item {
	t.Helper()
	w, err := load(path)
	if err != nil {
		return nil
	}
	tn, err := tenantBySlug(w, slug)
	if err != nil {
		return nil
	}
	return tn.Items
}

func hasTitle(items []Item, title string) bool {
	for _, it := range items {
		if it.Title == title {
			return true
		}
	}
	return false
}

// BUG-019 tests 5 and 9: in an outside, a nested and a symlinked worktree cwd, a command writes the
// main board and picks the tenant by main's folders (stored through a symlink or not).
func TestWorktreeWritesThroughByFolder(t *testing.T) {
	e := wtBoard(t, true)
	cases := []struct{ dir, tenant, title string }{
		{filepath.Join(e.wt, "api"), "api", "outside api"},
		{filepath.Join(e.wt, "web"), "web", "outside web"},
		{e.wt, "demo", "outside root"},
		{filepath.Join(e.nested, "api"), "api", "nested api"},
		{filepath.Join(e.nested, "web"), "web", "nested web"},
		{filepath.Join(e.alias, "wt", "web"), "web", "symlinked cwd"},
	}
	for _, c := range cases {
		chdirT(t, c.dir)
		runAll(t, []string{"add", "task", c.title})
		if !hasTitle(itemsOf(t, e.board(), c.tenant), c.title) {
			t.Errorf("%s: %q not on main's tenant %s", c.dir, c.title, c.tenant)
		}
	}
}

// BUG-019 test 6: an untracked board (none in the worktree) is main's board, with the cwd mapped.
func TestWorktreeUntrackedBoard(t *testing.T) {
	e := wtBoard(t, false)
	if existsT(filepath.Join(e.wt, dataFile)) {
		t.Fatal("setup: the worktree has a board")
	}
	chdirT(t, filepath.Join(e.wt, "web"))
	runAll(t, []string{"add", "task", "untracked"})
	if !hasTitle(itemsOf(t, e.board(), "web"), "untracked") {
		t.Fatal("not on main's web tenant")
	}
	if existsT(filepath.Join(e.wt, dataFile)) {
		t.Fatal("an untracked board got a worktree copy")
	}
}

// BUG-019 test 7: main without a board: the worktree board alone, as today.
func TestWorktreeMainWithoutBoard(t *testing.T) {
	gateEnv(t)
	t.Setenv("MW_WORKTREE", "")
	root := tmpReal(t)
	main := gitRepo(t, root, "main")
	wt := filepath.Join(root, "wt")
	gitW(t, main, "worktree", "add", "-q", "-b", "b1", wt)
	chdirT(t, wt)
	runAll(t, []string{"init", "local"}, []string{"add", "task", "local item"})
	if !hasTitle(itemsOf(t, filepath.Join(wt, dataFile), "local"), "local item") || existsT(filepath.Join(main, dataFile)) {
		t.Fatal("main without a board: the worktree board is used alone")
	}
}

// BUG-019 test 7: a tenant only the worktree board has is refused with exit 2; tenant add writes through.
func TestWorktreeTenantNotOnMain(t *testing.T) {
	e := wtBoard(t, true)
	chdirT(t, e.wt)
	t.Setenv("MW_WORKTREE", "off")
	runAll(t, []string{"tenant", "add", "extra"})
	t.Setenv("MW_WORKTREE", "")
	code, _, se := mwRun(t, "list", "-t", "extra")
	if code != 2 || se != "mw: tenant extra is not on the main board; run mw tenant add in the main checkout\n" {
		t.Fatalf("branch-only tenant: %d %q", code, se)
	}
	runAll(t, []string{"tenant", "add", "extra2"}, []string{"add", "task", "x2", "-t", "extra2"})
	if !hasTitle(itemsOf(t, e.board(), "extra2"), "x2") {
		t.Fatal("tenant add in a worktree must write through")
	}
	if code, _, se := mwRun(t, "list", "-t", "nope"); code != 1 || se != "mw: tenant nope not found\n" {
		t.Fatalf("unknown tenant: %d %q", code, se)
	}
}

// BUG-019 test 11: MW_WORKTREE=off keeps the worktree's own board.
func TestWorktreeOptOut(t *testing.T) {
	e := wtBoard(t, true)
	chdirT(t, filepath.Join(e.wt, "web"))
	t.Setenv("MW_WORKTREE", "off")
	runAll(t, []string{"add", "task", "local", "-t", "web"}) // the worktree board names main's folders: -t, as before
	if hasTitle(itemsOf(t, e.board(), "web"), "local") || !hasTitle(itemsOf(t, filepath.Join(e.wt, dataFile), "web"), "local") {
		t.Fatal("MW_WORKTREE=off must write the worktree board only")
	}
}

// BUG-019 test 17: in a worktree the status line, the hook context, log -f, configure, user, version
// and migrate all read main's board.
func TestWorktreeReadersFollowMain(t *testing.T) {
	e := wtBoard(t, true)
	runAll(t, []string{"add", "task", "A", "-t", "api", "-S", "ready"}, []string{"add", "task", "B", "-t", "api", "-S", "ready"},
		[]string{"session", "start", "w1", "-t", "api", "--claude-session", "cc-1"}, []string{"start", "TSK-001", "-t", "api", "-s", "w1"})
	want := statusLine(e.main, false)
	for _, d := range []string{e.wt, filepath.Join(e.nested, "web")} {
		if got := statusLine(d, false); got != want || got == "" {
			t.Errorf("status line in %s: %q, main %q", d, got, want)
		}
		if ctx := hookContextText(hookInput{SessionID: "cc-1", Cwd: d}); !strings.Contains(ctx, "TSK-001") {
			t.Errorf("hook context in %s: %q", d, ctx)
		}
		chdirT(t, d)
		resetRunState()
		if p, err := findWorkspace(); err != nil || p != e.board() { // log -f, configure, user get, version, migrate
			t.Errorf("findWorkspace in %s: %q %v", d, p, err)
		}
		if out := captureOutput(t, func() { runAll(t, []string{"version", "-j"}) }); !strings.Contains(out, jsonString(e.board())) {
			t.Errorf("version -j in %s: %s", d, out)
		}
		if out := captureOutput(t, func() { runAll(t, []string{"migrate", "--dry-run"}) }); !strings.Contains(out, e.board()) {
			t.Errorf("migrate in %s: %s", d, out)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	if got := statusLine(filepath.Join(e.alias, "wt"), false); got != want {
		t.Errorf("status line through a symlink: %q", got)
	}
}

func existsT(p string) bool { _, err := os.Stat(p); return err == nil }

// The worktree tenant check prints nothing for a named session (sessions live on main).
func TestWorktreeTenantCheckQuietForSessions(t *testing.T) {
	e := wtBoard(t, true)
	runAll(t, []string{"session", "start", "w1", "-t", "api"})
	chdirT(t, filepath.Join(e.wt, "api"))
	t.Setenv("MW_SESSION", "w1")
	code := -1
	se := captureFile(t, &os.Stderr, func() { captureOutput(t, func() { code = mainExit([]string{"list"}, os.Stderr) }) })
	if code != 0 || se != "" {
		t.Fatalf("list with MW_SESSION: %d %q", code, se)
	}
}

// Review finding 1: the tenant check selects with the mapped cwd. A folder tenant only the worktree
// has is refused; a branch-only tenant listed first never triggers it through the first-tenant fallback.
func TestWorktreeTenantCheckByFolder(t *testing.T) {
	e := wtBoard(t, true)
	sub := filepath.Join(e.wt, "web", "sub")
	os.MkdirAll(sub, 0o755)
	chdirT(t, sub)
	t.Setenv("MW_WORKTREE", "off")
	runAll(t, []string{"tenant", "add", "web2", "-d", "."}, []string{"tenant", "add", "extra"}, []string{"add", "task", "e", "-t", "extra"})
	// the worktree index (kept by the guard: extra has events main lacks) lists extra first
	ix := filepath.Join(e.wt, dataFile)
	var doc map[string]any
	b, _ := os.ReadFile(ix)
	json.Unmarshal(b, &doc)
	ts := doc["tenants"].([]any)
	doc["tenants"] = append([]any{ts[len(ts)-1]}, ts[:len(ts)-1]...)
	b, _ = json.Marshal(doc)
	os.WriteFile(ix, b, 0o600)
	t.Setenv("MW_WORKTREE", "")
	if code, _, se := mwRun(t, "list"); code != 2 || !strings.Contains(se, "tenant web2 is not on the main board") {
		t.Fatalf("folder tenant missing on main: %d %q", code, se)
	}
	for _, d := range []string{e.wt, filepath.Join(e.wt, "api")} {
		chdirT(t, d)
		code := -1
		captureFile(t, &os.Stderr, func() { captureOutput(t, func() { code = mainExit([]string{"list"}, os.Stderr) }) })
		if code != 0 {
			t.Fatalf("%s: the first-tenant fallback must not refuse: exit %d", d, code)
		}
	}
}

// unsetEnvT unsets k for the test (restored afterwards).
func unsetEnvT(t *testing.T, k string) {
	t.Helper()
	t.Setenv(k, "")
	os.Unsetenv(k)
}
