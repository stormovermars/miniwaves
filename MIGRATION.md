# Migrating a workspace: 0.1.0 to 0.2.1 (schema 8), 0.2.x to 0.3.0 (schema 9)

mw 0.2.1 has no compatibility code: it reads schema 8 only. Boards written by 0.1.0 (and any older schema) are refused with

```
mw: <path>/.miniwaves.json is schema 3 and this mw only reads schema 8: run mw migrate (see MIGRATION.md)
```

Run `mw migrate` once per workspace:

```
mw migrate --dry-run [PATH]   # print what would change, write nothing
mw migrate [PATH]             # upgrade in place; backup at <file>.bak<old schema>
```

PATH defaults to the `.miniwaves.json` found upward from the current folder. `mw migrate` is the only command that reads old schemas, and it is idempotent. A 0.1.0 board is backed up as `.miniwaves.json.bak<old schema>` (schema 7 boards: `.miniwaves.json.bak7`).

Schema 8 stores each tenant in its own file: `.miniwaves.json` is the index, items, counters, gate state and sessions are in `.miniwaves/<slug>.json`, the archive in `.miniwaves/<slug>.archive.json`. Commit `.miniwaves.json` and `.miniwaves/`, and add `.miniwaves/*.lock*` and `.miniwaves/*.tmp` to `.git/info/exclude` (`mw init` does it for new boards). An old mw cannot read schema 8.

## 0.4: panes replaced by md files (mw-jobs)

No schema change, no migration step. The mw-jobs panes (Jobs, News, settings, image pane, alert rows, toasts, chat-bar status, `mw_panes_open`) are gone. New `mw board --md` writes `jobs.md` next to the news draft; `/mw-jobs` and `/mw-news` print the file paths and the Claude desktop app shows them in its file pane; `/mw-publish` publishes them as a claude.ai artifact. `/mw-watch` and `/mw-configure` are removed (use `mw configure` in a terminal). Hooks refresh the files quietly; they send no notifications. The `pane.*`, `alerts.*` and `chat bar` user keys stay readable but nothing in the plugin uses them.

## 0.3.7

No schema change, no migration step. The finale Stop dialog removed in 0.3.6 is back, with its icon and the `stop_dialog` key (default true, no deprecation note). `mw fanfare stop`, the mw-jobs Stop button and the `playing` / `playing_finale` keys of `mw fanfare -j` stay.

## 0.3.6

No schema change, no migration step. The finale Stop dialog (macOS `osascript`, Windows popup, `zenity`, `kdialog`) and its built-in icon file `.mw-fanfare-icon.png` are removed: mw opens no dialog or notification of its own. The `stop_dialog` fanfare key is still read, so old boards parse, but it is ignored; `mw fanfare set stop_dialog=...` prints a deprecation note. Stop a playing fanfare with `mw fanfare stop` or the mw-jobs Stop button (shown while a finale plays). `mw fanfare -j` gains `playing` and `playing_finale`. A leftover `<dir>/.mw-fanfare-icon.png` is harmless and can be deleted.

## Schema 9 (0.3.0)

