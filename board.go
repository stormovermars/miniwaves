package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	dataFile      = ".miniwaves.json"
	currentSchema = 9
)

var kinds = map[string]string{
	"prompt": "PRM", "story": "STR", "task": "TSK", "spike": "SPK",
	"bug": "BUG", "chore": "CHR", "wave": "WAV",
}

var statuses = map[string]bool{"inbox": true, "ready": true, "doing": true, "blocked": true, "done": true}

type Item struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Title      string   `json:"title"`
	Status     string   `json:"status"`
	Parent     string   `json:"parent,omitempty"`
	Wave       string   `json:"wave,omitempty"`
	Assignee   string   `json:"session,omitempty"`
	Body       string   `json:"body,omitempty"`
	Acceptance string   `json:"acceptance,omitempty"`
	Note       string   `json:"note,omitempty"`
	Notes      []string `json:"notes,omitempty"`
	Steps      []Step   `json:"steps,omitempty"`
	Evidence   []string `json:"evidence,omitempty"`  // "relative/path size", recorded at done
	Judged     string   `json:"judged,omitempty"`    // RFC3339 | ungated
	By         string   `json:"by,omitempty"`        // session at done
	JudgedBy   string   `json:"judged_by,omitempty"` // human at accept
	Attempts   int      `json:"attempts,omitempty"`
	Events     []Event  `json:"events,omitempty"` // activity log, appended by every state-changing command
	Ticket     string   `json:"ticket,omitempty"` // optional tracker ticket in canonical form (see tracker.go)
	Created    string   `json:"created"`
	Updated    string   `json:"updated"`

	extra map[string]json.RawMessage // fields this mw does not know: kept, written back
}

// UnmarshalJSON reads the known fields and keeps unknown ones, so an older or newer mw's fields survive a save.
func (it *Item) UnmarshalJSON(b []byte) error {
	type plain Item
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	for _, k := range itemJSONKeys {
		delete(raw, k)
	}
	p.extra = nil
	if len(raw) > 0 {
		p.extra = raw
	}
	*it = Item(p)
	return nil
}

var itemJSONKeys = []string{"id", "kind", "title", "status", "parent", "wave", "session", "body", "acceptance", "note", "notes", "steps", "evidence", "judged", "by", "judged_by", "attempts", "events", "ticket", "created", "updated"}

// MarshalJSON writes the known fields, then the kept unknown ones.
func (it Item) MarshalJSON() ([]byte, error) {
	type plain Item
	b, err := json.Marshal(plain(it))
	if err != nil || len(it.extra) == 0 {
		return b, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	for k, v := range it.extra {
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}
	return json.Marshal(m)
}

// Event is one activity-log entry of an item.
type Event struct {
	At     string `json:"at"`
	Action string `json:"action"`
	By     string `json:"by,omitempty"`
	Note   string `json:"note,omitempty"`
	On     string `json:"on,omitempty"` // block events: the item this one waits for (mw block --on ID)
}

// Step is one delivery-protocol checkbox on a wave.
type Step struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
	NA    bool   `json:"na,omitempty"`
	Note  string `json:"note,omitempty"`
	When  string `json:"when,omitempty"`
}

type Tenant struct {
	Slug  string         `json:"slug"`
	Name  string         `json:"name"`
	Path  string         `json:"path,omitempty"`
	Next  map[string]int `json:"next"`
	Items []Item         `json:"items"`

	Gates           map[string]string `json:"gates,omitempty"`             // evidence|acceptance|judge: auto|on|off
	GateBy          string            `json:"gate_by,omitempty"`           // "NAME RFC3339" of the last gate change on this tenant
	GatesSeen       map[string]bool   `json:"gates_seen,omitempty"`        // effective state last recorded by a saving command
	FanfareFinaleAt string            `json:"fanfare_finale_at,omitempty"` // set when the tenant became finished (mw fanfare)
	Protocol        []string          `json:"protocol,omitempty"`          // mw tenant protocol: wave steps for new waves, wins over the workspace list
	GateLatch       map[string]string `json:"gate_latch,omitempty"`        // name: reason; auto stays on once latched
	AcceptStreak    int               `json:"accept_streak,omitempty"`
	Archived        int               `json:"archived,omitempty"` // done items moved to .miniwaves/<slug>.archive.json (mw archive)
	Job             string            `json:"job,omitempty"`      // coordinated job id (mw jobs enable); stored in the index
	Hidden          bool              `json:"-"`                  // mw tenant archive (stored in the index as "archived")
	Tags            []string          `json:"-"`                  // mw tenant tag (stored in the index)
	loaded          bool
}

