package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Coordinated jobs (board config only): an omitempty "jobs" object of .miniwaves.json keyed by job id,
// and an omitempty "job" field on a tenant. mw ships data and prompts; it never starts sessions.
// Nothing here executes: every value is data, and every path stays relative to the workspace.

var (
	jobIDPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	paneStylePattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	lockNamePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

// JobConfig holds the explicitly set keys of one job; zero values mean "default".
type JobConfig struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MaxWorkers  int    `json:"max_workers,omitempty"`
	Worktrees   string `json:"worktrees,omitempty"`
	Lock        string `json:"lock,omitempty"`
	NewsOut     string `json:"news_out,omitempty"`
	NewsURL     string `json:"news_url,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Pane        string `json:"pane,omitempty"`
	PaneStyle   string `json:"pane_style,omitempty"`

	extra map[string]json.RawMessage // keys this mw does not know: kept, written back
}

// jobDotted maps the 0.2.9-dev dotted storage keys to their snake_case names.
var jobDotted = map[string]string{"news.out": "news_out", "news.url": "news_url", "pane.style": "pane_style"}

// UnmarshalJSON reads the snake_case keys, falls back to the old dotted ones when the new one is missing,
// and keeps unknown keys. Writing always uses the snake_case names, so a save migrates the board.
func (c *JobConfig) UnmarshalJSON(b []byte) error {
	type plain JobConfig
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	known := map[string]bool{}
	for _, k := range []string{"title", "description", "max_workers", "worktrees", "lock", "news_out", "news_url", "summary", "pane", "pane_style"} {
		known[k] = true
	}
	for old, nw := range jobDotted {
		if v, ok := raw[old]; ok {
			if _, has := raw[nw]; !has {
				var s string
				if json.Unmarshal(v, &s) != nil {
					return fmt.Errorf("job key %s: want a string", old)
				}
				switch nw {
				case "news_out":
					p.NewsOut = s
				case "news_url":
					p.NewsURL = s
				case "pane_style":
					p.PaneStyle = s
				}
			}
			delete(raw, old)
		}
	}
	for k := range known {
		delete(raw, k)
	}
	p.extra = nil
	if len(raw) > 0 {
		p.extra = raw
	}
	*c = JobConfig(p)
	return nil
}

// MarshalJSON writes the known keys, then the kept unknown ones.
func (c JobConfig) MarshalJSON() ([]byte, error) {
	type plain JobConfig
	b, err := json.Marshal(plain(c))
	if err != nil || len(c.extra) == 0 {
		return b, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	for k, v := range c.extra {
		m[k] = v
	}
	return json.Marshal(m)
}

// JobView is a job with its defaults applied and its tenants listed.
type JobView struct {
	ID          string   `json:"id" toon:"id"`
	Title       string   `json:"title" toon:"title"`
	Description string   `json:"description" toon:"description"`
	Tenants     []string `json:"tenants" toon:"tenants"`
	MaxWorkers  int      `json:"max_workers" toon:"max_workers"`
	Worktrees   string   `json:"worktrees" toon:"worktrees"`
	Lock        string   `json:"lock" toon:"lock"`
	NewsOut     string   `json:"news_out" toon:"news_out"`
	NewsURL     string   `json:"news_url" toon:"news_url"`
	Summary     string   `json:"summary" toon:"summary"`
	Pane        string   `json:"pane" toon:"pane"`
	PaneStyle   string   `json:"pane_style" toon:"pane_style"`
}

var jobKeys = []string{"title", "description", "max_workers", "worktrees", "lock", "news.out", "news.url", "summary", "pane", "pane.style"}

// jobView applies the defaults.
func jobView(w *Workspace, id string) JobView {
	c := w.Jobs[id]
	v := JobView{ID: id, Title: c.Title, Description: c.Description, Tenants: []string{}, MaxWorkers: c.MaxWorkers, Worktrees: c.Worktrees, Lock: c.Lock,
		NewsOut: c.NewsOut, NewsURL: c.NewsURL, Summary: c.Summary, Pane: c.Pane, PaneStyle: c.PaneStyle}
	if v.Title == "" {
		v.Title = id
	}
	if v.MaxWorkers == 0 {
		v.MaxWorkers = 4
	}
	if v.Worktrees == "" {
		v.Worktrees = "on"
	}
	if v.NewsOut == "" {
		v.NewsOut = "docs/news/"
	}
	if v.Pane == "" {
		v.Pane = "on"
	}
	for _, t := range w.Tenants {
		if t.Job == id {
			v.Tenants = append(v.Tenants, t.Slug)
		}
	}
	return v
}

func noControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// workspaceRelPath checks a board-supplied path: relative, no "..", no "~", no drive or UNC, no control
// characters. It returns the cleaned form with forward slashes (a trailing slash, meaning a folder, is kept).
func workspaceRelPath(key, p string) (string, error) {
	bad := func(why string) (string, error) { return "", fmt.Errorf("%s: %q %s", key, p, why) }
	if strings.TrimSpace(p) == "" || !noControl(p) {
		return bad("must be a non-empty path without control characters")
	}
	s := strings.ReplaceAll(p, `\`, "/")
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~") || (len(s) >= 2 && s[1] == ':') {
		return bad("must be relative to the workspace (no absolute paths, no ~, no drive)")
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == ".." {
			return bad("must stay inside the workspace (no ..)")
		}
	}
	c := path.Clean(s)
	if c == "." || c == "" {
		return bad("must name a file or folder inside the workspace")
	}
	if strings.HasSuffix(s, "/") {
		c += "/"
	}
	return c, nil
}

// setJobKey validates one key=value and stores it; an empty value resets the key to its default.
func setJobKey(c *JobConfig, key, val string) error {
	if val != "" && !noControl(val) {
		return fmt.Errorf("%s: no control characters", key)
	}
	switch key {
	case "title":
		if len([]rune(val)) > 120 {
			return errors.New("title: at most 120 characters")
		}
		c.Title = val
	case "description":
		if len([]rune(val)) > 120 {
			return errors.New("description: at most 120 characters")
		}
		c.Description = val
	case "max_workers":
		if val == "" {
			c.MaxWorkers = 0
			return nil
		}
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 || n > 32 {
			return fmt.Errorf("max_workers: %q is not a number from 1 to 32", val)
		}
		c.MaxWorkers = n
	case "worktrees", "pane":
		if val != "" && val != "on" && val != "off" {
			return fmt.Errorf("%s: %q must be on or off", key, val)
		}
		if key == "pane" {
			c.Pane = val
		} else {
			c.Worktrees = val
		}
	case "lock":
		if val != "" && !lockNamePattern.MatchString(val) {
			return fmt.Errorf("lock: %q must be a command name the kit provides (letters, digits, . _ -), not a command line", val)
		}
		c.Lock = val
	case "news.out", "summary":
		if val != "" {
			var err error
			if val, err = workspaceRelPath(key, val); err != nil {
				return err
			}
		}
		if key == "summary" {
			c.Summary = val
		} else {
			c.NewsOut = val
		}
	case "news.url":
		if val != "" && (!(strings.HasPrefix(val, "https://") || strings.HasPrefix(val, "http://")) || len(val) > 500 || strings.ContainsAny(val, " <>\"'`")) {
			return fmt.Errorf("news.url: %q must be an http(s) URL", val)
		}
		c.NewsURL = val
	case "pane.style":
		if val != "" && !paneStylePattern.MatchString(val) {
			return fmt.Errorf("pane.style: %q must match ^[a-z0-9-]{1,32}$", val)
		}
		c.PaneStyle = val
	default:
		return fmt.Errorf("unknown job key %q (keys: %s)", key, strings.Join(jobKeys, ", "))
	}
	return nil
}

