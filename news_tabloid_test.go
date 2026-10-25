package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tabModel() *newsModel {
	one := 1
	m := &newsModel{Version: 1, Job: newsJobInfo{ID: "j", Title: "Demo"}, Release: newsRelease{At: "2026-10-25T10:00:00Z", Since: "2026-10-20T10:00:00Z", Previous: &one, Changes: 3}}
	m.Now.More, m.Happened.More, m.Next.More = map[string]int{}, map[string]int{}, map[string]int{}
	m.Happened.Counts = map[string]int{}
	return m
}

func di(tenant, id, title string) newsItem {
	return newsItem{Tenant: tenant, ID: id, Title: title, Since: "2026-10-24T09:00:00Z", By: "bo"}
}

func TestTabloidMasthead(t *testing.T) {
	md := renderTabloidMD(tabModel(), []int{1}, tabloidCtx{})
	if !strings.HasPrefix(md, "# Demo News — Edition 2 (draft)\n\n_since Edition 1, 2026-10-20 · up to 2026-10-25_\n") {
		t.Fatalf("masthead:\n%s", md)
	}
	m := tabModel()
	two := 2
	m.Release.Number = &two
	if md := renderTabloidMD(m, []int{2, 1}, tabloidCtx{}); !strings.HasPrefix(md, "# Demo News — Edition 2\n") || strings.Contains(md, "draft") {
		t.Fatalf("released masthead:\n%s", md)
	}
}

func TestTabloidLeadPriority(t *testing.T) {
	rel, ms, plain := di("a", "TSK-1", "Cut release 1.2"), di("a", "WAV-1", "Search"), di("b", "TSK-2", "Fix x")
	ms.Done, ms.Total = 3, 3
	rel.Release = true // release work is detected by its release-notes file, never by the title
	need := di("c", "TSK-9", "Pick a name")
	need.Note = "A or B?"
	cases := []struct {
		name  string
		set   func(m *newsModel)
		head  string
		inAlt string
	}{
		{"release beats all", func(m *newsModel) {
			m.Happened.Done = []newsItem{plain, plain, rel}
			m.Happened.ClosedMilestones = []newsItem{ms}
			m.Now.Needs = []newsItem{need}
		}, "## Done: Cut release 1.2", "b/TSK-2"},
		{"milestone beats tenant count", func(m *newsModel) {
			m.Happened.Done = []newsItem{plain, plain}
			m.Happened.ClosedMilestones = []newsItem{ms}
		}, "## Milestone closed: Search", "b/TSK-2"},
		{"tenant with most done", func(m *newsModel) {
			x := di("a", "TSK-5", "One")
			m.Happened.Done = []newsItem{x, plain, plain, di("b", "TSK-3", "Two")}
			m.Happened.Counts["done"] = 4
		}, "## 3 items done in b", ""},
		{"needs Sir", func(m *newsModel) {
			m.Now.Needs = []newsItem{need}
		}, "## Waiting on Sir: Pick a name", ""},
		{"counts fallback", func(m *newsModel) {
			m.Happened.Counts["new_items"] = 2
		}, "## 2 new items filed", ""},
	}
	for _, c := range cases {
		m := tabModel()
		c.set(m)
		md := renderTabloidMD(m, nil, tabloidCtx{})
		if !strings.Contains(md, "\n"+c.head+"\n") {
			t.Errorf("%s: lead is not %q:\n%s", c.name, c.head, md)
		}
		if len(m.Now.Needs) > 0 && !strings.Contains(md, "> **Needs you**") {
			t.Errorf("%s: no Needs you box", c.name)
		}
	}
	// the lead item is not repeated in "Also in this edition"
	m := tabModel()
	m.Happened.Done = []newsItem{rel, plain}
	md := renderTabloidMD(m, nil, tabloidCtx{})
	if strings.Count(md, "a/TSK-1") != 1 || !strings.Contains(md, "### Also in this edition\n\n- **b/TSK-2** Fix x") {
		t.Errorf("also:\n%s", md)
	}
}

