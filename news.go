package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// mw news: the news bulletin of a coordinated job (or of the whole workspace when no job exists), built from
// board data only. One model (news_model.go) is computed per job; json, md, html and text are drawn from it
// (news_bulletin.go). Files are written only when the content changed (sidecar <page>.hash). With --release a
// numbered release is cut into the job's releases folder. No network, no --publish.

const newsSummaryCap = 20480 // bytes read from the summary file

// newsFence picks a code fence longer than any backtick run in s.
func newsFence(s string) string {
	fence, run := "```", 0
	for _, r := range s {
		if r == '`' {
			run++
			if run >= len(fence) {
				fence = strings.Repeat("`", run+1)
			}
		} else {
			run = 0
		}
	}
	return fence
}

type newsPart struct {
	all     []*Tenant // every tenant of the job when tenants is filtered to one; nil otherwise
	job     string    // job id, "" for the workspace
	title   string
	desc    string // job description, one line; "" when none
	url     string
	tenants []*Tenant
	summary string // file contents, already capped; "" when none
}

// newsJobTenants lists the visible tenants of a job (all visible tenants when id is "").
func newsJobTenants(w *Workspace, id string) []*Tenant {
	var out []*Tenant
	for i := range w.Tenants {
		t := &w.Tenants[i]
		if t.Hidden || (id != "" && t.Job != id) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// newsConfined resolves a workspace-relative path to a real path that stays inside the workspace after
// symlinks are resolved. The deepest existing ancestor is resolved, so a not-yet-created target is judged
// by where its parent really lives.
func newsConfined(root, key, rel string) (string, error) {
	c, err := workspaceRelPath(key, rel)
	if err != nil {
		return "", err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	probe := filepath.Join(realRoot, filepath.FromSlash(strings.TrimSuffix(c, "/")))
	var tail []string
	for {
		if r, e := filepath.EvalSymlinks(probe); e == nil {
			probe = r
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", fmt.Errorf("%s: %q cannot be resolved", key, rel)
		}
		tail = append([]string{filepath.Base(probe)}, tail...)
		probe = parent
	}
	real := filepath.Join(append([]string{probe}, tail...)...)
	if boardOwnPath(c) {
		return "", fmt.Errorf("%s: %q is mw's own board file or folder", key, rel)
	}
	if real != realRoot && !strings.HasPrefix(real, realRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("%s: %q leaves the workspace (a symlink points outside it)", key, rel)
	}
	if r, e := filepath.Rel(realRoot, real); e == nil && boardOwnPath(filepath.ToSlash(r)) {
		return "", fmt.Errorf("%s: %q is mw's own board file or folder", key, rel)
	}
	return real, nil
}

// openConfined opens a workspace-relative file for reading, as data. newsConfined resolves the path (symlinks
// included) and keeps it inside the workspace and off mw's own files; the file is then opened by that resolved
// path and checked once more on the open descriptor: it must be a regular file and the very file the path still
// names (not a symlink swapped in). A swap of a parent folder between the resolve and the open cannot be closed
// portably (there is no cheap path-of-descriptor call on every OS), so that narrow race remains; the reader
// only ever gets a regular file of capped size, which is all it could leak.
func openConfined(root, key, rel string) (*os.File, error) {
	p, err := newsConfined(root, key, rel)
	if err != nil {
		return nil, err
	}
	if fi, err := os.Stat(p); err == nil && !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: %q is not a regular file", key, rel)
	}
	f, err := openNoBlock(p)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	li, lerr := os.Lstat(p)
	if err != nil || lerr != nil || !fi.Mode().IsRegular() || !li.Mode().IsRegular() || !os.SameFile(fi, li) {
		f.Close()
		return nil, fmt.Errorf("%s: %q is not a regular file", key, rel)
	}
	return f, nil
}

// newsSummary reads the job's summary file as data: confined, a regular file, at most 20 KB, a UTF-8 BOM dropped.
func newsSummary(root, rel string) (string, error) {
	f, err := openConfined(root, "summary", rel)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		var pe *fs.PathError
		if errors.As(err, &pe) {
			return "", err
		}
		if strings.Contains(err.Error(), "is not a regular file") {
			return "", nil // a FIFO or device would block the open under the board lock
		}
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, newsSummaryCap+1))
	if err != nil {
		return "", err
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	s := string(b)
	if len(b) > newsSummaryCap {
		s = strings.ToValidUTF8(string(b[:newsSummaryCap]), "") + "\n[truncated at 20 KB]"
	}
	return strings.ReplaceAll(strings.TrimSpace(s), "\r\n", "\n"), nil
}

// newsWrite stores content at out unless its hash matches the sidecar and the file exists. The sidecar holds
// "<unix time of the write>\n<sha256>".
func newsWrite(out, content string, now time.Time) (bool, error) {
	sum := sha256.Sum256([]byte(content))
	hash := hex.EncodeToString(sum[:])
	if b, err := readRegular(out+".hash", 4096); err == nil {
		if f := strings.Fields(string(b)); len(f) >= 2 && f[1] == hash {
			if _, err := os.Stat(out); err == nil {
				return false, nil
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return false, err
	}
	if err := writeFileAtomicMode(out, []byte(content), 0644); err != nil {
		return false, err
	}
	return true, writeFileAtomicMode(out+".hash", []byte(strconv.FormatInt(now.Unix(), 10)+"\n"+hash+"\n"), 0644)
}

type newsResult struct {
	Job     string `json:"job,omitempty"`
	Path    string `json:"path"`
	Written bool   `json:"written"`
}

type newsReleaseResult struct {
	Job      string `json:"job"`
	Released bool   `json:"released"`
	Number   int    `json:"number"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message,omitempty"`
}

// newsRender draws one model in a format; the releases list feeds the footer, prefix the html links.
func newsRender(format string, m *newsModel, releases []int, prefix string) (string, error) {
	switch format {
	case "html":
		return bulletinHTMLPage(bulletinTitle(m), []string{bulletinHTMLBlock(m, releases, prefix)}), nil
	case "text":
		return renderBulletinText(m, releases), nil
	case "json":
		b, err := newsModelJSON(m)
		return string(b), err
	}
	return renderBulletinMD(m, releases), nil
}

func relPrefix(from, dir string) string {
	r, err := filepath.Rel(filepath.Dir(from), dir)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(r) + "/"
}

func newsCommand(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return fmt.Errorf("unexpected argument %q", pos[0])
	}
	if err := allowOptions(o, "job", "format", "out", "json", "width", "release", "force", "tenant", "full"); err != nil {
		return err
	}
	format := value(o, "format", "md")
	if format != "md" && format != "html" && format != "text" && format != "json" {
		return fmt.Errorf("--format %q: use md, html, text or json", format)
	}
	release := o["release"] == "true"
	if release && o["out"] != "" {
		return errors.New("--release writes into the releases folder and takes no -o (use news --format ... -o PATH for a draft)")
	}
	if o["full"] == "true" && release {
		return errors.New("--full cannot be combined with --release (a release keeps the caps)")
	}
	if o["force"] == "true" && !release {
		return errors.New("--force is only for --release")
	}
	if o["width"] != "" { // bulletins are always 70 columns: the flag is checked for text and ignored
		if format != "text" {
			return errors.New("--width is only for --format text (bulletins are 70 columns wide)")
		}
		if _, e := strconv.Atoi(strings.TrimSpace(o["width"])); e != nil {
			return fmt.Errorf("--width %q: use a number", o["width"])
		}
	}
	if o["tenant"] != "" {
		if format != "json" {
			return errors.New("--tenant is only for --format json")
		}
		if release {
			return errors.New("--tenant cannot be combined with --release (a release is job-wide)")
		}
	}
	root := filepath.Dir(path)
	var ids []string
	if j := o["job"]; j != "" {
		if _, ok := w.Jobs[j]; !ok {
			return fmt.Errorf("job %s not found (mw jobs show)", j)
		}
		ids = []string{j}
	} else {
		for id := range w.Jobs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
	}
	if len(ids) == 0 {
		ids = []string{""} // no jobs: the workspace is the one section
	}
	if release && (len(ids) != 1 || ids[0] == "") {
		return errors.New("--release needs one job (--job ID; mw jobs enable ID -t SLUG creates one)")
	}
	if format == "json" && len(ids) != 1 {
		return errors.New("--format json needs --job ID when several jobs exist")
	}
	stdout := o["out"] == "-"
	plain := format == "text" || format == "json" // stdout unless -o PATH, no news.out, no sidecar
	if o["full"] == "true" && o["out"] == "" && !plain {
		// the default target is the desk's status.md/status.html draft: --full must never rewrite it
		return fmt.Errorf("--full would rewrite the desk's status.%s draft: send it to stdout (-o -), a file (-o FILE) or use --format text or json", format)
	}
	if o["json"] == "true" && !release && (stdout || plain && o["out"] == "") {
		return errors.New("-j needs a file target (-o PATH or news.out)")
	}
	type job struct {
		p        newsPart
		v        JobView
		model    *newsModel
		relDir   string
		nums     []int
		prev     *newsModel
		prevNum  *int
		metrics  *newsMetrics
		target   string
		nextNum  int
		draftWin newsWindow
	}
	// everything is validated and built before anything is written
	var jobs []*job
	seen := map[string]string{}
	now := time.Now()
	for _, id := range ids {
		j := &job{p: newsPart{job: id, title: w.Name, tenants: newsJobTenants(w, id)}}
		if slug := o["tenant"]; slug != "" { // the same model, filtered to one tenant; the window stays job-wide
			var one *Tenant
			for _, t := range j.p.tenants {
				if t.Slug == slug {
					one = t
				}
			}
			if one == nil {
				return fmt.Errorf("tenant %s is not a visible tenant of this job (mw jobs show)", slug)
			}
			j.p.all, j.p.tenants = j.p.tenants, []*Tenant{one}
		}
		dir := "docs/news/"
		if id != "" {
			j.v = jobView(w, id)
			v := j.v
			j.p.title, j.p.desc, j.p.url, dir = v.Title, v.Description, v.NewsURL, v.NewsOut
			if v.Summary != "" {
				if j.p.summary, err = newsSummary(root, v.Summary); err != nil {
					return err
				}
			}
			if v.Metrics != "" {
				j.metrics = newsMetricsLoad(root, v.Metrics)
			}
			if j.relDir, err = newsReleaseDir(root, v); err != nil {
				if release || w.Jobs[id].NewsReleases != "" {
					return err
				}
				j.relDir = "" // a bad news.out is reported where it is used (the page path); -o ignores it
			}
			if j.relDir != "" {
				if j.prev, j.prevNum, j.nums, err = newsPreviousRelease(j.relDir, release); err != nil {
					return err
				}
			}
		}
		switch {
		case stdout, plain && o["out"] == "": // no file, so no news.out and no sidecar
		case o["out"] != "":
			if j.target, err = newsOutPath(path, root, o["out"]); err != nil {
				return err
			}
		default:
			d, err := newsConfined(root, "news.out", dir)
			if err != nil {
				return err
			}
			j.target = filepath.Join(d, "status."+format)
			if other, dup := seen[j.target]; dup {
				return fmt.Errorf("jobs %q and %q share the news file %s: set news.out per job (mw jobs set ID news.out=docs/news/ID/)", other, id, j.target)
			}
			seen[j.target] = id
		}
		win := newsWindow{prev: j.prev, prevNum: j.prevNum, full: o["full"] == "true"}
		if release {
			n := newsMaxReleaseNumber(j.relDir) + 1 // above every N.* file and the latest pointer, readable or not
			j.nextNum = n
			win.number = &n
			win.at = now.UTC().Format(time.RFC3339Nano)
		}
		j.model = buildNewsModel(w, j.p, j.metrics, win)
		jobs = append(jobs, j)
	}

	if release {
		return newsCutRelease(o, format, jobs[0].model, jobs[0].relDir, jobs[0].nums, jobs[0].prevNum, jobs[0].nextNum, o["force"] == "true")
	}

	if plain { // stdout by default; a file only with -o PATH
		out := ""
		if o["out"] != "" && !stdout {
			if out, err = newsOutPath(path, root, o["out"]); err != nil {
				return err
			}
		}
		docs := make([]string, len(jobs))
		for i, j := range jobs {
			if docs[i], err = newsRender(format, j.model, j.nums, ""); err != nil {
				return err
			}
		}
		content := strings.Join(docs, "\n")
		if out == "" {
			fmt.Print(content)
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
			return err
		}
		if err := writeFileAtomicMode(out, []byte(content), 0644); err != nil {
			return err
		}
		if o["json"] == "true" {
			return printJSON([]newsResult{{"", out, true}})
		}
		fmt.Println("wrote", out)
		return nil
	}

	var results []newsResult
	put := func(job, target, content string) error {
		if target == "" {
			fmt.Print(content)
			return nil
		}
		wrote, err := newsWrite(target, content, now)
		results = append(results, newsResult{job, target, wrote})
		return err
	}
	if o["out"] != "" { // one file holding every section
		var content string
		if format == "html" {
			var blocks []string
			title := w.Name
			for _, j := range jobs {
				blocks = append(blocks, bulletinHTMLBlock(j.model, j.nums, relPrefix(jobs[0].target, j.relDir)))
			}
			if len(jobs) == 1 {
				title = bulletinTitle(jobs[0].model)
			}
			content = bulletinHTMLPage(title, blocks)
		} else {
			docs := make([]string, len(jobs))
			for i, j := range jobs {
				docs[i] = renderBulletinMD(j.model, j.nums)
			}
			content = strings.Join(docs, "\n")
		}
		if err := put("", jobs[0].target, content); err != nil {
			return err
		}
	} else {
		for _, j := range jobs {
			prefix := ""
			if j.target != "" {
				prefix = relPrefix(j.target, j.relDir)
			}
			content, err := newsRender(format, j.model, j.nums, prefix)
			if err != nil {
				return err
			}
			if err := put(j.p.job, j.target, content); err != nil {
				return err
			}
		}
	}
	if stdout {
		return nil
	}
	if o["json"] == "true" {
		return printJSON(results)
	}
	for _, r := range results {
		if r.Written {
			fmt.Println("wrote", r.Path)
		} else {
			fmt.Println("unchanged", r.Path)
		}
	}
	return nil
}

// newsCutRelease cuts release n (the caller holds the board lock, which is the job lock): N.md and N.html are
// drawn from the model, N.json (the frozen model) is linked in after them, and latest is updated last. Files
// of a failed run are removed again. Nothing new since the last release is refused with exit 0 unless force.
// Releases live only in the releases folder: news.out keeps holding the draft.
func newsCutRelease(o map[string]string, format string, m *newsModel, dir string, nums []int, prevNum *int, n int, force bool) (err error) {
	res := newsReleaseResult{Job: m.Job.ID, Number: n}
	say := func() error {
		if o["json"] == "true" {
			return printJSON(res)
		}
		fmt.Println(res.Message)
		return nil
	}
	if m.Release.Changes == 0 && !force {
		since := "the start"
		if prevNum != nil {
			since = fmt.Sprintf("release %d", *prevNum)
		}
		res.Number = 0
		if prevNum != nil {
			res.Number = *prevNum
		}
		res.Message = "nothing new since " + since
		return say()
	}
	js, err := newsModelJSON(m) // encode first: a model that cannot be encoded writes nothing
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	all := append([]int{n}, nums...)
	num := strconv.Itoa(n)
	var made []string
	defer func() {
		if err != nil {
			for _, p := range made {
				_ = os.Remove(p)
			}
		}
	}()
	put := func(name string, b []byte) error {
		p := filepath.Join(dir, name)
		if err := writeNewFile(p, b); err != nil {
			return err
		}
		made = append(made, p)
		return nil
	}
	if err = put(num+".md", []byte(renderBulletinMD(m, all))); err != nil { // the first file claims the number
		return err
	}
	if err = put(num+".html", []byte(renderBulletinHTML(m, all, ""))); err != nil {
		return err
	}
	if err = put(num+".json", js); err != nil {
		return err
	}
	if err = writeFileAtomicMode(filepath.Join(dir, "latest"), []byte(num+"\n"), 0644); err != nil {
		return err
	}
	res.Released, res.Path = true, filepath.Join(dir, num+".md")
	res.Message = fmt.Sprintf("released %d: %s", n, res.Path)
	if format == "json" {
		fmt.Print(string(js))
		return nil
	}
	return say()
}

// boardOwnPath reports a workspace-relative path at or under mw's own files: .miniwaves/ (tenant files,
// archives, locks), .miniwaves.json and its backups and locks. A job's output must never point there.
func boardOwnPath(rel string) bool {
	first := strings.ToLower(strings.SplitN(strings.TrimPrefix(strings.ReplaceAll(rel, `\`, "/"), "./"), "/", 2)[0])
	return strings.HasPrefix(first, ".miniwaves")
}

// newsOutPath resolves -o PATH. A path outside the workspace is allowed as before; one that lands inside it
// (symlinks resolved, a not-yet-created file judged by where its parent really lives) must not be mw's own
// board file or folder, so -o can never overwrite .miniwaves.json or a tenant file.
func newsOutPath(boardPath, root, out string) (string, error) {
	p := htmlPath(boardPath, out)
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return p, nil
	}
	probe, err := filepath.Abs(p)
	if err != nil {
		return p, nil
	}
	var tail []string
	for {
		if r, e := filepath.EvalSymlinks(probe); e == nil {
			probe = r
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return p, nil
		}
		tail = append([]string{filepath.Base(probe)}, tail...)
		probe = parent
	}
	real := filepath.Join(append([]string{probe}, tail...)...)
	if rel, e := filepath.Rel(realRoot, real); e == nil && rel != "." && !strings.HasPrefix(rel, "..") && boardOwnPath(filepath.ToSlash(rel)) {
		return "", fmt.Errorf("-o %q is mw's own board file or folder (.miniwaves/, .miniwaves.json and their locks and backups)", out)
	}
	return p, nil
}
