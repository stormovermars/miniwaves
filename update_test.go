package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeRelease serves /latest, the platform asset (holding payload) and checksums.txt.
func fakeRelease(t *testing.T, tag string, payload []byte, badSum bool) {
	t.Helper()
	name := assetName(runtime.GOOS, runtime.GOARCH)
	var arc bytes.Buffer
	if runtime.GOOS == "windows" {
		zw := zip.NewWriter(&arc)
		w, _ := zw.Create(binName + ".exe")
		w.Write(payload)
		zw.Close()
	} else {
		gz := gzip.NewWriter(&arc)
		tw := tar.NewWriter(gz)
		tw.WriteHeader(&tar.Header{Name: binName, Mode: 0o755, Size: int64(len(payload))})
		tw.Write(payload)
		tw.Close()
		gz.Close()
	}
	sum := sha256.Sum256(arc.Bytes())
	if badSum {
		sum[0] ^= 0xff
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			fmt.Fprintf(w, `{"tag_name":%q,"assets":[{"name":%q,"browser_download_url":"%s/a"},{"name":"checksums.txt","browser_download_url":"%s/sums"}]}`, tag, name, srv.URL, srv.URL)
		case "/a":
			w.Write(arc.Bytes())
		case "/sums":
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), name)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	oldAPI, oldExe, oldChk := releaseAPI, exePath, assetURLCheck
	t.Cleanup(func() { releaseAPI, exePath, assetURLCheck = oldAPI, oldExe, oldChk })
	assetURLCheck = func(u *url.URL) error {
		if u.Host == srv.Listener.Addr().String() {
			return nil
		}
		return fmt.Errorf("refusing %s", u.Host)
	}
	releaseAPI = srv.URL + "/latest"
}

func fakeExe(t *testing.T) string {
	exe := filepath.Join(t.TempDir(), binName)
	os.WriteFile(exe, []byte("old"), 0o755)
	exePath = func() (string, error) { return exe, nil }
	fakeClaude(t, false, "", "") // never touch the real claude
	return exe
}

func TestUpdateReplacesBinary(t *testing.T) {
	fakeRelease(t, "v9.9.9", []byte("new binary"), false)
	exe := fakeExe(t)
	if s := out(t, "update", "--check"); s != "update available: "+version+" → 9.9.9 (mw update)\n" {
		t.Fatalf("--check: %q", s)
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Fatal("--check replaced the binary")
	}
	out(t, "update")
	if b, _ := os.ReadFile(exe); string(b) != "new binary" {
		t.Fatalf("binary %q", b)
	}
}

func TestUpdateCurrentExitsTwo(t *testing.T) {
	fakeRelease(t, "v"+version, []byte("x"), false)
	fakeExe(t)
	var ee *exitError
	if err := run([]string{"update"}); !errors.As(err, &ee) || ee.code != 2 {
		t.Fatalf("want exit 2, got %v", err)
	}
}

func TestUpdateBadChecksumRefused(t *testing.T) {
	fakeRelease(t, "v9.9.9", []byte("evil"), true)
	exe := fakeExe(t)
	if err := run([]string{"update"}); err == nil {
		t.Fatal("bad checksum accepted")
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Fatal("binary replaced despite bad checksum")
	}
}

func TestVersionLinks(t *testing.T) {
	if s := out(t, "version"); !bytes.Contains([]byte(s), []byte(releasesURL+" · update: mw update")) {
		t.Fatalf("version: %q", s)
	}
}

func TestAssetURLPinned(t *testing.T) {
	for u, ok := range map[string]bool{
		"https://github.com/x/y/releases/download/v1/a.tar.gz":                             true,
		"https://objects.githubusercontent.com/a":                                          true,
		"https://release-assets.githubusercontent.com/github-production-release-asset/1/a": true,
		"http://github.com/a":                                         false,
		"http://release-assets.githubusercontent.com/a":               false,
		"https://evil.example/a":                                      false,
		"https://evilgithubusercontent.com/a":                         false,
		"https://github.com.evil.example/a":                           false,
		"https://release-assets.githubusercontent.com.evil.example/a": false,
		"https://x.release-assets.githubusercontent.com/a":            false,
	} {
		pu, _ := url.Parse(u)
		if err := assetURLCheck(pu); (err == nil) != ok {
			t.Errorf("%s: err %v, want ok=%v", u, err, ok)
		}
	}
	if _, err := getAsset("https://evil.example/a"); err == nil {
		t.Error("getAsset accepted a foreign host")
	}
}

