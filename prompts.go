package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

//go:embed prompts/*.md
var promptFS embed.FS

var promptTemplates = template.Must(template.New("prompts").
	Funcs(template.FuncMap{"q": func(s string) string { return fmt.Sprintf("%q", s) }}).
	ParseFS(promptFS, "prompts/*.md"))

type promptData struct {
	Role, ID, Slug, TenantName, Setup string
	Session                           string
	Headless                          bool
	Name, Workspace, ChangeDir, Root  string
	Tenants, Sessions                 int
	Folders                           string
	// Gate rules render only for effectively-on gates (empty = today's text).
	GateEvidence, GateAcceptance, GateJudge bool
	EvidenceDir                             string
	EvidenceTenant                          string // <evidence>/<tenant>/, the parent of the per-ID folders
	GatesRule                               string
	// Coordinated-job variables (neutral defaults without --job; jobs.<id> with it).
	Job, Title, Lock, Worktrees, NewsOut, NewsURL string
	MaxWorkers                                    int
	// Ceremony variables (docs/specs/ceremonies.md): a section renders only for the ceremonies the job lists.
	Ceremonies                                  bool
	CerReview, CerPlanning, CerTriage, CerSpike bool
	ReadyMin, CerPerDay                         int
}

// ceremonyPerDay is the default cap of ceremony sessions per day (a prompt variable, not a job key).
const ceremonyPerDay = 4

