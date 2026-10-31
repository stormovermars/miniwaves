package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

// mw context --hook-json: the SessionStart hook of Claude Code (fires on startup, resume, clear and
// compact). It prints the claimed (doing) items of the session so that a compacted or resumed
// conversation knows what it holds. It is silent (no output, exit 0) when there is no board, no
// matching session or no claimed item, and it never fails: the hook must not break a session start.

var claudeSessionRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

const hookContextCap = 2000 // characters of additionalContext, tail included

type hookInput struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	Source    string `json:"source"`
}

// hookContextCommand reads the hook JSON from stdin (missing, empty or broken input is tolerated) and
// writes the hook output to stdout, or nothing. It always returns nil.
func hookContextCommand(stdin io.Reader, stdout io.Writer) (err error) {
	defer func() {
		_ = recover()
		err = nil
	}()
	var in hookInput
	if f, ok := stdin.(*os.File); ok {
		if fi, e := f.Stat(); e != nil || fi.Mode()&os.ModeCharDevice != 0 {
			stdin = nil // a terminal: nothing was piped in, do not wait for typing
		}
	}
	if stdin != nil {
		if b, e := io.ReadAll(io.LimitReader(stdin, 1<<20)); e == nil {
			_ = json.Unmarshal(b, &in)
		}
	}
	text := hookContextText(in)
	if text == "" {
		return nil
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	var o struct {
		H struct {
			Name string `json:"hookEventName"`
			Ctx  string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	o.H.Name, o.H.Ctx = "SessionStart", text
	_ = enc.Encode(o)
	_, _ = stdout.Write(out.Bytes())
	return nil
}

// hookContextText is the additionalContext for the session found from MW_SESSION, else from the Claude
// session id, or "" when there is nothing to say.
func hookContextText(in hookInput) string {
	dir := in.Cwd
	if dir == "" {
		dir = mustGetwd()
	}
	path := statusFindBoard(dir)
	if path == "" {
		return ""
	}
	w, err := load(path)
	if err != nil {
		return ""
	}
	var s *Session
	if name := os.Getenv("MW_SESSION"); name != "" {
		s, _ = sessionByID(w, name)
	}
	if s == nil && in.SessionID != "" {
		for i := range w.Sessions {
			if w.Sessions[i].ClaudeSession == in.SessionID {
				s = &w.Sessions[i]
				break
			}
		}
	}
	if s == nil || s.State == "stopped" {
		return ""
	}
	t, err := tenantBySlug(w, s.Tenant)
	if err != nil {
		return ""
	}
	var doing []Item
	for _, it := range t.Items {
		if it.Status == "doing" && it.Assignee == s.ID && it.Kind != "wave" {
			doing = append(doing, it)
		}
	}
	if len(doing) == 0 {
		return ""
	}
	gs := gateStates(w, t)
	var b strings.Builder
	head := fmt.Sprintf("miniwaves: session %s holds %d claimed item(s) on tenant %s", displayName(s.ID), len(doing), t.Slug)
	if in.Source != "" {
		head += " (source: " + compactText(in.Source, 20) + ")"
	}
	b.WriteString(head + ". Continue it; finish with mw done ID NOTE or mw block ID REASON.\n")
	if g := gatesLine(t, gs); g != "" {
		b.WriteString(g)
		if byName(gs, "evidence").Effective {
			b.WriteString("; evidence is checked at done")
		}
		b.WriteString("\n")
	}
	for _, it := range doing {
		fmt.Fprintf(&b, "\n%s  %s  %s\n", it.ID, it.Kind, compactText(it.Title, 72))
		if a, _ := acceptanceSource(t, it); strings.TrimSpace(a) != "" {
			fmt.Fprintf(&b, "  acceptance: %s\n", compactText(a, 300))
		}
		notes := it.Notes
		if len(notes) > 3 {
			notes = notes[len(notes)-3:]
		}
		for _, n := range notes {
			if i := strings.IndexByte(n, ' '); i > 0 && strings.Contains(n[:i], "T") { // drop the timestamp
				n = n[i+1:]
			}
			fmt.Fprintf(&b, "  note: %s\n", compactText(n, 160))
		}
		from := 0
		for i, e := range it.Events {
			if e.Action == "start" {
				from = i // the latest claim
			}
		}
		for _, e := range it.Events[from:] {
			if e.Action == "answer" || e.Action == "decide" {
				fmt.Fprintf(&b, "  %s by %s: %s\n", e.Action, orDash(e.By), compactText(e.Note, 300))
			}
		}
	}
	text := strings.TrimRight(b.String(), "\n")
	if utf8.RuneCountInString(text) > hookContextCap {
		tail := "… (mw show " + doing[0].ID + " --full)"
		r := []rune(text)
		text = strings.TrimRight(string(r[:hookContextCap-utf8.RuneCountInString(tail)-1]), " \n") + "\n" + tail
	}
	return text
}
