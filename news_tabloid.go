package main

import (
	"fmt"
	"html"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The tabloid edition: the md and html renderings of the news model. One document structure (tabloidDoc) is
// built from the model, then drawn as Markdown or HTML, so both say the same thing. Text and json do not use
// it. Everything shown is a field of the model: a headline is chosen by rule from real events, never invented.

const (
	tabloidAlsoCap    = 8
	tabloidNeedsCap   = 5
	tabloidImageCap   = 6
	tabloidFileCap    = 6
	tabloidProofKeep  = 4 // proof files kept per item in the model
	tabloidComingTail = 2 // names shown per Coming up line
)

// newsProof is one evidence file of an item (the model keeps its absolute path, slash-separated).
type newsProof struct {
	Path  string `json:"path"`
	Image bool   `json:"image,omitempty"`
}

var proofImageExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true}

var releaseNotesRe = regexp.MustCompile(`(?i)^(release|releases|release[-_ ]?notes|releasenotes)$`)

// releaseWork reports release work: a chore that recorded a release-notes file as evidence. The title is never read.
func releaseWork(it Item) bool {
	if it.Kind != "chore" {
		return false
	}
	for _, e := range it.Evidence {
		name := e
		if i := strings.LastIndex(e, " "); i > 0 {
			name = e[:i]
		}
		name = filepath.Base(filepath.FromSlash(name))
		if releaseNotesRe.MatchString(strings.TrimSuffix(name, filepath.Ext(name))) {
			return true
		}
	}
	return false
}

// byShown is who did it, left out for the implicit local session.
func byShown(by string) string {
	if by == localName {
		return ""
	}
	return by
}

// proofFor lists the evidence files an item recorded at done (kept: images first, at most tabloidProofItem).
// Files that are gone are left out.
func proofFor(w *Workspace, t *Tenant, it Item) []newsProof {
	if len(it.Evidence) == 0 {
		return nil
	}
	dir := evidenceFolder(w, t, it.ID)
	var imgs, other []newsProof
	for _, e := range it.Evidence {
		rel := e
		if i := strings.LastIndex(e, " "); i > 0 {
			rel = e[:i]
		}
		if rel == "" || strings.Contains(rel, "..") || strings.ContainsAny(rel, "\x00\n\r") {
			continue
		}
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if fi, err := statShared(p); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		pr := newsProof{Path: filepath.ToSlash(p), Image: proofImageExt[strings.ToLower(filepath.Ext(rel))]}
		if pr.Image {
			imgs = append(imgs, pr)
		} else {
			other = append(other, pr)
		}
	}
	all := append(imgs, other...)
	if len(all) > tabloidProofKeep {
		all = all[:tabloidProofKeep]
	}
	return all
}

// tabloidCtx says where the document will live, so links can be relative.
type tabloidCtx struct {
	OutDir   string // folder of the file being written ("" = unknown: absolute paths)
	Prefix   string // path from that file to the editions folder ("" when it sits there)
	JobsHref string // link to jobs.md ("" = no link)
}

type tabloidLine struct {
	Ref, Title, Tail string
	TLabel, TURL     string
}

type tabloidImg struct{ Alt, Ref string }

type tabloidProofItem struct {
	Line   tabloidLine
	Images []tabloidImg
	Files  []tabloidImg
}

type tabloidLink struct{ Label, Href string }

type tabloidDoc struct {
	Title, Range, Note string
	Head, Body         string // the lead story
	Quiet              string
	Also               []tabloidLine
	AlsoMore           int
	Needs              []tabloidLine
	NeedsMore          int
	Proof              []tabloidProofItem
	Coming             []string
	JobsHref           string
	Archive            []tabloidLink
}

func tline(it newsItem, tail string) tabloidLine {
	l := tabloidLine{Ref: it.Tenant + "/" + it.ID, Title: bc(it.Title), Tail: tail}
	if it.TicketLabel != "" && urlsSafe([]newsItem{it}) {
		l.TLabel, l.TURL = it.TicketLabel, it.TicketURL
	}
	return l
}

