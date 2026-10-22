package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var pastBase = time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)

func writeMetricsFile(t *testing.T, root, doc string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "m.json"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
}

// H1: huge metric values are refused, and deltas at the 1e12 bound still encode
func TestNewsHugeMetricsNeverBreakARelease(t *testing.T) {
	path := newsBoardAt(t, pastBase)
	root := filepath.Dir(path)
	runAll(t, []string{"jobs", "set", "tiny", "metrics=m.json"})
	doc := func(v string) string {
		return `{"version":1,"sections":[{"title":"Q","kind":"value","rows":[{"name":"v","value":` + v + `}]}]}`
	}
	writeMetricsFile(t, root, doc("1e300"))
	m := decodeModel(t, newsOut(t, "--job", "tiny", "--format", "json"))
	if m.Metrics == nil || !strings.Contains(m.Metrics.Error, "1e12") {
		t.Fatalf("1e300 accepted: %+v", m.Metrics)
	}
	dir := filepath.Join(root, "docs", "news", "releases")
	writeMetricsFile(t, root, doc("1e12"))
	newsOut(t, "--job", "tiny", "--release")
	writeMetricsFile(t, root, doc("-1e12"))
	if out := newsOut(t, "--job", "tiny", "--release"); !strings.HasPrefix(out, "released 2") {
		t.Fatalf("release 2: %q", out)
	}
	r2 := decodeModel(t, mustRead(t, filepath.Join(dir, "2.json")))
	if d := r2.Metrics.Sections[0].Rows[0].Delta["value"]; d != -2e12 {
		t.Fatalf("delta %v", d)
	}
	// a delta that would not be finite is dropped, never encoded
	cur := &newsMetrics{Sections: []newsMSection{{Title: "Q", Kind: "value", Rows: []newsMRow{{Name: "v", Value: ptr(1e308)}}}}}
	prev := &newsMetrics{Sections: []newsMSection{{Title: "Q", Kind: "value", Rows: []newsMRow{{Name: "v", Value: ptr(-1e308)}}}}}
	metricsDeltas(cur, prev)
	if cur.Sections[0].Rows[0].Delta != nil {
		t.Fatal("an infinite delta was kept")
	}
	if _, err := json.Marshal(cur); err != nil {
		t.Fatal(err)
	}
}

func ptr(f float64) *float64 { return &f }

// H2: a corrupt release file never blocks a draft; it blocks only --release, and only for its own job
func TestNewsCorruptArchive(t *testing.T) {
	path := newsBoardAt(t, pastBase)
	root := filepath.Dir(path)
	newsOut(t, "--job", "tiny", "--release")
	runAll(t, []string{"add", "task", "More", "-S", "ready", "-t", "news-app"})
	newsOut(t, "--job", "tiny", "--release")
	dir := filepath.Join(root, "docs", "news", "releases")
	if err := os.WriteFile(filepath.Join(dir, "2.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	// a second job with its own news.out
	runAll(t, []string{"tenant", "add", "other", "Other"}, []string{"jobs", "enable", "second", "-t", "other"}, []string{"jobs", "set", "second", "news.out=docs/news/second/"})
	if out := newsOut(t, "--job", "tiny", "--format", "json"); decodeModel(t, out).Release.Previous == nil || *decodeModel(t, out).Release.Previous != 1 {
		t.Fatalf("draft did not fall back to release 1:\n%s", out)
	}
	newsOut(t) // every job: neither is blocked
	for _, f := range []string{filepath.Join(root, "docs", "news", "status.md"), filepath.Join(root, "docs", "news", "second", "status.md")} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("job page not written: %v", err)
		}
	}
	err := run([]string{"news", "--job", "tiny", "--release", "--force"})
	if err == nil || !strings.Contains(err.Error(), "2.json") {
		t.Fatalf("release with a corrupt archive: %v", err)
	}
	if out := newsOut(t, "--job", "second", "--release", "--force"); !strings.HasPrefix(out, "released 1") {
		t.Fatalf("the other job: %q", out)
	}
	// the corrupt file still holds its number
	_ = os.Remove(filepath.Join(dir, "2.json"))
	if out := newsOut(t, "--job", "tiny", "--release", "--force"); !strings.HasPrefix(out, "released 3") {
		t.Fatalf("number reuse: %q", out)
	}
}

