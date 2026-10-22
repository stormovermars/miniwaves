package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func fakeCollection(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "songs")
	for _, n := range []string{"a", "b", "c"} {
		f := filepath.Join(dir, n)
		if err := os.MkdirAll(f, 0o755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(f, "fanfare.mp3"), []byte("x"+n), 0o644)
		os.WriteFile(filepath.Join(f, "finale.ogg"), []byte("y"+n), 0o644)
	}
	os.MkdirAll(filepath.Join(dir, "star-wars"), 0o755) // empty bring-your-own slot
	os.WriteFile(filepath.Join(dir, "star-wars", "README.txt"), []byte("drop"), 0o644)
	os.MkdirAll(filepath.Join(dir, "half"), 0o755)
	os.WriteFile(filepath.Join(dir, "half", "fanfare.mp3"), []byte("x"), 0o644)
	return dir
}

func stubPlayer(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("sh stub")
	}
	log := filepath.Join(t.TempDir(), "played.log")
	stub := filepath.Join(t.TempDir(), "player.sh")
	os.WriteFile(stub, []byte("#!/bin/sh\necho \"$@\" >> '"+log+"'\n"), 0o755)
	t.Setenv("MW_FANFARE_PLAYER", stub)
	return log
}

func plays(t *testing.T, log string, want int) []string {
	t.Helper()
	var lines []string
	for i := 0; i < 60; i++ {
		b, _ := os.ReadFile(log)
		lines = nil
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if l != "" {
				lines = append(lines, l)
			}
		}
		if len(lines) >= want {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(150 * time.Millisecond) // anything extra would show up now
	b, _ := os.ReadFile(log)
	lines = nil
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) != want {
		t.Fatalf("want %d plays, got %d: %v", want, len(lines), lines)
	}
	return lines
}

func TestFanfareConfigRoundTrip(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "Ws"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"})
	got := captureOutput(t, func() { runAll(t, []string{"fanfare"}) })
	if !strings.Contains(got, "fanfare: disabled") || !strings.Contains(got, "milestone_seconds: 15") || !strings.Contains(got, "finale_seconds: 60") || !strings.Contains(got, "~/fanfare-finale-songs") {
		t.Fatalf("defaults: %s", got)
	}
	songs := filepath.Join(testCwd(t), "x", "songs")
	runAll(t, []string{"fanfare", "set", "enabled=true", "milestone_seconds=20", "finale_seconds=240", "dir=" + songs, "volume=0.5"})
	b, _ := os.ReadFile(path)
	sj, _ := json.Marshal(songs)
	if !strings.Contains(string(b), `"fanfare":{"enabled":true,"dir":`+string(sj)+`,"milestone_seconds":20,"finale_seconds":240,"volume":0.5,"stop_dialog":true}`) {
		t.Fatalf("not stored: %s", b)
	}
	w, _ := load(path)
	if c := fanfareCfg(w); !c.Enabled || c.Dir != songs || c.MilestoneSeconds != 20 || c.FinaleSeconds != 240 || c.Volume != 0.5 {
		t.Fatalf("%+v", c)
	}
	for _, bad := range []string{"enabled=maybe", "milestone_seconds=0", "volume=0", "nope=1", "dir"} {
		wantErr(t, "", "fanfare", "set", bad)
	}
}

func TestFanfareListAndPick(t *testing.T) {
	t.Setenv("MW_FANFARE_PLAYER", "/stub") // every format plays
	dir := fakeCollection(t)
	pairs := scanPairs(dir)
	states := map[string]string{}
	for _, p := range pairs {
		states[p.Name] = p.State
	}
	if states["a"] != "ready" || states["star-wars"] != "empty (bring your own)" || states["half"] != "incomplete" {
		t.Fatalf("%v", states)
	}
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		p, err := pickPair(dir, func(n int) int {
			if n != 3 {
				t.Fatalf("ready pairs %d, want 3", n)
			}
			return i
		})
		if err != nil {
			t.Fatal(err)
		}
		seen[p.Name] = true
	}
	if len(seen) != 3 || seen["star-wars"] || seen["half"] {
		t.Fatalf("%v", seen)
	}
	if _, err := pickPair(t.TempDir(), func(int) int { return 0 }); err == nil {
		t.Fatal("empty collection accepted")
	}
}

