package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// mw board --md: a readable Markdown "jobs" view over every job of the board (the file the Claude desktop
// app shows in its file pane). Plain headings, lists and one small table per job: nothing wide, nothing that
// garbles when wrapped. It reuses the news model (full, uncapped) so the numbers equal mw news.

const (
	jobsMDCapList  = 30 // rows per section before "... N more"
	jobsMDCapReady = 10
	jobsMDCapDone  = 10
	jobsMDBarWidth = 10
)

// mdText collapses whitespace and backslash-escapes the characters Markdown would act on.
func mdText(s string) string {
	var b strings.Builder
	for _, r := range strings.Join(strings.Fields(s), " ") {
		switch r {
		case '\\', '`', '*', '_', '[', ']', '<', '>', '|', '~', '&':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// mdTitle is an item title for a list row: at most 80 characters, then "…".
func mdTitle(s string) string { return mdText(compactText(s, 80)) }

// jobsMDValidURL accepts a published-page URL for a link: http or https, no spaces, parentheses or control characters.
func jobsMDValidURL(u string) bool {
	if !(strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://")) || len(u) > 2000 || strings.ContainsAny(u, " ()<>\"'`\\") {
		return false
	}
	return noControl(u)
}

// mdWhen is HH:MM for a time of today, "Mon DD HH:MM" for an older one, so events of mixed days read right.
func mdWhen(t, now time.Time) string {
	t, now = t.Local(), now.Local()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04")
	}
	return t.Format("Jan 02 15:04")
}

// mdAge is a coarse age (<1h, Nh, Nd) so the file changes at most hourly while nothing else happens.
func mdAge(since string, now time.Time) string {
	t, err := time.Parse(time.RFC3339Nano, since)
	if err != nil {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < 5*time.Minute:
		return "<5m"
	case d < time.Hour: // 5-minute steps, so the file does not change every minute
		return fmt.Sprintf("%dm", int(d.Minutes())/5*5)
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func mdAgeTail(it newsItem, now time.Time) string {
	if a := mdAge(it.Since, now); a != "" {
		return " · " + a
	}
	return ""
}

func mdBar(done, total int) string {
	fill := 0
	if total > 0 {
		fill = done * jobsMDBarWidth / total
	}
	return fmt.Sprintf("`%s%s` %d/%d", strings.Repeat("#", fill), strings.Repeat("-", jobsMDBarWidth-fill), done, total)
}

const jobsMDStalled = 45 * time.Minute

func mdStalled(it newsItem, now time.Time) bool {
	t, err := time.Parse(time.RFC3339Nano, it.Since)
	return err == nil && now.Sub(t) > jobsMDStalled
}

func mdMeta(it newsItem, now time.Time, extra ...string) string {
	var p []string
	if it.Tenant != "" {
		p = append(p, mdText(it.Tenant))
	}
	p = append(p, mdText(it.ID))
	p = append(p, extra...)
	if a := mdAge(it.Since, now); a != "" {
		p = append(p, a)
	}
	return strings.Join(p, " · ")
}

// jobsMDSummary is one job's segment of the status line: "! tiny 2 need you · C2 5/6" or "demo ▸ D1 1/2".
func jobsMDSummary(id string, m *newsModel) string {
	var waves []string
	for i, w := range m.Now.Milestones.Open {
		if i == 3 {
			break
		}
		waves = append(waves, fmt.Sprintf("%s %d/%d", mdText(w.ID), w.Done, w.Total))
	}
	name := id
	if name == "" {
		name = "board"
	}
	name = mdText(name)
	c := m.Now.Counts
	if c.Needs > 0 {
		return strings.Join(append([]string{fmt.Sprintf("! %s %d need you", name, c.Needs)}, waves...), " · ")
	}
	mark := ""
	if c.Review > 0 {
		mark = "◐ "
	}
	if len(waves) == 0 { // "quiet" only when nothing is going on
		switch {
		case c.Doing > 0:
			waves = []string{fmt.Sprintf("%d in progress", c.Doing)}
		case c.Blocked > 0:
			waves = []string{fmt.Sprintf("%d blocked", c.Blocked)}
		default:
			waves = []string{"quiet"}
		}
	}
	return mark + name + " ▸ " + strings.Join(waves, " · ")
}

func mdMetrics(b *strings.Builder, m *newsMetrics) {
	if m == nil || len(m.Sections) == 0 {
		return
	}
	for _, s := range m.Sections {
		fmt.Fprintf(b, "\n### Harness · %s\n\n", mdText(s.Title))
		for _, r := range s.Rows {
			var p []string
			delta := func(key string) string {
				d, ok := r.Delta[key]
				switch {
				case !ok || d == 0:
					return ""
				case d > 0:
					return fmt.Sprintf(" ▲%g", d)
				}
				return fmt.Sprintf(" ▼%g", -d)
			}
			switch {
			case s.Kind == "split":
				lg := newsLegend{"pass", "warn", "fail", "skip"}
				if s.Legend != nil {
					lg = *s.Legend
				}
				for _, x := range []struct {
					l string
					n int
					k string
				}{{lg.Good, r.Good, "good"}, {lg.Warn, r.Warn, "warn"}, {lg.Bad, r.Bad, "bad"}, {lg.None, r.None, "none"}} {
					if x.n > 0 {
						p = append(p, fmt.Sprintf("%s %d%s", mdText(x.l), x.n, delta(x.k)))
					}
				}
			case r.Total > 0 || r.Done > 0:
				p = append(p, fmt.Sprintf("%d/%d%s", r.Done, r.Total, delta("done")))
			case r.Value != nil:
				p = append(p, fmt.Sprintf("%g %s%s", *r.Value, mdText(r.Unit), delta("value")))
			}
			fmt.Fprintf(b, "- **%s** %s\n", mdText(r.Name), strings.Join(p, " · "))
		}
	}
}

// jobsMDInput is what one job's section needs besides its news model.
type jobsMDInput struct {
	id, title   string
	m           *newsModel
	tenants     []*Tenant
	newsLink    string // relative link to the news file, "" when none
	artifactURL string
}

// renderJobsMD renders one job (id "" = the workspace) in the order of the jobs-panel design: headline, needs
// you, review, in progress, milestones, harness, last events. Empty sections are left out; the file's one
// footer comes from jobsMDFooter.
func renderJobsMD(in jobsMDInput, now time.Time) string {
	m := in.m
	c := m.Now.Counts
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", mdText(in.title))
	stalled := 0
	for _, it := range m.Now.Doing {
		if mdStalled(it, now) {
			stalled++
		}
	}
	switch {
	case c.Needs > 0:
		fmt.Fprintf(&b, "**! %d need you**\n", c.Needs)
	case c.Review > 0:
		fmt.Fprintf(&b, "**◐ %d awaiting review**\n", c.Review)
	case c.Blocked > 0:
		fmt.Fprintf(&b, "**⏱ %d blocked**\n", c.Blocked)
	case c.Doing > 0:
		fmt.Fprintf(&b, "**● %d in progress**\n", c.Doing)
	default:
		b.WriteString("**✓ all quiet**\n")
	}
	var sub []string
	if c.Needs > 0 && c.Review > 0 {
		sub = append(sub, fmt.Sprintf("◐ %d awaiting review", c.Review))
	}
	if stalled > 0 {
		sub = append(sub, fmt.Sprintf("⏱ %d stalled", stalled))
	}
	if c.Blocked > 0 && (c.Needs > 0 || c.Review > 0) {
		sub = append(sub, fmt.Sprintf("⏱ %d blocked", c.Blocked))
	}
	if c.Doing > 0 && c.Needs+c.Review > 0 || len(sub) > 0 && c.Doing > 0 {
		sub = append(sub, fmt.Sprintf("%d in progress", c.Doing))
	}
	if len(sub) > 0 {
		b.WriteString("\n" + strings.Join(sub, " · ") + "\n")
	}
	if len(m.Now.Needs) > 0 {
		b.WriteString("\n### Needs you\n\n")
		for i, it := range m.Now.Needs {
			if i == jobsMDCapList {
				fmt.Fprintf(&b, "- … %d more\n", len(m.Now.Needs)-i)
				break
			}
			fmt.Fprintf(&b, "- **! %s**\n  %s\n", mdTitle(it.Title), mdMeta(it, now))
			if it.Note != "" {
				fmt.Fprintf(&b, "  - %s\n", mdText(compactText(it.Note, 140)))
			}
		}
	}
	if len(m.Now.Review) > 0 {
		b.WriteString("\n### Review\n\n")
		for i, it := range m.Now.Review {
			if i == jobsMDCapList {
				fmt.Fprintf(&b, "- … %d more\n", len(m.Now.Review)-i)
				break
			}
			fmt.Fprintf(&b, "- ◐ **%s** %s\n  %s\n", mdText(it.ID), mdTitle(it.Title), mdMeta(it, now, mdText(compactText(it.Note, 80))))
		}
	}
	if len(m.Now.Doing)+len(m.Now.Blocked) > 0 {
		b.WriteString("\n### In progress\n\n")
		n := 0
		row := func(it newsItem, sym, tag string) {
			if n == jobsMDCapList {
				fmt.Fprintf(&b, "- … more\n")
			}
			n++
			if n > jobsMDCapList {
				return
			}
			age := mdAge(it.Since, now)
			fmt.Fprintf(&b, "- %s **%s** %s · %s%s\n", sym, mdText(it.ID), mdTitle(it.Title), mdText(it.Tenant), tag+map[bool]string{true: " · " + age}[age != ""])
			if tag != "" && it.Note != "" {
				fmt.Fprintf(&b, "  - %s\n", mdText(compactText(it.Note, 140)))
			}
		}
		for _, it := range m.Now.Doing {
			if mdStalled(it, now) {
				row(it, "⏱", " · stalled")
			} else {
				row(it, "●", "")
			}
		}
		for _, it := range m.Now.Blocked {
			row(it, "⏱", " · blocked")
		}
	}
	if n := len(m.Next.Ready); n > 0 {
		b.WriteString("\n### Ready\n\n")
		for i, it := range m.Next.Ready {
			if i == jobsMDCapReady {
				fmt.Fprintf(&b, "- … %d more\n", n-i+m.Next.More["ready"])
				break
			}
			fmt.Fprintf(&b, "- **%s** %s · %s%s\n", mdText(it.ID), mdTitle(it.Title), mdText(it.Tenant), mdAgeTail(it, now))
		}
		if n <= jobsMDCapReady && m.Next.More["ready"] > 0 {
			fmt.Fprintf(&b, "- … %d more\n", m.Next.More["ready"])
		}
	}
	if rd := mdRecentDone(m.Now.RecentDone, now); len(rd) > 0 {
		b.WriteString("\n### Recently done\n\n")
		for _, it := range rd {
			fmt.Fprintf(&b, "- ✓ **%s** %s · %s%s\n", mdText(it.ID), mdTitle(it.Title), mdText(it.Tenant), mdAgeTail(it, now))
		}
	}
	var closed []string
	for _, t := range in.tenants {
		for _, it := range t.Items {
			if it.Kind == "wave" && it.Status == "done" {
				closed = append(closed, mdText(it.ID))
			}
		}
	}
	if ms := m.Now.Milestones.Open; len(ms) > 0 || len(closed) > 0 {
		b.WriteString("\n### Milestones\n\n")
		for _, w := range ms {
			fmt.Fprintf(&b, "- **%s** %s · %s\n  %s\n", mdText(w.ID), mdTitle(w.Title), mdText(w.Tenant), mdBar(w.Done, w.Total))
		}
		if len(closed) > 0 {
			shownN := closed
			if len(shownN) > 6 {
				shownN = shownN[:6]
			}
			fmt.Fprintf(&b, "- ✓ %d closed — %s\n", len(closed), strings.Join(shownN, ", "))
		}
	}
	mdMetrics(&b, m.Metrics)
	if rows := mdEventRows(in.tenants); len(rows) > 0 {
		b.WriteString("\n### Last events\n\n")
		for i, r := range rows {
			if i == 6 {
				break
			}
			text := strings.TrimSpace(r.Tenant + " " + r.Item + " " + r.Action + " " + compactText(r.Note, 70))
			fmt.Fprintf(&b, "- `%s` %s\n", mdWhen(r.sortKey, now), mdText(text))
		}
	}
	return b.String()
}

// mdRecentDone is what was done today (at most jobsMDCapDone), or, when nothing was done today, the last 5.
func mdRecentDone(done []newsItem, now time.Time) []newsItem {
	var today []newsItem
	for _, it := range done {
		if t, err := time.Parse(time.RFC3339Nano, it.Since); err == nil {
			if t, n := t.Local(), now.Local(); t.Year() == n.Year() && t.YearDay() == n.YearDay() {
				today = append(today, it)
			}
		}
	}
	switch {
	case len(today) > jobsMDCapDone:
		return today[:jobsMDCapDone]
	case len(today) > 0:
		return today
	case len(done) > 5:
		return done[:5]
	}
	return done
}

// mdEventRows are the tenants' events, newest first, without the "created" ones.
func mdEventRows(tenants []*Tenant) []logRow {
	var rows []logRow
	for _, t := range tenants {
		for _, r := range tenantRows(t) {
			if r.Action != "created" {
				rows = append(rows, r)
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].sortKey.After(rows[j].sortKey) })
	return rows
}

// jobsMDFooter is the one footer of a file: when it was last updated (newest event of any job), then a link to
// each job's own news file and published page.
func jobsMDFooter(ins []jobsMDInput, now time.Time) string {
	var newest time.Time
	for _, in := range ins {
		if rows := mdEventRows(in.tenants); len(rows) > 0 && rows[0].sortKey.After(newest) {
			newest = rows[0].sortKey
		}
	}
	var foot []string
	if !newest.IsZero() {
		foot = append(foot, "updated "+mdWhen(newest, now))
	}
	for _, in := range ins {
		who := ""
		if len(ins) > 1 && in.id != "" {
			who = " · " + mdText(in.id)
		}
		if in.newsLink != "" {
			foot = append(foot, fmt.Sprintf("[News%s](<%s>)", who, in.newsLink))
		}
		if in.artifactURL != "" {
			foot = append(foot, fmt.Sprintf("[Published page%s](%s)", who, in.artifactURL))
		}
	}
	if len(foot) == 0 {
		return ""
	}
	return "\n---\n" + strings.Join(foot, " · ") + "\n"
}

// jobsMDCommand implements mw board --md [--job ID] [-o PATH|-].
func jobsMDCommand(path string, w *Workspace, o map[string]string) error {
	root := filepath.Dir(path)
	var ids []string
	if j := o["job"]; j != "" {
		if _, ok := w.Jobs[j]; !ok {
			return fmt.Errorf("job %s not found (mw jobs show)", j)
		}
		ids = []string{j}
	} else {
		for id := range w.Jobs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
	}
	if len(ids) == 0 {
		ids = []string{""}
	}
	now := time.Now()
	type doc struct {
		in      jobsMDInput
		target  string
		newsAbs string // this job's own status.md
		dir     string // this job's own news folder
	}
	var docs []doc
	var segs []string
	stdout := o["out"] == "-"
	for _, id := range ids {
		p := newsPart{job: id, title: w.Name, tenants: newsJobTenants(w, id)}
		dir := "docs/news/"
		var metrics *newsMetrics
		win := newsWindow{full: true}
		if id != "" {
			v := jobView(w, id)
			p.title, p.desc, p.url, dir = v.Title, v.Description, v.NewsURL, v.NewsOut
			if v.Metrics != "" {
				metrics = newsMetricsLoad(root, v.Metrics)
			}
			if rd, err := newsReleaseDir(root, v); err == nil && rd != "" {
				if prev, num, _, err := newsPreviousRelease(rd, false); err == nil {
					win.prev, win.prevNum = prev, num
				}
			}
		}
		m := buildNewsModel(w, p, metrics, win)
		d := doc{in: jobsMDInput{id: id, title: p.title, m: m, tenants: p.tenants}}
		if cd, err := newsConfined(root, "news.out", dir); err == nil {
			d.dir, d.newsAbs = cd, filepath.Join(cd, "status.md")
		}
		switch {
		case stdout:
		case o["out"] != "":
			var err error
			if d.target, err = newsOutPath(path, root, o["out"]); err != nil {
				return err
			}
		default:
			cd, err := newsConfined(root, "news.out", dir)
			if err != nil {
				return err
			}
			d.target = filepath.Join(cd, "jobs.md")
		}
		segs = append(segs, jobsMDSummary(id, m))
		docs = append(docs, d)
	}
	build := func(ds []doc) string {
		var b strings.Builder
		fmt.Fprintf(&b, "# %s board\n\n%s\n", mdText(w.Name), strings.Join(segs, " │ "))
		var ins []jobsMDInput
		for _, d := range ds {
			if d.target != "" && d.newsAbs != "" { // the link is computed from this job's own news file
				if rel, err := filepath.Rel(filepath.Dir(d.target), d.newsAbs); err == nil {
					d.in.newsLink = filepath.ToSlash(rel)
				}
			}
			if d.dir != "" {
				if bs, err := os.ReadFile(filepath.Join(d.dir, "jobs.artifact-url")); err == nil {
					if u := strings.TrimSpace(string(bs)); jobsMDValidURL(u) {
						d.in.artifactURL = u
					}
				}
			}
			b.WriteString("\n" + renderJobsMD(d.in, now))
			ins = append(ins, d.in)
		}
		b.WriteString(jobsMDFooter(ins, now))
		return b.String()
	}
	if o["out"] != "" { // one file (or stdout) holding every job
		body := build(docs)
		if stdout {
			fmt.Print(body)
			return nil
		}
		return jobsMDPut(docs[0].target, body, now)
	}
	// one file per target: jobs that point at one file share one combined document
	var order []string
	byTarget := map[string][]doc{}
	for _, d := range docs {
		if _, ok := byTarget[d.target]; !ok {
			order = append(order, d.target)
		}
		byTarget[d.target] = append(byTarget[d.target], d)
	}
	for _, t := range order {
		if err := jobsMDPut(t, build(byTarget[t]), now); err != nil {
			return err
		}
	}
	return nil
}

func jobsMDPut(target, body string, now time.Time) error {
	if target == "" {
		return errors.New("no target")
	}
	wrote, err := newsWrite(target, body, now)
	if err != nil {
		return err
	}
	if wrote {
		fmt.Println("wrote", target)
	} else {
		fmt.Println("unchanged", target)
	}
	return nil
}
