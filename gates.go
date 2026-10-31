package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// version is the mw release; schema is currentSchema.
var version = "0.3.14"

var gateNames = []string{"evidence", "acceptance", "judge"}

// gateLevels are the cumulative rigour levels, lowest first: evidence,
// acceptance, judge.
var gateLevels = []struct {
	Name string
	On   [3]bool
}{
	{"trust", [3]bool{false, false, false}},
	{"proof", [3]bool{true, false, false}},
	{"spec", [3]bool{true, true, false}},
	{"judged", [3]bool{true, true, true}},
}

func validLevel(n string) bool {
	for _, l := range gateLevels {
		if l.Name == n {
			return true
		}
	}
	return false
}

// levelOf names the level the effective gates match exactly, else "custom".
func levelOf(gs []gateInfo) string {
	m := effectiveMap(gs)
	got := [3]bool{m["evidence"], m["acceptance"], m["judge"]}
	for _, l := range gateLevels {
		if l.On == got {
			return l.Name
		}
	}
	return "custom"
}

func levelSettings(level string) map[string]string {
	for _, l := range gateLevels {
		if l.Name == level {
			out := map[string]string{}
			for i, n := range gateNames {
				out[n] = map[bool]string{true: "on", false: "off"}[l.On[i]]
			}
			return out
		}
	}
	return nil
}

func levelLine(gs []gateInfo) string {
	lv := levelOf(gs)
	if lv == "custom" {
		return "level custom"
	}
	parts := make([]string, 0, 3)
	for _, g := range gs {
		parts = append(parts, g.Name+" "+map[bool]string{true: "on", false: "off"}[g.Effective])
	}
	return "level " + lv + " (" + strings.Join(parts, ", ") + ")"
}

type gateInfo struct {
	Name      string `json:"name" toon:"name"`
	Setting   string `json:"setting" toon:"setting"`
	Effective bool   `json:"effective" toon:"effective"`
	Reason    string `json:"reason" toon:"reason"`
}

// Per-run state, reset by run(): the tenants this command shows or saves
// (the file-system check runs only for them), flip lines already printed and
// the manifest probe cache.
var (
	touchedTenants = map[string]bool{}
	announced      = map[string]bool{}
	manifestCache  = map[string]manifestResult{}
)

type manifestResult struct {
	found  string
	reason string
}

func resetRunState() {
	touchedTenants = map[string]bool{}
	announced = map[string]bool{}
	manifestCache = map[string]manifestResult{}
	fanfarePending = nil
}

func validGateName(n string) bool {
	for _, g := range gateNames {
		if g == n {
			return true
		}
	}
	return false
}

func validSetting(s string) bool { return s == "on" || s == "off" || s == "auto" }

// settingFor resolves the tenant setting, else the workspace default, else auto.
func settingFor(w *Workspace, t *Tenant, name string) (string, bool) {
	if s := t.Gates[name]; validSetting(s) {
		return s, false
	}
	if s := w.Gates[name]; validSetting(s) {
		return s, true
	}
	return "auto", false
}

// productManifest reports whether the tenant dir looks like a product
// (a build manifest or releases/). Cached per run.
func productManifest(t *Tenant) manifestResult {
	if t.Path == "" {
		return manifestResult{reason: "tenant has no dir"}
	}
	if r, ok := manifestCache[t.Path]; ok {
		return r
	}
	var r manifestResult
	entries, err := os.ReadDir(t.Path)
	if err != nil {
		r = manifestResult{reason: "tenant dir " + t.Path + " is missing or unreadable"}
	} else {
		r = manifestResult{reason: "no build manifest or releases/ in " + t.Path}
		for _, e := range entries {
			n := e.Name()
			switch {
			case strings.HasPrefix(n, "build.gradle"), strings.HasSuffix(n, ".xcodeproj"),
				n == "project.yml", n == "Package.swift", n == "go.mod", n == "package.json", n == "Cargo.toml":
				r = manifestResult{found: n}
			case n == "releases" && e.IsDir():
				r = manifestResult{found: "releases/"}
			}
			if r.found != "" {
				break
			}
		}
	}
	manifestCache[t.Path] = r
	return r
}

