package main

import (
	"errors"
	"fmt"
	"strings"
)

// tenantProtocol runs mw tenant protocol SLUG [set STEP... | reset] and the --workspace
// forms (mw tenant protocol --workspace [set STEP... | reset]).
func tenantProtocol(path string, w *Workspace, args []string) error {
	const use = "usage: mw tenant protocol SLUG [set STEP... | reset]  |  mw tenant protocol --workspace [set STEP... | reset]"
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if err := allowOptions(o, "workspace", "json"); err != nil {
		return err
	}
	var t *Tenant
	if o["workspace"] != "true" {
		if len(pos) < 1 {
			return errors.New(use)
		}
		if t, err = tenantBySlug(w, pos[0]); err != nil {
			return err
		}
		pos = pos[1:]
	}
	verb := ""
	if len(pos) > 0 {
		verb, pos = pos[0], pos[1:]
	}
	switch verb {
	case "":
		if len(pos) != 0 {
			return errors.New(use)
		}
		steps, source := protocolFor(w, t)
		if o["json"] == "true" {
			return printJSON(map[string]any{"source": source, "steps": steps})
		}
		fmt.Println("source:", source)
		for _, s := range steps {
			fmt.Printf("  %-12s %s\n", s.Key, s.Title)
		}
		return nil
	case "set":
		steps := protocolNamed(pos)
		if len(steps) == 0 {
			return errors.New("set needs at least one step name (mw tenant protocol ... reset drops the list)")
		}
		names := make([]string, len(steps))
		for i, s := range steps {
			names[i] = s.Title
		}
		if t != nil {
			t.Protocol = names
		} else {
			w.Protocol = names
		}
	case "reset":
		if len(pos) != 0 {
			return errors.New(use)
		}
		if t != nil {
			t.Protocol = nil
		} else {
			w.Protocol = nil
		}
	default:
		return errors.New(use)
	}
	if err := save(path, w); err != nil {
		return err
	}
	steps, source := protocolFor(w, t)
	keys := make([]string, len(steps))
	for i, s := range steps {
		keys[i] = s.Key
	}
	scope := "workspace"
	if t != nil {
		scope = t.Slug
	}
	fmt.Printf("%s protocol %s: now %s (%s); applies to waves created from now on\n", scope, verb, source, strings.Join(keys, ", "))
	return nil
}
