package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

func tenantCommand(path string, w *Workspace, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: mw tenant add|list|default|archive|restore|tag")
	}
	switch args[0] {
	case "archive", "restore", "tag":
		return tenantMeta(path, w, args)
	case "protocol":
		return tenantProtocol(path, w, args[1:])
	case "list":
		o, pos, err := options(args[1:])
		if err != nil {
			return err
		}
		if len(pos) != 0 {
			return fmt.Errorf("unexpected argument %q", pos[0])
		}
		if err := allowOptions(o, "json", "verbose", "tag"); err != nil {
			return err
		}
		selected, _ := selectTenantAny(w, map[string]string{})
		if selected == nil {
			selected = &Tenant{}
		}
		if o["verbose"] == "true" {
			o["archived"] = "true"
			out := make([]tenantDetail, 0, len(w.Tenants))
			for _, t := range shown(w, o) {
				out = append(out, tenantDetail{t.Slug, t.Name, len(t.Items), t.Slug == selected.Slug, t.Path, t.Tags, t.Hidden})
			}
			return printData(out, o)
		}
		out := make([]tenantBrief, 0, len(w.Tenants))
		for _, t := range shown(w, o) {
			out = append(out, tenantBrief{t.Slug, len(t.Items), t.Slug == selected.Slug, t.Tags})
		}
		return printData(out, o)
	case "default":
		cwd, err := filepath.Abs(workDir())
		if err != nil {
			return err
		}
		if w.Defaults == nil {
			w.Defaults = map[string]string{}
		}
		folder, inherited := folderDefault(w, cwd)
		if len(args) == 1 {
			if inherited != "" {
				fmt.Println("default tenant:", inherited)
			} else {
				fmt.Println("default tenant: none")
			}
			return nil
		}
		if len(args) != 2 {
			return errors.New("usage: mw tenant default [SLUG|-]")
		}
		if args[1] == "-" {
			if folder != "" {
				delete(w.Defaults, folder)
			}
			if err := save(path, w); err != nil {
				return err
			}
			fmt.Println("default tenant: none")
			return nil
		}
		if _, err := tenantBySlug(w, args[1]); err != nil {
			return err
		}
		w.Defaults[filepath.Clean(cwd)] = args[1]
		if err := save(path, w); err != nil {
			return err
		}
		fmt.Printf("default tenant: %s (%s)\n", args[1], cwd)
		return nil
	case "add":
		o, pos, err := options(args[1:])
		if err != nil {
			return err
		}
		if len(pos) < 1 {
			return errors.New("usage: mw tenant add SLUG [NAME...]")
		}
		if err := allowOptions(o, "path"); err != nil {
			return err
		}
		slug := pos[0]
		if !validID(slug) {
			return errors.New("tenant slug must be at most 64 lowercase letters, numbers, and single dashes")
		}
		if existing, err := tenantBySlug(w, slug); err == nil {
			if o["path"] == "" {
				return fmt.Errorf("tenant %s already exists (give -d DIR to update its path)", slug)
			}
			if len(pos) > 1 {
				return fmt.Errorf("tenant %s already exists; the update path takes no NAME", slug)
			}
			newPath, _ := filepath.Abs(o["path"])
			newPath = workLoc.mapCwd(newPath)
			for _, other := range w.Tenants {
				if other.Slug != slug && other.Path != "" && filepath.Clean(other.Path) == filepath.Clean(newPath) {
					return fmt.Errorf("path %s is already held by tenant %s", newPath, other.Slug)
				}
			}
			old := existing.Path
			existing.Path = newPath
			if err := save(path, w); err != nil {
				return err
			}
			fmt.Printf("updated tenant %s: path %s -> %s\n", slug, old, newPath)
			return nil
		}
		name := slug
		if len(pos) > 1 {
			name = strings.Join(pos[1:], " ")
		}
		p := o["path"]
		if p != "" {
			p, _ = filepath.Abs(p)
			p = workLoc.mapCwd(p)
		}
		w.Tenants = append(w.Tenants, Tenant{Slug: slug, Name: name, Path: p, Next: map[string]int{}, Items: []Item{}, loaded: true})
		if err := save(path, w); err != nil {
			return err
		}
		fmt.Printf("added tenant %s\n", slug)
		return nil
	default:
		return fmt.Errorf("unknown tenant command %q", args[0])
	}
}

