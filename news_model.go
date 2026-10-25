package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The news model (version 1): one struct per job that every news rendering (json, md, html, text) is drawn
// from, so a number is computed once. See docs/specs/news-model.md and docs/specs/metrics.md.

const (
	newsCapNow       = 50   // each now list
	newsCapMilesOpen = 200  // now.milestones.open
	newsCapHappened  = 100  // each happened list
	newsCapReady     = 10   // next.ready
	newsCapNextMiles = 5    // next.next_milestones
	newsCapTenants   = 2000 // now.tenants
	newsCapBacklog   = 50   // now.backlog.items
	newsCapRecent    = 10   // now.recent_done
	newsCapSprint    = 50   // now.sprint.items
	newsItemText     = 200  // characters of any item text
	newsModelMax     = 1 << 20

	metricsMaxBytes    = 256 << 10
	metricsMaxSections = 8
	metricsMaxRows     = 40
	metricsText        = 60
	metricsNote        = 120
)

type newsItem struct {
	Tenant string `json:"tenant"`
	ID     string `json:"id"`
	Title  string `json:"title"`
	Note   string `json:"note"`
	Since  string `json:"since"`
	By     string `json:"by"`
	// the optional tracker ticket (tracker.go): absent unless the item has one; mw builds the link
	Ticket      string `json:"ticket,omitempty"`
	TicketLabel string `json:"ticket_label,omitempty"`
	TicketURL   string `json:"ticket_url,omitempty"`
	// the milestone (wave) the item belongs to, set on done items; state only on backlog items
	Milestone      string `json:"milestone,omitempty"`
	MilestoneTitle string `json:"milestone_title,omitempty"`
	State          string `json:"state,omitempty"`
	// member progress of a closed milestone (happened.closed_milestones only)
	Done  int `json:"done,omitempty"`
	Total int `json:"total,omitempty"`
	// evidence files of a done or review item (absolute slash path; image marks png/jpg/jpeg/gif/webp), at most 4
	Proof []newsProof `json:"proof,omitempty"`
}

// newsSprintItem is a member of the sprint with its state (todo, doing, review, blocked, done); needs marks an
// open ask.
type newsSprintItem struct {
	newsItem
	State string `json:"state"`
	Needs bool   `json:"needs,omitempty"`
}

// newsSprint is the open wave (mw's work batch), with its members, for the agile and kanban views.
type newsSprint struct {
	Kind   string           `json:"kind"`
	Tenant string           `json:"tenant"`
	ID     string           `json:"id"`
	Title  string           `json:"title"`
	Done   int              `json:"done"`
	Total  int              `json:"total"`
	Items  []newsSprintItem `json:"items"`
	More   int              `json:"more"`
}

type newsBacklog struct {
	Items []newsItem `json:"items"`
	Count int        `json:"count"`
}

type newsMilestone struct {
	Tenant string `json:"tenant"`
	ID     string `json:"id"`
	Title  string `json:"title"`
	Done   int    `json:"done"`
	Total  int    `json:"total"`
}

type newsJobInfo struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

// newsRelease: number null marks a draft. at is the release time (a draft: its newest event), since the
// previous release's at (empty for release 1 and for a draft with no release yet), previous the number of
// the release compared against, changes the events (plus changed metric rows) in the window.
type newsRelease struct {
	Number   *int   `json:"number"`
	At       string `json:"at"`
	Since    string `json:"since"`
	Previous *int   `json:"previous"`
	Changes  int    `json:"changes"`
}

type newsCounts struct {
	Draft   int `json:"draft"`
	Needs   int `json:"needs"`
	Review  int `json:"review"`
	Blocked int `json:"blocked"`
	Doing   int `json:"doing"`
	Ready   int `json:"ready"`
	Done    int `json:"done"`
	Total   int `json:"total"`
}

type newsMilestones struct {
	Open        []newsMilestone `json:"open"`
	ClosedCount int             `json:"closed_count"`
}

// newsTenant is one row of now.tenants: small on purpose (a job can have thousands of tenants).
type newsTenant struct {
	Slug       string     `json:"slug"`
	Name       string     `json:"name"`
	Counts     newsCounts `json:"counts"`
	LastChange string     `json:"last_change"`
}

type newsNow struct {
	Counts     newsCounts     `json:"counts"`
	Tenants    []newsTenant   `json:"tenants"`
	Backlog    newsBacklog    `json:"backlog"`
	RecentDone []newsItem     `json:"recent_done"`
	Sprint     *newsSprint    `json:"sprint"`
	Needs      []newsItem     `json:"needs"`
	Review     []newsItem     `json:"review"`
	Blocked    []newsItem     `json:"blocked"`
	Doing      []newsItem     `json:"doing"`
	Milestones newsMilestones `json:"milestones"`
	Caps       map[string]int `json:"caps"`
	More       map[string]int `json:"more"`
}

type newsHappened struct {
	ClosedMilestones []newsItem     `json:"closed_milestones"`
	Done             []newsItem     `json:"done"`
	Accepted         []newsItem     `json:"accepted"`
	SentBack         []newsItem     `json:"sent_back"`
	Answers          []newsItem     `json:"answers"`
	Decisions        []newsItem     `json:"decisions"`
	NewItems         []newsItem     `json:"new_items"`
	MovedToReview    []newsItem     `json:"moved_to_review"`
	MovedToDoing     []newsItem     `json:"moved_to_doing"`
	Counts           map[string]int `json:"counts"`
	Caps             map[string]int `json:"caps"`
	More             map[string]int `json:"more"`
}

type newsNext struct {
	Ready          []newsItem      `json:"ready"`
	NextMilestones []newsMilestone `json:"next_milestones"`
	Caps           map[string]int  `json:"caps"`
	More           map[string]int  `json:"more"`
}

