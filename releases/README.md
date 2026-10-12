# Releases

A release is a Markdown file in this folder named `<version>.md`. There is no `mw` command; the file is the record.

Front matter (between `---` lines), one `key: value` per line:

- `version`: release version, e.g. `1.2.0`
- `date`: YYYY-MM-DD
- `build`: path or URL of the build artifact
- `tested`: short result lines (inline list, e.g. `[go test ok, smoke ok]`)
- `items`: mw item ids shipped, e.g. `[TSK-001, BUG-002]`
- `wave`: optional wave id, e.g. `WAV-001`
- `evidence`: optional path to the proof folder of the release item itself (the chore that ships it), `<evidence>/<app>/<ID>/`, the same folder the evidence gate checks at `mw done`

Free-text release notes follow the front matter. See `0.0.0-example.md`.

Check a release file with `scripts/check-release.sh releases/<version>.md`.
