package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var origHome = os.Getenv("HOME")

// v7Board is a schema 7 single-file board: tenants x per items, a few sessions.
func v7Board(tenants, per int) []byte {
	var ts []map[string]any
	var ss []map[string]any
	for i := 0; i < tenants; i++ {
		slug := fmt.Sprintf("t%d", i)
		var items []map[string]any
		for j := 1; j <= per; j++ {
			st := "done"
			if j%10 == 0 {
				st = "ready"
			}
			items = append(items, map[string]any{"id": fmt.Sprintf("TSK-%03d", j), "kind": "task", "title": fmt.Sprintf("job %d", j), "status": st,
				"created": "2026-01-01T00:00:00Z", "updated": "2026-01-01T00:00:00Z", "judged": "ungated",
				"events": []map[string]any{{"at": "2026-01-01T00:00:00Z", "action": "created"}}})
		}
		ts = append(ts, map[string]any{"slug": slug, "name": slug, "path": "/x/" + slug, "next": map[string]int{"TSK": per + 1}, "items": items})
		ss = append(ss, map[string]any{"id": "s-" + slug, "tenant": slug, "state": "idle", "updated": "2026-01-01T00:00:00Z"})
	}
	b, _ := json.Marshal(map[string]any{"version": 7, "name": "old", "tenants": ts, "sessions": ss, "gates": map[string]string{"evidence": "off"}})
	return b
}

func TestMigrateSchema7To8(t *testing.T) {
	path := inTempDir(t)
	raw := v7Board(5, 1000) // 5000 items
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	arch := `{"version":7,"tenants":[{"slug":"t0","items":[{"id":"TSK-777","kind":"task","title":"archived one","status":"done","created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z","events":[{"at":"2026-01-01T00:00:00Z","action":"created"}]}]}]}`
	rootArchive := filepath.Join(filepath.Dir(path), ".miniwaves.archive.json")
	if err := os.WriteFile(rootArchive, []byte(arch), 0o600); err != nil {
		t.Fatal(err)
	}
	wantErr(t, "run mw migrate", "board")
	before, _, _, err := migrateBytes(raw, path)
	if err != nil {
		t.Fatal(err)
	}
	runAll(t, []string{"migrate"})
	if b, err := os.ReadFile(path + ".bak7"); err != nil || string(b) != string(raw) {
		t.Fatalf("backup: %v", err)
	}
	ix, _ := os.ReadFile(path)
	if strings.Contains(string(ix), `"items"`) || !strings.Contains(string(ix), `"version":9`) {
		t.Fatalf("index must be schema 9 without items: %.300s", ix)
	}
	if _, err := os.Stat(rootArchive); err == nil {
		t.Fatal("the root archive was left behind")
	}
	if m := jsonOut(t, "show", "TSK-777", "-t", "t0", "-j"); m["id"] != "TSK-777" {
		t.Fatalf("archived item lost by migrate: %v", m)
	}
	n := 0
	for i := 0; i < 5; i++ {
		b, err := os.ReadFile(tenantPath(path, fmt.Sprintf("t%d", i)))
		if err != nil {
			t.Fatal(err)
		}
		var td tenantDoc
		_ = json.Unmarshal(b, &td)
		n += len(td.Items)
		if len(td.Sessions) != 1 {
			t.Fatalf("t%d sessions %v", i, td.Sessions)
		}
	}
	if n != 5000 {
		t.Fatalf("%d items", n)
	}
	if got, want := canon(t, loadT(t, path)), canon(t, before); got != want {
		t.Fatal("migrated board differs from the schema 7 content")
	}
	runAll(t, []string{"board", "-A"}, []string{"list", "-t", "t3"})
	if out := captureOutput(t, func() { runAll(t, []string{"migrate"}) }); !strings.Contains(out, "already schema 9") {
		t.Fatal(out) // idempotent
	}
}

func TestIndexAndTenantFilesLayout(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "App"}, []string{"add", "task", "x", "-S", "ready"})
	ix, _ := os.ReadFile(path)
	if strings.Contains(string(ix), "TSK-001") {
		t.Fatalf("items leaked into the index: %s", ix)
	}
	if b, err := os.ReadFile(tenantPath(path, "app")); err != nil || !strings.Contains(string(b), "TSK-001") {
		t.Fatalf("tenant file: %v %s", err, b)
	}
	// a write to one tenant leaves the other tenant's file untouched
	runAll(t, []string{"tenant", "add", "other", "Other", "-d", filepath.Dir(path)})
	before, _ := os.ReadFile(tenantPath(path, "other"))
	runAll(t, []string{"add", "task", "y", "-t", "app"})
	if after, _ := os.ReadFile(tenantPath(path, "other")); string(after) != string(before) {
		t.Fatal("a write to tenant app rewrote tenant other")
	}
}

func TestTwentyParallelClaims(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "App"})
	for i := 0; i < 20; i++ {
		runAll(t, []string{"add", "task", fmt.Sprintf("job %d", i), "-S", "ready"})
	}
	bin := filepath.Join(t.TempDir(), "mw")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	_, thisFile, _, _ := runtime.Caller(0)
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = filepath.Dir(thisFile)
	build.Env = append(os.Environ(), "HOME="+origHome) // the real module cache, not the test HOME
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	dir := filepath.Dir(path)
	got := make([]string, 20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := exec.Command(bin, "next", "--claim", "-t", "app", "-s", fmt.Sprintf("p%02d", i), "-a", "builder", "-j")
			c.Dir = dir
			h := t.TempDir()
			c.Env = append(os.Environ(), "HOME="+h, "USERPROFILE="+h)
			out, err := c.CombinedOutput()
			if err != nil {
				got[i] = "ERR " + string(out)
				return
			}
			var m struct{ ID string }
			_ = json.Unmarshal(out, &m)
			got[i] = m.ID
		}(i)
	}
	wg.Wait()
	seen := map[string]bool{}
	for _, g := range got {
		if g == "" || strings.HasPrefix(g, "ERR") || seen[g] {
			t.Fatalf("bad or double claim %q in %v", g, got)
		}
		seen[g] = true
	}
	if len(seen) != 20 {
		t.Fatalf("%d distinct claims", len(seen))
	}
}

func TestSessionLiveInAnotherTenantIsRefused(t *testing.T) {
	inTempDir(t)
	runAll(t, []string{"init", "App"}, []string{"tenant", "add", "other", "Other", "-d", mustGetwd()},
		[]string{"add", "task", "a", "-t", "app", "-S", "ready"}, []string{"add", "task", "b", "-t", "other", "-S", "ready"})
	runAll(t, []string{"start", "TSK-001", "-t", "app", "-s", "bob"})
	err := run([]string{"start", "TSK-001", "-t", "other", "-s", "bob"})
	if err == nil || !strings.Contains(err.Error(), "session and tenant do not match") {
		t.Fatalf("a live session name must not be registered twice: %v", err)
	}
	if err := run([]string{"session", "start", "bob", "-t", "other"}); err == nil {
		t.Fatal("session start registered a live name in another tenant")
	}
}
