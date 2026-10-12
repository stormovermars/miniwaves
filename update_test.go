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
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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
		"https://github.com/x/y/releases/download/v1/a.tar.gz": true,
		"https://objects.githubusercontent.com/a":              true,
		"http://github.com/a":                                  false,
		"https://evil.example/a":                               false,
		"https://github.com.evil.example/a":                    false,
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
