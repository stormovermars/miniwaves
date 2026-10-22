package main

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// A schema-9 index must be refused by the guard of a schema-8 build (it would drop jobs, job and fanfare sets).
func TestSchema9RefusedBySchema8Reader(t *testing.T) {
	if err := checkSchema("x", 9, 8, true); err == nil || !strings.Contains(err.Error(), "newer than this mw supports (schema 8): upgrade mw") {
		t.Fatalf("schema 8 build accepted a schema 9 index: %v", err)
	}
	if err := checkSchema("x", 9, 9, true); err != nil {
		t.Fatal(err)
	}
}

// mw migrate takes a schema-8 board to 9 without losing jobs, a tenant's job, or the tenant files.
func TestMigrate8To9Lossless(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "Jobs app"}, []string{"tenant", "add", "sheet", "Sheet"},
		[]string{"jobs", "enable", "j1", "-t", "sheet"}, []string{"jobs", "set", "j1", "title=T"})
	b, _ := os.ReadFile(path)
	tb, _ := os.ReadFile(tenantPath(path, "sheet"))
	v8 := strings.Replace(string(b), `"version":9`, `"version":8`, 1)
	if v8 == string(b) {
		t.Fatal("no version key")
	}
	os.WriteFile(path, []byte(v8), 0o600)
	wantErr(t, "run mw migrate", "board")
	runAll(t, []string{"migrate"})
	after, _ := os.ReadFile(path)
	ta, _ := os.ReadFile(tenantPath(path, "sheet"))
	if string(after) != string(b) || string(ta) != string(tb) {
		t.Fatalf("migrate 8->9 changed more than the version:\n%s\n%s", b, after)
	}
	if bak, _ := os.ReadFile(path + ".bak8"); string(bak) != v8 {
		t.Fatal("no backup")
	}
}

// The real tiny-corpus index (0.2.9-dev, dotted job keys): migrating keeps every job value, rewrites the
// keys to snake_case, and `jobs show -j` is the same before and after. Idempotent.
func TestMigrateDottedJobKeys(t *testing.T) {
	path := inTempDir(t)
	os.WriteFile(path, fixture(t, "v8-tiny-corpus.json"), 0o600)
	w, err := loadScopedFrom(path, 8, true)
	if err != nil {
		t.Fatal(err)
	}
	if w.Jobs["tiny"].NewsURL == "" || w.Jobs["tiny"].MaxWorkers != 3 {
		t.Fatalf("dotted keys not read: %+v", w.Jobs["tiny"])
	}
	before, _ := json.Marshal(jobView(w, "tiny"))
	runAll(t, []string{"migrate"})
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), `"news.`) || strings.Contains(string(b), `"pane.`) || !strings.Contains(string(b), `"news_url"`) || !strings.Contains(string(b), `"version":9`) {
		t.Fatalf("not migrated: %s", b)
	}
	var after, beforeM map[string]any
	out := captureOutput(t, func() { runAll(t, []string{"jobs", "show", "tiny", "-j"}) })
	json.Unmarshal([]byte(out), &after)
	json.Unmarshal(before, &beforeM)
	if !reflect.DeepEqual(after, beforeM) {
		t.Fatalf("jobs show differs:\n%v\n%v", beforeM, after)
	}
	runAll(t, []string{"migrate"}) // already 9: nothing to do
	if b2, _ := os.ReadFile(path); string(b2) != string(b) {
		t.Fatal("second migrate changed the board")
	}
	// unknown job keys survive a rewrite
	x := strings.Replace(string(b), `"max_workers":3`, `"max_workers":3,"zzz":{"a":1}`, 1)
	os.WriteFile(path, []byte(x), 0o600)
	runAll(t, []string{"jobs", "set", "tiny", "title=T2"})
	if b3, _ := os.ReadFile(path); !strings.Contains(string(b3), `"zzz":{"a":1}`) {
		t.Fatalf("unknown job key dropped: %s", b3)
	}
}

// 8 -> 9 writes only the index: a tenant file that a rewrite would normalize stays byte for byte.
func TestMigrate8To9LeavesTenantFilesAlone(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "Jobs app"}, []string{"tenant", "add", "sheet", "Sheet"}, []string{"add", "task", "X", "-t", "sheet"})
	b, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(b), `"version":9`, `"version":8`, 1)), 0o600)
	tp := tenantPath(path, "sheet")
	tb, _ := os.ReadFile(tp)
	odd := append(append([]byte{}, tb...), '\n', '\n', ' ') // a rewrite would drop this
	os.WriteFile(tp, odd, 0o600)
	runAll(t, []string{"migrate"})
	if ta, _ := os.ReadFile(tp); string(ta) != string(odd) {
		t.Fatal("migrate 8->9 rewrote a tenant file")
	}
	if a, _ := os.ReadFile(path); !strings.Contains(string(a), `"version":9`) {
		t.Fatal("index not migrated")
	}
}

// A 0.2.x board with archived tenants stores "archived": true in the index; migrate must read it
// (it once failed: cannot unmarshal bool into Tenant.archived) and keep the tenants archived.
func TestMigrate8To9ArchivedTenants(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "W"}, []string{"tenant", "add", "a"}, []string{"tenant", "add", "b"},
		[]string{"add", "task", "kept", "-t", "b"}, []string{"tenant", "archive", "b"})
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"archived":true`) {
		t.Fatalf("fixture has no archived flag: %s", b)
	}
	os.WriteFile(path, []byte(strings.Replace(string(b), `"version":9`, `"version":8`, 1)), 0o600)
	runAll(t, []string{"migrate"})
	if after, _ := os.ReadFile(path); string(after) != string(b) {
		t.Fatalf("migrate 8->9 changed more than the version:\n%s\n%s", b, after)
	}
	runAll(t, []string{"migrate"}) // already 9: still readable
	runAll(t, []string{"tenant", "restore", "b"})
	if got := out(t, "list", "-t", "b"); !strings.Contains(got, "kept") {
		t.Fatalf("archived tenant lost its items: %q", got)
	}
}