func itemTail(it newsItem) string {
	var p []string
	if by := byShown(it.By); by != "" {
		p = append(p, "by "+bc(by))
	}
	if d := bulletinDate(it.Since); d != "-" {
		p = append(p, d)
	}
	return strings.Join(p, ", ")
}

func proofRef(path, outDir string) string {
	if outDir != "" {
		if r, err := filepath.Rel(outDir, filepath.FromSlash(path)); err == nil && r != "." && !strings.HasPrefix(r, "..") {
			return filepath.ToSlash(r)
		}
	}
	return path
}

func proofSafe(p string) bool { return p != "" && !strings.ContainsAny(p, "<>\"\x00\n\r") }

func edition(m *newsModel) (num int, draft bool) {
	if m.Release.Number != nil {
		return *m.Release.Number, false
	}
	if m.Release.Previous != nil {
		return *m.Release.Previous + 1, true
	}
	return 1, true
}

func namesOf(items []newsItem, n int) string {
	var p []string
	for i, it := range items {
		if i == n {
			p = append(p, fmt.Sprintf("+%d", len(items)-n))
			break
		}
		p = append(p, it.Tenant+"/"+it.ID+" "+compactText(it.Title, 80))
	}
	return strings.Join(p, "; ")
}

func newsTabloid(m *newsModel, releases []int, c tabloidCtx) tabloidDoc {
	d := tabloidDoc{JobsHref: c.JobsHref}
	num, draft := edition(m)
	d.Title = fmt.Sprintf("%s News — Edition %d", bc(m.Job.Title), num)
	if draft {
		d.Title += " (draft)"
	}
	since := "since the start"
	if m.Release.Previous != nil {
		since = fmt.Sprintf("since Edition %d", *m.Release.Previous)
		if dt := bulletinDate(m.Release.Since); dt != "-" {
			since += ", " + dt
		}
	}
	d.Range = since
	if dt := bulletinDate(m.Release.At); dt != "-" {
		d.Range += " · up to " + dt
	}
	d.Note = bc(m.Summary)
	h := m.Happened

	// lead story, by priority: release work done > milestone closed > most items done in one tenant >
	// an item that needs Sir > a count of what else happened
	used := map[string]bool{}
	key := func(it newsItem) string { return it.Tenant + "/" + it.ID }
	if m.Release.Changes > 0 {
		var lead *newsItem
		for i := range h.Done {
			if h.Done[i].Release { // the list is newest first: the newest release work leads
				lead = &h.Done[i]
				break
			}
		}
		switch {
		case lead != nil:
			used[key(*lead)] = true
			d.Head = "Done: " + bc(lead.Title)
			d.Body = fmt.Sprintf("%s was marked done%s.", key(*lead), onBy(*lead))
			if lead.MilestoneTitle != "" {
				d.Body += " It belongs to milestone " + bc(lead.MilestoneTitle) + "."
			}
		case len(h.ClosedMilestones) > 0:
			ms := h.ClosedMilestones[0]
			used[key(ms)] = true
			d.Head = "Milestone closed: " + bc(ms.Title)
			d.Body = fmt.Sprintf("%s closed with %d of %d items done%s.", key(ms), ms.Done, ms.Total, onBy(ms))
		case len(h.Done) > 0:
			per := map[string][]newsItem{}
			for _, it := range h.Done {
				per[it.Tenant] = append(per[it.Tenant], it)
			}
			var ts []string
			for t := range per {
				ts = append(ts, t)
			}
			sort.Slice(ts, func(i, j int) bool {
				if len(per[ts[i]]) != len(per[ts[j]]) {
					return len(per[ts[i]]) > len(per[ts[j]])
				}
				return ts[i] < ts[j]
			})
			t, l := ts[0], per[ts[0]]
			if len(l) == 1 {
				d.Head = bc(l[0].Title)
				d.Body = fmt.Sprintf("%s was marked done%s.", key(l[0]), onBy(l[0]))
			} else {
				d.Head = fmt.Sprintf("%d items done in %s", len(l), bc(t))
				d.Body = "Done: " + namesOf(l, 2) + "."
			}
			for i, it := range l {
				if i < 2 {
					used[key(it)] = true
				}
			}
		case len(m.Now.Needs) > 0:
			n := m.Now.Needs[0]
			d.Head = "Waiting on Sir: " + bc(n.Title)
			d.Body = fmt.Sprintf("%s asks for an answer%s.", key(n), onBy(n))
			if n.Note != "" {
				d.Body += " " + bc(n.Note)
			}
		default:
			var a []string
			for _, f := range []struct{ k, one, many string }{{"new_items", "new item filed", "new items filed"}, {"accepted", "item accepted", "items accepted"},
				{"sent_back", "item sent back", "items sent back"}, {"answers", "answer given", "answers given"}, {"decisions", "decision recorded", "decisions recorded"}} {
				if n := h.Counts[f.k]; n > 0 {
					a = append(a, plural(n, f.one, f.many))
				}
			}
			if len(a) == 0 {
				a = append(a, plural(m.Release.Changes, "change", "changes"))
			}
			d.Head = strings.ToUpper(a[0][:1]) + a[0][1:]
			d.Body = strings.Join(a, ", ") + "."
		}
	} else {
		d.Quiet = "Quiet edition — nothing new"
		if m.Release.Previous != nil {
			d.Quiet += fmt.Sprintf(" since Edition %d", *m.Release.Previous)
		}
	}

	// also in this edition
	if m.Release.Changes > 0 {
		var rest []tabloidLine
		for _, it := range h.ClosedMilestones {
			if !used[key(it)] {
				l := tline(it, itemTail(it))
				l.Title = fmt.Sprintf("Milestone closed: %s (%d/%d)", l.Title, it.Done, it.Total)
				rest = append(rest, l)
			}
		}
		for _, it := range h.Done {
			if !used[key(it)] {
				rest = append(rest, tline(it, itemTail(it)))
			}
		}
		flow := 0
		for k, n := range h.Counts {
			if k != "moved_to_review" && k != "moved_to_doing" {
				flow += n
			}
		}
		if n := m.Release.Changes - flow; n > 0 {
			rest = append(rest, tabloidLine{Ref: "Harness", Title: plural(n, "metric row changed", "metric rows changed")})
		}
		total := len(rest) + h.More["done"] + h.More["closed_milestones"]
		if len(rest) > tabloidAlsoCap {
			rest = rest[:tabloidAlsoCap]
		}
		d.Also, d.AlsoMore = rest, total-len(rest)
	}

	// needs you
	for i, it := range m.Now.Needs {
		if i == tabloidNeedsCap {
			break
		}
		l := tline(it, "")
		if it.Note != "" {
			l.Tail = bc(it.Note)
		}
		d.Needs = append(d.Needs, l)
	}
	d.NeedsMore = max(m.Now.Counts.Needs-len(d.Needs), 0)

	// proof is only for things worth seeing: images, and the files of items awaiting review. Release work adds
	// one link (its release-notes file); a plain text file of an ordinary done item is never proof.
	type candidate struct {
		it   newsItem
		mode int // proofReview: every file; proofRelease: images and one file; proofImages: images only
	}
	const (
		proofReview = iota
		proofRelease
		proofImages
	)
	var cand []candidate
	seen := map[string]bool{}
	add := func(l []newsItem, mode int, f func(newsItem) bool) {
		for _, it := range l {
			if !seen[key(it)] && len(it.Proof) > 0 && f(it) {
				seen[key(it)] = true
				cand = append(cand, candidate{it, mode})
			}
		}
	}
	hasImg := func(it newsItem) bool {
		for _, p := range it.Proof {
			if p.Image {
				return true
			}
		}
		return false
	}
	add(m.Now.Review, proofReview, func(newsItem) bool { return true })
	add(h.Done, proofRelease, func(it newsItem) bool { return it.Release })
	add(h.Done, proofImages, hasImg)
	imgs, files := 0, 0
	for _, cd := range cand {
		it := cd.it
		pi := tabloidProofItem{Line: tline(it, "")}
		itemFiles := 0
		for _, p := range it.Proof {
			if !proofSafe(p.Path) {
				continue
			}
			ref := proofRef(p.Path, c.OutDir)
			name := filepath.Base(p.Path)
			switch {
			case p.Image && imgs < tabloidImageCap:
				imgs++
				pi.Images = append(pi.Images, tabloidImg{it.ID + " " + name, ref})
			case !p.Image && cd.mode != proofImages && files < tabloidFileCap && (cd.mode == proofReview || itemFiles == 0):
				files++
				itemFiles++
				pi.Files = append(pi.Files, tabloidImg{name, ref})
			}
		}
		if len(pi.Images)+len(pi.Files) > 0 {
			d.Proof = append(d.Proof, pi)
		}
	}

	// coming up
	co := m.Now
	if n := co.Counts.Doing; n > 0 {
		d.Coming = append(d.Coming, fmt.Sprintf("In progress (%d): %s", n, namesOf(co.Doing, tabloidComingTail)))
	}
	if n := co.Counts.Review; n > 0 {
		d.Coming = append(d.Coming, fmt.Sprintf("Awaiting review: %d", n))
	}
	if n := co.Counts.Ready; n > 0 {
		d.Coming = append(d.Coming, fmt.Sprintf("Up next (%d ready): %s", n, namesOf(m.Next.Ready, tabloidComingTail)))
	}
	if len(co.Milestones.Open) > 0 {
		var p []string
		for i, ms := range co.Milestones.Open {
			if i == tabloidComingTail {
				p = append(p, fmt.Sprintf("+%d", len(co.Milestones.Open)-i))
				break
			}
			p = append(p, fmt.Sprintf("%s/%s %s %d/%d", ms.Tenant, ms.ID, compactText(ms.Title, 80), ms.Done, ms.Total))
		}
		d.Coming = append(d.Coming, "Milestones: "+strings.Join(p, "; "))
	}
	if n := co.Counts.Blocked; n > 0 {
		d.Coming = append(d.Coming, fmt.Sprintf("Blocked: %d", n))
	}
	if len(d.Coming) == 0 {
		d.Coming = []string{"Nothing queued."}
	}

	// archive: editions are N.md next to the folder prefix
	for i, n := range releases {
		if i == 5 {
			break
		}
		if n == num && !draft {
			continue
		}
		d.Archive = append(d.Archive, tabloidLink{strconv.Itoa(n), c.Prefix + strconv.Itoa(n)})
	}
	return d
}