func (d promptData) render(name string) string {
	var b strings.Builder
	if err := promptTemplates.ExecuteTemplate(&b, name+".md", d); err != nil {
		panic(err)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func promptWith(w *Workspace, name string, d promptData) string {
	f := workspaceFolders(w)
	d.Folders = folderSentence(f, d.Slug, d.Session)
	return d.render(name)
}

func buildSessionPrompt(path string, w *Workspace, t *Tenant, id, role string, headless bool) string {
	dir := t.Path
	if dir == "" {
		dir = filepath.Dir(path)
	}
	gs := gateStates(w, t)
	// A ceremony role block renders only when the tenant's job lists that ceremony.
	var cer promptData
	if _, ok := w.Jobs[t.Job]; ok && t.Job != "" {
		for _, n := range jobView(w, t.Job).Ceremonies {
			switch n {
			case "review":
				cer.CerReview = true
			case "planning":
				cer.CerPlanning = true
			case "spike":
				cer.CerSpike = true
			}
		}
	}
	return promptWith(w, "session", promptData{CerReview: cer.CerReview, CerPlanning: cer.CerPlanning, CerSpike: cer.CerSpike, Role: role, ID: id, Slug: t.Slug, Session: id, Headless: headless, TenantName: compactText(t.Name, 72),
		Setup:        sessionSetup(runtime.GOOS, dir, id, t.Slug, role),
		GateEvidence: byName(gs, "evidence").Effective, GateAcceptance: byName(gs, "acceptance").Effective, GateJudge: byName(gs, "judge").Effective,
		EvidenceDir:    filepath.Join(workspaceFolders(w).Evidence, t.Slug, "<ID>") + string(filepath.Separator),
		EvidenceTenant: filepath.Join(workspaceFolders(w).Evidence, t.Slug) + string(filepath.Separator)})
}

func sessionSetup(goos, dir, id, tenant, role string) string {
	if goos == "windows" {
		return fmt.Sprintf("  Set-Location %s\n  $env:MW_SESSION=%s; $env:MW_TENANT=%s",
			powerShellQuote(dir), powerShellQuote(id), powerShellQuote(tenant))
	}
	return fmt.Sprintf("  cd %s\n  export MW_SESSION=%s MW_TENANT=%s",
		shellQuote(dir), shellQuote(id), shellQuote(tenant))
}

// jobPromptData fills the job variables: neutral defaults, or jobs.<id> when job is set.
func jobPromptData(w *Workspace, job string) (promptData, error) {
	d := promptData{MaxWorkers: 4, Worktrees: "on", NewsOut: "docs/news/"}
	if job == "" {
		return d, nil
	}
	if _, ok := w.Jobs[job]; !ok {
		return d, fmt.Errorf("job %s not found (mw jobs show; mw jobs enable %s -t SLUG)", job, job)
	}
	v := jobView(w, job)
	d.Job, d.Title, d.Lock, d.Worktrees, d.MaxWorkers, d.NewsOut, d.NewsURL = job, v.Title, v.Lock, v.Worktrees, v.MaxWorkers, v.NewsOut, v.NewsURL
	for _, n := range v.Ceremonies {
		d.Ceremonies = true
		switch n {
		case "review":
			d.CerReview = true
		case "planning":
			d.CerPlanning = true
		case "triage":
			d.CerTriage = true
		case "spike":
			d.CerSpike = true
		}
	}
	if d.Ceremonies {
		d.ReadyMin, d.CerPerDay = v.ReadyMin, ceremonyPerDay
	}
	return d, nil
}

func getPromptCommand(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return errors.New("usage: mw get-prompt session NAME | dispatcher|coach|newsdesk [NAME] [--job ID]")
	}
	kind, name := pos[0], ""
	var prompt string
	result := map[string]string{"type": kind, "workspace": filepath.Dir(path)}
	switch kind {
	case "session":
		if err := allowOptions(o, "tenant", "agent", "json", "headless"); err != nil {
			return err
		}
		if len(pos) != 2 {
			return errors.New("usage: mw get-prompt session NAME [-t SLUG] [-a ROLE] [--headless] [-j]")
		}
		name = pos[1]
		if !validID(name) {
			return errors.New("prompt name must be at most 64 lowercase letters, numbers, and single dashes")
		}
		if _, err := sessionByID(w, name); err == nil {
			if o["json"] == "true" {
				b, _ := json.Marshal(map[string]string{"error": "session_exists", "session": name})
				fmt.Println(string(b))
				return &exitError{code: 3, msg: "session " + name + " already exists", quiet: true}
			}
			return &exitError{code: 3, msg: "session " + name + " already exists"}
		}
		t, err := selectTenant(w, map[string]string{"tenant": o["tenant"]})
		if err != nil {
			return err
		}
		role := value(o, "agent", "builder")
		if !validID(role) {
			return errors.New("agent role must be at most 64 lowercase letters, numbers, and single dashes")
		}
		prompt = buildSessionPrompt(path, w, t, name, role, o["headless"] == "true")
		result["tenant"], result["agent"] = t.Slug, role
	case "dispatcher", "newsdesk":
		if err := allowOptions(o, "json", "job"); err != nil {
			return err
		}
		if len(pos) > 2 {
			return errors.New("usage: mw get-prompt " + kind + " [NAME] [--job ID] [-j]")
		}
		name = kind
		if len(pos) == 2 {
			name = pos[1]
		}
		if !validID(name) {
			return errors.New("prompt name must be at most 64 lowercase letters, numbers, and single dashes")
		}
		jd, err := jobPromptData(w, o["job"])
		if err != nil {
			return err
		}
		if kind == "dispatcher" {
			prompt = buildDispatcherPromptJob(path, w, name, jd)
		} else {
			prompt = buildNewsdeskPrompt(path, w, name, jd)
		}
		if o["job"] != "" {
			result["job"] = o["job"]
		}
	case "coach":
		if err := allowOptions(o, "json"); err != nil {
			return err
		}
		if len(pos) > 2 {
			return errors.New("usage: mw get-prompt coach [NAME] [-j]")
		}
		name = "coach"
		if len(pos) == 2 {
			name = pos[1]
		}
		prompt = buildCoachPrompt(path, w, name)
	default:
		return errors.New("usage: mw get-prompt session NAME | dispatcher|coach|newsdesk [NAME] [--job ID]")
	}
	if !validID(name) {
		return errors.New("prompt name must be at most 64 lowercase letters, numbers, and single dashes")
	}
	result["name"], result["prompt"] = name, prompt
	if o["json"] == "true" {
		return printJSON(result)
	}
	fmt.Println(prompt)
	return nil
}

func buildDispatcherPromptJob(path string, w *Workspace, name string, d promptData) string {
	d.Name, d.Workspace, d.ChangeDir = name, compactText(w.Name, 72), changeDir(runtime.GOOS, filepath.Dir(path))
	d.Tenants, d.Sessions = len(w.Tenants), len(w.Sessions)
	d.GatesRule, d.GateJudge = workspaceGatesRule(w, "Gates are per tenant (mw gate -t SLUG): "), workspaceGatesOn(w)["judge"]
	return promptWith(w, "dispatcher", d)
}

func buildNewsdeskPrompt(path string, w *Workspace, name string, d promptData) string {
	d.Name, d.Workspace, d.ChangeDir = name, compactText(w.Name, 72), changeDir(runtime.GOOS, filepath.Dir(path))
	d.Title = compactText(d.Title, 72)
	return promptWith(w, "newsdesk", d)
}

func buildCoachPrompt(path string, w *Workspace, name string) string {
	return promptWith(w, "coach", promptData{Name: name, Workspace: compactText(w.Name, 72), Root: compactText(filepath.Dir(path), 120),
		GatesRule:      workspaceGatesRule(w, "Gates on somewhere (mw gate -t SLUG): "),
		GateAcceptance: workspaceGatesOn(w)["acceptance"], GateJudge: workspaceGatesOn(w)["judge"]})
}

// workspaceGatesRule lists the gates effectively on in any tenant ("" when none).
// It looks only at tenants that carry a stored gate setting, latch or
// gates_seen entry, so a prompt request never scans every tenant dir.
func workspaceGatesRule(w *Workspace, lead string) string {
	on := workspaceGatesOn(w)
	rules := map[string]string{
		"evidence":   "evidence = proof file in <evidence>/<tenant>/<ID>/ before done",
		"acceptance": "acceptance = a checkable test on every outcome (mw edit ID -c, humans only)",
		"judge":      "judge = only the human closes an outcome; sessions never judge their own work",
	}
	var parts []string
	for _, n := range gateNames {
		if on[n] {
			parts = append(parts, rules[n])
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return lead + strings.Join(parts, "; ") + "."
}

// workspaceGatesOn is the set of gates effectively on in any tenant that
// carries gate state.
func workspaceGatesOn(w *Workspace) map[string]bool {
	on := map[string]bool{}
	for i := range w.Tenants {
		if t := &w.Tenants[i]; len(t.Gates) == 0 && len(t.GateLatch) == 0 && len(t.GatesSeen) == 0 {
			continue
		}
		for _, g := range gateStates(w, &w.Tenants[i]) {
			if g.Effective {
				on[g.Name] = true
			}
		}
	}
	return on
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func powerShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func changeDir(goos, dir string) string {
	if goos == "windows" {
		return "Set-Location " + powerShellQuote(dir)
	}
	return "cd " + shellQuote(dir)
}
