package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// archiveDoc is .miniwaves/<slug>.archive.json: a tenant's done items (with their
// events) moved out of its file so every command loads less.
type archiveDoc struct {
	Version int    `json:"version"`
	Items   []Item `json:"items"`
}

func loadArchive(boardPath, slug string) (*archiveDoc, error) {
	b, err := readFileShared(archivePath(boardPath, slug))
	if err != nil {
		if os.IsNotExist(err) {
			return &archiveDoc{Version: currentSchema}, nil
		}
		return nil, err
	}
	var a archiveDoc
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, fmt.Errorf("read %s: %w", archivePath(boardPath, slug), err)
	}
	return &a, nil
}

// archivedItem finds one item of a tenant in the archive (read-only).
func archivedItem(boardPath, slug, id string) (Item, bool) {
	a, err := loadArchive(boardPath, slug)
	if err != nil {
		return Item{}, false
	}
	id = strings.ToUpper(id)
	for _, it := range a.Items {
		if it.ID == id {
			return it, true
		}
	}
	return Item{}, false
}

// archivable returns the indexes of done items older than cutoff that may
// leave the board: never waves, and never a parent with a child that stays.
func archivable(t *Tenant, cutoff time.Time) map[string]bool {
	cand := map[string]bool{}
	for _, it := range t.Items {
		if it.Status != "done" || it.Kind == "wave" {
			continue
		}
		if u := parseTS(it.Updated); u.IsZero() || !u.Before(cutoff) {
			continue
		}
		cand[it.ID] = true
	}
	for changed := true; changed; {
		changed = false
		for _, it := range t.Items {
			if it.Parent != "" && !cand[it.ID] && cand[it.Parent] {
				delete(cand, it.Parent)
				changed = true
			}
		}
	}
	return cand
}

func archiveCommand(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return fmt.Errorf("unexpected argument %q", pos[0])
	}
	if err := allowOptions(o, "tenant", "days", "dry-run"); err != nil {
		return err
	}
	days := 30
	if o["days"] != "" {
		if _, e := fmt.Sscanf(o["days"], "%d", &days); e != nil || days < 0 {
			return errors.New("--days needs a number of days (0 or more)")
		}
	}
	var tenants []*Tenant
	if o["tenant"] != "" {
		t, err := tenantBySlug(w, o["tenant"])
		if err != nil {
			return err
		}
		tenants = []*Tenant{t}
	} else {
		for i := range w.Tenants {
			if !w.Tenants[i].Hidden {
				tenants = append(tenants, &w.Tenants[i])
			}
		}
	}
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	dry := o["dry-run"] == "true"
	archives := map[string]*archiveDoc{}
	total := 0
	var parts []string
	for _, t := range tenants {
		cand := archivable(t, cutoff)
		if len(cand) == 0 {
			continue
		}
		total += len(cand)
		parts = append(parts, fmt.Sprintf("%s %d", t.Slug, len(cand)))
		if dry {
			continue
		}
		var moved, kept []Item
		for _, it := range t.Items {
			if cand[it.ID] {
				moved = append(moved, it)
			} else {
				kept = append(kept, it)
			}
		}
		at, err := loadArchive(path, t.Slug)
		if err != nil {
			return err
		}
		archives[t.Slug] = at
		seen := map[string]bool{}
		for _, it := range at.Items {
			seen[it.ID] = true
		}
		for _, it := range moved {
			if !seen[it.ID] {
				at.Items = append(at.Items, it)
			}
		}
		t.Items = kept
		t.Archived += len(moved)
	}
	verb := "archived"
	if dry {
		verb = "would archive"
	}
	if total == 0 {
		fmt.Printf("nothing to archive (done items older than %d days)\n", days)
		return nil
	}
	if !dry {
		if err := os.MkdirAll(filepath.Dir(archivePath(path, "x")), 0o755); err != nil {
			return err
		}
		for slug, a := range archives {
			b, err := json.Marshal(a)
			if err != nil {
				return err
			}
			if err := writeFileAtomic(archivePath(path, slug), append(b, '\n')); err != nil {
				return err
			}
		}
		if err := save(path, w); err != nil {
			return err
		}
	}
	fmt.Printf("%s %d done item(s) older than %d days (%s)\n", verb, total, days, strings.Join(parts, ", "))
	return nil
}
