package mw

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func buildMw(t *testing.T, root string) string {
	t.Helper()
	bin := filepath.Join(root, "mw")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, "github.com/stormovermars/miniwaves/cmd/mw").CombinedOutput(); err != nil {
		t.Skipf("cannot build mw: %v\n%s", err, out)
	}
	return bin
}

// TestMergeDriverHostile plays the hard offline case through real git: both computers add the same ids, start the same
// row and run their own sessions; one pulls with rebase (git swaps ours and theirs), the same state is then merged the
// other way round and must come out byte for byte the same. Lock files must stay out of git.
func TestMergeDriverHostile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := t.TempDir()
	bin := buildMw(t, root)
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
	ignore := ".miniwaves/*.lock\n.miniwaves/*.lock.pid\n.miniwaves/*.tmp\n.miniwaves/nodes/*.tmp\n.miniwaves.json.lock\n.miniwaves.json.lock.pid\n"
	for f, c := range map[string]string{".gitattributes": attrs, ".gitignore": ignore} {
		if err := os.WriteFile(filepath.Join(a, f), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mwIn := func(dir, node string, cmds ...[]string) {
		t.Helper()
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		runAll(t, append([][]string{{"node", "set", node}}, cmds...)...)
	}
	mwIn(a, "alpha", []string{"init", "Shared"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"}, []string{"add", "task", "base", "-S", "ready"})
	gitOK(t, a, "add", "-A")
	if ls := gitOK(t, a, "ls-files"); strings.Contains(ls, ".lock") || strings.Contains(ls, ".tmp") {
		t.Fatalf("lock or temp files are in git:\n%s", ls)
	}
	gitOK(t, a, "commit", "-m", "base")
	gitOK(t, a, "push", "origin", "main")
	gitOK(t, root, "clone", origin, b)
	for _, d := range []string{a, b} {
		gitOK(t, d, "config", "merge.mw.driver", `"`+filepath.ToSlash(bin)+`" merge-file %O %A %B %P`)
	}
	mwIn(a, "alpha", []string{"start", "TSK-001", "-s", "sa1"}, []string{"add", "task", "alpha X", "-S", "ready"}, []string{"start", "TSK-002", "-s", "sa2"})
	gitOK(t, a, "add", "-A")
	gitOK(t, a, "commit", "-m", "a")
	gitOK(t, a, "push", "origin", "main")
	time.Sleep(1100 * time.Millisecond) // beta is the later writer
	mwIn(b, "beta", []string{"start", "TSK-001", "-s", "sb1"}, []string{"add", "task", "beta Y", "-S", "ready"}, []string{"start", "TSK-002", "-s", "sb2"})
	gitOK(t, b, "add", "-A")
	gitOK(t, b, "commit", "-m", "b")
	gitOK(t, b, "branch", "pre")
	gitOK(t, b, "pull", "--rebase", "origin", "main")
	if s := strings.TrimSpace(gitOK(t, b, "status", "--short")); s != "" {
		t.Fatalf("dirty after rebase: %s", s)
	}
	tenant := filepath.Join(b, ".miniwaves", "shared.json")
	files, _ := filepath.Glob(filepath.Join(b, ".miniwaves", "*.json"))
	if len(files) != 1 {
		t.Fatalf("tenant files: %v", files)
	}
	tenant = files[0]
	rebased, _ := os.ReadFile(tenant)
	var td tenantDoc
	if err := json.Unmarshal(rebased, &td); err != nil {
		t.Fatal(err)
	}
	title := map[string]string{}
	for _, it := range td.Items {
		title[it.ID] = it.Title
	}
	if len(td.Items) != 3 {
		t.Fatalf("rows: %v", title)
	}
	owner := map[string]string{}
	for _, s := range td.Sessions {
		owner[s.ID] = title[s.Item]
	}
	if owner["sa2"] != "alpha X" || owner["sb2"] != "beta Y" {
		t.Fatalf("a session lost its row: %v", owner)
	}
	base, _ := itemByID(&Tenant{Items: td.Items}, "TSK-001")
	last := base.Events[len(base.Events)-1]
	if last.Action != "merge" || !strings.HasPrefix(last.Note, "clash") {
		t.Fatalf("double claim of TSK-001 not logged: %+v", last)
	}
	// the same two states merged the other way round (git merge: ours/theirs swapped) are identical
	gitOK(t, b, "reset", "--hard", "pre")
	gitOK(t, b, "pull", "--no-rebase", "--no-edit", "origin", "main")
	merged, _ := os.ReadFile(tenant)
	if string(merged) != string(rebased) {
		t.Fatalf("rebase and merge differ:\n%s\n%s", rebased, merged)
	}
	t.Logf("rebase and merge of the same two states give the same %d byte tenant file", len(merged))
}

// TestTwoCoordinatorsOneNode: two coordinator sessions on one node watch with fleet --changed while two processes
// beat the same node file in parallel. Each coordinator sees the view once, then "no change"; the node file stays valid.
func TestTwoCoordinatorsOneNode(t *testing.T) {
	root := t.TempDir()
	bin := buildMw(t, root)
	home := t.TempDir()
	board := filepath.Join(root, "board")
	if err := os.MkdirAll(board, 0o755); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "MW_USER_CONFIG="+filepath.Join(home, "mw-user.json"), "MW_SESSION=", "MW_TENANT=")
	mw := func(args ...string) (string, error) {
		c := exec.Command(bin, args...)
		c.Dir, c.Env = board, env
		out, err := c.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	for _, c := range [][]string{{"init", "Duo"}, {"gate", "level", "trust", "--workspace", "--by", "t"}, {"node", "set", "alpha"}, {"add", "task", "A", "-S", "ready"}, {"node", "beat"}} {
		if out, err := mw(c...); err != nil {
			t.Fatalf("%v: %v %s", c, err, out)
		}
	}
	first := map[string]string{}
	start := time.Now()
	for _, s := range []string{"coord-1", "coord-2"} {
		first[s], _ = mw("fleet", "--changed", "-s", s)
		if first[s] == "no change" || !strings.Contains(first[s], "alpha") {
			t.Fatalf("%s first look: %q", s, first[s])
		}
	}
	tb := time.Now()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failed []string
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 15; j++ {
				if out, err := mw("node", "beat"); err != nil {
					mu.Lock()
					failed = append(failed, out)
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	if len(failed) > 0 {
		t.Fatalf("beats failed: %v", failed[0])
	}
	b, err := os.ReadFile(filepath.Join(board, ".miniwaves", "nodes", "alpha.json"))
	var nb nodeBeat
	if err != nil || json.Unmarshal(b, &nb) != nil || nb.Node != "alpha" {
		t.Fatalf("node file damaged: %v %s", err, b)
	}
	beatsTook, t0 := time.Since(tb), time.Now()
	for _, s := range []string{"coord-1", "coord-2"} {
		if out, _ := mw("fleet", "--changed", "-s", s); out != "no change" {
			t.Fatalf("%s after beats: %q", s, out)
		}
	}
	t.Logf("two coordinators, one node: first looks %v, 30 beats from 2 parallel processes %v, the two --changed checks %v; each coordinator saw exactly one view, then 'no change'",
		time.Since(start).Round(time.Millisecond), beatsTook.Round(time.Millisecond), time.Since(t0).Round(time.Millisecond))
}
