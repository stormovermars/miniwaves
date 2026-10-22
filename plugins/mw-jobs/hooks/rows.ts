// Pure code: the rows both panes draw. A milestone, an item (needs you, review, blocked, in progress, done, ...) and a headline
// are described here once; the jobs pane and the news pane, in every style, take their text and numbers from these functions
// and only differ in the medium they paint on.
import type { MwJobsWave } from '../types'
/** How many rows of `len` to show under a cap of `n`: never leave exactly one row hidden ("+1 more" costs the same line as the row). */
export const capN = (len: number, n: number): number => (len === n + 1 ? len : Math.min(len, n))

/** Counts of 10,000 and more are abbreviated: 12.3k, 1.2M, 4B; beyond that an exponent (1e308). A value that rounds up to the next unit uses it (999,950 is 1M). */
export const abbr = (n: number): string => {
  const a = Math.abs(n)
  const sign = n < 0 ? '-' : ''
  const f = (v: number, u: string) => `${sign}${(Math.round(v * 10) / 10).toString().replace(/\.0$/, '')}${u}`
  if (a < 10_000) return String(Math.round(n * 100) / 100)
  const units: Array<[number, string]> = [[1e3, 'k'], [1e6, 'M'], [1e9, 'B']]
  for (let i = 0; i < units.length; i++) {
    const [div, u] = units[i]!
    const next = units[i + 1]?.[0] ?? 1e12
    if (a < next && Math.round((a / div) * 10) / 10 < next / div) return f(a / div, u)
  }
  return a < 1e12 ? f(a / 1e9, 'B') : n.toExponential(1).replace('e+', 'e').replace('.0e', 'e')
}


