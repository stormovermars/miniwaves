package main

import (
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
