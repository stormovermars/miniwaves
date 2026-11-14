package mw

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func readBeat(t *testing.T, path, name string) nodeBeat {
	t.Helper()
	b, err := os.ReadFile(nodePath(path, name))
	if err != nil {
		t.Fatal(err)
	}
	var nb nodeBeat
	if err := json.Unmarshal(b, &nb); err != nil {
		t.Fatal(err)
	}
	return nb
}

func TestNodeBeatAndList(t *testing.T) {
	path := inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t, []string{"init", "Fleet"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"})
	if err := run([]string{"node", "beat"}); err == nil || !strings.Contains(err.Error(), "mw node set") {
		t.Fatalf("beat without a name: %v", err)
	}
	if err := run([]string{"node", "set", "Bad Name"}); err == nil {
		t.Fatal("bad name accepted")
	}
	runAll(t, []string{"node", "set", "alpha"},
		[]string{"add", "task", "A", "-S", "ready"}, []string{"session", "start", "s1"},
		[]string{"start", "TSK-001", "-s", "s1"})
	runAll(t, []string{"node", "beat", "-m", "hello"})
	nb := readBeat(t, path, "alpha")
	if nb.State != "working" || nb.Item != "TSK-001" || nb.Sessions != 1 || nb.Doing != 1 || nb.Msg != "hello" || nb.Version != version {
		t.Fatalf("beat: %+v", nb)
	}
	runAll(t, []string{"node", "beat", "limit", "--item", "tsk-009"})
	if nb = readBeat(t, path, "alpha"); nb.State != "limit" || nb.Item != "TSK-009" {
		t.Fatalf("beat: %+v", nb)
	}
	if err := run([]string{"node", "beat", "sleepy"}); err == nil {
		t.Fatal("unknown state accepted")
	}
	out := captureOutput(t, func() { runAll(t, []string{"node", "list", "-j"}) })
	var rows []nodeRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 1 || !rows[0].Live || rows[0].Node != "alpha" {
		t.Fatalf("list: %s %v", out, err)
	}
	// an old beat is silent
	old := nb
	old.At = time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339)
	old.Node = "beta"
	b, _ := json.Marshal(old)
	if err := os.WriteFile(nodePath(path, "beta"), append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	out = captureOutput(t, func() { runAll(t, []string{"node", "list"}) })
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "silent") {
		t.Fatalf("list text: %s", out)
	}
	runAll(t, []string{"user", "set", "node.silent=300"})
	rows = nil
	out = captureOutput(t, func() { runAll(t, []string{"node", "list", "-j"}) })
	_ = json.Unmarshal([]byte(out), &rows)
	if len(rows) != 2 || !rows[1].Live {
		t.Fatalf("node.silent ignored: %s", out)
	}
}