func sessionCommand(path string, w *Workspace, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: mw session start|list|stop")
	}
	if args[0] == "reap" { // -n is --dry-run here (elsewhere it is --limit)
		args = append([]string(nil), args...)
		for i, a := range args {
			if a == "-n" {
				args[i] = "--dry-run"
			}
		}
	}
	o, pos, err := options(args[1:])
	if err != nil {
		return err
	}
	switch args[0] {
	case "reap":
		return reapCommand(path, w, o, pos)
	case "list":
		if len(pos) != 0 {
			return fmt.Errorf("unexpected argument %q", pos[0])
		}
		if err := allowOptions(o, "tenant", "json", "verbose"); err != nil {
			return err
		}
		if o["tenant"] != "" {
			if _, err := tenantBySlug(w, o["tenant"]); err != nil {
				return err
			}
		}
		if o["verbose"] == "true" {
			out := make([]sessionDetail, 0)
			for _, s := range w.Sessions {
				if o["tenant"] == "" || s.Tenant == o["tenant"] {
					out = append(out, sessionDetail{s.ID, s.Tenant, s.Agent, s.State, s.Item, s.Updated, ageShort(s.Updated)})
				}
			}
			return printData(out, o)
		}
		out := make([]sessionBrief, 0)
		for _, s := range w.Sessions {
			if o["tenant"] == "" || s.Tenant == o["tenant"] {
				out = append(out, sessionBrief{s.ID, s.Tenant, s.State, s.Item})
			}
		}
		return printData(out, o)
	case "start":
		if err := allowOptions(o, "tenant", "agent", "claude-session", "answerer"); err != nil {
			return err
		}
		if len(pos) != 1 {
			return errors.New("usage: mw session start NAME [--tenant SLUG] [--claude-session ID] [--answerer PARENT]")
		}
		cs := o["claude-session"]
		if cs != "" && !claudeSessionRe.MatchString(cs) {
			return errors.New("--claude-session must be 1 to 128 letters, numbers, dots, dashes and underscores")
		}
		id := pos[0]
		if id == localName {
			return errors.New(`session name "local" is reserved for the implicit per-tenant session`)
		}
		if !validID(id) {
			return errors.New("session name must be at most 64 lowercase letters, numbers, and single dashes")
		}
		role := value(o, "agent", id)
		if !validID(role) {
			return errors.New("agent role must be at most 64 lowercase letters, numbers, and single dashes")
		}
		ans := o["answerer"]
		if ans != "" {
			if !validID(ans) || ans == localName {
				return errors.New("--answerer must be a session name")
			}
			if err := answererCycle(w, id, ans); err != nil {
				return err
			}
		}
		os.Unsetenv("MW_SESSION") // start never resolves the current session
		t, err := selectTenant(w, map[string]string{"tenant": o["tenant"]})
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if existing, err := sessionByID(w, id); err == nil {
			if existing.State != "stopped" {
				return fmt.Errorf("session %s already exists", id)
			}
			existing.Tenant, existing.Agent, existing.State, existing.Updated = t.Slug, role, "idle", now
			existing.ClaudeSession, existing.Answerer, existing.AnswererAt = cs, ans, answererStamp(ans, now)
			if err := save(path, w); err != nil {
				return err
			}
			fmt.Printf("restarted session %s on %s\n", id, t.Slug)
			return nil
		}
		if err := sessionElsewhere(path, w, id, t.Slug); err != nil {
			return err
		}
		w.Sessions = append(w.Sessions, Session{ID: id, Tenant: t.Slug, Agent: role, State: "idle", Updated: now, ClaudeSession: cs, Answerer: ans, AnswererAt: answererStamp(ans, now)})
		if err := save(path, w); err != nil {
			return err
		}
		fmt.Printf("started session %s on %s\n", id, t.Slug)
		return nil
	case "stop":
		if err := allowOptions(o, "tenant", "release"); err != nil {
			return err
		}
		if len(pos) != 1 {
			return errors.New("usage: mw session stop NAME [-t SLUG] [--release]")
		}
		s, err := sessionByID(w, pos[0])
		if err != nil {
			return err
		}
		if o["release"] == "true" {
			ids := releaseSession(w, s, "released: session "+s.ID+" stopped with --release", actorName(o, "stop"))
			if err := save(path, w); err != nil {
				return err
			}
			fmt.Println("stopped session", s.ID)
			for _, id := range ids {
				fmt.Println("  released", id)
			}
			return nil
		}
		if s.Item != "" {
			return fmt.Errorf("session %s still owns %s; block, finish, or reopen it first", s.ID, s.Item)
		}
		s.State, s.Updated = "stopped", time.Now().UTC().Format(time.RFC3339)
		if err := save(path, w); err != nil {
			return err
		}
		fmt.Println("stopped session", s.ID)
		return nil
	default:
		return fmt.Errorf("unknown session command %q", args[0])
	}
}

func addItem(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return errors.New("usage: mw add KIND TITLE...")
	}
	if err := allowOptions(o, "tenant", "session", "parent", "wave", "body", "acceptance", "status", "ticket"); err != nil {
		return err
	}
	t, err := selectTenant(w, o)
	if err != nil {
		return err
	}
	kind := strings.ToLower(pos[0])
	prefix, ok := kinds[kind]
	if kind == "wave" {
		return errors.New("create waves with mw wave create")
	}
	if !ok {
		return fmt.Errorf("unknown kind %q", kind)
	}
	status := value(o, "status", "inbox")
	if !statuses[status] {
		return fmt.Errorf("unknown status %q", status)
	}
	parent := strings.ToUpper(o["parent"])
	if err := checkParent(t, &Item{Kind: kind}, parent); err != nil {
		return err
	}
	var wv *Item
	if o["wave"] != "" {
		if wv, err = waveByID(t, o["wave"]); err != nil {
			return fmt.Errorf("wave: %w", err)
		}
		if err := checkMember(&Item{Kind: kind}, wv); err != nil {
			return err
		}
	}
	gs := gateStates(w, t)
	if status == "done" && (byName(gs, "evidence").Effective || byName(gs, "judge").Effective) {
		return coded("GATE_REFUSED", errors.New("add -S done is refused while the evidence or judge gate is on: add it, then mw done ID CHECK"))
	}
	ticket := ""
	if o["ticket"] != "" {
		if ticket, err = ticketForItem(w, t, o["ticket"]); err != nil {
			return err
		}
	}
	t.Next[prefix]++
	now := time.Now().UTC().Format(time.RFC3339)
	it := Item{Ticket: ticket, ID: fmt.Sprintf("%s-%03d", prefix, t.Next[prefix]), Kind: kind,
		Title: strings.Join(pos[1:], " "), Status: status, Parent: parent,
		Body: o["body"], Acceptance: o["acceptance"], Created: now, Updated: now}
	if status == "done" && kind != "prompt" {
		it.Judged = "ungated"
	}
	emit(&it, now, "created", value(o, "session", os.Getenv("MW_SESSION")), "")
	if status != "inbox" {
		emit(&it, now, status, value(o, "session", os.Getenv("MW_SESSION")), "")
	}
	if wv != nil {
		it.Wave = wv.ID
	}
	t.Items = append(t.Items, it)
	if err := save(path, w); err != nil {
		return err
	}
	fmt.Printf("[%s] %s  %-7s %s\n", t.Slug, it.ID, it.Status, compactText(it.Title, 72))
	return nil
}

func itemByID(t *Tenant, id string) (*Item, error) {
	id = strings.ToUpper(id)
	for i := range t.Items {
		if t.Items[i].ID == id {
			return &t.Items[i], nil
		}
	}
	return nil, fmt.Errorf("item %s not found", id)
}

