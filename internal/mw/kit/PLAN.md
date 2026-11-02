# @@TITLE@@: plan

Date @@DATE@@ (rev 1). Owner: <name>. Board: mw in this folder (job `@@JOB@@`). Tenants: @@TENANTS@@.

## Goal
<one or two sentences: what is proven or delivered when this job is finished>

## Tracking decision
mw is the board for this job. If a repo's own instructions name another tracker, this job overrides them for this work only; say so in every session brief.

## Tenants and waves
| Tenant | Holds | Waves |
|---|---|---|
| <slug> | <what it holds> | <wave names> |

Each wave keeps mw's delivery checklist (n/a with a reason where it does not apply). Closing a wave plays the milestone fanfare. Wave close evidence: a short wave report, kept under the evidence folder (`mw dirs`), before `mw wave close`.

## Repos and limits
| Repo (base branch) | Role | Test command | Known limits |
|---|---|---|---|
| <path> (<branch>) | <role> | <command> | <limits> |

## Milestones
<one section per wave: scope, task ids, acceptance>

## Session protocol
- The dispatcher starts one session per ready task. The worker runs `mw start <ID> -t <tenant> -s <name>`, works on its own branch (in its own checkout when `mw jobs show @@JOB@@` says worktrees on), commits locally, then signals `mw note <ID> "ready for review"` and stays doing. It never marks done.
- The dispatcher reviews the diff, merges to the base branch, re-runs the tests (again after every product merge), then runs `mw done <ID> "<verification>"` and removes the extra checkout.
- Concurrency: at most `max_workers` working sessions (`mw jobs show @@JOB@@`) and at most one writing session per repo. Run every heavy build or test through `./buildlock.sh <command>` (Windows: `./buildlock.ps1 <command>`).
- Questions only a human can answer: `mw ask <ID> <question>`. Decisions: `mw decide <text> --by <name>`.

## Hard rules for every session
- No push unless the plan says so. Never switch branches in a shared checkout. Never commit builds, evidence or scratch files.
- <add project rules here: tools to use, folders, forbidden actions>
