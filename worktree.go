package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
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

// matchDir is a tenant or default folder as compared with workDir: resolved in a worktree, where
// workDir is (a /tmp folder stored on the board still matches /private/tmp).
func matchDir(p string) string {
	if workLoc.wt == "" {
		return p
	}
	return realDir(p)
}

// worktreeTenant refuses (exit 2) a command whose tenant, as the worktree's own board selects it from
// the real cwd, is missing on the main board: its history would split between the two.
func worktreeTenant(main *Workspace, args []string) error {
	if workLoc.wtPath == "" {
		return nil
	}
	w, err := loadScoped(workLoc.wtPath, false)
	if err != nil {
		return nil
	}
	o, _, err := options(args[1:])
	if err != nil {
		return nil
	}
	saved := workLoc
	workLoc = boardLoc{}
	t, err := selectTenantAny(w, o)
	workLoc = saved
	if err != nil {
		return nil
	}
	if _, err := tenantBySlug(main, t.Slug); err != nil {
		return &exitError{code: 2, msg: fmt.Sprintf("tenant %s is not on the main board; run mw tenant add in the main checkout", t.Slug)}
	}
	return nil
}
