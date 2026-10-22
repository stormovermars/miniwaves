package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Named fanfare sets: "sets" maps a name to pair folder names, "use" is the active set
// ("" or "all" = every playable pair).

var setNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

func activeSet(c FanfareConfig) string {
	if c.Use == "" || c.Use == "all" {
		return "all"
	}
	return c.Use
}

// pickPairFor is pickPair limited to the active set; a set without a playable pair falls back to all.
func pickPairFor(c FanfareConfig, dir string, pick func(int) int) (*pairInfo, error) {
	if use := activeSet(c); use != "all" {
		in := map[string]bool{}
		for _, n := range c.Sets[use] {
			in[n] = true
		}
		var ready []pairInfo
		for _, p := range scanPairs(dir) {
			if p.State == "ready" && in[p.Name] {
				ready = append(ready, p)
			}
		}
		if len(ready) > 0 {
			p := ready[pick(len(ready))]
			return &p, nil
		}
		fmt.Fprintf(os.Stderr, "mw: fanfare: set %q has no playable pair; using all\n", use)
	}
	return pickPair(dir, pick)
}

type setPair struct {
	Name  string `json:"name"`
	State string `json:"state"` // playable, unplayable or missing
}

func setPairStates(names []string, pairs []pairInfo) []setPair {
	st := map[string]string{}
	for _, p := range pairs {
		st[p.Name] = "unplayable"
		if p.State == "ready" {
			st[p.Name] = "playable"
		}
	}
	out := []setPair{}
	for _, n := range names {
		s := st[n]
		if s == "" {
			s = "missing"
		}
		out = append(out, setPair{n, s})
	}
	return out
}

func sortedSetNames(c FanfareConfig) []string {
	var ns []string
	for n := range c.Sets {
		ns = append(ns, n)
	}
	sort.Strings(ns)
	return ns
}

// fanfareStatusJSON is `mw fanfare -j`: the long-standing keys in base plus the set and UI keys (a superset).
func fanfareStatusJSON(base map[string]any, c FanfareConfig, pairs []pairInfo) error {
	type pj struct {
		Name     string `json:"name"`
		Playable bool   `json:"playable"`
	}
	ps := []pj{}
	for _, p := range pairs {
		ps = append(ps, pj{p.Name, p.State == "ready"})
	}
	sets := map[string][]string{}
	for n, v := range c.Sets {
		sets[n] = append([]string{}, v...)
	}
	base["enabled"], base["use"], base["sets"], base["pairs"] = c.Enabled, activeSet(c), sets, ps
	base["milestone_seconds"], base["finale_seconds"], base["volume"] = c.MilestoneSeconds, c.FinaleSeconds, c.Volume
	return printJSON(base)
}

func fanfareSetsCommand(path string, w *Workspace, o map[string]string, pos []string, dir string) error {
	c := fanfareCfg(w)
	pairs := scanPairs(dir)
	if len(pos) == 0 {
		use := activeSet(c)
		if o["json"] == "true" {
			type sj struct {
				Name   string    `json:"name"`
				Active bool      `json:"active"`
				Pairs  []setPair `json:"pairs"`
			}
			out := struct {
				Use  string `json:"use"`
				Sets []sj   `json:"sets"`
			}{use, []sj{}}
			for _, n := range sortedSetNames(c) {
				out.Sets = append(out.Sets, sj{n, n == use, setPairStates(c.Sets[n], pairs)})
			}
			b, err := json.Marshal(out)
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		}
		fmt.Printf("use: %s\n", use)
		for _, n := range sortedSetNames(c) {
			mark := " "
			if n == use {
				mark = "*"
			}
			var ps []string
			for _, p := range setPairStates(c.Sets[n], pairs) {
				ps = append(ps, p.Name+" ("+p.State+")")
			}
			fmt.Printf("%s %s: %s\n", mark, n, strings.Join(ps, ", "))
		}
		return nil
	}
	if o["json"] == "true" {
		return errors.New("--json is only valid with `mw fanfare sets`")
	}
	if w.Fanfare == nil {
		w.Fanfare = &FanfareConfig{}
	}
	f := w.Fanfare
	switch pos[0] {
	case "add":
		if len(pos) < 3 {
			return errors.New("usage: mw fanfare sets add NAME PAIR...")
		}
		name := pos[1]
		if name == "all" || !setNameRe.MatchString(name) {
			return fmt.Errorf("set name %q: want [a-z0-9][a-z0-9-]{0,31} and not \"all\"", name)
		}
		have := map[string]bool{}
		for _, p := range pairs {
			have[p.Name] = true
		}
		for _, n := range pos[2:] {
			if !have[n] {
				return fmt.Errorf("unknown pair %q (not a folder in %s; mw fanfare list)", n, dir)
			}
		}
		if f.Sets == nil {
			f.Sets = map[string][]string{}
		}
		cur := f.Sets[name]
		in := map[string]bool{}
		for _, n := range cur {
			in[n] = true
		}
		for _, n := range pos[2:] {
			if !in[n] {
				in[n] = true
				cur = append(cur, n)
			}
		}
		f.Sets[name] = cur
		if err := save(path, w); err != nil {
			return err
		}
		fmt.Printf("set %s: %s\n", name, strings.Join(cur, ", "))
		return nil
	case "drop":
		if len(pos) < 2 {
			return errors.New("usage: mw fanfare sets drop NAME [PAIR...]")
		}
		name := pos[1]
		cur, ok := f.Sets[name]
		if !ok {
			return fmt.Errorf("unknown set %q", name)
		}
		if len(pos) == 2 {
			delete(f.Sets, name)
			if f.Use == name {
				f.Use = ""
			}
			if err := save(path, w); err != nil {
				return err
			}
			fmt.Printf("dropped set %s\n", name)
			return nil
		}
		in := map[string]bool{}
		for _, n := range cur {
			in[n] = true
		}
		drop := map[string]bool{}
		for _, n := range pos[2:] {
			if !in[n] {
				return fmt.Errorf("pair %q is not in set %s", n, name)
			}
			drop[n] = true
		}
		keep := []string{}
		for _, n := range cur {
			if !drop[n] {
				keep = append(keep, n)
			}
		}
		f.Sets[name] = keep
		if err := save(path, w); err != nil {
			return err
		}
		fmt.Printf("set %s: %s\n", name, strings.Join(keep, ", "))
		return nil
	}
	return errors.New("usage: mw fanfare sets [-j] | sets add NAME PAIR... | sets drop NAME [PAIR...]")
}

func fanfareUseCommand(path string, w *Workspace, pos []string) error {
	if len(pos) != 1 {
		return errors.New("usage: mw fanfare use NAME|all")
	}
	if w.Fanfare == nil {
		w.Fanfare = &FanfareConfig{}
	}
	name := pos[0]
	if name == "all" {
		w.Fanfare.Use = ""
	} else if _, ok := w.Fanfare.Sets[name]; !ok {
		return fmt.Errorf("unknown set %q (mw fanfare sets)", name)
	} else {
		w.Fanfare.Use = name
	}
	if err := save(path, w); err != nil {
		return err
	}
	fmt.Printf("fanfare use: %s\n", name)
	return nil
}