func value(m map[string]string, key, fallback string) string {
	if m[key] != "" {
		return m[key]
	}
	return fallback
}

func filtered(t *Tenant, o map[string]string) []Item {
	var out []Item
	for _, it := range t.Items {
		if o["kind"] != "" && it.Kind != o["kind"] {
			continue
		}
		if o["status"] != "" && it.Status != o["status"] {
			continue
		}
		out = append(out, it)
	}
	return out
}

func listItems(w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return fmt.Errorf("unexpected argument %q", pos[0])
	}
	if err := allowOptions(o, "tenant", "session", "kind", "status", "json", "verbose", "all", "limit", "tag", "archived"); err != nil {
		return err
	}
	limit := 0
	if o["limit"] != "" {
		if _, e := fmt.Sscanf(o["limit"], "%d", &limit); e != nil || limit < 1 {
			return errors.New("-n needs a positive number")
		}
	}
	if o["all"] == "true" {
		if o["tenant"] != "" {
			return errors.New("mw list -A cannot be combined with -t")
		}
		return listAll(w, o, limit)
	}
	t, err := selectTenant(w, o)
	if err != nil {
		return err
	}
	items := filtered(t, o)
	var waves []Item
	var plain []Item
	for _, it := range items {
		if it.Kind == "wave" {
			waves = append(waves, it)
		} else {
			plain = append(plain, it)
		}
	}
	if limit > 0 && len(plain) > limit {
		plain = plain[:limit]
	}
	var list any
	if o["verbose"] == "true" {
		out := make([]itemDetail, 0, len(plain))
		for _, it := range plain {
			out = append(out, detailOf(it, true))
		}
		list = out
	} else {
		out := make([]itemBrief, 0, len(plain))
		for _, it := range plain {
			out = append(out, itemBrief{it.ID, it.Kind, it.Status, compactText(it.Title, 72), it.Ticket})
		}
		list = out
	}
	rows := make([]waveRow, 0, len(waves))
	for _, wv := range waves {
		rows = append(rows, waveRowOf(t, wv))
	}
	if len(rows) == 0 && o["json"] != "true" {
		return printData(list, o)
	}
	return printData(listDoc{list, rows}, o)
}

type itemRow struct {
	Tenant string `json:"tenant" toon:"tenant"`
	ID     string `json:"id" toon:"id"`
	Type   string `json:"type" toon:"type"`
	State  string `json:"state" toon:"state"`
	Title  string `json:"title" toon:"title"`
	Ticket string `json:"ticket,omitempty" toon:"ticket,omitempty"`
}

type itemDetailRow struct {
	Tenant string `json:"tenant" toon:"tenant"`
	itemDetail
}

// listAll is mw list -A: items of every tenant (waves are not listed); -s
// SESSION keeps only the items that session owns.
func listAll(w *Workspace, o map[string]string, limit int) error {
	owner := o["session"]
	brief := []itemRow{}
	detail := []itemDetailRow{}
	for _, t := range shown(w, o) {
		announceTenant(w, t)
		for _, it := range filtered(t, o) {
			if it.Kind == "wave" {
				continue
			}
			if owner != "" && it.Assignee != owner && !(owner == localName && isLocalID(it.Assignee)) {
				continue
			}
			if limit > 0 && len(brief) >= limit {
				break
			}
			brief = append(brief, itemRow{t.Slug, it.ID, it.Kind, it.Status, compactText(it.Title, 72), it.Ticket})
			if o["verbose"] == "true" {
				detail = append(detail, itemDetailRow{t.Slug, detailOf(it, true)})
			}
		}
	}
	var list any = brief
	if o["verbose"] == "true" {
		list = detail
	}
	if o["json"] != "true" {
		return printData(list, o)
	}
	return printData(listDoc{list, []waveRow{}}, o)
}

// listDoc is the single document printed when a list includes waves.
type listDoc struct {
	Items any       `json:"items" toon:"items"`
	Waves []waveRow `json:"waves" toon:"waves"`
}

func showItem(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: mw show ID")
	}
	if err := allowOptions(o, "tenant", "session", "json", "verbose", "full"); err != nil {
		return err
	}
	t, err := selectTenant(w, o)
	if err != nil {
		return err
	}
	verbose := o["verbose"] == "true" && o["json"] != "true" // -j ignores -v
	it, err := itemByID(t, pos[0])
	if err != nil {
		ai, ok := archivedItem(path, t.Slug, pos[0])
		if !ok {
			return err
		}
		it = &ai
	}
	d := briefOf(w, t, *it, verbose)
	full := o["full"] == "true"
	if full && len(it.Events) > 0 {
		d.Events = it.Events // --full: every event, notes uncut
	}
	if o["json"] == "true" {
		return printJSON(d)
	}
	printBriefFull(d, verbose, full)
	return nil
}

func moveCommand(path string, w *Workspace, command string, args []string) error {
	t, it, err := applyMove(path, w, command, args)
	if err != nil {
		return err
	}
	fmt.Printf("[%s] %s  %-7s %s\n", t.Slug, it.ID, it.Status, compactText(it.Title, 72))
	if command == "done" {
		printClosedAsk(it)
	}
	return nil
}

