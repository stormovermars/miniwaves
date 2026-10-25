---
description: Regenerate the mw news draft (status.md) and show it in the file pane
argument-hint: "[job id]"
---

Show the miniwaves news view. Argument (optional): a job id; `$ARGUMENTS`.

1. Run `mw news` (add `--job ID` when an id was given) in the current folder's board. Without an id it covers every job. If there is no board here, run `mw boards --up -j` and use the nearest one, or say there is none.
2. Each line of the output is `wrote PATH` or `unchanged PATH`; PATH is absolute. Collect the paths.
3. In the Claude desktop app, show each file in the file pane by replying with a Markdown link per file, e.g. [status.md](</abs/path/status.md>). In terminal-only Claude Code, just print the absolute path.
