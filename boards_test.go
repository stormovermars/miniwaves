package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func mkBoard(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".miniwaves.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const okIndex = `{"version":9,"name":"W","tenants":[{"slug":"a","name":"A"},{"slug":"b","name":"B","archived":true}]}`

func dirsOf(r boardsResult) []string {
	var o []string
	for _, b := range r.Boards {
		o = append(o, b.Dir)
	}
	return o
}

func eqDirs(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestBoardsDepth(t *testing.T) {
	root := t.TempDir()
	mkBoard(t, root, okIndex)
	mkBoard(t, filepath.Join(root, "a"), okIndex)
	mkBoard(t, filepath.Join(root, "a", "b", "c"), okIndex)
	eqDirs(t, dirsOf(discoverBoards(root, 3, false)), root, filepath.Join(root, "a"), filepath.Join(root, "a", "b", "c"))
	eqDirs(t, dirsOf(discoverBoards(root, 2, false)), root, filepath.Join(root, "a"))
	eqDirs(t, dirsOf(discoverBoards(root, 0, false)), root)
}

func TestBoardsUp(t *testing.T) {
	root := t.TempDir()
	mkBoard(t, root, okIndex)
	sub := filepath.Join(root, "x", "y")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	eqDirs(t, dirsOf(discoverBoards(sub, 3, false)))
	eqDirs(t, dirsOf(discoverBoards(sub, 3, true)), root)
	eqDirs(t, dirsOf(discoverBoards(root, 3, true)), root) // once
}

func TestBoardsSkips(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{".hidden", ".git", "node_modules", "build", "dist", "vendor", ".claude", ".venv"} {
		mkBoard(t, filepath.Join(root, n), okIndex)
	}
	mkBoard(t, filepath.Join(root, "real"), okIndex)
	other := t.TempDir()
	mkBoard(t, other, okIndex)
	if err := os.Symlink(other, filepath.Join(root, "link")); err != nil && runtime.GOOS != "windows" {
		t.Fatal(err)
	}
	eqDirs(t, dirsOf(discoverBoards(root, 3, false)), filepath.Join(root, "real"))
}

func TestBoardsCap(t *testing.T) {
	old := boardsMax
	boardsMax = 2
	defer func() { boardsMax = old }()
	root := t.TempDir()
	for _, n := range []string{"a", "b", "c", "d"} {
		mkBoard(t, filepath.Join(root, n), okIndex)
	}
	r := discoverBoards(root, 2, false)
	if len(r.Boards) != 2 || !r.Truncated || r.Reason == "" {
		t.Fatalf("%+v", r)
	}
}

func TestBoardsTimeLimit(t *testing.T) {
	old := boardsTimeLimit
	boardsTimeLimit = -time.Second
	defer func() { boardsTimeLimit = old }()
	root := t.TempDir()
	mkBoard(t, root, okIndex)
	r := discoverBoards(root, 2, false)
	if !r.Truncated || r.Reason == "" {
		t.Fatalf("%+v", r)
	}
}

func TestBoardsSchema8AndBroken(t *testing.T) {
	root := t.TempDir()
	mkBoard(t, filepath.Join(root, "old"), `{"version":8,"name":"Old","tenants":[{"slug":"x","name":"X","archived":true},{"slug":"y","name":"Y"}]}`)
	mkBoard(t, filepath.Join(root, "bad"), `{not json`)
	r := discoverBoards(root, 2, false)
	if len(r.Boards) != 2 {
		t.Fatalf("%+v", r)
	}
	bad, old := r.Boards[0], r.Boards[1]
	if bad.Error == "" || len(bad.Tenants) != 0 {
		t.Fatalf("%+v", bad)
	}
	if old.Error != "" || old.Schema != 8 || len(old.Tenants) != 2 || !old.Tenants[0].Archived || old.Tenants[1].Archived {
		t.Fatalf("%+v", old)
	}
}

func TestBoardsJSONShape(t *testing.T) {
	root := t.TempDir()
	mkBoard(t, filepath.Join(root, "p"), okIndex)
	b, err := json.Marshal(discoverBoards(root, 1, false))
	if err != nil {
		t.Fatal(err)
	}
	d := filepath.Join(root, "p")
	want, _ := json.Marshal(root)
	wd, _ := json.Marshal(d)
	wp, _ := json.Marshal(filepath.Join(d, ".miniwaves.json"))
	exp := `{"root":` + string(want) + `,"boards":[{"dir":` + string(wd) + `,"path":` + string(wp) +
		`,"name":"W","schema":9,"tenants":[{"slug":"a","name":"A","archived":false},{"slug":"b","name":"B","archived":true}]}],"truncated":false,"reason":""}`
	if string(b) != exp {
		t.Fatalf("got %s\nwant %s", b, exp)
	}
}
