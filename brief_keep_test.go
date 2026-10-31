package main

import "testing"

// TSK-080: the brief keeps every reject/answer/decide event; other events stay capped at the last 5.
func TestBriefKeepsDecisionEvents(t *testing.T) {
	var evs []Event
	add := func(a string) { evs = append(evs, Event{At: "2026-01-01T00:00:00Z", Action: a, Note: a}) }
	add("reject")
	for i := 0; i < 4; i++ {
		add("note")
	}
	add("answer")
	add("decide")
	for i := 0; i < 8; i++ {
		add("note")
	}
	add("reject")
	got := briefEvents(evs)
	counts := map[string]int{}
	for _, e := range got {
		counts[e.Action]++
	}
	if counts["reject"] != 2 || counts["answer"] != 1 || counts["decide"] != 1 {
		t.Fatalf("decision events dropped: %v", counts)
	}
	if counts["note"] != 4 {
		t.Fatalf("other events must stay capped at the last 5 (4 notes + the final reject): %v", counts)
	}
	// chronological order is kept: first is the oldest reject, last is the newest reject
	if got[0].Action != "reject" || got[len(got)-1].Action != "reject" {
		t.Fatalf("order: %+v", got)
	}
	if len(briefEvents(evs[:3])) != 3 || briefEvents(nil) == nil {
		t.Fatal("short or empty lists")
	}
}
