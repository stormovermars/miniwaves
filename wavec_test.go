package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionStartIgnoresExportedSession(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "ws"},
		[]string{"tenant", "add", "b", "B"},
		[]string{"session", "start", "one", "--tenant", "ws"})
	t.Setenv("MW_SESSION", "one")
	runAll(t, []string{"session", "start", "two", "--tenant", "b"})
	w := loadT(t, path)
	s, _ := sessionByID(w, "two")
	if s.Tenant != "b" {
		t.Fatalf("session two on %s", s.Tenant)
	}
}

const multiAcc = "opens fast\nno crash\nlogs clean"

func accBoard(t *testing.T) string {
	path, _ := judgeBoard(t)
	runAll(t, []string{"edit", "STR-001", "-c", multiAcc})
	gateSet(t, "acceptance", "on")
	return path
}

func TestNextPrintsFullAcceptance(t *testing.T) {
	accBoard(t)
	out := captureOutput(t, func() { runAll(t, []string{"next", "-k", "task"}) })
	for _, want := range []string{"acceptance (from STR-001):\n  opens fast\n  no crash\n  logs clean\n"} {
		if !strings.Contains(out, want) || strings.Contains(out, "…") {
			t.Fatalf("%s", out)
		}
	}
	j := captureOutput(t, func() { runAll(t, []string{"next", "-k", "task", "-j"}) })
	if !strings.Contains(j, `"acceptance": "`+strings.ReplaceAll(multiAcc, "\n", `\n`)+`"`) {
		t.Fatalf("%s", j)
	}
}

func TestInitAndTenantAddMessages(t *testing.T) {
	path := inTempDir(t)
	out := captureOutput(t, func() { runAll(t, []string{"init"}) })
	if !strings.Contains(out, "created tenant "+filepath.Base(filepath.Dir(path))) {
		t.Fatalf("%s", out)
	}
	runAll(t, []string{"tenant", "add", "x", "X"})
	d := filepath.Join(filepath.Dir(path), "elsewhere")
	out = captureOutput(t, func() { runAll(t, []string{"tenant", "add", "x", "-d", d}) })
	if !strings.Contains(out, "updated tenant x") {
		t.Fatalf("%s", out)
	}
	w := loadT(t, path)
	if tn, _ := tenantBySlug(w, "x"); tn.Path != d || tn.Name != "X" {
		t.Fatalf("%+v", tn)
	}
	wantErr(t, "already exists", "tenant", "add", "x")
}

func TestShowAcceptance(t *testing.T) {
	accBoard(t)
	out := captureOutput(t, func() { runAll(t, []string{"show", "STR-001"}) })
	if !strings.Contains(out, "acceptance:") || !strings.Contains(out, "opens fast") {
		t.Fatalf("%s", out)
	}
	out = captureOutput(t, func() { runAll(t, []string{"show", "TSK-001"}) })
	if !strings.Contains(out, "acceptance (from STR-001):") {
		t.Fatalf("child brief lacks inherited acceptance: %s", out)
	}
	out = captureOutput(t, func() { runAll(t, []string{"show", "TSK-001", "-v"}) })
	if !strings.Contains(out, "opens fast") || !strings.Contains(out, "(from STR-001)") {
		t.Fatalf("%s", out)
	}
	out = captureOutput(t, func() { runAll(t, []string{"show", "STR-001", "-v"}) })
	if strings.Contains(out, "inherited") {
		t.Fatalf("%s", out)
	}
}

func TestStoryDoingOnFirstChildStart(t *testing.T) {
	path, _ := judgeBoard(t)
	status := func(id string) string { return itemOf(t, path, id).Status }
	if status("STR-001") != "ready" {
		t.Fatal(status("STR-001"))
	}
	runAll(t, []string{"start", "TSK-001"})
	if status("STR-001") != "doing" {
		t.Fatalf("story %s", status("STR-001"))
	}
	before := itemOf(t, path, "STR-001")
	runAll(t, []string{"start", "TSK-002", "-s", "other"}) // already doing: untouched
	if after := itemOf(t, path, "STR-001"); after.Status != "doing" || after.Updated != before.Updated {
		t.Fatalf("%+v", after)
	}
	runAll(t, []string{"reopen", "TSK-001"})
	if status("STR-001") != "doing" {
		t.Fatalf("reopen changed the story: %s", status("STR-001"))
	}
	doneAll(t, "TSK-002")
	if status("STR-001") != "doing" {
		t.Fatalf("done of a child changed the story: %s", status("STR-001"))
	}
	out := captureOutput(t, func() { runAll(t, []string{"board", "-v"}) })
	if !strings.Contains(out, "STR-001") {
		t.Fatalf("%s", out)
	}
}

func TestInboxStoryDoingOnFirstChildStart(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "ws"}, []string{"add", "story", "S"}, []string{"add", "task", "T", "-p", "STR-001", "-S", "ready"},
		[]string{"start", "TSK-001"})
	if st := itemOf(t, path, "STR-001").Status; st != "doing" {
		t.Fatal(st)
	}
}

