package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
)

// mw log --since CURSOR: an event feed a poller can resume.
//
// The cursor is opaque (base64url of {"t": newest delivered timestamp, "seen": [8-byte key hashes of the delivered
// events stamped within the window (t-24h, t]]}), so it stays bounded by the window however long the board is. An
// event's key is tenant/item/H, H being 12 hex digits of sha256(at|action|by|note): content, not position, so a
// git merge that reorders an item's events cannot make one event inherit another's key. A read returns the events
// stamped after t, and those stamped within the window up to t whose key hash is not in seen: a merge from a slower
// clone, or a tenant that stamped before another's save, still arrives, once. Events older than t minus the window
// are skipped: read from "start" to see them. Old-form cursors (seen holding the full tenant/item/H keys) still
// match exactly.

// cursorWindow is how far behind t a late event is still delivered.
const cursorWindow = 24 * time.Hour

// keyHash is the 8-byte form of an event key stored in a cursor.
func keyHash(k string) string {
	h := sha256.Sum256([]byte(k))
	return hex.EncodeToString(h[:8])
}

type logCursor struct {
	T    string   `json:"t,omitempty"`
	Seen []string `json:"seen,omitempty"`
}

func encodeCursor(c logCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeCursor treats "" and "start" as the beginning of time.
func decodeCursor(s string) (logCursor, error) {
	var c logCursor
	s = strings.TrimSpace(s)
	if s == "" || s == "start" {
		return c, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
	if err == nil {
		err = json.Unmarshal(b, &c)
	}
	if err == nil && c.T != "" {
		_, err = time.Parse(time.RFC3339Nano, c.T)
	}
	if err != nil {
		return logCursor{}, errors.New("invalid --since cursor (pass the next value of a previous mw log --since, or start)")
	}
	return c, nil
}

// eventKey is tenant/item/H; n numbers byte-identical events of one item (0 for the first, no suffix).
func eventKey(tenant, item string, e Event, n int) string {
	h := sha256.Sum256([]byte(e.At + "|" + e.Action + "|" + e.By + "|" + e.Note))
	k := tenant + "/" + item + "/" + hex.EncodeToString(h[:6])
	if n > 0 {
		k += "." + strconv.Itoa(n)
	}
	return k
}

type keyedEvent struct {
	row logRow
	ts  time.Time
	idx int
}

// logSince returns the events the cursor has not delivered, oldest first (at most limit when limit > 0),
// and the next cursor.
func logSince(tenants []*Tenant, cursor string, limit int) ([]logRow, string, error) {
	cur, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	var curT time.Time
	if cur.T != "" {
		curT, _ = time.Parse(time.RFC3339Nano, cur.T)
	}
	hasT := cur.T != ""
	seen := make(map[string]bool, len(cur.Seen))
	for _, k := range cur.Seen {
		seen[k] = true
	}
	var fresh []keyedEvent
	var all []keyedEvent // every board event with its key, to rebuild the seen set
	for _, t := range tenants {
		for _, it := range t.Items {
			dup := map[string]int{}
			for i, e := range it.Events {
				base := eventKey(t.Slug, it.ID, e, 0)
				k := eventKey(t.Slug, it.ID, e, dup[base])
				dup[base]++
				ts := parseTS(e.At)
				if was0 := seen[keyHash(k)] || seen[k]; was0 {
					all = append(all, keyedEvent{row: logRow{Key: k}, ts: ts})
				}
				kh := keyHash(k)
				was := seen[kh] || seen[k]
				if hasT {
					if !ts.After(curT) && (was || !ts.After(curT.Add(-cursorWindow))) {
						continue
					}
				}
				ke := keyedEvent{row: logRow{At: e.At, Tenant: t.Slug, Item: it.ID, Action: e.Action, By: e.By, Note: e.Note, Key: k}, ts: ts, idx: i}
				fresh = append(fresh, ke)
			}
		}
	}
	sort.SliceStable(fresh, func(a, b int) bool {
		x, y := fresh[a], fresh[b]
		if !x.ts.Equal(y.ts) {
			return x.ts.Before(y.ts)
		}
		if x.row.Tenant != y.row.Tenant {
			return x.row.Tenant < y.row.Tenant
		}
		if x.row.Item != y.row.Item {
			return x.row.Item < y.row.Item
		}
		return x.idx < y.idx
	})
	if limit > 0 && len(fresh) > limit {
		fresh = fresh[:limit]
	}
	newT := curT
	rows := make([]logRow, 0, len(fresh))
	for _, ke := range fresh {
		rows = append(rows, ke.row)
		if ke.ts.After(newT) {
			newT = ke.ts
		}
	}
	next := logCursor{}
	if !newT.IsZero() {
		next.T = newT.UTC().Format(time.RFC3339Nano)
		lo := newT.Add(-cursorWindow)
		set := map[string]bool{}
		for _, ke := range all { // delivered earlier and still on the board
			if ke.ts.After(lo) && !ke.ts.After(newT) {
				set[keyHash(ke.row.Key)] = true
			}
		}
		for _, ke := range fresh { // delivered now
			if ke.ts.After(lo) && !ke.ts.After(newT) {
				set[keyHash(ke.row.Key)] = true
			}
		}
		for k := range set {
			next.Seen = append(next.Seen, k)
		}
		sort.Strings(next.Seen)
	}
	return rows, encodeCursor(next), nil
}
