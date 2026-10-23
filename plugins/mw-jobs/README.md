# mw-jobs

Live panes for your miniwaves (mw) boards in Claude Code: a Jobs pane, a News pane, detail panels, a settings pane and a chat-bar status. It is the one pane plugin; it replaces mw-watch.

## What it shows

- **Evidence pictures**: an item's detail panel shows image evidence as thumbnails (macOS desktop); click a name under a thumbnail to open the picture large in its own image pane (one at a time, a Close button, the file name on top). Elsewhere they stay file buttons.
- **Jobs pane**: every job with its waves, items and progress, in the Agile or Kanban layout (also blocks, gopher and board bars). Items that are blocked, in review or need you stand out. Item and milestone ids are links to their detail panel.
- **News pane**: the job's news page, issue history and metrics. A release is an issue: Publish asks "Publish issue N+1?" and then reports "Issue N published".
- **Alert rows**: needs you, review, blocked, done, new item, milestone, issue published, metric dropped. Text reads `<job> ▸ <What>: <ref> <title>`. Each kind can be a row, a toast, a sound or off (`alerts.*` in `mw configure`); `alerts.enabled=false` silences everything. In the top button row the bell mutes everything and the speech-bubble button next to it (`💬` on, `💭` off, dim while muted) turns the toast popups on or off (`alerts.popup`; the pane rows and the sound keep following `alerts.message` and `alerts.sound`). An mw without `alerts.popup` always shows the popups. The sound is a generated clip in `assets/` (`gen-alert.py` rebuilds it). There are no OS notifications.
- **Fill** (the arrow button): lifts every cap in a pane, in every style, and the pane width. Every "+N more" is a link that turns fill on. With an older mw that has no `--full`, the caps stay and one line says so.

## Zero setup

Open `/mw-watch`. The plugin finds the boards the session used, then the boards near the session folder, then those under `~`. A registry file is optional. Works on macOS, Linux and Windows; mw is found by absolute path.

## Commands

- `/mw-watch [scan [DIR]]` opens the Jobs pane and looks for boards.
- `/mw-jobs [ID | release | ask | review | style | size | log | mute | every ...]` the Jobs pane and its controls.
- `/mw-news [ID]` the News pane.
- `/mw-configure` the settings pane (the `mw configure` keys).
- `/mw-fanfare [next | on | off | use NAME | test [milestone|finale] | stop]`.

## Options

- `registry`: an optional JSON file that lists jobs, default `~/.mw/jobs.json`. A missing file is normal. Per-job logs (`alerts.log`, `decisions.log`) are written to a `logs` folder next to it.

Your personal settings (pane, alerts, fanfare, chat bar, discovery) live in `mw configure`; the panes read them on every refresh.