// gateStates computes the effective state of the three gates for a tenant.
func gateStates(w *Workspace, t *Tenant) []gateInfo {
	out := make([]gateInfo, 0, 3)
	for _, name := range gateNames {
		setting, dflt := settingFor(w, t, name)
		g := gateInfo{Name: name, Setting: setting}
		switch setting {
		case "on", "off":
			g.Effective = setting == "on"
			g.Reason = "set " + setting
			if dflt {
				g.Reason = "workspace default " + setting
			}
		default:
			if name == "evidence" {
				latch := t.GateLatch[name]
				product := productManifest(t)
				g.Effective = true
				switch {
				case product.found != "":
					g.Reason = "product tenant (" + product.found + ")"
				case latch != "":
					g.Reason = "latched: " + latch
				default:
					g.Reason = "default proof"
				}
			} else {
				g.Reason = "auto = off; set on to require " + map[string]string{"judge": "human accept", "acceptance": "acceptance on prompts/stories"}[name]
			}
		}
		out = append(out, g)
	}
	return out
}

func gateOn(w *Workspace, t *Tenant, name string) bool {
	for _, g := range gateStates(w, t) {
		if g.Name == name {
			return g.Effective
		}
	}
	return false
}

func effectiveMap(gs []gateInfo) map[string]bool {
	m := map[string]bool{}
	for _, g := range gs {
		m[g.Name] = g.Effective
	}
	return m
}

func anyGateOn(gs []gateInfo) bool {
	for _, g := range gs {
		if g.Effective {
			return true
		}
	}
	return false
}

// gatesLine is printed on board/context/next only when a gate is effectively on.
func gatesLine(t *Tenant, gs []gateInfo) string {
	if !anyGateOn(gs) {
		return ""
	}
	if lv := levelOf(gs); lv != "custom" {
		return "gates: " + lv
	}
	parts := make([]string, 0, 3)
	for _, g := range gs {
		s := "off"
		if g.Effective {
			s = "on"
		}
		parts = append(parts, g.Name+" "+s)
	}
	line := "gates: " + strings.Join(parts, ", ")
	return line
}

func touchTenant(slug string) { touchedTenants[slug] = true }

// announceFlips prints one stderr line per gate whose effective state differs
// from gates_seen. It repeats until a saving command records the new state.
func announceFlips(t *Tenant, eff map[string]bool) {
	if len(t.GatesSeen) == 0 {
		return
	}
	for _, name := range gateNames {
		prev := t.GatesSeen[name]
		if prev == eff[name] {
			continue
		}
		key := t.Slug + "/" + name + "/" + fmt.Sprint(eff[name])
		if announced[key] {
			continue
		}
		announced[key] = true
		now, was := "on", "off"
		if !eff[name] {
			now, was = "off", "on"
		}
		fmt.Fprintf(os.Stderr, "mw: [%s] gate %s is now %s (was %s)\n", t.Slug, name, now, was)
	}
}

func announceTenant(w *Workspace, t *Tenant) {
	touchTenant(t.Slug)
	announceFlips(t, effectiveMap(gateStates(w, t)))
}

// refreshGates runs inside save for the tenants of this command: it sets the
// records gates_seen.
func refreshGates(w *Workspace) {
	for i := range w.Tenants {
		t := &w.Tenants[i]
		if !touchedTenants[t.Slug] {
			continue
		}
		eff := effectiveMap(gateStates(w, t))
		announceFlips(t, eff)
		t.GatesSeen = eff
	}
}

func byName(gs []gateInfo, name string) gateInfo {
	for _, g := range gs {
		if g.Name == name {
			return g
		}
	}
	return gateInfo{Name: name}
}

func setLatch(t *Tenant, name, reason string) {
	if t.GateLatch == nil {
		t.GateLatch = map[string]string{}
	}
	t.GateLatch[name] = reason
}

