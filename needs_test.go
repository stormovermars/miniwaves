package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func needsBoard(t *testing.T) string {
	t.Helper()
	path := inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t,
		[]string{"init", "Needs app"},
		[]string{"gate", "level", "trust", "--workspace", "--by", "t"},
		[]string{"add", "task", "Build index", "-S", "ready"},
		[]string{"add", "task", "Second", "-S", "ready"},
	)
	return path
}

func reviewNeeds(t *testing.T, extra ...string) []needRow {
	t.Helper()
	out := captureOutput(t, func() { runAll(t, append([]string{"review", "-j"}, extra...)) })
	var d struct {
		Needs []needRow `json:"needs"`
	}
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if d.Needs == nil {
		t.Fatalf("needs must be an array: %s", out)
	}
	return d.Needs
}

func TestAskAnswerFlow(t *testing.T) {
	path := needsBoard(t)
	if n := reviewNeeds(t); len(n) != 0 {
		t.Fatalf("fresh board: %+v", n)
	}
	runAll(t, []string{"ask", "TSK-001", "Which", "format?", "-s", "bot"})
	n := reviewNeeds(t)
	if len(n) != 1 || n[0].Item != "TSK-001" || n[0].Question != "Which format?" || n[0].By != "bot" || n[0].Tenant != "needs-app" || n[0].At == "" {
		t.Fatalf("needs: %+v", n)
	}
	// the status is untouched and the event is on the item
	w, _ := load(path)
	it, _ := itemByID(&w.Tenants[0], "TSK-001")
	if it.Status != "ready" || it.Events[len(it.Events)-1].Action != "ask" {
		t.Fatalf("item: %+v", it)
	}
	text := captureOutput(t, func() { runAll(t, []string{"review"}) })
	if !strings.Contains(text, "needs you (1)") || !strings.Contains(text, "Which format?") {
		t.Fatalf("review text:\n%s", text)
	}
	board := captureOutput(t, func() { runAll(t, []string{"board"}) })
	if !strings.Contains(board, "NEEDS YOU (1)") || !strings.Contains(board, "Which format?") {
		t.Fatalf("board text:\n%s", board)
	}

	// answer is human only
	if err := run([]string{"answer", "TSK-001", "csv", "--by", "Sir", "-s", "bot"}); err == nil || !strings.Contains(err.Error(), "humans") && !strings.Contains(err.Error(), "for humans") {
		t.Fatalf("answer under -s must be refused: %v", err)
	}
	t.Setenv("MW_SESSION", "bot")
	if err := run([]string{"answer", "TSK-001", "csv", "--by", "Sir"}); err == nil {
		t.Fatal("answer under MW_SESSION must be refused")
	}
	t.Setenv("MW_SESSION", "")
	if err := run([]string{"answer", "TSK-001", "csv"}); err == nil { // stdin is not a terminal in tests
		t.Fatal("answer without --by must be refused off a terminal")
	}
	if len(reviewNeeds(t)) != 1 {
		t.Fatal("refused answers must not close the ask")
	}
	if err := run([]string{"answer", "TSK-002", "x", "--by", "Sir"}); err == nil {
		t.Fatal("answer without an open ask must be refused")
	}
	runAll(t, []string{"answer", "TSK-001", "use", "csv", "--by", "Sir"})
	if n := reviewNeeds(t); len(n) != 0 {
		t.Fatalf("answered: %+v", n)
	}
	w, _ = load(path)
	it, _ = itemByID(&w.Tenants[0], "TSK-001")
	last := it.Events[len(it.Events)-1]
	if last.Action != "answer" || last.By != "Sir" || last.Note != "use csv" {
		t.Fatalf("answer event: %+v", last)
	}
	if strings.Contains(captureOutput(t, func() { runAll(t, []string{"board"}) }), "NEEDS YOU") {
		t.Fatal("board still shows needs")
	}

	// an open ask is the last ask with no later answer: two asks, one answer closes both; a new ask reopens
	runAll(t, []string{"ask", "TSK-001", "one"}, []string{"ask", "TSK-001", "two"})
	if n := reviewNeeds(t); len(n) != 1 || n[0].Question != "two" || n[0].By != "" {
		t.Fatalf("last ask wins: %+v", n)
	}
	runAll(t, []string{"answer", "TSK-001", "both", "--by", "Sir"}, []string{"ask", "TSK-002", "other"})
	if n := reviewNeeds(t); len(n) != 1 || n[0].Item != "TSK-002" {
		t.Fatalf("per item: %+v", n)
	}
	// ask works on any board: no gate, any item kind, blank refused
	if err := run([]string{"ask", "TSK-001", "  "}); err == nil {
		t.Fatal("blank question")
	}
	if err := run([]string{"ask", "TSK-999", "x"}); err == nil {
		t.Fatal("unknown item")
	}
}

