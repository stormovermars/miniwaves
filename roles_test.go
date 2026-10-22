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

func TestDispatcherCeremoniesGoldens(t *testing.T) {
	path := rolesBoard(t)
	dir := filepath.Dir(path)
	r, _ := filepath.EvalSymlinks(dir)
	home := os.Getenv("HOME")
	rh, _ := filepath.EvalSymlinks(home)
	norm := func(s string) string { return osNorm(replacePath(replacePath(s, dir, r, "<DIR>"), home, rh, "<HOME>")) }
	cap := func(args ...string) string { return norm(captureOutput(t, func() { runAll(t, args) })) }
	base := cap("get-prompt", "dispatcher", "--job", "tiny")
	if strings.Contains(base, "Ceremonies") {
		t.Fatalf("no ceremonies set, yet the prompt has the section:\n%s", base)
	}
	runAll(t, []string{"jobs", "set", "tiny", "ceremonies=review,planning,triage,spike", "ready_min=5"})
	all := cap("get-prompt", "dispatcher", "--job", "tiny")
	rolesGolden(t, "dispatcher-ceremonies-all", all)
	for _, want := range []string{"fewer than 5 items are ready", "at most 4 per day", "`mw list -k chore`", "never `doing` ones", "At most one `mw ask`", "`Review WAV-NNN`", "`-a planner`", "`-a spike`", "older than 2h", "`URGENT`", "merging main is a release"} {
		if !strings.Contains(all, want) {
			t.Errorf("all-ceremonies prompt missing %q", want)
		}
	}
	// only listed names render
	runAll(t, []string{"jobs", "set", "tiny", "ceremonies=triage"})
	one := cap("get-prompt", "dispatcher", "--job", "tiny")
	rolesGolden(t, "dispatcher-ceremonies-triage", one)
	for _, gone := range []string{"- review:", "- planning:", "- spike:", "Hotfix lane", "fewer than"} {
		if strings.Contains(one, gone) {
			t.Errorf("triage-only prompt has %q", gone)
		}
	}
	if !strings.Contains(one, "- triage:") || !strings.Contains(one, "mw list -k chore") {
		t.Errorf("triage-only prompt lost its section:\n%s", one)
	}
	// clearing the list restores today's prompt byte for byte; without --job nothing changes either
	runAll(t, []string{"jobs", "set", "tiny", "ceremonies="})
	if again := cap("get-prompt", "dispatcher", "--job", "tiny"); again != base {
		t.Errorf("ceremonies cleared but the prompt differs:\n%s", again)
	}
	if def := cap("get-prompt", "dispatcher"); strings.Contains(def, "Ceremonies") {
		t.Errorf("default prompt has ceremonies")
	}
}

func TestSessionRoleCeremonyGoldens(t *testing.T) {
	path := rolesBoard(t)
	dir := filepath.Dir(path)
	r, _ := filepath.EvalSymlinks(dir)
	home := os.Getenv("HOME")
	rh, _ := filepath.EvalSymlinks(home)
	norm := func(s string) string { return osNorm(replacePath(replacePath(s, dir, r, "<DIR>"), home, rh, "<HOME>")) }
	cap := func(args ...string) string { return norm(captureOutput(t, func() { runAll(t, args) })) }
	wantIn := map[string][]string{
		"review":  {"`Review WAV-NNN`", "WAV-NNN/", "lessons.md", "Screenshots only", "at most one `mw ask`", "never accept or reject"},
		"planner": {"`Planning YYYY-MM-DD`", "never `doing` ones", "`mw edit -c` is for humans", "one `mw ask` digest", "screenshots only"},
		"spike":   {"SPK-NNN/", "throwaway branch", "never merged", "STALE", "Screenshots only", "At most one `mw ask`"},
	}
	for _, role := range []string{"review", "planner", "spike"} {
		got := cap("get-prompt", "session", "s-"+role, "-a", role)
		rolesGolden(t, "session-"+role, got)
		for _, want := range wantIn[role] {
			if !strings.Contains(got, want) {
				t.Errorf("%s prompt missing %q", role, want)
			}
		}
		for _, other := range []string{"review", "planner", "spike"} {
			if other != role && strings.Contains(got, "Ceremony role "+other) {
				t.Errorf("%s prompt carries the %s block", role, other)
			}
		}
	}
	// other roles never get a ceremony block
	for _, role := range []string{"builder", "dispatcher", "reviewer"} {
		if got := cap("get-prompt", "session", "s-"+role, "-a", role); strings.Contains(got, "Ceremony role") {
			t.Errorf("%s prompt has a ceremony block", role)
		}
	}
}

// The docs and the prompts must agree on the ceremony names, and the added PROTOCOL.md prose must not
// turn into protocol steps.
func TestCeremonyDocs(t *testing.T) {
	usage, err := os.ReadFile("USAGE.md")
	if err != nil {
		t.Fatal(err)
	}
	proto, err := os.ReadFile("PROTOCOL.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range ceremonyNames {
		if !strings.Contains(string(usage), "`"+n+"`") {
			t.Errorf("USAGE.md does not document ceremony %q", n)
		}
	}
	for _, want := range []string{"### Ceremonies", "`ready_min`", "never read from repo files", "Steps only a human performs"} {
		if !strings.Contains(string(usage), want) {
			t.Errorf("USAGE.md missing %q", want)
		}
	}
	for _, step := range []string{"merge", "push", "tag"} {
		if !strings.Contains(string(proto), "| "+step+" |") {
			t.Errorf("PROTOCOL.md does not list human-only step %q", step)
		}
	}
	if len(defaultProtocol) != 11 {
		t.Errorf("default protocol has %d steps, want 11", len(defaultProtocol))
	}
}
