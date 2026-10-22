package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	toon "github.com/toon-format/toon-go"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

var flagAliases = map[string]string{
	"t": "tenant", "s": "session", "a": "agent", "p": "parent",
	"d": "path", "b": "body", "c": "acceptance", "k": "kind",
	"S": "status", "n": "limit", "j": "json", "A": "all", "v": "verbose",
	"w": "wave", "o": "out",
}

var validOptions = map[string]bool{
	"tenant": true, "session": true, "agent": true, "parent": true, "tag": true, "archived": true,
	"path": true, "body": true, "acceptance": true, "kind": true,
	"status": true, "limit": true, "json": true, "all": true, "verbose": true,
	"na": true, "wave": true, "by": true, "no-evidence": true, "evidence-gap": true, "workspace": true, "na-rest": true, "dry-run": true,
	"since": true, "lookback": true, "claim": true, "headless": true, "release": true, "older-than": true, "except": true, "prompt": true, "days": true, "html": true, "out": true,
}

// exitError carries a specific exit code. quiet means the details were already printed
// (for -j output on stdout); the one-line reason still goes to stderr.
type exitError struct {
	code  int
	msg   string
	quiet bool
}

func (e *exitError) Error() string { return e.msg }

func main() {
	os.Exit(mainExit(os.Args[1:], os.Stderr))
}

// mainExit runs mw and returns the exit code. Every non-zero exit prints a one-line
// "mw: <reason>" on stderr (an exitError with no message gets one, a panic included), so a caller never sees a bare failure.
func mainExit(args []string, stderr io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "mw: internal error: %s\n", oneLine(fmt.Sprint(r)))
			code = 70
		}
	}()
	return mainExitErr(run(args), stderr)
}

// mainExitErr maps run's error to an exit code and prints its reason.
func mainExitErr(err error, stderr io.Writer) int {
	if err == nil {
		return 0
	}
	code := 1
	var ee *exitError
	if errors.As(err, &ee) {
		code = ee.code
		if code == 0 {
			code = 1
		}
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		msg = fmt.Sprintf("failed (exit %d)", code)
	}
	fmt.Fprintln(stderr, "mw:", msg)
	return code
}

// oneLine trims a message and folds it to its first line.
func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

func run(args []string) error {
	resetRunState()
	savedCount, lastSaved = 0, nil
	if len(args) == 0 {
		return usage("")
	}
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		topic := ""
		if len(args) > 1 {
			topic = args[1]
		}
		return usage(topic)
	}
	for _, arg := range args[1:] {
		if arg == "--help" || arg == "-h" {
			return usage(args[0])
		}
	}
	if args[0] == "init" {
		name := filepath.Base(mustGetwd())
		if len(args) > 1 {
			name = strings.Join(args[1:], " ")
		}
		return initWorkspace(name)
	}
	if args[0] == "description" {
		return descriptionCommand(args[1:])
	}
	if args[0] == "version" {
		return versionCommand(args[1:])
	}
	if args[0] == "update" {
		return updateCommand(args[1:])
	}
	if args[0] == "migrate" {
		return migrateCommand(args[1:])
	}
	path, err := findWorkspace()
	if err != nil {
		if args[0] == "dirs" {
			return dirsCommand(nil, "", args[1:])
		}
		return err
	}
	idx, err := loadScoped(path, false)
	if err != nil {
		return err
	}
	sc := scopeOf(idx, args)
	unlock := func() {}
	if !readOnly(args) { // reads take no lock; a writer locks only the tenants and index it writes
		for try := 0; ; try++ {
			slugs := sc.slugs
			if sc.all {
				slugs = nil
				for _, t := range idx.Tenants {
					slugs = append(slugs, t.Slug)
				}
			}
			if unlock, err = lockSet(path, sc.index, slugs); err != nil {
				return err
			}
			if !sc.all {
				break
			}
			// the tenant list may have changed between the first read and the locks: re-read and compare
			again, err2 := loadScoped(path, false)
			if err2 == nil && sameTenants(idx, again) {
				break
			}
			unlock()
			if err2 != nil {
				return err2
			}
			if try >= 5 {
				return errors.New("tenant list keeps changing: retry")
			}
			idx = again
			sc = scopeOf(idx, args)
		}
	}
	defer unlock()
	var w *Workspace
	if sc.all {
		w, err = load(path)
	} else {
		w, err = loadScoped(path, false, sc.slugs...)
	}
	if err != nil {
		return err
	}
	if err := dispatch(path, w, args); err != nil {
		return err
	}
	unlock()              // publish and hash work happen after the lock is released
	playPendingFanfare(w) // spawn after the locks are gone: the collection may sit on a slow mount
	if savedCount > 0 {
		afterWrite(path)
	} else {
		publishPending(path, idx)
	}
	return nil
}

