package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func out(t *testing.T, args ...string) string {
	t.Helper()
	return captureOutput(t, func() {
		if err := run(args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	})
}

func TestTenantArchiveRestore(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "App"}, []string{"tenant", "add", "old", "Old app"},
		[]string{"add", "task", "Legacy job", "-t", "old", "-S", "ready"})
	runAll(t, []string{"tenant", "archive", "old"}, []string{"tenant", "archive", "old"}) // idempotent
	for _, args := range [][]string{{"board", "-A"}, {"list", "-A"}, {"log", "-A"}, {"tenant", "list"}, {"version", "-j"}} {
		if s := out(t, args...); strings.Contains(s, "Legacy job") || strings.Contains(s, "old,") || args[0] == "board" && strings.Contains(s, "[old]") {
			t.Errorf("%v shows the archived tenant:\n%s", args, s)
		}
	}
	if s := out(t, "version", "-j"); !strings.Contains(s, `"archived": true`) && !strings.Contains(s, `"archived":true`) {
		// runners get the flag so they can skip it and count it
		if strings.Contains(s, `"old"`) {
			t.Errorf("version -j lists old without archived: %s", s)
		}
	}
	if s := out(t, "tenant", "list", "-v"); !strings.Contains(s, "old") || !strings.Contains(s, "archived") {
		t.Errorf("tenant list -v hides archived: %s", s)
	}
	if err := run([]string{"list", "-t", "old"}); err == nil || err.Error() != "old is archived: mw tenant restore old" {
		t.Fatalf("explicit -t on archived: %v", err)
	}
	if s := renderHTML(mustLoad(t, path)); strings.Contains(s, "Old app") {
		t.Error("page shows the archived tenant")
	}
	w := mustLoad(t, path)
	w.showArchived = true
	if s := renderHTML(w); !strings.Contains(s, "<section class=archived><h2>Old app") {
		t.Error("--archived page must show it greyed")
	}
	runAll(t, []string{"tenant", "restore", "old"}, []string{"tenant", "restore", "old"})
	if s := out(t, "list", "-A"); !strings.Contains(s, "Legacy job") {
		t.Errorf("restored tenant hidden:\n%s", s)
	}
	if err := run([]string{"tenant", "archive", "nope"}); err == nil {
		t.Fatal("unknown slug must fail")
	}
	if b, _ := os.ReadFile(filepath.Join(filepath.Dir(path), ".miniwaves", "old.json")); !strings.Contains(string(b), "Legacy job") {
		t.Fatal("tenant file touched")
	}
}

func TestTenantTags(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "App"}, []string{"tenant", "add", "droid", "Droid"}, []string{"tenant", "add", "web", "Web"},
		[]string{"add", "task", "Droid job", "-t", "droid"}, []string{"add", "task", "Web job", "-t", "web"})
	if s := out(t, "tenant", "tag", "droid", "+Store", "+android", "+store"); s != "droid tags: android store\n" {
		t.Fatalf("tag add: %q", s)
	}
	if s := out(t, "tenant", "tag", "droid", "-store", "-missing", "+old"); s != "droid tags: android old\n" {
		t.Fatalf("tag remove: %q", s)
	}
	runAll(t, []string{"tenant", "tag", "web", "+old"})
	if err := run([]string{"tenant", "tag", "web", "plain"}); err == nil {
		t.Fatal("a tag without +/- must fail")
	}
	if s := out(t, "list", "-A", "--tag", "android"); !strings.Contains(s, "Droid job") || strings.Contains(s, "Web job") {
		t.Errorf("list -A --tag:\n%s", s)
	}
	if s := out(t, "board", "-A", "--tag", "old", "--tag", "android"); !strings.Contains(s, "[droid] #android #old") || strings.Contains(s, "[web]") {
		t.Errorf("board -A --tag AND:\n%s", s)
	}
	if s := out(t, "tenant", "list", "-j", "--tag", "old"); !strings.Contains(s, `"tags": [`) || !strings.Contains(s, "web") {
		t.Errorf("tenant list -j:\n%s", s)
	}
	if s := out(t, "version", "-j"); !strings.Contains(s, `"android"`) {
		t.Errorf("version -j tags: %s", s)
	}
	if s := renderHTML(mustLoad(t, path)); !strings.Contains(s, "[droid]</span> <span class=tag>android</span> <span class=tag>old</span>") {
		t.Error("page lacks tag chips")
	}
}

func mustLoad(t *testing.T, path string) *Workspace {
	t.Helper()
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestTenantArchiveRefusesLiveWork(t *testing.T) {
	inTempDir(t)
	runAll(t, []string{"init", "App"}, []string{"tenant", "add", "old", "Old app"},
		[]string{"add", "task", "Busy job", "-t", "old", "-S", "ready"}, []string{"start", "TSK-001", "-t", "old"})
	err := run([]string{"tenant", "archive", "old"})
	if err == nil || !strings.Contains(err.Error(), "TSK-001") || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("want refusal naming the item: %v", err)
	}
	if s := out(t, "tenant", "list", "-v"); strings.Contains(s, "archived") {
		t.Fatalf("tenant archived despite refusal: %s", s)
	}
	runAll(t, []string{"tenant", "archive", "old", "--force"})
	if s := out(t, "tenant", "list", "-v"); !strings.Contains(s, "archived") {
		t.Fatalf("--force did not archive: %s", s)
	}
}

func TestArchivedTenantLeftOutOfAllViews(t *testing.T) {
	inTempDir(t)
	runAll(t, []string{"init", "App"}, []string{"tenant", "add", "old", "Old app"},
		[]string{"wave", "create", "Hidden wave", "-t", "old"},
		[]string{"add", "task", "Legacy job", "-t", "old", "-S", "ready"},
		[]string{"done", "TSK-001", "ok", "--no-evidence", "t", "-t", "old"},
		[]string{"tenant", "archive", "old"})
	if s := out(t, "wave", "list", "-A"); strings.Contains(s, "Hidden wave") {
		t.Errorf("wave list -A shows archived tenant:\n%s", s)
	}
	if s := out(t, "archive", "--days", "0", "--dry-run"); !strings.Contains(s, "nothing to archive") {
		t.Errorf("archive -A touches archived tenant:\n%s", s)
	}
}
