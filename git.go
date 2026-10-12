package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// PublishConfig is the optional static page (.miniwaves.json "publish").
type PublishConfig struct {
	HTML string `json:"html,omitempty"`
	Auto bool   `json:"auto,omitempty"`
	// Interval is seconds between page writes (default 60, minimum 5; lower values clamp to 5).
	Interval int `json:"interval,omitempty"`
}

var gitTimeout = 10 * time.Second

// per-run write tracking, reset by run()
var (
	savedCount int
	lastSaved  *Workspace
)

// afterWrite runs once per successful command that wrote the board: it
// regenerates the published page. Nothing here ever fails the command.
func afterWrite(path string) {
	w := lastSaved
	if w == nil || w.Publish == nil || !w.Publish.Auto || w.Publish.HTML == "" {
		return
	}
	if full, err := load(path); err == nil { // a command may have loaded one tenant only; the page shows all
		w = full
	}
	if _, err := publishHTML(path, w, w.Publish.HTML, false); err != nil {
		fmt.Fprintln(os.Stderr, "mw: publish:", err)
	}
}

// lockArtifacts are the files mw creates next to the board that git must never see.
var lockArtifacts = []string{".miniwaves.json.*.tmp", ".miniwaves/*.lock*", ".miniwaves/*.tmp"}

// ensureGitExclude adds the lock artifacts to .git/info/exclude (idempotent; no-op outside a git
// repo). Only mw init calls it, so an agent's commits never include them.
func ensureGitExclude(dir string) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-path", "info/exclude")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	have := map[string]bool{}
	if b, err := os.ReadFile(p); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			have[strings.TrimSpace(l)] = true
		}
	}
	var add []string
	for _, l := range lockArtifacts {
		if !have[l] {
			add = append(add, l)
		}
	}
	if len(add) == 0 {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	if st, _ := f.Stat(); st != nil && st.Size() > 0 {
		if b, _ := os.ReadFile(p); len(b) > 0 && b[len(b)-1] != '\n' {
			_, _ = f.WriteString("\n")
		}
	}
	_, _ = f.WriteString(strings.Join(add, "\n") + "\n")
}
