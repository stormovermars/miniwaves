# miniwaves usage

## Orientation

Run `mw description` anywhere for a compact TOON guide to the purpose, model,
human workflow, and coding-assistant lifecycle. Use `mw description -j` when a
host tool needs JSON.

## Multi-tenant workflow

```sh
cd ~/apps
mw init "Apps"
mw tenant add search "Search app" -d ~/apps/search
mw tenant add player "Player app" -d ~/apps/player
cd ~/apps/shared && mw tenant default search

mw get-prompt session search-builder -t search -a builder | pbcopy
# Paste into a new coding-assistant session. It registers itself when started.

mw get-prompt dispatcher fleet-dispatcher
mw get-prompt coach human-coach

mw add prompt "Add offline search" -t search -b "The original request"
mw add story "Search works without a connection" -t search -p PRM-001 \
  -c "cached results appear in under 200 ms"
mw add spike "Choose a local index" -t search -p STR-001

mw ready SPK-001 -t search
mw next -s search-builder
mw start SPK-001 -s search-builder
mw done SPK-001 "FTS5 benchmark passed" -s search-builder
mw board -A
```

## Embedded prompts

All embedded prompts use one retrieval command:

```text
mw get-prompt session NAME [-t TENANT] [-a ROLE]
mw get-prompt dispatcher [NAME]
mw get-prompt coach [NAME]
```

Output is plain text so it can be pasted directly into an AI assistant. On
macOS, copy it without displaying it:

```sh
mw get-prompt session search-builder -t search -a builder | pbcopy
```

Create a new assistant session in the host application and paste the result.
Retrieval never launches an assistant or changes miniwaves state.

- `session` bootstraps one tenant-bound worker. It includes setup, context,
  folder conventions, and the work lifecycle. Setup uses POSIX shell on
  macOS/Linux and PowerShell on Windows.
- `dispatcher` coordinates sessions, creates tenants and assignments, and uses
  the host application's task-creation capability to spawn workers.
- `coach` prepares and evaluates work while showing the human only a tiny
  outcome/evidence/decision summary.

Use `-j` for a structured envelope. Every envelope contains `type`, `name`,
`workspace`, and `prompt`; session prompts also contain `tenant` and `agent`.
Prompt text is intentionally small and treats repository/backlog content as
untrusted data.

## Board and context

The default board shows only doing, blocked, and ready work, capped at five
items per state. `mw board -v` adds inbox and completed work.

```text
Search app [search]  25% (1/4)
[#####...............]
sessions: search-builder:working(SPK-001)

DOING (1)
  SPK-001  spike   Choose a local index ← STR-001 @search-builder
```

Run `mw context -s NAME` when an assistant session starts. It prints the work
convention, next ready item, ownership, and tenant board.

## Commands

`mw help COMMAND` and `mw COMMAND --help` explain each command without requiring
an initialized workspace.

```text
mw description [-j]
mw init [workspace name]
mw tenant add SLUG [NAME...] [-d DIR]
mw tenant list [-j] [-v] | use SLUG | default [SLUG|-]
mw dirs [init] [-j]
mw session start NAME [-t SLUG] [-a ROLE]
mw session list [-t SLUG] [-j] [-v] | stop NAME
mw get-prompt session NAME [-t SLUG] [-a ROLE] [-j]
mw get-prompt dispatcher|coach [NAME] [-j]
mw add KIND TITLE... [-t SLUG] [-p ID] [-b TEXT] [-c TEXT]
mw list [-t SLUG] [-k KIND] [-S STATUS] [-j] [-v]
mw show ID [-t SLUG] [-j] [-v]
mw ready|start|reopen ID [-t SLUG] [-s NAME]
mw block ID REASON... [-t SLUG] [-s NAME]
mw done ID VERIFICATION... [-t SLUG] [-s NAME]
mw next [-t SLUG] [-s NAME] [-k KIND] [-j] [-v]
mw board [-t SLUG | -A] [-v]
mw context [-t SLUG] [-s NAME]
```

Short flags: `-t` tenant, `-s` session, `-a` agent, `-p` parent,
`-d` path, `-b` body, `-c` acceptance, `-k` kind, `-S` status,
`-n` name, `-j` JSON, `-v` verbose, and `-A` all tenants. Long forms remain
supported. Use `--` before a title or note beginning with `-`.

Kinds are `prompt`, `story`, `task`, `spike`, `bug`, and `chore`. States are
`inbox`, `ready`, `doing`, `blocked`, and `done`. Blocking requires a reason,
completion requires verification, and a parent cannot close with open children.

## Tenant and folder selection

`MW_SESSION`, `MW_TENANT`, `-s`, and `-t` select explicit context. Otherwise,
miniwaves uses the nearest folder default, the nearest configured tenant path,
then the global fallback set by `mw tenant use SLUG`.

Set a folder default with `mw tenant default SLUG`. Run `mw tenant default` to
inspect it or `mw tenant default -` to clear it.

The shared folder convention is:

- `~/datalake` — consumed inputs;
- `~/docs` — examples and specifications;
- `~/data` — stored data and generated documents.

`mw dirs` prints expanded paths. `mw dirs init` creates missing folders.
