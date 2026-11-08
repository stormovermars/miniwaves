package mw

import (
	"os"
	"path/filepath"
	"testing"
)

// BUG-025: a saving command writes only the tenants it changed; an index tenant without a
// file never gets an empty stub.
func TestSaveNeverCreatesStubTenantFiles(t *testing.T) {
	path := crossBoard(t)
	other := tenantPath(path, "job")
	if err := os.Remove(other); err != nil {
		t.Fatal(err)
	}
	ev := filepath.Join(t.TempDir(), "ev.txt")
	os.WriteFile(ev, []byte("proof"), 0o644)
	for _, args := range [][]string{
		{"start", "TSK-001", "-t", "app"},
		{"note", "TSK-001", "n", "-t", "app"},
		{"done", "TSK-001", "ok", "--evidence", ev, "-t", "app"},
		{"add", "task", "More", "-t", "app"},
		{"board", "-A"},
	} {
		runAll(t, args)
		if _, err := os.Stat(other); err == nil {
			t.Fatalf("%v created the stub file %s", args, other)
		}
	}
	if _, err := os.Stat(tenantPath(path, "app")); err != nil {
		t.Fatal("acting tenant file missing")
	}
	// a tenant that gains content is still written
	runAll(t, []string{"add", "task", "Real", "-t", "job"})
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("tenant with content not written: %v", err)
	}
}