type newsModel struct {
	Version   int          `json:"version"`
	Job       newsJobInfo  `json:"job"`
	Release   newsRelease  `json:"release"`
	Now       newsNow      `json:"now"`
	Happened  newsHappened `json:"happened"`
	Next      newsNext     `json:"next"`
	Metrics   *newsMetrics `json:"metrics"`
	Summary   string       `json:"summary"`
	Truncated bool         `json:"truncated,omitempty"` // set when the 1 MB guard had to drop rows
	// Full is set by --full: no list is capped. TrimmedRows (only with Full) counts the rows the 1 MB guard still dropped.
	Full        bool `json:"full,omitempty"`
	TrimmedRows int  `json:"trimmed_rows,omitempty"`
}

// ---- metrics ----

type newsLegend struct {
	Good string `json:"good"`
	Warn string `json:"warn"`
	Bad  string `json:"bad"`
	None string `json:"none"`
}

type newsMRow struct {
	Name   string             `json:"name"`
	Good   int                `json:"good,omitempty"`
	Warn   int                `json:"warn,omitempty"`
	Bad    int                `json:"bad,omitempty"`
	None   int                `json:"none,omitempty"`
	Done   int                `json:"done,omitempty"`
	Total  int                `json:"total,omitempty"`
	Value  *float64           `json:"value,omitempty"`
	Unit   string             `json:"unit,omitempty"`
	Target *float64           `json:"target,omitempty"`
	Better string             `json:"better,omitempty"`
	Delta  map[string]float64 `json:"delta,omitempty"`
}

type newsMSection struct {
	Title  string      `json:"title"`
	Kind   string      `json:"kind"`
	Legend *newsLegend `json:"legend,omitempty"`
	Note   string      `json:"note,omitempty"`
	Rows   []newsMRow  `json:"rows"`
}

type newsMetrics struct {
	Updated  string         `json:"updated,omitempty"`
	Sections []newsMSection `json:"sections,omitempty"`
	Notice   string         `json:"notice,omitempty"`
	Error    string         `json:"error,omitempty"`
}

type rawMRow struct {
	Name   *string  `json:"name"`
	Good   *float64 `json:"good"`
	Warn   *float64 `json:"warn"`
	Bad    *float64 `json:"bad"`
	None   *float64 `json:"none"`
	Done   *float64 `json:"done"`
	Total  *float64 `json:"total"`
	Value  *float64 `json:"value"`
	Unit   *string  `json:"unit"`
	Target *float64 `json:"target"`
	Better *string  `json:"better"`
}

type rawMSection struct {
	Title  *string `json:"title"`
	Kind   *string `json:"kind"`
	Legend *struct {
		Good *string `json:"good"`
		Warn *string `json:"warn"`
		Bad  *string `json:"bad"`
		None *string `json:"none"`
	} `json:"legend"`
	Note *string   `json:"note"`
	Rows []rawMRow `json:"rows"`
}

type rawMetrics struct {
	Version  *float64      `json:"version"`
	Updated  *string       `json:"updated"`
	Sections []rawMSection `json:"sections"`
}

// metricsText strips control characters and enforces a length.
func metricsStr(what string, s *string, max int, required bool) (string, error) {
	if s == nil {
		if required {
			return "", fmt.Errorf("%s is missing", what)
		}
		return "", nil
	}
	c := strings.TrimSpace(newsClean(*s, false))
	if required && c == "" {
		return "", fmt.Errorf("%s is empty", what)
	}
	if n := len([]rune(c)); n > max {
		return "", fmt.Errorf("%s is %d characters (at most %d)", what, n, max)
	}
	return c, nil
}

func metricsCount(what string, f *float64) (int, error) {
	if f == nil {
		return 0, nil
	}
	v := *f
	if math.IsNaN(v) || math.IsInf(v, 0) || v != math.Trunc(v) || v < 0 || v > 1e12 {
		return 0, fmt.Errorf("%s must be an integer >= 0", what)
	}
	return int(v), nil
}

func metricsNum(what string, f *float64) (*float64, error) {
	if f == nil {
		return nil, nil
	}
	if math.IsNaN(*f) || math.IsInf(*f, 0) || math.Abs(*f) > 1e12 {
		return nil, fmt.Errorf("%s must be a finite number with an absolute value of at most 1e12", what)
	}
	v := *f
	return &v, nil
}

