# miniwaves (`mw`)

A tiny local work board for humans and coding assistants: plain JSON files, one Go binary, no server or account.
Assistants claim the next ready item, finish it with proof, or block it with a reason.

## Install

For Claude Code (installs the skill, `/mw-next`, `/mw-board` and the `mw` binary):

```sh
claude plugin marketplace add stormovermars/miniwaves && claude plugin install mw@miniwaves
```

Or the binary alone:

```sh
curl -fsSL https://raw.githubusercontent.com/stormovermars/miniwaves/main/install.sh | sh   # macOS, Linux
irm https://raw.githubusercontent.com/stormovermars/miniwaves/main/install.ps1 | iex        # Windows
go install github.com/stormovermars/miniwaves@latest                                        # binary is named miniwaves
```

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

Runs on macOS and Linux (tested). Windows binaries are built but not yet verified.

## Links

[USAGE.md](USAGE.md) (commands, prompts, gates, waves) · [TECHNICAL.md](TECHNICAL.md) · [MIGRATION.md](MIGRATION.md) · runner: [wr](https://github.com/stormovermars/wr)

MIT
