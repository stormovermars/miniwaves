---
name: grill
description: Use when a plan, design or decision needs settling with the user in rounds, or the user says "grill me": ask the open design questions as board items, each with a recommended answer, so one "ok" settles a round and decisions stay findable.
---

# Grill, on the mw board

Settle a design with the user in rounds. The questions and the answers live on the board, so a later session looks up what was decided instead of asking again.

## Shape
- Think of the design as a tree: every decision has the decisions that hang off it. The **frontier** is every decision whose prerequisites are settled. A round asks the whole frontier at once, never a question that depends on another open one.
- Facts are yours to find (code, files, tools): hand a fact question to a delegate, or look it up, and cite the result. Only decisions go to the user.
- Ask only what matters. Only a cheap, reversible choice is yours: `mw decide "TEXT" -b "reversible default" -s NAME` (NAME is your session). List every one in the final summary; the user can overturn any of them. Never pass `--by` for the user: `--by` is only for a human typing it.
- Every question carries your recommended answer, worded so that "yes" accepts it. Write questions and recommendations in the language the user writes in, at most 2 lines each.
- Without a board above the working directory, run the same rounds in the chat only.

## One round
1. Look first: `mw log -A --action decide --grep TOPIC`. Drop every question a decision already answers.
2. Add the round: `mw add spike "Grill: TOPIC round N" -S ready`. Leave it unclaimed; never `mw start` it.
3. Ask once: `mw ask ID "Grill TOPIC RN: K questions, recs inside" "1) question? rec: answer" "2) ..."`. At most 4 numbered questions. The text starts by naming the round, because the needs list cuts it short.
4. Go on with other work. Wait (`mw block ITEM "waits for the round" --on ID`) only when your own item cannot move without the answer. A later round that depends on this one: `mw block LATER "..." --on ID`.
5. The answer is the user's, or the registered answerer session's (see below). Never run `mw answer`/`--accept` unless you are the registered answerer of the asking session; never pass `--by` for the user.
   - "ok" / yes: the user runs `mw answer ID --accept --by NAME`. That records one decide quoting the whole ask, then closes the round.
   - A partial answer: record one `mw decide "Q: answer" -p ID -s NAME` per question as the user said it, then the user answers with `mw answer`.
   - Sir can overturn any decision later: `mw decide "..." -p ID --by NAME`, or reopen the item.
6. `mw done ID "settled" --no-evidence "decision round"`. This frees whatever was blocked on it. Ask the next frontier as the next round.

## When several sessions work together
- One voice to the user: in a coordinated job only the coordinator grills. A worker never calls `mw ask` for design questions; it writes `mw note ID "Q: ... (rec: ...)"`, takes other ready work, or blocks on the round when it truly cannot go on.
- The coordinator collects those notes, drops what `mw log -A --action decide --grep` already answers, and batches the rest into the next round.
- After a round, copy each decision to the item that asked it: `mw decide "TEXT" -p ITEM -s NAME`. That is what reaches the worker's next brief.
- Good places to grill: the milestones and gates of a job plan before work starts; a story that has no acceptance (the recommended answer is the proposed acceptance; only the user sets it, with `mw edit ID -c`); a wave plan, before `mw wave create`.

## Answerer session
- A coordinator or parent starts its workers with `mw session start NAME --answerer <its own session>`. Their asks then wait for it, not for the user. A worker never answers asks.
- The answerer answers with `mw answer ID TEXT -s PARENT` (or `--accept -s PARENT`); the answer is marked `ai:`. It decides technical and reversible questions itself.
- It escalates to the user with its own `mw ask ID "..." -s PARENT` on the same item when a decision is irreversible, external (release, push, money, other people) or uncertain. That ask counts for the user.
- It ends a round with a summary of what it decided for the user, who can overturn any of it.

## Done
The frontier is empty and nothing is silently assumed. End with one paragraph that lists every decision, the ones you took yourself included. Do not act on the design until the user confirms.