func dispatch(path string, w *Workspace, args []string) error {
	switch args[0] {
	case "dirs":
		return dirsCommand(w, path, args[1:])
	case "note":
		return noteCommand(path, w, args[1:])
	case "tenant":
		return tenantCommand(path, w, args[1:])
	case "session":
		return sessionCommand(path, w, args[1:])
	case "get-prompt":
		return getPromptCommand(path, w, args[1:])
	case "add":
		return addItem(path, w, args[1:])
	case "list", "ls":
		return listItems(w, args[1:])
	case "show":
		return showItem(path, w, args[1:])
	case "ready", "start", "block", "done", "reopen":
		return moveCommand(path, w, args[0], args[1:])
	case "log":
		return logCommand(w, args[1:])
	case "wave":
		return waveCommand(path, w, args[1:])
	case "fanfare":
		return fanfareCommand(path, w, args[1:])
	case "jobs":
		return jobsCommand(path, w, args[1:])
	case "gate":
		return gateCommand(path, w, args[1:])
	case "edit":
		return editCommand(path, w, args[1:])
	case "review":
		return reviewCommand(w, args[1:])
	case "judge":
		return judgeCommand(w, args[1:])
	case "accept":
		return acceptCommand(path, w, args[1:])
	case "reject":
		return rejectCommand(path, w, args[1:])
	case "next":
		return nextItem(path, w, args[1:])
	case "board":
		return boardCommand(path, w, args[1:])
	case "archive":
		return archiveCommand(path, w, args[1:])
	case "context":
		return contextCommand(w, args[1:])
	default:
		return fmt.Errorf("unknown command %q (try mw help)", args[0])
	}
}

// readOnly reports whether the command only reads the board (shared lock).
func readOnly(args []string) bool {
	has := func(flag string) bool {
		for _, a := range args[1:] {
			if a == flag {
				return true
			}
		}
		return false
	}
	sub := ""
	if len(args) > 1 {
		sub = args[1]
	}
	switch args[0] {
	case "list", "ls", "show", "log", "board", "review", "judge", "context", "get-prompt":
		return true
	case "next":
		return !has("--claim")
	case "tenant":
		if sub == "protocol" { // only set and reset write
			return !has("set") && !has("reset")
		}
		return sub == "list"
	case "session":
		return sub == "list"
	case "wave":
		return sub == "show" || sub == "list"
	case "dirs":
		return sub != "set" && sub != "init"
	case "fanfare":
		return sub != "set"
	case "jobs":
		return sub == "show"
	}
	return false
}

