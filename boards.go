package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// mw boards: read-only discovery of boards below (and optionally above) a folder, so pane
// plugins need no setup. It reads only each board's index file.

var (
	boardsMax       = 200
	boardsTimeLimit = 5 * time.Second
)

const boardsDepthMax = 8

type boardTenant struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Archived bool   `json:"archived"`
}

type boardInfo struct {
	Dir     string        `json:"dir"`
	Path    string        `json:"path"`
	Name    string        `json:"name"`
	Schema  int           `json:"schema"`
	Tenants []boardTenant `json:"tenants"`
	Error   string        `json:"error,omitempty"`
}

type boardsResult struct {
	Root      string      `json:"root"`
	Boards    []boardInfo `json:"boards"`
	Truncated bool        `json:"truncated"`
	Reason    string      `json:"reason"`
}

var boardsSkipNames = map[string]bool{"node_modules": true, "build": true, "dist": true, "vendor": true}

func boardsProtected() []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	var out []string
	for _, n := range []string{"Documents", "Desktop", "Downloads", "Library", "Movies", "Music", "Pictures"} {
		out = append(out, filepath.Join(home, n))
	}
	return out
}

// readBoardIndex reads one index file and nothing else. A parse failure is reported in Error.
func readBoardIndex(dir string) boardInfo {
	p := filepath.Join(dir, ".miniwaves.json")
	b := boardInfo{Dir: dir, Path: p, Tenants: []boardTenant{}}
	data, err := readFileShared(p)
	if err == nil {
		var doc struct {
			Version int           `json:"version"`
			Name    string        `json:"name"`
			Tenants []boardTenant `json:"tenants"`
		}
		if err = json.Unmarshal(data, &doc); err == nil {
			b.Schema, b.Name = doc.Version, doc.Name
			if doc.Tenants != nil {
				b.Tenants = doc.Tenants
			}
			return b
		}
	}
	b.Error = err.Error()
	return b
}

func hasIndex(dir string) bool {
	fi, err := os.Lstat(filepath.Join(dir, ".miniwaves.json"))
	return err == nil && fi.Mode().IsRegular()
}

func discoverBoards(root string, depth int, up bool) boardsResult {
	res := boardsResult{Root: root, Boards: []boardInfo{}}
	deadline := time.Now().Add(boardsTimeLimit)
	protected := boardsProtected()
	seen := map[string]bool{}
	stop := false
	add := func(dir string) {
		if seen[dir] {
			return
		}
		if len(res.Boards) >= boardsMax {
			res.Truncated, res.Reason, stop = true, fmt.Sprintf("board cap %d reached", boardsMax), true
			return
		}
		seen[dir] = true
		res.Boards = append(res.Boards, readBoardIndex(dir))
	}
	var walk func(dir string, level int)
	walk = func(dir string, level int) {
		if stop {
			return
		}
		if time.Now().After(deadline) {
			res.Truncated, res.Reason, stop = true, "time limit "+boardsTimeLimit.String()+" reached", true
			return
		}
		if hasIndex(dir) {
			add(dir)
			if stop {
				return
			}
		}
		if level >= depth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			n := e.Name()
			if !e.IsDir() || e.Type()&os.ModeSymlink != 0 || strings.HasPrefix(n, ".") || boardsSkipNames[n] {
				continue
			}
			sub := filepath.Join(dir, n)
			skip := false
			for _, p := range protected {
				if sub == p {
					skip = true
				}
			}
			if !skip {
				walk(sub, level+1)
			}
		}
	}
	walk(root, 0)
	if up && !stop {
		for d := filepath.Dir(root); d != root; root, d = d, filepath.Dir(d) {
			if hasIndex(d) {
				add(d)
				break
			}
		}
	}
	sort.SliceStable(res.Boards, func(i, j int) bool { return res.Boards[i].Dir < res.Boards[j].Dir })
	return res
}

func boardsCommand(args []string) error {
	depth, up, asJSON, dir := 3, false, false, ""
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--up":
			up = true
		case a == "-j" || a == "--json":
			asJSON = true
		case a == "--depth":
			if i+1 >= len(args) {
				return errors.New("--depth needs a value")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 0 || n > boardsDepthMax {
				return fmt.Errorf("--depth must be 0..%d", boardsDepthMax)
			}
			depth = n
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown option %s", a)
		case dir == "":
			dir = a
		default:
			return errors.New("usage: mw boards [DIR] [--depth N] [--up] [-j]")
		}
	}
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s is not a folder", abs)
	}
	res := discoverBoards(abs, depth, up)
	if asJSON {
		out, err := json.Marshal(res)
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	}
	for _, b := range res.Boards {
		if b.Error != "" {
			fmt.Printf("%s  error: %s\n", b.Dir, b.Error)
			continue
		}
		var act []string
		arch := 0
		for _, t := range b.Tenants {
			if t.Archived {
				arch++
			} else {
				act = append(act, t.Slug)
			}
		}
		line := b.Dir + "  " + b.Name + "  tenants: " + strings.Join(act, ", ")
		if arch > 0 {
			line += fmt.Sprintf(" (+%d archived)", arch)
		}
		fmt.Println(line)
	}
	if len(res.Boards) == 0 {
		fmt.Println("no boards found")
	}
	if res.Truncated {
		fmt.Println("truncated: " + res.Reason)
	}
	return nil
}