func TestTabloidAlsoCap(t *testing.T) {
	m := tabModel()
	for i := 0; i < 12; i++ {
		m.Happened.Done = append(m.Happened.Done, di("a", "TSK-"+string(rune('A'+i)), "Item"))
	}
	m.Happened.Counts["done"], m.Release.Changes = 12, 12
	md := renderTabloidMD(m, nil, tabloidCtx{})
	if n := strings.Count(md, "\n- **a/"); n != 8 || !strings.Contains(md, "- +2 more") {
		t.Errorf("also cap: %d\n%s", n, md)
	}
}

func TestTabloidEmptyWindow(t *testing.T) {
	m := tabModel()
	m.Release.Changes = 0
	m.Now.Counts = newsCounts{Doing: 1, Ready: 1}
	m.Now.Doing = []newsItem{di("a", "TSK-1", "Work")}
	m.Next.Ready = []newsItem{di("a", "TSK-2", "Later")}
	md := renderTabloidMD(m, []int{1}, tabloidCtx{JobsHref: "jobs.md"})
	for _, want := range []string{"**Quiet edition — nothing new since Edition 1**", "### Coming up", "- In progress (1): a/TSK-1 Work", "- Up next (1 ready): a/TSK-2 Later", "[Now: jobs.md](<jobs.md>)", "Editions: [1](<1.md>)"} {
		if !strings.Contains(md, want) {
			t.Errorf("lacks %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "Also in this edition") || strings.Contains(md, "### Proof") || strings.Contains(md, "Needs you") {
		t.Errorf("empty sections shown:\n%s", md)
	}
	m.Now = newsNow{More: map[string]int{}}
	m.Next = newsNext{More: map[string]int{}}
	if md := renderTabloidMD(m, nil, tabloidCtx{}); !strings.Contains(md, "- Nothing queued.") {
		t.Errorf("empty coming up:\n%s", md)
	}
}

func TestTabloidProofSelection(t *testing.T) {
	dir := t.TempDir()
	m := tabModel()
	img := func(n string) newsProof {
		return newsProof{Path: filepath.ToSlash(filepath.Join(dir, "ev", n)), Image: true}
	}
	file := newsProof{Path: filepath.ToSlash(filepath.Join(dir, "ev", "log.txt"))}
	shot := di("a", "TSK-1", "Shot")
	shot.Proof = []newsProof{img("one.png"), file}
	logOnly := di("a", "TSK-2", "Log only")
	logOnly.Proof = []newsProof{file}
	rev := di("a", "TSK-3", "Review me")
	rev.Proof = []newsProof{file}
	rel := di("a", "TSK-4", "Cut 2.0")
	rel.Proof, rel.Release = []newsProof{file, {Path: filepath.ToSlash(filepath.Join(dir, "ev", "extra.txt"))}}, true
	m.Happened.Done = []newsItem{shot, logOnly, rel}
	m.Now.Review = []newsItem{rev}
	for i := 0; i < 8; i++ {
		x := di("a", "TSK-1"+string(rune('0'+i)), "More")
		x.Proof = []newsProof{img("m" + string(rune('0'+i)) + ".png")}
		m.Happened.Done = append(m.Happened.Done, x)
	}
	md := renderTabloidMD(m, nil, tabloidCtx{OutDir: dir})
	if !strings.Contains(md, "![TSK-1 one.png](<ev/one.png>)") {
		t.Errorf("relative image missing:\n%s", md)
	}
	if n := strings.Count(md, "!["); n != tabloidImageCap {
		t.Errorf("%d images, want %d", n, tabloidImageCap)
	}
	// review items show their files; release work links one; a done item with only a log does not
	if !strings.Contains(md, "**a/TSK-3**") || !strings.Contains(md, "**a/TSK-4**") || strings.Contains(md, "**a/TSK-2** Log only\n\n") {
		t.Errorf("selection:\n%s", md)
	}
	if strings.Contains(md, "extra.txt") || strings.Count(md, `[log\.txt]`) != 2 { // review item + the one release link
		t.Errorf("release work must link one file:\n%s", md)
	}
	if strings.Contains(md, "TSK-1** Shot\n\n- [") { // the shot's text file is not proof
		t.Errorf("a plain text file is not proof:\n%s", md)
	}
	pr := md[strings.Index(md, "### Proof"):]
	if strings.Index(pr, "a/TSK-3") > strings.Index(pr, "a/TSK-1** Shot") {
		t.Errorf("review proof should come first")
	}
	// no ctx: absolute path; html embeds <img>
	if abs := renderTabloidMD(m, nil, tabloidCtx{}); !strings.Contains(abs, "(<"+filepath.ToSlash(dir)) {
		t.Errorf("absolute path expected:\n%s", abs)
	}
	h := bulletinHTMLBlock(m, nil, tabloidCtx{OutDir: dir})
	if !strings.Contains(h, `<img src="ev/one.png" alt="TSK-1 one.png">`) {
		t.Errorf("html img:\n%s", h)
	}
	// nothing qualifies: no section
	m2 := tabModel()
	m2.Happened.Done = []newsItem{logOnly}
	if md := renderTabloidMD(m2, nil, tabloidCtx{}); strings.Contains(md, "### Proof") {
		t.Errorf("proof section without proof:\n%s", md)
	}
}

func TestTabloidProofFromBoard(t *testing.T) {
	path := newsBoard(t)
	w, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	var ten *Tenant
	for i := range w.Tenants {
		if w.Tenants[i].Slug == "news-app" {
			ten = &w.Tenants[i]
		}
	}
	it := Item{ID: "TSK-77", Evidence: []string{"a/shot.PNG 12", "notes.txt 3", "gone.png 5", "../x.png 1"}}
	dir := evidenceFolder(w, ten, it.ID)
	t.Setenv("HOME", t.TempDir()) // evidence lives under the home folder by default
	dir = evidenceFolder(w, ten, it.ID)
	if err := os.MkdirAll(filepath.Join(dir, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "a", "shot.PNG"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644)
	got := proofFor(w, ten, it)
	if len(got) != 2 || !got[0].Image || !strings.HasSuffix(got[0].Path, "a/shot.PNG") || got[1].Image {
		t.Fatalf("proofFor: %+v", got)
	}
}

func TestTabloidEscaping(t *testing.T) {
	bad := "a | b <i>x</i> `c` [l](http://e) & # d"
	m := tabModel()
	m.Job.Title = bad
	it := di("t", "TSK-1", bad)
	it.Proof = []newsProof{{Path: "/tmp/x/we ird.png", Image: true}, {Path: "/tmp/<bad>.png", Image: true}}
	m.Happened.Done = []newsItem{it}
	m.Now.Needs = []newsItem{it}
	md := renderTabloidMD(m, nil, tabloidCtx{})
	if strings.Contains(md, "<i>") || strings.Contains(md, " [l](") || strings.Contains(md, "<bad>") || !strings.Contains(md, `a \| b \<i\>x\</i\> \`+"`c\\` ") {
		t.Errorf("md not escaped:\n%s", md)
	}
	if !strings.Contains(md, "![TSK-1 we ird.png](</tmp/x/we ird.png>)") {
		t.Errorf("path with a space:\n%s", md)
	}
	h := bulletinHTMLBlock(m, nil, tabloidCtx{})
	if strings.Contains(h, "<i>") || !strings.Contains(h, "&lt;i&gt;x&lt;/i&gt;") || strings.Contains(h, "<bad>") {
		t.Errorf("html not escaped:\n%s", h)
	}
}

func TestTabloidLeadIsNewestRelease(t *testing.T) {
	m := tabModel()
	newer, older := di("a", "CHR-2", "Cut 0.3.9"), di("a", "CHR-1", "Cut 0.3.8")
	newer.Release, older.Release = true, true
	newer.Since, older.Since = "2026-10-25T09:00:00Z", "2026-10-23T09:00:00Z"
	m.Happened.Done = []newsItem{newer, older} // the model lists the newest first
	if md := renderTabloidMD(m, nil, tabloidCtx{}); !strings.Contains(md, "\n## Done: Cut 0.3.9\n") {
		t.Errorf("the newest release leads:\n%s", md)
	}
	if releaseWork(Item{Kind: "bug", Title: "Release 1", Evidence: []string{"release.txt 3"}}) || releaseWork(Item{Kind: "chore", Title: "Release 1"}) {
		t.Error("release work needs a chore with a release-notes file")
	}
	if !releaseWork(Item{Kind: "chore", Title: "x", Evidence: []string{"release.txt 3"}}) || !releaseWork(Item{Kind: "chore", Evidence: []string{"sub/Release-Notes.md 9"}}) {
		t.Error("a chore with release.txt or release-notes.md is release work")
	}
}

func TestTabloidNoByLocalAndFullTitles(t *testing.T) {
	m := tabModel()
	it := di("a", "TSK-1", "Done by the implicit session")
	it.By = localName
	m.Happened.Done = []newsItem{it}
	long := strings.Repeat("word ", 30)
	m.Now.Counts.Doing = 1
	m.Now.Doing = []newsItem{di("a", "TSK-2", long)}
	md := renderTabloidMD(m, nil, tabloidCtx{})
	if strings.Contains(md, "by local") {
		t.Errorf("by local shown:\n%s", md)
	}
	if strings.Contains(md, "~") {
		t.Errorf("tilde truncation:\n%s", md)
	}
	if !strings.Contains(md, "…") || !strings.Contains(md, "word word word") {
		t.Errorf("coming up wants long titles cut with …:\n%s", md)
	}
}

func TestTabloidQuietJobsOmitted(t *testing.T) {
	quiet, busy := tabModel(), tabModel()
	quiet.Job.Title, busy.Job.Title = "Idle", "Busy"
	quiet.Release.Changes = 0
	busy.Happened.Done = []newsItem{di("a", "TSK-1", "Shipped")}
	if tabloidQuiet(busy) || !tabloidQuiet(quiet) {
		t.Fatal("quiet rule")
	}
	busy.Now.Counts.Doing = 1
	if tabloidQuiet(busy) {
		t.Fatal("work in progress is not quiet")
	}
}

func TestNewsMultiJobOmitsQuietJobs(t *testing.T) {
	inTempDir(t)
	runAll(t, []string{"init", "Two"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"}, []string{"tenant", "add", "b", "B"},
		[]string{"jobs", "enable", "busy", "-t", "two"}, []string{"jobs", "enable", "idle", "-t", "b"})
	all := captureOutput(t, func() { runAll(t, []string{"news", "-o", "-"}) })
	if !strings.Contains(all, "busy News") || !strings.Contains(all, "idle News") {
		t.Errorf("every job quiet: all stay:\n%s", all)
	}
	runAll(t, []string{"add", "task", "work", "-S", "ready", "-t", "two"}, []string{"start", "TSK-001", "-t", "two"}, []string{"done", "TSK-001", "ok", "--no-evidence", "x", "-t", "two"})
	some := captureOutput(t, func() { runAll(t, []string{"news", "-o", "-"}) })
	if !strings.Contains(some, "busy News") || strings.Contains(some, "idle News") {
		t.Errorf("the quiet job must be omitted:\n%s", some)
	}
	one := captureOutput(t, func() { runAll(t, []string{"news", "--job", "idle", "-o", "-"}) })
	if !strings.Contains(one, "idle News") {
		t.Errorf("a single requested job always shows:\n%s", one)
	}
}
