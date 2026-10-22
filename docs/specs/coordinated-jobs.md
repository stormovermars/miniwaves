# Coordinated jobs in mw: plan

Draft 4.2, 2026-10-22: an audit against the built code (main ee82412), the tidewatch branch and the mw-jobs dev mod. It records the built status, decisions D1–D6 and 16 findings, listed at the end. Draft 4.1 added the mw news pane. Draft 3 had a second Opus audit with 20 findings, all resolved. Owner: Sir.

## Status (2026-10-22)

| Milestone | Built | Where | Board |
|---|---|---|---|
| M0 Decide | yes | `mw note TSK-011` | TSK-011 inbox |
| M1 Data | yes: jobs.go, needs.go, cursor.go, userconfig.go | main d37a58a | TSK-012..014, 018 inbox, noted "awaiting review" |
| M2 News | md/html built (news.go); `--format text` in progress on branch `news-blocks`; `news.path` not started | main 66cb01d | TSK-015 noted; TSK-029, TSK-030 new |
| M3 Roles + kit | yes; buildlock.ps1 has no test | main ee82412 | TSK-016, 017 noted; TSK-031 new |
| M4 Fanfare (0.2.9) | yes, untagged and unpushed | main dd19520, 0a59d82 | TSK-022 noted |
| M5 tidewatch | partly: job tabs, fanfare footer, needs, styles. The dev mod's news UX is missing, and the branch predates M2/M3 | branch `worktree-agent-a711da62307412bbf` 57d6ec0 | TSK-019, 021, 023; TSK-032..034 new |
| M6 Release 0.3.0 | no | — | TSK-020 |

Main is 20 commits ahead of origin (0.2.7) and is local only. Nothing is tagged after v0.2.7.

## Why

The TinySheet/TinyDoc consolidation proved a pattern on top of mw:
- a **coordinator** that spawns one session per task and reviews it before done;
- a **news desk** that keeps a news page and the human docs current;
- a **live pane** in Claude Code (**tidewatch**);
- alerts and fanfare when a milestone closes or a human is needed.

Today it is hand-built: tiny-corpus `tools/news.py`, `~/tools/jobs` (registry, jobstat, templates, the `job-start` skill) and the `mw-jobs` dev mod. That prototype is the acceptance reference, and it is retired once mw ships this.

## Principles

- **mw stays a board, not a runner.** It ships prompts, data and rendering. Sessions are started by the user's assistant or by wr. There is no build lock in mw.
- **Reuse before adding:**
  - the coordinator is the existing `dispatcher` role, extended;
  - needs-human and decisions are item **events**;
  - change-only output reuses `publish` and its hash;
  - reap protection reuses `--except`.
