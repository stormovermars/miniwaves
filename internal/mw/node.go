package mw

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stormovermars/miniwaves/internal/fsx"
)

// Nodes: the computers that work one shared board. A node's name is a per-user setting (node.name);
// its heartbeat is one small file per node, .miniwaves/nodes/<name>.json, so two computers never
// write the same file. Everything here is optional: a board that never names a node is unchanged.

var nodeStates = []string{"working", "idle", "blocked", "limit", "off"}

// nodeBeat is .miniwaves/nodes/<name>.json.
type nodeBeat struct {
	Node     string `json:"node"`
	At       string `json:"at"`
	State    string `json:"state"`
	Item     string `json:"item,omitempty"`
	Msg      string `json:"msg,omitempty"`
	Sessions int    `json:"sessions"`
	Ready    int    `json:"ready"`
	Doing    int    `json:"doing"`
	Left     *int   `json:"left,omitempty"` // budget left in percent, as the node reports it (mw node beat --left)
	Version  string `json:"version"`
}

func nodePath(board, name string) string {
	return storePath(board, filepath.Join("nodes", name+".json"))
}

// userSetting is a per-user setting's effective value (its default when unset or on a read error).
func userSetting(name string) string {
	k := userKeyByName(name)
	if k == nil {
		return ""
	}
	c, _, _, err := readUserConfig()
	if err != nil {
		return k.Default
	}
	v, _ := userValue(c, k)
	return v
}

// nodeName is this computer's node name, "" when none was set (mw node set).
func nodeName() string { return userSetting("node.name") }

func userMinutes(name string) time.Duration {
	n, err := strconv.Atoi(userSetting(name))
	if err != nil || n < 1 {
		n, _ = strconv.Atoi(userKeyByName(name).Default)
	}
	return time.Duration(n) * time.Minute
}

// routeOf is the node a row is routed to: its own node, else the tenant default, else "any".
func routeOf(t *Tenant, it Item) string {
	switch {
	case it.Node != "":
		return it.Node
	case t.Node != "":
		return t.Node
	}
	return "any"
}

// routesTo reports whether node may take the row: routed to it or to anyone. An empty node matches all.
func routesTo(t *Tenant, it Item, node string) bool {
	r := routeOf(t, it)
	return node == "" || node == "-" || r == "any" || r == node
}

// itemNode is the node a row in progress runs on: where it started, else where it is routed.
func itemNode(t *Tenant, it Item) string {
	if it.StartedOn != "" {
		return it.StartedOn
	}
	if r := routeOf(t, it); r != "any" {
		return r
	}
	return ""
}

func readBeats(board string) []nodeBeat {
	var out []nodeBeat
	ents, _ := os.ReadDir(filepath.Dir(nodePath(board, "x")))
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := fsx.ReadFile(filepath.Join(filepath.Dir(nodePath(board, "x")), e.Name()))
		var nb nodeBeat
		if err != nil || json.Unmarshal(b, &nb) != nil || nb.Node == "" {
			continue
		}
		out = append(out, nb)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Node < out[j].Node })
	return out
}

// silentBeat reports whether a node's last beat is older than node.silent.
func silentBeat(nb nodeBeat, now time.Time) bool {
	at, err := time.Parse(time.RFC3339, nb.At)
	return err != nil || now.Sub(at) > userMinutes("node.silent")
}

func nodeCommand(path string, w *Workspace, args []string) error {
	const use = "usage: mw node set NAME | beat [working|idle|blocked|limit|off] [--item ID] [-m TEXT] | list [-j]"
	if len(args) == 0 {
		return errors.New(use)
	}
	o, pos, err := options(args[1:])
	if err != nil {
		return err
	}
	switch args[0] {
	case "set":
		if len(pos) != 1 || !validID(pos[0]) {
			return errors.New("usage: mw node set NAME (at most 64 lowercase letters, numbers, and single dashes)")
		}
		p, err := userConfigPath()
		if err != nil {
			return err
		}
		if err := updateUserConfig(p, func(c *UserConfig) (bool, error) { return true, setUserKey(c, "node.name", pos[0]) }); err != nil {
			return err
		}
		fmt.Println("node: " + pos[0])
		return nil
	case "beat":
		return nodeBeatCommand(path, w, o, pos)
	case "list":
		if len(pos) != 0 {
			return fmt.Errorf("unexpected argument %q", pos[0])
		}
		if err := allowOptions(o, "json"); err != nil {
			return err
		}
		return nodeList(path, o)
	}
	return errors.New(use)
}

