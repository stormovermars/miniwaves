# Changelog

Release notes per version are in `releases/`. This file lists what is merged and not yet released.

## Unreleased

- `mw ask --option ""` (a blank first option) is refused like any blank option; before, it was dropped and the ask could be recorded as free text or with the remaining options. USAGE.md now says an older mw drops `options` (it said keeps).

Released in 0.3.24: closed questions (`mw ask --option`) and events keeping unknown fields.
