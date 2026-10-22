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
// The cursor is opaque (base64url of {"t": last timestamp, "seen": [keys of the delivered events stamped exactly t]}),
// so it stays small however long the board is. An event's key is tenant/item/H, H being 12 hex digits of
// sha256(at|action|by|note): content, not position, so a git merge that reorders an item's events cannot make one
// event inherit another's key. A read returns the events stamped after t, and those stamped at t whose key is not
// in seen. An event stamped before t that reaches the board later (a merge from a slower clone) is not returned:
// read from "start" to see it.

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
	onBoard := map[string]bool{} // seen keys still on the board (an archived item drops out of the cursor)
	for _, t := range tenants {
		for _, it := range t.Items {
			dup := map[string]int{}
			for i, e := range it.Events {
				base := eventKey(t.Slug, it.ID, e, 0)
				k := eventKey(t.Slug, it.ID, e, dup[base])
				dup[base]++
				ts := parseTS(e.At)
				if seen[k] {
					onBoard[k] = true
				}
				if hasT && (ts.Before(curT) || ts.Equal(curT) && seen[k]) {
					continue
				}
				fresh = append(fresh, keyedEvent{row: logRow{At: e.At, Tenant: t.Slug, Item: it.ID, Action: e.Action, By: e.By, Note: e.Note, Key: k}, ts: ts, idx: i})
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
		if newT.Equal(curT) { // t did not move: what was seen at t still counts
			for k := range onBoard {
				next.Seen = append(next.Seen, k)
			}
		}
		for _, ke := range fresh {
			if ke.ts.Equal(newT) {
				next.Seen = append(next.Seen, ke.row.Key)
			}
		}
		sort.Strings(next.Seen)
	}
	return rows, encodeCursor(next), nil
}
