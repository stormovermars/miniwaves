# mw as a radical engineering tool — minimal plan (v3 after two Opus audits, 2026-10-11)

Radical engineering: agents scaffold, build, fix and polish; the human only decides and judges. mw is the **ledger**; the active part (runner, scheduler, LLM roles, server, multi-user) is a separate tool, the **rider** (`wr`, spec `~/apps/wr/docs/specs/wr.md`), that talks to mw only through its CLI. This plan adds **proof and judgement** to mw, nothing else.

Threat model (one USAGE line): mw guards against lazy or confused agents, not hostile ones. An agent can edit the JSON, `reparent` out of a story or add a path-less tenant; the guard is friction plus an audit trail (`by`, `judged_by`, timestamps, notes).

## Gates — adaptive, per tenant, automatic or on request
`gates: {evidence, acceptance, judge}` on the Tenant (optional workspace default), each `auto|on|off`, absent = workspace default. **Boards migrated from schema 5 get workspace default `off`** (the live fleet keeps today's behaviour); new boards and the miniwaves dogfood board start at `auto`. `mw gate [-t]` prints setting, effective state and a one-line reason; `mw gate NAME on|off|auto [-t]` is the on-request override (manual wins; `auto` also clears the latch). The gates line prints on `board`, `context`, `next` **only when at least one gate is effectively on**. `gates_seen: {name: effective}` on the Tenant is written only by commands that already save and by `mw gate`; a change since `gates_seen` prints one stderr line (repeats until a saving command runs). Prompts render only rules that are effectively on. `gate`, `accept`, `reject`, `edit -c` refuse when `MW_SESSION` or `-s` is set (humans only); `accept`/`reject`/`gate` with no terminal on stdin need `--by NAME`.

Auto = rule OR latch (`gate_latch: {name: reason}` on the Tenant, e.g. `{"acceptance":"STR-003","evidence":"reject TSK-012"}`):
- `evidence`: rule = tenant dir has a build manifest (`build.gradle*`, `*.xcodeproj`, `project.yml`, `Package.swift`, `go.mod`, `package.json`, `Cargo.toml`) or `releases/` ("product tenant"); missing/absent dir = off (`mw gate` says so). Latch set by one `reject ID --evidence-gap`. File-system check only for the tenants shown.
- `acceptance`: latch set the first time any story in the tenant gets acceptance text; stays on.
- `judge`: rule = product tenant. **Never relaxes by itself.** `accept_streak` on the Tenant (reset by `reject`); at 20 `mw gate`/`board` show `judge: on (20 straight accepts — mw gate judge off?)`.

## Data model (schema 5 → 6; older binaries refuse the board)
Item gains, all omitempty: `evidence []string` (relative path + size, recorded at `done`), `judged` (RFC3339 | `legacy` | `ungated`), `by` (session at `done`), `judged_by` (human at `accept`), `attempts int`. Tenant gains `gates`, `gates_seen`, `gate_latch`, `accept_streak`. Status set unchanged. Migration: `loaded := w.Version` before overwrite; only when `loaded < 6` mark existing done items `judged: "legacy"`. `done` with judge effectively off sets `judged: "ungated"`; `review` and burndown treat `legacy`/`ungated` as judged. **`ready`/`start`/`reopen`/`reject` clear `judged`, `judged_by`, `evidence`; `done` re-records `evidence`.** `add -S done` is refused while evidence or judge is effectively on. Long flag `--accept` → `--acceptance` (alias kept). New keys in `validOptions`; `evidence-gap` boolean.

Judgeable = a story, or a work item (task/bug/spike/chore) with no story ancestor. Prompts and waves are never judged. Acceptance = the item's own, else the nearest non-empty ancestor's.

## Wave A — gates + evidence (R0+R1, Sonnet ≤60k, Opus review)
- `gates`/`gate_latch`/`gates_seen` keys, auto inference, `mw gate`, stderr flip line, refusals, `--by`.
- For the rider: `mw version -j` (`{version, schema}`); `mw next -j` prints `null` with exit 0 when nothing is ready (text mode unchanged).
- `mw edit ID [-c TEXT] [-b TEXT]` (acceptance was creation-only; `-c` humans only).
- Evidence gate on, for judgeable and work items: `done ID TEXT` requires `<evidence>/<slug>/<ID>/` with ≥1 regular non-empty file whose name does not start with `.` and is not `Thumbs.db`/`desktop.ini`; folder name must match the ID exactly (`ReadDir` of the parent, case-sensitive compare; `filepath.Join` only); else `--no-evidence REASON` (stored in a note). No path flag. `show -v` lists the files; `list -v` shows `E`. Waves use `<evidence>/<slug>/WAV-NNN/`, checked at `wave close` (also accepts `--no-evidence REASON`); gate off = no folder check.
- Acceptance gate on: `next` skips work kinds whose effective acceptance is empty and prints `skipped N without acceptance`. USAGE note: one story with acceptance latches the gate; standalone bugs then need their own `-c`.
- `-j`: `itemDetail` gains the new fields; `gate -j` = array of `{name, setting, effective, reason}`; `board -j`, `context -j`, `next -j` gain `gates: {name: effective}`; `next -j` gains `skipped`.
- Tests: each gate on/off, latches survive `edit`, inheritance across two levels, refusals (`gate`/`edit -c` under `MW_SESSION`), `add -S done` refused, `.DS_Store`-only and wrong-case folders fail, flip line + `gates_seen` only written by saving commands, 5→6 migration runs only below 6, schema-7 board refused, golden output of board/context/next/prompts with all gates off (byte for byte with today). Docs: USAGE six lines on gates + threat model, TECHNICAL schema 6, `mw description`, `usage()`.
Done when: with a gate on, nothing closes without a file the human can open; with gates off, today's output byte for byte.

## Wave B — judgement + loop (R2+R3, Sonnet ≤60k, Opus review; brief ships prompts/*.md and a target length per new prompt line)
- `mw review [-t] [-j]`: `judge` = judgeable items awaiting judgement (a story appears once all its children are done, whatever its own status; `attempts` shown as `×N`), `blocked`, `acceptance` = needs acceptance (omitted when the acceptance gate is off), `waves` ready to close (steps ticked, members done, and judged when the judge gate is on). Judge off → `judge: []`. `-j` = `{judge:[], blocked:[], acceptance:[], waves:[]}`.
- `mw judge ID`: an Outcome line, then an Evidence block (one folder per line; a story adds each child's) and a Decision block (one `[ ]` line per acceptance line).
- `mw accept ID` refuses non-judgeable items; sets `judged`, `judged_by`, `accept_streak++`; on a story whose children are done it sets `done` and `judged` together.
- `mw reject ID REASON [--evidence-gap]`: `attempts++`, note, clears judgement, `accept_streak = 0`. On a story: story → `ready` **and** a ready `task` child `Fix: REASON` (inherits the acceptance). On a child: child → `ready`, its done ancestor story → `ready`. Refused if the item's wave is closed ("reopen wave").
- Judge gate on: `wave close` refuses while members are unjudged and lists them; burndown counts done+judged.
- Prompts: `next` carries effective acceptance + evidence folder; session = claim → work → write evidence → done → idle, never judge own work; dispatcher routes from `review`/`board`; coach prepares acceptance first, shows `mw judge` output to the human, **never runs `accept`**; three bullets. Rendered only for active gates. Prompt tests. Tests: reopen clears `judged`; reject on story vs child; `review` with judge off / acceptance off; `wave close` with evidence gate on/off and judge on; judge gate turned on after an `ungated` done item.
Done when: the human judges a day's work from `mw review` + `mw judge` in under a minute per item, and a fresh session reaches an accepted story with no human text beyond `accept`.

## Dogfood (no build wave)
miniwaves tracks itself in `.miniwaves.json` (gates auto); one app repo closes a feature, a bug and a release through mw; findings become the next items.

## Non-goals
No runner, server, LLM calls, scheduling, multi-user, new kinds or statuses → the rider (`wr`).

## Cost
Two waves ≈ 250k tokens incl. reviews and the 11-step protocol. Not pushed until the owner says.
