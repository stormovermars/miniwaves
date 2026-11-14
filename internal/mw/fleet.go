package mw

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/stormovermars/miniwaves/internal/fsx"
)

// mw fleet: the fleets view. One read of the board and the node beat files answers: which nodes are live,
// what each has in flight, what is stalled, blocked or waiting for an answer, what each would take next,
// and which rows could move. Nothing is written except, with --changed, a small state file in the user's
// mw config folder.

type fleetRow struct {
	Item  string `json:"item" toon:"item"` // TENANT:ID
	Title string `json:"title" toon:"title"`
	Route string `json:"route,omitempty" toon:"route,omitempty"`
	Age   string `json:"age,omitempty" toon:"age,omitempty"`   // since it was started (display only)
	Last  string `json:"last,omitempty" toon:"last,omitempty"` // the last log entry: action and note
	At    string `json:"at,omitempty" toon:"at,omitempty"`     // when that entry was written
	Why   string `json:"why,omitempty" toon:"why,omitempty"`
	Cost  int    `json:"cost,omitempty" toon:"cost,omitempty"`
}

type fleetNode struct {
	Node      string     `json:"node" toon:"node"`
	Live      bool       `json:"live" toon:"live"`
	State     string     `json:"state,omitempty" toon:"state,omitempty"`
	Beat      string     `json:"beat,omitempty" toon:"beat,omitempty"`
	Left      *int       `json:"left,omitempty" toon:"left,omitempty"`
	Low       bool       `json:"low,omitempty" toon:"low,omitempty"` // budget left is 10% or less
	Out       bool       `json:"out,omitempty" toon:"out,omitempty"` // out of budget (left 0 or state limit) until OutUntil
	OutUntil  string     `json:"out_until,omitempty" toon:"out_until,omitempty"`
	Queued    []fleetRow `json:"queued" toon:"queued"` // ready rows pinned to this node while it is out
	Sessions  int        `json:"sessions" toon:"sessions"`
	InFlight  []fleetRow `json:"in_flight" toon:"in_flight"`
	Stalled   []fleetRow `json:"stalled" toon:"stalled"`
	Blocked   []fleetRow `json:"blocked" toon:"blocked"`
	Waiting   []fleetRow `json:"waiting" toon:"waiting"`
	Ready     int        `json:"ready" toon:"ready"`
	ReadyCost int        `json:"ready_cost,omitempty" toon:"ready_cost,omitempty"`
	DoingCost int        `json:"doing_cost,omitempty" toon:"doing_cost,omitempty"`
	Next      *fleetRow  `json:"next" toon:"next"`
}

type fleetView struct {
	Nodes   []fleetNode `json:"nodes" toon:"nodes"`
	Movable []fleetRow  `json:"movable" toon:"movable"` // stalled rows anyone may take
	Held    []fleetRow  `json:"held" toon:"held"`       // ready or stalled rows pinned to a silent node
	Any     fleetAny    `json:"any" toon:"any"`         // ready rows routed to anyone
}

type fleetAny struct {
	Ready int       `json:"ready" toon:"ready"`
	Cost  int       `json:"cost,omitempty" toon:"cost,omitempty"`
	Next  *fleetRow `json:"next" toon:"next"`
}

const unknownNode = "?"

func lastLogEvent(it Item) (Event, bool) {
	if n := len(it.Events); n > 0 {
		return it.Events[n-1], true
	}
	return Event{}, false
}

func startedAt(it Item) string {
	for i := len(it.Events) - 1; i >= 0; i-- {
		if it.Events[i].Action == "start" {
			return it.Events[i].At
		}
	}
	return it.Created
}

// readyForNode is the row mw next --node NAME would serve in tenant t ("" when none).
func nextFor(w *Workspace, ts []*Tenant, node string) *Item {
	for _, t := range ts {
		if it, _, _ := pickNext(w, t, map[string]string{"node": node, "askskip": "true"}); it != nil {
			c := *it
			c.Parent = t.Slug // carried to the caller: the tenant of the row
			return &c
		}
	}
	return nil
}

func buildFleet(path string, w *Workspace, o map[string]string, now time.Time) fleetView {
	var ts []*Tenant
	for _, t := range shown(w, map[string]string{}) {
		if o["tenant"] == "" || t.Slug == o["tenant"] {
			ts = append(ts, t)
		}
	}
	silentFor, stallFor := userMinutes("node.silent"), userMinutes("node.stall")
	beats := map[string]nodeBeat{}
	nodes := map[string]*fleetNode{}
	node := func(name string) *fleetNode {
		n := nodes[name]
		if n == nil {
			n = &fleetNode{Node: name, Queued: []fleetRow{}, InFlight: []fleetRow{}, Stalled: []fleetRow{}, Blocked: []fleetRow{}, Waiting: []fleetRow{}}
			nodes[name] = n
		}
		return n
	}
	for _, nb := range readBeats(path) {
		beats[nb.Node] = nb
		n := node(nb.Node)
		n.Out = beatOut(nb, now)
		n.OutUntil = nb.Resets
		n.Live = n.Out || !silentBeatAt(nb, now, silentFor) // out of budget is not silent
		n.State, n.Beat, n.Left, n.Sessions = nb.State, nb.At, nb.Left, nb.Sessions
		n.Low = nb.Left != nil && *nb.Left <= 10
	}
	silent := func(name string) bool {
		nb, ok := beats[name]
		return name != unknownNode && name != "any" && !(ok && beatOut(nb, now)) && (!ok || silentBeatAt(nb, now, silentFor))
	}
	outUntil := func(name string) (string, bool) {
		nb, ok := beats[name]
		if ok && beatOut(nb, now) {
			if nb.Resets == "" {
				return "reset time unknown", true
			}
			return nb.Resets, true
		}
		return "", false
	}
	row := func(t *Tenant, it Item) fleetRow {
		r := fleetRow{Item: t.Slug + ":" + it.ID, Title: compactText(it.Title, 60), Cost: it.Cost}
		if rt := routeOf(t, it); rt != "any" {
			r.Route = rt
		}
		if e, ok := lastLogEvent(it); ok {
			r.Last, r.At = e.Action, e.At
			if e.Note != "" {
				r.Last += " " + compactText(e.Note, 40)
			}
			r.Last += " " + ago(e.At)
		}
		return r
	}
	var view fleetView
	view.Movable, view.Held = []fleetRow{}, []fleetRow{}
	for _, t := range ts {
		for _, it := range t.Items {
			if it.Kind == "wave" || it.Status == "done" || it.Status == "inbox" {
				continue
			}
			where := itemNode(t, it)
			if where == "" {
				where = unknownNode
			}
			r := row(t, it)
			if _, asked := openAsk(it); asked {
				n := node(where)
				r.Why = "open ask"
				n.Waiting = append(n.Waiting, r)
				continue
			}
			switch it.Status {
			case "doing":
				n := node(where)
				r.Age = ago(startedAt(it))
				n.DoingCost += it.Cost
				stalled := ""
				if until, out := outUntil(where); out {
					stalled = "node out until " + until
				} else if silent(where) {
					stalled = "node silent"
				} else if e, ok := lastLogEvent(it); ok {
					if at := parseNano(e.At); !at.IsZero() && now.Sub(at) > stallFor {
						stalled = "no log entry for " + ago(e.At)
					}
				}
				if stalled == "" {
					n.InFlight = append(n.InFlight, r)
					continue
				}
				r.Why = stalled
				n.Stalled = append(n.Stalled, r)
				switch rt := routeOf(t, it); {
				case rt == "any":
					view.Movable = append(view.Movable, r)
				case silent(rt):
					view.Held = append(view.Held, r)
				}
			case "blocked":
				n := node(where)
				if e, ok := lastEvent(it, "block"); ok && e.On != "" {
					slug, id := t.Slug, e.On
					if m := qualifiedID.FindStringSubmatch(e.On); m != nil {
						slug, id = m[1], m[2]
					}
					target := ""
					done := false
					for _, tt := range ts {
						if x, err := itemByID(tt, id); err == nil && tt.Slug == slug {
							target, done = itemNode(tt, *x), x.Status == "done"
							if target == "" {
								target = routeOf(tt, *x)
							}
						}
					}
					if target == "" {
						target = "any"
					}
					r.Why = "waiting on " + target + " " + slug + ":" + id
					if done {
						r.Why = slug + ":" + id + " is done: unblock this row"
					} else if at := parseNano(e.At); !at.IsZero() && now.Sub(at) > stallFor {
						s := r
						s.Why = "unanswered by " + target + " for " + ago(e.At)
						n.Stalled = append(n.Stalled, s)
					}
				}
				n.Blocked = append(n.Blocked, r)
			case "ready":
				if it.Kind == "prompt" || it.Kind == "story" || inDrive(t, it) {
					continue
				}
				rt := routeOf(t, it)
				if rt == "any" {
					view.Any.Ready++
					view.Any.Cost += it.Cost
					continue
				}
				n := node(rt)
				n.Ready++
				n.ReadyCost += it.Cost
				if until, out := outUntil(rt); out {
					r.Why = "queued for " + rt + " at " + until
					n.Queued = append(n.Queued, r)
				} else if silent(rt) {
					r.Why = "node silent"
					view.Held = append(view.Held, r)
				}
			}
		}
	}
	names := make([]string, 0, len(nodes))
	for k := range nodes {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		n := nodes[name]
		if name != unknownNode && !n.Out && !n.Low { // out or nearly out: finish what is in flight, take nothing new
			if it := nextFor(w, ts, name); it != nil {
				t, _ := tenantBySlug(w, it.Parent)
				orig, _ := itemByID(t, it.ID)
				r := row(t, *orig)
				n.Next = &r
			}
		}
		view.Nodes = append(view.Nodes, *n)
	}
	if view.Nodes == nil {
		view.Nodes = []fleetNode{}
	}
	if it := nextFor(w, ts, "any"); it != nil && view.Any.Ready > 0 {
		t, _ := tenantBySlug(w, it.Parent)
		orig, _ := itemByID(t, it.ID)
		r := row(t, *orig)
		view.Any.Next = &r
	}
	return view
}

func silentBeatAt(nb nodeBeat, now time.Time, d time.Duration) bool {
	at, err := time.Parse(time.RFC3339, nb.At)
	return err != nil || now.Sub(at) > d
}

// signature is the view without what changes by itself (ages, beat times): equal signatures mean no news.
func (v fleetView) signature() string {
	c := v
	c.Nodes = append([]fleetNode(nil), v.Nodes...)
	clean := func(rs []fleetRow) []fleetRow {
		out := make([]fleetRow, len(rs))
		for i, r := range rs {
			r.Age = ""
			if i := strings.Index(r.Why, " for "); i > 0 { // the age in a reason changes by itself
				r.Why = r.Why[:i]
			}
			r.Last = "" // text with an age in it; the entry's own time (At) says whether it changed
			out[i] = r
		}
		return out
	}
	for i := range c.Nodes {
		n := c.Nodes[i]
		n.Beat = ""
		n.InFlight, n.Stalled, n.Blocked, n.Waiting, n.Queued = clean(n.InFlight), clean(n.Stalled), clean(n.Blocked), clean(n.Waiting), clean(n.Queued)
		if n.Next != nil {
			x := clean([]fleetRow{*n.Next})[0]
			n.Next = &x
		}
		c.Nodes[i] = n
	}
	c.Movable, c.Held = clean(v.Movable), clean(v.Held)
	if v.Any.Next != nil {
		x := clean([]fleetRow{*v.Any.Next})[0]
		c.Any.Next = &x
	}
	b, _ := json.Marshal(c)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func fleetStatePath(path, session string) (string, error) {
	p, err := userConfigPath()
	if err != nil {
		return "", err
	}
	abs, _ := filepath.Abs(path)
	h := sha256.Sum256([]byte(abs + "\x00" + session)) // one state per coordinator session
	return filepath.Join(filepath.Dir(p), "fleet-"+hex.EncodeToString(h[:6])+".json"), nil
}

func fleetCommand(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return fmt.Errorf("unexpected argument %q", pos[0])
	}
	if err := allowOptions(o, "json", "tenant", "changed", "session"); err != nil {
		return err
	}
	if o["tenant"] != "" {
		if _, err := tenantBySlug(w, o["tenant"]); err != nil {
			return err
		}
	}
	v := buildFleet(path, w, o, nowFn())
	if o["changed"] == "true" {
		sp, err := fleetStatePath(path, value(o, "session", os.Getenv("MW_SESSION")))
		if err != nil {
			return err
		}
		sig := v.signature()
		if b, err := fsx.ReadFile(sp); err == nil && strings.TrimSpace(string(b)) == sig {
			fmt.Println("no change")
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(sp), 0o700); err != nil {
			return err
		}
		if err := writeFileAtomic(sp, []byte(sig+"\n")); err != nil {
			return err
		}
	}
	if o["json"] == "true" {
		return printJSON(v)
	}
	printFleet(v)
	return nil
}

func costText(c int) string {
	if c == 0 {
		return ""
	}
	return fmt.Sprintf(" cost %dk", c)
}

func printFleetRows(label string, rs []fleetRow) {
	for _, r := range rs {
		fmt.Printf("  %-8s %s %s", label, r.Item, r.Title)
		if r.Age != "" {
			fmt.Printf("  %s", r.Age)
		}
		if r.Why != "" {
			fmt.Printf("  (%s)", r.Why)
		}
		if r.Last != "" {
			fmt.Printf("  last: %s", r.Last)
		}
		fmt.Print(costText(r.Cost), "\n")
	}
}

func printFleet(v fleetView) {
	if len(v.Nodes) == 0 && v.Any.Ready == 0 {
		fmt.Println("no nodes and nothing ready (mw node set NAME, mw node beat)")
		return
	}
	for _, n := range v.Nodes {
		st := "live"
		if !n.Live {
			st = "SILENT"
		}
		fmt.Printf("%s %s %s", n.Node, st, n.State)
		if n.Beat != "" {
			fmt.Printf(" %s", ago(n.Beat))
		}
		if n.Left != nil {
			fmt.Printf(" left %d%%", *n.Left)
			if n.Low {
				fmt.Print(" LOW")
			}
		}
		if n.Out {
			fmt.Printf(" OUT until %s", orDefault(n.OutUntil, "?"))
		}
		fmt.Printf("  sessions %d ready %d%s doing %d%s\n", n.Sessions, n.Ready, costText(n.ReadyCost), len(n.InFlight)+len(n.Stalled), costText(n.DoingCost))
		printFleetRows("doing", n.InFlight)
		printFleetRows("STALLED", n.Stalled)
		printFleetRows("blocked", n.Blocked)
		printFleetRows("waiting", n.Waiting)
		printFleetRows("queued", n.Queued)
		if n.Next != nil {
			fmt.Printf("  next     %s %s%s\n", n.Next.Item, n.Next.Title, costText(n.Next.Cost))
		}
	}
	if v.Any.Ready > 0 {
		fmt.Printf("anyone: ready %d%s", v.Any.Ready, costText(v.Any.Cost))
		if v.Any.Next != nil {
			fmt.Printf("  next %s %s", v.Any.Next.Item, v.Any.Next.Title)
		}
		fmt.Println()
	}
	printFleetRows("movable", v.Movable)
	printFleetRows("held", v.Held)
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
