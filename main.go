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
	"unicode"

	toon "github.com/toon-format/toon-go"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

var flagAliases = map[string]string{
	"t": "tenant", "s": "session", "a": "agent", "p": "parent",
	"d": "path", "b": "body", "c": "acceptance", "k": "kind",
	"S": "status", "n": "limit", "j": "json", "A": "all", "v": "verbose",
	"w": "wave", "o": "out", "m": "message",
}

var validOptions = map[string]bool{
	"tenant": true, "session": true, "agent": true, "parent": true, "tag": true, "archived": true,
	"path": true, "body": true, "acceptance": true, "kind": true,
	"status": true, "limit": true, "json": true, "all": true, "verbose": true,
	"na": true, "wave": true, "by": true, "no-evidence": true, "evidence-gap": true, "workspace": true, "na-rest": true, "dry-run": true,
	"since": true, "since-file": true, "follow": true, "exec": true, "poll": true, "kit": true, "job": true, "claim": true, "headless": true, "release": true, "full": true, "force": true, "older-than": true, "except": true, "prompt": true, "days": true, "html": true, "md": true, "out": true, "explain": true, "plugins": true, "ticket": true, "format": true, "width": true, "message": true, "on": true, "claude-session": true, "answerer": true, "accept": true, "action": true, "grep": true,
}

// exitError carries a specific exit code. quiet means the details were already printed
// (for -j output on stdout); the one-line reason still goes to stderr.
type exitError struct {
	code  int
	msg   string
	quiet bool
	ccode string // stable error code for -j (see errCode); empty = classify from the message
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
			printJSONError(coded("INTERNAL", errors.New("internal error")), code, args)
		}
	}()
	err := run(args)
	code = mainExitErr(err, stderr)
	if code != 0 {
		printJSONError(err, code, args)
	}
	return code
}

