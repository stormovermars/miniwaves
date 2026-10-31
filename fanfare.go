package main

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Fanfare: a short clip when a wave closes, a longer finale when a tenant has nothing left.
// The collection lives outside the board (default ~/fanfare-finale-songs): one folder per pair
// with fanfare.<ext>, finale.<ext> and LICENSE.txt. Playback is a detached child process and
// never fails a board command.

const (
	defaultFanfareDir = "~/fanfare-finale-songs"
	manifestName      = "manifest.json"
)

// FanfareConfig is the "fanfare" object of .miniwaves.json. Missing or zero fields take the defaults.
type FanfareConfig struct {
	Enabled          *bool               `json:"enabled,omitempty"` // nil = the board does not decide (reads as off; the user layer may switch it)
	Dir              string              `json:"dir"`
	MilestoneSeconds int                 `json:"milestone_seconds"`
	FinaleSeconds    int                 `json:"finale_seconds"`
	Volume           float64             `json:"volume"`
	StopDialog       *bool               `json:"stop_dialog,omitempty"` // finale Stop dialog; nil = true
	Sets             map[string][]string `json:"sets,omitempty"`        // named sets of pair folders (fanfare_sets.go)
	Use              string              `json:"use,omitempty"`         // active set; "" or "all" = every playable pair
}

// fanfareBoardOffHint is shown by mw fanfare when an explicit board "enabled": false beats the user's own
// fanfare.enabled=true. Boards written before 0.3.4 stored enabled:false for "never set" (a plain bool), which
// reads the same as a deliberate off, so mw cannot clear it on its own (TSK-058): it says so instead.
const fanfareBoardOffHint = "board says off; mw fanfare set enabled=true to follow your setting"

// fanfareBoardSilencesUser is true when the user layer says on and the board's explicit off wins.
func fanfareBoardSilencesUser(w *Workspace) bool {
	if !boardFanfareOff(w) {
		return false
	}
	u, _, _, err := readUserConfig()
	return err == nil && u.Fanfare.Enabled != nil && *u.Fanfare.Enabled
}

func fanfareCfg(w *Workspace) FanfareConfig {
	c := FanfareConfig{}
	if w != nil && w.Fanfare != nil {
		c = *w.Fanfare
	}
	if c.Dir == "" || strings.HasPrefix(c.Dir, "-") { // a dash-leading dir would reach the player as an option
		c.Dir = defaultFanfareDir
	}
	// the same ranges as `mw fanfare set`; hand-edited values are clamped
	if c.MilestoneSeconds <= 0 {
		c.MilestoneSeconds = 15
	}
	if c.FinaleSeconds <= 0 {
		c.FinaleSeconds = 60
	}
	c.MilestoneSeconds = min(c.MilestoneSeconds, 3600)
	c.FinaleSeconds = min(c.FinaleSeconds, 3600)
	if !(c.Volume > 0) {
		c.Volume = 1
	}
	c.Volume = math.Min(c.Volume, 4)
	if c.StopDialog == nil {
		t := true
		c.StopDialog = &t
	}
	return c
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[1:])
		}
	}
	return p
}

// ---- collection ----

type fanfareTrack struct {
	Kind, Ext, URL, Source, License, Attribution, Title string
	SHA256                                              string // optional pin; a download that differs fails
}

type fanfarePair struct {
	Name, Label string
	Tracks      []fanfareTrack
}

// byoSlots are pair folders the user fills from copies they own; mw never downloads them.
var byoSlots = []struct{ Name, Label, Fanfare, Finale string }{
	{"star-wars", "Star Wars", "Main Title", "Throne Room and End Title"},
	{"rocky", "Rocky", "Fanfare for Rocky (Gonna Fly Now intro)", "Gonna Fly Now"},
	{"indiana-jones", "Indiana Jones", "Raiders March (opening)", "Raiders March (full)"},
	{"top-gun", "Top Gun", "Top Gun Anthem (intro)", "Top Gun Anthem (full)"},
	{"chariots-of-fire", "Chariots of Fire", "Titles (intro)", "Titles (full)"},
	{"the-good-the-bad-and-the-ugly", "The Good, the Bad and the Ugly", "Main Title (intro)", "The Ecstasy of Gold"},
}

func byoReadme(label, fanfare, finale string) string {
	return "Bring your own: " + label + "\n\nmw never downloads this pair (the music is copyrighted). Drop tracks from your own purchased copy here:\n\n" +
		"  fanfare.<ext>  e.g. \"" + fanfare + "\" (played for the first milestone_seconds when a wave closes)\n" +
		"  finale.<ext>   e.g. \"" + finale + "\" (played for the first finale_seconds when a tenant is finished)\n\n" +
		"Any of mp3, m4a, aac, wav, flac, aiff, ogg, opus works (macOS afplay plays mp3, m4a, aac, wav, flac, aiff; others need ffplay). Until both files exist this pair is skipped.\n"
}

var audioExts = map[string]bool{"mp3": true, "m4a": true, "aac": true, "wav": true, "flac": true, "aif": true, "aiff": true, "caf": true, "ogg": true, "oga": true, "opus": true, "webm": true}

type pairInfo struct {
	Name    string `json:"name" toon:"name"`
	State   string `json:"state" toon:"state"`
	Label   string `json:"label,omitempty" toon:"label,omitempty"`
	Fanfare string `json:"fanfare,omitempty" toon:"fanfare,omitempty"`
	Finale  string `json:"finale,omitempty" toon:"finale,omitempty"`
	License string `json:"license,omitempty" toon:"license,omitempty"`
}