func TestFanfareTriggers(t *testing.T) {
	log := stubPlayer(t)
	path := inTempDir(t)
	dir := fakeCollection(t)
	runAll(t, []string{"init", "Ws"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"})
	runAll(t, []string{"add", "task", "One", "--status", "ready"}, []string{"add", "task", "Two", "--status", "ready"})
	runAll(t, []string{"wave", "create", "W"}, []string{"wave", "add", "WAV-001", "TSK-001"})
	// disabled: silence
	runAll(t, []string{"done", "TSK-001", "ok"})
	runAll(t, []string{"wave", "step", "WAV-001", "--na-rest", "test"}, []string{"wave", "close", "WAV-001", "--no-evidence", "t"})
	plays(t, log, 0)
	runAll(t, []string{"reopen", "WAV-001"}, []string{"reopen", "TSK-001"}, []string{"fanfare", "set", "enabled=true", "dir=" + dir, "milestone_seconds=7", "finale_seconds=9"})
	runAll(t, []string{"done", "TSK-001", "ok"}) // task done: no wave close, tenant not finished
	plays(t, log, 0)
	runAll(t, []string{"wave", "step", "WAV-001", "--na-rest", "test"}, []string{"wave", "close", "WAV-001", "--no-evidence", "t"})
	l := plays(t, log, 1) // wave closed, TSK-002 still ready: milestone
	if !strings.Contains(l[0], "fanfare.mp3") || !strings.Contains(l[0], " 7 ") {
		t.Fatalf("milestone: %v", l)
	}
	runAll(t, []string{"done", "TSK-002", "ok"}) // everything done: finale once
	l = plays(t, log, 2)
	if !strings.Contains(l[1], "finale.ogg") || !strings.Contains(l[1], " 9 ") {
		t.Fatalf("finale: %v", l)
	}
	w, _ := load(path)
	if w.Tenants[0].FanfareFinaleAt == "" {
		t.Fatal("finale not recorded")
	}
	runAll(t, []string{"note", "TSK-002", "again"}) // no transition, no replay
	plays(t, log, 2)
	runAll(t, []string{"add", "task", "Three", "--status", "ready"}) // new work resets
	if w, _ = load(path); w.Tenants[0].FanfareFinaleAt != "" {
		t.Fatal("finale stamp not reset")
	}
	runAll(t, []string{"done", "TSK-003", "ok"}) // finished again: fires again
	plays(t, log, 3)
}

func TestFanfareWaveCloseThatFinishesPlaysFinaleOnly(t *testing.T) {
	log := stubPlayer(t)
	inTempDir(t)
	dir := fakeCollection(t)
	runAll(t, []string{"init", "Ws"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"}, []string{"add", "task", "One", "--status", "ready"},
		[]string{"wave", "create", "W"}, []string{"wave", "add", "WAV-001", "TSK-001"}, []string{"done", "TSK-001", "ok"},
		[]string{"fanfare", "set", "enabled=true", "dir=" + dir},
		[]string{"wave", "step", "WAV-001", "--na-rest", "t"}, []string{"wave", "close", "WAV-001", "--no-evidence", "t"})
	l := plays(t, log, 1)
	if !strings.Contains(l[0], "finale.ogg") {
		t.Fatalf("%v", l)
	}
}

func TestFanfareTestCommandNeverBlocksOnMissingPlayer(t *testing.T) {
	log := stubPlayer(t)
	inTempDir(t)
	dir := fakeCollection(t)
	runAll(t, []string{"init", "Ws"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"}, []string{"fanfare", "set", "dir=" + dir})
	out := captureOutput(t, func() { runAll(t, []string{"fanfare", "test", "finale"}) })
	if !strings.Contains(out, "playing") || !strings.Contains(out, "finale") {
		t.Fatalf("%s", out)
	}
	plays(t, log, 1)
	wantErr(t, "usage", "fanfare", "test", "bogus")
	// a trigger with a broken player or empty collection only notes on stderr
	t.Setenv("MW_FANFARE_PLAYER", "/nonexistent/player")
	runAll(t, []string{"fanfare", "set", "enabled=true"}, []string{"add", "task", "A", "--status", "ready"}, []string{"done", "TSK-001", "ok"})
}

func TestPlayerCommandPerOS(t *testing.T) {
	t.Setenv("MW_FANFARE_PLAYER", "")
	have := func(names ...string) func(string) (string, error) {
		return func(n string) (string, error) {
			for _, x := range names {
				if x == n {
					return "/bin/" + n, nil
				}
			}
			return "", errors.New("missing")
		}
	}
	join := func(n string, a []string, err error) string {
		if err != nil {
			return "ERR " + err.Error()
		}
		return n + " " + strings.Join(a, " ")
	}
	cases := []struct {
		goos, file string
		vol        float64
		tools      []string
		want       string
	}{
		{"darwin", "/s/f.mp3", 1, []string{"afplay"}, "afplay -t 15 /s/f.mp3"},
		{"darwin", "/s/f.mp3", 0.5, []string{"afplay"}, "afplay -t 15 -v 0.5 /s/f.mp3"},
		{"darwin", "/s/f.ogg", 1, []string{"afplay", "ffplay"}, "ffplay -nodisp -autoexit -loglevel quiet -t 15 /s/f.ogg"},
		{"darwin", "/s/f.opus", 1, []string{"afplay"}, "ERR afplay cannot play .opus"},
		{"linux", "/s/f.mp3", 1, []string{"ffplay", "mpv"}, "ffplay -nodisp -autoexit -loglevel quiet -t 15 /s/f.mp3"},
		{"linux", "/s/f.mp3", 0.5, []string{"ffplay"}, "ffplay -nodisp -autoexit -loglevel quiet -t 15 -volume 50 /s/f.mp3"},
		{"linux", "/s/f.mp3", 1, []string{"mpv", "paplay"}, "mpv --no-video --really-quiet --length=15 /s/f.mp3"},
		{"linux", "/s/f.wav", 1, []string{"paplay"}, "paplay /s/f.wav"},
		{"linux", "/s/f.wav", 1, nil, "ERR no player for .wav found"},
		{"linux", "/s/f.mp3", 1, []string{"paplay"}, "ERR no player for .mp3 found"},
		{"windows", `C:\s\f.wav`, 1, nil, "powershell -NoProfile -c (New-Object Media.SoundPlayer 'C:\\s\\f.wav').PlaySync()"},
		{"windows", `C:\s\f.mp3`, 1, nil, "ERR on Windows only .wav plays"},
		{"plan9", "/s/f.mp3", 1, nil, "ERR fanfare playback is not supported on plan9"},
	}
	for _, c := range cases {
		got := join(playerCommand(c.goos, c.file, 15, c.vol, have(c.tools...)))
		if !strings.HasPrefix(got, c.want) {
			t.Errorf("%s %s: got %q want %q", c.goos, c.file, got, c.want)
		}
	}
	t.Setenv("MW_FANFARE_PLAYER", "/stub")
	if n, a, _ := playerCommand("darwin", "/f.mp3", 15, 1, have()); n != "/stub" || strings.Join(a, " ") != "/f.mp3 15 1" {
		t.Fatalf("override: %s %v", n, a)
	}
}

func TestFanfareFetch(t *testing.T) {
	body := map[string][]byte{"/a.mp3": []byte("fanfare-bytes"), "/b.ogg": []byte("finale-bytes")}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if b, ok := body[r.URL.Path]; ok {
			w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	sum := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	pairs := []fanfarePair{
		{Name: "good", Label: "Good", Tracks: []fanfareTrack{
			{Kind: "fanfare", Ext: "mp3", URL: srv.URL + "/a.mp3", Source: "s", License: "CC0", Attribution: "me", SHA256: sum(body["/a.mp3"])},
			{Kind: "finale", Ext: "ogg", URL: srv.URL + "/b.ogg", Source: "s", License: "Public domain", SHA256: sum(body["/b.ogg"])}}},
		{Name: "bad", Label: "Bad", Tracks: []fanfareTrack{
			{Kind: "fanfare", Ext: "mp3", URL: srv.URL + "/a.mp3", SHA256: strings.Repeat("0", 64)},
			{Kind: "finale", Ext: "ogg", URL: srv.URL + "/missing.ogg"}}},
	}
	t.Setenv("MW_FANFARE_PLAYER", "/stub")
	dir := filepath.Join(t.TempDir(), "songs")
	var out bytes.Buffer
	if n := fanfareFetch(dir, pairs, srv.Client(), &out); n != 1 {
		t.Fatalf("failed %d: %s", n, out.String())
	}
	if !strings.Contains(out.String(), "ok   good") || !strings.Contains(out.String(), "fail bad") || !strings.Contains(out.String(), "sha256 mismatch") {
		t.Fatalf("%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "bad", "fanfare.mp3")); err == nil {
		t.Fatal("mismatching download kept")
	}
	lic, _ := os.ReadFile(filepath.Join(dir, "good", "LICENSE.txt"))
	if !strings.Contains(string(lic), "License: CC0") || !strings.Contains(string(lic), "Attribution: me") || !strings.Contains(string(lic), sum(body["/b.ogg"])) {
		t.Fatalf("%s", lic)
	}
	m := readManifest(dir)
	if mp := m.pair("good"); mp == nil || mp.Source != "commons" || len(mp.Tracks) != 2 || mp.Tracks[1].SHA256 != sum(body["/b.ogg"]) {
		t.Fatalf("%+v", m)
	}
	if mp := m.pair("star-wars"); mp == nil || mp.Source != "user" {
		t.Fatalf("byo slot: %+v", m.pair("star-wars"))
	}
	if _, err := os.Stat(filepath.Join(dir, "star-wars", "README.txt")); err != nil {
		t.Fatal(err)
	}
	// idempotent: a second run downloads nothing for the good pair
	hits := 0
	srv.Config.Handler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits++ })
	out.Reset()
	fanfareFetch(dir, pairs[:1], srv.Client(), &out)
	if hits != 0 || !strings.Contains(out.String(), "ok   good (present)") {
		t.Fatalf("hits %d: %s", hits, out.String())
	}
	var ready, empty int
	for _, p := range scanPairs(dir) {
		if p.State == "ready" {
			ready++
		} else if p.State == "empty (bring your own)" {
			empty++
		}
	}
	if ready != 1 || empty != 7 { // good is ready; "bad" and the six bring-your-own slots are empty
		t.Fatalf("ready %d empty %d", ready, empty)
	}
}

func TestFanfareDownloadRetriesRateLimit(t *testing.T) {
	fanfareBackoff = time.Millisecond
	defer func() { fanfareBackoff = 5 * time.Second }()
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n++; n < 3 {
			http.Error(w, "slow down", 429)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	if _, err := download(srv.Client(), srv.URL, filepath.Join(t.TempDir(), "f"), ""); err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestBuiltinPairsAreFreeLicences(t *testing.T) {
	if len(builtinPairs) < 5 {
		t.Fatalf("%d pairs", len(builtinPairs))
	}
	ok := map[string]bool{"Public domain": true, "CC0": true, "CC BY 3.0": true, "CC BY 4.0": true}
	for _, p := range builtinPairs {
		if len(p.Tracks) != 2 || p.Tracks[0].Kind != "fanfare" || p.Tracks[1].Kind != "finale" {
			t.Fatalf("%s tracks", p.Name)
		}
		for _, tr := range p.Tracks {
			if len(tr.SHA256) != 64 || strings.Trim(tr.SHA256, "0123456789abcdef") != "" {
				t.Fatalf("%s %s: sha256 pin missing or malformed", p.Name, tr.Kind)
			}
			if strings.Contains(tr.Attribution, "see source") || strings.HasSuffix(tr.Attribution, " ") || len(tr.Attribution) < 8 || !strings.HasPrefix(tr.Source, "https://commons.wikimedia.org/wiki/File") {
				t.Fatalf("%s %s: incomplete attribution/source: %+v", p.Name, tr.Kind, tr)
			}
			if strings.HasPrefix(tr.License, "CC") && licenceURLs[tr.License] == "" {
				t.Fatalf("%s %s: no licence URL for %s", p.Name, tr.Kind, tr.License)
			}
			if !ok[tr.License] || !strings.HasPrefix(tr.URL, "https://upload.wikimedia.org/") || tr.Source == "" || tr.Attribution == "" {
				t.Fatalf("%s %s: %+v", p.Name, tr.Kind, tr)
			}
		}
	}
	for _, s := range byoSlots {
		for _, p := range builtinPairs {
			if p.Name == s.Name {
				t.Fatalf("%s is both downloaded and bring-your-own", s.Name)
			}
		}
	}
}

func TestFanfareDownloadVerifiesBeforeRename(t *testing.T) {
	body := []byte("track-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	h := sha256.Sum256(body)
	good := hex.EncodeToString(h[:])
	dest := filepath.Join(t.TempDir(), "fanfare.mp3")
	if _, err := download(srv.Client(), srv.URL, dest, strings.Repeat("0", 64)); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("err %v", err)
	}
	for _, p := range []string{dest, dest + ".part"} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("%s left behind after a bad hash", p)
		}
	}
	if _, err := download(srv.Client(), srv.URL, dest, good); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); string(b) != string(body) {
		t.Fatal("good download missing")
	}
}

func TestFanfareDownloadRedirectAndSize(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("x")) }))
	defer plain.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/r" {
			http.Redirect(w, r, plain.URL, 302)
			return
		}
		w.Write([]byte("0123456789abcdef"))
	}))
	defer srv.Close()
	c := &http.Client{CheckRedirect: fanfareRedirect}
	if _, err := download(c, srv.URL+"/r", filepath.Join(t.TempDir(), "f"), ""); err == nil || !strings.Contains(err.Error(), "refusing redirect") {
		t.Fatalf("redirect to another host accepted: %v", err)
	}
	maxDownload = 8
	defer func() { maxDownload = 64 << 20 }()
	dest := filepath.Join(t.TempDir(), "f")
	if _, err := download(c, srv.URL, dest, ""); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("oversize accepted: %v", err)
	}
	if _, err := os.Stat(dest + ".part"); err == nil {
		t.Fatal(".part left")
	}
}

