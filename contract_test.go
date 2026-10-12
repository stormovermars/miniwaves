package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var goldenDir, _ = filepath.Abs(filepath.Join("testdata", "golden"))

var rfcRe = regexp.MustCompile(`\d{4}-\d\d-\d\dT[\d:.]+Z`)

// contractBoard is a small fixed board in a temp dir (HOME is temp too, so
// the evidence/builds/scratch folders live there).
func contractBoard(t *testing.T) (path, home string, norm func(string) string) {
	path = inTempDir(t)
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t,
		[]string{"init", "Contract app"},
		[]string{"dirs", "set", "evidence", filepath.Join(home, "ev")},
		[]string{"dirs", "set", "builds", filepath.Join(home, "bl")},
		[]string{"dirs", "set", "scratch", filepath.Join(home, "sc")},
		[]string{"gate", "evidence", "off", "--by", "t"},
		[]string{"add", "task", "First", "-c", "works", "-S", "ready"},
		[]string{"add", "bug", "Second", "-S", "ready"},
	)
	dir := filepath.Dir(path)
	norm = func(s string) string {
		for _, p := range [][2]string{{dir, "<DIR>"}, {home, "<HOME>"}} {
			r, _ := filepath.EvalSymlinks(p[0])
			s = replacePath(s, p[0], r, p[1])
		}
		s = agoRe.ReplaceAllString(osNorm(s), "last: now")
		s = rfcRe.ReplaceAllString(s, "<TS>")
		return tsRe.ReplaceAllString(s, "<TS>")
	}
	return
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join(goldenDir, name+".txt")
	if os.Getenv("GOLDEN_UPDATE") == "1" {
		if err := os.WriteFile(p, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Errorf("golden %s changed:\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}

func TestContractGoldens(t *testing.T) {
	_, _, norm := contractBoard(t)
	cap := func(args ...string) string {
		return norm(captureOutput(t, func() {
			if err := run(args); err != nil {
				var ee *exitError
				if !errors.As(err, &ee) {
					t.Fatalf("%v: %v", args, err)
				}
			}
		}))
	}
	checkGolden(t, "next-j", cap("next", "-j"))
	runAll(t, []string{"session", "start", "bot", "-a", "builder"})
	checkGolden(t, "next-claim-j", cap("next", "--claim", "-j", "-s", "bot"))
	checkGolden(t, "list-j-v", cap("list", "-j", "-v"))
	checkGolden(t, "list-s-A-j", cap("list", "-s", "bot", "-A", "-j"))
	checkGolden(t, "show-j", cap("show", "TSK-001", "-j"))
	checkGolden(t, "version-j", cap("version", "-j"))
	checkGolden(t, "tenant-list-j-v", cap("tenant", "list", "-j", "-v"))
	checkGolden(t, "get-prompt-j-error", cap("get-prompt", "session", "bot", "-j"))
	checkGolden(t, "log-j", cap("log", "-j"))
	checkGolden(t, "prompt-session-headless", cap("get-prompt", "session", "fresh", "--headless"))
	checkGolden(t, "prompt-session-interactive", cap("get-prompt", "session", "fresh"))
	runAll(t, []string{"session", "start", "bot2", "-a", "builder"})
	checkGolden(t, "next-claim-prompt-j", cap("next", "--claim", "-j", "--prompt", "-s", "bot2", "-a", "builder"))
	checkGolden(t, "gate-j", cap("gate", "-t", "contract-app", "-j"))
	checkGolden(t, "next-j-null", cap("next", "-j"))
	runAll(t, []string{"tenant", "tag", "contract-app", "+android", "+store"}, []string{"tenant", "add", "gone", "Gone app"},
		[]string{"tenant", "tag", "gone", "+old"}, []string{"tenant", "archive", "gone"})
	checkGolden(t, "tags-version-j", cap("version", "-j"))
	checkGolden(t, "tags-tenant-list-j-v", cap("tenant", "list", "-j", "-v"))
	checkGolden(t, "tags-board-html", cap("board", "--html"))
	checkGolden(t, "tags-board-html-archived", cap("board", "--html", "--archived"))
}
