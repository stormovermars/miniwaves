package mw

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestAskOptionsValidation(t *testing.T) {
	needsBoard(t)
	for _, c := range [][]string{
		{"ask", "TSK-001", "Q?", "--option", "A"},
		{"ask", "TSK-001", "Q?", "--option", "A", "--option", "B", "--option", "C", "--option", "D", "--option", "E"},
		{"ask", "TSK-001", "Q?", "--option", "A", "--option", "a"},
		{"ask", "TSK-001", "Q?", "--option", "A", "--option", " "},
	} {
		if err := run(c); err == nil {
			t.Fatalf("%v must be refused", c)
		}
	}
	if n := len(reviewNeeds(t)); n != 0 {
		t.Fatalf("refused asks must record nothing: %d", n)
	}
}

func TestAskOptionsAnswer(t *testing.T) {
	path := needsBoard(t)
	runAll(t, []string{"ask", "TSK-001", "Ship now?", "--option", "yes", "--option", "Later, maybe"})
	if err := run([]string{"answer", "TSK-001", "never", "--by", "Sir"}); err == nil || !strings.Contains(err.Error(), "not one of the options") {
		t.Fatalf("answer outside the options: %v", err)
	}
	if err := run([]string{"answer", "TSK-001", "--accept", "--by", "Sir"}); err == nil || !strings.Contains(err.Error(), "--accept is refused") {
		t.Fatalf("accept with options: %v", err)
	}
	if len(reviewNeeds(t)) != 1 {
		t.Fatal("refused answers must not close the ask")
	}
	runAll(t, []string{"answer", "TSK-001", "later, MAYBE", "--by", "Sir"})
	w, _ := load(path)
	it, _ := itemByID(&w.Tenants[0], "TSK-001")
	if last := it.Events[len(it.Events)-1]; last.Action != "answer" || last.Note != "Later, maybe" {
		t.Fatalf("answer is stored as the option text: %+v", last)
	}
	// no options: any answer, --accept still works
	runAll(t, []string{"ask", "TSK-002", "Free?"}, []string{"answer", "TSK-002", "whatever", "--by", "Sir"})
	runAll(t, []string{"ask", "TSK-002", "Again?"}, []string{"answer", "TSK-002", "--accept", "--by", "Sir"})
}

func TestAskOptionsAnswererPath(t *testing.T) {
	needsBoard(t)
	runAll(t,
		[]string{"session", "start", "boss"},
		[]string{"session", "start", "bot", "--answerer", "boss"},
		[]string{"ask", "TSK-001", "Pick?", "--option", "x", "--option", "y", "-s", "bot"},
	)
	if err := run([]string{"answer", "TSK-001", "z", "-s", "boss"}); err == nil || !strings.Contains(err.Error(), "not one of the options") {
		t.Fatalf("answerer outside the options: %v", err)
	}
	runAll(t, []string{"answer", "TSK-001", "x", "-s", "boss"})
}

func TestEventKeepsUnknownFields(t *testing.T) {
	in := `{"at":"2026-01-01T00:00:00Z","action":"ask","note":"Q","options":["a","b"],"surface":{"k":[1,2]},"zeta":"z"}`
	var e Event
	if err := json.Unmarshal([]byte(in), &e); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if string(m["surface"]) != `{"k":[1,2]}` || string(m["zeta"]) != `"z"` || string(m["options"]) != `["a","b"]` {
		t.Fatalf("unknown event field lost: %s", b)
	}
	plain, _ := json.Marshal(Event{At: "t", Action: "note"})
	if string(plain) != `{"at":"t","action":"note"}` {
		t.Fatalf("an event without options or extras must serialize as before: %s", plain)
	}
}

func TestEventUnknownFieldSurvivesSave(t *testing.T) {
	path := needsBoard(t)
	runAll(t, []string{"ask", "TSK-001", "Q?"})
	tp := tenantPath(path, "needs-app")
	if _, err := os.Stat(tp); err != nil {
		tp = path
	}
	b, err := os.ReadFile(tp)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.Replace(string(b), `"action": "ask"`, `"action": "ask", "future_field": {"a": 1}`, 1)
	if s == string(b) {
		s = strings.Replace(string(b), `"action":"ask"`, `"action":"ask","future_field":{"a":1}`, 1)
	}
	if s == string(b) {
		t.Fatalf("could not inject a field into %s", tp)
	}
	if err := os.WriteFile(tp, []byte(s), 0644); err != nil {
		t.Fatal(err)
	}
	runAll(t, []string{"note", "TSK-001", "touch"}) // load + save
	out, _ := os.ReadFile(tp)
	if !strings.Contains(string(out), "future_field") {
		t.Fatalf("event field dropped by a save:\n%s", out)
	}
}

var (
	askTSRe  = regexp.MustCompile(`\d{4}-\d\d-\d\dT[\d:.]+(?:Z|[+-]\d\d:\d\d)`)
	askAgoRe = regexp.MustCompile(`\b(now|\d+[mhd] ago)\b`)
)

func TestGoldenAskOptions(t *testing.T) {
	p, err := filepath.Abs(filepath.Join("testdata", "golden-ask-options.txt")) // before needsBoard changes directory
	if err != nil {
		t.Fatal(err)
	}
	needsBoard(t)
	runAll(t, []string{"ask", "TSK-001", "Ship the index?", "--option", "yes", "--option", "later", "--option", "never", "-s", "bot"})
	norm := func(s string) string { return askAgoRe.ReplaceAllString(askTSRe.ReplaceAllString(s, "<TS>"), "<AGO>") }
	got := "$ mw review\n" + norm(captureOutput(t, func() { runAll(t, []string{"review"}) })) +
		"$ mw review -j (needs)\n" + norm(needsJSON(t)) + "\n"
	if os.Getenv("GOLDEN_UPDATE") == "1" {
		if err := os.WriteFile(p, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Errorf("golden ask-options changed:\n--- want\n%s\n--- got\n%s", want, got)
	}
}

func needsJSON(t *testing.T) string {
	t.Helper()
	out := captureOutput(t, func() { runAll(t, []string{"review", "-j"}) })
	var d struct {
		Needs []json.RawMessage `json:"needs"`
	}
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatal(err)
	}
	b, _ := json.MarshalIndent(d.Needs, "", "  ")
	return string(b)
}
