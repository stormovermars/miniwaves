package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestKitCoordinatedIdempotentNoOverwrite(t *testing.T) {
	path := inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t, []string{"init", "Kit app"})
	dir := filepath.Dir(path)
	if err := os.WriteFile(filepath.Join(dir, "PLAN.md"), []byte("mine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	out := captureOutput(t, func() { runAll(t, []string{"init", "--kit", "coordinated", "--job", "tiny"}) })
	for _, want := range []string{"job tiny created", "wrote NEWSDESK.md, buildlock.sh, buildlock.ps1", "skipped (exists) PLAN.md",
		"mw get-prompt dispatcher --job tiny", "mw get-prompt newsdesk --job tiny"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "PLAN.md")); string(b) != "mine\n" {
		t.Fatalf("PLAN.md overwritten: %q", b)
	}
	w := loadT(t, path)
	if _, ok := w.Jobs["tiny"]; !ok || len(jobView(w, "tiny").Tenants) != 0 || w.Tenants[0].Job != "" {
		t.Fatalf("job not created tenant-less: %+v", w.Jobs)
	}
	for _, f := range []string{"NEWSDESK.md", "buildlock.sh", "buildlock.ps1"} {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil || strings.Contains(string(b), "@@") || strings.Contains(string(b), "\r") {
			t.Fatalf("%s: %v %q", f, err, b)
		}
	}
	if st, _ := os.Stat(filepath.Join(dir, "buildlock.sh")); runtime.GOOS != "windows" && st.Mode()&0100 == 0 {
		t.Fatal("buildlock.sh not executable")
	}
	// second run: nothing written, all skipped, same bytes
	before, _ := os.ReadFile(filepath.Join(dir, "NEWSDESK.md"))
	out = captureOutput(t, func() { runAll(t, []string{"init", "--kit", "coordinated", "--job", "tiny"}) })
	if strings.Contains(out, "wrote") || strings.Contains(out, "created") || !strings.Contains(out, "skipped (exists) PLAN.md, NEWSDESK.md, buildlock.sh, buildlock.ps1") {
		t.Fatalf("not idempotent:\n%s", out)
	}
	if after, _ := os.ReadFile(filepath.Join(dir, "NEWSDESK.md")); string(after) != string(before) {
		t.Fatal("NEWSDESK.md changed")
	}
	// errors
	for _, args := range [][]string{
		{"init", "--kit", "coordinated"},
		{"init", "--kit", "coordinated", "--job", "Bad Id"},
		{"init", "--kit", "other", "--job", "tiny"},
	} {
		if err := run(args); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
}

func TestKitBuildlockShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh lock")
	}
	path := inTempDir(t)
	runAll(t, []string{"init", "Kit app"}, []string{"init", "--kit", "coordinated", "--job", "tiny"})
	script := filepath.Join(filepath.Dir(path), "buildlock.sh")
	lock := filepath.Join(t.TempDir(), "lock")
	t.Setenv("BUILDLOCK_DIR", lock)
	t.Setenv("BUILDLOCK_TIMEOUT", "1")
	// a held lock (live owner: this test process) times out with 75
	if err := os.Mkdir(lock, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lock, "pid"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		t.Fatal(err)
	}
	if code := runShell(t, script, "true"); code != 75 {
		t.Fatalf("held lock: exit %d", code)
	}
	// a stale lock (dead owner) is taken over, the command's exit code is passed through, and the lock is released
	if err := os.WriteFile(filepath.Join(lock, "pid"), []byte("999999"), 0600); err != nil {
		t.Fatal(err)
	}
	if code := runShell(t, script, "sh", "-c", "exit 3"); code != 3 {
		t.Fatalf("stale lock: exit %d", code)
	}
	if _, err := os.Stat(lock); err == nil {
		t.Fatal("lock not released")
	}
}