// parseMetrics validates the file and returns the model block: sections on success, an error block on an
// invalid file. A higher version still parses what it can and adds a notice; if it cannot be read, only the
// notice remains.
func parseMetrics(b []byte) *newsMetrics {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")) // a UTF-8 byte order mark
	var raw rawMetrics
	dec := json.NewDecoder(strings.NewReader(string(b)))
	err := dec.Decode(&raw)
	if err == nil && dec.More() {
		err = errors.New("unexpected data after the JSON document")
	}
	if err != nil {
		var probe struct {
			Version float64 `json:"version"`
		}
		if json.Unmarshal(b, &probe) == nil && probe.Version > 1 {
			return &newsMetrics{Notice: newerMetrics(probe.Version)}
		}
		return &newsMetrics{Error: compactText(err.Error(), 120)}
	}
	if raw.Version == nil || *raw.Version != math.Trunc(*raw.Version) || *raw.Version < 1 {
		return &newsMetrics{Error: "version must be an integer >= 1"}
	}
	newer := *raw.Version > 1
	out := &newsMetrics{}
	fail := func(err error) *newsMetrics {
		if newer {
			return &newsMetrics{Notice: newerMetrics(*raw.Version)}
		}
		return &newsMetrics{Error: err.Error()}
	}
	if newer {
		out.Notice = newerMetrics(*raw.Version)
	}
	if raw.Updated != nil {
		u, err := metricsStr("updated", raw.Updated, 40, false)
		if err != nil {
			return fail(err)
		}
		out.Updated = u
	}
	if len(raw.Sections) > metricsMaxSections {
		return fail(fmt.Errorf("%d sections (at most %d)", len(raw.Sections), metricsMaxSections))
	}
	for si, rs := range raw.Sections {
		sec := newsMSection{Rows: []newsMRow{}}
		at := fmt.Sprintf("section %d", si+1)
		var err error
		if sec.Title, err = metricsStr(at+" title", rs.Title, metricsText, true); err != nil {
			return fail(err)
		}
		if rs.Kind == nil {
			return fail(fmt.Errorf("%s kind is missing", at))
		}
		sec.Kind = *rs.Kind
		if sec.Kind != "split" && sec.Kind != "progress" && sec.Kind != "value" {
			return fail(fmt.Errorf("%s kind %q is not split, progress or value", at, compactText(sec.Kind, 20)))
		}
		if sec.Note, err = metricsStr(at+" note", rs.Note, metricsNote, false); err != nil {
			return fail(err)
		}
		if sec.Kind == "split" {
			lg := newsLegend{Good: "pass", Warn: "partial", Bad: "fail", None: "skip"}
			if rs.Legend != nil {
				for _, p := range []struct {
					dst *string
					src *string
					n   string
				}{{&lg.Good, rs.Legend.Good, "good"}, {&lg.Warn, rs.Legend.Warn, "warn"}, {&lg.Bad, rs.Legend.Bad, "bad"}, {&lg.None, rs.Legend.None, "none"}} {
					if p.src == nil {
						continue
					}
					v, err := metricsStr(at+" legend "+p.n, p.src, metricsText, false)
					if err != nil {
						return fail(err)
					}
					if v != "" {
						*p.dst = v
					}
				}
			}
			sec.Legend = &lg
		}
		if len(rs.Rows) > metricsMaxRows {
			return fail(fmt.Errorf("%s has %d rows (at most %d)", at, len(rs.Rows), metricsMaxRows))
		}
		names := map[string]bool{}
		for ri, rr := range rs.Rows {
			rat := fmt.Sprintf("%s row %d", at, ri+1)
			row := newsMRow{}
			if row.Name, err = metricsStr(rat+" name", rr.Name, metricsText, true); err != nil {
				return fail(err)
			}
			if names[row.Name] {
				return fail(fmt.Errorf("%s: duplicate row name %q", at, row.Name))
			}
			names[row.Name] = true
			switch sec.Kind {
			case "split":
				for _, p := range []struct {
					dst *int
					src *float64
					n   string
				}{{&row.Good, rr.Good, "good"}, {&row.Warn, rr.Warn, "warn"}, {&row.Bad, rr.Bad, "bad"}, {&row.None, rr.None, "none"}} {
					if *p.dst, err = metricsCount(rat+" "+p.n, p.src); err != nil {
						return fail(err)
					}
				}
			case "progress":
				if row.Done, err = metricsCount(rat+" done", rr.Done); err != nil {
					return fail(err)
				}
				if row.Total, err = metricsCount(rat+" total", rr.Total); err != nil {
					return fail(err)
				}
				if row.Done > row.Total {
					return fail(fmt.Errorf("%s: done %d is more than total %d", rat, row.Done, row.Total))
				}
			case "value":
				if rr.Value == nil {
					return fail(fmt.Errorf("%s value is missing", rat))
				}
				if row.Value, err = metricsNum(rat+" value", rr.Value); err != nil {
					return fail(err)
				}
				if row.Target, err = metricsNum(rat+" target", rr.Target); err != nil {
					return fail(err)
				}
				if row.Unit, err = metricsStr(rat+" unit", rr.Unit, metricsText, false); err != nil {
					return fail(err)
				}
				row.Better = "higher"
				if rr.Better != nil {
					if *rr.Better != "higher" && *rr.Better != "lower" {
						return fail(fmt.Errorf("%s better %q is not higher or lower", rat, compactText(*rr.Better, 20)))
					}
					row.Better = *rr.Better
				}
			}
			sec.Rows = append(sec.Rows, row)
		}
		out.Sections = append(out.Sections, sec)
	}
	return out
}

func newerMetrics(v float64) string {
	return fmt.Sprintf("newer metrics format (version %s): update mw to read it", strconv.FormatFloat(v, 'f', -1, 64))
}

// newsMetricsLoad reads the job's metrics file as data: confined, a regular file, at most 256 KB. A missing
// file is no metrics (nil); an unreadable or oversized one is an error block.
func newsMetricsLoad(root, rel string) *newsMetrics {
	f, err := openConfined(root, "metrics", rel)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return &newsMetrics{Error: compactText(err.Error(), 120)}
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, metricsMaxBytes+1))
	if err != nil {
		return &newsMetrics{Error: compactText(err.Error(), 120)}
	}
	if len(b) > metricsMaxBytes {
		return &newsMetrics{Error: "file is larger than 256 KB"}
	}
	return parseMetrics(b)
}

// metricsPrimary is the field a row's ▲/▼ is drawn from.
func metricsPrimary(kind string) string {
	switch kind {
	case "split":
		return "good"
	case "progress":
		return "done"
	}
	return "value"
}

func metricsFields(kind string, r newsMRow) map[string]float64 {
	switch kind {
	case "split":
		return map[string]float64{"good": float64(r.Good), "warn": float64(r.Warn), "bad": float64(r.Bad), "none": float64(r.None)}
	case "progress":
		return map[string]float64{"done": float64(r.Done), "total": float64(r.Total)}
	}
	v := 0.0
	if r.Value != nil {
		v = *r.Value
	}
	return map[string]float64{"value": v}
}

