//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A lock-free reader holding the board open (Go opens without FILE_SHARE_DELETE) makes the
// writer's rename fail with a sharing violation; renameRetry waits it out.
func TestWindowsRenameOverOpenFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "board.json")
	if err := os.WriteFile(target, []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	go func() { time.Sleep(50 * time.Millisecond); f.Close() }()
	if err := writeFileAtomic(target, []byte("new\n")); err != nil {
		t.Fatalf("rename over an open file: %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "new\n" {
		t.Fatalf("content %q", b)
	}
}
