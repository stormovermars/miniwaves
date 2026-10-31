package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// gitW runs git in dir for a test (mw itself never runs git); the test is skipped without git.
func gitW(t *testing.T, dir string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "init.defaultBranch=main",
		"-c", "protocol.file.allow=always", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// tmpReal is a fresh temp dir with symlinks resolved (/var vs /private/var on macOS).
func tmpReal(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// gitRepo makes a repo with one commit at root/name.
func gitRepo(t *testing.T, root, name string) string {
	t.Helper()
	d := filepath.Join(root, name)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	gitW(t, d, "init", "-q")
	if err := os.WriteFile(filepath.Join(d, "f"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitW(t, d, "add", "f")
	gitW(t, d, "commit", "-qm", "init")
	return d
}

// BUG-019 test 12: the Windows path functions (FromSlash, drive letters, case), on any host.
func TestWorktreeWindowsPaths(t *testing.T) {
	cases := []struct{ base, ref, want string }{
		{`C:\repo\.git\worktrees\x`, "../..", `C:\repo\.git`},
		{`C:\wt`, "C:/repo/.git/worktrees/x", `C:\repo\.git\worktrees\x`},
		{`C:\wt`, `d:\Repo\.git\worktrees\x`, `d:\Repo\.git\worktrees\x`},
		{`\\srv\share\repo\.git\worktrees\x`, "../..", `\\srv\share\repo\.git`},
	}
	for _, c := range cases {
		if got := gitPath(c.base, c.ref, "windows"); got != c.want {
			t.Errorf("gitPath(%q, %q) = %q, want %q", c.base, c.ref, got, c.want)
		}
	}
	if got := gitPath("/r/.git/worktrees/x", "../..", "linux"); got != "/r/.git" {
		t.Errorf("unix relative: %q", got)
	}
	if got := gitPath("/wt", "/r/.git/worktrees/x", "darwin"); got != "/r/.git/worktrees/x" {
		t.Errorf("unix absolute: %q", got)
	}
	if rel, ok := relWithin(`C:\Repo\WT\sub\x`, `c:\repo\wt`, "windows"); !ok || rel != `sub\x` {
		t.Errorf("windows case: %q %v", rel, ok)
	}
	if _, ok := relWithin("/Repo/WT/sub", "/repo/wt", "darwin"); ok {
		t.Error("unix paths are case-sensitive")
	}
	if _, ok := relWithin(`C:\repo\wt2`, `C:\repo\wt`, "windows"); ok {
		t.Error("a sibling with a common prefix is not inside")
	}
	if !pathEqual(".GIT", ".git", "windows") || pathEqual(".GIT", ".git", "linux") {
		t.Error("pathEqual case rules")
	}
}

// BUG-019 tests 5 and 8 (detection): nested and outside worktrees are found; a non-git dir, a plain
// checkout, a submodule, a bare repo's worktree and a --separate-git-dir repo are not worktrees.
func TestWorktreeDetection(t *testing.T) {
	root := tmpReal(t)
	main := gitRepo(t, root, "main")
	gitW(t, main, "worktree", "add", "-q", "-b", "out", filepath.Join(root, "wt"))
	gitW(t, main, "worktree", "add", "-q", "-b", "nested", filepath.Join(main, ".claude", "worktrees", "x"))
	sub := filepath.Join(root, "wt", "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ dir, wt string }{
		{filepath.Join(root, "wt"), filepath.Join(root, "wt")},
		{sub, filepath.Join(root, "wt")},
		{filepath.Join(main, ".claude", "worktrees", "x"), filepath.Join(main, ".claude", "worktrees", "x")},
	} {
		wt, m, ok := worktreeOf(c.dir)
		if !ok || wt != c.wt || m != main {
			t.Errorf("%s: %q %q %v", c.dir, wt, m, ok)
		}
	}
	// not worktrees
	plain := t.TempDir()
	lib := gitRepo(t, root, "lib")
	gitW(t, main, "submodule", "add", "-q", lib, "lib")
	gitW(t, root, "clone", "-q", "--bare", main, "bare.git")
	gitW(t, filepath.Join(root, "bare.git"), "worktree", "add", "-q", "-b", "bw", filepath.Join(root, "bwt"))
	gitW(t, root, "init", "-q", "--separate-git-dir="+filepath.Join(root, "sep"), "sepwork")
	for _, d := range []string{plain, main, filepath.Join(main, "lib"), filepath.Join(root, "bwt"), filepath.Join(root, "sepwork")} {
		if wt, m, ok := worktreeOf(d); ok {
			t.Errorf("%s detected as a worktree (%q, %q)", d, wt, m)
		}
	}
	// a symlinked cwd resolves to the same worktree (test 9)
	if runtime.GOOS != "windows" {
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(filepath.Join(root, "wt"), link); err != nil {
			t.Fatal(err)
		}
		if wt, m, ok := worktreeOf(filepath.Join(link, "a")); !ok || wt != filepath.Join(root, "wt") || m != main {
			t.Errorf("symlink: %q %q %v", wt, m, ok)
		}
	}
}