// metricsDeltas fills each row's delta against the previous release's frozen metrics; it returns how many
// rows changed.
func metricsDeltas(cur, prev *newsMetrics) int {
	if cur == nil || prev == nil || cur.Error != "" || prev.Error != "" {
		return 0
	}
	old := map[string]newsMRow{}
	for _, s := range prev.Sections {
		for _, r := range s.Rows {
			old[s.Kind+"\x00"+s.Title+"\x00"+r.Name] = r
		}
	}
	changed := 0
	for si := range cur.Sections {
		s := &cur.Sections[si]
		for ri := range s.Rows {
			o, ok := old[s.Kind+"\x00"+s.Title+"\x00"+s.Rows[ri].Name]
			if !ok {
				continue
			}
			now, was := metricsFields(s.Kind, s.Rows[ri]), metricsFields(s.Kind, o)
			d := map[string]float64{}
			for k, v := range now {
				diff := v - was[k]
				if r, err := strconv.ParseFloat(strconv.FormatFloat(diff, 'g', 12, 64), 64); err == nil {
					diff = r // 99.2-98.9 reads 0.3, not 0.30000000000000426
				}
				if diff != 0 && !math.IsInf(diff, 0) && !math.IsNaN(diff) {
					d[k] = diff
				}
			}
			if len(d) > 0 {
				s.Rows[ri].Delta = d
				changed++
			}
		}
	}
	return changed
}

// ---- model building ----

// awaitingReviewNote reports a blocked item whose latest block event's reason starts with "awaiting review";
// mw review and mw news use this one rule.
func awaitingReviewNote(it Item) bool {
	e, ok := lastEvent(it, "block")
	return ok && strings.HasPrefix(strings.ToLower(strings.TrimSpace(e.Note)), "awaiting review")
}

// itemStamp is the time of an item's last logged event (Updated only when it has none), so the model stays a
// function of the logged history.
func itemStamp(it Item) string {
	if n := len(it.Events); n > 0 {
		return it.Events[n-1].At
	}
	return it.Updated
}

func newsCap(s string) string { return compactText(s, newsItemText) }

func lastEvent(it Item, actions ...string) (Event, bool) {
	var out Event
	ok := false
	for _, e := range it.Events {
		for _, a := range actions {
			if e.Action == a {
				out, ok = e, true
			}
		}
	}
	return out, ok
}

func newsSortItems(l []newsItem, newestFirst bool) {
	sort.SliceStable(l, func(i, j int) bool {
		a, b := l[i], l[j]
		if a.Since != b.Since {
			if newestFirst {
				return parseTS(a.Since).After(parseTS(b.Since)) || parseTS(a.Since).Equal(parseTS(b.Since)) && a.Since > b.Since
			}
			return parseTS(a.Since).Before(parseTS(b.Since)) || parseTS(a.Since).Equal(parseTS(b.Since)) && a.Since < b.Since
		}
		if a.Tenant != b.Tenant {
			return a.Tenant < b.Tenant
		}
		return a.ID < b.ID
	})
}

func capItems(l []newsItem, max int) ([]newsItem, int) {
	if l == nil {
		l = []newsItem{}
	}
	if len(l) > max {
		return l[:max], len(l) - max
	}
	return l, 0
}

// newsWindow carries what a model is keyed on: the release number (nil = draft), the previous release,
// and the window (since, at].
type newsWindow struct {
	number   *int
	at       string // release time; "" for a draft (its newest event is used)
	prev     *newsModel
	prevNum  *int
	releases []int
	full     bool // --full: no per-list caps (the 200-character item text and the 1 MB guard stay)
}

