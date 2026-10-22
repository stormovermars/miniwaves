# mw changes audit, b2c9b2f..main (39ac6ed), 2026-10-22

Scope: fanfare sets, jobs/needs/cursor/userconfig (M1), `mw news` md/html/text and the description key (M2), roles/kit/reap (M3), and the docs.
Method: read-only. I ran `go vet` (clean), `go test -run TestLogSince -count=3`, and probes with a main build and a 0.2.8 build (`git archive b2c9b2f`) on scratch boards. HOME was a scratch folder and MW_USER_CONFIG pointed into it.

Tags: **[mech]** means a Sonnet can do the fix from this text. **[Sir]** means a design decision is needed first.
Counts: high 3, med 11, low 10.
Resolution: applied on branch audit-fixes, 2026-10-22; see the last column.

| # | Sev | Where | Finding | Tag | Resolution |
|---|---|---|---|---|---|
| 1 | high | store.go:46-58, :154 | An old mw drops `jobs`, tenant `job` and fanfare `sets`/`use` | Sir | Fixed: schema 9; `mw migrate` takes 8 to 9; guard test as a schema-8 build. Unknown-key carry-through not done (new surface; schema 9 already protects shipped binaries). |
| 2 | high | news.go:35 | Markdown link and image injection in `mw news` | mech | Fixed: `newsMD` cleans, then escapes all ASCII punctuation; tests extended. |
| 3 | high | news.go:118 | `summary` as a FIFO hangs `mw news` while it holds the board lock | mech | Fixed: regular-file check before open, non-blocking open; FIFO test (unix). |
| 4 | med | main.go:271-305 | `mw news -o -` and text to stdout take the exclusive lock | mech | Fixed: `readOnly` for `news -o -` and `--format text` without `-o`; table test. |
| 5 | med | news.go:552 | With `-o -` the headline always says "first run", and `./.hash` is read from the CWD | mech | Fixed: no sidecar read and no moved segment on stdout; test. |
| 6 | med | cursor.go:53, :82 | Cursor keys use the event index, so a git merge that reorders events loses one | Sir (light) | Fixed: content-hash keys (12 hex of sha256 at|action|by|note); merge-reorder test. |
| 7 | med | cursor.go:120-131 | The cursor grows without bound and breaks command-line limits | Sir | Fixed: cursor = last timestamp + keys at it; 10k-event test (< 512 chars). Cost: `--lookback` removed, events stamped before the cursor time are no longer redelivered. |
| 8 | med | main.go:869, needs.go:56, brief.go:165/:513 | ESC injection in the text of board, review and log | mech | Fixed: `newsClean` in `compactText` and `newsCollect`; strip test over board, review, log, log --since, news md/html/text. |
| 9 | med | kit/buildlock.ps1:5 | `param()` binding eats the wrapped command's `-v`/`-o`/`-e` | mech (verify on Windows) | Fixed as described (no `param()`, `$args`). Not run: no pwsh here; needs the Windows CI leg. |
| 10 | med | kit/buildlock.sh:10-15 | Stale takeover race, plus a busy spin when `rm` fails | mech | Fixed: rename takeover with owner re-check, `ps -p` liveness; race and foreign-owner tests. ps1 got the same change (unrun). |
| 11 | med | items.go:1039 | `mw board -j` has no `needs` (the text board shows NEEDS YOU) | mech | Fixed: `needs` in `board -j`; test. |
| 12 | med | fanfare.go:830 | `mw fanfare` and `-j` report the board's enabled/use, not the user's effective values | mech | Fixed: status and `fanfare sets` use the effective config, `source` added, text shows `(user; board: ...)`; test. |
| 13 | med | jobs.go:69, news.go:541-549 | Two jobs collide on the default `news.out`; `--format text` fails the same check | mech + Sir | (a) fixed: text/stdout skip the check. (b) decided: keep `docs/news/`; duplicates still refused, message says to set `news.out` per job. |
| 14 | med | needs.go:150-166 | `mw decide --by NAME` works from any session (decider can be spoofed) | Sir | Fixed: open to humans and sessions; session (`-s`/MW_SESSION) is `By`, `--by` goes to the note as `(for NAME)`; else the human. USAGE updated. |
| 15 | low | cursor_test.go:81; news_test.go:71/:245 | Flaky TestLogSinceStartAndResume: cause found | mech | Fixed: parsed-time comparison; golden minute masking (this minute and the next). `emit` width left as is. |
| 16 | low | reap.go:116; prompts/*.md | The job-role spare depends on a self-declared `-a`, and the prompts never register it | mech + Sir | (a) fixed: Register line in both prompts, goldens updated. (b) decided: no new flag; reap prints `spared ROLE session NAME (job ID)`, `-j` has `spared`. |
| 17 | low | news.go:335 | The summary is entity-escaped inside a code fence and shows `&lt;` literally | mech | Fixed: entity replacer removed from the fence. |
| 18 | low | jobs.go:255 | `jobs enable` silently moves a tenant out of its current job | mech | Fixed: prints `moved SLUG from job OLD`; test. |
| 19 | low | news.go:562, :613 | `-j` is silently ignored for news on stdout | mech | Fixed: `-j` without a file target is an error; test. |
| 20 | low | news.go:273-425, news_text.go:177 | Three renderers drift apart; one section model would be smaller | mech (Sonnet) | Skipped: a three-renderer refactor is large surface and churn for a low-severity drift; do it as its own change. |
| 21 | low | prompts.go:35-37, :182; jobs.go:19; userconfig.go:24 | Speculative or dead code; a regex mismatch | mech | Fixed: Ceremonies hook, `buildDispatcherPrompt` (moved to a test helper), typos, `fanfare.use` validated with `setNameRe`; test. |
| 22 | low | jobs.go:24-51, userconfig.go:149-156 | JSON keys mix dotted and snake_case | Sir | Decided: follow the majority. Existing `-j` keys: 22 snake_case, 0 dotted. New outputs use snake_case (`news_out`, `news_url`, `pane_style`, `alerts_on`, `alerts_exec`, `fanfare_enabled`, `fanfare_use`); CLI keys unchanged. |
| 23 | low | news.go:429; brief.go shortTS | "Updated ... local" has no offset | mech | Partly fixed: `Updated` has a numeric offset. The event-time note was skipped. |
| 24 | low | tests | Missing negative tests | mech | Partly fixed: tests for #1, #2, #3, #6, #8, #10, #11, #16, unknown-item ask/answer/decide, `jobs set` bad key. Skipped: symlink-summary, dangling `PLAN.md`, full readOnly table. |

---

## 1. high: an old mw silently drops job config, tenant job and fanfare sets [Sir]
**Where:** store.go:46-58 (`indexDoc`), store.go:154 (`indexBytes`). M1's acceptance line, "old mw round-trips a 0.3.0 board", is not met.
**Scenario (reproduced):**
1. New mw: `mw jobs enable j1 -t b-one; mw jobs set j1 title=T`.
2. mw 0.2.8: `mw tenant add three`.
3. `.miniwaves.json` now has no `"jobs"` and no `"job"`. The pane, the prompts and the reap spare all lose the job.

Any index write by an old binary does this: `tenant add`/`tag`/`archive`, a workspace `gate`, or `fanfare set`. Fanfare `sets`/`use` (0.2.9) are lost the same way to 0.2.8. TestJobsRoundTripAndOldReader only proves that the old binary can *read* the board.
**Fix:** a decision is needed.
- (a) Bump to schema 9 with a `mw migrate` step, so old binaries refuse the board. This is the only option that protects binaries already shipped.
- (b) Keep schema 8 and accept the loss, documented, with ensure-mw pinning ≥0.3.0 everywhere.

With either option, also make future additive fields survive. Decode the index and tenant docs a second time into `map[string]json.RawMessage`. Keep the keys that the structs don't know in `Workspace.extraIndex` and `Tenant.extra`, and merge them back in `indexBytes`/`tenantBytes`.
**Test:** TestUnknownIndexKeysSurvive. Hand-write an index with `"zzz":{"a":1}` and a tenant-entry key `"yyy":1`, run `mw tenant add x`, and assert that both keys are still present.

## 2. high: Markdown link and image injection in `mw news` [mech]
**Where:** news.go:35 `newsMD`. It escapes only `\ & < > \` |` and newlines.
**Scenario (reproduced):** a session adds an item titled `![](http://track.example/p.png) [click](https://evil.example) *bold*`. `mw news --job J -o -` emits the title verbatim in "Needs you" and in "Last events".
- The pane's md renderer (D2) shows a phishing link and fetches a remote image, which acts as a beacon. That breaks "no network".
- GFM autolinks (`https://…`, `www.…`) also turn into links.
- ESC bytes pass through unchanged (see #8).

**Fix:** in `newsMD`:
1. Run `newsClean(s, false)` first.
2. Backslash-escape every ASCII punctuation character that CommonMark lets you escape (``!"#$%&'()*+,-./:;<=>?@[\]^_`{|}~``), not a hand-picked set.
3. Keep `<`/`>`/`&` as entities.

Escaping `:` also defuses autolinks. Remove the now-redundant `*`/`_` escaping of the description in `renderNewsMD` (news.go:278).
**Test:** extend TestNewsEscapingMD with that title, plus `<https://e>` and `www.e.com`. Assert the output contains no `](`, no `![`, and no unescaped `://`. Regenerate the news-md golden and review the diff.

## 3. high: a FIFO `summary` hangs `mw news` forever under the board lock [mech]
**Where:** news.go:118 `newsSummary`. `os.Open` is called before the regular-file check.
**Scenario (reproduced):**
1. `mw jobs set j1 summary=s.txt; mkfifo s.txt`.
2. `mw news --job j1 -o -` blocks in `open(2)`. `timeout 4` killed it with exit 124.

`news` takes the exclusive lock (and still would take the shared lock after #4). The pane polls it, so every board write by every session stalls. A tracked FIFO or a `/dev` symlink can come from repo content.
**Fix:**
1. In `newsSummary`, `os.Stat(p)` and return `"", nil` unless `Mode().IsRegular()` before opening.
2. Open with `os.OpenFile(p, os.O_RDONLY|syscall.O_NONBLOCK, 0)` on unix, using a build-tagged helper, so a swap between the stat and the open can't block either.
3. Keep the post-open `f.Stat()` check.

**Test:** TestNewsSummaryFIFO (unix build tag). Use `syscall.Mkfifo`, run `news -o -` in a goroutine, and require it to finish within 2 s with no Summary section.

## 4. med: the pane's poll takes the exclusive board lock [mech]
**Where:** main.go:271-305 `readOnly`. `news` is never read-only.
**Scenario:** D2 has the pane run `mw news --job ID -o -` on a tick. Each run takes the exclusive lock and serializes against every worker's write. With #3 that becomes a total stall.
**Fix:** in `readOnly`, `case "news":` return true when the value after `-o`/`--out` is `-`, or when `--format text` is given without `-o`. Parse with `options(args[1:])` and ignore errors, returning false.
**Test:** extend TestNewsIsAWrite into a table: `news -o -` → true, `news --format text` → true, `news` → false, `news -o x.md` → false.

## 5. med: on stdout the headline is always "first run", and the CWD's `.hash` is read [mech]
**Where:** news.go:552 calls `newsPrevStamp(target)` with `target == ""`, which reads `".hash"` relative to the process CWD.
**Scenario:** the pane, the main consumer of `-o -` under D2, always shows "…; first run". A stray `.hash` in the CWD changes the moved count.
**Fix:** when `target == ""`, pass `time.Time{}` and render the headline with the moved segment blank. Give `newsFill` a mode that removes `newsMovedMarker` (as the hashing mode does) when writing to stdout.
**Test:** TestNewsStdoutNoFirstRun. Create a `.hash` file holding `1 x` in the CWD, run `news -o -`, and assert that "first run" and "moved" are both absent.

## 6. med: cursor keys use the event index, so a git merge that reorders events loses one [Sir, light]
**Where:** cursor.go:53 `eventKey` = tenant/item/index; cursor.go:82.
**Scenario:**
1. Clones A and B each append event #5 to TSK-001.
2. The merge keeps B's event at index 5 and A's at index 6.
3. The poller on A had already seen key `…/TSK-001/5`, which was A's event. B's event now holds that key and is suppressed forever, while A's own event is delivered again as `/6`.

This late-merge case is exactly what the cursor was built for.
**Fix:** `eventKey` = `tenant/item/` + the first 12 hex digits of sha256(`at|action|by|note`). Keep `key` documented as opaque. The pane dedupes by key, so change the format before the 0.3.0 freeze. Sir should approve the contract change; the code itself is mechanical.
**Test:** TestLogSinceMergeReorder. Deliver, then rewrite the item's events with the last two swapped and a foreign event inserted at the old index. Assert the foreign event comes back exactly once and nothing else repeats. Update the log-since-j golden.

## 7. med: the cursor grows without bound [Sir]
**Where:** cursor.go:120-131. `seen` holds every delivered key inside the lookback window (24 h).
**Scenario:** a busy job board writes about 1,500 events a day. Each key is ~25 bytes, so the cursor reaches ~50 KB of base64. That passes the Windows command-line limit (32,767 chars) and, at ~4k events, Linux's 128 KB per-argument limit. `mw log --since <cursor>` then fails in the shell, not in mw.
**Fix options:**
- (a) Compact `seen`: 8-byte hashes, and only for events with ts > newT − lookback. That is roughly 4× smaller but still unbounded.
- (b) Add `--since-file PATH`, where mw reads and writes the cursor itself, under the same lock.
- (c) Cap `seen`, document the cap, and drop the oldest keys.

**Test:** TestLogSinceCursorSize. Create 2,000 events within an hour and assert `len(next) < 8192` under the chosen design.

## 8. med: ESC and control-character injection in terminal text [mech]
**Where:** main.go:869 `compactText`, which only folds whitespace. It is used by needs.go:56 `printNeeds` (`mw board`, `mw review`) and by brief.go:165/:513 (`mw log`, including `--since`). The md and html news outputs keep ESC too.
**Scenario (reproduced):** `mw ask TSK-001 "$(printf 'q \033[31mRED\033[0m')"`. `mw board`, `mw log --since start` and the md/html news all emit the raw `ESC[31m`, so any session can drive the human's terminal: titles, cursor moves, OSC 8 links.
**Fix:**
1. Move `newsClean` from news_text.go to main.go.
2. Call `newsClean(s, false)` at the top of `compactText`. That gives one sanitizer for every text output.
3. Also apply it in `newsCollect` to titles, notes and detail, so md and html are covered.

**Test:** TestTextOutputsStripControls. Ask with `\x1b[31m‮`, then assert that board, review, log, log --since, news md, news html and news text contain neither `\x1b` nor U+202E.

## 9. med: buildlock.ps1 eats the wrapped command's flags [mech, verify on the Windows CI leg]
**Where:** kit/buildlock.ps1:5. The `[Parameter(...)]` attributes make the script an advanced script, so common parameters are bound first, with prefix matching.
**Scenario:**
- `./buildlock.ps1 go test -v ./...`: `-v` binds to `-Verbose` and go never sees it.
- `./buildlock.ps1 go build -o bin/x .`: "parameter name 'o' is ambiguous" (OutVariable/OutBuffer).
- `-e`/`-d` fail the same way.

pwsh is not installed here, so this is unverified. TSK-031 already notes the missing test.
**Fix:**
1. Delete the `param(...)` line.
2. Add `if ($args.Count -lt 1) { [Console]::Error.WriteLine('usage: buildlock.ps1 COMMAND [ARGS...]'); exit 2 }`, `$Command = $args[0]`, `$Rest = @($args | Select-Object -Skip 1)`.
3. Keep `& $Command @Rest`.

**Test:** on the Windows CI leg, run `pwsh -File buildlock.ps1 go version -v` and `... cmd /c echo -o -e` and assert that the flags arrive.

## 10. med: buildlock.sh stale-takeover race, and a busy spin when `rm` fails [mech]
**Where:** kit/buildlock.sh:10-15. The ps1 has the same loop.

**Race (known, TSK-031):** waiters A and B both see a dead owner. B removes the lock, recreates it and writes its pid. A then runs `rm -rf` on B's fresh lock, and both run their builds.

**Spin:** with TMPDIR unset on Linux, `/tmp` is shared between users. When another user's lock exists, `kill -0` fails with EPERM, which the script reads as a dead owner. `rm -rf` then fails because of the sticky bit, and `continue` skips both the sleep and the `waited++`. The result is 100% CPU forever, with no timeout.

**Fix:**
1. Take over with `mv "$lock" "$lock.stale.$$" 2>/dev/null && rm -rf "$lock.stale.$$"`, and `continue` only when the `mv` succeeded. Otherwise fall through to the sleep and timeout.
2. Treat an owner as alive when `kill -0` fails with "not permitted": `kill -0 "$owner" 2>&1 | grep -q 'not permitted'`, or use `ps -p "$owner" >/dev/null`.
3. In the ps1, use `Rename-Item` the same way.

**Test:** extend TestKitBuildlockShell. Create a stale lock with a dead pid and start 8 waiters, each running `sh -c 'mkdir marker || exit 9; sleep 0.2; rmdir marker'`. Assert that none exits 9. Add a second case where the lock dir is `chmod 000` inside a non-writable parent: the script must time out with exit 75 within `BUILDLOCK_TIMEOUT=2`.

## 11. med: `mw board -j` has no `needs` [mech]
**Where:** items.go:1039 `boardDoc`. The text board prints NEEDS YOU (items.go:1124), and `review -j` has `needs`, but `board -j` and `board -A -j` don't (checked: 0 matches).
**Fix:** add `Needs []needRow \`json:"needs"\`` to `boardDoc`, filled with `needsOf(t)` and always an array. Update the board golden(s).
**Test:** extend TestGoldenReviewNeeds with `board -j` and `board -A -j`.

## 12. med: fanfare status reports the board's config, not the effective one [mech]
**Where:** fanfare.go:830 `c := fanfareCfg(w)` feeds the text status, `-j` (`enabled`, `use`) and `fanfare sets` (the `*` marker).
**Scenario:** the user runs `mw user set fanfare.enabled=false`. `mw fanfare` still prints "fanfare: enabled", and a UI (the tidewatch footer) reading `-j.enabled` shows the wrong state. The same happens with `fanfare.use`.
**Fix:**
- Keep `config` as it is (the board, for back-compat).
- Set the top-level `enabled`/`use` from `fanfareEffective(w)`, and add `"source":{"enabled":"user|board","use":"user|board"}`.
- In the text output, print `fanfare: disabled (user; board: enabled)`.
- In `fanfareSetsCommand`, take `use` from the effective config.

**Test:** TestFanfareStatusUserOverride, alongside TestUserFanfareOverridesBoard.

## 13. med: two jobs collide on the default `news.out` [mech for (a), Sir for (b)]
**Where:** jobs.go:69-71 makes every job default to `docs/news/`. news.go:541-549 refuses duplicate targets.
**Scenario (reproduced):**
1. Two jobs without an explicit `news.out`: plain `mw news` errors with "jobs … share the news file".
2. `mw news --format text`, which writes no file, fails the same check, as does any text run against a `news.out` that doesn't resolve.

**Fix:**
- (a) [mech] In `newsCommand`, skip `newsConfined` and the duplicate check when `text || stdout`.
- (b) [Sir] Default `news.out` to `docs/news/<id>/`. This changes the documented default (USAGE, help and spec line 100).

**Test:** TestNewsTwoJobsDefaults covers both.

## 14. med: `mw decide --by NAME` from any session [Sir]
**Where:** needs.go:150-166. Unlike `answer`, it skips `humanName`. The dispatcher prompt even instructs agents to run `mw decide TEXT --by NAME`.
**Scenario:** an agent records "decided by Sir", and `mw news` lists it under Decisions with nothing to show it came from an agent. The fallback `value(map{"u":USER},…)` also yields "human" on Windows, where the variable is USERNAME.
**Fix options:**
- (a) Make it human-only, like `answer` (use `humanName`), and change the dispatcher prompt to `mw ask` for decisions.
- (b) Allow sessions, but store `By` = the session and put the named decider in the note as `for NAME`.

Either way, reuse `humanName`'s fallback.
**Test:** TestDecideUnderSession.

## 15. low: flaky TestLogSinceStartAndResume, cause found [mech]
**Where:** cursor_test.go:81 compares `At` *strings*. `emit` stamps RFC3339Nano, which drops trailing zeros.
**Cause:** `created` and `ready` from one `mw add` land microseconds apart. If the first stamp's microsecond value ends in 0, it is one digit shorter, for example `…36.12345Z`. The next is `…36.123453Z`, and `'Z' > '3'`, so the earlier event sorts *after* the later one and the test fails with "not oldest first". That happens about 10% of the time per pair, ~19% per run. The product is unaffected: `logSince` sorts on parsed times, and news.go:200 only uses the string as a tiebreak when the times are equal.
**Fix:**
- In the test, compare `time.Parse(time.RFC3339Nano, …)` values with `Before`.
- Also fix news_test.go:71/:245, which replace `time.Now()` taken *after* the render and so fail at a minute boundary. Capture `now` before the run and replace both that minute and the next.
- Optional, also [mech]: make `emit` use the fixed-width layout `2006-01-02T15:04:05.000000000Z07:00`, so stamps sort as strings for any consumer (jq).

**Test:** `go test -run TestLogSince -count=200` is green.

## 16. low: the job-role spare in reap depends on a self-declared `-a`, and the prompts never register it [mech for (a), Sir for (b)]
**Where:** reap.go:116 `isJobRole`; prompts/dispatcher.md and prompts/newsdesk.md.

**Answer to "could a session named dispatcher be spared?":** no. The check reads `Agent` (`-a`), not the name, so `mw session start dispatcher` without `-a` is reaped.

**The real issues:**
- Any session started with `-a dispatcher` or `-a newsdesk` on a job tenant is never reaped. Items it holds stay claimed, and no flag includes those roles.
- Neither prompt tells the coordinator to run `mw session start NAME -t T -a dispatcher|newsdesk`, so the spare only works if the job-start skill does it.

**Fix:**
- (a) [mech] Add a "Register:" line to both prompts and update the 4 role goldens.
- (b) [Sir] In reap, still list spared role sessions in `-j` with `"spared":"job-role"`, and add `--roles` to include them.

**Test:** extend TestReapSkipsJobRoles with `-j`, checking the `spared` rows.

## 17. low: the summary is entity-escaped inside a code fence [mech]
**Where:** news.go:335. CommonMark does not decode entities in code blocks, so a summary line `a < b & c` renders as `a &lt; b &amp; c`. A fence can't carry raw HTML anyway, and `newsFence` already prevents breaking out of it.
**Fix:** remove the replacer on line 335. Keep `newsClean(s, true)`, which keeps newlines.
**Test:** a news-md golden whose summary contains `<b> & x`, shown literally inside the fence.

## 18. low: `jobs enable` silently moves a tenant out of its current job [mech]
**Where:** jobs.go:255.
**Scenario (reproduced):** `enable j1 -t a; enable j2 -t a` leaves j1 with no tenants, and the output doesn't say so.
**Fix:** collect the previous `t.Job` values and print `moved a from job j1` (or refuse without `--move`, which is Sir's call; printing is the default).
**Test:** a TestJobsEnableSetShowDisable case.

## 19. low: `-j` silently ignored for news on stdout [mech]
**Where:** news.go:562 (text without `-o`) and news.go:613 (`-o -`). Elsewhere, `fanfare sets add -j` *errors*.
**Fix:** in `newsCommand`, return `errors.New("-j needs a file target (-o PATH or news.out)")` when `json && (stdout || text && out == "")`.
**Test:** TestNewsJSONNeedsFile.

## 20. low: three renderers drift apart; one section model would be smaller [mech, Sonnet]
**Where:** news.go:273-340 (md), news.go:371-425 (html), news_text.go:177-330 (text). Each repeats the needs/blocked/doing/milestones/decisions/events/summary layout.

**Parity gaps today:**
- Text drops the moved/first-run segment.
- HTML has no "Nothing."/"None." placeholders but shows counts (`Needs you (N)`), which md doesn't.
- The URL is plain text in md and in html.
- Sanitizing is done in text only.

**Fix:**
- `newsCollect` returns `[]newsSection{Head, Empty string; Rows []string; Cap int}` built once, with cleaning (#8) and escaping-neutral text.
- Each renderer only maps sections to its syntax (md list/table, `<ul>`, boxed text).
- The headline and summary stay separate.

This is expected to remove ~80-120 lines and the drift. Regenerate the three goldens, and review the diff for intended changes only.

## 21. low: speculative or dead code, and a regex mismatch [mech]
- prompts.go:35-37 plus `{{if .Ceremonies}}` in both prompts: a hook for ceremonies.md (0.3.1, not built), always empty. Remove it until 0.3.1, per "mw stays thin".
- prompts.go:182 `buildDispatcherPrompt` is now a wrapper with no non-test callers. Inline it in the tests or delete it.
- jobs.go:19 `paneStylePatten` and userconfig.go:24 `fanfareUsePatten` are typos; rename to `…Pattern`.
- userconfig.go:24 accepts `[A-Za-z0-9._-]{0,63}` for `fanfare.use`, but set names are `^[a-z0-9][a-z0-9-]{0,31}$` (fanfare_sets.go `setNameRe`). So `mw user set fanfare.use=Calm.x` is stored but can never match, and every play falls back to all with a stderr note. Use `setNameRe` (plus `all`) in `setUserKey`.

**Test:** extend TestUserValidation with `fanfare.use=Calm.x`, which should be refused.

## 22. low: JSON key style [Sir]
**Where:** jobs.go:24-51 and userconfig.go:149-156.
The board stores `"news.out"` and `"pane.style"` (dotted) next to `"max_workers"` (snake_case), so `jq` needs `.["news.out"]`. `user show -j` uses dotted keys too. The keys mirror the CLI, which is defensible, but they freeze at 0.3.0.
**Decision:** keep the dotted keys as they are, or use snake_case JSON (`news_out`, `pane_style`) and keep the dotted CLI keys as aliases.

## 23. low: "Updated … local" has no offset [mech]
**Where:** news.go:429. Event rows (`shortTS`) are local time with no marker and no year.
The newsdesk commits these files, so a reader in another time zone can't place the stamp.
**Fix:** use the `2006-01-02 15:04 -0700` layout for Updated. For news only, render event times as `01-02 15:04` and add a single "times are UTC±hh:mm" note after Updated.
**Test:** the goldens already force `time.Local=UTC`. Update them to `<NOW> +0000`.

## 24. low: missing negative tests [mech]
Add the tests named in #1, #2, #3, #6, #8, #10, #11 and #16, plus these:
- `ask`/`answer`/`decide -p` on an unknown item give exit 1 and save nothing (assert the board bytes are unchanged).
- `jobs set J title=ok bogus=1` stores nothing (USAGE line 326 promises this).
- `news.out` and `summary` pointing through a symlink to outside the workspace are refused (confirm TestNewsConfinement covers `summary`, not just `news.out`).
- `init --kit` with a dangling symlink at `PLAN.md` is reported as skipped, not followed.
- A table test of `readOnly` for every new command and subcommand: `jobs show`/`set`, `news` variants, `fanfare sets`/`use`, `ask`/`answer`/`decide`, `log --since`, `init --kit`.

---

### Decision items for Sir
- #1: schema 9 versus accepting data loss from old binaries; carry unknown keys from now on.
- #6: change the event-key format before 0.3.0.
- #7: the cursor-size design (compact hashes, `--since-file`, or a cap).
- #13(b): default `news.out` per job.
- #14: `decide` human-only, or recorded by the session.
- #16(b): `--roles` and `spared` rows in reap.
- #22: JSON key style.

### Known and already tracked (not counted as new)
These overlap with TSK-030 (`news.path`), TSK-031 (buildlock race and ps1 test) and TSK-035 (alerts inert). #9 and #10 add new failure modes beyond what TSK-031 records.
