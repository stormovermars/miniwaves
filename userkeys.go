package main

import (
	"fmt"
	"strconv"
	"strings"
)

// The per-user settings table: the one place that names every key, its group, allowed values, default
// and help. The CLI (user get/set, configure), the validation and USAGE.md all come from it.
// The keys are per user and never stored on a board. fanfare.enabled and fanfare.use are the existing
// UserFanfare fields; every other key lives in UserConfig.Settings as a canonical string.

type userKey struct {
	Name    string
	Group   string   // group id: pane, alerts, fanfare, status, discover
	Kind    string   // enum, bool, int, list, text
	Choices []string // allowed values (enum, bool, int range, list members); nil for free text
	Default string
	Help    string
	lo, hi  int
}

var userGroups = []struct{ ID, Label string }{
	{"pane", "Pane"}, {"alerts", "Alerts"}, {"fanfare", "Fanfare"}, {"status", "Chat bar"}, {"discover", "Discovery"},
}

var alertKinds = []string{"blocked", "ask", "review", "done", "release", "metrics", "new"}

var userKeys = []userKey{
	{Name: "pane.refresh", Group: "pane", Kind: "enum", Choices: []string{"15s", "30s", "1m", "5m", "off"}, Default: "30s", Help: "How often the live pane refreshes."},
	{Name: "pane.style", Group: "pane", Kind: "enum", Choices: []string{"normal", "blocks", "gopher", "agile", "kanban"}, Default: "normal", Help: "Look of the pane."},
	{Name: "pane.zoom", Group: "pane", Kind: "int", lo: -2, hi: 2, Default: "0", Help: "Pane text size step, -2 (smaller) to +2 (larger)."},
	{Name: "pane.fit", Group: "pane", Kind: "enum", Choices: []string{"80", "fill"}, Default: "80", Help: "Pane width: 80 columns or fill the window."},
	{Name: "pane.open", Group: "pane", Kind: "enum", Choices: []string{"jobs", "news"}, Default: "jobs", Help: "Which view the pane opens on."},
	{Name: "alerts.enabled", Group: "alerts", Kind: "bool", Default: "true", Help: "Raise alerts for board events."},
	{Name: "alerts.kinds", Group: "alerts", Kind: "list", Choices: alertKinds, Default: strings.Join(alertKinds, ","), Help: "Which events alert: a comma list, all, or none."},
	{Name: "alerts.message", Group: "alerts", Kind: "bool", Default: "true", Help: "Show a message with an alert."},
	{Name: "alerts.sound", Group: "alerts", Kind: "bool", Default: "true", Help: "Play a sound with an alert."},
	{Name: "fanfare.enabled", Group: "fanfare", Kind: "bool", Default: "false", Help: "Play fanfares on wave close; overrides the board's fanfare.enabled when set."},
	{Name: "fanfare.use", Group: "fanfare", Kind: "text", Default: "all", Help: "The fanfare set to play: a set name or all."},
	{Name: "status.show", Group: "status", Kind: "bool", Default: "true", Help: "Show the mw line in the chat status bar."},
	{Name: "status.detail", Group: "status", Kind: "enum", Choices: []string{"compact", "full"}, Default: "compact", Help: "How much the status line says."},
	{Name: "discover.home", Group: "discover", Kind: "bool", Default: "true", Help: "Also look for boards under your home folder."},
	{Name: "discover.depth", Group: "discover", Kind: "int", lo: 1, hi: 6, Default: "4", Help: "How many folder levels deep to look for boards."},
}

func init() {
	for i := range userKeys {
		k := &userKeys[i]
		switch k.Kind {
		case "bool":
			k.Choices = []string{"true", "false"}
		case "int":
			k.Choices = nil
			for n := k.lo; n <= k.hi; n++ {
				k.Choices = append(k.Choices, strconv.Itoa(n))
			}
		}
	}
}

func userKeyByName(name string) *userKey {
	for i := range userKeys {
		if userKeys[i].Name == name {
			return &userKeys[i]
		}
	}
	return nil
}

func userKeyNames() []string {
	var n []string
	for _, k := range userKeys {
		n = append(n, k.Name)
	}
	return n
}

