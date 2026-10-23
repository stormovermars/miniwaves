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
	if !strings.Contains(string(b), `"fanfare":{"enabled":true,"dir":`+string(sj)+`,"milestone_seconds":20,"finale_seconds":240,"volume":0.5}`) {
		t.Fatalf("not stored: %s", b)
	}
	w, _ := load(path)
	if c := fanfareCfg(w); !fanfareOn(c) || c.Dir != songs || c.MilestoneSeconds != 20 || c.FinaleSeconds != 240 || c.Volume != 0.5 {
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

func TestFinaleReason(t *testing.T) {
	tn := &Tenant{Slug: "infotool", Name: "infotool", Items: []Item{{Status: "done"}, {Status: "done"}, {Status: "cut"}}}
	if got := finaleReason(tn); got != "Tenant \"infotool\" finished \u2014 2 items done" {
		t.Fatal(got)
	}
	tn = &Tenant{Slug: "a", Name: "Alpha", Items: []Item{{Status: "done"}}}
	if got := finaleReason(tn); got != "Tenant \"a\" (Alpha) finished \u2014 1 item done" {
		t.Fatal(got)
	}
}

func TestStopDialogDeprecatedAndIgnored(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh stub")
	}
	path := inTempDir(t)
	runAll(t, []string{"init", "Ws"})
	var out string
	errOut := captureStderr(t, func() {
		out = captureOutput(t, func() { runAll(t, []string{"fanfare", "set", "stop_dialog=false"}) })
	})
	if !strings.Contains(errOut, "stop_dialog is deprecated and ignored since 0.3.6") || !strings.Contains(errOut, "mw fanfare stop") || strings.Contains(out, "stop_dialog") {
		t.Fatalf("stderr %q stdout %q", errOut, out)
	}
	w, _ := load(path)
	if got := captureOutput(t, func() { runAll(t, []string{"fanfare"}) }); strings.Contains(got, "stop_dialog") {
		t.Fatal(got)
	}
	got := captureOutput(t, func() { runAll(t, []string{"fanfare", "-j"}) })
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatal(err, got)
	}
	if m["stop_dialog_deprecated"] != true || m["playing"] != float64(0) || m["playing_finale"] != false {
		t.Fatalf("json: %s", got)
	}
	if strings.Contains(got, `"stop_dialog":`) {
		t.Fatalf("stop_dialog shown as live: %s", got)
	}
	if w.Fanfare == nil || w.Fanfare.StopDialog == nil || *w.Fanfare.StopDialog {
		t.Fatal("key must still parse")
	}
	// a finale starts exactly one process: no dialog
	d := t.TempDir()
	slow := filepath.Join(d, "slow.sh")
	os.WriteFile(slow, []byte("#!/bin/sh\nsleep 30\n"), 0o755)
	t.Setenv("MW_FANFARE_PLAYER", slow)
	dir := filepath.Join(d, "songs")
	os.MkdirAll(dir, 0o755)
	if err := playFinale(dir, "/x/f.mp3", 5, 1); err != nil {
		t.Fatal(err)
	}
	defer stopPlayers(dir)
	if rs := readPlayers(dir); len(rs) != 1 || rs[0].Kind != "finale" {
		t.Fatalf("want one finale player: %+v", rs)
	}
}

func TestFanfarePlayingKeys(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh stub")
	}
	d := t.TempDir()
	slow := filepath.Join(d, "slow.sh")
	os.WriteFile(slow, []byte("#!/bin/sh\nsleep 30\n"), 0o755)
	t.Setenv("MW_FANFARE_PLAYER", slow)
	dir := filepath.Join(d, "songs")
	os.MkdirAll(dir, 0o755)
	if n, f := playingState(dir); n != 0 || f {
		t.Fatal(n, f)
	}
	if err := playFile(dir, "/x/f.mp3", 5, 1); err != nil {
		t.Fatal(err)
	}
	if n, f := playingState(dir); n != 1 || f {
		t.Fatalf("milestone: %d %v", n, f)
	}
	if err := playFinale(dir, "/x/f.mp3", 5, 1); err != nil {
		t.Fatal(err)
	}
	rs := readPlayers(dir)
	writePlayers(dir, append(rs, playerRec{PID: 1, Stamp: "stale", Kind: "finale"}))
	before, _ := os.ReadFile(playersPath(dir))
	if n, f := playingState(dir); n != 2 || !f {
		t.Fatalf("both: %d %v", n, f)
	}
	if after, _ := os.ReadFile(playersPath(dir)); !bytes.Equal(before, after) {
		t.Fatal("playingState rewrote the file")
	}
	if rs[0].Kind != "milestone" || rs[1].Kind != "finale" {
		t.Fatalf("kinds: %+v", rs)
	}
	stopPlayers(dir)
	if n, f := playingState(dir); n != 0 || f {
		t.Fatal(n, f)
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

// TestNoSystemDialogsInSource guards TSK-067: mw never opens a system dialog or notification of its own.
// Sound players (afplay, ffplay, paplay, aplay, PowerShell SoundPlayer) are fine.
func TestNoSystemDialogsInSource(t *testing.T) {
	banned := []string{"osascript", "zenity", "kdialog", "MessageBox", "[System.Windows.Forms", "WScript.Shell", ".Popup(", `"msg"`, "msg.exe", "display dialog", "display notification"}
	var files []string
	for _, pat := range []string{"*.go", "kit/*", "kit/*/*", "scripts/*"} {
		m, _ := filepath.Glob(pat)
		files = append(files, m...)
	}
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		if st, err := os.Stat(f); err != nil || !st.Mode().IsRegular() {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		src := string(b)
		for _, w := range banned {
			if strings.Contains(src, w) {
				t.Errorf("%s contains %q: mw must not open system dialogs or notifications", f, w)
			}
		}
		if strings.HasSuffix(f, ".go") && (strings.Contains(src, `"msg "`) || strings.Contains(src, "msg * ")) {
			t.Errorf("%s runs the Windows msg command", f)
		}
	}
	if checked < 20 {
		t.Fatalf("scanned only %d files", checked)
	}
}
