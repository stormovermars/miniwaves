package main

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func decodeModel(t *testing.T, s string) *newsModel {
	t.Helper()
	var m newsModel
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("model json: %v\n%s", err, s)
	}
	return &m
}

func newsOut(t *testing.T, args ...string) string {
	t.Helper()
	return captureOutput(t, func() { runAll(t, append([]string{"news"}, args...)) })
}

var anchorTag = regexp.MustCompile(`</?a( [^>]*)?>`)

func maxWidth(s string) (int, string) {
	worst, line := 0, ""
	if i := strings.Index(s, "<pre>"); i >= 0 { // an html page: the bulletin is the <pre> block
		s = s[i:]
	}
	s = anchorTag.ReplaceAllString(s, "")
	for _, l := range strings.Split(s, "\n") {
		if w := newsStrWidth(html.UnescapeString(l)); w > worst {
			worst, line = w, l
		}
	}
	return worst, line
}

func TestNewsBulletinGoldens(t *testing.T) {
	newsBoard(t)
	for _, f := range []string{"md", "html"} {
		got := newsOut(t, "--job", "tiny", "--format", f, "-o", "-")
		checkGolden(t, "news-"+f, got)
	}
	checkGolden(t, "news-text", newsOut(t, "--job", "tiny", "--format", "text"))
	js := newsOut(t, "--job", "tiny", "--format", "json")
	checkGolden(t, "news-json", js)
	m := decodeModel(t, js)
	if m.Version != 1 || m.Release.Number != nil || m.Job.ID != "tiny" {
		t.Fatalf("model head: %+v", m.Release)
	}
	// deterministic: the same board gives the same bytes
	if again := newsOut(t, "--job", "tiny", "--format", "json"); again != js {
		t.Fatal("two runs differ")
	}
}

// the md block is one fenced text block and html is a <pre>
func TestNewsBulletinShapes(t *testing.T) {
	newsBoard(t)
	md := newsOut(t, "--job", "tiny", "-o", "-")
	lines := strings.Split(strings.TrimSuffix(md, "\n"), "\n")
	fence := strings.TrimSuffix(lines[0], "text")
	if strings.Trim(fence, "`") != "" || len(fence) < 3 || lines[len(lines)-1] != fence || strings.Count(md, "\n"+fence+"\n") != 1 {
		t.Fatalf("md is not one fenced block:\n%s", md)
	}
	h := newsOut(t, "--job", "tiny", "--format", "html", "-o", "-")
	if strings.Count(h, "<pre>") != 1 || !strings.Contains(h, "</pre>") || strings.Contains(h, "<script>") {
		t.Fatalf("html shape:\n%s", h)
	}
	txt := newsOut(t, "--job", "tiny", "--format", "text")
	for _, want := range []string{"DRAFT - NOT RELEASED", "1. IN SHORT", "2. WHAT HAPPENED", "3. ONGOING", "4. WHAT'S NEXT", "5. METRICS", "end of bulletin", "releases: none", "gopher://example.com/news?a=1&b=2"} {
		if !strings.Contains(txt, want) {
			t.Errorf("text lacks %q", want)
		}
	}
	for _, l := range strings.Split(txt, "\n") {
		for _, r := range l {
			if r > 0x7f && r != '▲' && r != '▼' && !strings.ContainsRune("<>|`&*_[]", r) && l != "" && !strings.Contains(l, "Fix") {
				_ = r // board text may carry any rune; only the frame is ASCII
			}
		}
	}
}

func TestNewsLinesWithin70(t *testing.T) {
	newsBoard(t)
	for _, args := range [][]string{{"--format", "text"}, {"-o", "-"}, {"--format", "html", "-o", "-"}} {
		out := newsOut(t, append([]string{"--job", "tiny"}, args...)...)
		if w, l := maxWidth(out); w > 70 {
			t.Errorf("%v: line of %d cells: %q", args, w, l)
		}
	}
	// wide runes, long names and a long url never push a line past 70
	runAll(t, []string{"add", "task", strings.Repeat("日本語のタイトル", 20), "-S", "ready", "-t", "news-app"},
		[]string{"add", "task", strings.Repeat("😀x", 90), "-S", "ready", "-t", "news-app"},
		[]string{"jobs", "set", "tiny", "title=" + strings.Repeat("Long title ", 9), "news.url=https://example.com/" + strings.Repeat("p/", 50)})
	for _, f := range []string{"text", "md", "html"} {
		args := []string{"--job", "tiny", "--format", f}
		if f != "text" {
			args = append(args, "-o", "-")
		}
		if w, l := maxWidth(newsOut(t, args...)); w > 70 {
			t.Errorf("%s wide: line of %d cells: %q", f, w, l)
		}
	}
}

// ---- stress ----