func TestFanfareFetchRefusesUnpinned(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer srv.Close()
	pairs := []fanfarePair{{Name: "p", Tracks: []fanfareTrack{{Kind: "fanfare", Ext: "mp3", URL: srv.URL}}}}
	var out bytes.Buffer
	if n := fanfareFetch(filepath.Join(t.TempDir(), "s"), pairs, srv.Client(), &out); n != 1 || hits != 0 || !strings.Contains(out.String(), "no sha256 pin") {
		t.Fatalf("n=%d hits=%d %s", n, hits, out.String())
	}
}

func TestFanfareUnplayablePairsSkipped(t *testing.T) {
	t.Setenv("MW_FANFARE_PLAYER", "")
	dir := fakeCollection(t) // fanfare.mp3 + finale.ogg
	old := lookPath
	defer func() { lookPath = old }()
	lookPath = func(n string) (string, error) {
		if n == "afplay" || n == "paplay" {
			return "/bin/" + n, nil
		}
		return "", errors.New("missing")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip()
	}
	if runtime.GOOS == "darwin" { // afplay cannot play the .ogg finales
		for _, p := range scanPairs(dir) {
			if p.Name == "a" && p.State != "unplayable here" {
				t.Fatalf("state %s", p.State)
			}
		}
		if _, err := pickPair(dir, func(int) int { return 0 }); err == nil {
			t.Fatal("unplayable pair picked")
		}
		var out bytes.Buffer
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("z")) }))
		defer srv.Close()
		h := sha256.Sum256([]byte("z"))
		sum := hex.EncodeToString(h[:])
		pairs := []fanfarePair{{Name: "p", Tracks: []fanfareTrack{
			{Kind: "fanfare", Ext: "mp3", URL: srv.URL, SHA256: sum}, {Kind: "finale", Ext: "ogg", URL: srv.URL, SHA256: sum}}}}
		fanfareFetch(filepath.Join(t.TempDir(), "s"), pairs, srv.Client(), &out)
		if !strings.Contains(out.String(), "warning: none of the 1") {
			t.Fatalf("%s", out.String())
		}
	}
}

