package main

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The coordinated kit: files only. Templates are embedded in the binary; nothing is read from repo
// content, nothing is executed, and an existing file is never overwritten.

//go:embed kit/*
var kitFS embed.FS

type kitFile struct {
	name string
	mode os.FileMode
}

var kitFiles = []kitFile{{"PLAN.md", 0644}, {"NEWSDESK.md", 0644}, {"buildlock.sh", 0755}, {"buildlock.ps1", 0644}}

// initKit handles "mw init --kit coordinated --job ID" inside an existing workspace.
func initKit(path string, w *Workspace, args []string) error {
	const use = "usage: mw init --kit coordinated --job ID"
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if err := allowOptions(o, "kit", "job"); err != nil {
		return err
	}
	if len(pos) > 0 || o["kit"] == "" {
		return errors.New(use)
	}
	if o["kit"] != "coordinated" {
		return fmt.Errorf("unknown kit %q (kits: coordinated)", o["kit"])
	}
	id := o["job"]
	if !jobIDPattern.MatchString(id) {
		return fmt.Errorf("--job: job id %q must match ^[a-z0-9][a-z0-9-]{0,31}$ (%s)", id, use)
	}
	created := false
	if _, ok := w.Jobs[id]; !ok {
		if w.Jobs == nil {
			w.Jobs = map[string]JobConfig{}
		}
		w.Jobs[id] = JobConfig{}
		created = true
	}
	v := jobView(w, id)
	tenants := strings.Join(v.Tenants, ", ")
	if tenants == "" {
		tenants = "(none yet: mw jobs enable " + id + " -t SLUG)"
	}
	rep := strings.NewReplacer("@@JOB@@", id, "@@TITLE@@", v.Title, "@@TENANTS@@", tenants, "@@DATE@@", time.Now().Format("2006-01-02"))
	dir := filepath.Dir(path)
	var wrote, skipped []string
	for _, f := range kitFiles {
		src, err := kitFS.ReadFile("kit/" + f.name)
		if err != nil {
			return err
		}
		fh, err := os.OpenFile(filepath.Join(dir, f.name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, f.mode)
		if err != nil {
			if os.IsExist(err) {
				skipped = append(skipped, f.name)
				continue
			}
			return err
		}
		_, werr := fh.WriteString(rep.Replace(string(src)))
		if cerr := fh.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return werr
		}
		wrote = append(wrote, f.name)
	}
	if created {
		if err := save(path, w); err != nil {
			return err
		}
		fmt.Printf("job %s created (no tenants)\n", id)
	}
	if len(wrote) > 0 {
		fmt.Println("wrote " + strings.Join(wrote, ", "))
	}
	if len(skipped) > 0 {
		fmt.Println("skipped (exists) " + strings.Join(skipped, ", "))
	}
	fmt.Printf("dispatcher prompt: mw get-prompt dispatcher --job %s\n", id)
	fmt.Printf("newsdesk prompt:   mw get-prompt newsdesk --job %s\n", id)
	return nil
}
