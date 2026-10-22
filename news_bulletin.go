package main

import (
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// The news bulletin: every news rendering (text, md, html) draws the same plain-monospace, 70-column,
// gopher-menu style page from the model. Text is ASCII except for the ▲/▼ deltas (board text is shown as it
// is, stripped of control characters). See docs/specs/news-model.md, "Look: an old gopher-style bulletin".

const (
	bulletinWidth = 70
	bulletinLimit = 9500 // characters of one bulletin (the pane's Markdown limit)
)

// bulletinLimits are the row caps of one drawing; bulletinLevels step down until the bulletin fits.
type bulletinLimits struct{ short, happen, ongoing, ready, metrics int }

var bulletinLevels = []bulletinLimits{{12, 10, 5, 10, 60}, {8, 5, 3, 5, 30}, {5, 3, 2, 3, 16}, {3, 2, 1, 2, 8}, {2, 1, 1, 1, 4}, {1, 0, 0, 0, 0}}

// bulletinCut cuts s to at most w cells; a cut ends with "~".
func bulletinCut(s string, w int) string {
	if newsStrWidth(s) <= w {
		return s
	}
	if w < 1 {
		return ""
	}
	return newsCut(s, w-1) + "~"
}

func bc(s string) string { return strings.Join(strings.Fields(newsClean(s, false)), " ") }

// bulletinPair puts left and right on one line inside a one-space margin on each side.
func bulletinPair(left, right string) string {
	inner := bulletinWidth - 2
	if newsStrWidth(right) > inner-8 {
		right = bulletinCut(right, inner-8)
	}
	left = bulletinCut(left, inner-newsStrWidth(right)-1)
	gap := inner - newsStrWidth(left) - newsStrWidth(right)
	return " " + left + strings.Repeat(" ", gap) + right
}

func bulletinDate(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return "-"
}

func bulletinItem(typ, tenant, id, title, state, tag string) string {
	head := typ + " " + tenant + "/" + id
	tail := " " + state
	maxHead := bulletinWidth - newsStrWidth(tail) - 14
	head = bulletinCut(head, maxHead)
	head += "  "
	avail := bulletinWidth - newsStrWidth(head) - newsStrWidth(tail)
	t := bc(title)
	if tag = bc(tag); tag != "" {
		tag = " [" + tag + "]"
	}
	if newsStrWidth(t)+newsStrWidth(tag) > avail-4 { // a space and at least three dots
		t = bulletinCut(t, avail-4-newsStrWidth(tag))
	}
	t += tag
	dots := avail - newsStrWidth(t) - 1
	return head + t + " " + strings.Repeat(".", max(dots, 0)) + tail
}

func bulletinSection(n int, name string) []string {
	h := strconv.Itoa(n) + ". " + name
	return []string{"", h, strings.Repeat("-", len(h))}
}

func bulletinMore(n int) string { return fmt.Sprintf("... %d more (see full board)", n) }

func bulletinNoteLine(note string) string {
	if n := bc(note); n != "" {
		return "    " + bulletinCut(n, bulletinWidth-4)
	}
	return ""
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func bulletinShortLines(m *newsModel, bulletinShort int) []string {
	if s := strings.TrimSpace(m.Summary); s != "" {
		var out []string
		for _, para := range strings.Split(s, "\n") {
			para = strings.TrimRight(newsClean(para, false), " ")
			if strings.TrimSpace(para) == "" {
				if len(out) > 0 && out[len(out)-1] != "" {
					out = append(out, "")
				}
				continue
			}
			out = append(out, newsWrap(para, bulletinWidth-2, "  ", "  ")...)
		}
		for len(out) > 0 && out[len(out)-1] == "" {
			out = out[:len(out)-1]
		}
		if len(out) > bulletinShort {
			n := len(out) - bulletinShort + 1
			out = append(out[:bulletinShort-1], "  "+bulletinMore(n))
		}
		return out
	}
	h, c := m.Happened.Counts, m.Now.Counts
	var a []string
	if n := h["done"]; n > 0 {
		a = append(a, plural(n, "item done", "items done"))
	}
	if n := h["closed_milestones"]; n > 0 {
		a = append(a, plural(n, "milestone closed", "milestones closed"))
	}
	if n := h["accepted"]; n > 0 {
		a = append(a, fmt.Sprintf("%d accepted", n))
	}
	if n := h["sent_back"]; n > 0 {
		a = append(a, fmt.Sprintf("%d sent back", n))
	}
	var b []string
	if c.Needs > 0 {
		b = append(b, fmt.Sprintf("%d need you", c.Needs))
	}
	if c.Review > 0 {
		b = append(b, fmt.Sprintf("%d in review", c.Review))
	}
	if c.Blocked > 0 {
		b = append(b, fmt.Sprintf("%d blocked", c.Blocked))
	}
	if c.Doing > 0 {
		b = append(b, fmt.Sprintf("%d in progress", c.Doing))
	}
	cap1 := func(s string) string { return strings.ToUpper(s[:1]) + s[1:] + "." }
	var out []string
	if len(a) == 0 {
		out = append(out, "  Nothing was finished in this window.")
	} else {
		out = append(out, "  "+cap1(strings.Join(a, ", ")))
	}
	if len(b) > 0 {
		out = append(out, "  "+cap1(strings.Join(b, ", ")))
	}
	if m.Metrics != nil {
		ch := 0
		for _, s := range m.Metrics.Sections {
			for _, r := range s.Rows {
				if len(r.Delta) > 0 {
					ch++
				}
			}
		}
		if ch > 0 {
			out = append(out, "  "+plural(ch, "metric row changed.", "metric rows changed."))
		}
	}
	return out
}

// bulletinList appends up to limit rows of a list, then the "... N more" line; total is the full count.
func bulletinList(out *[]string, items []newsItem, total, limit int, typ, state string, notes bool) {
	for i, it := range items {
		if i == limit {
			break
		}
		*out = append(*out, bulletinItem(typ, bc(it.Tenant), bc(it.ID), it.Title, state, it.TicketLabel))
		if notes {
			if l := bulletinNoteLine(it.Note); l != "" {
				*out = append(*out, l)
			}
		}
	}
	if total > limit {
		*out = append(*out, "  "+bulletinMore(total-limit))
	}
}

func bulletinDelta(r newsMRow, kind string) string {
	d, ok := r.Delta[metricsPrimary(kind)]
	if !ok || d == 0 {
		return ""
	}
	if d > 0 {
		return " ▲" + strconv.FormatFloat(d, 'f', -1, 64)
	}
	return " ▼" + strconv.FormatFloat(-d, 'f', -1, 64)
}

const bulletinBar = 20

// bulletinSplitBar spreads good/warn/bad/none over n cells (largest remainder, so the cells add up to n).
func bulletinSplitBar(parts [4]int, chars string, n int) string {
	tot := 0
	for _, p := range parts {
		tot += p
	}
	if tot == 0 {
		return strings.Repeat(".", n)
	}
	cells := [4]int{}
	rem := [4]int{}
	used := 0
	for i, p := range parts {
		cells[i], rem[i] = p*n/tot, p*n%tot
		used += cells[i]
	}
	for used < n {
		best := -1
		for i := range parts {
			if parts[i] > 0 && (best < 0 || rem[i] > rem[best]) {
				best = i
			}
		}
		cells[best]++
		rem[best] = -1
		used++
	}
	var b strings.Builder
	for i := range cells {
		b.WriteString(strings.Repeat(string(chars[i]), cells[i]))
	}
	return b.String()
}

func bulletinRatioBar(num, den float64, n int) string {
	f := 0
	if den > 0 {
		r := num / den
		if r > 1 {
			r = 1
		}
		if r < 0 {
			r = 0
		}
		f = int(r*float64(n) + 0.5)
	}
	return strings.Repeat("#", f) + strings.Repeat(".", n-f)
}

func bulletinMetrics(out *[]string, mt *newsMetrics) {
	if mt == nil {
		*out = append(*out, "  (no metrics)")
		return
	}
	if mt.Error != "" {
		for _, l := range newsWrap("metrics file invalid: "+bc(mt.Error), bulletinWidth-2, "  ", "    ") {
			*out = append(*out, l)
		}
		return
	}
	if mt.Notice != "" {
		for _, l := range newsWrap("notice: "+bc(mt.Notice), bulletinWidth-2, "  ", "    ") {
			*out = append(*out, l)
		}
	}
	if len(mt.Sections) == 0 && mt.Notice == "" {
		*out = append(*out, "  (no metrics)")
	}
	if mt.Updated != "" {
		*out = append(*out, "  updated "+bulletinCut(bc(mt.Updated), 50))
	}
	for _, s := range mt.Sections {
		*out = append(*out, "", "  "+bulletinCut(bc(s.Title), bulletinWidth-4))
		if s.Kind == "split" && s.Legend != nil {
			l := fmt.Sprintf("# %s  = %s  x %s  . %s", bc(s.Legend.Good), bc(s.Legend.Warn), bc(s.Legend.Bad), bc(s.Legend.None))
			*out = append(*out, "  "+bulletinCut(l, bulletinWidth-4))
		}
		if n := bc(s.Note); n != "" {
			*out = append(*out, "  "+bulletinCut(n, bulletinWidth-4))
		}
		for _, r := range s.Rows {
			var bar, count string
			switch s.Kind {
			case "split":
				tot := r.Good + r.Warn + r.Bad + r.None
				bar = bulletinSplitBar([4]int{r.Good, r.Warn, r.Bad, r.None}, "#=x.", bulletinBar)
				count = fmt.Sprintf("%d/%d", r.Good, tot)
			case "progress":
				if r.Total == 0 {
					bar = strings.Repeat(".", bulletinBar)
					count = "not started"
				} else {
					bar = bulletinRatioBar(float64(r.Done), float64(r.Total), bulletinBar)
					count = fmt.Sprintf("%d/%d", r.Done, r.Total)
				}
			default:
				v := 0.0
				if r.Value != nil {
					v = *r.Value
				}
				count = strconv.FormatFloat(v, 'f', -1, 64)
				if u := bc(r.Unit); u != "" {
					count += " " + u
				}
				if r.Target != nil && *r.Target != 0 {
					if r.Better == "lower" {
						bar = bulletinRatioBar(*r.Target, max(v, *r.Target), bulletinBar)
					} else {
						bar = bulletinRatioBar(v, *r.Target, bulletinBar)
					}
				} else {
					bar = strings.Repeat(".", bulletinBar)
				}
			}
			name := bulletinCut(bc(r.Name), 18)
			line := "  " + newsPad(name, 18) + " [" + bar + "]  " + count + bulletinDelta(r, s.Kind)
			*out = append(*out, bulletinCut(line, bulletinWidth))
		}
	}
}

// bulletinFooterParts picks the release numbers that fit beside "end of bulletin".
func bulletinFooterParts(releases []int) (shown []int, label string) {
	label = "releases:"
	if len(releases) == 0 {
		return nil, "releases: none"
	}
	room := bulletinWidth - 2 - len("end of bulletin") - 2 - len(label)
	used := 0
	for _, n := range releases {
		w := len(strconv.Itoa(n)) + 1
		if used+w+2 > room { // keep room for " .."
			break
		}
		shown = append(shown, n)
		used += w
	}
	return shown, label
}

// bulletinLines draws the bulletin. releases lists the release numbers for the footer, newest first;
// link, when set, renders one footer number (html).
func bulletinLines(m *newsModel, releases []int) []string {
	var out []string
	for i, lim := range bulletinLevels {
		out = bulletinDraw(m, releases, lim)
		n := 0
		for _, l := range out {
			n += len([]rune(l)) + 1
		}
		if n+20 <= bulletinLimit || i == len(bulletinLevels)-1 { // 20: the md fence
			break
		}
	}
	return out
}

func bulletinDraw(m *newsModel, releases []int, lim bulletinLimits) []string {
	var out []string
	rule := strings.Repeat("=", bulletinWidth)
	title := strings.ToUpper(bc(m.Job.Title))
	if title == "" {
		title = "NEWS"
	}
	right1, left2 := "", ""
	date := bulletinDate(m.Release.At)
	if m.Release.Number != nil {
		right1 = fmt.Sprintf("RELEASE %d / %s", *m.Release.Number, date)
	} else {
		right1 = "DRAFT - NOT RELEASED"
	}
	switch {
	case m.Release.Previous != nil && m.Release.Number != nil:
		left2 = fmt.Sprintf("vs release %d (%s)", *m.Release.Previous, bulletinDate(m.Release.Since))
	case m.Release.Previous != nil:
		left2 = fmt.Sprintf("since release %d (%s) - %s", *m.Release.Previous, bulletinDate(m.Release.Since), plural(m.Release.Changes, "change", "changes"))
	case m.Release.Number != nil:
		left2 = "since the start"
	default:
		left2 = "since the start - " + plural(m.Release.Changes, "change", "changes")
	}
	u := bc(m.Job.URL)
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	if u == "" {
		id := bc(m.Job.ID)
		if id == "" {
			id = "workspace"
		}
		u = "local/1/news/" + id
	}
	out = append(out, rule, bulletinPair(bulletinCut(title+" BULLETIN", 46), right1))
	if d := bc(m.Job.Description); d != "" { // the job's one-line description, dim, under the title
		out = append(out, " "+bulletinCut(d, 66))
	}
	out = append(out, bulletinPair(left2, "gopher://"+u), rule)

	out = append(out, bulletinSection(1, "IN SHORT")...)
	out = append(out, bulletinShortLines(m, lim.short)...)

	out = append(out, bulletinSection(2, "WHAT HAPPENED")...)
	h := m.Happened
	n0 := len(out)
	bulletinList(&out, h.ClosedMilestones, h.Counts["closed_milestones"], lim.happen, "1", "closed", false)
	bulletinList(&out, h.Done, h.Counts["done"], lim.happen, "0", "done", false)
	bulletinList(&out, h.Accepted, h.Counts["accepted"], lim.happen, "0", "accepted", false)
	bulletinList(&out, h.SentBack, h.Counts["sent_back"], lim.happen, "0", "sent back", true)
	bulletinList(&out, h.Decisions, h.Counts["decisions"], lim.happen, "0", "decided", true)
	bulletinList(&out, h.Answers, h.Counts["answers"], lim.happen, "0", "answered", true)
	if len(out) == n0 {
		out = append(out, "  (nothing happened)")
	}

	out = append(out, bulletinSection(3, "ONGOING")...)
	c := m.Now.Counts
	out = append(out, fmt.Sprintf("  %d of %d done, %d ready", c.Done, c.Total, c.Ready))
	n0 = len(out)
	bulletinList(&out, m.Now.Needs, c.Needs, lim.ongoing, "!", "needs you", true)
	bulletinList(&out, m.Now.Review, c.Review, lim.ongoing, "?", "review", false)
	bulletinList(&out, m.Now.Blocked, c.Blocked, lim.ongoing, "-", "blocked", true)
	bulletinList(&out, m.Now.Doing, c.Doing, lim.ongoing, "+", "doing", false)
	if len(out) == n0 {
		out = append(out, "  (nothing in flight)")
	}

	out = append(out, bulletinSection(4, "WHAT'S NEXT")...)
	n0 = len(out)
	bulletinList(&out, m.Next.Ready, c.Ready, lim.ready, ">", "ready", false)
	nm := m.Next.NextMilestones
	extra := m.Next.More["next_milestones"]
	if len(nm) > lim.ready {
		extra += len(nm) - lim.ready
		nm = nm[:lim.ready]
	}
	for _, ms := range nm {
		out = append(out, bulletinItem("1", bc(ms.Tenant), bc(ms.ID), ms.Title, fmt.Sprintf("%d/%d", ms.Done, ms.Total), ""))
	}
	if n := extra; n > 0 {
		out = append(out, "  "+bulletinMore(n))
	}
	if len(out) == n0 {
		out = append(out, "  (nothing queued)")
	}

	out = append(out, bulletinSection(5, "METRICS")...)
	var mt []string
	bulletinMetrics(&mt, m.Metrics)
	if len(mt) > lim.metrics {
		mt = append(mt[:lim.metrics], "  "+bulletinMore(len(mt)-lim.metrics))
	}
	out = append(out, mt...)

	out = append(out, "", strings.Repeat("-", bulletinWidth), bulletinFooter(releases, nil, nil), rule)
	return out
}

// bulletinFooter draws " releases: 7 6 5   end of bulletin". With link and esc set (html) the numbers are
// links and the text is escaped; the padding always counts the plain text.
func bulletinFooter(releases []int, link func(n int) string, esc func(string) string) string {
	if esc == nil {
		esc = func(s string) string { return s }
	}
	shown, label := bulletinFooterParts(releases)
	plain, rich := label, esc(label)
	for _, n := range shown {
		plain += " " + strconv.Itoa(n)
		if link != nil {
			rich += " " + link(n)
		} else {
			rich += " " + strconv.Itoa(n)
		}
	}
	if len(shown) < len(releases) {
		plain += " .."
		rich += " .."
	}
	const end = "end of bulletin"
	gap := bulletinWidth - 2 - newsStrWidth(plain) - len(end)
	return " " + rich + strings.Repeat(" ", max(gap, 1)) + end
}

func renderBulletinText(m *newsModel, releases []int) string {
	return strings.Join(bulletinLines(m, releases), "\n") + "\n"
}

// newsTicketLinks lists the ticket links of every item in the model, in list order, once per item.
func newsTicketLinks(m *newsModel) []newsItem {
	var out []newsItem
	seen := map[string]bool{}
	for _, l := range [][]newsItem{m.Now.Needs, m.Now.Review, m.Now.Blocked, m.Now.Doing, m.Happened.ClosedMilestones, m.Happened.Done, m.Happened.Accepted,
		m.Happened.SentBack, m.Happened.Answers, m.Happened.Decisions, m.Next.Ready} {
		for _, it := range l {
			k := it.Tenant + "/" + it.ID
			if it.TicketLabel == "" || it.TicketURL == "" || seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, it)
		}
	}
	return out
}

// mdLabel backslash-escapes every ASCII punctuation character of a link label.
func mdLabel(s string) string {
	var b strings.Builder
	for _, r := range bc(s) {
		if r < 0x80 && (unicode.IsPunct(r) || unicode.IsSymbol(r)) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// renderBulletinMD is one fenced text block; tickets that have a link follow it as a Markdown list
// ([KEY](url)), because links do not render inside a code block.
func renderBulletinMD(m *newsModel, releases []int) string {
	body := renderBulletinText(m, releases)
	f := newsFence(body)
	out := f + "text\n" + body + f + "\n"
	if links := newsTicketLinks(m); len(links) > 0 && urlsSafe(links) {
		out += "\nTickets:\n\n"
		for _, it := range links {
			out += fmt.Sprintf("- [%s](<%s>) %s\n", mdLabel(it.TicketLabel), it.TicketURL, mdLabel(it.Tenant+"/"+it.ID))
		}
	}
	return out
}

// urlsSafe re-checks the links of a model that may have come from a file: only https URLs of the tracker charset.
func urlsSafe(items []newsItem) bool {
	for _, it := range items {
		if !strings.HasPrefix(it.TicketURL, "https://") || !trackerURLRe.MatchString(it.TicketURL) {
			return false
		}
		if strings.ContainsAny(it.TicketURL, " <>\"'`()[]\\\x00\n\r") {
			return false
		}
	}
	return true
}

// linkTicket escapes a line and turns its "[LABEL]" into a link when a ticket of the model has that label.
func linkTicket(l string, tl []newsItem, esc func(string) string) string {
	if len(l) < 3 || (l[0] != '0' && l[0] != '1' && l[0] != '!' && l[0] != '?' && l[0] != '-' && l[0] != '+' && l[0] != '>') || l[1] != ' ' {
		return esc(l)
	}
	for _, it := range tl {
		tag := "[" + it.TicketLabel + "]"
		if i := strings.LastIndex(l, tag); i > 1 && strings.Contains(l[:i], it.Tenant+"/"+it.ID) {
			return esc(l[:i]) + `<a href="` + esc(it.TicketURL) + `">` + esc(tag) + `</a>` + esc(l[i+len(tag):])
		}
	}
	return esc(l)
}

func bulletinTitle(m *newsModel) string { return bulletinCut(bc(m.Job.Title), 72) + " bulletin" }

// bulletinHTMLBlock is one bulletin as <pre>; each footer release number links to hrefPrefix + N + ".html".
func bulletinHTMLBlock(m *newsModel, releases []int, hrefPrefix string) string {
	esc := html.EscapeString
	link := func(n int) string {
		u := (&url.URL{Path: hrefPrefix + strconv.Itoa(n) + ".html"}).String()
		return `<a href="` + esc(u) + `">` + strconv.Itoa(n) + `</a>`
	}
	lines := bulletinLines(m, releases)
	tl := newsTicketLinks(m)
	if !urlsSafe(tl) {
		tl = nil
	}
	var b strings.Builder
	b.WriteString("<pre>\n")
	for i, l := range lines {
		if i == len(lines)-2 { // the footer: release numbers become links
			b.WriteString(bulletinFooter(releases, link, esc) + "\n")
			continue
		}
		b.WriteString(linkTicket(l, tl, esc) + "\n")
	}
	b.WriteString("</pre>\n")
	return b.String()
}

// bulletinHTMLPage wraps bulletin blocks in a green-on-black page.
func bulletinHTMLPage(title string, blocks []string) string {
	return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + html.EscapeString(title) + `</title>
<style>
body{margin:0;background:#000;color:#33ff33;font:15px/1.3 ui-monospace,Menlo,Consolas,monospace}
pre{margin:0 auto;padding:16px;max-width:72ch;white-space:pre;overflow-x:auto}
a{color:#ffb000}a:visited{color:#cc8a00}
@media (prefers-color-scheme:light){body{background:#fffbe6;color:#1a4d1a}a{color:#8a5a00}}
</style></head><body>
` + strings.Join(blocks, "") + `</body></html>
`
}

func renderBulletinHTML(m *newsModel, releases []int, hrefPrefix string) string {
	return bulletinHTMLPage(bulletinTitle(m), []string{bulletinHTMLBlock(m, releases, hrefPrefix)})
}
