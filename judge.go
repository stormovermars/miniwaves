package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// ---- judgement: review, judge, accept, reject ----

func storyAncestor(t *Tenant, it Item) *Item {
	cur := it
	for i := 0; i <= len(t.Items) && cur.Parent != ""; i++ {
		p, err := itemByID(t, cur.Parent)
		if err != nil {
			return nil
		}
		if p.Kind == "story" {
			return p
		}
		cur = *p
	}
	return nil
}

// judgeable = a story, or a work item with no story ancestor.
func judgeable(t *Tenant, it Item) bool {
	if it.Kind == "story" {
		return true
	}
	return isWorkKind(it.Kind) && storyAncestor(t, it) == nil
}

func childrenOf(t *Tenant, id string) []Item {
	var out []Item
	for _, c := range t.Items {
		if c.Parent == id && c.Kind != "wave" {
			out = append(out, c)
		}
	}
	return out
}

// storyReady: every child is done (a childless story must itself be done).
// verb is the refused action ("accept", "reject") for the message.
func storyReady(t *Tenant, st Item, verb string) error {
	kids := childrenOf(t, st.ID)
	var open []string
	for _, c := range kids {
		if c.Status != "done" {
			open = append(open, c.ID+" "+c.Status)
		}
	}
	if len(open) > 0 {
		return fmt.Errorf("cannot %s %s: %d of %d children not done (%s); wait for them, or reject a done child", verb, st.ID, len(open), len(kids), strings.Join(open, ", "))
	}
	if len(kids) == 0 && st.Status != "done" {
		return fmt.Errorf("%s is not done", st.ID)
	}
	return nil
}

// judgementOwner is the item whose judgement covers it: itself when
// judgeable, else its story ancestor.
func judgementOwner(t *Tenant, it Item) Item {
	if judgeable(t, it) {
		return it
	}
	if st := storyAncestor(t, it); st != nil {
		return *st
	}
	return it
}

// isJudged treats ungated as judged; prompts and waves are never judged.
func isJudged(t *Tenant, it Item) bool {
	if it.Kind == "prompt" || it.Kind == "wave" {
		return true
	}
	return judgementOwner(t, it).Judged != ""
}

// countsDone is the burndown rule: done, and judged when the judge gate is on.
func countsDone(t *Tenant, it Item, judgeOn bool) bool {
	return it.Status == "done" && (!judgeOn || isJudged(t, it))
}

func awaitingJudgement(t *Tenant, it Item) bool {
	if !judgeable(t, it) || it.Judged != "" {
		return false
	}
	if it.Kind == "story" {
		return storyReady(t, it, "") == nil
	}
	return it.Status == "done"
}

func humanName(cmd string, o map[string]string) (string, error) {
	if err := refuseSession(o, cmd); err != nil {
		return "", err
	}
	return humanBy(cmd, o)
}