// tabloidQuiet is a job with nothing new since the last edition and nothing needing attention or running.
func tabloidQuiet(m *newsModel) bool {
	c := m.Now.Counts
	return m.Release.Changes == 0 && c.Needs == 0 && c.Review == 0 && c.Doing == 0
}

func onBy(it newsItem) string {
	var s string
	if by := byShown(it.By); by != "" {
		s += " by " + bc(by)
	}
	if dt := bulletinDate(it.Since); dt != "-" {
		s += " on " + dt
	}
	return s
}

// mdEsc backslash-escapes what Markdown would act on (mdText also covers & and ~).
func mdEsc(s string) string {
	return mdText(bc(s))
}

func mdLineText(l tabloidLine) string {
	s := "**" + mdEsc(l.Ref) + "** " + mdEsc(l.Title)
	if l.TLabel != "" {
		s += fmt.Sprintf(" [%s](<%s>)", mdLabel(l.TLabel), l.TURL)
	}
	if l.Tail != "" {
		s += " — " + mdEsc(l.Tail)
	}
	return s
}

func renderTabloidMD(m *newsModel, releases []int, c tabloidCtx) string {
	d := newsTabloid(m, releases, c)
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f, a...) }
	w("# %s\n\n_%s_\n\n", mdEsc(d.Title), mdEsc(d.Range))
	if d.Note != "" {
		w("%s\n\n", mdEsc(d.Note))
	}
	if d.Quiet != "" {
		w("**%s**\n\n", mdEsc(d.Quiet))
	} else {
		w("## %s\n\n%s\n\n", mdEsc(d.Head), mdEsc(d.Body))
	}
	if len(d.Also) > 0 || d.AlsoMore > 0 {
		w("### Also in this edition\n\n")
		for _, l := range d.Also {
			w("- %s\n", mdLineText(l))
		}
		if d.AlsoMore > 0 {
			w("- +%d more\n", d.AlsoMore)
		}
		w("\n")
	}
	if len(d.Needs) > 0 {
		w("> **Needs you**\n>\n")
		for _, l := range d.Needs {
			w("> - %s\n", mdLineText(l))
		}
		if d.NeedsMore > 0 {
			w("> - +%d more\n", d.NeedsMore)
		}
		w("\n")
	}
	if len(d.Proof) > 0 {
		w("### Proof\n\n")
		for _, p := range d.Proof {
			w("%s\n\n", mdLineText(p.Line))
			for _, im := range p.Images {
				w("![%s](<%s>)\n\n", mdEsc(im.Alt), im.Ref)
			}
			for _, f := range p.Files {
				w("- [%s](<%s>)\n", mdLabel(f.Alt), f.Ref)
			}
			if len(p.Files) > 0 {
				w("\n")
			}
		}
	}
	w("### Coming up\n\n")
	for _, l := range d.Coming {
		w("- %s\n", mdEsc(l))
	}
	w("\n---\n\n")
	var f []string
	if d.JobsHref != "" {
		f = append(f, fmt.Sprintf("[Now: jobs.md](<%s>)", d.JobsHref))
	}
	arch := "Editions:"
	for _, a := range d.Archive {
		arch += fmt.Sprintf(" [%s](<%s.md>)", a.Label, a.Href)
	}
	if len(d.Archive) == 0 {
		arch += " none yet"
	}
	f = append(f, arch)
	w("%s\n", strings.Join(f, " · "))
	return b.String()
}