// ---- who may run human-only commands ----

var stdinIsTerminal = realStdinIsTerminal

// realStdinIsTerminal is true only for a character device that is not /dev/null.
func realStdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	if null, err := os.Stat(os.DevNull); err == nil && os.SameFile(fi, null) {
		return false
	}
	return true
}

func refuseSession(o map[string]string, what string) error {
	if o["session"] != "" || os.Getenv("MW_SESSION") != "" {
		return coded("HUMAN_ONLY", fmt.Errorf("%s is for humans: unset MW_SESSION and drop -s", what))
	}
	return nil
}

func gateCommand(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if err := allowOptions(o, "tenant", "session", "json", "by", "workspace"); err != nil {
		return err
	}
	if err := refuseSession(o, "mw gate"); err != nil {
		return err
	}
	if len(pos) != 0 && len(pos) != 2 {
		return errors.New("usage: mw gate [-t SLUG] [-j] | mw gate level trust|proof|spec|judged [-t SLUG] [--by NAME] | mw gate evidence|acceptance|judge on|off|auto [-t SLUG] [--by NAME]")
	}
	isLevel := len(pos) == 2 && strings.EqualFold(pos[0], "level")
	if isLevel {
		pos[1] = strings.ToLower(pos[1])
		if !validLevel(pos[1]) {
			return fmt.Errorf("gate level must be trust, proof, spec or judged, not %q", pos[1])
		}
	}
	if o["workspace"] == "true" {
		return workspaceGate(path, w, o, pos)
	}
	t, err := selectTenant(w, o)
	if err != nil {
		return err
	}
	if isLevel {
		if !stdinIsTerminal() && strings.TrimSpace(o["by"]) == "" {
			return errors.New("mw gate needs --by NAME when stdin is not a terminal")
		}
		t.Gates = levelSettings(pos[1])
		t.GateBy = gateByStamp(o)
		if err := save(path, w); err != nil {
			return err
		}
	} else if len(pos) == 2 {
		name, setting := strings.ToLower(pos[0]), strings.ToLower(pos[1])
		if !validGateName(name) {
			return fmt.Errorf("unknown gate %q (evidence, acceptance, judge)", pos[0])
		}
		if !validSetting(setting) {
			return fmt.Errorf("gate setting must be on, off or auto, not %q", pos[1])
		}
		if !stdinIsTerminal() && strings.TrimSpace(o["by"]) == "" {
			return errors.New("mw gate needs --by NAME when stdin is not a terminal")
		}
		if t.Gates == nil {
			t.Gates = map[string]string{}
		}
		t.Gates[name] = setting
		t.GateBy = gateByStamp(o)
		if setting == "auto" {
			delete(t.GateLatch, name)
			if len(t.GateLatch) == 0 {
				t.GateLatch = nil
			}
		}
		if err := save(path, w); err != nil {
			return err
		}
	}
	gs := gateStates(w, t)
	if o["json"] == "true" {
		return printJSON(struct {
			Level string     `json:"level"`
			Gates []gateInfo `json:"gates"`
		}{levelOf(gs), gs})
	}
	fmt.Println(levelLine(gs))
	for _, g := range gs {
		on := "off"
		if g.Effective {
			on = "on"
		}
		reason := g.Reason
		if g.Setting == "on" || g.Setting == "off" {
			by := t.GateBy
			if strings.HasPrefix(reason, "workspace default") {
				by = w.GateBy
			}
			if n, d := splitGateBy(by); n != "" {
				reason += ", set by " + n + " " + d
			}
		}
		fmt.Printf("%-10s %-5s %-3s %s\n", g.Name, g.Setting, on, reason)
	}
	return nil
}

