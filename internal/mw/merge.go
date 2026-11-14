package mw

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// mw merge-file BASE OURS THEIRS [PATH]: a git merge driver for the board files. It works on the JSON,
// not on lines: rows are merged by id, fields by "newest wins", logs as a union ordered by time. The
// result is written to OURS (git's contract); no git command runs here. See USAGE.md, "Fleets".

type missingT struct{}

var missing = &missingT{} // an absent key, distinct from a JSON null

func decodeDoc(b []byte) (map[string]any, error) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return map[string]any{}, nil
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var m map[string]any
	if err := d.Decode(&m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func same(a, b any) bool { return reflect.DeepEqual(a, b) }

func get(m map[string]any, k string) any {
	if v, ok := m[k]; ok {
		return v
	}
	return missing
}

func asMap(v any) (map[string]any, bool) { m, ok := v.(map[string]any); return m, ok }

func parseNano(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// rowStamp is when a row last changed: its updated time or its newest log entry, whichever is later.
func rowStamp(r map[string]any) time.Time {
	t := parseNano(strOf(r["updated"]))
	if ev, ok := r["events"].([]any); ok {
		for _, e := range ev {
			if em, ok := asMap(e); ok {
				if x := parseNano(strOf(em["at"])); x.After(t) {
					t = x
				}
			}
		}
	}
	return t
}

func strOf(v any) string { s, _ := v.(string); return s }

// docStamp is the newest stamp of any row of a document (its "later writer" clock).
func docStamp(d map[string]any) time.Time {
	var t time.Time
	for _, k := range []string{"items", "sessions", "tenants"} {
		if rows, ok := d[k].([]any); ok {
			for _, r := range rows {
				if rm, ok := asMap(r); ok {
					if x := rowStamp(rm); x.After(t) {
						t = x
					}
				}
			}
		}
	}
	return t
}

// laterOf says whether theirs is the later writer (ties go to theirs).
func theirsLater(o, t time.Time) bool { return !o.After(t) }

type merger struct {
	shape  string // tenant | index | archive
	oStamp time.Time
	tStamp time.Time
	used   map[string]bool // item ids in use (renumbering)
}

func (m *merger) pick(o, t any, ot, tt time.Time) any {
	if theirsLater(ot, tt) {
		return t
	}
	return o
}

// val merges one value three ways; later is the pair of stamps deciding a real conflict.
func (m *merger) val(b, o, t any, ot, tt time.Time) (any, bool) {
	switch {
	case same(o, t):
		return o, false
	case same(o, b):
		return t, false
	case same(t, b):
		return o, false
	}
	if om, ok := asMap(o); ok {
		if tm, ok := asMap(t); ok {
			bm, _ := asMap(b)
			return m.obj(bm, om, tm, ot, tt), false
		}
	}
	return m.pick(o, t, ot, tt), true
}

// obj merges two JSON objects key by key (plain keys only: no row lists).
func (m *merger) obj(b, o, t map[string]any, ot, tt time.Time) map[string]any {
	out := map[string]any{}
	keys := map[string]bool{}
	for _, mm := range []map[string]any{b, o, t} {
		for k := range mm {
			keys[k] = true
		}
	}
	for k := range keys {
		v, _ := m.val(get(b, k), get(o, k), get(t, k), ot, tt)
		if v != missing {
			out[k] = v
		}
	}
	return out
}

func rowsByKey(rows []any, key string) (map[string]map[string]any, []string) {
	idx := map[string]map[string]any{}
	var order []string
	for _, r := range rows {
		if rm, ok := asMap(r); ok {
			id := strOf(rm[key])
			if _, dup := idx[id]; !dup {
				order = append(order, id)
			}
			idx[id] = rm
		}
	}
	return idx, order
}

func asRows(v any) []any { r, _ := v.([]any); return r }

// rows merges a list of objects keyed by key (id for items and sessions, slug for tenants).
func (m *merger) rows(b, o, t []any, key string) []any {
	bi, _ := rowsByKey(b, key)
	oi, oo := rowsByKey(o, key)
	ti, to := rowsByKey(t, key)
	order := append([]string(nil), oo...)
	for _, id := range to {
		if _, ok := oi[id]; !ok {
			order = append(order, id)
		}
	}
	var out []any
	for _, id := range order {
		br, hasB := bi[id]
		or, hasO := oi[id]
		tr, hasT := ti[id]
		switch {
		case hasO && hasT:
			if !hasB && key == "id" && m.shape != "archive" && !sameRowOrigin(or, tr) {
				tr = m.renumber(tr, or)
				out = append(out, or, tr)
				continue
			}
			if !hasB {
				br = nil
			}
			out = append(out, m.row(br, or, tr))
		case hasO:
			if hasB && same(or, br) {
				continue // deleted on their side, untouched on ours
			}
			out = append(out, or)
		default:
			if hasB && same(tr, br) {
				continue
			}
			out = append(out, tr)
		}
	}
	return out
}

func sameRowOrigin(a, b map[string]any) bool {
	return same(a["created"], b["created"]) && same(a["title"], b["title"]) && same(a["kind"], b["kind"])
}

var idNum = func(id string) (string, int, bool) {
	i := strings.LastIndex(id, "-")
	if i < 1 {
		return "", 0, false
	}
	n, err := strconv.Atoi(id[i+1:])
	return id[:i], n, err == nil
}

// renumber gives theirs, which collided with ours on a new id, the next free id of its prefix.
func (m *merger) renumber(t, o map[string]any) map[string]any {
	old := strOf(t["id"])
	pfx, _, ok := idNum(old)
	if !ok {
		pfx = old
	}
	max := 0
	for id := range m.used {
		if p, n, ok := idNum(id); ok && p == pfx && n > max {
			max = n
		}
	}
	nid := fmt.Sprintf("%s-%03d", pfx, max+1)
	m.used[nid] = true
	nt := map[string]any{}
	for k, v := range t {
		nt[k] = v
	}
	nt["id"] = nid
	ev, _ := nt["events"].([]any)
	at := rowStamp(o)
	if x := rowStamp(t); x.After(at) {
		at = x
	}
	nt["events"] = append(append([]any(nil), ev...), map[string]any{"at": at.Add(time.Nanosecond).UTC().Format(time.RFC3339Nano), "action": "merge", "note": "renumbered from " + old + ": the same id was added on two computers"})
	return nt
}

func evKey(e any) string {
	em, _ := asMap(e)
	return strOf(em["at"]) + "|" + strOf(em["action"]) + "|" + strOf(em["by"]) + "|" + strOf(em["note"]) + "|" + strOf(em["on"])
}

func unionEvents(lists ...[]any) []any {
	seen := map[string]bool{}
	var out []any
	for _, l := range lists {
		for _, e := range l {
			if k := evKey(e); !seen[k] {
				seen[k] = true
				out = append(out, e)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := parseNano(strOf(out[i].(map[string]any)["at"])), parseNano(strOf(out[j].(map[string]any)["at"]))
		return a.Before(b)
	})
	return out
}

func unionStrings(lists ...[]any) []any {
	seen := map[string]bool{}
	var out []any
	for _, l := range lists {
		for _, e := range l {
			k, _ := json.Marshal(e)
			if !seen[string(k)] {
				seen[string(k)] = true
				out = append(out, e)
			}
		}
	}
	return out
}

func short(v any) string {
	b, _ := json.Marshal(v)
	s := string(b)
	if len(s) > 80 {
		s = s[:80] + "..."
	}
	return s
}

// row merges one object that exists on both sides, field by field.
func (m *merger) row(b, o, t map[string]any) map[string]any {
	ot, tt := rowStamp(o), rowStamp(t)
	out := map[string]any{}
	keys := map[string]bool{}
	for _, mm := range []map[string]any{b, o, t} {
		for k := range mm {
			keys[k] = true
		}
	}
	var conflicts []string
	for k := range keys {
		var v any
		switch k {
		case "events":
			v = unionEvents(asRows(o[k]), asRows(t[k]))
		case "notes", "evidence":
			v = unionStrings(asRows(o[k]), asRows(t[k]))
		case "updated":
			v = o[k]
			if parseNano(strOf(t[k])).After(parseNano(strOf(o[k]))) {
				v = t[k]
			}
		default:
			var c bool
			v, c = m.val(get(b, k), get(o, k), get(t, k), ot, tt)
			if c && ot.Unix() == tt.Unix() {
				conflicts = append(conflicts, fmt.Sprintf("%s: ours %s, theirs %s", k, short(unmissing(get(o, k))), short(unmissing(get(t, k)))))
			}
		}
		if v != nil && v != missing {
			if l, ok := v.([]any); ok && len(l) == 0 && k != "items" {
				continue
			}
			out[k] = v
		}
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		at := ot
		if tt.After(at) {
			at = tt
		}
		ev, _ := out["events"].([]any)
		out["events"] = unionEvents(ev, []any{map[string]any{"at": at.Add(time.Nanosecond).UTC().Format(time.RFC3339Nano), "action": "merge", "note": "conflict, later writer kept: " + strings.Join(conflicts, "; ")}})
	}
	return out
}

func unmissing(v any) any {
	if v == missing {
		return nil
	}
	return v
}

func shapeOf(docs ...map[string]any) string {
	for _, d := range docs {
		if _, ok := d["tenants"]; ok {
			return "index"
		}
	}
	for _, d := range docs {
		if _, ok := d["node"]; ok {
			if _, ok := d["at"]; ok {
				return "node"
			}
		}
	}
	for _, d := range docs {
		if _, ok := d["next"]; ok {
			return "tenant"
		}
	}
	for _, d := range docs {
		if _, ok := d["items"]; ok {
			return "archive"
		}
	}
	return ""
}

// mergeBoardFiles merges three versions of one board file and returns the bytes to store.
func mergeBoardFiles(base, ours, theirs []byte) ([]byte, error) {
	bd, err := decodeDoc(base)
	if err != nil {
		return nil, fmt.Errorf("base: %w", err)
	}
	od, err := decodeDoc(ours)
	if err != nil {
		return nil, fmt.Errorf("ours: %w", err)
	}
	td, err := decodeDoc(theirs)
	if err != nil {
		return nil, fmt.Errorf("theirs: %w", err)
	}
	m := &merger{shape: shapeOf(bd, od, td), oStamp: docStamp(od), tStamp: docStamp(td), used: map[string]bool{}}
	var merged map[string]any
	switch m.shape {
	case "node": // one writer per file: the newer beat wins
		merged = od
		if !parseNano(strOf(od["at"])).After(parseNano(strOf(td["at"]))) {
			merged = td
		}
	case "":
		return nil, errors.New("not a miniwaves board file")
	default:
		for _, d := range []map[string]any{bd, od, td} {
			for _, r := range asRows(d["items"]) {
				if rm, ok := asMap(r); ok {
					m.used[strOf(rm["id"])] = true
				}
			}
		}
		merged = map[string]any{}
		keys := map[string]bool{}
		for _, d := range []map[string]any{bd, od, td} {
			for k := range d {
				keys[k] = true
			}
		}
		for k := range keys {
			var v any
			switch k {
			case "items":
				v = m.rows(asRows(bd[k]), asRows(od[k]), asRows(td[k]), "id")
			case "sessions":
				v = m.rows(asRows(bd[k]), asRows(od[k]), asRows(td[k]), "id")
			case "tenants":
				v = m.rows(asRows(bd[k]), asRows(od[k]), asRows(td[k]), "slug")
			default:
				v, _ = m.val(get(bd, k), get(od, k), get(td, k), m.oStamp, m.tStamp)
			}
			if v != nil && v != missing {
				merged[k] = v
			}
		}
		if next, ok := merged["next"].(map[string]any); ok { // counters: never behind an id in use
			for _, a := range []map[string]any{od, td, bd} {
				if n, ok := asMap(a["next"]); ok {
					for p, v := range n {
						if f, err := strconv.Atoi(fmt.Sprint(v)); err == nil {
							if cur, _ := strconv.Atoi(fmt.Sprint(next[p])); f > cur {
								next[p] = json.Number(strconv.Itoa(f))
							}
						}
					}
				}
			}
			for id := range m.used {
				if p, n, ok := idNum(id); ok {
					if cur, _ := strconv.Atoi(fmt.Sprint(next[p])); n > cur {
						next[p] = json.Number(strconv.Itoa(n))
					}
				}
			}
		}
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return nil, err
	}
	var typed any
	switch m.shape {
	case "index":
		typed = &indexDoc{}
	case "tenant":
		typed = &tenantDoc{}
	case "archive":
		typed = &archiveDoc{}
	case "node":
		typed = &nodeBeat{}
	}
	if err := json.Unmarshal(raw, typed); err != nil {
		return nil, err
	}
	out, err := json.Marshal(typed)
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// mergeFileCommand is mw merge-file BASE OURS THEIRS [PATH]: the result replaces OURS.
func mergeFileCommand(args []string) error {
	if len(args) < 3 || len(args) > 4 {
		return errors.New("usage: mw merge-file BASE OURS THEIRS [PATH]  (a git merge driver, see USAGE.md \"Fleets\")")
	}
	read := func(p string) ([]byte, error) {
		b, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			return nil, nil
		}
		return b, err
	}
	var in [3][]byte
	for i := range in {
		b, err := read(args[i])
		if err != nil {
			return err
		}
		in[i] = b
	}
	out, err := mergeBoardFiles(in[0], in[1], in[2])
	if err != nil {
		return fmt.Errorf("merge-file: %w", err)
	}
	return os.WriteFile(args[1], out, 0o600)
}
