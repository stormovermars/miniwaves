package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestNewsBacklogRecentSprint(t *testing.T) {
	newsBoard(t)
	runAll(t, []string{"add", "task", "Draft one", "-t", "news-app"}, []string{"add", "task", "Draft two", "-t", "doc"})
	m := decodeModel(t, newsOut(t, "--job", "tiny", "--format", "json"))
	c := m.Now.Counts
	if c.Draft != 2 || c.Ready != 1 {
		t.Fatalf("counts: %+v", c)
	}
	// the ready queue first, then the drafts by creation
	b := m.Now.Backlog
	if b.Count != 3 || len(b.Items) != 3 || b.Items[0].ID != "TSK-002" || b.Items[0].State != "ready" ||
		b.Items[1].Title != "Draft one" || b.Items[1].State != "draft" || b.Items[2].Title != "Draft two" {
		t.Fatalf("backlog: %+v", b)
	}
	if m.Now.More["backlog"] != 0 || m.Now.Caps["backlog"] != 50 || m.Now.Caps["recent_done"] != 10 || m.Now.Caps["sprint"] != 50 {
		t.Fatalf("caps: %v %v", m.Now.Caps, m.Now.More)
	}
	// recent done: newest first, with the done time and the milestone
	if len(m.Now.RecentDone) != 2 || m.Now.RecentDone[0].Since < m.Now.RecentDone[1].Since {
		t.Fatalf("recent_done: %+v", m.Now.RecentDone)
	}
	var wave *newsItem
	for i, it := range m.Now.RecentDone {
		if it.ID == "TSK-001" && it.Tenant == "news-app" {
			wave = &m.Now.RecentDone[i]
		}
	}
	if wave == nil || wave.Milestone != "WAV-001" || wave.MilestoneTitle != "First | milestone" || wave.Since == "" {
		t.Fatalf("recent_done milestone: %+v", m.Now.RecentDone)
	}
	// the done item of the window carries its milestone for the sprint review
	if d := m.Happened.Done; len(d) != 1 || d[0].Milestone != "WAV-001" || d[0].MilestoneTitle != "First | milestone" {
		t.Fatalf("happened.done: %+v", d)
	}
	// the sprint is the open wave, members in flow order
	sp := m.Now.Sprint
	if sp == nil || sp.Kind != "wave" || sp.ID != "WAV-001" || sp.Done != 1 || sp.Total != 2 || len(sp.Items) != 2 || sp.More != 0 {
		t.Fatalf("sprint: %+v", sp)
	}
	if sp.Items[0].ID != "TSK-002" || sp.Items[0].State != "todo" || sp.Items[1].ID != "TSK-001" || sp.Items[1].State != "done" {
		t.Fatalf("sprint items: %+v", sp.Items)
	}
	// an ask on a member flags it
	runAll(t, []string{"ask", "TSK-002", "which one?", "-s", "x", "-t", "news-app"}, []string{"start", "TSK-002", "-s", "bob", "-t", "news-app"})
	m = decodeModel(t, newsOut(t, "--job", "tiny", "--format", "json"))
	if it := m.Now.Sprint.Items[0]; it.ID != "TSK-002" || it.State != "doing" || !it.Needs {
		t.Fatalf("flagged member: %+v", it)
	}
	// no wave: null
	runAll(t, []string{"wave", "create", "Second", "-t", "doc"})
	w, _ := load(".miniwaves.json")
	_ = w
}

func TestNewsSprintNullWithoutWave(t *testing.T) {
	inTempDir(t)
	runAll(t, []string{"init", "N"}, []string{"jobs", "enable", "j", "-t", "n"}, []string{"add", "task", "A"})
	m := decodeModel(t, newsOut(t, "--job", "j", "--format", "json"))
	if m.Now.Sprint != nil || m.Now.Counts.Draft != 1 || m.Now.Backlog.Count != 1 {
		t.Fatalf("%+v %+v", m.Now.Sprint, m.Now.Backlog)
	}
	if !strings.Contains(newsOut(t, "--job", "j", "--format", "json"), `"sprint": null`) {
		t.Fatal("sprint is not null in the json")
	}
}