// applyMove runs ready/start/block/done/reopen and saves; it prints nothing.
func applyMove(path string, w *Workspace, command string, args []string) (*Tenant, *Item, error) {
	var evPaths []string
	if command == "done" {
		var e error
		if args, evPaths, e = splitEvidenceArgs(args); e != nil {
			return nil, nil, e
		}
	}
	o, pos, err := options(args)
	if err != nil {
		return nil, nil, err
	}
	if len(pos) < 1 {
		return nil, nil, fmt.Errorf("usage: mw %s ID", command)
	}
	if (command == "block" || command == "done") && len(pos) < 2 {
		return nil, nil, fmt.Errorf("mw %s requires a reason or verification note", command)
	}
	if command != "block" && command != "done" && len(pos) != 1 {
		return nil, nil, fmt.Errorf("usage: mw %s ID", command)
	}
	allowed := []string{"tenant", "session", "json"} // -j only shapes errors (code field); success prints the same line
	if command == "done" {
		allowed = append(allowed, "no-evidence")
	}
	if command == "reopen" {
		allowed = append(allowed, "message")
	}
	if command == "start" {
		allowed = append(allowed, "agent")
	}
	if command == "block" {
		allowed = append(allowed, "on")
	}
	if err := allowOptions(o, allowed...); err != nil {
		return nil, nil, err
	}
	noEvidence, err := noEvidenceReason(o)
	if err != nil {
		return nil, nil, err
	}
	t, err := selectTenant(w, o)
	if err != nil {
		return nil, nil, err
	}
	it, err := itemByID(t, pos[0])
	if err != nil {
		if command == "reopen" {
			if _, ok := archivedItem(path, t.Slug, pos[0]); ok {
				return nil, nil, fmt.Errorf("%s is archived (no unarchive)", strings.ToUpper(pos[0]))
			}
		}
		return nil, nil, err
	}
	sessionID := value(o, "session", os.Getenv("MW_SESSION"))
	implicitLocal := false
	if sessionID == localName {
		return nil, nil, errors.New(`session name "local" is reserved for the implicit per-tenant session`)
	}
	if sessionID == "" && command == "start" {
		sessionID, implicitLocal = localID(t.Slug), true
	}
	var s *Session
	if sessionID != "" {
		if !implicitLocal && !validID(sessionID) {
			return nil, nil, errors.New("session name must be at most 64 lowercase letters, numbers, and single dashes")
		}
		s, err = sessionByID(w, sessionID)
		switch {
		case err != nil && command == "start":
			// only start registers a missing session on this tenant
			agent := sessionID
			if implicitLocal {
				agent = localName
			} else if r := o["agent"]; r != "" {
				if !validID(r) {
					return nil, nil, errors.New("agent role must be at most 64 lowercase letters, numbers, and single dashes")
				}
				agent = r
			}
			if err := sessionElsewhere(path, w, sessionID, t.Slug); err != nil {
				return nil, nil, err
			}
			w.Sessions = append(w.Sessions, Session{ID: sessionID, Tenant: t.Slug, Agent: agent, State: "idle", Updated: time.Now().UTC().Format(time.RFC3339)})
			s = &w.Sessions[len(w.Sessions)-1]
		case err != nil:
			s = nil
		case s.Tenant != t.Slug:
			return nil, nil, errors.New("session and tenant do not match")
		case s.State == "stopped":
			s.State = "idle"
		}
	}
	// who acted: the session, or the item's owner for a session-less done/block
	actor := displayName(sessionID)
	if sessionID == "" {
		actor = displayName(it.Assignee)
	}
	if it.Kind == "wave" {
		switch {
		case command == "done":
			return nil, nil, fmt.Errorf("%s is a wave; use mw wave close", it.ID)
		case command != "reopen":
			return nil, nil, fmt.Errorf("%s is a wave; mw %s does not apply (use mw wave ...; only reopen works)", it.ID, command)
		case it.Status != "done":
			return nil, nil, fmt.Errorf("%s is not closed", it.ID)
		}
	}
	if it.Wave != "" && (command == "reopen" || command == "ready" || command == "start") {
		if wv, e := itemByID(t, it.Wave); e == nil && wv.Status == "done" {
			return nil, nil, fmt.Errorf("wave %s is closed; reopen it first", wv.ID)
		}
	}
	to := map[string]string{"ready": "ready", "start": "doing", "block": "blocked", "done": "done", "reopen": "ready"}[command]
	if command == "start" && it.Assignee != "" && it.Assignee != sessionID {
		return nil, nil, coded("TASK_CLAIMED", fmt.Errorf("%s is owned by session %s", it.ID, it.Assignee))
	}
	if command == "start" && it.Status != "inbox" && it.Status != "ready" && !(it.Assignee != "" && it.Assignee == sessionID) {
		hint := "use mw ready first"
		if it.Status == "done" {
			hint = "use mw reopen"
		} else if it.Status == "doing" {
			hint = "it is already being worked on"
		}
		err := fmt.Errorf("start: %s is %s — %s", it.ID, it.Status, hint)
		if it.Status == "doing" {
			return nil, nil, coded("TASK_CLAIMED", err)
		}
		return nil, nil, err
	}
	if command != "start" && sessionID != "" && it.Assignee != "" && it.Assignee != sessionID {
		return nil, nil, coded("TASK_CLAIMED", fmt.Errorf("%s is owned by session %s", it.ID, it.Assignee))
	}
	// the implicit local session is the human/controller and may own many items; named sessions keep the one-item rule
	if command == "start" && !implicitLocal && s != nil && s.Item != "" && s.Item != it.ID {
		return nil, nil, coded("WIP_LIMIT", fmt.Errorf("session %s already owns %s", s.ID, s.Item))
	}
	if command == "start" && sessionID != "" && wipSessionOn() {
		for _, other := range t.Items {
			if other.Status == "doing" && other.Assignee == sessionID && other.ID != it.ID && other.Kind != "wave" {
				return nil, nil, coded("WIP_LIMIT", fmt.Errorf("wip.session is on: session %s already has %s doing (finish or block it first)", displayName(sessionID), other.ID))
			}
		}
	}
	dep := ""
	if o["on"] != "" {
		d, e := itemByID(t, o["on"])
		if e != nil {
			return nil, nil, fmt.Errorf("--on: %v", e)
		}
		if d.ID == it.ID {
			return nil, nil, errors.New("--on: an item cannot wait for itself")
		}
		if d.Status == "done" {
			return nil, nil, fmt.Errorf("--on: %s is already done; nothing to wait on", d.ID)
		}
		dep = d.ID
	}
	var evidence []string
	judgeOn := false
	if to == "done" {
		for _, child := range t.Items {
			if child.Parent == it.ID && child.Status != "done" {
				return nil, nil, fmt.Errorf("%s is not done: child %s is %s", it.ID, child.ID, child.Status)
			}
		}
		gs := gateStates(w, t)
		judgeOn = byName(gs, "judge").Effective
		var copied []string
		if len(evPaths) > 0 {
			if copied, err = copyEvidence(evPaths, evidenceFolder(w, t, it.ID)); err != nil {
				return nil, nil, err
			}
			evidence = copied
		}
		if byName(gs, "evidence").Effective && it.Kind != "prompt" && noEvidence == "" {
			if evidence, err = checkEvidence(w, t, it.ID); err != nil {
				recordGatesSeen(path, w, t)
				return nil, nil, err
			}
		}
	}
	stamp := time.Now().UTC().Format(time.RFC3339)
	if command == "start" {
		startStoryAbove(t, it, stamp, actor)
		it.Assignee = sessionID
		if s != nil && !implicitLocal {
			s.State, s.Item = "working", it.ID
		}
	} else {
		releaseOwner(w, it)
	}
	it.Status, it.Updated = to, stamp
	if command == "ready" || command == "start" || command == "reopen" {
		it.Judged, it.JudgedBy, it.Evidence = "", "", nil
	}
	if to == "done" {
		it.By, it.JudgedBy, it.Evidence, it.Judged = actor, "", evidence, ""
		if !judgeOn && it.Kind != "prompt" {
			it.Judged = "ungated"
		}
		if noEvidence != "" {
			it.Notes = append(it.Notes, it.Updated+" no-evidence: "+noEvidence)
		}
	}
	if it.Kind == "wave" {
		for i := range it.Steps {
			it.Steps[i] = Step{Key: it.Steps[i].Key, Title: it.Steps[i].Title}
		}
	}
	if command == "block" || command == "done" {
		it.Note = strings.Join(pos[1:], " ")
	} else {
		it.Note = ""
	}
	if s != nil {
		s.Updated = it.Updated
	}
	enote := ""
	if command == "block" || command == "done" {
		enote = it.Note
	} else if command == "reopen" {
		enote = o["message"]
	}
	emit(it, it.Updated, command, actor, enote)
	if dep != "" {
		it.Events[len(it.Events)-1].On = dep
	}
	if to == "done" {
		if !judgeOn || !judgeable(t, *it) {
			unblockDependents(t, it.ID, stamp, actor)
		}
		fanfareAfterChange(w, t, it, false, stamp)
	}
	if err := save(path, w); err != nil {
		return nil, nil, err
	}
	return t, it, nil
}

