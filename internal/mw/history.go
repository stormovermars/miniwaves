package mw

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/stormovermars/miniwaves/internal/fsx"
)

// mw history: an append-only ledger of pushes, releases, store uploads and publishes, one file per node,
// .miniwaves/nodes/<node>.history.jsonl (so computers never write the same file; mw merge-file unions two
// versions line by line). query and last read every node's file, newest first.

var historyKinds = []string{"push", "release", "upload", "publish"}

type historyLine struct {
	At      string `json:"at"`
	Node    string `json:"node"`
	Kind    string `json:"kind"`
	Repo    string `json:"repo"`
	Ref     string `json:"ref"`
	Version string `json:"version,omitempty"`
	Target  string `json:"target,omitempty"`
	Row     string `json:"row,omitempty"`
	Note    string `json:"note,omitempty"`
}

func historyPath(board, node string) string {
	return storePath(board, filepath.Join("nodes", node+".history.jsonl"))
}

var (
	historyWord   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@+~-]{0,99}$`)
	secretLooking = regexp.MustCompile(`(?i)(ghp_|gho_|github_pat_|glpat-|sk-[a-z0-9_-]{20,}|akia[0-9a-z]{8}|xox[bap]-|-----begin |password\s*[=:]|passwd\s*[=:]|token\s*[=:]|secret\s*[=:]|api[_-]?key\s*[=:]|bearer\s)`)
)

func checkHistoryField(name, v string, word bool) error {
	if word && v != "" && !historyWord.MatchString(v) {
		return fmt.Errorf("%s %q is not valid (letters, digits and . _ / : @ + ~ -, at most 100)", name, v)
	}
	if len(v) > 300 {
		return fmt.Errorf("%s is longer than 300 characters", name)
	}
	if strings.ContainsAny(v, "\r\n\t\x00") || !noControl(v) {
		return fmt.Errorf("%s must be one line without control characters", name)
	}
	if secretLooking.MatchString(v) {
		return fmt.Errorf("%s looks like a secret: the ledger is committed and public-safe, never put keys, tokens or passwords in it", name)
	}
	return nil
}

func readHistory(board string) []historyLine {
	var out []historyLine
	dir := filepath.Dir(historyPath(board, "x"))
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".history.jsonl") {
			continue
		}
		b, err := fsx.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(bytes.NewReader(b))
		sc.Buffer(make([]byte, 1<<16), 1<<20)
		for sc.Scan() {
			var l historyLine
			if json.Unmarshal(sc.Bytes(), &l) == nil && l.At != "" {
				out = append(out, l)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { // newest first; same instant: by node, then content, so the order is stable
		if out[i].At != out[j].At {
			return out[i].At > out[j].At
		}
		a, _ := json.Marshal(out[i])
		b, _ := json.Marshal(out[j])
		return string(a) < string(b)
	})
	return out
}

func historyCommand(path string, w *Workspace, args []string) error {
	const use = "usage: mw history add KIND REPO REF [--version V] [--target T] [--row ID] [--note TEXT] | query [--node N] [--kind K] [--repo R] [--version V] [--since DATE] [-n N] [-j] [--changed] | last [REPO] [-j]"
	if len(args) == 0 {
		return errors.New(use)
	}
	o, pos, err := options(args[1:])
	if err != nil {
		return err
	}
	switch args[0] {
	case "add":
		if err := allowOptions(o, "version", "target", "row", "note", "message"); err != nil {
			return err
		}
		if len(pos) != 3 {
			return errors.New(use)
		}
		name := nodeName()
		if name == "" {
			return errors.New("no node name on this computer: run mw node set NAME")
		}
		l := historyLine{At: time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z"), Node: name, Kind: pos[0], Repo: pos[1], Ref: pos[2],
			Version: o["version"], Target: o["target"], Row: strings.ToUpper(o["row"]), Note: o["note"]}
		if l.Note == "" {
			l.Note = o["message"]
		}
		if !contains(historyKinds, l.Kind) {
			return fmt.Errorf("kind must be %s", strings.Join(historyKinds, "|"))
		}
		for _, f := range []struct {
			n, v string
			w    bool
		}{{"repo", l.Repo, true}, {"ref", l.Ref, true}, {"version", l.Version, true}, {"target", l.Target, false}, {"row", l.Row, true}, {"note", l.Note, false}} {
			if err := checkHistoryField(f.n, f.v, f.w); err != nil {
				return err
			}
		}
		b, _ := json.Marshal(l)
		p := historyPath(path, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, werr := f.Write(append(b, '\n')) // one write: two mw processes on one node never interleave a line
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return werr
		}
		fmt.Println(historyText(l))
		return nil
	case "query", "last":
		return historyRead(path, args[0], o, pos)
	}
	return errors.New(use)
}

func historyText(l historyLine) string {
	s := fmt.Sprintf("%s %s %s %s %s", l.At, l.Node, l.Kind, l.Repo, l.Ref)
	for _, kv := range [][2]string{{"v", l.Version}, {"to", l.Target}, {"row", l.Row}} {
		if kv[1] != "" {
			s += " " + kv[0] + "=" + kv[1]
		}
	}
	if l.Note != "" {
		s += "  " + l.Note
	}
	return s
}

func historyRead(path, mode string, o map[string]string, pos []string) error {
	allowed := []string{"json", "node", "kind", "repo", "version", "since", "limit", "changed", "session"}
	if err := allowOptions(o, allowed...); err != nil {
		return err
	}
	if mode == "last" {
		if len(pos) > 1 {
			return errors.New("usage: mw history last [REPO] [-j]")
		}
		if len(pos) == 1 {
			o["repo"] = pos[0]
		}
	} else if len(pos) != 0 {
		return fmt.Errorf("unexpected argument %q", pos[0])
	}
	var since time.Time
	if o["since"] != "" {
		var err error
		if since, err = time.Parse(time.RFC3339, o["since"]); err != nil {
			if since, err = time.Parse("2006-01-02", o["since"]); err != nil {
				return errors.New("--since needs a date (2026-11-14) or an RFC3339 time")
			}
		}
	}
	limit := 50
	if o["limit"] != "" {
		if _, e := fmt.Sscanf(o["limit"], "%d", &limit); e != nil || limit < 1 {
			return errors.New("-n needs a positive number")
		}
	}
	var out []historyLine
	seenRepo := map[string]bool{}
	for _, l := range readHistory(path) {
		switch {
		case o["node"] != "" && l.Node != o["node"], o["kind"] != "" && l.Kind != o["kind"], o["repo"] != "" && l.Repo != o["repo"],
			o["version"] != "" && l.Version != o["version"]:
			continue
		}
		if !since.IsZero() {
			if at, err := time.Parse(time.RFC3339, l.At); err != nil || at.Before(since) {
				continue
			}
		}
		if mode == "last" {
			if seenRepo[l.Repo] {
				continue
			}
			seenRepo[l.Repo] = true
		}
		out = append(out, l)
		if len(out) >= limit {
			break
		}
	}
	if out == nil {
		out = []historyLine{}
	}
	if o["changed"] == "true" {
		b, _ := json.Marshal(out)
		h := sha256.Sum256(b)
		sig := hex.EncodeToString(h[:])
		abs, _ := filepath.Abs(path)
		key := sha256.Sum256([]byte("history\x00" + abs + "\x00" + value(o, "session", os.Getenv("MW_SESSION")) + "\x00" + mode + strings.Join(pos, ",") + o["node"] + o["kind"] + o["repo"] + o["version"] + o["since"] + o["limit"]))
		p, err := userConfigPath()
		if err != nil {
			return err
		}
		sp := filepath.Join(filepath.Dir(p), "history-"+hex.EncodeToString(key[:6])+".json")
		if prev, err := fsx.ReadFile(sp); err == nil && strings.TrimSpace(string(prev)) == sig {
			fmt.Println("no change")
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(sp), 0o700); err != nil {
			return err
		}
		if err := writeFileAtomic(sp, []byte(sig+"\n")); err != nil {
			return err
		}
	}
	if o["json"] == "true" {
		return printJSON(out)
	}
	if len(out) == 0 {
		fmt.Println("no history lines")
		return nil
	}
	for _, l := range out {
		fmt.Println(historyText(l))
	}
	return nil
}

var releaseWord = regexp.MustCompile(`(?i)\brelease`)

// warnNoLedger tells (on stderr) that a row that looks like a release has no ledger line: done still succeeds.
func warnNoLedger(path string, t *Tenant, it *Item) {
	if !releaseWord.MatchString(it.Title) {
		return
	}
	for _, l := range readHistory(path) {
		if l.Row == it.ID || l.Row == strings.ToUpper(t.Slug)+":"+it.ID || strings.EqualFold(l.Row, t.Slug+":"+it.ID) {
			return
		}
	}
	fmt.Fprintf(os.Stderr, "warning: %s looks like a release but the ledger has no line for it: mw history add release REPO REF --row %s\n", it.ID, it.ID)
}

// mergeHistoryFiles unions two versions of a ledger file line by line, oldest first, without repeats.
func mergeHistoryFiles(parts ...[]byte) ([]byte, error) {
	seen := map[string]bool{}
	var lines []string
	for _, p := range parts {
		sc := bufio.NewScanner(bytes.NewReader(p))
		sc.Buffer(make([]byte, 1<<16), 1<<20)
		for sc.Scan() {
			if s := strings.TrimSpace(sc.Text()); s != "" && !seen[s] {
				seen[s] = true
				lines = append(lines, s)
			}
		}
		if err := sc.Err(); err != nil { // a line over 1 MB: fail loudly, never truncate the file
			return nil, fmt.Errorf("history file: %w", err)
		}
	}
	sort.Strings(lines) // every line starts with its UTC time
	if len(lines) == 0 {
		return nil, nil
	}
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}
