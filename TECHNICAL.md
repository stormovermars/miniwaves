# miniwaves technical details

## Architecture

miniwaves is one Go CLI with no server, account, database, or web interface.
Its model is deliberately small:

- tenant — isolated app/project;
- prompt — original user request;
- story — desired outcome;
- task, spike, bug, chore — implementation work;
- session — tenant-bound coding assistant with at most one owned item.

The application does not launch assistants. Embedded dispatcher and session
prompts describe how a capable host application should create them.

## Storage and concurrency

Workspace state lives in `.miniwaves.json`. The current schema version is 7 (4 added waves, 5 added `folders` and `notes`, 6 added gates and evidence, 7 added the per-item `events` activity list: `{at, action, by, note}` appended through `emit` by add, ready, start, block, done, reopen, accept, reject, note, edit and wave steps; `load` refuses any other schema; `migrate.go` (`mw migrate`, recipe in MIGRATION.md) is the only code that reads schemas 5/6 and synthesises created/updated events; older binaries refuse newer boards).
Schema 8 layout (`store.go`): `.miniwaves.json` is the index (version, name, folders, workspace gates, publish, `tenants: [{slug,name,path}]`); `.miniwaves/<slug>.json` holds a tenant's items, counters, gate state and sessions (sessions live in the tenant file, so registering one never touches the index). Reads take no lock: every write is tmp + fsync + rename (+ directory fsync), so a reader always sees a complete file. A writer locks only what it writes, with a blocking kernel lock (no polling) and a deadline of `MW_LOCK_WAIT` seconds (default 5, 15 on Windows): `.miniwaves/<slug>.lock` per tenant (taken in sorted order) and `.miniwaves/index.lock` for index changes (`tenant`, `dirs`, workspace `gate`); the pid of the last acquirer is in `<lock>.pid`. `scopeOf` decides from the arguments which tenant files a command loads (one with `-t`/`MW_TENANT`/the folder, else all); `save` writes only the files whose bytes changed. Writes hold the lock for read-modify-write-fsync only; publish work happens after unlock. Writes compact JSON
to a temporary file, `fsync` it, and atomically rename it. Read-only commands (`list`, `show`, `log`,
`board`, `next` without `--claim`, `review`, `judge`, `context`, `get-prompt`, `tenant|session list`,
`wave show|list`, `dirs`) take a shared lock; every lock waits at most `MW_LOCK_WAIT` s, then fails with
"board locked (pid P): lock wait timed out after D — retry, or raise MW_LOCK_WAIT (seconds)". On Windows the lockless reads open with
`FILE_SHARE_READ|WRITE|DELETE` and retry sharing violations (`fsread*.go`). Cross-platform file locking prevents concurrent local sessions from
overwriting one another. `mw archive` moves old done items to `.miniwaves/<slug>.archive.json` (
`{version, tenants: [{slug, items}]}`) and counts them in the tenant's `archived` field; only `show`
reads the archive. The optional `publish` board key drives `html.go` (static page, after the command succeeded, never fails it). mw never runs git itself, except that `mw init` writes the lock/tmp/pid names to `.git/info/exclude` (`git.go`).

State and lock files use `0600` where the operating system supports POSIX
permissions. `mw dirs init` creates missing shared folders with `0700` and
leaves existing folders unchanged.

The session, dispatcher and coach prompts live in `prompts/*.md`, embedded with
`//go:embed` and rendered with `text/template` (`prompts.go`). The optional
board key `folders` (`datalake`, `docs`, `data`, `tools`, `templates`, `builds`, `evidence`, `scratch`; additive, schema stays 5) is resolved by
`workspaceFolders`: relative and `~/` paths go under the home directory,
absolute paths are kept; the result feeds the prompts, `mw context` and
`mw dirs`. Items carry an optional `notes` list (appended by `mw note`).

This is single-machine coordination, not a distributed lease system.

## Waves in the data model