// Eight waiters on one stale lock must still run one at a time (the takeover is an atomic rename), and a live
// owner that kill -0 cannot signal (pid 1) is never mistaken for a dead one: the wait times out, no spin.
func TestKitBuildlockTakeoverRace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh lock")
	}
	path := inTempDir(t)
	runAll(t, []string{"init", "Kit app"}, []string{"init", "--kit", "coordinated", "--job", "tiny"})
	script := filepath.Join(filepath.Dir(path), "buildlock.sh")
	dir := t.TempDir()
	lock := filepath.Join(dir, "lock")
	marker := filepath.Join(dir, "marker")
	t.Setenv("BUILDLOCK_DIR", lock)
	t.Setenv("BUILDLOCK_TIMEOUT", "60")
	os.Mkdir(lock, 0700)
	os.WriteFile(filepath.Join(lock, "pid"), []byte("999999"), 0600)
	codes := make(chan int, 8)
	for i := 0; i < 8; i++ {
		go func() {
			codes <- runShell(t, script, "sh", "-c", "mkdir "+marker+" || exit 9; sleep 0.2; rmdir "+marker)
		}()
	}
	for i := 0; i < 8; i++ {
		if c := <-codes; c != 0 {
			t.Fatalf("waiter exit %d (two builds ran at once)", c)
		}
	}
	if os.Geteuid() == 0 {
		return
	}
	os.Mkdir(lock, 0700)
	os.WriteFile(filepath.Join(lock, "pid"), []byte("1"), 0600)
	os.Chmod(dir, 0500)
	defer os.Chmod(dir, 0700)
	t.Setenv("BUILDLOCK_TIMEOUT", "2")
	start := time.Now()
	if code := runShell(t, script, "true"); code != 75 || time.Since(start) > 10*time.Second {
		t.Fatalf("foreign live owner: exit %d after %v", code, time.Since(start))
	}
}

func runShell(t *testing.T, script string, args ...string) int {
	t.Helper()
	err := exec.Command("sh", append([]string{script}, args...)...).Run()
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	t.Fatal(err)
	return -1
}

func TestReapSkipsJobRoles(t *testing.T) {
	path := gatePath(t)
	runAll(t,
		[]string{"add", "task", "A", "-S", "ready"},
		[]string{"add", "task", "B", "-S", "ready"},
		[]string{"add", "task", "C", "-S", "ready"},
		[]string{"start", "TSK-001", "-s", "boss", "-a", "dispatcher"},
		[]string{"start", "TSK-002", "-s", "desk", "-a", "newsdesk"},
		[]string{"start", "TSK-003", "-s", "worker", "-a", "builder"},
	)
	old := 10 * time.Hour
	backdate(t, path, "boss", "TSK-001", old)
	backdate(t, path, "desk", "TSK-002", old)
	backdate(t, path, "worker", "TSK-003", old)
	// no job yet: today's behavior, everything idle is reaped (dry run)
	out := captureOutput(t, func() { runAll(t, []string{"session", "reap", "-t", "gate-app", "--older-than", "1h", "-n"}) })
	if !strings.Contains(out, "session boss") || !strings.Contains(out, "session desk") || !strings.Contains(out, "session worker") {
		t.Fatalf("no job:\n%s", out)
	}
	runAll(t, []string{"jobs", "enable", "tiny", "-t", "gate-app"})
	// wr's exact argv
	out = captureOutput(t, func() { runAll(t, []string{"session", "reap", "-t", "gate-app", "--older-than", "1h"}) })
	if strings.Contains(out, "reaped session boss") || strings.Contains(out, "reaped session desk") || !strings.Contains(out, "reaped session worker") ||
		!strings.Contains(out, "spared dispatcher session boss (job tiny)") || !strings.Contains(out, "spared newsdesk session desk (job tiny)") {
		t.Fatalf("job roles not skipped:\n%s", out)
	}
	if it := itemOf(t, path, "TSK-001"); it.Status != "doing" || it.Assignee != "boss" {
		t.Fatalf("dispatcher touched: %+v", it)
	}
	if it := itemOf(t, path, "TSK-003"); it.Status != "ready" {
		t.Fatalf("worker not reaped: %+v", it)
	}
	// -A too; --except still works for the others
	out = captureOutput(t, func() { runAll(t, []string{"session", "reap", "-A", "--older-than", "1h", "-n"}) })
	if strings.Contains(out, "would reap session boss") || strings.Contains(out, "would reap session desk") {
		t.Fatalf("-A:\n%s", out)
	}
	var js struct {
		Spared []sparedRow `json:"spared"`
	}
	if err := json.Unmarshal([]byte(captureOutput(t, func() { runAll(t, []string{"session", "reap", "-A", "--older-than", "1h", "-n", "-j"}) })), &js); err != nil ||
		len(js.Spared) != 2 || js.Spared[0] != (sparedRow{"boss", "gate-app", "dispatcher", "tiny"}) {
		t.Fatalf("spared rows: %+v %v", js.Spared, err)
	}
	// a tenant that left the job is reaped as before
	runAll(t, []string{"jobs", "disable", "tiny"})
	out = captureOutput(t, func() {
		runAll(t, []string{"session", "reap", "-t", "gate-app", "--older-than", "1h", "--except", "desk"})
	})
	if !strings.Contains(out, "reaped session boss") || strings.Contains(out, "reaped session desk") {
		t.Fatalf("after disable:\n%s", out)
	}
}

func gatePath(t *testing.T) string {
	p, _ := gateEnv(t)
	return p
}
