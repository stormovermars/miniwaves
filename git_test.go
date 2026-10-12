package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitExcludesLockFilesOnly(t *testing.T) {
	path := inTempDir(t)
	dir := filepath.Dir(path)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	gitT(t, dir, "init", "-q", "-b", "main")
	runAll(t, []string{"init", "App"}, []string{"add", "task", "x", "-S", "ready"})
	b, err := os.ReadFile(filepath.Join(dir, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range lockArtifacts {
		if n := strings.Count(string(b), f+"\n"); n != 1 {
			t.Errorf("%s appears %d times in info/exclude", f, n)
		}
	}
	if st := gitT(t, dir, "status", "--porcelain"); strings.Contains(st, ".lock") || strings.Contains(st, ".tmp") {
		t.Errorf("lock files visible to git: %q", st)
	}
}

func TestLoadMergeMarkers(t *testing.T) {
	path := inTempDir(t)
	if err := os.WriteFile(path, []byte("<<<<<<< HEAD\n{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := load(path); err == nil || !strings.Contains(err.Error(), "board has merge markers: resolve it in git") {
		t.Fatalf("got %v", err)
	}
}
