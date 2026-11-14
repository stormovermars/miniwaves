package mw

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func mrg(t *testing.T, b, o, th string) tenantDoc {
	t.Helper()
	out, err := mergeBoardFiles([]byte(b), []byte(o), []byte(th))
	if err != nil {
		t.Fatal(err)
	}
	var td tenantDoc
	if err := json.Unmarshal(out, &td); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return td
}

func item(id, title, status, updated, events string) string {
	return `{"id":"` + id + `","kind":"task","title":"` + title + `","status":"` + status + `","events":[` + events + `],"created":"2026-01-01T00:00:00Z","updated":"` + updated + `"}`
}

func doc(next string, items ...string) string {
	return `{"next":{"TSK":` + next + `},"items":[` + strings.Join(items, ",") + `]}`
}

func ev(at, action, note string) string {
	return `{"at":"` + at + `","action":"` + action + `","note":"` + note + `"}`
}

func TestMergeFieldsAndLogs(t *testing.T) {
	e0 := ev("2026-01-01T00:00:00Z", "created", "")
	base := doc("1", item("TSK-001", "A", "ready", "2026-01-01T00:00:00Z", e0))
	// ours changes the title, theirs changes the status: both survive, logs are a union in time order
	ours := doc("1", item("TSK-001", "A2", "ready", "2026-01-01T01:00:00Z", e0+","+ev("2026-01-01T01:00:00Z", "edit", "title")))
	theirs := doc("1", item("TSK-001", "A", "doing", "2026-01-01T02:00:00Z", e0+","+ev("2026-01-01T02:00:00Z", "doing", "")))
	td := mrg(t, base, ours, theirs)
	if len(td.Items) != 1 {
		t.Fatalf("%+v", td.Items)
	}
	it := td.Items[0]
	if it.Title != "A2" || it.Status != "doing" || len(it.Events) != 3 || it.Events[1].Action != "edit" || it.Events[2].Action != "doing" || it.Updated != "2026-01-01T02:00:00Z" {
		t.Fatalf("%+v", it)
	}
	// real conflict, different seconds: the later writer wins, no merge entry
	o2 := doc("1", item("TSK-001", "OURS", "ready", "2026-01-01T03:00:00Z", e0+","+ev("2026-01-01T03:00:00Z", "edit", "x")))
	t2 := doc("1", item("TSK-001", "THEIRS", "ready", "2026-01-01T04:00:00Z", e0+","+ev("2026-01-01T04:00:00Z", "edit", "y")))
	if it = mrg(t, base, o2, t2).Items[0]; it.Title != "THEIRS" || len(it.Events) != 3 {
		t.Fatalf("%+v", it)
	}
	if it = mrg(t, base, t2, o2).Items[0]; it.Title != "THEIRS" { // the later writer wins whichever side it is on
		t.Fatalf("%+v", it)
	}
	// same second, different nanoseconds: the later nano wins and a merge entry keeps both values
	o3 := doc("1", item("TSK-001", "OURS", "ready", "2026-01-01T05:00:00Z", e0+","+ev("2026-01-01T05:00:00.100000000Z", "edit", "x")))
	t3 := doc("1", item("TSK-001", "THEIRS", "ready", "2026-01-01T05:00:00Z", e0+","+ev("2026-01-01T05:00:00.900000000Z", "edit", "y")))
	it = mrg(t, base, t3, o3).Items[0]
	last := it.Events[len(it.Events)-1]
	if it.Title != "THEIRS" || last.Action != "merge" || !strings.Contains(last.Note, "OURS") || !strings.Contains(last.Note, "THEIRS") {
		t.Fatalf("%+v", it)
	}
}

func TestMergeRowsAndCounters(t *testing.T) {
	e0 := ev("2026-01-01T00:00:00Z", "created", "")
	base := doc("1", item("TSK-001", "A", "done", "2026-01-01T00:00:00Z", e0))
	// both add TSK-002 (different rows), ours also edits A; theirs archived-deleted A untouched? no: A edited on ours, deleted on theirs
	ours := doc("2", item("TSK-001", "A!", "done", "2026-01-02T00:00:00Z", e0), item("TSK-002", "from ours", "ready", "2026-01-02T00:00:00Z", ev("2026-01-02T00:00:00Z", "created", "")))
	theirs := doc("2", item("TSK-002", "from theirs", "ready", "2026-01-02T00:00:01Z", ev("2026-01-02T00:00:01Z", "created", "")))
	td := mrg(t, base, ours, theirs)
	ids := map[string]string{}
	for _, it := range td.Items {
		ids[it.ID] = it.Title
	}
	if len(td.Items) != 3 || ids["TSK-001"] != "A!" || ids["TSK-002"] != "from ours" || ids["TSK-003"] != "from theirs" || td.Next["TSK"] != 3 {
		t.Fatalf("%v next %v", ids, td.Next)
	}
	// deleted on one side, untouched on the other: gone
	td = mrg(t, base, doc("1", item("TSK-001", "A", "done", "2026-01-01T00:00:00Z", e0)), doc("1"))
	if len(td.Items) != 0 {
		t.Fatalf("%+v", td.Items)
	}
	// the same row added on both sides is one row
	add := doc("2", item("TSK-002", "same", "ready", "2026-01-02T00:00:00Z", e0))
	if td = mrg(t, doc("1"), add, add); len(td.Items) != 1 || td.Next["TSK"] != 2 {
		t.Fatalf("%+v", td)
	}
}

