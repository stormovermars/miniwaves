# One job model, two views: jobs = now, news = newsletter

Draft 3, 2026-10-22 (Sir: "jobs should be the current work indicator, but news should be a newsletter of what happened and what is ongoing, what is next").

## Review of what exists (why change)

- **Two summarizers:** jobs builds its own picture from `wave list`, `list -v` and `log -n 40`, while news renders `mw news`. The same counts disagree, and milestones, in progress and events read differently in the two panes.
- **News is a snapshot, not a newsletter:** each run rewrites one page. There is no "since last time", no history of releases and no "next".
- **Jobs shows history it doesn't need:** a 40-row events list and every closed milestone, while the live items (needs you, review) compete for space. At stress size (1000 milestones) it is unusable.
- **Metrics:** today they are a tiny-only parser (`parseSummary`). The generic `metrics.json` contract exists but no reader uses it yet.

## Concept

mw builds **one model** per job: `mw news --job ID --format json`. Everything else renders that model: md, html, text, the jobs pane and the news pane. A number is computed once.

| | jobs pane | news (pane, page, md) |
|---|---|---|
| Purpose | live work indicator: what is happening now, what needs me | newsletter: what happened, what is ongoing, what is next |
| Time | now | one **release** covers (previous release, this release] |
| Refresh | every poll | a release only when Sir asks; between releases, a live draft of the next one |
| History | none (one "last change HH:MM" line) | every release kept, with prev/next in the pane |

### Releases (on request only)

- A news **release** is cut only when Sir asks: with the **Release** button in the news pane (it asks first, then runs `mw news --job ID --release`), or by asking in chat. Nothing cuts a release automatically. The news desk tick never releases.
- Release N compares against release N−1: what happened between them, the metric changes since then, and the milestones closed since then. Release 1 compares against the job's start.
- Between releases, the news pane shows the **draft of the next release**: `mw news --job ID --format json` with no `--release`, covering everything since the last release. It is marked "Draft, not released", with "N changes since release M". The news desk tick only refreshes this draft (in `news.out`).
- `--release` writes `RELEASE_DIR/N.json` (the model, frozen), renders `RELEASE_DIR/N.md` and `N.html`, and updates `RELEASE_DIR/latest`. `RELEASE_DIR` is the job key `news_releases`, defaulting to `<dir of news_out>/releases`. If `news_url` is set, the published page shows the latest release.
- An empty release (nothing happened since the last one) is refused with "nothing new since release N", unless `--force` is given.
- Writes are atomic (temp file plus rename) under the job lock. Every release is kept; they are small.

### Model (json, version 1)

```
{ version: 1,
  job: {id, title, description, url},
  release: {number|null, at, since},                   // number null = draft
  now: {
    counts: {needs, review, blocked, doing, ready, done, total},
    needs[], review[], blocked[], doing[],             // item: {tenant, id, title, note, since, by}
    milestones: {open[], closed_count}                 // open sorted nearest-to-done first: {tenant, id, title, done, total}
  },
  happened: {                                          // events in (since, at]
    closed_milestones[], done[], accepted[], sent_back[], answers[], decisions[], new_items[],
    counts: {...per list}
  },
  next: {ready[] (queue order), next_milestones[] (open, nearest first)},
  metrics: {updated, sections[ metrics.md format, each row + delta{field: n} ]} | {error} | null,
  summary: "plain text from the job's summary file" }
```

**Caps:** each list has a cap and a `more` count. The `now` lists hold at most 50, `milestones.open` 200, each `happened` list 100, `next.ready` 10 and `next_milestones` 5. No item text is longer than 200 characters, and the whole model is at most 1 MB.

### Views

**Jobs pane (now):**
1. Headline: `!needs ◐review ⛔blocked ▶doing`, plus "last change HH:MM".
2. NEEDS YOU, then REVIEW, then BLOCKED. Each row has its ask action. Up to 10 per group, then "+N more", which expands in place.
3. IN PROGRESS: up to 10, then +N.
4. MILESTONES: the open ones only, nearest to done first, up to 8, then "+N open · ✓N closed".
5. METRICS: the metrics.json sections, each with ▲/▼ against the last poll.
6. No events list.