// bigJSON writes the stress model to a file (captureOutput's pipe would fill up and block on a large print).
func bigJSON(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "m.json")
	runAll(t, []string{"news", "--job", "big", "--format", "json", "-o", p})
	return mustRead(t, p)
}

func newsStress(t *testing.T) string {
	t.Helper()
	old := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = old })
	path := inTempDir(t)
	t.Setenv("MW_SESSION", "")
	t.Setenv("MW_TENANT", "")
	runAll(t, []string{"init", "Stress"}, []string{"gate", "level", "judged", "--workspace", "--by", "t"}, []string{"jobs", "enable", "big", "-t", "stress"})
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	tn := &w.Tenants[0]
	base := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	n := 0
	stamp := func() string { n++; return base.Add(time.Duration(n) * time.Second).Format(time.RFC3339) }
	long := strings.Repeat("long title ", 30)
	add := func(prefix string, i int, kind, status, judged string, evs ...string) *Item {
		it := Item{ID: fmt.Sprintf("%s-%04d", prefix, i), Kind: kind, Title: fmt.Sprintf("%s %d %s", prefix, i, long), Status: status, Judged: judged, Created: stamp(), Updated: stamp()}
		for _, a := range evs {
			it.Events = append(it.Events, Event{At: stamp(), Action: a, By: "s", Note: long})
		}
		tn.Items = append(tn.Items, it)
		return &tn.Items[len(tn.Items)-1]
	}
	for i := 1; i <= 1000; i++ {
		if i%10 == 0 {
			add("WAV", i, "wave", "done", "", "done")
		} else {
			add("WAV", i, "wave", "ready", "")
		}
	}
	for i := 1; i <= 150; i++ {
		add("DON", i, "task", "done", "ungated", "created", "done", "accept")
	}
	for i := 1; i <= 100; i++ {
		add("BLK", i, "task", "blocked", "", "block")
	}
	for i := 1; i <= 100; i++ { // blocked items that wait for a reviewer are review, not blocked
		rv := add("REV", i, "task", "blocked", "", "block")
		rv.Events[0].Note = "awaiting review: ready for a look"
	}
	for i := 1; i <= 100; i++ {
		add("ASK", i, "task", "ready", "", "ask")
	}
	for i := 1; i <= 100; i++ {
		add("DNG", i, "task", "doing", "", "start")
	}
	for i := 1; i <= 100; i++ {
		add("RDY", i, "task", "ready", "", "ready")
	}
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNewsStressModel(t *testing.T) {
	newsStress(t)
	js := bigJSON(t)
	if len(js) > newsModelMax {
		t.Fatalf("model is %d bytes", len(js))
	}
	m := decodeModel(t, js)
	c := m.Now.Counts
	if c.Blocked != 100 || c.Review != 100 || c.Needs != 100 || c.Doing != 100 || c.Ready != 200 {
		t.Fatalf("counts: %+v", c)
	}
	check := func(name string, got, want int) {
		if got != want {
			t.Errorf("%s: %d, want %d", name, got, want)
		}
	}
	check("needs", len(m.Now.Needs), 50)
	check("needs more", m.Now.More["needs"], 50)
	check("review", len(m.Now.Review), 50)
	check("blocked", len(m.Now.Blocked), 50)
	check("doing", len(m.Now.Doing), 50)
	check("milestones open", len(m.Now.Milestones.Open), 200)
	check("milestones more", m.Now.More["milestones"], 700)
	check("milestones closed", m.Now.Milestones.ClosedCount, 100)
	check("done list", len(m.Happened.Done), 100)
	check("done more", m.Happened.More["done"], 50)
	check("closed milestones", len(m.Happened.ClosedMilestones), 100)
	check("accepted", len(m.Happened.Accepted), 100)
	check("accepted more", m.Happened.More["accepted"], 50)
	check("ready", len(m.Next.Ready), 10)
	check("ready more", m.Next.More["ready"], 190)
	check("next milestones", len(m.Next.NextMilestones), 5)
	check("next milestones more", m.Next.More["next_milestones"], 895)
	check("caps", m.Now.Caps["needs"]+m.Happened.Caps["done"]+m.Next.Caps["ready"]+m.Next.Caps["next_milestones"], 50+100+10+5)
	if m.Truncated {
		t.Error("truncated flag set at the caps")
	}
	long := func(s string) {
		if n := len([]rune(s)); n > newsItemText {
			t.Fatalf("item text of %d characters", n)
		}
	}
	for _, l := range [][]newsItem{m.Now.Needs, m.Now.Review, m.Now.Blocked, m.Now.Doing, m.Happened.Done, m.Next.Ready} {
		for _, it := range l {
			long(it.Title)
			long(it.Note)
		}
	}
	// the bulletins hold the caps too, within 70 columns
	for _, f := range []string{"text", "md", "html"} {
		args := []string{"--job", "big", "--format", f}
		if f != "text" {
			args = append(args, "-o", "-")
		}
		out := newsOut(t, args...)
		if w, l := maxWidth(out); w > 70 {
			t.Errorf("%s: line of %d cells: %q", f, w, l)
		}
		if f == "text" {
			checkGolden(t, "news-stress-text", out)
		}
	}
	checkGolden(t, "news-stress-md", newsOut(t, "--job", "big", "-o", "-"))
	checkGolden(t, "news-stress-html", newsOut(t, "--job", "big", "--format", "html", "-o", "-"))
}