// workspaceGate sets the workspace default for one gate (mw gate NAME SETTING
// --workspace); auto removes the default. Tenant settings still win.
func workspaceGate(path string, w *Workspace, o map[string]string, pos []string) error {
	if len(pos) != 2 || o["tenant"] != "" {
		return errors.New("usage: mw gate evidence|acceptance|judge on|off|auto --workspace [--by NAME] | mw gate level trust|proof|spec|judged --workspace [--by NAME]")
	}
	if strings.EqualFold(pos[0], "level") {
		if !stdinIsTerminal() && strings.TrimSpace(o["by"]) == "" {
			return errors.New("mw gate needs --by NAME when stdin is not a terminal")
		}
		w.Gates = levelSettings(strings.ToLower(pos[1]))
		w.GateBy = gateByStamp(o)
		if err := save(path, w); err != nil {
			return err
		}
		fmt.Printf("workspace default level: %s\n", strings.ToLower(pos[1]))
		var over []string
		for _, t := range w.Tenants {
			if len(t.Gates) > 0 {
				over = append(over, t.Slug)
			}
		}
		if len(over) > 0 {
			fmt.Printf("overridden by: %s (set their level with -t)\n", strings.Join(over, ", "))
		}
		return nil
	}
	name, setting := strings.ToLower(pos[0]), strings.ToLower(pos[1])
	if !validGateName(name) {
		return fmt.Errorf("unknown gate %q (evidence, acceptance, judge)", pos[0])
	}
	if !validSetting(setting) {
		return fmt.Errorf("gate setting must be on, off or auto, not %q", pos[1])
	}
	if !stdinIsTerminal() && strings.TrimSpace(o["by"]) == "" {
		return errors.New("mw gate needs --by NAME when stdin is not a terminal")
	}
	if w.Gates == nil {
		w.Gates = map[string]string{}
	}
	w.GateBy = gateByStamp(o)
	if setting == "auto" {
		delete(w.Gates, name)
		if len(w.Gates) == 0 {
			w.Gates = nil
		}
	} else {
		w.Gates[name] = setting
	}
	if err := save(path, w); err != nil {
		return err
	}
	fmt.Printf("workspace default %s: %s\n", name, setting)
	return nil
}

// ---- mw edit ----

func editCommand(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return errors.New("usage: mw edit ID [-c TEXT] [-b TEXT] [-p PARENT|-] [ticket=KEY] [-t SLUG]")
	}
	ticket, hasT := "", false
	for _, kv := range pos[1:] {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k != "ticket" {
			return fmt.Errorf("unexpected argument %q (only ticket=KEY; ticket= clears it)", kv)
		}
		ticket, hasT = strings.TrimSpace(v), true
	}
	pos = pos[:1]
	if v, ok := o["ticket"]; ok { // --ticket KEY works like ticket=KEY
		ticket, hasT = strings.TrimSpace(v), true
	}
	if err := allowOptions(o, "tenant", "session", "acceptance", "body", "parent", "ticket"); err != nil {
		return err
	}
	_, hasC := o["acceptance"]
	_, hasB := o["body"]
	_, hasP := o["parent"]
	if !hasC && !hasB && !hasP && !hasT {
		return errors.New("mw edit needs -c TEXT (acceptance), -b TEXT (body), -p PARENT|- (parent) and/or ticket=KEY")
	}
	t, err := selectTenant(w, o)
	if err != nil {
		return err
	}
	it, err := itemByID(t, pos[0])
	if err != nil {
		return err
	}
	if hasC && strings.TrimSpace(it.Acceptance) != "" {
		// drafting (empty acceptance) is open to sessions; changing a set one is humans-only
		if err := refuseSession(o, "mw edit -c on a non-empty acceptance"); err != nil {
			return err
		}
	}
	who := value(o, "session", os.Getenv("MW_SESSION"))
	var what []string
	if hasP {
		parent := ""
		if o["parent"] != "-" {
			parent = strings.ToUpper(o["parent"])
		}
		if err := checkParent(t, it, parent); err != nil {
			return err
		}
		it.Parent = parent
		what = append(what, "parent="+orDash(parent))
	}
	if hasT {
		if it.Kind == "wave" {
			return errors.New("a milestone cannot carry a ticket")
		}
		if ticket != "" {
			if ticket, err = ticketForItem(w, t, ticket); err != nil {
				return err
			}
		}
		it.Ticket = ticket
		what = append(what, "ticket="+orDash(ticket))
	}
	if hasC {
		it.Acceptance = o["acceptance"]
		what = append(what, "acceptance")
	}
	if hasB {
		it.Body = o["body"]
		what = append(what, "body")
	}
	it.Updated = time.Now().UTC().Format(time.RFC3339)
	emit(it, it.Updated, "edit", who, strings.Join(what, ", "))
	touchSession(w, who, it.Updated)
	if err := save(path, w); err != nil {
		return err
	}
	fmt.Printf("[%s] %s  %-7s %s\n", t.Slug, it.ID, it.Status, compactText(it.Title, 72))
	return nil
}

