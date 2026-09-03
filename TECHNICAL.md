# miniwaves technical details

## Architecture

miniwaves is one Go CLI with no server, account, database, or web interface.
Its model is deliberately small:

- tenant — isolated app/project;
- prompt — original user request;
- story — desired outcome;
- task, spike, bug, chore — implementation work;
- session — tenant-bound coding assistant with at most one owned item.

The application does not launch assistants. Embedded dispatcher and session
prompts describe how a capable host application should create them.

## Storage and concurrency

Workspace state lives in `.miniwaves.json`. The current schema version is 3.
Writes hold `.miniwaves.json.lock`, write a temporary file, and atomically
rename it. Cross-platform file locking prevents concurrent local sessions from
overwriting one another.

State and lock files use `0600` where the operating system supports POSIX
permissions. `mw dirs init` creates missing shared folders with `0700` and
leaves existing folders unchanged.

This is single-machine coordination, not a distributed lease system.

## Output

Structured reads use compact [TOON](https://github.com/toon-format/spec) by
default. `-j` selects JSON and `-v` reveals full fields. Default output bounds
and flattens user text; verbose output preserves the stored value.

Prompt retrieval intentionally emits raw prompt text by default for direct
handoff. `-j` wraps it for host-tool integration.

## Supported systems

The release workflow builds arm64 and x86-64 binaries for macOS, Linux, and
Windows. Native CI runs the test suite on all three operating systems. Releases
contain checksummed `.tar.gz` archives for macOS/Linux and `.zip` archives for
Windows.

## Build and release

Source builds require Go 1.23 or newer:

```sh
./build.sh          # build/mw
./build.sh clean
```

On Windows PowerShell:

```powershell
./build.ps1         # build\mw.exe
./build.ps1 clean
```

Installers always install a compiled release binary. Remove it with:

```sh
curl -fsSL https://raw.githubusercontent.com/stormovermars/miniwaves/main/uninstall.sh | sh
```

```powershell
irm https://raw.githubusercontent.com/stormovermars/miniwaves/main/uninstall.ps1 | iex
```

Pushing a `v*` tag publishes release archives:

```sh
git tag vX.Y.Z
git push origin vX.Y.Z
```

Release binaries have no runtime dependencies. Third-party licenses are in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## Product boundary

There are no distributed leases, estimates, sprints, budgets, assistant
launchers, plugins, webhooks, analytics, or background services. If
multi-machine orchestration is required, use wvs rather than expanding
miniwaves into another fleet service.

See [AUDIT.md](AUDIT.md) for the source-app audit and scope decisions.