// codedError tags an error with a stable code (errCode) without changing its text or exit code.
type codedError struct {
	code string
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

// coded attaches a stable error code to err (nil stays nil).
func coded(code string, err error) error {
	if err == nil {
		return nil
	}
	return &codedError{code, err}
}

// errCode is the stable machine-readable code of an error, shown as "code" in -j errors (USAGE.md,
// "Error codes"). Sites that matter tag their error with coded; the rest is classified from the
// message prefixes below, and anything else is ERROR. The text and the exit code never depend on it.
func errCode(err error) string {
	var ce *codedError
	if errors.As(err, &ce) {
		return ce.code
	}
	var ee *exitError
	if errors.As(err, &ee) && ee.ccode != "" {
		return ee.ccode
	}
	m := strings.TrimSpace(err.Error())
	switch {
	case strings.HasPrefix(m, "board locked"):
		return "LOCKED"
	case strings.HasPrefix(m, "usage:"), strings.HasPrefix(m, "unknown option"), strings.HasPrefix(m, "unknown command"),
		strings.Contains(m, "needs a value"), strings.HasSuffix(m, "is not valid here"), strings.Contains(m, "requires a reason"):
		return "USAGE"
	case strings.Contains(m, " not found"):
		return "NOT_FOUND"
	}
	return "ERROR"
}

// printJSONError prints {"error":MSG,"code":CODE,"exit":N} on stdout for a failed command run with -j
// (stderr and the exit code are unchanged). An error that already printed its own JSON (quiet) is skipped.
func printJSONError(err error, code int, args []string) {
	var ee *exitError
	if errors.As(err, &ee) && ee.quiet {
		return
	}
	for _, a := range args {
		if a == "--" {
			return
		}
		if a == "-j" || a == "--json" {
			msg := strings.TrimSpace(err.Error())
			if msg == "" {
				msg = fmt.Sprintf("failed (exit %d)", code)
			}
			b, _ := json.Marshal(map[string]any{"error": msg, "code": errCode(err), "exit": code})
			fmt.Println(string(b))
			return
		}
	}
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
	if args[0] == "init" && !hasFlag(args[1:], "--kit") {
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
	if args[0] == "user" {
		return userCommand(args[1:])
	}
	if args[0] == "configure" {
		return configureCommand(args[1:])
	}
	if args[0] == "migrate" {
		return migrateCommand(args[1:])
	}
	if args[0] == "boards" {
		return boardsCommand(args[1:])
	}
	if args[0] == "context" && hasFlag(args[1:], "--hook-json") { // SessionStart hook: silent on any problem, always exit 0
		return hookContextCommand(os.Stdin, os.Stdout)
	}
	if args[0] == "status" { // read-only, silent on any problem, always exit 0
		return statusCommand(args[1:])
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
		return logCommand(path, w, args[1:])
	case "wave":
		return waveCommand(path, w, args[1:])
	case "fanfare":
		return fanfareCommand(path, w, args[1:])
	case "jobs":
		return jobsCommand(path, w, args[1:])
	case "news": // a write: it creates files (readOnly is false for it, so the board is locked)
		return newsCommand(path, w, args[1:])
	case "init": // only with --kit (a plain init runs before a workspace is loaded); it writes, so it is never in readOnly
		return initKit(path, w, args[1:])
	case "ask":
		return askCommand(path, w, args[1:])
	case "answer":
		return answerCommand(path, w, args[1:])
	case "decide":
		return decideCommand(path, w, args[1:])
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

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
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
		return sub != "set" && sub != "use" && sub != "on" && sub != "off" && !(sub == "sets" && (has("add") || has("drop")))
	case "jobs":
		return sub == "show"
	case "news": // stdout only (-o -, or --format text|json without -o; never --release) writes no file and no sidecar
		o, _, err := options(args[1:])
		return err == nil && o["release"] == "" && (o["out"] == "-" || (o["format"] == "text" || o["format"] == "json") && o["out"] == "")
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
		"status": `mw status --explain [--plugins DIR]
Explains the status line with live numbers for the nearest board, lists what needs Sir, shows the user settings and, with --plugins DIR
(the folder holding the installed plugins), the plugin versions and every slash command's description. Read-only; the /mw-status command runs it.

mw status --line [--link]
One short line for the Claude Code status bar: "mw:N2R1B1:12/40" = N needs you (an open ask, or blocked with no --on),
R awaiting review, B blocked on another item (each hidden at 0; an item counts once), then open/all items (inbox drafts
are in neither). Settings: "statusLine":{"type":"command","command":"mw status --line --link"}. Counts the nearest board (walking up from the cwd; or from .workspace.current_dir / .cwd of status-line JSON
on stdin); --link wraps the line in an OSC 8 hyperlink to the published page (jobs.artifact-url) or the file:// URL of jobs.md;
outside a board it sums the boards up to 2 folder levels below the cwd (no link unless exactly one; cached 30 s in the user cache folder) and prints nothing when there are none;
prints nothing when status.show is false or on any error; never writes a board; always exits 0.`,
		"boards": `mw boards [DIR] [--depth N] [--up] [-j]
Read-only discovery of mw boards: scans DIR (default .) down to --depth (1..6; default: your discover.depth, 4) for
.miniwaves.json; --up also adds the nearest ancestor board, never looking inside macOS-protected
folders (Documents, Desktop, Downloads, Library, Pictures, Movies, Music) unless DIR is inside one.
Skips dot folders, node_modules, build, dist, vendor, symlinks and, on Windows, AppData; capped at
200 boards and 5 s. Reads only the index files (an index over 1 MB is an error entry).`,
		"migrate": `mw migrate [--dry-run] [PATH]
Upgrade an old workspace file (schema 5 to 8) to schema 9 in place (the index stays .miniwaves.json, each tenant moves to .miniwaves/<slug>.json); PATH defaults to the
.miniwaves.json found upward from the current folder. Backs it up to <file>.bak<schema> (e.g. .miniwaves.json.bak7) and
prints what changed; --dry-run prints without writing. The only command that reads old schemas
(see MIGRATION.md).`,
		"init": `mw init [WORKSPACE NAME]
mw init --kit coordinated --job ID
Create .miniwaves.json (the index) and .miniwaves/<slug>.json in the current folder with one tenant.
Example: mw init "My apps"
--kit coordinated (inside an existing workspace) writes files only, into the workspace folder: PLAN.md (a plan skeleton), NEWSDESK.md (the
news desk's page spec), buildlock.sh and buildlock.ps1 (a mkdir lock with a timeout; BUILDLOCK_TIMEOUT seconds, default 1800; run ./buildlock.sh CMD...).
It never overwrites a file (existing ones are reported as skipped), creates job ID if missing like mw jobs enable with no tenants, and prints the
two get-prompt commands. Safe to repeat.`,
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
		"session": `mw session start NAME [-t SLUG] [-a ROLE] [--claude-session ID] [--answerer PARENT]
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
edit it runs. The implicit local session is never reaped.
--answerer PARENT names the session that may answer this session's asks (mw answer -s PARENT); refused for itself or a cycle.
Job roles: when the session's tenant belongs to a job that exists (mw jobs), reap also skips sessions whose role (-a) is dispatcher or newsdesk,
with no extra flag; --except still skips any named session.`,
		"get-prompt": `mw get-prompt session NAME [-t SLUG] [-a ROLE] [--headless] [-j]
mw get-prompt dispatcher|coach|newsdesk [NAME] [--job ID] [-j]
Print a minimal embedded prompt to paste into an assistant. Retrieval changes no state.
session: exit code 3 when the session NAME already exists (-j prints {"error":"session_exists","session":NAME}).
--headless drops the Setup block, Loop, folders hint and reply format (a runner handles those) and
says the item is already claimed.`,
		"add": `mw add KIND TITLE... [-t SLUG] [-p ID] [-w WAVE] [-b TEXT] [-c TEXT] [-S STATUS] [--ticket KEY]
Add work in inbox. KIND: prompt, story, task, spike, bug, or chore. -c is the
acceptance check (long form --acceptance). -S done is
refused while the evidence or judge gate is on.
Example: mw add task "Build index" -p STR-001`,
		"edit": `mw edit ID [-c TEXT] [-b TEXT] [-p PARENT|-] [ticket=KEY] [-t SLUG]
Change an item's acceptance (-c), body (-b) and/or parent (-p; "-" clears; kind hierarchy and
cycles are enforced, waves are not parents: use mw wave add). A session may set -c only while
the acceptance is empty (drafting); changing a non-empty acceptance is humans only.`,
		"fanfare": `mw fanfare [-j]
mw fanfare on | off
mw fanfare set enabled=true milestone_seconds=15 finale_seconds=60 volume=1 stop_dialog=true dir=~/fanfare-finale-songs
mw fanfare test [milestone|finale]
mw fanfare fetch
mw fanfare list [-j]
mw fanfare sets [-j]
mw fanfare sets add NAME PAIR...
mw fanfare sets drop NAME [PAIR...]
mw fanfare use NAME|all
mw fanfare stop
Play a short clip when a wave closes and a longer finale when a tenant is finished (items exist, none inbox/ready/doing/blocked, no open wave; once per transition).
Playback is a detached player (afplay, ffplay, mpv, paplay, Windows .wav) and never fails a board command. "fetch" downloads public-domain, CC0 and CC-BY pairs; the
bring-your-own pair folders stay empty until you add fanfare.<ext> and finale.<ext>. Example: mw fanfare set enabled=true`,
		"news": `mw news [--job ID] [--full] [--format md|html|text|json] [--tenant SLUG] [-o PATH|-] [-j]
mw news --job ID --release [--force] [--format ...] [-j]
The news bulletin of a job: one model (version 1: job, release, now, happened, next, metrics, summary) drawn as a
70-column gopher-style bulletin (md = one fenced text block, html = <pre>, text) or printed as json. Without --release it
is the draft since the latest release (release.number null). Default: every job, one file each at <news.out>/status.md
(.html), or the workspace when no job exists (docs/news/); the file is rewritten only when the content changed (hash in
PATH.hash), otherwise it prints "unchanged". text and json print to stdout (a file only with -o PATH). -o - prints to stdout.
--release cuts release N+1 under the board lock: N.json (frozen model), N.md, N.html and latest in the releases folder
(job key news_releases, default <news.out dir>/releases). Release N compares with release N-1 (release 1 with the start).
Nothing new since the last release is refused with exit 0 ("nothing new since release N") unless --force.
The job's metrics file (key metrics, see USAGE.md) and summary file (key summary, at most 20 KB) are read as data.
--full lists every row (no per-list caps, every more is 0; the 200-character item text and the 1 MB guard stay; a guard trim prints the line "trimmed: N rows", json trimmed_rows); refused with --release, and refused unless the output goes to stdout (-o -, or --format text|json without -o) or a file (-o FILE): it never rewrites the desk's status.md draft. --tenant SLUG (json only) filters the model to one tenant; the window stays job-wide. --width (text only) is accepted and ignored: bulletins are 70 columns. No network. Example: mw news --job tiny --release`,
		"jobs": `mw jobs enable ID -t SLUG...
mw jobs disable ID [-t SLUG]
mw jobs set ID KEY=VALUE...
mw jobs show [ID] [-j]
Coordinated-job config (data only; mw starts no sessions). Jobs live in .miniwaves.json, a tenant joins one with enable.
-t repeats or takes a comma list. disable clears the tenants (all, or -t SLUG) and keeps the job's data.
Keys (defaults): title (the id), description (none, one line ≤120), max_workers 1..32 (4), worktrees on|off (on), lock (a command name, none),
news.out (docs/news/<job>/), news.url (http/https), news_releases (<news.out>/releases), metrics (none), tracker (none; jira linear youtrack github gitlab azure shortcut clickup asana), tracker_url (https base URL), summary (none).
news.out, news_releases, metrics and summary must be relative, inside the workspace (no .., no absolute path, no ~). An empty VALUE resets a key.
Job ids match ^[a-z0-9][a-z0-9-]{0,31}$. Example: mw jobs enable tiny -t sheet,doc && mw jobs set tiny max_workers=3`,
		"ask": `mw ask ID QUESTION... [-t SLUG] [-s NAME]
Record a question that needs a human, as an "ask" event on the item (any board, no gate; the status is unchanged).
It stays open until an answer follows it on the same item. mw review shows it as "needs you" (-j: needs), and so does mw board.`,
		"answer": `mw answer ID TEXT... --by NAME [-t SLUG]
mw answer ID --accept --by NAME [-t SLUG]
Human only (refused under MW_SESSION or -s; --by NAME when stdin is not a terminal). Appends an "answer" event that
closes the item's open ask; refused when the item has none. --accept takes the ask as asked: it first records a
"decide" event "accepted: <the ask>" on the item, then the answer "accepted".`,
		"decide": `mw decide TEXT... [--by NAME] [-s SESSION] [-b WHY] [-p ID] [-t SLUG]
Open to humans and sessions: with -s (or MW_SESSION) the event is recorded by that session and --by NAME is kept in the
note as "(for NAME)"; else by --by NAME (required when stdin is not a terminal), else the login name. Record a decision as a "decide" event on item ID (-p), else on the tenant's "Decisions" chore, which is created
(already done, so it holds nothing open) on first use. -b WHY is kept in the note as "(why: WHY)". Any board, no gate.`,
		"user": `mw user show [-j]
mw user get [KEY] [-j]
mw user set KEY=VALUE...
mw user fanfare-defaults
Per-user choices, kept out of the committed board, in os.UserConfigDir()/miniwaves/mw-user.json (MW_USER_CONFIG overrides the path).
Works without a board. get lists every settings key (or one) with its effective value and source (default|user); -j:
{"keys":[{"key","value","default","source","group","choices"}]}. Keys: fanfare.enabled, fanfare.use, status.show,
discover.home, discover.depth (see mw configure). Also alerts.on (comma list of event names) and alerts.exec (a JSON argv
array, never run through a shell). fanfare.enabled overrides the board's fanfare enabled. An empty VALUE unsets a key.
On Unix a file that is group- or world-writable is refused (chmod 600); on Windows there is no mode check.
fanfare-defaults sets fanfare.enabled=true and fanfare.use=all only where they are unset and never overwrites (install and update run it; MW_NO_FANFARE=1 or MW_NO_PLUGINS=1 skips).
Example: mw user set fanfare.enabled=false status.show=false`,
		"configure": `mw configure [--show [-j]] | mw configure KEY=VALUE...
Walk your per-user settings (Fanfare, Chat bar, Discovery) with plain prompts: Enter keeps the value, a number
picks a choice, d resets to the default, t test-plays a fanfare. A summary and Save/Discard prompt end the walk; nothing is
written before Save. --show prints the settings like mw user get. KEY=VALUE... validates and saves without prompting.
Without a terminal on stdin it prints the settings and a hint, and exits 0. Works without a board.`,
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
whose members are all done (review-ready, even with delivery steps unticked; each carries closeable: true when mw wave close would pass now). "needs you" lists open asks (mw ask, closed by mw answer). -j prints {judge, review, blocked, acceptance, waves, stale, needs}. review (new in 0.3.0, always present, [] when empty) lists the blocked items whose block reason starts "awaiting review", the same rule mw news uses; they also stay in blocked, so scripts that count blocked keep working. Text output adds a review section when there are any.`,
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
		"update": `mw update [--check] [--plugins]
Replace this mw with the latest GitHub release for this OS/CPU (checksum verified; on Windows the old
binary is kept as mw.exe.old). --check only reports. Exit 2 when already current. After a real update it
also installs/updates the Claude Code plugins mw and mw-jobs and, once that worked, removes the retired
tidewatch and mw-watch (best effort; MW_NO_PLUGINS=1 skips; claude honours CLAUDE_CONFIG_DIR).
--plugins runs only that plugin step.`,
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
		"show": `mw show ID [-t SLUG] [-j] [-v] [--full]
One call, everything: title, parent/wave/session, body, effective acceptance and where it comes
from, evidence folder (path + file count), children with status, notes (always in full), the last 5 events plus the last 10 reject, answer and decide events
(each event note cut to 90 characters). --full shows every event with its note in full (also with -j).
-v adds raw timestamps (ignored with -j). Falls back to the archive (mw archive) for archived items.
Example: mw show TSK-001`,
		"ready": `mw ready ID [-t SLUG] [-s NAME]
Make an inbox or blocked item available for work.`,
		"start": `mw start ID [-t SLUG] [-s NAME] [-a ROLE]
Claim inbox/ready work and mark it doing (anything else is refused: "start: ID is done — use
mw reopen"; re-starting your own item is fine). A named session can own one item; an unknown name is
registered on first use, with role ROLE when -a is given. Without -s/MW_SESSION the tenant's implicit session "local" claims it
(it may own many items); done/block without -s are attributed to the item's owner.`,
		"block": `mw block ID REASON... [--on ID] [-t SLUG] [-s NAME]
Mark work blocked and record the required reason. Without --on the item waits for a human (the status line counts it
as N, needs you); --on ID names the open item it waits for (counted as B, blocked); a done item is refused. When that item is final
(done, and accepted when the judge gate is on; a wave: closed) the blocked item returns to ready with an event naming it.`,
		"done": `mw done ID VERIFICATION... [--evidence PATH]... [--no-evidence REASON] [-t SLUG] [-s NAME]
Complete work with evidence. --evidence copies the files (or a folder's files) into
<evidence>/<tenant>/<ID>/ before the gate check, so done is one command. One path per --evidence (also --evidence=PATH), repeat the flag for more; a word after the path is a note. Parents cannot close while children remain open.
With the evidence gate on, <evidence>/<tenant>/<ID>/ must hold a non-empty file
(not a dotfile, Thumbs.db or desktop.ini); --no-evidence REASON records a note instead.`,
		"reopen": `mw reopen ID [-t SLUG] [-s NAME] [-m REASON]
Return an item to inbox and release its session owner. -m REASON (optional) is stored on the reopen
event: mw show lists it in events and -j carries it as the event note, so a reviewer can tell rework from a review loop.`,
		"log": `mw log [-t SLUG | -A] [--action ACTION] [--grep TEXT] [-n N] [-j]
mw log [-t SLUG | -A] --since CURSOR|start | --since-file PATH [-n N] [-j]
Activity feed, newest first (default 20): time, tenant, item, action, session, note. Actions: created,
ready, start, block, done, reopen, accept, reject, note, ask, answer, decide, edit, step, unstep. -A spans all tenants.
--action ACTION keeps one action; --grep TEXT keeps the events whose note contains TEXT (case-insensitive) and also reads the archive (mw archive) and, with -A, archived tenants:
mw log -A --action decide --grep csv finds a decision after it left the board. Neither works with --follow or --since.
--since turns it into a resumable feed, oldest first: it prints the events the cursor has not delivered and a next cursor
(-j: {events, next}; text: a last "next: CURSOR" line). The cursor is opaque and bounded (the newest time plus 8-byte hashes of the events
of the 24h before it, so an event that reaches the board late with an older stamp is still delivered once); --since "" or start returns everything.
--since-file PATH reads the cursor from PATH (missing = start) and writes the next one back atomically, instead of --since.
An event stamped after the cursor time that arrives later (a git merge) is returned once; one stamped before it is not (read from start). -n keeps the first N.
mw log [-t SLUG | -A] --follow [--since CURSOR|start] [--poll SECONDS] [--exec] [-j]
--follow is a live feed: new events as they arrive, oldest first, from now unless --since names a cursor (default poll 2s; -j prints one JSON object per line; Ctrl-C stops it; not with -n or --since-file).
--exec also runs your alerts.exec (mw user set alerts.exec='["notify-send","mw"]') for each event whose action is in alerts.on (mw user set alerts.on=block,ask; empty means no event). The command is an argv array, never run through a shell; the event is a JSON file (ts, tenant, item, action, by, note, key) in a private temp file whose path is added as the last argument; stdin is closed; 30 s limit; a failure is reported on stderr and the feed goes on. MW_ALERT_EXEC (the same JSON array) overrides alerts.exec.`,
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
mw board --md [--job ID] [-o PATH|-]   a readable Markdown jobs view (Needs you, To review, In progress, Ready,
Recently done, Waves with progress) for the Claude desktop app's file view; default <news.out>/jobs.md per job (docs/news/jobs.md
with no job), rewritten only when the content changed (hash in jobs.md.hash), prints "wrote PATH" or "unchanged PATH"
Show a small progress bar, a "last: <ago> <item> <action>  today: N done" line and active work.
-A shows every tenant, most recently active first; -v includes history. A gates line appears when any gate is on.`,
		"context": `mw context [-t SLUG] [-s NAME] [-j]
Show a compact startup brief for one coding-assistant session.
mw context --hook-json
For Claude Code's SessionStart hook (startup, resume, clear, compact): reads the hook JSON on stdin and prints
{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":...}} with the session's claimed
(doing) items: id, title, acceptance, last notes, every answer/decide since the claim and the gates. The session is
MW_SESSION, else the one started with mw session start NAME --claude-session ID. Prints nothing (exit 0) with no
board, no matching session or no claimed item, never fails, and cuts the text at 2000 characters.`,
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
  mw status --line [--link]             one line for the Claude Code status bar (mw:N2R1B1:12/40); mw status --explain
  mw boards [DIR] [--depth N] [--up] [-j]
  mw migrate [--dry-run] [PATH]
  mw version [-j]
  mw update [--check] [--plugins]
  mw init [workspace name] | mw init --kit coordinated --job ID
  mw tenant add SLUG [NAME...] [-d DIR]
  mw tenant list [-j] [-v] [--tag T] | default [SLUG|-] | archive|restore SLUG | tag SLUG +T -T
  mw dirs [init] [-j] | set datalake|docs|data|tools|templates|builds|evidence|scratch PATH
  mw note ID TEXT... [-t SLUG] [-s NAME]
  mw session start NAME [-t SLUG] [-a ROLE]
  mw session list [-t SLUG] [-j] [-v] | stop NAME [--release] | reap [-t SLUG | -A] [--older-than DUR] [--except NAMES] [-n] [-j]
  mw get-prompt session NAME [-t SLUG] [-a ROLE] [--headless] [-j]
  mw get-prompt dispatcher|coach|newsdesk [NAME] [--job ID] [-j]
  mw add KIND TITLE... [-t SLUG] [-p ID] [-w WAVE] [-b TEXT] [-c TEXT] [--ticket KEY]
  mw edit ID [-c TEXT] [-b TEXT] [-p PARENT|-] [ticket=KEY]
  mw user show [-j] | get [KEY] [-j] | set KEY=VALUE...
  mw configure [--show [-j]] | KEY=VALUE...
  mw jobs enable ID -t SLUG... | disable ID [-t SLUG] | set ID KEY=VALUE... | show [ID] [-j]
  mw news [--job ID] [--full] [--format md|html|text|json] [--release [--force]] [-o PATH] [-j]
  mw ask ID QUESTION... [-s NAME] | mw answer ID TEXT... --by NAME | mw decide TEXT... [--by NAME] [-b WHY] [-p ID]
  mw gate [-t SLUG] [-j] | gate level trust|proof|spec|judged | gate evidence|acceptance|judge on|off|auto [--by NAME]
  mw review [-t SLUG] [-j]
  mw judge ID [-t SLUG]
  mw accept ID [--by NAME] | mw reject ID REASON... [--evidence-gap] [--by NAME]
  mw list [-t SLUG | -A [-s SESSION]] [-k KIND] [-S STATUS] [-n N] [-j] [-v]
  mw show ID [-t SLUG] [-j] [-v] [--full]
  mw ready ID [-t SLUG] [-s NAME]
  mw reopen ID [-t SLUG] [-s NAME] [-m REASON]
  mw start ID [-t SLUG] [-s NAME] [-a ROLE]
  mw block ID REASON... [--on ID] [-t SLUG] [-s NAME]
  mw done ID VERIFICATION... [--evidence PATH]... [--no-evidence REASON] [-t SLUG] [-s NAME]
  mw log [-t SLUG | -A] [-n N] [-j] | mw log [-t SLUG | -A] --since CURSOR|start [-j]
  mw wave create|add|remove|step|unstep|show|list|close ...  (mw help wave)
  mw next [-t SLUG] [-s NAME] [-k KIND] [-j] [-v] | mw next --claim [-s NAME] [-a ROLE] [-j] [--prompt]
  mw board [-t SLUG | -A] [-v] [-j] | mw board --html [-o PATH] | mw board --md [--job ID] [-o PATH|-]
  mw fanfare [on|off|stop|test|use NAME|set KEY=VALUE...]   sounds when a wave closes or a tenant is finished
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
		if key == "json" || key == "explain" || key == "all" || key == "verbose" || key == "na" || key == "evidence-gap" || key == "workspace" || key == "dry-run" || key == "release" || key == "full" || key == "force" || key == "claim" || key == "headless" || key == "prompt" || key == "html" || key == "md" || key == "archived" || key == "follow" || key == "exec" || key == "accept" {
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

// newsClean makes board text safe for a terminal: whitespace controls become spaces, every other control,
// format (bidi, zero-width) and invalid rune is dropped. keepNL keeps newlines (summary text).
func newsClean(s string, keepNL bool) string {
	s = strings.ToValidUTF8(s, "")
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			if keepNL {
				return r
			}
			return ' '
		case r == '\t' || r == '\r':
			return ' '
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == 0xFFFD:
			return -1
		}
		return r
	}, s)
}

func compactText(s string, limit int) string {
	s = strings.Join(strings.Fields(newsClean(s, false)), " ")
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
