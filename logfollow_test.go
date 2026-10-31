package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestAlertExecHelper is the stand-in for the user's alerts.exec program (the MW_ALERT_EXEC stub): it appends the
// event file named by its last argument to ALERT_HELPER_OUT, plus the argument count. Inert unless ALERT_HELPER_OUT is set.
func TestAlertExecHelper(t *testing.T) {
	out := os.Getenv("ALERT_HELPER_OUT")
	if out == "" {
		return
	}
	a := flag.Args() // ["ARGS", ..., eventfile]
	b, err := os.ReadFile(a[len(a)-1])
	if err != nil {
		os.Exit(7)
	}
	f, _ := os.OpenFile(out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	f.Write(append(bytes.TrimSpace(b), '\n'))
	f.Close()
	if st, err := os.Stdin.Stat(); err == nil && st.Mode()&os.ModeCharDevice == 0 && st.Size() > 0 {
		os.Exit(8) // stdin must be the null device
	}
}

// TestAlertSleepHelper is the grandchild of the hanging alerts.exec stub: it holds the inherited stderr open for a few seconds.
func TestAlertSleepHelper(t *testing.T) {
	if os.Getenv("ALERT_SLEEP_HELPER") == "" {
		return
	}
	time.Sleep(8 * time.Second)
}

// TestAlertHangHelper exits at once but leaves a child holding its stderr pipe: without WaitDelay, Run would wait for it.
func TestAlertHangHelper(t *testing.T) {
	if os.Getenv("ALERT_HANG_HELPER") == "" {
		return
	}
	c := exec.Command(os.Args[0], "-test.run=^TestAlertSleepHelper$")
	c.Env = append(os.Environ(), "ALERT_SLEEP_HELPER=1", "ALERT_HANG_HELPER=")
	c.Stderr = os.Stderr
	if err := c.Start(); err != nil {
		os.Exit(7)
	}
}

func TestLogFollowExecChildHoldingPipesDoesNotHangFeed(t *testing.T) {
	old := alertExecWaitDelay
	alertExecWaitDelay = 300 * time.Millisecond
	defer func() { alertExecWaitDelay = old }()
	t.Setenv("ALERT_HANG_HELPER", "1")
	var errb bytes.Buffer
	start := time.Now()
	done := make(chan struct{})
	go func() {
		runAlertExec([]string{os.Args[0], "-test.run=^TestAlertHangHelper$"}, logRow{Item: "TSK-001", Action: "block"}, &errb)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runAlertExec hung on a child that keeps the pipes open")
	}
	if d := time.Since(start); d > 4*time.Second {
		t.Fatalf("took %v", d)
	}
	if !strings.Contains(errb.String(), "alerts.exec") {
		t.Fatalf("no error reported for the abandoned pipes: %q", errb.String())
	}
}

func TestLogFollowTenantReappearsWithoutReplay(t *testing.T) {
	path := followBoard(t)
	runAll(t, []string{"tenant", "add", "other", "Other"})
	runAll(t, []string{"add", "task", "B", "-t", "other", "-S", "ready"})
	// follow both from now (fixed list), poll once to take cursors
	stop := make(chan struct{})
	var out, errb bytes.Buffer
	o := map[string]string{"all": "true"}
	done := make(chan error, 1)
	go func() { done <- logFollow(o, &syncW{w: &out}, &errb, 20*time.Millisecond, stop) }()
	time.Sleep(200 * time.Millisecond)
	// the tenant disappears: take it out of the board file, then put it back unchanged
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	full := append([]Tenant(nil), w.Tenants...)
	var gone Tenant
	kept := []Tenant{}
	for _, tn := range w.Tenants {
		if tn.Slug == "other" {
			gone = tn
		} else {
			kept = append(kept, tn)
		}
	}
	w.Tenants = kept
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	w, _ = load(path)
	w.Tenants = full
	_ = gone
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	close(stop)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if s := out.String(); s != "" {
		t.Fatalf("a tenant that came back replayed its history:\n%s", s)
	}
}

func TestLogFollowFirstReadRetriesMidWrite(t *testing.T) {
	path := followBoard(t)
	old := followStartBackoff
	followStartBackoff = 30 * time.Millisecond
	defer func() { followStartBackoff = old }()
	good, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// a half-written board, completed shortly after: the start retries, then follows from now
	if err := os.WriteFile(path, good[:len(good)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var out, errb bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- logFollow(map[string]string{}, &syncW{w: &out}, &errb, 20*time.Millisecond, stop) }()
	time.Sleep(80 * time.Millisecond)
	if err := os.WriteFile(path, good, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		t.Fatalf("feed ended instead of retrying: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	runAll(t, []string{"note", "TSK-001", "after the retry"})
	time.Sleep(300 * time.Millisecond)
	close(stop)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if s := out.String(); !strings.Contains(s, "note") || strings.Contains(s, "created") {
		t.Fatalf("want only the new event, got:\n%s", s)
	}
	// a board that stays broken still fails after the attempts
	if err := os.WriteFile(path, good[:len(good)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	open := make(chan struct{})
	defer close(open)
	if err := logFollow(map[string]string{}, &syncW{w: &out}, &errb, 20*time.Millisecond, open); err == nil {
		t.Fatal("a permanently broken board must still fail")
	}
}

// syncW serialises writes from the feed goroutine.
type syncW struct {
	mu sync.Mutex
	w  *bytes.Buffer
}

func (s *syncW) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

func alertStub(t *testing.T, extra ...string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "events.jsonl")
	t.Setenv("ALERT_HELPER_OUT", out)
	argv := append([]string{os.Args[0], "-test.run=^TestAlertExecHelper$", "ARGS"}, extra...)
	b, _ := json.Marshal(argv)
	t.Setenv("MW_ALERT_EXEC", string(b))
	return out
}

func followBoard(t *testing.T) string {
	t.Helper()
	path := inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t,
		[]string{"init", "Follow app"},
		[]string{"add", "task", "A", "-S", "ready"},
		[]string{"start", "TSK-001"},
		[]string{"block", "TSK-001", "waiting; touch pwned $(id)"},
	)
	return path
}

// once runs one poll of the feed (stop is already closed) and returns stdout and stderr.
func once(t *testing.T, o map[string]string) (string, string, error) {
	t.Helper()
	stop := make(chan struct{})
	close(stop)
	var out, errb bytes.Buffer
	err := logFollow(o, &out, &errb, time.Millisecond, stop)
	return out.String(), errb.String(), err
}

func TestLogFollowExecRunsForSelectedEventsOnly(t *testing.T) {
	followBoard(t)
	runAll(t, []string{"user", "set", "alerts.on=block,done"})
	out := alertStub(t)
	stdout, stderr, err := once(t, map[string]string{"since": "start", "exec": "true"})
	if err != nil {
		t.Fatal(err, stderr)
	}
	if !strings.Contains(stdout, "created") || !strings.Contains(stdout, "block") {
		t.Fatalf("feed:\n%s", stdout)
	}
	b, _ := os.ReadFile(out)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 1 {
		t.Fatalf("want one run (the block event), got %d: %s (stderr %s)", len(lines), b, stderr)
	}
	var ev logRow
	if err := json.Unmarshal([]byte(lines[0]), &ev); err != nil || ev.Action != "block" || ev.Item != "TSK-001" || !strings.Contains(ev.Note, "touch pwned $(id)") {
		t.Fatalf("event file: %v %+v", err, ev)
	}
	// the note is data: no shell saw it
	if _, err := os.Stat("pwned"); err == nil {
		t.Fatal("the note reached a shell")
	}
	// the temp files are gone
	if m, _ := filepath.Glob(filepath.Join(os.TempDir(), "mw-alert-*.json")); len(m) != 0 {
		t.Fatalf("event files left behind: %v", m)
	}
}

func TestLogFollowStartsFromNowAndJSONL(t *testing.T) {
	followBoard(t)
	runAll(t, []string{"user", "set", "alerts.on=block"})
	out := alertStub(t)
	stdout, stderr, err := once(t, map[string]string{"exec": "true"})
	if err != nil || stdout != "" {
		t.Fatalf("from now: %q %q %v", stdout, stderr, err)
	}
	if b, _ := os.ReadFile(out); len(b) != 0 {
		t.Fatalf("history ran the command: %s", b)
	}
	stdout, _, err = once(t, map[string]string{"since": "start", "json": "true"})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, l := range strings.Split(strings.TrimSpace(stdout), "\n") {
		var r logRow
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatalf("not one JSON object per line: %q", l)
		}
		n++
	}
	if n < 3 {
		t.Fatalf("want the 3+ board events, got %d", n)
	}
}

func TestLogFollowExecConfig(t *testing.T) {
	followBoard(t)
	t.Setenv("MW_ALERT_EXEC", "")
	if _, _, err := once(t, map[string]string{"exec": "true"}); err == nil || !strings.Contains(err.Error(), "alerts.exec") {
		t.Fatalf("no alerts.exec: %v", err)
	}
	// the user config's argv is used when the stub is unset; an empty alerts.on runs nothing and says so
	argv, _ := json.Marshal([]string{os.Args[0], "-test.run=^TestAlertExecHelper$", "ARGS"})
	runAll(t, []string{"user", "set", "alerts.exec=" + string(argv)})
	out := filepath.Join(t.TempDir(), "o.jsonl")
	t.Setenv("ALERT_HELPER_OUT", out)
	_, stderr, err := once(t, map[string]string{"since": "start", "exec": "true"})
	if err != nil || !strings.Contains(stderr, "alerts.on is empty") {
		t.Fatalf("%v %q", err, stderr)
	}
	if b, _ := os.ReadFile(out); len(b) != 0 {
		t.Fatalf("ran with an empty alerts.on: %s", b)
	}
	runAll(t, []string{"user", "set", "alerts.on=block"})
	if _, _, err := once(t, map[string]string{"since": "start", "exec": "true"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); !strings.Contains(string(b), `"action":"block"`) {
		t.Fatalf("user config argv not run: %s", b)
	}
	// option checks
	if err := run([]string{"log", "--exec"}); err == nil || !strings.Contains(err.Error(), "--follow") {
		t.Fatalf("--exec alone: %v", err)
	}
	if _, _, err := once(t, map[string]string{"limit": "3"}); err == nil {
		t.Fatal("-n with --follow accepted")
	}
}

// A real follow: events made while the feed polls reach the command once each.
func TestLogFollowLive(t *testing.T) {
	followBoard(t)
	runAll(t, []string{"user", "set", "alerts.on=done"})
	out := alertStub(t)
	stop := make(chan struct{})
	var buf syncBuf
	done := make(chan error, 1)
	loop, err := startFollow(map[string]string{"exec": "true"}, &buf, &buf, 20*time.Millisecond, stop)
	if err != nil {
		t.Fatal(err)
	}
	go func() { done <- loop() }()
	time.Sleep(200 * time.Millisecond)
	runAll(t, []string{"done", "TSK-001", "ok", "--no-evidence", "x"})
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if b, _ := os.ReadFile(out); strings.Contains(string(b), `"action":"done"`) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(150 * time.Millisecond) // a second poll must not deliver it again
	close(stop)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	if c := strings.Count(string(b), `"action":"done"`); c != 1 {
		t.Fatalf("done delivered %d times: %s\nfeed: %s", c, b, buf.String())
	}
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }
