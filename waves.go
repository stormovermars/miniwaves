package main

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

//go:embed PROTOCOL.md
var protocolMD string

// defaultProtocol is parsed from PROTOCOL.md; used unless the board JSON has a top-level "protocol" list.
var defaultProtocol = parseProtocol(protocolMD)

// parseProtocol reads "- key | title" lines.
func parseProtocol(md string) []Step {
	var steps []Step
	for _, line := range strings.Split(md, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		key, title, ok := strings.Cut(strings.TrimPrefix(line, "- "), "|")
		if !ok {
			panic("PROTOCOL.md: expected '- key | title', got: " + line)
		}
		k := strings.TrimSpace(key)
		if k == "" {
			panic("PROTOCOL.md: empty key in: " + line)
		}
		for _, s := range steps {
			if s.Key == k {
				panic("PROTOCOL.md: duplicate key " + k)
			}
		}
		steps = append(steps, Step{Key: k, Title: strings.TrimSpace(title)})
	}
	if len(steps) == 0 {
		panic("PROTOCOL.md: no steps")
	}
	return steps
}

// protocolNamed turns step names into steps (key = lowercase, words joined by dashes; duplicates dropped).
func protocolNamed(names []string) []Step {
	steps := make([]Step, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		key := strings.ToLower(strings.Join(strings.Fields(name), "-"))
		if key != "" && !seen[key] {
			seen[key] = true
			steps = append(steps, Step{Key: key, Title: name})
		}
	}
	return steps
}

// protocolFor resolves the steps a new wave of tenant t gets and where they come from:
// the tenant's own list, else the workspace list, else the embedded PROTOCOL.md.
func protocolFor(w *Workspace, t *Tenant) ([]Step, string) {
	if t != nil {
		if steps := protocolNamed(t.Protocol); len(steps) > 0 {
			return steps, "tenant"
		}
	}
	if steps := protocolNamed(w.Protocol); len(steps) > 0 {
		return steps, "workspace"
	}
	return append([]Step(nil), defaultProtocol...), "default"
}

// protocolSteps returns a fresh step list for a new wave of tenant t.
func protocolSteps(w *Workspace, t *Tenant) []Step {
	steps, _ := protocolFor(w, t)
	return steps
}

type waveRow struct {
	ID      string `json:"id" toon:"id"`
	Title   string `json:"title" toon:"title"`
	State   string `json:"state" toon:"state"`
	Members string `json:"members" toon:"members"`
	Percent int    `json:"percent" toon:"percent"`
	Blocked int    `json:"blocked" toon:"blocked"`
	Steps   string `json:"steps" toon:"steps"`
}

type waveDetail struct {
	waveRow
	Parent      string      `json:"parent,omitempty" toon:"parent"`
	MemberItems []itemBrief `json:"member_items" toon:"member_items"`
	Checklist   []Step      `json:"checklist" toon:"checklist"`
	Note        string      `json:"note,omitempty" toon:"note"`
}

func waveMembers(t *Tenant, id string) []Item {
	var out []Item
	for _, it := range t.Items {
		if it.Wave == id {
			out = append(out, it)
		}
	}
	return out
}

func waveRowOf(t *Tenant, wv Item) waveRow {
	members := waveMembers(t, wv.ID)
	done, blocked := 0, 0
	for _, m := range members {
		if m.Status == "done" {
			done++
		}
		if m.Status == "blocked" {
			blocked++
		}
	}
	pct := 0
	if len(members) > 0 {
		pct = done * 100 / len(members)
	}
	sd := 0
	for _, s := range wv.Steps {
		if s.Done {
			sd++
		}
	}
	return waveRow{wv.ID, compactText(wv.Title, 72), wv.Status, fmt.Sprintf("%d/%d", done, len(members)), pct, blocked, fmt.Sprintf("%d/%d", sd, len(wv.Steps))}
}

func waveByID(t *Tenant, id string) (*Item, error) {
	it, err := itemByID(t, id)
	if err != nil {
		return nil, err
	}
	if it.Kind != "wave" {
		return nil, fmt.Errorf("%s is not a wave", it.ID)
	}
	return it, nil
}

// wouldCycle reports whether making parent the parent of id creates a loop.
func wouldCycle(t *Tenant, id, parent string) bool {
	cur := parent
	for i := 0; cur != "" && i <= len(t.Items); i++ {
		if cur == id {
			return true
		}
		p, err := itemByID(t, cur)
		if err != nil {
			return false
		}
		cur = p.Parent
	}
	return false
}

// checkMember validates putting it into wave wv.
func checkMember(it *Item, wv *Item) error {
	if it.Kind == "wave" {
		return errors.New("waves nest with mw wave create -p, not as members")
	}
	if wv.Status == "done" {
		return fmt.Errorf("%s is closed; reopen it first", wv.ID)
	}
	if it.Wave != "" && it.Wave != wv.ID {
		return fmt.Errorf("%s is already in %s; remove from %s first", it.ID, it.Wave, it.Wave)
	}
	return nil
}

// checkParent enforces the kind hierarchy and cycle rule for it under parent.
func checkParent(t *Tenant, it *Item, parent string) error {
	if parent == "" {
		return nil
	}
	p, err := itemByID(t, parent)
	if err != nil {
		return fmt.Errorf("parent: %w", err)
	}
	if it.Kind == "wave" {
		if p.Kind != "wave" {
			return fmt.Errorf("a wave can only be a child of another wave, not %s %s", p.Kind, p.ID)
		}
		if p.Status == "done" {
			return fmt.Errorf("%s is closed; reopen it first", p.ID)
		}
	} else if p.Kind == "wave" {
		return fmt.Errorf("%s is a wave; wave membership is not a parent (use --wave / mw wave add)", p.ID)
	} else if it.Kind == "prompt" || (it.Kind == "story" && p.Kind != "prompt") ||
		(it.Kind != "prompt" && it.Kind != "story" && p.Kind != "story") {
		return fmt.Errorf("invalid hierarchy: %s cannot be parent of %s", p.Kind, it.Kind)
	}
	if wouldCycle(t, it.ID, p.ID) {
		return fmt.Errorf("cycle: %s cannot be placed under %s (it is that item's own descendant or itself)", it.ID, p.ID)
	}
	return nil
}