func TestFanfareClampsAndSetDir(t *testing.T) {
	inTempDir(t)
	c := fanfareCfg(&Workspace{Fanfare: &FanfareConfig{Dir: "-x", MilestoneSeconds: 999999, FinaleSeconds: 99999, Volume: 50}})
	if c.Dir != defaultFanfareDir || c.MilestoneSeconds != 3600 || c.FinaleSeconds != 3600 || c.Volume != 4 {
		t.Fatalf("%+v", c)
	}
	runAll(t, []string{"init", "Ws"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"}, []string{"fanfare", "set", "dir=songs"})
	w, _ := load(filepath.Join(testCwd(t), dataFile))
	if !filepath.IsAbs(w.Fanfare.Dir) || !strings.HasSuffix(w.Fanfare.Dir, "songs") {
		t.Fatalf("dir %q", w.Fanfare.Dir)
	}
	runAll(t, []string{"fanfare", "set", "dir=~/x"})
	w, _ = load(filepath.Join(testCwd(t), dataFile))
	if strings.HasPrefix(w.Fanfare.Dir, "~") || !filepath.IsAbs(w.Fanfare.Dir) {
		t.Fatalf("dir %q", w.Fanfare.Dir)
	}
	wantErr(t, "starts with '-'", "fanfare", "set", "dir=-x")
}

func testCwd(t *testing.T) string {
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestFindTrackCaseInsensitive(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "Fanfare.MP3"), []byte("x"), 0o644)
	if findTrack(d, "fanfare") == "" {
		t.Fatal("Fanfare.MP3 ignored")
	}
}

func TestFanfareJSONOutputUnchanged(t *testing.T) {
	log := stubPlayer(t)
	inTempDir(t)
	dir := fakeCollection(t)
	runAll(t, []string{"init", "Ws"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"}, []string{"add", "task", "One", "--status", "ready"})
	off := captureOutput(t, func() { runAll(t, []string{"done", "TSK-001", "ok"}) })
	runAll(t, []string{"reopen", "TSK-001"}, []string{"fanfare", "set", "enabled=true", "dir=" + dir})
	on := captureOutput(t, func() { runAll(t, []string{"done", "TSK-001", "ok"}) })
	plays(t, log, 1) // the finale played
	var js any
	if out := captureOutput(t, func() { runAll(t, []string{"list", "-j"}) }); json.Unmarshal([]byte(out), &js) != nil {
		t.Fatalf("list -j is not pure JSON with fanfare enabled: %s", out)
	}
	norm := func(s string) string {
		return strings.Join(strings.Fields(regexp.MustCompile(`"updated":"[^"]*"|\d{4}-\d\d-\d\dT[0-9:.]+Z?`).ReplaceAllString(s, "T")), " ")
	}
	if norm(on) != norm(off) {
		t.Fatalf("stdout differs:\n%s\n%s", off, on)
	}
}

func TestFanfareArchiveEmptyingTenantRearms(t *testing.T) {
	log := stubPlayer(t)
	path := inTempDir(t)
	dir := fakeCollection(t)
	runAll(t, []string{"init", "Ws"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"}, []string{"fanfare", "set", "enabled=true", "dir=" + dir},
		[]string{"add", "task", "One", "--status", "ready"}, []string{"done", "TSK-001", "ok"})
	plays(t, log, 1)
	runAll(t, []string{"archive", "--days", "0"})
	w, _ := load(path)
	if len(w.Tenants[0].Items) != 0 || w.Tenants[0].FanfareFinaleAt != "" {
		t.Fatalf("items %d stamp %q", len(w.Tenants[0].Items), w.Tenants[0].FanfareFinaleAt)
	}
	plays(t, log, 1) // archiving is not a finish
	runAll(t, []string{"add", "task", "Two", "--status", "ready"}, []string{"done", "TSK-002", "ok"})
	plays(t, log, 2)
}

func TestFanfareStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh stub")
	}
	t.Setenv("MW_FANFARE_PLAYER", "") // set below
	d := t.TempDir()
	stub := filepath.Join(d, "slow.sh")
	os.WriteFile(stub, []byte("#!/bin/sh\nsleep 30\n"), 0o755)
	t.Setenv("MW_FANFARE_PLAYER", stub)
	dir := filepath.Join(d, "songs")
	os.MkdirAll(dir, 0o755)
	if err := playFile(dir, "/x/f.mp3", 5, 1); err != nil {
		t.Fatal(err)
	}
	rs := readPlayers(dir)
	if len(rs) != 1 || procStamp(rs[0].PID) == "" {
		t.Fatalf("not recorded/running: %+v", rs)
	}
	pid := rs[0].PID
	writePlayers(dir, append(rs, playerRec{PID: 1, Stamp: "stale"})) // stale entry must be ignored
	if n := stopPlayers(dir); n != 1 {
		t.Fatalf("stopped %d", n)
	}
	for i := 0; i < 40 && procStamp(pid) != ""; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if st := procStamp(pid); st != "" && !strings.Contains(procState(pid), "Z") {
		t.Fatalf("player still running: %q", st)
	}
	if _, err := os.Stat(playersPath(dir)); err == nil {
		t.Fatal(".players.json kept")
	}
}

func procState(pid int) string {
	b, _ := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	return string(b)
}

func TestTenantProtocolShowIsReadOnly(t *testing.T) {
	for args, want := range map[string]bool{
		"tenant protocol b":                 true,
		"tenant protocol --workspace":       true,
		"tenant protocol b set X Y":         false,
		"tenant protocol b reset":           false,
		"tenant protocol --workspace set Z": false,
		"tenant list":                       true,
		"tenant add b B":                    false,
	} {
		if got := readOnly(strings.Fields(args)); got != want {
			t.Errorf("%s: readOnly=%v", args, got)
		}
	}
}

func TestStopControlCommand(t *testing.T) {
	t.Setenv("MW_FANFARE_PLAYER", "")
	t.Setenv("MW_FANFARE_STOP_CONTROL", "")
	none := func(string) (string, error) { return "", errors.New("no") }
	has := func(string) (string, error) { return "/x", nil }
	onlyK := func(n string) (string, error) {
		if n == "kdialog" {
			return "/k", nil
		}
		return "", errors.New("no")
	}
	hostile := "/tmp/a'\"$(touch x)/.mw-fanfare-icon.png"
	old := zenityBodyIcon
	defer func() { zenityBodyIcon = old }()
	zenityBodyIcon = func() bool { return true }
	evil := `Tenant "x'"$(touch x)` + "`;" + ` finished`
	script := func(a []string) string { return a[1] }
	n, a, env, ok := stopControlCommand("darwin", 4242, 60, evil, hostile, none)
	s := script(a)
	if !ok || n != "sh" || len(env) != 0 || !strings.Contains(s, "osascript") || !strings.Contains(s, "item 2 of argv as integer") || !strings.Contains(s, `kill -TERM -"$2"`) || !strings.Contains(s, "button returned:Stop") {
		t.Fatalf("darwin: %s %v", n, a)
	}
	if a[0] != "-c" || a[2] != "sh" || a[3] != evil || a[4] != "4242" || a[5] != "62" || len(a) != 7 || a[6] != hostile || !strings.Contains(s, "with icon (POSIX file (item 3 of argv))") || strings.Contains(s, "icon note") || strings.Contains(s, "touch") || strings.Contains(s, "4242") {
		t.Fatalf("darwin argv: %q", a)
	}
	_, a, _, _ = stopControlCommand("darwin", 4242, 60, evil, "", none)
	if !strings.Contains(a[1], "with icon note") || len(a) != 6 {
		t.Fatalf("darwin no icon: %q", a)
	}
	n, a, env, ok = stopControlCommand("windows", 77, 60, evil, hostile, none)
	s = a[2]
	if !ok || n != "powershell" || !strings.Contains(s, "$env:MW_FANFARE_MSG") || !strings.Contains(s, ",'miniwaves',64)") || strings.Contains(strings.Join(a, " "), "fanfare-icon") || !strings.Contains(s, ",62,") || !strings.Contains(s, "taskkill /PID 77 /T /F") || strings.Contains(s, "touch") || strings.Contains(strings.Join(a, " "), "touch") {
		t.Fatalf("windows: %s %v", n, a)
	}
	if len(env) != 1 || env[0] != "MW_FANFARE_MSG="+evil {
		t.Fatalf("windows env: %q", env)
	}
	n, a, env, ok = stopControlCommand("linux", 9, 10, evil, hostile, has)
	s = script(a)
	if !ok || n != "sh" || len(env) != 0 || !strings.Contains(s, "zenity") || !strings.Contains(s, "--no-markup") || !strings.Contains(s, `--text="$1"`) || !strings.Contains(s, `kill -TERM -"$2"`) || strings.Contains(s, "touch") || a[3] != evil || a[4] != "9" || a[5] != "12" || len(a) != 7 || a[6] != hostile || !strings.Contains(s, `--icon="$4"`) || strings.Contains(s, "window-icon") {
		t.Fatalf("linux zenity: %s %q", n, a)
	}
	zenityBodyIcon = func() bool { return false }
	_, a, _, _ = stopControlCommand("linux", 9, 10, evil, hostile, has)
	if len(a) != 6 || strings.Contains(a[1], "icon") || strings.Contains(strings.Join(a, " "), "fanfare-icon") {
		t.Fatalf("old zenity must get no icon: %q", a)
	}
	n, a, _, ok = stopControlCommand("linux", 9, 10, "", hostile, onlyK)
	s = script(a)
	if !ok || n != "sh" || strings.Contains(s, "zenity") || !strings.Contains(s, "kdialog") || !strings.Contains(s, `--msgbox "$1"`) || !strings.Contains(s, `sleep "$3"`) || !strings.Contains(s, `kill -TERM -"$2"`) || a[3] != "" || len(a) != 6 || strings.Contains(s, "icon") {
		t.Fatalf("linux kdialog: %s %q", n, a)
	}
	_, a, _, _ = stopControlCommand("linux", 9, 10, evil, hostile, onlyK)
	if s = script(a); strings.Contains(s, "touch") || a[3] != evil || a[4] != "9" || a[5] != "12" {
		t.Fatalf("kdialog argv: %q", a)
	}
	if _, _, _, ok = stopControlCommand("linux", 9, 10, "m", hostile, none); ok {
		t.Fatal("linux with neither zenity nor kdialog must have no control")
	}
	t.Setenv("MW_FANFARE_PLAYER", "/stub")
	if _, _, _, ok = stopControlCommand("darwin", 1, 5, "m", hostile, none); ok {
		t.Fatal("player stub must disable the control")
	}
	t.Setenv("MW_FANFARE_STOP_CONTROL", "/ctl")
	if n, a, _, ok = stopControlCommand("darwin", 1, 5, "hello", hostile, none); !ok || n != "/ctl" || len(a) != 3 || a[0] != "1" || a[1] != "5" || a[2] != "hello" {
		t.Fatalf("stub: %s %v", n, a)
	}
}

func TestFinaleIcon(t *testing.T) {
	dir := t.TempDir()
	pair := filepath.Join(dir, "pair")
	if err := os.MkdirAll(pair, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(pair, "finale.mp3")
	want := filepath.Join(dir, ".mw-fanfare-icon.png")
	if got := finaleIcon(dir, file); got != want {
		t.Fatalf("built-in: %s", got)
	}
	if b, _ := os.ReadFile(want); !bytes.Equal(b, fanfareIconPNG) || len(b) < 100 || len(b) > 30000 {
		t.Fatal("built-in icon not written or wrong size")
	}
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	os.Chtimes(want, old, old)
	finaleIcon(dir, file)
	if st, _ := os.Stat(want); !st.ModTime().Equal(old) {
		t.Fatal("identical built-in icon was rewritten")
	}
	os.WriteFile(want, []byte("junk"), 0o644)
	finaleIcon(dir, file)
	if b, _ := os.ReadFile(want); !bytes.Equal(b, fanfareIconPNG) {
		t.Fatal("differing icon not restored")
	}
	for _, n := range []string{"cover.icns", "cover.jpg", "cover.png"} { // each later in the loop outranks the former
		os.WriteFile(filepath.Join(pair, n), []byte("x"), 0o644)
		if got := finaleIcon(dir, file); got != filepath.Join(pair, n) {
			t.Fatalf("%s: got %s", n, got)
		}
	}
	os.Remove(filepath.Join(pair, "cover.png"))
	if got := finaleIcon(dir, file); got != filepath.Join(pair, "cover.jpg") {
		t.Fatalf("jpg before icns: %s", got)
	}
}

func TestFinaleReasonAndSong(t *testing.T) {
	tn := &Tenant{Slug: "infotool", Name: "infotool", Items: []Item{{Status: "done"}, {Status: "done"}, {Status: "cut"}}}
	if got := finaleReason(tn); got != "Tenant \"infotool\" finished \u2014 2 items done" {
		t.Fatal(got)
	}
	tn = &Tenant{Slug: "a", Name: "Alpha", Items: []Item{{Status: "done"}}}
	if got := finaleReason(tn); got != "Tenant \"a\" (Alpha) finished \u2014 1 item done" {
		t.Fatal(got)
	}
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "p1"), 0o755)
	os.MkdirAll(filepath.Join(d, "p2"), 0o755)
	f1, f2 := filepath.Join(d, "p1", "finale.mp3"), filepath.Join(d, "p2", "finale.ogg")
	if got := finaleSong(d, f2); got != "finale (p2)" {
		t.Fatal(got)
	}
	m := fanfareManifest{Pairs: []manifestPair{{Name: "p1", Tracks: []manifestTrack{{Kind: "finale", File: "finale.mp3", Title: "Pomp and Circumstance No. 1", Attribution: "US Marine Band"}}}}}
	b, _ := json.Marshal(m)
	os.WriteFile(filepath.Join(d, manifestName), b, 0o644)
	if got := finaleSong(d, f1); got != "Pomp and Circumstance No. 1 \u2014 US Marine Band" {
		t.Fatal(got)
	}
	m.Pairs[0].Tracks[0].Attribution = strings.Repeat("long ", 20)
	b, _ = json.Marshal(m)
	os.WriteFile(filepath.Join(d, manifestName), b, 0o644)
	if got := finaleSong(d, f1); got != "Pomp and Circumstance No. 1" {
		t.Fatal(got)
	}
	msg := finaleMessage(finaleTestReason, "Song X", 60)
	if msg != "Test (mw fanfare test)\nSong: Song X\nPlaying 60 s" {
		t.Fatal(msg)
	}
}

func TestFinaleStubReceivesMessage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh stub")
	}
	d := t.TempDir()
	out := filepath.Join(d, "args.txt")
	stub := filepath.Join(d, "ctl.sh")
	os.WriteFile(stub, []byte("#!/bin/sh\nprintf '%s|%s|%s' \"$1\" \"$2\" \"$3\" > "+out+"\n"), 0o755)
	slow := filepath.Join(d, "slow.sh")
	os.WriteFile(slow, []byte("#!/bin/sh\nsleep 30\n"), 0o755)
	t.Setenv("MW_FANFARE_PLAYER", slow)
	t.Setenv("MW_FANFARE_STOP_CONTROL", stub)
	dir := filepath.Join(d, "songs")
	os.MkdirAll(filepath.Join(dir, "pp"), 0o755)
	if err := playFinale(dir, filepath.Join(dir, "pp", "finale.mp3"), 5, 1, true, finaleTestReason); err != nil {
		t.Fatal(err)
	}
	defer stopPlayers(dir)
	var got []byte
	for i := 0; i < 60 && len(got) == 0; i++ {
		time.Sleep(50 * time.Millisecond)
		got, _ = os.ReadFile(out)
	}
	if !strings.HasSuffix(string(got), "|5|Test (mw fanfare test)\nSong: finale (pp)\nPlaying 5 s") {
		t.Fatalf("stub got %q", got)
	}
}

