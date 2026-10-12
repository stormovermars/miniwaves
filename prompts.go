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
	GatesRule                               string
}

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
	return promptWith(w, "session", promptData{Role: role, ID: id, Slug: t.Slug, Session: id, Headless: headless, TenantName: compactText(t.Name, 72),
		Setup:        sessionSetup(runtime.GOOS, dir, id, t.Slug, role),
		GateEvidence: byName(gs, "evidence").Effective, GateAcceptance: byName(gs, "acceptance").Effective, GateJudge: byName(gs, "judge").Effective,
		EvidenceDir: filepath.Join(workspaceFolders(w).Evidence, t.Slug, "<ID>") + string(filepath.Separator)})
}

func sessionSetup(goos, dir, id, tenant, role string) string {
	if goos == "windows" {
		return fmt.Sprintf("  Set-Location %s\n  $env:MW_SESSION=%s; $env:MW_TENANT=%s",
			powerShellQuote(dir), powerShellQuote(id), powerShellQuote(tenant))
	}
	return fmt.Sprintf("  cd %s\n  export MW_SESSION=%s MW_TENANT=%s",
		shellQuote(dir), shellQuote(id), shellQuote(tenant))
}

func getPromptCommand(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return errors.New("usage: mw get-prompt session NAME | dispatcher|coach [NAME]")
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
	case "dispatcher":
		if err := allowOptions(o, "json"); err != nil {
			return err
		}
		if len(pos) > 2 {
			return errors.New("usage: mw get-prompt dispatcher [NAME] [-j]")
		}
		name = "dispatcher"
		if len(pos) == 2 {
			name = pos[1]
		}
		prompt = buildDispatcherPrompt(path, w, name)
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
		return errors.New("usage: mw get-prompt session NAME | dispatcher|coach [NAME]")
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

func buildDispatcherPrompt(path string, w *Workspace, name string) string {
	return promptWith(w, "dispatcher", promptData{Name: name, Workspace: compactText(w.Name, 72),
		ChangeDir: changeDir(runtime.GOOS, filepath.Dir(path)), Tenants: len(w.Tenants), Sessions: len(w.Sessions),
		GatesRule: workspaceGatesRule(w, "Gates are per tenant (mw gate -t SLUG): "), GateJudge: workspaceGatesOn(w)["judge"]})
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