A wave is an `Item` with `kind: "wave"` (ID prefix `WAV`, counter in the
tenant's `Next`). It alone carries `steps`, an array of
`{key, title, done, na, note, when}` seeded from `defaultProtocol` in `waves.go` (parsed from `PROTOCOL.md`), or
from the tenant's `protocol: [names]` (tenant file), else the index's top-level `protocol` (`protocolFor`/`protocolSteps`). The default list lives in `PROTOCOL.md`, embedded with `go:embed` and parsed at startup.
Waves start `ready`; moveCommand verbs refuse them except `reopen`, which
resets every step. Membership is a separate `wave` field on the member
(`omitempty`; board schema was bumped to 4 (waves) and 5 (folders, notes); older binaries refuse newer boards with "upgrade mw"; `mw migrate` repairs cfa03cf-style boards: WAV parent on a non-wave becomes `wave`, an unclosed `doing` wave becomes `ready`); `parent` is only the story/prompt hierarchy and
wave nesting. `checkParent` enforces kind hierarchy, rejects waves as parents of
non-waves, and rejects cycles (`wouldCycle`), for `mw add` and `mw edit -p`;
`checkMember` refuses closed waves.

## Gates and evidence (schema 6)

Items gain `evidence` (relative path + size, recorded at `done`), `judged` (RFC3339, `legacy` or `ungated`), `by` (session at `done`), `judged_by` and `attempts` (all omitempty). Tenants gain `gates` (`evidence|acceptance|judge`: `auto|on|off`), `gates_seen` (effective state last recorded), `gate_latch` (`{name: reason}`) and `accept_streak`; the workspace may carry a default `gates`. `mw migrate` (schema below 6) sets the workspace default to `off` and marks done non-prompt, non-wave items `judged: ungated`; `load` refuses every schema but 7.

Effective state (`gates.go`): setting `on`/`off` wins; `auto` for evidence is always on (reason: product manifest, latch, else `default proof`; the manifest and latch no longer decide anything); judge and acceptance `auto` are off (no latches, `gate_latch.judge/acceptance` in old boards are ignored). Levels (`gateLevels`, `levelOf`, `levelSettings`): `trust`/`proof`/`spec`/`judged` map to explicit on/off settings for evidence, acceptance, judge; `mw gate level L` writes all three tenant (or workspace) settings; `levelOf` names the effective combination or `custom`, shown by `mw gate`, `gate -j` (`{level, gates}`) and the short `gates: LEVEL` line. Product tenant = tenant dir holds `build.gradle*`, `*.xcodeproj`, `project.yml`, `Package.swift`, `go.mod`, `package.json`, `Cargo.toml` or `releases/` (reason text only; it decides nothing). With acceptance `on`, `pickNext` skips only prompts and stories lacking effective acceptance. `save` refreshes latches and `gates_seen` only for the tenants the command touched (file-system checks happen only for tenants shown); non-saving commands print a stderr flip line when the effective state differs from `gates_seen`. `done` with the evidence gate on calls `checkEvidence`: `ReadDir` of `<evidence>/<slug>` and an exact, case-sensitive ID match, then at least one regular non-empty file that is not a dotfile, `Thumbs.db` or `desktop.ini`; paths are built with `filepath.Join` only; a symlinked evidence folder or file is resolved with `os.Stat` (symlinked subfolders are not descended). Acceptance is inherited from the nearest non-empty ancestor (`effectiveAcceptance`).

## Judgement loop (`judge.go`)

Judgeable = a story or a task/spike/bug/chore with no story ancestor (`judgeable`); a child's judgement belongs to its story (`judgementOwner`), prompts and waves count as judged. `ungated` counts as judged, so turning the judge gate on after ungated `done`s queues nothing. A story is awaiting judgement when it has no `judged`, every child is done and (childless) it is done itself; other items when `done` and unjudged.
Transitions: `done` records `evidence`, `by` and `judged: ungated` unless the judge gate is on; `accept` sets `judged` (RFC3339), `judged_by`, `accept_streak++`, and for a story also `status: done`; `reject` does `attempts++`, a note, clears `judged`/`judged_by`/`evidence`, `accept_streak = 0`, story to `ready` plus a ready `Fix: REASON` task child (a done story ancestor of a rejected child goes to `ready` and gets `attempts++` too, so `×N` shows in review); `ready`/`start`/`reopen` clear judgement. `reject` is refused while the item's (or its story's) wave is closed. `wave close` lists unjudged members with the judge gate on; board/burndown count a done item only when judged (`countsDone`). `burndownOf` (`burndown.go`) is the one burndown rule behind the board header (`mw burndown` is gone): `done` = all done items, `judged` = those counting via `countsDone`, `percent = judged/total`. `start` of a child moves its nearest story ancestor from inbox/ready to `doing` (`startStoryAbove`); `done`, `reopen` and an already-doing story are left alone. `mw judge` prints `Outcome:` then an `Evidence:` block (one folder per line, a story adds each child) and a `Decision:` block (one `[ ]` line per acceptance line). `accept`/`reject` reuse `refuseSession` and the `--by` rule of `gate`. Prompts: rule lines render from `GateEvidence/GateAcceptance/GateJudge` (session) and `workspaceGatesOn` (dispatcher, coach); `next` prints the full brief (evidence folder always).