// ---- mw version ----

type versionInfo struct {
	Version     string          `json:"version" toon:"version"`
	Schema      int             `json:"schema" toon:"schema"`
	BoardSchema *int            `json:"board_schema" toon:"board_schema"`
	Board       *string         `json:"board" toon:"board"`
	Tenants     []versionTenant `json:"tenants,omitempty" toon:"tenants"`
	Repo        string          `json:"repo" toon:"repo"`
	ReleasesURL string          `json:"releases_url" toon:"releases_url"`
}

type versionTenant struct {
	Slug     string   `json:"slug"`
	Name     string   `json:"name"`
	Dir      string   `json:"dir"`
	Tags     []string `json:"tags,omitempty"`
	Archived bool     `json:"archived,omitempty"` // a runner must skip it (single-tenant commands refuse it)
}

// boardSchemaOf reads only the "version" field of a board file (any schema).
func boardSchemaOf(path string) (int, bool) {
	b, err := readFileShared(path) // one short shared read: never blocks a writer's rename on Windows
	if err != nil {
		return 0, false
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return 0, false
	}
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			return 0, false
		}
		if k == "version" {
			var n int
			if dec.Decode(&n) != nil {
				return 0, false
			}
			return n, true
		}
		var skip json.RawMessage
		if dec.Decode(&skip) != nil {
			return 0, false
		}
	}
	return 0, false
}

func versionCommand(args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return errors.New("usage: mw version [-j]")
	}
	if err := allowOptions(o, "json"); err != nil {
		return err
	}
	v := versionInfo{Version: version, Schema: currentSchema, Repo: "stormovermars/" + repoName, ReleasesURL: releasesURL}
	if p, err := findWorkspace(); err == nil {
		v.Board = &p
		if n, ok := boardSchemaOf(p); ok {
			v.BoardSchema = &n
			if n == currentSchema {
				if w, err := loadScoped(p, false); err == nil {
					for _, t := range w.Tenants {
						v.Tenants = append(v.Tenants, versionTenant{t.Slug, t.Name, tenantDir(p, &t), t.Tags, t.Hidden})
					}
				}
			}
		}
	}
	if o["json"] == "true" {
		return printJSON(v)
	}
	fmt.Printf("mw %s (schema %d)\n%s · update: mw update\n", v.Version, v.Schema, releasesURL)
	return nil
}

// ---- acceptance inheritance ----

// effectiveAcceptance is the item's own acceptance, else the nearest
// non-empty ancestor's.
func effectiveAcceptance(t *Tenant, it Item) string {
	a, _ := acceptanceSource(t, it)
	return a
}

// acceptanceSource also returns the ID of the item the acceptance comes from.
func acceptanceSource(t *Tenant, it Item) (string, string) {
	cur := it
	for i := 0; i <= len(t.Items); i++ {
		if strings.TrimSpace(cur.Acceptance) != "" {
			return cur.Acceptance, cur.ID
		}
		if cur.Parent == "" {
			return "", ""
		}
		p, err := itemByID(t, cur.Parent)
		if err != nil {
			return "", ""
		}
		cur = *p
	}
	return "", ""
}

