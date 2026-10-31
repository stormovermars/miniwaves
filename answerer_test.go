package main

import (
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
