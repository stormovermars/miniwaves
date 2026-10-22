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
mw get-prompt dispatcher [NAME]
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
mw init [workspace name]
mw tenant add SLUG [NAME...] [-d DIR]   # on an existing slug with -d: updates the path
mw tenant list [-j] [-v] | default [SLUG|-]
mw migrate [--dry-run] [PATH]   # upgrade a schema 5/6 workspace file to 7 (MIGRATION.md)
mw dirs [init] [-j] | set NAME PATH   # NAME: datalake docs data tools templates builds evidence scratch (all eight shown by mw dirs)
mw note ID TEXT... [-t SLUG] [-s NAME]
mw session start NAME [-t SLUG] [-a ROLE]   # optional: `start` registers an unknown -s name; with none it uses the tenant's implicit "local" session (id `local@SLUG`, may own many items; the name `local` is reserved)
mw session list [-t SLUG] [-j] [-v] | stop NAME [--release]
mw session reap [-t SLUG | -A] [--older-than DUR] [--except NAME,NAME] [-n|--dry-run] [-j]
mw get-prompt session NAME [-t SLUG] [-a ROLE] [--headless] [-j]   # exit 3 if NAME exists
mw get-prompt dispatcher|coach [NAME] [-j]
mw add KIND TITLE... [-t SLUG] [-p ID] [-w WAVE] [-b TEXT] [-c TEXT]
mw edit ID [-c TEXT] [-b TEXT] [-p PARENT|-]   # -c under a session only while acceptance is empty
mw gate [-t SLUG] [-j] | mw gate level trust|proof|spec|judged [-t SLUG] [--workspace] [--by NAME] | mw gate evidence|acceptance|judge on|off|auto [--by NAME] [--workspace]
mw version [-j]   # {version, schema, board_schema, board}: nearest workspace and its schema (null if none)
mw review [-t SLUG] [-j]
mw judge ID [-t SLUG]
mw accept ID [--by NAME] | mw reject ID REASON... [--evidence-gap] [--by NAME]
mw list [-t SLUG] [-k KIND] [-S STATUS] [-n N] [-j] [-v]
mw list -A [-s SESSION] [-k KIND] [-S STATUS] [-n N] [-j] [-v]   # every tenant; -s = items that session owns (no waves)
mw show ID [-t SLUG] [-j] [-v]   # full brief: body, acceptance + source, evidence dir, children, notes, last 5 events; falls back to the archive; -j ignores -v
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
mw board [-t SLUG | -A] [-v] [-j]   # last: <ago> <item> <action>, today: N done; -A sorted by last activity
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
- `mw review [-t SLUG] [-j]`: `judge` (items awaiting judgement, `×N` = rejections; `[]` while the gate is off), `blocked`, `acceptance` (only with that gate on) and `waves` ready to close. `-j` = `{judge, blocked, acceptance, waves}`.
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

## Fanfare

An optional sound for milestones, off by default. `mw fanfare` shows the config and the collection; `mw fanfare set enabled=true milestone_seconds=20 finale_seconds=240 volume=1 stop_dialog=true dir=~/fanfare-finale-songs` changes it (stored as `"fanfare"` in `.miniwaves.json`); `mw fanfare test [milestone|finale]` plays one clip now; `mw fanfare list` lists the pair folders; `mw fanfare fetch` downloads the collection; `mw fanfare stop` kills the players mw started (and closes the Stop dialog), from any terminal.

- `mw wave close` plays the first `milestone_seconds` (default 15) of a random pair's `fanfare` track. A tenant becoming finished plays the first `finale_seconds` (default 60) of the pair's `finale` track. Finished means: after a `done` or `wave close` the tenant has at least one item, none in inbox/ready/doing/blocked (an open wave counts as an item), and `fanfare_finale_at` is not yet set. It fires once per transition: the stamp is stored on the tenant and cleared when a non-done item appears. A close that also finishes the tenant plays the finale only.
- Sets: `mw fanfare sets add NAME PAIR...` (pair folders of the collection; name `[a-z0-9-]`, max 32, not `all`) groups pairs, `sets drop NAME [PAIR...]` removes pairs or the set, `mw fanfare sets [-j]` lists them (pairs marked playable, unplayable or missing), and `mw fanfare use NAME|all` picks the active set (`"sets"` and `"use"` in the `"fanfare"` object). Wave close, finale and `test` pick only from the active set; a set with no playable pair falls back to all with a `mw: fanfare:` note on stderr. `mw fanfare -j` keeps its long-standing keys (config, dir, ready, unplayable, incomplete, empty, manifest) and adds enabled, use, sets, pairs (name, playable), milestone_seconds, finale_seconds and volume for a UI.
- The collection is `~/fanfare-finale-songs/<pair>/` with `fanfare.<ext>`, `finale.<ext>` and `LICENSE.txt` (source, license, attribution, sha256); `manifest.json` lists the pairs (`source: commons` or `user`).
- Playback is a detached child process, so the command returns at once, and a failure only prints a `mw: fanfare:` note on stderr. macOS: `afplay -t SECONDS [-v VOL]` for mp3/m4a/aac/wav/aiff, else `ffplay`/`mpv` (afplay cannot play Ogg or Opus). Linux: `ffplay -nodisp -autoexit -t SECONDS`, else `mpv --no-video --length=SECONDS`, else `paplay` (wav/ogg/flac only). Windows: PowerShell `Media.SoundPlayer` for `.wav` only (no volume); other formats are unsupported. **`paplay` and Windows cannot trim: they play the whole track** on every wave close. **Ctrl-C does not stop a finale** (the player runs in its own session): while a finale plays a **Stop dialog** (macOS `osascript`, Windows popup, Linux `zenity` if installed) lets you end it with one click, and it closes itself when the clip ends; `mw fanfare set stop_dialog=false` turns it off (the 15 s milestone never shows one). Or run `mw fanfare stop` from any terminal, which kills the players mw started (their pids are kept in `<dir>/.players.json`; stale entries are ignored).
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

