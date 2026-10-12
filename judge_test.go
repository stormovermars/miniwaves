package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// judgeBoard: story STR-001 (acceptance) with tasks TSK-001, TSK-002, judge gate on.
func judgeBoard(t *testing.T) (string, string) {
	path, home := gateEnv(t)
	runAll(t,
		[]string{"add", "story", "Outcome", "-c", "opens fast; no crash", "-S", "ready"},
		[]string{"add", "task", "A", "-p", "STR-001", "-S", "ready"},
		[]string{"add", "task", "B", "-p", "STR-001", "-S", "ready"},
		[]string{"add", "bug", "Loose", "-S", "ready"},
	)
	gateSet(t, "judge", "on")
	gateSet(t, "evidence", "off")
	gateSet(t, "acceptance", "off")
	return path, home
}

func doneAll(t *testing.T, ids ...string) {
	for _, id := range ids {
		runAll(t, []string{"done", id, "checked " + id})
	}
}

func review(t *testing.T) reviewDoc {
	out := captureOutput(t, func() { runAll(t, []string{"review", "-j"}) })
	var d reviewDoc
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	return d
}

func TestReviewAcceptRejectStory(t *testing.T) {
	path, _ := judgeBoard(t)
	if d := review(t); len(d.Judge) != 0 {
		t.Fatalf("%+v", d)
	}
	doneAll(t, "TSK-001")
	if d := review(t); len(d.Judge) != 0 {
		t.Fatalf("story listed with an open child: %+v", d)
	}
	doneAll(t, "TSK-002", "BUG-001")
	d := review(t)
	if len(d.Judge) != 2 || d.Judge[0].ID != "STR-001" || d.Judge[0].State != "ready" || d.Judge[1].ID != "BUG-001" {
		t.Fatalf("%+v", d)
	}
	out := captureOutput(t, func() { runAll(t, []string{"judge", "STR-001"}) })
	if strings.Count(out, "\n") != 8 || !strings.Contains(out, "Outcome: STR-001") || !strings.Contains(out, "Evidence:\n  ") ||
		strings.Count(out, "  TSK-00") != 2 || !strings.Contains(out, "Decision:\n  [ ] opens fast\n  [ ] no crash\n") {
		t.Fatalf("%q", out)
	}
	wantErr(t, "not judgeable", "judge", "TSK-001")
	wantErr(t, "not judgeable", "accept", "TSK-001", "--by", "me")
	// reject the story: ready + Fix child inheriting acceptance
	runAll(t, []string{"reject", "STR-001", "startup", "is", "slow", "--by", "me"})
	st := itemOf(t, path, "STR-001")
	fix := itemOf(t, path, "TSK-003")
	if st.Status != "ready" || st.Attempts != 1 || st.Judged != "" || fix.Title != "Fix: startup is slow" || fix.Status != "ready" || fix.Parent != "STR-001" || fix.Kind != "task" {
		t.Fatalf("%+v\n%+v", st, fix)
	}
	if effectiveAcceptance(&loadT(t, path).Tenants[0], fix) != "opens fast; no crash" {
		t.Fatal("fix child does not inherit acceptance")
	}
	if d := review(t); len(d.Judge) != 1 || d.Judge[0].ID != "BUG-001" {
		t.Fatalf("%+v", d)
	}
	doneAll(t, "TSK-003")
	d = review(t)
	if len(d.Judge) != 2 || d.Judge[0].Attempts != 1 {
		t.Fatalf("%+v", d)
	}
	if txt := captureOutput(t, func() { runAll(t, []string{"review"}) }); !strings.Contains(txt, "×1") {
		t.Fatal(txt)
	}
	runAll(t, []string{"accept", "STR-001", "--by", "me"})
	st = itemOf(t, path, "STR-001")
	if st.Status != "done" || st.Judged == "" || st.JudgedBy != "me" || loadT(t, path).Tenants[0].AcceptStreak != 1 {
		t.Fatalf("%+v", st)
	}
	wantErr(t, "already accepted", "accept", "STR-001", "--by", "me")
}

