import { atom, read, update } from 'claude-code'
import type { Register } from 'claude-code'

import { BUILTIN, BUILTIN_NAMES, NORMAL, colorCss, isCustomColor, isStyleName, mergeStyle, scaleFor } from './style'
import type { Style } from './style'
import type {
  TidewatchEvent,
  TidewatchHarness,
  TidewatchItem,
  TidewatchJob,
  TidewatchRegistry,
  TidewatchSeen,
  TidewatchSnapshot,
  TidewatchAsk,
  TidewatchFanfare,
  TidewatchUserSettings,
  TidewatchFound,
  TidewatchWave,
} from '../types'

const PANE = 'mw-watch'
const NEWS_PANE = 'mw-watch-news'
/** The Markdown element refuses text over 10,000 characters, so a news page is cut well under that. */
export const NEWS_MAX = 9500
const DEFAULT_NEWS = 'docs/news/status.md'
const jobsTitle = (id: string) => `mw jobs · ${id === '' ? 'no board' : id}`
const newsTitle = (id: string) => `mw news · ${id}`
const EVERY = ['off', '15s', '30s', '1m', '5m'] as const
const EVERY_MS: Record<string, number> = { '15s': 15_000, '30s': 30_000, '1m': 60_000, '5m': 300_000 }

/** The per-user settings from `mw user get -j` that the plugin honours; the defaults are what it did before they existed. */
export type UserSettings = TidewatchUserSettings
const ALERT_KINDS = ['blocked', 'ask', 'review', 'done', 'release', 'metrics']
const DEFAULT_SETTINGS: UserSettings = {
  alertsEnabled: true,
  alertKinds: ALERT_KINDS,
  alertMessage: true,
  alertSound: true,
  discoverHome: true,
  discoverDepth: 4,
  refresh: undefined,
}

const registry = atom({ plugin: 'mw-watch', key: 'registry' } as const, { jobs: [], error: null })
const snapshots = atom({ plugin: 'mw-watch', key: 'snapshots' } as const, {})
const previous = atom({ plugin: 'mw-watch', key: 'previous' } as const, {})
const staleSince = atom({ plugin: 'mw-watch', key: 'staleSince' } as const, {})
const selected = atom({ plugin: 'mw-watch', key: 'selected' } as const, '')
const muted = atom({ plugin: 'mw-watch', key: 'muted' } as const, false)
const refreshEvery = atom({ plugin: 'mw-watch', key: 'refreshEvery' } as const, '1m')
const seen = atom({ plugin: 'mw-watch', key: 'seen' } as const, {})
const fontStep = atom({ plugin: 'mw-watch', key: 'fontStep' } as const, 0)
const styleName = atom({ plugin: 'mw-watch', key: 'style' } as const, 'normal')
const styles = atom({ plugin: 'mw-watch', key: 'styles' } as const, {})
const styleIssues = atom({ plugin: 'mw-watch', key: 'styleIssues' } as const, {})
const styleNames = atom({ plugin: 'mw-watch', key: 'styleNames' } as const, ['normal', 'blocks'])
const stalledMin = atom({ plugin: 'mw-watch', key: 'stalledMin' } as const, 45)
const newsAtom = atom({ plugin: 'mw-watch', key: 'news' } as const, {})
const busy = atom({ plugin: 'mw-watch', key: 'busy' } as const, [])
const foundAtom = atom({ plugin: 'mw-watch', key: 'found' } as const, [])
const userSettings = atom({ plugin: 'mw-watch', key: 'userSettings' } as const, DEFAULT_SETTINGS)
const fanfareAtom = atom({ plugin: 'mw-watch', key: 'fanfare' } as const, {})

const two = (n: number) => (n < 10 ? `0${n}` : `${n}`)
const hhmm = (ms: number) => {
  const d = new Date(ms)
  return `${two(d.getHours())}:${two(d.getMinutes())}`
}
const isoHhmm = (iso: string) => {
  const ms = Date.parse(iso)
  return Number.isNaN(ms) ? '--:--' : hhmm(ms)
}

const expand = (path: string, home: string, base?: string) => {
  if (path.startsWith('~')) return `${home}${path.slice(1)}`
  if (path.startsWith('/')) return path
  return base ? `${base}/${path}` : path
}

const memberParts = (w: TidewatchWave) => {
  const [d = '0', t = '0'] = w.members.split('/')
  return { done: Number(d) || 0, total: Number(t) || 0 }
}
const isPlaceholder = (w: TidewatchWave) => w.state === 'done' && memberParts(w).total === 0

const bar = (percent: number, width = 10) => {
  const filled = Math.max(0, Math.min(width, Math.round((percent / 100) * width)))
  return '█'.repeat(filled) + '░'.repeat(width - filled)
}

/** Short tenant labels: the first letter(s) left once the prefix shared by the job's tenants is stripped. */
export const tenantLabels = (tenants: string[]): Record<string, string> => {
  const out: Record<string, string> = {}
  if (tenants.length < 2) {
    for (const t of tenants) out[t] = ''
    return out
  }
  let common = tenants[0]!
  for (const t of tenants) {
    while (!t.startsWith(common)) common = common.slice(0, -1)
  }
  const rests = tenants.map(t => t.slice(common.length).replace(/^[^A-Za-z0-9]+/, '') || t)
  tenants.forEach((t, idx) => {
    let n = 1
    const rest = rests[idx]!
    while (n < rest.length && rests.some((r, j) => j !== idx && r.toUpperCase().startsWith(rest.slice(0, n).toUpperCase()))) n += 1
    out[t] = rest.slice(0, n).toUpperCase()
  })
  return out
}

export const waveName = (w: TidewatchWave, labels: Record<string, string>) => {
  const tenant = w.id.split(':')[0] ?? ''
  const token = w.title.split(' ')[0] ?? w.title
  const label = labels[tenant] ?? ''
  if (label === '' || token.toUpperCase().startsWith(label)) return token
  return `${label}-${token}`
}

const WORK_TYPES = new Set(['task', 'bug', 'spike', 'chore'])
const isReview = (i: TidewatchItem) =>
  i.state === 'review' || (i.state === 'blocked' && /awaiting review/i.test(i.note))
export const isNeed = (i: TidewatchItem) =>
  i.state !== 'done' && !isReview(i) && (i.state === 'blocked' || /\bneeds? (you|human|input|a decision)\b/i.test(`${i.title} ${i.note}`))

/** What needs a person: open asks from `review -j` when mw has them, else the blocked-note heuristic. */
export const needItems = (s: TidewatchSnapshot): TidewatchItem[] =>
  s.asks !== undefined
    ? s.asks.map(a => ({ tenant: a.tenant, id: a.item, type: 'task', state: 'ask', title: a.question.slice(0, 200), note: '', updated: a.at }))
    : s.items.filter(isNeed)

const ageText = (ms: number) => (ms < 3_600_000 ? `${Math.max(1, Math.round(ms / 60_000))}m` : `${Math.floor(ms / 3_600_000)}h`)

export type Analysis = {
  needs: TidewatchItem[]
  review: TidewatchItem[]
  doing: { item: TidewatchItem; ageMs: number | undefined; isStalled: boolean }[]
  stalled: number
  doneNew: number
  blockedNew: number
  changed: Map<string, 'done' | 'other'>
}

/** What needs attention now, and what changed after the seen cursor. */
export const analyse = (s: TidewatchSnapshot, now: number, seenTs: number, stalledMs: number): Analysis => {
  const lastEvent = new Map<string, TidewatchEvent>()
  const needs = needItems(s)
  const needKeys = new Set(needs.map(i => `${i.tenant}:${i.id}`))
  const changed = new Map<string, 'done' | 'other'>()
  let doneNew = 0
  let blockedNew = 0
  const evs = [...s.events].sort((a, b) => Date.parse(a.ts) - Date.parse(b.ts))
  for (const e of evs) {
    const key = `${e.tenant}:${e.item}`
    lastEvent.set(key, e)
    const t = Date.parse(e.ts)
    if (t > seenTs) {
      if (e.action === 'done') {
        changed.set(key, 'done')
        if (!e.item.startsWith('WAV-')) doneNew += 1
      } else if (e.action !== 'step') {
        changed.set(key, 'other')
        if (/^block/.test(e.action)) blockedNew += 1
      }
    }
  }
  const doing = s.items
    .filter(i => i.state === 'doing' && WORK_TYPES.has(i.type) && !needKeys.has(`${i.tenant}:${i.id}`) && !(s.asks === undefined && isNeed(i)) && !isReview(i))
    .map(item => {
      const stamps = [Date.parse(item.updated), Date.parse(lastEvent.get(`${item.tenant}:${item.id}`)?.ts ?? '')].filter(n => !Number.isNaN(n))
      const ageMs = stamps.length === 0 ? undefined : now - Math.max(...stamps)
      return { item, ageMs, isStalled: ageMs !== undefined && ageMs > stalledMs }
    })
  return {
    needs,
    review: s.items.filter(i => isReview(i) && !needKeys.has(`${i.tenant}:${i.id}`)),
    doing,
    stalled: doing.filter(d => d.isStalled).length,
    doneNew,
    blockedNew,
    changed,
  }
}

export type Headline = { text: string; tone: 'error' | 'warning' | 'success'; said: 'needs' | 'review' | 'stalled' | 'none' }

/** The headline states the top urgency tier only; `summaryParts` lists what it left out. */
export const headline = (a: Analysis): Headline => {
  if (a.needs.length > 0) return { text: `! ${a.needs.length} need you`, tone: 'error', said: 'needs' }
  if (a.review.length > 0) return { text: `◐ ${a.review.length} awaiting review`, tone: 'warning', said: 'review' }
  if (a.stalled > 0) return { text: `⏱ ${a.stalled} stalled`, tone: 'warning', said: 'stalled' }
  return { text: '✓ All moving', tone: 'success', said: 'none' }
}

export const summaryParts = (a: Analysis, h: Headline, ready: number): { text: string; color?: string }[] => {
  const out: { text: string; color?: string }[] = []
  if (h.said !== 'review' && a.review.length > 0) out.push({ text: `◐ ${a.review.length} awaiting review`, color: 'warning' })
  if (h.said !== 'stalled' && a.stalled > 0) out.push({ text: `⏱ ${a.stalled} stalled`, color: 'warning' })
  out.push({ text: `${a.doing.length} in progress` })
  if (h.said === 'none') {
    if (ready > 0) out.push({ text: `${ready} ready` })
    out.push({ text: 'nothing needs you' })
  }
  return out
}

export const diffChanges = (
  prev: TidewatchSnapshot,
  next: TidewatchSnapshot,
  labels: Record<string, string>,
): { closed: string[]; changes: string[] } => {
  const closed: string[] = []
  const changes: string[] = []
  const prevWaves = new Map(prev.waves.map(w => [w.id, w]))
  for (const w of next.waves) {
    const old = prevWaves.get(w.id)
    if (old !== undefined && old.state !== 'done' && w.state === 'done') {
      closed.push(`Milestone closed: ${waveName(w, labels)} ${w.title}`)
    }
  }
  const prevItems = new Map(prev.items.map(i => [`${i.tenant}:${i.id}`, i]))
  for (const i of next.items) {
    const old = prevItems.get(`${i.tenant}:${i.id}`)
    if (old === undefined) changes.push(`New ${i.tenant}:${i.id} ${i.title}`)
    else if (old.state !== i.state && i.state === 'done') changes.push(`Done ${i.tenant}:${i.id} ${i.title}`)
    else if (old.state !== i.state && i.state === 'blocked') changes.push(`Blocked ${i.tenant}:${i.id} ${i.title}`)
  }
  return { closed, changes }
}

