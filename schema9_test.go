package main

import (
	"os"
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
