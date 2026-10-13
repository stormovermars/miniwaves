package main

import (
	"strings"
	"testing"
)

func TestProtocolPrecedenceAndKeys(t *testing.T) {
	w := &Workspace{}
	tn := &Tenant{}
	if st, src := protocolFor(w, tn); src != "default" || len(st) != len(defaultProtocol) {
		t.Fatalf("default: %s %v", src, st)
	}
	w.Protocol = []string{"Build it", "Ship"}
	if st, src := protocolFor(w, tn); src != "workspace" || len(st) != 2 {
		t.Fatalf("workspace: %s %v", src, st)
	}
	tn.Protocol = []string{"Plan  the work", "plan the work", "Go"}
	st, src := protocolFor(w, tn)
	if src != "tenant" || len(st) != 2 || st[0].Key != "plan-the-work" || st[0].Title != "Plan  the work" || st[1].Key != "go" {
		t.Fatalf("tenant: %s %+v", src, st)
	}
	tn.Protocol = []string{" "}
	if _, src := protocolFor(w, tn); src != "workspace" {
		t.Fatalf("blank tenant list must fall through: %s", src)
	}
}

func TestTenantProtocolSetResetShow(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "Ws"}, []string{"tenant", "add", "b", "B"})
	show := func(args ...string) string {
		return captureOutput(t, func() { runAll(t, append([]string{"tenant", "protocol"}, args...)) })
	}
	if got := show("b"); !strings.Contains(got, "source: default") {
		t.Fatalf("show: %s", got)
	}
	runAll(t, []string{"tenant", "protocol", "--workspace", "set", "W one", "W two"})
	if got := show("b"); !strings.Contains(got, "source: workspace") || !strings.Contains(got, "w-one") {
		t.Fatalf("show ws: %s", got)
	}
	runAll(t, []string{"tenant", "protocol", "b", "set", "Step one", "Step two", "Step three"})
	if got := show("b"); !strings.Contains(got, "source: tenant") || !strings.Contains(got, "step-three") {
		t.Fatalf("show tenant: %s", got)
	}
	if got := show("--workspace"); !strings.Contains(got, "source: workspace") {
		t.Fatalf("show --workspace: %s", got)
	}
	// the other tenant keeps the workspace list; new waves take the tenant list, old waves keep theirs
	runAll(t, []string{"wave", "create", "Old", "-t", "b"})
	runAll(t, []string{"tenant", "protocol", "b", "set", "Only"})
	runAll(t, []string{"wave", "create", "New", "-t", "b"})
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := tenantBySlug(w, "b")
	if len(b.Protocol) != 1 {
		t.Fatalf("not persisted: %v", b.Protocol)
	}
	steps := func(id string) int { it, _ := itemByID(b, id); return len(it.Steps) }
	if steps("WAV-001") != 3 || steps("WAV-002") != 1 {
		t.Fatalf("steps old %d new %d", steps("WAV-001"), steps("WAV-002"))
	}
	runAll(t, []string{"tenant", "protocol", "b", "reset"})
	if got := show("b"); !strings.Contains(got, "source: workspace") {
		t.Fatalf("after reset: %s", got)
	}
	runAll(t, []string{"tenant", "protocol", "--workspace", "reset"})
	if got := show("b"); !strings.Contains(got, "source: default") {
		t.Fatalf("after ws reset: %s", got)
	}
	wantErr(t, "at least one step", "tenant", "protocol", "b", "set")
	wantErr(t, "not found", "tenant", "protocol", "zz")
}
