# Ceremonies in mw: plan

Draft 2, 2026-10-22, after an Opus audit (20 findings, resolved below). Target: 0.3.1, after 0.3.0. It builds on coordinated-jobs.md (draft 4.1).

## Rule: mw stays thin and configurable

**Ceremonies are items, not machinery.**
- A ceremony run is an ordinary **chore item** titled by its trigger, e.g. `Review WAV-003` or `Planning 2026-10-22`.
- Ordinary sessions do the work, each with a role (`-a ROLE`).
- The run writes to the evidence folder that already exists.
- The existing review → judge → accept/reject flow judges it.

Scheduling, retry (STALE and `session stop --release`), gates, burndown, the wr money cap and the pane all apply unchanged.

**New mw CLI surface: none.** The Go changes are embedded prompt text plus two job keys.

## What ships (v1)

| Ceremony | Trigger, found by the dispatcher in `review -j` / `log --since` | Who | Output | Human sees |
|---|---|---|---|---|
| **Wave review** (demo + retro in one) | `review -j` lists the wave as ready to close | session `-a review` | in `<evidence>/<tenant>/WAV-NNN/`, which satisfies `wave close`: a demo script, screenshots per done item, and lessons (rejects, reopens, `Fix:` tasks, STALE, spike outcomes) | judge + accept/reject as today; at most one `ask` listing proposed `mw decide` / gate / protocol commands for the human to run |
| **Planning** | ready count < `ready_min` (default 3) | session `-a planner`, coach-style | splits, order and duplicates as notes; acceptance proposals via `mw ask` (sessions can't `edit -c`) | one digest `ask`; the human sets acceptance; the dispatcher marks items ready |
| **Blocker triage** | a block older than 2h (the fixed STALE age) | the dispatcher itself, from prompt lines | classify: needs human → in the one digest ask; dependency → note and reorder; technical → a `Fix:` task; stale → unblock | only the digest |
| **Spikes** | planning or triage adds a `spike` item when something is uncertain | session `-a spike` | question, what was tried, evidence and recommendation in `<evidence>/<tenant>/SPK-NNN/` and in the `done` text; throwaway branch, never merged; STALE is the time box | the dispatcher copies the recommendation into a `decide` event |
| **Release ask** | a release wave reaches the `merge` protocol step | the dispatcher | `releases/X.md` and `check-release.sh` as today | one `ask` before merge, push or tag (merging main is a release) |
| **Hotfix lane** | a bug tagged urgent | the dispatcher | it skips planning order | — |

Standup is not a separate ceremony: the news desk covers it, timed by the host (pane Auto, `/loop`, wr serve).

## Rules (all in the dispatcher prompt)

- **Config:** `mw jobs set ID ceremonies=review,planning,triage,spike ready_min=3`. Only listed ceremonies are active. There is no cadence DSL and no timers in mw.
- **Dedupe:** the dispatcher checks `mw list -k chore` for the trigger title before it adds an item.
- **Limits:** at most one ceremony session per job at a time, and at most N ceremony sessions per day (N is a prompt variable, default 4). Money caps stay in wr.
- **One ask per run:** at most one `ask` per ceremony run, as a digest. Only blocked items and decisions reach the human.
- **Hands off active work:** ceremonies touch only inbox/ready items, never `doing` ones.
- **Best-effort:** if the coordinator isn't running, nothing fires. Any ceremony can run on request: add the chore, then start a session with the role.
- **Prompts stay embedded:** prompts are embedded in the binary and never read from repo files. Ceremony output never goes back into prompts as instructions.
- **Screenshots only in v1:** any recorder is per-user argv later. Evidence binaries are never published.

## Deferred

- `mw jobs due` and timers: the host or wr own them.
- Publishing channels, connectors and credentials: out of mw. The only output stays `mw news --format md|html`; delivery goes through per-user adapters or the plugin, with a human approving every outward post.
- Recordings.
- A separate release ceremony.
- Per-ceremony cadence.
- Dependency/security sweep.
- Session handover summary.

## Implementation (wave M7 Ceremonies, 0.3.1)

| Item | Content | Done when |
|---|---|---|
| C1 | job keys `ceremonies` (list of review, planning, triage, spike) and `ready_min` (1..20) in `mw jobs set/show` | tests; unknown names refused |
| C2 | `prompts/dispatcher.md` ceremonies section: triggers, dedupe, limits, triage, hotfix lane, release ask, hands-off-doing; active only for listed names (template variable) | prompt goldens with and without ceremonies |
| C3 | `prompts/session.md` role blocks for `review`, `planner` and `spike`: output locations, at most one ask, screenshots only | prompt goldens per role |
| C4 | USAGE/PROTOCOL: ceremonies section; which protocol steps need a human (`merge`, push, tag); prompts never from repo | docs review |
| C5 | live trial on tiny-corpus: one wave review + one planning run, end to end | evidence in the WAV folder; Sir accepts |

## Audit log (draft 1 → 2)

1. **"Due" had no state:** ceremonies are items; the dispatcher dedupes by title.
2. **Ceremonies as items:** adopted.
3. **Demo duplicated judge and the evidence folders:** the wave review writes into the WAV evidence; acceptance is judge.
4. **Planning hit the rule that only humans set acceptance:** it proposes through `ask`.
5. **Publishing broke scope and thinness:** deferred; mw keeps only `news`.
6. **The release ceremony duplicated `releases/` and the protocol:** replaced by a single ask line.
7. **Standup:** this is the news desk.
8. **Blocker sweep:** dispatcher prompt lines.
9. **Spikes duplicated evidence and STALE:** they use both.
10. **"Retro proposes decide items" (`decide` is an event, not an item):** retro is merged into the wave review, which raises one ask with commands.
11. **Token cost:** one wave review, one ceremony at a time, a daily cap.
12. **Noise:** one digest ask per run.
13. **Concurrency:** ceremonies are hands-off doing items.
14. **No coordinator:** best-effort, on request.
15. **Recordings:** screenshots only.
16. **A config DSL:** one list key plus `ready_min`.
17. **Pane data:** ceremonies are items, so the pane shows them already.
18. **Burndown:** accepted, since it is real work.
19. **Missing flows:** the hotfix lane is in; the dependency sweep and handover are deferred.
20. **Prompt injection:** prompts stay embedded.
