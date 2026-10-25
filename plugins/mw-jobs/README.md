# mw-jobs

Your miniwaves (mw) jobs as Markdown files. No panes: `mw board --md` renders every job of the board to `jobs.md`, `mw news` writes the news draft `status.md`, and the Claude desktop app shows both in its file pane (it renders `.md` and reloads on change). One command can also publish them as a private claude.ai artifact.

## What jobs.md shows

A status line over every job (`! tiny 2 need you · C2 5/6 │ demo ▸ D1 1/2`), then per job: headline, **Needs you** (`!`), **Review** (`◐`), **In progress** (`●`, or `⏱ stalled` after 45 minutes), **Ready** (queue, 10 + `… N more`), **Recently done** (`✓`, newest first, 10), **Milestones** (`done/total` with a text bar; closed ones fold into `✓ N closed`), **Harness** (only with a metrics file), **Last events** (`HH:MM` for today, `Mon DD HH:MM` for older), and a footer (`updated HH:MM`, link to the news file, link to the published page). Empty sections are left out. Three meanings, always with their symbol: `!` needs you, `◐` `⏱` waiting, `✓` `●` progress.

Files live next to the job's news draft (`<news.out>/jobs.md` and `status.md`; `docs/news/<job>/` by default, `docs/news/` with no job). They are rewritten only when their content changed.

## Commands

- `/mw-jobs [ID]` regenerates `jobs.md` and shows it in the file pane (terminal: prints the path).
- `/mw-news [ID]` regenerates the draft tabloid edition (what happened since the last published one, proof, what is coming) and shows it.
- `/mw-publish ID` publishes the tabloid as a private artifact (local proof images uploaded as artifact assets), then cuts the edition (`mw news --release`) so the next draft starts from it; the URL is kept in `jobs.artifact-url` next to `jobs.md`, so later runs update the same artifact.
- `/mw-status` runs `mw status --explain`: the chat-bar line with live numbers, what needs you, your settings, versions and every installed command.
- `/mw-fanfare [ARGS]` runs `mw fanfare ARGS` (settings, `set`, `use`, `sets`, `test`, `stop`).

## Hooks

One PostToolUse hook on Bash: after an `mw` verb that writes the board (add, ready, start, done, block, unblock, ask, answer, accept, reject, wave, note, edit) it quietly runs `mw board --md` in the hook's `cwd` (`hooks/refresh.sh`). Nothing runs at session start, the news file is left to `/mw-news`, and it never prints or fails. No notifications, no toasts.

## Status line

`mw status --line --link` prints `mw:N2R1B1:12/40` for the Claude Code status bar: N needs you (an open ask, or blocked with no `--on`), R awaiting review, B blocked on another item (`mw block ID REASON --on OTHER`), each hidden at zero and no item counted twice, then open/all items (inbox drafts are in neither number). Nothing is printed outside a board or with `mw user set status.show=false`. To switch it on, add this to `~/.claude/settings.json` (the plugin never edits it):

```json
"statusLine":{"type":"command","command":"mw status --line --link"}
```

`--link` makes the whole line an OSC 8 hyperlink: a click opens the published artifact (the URL in `jobs.artifact-url`) or else `jobs.md` (Cmd+click on macOS, in terminals that support OSC 8). Without `--link` the line is plain. `mw status --explain` (or `/mw-status`) explains it with live numbers.

Settings live in the CLI: `mw configure`. Requires the `mw` binary (the `mw` plugin installs it).