// unblockDependents returns the items blocked --on id to ready; call it only when id is final (done and, with the judge gate on, accepted; or a closed wave).
func unblockDependents(t *Tenant, id, stamp, actor string) {
	for i := range t.Items {
		b := &t.Items[i]
		if b.Status != "blocked" {
			continue
		}
		if e, ok := lastEvent(*b, "block"); ok && e.On == id {
			b.Status, b.Updated, b.Note = "ready", stamp, ""
			emit(b, stamp, "ready", actor, "unblocked: "+id+" done")
		}
	}
}

// recordGatesSeen stores the tenant's effective gates in gates_seen when they
// differ, so a refused command does not repeat the flip line next time.
func recordGatesSeen(path string, w *Workspace, t *Tenant) {
	if reflect.DeepEqual(t.GatesSeen, effectiveMap(gateStates(w, t))) {
		return
	}
	_ = save(path, w)
}

// startStoryAbove moves the nearest story ancestor of a started child from
// inbox/ready to doing; any other status is left alone.
func startStoryAbove(t *Tenant, it *Item, now, by string) {
	cur := it
	for i := 0; i <= len(t.Items) && cur.Parent != ""; i++ {
		p, err := itemByID(t, cur.Parent)
		if err != nil {
			return
		}
		if p.Kind == "story" {
			if p.Status == "inbox" || p.Status == "ready" {
				p.Status, p.Updated = "doing", now
				emit(p, now, "start", by, "via "+it.ID)
			}
			return
		}
		cur = p
	}
}

// pickNext returns the first ready work item. With the acceptance gate on it
// skips prompts/stories (via -k) whose effective acceptance is empty and counts them.
func pickNext(w *Workspace, t *Tenant, o map[string]string) (*Item, int, []gateInfo) {
	gs := gateStates(w, t)
	var found *Item
	skipped := 0
	items := filtered(t, o)
	for i := range items {
		it := items[i]
		if it.Status != "ready" || it.Kind == "wave" {
			continue
		}
		if o["kind"] == "" && (it.Kind == "prompt" || it.Kind == "story") {
			continue
		}
		if byName(gs, "acceptance").Effective && !isWorkKind(it.Kind) && strings.TrimSpace(effectiveAcceptance(t, it)) == "" {
			skipped++
			continue
		}
		if found == nil {
			found = &items[i]
		}
	}
	return found, skipped, gs
}

type nextJSON struct {
	ID         string          `json:"id"`
	Title      string          `json:"title"`
	Acceptance string          `json:"acceptance"`
	Evidence   string          `json:"evidence,omitempty"`
	Level      string          `json:"level"`
	Gates      map[string]bool `json:"gates"`
	Skipped    int             `json:"skipped"`
}

// nextOf builds the -j next record; the evidence folder appears only with the evidence gate on.
func nextOf(w *Workspace, t *Tenant, it Item, gs []gateInfo, skipped int) nextJSON {
	n := nextJSON{ID: it.ID, Title: compactText(it.Title, 72), Acceptance: nextAcceptance(t, it, gs),
		Level: levelOf(gs), Gates: effectiveMap(gs), Skipped: skipped}
	if byName(gs, "evidence").Effective {
		n.Evidence = evidenceFolder(w, t, it.ID)
	}
	return n
}

type nextDetailJSON struct {
	itemDetail
	Evidence string          `json:"evidence,omitempty"`
	Gates    map[string]bool `json:"gates"`
	Skipped  int             `json:"skipped"`
}

