package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func jsonOut(t *testing.T, args ...string) map[string]any {
	t.Helper()
	out := captureOutput(t, func() { runAll(t, args) })
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("%v: not JSON: %v\n%s", args, err, out)
	}
	return m
}

func TestStartGuardAndClaim(t *testing.T) {
	contractBoard(t)
	runAll(t, []string{"start", "TSK-001"}, []string{"done", "TSK-001", "ok"})
	err := run([]string{"start", "TSK-001"})
	if err == nil || !strings.Contains(err.Error(), "start: TSK-001 is done — use mw reopen") {
		t.Fatalf("start of a done item: %v", err)
	}
	runAll(t, []string{"start", "BUG-001", "-s", "w1"})
	if err := run([]string{"start", "BUG-001", "-s", "w1"}); err != nil {
		t.Fatalf("restart of own item refused: %v", err)
	}
	if err := run([]string{"block", "BUG-001", "why", "-s", "w1"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"start", "BUG-001", "-s", "w1"}); err == nil {
		t.Fatal("start of a blocked item accepted")
	}
	// claim: atomic pick+start; null when nothing
	runAll(t, []string{"ready", "BUG-001"})
	m := jsonOut(t, "next", "--claim", "-s", "w2", "-a", "reviewer", "-j")
	if m["id"] != "BUG-001" || m["claimed"] != true || m["state"] != "doing" || m["session"] != "w2" {
		t.Fatalf("claim: %v", m)
	}
	w, _ := load(filepath.Join(mustGetwd(), dataFile))
	if s, e := sessionByID(w, "w2"); e != nil || s.Agent != "reviewer" || s.Item != "BUG-001" {
		t.Fatalf("session after claim: %+v %v", s, e)
	}
	out := captureOutput(t, func() { runAll(t, []string{"next", "--claim", "-s", "w3", "-j"}) })
	if strings.TrimSpace(out) != "null" {
		t.Fatalf("empty claim printed %q", out)
	}
	if err := run([]string{"next", "-A"}); err == nil {
		t.Fatal("next accepted -A")
	}
}

func TestVersionBoardSchema(t *testing.T) {
	path, _, _ := contractBoard(t)
	m := jsonOut(t, "version", "-j")
	if m["board_schema"] != float64(9) || !strings.HasSuffix(m["board"].(string), dataFile) || m["version"] != "0.3.3" {
		t.Fatalf("version: %v", m)
	}
	b, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(b), `"version":9`, `"version":5`, 1)), 0600)
	if m := jsonOut(t, "version", "-j"); m["board_schema"] != float64(5) {
		t.Fatalf("old schema: %v", m)
	}
	os.Remove(path)
	if m := jsonOut(t, "version", "-j"); m["board"] != nil || m["board_schema"] != nil {
		t.Fatalf("no board: %v", m)
	}
}

func TestBoardFileIsCompactAndAtomic(t *testing.T) {
	path, _, _ := contractBoard(t)
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "\n  ") || strings.Count(string(b), "\n") != 1 {
		t.Fatalf("board file is indented:\n%s", b)
	}
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Fatal("tmp file left behind")
	}
}

