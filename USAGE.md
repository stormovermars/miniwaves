# miniwaves usage

## Orientation

Run `mw description` anywhere for a compact TOON guide to the purpose, model,
human workflow, and coding-assistant lifecycle. Use `mw description -j` when a
host tool needs JSON.

## Multi-tenant workflow

```sh
cd ~/apps
mw init "Apps"
mw tenant add search "Search app" -d ~/apps/search
mw tenant add player "Player app" -d ~/apps/player
cd ~/apps/shared && mw tenant default search

mw get-prompt session search-builder -t search -a builder | pbcopy
# Paste into a new coding-assistant session. It registers itself when started.

mw get-prompt dispatcher fleet-dispatcher
mw get-prompt coach human-coach

mw add prompt "Add offline search" -t search -b "The original request"
mw add story "Search works without a connection" -t search -p PRM-001 \
  -c "cached results appear in under 200 ms"
mw add spike "Choose a local index" -t search -p STR-001

mw ready SPK-001 -t search
mw next -s search-builder
mw start SPK-001 -s search-builder
mw done SPK-001 "FTS5 benchmark passed" -s search-builder
mw board -A
```

Starting the first child of an inbox/ready story moves the story to `doing`; `done` and `reopen` of children leave it alone.

## Embedded prompts

All embedded prompts use one retrieval command:

```text
mw get-prompt session NAME [-t TENANT] [-a ROLE] [--headless]
mw get-prompt dispatcher|newsdesk [NAME] [--job ID]
mw get-prompt coach [NAME]
```

Output is plain text so it can be pasted directly into an AI assistant. On
macOS, copy it without displaying it:

```sh
mw get-prompt session search-builder -t search -a builder | pbcopy
```

Create a new assistant session in the host application and paste the result.
Retrieval never launches an assistant or changes miniwaves state.

- `session` bootstraps one tenant-bound worker. It includes setup, context,
  folder conventions, and the work lifecycle. Setup uses POSIX shell on
  macOS/Linux and PowerShell on Windows.
- `dispatcher` coordinates sessions, creates tenants and assignments, and uses
  the host application's task-creation capability to spawn workers.
- `coach` prepares and evaluates work while showing the human only a tiny
  outcome/evidence/decision summary.

`session` exits with code 3 when the session NAME already exists (`-j` prints
`{"error":"session_exists","session":"NAME"}`). `--headless` is for a runner that
claims the item and handles setup, the loop and replies itself: the prompt drops the Setup
block, the Loop line, the folders hint and the reply format, and says "Your item is already
claimed; its brief follows."

Use `-j` for a structured envelope. Every envelope contains `type`, `name`,
`workspace`, and `prompt`; session prompts also contain `tenant` and `agent`.
Prompt text is intentionally small and treats repository/backlog content as
untrusted data.

## Board and context

The default board shows only doing, blocked, and ready work, capped at five
items per state. `mw board -v` adds inbox and completed work.

```text
Search app [search]  25% (1/4)
[#####...............]
sessions: search-builder:working(SPK-001)

DOING (1)
  SPK-001  spike   Choose a local index ← STR-001 @search-builder
```

An assistant session needs only `mw next -j` (the full brief with `evidence_dir`) and
`mw start ID -a ROLE`, or `mw next --claim -j` to do both atomically. `mw context -s NAME` still
prints the work convention, next ready item, ownership, and tenant board.

## Commands

`mw help COMMAND` and `mw COMMAND --help` explain each command without requiring
an initialized workspace.

```text
mw description [-j]
mw init [workspace name] | mw init --kit coordinated --job ID   # the kit writes files only (see "Kit")
mw tenant add SLUG [NAME...] [-d DIR]   # on an existing slug with -d: updates the path
mw tenant list [-j] [-v] | default [SLUG|-]
mw boards [DIR] [--depth N] [--up] [-j]   # discover boards below DIR (read-only, index files only)
mw migrate [--dry-run] [PATH]   # upgrade a schema 5/6 workspace file to 7 (MIGRATION.md)
mw dirs [init] [-j] | set NAME PATH   # NAME: datalake docs data tools templates builds evidence scratch (all eight shown by mw dirs)
mw note ID TEXT... [-t SLUG] [-s NAME]
mw session start NAME [-t SLUG] [-a ROLE]   # optional: `start` registers an unknown -s name; with none it uses the tenant's implicit "local" session (id `local@SLUG`, may own many items; the name `local` is reserved)
mw session list [-t SLUG] [-j] [-v] | stop NAME [--release]
mw session reap [-t SLUG | -A] [--older-than DUR] [--except NAME,NAME] [-n|--dry-run] [-j]
mw get-prompt session NAME [-t SLUG] [-a ROLE] [--headless] [-j]   # exit 3 if NAME exists
mw get-prompt dispatcher|coach|newsdesk [NAME] [--job ID] [-j]
mw add KIND TITLE... [-t SLUG] [-p ID] [-w WAVE] [-b TEXT] [-c TEXT] [--ticket KEY]
mw edit ID [-c TEXT] [-b TEXT] [-p PARENT|-] [ticket=KEY]   # -c under a session only while acceptance is empty
mw gate [-t SLUG] [-j] | mw gate level trust|proof|spec|judged [-t SLUG] [--workspace] [--by NAME] | mw gate evidence|acceptance|judge on|off|auto [--by NAME] [--workspace]
mw version [-j]   # {version, schema, board_schema, board}: nearest workspace and its schema (null if none)
mw ask ID QUESTION... [-t SLUG] [-s NAME]   # a question for a human, as an event (see "Needs you and decisions")
mw answer ID TEXT... --by NAME [-t SLUG]   # human only; closes the item's open ask
mw decide TEXT... [--by NAME] [-s SESSION] [-b WHY] [-p ID] [-t SLUG]   # a decision event on ID, else on the tenant's "Decisions" chore
mw user show [-j] | get [KEY] [-j] | set KEY=VALUE...   # per-user config (see "Per-user config")
mw configure [--show [-j]] | KEY=VALUE...   # walk the per-user settings, then Save or Discard (see "Per-user config")
mw jobs enable ID -t SLUG... | disable ID [-t SLUG] | set ID KEY=VALUE... | show [ID] [-j]   # coordinated-job config (see "Coordinated jobs")
mw news [--job ID] [--full] [--format md|html|text|json] [--release [--force]] [-o PATH] [-j]   # news bulletin from board data (see "News")
mw review [-t SLUG] [-j]
mw judge ID [-t SLUG]
mw accept ID [--by NAME] | mw reject ID REASON... [--evidence-gap] [--by NAME]
mw list [-t SLUG] [-k KIND] [-S STATUS] [-n N] [-j] [-v]
mw list -A [-s SESSION] [-k KIND] [-S STATUS] [-n N] [-j] [-v]   # every tenant; -s = items that session owns (no waves)
mw show ID [-t SLUG] [-j] [-v] [--full]   # full brief: body, acceptance + source, evidence dir, children, notes (always whole), last 5 events (notes cut at 90 chars); --full = every event, notes whole (also with -j); falls back to the archive; -j ignores -v
mw reopen ID [-t SLUG] [-m REASON]   # back to inbox; optional -m REASON is kept on the reopen event (mw show events, -j, mw log)
mw ready|reopen ID [-t SLUG] [-s NAME]
mw start ID [-t SLUG] [-s NAME] [-a ROLE]   # inbox/ready only (or your own item); -a ROLE registers a missing session with that role
mw block ID REASON... [-t SLUG] [-s NAME]
mw done ID VERIFICATION... [--evidence PATH]... [--no-evidence REASON] [-t SLUG] [-s NAME]
mw wave create TITLE... [-t SLUG] [-p WAVE] [-b TEXT]
mw wave add|remove WAVE ID... [-t SLUG]
mw wave step WAVE KEY=NOTE [KEY=NOTE...] [--na] [-t SLUG]   # several steps per call, all-or-nothing
mw wave step WAVE --na-rest REASON [-t SLUG]   # marks every unticked step n/a with the reason, prints the count
mw wave unstep WAVE KEY [-t SLUG]
mw wave show WAVE [-t SLUG] [-j]
mw wave list [-t SLUG | -A] [-j]
mw wave close WAVE [--no-evidence REASON] [-t SLUG]
mw next [-t SLUG] [-s NAME] [-k KIND] [-j] [-v]   # the same full brief as show; -j: null (exit 0) when nothing, "skipped: N (...)" on stderr
mw next --claim [-s NAME] [-a ROLE] [-t SLUG] [-k KIND] [-j]   # pick + start in one locked step; -j adds "claimed": true
mw archive [--days N] [-t SLUG] [--dry-run]   # done items older than N days (30) -> .miniwaves/<slug>.archive.json
mw log [-t SLUG | -A] [-n N] [-j]   # activity feed, newest first
mw log [-t SLUG | -A] --since CURSOR|start [-n N] [-j]   # resumable feed, oldest first, plus a next cursor
mw log [-t SLUG | -A] --follow [--since CURSOR|start] [--poll SEC] [--exec] [-j]   # live feed; --exec runs alerts.exec per alerts.on event
mw board [-t SLUG | -A] [-v] [-j]   # last: <ago> <item> <action>, today: N done; -A sorted by last activity
mw board --md [--job ID] [-o PATH|-]   # Markdown jobs view (see "Jobs view")
mw context [-t SLUG] [-s NAME] [-j]
```

