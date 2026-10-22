# tidewatch

A live pane for your [miniwaves](../../README.md) (`mw`) boards: a status line, toasts, a ping when something needs a person, and a news page. Opt-in and generic: it knows nothing about your boards until you add them.

## Install and open

`install.sh`, `install.ps1` and `mw update` install or update this plugin (and `mw`) when Claude Code is present. `MW_NO_PLUGINS=1` skips that; `mw update --plugins` (re)installs them. By hand:

```
/plugin install tidewatch@miniwaves
/tidewatch
```

Then add a board with `/tidewatch add PATH`.

It needs the `mw` command; it finds it once at start (PATH, then the usual install folders) and always runs it by absolute path. If it is not found, the pane says so.

## Add a board

```
/tidewatch add PATH      # a folder holding an mw board; the id is the folder name
/tidewatch remove ID
/tidewatch ID            # pick a job; /tidewatch alone opens the pane
/tidewatch news [ID]     # the news pane (the News button does the same)
```

Boards are stored with absolute paths in the plugin's store, with their tenants from `mw list -A -j`. Other commands: `fanfare next`, `mute`, `every off|30s|1m|5m`, `bigger|smaller|reset`, `stalled MINUTES`.

The top button row has one `Fanfare: off|all|<set>` button; each press cycles off, all, then each fanfare set, then off again (like the `Auto:` timer button). `/tidewatch fanfare next` does the same. The footer keeps only Test.

The `mw news` pane renders `docs/news/status.md` of the board (or the job's `newsFile`) as Markdown. It only reads the file, cuts it at 9,500 characters, and shows a how-to when the file is missing.

## Styles

A style is a JSON file of tokens, never code. `normal` (sans, rounded bars) and `blocks` (mono, `█░`) are built in; see `styles/normal.json` and `styles/blocks.json`.

```
/tidewatch style list
/tidewatch style NAME                # choose
/tidewatch style new NAME            # store a copy of normal under NAME
/tidewatch style export NAME PATH    # write it to a file to edit
/tidewatch style import PATH         # read PATH (named NAME.json) into the store
```

Names are `[a-z0-9-]`, 1 to 32. A user style is merged over `normal`, so it may hold only what it changes. Unknown keys and bad values are ignored and reported once in a toast; nothing throws. The terminal draws cell bars from `bar` and `colors` (font and sizes do not apply there; it uses the dark colors, and only the ones you changed).

| Token | Meaning |
|---|---|
| `font` | `sans` or `mono`. Picks the drawing: sans is a three-column grid, mono is one monospace grid. |
| `sizes` | `headline`, `body`, `note`, `label` in px, 6 to 40. Mono uses `body` only. |
| `rowHeight` | Row height in px, 8 to 60. |
| `columns` | `name` column (px for sans, characters for mono) and `count` (minimum width of the right column). The bar column flexes. |
| `bar` | `{"kind":"rect","height":6,"radius":3}` or `{"kind":"glyphs","fill":"█","rest":"░"}`. A glyph is one character, not one of `<>&"'`. |
| `colors` | `fg`, `dim`, `track`, `needs`, `waiting`, `progress`, `new`, `pass`, `fail`; each `{"light":"#rrggbb","dark":"#rrggbb"}` (`#` and 3 to 8 hex digits). `new` marks dots and the since-chip; `pass` is also the fill of a milestone with news. |
| `sections` | `order` (list of `needs`, `review`, `progress`, `milestones`, `harness`, `events`; unlisted ones follow) and `show` (id to true/false). |
| `scale` | Three factors for `−`, `#`, `+` (0.5 to 2). |

Numbers are clamped, other bad values dropped. Mono draws sizes as whole pixels, so `blocks` uses `[0.92, 1, 1.17]` to get 11, 12 and 14.

## Alerts

New needs-you and blocked items ping (`assets/Ping.aiff`) and notify once per event; a closed milestone only toasts and notifies, since mw's fanfare already plays for it. `mute` silences both.