func TestFinaleStopControlLaunchedAndRecorded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh stub")
	}
	d := t.TempDir()
	slow := filepath.Join(d, "slow.sh")
	os.WriteFile(slow, []byte("#!/bin/sh\nsleep 30\n"), 0o755)
	t.Setenv("MW_FANFARE_PLAYER", slow)
	t.Setenv("MW_FANFARE_STOP_CONTROL", slow)
	dir := filepath.Join(d, "songs")
	os.MkdirAll(dir, 0o755)
	if err := playFinale(dir, "/x/f.mp3", 5, 1, true, "r"); err != nil {
		t.Fatal(err)
	}
	if rs := readPlayers(dir); len(rs) != 2 {
		t.Fatalf("want player + control recorded: %+v", rs)
	}
	pids := []int{}
	for _, r := range readPlayers(dir) {
		pids = append(pids, r.PID)
	}
	if n := stopPlayers(dir); n != 2 {
		t.Fatalf("stopped %d", n)
	}
	for _, pid := range pids {
		for i := 0; i < 40 && procStamp(pid) != "" && !strings.Contains(procState(pid), "Z"); i++ {
			time.Sleep(50 * time.Millisecond)
		}
		if procStamp(pid) != "" && !strings.Contains(procState(pid), "Z") {
			t.Fatalf("pid %d still running", pid)
		}
	}
	// milestone (playFile) and stop_dialog=false launch no control
	if err := playFile(dir, "/x/f.mp3", 5, 1); err != nil {
		t.Fatal(err)
	}
	if err := playFinale(dir, "/x/f.mp3", 5, 1, false, "r"); err != nil {
		t.Fatal(err)
	}
	if rs := readPlayers(dir); len(rs) != 2 {
		t.Fatalf("want 2 players, no control: %+v", rs)
	}
	stopPlayers(dir)
}

