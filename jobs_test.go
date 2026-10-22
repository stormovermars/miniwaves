package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func jobsBoard(t *testing.T) string {
	t.Helper()
	path := inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t,
		[]string{"init", "Jobs app"},
		[]string{"tenant", "add", "sheet", "Sheet"},
		[]string{"tenant", "add", "doc", "Doc"},
	)
	return path
}

func TestJobsEnableSetShowDisable(t *testing.T) {
	path := jobsBoard(t)
	runAll(t,
		[]string{"jobs", "enable", "tiny", "-t", "sheet", "-t", "doc"},
		[]string{"jobs", "set", "tiny", "title=Tiny corpus", "max_workers=3", "worktrees=off", "lock=buildlock.sh",
			"news.out=docs/n/", "news.url=https://example.com/n", "summary=out/summary.txt", "pane=off", "pane.style=blocks"},
	)
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	c := w.Jobs["tiny"]
	if c.Title != "Tiny corpus" || c.MaxWorkers != 3 || c.Worktrees != "off" || c.Lock != "buildlock.sh" || c.NewsOut != "docs/n/" ||
		c.NewsURL != "https://example.com/n" || c.Summary != "out/summary.txt" || c.Pane != "off" || c.PaneStyle != "blocks" {
		t.Fatalf("job: %+v", c)
	}
	if w.Version != 9 {
		t.Fatalf("schema %d", w.Version)
	}
	jobOf := map[string]string{}
	for _, tn := range w.Tenants {
		jobOf[tn.Slug] = tn.Job
	}
	if jobOf["sheet"] != "tiny" || jobOf["doc"] != "tiny" || jobOf["jobs-app"] != "" {
		t.Fatalf("tenant jobs: %v", jobOf)
	}
	// the index carries jobs and tenant job, nothing else changed shape
	b, _ := os.ReadFile(path)
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["jobs"].(map[string]any)["tiny"].(map[string]any)["news_out"]; !ok {
		t.Fatalf("index: %s", b)
	}

	out := captureOutput(t, func() { runAll(t, []string{"jobs", "show", "tiny", "-j"}) })
	var v JobView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if v.ID != "tiny" || len(v.Tenants) != 2 || v.MaxWorkers != 3 || v.NewsOut != "docs/n/" {
		t.Fatalf("show: %+v", v)
	}
	// reset to defaults with an empty value
	runAll(t, []string{"jobs", "set", "tiny", "max_workers=", "news.out=", "pane=", "title="})
	out = captureOutput(t, func() { runAll(t, []string{"jobs", "show", "-j"}) })
	var all []JobView
	if err := json.Unmarshal([]byte(out), &all); err != nil || len(all) != 1 {
		t.Fatalf("%v: %s", err, out)
	}
	if all[0].MaxWorkers != 4 || all[0].NewsOut != "docs/news/" || all[0].Pane != "on" || all[0].Title != "tiny" || all[0].Worktrees != "off" {
		t.Fatalf("defaults: %+v", all[0])
	}

	runAll(t, []string{"jobs", "disable", "tiny", "-t", "doc"})
	w, _ = load(path)
	if w.Tenants[2].Job != "" || w.Tenants[1].Job != "tiny" {
		t.Fatalf("disable -t: %+v", w.Tenants)
	}
	if err := run([]string{"jobs", "disable", "tiny", "-t", "doc"}); err == nil {
		t.Fatal("disable of a tenant outside the job should fail")
	}
	runAll(t, []string{"jobs", "disable", "tiny"})
	w, _ = load(path)
	if w.Tenants[1].Job != "" || w.Tenants[2].Job != "" || w.Jobs["tiny"].Worktrees != "off" {
		t.Fatalf("disable keeps job data: %+v %+v", w.Tenants, w.Jobs)
	}
	if err := run([]string{"jobs", "disable", "nope"}); err == nil {
		t.Fatal("unknown job")
	}
}

