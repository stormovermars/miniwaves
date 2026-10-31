package main

import (
	"strings"
	"testing"
)

func TestAnswerAccept(t *testing.T) {
	path := needsBoard(t)
	if err := run([]string{"answer", "TSK-001", "--accept", "--by", "Sir"}); err == nil || !strings.Contains(err.Error(), "no open question") {
		t.Fatalf("accept without an open ask must be refused: %v", err)
	}
	runAll(t, []string{"ask", "TSK-001", "Grill R1: 1) csv? rec: yes", "-s", "bot"})
	if err := run([]string{"answer", "TSK-001", "--accept", "-s", "bot", "--by", "Sir"}); err == nil || !strings.Contains(err.Error(), "humans") {
		t.Fatalf("accept under -s must be refused: %v", err)
	}
	t.Setenv("MW_SESSION", "bot")
	if err := run([]string{"answer", "TSK-001", "--accept", "--by", "Sir"}); err == nil {
		t.Fatal("accept under MW_SESSION must be refused")
	}
	t.Setenv("MW_SESSION", "")
	if err := run([]string{"answer", "TSK-001", "--accept"}); err == nil { // not a terminal: --by needed
		t.Fatal("accept without --by must be refused off a terminal")
	}
	if err := run([]string{"answer", "TSK-001", "also this", "--accept", "--by", "Sir"}); err == nil {
		t.Fatal("accept takes no text")
	}
	if len(reviewNeeds(t)) != 1 {
		t.Fatal("refused accepts must not close the ask")
	}
	runAll(t, []string{"answer", "TSK-001", "--accept", "--by", "Sir"})
	if n := reviewNeeds(t); len(n) != 0 {
		t.Fatalf("accepted: %+v", n)
	}
	w, _ := load(path)
	it, _ := itemByID(&w.Tenants[0], "TSK-001")
	n := len(it.Events)
	d, a := it.Events[n-2], it.Events[n-1]
	if d.Action != "decide" || d.By != "Sir" || d.Note != "accepted: Grill R1: 1) csv? rec: yes" || a.Action != "answer" || a.Note != "accepted" || a.By != "Sir" {
		t.Fatalf("events: %+v %+v", d, a)
	}
	if err := run([]string{"answer", "TSK-001", "--accept", "--by", "Sir"}); err == nil {
		t.Fatal("a closed round cannot be accepted twice")
	}
}