func usage(topic string) error {
	if topic == "ls" {
		topic = "list"
	}
	help := map[string]string{
		"description": `mw description [-j]
Explain what miniwaves does and show the human and coding-assistant workflows.
Works before initialization. Output is TOON by default; -j selects JSON.`,
		"migrate": `mw migrate [--dry-run] [PATH]
Upgrade an old workspace file (schema 5, 6 or 7) to schema 8 in place (the index stays .miniwaves.json, each tenant moves to .miniwaves/<slug>.json); PATH defaults to the
.miniwaves.json found upward from the current folder. Backs it up to <file>.bak<schema> (e.g. .miniwaves.json.bak7) and
prints what changed; --dry-run prints without writing. The only command that reads old schemas
(see MIGRATION.md).`,
		"init": `mw init [WORKSPACE NAME]
Create .miniwaves.json (the index) and .miniwaves/<slug>.json in the current folder with one tenant.
Example: mw init "My apps"`,
		"tenant": `mw tenant add SLUG [NAME...] [-d DIR]
mw tenant list [-j] [-v] [--tag T]
mw tenant default [SLUG|-]
mw tenant archive [--force]|restore SLUG
mw tenant tag SLUG +TAG -TAG...
mw tenant protocol SLUG [set STEP...|reset]   (--workspace for the workspace list)
Manage isolated apps/projects. "default" selects a tenant for the current
folder ("-" clears it); otherwise the tenant whose -d DIR holds the cwd, MW_TENANT, then the first tenant that is not archived.
"archive" refuses while an item is doing or a session is live (--force overrides) and hides a tenant (files untouched) from board -A, list -A, log -A, next, the page and version -j
runners; -t SLUG on it fails until "restore". list -v shows archived ones. "tag" adds/removes lowercase
tags (printed sorted); --tag T (repeatable, AND) filters tenant list, board -A, list -A and log -A. Example: mw tenant add api "API" -d ~/apps/api`,
		"dirs": `mw dirs [init] [-j]
mw dirs set NAME PATH
Show the eight folders: datalake (inputs), docs (examples/specs), data (outputs),
tools, templates, builds, evidence, scratch; defaults ~/<name>. "init" creates missing folders; "set"
stores a path (relative = under home, ~/ expanded) in the board.
Example: mw dirs set data D:\out`,
		"note": `mw note ID TEXT... [-t SLUG] [-s NAME]
Append a timestamped note to any item or wave; shown by mw show.`,
		"session": `mw session start NAME [-t SLUG] [-a ROLE]
mw session list [-t SLUG] [-j] [-v]
mw session stop NAME [-t SLUG] [--release]   (-t limits the command to one tenant file)
mw session reap [-t SLUG | -A] [--older-than DUR] [--except NAME,NAME] [-n|--dry-run] [-j]
Register and inspect tenant-bound coding-assistant sessions. Registering is optional:
mw start with an unknown -s/MW_SESSION name registers it silently (no other command does).
With no session, start uses the implicit session "local" of the tenant (id local@SLUG), which
may own many items; "local" cannot be named with -s or session start. start NAME is only needed
to set a role (-a).
stop refuses while the session owns an item; --release first sets its doing items back to ready (owner
cleared, a note appended) and then stops it. reap releases and stops every session idle longer than
--older-than (default 2h; 90m, 2h, 1d) in -t TENANT or all tenants (-A); --except NAME,NAME skips those sessions; -n/--dry-run only prints. A
session's last activity is set by session start and by every start/ready/block/done/reopen, note and
edit it runs. The implicit local session is never reaped.`,
		"get-prompt": `mw get-prompt session NAME [-t SLUG] [-a ROLE] [--headless] [-j]
mw get-prompt dispatcher|coach [NAME] [-j]
Print a minimal embedded prompt to paste into an assistant. Retrieval changes no state.
session: exit code 3 when the session NAME already exists (-j prints {"error":"session_exists","session":NAME}).
--headless drops the Setup block, Loop, folders hint and reply format (a runner handles those) and
says the item is already claimed.`,
		"add": `mw add KIND TITLE... [-t SLUG] [-p ID] [-w WAVE] [-b TEXT] [-c TEXT] [-S STATUS]
Add work in inbox. KIND: prompt, story, task, spike, bug, or chore. -c is the
acceptance check (long form --acceptance). -S done is
refused while the evidence or judge gate is on.
Example: mw add task "Build index" -p STR-001`,
		"edit": `mw edit ID [-c TEXT] [-b TEXT] [-p PARENT|-] [-t SLUG]
Change an item's acceptance (-c), body (-b) and/or parent (-p; "-" clears; kind hierarchy and
cycles are enforced, waves are not parents: use mw wave add). A session may set -c only while
the acceptance is empty (drafting); changing a non-empty acceptance is humans only.`,
		"fanfare": `mw fanfare [-j]
mw fanfare set enabled=true milestone_seconds=15 finale_seconds=60 volume=1 stop_dialog=true dir=~/fanfare-finale-songs
mw fanfare test [milestone|finale]
mw fanfare fetch
mw fanfare list [-j]
mw fanfare stop
Play a short clip when a wave closes and a longer finale when a tenant is finished (items exist, none inbox/ready/doing/blocked, no open wave; once per transition).
Playback is a detached player (afplay, ffplay, mpv, paplay, Windows .wav) and never fails a board command. "fetch" downloads public-domain, CC0 and CC-BY pairs; the
bring-your-own pair folders stay empty until you add fanfare.<ext> and finale.<ext>. Example: mw fanfare set enabled=true`,
		"jobs": `mw jobs enable ID -t SLUG...
mw jobs disable ID [-t SLUG]
mw jobs set ID KEY=VALUE...
mw jobs show [ID] [-j]
Coordinated-job config (data only; mw starts no sessions). Jobs live in .miniwaves.json, a tenant joins one with enable.
-t repeats or takes a comma list. disable clears the tenants (all, or -t SLUG) and keeps the job's data.
Keys (defaults): title (the id), max_workers 1..32 (4), worktrees on|off (on), lock (a command name, none),
news.out (docs/news/), news.url (http/https), summary (none), pane on|off (on), pane.style ([a-z0-9-]{1,32}).
news.out and summary must be relative, inside the workspace (no .., no absolute path, no ~). An empty VALUE resets a key.
Job ids match ^[a-z0-9][a-z0-9-]{0,31}$. Example: mw jobs enable tiny -t sheet,doc && mw jobs set tiny max_workers=3`,
		"gate": `mw gate [-t SLUG] [-j]
mw gate level trust|proof|spec|judged [-t SLUG | --workspace] [--by NAME]
mw gate evidence|acceptance|judge on|off|auto [-t SLUG] [--by NAME]
mw gate evidence|acceptance|judge on|off|auto --workspace [--by NAME]   (workspace default for every tenant)
Levels are cumulative: trust = no gates; proof (default) = evidence; spec = evidence + acceptance; judged = all three.
Gates are per tenant. Without arguments: the level, then setting, effective state and reason per gate (-j: {level, gates}).
evidence: done needs a non-empty file in <evidence>/<tenant>/<ID>/. acceptance: next skips
prompts/stories with no acceptance (own or an ancestor's); tasks are never skipped. judge: a human accepts or rejects finished work (mw review).
auto: evidence = on (level proof; reason notes a product tenant or latch); judge and acceptance auto = off. on/off wins.
Humans only (refused under MW_SESSION or -s); --by NAME is required when stdin is not a terminal.`,
		"review": `mw review [-t SLUG] [-j]
What needs a human: judge (judgeable items awaiting judgement, ×N = rejections; [] while the
judge gate is off), blocked, acceptance (needs acceptance; only with that gate on) and waves
ready to close. -j prints {judge, blocked, acceptance, waves}.`,
		"judge": `mw judge ID [-t SLUG]
Three lines to decide on: Outcome (title, status, verification), Evidence (the item's folder; a story
adds each child's), Decision (the effective acceptance as a checklist). Judgeable = a story or a
standalone work item.`,
		"accept": `mw accept ID [-t SLUG] [--by NAME]
Human only (refused under MW_SESSION or -s; --by NAME when stdin is not a terminal). Marks a done
judgeable item judged; a story whose children are all done becomes done and judged together.`,
		"reject": `mw reject ID REASON... [--evidence-gap] [-t SLUG] [--by NAME]
Human only. Counts an attempt, notes the reason and clears judgement. A story goes back to ready with a
ready task "Fix: REASON"; a child goes to ready and reopens its done story. Refused in a closed wave.
--evidence-gap latches the evidence gate.`,
		"update": `mw update [--check]
Replace this mw with the latest GitHub release for this OS/CPU (checksum verified; on Windows the old
binary is kept as mw.exe.old). --check only reports. Exit 2 when already current.`,
		"version": `mw version [-j]
Print the mw version and schema; -j prints {version, schema, board_schema, board, tenants}: the nearest
workspace file and its schema number (null when there is none; an old schema is reported, not an error)
and, for a current-schema board, tenants [{slug, name, dir, tags, archived}] so a runner needs no tenant list
call (a runner skips archived ones).`,
		"list": `mw list [-t SLUG] [-k KIND] [-S STATUS] [-n N] [-j] [-v]
mw list -A [-s SESSION] [-k KIND] [-S STATUS] [-n N] [-j] [-v]
List items as compact TOON. Use -j for JSON or -v for full fields; -n N keeps the first N.
Waves are listed apart: text gains a waves section when any match; -j always prints {items, waves}.
-A lists every tenant (rows gain a tenant column, waves are left out); with -s SESSION only the
items that session owns.`,
		"show": `mw show ID [-t SLUG] [-j] [-v]
One call, everything: title, parent/wave/session, body, effective acceptance and where it comes
from, evidence folder (path + file count), children with status, notes, the last 5 events.
-v adds raw timestamps (ignored with -j). Falls back to the archive (mw archive) for archived items.
Example: mw show TSK-001`,
		"ready": `mw ready ID [-t SLUG] [-s NAME]
Make an inbox or blocked item available for work.`,
		"start": `mw start ID [-t SLUG] [-s NAME] [-a ROLE]
Claim inbox/ready work and mark it doing (anything else is refused: "start: ID is done — use
mw reopen"; re-starting your own item is fine). A named session can own one item; an unknown name is
registered on first use, with role ROLE when -a is given. Without -s/MW_SESSION the tenant's implicit session "local" claims it
(it may own many items); done/block without -s are attributed to the item's owner.`,
		"block": `mw block ID REASON... [-t SLUG] [-s NAME]
Mark work blocked and record the required reason.`,
		"done": `mw done ID VERIFICATION... [--evidence PATH]... [--no-evidence REASON] [-t SLUG] [-s NAME]
Complete work with evidence. --evidence copies the files (or a folder's files) into
<evidence>/<tenant>/<ID>/ before the gate check, so done is one command. One path per --evidence (also --evidence=PATH), repeat the flag for more; a word after the path is a note. Parents cannot close while children remain open.
With the evidence gate on, <evidence>/<tenant>/<ID>/ must hold a non-empty file
(not a dotfile, Thumbs.db or desktop.ini); --no-evidence REASON records a note instead.`,
		"reopen": `mw reopen ID [-t SLUG] [-s NAME]
Return an item to inbox and release its session owner.`,
		"log": `mw log [-t SLUG | -A] [-n N] [-j]
Activity feed, newest first (default 20): time, tenant, item, action, session, note. Actions: created,
ready, start, block, done, reopen, accept, reject, note, edit, step, unstep. -A spans all tenants.`,
		"wave": `mw wave create TITLE... [-t SLUG] [-p WAVE] [-b TEXT]
mw wave add|remove WAVE ID... [-t SLUG]
mw wave step WAVE KEY=NOTE [KEY=NOTE...] [--na] [-t SLUG]   (several steps in one call; --na applies to all)
mw wave step WAVE --na-rest REASON [-t SLUG]   (every unticked step becomes n/a)
mw wave unstep WAVE KEY [-t SLUG]
mw wave show WAVE [-t SLUG] [-j]
mw wave list [-t SLUG | -A] [-j]
mw wave close WAVE [--no-evidence REASON] [-t SLUG]
A wave is a milestone bundle: members carry the wave in their own field (parent
is untouched; mw add KIND TITLE --wave WAVE). Waves start ready; only wave
commands and reopen (which resets every step) apply to them. Every wave carries
the delivery checklist (default: audit, adversarial, wrapup, commit, merge, test,
bump, install, cleanup, plan, plan-audit; override with a top-level "protocol"
list in .miniwaves.json). close refuses until all members are done and every
step is ticked (--na ticks "not applicable" with a reason). Closed waves accept
no new members or steps. With the evidence gate on, close also needs
<evidence>/<tenant>/WAV-NNN/ with a file (or --no-evidence REASON).`,
		"next": `mw next [-t SLUG] [-s NAME] [-k KIND] [-j] [-v]
mw next --claim [-s NAME] [-a ROLE] [-t SLUG] [-k KIND] [-j] [--prompt]
Show the first ready, unowned item as the full brief (same as mw show). With the
acceptance gate on it skips work without acceptance ("skipped N without
acceptance"). -j prints null (exit 0) when nothing is ready, and adds gates and skipped
(plus "skipped: N (...)" on stderr). --claim picks and starts in one locked step (like
mw start -a ROLE) and prints the same JSON plus "claimed": true; with --prompt (and -j) the JSON also
carries "prompt", the headless session prompt for role -a, so a runner needs no get-prompt call.`,
		"archive": `mw archive [--days N] [-t SLUG] [--dry-run]
Move done items older than N days (default 30) with their events into .miniwaves/<slug>.archive.json next
to the tenant file. Waves and parents with open children stay. show ID still finds archived items; board
totals keep counting them ("DONE (n, +N archived)"). Archived items leave mw log and today counts,
and cannot be reopened (no unarchive). Without -t every tenant.`,
		"board": `mw board [-t SLUG | -A] [-v] [-j]
mw board --html [-o PATH]   one static page for all tenants (no JS); PATH defaults to publish.html
("publish": {"html": PATH, "auto": true, "interval": 60} in .miniwaves.json regenerates it after a write when the
board changed and the last write is at least interval seconds old: default 60, minimum 5, lower values clamp to 5;
an explicit mw board --html always writes; the hash is kept in PATH.hash); no path = stdout
Show a small progress bar, a "last: <ago> <item> <action>  today: N done" line and active work.
-A shows every tenant, most recently active first; -v includes history. A gates line appears when any gate is on.`,
		"context": `mw context [-t SLUG] [-s NAME] [-j]
Show a compact startup brief for one coding-assistant session.`,
	}
	if topic != "" {
		text, ok := help[topic]
		if !ok {
			return fmt.Errorf("unknown help topic %q", topic)
		}
		fmt.Printf("%s\n\nCommon names when shown: -t/--tenant, -s/--session, -j/--json, -v/--verbose.\nRun `mw help` for all commands and flag mappings.\n", text)
		return nil
	}
	fmt.Print(`miniwaves — a tiny multi-tenant board for coding assistants

Commands use the nearest .miniwaves.json workspace. Read output is compact
TOON by default; use -j for JSON and -v for more detail.

Usage:
  mw description [-j]
  mw migrate [--dry-run] [PATH]
  mw version [-j]
  mw update [--check]
  mw init [workspace name]
  mw tenant add SLUG [NAME...] [-d DIR]
  mw tenant list [-j] [-v] [--tag T] | default [SLUG|-] | archive|restore SLUG | tag SLUG +T -T
  mw dirs [init] [-j] | set datalake|docs|data|tools|templates|builds|evidence|scratch PATH
  mw note ID TEXT... [-t SLUG] [-s NAME]
  mw session start NAME [-t SLUG] [-a ROLE]
  mw session list [-t SLUG] [-j] [-v] | stop NAME [--release] | reap [-t SLUG | -A] [--older-than DUR] [--except NAMES] [-n] [-j]
  mw get-prompt session NAME [-t SLUG] [-a ROLE] [--headless] [-j]
  mw get-prompt dispatcher|coach [NAME] [-j]
  mw add KIND TITLE... [-t SLUG] [-p ID] [-w WAVE] [-b TEXT] [-c TEXT]
  mw edit ID [-c TEXT] [-b TEXT] [-p PARENT|-]
  mw jobs enable ID -t SLUG... | disable ID [-t SLUG] | set ID KEY=VALUE... | show [ID] [-j]
  mw gate [-t SLUG] [-j] | gate level trust|proof|spec|judged | gate evidence|acceptance|judge on|off|auto [--by NAME]
  mw review [-t SLUG] [-j]
  mw judge ID [-t SLUG]
  mw accept ID [--by NAME] | mw reject ID REASON... [--evidence-gap] [--by NAME]
  mw list [-t SLUG | -A [-s SESSION]] [-k KIND] [-S STATUS] [-n N] [-j] [-v]
  mw show ID [-t SLUG] [-j] [-v]
  mw ready|reopen ID [-t SLUG] [-s NAME]
  mw start ID [-t SLUG] [-s NAME] [-a ROLE]
  mw block ID REASON... [-t SLUG] [-s NAME]
  mw done ID VERIFICATION... [--evidence PATH]... [--no-evidence REASON] [-t SLUG] [-s NAME]
  mw log [-t SLUG | -A] [-n N] [-j]
  mw wave create|add|remove|step|unstep|show|list|close ...  (mw help wave)
  mw next [-t SLUG] [-s NAME] [-k KIND] [-j] [-v] | mw next --claim [-s NAME] [-a ROLE] [-j] [--prompt]
  mw board [-t SLUG | -A] [-v] [-j] | mw board --html [-o PATH]
  mw archive [--days N] [-t SLUG] [--dry-run]
  mw context [-t SLUG] [-s NAME] [-j]

Kinds: prompt, story, task, spike, bug, chore (+ wave via mw wave create)
States: inbox, ready, doing, blocked, done
Environment: MW_TENANT, MW_SESSION, MW_LOCK_WAIT (seconds a writer waits for the board lock; default 5, Windows 15)
Flags: -t tenant, -s session, -a agent, -p parent, -d path, -b body,
       -c acceptance, -k kind, -S status, -n limit, -j json, -v verbose,
       -A all

Run mw help COMMAND or mw COMMAND --help for explanations and examples.
Releases: ` + releasesURL + ` · update: mw update
`)
	return nil
}

