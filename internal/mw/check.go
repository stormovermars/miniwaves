package mw

import (
	"fmt"
	"strings"
)

type staleParent struct {
	ID       string   `json:"id" toon:"id"`
	State    string   `json:"state" toon:"state"`
	Children []string `json:"children" toon:"children"`
	Why      string   `json:"why" toon:"why"`
}

// checkCommand: mw check [-t SLUG] [-j] lists parents still in inbox or ready while a child is doing or done.
func checkCommand(w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return fmt.Errorf("unexpected argument %q", pos[0])
	}
	if err := allowOptions(o, "tenant", "json"); err != nil {
		return err
	}
	t, err := selectTenant(w, o)
	if err != nil {
		return err
	}
	moved := map[string][]string{}
	for _, c := range t.Items {
		if c.Parent != "" && (c.Status == "doing" || c.Status == "done") {
			moved[c.Parent] = append(moved[c.Parent], c.ID)
		}
	}
	out := []staleParent{}
	for _, it := range t.Items {
		if kids := moved[it.ID]; len(kids) > 0 && (it.Status == "inbox" || it.Status == "ready") {
			out = append(out, staleParent{it.ID, it.Status, kids, fmt.Sprintf("%s while %s doing or done", it.Status, strings.Join(kids, ","))})
		}
	}
	return printData(out, o)
}
