package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// mw status --line: one short line for the Claude Code status bar, "mw:N2R1B1:12/40". It is read-only,
// never fails (exit 0, silent on any problem) and prints nothing outside a board or when status.show is off.

// statusCwd picks the working folder: the cwd (or workspace.current_dir) of the status-line JSON on stdin,
// else the process cwd.
func statusCwd(stdin io.Reader) string {
	if stdin != nil {
		if b, err := io.ReadAll(io.LimitReader(stdin, 1<<20)); err == nil && len(strings.TrimSpace(string(b))) > 0 {
			var j struct {
				Cwd       string `json:"cwd"`
				Workspace struct {
					Dir string `json:"current_dir"`
				} `json:"workspace"`
			}
			if json.Unmarshal(b, &j) == nil {
				if j.Workspace.Dir != "" {
					return j.Workspace.Dir
				}
				if j.Cwd != "" {
					return j.Cwd
				}
			}
		}
	}
	return mustGetwd()
}

// statusFindBoard walks up from dir to the nearest board file.
func statusFindBoard(dir string) string {
	d, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		p := filepath.Join(d, dataFile)
		if _, err := statShared(p); err == nil {
			return p
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// statusLine is the line for the board nearest to cwd, "" when there is nothing to show.
func statusLine(cwd string, link bool) string {
	if c, _, _, err := readUserConfig(); err == nil {
		if v, _ := userValue(c, userKeyByName("status.show")); v == "false" {
			return ""
		}
	}
	path := statusFindBoard(cwd)
	if path == "" {
		return ""
	}
	w, err := load(path)
	if err != nil {
		return ""
	}
	m := buildNewsModel(w, newsPart{title: w.Name, tenants: newsJobTenants(w, "")}, nil, newsWindow{full: true})
	line := formatStatusLine(m.Now.Counts)
	if link {
		line = statusLink(line, statusTarget(path, w))
	}
	return line
}

// statusTarget is what a click on the line opens: the published artifact URL (jobs.artifact-url next to
// jobs.md) when that file exists, else the file:// URL of jobs.md. Pure path work; nothing is created.
func statusTarget(path string, w *Workspace) string {
	root := filepath.Dir(path)
	dir := "docs/news/"
	var ids []string
	for id := range w.Jobs {
		ids = append(ids, id)
	}
	if len(ids) > 0 {
		sort.Strings(ids)
		dir = jobView(w, ids[0]).NewsOut
	}
	md := filepath.Join(root, filepath.FromSlash(dir), "jobs.md")
	if b, err := os.ReadFile(filepath.Join(filepath.Dir(md), "jobs.artifact-url")); err == nil {
		if u := strings.TrimSpace(string(b)); u != "" && !strings.ContainsAny(u, "\x00\x1b\x07\n\r") {
			return u
		}
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(md)}).String()
}

// statusLink wraps text in an OSC 8 hyperlink.
func statusLink(text, target string) string {
	return "\x1b]8;;" + target + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

func formatStatusLine(c newsCounts) string {
	var l strings.Builder
	for _, f := range []struct {
		k string
		n int
	}{{"N", c.Needs}, {"R", c.Review}, {"B", c.Blocked}} {
		if f.n > 0 {
			fmt.Fprintf(&l, "%s%d", f.k, f.n)
		}
	}
	if l.Len() > 0 {
		l.WriteString(":")
	}
	return fmt.Sprintf("mw:%s%d/%d", l.String(), c.Total-c.Done, c.Total)
}

func statusCommand(args []string) error {
	if !hasFlag(args, "--line") {
		return fmt.Errorf("usage: mw status --line [--link] (see mw help status)")
	}
	var in io.Reader
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice == 0 {
		in = os.Stdin
	}
	done := make(chan string, 1)
	go func() { done <- statusLine(statusCwd(in), hasFlag(args, "--link")) }()
	select {
	case s := <-done:
		if s != "" {
			fmt.Println(s)
		}
	case <-time.After(2 * time.Second):
	}
	return nil
}