- **No schema bump** (audit #2). Everything new is an omitempty field or an event in schema 8, as fanfare, gates, protocol and tags were. A board that never opts in behaves exactly as 0.2.x, and wr keeps working.
- **Nothing executes from repo content.** Board keys are data. Paths from board keys stay inside the workspace. Commands live only in per-user config.
- **Committed board = shared defaults. Per-user file = personal choices** (alerts, fanfare on/off, active fanfare set).
- **Cross-platform.** Go only, argv instead of shell strings, no HOME assumptions, tested on the existing 3-OS CI from its own milestone.

## Releases

| Version | Content | Schema |
|---|---|---|
| 0.2.8 | the pending Stop-dialog work in the main checkout, committed as is | 8 |
| 0.2.9 | fanfare sets + `fanfare use` + `fanfare -j` superset (TSK-022) | 8 |
| 0.3.0 | jobs: config, events, cursor, news, roles, kit, per-user config, tidewatch | 8 |

The marketplace serves `main`, so merging to main is a release (audit #4). tidewatch and the ensure-mw `want=` bump land on main only in the 0.3.0 release commit. Until then they live on a branch.

**Release order (D5, finding 3).**
1. 0.2.8 and 0.2.9 are tagged at their own commits (b2c9b2f, 0a59d82), not at the main tip, and origin/main is pushed only up to 0a59d82. M1–M3 touch no plugin file, so that push is safe.
2. The main tip is not pushed until 0.3.0. A binary built from main reports 0.2.9 but contains jobs.
3. The 0.3.0 release is one commit holding the `version` bump, `plugins/tidewatch`, its marketplace.json entry and ensure-mw `want="0.3.0"`. It lands after TSK-029..034 are merged, with the tag pushed in the same step.

## Configuration

### Board (committed, data only)

The board config is stored in `.miniwaves.json` as an omitempty `jobs` object, keyed by job id at workspace level (audit #11). Tenants carry only `job: ID`.

| Key (`jobs.<id>.`) | Default | Meaning |
|---|---|---|
| `title` | id | pane and news title |
| `max_workers` | `4` | dispatcher prompt variable |
| `worktrees` | `on` | prompt variable: one worktree + branch per task |
| `lock` | `` | prompt variable: an optional build-lock command *name* the kit provides |
| `news.out` | `docs/news/` | Markdown target; relative, cleaned, inside the workspace after symlink resolution (audit #9) |
| `news.url` | `` | link shown in the pane and news |
| `summary` | `` | relative path to a harness summary the pane shows as text (audit #17) |
| `pane` | `on` | whether tidewatch shows this job |
| `pane.style` | `` | style hint; a name matching `^[a-z0-9-]{1,32}$`, the user's choice wins (audit #8) |

Commands:
- `mw jobs enable ID -t SLUG...` adds tenants to a job, creating the job if needed.
- `mw jobs disable ID [-t SLUG]` removes tenants; the job's data stays.
- `mw jobs set ID KEY=VALUE...`
- `mw jobs show [ID] [-j]`

ask/answer/decide work on any board, with no enable gate (audit #20).

### Per-user (`mw-user.json`)

- It lives in `os.UserConfigDir()/miniwaves/`. `mw user show [-j]` prints the values and the path, and `mw user set KEY=VALUE...` writes them (audit #10).
- On Unix the file is refused if it is group- or world-writable. On Windows there is no mode check, and USAGE says so.
- Keys:
  - `alerts.on`: event names;
  - `alerts.exec`: an argv array;
  - `fanfare.enabled` and `fanfare.use`: these override the board's fanfare (audit #14).

## What goes into mw

| # | Piece | Design |
|---|---|---|
| 1 | Needs-human | `mw ask ID QUESTION... [-s NAME]` and `mw answer ID TEXT... --by NAME` append item events (`ask`, `answer`); old mw round-trips them. An open ask is the last `ask` with no later `answer`. `review -j` gains `needs`; the board shows "needs you". |
| 2 | Decisions | `mw decide TEXT... --by NAME [-b WHY] [-p ID]` appends a `decide` event to `-p ID`, else to the tenant's "Decisions" chore, created on first use. The Decisions page also lists accept and reject. |
| 3 | Event cursor | `mw log [-A] --since CURSOR [-j]` returns events plus `next`. The cursor is opaque. Each event's key is (tenant, item, event index). A read re-scans a lookback window (default 24 h before the cursor time) and dedupes by key, so events stamped earlier that arrive later via git are not lost (audit #12). Tests cover git-merged late events and archive pruning. |
| 4 | Roles | `prompts/dispatcher.md` is extended with the review → merge → re-test → done loop, one task per session, and "ping when ready". There is a new `prompts/newsdesk.md`. `mw get-prompt dispatcher --job ID` fills `.MaxWorkers`, `.Worktrees`, `.Lock`. **Review convention** (audit #16): a dispatcher-spawned worker signals with `mw note ID "ready for review"` and stays `doing`; the dispatcher runs `mw done`. wr builders keep `mw done`/`block` unchanged. |
| 5 | News | **As built (finding 9):** `mw news [--job ID] [--format md\|html\|text] [-o PATH\|-] [-j]` is its own command (news.go). It reuses publishHTML's page head, escaping and sidecar hash rather than living inside `publish`. Each job gets one file, `<news.out>/status.md` (or .html). The sidecar `PATH.hash` holds the stamp and the hash. The hash leaves out the Updated line and the "N moved since the last run" count, so an unchanged board writes nothing. `-o -` prints to stdout and writes nothing. `-j` prints `[{job,path,written}]`. Two jobs that share a target are refused. Markdown is cut to 9,500 characters per job, trimming events first, and cells escape `< > & \` \| \\` and newlines. `summary` is read confined: a regular file only, at most 20 KB. Only `-o` may leave the workspace. With several jobs, `-o PATH` writes one file of all sections, which can exceed the pane limit (finding 10), so the pane always passes `--job`. `--format text` (branch news-blocks, TSK-029) is plain text for terminals: box-drawing, no ANSI, control characters stripped, `--width`, stdout unless `-o`, and no sidecar. |
| 6 | Alerts | Per-user `alerts.exec` runs from `mw log --follow --exec` only. The event goes in a temp JSON file passed as an argument, with no stdin and no shell. `MW_ALERT_EXEC` is a stub for tests. `--follow` and `--exec` join options()' boolean list. **Deferred to 0.3.x** (audit #20 cut; TSK-035): the pane already alerts. `alerts.on` and `alerts.exec` are already validated and stored by `mw user set`, but they are inert until then. USAGE says so (D6). |
| 7 | Kit | `mw init --kit coordinated --job ID` writes a PLAN.md skeleton, NEWSDESK.md and buildlock.sh/.ps1 from embedded templates into the workspace folder. It never overwrites an existing file, and it prints the two `get-prompt` commands. **As built (finding 11):** if the job is missing, it also creates the empty job in the board. buildlock is a mkdir lock with stale-pid takeover and exit 75 on timeout. Two waiters can both take over a stale lock, and the .ps1 has no test (finding 6, TSK-031). |
| 8 | Reap | No new flag (audit #15). When a tenant's job exists, `reap` skips sessions whose agent is `dispatcher` or `newsdesk` by default, so wr's existing `reap` call is safe. `--except` stays the explicit override. |
| 9 | Fanfare sets | `fanfare.sets` / `fanfare.use`; `mw fanfare sets [-j] \| sets add NAME PAIR... \| sets drop NAME [PAIR...]`; `mw fanfare use NAME\|all`; `mw fanfare -j` is a superset of the old keys. Picking is random within the active set's playable pairs, and an empty set falls back to all. mw's fanfare is the **only** sound for milestones and finales (audit #13). Ships 0.2.9. |
| 10 | tidewatch | A separate opt-in plugin; see below. |

## tidewatch (pane plugin)

- **Install and use.** Install with `/plugin install tidewatch@miniwaves`. `/tidewatch` opens the pane, and `/tidewatch add PATH` / `remove ID` manage boards. It shows every job with `pane=on` on the registered boards.
- **One pane, two tabs (D1).** The pane API cannot make a separate panel, so news is a tab of the one pane. Tab titles use a lowercase `mw` and the short job id.
  - **mw jobs · &lt;job&gt;** is the board view. It has a primary **News** header button.
  - **mw news · &lt;job&gt;** shows the job's news.
    - It opens together with jobs, unfocused, and `/tidewatch news [job]` focuses it.
    - **◂ jobs** goes back.
    - A **●** in the tab title marks unseen news. The mark comes from a content hash, with the Updated line left out, kept in `$.store`.
- **News content comes from mw, never from a file (D2, findings 1 and 5).** The pane runs `mw news --job ID -o -`, which writes nothing.
  - `normal` style: `--format md`, drawn as Svg by a small md renderer on the sans scale. Link lines become clickable link rows, and the "Page:" line for `news.url` is dropped.
  - `blocks` style and the terminal surface: `--format text --width N`, drawn verbatim in one mono Svg (one terminal font and size, box-drawing characters). A **Links** row is built from `news.url`. The text layout lives only in mw.
  - Output is cut at 9,500 characters with a note. The summary arrives inside the news output, so the pane never reads `summary` or `news.out` itself.
  - **⧉** opens the news file for reading (D3); see Security.
- **What it shows.** Urgency colors (needs you, waiting, progress, new), plus milestones, harness `summary`, last events, the news link, the status line and toasts. It has Refresh, an Auto timer (off/30s/1m/5m, default 1m) and −/#/+ size.
  - The style, timer and font-size atoms are shared by both tabs.
  - Auto refresh has no expiry (D4): it runs local mw only and costs no tokens.
- **Security.**
  - The pane never reads `$.env` HOME and never reads a path a board supplies.
  - The registry's `newsFile`/`summary` overrides from the prototype are dropped (findings 1 and 8).
  - **⧉ (D3, finding 2):**
    - The path is `news.path` from `mw jobs show -j` (TSK-030), the confined real path, and it is skipped when empty.
    - `lstat` must show a regular file, not a symlink, ending in .md or .html.
    - It runs argv only: macOS `open -t PATH` (the text editor, never an app launch), Windows `notepad.exe PATH`, Linux `xdg-open PATH`.
    - file:// Links are refused by every surface, so there is no Link fallback.
- **Storage** (audit #7). The registry, user styles, the seen-set and the chosen style live in `$.store`. Board paths are stored absolute, and nothing reads HOME.
- **Running mw** (audit #6). An absolute mw path is resolved once (`command -v` / `where`, or the ensure-mw install path) and is always used as argv[0]. A bare `mw` is never run with a repo as cwd. If mw can't be found, the pane shows a how-to.
- **Alerts** (audit #13). The pane notifies once per event key (seen-set in `$.store`). Its only sound is a short Ping for needs-you/blocked. Milestone sounds come from mw's fanfare.
- **Fanfare footer.**
  - Buttons: on/off, a set selector (all + named sets) and Test.
  - Commands: `/tidewatch fanfare on|off|use NAME|test`.
  - Writes go to the per-user config (`mw user set fanfare.*`), never the board.
  - The footer is hidden when mw lacks `fanfare -j`, or lacks `user` before 0.3.0.
- **Surfaces** (audit #18). The desktop, vscode and mobile surfaces draw milestones and harness as Svg. The terminal draws cell bars. The footer uses Buttons only, since mobile has no Select or Input. `$.process.run` calls stay within its 30 s timeout, and `mw fanfare test` detaches.

### Styles

- **Tokens, never code:**
  - `font`: enum `sans|mono`;
  - `sizes`: headline, body, note and label in px, Svg only; Text uses the size enum;
  - `rowHeight`;
  - `columns`: name and count widths;
  - `bar`: `{kind:"rect",height,radius}` or `{kind:"glyphs",fill,rest}`;
  - `colors`: fg, dim, track, needs, waiting, progress, new, pass and fail, each in light and dark;
  - `sections`: order and visibility;
  - `scale`: the three −/#/+ factors.
- **Built-ins.** `normal` (sans, 6px rounded bars) and `blocks` (mono, `█░`) ship as JSON in the plugin and double as examples. They reproduce the designs on the Jobs panel design canvas.
- **User styles.** User styles are kept in `$.store` and merged over `normal`.
  - `/tidewatch style NAME|list` picks or lists styles.
  - `/tidewatch style new NAME` copies `normal`.
  - `/tidewatch style import PATH` and `export NAME PATH` read and write JSON files.
- **Validation** (audit #8).
  - Colors match `^#[0-9a-fA-F]{3,8}$`.
  - Numbers are finite and clamped.
  - Glyphs are exactly one code point, not one of `<>&"'`, and xml-escaped.
  - Names match `^[a-z0-9-]{1,32}$`.
  - Bad values are dropped with one toast.
  - The Svg source is capped below 131072 characters by limiting rows.

## Milestones (board waves)

| Wave | Content | Items | Done when |
|---|---|---|---|
| M0 Decide | versions as in Releases; schema stays 8; tidewatch separate, opt-in, merged at release; news md first, no `--publish` | TSK-011 | recorded with `mw note TSK-011` (decide does not exist yet, audit #1) |
| M1 Data | jobs config, ask/answer/decide events, review `needs`, `log --since` cursor, `mw user` | TSK-012..014, TSK-018 (user part) | golden + late-event cursor tests green on 3 OSes; old mw round-trips a 0.3.0 board |
| M2 News | `mw news` md/html/text; escaping; confined `news.out`; `jobs show -j` `news.path` | TSK-015, TSK-029, TSK-030 | goldens incl. `<script>` and `\|` titles; unchanged → no write; `../` refused; text strips ESC |
| M3 Roles + kit | dispatcher/newsdesk prompts, `--job` variables, `init --kit`, reap skips job roles, buildlock tests | TSK-016, TSK-017, TSK-031 | prompt goldens; kit idempotent; wr's reap call leaves a dispatcher alone; buildlock.ps1 runs on the windows CI leg |
| M4 Fanfare (0.2.9) | fanfare sets, `use`, `-j` superset | TSK-022 | tests green; released before M5 |
| M5 tidewatch | plugin port, styles, fanfare footer, absolute mw path, `$.store`; rebase onto main; the news tab port from the dev mod; safe ⧉; a live host run | TSK-019, TSK-021, TSK-023, TSK-032, TSK-033, TSK-034 | `claude plugin validate/test` green; a user style changes the pane; the pane reads no repo file and no HOME; screenshots of desktop + terminal in both styles |
| M6 Release 0.3.0 | Opus diff audit; USAGE/PROTOCOL/TECHNICAL; releases/0.3.0.md; plugin + ensure-mw on main in one commit; wr checked against a 0.3.0 board; tiny-corpus moved over; prototype retired | TSK-020 | released; tiny-corpus runs on it |

`log --follow --exec` is deferred to 0.3.x (TSK-035, no wave).

The board's wave titles are historical, and mw cannot retitle waves or items (finding 13):
- WAV-002..008 are M0..M6;
- WAV-006 "Jobs M4 Alerts" is M4 Fanfare;
- WAV-007 "Jobs M5 Plugin" is M5 tidewatch;
- WAV-009 is the ceremonies plan (0.3.1).

TSK-018 moved from WAV-006 to WAV-003 (M1). Notes on TSK-012, 017, 018 and 019 correct their stale titles.

## Decisions (draft 4.2)

- **D1, news is a tab.** It is a tab of the one tidewatch pane, opened with jobs and unfocused, because the pane API cannot make a separate panel.
- **D2, the renderer (accepted by Sir 2026-10-22).** The pane gets news only from `mw news --job ID -o -`. `blocks` and the terminal draw `--format text` verbatim, and `normal` draws `--format md` with a small md→Svg renderer.
  - Options weighed:
    - (a) The prototype's own md renderer reading status.md in both styles. It duplicates layout and needs path handling and confinement in TypeScript.
    - (b) `--format text` for everything. It gives one renderer and a thin plugin, but it loses the sans scale and the clickable link rows of `normal`.
  - Chosen: (b) where the output is mono anyway, plus a renderer for `normal` only. The plugin reads no file, mw owns confinement, sanitising and layout, and both are tested on 3 OSes. The cost is one local mw run per job per refresh, within the 30 s timeout.
  - If Sir wants strictly one renderer, `normal` news falls back to the same text output in mono.
- **D3, ⧉.** See tidewatch → Security.
- **D4, auto refresh.** It never expires.
- **D5, release order.** See Releases.
- **D6, thinness.** mw gains no runner and no new config surface beyond `news.path` (output only).
  - The `alerts.*` user keys stay, inert and documented.
  - `.Ceremonies` is an empty template hook, so ceremonies (0.3.1) add prompt text, not code paths.

## Out of scope

mw starting sessions; built-in push or email providers; hosting the news page; a build lock inside mw; a schema bump.

## Audit log (draft 3 → 4)

1. **M0 recorded with a missing `decide`:** use `mw note`.
2. **Schema 9 broke non-opted boards:** there is no bump; new data uses omitempty fields and events.
3. **wr MaxSchema 8:** moot, since there is no bump. wr is still checked at M6.
4. **The marketplace serves main:** the plugin and the ensure-mw bump land only in the release commit.
5. **Uncommitted 0.2.8 and the fanfare overlap:** 0.2.8 → 0.2.9 (sets) → 0.3.0, rebasing in that order.
6. **mw resolved from a repo cwd on Windows:** an absolute mw path.
7. **No plugin data dir:** `$.store`.
8. **Style and hint injection:** strict token validation and name regex.
9. **The `news.out` path escaped the workspace:** it is confined.
10. **Per-user config on Windows and its location:** `mw user show|set`, with no mode check on Windows.
11. **Job keys per tenant:** the keys are per job at workspace level, with `--job`.
12. **The cursor lost late git-merged events:** event key plus a lookback window and dedupe.
13. **Duplicate sounds and alerts:** the fanfare is the only sound, and the pane dedupes by key.
14. **Fanfare toggles wrote the board:** they write the per-user config instead.
15. **`--keep-role` was unknown to wr:** reap skips job roles by default.
16. **review=note conflicted with wr:** it applies to dispatcher-spawned sessions only.
17. **Prototype parity:** the `summary` and `news.url` keys.
18. **API details:** Svg-only px sizes, Buttons only on mobile, the 30 s timeout.
19. **Flags and escaping:** booleans in options(), `|` and newline escaping, stamp-free hash, a kit that never overwrites.
20. **Board drift and scope:** items re-synced, ask/decide ungated, `--follow --exec` deferred.

## Audit log (draft 4.1 → 4.2, against main ee82412)

Severity: H = fix before 0.3.0, M = needed for 0.3.0, L = record only.

1. **(H, security) tidewatch reads paths that a board supplies.**
   - It joins `summary` and `news.out` from `jobs show -j` to the board folder, or takes them as absolute paths.
   - `jobs show -j` echoes the board JSON unvalidated, and only `jobs set` validates it. A hand-edited `../../.ssh/id_rsa` would therefore be read into the pane.
   - Resolved: D2, so the pane reads no file, plus TSK-033.
2. **(H, security) The prototype's ⧉ runs `open <board>/docs/news/status.md`.**
   - If a repo commits status.md as a symlink to an `.app` or `.command`, opening it launches that program.
   - Resolved: D3. `news.path` comes from mw, `lstat` requires a regular file with an .md/.html extension, and macOS uses `open -t`. TSK-030, TSK-033.
3. **(H, release) 0.2.8 and 0.2.9 are untagged and unpushed, and M1–M3 sit on main above them.**
   - ensure-mw already says `want="0.2.9"`, and a binary built from main claims 0.2.9.
   - Resolved: D5. Tag at their own commits, push only to 0a59d82, and make 0.3.0 one commit. TSK-020 has a note.
4. **(M) The tidewatch branch is based on d168dcc, before M2/M3, and lacks the dev mod's news UX.**
   - Missing: the Svg md renderer (normal/blocks), shared atoms, link rows and the Links row, the News button, opening with jobs, the ● hash mark, ◂ jobs, ⧉, and refresh with no expiry.
   - Resolved: TSK-032. TSK-019 has a rebase note.
5. **(M) News path drift.**
   - The plugin treats a `news.out` ending in .md as the file itself and otherwise defaults to `docs/news/status.md`.
   - mw always writes `<news.out>/status.md`, and the dev mod hardcodes `docs/news/`.
   - Resolved: D2 (stdout) and `news.path` (TSK-030).
6. **(M) buildlock.**
   - The .ps1 is untested, and the CI has no pwsh step.
   - Both scripts race on stale takeover: two waiters can both `rm` the lock and both hold it.
   - Resolved: TSK-031. Take over by an atomic rename of the stale folder, then mkdir.
7. **(M) No live host run of the plugin exists.** `claude plugin test` alone misses surface quirks such as file:// refusal and the missing separate panel. Resolved: TSK-034, with screenshots on desktop and terminal.
8. **(L) The prototype reads HOME and the `~/tools/jobs` registry.** These must not be ported, since nothing should read HOME. Resolved: TSK-033.
9. **(L, drift) `mw news` is its own command, not part of `publish`.** It adds `-o -`, `-j`, one file per job, a duplicate-target refusal, and a stamp + hash sidecar with a "moved since the last run" count. Accepted; item 5 is updated.
10. **(L, drift) Multi-job `-o PATH` md can exceed 9,500 characters.** Accepted, because the pane always passes `--job`.
11. **(L, drift) `init --kit` creates a missing job in the board, not files only.** Accepted, because it is the natural first step; item 7 is updated.
12. **(L, drift) `.Ceremonies` is an empty prompt hook ahead of the ceremonies plan.** Accepted (D6).
13. **(L, board) TSK-011..023 were all inbox with stale titles, and WAV-006 was named Alerts and held TSK-018.**
    - Resolved: "awaiting review" notes on the built items, and TSK-018 moved to WAV-003.
    - Stale titles are explained in notes, because mw has no retitle command.
    - Built items stay open until review and evidence, since the evidence gate is on at level proof.
14. **(L, thinness) The `alerts.*` keys are stored but nothing runs them in 0.3.0.** Kept, and USAGE says so. TSK-035 (0.3.x).
15. **(OK, verified) Prompts, kit templates and confinement.**
    - Prompts and kit templates are `go:embed`, so nothing comes from the repo.
    - `news.out` and `summary` are confined after EvalSymlinks.
    - `summary` must be a regular file, capped at 20 KB.
    - `lock` is a name, not a command line.
    - `mw-user.json` is refused when group- or world-writable on Unix.
    - The prototype's osascript fallback escapes `\` and `"`.
16. **(L, tests) M1–M3 have unit and golden tests, but the 3-OS CI has not run on them.** main is not pushed. Run CI on a branch push before the 0.3.0 commit (TSK-020).
