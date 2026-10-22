package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// mw news: a status page per coordinated job (or for the whole workspace when no job exists), built from
// board data only. It reuses publishHTML's page head and escaping and its sidecar hash (<page>.hash), but
// the hash leaves out the "Updated" line and the moved-since-last-run count, so an unchanged board writes
// nothing. No network, no --publish.

const (
	newsMaxEvents  = 20
	newsMaxDecides = 10
	newsListCap    = 25    // rows per needs/blocked/doing list, then "... N more"
	newsMilestones = 30    // rows of the milestone table
	newsMDLimit    = 9500  // characters of Markdown per job (the pane's Markdown limit)
	newsSummaryCap = 20480 // bytes read from the summary file

	newsStampMarker = "\x00stamp\x00" // replaced by the generated-at time when writing, by "" when hashing
	newsMovedMarker = "\x00moved\x00" // replaced by "; N moved since the last run", by "" when hashing
)

// newsMD escapes text for a Markdown line or table cell: control characters and newlines are dropped, < > & become
// entities and every other ASCII punctuation character is backslash-escaped, so no link, image, autolink,
// emphasis or table syntax survives (escaping ":" also defuses bare URLs).
func newsMD(s string) string {
	var b strings.Builder
	for _, r := range newsClean(s, false) {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case r < 0x80 && unicode.IsPunct(r) || r < 0x80 && unicode.IsSymbol(r):
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

type newsPart struct {
	job     string // job id, "" for the workspace
	title   string
	desc    string // job description, one line; "" when none
	url     string
	tenants []*Tenant
	summary string // file contents, already capped; "" when none
}

type newsLine struct{ ID, Tenant, Title, Detail string }

type newsMile struct{ ID, Tenant, Title, Progress, State string }

type newsEvent struct {
	row   logRow
	title string
}

type newsData struct {
	needs, blocked, doing []newsLine
	miles                 []newsMile
	events, decisions     []newsEvent
	done, total, moved    int
	hasPrev               bool
}

// newsJobTenants lists the visible tenants of a job (all visible tenants when id is "").
func newsJobTenants(w *Workspace, id string) []*Tenant {
	var out []*Tenant
	for i := range w.Tenants {
		t := &w.Tenants[i]
		if t.Hidden || (id != "" && t.Job != id) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// newsConfined resolves a workspace-relative path to a real path that stays inside the workspace after
// symlinks are resolved. The deepest existing ancestor is resolved, so a not-yet-created target is judged
// by where its parent really lives.
func newsConfined(root, key, rel string) (string, error) {
	c, err := workspaceRelPath(key, rel)
	if err != nil {
		return "", err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	probe := filepath.Join(realRoot, filepath.FromSlash(strings.TrimSuffix(c, "/")))
	var tail []string
	for {
		if r, e := filepath.EvalSymlinks(probe); e == nil {
			probe = r
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", fmt.Errorf("%s: %q cannot be resolved", key, rel)
		}
		tail = append([]string{filepath.Base(probe)}, tail...)
		probe = parent
	}
	real := filepath.Join(append([]string{probe}, tail...)...)
	if real != realRoot && !strings.HasPrefix(real, realRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("%s: %q leaves the workspace (a symlink points outside it)", key, rel)
	}
	return real, nil
}

// newsSummary reads the job's summary file as data: confined, a regular file, at most 20 KB.
func newsSummary(root, rel string) (string, error) {
	p, err := newsConfined(root, "summary", rel)
	if err != nil {
		return "", err
	}
	if fi, err := os.Stat(p); err == nil && !fi.Mode().IsRegular() {
		return "", nil // a FIFO or device would block the open under the board lock
	}
	f, err := openNoBlock(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		return "", nil
	}
	b, err := io.ReadAll(io.LimitReader(f, newsSummaryCap+1))
	if err != nil {
		return "", err
	}
	s := string(b)
	if len(b) > newsSummaryCap {
		s = strings.ToValidUTF8(string(b[:newsSummaryCap]), "") + "\n[truncated at 20 KB]"
	}
	return strings.ReplaceAll(strings.TrimSpace(s), "\r\n", "\n"), nil
}

func newsCollect(p newsPart, since time.Time) newsData {
	c := func(s string) string { return newsClean(s, false) }
	var d newsData
	d.hasPrev = !since.IsZero()
	var rows []newsEvent
	for _, t := range p.tenants {
		title := map[string]string{}
		for _, it := range t.Items {
			title[it.ID] = c(it.Title)
			if it.Kind == "wave" {
				dn := 0
				m := waveMembers(t, it.ID)
				for _, x := range m {
					if x.Status == "done" {
						dn++
					}
				}
				d.miles = append(d.miles, newsMile{it.ID, t.Slug, c(it.Title), fmt.Sprintf("%d/%d", dn, len(m)), it.Status})
				continue
			}
			d.total++
			if it.Status == "done" {
				d.done++
			}
			switch it.Status {
			case "blocked":
				reason := ""
				for _, e := range it.Events {
					if e.Action == "block" {
						reason = c(e.Note)
					}
				}
				d.blocked = append(d.blocked, newsLine{it.ID, t.Slug, c(it.Title), reason})
			case "doing":
				who := ""
				if it.Assignee != "" {
					who = "@" + displayName(it.Assignee)
				}
				d.doing = append(d.doing, newsLine{it.ID, t.Slug, c(it.Title), who})
			}
			if e, ok := openAsk(it); ok {
				by := ""
				if e.By != "" {
					by = "@" + displayName(e.By)
				}
				d.needs = append(d.needs, newsLine{it.ID, t.Slug, c(it.Title), c(strings.TrimSpace(e.Note + " " + by))})
			}
		}
		for _, r := range tenantRows(t) {
			rows = append(rows, newsEvent{newsCleanRow(r), title[r.Item]})
			if d.hasPrev && r.sortKey.After(since) {
				d.moved++
			}
		}
		d.total += t.Archived
		d.done += t.Archived
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if !rows[i].row.sortKey.Equal(rows[j].row.sortKey) {
			return rows[i].row.sortKey.After(rows[j].row.sortKey)
		}
		return rows[i].row.At > rows[j].row.At
	})
	for _, e := range rows {
		if len(d.events) < newsMaxEvents {
			d.events = append(d.events, e)
		}
		switch e.row.Action {
		case "decide", "accept", "reject":
			if len(d.decisions) < newsMaxDecides {
				d.decisions = append(d.decisions, e)
			}
		}
	}
	for i, j := 0, len(d.decisions)-1; i < j; i, j = i+1, j-1 { // decisions read oldest first
		d.decisions[i], d.decisions[j] = d.decisions[j], d.decisions[i]
	}
	return d
}

func newsHeadline(d newsData) string {
	parts := []string{"nothing needs you"}
	if n := len(d.needs); n > 0 {
		parts[0] = fmt.Sprintf("%d need you", n)
	}
	if n := len(d.blocked); n > 0 {
		parts = append(parts, fmt.Sprintf("%d blocked", n))
	}
	parts = append(parts, fmt.Sprintf("%d in progress", len(d.doing)), fmt.Sprintf("%d of %d done", d.done, d.total))
	return strings.Join(parts, "; ") + newsMovedMarker
}

func newsEventText(e newsEvent) string {
	s := e.row.Action + ": " + e.row.Item
	if e.title != "" {
		s += " " + compactText(e.title, 70)
	}
	s += " (" + e.row.Tenant + ")"
	if e.row.By != "" {
		s += " @" + displayName(e.row.By)
	}
	if e.row.Note != "" {
		s += " - " + compactText(e.row.Note, 90)
	}
	return s
}

func newsRowText(l newsLine, multi bool) string {
	s := l.ID + " " + compactText(l.Title, 90)
	if multi {
		s += " (" + l.Tenant + ")"
	}
	if l.Detail != "" {
		s += ": " + compactText(l.Detail, 140)
	}
	return s
}

func newsFence(s string) string {
	fence, run := "```", 0
	for _, r := range s {
		if r == '`' {
			run++
			if run >= len(fence) {
				fence = strings.Repeat("`", run+1)
			}
		} else {
			run = 0
		}
	}
	return fence
}

// renderNewsMD renders one job's Markdown with the given number of events and summary bytes.
func renderNewsMD(p newsPart, d newsData, events, summaryMax int) string {
	var b strings.Builder
	multi := len(p.tenants) > 1
	fmt.Fprintf(&b, "# %s\n", newsMD(compactText(p.title, 72)))
	if p.desc != "" {
		fmt.Fprintf(&b, "_%s_\n", newsMD(p.desc))
	}
	fmt.Fprintf(&b, "Updated %s\n", newsStampMarker)
	if p.url != "" {
		fmt.Fprintf(&b, "Page: %s\n", newsMD(p.url))
	}
	fmt.Fprintf(&b, "\n## Headline\n%s.\n", newsMD(strings.TrimSuffix(newsHeadline(d), newsMovedMarker))+newsMovedMarker)
	list := func(head string, rows []newsLine) {
		fmt.Fprintf(&b, "\n## %s\n", head)
		if len(rows) == 0 {
			b.WriteString("- Nothing.\n")
		}
		for i, r := range rows {
			if i == newsListCap {
				fmt.Fprintf(&b, "- ... %d more\n", len(rows)-i)
				break
			}
			fmt.Fprintf(&b, "- %s\n", newsMD(newsRowText(r, multi)))
		}
	}
	list("Needs you", d.needs)
	list("Blocked", d.blocked)
	list("In progress", d.doing)
	b.WriteString("\n## Milestones\n")
	if len(d.miles) == 0 {
		b.WriteString("- None.\n")
	} else {
		b.WriteString("| Milestone | Tenant | Done/total | State |\n|---|---|---|---|\n")
		for i, m := range d.miles {
			if i == newsMilestones {
				fmt.Fprintf(&b, "| ... %d more | | | |\n", len(d.miles)-i)
				break
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", newsMD(m.ID+" "+compactText(m.Title, 60)), newsMD(m.Tenant), m.Progress, newsMD(m.State))
		}
	}
	b.WriteString("\n## Decisions\n")
	if len(d.decisions) == 0 {
		b.WriteString("- None.\n")
	}
	for _, e := range d.decisions {
		fmt.Fprintf(&b, "- %s %s\n", shortTS(e.row.At), newsMD(newsEventText(e)))
	}
	if events > 0 {
		b.WriteString("\n## Last events\n")
		for i, e := range d.events {
			if i == events {
				break
			}
			fmt.Fprintf(&b, "- %s %s\n", shortTS(e.row.At), newsMD(newsEventText(e)))
		}
	}
	if summaryMax > 0 && p.summary != "" {
		s := p.summary
		if len(s) > summaryMax {
			s = strings.ToValidUTF8(s[:summaryMax], "") + "\n[trimmed]"
		}
		f := newsFence(s)
		fmt.Fprintf(&b, "\n## Summary\n%stext\n%s\n%s\n", f, s, f)
	}
	return b.String()
}

// newsMDFit renders one job and trims it under the 9,500-character limit: events first, then the summary.
func newsMDFit(p newsPart, d newsData) string {
	// the markers expand to at most this many characters when the file is written
	fits := func(s string) bool {
		s = strings.ReplaceAll(s, newsStampMarker, "2006-01-02 15:04 local")
		s = strings.ReplaceAll(s, newsMovedMarker, "; 99999 moved since the last run")
		return len([]rune(s)) <= newsMDLimit
	}
	out := ""
	for ev := len(d.events); ev >= 0; ev-- {
		if out = renderNewsMD(p, d, ev, newsSummaryCap); fits(out) {
			return out
		}
	}
	for sm := len(p.summary) / 2; sm >= 256; sm /= 2 {
		if out = renderNewsMD(p, d, 0, sm); fits(out) {
			return out
		}
	}
	if out = renderNewsMD(p, d, 0, 0); fits(out) {
		return out
	}
	cut := string([]rune(out)[:newsMDLimit-120])
	if i := strings.LastIndex(cut, "\n"); i > 0 {
		cut = cut[:i]
	}
	return cut + "\n- ... trimmed to fit the limit\n"
}

func renderNewsHTML(parts []newsPart, datas []newsData, title string) string {
	var b strings.Builder
	esc := html.EscapeString
	b.WriteString(strings.Replace(htmlHead, "<title>miniwaves board</title>", "<title>"+esc(compactText(title, 72))+"</title>", 1))
	fmt.Fprintf(&b, "<h1>%s</h1>\n<div class=mut>Updated %s</div>\n", esc(compactText(title, 72)), newsStampMarker)
	for i, p := range parts {
		d := datas[i]
		multi := len(p.tenants) > 1
		fmt.Fprintf(&b, "<section><h2>%s</h2>\n", esc(compactText(p.title, 72)))
		if p.desc != "" {
			fmt.Fprintf(&b, "<div><em>%s</em></div>\n", esc(p.desc))
		}
		if p.url != "" {
			fmt.Fprintf(&b, "<div class=mut>%s</div>\n", esc(p.url))
		}
		fmt.Fprintf(&b, "<h3>Headline</h3><div>%s.</div>\n", esc(newsHeadline(d)))
		list := func(head string, rows []newsLine) {
			fmt.Fprintf(&b, "<h3>%s (%d)</h3><ul>\n", head, len(rows))
			for j, r := range rows {
				if j == newsListCap {
					fmt.Fprintf(&b, "<li class=mut>... %d more</li>\n", len(rows)-j)
					break
				}
				fmt.Fprintf(&b, "<li>%s</li>\n", esc(newsRowText(r, multi)))
			}
			b.WriteString("</ul>\n")
		}
		list("Needs you", d.needs)
		list("Blocked", d.blocked)
		list("In progress", d.doing)
		b.WriteString("<h3>Milestones</h3><ul>\n")
		for j, m := range d.miles {
			if j == newsMilestones {
				fmt.Fprintf(&b, "<li class=mut>... %d more</li>\n", len(d.miles)-j)
				break
			}
			fmt.Fprintf(&b, "<li><code>%s</code> %s <span class=mut>%s &middot; %s &middot; %s</span></li>\n", esc(m.ID), esc(compactText(m.Title, 60)), esc(m.Tenant), esc(m.Progress), esc(m.State))
		}
		b.WriteString("</ul>\n<h3>Decisions</h3><ul>\n")
		for _, e := range d.decisions {
			fmt.Fprintf(&b, "<li><span class=mut>%s</span> %s</li>\n", esc(shortTS(e.row.At)), esc(newsEventText(e)))
		}
		b.WriteString("</ul>\n<h3>Last events</h3><ul>\n")
		for _, e := range d.events {
			fmt.Fprintf(&b, "<li><span class=mut>%s</span> %s</li>\n", esc(shortTS(e.row.At)), esc(newsEventText(e)))
		}
		b.WriteString("</ul>\n")
		if p.summary != "" {
			fmt.Fprintf(&b, "<h3>Summary</h3><pre><code>%s</code></pre>\n", esc(p.summary))
		}
		b.WriteString("</section>\n")
	}
	b.WriteString("</main></body></html>\n")
	return b.String()
}

// newsFill replaces the markers: with the real stamp and moved count when writing, with "" when hashing.
func newsFill(content string, now time.Time, moved int, hasPrev, hashing bool) string {
	stamp, mv := now.Local().Format("2006-01-02 15:04")+" local", "; first run"
	if hasPrev {
		mv = fmt.Sprintf("; %d moved since the last run", moved)
	}
	if hashing {
		stamp, mv = "", ""
	}
	return strings.NewReplacer(newsStampMarker, stamp, newsMovedMarker, mv).Replace(content)
}

// newsWrite stores content at out unless its hash (markers left out) matches the sidecar and the file exists.
func newsWrite(out, content string, now time.Time, moved int, hasPrev bool) (bool, error) {
	sum := sha256.Sum256([]byte(newsFill(content, now, 0, false, true)))
	hash := hex.EncodeToString(sum[:])
	if b, err := readFileShared(out + ".hash"); err == nil {
		if f := strings.Fields(string(b)); len(f) >= 2 && f[1] == hash {
			if _, err := os.Stat(out); err == nil {
				return false, nil
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return false, err
	}
	if err := writeFileAtomicMode(out, []byte(newsFill(content, now, moved, hasPrev, false)), 0644); err != nil {
		return false, err
	}
	return true, writeFileAtomicMode(out+".hash", []byte(strconv.FormatInt(now.Unix(), 10)+"\n"+hash+"\n"), 0644)
}

// newsPrevStamp reads the unix time of the last write recorded beside out, zero when none.
func newsPrevStamp(out string) time.Time {
	if b, err := readFileShared(out + ".hash"); err == nil {
		if f := strings.Fields(string(b)); len(f) >= 1 {
			if n, e := strconv.ParseInt(f[0], 10, 64); e == nil {
				return time.Unix(n, 0)
			}
		}
	}
	return time.Time{}
}

type newsResult struct {
	Job     string `json:"job,omitempty"`
	Path    string `json:"path"`
	Written bool   `json:"written"`
}

func newsCommand(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return fmt.Errorf("unexpected argument %q", pos[0])
	}
	if err := allowOptions(o, "job", "format", "out", "json", "width"); err != nil {
		return err
	}
	format := value(o, "format", "md")
	if format != "md" && format != "html" && format != "text" {
		return fmt.Errorf("--format %q: use md, html or text", format)
	}
	width := 0
	if format == "text" {
		if width, err = newsTextWidth(o["width"]); err != nil {
			return err
		}
	} else if o["width"] != "" {
		return fmt.Errorf("--width is only for --format text")
	}
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
		ids = []string{""} // no jobs: the workspace is the one section
	}
	stdout := o["out"] == "-"
	text := format == "text"
	if o["json"] == "true" && (stdout || text && o["out"] == "") {
		return errors.New("-j needs a file target (-o PATH or news.out)")
	}
	// everything is validated and rendered before anything is written
	var parts []newsPart
	var datas []newsData
	var targets []string
	seen := map[string]string{}
	for _, id := range ids {
		p := newsPart{job: id, title: w.Name, tenants: newsJobTenants(w, id)}
		dir := "docs/news/"
		if id != "" {
			v := jobView(w, id)
			p.title, p.desc, p.url, dir = v.Title, v.Description, v.NewsURL, v.NewsOut
			if v.Summary != "" {
				if p.summary, err = newsSummary(root, v.Summary); err != nil {
					return err
				}
			}
		}
		target := ""
		switch {
		case stdout, text && o["out"] == "": // no file, so no news.out and no sidecar
		case o["out"] != "":
			target = htmlPath(path, o["out"])
		default:
			d, err := newsConfined(root, "news.out", dir)
			if err != nil {
				return err
			}
			target = filepath.Join(d, "status."+format)
			if other, dup := seen[target]; dup {
				return fmt.Errorf("jobs %q and %q share the news file %s: set news.out per job (mw jobs set ID news.out=docs/news/ID/)", other, id, target)
			}
			seen[target] = id
		}
		parts, targets = append(parts, p), append(targets, target)
		since := time.Time{}
		if target != "" {
			since = newsPrevStamp(target)
		}
		datas = append(datas, newsCollect(p, since))
	}
	if text { // stdout by default; a file only with -o PATH; no news.out, no sidecar
		out := ""
		if o["out"] != "" && !stdout {
			out = htmlPath(path, o["out"])
		}
		if err := newsTextOut(parts, datas, width, out); err != nil {
			return err
		}
		if out == "" {
			return nil
		}
		if o["json"] == "true" {
			return printJSON([]newsResult{{"", out, true}})
		}
		fmt.Println("wrote", out)
		return nil
	}
	now := time.Now()
	var results []newsResult
	put := func(job, target, content string, moved int, hasPrev bool) error {
		if target == "" {
			fmt.Print(newsFill(strings.ReplaceAll(content, newsMovedMarker, ""), now, 0, false, false)) // no sidecar: no "moved"
			return nil
		}
		wrote, err := newsWrite(target, content, now, moved, hasPrev)
		results = append(results, newsResult{job, target, wrote})
		return err
	}
	if o["out"] != "" { // one file holding every section
		var content string
		moved := 0
		if format == "html" {
			content = renderNewsHTML(parts, datas, w.Name)
		} else {
			docs := make([]string, len(parts))
			for i := range parts {
				docs[i] = newsMDFit(parts[i], datas[i])
			}
			content = strings.Join(docs, "\n")
		}
		for _, d := range datas {
			moved += d.moved
		}
		if err := put("", targets[0], content, moved, datas[0].hasPrev); err != nil {
			return err
		}
	} else {
		for i := range parts {
			var content string
			if format == "html" {
				content = renderNewsHTML(parts[i:i+1], datas[i:i+1], parts[i].title)
			} else {
				content = newsMDFit(parts[i], datas[i])
			}
			if err := put(parts[i].job, targets[i], content, datas[i].moved, datas[i].hasPrev); err != nil {
				return err
			}
		}
	}
	if stdout {
		return nil
	}
	if o["json"] == "true" {
		return printJSON(results)
	}
	for _, r := range results {
		if r.Written {
			fmt.Println("wrote", r.Path)
		} else {
			fmt.Println("unchanged", r.Path)
		}
	}
	return nil
}