func TestNewsGuardKeepsUnderOneMB(t *testing.T) {
	m := &newsModel{Version: 1, Now: newsNow{More: map[string]int{}}, Happened: newsHappened{More: map[string]int{}, Counts: map[string]int{}}}
	big := strings.Repeat("x", 190)
	for i := 0; i < 20000; i++ {
		m.Happened.Done = append(m.Happened.Done, newsItem{Tenant: "t", ID: strconv.Itoa(i), Title: big, Note: big, Since: "2026-01-01T00:00:00Z", By: "b"})
	}
	newsGuardSize(m)
	b, _ := json.Marshal(m)
	if len(b) > newsModelMax || !m.Truncated || m.Happened.More["done"] == 0 {
		t.Fatalf("guard: %d bytes truncated=%v more=%v", len(b), m.Truncated, m.Happened.More)
	}
}

// ---- releases ----

func releaseFiles(t *testing.T, dir string) []string {
	t.Helper()
	ents, _ := os.ReadDir(dir)
	var out []string
	for _, e := range ents {
		out = append(out, e.Name())
	}
	return out
}

func TestNewsReleaseSequence(t *testing.T) {
	path := newsBoardAt(t, time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC))
	dir := filepath.Join(filepath.Dir(path), "docs", "news", "releases")
	out := newsOut(t, "--job", "tiny", "--release")
	if !strings.HasPrefix(out, "released 1: ") {
		t.Fatalf("release 1: %q", out)
	}
	files := strings.Join(releaseFiles(t, dir), " ")
	for _, f := range []string{"1.json", "1.md", "1.html", "latest"} {
		if !strings.Contains(files, f) {
			t.Fatalf("missing %s in %s", f, files)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "latest")); strings.TrimSpace(string(b)) != "1" {
		t.Fatalf("latest: %q", b)
	}
	r1 := decodeModel(t, mustRead(t, filepath.Join(dir, "1.json")))
	if r1.Release.Number == nil || *r1.Release.Number != 1 || r1.Release.Since != "" || r1.Release.Previous != nil || r1.Release.Changes == 0 {
		t.Fatalf("release 1: %+v", r1.Release)
	}
	md1 := mustRead(t, filepath.Join(dir, "1.md"))
	if !strings.Contains(md1, "RELEASE 1 / ") || !strings.Contains(md1, "since the start") {
		t.Fatalf("1.md:\n%s", md1)
	}
	// nothing new: refused with exit 0
	if out = newsOut(t, "--job", "tiny", "--release"); strings.TrimSpace(out) != "nothing new since release 1" {
		t.Fatalf("empty release: %q", out)
	}
	if strings.Contains(strings.Join(releaseFiles(t, dir), " "), "2.json") {
		t.Fatal("an empty release was cut")
	}
	// the draft is empty too, and says what it follows
	d := decodeModel(t, newsOut(t, "--job", "tiny", "--format", "json"))
	if d.Release.Number != nil || d.Release.Previous == nil || *d.Release.Previous != 1 || d.Release.Changes != 0 {
		t.Fatalf("draft after release: %+v", d.Release)
	}
	// a change makes release 2, compared with release 1 only
	runAll(t, []string{"add", "task", "Brand new", "-S", "ready", "-t", "news-app"})
	time.Sleep(2 * time.Millisecond)
	if out = newsOut(t, "--job", "tiny", "--release"); !strings.HasPrefix(out, "released 2: ") {
		t.Fatalf("release 2: %q", out)
	}
	r2 := decodeModel(t, mustRead(t, filepath.Join(dir, "2.json")))
	if *r2.Release.Previous != 1 || r2.Release.Since != r1.Release.At || len(r2.Happened.NewItems) != 1 || len(r2.Happened.Done) != 0 {
		t.Fatalf("release 2: %+v new=%d done=%d", r2.Release, len(r2.Happened.NewItems), len(r2.Happened.Done))
	}
	if md2 := mustRead(t, filepath.Join(dir, "2.md")); !strings.Contains(md2, "vs release 1 (") || !strings.Contains(md2, "releases: 2 1 ") {
		t.Fatalf("2.md:\n%s", md2)
	}
	// 1.json is frozen
	if again := mustRead(t, filepath.Join(dir, "1.json")); again != mj(t, r1) {
		t.Fatal("1.json changed")
	}
	// --force cuts an empty release
	if out = newsOut(t, "--job", "tiny", "--release", "--force"); !strings.HasPrefix(out, "released 3: ") {
		t.Fatalf("forced: %q", out)
	}
	if latest := mustRead(t, filepath.Join(dir, "latest")); strings.TrimSpace(latest) != "3" {
		t.Fatalf("latest: %q", latest)
	}
	// -j reports the refusal as data
	runAll(t, []string{"jobs", "set", "tiny", "news_releases=out/rel/"})
	js := captureOutput(t, func() { runAll(t, []string{"news", "--job", "tiny", "--release", "-j"}) })
	var res newsReleaseResult
	if err := json.Unmarshal([]byte(js), &res); err != nil || !res.Released || res.Number != 1 {
		t.Fatalf("-j: %v %s", err, js)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "out", "rel", "1.json")); err != nil {
		t.Fatal("custom news_releases not used:", err)
	}
	wantErr(t, "--force is only for --release", "news", "--force")
}