type Session struct {
	ID      string `json:"id"`
	Tenant  string `json:"tenant"`
	Agent   string `json:"agent,omitempty"`
	State   string `json:"state"`
	Item    string `json:"item,omitempty"`
	Updated string `json:"updated"`
	// ClaudeSession is the Claude Code session id recorded by mw session start --claude-session, so that
	// mw context --hook-json can find this session from the SessionStart hook's stdin. A session-record
	// field (omitempty; older binaries ignore it), not an item field: the schema is unchanged.
	// An older mw (<0.3.15) drops it when it rewrites the board: re-run mw session start NAME --claude-session ID.
	ClaudeSession string `json:"claude_session,omitempty"`
	// Answerer is the session that may answer this session's asks (mw session start NAME --answerer PARENT).
	// Also a session-record field (omitempty): an older mw drops it when it rewrites the board.
	Answerer string `json:"answerer,omitempty"`
	// AnswererAt is when the answerer was set: it answers only asks made at or after it (no retroactive routing).
	AnswererAt string `json:"answerer_at,omitempty"`
}

type Workspace struct {
	showArchived bool                 // mw board --html --archived: archived tenants on the page, greyed
	Version      int                  `json:"version"`
	Name         string               `json:"name"`
	Defaults     map[string]string    `json:"folder_defaults,omitempty"`
	Tenants      []Tenant             `json:"tenants"`
	Sessions     []Session            `json:"sessions"`
	Protocol     []string             `json:"protocol,omitempty"`
	Folders      map[string]string    `json:"folders,omitempty"`
	Gates        map[string]string    `json:"gates,omitempty"`   // workspace default per gate; absent = auto
	GateBy       string               `json:"gate_by,omitempty"` // "NAME RFC3339" of the last workspace default change
	Publish      *PublishConfig       `json:"publish,omitempty"`
	Fanfare      *FanfareConfig       `json:"fanfare,omitempty"`
	Jobs         map[string]JobConfig `json:"jobs,omitempty"` // coordinated jobs: config by job id (mw jobs)

	path       string    // the index file, to read the sessions of tenants this command did not load
	otherSess  []Session // those sessions, read once (allSessions)
	otherDone  bool
	wantOthers bool              // read the unloaded tenants even when no loaded session has an answerer (the cycle check)
	base       map[string]string // sha of each loaded tenant file as read (save writes only what changed)
	indexBase  string
}

type itemBrief struct {
	ID     string `json:"id" toon:"id"`
	Type   string `json:"type" toon:"type"`
	State  string `json:"state" toon:"state"`
	Title  string `json:"title" toon:"title"`
	Ticket string `json:"ticket,omitempty" toon:"ticket,omitempty"`
}

type itemDetail struct {
	ID             string   `json:"id" toon:"id"`
	Type           string   `json:"type" toon:"type"`
	State          string   `json:"state" toon:"state"`
	Title          string   `json:"title" toon:"title"`
	Parent         string   `json:"parent" toon:"parent"`
	Wave           string   `json:"wave,omitempty" toon:"wave,omitempty"`
	Session        string   `json:"session" toon:"session"`
	Body           string   `json:"body" toon:"body"`
	Acceptance     string   `json:"acceptance" toon:"acceptance"`
	AcceptanceFrom string   `json:"acceptance_from,omitempty" toon:"acceptance_from,omitempty"`
	Note           string   `json:"note" toon:"note"`
	Notes          []string `json:"notes,omitempty" toon:"notes,omitempty"`
	Evidence       []string `json:"evidence,omitempty" toon:"evidence,omitempty"`
	E              string   `json:"e,omitempty" toon:"e,omitempty"` // "E" in list -v when evidence exists
	Judged         string   `json:"judged,omitempty" toon:"judged,omitempty"`
	By             string   `json:"by,omitempty" toon:"by,omitempty"`
	JudgedBy       string   `json:"judged_by,omitempty" toon:"judged_by,omitempty"`
	Attempts       int      `json:"attempts,omitempty" toon:"attempts,omitempty"`
	Created        string   `json:"created" toon:"created"`
	Updated        string   `json:"updated" toon:"updated"`
	Ticket         string   `json:"ticket,omitempty" toon:"ticket,omitempty"`
}

type tenantBrief struct {
	Tenant   string   `json:"tenant" toon:"tenant"`
	Items    int      `json:"items" toon:"items"`
	Selected bool     `json:"selected" toon:"selected"`
	Tags     []string `json:"tags,omitempty" toon:"tags,omitempty"`
}

