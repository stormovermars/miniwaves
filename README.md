# miniwaves (`mw`)

A tiny local work board for humans and coding assistants: plain JSON files, one Go binary, no server or account.
Assistants claim the next ready item, finish it with proof, or block it with a reason.

## Install

Fresh install (macOS, Linux; on Windows use `install.ps1`):

```sh
curl -fsSL https://raw.githubusercontent.com/stormovermars/miniwaves/main/install.sh | sh   # macOS, Linux
irm https://raw.githubusercontent.com/stormovermars/miniwaves/main/install.ps1 | iex        # Windows
```

This installs the `mw` binary and, when Claude Code (`claude`) is on PATH, the Claude Code plugins `mw` (skill, `/mw-next`, `/mw-board`) and `tidewatch` (live pane). The plugin step is best effort: if it fails, the binary is still installed.

Upgrade: `mw update` (checks GitHub releases, verifies the checksum, replaces the binary and updates both plugins; `--check` only reports).

- Upgrading from 0.3.2 or older: the old `mw` does not know the plugin step, so after `mw update` run `mw update --plugins` once (or re-run the install one-liner).
- `mw update --plugins` alone (re)installs or updates just the two plugins.
- `MW_NO_PLUGINS=1` skips the plugin step in `install.sh`, `install.ps1` and `mw update`.
- No Claude Code, or the binary only: `go install github.com/stormovermars/miniwaves@latest` (the binary is named `miniwaves`). By hand: `claude plugin marketplace add stormovermars/miniwaves && claude plugin install mw@miniwaves tidewatch@miniwaves`.

Open the pane: run `/tidewatch`, then `/tidewatch add PATH` (a folder holding an mw board).

## Use

```sh
mw init "My apps"                        # once per workspace
mw add task "Build index" -S ready       # a human adds work
mw next -j                               # assistant: take the full brief of the next ready item
mw start TSK-001 -a builder              # claim it (or: mw next --claim -j)
mw done TSK-001 "tests pass" --evidence out.png     # or: mw block TSK-001 "question"
mw board -A                              # everyone: progress across tenants
```

Static page: `mw board --html` (see USAGE.md). Storage is per tenant (`.miniwaves.json` index + `.miniwaves/<slug>.json`). Git belongs to the coding agent: commit `.miniwaves.json` and `.miniwaves/` with your code changes and resolve board conflicts in git like any file. `mw init` keeps the lock files out of git.

Runs on macOS and Linux, and natively on Windows 10/11 (amd64, arm64; PowerShell, cmd or Git Bash), all three tested in CI. Windows differences: [USAGE.md#windows](USAGE.md#windows).

## Optional features

All off until you turn them on. Details in [USAGE.md](USAGE.md).

| Feature | On | Off |
|---|---|---|
| Coordinated job (groups tenants) | `mw jobs enable ID -t SLUG,SLUG` | `mw jobs disable ID` |
| Job description | `mw jobs set ID description="One line"` | `mw jobs set ID description=` |
| News bulletin | `mw news --job ID` (`--format md\|html\|text\|json`) | don't run it |
| News releases | `mw news --job ID --release [--force]` (numbered N.json/N.md/N.html in `news_releases`, compared with the last release; empty ones refused) | don't cut one |
| News model | `mw news --job ID --format json [--tenant SLUG]` (version 1: now, happened, next, metrics, backlog, sprint, tenants) | don't run it |
| Tracker tickets (optional) | `mw jobs set ID tracker=jira tracker_url=URL`, `mw add ... --ticket KEY` | unset the key |
| Job metrics | `mw jobs set ID metrics=PATH` (a `metrics.json` your tooling writes) | unset the key |
| Coordinator kit (plan, news desk, build lock) | `mw init --kit coordinated --job ID` | delete the files it wrote |
| Fanfare, per board (tidewatch: the `Fanfare:` button) | `mw fanfare set enabled=true` | `mw fanfare set enabled=false` |
| Fanfare, just for you | `mw user set fanfare.enabled=true` | `mw user set fanfare.enabled=false` |
| Personal settings (pane, alerts, fanfare, chat bar, discovery) | `mw configure` (or `mw configure KEY=VALUE...`) | `mw user set KEY=` resets a key |
| Fanfare set (tidewatch: the `Fanfare:` button cycles off, all, each set; or `/tidewatch fanfare next`) | `mw fanfare use NAME` | `mw fanfare use all` |
| tidewatch pane (Claude Code) | installed by `install.sh`, `install.ps1` and `mw update` when Claude Code is present; or `mw update --plugins` (`MW_NO_PLUGINS=1` skips) | `claude plugin uninstall tidewatch` |

To share tidewatch with your team, install it, then ask Claude Code: "publish the tidewatch plugin to my organization". It shows every file and asks before sending; an admin may review it first. Colleagues then install it from the organization library.

## Links

[USAGE.md](USAGE.md) (commands, prompts, gates, waves) · [TECHNICAL.md](TECHNICAL.md) · [MIGRATION.md](MIGRATION.md) · runner: [wr](https://github.com/stormovermars/wr)

MIT