func TestMergeIndexAndNodeAndErrors(t *testing.T) {
	b := `{"version":9,"name":"W","tenants":[{"slug":"a","name":"A"}]}`
	o := `{"version":9,"name":"W","tenants":[{"slug":"a","name":"A"},{"slug":"b","name":"B"}]}`
	th := `{"version":9,"name":"W2","tenants":[{"slug":"a","name":"A","node":"beta"},{"slug":"c","name":"C"}]}`
	out, err := mergeBoardFiles([]byte(b), []byte(o), []byte(th))
	if err != nil {
		t.Fatal(err)
	}
	ixOut := out
	var ix indexDoc
	if err := json.Unmarshal(out, &ix); err != nil || ix.Name != "W2" || len(ix.Tenants) != 3 || ix.Tenants[0].Node != "beta" || ix.Tenants[1].Slug != "b" || ix.Tenants[2].Slug != "c" {
		t.Fatalf("%v %s", err, out)
	}
	n1 := `{"node":"alpha","at":"2026-01-01T00:00:00Z","state":"idle","sessions":0,"ready":0,"doing":0,"version":"x"}`
	n2 := strings.Replace(n1, "idle", "working", 1)
	n2 = strings.Replace(n2, "00:00:00Z", "01:00:00Z", 1)
	out, _ = mergeBoardFiles([]byte(n1), []byte(n1), []byte(n2))
	if !strings.Contains(string(out), "working") {
		t.Fatalf("%s", out)
	}
	if _, err := mergeBoardFiles(nil, []byte("<<<<<<< x"), nil); err == nil {
		t.Fatal("garbage accepted")
	}
	if _, err := mergeBoardFiles(nil, []byte(`{"x":1}`), nil); err == nil {
		t.Fatal("unknown shape accepted")
	}
	// idempotent: merging the result with itself changes nothing
	again, _ := mergeBoardFiles([]byte(b), ixOut, ixOut)
	if string(again) != string(ixOut) {
		t.Fatalf("not stable:\n%s\n%s", ixOut, again)
	}
}

