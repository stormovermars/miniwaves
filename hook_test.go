package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

// hookRun runs the SessionStart hook command with the given stdin and returns what it printed.
func hookRun(t *testing.T, stdin string) string {
	t.Helper()
	var out bytes.Buffer
	if err := hookContextCommand(strings.NewReader(stdin), &out); err != nil {
		t.Fatalf("hook must never fail: %v", err)
	}
	return out.String()
}

func hookCtx(t *testing.T, out string) string {
	t.Helper()
	var m struct {
		H struct {
			Name string `json:"hookEventName"`
			Ctx  string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &m); err != nil || m.H.Name != "SessionStart" {
		t.Fatalf("not hook JSON: %v %q", err, out)
	}
	return m.H.Ctx
}

func hookBoard(t *testing.T) {
	t.Helper()
	gateEnv(t)
	runAll(t,
		[]string{"add", "task", "Wire the hook", "-c", "Prints JSON when claimed.\nSilent otherwise.", "-S", "ready"},
		[]string{"add", "task", "Other", "-S", "ready"},
		[]string{"session", "start", "w1", "--claude-session", "cc-123"},
		[]string{"start", "TSK-001", "-s", "w1"},
		[]string{"note", "TSK-001", "first note", "-s", "w1"},
		[]string{"note", "TSK-001", "second note", "-s", "w1"},
		[]string{"ask", "TSK-001", "Which shell?", "-s", "w1"},
		[]string{"answer", "TSK-001", "Bash only", "--by", "sir"},
		[]string{"decide", "Use stdin JSON", "--by", "sir", "-b", "hook gives it", "-p", "TSK-001"},
	)
}

func TestHookContextGolden(t *testing.T) { // TSK-079
	hookBoard(t)
	dir := mustGetwd()
	in := `{"session_id":"cc-123","transcript_path":"/x/t.jsonl","cwd":` + jsonString(dir) + `,"source":"compact"}`
	t.Setenv("MW_SESSION", "")
	byClaude := hookRun(t, in)
	t.Setenv("MW_SESSION", "w1")
	byEnv := hookRun(t, `{"cwd":`+jsonString(dir)+`,"source":"compact"}`) // no session_id: MW_SESSION alone is enough
	if byEnv == "" || !strings.Contains(hookCtx(t, byEnv), "TSK-001") {
		t.Fatalf("MW_SESSION lookup: %q", byEnv)
	}
	ctx := hookCtx(t, byClaude)
	for _, want := range []string{"TSK-001", "Wire the hook", "Prints JSON when claimed.", "second note", "answer", "Bash only", "decide", "Use stdin JSON", "hook gives it", "compact"} {
		if !strings.Contains(ctx, want) {
			t.Errorf("missing %q in:\n%s", want, ctx)
		}
	}
	if strings.Contains(ctx, "Other") {
		t.Errorf("unclaimed item leaked:\n%s", ctx)
	}
	checkGolden(t, "hook-context-json", byClaude)
	checkGolden(t, "hook-context-text", ctx+"\n")
}

func jsonString(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestHookContextSilent(t *testing.T) { // TSK-079
	hookBoard(t)
	dir := mustGetwd()
	cwd := `"cwd":` + jsonString(dir)
	t.Setenv("MW_SESSION", "")
	for name, in := range map[string]string{
		"empty stdin":       "",
		"garbage stdin":     "not json {",
		"unknown claude id": `{"session_id":"nope",` + cwd + `}`,
		"no session id":     `{` + cwd + `}`,
	} {
		if out := hookRun(t, in); out != "" {
			t.Errorf("%s: printed %q", name, out)
		}
	}
	// a session that has nothing claimed
	runAll(t, []string{"session", "start", "idle1", "--claude-session", "cc-idle"})
	if out := hookRun(t, `{"session_id":"cc-idle",`+cwd+`}`); out != "" {
		t.Errorf("no claimed item: printed %q", out)
	}
	t.Setenv("MW_SESSION", "ghost")
	if out := hookRun(t, `{`+cwd+`}`); out != "" {
		t.Errorf("unknown MW_SESSION: printed %q", out)
	}
	// done items are not claimed any more
	t.Setenv("MW_SESSION", "w1")
	runAll(t, []string{"done", "TSK-001", "ok", "--no-evidence", "x", "-s", "w1"})
	if out := hookRun(t, `{`+cwd+`}`); out != "" {
		t.Errorf("done item: printed %q", out)
	}
	// no board from cwd
	t.Setenv("MW_SESSION", "w1")
	if out := hookRun(t, `{"session_id":"cc-123","cwd":`+jsonString(t.TempDir())+`}`); out != "" {
		t.Errorf("no board: printed %q", out)
	}
	// through the CLI: exit 0, nothing printed, outside any board and with a broken stdin
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
	if code, so, se := mwRun(t, "context", "--hook-json"); code != 0 || so != "" || se != "" {
		t.Fatalf("cli: %d %q %q", code, so, se)
	}
}

func TestHookContextCap(t *testing.T) { // TSK-079
	hookBoard(t)
	big := strings.Repeat("x", 600)
	for i := 0; i < 12; i++ {
		runAll(t, []string{"note", "TSK-001", big, "-s", "w1"}, []string{"decide", big, "--by", "sir", "-p", "TSK-001"})
	}
	t.Setenv("MW_SESSION", "w1")
	out := hookRun(t, `{"cwd":`+jsonString(mustGetwd())+`}`)
	ctx := hookCtx(t, out)
	if n := utf8.RuneCountInString(ctx); n > 2000 || n < 1500 {
		t.Fatalf("additionalContext is %d chars, want <= 2000 (and cut, not dropped)", n)
	}
	if !strings.HasSuffix(ctx, "… (mw show TSK-001 --full)") {
		t.Fatalf("tail: %q", ctx[len(ctx)-60:])
	}
}

func TestSessionStartClaudeSession(t *testing.T) { // TSK-079
	path, _ := gateEnv(t)
	runAll(t, []string{"session", "start", "a", "--claude-session", "cc-1"})
	if s, _ := sessionByID(loadT(t, path), "a"); s.ClaudeSession != "cc-1" {
		t.Fatalf("%+v", s)
	}
	b, _ := os.ReadFile(filepath.Join(filepath.Dir(path), ".miniwaves", "gate-app.json"))
	if !strings.Contains(string(b), `"claude_session":"cc-1"`) {
		t.Fatalf("stored on the session record: %s", b)
	}
	runAll(t, []string{"session", "start", "plain"})
	if b, _ := os.ReadFile(filepath.Join(filepath.Dir(path), ".miniwaves", "gate-app.json")); strings.Count(string(b), "claude_session") != 1 {
		t.Fatalf("omitempty: %s", b)
	}
	wantErr(t, "--claude-session must be", "session", "start", "b", "--claude-session", "bad id;")
	// stop + restart takes a new id
	runAll(t, []string{"session", "stop", "a"}, []string{"session", "start", "a", "--claude-session", "cc-2"})
	if s, _ := sessionByID(loadT(t, path), "a"); s.ClaudeSession != "cc-2" || s.State != "idle" {
		t.Fatalf("%+v", s)
	}
}

// The hook script keeps its install behaviour and chains the context command, passing the hook stdin on.
func TestEnsureMwChainsContext(t *testing.T) { // TSK-079
	if runtime.GOOS == "windows" {
		t.Skip("bash script; Windows runs it in Git Bash")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	script, _ := filepath.Abs(filepath.Join("plugins", "mw", "scripts", "ensure-mw.sh"))
	run := func(version string) string {
		home := t.TempDir()
		bin := filepath.Join(home, ".local", "bin")
		os.MkdirAll(bin, 0o755)
		stub := "#!/bin/sh\ncase \"$1\" in version) echo 'mw " + version + " (stub)';; context) echo \"ctx:$*:$(cat)\";; esac\n"
		os.WriteFile(filepath.Join(bin, "mw"), []byte(stub), 0o755)
		cmd := exec.Command(bash, script)
		cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
		cmd.Stdin = strings.NewReader(`{"session_id":"s1"}`)
		out, _ := cmd.CombinedOutput()
		return string(out)
	}
	if got := run("0.3.14"); strings.TrimSpace(got) != "" { // an mw without --hook-json is never called
		t.Fatalf("old mw: %q", got)
	}
	if got := run("0.3.99"); strings.TrimSpace(got) != `ctx:context --hook-json:{"session_id":"s1"}` {
		t.Fatalf("current mw: %q", got)
	}
}

// Review 1: the Claude-session lookup skips stopped sessions and prefers the newest live match.
func TestHookContextSkipsStopped(t *testing.T) {
	hookBoard(t)
	path := statusFindBoard(mustGetwd())
	w := loadT(t, path)
	w1, _ := sessionByID(w, "w1")
	tenant := w1.Tenant
	old := Session{ID: "w0", Tenant: tenant, State: "stopped", ClaudeSession: "cc-123", Updated: "2099-01-01T00:00:00Z"}
	older := Session{ID: "w2", Tenant: tenant, State: "idle", ClaudeSession: "cc-123", Updated: "2000-01-01T00:00:00Z"}
	w.Sessions = append([]Session{old, older}, w.Sessions...)
	w1, _ = sessionByID(w, "w1")
	w1.Updated = "2020-01-01T00:00:00Z"
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	in := `{"session_id":"cc-123","cwd":` + jsonString(mustGetwd()) + `}`
	t.Setenv("MW_SESSION", "")
	if out := hookRun(t, in); out == "" || !strings.Contains(hookCtx(t, out), "TSK-001") {
		t.Fatalf("stopped/older match shadowed the live newest one: %q", out)
	}
	t.Setenv("MW_SESSION", "w0") // names a stopped session: fall back to the Claude-session lookup
	if out := hookRun(t, in); out == "" || !strings.Contains(hookCtx(t, out), "TSK-001") {
		t.Fatalf("stopped MW_SESSION must fall back: %q", out)
	}
}
