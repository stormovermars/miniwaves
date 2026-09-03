# Source-app audit

## The miniwaves cut

 It is a durable agreement between a user
and an assistant about **what was asked, what outcome is wanted, what work is
being done, and what should happen next**.

- isolated tenants with path-aware selection and per-tenant item IDs;
- folder-scoped tenant defaults with nearest-ancestor selection;
- three shared conventional folders for consumed inputs, reference docs, and
  generated outputs;
- a hierarchy of prompt → story → task/spike/bug/chore;
- explicit inbox, ready, doing, blocked, and done states;
- lightweight assistant sessions with exclusive one-item ownership;
- validated, copy-ready prompts that bootstrap a session on a tenant;
- a dispatcher prompt for creating tenants, assignments, and assistant sessions;
- acceptance text and completion/verification notes;
- stable human-readable IDs;
- `next` and `context` views for an assistant;
- a terminal board and progress bar for the user;
- compact TOON output by default, optional JSON for scripting, and one atomic
  local data file.

## Implementation audit

The CLI was re-audited after the initial build. The review found and corrected:

- stale global-active labeling when a folder or session selected another tenant;
- folder defaults that could not be inspected, inherited, or cleared cleanly;
- an unversioned folder-default schema and acceptance of future schemas;
- prompt/session filters that silently accepted irrelevant flags or tenants;
- duplicate board expansion flags (`-f` and `-v`), standardized on `-v`;
- world-readable state, lock, and newly created shared data directories;
- unbounded multiline user text in default output and unsafe agent-role values;
- stale block/completion notes after reopening work.

Tests cover these boundaries. The remaining deliberate constraints are local
single-machine coordination, manual assistant spawning, and release-dependent
installation; the one-line installer becomes usable after the first GitHub tag.

The same CLI now builds for macOS, Linux, and Windows. Cross-platform file
locking preserves safe concurrent local sessions; shell and PowerShell
installers verify checksummed platform archives.
