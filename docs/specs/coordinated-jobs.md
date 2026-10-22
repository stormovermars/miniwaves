# Coordinated jobs in mw: plan

Draft 4.1, 2026-10-22 (adds the mw news pane). Draft 3 had a second Opus audit, with 20 findings; all are resolved below, and the audit is at the end. Owner: Sir.

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
| 5 | News | `mw news [--job ID] [--format md\|html] [-o PATH]` is built inside `publish`, reusing `publishHTML`, its escaping and its hash. The hash leaves out the generated-at stamp, so an unchanged board writes nothing. Markdown cells escape `< > & \` \|` and newlines. Only `-o` may leave the workspace. `--publish` is dropped. |
| 6 | Alerts | Per-user `alerts.exec` runs from `mw log --follow --exec` only. The event goes in a temp JSON file passed as an argument, with no stdin and no shell. `MW_ALERT_EXEC` is a stub for tests. `--follow` and `--exec` join options()' boolean list. **Deferred to 0.3.x** (audit #20 cut): the pane already alerts. |
| 7 | Kit | `mw init --kit coordinated --job ID` writes files only: a PLAN.md skeleton, NEWSDESK.md and buildlock.sh/.ps1. It never overwrites an existing file, and it prints the two `get-prompt` commands. |
| 8 | Reap | No new flag (audit #15). When a tenant's job exists, `reap` skips sessions whose agent is `dispatcher` or `newsdesk` by default, so wr's existing `reap` call is safe. `--except` stays the explicit override. |
| 9 | Fanfare sets | `fanfare.sets` / `fanfare.use`; `mw fanfare sets [-j] \| sets add NAME PAIR... \| sets drop NAME [PAIR...]`; `mw fanfare use NAME\|all`; `mw fanfare -j` is a superset of the old keys. Picking is random within the active set's playable pairs, and an empty set falls back to all. mw's fanfare is the **only** sound for milestones and finales (audit #13). Ships 0.2.9. |
| 10 | tidewatch | A separate opt-in plugin; see below. |

## tidewatch (pane plugin)

- **Install and use.** Install with `/plugin install tidewatch@miniwaves`. `/tidewatch` opens the pane, and `/tidewatch add PATH` / `remove ID` manage boards. It shows every job with `pane=on` on the registered boards.
- **Two panes.** Their titles use a lowercase `mw` and the short job id.
  - **mw jobs · &lt;job&gt;** is the board view.
  - **mw news · &lt;job&gt;** renders the job's news Markdown (`news.out`/status.md, from `mw news --format md`) with the Markdown element.
    - It reads the file only and never runs repo scripts. It is cut at 9,500 characters with a note, because the Markdown element refuses more than 10,000.
    - A small ↗ Link opens `news.url` in the browser.
    - It opens on request only: the News button in mw jobs, or `/tidewatch news [job]`.
- **What it shows.** Urgency colors (needs you, waiting, progress, new), plus milestones, harness `summary`, last events, the news link, the status line and toasts. It has Refresh, an Auto timer (off/30s/1m/5m) and −/#/+ size.
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
| M2 News | `mw news` md/html via publish; escaping; confined `news.out` | TSK-015 | goldens incl. `<script>` and `\|` titles; unchanged → no write; `../` refused |
| M3 Roles + kit | dispatcher/newsdesk prompts, `--job` variables, `init --kit`, reap skips job roles | TSK-016, TSK-017 | prompt goldens; kit idempotent; wr's reap call leaves a dispatcher alone |
| M4 Fanfare (0.2.9) | fanfare sets, `use`, `-j` superset | TSK-022 | tests green; released before M5 |
| M5 tidewatch | plugin port, styles, fanfare footer, absolute mw path, `$.store` | TSK-019, TSK-021, TSK-023 | `claude plugin validate/test` green; a user style changes the pane; one live check |
| M6 Release 0.3.0 | Opus diff audit; USAGE/PROTOCOL/TECHNICAL; releases/0.3.0.md; plugin + ensure-mw on main in one commit; wr checked against a 0.3.0 board; tiny-corpus moved over; prototype retired | TSK-020 | released; tiny-corpus runs on it |

`log --follow --exec` is deferred to 0.3.x.

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
