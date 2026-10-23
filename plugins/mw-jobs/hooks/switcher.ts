// Pure code: switching between many jobs and many tenants. Attention order, the top bar's fit rule, favorites, the paged
// summary pages (⌂) and pickers (☰) with letter jumps, and the one-line rows both panes draw. No `$` in here.
import { abbr, textBar } from './rows'

export type Counts = { needs: number; review: number; blocked: number; doing: number }
/** A job or a tenant as one row: its counts, progress and last change. `news` is the news pane's own tail. */
export type Entry = Counts & { id: string; title: string; done: number; total: number; last?: number; news?: string }

export const PAGE = 20

/** Attention order: needs desc, review desc, blocked desc, last change desc, id. */
export const attentionCmp = (a: Entry, b: Entry): number =>
  b.needs - a.needs || b.review - a.review || b.blocked - a.blocked || (b.last ?? 0) - (a.last ?? 0) || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0)

export const attentionSort = <T extends Entry>(rows: T[]): T[] => [...rows].sort(attentionCmp)

export const totalsOf = (rows: Counts[]): Counts => rows.reduce((t, r) => ({ needs: t.needs + r.needs, review: t.review + r.review, blocked: t.blocked + r.blocked, doing: t.doing + r.doing }), { needs: 0, review: 0, blocked: 0, doing: 0 })

// ---- pages and letter jumps ----

export const RANGES = [
  { key: 'a-f', label: 'a–f' },
  { key: 'g-l', label: 'g–l' },
  { key: 'm-r', label: 'm–r' },
  { key: 's-z', label: 's–z' },
  { key: '0-9', label: '0–9' },
] as const
export type RangeKey = (typeof RANGES)[number]['key']

export const inRange = (id: string, key: string): boolean => {
  const c = (id[0] ?? '').toLowerCase()
  if (key === '0-9') return c >= '0' && c <= '9'
  const m = /^([a-z])-([a-z])$/.exec(key)
  return m !== null && c >= m[1]! && c <= m[2]!
}

export type Paged<T> = { rows: T[]; page: number; pages: number; total: number }
export const paged = <T>(all: T[], page: number, size = PAGE): Paged<T> => {
  const pages = Math.max(1, Math.ceil(all.length / size))
  const p = Math.max(0, Math.min(pages - 1, Math.floor(page) || 0))
  return { rows: all.slice(p * size, p * size + size), page: p, pages, total: all.length }
}

/** The picker's state: closed (undefined), the ☰ picker or the ⌂ summary; page, letter range and favorites filter are kept after a click so Back lands on the same page. */
export type SwState = { view?: 'pick' | 'all'; page: number; range?: string; fav?: boolean }

/**
 * The rows a page lists. A letter range lists the slugs starting in it, alphabetical; the favorites filter lists them in the
 * order they were starred; otherwise attention order (the ☰ picker puts the rows that need you first as their own group).
 */
export const listFor = <T extends Entry>(all: T[], st: SwState, favs: string[], keepOrder: boolean): { rows: T[]; needs: number } => {
  if (st.fav) {
    const by = new Map(all.map(r => [r.id, r]))
    return { rows: favs.map(id => by.get(id)).filter((r): r is T => r !== undefined), needs: 0 }
  }
  if (st.range !== undefined) return { rows: all.filter(r => inRange(r.id, st.range!)).sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0)), needs: 0 }
  const ordered = keepOrder ? all : attentionSort(all)
  if (st.view === 'pick') {
    const top = ordered.filter(r => r.needs > 0)
    return { rows: [...top, ...ordered.filter(r => r.needs === 0)], needs: top.length }
  }
  return { rows: ordered, needs: 0 }
}

// ---- favorites: { jobs: [ids], tenants: { jobId: [slugs] } }, in the order they were starred ----

export type Favorites = { jobs: string[]; tenants: Record<string, string[]> }
export const noFavorites = (): Favorites => ({ jobs: [], tenants: {} })

const strs = (v: unknown, max = 5000): string[] => (Array.isArray(v) ? [...new Set(v.filter((x): x is string => typeof x === 'string' && x !== ''))].slice(0, max) : [])

/** Reads the stored shape defensively; anything else is empty. */
export const parseFavorites = (v: unknown): Favorites => {
  const o = v !== null && typeof v === 'object' ? (v as Record<string, unknown>) : {}
  const t = o.tenants !== null && typeof o.tenants === 'object' && !Array.isArray(o.tenants) ? (o.tenants as Record<string, unknown>) : {}
  const tenants: Record<string, string[]> = {}
  for (const [k, list] of Object.entries(t)) {
    const l = strs(list)
    if (l.length > 0) tenants[k] = l
  }
  return { jobs: strs(o.jobs), tenants }
}