## Session hook (`hookcontext.go`)

`mw context --hook-json` is handled in `run` before the board is loaded (like `status`): read-only, no lock, exit 0 on every path. It finds the board with `statusFindBoard` from the hook's `cwd`, the session from `MW_SESSION` or from `Session.ClaudeSession`, and prints the session's doing items (`hookContextText`), cut at 2000 runes. `ClaudeSession` (`claude_session`, omitempty) is set by `mw session start NAME --claude-session ID`. It is a field of the session record in the tenant file, not an item field and not a schema change: older binaries ignore the unknown key, and a rewrite by one of them drops it, which only means the hook falls back to `MW_SESSION`. `plugins/mw/scripts/ensure-mw.sh` chains it after its install check (only when the installed mw is 0.3.15 or later), keeping its Git Bash handling.

## Error codes and the session WIP limit

`main.go` `errCode`/`printJSONError`: a failed command run with `-j` prints `{"error","code","exit"}` on stdout; stderr text and the exit code are untouched (`TestErrorCodesContract` pins them). Sites tag errors with `coded(CODE, err)`; the rest is classified by message prefix. `wip.session` is a per-user key (`userkeys.go`, set with `mw configure`), read by `wipSessionOn` in `applyMove`.

## Session liveness

`Session.Updated` is the session's last activity (RFC3339, stored in the tenant file). It is set by `session start`/restart, by every `start|ready|block|done|reopen` the session runs, and by `note`/`edit` with `-s`/`MW_SESSION` (`touchSession`). Item age is the item's `Updated` (start, note and edit all refresh it). Nothing runs in the background: staleness is computed on read (`board` age, `review` STALE, `session list -v` age) and acted on only by `mw session stop --release` or `mw session reap`. Both go through `releaseSession` (reap.go): each doing item the session owns goes back to `ready` with owner, judgement and evidence cleared, a `ready` event and a note are appended, then the session becomes `stopped`. `reap` skips stopped sessions and the implicit `local@SLUG` session (it is derived from a scan of doing items and has no liveness). It runs under the normal tenant locks (`-t` locks one tenant, otherwise all), saves once, and `-n` returns before saving. A runner (wr) calls `mw session reap -t SLUG --except NAMES -j` per enabled tenant each tick (`--except` skips live runs); `--older-than` accepts Go durations plus a `d` suffix.

## Output

