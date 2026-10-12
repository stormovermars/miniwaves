package main

import (
	"testing"
)

func twoTenants(t *testing.T) (string, string) {
	path, home := gateEnv(t)
	runAll(t, []string{"tenant", "add", "two", "Two"},
		[]string{"add", "task", "A", "-S", "ready"},
		[]string{"add", "task", "B", "-S", "ready", "-t", "two"})
	return path, home
}

func TestLocalSessionPerTenantAndAttribution(t *testing.T) { // F1 F2 F4
	path, _ := twoTenants(t)
	runAll(t, []string{"start", "TSK-001", "-t", "gate-app"}, []string{"start", "TSK-001", "-t", "two"})
	w := loadT(t, path)
	for _, id := range []string{"local@gate-app", "local@two"} {
		s, err := sessionByID(w, id)
		if err != nil || s.State != "working" || s.Item != "TSK-001" || s.Agent != "local" {
			t.Fatalf("%s: %v %+v", id, err, s)
		}
	}
	runAll(t, []string{"done", "TSK-001", "ok", "--no-evidence", "x"}, []string{"done", "TSK-001", "ok", "-t", "two", "--no-evidence", "x"})
	w = loadT(t, path)
	for i := range w.Tenants {
		it := w.Tenants[i].Items[0]
		last := it.Events[len(it.Events)-1]
		if it.By != "local" || last.Action != "done" || last.By != "local" || it.Assignee != "" {
			t.Fatalf("%+v", it)
		}
	}
	for _, s := range w.Sessions {
		if s.State != "idle" || s.Item != "" {
			t.Fatalf("local not idle: %+v", s)
		}
	}
	// idle/Item come from a scan: one of two items finishing leaves local working on the other
	runAll(t, []string{"add", "task", "C", "-S", "ready"}, []string{"add", "task", "D", "-S", "ready"})
	runAll(t, []string{"start", "TSK-002"}, []string{"start", "TSK-003"}, []string{"block", "TSK-002", "why"})
	if s, _ := sessionByID(loadT(t, path), "local@gate-app"); s.State != "working" || s.Item != "TSK-003" {
		t.Fatalf("%+v", s)
	}
	if it := itemOf(t, path, "TSK-002"); it.Events[len(it.Events)-1].By != "local" {
		t.Fatalf("%+v", it.Events)
	}
}

func TestLocalReservedAndRegisterOnlyOnStart(t *testing.T) { // F5
	path, _ := gateEnv(t)
	runAll(t, []string{"add", "task", "A", "-S", "ready"})
	wantErr(t, "reserved", "session", "start", "local")
	wantErr(t, "reserved", "start", "TSK-001", "-s", "local")
	// reads and non-start moves never register a session
	runAll(t, []string{"list", "-s", "ghost"}, []string{"show", "TSK-001", "-s", "ghost"})
	runAll(t, []string{"block", "TSK-001", "x"})
	if _, err := sessionByID(loadT(t, path), "ghost"); err == nil {
		t.Fatal("ghost registered by a read or non-start")
	}
	runAll(t, []string{"ready", "TSK-001", "-s", "ghost"})
	if _, err := sessionByID(loadT(t, path), "ghost"); err == nil {
		t.Fatal("ghost registered by ready")
	}
	runAll(t, []string{"start", "TSK-001", "-s", "ghost"})
	if s, err := sessionByID(loadT(t, path), "ghost"); err != nil || s.Item != "TSK-001" {
		t.Fatalf("%v %+v", err, s)
	}
}

func TestStartNeedsNoSession(t *testing.T) {
	path, _ := gateEnv(t)
	runAll(t, []string{"add", "task", "A", "-S", "ready"}, []string{"add", "task", "B", "-S", "ready"}, []string{"add", "task", "C", "-S", "ready"})
	runAll(t, []string{"start", "TSK-001"})
	if it := itemOf(t, path, "TSK-001"); it.Status != "doing" || it.Assignee != "local@gate-app" {
		t.Fatalf("%+v", it)
	}
	if s, err := sessionByID(loadT(t, path), "local@gate-app"); err != nil || s.Item != "TSK-001" {
		t.Fatalf("%v %+v", err, s)
	}
	runAll(t, []string{"done", "TSK-001", "ok"})
	// an unknown session is registered silently, by any command
	t.Setenv("MW_SESSION", "ghost")
	runAll(t, []string{"start", "TSK-002"}, []string{"note", "TSK-002", "hi"}, []string{"list"}, []string{"log"})
	if s, err := sessionByID(loadT(t, path), "ghost"); err != nil || s.Item != "TSK-002" {
		t.Fatalf("%v %+v", err, s)
	}
	t.Setenv("MW_SESSION", "")
	runAll(t, []string{"session", "stop", "local@gate-app"}) // local owns nothing now
	runAll(t, []string{"start", "TSK-003"})                  // revived
	if s, _ := sessionByID(loadT(t, path), "local@gate-app"); s.State != "working" {
		t.Fatalf("%+v", s)
	}
}

func TestImplicitLocalMayOwnManyItems(t *testing.T) {
	path, _ := gateEnv(t)
	runAll(t, []string{"add", "task", "A", "-S", "ready"}, []string{"add", "task", "B", "-S", "ready"})
	runAll(t, []string{"start", "TSK-001"}, []string{"start", "TSK-002"})
	if a, b := itemOf(t, path, "TSK-001"), itemOf(t, path, "TSK-002"); a.Status != "doing" || b.Status != "doing" || a.Assignee != "local@gate-app" || b.Assignee != "local@gate-app" {
		t.Fatalf("%+v %+v", a, b)
	}
	// a named session keeps the one-item rule
	t.Setenv("MW_SESSION", "worker")
	runAll(t, []string{"add", "task", "C", "-S", "ready"}, []string{"add", "task", "D", "-S", "ready"})
	runAll(t, []string{"start", "TSK-003"})
	wantErr(t, "already owns", "start", "TSK-004")
}