// humanBy is --by NAME, else the login name when stdin is a terminal.
func humanBy(cmd string, o map[string]string) (string, error) {
	by := strings.TrimSpace(o["by"])
	if by == "" {
		if !stdinIsTerminal() {
			return "", fmt.Errorf("%s needs --by NAME when stdin is not a terminal", cmd)
		}
		by = value(map[string]string{"u": firstEnv("USER", "USERNAME")}, "u", "human")
	}
	return by, nil
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func releaseOwner(w *Workspace, it *Item) {
	if it.Assignee != "" {
		if owner, e := sessionByID(w, it.Assignee); e == nil && !isLocalID(owner.ID) {
			owner.State, owner.Item = "idle", ""
		}
		it.Assignee = ""
	}
}

func clearJudgement(it *Item) { it.Judged, it.JudgedBy, it.Evidence = "", "", nil }

// ---- review ----

type reviewItem struct {
	ID       string `json:"id" toon:"id"`
	Type     string `json:"type" toon:"type"`
	State    string `json:"state" toon:"state"`
	Title    string `json:"title" toon:"title"`
	Attempts int    `json:"attempts,omitempty" toon:"attempts,omitempty"`
	Note     string `json:"note,omitempty" toon:"note,omitempty"`
}

type reviewDoc struct {
	Judge      []reviewItem `json:"judge"`
	Blocked    []reviewItem `json:"blocked"`
	Acceptance []reviewItem `json:"acceptance"`
	Waves      []waveRow    `json:"waves"`
	Stale      []reviewItem `json:"stale"`
	Needs      []needRow    `json:"needs"`
}

func reviewOf(w *Workspace, t *Tenant) (reviewDoc, bool) {
	gs := gateStates(w, t)
	judgeOn, accOn := byName(gs, "judge").Effective, byName(gs, "acceptance").Effective
	evOn := byName(gs, "evidence").Effective
	d := reviewDoc{Judge: []reviewItem{}, Blocked: []reviewItem{}, Acceptance: []reviewItem{}, Waves: []waveRow{}, Stale: []reviewItem{}, Needs: needsOf(t)}
	mk := func(it Item) reviewItem {
		return reviewItem{ID: it.ID, Type: it.Kind, State: it.Status, Title: compactText(it.Title, 72), Attempts: it.Attempts}
	}
	for _, it := range t.Items {
		if it.Kind == "wave" {
			continue
		}
		if it.Status == "doing" {
			if at := parseTS(it.Updated); !at.IsZero() && time.Since(at) > staleDoingAge {
				r := mk(it)
				r.Note = "doing " + ageShort(it.Updated)
				if it.Assignee != "" {
					r.Note += " @" + displayName(it.Assignee)
				}
				d.Stale = append(d.Stale, r)
			}
		}
		if judgeOn && awaitingJudgement(t, it) {
			d.Judge = append(d.Judge, mk(it))
		}
		if it.Status == "blocked" {
			r := mk(it)
			r.Note = compactText(it.Note, 72)
			d.Blocked = append(d.Blocked, r)
		}
		if accOn && it.Status != "done" && (it.Kind == "story" || isWorkKind(it.Kind)) && strings.TrimSpace(effectiveAcceptance(t, it)) == "" {
			d.Acceptance = append(d.Acceptance, mk(it))
		}
	}
	for _, wv := range t.Items {
		if wv.Kind == "wave" && wv.Status != "done" && waveCloseable(w, t, wv, judgeOn, evOn) {
			d.Waves = append(d.Waves, waveRowOf(t, wv))
		}
	}
	return d, accOn
}

// waveCloseable: steps ticked, members done (and judged when the judge gate is on),
// and the wave's evidence folder present when the evidence gate is on (as wave close demands).
func waveCloseable(w *Workspace, t *Tenant, wv Item, judgeOn, evOn bool) bool {
	if evOn {
		if _, err := checkEvidence(w, t, wv.ID); err != nil {
			return false
		}
	}
	for _, s := range wv.Steps {
		if !s.Done {
			return false
		}
	}
	for _, m := range waveMembers(t, wv.ID) {
		if m.Status != "done" || (judgeOn && !isJudged(t, m)) {
			return false
		}
	}
	for _, c := range t.Items {
		if c.Kind == "wave" && c.Parent == wv.ID && c.Status != "done" {
			return false
		}
	}
	return true
}

func reviewCommand(w *Workspace, args []string) error {
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
	d, accOn := reviewOf(w, t)
	if o["json"] == "true" {
		return printJSON(d)
	}
	section := func(name string, n int, rows func()) {
		if n == 0 {
			fmt.Printf("%s: []\n", name)
			return
		}
		fmt.Printf("%s (%d)\n", name, n)
		rows()
	}
	items := func(list []reviewItem) func() {
		return func() {
			for _, r := range list {
				att, note := "", ""
				if r.Attempts > 0 {
					att = fmt.Sprintf(" ×%d", r.Attempts)
				}
				if r.Note != "" {
					note = " — " + r.Note
				}
				fmt.Printf("  %-8s %-7s %s%s%s\n", r.ID, r.Type, r.Title, att, note)
			}
		}
	}
	if len(d.Stale) > 0 {
		fmt.Printf("STALE (%d)\n", len(d.Stale))
		items(d.Stale)()
	}
	if len(d.Needs) > 0 {
		fmt.Printf("needs you (%d)\n", len(d.Needs))
		printNeeds(d.Needs)
	}
	section("judge", len(d.Judge), items(d.Judge))
	section("blocked", len(d.Blocked), items(d.Blocked))
	if accOn {
		section("acceptance", len(d.Acceptance), items(d.Acceptance))
	}
	section("waves", len(d.Waves), func() {
		for _, r := range d.Waves {
			fmt.Printf("  %-8s %s  members %s, steps %s\n", r.ID, r.Title, r.Members, r.Steps)
		}
	})
	return nil
}

// ---- judge ----

func evidenceSummary(w *Workspace, t *Tenant, it Item) string {
	s := evidenceFolder(w, t, it.ID)
	if len(it.Evidence) == 0 {
		return s + " (none recorded)"
	}
	return fmt.Sprintf("%s (%s)", s, strings.Join(it.Evidence, ", "))
}

func checklist(acc string) []string {
	acc = strings.TrimSpace(acc)
	if acc == "" {
		return []string{"[ ] (no acceptance set)"}
	}
	var parts []string
	for _, line := range strings.FieldsFunc(acc, func(r rune) bool { return r == '\n' || r == ';' }) {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "-*[] "))
		if line != "" {
			parts = append(parts, "[ ] "+line)
		}
	}
	return parts
}