func descriptionCommand(args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return errors.New("usage: mw description [-j]")
	}
	if err := allowOptions(o, "json"); err != nil {
		return err
	}
	return printData(descriptionInfo{
		Purpose: "Keep user requests, desired outcomes, and coding work clear across tenants and assistant sessions.",
		Model:   "tenant=app; prompt=request; story=outcome; task/spike/bug/chore=work; session=assistant",
		Human: []string{
			"record a request as a prompt and its desired outcome as a story",
			"split the story into small work items, mark them ready, then review mw board",
			"judge completed work from mw review and mw judge ID, then mw accept ID or mw reject ID REASON; mw gate shows which proofs are required",
		},
		Assistant: []string{
			"registering is optional (mw session start NAME -t TENANT -a ROLE sets a role); mw start ID -s NAME registers an unknown name; no session = the per-tenant \"local\" one",
			"loop: mw next -j (full brief), mw start ID -a ROLE (or mw next --claim -j), work",
			"finish with mw done ID CHECK --evidence FILE (copies the proof into the evidence folder) or stop with mw block ID REASON",
		},
		Start:   []string{"mw init WORKSPACE", "mw add prompt REQUEST", "mw add story OUTCOME -p PROMPT_ID", "mw board"},
		Storage: "local .miniwaves.json; no server, account, or web interface",
		More:    "mw help COMMAND",
	}, o)
}

