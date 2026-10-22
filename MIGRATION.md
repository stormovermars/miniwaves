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

## Schema 9 (0.3.0)

Schema 9 changes no file layout: `mw migrate` rewrites the index with `"version":9` (backup `.miniwaves.json.bak8`) and nothing else. It exists because 0.3.0 adds index keys (`jobs`, a tenant's `job`, fanfare `sets` and `use`) that a 0.2.8 or older mw drops when it saves the index. Job keys written by 0.2.9-dev builds with dotted names (`news.out`, `news.url`, `pane.style`) are read as before and rewritten as `news_out`, `news_url`, `pane_style` (idempotent; unknown job keys are kept). A 0.2.8 mw refuses a schema 9 board ("workspace schema 9 is newer than this mw supports: upgrade mw") instead of rewriting it, and 0.3.0 refuses a schema 8 board until you run `mw migrate`. Upgrade every machine and CI image to 0.3.0 before migrating the committed board.

## One mw version per workspace

Tenant tags and the archived flag live only in the index (`.miniwaves.json`) and the schema stays 8. If mw 0.2.0 or older saves the index of a 0.2.1 board, it silently drops every tag and un-archives every tenant. Keep a single mw binary version per workspace (check `which -a mw`).

## Removed or renamed since 0.1.0

- `mw tenant use` is gone: use `mw tenant default SLUG` (folder default), a tenant path, `MW_TENANT` or `-t`.
- Reparenting is `mw edit ID -p PARENT|-`.
- Flag `-n` means `--limit`.
- A session is always optional: `mw start ID` without a session uses `local`.
- mw has no git commands: commit the board with your code.