func nextItem(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return fmt.Errorf("unexpected argument %q", pos[0])
	}
	allowed := []string{"tenant", "session", "kind", "json", "verbose", "claim"}
	if o["claim"] == "true" {
		allowed = append(allowed, "agent", "prompt")
	}
	if err := allowOptions(o, allowed...); err != nil {
		return err
	}
	t, err := selectTenant(w, o)
	if err != nil {
		return err
	}
	if o["claim"] == "true" {
		return claimNext(path, w, t, o)
	}
	return printNext(w, t, o, true)
}

// claimNext picks and starts the first ready item in one locked
// read-modify-write; -j prints the next -j JSON plus "claimed": true.
func claimNext(path string, w *Workspace, t *Tenant, o map[string]string) error {
	it, skipped, gs := pickNext(w, t, o)
	if it == nil {
		return nothingReady(o, skipped)
	}
	args := []string{it.ID, "-t", t.Slug}
	for _, k := range []string{"session", "agent"} {
		if o[k] != "" {
			args = append(args, "--"+k, o[k])
		}
	}
	role := value(o, "agent", "builder")
	if o["prompt"] == "true" && !validID(role) {
		return errors.New("agent role must be at most 64 lowercase letters, numbers, and single dashes")
	}
	id := it.ID
	if _, _, err := applyMove(path, w, "start", args); err != nil {
		return err
	}
	it, _ = itemByID(t, id)
	d := briefOf(w, t, *it, o["verbose"] == "true")
	if o["json"] == "true" {
		nb := nextBriefJSON{d, levelOf(gs), effectiveMap(gs), skipped, true, ""}
		if o["prompt"] == "true" {
			nb.Prompt = buildSessionPrompt(path, w, t, o["session"], role, true)
		}
		return printJSON(nb)
	}
	printBrief(d, o["verbose"] == "true")
	return nil
}

// nothingReady is the "no work" outcome of next: null (exit 0) with -j.
func nothingReady(o map[string]string, skipped int) error {
	if skipped > 0 && o["json"] == "true" {
		fmt.Fprintf(os.Stderr, "skipped: %d (%d without acceptance)\n", skipped, skipped)
	}
	if o["json"] == "true" {
		fmt.Println("null")
		return nil
	}
	if skipped > 0 {
		return fmt.Errorf("nothing ready (skipped %d without acceptance)", skipped)
	}
	return errors.New("nothing ready")
}

// nextAcceptance is the full effective acceptance when the acceptance gate is
// on, else the legacy one-line compact form (keeps gates-off output stable).
func nextAcceptance(t *Tenant, it Item, gs []gateInfo) string {
	if byName(gs, "acceptance").Effective {
		return strings.TrimSpace(shownAcceptance(t, it, gs))
	}
	return compactText(shownAcceptance(t, it, gs), 120)
}

func shownAcceptance(t *Tenant, it Item, gs []gateInfo) string {
	if byName(gs, "acceptance").Effective {
		return effectiveAcceptance(t, it)
	}
	return it.Acceptance
}

type nextBriefJSON struct {
	briefDoc
	Level   string          `json:"level"`
	Gates   map[string]bool `json:"gates"`
	Skipped int             `json:"skipped"`
	Claimed bool            `json:"claimed,omitempty"`
	Prompt  string          `json:"prompt,omitempty"`
}

func printNext(w *Workspace, t *Tenant, o map[string]string, withGates bool) error {
	it, skipped, gs := pickNext(w, t, o)
	if it == nil {
		return nothingReady(o, skipped)
	}
	d := briefOf(w, t, *it, o["verbose"] == "true")
	if o["json"] == "true" {
		if skipped > 0 {
			fmt.Fprintf(os.Stderr, "skipped: %d (%d without acceptance)\n", skipped, skipped)
		}
		return printJSON(nextBriefJSON{d, levelOf(gs), effectiveMap(gs), skipped, false, ""})
	}
	printBrief(d, o["verbose"] == "true")
	if withGates {
		if line := gatesLine(t, gs); line != "" {
			fmt.Println(line)
		}
	}
	if skipped > 0 {
		fmt.Printf("skipped %d without acceptance\n", skipped)
	}
	return nil
}

// detailOf builds the -v view: show -v lists the evidence files, list -v
// shows only the E mark (listed is true for list).
func detailOf(it Item, listed bool) itemDetail {
	files, mark := it.Evidence, ""
	if listed {
		files = nil
		if len(it.Evidence) > 0 {
			mark = "E"
		}
	}
	return itemDetail{it.ID, it.Kind, it.Status, it.Title, it.Parent, it.Wave, it.Assignee, it.Body, it.Acceptance, "", it.Note, it.Notes,
		files, mark, it.Judged, it.By, it.JudgedBy, it.Attempts, it.Created, it.Updated, it.Ticket}
}

