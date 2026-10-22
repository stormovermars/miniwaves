package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Self-update from GitHub releases: stdlib only, no token.
const (
	repoName    = "miniwaves"
	binName     = "mw"
	releasesURL = "https://github.com/stormovermars/" + repoName + "/releases"
	installHint = "curl -fsSL https://raw.githubusercontent.com/stormovermars/miniwaves/main/install.sh | sh   (Windows: irm https://raw.githubusercontent.com/stormovermars/miniwaves/main/install.ps1 | iex)\nor: claude plugin marketplace update miniwaves && claude plugin update mw@miniwaves"
)

var (
	releaseAPI   = "https://api.github.com/repos/stormovermars/" + repoName + "/releases/latest"
	updateClient = &http.Client{Timeout: 5 * time.Second}
	exePath      = os.Executable
	// assetClient has a long overall timeout: the asset is several MB behind a redirect.
	assetClient = &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return assetURLCheck(r.URL)
	}}
	// assetURLCheck pins download URLs to GitHub (tests swap it for the httptest host).
	assetURLCheck = func(u *url.URL) error {
		if u.Scheme == "https" && (u.Host == "github.com" || u.Host == "objects.githubusercontent.com" || u.Host == "release-assets.githubusercontent.com") {
			return nil
		}
		return fmt.Errorf("refusing download from %s", u.Redacted())
	}
	// swapOld: Windows cannot overwrite the running exe, so it moves to exe.old first.
	swapOld = runtime.GOOS == "windows"
)

type ghRelease struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func latestRelease() (*ghRelease, error) {
	b, err := httpGet(releaseAPI)
	if err != nil {
		return nil, err
	}
	var r ghRelease
	if err := json.Unmarshal(b, &r); err != nil || r.Tag == "" {
		return nil, fmt.Errorf("unexpected release data from %s", releaseAPI)
	}
	return &r, nil
}

func httpGet(url string) ([]byte, error) { return getWith(updateClient, url) }

// getAsset downloads a release asset: GitHub hosts only (redirects included), long timeout.
func getAsset(u string) ([]byte, error) {
	pu, err := url.Parse(u)
	if err != nil {
		return nil, err
	}
	if err := assetURLCheck(pu); err != nil {
		return nil, err
	}
	return getWith(assetClient, u)
}

func getWith(c *http.Client, url string) ([]byte, error) {
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 200<<20))
}

// assetName is the release.yml name for this platform, e.g. miniwaves_Darwin_arm64.tar.gz.
func assetName(goos, goarch string) string {
	osn := map[string]string{"darwin": "Darwin", "linux": "Linux", "windows": "Windows"}[goos]
	arch := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[goarch]
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return repoName + "_" + osn + "_" + arch + ext
}

// newerVersion reports whether b is a newer x.y.z than a.
func newerVersion(a, b string) bool {
	pa, pb := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < 3; i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return y > x
		}
	}
	return false
}

func updateCommand(args []string) error {
	check := len(args) == 1 && args[0] == "--check"
	if len(args) == 1 && args[0] == "--plugins" {
		installPlugins(os.Stdout)
		return nil
	}
	if len(args) > 0 && !check {
		return errors.New("usage: " + binName + " update [--check] [--plugins]")
	}
	rel, err := latestRelease()
	if err != nil {
		return fmt.Errorf("cannot read the latest release (%v); see %s", err, releasesURL)
	}
	latest := strings.TrimPrefix(rel.Tag, "v")
	if !newerVersion(version, latest) {
		fmt.Printf("%s %s is current (latest release %s)\n", binName, version, latest)
		return &exitError{code: 2, msg: "already current (" + version + "); nothing to update", quiet: true}
	}
	if check {
		fmt.Printf("update available: %s → %s (%s update)\n", version, latest, binName)
		return nil
	}
	bin, err := fetchVerified(rel, assetName(runtime.GOOS, runtime.GOARCH))
	if err != nil {
		return err
	}
	exe, err := exePath()
	if err != nil {
		return err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	if err := replaceExe(exe, bin); err != nil {
		return fmt.Errorf("cannot replace %s (%v); install the new version with:\n%s", exe, err, installHint)
	}
	fmt.Printf("%s %s → %s (%s)\n", binName, version, latest, exe)
	installPlugins(os.Stdout)
	return nil
}

// Claude Code plugins: best effort, never changes the exit status.
var (
	pluginNames = []string{"mw", "tidewatch"}
	claudeFound = func() bool { _, err := exec.LookPath("claude"); return err == nil }
	// claudeRun runs `claude args...` with a timeout and returns its combined output.
	claudeRun = func(args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		b, err := exec.CommandContext(ctx, "claude", args...).CombinedOutput()
		if ctx.Err() != nil {
			return string(b), errors.New("timed out")
		}
		return string(b), err
	}
)

func lastLine(s string, err error) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		s = strings.TrimSpace(s[i+1:])
	}
	if s == "" && err != nil {
		s = err.Error()
	}
	return s
}