func TestArchive(t *testing.T) {
	path, _, _ := contractBoard(t)
	runAll(t, []string{"start", "TSK-001"}, []string{"done", "TSK-001", "ok"})
	w, _ := load(path)
	it, _ := itemByID(&w.Tenants[0], "TSK-001")
	it.Updated = time.Now().Add(-40 * 24 * time.Hour).UTC().Format(time.RFC3339)
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	dry := captureOutput(t, func() { runAll(t, []string{"archive", "--dry-run"}) })
	if !strings.Contains(dry, "would archive 1") {
		t.Fatalf("dry run: %s", dry)
	}
	if _, err := os.Stat(archivePath(path, "tiny-app")); err == nil {
		t.Fatal("dry run wrote the archive")
	}
	runAll(t, []string{"archive", "--days", "30"})
	w, _ = load(path)
	if len(w.Tenants[0].Items) != 1 || w.Tenants[0].Archived != 1 {
		t.Fatalf("board after archive: %+v", w.Tenants[0])
	}
	if m := jsonOut(t, "show", "TSK-001", "-j"); m["id"] != "TSK-001" || m["state"] != "done" {
		t.Fatalf("show fallback: %v", m)
	}
	if m := jsonOut(t, "board", "-j"); m["total"] != float64(2) || m["done"] != float64(1) || m["archived"] != float64(1) {
		t.Fatalf("board totals: %v", m)
	}
	runAll(t, []string{"archive", "--days", "30"}) // idempotent
	a, _ := loadArchive(path, w.Tenants[0].Slug)
	if len(a.Items) != 1 || len(a.Items[0].Events) == 0 {
		t.Fatalf("archive content: %+v", a)
	}
	if err := run([]string{"show", "TSK-009"}); err == nil {
		t.Fatal("show invented an item")
	}
	runAll(t, []string{"add", "task", "Third"})
	m := jsonOut(t, "show", "TSK-002", "-j") // ID counter not reused
	if m["id"] != "TSK-002" {
		t.Fatalf("id reuse: %v", m)
	}
}

func TestHeadlessPromptAndExistingSession(t *testing.T) {
	contractBoard(t)
	full := captureOutput(t, func() { runAll(t, []string{"get-prompt", "session", "fresh"}) })
	head := captureOutput(t, func() { runAll(t, []string{"get-prompt", "session", "fresh", "--headless"}) })
	for _, gone := range []string{"Setup:", "Loop:", "Reply only"} {
		if !strings.Contains(full, gone) || strings.Contains(head, gone) {
			t.Fatalf("%q: full=%v headless=%v", gone, strings.Contains(full, gone), strings.Contains(head, gone))
		}
	}
	if !strings.Contains(head, "Your item is already claimed; its brief follows.") || strings.Contains(head, "mw dirs") {
		t.Fatalf("headless:\n%s", head)
	}
	runAll(t, []string{"session", "start", "fresh"})
	var ee *exitError
	err := run([]string{"get-prompt", "session", "fresh"})
	if !errors.As(err, &ee) || ee.code != 3 {
		t.Fatalf("existing session: %v", err)
	}
	out := captureOutput(t, func() { err = run([]string{"get-prompt", "session", "fresh", "-j"}) })
	if !errors.As(err, &ee) || ee.code != 3 || strings.TrimSpace(out) != `{"error":"session_exists","session":"fresh"}` {
		t.Fatalf("json: %v %q", err, out)
	}
}

func TestListAllOwnerAndLimit(t *testing.T) {
	contractBoard(t)
	runAll(t, []string{"tenant", "add", "other", "Other"}, []string{"add", "task", "Theirs", "-t", "other", "-S", "ready"},
		[]string{"start", "TSK-001", "-s", "bot", "-a", "builder"}, []string{"start", "TSK-001", "-t", "other", "-s", "bot2"})
	out := captureOutput(t, func() { runAll(t, []string{"list", "-s", "bot", "-A", "-j"}) })
	var doc struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || len(doc.Items) != 1 || doc.Items[0]["id"] != "TSK-001" || doc.Items[0]["tenant"] == "other" {
		t.Fatalf("owner filter: %v %s", err, out)
	}
	all := captureOutput(t, func() { runAll(t, []string{"list", "-A", "-j"}) })
	if !strings.Contains(all, `"other"`) {
		t.Fatalf("-A misses tenants:\n%s", all)
	}
	one := captureOutput(t, func() { runAll(t, []string{"list", "-n", "1", "-j"}) })
	if strings.Count(one, `"id"`) != 1 {
		t.Fatalf("-n 1:\n%s", one)
	}
	if err := run([]string{"list", "-n", "0"}); err == nil {
		t.Fatal("-n 0 accepted")
	}
	w, _ := load(filepath.Join(mustGetwd(), dataFile))
	if s, _ := sessionByID(w, "bot"); s.Agent != "builder" {
		t.Fatalf("start -a role: %+v", s)
	}
}

