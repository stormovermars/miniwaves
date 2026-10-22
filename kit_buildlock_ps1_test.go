package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestBuildlockHelper is the command the buildlock scripts wrap: it is only active when BUILDLOCK_HELPER is set,
// so a normal `go test` run passes it by. It runs the same on every OS, which keeps the wrapped command out of
// shell syntax: "args" records its argv, "exit" exits with BUILDLOCK_HELPER_CODE, "excl" proves exclusion.
func TestBuildlockHelper(t *testing.T) {
	mode := os.Getenv("BUILDLOCK_HELPER")
	if mode == "" {
		return
	}
	switch mode {
	case "args":
		os.WriteFile(os.Getenv("BUILDLOCK_HELPER_OUT"), []byte(strings.Join(flag.Args()[1:], " ")), 0o600)
	case "exit":
		code, _ := strconv.Atoi(os.Getenv("BUILDLOCK_HELPER_CODE"))
		os.Exit(code)
	case "excl": // two holders at once: the second mkdir fails and the helper exits 9
		m := os.Getenv("BUILDLOCK_HELPER_OUT")
		if err := os.Mkdir(m, 0o700); err != nil {
			os.Exit(9)
		}
		time.Sleep(300 * time.Millisecond)
		os.Remove(m)
	}
}

// pwshOrSkip finds PowerShell 7. CI has it on every runner, so a missing pwsh there is a failure, not a skip.
func pwshOrSkip(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("pwsh")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("pwsh missing on a CI runner: the buildlock.ps1 test must run there")
		}
		t.Skip("pwsh not installed (CI runs this test on all three OSes)")
	}
	return p
}

func ps1Setup(t *testing.T) (pwsh, script, lock string) {
	t.Helper()
	pwsh = pwshOrSkip(t)
	path := inTempDir(t)
	runAll(t, []string{"init", "Kit app"}, []string{"init", "--kit", "coordinated", "--job", "tiny"})
	script = filepath.Join(filepath.Dir(path), "buildlock.ps1")
	lock = filepath.Join(t.TempDir(), "lock")
	t.Setenv("BUILDLOCK_DIR", lock)
	t.Setenv("BUILDLOCK_TIMEOUT", "60")
	return
}

// runPS1 runs buildlock.ps1 around this test binary in helper mode; extra are the helper's arguments (after a positional ARGS, where Go's flag parsing stops, so -v -o -e stay untouched).
func runPS1(t *testing.T, pwsh, script, mode string, extra ...string) int {
	t.Helper()
	args := append([]string{"-NoProfile", "-NonInteractive", "-File", script, os.Args[0], "-test.run=^TestBuildlockHelper$", "ARGS"}, extra...)
	cmd := exec.Command(pwsh, args...)
	cmd.Env = append(os.Environ(), "BUILDLOCK_HELPER="+mode)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		t.Logf("buildlock.ps1 output: %s", out)
		return ee.ExitCode()
	}
	t.Fatal(err)
	return -1
}

// deadPID is the pid of a process that has exited.
func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestBuildlockHelper$")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

func writeLock(t *testing.T, lock string, pid int) {
	t.Helper()
	if err := os.MkdirAll(lock, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lock, "pid"), []byte(strconv.Itoa(pid)), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The PowerShell lock, on every OS the CI runs: wait + timeout 75, exit code passthrough, lock released,
// stale pid takeover, and the wrapped command's own -v/-o/-e reaching it.
func TestKitBuildlockPowerShell(t *testing.T) {
	pwsh, script, lock := ps1Setup(t)
	// a held lock (live owner: this test process) times out with 75
	writeLock(t, lock, os.Getpid())
	t.Setenv("BUILDLOCK_TIMEOUT", "1")
	if code := runPS1(t, pwsh, script, "exit"); code != 75 {
		t.Fatalf("held lock: exit %d, want 75", code)
	}
	t.Setenv("BUILDLOCK_TIMEOUT", "60")
	// a stale lock (dead owner) is taken over, the command's exit code is passed through, and the lock is released
	writeLock(t, lock, deadPID(t))
	t.Setenv("BUILDLOCK_HELPER_CODE", "3")
	if code := runPS1(t, pwsh, script, "exit"); code != 3 {
		t.Fatalf("stale lock: exit %d, want 3", code)
	}
	if _, err := os.Stat(lock); err == nil {
		t.Fatal("lock not released")
	}
	if _, err := os.Stat(lock + ".takeover"); err == nil {
		t.Fatal("takeover folder left behind")
	}
	// the wrapped command's flags are not eaten by PowerShell parameter binding
	out := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("BUILDLOCK_HELPER_OUT", out)
	if code := runPS1(t, pwsh, script, "args", "-v", "-o", "x.bin", "-e"); code != 0 {
		t.Fatalf("args: exit %d", code)
	}
	if b, _ := os.ReadFile(out); string(b) != "-v -o x.bin -e" {
		t.Fatalf("wrapped command got %q", b)
	}
}

// Four waiters on one stale lock run strictly one at a time: a second holder would fail the helper's mkdir (exit 9).
func TestKitBuildlockPowerShellTwoWaiterTakeover(t *testing.T) {
	pwsh, script, lock := ps1Setup(t)
	t.Setenv("BUILDLOCK_TIMEOUT", "120")
	t.Setenv("BUILDLOCK_HELPER_OUT", filepath.Join(t.TempDir(), "marker"))
	writeLock(t, lock, deadPID(t))
	const n = 4
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = runPS1(t, pwsh, script, "excl")
		}(i)
	}
	wg.Wait()
	for i, c := range codes {
		if c != 0 {
			t.Fatalf("waiter %d exit %d (two builds ran at once, or a waiter failed)", i, c)
		}
	}
	if _, err := os.Stat(lock); err == nil {
		t.Fatal("lock not released")
	}
}
