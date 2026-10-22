package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	toon "github.com/toon-format/toon-go"
)

func inTempDir(t *testing.T) string {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	d := t.TempDir()
	// evidence/builds/scratch/... resolve under home: keep them in a temp dir
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("MW_USER_CONFIG", filepath.Join(home, "mw-user.json")) // never read the developer's real per-user file
	if err := os.Chdir(d); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return filepath.Join(d, dataFile)
}

func TestWorkflow(t *testing.T) {
	path := inTempDir(t)
	steps := [][]string{
		{"init", "Tiny app"}, {"gate", "level", "trust", "--workspace", "--by", "t"},
		{"add", "prompt", "Make it fast", "--status", "ready"},
		{"add", "story", "Instant startup", "--parent", "PRM-001", "--acceptance", "opens under 100ms"},
		{"ready", "STR-001"},
		{"start", "STR-001"},
		{"done", "STR-001", "measured at 42ms"},
	}
	for _, step := range steps {
		if err := run(step); err != nil {
			t.Fatalf("%v: %v", step, err)
		}
	}
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	p := &w.Tenants[0]
	if len(p.Items) != 2 {
		t.Fatalf("got %d items", len(p.Items))
	}
	if p.Items[1].Status != "done" || p.Items[1].Note != "measured at 42ms" {
		t.Fatalf("unexpected item: %+v", p.Items[1])
	}
}

func TestCommandHelpNeedsNoWorkspace(t *testing.T) {
	inTempDir(t)
	for _, args := range [][]string{{"help", "add"}, {"board", "--help"}, {"tenant", "-h"}} {
		got := captureOutput(t, func() {
			if err := run(args); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
		})
		if !strings.Contains(got, "mw ") || !strings.Contains(got, "Run `mw help`") || !strings.Contains(got, "--tenant") {
			t.Fatalf("unhelpful output for %v: %q", args, got)
		}
	}
	if err := run([]string{"help", "missing"}); err == nil {
		t.Fatal("unknown help topic accepted")
	}
}

func TestDescriptionNeedsNoWorkspace(t *testing.T) {
	inTempDir(t)
	toonOutput := captureOutput(t, func() {
		if err := run([]string{"description"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"purpose:", "human[3]", "assistant[3]", "mw session start", "mw help COMMAND"} {
		if !strings.Contains(toonOutput, want) {
			t.Fatalf("description missing %q:\n%s", want, toonOutput)
		}
	}
	jsonOutput := captureOutput(t, func() {
		if err := run([]string{"description", "-j"}); err != nil {
			t.Fatal(err)
		}
	})
	var got descriptionInfo
	if err := json.Unmarshal([]byte(jsonOutput), &got); err != nil || len(got.Assistant) != 3 {
		t.Fatalf("invalid JSON description: %v %q", err, jsonOutput)
	}
}

func TestRejectsUnknownParentWithoutWriting(t *testing.T) {
	path := inTempDir(t)
	if err := runTrust([]string{"init", "test"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"add", "task", "orphan", "--parent", "STR-999"}); err == nil {
		t.Fatal("expected error")
	}
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	p := &w.Tenants[0]
	if len(p.Items) != 0 || p.Next["TSK"] != 0 {
		t.Fatalf("invalid add mutated project: %+v", p)
	}
}

func TestHierarchyAndCompletionGate(t *testing.T) {
	path := inTempDir(t)
	steps := [][]string{{"init", "test"}, {"gate", "level", "trust", "--workspace", "--by", "t"}, {"add", "prompt", "request"},
		{"add", "story", "outcome", "--parent", "PRM-001"},
		{"add", "task", "work", "--parent", "STR-001"}}
	for _, step := range steps {
		if err := run(step); err != nil {
			t.Fatalf("%v: %v", step, err)
		}
	}
	if err := run([]string{"done", "STR-001", "looks good"}); err == nil {
		t.Fatal("expected open-child error")
	}
	if err := run([]string{"done", "TSK-001"}); err == nil {
		t.Fatal("expected missing-verification error")
	}
	if err := run([]string{"done", "TSK-001", "go test passed"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"done", "STR-001", "acceptance checked"}); err != nil {
		t.Fatal(err)
	}
	w, err := load(path)
	if err != nil {
		t.Fatalf("workspace=%+v err=%v", w, err)
	}
	p := &w.Tenants[0]
	if p.Items[1].Status != "done" {
		t.Fatalf("tenant=%+v", p)
	}
}

func TestStrictOptionsAndTransitionNotes(t *testing.T) {
	path := inTempDir(t)
	for _, step := range [][]string{{"init", "strict"}, {"gate", "level", "trust", "--workspace", "--by", "t"}, {"add", "task", "work"}, {"block", "TSK-001", "waiting"}, {"ready", "TSK-001"}} {
		if err := run(step); err != nil {
			t.Fatalf("%v: %v", step, err)
		}
	}
	w, err := load(path)
	if err != nil || w.Tenants[0].Items[0].Note != "" {
		t.Fatalf("stale transition note: workspace=%+v err=%v", w, err)
	}
	for _, args := range [][]string{
		{"list", "--agent", "x"},
		{"ready", "TSK-001", "extra"},
		{"board", "-A", "-t", "strict"},
		{"dirs", "-v"},
	} {
		if err := run(args); err == nil {
			t.Fatalf("accepted invalid arguments: %v", args)
		}
	}
}

func TestFindsProjectFromChildDirectory(t *testing.T) {
	path := inTempDir(t)
	if err := runTrust([]string{"init", "test"}); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(filepath.Dir(path), "a", "b")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(child); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"add", "bug", "nested command"}); err != nil {
		t.Fatal(err)
	}
	w, err := load(path)
	if err != nil || len(w.Tenants[0].Items) != 1 {
		t.Fatalf("load=%v workspace=%+v", err, w)
	}
}

func TestFolderDefaultTenant(t *testing.T) {
	path := inTempDir(t)
	if err := runTrust([]string{"init", "workspace"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"tenant", "add", "api", "API"}); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(filepath.Dir(path), "shared")
	child := filepath.Join(folder, "src")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(folder); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"tenant", "default", "api"}); err != nil {
		t.Fatal(err)
	}
	listed := captureOutput(t, func() {
		if err := run([]string{"tenant", "list"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(listed, "tenant,items,selected") || !strings.Contains(listed, "api,0,true") {
		t.Fatalf("folder-selected tenant not visible:\n%s", listed)
	}
	if err := os.Chdir(child); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"add", "task", "folder-selected"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"add", "task", "explicitly-selected", "-t", "workspace"}); err != nil {
		t.Fatal(err)
	}
	if got := captureOutput(t, func() {
		if err := run([]string{"tenant", "default"}); err != nil {
			t.Fatal(err)
		}
	}); !strings.Contains(got, "api") {
		t.Fatalf("default not inspectable: %s", got)
	}
	if err := run([]string{"tenant", "default", "-"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"add", "task", "after-clear"}); err != nil {
		t.Fatal(err)
	}
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Tenants[0].Items) != 2 || len(w.Tenants[1].Items) != 1 {
		t.Fatalf("wrong tenant selected: %+v", w.Tenants)
	}
}

