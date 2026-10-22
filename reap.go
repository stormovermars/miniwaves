package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Session liveness: Session.Updated is the session's last activity. It is set by
// session start/restart, by every start/ready/block/done/reopen the session runs, and
// by note and edit with -s/MW_SESSION (touchSession).

const (
	defaultReapAge = 2 * time.Hour
	staleDoingAge  = 2 * time.Hour
	maxReapDays    = 365
)

// actorName is the session running the command (-s / MW_SESSION), else fallback.
func actorName(o map[string]string, fallback string) string {
	if v := value(o, "session", os.Getenv("MW_SESSION")); v != "" {
		return v
	}
	return fallback
}

// touchSession records activity for the named session, when it exists.
func touchSession(w *Workspace, id, at string) {
	if id == "" {
		return
	}
	if s, err := sessionByID(w, id); err == nil {
		s.Updated = at
	}
}

// parseAge accepts Go durations plus a "d" (24h) suffix: 90m, 2h, 1d.
func parseAge(s string) (time.Duration, error) {
	if n, ok := strings.CutSuffix(s, "d"); ok {
		v, err := strconv.ParseFloat(n, 64)
		if err != nil || !(v > 0) || v > maxReapDays {
			return 0, fmt.Errorf("bad duration %q (use 90m, 2h, 1d; at most %dd)", s, maxReapDays)
		}
		d := time.Duration(v * float64(24*time.Hour))
		if d <= 0 {
			return 0, fmt.Errorf("bad duration %q (use 90m, 2h, 1d; at most %dd)", s, maxReapDays)
		}
		return d, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 || d > maxReapDays*24*time.Hour {
		return 0, fmt.Errorf("bad duration %q (use 90m, 2h, 1d; at most %dd)", s, maxReapDays)
	}
	return d, nil
}

// ageShort renders the age of a stamp as 5m, 3h, 2d (0m under a minute).
func ageShort(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return "?"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "0m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// ownedDoing returns the doing items the session owns.
func ownedDoing(w *Workspace, s *Session) []*Item {
	var out []*Item
	for i := range w.Tenants {
		t := &w.Tenants[i]
		if t.Slug != s.Tenant {
			continue
		}
		for j := range t.Items {
			it := &t.Items[j]
			if it.Assignee == s.ID && it.Status == "doing" && it.Kind != "wave" {
				out = append(out, it)
			}
		}
	}
	return out
}

// releaseSession puts every doing item the session owns back to ready (owner cleared,
// note appended) and stops the session. note is the text (the note line adds its own
// timestamp); by is the actor credited on the ready event. It returns the released item ids.
func releaseSession(w *Workspace, s *Session, note, by string) []string {
	now := time.Now().UTC().Format(time.RFC3339)
	text := note
	ids := []string{}
	for _, it := range ownedDoing(w, s) {
		it.Status, it.Assignee, it.Updated, it.Note = "ready", "", now, ""
		it.Judged, it.JudgedBy, it.Evidence = "", "", nil
		it.Notes = append(it.Notes, now+" "+text)
		emit(it, now, "ready", by, text)
		ids = append(ids, it.ID)
	}
	s.State, s.Item, s.Updated = "stopped", "", now
	return ids
}

// isJobRole reports whether the session is a dispatcher or newsdesk of a tenant whose job exists:
// reap leaves those alone by default, so wr's plain `reap -t T --older-than X` is safe for a job.
func isJobRole(w *Workspace, s *Session) bool {
	if s.Agent != "dispatcher" && s.Agent != "newsdesk" {
		return false
	}
	t, err := tenantBySlug(w, s.Tenant)
	if err != nil || t.Job == "" {
		return false
	}
	_, ok := w.Jobs[t.Job]
	return ok
}

// sparedRow is a dispatcher or newsdesk session reap would have released but left alone.
type sparedRow struct {
	Session string `json:"session"`
	Tenant  string `json:"tenant"`
	Role    string `json:"role"`
	Job     string `json:"job"`
}

type reapRow struct {
	Session string   `json:"session"`
	Tenant  string   `json:"tenant"`
	Items   []string `json:"items"`
}

func reapCommand(path string, w *Workspace, o map[string]string, pos []string) error {
	if len(pos) != 0 {
		return fmt.Errorf("unexpected argument %q", pos[0])
	}
	if err := allowOptions(o, "tenant", "all", "older-than", "dry-run", "json", "except", "session"); err != nil {
		return err
	}
	if o["all"] == "true" && o["tenant"] != "" {
		return errors.New("mw session reap -A cannot be combined with -t")
	}
	age := defaultReapAge
	label := "2h"
	if v := o["older-than"]; v != "" {
		d, err := parseAge(v)
		if err != nil {
			return err
		}
		age, label = d, v
	}
	slug := ""
	if o["all"] != "true" {
		t, err := selectTenant(w, o)
		if err != nil {
			return err
		}
		slug = t.Slug
	}
	except := map[string]bool{}
	for _, n := range strings.Split(o["except"], ",") {
		if n = strings.TrimSpace(n); n != "" {
			except[n] = true
		}
	}
	by := actorName(o, "reap")
	dry := o["dry-run"] == "true"
	now := time.Now().UTC()
	rows := []reapRow{}
	spared := []sparedRow{}
	for i := range w.Sessions {
		s := &w.Sessions[i]
		if s.State == "stopped" || isLocalID(s.ID) || except[s.ID] || (slug != "" && s.Tenant != slug) {
			continue
		}
		last := parseTS(s.Updated)
		if last.IsZero() || now.Sub(last) <= age {
			continue
		}
		if isJobRole(w, s) {
			t, _ := tenantBySlug(w, s.Tenant)
			spared = append(spared, sparedRow{s.ID, s.Tenant, s.Agent, t.Job})
			continue
		}
		row := reapRow{Session: s.ID, Tenant: s.Tenant, Items: []string{}}
		if dry {
			for _, it := range ownedDoing(w, s) {
				row.Items = append(row.Items, it.ID)
			}
		} else {
			row.Items = releaseSession(w, s, "reaped: no activity since "+s.Updated+" (> "+label+")", by)
		}
		rows = append(rows, row)
	}
	if !dry && len(rows) > 0 {
		if err := save(path, w); err != nil {
			return err
		}
	}
	if o["json"] == "true" {
		if dry {
			return printJSON(struct {
				DryRun bool        `json:"dry_run"`
				Reaped []reapRow   `json:"reaped"`
				Spared []sparedRow `json:"spared"`
			}{true, rows, spared})
		}
		return printJSON(struct {
			Reaped []reapRow   `json:"reaped"`
			Spared []sparedRow `json:"spared"`
		}{rows, spared})
	}
	for _, s := range spared {
		fmt.Printf("spared %s session %s (job %s)\n", s.Role, s.Session, s.Job)
	}
	if len(rows) == 0 && len(spared) == 0 {
		fmt.Println("nothing to reap")
		return nil
	}
	if len(rows) == 0 {
		return nil
	}
	verb, item := "reaped", "released"
	if dry {
		verb, item = "would reap", "would release"
	}
	for _, r := range rows {
		fmt.Printf("%s session %s [%s]: no activity for over %s\n", verb, r.Session, r.Tenant, label)
		for _, id := range r.Items {
			fmt.Printf("  %s %s\n", item, id)
		}
	}
	return nil
}