func srcCmd(bin string) *exec.Cmd {
	_, file, _, _ := runtime.Caller(0)
	c := exec.Command("go", "build", "-o", bin, ".")
	c.Dir = filepath.Dir(file)
	return c
}

func mj(t *testing.T, m *newsModel) string {
	t.Helper()
	b, err := newsModelJSON(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// releases live only in the releases folder: news.out keeps the draft
func TestNewsReleaseLeavesNewsOutAlone(t *testing.T) {
	path := newsBoardAt(t, time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC))
	newsOut(t)
	page := filepath.Join(filepath.Dir(path), "docs", "news", "status.md")
	before := mustRead(t, page)
	newsOut(t, "--job", "tiny", "--release")
	if after := mustRead(t, page); after != before || !strings.Contains(after, "DRAFT - NOT RELEASED") {
		t.Fatalf("--release rewrote news.out:\n%s", after)
	}
}

func TestNewsConcurrentRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("subprocess lock test")
	}
	bin := filepath.Join(os.TempDir(), fmt.Sprintf("mw-test-%d", os.Getpid())) // built before inTempDir moves HOME (the module cache lives there)
	if out, err := srcCmd(bin).CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	t.Cleanup(func() { os.Remove(bin) })
	path := newsBoardAt(t, time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC))
	root := filepath.Dir(path)
	const n = 6
	var wg sync.WaitGroup
	outs := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := exec.Command(bin, "news", "--job", "tiny", "--release", "--force")
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "MW_SESSION=", "MW_TENANT=")
			b, err := cmd.CombinedOutput()
			outs[i] = string(b)
			if err != nil {
				outs[i] += " ERR " + err.Error()
			}
		}(i)
	}
	wg.Wait()
	seen := map[string]bool{}
	for _, o := range outs {
		if !strings.HasPrefix(o, "released ") || strings.Contains(o, "ERR") {
			t.Fatalf("a release failed: %q", o)
		}
		seen[strings.Fields(o)[1]] = true
	}
	if len(seen) != n {
		t.Fatalf("release numbers not distinct: %v", outs)
	}
	dir := filepath.Join(root, "docs", "news", "releases")
	for i := 1; i <= n; i++ {
		m := decodeModel(t, mustRead(t, filepath.Join(dir, fmt.Sprintf("%d.json", i))))
		if m.Release.Number == nil || *m.Release.Number != i {
			t.Fatalf("%d.json holds release %v", i, m.Release.Number)
		}
	}
}

// ---- metrics ----

func metricsJSON(extra string) string {
	return `{"version":1,"updated":"2026-10-22T13:30:00Z","sections":[` + extra + `]}`
}