func htmlLine(l tabloidLine) string {
	esc := html.EscapeString
	s := "<strong>" + esc(l.Ref) + "</strong> " + esc(l.Title)
	if l.TLabel != "" {
		s += ` <a href="` + esc(l.TURL) + `">` + esc(l.TLabel) + `</a>`
	}
	if l.Tail != "" {
		s += " — " + esc(l.Tail)
	}
	return s
}

// htmlSrc turns a proof reference into a URL: relative stays relative, an absolute path becomes file://.
func htmlSrc(ref string) string {
	if filepath.IsAbs(filepath.FromSlash(ref)) || strings.HasPrefix(ref, "/") {
		return (&url.URL{Scheme: "file", Path: ref}).String()
	}
	return (&url.URL{Path: ref}).String()
}

func bulletinHTMLBlock(m *newsModel, releases []int, c tabloidCtx) string {
	esc := html.EscapeString
	d := newsTabloid(m, releases, c)
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f, a...) }
	w("<article>\n<h1>%s</h1>\n<p class=\"range\">%s</p>\n", esc(d.Title), esc(d.Range))
	if d.Note != "" {
		w("<p>%s</p>\n", esc(d.Note))
	}
	if d.Quiet != "" {
		w("<p><strong>%s</strong></p>\n", esc(d.Quiet))
	} else {
		w("<h2>%s</h2>\n<p>%s</p>\n", esc(d.Head), esc(d.Body))
	}
	if len(d.Also) > 0 || d.AlsoMore > 0 {
		w("<h3>Also in this edition</h3>\n<ul>\n")
		for _, l := range d.Also {
			w("<li>%s</li>\n", htmlLine(l))
		}
		if d.AlsoMore > 0 {
			w("<li>+%d more</li>\n", d.AlsoMore)
		}
		w("</ul>\n")
	}
	if len(d.Needs) > 0 {
		w("<blockquote><strong>Needs you</strong>\n<ul>\n")
		for _, l := range d.Needs {
			w("<li>%s</li>\n", htmlLine(l))
		}
		if d.NeedsMore > 0 {
			w("<li>+%d more</li>\n", d.NeedsMore)
		}
		w("</ul></blockquote>\n")
	}
	if len(d.Proof) > 0 {
		w("<h3>Proof</h3>\n")
		for _, p := range d.Proof {
			w("<p>%s</p>\n", htmlLine(p.Line))
			for _, im := range p.Images {
				w("<p><img src=\"%s\" alt=\"%s\"></p>\n", esc(htmlSrc(im.Ref)), esc(im.Alt))
			}
			if len(p.Files) > 0 {
				w("<ul>\n")
				for _, f := range p.Files {
					w("<li><a href=\"%s\">%s</a></li>\n", esc(htmlSrc(f.Ref)), esc(f.Alt))
				}
				w("</ul>\n")
			}
		}
	}
	w("<h3>Coming up</h3>\n<ul>\n")
	for _, l := range d.Coming {
		w("<li>%s</li>\n", esc(l))
	}
	w("</ul>\n<hr>\n<p class=\"foot\">")
	if d.JobsHref != "" {
		w("<a href=\"%s\">Now: jobs.md</a> · ", esc((&url.URL{Path: d.JobsHref}).String()))
	}
	w("Editions:")
	for _, a := range d.Archive {
		w(" <a href=\"%s\">%s</a>", esc((&url.URL{Path: a.Href + ".html"}).String()), esc(a.Label))
	}
	if len(d.Archive) == 0 {
		w(" none yet")
	}
	w("</p>\n</article>\n")
	return b.String()
}

