//go:build windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A reader's handle from readFileShared's open lets a writer rename over the file.
func TestWindowsSharedOpenAllowsRename(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.json")
	if err := os.WriteFile(p, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := openShared(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tmp := filepath.Join(dir, "x.json.tmp")
	if err := os.WriteFile(tmp, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, p); err != nil { // no retry: the shared handle must not block it
		t.Fatalf("rename over a shared-open file: %v", err)
	}
	if b, err := readFileShared(p); err != nil || string(b) != "new" {
		t.Fatalf("after rename %q %v", b, err)
	}
}

// A handle that denies all sharing (an antivirus scan, an exclusive writer) makes the open fail
// with a sharing violation; readFileShared waits it out.
func TestWindowsReadRetriesSharingViolation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.json")
	if err := os.WriteFile(p, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	u, _ := syscall.UTF16PtrFromString(p)
	h, err := syscall.CreateFile(u, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openShared(p); !transientShareErr(err) {
		syscall.CloseHandle(h)
		t.Fatalf("exclusive handle: want a sharing violation, got %v", err)
	}
	go func() { time.Sleep(100 * time.Millisecond); syscall.CloseHandle(h) }()
	if b, err := readFileShared(p); err != nil || string(b) != "data" {
		t.Fatalf("read after the exclusive handle closed: %q %v", b, err)
	}
}
