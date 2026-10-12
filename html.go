package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// htmlPath resolves a publish path: ~/ is home, relative is under the workspace folder.
func htmlPath(boardPath, p string) string {
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(filepath.Dir(boardPath), p)
}

// writeHTML always writes the page (an explicit mw board --html) and its hash sidecar.
func writeHTML(boardPath string, w *Workspace, target string) error {
	_, err := publishHTML(boardPath, w, target, true)
	return err
}

// publishInterval is publish.interval in seconds: default 60, never below 5.
func publishInterval(p *PublishConfig) time.Duration {
	n := 60
	if p != nil && p.Interval != 0 {
		n = p.Interval
	}
	if n < 5 {
		n = 5
	}
	return time.Duration(n) * time.Second
}

// publishHTML writes the page unless (not force) the last write was less than the
// interval ago or the rendered page is unchanged since it. The sidecar <page>.hash holds
// "<unix time of the last write>\n<sha256 of the page>" and a third line "pending" when a
// change was skipped because of the interval: any later mw invocation (reads included)
// publishes it once the interval is due (publishPending). It reports whether it wrote.
func publishHTML(boardPath string, w *Workspace, target string, force bool) (bool, error) {
	out := htmlPath(boardPath, target)
	page := []byte(renderHTML(w))
	sum := sha256.Sum256(page)
	hash := hex.EncodeToString(sum[:])
	if !force {
		if b, err := readFileShared(out + ".hash"); err == nil {
			f := strings.Fields(string(b))
			if len(f) >= 2 {
				at, _ := strconv.ParseInt(f[0], 10, 64)
				pending := len(f) > 2
				switch {
				case f[1] == hash:
					if pending { // nothing differs any more
						_ = writeFileAtomicMode(out+".hash", []byte(f[0]+"\n"+f[1]+"\n"), 0644)
					}
					return false, nil
				case time.Since(time.Unix(at, 0)) < publishInterval(w.Publish):
					if !pending {
						_ = writeFileAtomicMode(out+".hash", []byte(f[0]+"\n"+f[1]+"\npending\n"), 0644)
					}
					return false, nil
				}
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return false, err
	}
	if err := writeFileAtomicMode(out, page, 0644); err != nil { // the page first, then its hash
		return false, err
	}
	return true, writeFileAtomicMode(out+".hash", []byte(strconv.FormatInt(time.Now().Unix(), 10)+"\n"+hash+"\n"), 0644)
}

// publishPending publishes a change that was skipped inside the interval, once the interval
// is due. Every mw invocation calls it; it costs one small file read when publish.auto is set.
func publishPending(path string, ix *Workspace) {
	if ix == nil || ix.Publish == nil || !ix.Publish.Auto || ix.Publish.HTML == "" {
		return
	}
	b, err := readFileShared(htmlPath(path, ix.Publish.HTML) + ".hash")
	if err != nil {
		return
	}
	f := strings.Fields(string(b))
	if len(f) < 3 {
		return
	}
	at, _ := strconv.ParseInt(f[0], 10, 64)
	if time.Since(time.Unix(at, 0)) < publishInterval(ix.Publish) {
		return
	}
	if full, err := load(path); err == nil {
		if _, err := publishHTML(path, full, ix.Publish.HTML, false); err != nil {
			fmt.Fprintln(os.Stderr, "mw: publish:", err)
		}
	}
}

const htmlHead = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>miniwaves board</title>
<style>
:root{--bg:#fff;--fg:#1b1f24;--mut:#6b7280;--line:#e5e7eb;--bar:#2563eb;--card:#f8fafc}
@media (prefers-color-scheme:dark){:root{--bg:#0f1317;--fg:#e6e8eb;--mut:#9aa3ad;--line:#2a313a;--bar:#60a5fa;--card:#161b21}}
body{margin:0;background:var(--bg);color:var(--fg);font:15px/1.45 system-ui,sans-serif}
main{max-width:860px;margin:0 auto;padding:16px}
h1{font-size:20px;margin:0 0 12px}h2{font-size:16px;margin:0}h3{font-size:12px;letter-spacing:.06em;text-transform:uppercase;color:var(--mut);margin:12px 0 4px}
section{background:var(--card);border:1px solid var(--line);border-radius:8px;padding:12px;margin:0 0 12px}
.bar{height:6px;background:var(--line);border-radius:3px;margin:6px 0}.bar i{display:block;height:6px;background:var(--bar);border-radius:3px}
.mut{color:var(--mut);font-size:13px}.tag{font-size:11px;font-weight:400;border:1px solid var(--mut);border-radius:8px;padding:0 6px;margin-left:4px}.archived{opacity:.5}ul{list-style:none;margin:0;padding:0}li{padding:2px 0}
code{font:13px ui-monospace,monospace}
</style></head><body><main>
`

func renderHTML(w *Workspace) string {
	var b strings.Builder
	esc := html.EscapeString
	b.WriteString(htmlHead)
	fmt.Fprintf(&b, "<h1>%s</h1>\n", esc(compactText(w.Name, 72)))
	for _, t := range shown(w, map[string]string{"archived": fmt.Sprint(w.showArchived)}) {
		bd := burndownOf(t, byName(gateStates(w, t), "judge").Effective)
		fmt.Fprintf(&b, "<section%s><h2>%s <span class=mut>[%s]</span>%s</h2>\n", map[bool]string{true: " class=archived"}[t.Hidden], esc(compactText(t.Name, 72)), esc(t.Slug), tagChips(t))
		fmt.Fprintf(&b, "<div class=bar><i style=\"width:%d%%\"></i></div><div class=mut>%d%% (%d/%d) &middot; %s</div>\n", bd.Percent, bd.Percent, bd.Judged, bd.Total, esc(activityLine(t)))
		for _, state := range []string{"doing", "blocked", "ready"} {
			var rows []string
			for _, it := range t.Items {
				if it.Status == state && it.Kind != "wave" {
					owner := ""
					if it.Assignee != "" {
						owner = " @" + displayName(it.Assignee)
					}
					rows = append(rows, fmt.Sprintf("<li><code>%s</code> %s <span class=mut>%s%s</span></li>", esc(it.ID), esc(compactText(it.Title, 90)), esc(it.Kind), esc(owner)))
				}
			}
			if len(rows) == 0 {
				continue
			}
			fmt.Fprintf(&b, "<h3>%s (%d)</h3><ul>\n", state, len(rows))
			for j, r := range rows {
				if j == 10 {
					fmt.Fprintf(&b, "<li class=mut>… %d more</li>\n", len(rows)-j)
					break
				}
				b.WriteString(r + "\n")
			}
			b.WriteString("</ul>\n")
		}
		b.WriteString("</section>\n")
	}
	var rows []logRow
	for _, t := range shown(w, map[string]string{"archived": fmt.Sprint(w.showArchived)}) {
		rows = append(rows, tenantRows(t)...)
	}
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].sortKey.After(rows[j].sortKey) })
	if len(rows) > 40 {
		rows = rows[:40]
	}
	b.WriteString("<section><h2>Activity</h2><ul>\n")
	for _, r := range rows {
		by := r.By
		if by == "" {
			by = "-"
		}
		fmt.Fprintf(&b, "<li><span class=mut>%s</span> %s <code>%s</code> %s %s", esc(shortTS(r.At)), esc(r.Tenant), esc(r.Item), esc(r.Action), esc(by))
		if r.Note != "" {
			fmt.Fprintf(&b, " <span class=mut>%s</span>", esc(compactText(r.Note, 90)))
		}
		b.WriteString("</li>\n")
	}
	b.WriteString("</ul></section>\n</main></body></html>\n")
	return b.String()
}