const parseJson = (text: string): unknown => {
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

const str = (v: unknown) => String(v ?? '')
/** Drops the ` -t <slug>` tail some item titles carry. */
export const cleanTitle = (t: string) => t.replace(/\s+-t\s+\S+\s*$/, '').trim()

const asWaves = (v: unknown): TidewatchWave[] | undefined => {
  const list = Array.isArray(v) ? v : (v as { waves?: unknown } | undefined)?.waves
  if (!Array.isArray(list)) return undefined
  return list.map((w: Record<string, unknown>) => ({
    id: str(w.id),
    title: str(w.title),
    state: str(w.state),
    members: str(w.members ?? '0/0'),
    percent: Number(w.percent ?? 0) || 0,
    blocked: Number(w.blocked ?? 0) || 0,
  }))
}

const asItems = (v: unknown): TidewatchItem[] | undefined => {
  const list = Array.isArray(v) ? v : (v as { items?: unknown } | undefined)?.items
  if (!Array.isArray(list)) return undefined
  return list.map((i: Record<string, unknown>) => ({
    tenant: str(i.tenant),
    id: str(i.id),
    type: str(i.type),
    state: str(i.state),
    title: cleanTitle(str(i.title)).slice(0, 200),
    note: str(i.note).slice(0, 200),
    updated: str(i.updated),
  }))
}

const asEvents = (v: unknown): TidewatchEvent[] | undefined => {
  const list = Array.isArray(v) ? v : (v as { events?: unknown } | undefined)?.events
  if (!Array.isArray(list)) return undefined
  return list.map((e: Record<string, unknown>) => ({
    ts: str(e.ts),
    tenant: str(e.tenant),
    item: str(e.item),
    action: str(e.action),
    note: str(e.note).slice(0, 200),
  }))
}

export const parseSummary = (text: string): TidewatchHarness[] => {
  const out: TidewatchHarness[] = []
  let section = ''
  for (const line of text.split('\n')) {
    const h = /^##\s+(.+)$/.exec(line)
    if (h) {
      section = h[1]!.trim()
      continue
    }
    const m = /^- \*\*(.+?)\*\*.*?pass (\d+)(?:, partial (\d+))?, fail (\d+), skip (\d+)/.exec(line)
    if (m) out.push({ name: section ? `${section}/${m[1]}` : m[1]!, line: `pass ${m[2]} · fail ${m[4]} · skip ${m[5]}`, pass: Number(m[2]), fail: Number(m[4]), skip: Number(m[5]) })
  }
  return out
}

/** The one bar length every style and layout shares: half the body width, 10-40 cells, 20 when unknown. */
export const barLength = (width: number | undefined) =>
  width === undefined ? 20 : Math.max(10, Math.min(40, Math.floor(width / 2)))

// ---- the Svg styles: both builders read their sizes, columns, bar and colors from a style's tokens ----

export type SvgModel = {
  head: { text: string; tone: 'error' | 'warning' | 'success' }
  summary: { text: string; color?: string }[]
  chip: string
  needs: { title: string; meta: string; age: string; isNew: boolean }[]
  review: { id: string; title: string; meta: string; age: string; isNew: boolean }[]
  doing: { id: string; title: string; age: string; isStalled: boolean; isNew: boolean }[]
  waves: { name: string; done: number; total: number; pct: number; isNew: boolean; isClosed: boolean }[]
  folded: string
  harness: { name: string; pass: number; fail: number; skip: number; deltaPass: string; deltaFail: string; passUp: number; failUp: number }[]
  events: { t: string; text: string }[]
  footer: string
  density: 'compact' | 'default' | 'roomy'
  /** The sections to draw, in order (the style's order with the hidden ones left out). */
  order: string[]
}

const xml = (t: string) => t.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;')
const MONO = "ui-monospace,'SF Mono',Menlo,monospace"
const SANS = "-apple-system,'SF Pro Text',system-ui,sans-serif"
const TONE: Record<string, string> = { error: 'needs', warning: 'wait', success: 'pass', red: 'needs', amber: 'wait', green: 'pass' }

/** The most Svg source a pane draws; a bigger drawing is rebuilt smaller by the caller. */
export const SVG_LIMIT = 131_072

/** Drops drawn elements from the end until the source fits the limit (each entry is one whole element). */
const fitSvg = (out: string[], overhead: number) => {
  let size = out.reduce((n, p) => n + p.length, overhead)
  while (out.length > 1 && size > SVG_LIMIT) size -= out.pop()!.length
}

/**
 * The sans font off the terminal: four sizes (headline, body, note, label), a three-column grid
 * (name | bar | count) for milestones and harness, bars centred in rows of the style's row height.
 */
export const buildSvg = (m: SvgModel, st: Style, W = 420, k = 1): { source: string; alt: string; width: number } => {
  const f = { h: st.sizes.headline * k, b: st.sizes.body * k, n: st.sizes.note * k, l: st.sizes.label * k }
  const pad = 18 * k
  const rowH = st.rowHeight * k
  const gap = 12 * k
  const nameW = st.columns.name * k
  const digit = (f.b * 7) / 13
  const FIG = '\u2007'
  const dw = Math.max(1, ...m.waves.map(w => String(w.done).length), ...m.harness.map(h => String(h.pass).length))
  const tw = Math.max(1, ...m.waves.map(w => String(w.total).length), ...m.harness.map(h => String(h.pass + h.fail + h.skip).length))
  const countChars = dw + 1 + tw
  const deltaChars = Math.max(0, ...m.harness.map(h => (h.deltaPass ? h.deltaPass.length + 1 : 0) + (h.deltaFail ? h.deltaFail.length + 1 : 0)))
  const countW = countChars * digit
  const deltaW = deltaChars * digit
  const minRight = st.columns.count * k
  const rightW = Math.max(minRight, countW + deltaW)
  const right = W - pad
  const countEnd = right - deltaW
  const barX = pad + nameW + gap
  const barW = Math.max(20, W - pad - rightW - gap - barX)
  const barH = (st.bar.kind === 'rect' ? st.bar.height : 6) * k
  const barR = (st.bar.kind === 'rect' ? st.bar.radius : 3) * k
  const out: string[] = []
  let y = pad - 2 * k
  let uid = 0
  const num = (n: number) => Math.round(n * 100) / 100
  const t = (x: number, yy: number, size: number, cls: string, body: string, extra = '') =>
    out.push(`<text x="${num(x)}" y="${num(yy)}" font-size="${num(size)}" class="${cls}" xml:space="preserve" ${extra}>${body}</text>`)
  const base = (top: number, size: number) => top + rowH / 2 + size * 0.35
  // text cut to a box by a nested svg: clipped, no ellipsis
  const cutText = (x: number, top: number, w: number, size: number, body: string) =>
    out.push(`<svg x="${num(x)}" y="${num(top)}" width="${num(w)}" height="${num(rowH)}" overflow="hidden"><text x="0" y="${num(rowH / 2 + size * 0.35)}" font-size="${num(size)}" xml:space="preserve">${body}</text></svg>`)
  const label = (text: string, cls: string) => {
    t(pad, y + f.l, f.l, `s ${cls}`, xml(text.toUpperCase()))
    y += f.l + 4 * k
  }
  const sp = (cls: string, text: string) => `<tspan class="${cls}">${xml(text)}</tspan>`
  const bar = (top: number, segs: { w: number; cls: string }[]) => {
    if (st.bar.kind === 'glyphs') {
      const cells = Math.max(4, Math.round(barW / (f.b * 0.6)))
      let used = 0
      let acc = 0
      const parts: string[] = []
      for (const sg of segs) {
        acc += Math.max(0, sg.w)
        const upto = Math.min(cells, Math.round(acc * cells))
        const n = Math.max(0, upto - used)
        used += n
        parts.push(`<tspan class="${sg.cls}">${xml(st.bar.fill.repeat(n))}</tspan>`)
      }
      parts.push(`<tspan class="track">${xml(st.bar.rest.repeat(Math.max(0, cells - used)))}</tspan>`)
      t(barX, base(top, f.b), f.b, 'g', parts.join(''), `textLength="${num(barW)}" lengthAdjust="spacingAndGlyphs"`)
      return
    }
    const by = top + (rowH - barH) / 2
    const id = `c${uid++}`
    out.push(`<clipPath id="${id}"><rect x="${num(barX)}" y="${num(by)}" width="${num(barW)}" height="${num(barH)}" rx="${num(barR)}"/></clipPath>`)
    out.push(`<rect x="${num(barX)}" y="${num(by)}" width="${num(barW)}" height="${num(barH)}" rx="${num(barR)}" class="track"/>`)
    let x = barX
    for (const sg of segs) {
      const w = Math.max(0, Math.min(barW - (x - barX), sg.w * barW))
      if (w > 0) out.push(`<rect x="${num(x)}" y="${num(by)}" width="${num(w)}" height="${num(barH)}" class="${sg.cls}" clip-path="url(#${id})"/>`)
      x += w
    }
  }
  const countText = (top: number, done: number, total: number) =>
    t(countEnd, base(top, f.b), f.b, 'dim', xml(`${String(done).padStart(dw, FIG)}/${String(total).padEnd(tw, FIG)}`), 'text-anchor="end"')
  const wideRow = (nameBody: string, rightParts: string, top: number) => {
    cutText(pad, top, right - pad - minRight - gap, f.b, nameBody)
    if (rightParts) t(right, base(top, f.b), f.b, 'dim', rightParts, 'text-anchor="end"')
  }
  const note = (text: string) => {
    if (text === '' || m.density === 'compact') return
    cutText(pad + gap, y, W - 2 * pad - gap, f.n, sp('dim', text))
    y += rowH - 4 * k
  }
  const gapBetween = () => {
    y += (m.density === 'compact' ? 10 : m.density === 'roomy' ? 24 : 18) * k
  }

  // headline and summary
  t(pad, y + f.h, f.h, `b ${TONE[m.head.tone]}`, xml(m.head.text))
  y += f.h * 1.35
  const summary = m.summary.map((x, i) => sp(x.color ? TONE[x.color] ?? 'dim' : 'dim', `${i > 0 ? ' · ' : ''}${x.text}`)).join('')
  const chip = m.chip !== '' ? sp('dim', ' · ') + sp('new', `● ${m.chip}`) : ''
  t(pad, y + f.b, f.b, 'dim', summary + chip)
  y += f.b * 1.5
  gapBetween()

  const sections: Record<string, () => void> = {
    needs: () => {
      if (m.needs.length === 0) return
      label('Needs you', 'needs')
      for (const n of m.needs) {
        wideRow(sp('needs', '! ') + sp('fg', n.title), n.age ? sp('dim', n.age) + (n.isNew ? sp('new', ' ●') : '') : '', y)
        y += rowH
        note(n.meta)
      }
      gapBetween()
    },
    review: () => {
      if (m.review.length === 0) return
      label('Review', 'wait')
      for (const r of m.review) {
        wideRow(sp('dim', `${r.id} `) + sp('fg', r.title), (r.age ? sp('dim', r.age) : '') + (r.isNew ? sp('new', ' ●') : ''), y)
        y += rowH
        note(r.meta)
      }
      gapBetween()
    },
    progress: () => {
      if (m.doing.length === 0) return
      label('In progress', 'dim')
      for (const d of m.doing) {
        wideRow(sp('dim', `${d.id} `) + sp('fg', d.title), sp(d.isStalled ? 'wait' : 'dim', d.age) + (d.isNew ? sp('new', ' ●') : ''), y)
        y += rowH
      }
      gapBetween()
    },
    milestones: () => {
      if (m.waves.length === 0) return
      label('Milestones', 'dim')
      for (const w of m.waves) {
        const started = w.total > 0
        cutText(pad, y, nameW, f.b, sp(started ? 'fg' : 'dim', `${w.isClosed ? '✓ ' : ''}${w.name}`))
        if (!started) t(barX, base(y, f.n), f.n, 'dim', 'not started')
        else if (w.isClosed) t(barX, base(y, f.n), f.n, 'pass', '✓ closed')
        else bar(y, [{ w: w.total > 0 ? w.done / w.total : 0, cls: w.isNew ? 'pass' : 'prog' }])
        countText(y, w.done, w.total)
        if (w.isNew) t(right, base(y, f.b), f.b, 'pass', '●', 'text-anchor="end"')
        y += rowH
      }
      if (m.folded !== '') {
        t(pad, y + f.n + 2 * k, f.n, 'dim', xml(m.folded))
        y += rowH - 2 * k
      }
      gapBetween()
    },
    harness: () => {
      if (m.harness.length === 0) return
      label('Harness', 'dim')
      for (const h of m.harness) {
        const tot = Math.max(1, h.pass + h.fail + h.skip)
        cutText(pad, y, nameW, f.b, sp('fg', h.name))
        bar(y, [{ w: h.pass / tot, cls: 'pass' }, { w: h.fail / tot, cls: 'fail' }])
        countText(y, h.pass, tot)
        let dx = countEnd + digit
        if (h.deltaPass) {
          t(dx, base(y, f.b), f.b, h.passUp > 0 ? 'pass' : 'fail', xml(h.deltaPass))
          dx += (h.deltaPass.length + 1) * digit
        }
        if (h.deltaFail) t(dx, base(y, f.b), f.b, h.failUp > 0 ? 'fail' : 'pass', xml(h.deltaFail))
        y += rowH
      }
      t(pad, y + f.n + 2 * k, f.n, 'dim', `${sp('pass', '■ pass')}${sp('dim', ' · ')}${sp('fail', '■ fail')}${sp('dim', ' · ')}${sp('track', '■ skip')}${sp('dim', ' — count is pass/total')}`)
      y += rowH - 2 * k
      gapBetween()
    },
    events: () => {
      if (m.events.length === 0) return
      label('Last events', 'dim')
      for (const e of m.events) {
        cutText(pad, y, W - 2 * pad, f.n, sp('dim', `${e.t}  ${e.text}`))
        y += rowH - 4 * k
      }
      gapBetween()
    },
  }
  for (const id of m.order) sections[id]?.()
  t(pad, y + f.n, f.n, 'dim', xml(m.footer))
  y += f.n + pad

  fitSvg(out, 2000)
  const H = Math.ceil(y)
  const alt = `${m.head.text}. ${m.summary.map(x => x.text).join(', ')}`
  const css = `${colorCss(st)}\ntext{font-family:${SANS};font-variant-numeric:tabular-nums}.b{font-weight:700}.s{font-weight:600;letter-spacing:.08em}${st.bar.kind === 'glyphs' ? `.g{font-family:${MONO}}` : ''}`
  return {
    source: `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}"><style>${css}</style>${out.join('')}</svg>`,
    alt,
    width: W,
  }
}

export type MonoCell = { text: string; tone?: string }
export type MonoRow = {
  name: string
  nameTone?: string
  slot?:
    | { kind: 'text'; text: string; tone?: string }
    | { kind: 'bar'; pct: number; isNew: boolean }
    | { kind: 'stack'; pass: number; fail: number; skip: number }
  right?: MonoCell[]
}
export type MonoBlock =
  | { kind: 'head'; text: string; tone: string }
  | { kind: 'line'; parts: MonoCell[] }
  | { kind: 'label'; text: string; tone?: string }
  | { kind: 'rows'; rows: MonoRow[] }
  | { kind: 'gap' }

/** The font size a step gives the mono Svg: the style's body size times its scale factor, in whole pixels. */
export const monoSizeFor = (st: Style, step: number) => Math.round(st.sizes.body * scaleFor(st, step))

const cut = (t: string, n: number) => (t.length <= n ? t : t.slice(0, n))

/**
 * The mono font off the terminal: the whole body as monospace Svg text, one font and one size (the style's
 * body size). One grid: name column, 20-cell bar, count after it; nothing wraps, long text is cut without an
 * ellipsis. Bars are glyphs stretched to the cell grid, or rounded rects when the style's bar kind is rect.
 */
export const buildMonoSvg = (blocks: MonoBlock[], st: Style, step: number): { source: string; alt: string; width: number } => {
  const k = scaleFor(st, step)
  const size = monoSizeFor(st, step)
  const cw = size * 0.6
  const rowH = Math.round(st.rowHeight * k)
  const pad = 8
  const NAME = Math.round(st.columns.name)
  const BAR = 20
  const rightChars = Math.max(
    Math.round(st.columns.count),
    ...blocks.flatMap(b => (b.kind === 'rows' ? b.rows.map(r => (r.right ?? []).map(c => c.text).join('').length) : [0])),
  )
  const lineMax = Math.min(
    80,
    Math.max(0, ...blocks.map(b => (b.kind === 'line' ? b.parts.reduce((n, p) => n + p.text.length, 0) : b.kind === 'head' ? b.text.length : 0))),
  )
  const total = Math.max(NAME + BAR + rightChars, lineMax)
  const W = Math.ceil(pad * 2 + total * cw)
  const out: string[] = []
  const alt: string[] = []
  let y = 4
  const base = () => y + Math.round(rowH * 0.72)
  const tsp = (parts: MonoCell[], max: number) => {
    let left = max
    const o: string[] = []
    for (const p of parts) {
      if (left <= 0) break
      const t = cut(p.text, left)
      left -= t.length
      o.push(`<tspan class="${p.tone ?? 'fg'}">${xml(t)}</tspan>`)
    }
    return o.join('')
  }
  const line = (parts: MonoCell[], cls = 'm') => {
    out.push(`<text x="${pad}" y="${base()}" class="${cls}" xml:space="preserve">${tsp(parts, total)}</text>`)
    alt.push(parts.map(p => p.text).join(''))
    y += rowH
  }
  // a bar of BAR cells: glyphs stretched to the grid, or rects on a rounded track
  const cellBar = (sx: number, by: number, top: number, segs: { n: number; cls: string }[]) => {
    const used = segs.reduce((n, s) => n + s.n, 0)
    if (st.bar.kind === 'glyphs') {
      const g = st.bar
      const parts = segs.map(s => `<tspan class="${s.cls}">${xml(g.fill.repeat(s.n))}</tspan>`)
      parts.push(`<tspan class="track">${xml(g.rest.repeat(Math.max(0, BAR - used)))}</tspan>`)
      out.push(`<text x="${sx}" y="${by}" class="m" textLength="${BAR * cw}" lengthAdjust="spacingAndGlyphs" xml:space="preserve">${parts.join('')}</text>`)
      return
    }
    const h = st.bar.height * k
    const r = st.bar.radius * k
    const ry = Math.round((top + (rowH - h) / 2) * 100) / 100
    out.push(`<rect x="${sx}" y="${ry}" width="${BAR * cw}" height="${h}" rx="${r}" class="track"/>`)
    let x = sx
    for (const s of segs) {
      if (s.n > 0) out.push(`<rect x="${Math.round(x * 100) / 100}" y="${ry}" width="${Math.round(s.n * cw * 100) / 100}" height="${h}" class="${s.cls}"/>`)
      x += s.n * cw
    }
  }
  for (const b of blocks) {
    if (b.kind === 'gap') y += Math.round(rowH * 0.6)
    else if (b.kind === 'head') line([{ text: b.text, tone: b.tone }], 'm w')
    else if (b.kind === 'label') line([{ text: b.text.toUpperCase(), tone: b.tone ?? 'dim' }], 'm w')
    else if (b.kind === 'line') line(b.parts)
    else {
      for (const r of b.rows) {
        const top = y
        const by = base()
        out.push(`<text x="${pad}" y="${by}" class="m" xml:space="preserve"><tspan class="${r.nameTone ?? 'fg'}">${xml(cut(r.name, NAME - 1))}</tspan></text>`)
        const sx = pad + NAME * cw
        const slot = r.slot
        if (slot?.kind === 'text') {
          out.push(`<text x="${sx}" y="${by}" class="m" xml:space="preserve"><tspan class="${slot.tone ?? 'dim'}">${xml(cut(slot.text, BAR))}</tspan></text>`)
        } else if (slot?.kind === 'bar') {
          const f = Math.round((Math.max(0, Math.min(100, slot.pct)) / 100) * BAR)
          cellBar(sx, by, top, [{ n: f, cls: slot.isNew ? 'pass' : 'prog' }])
        } else if (slot?.kind === 'stack') {
          const t = Math.max(1, slot.pass + slot.fail + slot.skip)
          const pa = Math.round((BAR * slot.pass) / t)
          const fa = Math.round((BAR * slot.fail) / t)
          cellBar(sx, by, top, [{ n: pa, cls: 'pass' }, { n: fa, cls: 'fail' }])
        }
        if (r.right && r.right.length > 0) {
          out.push(`<text x="${pad + (NAME + BAR) * cw}" y="${by}" class="m" xml:space="preserve">${tsp(r.right, rightChars)}</text>`)
        }
        alt.push(`${r.name} ${(r.right ?? []).map(c => c.text).join('')}`.trim())
        y += rowH
      }
    }
  }
  fitSvg(out, 2000)
  const H = y + 6
  const css = `${colorCss(st)}\n.w{font-weight:600}.m{font-family:${MONO};font-size:${size}px}`
  return {
    source: `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}"><style>${css}</style>${out.join('')}</svg>`,
    alt: alt.join('; '),
    width: W,
  }
}

let timer: { cancel: () => void } | undefined
let sessionCwd = ''
let mwBin: string | undefined
const inFlight = new Set<string>()

const isAbsolute = (p: string) => p.startsWith('/') || /^[A-Za-z]:[\\/]/.test(p)
const baseName = (p: string) => p.replace(/[\\/]+$/, '').split(/[\\/]/).pop() ?? ''

/** Resolves a user-given path: absolute as given, relative to the session folder; `~` is never expanded. */
const absPath = (p: string) => {
  const t = p.trim().replace(/^(["'])(.*)\1$/, '$2')
  const full = isAbsolute(t) ? t : sessionCwd === '' ? t : `${sessionCwd.replace(/[\\/]+$/, '')}/${t}`
  return full.length > 1 ? full.replace(/[\\/]+$/, '') : full
}

/** The absolute path of the mw binary, resolved once: PATH lookup through the shell, then known install folders. */
const resolveMw = async ($: any): Promise<string | undefined> => {
  if (mwBin !== undefined) return mwBin
  const isWin = (await $.env.get('OS').catch(() => undefined)) === 'Windows_NT'
  try {
    const r = await $.process.run(isWin ? ['where.exe', 'mw'] : ['/bin/sh', '-c', 'command -v mw'], { timeoutMs: 5000 })
    const first = String(r.stdout ?? '').split(/\r?\n/)[0]?.trim() ?? ''
    if (r.exitCode === 0 && isAbsolute(first)) return (mwBin = first)
  } catch {
    // fall through to the known folders
  }
  const home: string = ((await $.env.get('HOME').catch(() => undefined)) as string | undefined) ?? ''
  const known = [
    ...(home !== '' ? [`${home}/.local/bin/mw`, `${home}/go/bin/mw`] : []),
    '/usr/local/bin/mw',
    '/opt/homebrew/bin/mw',
    '/usr/bin/mw',
  ]
  for (const p of known) {
    try {
      if (await $.fs.exists(p)) return (mwBin = p)
    } catch {
      // next
    }
  }
  return undefined
}

const MW_HOWTO = 'The mw command was not found. Install miniwaves (/plugin install mw@miniwaves, or put the mw binary on your PATH), then restart.'

type StoredJob = { hidden?: unknown; title?: unknown; board?: unknown; tenants?: unknown; news?: unknown; newsFile?: unknown; summary?: unknown }

const readRegistry = async ($: any): Promise<TidewatchRegistry> => {
  let raw: unknown
  try {
    raw = await $.store.get('registry')
  } catch {
    return { jobs: [], error: 'the plugin store is unavailable' }
  }
  const map = (raw as { jobs?: unknown } | undefined)?.jobs
  if (map === undefined || map === null || typeof map !== 'object' || Array.isArray(map)) return { jobs: [], error: null }
  const jobs: TidewatchJob[] = Object.entries(map as Record<string, StoredJob>)
    .filter(([, j]) => typeof j?.board === 'string' && isAbsolute(j.board as string))
    .map(([id, j]) => ({
      id,
      title: str(j.title ?? id),
      board: str(j.board),
      tenants: Array.isArray(j.tenants) ? j.tenants.map(str) : [],
      news: str(j.news),
      newsFile: str(j.newsFile),
      summary: str(j.summary),
      hidden: Array.isArray(j.hidden) ? j.hidden.map(str) : [],
    }))
  return { jobs, error: null }
}

const writeRegistry = async ($: any, jobs: TidewatchJob[]) => {
  const map: Record<string, StoredJob> = {}
  for (const j of jobs) map[j.id] = { title: j.title, board: j.board, tenants: j.tenants, news: j.news, newsFile: j.newsFile, summary: j.summary, ...((j.hidden ?? []).length > 0 ? { hidden: j.hidden } : {}) }
  await $.store.set('registry', { jobs: map })
}

const newsFileOf = (out: string) => {
  const o = out.trim()
  if (o === '') return ''
  return /\.md$/i.test(o) ? o : `${o.replace(/\/+$/, '')}/status.md`
}

/** One tab per job with `pane` on; a board with no jobs, or an mw too old for `jobs show`, stays one job of all its tenants. */
const expandBoards = async ($: any, boards: TidewatchJob[]): Promise<TidewatchJob[]> => {
  const bin = await resolveMw($)
  const out: TidewatchJob[] = []
  for (const b of boards) {
    let list: unknown
    if (bin !== undefined) {
      try {
        const r = await $.process.run([bin, 'jobs', 'show', '-j'], { cwd: b.board, timeoutMs: 20_000 })
        list = r.exitCode === 0 ? parseJson(r.stdout) : undefined
      } catch {
        list = undefined
      }
    }
    const jobs = Array.isArray(list) ? (list as Record<string, unknown>[]).filter(j => typeof j?.id === 'string' && j.id !== '') : []
    if (jobs.length === 0) {
      out.push({ ...b, name: '', boardId: b.id })
      continue
    }
    for (const j of jobs) {
      if (String(j.pane ?? 'on') === 'off') continue
      const tenants = Array.isArray(j.tenants) ? j.tenants.map(str) : []
      if (tenants.length === 0) continue
      out.push({
        id: `${b.id}/${str(j.id)}`,
        name: str(j.id),
        boardId: b.id,
        title: str(j.title ?? j.id),
        board: b.board,
        tenants,
        news: str(j['news.url']),
        newsFile: b.newsFile !== '' ? b.newsFile : newsFileOf(str(j['news.out'])),
        summary: b.summary !== '' ? b.summary : str(j.summary),
      })
    }
  }
  return out
}

const findJob = (jobs: TidewatchJob[], word: string): TidewatchJob | undefined => {
  const exact = jobs.find(j => j.id === word)
  if (exact !== undefined) return exact
  const by = jobs.filter(j => j.name === word || j.boardId === word)
  return by.length === 1 ? by[0] : undefined
}

/** Hidden tenants leave each job's tenant list (never the last one); `all` keeps the board's full list for the toggle row. */
const applyHidden = (boards: TidewatchJob[], jobs: TidewatchJob[]): TidewatchJob[] =>
  jobs.map(j => {
    const b = boards.find(x => x.id === (j.boardId ?? j.id))
    const hidden = b?.hidden ?? []
    if (b === undefined || b.tenants.length === 0) return j
    const left = j.tenants.filter(t => !hidden.includes(t))
    return { ...j, tenants: left.length > 0 ? left : j.tenants, hidden, all: b.tenants }
  })

const loadRegistry = async ($: any) => {
  const stored = await readRegistry($)
  const reg: TidewatchRegistry = stored.error !== null ? stored : { jobs: applyHidden(stored.jobs, await expandBoards($, stored.jobs)), error: null }
  await update($, registry, () => reg)
  const cur: string = await read($, selected)
  if (reg.jobs.length > 0 && !reg.jobs.some(j => j.id === cur)) {
    await update($, selected, () => reg.jobs[0]!.id)
  }
  return reg
}

// ---- styles: user styles live in $.store (key userStyles) as the JSON the person wrote ----

const readUserStyles = async ($: any): Promise<Record<string, unknown>> => {
  try {
    const v = await $.store.get('userStyles')
    return v !== null && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : {}
  } catch {
    return {}
  }
}

const refreshStyleNames = async ($: any) => {
  const user = Object.keys(await readUserStyles($)).filter(n => isStyleName(n) && !BUILTIN_NAMES.includes(n)).sort()
  const names = [...BUILTIN_NAMES, ...user]
  await update($, styleNames, () => names)
  return names
}

/** One toast per distinct set of problems in a style; none once it is clean again. */
const reportStyle = async ($: any, name: string, issues: string[]) => {
  const prev: Record<string, string> = await read($, styleIssues)
  const key = issues.join('\n')
  if (issues.length === 0) {
    if (prev[name] !== undefined) await update($, styleIssues, (m: Record<string, string>) => Object.fromEntries(Object.entries(m).filter(([k]) => k !== name)))
    return
  }
  if (prev[name] === key) return
  await update($, styleIssues, (m: Record<string, string>) => ({ ...m, [name]: key }))
  const shown = issues.slice(0, 3)
  if (issues.length > 3) shown.push(`+${issues.length - 3} more`)
  $.ui.toast(`mw-watch style "${name}": ${shown.join('; ')}. Those values are ignored.`, { timeoutMs: 10000 })
}

/** Resolves a style by name into the `styles` state (a missing one counts as normal). */
const resolveStyle = async ($: any, name: string): Promise<Style> => {
  const builtin = BUILTIN[name]
  let out: Style
  if (builtin !== undefined) out = builtin
  else {
    const user = await readUserStyles($)
    if (!isStyleName(name) || user[name] === undefined) out = NORMAL
    else {
      const { style, issues } = mergeStyle(NORMAL, user[name])
      await reportStyle($, name, issues)
      out = style
    }
  }
  await update($, styles, (m: Record<string, Style>) => ({ ...m, [name]: out }))
  return out
}

// ---- the news pane: the job's news Markdown, read as a file and never produced by running anything ----

const newsPathOf = (job: TidewatchJob) =>
  job.newsFile === '' ? `${job.board}/${DEFAULT_NEWS}` : isAbsolute(job.newsFile) ? job.newsFile : `${job.board}/${job.newsFile}`

const loadNews = async ($: any, job: TidewatchJob) => {
  const path = newsPathOf(job)
  let entry: { path: string; text: string; isCut: boolean; isMissing: boolean }
  try {
    const text: string = await $.fs.read(path)
    entry = { path, text: text.slice(0, NEWS_MAX), isCut: text.length > NEWS_MAX, isMissing: false }
  } catch {
    entry = { path, text: '', isCut: false, isMissing: true }
  }
  await update($, newsAtom, (m: Record<string, typeof entry>) => ({ ...m, [job.id]: entry }))
}

const openJobs = async ($: any) => {
  const reg: TidewatchRegistry = await read($, registry)
  const id: string = await read($, selected)
  const job = reg.jobs.find(j => j.id === id)
  await $.ui.open({ id: PANE, title: jobsTitle(job === undefined ? id : job.name || job.id) })
}

const openNews = async ($: any, wanted: string) => {
  const reg = await registryWithDiscovery($)
  if (reg.jobs.length === 0) return { text: 'mw-watch: no boards yet. Add one with /mw-watch add PATH.' }
  const id = wanted !== '' ? wanted : await read($, selected)
  const job = findJob(reg.jobs, id)
  if (job === undefined) return { text: `mw-watch: unknown job "${id}" (${reg.jobs.map(j => j.id).join(', ')}).` }
  await select($, job.id)
  await loadNews($, job)
  await $.ui.open({ id: NEWS_PANE, title: newsTitle(job.name || job.id), focus: true })
  return { text: `mw-watch: news pane opened for ${job.id}.` }
}

const setBusy = async ($: any, id: string, on: boolean) => {
  await update($, busy, (l: string[]) => (on ? [...l.filter(x => x !== id), id] : l.filter(x => x !== id)))
}

const saveSeen = async ($: any) => {
  try {
    await $.store.set('seen', await read($, seen))
  } catch {
    // store unavailable
  }
}

const harnessBase = (s: TidewatchSnapshot | undefined) => {
  const out: Record<string, { pass: number; fail: number }> = {}
  for (const h of s?.harness ?? []) out[h.name] = { pass: h.pass, fail: h.fail }
  return out
}

/** Advance a job's seen cursor to now, with the harness numbers as they stand. */
const markSeen = async ($: any, id: string) => {
  const now: number = await $.clock.now()
  const snaps: Record<string, TidewatchSnapshot> = await read($, snapshots)
  await update($, seen, (m: Record<string, TidewatchSeen>) => ({ ...m, [id]: { ts: now, harness: harnessBase(snaps[id]) } }))
  await saveSeen($)
}

const NOTIFIED_MAX = 500
/** Claims alert keys in the store: returns the ones not notified before and remembers all of them. */
let claimChain: Promise<unknown> = Promise.resolve()
const claimAlerts = ($: any, keys: string[]): Promise<Set<string>> => {
  // jobs refresh in parallel: one claim at a time, or two reads of the store would overwrite each other
  const run = claimChain.then(() => claimNow($, keys))
  claimChain = run.catch(() => undefined)
  return run
}
const claimNow = async ($: any, keys: string[]): Promise<Set<string>> => {
  let known: string[] = []
  try {
    const v = await $.store.get('notified')
    if (Array.isArray(v)) known = v.map(String)
  } catch {
    // none yet
  }
  const have = new Set(known)
  const fresh = new Set(keys.filter(k => !have.has(k)))
  if (fresh.size > 0) {
    try {
      await $.store.set('notified', [...known, ...fresh].slice(-NOTIFIED_MAX))
    } catch {
      // store unavailable: the alert still goes out once per run
    }
  }
  return fresh
}

const refreshJob = async ($: any, job: TidewatchJob) => {
  if (inFlight.has(job.id)) return
  inFlight.add(job.id)
  await setBusy($, job.id, true)
  try {
    const bin = await resolveMw($)
    const cwd = job.board
    const run = async (args: string[]): Promise<unknown> => {
      if (bin === undefined) return undefined
      try {
        const r = await $.process.run([bin, ...args], { cwd, timeoutMs: 20_000 })
        return r.exitCode === 0 ? parseJson(r.stdout) : undefined
      } catch {
        return undefined
      }
    }
    const [wv, lv, ev, rv] = await Promise.all([
      run(['wave', 'list', '-A', '-j']),
      run(['list', '-A', '-j', '-v']),
      run(['log', '-A', '-n', '40', '-j']),
      run(['review', '-j']),
    ])
    let summaryText = ''
    if (job.summary) {
      try {
        summaryText = await $.fs.read(isAbsolute(job.summary) ? job.summary : `${cwd}/${job.summary}`)
      } catch {
        summaryText = ''
      }
    }
    const now: number = await $.clock.now()
    const snaps: Record<string, TidewatchSnapshot> = await read($, snapshots)
    const old = snaps[job.id] as TidewatchSnapshot | undefined
    const inJob = (tenant: string) => job.tenants.length === 0 || job.tenants.includes(tenant)
    const waves = asWaves(wv)?.filter(w => inJob(w.id.split(':')[0] ?? ''))
    const items = asItems(lv)?.filter(i => inJob(i.tenant))
    const events = asEvents(ev)?.filter(e => inJob(e.tenant)).slice(0, 15)
    const needs = (rv as { needs?: unknown } | undefined)?.needs
    const asks: TidewatchAsk[] | undefined = Array.isArray(needs)
      ? needs
          .map((n: Record<string, unknown>) => ({ tenant: str(n.tenant), item: str(n.item), question: str(n.question), at: str(n.at) }))
          .filter(a => inJob(a.tenant))
      : undefined
    const isOk = waves !== undefined && items !== undefined && events !== undefined
    const hasNothing = waves === undefined && items === undefined && events === undefined

    if (hasNothing) {
      await update($, staleSince, (s: Record<string, number>) => ({ ...s, [job.id]: s[job.id] ?? now }))
      return
    }
    const next: TidewatchSnapshot = {
      waves: waves ?? old?.waves ?? [],
      items: items ?? old?.items ?? [],
      events: events ?? old?.events ?? [],
      harness: summaryText ? parseSummary(summaryText) : (old?.harness ?? []),
      asks: rv !== undefined ? asks : old?.asks,
      updatedAt: waves !== undefined && items !== undefined ? now : (old?.updatedAt ?? now),
    }
    if (old !== undefined) await update($, previous, (p: Record<string, TidewatchSnapshot>) => ({ ...p, [job.id]: old }))
    await update($, snapshots, (s: Record<string, TidewatchSnapshot>) => ({ ...s, [job.id]: next }))
    await update($, staleSince, (s: Record<string, number>) => {
      const { [job.id]: was, ...rest } = s
      return isOk ? rest : { ...rest, [job.id]: was ?? now }
    })
    const cursors: Record<string, TidewatchSeen> = await read($, seen)
    if (cursors[job.id] === undefined) {
      await update($, seen, (m: Record<string, TidewatchSeen>) => ({ ...m, [job.id]: { ts: now, harness: harnessBase(next) } }))
      await saveSeen($)
    }

    if (old !== undefined) {
      const labels = tenantLabels(job.tenants.length > 0 ? job.tenants : [...new Set(next.waves.map(w => w.id.split(':')[0] ?? ''))])
      const { closed, changes } = diffChanges(old, next, labels)
      const lines = [...closed, ...changes].map(l => `[${job.id}] ${l}`)
      const us0: UserSettings = await read($, userSettings)
      if (lines.length > 0 && us0.alertsEnabled && us0.alertMessage) {
        const shown = lines.slice(0, 4)
        if (lines.length > 4) shown.push(`+${lines.length - 4} more`)
        $.ui.toast(shown.join('\n'), { timeoutMs: 8000 })
      }
      // one alert per (tenant, item, event): the key carries the item's latest event time, kept in the store
      const oldKeys = new Map(old.items.map(i => [`${i.tenant}:${i.id}`, i]))
      const lastTs = new Map<string, string>()
      for (const e of next.events) {
        const k = `${e.tenant}:${e.item}`
        if (!lastTs.has(k) || Date.parse(e.ts) > Date.parse(lastTs.get(k)!)) lastTs.set(k, e.ts)
      }
      const keyOf = (kind: string, tenant: string, id: string, fallback: string) => `${job.id}|${kind}|${tenant}:${id}|${lastTs.get(`${tenant}:${id}`) ?? fallback}`
      void oldKeys
      const wasNeeding = new Set(needItems(old).map(i => `${i.tenant}:${i.id}`))
      const needing = needItems(next).filter(i => !wasNeeding.has(`${i.tenant}:${i.id}`))
      const alerts: { key: string; text: string; isUrgent: boolean; kind: string }[] = [
        ...next.waves
          .filter(w => old.waves.some(o => o.id === w.id && o.state !== 'done') && w.state === 'done')
          .map(w => ({ key: keyOf('closed', w.id.split(':')[0] ?? '', w.id.split(':')[1] ?? '', String(next.updatedAt)), text: `[${job.id}] Milestone closed: ${waveName(w, labels)} ${w.title}`, isUrgent: false, kind: 'done' })),
        ...needing.map(i => ({
          key: keyOf(i.state === 'blocked' ? 'blocked' : 'need', i.tenant, i.id, i.updated),
          text: `[${job.id}] ${i.state === 'blocked' ? 'Blocked' : 'Needs you'}: ${i.tenant}:${i.id} ${i.title}`,
          isUrgent: true,
          kind: i.state === 'blocked' ? 'blocked' : 'ask',
        })),
      ]
      const isMuted: boolean = await read($, muted)
      if (!isMuted && alerts.length > 0) {
        const us: UserSettings = await read($, userSettings)
        const fresh = await claimAlerts($, alerts.map(a => a.key))
        // every event is remembered, so switching a kind on later does not replay old ones; only the allowed kinds go out
        const send = us.alertsEnabled ? alerts.filter(a => fresh.has(a.key) && us.alertKinds.includes(a.kind)) : []
        // mw's own fanfare covers a closed milestone: the pane only pings for what needs a person
        if (us.alertSound && send.some(a => a.isUrgent)) {
          try {
            await $.audio.play({ asset: 'assets/Ping.aiff' })
          } catch {
            // no player: skip
          }
        }
        // no system notification banners: the in-app toast and the ping carry the alert
      }
    }
  } catch {
    // never throw out of a timer
  } finally {
    inFlight.delete(job.id)
    await setBusy($, job.id, false)
  }
}

const refreshAll = async ($: any) => {
  try {
    const reg = await loadRegistry($)
    const sel: string = await read($, selected)
    await refreshStyleNames($)
    await resolveStyle($, await read($, styleName))
    const cur = reg.jobs.find(j => j.id === sel)
    if (cur !== undefined && (await read($, newsAtom))[cur.id] !== undefined) await loadNews($, cur)
    await Promise.all([...new Set(reg.jobs.map(j => j.board))].map(b => loadFanfare($, b)))
    await Promise.all(reg.jobs.map(j => refreshJob($, j)))
  } catch {
    // never throw out of a timer or a detached refresh
  }
}

const refreshSelected = async ($: any) => {
  const reg = await loadRegistry($)
  await resolveStyle($, await read($, styleName))
  const id: string = await read($, selected)
  const job = reg.jobs.find(j => j.id === id)
  if (job) {
    await markSeen($, job.id)
    await refreshJob($, job)
  }
}

const TRUE_FALSE = (v: unknown, d: boolean) => (v === 'true' ? true : v === 'false' ? false : d)

/** Reads `mw user get -j`; an older mw (no `user get`) or any failure gives the defaults, silently. */
const loadUserSettings = async ($: any): Promise<UserSettings> => {
  let next = DEFAULT_SETTINGS
  try {
    const bin = await resolveMw($)
    if (bin !== undefined) {
      const r = (await $.process.run([bin, 'user', 'get', '-j'], { cwd: sessionCwd || '/', timeoutMs: 10_000 })) as { exitCode: number; stdout: string }
      const keys = r.exitCode === 0 ? (parseJson(r.stdout) as { keys?: unknown } | undefined)?.keys : undefined
      if (Array.isArray(keys)) {
        const m = new Map<string, unknown>()
        for (const k of keys) if (k !== null && typeof k === 'object' && typeof (k as { key?: unknown }).key === 'string') m.set((k as { key: string }).key, (k as { value?: unknown }).value)
        const kinds = m.get('alerts.kinds')
        const depth = Number(m.get('discover.depth'))
        const refresh = m.get('pane.refresh')
        next = {
          alertsEnabled: TRUE_FALSE(m.get('alerts.enabled'), true),
          alertKinds: typeof kinds === 'string' ? kinds.split(',').map(x => x.trim()).filter(x => x !== '') : ALERT_KINDS,
          alertMessage: TRUE_FALSE(m.get('alerts.message'), true),
          alertSound: TRUE_FALSE(m.get('alerts.sound'), true),
          discoverHome: TRUE_FALSE(m.get('discover.home'), true),
          discoverDepth: Number.isInteger(depth) && depth >= 1 && depth <= 6 ? depth : 4,
          refresh: typeof refresh === 'string' && (EVERY as readonly string[]).includes(refresh) ? refresh : undefined,
        }
      }
    }
  } catch {
    // older mw: defaults
  }
  await update($, userSettings, () => next)
  return next
}

/** The saved per-pane refresh override, if the user ever pressed the button or ran `every`. */
const savedEvery = async ($: any): Promise<string | undefined> => {
  try {
    const v = await $.store.get('refreshEvery')
    return typeof v === 'string' && (EVERY as readonly string[]).includes(v) ? v : undefined
  } catch {
    return undefined
  }
}

/** Re-reads the settings and, when there is no per-pane override, takes pane.refresh as the refresh interval. */
const syncSettings = async ($: any) => {
  const us = await loadUserSettings($)
  if (us.refresh !== undefined && (await savedEvery($)) === undefined) {
    await update($, refreshEvery, () => us.refresh!)
    await arm($)
  }
}

const arm = async ($: any) => {
  timer?.cancel()
  timer = undefined
  const every: string = await read($, refreshEvery)
  const ms = EVERY_MS[every]
  if (ms !== undefined) timer = $.clock.every(ms, () => void refreshAll($))
}

const persist = async ($: any, key: string, value: unknown) => {
  try {
    await $.store.set(key, value)
  } catch {
    // store unavailable
  }
}

const setEvery = async ($: any, value: string) => {
  await update($, refreshEvery, () => value)
  await persist($, 'refreshEvery', value)
  await arm($)
}

const cycleEvery = async ($: any) => {
  const cur: string = await read($, refreshEvery)
  const i = EVERY.indexOf(cur as (typeof EVERY)[number])
  await setEvery($, EVERY[(i + 1) % EVERY.length]!)
}

const setStep = async ($: any, next: number) => {
  const v = Math.max(-1, Math.min(1, next))
  await update($, fontStep, () => v)
  await persist($, 'fontStep', v)
}

const stepBy = async ($: any, delta: number) => {
  const cur: number = await read($, fontStep)
  await setStep($, delta === 0 ? 0 : cur + delta)
}

const setStyle = async ($: any, name: string) => {
  await update($, styleName, () => name)
  await persist($, 'style', name)
  await resolveStyle($, name)
}

const cycleStyle = async ($: any) => {
  const names = await refreshStyleNames($)
  const cur: string = await read($, styleName)
  await setStyle($, names[(names.indexOf(cur) + 1) % names.length] ?? 'normal')
}

const select = async ($: any, id: string) => {
  await update($, selected, () => id)
  await persist($, 'selectedJob', id)
}

// ---- fanfare footer: reads mw, writes only the per-user config (`mw user set`), never the board ----

const runMw = async ($: any, board: string, args: string[], timeoutMs = 20_000) => {
  const bin = await resolveMw($)
  if (bin === undefined) return undefined
  try {
    return (await $.process.run([bin, ...args], { cwd: board, timeoutMs })) as { exitCode: number; stdout: string; stderr?: string }
  } catch {
    return undefined
  }
}

/** Reads `fanfare -j` and `user show -j`; the per-user value wins. Hidden (removed) when either is missing the needed keys. */
const loadFanfare = async ($: any, board: string) => {
  const [f, u] = await Promise.all([runMw($, board, ['fanfare', '-j']), runMw($, board, ['user', 'show', '-j'])])
  const fan = f !== undefined && f.exitCode === 0 ? (parseJson(f.stdout) as Record<string, unknown> | undefined) : undefined
  const usr = u !== undefined && u.exitCode === 0 ? (parseJson(u.stdout) as Record<string, unknown> | undefined) : undefined
  const sets = fan?.sets
  let next: TidewatchFanfare | undefined
  if (fan !== undefined && usr !== undefined && fan !== null && usr !== null && sets !== null && typeof sets === 'object' && !Array.isArray(sets)) {
    const boardEnabled = typeof fan.enabled === 'boolean' ? fan.enabled : (fan.config as { enabled?: unknown } | undefined)?.enabled === true
    const userEnabled = usr['fanfare.enabled']
    const userUse = usr['fanfare.use']
    next = {
      enabled: typeof userEnabled === 'boolean' ? userEnabled : boardEnabled,
      use: typeof userUse === 'string' && userUse !== '' ? userUse : str(fan.use) || 'all',
      sets: Object.keys(sets as object).sort(),
    }
  }
  await update($, fanfareAtom, (m: Record<string, TidewatchFanfare>) => {
    const { [board]: _gone, ...rest } = m
    return next === undefined ? rest : { ...rest, [board]: next }
  })
  return next
}

const fanfareSet = async ($: any, board: string, kv: string[]) => {
  const r = await runMw($, board, ['user', 'set', ...kv])
  await loadFanfare($, board)
  await syncSettings($)
  return r !== undefined && r.exitCode === 0
}

const fanfareToggle = async ($: any, board: string, on?: boolean) => {
  const cur = ((await read($, fanfareAtom)) as Record<string, TidewatchFanfare>)[board]
  if (cur === undefined) return false
  return fanfareSet($, board, [`fanfare.enabled=${on ?? !cur.enabled}`])
}

/** The top-row button's label: off, all, or the set name. */
const fanfareLabel = (f: TidewatchFanfare) => (f.enabled ? f.use : 'off')

/** One press: off -> all -> <set 1> -> <set 2> ... -> off. Returns the `mw user set` args for the next step. */
const fanfareNextArgs = (f: TidewatchFanfare): string[] => {
  if (!f.enabled) return ['fanfare.enabled=true', 'fanfare.use=all']
  const i = f.sets.indexOf(f.use)
  const nextSet = i < 0 ? f.sets[0] : f.sets[i + 1] // 'all' (or a stale name) starts the sets
  return nextSet === undefined ? ['fanfare.enabled=false'] : [`fanfare.use=${nextSet}`]
}

const fanfareCycle = async ($: any, board: string) => {
  const cur = ((await read($, fanfareAtom)) as Record<string, TidewatchFanfare>)[board]
  if (cur === undefined) return false
  return fanfareSet($, board, fanfareNextArgs(cur))
}

/** Plays the milestone fanfare once; on failure the reason is mw's own last stderr line, shown in one toast. */
const fanfareTest = async ($: any, board: string): Promise<{ ok: boolean; reason: string }> => {
  // mw fanfare test detaches the player, so it returns at once; keep well inside the 30 s process timeout
  const r = await runMw($, board, ['fanfare', 'test', 'milestone'], 25_000)
  if (r !== undefined && r.exitCode === 0) return { ok: true, reason: '' }
  const err = String(r?.stderr ?? r?.stdout ?? '').trim().split(/\r?\n/).filter(l => l.trim() !== '').pop() ?? ''
  const reason = r === undefined ? 'mw did not run' : err !== '' ? err : `mw exited ${r.exitCode}`
  $.ui.toast(`mw-watch: fanfare test failed: ${reason}`, { timeoutMs: 8000 })
  return { ok: false, reason }
}

const fanfareCommand = async ($: any, rest: string) => {
  const reg = await registryWithDiscovery($)
  const id: string = await read($, selected)
  const job = reg.jobs.find(j => j.id === id) ?? reg.jobs[0]
  if (job === undefined) return { text: 'mw-watch: no boards yet. Add one with /mw-watch add PATH.' }
  const cur = await loadFanfare($, job.board)
  if (cur === undefined) return { text: 'mw-watch: this mw has no fanfare sets or per-user config (needs mw 0.3).' }
  const w = rest.trim()
  if (w === 'on' || w === 'off') {
    const done = await fanfareToggle($, job.board, w === 'on')
    return { text: done ? `mw-watch: fanfare ${w}.` : 'mw-watch: could not change fanfare.' }
  }
  if (w === 'next') {
    const done = await fanfareCycle($, job.board)
    const now = ((await read($, fanfareAtom)) as Record<string, TidewatchFanfare>)[job.board]
    return { text: done && now ? `mw-watch: fanfare ${fanfareLabel(now)}.` : 'mw-watch: could not change fanfare.' }
  }
  if (w === 'test') {
    const t = await fanfareTest($, job.board)
    return { text: t.ok ? 'mw-watch: fanfare test started.' : `mw-watch: fanfare test failed: ${t.reason}` }
  }
  const u = /^use\s+(\S+)$/.exec(w)
  if (u) {
    if (u[1] !== 'all' && !cur.sets.includes(u[1]!)) return { text: `mw-watch: unknown set "${u[1]}" (all, ${cur.sets.join(', ')}).` }
    const done = await fanfareSet($, job.board, [`fanfare.use=${u[1]}`])
    return { text: done ? `mw-watch: fanfare set ${u[1]}.` : 'mw-watch: could not change the fanfare set.' }
  }
  return { text: `mw-watch: fanfare ${cur.enabled ? 'on' : 'off'}, set ${cur.use}. Usage: /mw-watch fanfare on|off|next|use NAME|test` }
}

const FAN_PANE = 'mw-fanfare'

/** `/mw-fanfare`: bare shows the state and a cycle button; on, off, use NAME and test are the same as under /mw-watch fanfare. */
const fanfareSlash = async ($: any, args: string) => {
  const w = args.trim()
  if (w !== '') {
    const r = await fanfareCommand($, w === 'next' || w === 'cycle' ? 'next' : w)
    return { text: r.text.replace('/mw-watch fanfare', '/mw-fanfare') }
  }
  const reg = await registryWithDiscovery($)
  const id: string = await read($, selected)
  const job = reg.jobs.find(j => j.id === id) ?? reg.jobs[0]
  if (job === undefined) return { text: 'mw-watch: no boards yet. Run /mw-watch first.' }
  const cur = await loadFanfare($, job.board)
  if (cur === undefined) return { text: 'mw-watch: this mw has no fanfare sets or per-user config (needs mw 0.3).' }
  await $.ui.open({ id: FAN_PANE, title: 'mw fanfare' })
  return { text: `mw-fanfare: ${cur.enabled ? 'on' : 'off'}, set ${cur.use}; sets: all, ${cur.sets.join(', ')}. Usage: /mw-fanfare on|off|next|use NAME|test` }
}

const toggleMute = async ($: any) => {
  const now: boolean = !(await read($, muted))
  await update($, muted, () => now)
  return { text: now ? 'mw-watch: sound and notifications muted.' : 'mw-watch: sound and notifications on.' }
}

const addBoard = async ($: any, raw: string) => {
  if (raw.trim() === '') return { text: 'mw-watch: usage /mw-watch add PATH (a folder holding an mw board).' }
  const bin = await resolveMw($)
  if (bin === undefined) return { text: `mw-watch: ${MW_HOWTO}` }
  const board = absPath(raw)
  if (!isAbsolute(board)) return { text: 'mw-watch: the path must be absolute (the session folder is unknown).' }
  let isBoard = true
  try {
    isBoard = await $.fs.exists(`${board.replace(/[\\/]+$/, '')}/.miniwaves.json`)
  } catch {
    isBoard = true
  }
  if (!isBoard) return scanCommand($, board)
  const id = baseName(board)
  if (id === '') return { text: `mw-watch: no folder name in "${raw.trim()}".` }
  let items: TidewatchItem[] | undefined
  try {
    const r = await $.process.run([bin, 'list', '-A', '-j'], { cwd: board, timeoutMs: 20_000 })
    items = r.exitCode === 0 ? asItems(parseJson(r.stdout)) : undefined
  } catch {
    items = undefined
  }
  if (items === undefined) return { text: `mw-watch: ${board} is not an mw board (mw list failed there).` }
  const reg = await readRegistry($)
  const clash = reg.jobs.find(j => j.id === id && j.board !== board)
  if (clash !== undefined) return { text: `mw-watch: id "${id}" is already used by ${clash.board}; remove it first.` }
  const tenants = [...new Set(items.map(i => i.tenant).filter(t => t !== ''))].sort()
  const prev = reg.jobs.find(j => j.id === id)
  const job: TidewatchJob = { id, title: prev?.title ?? id, board, tenants, news: prev?.news ?? '', newsFile: prev?.newsFile ?? '', summary: prev?.summary ?? '' }
  await writeRegistry($, [...reg.jobs.filter(j => j.id !== id), job])
  const loaded = await loadRegistry($)
  await select($, loaded.jobs.find(j => j.boardId === id)?.id ?? id)
  await openJobs($)
  await refreshAll($)
  return { text: `mw-watch: ${prev ? 'updated' : 'added'} ${id} (${tenants.length} tenant${tenants.length === 1 ? '' : 's'}).` }
}

// ---- discovery: `mw boards` finds the boards and tenants, so no setup is needed ----

type BoardsReport = { boards: { dir: string; name: string; tenants: { slug: string; archived: boolean }[]; error?: string }[] }
const NO_BOARDS_CMD = 'mw-watch: update mw for auto-discovery (mw update).'

const homeDir = async ($: any): Promise<string> =>
  (((await $.env.get('HOME').catch(() => undefined)) as string | undefined) ?? ((await $.env.get('USERPROFILE').catch(() => undefined)) as string | undefined) ?? '')

/** Runs `mw boards DIR ... -j`; `unsupported` when mw is missing or too old to know the command. */
const discover = async ($: any, dir: string, flags: string[]): Promise<{ unsupported: boolean; found: TidewatchFound[] }> => {
  const r = await runMw($, dir, ['boards', dir, ...flags, '-j'], 10_000)
  const rep = r === undefined || r.exitCode !== 0 ? undefined : (parseJson(r.stdout) as BoardsReport | undefined)
  if (rep === undefined || rep === null || !Array.isArray(rep.boards)) return { unsupported: true, found: [] }
  const found = rep.boards
    .filter(b => typeof b?.dir === 'string' && isAbsolute(b.dir) && (b.error === undefined || b.error === ''))
    .map(b => ({ dir: b.dir, name: str(b.name ?? ''), tenants: (b.tenants ?? []).filter(t => t.archived !== true).map(t => str(t.slug)) }))
  return { unsupported: false, found }
}

/** Adds every found board that is not registered yet; returns the new ids. */
const addFound = async ($: any, boards: TidewatchFound[]): Promise<string[]> => {
  const reg = await readRegistry($)
  const jobs = [...reg.jobs]
  const added: string[] = []
  for (const b of boards) {
    if (jobs.some(j => j.board === b.dir)) continue
    const base = baseName(b.dir) || 'board'
    let id = base
    for (let n = 2; jobs.some(j => j.id === id); n++) id = `${base}-${n}`
    jobs.push({ id, title: id, board: b.dir, tenants: [...b.tenants].sort(), news: '', newsFile: '', summary: '' })
    added.push(id)
  }
  if (added.length === 0) return added
  await writeRegistry($, jobs)
  const loaded = await loadRegistry($)
  const cur: string = await read($, selected)
  if (!loaded.jobs.some(j => j.id === cur)) await select($, loaded.jobs[0]!.id)
  await refreshFound($)
  return added
}

const refreshFound = async ($: any) => {
  const reg = await readRegistry($)
  await update($, foundAtom, (f: TidewatchFound[]) => f.filter(b => !reg.jobs.some(j => j.board === b.dir)))
}

const listIds = (n: number) => `${n} board${n === 1 ? '' : 's'}`

// ---- the boards this session used: a read-only tool.call hook plus a backfill from the transcript ----

/** Folders a session touched that may hold a board; resolved lazily through `mw boards`. */
const usedDirs = new Set<string>()

const MW_WORD = /(^|[\s;&|(`])(?:\S*\/)?mw(?=\s|$)/

/** Folders a tool call points at: a Bash command that runs mw (its cd target, -C arg and absolute paths) or a file under .miniwaves(.json). Pure; used by the hook and the backfill. */
export const dirsOfToolUse = (tool: string, input: Record<string, unknown>, cwd: string): string[] => {
  const out: string[] = []
  if (tool === 'Bash' && typeof input.command === 'string' && MW_WORD.test(input.command)) {
    const cmd = input.command
    const cd = /(?:^|[;&|]\s*|&&\s*)cd\s+("[^"]+"|'[^']+'|\S+)/.exec(cmd)
    const dashC = [...cmd.matchAll(/(?:^|\s)-C\s+("[^"]+"|'[^']+'|\S+)/g)]
    // mw runs in the cd target, else in the session folder; an explicit -C names the board instead
    if (cd) out.push(cd[1]!.replace(/^["']|["']$/g, ''))
    else if (dashC.length === 0) out.push(cwd)
    for (const m of dashC) out.push(m[1]!.replace(/^["']|["']$/g, ''))
    for (const m of cmd.matchAll(/(?:^|\s)(\/[^\s"';&|]+)/g)) out.push(m[1]!)
  }
  if (tool === 'Read' || tool === 'Edit' || tool === 'Write' || tool === 'MultiEdit') {
    const f = input.file_path
    const m = typeof f === 'string' ? /^(.*?)\/\.miniwaves(?:\.json|\/)/.exec(f) : null
    if (m) out.push(m[1] === '' ? '/' : m[1]!)
  }
  return out.filter(d => d !== '' && isAbsolute(d)).map(d => d.replace(/\/+$/, '') || '/')
}

const noteUse = (tool: string, input: Record<string, unknown>) => {
  try {
    for (const d of dirsOfToolUse(tool, input, sessionCwd)) usedDirs.add(d)
  } catch {
    // an observer never fails a call
  }
}

const realDir = async ($: any, dir: string) => {
  try {
    const st = await $.fs.stat(dir, { resolve: true })
    return typeof st?.realPath === 'string' && st.realPath !== '' ? st.realPath : dir
  } catch {
    return dir
  }
}

/**
 * THE board finder (one function, same logic as the mw-jobs pane): tier 1 every board this
 * session used (live hook calls, subagents included, plus the transcript before the plugin loaded),
 * each resolved to its board root and deduped by real path, marked `session` and listed first;
 * tier 2 the session folder (and project root) looked at with --up --depth 3; tier 3, only when both are empty (and not `cheap`) and discover.home is on, the home folder at discover.depth (default 4).
 */
export const findSessionBoards = async ($: any, opts: { cheap?: boolean } = {}): Promise<{ unsupported: boolean; found: TidewatchFound[] }> => {
  const found: TidewatchFound[] = []
  const seen = new Set<string>()
  const take = async (list: TidewatchFound[], session: boolean) => {
    for (const b of list) {
      const real = await realDir($, b.dir)
      if (seen.has(real)) continue
      seen.add(real)
      found.push({ ...b, session })
    }
  }
  // tier 1
  // the transcript is older than the live hook calls, so its folders come first
  const dirs = new Set<string>()
  try {
    const msgs = (await $.session.messages()) as { toolUses?: { tool: string; input: Record<string, unknown> }[] }[]
    if (Array.isArray(msgs)) {
      for (const m of msgs) for (const u of m.toolUses ?? []) for (const d of dirsOfToolUse(u.tool, u.input ?? {}, sessionCwd)) dirs.add(d)
    }
  } catch {
    // no transcript to read: only live calls count
  }
  for (const d of usedDirs) dirs.add(d)
  for (const d of dirs) {
    const r = await discover($, d, ['--up', '--depth', '0'])
    await take(r.found, true)
  }
  // tier 2
  const roots = [sessionCwd]
  try {
    const root = (await $.session.root()) as string
    if (typeof root === 'string' && root !== '' && !roots.includes(root)) roots.push(root)
  } catch {
    // the session folder alone
  }
  for (const d of roots.filter(x => x !== '')) {
    const r = await discover($, d, ['--up', '--depth', '3'])
    if (r.unsupported) return { unsupported: true, found: [] }
    await take(r.found, false)
  }
  // tier 3
  const us: UserSettings = await read($, userSettings)
  if (found.length === 0 && opts.cheap !== true && us.discoverHome) {
    const home = await homeDir($)
    if (home !== '') {
      const r = await discover($, home, ['--depth', String(us.discoverDepth)])
      if (r.unsupported) return { unsupported: true, found: [] }
      await take(r.found, false)
    }
  }
  return { unsupported: false, found }
}

/** The whole setup: an empty registry takes every board the session used, then the session folder's, then the home folder's. */
const autoDiscover = async ($: any): Promise<string> => {
  const res = await findSessionBoards($)
  if (res.unsupported) {
    $.ui.toast('mw-watch: update mw for auto-discovery (mw update).', { timeoutMs: 8000 })
    return NO_BOARDS_CMD
  }
  const used = new Set(res.found.filter(b => b.session).map(b => b.dir))
  const added = await addFound($, res.found)
  if (added.length === 0) return 'mw-watch: no boards found. Add one with /mw-watch add PATH or /mw-watch scan DIR.'
  const reg = await readRegistry($)
  const label = (id: string) => (used.has(reg.jobs.find(j => j.id === id)?.board ?? '') ? `${id} [this session]` : id)
  return `mw-watch: found ${listIds(added.length)} (${added.map(label).join(', ')}).`
}

/** Session start: the cheap sources only (boards the session used, the session folder), never the home folder. */
const sessionDiscover = async ($: any) => {
  const res = await findSessionBoards($, { cheap: true })
  if (!res.unsupported) await addFound($, res.found)
}

/** The registry, after zero-setup discovery when it is empty, so "no boards yet" is only said when discovery found nothing. */
const registryWithDiscovery = async ($: any): Promise<TidewatchRegistry> => {
  const reg = await loadRegistry($)
  if (reg.jobs.length > 0) return reg
  await autoDiscover($)
  return loadRegistry($)
}

const scanCommand = async ($: any, dir: string): Promise<{ text: string }> => {
  const root = dir === '' ? await homeDir($) : absPath(dir)
  if (root === '' || !isAbsolute(root)) return { text: 'mw-watch: usage /mw-watch scan [DIR] (an absolute folder).' }
  const res = await discover($, root, ['--depth', '4'])
  if (res.unsupported) {
    $.ui.toast('mw-watch: update mw for auto-discovery (mw update).', { timeoutMs: 8000 })
    return { text: NO_BOARDS_CMD }
  }
  const reg = await readRegistry($)
  const fresh = res.found.filter(b => !reg.jobs.some(j => j.board === b.dir))
  await update($, foundAtom, () => fresh)
  await openJobs($)
  if (fresh.length === 0) return { text: `mw-watch: no new boards under ${root}.` }
  return { text: `mw-watch: found ${listIds(fresh.length)} not added yet: ${fresh.map(b => b.dir).join(', ')}. Press Add in the pane.` }
}

const toggleTenant = async ($: any, boardId: string, tenant: string): Promise<{ text: string }> => {
  const reg = await readRegistry($)
  const b = reg.jobs.find(j => j.id === boardId)
  if (b === undefined || !b.tenants.includes(tenant)) return { text: `mw-watch: unknown board or tenant "${boardId}" "${tenant}".` }
  const hidden = b.hidden ?? []
  const next = hidden.includes(tenant) ? hidden.filter(t => t !== tenant) : [...hidden, tenant]
  if (b.tenants.every(t => next.includes(t))) return { text: 'mw-watch: keep at least one tenant shown.' }
  await writeRegistry($, reg.jobs.map(j => (j.id === boardId ? { ...j, hidden: next } : j)))
  await loadRegistry($)
  await refreshAll($)
  return { text: `mw-watch: ${tenant} ${next.includes(tenant) ? 'hidden' : 'shown'} on ${boardId}.` }
}

const foundBlock = ($: any, found: TidewatchFound[], Box: any, Text: any, Button: any) => (
  <Box flexDirection="column">
    <Text dimColor>{`Found ${found.length} board${found.length === 1 ? '' : 's'} not added yet:`}</Text>
    {found.map(b => (
      <Box key={`found-${b.dir}`} gap={1}>
        <Button key={`add-${b.dir}`} label="Add" onPress={() => void addFound($, [b]).then(() => openJobs($))} />
        <Text wrap="truncate-end">{`${b.session === true ? '[this session] ' : ''}${baseName(b.dir)} (${b.tenants.length} tenant${b.tenants.length === 1 ? '' : 's'}) ${b.dir}`}</Text>
      </Box>
    ))}
    {found.length > 1 && <Button key="add-all" label="Add all" onPress={() => void addFound($, found).then(() => openJobs($))} />}
  </Box>
)

const removeBoard = async ($: any, id: string) => {
  const reg = await readRegistry($)
  if (!reg.jobs.some(j => j.id === id)) {
    return { text: `mw-watch: unknown job "${id}"${reg.jobs.length > 0 ? ` (${reg.jobs.map(j => j.id).join(', ')})` : ''}.` }
  }
  await writeRegistry($, reg.jobs.filter(j => j.id !== id))
  const drop = <T,>(m: Record<string, T>) => Object.fromEntries(Object.entries(m).filter(([k]) => k !== id && !k.startsWith(`${id}/`)))
  await update($, snapshots, drop)
  await update($, previous, drop)
  await update($, staleSince, drop)
  await update($, seen, drop)
  await saveSeen($)
  await loadRegistry($)
  return { text: `mw-watch: removed ${id}.` }
}

const styleCommand = async ($: any, rest: string) => {
  const [verb = '', a = '', b = ''] = rest.split(/\s+/)
  const names = await refreshStyleNames($)
  if (verb === 'list' || verb === '') {
    return { text: `mw-watch styles: ${names.join(', ')} (current ${await read($, styleName)}).` }
  }
  if (verb === 'new') {
    if (!isStyleName(a)) return { text: 'mw-watch: usage /mw-watch style new NAME (lowercase letters, digits, hyphens; up to 32).' }
    if (names.includes(a)) return { text: `mw-watch: style "${a}" already exists.` }
    await $.store.set('userStyles', { ...(await readUserStyles($)), [a]: JSON.parse(JSON.stringify(NORMAL)) })
    await refreshStyleNames($)
    return { text: `mw-watch: style "${a}" stored as a copy of normal. Edit it with /mw-watch style export ${a} PATH, then /mw-watch style import PATH.` }
  }
  if (verb === 'import') {
    const path = rest.slice('import'.length).trim()
    if (path === '') return { text: 'mw-watch: usage /mw-watch style import PATH (a JSON file named <style-name>.json).' }
    if (path.startsWith('~')) return { text: 'mw-watch: use an absolute path (~ is not expanded).' }
    const file = absPath(path)
    const name = baseName(file).replace(/\.json$/i, '')
    if (!isStyleName(name) || BUILTIN_NAMES.includes(name)) {
      return { text: `mw-watch: "${name}" is not a usable style name (lowercase letters, digits, hyphens; not normal or blocks). Rename the file.` }
    }
    let parsed: unknown
    try {
      parsed = JSON.parse(await $.fs.read(file))
    } catch {
      return { text: `mw-watch: could not read ${file} as JSON.` }
    }
    const { issues } = mergeStyle(NORMAL, parsed)
    await $.store.set('userStyles', { ...(await readUserStyles($)), [name]: parsed })
    await refreshStyleNames($)
    await resolveStyle($, name)
    return { text: `mw-watch: imported style "${name}"${issues.length > 0 ? ` (${issues.length} problem${issues.length === 1 ? '' : 's'} ignored)` : ''}. Use /mw-watch style ${name}.` }
  }
  if (verb === 'export') {
    const path = rest.replace(/^export\s+\S+\s*/, '').trim()
    if (!names.includes(a) || path === '') return { text: 'mw-watch: usage /mw-watch style export NAME PATH.' }
    if (path.startsWith('~')) return { text: 'mw-watch: use an absolute path (~ is not expanded).' }
    const body = BUILTIN[a] ?? (await readUserStyles($))[a]
    const file = absPath(path)
    try {
      await $.fs.write(file, `${JSON.stringify(body, null, 2)}\n`)
    } catch {
      return { text: `mw-watch: could not write ${file}.` }
    }
    void b
    return { text: `mw-watch: style "${a}" written to ${file}.` }
  }
  if (!names.includes(verb)) return { text: `mw-watch: unknown style "${verb}" (${names.join(', ')}).` }
  await setStyle($, verb)
  return { text: `mw-watch: style ${verb}.` }
}

/** One command line: `/mw-watch [job | mute | every X | bigger|smaller|reset | style ... | add PATH | remove ID]`. */
const runCommand = async ($: any, args: string) => {
  const word = args.trim()
  await syncSettings($)
  if (word === 'mute') return toggleMute($)
  if (word === 'fanfare' || word.startsWith('fanfare ')) return fanfareCommand($, word.slice(7))
  if (word === 'style' || word.startsWith('style ')) return styleCommand($, word.slice(5).trim())
  if (word === 'add' || word.startsWith('add ')) return addBoard($, word.slice(3))
  if (word === 'scan' || word.startsWith('scan ')) return scanCommand($, word.slice(4).trim())
  const hd = /^hide\s+(\S+)\s+(\S+)$/.exec(word)
  if (hd) return toggleTenant($, hd[1]!, hd[2]!)
  const nw = /^news(?:\s+(.*))?$/.exec(word)
  if (nw) return openNews($, (nw[1] ?? '').trim())
  const rm = /^remove(?:\s+(.*))?$/.exec(word)
  if (rm) return removeBoard($, (rm[1] ?? '').trim())
  if (word === 'bigger' || word === 'smaller' || word === 'reset') {
    await stepBy($, word === 'bigger' ? 1 : word === 'smaller' ? -1 : 0)
    return { text: `mw-watch: size ${word === 'reset' ? 'default' : word}.` }
  }
  const m = /^every\s+(off|15s|30s|1m|5m)$/.exec(word)
  if (m) {
    await setEvery($, m[1]!)
    return { text: `mw-watch: auto-refresh ${m[1]}.` }
  }
  const st = /^stalled\s+(\d+)$/.exec(word)
  if (st) {
    await update($, stalledMin, () => Number(st[1]))
    await persist($, 'stalledMin', Number(st[1]))
    return { text: `mw-watch: stalled after ${st[1]} min.` }
  }
  const reg = await loadRegistry($)
  if (word !== '') {
    const hit = findJob(reg.jobs, word)
    if (hit !== undefined) await select($, hit.id)
    else return { text: `mw-watch: unknown job "${word}"${reg.jobs.length > 0 ? ` (${reg.jobs.map(j => j.id).join(', ')})` : ''}. Add a board with /mw-watch add PATH.` }
  }
  let note = ''
  if (reg.jobs.length === 0 && word === '') note = await autoDiscover($)
  await openJobs($)
  void refreshAll($)
  const after: TidewatchRegistry = await read($, registry)
  if (after.jobs.length === 0) return { text: note !== '' ? note : 'mw-watch: no boards yet. Add one with /mw-watch add PATH.' }
  return { text: note !== '' ? note : 'mw-watch pane opened.' }
}

const ALIAS_HINT = '`/tidewatch` is now `/mw-watch` (the old name goes in the next release).'
const IMPORT_KEYS = ['registry', 'refreshEvery', 'seen', 'fontStep', 'style', 'stalledMin', 'selectedJob', 'userStyles', 'notified']

/** Renamed from tidewatch: on the first start with an empty registry, copy the old plugin's per-user store (best effort; the store file name carries a hash, so the shell finds it). */
const importOldStore = async ($: any) => {
  try {
    const cur = (await $.store.get('registry')) as { jobs?: unknown } | undefined
    if (cur?.jobs !== undefined && cur.jobs !== null && Object.keys(cur.jobs as object).length > 0) return
    const isWin = (await $.env.get('OS').catch(() => undefined)) === 'Windows_NT'
    const argv = isWin
      ? ['powershell.exe', '-NoProfile', '-Command', "$f = Get-ChildItem \"$env:USERPROFILE\\.claude\\plugins\\store\\tidewatch_*.json\" -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1; if ($f) { Get-Content -Raw $f.FullName }"]
      : ['/bin/sh', '-c', 'f=$(ls -t "$HOME"/.claude/plugins/store/tidewatch_*.json 2>/dev/null | head -n 1); [ -n "$f" ] && cat "$f"']
    const r = await $.process.run(argv, { timeoutMs: 5000 })
    if (r.exitCode !== 0 || String(r.stdout ?? '').trim() === '') return
    const old = JSON.parse(String(r.stdout)) as Record<string, unknown>
    for (const k of IMPORT_KEYS) {
      if (old[k] !== undefined && (await $.store.get(k)) === undefined) await $.store.set(k, old[k])
    }
  } catch {
    // no old store, or unreadable: start empty
  }
}

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    await $.command.register({ name: 'mw-watch', description: 'Live mw boards: /mw-watch [job | mute | every off|15s|30s|1m|5m | bigger|smaller|reset | style NAME|list|new NAME|import PATH|export NAME PATH | add PATH | remove ID | fanfare on|off|next|use NAME|test]' })
    await $.command.register({ name: 'mw-fanfare', description: 'Fanfare: /mw-fanfare [on | off | next | use NAME | test]' })
    await $.command.register({ name: 'tidewatch', description: 'Deprecated alias of /mw-watch (same subcommands); the old name goes in the next release.' })
    sessionCwd = String((e as { cwd?: string }).cwd ?? '')
    usedDirs.clear()
    await importOldStore($)
    try {
      const saved = await $.store.get('refreshEvery')
      if (typeof saved === 'string' && (EVERY as readonly string[]).includes(saved)) {
        await update($, refreshEvery, () => saved)
      }
      const savedSeen = await $.store.get('seen')
      if (savedSeen !== null && typeof savedSeen === 'object') await update($, seen, () => savedSeen as Record<string, TidewatchSeen>)
      const savedStep = await $.store.get('fontStep')
      if (typeof savedStep === 'number') await update($, fontStep, () => Math.max(-1, Math.min(1, savedStep)))
      const savedStyle = await $.store.get('style')
      if (typeof savedStyle === 'string' && isStyleName(savedStyle)) await update($, styleName, () => savedStyle)
      const savedStall = await $.store.get('stalledMin')
      if (typeof savedStall === 'number') await update($, stalledMin, () => savedStall)
      const sel = await $.store.get('selectedJob')
      if (typeof sel === 'string') await update($, selected, () => sel)
    } catch {
      // defaults
    }
    await resolveMw($)
    const us = await loadUserSettings($)
    if (us.refresh !== undefined && (await savedEvery($)) === undefined) await update($, refreshEvery, () => us.refresh!)
    await loadRegistry($)
    if ((await read($, registry)).jobs.length === 0) await sessionDiscover($)
    void openJobs($)
    await arm($)
    await refreshAll($)

    return next(e)
  })

  on('command.run', { command: 'mw-watch' }, async ($, e) => runCommand($, String((e as { args?: string }).args ?? '')))
  // observe only: records which boards the session touches, never changes or blocks a call
  on('tool.call', async ($, e, next) => {
    noteUse(String((e as { tool?: string }).tool ?? ''), e as unknown as Record<string, unknown>)
    return next(e)
  })

  on('command.run', { command: 'mw-fanfare' }, async ($, e) => fanfareSlash($, String((e as { args?: string }).args ?? '')))
  on('ui.render', { component: 'Pane', requestId: FAN_PANE }, async ($, e) => {
    const { Box, Text, Button } = $.ui.resolve(e) as any
    const reg = await read($, registry)
    const sel = await read($, selected)
    const job = reg.jobs.find(j => j.id === sel) ?? reg.jobs[0]
    const fan = job === undefined ? undefined : ((await read($, fanfareAtom)) as Record<string, TidewatchFanfare>)[job.board]
    if (job === undefined || fan === undefined) return <Text dimColor>No fanfare settings (run /mw-watch first; needs mw 0.3).</Text>
    return (
      <Box flexDirection="column">
        <Text bold>Fanfare</Text>
        <Text>{`${fan.enabled ? 'on' : 'off'}, set ${fan.use}`}</Text>
        <Text dimColor>{`Sets: all, ${fan.sets.join(', ')}`}</Text>
        <Box gap={1}>
          <Button key="fanfare" label={`Fanfare: ${fanfareLabel(fan)}`} onPress={() => void fanfareCycle($, job.board)} />
        </Box>
        <Text dimColor>/mw-fanfare test plays it once; /mw-fanfare on|off|use NAME.</Text>
      </Box>
    )
  })
  on('command.run', { command: 'tidewatch' }, async ($, e) => {
    const r = await runCommand($, String((e as { args?: string }).args ?? ''))
    return { ...r, text: `${ALIAS_HINT}\n${r.text}` }
  })

  on('ui.render', { component: 'Pane', requestId: NEWS_PANE }, async ($, e) => {
    const { Box, Text, Link, Button, Markdown } = $.ui.resolve(e) as any
    const reg = await read($, registry)
    const sel = await read($, selected)
    const job = reg.jobs.find(j => j.id === sel) ?? reg.jobs[0]
    if (job === undefined) {
      return (
        <Box flexDirection="column">
          <Text dimColor>No boards yet. Add one with /mw-watch add PATH.</Text>
        </Box>
      )
    }
    const entry = (await read($, newsAtom))[job.id] as { path: string; text: string; isCut: boolean; isMissing: boolean } | undefined
    return (
      <Box flexDirection="column">
        <Box alignItems="center" gap={1}>
          <Text bold wrap="truncate-end">{newsTitle(job.name || job.id)}</Text>
          <Box flexGrow={1} />
          <Button key="refresh-news" label="Refresh" onPress={() => void loadNews($, job)} />
          <Button key="jobs" label="Jobs" dimColor onPress={() => void openJobs($)} />
        </Box>
        {entry === undefined && <Text dimColor>Loading…</Text>}
        {entry?.isMissing === true && (
          <Box flexDirection="column">
            <Text>No news page yet.</Text>
            <Text dimColor>{`mw-watch reads ${entry.path} and never runs anything to make it.`}</Text>
            <Text dimColor>Have mw write it there (mw news --format md -o PATH), or set newsFile for this job in the registry.</Text>
          </Box>
        )}
        {entry !== undefined && !entry.isMissing && (Markdown !== undefined ? <Markdown text={entry.text} /> : <Text>{entry.text}</Text>)}
        {entry?.isCut === true && <Text color="warning">{`Truncated: showing the first ${NEWS_MAX} characters. Open the file for the rest.`}</Text>}
        {job.news !== '' && <Link href={job.news}>News page ↗</Link>}
      </Box>
    )
  })

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const { Box, Text, Link, Button, Svg } = $.ui.resolve(e) as any
    const reg = await read($, registry)
    // Text `size`, `weight` and `mono` are refused on the terminal and desktop tables of this build:
    // sz() and mo add nothing and wt is plain bold
    const wt: any = { bold: true }
    const mo: any = {}
    const sz = (_v: string): any => ({})

    if (reg.jobs.length === 0) {
      const found: TidewatchFound[] = await read($, foundAtom)
      const hasMw = (await resolveMw($)) !== undefined
      return (
        <Box flexDirection="column">
          <Text {...wt}>mw-watch</Text>
          {reg.error !== null && <Text color="warning">{reg.error}</Text>}
          {!hasMw && <Text color="warning">{MW_HOWTO}</Text>}
          <Text dimColor>mw-watch shows the state of your miniwaves boards.</Text>
          <Text>No boards yet. Run /mw-watch to look for them, or /mw-watch scan DIR.</Text>
          <Text dimColor>/mw-watch add PATH adds one folder (a folder with no board is scanned instead).</Text>
          {found.length > 0 && foundBlock($, found, Box, Text, Button)}
        </Box>
      )
    }

    const sel = await read($, selected)
    const job = reg.jobs.find(j => j.id === sel) ?? reg.jobs[0]!
    const snaps = await read($, snapshots)
    const stale = (await read($, staleSince))[job.id]
    const every = await read($, refreshEvery)
    const isBusy = (await read($, busy)).includes(job.id)
    const cursors = await read($, seen)
    const stallMin = await read($, stalledMin)
    const now: number = await $.clock.now()
    const snap = snaps[job.id]
    const given = Number((e.props as { bodyColumns?: number }).bodyColumns)
    const hasWidth = Number.isFinite(given) && given > 0
    const columns = hasWidth ? Math.max(20, Math.min(100, given)) : 40
    // Text has no font size here, so the +/-/# buttons change density
    const wanted: string = await read($, styleName)
    const st: Style = ((await read($, styles)) as Record<string, Style>)[wanted] ?? NORMAL
    // columns from 50 body columns, stacked below
    const rowsLayout = Number((e.props as { bodyColumns?: number }).bodyColumns) >= 50 ? 'columns' : 'stacked'
    const step: number = await read($, fontStep)
    const density = step < 0 ? 'compact' : step > 0 ? 'roomy' : 'default'

    const needsOf = (id: string) => (snaps[id] ? needItems(snaps[id]!).length : 0)
    const fan = ((await read($, fanfareAtom)) as Record<string, TidewatchFanfare>)[job.board]
    const headerRow = (
      <Box alignItems="center" gap={1}>
        {reg.jobs.length < 2 && <Text {...wt} wrap="truncate-end">{job.title}</Text>}
        {reg.jobs.length >= 2 && reg.jobs.map(j => (
          <Box key={`tab-${j.id}`}>
            {needsOf(j.id) > 0 ? <Text color="error" bold>! </Text> : <Text> </Text>}
            <Button
              key={`job-${j.id}`}
              plain
              label={j.name ? j.title : j.id}
              variant={j.id === job.id ? 'primary' : 'secondary'}
              dimColor={j.id !== job.id}
              onPress={() => void select($, j.id)}
            />
          </Box>
        ))}
        <Box flexGrow={1} />
        {isBusy && <Text dimColor>refreshing…</Text>}
        <Button key="refresh" label="Refresh" onPress={() => void refreshSelected($)} />
        <Button key="news" label="News" dimColor onPress={() => void openNews($, job.id)} />
        <Button key="every" label={`Auto: ${every}`} dimColor onPress={() => void cycleEvery($)} />
        {fan !== undefined && <Button key="fanfare" label={`Fanfare: ${fanfareLabel(fan)}`} dimColor onPress={() => void fanfareCycle($, job.board)} />}
        <Button
          key="style"
          label={`Style: ${wanted}`}
          dimColor
          onPress={() => void cycleStyle($)}
        />
        <Button key="smaller" label=" − " dimColor onPress={() => void stepBy($, -1)} />
        <Button key="reset" label=" # " dimColor onPress={() => void stepBy($, 0)} />
        <Button key="bigger" label=" + " dimColor onPress={() => void stepBy($, 1)} />
      </Box>
    )
    const hiddenOf = job.hidden ?? []
    const allOf = job.all ?? []
    const found: TidewatchFound[] = await read($, foundAtom)
    const header = (
      <Box flexDirection="column">
        {headerRow}
        {allOf.length >= 2 && (
          <Box gap={1}>
            <Text dimColor>Tenants:</Text>
            {allOf.map(t => (
              <Button key={`tenant-${t}`} plain label={`${hiddenOf.includes(t) ? '○' : '●'} ${t}`} dimColor={hiddenOf.includes(t)} onPress={() => void toggleTenant($, job.boardId ?? job.id, t)} />
            ))}
          </Box>
        )}
        {found.length > 0 && foundBlock($, found, Box, Text, Button)}
      </Box>
    )
    const separator = <Text dimColor wrap="truncate-end">{'─'.repeat(columns)}</Text>
    // buttons only: no Select or Input on mobile
    const footer = null

    if (snap === undefined) {
      return (
        <Box flexDirection="column">
          {header}
          {separator}
          <Text dimColor>{stale === undefined ? 'Loading…' : `mw unavailable since ${hhmm(stale)}`}</Text>
          {footer}
        </Box>
      )
    }

    const cursor = cursors[job.id] as TidewatchSeen | undefined
    const seenTs = cursor?.ts ?? now
    const a = analyse(snap, now, seenTs, stallMin * 60_000)
    // a pane never draws more rows than this per section (the counts above still say how many there are)
    const CAP = 30
    const shown = { needs: a.needs.slice(0, CAP), review: a.review.slice(0, CAP), doing: a.doing.slice(0, CAP) }
    const order = st.sections.order.filter(id => st.sections.show[id])
    const head = headline(a)
    const labels = tenantLabels(job.tenants.length > 0 ? job.tenants : [...new Set(snap.waves.map(w => w.id.split(':')[0] ?? ''))])
    const waves = snap.waves.filter(w => !isPlaceholder(w))
    const openWaves = waves.filter(w => w.state !== 'done')
    const closedWaves = waves.filter(w => w.state === 'done')
    const newlyClosed = closedWaves.filter(w => a.changed.has(w.id))
    const folded = closedWaves.filter(w => !a.changed.has(w.id))
    const readyCount = snap.items.filter(i => i.state === 'ready').length
    const events = [...snap.events].sort((x, y) => Date.parse(y.ts) - Date.parse(x.ts)).slice(0, density === 'compact' ? 4 : density === 'roomy' ? 10 : 6)
    // one bar length for every wave, style and layout: a single function of the body width
    const barLen = barLength(hasWidth ? given : undefined)
    // one line only where name, bar and count fit; blocks always take two
    const twoLine = rowsLayout === 'stacked'
    const itemAge = (i: TidewatchItem) => {
      const t = Date.parse(i.updated)
      return Number.isNaN(t) ? '' : ageText(now - t)
    }
    const label = (text: string, color?: string) => (
      <Text {...sz("caption")} {...wt} color={color} dimColor={color === undefined}>{text.toUpperCase()}</Text>
    )
    const pid = (tenant: string, id: string) => (labels[tenant] ? `${labels[tenant]}·${id}` : id)
    const dot = (key: string) => (a.changed.has(key) ? <Text color="suggestion">●</Text> : <Text> </Text>)
    // one grid for the whole pane: name | status column (as wide as a bar) | right column
    const closedShown = [...openWaves, ...newlyClosed]
    const waveRows = closedShown.map(w => ({
      w,
      name: `${w.state === 'done' ? '✓ ' : ''}${`${waveName(w, labels)} ${w.title.split(' ').slice(1).join(' ')}`.trim()}`,
    }))
    const nameLens = [
      ...shown.needs.map(i => `! ${i.title}`.length),
      ...shown.review.map(i => `${pid(i.tenant, i.id)} ${i.title}`.length),
      ...shown.doing.map(d => `${pid(d.item.tenant, d.item.id)} ${d.item.title}`.length),
      ...waveRows.map(r => r.name.length),
      ...snap.harness.map(h => h.name.length),
    ]
    const rightLens = [
      3,
      2,
      ...closedShown.map(w => w.members.length),
      ...shown.needs.map(i => itemAge(i).length),
      ...shown.review.map(i => itemAge(i).length),
      ...shown.doing.map(d => (d.ageMs === undefined ? 0 : ageText(d.ageMs).length)),
    ]
    const deltaText = (row: TidewatchHarness) => {
      const base = cursor?.harness[row.name]
      const d = (n: number) => (n === 0 ? '' : `${n > 0 ? '▲' : '▼'}${Math.abs(n)}`)
      return [d(base !== undefined ? row.pass - base.pass : 0), d(base !== undefined ? row.fail - base.fail : 0)] as const
    }
    rightLens.push(...snap.harness.map(h => deltaText(h).join(' ').length))
    const rightW = Math.max(...rightLens)
    // done / total as fixed Boxes: digits of a proportional font never line up as a plain string
    const doneWd = Math.max(1, ...closedShown.map(w => String(memberParts(w).done).length)) + 1
    const totalWt = Math.max(1, ...closedShown.map(w => String(memberParts(w).total).length)) + 1
    const countW = doneWd + 1 + totalWt
    const FIG = '\u2007'
    const countCell = (w: TidewatchWave) => {
      const { done, total } = memberParts(w)
      return (
        <Box key={`count-${w.id}`} width={countW} flexShrink={0} flexDirection="row">
          <Box key={`done-${w.id}`} width={doneWd} flexShrink={0} justifyContent="flex-end">
            <Text dimColor>{String(done).padStart(doneWd, FIG)}</Text>
          </Box>
          <Text dimColor>/</Text>
          <Box key={`total-${w.id}`} width={totalWt} flexShrink={0} justifyContent="flex-end">
            <Text dimColor>{String(total).padStart(totalWt, FIG)}</Text>
          </Box>
        </Box>
      )
    }
    const nameW = Math.max(6, Math.min(Math.max(4, ...nameLens), Math.floor(columns * 0.4), hasWidth ? columns - barLen - rightW - 4 : 99))
    const isCols = rowsLayout === 'columns'
    // the terminal is monospace: bars are counted in cells. Elsewhere the font is proportional, so flex sizes them.
    const isMono = e.surface === 'terminal'

    type Cell = { text?: string; color?: string; dim?: boolean; node?: unknown }
    const gridRow = (key: string, name: string, nameColor: string | undefined, status: Cell | undefined, right: unknown, marker: unknown) => {
      const statusNode = status === undefined ? null : status.node !== undefined ? (status.node as any) : (
        <Text color={status.color} dimColor={status.dim ?? status.color === undefined} wrap="wrap">{status.text}</Text>
      )
      const rightBox = (k?: string) => (
        <Box key={k} width={isMono ? Math.max(rightW, countW) : Math.max(6, countW)} flexShrink={0} flexGrow={0} justifyContent="flex-end">
          {right as any}
        </Box>
      )
      if (isCols) {
        return (
          <Box key={`row-${key}`} height={1} flexShrink={0} overflow="hidden">
            <Box key={`name-${key}`} width={isMono ? nameW : '38%'} flexShrink={0} flexGrow={0} overflow="hidden">
              <Text color={nameColor} wrap="truncate-end">{name}</Text>
            </Box>
            <Text> </Text>
            <Box key={`status-${key}`} width={isMono ? barLen : 0} minWidth={0} height={1} flexGrow={isMono ? 0 : 1} flexShrink={0} overflow="hidden">
              {statusNode}
            </Box>
            <Text> </Text>
            {rightBox(`right-${key}`)}
            {marker as any}
          </Box>
        )
      }
      return (
        <Box key={`row-${key}`} flexDirection="column" height={statusNode === null ? 1 : 2} flexShrink={0} overflow="hidden">
          <Box>
            <Box flexGrow={1} flexShrink={1} overflow="hidden">
              <Text color={nameColor} wrap="truncate-end">{name}</Text>
            </Box>
            {rightBox()}
            {marker as any}
          </Box>
          <Box key={`status-${key}`} flexGrow={isMono ? 0 : 1} width={isMono ? undefined : 0} height={1} overflow="hidden">
            {statusNode}
          </Box>
        </Box>
      )
    }
    const noteLine = (text: string) => (density === 'compact' || text === '' ? null : <Text dimColor wrap="truncate-end">{`  ${text}`}</Text>)
    const barCell = (w: TidewatchWave, isNew: boolean): Cell => {
      const glyphs = st.bar.kind === 'glyphs' ? [st.bar.fill, st.bar.rest] : ['█', '░']
      // the terminal has no light/dark: a user style's own colours apply as their dark value, the built-ins keep the theme's
      const fillColor = isCustomColor(wanted, st, 'progress') ? st.colors.progress.dark : undefined
      const newColor = isCustomColor(wanted, st, 'pass') ? st.colors.pass.dark : 'success'
      const restColor = isCustomColor(wanted, st, 'track') ? st.colors.track.dark : undefined
      if (isMono) {
        const filled = Math.round((Math.max(0, Math.min(100, w.percent)) / 100) * barLen)
        return {
          node: (
            <Box key={`bar-${w.id}`} flexShrink={0}>
              <Text color={isNew ? newColor : fillColor}>{glyphs[0]!.repeat(filled)}</Text>
              <Text color={restColor} dimColor={restColor === undefined}>{glyphs[1]!.repeat(barLen - filled)}</Text>
            </Box>
          ),
        }
      }
      return { text: '' }
    }
    const chipParts: string[] = []
    if (a.doneNew > 0) chipParts.push(`+${a.doneNew} done`)
    if (a.blockedNew > 0) chipParts.push(`+${a.blockedNew} blocked`)

    const summary = summaryParts(a, head, readyCount)

    if (!isMono && st.font === 'mono') {
      const tone = (c?: string) => (c === 'warning' ? 'wait' : c === 'error' ? 'needs' : c === 'success' ? 'pass' : 'dim')
      const mark = (k: string): MonoCell[] => (a.changed.has(k) ? [{ text: ' ●', tone: 'new' }] : [])
      const compact = density === 'compact'
      const blocks: MonoBlock[] = [
        { kind: 'head', text: head.text, tone: tone(head.tone) },
        { kind: 'line', parts: summary.flatMap((x, i) => [{ text: i > 0 ? ' · ' : '', tone: 'dim' }, { text: x.text, tone: tone(x.color) }]) },
      ]
      if (chipParts.length > 0) blocks.push({ kind: 'line', parts: [{ text: `● ${chipParts.join(' · ')} since ${hhmm(seenTs)}`, tone: 'fg' }] })
      const buckets: Record<string, MonoBlock[]> = { needs: [], review: [], progress: [], milestones: [], harness: [], events: [] }
      let bucket: MonoBlock[] = blocks
      const section = (title: string, titleTone: string | undefined, rows: MonoRow[], metas: string[]) => {
        bucket.push({ kind: 'gap' }, { kind: 'label', text: title, tone: titleTone })
        rows.forEach((r, i) => {
          bucket.push({ kind: 'rows', rows: [r] })
          if (!compact && metas[i]) bucket.push({ kind: 'line', parts: [{ text: `  ${metas[i]}`, tone: 'dim' }] })
        })
      }
      bucket = buckets.needs!
      if (shown.needs.length > 0) {
        section(
          'Needs you',
          'needs',
          shown.needs.map(i => ({ name: i.title, nameTone: 'needs', slot: { kind: 'text' as const, text: 'needs you', tone: 'needs' }, right: [{ text: itemAge(i) }, ...mark(`${i.tenant}:${i.id}`)] })),
          shown.needs.map(i => [i.tenant, i.id, i.note].filter(Boolean).join(' · ')),
        )
      }
      bucket = buckets.review!
      if (shown.review.length > 0) {
        section(
          'Review',
          'wait',
          shown.review.map(i => ({ name: `${pid(i.tenant, i.id)} ${i.title}`, slot: { kind: 'text' as const, text: 'awaiting review', tone: 'wait' }, right: [{ text: itemAge(i) }, ...mark(`${i.tenant}:${i.id}`)] })),
          shown.review.map(i => (i.note === 'awaiting review' ? '' : i.note)),
        )
      }
      bucket = buckets.progress!
      if (shown.doing.length > 0) {
        section(
          'In progress',
          undefined,
          shown.doing.map(d => ({
            name: `${pid(d.item.tenant, d.item.id)} ${d.item.title}`,
            slot: d.isStalled ? { kind: 'text' as const, text: 'stalled', tone: 'wait' } : undefined,
            right: [{ text: d.ageMs === undefined ? '' : ageText(d.ageMs), tone: d.isStalled ? 'wait' : 'dim' }, ...mark(`${d.item.tenant}:${d.item.id}`)],
          })),
          [],
        )
      }
      bucket = buckets.milestones!
      if (waves.length > 0) {
        const dw = Math.max(1, ...waveRows.map(r => String(memberParts(r.w).done).length))
        const tw = Math.max(1, ...waveRows.map(r => String(memberParts(r.w).total).length))
        const rows: MonoRow[] = waveRows.map(({ w, name }) => {
          const { done, total } = memberParts(w)
          const isNew = a.changed.has(w.id)
          return {
            name,
            nameTone: total === 0 ? 'dim' : 'fg',
            slot: total === 0 ? { kind: 'text', text: 'not started', tone: 'dim' } : w.state === 'done' ? { kind: 'text', text: '✓ closed', tone: 'pass' } : { kind: 'bar', pct: w.percent, isNew },
            right: [{ text: ` ${String(done).padStart(dw)}/${String(total).padEnd(tw)}`, tone: 'dim' }, ...(isNew ? [{ text: ' ●', tone: 'new' }] : [])],
          }
        })
        section('Milestones', undefined, rows, [])
        if (folded.length > 0) bucket.push({ kind: 'line', parts: [{ text: `✓ ${folded.length} closed — ${folded.map(w => waveName(w, labels)).join(', ')}`, tone: 'dim' }] })
      }
      bucket = buckets.harness!
      if (snap.harness.length > 0 && !compact) {
        const pw = Math.max(1, ...snap.harness.map(h => String(h.pass).length))
        const aw = Math.max(1, ...snap.harness.map(h => String(h.pass + h.fail + h.skip).length))
        const rows: MonoRow[] = snap.harness.map(h => {
          const [dp, df] = deltaText(h)
          const base = cursor?.harness[h.name]
          const pu = base !== undefined ? h.pass - base.pass : 0
          const fu = base !== undefined ? h.fail - base.fail : 0
          return {
            name: h.name,
            slot: { kind: 'stack', pass: h.pass, fail: h.fail, skip: h.skip },
            right: [
              { text: ` ${String(h.pass).padStart(pw)}/${String(h.pass + h.fail + h.skip).padEnd(aw)}`, tone: 'dim' },
              ...(dp !== '' ? [{ text: ` ${dp}`, tone: pu > 0 ? 'green' : 'needs' }] : []),
              ...(df !== '' ? [{ text: ` ${df}`, tone: fu > 0 ? 'fail' : 'pass' }] : []),
            ],
          }
        })
        section('Harness', undefined, rows, [])
        bucket.push({ kind: 'line', parts: [{ text: '█ pass', tone: 'pass' }, { text: '  ', tone: 'dim' }, { text: '█ fail', tone: 'fail' }, { text: '  ', tone: 'dim' }, { text: '█', tone: 'track' }, { text: ' skip · counts pass/total', tone: 'dim' }] })
      }
      bucket = buckets.events!
      if (events.length > 0) {
        bucket.push({ kind: 'gap' }, { kind: 'label', text: 'Last events' })
        for (const ev of events) {
          bucket.push({ kind: 'line', parts: [{ text: `${isoHhmm(ev.ts)}  ${ev.tenant} ${ev.item} ${ev.action}${ev.note ? ` — ${ev.note}` : ''}`, tone: 'dim' }] })
        }
      }
      for (const id of order) blocks.push(...buckets[id]!)
      blocks.push({ kind: 'gap' }, { kind: 'line', parts: [{ text: `updated ${hhmm(snap.updatedAt)} · auto ${every}`, tone: 'dim' }] })
      const mono = buildMonoSvg(blocks, st, step)
      return (
        <Box flexDirection="column">
          {header}
          {stale !== undefined && <Text color="warning">⏱ stale since {hhmm(stale)}</Text>}
          <Svg source={mono.source} alt={mono.alt} width={mono.width} />
          {job.news !== '' && <Link href={job.news}>News page ↗</Link>}
          {footer}
        </Box>
      )
    }

    if (!isMono) {
      const model: SvgModel = {
        head,
        summary,
        chip: chipParts.length > 0 ? `${chipParts.join(' · ')} since ${hhmm(seenTs)}` : '',
        needs: shown.needs.map(i => ({
          title: i.title,
          meta: [i.tenant, i.id, i.note].filter(Boolean).join(' · '),
          age: itemAge(i),
          isNew: a.changed.has(`${i.tenant}:${i.id}`),
        })),
        review: shown.review.map(i => ({
          id: pid(i.tenant, i.id),
          title: i.title,
          meta: i.note === 'awaiting review' ? '' : i.note,
          age: itemAge(i),
          isNew: a.changed.has(`${i.tenant}:${i.id}`),
        })),
        doing: shown.doing.map(d => ({
          id: pid(d.item.tenant, d.item.id),
          title: d.item.title,
          age: d.ageMs === undefined ? '' : d.isStalled ? `stalled ${ageText(d.ageMs)}` : ageText(d.ageMs),
          isStalled: d.isStalled,
          isNew: a.changed.has(`${d.item.tenant}:${d.item.id}`),
        })),
        waves: [...openWaves, ...newlyClosed].map(w => ({
          name: `${waveName(w, labels)} ${w.title.split(' ').slice(1).join(' ')}`.trim(),
          done: memberParts(w).done,
          total: memberParts(w).total,
          pct: w.percent,
          isNew: a.changed.has(w.id),
          isClosed: w.state === 'done',
        })),
        folded: folded.length > 0 ? `✓ ${folded.length} closed — ${folded.map(w => waveName(w, labels)).join(', ')}` : '',
        harness:
          density === 'compact'
            ? []
            : snap.harness.map(h => {
                const [dp, df] = deltaText(h)
                const base = cursor?.harness[h.name]
                return {
                  name: h.name,
                  pass: h.pass,
                  fail: h.fail,
                  skip: h.skip,
                  deltaPass: dp,
                  deltaFail: df,
                  passUp: base !== undefined ? h.pass - base.pass : 0,
                  failUp: base !== undefined ? h.fail - base.fail : 0,
                }
              }),
        events: events.map(ev => ({ t: isoHhmm(ev.ts), text: `${ev.tenant} ${ev.item} ${ev.action}${ev.note ? ` — ${ev.note.slice(0, 60)}` : ''}` })),
        footer: `updated ${hhmm(snap.updatedAt)} · auto ${every}`,
        density,
        order,
      }
      const baseWidth = hasWidth ? Math.max(300, Math.min(520, given * 8)) : 420
      const svg = buildSvg(model, st, Math.round(baseWidth * scaleFor(st, step)), scaleFor(st, step))
      return (
        <Box flexDirection="column">
          {header}
          {stale !== undefined && <Text color="warning">⏱ stale since {hhmm(stale)}</Text>}
          <Svg source={svg.source} alt={svg.alt} width={svg.width} />
          {job.news !== '' && <Link href={job.news}>News page ↗</Link>}
          {footer}
        </Box>
      )
    }

    const trendLine = (row: TidewatchHarness) => {
      const base = cursor?.harness[row.name]
      const passUp = base !== undefined ? row.pass - base.pass : 0
      const failUp = base !== undefined ? row.fail - base.fail : 0
      const pass = passUp !== 0 ? `pass ${base!.pass}→${row.pass} ${passUp > 0 ? '▲' : '▼'}` : `pass ${row.pass}`
      const fail = failUp !== 0 ? `fail ${base!.fail}→${row.fail} ${failUp > 0 ? '▲' : '▼'}` : `fail ${row.fail}`
      return (
        <Box gap={2}>
          <Text {...mo} color={passUp > 0 ? 'success' : passUp < 0 ? 'error' : undefined}>{pass}</Text>
          <Text {...mo} color={failUp > 0 ? 'error' : failUp < 0 ? 'success' : undefined}>{fail}</Text>
          <Text {...mo} dimColor>{`skip ${row.skip}`}</Text>
        </Box>
      )
    }

    const secs: Record<string, any> = {
        needs: shown.needs.length > 0 && (
          <Box key="sec-needs" flexDirection="column">
            {label('Needs you', 'error')}
            {shown.needs.map(i => {
              const k = `${i.tenant}:${i.id}`
              return (
                <Box flexDirection="column">
                  {gridRow(`need-${k}`, `! ${i.title}`, undefined, { text: 'needs you', color: 'error' }, <Text dimColor>{itemAge(i)}</Text>, dot(k))}
                  {noteLine([i.tenant, i.id, i.note].filter(Boolean).join(' · '))}
                </Box>
              )
            })}
          </Box>
        ),

        review: shown.review.length > 0 && (
          <Box key="sec-review" flexDirection="column">
            {label('Review', 'warning')}
            {shown.review.map(i => {
              const k = `${i.tenant}:${i.id}`
              return (
                <Box flexDirection="column">
                  {gridRow(`rev-${k}`, `${pid(i.tenant, i.id)} ${i.title}`, undefined, { text: 'awaiting review', color: 'warning' }, <Text dimColor>{itemAge(i)}</Text>, dot(k))}
                  {noteLine(i.note === 'awaiting review' ? '' : i.note)}
                </Box>
              )
            })}
          </Box>
        ),

        progress: shown.doing.length > 0 && (
          <Box key="sec-progress" flexDirection="column">
            {label('In progress')}
            {shown.doing.map(d => {
              const k = `${d.item.tenant}:${d.item.id}`
              const age = d.ageMs === undefined ? '' : ageText(d.ageMs)
              return gridRow(
                `do-${k}`,
                `${pid(d.item.tenant, d.item.id)} ${d.item.title}`,
                undefined,
                d.isStalled ? { text: 'stalled', color: 'warning' } : isCols ? { text: '' } : undefined,
                <Text color={d.isStalled ? 'warning' : undefined} dimColor={!d.isStalled}>{age}</Text>,
                dot(k),
              )
            })}
          </Box>
        ),

        milestones: waves.length > 0 && (
          <Box key="sec-milestones" flexDirection="column">
            {label('Milestones')}
            {waveRows.map(({ w, name }) => {
              const isNew = a.changed.has(w.id)
              const status: Cell =
                memberParts(w).total === 0
                  ? { text: 'not started', dim: true }
                  : w.state === 'done'
                    ? { text: '✓ closed', color: 'success' }
                    : barCell(w, isNew)
              const marker = isNew ? <Text color="success"> ●</Text> : <Text>{'  '}</Text>
              return gridRow(`wav-${w.id}`, name, undefined, status, countCell(w), marker)
            })}
            {folded.length > 0 && (
              <Text dimColor wrap="truncate-end" {...sz("footnote")}>{`✓ ${folded.length} closed — ${folded.map(w => waveName(w, labels)).join(', ')}`}</Text>
            )}
          </Box>
        ),

        harness: snap.harness.length > 0 && density !== 'compact' && (
          <Box key="sec-harness" flexDirection="column">
            {label('Harness')}
            {snap.harness.map(row => {
              const base = cursor?.harness[row.name]
              const pu = base !== undefined ? row.pass - base.pass : 0
              const fu = base !== undefined ? row.fail - base.fail : 0
              const [dp, df] = deltaText(row)
              const arrows = (
                <Text>
                  <Text color={pu > 0 ? 'success' : pu < 0 ? 'error' : undefined} dimColor={pu === 0}>{dp}</Text>
                  <Text>{dp !== '' && df !== '' ? ' ' : ''}</Text>
                  <Text color={fu > 0 ? 'error' : fu < 0 ? 'success' : undefined} dimColor={fu === 0}>{df}</Text>
                </Text>
              )
              return gridRow(`har-${row.name}`, row.name, undefined, { text: `pass ${row.pass} · fail ${row.fail} · skip ${row.skip}`, dim: true }, arrows, <Text>{'  '}</Text>)
            })}
          </Box>
        ),

        events: events.length > 0 && (
          <Box key="sec-events" flexDirection="column">
            {label('Last events')}
            {events.map(ev => (
              <Box gap={1}>
                <Text {...mo} dimColor {...sz("footnote")}>{isoHhmm(ev.ts)}</Text>
                <Text dimColor wrap="truncate-end" {...sz("footnote")}>{`${ev.tenant} ${ev.item} ${ev.action}${ev.note ? ` — ${ev.note.slice(0, 60)}` : ''}`}</Text>
              </Box>
            ))}
          </Box>
        ),

    }
    return (
      <Box flexDirection="column" gap={density === 'compact' ? 0 : density === 'roomy' ? 2 : 1}>
        <Box flexDirection="column">
          {header}
          {separator}
        </Box>
        {stale !== undefined && <Text color="warning">⏱ stale since {hhmm(stale)}</Text>}

        <Box flexDirection="column">
          <Text {...sz("heading")} {...wt} color={head.tone}>{head.text}</Text>
          <Text dimColor>
            {summary.map((x, idx) => (
              <Text color={x.color} dimColor={x.color === undefined}>{`${idx > 0 ? ' · ' : ''}${x.text}`}</Text>
            ))}
          </Text>
          {chipParts.length > 0 && (
            <Box alignSelf="flex-start">
              <Text color="suggestion">●</Text>
              <Text dimColor>{` ${chipParts.join(' · ')} since ${hhmm(seenTs)}`}</Text>
            </Box>
          )}
        </Box>

        {order.map(id => secs[id])}

        <Box flexDirection="column">
          {separator}
          <Box>
            <Text dimColor {...sz("footnote")}>{`updated ${hhmm(snap.updatedAt)} · auto ${every}`}</Text>
            <Box flexGrow={1} />
            {job.news !== '' && <Link href={job.news}>News page ↗</Link>}
          </Box>
          {footer}
        </Box>
      </Box>
    )
  })
}