func TestJobsValidation(t *testing.T) {
	jobsBoard(t)
	runAll(t, []string{"jobs", "enable", "j1", "-t", "sheet"})
	bad := [][]string{
		{"jobs", "enable", "Bad_ID", "-t", "sheet"},
		{"jobs", "enable", "-x", "-t", "sheet"},
		{"jobs", "enable", "UPPER", "-t", "sheet"},
		{"jobs", "enable", strings.Repeat("a", 33), "-t", "sheet"},
		{"jobs", "enable", "j2"},
		{"jobs", "enable", "j2", "-t", "nope"},
		{"jobs", "set", "j1", "news.out=../x"},
		{"jobs", "set", "j1", "news.out=a/../../x"},
		{"jobs", "set", "j1", "news.out=/abs/x"},
		{"jobs", "set", "j1", "news.out=~/x"},
		{"jobs", "set", "j1", `news.out=C:\x`},
		{"jobs", "set", "j1", `news.out=..\x`},
		{"jobs", "set", "j1", "summary=../s.txt"},
		{"jobs", "set", "j1", "summary=/s.txt"},
		{"jobs", "set", "j1", "summary=~"},
		{"jobs", "set", "j1", "summary=."},
		{"jobs", "set", "j1", "pane.style=Bad"},
		{"jobs", "set", "j1", "pane.style=" + strings.Repeat("a", 33)},
		{"jobs", "set", "j1", "pane.style=a b"},
		{"jobs", "set", "j1", "max_workers=0"},
		{"jobs", "set", "j1", "max_workers=33"},
		{"jobs", "set", "j1", "max_workers=x"},
		{"jobs", "set", "j1", "worktrees=maybe"},
		{"jobs", "set", "j1", "lock=rm -rf /"},
		{"jobs", "set", "j1", "news.url=javascript:alert(1)"},
		{"jobs", "set", "j1", "nokey=1"},
		{"jobs", "set", "j1", "title"},
		{"jobs", "set", "ghost", "title=x"},
		{"jobs", "show", "ghost"},
	}
	for _, args := range bad {
		if err := run(args); err == nil {
			t.Errorf("%v: expected an error", args)
		}
	}
	// a failed set stores nothing, even for the valid keys before the bad one
	if err := run([]string{"jobs", "set", "j1", "title=Half", "max_workers=99"}); err == nil {
		t.Fatal("expected error")
	}
	out := captureOutput(t, func() { runAll(t, []string{"jobs", "show", "j1", "-j"}) })
	if strings.Contains(out, "Half") {
		t.Fatalf("partial write: %s", out)
	}
	// clean forms are accepted and normalized
	runAll(t, []string{"jobs", "set", "j1", "news.out=docs//news/./", "summary=out\\s.txt", "max_workers=32", "pane.style=a-1"})
	out = captureOutput(t, func() { runAll(t, []string{"jobs", "show", "j1", "-j"}) })
	var v JobView
	_ = json.Unmarshal([]byte(out), &v)
	if v.NewsOut != "docs/news/" || v.Summary != "out/s.txt" || v.MaxWorkers != 32 {
		t.Fatalf("normalized: %+v", v)
	}
}

func TestJobsReadOnlyShow(t *testing.T) {
	if !readOnly([]string{"jobs", "show"}) || readOnly([]string{"jobs", "set", "a", "b=c"}) || readOnly([]string{"jobs", "enable", "a"}) {
		t.Fatal("jobs show is a read; the rest write")
	}
}

