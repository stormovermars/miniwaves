# mw-jobs

Your miniwaves (mw) jobs as Markdown files. No panes: `mw board --md` renders every job of the board to `jobs.md`, `mw news` writes the news draft `status.md`, and the Claude desktop app shows both in its file pane (it renders `.md` and reloads on change). One command can also publish them as a private claude.ai artifact.

## What jobs.md shows

A status line over every job (`! tiny 2 need you · C2 5/6 │ demo ▸ D1 1/2`), then per job: headline, **Needs you** (`!`), **Review** (`◐`), **In progress** (`●`, or `⏱ stalled` after 45 minutes), **Milestones** (`done/total` with a text bar; closed ones fold into `✓ N closed`), **Harness** (only with a metrics file), **Last events**, and a footer (`updated HH:MM`, link to the news file, link to the published page). Empty sections are left out. Three meanings, always with their symbol: `!` needs you, `◐` `⏱` waiting, `✓` `●` progress.

Files live next to the job's news draft (`<news.out>/jobs.md` and `status.md`; `docs/news/` with no job). They are rewritten only when their content changed.

## Commands

- `/mw-jobs [ID]` regenerates `jobs.md` and shows it in the file pane (terminal: prints the path).
- `/mw-news [ID]` regenerates the news draft and shows it.
- `/mw-publish [ID]` publishes `jobs.md` + `status.md` as one private artifact `mw <job> board`; the URL is kept in `jobs.artifact-url` next to `jobs.md`, so later runs update the same artifact.
- `/mw-fanfare [next | on | off | use NAME | test [milestone|finale] | stop]` the fanfare settings (the fanfare itself lives in the mw CLI).

## Hooks

SessionStart and PostToolUse on Bash commands that run `mw ` quietly refresh `jobs.md` and `status.md` (`hooks/refresh.sh`; silent, fast, failures ignored). No notifications, no toasts.

Settings live in the CLI: `mw configure`. Requires the `mw` binary (the `mw` plugin installs it).
