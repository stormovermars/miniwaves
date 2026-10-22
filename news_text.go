package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// mw news --format text: plain text for terminals. No ANSI or colour codes; styling is box-drawing and
// symbol characters only. Board text is stripped of control characters (ESC included) so it cannot inject
// terminal codes, and it is shown as it is (no Markdown or HTML escapes). Output goes to stdout unless -o
// PATH is given; there is no news.out default and no hash sidecar.

const (
	newsTextMin     = 40
	newsTextMax     = 160
	newsTextDefault = 80
	newsBarLen      = 20
)

// newsTextWidth picks the width: --width, else 80 ($COLUMNS is not read); clamped to 40..160.
func newsTextWidth(flag string) (int, error) {
	w := newsTextDefault
	if flag != "" {
		n, err := strconv.Atoi(strings.TrimSpace(flag))
		if err != nil {
			return 0, fmt.Errorf("--width %q: use a number", flag)
		}
		w = n
	}
	if w < newsTextMin {
		w = newsTextMin
	}
	if w > newsTextMax {
		w = newsTextMax
	}
	return w, nil
}

// newsClean makes board text safe for a terminal: whitespace controls become spaces, every other control,
// format (bidi, zero-width) and invalid rune is dropped. keepNL keeps newlines (summary text).
func newsClean(s string, keepNL bool) string {
	s = strings.ToValidUTF8(s, "")
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			if keepNL {
				return r
			}
			return ' '
		case r == '\t' || r == '\r':
			return ' '
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == 0xFFFD:
			return -1
		}
		return r
	}, s)
}

// newsRuneWidth is the terminal cell width of a rune: 0 for combining marks, 2 for East Asian wide and
// emoji ranges, else 1. There is no width helper elsewhere in mw and no x/text dependency, so this is a
// small built-in table, not a full wcwidth.
func newsRuneWidth(r rune) int {
	switch {
	case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r):
		return 0
	case r >= 0x1100 && r <= 0x115F, r >= 0x2E80 && r <= 0xA4CF, r >= 0xAC00 && r <= 0xD7A3,
		r >= 0xF900 && r <= 0xFAFF, r >= 0xFE30 && r <= 0xFE6F, r >= 0xFF00 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6, r >= 0x1F300 && r <= 0x1F64F, r >= 0x1F900 && r <= 0x1F9FF,
		r >= 0x20000 && r <= 0x3FFFD:
		return 2
	}
	return 1
}

func newsStrWidth(s string) int {
	n := 0
	for _, r := range s {
		n += newsRuneWidth(r)
	}
	return n
}

// newsCut cuts s to at most w cells, with no ellipsis.
func newsCut(s string, w int) string {
	n := 0
	for i, r := range s {
		rw := newsRuneWidth(r)
		if n+rw > w {
			return s[:i]
		}
		n += rw
	}
	return s
}

