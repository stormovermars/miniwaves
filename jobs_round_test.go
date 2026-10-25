package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A board with jobs loads and saves byte for byte, and an mw that predates jobs still reads it
// (unknown index fields are ignored by its decoder; the old fields keep their meaning).
func TestJobsRoundTripAndOldReader(t *testing.T) {
	path := jobsBoard(t)
	runAll(t,
		[]string{"jobs", "enable", "tiny", "-t", "sheet"},
		[]string{"jobs", "set", "tiny", "title=Tiny", "news.out=docs/n/"},
		[]string{"add", "task", "x", "-t", "sheet"},
	)
	before, _ := os.ReadFile(path)
	tenantBefore, _ := os.ReadFile(tenantPath(path, "sheet"))
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	w.indexBase, w.base = "", map[string]string{} // force the writer to write everything it would
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	tenantAfter, _ := os.ReadFile(tenantPath(path, "sheet"))
	if string(before) != string(after) || string(tenantBefore) != string(tenantAfter) {
		t.Fatalf("round trip changed the board:\n%s\n%s", before, after)
	}
	var old struct {
		Version int `json:"version"`
		Name    string
		Tenants []struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		} `json:"tenants"`
	}
	if err := json.Unmarshal(after, &old); err != nil || old.Version != 9 || len(old.Tenants) != 3 {
		t.Fatalf("old reader: %v %+v", err, old)
	}
	// a board that never opts in has no jobs or job keys at all
	plain := inTempDir(t)
	runAll(t, []string{"init", "Plain"})
	b, _ := os.ReadFile(plain)
	if strings.Contains(string(b), "job") {
		t.Fatalf("plain board gained job keys: %s", b)
	}
}

func TestGoldenJobsShow(t *testing.T) {
	gpath := jobsBoard(t)
	realRoot, _ := filepath.EvalSymlinks(filepath.Dir(gpath))
	esc, _ := json.Marshal(realRoot)
	norm := func(s string) string { // news_path is absolute: pin the temp root and the separator
		s = strings.ReplaceAll(s, strings.Trim(string(esc), `"`), "<root>")
		s = strings.ReplaceAll(s, realRoot, "<root>")
		return strings.ReplaceAll(strings.ReplaceAll(s, `\\`, "/"), `\`, "/")
	}
	runAll(t,
		[]string{"jobs", "enable", "tiny", "-t", "sheet,doc"},
		[]string{"jobs", "set", "tiny", "title=Tiny corpus", "max_workers=2", "news.url=https://example.com/news"},
		[]string{"jobs", "enable", "other", "-t", "jobs-app"},
	)
	got := captureOutput(t, func() { runAll(t, []string{"jobs", "show", "-j"}) })
	checkGolden(t, "jobs-show-j", norm(got))
	// TOON quotes a value with a colon (a Windows drive path): unquote the pinned root so the golden is the same on every OS
	unq := regexp.MustCompile(`"(<root>[^"]*)"`)
	toon := unq.ReplaceAllString(norm(captureOutput(t, func() { runAll(t, []string{"jobs", "show", "tiny"}) })), "$1")
	checkGolden(t, "jobs-show-toon", toon)
}