func TestDecide(t *testing.T) {
	path := needsBoard(t)
	if err := run([]string{"decide", "Use", "SQLite"}); err == nil {
		t.Fatal("decide needs --by off a terminal")
	}
	runAll(t, []string{"decide", "Use", "SQLite", "--by", "Sir", "-b", "no server"})
	w, _ := load(path)
	tn := &w.Tenants[0]
	if len(tn.Items) != 3 {
		t.Fatalf("items: %+v", tn.Items)
	}
	ch, _ := itemByID(tn, "CHR-001")
	if ch.Title != "Decisions" || ch.Kind != "chore" || ch.Status != "done" {
		t.Fatalf("chore: %+v", ch)
	}
	e := ch.Events[len(ch.Events)-1]
	if e.Action != "decide" || e.By != "Sir" || e.Note != "Use SQLite (why: no server)" {
		t.Fatalf("event: %+v", e)
	}
	// the chore is reused, not duplicated; -p targets any item
	runAll(t, []string{"decide", "Ship", "Friday", "--by", "Sir"}, []string{"decide", "Skip", "x", "--by", "Sir", "-p", "TSK-002"})
	w, _ = load(path)
	if len(w.Tenants[0].Items) != 3 || w.Tenants[0].Next["CHR"] != 1 {
		t.Fatalf("chore duplicated: %+v", w.Tenants[0].Items)
	}
	ch, _ = itemByID(&w.Tenants[0], "CHR-001")
	if n := countAction(*ch, "decide"); n != 2 {
		t.Fatalf("decide events on the chore: %d", n)
	}
	it, _ := itemByID(&w.Tenants[0], "TSK-002")
	if countAction(*it, "decide") != 1 || it.Status != "ready" {
		t.Fatalf("-p item: %+v", it)
	}
	if err := run([]string{"decide", "x", "--by", "Sir", "-p", "TSK-999"}); err == nil {
		t.Fatal("unknown -p item")
	}
	if err := run([]string{"decide", "--by", "Sir"}); err == nil {
		t.Fatal("blank decision")
	}
	// the done chore holds nothing open: the tenant can still finish
	_ = path
}

func countAction(it Item, action string) int {
	n := 0
	for _, e := range it.Events {
		if e.Action == action {
			n++
		}
	}
	return n
}

func TestNeedsCommandsAreWrites(t *testing.T) {
	for _, c := range []string{"ask", "answer", "decide"} {
		if readOnly([]string{c, "x"}) {
			t.Errorf("%s writes", c)
		}
	}
}

// New events and fields survive a load/save unchanged, and an mw that predates them reads the
// events as plain strings (Event keeps the same four fields).
func TestEventsRoundTripAndOldReader(t *testing.T) {
	path := needsBoard(t)
	runAll(t,
		[]string{"ask", "TSK-001", "which?", "-s", "bot"},
		[]string{"answer", "TSK-001", "this", "--by", "Sir"},
		[]string{"decide", "Go", "--by", "Sir", "-b", "why"},
		[]string{"jobs", "enable", "tiny", "-t", "needs-app"},
	)
	tp := tenantPath(path, "needs-app")
	before, _ := os.ReadFile(tp)
	ib, _ := os.ReadFile(path)
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	w.indexBase, w.base = "", map[string]string{}
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(tp)
	ia, _ := os.ReadFile(path)
	if string(before) != string(after) || string(ib) != string(ia) {
		t.Fatalf("round trip changed files:\n%s\n%s", before, after)
	}
	var old struct {
		Items []struct {
			ID     string `json:"id"`
			Events []struct {
				At     string `json:"at"`
				Action string `json:"action"`
				By     string `json:"by"`
				Note   string `json:"note"`
			} `json:"events"`
		} `json:"items"`
	}
	if err := json.Unmarshal(after, &old); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, it := range old.Items {
		for _, e := range it.Events {
			seen[e.Action] = true
		}
	}
	if !seen["ask"] || !seen["answer"] || !seen["decide"] {
		t.Fatalf("actions: %v", seen)
	}
	// and the old tenant file is unchanged by an unrelated command (no data lost)
	runAll(t, []string{"note", "TSK-002", "hello"})
	again, _ := load(path)
	if len(again.Jobs) != 1 || again.Tenants[0].Job != "tiny" {
		t.Fatalf("jobs lost: %+v", again.Jobs)
	}
	it, _ := itemByID(&again.Tenants[0], "TSK-001")
	if countAction(*it, "ask") != 1 || countAction(*it, "answer") != 1 {
		t.Fatalf("events lost: %+v", it.Events)
	}
}

