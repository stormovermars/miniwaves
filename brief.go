package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ---- activity log ----

// emit appends one event to the item's activity log.
// Events are stamped RFC3339Nano so same-second events sort by real time; the
// at argument only documents the caller's second-resolution stamp.
func emit(it *Item, at, action, by, note string) {
	at = time.Now().UTC().Format(time.RFC3339Nano)
	it.Events = append(it.Events, Event{At: at, Action: action, By: by, Note: strings.TrimSpace(note)})
}

type logRow struct {
	At      string `json:"ts" toon:"ts"`
	Tenant  string `json:"tenant" toon:"tenant"`
	Item    string `json:"item" toon:"item"`
	Action  string `json:"action" toon:"action"`
	By      string `json:"by" toon:"by"`
	Note    string `json:"note" toon:"note"`
	Key     string `json:"key,omitempty" toon:"key,omitempty"` // tenant/item/content hash, set by log --since
	sortKey time.Time
}

func parseTS(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// shortTS renders an RFC3339 stamp as local "MM-DD HH:MM"; unparsable input is returned as is.
func shortTS(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.Local().Format("01-02 15:04")
}

// ago renders the age of a stamp compactly: 5m, 3h, 2d.
func ago(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return "?"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func tenantRows(t *Tenant) []logRow {
	var rows []logRow
	for _, it := range t.Items {
		for _, e := range it.Events {
			rows = append(rows, logRow{At: e.At, Tenant: t.Slug, Item: it.ID, Action: e.Action, By: e.By, Note: e.Note, sortKey: parseTS(e.At)})
		}
	}
	return rows
}

// lastActivity is the newest event of a tenant: its stamp, item and action.
func lastActivity(t *Tenant) (string, string, string) {
	var at, id, act string
	var best time.Time
	for _, r := range tenantRows(t) {
		if at == "" || !r.sortKey.Before(best) {
			best, at, id, act = r.sortKey, r.At, r.Item, r.Action
		}
	}
	return at, id, act
}

// doneToday counts today's (local) done events of non-wave items.
func doneToday(t *Tenant) int {
	today := time.Now().Format("2006-01-02")
	n := 0
	for _, it := range t.Items {
		if it.Kind == "wave" {
			continue
		}
		for _, e := range it.Events {
			if e.Action == "done" && parseTS(e.At).Local().Format("2006-01-02") == today {
				n++
			}
		}
	}
	return n
}

func logCommand(w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return fmt.Errorf("unexpected argument %q", pos[0])
	}
	if err := allowOptions(o, "tenant", "session", "all", "limit", "json", "tag", "since", "since-file", "follow", "exec", "poll"); err != nil {
		return err
	}
	if o["follow"] == "true" {
		return logFollowCommand(o)
	}
	if o["exec"] == "true" || o["poll"] != "" {
		return errors.New("--exec and --poll need --follow")
	}
	_, since := o["since"]
	_, sinceFile := o["since-file"]
	if since || sinceFile {
		return logSinceCommand(w, o)
	}
	limit := 20
	if o["limit"] != "" {
		if _, e := fmt.Sscanf(o["limit"], "%d", &limit); e != nil || limit < 1 {
			return errors.New("-n needs a positive number")
		}
	}
	var rows []logRow
	if o["all"] == "true" {
		if o["tenant"] != "" {
			return errors.New("mw log -A cannot be combined with -t")
		}
		for _, t := range shown(w, o) {
			announceTenant(w, t)
			rows = append(rows, tenantRows(t)...)
		}
	} else {
		t, err := selectTenant(w, o)
		if err != nil {
			return err
		}
		rows = tenantRows(t)
	}
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 { // same-second ties: later events first
		rows[i], rows[j] = rows[j], rows[i]
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].sortKey.After(rows[j].sortKey) })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	if o["json"] == "true" {
		if rows == nil {
			rows = []logRow{}
		}
		return printJSON(rows)
	}
	for _, r := range rows {
		by := r.By
		if by == "" {
			by = "-"
		}
		line := fmt.Sprintf("%s  %s  %-8s %-9s %s", shortTS(r.At), r.Tenant, r.Item, r.Action, by)
		if r.Note != "" {
			line += "  " + compactText(r.Note, 90)
		}
		fmt.Println(line)
	}
	return nil
}