func boardCommand(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return fmt.Errorf("unexpected argument %q", pos[0])
	}
	if err := allowOptions(o, "tenant", "session", "all", "verbose", "json", "html", "md", "job", "out", "tag", "archived"); err != nil {
		return err
	}
	w.showArchived = o["archived"] == "true"
	if o["md"] == "true" {
		if o["html"] == "true" || o["json"] == "true" || o["tenant"] != "" || o["session"] != "" {
			return errors.New("mw board --md takes only --job ID and -o PATH|-")
		}
		for i := range w.Tenants {
			if !w.Tenants[i].Hidden {
				announceTenant(w, &w.Tenants[i])
			}
		}
		return jobsMDCommand(path, w, o)
	}
	if o["job"] != "" {
		return errors.New("--job needs --md")
	}
	if o["out"] != "" && o["html"] != "true" {
		return errors.New("-o needs --html or --md")
	}
	if o["html"] == "true" {
		for i := range w.Tenants {
			if !w.Tenants[i].Hidden || w.showArchived {
				announceTenant(w, &w.Tenants[i])
			}
		}
		target := o["out"]
		if target == "" && w.Publish != nil {
			target = w.Publish.HTML
		}
		if target == "" {
			fmt.Print(renderHTML(w))
			return nil
		}
		if err := writeHTML(path, w, target); err != nil {
			return err
		}
		fmt.Println("wrote", htmlPath(path, target))
		return nil
	}
	if o["all"] == "true" {
		if o["tenant"] != "" || o["session"] != "" {
			return errors.New("mw board -A cannot be combined with -t or -s")
		}
		full := o["verbose"] == "true"
		var docs []boardDoc
		order := shown(w, o)
		last := map[*Tenant]time.Time{}
		for _, t := range order {
			x, _, _ := lastActivity(t)
			last[t] = parseTS(x)
		}
		sort.SliceStable(order, func(a, b int) bool { // most recently active first
			return last[order[a]].After(last[order[b]])
		})
		for i, t := range order {
			announceTenant(w, t)
			gs := gateStates(w, t)
			if o["json"] == "true" {
				docs = append(docs, boardData(t, w.allSessions(), full, gs))
				continue
			}
			printBoardJ(t, w.allSessions(), full, gatesLine(t, gs), byName(gs, "judge").Effective)
			if i < len(order)-1 {
				fmt.Println(strings.Repeat("─", 64))
			}
		}
		if o["json"] == "true" {
			return printJSON(docs)
		}
		return nil
	}
	t, err := selectTenant(w, o)
	if err != nil {
		return err
	}
	gs := gateStates(w, t)
	if o["json"] == "true" {
		return printJSON(boardData(t, w.allSessions(), o["verbose"] == "true", gs))
	}
	printBoardJ(t, w.allSessions(), o["verbose"] == "true", gatesLine(t, gs), byName(gs, "judge").Effective)
	return nil
}

// activityLine is "last: <ago> <item> <action> | today: N done".
func activityLine(t *Tenant) string {
	at, id, act := lastActivity(t)
	last := "-"
	if at != "" {
		last = ago(at) + " " + id + " " + act
	}
	return fmt.Sprintf("last: %s  today: %d done", last, doneToday(t))
}