type tenantDetail struct {
	Tenant   string   `json:"tenant" toon:"tenant"`
	Name     string   `json:"name" toon:"name"`
	Items    int      `json:"items" toon:"items"`
	Selected bool     `json:"selected" toon:"selected"`
	Path     string   `json:"path" toon:"path"`
	Tags     []string `json:"tags,omitempty" toon:"tags,omitempty"`
	Archived bool     `json:"archived,omitempty" toon:"archived,omitempty"`
}

type sessionBrief struct {
	Session string `json:"session" toon:"session"`
	Tenant  string `json:"tenant" toon:"tenant"`
	State   string `json:"state" toon:"state"`
	Item    string `json:"item" toon:"item"`
}

type sessionDetail struct {
	Session string `json:"session" toon:"session"`
	Tenant  string `json:"tenant" toon:"tenant"`
	Agent   string `json:"agent" toon:"agent"`
	State   string `json:"state" toon:"state"`
	Item    string `json:"item" toon:"item"`
	Updated string `json:"updated" toon:"updated"`
	Age     string `json:"age" toon:"age"`
}

type folderPaths struct {
	Datalake  string `json:"datalake" toon:"datalake"`
	Docs      string `json:"docs" toon:"docs"`
	Data      string `json:"data" toon:"data"`
	Tools     string `json:"tools" toon:"tools"`
	Templates string `json:"templates" toon:"templates"`
	Builds    string `json:"builds" toon:"builds"`
	Evidence  string `json:"evidence" toon:"evidence"`
	Scratch   string `json:"scratch" toon:"scratch"`
}

type descriptionInfo struct {
	Purpose   string   `json:"purpose" toon:"purpose"`
	Model     string   `json:"model" toon:"model"`
	Human     []string `json:"human" toon:"human"`
	Assistant []string `json:"assistant" toon:"assistant"`
	Start     []string `json:"start" toon:"start"`
	Storage   string   `json:"storage" toon:"storage"`
	More      string   `json:"more" toon:"more"`
}

