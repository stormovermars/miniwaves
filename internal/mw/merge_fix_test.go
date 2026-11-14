package mw

import (
	"encoding/json"
	"strings"
	"testing"
)

// row with a session owner and nanosecond log time (distinct origins for rows created on two computers)
func orow(id, title, status, session, startedOn, created, updated string, events ...string) string {
	s := `{"id":"` + id + `","kind":"task","title":"` + title + `","status":"` + status + `"`
	if session != "" {
		s += `,"session":"` + session + `"`
	}
	if startedOn != "" {
		s += `,"started_on":"` + startedOn + `"`
	}
	return s + `,"events":[` + strings.Join(events, ",") + `],"created":"` + created + `","updated":"` + updated + `"}`
}

func mergeSib(t *testing.T, b, o, th, sib string) []byte {
	t.Helper()
	out, err := mergeBoardFilesSib([]byte(b), []byte(o), []byte(th), []byte(sib))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func rowsOf(t *testing.T, out []byte) map[string]Item {
	t.Helper()
	var td tenantDoc
	if err := json.Unmarshal(out, &td); err != nil {
		t.Fatal(err)
	}
	m := map[string]Item{}
	for _, it := range td.Items {
		m[it.ID] = it
	}
	return m
}

func TestFixArchiveCollision(t *testing.T) {
	ea := ev("2026-01-01T00:00:00.100000000Z", "created", "")
	eb := ev("2026-01-01T00:00:00.700000000Z", "created", "")
	ia := orow("TSK-006", "alpha X", "done", "", "", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z", ea)
	ib := orow("TSK-006", "beta Y", "done", "", "", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z", eb)
	base := `{"version":9,"items":[]}`
	ours := `{"version":9,"items":[` + ia + `]}`
	theirs := `{"version":9,"items":[` + ib + `]}`
	out := mergeSib(t, base, ours, theirs, "")
	got := rowsOf(t, out)
	titles := ""
	for _, it := range got {
		titles += it.Title + "|"
	}
	if len(got) != 2 || !strings.Contains(titles, "alpha X") || !strings.Contains(titles, "beta Y") {
		t.Fatalf("a row was lost in the archive: %s", out)
	}
	// the archive holds beta's row as TSK-006 while ours tenant file has a different TSK-006: the archived one is renumbered
	tenantOurs := `{"next":{"TSK":6},"items":[` + ia + `]}`
	out = mergeSib(t, base, `{"version":9,"items":[]}`, `{"version":9,"items":[`+ib+`]}`, tenantOurs)
	got = rowsOf(t, out)
	if _, kept := got["TSK-006"]; kept || len(got) != 1 {
		t.Fatalf("archived row kept the id of a different live row: %s", out)
	}
	// and a tenant row whose id is archived (a different row) in ours' archive is renumbered
	arch := `{"version":9,"items":[` + ib + `]}`
	out = mergeSib(t, `{"next":{},"items":[]}`, `{"next":{"TSK":6},"items":[`+ia+`]}`, `{"next":{"TSK":6},"items":[`+ia+`]}`, arch)
	got = rowsOf(t, out)
	if _, kept := got["TSK-006"]; kept || len(got) != 1 {
		t.Fatalf("live row kept the id of a different archived row: %s", out)
	}
}

func TestFixRenumberKeepsOwner(t *testing.T) {
	ea := ev("2026-01-01T00:00:00.100000000Z", "start", "")
	eb := ev("2026-01-01T00:00:00.700000000Z", "start", "")
	ia := orow("TSK-002", "alpha job", "doing", "s-alpha", "alpha", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", ea)
	ib := orow("TSK-002", "beta job", "doing", "s-beta", "beta", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", eb)
	sa := `{"id":"s-alpha","tenant":"t","state":"working","item":"TSK-002","updated":"2026-01-01T00:00:00Z"}`
	sb := `{"id":"s-beta","tenant":"t","state":"working","item":"TSK-002","updated":"2026-01-01T00:00:00Z"}`
	base := `{"next":{"TSK":1},"items":[]}`
	ours := `{"next":{"TSK":2},"items":[` + ia + `],"sessions":[` + sa + `]}`
	theirs := `{"next":{"TSK":2},"items":[` + ib + `],"sessions":[` + sb + `]}`
	out := mergeSib(t, base, ours, theirs, "")
	var td tenantDoc
	if err := json.Unmarshal(out, &td); err != nil {
		t.Fatal(err)
	}
	byTitle := map[string]Item{}
	for _, it := range td.Items {
		byTitle[it.Title] = it
	}
	item := map[string]string{}
	for _, s := range td.Sessions {
		item[s.ID] = s.Item
	}
	if byTitle["alpha job"].ID == byTitle["beta job"].ID {
		t.Fatalf("not renumbered: %s", out)
	}
	for who, title := range map[string]string{"s-alpha": "alpha job", "s-beta": "beta job"} {
		if item[who] != byTitle[title].ID {
			t.Fatalf("session %s holds %s but its row is %s: %s", who, item[who], byTitle[title].ID, out)
		}
	}
	// the same merge from the other side gives the same file
	if again := mergeSib(t, base, theirs, ours, ""); string(again) != string(out) {
		t.Fatalf("direction changes the result:\n%s\n%s", out, again)
	}
}

func TestFixDoubleClaimIsLogged(t *testing.T) {
	e0 := ev("2026-01-01T00:00:00Z", "created", "")
	base := `{"next":{"TSK":3},"items":[` + orow("TSK-003", "job", "ready", "", "", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", e0) + `]}`
	a := `{"next":{"TSK":3},"items":[` + orow("TSK-003", "job", "doing", "s-a2", "alpha", "2026-01-01T00:00:00Z", "2026-01-01T00:10:00Z", e0, ev("2026-01-01T00:10:00Z", "start", "")) + `]}`
	b := `{"next":{"TSK":3},"items":[` + orow("TSK-003", "job", "doing", "s-b2", "beta", "2026-01-01T00:00:00Z", "2026-01-01T00:10:05Z", e0, ev("2026-01-01T00:10:05Z", "start", "")) + `]}`
	out := mergeSib(t, base, a, b, "")
	it := rowsOf(t, out)["TSK-003"]
	last := it.Events[len(it.Events)-1]
	if it.Assignee != "s-b2" || last.Action != "merge" || !strings.HasPrefix(last.Note, "clash") || !strings.Contains(last.Note, "s-a2") || !strings.Contains(last.Note, "s-b2") {
		t.Fatalf("double claim not logged: %+v", it)
	}
	if again := mergeSib(t, base, b, a, ""); string(again) != string(out) {
		t.Fatalf("direction changes the result")
	}
}

func TestFixUnknownFieldsAndOrder(t *testing.T) {
	e0 := ev("2026-01-01T00:00:00Z", "created", "")
	it := func(id, created string) string {
		return orow(id, "row "+id, "ready", "", "", created, created, ev(created, "created", ""))
	}
	_ = e0
	base := `{"next":{"TSK":1},"future":1,"items":[` + it("TSK-001", "2026-01-01T00:00:00Z") + `],"sessions":[{"id":"s1","tenant":"t","state":"idle","updated":"2026-01-01T00:00:00Z","x":2}]}`
	ours := strings.Replace(base, `"TSK":1`, `"TSK":2`, 1)
	ours = strings.Replace(ours, `],"sessions"`, `,`+it("TSK-002", "2026-01-02T00:00:00Z")+`],"sessions"`, 1)
	theirs := strings.Replace(base, `"TSK":1`, `"TSK":3`, 1)
	theirs = strings.Replace(theirs, `],"sessions"`, `,`+it("TSK-003", "2026-01-03T00:00:00Z")+`],"sessions"`, 1)
	out := mergeSib(t, base, ours, theirs, "")
	for _, want := range []string{`"future":1`, `"x":2`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("lost %s: %s", want, out)
		}
	}
	if again := mergeSib(t, base, theirs, ours, ""); string(again) != string(out) {
		t.Fatalf("row order depends on the side:\n%s\n%s", out, again)
	}
	ix := `{"version":9,"name":"W","tenants":[{"slug":"a","name":"A","y":3}]}`
	if o2 := mergeSib(t, ix, ix, ix, ""); !strings.Contains(string(o2), `"y":3`) {
		t.Fatalf("index tenant field lost: %s", o2)
	}
}