// buildNewsModel computes the model for one job. Everything in it is a function of the board, the metrics
// and summary files, and the window.
func buildNewsModel(w *Workspace, p newsPart, metrics *newsMetrics, win newsWindow) *newsModel {
	m := &newsModel{Version: 1, Full: win.full}
	// capOf is the row cap of a list; with --full there is none (the reported caps are 0 = uncapped)
	capOf := func(n int) int {
		if win.full {
			return math.MaxInt
		}
		return n
	}
	capNow, capMilesOpen, capHappened, capReady := capOf(newsCapNow), capOf(newsCapMilesOpen), capOf(newsCapHappened), capOf(newsCapReady)
	capNextMiles, capTenants, capBacklog, capRecent, capSprint := capOf(newsCapNextMiles), capOf(newsCapTenants), capOf(newsCapBacklog), capOf(newsCapRecent), capOf(newsCapSprint)
	shown := func(n int) int { // the cap as reported in "caps"
		if win.full {
			return 0
		}
		return n
	}
	m.Job = newsJobInfo{ID: p.job, Title: newsCap(p.title), Description: newsCap(p.desc), URL: newsClean(p.url, false)}
	if m.Job.Title == "" {
		m.Job.Title = p.job
	}
	var since time.Time
	if win.prev != nil {
		m.Release.Since = win.prev.Release.At
		since = parseTS(m.Release.Since)
		m.Release.Previous = win.prevNum
	}
	// draft: the window ends at the newest event of the job, so the same board gives the same model
	var newest string
	var newestT time.Time
	scope := p.all // the window is job-wide even when the model is filtered to one tenant
	if scope == nil {
		scope = p.tenants
	}
	for _, t := range scope {
		for _, r := range tenantRows(t) {
			if newest == "" || r.sortKey.After(newestT) {
				newest, newestT = r.At, r.sortKey
			}
		}
	}
	atStr := win.at
	if atStr == "" {
		atStr = newest
		if newestT.Before(since) {
			atStr = m.Release.Since
		}
	}
	at := parseTS(atStr)
	m.Release.Number, m.Release.At = win.number, atStr
	inWin := func(t time.Time) bool {
		return !t.IsZero() && t.After(since) && !t.After(at)
	}

	cfg := w.Jobs[p.job]
	tick := func(x *newsItem, it Item) {
		x.Ticket, x.TicketLabel, x.TicketURL = ticketShown(cfg, it.Ticket)
	}
	var needs, review, blocked, doing, ready []newsItem
	var open, openAll []newsMilestone
	var closedMs, done, accepted, sentBack, answers, decisions, newItems []newsItem
	var drafts, recent, movedReview, movedDoing []newsItem
	type openWave struct {
		t       *Tenant
		judgeOn bool
	}
	waves := map[string]openWave{}
	waveTitle := map[string]string{}
	waveProgress := map[string][2]int{}
	events := 0
	c := &m.Now.Counts
	var trows []newsTenant
	for _, t := range p.tenants {
		tr := newsTenant{Slug: compactText(t.Slug, 40), Name: compactText(t.Name, 40)}
		before := *c
		cNeeds, cReview, cBlocked, cDoing, cReady := len(needs), len(review), len(blocked), len(doing), len(ready)
		gs := gateStates(w, t)
		judgeOn := byName(gs, "judge").Effective
		title := map[string]string{}
		kind := map[string]string{}
		for _, it := range t.Items {
			title[it.ID], kind[it.ID] = it.Title, it.Kind
		}
		for _, it := range t.Items {
			base := newsItem{Tenant: t.Slug, ID: it.ID, Title: newsCap(it.Title)}
			tick(&base, it)
			if it.Kind == "wave" {
				dn := 0
				mem := waveMembers(t, it.ID)
				for _, x := range mem {
					if x.Status == "done" {
						dn++
					}
				}
				waveTitle[t.Slug+"/"+it.ID] = newsCap(it.Title)
				waveProgress[t.Slug+"/"+it.ID] = [2]int{dn, len(mem)}
				if it.Status == "done" {
					m.Now.Milestones.ClosedCount++
				} else {
					open = append(open, newsMilestone{t.Slug, it.ID, newsCap(it.Title), dn, len(mem)})
					waves[t.Slug+"/"+it.ID] = openWave{t, judgeOn}
				}
				continue
			}
			c.Total++
			if it.Status == "done" {
				c.Done++
				x := base
				x.Since = it.Updated
				if n := len(it.Events); n > 0 { // event stamps only, so the model stays a function of the logged history
					x.Since = it.Events[n-1].At
				}
				if e, ok := lastEvent(it, "done"); ok {
					x.Since, x.By = e.At, displayName(e.By)
				}
				x.Milestone = it.Wave
				recent = append(recent, x)
			}
			if it.Status == "inbox" {
				x := base
				x.Since, x.State = it.Created, "draft"
				if e, ok := lastEvent(it, "created"); ok {
					x.Since = e.At
				}
				drafts = append(drafts, x)
				c.Draft++
			}
			if e, ok := openAsk(it); ok {
				x := base
				x.Note, x.Since, x.By = newsCap(e.Note), e.At, displayName(e.By)
				needs = append(needs, x)
			}
			if judgeOn && awaitingJudgement(t, it) {
				x := base
				if e, ok := lastEvent(it, "done"); ok {
					x.Since, x.By = e.At, displayName(e.By)
				}
				if it.Attempts > 0 {
					x.Note = fmt.Sprintf("attempt %d", it.Attempts+1)
				}
				x.Proof = proofFor(w, t, it)
				review = append(review, x)
			}
			switch it.Status {
			case "blocked":
				x := base
				if e, ok := lastEvent(it, "block"); ok {
					x.Note, x.Since, x.By = newsCap(e.Note), e.At, displayName(e.By)
				}
				if awaitingReviewNote(it) { // a block that waits for a reviewer is a review, not a block
					x.Proof = proofFor(w, t, it)
					review = append(review, x)
					break
				}
				blocked = append(blocked, x)
			case "doing":
				x := base
				if e, ok := lastEvent(it, "start"); ok {
					x.Since = e.At
				}
				x.By = displayName(it.Assignee)
				if it.Assignee == "" {
					x.By = ""
				}
				doing = append(doing, x)
			case "ready":
				if it.Kind != "prompt" && it.Kind != "story" {
					x := base
					if e, ok := lastEvent(it, "ready"); ok {
						x.Since, x.By = e.At, displayName(e.By)
					}
					ready = append(ready, x)
				}
			}
		}
		c.Total += t.Archived
		c.Done += t.Archived
		lastDone := map[string]int{}
		var tdone []newsItem
		for _, it := range t.Items {
			for _, e := range it.Events {
				et := parseTS(e.At)
				if !inWin(et) {
					continue
				}
				events++
				x := newsItem{Tenant: t.Slug, ID: it.ID, Title: newsCap(it.Title), Note: newsCap(e.Note), Since: e.At, By: displayName(e.By)}
				if e.By == "" {
					x.By = ""
				}
				tick(&x, it)
				if e.Action == "done" && it.Kind != "wave" {
					x.Proof = proofFor(w, t, it)
					x.Milestone = it.Wave
					x.MilestoneTitle = waveTitle[t.Slug+"/"+it.Wave]
				}
				if it.Kind != "wave" {
					switch {
					case e.Action == "start":
						movedDoing = append(movedDoing, x)
					case e.Action == "done" && judgeOn && judgeable(t, it), e.Action == "block" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(e.Note)), "awaiting review"):
						movedReview = append(movedReview, x)
					}
				}
				switch {
				case e.Action == "done" && it.Kind == "wave":
					p := waveProgress[t.Slug+"/"+it.ID]
					x.Done, x.Total = p[0], p[1]
					closedMs = append(closedMs, x)
				case e.Action == "done":
					if i, ok := lastDone[it.ID]; ok { // done again after a send-back: the later stamp
						if tdone[i].Since < x.Since {
							tdone[i] = x
						}
						continue
					}
					lastDone[it.ID] = len(tdone)
					tdone = append(tdone, x)
				case e.Action == "accept":
					accepted = append(accepted, x)
				case e.Action == "reject":
					sentBack = append(sentBack, x)
				case e.Action == "answer":
					answers = append(answers, x)
				case e.Action == "decide":
					decisions = append(decisions, x)
				case e.Action == "created" && it.Kind != "wave":
					newItems = append(newItems, x)
				}
			}
		}
		done = append(done, tdone...)
		tr.Counts = newsCounts{Needs: len(needs) - cNeeds, Review: len(review) - cReview, Blocked: len(blocked) - cBlocked, Doing: len(doing) - cDoing, Ready: len(ready) - cReady, Draft: c.Draft - before.Draft,
			Done: c.Done - before.Done, Total: c.Total - before.Total}
		tr.LastChange, _, _ = lastActivity(t)
		trows = append(trows, tr)
	}
	c.Needs, c.Review, c.Blocked, c.Doing, c.Ready = len(needs), len(review), len(blocked), len(doing), len(ready)

	sort.SliceStable(trows, func(i, j int) bool {
		a, b := trows[i], trows[j]
		if a.Counts.Needs != b.Counts.Needs {
			return a.Counts.Needs > b.Counts.Needs
		}
		if a.Counts.Review != b.Counts.Review {
			return a.Counts.Review > b.Counts.Review
		}
		if a.Counts.Blocked != b.Counts.Blocked {
			return a.Counts.Blocked > b.Counts.Blocked
		}
		if ta, tb := parseTS(a.LastChange), parseTS(b.LastChange); !ta.Equal(tb) {
			return ta.After(tb)
		}
		return a.Slug < b.Slug
	})
	m.Now.Tenants = []newsTenant{}
	tmore := 0
	if len(trows) > capTenants {
		tmore = len(trows) - capTenants
		trows = trows[:capTenants]
	}
	m.Now.Tenants = append(m.Now.Tenants, trows...)
	// backlog: the ready queue in order, then the drafts by creation
	newsSortItems(drafts, false)
	backlog := make([]newsItem, 0, len(ready)+len(drafts))
	for _, r := range ready {
		r.State = "ready"
		backlog = append(backlog, r)
	}
	backlog = append(backlog, drafts...)
	m.Now.Backlog.Count = len(backlog)
	backlogMore := 0
	if len(backlog) > capBacklog {
		backlogMore = len(backlog) - capBacklog
		backlog = backlog[:capBacklog]
	}
	m.Now.Backlog.Items = append([]newsItem{}, backlog...)
	newsSortItems(recent, true)
	if len(recent) > capRecent {
		recent = recent[:capRecent]
	}
	for i := range recent {
		recent[i].MilestoneTitle = waveTitle[recent[i].Tenant+"/"+recent[i].Milestone]
	}
	m.Now.RecentDone = append([]newsItem{}, recent...)
	m.Now.Caps = map[string]int{"backlog": shown(newsCapBacklog), "recent_done": shown(newsCapRecent), "sprint": shown(newsCapSprint), "tenants": shown(newsCapTenants), "needs": shown(newsCapNow), "review": shown(newsCapNow), "blocked": shown(newsCapNow), "doing": shown(newsCapNow), "milestones": shown(newsCapMilesOpen)}
	m.Now.More = map[string]int{"tenants": tmore, "backlog": backlogMore}
	for _, l := range []struct {
		name string
		src  []newsItem
		dst  *[]newsItem
	}{{"needs", needs, &m.Now.Needs}, {"review", review, &m.Now.Review}, {"blocked", blocked, &m.Now.Blocked}, {"doing", doing, &m.Now.Doing}} {
		newsSortItems(l.src, false)
		*l.dst, m.Now.More[l.name] = capItems(l.src, capNow)
	}
	sort.SliceStable(open, func(i, j int) bool {
		a, b := open[i], open[j]
		// nearest to done first: the larger done fraction, then fewer left, then a stable name order
		if x, y := a.Done*max(b.Total, 1), b.Done*max(a.Total, 1); (a.Total > 0 || b.Total > 0) && x != y {
			return x > y
		}
		if ra, rb := a.Total-a.Done, b.Total-b.Done; ra != rb {
			return ra < rb
		}
		if a.Tenant != b.Tenant {
			return a.Tenant < b.Tenant
		}
		return a.ID < b.ID
	})
	var sprintPick *newsMilestone
	if len(open) > 0 {
		first := open[0]
		sprintPick = &first
	}
	openAll = open
	m.Now.Milestones.Open = []newsMilestone{}
	if len(open) > capMilesOpen {
		m.Now.More["milestones"] = len(open) - capMilesOpen
		open = open[:capMilesOpen]
	} else {
		m.Now.More["milestones"] = 0
	}
	m.Now.Milestones.Open = append(m.Now.Milestones.Open, open...)

	m.Happened.Counts = map[string]int{}
	m.Happened.Caps = map[string]int{}
	m.Happened.More = map[string]int{}
	for _, l := range []struct {
		name string
		src  []newsItem
		dst  *[]newsItem
	}{{"closed_milestones", closedMs, &m.Happened.ClosedMilestones}, {"done", done, &m.Happened.Done}, {"accepted", accepted, &m.Happened.Accepted},
		{"sent_back", sentBack, &m.Happened.SentBack}, {"answers", answers, &m.Happened.Answers}, {"decisions", decisions, &m.Happened.Decisions},
		{"new_items", newItems, &m.Happened.NewItems}, {"moved_to_review", movedReview, &m.Happened.MovedToReview}, {"moved_to_doing", movedDoing, &m.Happened.MovedToDoing}} {
		newsSortItems(l.src, true)
		m.Happened.Counts[l.name] = len(l.src)
		m.Happened.Caps[l.name] = shown(newsCapHappened)
		*l.dst, m.Happened.More[l.name] = capItems(l.src, capHappened)
	}

	m.Next.Caps = map[string]int{"ready": shown(newsCapReady), "next_milestones": shown(newsCapNextMiles)}
	m.Next.More = map[string]int{}
	m.Next.Ready, m.Next.More["ready"] = capItems(ready, capReady) // queue order: tenant, then item order
	m.Next.NextMilestones = []newsMilestone{}
	if len(openAll) > capNextMiles {
		m.Next.More["next_milestones"] = len(openAll) - capNextMiles
		openAll = openAll[:capNextMiles]
	} else {
		m.Next.More["next_milestones"] = 0
	}
	m.Next.NextMilestones = append(m.Next.NextMilestones, openAll...)

	if sprintPick != nil { // the open wave nearest to done (mw's work batch is the wave; there is no other milestone kind)
		ow := waves[sprintPick.Tenant+"/"+sprintPick.ID]
		sp := &newsSprint{Kind: "wave", Tenant: sprintPick.Tenant, ID: sprintPick.ID, Title: sprintPick.Title, Done: sprintPick.Done, Total: sprintPick.Total, Items: []newsSprintItem{}}
		rank := map[string]int{"doing": 0, "review": 1, "blocked": 2, "todo": 3, "done": 4}
		var mem []newsSprintItem
		for _, it := range waveMembers(ow.t, sprintPick.ID) {
			if it.Kind == "wave" {
				continue
			}
			x := newsSprintItem{newsItem: newsItem{Tenant: ow.t.Slug, ID: it.ID, Title: newsCap(it.Title), Since: itemStamp(it)}}
			tick(&x.newsItem, it)
			_, x.Needs = openAsk(it)
			switch {
			case it.Status == "done" && ow.judgeOn && awaitingJudgement(ow.t, it):
				x.State = "review"
			case it.Status == "done":
				x.State = "done"
			case it.Status == "blocked" && awaitingReviewNote(it):
				x.State = "review"
			case it.Status == "blocked":
				x.State = "blocked"
			case it.Status == "doing":
				x.State = "doing"
			default:
				x.State = "todo"
			}
			mem = append(mem, x)
		}
		sort.SliceStable(mem, func(i, j int) bool {
			if rank[mem[i].State] != rank[mem[j].State] {
				return rank[mem[i].State] < rank[mem[j].State]
			}
			return mem[i].ID < mem[j].ID
		})
		if len(mem) > capSprint {
			sp.More = len(mem) - capSprint
			mem = mem[:capSprint]
		}
		sp.Items = append(sp.Items, mem...)
		m.Now.Sprint = sp
	}
	m.Metrics = metrics
	var prevM *newsMetrics
	if win.prev != nil {
		prevM = win.prev.Metrics
	}
	changed := metricsDeltas(m.Metrics, prevM)
	// changes are what the newsletter reports: the happened lists plus changed metric rows (edits, notes, steps,
	// ready/start events and the like never count)
	_ = events
	for k, n := range m.Happened.Counts {
		if k != "moved_to_review" && k != "moved_to_doing" { // flow views only; they never make a release
			m.Release.Changes += n
		}
	}
	m.Release.Changes += changed
	m.Summary = newsClean(p.summary, true)
	newsGuardSize(m)
	return m
}

