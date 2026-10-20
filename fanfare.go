package main

import (
	"crypto/sha256"
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
	Enabled          bool    `json:"enabled"`
	Dir              string  `json:"dir"`
	MilestoneSeconds int     `json:"milestone_seconds"`
	FinaleSeconds    int     `json:"finale_seconds"`
	Volume           float64 `json:"volume"`
	StopDialog       *bool   `json:"stop_dialog,omitempty"` // finale Stop dialog; nil = true
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
	_, err := startPlayer(dir, file, secs, vol)
	return err
}

// playFinale plays the finale and, when stopDialog is set, opens the Stop control beside it.
func playFinale(dir, file string, secs int, vol float64, stopDialog bool) error {
	pid, err := startPlayer(dir, file, secs, vol)
	if err != nil {
		return err
	}
	if stopDialog {
		startStopControl(dir, pid, secs)
	}
	return nil
}

// stopControlCommand builds the command of the finale's Stop dialog: a dialog with a Stop button
// that times out after secs+2 seconds and kills the player (pid) itself. ok is false when this
// platform has no dialog. MW_FANFARE_PLAYER (test stub) disables it unless MW_FANFARE_STOP_CONTROL
// names a stub, called as: STUB PLAYERPID SECONDS.
func stopControlCommand(goos string, playerPID, secs int, look func(string) (string, error)) (string, []string, bool) {
	if stub := os.Getenv("MW_FANFARE_STOP_CONTROL"); stub != "" {
		return stub, []string{strconv.Itoa(playerPID), strconv.Itoa(secs)}, true
	}
	if os.Getenv("MW_FANFARE_PLAYER") != "" {
		return "", nil, false
	}
	n := strconv.Itoa(secs + 2)
	p := strconv.Itoa(playerPID)
	switch goos {
	case "darwin":
		script := `r=$(osascript -e 'display dialog "Celebration playing" with title "miniwaves" buttons {"Stop"} default button 1 giving up after ` + n + `')` +
			`; case "$r" in *"gave up:true"*) ;; *"button returned:Stop"*) kill -TERM -` + p + ` 2>/dev/null || kill -TERM ` + p + `;; esac`
		return "sh", []string{"-c", script}, true
	case "windows":
		script := `$r=(New-Object -ComObject WScript.Shell).Popup('Celebration playing - click OK to stop',` + n + `,'miniwaves',0); if($r -eq 1){taskkill /PID ` + p + ` /T /F}`
		return "powershell", []string{"-NoProfile", "-c", script}, true
	}
	if _, err := look("zenity"); err != nil {
		return "", nil, false
	}
	script := `zenity --question --title=miniwaves --text='Celebration playing' --ok-label=Stop --cancel-label=Close --timeout=` + n + ` && kill -TERM -` + p + ` 2>/dev/null || true`
	return "sh", []string{"-c", script}, true
}

// startStopControl launches the Stop dialog detached and records it (best effort, never fails).
func startStopControl(dir string, playerPID, secs int) {
	name, args, ok := stopControlCommand(runtime.GOOS, playerPID, secs, lookPath)
	if !ok {
		return
	}
	if pid, err := startDetached(name, args); err == nil {
		recordPlayer(dir, pid)
	}
}

func startPlayer(dir, file string, secs int, vol float64) (int, error) {
	name, args, err := playerCommand(runtime.GOOS, file, secs, vol, lookPath)
	if err != nil {
		return 0, err
	}
	pid, err := startDetached(name, args)
	if err != nil {
		return 0, err
	}
	recordPlayer(dir, pid)
	return pid, nil
}