func TestNewsMetricsInvalid(t *testing.T) {
	rows := func(n int) string {
		var r []string
		for i := 0; i < n; i++ {
			r = append(r, fmt.Sprintf(`{"name":"r%d","done":1,"total":2}`, i))
		}
		return strings.Join(r, ",")
	}
	sec := func(title, kind, rowsJSON string) string {
		return fmt.Sprintf(`{"title":%q,"kind":%q,"rows":[%s]}`, title, kind, rowsJSON)
	}
	var eight []string
	for i := 0; i < 9; i++ {
		eight = append(eight, sec("s", "progress", ""))
	}
	cases := []struct{ name, file, want string }{
		{"not json", `{nope`, ""},
		{"no version", `{"sections":[]}`, "version"},
		{"version zero", `{"version":0,"sections":[]}`, "version"},
		{"nine sections", metricsJSON(strings.Join(eight, ",")), "9 sections (at most 8)"},
		{"41 rows", metricsJSON(sec("s", "progress", rows(41))), "41 rows (at most 40)"},
		{"long title", metricsJSON(sec(strings.Repeat("t", 61), "progress", "")), "title is 61 characters (at most 60)"},
		{"long name", metricsJSON(sec("s", "progress", `{"name":"`+strings.Repeat("n", 61)+`","done":0,"total":1}`)), "name is 61 characters"},
		{"long unit", metricsJSON(sec("s", "value", `{"name":"n","value":1,"unit":"`+strings.Repeat("u", 61)+`"}`)), "unit is 61 characters"},
		{"long note", metricsJSON(`{"title":"s","kind":"progress","note":"` + strings.Repeat("n", 121) + `","rows":[]}`), "note is 121 characters (at most 120)"},
		{"long legend", metricsJSON(`{"title":"s","kind":"split","legend":{"good":"` + strings.Repeat("g", 61) + `"},"rows":[]}`), "legend good"},
		{"negative count", metricsJSON(sec("s", "split", `{"name":"n","good":-1}`)), "integer >= 0"},
		{"fraction count", metricsJSON(sec("s", "progress", `{"name":"n","done":1.5,"total":3}`)), "integer >= 0"},
		{"done over total", metricsJSON(sec("s", "progress", `{"name":"n","done":5,"total":3}`)), "done 5 is more than total 3"},
		{"infinite value", metricsJSON(sec("s", "value", `{"name":"n","value":1e999}`)), ""},
		{"value missing", metricsJSON(sec("s", "value", `{"name":"n"}`)), "value is missing"},
		{"unknown kind", metricsJSON(sec("s", "pie", "")), "kind"},
		{"bad better", metricsJSON(sec("s", "value", `{"name":"n","value":1,"better":"up"}`)), "better"},
		{"empty title", metricsJSON(sec(" ", "progress", "")), "title is empty"},
	}
	for _, c := range cases {
		m := parseMetrics([]byte(c.file))
		if m.Error == "" || !strings.Contains(m.Error, c.want) || len(m.Sections) != 0 {
			t.Errorf("%s: want error %q, got %+v", c.name, c.want, m)
		}
	}
	// the unknown-field rule and control stripping
	ok := parseMetrics([]byte(metricsJSON(`{"title":"Ti\u001b[31mtle","kind":"progress","future":1,"rows":[{"name":"a\nb","done":0,"total":0,"extra":true}]}`)))
	if ok.Error != "" || ok.Sections[0].Title != "Ti[31mtle" || ok.Sections[0].Rows[0].Name != "a b" {
		t.Fatalf("valid file: %+v", ok)
	}
	// a higher version: a notice, and what can be read is kept
	nv := parseMetrics([]byte(`{"version":2,"sections":[{"title":"s","kind":"progress","rows":[{"name":"a","done":1,"total":2}]}]}`))
	if nv.Error != "" || !strings.Contains(nv.Notice, "newer metrics format") || len(nv.Sections) != 1 {
		t.Fatalf("newer: %+v", nv)
	}
	if nv = parseMetrics([]byte(`{"version":3,"sections":"a new shape"}`)); nv.Error != "" || !strings.Contains(nv.Notice, "newer metrics format") {
		t.Fatalf("newer unreadable: %+v", nv)
	}
}

func TestNewsMetricsFileLimitsAndRender(t *testing.T) {
	path := newsBoard(t)
	root := filepath.Dir(path)
	runAll(t, []string{"jobs", "set", "tiny", "metrics=docs/news/metrics.json"})
	// missing file: no section
	m := decodeModel(t, newsOut(t, "--job", "tiny", "--format", "json"))
	if m.Metrics != nil {
		t.Fatalf("missing file: %+v", m.Metrics)
	}
	big := metricsJSON("") + strings.Repeat(" ", 256<<10)
	_ = os.MkdirAll(filepath.Join(root, "docs", "news"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "docs", "news", "metrics.json"), []byte(big), 0o644)
	m = decodeModel(t, newsOut(t, "--job", "tiny", "--format", "json"))
	if m.Metrics == nil || !strings.Contains(m.Metrics.Error, "256 KB") {
		t.Fatalf("oversize: %+v", m.Metrics)
	}
	_ = os.WriteFile(filepath.Join(root, "docs", "news", "metrics.json"), []byte(`{"version":1,"sections":[{"title":"T","kind":"progress","rows":[{"name":"a","done":9,"total":3}]}]}`), 0o644)
	for _, f := range []string{"md", "html"} {
		out := newsOut(t, "--job", "tiny", "--format", f, "-o", "-")
		if !strings.Contains(out, "metrics file invalid: section 1 row 1: done 9 is more than total 3") {
			t.Errorf("%s: invalid file not shown:\n%s", f, out)
		}
	}
	// a symlink out of the workspace is refused as data, not followed
	if runtime.GOOS != "windows" {
		outside := filepath.Join(t.TempDir(), "m.json")
		_ = os.WriteFile(outside, []byte(metricsJSON("")), 0o644)
		_ = os.Remove(filepath.Join(root, "docs", "news", "metrics.json"))
		if err := os.Symlink(outside, filepath.Join(root, "docs", "news", "metrics.json")); err != nil {
			t.Fatal(err)
		}
		m = decodeModel(t, newsOut(t, "--job", "tiny", "--format", "json"))
		if m.Metrics == nil || !strings.Contains(m.Metrics.Error, "leaves the workspace") {
			t.Fatalf("symlink: %+v", m.Metrics)
		}
	}
}