func judgeCommand(w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: mw judge ID [-t SLUG]")
	}
	if err := allowOptions(o, "tenant", "session"); err != nil {
		return err
	}
	t, err := selectTenant(w, o)
	if err != nil {
		return err
	}
	it, err := itemByID(t, pos[0])
	if err != nil {
		return err
	}
	if !judgeable(t, *it) {
		return fmt.Errorf("%s is not judgeable: judge its story or a standalone work item", it.ID)
	}
	verification := "-"
	if it.Note != "" {
		verification = compactText(it.Note, 120)
	}
	out := fmt.Sprintf("Outcome: %s %s [%s] verification: %s", it.ID, compactText(it.Title, 72), it.Status, verification)
	ev := []string{evidenceSummary(w, t, *it)}
	kids := childrenOf(t, it.ID)
	if it.Kind == "story" {
		out = fmt.Sprintf("Outcome: %s %s [%s] children %d/%d done", it.ID, compactText(it.Title, 72), it.Status, doneCount(kids), len(kids))
		for _, c := range kids {
			ev = append(ev, fmt.Sprintf("%s: %s", c.ID, evidenceSummary(w, t, c)))
		}
	}
	fmt.Println(out)
	fmt.Println("Evidence:")
	for _, l := range ev {
		fmt.Println("  " + l)
	}
	fmt.Println("Decision:")
	for _, l := range checklist(effectiveAcceptance(t, *it)) {
		fmt.Println("  " + l)
	}
	return nil
}

func doneCount(items []Item) int {
	n := 0
	for _, c := range items {
		if c.Status == "done" {
			n++
		}
	}
	return n
}

// ---- accept ----

func acceptCommand(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: mw accept ID [-t SLUG] [--by NAME]")
	}
	if err := allowOptions(o, "tenant", "session", "by"); err != nil {
		return err
	}
	by, err := humanName("mw accept", o)
	if err != nil {
		return err
	}
	t, err := selectTenant(w, o)
	if err != nil {
		return err
	}
	it, err := itemByID(t, pos[0])
	if err != nil {
		return err
	}
	if !judgeable(t, *it) {
		return fmt.Errorf("%s is not judgeable: accept its story or a standalone work item", it.ID)
	}
	if it.Judged != "" && it.Judged != "ungated" {
		return fmt.Errorf("%s is already accepted (%s by %s)", it.ID, it.Judged, it.JudgedBy)
	}
	if it.Kind == "story" {
		if err := storyReady(t, *it, "accept"); err != nil {
			return err
		}
	} else if it.Status != "done" {
		return fmt.Errorf("%s is not done (it is %s)", it.ID, it.Status)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if it.Status != "done" {
		releaseOwner(w, it)
		it.Status = "done"
	}
	it.Judged, it.JudgedBy, it.Updated = now, by, now
	emit(it, now, "accept", by, "")
	if strings.HasPrefix(it.Note, "rejected") {
		it.Note = "" // the rejection is history (kept in Notes), not the current state
	}
	t.AcceptStreak++
	if err := save(path, w); err != nil {
		return err
	}
	fmt.Printf("[%s] %s  accepted by %s\n", t.Slug, it.ID, by)
	return nil
}

