package mw

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func fleetJSON(t *testing.T, args ...string) fleetView {
	t.Helper()
	out := captureOutput(t, func() { runAll(t, append([]string{"fleet", "-j"}, args...)) })
	var v fleetView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return v
}

func fleetNodeOf(v fleetView, name string) fleetNode {
	for _, n := range v.Nodes {
		if n.Node == name {
			return n
		}
	}
	return fleetNode{}
}

func TestFleetView(t *testing.T) {
	path := inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t, []string{"init", "Fleet"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"},
		[]string{"add", "task", "pinned to beta", "-S", "ready", "--node", "beta", "--cost", "30"},
		[]string{"add", "task", "beta runs this", "-S", "ready"},
		[]string{"add", "task", "alpha runs this", "-S", "ready", "--node", "alpha"},
		[]string{"add", "task", "stuck blocker", "-S", "ready"},
		[]string{"add", "task", "needs an answer", "-S", "ready"},
		[]string{"add", "task", "up for grabs", "-S", "ready", "--cost", "5"})
	runAll(t, []string{"node", "set", "beta"}, []string{"start", "TSK-002", "-s", "sb"},
		[]string{"node", "set", "alpha"}, []string{"start", "TSK-003", "-s", "sa"}, []string{"start", "TSK-004", "-s", "sc"},
		[]string{"block", "TSK-004", "waits for a key"}, []string{"ask", "TSK-005", "which key?"},
		[]string{"node", "beat", "--left", "5"})
	old := nodeBeat{Node: "beta", At: time.Now().Add(-4 * time.Hour).UTC().Format(time.RFC3339), State: "working", Version: "x"}
	b, _ := json.Marshal(old)
	if err := os.WriteFile(nodePath(path, "beta"), append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	v := fleetJSON(t)
	a, bt := fleetNodeOf(v, "alpha"), fleetNodeOf(v, "beta")
	if !a.Live || bt.Live || !a.Low || a.Left == nil || *a.Left != 5 {
		t.Fatalf("liveness: %+v / %+v", a, bt)
	}
	if len(a.InFlight) != 1 || a.InFlight[0].Item != "fleet:TSK-003" || a.InFlight[0].Last == "" || a.InFlight[0].Age == "" {
		t.Fatalf("alpha in flight: %+v", a.InFlight)
	}
	if len(a.Blocked) != 1 || a.Blocked[0].Item != "fleet:TSK-004" {
		t.Fatalf("alpha blocked: %+v", a.Blocked)
	}
	if len(bt.Stalled) != 1 || bt.Stalled[0].Item != "fleet:TSK-002" || bt.Stalled[0].Why != "node silent" {
		t.Fatalf("beta stalled: %+v", bt)
	}
	if len(v.Movable) != 1 || v.Movable[0].Item != "fleet:TSK-002" {
		t.Fatalf("movable: %+v", v.Movable)
	}
	if len(v.Held) != 1 || v.Held[0].Item != "fleet:TSK-001" || bt.Ready != 1 || bt.ReadyCost != 30 {
		t.Fatalf("held: %+v beta %+v", v.Held, bt)
	}
	un := fleetNodeOf(v, unknownNode)
	if len(un.Waiting) != 1 || un.Waiting[0].Item != "fleet:TSK-005" {
		t.Fatalf("waiting: %+v", v)
	}
	if v.Any.Ready != 1 || v.Any.Cost != 5 || v.Any.Next == nil || v.Any.Next.Item != "fleet:TSK-006" {
		t.Fatalf("any: %+v", v.Any)
	}
	if a.Next == nil || a.Next.Item != "fleet:TSK-006" || bt.Next == nil || bt.Next.Item != "fleet:TSK-001" {
		t.Fatalf("next: %+v / %+v", a.Next, bt.Next)
	}
	text := captureOutput(t, func() { runAll(t, []string{"fleet"}) })
	for _, want := range []string{"alpha live", "beta SILENT", "LOW", "STALLED", "movable", "held", "waiting"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text view lacks %q:\n%s", want, text)
		}
	}
	// the stall threshold: a live node's row with no log entry for long is stalled, a silent node's is not needed for it
	real := nowFn
	defer func() { nowFn = real }()
	nowFn = func() time.Time { return time.Now().Add(5 * time.Hour).UTC() }
	runAll(t, []string{"node", "beat"}) // alpha beats "now" in real time: 5 h behind the fake clock, so silent
	v = fleetJSON(t)
	if a = fleetNodeOf(v, "alpha"); a.Live || len(a.Stalled) != 1 {
		t.Fatalf("after 5h: %+v", a)
	}
	runAll(t, []string{"user", "set", "node.silent=1440", "node.stall=1440"})
	if v = fleetJSON(t); len(fleetNodeOf(v, "alpha").Stalled) != 0 || !fleetNodeOf(v, "alpha").Live {
		t.Fatalf("thresholds ignored: %+v", v)
	}
}

func TestFleetChanged(t *testing.T) {
	inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t, []string{"init", "Fleet"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"},
		[]string{"node", "set", "alpha"}, []string{"add", "task", "A", "-S", "ready"}, []string{"node", "beat"})
	run1 := captureOutput(t, func() { runAll(t, []string{"fleet", "--changed"}) })
	if strings.TrimSpace(run1) == "no change" || !strings.Contains(run1, "alpha") {
		t.Fatalf("first run: %q", run1)
	}
	if out := strings.TrimSpace(captureOutput(t, func() { runAll(t, []string{"fleet", "--changed"}) })); out != "no change" {
		t.Fatalf("second run: %q", out)
	}
	runAll(t, []string{"node", "beat"}) // only the beat time moved: still no news
	if out := strings.TrimSpace(captureOutput(t, func() { runAll(t, []string{"fleet", "--changed", "-j"}) })); out != "no change" {
		t.Fatalf("beat time counted as a change: %q", out)
	}
	// another coordinator session keeps its own state: it sees the view once
	t.Setenv("MW_SESSION", "coord-two")
	if out := strings.TrimSpace(captureOutput(t, func() { runAll(t, []string{"fleet", "--changed"}) })); out == "no change" {
		t.Fatal("a second coordinator was told no change on its first look")
	}
	t.Setenv("MW_SESSION", "")
	runAll(t, []string{"add", "task", "B", "-S", "ready", "--node", "alpha"})
	if out := strings.TrimSpace(captureOutput(t, func() { runAll(t, []string{"fleet", "--changed"}) })); out == "no change" {
		t.Fatal("a new ready row went unnoticed")
	}
	runAll(t, []string{"node", "beat", "limit"})
	if out := strings.TrimSpace(captureOutput(t, func() { runAll(t, []string{"fleet", "--changed"}) })); out == "no change" {
		t.Fatal("a state change went unnoticed")
	}
}

func TestFleetPromptAndEmpty(t *testing.T) {
	inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t, []string{"init", "Fleet"})
	out := captureOutput(t, func() { runAll(t, []string{"fleet"}) })
	if !strings.Contains(out, "no nodes") {
		t.Fatalf("%s", out)
	}
	p := captureOutput(t, func() { runAll(t, []string{"get-prompt", "fleet-coordinator"}) })
	if !strings.Contains(p, "mw fleet --changed") || !strings.Contains(p, "node=NAME|any|-") || strings.Count(p, "\n") > 8 {
		t.Fatalf("prompt:\n%s", p)
	}
}
