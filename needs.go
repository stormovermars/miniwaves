package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Needs-human and decisions are item events (ask, answer, decide), ungated and valid on any board.
// An open ask is the last ask of an item with no later answer on that item.

// needRow is one open question: what review -j lists under "needs".
type needRow struct {
	Tenant   string `json:"tenant" toon:"tenant"`
	Item     string `json:"item" toon:"item"`
	Question string `json:"question" toon:"question"`
	By       string `json:"by" toon:"by"`
	At       string `json:"at" toon:"at"`
}

// openAsk returns the item's open ask: its last ask event when no answer follows it.
func openAsk(it Item) (Event, bool) {
	var ask Event
	open := false
	for _, e := range it.Events {
		switch e.Action {
		case "ask":
			ask, open = e, true
		case "answer":
			open = false
		}
	}
	return ask, open
}

// needsOf lists the open asks of a tenant, in item order.
func needsOf(t *Tenant) []needRow {
	rows := []needRow{}
	for _, it := range t.Items {
		if e, ok := openAsk(it); ok {
			rows = append(rows, needRow{t.Slug, it.ID, e.Note, e.By, e.At})
		}
	}
	return rows
}

// printNeeds writes the "needs you" block of the text board and review (rows indented by two).
func printNeeds(rows []needRow) {
	for _, r := range rows {
		by := ""
		if r.By != "" {
			by = " @" + displayName(r.By)
		}
		fmt.Printf("  %-8s %s%s  %s\n", r.Item, compactText(r.Question, 90), by, ago(r.At))
	}
}

func askCommand(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return errors.New("usage: mw ask ID QUESTION... [-t SLUG] [-s NAME]")
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
	q := strings.TrimSpace(strings.Join(pos[1:], " "))
	if q == "" {
		return errors.New("the question must not be blank")
	}
	who := value(o, "session", os.Getenv("MW_SESSION"))
	now := time.Now().UTC().Format(time.RFC3339)
	emit(it, now, "ask", who, q)
	touchSession(w, who, now)
	it.Updated = now
	if err := save(path, w); err != nil {
		return err
	}
	fmt.Printf("[%s] %s  asked (needs you)\n", t.Slug, it.ID)
	return nil
}

func answerCommand(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return errors.New("usage: mw answer ID TEXT... --by NAME [-t SLUG]")
	}
	if err := allowOptions(o, "tenant", "session", "by"); err != nil {
		return err
	}
	by, err := humanName("mw answer", o)
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
	text := strings.TrimSpace(strings.Join(pos[1:], " "))
	if text == "" {
		return errors.New("the answer must not be blank")
	}
	if _, ok := openAsk(*it); !ok {
		return fmt.Errorf("%s has no open question", it.ID)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	emit(it, now, "answer", by, text)
	it.Updated = now
	if err := save(path, w); err != nil {
		return err
	}
	fmt.Printf("[%s] %s  answered by %s\n", t.Slug, it.ID, by)
	return nil
}

// decisionsChore finds the tenant's "Decisions" chore or creates it (done, so it never holds the tenant open).
func decisionsChore(t *Tenant, now string) *Item {
	for i := range t.Items {
		if t.Items[i].Kind == "chore" && t.Items[i].Title == "Decisions" {
			return &t.Items[i]
		}
	}
	t.Next["CHR"]++
	it := Item{ID: fmt.Sprintf("CHR-%03d", t.Next["CHR"]), Kind: "chore", Title: "Decisions", Status: "done",
		Judged: "ungated", Body: "Decisions recorded with mw decide.", Created: now, Updated: now}
	emit(&it, now, "created", "", "")
	t.Items = append(t.Items, it)
	return &t.Items[len(t.Items)-1]
}

func decideCommand(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return errors.New("usage: mw decide TEXT... --by NAME [-b WHY] [-p ID] [-t SLUG]")
	}
	if err := allowOptions(o, "tenant", "session", "by", "body", "parent"); err != nil {
		return err
	}
	by := strings.TrimSpace(o["by"])
	if by == "" {
		if !stdinIsTerminal() {
			return errors.New("mw decide needs --by NAME when stdin is not a terminal")
		}
		by = value(map[string]string{"u": os.Getenv("USER")}, "u", "human")
	}
	text := strings.TrimSpace(strings.Join(pos, " "))
	if text == "" {
		return errors.New("the decision must not be blank")
	}
	if why := strings.TrimSpace(o["body"]); why != "" {
		text += " (why: " + why + ")"
	}
	t, err := selectTenant(w, o)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var it *Item
	if o["parent"] != "" {
		if it, err = itemByID(t, o["parent"]); err != nil {
			return err
		}
	} else {
		it = decisionsChore(t, now)
	}
	emit(it, now, "decide", by, text)
	it.Updated = now
	if err := save(path, w); err != nil {
		return err
	}
	fmt.Printf("[%s] %s  decided by %s\n", t.Slug, it.ID, by)
	return nil
}
