# Migrating a workspace: 0.1.0 to 0.2.0 (schema 8)

mw 0.2.0 has no compatibility code: it reads schema 8 only. Boards written by 0.1.0 (and any older schema) are refused with

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

## Removed or renamed since 0.1.0

- `mw tenant use` is gone: use `mw tenant default SLUG` (folder default), a tenant path, `MW_TENANT` or `-t`.
- Reparenting is `mw edit ID -p PARENT|-`.
- Flag `-n` means `--limit`.
- A session is always optional: `mw start ID` without a session uses `local`.
- mw has no git commands: commit the board with your code.