// boardDoc is the -j form of the board.
type boardDoc struct {
	Last         string                 `json:"last"`
	LastAt       string                 `json:"last_at,omitempty"`
	Today        int                    `json:"today_done"`
	Tenant       string                 `json:"tenant"`
	Name         string                 `json:"name"`
	Done         int                    `json:"done"`
	Total        int                    `json:"total"`
	Archived     int                    `json:"archived,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
	Percent      int                    `json:"percent"`
	Level        string                 `json:"level"`
	Gates        map[string]bool        `json:"gates"`
	Sessions     []string               `json:"sessions"`
	Waves        []waveRow              `json:"waves"`
	States       map[string][]itemBrief `json:"states"`
	Needs        []needRow              `json:"needs"`
	ForAnswerers []needRow              `json:"for_answerers,omitempty"`
}

func boardData(t *Tenant, sessions []Session, full bool, gs []gateInfo) boardDoc {
	judgeOn := byName(gs, "judge").Effective
	at, id, act := lastActivity(t)
	d := boardDoc{LastAt: at, Today: doneToday(t), Tenant: t.Slug, Name: t.Name, Tags: t.Tags, Level: levelOf(gs), Gates: effectiveMap(gs), Sessions: []string{}, Waves: []waveRow{}, States: map[string][]itemBrief{}, Needs: needsOf(sessions, t), ForAnswerers: answererNeeds(sessions, t)}
	for _, it := range t.Items {
		if it.Kind == "wave" {
			if full || it.Status != "done" {
				d.Waves = append(d.Waves, waveRowOf(t, it))
			}
			continue
		}
		if full || it.Status == "doing" || it.Status == "blocked" || it.Status == "ready" {
			d.States[it.Status] = append(d.States[it.Status], itemBrief{it.ID, it.Kind, it.Status, compactText(it.Title, 72), it.Ticket})
		}
	}
	if at != "" {
		d.Last = id + " " + act
	}
	bd := burndownOf(t, judgeOn)
	d.Total, d.Done, d.Percent, d.Archived = bd.Total, bd.Judged, bd.Percent, t.Archived
	for _, s := range sessions {
		if s.Tenant == t.Slug && s.State != "stopped" && (full || s.State == "working") {
			label := s.ID + ":" + s.State
			if s.Item != "" {
				label += "(" + s.Item + ")"
			}
			d.Sessions = append(d.Sessions, label)
		}
	}
	return d
}

func printBoard(t *Tenant, sessions []Session, full bool, gates string) {
	printBoardJ(t, sessions, full, gates, false)
}

// printBoardJ prints the board; with the judge gate on, the percentage counts
// only done items that are judged.
func printBoardJ(t *Tenant, sessions []Session, full bool, gates string, judgeOn bool) {
	counts := map[string]int{}
	for _, it := range t.Items {
		if it.Kind != "wave" {
			counts[it.Status]++
		}
	}
	bd := burndownOf(t, judgeOn)
	total, done, pct := bd.Total, bd.Judged, bd.Percent
	fmt.Printf("%s [%s]%s  %d%% (%d/%d)\n", compactText(t.Name, 72), t.Slug, tagText(t), pct, done, total)
	fmt.Printf("[%s%s]\n", strings.Repeat("#", pct/5), strings.Repeat(".", 20-pct/5))
	var active []string
	for _, s := range sessions {
		if s.Tenant == t.Slug && s.State != "stopped" && (full || s.State == "working") {
			label := s.ID + ":" + s.State
			if s.Item != "" {
				label += "(" + s.Item + ")"
			}
			active = append(active, label)
		}
	}
	if len(active) > 0 {
		fmt.Println("sessions:", strings.Join(active, ", "))
	}
	fmt.Println(activityLine(t))
	if gates != "" {
		fmt.Println(gates)
	}
	fmt.Println()
	needs := needsOf(sessions, t)
	forAnswerers := answererNeeds(sessions, t)
	if len(needs) > 0 {
		fmt.Printf("NEEDS YOU (%d)\n", len(needs))
		printNeeds(needs)
		fmt.Println()
	}
	if len(forAnswerers) > 0 {
		printAnswererNeeds("FOR ANSWERERS", forAnswerers)
		fmt.Println()
	}
	shownWaves := false
	for _, it := range t.Items {
		if it.Kind == "wave" && (full || it.Status != "done") {
			if !shownWaves {
				fmt.Println("WAVES")
				shownWaves = true
			}
			r := waveRowOf(t, it)
			fmt.Printf("  %-8s %-7s %s  members %s %d%%, steps %s", it.ID, it.Status, compactText(it.Title, 60), r.Members, r.Percent, r.Steps)
			if r.Blocked > 0 {
				fmt.Printf(", %d blocked", r.Blocked)
			}
			fmt.Println()
		}
	}
	if shownWaves {
		fmt.Println()
	}
	states := []string{"doing", "blocked", "ready"}
	if full {
		states = append(states, "inbox", "done")
	}
	shownAny := false
	for _, state := range states {
		if counts[state] == 0 {
			continue
		}
		shownAny = true
		if state == "done" && t.Archived > 0 {
			fmt.Printf("%s (%d, +%d archived)\n", strings.ToUpper(state), counts[state], t.Archived)
		} else {
			fmt.Printf("%s (%d)\n", strings.ToUpper(state), counts[state])
		}
		shown := 0
		for _, it := range t.Items {
			if it.Status == state && it.Kind != "wave" {
				if !full && shown == 5 {
					fmt.Printf("  … %d more\n", counts[state]-shown)
					break
				}
				parent, owner := "", ""
				if it.Parent != "" {
					parent = " ← " + it.Parent
				}
				if it.Assignee != "" {
					owner = " @" + displayName(it.Assignee)
				}
				age := ""
				if state == "doing" {
					age = "  doing " + ageShort(it.Updated)
				}
				fmt.Printf("  %-8s %-7s %s%s%s%s\n", it.ID, it.Kind, compactText(it.Title, 72), parent, owner, age)
				shown++
			}
		}
		fmt.Println()
	}
	if !full && !shownAny && !shownWaves && len(needs) == 0 && len(forAnswerers) == 0 {
		fmt.Println("quiet")
	}
}

func contextCommand(w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return fmt.Errorf("unexpected argument %q", pos[0])
	}
	if err := allowOptions(o, "tenant", "session", "json"); err != nil {
		return err
	}
	t, err := selectTenant(w, o)
	if err != nil {
		return err
	}
	gs := gateStates(w, t)
	if o["json"] == "true" {
		it, skipped, _ := pickNext(w, t, map[string]string{})
		var next any
		if it != nil {
			next = nextOf(w, t, *it, gs, skipped)
		}
		return printJSON(struct {
			Tenant  string          `json:"tenant"`
			Name    string          `json:"name"`
			Session string          `json:"session,omitempty"`
			Guide   string          `json:"guide"`
			Folders string          `json:"folders"`
			Level   string          `json:"level"`
			Gates   map[string]bool `json:"gates"`
			Next    any             `json:"next"`
			Board   boardDoc        `json:"board"`
		}{t.Slug, t.Name, value(o, "session", os.Getenv("MW_SESSION")), contextGuide,
			folderSentence(workspaceFolders(w), t.Slug, value(o, "session", os.Getenv("MW_SESSION"))),
			levelOf(gs), effectiveMap(gs), next, boardData(t, w.allSessions(), true, gs)})
	}
	fmt.Printf("# miniwaves context: %s [%s]\n\n", compactText(t.Name, 72), t.Slug)
	if s := value(o, "session", os.Getenv("MW_SESSION")); s != "" {
		fmt.Println("Session:", s)
	}
	fmt.Println(contextGuide)
	f := workspaceFolders(w)
	fmt.Println(folderSentence(f, t.Slug, value(o, "session", os.Getenv("MW_SESSION"))), "Run mw dirs.")
	fmt.Println()
	if err := printNext(w, t, map[string]string{}, false); err != nil {
		fmt.Println("Next: nothing ready; refine an inbox item or ask the user.")
	}
	fmt.Println("\nCurrent board:")
	printBoardJ(t, w.allSessions(), true, gatesLine(t, gs), byName(gs, "judge").Effective)
	return nil
}

const contextGuide = "prompt=request; story=outcome; task/spike/bug/chore=work. Claim one ready item. Finish with verification; block with reason. Never close a parent with open children."

// ---- waves: milestone bundles with a delivery protocol ----

// ---- implicit local session (one per tenant) ----

const localName = "local"

func localID(slug string) string { return localName + "@" + slug }

func isLocalID(id string) bool { return strings.HasPrefix(id, localName+"@") }

// displayName shows the implicit session as plain "local".
func displayName(id string) string {
	if isLocalID(id) {
		return localName
	}
	return id
}

// syncLocal derives each implicit session's item and state from a scan of its
// tenant's doing items, so it may own many and never goes stale.
func syncLocal(w *Workspace) {
	for i := range w.Sessions {
		s := &w.Sessions[i]
		if !isLocalID(s.ID) {
			continue
		}
		if s.State == "stopped" {
			continue
		}
		s.Item, s.State = "", "idle"
		for _, t := range w.Tenants {
			if t.Slug != s.Tenant {
				continue
			}
			for _, it := range t.Items {
				if it.Assignee == s.ID && it.Status == "doing" {
					s.Item, s.State = it.ID, "working"
					break
				}
			}
		}
	}
}

// wipSessionOn reports the opt-in per-user key wip.session (default off; mw configure wip.session=true).
// A missing or unreadable user config means off: a broken file never blocks a board command.
func wipSessionOn() bool {
	c, _, _, err := readUserConfig()
	if err != nil {
		return false
	}
	v, _ := userValue(c, userKeyByName("wip.session"))
	return v == "true"
}
