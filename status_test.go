package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestFormatStatusLine(t *testing.T) {
	for _, c := range []struct {
		in   newsCounts
		want string
	}{
		{newsCounts{Needs: 2, Review: 1, Blocked: 1, Done: 28, Total: 40}, "mw:N2R1B1:12/40"},
		{newsCounts{Review: 1, Done: 3, Total: 4}, "mw:R1:1/4"},
		{newsCounts{Blocked: 3, Total: 3}, "mw:B3:3/3"},
		{newsCounts{Done: 28, Total: 40}, "mw:12/40"},
		{newsCounts{}, "mw:0/0"},
	} {
		if got := formatStatusLine(c.in); got != c.want {
			t.Errorf("%+v: %q want %q", c.in, got, c.want)
		}
	}
}

func TestStatusLineCommand(t *testing.T) {
	userCfgPath(t)
	path := newsBoard(t)
	root := filepath.Dir(path)
	sub := filepath.Join(root, "deep", "er")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	line := statusLine(sub, false) // walks up to the board
	if !strings.HasPrefix(line, "mw:") || !strings.Contains(line, "N1") || !strings.Contains(line, "/") {
		t.Fatalf("%q", line)
	}
	// stdin cwd: both forms point at the board, from a process cwd that is not one
	other := t.TempDir()
	old := mustGetwd()
	if err := os.Chdir(other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
	if statusLine(mustGetwd(), false) != "" {
		t.Fatal("no board must print nothing")
	}
	for _, js := range []string{`{"workspace":{"current_dir":"` + sub + `"},"cwd":"/nonexistent"}`, `{"cwd":"` + sub + `"}`} {
		if got := statusLine(statusCwd(strings.NewReader(js)), false); got != line {
			t.Errorf("stdin %s: %q want %q", js, got, line)
		}
	}
	if statusCwd(strings.NewReader("not json")) != mustGetwd() || statusCwd(nil) != mustGetwd() {
		t.Fatal("fallback cwd")
	}
	// status.show=false silences it
	runAll(t, []string{"user", "set", "status.show=false"})
	if got := statusLine(sub, false); got != "" {
		t.Fatalf("status.show=false printed %q", got)
	}
	// the command never fails and writes nothing
	if err := run([]string{"status", "--line"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"status"}); err == nil {
		t.Fatal("--line is required")
	}
}

func TestStatusLineLink(t *testing.T) {
	userCfgPath(t)
	path := newsBoard(t)
	root := filepath.Dir(path)
	plain := statusLine(root, false)
	file := statusLine(root, true)
	md := filepath.Join(root, "docs", "news", "tiny", "jobs.md")
	_ = md
	if !strings.HasPrefix(file, "\x1b]8;;file://") || !strings.HasSuffix(file, "jobs.md\x1b\\"+plain+"\x1b]8;;\x1b\\") {
		t.Fatalf("file link: %q", file)
	}
	if strings.Contains(plain, "\x1b") {
		t.Fatalf("plain line has escapes: %q", plain)
	}
	// the artifact url wins when jobs.artifact-url exists next to jobs.md
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "docs", "news")
	if len(w.Jobs) > 0 {
		var ids []string
		for id := range w.Jobs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		dir = filepath.Join(root, filepath.FromSlash(jobView(w, ids[0]).NewsOut))
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "jobs.artifact-url"), []byte("https://claude.ai/artifact/abc\n"), 0644); err != nil {
		t.Fatal(err)
	}
	want := "\x1b]8;;https://claude.ai/artifact/abc\x1b\\" + plain + "\x1b]8;;\x1b\\"
	if got := statusLine(root, true); got != want {
		t.Fatalf("artifact link: %q want %q", got, want)
	}
}

func TestStatusExplain(t *testing.T) {
	userCfgPath(t)
	path := newsBoard(t)
	root := filepath.Dir(path)
	plug := filepath.Join(t.TempDir(), "p")
	for _, p := range []string{"alpha", "beta"} {
		_ = os.MkdirAll(filepath.Join(plug, p, ".claude-plugin"), 0755)
		_ = os.MkdirAll(filepath.Join(plug, p, "commands"), 0755)
		_ = os.WriteFile(filepath.Join(plug, p, ".claude-plugin", "plugin.json"), []byte(`{"name":"`+p+`","version":"9.1"}`), 0644)
		_ = os.WriteFile(filepath.Join(plug, p, "commands", "x-"+p+".md"), []byte("---\ndescription: does "+p+"\n---\nbody\n"), 0644)
	}
	out := statusExplain(root, plug)
	for _, want := range []string{"mw " + version + " (schema 9) · plugins alpha 9.1, beta 9.1", "N = needs you: 1", "\nNeeds you\n- doc/TSK-001 Doc work", "- status.show = true (default;", "- /x-alpha: does alpha", "- /x-beta: does beta"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
	// no board, no plugins folder: still a version line and settings
	out = statusExplain(t.TempDir(), "")
	if !strings.HasPrefix(out, "mw "+version) || !strings.Contains(out, "No mw board here") || strings.Contains(out, "\nCommands") {
		t.Errorf("no board:\n%s", out)
	}
	wantErr(t, "unknown option", "status", "--explain", "--bogus")
}
