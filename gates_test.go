package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()
	fn()
	_ = w.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}

// gateEnv makes a temp board and home; evidence lives in <home>/evidence.
func gateEnv(t *testing.T) (path, home string) {
	t.Helper()
	path = inTempDir(t)
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	stdinIsTerminal = func() bool { return false }
	t.Cleanup(func() { stdinIsTerminal = func() bool { return false } })
	runAll(t, []string{"init", "Gate app"})
	return path, home
}

// gateEnvTrust is gateEnv with the workspace default at level trust (every
// gate off), for tests of behaviour that predate the proof default.
func gateEnvTrust(t *testing.T) (path, home string) {
	t.Helper()
	path, home = gateEnv(t)
	runAll(t, []string{"gate", "level", "trust", "--workspace", "--by", "test"})
	return path, home
}

func gateSet(t *testing.T, name, setting string) {
	t.Helper()
	runAll(t, []string{"gate", name, setting, "--by", "test"})
}

func putEvidence(t *testing.T, home, id string, files map[string]string) string {
	t.Helper()
	d := filepath.Join(home, "evidence", "gate-app", id)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	for n, c := range files {
		if err := os.WriteFile(filepath.Join(d, n), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func itemOf(t *testing.T, path, id string) Item {
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

func TestEvidenceGateOffAndOn(t *testing.T) {
	path, home := gateEnv(t)
	runAll(t, []string{"add", "task", "A"}, []string{"add", "task", "B"})
	// default proof: even a non-product tenant needs evidence
	wantErr(t, "evidence required", "done", "TSK-001", "ok")
	gateSet(t, "evidence", "off")
	// off: done needs nothing, judged ungated
	runAll(t, []string{"done", "TSK-001", "ok"})
	if it := itemOf(t, path, "TSK-001"); it.Judged != "ungated" || len(it.Evidence) != 0 {
		t.Fatalf("%+v", it)
	}
	gateSet(t, "evidence", "on")
	wantErr(t, "evidence required", "done", "TSK-002", "ok")
	putEvidence(t, home, "TSK-002", map[string]string{"shot.txt": "proof"})
	runAll(t, []string{"done", "TSK-002", "ok"})
	it := itemOf(t, path, "TSK-002")
	if len(it.Evidence) != 1 || it.Evidence[0] != "shot.txt 5" {
		t.Fatalf("%+v", it)
	}
	out := captureOutput(t, func() { runAll(t, []string{"show", "TSK-002", "-v"}) })
	if !strings.Contains(out, "shot.txt 5") {
		t.Fatalf("show -v lacks evidence:\n%s", out)
	}
	out = captureOutput(t, func() { runAll(t, []string{"list", "-v", "-j"}) })
	if !strings.Contains(out, `"e": "E"`) || strings.Contains(out, "shot.txt") {
		t.Fatalf("list -v lacks E:\n%s", out)
	}
	// reopen clears, done records again
	runAll(t, []string{"reopen", "TSK-002"})
	if it := itemOf(t, path, "TSK-002"); len(it.Evidence) != 0 || it.Judged != "" {
		t.Fatalf("reopen kept: %+v", it)
	}
	runAll(t, []string{"done", "TSK-002", "again"})
	if it := itemOf(t, path, "TSK-002"); len(it.Evidence) != 1 {
		t.Fatalf("done did not re-record: %+v", it)
	}
	// off again: no check
	gateSet(t, "evidence", "off")
	runAll(t, []string{"add", "task", "C"}, []string{"done", "TSK-003", "ok"})
}

func TestEvidenceFolderRules(t *testing.T) {
	path, home := gateEnv(t)
	gateSet(t, "evidence", "on")
	runAll(t, []string{"add", "task", "A"}, []string{"add", "task", "B"}, []string{"add", "task", "C"}, []string{"add", "task", "D"})
	// only junk files
	putEvidence(t, home, "TSK-001", map[string]string{".DS_Store": "x", "Thumbs.db": "x", "desktop.ini": "x", "empty.png": ""})
	wantErr(t, "no non-empty file", "done", "TSK-001", "ok")
	// wrong case folder
	d := filepath.Join(home, "evidence", "gate-app", "tsk-002")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantErr(t, "wrong case", "done", "TSK-002", "ok")
	// no folder at all, then --no-evidence stores a note
	wantErr(t, "evidence required", "done", "TSK-003", "ok")
	runAll(t, []string{"done", "TSK-003", "ok", "--no-evidence", "pure refactor"})
	it := itemOf(t, path, "TSK-003")
	if len(it.Notes) != 1 || !strings.Contains(it.Notes[0], "no-evidence: pure refactor") {
		t.Fatalf("%+v", it)
	}
	wantErr(t, "needs a reason", "done", "TSK-004", "ok", "--no-evidence", " ")
	// a real file beside junk passes
	putEvidence(t, home, "TSK-001", map[string]string{"log.txt": "ok"})
	runAll(t, []string{"done", "TSK-001", "ok"})
}

func TestWaveEvidenceAtClose(t *testing.T) {
	path, home := gateEnv(t)
	runAll(t, []string{"wave", "create", "M"})
	gateSet(t, "evidence", "on")
	for _, s := range defaultProtocol {
		runAll(t, []string{"wave", "step", "WAV-001", s.Key + "=ok"})
	}
	wantErr(t, "evidence required", "wave", "close", "WAV-001")
	putEvidence(t, home, "WAV-001", map[string]string{"notes.txt": "ok"})
	runAll(t, []string{"wave", "close", "WAV-001"})
	if wv := itemOf(t, path, "WAV-001"); wv.Status != "done" || len(wv.Evidence) != 1 {
		t.Fatalf("%+v", wv)
	}
	runAll(t, []string{"reopen", "WAV-001"})
	if wv := itemOf(t, path, "WAV-001"); len(wv.Evidence) != 0 {
		t.Fatalf("%+v", wv)
	}
	for _, s := range defaultProtocol {
		runAll(t, []string{"wave", "step", "WAV-001", s.Key + "=ok"})
	}
	if err := os.RemoveAll(filepath.Join(home, "evidence")); err != nil {
		t.Fatal(err)
	}
	runAll(t, []string{"wave", "close", "WAV-001", "--no-evidence", "docs only"})
	// gate off: no folder check
	runAll(t, []string{"wave", "create", "N"})
	gateSet(t, "evidence", "off")
	for _, s := range defaultProtocol {
		runAll(t, []string{"wave", "step", "WAV-002", s.Key + "=ok"})
	}
	runAll(t, []string{"wave", "close", "WAV-002"})
}

func TestAcceptanceGateAndInheritance(t *testing.T) {
	path, _ := gateEnvTrust(t)
	runAll(t, []string{"add", "prompt", "P", "-c", "prompt-level check"},
		[]string{"add", "story", "S", "-p", "PRM-001"},
		[]string{"add", "task", "T", "-p", "STR-001", "-S", "ready"},
		[]string{"add", "bug", "B", "-S", "ready"})
	// auto = off, whatever has acceptance
	out := captureOutput(t, func() { runAll(t, []string{"next"}) })
	if strings.Contains(out, "skipped") {
		t.Fatalf("gate should be off:\n%s", out)
	}
	gateSet(t, "acceptance", "on")
	w, _ := load(path)
	tn := &w.Tenants[0]
	task, _ := itemByID(tn, "TSK-001")
	if got := effectiveAcceptance(tn, *task); got != "prompt-level check" {
		t.Fatalf("two-level inheritance: %q", got)
	}
	out = captureOutput(t, func() { runAll(t, []string{"next"}) })
	if !strings.Contains(out, "TSK-001  task") || !strings.Contains(out, "prompt-level check") || strings.Contains(out, "skipped") {
		t.Fatalf("next:\n%s", out)
	}
	// a standalone bug without acceptance is handed out too
	runAll(t, []string{"start", "TSK-001"}, []string{"done", "TSK-001", "ok"})
	out = captureOutput(t, func() { runAll(t, []string{"next", "-j"}) })
	var doc struct {
		ID      string          `json:"id"`
		Gates   map[string]bool `json:"gates"`
		Skipped int             `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || doc.ID != "BUG-001" || !doc.Gates["acceptance"] || doc.Skipped != 0 {
		t.Fatalf("%v %s", err, out)
	}
}

func TestAcceptanceOnSkipsOnlyPromptsAndStoriesWithoutAcceptance(t *testing.T) {
	_, _ = gateEnvTrust(t)
	runAll(t, []string{"add", "story", "Bare", "-S", "ready"}, []string{"add", "chore", "C", "-S", "ready"})
	gateSet(t, "acceptance", "on")
	out := captureOutput(t, func() { runAll(t, []string{"next"}) })
	if !strings.Contains(out, "CHR-001  chore") {
		t.Fatalf("chore not handed out:\n%s", out)
	}
	runAll(t, []string{"start", "CHR-001"}, []string{"done", "CHR-001", "ok"})
	wantErr(t, "skipped 1 without acceptance", "next", "-k", "story")
	runAll(t, []string{"edit", "STR-001", "-c", "works"})
	if out = captureOutput(t, func() { runAll(t, []string{"next", "-k", "story"}) }); !strings.Contains(out, "STR-001  story") {
		t.Fatalf("%s", out)
	}
}

func TestAcceptanceAutoNeverLatches(t *testing.T) {
	path, _ := gateEnv(t)
	runAll(t, []string{"add", "story", "S", "--acceptance", "works"})
	w, _ := load(path)
	if len(w.Tenants[0].GateLatch) != 0 || gateOn(w, &w.Tenants[0], "acceptance") {
		t.Fatalf("latched: %+v", w.Tenants[0].GateLatch)
	}
	g := byName(gateStates(w, &w.Tenants[0]), "acceptance")
	if g.Setting != "auto" || g.Effective {
		t.Fatalf("%+v", g)
	}
}

func TestJudgeAutoIsOffEvenForProductTenant(t *testing.T) {
	path, _ := gateEnv(t)
	w, _ := load(path)
	if err := os.WriteFile(filepath.Join(w.Tenants[0].Path, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runAll(t, []string{"add", "task", "x"})
	out := captureOutput(t, func() { runAll(t, []string{"gate"}) })
	if !strings.Contains(out, "auto = off; set on to require human accept") {
		t.Fatalf("%s", out)
	}
	w, _ = load(path)
	if gateOn(w, &w.Tenants[0], "judge") || w.Tenants[0].GateLatch["judge"] != "" {
		t.Fatal("judge turned on by itself")
	}
	if !gateOn(w, &w.Tenants[0], "evidence") {
		t.Fatal("evidence rule must stay")
	}
}

func TestNotesShownWithoutVerboseAndAuthored(t *testing.T) {
	_, _ = gateEnv(t)
	runAll(t, []string{"add", "chore", "C"}, []string{"session", "start", "sess1"}, []string{"session", "start", "envsess"}, []string{"note", "CHR-001", "first", "-s", "sess1"})
	t.Setenv("MW_SESSION", "envsess")
	runAll(t, []string{"note", "CHR-001", "second"})
	out := captureOutput(t, func() { runAll(t, []string{"show", "CHR-001"}) })
	if !strings.Contains(out, "[sess1] first") || !strings.Contains(out, "[envsess] second") || !strings.Contains(out, "notes:") {
		t.Fatalf("%s", out)
	}
}

func TestGateAndEditRefusedForSessions(t *testing.T) {
	gateEnv(t)
	runAll(t, []string{"add", "task", "A"}, []string{"session", "start", "bot"})
	wantErr(t, "for humans", "gate", "judge", "on", "--by", "x", "-s", "bot")
	wantErr(t, "for humans", "gate", "-s", "bot")
	runAll(t, []string{"edit", "TSK-001", "-c", "drafted by a session", "-s", "bot"}) // empty acceptance: drafting is open
	wantErr(t, "for humans", "edit", "TSK-001", "-c", "x", "-s", "bot")
	t.Setenv("MW_SESSION", "bot")
	wantErr(t, "for humans", "gate", "judge", "on", "--by", "x")
	wantErr(t, "for humans", "edit", "TSK-001", "-c", "x")
	runAll(t, []string{"edit", "TSK-001", "-b", "body is fine for sessions"})
	t.Setenv("MW_SESSION", "")
	wantErr(t, "--by", "gate", "judge", "on") // no terminal on stdin
	stdinIsTerminal = func() bool { return true }
	runAll(t, []string{"gate", "judge", "on"})
	wantErr(t, "unknown gate", "gate", "nope", "on", "--by", "x")
	wantErr(t, "on, off or auto", "gate", "judge", "maybe", "--by", "x")
	wantErr(t, "needs -c", "edit", "TSK-001")
}

func TestAddDoneRefusedWhileGateOn(t *testing.T) {
	gateEnvTrust(t)
	runAll(t, []string{"add", "task", "fine", "-S", "done"})
	gateSet(t, "evidence", "on")
	wantErr(t, "refused", "add", "task", "x", "-S", "done")
	gateSet(t, "evidence", "off")
	gateSet(t, "judge", "on")
	wantErr(t, "refused", "add", "task", "x", "-S", "done")
	gateSet(t, "judge", "off")
	runAll(t, []string{"add", "task", "ok", "-S", "done"})
}

func TestGateViewAndProductRule(t *testing.T) {
	path, _ := gateEnv(t)
	out := captureOutput(t, func() { runAll(t, []string{"gate"}) })
	if !strings.HasPrefix(out, "level proof (evidence on, acceptance off, judge off)\n") || !strings.Contains(out, "default proof") {
		t.Fatalf("%s", out)
	}
	w, _ := load(path)
	dir := w.Tenants[0].Path
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out = captureOutput(t, func() { runAll(t, []string{"gate", "-j"}) })
	var doc struct {
		Level string     `json:"level"`
		Gates []gateInfo `json:"gates"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || len(doc.Gates) != 3 || doc.Level != "proof" {
		t.Fatalf("%v %s", err, out)
	}
	gs := doc.Gates
	if !gs[0].Effective || gs[2].Effective || gs[1].Effective || !strings.Contains(gs[0].Reason, "go.mod") {
		t.Fatalf("%+v", gs)
	}
	if b := captureOutput(t, func() { runAll(t, []string{"board"}) }); !strings.Contains(b, "gates: proof") {
		t.Fatalf("board lacks gates line:\n%s", b)
	}
	// judge never relaxes by itself: latched by the next saving command
	runAll(t, []string{"add", "task", "x"})
	if err := os.Remove(filepath.Join(dir, "go.mod")); err != nil {
		t.Fatal(err)
	}
	w, _ = load(path)
	tn := &w.Tenants[0]
	manifestCache = map[string]manifestResult{}
	if g := byName(gateStates(w, tn), "evidence"); !g.Effective || g.Reason != "default proof" {
		t.Fatalf("evidence stays on by default: %+v", g)
	}
	// a missing dir is on too (default proof)
	tn.Path = filepath.Join(dir, "gone")
	manifestCache = map[string]manifestResult{}
	g := gateStates(w, &Tenant{Slug: "z", Path: tn.Path})
	if !g[0].Effective || g[0].Reason != "default proof" {
		t.Fatalf("%+v", g[0])
	}
}

func TestFlipLineAndGatesSeenWrittenBySavesOnly(t *testing.T) {
	path, _ := gateEnv(t)
	w, _ := load(path)
	dir := w.Tenants[0].Path
	before, _ := os.ReadFile(tenantPath(path, w.Tenants[0].Slug))
	if !strings.Contains(string(before), `"gates_seen"`) {
		t.Fatalf("init did not record gates_seen:\n%s", before)
	}
	_ = dir
	// another process turns evidence off behind this one's back
	w.Tenants[0].Gates = map[string]string{"evidence": "off"}
	resetRunState()
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	before, _ = os.ReadFile(tenantPath(path, w.Tenants[0].Slug))
	errOut := captureStderr(t, func() { captureOutput(t, func() { runAll(t, []string{"board"}) }) })
	if !strings.Contains(errOut, "gate evidence is now off") {
		t.Fatalf("no flip line: %q", errOut)
	}
	after, _ := os.ReadFile(tenantPath(path, w.Tenants[0].Slug))
	if string(after) != string(before) {
		t.Fatal("a read-only command rewrote the board")
	}
	// repeats until a saving command runs
	errOut = captureStderr(t, func() { captureOutput(t, func() { runAll(t, []string{"list"}) }) })
	if !strings.Contains(errOut, "gate evidence is now off") {
		t.Fatalf("flip line did not repeat: %q", errOut)
	}
	runAll(t, []string{"add", "task", "saves"})
	w, _ = load(path)
	if w.Tenants[0].GatesSeen["evidence"] {
		t.Fatalf("gates_seen not recorded: %+v", w.Tenants[0].GatesSeen)
	}
	errOut = captureStderr(t, func() { captureOutput(t, func() { runAll(t, []string{"board"}) }) })
	if errOut != "" {
		t.Fatalf("flip line after save: %q", errOut)
	}
}

func TestVersionAndEditAndBoardJSON(t *testing.T) {
	gateEnv(t)
	out := captureOutput(t, func() { runAll(t, []string{"version", "-j"}) })
	var v versionInfo
	if err := json.Unmarshal([]byte(out), &v); err != nil || v.Version == "" || v.Schema != currentSchema {
		t.Fatalf("%v %s", err, out)
	}
	runAll(t, []string{"add", "task", "A", "-b", "old"}, []string{"edit", "TSK-001", "-b", "new", "-c", "fresh"})
	out = captureOutput(t, func() { runAll(t, []string{"show", "TSK-001", "-v", "-j"}) })
	if !strings.Contains(out, `"body": "new"`) || !strings.Contains(out, `"acceptance": "fresh"`) {
		t.Fatalf("%s", out)
	}
	for _, args := range [][]string{{"board", "-j"}, {"context", "-j"}, {"board", "-A", "-j"}} {
		out = captureOutput(t, func() { runAll(t, args) })
		if !json.Valid([]byte(out)) || !strings.Contains(out, `"gates"`) {
			t.Fatalf("%v: %s", args, out)
		}
	}
}

func TestPromptsRenderOnlyEffectiveGates(t *testing.T) {
	path, _ := gateEnvTrust(t)
	render := func() (string, string, string) {
		w, _ := load(path)
		tn := &w.Tenants[0]
		return buildSessionPrompt(path, w, tn, "bot", "builder", false), buildDispatcherPrompt(path, w, "d"), buildCoachPrompt(path, w, "c")
	}
	s, d, c := render()
	for _, p := range []string{s, d, c} {
		for _, bad := range []string{"Evidence gate", "Acceptance gate", "Judge gate", "Gates ", "Gates on", "evidence =", "judge ="} {
			if strings.Contains(p, bad) {
				t.Fatalf("gate text %q with all gates off:\n%s", bad, p)
			}
		}
	}
	gateSet(t, "evidence", "on")
	s, d, c = render()
	if !strings.Contains(s, "Before `mw done`") || strings.Contains(s, "Flow: claim") || strings.Contains(s, "Acceptance for your item") {
		t.Fatalf("%s", s)
	}
	if !strings.Contains(d, "evidence =") || strings.Contains(d, "judge =") || !strings.Contains(c, "evidence =") {
		t.Fatalf("%s\n%s", d, c)
	}
	gateSet(t, "judge", "on")
	gateSet(t, "acceptance", "on")
	s, _, _ = render()
	for _, want := range []string{"Before `mw done`", "Acceptance for your item", "Flow: claim"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %s:\n%s", want, s)
		}
	}
}

func TestWorkspaceGatesRuleSkipsTenantsWithoutGateState(t *testing.T) {
	gateEnv(t)
	w := &Workspace{Tenants: []Tenant{
		{Slug: "bare", Path: filepath.Join(t.TempDir(), "missing")},
		{Slug: "set", Path: filepath.Join(t.TempDir(), "missing2"), Gates: map[string]string{"judge": "on"}},
	}}
	resetRunState()
	rule := workspaceGatesRule(w, "x: ")
	if !strings.Contains(rule, "judge") {
		t.Fatalf("%q", rule)
	}
	if _, ok := manifestCache[w.Tenants[0].Path]; ok {
		t.Fatal("tenant without gate state was scanned")
	}
	if _, ok := manifestCache[w.Tenants[1].Path]; !ok {
		t.Fatal("tenant with gate settings was not examined")
	}
}

func TestGateWorkspaceDefault(t *testing.T) {
	path, _ := gateEnv(t)
	wantErr(t, "--by", "gate", "evidence", "on", "--workspace")
	runAll(t, []string{"gate", "evidence", "on", "--workspace", "--by", "me"})
	w, _ := load(path)
	if w.Gates["evidence"] != "on" || !gateOn(w, &w.Tenants[0], "evidence") {
		t.Fatalf("%+v", w.Gates)
	}
	runAll(t, []string{"gate", "evidence", "off", "--by", "me"}) // tenant setting wins
	if w, _ = load(path); gateOn(w, &w.Tenants[0], "evidence") {
		t.Fatal("tenant setting did not win")
	}
	runAll(t, []string{"gate", "evidence", "auto", "--workspace", "--by", "me"})
	if w, _ = load(path); w.Gates["evidence"] != "" {
		t.Fatalf("%+v", w.Gates)
	}
	wantErr(t, "unknown gate", "gate", "nope", "on", "--workspace", "--by", "me")
}

func TestRealStdinTerminalCheck(t *testing.T) {
	orig := os.Stdin
	t.Cleanup(func() { os.Stdin = orig })
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	os.Stdin = null
	if realStdinIsTerminal() {
		t.Fatal("/dev/null counted as a terminal")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	os.Stdin = r
	if realStdinIsTerminal() {
		t.Fatal("a pipe counted as a terminal")
	}
	// end to end: with the real check and no terminal, mw gate needs --by
	path := inTempDir(t)
	_ = path
	t.Setenv("MW_SESSION", "")
	runAll(t, []string{"init", "Real"})
	stdinIsTerminal = realStdinIsTerminal
	t.Cleanup(func() { stdinIsTerminal = func() bool { return false } })
	wantErr(t, "--by", "gate", "judge", "on")
}

func TestEvidenceFollowsSymlinks(t *testing.T) {
	path, home := gateEnv(t)
	w, _ := load(path)
	tn := &w.Tenants[0]
	real := t.TempDir()
	if err := os.WriteFile(filepath.Join(real, "proof.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	slugDir := filepath.Join(home, "evidence", tn.Slug)
	if err := os.MkdirAll(slugDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(slugDir, "TSK-001")); err != nil {
		t.Skip("no symlinks")
	}
	files, err := checkEvidence(w, tn, "TSK-001")
	if err != nil || len(files) != 1 || files[0] != "proof.txt 1" {
		t.Fatalf("%v %v", files, err)
	}
	// a symlinked file inside a real folder
	dir := filepath.Join(slugDir, "TSK-002")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(real, "proof.txt"), filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if files, err = checkEvidence(w, tn, "TSK-002"); err != nil || len(files) != 1 {
		t.Fatalf("%v %v", files, err)
	}
}

func TestEvidenceFlagParsing(t *testing.T) { // F3
	rest, paths, err := splitEvidenceArgs([]string{"T-1", "--evidence", "a.png", "my note", "--evidence=b.png", "--evidence", "c"})
	if err != nil || strings.Join(rest, "|") != "T-1|my note" || strings.Join(paths, "|") != "a.png|b.png|c" {
		t.Fatalf("%v %v %v", rest, paths, err)
	}
	if _, _, err := splitEvidenceArgs([]string{"T-1", "ok", "--evidence"}); err == nil {
		t.Fatal("dangling --evidence accepted")
	}
	if _, _, err := splitEvidenceArgs([]string{"--evidence", "--no-evidence"}); err == nil {
		t.Fatal("flag taken as a path")
	}
}

func TestCopyEvidencePreflight(t *testing.T) { // F7
	src, dest := t.TempDir(), filepath.Join(t.TempDir(), "ev")
	good := filepath.Join(src, "a.txt")
	os.WriteFile(good, []byte("a"), 0o600)
	os.MkdirAll(filepath.Join(src, "d"), 0o755)
	os.WriteFile(filepath.Join(src, "d", "a.txt"), []byte("other"), 0o600)
	// a missing later path copies nothing
	if _, err := copyEvidence([]string{good, filepath.Join(src, "missing")}, dest); err == nil {
		t.Fatal("missing path accepted")
	}
	if _, err := os.Stat(filepath.Join(dest, "a.txt")); err == nil {
		t.Fatal("copied before the preflight finished")
	}
	// same base name from two places is refused, nothing copied
	if _, err := copyEvidence([]string{good, filepath.Join(src, "d", "a.txt")}, dest); err == nil || !strings.Contains(err.Error(), "both become") {
		t.Fatalf("%v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "a.txt")); err == nil {
		t.Fatal("copied despite collision")
	}
	// symlinks inside a folder are skipped
	os.Symlink(good, filepath.Join(src, "d", "link.txt"))
	out, err := copyEvidence([]string{filepath.Join(src, "d")}, dest)
	if err != nil || len(out) != 1 || out[0] != "a.txt 5" {
		t.Fatalf("%v %v", out, err)
	}
	if _, err := os.Lstat(filepath.Join(dest, "link.txt")); err == nil {
		t.Fatal("symlink copied")
	}
}

func TestEvidenceRecordedWithGateOff(t *testing.T) { // F13
	path, _ := gateEnv(t)
	runAll(t, []string{"add", "task", "A", "-S", "ready"})
	f := filepath.Join(t.TempDir(), "p.png")
	os.WriteFile(f, []byte("png"), 0o600)
	runAll(t, []string{"done", "TSK-001", "ok", "--evidence", f})
	if it := itemOf(t, path, "TSK-001"); strings.Join(it.Evidence, "|") != "p.png 3" {
		t.Fatalf("%v", it.Evidence)
	}
}

func TestDoneEvidenceCopiesFilesAndFolders(t *testing.T) {
	path, home := gateEnv(t)
	runAll(t, []string{"add", "task", "A"}, []string{"add", "task", "B"}, []string{"gate", "evidence", "on", "--by", "t"})
	wantErr(t, "evidence required", "done", "TSK-001", "x")
	src := t.TempDir()
	f := filepath.Join(src, "check.txt")
	os.WriteFile(f, []byte("proof"), 0o600)
	runAll(t, []string{"done", "TSK-001", "ok", "--evidence", f})
	if b, err := os.ReadFile(filepath.Join(home, "evidence", "gate-app", "TSK-001", "check.txt")); err != nil || string(b) != "proof" {
		t.Fatalf("%v %s", err, b)
	}
	if it := itemOf(t, path, "TSK-001"); it.Status != "done" || len(it.Evidence) != 1 {
		t.Fatalf("%+v", it)
	}
	dir := filepath.Join(src, "shots")
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "a.png"), []byte("a"), 0o600)
	os.WriteFile(filepath.Join(dir, "sub", "b.png"), []byte("b"), 0o600)
	os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("x"), 0o600)
	runAll(t, []string{"done", "TSK-002", "ok", "--evidence", dir, "--evidence", f})
	it := itemOf(t, path, "TSK-002")
	if strings.Join(it.Evidence, "|") != "a.png 1|check.txt 5|sub/b.png 1" {
		t.Fatalf("%v", it.Evidence)
	}
	// a missing path aborts before any state change
	runAll(t, []string{"reopen", "TSK-002"})
	wantErr(t, "evidence path", "done", "TSK-002", "ok", "--evidence", filepath.Join(src, "missing"))
	if itemOf(t, path, "TSK-002").Status == "done" {
		t.Fatal("done despite missing evidence path")
	}
}

func TestGateLevelsRoundTripAndCustom(t *testing.T) {
	path, _ := gateEnv(t)
	gateLine := func() string {
		out := captureOutput(t, func() { runAll(t, []string{"gate"}) })
		return strings.SplitN(out, "\n", 2)[0]
	}
	if got := gateLine(); got != "level proof (evidence on, acceptance off, judge off)" {
		t.Fatalf("default: %q", got)
	}
	for _, c := range []struct{ level, want string }{
		{"trust", "level trust (evidence off, acceptance off, judge off)"},
		{"spec", "level spec (evidence on, acceptance on, judge off)"},
		{"judged", "level judged (evidence on, acceptance on, judge on)"},
		{"proof", "level proof (evidence on, acceptance off, judge off)"},
	} {
		runAll(t, []string{"gate", "level", c.level, "--by", "test"})
		if got := gateLine(); got != c.want {
			t.Fatalf("%s: %q", c.level, got)
		}
		w, _ := load(path)
		for _, g := range gateStates(w, &w.Tenants[0]) {
			if g.Setting != "on" && g.Setting != "off" {
				t.Fatalf("%s: %+v", c.level, g)
			}
		}
	}
	// the -j form carries the level
	out := captureOutput(t, func() { runAll(t, []string{"gate", "-j"}) })
	var doc struct {
		Level string
		Gates []gateInfo
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || doc.Level != "proof" || len(doc.Gates) != 3 {
		t.Fatalf("%v %s", err, out)
	}
	// a combination that is no level
	gateSet(t, "judge", "on")
	if got := gateLine(); got != "level custom" {
		t.Fatalf("custom: %q", got)
	}
	out = captureOutput(t, func() { runAll(t, []string{"gate", "-j"}) })
	if !strings.Contains(out, `"level": "custom"`) {
		t.Fatalf("%s", out)
	}
	// level is human-only and validated
	wantErr(t, "must be trust", "gate", "level", "strict", "--by", "x")
	wantErr(t, "--by", "gate", "level", "trust")
	wantErr(t, "for humans", "gate", "level", "trust", "--by", "x", "-s", "bot")
	// workspace level
	runAll(t, []string{"gate", "level", "spec", "--workspace", "--by", "me"})
	runAll(t, []string{"tenant", "add", "other", "Other"})
	out = captureOutput(t, func() { runAll(t, []string{"gate", "-t", "other"}) })
	if !strings.HasPrefix(out, "level spec") {
		t.Fatalf("%s", out)
	}
}

func TestGatesLineShowsLevelName(t *testing.T) {
	gateEnv(t)
	if b := captureOutput(t, func() { runAll(t, []string{"board"}) }); !strings.Contains(b, "gates: proof") {
		t.Fatalf("board:\n%s", b)
	}
	gateSet(t, "judge", "on")
	if b := captureOutput(t, func() { runAll(t, []string{"board"}) }); !strings.Contains(b, "gates: evidence on, acceptance off, judge on") {
		t.Fatalf("board:\n%s", b)
	}
	runAll(t, []string{"gate", "level", "trust", "--by", "t"})
	if b := captureOutput(t, func() { runAll(t, []string{"board"}) }); strings.Contains(b, "gates:") {
		t.Fatalf("board:\n%s", b)
	}
}

// A board written before levels existed (gates untouched = auto, evidence last
// seen off) now needs proof; the error says why and how a human lifts it.
func TestUpgradeAutoBoardEvidenceHint(t *testing.T) {
	path, _ := gateEnv(t)
	runAll(t, []string{"add", "task", "A"}, []string{"add", "task", "B"})
	w, _ := load(path)
	w.Gates = nil
	w.Tenants[0].Gates, w.Tenants[0].GateLatch = nil, nil
	w.Tenants[0].GatesSeen = map[string]bool{"evidence": false, "acceptance": false, "judge": false}
	resetRunState()
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	wantErr(t, "a human can run `mw gate level trust -t SLUG`", "done", "TSK-001", "ok")
	runAll(t, []string{"done", "TSK-002", "ok", "--no-evidence", "legacy"})
	// an explicit evidence setting gets no hint
	gateSet(t, "evidence", "on")
	if err := run([]string{"done", "TSK-001", "ok"}); err == nil || strings.Contains(err.Error(), "0.2.5 default") {
		t.Fatalf("hint on explicit setting: %v", err)
	}
}

func TestWorkspaceLevelListsOverridesAndRecordsBy(t *testing.T) {
	gateEnv(t)
	runAll(t, []string{"tenant", "add", "other", "Other"})
	runAll(t, []string{"gate", "level", "judged", "-t", "other", "--by", "alice"})
	out := captureOutput(t, func() { runAll(t, []string{"gate", "level", "trust", "--workspace", "--by", "bob"}) })
	if !strings.Contains(out, "overridden by: other (set their level with -t)") {
		t.Fatalf("%s", out)
	}
	out = captureOutput(t, func() { runAll(t, []string{"gate", "-t", "other"}) })
	if !strings.Contains(out, "set by alice "+time.Now().UTC().Format("2006-01-02")) {
		t.Fatalf("%s", out)
	}
	out = captureOutput(t, func() { runAll(t, []string{"gate"}) })
	if !strings.Contains(out, "workspace default off, set by bob ") {
		t.Fatalf("%s", out)
	}
}

func TestJSONCarriesLevel(t *testing.T) {
	gateEnv(t)
	runAll(t, []string{"add", "task", "A", "-S", "ready"})
	for _, args := range [][]string{{"board", "-j"}, {"context", "-j"}, {"next", "-j"}} {
		out := captureOutput(t, func() { runAll(t, args) })
		if !strings.Contains(out, `"level": "proof"`) {
			t.Fatalf("%v: %s", args, out)
		}
	}
}
