package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Worktree boards (BUG-019, docs: TECHNICAL.md "Worktree boards"). In a linked git worktree whose
// main checkout has the board, every command reads and writes the main board (write-through) and
// then copies what it touched into the worktree's board. mw runs no git here: the worktree is
// found from the .git file, gitdir and commondir alone.

// worktreeOf finds the linked worktree that holds dir: the first .git walking up from dir (symlinks
// resolved). A .git file "gitdir: G" whose G/commondir names a common dir called .git is a worktree;
// wt is its root, main the parent of the common dir. A .git folder (a plain checkout), a submodule
// (no commondir) and a bare or --separate-git-dir repo (the common dir is not named .git) are not.
func worktreeOf(dir string) (wt, main string, ok bool) {
	d := realDir(dir)
	for {
		g := filepath.Join(d, ".git")
		if fi, err := os.Lstat(g); err == nil {
			if fi.IsDir() {
				return "", "", false
			}
			b, err := readFileShared(g)
			if err != nil {
				return "", "", false
			}
			ref, found := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir:")
			if !found {
				return "", "", false
			}
			gd := gitPath(d, strings.TrimSpace(ref), runtime.GOOS)
			c, err := readFileShared(filepath.Join(gd, "commondir"))
			if err != nil {
				return "", "", false
			}
			common := gitPath(gd, strings.TrimSpace(string(c)), runtime.GOOS)
			if !pathEqual(filepath.Base(common), ".git", runtime.GOOS) {
				return "", "", false
			}
			m, err := filepath.EvalSymlinks(filepath.Dir(common))
			if err != nil {
				return "", "", false
			}
			return d, m, true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", "", false
		}
		d = parent
	}
}

