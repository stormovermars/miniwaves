// Pure code: the news model (docs/specs/news-model.md, version 1) and the old gopher-style bulletin drawn from it.
// No `$` in here, so every function is testable on its own.
import { abbr, capN, INVISIBLE, itemRow, mileCount, mileOf, safeSlice, textBar } from './rows'
import type { ItemKind, MileRow } from './rows'
import type { MDelta, MItem, MList, MSprint, Model, ModelBrief, TenantBrief, MwJobsAsk, MwJobsItem, MwJobsSnapshot, MwJobsWave, MwMetricSection, MwMetrics } from '../types'

export type { MDelta, MItem, MList, Model }
export { abbr }

/** The model's own limits (spec): per list, and the whole file. */
export const CAPS = { now: 50, open: 200, happened: 100, ready: 10, nextMilestones: 5, backlog: 50, recent: 10, sprint: 50, text: 200, bytes: 1_048_576 } as const
/** `mw news --full` (gopher with fill on): every row; still a bound so a broken file cannot run away. */
export const FULL_CAPS = { now: 5000, open: 5000, happened: 5000, ready: 5000, nextMilestones: 5000, backlog: 5000, recent: 5000, sprint: 5000, text: 200, bytes: 8_388_608 } as const
type Caps = { readonly [k in keyof typeof CAPS]: number }
// the caps of the parse in progress (parseModel sets it; it is synchronous)
let K: Caps = CAPS

const S = (v: unknown): string => (typeof v === 'string' ? safeSlice(v.replace(INVISIBLE, '').replace(/[\u0000-\u001f\u007f-\u009f\u2028\u2029]/g, ' '), CAPS.text) : typeof v === 'number' ? String(v) : '')
const N = (v: unknown): number => (typeof v === 'number' && Number.isFinite(v) ? v : 0)
const O = (v: unknown): Record<string, unknown> => (v !== null && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : {})