// A failed second rename puts the old exe back.
func TestReplaceExeRollback(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "tool.exe")
	os.WriteFile(exe, []byte("old"), 0o755)
	oldFn, oldSwap, oldOn := renameFn, swapOld, renameRetryOn
	t.Cleanup(func() { renameFn, swapOld, renameRetryOn = oldFn, oldSwap, oldOn })
	swapOld, renameRetryOn = true, false
	n := 0
	renameFn = func(a, b string) error {
		if n++; n == 2 {
			return errors.New("sharing violation")
		}
		return os.Rename(a, b)
	}
	if err := replaceExe(exe, []byte("new")); err == nil {
		t.Fatal("want error")
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Fatalf("exe %q after failed swap", b)
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "*.new")); len(m) != 0 {
		t.Fatalf("temp left: %v", m)
	}
}

// fakeClaude swaps the claude runner; calls collects the argument lines.
func fakeClaude(t *testing.T, found bool, fail string, installed string) *[]string {
	t.Helper()
	var calls []string
	oF, oR := claudeFound, claudeRun
	t.Cleanup(func() { claudeFound, claudeRun = oF, oR })
	claudeFound = func() bool { return found }
	claudeRun = func(args ...string) (string, error) {
		line := strings.Join(args, " ")
		calls = append(calls, line)
		if fail != "" && strings.Contains(line, fail) {
			return "boom: bad thing\n", errors.New("exit 1")
		}
		if line == "plugin list" {
			return "Installed plugins:\n\n  ❯ " + installed + "\n    Version: 1\n", nil
		}
		return "", nil
	}
	return &calls
}

func TestUpdateRunsPluginStep(t *testing.T) {
	fakeRelease(t, "v9.9.9", []byte("new binary"), false)
	fakeExe(t)
	calls := fakeClaude(t, true, "", "mw@miniwaves")
	t.Setenv("MW_NO_PLUGINS", "")
	s := out(t, "update")
	for _, want := range []string{"plugin mw: updated", "plugin mw-jobs: installed"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in %q", want, s)
		}
	}
	all := strings.Join(*calls, "|")
	for _, want := range []string{"plugin marketplace add stormovermars/miniwaves", "plugin marketplace update miniwaves", "plugin update mw@miniwaves", "plugin install mw-jobs@miniwaves"} {
		if !strings.Contains(all, want) {
			t.Fatalf("missing call %q in %q", want, all)
		}
	}
}

func TestUpdatePluginsOptOut(t *testing.T) {
	fakeRelease(t, "v9.9.9", []byte("new binary"), false)
	fakeExe(t)
	calls := fakeClaude(t, true, "", "")
	t.Setenv("MW_NO_PLUGINS", "1")
	if s := out(t, "update"); !strings.Contains(s, "plugins: skipped") || len(*calls) != 0 {
		t.Fatalf("out %q calls %v", s, *calls)
	}
}

func TestUpdatePluginsNoClaudeHint(t *testing.T) {
	t.Setenv("MW_NO_PLUGINS", "")
	calls := fakeClaude(t, false, "", "")
	if s := out(t, "update", "--plugins"); !strings.Contains(s, "claude not found; later run: mw update --plugins") || len(*calls) != 0 {
		t.Fatalf("out %q calls %v", s, *calls)
	}
}