/** Drops ids that no longer exist, quietly. `slugs(jobId)` is undefined while a job's tenants are not known: its list is kept. */
export const cleanFavorites = (f: Favorites, jobIds: string[], slugs: (jobId: string) => string[] | undefined): Favorites => {
  const tenants: Record<string, string[]> = {}
  for (const [jid, list] of Object.entries(f.tenants)) {
    if (!jobIds.includes(jid)) continue
    const known = slugs(jid)
    const l = known === undefined ? list : list.filter(s => known.includes(s))
    if (l.length > 0) tenants[jid] = l
  }
  return { jobs: f.jobs.filter(id => jobIds.includes(id)), tenants }
}

export const toggleIn = (list: string[], id: string): string[] => (list.includes(id) ? list.filter(x => x !== id) : [...list, id])
export const moveIn = (list: string[], id: string, dir: -1 | 1): string[] => {
  const i = list.indexOf(id)
  const j = i + dir
  if (i < 0 || j < 0 || j >= list.length) return list
  const out = [...list]
  out[i] = out[j]!
  out[j] = id
  return out
}

// ---- the top bar: every item as a tab when they fit, else favorites (or the automatic few) + ◂ ▸ ☰ ----

export type Tab = { id: string; label: string; needs: number; review: number }

/** Width of the real pane in columns: bodyColumns, scaled down when the size step makes the text larger. */
export const fitCols = (bodyColumns: number | undefined, sizePx: number): number => Math.max(10, Math.floor(((bodyColumns && bodyColumns > 0 ? bodyColumns : 52) * 12) / Math.max(8, sizePx)))

export const LABEL_MAX = 14
export const cutLabel = (s: string, n = LABEL_MAX): string => (s.length <= n ? s : `${s.slice(0, Math.max(1, n - 1))}~`)
export const badgeOf = (t: { needs: number; review: number }): string => `${t.needs > 0 ? `!${t.needs}` : ''}${t.review > 0 ? `◐${t.review}` : ''}`
/** A tab's width: an optional badge + one space, its label, plus the `|` before it (no padding: icons sit one after another). */
export const tabWidth = (t: Tab): number => 1 + cutLabel(t.label).length + (badgeOf(t) === '' ? 0 : badgeOf(t).length + 1)

export type BarPlan =
  | { mode: 'all'; tabs: Tab[] }
  | { mode: 'compact'; tabs: Tab[]; plus: number; cur: Tab | undefined; count: number }

/** How many columns the compact group (`◂ | cur | ▸ | ☰ N`) takes. */
const compactWidth = (cur: Tab | undefined, n: number): number => 2 + (cur ? tabWidth(cur) : 2) + 3 + 2 + String(n).length + 1

export const planBar = (o: { tabs: Tab[]; current: string; favs: string[]; cols: number; lead: number; reserve?: number; auto?: number }): BarPlan => {
  const room = o.cols - (o.reserve ?? 0) - o.lead
  const all = o.tabs.reduce((n, t) => n + tabWidth(t), 0)
  if (all <= room) return { mode: 'all', tabs: o.tabs }
  const cur = o.tabs.find(t => t.id === o.current) ?? o.tabs[0]
  let left = room - compactWidth(cur, o.tabs.length)
  const byId = new Map(o.tabs.map(t => [t.id, t]))
  const favTabs = o.favs.map(id => byId.get(id)).filter((t): t is Tab => t !== undefined && t.id !== cur?.id)
  const useFavs = o.favs.length > 0
  const pool = useFavs ? favTabs : o.tabs.filter(t => t.id !== cur?.id).slice(0, o.auto ?? 3)
  // the +N tab takes room only when something is hidden
  const PLUS = 4
  const taken: Tab[] = []
  for (const t of pool) {
    const w = tabWidth(t)
    const rest = pool.length - taken.length - 1
    if (w + (useFavs && rest > 0 ? PLUS : 0) > left) break
    taken.push(t)
    left -= w
  }
  return { mode: 'compact', tabs: taken, plus: useFavs ? favTabs.length - taken.length : 0, cur, count: o.tabs.length }
}

// ---- rows ----

