---
description: Explain the mw chat-bar line with live numbers, list what needs you, show settings and commands
---

Explain the miniwaves status line. Read-only: change nothing.

1. **Version line first.** Run `mw version` (e.g. `mw 0.4.0 (schema 9)`). Find the installed plugin versions in the `.claude-plugin/plugin.json` of this plugin (`${CLAUDE_PLUGIN_ROOT}`) and of its siblings `${CLAUDE_PLUGIN_ROOT}/../mw` and `${CLAUDE_PLUGIN_ROOT}/../tidewatch` (skip any that cannot be found). Print one line: `mw 0.4.0 (schema 9) · plugins mw 0.4.0, mw-jobs 0.4.0`. If a plugin version differs from the binary's, add: "run mw update --plugins".
2. Run `mw status --line` (in the current folder's board; no board prints nothing, then say so and stop after the version line). Strip any escape sequences from the text. Explain each part with the live numbers, one line each, only for parts that are present:
   - `mw:` is the mw board.
   - N is needs you: items waiting on Sir's answer or action.
   - R is review: finished items waiting for their review.
   - B is blocked: items stuck on something other than Sir.
   - O/A is open items (not done) over all items (done included).
   A letter that is missing means its count is zero.
3. If N is above zero, list those items with id and title (`mw board`, or `mw list` and pick the ones that need Sir; read-only commands only).
4. **Settings.** Run `mw user get -j` and show a short block: for each key its value, the one-line help (`mw help user` explains the keys; `mw configure --show` lists them) and how to change it: `mw user set KEY=VALUE` (an empty VALUE resets it) or `mw configure` in a terminal. Cover the fanfare keys (on/off, the sound set, and the other fanfare settings from `mw fanfare -j`: milestone and finale sounds, Stop dialog, volume), the status bar (`status.show`), discovery (`discover.*`) and any other user key that is listed. State that since 0.4 alerts are only the fanfare sounds plus the Stop dialog: no popups, no OS notifications, and the old `pane.*` and `alerts.*` keys are gone.
5. **Commands.** List every slash command of the installed plugins, one line each with its purpose: read the `description:` of the frontmatter of each `${CLAUDE_PLUGIN_ROOT}/../*/commands/*.md` (this includes /mw-status, /mw-jobs, /mw-news, /mw-publish, /mw-fanfare, /mw-board, /mw-next and /tidewatch when installed). Do not use a hard-coded list; the command name is the file name without `.md`.

Keep it short. Reply with the blocks only.
