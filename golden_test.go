package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// goldenBoard builds a fixed board whose outputs must not change while every
// gate is off (byte for byte with the pre-gate binary).
var (
	tsRe  = regexp.MustCompile(`\d\d-\d\d \d\d:\d\d`)
	agoRe = regexp.MustCompile(`last: (now|\d+m ago)`)
)

func goldenOutputs(t *testing.T, migrated bool) map[string]string {
	path := inTempDir(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t,
		[]string{"init", "Golden app"},
		[]string{"add", "prompt", "Make it fast", "--status", "ready"},
		[]string{"add", "story", "Instant startup", "-p", "PRM-001", "-c", "opens under 100ms"},
		[]string{"add", "task", "Build index", "-p", "STR-001", "-S", "ready"},
		[]string{"add", "bug", "Crash on empty", "-S", "ready"},
		[]string{"add", "chore", "Tidy", "-S", "doing"},
		[]string{"add", "spike", "Hidden inbox"},
		[]string{"wave", "create", "First milestone"},
		[]string{"session", "start", "golden", "-a", "builder"},
	)
	if migrated {
		// as if loaded from schema 5: workspace default off, no tenant overrides or latches
		w, err := load(path)
		if err != nil {
			t.Fatal(err)
		}
		w.Gates = map[string]string{"evidence": "off", "acceptance": "off", "judge": "off"}
		w.Tenants[0].Gates, w.Tenants[0].GateLatch = nil, nil
		if err := save(path, w); err != nil {
			t.Fatal(err)
		}
	} else {
		runAll(t,
			[]string{"gate", "evidence", "off", "--by", "golden"},
			[]string{"gate", "acceptance", "off", "--by", "golden"},
			[]string{"gate", "judge", "off", "--by", "golden"},
		)
	}
	dir := filepath.Dir(path)
	norm := func(s string) string {
		s = strings.ReplaceAll(s, dir, "<DIR>")
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			s = strings.ReplaceAll(s, r, "<DIR>")
		}
		s = strings.ReplaceAll(s, home, "<HOME>")
		s = tsRe.ReplaceAllString(s, "<TS>")
		return agoRe.ReplaceAllString(s, "last: now")
	}
	cap := func(args ...string) string {
		return norm(captureOutput(t, func() { runAll(t, args) }))
	}
	return map[string]string{
		"board":          cap("board"),
		"board-v":        cap("board", "-v"),
		"board-all":      cap("board", "-A"),
		"context":        cap("context", "-s", "golden"),
		"next":           cap("next"),
		"prompt-session": cap("get-prompt", "session", "golden2", "-a", "builder"),
		"prompt-disp":    cap("get-prompt", "dispatcher"),
		"prompt-coach":   cap("get-prompt", "coach"),
	}
}

func TestGoldenGatesOff(t *testing.T) {
	gdir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	got := goldenOutputs(t, false)
	for name, out := range got {
		p := filepath.Join(gdir, "golden-"+name+".txt")
		if os.Getenv("GOLDEN_UPDATE") == "1" {
			if err := os.WriteFile(p, []byte(out), 0644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != out {
			t.Errorf("golden %s changed:\n--- want\n%s\n--- got\n%s", name, want, out)
		}
	}
}

func TestGoldenMigratedBoardMatches(t *testing.T) {
	gdir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for name, out := range goldenOutputs(t, true) {
		want, err := os.ReadFile(filepath.Join(gdir, "golden-"+name+".txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != out {
			t.Errorf("migrated-board output %s differs:\n%s", name, out)
		}
	}
}