func gitOK(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	if runtime.GOOS == "windows" {
		cmd.Env = append(cmd.Env, "GIT_CONFIG_GLOBAL=NUL")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// TestMergeDriverTwoClones plays two computers through real git: each adds and edits rows and logs in its own
// clone, then the second pulls with rebase. The driver is the real mw binary built for the test.
func TestMergeDriverTwoClones(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "mw")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, "github.com/stormovermars/miniwaves/cmd/mw")
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("cannot build mw: %v\n%s", err, out)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("MW_USER_CONFIG", filepath.Join(home, "mw-user.json"))
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })

	origin := filepath.Join(root, "origin.git")
	gitOK(t, root, "init", "--bare", "-b", "main", origin)
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	gitOK(t, root, "clone", origin, a)
	attrs := ".miniwaves.json merge=mw\n.miniwaves/*.json merge=mw\n.miniwaves/nodes/*.json merge=mw\n"
	if err := os.WriteFile(filepath.Join(a, ".gitattributes"), []byte(attrs), 0o644); err != nil {
		t.Fatal(err)
	}
	mwIn := func(dir string, cmds ...[]string) {
		t.Helper()
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		runAll(t, cmds...)
	}
	mwIn(a, []string{"init", "Shared"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"}, []string{"add", "task", "base", "-S", "ready"})
	gitOK(t, a, "add", "-A")
	gitOK(t, a, "commit", "-m", "base")
	gitOK(t, a, "push", "origin", "main")
	gitOK(t, root, "clone", origin, b)
	for _, d := range []string{a, b} {
		gitOK(t, d, "config", "merge.mw.driver", `"`+filepath.ToSlash(bin)+`" merge-file %O %A %B %P`)
	}
	mwIn(a, []string{"add", "task", "from a", "-S", "ready"}, []string{"note", "TSK-001", "note from a"})
	gitOK(t, a, "add", "-A")
	gitOK(t, a, "commit", "-m", "a")
	gitOK(t, a, "push", "origin", "main")
	mwIn(b, []string{"add", "task", "from b", "-S", "ready"}, []string{"start", "TSK-001", "-s", "sb"}, []string{"note", "TSK-001", "note from b"})
	gitOK(t, b, "add", "-A")
	gitOK(t, b, "commit", "-m", "b")
	gitOK(t, b, "pull", "--rebase", "origin", "main")
	if s := gitOK(t, b, "status", "--short"); strings.TrimSpace(s) != "" {
		t.Fatalf("dirty after rebase: %s", s)
	}
	w, err := load(filepath.Join(b, dataFile))
	if err != nil {
		t.Fatal(err)
	}
	titles := map[string]string{}
	for _, it := range w.Tenants[0].Items {
		titles[it.ID] = it.Title + "/" + it.Status
	}
	if len(titles) != 3 || titles["TSK-001"] != "base/doing" || w.Tenants[0].Next["TSK"] != 3 {
		t.Fatalf("%v next %v", titles, w.Tenants[0].Next)
	}
	it, _ := itemByID(&w.Tenants[0], "TSK-001")
	notes := strings.Join(it.Notes, "|")
	if !strings.Contains(notes, "note from a") || !strings.Contains(notes, "note from b") {
		t.Fatalf("notes lost: %v", it.Notes)
	}
	var from []string
	for _, x := range w.Tenants[0].Items {
		from = append(from, x.Title)
	}
	if !strings.Contains(strings.Join(from, "|"), "from a") || !strings.Contains(strings.Join(from, "|"), "from b") {
		t.Fatalf("rows lost: %v", from)
	}
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return append(b, '\n')
}

func TestMergeAuditCases(t *testing.T) {
	e0 := ev("2026-01-01T00:00:00Z", "created", "")
	mk := func(id, title, created string) string {
		return strings.Replace(item(id, title, "ready", "2026-01-02T00:00:00Z", ev(created, "created", "")), "2026-01-01T00:00:00Z\"}", created+"\"}", 1)
	}
	// A's T-005 was renumbered to T-006 on an earlier merge; now A (base T-005 = A's row, doing) meets B's T-005
	baseA := doc("5", strings.Replace(mk("TSK-005", "A row", "2026-01-01T01:00:00Z"), `"id":"TSK-005"`, `"id":"TSK-005"`, 1))
	ours := doc("5", strings.Replace(mk("TSK-005", "A row", "2026-01-01T01:00:00Z"), `"status":"ready"`, `"status":"doing"`, 1))
	theirs := doc("5", mk("TSK-005", "B row", "2026-01-01T02:00:00Z"))
	td := mrg(t, baseA, ours, theirs)
	got := map[string]string{}
	for _, it := range td.Items {
		got[it.ID] = it.Title + "/" + it.Status
	}
	if got["TSK-005"] != "A row/doing" || got["TSK-006"] != "B row/ready" || len(got) != 2 {
		t.Fatalf("corrupted a row: %v", got)
	}
	// the renumbered side's own links follow: a child of B's T-005 added on B
	child := strings.Replace(item("TSK-007", "child of B", "ready", "2026-01-02T00:00:00Z", e0), `"title"`, `"parent":"TSK-005","title"`, 1)
	td = mrg(t, baseA, ours, doc("7", mk("TSK-005", "B row", "2026-01-01T02:00:00Z"), child))
	newID := ""
	for _, it := range td.Items {
		if it.Title == "B row" {
			newID = it.ID
		}
	}
	for _, it := range td.Items {
		if it.ID == "TSK-007" && (newID == "TSK-005" || it.Parent != newID) {
			t.Fatalf("link not rewritten (B row is %s): %+v", newID, it)
		}
	}
	// a cleared evidence list stays cleared when the other side did not touch the row
	withEv := strings.Replace(item("TSK-001", "A", "done", "2026-01-01T00:00:00Z", e0), `"events"`, `"evidence":["e1 3"],"events"`, 1)
	cleared := item("TSK-001", "A", "ready", "2026-01-03T00:00:00Z", e0)
	other := strings.Replace(withEv, `"A"`, `"A2"`, 1)
	if it := mrg(t, doc("1", withEv), doc("1", cleared), doc("1", other)).Items[0]; len(it.Evidence) != 0 {
		t.Fatalf("evidence came back: %+v", it.Evidence)
	}
	// counters add up; empty lists stay lists; merging A,B equals B,A
	cnt := func(n string) string { return `{"next":{},"items":[],"archived":` + n + `}` }
	if td = mrg(t, cnt("2"), cnt("5"), cnt("3")); td.Archived != 6 {
		t.Fatalf("archived %d", td.Archived)
	}
	out, _ := mergeBoardFiles([]byte(cnt("0")), []byte(cnt("0")), []byte(cnt("0")))
	if !strings.Contains(string(out), `"items":[]`) {
		t.Fatalf("%s", out)
	}
	same1 := ev("2026-01-05T00:00:00Z", "ask", "q")
	same2 := ev("2026-01-05T00:00:00Z", "answer", "a")
	b0 := doc("1", item("TSK-001", "A", "ready", "2026-01-01T00:00:00Z", e0))
	sa := doc("1", item("TSK-001", "A", "ready", "2026-01-05T00:00:00Z", e0+","+same1))
	sb := doc("1", item("TSK-001", "A", "ready", "2026-01-05T00:00:00Z", e0+","+same2))
	x, _ := mergeBoardFiles([]byte(b0), []byte(sa), []byte(sb))
	y, _ := mergeBoardFiles([]byte(b0), []byte(sb), []byte(sa))
	if string(x) != string(y) {
		t.Fatalf("order dependent:\n%s\n%s", x, y)
	}
}