// M1: numbers never reuse a stale N.* file, and a failed run leaves nothing behind
func TestNewsReleaseNumbersAndCleanup(t *testing.T) {
	path := newsBoardAt(t, pastBase)
	dir := filepath.Join(filepath.Dir(path), "docs", "news", "releases")
	newsOut(t, "--job", "tiny", "--release")
	if err := os.Remove(filepath.Join(dir, "1.json")); err != nil {
		t.Fatal(err)
	}
	if out := newsOut(t, "--job", "tiny", "--release", "--force"); !strings.HasPrefix(out, "released 2") {
		t.Fatalf("stale 1.md: %q", out)
	}
	// a model that cannot be encoded writes nothing
	nan := math.NaN()
	bad := &newsModel{Version: 1, Job: newsJobInfo{ID: "tiny"}, Metrics: &newsMetrics{Sections: []newsMSection{{Title: "Q", Kind: "value", Rows: []newsMRow{{Name: "v", Value: &nan}}}}}}
	bad.Release.Changes = 1
	if err := newsCutRelease(map[string]string{}, "md", bad, dir, nil, nil, 3, true); err == nil || !strings.Contains(err.Error(), "cannot be encoded") {
		t.Fatalf("NaN model: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "3.md")); err == nil {
		t.Fatal("3.md written for an unencodable model")
	}
	// a lost race (3.json already exists) removes this run's other files again
	if err := os.WriteFile(filepath.Join(dir, "3.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	good := &newsModel{Version: 1, Job: newsJobInfo{ID: "tiny"}}
	if err := newsCutRelease(map[string]string{}, "md", good, dir, nil, nil, 3, true); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("lost race: %v", err)
	}
	for _, f := range []string{"3.md", "3.html"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			t.Errorf("%s left behind", f)
		}
	}
	if b := mustRead(t, filepath.Join(dir, "latest")); strings.TrimSpace(b) != "2" {
		t.Fatalf("latest moved: %q", b)
	}
	_ = os.Remove(filepath.Join(dir, "3.json"))
	// L8: 18 digits count, 19 do not
	_ = os.WriteFile(filepath.Join(dir, "123456789012345678.json"), []byte("{}"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "1234567890123456789.json"), []byte("{}"), 0o644)
	nums := newsReleaseNumbers(dir)
	if nums[0] != 123456789012345678 {
		t.Fatalf("numbers: %v", nums)
	}
	for _, n := range nums {
		if n == 0 {
			t.Fatal("overflowed number listed")
		}
	}
}

// M2: mw's own files are never a news output
func TestNewsRefusesBoardFiles(t *testing.T) {
	path := newsBoard(t)
	for _, p := range []string{".miniwaves/x.json", ".miniwaves.json", ".miniwaves.json.bak8", ".miniwaves.lock", ".MiniWaves/a"} {
		for _, key := range []string{"news_releases", "metrics", "summary", "news.out"} {
			wantErr(t, "mw's own board file", "jobs", "set", "tiny", key+"="+p)
		}
	}
	// a hand-edited index is caught when it is used
	w, _ := load(path)
	c := w.Jobs["tiny"]
	c.Metrics = ".miniwaves/news-app.json"
	w.Jobs["tiny"] = c
	_ = save(path, w)
	m := decodeModel(t, newsOut(t, "--job", "tiny", "--format", "json"))
	if m.Metrics == nil || !strings.Contains(m.Metrics.Error, "board file") {
		t.Fatalf("metrics: %+v", m.Metrics)
	}
	c.NewsReleases = ".miniwaves/rel/"
	w.Jobs["tiny"] = c
	_ = save(path, w)
	if err := run([]string{"news", "--job", "tiny", "--format", "json"}); err == nil || !strings.Contains(err.Error(), "board file") {
		t.Fatalf("draft with a releases folder in .miniwaves: %v", err)
	}
	if err := run([]string{"news", "--job", "tiny", "--release", "--force"}); err == nil || !strings.Contains(err.Error(), "board file") {
		t.Fatalf("release into .miniwaves: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), ".miniwaves", "rel")); err == nil {
		t.Fatal("wrote into .miniwaves")
	}
}

// M3: only what the newsletter reports makes a release non-empty
func TestNewsEmptyReleaseIgnoresEdits(t *testing.T) {
	newsBoardAt(t, pastBase)
	runAll(t, []string{"jobs", "set", "tiny", "tracker=github", "tracker_url=https://github.com/a/b"})
	newsOut(t, "--job", "tiny", "--release")
	runAll(t, []string{"edit", "TSK-002", "ticket=12", "-t", "news-app"})
	if out := newsOut(t, "--job", "tiny", "--release"); strings.TrimSpace(out) != "nothing new since release 1" {
		t.Fatalf("ticket edit: %q", out)
	}
	runAll(t, []string{"note", "TSK-002", "just a note", "-t", "news-app"}, []string{"edit", "TSK-002", "-b", "body", "-t", "news-app"})
	if out := newsOut(t, "--job", "tiny", "--release"); strings.TrimSpace(out) != "nothing new since release 1" {
		t.Fatalf("note/edit: %q", out)
	}
	d := decodeModel(t, newsOut(t, "--job", "tiny", "--format", "json"))
	if d.Release.Changes != 0 {
		t.Fatalf("changes: %d", d.Release.Changes)
	}
	runAll(t, []string{"start", "TSK-002", "-s", "x", "-t", "news-app"}, []string{"done", "TSK-002", "ok", "--no-evidence", "none", "-t", "news-app"})
	if out := newsOut(t, "--job", "tiny", "--release"); !strings.HasPrefix(out, "released 2") {
		t.Fatalf("a done item: %q", out)
	}
}

// L1: mw review and mw news classify blocked items the same way (on the stress board)
func TestNewsAndReviewAgree(t *testing.T) {
	path := newsStress(t)
	w, _ := load(path)
	judge, review, blocked := 0, 0, 0
	for _, tn := range w.Tenants {
		d, _ := reviewOf(w, &tn)
		judge, review, blocked = judge+len(d.Judge), review+len(d.Review), blocked+len(d.Blocked)
		for _, b := range d.Blocked {
			if strings.HasPrefix(b.ID, "REV") {
				t.Fatal("review item left in blocked")
			}
		}
	}
	m := decodeModel(t, bigJSON(t))
	if m.Now.Counts.Review != judge+review || m.Now.Counts.Blocked != blocked || review != 100 || blocked != 100 {
		t.Fatalf("news review %d blocked %d; review cmd judge %d review %d blocked %d", m.Now.Counts.Review, m.Now.Counts.Blocked, judge, review, blocked)
	}
}

// L2, L3: input hardening
func TestTrackerInputHardening(t *testing.T) {
	for kind, in := range map[string]string{"jira": "ınk-1", "shortcut": "ſc-1", "github": "１２３", "azure": "ab#1ı"} {
		if _, err := canonTicket(kind, in); err == nil {
			t.Errorf("%s accepted %q", kind, in)
		}
	}
	for _, bad := range []string{"https://a.com/x/../y", "https://a.com/./x", "https://a.com/%2e%2e/x", "https://a.com/%2E/x", "https://a.com/a%2Fb", "https://a.com/a%5Cb", "https://a.com/.."} {
		if _, err := validTrackerURL(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if u, err := validTrackerURL("https://dev.azure.com/acme/App/"); err != nil || u != "https://dev.azure.com/acme/App" {
		t.Fatalf("%q %v", u, err)
	}
}

// L4
func TestMetricsTrailingJunkAndDuplicates(t *testing.T) {
	ok := metricsJSON(`{"title":"s","kind":"progress","rows":[{"name":"a","done":1,"total":2}]}`)
	if m := parseMetrics([]byte(ok)); m.Error != "" {
		t.Fatal(m.Error)
	}
	for name, doc := range map[string]string{"trailing": ok + `{"x":1}`, "trailing text": ok + " junk"} {
		if m := parseMetrics([]byte(doc)); m.Error == "" || !strings.Contains(m.Error, "after") && name == "trailing" {
			t.Errorf("%s: %+v", name, m)
		}
	}
	dup := metricsJSON(`{"title":"s","kind":"progress","rows":[{"name":"a","done":1,"total":2},{"name":"a ","done":1,"total":2}]}`)
	if m := parseMetrics([]byte(dup)); !strings.Contains(m.Error, "duplicate row name") {
		t.Fatalf("duplicate: %+v", m)
	}
	// the same name in two sections is fine
	two := metricsJSON(`{"title":"s","kind":"progress","rows":[{"name":"a","done":1,"total":2}]},{"title":"t","kind":"progress","rows":[{"name":"a","done":1,"total":2}]}`)
	if m := parseMetrics([]byte(two)); m.Error != "" {
		t.Fatal(m.Error)
	}
}

// L5 and L7
func TestNewsDescriptionLine(t *testing.T) {
	newsBoard(t)
	runAll(t, []string{"jobs", "set", "tiny", "description=Harness <b>|x</b> `tick` " + strings.Repeat("long ", 14)})
	txt := newsOut(t, "--job", "tiny", "--format", "text")
	lines := strings.Split(txt, "\n")
	if !strings.HasPrefix(lines[2], " Harness <b>|x</b> `tick` long") || !strings.HasSuffix(lines[2], "~") || newsStrWidth(lines[2]) > 67 {
		t.Fatalf("description line: %q", lines[2])
	}
	h := newsOut(t, "--job", "tiny", "--format", "html", "-o", "-")
	if strings.Contains(h, "<b>|x</b>") || !strings.Contains(h, "Harness &lt;b&gt;|x&lt;/b&gt;") {
		t.Fatalf("html description not escaped:\n%s", h)
	}
	md := newsOut(t, "--job", "tiny", "-o", "-")
	if !strings.Contains(md, "\n Harness <b>|x</b>") {
		t.Fatal("md lacks the description inside the block")
	}
	if w, l := maxWidth(txt); w > 70 {
		t.Fatalf("%d cells: %q", w, l)
	}
	out := captureOutput(t, func() { runAll(t, []string{"jobs", "show", "tiny", "-j"}) })
	if !strings.Contains(out, `"news_releases": "docs/news/releases"`) {
		t.Fatalf("jobs show lacks the effective releases folder:\n%s", out)
	}
}