**News (newsletter, release N vs release N−1, or the draft):**
1. **In short:** the summary text when one is set. Otherwise 1–3 generated lines, e.g. "3 items done, 1 milestone closed, 2 need you".
2. **What happened:** closed milestones, then done/accepted, sent back, and decisions and answers. Each list shows its top 10 and "+N more".
3. **Ongoing:** the counts, plus the top 5 each of needs you, review, blocked and in progress.
4. **What's next:** the ready queue (10) and the next milestones (5).
5. **Metrics:** each row with its change since the previous release (▲/▼).
6. **Navigating releases (news pane):**
   - A title row reads "Release N · date · vs release N−1", or "Draft · since release M".
   - **◂ prev / next ▸** steps one release at a time; the draft is the newest page. **⇤ first / latest ⇥** jump to either end.
   - **☰ releases** opens a list of every release, newest first: number, date and a one-line headline (e.g. "5 done · 1 milestone · 2 need you"). Click a row to open that release. The list pages 20 at a time.
   - The page you are on stays put across refreshes. A new release shows a dot on ☰ and on "latest", and never moves you.
   - Past releases are read-only: the frozen N.json. Only the draft has the **Release** button (it asks first).
   - The ▦ jobs button stays.

### Look: an old gopher-style bulletin (Sir, 2026-10-22)

Every news rendering is a plain-text bulletin, in the style of a 1990s gopher hole. Text, md, html and the news pane all draw the same layout.

- **Plain monospace, 70 columns wide.** Text is ASCII except for ▲/▼. md wraps the whole bulletin in one fenced `text` block. html uses `<pre>` on a dark green-on-black or amber page, with one `<a>` link per release.
- **Masthead:**
```
======================================================================
 TINY CONSOLIDATION BULLETIN                       RELEASE 7 / 2026-10-22
 vs release 6 (2026-10-20)                    gopher://local/1/news/tiny
======================================================================
```
  The draft reads `DRAFT - NOT RELEASED` in place of `RELEASE N`, with `since release M`. The gopher URL is decoration only: `news_url` if one is set, otherwise the job id.
- **Sections** are numbered like a gopher menu, each underlined with dashes:
  `1. IN SHORT` / `2. WHAT HAPPENED` / `3. ONGOING` / `4. WHAT'S NEXT` / `5. METRICS`.
- **Item lines** start with a gopher type character, then the tenant/id, then the title, dot-leaders and a right-aligned state:
  `0 tinydoc/TSK-013  inline images on iOS ............... done`.
  The type characters are: `0` item, `1` milestone, `!` needs you, `?` review, `-` blocked, `+` in progress, `>` next. Long titles are cut with `~`.
- **Metrics** are drawn as ASCII bars, `[#####=====....]  5/68 ▲2`. A split row shows `#` good, `=` warn, `x` bad and `.` none.
- **"+N more"** becomes `... N more (see full board)`.
- **Footer:**
```
----------------------------------------------------------------------
 releases: 7 6 5 4 3 2 1                      end of bulletin
======================================================================
```
  In html each number in the footer links to that release. In the pane, ☰ and prev/next do the same.
- **The jobs pane is not a bulletin.** It keeps its live dashboard look.

### Alerts

Alerts stay in the panes (per type, deduplicated, one path). A new release counts as one alert (alertNew). Draft changes raise no alerts, and a metric drop counts as an alertMilestone-class alert.

## Compatibility

- `--format md|html|text` without `--release` keeps working and renders the draft.
- The news desk kit's tick keeps writing the draft. Releasing is never part of the tick.
- The panes read `latest` and `N.json` from the releases folder. When no archive exists, they fall back to running `mw news --format json`.