func newsPad(s string, w int) string {
	if d := w - newsStrWidth(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func newsPadLeft(s string, w int) string {
	if d := w - newsStrWidth(s); d > 0 {
		return strings.Repeat(" ", d) + s
	}
	return s
}

// newsWrap wraps one paragraph at word boundaries (long words are broken by cell width); the first line
// starts with first, the others with rest (a hanging indent).
func newsWrap(s string, width int, first, rest string) []string {
	var out []string
	line, prefix := first, first
	have := false
	room := func() int { return width - newsStrWidth(prefix) }
	flush := func() {
		out = append(out, line)
		prefix = rest
		line, have = rest, false
	}
	for _, word := range strings.Fields(s) {
		for newsStrWidth(word) > room() && room() > 0 { // a word longer than the line
			if have {
				flush()
				continue
			}
			part := newsCut(word, room())
			if part == "" {
				part = word[:len(string([]rune(word)[:1]))]
			}
			line += part
			word = word[len(part):]
			flush()
		}
		if have && newsStrWidth(line)+1+newsStrWidth(word) > width {
			flush()
		}
		if have {
			line += " "
		}
		line += word
		have = true
	}
	if have || len(out) == 0 {
		out = append(out, line)
	}
	return out
}

func newsRule(head string, width int) string {
	s := "── " + head + " "
	if n := width - newsStrWidth(s); n > 0 {
		s += strings.Repeat("─", n)
	}
	return newsCut(s, width)
}

func newsBar(progress string, n int) string {
	var dn, tot int
	if _, err := fmt.Sscanf(progress, "%d/%d", &dn, &tot); err != nil || tot <= 0 {
		return strings.Repeat("░", n)
	}
	f := (dn*n + tot/2) / tot
	if f > n {
		f = n
	}
	return strings.Repeat("█", f) + strings.Repeat("░", n-f)
}

// renderNewsText renders one job as plain text at the given width.
func renderNewsText(p newsPart, d newsData, width int) string {
	var b strings.Builder
	c := func(s string) string { return newsClean(s, false) }
	multi := len(p.tenants) > 1
	title := "══ " + newsCut(c(compactText(c(p.title), 72)), width-4) + " "
	if n := width - newsStrWidth(title); n > 0 {
		title += strings.Repeat("═", n)
	}
	b.WriteString(title + "\n")
	fmt.Fprintf(&b, "Updated %s\n", newsStampMarker)
	if p.url != "" {
		for _, l := range newsWrap("Page: "+c(p.url), width, "", "      ") {
			b.WriteString(l + "\n")
		}
	}
	head := strings.ReplaceAll(newsHeadline(d), newsMovedMarker, "") + "."
	b.WriteString("\n" + newsRule("HEADLINE", width) + "\n")
	for _, l := range newsWrap(head, width, "", "") {
		b.WriteString(l + "\n")
	}
	list := func(label string, rows []newsLine) {
		b.WriteString("\n" + newsRule(label, width) + "\n")
		if len(rows) == 0 {
			b.WriteString("▸ Nothing.\n")
		}
		for i, r := range rows {
			if i == newsListCap {
				fmt.Fprintf(&b, "▸ ... %d more\n", len(rows)-i)
				break
			}
			r.ID, r.Tenant, r.Title, r.Detail = c(r.ID), c(r.Tenant), c(r.Title), c(r.Detail)
			for _, l := range newsWrap(newsRowText(r, multi), width, "▸ ", "  ") {
				b.WriteString(l + "\n")
			}
		}
	}
	list("! NEEDS YOU", d.needs)
	list("◐ BLOCKED", d.blocked)
	list("IN PROGRESS", d.doing)

	b.WriteString("\n" + newsRule("✓ MILESTONES", width) + "\n")
	if len(d.miles) == 0 {
		b.WriteString("▸ None.\n")
	} else {
		rows := d.miles
		more := 0
		if len(rows) > newsMilestones {
			more, rows = len(rows)-newsMilestones, rows[:newsMilestones]
		}
		dw, tw := 0, 0
		for _, m := range rows {
			a, z, _ := strings.Cut(m.Progress, "/")
			dw, tw = max(dw, len(a)), max(tw, len(z))
		}
		prog := func(s string) string {
			a, z, _ := strings.Cut(s, "/")
			return newsPadLeft(a, dw) + "/" + newsPad(z, tw)
		}
		pw := max(dw+1+tw, len("DONE/TOTAL"))
		tenants := map[string]bool{}
		natural, ten, st := 9, 0, 5
		for _, m := range rows {
			tenants[m.Tenant] = true
			natural = max(natural, newsStrWidth(c(m.ID)+" "+c(compactText(m.Title, 60))))
			st = max(st, newsStrWidth(c(m.State)))
		}
		showTen := len(tenants) > 1 // one shared tenant: the column adds nothing
		if showTen {
			ten = 6
			for _, m := range rows {
				ten = max(ten, newsStrWidth(c(m.Tenant)))
			}
			ten = min(ten, 14)
		}
		st = min(st, 10)
		cols := 4
		if showTen {
			cols = 5
		}
		// row = "│ " + cells joined by " │ " + " │": sum(cells) + 3*cols + 1. State always stays; the bar
		// shrinks first (20 down to 8), then the milestone name is cut (min 4), then the tenant (min 2), then the DONE/TOTAL header.
		barW, mw := newsBarLen, natural
		over := func() int { return mw + ten + pw + st + barW + 3*cols + 1 - width }
		for over() > 0 && barW > 8 {
			barW--
		}
		for over() > 0 && mw > 4 {
			mw--
		}
		for over() > 0 && showTen && ten > 2 {
			ten--
		}
		for over() > 0 && pw > dw+1+tw { // last: the DONE/TOTAL header gives way to the data width
			pw--
		}
		cells := func(a, t, p, s, g string) string {
			line := "│ " + newsPad(newsCut(a, mw), mw) + " │ "
			if showTen {
				line += newsPad(newsCut(t, ten), ten) + " │ "
			}
			return line + newsPad(newsCut(p, pw), pw) + " │ " + newsPad(newsCut(s, st), st) + " │ " + newsPad(newsCut(g, barW), barW) + " │"
		}
		seg := func(n int) string { return strings.Repeat("─", n+2) }
		rule := "├" + seg(mw) + "┼"
		if showTen {
			rule += seg(ten) + "┼"
		}
		rule += seg(pw) + "┼" + seg(st) + "┼" + seg(barW) + "┤"
		b.WriteString(cells("MILESTONE", "TENANT", newsPad("DONE/TOTAL", pw), "STATE", "PROGRESS") + "\n" + rule + "\n")
		for _, m := range rows {
			b.WriteString(cells(c(m.ID)+" "+c(compactText(m.Title, 60)), c(m.Tenant), newsPad(prog(m.Progress), pw), c(m.State), newsBar(m.Progress, barW)) + "\n")
		}
		if more > 0 {
			fmt.Fprintf(&b, "▸ ... %d more\n", more)
		}
	}

	ev := func(e newsEvent) string {
		return shortTS(e.row.At) + " " + newsEventText(newsEvent{newsCleanRow(e.row), c(e.title)})
	}
	b.WriteString("\n" + newsRule("DECISIONS", width) + "\n")
	if len(d.decisions) == 0 {
		b.WriteString("▸ None.\n")
	}
	for _, e := range d.decisions {
		for _, l := range newsWrap(ev(e), width, "▸ ", "  ") {
			b.WriteString(l + "\n")
		}
	}
	b.WriteString("\n" + newsRule("LAST EVENTS", width) + "\n")
	for _, e := range d.events {
		b.WriteString(newsCut("▸ "+ev(e), width) + "\n")
	}
	if p.summary != "" {
		inner := width - 4
		b.WriteString("\n┌─ Summary " + strings.Repeat("─", max(width-len("┌─ Summary ")-1, 0)) + "┐\n")
		for _, para := range strings.Split(newsClean(p.summary, true), "\n") {
			for _, l := range newsWrap(strings.TrimRight(para, " "), inner, "", "") {
				b.WriteString("│ " + newsPad(l, inner) + " │\n")
			}
		}
		b.WriteString("└" + strings.Repeat("─", width-2) + "┘\n")
	}
	return b.String()
}

func newsCleanRow(r logRow) logRow {
	c := func(s string) string { return newsClean(s, false) }
	r.Action, r.Item, r.Tenant, r.By, r.Note = c(r.Action), c(r.Item), c(r.Tenant), c(r.By), c(r.Note)
	return r
}

// newsTextOut renders every part and writes to stdout, or to out (a plain atomic write, no hash).
func newsTextOut(parts []newsPart, datas []newsData, width int, out string) error {
	docs := make([]string, len(parts))
	for i := range parts {
		docs[i] = renderNewsText(parts[i], datas[i], width)
	}
	content := newsFill(strings.Join(docs, "\n"), time.Now(), 0, false, false)
	if out == "" {
		fmt.Print(content)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	return writeFileAtomicMode(out, []byte(content), 0644)
}