func nodeBeatCommand(path string, w *Workspace, o map[string]string, pos []string) error {
	if err := allowOptions(o, "item", "message", "left"); err != nil {
		return err
	}
	if len(pos) > 1 {
		return errors.New("usage: mw node beat [working|idle|blocked|limit|off] [--item ID] [-m TEXT]")
	}
	name := nodeName()
	if name == "" {
		return errors.New("no node name on this computer: run mw node set NAME")
	}
	nb := nodeBeat{Node: name, At: nowStamp(), Item: strings.ToUpper(o["item"]), Msg: compactText(o["message"], 200), Version: version}
	for _, t := range shown(w, map[string]string{}) {
		for _, it := range t.Items {
			switch it.Status {
			case "ready":
				if routesTo(t, it, name) && routeOf(t, it) == name {
					nb.Ready++
				}
			case "doing":
				if itemNode(t, it) == name {
					nb.Doing++
					if nb.Item == "" && o["item"] == "" {
						nb.Item = it.ID
					}
				}
			}
		}
	}
	for _, s := range w.Sessions {
		if s.Node == name && s.State != "stopped" {
			nb.Sessions++
		}
	}
	if o["left"] != "" {
		n, err := strconv.Atoi(strings.TrimSuffix(o["left"], "%"))
		if err != nil || n < 0 || n > 100 {
			return errors.New("--left needs a percent from 0 to 100")
		}
		nb.Left = &n
	}
	if len(pos) == 1 {
		nb.State = pos[0]
		if !contains(nodeStates, nb.State) {
			return fmt.Errorf("unknown state %q (%s)", nb.State, strings.Join(nodeStates, "|"))
		}
	} else if nb.Doing > 0 {
		nb.State = "working"
	} else {
		nb.State = "idle"
	}
	b, _ := json.Marshal(nb)
	if err := os.MkdirAll(filepath.Dir(nodePath(path, name)), 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(nodePath(path, name), append(b, '\n')); err != nil {
		return err
	}
	fmt.Printf("beat %s %s sessions %d ready %d doing %d\n", name, nb.State, nb.Sessions, nb.Ready, nb.Doing)
	return nil
}

type nodeRow struct {
	Node     string `json:"node" toon:"node"`
	Live     bool   `json:"live" toon:"live"`
	State    string `json:"state" toon:"state"`
	At       string `json:"at,omitempty" toon:"at,omitempty"`
	Item     string `json:"item,omitempty" toon:"item,omitempty"`
	Msg      string `json:"msg,omitempty" toon:"msg,omitempty"`
	Sessions int    `json:"sessions" toon:"sessions"`
	Ready    int    `json:"ready" toon:"ready"`
	Doing    int    `json:"doing" toon:"doing"`
	Version  string `json:"version,omitempty" toon:"version,omitempty"`
}

func nodeRows(path string, now time.Time) []nodeRow {
	rows := []nodeRow{}
	for _, nb := range readBeats(path) {
		rows = append(rows, nodeRow{nb.Node, !silentBeat(nb, now), nb.State, nb.At, nb.Item, nb.Msg, nb.Sessions, nb.Ready, nb.Doing, nb.Version})
	}
	return rows
}

func nodeList(path string, o map[string]string) error {
	rows := nodeRows(path, time.Now())
	if o["json"] == "true" {
		return printJSON(rows)
	}
	if len(rows) == 0 {
		fmt.Println("no nodes (mw node set NAME, then mw node beat)")
		return nil
	}
	for _, r := range rows {
		st := "live"
		if !r.Live {
			st = "silent"
		}
		fmt.Printf("%-12s %-6s %-8s %-5s sessions %d ready %d doing %d %s\n", r.Node, st, r.State, ago(r.At), r.Sessions, r.Ready, r.Doing, r.Item)
	}
	return nil
}

// checkRoute validates a route: a node name, any, or - (clear; the tenant default applies again).
func checkRoute(v string) (string, error) {
	switch {
	case v == "-":
		return "", nil
	case v == "any" || validID(v):
		return v, nil
	}
	return "", errors.New("node must be a node name, any, or - (at most 64 lowercase letters, numbers, and single dashes)")
}

func parseCost(v string) (int, error) {
	if v == "-" || v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 100000000 {
		return 0, errors.New("cost must be a whole number of thousands of tokens, or - to clear it")
	}
	return n, nil
}

// tenantNode runs mw tenant node SLUG [NAME|any|-]: the default route of the tenant's rows.
func tenantNode(path string, w *Workspace, args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errors.New("usage: mw tenant node SLUG [NAME|any|-]")
	}
	t, err := tenantBySlug(w, args[0])
	if err != nil {
		return err
	}
	if len(args) == 1 {
		fmt.Printf("%s node: %s\n", t.Slug, orDash(t.Node))
		return nil
	}
	if t.Node, err = checkRoute(args[1]); err != nil {
		return err
	}
	if err := save(path, w); err != nil {
		return err
	}
	fmt.Printf("%s node: %s\n", t.Slug, orDash(t.Node))
	return nil
}
