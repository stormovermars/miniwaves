package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// mwRun runs mainExit and returns exit code, stdout and stderr.
func mwRun(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var errb bytes.Buffer
	code := 0
	stdout := captureOutput(t, func() { code = mainExit(args, &errb) })
	return code, stdout, errb.String()
}

// TSK-081: -j errors carry a stable "code"; exit codes, stderr text and the existing JSON keys are
// exactly what 0.3.14 produced (wr depends on them), so every case pins all of them.
func TestErrorCodesContract(t *testing.T) {
	gateEnv(t)
	runAll(t, []string{"add", "task", "A", "-S", "ready"}, []string{"add", "task", "B", "-S", "ready"},
		[]string{"start", "TSK-001", "-s", "w1"}, []string{"session", "start", "dup"})
	cases := []struct {
		args   []string
		exit   int
		stderr string
		code   string
	}{
		{[]string{"start", "TSK-001", "-s", "w2"}, 1, "mw: TSK-001 is owned by session w1\n", "TASK_CLAIMED"},
		{[]string{"show", "TSK-099"}, 1, "mw: item TSK-099 not found\n", "NOT_FOUND"},
		{[]string{"list", "--bogus"}, 1, "mw: unknown option --bogus\n", "USAGE"},
		{[]string{"frobnicate"}, 1, "mw: unknown command \"frobnicate\" (try mw help)\n", "USAGE"},
		{[]string{"done", "TSK-001", "ok", "-s", "w1"}, 1, "", "GATE_REFUSED"},
		{[]string{"gate", "evidence", "off", "-s", "w1"}, 1, "mw: mw gate is for humans: unset MW_SESSION and drop -s\n", "HUMAN_ONLY"},
		{[]string{"get-prompt", "session", "dup"}, 3, "mw: session dup already exists\n", "SESSION_EXISTS"},
	}
	for _, c := range cases {
		// without -j: stdout stays empty (except nothing), stderr and exit are the contract
		code, so, se := mwRun(t, c.args...)
		if code != c.exit || so != "" {
			t.Fatalf("%v: exit %d stdout %q", c.args, code, so)
		}
		if c.stderr != "" && se != c.stderr {
			t.Fatalf("%v: stderr %q want %q", c.args, se, c.stderr)
		}
		jcode, jso, jse := mwRun(t, append(append([]string{}, c.args...), "-j")...)
		if jcode != c.exit || jse != se {
			t.Fatalf("%v -j: exit %d/%d stderr %q vs %q", c.args, jcode, c.exit, jse, se)
		}
		if c.code == "SESSION_EXISTS" { // the 0.3.14 object, byte for byte, no code field
			if jso != "{\"error\":\"session_exists\",\"session\":\"dup\"}\n" {
				t.Fatalf("session_exists object changed: %q", jso)
			}
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(jso), &m); err != nil {
			t.Fatalf("%v -j: stdout not JSON: %q", c.args, jso)
		}
		if m["code"] != c.code {
			t.Fatalf("%v -j: code %v want %s (%s)", c.args, m["code"], c.code, jso)
		}
		if msg, _ := m["error"].(string); msg == "" || !strings.HasPrefix(se, "mw: "+msg) {
			t.Fatalf("%v -j: error %q does not match stderr %q", c.args, m["error"], se)
		}
	}
	// "nothing ready" stays a plain failure; -j stays null with exit 0
	runAll(t, []string{"start", "TSK-002", "-s", "w9"})
	if code, so, se := mwRun(t, "next"); code != 1 || so != "" || se != "mw: nothing ready\n" {
		t.Fatalf("next: %d %q %q", code, so, se)
	}
	if code, so, se := mwRun(t, "next", "-j"); code != 0 || strings.TrimSpace(so) != "null" || se != "" {
		t.Fatalf("next -j: %d %q %q", code, so, se)
	}
	// success output is untouched by the code field
	if code, so, _ := mwRun(t, "show", "TSK-001", "-j"); code != 0 || strings.Contains(so, `"code"`) {
		t.Fatalf("show -j: %d %s", code, so)
	}
}

func TestErrCodeMapping(t *testing.T) {
	for _, c := range []struct {
		err  error
		want string
	}{
		{lockTimeoutError("1", 0), "LOCKED"},
		{coded("WIP_LIMIT", errTest("x")), "WIP_LIMIT"},
		{errTest("something odd"), "ERROR"},
	} {
		if got := errCode(c.err); got != c.want {
			t.Fatalf("%v: %s want %s", c.err, got, c.want)
		}
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