func initWorkspace(name string) error {
	path := filepath.Join(mustGetwd(), dataFile)
	if _, err := statShared(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	slug := slugify(name)
	if slug == "" {
		slug = "default"
	}
	w := &Workspace{Version: currentSchema, Name: name,
		Tenants: []Tenant{{Slug: slug, Name: name, Path: mustGetwd(), Next: map[string]int{}, Items: []Item{}, loaded: true}}}
	touchTenant(slug)
	if err := save(path, w); err != nil {
		return err
	}
	ensureGitExclude(filepath.Dir(path))
	fmt.Printf("initialized %s in %s; created tenant %s for folder %s\n", name, path, slug, filepath.Base(mustGetwd()))
	return nil
}

// findWorkspace is the board for the cwd (boardPath; in a worktree usually the main checkout's) and
// remembers where it was found for the cwd mapping (workDir) and the worktree sync.
func findWorkspace() (string, error) {
	workLoc = boardPath(mustGetwd())
	if workLoc.warn != "" {
		fmt.Fprintln(os.Stderr, "mw: "+workLoc.warn)
	}
	if workLoc.path == "" {
		return "", errors.New("no .miniwaves.json found; run `mw init`")
	}
	return workLoc.path, nil
}

// syncWrites fsyncs the temp file before the rename (tests switch it off: fsync is slow on macOS).
var syncWrites = true

// writeFileAtomic writes tmp, syncs it to disk, then renames over path.
func writeFileAtomic(path string, b []byte) error { return writeFileAtomicMode(path, b, 0600) }

// writeFileAtomicMode is writeFileAtomic with an explicit file mode (the board is 0600, the published page 0644).
func writeFileAtomicMode(path string, b []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp") // unique: concurrent writers never share a temp file
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if syncWrites {
		if err := f.Sync(); err != nil {
			f.Close()
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	if err := renameRetry(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if syncWrites && runtime.GOOS != "windows" { // Windows cannot open (or fsync) a directory
		if d, err := os.Open(filepath.Dir(path)); err == nil {
			_ = d.Sync()
			d.Close()
		}
	}
	return nil
}

// renameTries and renameBackoff bound renameRetry: 50 x 20 ms.
var (
	renameTries   = 50
	renameBackoff = 20 * time.Millisecond
	renameFn      = replaceFile
	// renameRetryOn: only Windows has transient sharing violations; elsewhere one attempt.
	renameRetryOn = runtime.GOOS == "windows"
)

// renameRetry renames tmp over path (replaceFile), retrying a failed rename. On Windows
// replaceFile uses the POSIX rename, which replaces a target that readers hold open with
// FILE_SHARE_DELETE (openShared); a handle without it (an antivirus scan, a foreign os.Open)
// still refuses until it closes, hence the retries. A missing tmp is not retried.
func renameRetry(tmp, path string) error {
	var err error
	tries := renameTries
	if !renameRetryOn {
		tries = 1
	}
	for i := 0; i < tries; i++ {
		if err = renameFn(tmp, path); err == nil || errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if i+1 < tries {
			time.Sleep(renameBackoff)
		}
	}
	return err
}

func tenantBySlug(w *Workspace, slug string) (*Tenant, error) {
	for i := range w.Tenants {
		if w.Tenants[i].Slug == slug {
			return &w.Tenants[i], nil
		}
	}
	return nil, fmt.Errorf("tenant %s not found", slug)
}

func sessionByID(w *Workspace, id string) (*Session, error) {
	found := -1
	for i := range w.Sessions {
		if w.Sessions[i].ID == id && (found < 0 || w.Sessions[found].State == "stopped" && w.Sessions[i].State != "stopped") {
			found = i // an id may sit in two tenant files: the record that is not stopped wins
		}
	}
	if found < 0 {
		return nil, fmt.Errorf("session %s not found", id)
	}
	return &w.Sessions[found], nil
}

func folderDefault(w *Workspace, cwd string) (string, string) {
	best, folder, slug := -1, "", ""
	for candidate, tenant := range w.Defaults {
		p, _ := filepath.Abs(candidate)
		p = matchDir(p)
		if (cwd == p || strings.HasPrefix(cwd, p+string(os.PathSeparator))) && len(p) > best {
			best, folder, slug = len(p), candidate, tenant
		}
	}
	return folder, slug
}

// selectTenant resolves the tenant and prints the gate flip line (stderr)
// when its effective gates differ from gates_seen.
func selectTenant(w *Workspace, o map[string]string) (*Tenant, error) {
	t, err := selectTenantRaw(w, o)
	if err == nil {
		announceTenant(w, t)
	}
	return t, err
}

func selectTenantRaw(w *Workspace, o map[string]string) (*Tenant, error) {
	t, err := selectTenantAny(w, o)
	if err == nil {
		err = refuseArchived(t)
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

func selectTenantAny(w *Workspace, o map[string]string) (*Tenant, error) {
	sessionID := value(o, "session", os.Getenv("MW_SESSION"))
	if sessionID != "" {
		// an unknown session is never an error: fall through to the other rules
		if s, err := sessionByID(w, sessionID); err == nil {
			if o["tenant"] != "" && o["tenant"] != s.Tenant {
				return nil, errors.New("session and tenant do not match")
			}
			return tenantBySlug(w, s.Tenant)
		}
	}
	if slug := value(o, "tenant", os.Getenv("MW_TENANT")); slug != "" {
		return tenantBySlug(w, slug)
	}
	if os.Getenv("MW_SESSION") != "" && o["session"] == "" {
		if _, err := sessionByID(w, sessionID); err != nil {
			fmt.Fprintf(os.Stderr, "mw: MW_SESSION %s is not registered and no tenant was given; using the folder/first tenant (mw session start %s -t SLUG)\n", sessionID, sessionID)
		}
	}
	cwd, _ := filepath.Abs(workDir())
	if t, err := tenantByFolder(w, cwd); t != nil || err != nil {
		return t, err
	}
	for i := range w.Tenants { // the first tenant that is not archived
		if !w.Tenants[i].Hidden {
			return &w.Tenants[i], nil
		}
	}
	if len(w.Tenants) == 0 {
		return nil, errors.New("no tenants")
	}
	return &w.Tenants[0], nil
}

// tenantByFolder is the tenant the folder cwd selects: the nearest folder default, else the nearest
// tenant path; nil when none matches.
func tenantByFolder(w *Workspace, cwd string) (*Tenant, error) {
	if folder, slug := folderDefault(w, cwd); slug != "" {
		t, err := tenantBySlug(w, slug)
		if err != nil {
			return nil, fmt.Errorf("folder default for %s: %w", folder, err)
		}
		return t, nil
	}
	best := -1
	var match *Tenant
	for i := range w.Tenants {
		if w.Tenants[i].Path == "" {
			continue
		}
		p, _ := filepath.Abs(w.Tenants[i].Path)
		p = matchDir(p)
		if (cwd == p || strings.HasPrefix(cwd, p+string(os.PathSeparator))) && len(p) > best {
			match, best = &w.Tenants[i], len(p)
		}
	}
	return match, nil
}
