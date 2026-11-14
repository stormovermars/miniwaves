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

func TestNodeBeat(t *testing.T) {
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
	runAll(t, []string{"node", "beat", "blocked", "--item", "tsk-009"})
	if nb = readBeat(t, path, "alpha"); nb.State != "blocked" || nb.Item != "TSK-009" {
		t.Fatalf("beat: %+v", nb)
	}
	for _, bad := range []string{"sleepy", "limit"} {
		if err := run([]string{"node", "beat", bad}); err == nil {
			t.Fatalf("state %q accepted", bad)
		}
	}
	if err := run([]string{"node", "list"}); err == nil {
		t.Fatal("node list still exists: the fleet view shows nodes")
	}
	if err := run([]string{"node", "set", "any"}); err == nil {
		t.Fatal("node name any accepted")
	}
	v := fleetJSON(t)
	if a := fleetNodeOf(v, "alpha"); !a.Live || a.State != "blocked" || a.Sessions != 1 {
		t.Fatalf("fleet: %+v", a)
	}
	// an old beat is silent; node.silent moves the line
	old := nb
	old.At = time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339)
	old.Node = "beta"
	b, _ := json.Marshal(old)
	if err := os.WriteFile(nodePath(path, "beta"), append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if fleetNodeOf(fleetJSON(t), "beta").Live {
		t.Fatal("a 3 h old beat is live")
	}
	runAll(t, []string{"user", "set", "node.silent=300"})
	if !fleetNodeOf(fleetJSON(t), "beta").Live {
		t.Fatal("node.silent ignored")
	}
}