func TestJobsDescription(t *testing.T) {
	path := jobsBoard(t)
	runAll(t, []string{"jobs", "enable", "j1", "-t", "sheet"})
	show := func() JobView {
		var v JobView
		out := captureOutput(t, func() { runAll(t, []string{"jobs", "show", "j1", "-j"}) })
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, `"description"`) {
			t.Fatalf("no description key: %s", out)
		}
		return v
	}
	runAll(t, []string{"jobs", "set", "j1", "description=One line about the job"})
	if v := show(); v.Description != "One line about the job" {
		t.Fatalf("set: %+v", v)
	}
	if txt := captureOutput(t, func() { runAll(t, []string{"jobs", "show", "j1"}) }); !strings.Contains(txt, "One line about the job") {
		t.Fatalf("text show: %s", txt)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"description"`) {
		t.Fatalf("not stored: %s", b)
	}
	// refusals store nothing
	for _, bad := range []string{strings.Repeat("é", 121), "a\nb", "a\x1b[31mb", "a\tb"} {
		if err := run([]string{"jobs", "set", "j1", "description=" + bad}); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
	if v := show(); v.Description != "One line about the job" {
		t.Fatalf("changed by a refusal: %+v", v)
	}
	runAll(t, []string{"jobs", "set", "j1", "description=" + strings.Repeat("é", 120)})
	runAll(t, []string{"jobs", "set", "j1", "description="})
	if v := show(); v.Description != "" {
		t.Fatalf("reset: %+v", v)
	}
	b, _ = os.ReadFile(path)
	if strings.Contains(string(b), `"description"`) {
		t.Fatalf("omitempty: %s", b)
	}
}

func TestJobsCeremoniesKeys(t *testing.T) {
	path := jobsBoard(t)
	runAll(t, []string{"jobs", "enable", "c1", "-t", "sheet"})
	// a job without ceremonies prints as before; ready_min defaults to 3 once ceremonies are on
	out := captureOutput(t, func() { runAll(t, []string{"jobs", "show", "c1", "-j"}) })
	var v JobView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Ceremonies) != 0 || v.ReadyMin != 0 || strings.Contains(out, "ceremonies") || strings.Contains(out, "ready_min") {
		t.Fatalf("defaults: %s", out)
	}
	runAll(t, []string{"jobs", "set", "c1", "ceremonies=planning"})
	out = captureOutput(t, func() { runAll(t, []string{"jobs", "show", "c1", "-j"}) })
	v = JobView{}
	_ = json.Unmarshal([]byte(out), &v)
	if len(v.Ceremonies) != 1 || v.ReadyMin != 3 {
		t.Fatalf("ceremonies on: %s", out)
	}
	// list is validated, de-duplicated and stored in canonical order
	runAll(t, []string{"jobs", "set", "c1", "ceremonies=spike, review,review,triage", "ready_min=7"})
	out = captureOutput(t, func() { runAll(t, []string{"jobs", "show", "c1", "-j"}) })
	v = JobView{}
	_ = json.Unmarshal([]byte(out), &v)
	if strings.Join(v.Ceremonies, ",") != "review,triage,spike" || v.ReadyMin != 7 {
		t.Fatalf("set: %s", out)
	}
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	if w.Jobs["c1"].Ceremonies != "review,triage,spike" || w.Jobs["c1"].ReadyMin != 7 {
		t.Fatalf("stored: %+v", w.Jobs["c1"])
	}
	// unknown names and bad numbers are refused and store nothing
	for _, kv := range []string{"ceremonies=review,standup", "ceremonies=Review", "ceremonies=release", "ready_min=0", "ready_min=21", "ready_min=x"} {
		if err := run([]string{"jobs", "set", "c1", kv}); err == nil {
			t.Errorf("%s: expected an error", kv)
		}
	}
	if err := run([]string{"jobs", "set", "c1", "ceremonies=planning", "ready_min=99"}); err == nil {
		t.Fatal("expected error")
	}
	w, _ = load(path)
	if w.Jobs["c1"].Ceremonies != "review,triage,spike" {
		t.Fatalf("partial write: %+v", w.Jobs["c1"])
	}
	// the boundaries and the reset
	runAll(t, []string{"jobs", "set", "c1", "ready_min=1"})
	runAll(t, []string{"jobs", "set", "c1", "ready_min=20"})
	runAll(t, []string{"jobs", "set", "c1", "ceremonies=", "ready_min="})
	w, _ = load(path)
	if c := w.Jobs["c1"]; c.Ceremonies != "" || c.ReadyMin != 0 {
		t.Fatalf("reset: %+v", c)
	}
	// text show works and the keys survive a round trip
	runAll(t, []string{"jobs", "set", "c1", "ceremonies=planning"})
	_ = captureOutput(t, func() { runAll(t, []string{"jobs", "show"}) })
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"ceremonies": "planning"`) && !strings.Contains(string(b), `"ceremonies":"planning"`) {
		t.Fatalf("index: %s", b)
	}
}
