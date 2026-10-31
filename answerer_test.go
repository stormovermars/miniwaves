package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func answererBoard(t *testing.T) string {
	t.Helper()
	path := needsBoard(t)
	runAll(t,
		[]string{"session", "start", "parent"},
		[]string{"session", "start", "worker", "--answerer", "parent"},
	)
	return path
}

func sessionRec(t *testing.T, path, id string) Session {
	t.Helper()
	w, _ := load(path)
	s, err := sessionByID(w, id)
	if err != nil {
		t.Fatal(err)
	}
	return *s
}

func TestSessionStartAnswerer(t *testing.T) {
	path := answererBoard(t)
	if got := sessionRec(t, path, "worker").Answerer; got != "parent" {
		t.Fatalf("answerer = %q", got)
	}
	if sessionRec(t, path, "parent").Answerer != "" {
		t.Fatal("parent has no answerer")
	}
	for _, args := range [][]string{
		{"session", "start", "me", "--answerer", "me"},          // itself
		{"session", "start", "parent2", "--answerer", "Bad Id"}, // not a session name
	} {
		if err := run(args); err == nil {
			t.Fatalf("%v must be refused", args)
		}
	}
	// a cycle: parent -> worker -> parent
	runAll(t, []string{"session", "stop", "parent"})
	err := run([]string{"session", "start", "parent", "--answerer", "worker"})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle must be refused: %v", err)
	}
}

func lastEvents(t *testing.T, path, id string, n int) []Event {
	t.Helper()
	w, _ := load(path)
	it, _ := itemByID(&w.Tenants[0], id)
	return it.Events[len(it.Events)-n:]
}

func TestAnswererAnswers(t *testing.T) {
	path := answererBoard(t)
	runAll(t, []string{"ask", "TSK-001", "which format?", "-s", "worker"})
	if e := lastEvents(t, path, "TSK-001", 1)[0]; e.Action != "ask" || e.By != "worker" {
		t.Fatalf("ask -s records the session: %+v", e)
	}
	refusal := func(args ...string) string {
		err := run(args)
		if err == nil {
			t.Fatalf("%v must be refused", args)
		}
		return err.Error()
	}
	human := refusal("answer", "TSK-001", "csv", "-s", "parent", "--by", "Sir") // --by is for humans: refused
	for _, args := range [][]string{
		{"answer", "TSK-001", "csv", "-s", "worker"},   // never its own ask
		{"answer", "TSK-001", "csv", "-s", "stranger"}, // not the answerer
	} {
		if got := refusal(args...); got != refusal("answer", "TSK-001", "csv", "-s", "bot") || got != human {
			t.Fatalf("refusal differs for %v: %q vs %q", args, got, human)
		}
	}
	t.Setenv("MW_SESSION", "parent")
	runAll(t, []string{"answer", "TSK-001", "csv"})
	t.Setenv("MW_SESSION", "")
	if n := reviewNeeds(t); len(n) != 0 {
		t.Fatalf("closed: %+v", n)
	}
	if e := lastEvents(t, path, "TSK-001", 1)[0]; e.Action != "answer" || e.By != "parent" || e.Note != "ai: csv" {
		t.Fatalf("answer event: %+v", e)
	}

	// --accept: ai-marked decide, then the answer
	runAll(t, []string{"ask", "TSK-001", "ok to use csv? rec: yes", "-s", "worker"}, []string{"answer", "TSK-001", "--accept", "-s", "parent"})
	ev := lastEvents(t, path, "TSK-001", 2)
	if ev[0].Action != "decide" || ev[0].By != "parent" || ev[0].Note != "ai: accepted: ok to use csv? rec: yes" || ev[1].Action != "answer" || ev[1].Note != "ai: accepted" || ev[1].By != "parent" {
		t.Fatalf("accept events: %+v", ev)
	}

	// a human ask (no session) and a session without an answerer are not answerable by a session
	runAll(t, []string{"ask", "TSK-002", "human asks"})
	if got := refusal("answer", "TSK-002", "x", "-s", "parent"); got != human {
		t.Fatalf("human ask: %q", got)
	}
	runAll(t, []string{"ask", "TSK-002", "parent asks", "-s", "parent"})
	if got := refusal("answer", "TSK-002", "x", "-s", "parent"); got != human {
		t.Fatalf("not its own ask: %q", got)
	}
	if got := refusal("answer", "TSK-002", "x", "-s", "worker"); got != human { // not transitive, not the answerer
		t.Fatalf("worker: %q", got)
	}

	// a stopped answerer cannot answer
	runAll(t, []string{"ask", "TSK-001", "again", "-s", "worker"}, []string{"session", "stop", "parent"})
	if got := refusal("answer", "TSK-001", "x", "-s", "parent"); got != human {
		t.Fatalf("stopped answerer: %q", got)
	}
}