func loadT(t *testing.T, path string) *Workspace {
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestAcceptStoryWithDoneChildrenSetsDoneAndJudged(t *testing.T) {
	path, _ := judgeBoard(t)
	doneAll(t, "TSK-001", "TSK-002")
	if itemOf(t, path, "STR-001").Status == "done" {
		t.Fatal("story closed itself")
	}
	runAll(t, []string{"accept", "STR-001", "--by", "me"})
	st := itemOf(t, path, "STR-001")
	if st.Status != "done" || st.Judged == "" {
		t.Fatalf("%+v", st)
	}
	wantErr(t, "not done", "accept", "BUG-001", "--by", "me")
}

func TestRejectChildReopensDoneStory(t *testing.T) {
	path, _ := judgeBoard(t)
	doneAll(t, "TSK-001", "TSK-002")
	runAll(t, []string{"accept", "STR-001", "--by", "me"})
	runAll(t, []string{"reject", "TSK-002", "wrong output", "--by", "me"})
	c, st := itemOf(t, path, "TSK-002"), itemOf(t, path, "STR-001")
	if c.Status != "ready" || c.Attempts != 1 || st.Status != "ready" || st.Judged != "" || st.JudgedBy != "" {
		t.Fatalf("%+v\n%+v", c, st)
	}
	if loadT(t, path).Tenants[0].AcceptStreak != 0 {
		t.Fatal("streak not reset")
	}
	wantErr(t, "not done", "reject", "TSK-002", "again", "--by", "me")
	wantErr(t, "usage", "reject", "TSK-001")
}

func TestReopenClearsJudged(t *testing.T) {
	path, _ := judgeBoard(t)
	doneAll(t, "BUG-001")
	runAll(t, []string{"accept", "BUG-001", "--by", "me"})
	if itemOf(t, path, "BUG-001").Judged == "" {
		t.Fatal("not judged")
	}
	runAll(t, []string{"reopen", "BUG-001"})
	if it := itemOf(t, path, "BUG-001"); it.Judged != "" || it.JudgedBy != "" {
		t.Fatalf("%+v", it)
	}
}

func TestAcceptRejectHumansOnly(t *testing.T) {
	judgeBoard(t)
	doneAll(t, "BUG-001")
	runAll(t, []string{"session", "start", "bot"})
	wantErr(t, "for humans", "accept", "BUG-001", "--by", "x", "-s", "bot")
	wantErr(t, "for humans", "reject", "BUG-001", "no", "--by", "x", "-s", "bot")
	t.Setenv("MW_SESSION", "bot")
	wantErr(t, "for humans", "accept", "BUG-001", "--by", "x")
	t.Setenv("MW_SESSION", "")
	wantErr(t, "--by", "accept", "BUG-001") // no terminal
	wantErr(t, "--by", "reject", "BUG-001", "no")
	stdinIsTerminal = func() bool { return true }
	runAll(t, []string{"accept", "BUG-001"})
}

func TestRejectRefusedInClosedWaveAndEvidenceGap(t *testing.T) {
	path, _ := judgeBoard(t)
	runAll(t, []string{"wave", "create", "W"}, []string{"wave", "add", "WAV-001", "BUG-001"})
	doneAll(t, "BUG-001")
	runAll(t, []string{"accept", "BUG-001", "--by", "me"})
	w := loadT(t, path)
	wv, _ := itemByID(&w.Tenants[0], "WAV-001")
	for i := range wv.Steps {
		wv.Steps[i].Done = true
	}
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	runAll(t, []string{"wave", "close", "WAV-001"})
	wantErr(t, "closed", "reject", "BUG-001", "nope", "--by", "me")
	runAll(t, []string{"reopen", "WAV-001"})
	runAll(t, []string{"reject", "BUG-001", "no proof", "--evidence-gap", "--by", "me"})
	if got := loadT(t, path).Tenants[0].GateLatch["evidence"]; got != "reject BUG-001" {
		t.Fatalf("latch %q", got)
	}
}

func TestReviewJudgeOffAndAcceptanceOff(t *testing.T) {
	path, _ := gateEnv(t)
	runAll(t, []string{"add", "story", "S", "-S", "ready"}, []string{"add", "bug", "B", "-S", "blocked"})
	gateSet(t, "judge", "off")
	gateSet(t, "acceptance", "off")
	doneAll(t)
	runAll(t, []string{"done", "STR-001", "ok"}) // ungated: not in judge list
	out := captureOutput(t, func() { runAll(t, []string{"review"}) })
	if !strings.Contains(out, "judge: []") || strings.Contains(out, "acceptance") || !strings.Contains(out, "blocked (1)") {
		t.Fatal(out)
	}
	if d := review(t); d.Judge == nil || d.Acceptance == nil || d.Waves == nil || len(d.Judge) != 0 {
		t.Fatalf("%+v", d)
	}
	gateSet(t, "acceptance", "on")
	out = captureOutput(t, func() { runAll(t, []string{"review"}) })
	if !strings.Contains(out, "acceptance (1)") || !strings.Contains(out, "BUG-001") {
		t.Fatal(out)
	}
	_ = path
}

func TestJudgeGateAfterUngatedDoneIsJudged(t *testing.T) {
	path, _ := gateEnv(t)
	gateSet(t, "judge", "off")
	runAll(t, []string{"add", "bug", "old", "-S", "ready"})
	runAll(t, []string{"done", "BUG-001", "ok"})
	if itemOf(t, path, "BUG-001").Judged != "ungated" {
		t.Fatal("not ungated")
	}
	gateSet(t, "judge", "on")
	if d := review(t); len(d.Judge) != 0 {
		t.Fatalf("ungated item queued for judgement: %+v", d)
	}
	out := captureOutput(t, func() { runAll(t, []string{"board"}) })
	if !strings.Contains(out, "100% (1/1)") {
		t.Fatal(out)
	}
}

func TestWaveCloseJudgeAndEvidenceGates(t *testing.T) {
	path, home := judgeBoard(t)
	runAll(t, []string{"wave", "create", "W"}, []string{"wave", "add", "WAV-001", "BUG-001"})
	doneAll(t, "BUG-001")
	w := loadT(t, path)
	wv, _ := itemByID(&w.Tenants[0], "WAV-001")
	for i := range wv.Steps {
		wv.Steps[i].Done = true
	}
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	if d := review(t); len(d.Waves) != 0 {
		t.Fatalf("wave offered with an unjudged member: %+v", d)
	}
	wantErr(t, "unjudged members (mw accept ID): BUG-001", "wave", "close", "WAV-001")
	runAll(t, []string{"accept", "BUG-001", "--by", "me"})
	if d := review(t); len(d.Waves) != 1 || d.Waves[0].ID != "WAV-001" {
		t.Fatalf("%+v", d)
	}
	gateSet(t, "evidence", "on")
	wantErr(t, "evidence required", "wave", "close", "WAV-001")
	if d := review(t); len(d.Waves) != 0 {
		t.Fatalf("review offers a wave whose evidence folder is missing: %+v", d)
	}
	putEvidence(t, home, "WAV-001", map[string]string{"proof.txt": "ok"})
	if d := review(t); len(d.Waves) != 1 {
		t.Fatalf("review omits a wave with evidence: %+v", d)
	}
	runAll(t, []string{"wave", "close", "WAV-001"})
	// judge off: members need no judgement
	gateSet(t, "evidence", "off")
	runAll(t, []string{"wave", "create", "W2"}, []string{"wave", "add", "WAV-002", "TSK-001"})
	doneAll(t, "TSK-001")
	w = loadT(t, path)
	wv, _ = itemByID(&w.Tenants[0], "WAV-002")
	for i := range wv.Steps {
		wv.Steps[i].Done = true
	}
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	wantErr(t, "unjudged", "wave", "close", "WAV-002") // child of unjudged story
	gateSet(t, "judge", "off")
	runAll(t, []string{"wave", "close", "WAV-002"})
}

func TestBurndownCountsJudgedWhenJudgeOn(t *testing.T) {
	_, _ = judgeBoard(t)
	doneAll(t, "BUG-001")
	if out := captureOutput(t, func() { runAll(t, []string{"board"}) }); !strings.Contains(out, "0% (0/4)") {
		t.Fatal(out)
	}
	runAll(t, []string{"accept", "BUG-001", "--by", "me"})
	if out := captureOutput(t, func() { runAll(t, []string{"board"}) }); !strings.Contains(out, "25% (1/4)") {
		t.Fatal(out)
	}
}

func TestNextCarriesEvidenceFolderOnlyWhenGateOn(t *testing.T) {
	_, home := judgeBoard(t)
	if out := captureOutput(t, func() { runAll(t, []string{"next", "-k", "bug"}) }); !strings.Contains(out, "(0 files)") {
		t.Fatal(out)
	}
	gateSet(t, "evidence", "on")
	want := home + "/evidence/gate-app/BUG-001"
	if out := captureOutput(t, func() { runAll(t, []string{"next", "-k", "bug"}) }); !strings.Contains(out, want) {
		t.Fatal(out)
	}
	if out := captureOutput(t, func() { runAll(t, []string{"next", "-k", "bug", "-j"}) }); !strings.Contains(out, `"evidence_dir": "`+want) {
		t.Fatal(out)
	}
}

func TestJudgePromptsGatesOnGoldenLines(t *testing.T) {
	path, _ := gateEnv(t)
	gateSet(t, "evidence", "on")
	gateSet(t, "acceptance", "on")
	gateSet(t, "judge", "on")
	w := loadT(t, path)
	s := buildSessionPrompt(path, w, &w.Tenants[0], "bot", "builder", false)
	d := buildDispatcherPrompt(path, w, "d")
	c := buildCoachPrompt(path, w, "c")
	for p, lines := range map[string][]string{
		s: {"Before `mw done`, write proof into ", "Acceptance for your item is the one in its brief.", "Flow: claim → work → write evidence → `mw done` → idle. Never run `mw accept` or `mw reject`, even on your own work.\n"},
		d: {"Route from `mw review`: judge-list items wait for a human; dispatch only `ready` items; a story with a `Fix:` child goes back to its session.\n\nState:"},
		c: {"Prepare acceptance text (`mw edit ID -c`) before any story is dispatched; a story without it is skipped by `next`.\n",
			"Show the human `mw judge ID` output and wait; you never run `mw accept`.\n- evidence = a file the human can open\n- acceptance = the decision checklist\n- judge = a human says accept/reject"},
	} {
		for _, l := range lines {
			if !strings.Contains(p, l) {
				t.Errorf("missing %q in\n%s", l, p)
			}
		}
	}
	for _, line := range strings.Split(s+d+c, "\n") {
		if strings.HasPrefix(line, "Before `mw done`") {
			continue // carries the evidence path
		}
		if (strings.Contains(line, "Route from") || strings.Contains(line, "Prepare acceptance") || strings.Contains(line, "Show the human") || strings.Contains(line, "Flow: claim") || strings.HasPrefix(line, "Acceptance for")) && len([]rune(line)) > 160 {
			t.Errorf("prompt line over 160 chars (%d): %s", len([]rune(line)), line)
		}
	}
}

func TestAcceptClearsStaleRejectedNote(t *testing.T) {
	path, _ := judgeBoard(t)
	doneAll(t, "TSK-001", "TSK-002")
	runAll(t, []string{"reject", "STR-001", "too slow", "--by", "me"})
	if n := itemOf(t, path, "STR-001").Note; !strings.HasPrefix(n, "rejected") {
		t.Fatalf("setup: %q", n)
	}
	doneAll(t, "TSK-003")
	runAll(t, []string{"accept", "STR-001", "--by", "me"})
	st := itemOf(t, path, "STR-001")
	if st.Status != "done" || st.Note != "" || len(st.Notes) == 0 {
		t.Fatalf("%+v", st)
	}
}

func TestRejectAcceptStoryUndoneChildrenWording(t *testing.T) {
	judgeBoard(t)
	doneAll(t, "TSK-001")
	wantErr(t, "cannot reject STR-001: 1 of 2 children not done (TSK-002 ready)", "reject", "STR-001", "x", "--by", "me")
	wantErr(t, "cannot accept STR-001: 1 of 2 children not done (TSK-002 ready)", "accept", "STR-001", "--by", "me")
}