// ---- reject ----

func closedWaveOf(t *Tenant, it Item) (string, bool) {
	if it.Wave == "" {
		return "", false
	}
	wv, err := itemByID(t, it.Wave)
	return it.Wave, err == nil && wv.Status == "done"
}

func rejectCommand(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return errors.New("usage: mw reject ID REASON... [--evidence-gap] [-t SLUG] [--by NAME]")
	}
	if err := allowOptions(o, "tenant", "session", "by", "evidence-gap"); err != nil {
		return err
	}
	by, err := humanName("mw reject", o)
	if err != nil {
		return err
	}
	t, err := selectTenant(w, o)
	if err != nil {
		return err
	}
	it, err := itemByID(t, pos[0])
	if err != nil {
		return err
	}
	reason := strings.Join(pos[1:], " ")
	if it.Kind != "story" && !isWorkKind(it.Kind) {
		return fmt.Errorf("%s is a %s; only stories and work items are rejected", it.ID, it.Kind)
	}
	if it.Kind == "story" {
		if err := storyReady(t, *it, "reject"); err != nil {
			return err
		}
	} else if it.Status != "done" {
		return fmt.Errorf("%s is not done (it is %s)", it.ID, it.Status)
	}
	story := storyAncestor(t, *it)
	if id, closed := closedWaveOf(t, *it); closed {
		return fmt.Errorf("wave %s is closed; reopen wave first", id)
	}
	if story != nil {
		if id, closed := closedWaveOf(t, *story); closed {
			return fmt.Errorf("wave %s is closed (story %s); reopen wave first", id, story.ID)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	reset := func(x *Item) {
		releaseOwner(w, x)
		x.Status, x.Updated, x.Note = "ready", now, "rejected: "+reason
		x.Attempts++
		x.Notes = append(x.Notes, now+" rejected by "+by+": "+reason)
		emit(x, now, "reject", by, reason)
		clearJudgement(x)
	}
	var fix *Item
	reset(it)
	if it.Kind == "story" {
		t.Next["TSK"]++
		t.Items = append(t.Items, Item{ID: fmt.Sprintf("TSK-%03d", t.Next["TSK"]), Kind: "task", Title: "Fix: " + reason,
			Status: "ready", Parent: it.ID, Created: now, Updated: now})
		fix = &t.Items[len(t.Items)-1]
		emit(fix, now, "created", by, "")
		emit(fix, now, "ready", by, "")
		it, _ = itemByID(t, pos[0]) // append may have moved the slice
	} else if story != nil {
		story, _ = itemByID(t, story.ID)
		if story.Status == "done" {
			reset(story)
			story.Note = "rejected via " + it.ID + ": " + reason
		}
	}
	t.AcceptStreak = 0
	if o["evidence-gap"] == "true" && t.GateLatch["evidence"] == "" {
		setLatch(t, "evidence", "reject "+it.ID)
	}
	if err := save(path, w); err != nil {
		return err
	}
	fmt.Printf("[%s] %s  %-7s %s\n", t.Slug, it.ID, it.Status, compactText(it.Title, 72))
	if fix != nil {
		fmt.Printf("[%s] %s  %-7s %s\n", t.Slug, fix.ID, fix.Status, compactText(fix.Title, 72))
	}
	return nil
}