func dirsCommand(w *Workspace, path string, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if err := allowOptions(o, "json"); err != nil {
		return err
	}
	if len(pos) == 3 && pos[0] == "set" {
		if w == nil {
			return errors.New("no .miniwaves.json found; run `mw init`")
		}
		key := pos[1]
		if !validFolderKey(key) {
			return errors.New("usage: mw dirs set " + folderKeyList + " PATH")
		}
		if w.Folders == nil {
			w.Folders = map[string]string{}
		}
		if strings.TrimSpace(pos[2]) == "" || strings.ContainsAny(pos[2], "\n\r\t") {
			return errors.New("folder path must be non-empty and contain no control characters")
		}
		w.Folders[key] = pos[2]
		if err := save(path, w); err != nil {
			return err
		}
		fmt.Printf("%s: %s\n", key, workspaceFolders(w).field(key))
		return nil
	}
	if len(pos) > 1 || len(pos) == 1 && pos[0] != "init" {
		return errors.New("usage: mw dirs [init] [-j] | set NAME PATH (NAME: " + folderKeyList + ")")
	}
	dirs := workspaceFolders(w)
	if len(pos) == 1 {
		for _, key := range folderKeys {
			if err := os.MkdirAll(dirs.field(key), 0700); err != nil {
				return err
			}
		}
		fmt.Println("created " + strings.ReplaceAll(folderKeyList, "|", ", "))
		return nil
	}
	return printData(dirs, o)
}