// newsGuardSize keeps the encoded model (as it is written, indented) under 1 MB: if it is over, rows are
// dropped from the longest lists (counted in their more) until it fits; tenant rows go last.
func newsGuardSize(m *newsModel) {
	trimmed := 0 // rows dropped; reported as trimmed_rows under --full
	defer func() {
		if m.Full {
			m.TrimmedRows = trimmed
		}
	}()
	for round := 0; round < 64; round++ {
		b, err := json.MarshalIndent(m, "", "  ")
		if err == nil && len(b) <= newsModelMax {
			return
		}
		if err != nil {
			return // reported by the caller's own encode
		}
		m.Truncated = true
		type lst struct {
			l    *[]newsItem
			more map[string]int
			key  string
		}
		all := []lst{{&m.Now.Needs, m.Now.More, "needs"}, {&m.Now.Review, m.Now.More, "review"}, {&m.Now.Blocked, m.Now.More, "blocked"}, {&m.Now.Doing, m.Now.More, "doing"},
			{&m.Happened.ClosedMilestones, m.Happened.More, "closed_milestones"}, {&m.Happened.Done, m.Happened.More, "done"}, {&m.Happened.Accepted, m.Happened.More, "accepted"},
			{&m.Happened.SentBack, m.Happened.More, "sent_back"}, {&m.Happened.Answers, m.Happened.More, "answers"}, {&m.Happened.Decisions, m.Happened.More, "decisions"},
			{&m.Happened.NewItems, m.Happened.More, "new_items"}, {&m.Happened.MovedToReview, m.Happened.More, "moved_to_review"}, {&m.Happened.MovedToDoing, m.Happened.More, "moved_to_doing"},
			{&m.Now.Backlog.Items, m.Now.More, "backlog"}}
		reduced := false
		for _, x := range all {
			if n := len(*x.l); n > 1 {
				k := n / 2
				*x.l = (*x.l)[:n-k]
				x.more[x.key] += k
				trimmed += k
				reduced = true
			}
		}
		if n := len(m.Now.Milestones.Open); n > 1 {
			m.Now.Milestones.Open = m.Now.Milestones.Open[:n-n/2]
			m.Now.More["milestones"] += n / 2
			trimmed += n / 2
			reduced = true
		}
		if len(m.Summary) > 4096 {
			m.Summary = strings.ToValidUTF8(m.Summary[:4096], "")
			reduced = true
		}
		if !reduced { // the items are as small as they get: now the tenant rows
			if n := len(m.Now.Tenants); n > 1 {
				k := n / 2
				m.Now.Tenants = m.Now.Tenants[:n-k]
				m.Now.More["tenants"] += k
				trimmed += k
			} else {
				return
			}
		}
	}
}

