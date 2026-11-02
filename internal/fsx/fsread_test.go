package fsx

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadFileSharedReads(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	if err := os.WriteFile(p, []byte(`{"version":8}`), 0600); err != nil {
		t.Fatal(err)
	}
	if b, err := ReadFile(p); err != nil || string(b) != `{"version":8}` {
		t.Fatalf("read %q %v", b, err)
	}
	if _, err := Stat(p); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	if _, err := ReadFile(p + ".missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file: %v", err)
	}
	if time.Since(t0) > 200*time.Millisecond {
		t.Fatal("a missing file must not be retried")
	}
	if _, err := Stat(p + ".missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat missing: %v", err)
	}
}
