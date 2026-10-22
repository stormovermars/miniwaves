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

// migrateCommand is the only code that reads old schemas. See MIGRATION.md for
// the same steps written for a human or an assistant.
func migrateCommand(args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if err := allowOptions(o, "dry-run"); err != nil {
		return err
	}
	if len(pos) > 1 {
		return errors.New("usage: mw migrate [--dry-run] [PATH]")
	}
	var path string
	if len(pos) == 1 {
		path, _ = filepath.Abs(pos[0])
	} else if path, err = findWorkspace(); err != nil {
		return err
	}
	unlock, err := lockSet(path, true, nil)
	if err != nil {
		return err
	}
	defer unlock()
	if old, err := lockFile(path+".lock", time.Now().Add(lockWait)); err == nil { // the schema 7 lock file
		defer old()
	}
	b, err := readFileShared(path)
	if err != nil {
		return err
	}
	w, from, report, err := migrateBytes(b, path)
	if err != nil {
		return err
	}
	if from == currentSchema {
		fmt.Printf("%s is already schema %d; nothing to do\n", path, currentSchema)
		return nil
	}
	dry := o["dry-run"] == "true"
	if dry {
		fmt.Printf("dry run: %s (schema %d -> %d), nothing written\n", path, from, currentSchema)
	} else {
		bak := fmt.Sprintf("%s.bak%d", path, from)
		if err := os.WriteFile(bak, b, 0600); err != nil {
			return err
		}
		for i := range w.Tenants {
			// 8 -> 9 changes only the index: tenant files are not rewritten (only the index lock is held)
			w.Tenants[i].loaded = from != currentSchema-1
		}
		if err := splitOldArchive(path); err != nil {
			return err
		}
		if err := save(path, w); err != nil {
			return err
		}
		_ = os.Remove(filepath.Join(filepath.Dir(path), ".miniwaves.archive.json"))
		ensureGitExclude(filepath.Dir(path))
		fmt.Printf("migrated %s (schema %d -> %d); backup %s\n", path, from, currentSchema, bak)
	}
	for _, l := range report {
		fmt.Println("  " + l)
	}
	return nil
}

// migrateBytes upgrades the file contents in memory and returns the workspace,
// the schema it came from and a list of what changed.
func migrateBytes(b []byte, path string) (*Workspace, int, []string, error) {
	// Read only the version first: a schema 8 index stores a tenant's archive flag as a bool
	// "archived", which does not fit Tenant.Archived (the archived item count) of older layouts.
	var head struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(b, &head); err != nil {
		return nil, 0, nil, fmt.Errorf("read %s: %w", path, err)
	}
	from := head.Version
	if from == currentSchema-1 { // schema 8 -> 9: same layout, the version is the whole change
		w8, err := loadScopedFrom(path, from, true)
		if err != nil {
			return nil, from, nil, err
		}
		w8.Version = currentSchema
		return w8, from, []string{"schema 9 stops older mw builds from rewriting the index and dropping jobs, tenant jobs and fanfare sets"}, nil
	}
	var w Workspace
	if from < currentSchema-1 {
		if err := json.Unmarshal(b, &w); err != nil {
			return nil, from, nil, fmt.Errorf("read %s: %w", path, err)
		}
	}
	if from > currentSchema {
		return nil, from, nil, fmt.Errorf("workspace schema %d is newer than this mw supports (schema %d): upgrade mw", from, currentSchema)
	}
	if from == currentSchema { // already current: the index loader reads its archive flags
		w9, err := loadScopedFrom(path, from, true)
		if err != nil {
			return nil, from, nil, err
		}
		return w9, from, nil, nil
	}
	var report []string
	if len(w.Tenants) == 0 { // the original single-tenant format
		var old Tenant
		if err := json.Unmarshal(b, &old); err != nil {
			return nil, from, nil, err
		}
		old.Slug, old.Path = slugify(old.Name), filepath.Dir(path)
		w = Workspace{Name: old.Name, Tenants: []Tenant{old}}
		report = append(report, "wrapped the single-tenant file into tenant "+old.Slug)
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(b, &raw) == nil {
		if _, ok := raw["active_tenant"]; ok {
			report = append(report, "dropped active_tenant")
		}
	}
	for i := range w.Tenants {
		t := &w.Tenants[i]
		if t.Next == nil {
			t.Next = map[string]int{}
		}
		moved := 0
		isWave := map[string]bool{}
		for _, it := range t.Items {
			if it.Kind == "wave" {
				isWave[it.ID] = true
			}
		}
		for j := range t.Items {
			it := &t.Items[j]
			if it.Kind != "wave" && it.Wave == "" && isWave[it.Parent] {
				it.Wave, it.Parent = it.Parent, ""
				moved++
			}
			if it.Kind == "wave" && it.Status == "doing" && !strings.HasPrefix(it.Note, "closed") {
				it.Status = "ready"
				moved++
			}
		}
		if moved > 0 {
			report = append(report, fmt.Sprintf("%s: repaired %d wave links/statuses", t.Slug, moved))
		}
	}
	if w.Defaults == nil {
		w.Defaults = map[string]string{}
	}
	if from < 6 {
		if w.Gates == nil {
			w.Gates = map[string]string{"evidence": "off", "acceptance": "off", "judge": "off"}
		}
		report = append(report, "set every workspace gate to off")
	}
	norm, events := 0, 0
	for i := range w.Tenants {
		t := &w.Tenants[i]
		for j := range t.Items {
			it := &t.Items[j]
			if it.Judged == "legacy" || (from < 6 && it.Status == "done" && it.Kind != "wave" && it.Kind != "prompt" && it.Judged == "") {
				it.Judged = "ungated"
				norm++
			}
			if len(it.Events) == 0 {
				it.Events = append(it.Events, Event{At: it.Created, Action: "created"})
				if it.Updated != "" && it.Updated != it.Created {
					it.Events = append(it.Events, Event{At: it.Updated, Action: "updated", By: it.By, Note: it.Status})
				}
				events++
			}
		}
	}
	report = append(report, fmt.Sprintf("judged set to ungated on %d items", norm), fmt.Sprintf("synthesised events for %d items", events))
	w.Version = currentSchema
	return &w, from, report, nil
}

// splitOldArchive moves the schema 7 root archive (.miniwaves.archive.json) into one archive
// file per tenant, merging with any per-tenant archive already there (a re-run is harmless).
func splitOldArchive(path string) error {
	b, err := readFileShared(filepath.Join(filepath.Dir(path), ".miniwaves.archive.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var old struct {
		Tenants []struct {
			Slug  string `json:"slug"`
			Items []Item `json:"items"`
		} `json:"tenants"`
	}
	if err := json.Unmarshal(b, &old); err != nil {
		return fmt.Errorf("read the old archive: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(filepath.Dir(path), storeDir), 0o755); err != nil {
		return err
	}
	for _, t := range old.Tenants {
		a, err := loadArchive(path, t.Slug)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, it := range a.Items {
			seen[it.ID] = true
		}
		for _, it := range t.Items {
			if !seen[it.ID] {
				a.Items = append(a.Items, it)
			}
		}
		nb, _ := json.Marshal(a)
		if err := writeFileAtomic(archivePath(path, t.Slug), append(nb, '\n')); err != nil {
			return err
		}
	}
	return nil
}