Short flags: `-t` tenant, `-s` session, `-a` agent, `-p` parent,
`-d` path, `-w` wave, `-b` body, `-c` acceptance (`--acceptance`, alias `--accept`), `-k` kind, `-S` status,
`-n` name, `-j` JSON, `-v` verbose, and `-A` all tenants. Long forms remain
supported. Use `--` before a title or note beginning with `-`.

Kinds are `prompt`, `story`, `task`, `spike`, `bug`, and `chore`. States are
`inbox`, `ready`, `doing`, `blocked`, and `done`. Blocking requires a reason,
completion requires verification, and a parent cannot close with open children.

Session liveness: a session's last activity is set by `session start` and by every
start/ready/block/done/reopen, `note`, `edit` and `wave add/remove/step/unstep` it runs
with `-s`. During long work with no mw command, run `mw note ID "still on it" -s NAME`
at least hourly so `reap` leaves the session alone. A reaped session's later `mw done` on
an item that is still `ready` succeeds and revives the session; it is refused if
another session claimed the item meanwhile.

## Waves (milestones)

A wave is a milestone bundle: members carry the wave in their own `wave` field
(`mw wave add`, or `mw add KIND TITLE --wave WAV-001`); `parent` is untouched, so
a member keeps its story and prompt. Waves may nest (`mw wave create -p`). Delivery goes by milestone: each one is
audited and fixed, adversarially reviewed and fixed, wrapped up, committed,
merged, tested, version-bumped, built/installed, cleaned up, and only then is
the plan revised and the revised plan audited. Every wave is created `ready`
with this checklist by default, in this order (key in parentheses):

1. audit (`audit`): result audit, issues solved
2. adversarial review (`adversarial`): issues solved
3. wrap-up (`wrapup`): evidence on the items
4. commit (`commit`)
5. merge (`merge`)
6. test (`test`)
7. version bump (`bump`)
8. build + install, if it applies (`install`)
9. clean up (`cleanup`)
10. plan revised (`plan`)
11. plan audited (`plan-audit`): plan issues solved

The default checklist is `PROTOCOL.md` (embedded in the binary). Precedence for a new wave: the tenant's own list, else the workspace list, else the default.
`mw tenant protocol SLUG` shows the steps and their source (`tenant`, `workspace` or `default`); `mw tenant protocol SLUG set "Step one" "Step two" ...` stores a tenant list
(key = lower-cased name with dashes; duplicates dropped); `mw tenant protocol SLUG reset` drops it. `--workspace` (no SLUG) does the same for the workspace list (`"protocol"` in `.miniwaves.json`).
A change applies to waves created afterwards; existing waves keep their steps.

Tick steps with evidence, several per call: `mw wave step WAV-001 test="go test: 19 pass" bump=0.5.0`.
Use `--na` when a step does not apply; the note is then the reason and the step
counts as done; `mw wave unstep WAV-001 test` unticks. `mw wave close` refuses (exit 1, listing what is missing) while
any member is not done or any step is unticked; `mw done` also refuses on
waves, as do `start`, `block`, `ready` and `unclaim`; only `mw reopen WAV-001`
works on a closed wave and it resets every step. A closed wave accepts no new
members (`wave add`, `add --wave`) or steps. `mw next` never hands out a wave;
`mw board` shows waves with their rollup, and `mw list` prints one document with
a `waves` key when waves are listed; `mw list -j` always prints `{items, waves}` (waves may be empty).
`mw edit ID -p PARENT|-` changes or clears the parent of any item; the kind
hierarchy (prompt > story > task/spike/bug/chore) and cycles are enforced, and
a wave is never the parent of a non-wave (`-p WAV-001` or `edit -p` to it errors: use `--wave`). A closed wave refuses new members, removals, closed parent waves, and `ready`/`start`/`reopen` of its members; `wave close` also needs every child wave closed. An item belongs to one wave: `wave remove` it first to move it.

## Gates (proof before closing)

Rigour is set as a cumulative **level** per tenant (or `--workspace` for the default):

| level | evidence | acceptance | judge |
|---|---|---|---|
| `trust` | off | off | off |
| `proof` (default) | on | off | off |
| `spec` | on | on | off |
| `judged` | on | on | on |

`mw gate level trust|proof|spec|judged [-t SLUG|--workspace] [--by NAME]` sets the three gates (humans only, like `mw gate NAME on|off`). New tenants and boards start at `proof` unless the workspace default says otherwise (boards migrated from schema 5 keep default `off`, so new tenants there start at `trust`); `mw gate level trust` goes back to no gates. `mw gate level L --workspace` lists tenants whose own settings override it. Gate changes record `--by` and the date (shown as "set by NAME DATE" in `mw gate`). Under the hood there are three gates, `evidence`, `acceptance`, `judge`, each `auto|on|off`; `mw gate NAME on|off|auto` still works and can make a combination that is no level (`level custom`). `mw gate [-t SLUG] [-j]` prints the level first (`level proof (evidence on, acceptance off, judge off)`), then the setting, effective state and reason per gate; `-j` prints `{level, gates}`. Gates are humans only: refused under `MW_SESSION` or `-s`; `--by NAME` is required when stdin is not a terminal. `--workspace` sets the default for every tenant without its own setting. A short gates line (`gates: proof`, or the per-gate text for a custom mix) appears on `board`, `context` and `next` when any gate is on; a change since the last saving command prints one stderr line.
- **evidence**: `mw done ID CHECK` needs `<evidence>/<tenant>/<ID>/` (folder name exactly the ID) with at least one non-empty file (dotfiles, `Thumbs.db`, `desktop.ini` do not count); or `--no-evidence REASON`, kept as a note. `wave close` checks `<evidence>/<tenant>/WAV-NNN/` the same way. `show -v` lists the files, `list -v` shows `E`. `auto` = on for every tenant (level `proof`); `mw gate` names the reason: product tenant (build manifest or `releases/`), latch, else `default proof`.
- **acceptance**: `auto` = off (no latch). With it `on`, `mw next` skips only prompts and stories (`-k story`) whose own or nearest ancestor acceptance is empty (`skipped N without acceptance`); standalone task/spike/bug/chore and children of a story are handed out regardless. `mw edit ID -c TEXT` sets acceptance (humans only).
- **judge**: `auto` = off (`mw gate` says "auto = off; set on to require human accept"); no product-tenant latch, no accept-streak hint. Only a human setting `on` turns it on. `done` records `judged: ungated` while it is off; on, `mw add -S done` is refused.
- Boards migrated from schema 5 start with every gate `off` (finished items are `judged: ungated`); new boards start at level `proof`. `ready`/`start`/`reopen` clear evidence and judgement; `done` records them again.
- Threat model: mw guards against lazy or confused agents, not hostile ones. An agent can edit the JSON or `edit -p` around a gate; the guard is friction plus an audit trail (`by`, `judged_by`, timestamps, notes).
- `mw show ID` prints the item's notes (timestamp, `[session]` author, text) without `-v`; `mw note ID TEXT` records `-s`/`MW_SESSION` as author.
- `mw version -j` prints `{version, schema}`; `mw next -j` prints `null` (exit 0) when nothing is ready and carries `level`, `gates` and `skipped`; `board -j` and `context -j` carry `level` and `gates`.

## Review and judgement

