package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// crossBoard: tenant app (item TSK-001, TSK-002; session wk, answerer boss) and tenant job (session boss).
func crossBoard(t *testing.T) string {
	t.Helper()
	path := inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t,
		[]string{"init", "App"},
		[]string{"gate", "level", "trust", "--workspace", "--by", "t"},
		[]string{"tenant", "add", "job", "Job"},
		[]string{"add", "task", "Build", "-t", "app", "-S", "ready"},
		[]string{"add", "task", "Second", "-t", "app", "-S", "ready"},
		[]string{"session", "start", "boss", "-t", "job"},
		[]string{"session", "start", "wk", "-t", "app", "--answerer", "boss"},
	)
	return path
}

func reviewDocOf(t *testing.T, args ...string) (needs, forAns []needRow) {
	t.Helper()
	var d struct {
		Needs []needRow `json:"needs"`
		For   []needRow `json:"for_answerers"`
	}
	out := captureOutput(t, func() { runAll(t, append([]string{"review", "-j"}, args...)) })
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return d.Needs, d.For
}

func statusN(t *testing.T) int {
	t.Helper()
	w, err := load(mustBoard(t))
	if err != nil {
		t.Fatal(err)
	}
	return statusCounts(w).Needs
}

func mustBoard(t *testing.T) string {
	t.Helper()
	wd, _ := os.Getwd()
	return wd + string(os.PathSeparator) + ".miniwaves.json"
}

// An answerer on another tenant (a coordinator on the job tenant, workers on app tenants) works everywhere.
func TestAnswererCrossTenant(t *testing.T) {
	crossBoard(t)
	runAll(t, []string{"ask", "TSK-001", "which format?", "-t", "app", "-s", "wk"})
	for _, args := range [][]string{{"-t", "app"}, {"-t", "job"}, nil} {
		if n, f := reviewDocOf(t, args...); (len(args) == 0 || args[1] == "app") && (len(n) != 0 || len(f) != 1 || f[0].For != "boss") {
			t.Fatalf("review %v: needs %+v for %+v", args, n, f)
		}
	}
	if n := statusN(t); n != 0 {
		t.Fatalf("status N must not count the routed ask: %d", n)
	}
	// boss answers it from its own tenant, with and without -t
	runAll(t, []string{"answer", "TSK-001", "csv", "-s", "boss"})
	if n, f := reviewDocOf(t, "-t", "app"); len(n)+len(f) != 0 {
		t.Fatalf("answered: %+v %+v", n, f)
	}
	runAll(t, []string{"ask", "TSK-002", "again?", "-t", "app", "-s", "wk"})
	t.Setenv("MW_SESSION", "boss")
	runAll(t, []string{"answer", "TSK-002", "--accept", "-t", "app"}) // MW_SESSION path
	t.Setenv("MW_SESSION", "")
	if n, f := reviewDocOf(t, "-t", "app"); len(n)+len(f) != 0 {
		t.Fatalf("accepted: %+v %+v", n, f)
	}
	// a stopped boss: the ask is the human's again, in every list
	runAll(t, []string{"ask", "TSK-001", "third?", "-t", "app", "-s", "wk"}, []string{"session", "stop", "boss", "-t", "job"})
	if n, f := reviewDocOf(t, "-t", "app"); len(n) != 1 || len(f) != 0 {
		t.Fatalf("stopped boss, review: %+v %+v", n, f)
	}
	if n := statusN(t); n != 1 {
		t.Fatalf("stopped boss, status N = %d", n)
	}
	if err := run([]string{"answer", "TSK-001", "x", "-s", "boss"}); err == nil {
		t.Fatal("a stopped boss must not answer")
	}
}

// Duplicate ids in two tenant files: the record that is not stopped counts.
func TestAnswererDuplicateIDs(t *testing.T) {
	crossBoard(t)
	runAll(t, []string{"session", "stop", "boss", "-t", "job"})
	runAll(t, []string{"session", "start", "wk2", "-t", "job", "--answerer", "boss2"})
	// a stopped boss in app (listed first) and a live one in job
	runAll(t, []string{"session", "start", "boss", "-t", "app"}, []string{"session", "stop", "boss", "-t", "app"}, []string{"session", "start", "boss", "-t", "job"})
	runAll(t, []string{"ask", "TSK-001", "q?", "-t", "app", "-s", "wk"})
	if n, f := reviewDocOf(t, "-t", "app"); len(n) != 0 || len(f) != 1 {
		t.Fatalf("live boss in job must count: %+v %+v", n, f)
	}
	runAll(t, []string{"answer", "TSK-001", "ok", "-s", "boss"})
}

func TestAnswererCycleAcrossTenants(t *testing.T) {
	crossBoard(t)
	runAll(t, []string{"session", "stop", "boss", "-t", "job"})
	// boss restarts on job with wk (app) as answerer: wk -> boss -> wk
	err := run([]string{"session", "start", "boss", "-t", "job", "--answerer", "wk"})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cross-tenant cycle must be refused: %v", err)
	}
}

// An answerer set after an ask does not take that ask over.
func TestAnswererRetroactive(t *testing.T) {
	path := crossBoard(t)
	runAll(t, []string{"ask", "TSK-001", "old question", "-t", "app", "-s", "wk"})
	w, _ := load(path)
	s, _ := sessionByID(w, "wk")
	if s.AnswererAt == "" {
		t.Fatal("answerer_at must be recorded")
	}
	s.AnswererAt = "2099-01-01T00:00:00Z" // set after the ask
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	if n, f := reviewDocOf(t, "-t", "app"); len(n) != 1 || len(f) != 0 {
		t.Fatalf("an ask older than the answerer is the human's: %+v %+v", n, f)
	}
	if err := run([]string{"answer", "TSK-001", "x", "-s", "boss"}); err == nil {
		t.Fatal("boss must not answer an ask older than the answerer")
	}
}