// ---- the full brief: show and next ----

type briefDoc struct {
	ID             string      `json:"id"`
	Type           string      `json:"type"`
	State          string      `json:"state"`
	Title          string      `json:"title"`
	Ticket         string      `json:"ticket,omitempty"`
	Parent         string      `json:"parent,omitempty"`
	Wave           string      `json:"wave,omitempty"`
	Session        string      `json:"session,omitempty"`
	Body           string      `json:"body,omitempty"`
	Acceptance     string      `json:"acceptance"`
	AcceptanceFrom string      `json:"acceptance_from,omitempty"`
	EvidenceDir    string      `json:"evidence_dir"`
	EvidenceFiles  int         `json:"evidence_files"`
	Evidence       []string    `json:"evidence,omitempty"`
	Children       []itemBrief `json:"children"`
	Note           string      `json:"note,omitempty"`
	Notes          []string    `json:"notes"`
	Judged         string      `json:"judged,omitempty"`
	By             string      `json:"by,omitempty"`
	JudgedBy       string      `json:"judged_by,omitempty"`
	Attempts       int         `json:"attempts,omitempty"`
	Events         []Event     `json:"events"`
	Created        string      `json:"created,omitempty"`
	Updated        string      `json:"updated,omitempty"`
}

const eventsTail = 5

// decisionEventsKept caps each of reject, answer and decide in a brief.
const decisionEventsKept = 10

// briefEvents is the event list a brief shows. TSK-080: reject, answer and decide events are kept (a
// send-back reason or a decision must not fall out of the brief), the last decisionEventsKept of each
// kind; the rest stay capped at the last eventsTail. Chronological order is preserved.
func briefEvents(evs []Event) []Event {
	isDecision := func(a string) bool { return a == "reject" || a == "answer" || a == "decide" }
	seen := map[string]int{} // newer events of each kind, counted from the end
	keep := make([]bool, len(evs))
	for i := len(evs) - 1; i >= 0; i-- {
		a := evs[i].Action
		if isDecision(a) {
			seen[a]++
			keep[i] = seen[a] <= decisionEventsKept || i >= len(evs)-eventsTail
		} else {
			keep[i] = i >= len(evs)-eventsTail
		}
	}
	out := []Event{}
	for i, e := range evs {
		if keep[i] {
			out = append(out, e)
		}
	}
	return out
}

func countEvidence(dir string) int {
	root := dir
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		root = r
	}
	n := 0
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p != root && ignoredEvidenceName(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info, e := os.Stat(p); e == nil && info.Mode().IsRegular() && info.Size() > 0 {
			n++
		}
		return nil
	})
	return n
}

func briefOf(w *Workspace, t *Tenant, it Item, verbose bool) briefDoc {
	a, src := acceptanceSource(t, it)
	d := briefDoc{ID: it.ID, Type: it.Kind, State: it.Status, Title: compactText(it.Title, 72), Ticket: it.Ticket, Parent: it.Parent,
		Wave: it.Wave, Session: it.Assignee, Body: it.Body, Acceptance: strings.TrimSpace(a),
		Children: []itemBrief{}, Note: it.Note, Notes: []string{}, Judged: it.Judged, By: it.By, JudgedBy: it.JudgedBy,
		Attempts: it.Attempts, Evidence: it.Evidence}
	if src != "" && src != it.ID {
		d.AcceptanceFrom = src
	}
	d.EvidenceDir = evidenceFolder(w, t, it.ID)
	d.EvidenceFiles = countEvidence(d.EvidenceDir)
	for _, c := range t.Items {
		if c.Parent == it.ID || (it.Kind == "wave" && c.Wave == it.ID) {
			d.Children = append(d.Children, itemBrief{c.ID, c.Kind, c.Status, compactText(c.Title, 72), c.Ticket})
		}
	}
	d.Notes = append(d.Notes, it.Notes...)
	d.Events = briefEvents(it.Events)
	if verbose {
		d.Created, d.Updated = it.Created, it.Updated
	}
	return d
}

