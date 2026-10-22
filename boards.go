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

// The --depth range is the discover.depth range of userkeys.go.
const (
	boardsDepthMin  = 1
	boardsDepthMax  = 6
	boardsIndexMax  = 1 << 20 // a board index larger than this is an error entry, never read
	boardsDefaultDp = 4       // discover.depth default when the user config has none
)

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

// boardsProtected lists the macOS folders behind TCC prompts (a var so tests can use a fake home).
var boardsProtected = func() []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	var out []string
	// iCloud Drive (~/Library/Mobile Documents) lies inside ~/Library
	for _, n := range []string{"Documents", "Desktop", "Downloads", "Library", "Movies", "Music", "Pictures"} {
		out = append(out, filepath.Join(home, n))
	}
	return out
}

// inside reports whether dir is p or below it.
func inside(dir, p string) bool {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" { // case-insensitive file systems
		dir, p = strings.ToLower(dir), strings.ToLower(p)
	}
	return dir == p || strings.HasPrefix(dir, p+string(filepath.Separator))
}

func insideAny(dir string, ps []string) bool {
	for _, p := range ps {
		if inside(dir, p) {
			return true
		}
	}
	return false
}

// boardsAppData lists the Windows AppData folders (empty elsewhere): never scanned.
var boardsAppData = func() []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	var out []string
	for _, e := range []string{"APPDATA", "LOCALAPPDATA"} {
		if v := os.Getenv(e); v != "" {
			out = append(out, filepath.Clean(v))
		}
	}
	return out
}

// boardsDefaultDepth is discover.depth from the user config, else 4.
func boardsDefaultDepth() int {
	if c, _, _, err := readUserConfig(); err == nil {
		if n, err := strconv.Atoi(c.Settings["discover.depth"]); err == nil && n >= boardsDepthMin && n <= boardsDepthMax {
			return n
		}
	}
	return boardsDefaultDp
}

// readBoardIndex reads one index file and nothing else. A parse failure is reported in Error.
func readBoardIndex(dir string) boardInfo {
	p := filepath.Join(dir, ".miniwaves.json")
	b := boardInfo{Dir: dir, Path: p, Tenants: []boardTenant{}}
	var data []byte
	var err error
	if fi, serr := os.Stat(p); serr == nil && fi.Size() > boardsIndexMax {
		err = fmt.Errorf("%s is %d bytes: larger than the %d byte limit, not read", p, fi.Size(), boardsIndexMax)
	} else if data, err = readFileShared(p); err == nil && len(data) > boardsIndexMax {
		err = fmt.Errorf("%s is larger than the %d byte limit, not read", p, boardsIndexMax)
	}
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
	appData := boardsAppData()
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
			if runtime.GOOS == "windows" && strings.EqualFold(n, "AppData") {
				continue
			}
			sub := filepath.Join(dir, n)
			skip := false
			for _, p := range protected {
				if sub == p {
					skip = true
				}
			}
			for _, p := range appData {
				if strings.EqualFold(sub, p) || inside(sub, p) {
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
		// never stat an ancestor inside a TCC-protected folder (it would raise a permission prompt),
		// unless the start folder is itself inside one: then the user asked for it
		userAsked := insideAny(root, protected)
		for d := filepath.Dir(root); ; d = filepath.Dir(d) {
			if userAsked || !insideAny(d, protected) {
				if hasIndex(d) {
					add(d)
					break
				}
			}
			if filepath.Dir(d) == d {
				break
			}
		}
	}
	sort.SliceStable(res.Boards, func(i, j int) bool { return res.Boards[i].Dir < res.Boards[j].Dir })
	return res
}

func boardsCommand(args []string) error {
	depth, up, asJSON, dir := 0, false, false, ""
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
			if err != nil || n < boardsDepthMin || n > boardsDepthMax {
				return fmt.Errorf("--depth must be %d..%d (the discover.depth range)", boardsDepthMin, boardsDepthMax)
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
	if depth == 0 {
		depth = boardsDefaultDepth()
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