func TestMultipleTenantsAndSessionOwnership(t *testing.T) {
	path := inTempDir(t)
	steps := [][]string{
		{"init", "workspace"}, {"gate", "level", "trust", "--workspace", "--by", "t"},
		{"tenant", "add", "api", "API", "--path", filepath.Join(filepath.Dir(path), "api")},
		{"add", "story", "Reliable API", "--tenant", "api"},
		{"add", "task", "Add retries", "--tenant", "api", "--parent", "STR-001"},
		{"ready", "TSK-001", "--tenant", "api"},
		{"session", "start", "builder-1", "--tenant", "api", "--agent", "builder"},
		{"session", "start", "builder-2", "--tenant", "api", "--agent", "builder"},
		{"start", "TSK-001", "--session", "builder-1"},
	}
	for _, step := range steps {
		if err := run(step); err != nil {
			t.Fatalf("%v: %v", step, err)
		}
	}
	if err := run([]string{"start", "TSK-001", "--session", "builder-2"}); err == nil {
		t.Fatal("second session claimed owned item")
	}
	if err := run([]string{"done", "TSK-001", "verified", "--session", "builder-2"}); err == nil {
		t.Fatal("second session completed owned item")
	}
	if err := run([]string{"done", "TSK-001", "verified", "--session", "builder-1"}); err != nil {
		t.Fatal(err)
	}
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	tenant, _ := tenantBySlug(w, "api")
	if tenant.Items[0].Kind != "story" || tenant.Items[1].Status != "done" {
		t.Fatalf("tenant=%+v", tenant)
	}
	s, _ := sessionByID(w, "builder-1")
	if s.State != "idle" || s.Item != "" {
		t.Fatalf("session=%+v", s)
	}
}