// gitPath resolves a path git wrote into a .git file or commondir (forward slashes, also on
// Windows) against base when it is relative. goos is a parameter so tests cover Windows paths
// (drive letters, UNC) on any host.
func gitPath(base, ref, goos string) string {
	win := goos == "windows"
	if win {
		base, ref = strings.ReplaceAll(base, `\`, "/"), strings.ReplaceAll(ref, `\`, "/")
	}
	drive := len(ref) >= 2 && ref[1] == ':' && (ref[0]|0x20 >= 'a' && ref[0]|0x20 <= 'z')
	if !(strings.HasPrefix(ref, "/") || win && drive) {
		ref = strings.TrimSuffix(base, "/") + "/" + ref
	}
	unc := win && strings.HasPrefix(ref, "//")
	p := path.Clean(ref)
	if unc {
		p = "/" + p
	}
	if win {
		p = strings.ReplaceAll(p, "/", `\`)
	}
	return p
}

// pathEqual compares two paths, ignoring case on Windows.
func pathEqual(a, b, goos string) bool {
	if goos == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// relWithin is p relative to root ("." for root itself) when p is root or inside it (case-insensitive on Windows).
func relWithin(p, root, goos string) (string, bool) {
	sep := "/"
	if goos == "windows" {
		sep = `\`
	}
	root = strings.TrimSuffix(root, sep)
	if pathEqual(p, root, goos) {
		return ".", true
	}
	if len(p) > len(root) && pathEqual(p[:len(root)], root, goos) && p[len(root):len(root)+1] == sep {
		return p[len(root)+1:], true
	}
	return "", false
}

// boardLoc is where a command's board lives.
type boardLoc struct {
	path     string // the board every read and write goes to ("" = none found)
	wtPath   string // write-through: the worktree's own board, synced after each command
	wt, main string // set when path is the main checkout's board: the cwd wt/X reads as main/X
}

// worktreeOff is MW_WORKTREE=off: the nearest board, worktree or not (the behaviour before 0.3.16).
func worktreeOff() bool { return strings.EqualFold(strings.TrimSpace(os.Getenv("MW_WORKTREE")), "off") }

// boardPath is the one resolver every board lookup goes through. Outside a linked worktree it is
// the nearest board walking up from dir. In one, with R the nearest board folder from dir up to the
// worktree root: main/R's board when both wt/R and main/R have one (write-through), wt/R's board
// alone when main/R has none, and with no board in the worktree (an untracked board) the board
// nearest to main/X for dir wt/X.
func boardPath(dir string) boardLoc {
	local := nearestBoard(dir)
	if worktreeOff() {
		return boardLoc{path: local}
	}
	wt, main, ok := worktreeOf(dir)
	if !ok {
		return boardLoc{path: local}
	}
	rel, ok := relWithin(realDir(dir), wt, runtime.GOOS)
	if !ok {
		return boardLoc{path: local}
	}
	for r := rel; ; r = filepath.Dir(r) {
		if p := filepath.Join(wt, r, dataFile); fileExists(p) {
			if m := filepath.Join(main, r, dataFile); fileExists(m) {
				return boardLoc{path: m, wtPath: p, wt: wt, main: main}
			}
			return boardLoc{path: p}
		}
		if r == "." {
			break
		}
	}
	if p := nearestBoard(filepath.Join(main, rel)); p != "" {
		return boardLoc{path: p, wt: wt, main: main}
	}
	return boardLoc{path: local}
}

// nearestBoard walks up from dir to the nearest board file ("" when there is none).
func nearestBoard(dir string) string {
	d, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		p := filepath.Join(d, dataFile)
		if fileExists(p) {
			return p
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

func fileExists(p string) bool { _, err := statShared(p); return err == nil }

// mapCwd reads a path inside the worktree as the same path in the main checkout (board paths are
// never rewritten: tenant folders and folder defaults stay main's). Anything else is unchanged.
func (l boardLoc) mapCwd(dir string) string {
	if l.wt == "" {
		return dir
	}
	if rel, ok := relWithin(realDir(dir), l.wt, runtime.GOOS); ok {
		return filepath.Join(l.main, rel)
	}
	return dir
}

// workLoc is the board findWorkspace resolved for this run (reset by resetRunState).
var workLoc boardLoc

// workDir is the cwd for tenant and folder-default selection: in a worktree, main's matching folder.
func workDir() string { return workLoc.mapCwd(mustGetwd()) }

// matchDir is a tenant or default folder as compared with workDir: in a worktree resolved, as workDir
// is (a /tmp folder stored on the board still matches /private/tmp), and a folder inside the worktree
// (stored by an older mw or with MW_WORKTREE=off) read as main's.
func matchDir(p string) string {
	if workLoc.wt == "" {
		return p
	}
	return workLoc.mapCwd(realDir(p))
}

// worktreeTenant refuses (exit 2) a command whose tenant, as the worktree's own board selects it (-t,
// MW_TENANT or the mapped cwd's folder; never the first-tenant fallback), is missing on the main board:
// its history would split.
func worktreeTenant(main *Workspace, args []string) error {
	if workLoc.wtPath == "" {
		return nil
	}
	w, err := loadScoped(workLoc.wtPath, false)
	if err != nil {
		return nil
	}
	o, _, err := options(args[1:])
	if err != nil || value(o, "tenant", os.Getenv("MW_TENANT")) == "" && value(o, "session", os.Getenv("MW_SESSION")) != "" {
		return nil // a session's tenant is resolved on main, where sessions live
	}
	slug := value(o, "tenant", os.Getenv("MW_TENANT"))
	if slug == "" {
		cwd, _ := filepath.Abs(workDir())
		t, err := tenantByFolder(w, cwd)
		if t == nil || err != nil {
			return nil
		}
		slug = t.Slug
	} else if _, err := tenantBySlug(w, slug); err != nil {
		return nil // unknown here too: main reports it as before
	}
	if _, err := tenantBySlug(main, slug); err != nil {
		return &exitError{code: 2, msg: fmt.Sprintf("tenant %s is not on the main board; run mw tenant add in the main checkout", slug)}
	}
	return nil
}

// wroteTenants are the tenant files this run saved (the worktree sync copies them; reset by resetRunState).
var wroteTenants = map[string]bool{}

// syncWorktree copies main's board into the worktree's after a command, reads included. A failure is a
// warning only: the command's exit code stands and the next command heals the copy.
func syncWorktree(loc boardLoc) {
	if err := syncBoard(loc.path, loc.wtPath, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "mw: worktree board not synced: %v\n", err)
	}
}

// syncBoard byte-copies main's index and every tenant file the worktree holds or this run wrote into
// the worktree board. It runs after main's locks are released, under the worktree's own locks, and
// re-reads main's files (never the command's in-memory board): the last process to sync writes the
// newest state. A worktree tenant file that changed since the last copy (an older mw, MW_WORKTREE=off,
// a git checkout) is never overwritten; it is reported on errw instead.
func syncBoard(main, wt string, errw io.Writer) error {
	want := map[string]bool{}
	for s := range wroteTenants {
		want[s] = true
	}
	ents, err := os.ReadDir(filepath.Join(filepath.Dir(wt), storeDir))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, e := range ents {
		if s, ok := strings.CutSuffix(e.Name(), ".json"); ok && validID(s) {
			want[s] = true
		}
	}
	slugs := make([]string, 0, len(want))
	for s := range want {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)
	unlock, err := lockSet(wt, true, slugs)
	if err != nil {
		return err
	}
	defer unlock()
	ib, err := readIndexBytes(main)
	if err != nil {
		return err
	}
	var ix indexDoc
	if err := json.Unmarshal(ib, &ix); err != nil {
		return fmt.Errorf("read %s: %w", main, err)
	}
	onMain := map[string]bool{}
	for _, t := range ix.Tenants {
		onMain[t.Slug] = true
	}
	keepIndex := false
	for _, slug := range slugs {
		mb, err := readFileShared(tenantPath(main, slug))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		missing := err != nil
		wb, err := readFileShared(tenantPath(wt, slug))
		if err == nil {
			if bytes.Equal(mb, wb) {
				recordCopy(wt, slug, wb)
				continue
			}
			if !lastCopy(main, wt, slug, mb, wb) {
				ab, _ := readFileShared(archivePath(main, slug))
				n, _ := unsharedEvents(wb, mb, ab)
				fmt.Fprintf(errw, "mw: worktree board has %d events not on main (%s); not synced\n", n, slug)
				keepIndex = keepIndex || !onMain[slug]
				continue
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if missing {
			continue
		}
		if err := writeFileAtomic(tenantPath(wt, slug), mb); err != nil {
			return err
		}
		recordCopy(wt, slug, mb)
	}
	if keepIndex { // main's index would drop a tenant whose events are only here
		return nil
	}
	if wb, err := readFileShared(wt); err == nil && bytes.Equal(wb, ib) {
		return nil
	}
	return writeFileAtomic(wt, ib)
}

// copyPath holds the sha256 of the bytes last copied into a worktree tenant file. It sits beside the
// locks (the .git/info/exclude pattern .miniwaves/*.lock* keeps it out of commits).
func copyPath(wt, slug string) string { return storePath(wt, slug+".lock.copied") }

func hexSum(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

// recordCopy remembers b as the last copy of the tenant file (best effort: a missing record only
// means the next sync falls back to the event check).
func recordCopy(wt, slug string, b []byte) {
	h := hexSum(b)
	if old, err := readFileShared(copyPath(wt, slug)); err == nil && string(old) == h {
		return
	}
	_ = writeFileAtomic(copyPath(wt, slug), []byte(h))
}

// lastCopy reports whether the worktree tenant file wb is still the copy this worktree last made, so
// overwriting it loses nothing. Without a record (the first sync of a worktree, or a board from an
// older mw) a file whose events main all has counts as a copy.
func lastCopy(main, wt, slug string, mb, wb []byte) bool {
	if h, err := readFileShared(copyPath(wt, slug)); err == nil {
		return string(h) == hexSum(wb)
	}
	ab, _ := readFileShared(archivePath(main, slug))
	n, err := unsharedEvents(wb, mb, ab)
	return err == nil && n == 0
}

// unsharedEvents counts the events of a worktree tenant file that main's tenant and archive files lack
// (an event is its item id, time, action and actor).
func unsharedEvents(wtb, mainb, archiveb []byte) (int, error) {
	var w tenantDoc
	if err := json.Unmarshal(wtb, &w); err != nil {
		return 0, err
	}
	seen := map[string]bool{}
	add := func(items []Item) {
		for _, it := range items {
			for _, e := range it.Events {
				seen[it.ID+"\x00"+e.At+"\x00"+e.Action+"\x00"+e.By] = true
			}
		}
	}
	var m tenantDoc
	if len(mainb) > 0 && json.Unmarshal(mainb, &m) == nil {
		add(m.Items)
	}
	var a archiveDoc
	if len(archiveb) > 0 && json.Unmarshal(archiveb, &a) == nil {
		add(a.Items)
	}
	n := 0
	for _, it := range w.Items {
		for _, e := range it.Events {
			if !seen[it.ID+"\x00"+e.At+"\x00"+e.Action+"\x00"+e.By] {
				n++
			}
		}
	}
	return n, nil
}
