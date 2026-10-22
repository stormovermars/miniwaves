package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// mw log --since CURSOR: an event feed a poller can resume.
//
// The cursor is opaque (base64url of {"t": time, "seen": [event keys]}). An event's key is
// tenant/item/eventIndex. A read re-scans from t minus a lookback (default 24 h) and drops the keys
// already seen, so an event stamped before t that reaches the board later (a git merge from another
// machine) is returned exactly once. An event that arrives later than the lookback is not returned.
// Use the same lookback on every read of one cursor: a larger one may return older events again.

const defaultLookback = 24 * time.Hour

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

func eventKey(tenant, item string, idx int) string { return fmt.Sprintf("%s/%s/%d", tenant, item, idx) }

type keyedEvent struct {
	row logRow
	ts  time.Time
	idx int
}

// logSince returns the events the cursor has not delivered, oldest first (at most limit when limit > 0),
// and the next cursor.
func logSince(tenants []*Tenant, cursor string, lookback time.Duration, limit int) ([]logRow, string, error) {
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
	byKey := map[string]keyedEvent{}
	var fresh []keyedEvent
	for _, t := range tenants {
		for _, it := range t.Items {
			for i, e := range it.Events {
				k := eventKey(t.Slug, it.ID, i)
				ke := keyedEvent{row: logRow{At: e.At, Tenant: t.Slug, Item: it.ID, Action: e.Action, By: e.By, Note: e.Note, Key: k}, ts: parseTS(e.At), idx: i}
				byKey[k] = ke
				if seen[k] || hasT && ke.ts.Before(curT.Add(-lookback)) {
					continue
				}
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
	delivered := map[string]bool{}
	rows := make([]logRow, 0, len(fresh))
	for _, ke := range fresh {
		rows = append(rows, ke.row)
		delivered[ke.row.Key] = true
		if ke.ts.After(newT) {
			newT = ke.ts
		}
	}
	// the next seen set: every delivered key (old or new) a later scan from newT-lookback would meet again;
	// a key whose item left the board (archive) is dropped, its events are never scanned again
	next := logCursor{}
	if !newT.IsZero() {
		next.T = newT.UTC().Format(time.RFC3339Nano)
		for k := range seen {
			delivered[k] = true
		}
		for k := range delivered {
			if ke, ok := byKey[k]; ok && !ke.ts.Before(newT.Add(-lookback)) {
				next.Seen = append(next.Seen, k)
			}
		}
		sort.Strings(next.Seen)
	}
	return rows, encodeCursor(next), nil
}