func (k *userKey) allowed() string {
	switch k.Kind {
	case "int":
		return fmt.Sprintf("%d..%d", k.lo, k.hi)
	case "list":
		return "a comma list of " + strings.Join(k.Choices, ", ") + ", all, or none"
	case "text":
		return "a set name or all"
	}
	return strings.Join(k.Choices, ", ")
}

// normalize validates val and returns its canonical form.
func (k *userKey) normalize(val string) (string, error) {
	v := strings.TrimSpace(val)
	bad := func() (string, error) {
		return "", fmt.Errorf("%s: %q is not valid (allowed: %s)", k.Name, val, k.allowed())
	}
	switch k.Kind {
	case "enum", "bool":
		v = strings.ToLower(v)
		for _, c := range k.Choices {
			if v == c {
				return v, nil
			}
		}
		return bad()
	case "int":
		n, err := strconv.Atoi(strings.TrimPrefix(v, "+"))
		if err != nil || n < k.lo || n > k.hi {
			return bad()
		}
		return strconv.Itoa(n), nil
	case "list":
		if strings.ToLower(v) == "all" {
			return k.Default, nil
		}
		if strings.ToLower(v) == "none" {
			return "", nil // the empty set: no event alerts
		}
		seen := map[string]bool{}
		for _, p := range strings.Split(v, ",") {
			p = strings.ToLower(strings.TrimSpace(p))
			ok := false
			for _, c := range k.Choices {
				ok = ok || p == c
			}
			if !ok {
				return bad()
			}
			seen[p] = true
		}
		var out []string
		for _, c := range k.Choices { // canonical order
			if seen[c] {
				out = append(out, c)
			}
		}
		return strings.Join(out, ","), nil
	}
	return bad()
}

// userValue is the effective value of key and where it came from ("user" or "default").
func userValue(c UserConfig, k *userKey) (string, string) {
	switch k.Name {
	case "fanfare.enabled":
		if c.Fanfare.Enabled != nil {
			return strconv.FormatBool(*c.Fanfare.Enabled), "user"
		}
	case "fanfare.use":
		if c.Fanfare.Use != "" {
			return c.Fanfare.Use, "user"
		}
	default:
		if v, ok := c.Settings[k.Name]; ok {
			return v, "user"
		}
	}
	return k.Default, "default"
}

// userGetJSON is the stable -j shape of mw user get: {"keys":[{key,value,default,source,group,choices}]}.
type userGetKey struct {
	Key     string   `json:"key"`
	Value   string   `json:"value"`
	Default string   `json:"default"`
	Source  string   `json:"source"`
	Group   string   `json:"group"`
	Choices []string `json:"choices"`
}

// boardWorkspace is the nearest board's index (for the fanfare block), or nil when there is none.
func boardWorkspace() *Workspace {
	if path, err := findWorkspace(); err == nil {
		if w, err := loadScoped(path, false); err == nil {
			return w
		}
	}
	return nil
}

// userGetKeys lists the keys; run inside a board whose fanfare is explicitly off, fanfare.enabled is
// "false" with source "board", because the board's off wins over the user layer.
func userGetKeys(c UserConfig, only string, w *Workspace) []userGetKey {
	out := []userGetKey{}
	for i := range userKeys {
		k := &userKeys[i]
		if only != "" && k.Name != only {
			continue
		}
		v, src := userValue(c, k)
		if k.Name == "fanfare.enabled" && boardFanfareOff(w) {
			v, src = "false", "board"
		}
		ch := k.Choices
		if ch == nil {
			ch = []string{}
		}
		out = append(out, userGetKey{k.Name, v, k.Default, src, k.Group, ch})
	}
	return out
}

func printUserSettings(c UserConfig, only string, asJSON bool, bw *Workspace) error {
	keys := userGetKeys(c, only, bw)
	if asJSON {
		return printJSON(map[string]any{"keys": keys})
	}
	w := 0
	for _, k := range keys {
		w = max(w, len(k.Key))
	}
	for _, k := range keys {
		fmt.Printf("%-*s  %s  (%s)\n", w, k.Key, k.Value, k.Source)
	}
	return nil
}