func bulletinHTMLPage(title string, blocks []string) string {
	return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + html.EscapeString(title) + `</title>
<style>
:root{--bg:#fff;--fg:#1b1b1f;--dim:#5b5b66;--acc:#2563eb;--box:#fef2f2;--bar:#b91c1c}
@media (prefers-color-scheme:dark){:root{--bg:#121214;--fg:#e8e8ec;--dim:#a0a0ab;--acc:#7aa7ff;--box:#2a1517;--bar:#f87171}}
body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.5 -apple-system,system-ui,sans-serif}
article{max-width:720px;margin:0 auto;padding:16px}
h1{font-size:1.6rem;margin:.2em 0}h2{font-size:1.35rem}h3{font-size:1.05rem;margin-top:1.4em}
.range,.foot{color:var(--dim)}a{color:var(--acc)}img{max-width:100%;height:auto}
blockquote{margin:1em 0;padding:.5em 1em;background:var(--box);border-left:4px solid var(--bar)}
</style></head><body>
` + strings.Join(blocks, "") + `</body></html>
`
}

func renderBulletinHTML(m *newsModel, releases []int, hrefPrefix string) string {
	return bulletinHTMLPage(bulletinTitle(m), []string{bulletinHTMLBlock(m, releases, tabloidCtx{Prefix: hrefPrefix})})
}

func renderBulletinMD(m *newsModel, releases []int) string {
	return renderTabloidMD(m, releases, tabloidCtx{})
}

func bulletinTitle(m *newsModel) string {
	num, draft := edition(m)
	s := fmt.Sprintf("%s News — Edition %d", bulletinCut(bc(m.Job.Title), 72), num)
	if draft {
		s += " (draft)"
	}
	return s
}
