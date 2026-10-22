package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// within fails the test when fn does not return in time (a blocked FIFO read would hang forever)
func within(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s hangs", what)
	}
}

func mkfifo(t *testing.T, p string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("no fifo")
	}
	if out, err := exec.Command("mkfifo", p).CombinedOutput(); err != nil {
		t.Skipf("mkfifo: %v %s", err, out)
	}
}

// 1: -o never overwrites a board file
func TestNewsOutRefusesBoardFiles(t *testing.T) {
	path := newsBoard(t)
	root := filepath.Dir(path)
	before := mustRead(t, path)
	targets := []string{".miniwaves.json", filepath.Join(root, ".miniwaves.json"), ".miniwaves/news-app.json", "docs/../.miniwaves.json", ".miniwaves.json.bak8", ".miniwaves/new.json"}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(path, filepath.Join(root, "alias.md")); err != nil {
			t.Fatal(err)
		}
		targets = append(targets, "alias.md")
	}
	for _, tg := range targets {
		for _, f := range []string{"md", "html", "text", "json"} {
			wantErr(t, "mw's own board file", "news", "--job", "tiny", "--format", f, "-o", tg)
		}
	}
	if mustRead(t, path) != before {
		t.Fatal("the index changed")
	}
	if _, err := os.Stat(filepath.Join(root, ".miniwaves", "new.json")); err == nil {
		t.Fatal("wrote into .miniwaves")
	}
	// inside the workspace but not mw's own, and outside the workspace: still fine
	runAll(t, []string{"news", "--job", "tiny", "-o", "docs/out.md"}, []string{"news", "--job", "tiny", "-o", filepath.Join(t.TempDir(), "x.md")})
}

