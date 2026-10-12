package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestReopenArchivedRefused(t *testing.T) {
	_ = inTempDir(t)
	runAll(t, []string{"init", "App"}, []string{"add", "task", "old", "-S", "ready"}, []string{"start", "TSK-001"},
		[]string{"done", "TSK-001", "ok", "--no-evidence", "t"}, []string{"archive", "--days", "0"})
	err := run([]string{"reopen", "TSK-001"})
	if err == nil || !strings.Contains(err.Error(), "TSK-001 is archived (no unarchive)") {
		t.Fatalf("got %v", err)
	}
}

func TestLockPidSidecar(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "App"})
	old := lockWait
	lockWait = 50 * 1e6
	defer func() { lockWait = old }()
	unlock, err := lockSet(path, false, []string{"app"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = lockSet(path, false, []string{"app"})
	unlock()
	if err == nil || strings.Contains(err.Error(), "pid ?") {
		t.Errorf("want a real pid, got %v", err)
	}
}

func TestClaimPromptAndVersionTenants(t *testing.T) {
	contractBoard(t)
	runAll(t, []string{"session", "start", "bot", "-a", "builder"})
	out := captureOutput(t, func() { runAll(t, []string{"next", "--claim", "-j", "--prompt", "-s", "bot"}) })
	if !strings.Contains(out, `"prompt": "`) || !strings.Contains(out, "bot") {
		t.Errorf("no prompt in %s", out)
	}
	if v := captureOutput(t, func() { runAll(t, []string{"version", "-j"}) }); !strings.Contains(v, `"slug": "contract-app"`) || !strings.Contains(v, `"dir": "`) {
		t.Errorf("version -j lacks tenants: %s", v)
	}
	if err := run([]string{"next", "--prompt"}); err == nil {
		t.Error("--prompt without --claim accepted")
	}
}

func TestPublishedPageMode(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "App"})
	out := filepath.Join(filepath.Dir(path), "pub.html")
	if err := writeHTML(path, &Workspace{Name: "x"}, out); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(out); st.Mode().Perm() != 0644 {
		t.Errorf("publish mode %v", st.Mode().Perm())
	}
}

func TestPublishThrottleAndHash(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "App"})
	target := filepath.Join(filepath.Dir(path), "site", "index.html")
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	w.Publish = &PublishConfig{HTML: target, Auto: true, Interval: 1} // clamps to 5 s
	if publishInterval(w.Publish) != 5*time.Second || publishInterval(&PublishConfig{}) != 60*time.Second {
		t.Fatal("interval clamp/default")
	}
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	mt := func() time.Time { st, _ := os.Stat(target); return st.ModTime() }
	runAll(t, []string{"add", "task", "one", "-S", "ready"}) // first write: no sidecar yet
	first := mt()
	for _, n := range []string{"two", "three", "four"} { // inside the interval: no more page writes
		runAll(t, []string{"add", "task", n, "-S", "ready"})
	}
	if !mt().Equal(first) {
		t.Fatal("writes within the interval rewrote the page")
	}
	// the interval ends and the board changed: the next write publishes
	age := func(d time.Duration) {
		b, _ := os.ReadFile(target + ".hash")
		f := strings.Fields(string(b))
		at := strconv.FormatInt(time.Now().Add(-d).Unix(), 10)
		rest := strings.Join(f[1:], "\n")
		os.WriteFile(target+".hash", []byte(at+"\n"+rest+"\n"), 0644)
	}
	if b, _ := os.ReadFile(target + ".hash"); !strings.Contains(string(b), "pending") {
		t.Fatalf("a skipped change must leave a pending marker: %q", b)
	}
	// the interval ends and nothing writes: the next invocation, even a read, publishes the last change
	age(10 * time.Second)
	time.Sleep(1100 * time.Millisecond)
	runAll(t, []string{"list"})
	if !mt().After(first) {
		t.Fatal("a read after the interval did not publish the pending change")
	}
	if b, _ := os.ReadFile(target); !strings.Contains(string(b), "four") {
		t.Fatal("page is stale after the interval")
	}
	if b, _ := os.ReadFile(target + ".hash"); strings.Contains(string(b), "pending") {
		t.Fatal("pending marker not cleared")
	}
	first = mt()
	age(10 * time.Second)
	time.Sleep(1100 * time.Millisecond)
	runAll(t, []string{"add", "task", "five", "-S", "ready"})
	if !mt().After(first) {
		t.Fatal("a write after the interval did not publish")
	}
	// unchanged board: nothing is written even after the interval
	second := mt()
	age(10 * time.Second)
	time.Sleep(1100 * time.Millisecond)
	runAll(t, []string{"gate", "evidence", "off", "--by", "t"}) // a write that leaves the page identical
	if !mt().Equal(second) {
		t.Fatal("an unchanged page was rewritten")
	}
	// explicit board --html always writes
	time.Sleep(1100 * time.Millisecond)
	runAll(t, []string{"board", "--html"})
	if !mt().After(second) {
		t.Fatal("explicit mw board --html did not write")
	}
}