func TestAnswererNeedsRouting(t *testing.T) {
	path := answererBoard(t)
	runAll(t, []string{"ask", "TSK-001", "which format?", "-s", "worker"}, []string{"ask", "TSK-002", "human asks"})
	// only the human ask counts for the human; the worker's ask is listed for its answerer
	if n := reviewNeeds(t); len(n) != 1 || n[0].Item != "TSK-002" {
		t.Fatalf("human needs: %+v", n)
	}
	var d struct {
		For []needRow `json:"for_answerers"`
	}
	if err := json.Unmarshal([]byte(captureOutput(t, func() { runAll(t, []string{"review", "-j"}) })), &d); err != nil || len(d.For) != 1 || d.For[0].Item != "TSK-001" || d.For[0].For != "parent" {
		t.Fatalf("for_answerers: %+v %v", d.For, err)
	}
	for _, cmd := range []string{"review", "board"} {
		out := captureOutput(t, func() { runAll(t, []string{cmd}) })
		if !strings.Contains(out, "for parent") || strings.Count(out, "which format?") != 1 {
			t.Fatalf("%s text:\n%s", cmd, out)
		}
	}
	w, _ := load(path)
	if c := statusCounts(w); c.Needs != 1 {
		t.Fatalf("status N must count only the human ask: %+v", c)
	}
	md := captureOutput(t, func() { runAll(t, []string{"board", "--md", "-o", "-"}) })
	needsPart, _, _ := strings.Cut(md, "### For answerers")
	if !strings.Contains(md, "### For answerers") || !strings.Contains(md, "for parent: which format?") || strings.Contains(needsPart, "which format?") {
		t.Fatalf("jobs md:\n%s", md)
	}

	// (a) a stopped answerer: the ask falls back to the human list
	runAll(t, []string{"session", "stop", "parent"})
	if n := reviewNeeds(t); len(n) != 2 {
		t.Fatalf("stopped answerer must not orphan the ask: %+v", n)
	}
	w, _ = load(path)
	if c := statusCounts(w); c.Needs != 2 {
		t.Fatalf("status N after the answerer stopped: %+v", c)
	}
}

// (b) the answerer escalates with its own ask on the same item: it counts for the human.
func TestAnswererEscalates(t *testing.T) {
	answererBoard(t)
	runAll(t, []string{"ask", "TSK-001", "push the release?", "-s", "worker"})
	if n := reviewNeeds(t); len(n) != 0 {
		t.Fatalf("before escalation: %+v", n)
	}
	runAll(t, []string{"ask", "TSK-001", "escalated: push the release?", "-s", "parent"})
	n := reviewNeeds(t)
	if len(n) != 1 || n[0].By != "parent" || n[0].Question != "escalated: push the release?" {
		t.Fatalf("the escalation is the open ask and counts for the human: %+v", n)
	}
	// the answerer cannot answer its own escalation, nor can the worker
	for _, s := range []string{"parent", "worker"} {
		if err := run([]string{"answer", "TSK-001", "yes", "-s", s}); err == nil {
			t.Fatalf("%s must not answer the escalation", s)
		}
	}
	runAll(t, []string{"answer", "TSK-001", "no", "--by", "Sir"})
	if n := reviewNeeds(t); len(n) != 0 {
		t.Fatalf("the human closed it: %+v", n)
	}
}

// (e) the answerer answers asks only: accept, reject, edit -c and gates stay human.
func TestAnswererScope(t *testing.T) {
	answererBoard(t)
	runAll(t, []string{"edit", "TSK-001", "-c", "set by a human"}) // a session may only draft an empty acceptance
	for _, args := range [][]string{
		{"accept", "TSK-001", "-s", "parent"},
		{"reject", "TSK-001", "why", "-s", "parent"},
		{"edit", "TSK-001", "-c", "some acceptance", "-s", "parent"},
		{"gate", "level", "proof", "-s", "parent"},
	} {
		if err := run(args); err == nil {
			t.Fatalf("%v must stay human-only", args)
		}
	}
}