// printBrief is the one-call human/assistant view of an item.
func printBrief(d briefDoc, verbose bool) { printBriefFull(d, verbose, false) }

// printBriefFull is printBrief; full prints event notes uncut.
func printBriefFull(d briefDoc, verbose, full bool) {
	stamp := shortTS
	if verbose {
		stamp = func(s string) string { return s }
	}
	fmt.Printf("%s  %s  %s  %s\n", d.ID, d.Type, d.State, d.Title)
	var meta []string
	for _, kv := range [][2]string{{"parent", d.Parent}, {"wave", d.Wave}, {"session", d.Session}} {
		if kv[1] != "" {
			meta = append(meta, kv[0]+": "+kv[1])
		}
	}
	if len(meta) > 0 {
		fmt.Println(strings.Join(meta, "  "))
	}
	block := func(head, text string) {
		fmt.Println(head)
		for _, l := range strings.Split(strings.TrimSpace(text), "\n") {
			fmt.Println("  " + strings.TrimRight(l, " \t\r"))
		}
	}
	if strings.TrimSpace(d.Body) != "" {
		block("body:", d.Body)
	}
	if d.Acceptance != "" {
		head := "acceptance:"
		if d.AcceptanceFrom != "" {
			head = "acceptance (from " + d.AcceptanceFrom + "):"
		}
		block(head, d.Acceptance)
	} else {
		fmt.Println("acceptance: none")
	}
	fmt.Printf("evidence: %s (%d files)\n", d.EvidenceDir, d.EvidenceFiles)
	for _, f := range d.Evidence {
		fmt.Println("  " + f)
	}
	if d.Judged != "" || d.By != "" || d.JudgedBy != "" || d.Attempts > 0 {
		fmt.Printf("judged: %s  by: %s  accepted-by: %s  attempts: %d\n", orDash(d.Judged), orDash(d.By), orDash(d.JudgedBy), d.Attempts)
	}
	if d.Note != "" {
		fmt.Println("note:", d.Note)
	}
	if len(d.Children) > 0 {
		fmt.Println("children:")
		for _, c := range d.Children {
			fmt.Printf("  %-8s %-7s %-7s %s\n", c.ID, c.Type, c.State, c.Title)
		}
	}
	if len(d.Notes) > 0 {
		fmt.Println("notes:")
		for _, n := range d.Notes {
			if i := strings.IndexByte(n, ' '); i > 0 {
				n = stamp(n[:i]) + n[i:]
			}
			fmt.Println("  " + n)
		}
	}
	if len(d.Events) > 0 {
		fmt.Println("events:")
		for _, e := range d.Events {
			line := fmt.Sprintf("  %s  %-9s %s", stamp(e.At), e.Action, orDash(e.By))
			if e.Note != "" {
				if full {
					line += "  " + e.Note
				} else {
					line += "  " + compactText(e.Note, 90)
				}
			}
			fmt.Println(line)
		}
	}
	if verbose {
		fmt.Printf("created: %s  updated: %s\n", d.Created, d.Updated)
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// ---- done --evidence: copy proof into the evidence folder ----

// splitEvidenceArgs removes every "--evidence PATH" / "--evidence=PATH" pair
// from args and returns the paths. Each flag takes exactly one path, so a
// following word is a note, not another path.
func splitEvidenceArgs(args []string) ([]string, []string, error) {
	var rest, paths []string
	for i := 0; i < len(args); i++ {
		switch {
		case strings.HasPrefix(args[i], "--evidence="):
			p := strings.TrimPrefix(args[i], "--evidence=")
			if p == "" {
				return nil, nil, errors.New("--evidence needs a path")
			}
			paths = append(paths, p)
		case args[i] == "--evidence":
			if i+1 >= len(args) || args[i+1] == "" || (strings.HasPrefix(args[i+1], "-") && len(args[i+1]) > 1) {
				return nil, nil, errors.New("--evidence needs a path (one per flag; repeat the flag for more)")
			}
			paths = append(paths, args[i+1])
			i++
		default:
			rest = append(rest, args[i])
		}
	}
	return rest, paths, nil
}

func copyFile(src, dst string) error {
	if a, e1 := filepath.Abs(src); e1 == nil {
		if b, e2 := filepath.Abs(dst); e2 == nil && a == b {
			return nil
		}
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// copyEvidence copies files (or the files of folders, recursively, skipping
// junk and symlinks inside folders) into dest. Every path is checked and every
// destination name planned before anything is copied; two sources landing on
// the same name are refused. It returns the copied files as "rel/path size".
func copyEvidence(paths []string, dest string) ([]string, error) {
	type job struct{ src, dst, rel string }
	var jobs []job
	seen := map[string]string{}
	add := func(src, rel string) error {
		if prev, dup := seen[rel]; dup {
			return fmt.Errorf("--evidence: %s and %s would both become %s", prev, src, rel)
		}
		seen[rel] = src
		jobs = append(jobs, job{src, filepath.Join(dest, rel), rel})
		return nil
	}
	for _, p := range paths {
		if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
			if h, err := os.UserHomeDir(); err == nil {
				p = filepath.Join(h, p[2:])
			}
		}
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("evidence path %s: %v", p, err)
		}
		if !info.IsDir() {
			if err := add(p, filepath.Base(p)); err != nil {
				return nil, err
			}
			continue
		}
		root := p
		if r, e := filepath.EvalSymlinks(p); e == nil {
			root = r
		}
		err = filepath.WalkDir(root, func(f string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if f != root && ignoredEvidenceName(d.Name()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() { // directories, symlinks, devices
				return nil
			}
			rel, _ := filepath.Rel(root, f)
			return add(f, rel)
		})
		if err != nil {
			return nil, err
		}
	}
	if len(jobs) == 0 {
		return nil, errors.New("--evidence: no files found to copy")
	}
	sort.Slice(jobs, func(a, b int) bool { return jobs[a].rel < jobs[b].rel })
	var out []string
	for _, j := range jobs {
		if err := copyFile(j.src, j.dst); err != nil {
			return out, err
		}
		size := int64(0)
		if fi, e := os.Stat(j.dst); e == nil {
			size = fi.Size()
		}
		out = append(out, fmt.Sprintf("%s %d", filepath.ToSlash(j.rel), size))
	}
	return out, nil
}

// logSinceCommand is mw log --since CURSOR: events the cursor has not delivered, oldest first, plus the next cursor.
func logSinceCommand(w *Workspace, o map[string]string) error {
	limit := 0 // a feed is complete unless -n says otherwise
	if o["limit"] != "" {
		if _, e := fmt.Sscanf(o["limit"], "%d", &limit); e != nil || limit < 1 {
			return errors.New("-n needs a positive number")
		}
	}
	var tenants []*Tenant
	if o["all"] == "true" {
		if o["tenant"] != "" {
			return errors.New("mw log -A cannot be combined with -t")
		}
		for _, t := range shown(w, o) {
			announceTenant(w, t)
			tenants = append(tenants, t)
		}
	} else {
		t, err := selectTenant(w, o)
		if err != nil {
			return err
		}
		tenants = []*Tenant{t}
	}
	cursor := o["since"]
	sf := o["since-file"]
	if _, both := o["since"]; both && sf != "" {
		return errors.New("mw log: use --since or --since-file, not both")
	}
	if _, ok := o["since-file"]; ok && sf == "" {
		return errors.New("--since-file needs a path")
	}
	if sf != "" {
		cursor = "start"
		if b, err := os.ReadFile(sf); err == nil {
			cursor = string(b)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	rows, next, err := logSince(tenants, cursor, limit)
	if err != nil {
		return err
	}
	if sf != "" {
		if err := writeFileAtomic(sf, []byte(next+"\n")); err != nil {
			return err
		}
	}
	if o["json"] == "true" {
		return printJSON(struct {
			Events []logRow `json:"events"`
			Next   string   `json:"next"`
		}{rows, next})
	}
	for _, r := range rows {
		by := r.By
		if by == "" {
			by = "-"
		}
		line := fmt.Sprintf("%s  %s  %-8s %-9s %s", shortTS(r.At), r.Tenant, r.Item, r.Action, by)
		if r.Note != "" {
			line += "  " + compactText(r.Note, 90)
		}
		fmt.Println(line)
	}
	fmt.Println("next: " + next)
	return nil
}
