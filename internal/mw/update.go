package mw

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

func latestRelease() (*ghRelease, error) { return releaseFrom(releaseAPI) }

func releaseFrom(api string) (*ghRelease, error) {
	b, err := httpGet(api)
	if err != nil {
		return nil, err
	}
	var r ghRelease
	if err := json.Unmarshal(b, &r); err != nil || r.Tag == "" {
		return nil, fmt.Errorf("unexpected release data from %s", api)
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
func assetName(goos, goarch string) string { return assetFor(repoName, goos, goarch) }

// assetFor is the release asset name of repo (miniwaves or wr) for a platform.
func assetFor(repo, goos, goarch string) string {
	osn := map[string]string{"darwin": "Darwin", "linux": "Linux", "windows": "Windows"}[goos]
	arch := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[goarch]
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return repo + "_" + osn + "_" + arch + ext
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
		fanfareDefaults(os.Stdout)
		installPlugins(os.Stdout)
		return nil
	}
	if len(args) > 0 && !check {
		return errors.New("usage: " + binName + " update [--check] [--plugins]")
	}
	exe, err := exePath()
	if err != nil {
		return err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	changed, err := updateSelf(exe, check)
	// wr is updated with mw (latest release of each); its failure is a note, never mw's exit status.
	if updatePartner(os.Stdout, exe, check) {
		changed = true
	}
	if err != nil {
		return err
	}
	if !changed { // --check: 0 when something can be updated
		return &exitError{code: 2, msg: "already current (" + version + "); nothing to update", quiet: true}
	}
	return nil
}

// updateSelf replaces exe with the latest mw release and reports whether it did
// (check: only says whether one exists).
func updateSelf(exe string, check bool) (bool, error) {
	rel, err := latestRelease()
	if err != nil {
		return false, fmt.Errorf("cannot read the latest release (%v); see %s", err, releasesURL)
	}
	latest := strings.TrimPrefix(rel.Tag, "v")
	if !newerVersion(version, latest) {
		fmt.Printf("%s %s is current (latest release %s)\n", binName, version, latest)
		return false, nil
	}
	if check {
		fmt.Printf("update available: %s → %s (%s update)\n", version, latest, binName)
		return true, nil
	}
	bin, err := fetchVerified(rel, repoName, binName)
	if err != nil {
		return false, err
	}
	if err := replaceExe(exe, bin); err != nil {
		return false, fmt.Errorf("cannot replace %s (%v); install the new version with:\n%s", exe, err, installHint)
	}
	fmt.Printf("%s %s → %s (%s)\n", binName, version, latest, exe)
	fanfareDefaults(os.Stdout)
	installPlugins(os.Stdout)
	return true, nil
}

// The partner binary: mw and wr install and update together, each at its latest release.
const partnerRepo, partnerBin = "wr", "wr"

var partnerAPI = "https://api.github.com/repos/stormovermars/" + partnerRepo + "/releases/latest"

// updatePartner installs or updates wr next to exe (best effort: a failure is one note line).
// It reports whether it changed wr (check: whether it would).
func updatePartner(w io.Writer, exe string, check bool) bool {
	p := filepath.Join(filepath.Dir(exe), partnerBin+filepath.Ext(exe))
	have := binVersion(p) // "" when missing or not runnable
	rel, err := releaseFrom(partnerAPI)
	if err != nil {
		fmt.Fprintf(w, "%s: not updated: cannot read the latest release (%v); retry: %s update\n", partnerBin, err, binName)
		return false
	}
	latest := strings.TrimPrefix(rel.Tag, "v")
	switch {
	case have != "" && !newerVersion(have, latest):
		fmt.Fprintf(w, "%s %s is current (latest release %s)\n", partnerBin, have, latest)
		return false
	case check && have == "":
		fmt.Fprintf(w, "%s %s available, not installed at %s (%s update installs it)\n", partnerBin, latest, p, binName)
		return true
	case check:
		fmt.Fprintf(w, "update available: %s %s → %s (%s update)\n", partnerBin, have, latest, binName)
		return true
	}
	bin, err := fetchVerified(rel, partnerRepo, partnerBin)
	if err == nil {
		err = replaceExe(p, bin)
	}
	if err != nil {
		fmt.Fprintf(w, "%s: not updated: %v; retry: %s update\n", partnerBin, oneLine(err.Error()), binName)
		return false
	}
	if have == "" {
		have = "(new)"
	}
	fmt.Fprintf(w, "%s %s → %s (%s)\n", partnerBin, have, latest, p)
	return true
}

// binVersion is the version a mw or wr binary reports in `version -j` ("" when it cannot say).
func binVersion(p string) string {
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, p, "version", "-j").Output()
	var v struct {
		Version string `json:"version"`
	}
	if err != nil || json.Unmarshal(b, &v) != nil {
		return ""
	}
	return v.Version
}

// Claude Code plugins: best effort, never changes the exit status.
var (
	pluginNames = []string{"mw", "mw-jobs"}
	// retiredPlugins were replaced by mw-jobs; uninstalled once the new ones are in.
	retiredPlugins = []string{"tidewatch", "mw-watch"}
	claudeFound    = func() bool { _, err := exec.LookPath("claude"); return err == nil }
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

// fanfareDefaults runs `mw user fanfare-defaults` for the freshly installed binary (best effort, idempotent).
var fanfareDefaults = func(w io.Writer) {
	if os.Getenv("MW_NO_FANFARE") == "1" || os.Getenv("MW_NO_PLUGINS") == "1" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if b, err := exec.CommandContext(ctx, exe, "user", "fanfare-defaults").CombinedOutput(); err == nil {
		fmt.Fprint(w, string(b))
	}
}

// installPlugins installs or updates the mw and mw-jobs plugins from the miniwaves marketplace, then removes tidewatch and mw-watch.
// claude itself resolves CLAUDE_CONFIG_DIR from the inherited environment.
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
	allOK := true
	for _, p := range pluginNames {
		id := p + "@" + repoName
		verb, args := "installed", []string{"plugin", "install", id}
		if pluginListed(listing, id) {
			verb, args = "updated", []string{"plugin", "update", id}
		}
		if o, err := claudeRun(args...); err != nil {
			allOK = false
			fmt.Fprintf(w, "plugin %s: failed: %s\n", p, lastLine(o, err))
		} else {
			fmt.Fprintf(w, "plugin %s: %s\n", p, verb)
		}
	}
	if !allOK {
		return
	}
	// The retired pane plugins go only after the new ones are in place (best effort).
	for _, old := range retiredPlugins {
		id := old + "@" + repoName
		if !pluginListed(listing, id) {
			continue
		}
		if _, err := claudeRun("plugin", "uninstall", id); err == nil {
			fmt.Fprintf(w, "plugin %s: removed (replaced by mw-jobs)\n", old)
		}
	}
}

// pluginListed reports whether `claude plugin list` output names the plugin id.
func pluginListed(listing, id string) bool {
	for _, l := range strings.Split(listing, "\n") {
		if t := strings.TrimSpace(l); t == id || strings.HasSuffix(t, " "+id) {
			return true
		}
	}
	return false
}

// fetchVerified downloads repo's asset for this platform and checksums.txt, checks the sha256
// and returns the binary bin from the archive.
func fetchVerified(rel *ghRelease, repo, bin string) ([]byte, error) {
	name := assetFor(repo, runtime.GOOS, runtime.GOARCH)
	urls := map[string]string{}
	for _, a := range rel.Assets {
		urls[a.Name] = a.URL
	}
	if urls[name] == "" || urls["checksums.txt"] == "" {
		return nil, fmt.Errorf("release %s has no %s (or checksums.txt); see https://github.com/stormovermars/%s/releases", rel.Tag, name, repo)
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
	return extractBinary(arc, strings.HasSuffix(name, ".zip"), bin)
}

func extractBinary(arc []byte, isZip bool, binName string) ([]byte, error) {
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
	if _, err := os.Stat(exe); swapOld && err == nil {
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
