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
- `--release` writes `RELEASE_DIR/N.json` (the model, frozen), renders `RELEASE_DIR/N.md` and `N.html`, and updates `RELEASE_DIR/latest`. `RELEASE_DIR` is the job key `news_releases`, defaulting to `<dir of news_out>/releases`. The job's own published page is the desk's business; mw writes releases only to RELEASE_DIR, and `news.out` always holds the draft.
- An empty release (nothing happened since the last one) is refused with "nothing new since release N", unless `--force` is given.
- Writes are atomic (temp file plus rename) under the job lock. Every release is kept; they are small.

### Model (json, version 1)

```
{ version: 1,
  job: {id, title, description, url},
  release: {number|null, at, since},                   // number null = draft
  now: {
    counts: {draft, needs, review, blocked, doing, ready, done, total},
    tenants[],                                         // {slug, name, counts{needs,review,blocked,doing,ready,done,total}, last_change}, by attention
    backlog: {items[], count},                         // ready queue order first, then drafts by creation; items carry state ready|draft; cap 50, more
    recent_done[],                                     // the last 10 done items, since = done time, milestone, milestone_title
    sprint: {kind:"wave", tenant, id, title, done, total, items[{...item, state todo|doing|review|blocked|done, needs?}], more} | null,  // the open wave nearest to done; items cap 50
    needs[], review[], blocked[], doing[],             // item: {tenant, id, title, note, since, by}
    milestones: {open[], closed_count}                 // open sorted nearest-to-done first: {tenant, id, title, done, total}
  },
  happened: {                                          // events in (since, at]
    closed_milestones[], done[], accepted[], sent_back[], answers[], decisions[], new_items[], moved_to_review[], moved_to_doing[],
    counts: {...per list}
  },
  next: {ready[] (queue order), next_milestones[] (open, nearest first)},
  metrics: {updated, sections[ metrics.md format, each row + delta{field: n} ]} | {error} | null,
  summary: "plain text from the job's summary file" }
```

**Caps:** each list has a cap and a `more` count. `now.tenants` holds at most 2000 rows (a row is about 150 bytes) plus `more`, sorted by attention: needs desc, review desc, blocked desc, last_change desc, slug. A job can have thousands of tenants; asks are gathered from every tenant in one run, so a pane needs exactly one mw call per job per poll. When the model would pass 1 MB, the item lists are trimmed first and the tenant rows last. `mw news --job ID --format json --tenant SLUG` is the same model filtered to one tenant; the release window and releases stay job-wide, and `--tenant` is refused with `--release` and with other formats. The `now` lists hold at most 50, `milestones.open` 200, each `happened` list 100, `next.ready` 10 and `next_milestones` 5. No item text is longer than 200 characters, and the whole model is at most 1 MB.

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

### Pane styles (Sir, 2026-10-22)

Both panes keep their current look, which stays the default. The style control is one group in both panes, `N | ░ | = | A | K` (bold letters, centred, equal widths), and the choice is stored per pane:

| Button | Style id | Jobs pane (now) | News pane (release) |
|---|---|---|---|
| **N** | `normal` | today's dashboard | today's look + newsletter sections |
| ░ | `blocks` | mono blocks | mono blocks |
| = | `gopher` | a live "NOW" sheet as gopher menu lines, same sections and caps | the bulletin below |
| **A** | `agile` | **Sprint + backlog:** the sprint is the open wave (else the open milestone nearest to done), with a progress bar and its items under To do / In progress / Review / Done; needs-you and blocked items are flagged. Then **Backlog:** the ready queue in order, then drafts | **Sprint review:** what was done, grouped by milestone; carry-over (still open); then the backlog as it stands |
| **K** | `kanban` | **Columns:** Backlog · Ready · In progress · Review · Done (recent). Blocked and needs-you cards stay in their column, flagged ⛔ / !. Narrow panes stack the columns | **Flow since the previous release:** what moved into Done, Review and In progress, plus what was added to Backlog |

- **Backlog** = drafts plus ready items. In both A and K it shows the top 10 with "+N", and the counts.
- **Naming:** the earlier "board / J (Jira-like)" style is replaced by A and K. A stored `board` choice opens as `kanban`. "Jira" survives only as a tracker kind (below).
- The news pane in normal and blocks style keeps today's look and gains the newsletter sections, the releases, the navigation and the Release button.

### Look: an old gopher-style bulletin (Sir, 2026-10-22)

mw's text, md and html news, and the panes in gopher style, are plain-text bulletins in the style of a 1990s gopher hole.

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
- **The published page stays as it is (Sir, 2026-10-22).** A job's own page at `news_url` (tiny: the claude.ai news page the news desk publishes) belongs to the desk and keeps its design. mw never replaces it, and the gopher look doesn't apply to it. The ↗ button keeps opening it. mw's own `--format html` is the bulletin, for jobs that have no page of their own.
- **The jobs pane is not a bulletin.** It keeps its live dashboard look.

