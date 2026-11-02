package mw

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLockWaitFrom(t *testing.T) {
	for _, c := range []struct {
		env, goos string
		want      time.Duration
	}{
		{"", "linux", 5 * time.Second},
		{"", "darwin", 5 * time.Second},
		{"", "windows", 15 * time.Second},
		{"30", "linux", 30 * time.Second},
		{" 2.5 ", "windows", 2500 * time.Millisecond},
		{"0", "linux", 5 * time.Second},
		{"-3", "windows", 15 * time.Second},
		{"abc", "linux", 5 * time.Second},
	} {
		if got := lockWaitFrom(c.env, c.goos); got != c.want {
			t.Errorf("lockWaitFrom(%q, %s) = %s, want %s", c.env, c.goos, got, c.want)
		}
	}
	msg := lockTimeoutError("42", 15*time.Second).Error()
	for _, s := range []string{"board locked (pid 42)", "lock wait timed out after 15s", "MW_LOCK_WAIT"} {
		if !strings.Contains(msg, s) {
			t.Errorf("lock timeout message %q lacks %q", msg, s)
		}
	}
}

// Every non-zero exit names a reason on stderr, even a quiet or message-less exitError.
func TestMainExitAlwaysExplains(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	var e bytes.Buffer
	captureOutput(t, func() {
		if code := mainExit([]string{"list", "-j"}, &e); code == 0 {
			t.Fatal("list without a board must fail")
		}
	})
	if !strings.HasPrefix(e.String(), "mw: ") || strings.TrimSpace(e.String()) == "mw:" {
		t.Fatalf("stderr %q", e.String())
	}
	e.Reset()
	if code := mainExitErr(&exitError{code: 2, quiet: true}, &e); code != 2 || strings.TrimSpace(e.String()) != "mw: failed (exit 2)" {
		t.Fatalf("message-less exit: %d %q", code, e.String())
	}
	e.Reset()
	if code := mainExitErr(&exitError{code: 3, msg: "session x already exists", quiet: true}, &e); code != 3 || !strings.Contains(e.String(), "session x already exists") {
		t.Fatalf("quiet exit: %d %q", code, e.String())
	}
}