Structured reads use compact [TOON](https://github.com/toon-format/spec) by
default. `-j` selects JSON and `-v` reveals full fields. Default output bounds
and flattens user text; verbose output preserves the stored value.

Prompt retrieval intentionally emits raw prompt text by default for direct
handoff. `-j` wraps it for host-tool integration.

## Supported systems

The release workflow builds arm64 and x86-64 binaries for macOS, Linux, and
Windows. Native CI runs the test suite on all three operating systems. Releases
contain checksummed `.tar.gz` archives for macOS/Linux and `.zip` archives for
Windows.

## Build and release

Source builds require Go 1.23 or newer:

```sh
./build.sh          # build/mw
./build.sh clean
```

On Windows PowerShell:

```powershell
./build.ps1         # build\mw.exe
./build.ps1 clean
```

Installers always install a compiled release binary. Remove it with:

```sh
curl -fsSL https://raw.githubusercontent.com/stormovermars/miniwaves/main/uninstall.sh | sh
```

```powershell
irm https://raw.githubusercontent.com/stormovermars/miniwaves/main/uninstall.ps1 | iex
```

Pushing a `v*` tag publishes release archives:

```sh
git tag vX.Y.Z
git push origin vX.Y.Z
```

Release binaries have no runtime dependencies. Third-party licenses are in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## User config, board discovery, plugins (0.3.4)

- **User config** (`userconfig.go`, `userkeys.go`): `mw-user.json` in `os.UserConfigDir()/miniwaves/` (`MW_USER_CONFIG` overrides). `userkeys.go` is the one table of keys (group, kind, choices, default, help); `mw user get -j`, `mw configure` and the validation all come from it. `fanfare.enabled` and `fanfare.use` are fields of `UserFanfare`; every other key is a canonical string in `Settings`. Writers go through `updateUserConfig`: take `<file>.lock`, re-read the file, change it, write atomically, so concurrent `mw user set` and `mw configure` lose nothing. `user get` reports source `default`, `user`, or `board` (`fanfare.enabled` when the board explicitly turned fanfare off, which beats the user layer; `mw fanfare use` is not such a decision).
- **`mw boards`** (`boards.go`): read-only; reads only the index files (over 1 MB is an error entry), depth 1..6 (default `discover.depth`), skips dot folders, `node_modules`, `build`, `dist`, `vendor`, symlinks and on Windows `AppData`; caps at 200 boards and 5 s. `--up` does not stat ancestors inside macOS-protected folders (Documents, Desktop, Downloads, Library, Movies, Music, Pictures) unless it starts inside one, which avoids permission prompts.
- **Default fanfare**: with no pair on disk a WAV generated by `scripts/gen_fanfare_default.py` is played; it is cached under the user cache dir, named by content hash.
- **Plugins**: `plugins/mw` and `plugins/mw-jobs` are installed by `install.sh`, `install.ps1` and `installPlugins` in `update.go`; the retired `tidewatch` and `mw-watch` are uninstalled (`retiredPlugins`) only after the installs succeeded. `plugins/mw-jobs` is md-based (commands, a refresh hook, no TSX panes; `mw board --md` in `jobsmd.go`). `plugins/tidewatch` and its marketplace entry are removed; an installed copy is uninstalled by `mw update --plugins`.

## Product boundary

There are no distributed leases, estimates, sprints, budgets, assistant
launchers, plugins, webhooks, analytics, or background services. If
multi-machine orchestration is required, use a dedicated tool rather than expanding
miniwaves into another fleet service.

## One-call views

`brief.go` holds `briefOf`/`printBrief`, the single full view behind `show` and `next` (`briefDoc` in -j; `next -j` adds `gates` and `skipped`), the `log` feed (`tenantRows`, newest first; events are stamped RFC3339Nano by `emit`, so ties sort by time), `lastActivity`/`doneToday` for the board lines, and `done --evidence` (`splitEvidenceArgs`, `copyEvidence`: one path per `--evidence` flag, folders are copied recursively without junk or symlinks, every path is checked and name collisions refused before the first copy, copy happens before `checkEvidence` and aborts before any state change; the copied files are recorded on the item even with the gate off). `moveCommand` registers an unknown session only on `start`, revives a stopped one, and uses the per-tenant implicit session `local@<slug>` (`localID`, `syncLocal` derives its item/state from the doing items, `displayName` shows it as `local`) for a session-less `start`; `done`/`block` without `-s` attribute `by` to the item's assignee; `selectTenantRaw` ignores an unknown session (with a stderr warning when no tenant was given). `wave step` parses `KEY=NOTE` pairs and validates them all before ticking any.

## Fanfare (`fanfare.go`)

Config is the index's `fanfare` object; a tenant's `fanfare_finale_at` stamps the finished transition (set in `done`/`wave close` before the save, cleared by `save` when a non-done item exists). The trigger only queues a clip (`fanfarePending`); `run` plays it after the command succeeded and after the locks are released, as a detached player (`Setsid`; `DETACHED_PROCESS|CREATE_NEW_PROCESS_GROUP|CREATE_BREAKAWAY_FROM_JOB` on Windows, retried without the breakaway flag if the start is refused) so the CLI never waits and a failure is a stderr note. `playerCommand` builds the command per OS; `MW_FANFARE_PLAYER` (a stub called `STUB FILE SECONDS VOLUME`) replaces it in tests. `pickPair`/`scanPairs` skip or mark (`unplayable here`) pairs whose formats `playerCommand` rejects for this OS and the installed players. Pids of started players go to `<dir>/.players.json` with a process stamp (`ps lstart`, tasklist image name) so `mw fanfare stop` never kills a reused pid. `fanfare_sources.go` is generated by `scripts/gen_fanfare_sources.py` from `scripts/fanfare_sources.json` (Commons file pages checked by hand: US government band or CC0/CC BY by the performer); every track has a pinned sha256 (`TestBuiltinPairsAreFreeLicences` enforces it), `download` compares the streamed hash before renaming `.part` and fetch refuses unpinned tracks. The HTTP client follows redirects only to https on upload.wikimedia.org, caps a track at 64 MB and bounds the transfer (600 s, 30 s to headers). `mw tenant protocol SLUG` without set/reset is read-only (no write locks). The collection stays outside the board folder.