func TestStopDialogConfig(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "Ws"})
	w, _ := load(path)
	if c := fanfareCfg(w); *c.StopDialog != true || c.FinaleSeconds != 60 {
		t.Fatalf("defaults: %+v", c)
	}
	runAll(t, []string{"fanfare", "set", "stop_dialog=false"})
	w, _ = load(path)
	if c := fanfareCfg(w); *c.StopDialog {
		t.Fatal("stop_dialog=false not stored")
	}
	if got := captureOutput(t, func() { runAll(t, []string{"fanfare"}) }); !strings.Contains(got, "stop_dialog: false") {
		t.Fatal(got)
	}
}

// fakeCacheDir points os.UserCacheDir at a temp folder and returns it.
func fakeCacheDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	t.Setenv("HOME", d)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(d, "xdg"))
	t.Setenv("LocalAppData", filepath.Join(d, "local"))
	c, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestBuiltinFanfareCachedByHash(t *testing.T) {
	cache := fakeCacheDir(t)
	f, err := builtinFanfareFile()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(defaultFanfareWAV)
	want := filepath.Join(cache, "miniwaves", "fanfare-default-"+hex.EncodeToString(sum[:])[:12]+".wav")
	if f != want {
		t.Fatalf("path %s, want %s", f, want)
	}
	if b, _ := os.ReadFile(f); !bytes.Equal(b, defaultFanfareWAV) {
		t.Fatal("content differs")
	}
	// a damaged cache file is rewritten; no temp files are left behind
	if err := os.WriteFile(f, []byte("junk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if f2, err := builtinFanfareFile(); err != nil || f2 != f {
		t.Fatalf("%v %s", err, f2)
	}
	if b, _ := os.ReadFile(f); !bytes.Equal(b, defaultFanfareWAV) {
		t.Fatal("damaged cache not repaired")
	}
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(f), "*.tmp")); len(m) != 0 {
		t.Fatalf("temp left: %v", m)
	}
}