func TestLockWaitsAndNamesHolder(t *testing.T) {
	path, _, _ := contractBoard(t)
	old := lockWait
	lockWait = 150 * time.Millisecond
	defer func() { lockWait = old }()
	unlock, err := lockSet(path, false, []string{"contract-app"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err = run([]string{"add", "task", "blocked by the tenant lock"})
	if err == nil || !strings.Contains(err.Error(), "board locked (pid ") || !strings.Contains(err.Error(), "retry") {
		t.Fatalf("write under the tenant lock: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("lock wait too long")
	}
	// reads take no lock at all
	if err := run([]string{"board"}); err != nil {
		t.Fatalf("a read must not wait for a writer: %v", err)
	}
	unlock()
	runAll(t, []string{"add", "task", "now fine"})
}

func TestRefusedDoneRecordsGatesSeen(t *testing.T) {
	path, _, _ := contractBoard(t)
	runAll(t, []string{"gate", "evidence", "on", "--by", "t"}, []string{"start", "TSK-001"})
	w, _ := load(path)
	w.Tenants[0].GatesSeen = map[string]bool{} // as if never recorded
	save(path, w)
	w, _ = load(path)
	w.Tenants[0].GatesSeen = nil
	save(path, w)
	if err := run([]string{"done", "TSK-001", "ok"}); err == nil {
		t.Fatal("done without evidence accepted")
	}
	w, _ = load(path)
	if !w.Tenants[0].GatesSeen["evidence"] {
		t.Fatalf("gates_seen not recorded after refusal: %v", w.Tenants[0].GatesSeen)
	}
}

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestPluginFilesParse(t *testing.T) {
	for _, f := range []string{".claude-plugin/marketplace.json", "plugins/mw/.claude-plugin/plugin.json", "plugins/mw/hooks/hooks.json"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	b, _ := os.ReadFile("plugins/mw/.claude-plugin/plugin.json")
	if !strings.Contains(string(b), `"version": "`+version+`"`) {
		t.Fatalf("plugin version != %s", version)
	}
	if _, err := exec.LookPath("bash"); err == nil {
		if out, err := exec.Command("bash", "-n", "plugins/mw/scripts/ensure-mw.sh").CombinedOutput(); err != nil {
			t.Fatalf("bash -n: %v\n%s", err, out)
		}
	}
	for _, f := range []string{"plugins/mw/skills/mw/SKILL.md", "plugins/mw/commands/mw-next.md", "plugins/mw/commands/mw-board.md"} {
		if _, err := os.Stat(f); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat("plugins/mw/scripts/ensure-mw.sh"); err != nil {
		t.Fatal(err)
	}
}

func TestHTMLBoardAndPublish(t *testing.T) {
	path, _, _ := contractBoard(t)
	runAll(t, []string{"add", "task", "<script>x</script>", "-S", "ready"})
	out := captureOutput(t, func() { runAll(t, []string{"board", "--html"}) })
	for _, want := range []string{"<!doctype html>", "prefers-color-scheme:dark", "TSK-001", "Activity", "&lt;script&gt;"} {
		if !strings.Contains(out, want) {
			t.Fatalf("html lacks %q", want)
		}
	}
	if strings.Contains(out, "<script") || strings.Contains(out, "http") {
		t.Fatalf("html must be self-contained without scripts:\n%s", out)
	}
	target := filepath.Join(t.TempDir(), "site", "index.html")
	runAll(t, []string{"board", "--html", "-o", target})
	if b, err := os.ReadFile(target); err != nil || !strings.Contains(string(b), "Contract app") {
		t.Fatalf("-o: %v", err)
	}
	// auto publish after every write
	os.Remove(target)
	w, _ := load(path)
	w.Publish = &PublishConfig{HTML: target, Auto: true}
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	os.Remove(target)
	os.Remove(target + ".hash") // the explicit write above started the interval
	runAll(t, []string{"add", "task", "published", "-S", "ready"})
	if b, err := os.ReadFile(target); err != nil || !strings.Contains(string(b), "published") {
		t.Fatalf("auto publish: %v", err)
	}
	if err := run([]string{"board", "-o", target}); err == nil {
		t.Fatal("-o without --html accepted")
	}
}
