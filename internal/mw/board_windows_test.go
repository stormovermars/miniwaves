//go:build windows

package mw

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stormovermars/miniwaves/internal/fsx"
)

// The atomic writer replaces the board while a shared reader holds it open, at once (no retry
// loop), and the reader keeps seeing the old contents through its handle.
func TestWindowsAtomicWriteWhileSharedReaderOpen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	if err := os.WriteFile(p, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := fsx.OpenShared(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	defer func(n int) { renameTries = n }(renameTries)
	renameTries = 1
	if err := writeFileAtomic(p, []byte("new")); err != nil {
		t.Fatalf("atomic write over a shared-open file: %v", err)
	}
	if b, err := fsx.ReadFile(p); err != nil || string(b) != "new" {
		t.Fatalf("after write %q %v", b, err)
	}
	buf := make([]byte, 8)
	if n, _ := f.Read(buf); string(buf[:n]) != "old" {
		t.Fatalf("open reader sees %q, want the old contents", buf[:n])
	}
}
