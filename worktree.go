package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
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
	warn     string // one MW_MAIN note for stderr (findWorkspace prints it; the status line and hook stay quiet)
}

// worktreeOff is MW_WORKTREE=off: the nearest board, worktree or not (the behaviour before 0.3.16).
func worktreeOff() bool { return strings.EqualFold(strings.TrimSpace(os.Getenv("MW_WORKTREE")), "off") }

// boardPath is the one resolver every board lookup goes through. Outside a linked worktree (or a
// clone run with MW_MAIN) it is the nearest board walking up from dir. In one, with R the nearest
// board folder from dir up to the worktree root: main/R's board when both wt/R and main/R have one
// (write-through), wt/R's board alone when main/R has none, and with no board in the worktree (an
// untracked board) the board nearest to main/X for dir wt/X.
func boardPath(dir string) boardLoc {
	local := nearestBoard(dir)
	if worktreeOff() {
		return boardLoc{path: local}
	}
	wt, main, ok := worktreeOf(dir)
	warn := ""
	if env, set := os.LookupEnv("MW_MAIN"); set { // MW_MAIN overrides the link; set but empty ignores it
		if env != "" && !ok {
			wt, main, ok, warn = cloneMain(dir, env, "MW_MAIN "+env)
			if ok && !sameBoardAs(dir, local, wt, main) {
				ok, warn = false, "MW_MAIN ignored; "+wt+" is not a clone of it"
			}
		} else if env != "" {
			if m, err := filepath.EvalSymlinks(env); err != nil || !pathEqual(m, main, runtime.GOOS) {
				warn = "MW_MAIN ignored; worktree of " + main
			}
		}
	} else if link := linkOf(dir); link != "" && !ok {
		wt, main, ok, warn = cloneMain(dir, link, ".git/mw-main "+link)
	}
	loc := boardLoc{path: local}
	if ok {
		loc = boardIn(dir, local, wt, main)
	}
	loc.warn = warn
	return loc
}

// cloneMain is MW_MAIN (or the mw link) for a separate clone or any other checkout: the checkout
// holding dir plays the worktree and env the main checkout. env must be absolute, exist, hold a board
// for dir and not be the checkout itself (that is main: silent); otherwise warn, which starts with
// label, names why it is ignored.
func cloneMain(dir, env, label string) (wt, main string, ok bool, warn string) {
	root, found := checkoutRoot(dir)
	if !found {
		return "", "", false, ""
	}
	if !filepath.IsAbs(env) {
		return "", "", false, label + " ignored: not an absolute path"
	}
	m, err := filepath.EvalSymlinks(env)
	if fi, serr := os.Stat(m); err != nil || serr != nil || !fi.IsDir() {
		return "", "", false, label + " ignored: not found"
	}
	if pathEqual(m, root, runtime.GOOS) {
		return "", "", false, ""
	}
	rel, _ := relWithin(realDir(dir), root, runtime.GOOS)
	for r := rel; ; r = filepath.Dir(r) {
		if fileExists(filepath.Join(m, r, dataFile)) {
			return root, m, true, ""
		}
		if r == "." {
			return "", "", false, label + " ignored: no board there"
		}
	}
}

// sameBoardAs reports whether the checkout wt is a copy of main for MW_MAIN: its board (local, the nearest
// one from dir) is inside wt and has the name of main's board in the same folder, or mw link already
// links wt to main. An unrelated repo that happens to run with MW_MAIN set keeps its own board.
func sameBoardAs(dir, local, wt, main string) bool {
	if l, err := filepath.EvalSymlinks(linkOf(dir)); err == nil && pathEqual(l, main, runtime.GOOS) {
		return true
	}
	rel, ok := relWithin(filepath.Dir(local), wt, runtime.GOOS)
	if !ok || local == "" {
		return false
	}
	name := func(p string) string {
		var ix indexDoc
		if b, err := readIndexBytes(p); err != nil || json.Unmarshal(b, &ix) != nil {
			return "\x00" + p // unreadable: never equal
		}
		return ix.Name
	}
	return name(local) == name(filepath.Join(main, rel, dataFile))
}

// gitDirs reads the checkout at root without running git: its git dir (root/.git, or the gitdir of a
// .git file: a worktree, a submodule, a --separate-git-dir clone) and the common dir that holds
// info/exclude. linked is a linked worktree (its git dir has a commondir file).
func gitDirs(root string) (gd, common string, linked, ok bool) {
	g := filepath.Join(root, ".git")
	fi, err := os.Lstat(g)
	if err != nil {
		return "", "", false, false
	}
	if fi.IsDir() {
		return g, g, false, true
	}
	b, err := readFileShared(g)
	if err != nil {
		return "", "", false, false
	}
	ref, found := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir:")
	if !found {
		return "", "", false, false
	}
	gd = gitPath(root, strings.TrimSpace(ref), runtime.GOOS)
	if c, err := readFileShared(filepath.Join(gd, "commondir")); err == nil {
		return gd, gitPath(gd, strings.TrimSpace(string(c)), runtime.GOOS), true, true
	}
	return gd, gd, false, true
}