func (f folderPaths) field(key string) string {
	switch key {
	case "datalake":
		return f.Datalake
	case "docs":
		return f.Docs
	case "tools":
		return f.Tools
	case "templates":
		return f.Templates
	case "builds":
		return f.Builds
	case "evidence":
		return f.Evidence
	case "scratch":
		return f.Scratch
	}
	return f.Data
}

var folderKeys = []string{"datalake", "docs", "data", "tools", "templates", "builds", "evidence", "scratch"}

const folderKeyList = "datalake|docs|data|tools|templates|builds|evidence|scratch"

func validFolderKey(k string) bool {
	for _, c := range folderKeys {
		if k == c {
			return true
		}
	}
	return false
}

// folderSentence is the three-line folder convention handed to agents.
// tenant and session may be empty (roots are shown with <app>/<ver> placeholders).
func folderSentence(f folderPaths, tenant, session string) string {
	app := "<app>"
	scratch := f.Scratch + string(filepath.Separator) + "<app>"
	if tenant != "" {
		app = tenant
		scratch = filepath.Join(f.Scratch, tenant)
		if session != "" {
			scratch = filepath.Join(scratch, session)
		}
	}
	return "Folders: code in the tenant repo (plans + ADRs + test fixtures inside); shared tooling " + f.Tools + ", scaffolds " + f.Templates + " (copy, never edit in place).\n" +
		"Artefacts " + filepath.Join(f.Builds, app) + string(filepath.Separator) + "<ver>; proof (screenshots, logs, crash pulls, repro fixtures) " + filepath.Join(f.Evidence, app) + string(filepath.Separator) + "<item>; throwaway " + scratch + ".\n" +
		"Inputs " + f.Datalake + ", generated outputs " + f.Data + ", shared reference/specs/plans " + f.Docs + string(filepath.Separator) + "<topic>. Never write elsewhere; never commit builds, evidence or scratch."
}