// caps: backlog 50, recent_done 10, sprint 50, moved lists 100, each with its more count
func TestNewsStyleCaps(t *testing.T) {
	path := newsStress(t)
	w, _ := load(path)
	tn := &w.Tenants[0]
	base := time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)
	n := 0
	stamp := func() string { n++; return base.Add(time.Duration(n) * time.Second).Format(time.RFC3339) }
	// an open wave with 60 members of mixed state
	tn.Items = append(tn.Items, Item{ID: "WAV-5000", Kind: "wave", Title: "Big sprint", Status: "ready", Created: stamp(), Updated: stamp()})
	for i := 0; i < 60; i++ {
		st := []string{"ready", "doing", "done"}[i%3]
		tn.Items = append(tn.Items, Item{ID: fmt.Sprintf("SPR-%03d", i), Kind: "task", Title: "member", Status: st, Wave: "WAV-5000", Judged: "ungated", Created: stamp(), Updated: stamp()})
	}
	// 150 extra items that started
	for i := 0; i < 150; i++ {
		tn.Items = append(tn.Items, Item{ID: fmt.Sprintf("STA-%03d", i), Kind: "task", Title: "started", Status: "doing", Created: stamp(), Updated: stamp(),
			Events: []Event{{At: stamp(), Action: "start", By: "s"}}})
	}
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	m := decodeModel(t, bigJSON(t))
	if sp := m.Now.Sprint; sp == nil || len(sp.Items) != 50 || sp.More != 10 || sp.Total != 60 || sp.Items[0].State != "doing" || sp.Items[len(sp.Items)-1].State != "done" {
		t.Fatalf("sprint: %+v", sp)
	}
	if len(m.Now.Backlog.Items) != 50 || m.Now.More["backlog"] != m.Now.Backlog.Count-50 || m.Now.Backlog.Count < 100 {
		t.Fatalf("backlog: %d of %d", len(m.Now.Backlog.Items), m.Now.Backlog.Count)
	}
	if len(m.Now.RecentDone) != 10 {
		t.Fatalf("recent_done %d", len(m.Now.RecentDone))
	}
	h := m.Happened
	if len(h.MovedToDoing) != 100 || h.More["moved_to_doing"] != h.Counts["moved_to_doing"]-100 || h.Counts["moved_to_doing"] < 250 || h.Caps["moved_to_doing"] != 100 {
		t.Fatalf("moved_to_doing: %d counts %v more %v", len(h.MovedToDoing), h.Counts, h.More)
	}
	// the judge gate is on here: the awaiting-review blocks and the unjudged done items moved to review
	if len(h.MovedToReview) != 100 || h.Counts["moved_to_review"] < 100 || h.Caps["moved_to_review"] != 100 {
		t.Fatalf("moved_to_review: %d counts %v", len(h.MovedToReview), h.Counts)
	}
	for _, it := range h.MovedToReview {
		if it.Since == "" || it.ID == "" {
			t.Fatalf("moved item: %+v", it)
		}
	}
}

// flow events never make a release on their own
func TestNewsFlowEventsAreNotChanges(t *testing.T) {
	newsBoardAt(t, pastBase)
	newsOut(t, "--job", "tiny", "--release")
	runAll(t, []string{"start", "TSK-002", "-s", "x", "-t", "news-app"})
	d := decodeModel(t, newsOut(t, "--job", "tiny", "--format", "json"))
	if len(d.Happened.MovedToDoing) != 1 || d.Release.Changes != 0 {
		t.Fatalf("moved %d changes %d", len(d.Happened.MovedToDoing), d.Release.Changes)
	}
	if out := newsOut(t, "--job", "tiny", "--release"); strings.TrimSpace(out) != "nothing new since release 1" {
		t.Fatalf("flow-only release: %q", out)
	}
}