func TestUpdatePluginFailureKeepsExit(t *testing.T) {
	fakeRelease(t, "v9.9.9", []byte("new binary"), false)
	exe := fakeExe(t)
	fakeClaude(t, true, "install mw-jobs", "")
	t.Setenv("MW_NO_PLUGINS", "")
	if err := run([]string{"update"}); err != nil {
		t.Fatalf("exit changed: %v", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new binary" {
		t.Fatal("binary not replaced")
	}
	fakeClaude(t, true, "marketplace update", "")
	if s := out(t, "update", "--plugins"); !strings.Contains(s, "failed: marketplace update: boom: bad thing") {
		t.Fatalf("out %q", s)
	}
}

func TestUpdatePluginsAlone(t *testing.T) {
	t.Setenv("MW_NO_PLUGINS", "")
	calls := fakeClaude(t, true, "", "mw-jobs@miniwaves")
	s := out(t, "update", "--plugins")
	if !strings.Contains(s, "plugin mw: installed") || !strings.Contains(s, "plugin mw-jobs: updated") || len(*calls) == 0 {
		t.Fatalf("out %q", s)
	}
}

func TestUpdateRemovesOldTidewatch(t *testing.T) {
	t.Setenv("MW_NO_PLUGINS", "")
	calls := fakeClaude(t, true, "", "tidewatch@miniwaves")
	s := out(t, "update", "--plugins")
	if !strings.Contains(s, "plugin tidewatch: removed (replaced by mw-jobs)") || !strings.Contains(s, "plugin mw-jobs: installed") {
		t.Fatalf("out %q", s)
	}
	if !strings.Contains(strings.Join(*calls, "|"), "plugin uninstall tidewatch@miniwaves") {
		t.Fatalf("calls %v", *calls)
	}
	calls = fakeClaude(t, true, "", "mw@miniwaves")
	out(t, "update", "--plugins")
	if strings.Contains(strings.Join(*calls, "|"), "uninstall") {
		t.Fatalf("uninstall without tidewatch: %v", *calls)
	}
}

func init() { fanfareDefaults = func(io.Writer) {} } // never re-run the test binary as mw

func TestUpdateRunsFanfareDefaults(t *testing.T) {
	fakeRelease(t, "v9.9.9", []byte("new binary"), false)
	fakeExe(t)
	fakeClaude(t, false, "", "")
	n := 0
	old := fanfareDefaults
	t.Cleanup(func() { fanfareDefaults = old })
	fanfareDefaults = func(io.Writer) { n++ }
	out(t, "update")
	out(t, "update", "--plugins")
	if n != 2 {
		t.Fatalf("fanfareDefaults ran %d times, want 2", n)
	}
}

// indexOf returns the position of the first call containing sub, or -1.
func indexOf(calls []string, sub string) int {
	for i, c := range calls {
		if strings.Contains(c, sub) {
			return i
		}
	}
	return -1
}

func TestUpdateInstallFailureKeepsOldPlugins(t *testing.T) {
	t.Setenv("MW_NO_PLUGINS", "")
	calls := fakeClaude(t, true, "install mw-jobs", "tidewatch@miniwaves\n  mw-watch@miniwaves")
	out(t, "update", "--plugins")
	if indexOf(*calls, "uninstall") >= 0 {
		t.Fatalf("uninstall after a failed install: %v", *calls)
	}
}

func TestUpdateInstallSuccessUninstallsOldPlugins(t *testing.T) {
	t.Setenv("MW_NO_PLUGINS", "")
	calls := fakeClaude(t, true, "", "tidewatch@miniwaves\n  mw-watch@miniwaves")
	s := out(t, "update", "--plugins")
	u1, u2 := indexOf(*calls, "plugin uninstall tidewatch@miniwaves"), indexOf(*calls, "plugin uninstall mw-watch@miniwaves")
	i1, i2 := indexOf(*calls, "plugin install mw@miniwaves"), indexOf(*calls, "plugin install mw-jobs@miniwaves")
	if u1 < 0 || u2 < 0 || i1 < 0 || i2 < 0 || u1 < i2 || u2 < i2 || u1 < i1 {
		t.Fatalf("order wrong: %v", *calls)
	}
	if !strings.Contains(s, "plugin mw-watch: removed") {
		t.Fatalf("out %q", s)
	}
}

func TestUpdateClaudeConfigDirReachesClaude(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	dir := t.TempDir()
	cfg := filepath.Join(dir, "cfg")
	log := filepath.Join(dir, "seen")
	stub := "#!/bin/sh\necho \"$CLAUDE_CONFIG_DIR\" >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	t.Setenv("MW_NO_PLUGINS", "")
	out(t, "update", "--plugins")
	b, _ := os.ReadFile(log)
	lines := strings.Fields(string(b))
	if len(lines) == 0 {
		t.Fatal("stub claude never ran")
	}
	for _, l := range lines {
		if l != cfg {
			t.Fatalf("claude saw CLAUDE_CONFIG_DIR=%q, want %q", l, cfg)
		}
	}
}