type manifestTrack struct {
	Kind        string `json:"kind"`
	File        string `json:"file"`
	URL         string `json:"url,omitempty"`
	Source      string `json:"source,omitempty"`
	License     string `json:"license,omitempty"`
	Attribution string `json:"attribution,omitempty"`
	Title       string `json:"title,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

type manifestPair struct {
	Name   string          `json:"name"`
	Label  string          `json:"label,omitempty"`
	Source string          `json:"source"` // commons | user
	Tracks []manifestTrack `json:"tracks,omitempty"`
}

type fanfareManifest struct {
	Pairs []manifestPair `json:"pairs"`
}

func readManifest(dir string) fanfareManifest {
	var m fanfareManifest
	if b, err := os.ReadFile(filepath.Join(dir, manifestName)); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func (m fanfareManifest) pair(name string) *manifestPair {
	for i := range m.Pairs {
		if m.Pairs[i].Name == name {
			return &m.Pairs[i]
		}
	}
	return nil
}

func findTrack(folder, kind string) string {
	es, err := os.ReadDir(folder)
	if err != nil {
		return ""
	}
	for _, e := range es {
		n := e.Name()
		if e.IsDir() || strings.HasPrefix(n, ".") {
			continue
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(n)), ".")
		if strings.EqualFold(strings.TrimSuffix(n, filepath.Ext(n)), kind) && audioExts[ext] {
			return filepath.Join(folder, n)
		}
	}
	return ""
}

func licenseOf(folder string) string {
	b, err := os.ReadFile(filepath.Join(folder, "LICENSE.txt"))
	if err != nil {
		return ""
	}
	var ls []string
	for _, l := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(l, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "license") {
			if v = strings.TrimSpace(v); v != "" && !contains(ls, v) {
				ls = append(ls, v)
			}
		}
	}
	return strings.Join(ls, ", ")
}

// scanPairs lists the pair folders of dir: "ready" when both tracks exist and the player here can
// play both, "unplayable here" when both exist but not, "incomplete" with one track, else "empty (bring your own)".
func scanPairs(dir string) []pairInfo {
	es, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	m := readManifest(dir)
	var out []pairInfo
	for _, e := range es {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		f := filepath.Join(dir, e.Name())
		p := pairInfo{Name: e.Name(), Fanfare: findTrack(f, "fanfare"), Finale: findTrack(f, "finale"), License: licenseOf(f)}
		if mp := m.pair(e.Name()); mp != nil {
			p.Label = mp.Label
		}
		switch {
		case p.Fanfare != "" && p.Finale != "":
			p.State = "ready"
			if !canPlay(p.Fanfare) || !canPlay(p.Finale) {
				p.State = "unplayable here"
			}
		case p.Fanfare != "" || p.Finale != "":
			p.State = "incomplete"
		default:
			p.State = "empty (bring your own)"
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// lookPath finds players; tests replace it.
var lookPath = exec.LookPath

// canPlay reports whether a player available here can play file's format.
func canPlay(file string) bool {
	_, _, err := playerCommand(runtime.GOOS, file, 1, 1, lookPath)
	return err == nil
}

// pickPair chooses a random ready pair (pairs this machine cannot play are skipped); pick(n) returns an index in [0,n).
func pickPair(dir string, pick func(int) int) (*pairInfo, error) {
	var ready []pairInfo
	for _, p := range scanPairs(dir) {
		if p.State == "ready" {
			ready = append(ready, p)
		}
	}
	if len(ready) == 0 {
		return nil, fmt.Errorf("no playable fanfare pairs in %s (mw fanfare fetch downloads the public-domain collection; mw fanfare list shows what is unplayable here)", dir)
	}
	p := ready[pick(len(ready))]
	return &p, nil
}

// ---- playback ----

var afplayExts = map[string]bool{"mp3": true, "m4a": true, "aac": true, "wav": true, "flac": true, "aif": true, "aiff": true, "caf": true}

func pct(v float64, max int) int {
	n := int(math.Round(v * 100))
	if n > max {
		n = max
	}
	return n
}

// playerCommand builds the player command for goos. MW_FANFARE_PLAYER (a stub script, used by
// tests) is called as: STUB FILE SECONDS VOLUME.
func playerCommand(goos, file string, secs int, vol float64, look func(string) (string, error)) (string, []string, error) {
	if stub := os.Getenv("MW_FANFARE_PLAYER"); stub != "" {
		return stub, []string{file, strconv.Itoa(secs), strconv.FormatFloat(vol, 'g', -1, 64)}, nil
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(file)), ".")
	has := func(n string) bool { _, err := look(n); return err == nil }
	s := strconv.Itoa(secs)
	ffplay := func() (string, []string) {
		a := []string{"-nodisp", "-autoexit", "-loglevel", "quiet", "-t", s}
		if vol != 1 {
			a = append(a, "-volume", strconv.Itoa(pct(vol, 100)))
		}
		return "ffplay", append(a, file)
	}
	mpv := func() (string, []string) {
		a := []string{"--no-video", "--really-quiet", "--length=" + s}
		if vol != 1 {
			a = append(a, "--volume="+strconv.Itoa(pct(vol, 130)))
		}
		return "mpv", append(a, file)
	}
	switch goos {
	case "darwin":
		if afplayExts[ext] && has("afplay") {
			a := []string{"-t", s}
			if vol != 1 {
				a = append(a, "-v", strconv.FormatFloat(vol, 'g', -1, 64))
			}
			return "afplay", append(a, file), nil
		}
		if has("ffplay") {
			n, a := ffplay()
			return n, a, nil
		}
		if has("mpv") {
			n, a := mpv()
			return n, a, nil
		}
		return "", nil, fmt.Errorf("afplay cannot play .%s and neither ffplay nor mpv is installed (brew install ffmpeg)", ext)
	case "linux":
		if has("ffplay") {
			n, a := ffplay()
			return n, a, nil
		}
		if has("mpv") {
			n, a := mpv()
			return n, a, nil
		}
		if has("paplay") && (ext == "wav" || ext == "ogg" || ext == "oga" || ext == "flac") { // no trim: plays the whole file
			a := []string{}
			if vol != 1 {
				a = append(a, "--volume="+strconv.Itoa(int(math.Round(math.Min(vol, 4)*65536))))
			}
			return "paplay", append(a, file), nil
		}
		return "", nil, fmt.Errorf("no player for .%s found (install ffmpeg or mpv)", ext)
	case "windows":
		if ext != "wav" {
			return "", nil, fmt.Errorf("on Windows only .wav plays (got .%s)", ext)
		}
		return "powershell", []string{"-NoProfile", "-c", "(New-Object Media.SoundPlayer '" + strings.ReplaceAll(file, "'", "''") + "').PlaySync()"}, nil
	}
	return "", nil, fmt.Errorf("fanfare playback is not supported on %s", goos)
}

// playFile starts the player detached, records its pid for `mw fanfare stop` and returns at once.
func playFile(dir, file string, secs int, vol float64) error {
	_, err := startPlayer(dir, file, secs, vol, "milestone")
	return err
}

// playFinale plays the finale and, when stopDialog is set, opens the Stop control beside it,
// telling why (reason), which song and for how long.
func playFinale(dir, file string, secs int, vol float64, stopDialog bool, reason string) error {
	pid, err := startPlayer(dir, file, secs, vol, "finale")
	if err != nil {
		return err
	}
	if stopDialog {
		startStopControl(dir, pid, secs, finaleMessage(reason, finaleSong(dir, file), secs), finaleIcon(dir, file))
	}
	return nil
}

// finaleMessage is the text of the Stop dialog: reason, song, duration. The track length is not
// read (no audio parsing), so there is no "(of M:SS)" part.
func finaleMessage(reason, song string, secs int) string {
	return reason + "\nSong: " + song + "\nPlaying " + strconv.Itoa(secs) + " s"
}

// finaleReason: why the finale plays for a finished tenant: slug (and name when different) and the done count.
func finaleReason(t *Tenant) string {
	done := 0
	for _, it := range t.Items {
		if it.Status == "done" {
			done++
		}
	}
	who := strconv.Quote(t.Slug)
	if t.Name != "" && t.Name != t.Slug {
		who += " (" + t.Name + ")"
	}
	noun := "items"
	if done == 1 {
		noun = "item"
	}
	r := fmt.Sprintf("Tenant %s finished \u2014 %d %s done", who, done, noun)
	if last := lastDone(t, 3); len(last) > 0 {
		r += "; last: " + strings.Join(last, ", ")
	}
	return r
}

// itemLabel is "ID title" with the title cut to 60 characters.
func itemLabel(it *Item) string { return it.ID + " " + compactText(it.Title, 60) }

// lastDone labels the n most recently finished items, newest first.
func lastDone(t *Tenant, n int) []string {
	var done []*Item
	for i := range t.Items {
		if t.Items[i].Status == "done" && t.Items[i].ID != "" {
			done = append(done, &t.Items[i])
		}
	}
	sort.SliceStable(done, func(a, b int) bool { return done[a].Updated > done[b].Updated })
	var out []string
	for i := 0; i < len(done) && i < n; i++ {
		out = append(out, itemLabel(done[i]))
	}
	return out
}

// milestoneReason is the text of the milestone notification.
func milestoneReason(t *Tenant, it *Item) string {
	return strings.ToUpper(t.Slug) + " " + itemLabel(it) + " \u2014 done"
}

// notifyCommand builds the non-blocking notification for goos; msg is data, passed only as an argument.
// MW_FANFARE_NOTIFY (test stub) is called as STUB MSG; MW_FANFARE_PLAYER alone disables notifications.
func notifyCommand(goos, msg string, look func(string) (string, error)) (string, []string, bool) {
	if stub := os.Getenv("MW_FANFARE_NOTIFY"); stub != "" {
		return stub, []string{msg}, true
	}
	if os.Getenv("MW_FANFARE_PLAYER") != "" {
		return "", nil, false
	}
	switch goos {
	case "darwin":
		return "osascript", []string{"-e", "on run argv", "-e", `display notification (item 1 of argv) with title "miniwaves"`, "-e", "end run", msg}, true
	case "windows":
		return "", nil, false
	}
	if _, err := look("notify-send"); err == nil {
		return "notify-send", []string{"miniwaves", msg}, true
	}
	return "", nil, false
}

func notifyMilestone(msg string) {
	if name, args, ok := notifyCommand(runtime.GOOS, msg, lookPath); ok {
		startDetached(name, args, nil)
	}
}

const finaleTestReason = "Test (mw fanfare test)"

// finaleSong names the finale track: the manifest title (plus a short attribution) when known,
// else the file name without extension and the pair folder.
func finaleSong(dir, file string) string {
	pair := filepath.Base(filepath.Dir(file))
	title, attr, label := "", "", ""
	if mp := readManifest(dir).pair(pair); mp != nil {
		label = mp.Label
		for _, tr := range mp.Tracks {
			if tr.Kind != "finale" {
				continue
			}
			title, attr = tr.Title, tr.Attribution
			if title == "" { // collections fetched before manifests carried titles
				for _, bp := range builtinPairs {
					for _, bt := range bp.Tracks {
						if bp.Name == pair && bt.Kind == "finale" {
							title = bt.Title
						}
					}
				}
			}
		}
	}
	if title == "" && label != "" { // BYO and older collections: the pair label names the film/piece
		title = label
		if attr != "" {
			if r := []rune(attr); len(r) > 80 {
				attr = string(r[:79]) + "\u2026"
			}
			return title + " \u2014 " + attr
		}
	}
	if title == "" {
		return strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)) + " (" + pair + ")"
	}
	if attr != "" && len([]rune(attr)) <= 50 && !strings.Contains(title, attr) {
		title += " \u2014 " + attr
	}
	return title
}

// stopControlCommand builds the command of the finale's Stop dialog: a dialog showing msg with a
// Stop button that times out after secs+2 seconds and kills the player (pid) itself. ok is false
// when this platform has no dialog. icon (a file path, may be "") is data too. msg is data: it travels only as an argument (or, on Windows,
// the MW_FANFARE_MSG variable in env) and never inside the script text. MW_FANFARE_PLAYER (test
// stub) disables it unless MW_FANFARE_STOP_CONTROL names a stub, called as: STUB PLAYERPID SECONDS MSG.
func stopControlCommand(goos string, playerPID, secs int, msg, icon string, look func(string) (string, error)) (name string, args, env []string, ok bool) {
	if stub := os.Getenv("MW_FANFARE_STOP_CONTROL"); stub != "" {
		return stub, []string{strconv.Itoa(playerPID), strconv.Itoa(secs), msg}, nil, true
	}
	if os.Getenv("MW_FANFARE_PLAYER") != "" {
		return "", nil, nil, false
	}
	n := strconv.Itoa(secs + 2)
	p := strconv.Itoa(playerPID)
	switch goos {
	case "darwin": // sh -c SCRIPT sh MSG PID SECONDS [ICON]
		ic, extra := ` with icon note`, []string(nil)
		if icon != "" {
			ic, extra = ` with icon (POSIX file (item 3 of argv))`, []string{icon}
		}
		script := `r=$(osascript -e 'on run argv' -e 'display dialog (item 1 of argv) with title "miniwaves"` + ic + ` buttons {"Stop"} default button 1 giving up after (item 2 of argv as integer)' -e 'end run' "$1" "$3" "$4")` +
			`; case "$r" in *"gave up:true"*) ;; *"button returned:Stop"*) kill -TERM -"$2" 2>/dev/null || kill -TERM "$2";; esac`
		return "sh", append([]string{"-c", script, "sh", msg, p, n}, extra...), nil, true
	case "windows": // the Popup cannot show a picture: flag 64 is the information icon
		script := `$r=(New-Object -ComObject WScript.Shell).Popup($env:MW_FANFARE_MSG + "` + "`n`n" + `Click OK to stop.",` + n + `,'miniwaves',64); if($r -eq 1){taskkill /PID ` + p + ` /T /F}`
		return "powershell", []string{"-NoProfile", "-c", script}, []string{"MW_FANFARE_MSG=" + msg}, true
	}
	if _, err := look("zenity"); err == nil {
		argv := []string{"-c", "", "sh", msg, p, n}
		flags := ""
		if icon != "" && zenityBodyIcon() { // older zenity has no body icon: nothing is added
			flags = ` --icon="$4"`
			argv = append(argv, icon)
		}
		argv[1] = `zenity --question --no-markup --title=miniwaves` + flags + ` --text="$1" --ok-label=Stop --cancel-label=Close --timeout="$3" && kill -TERM -"$2" 2>/dev/null || true`
		return "sh", argv, nil, true
	}
	if _, err := look("kdialog"); err == nil { // kdialog's --icon is only the title-bar icon: none is passed
		// kdialog has no timeout: a timer closes it, and the player is killed only on a click (exit 0), not by the timer (143).
		script := `kdialog --title miniwaves --msgbox "$1" --ok-label Stop & k=$!; (sleep "$3"; kill $k 2>/dev/null) & t=$!; wait $k; r=$?; kill $t 2>/dev/null; [ $r -eq 0 ] && kill -TERM -"$2" 2>/dev/null; true`
		return "sh", []string{"-c", script, "sh", msg, p, n}, nil, true
	}
	return "", nil, nil, false
}

// zenityBodyIcon reports whether zenity is 3.90 or newer (GTK4), the first with --icon=PATH, the
// picture beside the text; older zenity rejects the flag. A variable so tests need no zenity.
var zenityBodyIcon = func() bool {
	out, err := exec.Command("zenity", "--version").Output()
	if err != nil {
		return false
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), ".", 3)
	if len(parts) < 2 {
		return false
	}
	maj, e1 := strconv.Atoi(parts[0])
	min, e2 := strconv.Atoi(parts[1])
	return e1 == nil && e2 == nil && (maj > 3 || (maj == 3 && min >= 90))
}

//go:embed assets/fanfare-icon.png
var fanfareIconPNG []byte

// finaleIcon picks the picture of the Stop dialog: cover.png, cover.jpg or cover.icns in the
// played pair's folder, else mw's built-in icon written once to <dir>/.mw-fanfare-icon.png.
// "" (no picture) when the built-in cannot be written.
func finaleIcon(dir, file string) string {
	for _, n := range []string{"cover.png", "cover.jpg", "cover.icns"} {
		p := filepath.Join(filepath.Dir(file), n)
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return p
		}
	}
	p := filepath.Join(dir, ".mw-fanfare-icon.png")
	if cur, err := os.ReadFile(p); err == nil && bytes.Equal(cur, fanfareIconPNG) {
		return p
	}
	if os.MkdirAll(dir, 0o755) != nil || os.WriteFile(p, fanfareIconPNG, 0o644) != nil {
		return ""
	}
	return p
}

// startStopControl launches the Stop dialog detached and records it (best effort, never fails).
func startStopControl(dir string, playerPID, secs int, msg, icon string) {
	name, args, env, ok := stopControlCommand(runtime.GOOS, playerPID, secs, msg, icon, lookPath)
	if !ok {
		return
	}
	if pid, err := startDetached(name, args, env); err == nil {
		recordPlayer(dir, pid, "control")
	}
}

func startPlayer(dir, file string, secs int, vol float64, kind string) (int, error) {
	name, args, err := playerCommand(runtime.GOOS, file, secs, vol, lookPath)
	if err != nil {
		return 0, err
	}
	pid, err := startDetached(name, args, nil)
	if err != nil {
		return 0, err
	}
	recordPlayer(dir, pid, kind)
	return pid, nil
}

func startDetached(name string, args, env []string) (int, error) {
	cmd := exec.Command(name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	detach(cmd, true)
	if err := cmd.Start(); err != nil {
		if runtime.GOOS != "windows" {
			return 0, err
		}
		cmd = exec.Command(name, args...) // a job object may refuse the breakaway: retry with the plain flags
		if len(env) > 0 {
			cmd.Env = append(os.Environ(), env...)
		}
		detach(cmd, false)
		if err := cmd.Start(); err != nil {
			return 0, err
		}
	}
	pid := cmd.Process.Pid
	cmd.Process.Release()
	return pid, nil
}

// ---- players started by mw (mw fanfare stop) ----

type playerRec struct {
	PID   int    `json:"pid"`
	Stamp string `json:"stamp"`          // identifies the process so a reused pid is never killed
	Kind  string `json:"kind,omitempty"` // "milestone", "finale" or "control" (the Stop dialog)
}

func playersPath(dir string) string { return filepath.Join(dir, ".players.json") }

func readPlayers(dir string) []playerRec {
	var rs []playerRec
	if b, err := os.ReadFile(playersPath(dir)); err == nil {
		_ = json.Unmarshal(b, &rs)
	}
	return rs
}

func writePlayers(dir string, rs []playerRec) {
	if len(rs) == 0 {
		os.Remove(playersPath(dir))
		return
	}
	b, _ := json.Marshal(rs)
	_ = os.WriteFile(playersPath(dir), b, 0o644)
}

// recordPlayer remembers pid (best effort) and drops entries that are no longer running.
func recordPlayer(dir string, pid int, kind string) {
	stamp := procStamp(pid)
	if stamp == "" {
		return // already finished
	}
	var keep []playerRec
	for _, r := range readPlayers(dir) {
		if r.PID != pid && procStamp(r.PID) == r.Stamp {
			keep = append(keep, r)
		}
	}
	writePlayers(dir, append(keep, playerRec{pid, stamp, kind}))
}

// playingState counts the recorded players still running (verified by procStamp, the file is not rewritten;
// the Stop dialog's own process is not a player) and says whether one of them is a finale.
func playingState(dir string) (n int, finale bool) {
	for _, r := range readPlayers(dir) {
		if st := procStamp(r.PID); st != "" && st == r.Stamp && r.Kind != "control" {
			n++
			finale = finale || r.Kind == "finale"
		}
	}
	return n, finale
}

// stopPlayers kills the players mw started that are still running and returns how many.
func stopPlayers(dir string) int {
	n := 0
	for _, r := range readPlayers(dir) {
		if st := procStamp(r.PID); st != "" && st == r.Stamp {
			killPlayer(r.PID)
			n++
		}
	}
	writePlayers(dir, nil)
	return n
}

// ---- triggers ----

type fanfareEvent struct{ kind, reason string }

var fanfarePending *fanfareEvent

// tenantFinished: at least one item and none in inbox, ready, doing or blocked (a wave is an item too).
func tenantFinished(t *Tenant) bool {
	if len(t.Items) == 0 {
		return false
	}
	for _, it := range t.Items {
		switch it.Status {
		case "inbox", "ready", "doing", "blocked":
			return false
		}
	}
	return true
}

// fanfareAfterChange runs inside a done / wave close, before the save: it stamps fanfare_finale_at
// on the first finished transition and queues the clip to play once the command succeeded.
// A close that also finishes the tenant plays the finale only.
func fanfareAfterChange(w *Workspace, t *Tenant, it *Item, waveClosed bool, now string) {
	if !fanfareOn(fanfareEffective(w)) { // the per-user fanfare.enabled wins over the board
		return
	}
	if tenantFinished(t) && t.FanfareFinaleAt == "" {
		t.FanfareFinaleAt = now
		fanfarePending = &fanfareEvent{"finale", finaleReason(t)}
		return
	}
	if waveClosed && fanfarePending == nil {
		reason := ""
		if it != nil {
			reason = milestoneReason(t, it)
		}
		fanfarePending = &fanfareEvent{"milestone", reason}
	}
}

// resetFinale clears the finished stamp of tenants that have work again (called by save).
func resetFinale(w *Workspace) {
	for i := range w.Tenants {
		if t := &w.Tenants[i]; t.loaded && t.FanfareFinaleAt != "" && !tenantFinished(t) {
			t.FanfareFinaleAt = ""
		}
	}
}

// playPendingFanfare plays the queued clip; a failure is a stderr note, never an error.
func playPendingFanfare(w *Workspace) {
	ev := fanfarePending
	fanfarePending = nil
	if ev == nil {
		return
	}
	if err := playClip(w, ev.kind, ev.reason); err != nil {
		fmt.Fprintln(os.Stderr, "mw: fanfare:", err)
	}
}

//go:embed assets/fanfare-default.wav
var defaultFanfareWAV []byte

// pickPairOrBuiltin is pickPairFor; with no playable pair at all it falls back to the built-in WAV
// (a short synthesized fanfare, played for both milestone and finale), so a fresh user hears something.
func pickPairOrBuiltin(c FanfareConfig, dir string, pick func(int) int) (*pairInfo, error) {
	p, err := pickPairFor(c, dir, pick)
	if err == nil {
		return p, nil
	}
	file, werr := builtinFanfareFile()
	if werr != nil {
		return nil, err
	}
	if !canPlay(file) {
		return nil, fmt.Errorf("%v; the built-in sound cannot play either (no audio player found)", err)
	}
	return &pairInfo{Name: "built-in", Fanfare: file, Finale: file, State: "ready"}, nil
}

// builtinFanfareFile extracts the embedded WAV to os.UserCacheDir()/miniwaves/fanfare-default-<sha256[:12]>.wav
// (os.TempDir only when there is no cache dir) and returns its path. A file with the right size is reused;
// the write is a temp file plus rename.
func builtinFanfareFile() (string, error) {
	sum := sha256.Sum256(defaultFanfareWAV)
	name := "fanfare-default-" + hex.EncodeToString(sum[:])[:12] + ".wav"
	dir, err := os.UserCacheDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "miniwaves")
	if os.MkdirAll(dir, 0o700) != nil {
		dir = os.TempDir()
	}
	file := filepath.Join(dir, name)
	if st, serr := os.Stat(file); serr == nil && st.Size() == int64(len(defaultFanfareWAV)) {
		if b, rerr := os.ReadFile(file); rerr == nil && sha256.Sum256(b) == sum {
			return file, nil
		}
	}
	if err := writeFileAtomic(file, defaultFanfareWAV); err != nil {
		return "", err
	}
	return file, nil
}

func playClip(w *Workspace, kind, reason string) error {
	c := fanfareEffective(w)
	p, err := pickPairOrBuiltin(c, expandHome(c.Dir), rand.Intn)
	if err != nil {
		return err
	}
	file, secs := p.Fanfare, c.MilestoneSeconds
	if kind == "finale" {
		file, secs = p.Finale, c.FinaleSeconds
		return playFinale(expandHome(c.Dir), file, secs, c.Volume, *c.StopDialog, reason)
	}
	if reason != "" { // a banner, never a blocking dialog; independent of stop_dialog (that is the finale's Stop dialog)
		notifyMilestone(reason)
	}
	return playFile(expandHome(c.Dir), file, secs, c.Volume)
}

// ---- fetch ----

var fanfareBackoff = 5 * time.Second

// licenceURLs: written to LICENSE.txt so CC attribution carries a link to the licence.
var licenceURLs = map[string]string{
	"CC0":       "https://creativecommons.org/publicdomain/zero/1.0/",
	"CC BY 3.0": "https://creativecommons.org/licenses/by/3.0/",
	"CC BY 4.0": "https://creativecommons.org/licenses/by/4.0/",
}

const fanfareHost = "upload.wikimedia.org"

// maxDownload bounds one track.
var maxDownload int64 = 64 << 20

// fanfareRedirect allows only https redirects within the Commons upload host.
func fanfareRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	if req.URL.Scheme != "https" || req.URL.Hostname() != fanfareHost {
		return fmt.Errorf("refusing redirect to %s", req.URL.Redacted())
	}
	return nil
}

// fanfareHTTP: 30 s for the response headers, 600 s for the whole transfer (the largest track is a few MB).
var fanfareHTTP = &http.Client{
	Timeout:       600 * time.Second,
	CheckRedirect: fanfareRedirect,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	},
}

func sha256File(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// download streams url to dest+".part", checks the sha256 against want (when set) and only then renames.
func download(client *http.Client, url, dest, want string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "mw-fanfare/"+version+" (https://github.com/stormovermars/miniwaves)")
	var resp *http.Response
	for try := 0; ; try++ {
		if resp, err = client.Do(req); err != nil {
			return "", err
		}
		if resp.StatusCode != 429 && resp.StatusCode != 503 || try >= 3 {
			break
		}
		wait := fanfareBackoff * time.Duration(try+1) // the Commons CDN rate-limits bursts
		if ra, e := strconv.Atoi(resp.Header.Get("Retry-After")); e == nil && ra > 0 && ra <= 60 {
			wait = time.Duration(ra) * time.Second
		}
		resp.Body.Close()
		time.Sleep(wait)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, maxDownload+1))
	if err == nil && n > maxDownload {
		err = fmt.Errorf("larger than %d bytes", maxDownload)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if want != "" && sum != want {
		os.Remove(tmp)
		return sum, fmt.Errorf("sha256 mismatch (got %.12s, want %.12s)", sum, want)
	}
	return sum, os.Rename(tmp, dest)
}

// fanfareFetch downloads pairs into dir, writes LICENSE.txt, README.txt for the bring-your-own
// slots and manifest.json, and prints ok/fail per pair. It returns the number of failed pairs.
func fanfareFetch(dir string, pairs []fanfarePair, client *http.Client, out io.Writer) int {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(out, "fail %s: %v\n", dir, err)
		return 1
	}
	m := fanfareManifest{}
	failed := 0
	for _, p := range pairs {
		folder := filepath.Join(dir, p.Name)
		mp := manifestPair{Name: p.Name, Label: p.Label, Source: "commons"}
		var errs []string
		var lic strings.Builder
		fmt.Fprintf(&lic, "Pair: %s (%s)\nCollection fetched by mw fanfare fetch; every track is public domain, CC0 or CC-BY.\n", p.Name, p.Label)
		if err := os.MkdirAll(folder, 0o755); err != nil {
			errs = append(errs, err.Error())
		}
		cached := true
		for _, tr := range p.Tracks {
			file := tr.Kind + "." + tr.Ext
			dest := filepath.Join(folder, file)
			want := tr.SHA256
			if len(want) != 64 {
				errs = append(errs, tr.Kind+": no sha256 pin, refusing to download")
				continue
			}
			sum, err := sha256File(dest)
			if err != nil || sum != want {
				cached = false
				if sum, err = download(client, tr.URL, dest, want); err != nil {
					errs = append(errs, tr.Kind+": "+err.Error())
					continue
				}
			}
			mp.Tracks = append(mp.Tracks, manifestTrack{tr.Kind, file, tr.URL, tr.Source, tr.License, tr.Attribution, tr.Title, sum})
			fmt.Fprintf(&lic, "\n%s: %s\nSource: %s\nDownload: %s\nLicense: %s\n%sAttribution: %s\nSHA-256: %s\n", tr.Kind, tr.Title, tr.Source, tr.URL, tr.License, licenseLine(tr.License), tr.Attribution, sum)
		}
		if len(errs) == 0 {
			if err := os.WriteFile(filepath.Join(folder, "LICENSE.txt"), []byte(lic.String()), 0o644); err != nil {
				errs = append(errs, err.Error())
			}
		}
		m.Pairs = append(m.Pairs, mp)
		if len(errs) > 0 {
			failed++
			fmt.Fprintf(out, "fail %s: %s\n", p.Name, strings.Join(errs, "; "))
		} else if cached {
			fmt.Fprintf(out, "ok   %s (present)\n", p.Name)
		} else {
			fmt.Fprintf(out, "ok   %s\n", p.Name)
		}
	}
	for _, s := range byoSlots {
		folder := filepath.Join(dir, s.Name)
		err := os.MkdirAll(folder, 0o755)
		if err == nil {
			err = os.WriteFile(filepath.Join(folder, "README.txt"), []byte(byoReadme(s.Label, s.Fanfare, s.Finale)), 0o644)
		}
		if err != nil {
			fmt.Fprintf(out, "fail %s: %v\n", s.Name, err)
			failed++
		}
		m.Pairs = append(m.Pairs, manifestPair{Name: s.Name, Label: s.Label, Source: "user"})
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, manifestName), append(b, '\n'), 0o644); err != nil {
		fmt.Fprintf(out, "fail manifest: %v\n", err)
		failed++
	}
	ready, unplayable := 0, 0
	for _, p := range scanPairs(dir) {
		switch p.State {
		case "ready":
			ready++
		case "unplayable here":
			unplayable++
		}
	}
	if ready == 0 && unplayable > 0 {
		fmt.Fprintf(out, "warning: none of the %d downloaded pair(s) can be played with the player available here (macOS afplay: mp3/m4a/aac/wav/aiff; Linux paplay: wav/ogg/flac; Windows: wav); install ffmpeg (ffplay) or mpv\n", unplayable)
	}
	return failed
}

// ---- command ----

func fanfareCommand(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if err := allowOptions(o, "json"); err != nil {
		return err
	}
	sub := ""
	if len(pos) > 0 {
		sub, pos = pos[0], pos[1:]
	}
	if sub == "on" || sub == "off" { // short for: set enabled=true|false
		if len(pos) != 0 {
			return fmt.Errorf("usage: mw fanfare %s", sub)
		}
		sub, pos = "set", []string{"enabled=" + map[string]string{"on": "true", "off": "false"}[sub]}
	}
	c := fanfareCfg(w)
	dir := expandHome(c.Dir)
	switch sub {
	case "":
		if len(pos) != 0 {
			return errors.New("usage: mw fanfare [on | off | set K=V... | sets | use NAME|all | test [milestone|finale] | fetch | list | stop]")
		}
		pairs := scanPairs(dir)
		ready, unplayable, incomplete, empty := 0, 0, 0, 0
		for _, p := range pairs {
			switch p.State {
			case "ready":
				ready++
			case "unplayable here":
				unplayable++
			case "incomplete":
				incomplete++
			default:
				empty++
			}
		}
		_, merr := os.Stat(filepath.Join(dir, manifestName))
		eff, src := fanfareEffectiveSrc(w) // config stays the board's; enabled/use are what a play would do
		if o["json"] == "true" {
			playing, playingFinale := playingState(dir)
			base := map[string]any{"playing": playing, "playing_finale": playingFinale, "config": c, "dir": dir, "ready": ready, "unplayable": unplayable, "incomplete": incomplete, "empty": empty, "manifest": merr == nil, "source": src}
			if fanfareBoardSilencesUser(w) {
				base["hint"] = fanfareBoardOffHint
			}
			return fanfareStatusJSON(base, eff, pairs)
		}
		onOff := func(b bool) string { return map[bool]string{true: "enabled", false: "disabled"}[b] }
		state := onOff(fanfareOn(eff))
		boardState := "unset"
		if c.Enabled != nil {
			boardState = onOff(*c.Enabled)
		}
		if src["enabled"] == "user" {
			state += " (user; board: " + boardState + ")"
		}
		fmt.Printf("fanfare: %s\ndir: %s\nmilestone_seconds: %d\nfinale_seconds: %d\nvolume: %g\nstop_dialog: %v\n", state, c.Dir, c.MilestoneSeconds, c.FinaleSeconds, c.Volume, *c.StopDialog)
		if src["use"] == "user" {
			fmt.Printf("use: %s (user; board: %s)\n", activeSet(eff), activeSet(c))
		} else {
			fmt.Printf("use: %s\n", activeSet(c))
		}
		if fanfareBoardSilencesUser(w) {
			fmt.Println("hint: " + fanfareBoardOffHint)
		}
		man := "manifest present"
		if merr != nil {
			man = "no manifest (mw fanfare fetch)"
		}
		fmt.Printf("collection: %d ready, %d unplayable here, %d incomplete, %d empty (bring your own); %s\n", ready, unplayable, incomplete, empty, man)
		return nil
	case "sets":
		return fanfareSetsCommand(path, w, o, pos, dir)
	case "use":
		return fanfareUseCommand(path, w, pos)
	case "list":
		return printData(append([]pairInfo{}, scanPairs(dir)...), o)
	case "set":
		if len(pos) == 0 {
			return errors.New("usage: mw fanfare set enabled=true milestone_seconds=N finale_seconds=N volume=V stop_dialog=true dir=PATH")
		}
		for _, kv := range pos {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return fmt.Errorf("%q: expected KEY=VALUE", kv)
			}
			switch k {
			case "enabled":
				b, e := strconv.ParseBool(v)
				if e != nil {
					return fmt.Errorf("enabled: want true or false, got %q", v)
				}
				c.Enabled = &b
			case "dir":
				if strings.TrimSpace(v) == "" {
					return errors.New("dir: empty")
				}
				if strings.HasPrefix(strings.TrimSpace(v), "-") {
					return fmt.Errorf("dir: %q starts with '-'", v)
				}
				abs, e := filepath.Abs(expandHome(strings.TrimSpace(v)))
				if e != nil {
					return fmt.Errorf("dir: %v", e)
				}
				c.Dir = abs
			case "milestone_seconds", "finale_seconds":
				n, e := strconv.Atoi(v)
				if e != nil || n < 1 || n > 3600 {
					return fmt.Errorf("%s: want 1..3600, got %q", k, v)
				}
				if k == "milestone_seconds" {
					c.MilestoneSeconds = n
				} else {
					c.FinaleSeconds = n
				}
			case "stop_dialog":
				b, e := strconv.ParseBool(v)
				if e != nil {
					return fmt.Errorf("stop_dialog: want true or false, got %q", v)
				}
				c.StopDialog = &b
			case "volume":
				f, e := strconv.ParseFloat(v, 64)
				if e != nil || f <= 0 || f > 4 {
					return fmt.Errorf("volume: want a number above 0 up to 4, got %q", v)
				}
				c.Volume = f
			default:
				return fmt.Errorf("unknown setting %q (enabled, dir, milestone_seconds, finale_seconds, volume, stop_dialog)", k)
			}
		}
		w.Fanfare = &c
		if err := save(path, w); err != nil {
			return err
		}
		fmt.Printf("fanfare saved: enabled=%v dir=%s milestone_seconds=%d finale_seconds=%d volume=%g stop_dialog=%v\n", fanfareOn(c), c.Dir, c.MilestoneSeconds, c.FinaleSeconds, c.Volume, *c.StopDialog)
		return nil
	case "test":
		kind := "milestone"
		if len(pos) > 1 || len(pos) == 1 && pos[0] != "milestone" && pos[0] != "finale" {
			return errors.New("usage: mw fanfare test [milestone|finale]")
		}
		if len(pos) == 1 {
			kind = pos[0]
		}
		p, err := pickPairOrBuiltin(fanfareEffective(w), dir, rand.Intn)
		if err != nil {
			return err
		}
		file, secs := p.Fanfare, c.MilestoneSeconds
		var perr error
		if kind == "finale" {
			file, secs = p.Finale, c.FinaleSeconds
			perr = playFinale(dir, file, secs, c.Volume, *c.StopDialog, finaleTestReason)
		} else {
			perr = playFile(dir, file, secs, c.Volume)
		}
		if perr != nil {
			return perr
		}
		fmt.Printf("playing %s %s (%ds): %s\n", p.Name, kind, secs, file)
		return nil
	case "stop":
		if len(pos) != 0 {
			return errors.New("usage: mw fanfare stop")
		}
		fmt.Printf("stopped %d player(s)\n", stopPlayers(dir))
		return nil
	case "fetch":
		if len(pos) != 0 {
			return errors.New("usage: mw fanfare fetch")
		}
		if n := fanfareFetch(dir, builtinPairs, fanfareHTTP, os.Stdout); n > 0 {
			return fmt.Errorf("%d fetch step(s) failed", n)
		}
		return nil
	}
	return fmt.Errorf("unknown fanfare command %q (set, sets, use, test, fetch, list, stop)", sub)
}

func licenseLine(l string) string {
	if u := licenceURLs[l]; u != "" {
		return "License URL: " + u + "\n"
	}
	return ""
}