/** Short tenant labels: the first letter(s) left once the prefix shared by the job's tenants is stripped. */
const labelCache = new Map<string, Record<string, string>>()
export const tenantLabels = (tenants: string[]): Record<string, string> => {
  // O(n^2) in the tenants: cached per list (a job with a thousand tenants is asked on every render)
  const ck = tenants.length < 2 ? '' : tenants.join('\u0000')
  if (ck !== '') {
    const hit = labelCache.get(ck)
    if (hit !== undefined) return hit
  }
  const out = tenantLabelsRaw(tenants)
  if (ck !== '') {
    if (labelCache.size > 50) labelCache.clear()
    labelCache.set(ck, out)
  }
  return out
}
const tenantLabelsRaw = (tenants: string[]): Record<string, string> => {
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

/** The id as one pane shows it: `LABEL-ID` when the job has several tenants, else the id. */
export const labelledId = (tenant: string, id: string, labels: Record<string, string>): string => {
  const label = labels[tenant] ?? ''
  if (label === '' || id.toUpperCase().startsWith(label)) return id
  return `${label}-${id}`
}

export const waveName = (w: MwJobsWave, labels: Record<string, string>) => {
  const tenant = w.id.split(':')[0] ?? ''
  const token = w.title.split(' ')[0] ?? w.title
  return labelledId(tenant, token, labels)
}

export const memberParts = (w: MwJobsWave) => {
  const [d = '0', t = '0'] = w.members.split('/')
  return { done: Number(d) || 0, total: Number(t) || 0 }
}

// ---- milestones: `ID title  done/total  state  bar` ----

export type MileRow = {
  tenant: string
  id: string
  /** The id as shown: LABEL-ID with several tenants. */
  key: string
  /** tenant/ID, for the gopher menu lines. */
  full: string
  title: string
  done: number
  total: number
  /** open, not started, closed, or the board's own word (ready, blocked, ...). */
  state: string
  pct: number
  ticket?: string
  ticketUrl?: string
}

export const mileState = (done: number, total: number, closed: boolean, given?: string) =>
  closed ? 'closed' : total === 0 ? 'not started' : given && given !== 'open' && given !== 'done' ? given : 'open'

export const mileOf = (m: { tenant: string; id: string; title: string; done: number; total: number; state?: string; ticket?: string; ticketUrl?: string }, labels: Record<string, string>, closed = false): MileRow => ({
  tenant: m.tenant,
  id: m.id,
  key: labelledId(m.tenant, m.id, labels),
  full: m.tenant ? `${m.tenant}/${m.id}` : m.id,
  title: m.title,
  done: m.done,
  total: m.total,
  state: mileState(m.done, m.total, closed, m.state),
  pct: m.total > 0 ? Math.max(0, Math.min(100, Math.round((m.done / m.total) * 100))) : 0,
  ticket: m.ticket || undefined,
  ticketUrl: m.ticketUrl || undefined,
})

/** A board wave (`tenant:WAV-1`, title "WAV-1 name") as a milestone row. */
export const waveToMile = (w: MwJobsWave, labels: Record<string, string>): MileRow => {
  const { done, total } = memberParts(w)
  const tenant = w.id.split(':')[0] ?? ''
  return mileOf({ tenant, id: w.title.split(' ')[0] ?? w.id, title: w.title.split(' ').slice(1).join(' '), done, total, state: w.state }, labels, w.state === 'done')
}

export const mileName = (r: MileRow) => `${r.key} ${r.title}`.trim()
export const mileCount = (r: MileRow) => `${abbr(r.done)}/${abbr(r.total)}`

/** `[####......]` in `w` cells, the same bar in every text medium. */
export const textBar = (pct: number, w: number): string => {
  const f = Math.round((Math.max(0, Math.min(100, pct)) / 100) * w)
  return `[${'#'.repeat(f)}${'.'.repeat(w - f)}]`
}

// ---- items ----

export type ItemKind = 'needs' | 'review' | 'blocked' | 'doing' | 'ready' | 'done' | 'accepted' | 'sent_back' | 'decision' | 'answer' | 'closed'
export const STATE_WORD: Record<ItemKind, string> = { needs: 'needs you', review: 'review', blocked: 'blocked', doing: 'doing', ready: 'ready', done: 'done', accepted: 'accepted', sent_back: 'sent back', decision: 'decided', answer: 'answered', closed: 'closed' }
/** The gopher type character of each kind. */
export const TYPE_CHAR: Record<ItemKind, string> = { needs: '!', review: '?', blocked: '-', doing: '+', ready: '>', done: '0', accepted: '0', sent_back: '0', decision: '0', answer: '0', closed: '1' }

export type ItemRow = {
  kind: ItemKind
  tenant: string
  id: string
  /** The id as shown (label prefix with several tenants). */
  key: string
  full: string
  title: string
  /** The tracker label (MOB-142, #123, ...) and its link, when mw gave them. */
  ticket?: string
  ticketUrl?: string
  /** Right-hand state: the kind's word, or the age when the pane knows one. */
  state: string
  age: string
  note: string
}

export const itemRow = (kind: ItemKind, i: { tenant: string; id: string; title: string; note?: string; ticket?: string; ticketUrl?: string }, labels: Record<string, string>, age = ''): ItemRow => ({
  kind,
  tenant: i.tenant,
  id: i.id,
  key: labelledId(i.tenant, i.id, labels),
  full: i.tenant ? `${i.tenant}/${i.id}` : i.id,
  title: i.title,
  ticket: i.ticket || undefined,
  ticketUrl: i.ticketUrl || undefined,
  state: STATE_WORD[kind],
  age,
  note: i.note ?? '',
})

/** `KEY title [TICKET]`: the one text every non-gopher style draws for an item. */
export const itemName = (r: ItemRow) => `${r.key} ${r.title}${r.ticket ? ` [${r.ticket}]` : ''}`.trim()

// ---- text measured and cut by grapheme and display width (S4) ----

const segmenter: { segment: (s: string) => Iterable<{ segment: string }> } | undefined =
  typeof Intl !== 'undefined' && 'Segmenter' in Intl ? new (Intl as unknown as { Segmenter: new (l?: string, o?: { granularity: string }) => { segment: (s: string) => Iterable<{ segment: string }> } }).Segmenter(undefined, { granularity: 'grapheme' }) : undefined

export const graphemes = (s: string): string[] => (segmenter ? Array.from(segmenter.segment(s), x => x.segment) : Array.from(s))

/** Terminal cells of one grapheme: 0 for control and combining, 2 for East Asian wide and emoji, else 1. */
export const graphemeWidth = (g: string): number => {
  const cp = g.codePointAt(0) ?? 0
  if (cp < 0x20 || (cp >= 0x7f && cp < 0xa0)) return 0
  if ((cp >= 0x300 && cp <= 0x36f) || (cp >= 0x200b && cp <= 0x200f) || (cp >= 0xfe00 && cp <= 0xfe0f) || (cp >= 0x202a && cp <= 0x202e) || (cp >= 0x2066 && cp <= 0x2069)) return 0
  if (
    (cp >= 0x1100 && cp <= 0x115f) || (cp >= 0x2e80 && cp <= 0xa4cf) || (cp >= 0xac00 && cp <= 0xd7a3) || (cp >= 0xf900 && cp <= 0xfaff) ||
    (cp >= 0xfe30 && cp <= 0xfe6f) || (cp >= 0xff00 && cp <= 0xff60) || (cp >= 0xffe0 && cp <= 0xffe6) || (cp >= 0x1f000 && cp <= 0x1faff) || (cp >= 0x20000 && cp <= 0x3fffd)
  ) return 2
  return 1
}

export const cellWidth = (s: string): number => (/^[\x20-\x7e]*$/.test(s) ? s.length : graphemes(s).reduce((n, g) => n + graphemeWidth(g), 0))

/** The longest prefix that fits `n` cells, never splitting a grapheme or a surrogate pair. */
export const cutCells = (s: string, n: number): string => {
  if (/^[\x20-\x7e]*$/.test(s)) return s.length <= n ? s : s.slice(0, Math.max(0, n))
  let w = 0
  let out = ''
  for (const g of graphemes(s)) {
    const gw = graphemeWidth(g)
    if (w + gw > n) break
    out += g
    w += gw
  }
  return out
}

/** `s.slice(0, n)` by code units that never ends inside a surrogate pair or a grapheme. */
export const safeSlice = (s: string, n: number): string => {
  if (s.length <= n) return s
  let out = ''
  for (const g of graphemes(s)) {
    if (out.length + g.length > n) break
    out += g
  }
  return out
}

/** Bidi controls, zero-width characters and the like: never drawn, never kept in board text (S5). */
export const INVISIBLE = /[​-‏‪-‮⁦-⁩﻿­⁠]/g
