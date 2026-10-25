---
description: Regenerate the mw jobs view (jobs.md) and show it in the file pane
argument-hint: "[job id]"
---

Show the miniwaves jobs view. Argument (optional): a job id; `$ARGUMENTS`.

1. Run `mw board --md` (add `--job ID` when an id was given) in the current folder's board. Without an id it covers every job of that board. If there is no board here, run `mw boards --up -j` and use the nearest one, or say there is none.
2. Each line of the output is `wrote PATH` or `unchanged PATH`; PATH is absolute. Collect the paths.
3. In the Claude desktop app, show each file to the user in the file pane by replying with a Markdown link per file, e.g. [jobs.md](</abs/path/jobs.md>) (the app renders .md and reloads it when it changes). In terminal-only Claude Code, just print the absolute path.
4. Add one line: the status line at the top of the file (the third line of jobs.md). Do not paste the whole file.