// 2: release reads never block or read without a bound
func TestNewsReleaseReadsAreBounded(t *testing.T) {
	path := newsBoardAt(t, pastBase)
	root := filepath.Dir(path)
	dir := filepath.Join(root, "docs", "news", "releases")
	newsOut(t, "--job", "tiny", "--release")
	// a FIFO named 2.json: drafts warn and fall back, --release refuses naming it, nothing hangs
	mkfifo(t, filepath.Join(dir, "2.json"))
	within(t, "draft with a FIFO release", func() { newsOut(t, "--job", "tiny", "--format", "json") })
	within(t, "release with a FIFO release", func() {
		err := run([]string{"news", "--job", "tiny", "--release", "--force"})
		if err == nil || !strings.Contains(err.Error(), "2.json") {
			t.Errorf("release: %v", err)
		}
	})
	_ = os.Remove(filepath.Join(dir, "2.json"))
	// the latest pointer as a FIFO
	_ = os.Remove(filepath.Join(dir, "latest"))
	mkfifo(t, filepath.Join(dir, "latest"))
	within(t, "FIFO latest", func() {
		if out := newsOut(t, "--job", "tiny", "--release", "--force"); !strings.HasPrefix(out, "released 2") {
			t.Errorf("release with a FIFO latest: %q", out)
		}
	})
	// a /dev/zero symlink as a release file, and as the hash sidecar
	if _, err := os.Stat("/dev/zero"); err == nil {
		_ = os.Remove(filepath.Join(dir, "latest"))
		if err := os.Symlink("/dev/zero", filepath.Join(dir, "9.json")); err == nil {
			within(t, "draft with a /dev/zero release", func() { newsOut(t, "--job", "tiny", "--format", "json") })
			_ = os.Remove(filepath.Join(dir, "9.json"))
		}
		page := filepath.Join(root, "docs", "news", "status.md")
		_ = os.Remove(page + ".hash")
		if err := os.Symlink("/dev/zero", page+".hash"); err == nil {
			within(t, "/dev/zero hash sidecar", func() { newsOut(t) })
		}
	}
	// an oversized release file is refused, not slurped
	big := filepath.Join(dir, "7.json")
	if err := os.WriteFile(big, []byte(strings.Repeat(" ", newsModelMax+10)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readRegular(big, newsModelMax); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("oversized: %v", err)
	}
	mkfifo(t, filepath.Join(root, "sidecar.hash"))
	within(t, "FIFO hash sidecar", func() { _, _ = readRegular(filepath.Join(root, "sidecar.hash"), 4096) })
}

// 3: mw review -j always has review, and keeps the items in blocked
func TestReviewJSONKeepsBlocked(t *testing.T) {
	newsBoard(t)
	out := captureOutput(t, func() { runAll(t, []string{"review", "-j", "-t", "doc"}) })
	if !strings.Contains(out, `"review": []`) {
		t.Fatalf("review missing when empty:\n%s", out)
	}
	runAll(t, []string{"add", "task", "Needs eyes", "-S", "ready", "-t", "doc"}, []string{"start", "TSK-002", "-s", "x", "-t", "doc"}, []string{"block", "TSK-002", "awaiting review: ready for a look", "-t", "doc"})
	out = captureOutput(t, func() { runAll(t, []string{"review", "-j", "-t", "doc"}) })
	if strings.Count(out, `"id": "TSK-002"`) != 2 || !strings.Contains(out, `"review": [`) || strings.Contains(out, `"review": []`) {
		t.Fatalf("the item should be in review and in blocked:\n%s", out)
	}
	if txt := captureOutput(t, func() { runAll(t, []string{"review", "-t", "doc"}) }); !strings.Contains(txt, "review (1)") || !strings.Contains(txt, "blocked (") {
		t.Fatalf("text:\n%s", txt)
	}
}

// 4: only mw's own release files raise the number
func TestReleaseNumberIgnoresStrayFiles(t *testing.T) {
	path := newsBoardAt(t, pastBase)
	dir := filepath.Join(filepath.Dir(path), "docs", "news", "releases")
	newsOut(t, "--job", "tiny", "--release")
	for _, f := range []string{"2026.notes.txt", "2026.json.bak", "2026.md.old", "99.txt", "2027.html.orig"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if out := newsOut(t, "--job", "tiny", "--release", "--force"); !strings.HasPrefix(out, "released 2") {
		t.Fatalf("stray files raised the number: %q", out)
	}
	// a leftover temp file of mw's own does count
	_ = os.WriteFile(filepath.Join(dir, "5.json.123.tmp"), []byte("x"), 0o644)
	if out := newsOut(t, "--job", "tiny", "--release", "--force"); !strings.HasPrefix(out, "released 6") {
		t.Fatalf("tmp file: %q", out)
	}
}

// 5: durable, exclusive writes, with the no-hard-link fallback
func TestWriteNewFileExclusive(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "1.json")
	if err := writeNewFile(p, []byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := writeNewFile(p, []byte("b")); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("overwrite: %v", err)
	}
	q := filepath.Join(d, "2.json")
	if err := writeExclusive(q, []byte("c")); err != nil || mustRead(t, q) != "c" {
		t.Fatalf("fallback: %v", err)
	}
	if err := writeExclusive(q, []byte("d")); err == nil || mustRead(t, q) != "c" {
		t.Fatalf("fallback overwrite: %v", err)
	}
	if ents, _ := os.ReadDir(d); len(ents) != 2 {
		t.Fatalf("temp files left: %v", ents)
	}
}

// 6, 7
func TestReleaseRefusesOutAndEditTicketFlag(t *testing.T) {
	newsBoard(t)
	wantErr(t, "takes no -o", "news", "--job", "tiny", "--release", "-o", "x.md")
	runAll(t, []string{"jobs", "set", "tiny", "tracker=jira", "tracker_url=https://acme.atlassian.net"})
	runAll(t, []string{"edit", "TSK-002", "--ticket", "mob-7", "-t", "news-app"})
	if out := captureOutput(t, func() { runAll(t, []string{"show", "TSK-002", "-j", "-t", "news-app"}) }); !strings.Contains(out, `"ticket": "MOB-7"`) {
		t.Fatalf("--ticket:\n%s", out)
	}
	runAll(t, []string{"edit", "TSK-002", "--ticket", "", "-t", "news-app"})
	if out := captureOutput(t, func() { runAll(t, []string{"show", "TSK-002", "-j", "-t", "news-app"}) }); strings.Contains(out, `"ticket"`) {
		t.Fatalf("--ticket '' did not clear:\n%s", out)
	}
	wantErr(t, "not a valid jira ticket", "edit", "TSK-002", "--ticket", "nope", "-t", "news-app")
}

// 8
func TestUrlsSafeNeedsBoth(t *testing.T) {
	ok := []newsItem{{TicketURL: "https://acme.atlassian.net/browse/MOB-1"}}
	if !urlsSafe(ok) {
		t.Fatal("a good url refused")
	}
	for _, u := range []string{"https://e.com/x?y=1", "https://e.com/a b", "http://e.com/x", "https://e.com/x#f", "javascript:alert(1)"} {
		if urlsSafe([]newsItem{{TicketURL: u}}) {
			t.Errorf("%q accepted", u)
		}
	}
}

// 9 and 10
func TestBOMAndConfinedOpen(t *testing.T) {
	path := newsBoard(t)
	root := filepath.Dir(path)
	bom := "\xef\xbb\xbf"
	_ = os.WriteFile(filepath.Join(root, "summary.txt"), []byte(bom+"Harness: 3 pass\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "m.json"), []byte(bom+metricsJSON(`{"title":"s","kind":"progress","rows":[{"name":"a","done":1,"total":2}]}`)), 0o644)
	runAll(t, []string{"jobs", "set", "tiny", "metrics=m.json"})
	out := newsOut(t, "--job", "tiny", "--format", "json")
	m := decodeModel(t, out)
	if m.Metrics == nil || m.Metrics.Error != "" || len(m.Metrics.Sections) != 1 || strings.Contains(m.Summary, "\xef\xbb\xbf") || !strings.HasPrefix(m.Summary, "Harness") {
		t.Fatalf("BOM: metrics %+v summary %q", m.Metrics, m.Summary)
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink("summary.txt", filepath.Join(root, "inside.txt")); err != nil {
			t.Fatal(err)
		}
		f, err := openConfined(root, "summary", "inside.txt")
		if err != nil {
			t.Fatalf("a symlink inside the workspace: %v", err)
		}
		f.Close()
		mkfifo(t, filepath.Join(root, "pipe.txt"))
		within(t, "openConfined on a FIFO", func() {
			if _, err := openConfined(root, "summary", "pipe.txt"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
				t.Errorf("fifo: %v", err)
			}
		})
	}
}
