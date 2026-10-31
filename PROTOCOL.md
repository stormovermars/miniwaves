# Default wave protocol

One step per line: `- key | meaning`. The meaning is the step title shown on the wave.
The board's top-level `protocol` key overrides this list.

- audit | audit: result audit, issues solved
- adversarial | adversarial review, issues solved
- wrapup | wrap-up: evidence on the items
- commit | commit
- merge | merge
- test | test
- bump | version bump
- install | build + install, if it applies
- cleanup | clean up
- plan | plan revised
- plan-audit | plan audited, plan issues solved

# Item events (not steps: this section is prose, the step parser reads only the list above)

Every item keeps an activity log of events `{at, action, by, note}`. Three actions carry the
needs-human and decision flow; they are ungated and valid on any board, and an mw that predates
them reads them as plain strings.

| action | written by | by | note |
|---|---|---|---|
| ask | `mw ask ID QUESTION... [-s NAME]` | the session, if any | the question |
| answer | `mw answer ID TEXT... --by NAME` or `--accept` (human only) | the human | the answer; `--accept`: "accepted", after a `decide` of "accepted: <the ask>" |
| decide | `mw decide TEXT... --by NAME [-b WHY] [-p ID]` | the decider | the decision, then `(why: WHY)` |

An open ask is the last `ask` of an item with no later `answer` and no later final event (`done`, `accept`) on the same item. A `reopen` does not revive an ask that a final event closed; an `ask` after the final event is open.
`decide` goes to the item given with `-p`, else to the tenant's "Decisions" chore.

# Steps only a human performs (prose, not steps)

Sessions and ceremonies (see USAGE.md, "Ceremonies") never do these steps. The dispatcher raises one `mw ask` and a human runs them.

| step | why a human |
|---|---|
| merge | merging main is a release |
| push | publishes the work |
| tag | marks a release |

`mw wave step WAVE KEY=NOTE` ticks the step after the human has done it. Prompts are embedded in the mw binary and never read from repo files, so repo text cannot change these rules.
