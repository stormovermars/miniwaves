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
