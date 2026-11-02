//go:build windows

package fsx

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// A reader's handle from ReadFile's open lets a writer replace the file. MoveFileEx
// (os.Rename) cannot replace an open target even with FILE_SHARE_DELETE; Replace's POSIX
// rename can.
func TestWindowsSharedOpenAllowsRename(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.json")
	if err := os.WriteFile(p, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := OpenShared(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tmp := filepath.Join(dir, "x.json.tmp")
	if err := os.WriteFile(tmp, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Replace(tmp, p); err != nil { // no retry: the shared handle must not block it
		t.Fatalf("rename over a shared-open file: %v", err)
	}
	if b, err := ReadFile(p); err != nil || string(b) != "new" {
		t.Fatalf("after rename %q %v", b, err)
	}
}

// A handle that denies all sharing (an antivirus scan, an exclusive writer) makes the open fail
// with a sharing violation; ReadFile waits it out.
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
	if _, err := OpenShared(p); !transientShareErr(err) {
		syscall.CloseHandle(h)
		t.Fatalf("exclusive handle: want a sharing violation, got %v", err)
	}
	go func() { time.Sleep(100 * time.Millisecond); syscall.CloseHandle(h) }()
	if b, err := ReadFile(p); err != nil || string(b) != "data" {
		t.Fatalf("read after the exclusive handle closed: %q %v", b, err)
	}
}

// With the POSIX rename unavailable (pre-1809, FAT) Replace falls back to MoveFileEx.
func TestWindowsReplaceFallback(t *testing.T) {
	defer posixRenameOff.Store(posixRenameOff.Load())
	posixRenameOff.Store(true)
	dir := t.TempDir()
	p, tmp := filepath.Join(dir, "x.json"), filepath.Join(dir, "x.json.tmp")
	if err := os.WriteFile(p, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Replace(tmp, p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "new" {
		t.Fatalf("content %q", b)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("tmp left behind: %v", err)
	}
	if !posixUnsupported(windows.ERROR_INVALID_PARAMETER) || posixUnsupported(windows.ERROR_ACCESS_DENIED) {
		t.Fatal("posixUnsupported classification")
	}
}
