package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	dataFile      = ".miniwaves.json"
	currentSchema = 8
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
	Created    string   `json:"created"`
	Updated    string   `json:"updated"`
}

// Event is one activity-log entry of an item.
type Event struct {
	At     string `json:"at"`
	Action string `json:"action"`
	By     string `json:"by,omitempty"`
	Note   string `json:"note,omitempty"`
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

	Gates        map[string]string `json:"gates,omitempty"`      // evidence|acceptance|judge: auto|on|off
	GatesSeen    map[string]bool   `json:"gates_seen,omitempty"` // effective state last recorded by a saving command
	GateLatch    map[string]string `json:"gate_latch,omitempty"` // name: reason; auto stays on once latched
	AcceptStreak int               `json:"accept_streak,omitempty"`
	Archived     int               `json:"archived,omitempty"` // done items moved to .miniwaves/<slug>.archive.json (mw archive)
	loaded       bool
}

type Session struct {
	ID      string `json:"id"`
	Tenant  string `json:"tenant"`
	Agent   string `json:"agent,omitempty"`
	State   string `json:"state"`
	Item    string `json:"item,omitempty"`
	Updated string `json:"updated"`
}

type Workspace struct {
	Version  int               `json:"version"`
	Name     string            `json:"name"`
	Defaults map[string]string `json:"folder_defaults,omitempty"`
	Tenants  []Tenant          `json:"tenants"`
	Sessions []Session         `json:"sessions"`
	Protocol []string          `json:"protocol,omitempty"`
	Folders  map[string]string `json:"folders,omitempty"`
	Gates    map[string]string `json:"gates,omitempty"` // workspace default per gate; absent = auto
	Publish  *PublishConfig    `json:"publish,omitempty"`

	base      map[string]string // sha of each loaded tenant file as read (save writes only what changed)
	indexBase string
}

type itemBrief struct {
	ID    string `json:"id" toon:"id"`
	Type  string `json:"type" toon:"type"`
	State string `json:"state" toon:"state"`
	Title string `json:"title" toon:"title"`
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
}

type tenantBrief struct {
	Tenant   string `json:"tenant" toon:"tenant"`
	Items    int    `json:"items" toon:"items"`
	Selected bool   `json:"selected" toon:"selected"`
}

type tenantDetail struct {
	Tenant   string `json:"tenant" toon:"tenant"`
	Name     string `json:"name" toon:"name"`
	Items    int    `json:"items" toon:"items"`
	Selected bool   `json:"selected" toon:"selected"`
	Path     string `json:"path" toon:"path"`
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
	if _, err := os.Stat(path); err == nil {
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

func findWorkspace() (string, error) {
	d := mustGetwd()
	for {
		p := filepath.Join(d, dataFile)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return "", errors.New("no .miniwaves.json found; run `mw init`")
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
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	if syncWrites && runtime.GOOS != "windows" {
		if d, err := os.Open(filepath.Dir(path)); err == nil {
			_ = d.Sync()
			d.Close()
		}
	}
	return nil
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
	for i := range w.Sessions {
		if w.Sessions[i].ID == id {
			return &w.Sessions[i], nil
		}
	}
	return nil, fmt.Errorf("session %s not found", id)
}

func folderDefault(w *Workspace, cwd string) (string, string) {
	best, folder, slug := -1, "", ""
	for candidate, tenant := range w.Defaults {
		p, _ := filepath.Abs(candidate)
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
	cwd, _ := filepath.Abs(mustGetwd())
	best := -1
	var match *Tenant
	if folder, slug := folderDefault(w, cwd); slug != "" {
		t, err := tenantBySlug(w, slug)
		if err != nil {
			return nil, fmt.Errorf("folder default for %s: %w", folder, err)
		}
		return t, nil
	}
	for i := range w.Tenants {
		if w.Tenants[i].Path == "" {
			continue
		}
		p, _ := filepath.Abs(w.Tenants[i].Path)
		if (cwd == p || strings.HasPrefix(cwd, p+string(os.PathSeparator))) && len(p) > best {
			match, best = &w.Tenants[i], len(p)
		}
	}
	if match != nil {
		return match, nil
	}
	if len(w.Tenants) == 0 {
		return nil, errors.New("no tenants")
	}
	return &w.Tenants[0], nil
}