The loop with the judge gate on (set by a human; `auto` is off): a session claims, works, writes proof into the evidence folder, runs `mw done` and idles; the human runs `mw review`, then `mw judge ID` and `mw accept ID` or `mw reject ID REASON`.
- `mw review [-t SLUG] [-j]`: `judge` (items awaiting judgement, `×N` = rejections; `[]` while the gate is off), `blocked`, `acceptance` (only with that gate on) and `waves` whose members are all done (review-ready; each row carries `closeable`, true only when `mw wave close` would pass: steps ticked, members judged, evidence present). The delivery steps follow the review, so a wave is listed before they are ticked. `-j` = `{judge, blocked, acceptance, waves}`.
- Judgeable = a story or a work item with no story ancestor. A story appears once all its children are done, whatever its own status.
- `mw judge ID` prints Outcome, then Evidence (one folder per line; a story adds each child's) and Decision (one `[ ]` line per acceptance line).
- `mw accept ID` records `judged` and `judged_by`; a story with done children becomes done and judged together.
- `mw reject ID REASON [--evidence-gap]`: story back to `ready` plus a ready task `Fix: REASON` (inherits the acceptance); a child goes `ready` and reopens its done story. Refused in a closed wave. `--evidence-gap` latches the evidence gate.
- `accept`/`reject` are humans only (refused under `MW_SESSION` or `-s`; `--by NAME` without a terminal). With the judge gate on, `wave close` lists unjudged members and the board header counts done and judged.

## Releases

Releases are files, not commands: `releases/<version>.md` with front matter (`version`, `date`, `build`, `tested`, `items`, optional `wave`, optional `evidence` = the release item's own `<evidence>/<tenant>/<ID>/` folder) and free-text notes. See `releases/README.md`; validate with `scripts/check-release.sh <file>`.

## Tenant and folder selection

`MW_SESSION`, `MW_TENANT`, `-s`, and `-t` select explicit context. Otherwise,
miniwaves uses the nearest folder default, the nearest configured tenant path,
then the first tenant that is not archived.

`mw tenant archive SLUG` hides a finished or parked tenant (its files stay); `mw tenant restore SLUG` brings it back.
`mw tenant tag SLUG +android +store -old` adds and removes tags; `--tag T` (repeatable, all must match) filters
`tenant list`, `board -A`, `list -A` and `log -A`.

Set a folder default with `mw tenant default SLUG`. Run `mw tenant default` to
inspect it or `mw tenant default -` to clear it.

The shared folder convention is:

- `~/datalake` — consumed inputs;
- `~/docs` — examples and specifications;
- `~/data` — stored data and generated documents;
- `~/tools` — shared tooling; `~/templates` — scaffolds (copy, never edit in place);
- `~/builds/<app>/<ver>` — artefacts; `~/evidence/<app>/<item>` — proof (screenshots, logs, crash pulls, repro fixtures); `~/scratch/<app>/<session>` — throwaway.

`mw dirs` prints all eight expanded paths. `mw dirs init` creates missing folders.

The locations are configurable per board with an optional top-level `folders`
key in `.miniwaves.json`, e.g. `{"datalake": "D:\\lake", "docs": "~/specs", "data": "out"}` (also `tools`, `templates`, `builds`, `evidence`, `scratch`) (a drive-letter path is absolute on Windows only; elsewhere it resolves under the home directory). Control characters are rejected.
Relative paths and a `~/` prefix resolve against the home directory; absolute
paths (including Windows drive paths) are kept. `mw dirs set
NAME PATH` writes the key. Prompts and `mw context` print the configured paths as a three-line sentence
(session prompts render `<builds>/<tenant>`, `<evidence>/<tenant>`,
`<scratch>/<tenant>/<session>`; dispatcher and coach render the roots):

```
Folders: code in the tenant repo (plans + ADRs + test fixtures inside); shared tooling ~/tools, scaffolds ~/templates (copy, never edit in place).
Artefacts ~/builds/<app>/<ver>; proof (screenshots, logs, crash pulls, repro fixtures) ~/evidence/<app>/<item>; throwaway ~/scratch/<app>.
Inputs ~/datalake, generated outputs ~/data, shared reference/specs/plans ~/docs/<topic>. Never write elsewhere; never commit builds, evidence or scratch.
```

Without the key the defaults above apply.

`mw note ID TEXT...` appends a timestamped note to any item or wave (any
status); `mw show ID` lists the notes. It never changes the status or the
block/done note.

## Needs you and decisions

`ask`, `answer` and `decide` append item events (`ask`, `answer`, `decide`; see PROTOCOL.md). They work on any board: there is no gate and no job is needed, and they never change an item's status.

- `mw ask ID QUESTION... [-s NAME]` records a question for a human. Any session may ask.
- `mw answer ID TEXT... --by NAME` is human only, like `accept`: it is refused under `MW_SESSION` or `-s`, and `--by` is required when stdin is not a terminal. It is refused when the item has no open ask.
- An **open ask** is the last `ask` of an item with no later `answer` on that item (one answer closes every earlier ask).
- `mw review` shows open asks as `needs you`, `mw review -j` and `mw board -j` gain `needs` (`[{tenant, item, question, by, at}]`, always an array), and the text `mw board` prints a `NEEDS YOU` block.
- `mw decide TEXT... [--by NAME] [-b WHY] [-p ID]` is open to humans and sessions. With `-s` (or `MW_SESSION`) the event is recorded by that session and a `--by NAME` becomes `(for NAME)` at the end of the note; without a session it is recorded by `--by NAME` (required when stdin is not a terminal), else the login name. It appends a `decide` event to `-p ID`, else to the tenant's `Decisions` chore, created on first use (already `done`, so it never holds the tenant open). `-b WHY` is stored as `(why: WHY)` at the end of the note.

## Discovering boards

`mw boards [DIR] [--depth N] [--up] [-j]` lists the boards under DIR (default `.`) so pane plugins need no setup.

- `--depth N` is 1..6; the default is your `discover.depth` (4, see "Per-user config"). `--up` also adds the nearest ancestor board.
- It reads only each `.miniwaves.json` (no tenant files, no locks, no writes). An index over 1 MB is listed as an error entry, not read.
- It skips dot folders, `node_modules`, `build`, `dist`, `vendor`, symlinks and, on Windows, `AppData`.
- On macOS, `--up` never stats ancestors inside the protected folders (`~/Documents`, `~/Desktop`, `~/Downloads`, `~/Library`, `~/Movies`, `~/Music`, `~/Pictures`) unless DIR itself starts inside one, so it raises no permission prompt.
- It stops at 200 boards or 5 s and says so (`truncated`, `reason`). An unreadable index is listed with `error`.
- `-j` prints `{"root","boards":[{"dir","path","name","schema","tenants":[{"slug","name","archived"}],"error"?}],"truncated","reason"}`.

## Resumable log (`log --since`)

`mw log [-t SLUG | -A] --since CURSOR|start [-n N] [-j]` is an event feed a poller can resume. It lists, oldest first, the events the cursor has not delivered, then a `next` cursor (`-j`: `{"events": [...], "next": "..."}`, each event with a `key`; text: a last `next: CURSOR` line). `--since ""` and `--since start` return everything; `-n` keeps the first N and `next` resumes after them.

`mw log --follow` polls the board (every 2 s, `--poll SEC`) and prints new events as they arrive, oldest first, starting from now (the first read is retried a few times if the board is mid-write; `--since CURSOR|start` catches up first; `-j` is one JSON object per line). With `--exec` it also runs your `alerts.exec` for each event whose action is listed in `alerts.on` (an empty `alerts.on` runs nothing). The command is an argv array, never a shell; the event is written as JSON (`ts`, `tenant`, `item`, `action`, `by`, `note`, `key`) to a private temp file whose path is appended as the last argument, stdin is closed, it has 30 s (and 5 s more to release its pipes, so a child that keeps them open cannot hang the feed), and a failure goes to stderr without stopping the feed. On Windows a `.bat` or `.cmd` `alerts.exec` runs through `cmd.exe`, so its arguments get cmd parsing (the temp file path is the only argument mw adds); use an `.exe` or `pwsh -File` for exact argv. `MW_ALERT_EXEC` (the same JSON array) overrides `alerts.exec`; the tests use it as a stub. Board text only ever reaches the command inside that file. With `-A` the tenant list is fixed when the feed starts: a tenant added later is not followed. Each followed tenant has its own cursor, so one that is archived and restored while the feed runs does not replay its history.

- The cursor is opaque and bounded (base64 of the newest delivered time and 8-byte hashes of the delivered events stamped within the 24 hours before it). An event's key is `tenant/item/H`, H being 12 hex digits of a hash of the event's stamp, action, actor and note, so a `git merge` that reorders events never moves a key.
- Each read returns the events stamped after the cursor time, plus those stamped within the 24h window up to it whose key hash is not in the cursor. An event from another clone, or from a tenant that stamped before another tenant's save, is returned exactly once when it reaches the board, even with an older stamp. One older than the window is not returned: read from `start` to see it. The window is fixed at 24 hours.
- `--since-file PATH` reads the cursor from PATH (a missing file means `start`) and writes the next cursor back atomically, so the cursor need not fit on a command line. Do not combine it with `--since`.
- `mw archive` removes whole items from the feed; their keys fall out of the cursor and nothing is replayed.

## Per-user config

Personal choices live outside the committed board, in `mw-user.json` under `os.UserConfigDir()/miniwaves/` (`MW_USER_CONFIG` overrides the path). `mw user` works without a board.

```text
mw user show [-j]            # the values and the file's path
mw user get [KEY] [-j]       # every settings key (or one) with its effective value and source (default, user or board)
mw user set KEY=VALUE...     # validates every key and value, then writes the file (mode 0600); an empty VALUE unsets a key
mw configure                 # walk the settings in a terminal; Enter keeps, a number picks, d resets, t test-plays a fanfare; nothing is written before Save
mw configure --show [-j]     # the same as user get
mw user fanfare-defaults     # fanfare.enabled=true and fanfare.use=all, only where unset (install and update run it)
mw configure KEY=VALUE...    # validate and save without prompting
```

`mw configure` without a terminal on stdin prints the settings plus a hint and exits 0; it never reads stdin then. It uses plain stdin and stdout, so it works on Windows too. `user get -j` prints `{"keys":[{"key","value","default","source","group","choices"}]}`: `value` is the effective value, `source` is `default`, `user` or `board` (only `fanfare.enabled`, when the board runs `mw fanfare set enabled=false`), `group` is one of pane, alerts, fanfare, status (the chat bar), discover, and `choices` lists the allowed values (empty for free text). These keys are per user and never stored on a board; an invalid value is refused with a non-zero exit.

mw-jobs alerts appear as pane rows plus sound only, never as OS notification banners. The fanfare Stop dialog and your own `alerts.exec` are unaffected.

| Key | Allowed | Default | Meaning |
|---|---|---|---|
| `pane.refresh` | `15s`, `30s`, `1m`, `5m`, `off` | `30s` | how often the live pane refreshes |
| `pane.style` | `normal`, `blocks`, `gopher`, `agile`, `kanban` | `normal` | look of the pane |
| `pane.zoom` | `-2`..`2` | `0` | pane text size step |
| `pane.fit` | `80`, `fill` | `80` | pane width |
| `pane.open` | `jobs`, `news` | `jobs` | the view the pane opens on |
| `alerts.enabled` | `true`, `false` | `true` | raise alerts for board events |
| `alerts.kinds` | comma list of `new`, `blocked`, `ask`, `review`, `done`, `release`, `metrics`; or `all`; or `none` | all seven | which events alert |
| `alerts.message` | `true`, `false` | `true` | show a message with an alert |
| `alerts.popup` | `true`, `false` | `true` | show a popup (toast) with an alert |
| `alerts.sound` | `true`, `false` | `true` | play a sound with an alert |
| `fanfare.enabled` | `true`, `false` | `false` (`mw user fanfare-defaults` sets `true` at install) | play fanfares; a board that runs `mw fanfare set enabled=false` still wins (see Fanfare) |
| `fanfare.use` | a set name or `all` | `all` | the fanfare set to play |
| `status.show` | `true`, `false` | `true` | show the mw line in the chat status bar |
| `status.detail` | `compact`, `full` | `compact` | how much the status line says |
| `discover.home` | `true`, `false` | `true` | also look for boards under your home folder |
| `discover.depth` | `1`..`6` | `4` | folder levels to search |

Two older keys are set the same way and are not part of `user get`:

| Key | Value |
|---|---|
| `alerts.on` | comma list of event names (lowercase letters, digits, `.` `_` `-`) |
| `alerts.exec` | a JSON argv array, for example `["notify-send","mw"]`; stored as data; run only by `mw log --follow --exec`, never through a shell |

Writes (`user set`, `configure`, `user fanfare-defaults`) take `mw-user.json.lock` and re-read the file inside it, so concurrent writers lose nothing. On Unix mw refuses to read the file when it is group- or world-writable (`chmod 600` fixes it), because `alerts.exec` names a command. On Windows there are no comparable mode bits and no check is made. A broken file never fails a board command: fanfare ignores it with one note on stderr. `mw log --follow --exec` runs `alerts.exec` (see `mw log --follow` below).

## Coordinated jobs

A job groups tenants under one coordinator. mw only stores the config and ships prompts and data; it never starts a session. The config lives in `.miniwaves.json` as an omitempty `jobs` object keyed by job id (a tenant carries `"job": ID`), in schema 9 (a board from 0.2.x needs `mw migrate`, which only bumps the version; a 0.2.8 or older mw then refuses the board instead of silently rewriting the index without `jobs`, `job` and fanfare `sets`).

JSON uses snake_case keys like every other `-j` output: the board and `jobs show -j` store `news_out`, `news_url` and `pane_style`, and `user show -j` has `alerts_on`, `alerts_exec`, `fanfare_enabled`, `fanfare_use`. `news_releases`, `metrics`, `tracker` and `tracker_url` are plain snake_case in both places; the snake_case names `news_out`, `news_url` and `pane_style` also work in `mw jobs set`. The dotted CLI keys stay (`mw jobs set ID news.out=...`, `mw user set fanfare.use=...`). `mw jobs enable` prints `moved SLUG from job OLD` when a tenant leaves another job.

```text
mw jobs enable ID -t SLUG...        # create the job if needed and put the tenants in it (-t repeats or takes a comma list)
mw jobs disable ID [-t SLUG]        # clear the tenants (all, or one); the job's data stays
mw jobs set ID KEY=VALUE...         # every key and value is validated; nothing is stored when one is refused; an empty VALUE resets the key
mw jobs show [ID] [-j]              # effective values (defaults applied) and the job's tenants
```

| Key | Default | Rule |
|---|---|---|
| `title` | the id | up to 120 characters |
| `description` | none | one line, up to 120 characters, no control characters; shown by `jobs show` and under the title in `mw news` |
| `max_workers` | `4` | 1..32 |
| `worktrees` | `on` | `on` or `off` |
| `lock` | none | a command name only (`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`), never a command line |
| `news.out` | `docs/news/` | relative, cleaned, inside the workspace: no `..`, no absolute path, no `~`, no drive |
| `news.url` | none | an `http(s)` URL; the link to the job's own page (mw never replaces that page) |
| `news_releases` | `<news.out>/releases` | the folder of numbered releases; a path with the `news.out` rules, never at or under `.miniwaves/`, `.miniwaves.json` or their locks and backups |
| `metrics` | none | a path to the job's `metrics.json` (the `news.out` rules; see "Metrics") |
| `tracker` | none | one of `jira linear youtrack github gitlab azure shortcut clickup asana`; off unless set (see "Tickets") |
| `tracker_url` | none | the tracker's base URL: https only, no query, fragment or credentials; a trailing `/` is trimmed; clickup and asana may leave it empty |
| `summary` | none | a path with the `news.out` rules |
| `pane` | `on` | `on` or `off` |
| `pane.style` | none | `^[a-z0-9-]{1,32}$` |
| `ceremonies` | none | a comma list of `review`, `planning`, `triage`, `spike`; an unknown name is refused; stored in that order without duplicates; only the listed ceremonies reach the dispatcher prompt (see "Ceremonies") |
| `ready_min` | `3` (shown once `ceremonies` is set) | 1..20; the planning trigger: fewer ready items than this |

Job ids match `^[a-z0-9][a-z0-9-]{0,31}$`. `jobs show` is a read; the other three write the index. Values are data: nothing here is executed.

## News

`mw news` builds one **model** per job (version 1: `job`, `release`, `now`, `happened`, `next`, `metrics`, `summary`) from board data only (no network, no publishing) and draws every format from it, so a number is computed once. The default view is a 70-column, plain-monospace **bulletin** in the style of a 1990s gopher hole: a masthead (`RELEASE N / date`, or `DRAFT - NOT RELEASED`), numbered sections `1. IN SHORT`, `2. WHAT HAPPENED`, `3. ONGOING`, `4. WHAT'S NEXT`, `5. METRICS`, item lines `T tenant/ID  title ..... state` with a type character (`0` item, `1` milestone, `!` needs you, `?` review, `-` blocked, `+` in progress, `>` next), ASCII metric bars with `▲`/`▼` changes, and a footer listing the releases. Lists show their top rows and then `... N more (see full board)`.

```text
mw news [--job ID] [--full] [--format md|html|text|json] [--tenant SLUG] [-o PATH|-] [-j]
mw news --job ID --release [--force] [--format ...] [-j]   # (shown as issue N in the mw-jobs pane)
```

- **Formats.** `md` is one fenced `text` block (a ticket with a link adds a short Markdown list after the block, because links do not render inside a code block); `html` is a green-on-black page with one `<pre>` (each footer release number links to that release); `text` is the bare bulletin; `json` is the model. The bulletin is kept under 9,500 characters by showing fewer rows per list. The bulletin is for jobs that have no page of their own: a job's own page (`news_url`) is never replaced by mw, and `news_url` keeps meaning "the link to that page". `--width` is accepted for `--format text` and ignored (the bulletin is always 70 columns); it is refused for the other formats.
- **Draft.** Without `--release`, the output is the draft since the latest release (`release.number` is `null`; `release.previous` names the release it follows, `release.changes` counts the events and changed metric rows). It is a function of the board and the window, so an unchanged board gives identical bytes. Default: every job, one file each at `<news.out>/status.md` (`.html` with `--format html`); with no job at all, the workspace to `docs/news/status.md`. Two jobs that resolve to the same file are refused (set `news.out` per job). `text` and `json` print to stdout and write a file only with `-o PATH` (no `news.out`, no sidecar); `-o -` prints md or html to stdout. `-j` prints `[{job, path, written}]` and is an error without a file target. `--format json` needs `--job` when several jobs exist.
- **Model limits.** Every list has a cap and a `more` count: the `now` lists hold 50, `milestones.open` 200, each `happened` list 100, `next.ready` 10, `next_milestones` 5. No item text is longer than 200 characters and the whole model stays under 1 MB (`truncated: true` marks rows dropped to fit). Item shape: `{tenant, id, title, note, since, by}` plus `ticket`, `ticket_label`, `ticket_url` when the item has a ticket. Asks are gathered from every tenant of the job in one run, so a pane needs one mw call per job per poll. `now.tenants` lists `{slug, name, counts{needs, review, blocked, doing, ready, done, total}, last_change}` per tenant, sorted by attention (needs, review, blocked, newest change, slug), at most 2000 rows plus `more` (a row is about 150 bytes; the 1 MB guard trims tenant rows after the item lists). For the agile and kanban pane styles the model also has `now.backlog` (`{items[], count}`: the ready queue in order, then drafts by creation, items carrying `state` ready or draft, cap 50 plus `more`), `counts.draft`, `now.recent_done` (the last 10 done items, `since` = done time, with `milestone` and `milestone_title`), `now.sprint` (`null`, or the open wave nearest to done: `{kind: "wave", tenant, id, title, done, total, items[] with state todo|doing|review|blocked|done and needs, more}`, items capped at 50), and in `happened` the flow lists `moved_to_doing` (start events) and `moved_to_review` (a done item that awaits the judge, or a block that starts `awaiting review`), capped like the others; done items carry their `milestone`. Flow events never make a release non-empty. `--tenant SLUG` (json only) gives the same model for one tenant; the release window stays job-wide, and `--tenant` is refused with `--release`. Measured on temp boards: 100 tenants x 30 items about 0.1 s and 137 KB; 1000 tenants x 10 items about 0.45 s (mostly one lock and one read per tenant file) and 470 KB. A blocked item whose latest block reason starts with `awaiting review` is listed under `review`, not `blocked` (`mw review -j` has the same `review` list).
- **Full.** `--full` (draft only) drops the per-list row caps of the model and the bulletin in every format: every row is listed, every `more` is 0, and `caps` reports 0 (uncapped). The 200-character item text and the 1 MB model guard stay. When the guard still has to drop rows, the bulletin prints exactly one line `trimmed: N rows`, and the json model carries `full: true` and `trimmed_rows: N` (both absent without `--full`). `--full --release` is refused (a frozen release keeps the caps). `--full` is allowed only with `-o FILE`, `-o -` (stdout), `--format json` or `--format text`; it never rewrites the desk's `status.md` draft.
- **Releases.** (Shown as issue N in the mw-jobs pane: the "Publish" button, "Issue N · date · vs issue N-1".) `--release` cuts release N+1 under the board lock (the job lock) with atomic writes: `RELEASE_DIR/N.json` (the frozen model), `N.md`, `N.html`, then `latest` (the number). `RELEASE_DIR` is the job key `news_releases`, default `<news.out dir>/releases` (`jobs show` prints the effective path). The next number is above every `N.*` file and `latest`, so a half-written or pruned archive never reuses a number; a failed run removes its own files. A corrupt `N.json` makes `--release` refuse (the error names the file), while a draft warns on stderr and compares with the newest readable release. Release N compares with release N-1 (the window is `(N-1.at, N.at]`, metric changes are against N-1's frozen metrics); release 1 compares with the start. An empty release (nothing in the `happened` lists and no changed metric row; edits, notes, steps and ready/start events do not count) is refused with exit 0 and `nothing new since release N` unless `--force`. Releases live only in the releases folder: `news.out` always holds the draft, and a job's own published page (`news_url`) is its desk's business. Nothing cuts a release automatically: the news desk tick only refreshes the draft. `-j` prints `{job, released, number, path, message}`; `--format json` also prints the model.
- **Metrics.** The job key `metrics` names a `metrics.json` that the job's own tooling writes (temp file plus rename). It is read as data: confined to the workspace, a regular file, at most 256 KB, at most 8 sections of at most 40 rows, `title`, `name`, `unit` and legend names at most 60 characters and `note` at most 120 (control characters are stripped), finite numbers, counts integers >= 0, `done` <= `total`; unknown fields are ignored, row names are unique within a section, values and targets are at most 1e12 in size, and data after the JSON document is refused. Kinds: `split` (`good warn bad none`, drawn `[#####=====xxxx....]`), `progress` (`done total`, or `not started`), `value` (`value`, `unit`, `target`, `better`). An invalid file gives `{error}` in the model and `metrics file invalid: REASON` in the bulletin. A higher `version` gives a `newer metrics format` notice (what can be read is still shown). No key or no file means no section. Deltas (`delta{field: n}`, drawn `▲n`/`▼n`) are against the previous release's frozen metrics. Full format: `docs/specs/metrics.md`.
- **Tickets (optional, off by default).** A job may name one issue tracker (`mw jobs set ID tracker=jira tracker_url=https://acme.atlassian.net`) and an item may carry one ticket from it (`mw add task TITLE --ticket MOB-142`, `mw edit ID ticket=MOB-142`, `ticket=` clears it). mw validates the ticket against the tracker kind, stores it in canonical form, and builds the link from a fixed template; it never calls the tracker. A ticket is refused when the job has no tracker. mw ids (`TSK-011`) are never taken as tickets and the mw id is always in the model.

  | `tracker` | Accepted | Shown as | Link |
  |---|---|---|---|
  | `jira` | `MOB-142` | MOB-142 | `{base}/browse/{key}` |
  | `linear` | `ENG-42` | ENG-42 | `{base}/issue/{key}` |
  | `youtrack` | `APP-7` | APP-7 | `{base}/issue/{key}` |
  | `github` | `123`, `#123` | #123 | `{base}/issues/{n}` |
  | `gitlab` | `123`, `#123` | #123 | `{base}/-/issues/{n}` |
  | `azure` | `123`, `AB#123` | AB#123 | `{base}/_workitems/edit/{n}` |
  | `shortcut` | `123`, `sc-123` | sc-123 | `{base}/story/{n}` |
  | `clickup` | `86abc12xy`, `#86abc12xy` | #86abc12xy | `https://app.clickup.com/t/{id}` |
  | `asana` | `1203456789012345` | Asana 1203456789012345 | `https://app.asana.com/0/0/{gid}` |

  Key kinds are upper-cased before checking (`^[A-Z][A-Z0-9_]{0,9}-[0-9]{1,9}$`). A ticket under an unknown or newer `tracker` value is kept and shown as plain text, without a link. In the bulletin a ticket shows as `[MOB-142]` after the title (a link in html, a link in the list after the md block, plain text in `text`). `list -j` and `show -j` carry `ticket`. With no tracker and no tickets every output is identical to a board without the feature.
- **Safety.** `news.out`, `news_releases`, `metrics` and `summary` are checked each run: relative, no `..`, no absolute path, no `~`, and inside the workspace after symlinks are resolved. Only `-o` may point elsewhere. `summary` is read as data: a regular file, at most 20 KB, shown under 1. IN SHORT (a FIFO or device is ignored). Board text is stripped of control characters and bidi marks in every format; md puts the bulletin in a code fence longer than any backtick run in it, and html escapes everything, so a title such as `<script>` stays inert.
- **Locking and writes.** A file is rewritten only when the content changed (hash in `PATH.hash`); otherwise it prints `unchanged PATH`. `news` takes the board lock like any write, except when it only prints (`-o -`, or `--format text|json` without `-o`, and never with `--release`).

### Roles (prompts)

```text
mw get-prompt dispatcher [NAME] [--job ID] [-j]
mw get-prompt newsdesk [NAME] [--job ID] [-j]
```

- **dispatcher** routes work and reviews it. For sessions it spawns, the review loop is: the worker signals `mw note ID "ready for review"` and stays `doing`; the dispatcher reviews the diff, merges, re-runs the tests, then runs `mw done`. One task per session; a brief ends with "ping when ready, need input, or idle"; a question only a human can answer goes through `mw ask`. wr builders are unchanged: they keep `mw done` / `mw block`.
- **newsdesk** runs `mw news`, tells the human what changed in a few lines, keeps the human docs current, and alerts only on two events: a milestone closed or a new needs-you item. It never edits code, the board or the plan.
- `--job ID` fills the variables from `jobs.<ID>`: `max_workers`, `worktrees`, `lock` (the dispatcher), `title`, `news.out`, `news.url` (the newsdesk). Without `--job` the defaults apply (4 workers, worktrees on, no lock). An unknown job is an error. Prompts are embedded in mw and never read from repo files.

### Ceremonies

A ceremony is an ordinary **chore item** titled by its trigger (`Review WAV-003`, `Planning 2026-10-22`), done by a session with a role (`-a review`, `-a planner`, `-a spike`) and judged by the usual review, judge and accept/reject flow. mw adds no commands and no timers: it ships only embedded prompt text and two job keys. Retry (STALE, `session stop --release`), gates, burndown and the pane apply unchanged. Design: `docs/specs/ceremonies.md`.

```text
mw jobs set ID ceremonies=review,planning,triage,spike ready_min=3
mw get-prompt dispatcher --job ID      # now carries the Ceremonies section for the listed names only
mw get-prompt session NAME -t SLUG -a review|planner|spike
```

| Ceremony | Trigger (found by the dispatcher) | Session role | Output |
|---|---|---|---|
| `review` | `mw review -j` lists a wave (all members done; steps may still be open) | `review` | demo script, the evidence that exists per done member (screenshots only for UI items; members without evidence are listed, not blocked on), done items outside the wave, and lessons in `<evidence>/<tenant>/WAV-NNN/`; the ask goes on the review chore, which stays blocked until answered; at most one `mw ask` listing proposed `mw decide`, gate or protocol commands; a release wave reaching `merge` gets one `ask` before merge, push or tag |
| `planning` | fewer than `ready_min` ready items | `planner` | splits, order and duplicates as notes (a duplicate: the cut is proposed in the digest, a human cuts); may note stale `doing` stories (all children done); acceptance proposals through one `mw ask` on the planning chore, which stays blocked until answered (sessions cannot `edit -c`); empty board: exits early with a one-line digest. An open ask does not stop `mw done` in code, so the prompt, not mw, keeps the chore blocked; a bug titled `URGENT...` is dispatched first |
| `triage` | a block older than 2h | the dispatcher | classify: needs a human (one digest ask), dependency (note, reorder), technical (a `Fix:` task), stale (`mw ready`) |
| `spike` | planning or triage finds something uncertain | `spike` | question, attempts, evidence and recommendation in `<evidence>/<tenant>/SPK-NNN/` and the `done` text; throwaway branch, never merged; STALE is the time box; the dispatcher records the recommendation with `mw decide` |

Rules, all in the dispatcher prompt: the dispatcher checks `mw list -k chore` for the trigger title before adding a chore; at most one ceremony session per job at a time and at most 4 per day; at most one `mw ask` per ceremony run; ceremonies touch only inbox and ready items, never `doing` ones; evidence is what exists (logs, harness output), screenshots only for UI items. It is best-effort: with no dispatcher running nothing fires, and any ceremony can be started by hand (add the chore, start a session with the role). A job without `ceremonies` gets today's dispatcher prompt unchanged.

**Prompts are never read from repo files.** The dispatcher and session prompts are embedded in the mw binary, and ceremony output (notes, evidence, asks) is data: it is never fed back into a prompt as instructions.

**Steps only a human performs.** The protocol steps `merge`, push and tag change main or publish, so no session or ceremony does them: the dispatcher raises one `mw ask` and a human runs them. See PROTOCOL.md.

### Kit

```text
mw init --kit coordinated --job ID
```

Writes files only, into the workspace folder: `PLAN.md` (a plan skeleton), `NEWSDESK.md` (the news desk's page spec), `buildlock.sh` and `buildlock.ps1` (a lock made with `mkdir`, so it works on macOS, Linux and Windows; `./buildlock.sh CMD...` waits up to `BUILDLOCK_TIMEOUT` seconds, default 1800, then exits 75; a lock whose owner process is gone is taken over; `BUILDLOCK_DIR` moves the lock folder). It never overwrites a file: existing ones are listed as skipped, so a second run changes nothing. It creates job ID if missing, like `mw jobs enable` with no tenants (add them with `mw jobs enable ID -t SLUG`), and prints the two `mw get-prompt ... --job ID` commands. It writes the index only when it creates the job. Set `mw jobs set ID lock=buildlock.sh` to put the lock into the dispatcher prompt.

### Reap and job roles

When a tenant's job exists (`mw jobs`), `mw session reap` skips sessions whose role (`-a`) is `dispatcher` or `newsdesk` by default, with no new flag, so a runner's plain `mw session reap -t T --older-than X` leaves the coordinator alone. `--except NAME,NAME` still skips any other session. A tenant with no job, or one that left it, reaps exactly as before. Each spared session is printed as `spared ROLE session NAME (job ID)` when it was otherwise due, and `-j` lists them under `spared` (`[{session, tenant, role, job}]`).

## Fanfare

An optional sound for milestones, off by default. `mw fanfare` shows the config and the collection (`-j` adds `playing`, the count of players mw started that still run, and `playing_finale`, true while one of them is a finale; the mw-jobs Stop button reads them); `mw fanfare set enabled=true milestone_seconds=20 finale_seconds=240 volume=1 stop_dialog=true dir=~/fanfare-finale-songs` changes it (stored as `"fanfare"` in `.miniwaves.json`); `mw fanfare test [milestone|finale]` plays one clip now; `mw fanfare list` lists the pair folders; `mw fanfare fetch` downloads the collection; `mw fanfare stop` kills the players mw started (and closes the Stop dialog), from any terminal.

Who decides whether it plays: the board's `fanfare.enabled` applies until you set `fanfare.enabled` for yourself (`mw user set`, `mw configure`; install and update set `true` where it is unset). Your value then overrides the board's, with one exception: a board that explicitly runs `mw fanfare set enabled=false` wins over the user layer (`mw user get` then shows `fanfare.enabled=false`, source `board`). Boards written before 0.3.4 stored `"enabled": false` for "never set", which looks the same as that explicit off, so mw cannot tell them apart and leaves them alone; when such an off silences a user who turned fanfare on, `mw fanfare` (and `-j`, key `hint`) prints `board says off; mw fanfare set enabled=true to follow your setting`. `mw fanfare use NAME` is not a board decision about on/off. `fanfare.use` is the user's set choice. With no fanfare songs on disk a built-in fanfare plays: a short WAV generated by `scripts/gen_fanfare_default.py` (shipped in the repo) and cached in the user cache dir. `MW_NO_FANFARE=1` or `MW_NO_PLUGINS=1` stops install and update from setting the defaults.

- `mw wave close` plays the first `milestone_seconds` (default 15) of a random pair's `fanfare` track. A tenant becoming finished plays the first `finale_seconds` (default 60) of the pair's `finale` track. Finished means: after a `done` or `wave close` the tenant has at least one item, none in inbox/ready/doing/blocked (an open wave counts as an item), and `fanfare_finale_at` is not yet set. It fires once per transition: the stamp is stored on the tenant and cleared when a non-done item appears. A close that also finishes the tenant plays the finale only.
- Sets: `mw fanfare sets add NAME PAIR...` (pair folders of the collection; name `[a-z0-9-]`, max 32, not `all`) groups pairs, `sets drop NAME [PAIR...]` removes pairs or the set, `mw fanfare sets [-j]` lists them (pairs marked playable, unplayable or missing), and `mw fanfare use NAME|all` picks the active set (`"sets"` and `"use"` in the `"fanfare"` object). Wave close, finale and `test` pick only from the active set; a set with no playable pair falls back to all with a `mw: fanfare:` note on stderr. `mw fanfare -j` keeps its long-standing keys (config, dir, ready, unplayable, incomplete, empty, manifest) and adds enabled, use, sets, pairs (name, playable), milestone_seconds, finale_seconds and volume for a UI.
- The collection is `~/fanfare-finale-songs/<pair>/` with `fanfare.<ext>`, `finale.<ext>` and `LICENSE.txt` (source, license, attribution, sha256); `manifest.json` lists the pairs (`source: commons` or `user`).
- Playback is a detached child process, so the command returns at once, and a failure only prints a `mw: fanfare:` note on stderr. macOS: `afplay -t SECONDS [-v VOL]` for mp3/m4a/aac/wav/aiff, else `ffplay`/`mpv` (afplay cannot play Ogg or Opus). Linux: `ffplay -nodisp -autoexit -t SECONDS`, else `mpv --no-video --length=SECONDS`, else `paplay` (wav/ogg/flac only). Windows: PowerShell `Media.SoundPlayer` for `.wav` only (no volume); other formats are unsupported. **`paplay` and Windows cannot trim: they play the whole track** on every wave close. **Ctrl-C does not stop a finale** (the player runs in its own session): while a finale plays a **Stop dialog** (macOS `osascript`, Windows popup, Linux `zenity`, else `kdialog`, if installed) lets you end it with one click; it names the reason (tenant finished and items done, or the test), the song and the seconds it plays, passed as data and never interpolated into a script, and it closes itself when the clip ends; the dialog shows a picture left of the text (`cover.png`, `cover.jpg` or `cover.icns` in the played pair's folder, else mw's built-in icon) on macOS and zenity 3.90+ (older zenity and kdialog show none; Windows shows the information icon); `mw fanfare set stop_dialog=false` turns it off (the 15 s milestone never shows one). Or run `mw fanfare stop` from any terminal, which kills the players mw started (their pids are kept in `<dir>/.players.json`; stale entries are ignored).
- Pairs with a track the player here cannot play are skipped when picking and shown as `unplayable here` in `mw fanfare` and `mw fanfare list`; `fetch` warns when no downloaded pair is playable. Only graduation-day has mp3 for both tracks, so a stock Mac without `ffplay`/`mpv` (`brew install ffmpeg`) hears just that pair, and Windows plays none of the built-in collection.
- `dir` set with `mw fanfare set` is stored as an absolute path (`~` expanded, relative paths resolved against the current folder; a value starting with `-` is refused); hand-edited values in `.miniwaves.json` are clamped (seconds 1..3600, volume up to 4).
- **Licences.** `mw fanfare fetch` downloads only recordings whose composition is public domain and whose performance is a US government band or a CC0/CC BY release by the performer (checked on the Wikimedia Commons file page; source page, full attribution and, for CC, the licence URL are written to each `LICENSE.txt`; the source list is generated by `scripts/gen_fanfare_sources.py`), labelled by the film that made the piece famous (2001: A Space Odyssey, Apocalypse Now, The Lone Ranger, Die Hard and others). Copyrighted film scores (Star Wars, Rocky, Indiana Jones, Top Gun, Chariots of Fire, The Good, the Bad and the Ugly) are never downloaded: those pair folders are empty "bring your own" slots with a `README.txt` saying which track to drop in as `fanfare.<ext>` / `finale.<ext>` from your own purchased copy; `mw fanfare list` shows them as `empty (bring your own)` and random selection skips them.
- Every built-in track carries a pinned sha256 that is verified on the streamed bytes before the file is renamed into place (a mismatch removes the `.part` file); a track without a pin is refused. Redirects are followed only to https on `upload.wikimedia.org`, a track is capped at 64 MB, and the transfer is bounded at 600 s (30 s for the response headers). `fetch` is idempotent (present files with the pinned sha256 are skipped), retries rate-limited requests and prints `ok` or `fail` per pair.

## Archive

`mw archive [--days N] [-t SLUG] [--dry-run]` moves done items older than N days (default 30) with
their events into `.miniwaves/<slug>.archive.json`, so every command loads less (5000 items:
the board file shrinks from 5.8 MB to 0.3 MB). Waves and parents that still
have open children stay. `mw show ID` finds archived items read-only; the board counts them in its
totals (`DONE (n, +N archived)` with `-v`, `archived` in `board -j`); IDs are never reused.
`mw list` and `mw log` no longer show archived items.

## Static page

`mw board --html [-o PATH]` writes one self-contained page (all tenants with counts and open items, last/today, the last 40 events; dark mode via `prefers-color-scheme`; no JS, no CDN). Set `"publish": {"html": "site/index.html", "auto": true}` in `.miniwaves.json` to regenerate it after writes. The page is rewritten only when the board changed (a hash of the rendered page is kept in `<page>.hash`) and the last write is at least `"interval"` seconds old (default 60, minimum 5; lower values clamp to 5); a change skipped inside the interval is marked `pending` in `<page>.hash`, and the next mw invocation after the interval (a read such as `mw list` too, while `publish.auto` is on) publishes it, so the last change is never lost; an explicit `mw board --html` always writes. The page is written through a unique temp file, then its hash. GitHub Pages: copy `docs/publish.yml` to `.github/workflows/`. Vercel: set the output directory to the folder that holds the page (no build command), or add `vercel.json` with `{"outputDirectory": "site"}`, and commit the page (`git add site/index.html`).

## Jobs view

`mw board --md [--job ID] [-o PATH|-]` renders every job as Markdown for the Claude desktop file pane (the mw-jobs plugin's `/mw-jobs`). Layout per job: a headline (`! 2 need you`, else `◐ N awaiting review`, `⏱ N blocked`, `● N in progress`), then Needs you, Review, In progress (`●`, or `⏱ stalled` after 45 minutes), Milestones (open waves with a text bar, closed ones folded into `✓ N closed`), Harness (only with a job `metrics` file, with ▲▼ deltas against the latest release), Last events (6) and a footer (`updated HH:MM` is the newest event, a link to `status.md` and, when a `jobs.artifact-url` file sits next to the page, the published page). Empty sections are left out. The first line under the title is a status line over every job: `! tiny 2 need you · C2 5/6 │ demo ▸ D1 1/2`. Default file: `<news.out>/jobs.md` per job (`docs/news/jobs.md` with no job); `-o PATH` writes one file with every job, `-o -` prints it. The file is rewritten only when its content changed (hash in `jobs.md.hash`); mw prints `wrote PATH` or `unchanged PATH`. Ages are in 5-minute steps (hours, then days) so an idle board does not rewrite the file. `mw news` stays the news view.

## Git

Git is the coding agent's job, not mw's. Commit `.miniwaves.json` and `.miniwaves/` (and the published page) together with your code changes and resolve board conflicts in git like any file; a board with merge markers is refused with "board has merge markers: resolve it in git". `mw init` adds `.miniwaves/*.lock*`, `.miniwaves/*.tmp` and `.miniwaves.json.tmp` to `.git/info/exclude` so commits never include them.

## Windows

mw runs natively on Windows 10/11 (amd64, arm64); the same board works from PowerShell, cmd and Git Bash, and a board folder can be shared with macOS/Linux checkouts.

- Install: `install.ps1` also installs/updates the Claude Code plugins when `claude` is present (`MW_NO_PLUGINS=1` skips). It puts `mw.exe` in `%USERPROFILE%\.local\bin` and adds it to the user PATH (set `MINIWAVES_NO_PATH=1` to skip). The Claude Code plugin hook runs in Git Bash and installs the Windows release there.
- Home is `%USERPROFILE%`; `~/` and `~\` both expand in folders, `--evidence` and `publish.html`.
- Writes and reads: lockless readers open the board, tenant and index files with `FILE_SHARE_READ|WRITE|DELETE`, so they never block a writer's rename, and retry a sharing violation or access-denied open (50 x 20 ms); a writer retries its rename the same way before failing.
- Lock wait: a writer waits up to `MW_LOCK_WAIT` seconds for the board lock (default 15 on Windows, 5 elsewhere), then fails with `board locked (pid P): lock wait timed out ...`; raise it for many concurrent runners on a slow machine. Directory fsync is skipped (Windows cannot open a directory); file fsync stays. Locks are `LockFileEx` and are released when the process exits.
- File modes (0600 board, 0644 page) are not enforced; Windows ACLs of the folder apply.
- Prompts print PowerShell setup (`$env:MW_SESSION=...`, `Set-Location`).

## Changelog

### 0.3.5

No schema change. Details: `releases/0.3.5.md`.

- Ceremonies: job keys `ceremonies` (`review`, `planning`, `triage`, `spike`) and `ready_min` (1..20, default 3); a Ceremonies section in the dispatcher prompt for the listed names only; `review`, `planner` and `spike` role blocks in the session prompt, each rendered only when the session's job lists that ceremony. No new commands. See "Ceremonies".
- `mw log --follow [--exec]`: `alerts.exec` runs per `alerts.on` event; 5 s to release pipes after the 30 s limit; the first read of a from-now feed is retried; one cursor per tenant, so an archived and restored tenant does not replay; with `-A` the tenant list is fixed at start. On Windows a `.bat` or `.cmd` `alerts.exec` runs through `cmd.exe` (cmd argument parsing).
- `mw jobs show -j` has `news_path`.
- Build lock (kit): atomic PowerShell lock, pid-less lock folders older than a minute are taken over, tested under pwsh in CI.
- Fanfare: a hint when a pre-0.3.4 board's implicit `enabled=false` silences your setting; the default fanfare WAV is byte-identical on all OSes.
- mw-jobs: safe news-file open (no `HOME`, argv only), `review` and `metrics` alerts, `mw_panes_open` answers during a cold board scan, no OS notification banners.

### 0.3.4

Upgrading from 0.3.2 or 0.3.3: after `mw update`, run `mw update --plugins` once (the old binary updates its old plugin list). It installs `mw` and `mw-jobs`, then removes `tidewatch` and `mw-watch`.

- New `mw boards [DIR] [--depth N] [--up] [-j]` finds boards (depth 1..6, default `discover.depth`; index files over 1 MB are errors; Windows skips AppData; macOS `--up` does not stat ancestors inside protected folders unless it starts in one).
- `mw news --full` lifts the row caps (`full: true`, `trimmed_rows` in the json); only with `-o FILE`, `-o -`, `--format json` or `--format text`; never rewrites `status.md`; `--full --release` is refused.
- New `mw configure` (Pane, Alerts, Fanfare, Chat bar, Discovery; Save or Discard; `--show [-j]`; `KEY=VALUE...` saves without prompting) and `mw user get [KEY] [-j]` (value, default, source `default|user|board`, group, choices). Writes to `mw-user.json` are locked.
- Fanfare: `mw user fanfare-defaults` (install, update) sets `fanfare.enabled=true` and `fanfare.use=all` only where unset (`MW_NO_FANFARE=1` or `MW_NO_PLUGINS=1` skips). A board that runs `mw fanfare set enabled=false` wins over the user layer; `mw fanfare use NAME` does not count as that. With no fanfare songs, a built-in fanfare plays.
- `alerts.kinds` accepts `new`, `none` and `all`.
- Plugins: `mw-jobs` is the only pane plugin; `mw-watch` (formerly `tidewatch`) is retired. `install.sh`, `install.ps1` and `mw update` install `mw` and `mw-jobs` first and only then uninstall `tidewatch` and `mw-watch`. A stub `tidewatch` plugin for one release tells users to run `mw update --plugins`.
- mw-jobs: alerts show in the pane and as a toast plus a ping, never as macOS notification banners; alert text is `<job> ▸ <What>: <ref> <title>`; the chat-bar status shows per-job segments with zero setup; news releases are called issues in the pane (`mw news --release` is unchanged).

### 0.3.3

- Installing or upgrading mw also installs or updates the Claude Code plugins `mw` and `tidewatch` when `claude` is on PATH: `install.sh`, `install.ps1` and `mw update` (after a real update) do it, best effort; a failed plugin step never fails the install or changes the exit status. `MW_NO_PLUGINS=1` skips it. `mw update --plugins` runs only that step.
- tidewatch: one `Fanfare: off|all|<set>` button in the top row cycles off, all, each fanfare set, then off on every press (like `Auto:`); `/tidewatch fanfare next` does the same. The footer Fanfare and Set buttons are gone; Test stays.

### 0.3.2

- The tidewatch pane plugin ships in the miniwaves marketplace (`claude plugin install tidewatch@miniwaves`, then `/tidewatch`); 0.3.0 documented it but the install failed.

### 0.3.1

- `mw migrate` reads boards from 0.2.x with archived tenants (0.3.0 failed with `cannot unmarshal bool into ... Tenant.tenants.archived`); archived tenants stay archived.

### 0.3.0

See `releases/0.3.0.md` and MIGRATION.md (schema 9: run `mw migrate`; `mw review -j` has a new `review` list; the news pages are now a gopher-style bulletin drawn from a json model).

### 0.2.9

- Fanfare sets: `mw fanfare sets add|drop`, `mw fanfare use NAME|all` picks only from that set (falls back to all when it has no playable pair); `mw fanfare -j` keeps its keys and adds `enabled`, `use`, `sets`, `pairs`; plain `mw fanfare` shows `use:`.

### 0.2.8

- Fanfare: the finale's Stop dialog now says why it plays (tenant finished with its done count, or `mw fanfare test`), which song (manifest title, else file name and pair folder) and for how many seconds. Linux falls back to `kdialog` when `zenity` is missing. The text is passed to the dialog as data, never inside a script.

### 0.2.7

- Fanfare: the default finale is 60 s (was 300), and the finale shows a Stop dialog (macOS, Windows; Linux with `zenity`) that kills the player; `mw fanfare set stop_dialog=false` disables it. `mw fanfare stop` also closes the dialog.

### 0.2.6

- `mw update` accepts the `release-assets.githubusercontent.com` redirect GitHub now uses for release downloads (0.2.5 and older refuse it; update those once via `install.sh` or the plugin update). Hosts still match exactly over https.

### 0.2.5

- Gate levels: `mw gate level trust|proof|spec|judged [-t SLUG|--workspace]` sets evidence/acceptance/judge cumulatively (trust none, proof evidence, spec + acceptance, judged + judge). The default is now `proof`: evidence is on for every tenant, not only product tenants; `mw gate level trust` restores the old non-product behaviour.
- `mw gate` prints `level NAME (...)` first (`level custom` for a mix that matches no level); `mw gate -j` now prints `{"level", "gates"}` instead of a bare array. The gates line on board/context/next is `gates: proof` when the level has a name.
- Upgrade: boards whose tenants left evidence on `auto` now require evidence on `done`; set `mw gate level trust` to keep the previous behaviour.

### 0.2.4

- Fanfare (see the Fanfare section): `mw fanfare [set|test|fetch|list|stop]` plays a clip on `wave close` and a finale when a tenant is finished; off by default; the public-domain/CC collection is fetched into `~/fanfare-finale-songs`, copyrighted film scores are bring-your-own slots only.
- `mw tenant protocol SLUG [set STEP... | reset]` (and `--workspace`): per-tenant wave steps, stored as `protocol` in `.miniwaves/<slug>.json`; precedence tenant > workspace > embedded default; show prints the source.

### 0.2.3

A killed session (controller stopped, context lost) no longer holds its items and its record forever; no schema change, no migration.

- `mw session stop NAME -t T --release` sets every `doing` item the session owns back to `ready` (owner cleared, note `released: session NAME stopped with --release`), then stops the session. Without `--release` the refusal stays.
- `mw session reap [-t T | -A] [--older-than DUR] [-n|--dry-run] [-j]` does the same for every session idle longer than DUR (default `2h`; `90m`, `2h`, `1d`; at most `365d`), note `reaped: no activity since <last> (> DUR)`; `--except NAME,NAME` (comma-separated) skips those sessions. The event actor is the reaper (`-s`/`MW_SESSION`, else `reap`). One line per session and item; `-j` prints `{"reaped":[{"session","tenant","items":[...]}]}` (`"dry_run":true` added with `-n`). Takes the tenant lock(s). The implicit `local` session is never reaped.
- Last activity per session (`updated`) is now set by every start/ready/block/done/reopen, `note` and `edit` the session runs, and by `session start`.
- `mw board` shows the age of each doing item (`doing 3h`); `mw review` lists doing items older than 2h under `STALE` (and `stale` in `-j`); `session list -v` gains `age`.
- Prompts: "Before you idle: `mw done`, `mw block`, or `mw session stop NAME -t T --release`."

### 0.2.2

- Windows: files are replaced with a POSIX-semantics rename (`SetFileInformationByHandle`), so a writer no longer fails with `Access is denied` while a lock-free reader holds the board open; falls back to `MoveFileEx` with retries where unsupported.
- Windows: board, tenant, index, pid and page-hash files are read with `FILE_SHARE_READ|WRITE|DELETE` and a read that hits a sharing violation or access denied is retried (50 x 20 ms), as is a `stat` of the board; fixes `The process cannot access the file because it is being used by another process` from `mw list` while writers rename.
- `MW_LOCK_WAIT` (seconds) sets how long a writer waits for its locks: default 5, 15 on Windows. The timeout message now says it was a lock wait timeout and names `MW_LOCK_WAIT` (it still starts with `board locked (pid P)`).
- Every non-zero exit prints a one-line `mw: <reason>` on stderr, also for `-j` errors (`session_exists`), `mw update` when already current, and panics (`mw: internal error: ...`, exit 70).

### 0.2.1 (schema 8: run `mw migrate`)

The first working release after 0.1.0 (the v0.2.0 tag never produced release assets). mw reads schema 8 only; boards from 0.1.0 are upgraded with `mw migrate` (see [MIGRATION.md](MIGRATION.md)). Git is the agent's job: mw never commits, sessions are told to commit the board with their code.

- `mw next --claim [-s NAME] [-a ROLE] [-j]` picks and starts atomically; `--prompt -j` adds the inline headless prompt for the role.
- `mw start` guard: only inbox/ready (or your own) items start; `mw reopen` for done ones.
- `mw archive` moves done items out of the hot board file (archived tenants are skipped).
- `mw tenant archive` refuses while a tenant has a `doing` item or a live session (names them); `--force` overrides. Archived tenants no longer show in `wave list -A` or the gate lines of `board --html`.
- `mw update` plugin step: each `claude` call has a 60 s timeout; output is one line per plugin. `mw update`: 120 s timeout for the downloads (5 s for the API call); asset URLs are pinned to github.com and objects.githubusercontent.com (redirects too); on Windows a failed swap restores the old exe. Renames retry only on Windows (50 x 20 ms).
- Schema 8, per-tenant storage: `.miniwaves.json` is the index, each tenant lives in `.miniwaves/<slug>.json` (+ `.archive.json`); `mw migrate` converts older boards in place (backup `.miniwaves.json.bak7`).
- Kernel locks for writers (per tenant, 5 s deadline) and lockless reads; 40 runners on one board finish every item exactly once.
- `mw board --html` and `publish` write a static page, rewritten only when the board changed and at most every `publish.interval` seconds (default 60, minimum 5).
- Embedded prompts are at most 20 lines each.
- Waves, gates (evidence, acceptance, judge), `mw log`, `mw show` full brief, `mw note`, `mw edit`.
- Claude Code plugin (`claude plugin marketplace add stormovermars/miniwaves && claude plugin install mw@miniwaves`), install scripts for macOS, Linux and Windows, CI with `go vet` and `go test -race` (plain `go test` on Windows).
- `mw tenant archive|restore SLUG`: an archived tenant keeps its files but is hidden from `board -A`, `list -A`, `log -A`, `next`, `tenant list` (shown with `-v`) and the page (shown greyed with `--archived`); `version -j` marks it `archived` so runners skip it; `-t SLUG` on it fails with "SLUG is archived: mw tenant restore SLUG".
- `mw tenant tag SLUG +TAG -TAG...` (lowercase, unique, sorted; stored in the index); `--tag T` (repeatable, AND) filters `tenant list`, `board -A`, `list -A`, `log -A`; tags show in `board`, `tenant list`, `version -j` and as chips on the page.
- `mw update [--check] [--plugins]` replaces the binary with the latest GitHub release (checksum verified, exit 2 when current); `mw version` and `mw help` print the releases URL, `version -j` adds `repo` and `releases_url`.
- Windows: rename retry for writes over an open board file, `~\` paths, `install.ps1` adds mw to the user PATH, the plugin hook installs the Windows asset from Git Bash, CI on windows-latest is a gate (see [Windows](#windows)).
- Removed since 0.1.0: `mw tenant use` (folder default, tenant path, `MW_TENANT` and the first tenant select the tenant; `tenant default` stays).
