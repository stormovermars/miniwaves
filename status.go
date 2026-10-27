package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// mw status --line: one short line for the Claude Code status bar, "mw:N2R1B1:12/40". It is read-only,
// never fails (exit 0, silent on any problem) and prints nothing outside a board (see statusBelow) or when status.show is off.

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
	if path := statusFindBoard(cwd); path != "" {
		w, err := load(path)
		if err != nil {
			return ""
		}
		line := formatStatusLine(statusCounts(w))
		if link {
			line = statusLink(line, statusTarget(path, w, cwd))
		}
		return line
	}
	return statusBelow(cwd, link)
}

func statusCounts(w *Workspace) newsCounts {
	return buildNewsModel(w, newsPart{title: w.Name, tenants: newsJobTenants(w, "")}, nil, newsWindow{full: true}).Now.Counts
}

const (
	statusFallbackDepth = 2
	statusCacheTTL      = 30 * time.Second
)

// statusCacheDir is where the multi-board result is cached (a var so tests can point it elsewhere).
var statusCacheDir = func() string {
	d, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "miniwaves")
}

// statusBelow is the line for cwd outside any board: the boards up to statusFallbackDepth levels below it, summed.
// The link goes only to a single board. The result is cached for statusCacheTTL (write failures are ignored).
func statusBelow(cwd string, link bool) string {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return ""
	}
	var cache string
	if d := statusCacheDir(); d != "" {
		sum := sha256.Sum256([]byte(abs + "\x00" + strconv.FormatBool(link)))
		cache = filepath.Join(d, "statusline-"+hex.EncodeToString(sum[:8])+".txt")
		if fi, err := os.Stat(cache); err == nil && time.Since(fi.ModTime()) < statusCacheTTL {
			if b, err := os.ReadFile(cache); err == nil {
				return string(b)
			}
		}
	}
	var total newsCounts
	n, one, oneW := 0, "", (*Workspace)(nil)
	for _, b := range discoverBoards(abs, statusFallbackDepth, false).Boards {
		if b.Error != "" {
			continue
		}
		w, err := load(b.Path)
		if err != nil {
			continue
		}
		c := statusCounts(w)
		total.Needs, total.Review, total.Blocked = total.Needs+c.Needs, total.Review+c.Review, total.Blocked+c.Blocked
		total.Work, total.Done = total.Work+c.Work, total.Done+c.Done
		n, one, oneW = n+1, b.Path, w
	}
	line := ""
	if n > 0 {
		line = formatStatusLine(total)
		if link && n == 1 {
			line = statusLink(line, statusTarget(one, oneW, cwd))
		}
	}
	if cache != "" && os.MkdirAll(filepath.Dir(cache), 0755) == nil {
		_ = os.WriteFile(cache, []byte(line), 0644)
	}
	return line
}

// statusTarget is what a click on the line opens: the published artifact URL (jobs.artifact-url next to
// jobs.md) when that file exists, else the file:// URL of jobs.md. Pure path work; nothing is created.
func statusTarget(path string, w *Workspace, cwd string) string {
	root := filepath.Dir(path)
	dir := "docs/news/"
	if id := statusJob(w, root, cwd); id != "" {
		dir = jobView(w, id).NewsOut
	}
	md := filepath.Join(root, filepath.FromSlash(dir), "jobs.md")
	if b, err := os.ReadFile(filepath.Join(filepath.Dir(md), "jobs.artifact-url")); err == nil {
		if u := strings.TrimSpace(string(b)); jobsMDValidURL(u) {
			return u
		}
	}
	return fileURL(md)
}

// statusJob is the job of the folder cwd: the job of the tenant whose path holds cwd (the longest match), else
// the first job by id, "" when the board has no jobs.
func statusJob(w *Workspace, root, cwd string) string {
	abs, _ := filepath.Abs(cwd)
	best, id := -1, ""
	for _, t := range w.Tenants {
		if t.Path == "" || t.Job == "" {
			continue
		}
		p := t.Path
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		p = filepath.Clean(p)
		if (abs == p || strings.HasPrefix(abs, p+string(os.PathSeparator))) && len(p) > best {
			best, id = len(p), t.Job
		}
	}
	if id != "" {
		return id
	}
	ids := make([]string, 0, len(w.Jobs))
	for j := range w.Jobs {
		ids = append(ids, j)
	}
	sort.Strings(ids)
	if len(ids) > 0 {
		return ids[0]
	}
	return ""
}