export const hhmm = (ms: number | undefined): string => {
  if (ms === undefined) return '--:--'
  const d = new Date(ms)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

export const cut = (s: string, n: number): string => (n <= 0 ? '' : s.length <= n ? s : n === 1 ? '~' : `${s.slice(0, n - 1)}~`)

/** Long keys are cut with ~ to keep columns aligned. */
export const keyCol = (s: string, n: number): string => cut(s, n).padEnd(n)

export type RowStyle = 'text' | 'gopher' | 'loz'
export const countsText = (c: Counts, style: RowStyle = 'text'): string => {
  if (style === 'loz') return [`!${c.needs}`, `◐${c.review}`, `⛔${c.blocked}`, `▶${c.doing}`].map(x => `[${x}]`).join('') + ' '
  const segs = style === 'gopher' ? [`!${c.needs}`, `?${c.review}`, `-${c.blocked}`, `+${c.doing}`] : [`!${c.needs}`, `◐${c.review}`, `⛔${c.blocked}`, `▶${c.doing}`]
  return segs.map(s => s.padEnd(4)).join('')
}
export const countsShort = (c: Counts): string => `${c.needs > 0 ? `!${c.needs}` : ''}${c.review > 0 ? ` ◐${c.review}` : ''}`.trim()

/** `left` + title + dot leaders + `right`, exactly `w` characters (the title is cut with ~ first, then the right part). */
export const fitMid = (left: string, title: string, right: string, w: number): string => {
  const r = cut(right, Math.max(0, w))
  const l = cut(left, Math.max(0, w - r.length))
  const room = w - l.length - r.length
  if (room <= 1) return `${l}${' '.repeat(Math.max(0, room))}${r}`.slice(0, w)
  const t = cut(title, room - 1)
  const dots = room - t.length - 1
  return `${l}${t} ${dots > 1 ? '.'.repeat(dots - 1) + ' ' : ' '.repeat(Math.max(0, dots))}${r}`.slice(0, w).padEnd(w)
}

const progress = (e: Entry) => `${abbr(e.done)}/${abbr(e.total)} ${textBar(e.total > 0 ? Math.round((e.done / e.total) * 100) : 0, 8)}`

/** The jobs pane's summary line for a job or a tenant: `!N ◐N ⛔N ▶N  key  name ........ done/total [bar]  14:02`. */
export const summaryLine = (e: Entry, w: number, style: RowStyle = 'text', o: { time?: boolean; keyW?: number } = {}): string => {
  const kw = o.keyW ?? 10
  const left = style === 'gopher' ? `1 ${countsText(e, 'gopher')}${keyCol(e.id, kw)}  ` : `${countsText(e, style)}${keyCol(e.id, kw)}  `
  const right = ` ${progress(e)}${o.time === false ? '' : `  ${hhmm(e.last)}`}`
  return fitMid(left, e.title, right, w)
}

/** The picker's line (☰): `!N ◐N  key  name  tail`, the tail being the last change for a job and done/total for a tenant. */
export const pickLine = (e: Entry, w: number, tail: string, o: { keyW?: number } = {}): string => {
  const kw = o.keyW ?? 10
  return fitMid(`${countsShort(e).padEnd(8)}${keyCol(e.id, kw)}  `, e.title, ` ${tail}`, w)
}

/** The news pane's job line: `id  title  issue N (date) · draft: K changes  !N`. */
export const newsLine = (e: Entry, w: number, style: RowStyle = 'text', o: { keyW?: number } = {}): string => {
  const kw = o.keyW ?? 10
  const tail = `${e.news ?? ''}${e.needs > 0 ? `  !${e.needs}` : ''}`
  return fitMid(style === 'gopher' ? `1 ${keyCol(e.id, kw)}  ` : `${keyCol(e.id, kw)}  `, e.title, ` ${tail}`.trimEnd() === '' ? '' : ` ${tail}`, w)
}

export const pagerLabel = (p: { page: number; pages: number }): string => `page ${p.page + 1}/${p.pages}`

// ---- which jobs one poll refreshes ----

export const POLL_OTHERS = 5

/** The jobs a poll refreshes: the `must` ones (current, quick tabs, the news job), then the others round-robin, at most `limit`, least recently polled first. */
export const pollPlan = (ids: string[], must: string[], polled: Map<string, number>, limit = POLL_OTHERS): string[] => {
  const mustSet = new Set(must.filter(id => ids.includes(id)))
  const others = ids.filter(id => !mustSet.has(id)).sort((a, b) => (polled.get(a) ?? 0) - (polled.get(b) ?? 0) || ids.indexOf(a) - ids.indexOf(b))
  return [...ids.filter(id => mustSet.has(id)), ...others.slice(0, limit)]
}

/** The next `n` of a list round-robin from a cursor (wrapping); returns the slice and the new cursor. */
export const rrSlice = <T>(all: T[], cursor: number, n: number): { slice: T[]; cursor: number } => {
  if (all.length <= n) return { slice: all, cursor: 0 }
  const start = cursor % all.length
  const slice = Array.from({ length: n }, (_, i) => all[(start + i) % all.length]!)
  return { slice, cursor: (start + n) % all.length }
}
