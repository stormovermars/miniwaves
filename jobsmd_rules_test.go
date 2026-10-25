package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func twoJobBoard(t *testing.T) (root string) {
	t.Helper()
	path := inTempDir(t)
	runAll(t, []string{"init", "Two"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"}, []string{"tenant", "add", "b", "B"},
		[]string{"jobs", "enable", "j1", "-t", "two"}, []string{"jobs", "enable", "j2", "-t", "b"})
	return filepath.Dir(path)
}

func jobsMDOf(t *testing.T, root string, id string) (*Workspace, *newsModel) {
	t.Helper()
	w, err := load(filepath.Join(root, dataFile))
	if err != nil {
		t.Fatal(err)
	}
	return w, buildNewsModel(w, newsPart{job: id, title: id, tenants: newsJobTenants(w, id)}, nil, newsWindow{full: true})
}

func TestJobsMDNeverQuietWhileDoing(t *testing.T) {
	root := twoJobBoard(t)
	runAll(t, []string{"add", "task", "work", "-S", "ready", "-t", "two"}, []string{"start", "TSK-001", "-t", "two"})
	_, m := jobsMDOf(t, root, "j1")
	if got := jobsMDSummary("j1", m); strings.Contains(got, "quiet") || !strings.Contains(got, "1 in progress") {
		t.Errorf("busy job: %q", got)
	}
	_, m2 := jobsMDOf(t, root, "j2")
	if got := jobsMDSummary("j2", m2); !strings.Contains(got, "quiet") {
		t.Errorf("idle job: %q", got)
	}
}

func TestJobsMDEventsTitlesAndDone(t *testing.T) {
	root := twoJobBoard(t)
	long := strings.Repeat("word ", 30)
	runAll(t, []string{"add", "task", long, "-S", "ready", "-t", "two"}, []string{"start", "TSK-001", "-t", "two"},
		[]string{"done", "TSK-001", "ok", "--no-evidence", "x", "-t", "two"})
	w, m := jobsMDOf(t, root, "j1")
	out := renderJobsMD(jobsMDInput{id: "j1", title: "j1", m: m, tenants: newsJobTenants(w, "j1")}, time.Now())
	if strings.Contains(out, " created") {
		t.Errorf("created event shown:\n%s", out)
	}
	found := false
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "- ✓ ") && strings.Contains(l, "word") {
			found = true
			if n := len([]rune(strings.SplitN(strings.TrimPrefix(l, "- ✓ **TSK-001** "), " · ", 2)[0])); n > 80 || !strings.Contains(l, "…") {
				t.Errorf("title not capped at 80 with …: %d %q", n, l)
			}
		}
	}
	if !found {
		t.Errorf("the long done title is missing:\n%s", out)
	}
	if !strings.Contains(out, "### Recently done") {
		t.Errorf("done today is missing:\n%s", out)
	}
}

func TestMDRecentDone(t *testing.T) {
	now := time.Date(2026, 10, 25, 15, 0, 0, 0, time.Local)
	mk := func(id string, at time.Time) newsItem {
		return newsItem{ID: id, Since: at.UTC().Format(time.RFC3339Nano)}
	}
	var old, today []newsItem
	for i := 0; i < 8; i++ {
		old = append(old, mk("O"+string(rune('1'+i)), now.Add(-time.Duration(48+i)*time.Hour)))
	}
	if got := mdRecentDone(old, now); len(got) != 5 {
		t.Errorf("nothing today: %d items, want the last 5", len(got))
	}
	today = append([]newsItem{mk("T1", now.Add(-time.Hour)), mk("T2", now.Add(-2*time.Hour))}, old...)
	if got := mdRecentDone(today, now); len(got) != 2 || got[0].ID != "T1" {
		t.Errorf("today: %+v, want just the two done today", got)
	}
}

