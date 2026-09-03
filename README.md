# miniwaves

`miniwaves` is a tiny local delivery board for humans and coding assistants.
It tracks user prompts, desired outcomes as stories, implementation work, and
multiple tenant-bound assistant sessions. There is no account, service,
database, or web interface.

## Install

macOS or Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/stormovermars/miniwaves/main/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/stormovermars/miniwaves/main/install.ps1 | iex
```

arm64 and x86-64 are supported. Installers verify release checksums and place
the compiled `mw` binary in `~/.local/bin` (`mw.exe` on Windows). Run
`mw help` after installation.

Run `mw description` for a short guide for humans and coding assistants.

## Quick start

```sh
mw init "My apps"
mw add prompt "Add offline search" -b "Original request"
mw add story "Search works offline" -p PRM-001 -c "cached results appear"
mw add task "Build local index" -p STR-001
mw ready TSK-001
mw board
```

Read [USAGE.md](USAGE.md) for workflows, embedded prompts, and the full command
reference. Read [TECHNICAL.md](TECHNICAL.md) for storage, concurrency, design,
build, clean, uninstall, and release details.

## License

MIT
