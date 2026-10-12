package main

import (
	"errors"
	"fmt"
	"html"
	"sort"
	"strings"
)

// Tenant archive and tags live in the index: an archived tenant keeps its files but is
// hidden from every multi-tenant view and refused by single-tenant commands.

// shown is the tenants a multi-tenant view lists: archived ones only with --archived,
// and with --tag (repeatable, AND) only tenants carrying every tag.
func shown(w *Workspace, o map[string]string) []*Tenant {
	var out []*Tenant
	for i := range w.Tenants {
		if t := &w.Tenants[i]; (!t.Hidden || o["archived"] == "true") && hasTags(t, o["tag"]) {
			out = append(out, t)
		}
	}
	return out
}

func hasTags(t *Tenant, want string) bool {
	for _, x := range strings.Split(want, ",") {
		if x = strings.ToLower(strings.TrimSpace(x)); x != "" && !contains(t.Tags, x) {
			return false
		}
	}
	return true
}

func contains(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}

// tagText is " #a #b" for text views ("" without tags), plus " (archived)" when hidden.
func tagText(t *Tenant) string {
	s := ""
	for _, x := range t.Tags {
		s += " #" + x
	}
	if t.Hidden {
		s += " (archived)"
	}
	return s
}

// tagChips is the html page form of the tags.
func tagChips(t *Tenant) string {
	s := ""
	for _, x := range t.Tags {
		s += " <span class=tag>" + html.EscapeString(x) + "</span>"
	}
	return s
}

func refuseArchived(t *Tenant) error {
	if t.Hidden {
		return fmt.Errorf("%s is archived: mw tenant restore %s", t.Slug, t.Slug)
	}
	return nil
}

// liveWork names the doing items and the not-stopped sessions of a tenant.
func liveWork(w *Workspace, t *Tenant) []string {
	var out []string
	for _, it := range t.Items {
		if it.Status == "doing" {
			out = append(out, "item "+it.ID+" is doing")
		}
	}
	for _, s := range w.Sessions {
		if s.Tenant == t.Slug && s.State != "stopped" {
			out = append(out, "session "+s.ID+" is "+s.State)
		}
	}
	return out
}

// tenantMeta runs mw tenant archive [--force]|restore SLUG and mw tenant tag SLUG +a -b.
func tenantMeta(path string, w *Workspace, args []string) error {
	force := false
	if len(args) > 0 && args[0] == "archive" {
		var rest []string
		for _, a := range args {
			if a == "--force" {
				force = true
			} else {
				rest = append(rest, a)
			}
		}
		args = rest
	}
	if len(args) < 2 || args[0] != "tag" && len(args) != 2 {
		return errors.New("usage: mw tenant archive|restore SLUG, mw tenant tag SLUG +TAG -TAG...")
	}
	t, err := tenantBySlug(w, args[1])
	if err != nil {
		return err
	}
	switch args[0] {
	case "archive", "restore":
		if args[0] == "archive" && !force {
			if live := liveWork(w, t); len(live) > 0 {
				return fmt.Errorf("%s has live work: %s; finish or stop it, or use --force", t.Slug, strings.Join(live, ", "))
			}
		}
		t.Hidden = args[0] == "archive"
	case "tag":
		for _, a := range args[2:] {
			x := strings.ToLower(strings.TrimLeft(a, "+-"))
			if x == "" || !validID(x) || a[0] != '+' && a[0] != '-' {
				return errors.New("tags are +TAG or -TAG (lowercase letters, numbers, dashes)")
			}
			if a[0] == '-' {
				t.Tags = remove(t.Tags, x)
			} else if !contains(t.Tags, x) {
				t.Tags = append(t.Tags, x)
			}
		}
		sort.Strings(t.Tags)
	}
	if err := save(path, w); err != nil {
		return err
	}
	if args[0] == "tag" {
		fmt.Printf("%s tags: %s\n", t.Slug, strings.Join(t.Tags, " "))
	} else {
		fmt.Printf("%s %sd\n", t.Slug, args[0])
	}
	return nil
}

func remove(s []string, x string) []string {
	var out []string
	for _, v := range s {
		if v != x {
			out = append(out, v)
		}
	}
	return out
}
