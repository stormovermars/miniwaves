# mw-jobs

Live panes for your miniwaves (mw) boards in Claude Code: a Jobs pane, a News pane, detail panels, a settings pane and a chat-bar status.

## What it shows

- **Jobs pane**: every job with its waves, items and progress, in the Agile or Kanban layout (also blocks, gopher and board bars). Items that are blocked, in review or need you stand out. Click an item or a milestone for its detail panel.
- **News pane**: the job's news page, release history and metrics.
- **Alert rows**: milestone closed, blocked or needs you, item done, new item, metric dropped. Each can be a toast, a notification, both or off, with an optional sound. Rows stay on top of the panes until you dismiss them.
- **Gopher fill**: the gopher bar fills as the job progresses.

## Zero setup

Open `/mw jobs`. The plugin finds the boards the session used, then the boards near the session folder, then those under `~`. A registry file is optional.

## Commands

- `/mw jobs [ID]` opens the Jobs pane; `/mw news [ID]` the News pane.
- `/mw-configure` (or `/mw configure`) opens the settings pane.
- More: `/mw style`, `/mw size`, `/mw mute`, `/mw every`, `/mw log`, `/mw release`, `/mw ask`, `/mw review`, `/mw fanfare`; bare `/mw` lists them.

## Options

- `registry`: an optional JSON file that lists jobs, default `~/.mw/jobs.json`. A missing file is normal. Per-job logs (`alerts.log`, `decisions.log`) are written to a `logs` folder next to it.
- The alert options (`alertMilestone`, `alertNeeds`, `alertDone`, `alertNew`, `alertMetric`, `alertSound`) are plugin settings in Claude Code.

Your personal settings (pane, alerts, fanfare, chat bar, discovery) live in `mw configure`; the panes follow them.