Schema 9 changes no file layout: `mw migrate` rewrites the index with `"version":9` (backup `.miniwaves.json.bak8`) and nothing else. It exists because 0.3.0 adds index keys (`jobs`, a tenant's `job`, fanfare `sets` and `use`) that a 0.2.8 or older mw drops when it saves the index. Job keys written by 0.2.9-dev builds with dotted names (`news.out`, `news.url`, `pane.style`) are read as before and rewritten as `news_out`, `news_url`, `pane_style` (idempotent; unknown job keys are kept). A 0.2.8 mw refuses a schema 9 board ("workspace schema 9 is newer than this mw supports: upgrade mw") instead of rewriting it, and 0.3.0 refuses a schema 8 board until you run `mw migrate`. Upgrade every machine and CI image to 0.3.0 before migrating the committed board.

## Changes in the 0.3.0 line to know about

- `mw migrate` 8 to 9 writes only the index; tenant files are not touched.
- `mw log --since`: the `--lookback` option is removed. Instead the cursor carries a fixed 24 hour window (there is no `--window` flag): events stamped up to 24h before the cursor time are still delivered once. Cursor keys are now 8-byte hashes; old cursors whose `seen` holds full `tenant/item/N` keys keep working and match exactly (no duplicates). New: `--since-file PATH`.
- `mw reap` prints `spared ...` instead of `nothing to reap` when only dispatcher or news-desk sessions are due.
- News pages: the layout changed completely (a 70-column bulletin drawn from `mw news --format json`); see "News bulletin and releases" below.

## `mw review -j` (0.3.0)

`mw review -j` adds a `review` list (always present, `[]` when empty): the blocked items whose block reason starts with "awaiting review", the same rule `mw news` uses. These items also stay in `blocked`, so scripts written for 0.2.7 that count `blocked` see the same numbers; a script that wants only real blocks subtracts `review`. The plain text output gains a `review` section when there are any.

## One mw version per workspace

Tenant tags and the archived flag live only in the index (`.miniwaves.json`) and the schema stays 8. If mw 0.2.0 or older saves the index of a 0.2.1 board, it silently drops every tag and un-archives every tenant. Keep a single mw binary version per workspace (check `which -a mw`).

## Removed or renamed since 0.1.0

- `mw tenant use` is gone: use `mw tenant default SLUG` (folder default), a tenant path, `MW_TENANT` or `-t`.
- Reparenting is `mw edit ID -p PARENT|-`.
- Flag `-n` means `--limit`.
- A session is always optional: `mw start ID` without a session uses `local`.
- mw has no git commands: commit the board with your code.

## News bulletin and releases (0.3.0, schema 9, no migration step)

`mw news` changed its output. The Markdown, HTML and text pages are now one gopher-style, 70-column bulletin drawn from a single model, so the old layout (the `Headline`, `Needs you`, `Last events` sections, the milestone table, the `Updated` line, the "moved since the last run" count and the 80-column `══`/`──` text) is gone; anything that parsed those pages must read `mw news --job ID --format json` instead (model version 1). `--width` still works for `--format text` and is ignored; it is refused for md and html. `status.md` and `status.html` are rewritten once on the first run; after that they change only when the board does (the page has no generated-at line any more).

New optional job keys, all in schema 9 (an older mw drops unknown index keys when it saves, so use one mw version per workspace): `news_releases`, `metrics`, `tracker`, `tracker_url`. New optional item field `ticket` (omitted when empty); mw now also keeps item fields it does not know when it saves a tenant file. Boards that set no tracker and no tickets produce the same `list`, `show`, `board` and `jobs show` output as before apart from the two new job keys in `jobs show`. `mw news --release` creates `docs/news/releases/` (or `news_releases`) next to the page; commit it if you want the history.

## Upgrading to 0.3.4 (no schema change, no migration step)

- **Run `mw update --plugins` once after `mw update`** when you come from 0.3.2 or 0.3.3. The old binary updates its old plugin list (`mw`, `tidewatch`), so it cannot install `mw-jobs` or remove the old pane plugins. Re-running the install one-liner does the same.
- The pane plugins `tidewatch` and `mw-watch` are retired; `mw-jobs` is the only one. `install.sh`, `install.ps1` and `mw update` install `mw` and `mw-jobs` first and uninstall `tidewatch` and `mw-watch` only after that succeeded. A stub `tidewatch` plugin stays for one release and only tells users to run `mw update --plugins`.
- Personal settings live in `mw-user.json` under `os.UserConfigDir()/miniwaves/` (`MW_USER_CONFIG` overrides the path), never on a board. `mw user get [KEY] [-j]` lists every key with its value, default and source (`default`, `user`, or `board`); `mw configure` edits them. The file is written under `mw-user.json.lock`, so concurrent `mw user set` and `mw configure` lose nothing. Keys: `pane.refresh`, `pane.style`, `pane.zoom`, `pane.fit`, `pane.open`, `alerts.enabled`, `alerts.kinds`, `alerts.message`, `alerts.sound`, `fanfare.enabled`, `fanfare.use`, `status.show`, `status.detail`, `discover.home`, `discover.depth` (plus `alerts.on` and `alerts.exec`); see USAGE.md.
- Install and update run `mw user fanfare-defaults`: `fanfare.enabled=true` and `fanfare.use=all` are set only where unset (`MW_NO_FANFARE=1` or `MW_NO_PLUGINS=1` skips). A board that explicitly runs `mw fanfare set enabled=false` still wins over the user layer.
- New read-only `mw boards [DIR] [--depth N] [--up] [-j]` replaces any hand-rolled board search; mw-jobs uses it.
- `mw news --full` is new and changes no existing output; in the pane a news release is called an issue (the CLI keeps `--release`).

## Upgrading to 0.3.5 (no schema change, no migration step)

- New optional job keys `ceremonies` and `ready_min`; a job that sets neither prints as before.
- `mw jobs show -j` has a new `news_path` field.
- `mw log --follow` is new (an alerts runner for `alerts.exec`); nothing runs unless you pass `--exec`.