### Alerts

Alerts stay in the panes (per type, deduplicated, one path). A new release counts as one alert (alertNew). Draft changes raise no alerts, and a metric drop counts as an alertMilestone-class alert.
- **Now events still alert in model mode:** a new needs-you, a new review, and a done item each raise an alert. They come from the `now` lists, not from the draft.
- **Dedupe key:** type + tenant:id + event time, never the text. When two sessions show the same job, an alert key is claimed in the store with compare-and-set before the alert is sent.
- **Overflow:** at most 3 notifications go out per poll. A 4th summary notification says "+N more", and the rest are logged as "capped".
- **No diff across sources:** a failed model poll keeps the last model snapshot and marks it stale. A model snapshot is never diffed against a legacy one.

## Compatibility

- `--format md|html|text` without `--release` keeps working and renders the draft.
- The news desk kit's tick keeps writing the draft. Releasing is never part of the tick.
- The panes read `latest` and `N.json` from the releases folder. When no archive exists, they fall back to running `mw news --format json`.

## Tracker tickets: Jira, Linear and similar tools (Sir, 2026-10-22)

An item can carry a ticket from the job's issue tracker. Wherever a ticket is shown, it is a real link.

**Optional throughout.** mw tracks all the work itself. Every item is an mw item with its own id, whether or not it has a ticket.
- A tracker and a ticket are optional cross-references: off by default, set per job and per item.
- mw never syncs with, reads from or writes to any tracker. The ticket is only a link to it.
- Jobs and items without a ticket look and behave exactly as they do today, in every style.

- **Job keys:** `tracker` (one of the kinds below) and `tracker_url` (the base URL). Set them with `mw jobs set ID tracker=jira tracker_url=https://acme.atlassian.net`. One tracker per job.
  - `tracker_url` must be https, with no query, fragment or credentials. A trailing slash is trimmed.
  - The ClickUp and Asana kinds may leave `tracker_url` empty; they then use their fixed host.
- **Item field `ticket`:**
  - set with `mw add … --ticket KEY` or `mw edit ID ticket=KEY` (`ticket=` clears it);
  - validated against the job's tracker kind, and refused when the job has no tracker;
  - stored in canonical form, the "Shown as" column below.
  - mw's own ids (`TSK-011`) are never taken as tickets; `ticket` is an explicit field only.
- **Model:** an item with a ticket gets `ticket`, `ticket_label` and `ticket_url`. mw builds and validates the URL from the table. Renderers never build URLs themselves.

| `tracker` | Accepted ticket | Shown as | Link | `tracker_url` example |
|---|---|---|---|---|
| `jira` | `MOB-142` | MOB-142 | `{base}/browse/{key}` | `https://acme.atlassian.net` |
| `linear` | `ENG-42` | ENG-42 | `{base}/issue/{key}` | `https://linear.app/acme` |
| `youtrack` | `APP-7` | APP-7 | `{base}/issue/{key}` | `https://acme.youtrack.cloud` |
| `github` | `123` or `#123` | #123 | `{base}/issues/{n}` | `https://github.com/acme/app` |
| `gitlab` | `123` or `#123` | #123 | `{base}/-/issues/{n}` | `https://gitlab.com/acme/app` |
| `azure` | `123` or `AB#123` | AB#123 | `{base}/_workitems/edit/{n}` | `https://dev.azure.com/acme/App` |
| `shortcut` | `123` or `sc-123` | sc-123 | `{base}/story/{n}` | `https://app.shortcut.com/acme` |
| `clickup` | `86abc12xy` or `#86abc12xy` | #86abc12xy | `https://app.clickup.com/t/{id}` | (empty) |
| `asana` | `1203456789012345` | Asana 1203456789012345 | `https://app.asana.com/0/0/{gid}` | (empty) |

- **Patterns:**
  - Key kinds (jira, linear, youtrack): `^[A-Z][A-Z0-9_]{0,9}-[0-9]{1,9}$`.
  - Numbers: `[0-9]{1,9}`.
  - ClickUp ids: `[a-z0-9]{6,12}`.
  - Asana gids: `[0-9]{10,20}`.
  - Keys are upper-cased before checking.
- **Agile and Kanban styles (A, K):** the card or row key is `ticket_label`, as a real link when `ticket_url` is set, and plain text otherwise. With no ticket, it shows `tenant/ID`. The mw key goes in the tooltip.
- **Other styles:** the ticket appears after the title as `[MOB-142]`, `[#123]` and so on. It is a link in md, html and the normal and blocks pane styles, and plain text in gopher text.
- **Unknown or newer `tracker` values:** the ticket is kept and shown as plain text, without a link.