const item = (v: unknown): MItem => {
  const o = O(v)
  // the tracker ticket as mw labels it (MOB-142, #123, AB#123, sc-123): that label is shown, never the raw key; the link only when mw built one
  const ticket = S(o.ticket_label).slice(0, 40).trim()
  const url = ticket !== '' && /^https:\/\/[^\s?#@]+$/.test(String(o.ticket_url ?? '')) ? String(o.ticket_url).slice(0, 300) : ''
  return { tenant: S(o.tenant), id: S(o.id), title: S(o.title), note: S(o.note), since: S(o.since), by: S(o.by), done: N(o.done), total: N(o.total), ticket, ticket_url: url, milestone: S(o.milestone).slice(0, 40), milestone_title: S(o.milestone_title), state: S(o.state).slice(0, 20), needs: o.needs === true }
}

/** A list is an array, or `{ items, more }`; a sibling `<name>_more` counts too. Cut at `cap`; what is cut joins `more`. */
const list = (v: unknown, cap: number, siblingMore = 0): MList => {
  const arr = Array.isArray(v) ? v : Array.isArray(O(v).items) ? (O(v).items as unknown[]) : []
  const more = Math.max(0, Math.floor(N(O(v).more))) + Math.max(0, Math.floor(siblingMore)) + Math.max(0, arr.length - cap)
  return { items: arr.slice(0, cap).map(item), more }
}

const counts = (v: unknown): Record<string, number> => {
  const out: Record<string, number> = {}
  for (const [k, x] of Object.entries(O(v))) if (typeof x === 'number' && Number.isFinite(x)) out[k] = x
  return out
}

const relNo = (v: unknown): number | null => {
  const n = typeof v === 'number' ? v : typeof v === 'string' && /^\d+$/.test(v) ? Number(v) : NaN
  return Number.isInteger(n) && n >= 0 ? n : null
}

/** `since` may be the previous release's number, a timestamp, or { number, at }. */
const sinceOf = (v: unknown): { number: number | null; at: string } => {
  if (typeof v === 'number') return { number: relNo(v), at: '' }
  if (typeof v === 'string') return /^\d+$/.test(v) ? { number: relNo(v), at: '' } : { number: null, at: S(v) }
  const o = O(v)
  return { number: relNo(o.number), at: S(o.at) }
}

/** now.tenants: [{ slug|tenant, name, counts: {...} | flat counts }] in the model's order; at most 5000. */
const tenantsOf = (v: unknown): TenantBrief[] => {
  const arr = Array.isArray(v) ? v : Array.isArray(O(v).items) ? (O(v).items as unknown[]) : []
  const out: TenantBrief[] = []
  for (const t of arr.slice(0, 5000)) {
    const o = O(t)
    const slug = S(o.slug ?? o.tenant ?? o.id)
    if (slug === '') continue
    const c = { ...o, ...O(o.counts) }
    const at = typeof o.last_change === 'string' ? Date.parse(o.last_change) : NaN
    out.push({ slug, name: S(o.name ?? o.title), draft: N(c.draft), needs: N(c.needs), review: N(c.review), blocked: N(c.blocked), doing: N(c.doing), ready: N(c.ready), done: N(c.done), total: N(c.total), last: Number.isNaN(at) ? undefined : at })
  }
  return out
}

/** The release compared against: mw's `previous` number, with `since` the previous release's time (older shapes: since alone). */
const previousOf = (rel: Record<string, unknown>): { number: number | null; at: string } => {
  const s = sinceOf(rel.since)
  const prev = relNo(rel.previous)
  return { number: prev ?? s.number, at: s.at }
}

const sprintOf = (v: unknown): MSprint | null => {
  const o = O(v)
  if (v === null || v === undefined || typeof v !== 'object' || S(o.id) === '') return null
  const items = Array.isArray(o.items) ? (o.items as unknown[]) : []
  return { kind: S(o.kind), tenant: S(o.tenant), id: S(o.id), title: S(o.title), done: N(o.done), total: N(o.total), items: items.slice(0, K.sprint).map(item), more: Math.max(0, Math.floor(N(o.more))) + Math.max(0, items.length - K.sprint) }
}

export type ModelResult = { model?: Model; error?: string }

/**
 * Parses a news model. `version` marks it: no version means this is not a model (the caller falls back);
 * a newer version is refused with a notice. `metricsOf` validates the metrics block the way a metrics.json is.
 */
export const parseModel = (text: string, metricsOf: (m: unknown) => { metrics?: MwMetrics; notice?: string }, opts: { full?: boolean } = {}): ModelResult => {
  K = opts.full === true ? FULL_CAPS : CAPS
  try {
    return parseModelK(text, metricsOf)
  } finally {
    K = CAPS
  }
}

const parseModelK = (text: string, metricsOf: (m: unknown) => { metrics?: MwMetrics; notice?: string }): ModelResult => {
  if (text.length > K.bytes) return { error: `news model over ${K.bytes === CAPS.bytes ? '1' : '8'} MB` }
  let raw: unknown
  try {
    raw = JSON.parse(text)
  } catch {
    return { error: 'news model is not valid JSON' }
  }
  const r = O(raw)
  if (r.version === undefined) return {}
  if (typeof r.version !== 'number' || r.version < 1) return { error: 'news model: bad version' }
  if (r.version > 1) return { error: 'newer news model format' }
  const now = O(r.now)
  const ms = O(now.milestones)
  const hp = O(r.happened)
  const nx = O(r.next)
  const rel = O(r.release)
  const job = O(r.job)
  const nowMore = O(now.more)
  const hpMore = O(hp.more)
  let metrics: Model['metrics'] = null
  const deltas: MDelta = {}
  if (r.metrics !== null && r.metrics !== undefined) {
    const mo = O(r.metrics)
    if (typeof mo.error === 'string') metrics = { error: S(mo.error) }
    else {
      for (const sec of Array.isArray(mo.sections) ? mo.sections : [])
        for (const row of Array.isArray(O(sec).rows) ? (O(sec).rows as unknown[]) : []) {
          const d = counts(O(row).delta)
          if (Object.keys(d).length > 0) deltas[`${S(O(sec).title)}/${S(O(row).name)}`] = d
        }
      const res = metricsOf({ version: 1, ...mo })
      metrics = res.metrics ?? { error: res.notice ?? 'metrics unavailable' }
    }
  }
  const model: Model = {
    version: 1,
    job: { id: S(job.id), title: S(job.title), description: S(job.description), url: S(job.url) },
    release: { number: relNo(rel.number), at: S(rel.at), since: previousOf(rel), changes: typeof rel.changes === 'number' ? Math.max(0, Math.floor(rel.changes)) : undefined },
    now: {
      counts: counts(now.counts),
      needs: list(now.needs, K.now, N(nowMore.needs)),
      review: list(now.review, K.now, N(nowMore.review)),
      blocked: list(now.blocked, K.now, N(nowMore.blocked)),
      doing: list(now.doing, K.now, N(nowMore.doing)),
      milestones: list(ms.open, K.open, N(ms.more)),
      closed: Math.max(0, Math.floor(N(ms.closed_count))),
      tenants: tenantsOf(now.tenants),
      tenantsMore: Math.max(0, Math.floor(N(O(now.tenants).more) + N(nowMore.tenants))),
      backlog: { ...list(O(now.backlog).items ?? now.backlog, K.backlog, N(nowMore.backlog)), count: Math.max(0, Math.floor(N(O(now.backlog).count))) },
      recentDone: list(now.recent_done, K.recent),
      sprint: sprintOf(now.sprint),
      backlogKnown: now.backlog !== undefined,
    },
    happened: {
      closed_milestones: list(hp.closed_milestones, K.happened, N(hpMore.closed_milestones)),
      done: list(hp.done, K.happened, N(hpMore.done)),
      accepted: list(hp.accepted, K.happened, N(hpMore.accepted)),
      sent_back: list(hp.sent_back, K.happened, N(hpMore.sent_back)),
      answers: list(hp.answers, K.happened, N(hpMore.answers)),
      decisions: list(hp.decisions, K.happened, N(hpMore.decisions)),
      new_items: list(hp.new_items, K.happened, N(hpMore.new_items)),
      moved_to_review: list(hp.moved_to_review, K.happened, N(hpMore.moved_to_review)),
      moved_to_doing: list(hp.moved_to_doing, K.happened, N(hpMore.moved_to_doing)),
      counts: counts(hp.counts),
    },
    next: { ready: list(nx.ready, K.ready, N(O(nx.more).ready)), next_milestones: list(nx.next_milestones, K.nextMilestones, N(O(nx.more).next_milestones)) },
    metrics,
    deltas,
    summary: S(r.summary).trim(),
  }
  return { model }
}

const fmtNum = (n: number) => String(Math.round(n * 100) / 100)

/** Total in a list: the model's count when it has one, otherwise what the list holds plus its `more`. */
export const total = (l: MList, counted?: number) => Math.max(counted ?? 0, l.items.length + l.more)

const HAPPENED_KEYS = ['closed_milestones', 'done', 'accepted', 'sent_back', 'answers', 'decisions', 'new_items'] as const
export const happenedCount = (m: Model, key: (typeof HAPPENED_KEYS)[number]) => total(m.happened[key], m.happened.counts[key])

/** How many things happened in the span: the draft's "N changes since issue M". */
export const changeCount = (m: Model): number => m.release.changes ?? HAPPENED_KEYS.reduce((n, k) => n + happenedCount(m, k), 0)

const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`

/** One line for the release list: "5 done · 1 milestone · 2 need you". */
export const relHeadline = (m: Model): string => {
  const done = happenedCount(m, 'done') + happenedCount(m, 'accepted')
  const mil = happenedCount(m, 'closed_milestones')
  const needs = total(m.now.needs, m.now.counts.needs)
  const sent = happenedCount(m, 'sent_back')
  const parts = [done > 0 ? `${done} done` : '', mil > 0 ? plural(mil, 'milestone') : '', sent > 0 ? `${sent} sent back` : '', needs > 0 ? `${needs} need you` : ''].filter(Boolean)
  return parts.length > 0 ? parts.join(' · ') : 'no changes'
}

const date10 = (iso: string) => (/^\d{4}-\d{2}-\d{2}/.test(iso) ? iso.slice(0, 10) : iso)
export const releaseDate = (m: Model) => date10(m.release.at)

/** Epoch ms of the newest `since` among the live items, else the model's own time. */
export const lastChangeMs = (m: Model): number | undefined => {
  const all = [m.now.needs, m.now.review, m.now.blocked, m.now.doing].flatMap(l => l.items.map(i => Date.parse(i.since)))
  const ms = all.filter(n => !Number.isNaN(n))
  if (ms.length > 0) return Math.max(...ms)
  const at = Date.parse(m.release.at)
  return Number.isNaN(at) ? undefined : at
}

// ---- the bulletin ----

export const BULLETIN_MAX = 70
/** The narrowest gopher sheet: 30 columns; a pane with less room says it is too narrow. */
export const GOPHER_MIN = 30
export type BTone = 'fg' | 'dim' | 'head' | 'red' | 'amber' | 'green'
export type BPiece = { text: string; tone: BTone }
export type BLine = { t: string; tone: BTone; pieces?: BPiece[]; ask?: { tenant: string; id: string } }

/** Plain ASCII for the bulletin: the few typographic characters map to look-alikes, anything else to `?`; ▲ and ▼ stay. */
export const ascii = (t: string): string =>
  t
    .replace(/[–—−]/g, '-')
    .replace(/[‘’]/g, "'")
    .replace(/[“”]/g, '"')
    .replace(/…/g, '~')
    .replace(/·/g, '.')
    .replace(/[^\x20-\x7e▲▼]/g, '?')

const cutTo = (t: string, n: number) => (t.length <= n ? t : n <= 1 ? t.slice(0, n) : `${t.slice(0, n - 1)}~`)

/** Greedy word wrap to `w`; a word longer than the line is cut with `~`. */
export const wrapPlain = (text: string, w: number): string[] => {
  const out: string[] = []
  let cur = ''
  for (const word of ascii(text).split(/\s+/).filter(Boolean)) {
    const wd = cutTo(word, w)
    if (cur === '') cur = wd
    else if (cur.length + 1 + wd.length <= w) cur += ` ${wd}`
    else {
      out.push(cur)
      cur = wd
    }
  }
  if (cur !== '') out.push(cur)
  return out
}

/** `left` at the left edge and `right` ending at column `w`; the left side is cut when they do not both fit. */
const lr = (left: string, right: string, w: number): string => {
  const room = w - right.length - 1
  const l = left.length > room ? cutTo(left, Math.max(0, room)) : left
  return `${l}${' '.repeat(Math.max(1, w - l.length - right.length))}${right}`
}

const GLYPH: Record<string, BTone> = { '!': 'red', '?': 'amber', '-': 'red', '+': 'fg', '0': 'fg', '1': 'fg', '>': 'fg' }

/** One item line: type char, tenant/id, title, dot leaders, state ending at column `w`. Long titles end in `~`. */
/** While one tenant is shown, its items drop the `tenant/` prefix. Set around one synchronous build, never across an await. */
let hideTenant = false
export const tenantHidden = () => hideTenant
export const withTenantHidden = <T>(on: boolean, fn: () => T): T => {
  const was = hideTenant
  hideTenant = on
  try {
    return fn()
  } finally {
    hideTenant = was
  }
}
/** `tenant/id` with a long slug cut by ~ (14 characters), or the id alone while one tenant is shown. */
export const refOf = (tenant: string, id: string): string => (tenant === '' || hideTenant ? id : `${tenant.length > 14 ? `${tenant.slice(0, 13)}~` : tenant}/${id}`)

export const itemLine = (type: string, i: { tenant: string; id: string; title: string; ticket?: string }, state: string, w: number, ask?: boolean): BLine => {
  const st = ascii(state)
  const ref = cutTo(ascii(refOf(i.tenant, i.id)), Math.max(8, w - st.length - 22))
  const left = `${type} ${ref}  `
  const title = cutTo(ascii(i.ticket ? `${i.title} [${i.ticket}]` : i.title), Math.max(1, w - left.length - st.length - 5))
  const dots = Math.max(3, w - left.length - title.length - st.length - 2)
  const t = `${left}${title} ${'.'.repeat(dots)} ${st}`
  return { t, tone: GLYPH[type] ?? 'fg', ask: ask ? { tenant: i.tenant, id: i.id } : undefined }
}

/** A milestone line, the one format everywhere: `1 tenant/ID  title ..... done/total state [bar]`. */
export const mileLine = (r: MileRow, w: number): BLine => itemLine('1', { tenant: r.tenant, id: r.id, title: r.title, ticket: r.ticket }, `${mileCount(r)} ${r.state} ${textBar(r.pct, 10)}`, w)

/** The first `top` items of a list, one more when only one would stay hidden. */
export const topOf = (l: MList, top: number, counted?: number): MItem[] => l.items.slice(0, capN(total(l, counted), top))

const milesGroup = (l: MList, top: number, closed: boolean, w: number, counted?: number): BLine[] => {
  const shown = topOf(l, top, counted)
  const out = shown.map(i => mileLine(mileOf({ tenant: i.tenant, id: i.id, title: i.title, done: i.done, total: i.total, ticket: i.ticket, ticketUrl: i.ticket_url }, {}, closed), w))
  const rest = Math.max(total(l, counted) - shown.length, 0)
  if (rest > 0) out.push(moreLine(rest))
  return out
}

const moreLine = (n: number): BLine => ({ t: `... ${n} more (see full board)`, tone: 'dim' })

const section = (no: number, title: string, w: number): BLine[] => {
  const t = `${no}. ${title}`
  return [{ t, tone: 'head' }, { t: '-'.repeat(Math.min(t.length, w)), tone: 'dim' }]
}

type Group = { type: string; list: MList; state: (i: MItem) => string; ask?: boolean; counted?: number; top: number }

const group = (g: Group, w: number): BLine[] => {
  const shown = topOf(g.list, g.top, g.counted)
  const out = shown.map(i => itemLine(g.type, i, g.state(i), w, g.ask))
  const rest = Math.max(total(g.list, g.counted) - shown.length, 0)
  if (rest > 0) out.push(moreLine(rest))
  return out
}


const ordinal = (n: number, one: string, many = `${one}s`) => (n === 1 ? one : many)

/** The generated In short lines: what happened, what is ongoing, what is next. At most three. */
export const shortLines = (m: Model): string[] => {
  const done = happenedCount(m, 'done') + happenedCount(m, 'accepted')
  const mil = happenedCount(m, 'closed_milestones')
  const sent = happenedCount(m, 'sent_back')
  const dec = happenedCount(m, 'decisions') + happenedCount(m, 'answers')
  const happened = [done > 0 ? `${done} ${ordinal(done, 'item')} done` : '', mil > 0 ? `${mil} ${ordinal(mil, 'milestone')} closed` : '', sent > 0 ? `${sent} sent back` : '', dec > 0 ? `${dec} ${ordinal(dec, 'decision')}` : ''].filter(Boolean)
  const c = m.now.counts
  const needs = total(m.now.needs, c.needs)
  const review = total(m.now.review, c.review)
  const blocked = total(m.now.blocked, c.blocked)
  const doing = total(m.now.doing, c.doing)
  const ongoing = [needs > 0 ? `${needs} need you` : '', review > 0 ? `${review} to review` : '', blocked > 0 ? `${blocked} blocked` : '', doing > 0 ? `${doing} in progress` : ''].filter(Boolean)
  const ready = total(m.next.ready, c.ready)
  const lines = [happened.length > 0 ? happened.join(', ') : m.release.number === null ? 'nothing new yet' : 'nothing happened in this issue']
  if (ongoing.length > 0) lines.push(ongoing.join(', '))
  if (ready > 0 || m.next.next_milestones.items.length > 0) lines.push([ready > 0 ? `${ready} ready` : '', m.next.next_milestones.items.length > 0 ? `${plural(total(m.next.next_milestones), 'milestone')} open next` : ''].filter(Boolean).join(', '))
  return lines.slice(0, 3)
}

const delta = (n: number | undefined, lowerBetter = false) => (n === undefined || n === 0 ? '' : ` ${n > 0 ? '▲' : '▼'}${fmtNum(Math.abs(n))}`) + (lowerBetter ? '' : '')

/** `[#####=====....]` from counts, scaled to `bw` cells by largest remainder. */
export const asciiBar = (parts: Array<{ ch: string; n: number }>, bw: number): string => {
  const sum = parts.reduce((s, p) => s + p.n, 0)
  if (sum <= 0) return `[${'.'.repeat(bw)}]`
  const raw = parts.map(p => (p.n / sum) * bw)
  const cells = raw.map(Math.floor)
  let left = bw - cells.reduce((a, b) => a + b, 0)
  const order = raw.map((r, i) => ({ i, f: r - Math.floor(r) })).sort((a, b) => b.f - a.f)
  for (const o of order) {
    if (left <= 0) break
    if (parts[o.i]!.n > 0) {
      cells[o.i]! += 1
      left -= 1
    }
  }
  return `[${parts.map((p, i) => p.ch.repeat(cells[i]!)).join('')}]`
}

/** The metric view model the panes already build (register.tsx metricViews), as far as the gopher drawing reads it. */
export type MViewRow = {
  name: string
  parts: Array<{ f: number; tone: 'green' | 'amber' | 'red' }>
  showBar: boolean
  barNote: string
  ratio?: { a: number; b: number }
  valueText?: string
  delta: Array<{ text: string }>
}
export type MViewSection = { title: string; note: string; legend: Array<{ tone: string; name: string }>; rows: MViewRow[]; more?: number }

/** Words wrapped into lines of `w` columns, each line one string. */
export const flowLines = (words: string[], w: number, indent = ' '): string[] => {
  const out: string[] = []
  let cur = indent
  for (const word of words.map(x => cutTo(ascii(x), w - indent.length))) {
    if (cur.trim() === '') cur += word
    else if (cur.length + 1 + word.length <= w) cur += ` ${word}`
    else {
      out.push(cur)
      cur = indent + word
    }
  }
  if (cur.trim() !== '') out.push(cur)
  return out
}

/** Metrics as gopher lines: a title, a wrapped legend, then `name [####==..]  5/68 up2`; value rows on one line of their own. */
export const metricLines = (secs: MViewSection[], w: number, notice?: string): BLine[] => {
  const out: BLine[] = []
  if (notice) out.push(...flowLines(notice.split(/\s+/), w).map(t => ({ t, tone: 'dim' as const })))
  for (const sec of secs) {
    out.push({ t: cutTo(` ${ascii(sec.title)}`, w), tone: 'head' })
    const legend = sec.legend.length > 0 ? sec.legend.map(l => `${l.tone === 'green' ? '#' : l.tone === 'amber' ? '=' : l.tone === 'red' ? 'x' : '.'}=${l.name}`) : []
    const noteWords = sec.note ? ['-', ...sec.note.split(/\s+/)] : []
    for (const t of flowLines([...legend, ...noteWords], w)) out.push({ t, tone: 'dim' })
    const barRows = sec.rows.filter(r => r.ratio !== undefined)
    const L = Math.min(18, Math.max(4, ...sec.rows.map(r => ascii(r.name).length)))
    const cmax = Math.max(0, ...barRows.map(r => `${abbr(r.ratio!.a)}/${abbr(r.ratio!.b)}`.length + r.delta.reduce((n, d) => n + d.text.length + 1, 0)))
    const bw = Math.max(6, Math.min(15, w - 1 - L - 1 - 2 - 2 - cmax - 1))
    for (const r of sec.rows) {
      const dl = r.delta.map(d => ` ${d.text}`).join('')
      if (r.ratio === undefined) {
        // a value row: its own line, name then value, never in a bar column
        const name = cutTo(ascii(r.name), L).padEnd(L)
        out.push({ t: cutTo(` ${name}  ${ascii(r.valueText ?? '')}${r.barNote ? `  (${ascii(r.barNote)})` : ''}${dl}`, w), tone: 'fg' })
        continue
      }
      const bar = r.showBar ? asciiBar(r.parts.map(pt => ({ ch: pt.tone === 'green' ? '#' : pt.tone === 'amber' ? '=' : 'x', n: pt.f })).concat([{ ch: '.', n: Math.max(0, 1 - r.parts.reduce((x, pt) => x + pt.f, 0)) }]), bw) : `[${'.'.repeat(bw)}]`
      out.push({ t: cutTo(` ${cutTo(ascii(r.name), L).padEnd(L)} ${bar}  ${abbr(r.ratio.a)}/${abbr(r.ratio.b)}${dl}`, w), tone: 'fg' })
    }
    if ((sec.more ?? 0) > 0) out.push({ t: ` +${sec.more} rows`, tone: 'dim' })
  }
  return out
}

export type BulletinOpts = {
  width: number
  /** Every release number, any order. */
  releases: number[]
  /** The previous release's date, when known. */
  prevAt?: string
  /** The job's news url, decoration only. */
  url?: string
  /** Metric sections drawn in section 5 (the pane builds them: deltas from the model). */
  metrics?: MViewSection[]
  /** Every row of every list (gopher with fill on and an mw with `news --full`): no tops, no "more" lines. */
  full?: boolean
}

/** The whole bulletin, one BLine per row, `width` columns at most (BULLETIN_MAX). */
export const bulletin = (m: Model, o: BulletinOpts): BLine[] => {
  const w = Math.max(GOPHER_MIN, Math.min(BULLETIN_MAX, o.width))
  const T = (n: number) => (o.full === true ? Number.MAX_SAFE_INTEGER : n)
  const rule = (ch: string): BLine => ({ t: ch.repeat(w), tone: 'dim' })
  const draft = m.release.number === null
  const title = `${ascii(m.job.title || m.job.id).toUpperCase()} BULLETIN`
  const head1 = lr(` ${title}`, draft ? 'DRAFT - NOT PUBLISHED ' : `ISSUE ${m.release.number} / ${releaseDate(m)} `, w)
  const since = m.release.since
  const left2 = draft ? (since.number !== null ? ` since issue ${since.number}${o.prevAt ? ` (${date10(o.prevAt)})` : ''}` : ' since the start') : since.number !== null ? ` vs issue ${since.number}${o.prevAt ? ` (${date10(o.prevAt)})` : ''}` : ' since the start'
  const url = ascii(o.url || m.job.url || `gopher://local/1/news/${m.job.id}`)
  const head2 = lr(left2, `${url} `, w)
  const out: BLine[] = [rule('='), { t: head1, tone: 'head' }, { t: head2, tone: 'dim' }, rule('=')]

  out.push({ t: '', tone: 'fg' }, ...section(1, 'IN SHORT', w))
  const short = m.summary !== '' ? wrapPlain(m.summary, w).slice(0, 6) : shortLines(m).flatMap(l => wrapPlain(l, w))
  for (const l of short) out.push({ t: l, tone: 'fg' })

  out.push({ t: '', tone: 'fg' }, ...section(2, 'WHAT HAPPENED', w))
  const hp = m.happened
  const happenedLines: BLine[] = [
    ...milesGroup(hp.closed_milestones, T(10), true, w, hp.counts.closed_milestones),
    ...group({ type: '0', list: hp.done, state: () => 'done', top: T(10), counted: hp.counts.done }, w),
    ...group({ type: '0', list: hp.accepted, state: () => 'accepted', top: T(10), counted: hp.counts.accepted }, w),
    ...group({ type: '0', list: hp.sent_back, state: () => 'sent back', top: T(10), counted: hp.counts.sent_back }, w),
    ...group({ type: '0', list: hp.decisions, state: () => 'decided', top: T(10), counted: hp.counts.decisions }, w),
    ...group({ type: '0', list: hp.answers, state: () => 'answered', top: T(10), counted: hp.counts.answers }, w),
  ]
  out.push(...(happenedLines.length > 0 ? happenedLines : [{ t: draft ? 'nothing yet since the last issue' : 'nothing happened in this issue', tone: 'dim' as const }]))

  out.push({ t: '', tone: 'fg' }, ...section(3, 'ONGOING', w))
  const c = m.now.counts
  const ctext = [`needs ${total(m.now.needs, c.needs)}`, `review ${total(m.now.review, c.review)}`, `blocked ${total(m.now.blocked, c.blocked)}`, `doing ${total(m.now.doing, c.doing)}`, ...(c.total !== undefined ? [`done ${c.done ?? 0}/${c.total}`] : [])].join(' | ')
  out.push({ t: cutTo(ctext, w), tone: 'fg' })
  out.push(
    ...group({ type: '!', list: m.now.needs, state: () => 'needs you', ask: true, counted: c.needs, top: T(5) }, w),
    ...group({ type: '?', list: m.now.review, state: () => 'review', ask: true, counted: c.review, top: T(5) }, w),
    ...group({ type: '-', list: m.now.blocked, state: () => 'blocked', ask: true, counted: c.blocked, top: T(5) }, w),
    ...group({ type: '+', list: m.now.doing, state: () => 'doing', counted: c.doing, top: T(5) }, w),
  )

  out.push({ t: '', tone: 'fg' }, ...section(4, "WHAT'S NEXT", w))
  const next = [
    ...group({ type: '>', list: m.next.ready, state: () => 'ready', top: T(10), counted: c.ready }, w),
    ...milesGroup(m.next.next_milestones, T(5), false, w),
  ]
  out.push(...(next.length > 0 ? next : [{ t: 'nothing queued', tone: 'dim' as const }]))

  const metricNotice = m.metrics === null ? 'no metrics file' : 'error' in m.metrics ? `metrics unavailable: ${m.metrics.error}` : undefined
  out.push({ t: '', tone: 'fg' }, ...section(5, 'METRICS', w), ...metricLines(o.metrics ?? [], w, metricNotice))

  const nums = [...o.releases].sort((a, b) => b - a)
  const tail = 'end of bulletin '
  let list = nums.join(' ')
  const maxList = w - ' issues: '.length - tail.length - 2
  if (list.length > maxList) {
    let cut = ''
    for (const n of nums) {
      if ((cut === '' ? '' : `${cut} `).length + String(n).length + 4 > maxList) break
      cut = cut === '' ? String(n) : `${cut} ${n}`
    }
    list = `${cut} ...`
  }
  out.push({ t: '', tone: 'fg' }, rule('-'), { t: lr(` issues: ${list}`, tail, w), tone: 'dim' }, rule('='))
  return out
}

/** The bulletin as one fenced text block (md). */
export const bulletinMd = (lines: BLine[]): string => `\`\`\`text\n${lines.map(l => l.t).join('\n')}\n\`\`\``

// ---- the jobs pane's snapshot, built from the model ----

export const briefOf = (m: Model): ModelBrief => {
  const c = m.now.counts
  const cnt = (l: MList, k: string) => total(l, c[k])
  return {
    counts: { needs: cnt(m.now.needs, 'needs'), review: cnt(m.now.review, 'review'), blocked: cnt(m.now.blocked, 'blocked'), doing: cnt(m.now.doing, 'doing') },
    more: { needs: m.now.needs.more, review: m.now.review.more, blocked: m.now.blocked.more, doing: m.now.doing.more },
    openMilestones: total(m.now.milestones),
    closedMilestones: m.now.closed,
    lastChange: lastChangeMs(m),
    done: c.done ?? 0,
    total: c.total ?? 0,
    tenants: m.now.tenants,
    tenantsMore: m.now.tenantsMore,
    ready: total(m.next.ready, c.ready),
  }
}

/**
 * The model as the jobs pane's snapshot: needs are the open asks, blocked is its own group, no events,
 * only open milestones (nearest first). Everything the three renderers already read stays the same shape.
 */
export const snapshotFromModel = (m: Model, base: { inChat?: Record<string, number>; metrics?: MwMetrics; metricsNotice?: string; now: number }): MwJobsSnapshot => {
  const mk = (i: MItem, state: string, type = 'task'): MwJobsItem => ({ tenant: i.tenant, id: i.id, type, state, title: i.title, note: i.note, updated: i.since, ticket: i.ticket || undefined, ticketUrl: i.ticket_url || undefined })
  const asks: MwJobsAsk[] = m.now.needs.items.map(i => ({ tenant: i.tenant, item: i.id, question: i.note || i.title, by: i.by, at: i.since }))
  const items = [
    ...m.now.needs.items.map(i => mk(i, 'open')),
    ...m.now.review.items.map(i => mk(i, 'review')),
    ...m.now.blocked.items.map(i => mk(i, 'blocked')),
    ...m.now.doing.items.map(i => mk(i, 'doing')),
    ...m.next.ready.items.map(i => mk(i, 'ready')),
  ]
  const waves: MwJobsWave[] = m.now.milestones.items.map(i => ({
    id: `${i.tenant}:${i.id}`,
    title: `${i.id} ${i.title}`,
    state: 'open',
    members: `${i.done}/${i.total}`,
    percent: i.total > 0 ? Math.round((i.done / i.total) * 100) : 0,
    blocked: 0,
  }))
  const metrics = m.metrics !== null && !('error' in m.metrics) ? m.metrics : base.metrics
  const notice = m.metrics !== null && 'error' in m.metrics ? `metrics unavailable: ${m.metrics.error}` : base.metricsNotice
  return { waves, items, events: [], metrics, metricsNotice: notice, updatedAt: base.now, asks, inChat: base.inChat, model: briefOf(m) }
}

// ---- the jobs pane in gopher style: a live NOW sheet ----

export type NowRow = { tenant: string; id: string; title: string; state: string; ticket?: string }
export type NowGroup = { type: '!' | '?' | '-' | '+'; title: string; rows: NowRow[]; hidden: number }
export type NowView = {
  title: string
  /** HH:MM of the last refresh. */
  time: string
  url: string
  counts: { needs: number; review: number; blocked: number; doing: number }
  groups: NowGroup[]
  /** Open milestones shown (nearest to done first), then the "+N open · ✓N closed" tail. */
  miles: MileRow[]
  tail: string
  metrics: MViewSection[]
  metricsNotice?: string
  notes: string[]
  /** "(trimmed to fit)" and the like, last line. */
  footer: string[]
}

/** The NOW sheet: masthead, then numbered sections like the bulletin's, the same item lines and bars. */
export const nowSheet = (v: NowView, width: number): BLine[] => {
  const w = Math.max(GOPHER_MIN, Math.min(BULLETIN_MAX, width))
  const rule = (ch: string): BLine => ({ t: ch.repeat(w), tone: 'dim' })
  const c = v.counts
  const out: BLine[] = [
    rule('='),
    { t: lr(` ${ascii(v.title).toUpperCase()} - NOW`, `${v.time} / live `, w), tone: 'head' },
    { t: lr(` !${c.needs} ?${c.review} -${c.blocked} +${c.doing}`, `${ascii(v.url)} `, w), tone: c.needs > 0 ? 'red' : c.review + c.blocked > 0 ? 'amber' : 'green' },
    rule('='),
  ]
  for (const n of v.notes) out.push({ t: cutTo(` ${ascii(n)}`, w), tone: 'amber' })
  let no = 0
  for (const g of v.groups) {
    if (g.rows.length === 0 && g.hidden === 0) continue
    no += 1
    out.push({ t: '', tone: 'fg' }, ...section(no, g.title, w))
    for (const r of g.rows) out.push(itemLine(g.type, r, r.state, w, g.type !== '+'))
    if (g.hidden > 0) out.push(moreLine(g.hidden))
  }
  if (v.miles.length > 0 || v.tail !== '') {
    no += 1
    out.push({ t: '', tone: 'fg' }, ...section(no, 'MILESTONES', w))
    for (const mi of v.miles) out.push(mileLine(mi, w))
    if (v.tail !== '') out.push({ t: cutTo(v.tail, w), tone: 'dim' })
  }
  if (v.metrics.length > 0 || v.metricsNotice) {
    no += 1
    out.push({ t: '', tone: 'fg' }, ...section(no, 'METRICS', w), ...metricLines(v.metrics, w, v.metricsNotice))
  }
  if (no === 0) out.push({ t: '', tone: 'fg' }, { t: ' nothing is waiting on anyone', tone: 'green' })
  for (const f of v.footer) out.push({ t: ` ${f}`, tone: 'dim' })
  out.push(rule('-'))
  return out
}

// ---- the newsletter as markdown (normal and blocks styles draw it with the md pipeline) ----

const mdText = (t: string) => t.replace(/[*`|\[\]\\]/g, ' ').replace(/\s+/g, ' ').trim()
/** ` [LABEL]` after a title (the ticket_label): a link when mw gave a ticket_url, plain otherwise; nothing without a ticket. */
export const ticketMd = (i: { ticket?: string; ticket_url?: string }): string => (i.ticket ? (i.ticket_url ? ` \\[[${i.ticket.replace(/[\[\]()]/g, '')}](${i.ticket_url})\\]` : ` [${i.ticket}]`) : '')

const mdList = (l: MList, kind: ItemKind, top: number, labels: Record<string, string>, counted?: number): string[] => {
  const shown = topOf(l, top, counted)
  const rows = shown.map(i => {
    const r = itemRow(kind, { tenant: i.tenant, id: i.id, title: i.title, ticket: i.ticket, ticketUrl: i.ticket_url }, labels)
    return `- **${mdText(r.key)}** ${mdText(r.title)}${ticketMd(i)} \u2014 ${r.state}`
  })
  const rest = Math.max(total(l, counted) - shown.length, 0)
  if (rest > 0) rows.push(`- *+${rest} more (see full board)*`)
  return rows
}

/** The newsletter as segments: markdown text, and milestone rows (drawn by the one shared milestone row function, not as text). */
export type NewsSeg = { md: string } | { miles: MileRow[]; tail: string }

const milesOf = (l: MList, closed: boolean, top: number, labels: Record<string, string>) =>
  topOf(l, top).map(i => mileOf({ tenant: i.tenant, id: i.id, title: i.title, done: i.done, total: i.total, ticket: i.ticket, ticketUrl: i.ticket_url }, labels, closed))

export const newsSegments = (m: Model, o: { metrics?: MViewSection[]; labels?: Record<string, string> }): NewsSeg[] => {
  const labels = o.labels ?? {}
  const segs: NewsSeg[] = []
  const md = (...lines: string[]) => {
    const last = segs[segs.length - 1]
    const text = lines.join('\n')
    if (last && 'md' in last) last.md += `\n${text}`
    else segs.push({ md: text })
  }
  const c = m.now.counts
  const hp = m.happened
  md('## In short', '', ...(m.summary !== '' ? [mdText(m.summary)] : shortLines(m).map(mdText)), '', '## What happened', '')
  const happened: Array<[string, string[]]> = [
    ['Done', mdList(hp.done, 'done', 10, labels, hp.counts.done)],
    ['Accepted', mdList(hp.accepted, 'accepted', 10, labels, hp.counts.accepted)],
    ['Sent back', mdList(hp.sent_back, 'sent_back', 10, labels, hp.counts.sent_back)],
    ['Decisions', mdList(hp.decisions, 'decision', 10, labels, hp.counts.decisions)],
    ['Answers', mdList(hp.answers, 'answer', 10, labels, hp.counts.answers)],
  ]
  const closed = milesOf(hp.closed_milestones, true, 10, labels)
  const closedRest = Math.max(total(hp.closed_milestones, hp.counts.closed_milestones) - closed.length, 0)
  if (closed.length > 0) {
    md('### Milestones closed', '')
    segs.push({ miles: closed, tail: closedRest > 0 ? `+${closedRest} more (see full board)` : '' })
    md('')
  }
  for (const [title, rows] of happened) if (rows.length > 0) md(`### ${title}`, '', ...rows, '')
  if (closed.length === 0 && happened.every(([, r]) => r.length === 0)) md(m.release.number === null ? 'Nothing yet since the last issue.' : 'Nothing happened in this issue.', '')
  md('## Ongoing', '', `needs ${total(m.now.needs, c.needs)} \u00b7 review ${total(m.now.review, c.review)} \u00b7 blocked ${total(m.now.blocked, c.blocked)} \u00b7 doing ${total(m.now.doing, c.doing)}${c.total !== undefined ? ` \u00b7 done ${c.done ?? 0}/${c.total}` : ''}`, '')
  const ongoing: Array<[string, string[]]> = [
    ['Needs you', mdList(m.now.needs, 'needs', 5, labels, c.needs)],
    ['In review', mdList(m.now.review, 'review', 5, labels, c.review)],
    ['Blocked', mdList(m.now.blocked, 'blocked', 5, labels, c.blocked)],
    ['In progress', mdList(m.now.doing, 'doing', 5, labels, c.doing)],
  ]
  for (const [title, rows] of ongoing) if (rows.length > 0) md(`### ${title}`, '', ...rows, '')
  md("## What's next", '')
  const ready = mdList(m.next.ready, 'ready', 10, labels, c.ready)
  const nextMiles = milesOf(m.next.next_milestones, false, 5, labels)
  if (ready.length > 0) md(...ready, '')
  if (nextMiles.length > 0) segs.push({ miles: nextMiles, tail: '' })
  if (ready.length === 0 && nextMiles.length === 0) md('Nothing queued.', '')
  md('', '## Metrics', '')
  if (m.metrics === null) md('No metrics file.')
  else if ('error' in m.metrics) md(mdText(`Metrics unavailable: ${m.metrics.error}`))
  else
    for (const sec of o.metrics ?? []) {
      md(`### ${mdText(sec.title)}`, '')
      for (const r of sec.rows) md(`- **${mdText(r.name)}** ${r.ratio ? `${abbr(r.ratio.a)}/${abbr(r.ratio.b)}` : mdText(r.valueText ?? '')}${r.delta.map(d => ` ${d.text}`).join('')}`)
      if ((sec.more ?? 0) > 0) md(`- *+${sec.more} rows*`)
      md('')
    }
  return segs
}

/** The same newsletter as one markdown text (the terminal's Markdown element, and tests); milestone rows become list lines. */
export const newsMd = (m: Model, o: { metrics?: MViewSection[]; labels?: Record<string, string> }): string =>
  newsSegments(m, o)
    .map(sg => ('md' in sg ? sg.md : [...sg.miles.map(r => `- **${mdText(r.key)}** ${mdText(r.title)}${r.ticket ? ` [${mdText(r.ticket)}]` : ''} \u2014 ${mileCount(r)} \u00b7 ${r.state}`), ...(sg.tail ? [`- *${sg.tail}*`] : [])].join('\n')))
    .join('\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim()

// ---- the tenant filter: one tenant's view of a model ----

const only = (l: MList, slug: string, counted?: number): MList => {
  const items = l.items.filter(i => i.tenant === slug)
  // a list the model cut: what the tenant's own count says is hidden stays "more"
  return { items, more: counted === undefined ? 0 : Math.max(0, counted - items.length) }
}

/**
 * The model as one tenant sees it: every list holds only that tenant's items, the counts are the tenant's row of `now.tenants`
 * (else what the lists hold), the headline included. Metrics are the job's and stay as they are.
 */
export const filterModel = (m: Model, slug: string): Model => {
  const row = m.now.tenants.find(t => t.slug === slug)
  const c = row ? { needs: row.needs, review: row.review, blocked: row.blocked, doing: row.doing, ready: row.ready, done: row.done, total: row.total } : undefined
  const needs = only(m.now.needs, slug, c?.needs)
  const review = only(m.now.review, slug, c?.review)
  const blocked = only(m.now.blocked, slug, c?.blocked)
  const doing = only(m.now.doing, slug, c?.doing)
  const ready = only(m.next.ready, slug, c?.ready)
  const hp = (l: MList) => only(l, slug)
  const happened = {
    closed_milestones: hp(m.happened.closed_milestones),
    done: hp(m.happened.done),
    accepted: hp(m.happened.accepted),
    sent_back: hp(m.happened.sent_back),
    answers: hp(m.happened.answers),
    decisions: hp(m.happened.decisions),
    new_items: hp(m.happened.new_items),
    moved_to_review: hp(m.happened.moved_to_review),
    moved_to_doing: hp(m.happened.moved_to_doing),
    counts: {} as Record<string, number>,
  }
  for (const k of [...HAPPENED_KEYS, 'moved_to_review', 'moved_to_doing'] as const) happened.counts[k] = happened[k].items.length
  const miles = only(m.now.milestones, slug)
  return {
    ...m,
    only: slug,
    release: { ...m.release, changes: undefined },
    now: {
      counts: c ?? { needs: needs.items.length, review: review.items.length, blocked: blocked.items.length, doing: doing.items.length, ready: ready.items.length, done: 0, total: 0 },
      needs,
      review,
      blocked,
      doing,
      milestones: miles,
      backlog: { ...only(m.now.backlog, slug, row ? row.ready + row.draft : undefined), count: row ? row.ready + row.draft : m.now.backlog.items.filter(i => i.tenant === slug).length },
      recentDone: only(m.now.recentDone, slug),
      sprint: m.now.sprint && m.now.sprint.tenant === slug ? m.now.sprint : null,
      closed: 0,
      tenants: row ? [row] : [],
      tenantsMore: 0,
    },
    happened,
    next: { ready, next_milestones: only(m.next.next_milestones, slug) },
  }
}