func isWorkKind(kind string) bool {
	return kind == "task" || kind == "spike" || kind == "bug" || kind == "chore"
}

// ---- evidence ----

func evidenceFolder(w *Workspace, t *Tenant, id string) string {
	return filepath.Join(workspaceFolders(w).Evidence, t.Slug, id)
}

func ignoredEvidenceName(n string) bool {
	return strings.HasPrefix(n, ".") || strings.EqualFold(n, "Thumbs.db") || strings.EqualFold(n, "desktop.ini")
}

// checkEvidence requires <evidence>/<slug>/<ID>/ with at least one regular,
// non-empty, non-junk file. The folder name must equal the ID exactly.
// It returns the files as "relative/path size".
func checkEvidence(w *Workspace, t *Tenant, id string) ([]string, error) {
	files, err := checkEvidenceFiles(w, t, id)
	if err != nil && byName(gateStates(w, t), "evidence").Reason == "default proof" {
		err = fmt.Errorf("%w (0.2.5 default level is proof; a human can run `mw gate level trust -t SLUG`)", err)
	}
	return files, err
}

func checkEvidenceFiles(w *Workspace, t *Tenant, id string) ([]string, error) {
	want := evidenceFolder(w, t, id)
	parent := filepath.Dir(want)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil, coded("GATE_REFUSED", fmt.Errorf("evidence required: put proof in %s (or --no-evidence REASON)", want))
	}
	found := false
	wrongCase := ""
	for _, e := range entries {
		isDir := false
		if fi, e2 := os.Stat(filepath.Join(parent, e.Name())); e2 == nil { // follows a symlinked folder
			isDir = fi.IsDir()
		}
		switch {
		case e.Name() == id && isDir:
			found = true
		case strings.EqualFold(e.Name(), id) && isDir:
			wrongCase = e.Name()
		}
	}
	if !found {
		if wrongCase != "" {
			return nil, coded("GATE_REFUSED", fmt.Errorf("evidence folder %s has the wrong case: it must be named exactly %s", wrongCase, id))
		}
		return nil, coded("GATE_REFUSED", fmt.Errorf("evidence required: put proof in %s (or --no-evidence REASON)", want))
	}
	var files []string
	root := want
	if r, e := filepath.EvalSymlinks(want); e == nil {
		root = r // WalkDir does not descend a symlinked root
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p != root && ignoredEvidenceName(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, e := os.Stat(p) // follows a symlinked file
		if e != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return nil
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return nil
		}
		files = append(files, fmt.Sprintf("%s %d", filepath.ToSlash(rel), info.Size()))
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("evidence folder %s has no non-empty file (dotfiles, Thumbs.db, desktop.ini do not count; or --no-evidence REASON)", want)
	}
	sort.Strings(files)
	return files, nil
}

// noEvidenceReason returns the --no-evidence reason ("" when not given).
func noEvidenceReason(o map[string]string) (string, error) {
	r, ok := o["no-evidence"]
	if !ok {
		return "", nil
	}
	r = strings.TrimSpace(r)
	if r == "" {
		return "", errors.New("--no-evidence needs a reason")
	}
	return r, nil
}

// tenantDir is the tenant's folder (its path, else the workspace folder).
func tenantDir(boardPath string, t *Tenant) string {
	if t.Path != "" {
		return t.Path
	}
	return filepath.Dir(boardPath)
}

// gateByStamp is "NAME RFC3339" for the --by value ("human" at a terminal).
func gateByStamp(o map[string]string) string {
	by := strings.Join(strings.Fields(o["by"]), "_")
	if by == "" {
		by = "human"
	}
	return by + " " + time.Now().UTC().Format(time.RFC3339)
}

// splitGateBy returns the name and the date (YYYY-MM-DD) of a gate_by stamp.
func splitGateBy(s string) (name, date string) {
	n, ts, ok := strings.Cut(s, " ")
	if !ok {
		return "", ""
	}
	if len(ts) >= 10 {
		ts = ts[:10]
	}
	return n, ts
}