// excludeLocks adds the lock artifacts, unanchored, to the info/exclude of the checkout at root (its
// common dir), so a clone or a worktree with a board below the repo root never commits a lock or a
// .lock.copied record. A plain idempotent file write; mw init's anchored patterns stay.
func excludeLocks(root string) {
	if _, common, _, ok := gitDirs(root); ok {
		lines := make([]string, len(lockArtifacts))
		for i, l := range lockArtifacts {
			lines[i] = "**/" + l
		}
		appendExclude(filepath.Join(common, "info", "exclude"), lines)
	}
}

// linkFile is where mw link keeps the main checkout of a clone: one line, mw-main in its git dir
// (.git/mw-main in a plain clone); "" in a linked worktree or outside a checkout.
func linkFile(root string) string {
	gd, _, linked, ok := gitDirs(root)
	if !ok || linked {
		return ""
	}
	return filepath.Join(gd, "mw-main")
}

// linkOf is the main checkout mw link recorded for the clone holding dir, or "" (none, or a worktree).
func linkOf(dir string) string {
	root, ok := checkoutRoot(dir)
	if !ok || linkFile(root) == "" {
		return ""
	}
	b, err := os.ReadFile(linkFile(root))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// linkCommand is mw link [MAIN_DIR|-] [-j]: show, set or remove the clone's main checkout. Local only:
// it validates like MW_MAIN, writes .git/mw-main and never touches a board or runs git.
func linkCommand(args []string) error {
	asJSON, arg := false, ""
	for _, a := range args {
		switch {
		case a == "-j" || a == "--json":
			asJSON = true
		case a == "-" && arg == "":
			arg = a
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown option %s", a)
		case arg == "":
			arg = a
		default:
			return errors.New("usage: mw link [MAIN_DIR|-] [-j]")
		}
	}
	cwd := mustGetwd()
	root, ok := checkoutRoot(cwd)
	if !ok {
		return errors.New("not inside a git checkout")
	}
	file := linkFile(root)
	if _, _, linked, _ := gitDirs(root); linked {
		return &exitError{code: 2, msg: "worktree already shares the main board"}
	} else if file == "" {
		return errors.New("cannot read " + filepath.Join(root, ".git"))
	}
	switch arg {
	case "":
		m := linkOf(cwd)
		if asJSON {
			b, _ := json.Marshal(map[string]string{"main": m})
			fmt.Println(string(b))
		} else if m == "" {
			fmt.Println("not linked")
		} else {
			fmt.Println(m)
		}
		return nil
	case "-":
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		return err
	}
	_, m, ok, warn := cloneMain(cwd, abs, abs)
	if !ok {
		if warn == "" {
			warn = abs + " is this checkout"
		}
		return errors.New(strings.Replace(warn, " ignored: ", ": ", 1))
	}
	if err := os.WriteFile(file, []byte(m+"\n"), 0o644); err != nil {
		return err
	}
	excludeLocks(root)
	if asJSON {
		b, _ := json.Marshal(map[string]string{"main": m})
		fmt.Println(string(b))
	} else {
		fmt.Println("linked to " + m)
	}
	return nil
}

// checkoutRoot is the folder of the first .git (folder or file) walking up from dir, symlinks resolved.
func checkoutRoot(dir string) (string, bool) {
	d := realDir(dir)
	for {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d, true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", false
		}
		d = parent
	}
}

// boardIn is boardPath's choice for dir inside the worktree (or clone) wt of the main checkout main.
func boardIn(dir, local, wt, main string) boardLoc {
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
	excludeLocks(loc.wt)
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
				// Changed since the last copy, but nothing of its own (a git checkout, pull, stash or
				// branch switch brought an older copy back): overwrite. Otherwise keep it and warn.
				if n, err := unsharedEvents(wb, mb, ab); err != nil || n > 0 || !sessionsKnown(wb, mb) {
					fmt.Fprintf(errw, "mw: worktree board has %d events not on main (%s); not synced (check, then delete %s/%s.lock.copied to resync)\n", n, slug, storeDir, slug)
					keepIndex = keepIndex || !onMain[slug]
					continue
				}
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
// locks (the info/exclude patterns excludeLocks adds keep it out of commits).
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

// sessionsKnown reports whether every session of the worktree tenant file wtb is on main (same id and
// tenant) and equal there or older (main's record was updated later).
func sessionsKnown(wtb, mainb []byte) bool {
	var w, m tenantDoc
	if json.Unmarshal(wtb, &w) != nil || (len(mainb) > 0 && json.Unmarshal(mainb, &m) != nil) {
		return false
	}
	on := map[string]Session{}
	for _, s := range m.Sessions {
		on[s.ID+"\x00"+s.Tenant] = s
	}
	for _, s := range w.Sessions {
		ms, found := on[s.ID+"\x00"+s.Tenant]
		if !found || (s != ms && s.Updated >= ms.Updated) {
			return false
		}
	}
	return true
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