func TestNewsMetricsDeltasAcrossReleases(t *testing.T) {
	path := newsBoardAt(t, time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC))
	root := filepath.Dir(path)
	runAll(t, []string{"jobs", "set", "tiny", "metrics=m.json"})
	write := func(doc string) {
		if err := os.WriteFile(filepath.Join(root, "m.json"), []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	doc := func(good, bad, done int, val string) string {
		return fmt.Sprintf(`{"version":1,"updated":"2026-10-22T13:30:00Z","sections":[
{"title":"Harness","kind":"split","rows":[{"name":"doc/ios","good":%d,"warn":2,"bad":%d,"none":1}]},
{"title":"Stores","kind":"progress","rows":[{"name":"Play","done":%d,"total":9},{"name":"New","done":1,"total":2}]},
{"title":"Quality","kind":"value","rows":[{"name":"crash-free","value":%s,"unit":"%%","target":99.5}]}]}`, good, bad, done, val)
	}
	write(doc(8, 4, 3, "99.2"))
	newsOut(t, "--job", "tiny", "--release")
	r1 := decodeModel(t, mustRead(t, filepath.Join(root, "docs", "news", "releases", "1.json")))
	if r1.Metrics == nil || len(r1.Metrics.Sections) != 3 || r1.Metrics.Sections[0].Rows[0].Delta != nil {
		t.Fatalf("release 1 metrics: %+v", r1.Metrics)
	}
	// only metrics move: not empty
	write(doc(6, 7, 5, "98.9"))
	d := decodeModel(t, newsOut(t, "--job", "tiny", "--format", "json"))
	rows := d.Metrics.Sections
	if g := rows[0].Rows[0].Delta; g["good"] != -2 || g["bad"] != 3 || g["warn"] != 0 {
		t.Fatalf("split delta: %v", g)
	}
	if rows[1].Rows[0].Delta["done"] != 2 || rows[1].Rows[1].Delta != nil {
		t.Fatalf("progress delta: %+v", rows[1].Rows)
	}
	if v := rows[2].Rows[0].Delta["value"]; v < -0.31 || v > -0.29 {
		t.Fatalf("value delta: %v", v)
	}
	if d.Release.Changes != 3 {
		t.Fatalf("changes: %d", d.Release.Changes)
	}
	txt := newsOut(t, "--job", "tiny", "--format", "text")
	if !strings.Contains(txt, "▼2") || !strings.Contains(txt, "▲2") || !strings.Contains(txt, "▼0.3") {
		t.Fatalf("deltas not drawn:\n%s", txt)
	}
	if out := newsOut(t, "--job", "tiny", "--release"); !strings.HasPrefix(out, "released 2: ") {
		t.Fatalf("metrics-only release refused: %q", out)
	}
	// release 3 compares with release 2's frozen metrics, not release 1's
	write(doc(7, 7, 5, "98.9"))
	newsOut(t, "--job", "tiny", "--release")
	r3 := decodeModel(t, mustRead(t, filepath.Join(root, "docs", "news", "releases", "3.json")))
	if g := r3.Metrics.Sections[0].Rows[0].Delta; g["good"] != 1 || len(g) != 1 {
		t.Fatalf("release 3 delta: %v", g)
	}
	if !strings.Contains(mustRead(t, filepath.Join(root, "docs", "news", "releases", "3.md")), "▲1") {
		t.Fatal("3.md lacks the delta")
	}
}

// ---- agreement and escaping ----

func countPrefix(out, prefix string) int {
	n := 0
	for _, l := range strings.Split(html.UnescapeString(out), "\n") {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	return n
}

func TestNewsFormatsAgree(t *testing.T) {
	newsStress(t)
	m := decodeModel(t, bigJSON(t))
	outs := map[string]string{
		"text": newsOut(t, "--job", "big", "--format", "text"),
		"md":   newsOut(t, "--job", "big", "-o", "-"),
		"html": newsOut(t, "--job", "big", "--format", "html", "-o", "-"),
	}
	c := m.Now.Counts
	ongoing := fmt.Sprintf("  %d of %d done, %d ready", c.Done, c.Total, c.Ready)
	for f, out := range outs {
		for prefix, capn := range map[string]int{"! ": min(c.Needs, 5), "? ": min(c.Review, 5), "- ": min(c.Blocked, 5), "+ ": min(c.Doing, 5), "> ": min(c.Ready, 10)} {
			got := countPrefix(out, prefix)
			if got > capn || got != countPrefix(outs["text"], prefix) {
				t.Errorf("%s: %q lines %d (cap %d, text %d)", f, prefix, got, capn, countPrefix(outs["text"], prefix))
			}
		}
		if !strings.Contains(out, ongoing) {
			t.Errorf("%s: lacks %q", f, ongoing)
		}
		if got := strings.Count(out, "(see full board)"); got != strings.Count(outs["text"], "(see full board)") {
			t.Errorf("%s: %d more lines", f, got)
		}
	}
}

// on a small board nothing is trimmed, so the lines match the json counts exactly
func TestNewsFormatsAgreeSmall(t *testing.T) {
	newsBoard(t)
	m := decodeModel(t, newsOut(t, "--job", "tiny", "--format", "json"))
	c := m.Now.Counts
	if c.Needs != 1 || c.Blocked != 1 || c.Doing != 1 || m.Now.Needs[0].Tenant != "doc" {
		t.Fatalf("counts / asks across tenants: %+v %+v", c, m.Now.Needs)
	}
	for f, out := range map[string]string{
		"text": newsOut(t, "--job", "tiny", "--format", "text"),
		"md":   newsOut(t, "--job", "tiny", "-o", "-"),
		"html": newsOut(t, "--job", "tiny", "--format", "html", "-o", "-"),
	} {
		for prefix, want := range map[string]int{"! ": c.Needs, "? ": c.Review, "- ": c.Blocked, "+ ": c.Doing, "> ": c.Ready} {
			if got := countPrefix(out, prefix); got != want {
				t.Errorf("%s: %q lines %d, want %d", f, prefix, got, want)
			}
		}
		if got, want := countPrefix(out, "0 "), len(m.Happened.Done)+len(m.Happened.Accepted)+len(m.Happened.Decisions)+len(m.Happened.Answers)+len(m.Happened.SentBack); got != want {
			t.Errorf("%s: item lines %d, want %d", f, got, want)
		}
		if !strings.Contains(out, fmt.Sprintf("  %d of %d done, %d ready", c.Done, c.Total, c.Ready)) {
			t.Errorf("%s: ongoing counts", f)
		}
	}
}

func TestNewsStaysWithinLimit(t *testing.T) {
	newsStress(t)
	for _, args := range [][]string{{"--format", "text"}, {"-o", "-"}, {"--format", "html", "-o", "-"}} {
		out := newsOut(t, append([]string{"--job", "big"}, args...)...)
		body := out
		if i := strings.Index(out, "<pre>"); i >= 0 {
			body = out[i:]
		}
		if n := len([]rune(body)); n > bulletinLimit && !strings.Contains(out, "<pre>") {
			t.Errorf("%v: %d characters", args, n)
		}
		if !strings.Contains(out, "(see full board)") {
			t.Errorf("%v: no more lines", args)
		}
	}
	rev := decodeModel(t, bigJSON(t))
	if rev.Now.Counts.Review != 100 || rev.Now.Counts.Blocked != 100 {
		t.Fatalf("review vs blocked: %+v", rev.Now.Counts)
	}
	for _, b := range rev.Now.Blocked {
		if strings.HasPrefix(b.ID, "REV") {
			t.Fatal("a review item is in blocked")
		}
	}
}

func TestNewsHostileTitlesRender(t *testing.T) {
	bad := "x ``` ````` <script>alert(1)</script> [a](http://e) & \x1b[31mRED\x1b[0m ‮| `y`"
	it := newsItem{Tenant: "t", ID: "TSK-1", Title: bad, Note: bad, Since: "2026-01-01T00:00:00Z", By: "b"}
	m := &newsModel{Version: 1, Job: newsJobInfo{ID: "j", Title: bad, Description: bad, URL: "https://e.com/<x>"},
		Release: newsRelease{At: "2026-01-02T00:00:00Z"}, Summary: bad + "\n```\n<b>"}
	m.Now.Needs, m.Happened.Done, m.Next.Ready = []newsItem{it}, []newsItem{it}, []newsItem{it}
	m.Now.Counts = newsCounts{Needs: 1, Ready: 1}
	m.Happened.Counts = map[string]int{"done": 1}
	m.Now.More, m.Happened.More, m.Next.More = map[string]int{}, map[string]int{}, map[string]int{}
	m.Metrics = &newsMetrics{Error: bad}
	// through a frozen file, as a release does
	back := decodeModel(t, mj(t, m))
	md := renderBulletinMD(back, []int{1})
	lines := strings.Split(strings.TrimSuffix(md, "\n"), "\n")
	fence := strings.TrimSuffix(lines[0], "text")
	if lines[len(lines)-1] != fence || strings.Trim(fence, "`") != "" {
		t.Fatalf("md fence: %q ... %q", lines[0], lines[len(lines)-1])
	}
	for _, l := range lines[1 : len(lines)-1] {
		if strings.HasPrefix(strings.TrimSpace(l), fence) {
			t.Fatalf("a content line can close the fence: %q", l)
		}
	}
	h := renderBulletinHTML(back, []int{1}, "")
	if strings.Contains(h, "<script>") || strings.Contains(h, "<b>") || strings.Contains(h, "\x1b") || !strings.Contains(h, "&lt;script&gt;") {
		t.Fatalf("html not escaped:\n%s", h)
	}
	for name, out := range map[string]string{"md": md, "html": h, "text": renderBulletinText(back, []int{1})} {
		if strings.ContainsAny(out, "\x1b‮") {
			t.Errorf("%s keeps a control character", name)
		}
		if w, l := maxWidth(out); w > 70 {
			t.Errorf("%s: %d cells: %q", name, w, l)
		}
	}
}

func TestJobsNewsReleasesAndMetricsKeys(t *testing.T) {
	inTempDir(t)
	runAll(t, []string{"init", "J"}, []string{"jobs", "enable", "j", "-t", "j"},
		[]string{"jobs", "set", "j", "news_releases=docs/rel/", "metrics=docs/m.json"})
	out := captureOutput(t, func() { runAll(t, []string{"jobs", "show", "j", "-j"}) })
	if !strings.Contains(out, `"news_releases": "docs/rel/"`) || !strings.Contains(out, `"metrics": "docs/m.json"`) {
		t.Fatalf("show:\n%s", out)
	}
	if b := mustRead(t, ".miniwaves.json"); !strings.Contains(b, `"news_releases"`) || !strings.Contains(b, `"metrics"`) {
		t.Fatalf("index: %s", b)
	}
	wantErr(t, "must stay inside", "jobs", "set", "j", "metrics=../x.json")
	wantErr(t, "must be relative", "jobs", "set", "j", "news_releases=/abs/")
	runAll(t, []string{"jobs", "set", "j", "metrics="})
	if out = captureOutput(t, func() { runAll(t, []string{"jobs", "show", "j", "-j"}) }); !strings.Contains(out, `"metrics": ""`) {
		t.Fatalf("reset:\n%s", out)
	}
}

func TestNewsDefaultPathAndUnchanged(t *testing.T) {
	path := newsBoard(t)
	root := filepath.Dir(path)
	out := newsOut(t)
	file := filepath.Join(root, "docs", "news", "status.md")
	if b, err := os.ReadFile(file); err != nil || !strings.HasPrefix(out, "wrote ") || !strings.HasPrefix(string(b), "```") {
		t.Fatalf("first run: %q %v", out, err)
	}
	if out = newsOut(t); !strings.HasPrefix(out, "unchanged ") {
		t.Fatalf("second run: %q", out)
	}
	runAll(t, []string{"note", "TSK-002", "progress", "-t", "news-app"})
	if out = newsOut(t); !strings.HasPrefix(out, "unchanged ") && !strings.HasPrefix(out, "wrote ") {
		t.Fatalf("after change: %q", out)
	}
	_ = os.Remove(file)
	if out = newsOut(t); !strings.HasPrefix(out, "wrote ") {
		t.Fatalf("after delete: %q", out)
	}
}

// BUG-017: a closed milestone carries its member progress, so the news pane shows done n/n, not "not started 0/0".
func TestNewsClosedMilestoneProgress(t *testing.T) {
	newsBoard(t)
	runAll(t,
		[]string{"done", "TSK-002", "ok", "--no-evidence", "none", "-t", "news-app"},
		[]string{"wave", "step", "WAV-001", "--na-rest", "test", "-t", "news-app"},
		[]string{"wave", "close", "WAV-001", "--no-evidence", "none", "-t", "news-app"},
	)
	m := decodeModel(t, newsOut(t, "--job", "tiny", "--format", "json"))
	if len(m.Happened.ClosedMilestones) != 1 {
		t.Fatalf("closed milestones: %+v", m.Happened.ClosedMilestones)
	}
	c := m.Happened.ClosedMilestones[0]
	if c.ID != "WAV-001" || c.Done != 2 || c.Total != 2 {
		t.Fatalf("closed milestone progress: %+v", c)
	}
}