func startDetached(name string, args []string) (int, error) {
	cmd := exec.Command(name, args...)
	detach(cmd, true)
	if err := cmd.Start(); err != nil {
		if runtime.GOOS != "windows" {
			return 0, err
		}
		cmd = exec.Command(name, args...) // a job object may refuse the breakaway: retry with the plain flags
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
	Stamp string `json:"stamp"` // identifies the process so a reused pid is never killed
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
func recordPlayer(dir string, pid int) {
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
	writePlayers(dir, append(keep, playerRec{pid, stamp}))
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

type fanfareEvent struct{ kind string }

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
func fanfareAfterChange(w *Workspace, t *Tenant, waveClosed bool, now string) {
	if !fanfareCfg(w).Enabled {
		return
	}
	if tenantFinished(t) && t.FanfareFinaleAt == "" {
		t.FanfareFinaleAt = now
		fanfarePending = &fanfareEvent{"finale"}
		return
	}
	if waveClosed && fanfarePending == nil {
		fanfarePending = &fanfareEvent{"milestone"}
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
	if err := playClip(w, ev.kind); err != nil {
		fmt.Fprintln(os.Stderr, "mw: fanfare:", err)
	}
}

func playClip(w *Workspace, kind string) error {
	c := fanfareCfg(w)
	p, err := pickPair(expandHome(c.Dir), rand.Intn)
	if err != nil {
		return err
	}
	file, secs := p.Fanfare, c.MilestoneSeconds
	if kind == "finale" {
		file, secs = p.Finale, c.FinaleSeconds
		return playFinale(expandHome(c.Dir), file, secs, c.Volume, *c.StopDialog)
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
			mp.Tracks = append(mp.Tracks, manifestTrack{tr.Kind, file, tr.URL, tr.Source, tr.License, tr.Attribution, sum})
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
	c := fanfareCfg(w)
	dir := expandHome(c.Dir)
	switch sub {
	case "":
		if len(pos) != 0 {
			return errors.New("usage: mw fanfare [set K=V... | test [milestone|finale] | fetch | list | stop]")
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
		if o["json"] == "true" {
			return printJSON(map[string]any{"config": c, "dir": dir, "ready": ready, "unplayable": unplayable, "incomplete": incomplete, "empty": empty, "manifest": merr == nil})
		}
		state := "disabled"
		if c.Enabled {
			state = "enabled"
		}
		fmt.Printf("fanfare: %s\ndir: %s\nmilestone_seconds: %d\nfinale_seconds: %d\nvolume: %g\nstop_dialog: %v\n", state, c.Dir, c.MilestoneSeconds, c.FinaleSeconds, c.Volume, *c.StopDialog)
		man := "manifest present"
		if merr != nil {
			man = "no manifest (mw fanfare fetch)"
		}
		fmt.Printf("collection: %d ready, %d unplayable here, %d incomplete, %d empty (bring your own); %s\n", ready, unplayable, incomplete, empty, man)
		return nil
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
				c.Enabled = b
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
		fmt.Printf("fanfare saved: enabled=%v dir=%s milestone_seconds=%d finale_seconds=%d volume=%g stop_dialog=%v\n", c.Enabled, c.Dir, c.MilestoneSeconds, c.FinaleSeconds, c.Volume, *c.StopDialog)
		return nil
	case "test":
		kind := "milestone"
		if len(pos) > 1 || len(pos) == 1 && pos[0] != "milestone" && pos[0] != "finale" {
			return errors.New("usage: mw fanfare test [milestone|finale]")
		}
		if len(pos) == 1 {
			kind = pos[0]
		}
		p, err := pickPair(dir, rand.Intn)
		if err != nil {
			return err
		}
		file, secs := p.Fanfare, c.MilestoneSeconds
		var perr error
		if kind == "finale" {
			file, secs = p.Finale, c.FinaleSeconds
			perr = playFinale(dir, file, secs, c.Volume, *c.StopDialog)
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
	return fmt.Errorf("unknown fanfare command %q (set, test, fetch, list, stop)", sub)
}

func licenseLine(l string) string {
	if u := licenceURLs[l]; u != "" {
		return "License URL: " + u + "\n"
	}
	return ""
}