// fileURL is the file:// URL of an absolute path. A Windows volume gets a leading slash (file:///C:/...).
func fileURL(p string) string {
	s := strings.ReplaceAll(p, `\`, "/")
	if len(s) >= 2 && s[1] == ':' {
		s = "/" + s
	}
	return (&url.URL{Scheme: "file", Path: s}).String()
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
	return fmt.Sprintf("mw:%s%d/%d", l.String(), c.Work-c.Done, c.Work)
}

func statusCommand(args []string) error {
	if hasFlag(args, "--explain") {
		o, _, err := options(args)
		if err != nil {
			return err
		}
		if err := allowOptions(o, "explain", "plugins"); err != nil {
			return err
		}
		fmt.Print(statusExplain(mustGetwd(), o["plugins"]))
		return nil
	}
	if !hasFlag(args, "--line") {
		return fmt.Errorf("usage: mw status --line [--link] | mw status --explain [--plugins DIR] (see mw help status)")
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

// pluginInfo reads name and version of every plugin folder under dir (dir/*/.claude-plugin/plugin.json) and the
// description of each of its slash commands (dir/*/commands/*.md), sorted by name.
func pluginInfo(dir string) (vers []string, cmds []string) {
	plugins, _ := filepath.Glob(filepath.Join(dir, "*", ".claude-plugin", "plugin.json"))
	sort.Strings(plugins)
	for _, pj := range plugins {
		var j struct{ Name, Version string }
		if b, err := os.ReadFile(pj); err == nil && json.Unmarshal(b, &j) == nil && j.Name != "" {
			vers = append(vers, j.Name+" "+j.Version)
		}
		files, _ := filepath.Glob(filepath.Join(filepath.Dir(filepath.Dir(pj)), "commands", "*.md"))
		sort.Strings(files)
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			desc := ""
			for _, l := range strings.Split(string(b), "\n") {
				if v, ok := strings.CutPrefix(l, "description:"); ok {
					desc = strings.Trim(strings.TrimSpace(v), `"`)
					break
				}
			}
			cmds = append(cmds, "/"+strings.TrimSuffix(filepath.Base(f), ".md")+": "+desc)
		}
	}
	return vers, cmds
}

// statusExplain is `mw status --explain`: version line, the status line explained with live numbers, what needs
// Sir, the user settings, and the slash commands of the installed plugins. Read-only.
func statusExplain(cwd, pluginsDir string) string {
	var b strings.Builder
	vers, cmds := pluginInfo(pluginsDir)
	line := fmt.Sprintf("mw %s (schema %d)", version, currentSchema)
	if len(vers) > 0 {
		line += " · plugins " + strings.Join(vers, ", ")
	}
	fmt.Fprintln(&b, line)
	var m *newsModel
	if path := statusFindBoard(cwd); path != "" {
		if w, err := load(path); err == nil {
			m = buildNewsModel(w, newsPart{title: w.Name, tenants: newsJobTenants(w, "")}, nil, newsWindow{full: true})
		}
	}
	fmt.Fprintln(&b, "\nStatus line")
	if m == nil {
		fmt.Fprintln(&b, "No mw board here, so the chat bar shows nothing.")
	} else {
		c := m.Now.Counts
		fmt.Fprintf(&b, "%s\n", formatStatusLine(c))
		fmt.Fprintln(&b, "- mw: is the mw board.")
		fmt.Fprintf(&b, "- N = needs you: %d (an open ask, or blocked with no dependency).\n", c.Needs)
		fmt.Fprintf(&b, "- R = review: %d (finished items waiting for their review).\n", c.Review)
		fmt.Fprintf(&b, "- B = blocked: %d (blocked on another item, mw block --on ID).\n", c.Blocked)
		fmt.Fprintf(&b, "- %d/%d = open items over all items (done included, inbox drafts left out).\n", c.Work-c.Done, c.Work)
		fmt.Fprintln(&b, "An item is counted once. A letter that is missing means its count is zero.")
		fmt.Fprintln(&b, "--link adds the board link only when exactly one board is found (otherwise the line prints without a link).")
		fmt.Fprintln(&b, `To show it in the chat bar add to ~/.claude/settings.json: "statusLine":{"type":"command","command":"mw status --line --link"}`)
		if len(m.Now.Needs) > 0 {
			fmt.Fprintln(&b, "\nNeeds you")
			for _, it := range m.Now.Needs {
				fmt.Fprintf(&b, "- %s/%s %s\n", it.Tenant, it.ID, bc(it.Title))
			}
		}
	}
	fmt.Fprintln(&b, "\nSettings (mw user set KEY=VALUE; empty VALUE resets; mw configure in a terminal)")
	c, _, _, _ := readUserConfig()
	for _, k := range userKeys {
		v, src := userValue(c, &k)
		fmt.Fprintf(&b, "- %s = %s (%s; %s) %s\n", k.Name, v, src, k.allowed(), k.Help)
	}
	fmt.Fprintln(&b, "- More fanfare settings (milestone and finale sounds, Stop dialog, volume): mw fanfare -j, mw fanfare set KEY=VALUE.")
	fmt.Fprintln(&b, "- Alerts are the fanfare sounds plus the Stop dialog: no popups, no OS notifications; the old pane.* and alerts.* keys are gone.")
	if len(cmds) > 0 {
		fmt.Fprintln(&b, "\nCommands")
		for _, cm := range cmds {
			fmt.Fprintf(&b, "- %s\n", cm)
		}
	}
	return b.String()
}