// takeTenants pulls every -t/--tenant value (repeatable, comma lists allowed) out of args.
func takeTenants(args []string) (slugs, rest []string, err error) {
	for i := 0; i < len(args); i++ {
		if args[i] == "-t" || args[i] == "--tenant" {
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("%s needs a value", args[i])
			}
			for _, s := range strings.Split(args[i+1], ",") {
				if s = strings.TrimSpace(s); s != "" {
					slugs = append(slugs, s)
				}
			}
			i++
			continue
		}
		rest = append(rest, args[i])
	}
	return slugs, rest, nil
}

func jobsCommand(p string, w *Workspace, args []string) error {
	const use = "usage: mw jobs enable ID -t SLUG... | disable ID [-t SLUG] | set ID KEY=VALUE... | show [ID] [-j]"
	if len(args) == 0 {
		return errors.New(use)
	}
	sub := args[0]
	slugs, rest, err := takeTenants(args[1:])
	if err != nil {
		return err
	}
	o, pos, err := options(rest)
	if err != nil {
		return err
	}
	if err := allowOptions(o, "json"); err != nil {
		return err
	}
	checkID := func(id string) error {
		if !jobIDPattern.MatchString(id) {
			return fmt.Errorf("job id %q must match ^[a-z0-9][a-z0-9-]{0,31}$", id)
		}
		return nil
	}
	switch sub {
	case "enable":
		if len(pos) != 1 || len(slugs) == 0 {
			return errors.New("usage: mw jobs enable ID -t SLUG...")
		}
		id := pos[0]
		if err := checkID(id); err != nil {
			return err
		}
		var ts []*Tenant
		for _, s := range slugs {
			t, err := tenantBySlug(w, s)
			if err != nil {
				return err
			}
			if err := refuseArchived(t); err != nil {
				return err
			}
			ts = append(ts, t)
		}
		if w.Jobs == nil {
			w.Jobs = map[string]JobConfig{}
		}
		if _, ok := w.Jobs[id]; !ok {
			w.Jobs[id] = JobConfig{}
		}
		for _, t := range ts {
			if t.Job != "" && t.Job != id {
				fmt.Printf("moved %s from job %s\n", t.Slug, t.Job)
			}
			t.Job = id
		}
		if err := save(p, w); err != nil {
			return err
		}
		fmt.Printf("job %s: tenants %s\n", id, strings.Join(jobView(w, id).Tenants, ", "))
		return nil
	case "disable":
		if len(pos) != 1 {
			return errors.New("usage: mw jobs disable ID [-t SLUG]")
		}
		id := pos[0]
		if _, ok := w.Jobs[id]; !ok {
			return fmt.Errorf("job %s not found", id)
		}
		var cleared []string
		if len(slugs) == 0 {
			for i := range w.Tenants {
				if w.Tenants[i].Job == id {
					w.Tenants[i].Job = ""
					cleared = append(cleared, w.Tenants[i].Slug)
				}
			}
		}
		for _, s := range slugs {
			t, err := tenantBySlug(w, s)
			if err != nil {
				return err
			}
			if t.Job != id {
				return fmt.Errorf("tenant %s is not in job %s", s, id)
			}
			t.Job = ""
			cleared = append(cleared, s)
		}
		if err := save(p, w); err != nil {
			return err
		}
		fmt.Printf("job %s: cleared %d tenant(s) (job data kept)\n", id, len(cleared))
		return nil
	case "set":
		if len(slugs) > 0 {
			return errors.New("mw jobs set takes no -t")
		}
		if len(pos) < 2 {
			return errors.New("usage: mw jobs set ID KEY=VALUE...")
		}
		id := pos[0]
		c, ok := w.Jobs[id]
		if !ok {
			return fmt.Errorf("job %s not found (mw jobs enable %s -t SLUG)", id, id)
		}
		for _, kv := range pos[1:] {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return fmt.Errorf("%q is not KEY=VALUE", kv)
			}
			if err := setJobKey(&c, strings.TrimSpace(k), v); err != nil {
				return err
			}
		}
		w.Jobs[id] = c
		if err := save(p, w); err != nil {
			return err
		}
		fmt.Printf("job %s saved\n", id)
		return nil
	case "show":
		if len(slugs) > 0 || len(pos) > 1 {
			return errors.New("usage: mw jobs show [ID] [-j]")
		}
		if len(pos) == 1 {
			if _, ok := w.Jobs[pos[0]]; !ok {
				return fmt.Errorf("job %s not found", pos[0])
			}
			return printData(jobView(w, pos[0]), o)
		}
		ids := make([]string, 0, len(w.Jobs))
		for id := range w.Jobs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		views := make([]JobView, 0, len(ids))
		for _, id := range ids {
			views = append(views, jobView(w, id))
		}
		return printData(views, o)
	}
	return errors.New(use)
}
