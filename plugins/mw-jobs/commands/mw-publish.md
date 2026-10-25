---
description: Publish the draft tabloid as a private artifact, then cut the edition
argument-hint: "<job id>"
---

Job: `$ARGUMENTS`.

1. Run `mw news --job $ARGUMENTS --format html`; it prints `wrote PATH` or `unchanged PATH`.
2. Publish PATH with the Artifact tool, private, declaring the `assets` capability. Reuse the URL in `jobs.artifact-url` next to it if that file exists, else write the returned URL there.
3. Upload every local image the page references (`<img src>`) as an artifact asset, put the returned URLs in a scratch copy of the page and publish that copy again. No `data:` URIs, no local paths.
4. Run `mw news --job $ARGUMENTS --release`.

Reply with the artifact URL.
