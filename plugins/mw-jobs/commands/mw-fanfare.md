---
description: mw fanfare settings and test playback
argument-hint: "[next | on | off | use NAME | test [milestone|finale] | stop]"
---

Control the mw fanfare (played by the mw CLI when a wave closes or a tenant finishes). Arguments: `$ARGUMENTS`.

Map the first word to one command and run it, then report its output in one line:

- none: `mw fanfare`
- `on` / `off`: `mw fanfare set enabled=true` / `mw fanfare set enabled=false`
- `use NAME`: `mw fanfare use NAME` (NAME is a set name or `all`)
- `next`: `mw fanfare sets -j`, then `mw fanfare use` the next set after the current one in the list (wrapping)
- `test [milestone|finale]`: `mw fanfare test [milestone|finale]`
- `stop`: `mw fanfare stop`

Anything else: print the usage line above and run nothing.
