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
	if err := os.Chdir(d); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return filepath.Join(d, dataFile)
}

func TestWorkflow(t *testing.T) {
	path := inTempDir(t)
	steps := [][]string{
		{"init", "Tiny app"},
		{"add", "prompt", "Make it fast", "--status", "ready"},
		{"add", "story", "Instant startup", "--parent", "PRM-001", "--accept", "opens under 100ms"},
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
	if err := run([]string{"init", "test"}); err != nil {
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
	steps := [][]string{{"init", "test"}, {"add", "prompt", "request"},
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
	for _, step := range [][]string{{"init", "strict"}, {"add", "task", "work"}, {"block", "TSK-001", "waiting"}, {"ready", "TSK-001"}} {
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
	if err := run([]string{"init", "test"}); err != nil {
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
	if err := run([]string{"init", "workspace"}); err != nil {
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
	if w.Active != "workspace" {
		t.Fatalf("folder default changed global fallback: %s", w.Active)
	}
}

func TestMultipleTenantsAndSessionOwnership(t *testing.T) {
	path := inTempDir(t)
	steps := [][]string{
		{"init", "workspace"},
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
	if err := run([]string{"init", "workspace"}); err != nil {
		t.Fatal(err)
	}
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	tenant := &w.Tenants[0]
	prompt := buildSessionPrompt(path, tenant, "api-builder", "builder")
	envSetup := "export MW_SESSION='api-builder'"
	if runtime.GOOS == "windows" {
		envSetup = "$env:MW_SESSION='api-builder'"
	}
	for _, want := range []string{"mw session start 'api-builder'", "-t 'workspace'", envSetup, "mw context", "mw block ID REASON", "mw done ID CHECK", "No narration"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if n := len(strings.Fields(prompt)); n > 80 {
		t.Fatalf("session prompt too long: %d words", n)
	}
	windows := sessionSetup("windows", `C:\work\app`, "api-builder", "workspace", "builder")
	for _, want := range []string{"Set-Location 'C:\\work\\app'", "$env:MW_SESSION='api-builder'", "mw context"} {
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
	if err := run([]string{"init", "apps"}); err != nil {
		t.Fatal(err)
	}
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	prompt := buildDispatcherPrompt(path, w, "fleet-dispatcher")
	for _, want := range []string{
		"mw tenant list", "mw session list", "mw board -A",
		"mw get-prompt session SESSION -t TENANT -a ROLE",
		"spawn its output with the host tool", "mw tenant add SLUG NAME",
		"maximum two short bullets", "no raw detail",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("dispatcher prompt missing %q:\n%s", want, prompt)
		}
	}
	if n := len(strings.Fields(prompt)); n > 125 {
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
		{"init", "workspace"}, {"tenant", "add", "api", "API", "-d", apiPath},
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
	brief := []itemBrief{{"TSK-001", "task", "ready", "Build"}}
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
	detail := detailOf(Item{ID: "TSK-001", Kind: "task", Status: "ready", Title: "Build", Body: "detail", Created: "now", Updated: "now"})
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
	brief := itemBrief{"TSK-001", "task", "ready", compactText(raw, 20)}
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
		if err := dirsCommand(nil); err != nil {
			t.Fatal(err)
		}
	})
	var got folderPaths
	if err := toon.UnmarshalString(out, &got); err != nil {
		t.Fatalf("decode folders: %v\n%s", err, out)
	}
	want := folderPaths{filepath.Join(home, "datalake"), filepath.Join(home, "docs"), filepath.Join(home, "data")}
	if got != want {
		t.Fatalf("folders=%+v want=%+v", got, want)
	}
	if err := dirsCommand([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"datalake", "docs", "data"} {
		if info, err := os.Stat(filepath.Join(home, name)); err != nil || !info.IsDir() {
			t.Fatalf("%s not created: %v", name, err)
		} else if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			t.Fatalf("%s permissions are not private: %o", name, info.Mode().Perm())
		}
	}
}

func TestSchemaAndPrivateState(t *testing.T) {
	path := inTempDir(t)
	if err := run([]string{"init", "private"}); err != nil {
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
	if err := run([]string{"init", "apps"}); err != nil {
		t.Fatal(err)
	}
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	prompt := buildCoachPrompt(path, w, "human-coach")
	for _, want := range []string{"tenant=app", "coding sessions implement", "no routine updates", "maximum three 12-word bullets", "Never send raw logs", "consumed=~/datalake"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("coach prompt missing %q:\n%s", want, prompt)
		}
	}
	if n := len(strings.Fields(prompt)); n > 115 {
		t.Fatalf("coach prompt too long: %d words", n)
	}
	if n := len(strings.Fields(contextGuide)); n > 30 {
		t.Fatalf("context guide too long: %d words", n)
	}
	tenant := &w.Tenants[0]
	tenant.Items = []Item{{ID: "TSK-001", Kind: "task", Status: "ready", Title: "Act"}, {ID: "TSK-002", Kind: "task", Status: "inbox", Title: "Hidden"}, {ID: "TSK-003", Kind: "task", Status: "done", Title: "Hidden too"}}
	compact := captureOutput(t, func() { printBoard(tenant, nil, false) })
	if !strings.Contains(compact, "READY (1)") || strings.Contains(compact, "INBOX") || strings.Contains(compact, "DONE") {
		t.Fatalf("unexpected compact board:\n%s", compact)
	}
	full := captureOutput(t, func() { printBoard(tenant, nil, true) })
	if !strings.Contains(full, "INBOX (1)") || !strings.Contains(full, "DONE (1)") {
		t.Fatalf("unexpected full board:\n%s", full)
	}
}

func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	return string(b)
}
