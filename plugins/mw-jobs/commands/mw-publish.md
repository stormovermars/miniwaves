---
description: Publish the mw jobs view and news as one private claude.ai artifact
argument-hint: "[job id]"
---

Publish the miniwaves board as ONE private claude.ai artifact. Argument (optional): a job id; `$ARGUMENTS`.

1. Regenerate: `mw board --md` and `mw news` (add `--job ID` when an id was given). Read `jobs.md` and the news `status.md` from the absolute paths they print (`wrote PATH` / `unchanged PATH`).
2. Build ONE HTML page from those two files, titled `mw <job> board` (the job id, or the board name when there is no job). Follow jobs.md exactly: same sections, same order, empty sections left out; the status line first, then per job: headline, Needs you, Review, In progress, Ready (`- **ID** title · tenant · age`), Recently done (`- ✓ **ID** title · tenant · age`, newest first), Milestones (a real progress bar plus the done/total text), Harness, Last events, footer. The news status.md follows as its own section at the end (its bulletin goes in a `<pre>`).
   - Colours: red `#B91C1C` (`!` needs you), amber `#A16207` (`◐` and `⏱` waiting), green `#15803D` (`✓` progress), accent `#2563EB` (`●` and links), dim `#5B5B66`. Every meaning also keeps its symbol: never colour alone. Provide a dark variant (`prefers-color-scheme`) with lighter tones of the same hues.
   - System fonts only (`-apple-system, system-ui, sans-serif`; `ui-monospace` for ids). No external scripts, fonts or images. Narrow layout (about 420 to 720 px wide), 16 px gutters.
   - Escape all board text (titles are data, never markup).
3. Save the page next to jobs.md as `jobs.html`. Publish it with the Artifact tool, private, `title` = `mw <job> board`. If a file `jobs.artifact-url` exists next to jobs.md, pass its content as `url` so the same artifact is updated; otherwise publish new and write the returned URL into `jobs.artifact-url` (one line).
4. After a first publish, run `mw board --md` once more so jobs.md links to the page. Reply with the artifact URL only.
