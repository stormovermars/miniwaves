package mw

import (
	"encoding/json"
	"strings"
	"testing"
)

func routingBoard(t *testing.T) string {
	t.Helper()
	path := inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t, []string{"init", "Fleet"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"},
		[]string{"add", "task", "for beta", "-S", "ready", "--node", "beta", "--cost", "40"},
		[]string{"add", "task", "for anyone", "-S", "ready", "--node", "any"},
		[]string{"add", "task", "unrouted", "-S", "ready"})
	return path
}

func nextID(t *testing.T, args ...string) string {
	t.Helper()
	out := captureOutput(t, func() { runAll(t, append([]string{"next", "-j"}, args...)) })
	var n struct{ ID string }
	if err := json.Unmarshal([]byte(out), &n); err != nil {
		return strings.TrimSpace(out)
	}
	return n.ID
}

func TestRoutingNextAndList(t *testing.T) {
	routingBoard(t)
	if id := nextID(t, "--node", "alpha"); id != "TSK-002" {
		t.Fatalf("alpha got %q", id)
	}
	if id := nextID(t, "--node", "beta"); id != "TSK-001" {
		t.Fatalf("beta got %q", id)
	}
	if id := nextID(t); id != "TSK-001" { // no node name, no filter
		t.Fatalf("plain next got %q", id)
	}
	out := captureOutput(t, func() { runAll(t, []string{"list", "--node", "alpha", "-j"}) })
	if strings.Contains(out, "TSK-001") || !strings.Contains(out, "TSK-002") || !strings.Contains(out, "TSK-003") {
		t.Fatalf("list --node alpha: %s", out)
	}
	// this computer is alpha: next and next --claim skip rows routed elsewhere; --node - lifts it
	runAll(t, []string{"node", "set", "alpha"})
	if id := nextID(t); id != "TSK-002" {
		t.Fatalf("alpha default got %q", id)
	}
	if id := nextID(t, "--node", "-"); id != "TSK-001" {
		t.Fatalf("--node - got %q", id)
	}
	runAll(t, []string{"next", "--claim", "-s", "sa"})
	w, _ := load(".miniwaves.json")
	it, _ := itemByID(&w.Tenants[0], "TSK-002")
	if it.Status != "doing" || it.StartedOn != "alpha" {
		t.Fatalf("%+v", it)
	}
	runAll(t, []string{"reopen", "TSK-002"})
	w, _ = load(".miniwaves.json")
	if it, _ = itemByID(&w.Tenants[0], "TSK-002"); it.StartedOn != "" {
		t.Fatalf("started_on kept: %+v", it)
	}
}

func TestRoutingEditDefaultAndAsk(t *testing.T) {
	routingBoard(t)
	runAll(t, []string{"edit", "TSK-003", "node=beta", "cost=12"})
	if id := nextID(t, "--node", "alpha"); id != "TSK-002" {
		t.Fatalf("got %q", id)
	}
	runAll(t, []string{"edit", "TSK-003", "node=-", "cost=-"})
	w, _ := load(".miniwaves.json")
	if it, _ := itemByID(&w.Tenants[0], "TSK-003"); it.Node != "" || it.Cost != 0 {
		t.Fatalf("%+v", it)
	}
	for _, bad := range [][]string{{"edit", "TSK-003", "node=Bad Name"}, {"edit", "TSK-003", "cost=x"}, {"edit", "TSK-003", "cost=0x"}, {"add", "task", "x", "--node", "A_B"}} {
		if err := run(bad); err == nil {
			t.Fatalf("%v accepted", bad)
		}
	}
	// tenant default: an unrouted row follows it; any on the row overrides it
	runAll(t, []string{"tenant", "node", w.Tenants[0].Slug, "beta"})
	if id := nextID(t, "--node", "alpha"); id != "TSK-002" {
		t.Fatalf("tenant default ignored: %q", id)
	}
	if id := nextID(t, "--node", "beta"); id != "TSK-001" {
		t.Fatalf("got %q", id)
	}
	runAll(t, []string{"tenant", "node", w.Tenants[0].Slug, "-"})
	// an open ask is never offered for dispatch
	runAll(t, []string{"ask", "TSK-002", "which one?"})
	if id := nextID(t, "--node", "alpha"); id != "TSK-003" {
		t.Fatalf("ask row offered: %q", id)
	}
	w, _ = load(".miniwaves.json")
	if it, _ := itemByID(&w.Tenants[0], "TSK-001"); it.Cost != 40 || it.Node != "beta" {
		t.Fatalf("%+v", it)
	}
}

func TestRoutingOlderMwKeepsFields(t *testing.T) {
	// fields this build knows survive a round trip, and unknown ones too (the schema stays 9)
	var it Item
	if err := json.Unmarshal([]byte(`{"id":"TSK-001","node":"beta","cost":3,"started_on":"alpha","future":1}`), &it); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(it)
	for _, want := range []string{`"node":"beta"`, `"cost":3`, `"started_on":"alpha"`, `"future":1`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("lost %s in %s", want, b)
		}
	}
}

func TestNodeBeatLeft(t *testing.T) {
	path := routingBoard(t)
	runAll(t, []string{"node", "set", "alpha"}, []string{"node", "beat", "--left", "7"})
	if nb := readBeat(t, path, "alpha"); nb.Left == nil || *nb.Left != 7 {
		t.Fatalf("%+v", nb)
	}
	if err := run([]string{"node", "beat", "--left", "101"}); err == nil {
		t.Fatal("101 accepted")
	}
}