func TestJobsMDOneFooterPerJobLinks(t *testing.T) {
	root := twoJobBoard(t)
	runAll(t, []string{"add", "task", "a", "-S", "ready", "-t", "two"}, []string{"add", "task", "b", "-S", "ready", "-t", "b"})
	good := filepath.Join(root, "docs", "news", "j1")
	bad := filepath.Join(root, "docs", "news", "j2")
	for d, u := range map[string]string{good: "https://claude.ai/artifact/abc\n", bad: "https://x.test/a b)(\n"} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "jobs.artifact-url"), []byte(u), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runAll(t, []string{"board", "--md", "-o", "all.md"})
	b, err := os.ReadFile(filepath.Join(root, "all.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if n := strings.Count(s, "\n---\n"); n != 1 {
		t.Errorf("%d footers, want 1:\n%s", n, s)
	}
	if !strings.HasSuffix(strings.TrimSpace(s), "[Published page · j1](https://claude.ai/artifact/abc)") && !strings.Contains(s, "[Published page · j1](https://claude.ai/artifact/abc)") {
		t.Errorf("no published link for j1:\n%s", s)
	}
	if strings.Contains(s, "x.test") {
		t.Errorf("an invalid artifact url became a link:\n%s", s)
	}
	for _, j := range []string{"j1", "j2"} { // each job's News link comes from its own news folder
		if !strings.Contains(s, "[News · "+j+"](<docs/news/"+j+"/status.md>)") {
			t.Errorf("news link of %s:\n%s", j, s)
		}
	}
	if strings.Index(s, "\n---\n") < strings.LastIndex(s, "\n## ") {
		t.Errorf("the footer must come after every job:\n%s", s)
	}
}

func TestMDTextEscapesTildeAmp(t *testing.T) {
	if got := mdText("a ~b~ & c"); got != `a \~b\~ \& c` {
		t.Errorf("%q", got)
	}
}

func TestJobsMDValidURL(t *testing.T) {
	for u, want := range map[string]bool{
		"https://claude.ai/artifact/x": true, "http://localhost:5/a": true,
		"javascript:alert(1)": false, "ftp://x/y": false, "https://x/a b": false, "https://x/a)b": false, "https://x/(a": false, "https://x/\x1b": false, "": false,
	} {
		if got := jobsMDValidURL(u); got != want {
			t.Errorf("%q: %v", u, got)
		}
	}
}

func TestFileURLWindowsVolume(t *testing.T) {
	for in, want := range map[string]string{
		`C:\Users\a b\news\jobs.md`: "file:///C:/Users/a%20b/news/jobs.md",
		"C:/Users/x/jobs.md":        "file:///C:/Users/x/jobs.md",
		"/Users/x/jobs.md":          "file:///Users/x/jobs.md",
	} {
		if got := fileURL(in); got != want {
			t.Errorf("%s: %s want %s", in, got, want)
		}
	}
}

func TestStatusTargetUsesJobOfCwd(t *testing.T) {
	root := twoJobBoard(t)
	w, err := load(filepath.Join(root, dataFile))
	if err != nil {
		t.Fatal(err)
	}
	for i := range w.Tenants {
		w.Tenants[i].Path = filepath.Join(root, w.Tenants[i].Slug)
		_ = os.MkdirAll(w.Tenants[i].Path, 0o755)
	}
	if err := save(filepath.Join(root, dataFile), w); err != nil {
		t.Fatal(err)
	}
	w, _ = load(filepath.Join(root, dataFile))
	if got := statusTarget(filepath.Join(root, dataFile), w, filepath.Join(root, "b")); !strings.HasSuffix(got, "/docs/news/j2/jobs.md") {
		t.Errorf("cwd in tenant b (job j2): %s", got)
	}
	if got := statusTarget(filepath.Join(root, dataFile), w, filepath.Join(root, "two")); !strings.HasSuffix(got, "/docs/news/j1/jobs.md") {
		t.Errorf("cwd in tenant two (job j1): %s", got)
	}
}