func newsModelJSON(m *newsModel) ([]byte, error) {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("news model cannot be encoded: %w", err)
	}
	return append(b, '\n'), nil
}

// ---- file reads: regular files only, size capped ----

// readRegular reads a regular file of at most max bytes without ever blocking: a FIFO, a device (or a symlink
// to one) and an oversized file are refused, like the summary and metrics reads.
func readRegular(path string, max int) ([]byte, error) {
	if fi, err := os.Stat(path); err != nil {
		return nil, err
	} else if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	f, err := openNoBlock(path) // a swap to a FIFO after the check does not block
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(max)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > max {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, max)
	}
	return b, nil
}

// ---- releases ----

var releaseFile = regexp.MustCompile(`^([1-9][0-9]{0,17})\.json$`)

// newsReleaseDir resolves the releases folder of a job: news_releases, else <news_out dir>/releases.
func newsReleaseDir(root string, v JobView) (string, error) {
	rel := v.NewsReleases
	if rel == "" {
		rel = strings.TrimSuffix(v.NewsOut, "/") + "/releases"
	}
	return newsConfined(root, "news_releases", rel)
}

// newsReleaseNumbers lists the release numbers in dir, newest first.
func newsReleaseNumbers(dir string) []int {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []int
	for _, e := range ents {
		if m := releaseFile.FindStringSubmatch(e.Name()); m != nil && !e.IsDir() {
			n, _ := strconv.Atoi(m[1])
			out = append(out, n)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}

// only files mw itself writes count: N.md, N.html, N.json and their temp files
var releaseAny = regexp.MustCompile(`^([1-9][0-9]{0,17})\.(?:md|html|json)(?:\.[^/]*\.tmp)?$`)

// newsMaxReleaseNumber is the highest number any N.* file or the latest pointer claims, so a half-written or
// hand-pruned archive never gets a number reused.
func newsMaxReleaseNumber(dir string) int {
	max := 0
	if ents, err := os.ReadDir(dir); err == nil {
		for _, e := range ents {
			if m := releaseAny.FindStringSubmatch(e.Name()); m != nil {
				if n, _ := strconv.Atoi(m[1]); n > max {
					max = n
				}
			}
		}
	}
	if b, err := readRegular(filepath.Join(dir, "latest"), 64); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && n > max {
			max = n
		}
	}
	return max
}

func newsReadRelease(dir string, n int) (*newsModel, error) {
	b, err := readRegular(filepath.Join(dir, strconv.Itoa(n)+".json"), newsModelMax)
	if err != nil {
		return nil, fmt.Errorf("release %d: %w", n, err)
	}
	var m newsModel
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("release %d.json is not a valid news model: %w", n, err)
	}
	return &m, nil
}

