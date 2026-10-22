# News desk: human-facing documents for @@TITLE@@

Owner: the news desk session. Reader: the human who owns the job. Rev 1, @@DATE@@.

## Pages (one page or artifact, tabs)

| Page | Content | Source | Made by |
|---|---|---|---|
| Status | Headline; needs-you at the top; milestone bars; numbers; last 10 events | `mw news --job @@JOB@@`, `mw review` | mw |
| Plan | Goal, scope, milestones and what each means, task table (id, title, owner, state) | PLAN.md + mw | script + short intro |
| Changelog | Dated entries, newest first: merges, closed milestones, rejections, new tasks, failures | `mw log -A --since`, git logs of the repos in PLAN.md | script |
| Decisions | D-NNN: date, decision, decided by, why, alternatives dropped, link. Append-only | `mw decide` events, PLAN.md | desk, by hand |
| Product guide | What each product is, formats, known limits; a glossary of every jargon word | PLAN.md | desk, by hand |
| How-to | Task recipes that are verified; a "doesn't work yet" list | results | desk |

## Rules
- Sources live in `docs/human/*.md` (committed); the page is built from them. The desk commits only `docs/human/` and its own helper scripts, nothing else.
- Facts come from the board (`mw news`, `mw review`, `mw log`). `mw news` writes only when the facts changed.
- Update triggers:
  - **On event:** the dispatcher sends the desk a one-line message (merge, task done or rejected, milestone closed, new tasks, needs-you item). The desk updates at once and records any decision with a named decider.
  - **Periodic:** a tick as a safety net; it does nothing when `mw news` reports no change.
  - **On request:** the human asks ("what changed?"); regenerate and answer.
- Hand-written text: short and plain; no jargon without a glossary entry. Never claim something works unless the results say so.
- Decisions: record one only with a named decider and a source (message, item id, commit).
- Report every few hours and on milestone close or a new needs-you item: 5 to 8 lines plus the link.

## Alerts
- Fire only when a milestone (wave) closes or a new needs-you item appears, once per event. Keep the event ids in the desk's scratch notes and skip ids already listed.
- Channels are the human's choice: sound is mw's own fanfare, the rest is the chat report, plus a push or email only if the human set one up. No alert for routine progress.