func TestGoldenReviewNeeds(t *testing.T) {
	needsBoard(t)
	runAll(t, []string{"ask", "TSK-001", "Which format?", "-s", "bot"})
	got := captureOutput(t, func() { runAll(t, []string{"review", "-j"}) })
	got = rfcRe.ReplaceAllString(got, "<TS>")
	checkGolden(t, "review-needs-j", got)
}

// BUG-018: a final transition (done, accept) closes an earlier open ask; an ask after it still counts.
func TestFinalTransitionClosesAsk(t *testing.T) {
	needsBoard(t)
	runAll(t,
		[]string{"start", "TSK-001"},
		[]string{"ask", "TSK-001", "Merge?"},
	)
	if n := reviewNeeds(t); len(n) != 1 {
		t.Fatalf("ask before done: %+v", n)
	}
	out := captureOutput(t, func() { runAll(t, []string{"done", "TSK-001", "merged"}) })
	if !strings.Contains(out, "closed open ask") || !strings.Contains(out, "Merge?") {
		t.Fatalf("done must say it closed the ask:\n%s", out)
	}
	if n := reviewNeeds(t); len(n) != 0 {
		t.Fatalf("ask then done must not need you: %+v", n)
	}
	board := captureOutput(t, func() { runAll(t, []string{"board"}) })
	if strings.Contains(board, "NEEDS YOU") {
		t.Fatalf("board still shows needs:\n%s", board)
	}
	// ask after done counts
	runAll(t, []string{"ask", "TSK-001", "Again?"})
	if n := reviewNeeds(t); len(n) != 1 || n[0].Question != "Again?" {
		t.Fatalf("ask after done: %+v", n)
	}
	runAll(t, []string{"answer", "TSK-001", "ok", "--by", "Sir"})
	// ask, done, reopen: the old ask stays closed
	runAll(t, []string{"reopen", "TSK-001"}, []string{"start", "TSK-001"}, []string{"ask", "TSK-001", "Third?"}, []string{"done", "TSK-001", "x"}, []string{"reopen", "TSK-001"})
	if n := reviewNeeds(t); len(n) != 0 {
		t.Fatalf("ask, done, reopen: %+v", n)
	}
	// plain answer still works
	runAll(t, []string{"ask", "TSK-002", "Q"})
	if n := reviewNeeds(t); len(n) != 1 {
		t.Fatalf("ask: %+v", n)
	}
	runAll(t, []string{"answer", "TSK-002", "A", "--by", "Sir"})
	if n := reviewNeeds(t); len(n) != 0 {
		t.Fatalf("answer: %+v", n)
	}
}

func TestOpenAskUnit(t *testing.T) {
	ev := func(as ...string) Item {
		var it Item
		for _, a := range as {
			it.Events = append(it.Events, Event{Action: a})
		}
		return it
	}
	for _, c := range []struct {
		ev   []string
		open bool
	}{
		{[]string{"start", "ask", "done"}, false},
		{[]string{"ask", "done", "ask"}, true},
		{[]string{"ask", "done", "reopen"}, false},
		{[]string{"ask", "done", "accept"}, false},
		{[]string{"ask", "answer"}, false},
		{[]string{"ask"}, true},
	} {
		if _, ok := openAsk(ev(c.ev...)); ok != c.open {
			t.Errorf("%v: open=%v want %v", c.ev, ok, c.open)
		}
	}
}