// resolveFolder expands one configured folder: "~/" and relative paths are
// under home; absolute paths (including Windows drive paths) are kept.
func resolveFolder(home, configured, def string) string {
	c := strings.TrimSpace(configured)
	c = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, c) // a hand-edited board must not inject lines into prompts
	switch {
	case c == "":
		return filepath.Join(home, def)
	case c == "~":
		return home
	case strings.HasPrefix(c, "~/") || strings.HasPrefix(c, `~\`):
		return filepath.Join(home, c[2:])
	case filepath.IsAbs(c):
		return filepath.Clean(c)
	}
	return filepath.Join(home, c)
}

// workspaceFolders returns the configured folders (w may be nil). If the home
// directory is unknown it falls back to the literal ~/ form.
func workspaceFolders(w *Workspace) folderPaths {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "~"
	}
	var cfg map[string]string
	if w != nil {
		cfg = w.Folders
	}
	return folderPaths{
		Datalake:  resolveFolder(home, cfg["datalake"], "datalake"),
		Docs:      resolveFolder(home, cfg["docs"], "docs"),
		Data:      resolveFolder(home, cfg["data"], "data"),
		Tools:     resolveFolder(home, cfg["tools"], "tools"),
		Templates: resolveFolder(home, cfg["templates"], "templates"),
		Builds:    resolveFolder(home, cfg["builds"], "builds"),
		Evidence:  resolveFolder(home, cfg["evidence"], "evidence"),
		Scratch:   resolveFolder(home, cfg["scratch"], "scratch"),
	}
}

func noteCommand(path string, w *Workspace, args []string) error {
	o, pos, err := options(args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return errors.New("usage: mw note ID TEXT... [-t SLUG] [-s NAME]")
	}
	if err := allowOptions(o, "tenant", "session"); err != nil {
		return err
	}
	t, err := selectTenant(w, o)
	if err != nil {
		return err
	}
	it, err := itemByID(t, pos[0])
	if err != nil {
		return err
	}
	text := strings.TrimSpace(strings.Join(pos[1:], " "))
	if text == "" {
		return errors.New("note text must not be blank")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	who := value(o, "session", os.Getenv("MW_SESSION"))
	if who != "" {
		who = "[" + who + "] "
	}
	it.Notes = append(it.Notes, now+" "+who+text)
	emit(it, now, "note", value(o, "session", os.Getenv("MW_SESSION")), text)
	touchSession(w, value(o, "session", os.Getenv("MW_SESSION")), now)
	it.Updated = now
	if err := save(path, w); err != nil {
		return err
	}
	fmt.Printf("[%s] %s note added (%d)\n", t.Slug, it.ID, len(it.Notes))
	return nil
}

func mustGetwd() string {
	d, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return d
}

func options(args []string) (map[string]string, []string, error) {
	o, rest := map[string]string{}, []string{}
	literal := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if literal {
			rest = append(rest, a)
			continue
		}
		if a == "--" {
			literal = true
			continue
		}
		key := ""
		if strings.HasPrefix(a, "--") {
			key = strings.TrimPrefix(a, "--")
		} else if strings.HasPrefix(a, "-") && len(a) == 2 {
			key = flagAliases[strings.TrimPrefix(a, "-")]
		} else {
			rest = append(rest, a)
			continue
		}
		if key == "" || !validOptions[key] {
			return nil, nil, fmt.Errorf("unknown option %s", a)
		}
		if key == "json" || key == "all" || key == "verbose" || key == "na" || key == "evidence-gap" || key == "workspace" || key == "dry-run" || key == "release" || key == "claim" || key == "headless" || key == "prompt" || key == "html" || key == "archived" {
			o[key] = "true"
			continue
		}
		if i+1 >= len(args) {
			return nil, nil, fmt.Errorf("--%s needs a value", key)
		}
		if key == "tag" && o[key] != "" { // repeatable: --tag a --tag b means both
			o[key], i = o[key]+","+args[i+1], i+1
			continue
		}
		o[key], i = args[i+1], i+1
	}
	return o, rest, nil
}

func allowOptions(o map[string]string, allowed ...string) error {
	ok := map[string]bool{}
	for _, key := range allowed {
		ok[key] = true
	}
	for key := range o {
		if !ok[key] {
			return fmt.Errorf("--%s is not valid here", key)
		}
	}
	return nil
}

func compactText(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit-1]) + "…"
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 64 {
		out = strings.TrimRight(out[:64], "-")
	}
	return out
}

func validID(s string) bool {
	return len(s) <= 64 && slugPattern.MatchString(s)
}

func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		fmt.Println(string(b))
	}
	return err
}

func printData(v any, o map[string]string) error {
	if o["json"] == "true" {
		return printJSON(v)
	}
	rv := reflect.ValueOf(v)
	if rv.IsValid() && (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array) && rv.Len() == 0 {
		fmt.Println("[]")
		return nil
	}
	b, err := toon.Marshal(v)
	if err == nil {
		fmt.Println(string(b))
	}
	return err
}
