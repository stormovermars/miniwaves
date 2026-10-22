package main

import (
	"strings"
	"unicode"
)

// Terminal-cell helpers for the bulletin: width of runes (East Asian wide and combining marks), cutting,
// padding and word wrapping. There is no x/text dependency, so this is a small built-in table, not a full wcwidth.

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
		r >= 0x20000 && r <= 0x3FFFD,
		r >= 0x1F680 && r <= 0x1F6FF, r >= 0x1FA70 && r <= 0x1FAFF, r >= 0x1F1E6 && r <= 0x1F1FF,
		r == 0x231A, r == 0x231B, r >= 0x23E9 && r <= 0x23EC, r == 0x2614, r == 0x2615, r >= 0x2648 && r <= 0x2653, r == 0x267F, r == 0x2693, r == 0x26A1,
		r == 0x26AA, r == 0x26AB, r == 0x26BD, r == 0x26BE, r == 0x26C4, r == 0x26C5, r == 0x26CE, r == 0x26D4, r == 0x26EA, r == 0x26F2, r == 0x26F3, r == 0x26F5, r == 0x26FA, r == 0x26FD,
		r == 0x2705, r == 0x270A, r == 0x270B, r == 0x2728, r == 0x274C, r == 0x274E, r >= 0x2753 && r <= 0x2755, r == 0x2757, r >= 0x2795 && r <= 0x2797, r == 0x27B0, r == 0x27BF,
		r == 0x2B1B, r == 0x2B1C, r == 0x2B50, r == 0x2B55:
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
