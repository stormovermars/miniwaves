package main

import (
	"strings"
	"testing"
)

func helpOf(t *testing.T, args ...string) string {
	t.Helper()
	return captureOutput(t, func() { runAll(t, append([]string{"help"}, args...)) })
}

func TestHelpHygiene(t *testing.T) {
	inTempDir(t)
	top := helpOf(t)
	if !strings.Contains(top, "mw fanfare ") {
		t.Errorf("the top help screen lacks fanfare:\n%s", top)
	}
	for _, topic := range []string{"", "jobs", "status", "user", "configure", "board", "log", "fanfare", "block", "news"} {
		out := helpOf(t, topic)
		if topic == "" {
			out = top
		}
		low := strings.ToLower(out)
		if strings.Contains(low, "pane") || strings.Contains(low, "alerts.kinds") || strings.Contains(low, "alerts.popup") || strings.Contains(low, "alerts.enabled") || strings.Contains(low, "alerts.sound") || strings.Contains(low, "alerts.message") {
			t.Errorf("help %q still mentions panes or dead alerts keys:\n%s", topic, out)
		}
	}
	// alerts.on and alerts.exec are still used by mw log --follow --exec
	if out := helpOf(t, "log"); !strings.Contains(out, "alerts.exec") || !strings.Contains(out, "alerts.on") {
		t.Errorf("help log must keep alerts.on and alerts.exec")
	}
}

func TestFanfareOnOff(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "F"})
	captureOutput(t, func() { runAll(t, []string{"fanfare", "off"}) })
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c := fanfareCfg(w); c.Enabled == nil || *c.Enabled {
		t.Fatalf("fanfare off: %+v", c.Enabled)
	}
	captureOutput(t, func() { runAll(t, []string{"fanfare", "on"}) })
	w, _ = load(path)
	if c := fanfareCfg(w); c.Enabled == nil || !*c.Enabled {
		t.Fatalf("fanfare on: %+v", c.Enabled)
	}
	wantErr(t, "usage: mw fanfare on", "fanfare", "on", "extra")
}
