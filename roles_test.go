package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rolesBoard(t *testing.T) (path string) {
	path = inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t,
		[]string{"init", "Roles app"},
		[]string{"tenant", "add", "sheet", "Sheet"},
		[]string{"jobs", "enable", "tiny", "-t", "sheet"},
		[]string{"jobs", "set", "tiny", "title=Tiny corpus", "max_workers=3", "worktrees=off", "lock=buildlock.sh", "news.url=https://example.com/news"},
	)
	return path
}

func rolesGolden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join(rolesTestdata, "golden-roles-"+name+".txt")
	if os.Getenv("GOLDEN_UPDATE") == "1" {
		if err := os.WriteFile(p, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(want), "\r") {
		t.Fatalf("golden %s has CR", name)
	}
	if string(want) != got {
		t.Errorf("golden %s changed:\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}

func TestRolePromptGoldens(t *testing.T) {
	path := rolesBoard(t)
	dir := filepath.Dir(path)
	r, _ := filepath.EvalSymlinks(dir)
	home := os.Getenv("HOME")
	rh, _ := filepath.EvalSymlinks(home)
	norm := func(s string) string { return osNorm(replacePath(replacePath(s, dir, r, "<DIR>"), home, rh, "<HOME>")) }
	cap := func(args ...string) string { return norm(captureOutput(t, func() { runAll(t, args) })) }
	rolesGolden(t, "dispatcher-job", cap("get-prompt", "dispatcher", "--job", "tiny"))
	rolesGolden(t, "newsdesk-job", cap("get-prompt", "newsdesk", "--job", "tiny"))
	rolesGolden(t, "dispatcher-default", cap("get-prompt", "dispatcher"))
	rolesGolden(t, "newsdesk-default", cap("get-prompt", "newsdesk"))
}

func TestRolePromptVariables(t *testing.T) {
	rolesBoard(t)
	d := captureOutput(t, func() { runAll(t, []string{"get-prompt", "dispatcher", "--job", "tiny"}) })
	for _, want := range []string{"at most 3 working sessions", "Worktrees are off", "`./buildlock.sh`", `mw note ID "ready for review"`, "mw ask ID", "wr builders keep"} {
		if !strings.Contains(d, want) {
			t.Errorf("dispatcher --job missing %q", want)
		}
	}
	d = captureOutput(t, func() { runAll(t, []string{"get-prompt", "dispatcher"}) })
	if !strings.Contains(d, "at most 4 working sessions") || !strings.Contains(d, "Worktrees: one") || strings.Contains(d, "buildlock") {
		t.Errorf("default dispatcher prompt wrong:\n%s", d)
	}
	n := captureOutput(t, func() { runAll(t, []string{"get-prompt", "newsdesk", "--job", "tiny"}) })
	for _, want := range []string{`for "Tiny corpus"`, "mw news --job tiny", "https://example.com/news", "needing the human"} {
		if !strings.Contains(n, want) {
			t.Errorf("newsdesk --job missing %q", want)
		}
	}
	for _, kind := range []string{"dispatcher", "newsdesk"} {
		if err := run([]string{"get-prompt", kind, "--job", "nope"}); err == nil || !strings.Contains(err.Error(), "job nope not found") {
			t.Errorf("%s unknown job: %v", kind, err)
		}
	}
	if err := run([]string{"get-prompt", "session", "x1", "--job", "tiny"}); err == nil {
		t.Error("session accepted --job")
	}
	if strings.Contains(d+n, "{{") || strings.Contains(d+n, "<no value>") {
		t.Error("unfilled template variable")
	}
}

var rolesTestdata, _ = filepath.Abs("testdata")