func TestWaveStepNARest(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "ws"}, []string{"wave", "create", "W"})
	w := loadT(t, path)
	wv, _ := itemByID(&w.Tenants[0], "WAV-001")
	total, first := len(wv.Steps), wv.Steps[0].Key
	runAll(t, []string{"wave", "step", "WAV-001", first + "=did it"})
	out := captureOutput(t, func() { runAll(t, []string{"wave", "step", "WAV-001", "--na-rest", "solo repo"}) })
	if !strings.Contains(out, fmt.Sprintf("%d steps n/a", total-1)) {
		t.Fatalf("%s", out)
	}
	w = loadT(t, path)
	wv, _ = itemByID(&w.Tenants[0], "WAV-001")
	for i, s := range wv.Steps {
		if !s.Done || (i == 0) == s.NA || (i > 0 && s.Note != "solo repo") {
			t.Fatalf("%+v", s)
		}
	}
	wantErr(t, "usage", "wave", "step", "WAV-001", first+"=x", "--na-rest", "y")
}

func TestBoardPercentFollowsJudge(t *testing.T) {
	judgeBoard(t)
	runAll(t, []string{"start", "TSK-001"})
	doneAll(t, "BUG-001")
	var d boardDoc
	out := captureOutput(t, func() { runAll(t, []string{"board", "-j"}) })
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	// 4 items: story ready, TSK-001 doing, TSK-002 ready, BUG-001 done but unjudged
	if d.Total != 4 || d.Done != 0 || d.Percent != 0 {
		t.Fatalf("%+v", d)
	}
	runAll(t, []string{"accept", "BUG-001", "--by", "me"})
	if hdr := captureOutput(t, func() { runAll(t, []string{"board"}) }); !strings.Contains(hdr, "25% (1/4)") {
		t.Fatalf("%s", hdr)
	}
	runAll(t, []string{"done", "TSK-001", "x"})
	gateSet(t, "judge", "off")
	out = captureOutput(t, func() { runAll(t, []string{"board", "-j"}) })
	if err := json.Unmarshal([]byte(out), &d); err != nil || d.Done != 2 || d.Percent != 50 {
		t.Fatalf("%v %+v", err, d)
	}
}

func TestTenantAddUpdateRefusals(t *testing.T) {
	path := inTempDir(t)
	base := filepath.Dir(path)
	runAll(t, []string{"init", "ws"}, []string{"tenant", "add", "a", "A", "-d", base + "/a"}, []string{"tenant", "add", "b", "B", "-d", base + "/b"})
	wantErr(t, "held by tenant b", "tenant", "add", "a", "-d", base+"/b")
	wantErr(t, "no NAME", "tenant", "add", "a", "New Name", "-d", base+"/c")
	out := captureOutput(t, func() { runAll(t, []string{"tenant", "add", "a", "-d", base + "/c"}) })
	if !strings.Contains(out, base+"/a -> "+base+"/c") {
		t.Fatalf("%s", out)
	}
}

func TestShowVJSONAcceptanceFrom(t *testing.T) {
	accBoard(t)
	out := captureOutput(t, func() { runAll(t, []string{"show", "TSK-001", "-v", "-j"}) })
	if !strings.Contains(out, `"acceptance_from": "STR-001"`) || strings.Contains(out, "inherited") {
		t.Fatalf("%s", out)
	}
	out = captureOutput(t, func() { runAll(t, []string{"show", "STR-001", "-v", "-j"}) })
	if strings.Contains(out, "acceptance_from") {
		t.Fatalf("%s", out)
	}
}

func TestRejectFixStartAccept(t *testing.T) {
	path, _ := judgeBoard(t)
	doneAll(t, "TSK-001", "TSK-002")
	runAll(t, []string{"reject", "STR-001", "needs polish", "--by", "me"})
	if st := itemOf(t, path, "STR-001").Status; st != "ready" {
		t.Fatal(st)
	}
	runAll(t, []string{"start", "TSK-003"})
	if st := itemOf(t, path, "STR-001").Status; st != "doing" {
		t.Fatalf("story %s", st)
	}
	doneAll(t, "TSK-003")
	runAll(t, []string{"accept", "STR-001", "--by", "me"})
	if it := itemOf(t, path, "STR-001"); it.Status != "done" || it.Judged == "" {
		t.Fatalf("%+v", it)
	}
}

func TestWaveStepSeveralInOneCall(t *testing.T) {
	path, _ := gateEnv(t)
	runAll(t, []string{"wave", "create", "W"})
	steps := loadT(t, path).Tenants[0].Items[0].Steps
	a, b, c := steps[0].Key, steps[1].Key, steps[2].Key
	// all-or-nothing: a bad pair ticks nothing
	wantErr(t, "no step", "wave", "step", "WAV-001", a+"=ok", "nope=x")
	wantErr(t, "KEY=NOTE", "wave", "step", "WAV-001", a+"=ok", b)
	if steps = loadT(t, path).Tenants[0].Items[0].Steps; steps[0].Done {
		t.Fatal("partial tick")
	}
	runAll(t, []string{"wave", "step", "WAV-001", a + "=19 pass", b + "=bump 0.5.0"})
	steps = loadT(t, path).Tenants[0].Items[0].Steps
	if !steps[0].Done || !steps[1].Done || steps[0].Note != "19 pass" || steps[1].Note != "bump 0.5.0" || steps[2].Done {
		t.Fatalf("%+v", steps[:3])
	}
	runAll(t, []string{"wave", "step", "WAV-001", c + "=none", "--na"})
	if s := loadT(t, path).Tenants[0].Items[0].Steps[2]; !s.NA || !s.Done {
		t.Fatalf("%+v", s)
	}
	out := captureOutput(t, func() { runAll(t, []string{"log", "-n", "2"}) })
	if !strings.Contains(out, "step") || !strings.Contains(out, c) {
		t.Fatal(out)
	}
}
