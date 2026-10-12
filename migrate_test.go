package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var testdataDir, _ = filepath.Abs("testdata")

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(testdataDir, "migrate", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func canon(t *testing.T, w *Workspace) string {
	t.Helper()
	b, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMigrateMatchesHandRecipe(t *testing.T) {
	for _, c := range []struct {
		old, hand string
		from      int
	}{{"v5.json", "v5-by-hand.json", 5}, {"v6.json", "v6-by-hand.json", 6}} {
		path := inTempDir(t)
		if err := os.WriteFile(path, fixture(t, c.old), 0o600); err != nil {
			t.Fatal(err)
		}
		wantErr(t, "run mw migrate", "board")
		before, _ := os.ReadFile(path)
		runAll(t, []string{"migrate", "--dry-run"})
		if after, _ := os.ReadFile(path); string(after) != string(before) {
			t.Fatal("dry run changed the file")
		}
		if _, err := os.Stat(path + ".bak" + string(rune('0'+c.from))); err == nil {
			t.Fatal("dry run wrote a backup")
		}
		runAll(t, []string{"migrate"})
		bak, err := os.ReadFile(path + ".bak" + string(rune('0'+c.from)))
		if err != nil || string(bak) != string(before) {
			t.Fatalf("backup: %v", err)
		}
		hw, _, _, err := migrateBytes(fixture(t, c.hand), "hand.json") // the by-hand recipe result, read as schema 7
		if err != nil {
			t.Fatal(err)
		}
		if got, want := canon(t, loadT(t, path)), canon(t, hw); got != want {
			t.Fatalf("%s: mw migrate and the hand recipe differ\n got %s\nwant %s", c.old, got, want)
		}
		runAll(t, []string{"board"}, []string{"log"})
		out := captureOutput(t, func() { runAll(t, []string{"migrate"}) })
		if !strings.Contains(out, "already schema 8") {
			t.Fatal(out)
		}
	}
}

func TestOldAndNewerBoardsAreRefused(t *testing.T) {
	path := inTempDir(t)
	os.WriteFile(path, fixture(t, "v5.json"), 0o600)
	for _, args := range [][]string{{"board"}, {"add", "task", "x"}, {"log"}} {
		wantErr(t, "is schema 5 and this mw only reads schema 8: run mw migrate", args...)
	}
	os.WriteFile(path, []byte(`{"version":99,"name":"N","tenants":[{"slug":"n","name":"N"}]}`), 0o600)
	wantErr(t, "upgrade mw", "board")
	wantErr(t, "upgrade mw", "migrate")
}
