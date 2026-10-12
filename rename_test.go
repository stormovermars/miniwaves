package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A rename that fails while a reader holds the target (Windows sharing violation) is retried.
func TestRenameRetryTransientFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "board.json")
	calls := 0
	old, oldBackoff, oldOn := renameFn, renameBackoff, renameRetryOn
	t.Cleanup(func() { renameFn, renameBackoff, renameRetryOn = old, oldBackoff, oldOn })
	renameRetryOn = true
	renameBackoff = time.Millisecond
	renameFn = func(a, b string) error {
		calls++
		if calls < 4 {
			return &os.LinkError{Op: "rename", Old: a, New: b, Err: errors.New("Access is denied.")}
		}
		return os.Rename(a, b)
	}
	if err := writeFileAtomic(target, []byte("ok\n")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "ok\n" || calls != 4 {
		t.Fatalf("content %q after %d calls", b, calls)
	}
}

// A rename that never succeeds fails after renameTries and leaves no temp file behind.
func TestRenameRetryGivesUp(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	old, oldBackoff, oldOn := renameFn, renameBackoff, renameRetryOn
	t.Cleanup(func() { renameFn, renameBackoff, renameRetryOn = old, oldBackoff, oldOn })
	renameRetryOn = true
	renameBackoff = time.Millisecond
	renameFn = func(a, b string) error { calls++; return errors.New("Access is denied.") }
	if err := writeFileAtomic(filepath.Join(dir, "board.json"), []byte("x")); err == nil {
		t.Fatal("want error")
	}
	if calls != renameTries {
		t.Fatalf("calls %d, want %d", calls, renameTries)
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(m) != 0 {
		t.Fatalf("temp left: %v", m)
	}
}

// A missing source is not worth retrying.
func TestRenameRetryNotExist(t *testing.T) {
	calls := 0
	old := renameFn
	t.Cleanup(func() { renameFn = old })
	renameFn = func(a, b string) error { calls++; return os.Rename(a, b) }
	err := renameRetry(filepath.Join(t.TempDir(), "nope"), filepath.Join(t.TempDir(), "x"))
	if !errors.Is(err, fs.ErrNotExist) || calls != 1 {
		t.Fatalf("err %v after %d calls", err, calls)
	}
}

// After a lock wait times out, the abandoned waiter must give the lock back once it wins it,
// so a later writer in the same process is not blocked by a leaked handle (on Windows a
// leaked LockFileEx handle would hold the lock until process exit).
func TestLockTimeoutDoesNotLeakLock(t *testing.T) {
	lp := filepath.Join(t.TempDir(), "x.lock")
	unlock, err := lockFile(lp, time.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockFile(lp, time.Now().Add(50*time.Millisecond)); err == nil {
		t.Fatal("second lock should time out")
	}
	unlock()
	u2, err := lockFile(lp, time.Now().Add(2*time.Second))
	if err != nil {
		t.Fatalf("lock leaked after timeout: %v", err)
	}
	u2()
}

// Off Windows a failed rename is a single attempt.
func TestRenameSingleAttemptOffWindows(t *testing.T) {
	calls := 0
	old, oldOn := renameFn, renameRetryOn
	t.Cleanup(func() { renameFn, renameRetryOn = old, oldOn })
	renameRetryOn = false
	renameFn = func(a, b string) error { calls++; return errors.New("denied") }
	if err := renameRetry("a", "b"); err == nil || calls != 1 {
		t.Fatalf("err %v after %d calls", err, calls)
	}
}
