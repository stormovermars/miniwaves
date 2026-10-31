package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TSK-082: the opt-in user key wip.session=true refuses a start by a session that already has a doing item.
func TestWipSessionLimit(t *testing.T) {
	gateEnv(t)
	t.Setenv("MW_USER_CONFIG", filepath.Join(t.TempDir(), "mw-user.json"))
	runAll(t, []string{"add", "task", "A", "-S", "ready"}, []string{"add", "task", "B", "-S", "ready"}, []string{"add", "task", "C", "-S", "ready"}, []string{"add", "task", "D", "-S", "ready"})

	// off (default): the implicit local session may own several items, as before
	runAll(t, []string{"start", "TSK-001"}, []string{"start", "TSK-002"}, []string{"block", "TSK-002", "x"}, []string{"block", "TSK-001", "x"},
		[]string{"ready", "TSK-001"}, []string{"ready", "TSK-002"})

	runAll(t, []string{"configure", "wip.session=true"})
	runAll(t, []string{"start", "TSK-001"})
	err := run([]string{"start", "TSK-002"})
	if err == nil || errCode(err) != "WIP_LIMIT" || !strings.Contains(err.Error(), "TSK-001") {
		t.Fatalf("second start by local: %v (%s)", err, errCode(err))
	}
	code, so, se := mwRun(t, "start", "TSK-002", "-j")
	if code != 1 || !strings.Contains(so, `"code":"WIP_LIMIT"`) || !strings.HasPrefix(se, "mw: ") {
		t.Fatalf("-j: %d %q %q", code, so, se)
	}
	// restarting the item it already has is not a second item; finishing frees the slot
	runAll(t, []string{"start", "TSK-001"}, []string{"done", "TSK-001", "ok", "--no-evidence", "x"}, []string{"start", "TSK-002"})
	// a named session is held to it too, through next --claim
	runAll(t, []string{"next", "--claim", "-s", "w1", "-j"})
	if code, so, _ := mwRun(t, "next", "--claim", "-s", "w1", "-j"); code != 1 || !strings.Contains(so, `"code":"WIP_LIMIT"`) {
		t.Fatalf("named: %d %q", code, so)
	}
	// off again: unchanged
	runAll(t, []string{"configure", "wip.session=false"}, []string{"start", "TSK-004"})
}