// installPlugins installs or updates the mw and tidewatch plugins from the miniwaves marketplace.
func installPlugins(w io.Writer) {
	if os.Getenv("MW_NO_PLUGINS") == "1" {
		fmt.Fprintln(w, "plugins: skipped (MW_NO_PLUGINS=1)")
		return
	}
	if !claudeFound() {
		fmt.Fprintln(w, "plugins: claude not found; later run: mw update --plugins")
		return
	}
	// "already added" is fine: ignore the add result, then refresh the catalog.
	claudeRun("plugin", "marketplace", "add", "stormovermars/"+repoName)
	if o, err := claudeRun("plugin", "marketplace", "update", repoName); err != nil {
		fmt.Fprintf(w, "plugins: failed: marketplace update: %s\n", lastLine(o, err))
		return
	}
	listing, _ := claudeRun("plugin", "list")
	for _, p := range pluginNames {
		id := p + "@" + repoName
		verb, args := "installed", []string{"plugin", "install", id}
		for _, l := range strings.Split(listing, "\n") {
			if strings.HasSuffix(strings.TrimSpace(l), " "+id) || strings.TrimSpace(l) == id {
				verb, args = "updated", []string{"plugin", "update", id}
			}
		}
		if o, err := claudeRun(args...); err != nil {
			fmt.Fprintf(w, "plugin %s: failed: %s\n", p, lastLine(o, err))
		} else {
			fmt.Fprintf(w, "plugin %s: %s\n", p, verb)
		}
	}
}

// fetchVerified downloads the asset and checksums.txt, checks the sha256 and returns the binary.
func fetchVerified(rel *ghRelease, name string) ([]byte, error) {
	urls := map[string]string{}
	for _, a := range rel.Assets {
		urls[a.Name] = a.URL
	}
	if urls[name] == "" || urls["checksums.txt"] == "" {
		return nil, fmt.Errorf("release %s has no %s (or checksums.txt); see %s", rel.Tag, name, releasesURL)
	}
	arc, err := getAsset(urls[name])
	if err != nil {
		return nil, err
	}
	sums, err := getAsset(urls["checksums.txt"])
	if err != nil {
		return nil, err
	}
	want := ""
	for _, l := range strings.Split(string(sums), "\n") {
		if f := strings.Fields(l); len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			want = strings.ToLower(f[0])
		}
	}
	got := sha256.Sum256(arc)
	if want == "" || want != hex.EncodeToString(got[:]) {
		return nil, fmt.Errorf("checksum mismatch for %s: not installed", name)
	}
	return extractBinary(arc, strings.HasSuffix(name, ".zip"))
}

func extractBinary(arc []byte, isZip bool) ([]byte, error) {
	if isZip {
		zr, err := zip.NewReader(bytes.NewReader(arc), int64(len(arc)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if f.Name == binName+".exe" {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, errors.New(binName + ".exe not in the archive")
	}
	gz, err := gzip.NewReader(bytes.NewReader(arc))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			return nil, errors.New(binName + " not in the archive")
		}
		if h.Name == binName || h.Name == "./"+binName {
			return io.ReadAll(tr)
		}
	}
}

// replaceExe writes the new binary next to exe and renames it over; on Windows the running
// exe cannot be overwritten but can be renamed, so it moves to exe.old first.
func replaceExe(exe string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(exe), "."+filepath.Base(exe)+".*.new")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(tmp, 0o755)
	}
	if werr != nil {
		os.Remove(tmp)
		return werr
	}
	if swapOld {
		_ = os.Remove(exe + ".old")
		if err := renameRetry(exe, exe+".old"); err != nil {
			os.Remove(tmp)
			return err
		}
		if err := renameRetry(tmp, exe); err != nil {
			os.Remove(tmp)
			if rerr := renameRetry(exe+".old", exe); rerr != nil {
				return fmt.Errorf("%v; restoring %s.old failed too: %v", err, exe, rerr)
			}
			return err
		}
		return nil
	}
	if err := renameRetry(tmp, exe); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
