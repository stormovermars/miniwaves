package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

// Schema 8 storage: .miniwaves.json is the index (workspace settings and the tenant
// list); each tenant keeps its items, counters, gate state and sessions in
// .miniwaves/<slug>.json and its archive in .miniwaves/<slug>.archive.json. Reads take no
// lock (every write is tmp + rename + fsync, so a file is always complete). A writer
// locks only what it writes: .miniwaves/<slug>.lock per tenant and .miniwaves/index.lock
// for index changes (tenant list, folders, workspace gates, publish).

const storeDir = ".miniwaves"

func storePath(board, name string) string {
	return filepath.Join(filepath.Dir(board), storeDir, name)
}
func tenantPath(board, slug string) string  { return storePath(board, slug+".json") }
func archivePath(board, slug string) string { return storePath(board, slug+".archive.json") }

type indexTenant struct {
	Slug     string   `json:"slug"`
	Name     string   `json:"name"`
	Path     string   `json:"path,omitempty"`
	Archived bool     `json:"archived,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Job      string   `json:"job,omitempty"`
}

// indexDoc is the schema 9 .miniwaves.json.
type indexDoc struct {
	Version  int                  `json:"version"`
	Name     string               `json:"name"`
	Defaults map[string]string    `json:"folder_defaults,omitempty"`
	Tenants  []indexTenant        `json:"tenants"`
	Protocol []string             `json:"protocol,omitempty"`
	Folders  map[string]string    `json:"folders,omitempty"`
	Gates    map[string]string    `json:"gates,omitempty"`
	GateBy   string               `json:"gate_by,omitempty"`
	Publish  *PublishConfig       `json:"publish,omitempty"`
	Fanfare  *FanfareConfig       `json:"fanfare,omitempty"`
	Jobs     map[string]JobConfig `json:"jobs,omitempty"`
}

// tenantDoc is .miniwaves/<slug>.json.
type tenantDoc struct {
	Next            map[string]int    `json:"next"`
	Items           []Item            `json:"items"`
	Sessions        []Session         `json:"sessions,omitempty"`
	Gates           map[string]string `json:"gates,omitempty"`
	GatesSeen       map[string]bool   `json:"gates_seen,omitempty"`
	GateBy          string            `json:"gate_by,omitempty"`
	GateLatch       map[string]string `json:"gate_latch,omitempty"`
	AcceptStreak    int               `json:"accept_streak,omitempty"`
	Archived        int               `json:"archived,omitempty"`
	Protocol        []string          `json:"protocol,omitempty"`
	FanfareFinaleAt string            `json:"fanfare_finale_at,omitempty"`
}

func sum(b []byte) string { h := sha256.Sum256(b); return string(h[:]) }

func readIndexBytes(path string) ([]byte, error) {
	b, err := readFileShared(path)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(bytes.TrimLeft(b, " \t\r\n"), []byte("<<<<<<<")) {
		return nil, errors.New("board has merge markers: resolve it in git")
	}
	return b, nil
}

// load reads the index and every tenant.
func load(path string) (*Workspace, error) { return loadScoped(path, true) }

// loadScoped reads the index and, with all, every tenant file, else only the named ones
// (the other tenants stay listed but empty: a command must not touch them).
func loadScoped(path string, all bool, slugs ...string) (*Workspace, error) {
	return loadScopedFrom(path, currentSchema, all, slugs...)
}

// checkSchema is the guard every reader runs: a board newer than cur is refused (upgrade mw), an older one
// needs mw migrate. cur is currentSchema (a parameter so a test can play an older build).
func checkSchema(path string, v, cur int, hasTenants bool) error {
	if v > cur {
		return fmt.Errorf("workspace schema %d is newer than this mw supports (schema %d): upgrade mw", v, cur)
	}
	if v < cur || !hasTenants {
		return fmt.Errorf("%s is schema %d and this mw only reads schema %d: run mw migrate (see MIGRATION.md)", path, v, cur)
	}
	return nil
}

// loadScopedFrom is loadScoped for a board of schema minSchema (mw migrate reads the previous one).
func loadScopedFrom(path string, minSchema int, all bool, slugs ...string) (*Workspace, error) {
	b, err := readIndexBytes(path)
	if err != nil {
		return nil, err
	}
	var ix indexDoc
	if err := json.Unmarshal(b, &ix); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := checkSchema(path, ix.Version, minSchema, len(ix.Tenants) > 0); err != nil {
		return nil, err
	}
	w := &Workspace{Version: ix.Version, Name: ix.Name, Defaults: ix.Defaults, Protocol: ix.Protocol, Folders: ix.Folders,
		Gates: ix.Gates, GateBy: ix.GateBy, Publish: ix.Publish, Fanfare: ix.Fanfare, Jobs: ix.Jobs, Sessions: []Session{}, base: map[string]string{}, path: path}
	if w.Defaults == nil {
		w.Defaults = map[string]string{}
	}
	want := map[string]bool{}
	for _, s := range slugs {
		want[s] = true
	}
	for _, it := range ix.Tenants {
		t := Tenant{Slug: it.Slug, Name: it.Name, Path: it.Path, Hidden: it.Archived, Tags: it.Tags, Job: it.Job, Next: map[string]int{}, Items: []Item{}}
		if all || want[it.Slug] {
			tb, err := readFileShared(tenantPath(path, it.Slug))
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			if err == nil {
				var td tenantDoc
				if err := json.Unmarshal(tb, &td); err != nil {
					return nil, fmt.Errorf("read %s: %w", tenantPath(path, it.Slug), err)
				}
				t.applyDoc(td)
				w.Sessions = append(w.Sessions, td.Sessions...)
				w.base[it.Slug] = sum(tb)
			}
			t.loaded = true
		}
		w.Tenants = append(w.Tenants, t)
	}
	w.indexBase = sum(indexBytes(w))
	syncLocal(w)
	return w, nil
}

func (t *Tenant) applyDoc(td tenantDoc) {
	t.GateBy = td.GateBy
	t.Next, t.Items, t.Gates, t.GatesSeen, t.GateLatch, t.AcceptStreak, t.Archived, t.Protocol, t.FanfareFinaleAt =
		td.Next, td.Items, td.Gates, td.GatesSeen, td.GateLatch, td.AcceptStreak, td.Archived, td.Protocol, td.FanfareFinaleAt
	if t.Next == nil {
		t.Next = map[string]int{}
	}
	if t.Items == nil {
		t.Items = []Item{}
	}
}

func indexBytes(w *Workspace) []byte {
	ix := indexDoc{Version: w.Version, Name: w.Name, Defaults: w.Defaults, Protocol: w.Protocol, Folders: w.Folders, Gates: w.Gates, GateBy: w.GateBy, Publish: w.Publish, Fanfare: w.Fanfare}
	if len(w.Jobs) > 0 {
		ix.Jobs = w.Jobs
	}
	if len(ix.Defaults) == 0 {
		ix.Defaults = nil
	}
	for _, t := range w.Tenants {
		ix.Tenants = append(ix.Tenants, indexTenant{t.Slug, t.Name, t.Path, t.Hidden, t.Tags, t.Job})
	}
	b, _ := json.Marshal(ix)
	return append(b, '\n')
}

func tenantBytes(w *Workspace, t *Tenant) []byte {
	td := tenantDoc{Next: t.Next, Items: t.Items, Gates: t.Gates, GateBy: t.GateBy, GatesSeen: t.GatesSeen, GateLatch: t.GateLatch, AcceptStreak: t.AcceptStreak, Archived: t.Archived, Protocol: t.Protocol, FanfareFinaleAt: t.FanfareFinaleAt}
	for _, s := range w.Sessions {
		if s.Tenant == t.Slug {
			td.Sessions = append(td.Sessions, s)
		}
	}
	b, _ := json.Marshal(td)
	return append(b, '\n')
}

// save writes what changed: each loaded tenant whose file differs, and the index.
func save(path string, w *Workspace) error {
	syncLocal(w)
	refreshGates(w)
	resetFinale(w)
	if err := os.MkdirAll(filepath.Join(filepath.Dir(path), storeDir), 0o755); err != nil {
		return err
	}
	if w.base == nil {
		w.base = map[string]string{}
	}
	wrote := false
	for i := range w.Tenants {
		t := &w.Tenants[i]
		if !t.loaded {
			continue
		}
		b := tenantBytes(w, t)
		if s := sum(b); s != w.base[t.Slug] {
			if err := writeFileAtomic(tenantPath(path, t.Slug), b); err != nil {
				return err
			}
			w.base[t.Slug] = s
			wroteTenants[t.Slug] = true
			wrote = true
		}
	}
	if b := indexBytes(w); sum(b) != w.indexBase {
		if err := writeFileAtomic(path, b); err != nil {
			return err
		}
		w.indexBase = sum(b)
		wrote = true
	}
	if wrote {
		savedCount++
	}
	lastSaved = w
	return nil
}

// sessionElsewhere refuses a session name that is live in another tenant (sessions are stored
// per tenant, so the other tenant files are read, lock-free, before a new one is registered).
func sessionElsewhere(path string, w *Workspace, id, tenant string) error {
	needle := []byte(`"id":"` + id + `"`)
	for _, t := range w.Tenants {
		if t.Slug == tenant {
			continue
		}
		b, err := readFileShared(tenantPath(path, t.Slug))
		if err != nil || !bytes.Contains(b, needle) {
			continue
		}
		var td tenantDoc
		if json.Unmarshal(b, &td) != nil {
			continue
		}
		for _, s := range td.Sessions {
			if s.ID == id && s.State != "stopped" {
				return fmt.Errorf("session and tenant do not match: %s is live in tenant %s", id, t.Slug)
			}
		}
	}
	return nil
}

// ---- locks ----

// lockWait is how long a writer may wait for its locks before the command fails:
// MW_LOCK_WAIT seconds, else 5 s (15 s on Windows, where process start and file I/O are slower).
var lockWait = lockWaitFrom(os.Getenv("MW_LOCK_WAIT"), runtime.GOOS)

// lockWaitFrom parses MW_LOCK_WAIT (seconds, fractions allowed); empty, invalid or
// non-positive values fall back to the platform default.
func lockWaitFrom(env, goos string) time.Duration {
	if s, err := strconv.ParseFloat(strings.TrimSpace(env), 64); err == nil && s > 0 && s <= 3600 {
		return time.Duration(s * float64(time.Second))
	}
	if goos == "windows" {
		return 15 * time.Second
	}
	return 5 * time.Second
}

// lockTimeoutError is the message for a lock wait that ran out (wr matches "board locked").
func lockTimeoutError(pid string, wait time.Duration) error {
	return fmt.Errorf("board locked (pid %s): lock wait timed out after %s — retry, or raise MW_LOCK_WAIT (seconds)", pid, wait.Round(time.Millisecond))
}

// lockFile takes an exclusive lock on lp. The kernel does the waiting (a blocking
// flock/LockFileEx in a goroutine, no polling); deadline is when the wait gives up, naming
// the pid last recorded in <lp>.pid.
func lockFile(lp string, deadline time.Time) (func(), error) {
	pp := lp + ".pid"
	wait := lockWait
	if err := os.MkdirAll(filepath.Dir(lp), 0o755); err != nil {
		return nil, err
	}
	l := flock.New(lp, flock.SetPermissions(0600))
	res := make(chan error, 1)
	go func() { res <- l.Lock() }()
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case err := <-res:
		if err != nil {
			return nil, err
		}
	case <-timer.C:
		go func() { // the abandoned wait may still win the lock: give it straight back
			if <-res == nil {
				_ = l.Unlock()
			}
		}()
		pid := "?"
		if b, e := readFileShared(pp); e == nil && strings.TrimSpace(string(b)) != "" {
			pid = strings.TrimSpace(string(b))
		}
		return nil, lockTimeoutError(pid, wait)
	}
	// written first and cheaply (tmp + rename, no fsync): the last acquirer's pid stays
	pid := strconv.Itoa(os.Getpid())
	if tmp := pp + "." + pid; os.WriteFile(tmp, []byte(pid), 0600) == nil {
		if renameRetry(tmp, pp) != nil {
			_ = os.Remove(tmp)
		}
	}
	var once sync.Once
	return func() { once.Do(func() { _ = l.Unlock() }) }, nil
}

// lockSet locks the given tenants (sorted, so writers never deadlock) and, when index, the index lock first.
func lockSet(path string, index bool, slugs []string) (func(), error) {
	deadline := time.Now().Add(lockWait)
	var unlocks []func()
	release := func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
	take := func(name string) error {
		u, err := lockFile(storePath(path, name), deadline)
		if err != nil {
			release()
			return err
		}
		unlocks = append(unlocks, u)
		return nil
	}
	if index {
		if err := take("index.lock"); err != nil {
			return nil, err
		}
	}
	s := append([]string(nil), slugs...)
	sort.Strings(s)
	for _, slug := range s {
		if err := take(slug + ".lock"); err != nil {
			return nil, err
		}
	}
	return release, nil
}

// scope says what a command touches: which tenants to load (all, or just one) and whether it may write the index.
type scope struct {
	all   bool
	slugs []string
	index bool
}

// scopeOf decides from the arguments and the index. Unsure means all tenants.
func scopeOf(w *Workspace, args []string) scope {
	all := scope{all: true}
	sub := ""
	if len(args) > 1 {
		sub = args[1]
	}
	o, _, err := options(args[1:])
	if err != nil {
		return all
	}
	hasT := o["tenant"] != "" || os.Getenv("MW_TENANT") != ""
	if o["all"] == "true" || o["html"] == "true" || o["md"] == "true" {
		return all
	}
	if !hasT && (o["session"] != "" || os.Getenv("MW_SESSION") != "") {
		return all // the session's tenant lives in a tenant file
	}
	single := false
	idx := false
	switch args[0] {
	case "add", "ready", "start", "block", "done", "reopen", "show", "edit", "ask", "answer", "decide", "next", "accept", "reject", "wave", "review", "judge", "log", "list", "ls", "board", "context":
		single = true
	case "archive":
		single = o["tenant"] != ""
	case "gate":
		single = hasT
		idx = !hasT
	case "session":
		single = hasT
	case "get-prompt":
		single = sub == "session"
	case "tenant", "dirs", "fanfare", "jobs", "init":
		idx = true
	}
	if !single {
		return scope{all: true, index: idx}
	}
	t, err := selectTenantRaw(w, o)
	if err != nil {
		return all
	}
	return scope{slugs: []string{t.Slug}, index: idx}
}

func sameTenants(a, b *Workspace) bool {
	if len(a.Tenants) != len(b.Tenants) {
		return false
	}
	for i := range a.Tenants {
		if a.Tenants[i].Slug != b.Tenants[i].Slug {
			return false
		}
	}
	return true
}