func TestBuiltinFanfareWhenNoPairs(t *testing.T) {
	fakeCacheDir(t)
	if len(defaultFanfareWAV) < 100<<10 || len(defaultFanfareWAV) > 200<<10 || string(defaultFanfareWAV[:4]) != "RIFF" {
		t.Fatalf("embedded wav: %d bytes", len(defaultFanfareWAV))
	}
	old := lookPath
	t.Cleanup(func() { lookPath = old })
	lookPath = func(n string) (string, error) { return "/bin/" + n, nil }
	p, err := pickPairOrBuiltin(FanfareConfig{}, t.TempDir(), func(int) int { return 0 })
	if err != nil || p.Name != "built-in" || !strings.HasSuffix(p.Fanfare, ".wav") || p.Finale != p.Fanfare {
		t.Fatalf("pair %+v err %v", p, err)
	}
	if b, _ := os.ReadFile(p.Fanfare); len(b) != len(defaultFanfareWAV) {
		t.Fatal("wav not written")
	}
	if runtime.GOOS == "windows" {
		return // Windows plays through the built-in PowerShell SoundPlayer, not a looked-up player
	}
	lookPath = func(string) (string, error) { return "", errors.New("none") }
	if _, err := pickPairOrBuiltin(FanfareConfig{}, t.TempDir(), func(int) int { return 0 }); err == nil {
		t.Fatal("want error with no player")
	}
}