// newsPreviousRelease loads the newest release, nil when none exists. A release file that cannot be read is an
// error when strict (cutting a release); otherwise a warning goes to stderr and the newest readable release is
// used (or none), so a corrupt archive never blocks a draft.
func newsPreviousRelease(dir string, strict bool) (*newsModel, *int, []int, error) {
	nums := newsReleaseNumbers(dir)
	for _, n := range nums {
		m, err := newsReadRelease(dir, n)
		if err == nil {
			n := n
			return m, &n, nums, nil
		}
		if strict {
			return nil, nil, nums, fmt.Errorf("%s is unreadable (%v): fix or remove it before cutting a release", filepath.Join(dir, strconv.Itoa(n)+".json"), err)
		}
		fmt.Fprintf(os.Stderr, "mw: warning: %s is unreadable (%v); the draft compares with the newest readable release\n", filepath.Join(dir, strconv.Itoa(n)+".json"), err)
	}
	return nil, nil, nums, nil
}

// writeNewFile writes content durably and never replaces an existing file (a lost race becomes an error). It
// writes a synced temp file and hard-links it into place; where the filesystem has no hard links (FAT, exFAT,
// some network shares) it falls back to creating the file with O_EXCL, then writing and syncing it.
func writeNewFile(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0644); err != nil {
		return err
	}
	lerr := os.Link(tmp, path)
	if lerr == nil {
		return nil
	}
	if os.IsExist(lerr) {
		return fmt.Errorf("%s already exists (another release was cut at the same time)", filepath.Base(path))
	}
	return writeExclusive(path, b)
}

// writeExclusive creates path with O_EXCL (never replacing a file), writes and syncs it, and removes the partial
// file on any failure. It is the fallback where hard links are unavailable.
func writeExclusive(path string, b []byte) error {
	x, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("%s already exists (another release was cut at the same time)", filepath.Base(path))
		}
		return err
	}
	if _, err = x.Write(b); err == nil {
		err = x.Sync()
	}
	if cerr := x.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}
