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

SessionStart and PostToolUse on Bash commands that run `mw ` quietly refresh `jobs.md` and `status.md` (`hooks/refresh.sh`; silent, fast, failures ignored). No notifications, no toasts.

## Status line

`scripts/statusline.sh` (a thin wrapper around `mw status --line`) prints `mw:N2R1B1:12/40` for the Claude Code status bar: N needs you, R awaiting review, B blocked (each hidden at zero), then open/all items. Nothing is printed outside a board or with `mw user set status.show=false`. To switch it on, add this to `~/.claude/settings.json` (the plugin never edits it):

```json
"statusLine": {"type": "command", "command": "<abs path to the plugin>/scripts/statusline.sh"}
```

or simply `"command": "mw status --line"`. The wrapper passes `--link`: the whole line is an OSC 8 hyperlink, so a click opens the published artifact (the URL in `jobs.artifact-url`) or else `jobs.md` (Cmd+click on macOS, in terminals that support OSC 8). Plain `mw status --line` is not linked. For the explanation in chat, use `/mw-status`.

Settings live in the CLI: `mw configure`. Requires the `mw` binary (the `mw` plugin installs it).