// A session that is not the answerer gets exactly today's refusal (checked once against a 0.3.15 binary: same text, no stdout, no stderr).
func TestAnswerRefusalUnchanged(t *testing.T) {
	crossBoard(t)
	runAll(t, []string{"ask", "TSK-001", "q?", "-t", "app", "-s", "wk"})
	var err error
	var out string
	errText := captureFile(t, &os.Stderr, func() {
		out = captureOutput(t, func() { err = run([]string{"answer", "TSK-001", "x", "-s", "stranger", "-t", "app"}) })
	})
	var ee *exitError
	if err == nil || err.Error() != "mw answer is for humans: unset MW_SESSION and drop -s" || out != "" || errText != "" {
		t.Fatalf("refusal changed: err=%v out=%q stderr=%q", err, out, errText)
	}
	_ = ee
}

func TestAnswererMisc(t *testing.T) {
	crossBoard(t)
	// --by with -s takes the human path: refused, the ask stays open for the routing
	runAll(t, []string{"ask", "TSK-001", "q?", "-t", "app", "-s", "wk"})
	err := run([]string{"answer", "TSK-001", "x", "-s", "boss", "--by", "Sir", "-t", "app"})
	if err == nil || !strings.Contains(err.Error(), "humans") {
		t.Fatalf("--by with -s: %v", err)
	}
	// done closes a routed ask
	runAll(t, []string{"done", "TSK-001", "ok", "--no-evidence", "x", "-t", "app"})
	if n, f := reviewDocOf(t, "-t", "app"); len(n)+len(f) != 0 {
		t.Fatalf("done must close the routed ask: %+v %+v", n, f)
	}
	// ask, ai answer, ask again: routed again
	runAll(t, []string{"ask", "TSK-002", "one?", "-t", "app", "-s", "wk"}, []string{"answer", "TSK-002", "yes", "-s", "boss"}, []string{"ask", "TSK-002", "two?", "-t", "app", "-s", "wk"})
	if n, f := reviewDocOf(t, "-t", "app"); len(n) != 0 || len(f) != 1 || f[0].Question != "two?" {
		t.Fatalf("re-ask: %+v %+v", n, f)
	}
}

// Sprint rows of the news model do not count a routed ask as needing the human.
func TestAnswererSprintNeeds(t *testing.T) {
	path := crossBoard(t)
	runAll(t, []string{"wave", "create", "Sprint", "-t", "app"}, []string{"wave", "add", "WAV-001", "TSK-001", "-t", "app"},
		[]string{"ask", "TSK-001", "q?", "-t", "app", "-s", "wk"})
	w, _ := load(path)
	m := buildNewsModel(w, newsPart{title: w.Name, tenants: newsJobTenants(w, "")}, nil, newsWindow{full: true})
	if m.Now.Sprint == nil || len(m.Now.Sprint.Items) == 0 {
		t.Fatalf("no sprint rows: %+v", m.Now.Sprint)
	}
	for _, it := range m.Now.Sprint.Items {
		if it.Needs {
			t.Fatalf("a routed ask must not mark the sprint row as needing the human: %+v", it)
		}
	}
}

func ageSession(t *testing.T, path, id string) {
	t.Helper()
	w, _ := load(path)
	s, _ := sessionByID(w, id)
	s.Updated = "2020-01-01T00:00:00Z"
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
}

// reap counts an answer or decide event of the session as activity: an answerer that only answers is not reaped.
func TestReapSparesAnswerer(t *testing.T) {
	path := crossBoard(t)
	runAll(t, []string{"session", "start", "idle", "-t", "job"})
	runAll(t, []string{"ask", "TSK-001", "q?", "-t", "app", "-s", "wk"}, []string{"answer", "TSK-001", "yes", "-s", "boss"})
	ageSession(t, path, "boss")
	ageSession(t, path, "idle")
	runAll(t, []string{"session", "reap", "-t", "job", "--older-than", "2h"}) // job only: the answer sits on app
	w, _ := load(path)
	if s, _ := sessionByID(w, "boss"); s.State == "stopped" {
		t.Fatal("an answerer that answered recently must not be reaped")
	}
	if s, _ := sessionByID(w, "idle"); s.State != "stopped" {
		t.Fatal("a session with no activity is still reaped")
	}
}

// An item id that matches an answerable ask in two tenants is refused, not guessed.
func TestAnswererAmbiguousID(t *testing.T) {
	crossBoard(t)
	runAll(t,
		[]string{"tenant", "add", "app2", "App2"},
		[]string{"add", "task", "Other", "-t", "app2", "-S", "ready"},
		[]string{"session", "start", "wk2", "-t", "app2", "--answerer", "boss"},
		[]string{"ask", "TSK-001", "a?", "-t", "app", "-s", "wk"},
		[]string{"ask", "TSK-001", "b?", "-t", "app2", "-s", "wk2"},
	)
	err := run([]string{"answer", "TSK-001", "x", "-s", "boss"})
	if err == nil || err.Error() != "TSK-001 is in several tenants; add -t SLUG" {
		t.Fatalf("ambiguous id: %v", err)
	}
	runAll(t, []string{"answer", "TSK-001", "x", "-s", "boss", "-t", "app2"})
}