## Git

Git is the coding agent's job, not mw's. Commit `.miniwaves.json` and `.miniwaves/` (and the published page) together with your code changes and resolve board conflicts in git like any file; a board with merge markers is refused with "board has merge markers: resolve it in git". `mw init` adds `.miniwaves/*.lock*`, `.miniwaves/*.tmp` and `.miniwaves.json.tmp` to `.git/info/exclude` so commits never include them.

## Windows

mw runs natively on Windows 10/11 (amd64, arm64); the same board works from PowerShell, cmd and Git Bash, and a board folder can be shared with macOS/Linux checkouts.

- Install: `install.ps1` puts `mw.exe` in `%USERPROFILE%\.local\bin` and adds it to the user PATH (set `MINIWAVES_NO_PATH=1` to skip). The Claude Code plugin hook runs in Git Bash and installs the Windows release there.
- Home is `%USERPROFILE%`; `~/` and `~\` both expand in folders, `--evidence` and `publish.html`.
- Writes and reads: lockless readers open the board, tenant and index files with `FILE_SHARE_READ|WRITE|DELETE`, so they never block a writer's rename, and retry a sharing violation or access-denied open (50 x 20 ms); a writer retries its rename the same way before failing.
- Lock wait: a writer waits up to `MW_LOCK_WAIT` seconds for the board lock (default 15 on Windows, 5 elsewhere), then fails with `board locked (pid P): lock wait timed out ...`; raise it for many concurrent runners on a slow machine. Directory fsync is skipped (Windows cannot open a directory); file fsync stays. Locks are `LockFileEx` and are released when the process exits.
- File modes (0600 board, 0644 page) are not enforced; Windows ACLs of the folder apply.
- Prompts print PowerShell setup (`$env:MW_SESSION=...`, `Set-Location`).

## Changelog

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
- `mw update`: 120 s timeout for the downloads (5 s for the API call); asset URLs are pinned to github.com and objects.githubusercontent.com (redirects too); on Windows a failed swap restores the old exe. Renames retry only on Windows (50 x 20 ms).
- Schema 8, per-tenant storage: `.miniwaves.json` is the index, each tenant lives in `.miniwaves/<slug>.json` (+ `.archive.json`); `mw migrate` converts older boards in place (backup `.miniwaves.json.bak7`).
- Kernel locks for writers (per tenant, 5 s deadline) and lockless reads; 40 runners on one board finish every item exactly once.
- `mw board --html` and `publish` write a static page, rewritten only when the board changed and at most every `publish.interval` seconds (default 60, minimum 5).
- Embedded prompts are at most 20 lines each.
- Waves, gates (evidence, acceptance, judge), `mw log`, `mw show` full brief, `mw note`, `mw edit`.
- Claude Code plugin (`claude plugin marketplace add stormovermars/miniwaves && claude plugin install mw@miniwaves`), install scripts for macOS, Linux and Windows, CI with `go vet` and `go test -race` (plain `go test` on Windows).
- `mw tenant archive|restore SLUG`: an archived tenant keeps its files but is hidden from `board -A`, `list -A`, `log -A`, `next`, `tenant list` (shown with `-v`) and the page (shown greyed with `--archived`); `version -j` marks it `archived` so runners skip it; `-t SLUG` on it fails with "SLUG is archived: mw tenant restore SLUG".
- `mw tenant tag SLUG +TAG -TAG...` (lowercase, unique, sorted; stored in the index); `--tag T` (repeatable, AND) filters `tenant list`, `board -A`, `list -A`, `log -A`; tags show in `board`, `tenant list`, `version -j` and as chips on the page.
- `mw update [--check]` replaces the binary with the latest GitHub release (checksum verified, exit 2 when current); `mw version` and `mw help` print the releases URL, `version -j` adds `repo` and `releases_url`.
- Windows: rename retry for writes over an open board file, `~\` paths, `install.ps1` adds mw to the user PATH, the plugin hook installs the Windows asset from Git Bash, CI on windows-latest is a gate (see [Windows](#windows)).
- Removed since 0.1.0: `mw tenant use` (folder default, tenant path, `MW_TENANT` and the first tenant select the tenant; `tenant default` stays).