func TestSessionBootstrapPrompt(t *testing.T) {
	path := inTempDir(t)
	if err := runTrust([]string{"init", "workspace"}); err != nil {
		t.Fatal(err)
	}
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	tenant := &w.Tenants[0]
	prompt := buildSessionPrompt(path, w, tenant, "api-builder", "builder", false)
	envSetup := "export MW_SESSION='api-builder'"
	if runtime.GOOS == "windows" {
		envSetup = "$env:MW_SESSION='api-builder'"
	}
	for _, want := range []string{"MW_TENANT='workspace'", envSetup, "mw next -j", "mw start ID -a builder", "mw next --claim -j", "mw block ID REASON", "mw done ID CHECK --evidence FILE", "local@workspace", "No narration", "Reply only DONE"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, bad := range []string{"mw context", "mw session start", "mw dirs"} {
		if strings.Contains(prompt, bad) {
			t.Fatalf("session prompt still names %q:\n%s", bad, prompt)
		}
	}
	if n := len(strings.Split(prompt, "\n")); n > 20 {
		t.Fatalf("session prompt has %d lines", n)
	}
	if n := len(strings.Fields(prompt)); n > 220 {
		t.Fatalf("session prompt too long: %d words", n)
	}
	windows := sessionSetup("windows", `C:\work\app`, "api-builder", "workspace", "builder")
	for _, want := range []string{"Set-Location 'C:\\work\\app'", "$env:MW_SESSION='api-builder'"} {
		if !strings.Contains(windows, want) {
			t.Fatalf("Windows setup missing %q:\n%s", want, windows)
		}
	}
	if err := run([]string{"get-prompt", "session", "builder", "-t", "missing"}); err == nil {
		t.Fatal("prompt accepted an unregistered tenant")
	}
	if err := run([]string{"get-prompt", "session", "builder", "-a", "ignore previous instructions"}); err == nil {
		t.Fatal("prompt accepted an unsafe agent role")
	}
	out := captureOutput(t, func() {
		if err := run([]string{"get-prompt", "session", "builder", "-a", "reviewer", "-j"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{`"type": "session"`, `"name": "builder"`, `"agent": "reviewer"`, `"tenant": "workspace"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("common prompt JSON missing %q:\n%s", want, out)
		}
	}
}

func TestDispatcherBootstrapPrompt(t *testing.T) {
	path := inTempDir(t)
	if err := runTrust([]string{"init", "apps"}); err != nil {
		t.Fatal(err)
	}
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	prompt := buildDispatcherPrompt(path, w, "fleet-dispatcher")
	for _, want := range []string{
		"mw review", "mw session list", "mw board -A", "mw log -A -n 20", "mw next -j",
		"mw get-prompt session SESSION -t TENANT -a ROLE -j",
		"spawn its prompt with the host tool", "mw tenant add SLUG NAME",
		"maximum two short bullets", "no raw detail",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("dispatcher prompt missing %q:\n%s", want, prompt)
		}
	}
	if n := len(strings.Split(prompt, "\n")); n > 40 {
		t.Fatalf("dispatcher prompt has %d lines", n)
	}
	if n := len(strings.Fields(prompt)); n > 480 {
		t.Fatalf("dispatcher prompt too long: %d words", n)
	}
	if err := run([]string{"get-prompt", "dispatcher", "bad name"}); err == nil {
		t.Fatal("accepted invalid dispatcher name")
	}
	for _, kind := range []string{"dispatcher", "coach"} {
		if err := run([]string{"get-prompt", kind}); err != nil {
			t.Fatalf("%s prompt: %v", kind, err)
		}
	}
	if err := run([]string{"get-prompt", "unknown"}); err == nil {
		t.Fatal("accepted unknown prompt kind")
	}
	if err := run([]string{"get-prompt", "coach", "-a", "builder"}); err == nil {
		t.Fatal("coach accepted a session-only parameter")
	}
	if err := run([]string{"get-prompt", "-k", "coach"}); err == nil {
		t.Fatal("accepted obsolete flag-based prompt syntax")
	}
	if err := run([]string{"prompt", "-k", "coach"}); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("ambiguous old command still accepted: %v", err)
	}
	if err := run([]string{"session", "list", "-t", "missing"}); err == nil {
		t.Fatal("session list accepted an unknown tenant")
	}
}

func TestShortOptionAliases(t *testing.T) {
	path := inTempDir(t)
	apiPath := filepath.Join(filepath.Dir(path), "api")
	steps := [][]string{
		{"init", "workspace"}, {"gate", "level", "trust", "--workspace", "--by", "t"}, {"tenant", "add", "api", "API", "-d", apiPath},
		{"add", "story", "Outcome", "-t", "api"},
		{"add", "task", "Work", "-t", "api", "-p", "STR-001", "-b", "detail", "-c", "check"},
		{"session", "start", "builder", "-t", "api", "-a", "builder"},
		{"ready", "TSK-001", "-t", "api"}, {"start", "TSK-001", "-s", "builder"},
	}
	for _, step := range steps {
		if err := run(step); err != nil {
			t.Fatalf("%v: %v", step, err)
		}
	}
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	tenant, _ := tenantBySlug(w, "api")
	if tenant.Path != apiPath || tenant.Items[1].Body != "detail" || tenant.Items[1].Acceptance != "check" || tenant.Items[1].Assignee != "builder" {
		t.Fatalf("aliases not applied: %+v", tenant)
	}
	o, rest, err := options([]string{"-t", "api", "-k", "task", "-S", "doing", "-j", "-v", "--", "-literal"})
	if err != nil || o["tenant"] != "api" || o["kind"] != "task" || o["status"] != "doing" || o["json"] != "true" || o["verbose"] != "true" || len(rest) != 1 || rest[0] != "-literal" {
		t.Fatalf("options=%v rest=%v err=%v", o, rest, err)
	}
}

func TestTOONDefaultJSONAndVerboseShapes(t *testing.T) {
	brief := []itemBrief{{"TSK-001", "task", "ready", "Build", ""}}
	toonOut := captureOutput(t, func() {
		if err := printData(brief, map[string]string{}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(toonOut, "[1]{id,type,state,title}:") || strings.Contains(toonOut, "{") && strings.HasPrefix(strings.TrimSpace(toonOut), "{") {
		t.Fatalf("not compact TOON:\n%s", toonOut)
	}
	var decoded []map[string]any
	if err := toon.UnmarshalString(toonOut, &decoded); err != nil || len(decoded) != 1 || decoded[0]["id"] != "TSK-001" {
		t.Fatalf("TOON round trip failed: decoded=%v err=%v", decoded, err)
	}
	empty := captureOutput(t, func() {
		if err := printData([]itemBrief{}, map[string]string{}); err != nil {
			t.Fatal(err)
		}
	})
	if strings.TrimSpace(empty) != "[]" {
		t.Fatalf("empty array is not canonical TOON: %q", empty)
	}
	jsonOut := captureOutput(t, func() {
		if err := printData(brief, map[string]string{"json": "true"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.HasPrefix(strings.TrimSpace(jsonOut), "[") || !strings.Contains(jsonOut, `"state": "ready"`) {
		t.Fatalf("not JSON:\n%s", jsonOut)
	}
	detail := detailOf(Item{ID: "TSK-001", Kind: "task", Status: "ready", Title: "Build", Body: "detail", Created: "now", Updated: "now"}, false)
	verboseOut := captureOutput(t, func() {
		if err := printData([]itemDetail{detail}, map[string]string{"verbose": "true"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(verboseOut, "body,acceptance,note,created,updated") || !strings.Contains(verboseOut, "detail") {
		t.Fatalf("verbose fields missing:\n%s", verboseOut)
	}
}

func TestCompactTextBoundsDefaultOutput(t *testing.T) {
	raw := "first\n" + strings.Repeat("x", 100)
	got := compactText(raw, 20)
	if strings.Contains(got, "\n") || len([]rune(got)) != 20 || !strings.HasSuffix(got, "…") {
		t.Fatalf("not compact: %q", got)
	}
	brief := itemBrief{"TSK-001", "task", "ready", compactText(raw, 20), ""}
	out := captureOutput(t, func() {
		if err := printData(brief, map[string]string{}); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(out, "\nxxxxxxxx") || strings.Count(out, "\n") > 5 {
		t.Fatalf("brief output expanded user text: %q", out)
	}
}

func TestConventionalDataFolders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	out := captureOutput(t, func() {
		if err := dirsCommand(nil, "", nil); err != nil {
			t.Fatal(err)
		}
	})
	var got folderPaths
	if err := toon.UnmarshalString(out, &got); err != nil {
		t.Fatalf("decode folders: %v\n%s", err, out)
	}
	want := defaultFolders(home)
	if got != want {
		t.Fatalf("folders=%+v want=%+v", got, want)
	}
	if err := dirsCommand(nil, "", []string{"init"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range folderKeys {
		if info, err := os.Stat(filepath.Join(home, name)); err != nil || !info.IsDir() {
			t.Fatalf("%s not created: %v", name, err)
		} else if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			t.Fatalf("%s permissions are not private: %o", name, info.Mode().Perm())
		}
	}
}

func home0(t *testing.T) string {
	t.Helper()
	h, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func defaultFolders(home string) folderPaths {
	j := func(n string) string { return filepath.Join(home, n) }
	return folderPaths{j("datalake"), j("docs"), j("data"), j("tools"), j("templates"), j("builds"), j("evidence"), j("scratch")}
}

func TestConfiguredFolders(t *testing.T) {
	path := inTempDir(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	abs := filepath.Join(t.TempDir(), "abs-out")
	runAll(t, []string{"init", "f"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"},
		[]string{"dirs", "set", "datalake", "lake/in"},
		[]string{"dirs", "set", "docs", "~/specs"},
		[]string{"dirs", "set", "data", abs},
		[]string{"dirs", "set", "tools", "~/t"},
		[]string{"dirs", "set", "templates", "tpl"},
		[]string{"dirs", "set", "builds", "~/b"},
		[]string{"dirs", "set", "evidence", "~/ev"},
		[]string{"dirs", "set", "scratch", "~/sc"})
	w, _ := load(path)
	want := folderPaths{filepath.Join(home, "lake", "in"), filepath.Join(home, "specs"), abs,
		filepath.Join(home, "t"), filepath.Join(home, "tpl"), filepath.Join(home, "b"), filepath.Join(home, "ev"), filepath.Join(home, "sc")}
	if got := workspaceFolders(w); got != want {
		t.Fatalf("folders=%+v want=%+v", got, want)
	}
	runAll(t, []string{"dirs", "init"})
	for _, d := range []string{want.Datalake, want.Docs, want.Data, want.Tools, want.Templates, want.Builds, want.Evidence, want.Scratch} {
		if info, err := os.Stat(d); err != nil || !info.IsDir() {
			t.Fatalf("%s not created: %v", d, err)
		}
	}
	if err := run([]string{"dirs", "set", "bogus", "x"}); err == nil {
		t.Fatal("accepted unknown folder key")
	}
	slug := w.Tenants[0].Slug
	l1 := "Folders: code in the tenant repo (plans + ADRs + test fixtures inside); shared tooling " + want.Tools + ", scaffolds " + want.Templates + " (copy, never edit in place)."
	l3 := "Inputs " + want.Datalake + ", generated outputs " + want.Data + ", shared reference/specs/plans " + want.Docs + "/<topic>. Never write elsewhere; never commit builds, evidence or scratch."
	sep := string(filepath.Separator)
	l3 = "Inputs " + want.Datalake + ", generated outputs " + want.Data + ", shared reference/specs/plans " + want.Docs + sep + "<topic>. Never write elsewhere; never commit builds, evidence or scratch."
	line := l1 + "\nArtefacts " + filepath.Join(want.Builds, slug) + sep + "<ver>; proof (screenshots, logs, crash pulls, repro fixtures) " + filepath.Join(want.Evidence, slug) + sep + "<item>; throwaway " + filepath.Join(want.Scratch, slug, "s1") + ".\n" + l3
	if p := buildSessionPrompt(path, w, &w.Tenants[0], "s1", "builder", false); !strings.Contains(p, line) {
		t.Fatalf("session prompt lacks per-tenant folders:\n%s", p)
	}
	root := l1 + "\nArtefacts " + filepath.Join(want.Builds, "<app>") + sep + "<ver>; proof (screenshots, logs, crash pulls, repro fixtures) " + filepath.Join(want.Evidence, "<app>") + sep + "<item>; throwaway " + want.Scratch + sep + "<app>.\n" + l3
	for _, p := range []string{buildDispatcherPrompt(path, w, "d"), buildCoachPrompt(path, w, "c")} {
		if !strings.Contains(p, root) {
			t.Fatalf("prompt lacks folder roots:\n%s", p)
		}
	}
	if p := buildDispatcherPrompt(path, w, "d"); !strings.Contains(p, "untrusted") || !strings.Contains(p, "A wave (mw wave) closes only after every protocol step") {
		t.Fatalf("dispatcher prompt lost wave/untrusted wording:\n%s", p)
	}
	out := captureOutput(t, func() { runAll(t, []string{"context"}) })
	if !strings.Contains(out, l1) || !strings.Contains(out, "throwaway "+filepath.Join(want.Scratch, slug)+".") || !strings.Contains(out, l3) {
		t.Fatalf("context lacks folders:\n%s", out)
	}
	// An old board without the key keeps the defaults.
	w.Folders = nil
	if got := workspaceFolders(w); got != defaultFolders(home) {
		t.Fatalf("default folders changed: %+v", got)
	}
	runAll(t, []string{"dirs", "set", "data", "~/x"})
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), `"folders"`) {
		t.Fatal("folders key not written")
	}
}

func TestNote(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "n"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"}, []string{"add", "task", "A"}, []string{"wave", "create", "W"},
		[]string{"note", "TSK-001", "found", "the", "cause"},
		[]string{"ready", "TSK-001"}, []string{"note", "WAV-001", "wave", "note"})
	w, _ := load(path)
	it, _ := itemByID(&w.Tenants[0], "TSK-001")
	wv, _ := itemByID(&w.Tenants[0], "WAV-001")
	if len(it.Notes) != 1 || !strings.HasSuffix(it.Notes[0], " found the cause") || len(wv.Notes) != 1 {
		t.Fatalf("notes: %v / %v", it.Notes, wv.Notes)
	}
	out := captureOutput(t, func() { runAll(t, []string{"show", "TSK-001", "-v", "-j"}) })
	if !strings.Contains(out, "found the cause") {
		t.Fatalf("show -v lacks note:\n%s", out)
	}
	if err := run([]string{"note", "TSK-001"}); err == nil {
		t.Fatal("accepted empty note")
	}
	if err := run([]string{"note", "TSK-999", "x"}); err == nil {
		t.Fatal("accepted unknown id")
	}
}

func TestSchemaAndPrivateState(t *testing.T) {
	path := inTempDir(t)
	if err := runTrust([]string{"init", "private"}); err != nil {
		t.Fatal(err)
	}
	w, err := load(path)
	if err != nil || w.Version != currentSchema {
		t.Fatalf("schema=%v workspace=%+v", err, w)
	}
	info, err := os.Stat(path)
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		t.Fatalf("state permissions: info=%v err=%v", info, err)
	}
	w.Version = currentSchema + 1
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	if _, err := load(path); err == nil {
		t.Fatal("accepted a future workspace schema")
	}
}

func TestCoachPromptAndCompactBoard(t *testing.T) {
	path := inTempDir(t)
	if err := runTrust([]string{"init", "apps"}); err != nil {
		t.Fatal(err)
	}
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	prompt := buildCoachPrompt(path, w, "human-coach")
	for _, want := range []string{"tenant=app", "coding sessions implement", "no routine updates", "maximum three 12-word bullets", "Never send raw logs", "Inputs " + filepath.Join(home0(t), "datalake")} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("coach prompt missing %q:\n%s", want, prompt)
		}
	}
	if n := len(strings.Fields(prompt)); n > 175 {
		t.Fatalf("coach prompt too long: %d words", n)
	}
	if n := len(strings.Fields(contextGuide)); n > 30 {
		t.Fatalf("context guide too long: %d words", n)
	}
	tenant := &w.Tenants[0]
	tenant.Items = []Item{{ID: "TSK-001", Kind: "task", Status: "ready", Title: "Act"}, {ID: "TSK-002", Kind: "task", Status: "inbox", Title: "Hidden"}, {ID: "TSK-003", Kind: "task", Status: "done", Title: "Hidden too"}}
	compact := captureOutput(t, func() { printBoard(tenant, nil, false, "") })
	if !strings.Contains(compact, "READY (1)") || strings.Contains(compact, "INBOX") || strings.Contains(compact, "DONE") {
		t.Fatalf("unexpected compact board:\n%s", compact)
	}
	full := captureOutput(t, func() { printBoard(tenant, nil, true, "") })
	if !strings.Contains(full, "INBOX (1)") || !strings.Contains(full, "DONE (1)") {
		t.Fatalf("unexpected full board:\n%s", full)
	}
}

func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	return captureFile(t, &os.Stdout, fn)
}

// captureFile swaps *f for a pipe while fn runs and returns what fn wrote.
// The pipe is drained concurrently: a Windows pipe buffers only a few KB, so
// reading after fn returns deadlocks once the output outgrows the buffer.
func captureFile(t *testing.T, f **os.File, fn func()) string {
	t.Helper()
	old := *f
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		b   []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		b, err := io.ReadAll(r)
		done <- result{b, err}
	}()
	*f = w
	func() {
		defer func() {
			_ = w.Close()
			*f = old
		}()
		fn()
	}()
	res := <-done
	_ = r.Close()
	if res.err != nil {
		t.Fatal(res.err)
	}
	return string(res.b)
}

func runAll(t *testing.T, cmds ...[]string) {
	t.Helper()
	for _, c := range cmds {
		if err := run(c); err != nil {
			t.Fatalf("%v: %v", c, err)
		}
	}
}

func TestWaves(t *testing.T) {
	path := inTempDir(t)
	runAll(t,
		[]string{"init", "Tiny app"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"},
		[]string{"wave", "create", "Milestone", "one"},
		[]string{"add", "task", "A"}, []string{"add", "bug", "B"},
		[]string{"wave", "add", "WAV-001", "TSK-001", "BUG-001"},
	)
	w, _ := load(path)
	wv, _ := itemByID(&w.Tenants[0], "WAV-001")
	if wv.Kind != "wave" || len(wv.Steps) != 11 || wv.Steps[0].Key != "audit" || wv.Steps[10].Key != "plan-audit" {
		t.Fatalf("bad wave: %+v", wv)
	}
	if m := waveMembers(&w.Tenants[0], "WAV-001"); len(m) != 2 {
		t.Fatalf("members %d", len(m))
	}
	// close refused: open members
	if err := run([]string{"wave", "close", "WAV-001"}); err == nil || !strings.Contains(err.Error(), "TSK-001") {
		t.Fatalf("close with open member: %v", err)
	}
	runAll(t, []string{"done", "TSK-001", "ok"}, []string{"done", "BUG-001", "ok"})
	// close refused: unticked steps
	if err := run([]string{"wave", "close", "WAV-001"}); err == nil || !strings.Contains(err.Error(), "step audit") {
		t.Fatalf("close with open steps: %v", err)
	}
	// note required; unknown key rejected; done refused for waves
	if err := run([]string{"wave", "step", "WAV-001", "audit"}); err == nil {
		t.Fatal("step without note accepted")
	}
	if err := run([]string{"wave", "step", "WAV-001", "nope=x"}); err == nil {
		t.Fatal("unknown step accepted")
	}
	if err := run([]string{"done", "WAV-001", "x"}); err == nil {
		t.Fatal("mw done closed a wave")
	}
	for _, s := range defaultProtocol {
		if s.Key == "install" {
			runAll(t, []string{"wave", "step", "WAV-001", "install=CLI, nothing to install", "--na"})
		} else {
			runAll(t, []string{"wave", "step", "WAV-001", s.Key + "=evidence for " + s.Key})
		}
	}
	w, _ = load(path)
	wv, _ = itemByID(&w.Tenants[0], "WAV-001")
	if !wv.Steps[7].NA || !wv.Steps[7].Done || wv.Steps[7].Note == "" {
		t.Fatalf("n/a step: %+v", wv.Steps[7])
	}
	out := captureOutput(t, func() {
		runAll(t, []string{"wave", "show", "WAV-001"}, []string{"wave", "list"}, []string{"board"}, []string{"list"})
	})
	for _, want := range []string{"checklist 11/11", "n/a: CLI", "2/2", "WAVES"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	runAll(t, []string{"wave", "close", "WAV-001"})
	w, _ = load(path)
	wv, _ = itemByID(&w.Tenants[0], "WAV-001")
	if wv.Status != "done" || !strings.HasPrefix(wv.Note, "closed ") {
		t.Fatalf("not closed: %+v", wv)
	}
	// waves never handed out by next
	runAll(t, []string{"wave", "create", "Two"})
	if err := run([]string{"next"}); err == nil {
		t.Fatal("next handed out a wave")
	}
	if err := run([]string{"wave", "remove", "WAV-001", "TSK-001"}); err == nil {
		t.Fatal("removed from a closed wave")
	}
}

func TestReparent(t *testing.T) {
	path := inTempDir(t)
	runAll(t,
		[]string{"init", "Tiny app"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"},
		[]string{"wave", "create", "Outer"},
		[]string{"wave", "create", "Inner", "-p", "WAV-001"},
		[]string{"add", "task", "A"},
	)
	if err := run([]string{"edit", "WAV-001", "-p", "WAV-002"}); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle accepted: %v", err)
	}
	if err := run([]string{"wave", "add", "WAV-002", "WAV-001"}); err == nil {
		t.Fatal("wave add cycle accepted")
	}
	if err := run([]string{"edit", "WAV-001", "-p", "WAV-001"}); err == nil {
		t.Fatal("self parent accepted")
	}
	if err := run([]string{"edit", "WAV-001", "-p", "TSK-001"}); err == nil {
		t.Fatal("wave under task accepted")
	}
	if err := run([]string{"edit", "TSK-001", "-p", "WAV-002"}); err == nil || !strings.Contains(err.Error(), "--wave") {
		t.Fatalf("reparent to wave accepted: %v", err)
	}
	runAll(t, []string{"add", "story", "S"}, []string{"edit", "TSK-001", "-p", "STR-001"})
	w, _ := load(path)
	if it, _ := itemByID(&w.Tenants[0], "TSK-001"); it.Parent != "STR-001" {
		t.Fatalf("parent %q", it.Parent)
	}
	runAll(t, []string{"edit", "TSK-001", "-p", "-"})
	w, _ = load(path)
	if it, _ := itemByID(&w.Tenants[0], "TSK-001"); it.Parent != "" {
		t.Fatalf("parent not cleared %q", it.Parent)
	}
	// hierarchy enforced by reparent
	runAll(t, []string{"add", "prompt", "P"}, []string{"add", "bug", "B"})
	for _, c := range [][]string{
		{"edit", "PRM-001", "-p", "TSK-001"}, {"edit", "STR-001", "-p", "BUG-001"},
		{"edit", "BUG-001", "-p", "PRM-001"}, {"edit", "STR-001", "-p", "STR-001"},
	} {
		if err := run(c); err == nil {
			t.Fatalf("%v accepted", c)
		}
	}
	runAll(t, []string{"edit", "STR-001", "-p", "PRM-001"})
}

func waveOf(t *testing.T, path, id string) Item {
	t.Helper()
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	it, err := itemByID(&w.Tenants[0], id)
	if err != nil {
		t.Fatal(err)
	}
	return *it
}

func TestWaveMembershipField(t *testing.T) {
	path := inTempDir(t)
	runAll(t,
		[]string{"init", "Tiny app"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"},
		[]string{"wave", "create", "M"},
		[]string{"add", "story", "S"},
		[]string{"add", "task", "A", "-p", "STR-001", "--wave", "WAV-001"},
		[]string{"add", "task", "B", "-w", "WAV-001"},
	)
	if wv := waveOf(t, path, "WAV-001"); wv.Status != "ready" {
		t.Fatalf("wave status %q", wv.Status)
	}
	if a := waveOf(t, path, "TSK-001"); a.Parent != "STR-001" || a.Wave != "WAV-001" {
		t.Fatalf("membership: %+v", a)
	}
	runAll(t, []string{"wave", "add", "WAV-001", "STR-001"})
	if s := waveOf(t, path, "STR-001"); s.Wave != "WAV-001" || s.Parent != "" {
		t.Fatalf("story: %+v", s)
	}
	runAll(t, []string{"wave", "remove", "WAV-001", "TSK-001"})
	if a := waveOf(t, path, "TSK-001"); a.Parent != "STR-001" || a.Wave != "" {
		t.Fatalf("after remove: %+v", a)
	}
	// -p WAV is an error
	if err := run([]string{"add", "task", "C", "-p", "WAV-001"}); err == nil || !strings.Contains(err.Error(), "--wave") {
		t.Fatalf("add -p WAV: %v", err)
	}
	// -A wave list
	out := captureOutput(t, func() { runAll(t, []string{"wave", "list", "-A"}) })
	if !strings.Contains(out, "tiny-app:WAV-001") || !strings.Contains(out, "0/2") {
		t.Fatalf("-A list:\n%s", out)
	}
	// state guards
	for _, c := range []string{"start", "ready", "unclaim"} {
		if err := run([]string{c, "WAV-001"}); err == nil {
			t.Fatalf("%s on wave accepted", c)
		}
	}
	if err := run([]string{"block", "WAV-001", "why"}); err == nil {
		t.Fatal("block on wave accepted")
	}
	if err := run([]string{"reopen", "WAV-001"}); err == nil {
		t.Fatal("reopen of an open wave accepted")
	}
	// unstep
	runAll(t, []string{"wave", "step", "WAV-001", "audit=ok"})
	if waveOf(t, path, "WAV-001").Steps[0].Done != true {
		t.Fatal("step not ticked")
	}
	runAll(t, []string{"wave", "unstep", "WAV-001", "audit"})
	if st := waveOf(t, path, "WAV-001").Steps[0]; st.Done || st.Note != "" {
		t.Fatalf("unstep: %+v", st)
	}
	// close a wave, then guards on the closed wave
	runAll(t, []string{"wave", "remove", "WAV-001", "STR-001", "TSK-002"})
	for _, s := range defaultProtocol {
		runAll(t, []string{"wave", "step", "WAV-001", s.Key + "=ok"})
	}
	runAll(t, []string{"wave", "close", "WAV-001"})
	for _, c := range [][]string{
		{"wave", "add", "WAV-001", "TSK-001"}, {"add", "task", "D", "--wave", "WAV-001"},
		{"wave", "step", "WAV-001", "audit=x"}, {"wave", "unstep", "WAV-001", "audit"},
	} {
		if err := run(c); err == nil {
			t.Fatalf("%v accepted on closed wave", c)
		}
	}
	runAll(t, []string{"reopen", "WAV-001"})
	wv := waveOf(t, path, "WAV-001")
	if wv.Status != "ready" || wv.Note != "" {
		t.Fatalf("reopen: %+v", wv)
	}
	for _, s := range wv.Steps {
		if s.Done || s.Note != "" || s.When != "" {
			t.Fatalf("step not reset: %+v", s)
		}
	}
}

func TestOldBoardAndProtocolOverride(t *testing.T) {
	path := inTempDir(t)
	old := `{"version":3,"name":"Old","active_tenant":"old","tenants":[{"slug":"old","name":"Old","next":{"TSK":1},"items":[{"id":"TSK-001","kind":"task","title":"x","status":"ready","created":"a","updated":"a"}]}],"sessions":[]}`
	if err := os.WriteFile(path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	runAll(t, []string{"migrate"})
	runAll(t, []string{"wave", "create", "W"}, []string{"wave", "add", "WAV-001", "TSK-001"})
	if wv := waveOf(t, path, "WAV-001"); len(wv.Steps) != 11 {
		t.Fatalf("default steps %d", len(wv.Steps))
	}
	// board-level protocol override
	b, _ := os.ReadFile(path)
	s := strings.Replace(string(b), `"tenants"`, `"protocol":["Plan audit","ship"],"tenants"`, 1)
	if err := os.WriteFile(path, []byte(s), 0600); err != nil {
		t.Fatal(err)
	}
	runAll(t, []string{"wave", "create", "W2"})
	wv := waveOf(t, path, "WAV-002")
	if len(wv.Steps) != 2 || wv.Steps[0].Key != "plan-audit" || wv.Steps[0].Title != "Plan audit" || wv.Steps[1].Key != "ship" {
		t.Fatalf("override: %+v", wv.Steps)
	}
	if w, _ := load(path); len(w.Protocol) != 2 {
		t.Fatalf("protocol not persisted: %v", w.Protocol)
	}
}

func TestListIsOneTOONDocument(t *testing.T) {
	inTempDir(t)
	runAll(t, []string{"init", "Tiny app"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"}, []string{"add", "task", "A"}, []string{"wave", "create", "M"})
	out := captureOutput(t, func() { runAll(t, []string{"list"}) })
	if strings.Count(out, "\nwaves") != 1 && !strings.HasPrefix(out, "waves") {
		t.Fatalf("no waves key:\n%s", out)
	}
	if !strings.HasPrefix(out, "items") || strings.Contains(out, "\n\n") {
		t.Fatalf("not a single document:\n%q", out)
	}
	if !strings.Contains(out, "TSK-001") || !strings.Contains(out, "WAV-001") {
		t.Fatalf("missing rows:\n%s", out)
	}
	// only a wave: still one document
	out = captureOutput(t, func() { runAll(t, []string{"list", "-k", "wave"}) })
	if strings.Contains(out, "\n\n") || !strings.Contains(out, "WAV-001") {
		t.Fatalf("wave-only list:\n%q", out)
	}
}

func wantErr(t *testing.T, sub string, args ...string) {
	t.Helper()
	if err := run(args); err == nil || !strings.Contains(err.Error(), sub) {
		t.Fatalf("%v: want error containing %q, got %v", args, sub, err)
	}
}

func closeWave(t *testing.T, path, id string) {
	t.Helper()
	w, _ := load(path)
	wv, _ := itemByID(&w.Tenants[0], id)
	for i := range wv.Steps {
		wv.Steps[i].Done = true
	}
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	runAll(t, []string{"wave", "close", id})
}

func TestClosedWaveAndMembershipGuards(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "Tiny app"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"}, []string{"add", "task", "A"}, []string{"add", "task", "B"},
		[]string{"wave", "create", "One"}, []string{"wave", "create", "Two"},
		[]string{"wave", "add", "WAV-001", "TSK-001"}, []string{"done", "TSK-001", "ok"})
	// one wave per item
	wantErr(t, "remove from WAV-001 first", "wave", "add", "WAV-002", "TSK-001")
	closeWave(t, path, "WAV-001")
	// closed wave: no remove, no new members, members cannot be revived
	wantErr(t, "closed", "wave", "remove", "WAV-001", "TSK-001")
	wantErr(t, "closed", "wave", "add", "WAV-001", "TSK-002")
	wantErr(t, "closed", "add", "task", "C", "--wave", "WAV-001")
	for _, c := range []string{"reopen", "ready", "start"} {
		wantErr(t, "wave WAV-001 is closed; reopen it first", c, "TSK-001")
	}
	// closed parent wave
	wantErr(t, "closed", "wave", "create", "Child", "-p", "WAV-001")
	runAll(t, []string{"wave", "create", "Child", "-p", "WAV-002"})
	wantErr(t, "closed", "edit", "WAV-003", "-p", "WAV-001")
	// parent cannot close while a child wave is open
	wantErr(t, "child wave WAV-003", "wave", "close", "WAV-002")
}

func TestProtocolDedupeAndBlank(t *testing.T) {
	w := &Workspace{Protocol: []string{"Test", " ", "test", "Ship it", ""}}
	st := protocolSteps(w, nil)
	if len(st) != 2 || st[0].Key != "test" || st[1].Key != "ship-it" {
		t.Fatalf("%+v", st)
	}
	w.Protocol = []string{"", "  "}
	if st := protocolSteps(w, nil); len(st) != len(defaultProtocol) {
		t.Fatalf("no fallback: %+v", st)
	}
}

func TestListJSONShapeWithoutWaves(t *testing.T) {
	inTempDir(t)
	runAll(t, []string{"init", "Tiny app"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"}, []string{"add", "task", "A"})
	out := captureOutput(t, func() { runAll(t, []string{"list", "-j"}) })
	var doc struct {
		Items []map[string]any `json:"items"`
		Waves []map[string]any `json:"waves"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || len(doc.Items) != 1 || doc.Waves == nil || len(doc.Waves) != 0 {
		t.Fatalf("bad shape %q: %v", out, err)
	}
	if txt := captureOutput(t, func() { runAll(t, []string{"list"}) }); strings.Contains(txt, "waves") {
		t.Fatalf("text output changed:\n%s", txt)
	}
}

func TestMain(m *testing.M) {
	syncWrites = false
	os.Exit(m.Run())
}

func TestEventOrderAndCoverage(t *testing.T) { // F8 F9
	path, _ := gateEnvTrust(t)
	runAll(t, []string{"add", "story", "S", "-c", "x"}, []string{"add", "task", "A", "-p", "STR-001", "-S", "ready"})
	runAll(t, []string{"start", "TSK-001"})
	st := itemOf(t, path, "STR-001")
	if last := st.Events[len(st.Events)-1]; last.Action != "start" || !strings.Contains(last.Note, "TSK-001") {
		t.Fatalf("story start not logged: %+v", st.Events)
	}
	if !strings.Contains(itemOf(t, path, "TSK-001").Events[0].At, ".") {
		t.Fatal("event stamp has no sub-second part")
	}
	runAll(t, []string{"wave", "create", "W"}, []string{"wave", "add", "WAV-001", "TSK-001"}, []string{"wave", "remove", "WAV-001", "TSK-001"})
	ev := itemOf(t, path, "TSK-001").Events
	if a, b := ev[len(ev)-2].Action, ev[len(ev)-1].Action; a != "wave-add" || b != "wave-remove" {
		t.Fatalf("%+v", ev)
	}
	// reject cascade: Fix child gets created+ready, the story a reject event
	runAll(t, []string{"gate", "judge", "on", "--by", "h"}, []string{"done", "TSK-001", "ok"}, []string{"reject", "STR-001", "bad", "--by", "h"})
	w := loadT(t, path)
	fix, _ := itemByID(&w.Tenants[0], "TSK-002")
	if len(fix.Events) != 2 || fix.Events[0].Action != "created" || fix.Events[1].Action != "ready" {
		t.Fatalf("%+v", fix.Events)
	}
	// log orders same-second ties by time
	out := captureOutput(t, func() { runAll(t, []string{"log", "-n", "1"}) })
	if !strings.Contains(out, "TSK-002") {
		t.Fatal(out)
	}
}

func TestBoardWithoutActiveTenantField(t *testing.T) { // F11
	path, _ := gateEnv(t)
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "active_tenant") {
		t.Fatal("active_tenant still written")
	}
}

func TestVersion051(t *testing.T) {
	if version != "0.3.1" || currentSchema != 9 {
		t.Fatal(version, currentSchema)
	}
}

func TestLogActivityFeed(t *testing.T) {
	path, _ := gateEnvTrust(t)
	runAll(t, []string{"add", "task", "A", "-S", "ready"}, []string{"start", "TSK-001", "-s", "bot"},
		[]string{"note", "TSK-001", "halfway", "-s", "bot"}, []string{"done", "TSK-001", "all checked", "-s", "bot"})
	var rows []logRow
	out := captureOutput(t, func() { runAll(t, []string{"log", "-j"}) })
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 5 {
		t.Fatalf("%v %s", err, out)
	}
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Action)
	}
	if len(got) != 5 || got[0] != "done" || rows[0].By != "bot" || rows[0].Note != "all checked" || rows[0].Tenant != "gate-app" {
		t.Fatalf("%v %+v", got, rows[0])
	}
	if out := captureOutput(t, func() { runAll(t, []string{"log", "-n", "1"}) }); strings.Count(out, "\n") != 1 || !strings.Contains(out, "TSK-001") {
		t.Fatal(out)
	}
	runAll(t, []string{"tenant", "add", "other"}, []string{"add", "bug", "B", "-t", "other"})
	if out := captureOutput(t, func() { runAll(t, []string{"log"}) }); strings.Contains(out, "BUG-001") {
		t.Fatal(out)
	}
	out = captureOutput(t, func() { runAll(t, []string{"log", "-A"}) })
	if !strings.Contains(out, "BUG-001") || !strings.Contains(out, "done") || !strings.Contains(out, "gate-app") {
		t.Fatal(out)
	}
	wantErr(t, "-A cannot", "log", "-A", "-t", "other")
	_ = path
}

func TestShowPrintsEverythingWithoutV(t *testing.T) {
	_, home := judgeBoard(t)
	runAll(t, []string{"start", "TSK-001", "-s", "bot"}, []string{"note", "TSK-001", "found it", "-s", "bot"})
	out := captureOutput(t, func() { runAll(t, []string{"show", "TSK-001"}) })
	for _, want := range []string{"TSK-001  task  doing", "parent: STR-001", "session: bot", "acceptance (from STR-001):", "opens fast",
		"evidence: " + filepath.Join(home, "evidence", "gate-app", "TSK-001") + " (0 files)", "notes:", "[bot] found it", "events:", "start"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "T") && strings.Contains(out, "Z ") {
		t.Fatalf("raw timestamps without -v:\n%s", out)
	}
	out = captureOutput(t, func() { runAll(t, []string{"show", "STR-001"}) })
	if !strings.Contains(out, "children:") || !strings.Contains(out, "TSK-001  task    doing") || !strings.Contains(out, "TSK-002") {
		t.Fatal(out)
	}
	if v := captureOutput(t, func() { runAll(t, []string{"show", "TSK-001", "-v"}) }); !strings.Contains(v, "Z  ") || !strings.Contains(v, "created: 20") {
		t.Fatal(v)
	}
	var d briefDoc
	j := captureOutput(t, func() { runAll(t, []string{"show", "TSK-001", "-j"}) })
	if err := json.Unmarshal([]byte(j), &d); err != nil || d.AcceptanceFrom != "STR-001" || len(d.Notes) != 1 || len(d.Events) == 0 || d.EvidenceDir == "" {
		t.Fatalf("%v %s", err, j)
	}
	// the events tail is bounded
	for i := 0; i < 8; i++ {
		runAll(t, []string{"note", "TSK-001", "n", "-s", "bot"})
	}
	if err := json.Unmarshal([]byte(captureOutput(t, func() { runAll(t, []string{"show", "TSK-001", "-j"}) })), &d); err != nil || len(d.Events) != eventsTail {
		t.Fatalf("%v %d", err, len(d.Events))
	}
}

func TestNextIsTheShowBrief(t *testing.T) {
	judgeBoard(t)
	next := captureOutput(t, func() { runAll(t, []string{"next", "-k", "task"}) })
	show := captureOutput(t, func() { runAll(t, []string{"show", "TSK-001"}) })
	if !strings.HasPrefix(next, show) {
		t.Fatalf("next:\n%s\nshow:\n%s", next, show)
	}
	var d nextBriefJSON
	if err := json.Unmarshal([]byte(captureOutput(t, func() { runAll(t, []string{"next", "-k", "task", "-j"}) })), &d); err != nil || d.ID != "TSK-001" || d.Acceptance == "" || d.Gates == nil {
		t.Fatalf("%v %+v", err, d)
	}
}

func TestBoardAllSortedByActivityWithLastAndToday(t *testing.T) {
	_, _ = gateEnvTrust(t)
	runAll(t, []string{"tenant", "add", "quiet"}, []string{"tenant", "add", "busy"})
	runAll(t, []string{"add", "task", "Q", "-t", "quiet"})
	w := loadT(t, ".miniwaves.json")
	_ = w
	// age the quiet tenant's events and gate-app's, so busy is newest
	path := findWS(t)
	ws := loadT(t, path)
	for i := range ws.Tenants {
		if ws.Tenants[i].Slug != "busy" {
			for j := range ws.Tenants[i].Items {
				for k := range ws.Tenants[i].Items[j].Events {
					ws.Tenants[i].Items[j].Events[k].At = "2020-01-01T00:00:00Z"
				}
			}
		}
	}
	save(path, ws)
	runAll(t, []string{"add", "task", "T", "-t", "busy", "-S", "ready"}, []string{"start", "TSK-001", "-t", "busy"}, []string{"done", "TSK-001", "ok", "-t", "busy"})
	out := captureOutput(t, func() { runAll(t, []string{"board", "-A"}) })
	ib, iq := strings.Index(out, "[busy]"), strings.Index(out, "[quiet]")
	if ib < 0 || iq < 0 || ib > iq {
		t.Fatalf("not sorted by activity:\n%s", out)
	}
	if !strings.Contains(out, "last: now TSK-001 done  today: 1 done") || !strings.Contains(out, "last: ") {
		t.Fatal(out)
	}
	if !strings.Contains(out, "last: 2000d ago") && !strings.Contains(out, "d ago TSK-001 created  today: 0 done") {
		t.Fatal(out)
	}
	var docs []boardDoc
	if err := json.Unmarshal([]byte(captureOutput(t, func() { runAll(t, []string{"board", "-A", "-j"}) })), &docs); err != nil || docs[0].Tenant != "busy" || docs[0].Today != 1 || docs[0].Last != "TSK-001 done" {
		t.Fatalf("%v %+v", err, docs)
	}
}

func findWS(t *testing.T) string {
	p, err := findWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestEditDraftingAndParent(t *testing.T) {
	path, _ := gateEnv(t)
	runAll(t, []string{"add", "story", "S"}, []string{"add", "task", "T"})
	runAll(t, []string{"edit", "STR-001", "-c", "draft", "-s", "bot"}) // empty: a session may draft
	wantErr(t, "for humans", "edit", "STR-001", "-c", "changed", "-s", "bot")
	runAll(t, []string{"edit", "TSK-001", "-p", "STR-001", "-s", "bot"})
	if itemOf(t, path, "TSK-001").Parent != "STR-001" {
		t.Fatal("parent not set")
	}
	runAll(t, []string{"edit", "TSK-001", "-p", "-"})
	if itemOf(t, path, "TSK-001").Parent != "" {
		t.Fatal("parent not cleared")
	}
}

func TestRemovedSurfaces(t *testing.T) {
	gateEnv(t)
	wantErr(t, "unknown command", "burndown")
	wantErr(t, "unknown command", "reparent", "TSK-001", "-")
	wantErr(t, "unknown tenant command", "tenant", "use", "x")
}

// runTrust runs an init command, then sets level trust (the tests that use it
// predate the proof default and exercise behaviour with the gates off).
func runTrust(args []string) error {
	if err := run(args); err != nil {
		return err
	}
	return run([]string{"gate", "level", "trust", "--workspace", "--by", "t"})
}