func waveCommand(path string, w *Workspace, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: mw wave create|add|remove|step|unstep|show|list|close (mw help wave)")
	}
	cmd := args[0]
	o, pos, err := options(args[1:])
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	switch cmd {
	case "list":
		if len(pos) != 0 {
			return fmt.Errorf("unexpected argument %q", pos[0])
		}
		if err := allowOptions(o, "tenant", "session", "all", "json"); err != nil {
			return err
		}
		var tenants []*Tenant
		if o["all"] == "true" {
			for i := range w.Tenants {
				if !w.Tenants[i].Hidden {
					tenants = append(tenants, &w.Tenants[i])
				}
			}
		} else {
			t, err := selectTenant(w, o)
			if err != nil {
				return err
			}
			tenants = []*Tenant{t}
		}
		rows := make([]waveRow, 0)
		for _, t := range tenants {
			for _, it := range t.Items {
				if it.Kind == "wave" {
					r := waveRowOf(t, it)
					if o["all"] == "true" {
						r.ID = t.Slug + ":" + r.ID
					}
					rows = append(rows, r)
				}
			}
		}
		return printData(rows, o)
	case "create":
		if len(pos) < 1 {
			return errors.New("usage: mw wave create TITLE... [-t SLUG] [-p WAVE] [-b TEXT]")
		}
		if err := allowOptions(o, "tenant", "session", "parent", "body"); err != nil {
			return err
		}
		t, err := selectTenant(w, o)
		if err != nil {
			return err
		}
		parent := strings.ToUpper(o["parent"])
		if parent != "" {
			if err := checkParent(t, &Item{Kind: "wave"}, parent); err != nil {
				return err
			}
		}
		t.Next["WAV"]++
		steps := protocolSteps(w, t)
		it := Item{ID: fmt.Sprintf("WAV-%03d", t.Next["WAV"]), Kind: "wave", Title: strings.Join(pos, " "),
			Status: "ready", Parent: parent, Body: o["body"], Steps: steps, Created: now, Updated: now}
		emit(&it, now, "created", os.Getenv("MW_SESSION"), "")
		t.Items = append(t.Items, it)
		if err := save(path, w); err != nil {
			return err
		}
		fmt.Printf("[%s] %s  %-7s %s\n", t.Slug, it.ID, it.Status, compactText(it.Title, 72))
		return nil
	}
	// remaining commands: WAVE first
	if len(pos) < 1 {
		return fmt.Errorf("usage: mw wave %s WAVE ...", cmd)
	}
	t, err := selectTenant(w, o)
	if err != nil {
		return err
	}
	wv, err := waveByID(t, pos[0])
	if err != nil {
		return err
	}
	switch cmd {
	case "add", "remove":
		if err := allowOptions(o, "tenant", "session"); err != nil {
			return err
		}
		if len(pos) < 2 {
			return fmt.Errorf("usage: mw wave %s WAVE ID...", cmd)
		}
		var targets []*Item
		for _, id := range pos[1:] {
			it, err := itemByID(t, id)
			if err != nil {
				return err
			}
			if cmd == "add" {
				if err := checkMember(it, wv); err != nil {
					return err
				}
			} else if wv.Status == "done" {
				return fmt.Errorf("%s is closed; reopen it first", wv.ID)
			} else if it.Wave != wv.ID {
				return fmt.Errorf("%s is not a member of %s", it.ID, wv.ID)
			}
			targets = append(targets, it)
		}
		for _, it := range targets {
			if cmd == "add" {
				it.Wave = wv.ID
			} else {
				it.Wave = ""
			}
			it.Updated = now
			emit(it, now, "wave-"+cmd, value(o, "session", os.Getenv("MW_SESSION")), wv.ID)
		}
		touchSession(w, value(o, "session", os.Getenv("MW_SESSION")), now)
		if err := save(path, w); err != nil {
			return err
		}
		fmt.Printf("[%s] %s members: %s\n", t.Slug, wv.ID, waveRowOf(t, *wv).Members)
		return nil
	case "step", "unstep":
		if wv.Status == "done" {
			return fmt.Errorf("%s is closed; reopen it first", wv.ID)
		}
		if cmd == "unstep" {
			if err := allowOptions(o, "tenant", "session"); err != nil {
				return err
			}
			if len(pos) != 2 {
				return errors.New("usage: mw wave unstep WAVE KEY")
			}
		} else if err := allowOptions(o, "tenant", "session", "na", "na-rest"); err != nil {
			return err
		}
		if cmd == "step" && o["na-rest"] != "" {
			if len(pos) != 1 || o["na"] != "" {
				return errors.New("usage: mw wave step WAVE --na-rest REASON")
			}
			n := 0
			for i := range wv.Steps {
				if !wv.Steps[i].Done {
					wv.Steps[i].Done, wv.Steps[i].NA = true, true
					wv.Steps[i].Note, wv.Steps[i].When = o["na-rest"], now
					n++
				}
			}
			wv.Updated = now
			emit(wv, now, "step", value(o, "session", os.Getenv("MW_SESSION")), fmt.Sprintf("%d steps n/a: %s", n, o["na-rest"]))
			touchSession(w, value(o, "session", os.Getenv("MW_SESSION")), now)
			if err := save(path, w); err != nil {
				return err
			}
			fmt.Printf("[%s] %s %d steps n/a (%s)\n", t.Slug, wv.ID, n, waveRowOf(t, *wv).Steps)
			return nil
		}
		if cmd == "step" && len(pos) < 2 {
			return errors.New("usage: mw wave step WAVE KEY=NOTE [KEY=NOTE...] [--na]  (a note per step is required)")
		}
		who := value(o, "session", os.Getenv("MW_SESSION"))
		type tick struct{ key, note string }
		var ticks []tick
		if cmd == "unstep" {
			ticks = append(ticks, tick{strings.ToLower(pos[1]), ""})
		} else {
			for _, a := range pos[1:] {
				k, n, ok := strings.Cut(a, "=")
				if !ok || strings.TrimSpace(n) == "" || strings.TrimSpace(k) == "" {
					return fmt.Errorf("step %q: expected KEY=NOTE (a note is required)", a)
				}
				ticks = append(ticks, tick{strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(n)})
			}
		}
		idx := make([]int, len(ticks))
		for n, tk := range ticks {
			idx[n] = -1
			for i := range wv.Steps {
				if wv.Steps[i].Key == tk.key {
					idx[n] = i
				}
			}
			if idx[n] < 0 {
				keys := make([]string, len(wv.Steps))
				for i, s := range wv.Steps {
					keys[i] = s.Key
				}
				return fmt.Errorf("%s has no step %q (steps: %s)", wv.ID, tk.key, strings.Join(keys, ", "))
			}
		}
		var marks []string
		for n, tk := range ticks {
			i := idx[n]
			mark := "done"
			if cmd == "unstep" {
				wv.Steps[i] = Step{Key: tk.key, Title: wv.Steps[i].Title}
				mark = "unticked"
				emit(wv, now, "unstep", who, tk.key)
			} else {
				wv.Steps[i].Done, wv.Steps[i].NA = true, o["na"] == "true"
				wv.Steps[i].Note, wv.Steps[i].When = tk.note, now
				if wv.Steps[i].NA {
					mark = "n/a"
				}
				emit(wv, now, "step", who, tk.key+": "+tk.note)
			}
			marks = append(marks, tk.key+" "+mark)
		}
		wv.Updated = now
		touchSession(w, who, now)
		if err := save(path, w); err != nil {
			return err
		}
		fmt.Printf("[%s] %s step %s (%s)\n", t.Slug, wv.ID, strings.Join(marks, ", "), waveRowOf(t, *wv).Steps)
		return nil
	case "show":
		if err := allowOptions(o, "tenant", "session", "json"); err != nil {
			return err
		}
		if len(pos) != 1 {
			return errors.New("usage: mw wave show WAVE [-j]")
		}
		members := waveMembers(t, wv.ID)
		briefs := make([]itemBrief, 0, len(members))
		for _, m := range members {
			briefs = append(briefs, itemBrief{m.ID, m.Kind, m.Status, compactText(m.Title, 72), m.Ticket})
		}
		r := waveRowOf(t, *wv)
		if o["json"] == "true" {
			return printJSON(waveDetail{r, wv.Parent, briefs, wv.Steps, wv.Note})
		}
		fmt.Printf("%s  %s  [%s]\n", wv.ID, compactText(wv.Title, 72), wv.Status)
		fmt.Printf("members %s (%d%%), %d blocked\n", r.Members, r.Percent, r.Blocked)
		for _, m := range briefs {
			mark := " "
			if m.State == "done" {
				mark = "x"
			}
			fmt.Printf("  [%s] %-8s %-7s %-7s %s\n", mark, m.ID, m.Type, m.State, m.Title)
		}
		fmt.Printf("checklist %s\n", r.Steps)
		for _, s := range wv.Steps {
			mark, note := " ", ""
			if s.Done {
				mark = "x"
			}
			if s.NA {
				mark = "-"
			}
			if s.Note != "" {
				note = " — " + compactText(s.Note, 100)
				if s.NA {
					note = " — n/a: " + compactText(s.Note, 100)
				}
			}
			fmt.Printf("  [%s] %-10s %s%s\n", mark, s.Key, s.Title, note)
		}
		if wv.Note != "" {
			fmt.Println(wv.Note)
		}
		for _, n := range wv.Notes {
			fmt.Println("note", n)
		}
		return nil
	case "close":
		if err := allowOptions(o, "tenant", "session", "no-evidence"); err != nil {
			return err
		}
		if len(pos) != 1 {
			return errors.New("usage: mw wave close WAVE [--no-evidence REASON]")
		}
		noEvidence, err := noEvidenceReason(o)
		if err != nil {
			return err
		}
		if wv.Status == "done" {
			return fmt.Errorf("%s is already closed", wv.ID)
		}
		var missing []string
		for _, m := range waveMembers(t, wv.ID) {
			if m.Status != "done" {
				missing = append(missing, fmt.Sprintf("member %s is %s", m.ID, m.Status))
			}
		}
		if gateOn(w, t, "judge") {
			var unjudged []string
			for _, m := range waveMembers(t, wv.ID) {
				if m.Status == "done" && !isJudged(t, m) {
					unjudged = append(unjudged, m.ID)
				}
			}
			if len(unjudged) > 0 {
				missing = append(missing, "unjudged members (mw accept ID): "+strings.Join(unjudged, ", "))
			}
		}
		for _, c := range t.Items {
			if c.Kind == "wave" && c.Parent == wv.ID && c.Status != "done" {
				missing = append(missing, fmt.Sprintf("child wave %s is %s", c.ID, c.Status))
			}
		}
		for _, s := range wv.Steps {
			if !s.Done {
				missing = append(missing, "step "+s.Key+" is not ticked")
			}
		}
		var evidence []string
		if noEvidence == "" && gateOn(w, t, "evidence") {
			var e error
			if evidence, e = checkEvidence(w, t, wv.ID); e != nil {
				missing = append(missing, e.Error())
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("cannot close %s:\n  %s", wv.ID, strings.Join(missing, "\n  "))
		}
		wv.Evidence = evidence
		if noEvidence != "" {
			wv.Notes = append(wv.Notes, now+" no-evidence: "+noEvidence)
		}
		wv.Status, wv.Updated, wv.Note = "done", now, "closed "+time.Now().Format("2006-01-02")
		emit(wv, now, "done", value(o, "session", os.Getenv("MW_SESSION")), "closed")
		fanfareAfterChange(w, t, true, now)
		if err := save(path, w); err != nil {
			return err
		}
		fmt.Printf("[%s] %s  done    %s\n", t.Slug, wv.ID, wv.Note)
		return nil
	}
	return fmt.Errorf("unknown wave command %q", cmd)
}
