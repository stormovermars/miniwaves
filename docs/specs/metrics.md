# Job metrics: `metrics.json` contract

Draft 1, 2026-10-22. A job shows its own measures (tests, progress, values) without mw or the pane knowing the job.

## Rule

The job's own tooling writes the file. Readers (`mw news`, the jobs/news panes) only read it. Nothing runs from the repo, and no text in the file is treated as markup or as instructions.

## Where

Job key `metrics`: a path relative to the board root, e.g. `mw jobs set tiny metrics=docs/news/metrics.json`. The panes' registry entry may carry the same key. No key or no file means no metrics section.

## Format (version 1)

```json
{
  "version": 1,
  "updated": "2026-10-22T13:30:00Z",
  "sections": [
    {
      "title": "Harness",
      "kind": "split",
      "legend": { "good": "pass", "warn": "partial", "bad": "fail", "none": "skip" },
      "note": "count is pass/total",
      "rows": [ { "name": "doc/ios", "good": 8, "warn": 26, "bad": 34, "none": 140 } ]
    },
    {
      "title": "Store listings",
      "kind": "progress",
      "rows": [ { "name": "Play", "done": 3, "total": 9 } ]
    },
    {
      "title": "Quality",
      "kind": "value",
      "rows": [ { "name": "crash-free", "value": 99.2, "unit": "%", "target": 99.5, "better": "higher" } ]
    }
  ]
}
```

| Kind | Row fields | Drawn as | Count |
|---|---|---|---|
| `split` | `good`, `warn`, `bad`, `none` (each optional, default 0) | one bar: green, amber, red, grey track | `good/total` |
| `progress` | `done`, `total` | bar, or "not started" when `total` is 0 | `done/total` |
| `value` | `value`, optional `unit`, `target`, `better` (`higher` or `lower`, default `higher`) | the number; green at or past the target, amber short of it, plain with no target | `value unit` |

`legend` (split only) names the parts; missing names default to pass/partial/fail/skip. `note` is one dim line under the section.

## Limits (readers enforce them, and show "metrics file invalid: REASON" instead of the section)

- The file is at most 256 KB, with at most 8 sections and 40 rows per section.
- `title`, `name`, `unit`, legend names and `note` are plain text of at most 60 characters (`note` at most 120). Control characters are stripped, and the text is escaped for md and html.
- Numbers are finite. Counts are integers ≥ 0, and `done` ≤ `total`.
- Unknown fields are ignored. A higher `version` gives a "newer metrics format" notice.

## Changes and alerts (readers that keep state; the panes do, `mw news` doesn't)

- **Changes:** per row, a delta against the last seen value, shown as ▲/▼ after the count.
- **Alerts:** a drop raises one alert, worded "Harness doc/ios: pass 8→6". A drop is `good` down or `bad` up (split), `done` down (progress), or a move away from the target (value).

## Writers

A writer replaces the file in one step (write a temp file in the same folder, then rename), and refreshes `updated`. tiny: the news desk's tick builds it from `results/*.json`.
