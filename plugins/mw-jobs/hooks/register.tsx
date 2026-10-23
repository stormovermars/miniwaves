import { atom, read, update } from 'claude-code'
import type { Register } from 'claude-code'

import type { Counts, Entry, Tab, SwState, Favorites } from './switcher'
import { pollPlan, rrSlice, attentionSort, badgeOf, cleanFavorites, countsShort, cutLabel, fitCols, hhmm as hhmmMs, inRange, keyCol, listFor, moveIn, newsLine, pagerLabel, paged, parseFavorites, pickLine, planBar, RANGES, summaryLine, toggleIn, totalsOf, PAGE } from './switcher'
import type { ItemRow, MileRow } from './rows'
import { buildIdIndex, splitIds, type IdIndex, type IdRef } from './ids'
import { capN, cellWidth, cutCells, safeSlice, INVISIBLE, memberParts, tenantLabels, waveName, mileOf, waveToMile, mileName, mileCount, textBar, itemRow, itemName } from './rows'
export { tenantLabels, waveName }
import { parseItemDetail, parseWaveDetail } from './detail'
import { abbr, topOf, filterModel, tenantHidden, withTenantHidden, refOf, bulletin, bulletinMd, changeCount, flowLines, metricLines, newsMd, newsSegments, nowSheet, parseModel, relHeadline, releaseDate, shortLines, snapshotFromModel, total, BULLETIN_MAX, GOPHER_MIN } from './bulletin'
import { agileJobs, agileNews, kanbanJobs, kanbanNews, flowText } from './flow'
import type { FDoc, FRow } from './flow'
import type { BLine, MList, Model, MViewSection, NowView } from './bulletin'

import type {
  ModelBrief,
  MwJobsAsk,
  MwJobsEvent,
  MwMetricRow,
  MwMetricSection,
  MwMetrics,
  MwJobsItem,
  MwJobsFanfare, MwJobsUsedBoard, MwJobsDetail, MwJobsDetailRef, MwJobsAlertRow, MwJobsCfgKey, MwJobsSettings,
  MwJobsJob,
  MwJobsRegistry,
  MwJobsSeen,
  MwJobsSnapshot,
  MwJobsWave,
} from '../types'

const PANE = 'mw-jobs'
/** How long mw_panes_open waits for the job list before it answers "still looking". */
const PANES_OPEN_WAIT_MS = 5000
const NEWS_PANE = 'mw-news'
/** The one detail pane both panes open (an item or a milestone), read only. */
export const DETAIL_PANE = 'mw-detail'
export const BACK_LABEL = '< Back'
// the Markdown element refuses a text over 10,000 characters, so the file is cut well inside that
const NEWS_MAX = 9500
const NEWS_MISSING = 'No docs/news/status.md yet — the news desk writes it on its next update.'
const REGISTRY_DEFAULT = '~/.mw/jobs.json'
const EVERY = ['off', '30s', '1m', '5m'] as const
/** Every refresh value a setting may hold (pane.refresh adds 15s; the ⏱ button cycles EVERY). */
const EVERY_ALL: string[] = ['off', '15s', '30s', '1m', '5m']
const EVERY_MS: Record<string, number> = { '15s': 15_000, '30s': 30_000, '1m': 60_000, '5m': 300_000 }
/** The zoom steps: -2 … +2 (pane.zoom). */
export const STEP_MAX = 2

const registry = atom({ plugin: 'mw-jobs', key: 'registry' } as const, { jobs: [], error: null })
const snapshots = atom({ plugin: 'mw-jobs', key: 'snapshots' } as const, {})
const previous = atom({ plugin: 'mw-jobs', key: 'previous' } as const, {})
const staleSince = atom({ plugin: 'mw-jobs', key: 'staleSince' } as const, {})
const selected = atom({ plugin: 'mw-jobs', key: 'selected' } as const, '')
const muted = atom({ plugin: 'mw-jobs', key: 'muted' } as const, false)
/** alerts.popup as last read from mw (on until a read says off); the panes draw from this atom. */
const popupAtom = atom({ plugin: 'mw-jobs', key: 'popup' } as const, true)
const refreshEvery = atom({ plugin: 'mw-jobs', key: 'refreshEvery' } as const, '30s')
const newsSeen = atom({ plugin: 'mw-jobs', key: 'newsSeen' } as const, {})
const jobDesc = atom({ plugin: 'mw-jobs', key: 'jobDesc' } as const, {})
const inChat = atom({ plugin: 'mw-jobs', key: 'inChat' } as const, {})
const alerted = atom({ plugin: 'mw-jobs', key: 'alerted' } as const, {})
const newsOut = atom({ plugin: 'mw-jobs', key: 'newsOut' } as const, {})
const news = atom({ plugin: 'mw-jobs', key: 'news' } as const, {})
const newsModel = atom({ plugin: 'mw-jobs', key: 'newsModel' } as const, {})
const newsView = atom({ plugin: 'mw-jobs', key: 'newsView' } as const, {})
const relKnown = atom({ plugin: 'mw-jobs', key: 'relKnown' } as const, {})
const expanded = atom({ plugin: 'mw-jobs', key: 'expanded' } as const, {})
const newsJob = atom({ plugin: 'mw-jobs', key: 'newsJob' } as const, '')
const seen = atom({ plugin: 'mw-jobs', key: 'seen' } as const, {})
const fontStep = atom({ plugin: 'mw-jobs', key: 'fontStep' } as const, 0)
const newsStyle = atom({ plugin: 'mw-jobs', key: 'newsStyle' } as const, 'normal')
const barStyle = atom({ plugin: 'mw-jobs', key: 'barStyle' } as const, 'normal')
const stalledMin = atom({ plugin: 'mw-jobs', key: 'stalledMin' } as const, 45)
const busy = atom({ plugin: 'mw-jobs', key: 'busy' } as const, [])
const releasing = atom({ plugin: 'mw-jobs', key: 'releasing' } as const, [])
const sw = atom({ plugin: 'mw-jobs', key: 'sw' } as const, {})
const favorites = atom({ plugin: 'mw-jobs', key: 'favorites' } as const, { jobs: [], tenants: {} })
const tenantSel = atom({ plugin: 'mw-jobs', key: 'tenantSel' } as const, {})
const blocksFill = atom({ plugin: 'mw-jobs', key: 'blocksFill' } as const, { jobs: false, news: false })
const chosen = atom({ plugin: 'mw-jobs', key: 'chosen' } as const, false)
const fanfare = atom({ plugin: 'mw-jobs', key: 'fanfare' } as const, {})
const usedBoards = atom({ plugin: 'mw-jobs', key: 'usedBoards' } as const, [])
const detail = atom({ plugin: 'mw-jobs', key: 'detail' } as const, { cur: null, trail: [], from: 'jobs' })
const detailData = atom({ plugin: 'mw-jobs', key: 'detailData' } as const, {})
const alertRows = atom({ plugin: 'mw-jobs', key: 'alertRows' } as const, [])
const settings = atom({ plugin: 'mw-jobs', key: 'settings' } as const, { ok: null, keys: [], hint: '', err: {} })

const two = (n: number) => (n < 10 ? `0${n}` : `${n}`)
const hhmm = (ms: number) => {
  const d = new Date(ms)
  return `${two(d.getHours())}:${two(d.getMinutes())}`
}
const isoHhmm = (iso: string) => {
  const ms = Date.parse(iso)
  return Number.isNaN(ms) ? '--:--' : hhmm(ms)
}

/** An absolute path on any OS: `/x`, `C:\x` or `C:/x`, or a UNC `\\host`. */
export const isAbs = (p: string) => p.startsWith('/') || /^[A-Za-z]:[\\/]/.test(p) || p.startsWith('\\\\')
const expand = (path: string, home: string, base?: string) => {
  if (path === '~' || path.startsWith('~/') || path.startsWith('~\\')) return `${home}${path.slice(1)}`
  if (isAbs(path)) return path
  return base ? `${base}/${path}` : path
}

let homeCache: string | undefined
/** The home folder from a spawned shell (`printf %s "$HOME"`, or `echo %USERPROFILE%` on Windows); the plugin never reads HOME or USERPROFILE through env APIs, and never asks mw (resolveMw needs the home itself). Never cached when empty or failed: the next call asks again. */
const homeOf = async ($: any): Promise<string> => {
  if (homeCache !== undefined && homeCache !== '') return homeCache
  let home = ''
  try {
    const os = await osKind($)
    const r = await $.process.run(os === 'windows' ? ['cmd', '/c', 'echo %USERPROFILE%'] : ['/bin/sh', '-c', 'printf %s "$HOME"'], { cwd: '/', timeoutMs: 5000 })
    const out = r.exitCode === 0 ? String(r.stdout ?? '').trim() : ''
    if (isAbs(out) && !out.includes('%')) home = out.replace(/[\\/]+$/, '')
  } catch {
    home = ''
  }
  if (home !== '') homeCache = home
  return home
}

let osCache: 'windows' | 'mac' | 'linux' | undefined
/** The OS: Windows from the OS variable, else `uname -s` (a failed run counts as macOS). */
const osKind = async ($: any): Promise<'windows' | 'mac' | 'linux'> => {
  if (osCache !== undefined) return osCache
  let env: unknown
  try {
    env = await $.env.get('OS')
  } catch {
    env = undefined
  }
  if (env === 'Windows_NT') return (osCache = 'windows')
  try {
    const r = await $.process.run(['uname', '-s'], { timeoutMs: 5000 })
    const u = String(r.stdout ?? '').trim()
    if (r.exitCode === 0 && u !== '') return (osCache = u === 'Darwin' ? 'mac' : 'linux')
  } catch {
    // default
  }
  return (osCache = 'mac')
}

/** The command that opens a plain file in a text editor, per OS: `open -t`, `xdg-open`, `notepad.exe` (argv only, no shell string). */
export const openerArgv = (os: 'windows' | 'mac' | 'linux', path: string): string[] =>
  os === 'windows' ? ['notepad.exe', path] : os === 'linux' ? ['xdg-open', path] : ['open', '-t', path]

let mwBin: string | undefined
let mwProbed = false
/** The mw binary as an absolute path: PATH first, then the usual install folders (a desktop app's PATH lacks ~/.local/bin). Undefined when not found: the bare name is used. */
const resolveMw = async ($: any): Promise<string | undefined> => {
  if (mwBin !== undefined || mwProbed) return mwBin
  mwProbed = true
  const os = await osKind($)
  try {
    const r = await $.process.run(os === 'windows' ? ['where.exe', 'mw'] : ['/bin/sh', '-c', 'command -v mw'], { timeoutMs: 5000 })
    const first = String(r.stdout ?? '').split(/\r?\n/)[0]?.trim() ?? ''
    if (r.exitCode === 0 && isAbs(first)) return (mwBin = first)
  } catch {
    // fall through to the known folders
  }
  const home = await homeOf($)
  // no home yet (a timeout, an empty answer): do not keep "not found"; the next call probes again
  if (home === '') mwProbed = false
  let local = ''
  try {
    local = (await $.env.get('LOCALAPPDATA')) ?? ''
  } catch {
    local = ''
  }
  const known =
    os === 'windows'
      ? [...(home !== '' ? [`${home}\\.local\\bin\\mw.exe`] : []), ...(local !== '' ? [`${local}\\Programs\\mw\\mw.exe`, `${local}\\mw\\mw.exe`] : [])]
      : [...(home !== '' ? [`${home}/.local/bin/mw`, `${home}/go/bin/mw`] : []), '/opt/homebrew/bin/mw', '/usr/local/bin/mw', '/usr/bin/mw']
  for (const p of known) {
    try {
      if (await $.fs.exists(p)) return (mwBin = p)
    } catch {
      // next
    }
  }
  return undefined
}

/** Every process the panes run: `mw` becomes its absolute path when found; anything else runs as given. */
const proc = async ($: any, argv: string[], init?: Record<string, unknown>): Promise<any> => {
  const bin = argv[0] === 'mw' ? await resolveMw($) : undefined
  return $.process.run(bin !== undefined ? [bin, ...argv.slice(1)] : argv, init)
}

const isPlaceholder = (w: MwJobsWave) => w.state === 'done' && memberParts(w).total === 0

const bar = (percent: number, width = 10) => {
  const filled = Math.max(0, Math.min(width, Math.round((percent / 100) * width)))
  return '█'.repeat(filled) + '░'.repeat(width - filled)
}


const WORK_TYPES = new Set(['task', 'bug', 'spike', 'chore'])
const isReview = (i: MwJobsItem) =>
  i.state === 'review' || (i.state === 'blocked' && /awaiting review/i.test(i.note))
/** Open asks of a snapshot by `tenant:item` (from `mw review -j` needs). */
export const askMap = (s: MwJobsSnapshot | undefined): Map<string, MwJobsAsk> => new Map((s?.asks ?? []).map(a => [`${a.tenant}:${a.item}`, a]))
/** Needs you means exactly two things: an open ask, or a blocked item. A name in a title or note counts for nothing. */
export const inChatNow = (i: MwJobsItem, s: MwJobsSnapshot | undefined) => s?.inChat?.[`${i.tenant}:${i.id}`] !== undefined
export const isNeedIn = (i: MwJobsItem, s: MwJobsSnapshot | undefined) =>
  i.state !== 'done' && !isReview(i) && !inChatNow(i, s) && ((i.state === 'blocked' && s?.model === undefined) || askMap(s).has(`${i.tenant}:${i.id}`))
/** From a news model, blocked is its own group (not "needs you"). */
export const isBlockedIn = (i: MwJobsItem, s: MwJobsSnapshot | undefined) => s?.model !== undefined && i.state === 'blocked' && !isReview(i) && !inChatNow(i, s)
export const isReviewIn = (i: MwJobsItem, s: MwJobsSnapshot | undefined) => isReview(i) && !inChatNow(i, s)
/**
 * THE one count source: from a news model the model's own counts minus what is answered in chat now; from the old path the
 * item lists. The status line, the headline, the tabs, the toolbar, the summary pages and the news all read this.
 */
export const countsOf = (s: MwJobsSnapshot | undefined): Counts => {
  if (s === undefined) return { needs: 0, review: 0, blocked: 0, doing: 0 }
  if (s.model === undefined) {
    return {
      needs: s.items.filter(i => isNeedIn(i, s)).length,
      review: s.items.filter(i => isReviewIn(i, s)).length,
      blocked: s.items.filter(i => isBlockedIn(i, s)).length,
      doing: s.items.filter(i => i.state === 'doing').length,
    }
  }
  const chat = { needs: 0, review: 0, blocked: 0, doing: 0 }
  for (const i of s.items) {
    if (!inChatNow(i, s)) continue
    if (i.state === 'open') chat.needs += 1
    else if (i.state === 'review') chat.review += 1
    else if (i.state === 'blocked') chat.blocked += 1
    else if (i.state === 'doing') chat.doing += 1
  }
  const c = s.model.counts
  return { needs: Math.max(0, c.needs - chat.needs), review: Math.max(0, c.review - chat.review), blocked: Math.max(0, c.blocked - chat.blocked), doing: Math.max(0, c.doing - chat.doing) }
}
export const needCount = (s: MwJobsSnapshot | undefined) => countsOf(s).needs

/** An age; a time more than 5 minutes ahead of this clock reads "skew" (T9). */
export const ageText = (ms: number) => (ms < -300_000 ? 'skew' : ms < 3_600_000 ? `${Math.max(1, Math.round(ms / 60_000))}m` : `${Math.floor(ms / 3_600_000)}h`)

export type Analysis = {
  needs: MwJobsItem[]
  /** Blocked items, only when the snapshot came from a news model (else they are in needs). */
  blocked: MwJobsItem[]
  review: MwJobsItem[]
  /** Items the person is answering in chat right now: out of the counts, shown dim. */
  chat: MwJobsItem[]
  doing: { item: MwJobsItem; ageMs: number | undefined; isStalled: boolean }[]
  stalled: number
  doneNew: number
  blockedNew: number
  changed: Map<string, 'done' | 'other'>
}

/** What needs attention now, and what changed after the seen cursor. */
export const analyse = (s: MwJobsSnapshot, now: number, seenTs: number, stalledMs: number): Analysis => {
  const lastEvent = new Map<string, MwJobsEvent>()
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
    .filter(i => i.state === 'doing' && WORK_TYPES.has(i.type) && !isNeedIn(i, s) && !isBlockedIn(i, s) && !isReview(i) && !inChatNow(i, s))
    .map(item => {
      const stamps = [Date.parse(item.updated), Date.parse(lastEvent.get(`${item.tenant}:${item.id}`)?.ts ?? '')].filter(n => !Number.isNaN(n))
      const ageMs = stamps.length === 0 ? undefined : now - Math.max(...stamps)
      return { item, ageMs, isStalled: ageMs !== undefined && ageMs > stalledMs }
    })
  return {
    needs: s.items.filter(i => isNeedIn(i, s)),
    blocked: s.items.filter(i => isBlockedIn(i, s)),
    review: s.items.filter(i => isReviewIn(i, s)),
    chat: s.items.filter(i => i.state !== 'done' && inChatNow(i, s)),
    doing,
    stalled: doing.filter(d => d.isStalled).length,
    doneNew,
    blockedNew,
    changed,
  }
}

export const GROUP_BASE = 10
export const GROUP_STEP = 10
export const GROUP_MAX = 50
export const MILES_MAX = 8
export const METRIC_ROWS_MAX = 8
/** Sections given up first when a pane would be refused, lowest priority first. */
export const TRIM_ORDER = ['metrics', 'milestones', 'progress'] as const
/** A drawing above 80% of the limit (Svg 131072, text 100000) is trimmed. */
export const SVG_BUDGET = 104_000
export const TEXT_BUDGET = 80_000

/** One group's rows: the first `shown` (10 by default, never over 50) and how many stay hidden (`extra` = what the model cut already). */
export function capList<T>(list: T[], shown: number | undefined, extra = 0): { rows: T[]; hidden: number; more: boolean; shown: number } {
  const n = Math.min(GROUP_MAX, Math.max(GROUP_BASE, shown ?? GROUP_BASE))
  const rows = list.slice(0, n < GROUP_MAX ? capN(list.length + extra, n) : n)
  return { rows, hidden: list.length - rows.length + extra, more: rows.length < Math.min(list.length, GROUP_MAX), shown: n }
}

/** The jobs headline from a news model: `!needs ◐review ⛔blocked ▶doing`, counts including what the lists left out. */
export const modelHeadline = (a: Analysis, b: ModelBrief, c?: Counts): Headline => {
  const n = c ? c.needs : a.needs.length + b.more.needs
  const r = c ? c.review : a.review.length + b.more.review
  const bl = c ? c.blocked : a.blocked.length + b.more.blocked
  const d = c ? c.doing : a.doing.length + b.more.doing
  const text = `!${n} \u25d0${r} \u26d4${bl} \u25b6${d}`
  if (n > 0) return { text, tone: 'error', said: 'needs' }
  if (r > 0 || bl > 0) return { text, tone: 'warning', said: 'review' }
  return { text, tone: 'success', said: 'none' }
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

/** Up to this many jobs the status line lists each job; beyond it, the totals and the selected job. */
export const STATUS_FULL = 4

/** `mw ▸ 100 jobs !7 ◐3 │ selected ▸ …`, at most 200 characters. Counts come from the one count source. */
export const statusTotals = (snaps: Array<MwJobsSnapshot | undefined>, selected: string): string => {
  const t = totalsOf(snaps.map(countsOf))
  const parts = [t.needs > 0 ? `!${t.needs}` : '', t.review > 0 ? `\u25d0${t.review}` : '', t.blocked > 0 ? `\u26d4${t.blocked}` : ''].filter(Boolean)
  return `mw \u25b8 ${snaps.length} jobs${parts.length > 0 ? ` ${parts.join(' ')}` : ''}${selected !== '' ? ` \u2502 ${selected}` : ''}`.slice(0, 200)
}

/** Ready items of a snapshot: the model's count, else the listed ones. */
export const readyOf = (s: MwJobsSnapshot | undefined): number => s?.model?.ready ?? (s?.items ?? []).filter(i => i.state === 'ready').length

/**
 * A board found by scan that has no mw job: its tenant rows make one segment, `<board name> ▸ ▶ready ⛔blocked`
 * (⛔ only when something is blocked), "loading" until any of its tenants has a snapshot.
 */
export const boardSegment = (name: string, snaps: Array<MwJobsSnapshot | undefined>, full = false): string => {
  const got = snaps.filter((x): x is MwJobsSnapshot => x !== undefined)
  if (got.length === 0) return `${name} ▸ loading`.slice(0, 120)
  const ready = got.reduce((n, x) => n + readyOf(x), 0)
  const blocked = got.reduce((n, x) => n + (x.model !== undefined ? x.model.counts.blocked : x.items.filter(i => i.state === 'blocked').length), 0)
  const done = got.reduce((n, x) => n + doneOf(x).done, 0)
  const total = got.reduce((n, x) => n + doneOf(x).total, 0)
  return `${name} ▸ \u25b6${ready}${blocked > 0 || full ? ` \u26d4${blocked}` : ''}${full ? ` \u2713${done}/${total}` : ''}`.slice(0, 120)
}

/** The status units: a job is one, the tenant rows of one scanned board with no jobs are one together (in first-seen order). */
export const statusUnits = (jobs: MwJobsJob[]): Array<{ job: MwJobsJob } | { board: string; jobs: MwJobsJob[] }> => {
  const out: Array<{ job: MwJobsJob } | { board: string; jobs: MwJobsJob[] }> = []
  const byBoard = new Map<string, { board: string; jobs: MwJobsJob[] }>()
  for (const j of jobs) {
    if (j.scanned !== 'tenant') {
      out.push({ job: j })
      continue
    }
    const key = j.board
    let u = byBoard.get(key)
    if (u === undefined) {
      u = { board: j.boardName || key.split('/').filter(Boolean).at(-1) || key, jobs: [] }
      byBoard.set(key, u)
      out.push(u)
    }
    u.jobs.push(j)
  }
  return out
}

/** Done / total items of a snapshot (the model's, else the item list). */
export const doneOf = (s: MwJobsSnapshot): { done: number; total: number } =>
  s.model !== undefined ? { done: s.model.done, total: s.model.total } : { done: s.items.filter(i => i.state === 'done').length, total: s.items.length }

export const statusSegment = (
  job: MwJobsJob,
  s: MwJobsSnapshot | undefined,
  stale: number | undefined,
  full = false,
) => {
  if (s === undefined) return `${job.id} ▸ ${stale === undefined ? 'loading' : 'mw unavailable'}`
  // counts only, never a list of waves: !needs ◐review ⛔blocked ▶doing, at most 120 characters; status.detail=full adds ✓done/total and ⛔0
  const { needs, review, blocked, doing } = countsOf(s)
  const d = doneOf(s)
  const parts = [needs > 0 ? `!${needs}` : '', review > 0 ? `\u25d0${review}` : '', blocked > 0 || full ? `\u26d4${blocked}` : '', doing > 0 ? `\u25b6${doing}` : '', full ? `\u2713${d.done}/${d.total}` : ''].filter(Boolean)
  const tail = stale === undefined ? '' : ` (stale since ${hhmm(stale)})`
  return `${job.id} ▸ ${parts.length > 0 ? parts.join(' ') : 'idle'}${tail}`.slice(0, 120)
}

export const diffChanges = (
  prev: MwJobsSnapshot,
  next: MwJobsSnapshot,
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

const asWaves = (v: unknown): MwJobsWave[] | undefined => {
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

const asItems = (v: unknown): MwJobsItem[] | undefined => {
  const list = Array.isArray(v) ? v : (v as { items?: unknown } | undefined)?.items
  if (!Array.isArray(list)) return undefined
  return list.map((i: Record<string, unknown>) => ({
    tenant: str(i.tenant),
    id: str(i.id),
    type: str(i.type),
    state: str(i.state),
    title: cleanTitle(str(i.title)),
    note: str(i.note),
    updated: str(i.updated),
  }))
}

const asEvents = (v: unknown): MwJobsEvent[] | undefined => {
  const list = Array.isArray(v) ? v : (v as { events?: unknown } | undefined)?.events
  if (!Array.isArray(list)) return undefined
  return list.map((e: Record<string, unknown>) => ({
    ts: str(e.ts),
    tenant: str(e.tenant),
    item: str(e.item),
    action: str(e.action),
    note: str(e.note),
    by: str(e.by),
  }))
}

// ---- metrics.json (docs/specs/metrics.md): any job's own measures, read with $.fs only ----
export const METRICS_MAX_BYTES = 256 * 1024
const METRICS_SECTIONS = 8
const METRICS_ROWS = 40
const ctl = /[\u0000-\u001f\u007f-\u009f\u2028\u2029]/g

export type MetricsResult = { metrics?: MwMetrics; notice?: string }

/** Parses and limit-checks a metrics file: an invalid one gives "metrics file invalid: REASON", a newer version "newer metrics format". */
export const parseMetrics = (text: string, bytes = text.length): MetricsResult => {
  const bad = (reason: string): MetricsResult => ({ notice: `metrics file invalid: ${reason}` })
  if (bytes > METRICS_MAX_BYTES) return bad('file over 256 KB')
  let doc: unknown
  try {
    doc = JSON.parse(text)
  } catch {
    return bad('not JSON')
  }
  if (doc === null || typeof doc !== 'object' || Array.isArray(doc)) return bad('not an object')
  const d = doc as Record<string, unknown>
  if (typeof d.version !== 'number' || !Number.isFinite(d.version)) return bad('no version')
  if (d.version > 1) return { notice: 'newer metrics format' }
  if (!Array.isArray(d.sections)) return bad('no sections')
  if (d.sections.length > METRICS_SECTIONS) return bad(`more than ${METRICS_SECTIONS} sections`)
  const txt = (v: unknown, max: number, what: string): string | undefined => {
    if (v === undefined) return undefined
    if (typeof v !== 'string') throw new Error(`${what} is not text`)
    const c = v.replace(ctl, ' ').replace(/\s+/g, ' ').trim()
    if (c.length > max) throw new Error(`${what} over ${max} characters`)
    return c
  }
  const num = (v: unknown, what: string, count: boolean): number | undefined => {
    if (v === undefined) return undefined
    if (typeof v !== 'number' || !Number.isFinite(v)) throw new Error(`${what} is not a finite number`)
    if (count && (!Number.isInteger(v) || v < 0)) throw new Error(`${what} is not a count`)
    return v
  }
  try {
    const sections: MwMetricSection[] = (d.sections as Array<Record<string, unknown>>).map(sec => {
      if (sec === null || typeof sec !== 'object') throw new Error('a section is not an object')
      const kind = sec.kind
      if (kind !== 'split' && kind !== 'progress' && kind !== 'value') throw new Error(`unknown kind ${String(kind).slice(0, 20)}`)
      const title = txt(sec.title, 60, 'title')
      if (title === undefined || title === '') throw new Error('a section has no title')
      if (!Array.isArray(sec.rows)) throw new Error('a section has no rows')
      if (sec.rows.length > METRICS_ROWS) throw new Error(`more than ${METRICS_ROWS} rows in ${title}`)
      const lg = (sec.legend ?? {}) as Record<string, unknown>
      const legend = {
        good: txt(lg.good, 60, 'legend') || 'pass',
        warn: txt(lg.warn, 60, 'legend') || 'partial',
        bad: txt(lg.bad, 60, 'legend') || 'fail',
        none: txt(lg.none, 60, 'legend') || 'skip',
      }
      const rows: MwMetricRow[] = (sec.rows as Array<Record<string, unknown>>).map(r => {
        if (r === null || typeof r !== 'object') throw new Error('a row is not an object')
        const name = txt(r.name, 60, 'name')
        if (name === undefined || name === '') throw new Error(`a row of ${title} has no name`)
        if (kind === 'split') {
          return { name, good: num(r.good, 'good', true) ?? 0, warn: num(r.warn, 'warn', true) ?? 0, bad: num(r.bad, 'bad', true) ?? 0, none: num(r.none, 'none', true) ?? 0 }
        }
        if (kind === 'progress') {
          const done = num(r.done, 'done', true) ?? 0
          const total = num(r.total, 'total', true) ?? 0
          if (done > total) throw new Error(`done over total in ${name}`)
          return { name, done, total }
        }
        const value = num(r.value, 'value', false)
        if (value === undefined) throw new Error(`no value in ${name}`)
        const better = r.better === 'lower' ? 'lower' : 'higher'
        return { name, value, unit: txt(r.unit, 60, 'unit'), target: num(r.target, 'target', false), better }
      })
      return { title, kind, legend, note: txt(sec.note, 120, 'note') || undefined, rows }
    })
    return { metrics: { updated: typeof d.updated === 'string' ? d.updated.slice(0, 40) : '', sections } }
  } catch (err) {
    return bad((err as Error).message)
  }
}

const rowKey = (sec: MwMetricSection, r: MwMetricRow) => `${sec.title}/${r.name}`
const primary = (sec: MwMetricSection, r: MwMetricRow): number[] => (sec.kind === 'split' ? [r.good ?? 0, r.bad ?? 0] : sec.kind === 'progress' ? [r.done ?? 0, 0] : [r.value ?? 0, 0])
/** What the seen cursor remembers per `section/row`: [good|done|value, bad]. */
export const metricsBase = (s: MwJobsSnapshot | undefined): Record<string, number[]> => {
  const out: Record<string, number[]> = {}
  for (const sec of s?.metrics?.sections ?? []) for (const r of sec.rows) out[rowKey(sec, r)] = primary(sec, r)
  return out
}

export type MetricRowView = {
  name: string
  /** Bar parts as fractions of the bar; the track shows the rest. */
  parts: Array<{ f: number; tone: 'green' | 'amber' | 'red' }>
  showBar: boolean
  /** Dim text in the bar's place: "not started", or the target of a value. */
  barNote: string
  ratio?: { a: number; b: number }
  valueText?: string
  valueTone: 'green' | 'amber' | 'fg'
  delta: Array<{ text: string; tone: 'green' | 'red' }>
  /** Glyphs that carry the state without colour: ✓ all good / on target, ~N partial, ✗N failed / off target (C1). */
  marks: string
  markTone: 'green' | 'amber' | 'red' | 'fg'
}
export type MetricSectionView = { more?: number; title: string; note: string; legend: Array<{ tone: 'green' | 'amber' | 'red' | 'track'; name: string }>; rows: MetricRowView[] }

const fmtNum = (n: number) => abbr(n)

/** One view model for every renderer: bars as fractions, counts, colours and deltas against the seen cursor. */
export const metricViews = (m: MwMetrics | undefined, base: Record<string, number[]> | undefined): MetricSectionView[] =>
  (m?.sections ?? []).map(sec => ({
    title: sec.title,
    note: sec.note ?? (sec.kind === 'split' ? `count is ${sec.legend.good}/total` : ''),
    legend: sec.kind === 'split' ? [{ tone: 'green', name: sec.legend.good }, { tone: 'amber', name: sec.legend.warn }, { tone: 'red', name: sec.legend.bad }, { tone: 'track', name: sec.legend.none }] : [],
    rows: sec.rows.map(r => {
      const was = base?.[rowKey(sec, r)]
      const pr = primary(sec, r)
      const p = pr[0] ?? 0
      const b2 = pr[1] ?? 0
      const dp = was !== undefined ? p - (was[0] ?? 0) : 0
      const db = was !== undefined ? b2 - (was[1] ?? 0) : 0
      const arrow = (n: number) => (n === 0 ? '' : `${n > 0 ? '▲' : '▼'}${fmtNum(Math.abs(n))}`)
      const delta: MetricRowView['delta'] = []
      if (dp !== 0) delta.push({ text: arrow(dp), tone: sec.kind === 'value' && r.better === 'lower' ? (dp > 0 ? 'red' : 'green') : dp > 0 ? 'green' : 'red' })
      if (sec.kind === 'split' && db !== 0) delta.push({ text: arrow(db), tone: db > 0 ? 'red' : 'green' })
      if (sec.kind === 'split') {
        const tot = (r.good ?? 0) + (r.warn ?? 0) + (r.bad ?? 0) + (r.none ?? 0)
        const t = Math.max(1, tot)
        const marks = [tot > 0 && (r.good ?? 0) === tot ? '\u2713' : '', (r.warn ?? 0) > 0 ? `~${abbr(r.warn ?? 0)}` : '', (r.bad ?? 0) > 0 ? `\u2717${abbr(r.bad ?? 0)}` : ''].filter(Boolean).join(' ')
        return { name: r.name, parts: [{ f: (r.good ?? 0) / t, tone: 'green' }, { f: (r.warn ?? 0) / t, tone: 'amber' }, { f: (r.bad ?? 0) / t, tone: 'red' }], showBar: true, barNote: '', ratio: { a: r.good ?? 0, b: tot }, valueTone: 'fg', delta, marks, markTone: (r.bad ?? 0) > 0 ? 'red' : (r.warn ?? 0) > 0 ? 'amber' : marks === '' ? 'fg' : 'green' }
      }
      if (sec.kind === 'progress') {
        const total = r.total ?? 0
        const done = r.done ?? 0
        return { name: r.name, parts: total > 0 ? [{ f: done / total, tone: done >= total ? 'green' : 'green' }] : [], showBar: total > 0, barNote: total > 0 ? '' : 'not started', ratio: { a: done, b: total }, valueTone: 'fg', delta, marks: total > 0 && done >= total ? '\u2713' : '', markTone: 'green' }
      }
      const v = r.value ?? 0
      const hit = r.target === undefined ? undefined : r.better === 'lower' ? v <= r.target : v >= r.target
      return {
        name: r.name,
        parts: [],
        showBar: false,
        barNote: r.target === undefined ? '' : `target ${fmtNum(r.target)}`,
        valueText: `${fmtNum(v)}${r.unit ? ` ${r.unit}` : ''}`,
        valueTone: hit === undefined ? 'fg' : hit ? 'green' : 'amber',
        delta,
        marks: hit === undefined ? '' : hit ? '\u2713' : '\u2717',
        markTone: hit === undefined ? 'fg' : hit ? 'green' : 'amber',
      }
    }),
  }))

/** Alert lines for rows that got worse since the last snapshot: good down or bad up (split), done down (progress), away from the target (value). */
export const metricDrops = (prev: MwMetrics | undefined, next: MwMetrics | undefined): Array<{ line: string; who: string }> => {
  const out: Array<{ line: string; who: string }> = []
  for (const sec of next?.sections ?? []) {
    const old = prev?.sections.find(x => x.title === sec.title && x.kind === sec.kind)
    if (old === undefined) continue
    for (const r of sec.rows) {
      const o = old.rows.find(x => x.name === r.name)
      if (o === undefined) continue
      const who = rowKey(sec, r)
      const head = `${sec.title} ${r.name}`
      if (sec.kind === 'split') {
        if ((r.good ?? 0) < (o.good ?? 0)) out.push({ line: `${head}: ${sec.legend.good} ${o.good ?? 0}→${r.good ?? 0}`, who })
        if ((r.bad ?? 0) > (o.bad ?? 0)) out.push({ line: `${head}: ${sec.legend.bad} ${o.bad ?? 0}→${r.bad ?? 0}`, who })
      } else if (sec.kind === 'progress') {
        if ((r.done ?? 0) < (o.done ?? 0)) out.push({ line: `${head}: done ${o.done ?? 0}→${r.done ?? 0}`, who })
      } else if (r.target !== undefined && r.value !== undefined && o.value !== undefined) {
        const worse = r.better === 'lower' ? r.value > o.value : r.value < o.value
        if (worse) out.push({ line: `${head}: ${fmtNum(o.value)}→${fmtNum(r.value)}${r.unit ? ` ${r.unit}` : ''}`, who })
      }
    }
  }
  return out
}


const rowNums = (sec: MwMetricSection, r: MwMetricRow): number[] => (sec.kind === 'split' ? [r.good ?? 0, r.warn ?? 0, r.bad ?? 0, r.none ?? 0] : sec.kind === 'progress' ? [r.done ?? 0, r.total ?? 0] : [r.value ?? 0])
/** Alert lines for rows that changed since the last snapshot without getting worse (an improvement, a moved total, a new row): the rest of a metrics.json change. */
export const metricChanges = (prev: MwMetrics | undefined, next: MwMetrics | undefined): Array<{ line: string; who: string }> => {
  const out: Array<{ line: string; who: string }> = []
  const dropped = new Set(metricDrops(prev, next).map(d => d.who))
  for (const sec of next?.sections ?? []) {
    const old = prev?.sections.find(x => x.title === sec.title && x.kind === sec.kind)
    if (old === undefined) continue
    for (const r of sec.rows) {
      const who = rowKey(sec, r)
      if (dropped.has(who)) continue
      const o = old.rows.find(x => x.name === r.name)
      const head = `${sec.title} ${r.name}`
      if (o === undefined) {
        out.push({ line: `${head}: new`, who })
        continue
      }
      const a = rowNums(sec, o)
      const b = rowNums(sec, r)
      if (a.every((n, i) => n === b[i])) continue
      const [p0, b0] = sec.kind === 'split' ? [o.good ?? 0, r.good ?? 0] : sec.kind === 'progress' ? [`${o.done ?? 0}/${o.total ?? 0}`, `${r.done ?? 0}/${r.total ?? 0}`] : [fmtNum(o.value ?? 0), `${fmtNum(r.value ?? 0)}${r.unit ? ` ${r.unit}` : ''}`]
      out.push({ line: `${head}: ${sec.kind === 'split' ? `${sec.legend.good} ` : ''}${p0}\u2192${b0}`, who })
    }
  }
  return out
}


const parseRegistry = (text: string): MwJobsRegistry => {
  const v = parseJson(text) as { jobs?: unknown } | undefined
  if (v === undefined || !Array.isArray(v.jobs)) return { jobs: [], error: 'registry is not valid JSON' }
  const jobs = v.jobs
    .filter((j: Record<string, unknown>) => typeof j?.id === 'string' && typeof j?.board === 'string')
    .map((j: Record<string, unknown>) => ({
      id: str(j.id),
      title: str(j.title ?? j.id),
      board: str(j.board),
      tenants: Array.isArray(j.tenants) ? j.tenants.map(str) : [],
      news: str(j.news),
      metrics: str(j.metrics),
      description: str(j.description),
      coordinator: str(j.coordinator),
      desk: str(j.desk),
      alerts: (j.alerts ?? {}) as MwJobsJob['alerts'],
    }))
  return { jobs, error: null }
}


/** The one bar length every style and layout shares: half the body width, 10-40 cells, 20 when unknown. */
export const barLength = (width: number | undefined) =>
  width === undefined ? 20 : Math.max(10, Math.min(40, Math.floor(width / 2)))

// ---- the "normal" style: one SVG for the whole per-job view (desktop, vscode, mobile) ----

export type SvgModel = {
  head: { text: string; tone: 'error' | 'warning' | 'success' }
  summary: { text: string; color?: string }[]
  chip: string
  needs: { title: string; meta: string; age: string; isNew: boolean }[]
  review: { id: string; title: string; meta: string; age: string; isNew: boolean }[]
  chat?: { id: string; title: string }[]
  /** Blocked items (a snapshot from a news model only). */
  blocked?: { id: string; title: string; meta: string; age: string; isNew: boolean }[]
  doing: { id: string; title: string; age: string; isStalled: boolean; isNew: boolean }[]
  miles: MileRow[]
  folded: string
  metrics: MetricSectionView[]
  metricsNotice?: string
  events: { t: string; text: string }[]
  footer: string
  density: 'compact' | 'default' | 'roomy'
}

const xml = (t: string) => t.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
const MONO = '"SF Mono",Menlo,Monaco,"JetBrains Mono","Cascadia Mono",Consolas,"DejaVu Sans Mono",ui-monospace,monospace'

const SANS = "-apple-system,'SF Pro Text',system-ui,sans-serif"
const TONE: Record<string, string> = { error: 'red', warning: 'amber', success: 'green' }

/** -/#/+ scale every size and the row height by 0.85, 1 and 1.15. */
export const svgScaleFor = (step: number) => (step <= -2 ? 0.7 : step < 0 ? 0.85 : step >= 2 ? 1.3 : step > 0 ? 1.15 : 1)

/** The palette follows the HOST theme (`/config` theme), not the OS: light or dark when the host says so, the OS preference only on auto (W1). */
export type ThemeMode = 'auto' | 'light' | 'dark'
let themeMode: ThemeMode = 'auto'
const themeCss = (light: string, dark: string): string =>
  themeMode === 'light' ? `:root{${light}}` : themeMode === 'dark' ? `:root{${dark}}` : `:root{${light}}\n@media (prefers-color-scheme:dark){:root{${dark}}}`
const SVG_LIGHT = '--fg:#18181B;--dim:#5B5B66;--track:#E4E4E7;--red:#B91C1C;--amber:#A16207;--green:#15803D;--acc:#2563EB;--chip:#F4F4F5'
const SVG_DARK = '--fg:#F4F4F5;--dim:#A1A1AA;--track:#3F3F46;--red:#F87171;--amber:#FBBF24;--green:#4ADE80;--acc:#60A5FA;--chip:#27272A'
const MONO_LIGHT = '--fg:#18181B;--dim:#5B5B66;--red:#B91C1C;--amber:#A16207;--green:#15803D;--trk:#D4D4D8;--acc:#2563EB;--chip:#F4F4F5;--track:#E4E4E7'
const MONO_DARK = '--fg:#F4F4F5;--dim:#A1A1AA;--red:#F87171;--amber:#FBBF24;--green:#4ADE80;--trk:#3F3F46;--acc:#60A5FA;--chip:#27272A;--track:#3F3F46'
const SVG_TAIL = `text{font-family:${SANS};font-variant-numeric:tabular-nums}.b{font-weight:700}.s{font-weight:600;letter-spacing:.08em}
.fg{fill:var(--fg)}.dim{fill:var(--dim)}.red{fill:var(--red)}.amber{fill:var(--amber)}.green{fill:var(--green)}.acc{fill:var(--acc)}.track{fill:var(--track)}.chip{fill:var(--chip)}.rule{stroke:var(--track);stroke-width:1}.u{text-decoration:underline}.i{font-style:italic}.inkfill{fill:var(--fg)}.gfill{fill:var(--green)}.rfill{fill:var(--red)}.redfill{fill:var(--red)}.amberfill{fill:var(--amber)}.greenfill{fill:var(--green)}.accfill{fill:var(--acc)}`
let SVG_CSS = ''


/**
 * The milestone rows both panes draw in the normal style: `ID title | bar | done/total state`, one line each, the same
 * columns and widths wherever they appear (jobs and news). Returns the Svg markup (no wrapper) and its height.
 */
export const mileContent = (rows: MileRow[], tail: string, W: number, k = 1, pad = 18 * k): { markup: string; height: number } => {
  const f = { b: 13 * k, n: 12 * k }
  const rowH = 22 * k
  const gap = 12 * k
  const digit = 7 * k
  const nameW = 150 * k
  const num = (n: number) => Math.round(n * 100) / 100
  const countW = Math.max(1, ...rows.map(r => mileCount(r).length)) * digit
  const stateW = Math.max(1, ...rows.map(r => r.state.length)) * digit * 0.9
  const rightW = countW + gap / 2 + stateW
  const barX = pad + nameW + gap
  const barW = Math.max(20, W - pad - rightW - gap - barX)
  const out: string[] = []
  let uid = 0
  rows.forEach((r, i) => {
    const top = i * rowH
    const ty = (size: number) => num(top + rowH / 2 + size * 0.35)
    const name = `${mileName(r)}${r.ticket ? ` [${r.ticket}]` : ''}`
    out.push(`<svg x="${num(pad)}" y="${num(top)}" width="${num(nameW)}" height="${num(rowH)}" overflow="hidden"><text x="0" y="${num(rowH / 2 + f.b * 0.35)}" font-size="${num(f.b)}" xml:space="preserve"><tspan class="${r.total > 0 ? 'fg' : 'dim'}">${xml(name)}</tspan></text></svg>`)
    if (r.total > 0) {
      const by = top + (rowH - 6 * k) / 2
      const id = `m${uid++}`
      const fill = Math.max(0, Math.min(barW, (r.pct / 100) * barW))
      out.push(`<clipPath id="${id}"><rect x="${num(barX)}" y="${num(by)}" width="${num(barW)}" height="${num(6 * k)}" rx="${num(3 * k)}"/></clipPath>`)
      out.push(`<rect x="${num(barX)}" y="${num(by)}" width="${num(barW)}" height="${num(6 * k)}" rx="${num(3 * k)}" class="track"/>`)
      if (fill > 0) out.push(`<rect x="${num(barX)}" y="${num(by)}" width="${num(fill)}" height="${num(6 * k)}" class="${r.state === 'closed' ? 'gfill' : 'inkfill'}" clip-path="url(#${id})"/>`)
    }
    out.push(`<text x="${num(W - pad - stateW - 6 * k)}" y="${ty(f.b)}" font-size="${num(f.b)}" class="dim" text-anchor="end" xml:space="preserve">${xml(mileCount(r))}</text>`)
    out.push(`<text x="${num(W - pad)}" y="${ty(f.n)}" font-size="${num(f.n)}" class="${r.state === 'closed' ? 'green' : 'dim'}" text-anchor="end" xml:space="preserve">${xml(r.state)}</text>`)
  })
  let height = rows.length * rowH
  if (tail !== '') {
    out.push(`<text x="${num(pad)}" y="${num(height + f.n + 2 * k)}" font-size="${num(f.n)}" class="dim" xml:space="preserve">${xml(tail)}</text>`)
    height += rowH - 2 * k
  }
  return { markup: out.join(''), height }
}

/** The same rows as a standalone Svg (the news pane draws them between its text sections). */
export const buildMileSvg = (rows: MileRow[], tail: string, W = 420, k = 1): { source: string; alt: string; width: number } => {
  const c = mileContent(rows, tail, W, k, 18 * k)
  const H = Math.ceil(c.height + 4 * k)
  return { source: `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}"><style>${SVG_CSS}</style>${c.markup}</svg>`, alt: rows.map(r => `${mileName(r)} ${mileCount(r)} ${r.state}`).join('; '), width: W }
}

/**
 * The normal style off the terminal: one sans font in four sizes (18 headline, 13 body, 12 notes, 11 labels),
 * a three-column grid (name | bar | count) for milestones and metrics, 6px rounded bars centred in 22px rows.
 */
export const buildSvg = (m: SvgModel, W = 420, k = 1): { source: string; alt: string; width: number } => {
  const f = { h: 18 * k, b: 13 * k, n: 12 * k, l: 11 * k }
  const pad = 18 * k
  const rowH = 22 * k
  const gap = 12 * k
  const nameW = 150 * k
  const digit = 7 * k
  const FIG = ' '
  const mrows = m.metrics.flatMap(sec => sec.rows)
  const dw = Math.max(1, ...mrows.map(r => (r.ratio ? abbr(r.ratio.a).length : 0)))
  const tw = Math.max(1, ...mrows.map(r => (r.ratio ? abbr(r.ratio.b).length : 0)))
  const countChars = dw + 1 + tw
  const deltaChars = Math.max(0, ...mrows.map(r => r.delta.reduce((n, d) => n + d.text.length + 1, 0) + (r.marks ? r.marks.length + 1 : 0)))
  const countW = countChars * digit
  const deltaW = deltaChars * digit
  const rightW = Math.max(56 * k, countW + deltaW)
  const right = W - pad
  const countEnd = right - deltaW
  const barX = pad + nameW + gap
  const barW = Math.max(20, W - pad - rightW - gap - barX)
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
    const by = top + (rowH - 6 * k) / 2
    const id = `c${uid++}`
    out.push(`<clipPath id="${id}"><rect x="${num(barX)}" y="${num(by)}" width="${num(barW)}" height="${num(6 * k)}" rx="${num(3 * k)}"/></clipPath>`)
    out.push(`<rect x="${num(barX)}" y="${num(by)}" width="${num(barW)}" height="${num(6 * k)}" rx="${num(3 * k)}" class="track"/>`)
    let x = barX
    for (const sg of segs) {
      const w = Math.max(0, Math.min(barW - (x - barX), sg.w * barW))
      if (w > 0) out.push(`<rect x="${num(x)}" y="${num(by)}" width="${num(w)}" height="${num(6 * k)}" class="${sg.cls}" clip-path="url(#${id})"/>`)
      x += w
    }
  }
  const countText = (top: number, done: number, total: number) =>
    t(countEnd, base(top, f.b), f.b, 'dim', xml(`${abbr(done).padStart(dw, FIG)}/${abbr(total).padEnd(tw, FIG)}`), 'text-anchor="end"')
  const wideRow = (nameBody: string, rightParts: string, top: number) => {
    cutText(pad, top, right - pad - 56 * k - gap, f.b, nameBody)
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
  const chip = m.chip !== '' ? sp('dim', ' · ') + sp('acc', `● ${m.chip}`) : ''
  t(pad, y + f.b, f.b, 'dim', summary + chip)
  y += f.b * 1.5
  gapBetween()

  if (m.needs.length > 0) {
    label('Needs you', 'red')
    for (const n of m.needs) {
      wideRow(sp('red', '! ') + sp('fg', n.title), n.age ? sp('dim', n.age) + (n.isNew ? sp('acc', ' ●') : '') : '', y)
      y += rowH
      note(n.meta)
    }
    gapBetween()
  }
  if (m.review.length > 0) {
    label('Review', 'amber')
    for (const r of m.review) {
      wideRow(sp('dim', `${r.id} `) + sp('fg', r.title), (r.age ? sp('dim', r.age) : '') + (r.isNew ? sp('acc', ' ●') : ''), y)
      y += rowH
      note(r.meta)
    }
    gapBetween()
  }
  if ((m.blocked ?? []).length > 0) {
    label('Blocked', 'red')
    for (const r of m.blocked ?? []) {
      wideRow(sp('dim', `${r.id} `) + sp('fg', r.title), (r.age ? sp('dim', r.age) : '') + (r.isNew ? sp('acc', ' ●') : ''), y)
      y += rowH
      note(r.meta)
    }
    gapBetween()
  }
  if ((m.chat ?? []).length > 0) {
    label('In chat', 'dim')
    for (const c of m.chat ?? []) {
      wideRow(sp('dim', `${c.id} ${c.title}`), sp('dim', 'in chat'), y)
      y += rowH
    }
    gapBetween()
  }
  if (m.doing.length > 0) {
    label('In progress', 'dim')
    for (const d of m.doing) {
      wideRow(sp('dim', `${d.id} `) + sp('fg', d.title), sp(d.isStalled ? 'amber' : 'dim', d.age) + (d.isNew ? sp('acc', ' ●') : ''), y)
      y += rowH
    }
    gapBetween()
  }
  if (m.miles.length > 0 || m.folded !== '') {
    label('Milestones', 'dim')
    const mc = mileContent(m.miles, m.folded, W, k, pad)
    out.push(`<g transform="translate(0 ${num(y)})">${mc.markup}</g>`)
    y += mc.height
    gapBetween()
  }
  if (m.metricsNotice) {
    t(pad, y + f.n, f.n, 'dim', xml(m.metricsNotice))
    y += rowH
    gapBetween()
  }
  for (const sec of m.metrics) {
    label(sec.title, 'dim')
    const TONECLS = { green: 'gfill', amber: 'amberfill', red: 'rfill' } as const
    for (const r of sec.rows) {
      if (!r.ratio && !r.showBar) {
        // a value row: its own two lines, so nothing overprints
        cutText(pad, y, W - 2 * pad, f.b, sp('fg', r.name))
        y += rowH - 4 * k
        const val = [r.valueText ?? '', r.marks, r.barNote ? `(${r.barNote})` : '', ...r.delta.map(d => d.text)].filter(Boolean).join('  ')
        cutText(pad + gap, y, W - 2 * pad - gap, f.n, sp(r.valueTone, val))
        y += rowH
        continue
      }
      cutText(pad, y, nameW, f.b, sp('fg', r.name))
      if (r.showBar) bar(y, r.parts.map(pt => ({ w: pt.f, cls: TONECLS[pt.tone] })))
      else if (r.barNote) t(barX, base(y, f.b), f.n, 'dim', xml(r.barNote))
      if (r.ratio) countText(y, r.ratio.a, r.ratio.b)
      let dx = countEnd + digit
      for (const d of r.delta) {
        t(dx, base(y, f.b), f.b, d.tone, xml(d.text))
        dx += (d.text.length + 1) * digit
      }
      if (r.marks) t(dx, base(y, f.b), f.b, r.markTone === 'fg' ? 'dim' : r.markTone, xml(r.marks))
      y += rowH
    }
    if ((sec.more ?? 0) > 0) {
      t(pad + gap, y + f.n, f.n, 'dim', xml(`+${sec.more} rows`))
      y += rowH - 4 * k
    }
    // legend and note: one flow that wraps inside the width
    const cwN = f.n * 0.56
    const flowTokens: Array<{ cls: string; text: string }> = [...sec.legend.map(l => ({ cls: l.tone, text: `■ ${l.name.slice(0, 40)}` })), ...(sec.note ? [{ cls: 'dim', text: '—' }, ...sec.note.split(/\s+/).filter(Boolean).map(w => ({ cls: 'dim', text: w.slice(0, 60) }))] : [])]
    if (flowTokens.length > 0) {
      let fx = pad
      let line = ''
      const flush = () => {
        if (line !== '') {
          t(pad, y + f.n + 2 * k, f.n, 'dim', line)
          y += rowH - 6 * k
        }
        line = ''
        fx = pad
      }
      for (const tk of flowTokens) {
        const wd = (tk.text.length + 1) * cwN
        if (fx + wd > W - pad && line !== '') flush()
        line += sp(tk.cls, tk.text) + sp('dim', ' ')
        fx += wd
      }
      flush()
      y += 6 * k
    }
    gapBetween()
  }
  if (m.events.length > 0) {
    label('Last events', 'dim')
    for (const e of m.events) {
      cutText(pad, y, W - 2 * pad, f.n, sp('dim', `${e.t}  ${e.text}`))
      y += rowH - 4 * k
    }
    gapBetween()
  }
  t(pad, y + f.n, f.n, 'dim', xml(m.footer))
  y += f.n + pad

  const H = Math.ceil(y)
  const alt = `${m.head.text}. ${m.summary.map(x => x.text).join(', ')}`
  return {
    source: `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}"><style>${SVG_CSS}</style>${out.join('')}</svg>`,
    alt,
    width: W,
  }
}

const MONO_TAIL = `.acc{fill:var(--acc)}.chip{fill:var(--chip)}.rule{stroke:var(--track);stroke-width:1}.u{text-decoration:underline}.fg{fill:var(--fg)}.dim{fill:var(--dim)}.red{fill:var(--red)}.amber{fill:var(--amber)}.green{fill:var(--green)}.trk{fill:var(--trk)}.w{font-weight:600}`

let MONO_CSS = ''
/** Recomputes the two stylesheets for the host theme; called when the theme is read (session start, every refresh). */
export const setThemeMode = (mode: ThemeMode) => {
  themeMode = mode
  SVG_CSS = `${themeCss(SVG_LIGHT, SVG_DARK)}\n${SVG_TAIL}`
  MONO_CSS = `${themeCss(MONO_LIGHT, MONO_DARK)}\n${MONO_TAIL}`
}
setThemeMode('auto')

export type MonoCell = { text: string; tone?: string }
export type MonoRow = {
  name: string
  nameTone?: string
  slot?:
    | { kind: 'text'; text: string; tone?: string }
    | { kind: 'bar'; pct: number; isNew: boolean }
    | { kind: 'parts'; parts: Array<{ f: number; tone: string }> }
  right?: MonoCell[]
  /** A click on the name opens the detail panel. */
  open?: () => void
}
export type MonoBlock =
  | { kind: 'head'; text: string; tone: string }
  | { kind: 'line'; parts: MonoCell[] }
  | { kind: 'label'; text: string; tone?: string }
  | { kind: 'rows'; rows: MonoRow[] }
  | { kind: 'notice'; tone: string; parts: MonoCell[] }
  | { kind: 'gap' }

/** The milestone rows both panes draw in the blocks style: `ID title | bar | done/total state`, the same mono grid. */
export const mileMonoRows = (rows: MileRow[], open?: (r: MileRow) => void): MonoRow[] => {
  const dw = Math.max(1, ...rows.map(r => abbr(r.done).length))
  const tw = Math.max(1, ...rows.map(r => abbr(r.total).length))
  return rows.map(r => ({
    name: `${mileName(r)}${r.ticket ? ` [${r.ticket}]` : ''}`,
    nameTone: r.total === 0 ? 'dim' : 'fg',
    slot: r.total === 0 ? { kind: 'text' as const, text: 'not started', tone: 'dim' } : r.state === 'closed' ? { kind: 'text' as const, text: '\u2713 closed', tone: 'green' } : { kind: 'bar' as const, pct: r.pct, isNew: false },
    right: [{ text: ` ${abbr(r.done).padStart(dw)}/${abbr(r.total).padEnd(tw)}`, tone: 'dim' }, { text: r.total === 0 || r.state === 'closed' ? '' : ` ${r.state}`, tone: 'dim' }],
    ...(open !== undefined ? { open: () => open(r) } : {}),
  }))
}

/** The font size a step gives the Svg styles: 11px below 0, 12px at 0, 14px above. */
export const svgSizeFor = (step: number) => (step <= -2 ? 10 : step < 0 ? 11 : step >= 2 ? 16 : step > 0 ? 14 : 12)

/** The standard terminal width the blocks style lays out on. */
export const COLS = 80

/** The most columns the blocks grid takes: 80 by default, 160 with the fill option (↔) on. Set by each pane's render before it builds. */
export const FILL_MAX = 160
let monoMax = COLS
export const setMonoMax = (n: number) => {
  monoMax = n
}

/** Columns that fit the pane at this char advance: at most 80 (160 with fill), at least 40 (below that the drawing is clipped, never shrunk). */
export const monoCols = (paneW: number | undefined, cw: number, pad = 8): number =>
  paneW === undefined ? COLS : Math.max(40, Math.min(monoMax, Math.floor((paneW - 2 * pad) / cw + 1e-6)))

/** The terminal's columns for blocks: its real width clamped 40..80, or 40..160 with fill. */
export const termCols = (given: number | undefined, fill: boolean): number => (given === undefined || !(given > 0) ? COLS : Math.max(40, Math.min(fill ? FILL_MAX : COLS, Math.floor(given))))

const cut = (t: string, n: number) => cutCells(t, n)

/**
 * The blocks style off the terminal: the whole body as monospace Svg text, one font and one size.
 * One grid: name 22ch, bar 20ch, count after it; every row 1.6 x the size, nothing wraps, long text is cut
 * without an ellipsis.
 */
export const buildMonoSvg = (blocks: MonoBlock[], size: number, fit?: number): { source: string; alt: string; width: number } => {
  const cw = size * 0.6
  const rowH = Math.round(size * 1.6)
  const pad = 8
  const total = monoCols(fit, cw)
  const NAME = total >= 70 ? 22 : Math.max(12, Math.floor(total * 0.3))
  const rightChars = Math.min(
    total - NAME - 8,
    Math.max(6, ...blocks.flatMap(b => (b.kind === 'rows' ? b.rows.map(r => (r.right ?? []).map(c => c.text).join('').length) : [0]))),
  )
  const BAR = Math.max(6, Math.min(20, total - NAME - rightChars))
  const W = Math.round((pad * 2 + total * cw) * 100) / 100
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
      left -= cellWidth(t)
      o.push(`<tspan class="${p.tone ?? 'fg'}">${xml(t)}</tspan>`)
    }
    return o.join('')
  }
  const line = (parts: MonoCell[], cls = 'm') => {
    out.push(`<text x="${pad}" y="${base()}" class="${cls}" xml:space="preserve">${tsp(parts, total)}</text>`)
    alt.push(parts.map(p => p.text).join(''))
    y += rowH
  }
  for (const b of blocks) {
    if (b.kind === 'gap') y += Math.round(rowH * 0.6)
    else if (b.kind === 'head') line([{ text: b.text, tone: b.tone }], 'm w')
    else if (b.kind === 'label') line([{ text: b.text.toUpperCase(), tone: b.tone ?? 'dim' }], 'm w')
    else if (b.kind === 'notice') line([{ text: '▌', tone: b.tone }, { text: ' ' }, ...b.parts])
    else if (b.kind === 'line') line(b.parts)
    else {
      for (const r of b.rows) {
        const by = base()
        out.push(`<text x="${pad}" y="${by}" class="m" xml:space="preserve"><tspan class="${r.nameTone ?? 'fg'}">${xml(cut(r.name, NAME - 1))}</tspan></text>`)
        const sx = pad + NAME * cw
        const slot = r.slot
        if (slot?.kind === 'text') {
          out.push(`<text x="${sx}" y="${by}" class="m" xml:space="preserve"><tspan class="${slot.tone ?? 'dim'}">${xml(cut(slot.text, BAR))}</tspan></text>`)
        } else if (slot?.kind === 'bar') {
          const f = Math.round((Math.max(0, Math.min(100, slot.pct)) / 100) * BAR)
          out.push(`<text x="${sx}" y="${by}" class="m" textLength="${BAR * cw}" lengthAdjust="spacingAndGlyphs" xml:space="preserve"><tspan class="${slot.isNew ? 'green' : 'fg'}">${'█'.repeat(f)}</tspan><tspan class="trk">${'░'.repeat(BAR - f)}</tspan></text>`)
        } else if (slot?.kind === 'parts') {
          // cumulative rounding so the parts and the track always add up to the bar
          let used = 0
          let acc = 0
          const spans = slot.parts.map(pt => {
            acc += pt.f
            const upto = Math.min(BAR, Math.round(acc * BAR))
            const n = Math.max(0, upto - used)
            used += n
            return `<tspan class="${pt.tone}">${'█'.repeat(n)}</tspan>`
          })
          out.push(`<text x="${sx}" y="${by}" class="m" textLength="${BAR * cw}" lengthAdjust="spacingAndGlyphs" xml:space="preserve">${spans.join('')}<tspan class="trk">${'░'.repeat(Math.max(0, BAR - used))}</tspan></text>`)
        }
        if (r.right && r.right.length > 0) {
          // the count is right-aligned in its column; text longer than the column is cut, never overprinting (S9)
          const rw = Math.min(rightChars, r.right.reduce((n, c) => n + cellWidth(c.text), 0))
          out.push(`<text x="${pad + (NAME + BAR) * cw + Math.max(0, rightChars - rw) * cw}" y="${by}" class="m" xml:space="preserve">${tsp(r.right, rightChars)}</text>`)
        }
        alt.push(`${r.name} ${(r.right ?? []).map(c => c.text).join('')}`.trim())
        y += rowH
      }
    }
  }
  const H = y + 6
  const dispW = W
  const dispH = H
  const css = `${MONO_CSS}text{font-family:${MONO};white-space:pre}.m{font-family:${MONO};font-size:${size}px}`
  return {
    source: `<svg xmlns="http://www.w3.org/2000/svg" width="${dispW}" height="${dispH}" viewBox="0 0 ${W} ${H}"><style>${css}</style>${out.join('')}</svg>`,
    alt: alt.join('; '),
    width: dispW,
  }
}

// ---- docs/news/status.md as Svg: a small markdown renderer ----

export type MdRun = { text: string; bold?: boolean; italic?: boolean; code?: boolean; tone?: string; id?: boolean; href?: string }
export type MdBlock =
  | { kind: 'h1' | 'h2' | 'h3' | 'p'; runs: MdRun[] }
  | { kind: 'li'; marker: string; depth: number; runs: MdRun[] }
  | { kind: 'table'; rows: string[][] }
  | { kind: 'codeblock'; lines: string[] }
  | { kind: 'notice'; tone: string; text: string }
  | { kind: 'gap' }

/** Decodes the HTML entities the news writer emits (&lt; &gt; &quot; &#39; &amp;). */
const unEntity = (t: string) => t.replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&amp;/g, '&')
const ESC_OPEN = '\uE000'
const ESC_CLOSE = '\uE001'
/** A backslash escape (\| \` \* ...) hidden from the parser, restored as the plain character afterwards. */
const hideEscapes = (t: string) => t.replace(/\\([\\`*_{}[\]()#+\-.!|<>~])/g, (_m, c: string) => `${ESC_OPEN}${c.charCodeAt(0)}${ESC_CLOSE}`)
const showEscapes = (t: string) => t.replace(new RegExp(`${ESC_OPEN}(\\d+)${ESC_CLOSE}`, 'g'), (_m, n: string) => String.fromCharCode(Number(n)))
const unescapeMd = (t: string) => unEntity(showEscapes(t))

const mdFormat = (plain: string): MdRun[] => {
  const out: MdRun[] = []
  const EM = /(\*\*[^*]+\*\*|`[^`]+`|(?<![\w*])\*(?=[^\s*])[^*]+?(?<=[^\s*])\*(?![*\w])|(?<!\w)_(?=\S)[^_]+?(?<=\S)_(?!\w))/
  for (const part of plain.split(EM)) {
    if (part === '') continue
    if (part.startsWith('**') && part.endsWith('**') && part.length > 4) out.push({ text: unescapeMd(part.slice(2, -2)), bold: true })
    else if (/^(\*|_)[^*_]/.test(part) && part.length > 2 && part.endsWith(part[0]!) && !part.startsWith('**')) out.push({ text: unescapeMd(part.slice(1, -1)), italic: true })
    else if (part.startsWith('`') && part.endsWith('`') && part.length > 2) out.push({ text: unescapeMd(part.slice(1, -1)), code: true })
    else out.push({ text: unescapeMd(part) })
  }
  return out
}

const mdInline = (src: string): MdRun[] => {
  const out: MdRun[] = []
  for (const part of hideEscapes(src).split(/(\[[^\]]+\]\([^)]*\)|https?:\/\/[^\s)<>]+)/)) {
    if (part === '') continue
    const ml = /^\[([^\]]+)\]\(([^)]*)\)$/.exec(part)
    if (ml) {
      const href = ml[2]!.trim()
      if (/^https?:\/\//.test(href)) out.push({ text: unescapeMd(ml[1]!), href })
      else out.push(...mdFormat(ml[1]!))
    } else if (/^https?:\/\//.test(part)) {
      const m = /^(.*?)([.,;:!?]*)$/.exec(part)!
      out.push({ text: m[1]!, href: m[1]! })
      if (m[2]) out.push({ text: m[2] })
    } else out.push(...mdFormat(part))
  }
  return out
}

/** file:// address of an absolute path, percent-encoded per segment. */
export const fileHref = (path: string): string => `file://${path.split('/').map(encodeURIComponent).join('/')}`

/** A URL as host + path, cut near 48 characters. */
export const shortUrl = (url: string): string => {
  const t = url.replace(/^https?:\/\//, '').replace(/\/$/, '')
  return t.length > 48 ? `${t.slice(0, 47)}…` : t
}

/** Content hash of the news file, ignoring its "Updated" line. */
export const newsHash = (text: string): string => {
  const t = text.replace(/\r/g, '').split('\n').filter(l => !/^\W*updated\b/i.test(l)).join('\n')
  let h = 5381
  for (let i = 0; i < t.length; i++) h = ((h * 33) ^ t.charCodeAt(i)) >>> 0
  return h.toString(36)
}

const hasLink = (b: MdBlock): boolean => (b.kind === 'p' || b.kind === 'li') && b.runs.some(r => r.href !== undefined)

/** Splits the page into Svg chunks and the lines that hold a link (drawn as clickable rows). The "Page:" line for the job's own news URL is dropped. */
export const chunkMd = (blocks: MdBlock[], newsUrl: string): Array<{ svg: MdBlock[]; sec: string } | { link: MdBlock }> => {
  const parts: Array<{ svg: MdBlock[]; sec: string } | { link: MdBlock }> = []
  let cur: MdBlock[] = []
  let curSec = ''
  let sec = ''
  const flush = () => {
    if (cur.length > 0) parts.push({ svg: cur, sec: curSec })
    cur = []
  }
  for (const b of blocks) {
    if (b.kind === 'h2') sec = b.runs.map(r => r.text).join('')
    if (hasLink(b) && (b.kind === 'p' || b.kind === 'li')) {
      const text = b.runs.map(r => r.text).join('')
      const own = b.runs.some(r => r.href === newsUrl) && /^\s*page\s*:\s*/i.test(text) && b.runs.filter(r => r.href !== undefined).length === 1 && text.replace(/^\s*page\s*:\s*/i, '') === newsUrl
      if (own) continue
      flush()
      parts.push({ link: b })
      continue
    }
    if (cur.length === 0) curSec = sec
    cur.push(b)
  }
  flush()
  return parts
}

const ID_RE = /\b((?:TSK|WAV|CHR|SPK|BUG|FTR|STR|EPC|ANL|RUN)-\d+)\b|(@[A-Za-z0-9_-]+)/g
/** Item ids go dim and @names accent, in plain runs. */
const decorate = (runs: MdRun[]): MdRun[] =>
  runs.flatMap(r => {
    if (r.code || r.href) return [r]
    const out: MdRun[] = []
    let last = 0
    for (const m of r.text.matchAll(ID_RE)) {
      if (m.index! > last) out.push({ ...r, text: r.text.slice(last, m.index) })
      out.push(m[1] ? { ...r, text: m[0], tone: 'dim', id: true } : { ...r, text: m[0], tone: 'acc' })
      last = m.index! + m[0].length
    }
    if (last < r.text.length) out.push({ ...r, text: r.text.slice(last) })
    return out.length > 0 ? out : [r]
  })

/** Drops a `Page: <url>` (or `Page: [text](url)`) segment from a line. */
export const stripPage = (line: string): string => line.replace(/(^|\s)Page:\s*(?:\[[^\]]*\]\([^)]*\)|https?:\/\/\S+)/gi, '$1').replace(/\s+/g, ' ').trim()

/** Markdown text to blocks: # ## ### headings, - * and 1. lists, | tables |, **bold**, `code`, blank-line gaps. */
export const parseMd = (text: string): MdBlock[] => {
  const blocks: MdBlock[] = []
  const lines = text.replace(/\r/g, '').split('\n')
  let i = 0
  const push = (b: MdBlock) => {
    if (b.kind === 'gap' && (blocks.length === 0 || blocks[blocks.length - 1]!.kind === 'gap')) return
    blocks.push(b)
  }
  while (i < lines.length) {
    const line = lines[i]!
    if (line.trim() === '') {
      push({ kind: 'gap' })
      i += 1
      continue
    }
    if (line.trim().startsWith('```')) {
      const code: string[] = []
      i += 1
      while (i < lines.length && !lines[i]!.trim().startsWith('```')) {
        code.push(lines[i]!)
        i += 1
      }
      i += 1
      push({ kind: 'codeblock', lines: code })
      continue
    }
    const h = /^(#{1,3})\s+(.*)$/.exec(line)
    if (h) {
      push({ kind: (`h${h[1]!.length}`) as 'h1', runs: mdInline(h[2]!.trim()) })
      i += 1
      continue
    }
    if (line.trim().startsWith('|')) {
      const rows: string[][] = []
      while (i < lines.length && lines[i]!.trim().startsWith('|')) {
        // split on unescaped | only; the cell text is unescaped afterwards
        const cells = lines[i]!.trim().split(/(?<!\\)\|/).map(c => c.trim())
        if (cells[0] === '') cells.shift()
        if (cells[cells.length - 1] === '') cells.pop()
        if (!cells.every(c => /^:?-{2,}:?$/.test(c))) rows.push(cells.map(c => mdInline(c).map(r => r.text).join('')))
        i += 1
      }
      push({ kind: 'table', rows })
      continue
    }
    const li = /^(\s*)([-*])\s+(.*)$/.exec(line)
    const ol = /^(\s*)(\d+)\.\s+(.*)$/.exec(line)
    if (li || ol) {
      const m = (li ?? ol)!
      push({ kind: 'li', marker: li ? '•' : `${ol![2]}.`, depth: Math.floor(m[1]!.length / 2), runs: decorate(mdInline(m[3]!)) })
      i += 1
      continue
    }
    // a paragraph: every source line is its own line (soft breaks kept); a Page: link is dropped wherever it stands
    const para: string[] = [line.trim()]
    i += 1
    while (i < lines.length && lines[i]!.trim() !== '' && !/^(#{1,3}\s|\s*[-*]\s|\s*\d+\.\s|\s*\|)/.test(lines[i]!)) {
      para.push(lines[i]!.trim())
      i += 1
    }
    for (const src of para) {
      const t = stripPage(src)
      if (t !== '') push({ kind: 'p', runs: decorate(mdInline(t)) })
    }
  }
  return blocks
}

/** Word-wraps styled runs to `max` characters per line; a word longer than a line is cut. */
export const wrapRuns = (runs: MdRun[], max: number): MdRun[][] => {
  const lines: MdRun[][] = [[]]
  let len = 0
  const add = (text: string, r: MdRun) => {
    const cur = lines[lines.length - 1]!
    const last = cur[cur.length - 1]
    if (last && !!last.bold === !!r.bold && !!last.code === !!r.code && !!last.italic === !!r.italic && last.tone === r.tone && !!last.id === !!r.id && last.href === r.href) last.text += text
    else cur.push({ text, bold: r.bold, italic: r.italic, code: r.code, tone: r.tone, id: r.id, href: r.href })
    len += text.length
  }
  for (const r of runs) {
    for (const tok of r.text.split(/(\s+)/)) {
      if (tok === '') continue
      const isSpace = /^\s+$/.test(tok)
      if (isSpace) {
        if (len > 0) add(' ', r)
        continue
      }
      let word = tok
      while (word.length > max) {
        if (len > 0) {
          lines.push([])
          len = 0
        }
        add(word.slice(0, max), r)
        lines.push([])
        len = 0
        word = word.slice(max)
      }
      if (len > 0 && len + word.length > max) {
        const cur = lines[lines.length - 1]!
        const last = cur[cur.length - 1]
        if (last && last.text.endsWith(' ')) last.text = last.text.slice(0, -1)
        lines.push([])
        len = 0
      }
      add(word, r)
    }
  }
  return lines.filter(l => l.length > 0)
}

/** The Svg size a step gives the blocks style (mono 11/12/14) and the scale the normal style uses. */
const secTone = (name: string): { tone: string; prefix: string } =>
  /need/i.test(name) ? { tone: 'red', prefix: '! ' } : /block/i.test(name) ? { tone: 'amber', prefix: '' } : /headline|in progress/i.test(name) ? { tone: 'fg', prefix: '' } : { tone: 'dim', prefix: '' }

/** In the Headline paragraph, "2 need you" is red and "1 blocked" amber when the number is above 0. */
const tintHeadline = (runs: MdRun[]): MdRun[] =>
  runs.flatMap(r => {
    if (r.code || r.tone) return [r]
    return r.text
      .split(/(\d+\s+(?:needs? you|needs?|blocked))/i)
      .filter(x => x !== '')
      .map(x => {
        const m = /^(\d+)\s+(?:needs? you|needs?|blocked)$/i.exec(x)
        if (m && Number(m[1]) > 0) return { ...r, text: x, tone: /blocked/i.test(x) ? 'amber' : 'red' }
        return { ...r, text: x }
      })
  })

const cutRuns = (runs: MdRun[], max: number): MdRun[] => {
  let left = max
  const out: MdRun[] = []
  for (const r of runs) {
    if (left <= 0) break
    const text = cutCells(r.text, left)
    left -= cellWidth(text)
    out.push({ ...r, text })
  }
  return out
}

/** The news page as Svg: the jobs pane palette, headings by section, id/@name colours, tables with bars. */
export const buildMdSvg = (
  blocks: MdBlock[],
  style: 'normal' | 'blocks',
  baseWidth: number,
  step: number,
  opts: { sec?: string; top?: boolean; bottom?: boolean } = {},
): { source: string; alt: string; width: number; height: number; lines: string[] } => {
  const k = svgScaleFor(step)
  const mono = style === 'blocks'
  const size = mono ? svgSizeFor(step) : 13 * k
  const pad = mono ? 8 : 18 * k
  const cw = mono ? size * 0.6 : size * 0.52
  // blocks: a fixed 80-column grid; the pane width only scales the drawing (viewBox)
  const cols80 = mono ? monoCols(baseWidth, cw, pad) : 0
  const W = mono ? Math.round((2 * pad + cols80 * cw) * 100) / 100 : Math.round(baseWidth * k)
  const maxChars = mono ? cols80 : Math.max(12, Math.floor((W - 2 * pad) / cw))
  const pitch = mono ? Math.round(size * 1.6) : size * 1.55
  const out: string[] = []
  const plain: string[] = []
  const r2 = (n: number) => Math.round(n * 100) / 100
  let y = opts.top === false ? 0 : mono ? 4 : pad
  let uid = 0
  let sec = opts.sec ?? ''
  let afterH1 = false
  const sizes = { h1: 18 * k, body: size, note: mono ? size : 12 * k, label: mono ? size : 11 * k }
  const clsOf = (r: MdRun, base: string, forceBold: boolean) => {
    const parts = [r.href ? 'acc u' : (r.tone ?? base)]
    if (r.bold || forceBold) parts.push(mono ? 'w' : 'b')
    if (r.italic) parts.push(mono ? 'dim' : 'i')
    if ((r.code || r.id) && !mono) parts.push('mono')
    return parts.join(' ')
  }
  const tspans = (runs: MdRun[], base: string, sz: number, forceBold = false) =>
    runs
      .map(r => {
        const fs = (r.code || r.id) && !mono ? ` font-size="${r2(sz * 0.92)}"` : ''
        return `<tspan class="${clsOf(r, base, forceBold)}"${fs}>${xml(r.text)}</tspan>`
      })
      .join('')
  const textLine = (x: number, yy: number, sz: number, body: string, extra = '') =>
    out.push(`<text x="${r2(x)}" y="${r2(yy)}" font-size="${r2(sz)}" ${mono ? 'class="m"' : ''} xml:space="preserve" ${extra}>${body}</text>`)
  // a chip behind inline code
  const codeChips = (x0: number, yy: number, sz: number, runs: MdRun[]) => {
    let off = 0
    for (const r of runs) {
      const per = r.code || r.id ? (mono ? cw : sz * 0.92 * 0.6) : mono ? cw : sz * 0.52
      if (r.code) out.push(`<rect x="${r2(x0 + off * 1 - 2)}" y="${r2(yy - sz * 0.85)}" width="${r2(r.text.length * per + 4)}" height="${r2(sz * 1.2)}" rx="3" class="chip"/>`)
      off += r.text.length * per
    }
  }
  // note: chip offsets above are in px already (off accumulates px)

  for (const b of blocks) {
    if (b.kind === 'gap') {
      y += pitch * 0.55
      afterH1 = false
      continue
    }
    if (b.kind === 'notice') {
      y += pitch
      if (mono) textLine(pad, y, size, `<tspan class="${b.tone}">▌</tspan><tspan class="fg"> ${xml(b.text.slice(0, maxChars - 2))}</tspan>`)
      else {
        out.push(`<rect x="${r2(pad)}" y="${r2(y - size)}" width="3" height="${r2(size * 1.3)}" rx="1.5" class="${b.tone}fill"/>`)
        textLine(pad + 10, y, size, `<tspan class="fg">${xml(b.text.slice(0, maxChars - 2))}</tspan>`)
      }
      plain.push(b.text)
      continue
    }
    if (b.kind === 'codeblock') {
      const sz = mono ? size : 12 * k
      const per = mono ? cw : sz * 0.6
      const room = Math.max(8, Math.floor((W - 2 * pad - 16) / per + 1e-6))
      if (mono) {
        const n = Math.max(8, maxChars)
        const row = (body: string) => {
          y += pitch
          textLine(pad, y, size, body)
        }
        row(`<tspan class="dim">┌${'─'.repeat(n - 2)}┐</tspan>`)
        for (const ln of b.lines) {
          const t = ln.slice(0, n - 4)
          row(`<tspan class="dim">│ </tspan><tspan class="fg">${xml(t)}</tspan><tspan class="dim">${' '.repeat(n - 4 - t.length)} │</tspan>`)
          plain.push(ln)
        }
        row(`<tspan class="dim">└${'─'.repeat(n - 2)}┘</tspan>`)
        continue
      }
      const h = b.lines.length * pitch + 12 * k
      out.push(`<rect x="${r2(pad)}" y="${r2(y + 4 * k)}" width="${r2(W - 2 * pad)}" height="${r2(h)}" rx="8" class="chip"/>`)
      y += 4 * k + 6 * k
      for (const ln of b.lines) {
        y += pitch
        out.push(`<text x="${r2(pad + 8)}" y="${r2(y - 3 * k)}" font-size="${r2(sz)}" class="${mono ? 'm' : 'mono'} fg" xml:space="preserve">${xml(ln.slice(0, room))}</text>`)
        plain.push(ln)
      }
      y += 8 * k
      continue
    }
    if (b.kind === 'table') {
      // a Tenant column that holds one value in every row is dropped
      let rows = b.rows
      const tcol = (rows[0] ?? []).findIndex(h => /^tenant$/i.test(h.trim()))
      if (tcol >= 0 && rows.length > 1 && new Set(rows.slice(1).map(r => r[tcol] ?? '')).size === 1) rows = rows.map(r => r.filter((_, i) => i !== tcol))
      const cols = Math.max(...rows.map(r => r.length))
      const sz = mono ? size : 12 * k
      const tcw = mono ? cw : sz * 0.52
      const body = rows.slice(1)
      const isRatio = (c: number) => body.length > 0 && body.every(r => /^\d+\/\d+$/.test(r[c] ?? ''))
      const ratio = Array.from({ length: cols }, (_, c) => isRatio(c))
      const dw = Math.max(1, ...body.flatMap(r => r.map((cell, c) => (ratio[c] ? cell.split('/')[0]!.length : 0))))
      const tw = Math.max(1, ...body.flatMap(r => r.map((cell, c) => (ratio[c] ? cell.split('/')[1]!.length : 0))))
      const stc = (rows[0] ?? []).findIndex(h => /^(state|status)$/i.test(h.trim()))
      const ftc = ratio.findIndex(r => !r)
      const natural = Array.from({ length: cols }, (_, c) => (ratio[c] ? 0 : Math.max(...rows.map(r => (r[c] ?? '').length)) * tcw))
      const gapPx = mono ? 3 * tcw : 2 * tcw
      const avail = W - 2 * pad
      const barGap = mono ? cw : 6 * k
      const barMax = mono ? 12 * cw : 60 * k
      const barMin = mono ? 8 * cw : 40
      const countPx = (dw + 1 + tw) * tcw
      let barPx = barMax
      let ftcW = ftc >= 0 ? natural[ftc]! : 0
      let wrapState = false
      const colWs = () =>
        Array.from({ length: cols }, (_, c) => (ratio[c] ? barPx + barGap + countPx : c === ftc ? ftcW : wrapState && c === stc ? 0 : natural[c]!))
      const totalOf = () => {
        const w = colWs()
        const n = w.filter((_, c) => !(wrapState && c === stc)).length
        return w.reduce((x, y2) => x + y2, 0) + gapPx * Math.max(0, n - 1)
      }
      if (totalOf() > avail) barPx = Math.max(barMin, barPx - (totalOf() - avail))
      if (totalOf() > avail && ftc >= 0) ftcW = Math.max(4 * tcw, ftcW - (totalOf() - avail))
      if (totalOf() > avail && stc >= 0 && stc !== ftc) wrapState = true
      const colW = colWs()
      const vis = Array.from({ length: cols }, (_, c) => c).filter(c => !(wrapState && c === stc))
      const colX: number[] = Array.from({ length: cols }, () => pad)
      let acc = pad
      for (const c of vis) {
        colX[c] = acc
        acc += colW[c]! + gapPx
      }
      const barChars = mono ? Math.max(1, Math.round(barPx / cw)) : 0
      const FIG = mono ? ' ' : ' '
      const stateTone = (t: string) => (/^(done|closed)/i.test(t.trim()) ? 'green' : /^ready/i.test(t.trim()) ? 'amber' : /^blocked/i.test(t.trim()) ? 'red' : 'dim')
      rows.forEach((row, ri) => {
        y += pitch
        if (ri > 0 && ri % 2 === 0) out.push(`<rect x="${r2(pad - 6)}" y="${r2(y - pitch * 0.72)}" width="${r2(W - 2 * pad + 12)}" height="${r2(pitch * (wrapState && ri > 0 ? 2 : 1))}" rx="4" class="chip"/>`)
        for (const c of vis) {
          const cell = row[c] ?? ''
          const x = colX[c]!
          const room = Math.max(1, Math.floor((colW[c]! + 0.01) / tcw))
          if (ri === 0) {
            const label = (mono ? cell : cell.toUpperCase()).slice(0, room)
            const sl = mono ? size : 11 * k
            if (ratio[c]) textLine(x + colW[c]!, y, sl, `<tspan class="${mono ? 'w ' : 's '}dim">${xml(label)}</tspan>`, 'text-anchor="end"')
            else textLine(x, y, sl, `<tspan class="${mono ? 'w ' : 's '}dim">${xml(label)}</tspan>`)
            continue
          }
          if (ratio[c]) {
            const [d, t] = cell.split('/').map(Number) as [number, number]
            const f = t > 0 ? Math.min(1, d / t) : 0
            if (mono) {
              const n = Math.round(f * barChars)
              out.push(`<text x="${r2(x)}" y="${r2(y)}" font-size="${r2(sz)}" class="m" textLength="${r2(barChars * cw)}" lengthAdjust="spacingAndGlyphs" xml:space="preserve"><tspan class="fg">${'█'.repeat(n)}</tspan><tspan class="trk">${'░'.repeat(barChars - n)}</tspan></text>`)
            } else {
              const id = `t${uid++}`
              const by = y - sz * 0.35 - 3 * k
              out.push(`<clipPath id="${id}"><rect x="${r2(x)}" y="${r2(by)}" width="${r2(barPx)}" height="${r2(6 * k)}" rx="${r2(3 * k)}"/></clipPath>`)
              out.push(`<rect x="${r2(x)}" y="${r2(by)}" width="${r2(barPx)}" height="${r2(6 * k)}" rx="${r2(3 * k)}" class="track"/>`)
              if (f > 0) out.push(`<rect x="${r2(x)}" y="${r2(by)}" width="${r2(barPx * f)}" height="${r2(6 * k)}" class="${f >= 1 ? 'gfill' : 'inkfill'}" clip-path="url(#${id})"/>`)
            }
            const count = `${String(d).padStart(dw, FIG)}/${String(t).padEnd(tw, FIG)}`
            textLine(x + barPx + barGap, y, sz, `<tspan class="dim">${xml(count)}</tspan>`)
            continue
          }
          if (c === stc) {
            textLine(x, y, sz, `<tspan class="${stateTone(cell)}">${xml(cell.slice(0, room))}</tspan>`)
            continue
          }
          textLine(x, y, sz, tspans(decorate([{ text: cell.slice(0, c === ftc ? room : cell.length) }]), 'fg', sz))
        }
        if (mono && vis.length > 1) {
          for (const c of vis.slice(1)) textLine(colX[c]! - 2 * tcw, y, size, '<tspan class="dim">│</tspan>')
        }
        if (ri === 0 && mono) {
          y += pitch
          const total = mono ? cols80 : Math.max(1, Math.floor((W - 2 * pad) / cw))
          const chars = Array.from({ length: total }, () => '─')
          for (const c of vis.slice(1)) {
            const at = Math.round((colX[c]! - 2 * tcw - pad) / tcw)
            if (at >= 0 && at < total) chars[at] = '┼'
          }
          textLine(pad, y, size, `<tspan class="dim">${chars.join('')}</tspan>`)
        }
        if (ri === 0 && !mono) out.push(`<line x1="${r2(pad)}" y1="${r2(y + pitch * 0.28)}" x2="${r2(W - pad)}" y2="${r2(y + pitch * 0.28)}" class="rule"/>`)
        if (wrapState && ri > 0) {
          const cell = row[stc] ?? ''
          y += pitch
          textLine(pad + 2 * tcw, y, sz, `<tspan class="${stateTone(cell)}">${xml(cell.slice(0, Math.max(1, Math.floor(avail / tcw) - 2)))}</tspan>`)
        }
        plain.push(row.join(' | '))
      })
      continue
    }

    const kind = b.kind
    if (kind === 'h2') {
      const st = secTone(b.runs.map(r => r.text).join(''))
      sec = b.runs.map(r => r.text).join('').toLowerCase()
      y += mono ? pitch : 18 * k
      const label0 = `${st.prefix}${b.runs.map(r => r.text).join('')}`
      const label = mono ? label0 : label0.toUpperCase()
      y += mono ? pitch : sizes.label * 1.5
      if (mono) {
        const name = b.runs.map(r => r.text).join('')
        const mark = st.tone === 'red' ? '!' : st.tone === 'amber' ? '◐' : /milestone|done|decision/i.test(name) ? '✓' : ''
        const head = `${mark ? `${mark} ` : ''}${name.toUpperCase()}`
        const fill = Math.max(2, maxChars - head.length - 4)
        textLine(pad, y, size, `<tspan class="dim">── </tspan><tspan class="w ${st.tone}">${xml(head)}</tspan><tspan class="dim"> ${'─'.repeat(fill)}</tspan>`)
      } else textLine(pad, y, sizes.label, `<tspan class="s ${st.tone}">${xml(label)}</tspan>`)
      plain.push(label)
      afterH1 = false
      continue
    }
    if (kind === 'h1') {
      const lines = wrapRuns(b.runs, Math.floor(maxChars * (mono ? 1 : 13 / 18)))
      if (mono) {
        const t = b.runs.map(r => r.text).join('').slice(0, Math.max(4, maxChars - 6))
        y += pitch
        textLine(pad, y, size, `<tspan class="dim">══ </tspan><tspan class="w fg">${xml(t)}</tspan><tspan class="dim"> ${'═'.repeat(Math.max(2, maxChars - t.length - 4))}</tspan>`)
        plain.push(t)
        afterH1 = true
        continue
      }
      for (const ln of lines) {
        y += sizes.h1 * 1.35
        textLine(pad, y, sizes.h1, tspans(ln, 'fg', sizes.h1, true))
        plain.push(ln.map(r => r.text).join(''))
      }
      afterH1 = true
      continue
    }
    if (kind === 'h3') {
      y += pitch * 0.35
      const lines = wrapRuns(b.runs.map(r => ({ ...r, text: mono ? r.text : r.text.toUpperCase() })), maxChars)
      for (const ln of lines) {
        y += mono ? pitch : sizes.label * 1.6
        textLine(pad, y, sizes.label, `<tspan class="${mono ? 'w' : 's'} dim">${xml(ln.map(r => r.text).join(''))}</tspan>`)
        plain.push(ln.map(r => r.text).join(''))
      }
      continue
    }

    // paragraphs and list items
    const isLi = kind === 'li'
    const isEvents = /event/i.test(sec)
    const isNote = !isLi && sec === '' && (afterH1 || /^updated\b/i.test(b.runs.map(r => r.text).join('')))
    const indent = isLi ? (b.depth + 1) * 2 : 0
    const max = Math.max(8, maxChars - indent - (isLi ? 2 : 0))
    let runs = b.runs
    if (/headline/i.test(sec) && !isLi) runs = tintHeadline(runs)
    if (isEvents) {
      const first = runs[0]
      const m = first ? /^(\d\d:\d\d)\s*/.exec(first.text) : null
      if (first && m) runs = [{ text: mono ? `${m[1]} · ` : `${m[1]}  `, tone: 'dim' }, { ...first, text: first.text.slice(m[0].length) }, ...runs.slice(1)]
    }
    const lines = isEvents ? [cutRuns(runs, max)] : wrapRuns(runs, max)
    const sz = isNote ? sizes.note : sizes.body
    lines.forEach((ln, li) => {
      y += pitch
      const base = isNote || isEvents ? 'dim' : 'fg'
      const x0 = pad + indent * cw
      if (isLi && li === 0) textLine(pad + (indent - 2) * cw, y, sz, `<tspan class="dim">${xml(mono && b.marker === '•' ? '▸' : b.marker)}</tspan>`)
      codeChips(x0, y, sz, ln)
      textLine(x0, y, sz, tspans(ln, base, sz, /headline/i.test(sec) && !isLi))
      plain.push(ln.map(r => r.text).join(''))
    })
  }
  if (opts.bottom !== false) y += pad
  const H = Math.max(1, Math.ceil(y))
  const dispW = W
  const dispH = H
  const css = mono ? `${MONO_CSS}text{font-family:${MONO};white-space:pre}.m{font-family:${MONO};font-size:${size}px}` : `${SVG_CSS}.mono{font-family:${MONO}}`
  return {
    source: `<svg xmlns="http://www.w3.org/2000/svg" width="${dispW}" height="${dispH}" viewBox="0 0 ${W} ${H}"><style>${css}</style>${out.join('')}</svg>`,
    alt: plain.slice(0, 6).join(' '),
    width: dispW,
    height: dispH,
    lines: plain,
  }
}

/** One line of `mw news --format text`, split into styled pieces by simple patterns. */
export type TextPiece = { text: string; cls: string }
const STATE_CLS: Record<string, string> = { done: 'green', closed: 'green', ready: 'amber', blocked: 'red' }
const URL_G = /https?:\/\/[^\s)<>]+/g

export const tokenizeNewsLine = (line: string): TextPiece[] => {
  if (/^══/.test(line)) return [{ text: line, cls: 'w fg' }]
  const h = /^── (.*?)( ─+)?$/.exec(line)
  if (h && line.startsWith('── ')) {
    const cls = line.startsWith('── !') ? 'w red' : line.startsWith('── ◐') ? 'w amber' : line.startsWith('── ✓') ? 'w green' : 'w fg'
    return [{ text: '── ', cls: 'dim' }, { text: h[1]!, cls }, { text: h[2] ?? '', cls: 'dim' }]
  }
  if (/^[│├]/.test(line)) {
    const out: TextPiece[] = []
    for (const part of line.split(/([│├┼┤─┬┴┌┐└┘]+)/)) {
      if (part === '') continue
      if (/^[│├┼┤─┬┴┌┐└┘]+$/.test(part)) out.push({ text: part, cls: 'dim' })
      else
        for (const q of part.split(/([█░]+)/)) {
          if (q === '') continue
          if (/^[█░]+$/.test(q)) {
            const full = q.match(/^█*/)![0]
            if (full) out.push({ text: full, cls: 'fg' })
            if (q.length > full.length) out.push({ text: q.slice(full.length), cls: 'trk' })
          } else {
            const w = STATE_CLS[q.trim().toLowerCase()]
            out.push({ text: q, cls: w ?? 'fg' })
          }
        }
    }
    return out
  }
  const out: TextPiece[] = []
  let rest = line
  const base = /^Updated\b/.test(line) ? 'dim' : 'fg'
  if (rest.startsWith('▸')) {
    out.push({ text: '▸', cls: 'dim' })
    rest = rest.slice(1)
  }
  let last = 0
  for (const m of rest.matchAll(URL_G)) {
    if (m.index! > last) out.push({ text: rest.slice(last, m.index), cls: base })
    out.push({ text: m[0], cls: 'acc u' })
    last = m.index! + m[0].length
  }
  if (last < rest.length) out.push({ text: rest.slice(last), cls: base })
  return out
}

/** The Page line (the header link already opens it) is dropped; the rest of the lines stay. */
export const newsTextLines = (text: string): string[] => {
  const out: string[] = []
  for (const raw of text.replace(/\r/g, '').replace(/\n+$/, '').split('\n')) {
    const l = /^\s*Page:/i.test(raw) ? '' : raw.replace(/(^|\s)Page:\s*https?:\/\/\S+/gi, '$1')
    if (l === '' && (out.length === 0 || out[out.length - 1] === '')) continue
    out.push(l)
  }
  return out
}

/** The header already shows the job's title and description: drop mw's `══` title line and the description lines before "Updated". */
export const stripTextHead = (lines: string[]): string[] => {
  let i = 0
  if (/^══/.test(lines[0] ?? '')) i = 1
  const up = lines.findIndex((l, k) => k >= i && k < i + 5 && /^Updated\b/i.test(l))
  if (up > i) i = up
  return lines.slice(i).filter((l, k, all) => !(l === '' && k === 0) && !(l === '' && all[k - 1] === ''))
}

/** Same for mw's md: its H1 and the italic description paragraph go; "Updated …" stays. */
export const stripMdHead = (blocks: MdBlock[]): MdBlock[] => {
  let i = 0
  if (blocks[0]?.kind === 'h1') i = 1
  while (blocks[i]?.kind === 'p' && (blocks[i] as { runs: MdRun[] }).runs.every(r => r.italic) && !/^updated/i.test((blocks[i] as { runs: MdRun[] }).runs.map(r => r.text).join(''))) i += 1
  while (blocks[i]?.kind === 'gap') i += 1
  return blocks.slice(i)
}

/** The description wrapped to `cols`, at most 2 lines; "…" only when still longer. */
export const wrapDesc = (text: string, cols: number): string[] => {
  const words = text.replace(/\s+/g, ' ').trim().split(' ').filter(Boolean)
  const lines: string[] = []
  let cur = ''
  let i = 0
  for (; i < words.length; i++) {
    const w = words[i]!
    if (cur === '' ? w.length <= cols : cur.length + 1 + w.length <= cols) cur = cur === '' ? w : `${cur} ${w}`
    else if (lines.length === 0) {
      lines.push(cur)
      cur = w.length <= cols ? w : w.slice(0, cols)
    } else break
  }
  if (cur !== '') lines.push(cur)
  if (i < words.length) {
    const last = lines[lines.length - 1] ?? ''
    lines[lines.length - 1] = `${last.length >= cols ? last.slice(0, cols - 1) : last}\u2026`
  }
  return lines.slice(0, 2)
}

export const newsTextUrls = (lines: string[]): Array<[string, string]> => {
  const seenUrls = new Map<string, string>()
  for (const l of lines) for (const m of l.matchAll(URL_G)) {
    const u = m[0].replace(/[.,;:!?]+$/, '')
    if (!seenUrls.has(u)) seenUrls.set(u, shortUrl(u))
  }
  return [...seenUrls]
}

/** mw's own text render drawn in one mono size: colours only by line pattern. */
export const buildTextSvg = (lines: string[], step: number, paneW: number | undefined): { source: string; alt: string; width: number; height: number } => {
  const size = svgSizeFor(step)
  const cw = size * 0.6
  const pad = 8
  const cols = monoCols(paneW, cw, pad)
  const pitch = Math.round(size * 1.6)
  const W = Math.round((2 * pad + cols * cw) * 100) / 100
  const out = lines.map((l, i) => {
    const body = tokenizeNewsLine(l).map(pc => `<tspan class="${pc.cls}">${xml(pc.text)}</tspan>`).join('')
    return `<text x="${pad}" y="${4 + i * pitch + Math.round(pitch * 0.72)}" class="m" xml:space="preserve">${body}</text>`
  })
  const H = 4 + lines.length * pitch + 6
  const css = `${MONO_CSS}text{font-family:${MONO};white-space:pre}.m{font-family:${MONO};font-size:${size}px}`
  return { source: `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}"><style>${css}</style>${out.join('')}</svg>`, alt: lines.slice(0, 6).join(' '), width: W, height: H }
}

/** Why the last poll of a job failed, in words: mw not found, board folder missing, mw timed out, board busy. */
const jobWhy = new Map<string, string>()
let timer: { cancel: () => void } | undefined
const inFlight = new Set<string>()

/** A job that left the registry: say so once and drop what was kept for it (T5). */
let knownIds: string[] | undefined
const goneJobs = async ($: any, reg: MwJobsRegistry) => {
  const ids = reg.jobs.map(j => j.id)
  const was = knownIds
  if (reg.error === null || reg.jobs.length > 0) knownIds = ids
  if (was === undefined) return
  for (const id of was.filter(x => !ids.includes(x))) {
    $.ui.toast(`Jobs: job ${id} gone`, { timeoutMs: 6000 })
    const drop = (m: any) => Object.fromEntries(Object.entries(m ?? {}).filter(([k]) => k !== id))
    await update($, snapshots, drop as any)
    await update($, previous, drop as any)
    await update($, staleSince, drop as any)
    await update($, seen, drop as any)
    await update($, jobDesc, drop as any)
    await update($, inChat, drop as any)
    await update($, alerted, drop as any)
    await update($, news, drop as any)
    await update($, newsModel, drop as any)
    await update($, newsView, drop as any)
    await update($, relKnown, drop as any)
    await update($, expanded, drop as any)
    await update($, tenantSel, drop as any)
    lastDrafts.delete(id)
    polledAt.delete(id)
  }
}

// ---- board discovery: when the registry is empty, the jobs come from the boards `mw boards` finds (read-only) ----
export const SCAN_OLD_HINT = 'Scan needs mw 0.3.4+ (mw boards); showing the registry only.'
type ScanBoard = { dir: string; name: string; tenants: { slug: string; name: string; archived: boolean }[] }
type ScanResult = { jobs: MwJobsJob[]; hint: string | null; error: string | null }
let sessionCwd = ''
let scanCache: ScanResult | undefined
let scanRun: Promise<ScanResult> | undefined

/** `mw boards -j` as boards (those with an error left out), or undefined when it is not that JSON. */
export const parseBoards = (text: string | undefined): ScanBoard[] | undefined => {
  const v = text === undefined ? undefined : (parseJson(text) as { boards?: unknown } | undefined)
  if (v === undefined || v === null || typeof v !== 'object' || !Array.isArray(v.boards)) return undefined
  return (v.boards as Record<string, unknown>[])
    .filter(b => typeof b?.dir === 'string' && (b.error === undefined || b.error === null || b.error === ''))
    .map(b => ({
      dir: str(b.dir),
      name: str(b.name) || str(b.dir).split('/').pop() || str(b.dir),
      tenants: (Array.isArray(b.tenants) ? (b.tenants as Record<string, unknown>[]) : []).filter(t => typeof t?.slug === 'string').map(t => ({ slug: str(t.slug), name: str(t.name) || str(t.slug), archived: t.archived === true })),
    }))
}

/** Rows for the panes: every mw job of every board, and for a board with no jobs one row per (live) tenant. Ids stay unique (a clash gets "@board"). */
export const jobsFromBoards = (boards: ScanBoard[], jobsOf: Record<string, string | undefined>, home: string): MwJobsJob[] => {
  const out: MwJobsJob[] = []
  const used = new Set<string>()
  const uniq = (id: string, b: ScanBoard) => {
    let u = used.has(id) ? `${id}@${b.name}` : id
    for (let n = 2; used.has(u); n += 1) u = `${id}@${b.name}${n}`
    used.add(u)
    return u
  }
  const tilde = (dir: string) => (home !== '' && (dir === home || dir.startsWith(`${home}/`)) ? `~${dir.slice(home.length)}` : dir)
  const base = { news: '', coordinator: '', desk: '', alerts: {} as MwJobsJob['alerts'] }
  for (const b of boards) {
    const js = parseJson(jobsOf[b.dir] ?? '')
    const list = Array.isArray(js) ? (js as Record<string, unknown>[]).filter(j => typeof j?.id === 'string') : []
    if (list.length > 0) {
      for (const j of list) {
        out.push({ ...base, id: uniq(str(j.id), b), title: str(j.title) || str(j.id), board: tilde(b.dir), tenants: Array.isArray(j.tenants) ? j.tenants.map(str) : [], metrics: str(j.metrics), description: str(j.description), scanned: 'job', boardName: b.name })
      }
      continue
    }
    const live = b.tenants.filter(t => !t.archived)
    for (const t of live.length > 0 ? live : b.tenants) {
      out.push({ ...base, id: uniq(t.slug, b), title: t.name, board: tilde(b.dir), tenants: [t.slug], metrics: '', description: `${t.name}: a tenant of the ${b.name} board (no mw job)`, scanned: 'tenant', boardName: b.name })
    }
  }
  return out
}

// ---- boards this session used: seen at tool.call (after the call ran, never changing it) and in the transcript at start ----
export const USED_MAX = 200
const MW_CMD = /(?:^|[;&|(`\s/])mw(?:\s|$)/
const resolvedDir = new Map<string, string[]>()

const unq = (t: string) => t.replace(/^['"]|['"]$/g, '')
const absDir = (p: string, cwd: string, home: string): string => {
  const x = unq(p)
  const a = x === '~' ? home : x.startsWith('~/') ? `${home}${x.slice(1)}` : isAbs(x) ? x : `${cwd.replace(/[\\/]$/, '')}/${x}`
  return a.replace(/\/\.(?=\/|$)/g, '').replace(/\/+$/, '') || '/'
}

/** The folders a tool call points at a board through: a Bash command that runs mw (its cwd, a `cd X`, `-C X`, `--home X`, path arguments), a Read/Edit/Write of a .miniwaves file. Pure. */
export const boardHints = (tool: string, input: Record<string, unknown>, cwd: string, home: string): string[] => {
  const out: string[] = []
  const add = (d: string) => { if (d !== '' && !out.includes(d)) out.push(d) }
  if (tool === 'Bash') {
    const cmd = typeof input.command === 'string' ? input.command : ''
    if (!MW_CMD.test(cmd)) return []
    let here = cwd
    // each `cd X` moves the folder the next mw runs in
    for (const part of cmd.split(/&&|\|\||;|\n/)) {
      const words = part.trim().split(/\s+/).filter(w => w !== '')
      if (words[0] === 'cd' && words[1] !== undefined) { here = absDir(words[1], here, home); continue }
      const at = words.findIndex(w => w === 'mw' || /[\\/]mw(?:\.exe)?$/.test(w))
      if (at < 0) continue
      add(here)
      const rest = words.slice(at + 1)
      rest.forEach((w, i) => {
        if ((w === '-C' || w === '--home' || w === '--dir') && rest[i + 1] !== undefined) add(absDir(rest[i + 1]!, here, home))
        else if (w.startsWith('--home=')) add(absDir(w.slice(7), here, home))
        else if (/^['"]?(~|\/|\.\.?\/)/.test(w)) add(absDir(w, here, home))
      })
    }
    return out
  }
  if (tool === 'Read' || tool === 'Edit' || tool === 'Write' || tool === 'MultiEdit' || tool === 'NotebookEdit') {
    const f = String(input.file_path ?? input.notebook_path ?? '')
    const m = /^(.*?)\/\.miniwaves(?:\.json|\/|$)/.exec(absDir(f, cwd, home))
    if (m) add(m[1] || '/')
  }
  return out
}

/** Folders to board roots (`mw boards DIR --up --depth 0 -j`, cached per folder), added to the session's used set, deduped by root. */
const noteUsed = async ($: any, dirs: string[]): Promise<boolean> => {
  let added = false
  for (const d of dirs) {
    let roots = resolvedDir.get(d)
    let found: ScanBoard[] = []
    if (roots === undefined) {
      try {
        const r = await proc($, ['mw', 'boards', d, '--up', '--depth', '0', '-j'], { cwd: '/', timeoutMs: 20_000 })
        found = (r.exitCode === 0 ? parseBoards(String(r.stdout)) : undefined) ?? []
      } catch {
        found = []
      }
      roots = found.map(b => b.dir)
      resolvedDir.set(d, roots)
    }
    if (found.length === 0) continue
    const cur = (await read($, usedBoards)) as MwJobsUsedBoard[]
    const fresh = found.filter(b => !cur.some(c => c.dir === b.dir))
    if (fresh.length === 0) continue
    await update($, usedBoards, (l: MwJobsUsedBoard[]) => [...l, ...fresh.filter(b => !l.some(c => c.dir === b.dir))].slice(-USED_MAX))
    added = true
  }
  return added
}

/** After a tool call: note the boards it used; a new one rescans the pane's list (only when the list comes from discovery). */
const watchUse = async ($: any, e: any) => {
  try {
    const home: string = (await homeOf($))
    const cwd: string = (await $.session.cwd().catch(() => '')) || sessionCwd || home
    const dirs = boardHints(String(e?.tool ?? ''), (e ?? {}) as Record<string, unknown>, cwd, home)
    if (dirs.length === 0) return
    if (await noteUsed($, dirs)) {
      const reg = (await read($, registry)) as MwJobsRegistry
      if (reg.source !== 'registry' || reg.jobs.length === 0) {
        scanCache = undefined
        await loadRegistry($)
      }
    }
  } catch {
    // watching is best effort
  }
}

/** At start and on reload: the transcript so far (and its agents') for mw runs and .miniwaves files, read only. */
const replayTranscript = async ($: any) => {
  try {
    const home: string = (await homeOf($))
    const cwd: string = (await $.session.cwd().catch(() => '')) || sessionCwd || home
    const msgs = await $.session.messages()
    if (!Array.isArray(msgs)) return
    const uses: { tool: string; input: Record<string, unknown> }[] = []
    const agents: string[] = []
    for (const m of msgs) {
      for (const u of (m?.toolUses ?? []) as any[]) {
        uses.push({ tool: String(u?.tool ?? ''), input: (u?.input ?? {}) as Record<string, unknown> })
        if (typeof u?.agentId === 'string' && !agents.includes(u.agentId)) agents.push(u.agentId)
      }
    }
    for (const id of agents.slice(-USED_MAX)) {
      try {
        const am = await $.session.messages({ agentId: id })
        if (Array.isArray(am)) for (const m of am) for (const u of (m?.toolUses ?? []) as any[]) uses.push({ tool: String(u?.tool ?? ''), input: (u?.input ?? {}) as Record<string, unknown> })
      } catch {
        // an agent the session cannot read
      }
    }
    const dirs: string[] = []
    for (const u of uses) for (const d of boardHints(u.tool, u.input, cwd, home)) if (!dirs.includes(d)) dirs.push(d)
    if (dirs.length > 0) await noteUsed($, dirs)
  } catch {
    // no transcript to read
  }
}

/** One discovery, first hit first, the rest appended, deduped by root: the boards this session used ("this session"), then up from the session's folders (3 deep); only when both are empty the home folder (4 deep). Then `mw jobs show -j` on each board. An mw without `boards` gives the hint. */
const scanBoards = async ($: any, dir?: string): Promise<ScanResult> => {
  const home: string = (await homeOf($))
  const run = async (argv: string[], cwd: string): Promise<string | undefined> => {
    try {
      const r = await proc($, argv, { cwd, timeoutMs: 30_000 })
      return r.exitCode === 0 ? String(r.stdout) : undefined
    } catch {
      return undefined
    }
  }
  const used = (await read($, usedBoards)) as MwJobsUsedBoard[]
  const cwd = sessionCwd || home || '/'
  let root = ''
  try {
    root = String((await $.session.root()) ?? '')
  } catch {
    root = ''
  }
  const folders = dir !== undefined ? [expand(dir, home)] : [cwd, ...(root && root !== cwd ? [root] : [])]
  const boards: ScanBoard[] = [...used]
  const addAll = (l: ScanBoard[]) => { for (const b of l) if (!boards.some(x => x.dir === b.dir)) boards.push(b) }
  let old = false
  // discover.home / discover.depth (mw keys): whether and how deep ~ (or an asked-for folder) is scanned
  const scanHome = (await cfgGet($, 'discover.home')) !== 'false'
  const depth0 = Number(await cfgGet($, 'discover.depth'))
  const depth = Number.isInteger(depth0) && depth0 >= 1 && depth0 <= 6 ? depth0 : 4
  for (const f of folders) {
    const got = parseBoards(await run(dir !== undefined ? ['mw', 'boards', f, '--depth', String(depth), '-j'] : ['mw', 'boards', f, '--up', '--depth', '3', '-j'], f))
    if (got === undefined) old = true
    else addAll(got)
  }
  if (old && boards.length === 0) return { jobs: [], hint: SCAN_OLD_HINT, error: null }
  if (boards.length === 0 && dir === undefined && home !== '' && scanHome) addAll(parseBoards(await run(['mw', 'boards', home, '--depth', String(depth), '-j'], home)) ?? [])
  if (boards.length === 0) return { jobs: [], hint: null, error: `no mw boards found under ${cwd} or ~` }
  const outs = await Promise.all(boards.map(async b => [b.dir, await run(['mw', 'jobs', 'show', '-j'], b.dir)] as const))
  const usedDirs = new Set(used.map(b => b.dir))
  const jobs = jobsFromBoards(boards, Object.fromEntries(outs), home).map(j => (usedDirs.has(expand(j.board, home)) ? { ...j, thisSession: true, description: `this session \u00b7 ${j.description ?? ''}`.replace(/ \u00b7 $/, '') } : j))
  const nUsed = boards.filter(b => usedDirs.has(b.dir)).length
  return { jobs, hint: `Found by scan: ${boards.length} board${boards.length === 1 ? '' : 's'}${nUsed > 0 ? `, ${nUsed} used this session` : ''} (no registry at ${registryPath()})`, error: null }
}

/** The discovery result, run once per session (or again with force, the Scan button); concurrent callers share one run. */
const scanned = async ($: any, force = false, dir?: string): Promise<ScanResult> => {
  if (!force && scanCache !== undefined) return scanCache
  if (scanRun === undefined) {
    scanRun = scanBoards($, dir).finally(() => { scanRun = undefined })
  }
  scanCache = await scanRun
  return scanCache
}

/** The Scan button: discover the boards again, then refresh. */
const rescan = async ($: any, dir?: string) => {
  await scanned($, true, dir)
  await loadRegistry($)
  await refreshAll($)
}

/** `/mw-watch scan [DIR]`: look for boards again (under DIR, else the usual places), then open the jobs pane. */
const scanCommand = async ($: any, dir?: string) => {
  await rescan($, dir)
  await openMain($, true)
  const reg: MwJobsRegistry = await read($, registry)
  return { text: reg.jobs.length === 0 ? 'Jobs: no mw boards found.' : `Jobs: found ${reg.jobs.length} job${reg.jobs.length === 1 ? '' : 's'}.` }
}

/** The one place the panes' job list comes from: the registry file, else the boards discovery finds. */
const loadRegistry = async ($: any) => {
  const home: string = (await homeOf($))
  let reg: MwJobsRegistry
  try {
    reg = { ...parseRegistry(await $.fs.read(expand(registryPath(), home))), source: 'registry' }
  } catch {
    reg = { jobs: [], error: 'no jobs registered', source: 'registry' }
  }
  if (reg.jobs.length === 0) {
    const sc = await scanned($)
    reg = sc.jobs.length > 0 ? { jobs: sc.jobs, error: null, source: 'scan', hint: sc.hint } : { ...reg, error: reg.error === 'no jobs registered' ? (sc.error ?? reg.error) : reg.error, hint: sc.hint }
  }
  reg.loaded = true
  await goneJobs($, reg)
  await update($, registry, () => reg)
  const cur: string = await read($, selected)
  if (reg.jobs.length > 0 && !reg.jobs.some(j => j.id === cur)) {
    await update($, selected, () => reg.jobs[0]!.id)
  }
  return reg
}

const renderStatus = async ($: any) => {
  const reg: MwJobsRegistry = await read($, registry)
  // status.show=false (mw key): no chat-bar status at all
  if (reg.jobs.length === 0 || (await cfgGet($, 'status.show')) === 'false') {
    $.ui.status(undefined)
    return
  }
  const snaps: Record<string, MwJobsSnapshot> = await read($, snapshots)
  const stale: Record<string, number> = await read($, staleSince)
  const units = statusUnits(reg.jobs)
  const full = (await cfgGet($, 'status.detail')) === 'full'
  const seg = (u: (typeof units)[number]) => ('job' in u ? statusSegment(u.job, snaps[u.job.id], stale[u.job.id], full) : boardSegment(u.board, u.jobs.map(j => snaps[j.id]), full))
  if (units.length <= STATUS_FULL) {
    $.ui.status(units.map(seg).join(' │ '))
    return
  }
  // many jobs: the totals, then the selected job (or its board), at most 200 characters
  const sel: string = await read($, selected)
  const selUnit = units.find(u => ('job' in u ? u.job.id === sel : u.jobs.some(j => j.id === sel)))
  $.ui.status(statusTotals(reg.jobs.map(j => snaps[j.id]), selUnit ? seg(selUnit) : ''))
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

/** Advance a job's seen cursor to now, with the metric values as they stand. */
const markSeen = async ($: any, id: string) => {
  const now: number = await $.clock.now()
  const snaps: Record<string, MwJobsSnapshot> = await read($, snapshots)
  await update($, seen, (m: Record<string, MwJobsSeen>) => ({ ...m, [id]: { ts: now, metrics: metricsBase(snaps[id]) } }))
  await saveSeen($)
}

// ---- per-job log files: <dirname(registry)>/logs/<job-id>/{alerts,decisions}.log, written with $.fs only, nowhere else ----
export const LOG_MAX = 1_000_000
const logRoot = (home: string) => `${dirOf(expand(registryPath(), home))}/logs`
const dirOf = (p: string) => {
  const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'))
  return i > 0 ? p.slice(0, i) : '.'
}
const safeId = (id: string) => /^[A-Za-z0-9_.-]+$/.test(id) && !id.includes('..')
let logChain: Promise<void> = Promise.resolve()

/** Appends lines to one of a job's log files. $.fs has no append: read, add, write. Over 1 MB the file becomes <name>.1 (one old copy). */
export const appendLog = ($: any, jobId: string, file: 'alerts.log' | 'decisions.log', lines: string[]): Promise<void> => {
  if (lines.length === 0 || !safeId(jobId)) return Promise.resolve()
  logChain = logChain.then(async () => {
    try {
      const home: string = (await homeOf($))
      const path = `${logRoot(home)}/${jobId}/${file}`
      let cur = ''
      try {
        cur = String(await $.fs.read(path))
      } catch {
        cur = ''
      }
      if (cur.length > LOG_MAX) {
        await $.fs.write(`${path}.1`, cur)
        cur = ''
      }
      await $.fs.write(path, `${cur}${lines.join('\n')}\n`)
    } catch {
      await logWriteFailed($)
    }
  })
  return logChain
}

/** One toast per session-and-store: the write failure key rides in the same once-only list as the alerts. */
const logWriteFailed = async ($: any) => {
  const m = (await read($, alerted)) as Record<string, string[]>
  if ((m._sys ?? []).includes('fail:logwrite')) return
  await update($, alerted, (x: Record<string, string[]>) => ({ ...x, _sys: capAlerted(x._sys ?? [], ['fail:logwrite']) }))
  const home: string = (await homeOf($))
  $.ui.toast(`Jobs: could not write the log files in ${logRoot(home)}`, { timeoutMs: 8000 })
}

const oneLine = (t: string, max = 300) => cleanBoard(t, max)
export const alertLine = (iso: string, kind: string, who: string, text: string, delivered: string) => `${iso}  ${kind}  ${who}  ${oneLine(text)}  -> ${delivered}`
export const decisionLine = (e: { ts: string; tenant: string; item: string; action: string; by?: string; note?: string }) =>
  `${e.ts}  ${e.tenant} ${e.item}  ${e.action}  ${oneLine(e.by ?? '', 60) || '-'}  ${oneLine(e.note ?? '', 400) || '-'}`

/** A failure (pane not placed, fill refused, ⧉ refused, mw failed or stale) goes to alerts.log once per distinct failure. */
const logFail = async ($: any, jobId: string, what: string, reason: string) => {
  if (!safeId(jobId)) return
  const key = `fail:${what}:${reason}`
  const m = (await read($, alerted)) as Record<string, string[]>
  const done = m[jobId] ?? []
  if (done.includes(key)) return
  await update($, alerted, (x: Record<string, string[]>) => ({ ...x, [jobId]: capAlerted(x[jobId] ?? [], [key]) }))
  try {
    await $.store.set('alerted', await read($, alerted))
  } catch {
    // store unavailable
  }
  const now: number = await $.clock.now()
  await appendLog($, jobId, 'alerts.log', [alertLine(new Date(now).toISOString(), 'failure', '-', `${what}: ${reason}`, 'logged')])
}

/** The audit trail: accept, reject, answer and decide events per tenant, resumed from a cursor kept in $.store (first read starts at `start`). */
const DECISION_ACTIONS = new Set(['accept', 'reject', 'answer', 'decide'])
const syncDecisions = async ($: any, job: MwJobsJob, cwd: string) => {
  if (!safeId(job.id)) return
  for (const tenant of job.tenants) {
    const ckey = `logCursor:${job.id}:${tenant}`
    let cursor = 'start'
    try {
      const c = await $.store.get(ckey)
      if (typeof c === 'string' && c !== '') cursor = c
    } catch {
      cursor = 'start'
    }
    let out: { events?: unknown; next?: unknown } | undefined
    try {
      const r = await proc($, ['mw', 'log', '-t', tenant, '--since', cursor, '-j'], { cwd, timeoutMs: 20_000 })
      out = r.exitCode === 0 ? (parseJson(r.stdout) as { events?: unknown; next?: unknown } | undefined) : undefined
    } catch {
      out = undefined
    }
    if (!out || !Array.isArray(out.events)) continue
    const lines = (out.events as Array<Record<string, unknown>>)
      .filter(e => DECISION_ACTIONS.has(str(e.action)))
      .map(e => decisionLine({ ts: str(e.ts), tenant: str(e.tenant) || tenant, item: str(e.item), action: str(e.action), by: str(e.by), note: str(e.note) }))
    await appendLog($, job.id, 'decisions.log', lines)
    if (typeof out.next === 'string' && out.next !== '') {
      try {
        await $.store.set(ckey, out.next)
      } catch {
        // store unavailable
      }
    }
  }
}

// ---- "in chat": an item whose question was sent to the chat stops counting as needing the person ----
export const CHAT_MS = 30 * 60_000
const openOnBoard = (i: MwJobsItem, s: MwJobsSnapshot) => i.state !== 'done' && (isReview(i) || i.state === 'blocked' || askMap(s).has(`${i.tenant}:${i.id}`))

/** Recomputes a job's active in-chat set: a mark ends when the board shows the item resolved or after 30 minutes. */
const applyChat = async ($: any, jobId: string, now: number) => {
  const snaps = (await read($, snapshots)) as Record<string, MwJobsSnapshot>
  const snap = snaps[jobId]
  const marks = (((await read($, inChat)) as Record<string, Record<string, number>>)[jobId] ?? {}) as Record<string, number>
  if (snap === undefined) return
  const active: Record<string, number> = {}
  for (const [key, ts] of Object.entries(marks)) {
    const item = snap.items.find(i => `${i.tenant}:${i.id}` === key)
    if (item !== undefined && openOnBoard(item, snap) && now - ts < CHAT_MS) active[key] = ts
  }
  const changed = JSON.stringify(active) !== JSON.stringify(marks)
  if (changed) {
    await update($, inChat, (m: Record<string, Record<string, number>>) => ({ ...m, [jobId]: active }))
    try {
      await $.store.set('inChat', await read($, inChat))
    } catch {
      // store unavailable
    }
  }
  await update($, snapshots, (m: Record<string, MwJobsSnapshot>) => (m[jobId] ? { ...m, [jobId]: { ...m[jobId]!, inChat: active } } : m))
}

/** Items a submitted text answers: our filled questions start "About t ID", "Review t ID" or "t ID ... is blocked", or list "- t ID" lines under "For each:". */
export const chatKeys = (text: string): string[] => {
  const keys = new Set<string>()
  for (const m of text.matchAll(/(?:^|\n)(?:About|Review) (\S+) (\S+) "/g)) keys.add(`${m[1]}:${m[2]}`)
  for (const m of text.matchAll(/(?:^|\n)(\S+) (\S+) ".*?" is blocked:/g)) keys.add(`${m[1]}:${m[2]}`)
  if (/For each:/.test(text)) for (const m of text.matchAll(/(?:^|\n)- (\S+) (\S+) "/g)) keys.add(`${m[1]}:${m[2]}`)
  return [...keys]
}

const markChat = async ($: any, text: string) => {
  const keys = chatKeys(text)
  if (keys.length === 0) return
  const reg: MwJobsRegistry = await read($, registry)
  const snaps = (await read($, snapshots)) as Record<string, MwJobsSnapshot>
  const now: number = await $.clock.now()
  for (const job of reg.jobs) {
    const snap = snaps[job.id]
    if (snap === undefined) continue
    const mine = keys.filter(k => snap.items.some(i => `${i.tenant}:${i.id}` === k && openOnBoard(i, snap)))
    if (mine.length === 0) continue
    await update($, inChat, (m: Record<string, Record<string, number>>) => ({ ...m, [job.id]: { ...(m[job.id] ?? {}), ...Object.fromEntries(mine.map(k => [k, now])) } }))
    await applyChat($, job.id, now)
  }
  await renderStatus($)
}

/** Reads a metrics file with $.fs: a missing file is no section (undefined); a link, a non-file, an oversize or a bad file is a one-line notice. */
const readMetrics = async ($: any, path: string): Promise<MetricsResult | undefined> => {
  let st: { isLink?: boolean; kind?: string; size?: number }
  try {
    st = await $.fs.stat(path, { resolve: true })
  } catch {
    return undefined
  }
  if (st.isLink) return { notice: 'metrics file invalid: symbolic link' }
  if (st.kind !== 'file') return { notice: 'metrics file invalid: not a file' }
  if ((st.size ?? 0) > METRICS_MAX_BYTES) return { notice: 'metrics file invalid: file over 256 KB' }
  try {
    return parseMetrics(String(await $.fs.read(path)), st.size)
  } catch {
    return undefined
  }
}


/** What an alert is about; alerts.kinds (an mw key) picks among these. Decided from the item's structured field, never from the text. */
export type AlertKind = 'ask' | 'review' | 'blocked' | 'done' | 'new' | 'release' | 'metrics'
export type AlertEvent = { type: AlertType; kind: AlertKind; text: string; who?: string; key?: string; ref?: string; link?: { kind: 'item' | 'wave'; tenant: string; id: string } }

export const ALERT_TITLE_MAX = 60
/** `<job> ▸ <What>: <ref> <title>`: the one text of every alert (toast and pane row). The ref is dropped when the title already starts with it; the title is cut to 60 characters. */
export const alertText = (job: string, what: string, ref: string, title: string): string => {
  const t = title.replace(/\s+/g, ' ').trim()
  const bare = ref.includes('\u00b7') ? ref.slice(ref.indexOf('\u00b7') + 1) : ref
  const dup = ref !== '' && [ref, bare].some(r => t === r || t.startsWith(`${r} `))
  const cut = t.length > ALERT_TITLE_MAX ? `${t.slice(0, ALERT_TITLE_MAX - 1).trimEnd()}\u2026` : t
  const tail = [dup ? '' : ref, cut].filter(Boolean).join(' ')
  return `${job} \u25b8 ${what}${tail !== '' ? `: ${tail}` : ''}`
}
const itemRef = (labels: Record<string, string>, tenant: string, id: string) => (labels[tenant] ? `${labels[tenant]}\u00b7${id}` : id)

/**
 * "Now" events from two news models of one job: needs you, review, blocked, done, closed milestones and new items that are in
 * `next` and were not in `old`. Keyed by type + tenant:id + event time. Draft changes alone are not alerts; these are items.
 */
export const modelEvents = (job: { id: string; tenants?: string[] }, old: Model, next: Model): AlertEvent[] => {
  const out: AlertEvent[] = []
  const lists = [next.happened.closed_milestones, next.now.needs, next.now.review, next.now.blocked, next.happened.done, next.happened.new_items]
  const tenants = job.tenants !== undefined && job.tenants.length > 0 ? job.tenants : [...new Set(lists.flatMap(l => l.items.map(i => i.tenant)))]
  const labels = tenantLabels(tenants)
  const add = (type: AlertType, what: string, ak: AlertKind, oldList: MList, list: MList, kind: string) => {
    const was = new Set(oldList.items.map(i => `${i.tenant}:${i.id}`))
    for (const i of list.items) {
      const k = `${i.tenant}:${i.id}`
      if (was.has(k)) continue
      out.push({ type, kind: ak, text: alertText(job.id, what, itemRef(labels, i.tenant, i.id), i.title), who: `${i.tenant} ${i.id}`, key: `${kind}|${k}|${i.since}`, ref: itemRef(labels, i.tenant, i.id), link: { kind: kind === 'milestone' ? 'wave' : 'item', tenant: i.tenant, id: i.id } })
    }
  }
  add('milestone', 'Milestone done', 'done', old.happened.closed_milestones, next.happened.closed_milestones, 'milestone')
  add('needs', 'Needs you', 'ask', old.now.needs, next.now.needs, 'needs')
  add('needs', 'Review', 'review', old.now.review, next.now.review, 'review')
  add('needs', 'Blocked', 'blocked', old.now.blocked, next.now.blocked, 'blocked')
  add('done', 'Done', 'done', old.happened.done, next.happened.done, 'done')
  add('new', 'New', 'new', old.happened.new_items, next.happened.new_items, 'new')
  return out
}

const metricEvent = (jobId: string, d: { line: string; who: string }): AlertEvent => ({ type: 'metric', kind: 'metrics', text: `${jobId} \u25b8 Metrics drop: ${d.line}`, who: d.who, key: `metric|${d.line}` })
const metricChangeEvent = (jobId: string, d: { line: string; who: string }): AlertEvent => ({ type: 'metric', kind: 'metrics', text: `${jobId} \u25b8 Metrics change: ${d.line}`, who: d.who, key: `metric-change|${d.line}` })
/** The alerts of a metrics.json change: rows that got worse ("Metrics drop") and every other change ("Metrics change"). */
export const metricAlerts = (jobId: string, prev: MwMetrics | undefined, next: MwMetrics | undefined): AlertEvent[] => [
  ...metricDrops(prev, next).map(d => metricEvent(jobId, d)),
  ...metricChanges(prev, next).map(d => metricChangeEvent(jobId, d)),
]

/** The last draft model polled per job: the baseline the next poll's events are measured against (first sight is silent). */
const lastDrafts = new Map<string, Model>()

/** A release being cut: the refresh that follows runs once the one in flight is done. */
const forceAfter = new Set<string>()

export const legacyEvents = (job: MwJobsJob, old: MwJobsSnapshot, next: MwJobsSnapshot): AlertEvent[] => {
  const labels = tenantLabels(job.tenants.length > 0 ? job.tenants : [...new Set(next.waves.map(w => w.id.split(':')[0] ?? ''))])
  const oldKeys = new Map(old.items.map(i => [`${i.tenant}:${i.id}`, i]))
  const oldWaves = new Map(old.waves.map(w => [w.id, w]))
  const flag = (x: MwJobsItem, snapOf: MwJobsSnapshot) => x.state === 'blocked' || askMap(snapOf).has(`${x.tenant}:${x.id}`)
  const out: AlertEvent[] = []
  for (const w of next.waves) {
    const was = oldWaves.get(w.id)
    if (was !== undefined && was.state !== 'done' && w.state === 'done') {
      out.push({ type: 'milestone', kind: 'done', text: alertText(job.id, 'Milestone done', waveName(w, labels), w.title), who: w.id.replace(':', ' '), key: `milestone|${w.id}`, ref: waveName(w, labels), link: { kind: 'wave', tenant: w.id.split(':')[0] ?? '', id: w.id } })
    }
  }
  for (const i of next.items) {
    const was = oldKeys.get(`${i.tenant}:${i.id}`)
    const k = `${i.tenant}:${i.id}`
    if (i.state !== 'done' && !isReview(i) && flag(i, next) && (was === undefined || !flag(was, old))) {
      const blocked = i.state === 'blocked'
      out.push({ type: 'needs', kind: blocked ? 'blocked' : 'ask', text: alertText(job.id, blocked ? 'Blocked' : 'Needs you', itemRef(labels, i.tenant, i.id), i.title), who: `${i.tenant} ${i.id}`, key: `${blocked ? 'blocked' : 'needs'}|${k}`, ref: itemRef(labels, i.tenant, i.id), link: { kind: 'item', tenant: i.tenant, id: i.id } })
    }
  }
  for (const i of next.items) {
    const was = oldKeys.get(`${i.tenant}:${i.id}`)
    if (isReviewIn(i, next) && (was === undefined || !isReviewIn(was, old))) out.push({ type: 'needs', kind: 'review', text: alertText(job.id, 'Review', itemRef(labels, i.tenant, i.id), i.title), who: `${i.tenant} ${i.id}`, key: `review|${i.tenant}:${i.id}`, ref: itemRef(labels, i.tenant, i.id), link: { kind: 'item', tenant: i.tenant, id: i.id } })
  }
  for (const i of next.items) {
    const was = oldKeys.get(`${i.tenant}:${i.id}`)
    if (was !== undefined && was.state !== i.state && i.state === 'done') out.push({ type: 'done', kind: 'done', text: alertText(job.id, 'Done', itemRef(labels, i.tenant, i.id), i.title), who: `${i.tenant} ${i.id}`, key: `done|${i.tenant}:${i.id}`, ref: itemRef(labels, i.tenant, i.id), link: { kind: 'item', tenant: i.tenant, id: i.id } })
  }
  for (const i of next.items) {
    if (!oldKeys.has(`${i.tenant}:${i.id}`) && !isReviewIn(i, next)) out.push({ type: 'new', kind: 'new', text: alertText(job.id, 'New', itemRef(labels, i.tenant, i.id), i.title), who: `${i.tenant} ${i.id}`, key: `new|${i.tenant}:${i.id}`, ref: itemRef(labels, i.tenant, i.id), link: { kind: 'item', tenant: i.tenant, id: i.id } })
  }
  out.push(...metricAlerts(job.id, old.metrics, next.metrics))
  return out
}

const inFlightP = new Map<string, Promise<void>>()

/** A refresh of one job; one at a time per job. A call made while one runs waits for it (the caller then sees its result), it does not start another. */
const refreshJob = async ($: any, job: MwJobsJob): Promise<void> => {
  const running = inFlightP.get(job.id)
  if (running !== undefined) {
    await running.catch(() => undefined)
    return
  }
  const p = refreshJobRun($, job)
  inFlightP.set(job.id, p)
  try {
    await p
    // the fanfare button's state: read with every poll, so a change made elsewhere (mw user set) shows
    await loadFanfare($, job).catch(() => null)
  } finally {
    if (inFlightP.get(job.id) === p) inFlightP.delete(job.id)
  }
}

const refreshJobRun = async ($: any, job: MwJobsJob): Promise<void> => {
  if (inFlight.has(job.id)) return
  inFlight.add(job.id)
  await setBusy($, job.id, true)
  try {
    const home: string = (await homeOf($))
    const cwd = expand(job.board, home)
    let lastWhy = ''
    const run = async (argv: string[]): Promise<unknown> => {
      try {
        const r = await proc($, argv, { cwd, timeoutMs: 20_000 })
        if (r.exitCode !== 0) {
          lastWhy = await whyFailed($, cwd, { stderr: String(r.stderr ?? '') })
          return undefined
        }
        return parseJson(r.stdout)
      } catch (err) {
        lastWhy = await whyFailed($, cwd, { err })
        return undefined
      }
    }
    // the one-line description comes from `mw jobs show ID -j` (cached); the registry's own field is the fallback
    const js = (await run(['mw', 'jobs', 'show', job.id, '-j'])) as { description?: unknown; job?: { description?: unknown } } | undefined
    // the news model (releases folder, then the draft): when it is there, it is the one source of the jobs pane
    const mentry = await loadModels($, job, js && !Array.isArray(js) ? (js as Record<string, unknown>) : undefined)
    const useModel = mentry?.draft !== undefined
    const modelStale = useModel && mentry?.stale === true
    const [wv, lv, ev] = useModel
      ? [undefined, undefined, undefined]
      : await Promise.all([run(['mw', 'wave', 'list', '-A', '-j']), run(['mw', 'list', '-A', '-j', '-v']), run(['mw', 'log', '-A', '-n', '40', '-j'])])
    const d = js && !Array.isArray(js) ? (js.description ?? js.job?.description) : undefined
    if (typeof d === 'string') await update($, jobDesc, (m: Record<string, string>) => ({ ...m, [job.id]: d.trim() }))
    // open asks come from `mw review -j` (needs); a failed run keeps the last known asks
    // `mw review -j` reads one tenant (the default one when bare): run it once per job tenant with -t and merge, keeping the tenant on each ask
    // an old mw reads one tenant per call: at most 10 tenants per poll, round-robin, the rest keep their last known asks
    const rr = rrSlice(job.tenants, tenantCursor.get(job.id) ?? 0, REVIEW_PER_POLL)
    tenantCursor.set(job.id, rr.cursor)
    const rvRuns = useModel
      ? []
      : await Promise.all(rr.slice.map(async t => ({ t, v: (await run(['mw', 'review', '-j', '-t', t])) as { needs?: unknown } | undefined })))
    const rvOk = rvRuns.filter(r => r.v && !Array.isArray(r.v) && Array.isArray(r.v.needs))
    const rv = rvOk.length === 0 ? undefined : { needs: rvOk.flatMap(r => (r.v!.needs as Record<string, unknown>[]).map(n => ({ ...n, tenant: str(n.tenant) || r.t }))) }
    const asksNow: MwJobsAsk[] | undefined =
      rv && !Array.isArray(rv) && Array.isArray(rv.needs)
        ? (rv.needs as Record<string, unknown>[])
            .map(n => ({ tenant: str(n.tenant), item: str(n.item), question: str(n.question), by: str(n.by), at: str(n.at) }))
            .filter(a => job.tenants.includes(a.tenant))
        : undefined
    // the tenants not polled this time keep their last asks

    // the job's metrics.json: the registry key first, then `metrics` from mw jobs show; $.fs only, a plain file only
    const fromMw = js && !Array.isArray(js) && typeof (js as { metrics?: unknown }).metrics === 'string' ? ((js as { metrics?: string }).metrics ?? '') : ''
    const metricsRel = job.metrics || fromMw
    const mres = metricsRel === '' ? undefined : await readMetrics($, expand(metricsRel, home, cwd))
    const now: number = await $.clock.now()
    const snaps: Record<string, MwJobsSnapshot> = await read($, snapshots)
    const old = snaps[job.id] as MwJobsSnapshot | undefined
    // a job with no tenants set does not grab every tenant (E1)
    const inJob = (tenant: string) => job.tenants.includes(tenant)
    const waves = asWaves(wv)?.filter(w => inJob(w.id.split(':')[0] ?? ''))
    const items = asItems(lv)?.filter(i => inJob(i.tenant))
    const events = asEvents(ev)?.filter(e => inJob(e.tenant)).slice(0, 15)
    const isOk = useModel ? !modelStale : (waves !== undefined && items !== undefined && events !== undefined)
    // a job that had a model never falls back to the old lists: the old snapshot stays, marked stale
    const lostModel = !useModel && old?.model !== undefined
    const hasNothing = lostModel || (!useModel && waves === undefined && items === undefined && events === undefined)

    if (hasNothing) {
      await update($, staleSince, (s: Record<string, number>) => ({ ...s, [job.id]: s[job.id] ?? now }))
      jobWhy.set(job.id, lastWhy || (lostModel ? 'the news model is gone' : ''))
      await logFail($, job.id, 'mw unavailable', `every mw command failed (stale)${lastWhy ? `: ${lastWhy}` : ''}`)
      await renderStatus($)
      return
    }
    const next: MwJobsSnapshot = useModel
      ? snapshotFromModel(mentry!.draft!, { inChat: old?.inChat, metrics: mres?.metrics, metricsNotice: mres?.notice, now })
      : {
      waves: waves ?? old?.waves ?? [],
      items: items ?? old?.items ?? [],
      events: events ?? old?.events ?? [],
      metrics: mres?.metrics,
      metricsNotice: mres?.notice,
      asks: asksNow === undefined ? (old?.asks ?? []) : [...(old?.asks ?? []).filter(a => !rr.slice.includes(a.tenant) && job.tenants.includes(a.tenant)), ...asksNow],
      updatedAt: waves !== undefined && items !== undefined ? now : (old?.updatedAt ?? now),
    }
    if (old !== undefined) await update($, previous, (p: Record<string, MwJobsSnapshot>) => ({ ...p, [job.id]: old }))
    await update($, snapshots, (s: Record<string, MwJobsSnapshot>) => ({ ...s, [job.id]: next }))
    await update($, staleSince, (s: Record<string, number>) => {
      const { [job.id]: was, ...rest } = s
      return isOk ? rest : { ...rest, [job.id]: was ?? now }
    })
    await applyChat($, job.id, now)
    await renderStatus($)
    await syncDecisions($, job, cwd)
    const cursors: Record<string, MwJobsSeen> = await read($, seen)
    if (cursors[job.id] === undefined) {
      await update($, seen, (m: Record<string, MwJobsSeen>) => ({ ...m, [job.id]: { ts: now, metrics: metricsBase(next) } }))
      await saveSeen($)
    }

    const modelMode = next.model !== undefined
    const relEv = mentry !== undefined ? await releaseEvents($, job, mentry) : []
    const baseline = modelMode ? lastDrafts.get(job.id) : undefined
    if (modelMode && !modelStale) lastDrafts.set(job.id, mentry!.draft!)
    if ((old !== undefined || modelMode) && !modelStale) {
      const events: AlertEvent[] = []
      if (modelMode) {
        // from a news model: a new release, metric drops, and the "now" events (needs, review, blocked, done, milestones, new) against the last poll; first sight is silent
        events.push(...relEv, ...(old?.model !== undefined ? metricAlerts(job.id, old.metrics, next.metrics) : []), ...(baseline !== undefined ? modelEvents(job, baseline, mentry!.draft!) : []))
      } else if (old !== undefined) {
        events.push(...legacyEvents(job, old, next), ...relEv)
      }
      const isMuted: boolean = await read($, muted)
      // one alert per change, whichever pane, timer or reload saw it: a per-job list of already alerted lines, kept in $.store
      // another session may have alerted since: the shared store is read again right before the claim (the store has no atomic compare-and-set, so this is as close as the API allows)
      let stored: string[] = []
      try {
        const all = (await $.store.get('alerted')) as Record<string, string[]> | undefined
        stored = Array.isArray(all?.[job.id]) ? all![job.id]! : []
      } catch {
        stored = []
      }
      const doneKeys: string[] = [...new Set([...(((await read($, alerted)) as Record<string, string[]>)[job.id] ?? []), ...stored])]
      const keyOf = (x: AlertEvent) => x.key ?? x.text
      const fresh = events.filter(x => !doneKeys.includes(keyOf(x)))
      if (fresh.length > 0) {
        const nextKeys = capAlerted(doneKeys, fresh.map(keyOf))
        await update($, alerted, (m: Record<string, string[]>) => ({ ...m, [job.id]: nextKeys }))
        try {
          const all = ((await $.store.get('alerted')) as Record<string, string[]> | undefined) ?? {}
          await $.store.set('alerted', { ...all, ...((await read($, alerted)) as Record<string, string[]>) })
        } catch {
          // store unavailable
        }
      }
      // the mw keys: alerts.kinds picks the kinds, alerts.sound and alerts.message act separately; alerts.enabled=false (mute) silences all of it
      const sw0 = await alertSwitches($)
      const kept = fresh.filter(x => alertAllowed(sw0, x.kind))
      const plan = planAlerts(kept, { enabled: !isMuted, message: sw0.message, sound: sw0.sound }, job.alerts)
      // every raised alert is a row at the top of both panes (and one toast bubble per event), until dismissed; the sound is the one asset; no system notification
      if (plan.lines.length > 0) await addAlertRows($, job, kept)
      if (plan.sound) {
        try {
          await $.audio.play({ asset: 'assets/alert.wav' })
        } catch {
          // no player: skip
        }
      }
      // alerts.log: one line per raised alert, how it was delivered
      const nowIso = new Date(await $.clock.now()).toISOString()
      const toasts = toastsOn({ enabled: !isMuted, message: sw0.message, popup: sw0.popup })
      const delivered = isMuted ? 'muted' : sw0.message ? (toasts ? (plan.sound ? 'row+toast+sound' : 'row+toast') : plan.sound ? 'row+sound' : 'row') : plan.sound ? 'sound' : 'none'
      await appendLog($, job.id, 'alerts.log', kept.map(x => alertLine(nowIso, x.type, x.who ?? '-', x.text, delivered)))
      if (toasts) for (const b of toastBubbles(plan.lines)) $.ui.toast(b, { timeoutMs: 8000 })
    }
  } catch {
    // never throw out of a timer
  } finally {
    inFlight.delete(job.id)
    await setBusy($, job.id, false)
    if (forceAfter.delete(job.id)) void refreshJob($, job)
  }
}

let newsOpen = false

export const JOBS_LABEL = '< Jobs'
export const NEWS_LABEL = 'News >'
const drawFailed = new Set<string>()

export const ALERT_ROWS_MAX = 20
export const ALERT_ROWS_SHOWN = 5
let alertSeq = 0

/** Adds raised alerts as pane rows (newest first, at most ALERT_ROWS_MAX kept) and saves them, so a reload still shows them. */
export const addAlertRows = async ($: any, job: MwJobsJob, events: Array<{ type: string; text: string; ref?: string; link?: MwJobsAlertRow['link'] }>, at?: number) => {
  if (events.length === 0) return
  const now: number = at ?? (await $.clock.now())
  const rows: MwJobsAlertRow[] = events.map(x => ({ id: `${now}-${(alertSeq += 1)}`, at: now, job: job.id, type: x.type, text: x.text, ...(x.link !== undefined ? { ref: x.ref, link: x.link } : {}) })).reverse()
  await update($, alertRows, (l: MwJobsAlertRow[]) => [...rows, ...l].slice(0, ALERT_ROWS_MAX))
  await saveAlertRows($)
}
const saveAlertRows = async ($: any) => {
  try {
    await $.store.set('alertRows', await read($, alertRows))
  } catch {
    // store unavailable
  }
}
const dismissAlert = async ($: any, id: string | null) => {
  await update($, alertRows, (l: MwJobsAlertRow[]) => (id === null ? [] : l.filter(r => r.id !== id)))
  await saveAlertRows($)
}
export const parseAlertRows = (v: unknown): MwJobsAlertRow[] =>
  (Array.isArray(v) ? v : [])
    .filter((r): r is MwJobsAlertRow => r !== null && typeof r === 'object' && typeof r.id === 'string' && typeof r.text === 'string' && typeof r.at === 'number')
    .map(r => ({ id: r.id, at: r.at, job: String(r.job ?? ''), type: String(r.type ?? ''), text: r.text, ...(r.link !== undefined && r.link !== null && typeof r.link.id === 'string' && (r.link.kind === 'item' || r.link.kind === 'wave') ? { ref: typeof r.ref === 'string' ? r.ref : undefined, link: { kind: r.link.kind, tenant: String(r.link.tenant ?? ''), id: r.link.id } } : {}) }))
    .slice(0, ALERT_ROWS_MAX)

/** The alert rows above a pane's own drawing: one row each (time, text, x), newest first; past ALERT_ROWS_SHOWN a "+N more · Clear all" line. */
/** An alert row's text; its id (item or milestone) is a link to the detail panel. */
const alertText2 = (ui: any, $: any, r: MwJobsAlertRow) => {
  const { Box, Text, Button } = ui
  const bare = (t: string) => (t.includes('\u00b7') ? t.slice(t.indexOf('\u00b7') + 1) : t)
  const tok = r.link === undefined || r.ref === undefined ? undefined : [r.ref, bare(r.ref)].find(t => t !== '' && r.text.indexOf(t, Math.max(0, r.text.indexOf('\u25b8'))) >= 0)
  if (tok === undefined) return <Text key={`alert-text-${r.id}`}>{r.text}</Text>
  const at = r.text.indexOf(tok, Math.max(0, r.text.indexOf('\u25b8')))
  const link = r.link!
  return (
    <Box key={`alert-text-${r.id}`}>
      <Text>{r.text.slice(0, at)}</Text>
      <Button key={`alert-id-${r.id}`} plain label={tok} onPress={() => void openDetail($, { kind: link.kind, jobId: r.job, tenant: link.tenant, id: link.id }, 'jobs')} />
      <Text>{r.text.slice(at + tok.length)}</Text>
    </Box>
  )
}

const alertBox = ($: any, e: any, rows: MwJobsAlertRow[], pane: 'jobs' | 'news', fill = false) => {
  if (rows.length === 0) return null
  const { Box, Text, Button } = $.ui.resolve(e) as any
  const shown = fill ? rows : rows.slice(0, ALERT_ROWS_SHOWN)
  return (
    <Box key="alerts" flexDirection="column" borderStyle="round" borderColor="warning" paddingX={1}>
      {shown.map(r => (
        <Box key={`alert-${r.id}`} gap={1}>
          <Text color="warning">{'\u25cf'}</Text>
          <Text dimColor>{hhmm(r.at)}</Text>
          <Box flexGrow={1} flexShrink={1}>
            {alertText2({ Box, Text, Button }, $, r)}
          </Box>
          <Button key={`alert-x-${r.id}`} plain dimColor label="x" onPress={() => void dismissAlert($, r.id)} />
        </Box>
      ))}
      {rows.length > 1 ? (
        <Box key="alerts-tail" gap={1}>
          {rows.length > shown.length ? <Button key="alert-more" plain dimColor label={`+${rows.length - shown.length} more`} onPress={() => void setFill($, pane, true)} /> : null}
          <Button key="alert-clear" plain dimColor label="Clear all" onPress={() => void dismissAlert($, null)} />
        </Box>
      ) : null}
    </Box>
  )
}

/** A pane hook that throws leaves the host's "has not drawn in this pane" placeholder: the pane draws a short fallback instead
 * (its title, the way to the other pane, Loading… and the reason), and the reason is logged once per pane and message. */
/** The fill state guardDraw read just before the pane draws (drawing reads it here, with no extra await). */
const drawFill = { jobs: false, news: false }
const guardDraw = async (pane: 'jobs' | 'news' | 'detail' | 'settings', $: any, e: any, draw: () => Promise<any>) => {
  // the jobs and news panes carry the alert rows on top, whatever they draw below (a fallback included)
  const rows: MwJobsAlertRow[] = pane === 'jobs' || pane === 'news' ? await read($, alertRows) : []
  const fillOn: boolean = pane === 'jobs' || pane === 'news' ? ((await read($, blocksFill)) as { jobs: boolean; news: boolean })[pane] : false
  if (pane === 'jobs' || pane === 'news') drawFill[pane] = fillOn
  const withAlerts = (tree: any) => {
    const box = alertBox($, e, rows, pane === 'news' ? 'news' : 'jobs', fillOn)
    if (box === null) return tree
    const { Box } = $.ui.resolve(e) as any
    return (
      <Box flexDirection="column">
        {box}
        {tree}
      </Box>
    )
  }
  try {
    return withAlerts(await draw())
  } catch (err) {
    const why = String((err as Error)?.stack ?? (err as Error)?.message ?? err).split('\n').slice(0, 3).join(' ').slice(0, 300)
    if (!drawFailed.has(`${pane}|${why}`)) {
      drawFailed.add(`${pane}|${why}`)
      try {
        $.ui.log(`Jobs: the ${pane} pane could not draw: ${why}`)
      } catch {
        // no log
      }
    }
    return withAlerts(fallbackTree($, e, pane, why))
  }
}

const fallbackTree = ($: any, e: any, pane: 'jobs' | 'news' | 'detail' | 'settings', why: string) => {
  const { Box, Text, Button } = $.ui.resolve(e) as any
  return (
    <Box flexDirection="column">
      <Box alignItems="center" gap={2} flexWrap="wrap">
        <Text bold>{pane === 'jobs' ? 'mw jobs' : pane === 'news' ? 'mw news' : pane === 'settings' ? 'mw settings' : 'mw detail'}</Text>
        <Text key="sep-nav" dimColor>{'\u00a6'}</Text>
        {pane === 'detail'
          ? <Button key="detail-back" plain label={BACK_LABEL} onPress={() => void detailBack($)} />
          : pane === 'news' || pane === 'settings'
          ? <Button key="back-jobs" plain label={JOBS_LABEL} onPress={() => void backToJobs($, '')} />
          : <Button key="news" plain label={NEWS_LABEL} onPress={() => void (async () => { const reg: MwJobsRegistry = await read($, registry); const sel: string = await read($, selected); const id = reg.jobs.some(j => j.id === sel) ? sel : (reg.jobs[0]?.id ?? ''); if (id !== '') await openNews($, id) })()} />}
      </Box>
      <Text dimColor>Loading…</Text>
      <Text color="warning">{`could not draw: ${why.slice(0, 200)}`}</Text>
    </Box>
  )
}

/** A control with a hover-reveal tooltip card one row below it (display none until the pointer is over the keyed Box). */
const tipped = (ui: any, key: string, node: any, text: string, side: 'left' | 'right' = 'left') => {
  const { Box, Text } = ui
  const place = side === 'left' ? { left: 0 } : { right: 0 }
  return (
    <Box key={`tip-${key}`}>
      {node}
      <Box position="absolute" top={1} {...place} display="none" hover={{ display: 'flex' }} backgroundColor="text" paddingX={1}>
        <Text color="inverseText">{text}</Text>
      </Box>
    </Box>
  )
}

const setFill = async ($: any, pane: 'jobs' | 'news', on: boolean) => {
  await update($, blocksFill, (f: { jobs: boolean; news: boolean }) => ({ ...f, [pane]: on }))
  try {
    await $.store.set('blocksFill', await read($, blocksFill))
  } catch {
    // store unavailable
  }
  await cfgWrite($, { 'pane.fit': on ? 'fill' : '80' })
  await reloadNewsOut($)
  // every style: the lists come from mw again, with --full when fill is on
  await refreshAll($)
}

// ---- gopher + fill: every row (`mw news --full`); an older mw keeps the caps and says so ----
export const FULL_HINT = 'Full lists need mw 0.3.4+ (mw news --full); showing the capped lists.'
const fullUnsupported = new Set<string>()
/** mw's complaint about a flag it does not have (an older mw): the only failure that marks --full unsupported. */
export const unknownOption = (stderr: string): boolean => /unknown (flag|option)|unrecognized (flag|option)|flag provided but not defined|bad flag/i.test(stderr)
/** Whether either pane wants the job's lists uncapped: its fill (↔) is on, in any style. */
const wantsFull = async ($: any): Promise<boolean> => {
  const f = (await read($, blocksFill)) as { jobs: boolean; news: boolean }
  return f.jobs || f.news
}

type FillIn = { pane: 'jobs' | 'news'; on: boolean; blocks: boolean; gopher?: boolean }

const sizeButtons = (ui: any, $: any, minus: string, hash: string, plus: string, off?: string, fill?: FillIn) => {
  const { Button } = ui
  // a style that cannot use size keeps the group (the row does not jump), dim, with the reason as the tooltip, and the press does nothing
  const press = (d: number) => () => {
    if (off === undefined) void stepBy($, d)
  }
  return [
    tipped(ui, 'smaller', <Button key="smaller" plain label={minus} dimColor onPress={press(-1)} />, off ?? 'Smaller text'),
    tipped(ui, 'reset', <Button key="reset" plain label={hash} dimColor onPress={press(0)} />, off ?? 'Reset size'),
    tipped(ui, 'bigger', <Button key="bigger" plain label={plus} dimColor onPress={press(1)} />, off ?? 'Larger text'),
    fill === undefined
      ? null
      : tipped(
          ui,
          'fill',
          <Button key="fill" plain label={'\u2194'} variant={fill.on ? 'primary' : undefined} dimColor={!fill.on} onPress={() => void setFill($, fill.pane, !fill.on)} />,
          fill.on ? 'Back to the capped lists and 80 columns' : 'Fill: every list in full (no "+N more") and the pane width (up to 160)',
        ),
  ]
}

/** A set of related controls drawn as one unit: gap 0, a dim `|` between them, never split across a wrapped line. */
const group = (ui: any, key: string, nodes: any[], trail = false) => {
  const { Box, Text } = ui
  const kids = nodes.filter(n => n !== null && n !== undefined && n !== false)
  if (kids.length === 0) return null
  return (
    <Box key={key} gap={0} flexShrink={0}>
      {kids.flatMap((n, i) => (i === 0 ? [n] : [<Text key={`sep-${key}-${i}`} dimColor>|</Text>, n]))}
      {trail && <Text key={`trail-${key}`} dimColor>{' \u00a6'}</Text>}
    </Box>
  )
}

/** The size control as one group: − | # | + */
const sizeGroup = (ui: any, $: any, off?: string, fill?: FillIn) => group(ui, 'size-group', sizeButtons(ui, $, '−', '#', '+', off, fill), true)

export const STYLES = ['normal', 'blocks', 'gopher', 'agile', 'kanban'] as const
const STYLE_LABEL: Record<string, string> = { normal: 'N', blocks: '░', gopher: '=', agile: 'A', kanban: 'K' }
const STYLE_TIP: Record<string, string> = { normal: 'Normal', blocks: 'Blocks', gopher: 'Gopher', agile: 'Agile (sprint + backlog)', kanban: 'Kanban (columns)' }

/** The style control as one group: N | ░ | = | J (normal, blocks, gopher, board); the active one is highlighted. Off the Svg surfaces normal shows as blocks. */
const styleGroup = (ui: any, $: any, pane: 'jobs' | 'news', wanted: string, canSvg: boolean, unavailable?: Partial<Record<string, string>>) => {
  const { Button } = ui
  const active = wanted === 'normal' && !canSvg ? 'blocks' : wanted
  return group(
    ui,
    'style-group',
    STYLES.map(st =>
      tipped(ui, `style-${st}`, <Button key={`style-${st}`} plain label={` ${STYLE_LABEL[st]} `} variant={st === active ? 'primary' : undefined} dimColor={st !== active || unavailable?.[st] !== undefined} onPress={() => void setBarStyle($, st, pane)} />, unavailable?.[st] !== undefined ? `${STYLE_TIP[st]} \u2014 ${unavailable[st]}` : st === 'normal' && !canSvg ? 'Normal (drawn as blocks here)' : STYLE_TIP[st]!),
    ),
    true,
  )
}

// ---- fanfare: reads `mw fanfare -j` and `mw user show -j` (the per-user value wins); writes only `mw user set`, never the board ----

type FanfareButton = { label: string; tip: string; on: boolean; onPress: () => void; onStop?: () => void }

/** The fanfare state from mw's two json answers, or null when either lacks the fanfare keys (an older mw). */
export const parseFanfare = (fanText: string | undefined, userText: string | undefined): MwJobsFanfare | null => {
  const fan = fanText === undefined ? undefined : parseJson(fanText)
  const usr = userText === undefined ? undefined : parseJson(userText)
  if (fan === null || typeof fan !== 'object' || Array.isArray(fan) || usr === null || typeof usr !== 'object' || Array.isArray(usr)) return null
  const f = fan as Record<string, unknown>
  const u = usr as Record<string, unknown>
  const sets = f.sets
  if (sets === null || typeof sets !== 'object' || Array.isArray(sets)) return null
  // mw prints fanfare_enabled / fanfare_use; the dotted spelling is the `mw user set` key
  const hasKey = (k: string) => k in u || k.replace('_', '.') in u
  if (!hasKey('fanfare_enabled') && !hasKey('fanfare_use')) return null
  const pick = (k: string) => (u[k] !== undefined && u[k] !== null ? u[k] : u[k.replace('_', '.')])
  const boardEnabled = typeof f.enabled === 'boolean' ? f.enabled : (f.config as { enabled?: unknown } | undefined)?.enabled === true
  const ue = pick('fanfare_enabled')
  const uu = pick('fanfare_use')
  return {
    enabled: typeof ue === 'boolean' ? ue : boardEnabled,
    use: typeof uu === 'string' && uu !== '' ? uu : str(f.use) || 'all',
    sets: Object.keys(sets as object),
    ...(f.playing_finale === true ? { playingFinale: true } : {}),
  }
}

/** The Stop button shows only while mw says a finale plays (mw 0.3.6+); a missing key (older mw) hides it. */
export const stopVisible = (f: MwJobsFanfare | null | undefined): boolean => f?.playingFinale === true

/** The 1-based place of the set in use in [all, ...sets]; a stale name counts as all (1); 0 when off. */
export const fanfarePos = (f: MwJobsFanfare): number => {
  if (!f.enabled) return 0
  const i = f.sets.indexOf(f.use)
  return i < 0 ? 1 : i + 2
}

export const fanfareLabel = (f: MwJobsFanfare) => (f.enabled ? `F:${fanfarePos(f)}` : 'F:off')

export const fanfareTip = (f: MwJobsFanfare) => {
  const n = f.sets.length + 1
  if (!f.enabled) return `Fanfare: off (${n === 1 ? '1 setting' : `${n} settings`}) — click to turn on`
  const p = fanfarePos(f)
  return `Fanfare: ${p === 1 ? 'all sets' : `set ${f.use}`} (${p} of ${n}) — click for next`
}

/** One click: off -> 1 (all) -> 2 (first set) -> ... -> last -> off, as `mw user set` arguments. */
export const fanfareNextArgs = (f: MwJobsFanfare): string[] => {
  if (!f.enabled) return ['fanfare.enabled=true', 'fanfare.use=all']
  const p = fanfarePos(f)
  const next = f.sets[p - 1]
  return next === undefined ? ['fanfare.enabled=false'] : [`fanfare.use=${next}`]
}

const fanfareBusy = new Set<string>()

const loadFanfare = async ($: any, job: MwJobsJob): Promise<MwJobsFanfare | null> => {
  const home: string = (await homeOf($))
  const cwd = expand(job.board, home)
  const out = async (argv: string[]) => {
    try {
      const r = await proc($, argv, { cwd, timeoutMs: 20_000 })
      return r.exitCode === 0 ? String(r.stdout) : undefined
    } catch {
      return undefined
    }
  }
  const [f, u] = await Promise.all([out(['mw', 'fanfare', '-j']), out(['mw', 'user', 'show', '-j'])])
  const next = parseFanfare(f, u)
  await update($, fanfare, (m: Record<string, MwJobsFanfare | null>) => ({ ...m, [job.id]: next }))
  return next
}

/** A click: the next setting is written to the per-user config only, then mw is read again so the label is what mw now says. */
const fanfareCycle = async ($: any, job: MwJobsJob) => {
  if (fanfareBusy.has(job.id)) return
  fanfareBusy.add(job.id)
  try {
    const cur = ((await read($, fanfare)) as Record<string, MwJobsFanfare | null>)[job.id] ?? (await loadFanfare($, job))
    if (!cur) return
    const home: string = (await homeOf($))
    let ok = false
    try {
      // the newer mw: `mw configure` (the same keys); an older one: `mw user set`
      const r = await proc($, ['mw', ...(cfgOk === true ? ['configure'] : ['user', 'set']), ...fanfareNextArgs(cur)], { cwd: expand(job.board, home), timeoutMs: 20_000 })
      ok = r.exitCode === 0
    } catch {
      ok = false
    }
    const now = await loadFanfare($, job)
    if (!ok) $.ui.toast('Jobs: could not change the fanfare setting.', { timeoutMs: 6000 })
    else if (now) $.ui.toast(`Jobs: ${fanfareTip(now).replace(/ \u2014 .*$/, '')}`, { timeoutMs: 4000 })
  } finally {
    fanfareBusy.delete(job.id)
  }
}

/** The Stop button: `mw fanfare stop` in the job's board dir, then the fanfare state is read again so the button goes away. */
const fanfareStop = async ($: any, job: MwJobsJob) => {
  if (fanfareBusy.has(job.id)) return
  fanfareBusy.add(job.id)
  try {
    const home: string = (await homeOf($))
    let ok = false
    try {
      const r = await proc($, ['mw', 'fanfare', 'stop'], { cwd: expand(job.board, home), timeoutMs: 20_000 })
      ok = r.exitCode === 0
    } catch {
      ok = false
    }
    await loadFanfare($, job)
    $.ui.toast(ok ? 'Jobs: fanfare stopped.' : 'Jobs: could not stop the fanfare.', { timeoutMs: ok ? 4000 : 6000 })
  } finally {
    fanfareBusy.delete(job.id)
  }
}

/** The pane's fanfare button for this job, or undefined (hidden) while unknown or when this mw has no fanfare keys. */
const fanfareButton = async ($: any, job: MwJobsJob): Promise<FanfareButton | undefined> => {
  const all = (await read($, fanfare)) as Record<string, MwJobsFanfare | null>
  const f = all[job.id]
  if (!f) return undefined
  return { label: fanfareLabel(f), tip: fanfareTip(f), on: f.enabled, onPress: () => void fanfareCycle($, job), ...(stopVisible(f) ? { onStop: () => void fanfareStop($, job) } : {}) }
}

// =====================================================================================
// SETTINGS: the per-user mw keys. Read `mw user get -j`; write `mw configure KEY=VALUE…` (validates, never prompts).
// The panes' own controls write back, so the terminal (`mw configure`) and the panes stay in sync; $.store is only a cache.
// =====================================================================================

export const SETTINGS_PANE = 'mw-settings'
export const CFG_OLD_HINT = 'Settings need a newer mw (mw user get -j, mw configure); using the settings saved in this app.'
export type CfgKey = MwJobsCfgKey
export type CfgState = MwJobsSettings

/** `mw user get -j` as keys, or undefined when it is not that JSON (an older mw). */
export const parseUserKeys = (text: string | undefined): CfgKey[] | undefined => {
  const v = text === undefined ? undefined : parseJson(text)
  const keys = v !== null && typeof v === 'object' && !Array.isArray(v) ? (v as { keys?: unknown }).keys : undefined
  if (!Array.isArray(keys)) return undefined
  return keys
    .filter((k): k is Record<string, unknown> => k !== null && typeof k === 'object' && typeof (k as { key?: unknown }).key === 'string')
    .map(k => ({ key: str(k.key), value: str(k.value), default: str(k.default), source: str(k.source) || 'default', group: str(k.group) || 'other', choices: Array.isArray(k.choices) ? k.choices.map(str) : [] }))
}

/** The next value of a cycle: the choice after the current one (the first when the current one is not a choice). */
export const cycleNext = (choices: string[], cur: string): string => choices[(choices.indexOf(cur) + 1) % choices.length] ?? cur

/** alerts.kinds: a comma list, or `all`; one kind toggled, written back as `all` when every kind is on. */
export const kindsOf = (value: string, choices: string[]): string[] => (value.trim() === 'all' ? [...choices] : value.trim() === 'none' ? [] : value.split(',').map(s => s.trim()).filter(s => choices.length === 0 || choices.includes(s)))
export const kindsToggle = (value: string, choices: string[], kind: string): string => {
  const on = new Set(kindsOf(value, choices))
  if (on.has(kind)) on.delete(kind)
  else on.add(kind)
  const list = choices.filter(c => on.has(c))
  return list.length === choices.length ? 'all' : list.length === 0 ? 'none' : list.join(',')
}

/** Which alerts.kinds an alert belongs to: the structured kind it was raised with ('' for none). */
export const alertKind = (x: { kind?: string }): string => x.kind ?? ''
/** Whether a kind may alert: no list (older mw), no kind, a kind mw does not know yet (`new` until mw lists it), or one that is on. */
export const alertAllowed = (sw: { kinds: Set<string> | null; choices: string[] }, kind: string): boolean =>
  sw.kinds === null || kind === '' || (sw.choices.length > 0 && !sw.choices.includes(kind)) || sw.kinds.has(kind)

/** The key values last applied to the panes: a key is applied again only when its value changes (so a pane's own choice stays). */
const cfgApplied = new Map<string, string>()
let cfgOk: boolean | null = null

const mwHome = async ($: any, args: string[]) => {
  const home: string = (await homeOf($))
  try {
    return (await proc($, ['mw', ...args], { cwd: home || '/', timeoutMs: 20_000 })) as { exitCode: number; stdout: unknown; stderr: unknown }
  } catch (err) {
    return { exitCode: -1, stdout: '', stderr: String((err as Error)?.message ?? err) }
  }
}

/** A key's value as mw last said, or undefined (no key, or an older mw). */
const cfgGet = async ($: any, key: string): Promise<string | undefined> => {
  const s: CfgState = await read($, settings)
  return s.ok === true ? s.keys.find(k => k.key === key)?.value : undefined
}

/** Reads `mw user get -j`; false (and the hint) when this mw has no such command. With apply, changed keys reach the panes. */
const loadSettings = async ($: any, apply = true): Promise<CfgKey[] | undefined> => {
  const r = await mwHome($, ['user', 'get', '-j'])
  const keys = r.exitCode === 0 ? parseUserKeys(String(r.stdout)) : undefined
  cfgOk = keys !== undefined
  await update($, settings, (s: CfgState) => (keys === undefined ? { ...s, ok: false, keys: [], hint: CFG_OLD_HINT } : { ...s, ok: true, keys, hint: '' }))
  // only on a change: an update made while a pane draws is denied
  const popNow = popupOf({ ok: keys !== undefined, keys: keys ?? [] })
  if ((await read($, popupAtom)) !== popNow) await update($, popupAtom, () => popNow)
  if (keys !== undefined && apply) await applySettings($, keys)
  return keys
}

/** The keys that changed since last applied, into the panes' state (no write back). */
const applySettings = async ($: any, keys: CfgKey[]) => {
  let rearm = false
  let relayout = false
  for (const k of keys) {
    if (cfgApplied.get(k.key) === k.value) continue
    cfgApplied.set(k.key, k.value)
    const v = k.value
    if (k.key === 'pane.refresh' && EVERY_ALL.includes(v)) {
      await update($, refreshEvery, () => v)
      await cacheSet($, 'refreshEvery', v)
      rearm = true
    } else if (k.key === 'pane.style' && (STYLES as readonly string[]).includes(v)) {
      await update($, barStyle, () => v)
      await update($, newsStyle, () => v)
      await cacheSet($, 'barStyle', v)
      await cacheSet($, 'newsStyle', v)
      relayout = true
    } else if (k.key === 'pane.zoom' && Number.isFinite(Number(v))) {
      const n = Math.max(-STEP_MAX, Math.min(STEP_MAX, Math.round(Number(v))))
      await update($, fontStep, () => n)
      await cacheSet($, 'fontStep', n)
      relayout = true
    } else if (k.key === 'pane.fit' && (v === 'fill' || v === '80')) {
      await update($, blocksFill, () => ({ jobs: v === 'fill', news: v === 'fill' }))
      await cacheSet($, 'blocksFill', { jobs: v === 'fill', news: v === 'fill' })
      relayout = true
    } else if (k.key === 'alerts.enabled' && (v === 'true' || v === 'false')) {
      await update($, muted, () => v === 'false')
      await cacheSet($, 'muted', v === 'false')
    }
  }
  if (rearm) await arm($)
  if (relayout) await reloadNewsOut($)
}

const cacheSet = async ($: any, key: string, v: unknown) => {
  try {
    await $.store.set(key, v)
  } catch {
    // store unavailable
  }
}

/**
 * Writes keys with `mw configure KEY=VALUE…` (all or nothing) when this mw has it, then reads them back so every pane follows.
 * A bad value leaves mw's stderr, one line, on each key's row. With an older mw nothing is written (the store keeps the value).
 */
const cfgWrite = async ($: any, kv: Record<string, string>, fromPane = true): Promise<boolean> => {
  if (cfgOk !== true) return false
  const keys = Object.keys(kv)
  if (keys.length === 0) return true
  // a pane control already shows its change: mark it applied so the read back does not move the other pane;
  // a Settings row is applied to both panes by the read back
  if (fromPane) for (const k of keys) cfgApplied.set(k, kv[k]!)
  const r = await mwHome($, ['configure', ...keys.map(k => `${k}=${kv[k]}`)])
  const ok = r.exitCode === 0
  const why = ok ? '' : (`${String(r.stderr)}\n${String(r.stdout)}`.trim().split('\n')[0] ?? '').slice(0, 160) || `mw configure failed (exit ${r.exitCode})`
  await update($, settings, (s: CfgState) => {
    const err = { ...s.err }
    for (const k of keys) {
      if (ok) delete err[k]
      else err[k] = why
    }
    return { ...s, err }
  })
  if (!ok) for (const k of keys) cfgApplied.delete(k)
  await loadSettings($)
  return ok
}

/** One time: the settings this app saved before the keys existed go to mw (only keys still at mw's default), then the store is a cache. */
const migrateStore = async ($: any, keys: CfgKey[]): Promise<boolean> => {
  let done = false
  try {
    done = (await $.store.get('cfgMigrated')) === true
  } catch {
    return false
  }
  if (done) return false
  const want: Record<string, string> = {}
  const put = (key: string, v: string | undefined) => {
    const k = keys.find(x => x.key === key)
    if (k === undefined || v === undefined || k.source !== 'default' || v === k.value) return
    if (k.choices.length > 0 && !k.choices.includes(v)) return
    want[key] = v
  }
  const g = async (k: string) => {
    try {
      return await $.store.get(k)
    } catch {
      return undefined
    }
  }
  const every = await g('refreshEvery')
  put('pane.refresh', typeof every === 'string' ? every : undefined)
  const style = await g('barStyle')
  put('pane.style', typeof style === 'string' ? style : undefined)
  const step = await g('fontStep')
  put('pane.zoom', typeof step === 'number' ? String(step) : undefined)
  const fill = await g('blocksFill')
  put('pane.fit', fill !== null && typeof fill === 'object' ? ((fill as { jobs?: unknown }).jobs === true ? 'fill' : '80') : undefined)
  const mute = await g('muted')
  put('alerts.enabled', typeof mute === 'boolean' ? String(!mute) : undefined)
  let wrote = false
  if (Object.keys(want).length > 0) {
    const r = await mwHome($, ['configure', ...Object.entries(want).map(([k, v]) => `${k}=${v}`)])
    wrote = r.exitCode === 0
    if (!wrote) return false
  }
  await cacheSet($, 'cfgMigrated', true)
  return wrote
}

/** At start and on reload: the keys (migrating the old store once), applied in full. */
const initSettings = async ($: any) => {
  cfgApplied.clear()
  const keys = await loadSettings($, false)
  if (keys === undefined) return
  if (await migrateStore($, keys)) {
    await loadSettings($)
    return
  }
  await applySettings($, keys)
}

/** The alert switches: kinds (null: all), message rows/toasts/notifications, sound. */
const alertSwitches = async ($: any): Promise<{ kinds: Set<string> | null; choices: string[]; message: boolean; popup: boolean; sound: boolean }> => {
  const s: CfgState = await read($, settings)
  if (s.ok !== true) return { kinds: null, choices: [], message: true, popup: true, sound: true }
  const k = s.keys.find(x => x.key === 'alerts.kinds')
  return {
    kinds: k === undefined ? null : new Set(kindsOf(k.value, k.choices)),
    choices: k?.choices ?? [],
    message: s.keys.find(x => x.key === 'alerts.message')?.value !== 'false',
    popup: s.keys.find(x => x.key === 'alerts.popup')?.value !== 'false',
    sound: s.keys.find(x => x.key === 'alerts.sound')?.value !== 'false',
  }
}

const openSettings = async ($: any) => {
  await loadSettings($)
  const reg: MwJobsRegistry = await read($, registry)
  const sel: string = await read($, selected)
  const job = reg.jobs.find(j => j.id === sel) ?? reg.jobs[0]
  if (job) await loadFanfare($, job)
  await openChecked($, { id: SETTINGS_PANE, title: 'mw settings', focus: true }, 'settings', '/mw-configure')
}

const GROUP_TITLE: Record<string, string> = { pane: 'Panes', alerts: 'Alerts', fanfare: 'Fanfare', status: 'Chat bar status', discover: 'Board discovery' }
const KEY_LABEL: Record<string, string> = {
  'pane.refresh': 'Refresh every',
  'pane.style': 'Style',
  'pane.zoom': 'Zoom',
  'pane.fit': 'Fit',
  'pane.open': 'Opens first',
  'alerts.enabled': 'Alerts',
  'alerts.kinds': 'Kinds',
  'alerts.message': 'Message row',
  'alerts.popup': 'Popup',
  'alerts.sound': 'Sound',
  'fanfare.enabled': 'Fanfare',
  'fanfare.use': 'Set',
  'status.show': 'Show',
  'status.detail': 'Detail',
  'discover.home': 'Scan home',
  'discover.depth': 'Scan depth',
}

/** The Settings pane: a section per group (mw's order), a row per key: label, value, a cycle button; kinds toggle one by one. */
const settingsTree = (ui: any, $: any, s: CfgState, fan: MwJobsFanfare | null | undefined, job: MwJobsJob | undefined) => {
  const { Box, Text, Button } = ui
  const head = (
    <Box key="settings-title" alignItems="center" gap={2} flexWrap="wrap">
      <Text bold>mw settings</Text>
      <Text key="sep-nav" dimColor>{'¦'}</Text>
      <Button key="settings-back" plain label={JOBS_LABEL} onPress={() => void backToJobs($, '')} />
    </Box>
  )
  if (s.ok !== true) {
    return (
      <Box flexDirection="column">
        {head}
        <Box key="settings-hint">
          <Text dimColor>{s.ok === null ? 'Loading…' : s.hint || CFG_OLD_HINT}</Text>
        </Box>
      </Box>
    )
  }
  const groups = [...new Set(s.keys.map(k => k.group))]
  const row = (k: CfgKey) => {
    const label = KEY_LABEL[k.key] ?? k.key
    const err = s.err[k.key]
    let control: any = null
    let value = k.value === '' ? '(none)' : k.value
    if (k.key === 'alerts.kinds') {
      const on = new Set(kindsOf(k.value, k.choices))
      value = on.size === k.choices.length ? 'all' : on.size === 0 ? 'none' : [...on].join(', ')
      control = (
        <Box key="kinds" gap={1} flexWrap="wrap">
          {k.choices.map(c => (
            <Button key={`cfg-kind-${c}`} plain dimColor={!on.has(c)} variant={on.has(c) ? 'primary' : undefined} label={on.has(c) ? c : `·${c}`} onPress={() => void cfgWrite($, { [k.key]: kindsToggle(k.value, k.choices, c) }, false)} />
          ))}
        </Box>
      )
    } else if (k.key === 'fanfare.use') {
      const choices = ['all', ...(fan?.sets ?? [])]
      control = choices.length > 1 ? <Button key="cfg-fanfare.use" plain label="Next" onPress={() => void cfgWrite($, { [k.key]: cycleNext(choices, k.value || 'all') }, false)} /> : null
    } else if (k.choices.length > 0) {
      control = <Button key={`cfg-${k.key}`} plain label="Next" onPress={() => void cfgWrite($, { [k.key]: cycleNext(k.choices, k.value) }, false)} />
    }
    return (
      <Box key={`cfg-row-${k.key}`} flexDirection="column">
        <Box gap={1}>
          <Box width={16} flexShrink={0}>
            <Text>{label}</Text>
          </Box>
          <Box key={`cfg-val-${k.key}`} width={22} flexShrink={0}>
            <Text bold={k.source === 'user'} dimColor={k.source !== 'user'}>{value}</Text>
          </Box>
          {control}
        </Box>
        {err ? (
          <Box key={`cfg-err-${k.key}`}>
            <Text color="error">{err}</Text>
          </Box>
        ) : null}
      </Box>
    )
  }
  return (
    <Box flexDirection="column">
      {head}
      <Text key="settings-sub" dimColor>Changes apply at once; the same keys as mw configure in a terminal. Bold: set by you.</Text>
      {groups.map(g => (
        <Box key={`cfg-group-${g}`} flexDirection="column" marginTop={1}>
          <Box gap={2}>
            <Text bold color="suggestion">{(GROUP_TITLE[g] ?? g).toUpperCase()}</Text>
            {g === 'fanfare' && job ? <Button key="cfg-fanfare-play" plain label={'▶ Play'} onPress={() => void runMw($, 'fanfare test')} /> : null}
          </Box>
          {s.keys.filter(k => k.group === g).map(row)}
        </Box>
      ))}
    </Box>
  )
}

type ControlsIn = {
  job: MwJobsJob
  refresh: { key: string; onPress: () => void }
  every: string
  wanted: string
  muted: boolean
  /** alerts.popup: toast popups on. */
  popup: boolean
  needs: number
  review: number
  hasPage: boolean
  canOpen: boolean
  canSvg: boolean
  pane: 'jobs' | 'news'
  /** A phone has no hover: show the legend of the icons as text. */
  legend?: boolean
  /** The fill option (↔) of this pane: blocks fills the pane width up to 160 columns. */
  fill: boolean
  /** Set (the reason) when the style cannot use size: the size group stays, dim, with this as its tooltip. */
  sizeOff?: string
  /** Styles that cannot be drawn now, with the reason (the button is dim and the tooltip says why). */
  unavailable?: Partial<Record<string, string>>
  /** The fanfare button (F:off, F:1 …), absent when this mw has no fanfare keys. */
  fanfare?: FanfareButton
  /** The Scan button (jobs found by `mw boards`, not the registry): discover the boards again. */
  scan?: { tip: string; onPress: () => void }
}

/**
 * The controls row both panes share, in groups (the row wraps between groups, never inside one):
 * attention !N | ◐N, refresh ↻ | ⏱, view style | − | # | +, alerts, open elsewhere ⧉ | ↗.
 */
const controlsRow = (ui: any, $: any, c: ControlsIn) => {
  const { Box, Text } = ui
  const row = controlsRowInner(ui, $, c)
  // no hover on a phone: the same controls named in plain words under the row (W5)
  if (!c.legend) return row
  return (
    <Box key="controls-legend" flexDirection="column">
      {row}
      <Text dimColor wrap="wrap">{'\u21bb refresh \u00b7 \u23f1 auto refresh \u00b7 N normal \u00b7 \u2591 blocks \u00b7 = gopher \u00b7 A agile \u00b7 K kanban \u00b7 \u2212 # + text size \u00b7 \u2194 fill width \u00b7 \u29c9 open in editor \u00b7 \u2197 news page'}</Text>
    </Box>
  )
}

const controlsRowInner = (ui: any, $: any, c: ControlsIn) => {
  const { Box, Text, Button, Link } = ui
  return (
    <Box key="controls" alignItems="center" gap={3} flexWrap="wrap">
      {group(ui, 'grp-attention', askIcons(ui, $, c.job, c.needs, c.review), true)}
      {group(ui, 'grp-refresh', [
        tipped(ui, c.refresh.key, <Button key={c.refresh.key} plain label="↻" onPress={c.refresh.onPress} />, 'Refresh now'),
        tipped(ui, 'every', <Button key="every" plain label={`⏱ ${c.every === 'off' ? '–' : c.every}`} dimColor onPress={() => void cycleEvery($)} />, `Auto refresh: ${c.every} (off/30s/1m/5m, click to cycle)`),
        ...(c.scan !== undefined ? [tipped(ui, 'scan', <Button key="scan" plain label="Scan" dimColor onPress={c.scan.onPress} />, c.scan.tip)] : []),
      ])}
      {styleGroup(ui, $, c.pane, c.wanted, c.canSvg, c.unavailable)}
      {sizeGroup(ui, $, c.sizeOff, { pane: c.pane, on: c.fill, blocks: true, gopher: c.wanted === 'gopher' })}
      {group(ui, 'grp-alerts', [tipped(ui, 'mute', <Button key="mute" plain label={c.muted ? '🔕' : '🔔'} dimColor onPress={() => void toggleMute($)} />, c.muted ? 'Unmute alerts' : 'Mute alerts', 'right'), tipped(ui, 'popup', <Button key="popup" plain label={popupButton(c.popup, c.muted).label} dimColor={popupButton(c.popup, c.muted).dim} onPress={() => void togglePopup($)} />, popupButton(c.popup, c.muted).tip, 'right')], true)}
      {c.fanfare !== undefined ? group(ui, 'grp-fanfare', [tipped(ui, 'fanfare', <Button key="fanfare" plain label={c.fanfare.label} dimColor={!c.fanfare.on} onPress={c.fanfare.onPress} />, c.fanfare.tip, 'right'), ...(c.fanfare.onStop !== undefined ? [tipped(ui, 'fanfare-stop', <Button key="fanfare-stop" plain label="Stop" onPress={c.fanfare.onStop} />, 'Stop the finale that is playing', 'right')] : [])], true) : null}
      {group(ui, 'grp-open', [
        c.canOpen ? tipped(ui, 'panel', <Button key="panel" plain label="⧉" onPress={() => void openPanel($, c.job)} />, 'Open the news file in the editor', 'right') : null,
        c.hasPage ? tipped(ui, 'page', <Link href={c.job.news}><Text>{'\u2197'}</Text></Link>, 'Open news page', 'right') : null,
      ])}
    </Box>
  )
}

const mainTitle = async ($: any) => {
  const sel: string = await read($, selected)
  return sel !== '' ? `mw jobs · ${sel}` : 'mw jobs'
}


/** Reads <board>/docs/news/status.md with $.fs only (never a script), capped at 9,500 characters (the Markdown element's own limit is 10,000). */
const loadNews = async ($: any, job: MwJobsJob) => {
  const home: string = (await homeOf($))
  let text: string | null = null
  let truncated = false
  try {
    const raw: string = await $.fs.read(`${expand(job.board, home)}/docs/news/status.md`)
    truncated = raw.length > NEWS_MAX
    text = truncated ? raw.slice(0, NEWS_MAX) : raw
  } catch {
    text = null
  }
  await update($, news, (m: Record<string, { text: string | null; truncated: boolean; hash: string }>) => ({ ...m, [job.id]: { text, truncated, hash: text === null ? '' : newsHash(text) } }))
}

/** mw's own render of the news, run on open, refresh and timer only; the result (or the failure) is cached per job + kind + columns. */
let lastPaneW = 640
let lastBodyCols: number | undefined
let lastTerminal = false
const outInFlight = new Set<string>()
const outKey = (id: string, kind: 'text' | 'md', cols: number) => (kind === 'md' ? `${id}|md` : `${id}|text|${cols}`)

const runNewsOut = async ($: any, job: MwJobsJob, kind: 'text' | 'md', cols: number) => {
  const key = outKey(job.id, kind, cols)
  if (outInFlight.has(key)) return
  outInFlight.add(key)
  try {
    const home: string = (await homeOf($))
    const argv = kind === 'text' ? ['mw', 'news', '--job', job.id, '--format', 'text', '--width', String(cols)] : ['mw', 'news', '--job', job.id, '--format', 'md', '-o', '-']
    let ok = false
    let text = ''
    try {
      const r = await proc($, argv, { cwd: expand(job.board, home), timeoutMs: 30_000 })
      ok = r.exitCode === 0 && String(r.stdout).trim() !== ''
      text = ok ? String(r.stdout) : ''
    } catch {
      ok = false
    }
    await update($, newsOut, (m: Record<string, { ok: boolean; text: string }>) => ({ ...m, [key]: { ok, text } }))
  } catch {
    // never throw out of a render-time load
  } finally {
    outInFlight.delete(key)
  }
}

/** What the news pane draws now: the kind and columns follow the style, the surface, the step and the last pane width. */
const newsWant = async ($: any): Promise<{ kind: 'text' | 'md'; cols: number }> => {
  const style: string = await read($, newsStyle)
  const step: number = await read($, fontStep)
  const fill = ((await read($, blocksFill)) as { jobs: boolean; news: boolean }).news
  void style
  void step
  void fill
  return { kind: 'md', cols: 0 }
}

const loadNewsOut = async ($: any, job: MwJobsJob) => {
  const w = await newsWant($)
  await runNewsOut($, job, w.kind, w.cols)
}


// ---- the news model (docs/specs/news-model.md): releases folder, draft, navigation ----

type NewsEntry = { full?: boolean; draft?: Model; draftError?: string; stale?: boolean; tdraft?: { slug: string; model: Model }; releases: Record<number, Model>; bad: Record<number, string>; nums: number[]; latest: number; dir: string }
type NewsViewState = { page?: number; list?: boolean; listPage?: number; seen?: number; confirm?: boolean }

const metricsOfModel = (m: unknown): { metrics?: MwMetrics; notice?: string } => parseMetrics(JSON.stringify(m))

/** RELEASE_DIR: the job key news_releases, else <dir of news_out>/releases (mw jobs show -j; news.out is a folder or a file in one). */
export const releasesDirOf = (js: Record<string, unknown> | undefined, board: string, home = ''): string => {
  const rel = js && (js['news.releases'] ?? js.news_releases)
  if (typeof rel === 'string' && rel !== '') return expand(rel, home, board)
  const out = js && (js['news.out'] ?? js.news_out)
  let dir = 'docs/news'
  if (typeof out === 'string' && out !== '') {
    const trimmed = out.replace(/\/+$/, '')
    dir = out.endsWith('/') || !/\.[A-Za-z0-9]+$/.test(trimmed.split('/').pop() ?? '') ? trimmed : trimmed.split('/').slice(0, -1).join('/') || '.'
  }
  return expand(`${dir}/releases`, home, board)
}

const pagesOf = (e: NewsEntry): number[] => [...e.nums, ...(e.draft ? [0] : [])]

/** The page shown: the reader's own, else the newest one (the draft, or the latest release). Always one that exists. */
export const currentPage = (e: NewsEntry, v: NewsViewState | undefined): number => {
  const pages = pagesOf(e)
  const want = v?.page
  if (want !== undefined && pages.includes(want)) return want
  return pages.length === 0 ? 0 : pages[pages.length - 1]!
}

const setNewsView = async ($: any, id: string, patch: NewsViewState) => {
  await update($, newsView, (m: Record<string, NewsViewState>) => ({ ...m, [id]: { ...(m[id] ?? {}), ...patch } }))
  try {
    const all = (await read($, newsView)) as Record<string, NewsViewState>
    await $.store.set('relSeen', Object.fromEntries(Object.entries(all).filter(([, v]) => v.seen !== undefined).map(([k, v]) => [k, v.seen])))
    await $.store.set('newsPage', Object.fromEntries(Object.entries(all).filter(([, v]) => v.page !== undefined).map(([k, v]) => [k, v.page])))
  } catch {
    // store unavailable
  }
}

const readRelease = async ($: any, entry: NewsEntry, n: number): Promise<Model | undefined> => {
  try {
    const res = parseModel(String(await $.fs.read(`${entry.dir}/${n}.json`)), metricsOfModel)
    return res.model
  } catch {
    return undefined
  }
}

/** Why an mw run failed, in words a person can act on. */
export const whyFailed = async ($: any, cwd: string, o: { err?: unknown; stderr?: string }): Promise<string> => {
  const text = o.err !== undefined ? String((o.err as Error)?.message ?? o.err) : (o.stderr ?? '')
  try {
    await $.fs.stat(cwd)
  } catch {
    return `board folder missing: ${cwd}`
  }
  if (/time(d)? ?out|timeout/i.test(text)) return 'mw timed out'
  if (/lock|busy/i.test(text)) return 'board busy, try again'
  if (o.err !== undefined && /ENOENT|not found|spawn|no such/i.test(text)) return 'mw not found'
  const first = text.split('\n').map(l => l.trim()).find(l => l !== '') ?? ''
  return first === '' ? 'mw failed' : `mw failed: ${first.slice(0, 120)}`
}

/** Why a job has no news model (the last failed json run), per job id. */
const modelWhy = new Map<string, string>()

/** The notice for a style that needs the news model: the newer-mw words, or the real reason the json run failed. */
export const noModelNotice = (style: string, why: string | undefined): string | undefined => {
  if (style !== 'gopher' && style !== 'agile' && style !== 'kanban') return undefined
  const name = style === 'gopher' ? 'Gopher' : style === 'agile' ? 'Agile' : 'Kanban'
  if (why === undefined || /format|unknown|usage|invalid|unrecognized/i.test(why)) return `${name} needs the newer mw (no news model): showing blocks`
  return `${name} needs the news model (${why}): showing blocks`
}

const modelInFlight = new Set<string>()
let lastJobShow: Record<string, Record<string, unknown> | undefined> = {}
/** Forgets the jobs-show answers (a new session start does; tests call it between sessions). */
export const lastJobShowReset = () => {
  lastJobShow = {}
}

/**
 * Loads the news model: the releases folder (`latest`, `N.json`) first, then the draft from
 * `mw news --job ID --format json`. Returns the entry, or undefined when there is no model at all (the old path stays).
 */
const loadModels = async ($: any, job: MwJobsJob, js?: Record<string, unknown>): Promise<NewsEntry | undefined> => {
  const key = job.id
  if (modelInFlight.has(key)) return ((await read($, newsModel)) as Record<string, NewsEntry>)[key]
  modelInFlight.add(key)
  modelProbed.add(key)
  try {
    const home: string = (await homeOf($))
    const cwd = expand(job.board, home)
    if (js !== undefined) lastJobShow[key] = js
    const dir = releasesDirOf(lastJobShow[key], cwd, home)
    const old = ((await read($, newsModel)) as Record<string, NewsEntry>)[key]
    const entry: NewsEntry = { releases: { ...(old?.releases ?? {}) }, bad: {}, nums: [], latest: 0, dir }
    try {
      const txt = String(await $.fs.read(`${dir}/latest`))
      entry.latest = Number(/\d+/.exec(txt)?.[0] ?? 0) || 0
    } catch {
      entry.latest = 0
    }
    try {
      const names = (await $.fs.list(dir)) as Array<{ name: string }>
      entry.nums = names.map(n => /^(\d+)\.json$/.exec(n.name)?.[1]).filter((x): x is string => x !== undefined).map(Number).sort((a, b) => a - b)
    } catch {
      entry.nums = []
    }
    if (entry.latest > 0 && !entry.nums.includes(entry.latest)) entry.nums.push(entry.latest)
    entry.nums.sort((a, b) => a - b)
    entry.latest = Math.max(entry.latest, entry.nums[entry.nums.length - 1] ?? 0)
    try {
      const full = (await wantsFull($)) && !fullUnsupported.has(job.id)
      let r = await proc($, ['mw', 'news', '--job', job.id, '--format', 'json', ...(full ? ['--full'] : [])], { cwd, timeoutMs: 30_000 })
      if (full && r.exitCode !== 0 && unknownOption(String(r.stderr ?? ''))) {
        // an mw before --full says so (the option is unknown): the capped lists, and the pane says why. Any other failure is not that.
        fullUnsupported.add(job.id)
        r = await proc($, ['mw', 'news', '--job', job.id, '--format', 'json'], { cwd, timeoutMs: 30_000 })
      }
      entry.full = full && !fullUnsupported.has(job.id) && r.exitCode === 0
      if (r.exitCode === 0) {
        const res = parseModel(String(r.stdout), metricsOfModel, { full: entry.full })
        if (res.model) entry.draft = res.model
        else if (res.error) entry.draftError = res.error
      } else {
        entry.draftError = await whyFailed($, cwd, { stderr: String(r.stderr ?? '') })
      }
    } catch (err) {
      entry.draftError = await whyFailed($, cwd, { err })
    }
    // a chosen tenant: the model filtered by mw (`--tenant`), one more run for this job only; an older mw is filtered here instead
    const chosenTenant = (((await read($, tenantSel)) as Record<string, string>)[job.id] ?? '') as string
    if (chosenTenant !== '' && entry.draft !== undefined) {
      try {
        const r = await proc($, ['mw', 'news', '--job', job.id, '--format', 'json', ...(entry.full === true ? ['--full'] : []), '--tenant', chosenTenant], { cwd, timeoutMs: 30_000 })
        const res = r.exitCode === 0 ? parseModel(String(r.stdout), metricsOfModel, { full: entry.full === true }) : {}
        if (res.model) entry.tdraft = { slug: chosenTenant, model: { ...res.model, only: chosenTenant } }
      } catch {
        // filtered here
      }
    }
    // a failed poll keeps the last model, marked stale: never a fall back to the old path, never a diff across the two kinds
    if (entry.draft === undefined && old?.draft !== undefined) {
      entry.draft = old.draft
      entry.stale = true
    }
    if (entry.draft === undefined && entry.nums.length === 0) {
      if (entry.draftError !== undefined) modelWhy.set(key, entry.draftError)
      else modelWhy.delete(key)
      await update($, newsModel, (m: Record<string, NewsEntry>) => {
        const { [key]: _gone, ...rest } = m
        return rest
      })
      return undefined
    }
    // the newest release is kept ready (the title row and the alert read it)
    if (entry.latest > 0 && entry.releases[entry.latest] === undefined) {
      const m = await readRelease($, entry, entry.latest)
      if (m) entry.releases[entry.latest] = m
    }
    await update($, newsModel, (m: Record<string, NewsEntry>) => ({ ...m, [key]: entry }))
    const v = ((await read($, newsView)) as Record<string, NewsViewState>)[key]
    if (v?.seen === undefined) await setNewsView($, key, { seen: entry.latest })
    return entry
  } catch {
    return undefined
  } finally {
    modelInFlight.delete(key)
  }
}

/** Reads releases the reader needs (the page, the one before it, a list page) into the cache. */
const ensureReleases = async ($: any, id: string, nums: number[]) => {
  const entry = ((await read($, newsModel)) as Record<string, NewsEntry>)[id]
  if (entry === undefined) return
  const need = [...new Set(nums)].filter(n => n > 0 && entry.releases[n] === undefined && entry.bad[n] === undefined && entry.nums.includes(n))
  if (need.length === 0) return
  const got: Record<number, Model> = {}
  const bad: Record<number, string> = {}
  for (const n of need) {
    const m = await readRelease($, entry, n)
    if (m) got[n] = m
    else bad[n] = 'unreadable'
  }
  await update($, newsModel, (all: Record<string, NewsEntry>) => {
    const cur = all[id]
    return cur === undefined ? all : { ...all, [id]: { ...cur, releases: { ...cur.releases, ...got }, bad: { ...cur.bad, ...bad } } }
  })
}

const goPage = async ($: any, id: string, page: number) => {
  const entry = ((await read($, newsModel)) as Record<string, NewsEntry>)[id]
  const atNewest = entry !== undefined && (page === 0 || page >= entry.latest)
  await setNewsView($, id, { page, list: false, confirm: false, ...(atNewest ? { seen: entry!.latest } : {}) })
}

/** Publish button, second press: `mw news --job ID --release`. "nothing new since issue N" is shown as a toast. */
const releasingNow = new Set<string>()
const doRelease = async ($: any, job: MwJobsJob) => {
  // one release at a time per job: a second press while it runs does nothing
  if (releasingNow.has(job.id)) return
  releasingNow.add(job.id)
  await update($, releasing, (l: string[]) => [...l, job.id])
  try {
    await doReleaseRun($, job)
  } finally {
    releasingNow.delete(job.id)
    await update($, releasing, (l: string[]) => l.filter(x => x !== job.id))
  }
}

const doReleaseRun = async ($: any, job: MwJobsJob) => {
  await setNewsView($, job.id, { confirm: false })
  const home: string = (await homeOf($))
  let out = ''
  let code = 1
  try {
    const r = await proc($, ['mw', 'news', '--job', job.id, '--release'], { cwd: expand(job.board, home), timeoutMs: 60_000 })
    code = r.exitCode
    out = `${String(r.stdout)}\n${String(r.stderr)}`.trim()
  } catch (err) {
    out = String((err as Error)?.message ?? err)
  }
  const nothing = /nothing new since release \d+/i.exec(out)
  if (nothing) {
    $.ui.toast(`Jobs: ${nothing[0].replace(/release/i, 'issue')}`, { timeoutMs: 6000 })
    return
  }
  if (code !== 0) {
    $.ui.toast(`Jobs: publish failed${out ? ` — ${out.split('\n')[0]!.slice(0, 120)}` : ''}`, { timeoutMs: 8000 })
    return
  }
  // queue the refresh behind the one in flight (never clear its guard)
  if (inFlight.has(job.id)) forceAfter.add(job.id)
  else await refreshJob($, job)
}

/** The alert for a new release: one alertNew per job and release, baseline on first sight. Draft changes raise none. */
const releaseEvents = async ($: any, job: MwJobsJob, entry: NewsEntry): Promise<AlertEvent[]> => {
  const known = ((await read($, relKnown)) as Record<string, number>)[job.id]
  if (known !== undefined && known >= entry.latest) return []
  await update($, relKnown, (m: Record<string, number>) => ({ ...m, [job.id]: entry.latest }))
  try {
    await $.store.set('relKnown', await read($, relKnown))
  } catch {
    // store unavailable
  }
  if (known === undefined || entry.latest === 0) return []
  return [{ type: 'new', kind: 'release', text: alertText(job.id, `Issue ${entry.latest} published`, '', ''), who: `release ${entry.latest}`, key: `release|${entry.latest}` }]
}

// ---- the news pane from the news model: navigation, the release list, and the page in the chosen style ----

const releaseLoading = new Set<string>()
const modelProbed = new Set<string>()

/** The metric view model of a model page: deltas come from the model (change since the previous release). */
const modelMetricViews = (m: Model): MetricSectionView[] => {
  if (m.metrics === null || 'error' in m.metrics) return []
  const base: Record<string, number[]> = {}
  for (const sec of m.metrics.sections)
    for (const r of sec.rows) {
      const d = m.deltas[rowKey(sec, r)]
      if (d === undefined) continue
      const p = primary(sec, r)
      base[rowKey(sec, r)] = sec.kind === 'split' ? [p[0]! - (d.good ?? 0), p[1]! - (d.bad ?? 0)] : sec.kind === 'progress' ? [p[0]! - (d.done ?? 0), 0] : [p[0]! - (d.value ?? 0), 0]
    }
  return metricViews(m.metrics, base).map(sec => ({ ...sec, rows: sec.rows.slice(0, capN(sec.rows.length, METRIC_ROWS_MAX)), more: Math.max(0, sec.rows.length - capN(sec.rows.length, METRIC_ROWS_MAX)) }))
}

const pageDate = (m: Model | undefined) => (m ? releaseDate(m) : '')

/** "Issue N · date · vs issue N-1" or "Draft · since issue M". A news release is an issue of the bulletin in the panes; mw's own CLI keeps --release. */
export const pageTitle = (entry: NewsEntry, cur: number): { title: string; marker: string; ref?: number } => {
  if (cur === 0) {
    const m = entry.draft
    const since = m?.release.since.number ?? (entry.latest > 0 ? entry.latest : null)
    return { ref: since ?? undefined, title: since === null ? 'Draft · since the start' : `Draft · since issue ${since}`, marker: m ? `Draft, not published · ${changeCount(m)} changes since ${since === null ? 'the start' : `issue ${since}`}` : 'Draft, not published' }
  }
  const m = entry.releases[cur]
  // the release's own `since` names the one before it (releases may have gaps); without it, the one numbered before
  const since = m?.release.since.number
  const none = since === 0 || (since === undefined && cur <= 1) || (since === null && cur <= 1)
  const prev = none ? 'vs the start' : `vs issue ${since ?? cur - 1}`
  return { title: `Issue ${cur} · ${pageDate(m) || '…'} · ${prev}`, marker: '', ref: none ? undefined : (since ?? cur - 1) }
}

/** ⇤ | ◂ | ☰ | ▸ | ⇥ as one group (a dot after ☰ and ⇥ when a release came that the reader has not seen), and the Release button on the draft only. */
const releaseNav = (ui: any, $: any, job: MwJobsJob, entry: NewsEntry, view: NewsViewState, cur: number, isReleasing = false) => {
  const { Box, Text, Button } = ui
  const pages = pagesOf(entry)
  const idx = pages.indexOf(cur)
  const prev = idx > 0 ? pages[idx - 1] : undefined
  const next = idx >= 0 && idx < pages.length - 1 ? pages[idx + 1] : undefined
  const dot = entry.latest > (view.seen ?? entry.latest)
  const go = (p: number | undefined) => () => {
    if (p !== undefined) void goPage($, job.id, p)
  }
  const nav = group(ui, 'nav-group', [
    tipped(ui, 'nav-first', <Button key="nav-first" plain label="⇤" dimColor={idx <= 0} onPress={go(pages[0])} />, 'First issue'),
    tipped(ui, 'nav-prev', <Button key="nav-prev" plain label="◂" dimColor={prev === undefined} onPress={go(prev)} />, 'Previous issue'),
    tipped(ui, 'nav-list', <Button key="nav-list" plain label={dot ? '☰●' : '☰'} onPress={() => void (async () => { await setNewsView($, job.id, { list: !view.list, listPage: 0, confirm: false, ...(view.list ? {} : { seen: entry.latest }) }) })()} />, dot ? 'All issues (a new one)' : 'All issues'),
    tipped(ui, 'nav-next', <Button key="nav-next" plain label="▸" dimColor={next === undefined} onPress={go(next)} />, 'Next issue'),
    tipped(ui, 'nav-last', <Button key="nav-last" plain label={dot ? '⇥●' : '⇥'} dimColor={idx === pages.length - 1 && !dot} onPress={go(pages[pages.length - 1])} />, dot ? 'Latest issue (a new one)' : 'Latest issue'),
  ])
  const release =
    cur !== 0
      ? null
      : isReleasing
        ? tipped(ui, 'release', <Button key="release" plain dimColor label="publishing…" />, 'An issue is being published')
        : view.confirm === true
        ? (
            <Box key="release-confirm" gap={1} alignItems="center">
              <Text>{`Publish issue ${entry.latest + 1}?`}</Text>
              <Button key="release-yes" plain variant="primary" label="Yes, publish" onPress={() => void doRelease($, job)} />
              <Button key="release-no" plain dimColor label="Cancel" onPress={() => { releaseArmed.delete(job.id); void setNewsView($, job.id, { confirm: false }) }} />
            </Box>
          )
        : entry.draft !== undefined && changeCount(entry.draft) === 0
          ? tipped(ui, 'release', <Button key="release" plain dimColor label="Publish" onPress={() => $.ui.toast(`Jobs: nothing new since issue ${entry.latest}`, { timeoutMs: 6000 })} />, `Nothing new since issue ${entry.latest}`)
          : tipped(ui, 'release', <Button key="release" plain variant="primary" label="Publish" onPress={() => void setNewsView($, job.id, { confirm: true })} />, `Publish issue ${entry.latest + 1} from the draft; asks first`)
  return (
    <Box key="release-nav" alignItems="center" gap={2} flexWrap="wrap">
      {nav}
      {release}
    </Box>
  )
}

const LIST_PAGE = 20

/** The release list: newest first, number, date and a one-line headline, 20 to a page; a click opens that release. */
const releaseList = (ui: any, $: any, job: MwJobsJob, entry: NewsEntry, view: NewsViewState) => {
  const { Box, Text, Button } = ui
  const nums = [...entry.nums].sort((a, b) => b - a)
  const pages = Math.max(1, Math.ceil(nums.length / LIST_PAGE))
  const lp = Math.max(0, Math.min(pages - 1, view.listPage ?? 0))
  const slice = nums.slice(lp * LIST_PAGE, (lp + 1) * LIST_PAGE)
  const rows: any[] = []
  if (lp === 0 && entry.draft) {
    const t = pageTitle(entry, 0)
    rows.push(<Button key="list-draft" plain label={`Draft · ${t.marker.replace('Draft, not released · ', '')}`} variant="primary" onPress={() => void goPage($, job.id, 0)} />)
  }
  for (const n of slice) {
    const m = entry.releases[n]
    rows.push(<Button key={`list-${n}`} plain label={`${n} · ${pageDate(m) || '…'} · ${m ? relHeadline(m) : entry.bad[n] ? entry.bad[n] : 'loading…'}`} onPress={() => void goPage($, job.id, n)} />)
  }
  return (
    <Box key="release-list" flexDirection="column" gap={0}>
      <Box gap={2} alignItems="center">
        <Text bold>Issues, newest first</Text>
        <Text dimColor>{`page ${lp + 1}/${pages}`}</Text>
        {lp > 0 && <Button key="list-newer" plain dimColor label="◂ newer" onPress={() => void setNewsView($, job.id, { listPage: lp - 1 })} />}
        {lp < pages - 1 && <Button key="list-older" plain dimColor label="older ▸" onPress={() => void setNewsView($, job.id, { listPage: lp + 1 })} />}
        <Button key="list-close" plain dimColor label="✕" onPress={() => void setNewsView($, job.id, { list: false })} />
      </Box>
      {rows}
    </Box>
  )
}

/** One release-notes row in the board style: key (accent; the tracker label as a link), title, state lozenge on the right. */
const boardNoteRow = (ui: any, k: string, r: ItemRow, open?: () => void) => {
  const { Box, Text } = ui
  const tone = r.kind === 'needs' || r.kind === 'blocked' || r.kind === 'sent_back' ? 'error' : r.kind === 'review' ? 'warning' : r.kind === 'done' || r.kind === 'accepted' || r.kind === 'closed' ? 'success' : undefined
  return (
    <Box key={k} gap={1} alignItems="center">
      {ticketKey(ui, k, { tenant: r.tenant, id: r.id, ticket: r.ticket, ticketUrl: r.ticketUrl }, open)}
      <Box flexGrow={1} flexShrink={1}>
        {openTitle(ui, k, r.title, r.ticketUrl ?? '', open)}
      </Box>
      <Text inverse color={tone} bold>{` ${r.state.toUpperCase()} `}</Text>
    </Box>
  )
}

/** A milestone row in the board style (the jobs epic strip and the release notes draw it with this): ID title, done/total, state lozenge, bar. */
const boardMileRow = (ui: any, k: string, r: MileRow, canSvg: boolean, open?: () => void) => {
  const { Box, Text, Svg } = ui
  const tone = r.state === 'closed' ? 'success' : r.state === 'not started' ? undefined : 'warning'
  return (
    <Box key={k} gap={1} alignItems="center">
      {ticketKey(ui, k, { tenant: r.tenant, id: r.id, ticket: r.ticket, ticketUrl: r.ticketUrl }, open)}
      <Box flexGrow={1} flexShrink={1}>
        {openTitle(ui, k, r.title, r.ticketUrl ?? '', open)}
      </Box>
      {canSvg ? (() => { const eb = epicBarSvg(r.pct, 80); return <Svg key={`${k}-bar`} source={eb.source} alt={`${r.pct}%`} width={eb.width} /> })() : <Text>{textBar(r.pct, 10)}</Text>}
      <Text dimColor>{mileCount(r)}</Text>
      <Text inverse color={tone} bold>{` ${r.state.toUpperCase()} `}</Text>
    </Box>
  )
}

/** The release notes of a page in the board style: Milestones closed / Done / Sent back / Ongoing / Next / Metrics. */
const boardNotes = (ui: any, $: any, job: MwJobsJob, m: Model, labels: Record<string, string>, canSvg: boolean, views: MViewSection[], ask: (tenant: string, id: string) => void, fill = false) => {
  const { Box, Text, Button } = ui
  const capOf = (n: number) => (fill ? Number.MAX_SAFE_INTEGER : n)
  const c = m.now.counts
  const rowsOf = (kind: Parameters<typeof itemRow>[0], l: MList, top: number, counted?: number) => {
    const shown = topOf(l, capOf(top), counted)
    const rest = Math.max(total(l, counted) - shown.length, 0)
    return { rows: shown.map(i => itemRow(kind, { tenant: i.tenant, id: i.id, title: i.title, note: i.note, ticket: i.ticket, ticketUrl: i.ticket_url }, labels)), rest }
  }
  const section = (key: string, title: string, nodes: any[], rest: number) =>
    nodes.length === 0 && rest === 0 ? null : (
      <Box key={key} flexDirection="column">
        <Text bold dimColor>{title.toUpperCase()}</Text>
        {nodes}
        {rest > 0 && <Button key={`${key}-more`} plain dimColor label={`+${rest} more`} onPress={() => void setFill($, 'news', true)} />}
      </Box>
    )
  const itemSection = (key: string, title: string, kind: Parameters<typeof itemRow>[0], l: MList, top: number, counted?: number, withAsk = false) => {
    const r = rowsOf(kind, l, top, counted)
    return section(
      key,
      title,
      r.rows.map(row => (
        <Box key={`${key}-${row.full}`} gap={1} alignItems="center">
          {boardNoteRow(ui, `${key}-n-${row.full}`, row, () => void openDetail($, { kind: 'item', jobId: job.id, tenant: row.tenant, id: row.id }, 'news'))}
          {withAsk && tipped(ui, `${key}-ask-${row.full}`, <Button key={`${key}-ask-${row.full}`} plain dimColor label="ask" onPress={() => ask(row.tenant, row.id)} />, `Ask in chat: ${row.title.slice(0, 80)}`)}
        </Box>
      )),
      r.rest,
    )
  }
  const mileSection = (key: string, title: string, l: MList, top: number, closed: boolean) => {
    const shown = topOf(l, capOf(top))
    const rest = Math.max(total(l) - shown.length, 0)
    return section(key, title, shown.map(i => boardMileRow(ui, `${key}-${i.tenant}-${i.id}`, mileOf({ tenant: i.tenant, id: i.id, title: i.title, done: i.done, total: i.total, ticket: i.ticket, ticketUrl: i.ticket_url }, labels, closed), canSvg, () => void openDetail($, { kind: 'wave', jobId: job.id, tenant: i.tenant, id: i.id }, 'news'))), rest)
  }
  const draft = m.release.number === null
  const hp = m.happened
  return (
    <Box key="board-notes" flexDirection="column" gap={1}>
      <Text bold wrap="truncate-end">{`${job.title} · ${draft ? 'draft' : `issue ${m.release.number}`}`}</Text>
      <Text dimColor>{shortLines(m).join(' · ')}</Text>
      {mileSection('b-closed', 'Milestones closed', hp.closed_milestones, 10, true)}
      {itemSection('b-done', 'Done', 'done', hp.done, 10, hp.counts.done)}
      {itemSection('b-acc', 'Accepted', 'accepted', hp.accepted, 10, hp.counts.accepted)}
      {itemSection('b-sent', 'Sent back', 'sent_back', hp.sent_back, 10, hp.counts.sent_back)}
      {itemSection('b-dec', 'Decisions', 'decision', hp.decisions, 10, hp.counts.decisions)}
      {itemSection('b-ans', 'Answers', 'answer', hp.answers, 10, hp.counts.answers)}
      <Text bold>ONGOING</Text>
      <Text dimColor>{`needs ${total(m.now.needs, c.needs)} · review ${total(m.now.review, c.review)} · blocked ${total(m.now.blocked, c.blocked)} · doing ${total(m.now.doing, c.doing)}`}</Text>
      {itemSection('b-needs', 'Needs you', 'needs', m.now.needs, 5, c.needs, true)}
      {itemSection('b-review', 'In review', 'review', m.now.review, 5, c.review, true)}
      {itemSection('b-blocked', 'Blocked', 'blocked', m.now.blocked, 5, c.blocked, true)}
      {itemSection('b-doing', 'In progress', 'doing', m.now.doing, 5, c.doing)}
      <Text bold>NEXT</Text>
      {itemSection('b-ready', 'Ready', 'ready', m.next.ready, 10, c.ready)}
      {mileSection('b-next', 'Next milestones', m.next.next_milestones, 5, false)}
      <Box key="b-metrics" flexDirection="column">
        <Text bold dimColor>METRICS</Text>
        {gopherNodes({ Box, Text, Button, Svg: ui.Svg }, metricLines(views, 66, m.metrics === null ? 'no metrics file' : 'error' in m.metrics ? `metrics unavailable: ${m.metrics.error}` : undefined), { canSvg, cols: 66, size: 12, key: 'bnm', ask: () => undefined })}
      </Box>
    </Box>
  )
}

/** Everything the news pane draws below its controls when a news model is there: navigation, title row, and the page or the list. */
const modelNewsNodes = async (
  $: any,
  ui: any,
  job: MwJobsJob,
  o: { entry: NewsEntry; view: NewsViewState; canSvg: boolean; paneW: number; given: number | undefined; step: number; style: string },
): Promise<any[]> => {
  const { Box, Text, Link, Svg, Markdown, Button } = ui
  const { entry, view, canSvg } = o
  const cur = currentPage(entry, view)
  const nodes: any[] = [releaseNav(ui, $, job, entry, view, cur, ((await read($, releasing)) as string[]).includes(job.id))]
  // a release the reader was on that is no longer there; the draft that could not be read (E5), or is out of date (E6)
  if (view.page !== undefined && view.page > 0 && !pagesOf(entry).includes(view.page)) nodes.push(<Text key="page-gone" color="warning">{`issue ${view.page} gone`}</Text>)
  if (entry.draftError !== undefined && entry.draft !== undefined) nodes.push(<Text key="draft-note" color="warning">{`draft out of date: ${entry.draftError} (showing the last poll)`}</Text>)
  else if (entry.draftError !== undefined && entry.draft === undefined) nodes.push(<Text key="draft-note" color="warning">{`draft unavailable: ${entry.draftError}`}</Text>)
  const loadKey = `${job.id}|${cur}|${view.list ? view.listPage ?? 0 : 'p'}`
  if (!releaseLoading.has(loadKey)) {
    const want = view.list ? [...entry.nums].sort((a, b) => b - a).slice((view.listPage ?? 0) * LIST_PAGE, ((view.listPage ?? 0) + 1) * LIST_PAGE) : cur > 0 ? [cur, cur - 1] : [entry.latest]
    if (want.some(n => n > 0 && entry.releases[n] === undefined && entry.bad[n] === undefined && entry.nums.includes(n))) {
      releaseLoading.add(loadKey)
      void ensureReleases($, job.id, want).finally(() => releaseLoading.delete(loadKey))
    }
  }
  if (view.list) {
    nodes.push(releaseList(ui, $, job, entry, view))
    return nodes
  }
  const t = pageTitle(entry, cur)
  nodes.push(
    <Box key="page-title" flexDirection="column">
      {t.ref !== undefined && entry.nums.includes(t.ref) && t.title.endsWith(`issue ${t.ref}`) ? (
        // the issue it is compared with is a link to that issue
        <Box key="page-title-line">
          <Text bold>{t.title.slice(0, t.title.length - `issue ${t.ref}`.length)}</Text>
          <Button key="page-title-ref" plain label={`issue ${t.ref}`} onPress={() => void goPage($, job.id, t.ref!)} />
        </Box>
      ) : (
        <Text bold wrap="truncate-end">{t.title}</Text>
      )}
      {t.marker !== '' && <Text dimColor wrap="truncate-end">{t.marker}</Text>}
    </Box>,
  )
  const m = cur === 0 ? entry.draft : entry.releases[cur]
  if (m === undefined) {
    nodes.push(<Text key="page-wait" dimColor={!(cur > 0 && entry.bad[cur])} color={cur > 0 && entry.bad[cur] ? 'warning' : undefined}>{cur > 0 && entry.bad[cur] ? `issue ${cur} ${entry.bad[cur]}` : cur === 0 && entry.draftError ? `draft unavailable: ${entry.draftError}` : 'Loading…'}</Text>)
    return nodes
  }
  const labels = m.only !== undefined ? tenantLabels([m.only]) : tenantLabels(job.tenants)
  const views: MViewSection[] = modelMetricViews(m)
  const askIt = (tn: string, id: string) => void askInChat($, job, tn, id)
  const size = svgSizeFor(o.step)
  const prevAt = cur === 0 ? (entry.releases[entry.latest] ? releaseDate(entry.releases[entry.latest]!) : undefined) : entry.releases[cur - 1] ? releaseDate(entry.releases[cur - 1]!) : undefined
  if (o.style === 'gopher') {
    const cols = gopherCols(o.paneW, size, canSvg, o.given)
    if (cols < GOPHER_MIN) {
      nodes.push(<Text key="gopher-narrow" color="warning">{`too narrow for gopher (needs ${GOPHER_MIN} columns)`}</Text>)
      return nodes
    }
    const fillOn = ((await read($, blocksFill)) as { jobs: boolean; news: boolean }).news
    if (fillOn && fullUnsupported.has(job.id)) nodes.push(<Text key="full-hint" dimColor>{FULL_HINT}</Text>)
    const lines = withTenantHidden(m.only !== undefined, () => bulletin(m, { width: cols, releases: entry.nums, prevAt, url: job.news || undefined, metrics: views, full: fillOn && entry.full === true && cur === 0 }))
    nodes.push(<Box key="gopher" flexDirection="column">{gopherNodes(ui, lines, { canSvg, cols, size, key: 'bul', ask: askIt })}{idsRow(ui, $, job.id, lines, modelIdIndex(m), 'bul', 'news')}</Box>)
    return nodes
  }
  if (o.style === 'kanban' || o.style === 'agile') {
    const noCap = ((await read($, blocksFill)) as { jobs: boolean; news: boolean }).news && !fullUnsupported.has(job.id)
    const doc = o.style === 'agile' ? agileNews(m, noCap ? Number.MAX_SAFE_INTEGER : undefined) : kanbanNews(m, noCap ? Number.MAX_SAFE_INTEGER : undefined)
    const cols = o.style === 'agile' ? 1 : flowCols(o.given)
    nodes.push(withTenantHidden(m.only !== undefined, () => flowNodes(ui, $, job, doc, { cols, ageOf: () => '', keyPrefix: `n${o.style}`, ask: false, width: o.given })))
    return nodes
  }
  // normal and blocks: the newsletter as markdown through the same pipeline as mw's own news; milestone rows through the shared milestone row drawing
  const mono = o.style === 'blocks'
  const nFill = drawFill.news && !fullUnsupported.has(job.id)
  const topN = nFill ? Number.MAX_SAFE_INTEGER : undefined
  const segs = newsSegments(m, { metrics: views, labels, top: topN })
  const hasMore = !nFill && segs.some(sg => ('md' in sg ? /\+\d+ more/.test(sg.md) : sg.tail !== ''))
  const moreBtn = hasMore ? <Button key="news-more" plain dimColor label="+ more" onPress={() => void setFill($, 'news', true)} /> : null
  if (!canSvg) {
    const md = newsMd(m, { metrics: views, labels, top: topN })
    nodes.push(<Markdown key="md" text={md.length > NEWS_MAX ? `${md.slice(0, NEWS_MAX)}\n\n\u2026 truncated` : md} />)
    if (moreBtn) nodes.push(moreBtn)
    return nodes
  }
  const urls = new Map<string, string>()
  const body: any[] = []
  // columns of the cell grid: blocks takes the mono count (80, 160 with fill); normal the pane
  const gridCols = mono ? monoCols(o.paneW, size * 0.6) : Math.max(40, Math.min(100, o.given ?? 60))
  segs.forEach((sg, si) => {
    if ('miles' in sg) {
      body.push(monoRowsNodes(ui, mileMonoRows(sg.miles, mr => void openDetail($, { kind: 'wave', jobId: job.id, tenant: mr.tenant, id: mr.id }, 'news')), { key: `mile${si}`, cols: gridCols, svgBars: !mono }))
      if (sg.tail) body.push(<Text key={`miletail${si}`} dimColor>{sg.tail}</Text>)
      return
    }
    const chunks = chunkMd(parseMd(sg.md), job.news)
    const flat = chunks.flatMap(c => ('svg' in c ? c.svg : [c.link]))
    if (mono) for (const b of flat) if (b.kind === 'p' || b.kind === 'li') for (const r of b.runs) if (r.href && !urls.has(r.href)) urls.set(r.href, r.text === r.href ? shortUrl(r.href) : r.text)
    body.push(...mdNodes(ui, flat, { key: `md${si}`, mono }))
  })
  if (urls.size > 0) {
    body.push(
      <Box key="links" gap={1}>
        <Text dimColor>Links</Text>
        {[...urls].map(([href, label]) => <Link key={`lk-${href}`} href={href}>{label}</Link>)}
      </Box>,
    )
  }
  if (moreBtn) body.push(moreBtn)
  nodes.push(mono ? <Box key="grid" width={gridCols} flexDirection="column">{body}</Box> : <Box key="grid" flexDirection="column">{body}</Box>)
  return nodes
}


let lastNewsTitle = ''

const newsUnseen = async ($: any, id: string): Promise<boolean> => {
  const me = ((await read($, newsModel)) as Record<string, NewsEntry>)[id]
  if (me !== undefined && (me.draft !== undefined || me.nums.length > 0)) {
    const v = ((await read($, newsView)) as Record<string, NewsViewState>)[id]
    return me.latest > (v?.seen ?? me.latest)
  }
  const entry = ((await read($, news)) as Record<string, { text: string | null; hash: string }>)[id]
  if (entry === undefined || entry.text === null) return false
  const m: Record<string, string> = await read($, newsSeen)
  return m[id] !== entry.hash
}

const newsTitleOf = async ($: any, id: string) => `${(await newsUnseen($, id)) ? '● ' : ''}mw news · ${id}`

/** Retitle the news tab when its unseen mark changed (no focus). */
const syncNewsTitle = async ($: any, id: string) => {
  const title = await newsTitleOf($, id)
  if (title === lastNewsTitle) return
  lastNewsTitle = title
  await $.ui.open({ id: NEWS_PANE, title })
}

const markNewsSeen = async ($: any, id: string) => {
  const entry = ((await read($, news)) as Record<string, { text: string | null; hash: string }>)[id]
  if (entry === undefined || entry.text === null) return
  await update($, newsSeen, (m: Record<string, string>) => ({ ...m, [id]: entry.hash }))
  try {
    await $.store.set('newsSeen', await read($, newsSeen))
  } catch {
    // store unavailable
  }
  await syncNewsTitle($, id)
}

const openNewsTab = async ($: any, id: string, focus: boolean) => {
  const reg: MwJobsRegistry = await read($, registry)
  const job = reg.jobs.find(j => j.id === id)
  if (!job) return
  await update($, newsJob, () => id)
  if (focus) await update($, chosen, () => true)
  newsOpen = true
  await loadNews($, job)
  await loadNewsOut($, job)
  if (focus) await markNewsSeen($, id)
  const title = await newsTitleOf($, id)
  lastNewsTitle = title
  if (focus) await openChecked($, { id: NEWS_PANE, title, focus: true }, 'news', '/mw-news')
  else await $.ui.open({ id: NEWS_PANE, title })
}
const openNews = (($: any, id: string) => openNewsTab($, id, true))

/** Opens a pane and, when it is not placed, says why (toast + log) instead of failing silently. */
const openChecked = async ($: any, spec: Record<string, unknown>, what: string, hint: string) => {
  let r: { isPlaced?: boolean; reason?: string } | undefined
  try {
    r = await $.ui.open(spec)
  } catch (err) {
    r = { isPlaced: false, reason: String((err as Error)?.message ?? err) }
  }
  if (r && r.isPlaced === false) {
    await logFail($, (await read($, selected)) || 'jobs', `${what} pane not placed`, r.reason ?? 'unknown')
    const msg = `Jobs: could not show the ${what} pane (${r.reason ?? 'unknown'}). Try ${hint}.`
    try {
      $.ui.log(msg)
    } catch {
      // no log
    }
    $.ui.toast(msg, { timeoutMs: 8000 })
  }
  return r
}

const openMain = async ($: any, focus = false, first?: string) => {
  const reg: MwJobsRegistry = await read($, registry)
  const sel: string = await read($, selected)
  const job = reg.jobs.find(j => j.id === sel) ?? reg.jobs[0]
  if (job) await openNewsTab($, job.id, false)
  const spec = { id: PANE, title: await mainTitle($) }
  if (focus) await openChecked($, { ...spec, focus: true }, 'jobs', '/mw-jobs')
  else await $.ui.open(spec)
  // pane.open=news (mw key): the news pane is the one in front
  if (first === 'news' && job) await $.ui.open({ id: NEWS_PANE, title: await newsTitleOf($, job.id), focus: true })
}

/** The news file the panel button opens: `news_path` from `mw jobs show -j` (mw's own answer), else the job's `news_out` inside the board
 * (a folder holds status.md; a file is itself), else docs/news/status.md. Undefined (button hidden) for an absolute or `..` news_out, or a file that is not .md/.html. */
export const newsStatusPath = (js: Record<string, unknown> | undefined, board: string, home: string): string | undefined => {
  const base = expand(board, home)
  const okExt = (p: string) => /\.(md|html)$/i.test(p)
  const given = js && js.news_path
  // news_path present but "": mw refused the path. Absent: an older mw, fall back to news_out.
  if (typeof given === 'string' && given === '') return undefined
  if (typeof given === 'string') {
    const full = isAbs(given) ? given : `${base}/${given}`
    return /(^|[\\/])\.\.([\\/]|$)/.test(full) || !okExt(full) ? undefined : full
  }
  const out = js && (js['news.out'] ?? js.news_out)
  if (typeof out !== 'string' || out === '') return `${base}/docs/news/status.md`
  if (isAbs(out) || out.startsWith('~') || out.split(/[\\/]/).includes('..')) return undefined
  const full = `${base}/${out}`
  const last = full.replace(/[\\/]+$/, '').split(/[\\/]/).pop() ?? ''
  const path = /[\\/]$/.test(out) || !/\.[A-Za-z0-9]+$/.test(last) ? `${full.replace(/[\\/]+$/, '')}/status.md` : full
  return okExt(path) ? path : undefined
}

/** What a stat output (macOS `%HT`, Linux `%F`) says about a file: 'file', 'link' or 'other'. */
export const kindFromStat = (out: string): 'file' | 'link' | 'other' => {
  const t = out.trim().toLowerCase()
  return t === 'regular file' || t === 'regular empty file' ? 'file' : t === 'symbolic link' ? 'link' : 'other'
}

/**
 * Whether a path is a plain file that may be opened: not a symbolic link, a regular file, and (mdOnly) a .md file.
 * $.fs.stat can refuse a path outside the session's folder; then the OS says (argv only, no shell): `/usr/bin/stat -f %HT` on macOS,
 * `stat -c %F` on Linux; on Windows the kind is not checked. `why` is '' when it may be opened, 'missing' when there is no such file,
 * else one line with the real reason.
 */
export const checkPlainFile = async ($: any, path: string, mdOnly: boolean): Promise<string> => {
  let kind: 'file' | 'link' | 'other' | undefined
  let real = path
  let err = ''
  try {
    const st = await $.fs.stat(path, { resolve: true })
    real = st.realPath ?? path
    kind = st.isLink ? 'link' : st.kind === 'file' ? 'file' : 'other'
  } catch (e) {
    err = String((e as Error)?.message ?? e).split('\n')[0]!.slice(0, 160)
    if (/ENOENT|no such file/i.test(err)) return 'missing'
    const os = await osKind($)
    if (os === 'windows') {
      kind = 'file'
    } else {
      try {
        const r = await proc($, os === 'mac' ? ['/usr/bin/stat', '-f', '%HT', path] : ['stat', '-c', '%F', path], { timeoutMs: 5000 })
        if (r.exitCode === 0) kind = kindFromStat(String(r.stdout))
        else if (/no such file/i.test(String(r.stderr))) return 'missing'
        else err = `${err}; stat: ${String(r.stderr).trim().split('\n')[0]!.slice(0, 100)}`
      } catch (e2) {
        err = `${err}; stat: ${String((e2 as Error)?.message ?? e2).slice(0, 100)}`
      }
    }
  }
  if (kind === undefined) return `it cannot be checked (${err || 'no answer'})`
  if (kind === 'link') return 'it is a symbolic link'
  if (kind !== 'file') return 'it is not a file'
  if (mdOnly && (!/\.(md|html)$/i.test(path) || !/\.(md|html)$/i.test(real))) return 'it is not a .md or .html file'
  return ''
}

/** A file: Link is refused on every surface, so the panel button asks the OS to open the job's news page (no script of the repo runs). */
const openPanel = async ($: any, job: MwJobsJob) => {
  const home: string = (await homeOf($))
  let js = lastJobShow[job.id]
  if (js === undefined) {
    try {
      const r = await proc($, ['mw', 'jobs', 'show', job.id, '-j'], { cwd: expand(job.board, home), timeoutMs: 20_000 })
      const v = r.exitCode === 0 ? parseJson(String(r.stdout)) : undefined
      if (v !== null && typeof v === 'object' && !Array.isArray(v)) js = v as Record<string, unknown>
    } catch {
      js = undefined
    }
  }
  const path = newsStatusPath(js, job.board, home)
  if (path === undefined) {
    await logFail($, job.id, 'news file not opened', 'the news file is outside the board or not .md/.html')
    $.ui.toast('Jobs: not opening the news file, it is not a .md or .html file inside the board.', { timeoutMs: 6000 })
    return
  }
  const name = path.split(/[\\/]/).pop() ?? 'status.md'
  // a repo could commit the page as a link to an .app or .command: only a plain .md file is ever opened, and only in the text editor
  const why = await checkPlainFile($, path, true)
  if (why !== '') {
    const shown = why === 'missing' ? `${name} does not exist yet (${path})` : why
    await logFail($, job.id, `${name} not opened`, shown)
    $.ui.toast(`Jobs: not opening ${name}, ${shown}.`, { timeoutMs: 6000 })
    return
  }
  try {
    await proc($, openerArgv(await osKind($), path))
  } catch {
    // no opener
  }
}

/** Brings the jobs pane forward by the path the jobs pane's news button uses for news: the job's data is loaded first
 * (real async work, so the press that raised this has finished), then the pane is opened with focus. News stays open. */
// ---- the detail panel: an item or a milestone, from `mw show` / `mw wave show` -j only; nothing runs but those reads ----
export const detailKey = (r: MwJobsDetailRef) => `${r.jobId}|${r.kind}|${r.tenant}|${r.id}`
/** A wave id as the lists give it ("tenant:WAV-1") split into tenant and id. */
const splitRef = (tenant: string, id: string) => (id.includes(':') ? { tenant: id.slice(0, id.indexOf(':')), id: id.slice(id.indexOf(':') + 1) } : { tenant, id })

const loadDetail = async ($: any, r: MwJobsDetailRef) => {
  const reg: MwJobsRegistry = await read($, registry)
  const job = reg.jobs.find(j => j.id === r.jobId)
  const home: string = (await homeOf($))
  const argv = r.kind === 'item' ? ['mw', 'show', r.id, '-t', r.tenant, '-j'] : ['mw', 'wave', 'show', r.id, '-t', r.tenant, '-j']
  let next: { text?: string; why?: string }
  if (job === undefined) next = { why: `unknown job ${r.jobId}` }
  else {
    try {
      const out = await proc($, argv, { cwd: expand(job.board, home), timeoutMs: 20_000 })
      next = out.exitCode === 0 ? { text: String(out.stdout) } : { why: String(out.stderr || out.stdout || `exit ${out.exitCode}`).trim().split('\n')[0]!.slice(0, 200) }
    } catch (err) {
      next = { why: String((err as Error)?.message ?? err).slice(0, 200) }
    }
  }
  await update($, detailData, (m: Record<string, { text?: string; why?: string }>) => ({ ...m, [detailKey(r)]: next }))
}

const detailTitle = (r: MwJobsDetailRef) => `mw ${r.kind === 'wave' ? 'milestone' : 'item'} · ${r.tenant}/${r.id}`

/** A click on an item or a milestone: show it in the detail pane (from a pane: a fresh trail; from the detail pane itself: remember where to go back). */
/**
 * A bar of `total` cells: coloured parts (n cells each) then the track. On a monospace terminal it is block glyphs; anywhere else (a proportional
 * font, where a full block is wider than a light shade) it is Boxes with exact widths and a background colour, so the parts always add up to the box.
 */
const meterBar = (ui: any, key: string, parts: Array<{ n: number; color?: string }>, total: number, glyphs: boolean) => {
  const { Box, Text } = ui
  const used = Math.min(total, parts.reduce((n, p) => n + p.n, 0))
  if (glyphs) {
    return (
      <Box key={key} flexShrink={0}>
        {parts.filter(p => p.n > 0).map((p, i) => <Text key={`${key}-p${i}`} color={p.color}>{'\u2588'.repeat(p.n)}</Text>)}
        <Text dimColor>{'\u2591'.repeat(Math.max(0, total - used))}</Text>
      </Box>
    )
  }
  return (
    <Box key={key} width={total} flexShrink={0} height={1}>
      {parts.filter(p => p.n > 0).map((p, i) => <Box key={`${key}-p${i}`} width={p.n} height={1} flexShrink={0} backgroundColor={p.color ?? 'text'}><Text> </Text></Box>)}
      <Box key={`${key}-track`} width={Math.max(0, total - used)} height={1} flexShrink={0} backgroundColor="subtle"><Text> </Text></Box>
    </Box>
  )
}

/** The ids that exist on a job's board (items and milestones of its snapshot), cached per snapshot. */
const idIndexCache = new WeakMap<object, IdIndex>()
/** The ids of a news model alone (its items and milestones). */
const modelIdIndex = (model: Model): IdIndex => idIndexOf({ waves: [], items: [], events: [], harness: [], updatedAt: 0 } as unknown as MwJobsSnapshot, model)

/** Gopher draws its text as a Code block for copying, so ids in it stay plain; this small row under it lists the ids in view as links. */
const idsRow = (ui: any, $: any, jobId: string, lines: Array<{ t: string }>, index: IdIndex, key: string, from: 'jobs' | 'news') => {
  const { Box, Text, Button } = ui
  const seen = new Map<string, IdRef>()
  for (const l of lines) for (const p of splitIds(l.t, index)) if (p.ref !== undefined && !seen.has(p.ref.id)) seen.set(p.ref.id, p.ref)
  if (seen.size === 0) return null
  return (
    <Box key={`${key}-ids`} flexWrap="wrap" gap={1}>
      <Text dimColor>ids</Text>
      {[...seen.values()].map(ref => (
        <Button key={`${key}-id-${ref.id}`} plain label={ref.id.includes(':') ? ref.id.slice(ref.id.indexOf(':') + 1) : ref.id} onPress={() => void openDetail($, { kind: ref.kind, jobId, tenant: ref.tenant, id: ref.id }, from)} />
      ))}
    </Box>
  )
}

const modelIndexCache = new WeakMap<object, IdIndex>()
const idIndexOf = (snap: MwJobsSnapshot, model?: Model): IdIndex => {
  let base = idIndexCache.get(snap)
  if (base === undefined) {
    base = buildIdIndex(snap.items.map(i => ({ tenant: i.tenant, id: i.id })), snap.waves.map(w => ({ id: w.id, title: w.title })))
    idIndexCache.set(snap, base)
  }
  if (model === undefined) return base
  const hit = modelIndexCache.get(model)
  if (hit !== undefined) return hit
  // a news model carries its own lists: items and milestones the snapshot may not hold
  const itemLists = [model.now.needs, model.now.review, model.now.blocked, model.now.doing, model.now.backlog, model.now.recentDone, model.happened.done, model.happened.accepted, model.happened.sent_back, model.happened.new_items, model.happened.moved_to_review, model.happened.moved_to_doing, model.next.ready]
  const waveLists = [model.now.milestones, model.happened.closed_milestones, model.next.next_milestones]
  const extra = buildIdIndex(
    itemLists.flatMap(l => l.items.map(i => ({ tenant: i.tenant, id: i.id }))),
    waveLists.flatMap(l => l.items.map(i => ({ id: i.id.includes(':') ? i.id : `${i.tenant}:${i.id}`, title: i.title }))),
  )
  const merged = new Map(base)
  for (const [k, v] of extra) if (!merged.has(k)) merged.set(k, v)
  modelIndexCache.set(model, merged)
  return merged
}

/** Text whose ids that exist on the board are link-style Buttons: an item id opens its detail panel, a milestone id its milestone panel. Unknown ids stay text. */
const linkedText = (ui: any, $: any, jobId: string, text: string, index: IdIndex, key: string, from: 'jobs' | 'news' | 'detail', o: { dimColor?: boolean; color?: string; wrap?: 'wrap' | 'truncate-end' } = {}) => {
  const { Box, Text, Button } = ui
  const parts = splitIds(text, index)
  if (!parts.some(p => p.ref !== undefined)) return <Text key={key} dimColor={o.dimColor} color={o.color} wrap={o.wrap ?? 'truncate-end'}>{text}</Text>
  return (
    <Box key={key} flexShrink={1} overflow={o.wrap === 'wrap' ? undefined : 'hidden'} flexWrap={o.wrap === 'wrap' ? 'wrap' : undefined}>
      {parts.map((p, i) =>
        p.ref !== undefined ? (
          <Button key={`${key}-id${i}`} plain dimColor={o.dimColor} label={p.text} onPress={() => void openDetail($, { kind: p.ref!.kind, jobId, tenant: p.ref!.tenant, id: p.ref!.id }, from)} />
        ) : (
          <Text key={`${key}-t${i}`} dimColor={o.dimColor} color={o.color}>{p.text}</Text>
        ),
      )}
    </Box>
  )
}

export const openDetail = async ($: any, r0: MwJobsDetailRef, from: 'jobs' | 'news' | 'detail') => {
  const r = { ...r0, ...splitRef(r0.tenant, r0.id) }
  await update($, detail, (d: MwJobsDetail) => (from === 'detail' ? { cur: r, trail: d.cur ? [...d.trail, d.cur].slice(-20) : d.trail, from: d.from } : { cur: r, trail: [], from }))
  await loadDetail($, r)
  await openChecked($, { id: DETAIL_PANE, title: detailTitle(r), focus: true }, 'detail', 'the jobs pane')
}

/** < Back: the previous panel, else the pane the detail was opened from. */
const detailBack = async ($: any) => {
  const d: MwJobsDetail = await read($, detail)
  const prev = d.trail.at(-1)
  if (prev !== undefined) {
    await update($, detail, (x: MwJobsDetail) => ({ ...x, cur: prev, trail: x.trail.slice(0, -1) }))
    if (((await read($, detailData)) as Record<string, unknown>)[detailKey(prev)] === undefined) await loadDetail($, prev)
    await openChecked($, { id: DETAIL_PANE, title: detailTitle(prev), focus: true }, 'detail', 'the jobs pane')
    return
  }
  const jobId = d.cur?.jobId ?? ''
  if (d.from === 'news') await openNews($, jobId)
  else await backToJobs($, jobId)
}

const STATE_TONE: Record<string, string | undefined> = { blocked: 'error', needs: 'error', review: 'warning', doing: 'warning', done: 'success', closed: 'success' }

/** The detail pane's tree: title row ending in ¦ < Back, then the item's or the milestone's fields as selectable Text. */
const detailTree = (ui: any, $: any, d: MwJobsDetail, data: Record<string, { text?: string; why?: string }>, index: IdIndex = new Map()) => {
  const { Box, Text, Button, Link } = ui
  // free text with the board's ids as links (a milestone's own children and log included)
  const lk = (text: string, key: string, o: { dimColor?: boolean; color?: string; wrap?: 'wrap' | 'truncate-end' } = {}) => linkedText(ui, $, d.cur?.jobId ?? '', text, index, key, 'detail', o)
  const back = <Button key="detail-back" plain label={BACK_LABEL} onPress={() => void detailBack($)} />
  const titleRow = (title: string) => (
    <Box key="detail-title" gap={1}>
      <Text bold wrap="truncate-end">{title}</Text>
      <Text key="sep-nav" dimColor>{'\u00a6'}</Text>
      {back}
    </Box>
  )
  const r = d.cur
  if (r === null) return <Box flexDirection="column">{titleRow('mw detail')}<Text dimColor>Click an item or a milestone in the jobs or news pane.</Text></Box>
  const got = data[detailKey(r)]
  if (got === undefined) return <Box flexDirection="column">{titleRow(`${r.tenant}/${r.id}`)}<Text dimColor>Loading…</Text></Box>
  const field = (k: string, label: string, value: any) => (
    <Box key={`df-${k}`} gap={1}>
      <Box width={11} flexShrink={0}><Text dimColor>{label}</Text></Box>
      <Box flexGrow={1} flexShrink={1}>{typeof value === 'string' ? <Text wrap="wrap">{value}</Text> : value}</Box>
    </Box>
  )
  const itemBtn = (k: string, tenant: string, id: string, label: string) => <Button key={k} plain label={label} onPress={() => void openDetail($, { kind: 'item', jobId: r.jobId, tenant, id }, 'detail')} />
  if (r.kind === 'item') {
    const it = parseItemDetail(got.text, r.tenant)
    if (it === undefined) return <Box flexDirection="column">{titleRow(`${r.tenant}/${r.id}`)}<Text color="warning">{`mw show ${r.id} -t ${r.tenant}: ${got.why ?? 'not readable'}`}</Text></Box>
    return (
      <Box flexDirection="column" gap={0}>
        {titleRow(`${it.id} · ${it.title}`)}
        {field('id', 'id', `${it.tenant}/${it.id}`)}
        {field('kind', 'kind', it.kind || '\u2014')}
        {field('state', 'state', <Text bold color={STATE_TONE[it.state]}>{it.state.toUpperCase()}</Text>)}
        {field('tenant', 'tenant', it.tenant)}
        {field('wave', 'milestone', it.wave === '' ? '\u2014' : <Button key="df-wave-open" plain label={it.wave} onPress={() => void openDetail($, { kind: 'wave', jobId: r.jobId, tenant: it.tenant, id: it.wave }, 'detail')} />)}
        {field('owner', 'owner', it.owner || '\u2014')}
        {it.reason !== null && field('reason', it.reason.label.toLowerCase(), lk(it.reason.text, 'df-reason', { wrap: 'wrap', color: it.reason.label === 'Blocked' ? 'error' : undefined }))}
        {it.evidence.length > 0 && field('evidence', 'evidence', <Box flexDirection="column">{it.evidence.slice(0, 8).map((e, i) => <Text key={`ev-${i}`} dimColor wrap="truncate-end">{e}</Text>)}</Box>)}
        {it.ticket !== null && field('ticket', 'ticket', it.ticket.url !== '' ? <Link key="df-ticket-link" href={it.ticket.url}>{it.ticket.label}</Link> : it.ticket.label)}
        {it.acceptance !== '' && field('acc', 'acceptance', lk(it.acceptance, 'df-acc', { wrap: 'wrap' }))}
        {it.children.length > 0 && field('children', 'children', <Box flexDirection="column">{it.children.map(c => <Box key={`ch-${c.id}`} gap={1}>{itemBtn(`ch-open-${c.id}`, it.tenant, c.id, c.id)}<Text color={STATE_TONE[c.state]}>{c.state}</Text>{lk(c.title, `ch-title-${c.id}`)}</Box>)}</Box>)}
        <Text key="log-head" bold dimColor>RECENT LOG</Text>
        {it.log.length === 0 ? <Text dimColor>{'\u2014'}</Text> : it.log.map((l, i) => <Box key={`log-${i}`} overflow="hidden"><Text dimColor>{`${l.at.slice(0, 16).replace('T', ' ')} `}</Text><Text>{l.action}</Text><Text dimColor>{l.by ? ` ${l.by}` : ''}</Text>{l.note ? <Text dimColor>{' \u2014 '}</Text> : null}{l.note ? lk(l.note, `log-note-${i}`, { dimColor: true }) : null}</Box>)}
      </Box>
    )
  }
  const w = parseWaveDetail(got.text, r.tenant)
  if (w === undefined) return <Box flexDirection="column">{titleRow(`${r.tenant}/${r.id}`)}<Text color="warning">{`mw wave show ${r.id} -t ${r.tenant}: ${got.why ?? 'not readable'}`}</Text></Box>
  return (
    <Box flexDirection="column" gap={0}>
      {titleRow(`${w.id} · ${w.title}`)}
      {field('id', 'id', `${w.tenant}/${w.id}`)}
      {field('state', 'state', <Text bold color={STATE_TONE[w.state]}>{w.state.toUpperCase()}</Text>)}
      {field('done', 'done', `${w.done}/${w.total} \u00b7 ${w.pct}%`)}
      {w.groups.length === 0 && <Text dimColor>No items yet.</Text>}
      {w.groups.map(g => (
        <Box key={`grp-${g.state}`} flexDirection="column">
          <Text bold color={STATE_TONE[g.state]} dimColor={STATE_TONE[g.state] === undefined}>{`${g.state.toUpperCase()} ${g.items.length}`}</Text>
          {g.items.map(i => <Box key={`wi-${i.id}`} gap={1}>{itemBtn(`wi-open-${i.id}`, w.tenant, i.id, i.id)}{lk(i.title, `wi-title-${i.id}`)}</Box>)}
        </Box>
      ))}
    </Box>
  )
}

const backToJobs = async ($: any, id: string) => {
  if (id !== '') {
    await update($, selected, () => id)
    try {
      await $.store.set('selectedJob', id)
    } catch {
      // store unavailable
    }
  }
  const reg: MwJobsRegistry = await read($, registry)
  const sel: string = await read($, selected)
  const job = reg.jobs.find(j => j.id === sel) ?? reg.jobs[0]
  if (job) await refreshJob($, job)
  await openChecked($, { id: PANE, title: await mainTitle($), focus: true }, 'jobs', '/mw-jobs')
}

const tenantCursor = new Map<string, number>()
const REVIEW_PER_POLL = 10
const polledAt = new Map<string, number>()
let pollSeq = 0

/** Which jobs this poll refreshes: the current one, the news job, the quick-tab jobs, then the others round-robin, at most 5. */
const pollSet = async ($: any, reg: MwJobsRegistry): Promise<MwJobsJob[]> => {
  const snaps = (await read($, snapshots)) as Record<string, MwJobsSnapshot>
  const fav = ((await read($, favorites)) as Favorites).jobs.slice(0, 8)
  const auto = attentionSort(reg.jobs.map(j => jobEntry(j, snaps[j.id], undefined))).slice(0, 3).map(e => e.id)
  const must = [(await read($, selected)) as string, newsOpen ? ((await read($, newsJob)) as string) : '', ...fav, ...auto]
  const ids = pollPlan(reg.jobs.map(j => j.id), must, polledAt)
  pollSeq += 1
  for (const id of ids) polledAt.set(id, pollSeq)
  return reg.jobs.filter(j => ids.includes(j.id))
}

/** The host's theme from /config: dark and light are taken as they are, anything else (auto, unknown) follows the OS preference. */
const readTheme = async ($: any) => {
  try {
    const row = ((await $.config.list()) as Array<{ key: string; value: unknown }>).find(r => r.key === 'theme')
    const v = String(row?.value ?? '').toLowerCase()
    setThemeMode(v.includes('dark') ? 'dark' : v.includes('light') ? 'light' : 'auto')
  } catch {
    // no config rows: auto
  }
}

const refreshAll = async ($: any) => {
  await readTheme($)
  // a key changed in a terminal (mw configure) reaches the panes with the next refresh
  await loadSettings($)
  const reg = await loadRegistry($)
  await cleanFavs($, reg)
  await renderStatus($)
  await Promise.all((await pollSet($, reg)).map(j => refreshJob($, j)))
  if (newsOpen) {
    const id: string = await read($, newsJob)
    const nj = reg.jobs.find(j => j.id === id)
    if (nj) {
      await loadNews($, nj)
      await loadNewsOut($, nj)
      await syncNewsTitle($, nj.id)
    }
  }
}

/** A refresh of every job and the open news, one in flight at a time: requests made meanwhile coalesce into one more run. */
let rfBusy = false
let rfAgain = false
const requestRefresh = async ($: any) => {
  if (rfBusy) {
    rfAgain = true
    return
  }
  rfBusy = true
  try {
    do {
      rfAgain = false
      await refreshAll($).catch(() => undefined)
    } while (rfAgain)
  } finally {
    rfBusy = false
  }
}

/** Open items across all jobs (needs, review, blocked, in chat) as one string: a change in it is a change in the counts. */
const openSignature = async ($: any): Promise<string> => {
  const snaps = (await read($, snapshots)) as Record<string, MwJobsSnapshot>
  return Object.entries(snaps)
    .map(([id, sn]) => `${id}:${sn.items.filter(i => openOnBoard(i, sn)).map(i => `${i.tenant}/${i.id}`).sort().join(',')}`)
    .sort()
    .join('|')
}

let chatPoll: { cancel: () => void } | undefined
const CHAT_POLL_MS = 10_000
const CHAT_POLL_TICKS = 12

/** While an item is in chat: refresh every 10 s for up to 2 minutes, stopping at the first change in what is open. */
const startChatPoll = async ($: any) => {
  chatPoll?.cancel()
  chatPoll = undefined
  const base = await openSignature($)
  let ticks = 0
  const me: { cancel: () => void } = $.clock.every(CHAT_POLL_MS, () =>
    void (async () => {
      ticks += 1
      await requestRefresh($)
      const changed = (await openSignature($)) !== base
      const marks = (await read($, inChat)) as Record<string, Record<string, number>>
      const stillChat = Object.values(marks).some(m => Object.keys(m).length > 0)
      if (changed || ticks >= CHAT_POLL_TICKS || !stillChat) {
        me.cancel()
        if (chatPoll === me) chatPoll = undefined
      }
    })(),
  )
  chatPoll = me
}

const refreshSelected = async ($: any) => {
  const reg = await loadRegistry($)
  const id: string = await read($, selected)
  const job = reg.jobs.find(j => j.id === id)
  if (job) {
    await markSeen($, job.id)
    await refreshJob($, job)
  }
}

const arm = async ($: any) => {
  timer?.cancel()
  timer = undefined
  const every: string = await read($, refreshEvery)
  const ms = EVERY_MS[every]
  if (ms !== undefined) timer = $.clock.every(ms, () => void refreshAll($))
}

const setEvery = async ($: any, value: string) => {
  await update($, refreshEvery, () => value)
  try {
    await $.store.set('refreshEvery', value)
  } catch {
    // store unavailable
  }
  await arm($)
  await cfgWrite($, { 'pane.refresh': value })
}

const cycleEvery = async ($: any) => {
  const cur: string = await read($, refreshEvery)
  const i = EVERY.indexOf(cur as (typeof EVERY)[number])
  await setEvery($, EVERY[(i + 1) % EVERY.length]!)
}

/** A style or size change moves the news to other columns or another format: fetch them now, not on the next draw. */
const reloadNewsOut = async ($: any) => {
  if (!newsOpen) return
  const reg: MwJobsRegistry = await read($, registry)
  const id: string = (await read($, newsJob)) || (await read($, selected))
  const job = reg.jobs.find(j => j.id === id) ?? reg.jobs[0]
  if (job) await loadNewsOut($, job)
}

const setStep = async ($: any, next: number) => {
  const v = Math.max(-STEP_MAX, Math.min(STEP_MAX, next))
  await update($, fontStep, () => v)
  try {
    await $.store.set('fontStep', v)
  } catch {
    // store unavailable
  }
  await reloadNewsOut($)
  await cfgWrite($, { 'pane.zoom': String(v) })
}

const stepBy = async ($: any, delta: number) => {
  const cur: number = await read($, fontStep)
  await setStep($, delta === 0 ? 0 : cur + delta)
}

const setBarStyle = async ($: any, v0: string, pane: 'jobs' | 'news' | 'both' = 'both') => {
  const v = v0 === 'board' || v0 === 'j' ? 'kanban' : v0
  if (pane !== 'news') await update($, barStyle, () => v)
  if (pane !== 'jobs') await update($, newsStyle, () => v)
  try {
    if (pane !== 'news') await $.store.set('barStyle', v)
    if (pane !== 'jobs') await $.store.set('newsStyle', v)
  } catch {
    // store unavailable
  }
  await reloadNewsOut($)
  if ((STYLES as readonly string[]).includes(v)) await cfgWrite($, { 'pane.style': v })
}

/** Board text written by sessions: control characters and backticks stripped, quotes softened, one line, cut. */
export const cleanBoard = (t: string, max = 200): string => {
  const one = String(t ?? '').replace(INVISIBLE, '').replace(/[\u0000-\u001f\u007f-\u009f\u2028\u2029]+/g, ' ').replace(/`/g, '').replace(/"/g, "'").replace(/\s+/g, ' ').trim()
  return one.length > max ? safeSlice(one, max).trimEnd() : one
}

export type AskKind = 'needs' | 'review' | 'blocked'
export const askKind = (i: MwJobsItem, snap?: MwJobsSnapshot): AskKind => (isReview(i) ? 'review' : askMap(snap).has(`${i.tenant}:${i.id}`) ? 'needs' : i.state === 'blocked' ? 'blocked' : 'needs')

/** The question for the chat composer: fixed wording first, every board-written field quoted, a pointer to the board last. */
export const askText = (i: MwJobsItem, snap: MwJobsSnapshot | undefined): { kind: AskKind; text: string } => {
  const kind = askKind(i, snap)
  const title = cleanBoard(i.title, 120)
  const tail = ''
  if (kind === 'review') return { kind, text: `Review ${i.tenant} ${i.id} "${title}": what changed, what's the evidence, and is it ready to accept?${tail} \u2014 record my decision on the board first, then act on it.` }
  if (kind === 'blocked') {
    const reason = cleanBoard(i.note || 'no reason given')
    return { kind, text: `${i.tenant} ${i.id} "${title}" is blocked: "${reason}". Why, and how do we unblock it?${tail}` }
  }
  const ask = askMap(snap).get(`${i.tenant}:${i.id}`)
  const question = cleanBoard(ask?.question ?? '')
  const by = /^[\w@.-]{1,40}$/.test(ask?.by ?? '') ? ask!.by! : 'a session'
  const lead = question !== '' ? `About ${i.tenant} ${i.id} "${title}": ${by} asks "${question}". ` : `About ${i.tenant} ${i.id} "${title}": `
  return { kind, text: `${lead}What do you need from me, and what do you recommend?${tail}${ask ? ' \u2014 record my answer on the board first, then act on it.' : ''}` }
}

const lastAsk = { key: '', ts: 0 }

/** Click on an attention item: reload the job, check the item is still open, then fill the composer (never submits). */
const askInChat = async ($: any, job: MwJobsJob, tenant: string, id: string) => {
  const key = `${job.id}|${tenant}|${id}`
  const now: number = await $.clock.now()
  if (lastAsk.key === key && now - lastAsk.ts < 2000) return
  lastAsk.key = key
  lastAsk.ts = now
  await refreshJob($, job)
  void requestRefresh($)
  const snap = ((await read($, snapshots)) as Record<string, MwJobsSnapshot>)[job.id]
  const item = snap?.items.find(i => i.tenant === tenant && i.id === id)
  const open = item !== undefined && item.state !== 'done' && (isReview(item) || isNeedIn(item, snap) || item.state === 'blocked')
  if (!open || item === undefined) {
    $.ui.toast(`Jobs: ${id} is no longer open`, { timeoutMs: 6000 })
    return
  }
  await fillComposer($, askText(item, snap).text)
}

/** Puts text in the composer (replace when empty, insert otherwise); never submits. */
const fillComposer = async ($: any, text: string) => {
  let isFilled = false
  let draft = ''
  try {
    draft = String((await $.prompt.read())?.text ?? '')
  } catch {
    draft = ''
  }
  try {
    // never spliced into a draft: a new line after it, or the box itself when it is empty (I4)
    const r = await $.prompt.fill(draft.trim() === '' ? { text, mode: 'replace' } : { text: `\n${text}`, mode: 'append' })
    isFilled = r.isFilled === true
  } catch {
    isFilled = false
  }
  if (!isFilled) {
    await logFail($, (await read($, selected)) || 'jobs', 'prompt fill refused', 'the composer did not take the text')
    $.ui.toast('Jobs: could not fill the prompt', { timeoutMs: 6000 })
  }
}

/** One question for every open item of a kind: one line each, then the closing line and the board hint. */
export const askGroupText = (kind: 'needs' | 'review', items: MwJobsItem[], snap: MwJobsSnapshot | undefined, count = items.length): string => {
  if (items.length === 1 && count <= 1) return askText(items[0]!, snap).text
  const detail = (i: MwJobsItem) => {
    if (kind === 'review') return i.note && !/awaiting review/i.test(i.note) ? i.note : 'awaiting review'
    const ask = askMap(snap).get(`${i.tenant}:${i.id}`)
    return ask?.question || i.note || (i.state === 'blocked' ? 'blocked' : 'open')
  }
  const lines = items.slice(0, 10).map(i => `- ${i.tenant} ${i.id} "${cleanBoard(i.title, 120)}" \u2014 ${cleanBoard(detail(i), 200)}`)
  if (Math.max(items.length, count) > 10) lines.push(`- +${Math.max(items.length, count) - 10} more`)
  const close = kind === 'review' ? "For each: what changed, what's the evidence, and is it ready to accept? \u2014 record my decision on the board first, then act on it." : 'For each: what do you need from me, why is it blocked if it is, and what do you recommend? \u2014 record my answer on the board first, then act on it.'
  return `${lines.join('\n')}\n${close}`
}

/** The news and jobs top bars' two icons: fill one question covering all open needs / review items. */
const askGroup = async ($: any, job: MwJobsJob, kind: 'needs' | 'review') => {
  const key = `${job.id}|group|${kind}`
  const now: number = await $.clock.now()
  if (lastAsk.key === key && now - lastAsk.ts < 2000) return
  lastAsk.key = key
  lastAsk.ts = now
  await refreshJob($, job)
  void requestRefresh($)
  const snap = ((await read($, snapshots)) as Record<string, MwJobsSnapshot>)[job.id]
  const stall: number = await read($, stalledMin)
  const a = snap ? analyse(snap, now, now, stall * 60_000) : undefined
  const items = a ? (kind === 'needs' ? a.needs : a.review) : []
  if (items.length === 0) {
    $.ui.toast(kind === 'needs' ? 'Jobs: nothing needs you any more' : 'Jobs: nothing to review any more', { timeoutMs: 6000 })
    return
  }
  await fillComposer($, askGroupText(kind, items, snap, countsOf(snap)[kind]))
}

/** The top row's two attention indicators: always shown, `!N` and `\u25d0N` (N may be 0, then dimmed and inert). */
export const attentionLabels = (needs: number, review: number) => ({
  needs: { label: `!${needs}`, live: needs > 0 },
  review: { label: `\u25d0${review}`, live: review > 0 },
})

const askIcons = (ui: any, $: any, job: MwJobsJob, needs: number, review: number) => {
  const { Button } = ui
  const l = attentionLabels(needs, review)
  return [
    l.needs.live
      ? tipped(ui, 'ask-needs', <Button key="ask-needs" plain label={l.needs.label} variant="primary" onPress={() => void askGroup($, job, 'needs')} />, 'Ask in chat about what needs you')
      : tipped(ui, 'ask-needs', <Button key="ask-needs" plain label={l.needs.label} dimColor onPress={() => $.ui.toast('Jobs: nothing needs you.', { timeoutMs: 3000 })} />, 'Nothing needs you'),
    l.review.live
      ? tipped(ui, 'ask-review', <Button key="ask-review" plain label={l.review.label} dimColor onPress={() => void askGroup($, job, 'review')} />, 'Ask in chat about items to review')
      : tipped(ui, 'ask-review', <Button key="ask-review" plain label={l.review.label} dimColor onPress={() => $.ui.toast('Jobs: nothing to review.', { timeoutMs: 3000 })} />, 'Nothing to review'),
  ]
}

/** One compact row of "? ID" buttons under the Svg: needs, review and blocked items, at most 6. */
const askRow = (ui: any, $: any, job: MwJobsJob, items: MwJobsItem[]) => {
  const { Box, Text, Button } = ui
  const shown = items.slice(0, 6)
  if (shown.length === 0) return null
  return (
    <Box key="ask-row" gap={1} flexWrap="wrap">
      <Text dimColor>ask</Text>
      {shown.map(i =>
        tipped(ui, `ask-${i.tenant}-${i.id}`, <Button key={`ask-${i.tenant}-${i.id}`} plain label={`? ${i.id}`} dimColor onPress={() => void askInChat($, job, i.tenant, i.id)} />, `Ask in chat: ${cleanBoard(i.title, 80)}`),
      )}
    </Box>
  )
}

// =====================================================================================
// ONE PLACE FOR THE GOPHER AND BOARD DRAWING (adjust the look here; the line layout is bulletin.ts)
// =====================================================================================

const TONE_PROPS: Record<string, Record<string, unknown>> = {
  fg: {},
  dim: { dimColor: true },
  head: { bold: true },
  red: { color: 'error' },
  amber: { color: 'warning' },
  green: { color: 'success' },
}
const TONE_CLS: Record<string, string> = { fg: 'fg', dim: 'dim', head: 'w fg', red: 'red', amber: 'amber', green: 'green' }

/** A bulletin line as styled pieces: bars coloured per cell, up green and down red, the rest in the line's tone. */
export const bulletinPieces = (l: BLine): Array<{ text: string; cls: string }> => {
  const base = TONE_CLS[l.tone] ?? 'fg'
  if (l.t === '') return [{ text: ' ', cls: base }]
  if (!/\[[#=x.]+\]|[▲▼]/.test(l.t)) return [{ text: l.t, cls: base }]
  const out: Array<{ text: string; cls: string }> = []
  for (const part of l.t.split(/(\[[#=x.]+\]|[▲▼][^\s]*)/)) {
    if (part === '') continue
    if (/^\[[#=x.]+\]$/.test(part)) {
      out.push({ text: '[', cls: 'dim' })
      for (const run of part.slice(1, -1).match(/#+|=+|x+|\.+/g) ?? []) out.push({ text: run, cls: run[0] === '#' ? 'green' : run[0] === '=' ? 'amber' : run[0] === 'x' ? 'red' : 'trk' })
      out.push({ text: ']', cls: 'dim' })
    } else if (part[0] === '▲') out.push({ text: part, cls: 'green' })
    else if (part[0] === '▼') out.push({ text: part, cls: 'red' })
    else out.push({ text: part, cls: base })
  }
  return out
}

/** Gopher lines as one monospace Svg, `cols` columns at `size` px. */
export const buildBulletinSvg = (lines: BLine[], size: number, cols: number): { source: string; alt: string; width: number; height: number } => {
  const cw = size * 0.6
  const pad = 8
  const pitch = Math.round(size * 1.5)
  const W = Math.round((2 * pad + cols * cw) * 100) / 100
  const rows = lines.map((l, i) => {
    const body = bulletinPieces(l).map(pc => `<tspan class="${pc.cls}">${xml(pc.text)}</tspan>`).join('')
    return `<text x="${pad}" y="${4 + i * pitch + Math.round(pitch * 0.72)}" class="m" xml:space="preserve">${body}</text>`
  })
  const H = 4 + lines.length * pitch + 6
  const css = `${MONO_CSS}text{font-family:${MONO};white-space:pre}.m{font-family:${MONO};font-size:${size}px}`
  return { source: `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}"><style>${css}</style>${rows.join('')}</svg>`, alt: lines.slice(0, 6).map(l => l.t).join(' '), width: W, height: H }
}

/** Columns a gopher sheet gets: 70 at most, 40 at least, fitted to the pane (a terminal takes its own columns). */
export const gopherCols = (paneW: number | undefined, size: number, canSvg: boolean, given: number | undefined): number =>
  canSvg ? Math.min(BULLETIN_MAX, Math.floor(((paneW ?? 640) - 16) / (size * 0.6))) : Math.min(BULLETIN_MAX, given && given > 0 ? given : BULLETIN_MAX)

/** Gopher lines painted: mono Svg chunks on the Svg surfaces, Text rows on the terminal. `!`, `?` and `-` lines stay clickable (ask). */
const gopherNodes = (ui: any, lines: BLine[], o: { canSvg: boolean; cols: number; size: number; key: string; ask: (tenant: string, id: string) => void }): any[] => {
  const { Box, Text, Button, Svg, Code } = ui
  if (o.canSvg && Code !== undefined) {
    // selectable: the sheet as plain monospace text (one Code per run of lines); an ask line is its own row with a small action cell
    const out: any[] = []
    let run: string[] = []
    let n = 0
    const flush = () => {
      if (run.length === 0) return
      out.push(<Code key={`${o.key}-c${n}`} source={run.join('\n')} />)
      n += 1
      run = []
    }
    lines.forEach((l, i) => {
      if (!l.ask) {
        run.push(l.t === '' ? ' ' : l.t)
        return
      }
      flush()
      out.push(
        <Box key={`${o.key}-a${i}`} alignItems="center" gap={1}>
          <Code source={l.t} />
          {tipped(ui, `${o.key}-ask-${i}`, <Button key={`${o.key}-b${i}`} plain label="? ask" dimColor onPress={() => o.ask(l.ask!.tenant, l.ask!.id)} />, 'Ask in chat about this')}
        </Box>,
      )
    })
    flush()
    return out
  }
  if (!o.canSvg) {
    return lines.map((l, i) =>
      l.ask ? (
        tipped(ui, `${o.key}-ask-${i}`, <Button key={`${o.key}-b${i}`} plain label={l.t} onPress={() => o.ask(l.ask!.tenant, l.ask!.id)} />, 'Ask in chat about this')
      ) : (
        <Text key={`${o.key}-t${i}`} {...(TONE_PROPS[l.tone] ?? {})}>{l.t === '' ? ' ' : l.t}</Text>
      ),
    )
  }
  const out: any[] = []
  let chunk: BLine[] = []
  let n = 0
  const flush = () => {
    if (chunk.length === 0) return
    const sv = buildBulletinSvg(chunk, o.size, o.cols)
    out.push(<Svg key={`${o.key}-s${n}`} source={sv.source} alt={sv.alt} width={sv.width} />)
    n += 1
    chunk = []
  }
  lines.forEach((l, i) => {
    if (!l.ask) {
      chunk.push(l)
      return
    }
    flush()
    const sv = buildBulletinSvg([l], o.size, o.cols)
    out.push(
      <Box key={`${o.key}-a${i}`} alignItems="center" gap={1}>
        <Svg source={sv.source} alt={sv.alt} width={sv.width} />
        {tipped(ui, `${o.key}-ask-${i}`, <Button key={`${o.key}-b${i}`} plain label="? ask" dimColor onPress={() => o.ask(l.ask!.tenant, l.ask!.id)} />, 'Ask in chat about this')}
      </Box>,
    )
  })
  flush()
  return out
}

/** The ticket of an item as a board key: the tracker's label as a real link when mw gave a URL (mw key in the tooltip), plain text with a label only, else tenant/ID. */
const ticketKey = (ui: any, k: string, i: { tenant: string; id: string; ticket?: string; ticketUrl?: string; ticket_url?: string }, open?: () => void) => {
  const { Text, Link, Button } = ui
  const mw = refOf(i.tenant, i.id)
  const url = i.ticketUrl ?? i.ticket_url ?? ''
  // clickable: the key opens the detail panel (a tracker link stays a link; the title opens it then, see openTitle)
  if (open !== undefined && url === '') return tipped(ui, `${k}-tk`, <Button key={`${k}-open`} plain dimColor={!i.ticket} label={i.ticket || mw} onPress={open} />, `${mw}: details`)
  if (i.ticket && url !== '') return tipped(ui, `${k}-tk`, <Link key={`${k}-link`} href={url}>{i.ticket}</Link>, mw)
  if (i.ticket) return tipped(ui, `${k}-tk`, <Text key={`${k}-key`} color="suggestion">{i.ticket}</Text>, mw)
  return <Text key={`${k}-key`} dimColor>{mw}</Text>
}

/** A row title: a plain Button that opens the detail panel when the key cell is a tracker link (so the row still opens it), else Text. */
const openTitle = (ui: any, k: string, title: string, url: string, open: (() => void) | undefined, props: Record<string, unknown> = {}) => {
  const { Text, Button } = ui
  return open !== undefined && url !== '' ? <Button key={`${k}-ttl`} plain label={title} onPress={open} /> : <Text wrap="truncate-end" {...props}>{title}</Text>
}

/** " [LABEL]" after a title in the other styles: a link where the style can draw one, plain text otherwise. */
const ticketSuffix = (ui: any, k: string, i: { ticket?: string; ticketUrl?: string; ticket_url?: string }) => {
  const { Text, Link } = ui
  if (!i.ticket) return null
  const url = i.ticketUrl ?? i.ticket_url ?? ''
  return url !== '' ? <Link key={`${k}-sfx`} href={url}>{` [${i.ticket}]`}</Link> : <Text key={`${k}-sfx`} dimColor>{` [${i.ticket}]`}</Text>
}

/** A small progress bar as Svg (the board's epic strip). */
const epicBarSvg = (pct: number, w: number): { source: string; width: number } => {
  const f = Math.max(0, Math.min(1, pct / 100)) * (w - 4)
  return {
    source: `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="14" viewBox="0 0 ${w} 14"><style>${MONO_CSS}.gfill{fill:var(--green)}.tr{fill:var(--track)}</style><rect x="2" y="4" width="${w - 4}" height="6" rx="3" class="tr"/><rect x="2" y="4" width="${f}" height="6" rx="3" class="gfill"/></svg>`,
    width: w,
  }
}

export const BOARD_BASE = 5
export const BOARD_STEP = 5
/** Board columns for a pane `px` wide: 4 from 4 x 120, then 2 x 2, then one. */
export const boardColumns = (px: number): 1 | 2 | 4 => (px >= 480 ? 4 : px >= 240 ? 2 : 1)

type BoardCol = { key: 'needs' | 'review' | 'doing' | 'blocked'; title: string; tone: 'error' | 'warning' | undefined; items: MwJobsItem[]; total: number }

const saveExpanded = async ($: any) => {
  try {
    await $.store.set('expanded', await read($, expanded))
  } catch {
    // store unavailable
  }
}

const setBoardShown = async ($: any, id: string, key: string, to: number) => {
  await update($, expanded, (m: Record<string, Record<string, number>>) => ({ ...m, [id]: { ...(m[id] ?? {}), [`b:${key}`]: Math.max(BOARD_BASE, Math.min(GROUP_MAX, to)) } }))
  await saveExpanded($)
}

/** Agile / Kanban document as selectable Text: sections stacked (agile) or in columns of `cols` (kanban). */
const flowNodes = (ui: any, $: any, job: MwJobsJob, d: FDoc, o: { cols: number; ageOf: (since: string) => string; keyPrefix: string; ask: boolean; width?: number; from?: 'jobs' | 'news' }) => {
  const { Box, Text, Button } = ui
  const dim = d.dim
  // a column is a fixed number of cells (body columns / n); every part of a row is cut to fit, nothing spills into the next column
  const colW = o.cols > 1 && o.width !== undefined ? Math.max(12, Math.floor(o.width / o.cols) - 1) : undefined
  // side by side, each column slot stays colW + 1 cells: colW - 1 for the section (content colW - 2 plus 1 padding) and 2 for the "| " separator
  const sideBySide = o.cols > 1
  const secW = colW === undefined ? undefined : sideBySide ? colW - 1 : colW
  const rowW = secW === undefined ? undefined : sideBySide ? secW - 1 : secW
  const row = (sk: string, r: FRow) => {
    const k = `${o.keyPrefix}-${sk}-${r.tenant}-${r.id}`
    const age = o.cols >= 5 ? '' : o.ageOf(r.since)
    const idLabel = r.ticket || (o.cols >= 5 ? r.id : refOf(r.tenant, r.id))
    const idW = rowW === undefined ? Math.min(cellWidth(idLabel), 40) : Math.min(cellWidth(idLabel), Math.max(6, Math.floor(rowW * 0.4)))
    const askBtn = o.ask && r.ask ? tipped(ui, `ask-${k}`, <Button key={`ask-${k}`} plain label="ask" variant="primary" onPress={() => void askInChat($, job, r.tenant, r.id)} />, `Ask in chat: ${cleanBoard(r.title, 80)}`) : null
    const open = () => void openDetail($, { kind: 'item', jobId: job.id, tenant: r.tenant, id: r.id }, o.from ?? (o.keyPrefix.startsWith('n') ? 'news' : 'jobs'))
    const idNode = ticketKey(ui, k, { ...r, ticket: r.ticket !== '' ? cutCells(r.ticket, idW) : '' }, open)
    return (
      <Box key={`fr-${k}`} width={rowW} flexShrink={0} height={rowW === undefined ? undefined : 1} overflow={rowW === undefined ? undefined : 'hidden'} gap={1}>
        <Box key={`fl-${k}`} width={2} flexShrink={0}><Text color="error" bold>{r.flag}</Text></Box>
        <Box key={`id-${k}`} width={idW} flexShrink={0} overflow="hidden">{r.ticket === '' ? <Button key={`open-${k}`} plain dimColor label={idLabel} onPress={open} /> : idNode}</Box>
        <Box key={`ti-${k}`} flexGrow={1} flexShrink={1} overflow="hidden">{openTitle(ui, `ti-${k}`, r.title, r.ticketUrl, open, { dimColor: dim })}</Box>
        {age !== '' && <Box key={`ag-${k}`} flexShrink={0}><Text dimColor>{age}</Text></Box>}
        {askBtn}
      </Box>
    )
  }
  const section = (s: FDoc['sections'][number], width?: number) => (
    <Box key={`fs-${o.keyPrefix}-${s.key}`} flexDirection="column" width={width} flexShrink={0} paddingRight={sideBySide ? 1 : 0}>
      <Text bold color={s.tone} dimColor={dim} wrap={sideBySide ? 'truncate-end' : undefined}>{`${s.title.toUpperCase()} ${s.count}`}</Text>
      {s.sub && <Text dimColor wrap={sideBySide ? 'truncate-end' : undefined}>{s.sub}</Text>}
      {s.rows.length === 0 && <Text dimColor>{'\u2014'}</Text>}
      {s.rows.map(r => row(s.key, r))}
      {s.rest > 0 && <Button key={`more-${o.keyPrefix}-${s.key}`} plain dimColor label={`+${s.rest} more`} onPress={() => void setFill($, o.from ?? (o.keyPrefix.startsWith('n') ? 'news' : 'jobs'), true)} />}
    </Box>
  )
  const head = d.head ? (
    <Text key="fhead" dimColor={dim}>{`${d.head.title}  ${'\u2588'.repeat(Math.round(d.head.pct / 10))}${'\u2591'.repeat(10 - Math.round(d.head.pct / 10))} ${d.head.done}/${d.head.total}`}</Text>
  ) : null
  const notice = d.notice ? <Text key="fnotice" color="warning">{d.notice}</Text> : null
  if (o.cols <= 1) {
    return <Box key={`flow-${o.keyPrefix}`} flexDirection="column" gap={1}>{notice}{head}{d.sections.map(s => section(s))}</Box>
  }
  const rows: FDoc['sections'][] = []
  for (let n = 0; n < d.sections.length; n += o.cols) rows.push(d.sections.slice(n, n + o.cols))
  return (
    <Box key={`flow-${o.keyPrefix}`} flexDirection="column" gap={1}>
      {notice}
      {head}
      {rows.map((r, n) => {
        // one "|" per drawn line (header, sub, rows or dash, "+N more"), as tall as the tallest column of this band
        const lines = Math.max(...r.map(sectionLines))
        const kids: any[] = []
        r.forEach((s, i) => {
          if (i > 0) kids.push(<Box key={`fsep-${o.keyPrefix}-${n}-${i}`} flexDirection="column" width={2} flexShrink={0}>{Array.from({ length: lines }, (_, j) => <Text key={`fsep-${o.keyPrefix}-${n}-${i}-${j}`} dimColor>{'|'}</Text>)}</Box>)
          kids.push(section(s, secW))
        })
        return <Box key={`frow-${o.keyPrefix}-${n}`}>{kids}</Box>
      })}
    </Box>
  )
}
/** Lines a flow section draws: title, optional sub, its rows (or one dash), optional "+N more". */
const sectionLines = (s: FDoc['sections'][number]): number => 1 + (s.sub ? 1 : 0) + Math.max(1, s.rows.length) + (s.rest > 0 ? 1 : 0)
const flowCols = (given: number | undefined): number => (given === undefined ? 2 : given >= 100 ? 5 : given >= 60 ? 2 : 1)

const TONE_COLOR: Record<string, string | undefined> = { red: 'error', amber: 'warning', green: 'success', acc: 'suggestion', error: 'error', warning: 'warning', success: 'success' }
const toneProps = (tone: string | undefined): { color?: string; dimColor?: boolean } => (tone === 'dim' || tone === 'trk' ? { dimColor: true } : TONE_COLOR[tone ?? ''] !== undefined ? { color: TONE_COLOR[tone!] } : {})

/** Markdown blocks as selectable, coloured Text (no Svg): headings bold, runs with their tones, links as Link, tables as aligned cells. */
const mdNodes = (ui: any, blocks: MdBlock[], o: { key: string; mono: boolean }): any[] => {
  const { Box, Text, Link } = ui
  const out: any[] = []
  blocks.forEach((b, i) => {
    const k = `${o.key}-${i}`
    if (b.kind === 'gap') {
      // spacing comes from the headings, not from the blank lines of the source
    } else if (b.kind === 'h1' || b.kind === 'h2' || b.kind === 'h3') {
      if (out.length > 0) out.push(<Text key={`${k}-sp`}> </Text>)
      const t = b.runs.map(r => r.text).join('')
      const color = t.startsWith('!') ? 'error' : t.startsWith('\u25d0') ? 'warning' : t.startsWith('\u2713') ? 'success' : undefined
      out.push(<Text key={k} bold color={color} dimColor={b.kind === 'h2' && color === undefined ? true : undefined}>{b.kind === 'h2' ? t.toUpperCase() : t}</Text>)
    } else if (b.kind === 'p' || b.kind === 'li') {
      const kids: any[] = []
      if (b.kind === 'li') kids.push(<Text key={`${k}-mk`} dimColor>{`${'  '.repeat(b.depth)}${b.marker} `}</Text>)
      const hasHref = b.runs.some(r => r.href)
      b.runs.forEach((r, j) => {
        if (r.href) kids.push(<Link key={`${k}-l${j}`} href={r.href}>{r.text === r.href ? shortUrl(r.href) : r.text}</Link>)
        else kids.push(<Text key={`${k}-t${j}`} bold={r.bold} italic={r.italic} {...toneProps(r.code ? 'acc' : r.tone)}>{r.text}</Text>)
      })
      out.push(hasHref ? <Box key={k} flexWrap="wrap">{kids}</Box> : <Text key={k} wrap="wrap">{kids}</Text>)
    } else if (b.kind === 'table') {
      const widths = b.rows[0]!.map((_, c) => Math.max(...b.rows.map(r => cellWidth(r[c] ?? ''))) + 2)
      out.push(
        <Box key={k} flexDirection="column">
          {b.rows.map((r, ri) => (
            <Box key={`${k}-r${ri}`} height={1} overflow="hidden">
              {r.map((cell, c) => <Box key={`${k}-c${c}`} width={widths[c]} flexShrink={0}><Text bold={ri === 0} dimColor={ri === 0} wrap="truncate-end">{cell}</Text></Box>)}
            </Box>
          ))}
        </Box>,
      )
    } else if (b.kind === 'codeblock') {
      out.push(<Box key={k} flexDirection="column">{b.lines.map((l, li) => <Text key={`${k}-${li}`} dimColor>{l === '' ? ' ' : l}</Text>)}</Box>)
    } else if (b.kind === 'notice') {
      out.push(<Text key={k} {...toneProps(b.tone)}>{o.mono ? `\u258c ${b.text}` : b.text}</Text>)
    }
  })
  return out
}

/** Milestone-style rows as Text cells: name | bar (Svg bar in normal, block glyphs in blocks) | count and state. */
const monoRowsNodes = (ui: any, rows: MonoRow[], o: { key: string; cols: number; svgBars: boolean }): any => {
  const { Box, Text, Svg, Button } = ui
  const NAME = o.cols >= 70 ? 22 : Math.max(12, Math.floor(o.cols * 0.3))
  const rightChars = Math.min(o.cols - NAME - 8, Math.max(6, ...rows.map(r => (r.right ?? []).map(c => c.text).join('').length)))
  const BAR = Math.max(6, Math.min(20, o.cols - NAME - rightChars))
  return (
    <Box key={o.key} flexDirection="column">
      {rows.map((r, i) => {
        const k = `${o.key}-${i}`
        const slot = r.slot
        let mid: any = null
        if (slot?.kind === 'text') mid = <Text {...toneProps(slot.tone ?? 'dim')}>{cutCells(slot.text, BAR)}</Text>
        else if (slot?.kind === 'bar' && o.svgBars) {
          const eb = epicBarSvg(slot.pct, BAR * 6)
          mid = <Svg key={`${k}-bar`} source={eb.source} alt={`${slot.pct}%`} width={eb.width} />
        } else if (slot?.kind === 'bar') {
          const f = Math.round((Math.max(0, Math.min(100, slot.pct)) / 100) * BAR)
          mid = (
            <Box key={`${k}-bar`}>
              <Text color={slot.isNew ? 'success' : undefined}>{'\u2588'.repeat(f)}</Text>
              <Text dimColor>{'\u2591'.repeat(Math.max(0, BAR - f))}</Text>
            </Box>
          )
        }
        return (
          <Box key={`row-${k}`} height={1} overflow="hidden">
            <Box key={`name-${k}`} width={NAME} flexShrink={0} overflow="hidden">{r.open !== undefined ? <Button key={`open-${k}`} plain label={r.name} dimColor={r.nameTone === 'dim'} onPress={r.open} /> : <Text {...toneProps(r.nameTone)} wrap="truncate-end">{r.name}</Text>}</Box>
            <Box key={`status-${k}`} width={BAR} flexShrink={0} overflow="hidden">{mid}</Box>
            <Box key={`right-${k}`} width={rightChars} flexShrink={0} justifyContent="flex-end">
              <Text wrap="truncate-end">{(r.right ?? []).map((c, ci) => <Text key={`${k}-rc${ci}`} {...toneProps(c.tone)}>{c.text}</Text>)}</Text>
            </Box>
          </Box>
        )
      })}
    </Box>
  )
}

/** The jobs board: epic strip, a kanban of four columns, metrics below as compact rows. */
const boardJobs = (
  ui: any,
  $: any,
  job: MwJobsJob,
  d: { epic?: { name: string; done: number; total: number; pct: number }; tail: string; cols: BoardCol[]; exp: Record<string, number>; px: number; metrics: any; canSvg: boolean; ageOf: (i: MwJobsItem) => string },
) => {
  const { Box, Text, Button, Svg } = ui
  const n = boardColumns(d.px)
  const colNodes = d.cols.map(c => {
    const shown = Math.min(GROUP_MAX, d.exp[`b:${c.key}`] ?? BOARD_BASE)
    const rows = c.items.slice(0, shown < GROUP_MAX ? capN(c.total, shown) : shown)
    const hidden = c.total - rows.length
    const more = rows.length < Math.min(c.items.length, GROUP_MAX)
    return (
      <Box key={`bcol-${c.key}`} flexDirection="column" width={n === 4 ? '25%' : n === 2 ? '50%' : '100%'} flexShrink={0} paddingRight={1}>
        <Text bold color={c.tone}>{`${c.title} ${c.total}`}</Text>
        {rows.map(i => {
          const k = `${c.key}-${i.tenant}-${i.id}`
          const ask = c.key !== 'doing'
          return (
            <Box key={`card-${k}`} flexDirection="column" borderStyle="round" borderDimColor paddingX={1}>
              <Text wrap="truncate-end">{i.title}</Text>
              <Box gap={1} alignItems="center">
                {ask ? tipped(ui, `ask-${k}`, <Button key={`ask-${k}`} plain label="ask" variant={c.key === 'blocked' ? undefined : 'primary'} dimColor={c.key === 'blocked'} onPress={() => void askInChat($, job, i.tenant, i.id)} />, `Ask in chat: ${cleanBoard(i.title, 80)}`) : null}
                {ticketKey(ui, k, i, () => void openDetail($, { kind: 'item', jobId: job.id, tenant: i.tenant, id: i.id }, 'jobs'))}
                <Text dimColor>{d.ageOf(i)}</Text>
              </Box>
            </Box>
          )
        })}
        {hidden > 0 &&
          (more ? (
            <Button key={`bmore-${c.key}`} plain label={`+${hidden} more`} dimColor onPress={() => void setBoardShown($, job.id, c.key, shown + BOARD_STEP)} />
          ) : (
            <Text dimColor>{`+${hidden} more on the board`}</Text>
          ))}
        {shown > BOARD_BASE && <Button key={`bless-${c.key}`} plain label="less" dimColor onPress={() => void setBoardShown($, job.id, c.key, shown - BOARD_STEP)} />}
      </Box>
    )
  })
  const rows: any[][] = []
  for (let i = 0; i < colNodes.length; i += n) rows.push(colNodes.slice(i, i + n))
  const epic = d.epic
  return (
    <Box flexDirection="column" gap={1}>
      {epic !== undefined && (
        <Box key="epic" flexDirection="column">
          <Text bold wrap="truncate-end">{`EPIC  ${epic.name}`}</Text>
          <Box alignItems="center" gap={1}>
            {d.canSvg ? (
              (() => {
                const eb = epicBarSvg(epic.pct, Math.max(120, Math.min(300, d.px - 120)))
                return <Svg key="epic-bar" source={eb.source} alt={`${epic.pct}%`} width={eb.width} />
              })()
            ) : (
              <Text>{`[${'#'.repeat(Math.round(epic.pct / 10))}${'.'.repeat(10 - Math.round(epic.pct / 10))}]`}</Text>
            )}
            <Text dimColor>{`${abbr(epic.done)}/${abbr(epic.total)}${d.tail ? ` · ${d.tail}` : ''}`}</Text>
          </Box>
        </Box>
      )}
      {epic === undefined && d.tail !== '' && <Text dimColor>{d.tail}</Text>}
      {rows.map((r, i) => (
        <Box key={`brow-${i}`} flexDirection="row">
          {r}
        </Box>
      ))}
      {d.metrics}
    </Box>
  )
}

type Caps = Record<'needs' | 'review' | 'blocked' | 'doing', { hidden: number; more: boolean; shown: number }>
const GROUP_NAME: Record<string, string> = { needs: 'needs you', review: 'review', blocked: 'blocked', doing: 'in progress' }

/** Under a pane: "+N more X" (expands that group by 10, to 50 rows) and "less X"; past 50 the rest stays on the board. */
const groupMoreRow = (ui: any, $: any, _jobId: string, cap: Caps) => {
  const { Box, Button } = ui
  const nodes: any[] = []
  // every "+N more" is a link that turns fill on for this pane (fill shows every row)
  for (const k of ['needs', 'review', 'blocked', 'doing'] as const) {
    const c = cap[k]
    if (c.hidden > 0) nodes.push(<Button key={`more-${k}`} plain label={`+${c.hidden} more ${GROUP_NAME[k]}`} dimColor onPress={() => void setFill($, 'jobs', true)} />)
  }
  return nodes.length === 0 ? null : (
    <Box key="more-row" gap={2} flexWrap="wrap">
      {nodes}
    </Box>
  )
}

/** "+N more" and "less": the shown rows of one group, by 10, from 10 to 50. */
const expandGroup = async ($: any, id: string, key: string, delta: number) => {
  await update($, expanded, (m: Record<string, Record<string, number>>) => {
    const cur = m[id]?.[key] ?? GROUP_BASE
    const next = Math.max(GROUP_BASE, Math.min(GROUP_MAX, cur + delta))
    return { ...m, [id]: { ...(m[id] ?? {}), [key]: next } }
  })
  await saveExpanded($)
}

const select = async ($: any, id: string) => {
  await update($, selected, () => id)
  try {
    await $.store.set('selectedJob', id)
  } catch {
    // store unavailable
  }
  await openMain($)
}

const toggleMute = async ($: any) => {
  const now: boolean = !(await read($, muted))
  await update($, muted, () => now)
  try {
    await $.store.set('muted', now)
  } catch {
    // store unavailable
  }
  await cfgWrite($, { 'alerts.enabled': String(!now) })
  return { text: now ? 'Jobs: sound and notifications muted.' : 'Jobs: sound and notifications on.' }
}

/** alerts.popup from the loaded keys; a missing key (older mw) or no keys yet counts as on. */
export const popupOf = (s: { ok: boolean | null; keys: Array<{ key: string; value: string }> }): boolean => s.ok !== true || s.keys.find(x => x.key === 'alerts.popup')?.value !== 'false'

/** The bell and popup states in one read (one await: the pane draws in the same ticks as before). */
const readMutedPopup = async ($: any): Promise<{ muted: boolean; popup: boolean }> => {
  const [m, p] = await Promise.all([read($, muted), read($, popupAtom)])
  return { muted: m as boolean, popup: p as boolean }
}

const togglePopup = async ($: any) => {
  const now = !popupOf(await read($, settings))
  const ok = await cfgWrite($, { 'alerts.popup': String(now) })
  if (!ok) {
    $.ui.toast('Jobs: this mw has no popup setting; update mw.', { timeoutMs: 8000 })
    return
  }
  if (now) $.ui.toast('Jobs: popups on.', { timeoutMs: 4000 })
}

/** A snapshot whose "in chat" marks older than 30 minutes are dropped at once, between refreshes too (T4). */
export const expireChat = (s: MwJobsSnapshot | undefined, now: number): MwJobsSnapshot | undefined => {
  if (s?.inChat === undefined) return s
  const keep = Object.fromEntries(Object.entries(s.inChat).filter(([, ts]) => now - ts < CHAT_MS))
  return Object.keys(keep).length === Object.keys(s.inChat).length ? s : { ...s, inChat: keep }
}

/** The first 10 ids, then "+N" (S6). */
export const idList = (ids: string[]): string => (ids.length <= 10 ? ids.join(', ') : `${ids.slice(0, 10).join(', ')}, +${ids.length - 10}`)

const runCommand = async ($: any, args: string, alias: string | undefined) => {
  const word = args.trim()
  if (word === 'mute') return toggleMute($)
  const bs = /^style\s+(normal|thin|blocks|gopher|agile|kanban|board)$/.exec(word) // thin was dropped: it means blocks
  if (bs) {
    const chosen = bs[1] === 'thin' ? 'blocks' : bs[1]!
    await setBarStyle($, chosen)
    return { text: `Jobs: style ${chosen}.` }
  }
  if (word === 'bigger' || word === 'smaller' || word === 'reset') {
    await stepBy($, word === 'bigger' ? 1 : word === 'smaller' ? -1 : 0)
    return { text: `Jobs: density ${word === 'reset' ? 'default' : word}.` }
  }
  const m = /^every\s+(off|15s|30s|1m|5m)$/.exec(word)
  if (m) {
    await setEvery($, m[1]!)
    return { text: `Jobs: auto-refresh ${m[1]}.` }
  }
  const st = /^stalled\s+(\d+)$/.exec(word)
  if (st) {
    await update($, stalledMin, () => Number(st[1]))
    try {
      await $.store.set('stalledMin', Number(st[1]))
    } catch {
      // store unavailable
    }
    return { text: `Jobs: stalled after ${st[1]} min.` }
  }
  const nm = /^news(?:\s+(\S+))?$/.exec(word)
  if (nm) {
    const reg = await loadRegistry($)
    const id: string = nm[1] ?? (alias !== undefined ? alias : await read($, selected))
    if (!reg.jobs.some(j => j.id === id)) return { text: reg.jobs.length === 0 ? 'Jobs: no jobs registered.' : `Jobs: unknown job "${id}" (${idList(reg.jobs.map(j => j.id))}).` }
    await openNews($, id)
    return { text: `News pane opened for ${id}.` }
  }
  const reg = await loadRegistry($)
  const wanted = word !== '' ? word : (alias ?? '')
  if (wanted !== '') {
    if (reg.jobs.some(j => j.id === wanted)) await select($, wanted)
    else if (reg.jobs.length > 0) return { text: `Jobs: unknown job "${wanted}" (${idList(reg.jobs.map(j => j.id))}).` }
  }
  await openMain($, true)
  void refreshAll($).catch(() => undefined)
  return { text: reg.jobs.length === 0 ? 'Jobs: no jobs registered.' : 'Jobs pane opened.' }
}

const MW_USAGE = [
  '/mw-watch                open the jobs pane (zero setup); /mw-watch scan [DIR] finds boards',
  '/mw-jobs [id]            open the jobs pane',
  '/mw-news [id]            open the news pane',
  '/mw-jobs release [id]    publish a news issue (asks, then run it again)',
  '/mw-jobs ask [id]        ask in chat about what needs you',
  '/mw-jobs review [id]     ask in chat about the items to review',
  '/mw-jobs style n|b|g|a|k [jobs|news]   normal, blocks, gopher, agile, kanban',
  '/mw-jobs size -|#|+      text size',
  '/mw-jobs log [id] [work|alerts|decisions]   open the job log',
  '/mw-jobs mute            alerts on/off',
  '/mw-jobs every off|15s|30s|1m|5m  auto refresh',
  '/mw-configure            the settings pane',
  '/mw-fanfare [next|on|off|use NAME|test [milestone|finale]|stop]',
]
/** The first words /mw-jobs hands to runMw; anything else is a job id (or `stalled N`) for runCommand. */
const MW_SUBS = new Set(['list', 'release', 'ask', 'review', 'style', 'size', 'log', 'mute', 'every', 'news', 'jobs', 'configure', 'fanfare'])
const STYLE_KEYS: Record<string, string> = { n: 'normal', b: 'blocks', g: 'gopher', a: 'agile', k: 'kanban', j: 'kanban', normal: 'normal', blocks: 'blocks', gopher: 'gopher', agile: 'agile', kanban: 'kanban', thin: 'blocks', board: 'kanban' }
const LOG_FILES: Record<string, string> = { work: 'work.md', alerts: 'alerts.log', decisions: 'decisions.log' }
/** A release asked for by command arms for a minute; the second /mw release inside it runs. */
const releaseArmed = new Map<string, number>()

/** Opens a job log in the text editor the way the status.md button does: a plain file only, and a missing one is said in a toast. */
const openLogFile = async ($: any, job: MwJobsJob, which: string) => {
  const home: string = (await homeOf($))
  const file = LOG_FILES[which]!
  const path = `${logRoot(home)}/${job.id}/${file}`
  const why = await checkPlainFile($, path, false)
  if (why === 'missing') {
    $.ui.toast(`Jobs: no ${file} for ${job.id} yet (${logRoot(home)}/${job.id}/${file})`, { timeoutMs: 6000 })
    return `No ${file} for ${job.id} yet.`
  }
  if (why !== '') {
    $.ui.toast(`Jobs: not opening ${file}, ${why}.`, { timeoutMs: 6000 })
    return `Not opening ${file}: ${why}.`
  }
  try {
    await proc($, openerArgv(await osKind($), path))
  } catch {
    // no opener
  }
  return `Opened ${file} for ${job.id}.`
}

/** `/mw-fanfare [next|on|off|use NAME|test [milestone|finale]|stop]`: the same settings as the pane's button, one word. Arguments are checked against fixed sets. */
const fanfareCommand = async ($: any, args: string) => {
  const words = args.trim().split(/\s+/).filter(Boolean)
  const reg = await loadRegistry($)
  const sel: string = await read($, selected)
  const job = reg.jobs.find(x => x.id === sel) ?? reg.jobs[0]
  const home: string = await homeOf($)
  const bad = (why: string) => {
    $.ui.toast(`Jobs: ${why}`, { timeoutMs: 6000 })
    return { text: `${why}\n/mw-fanfare [next|on|off|use NAME|test [milestone|finale]|stop]` }
  }
  const sub = words[0] ?? 'next'
  const cwd = job ? expand(job.board, home) : home || '/'
  if (sub === 'next') {
    if (job === undefined) return bad('no job to read the fanfare from')
    await fanfareCycle($, job)
    return { text: 'Fanfare: next setting.' }
  }
  if (sub === 'on' || sub === 'off') {
    if (words.length !== 1) return bad(`/mw-fanfare ${sub} takes nothing`)
    const ok = await cfgWriteOrSet($, [`fanfare.enabled=${sub === 'on' ? 'true' : 'false'}`], cwd)
    if (job) await loadFanfare($, job)
    return { text: ok ? `Fanfare ${sub}.` : 'Could not change the fanfare setting.' }
  }
  if (sub === 'use') {
    const name = words[1]
    if (words.length !== 2 || name === undefined || !/^[A-Za-z0-9_.-]{1,60}$/.test(name)) return bad('/mw-fanfare use needs one set name or all')
    const ok = await cfgWriteOrSet($, [`fanfare.use=${name}`], cwd)
    if (job) await loadFanfare($, job)
    return { text: ok ? `Fanfare: using ${name}.` : 'Could not change the fanfare setting.' }
  }
  const kind = words[1]
  const okStop = sub === 'stop' && words.length === 1
  const okTest = sub === 'test' && words.length <= 2 && (kind === undefined || kind === 'milestone' || kind === 'finale')
  if (!okStop && !okTest) return bad('/mw-fanfare needs next, on, off, use NAME, stop, or test [milestone|finale]')
  const argv = okStop ? ['mw', 'fanfare', 'stop'] : kind === undefined ? ['mw', 'fanfare', 'test'] : ['mw', 'fanfare', 'test', kind]
  let out = ''
  try {
    const r = await proc($, argv, { cwd, timeoutMs: 20_000 })
    out = `${String(r.stdout)}${String(r.stderr)}`.trim().split('\n')[0] ?? ''
    if (r.exitCode !== 0) out = out || `exit ${r.exitCode}`
  } catch {
    out = 'mw not available'
  }
  $.ui.toast(`Jobs: ${argv.slice(1).join(' ')}${out ? ` \u2014 ${out.slice(0, 120)}` : ''}`, { timeoutMs: 6000 })
  return { text: `Ran ${argv.join(' ')}.` }
}

/** `mw configure KEY=VALUE…` when this mw has it, else `mw user set`. */
const cfgWriteOrSet = async ($: any, kv: string[], cwd: string): Promise<boolean> => {
  try {
    const r = await proc($, ['mw', ...(cfgOk === true ? ['configure'] : ['user', 'set']), ...kv], { cwd, timeoutMs: 20_000 })
    return r.exitCode === 0
  } catch {
    return false
  }
}

/** `/mw-jobs` and friends: panes, release, ask/review, style, size, log, mute, every, fanfare. Arguments are checked against fixed sets; nothing a person types reaches a shell. */
const runMw = async ($: any, args: string) => {
  const words = args.trim().split(/\s+/).filter(Boolean)
  const reg = await loadRegistry($)
  const usage = (why: string) => {
    $.ui.toast(`Jobs: ${why}. Try /mw-jobs list for the list.`, { timeoutMs: 6000 })
    return { text: `${why}\n${MW_USAGE.join('\n')}` }
  }
  const listJobs = async () => {
    const snaps: Record<string, MwJobsSnapshot> = await read($, snapshots)
    const stale: Record<string, number> = await read($, staleSince)
    const lines = reg.jobs.length === 0 ? ['no jobs registered'] : reg.jobs.map(j => `${statusSegment(j, snaps[j.id], stale[j.id])}  \u2014 ${j.title}`)
    return { text: [...lines, '', ...MW_USAGE].join('\n') }
  }
  if (words.length === 0 || (words.length === 1 && words[0] === 'list')) return listJobs()
  const cmdWord = words[0]!
  const rest = words.slice(1)
  // [id]: the one given, else the job open in the pane, else the only job, else the list
  const pick = async (token: string | undefined): Promise<MwJobsJob | { text: string }> => {
    if (token !== undefined) {
      const j = reg.jobs.find(x => x.id === token)
      return j ?? usage(`unknown job "${token}" (${reg.jobs.map(x => x.id).join(', ') || 'none registered'})`)
    }
    const sel: string = await read($, selected)
    const cur = reg.jobs.find(x => x.id === sel)
    if (cur) return cur
    if (reg.jobs.length === 1) return reg.jobs[0]!
    return listJobs()
  }
  const isJob = (x: MwJobsJob | { text: string }): x is MwJobsJob => 'id' in x
  switch (cmdWord) {
    case 'jobs':
    case 'news': {
      if (rest.length > 1) return usage(`/mw-${cmdWord === 'news' ? 'news' : 'jobs ' + cmdWord} takes one job id`)
      return runCommand($, cmdWord === 'news' ? `news${rest[0] ? ` ${rest[0]}` : ''}` : (rest[0] ?? ''), undefined)
    }
    case 'release': {
      if (rest.length > 1) return usage('/mw-jobs release takes one job id')
      const j = await pick(rest[0])
      if (!isJob(j)) return j
      const now: number = await $.clock.now()
      const armed = releaseArmed.get(j.id)
      if (armed !== undefined && now - armed < 60_000) {
        releaseArmed.delete(j.id)
        await doRelease($, j)
        return { text: `Publishing the next issue of ${j.id} asked for.` }
      }
      releaseArmed.set(j.id, now)
      await openNews($, j.id)
      await setNewsView($, j.id, { page: 0, list: false, confirm: true })
      $.ui.toast(`Jobs: publish the next issue of ${j.id}? Run /mw-jobs release ${j.id} again within a minute, or press Publish.`, { timeoutMs: 8000 })
      return { text: `Publishing the next issue of ${j.id} is armed: run /mw-jobs release ${j.id} again to confirm.` }
    }
    case 'ask':
    case 'review': {
      if (rest.length > 1) return usage(`/mw-jobs ${cmdWord} takes one job id`)
      const j = await pick(rest[0])
      if (!isJob(j)) return j
      await askGroup($, j, cmdWord === 'ask' ? 'needs' : 'review')
      return { text: cmdWord === 'ask' ? `Asking about what needs you in ${j.id}.` : `Asking about the review items in ${j.id}.` }
    }
    case 'style': {
      const st = STYLE_KEYS[rest[0] ?? '']
      const pane = rest[1]
      if (st === undefined || rest.length > 2 || (pane !== undefined && pane !== 'jobs' && pane !== 'news')) return usage('/mw-jobs style needs n, b, g, a or k, and optionally jobs or news')
      await setBarStyle($, st, pane === undefined ? 'both' : pane)
      return { text: `Jobs: style ${st}${pane ? ` (${pane})` : ''}.` }
    }
    case 'size': {
      const d = rest[0] === '+' ? 1 : rest[0] === '-' ? -1 : rest[0] === '#' ? 0 : undefined
      if (d === undefined || rest.length !== 1) return usage('/mw-jobs size needs -, # or +')
      await stepBy($, d)
      return { text: `Jobs: density ${d === 0 ? 'default' : d > 0 ? 'bigger' : 'smaller'}.` }
    }
    case 'log': {
      const which = rest.find(x => x in LOG_FILES) ?? 'work'
      const idTok = rest.find(x => !(x in LOG_FILES))
      if (rest.length > 2 || (rest.length === 2 && idTok === undefined)) return usage('/mw-jobs log takes an optional job id and work, alerts or decisions')
      const j = await pick(idTok)
      if (!isJob(j)) return j
      return { text: await openLogFile($, j, which) }
    }
    case 'configure': {
      await openSettings($)
      return { text: 'Opened the mw settings pane.' }
    }
    case 'mute': {
      if (rest.length > 0) return usage('/mw-jobs mute takes nothing')
      return toggleMute($)
    }
    case 'every': {
      if (rest.length !== 1 || !['off', '15s', '30s', '1m', '5m'].includes(rest[0]!)) return usage('/mw-jobs every needs off, 15s, 30s, 1m or 5m')
      return runCommand($, `every ${rest[0]}`, undefined)
    }
    case 'fanfare': {
      return fanfareCommand($, rest.join(' '))
    }
    default: {
      // a bare job id still opens that job
      if (words.length === 1 && reg.jobs.some(j => j.id === cmdWord)) return runCommand($, cmdWord, undefined)
      return usage(`unknown subcommand "${cmdWord}"`)
    }
  }
}

/** The user's alert settings (userConfig), set when the module loads. */
export type AlertType = 'milestone' | 'needs' | 'done' | 'new' | 'metric'
let opts: Record<string, unknown> = {}
/** The registry file: the `registry` option, else ~/.mw/jobs.json. A missing file is normal. */
const registryPath = (): string => (typeof opts.registry === 'string' && opts.registry.trim() !== '' ? opts.registry.trim() : REGISTRY_DEFAULT)

/** How many alerted lines are kept per job in $.store. */
export const ALERT_KEEP = 200
/** Appends new alert lines to the per-job list and keeps only the last ALERT_KEEP, so the store cannot grow without bound. */
export const capAlerted = (keys: string[], fresh: string[]): string[] => [...keys, ...fresh.filter(f => !keys.includes(f))].slice(-ALERT_KEEP)

/** One toast bubble per event: the first 4, then one "+N more" bubble for the rest. */
export const TOAST_MAX = 4
export const toastBubbles = (lines: string[]): string[] => {
  const out = lines.slice(0, TOAST_MAX)
  if (lines.length > TOAST_MAX) out.push(`Jobs: +${lines.length - TOAST_MAX} more \u2014 see the news pane`)
  return out
}

/** What one refresh does with its alerts: the grouped toast lines (and rows), and whether the sound plays. */
/** Whether this refresh shows toast popups: alerts on (not muted), message on, and alerts.popup not off (a missing key counts as on). */
export const toastsOn = (sw: { enabled: boolean; message: boolean; popup?: boolean }): boolean => sw.enabled && sw.message && sw.popup !== false

/** The popup button next to the bell: the same width on or off; dim when off or while muted. */
export const popupButton = (popup: boolean, muted: boolean): { label: string; tip: string; dim: boolean } => ({
  label: popup ? '💬' : '💭',
  tip: popup ? 'Popups on \u2014 click to turn off' : 'Popups off \u2014 click to turn on',
  dim: muted || !popup,
})

export const planAlerts = (
  events: AlertEvent[],
  sw: { enabled: boolean; message: boolean; sound: boolean },
  jobAlerts: { sound?: boolean },
): { lines: string[]; sound: boolean } => ({
  lines: sw.enabled && sw.message ? events.map(x => x.text) : [],
  sound: sw.enabled && sw.sound && jobAlerts.sound !== false && events.length > 0,
})

// =====================================================================================
// MANY JOBS, MANY TENANTS: the top bars, the summary pages (⌂), the pickers (☰), favorites
// =====================================================================================

/** The news tail of a job's summary line: `issue 7 (2026-10-22) · draft: 3 changes`. */
const newsTail = (n: NewsEntry | undefined): string => {
  if (n === undefined) return ''
  const rel = n.latest > 0 ? `issue ${n.latest}${n.releases[n.latest] ? ` (${releaseDate(n.releases[n.latest]!)})` : ''}` : 'no issue'
  return n.draft ? `${rel} · draft: ${changeCount(n.draft)} changes` : rel
}

/** A job as one row: the one count source, progress, last change. */
export const jobEntry = (job: MwJobsJob, snap: MwJobsSnapshot | undefined, news: NewsEntry | undefined): Entry => {
  const c = countsOf(snap)
  let done = 0
  let total = 0
  if (snap?.model !== undefined) {
    done = snap.model.done
    total = snap.model.total
  } else if (snap !== undefined) {
    for (const w of snap.waves) {
      const m = memberParts(w)
      done += m.done
      total += m.total
    }
  }
  return { id: job.id, title: job.title, ...c, done, total, last: snap?.model?.lastChange ?? snap?.updatedAt, news: newsTail(news) }
}

/** The job's tenants as rows: the model's order from `now.tenants`; the old path derives them from the lists, in attention order. */
export const tenantEntries = (job: MwJobsJob, snap: MwJobsSnapshot | undefined, now: number): Entry[] => {
  if (snap === undefined) return []
  if (snap.model !== undefined && snap.model.tenants.length > 0) {
    return snap.model.tenants.map(t => ({ id: t.slug, title: t.name, needs: t.needs, review: t.review, blocked: t.blocked, doing: t.doing, done: t.done, total: t.total, last: t.last }))
  }
  const a = analyse(snap, now, now, 0)
  const slugs = [...new Set([...job.tenants, ...snap.items.map(i => i.tenant), ...snap.waves.map(w => w.id.split(':')[0] ?? '')])].filter(t => t !== '' && (job.tenants.length === 0 || job.tenants.includes(t)))
  const rows = slugs.map(t => {
    let done = 0
    let total = 0
    for (const w of snap.waves) {
      if ((w.id.split(':')[0] ?? '') !== t) continue
      const m = memberParts(w)
      done += m.done
      total += m.total
    }
    return { id: t, title: '', needs: a.needs.filter(i => i.tenant === t).length, review: a.review.filter(i => i.tenant === t).length, blocked: a.blocked.filter(i => i.tenant === t).length, doing: a.doing.filter(d => d.item.tenant === t).length, done, total, last: undefined as number | undefined }
  })
  return attentionSort(rows)
}

/** The old path's snapshot for one tenant: every list holds only that tenant. */
export const filterSnapshot = (s: MwJobsSnapshot, tenant: string): MwJobsSnapshot => ({
  ...s,
  waves: s.waves.filter(w => (w.id.split(':')[0] ?? '') === tenant),
  items: s.items.filter(i => i.tenant === tenant),
  events: s.events.filter(e => e.tenant === tenant),
  asks: s.asks?.filter(a => a.tenant === tenant),
})

/** The snapshot a pane draws: the job's, or the one tenant's slice of it. */
export const viewSnapshot = (snap: MwJobsSnapshot | undefined, entry: NewsEntry | undefined, tenant: string, now: number): MwJobsSnapshot | undefined => {
  if (snap === undefined || tenant === '') return snap
  if (snap.model !== undefined && entry?.draft !== undefined) {
    const fm = entry.tdraft?.slug === tenant ? entry.tdraft.model : filterModel(entry.draft, tenant)
    return snapshotFromModel(fm, { inChat: snap.inChat, metrics: snap.metrics, metricsNotice: snap.metricsNotice, now: snap.updatedAt || now })
  }
  return filterSnapshot(snap, tenant)
}

/** The entry a news pane draws for one tenant: the draft and every release filtered to it. */
export const viewEntry = (entry: NewsEntry, tenant: string): NewsEntry => {
  if (tenant === '') return entry
  const releases: Record<number, Model> = {}
  for (const [k, m] of Object.entries(entry.releases)) releases[Number(k)] = filterModel(m, tenant)
  return { ...entry, draft: entry.draft ? (entry.tdraft?.slug === tenant ? entry.tdraft.model : filterModel(entry.draft, tenant)) : undefined, releases }
}

// ---- state ----

const SW_NONE: SwState = { page: 0 }
const swOf = (all: Record<string, SwState>, key: string): SwState => all[key] ?? SW_NONE
const setSw = async ($: any, key: string, patch: Partial<SwState>) => {
  await update($, sw, (m: Record<string, SwState>) => ({ ...m, [key]: { ...(m[key] ?? SW_NONE), ...patch } }))
}
const jobsKey = (pane: 'jobs' | 'news') => `${pane}|jobs`
const tenantsKey = (pane: 'jobs' | 'news', jobId: string) => `${pane}|tenants|${jobId}`

const saveFavorites = async ($: any) => {
  try {
    await $.store.set('favorites', await read($, favorites))
  } catch {
    // store unavailable
  }
}
const starJob = async ($: any, id: string) => {
  await update($, favorites, (f: Favorites) => ({ ...f, jobs: toggleIn(f.jobs, id) }))
  await saveFavorites($)
}
const starTenant = async ($: any, jobId: string, slug: string) => {
  await update($, favorites, (f: Favorites) => ({ ...f, tenants: { ...f.tenants, [jobId]: toggleIn(f.tenants[jobId] ?? [], slug) } }))
  await saveFavorites($)
}
const moveJobFav = async ($: any, id: string, dir: -1 | 1) => {
  await update($, favorites, (f: Favorites) => ({ ...f, jobs: moveIn(f.jobs, id, dir) }))
  await saveFavorites($)
}
const moveTenantFav = async ($: any, jobId: string, slug: string, dir: -1 | 1) => {
  await update($, favorites, (f: Favorites) => ({ ...f, tenants: { ...f.tenants, [jobId]: moveIn(f.tenants[jobId] ?? [], slug, dir) } }))
  await saveFavorites($)
}

/** Drops favorites whose job or tenant no longer exists, quietly. A job whose tenants are not known yet keeps its list. */
const cleanFavs = async ($: any, reg: MwJobsRegistry) => {
  const f = (await read($, favorites)) as Favorites
  const snaps = (await read($, snapshots)) as Record<string, MwJobsSnapshot>
  const slugs = (jid: string): string[] | undefined => {
    const s = snaps[jid]
    if (s === undefined) return undefined
    const job = reg.jobs.find(j => j.id === jid)
    if (s.model !== undefined) return s.model.tenants.length > 0 ? s.model.tenants.map(t => t.slug) : undefined
    return job ? job.tenants : undefined
  }
  const next = cleanFavorites(f, reg.jobs.map(j => j.id), slugs)
  if (JSON.stringify(next) !== JSON.stringify(f)) {
    await update($, favorites, () => next)
    await saveFavorites($)
  }
}

const setTenant = async ($: any, jobId: string, slug: string) => {
  await update($, tenantSel, (m: Record<string, string>) => ({ ...m, [jobId]: slug }))
  try {
    await $.store.set('tenantSel', await read($, tenantSel))
  } catch {
    // store unavailable
  }
  const reg: MwJobsRegistry = await read($, registry)
  const job = reg.jobs.find(j => j.id === jobId)
  if (job && slug !== '') {
    await inFlightP.get(job.id)?.catch(() => undefined)
    await refreshJob($, job)
  }
}

/** The job switch: the jobs pane selects and shows it, the news pane shows that job's news. */
const openJobIn = async ($: any, pane: 'jobs' | 'news', id: string) => {
  await update($, chosen, () => true)
  await setSw($, jobsKey(pane), { view: undefined })
  if (pane === 'jobs') await select($, id)
  else await openNews($, id)
}

// ---- the top bar: tabs ----

type BarIn = {
  key: string
  pane: 'jobs' | 'news'
  /** Nodes before the tabs (⌂, all). */
  lead: any[]
  leadWidth: number
  tabs: Tab[]
  current: string
  favs: string[]
  cols: number
  reserve: number
  /** Key prefix of a tab button (`job` or `tenant`). */
  prefix: string
  /** Keys of ◂ ▸ ☰ and the +N tab. */
  keys: { prev: string; next: string; pick: string; plus: string }
  noun: string
  open: (id: string) => void
  step: (dir: -1 | 1) => void
  pick: (fav: boolean) => void
  pickActive: boolean
}

const tabNode = (ui: any, o: BarIn, t: Tab, active: boolean) => {
  const { Box, Text, Button } = ui
  return (
    <Box key={`tab-${o.prefix}-${t.id}`}>
      {t.needs > 0 ? <Text color="error" bold>{`!${t.needs} `}</Text> : t.review > 0 ? <Text color="warning">{`◐${t.review} `}</Text> : <Text> </Text>}
      <Button key={`${o.prefix}-${t.id}`} plain label={cutLabel(t.label)} variant={active ? 'primary' : 'secondary'} dimColor={!active} onPress={() => o.open(t.id)} />
    </Box>
  )
}

/** Every item as a tab when they fit the real width; else favorites (or the automatic few) + +N + ◂ | current | ▸ | ☰ N. */
const barGroup = (ui: any, o: BarIn) => {
  const { Button } = ui
  const plan = planBar({ tabs: o.tabs, current: o.current, favs: o.favs, cols: o.cols, lead: o.leadWidth, reserve: o.reserve })
  if (plan.mode === 'all') return group(ui, o.key, [...o.lead, ...plan.tabs.map(t => tabNode(ui, o, t, t.id === o.current))])
  const nodes: any[] = [...o.lead, ...plan.tabs.map(t => tabNode(ui, o, t, false))]
  if (plan.plus > 0) nodes.push(tipped(ui, o.keys.plus, <Button key={o.keys.plus} plain label={`+${plan.plus}`} dimColor onPress={() => o.pick(true)} />, `${plan.plus} more favorites`))
  nodes.push(
    tipped(ui, o.keys.prev, <Button key={o.keys.prev} plain label={'◂'} dimColor={plan.cur === undefined} onPress={() => o.step(-1)} />, `Previous ${o.noun}`),
    ...(plan.cur ? [tabNode(ui, o, plan.cur, true)] : []),
    tipped(ui, o.keys.next, <Button key={o.keys.next} plain label={'▸'} dimColor={plan.cur === undefined} onPress={() => o.step(1)} />, `Next ${o.noun}`),
    tipped(ui, o.keys.pick, <Button key={o.keys.pick} plain label={`☰ ${plan.count}`} variant={o.pickActive ? 'primary' : undefined} onPress={() => o.pick(false)} />, `Pick a ${o.noun} (all ${plan.count})`),
  )
  return group(ui, o.key, nodes)
}

const tabsOf = (rows: Entry[]): Tab[] => rows.map(e => ({ id: e.id, label: e.id, needs: e.needs, review: e.review }))

/**
 * The job bar: `⌂ | tabs…` when everything fits, else `⌂ | favorites… | +N | ◂ | job | ▸ | ☰ N`. One job: no group.
 * Order is attention order. The ⌂ opens the all-jobs summary page.
 */
const jobBar = (ui: any, $: any, o: { pane: 'jobs' | 'news'; entries: Entry[]; current: string; favs: string[]; cols: number; reserve: number; swAll: Record<string, SwState> }) => {
  const { Button } = ui
  if (o.entries.length < 2) return null
  const ordered = attentionSort(o.entries)
  const st = swOf(o.swAll, jobsKey(o.pane))
  const key = jobsKey(o.pane)
  const idx = ordered.findIndex(e => e.id === o.current)
  return barGroup(ui, {
    key: 'tabs',
    pane: o.pane,
    lead: [tipped(ui, 'jobs-all', <Button key="jobs-all" plain label={'⌂'} variant={st.view === 'all' ? 'primary' : undefined} onPress={() => void setSw($, key, { view: st.view === 'all' ? undefined : 'all' })} />, 'All jobs, one line each')],
    leadWidth: 4,
    tabs: tabsOf(ordered),
    current: o.current,
    favs: o.favs,
    cols: o.cols,
    reserve: o.reserve,
    prefix: 'job',
    keys: { prev: 'jobs-prev', next: 'jobs-next', pick: 'jobs-pick', plus: 'jobs-plus' },
    noun: 'job',
    open: id => void openJobIn($, o.pane, id),
    step: dir => {
      const to = ordered[Math.max(0, Math.min(ordered.length - 1, (idx < 0 ? 0 : idx) + dir))]
      if (to) void openJobIn($, o.pane, to.id)
    },
    pick: fav => void setSw($, key, fav ? { view: 'pick', fav: true, page: 0, range: undefined } : { view: 'pick' }),
    pickActive: st.view === 'pick',
  })
}

/**
 * The tenant bar: `⌂ | all | tabs…` or `⌂ | all | favorites… | +N | ◂ | tenant | ▸ | ☰ N`. One tenant: no group.
 * Tenant order is the model's. `all` clears the filter.
 */
const tenantBar = (ui: any, $: any, o: { pane: 'jobs' | 'news'; job: MwJobsJob; entries: Entry[]; current: string; favs: string[]; cols: number; swAll: Record<string, SwState> }) => {
  const { Button } = ui
  if (o.entries.length < 2) return null
  const key = tenantsKey(o.pane, o.job.id)
  const st = swOf(o.swAll, key)
  const idx = o.entries.findIndex(e => e.id === o.current)
  const to = (slug: string) => void (async () => {
    await setSw($, key, { view: undefined })
    await setTenant($, o.job.id, slug)
  })()
  return barGroup(ui, {
    key: 'tenant-tabs',
    pane: o.pane,
    lead: [
      tipped(ui, 'tenants-all', <Button key="tenants-all" plain label={'⌂'} variant={st.view === 'all' ? 'primary' : undefined} onPress={() => void setSw($, key, { view: st.view === 'all' ? undefined : 'all' })} />, 'All tenants, one line each'),
      tipped(ui, 'tenant-all', <Button key="tenant-all" plain label="all" variant={o.current === '' ? 'primary' : 'secondary'} dimColor={o.current !== ''} onPress={() => to('')} />, 'All tenants'),
    ],
    leadWidth: 10,
    tabs: tabsOf(o.entries),
    current: o.current,
    favs: o.favs,
    cols: o.cols,
    reserve: 0,
    prefix: 'tenant',
    keys: { prev: 'tenants-prev', next: 'tenants-next', pick: 'tenants-pick', plus: 'tenants-plus' },
    noun: 'tenant',
    open: slug => to(slug),
    step: dir => {
      // all, then the tenants in the model's order
      const at = o.current === '' ? -1 : idx
      const n = at + dir
      to(n < 0 ? '' : (o.entries[Math.min(o.entries.length - 1, n)]?.id ?? ''))
    },
    pick: fav => void setSw($, key, fav ? { view: 'pick', fav: true, page: 0, range: undefined } : { view: 'pick' }),
    pickActive: st.view === 'pick',
  })
}

/** "tenant: slug ✕" while a tenant filter is on. */
const tenantChip = (ui: any, $: any, jobId: string, slug: string) => {
  const { Box, Text, Button } = ui
  if (slug === '') return null
  return (
    <Box key="tenant-chip" gap={1} alignItems="center">
      <Text dimColor>{`tenant: ${slug}`}</Text>
      {tipped(ui, 'tenant-clear', <Button key="tenant-clear" plain label={'✕'} onPress={() => void setTenant($, jobId, '')} />, 'Show all tenants')}
    </Box>
  )
}

// ---- the pages: ⌂ summary and ☰ picker ----

type PageIn = {
  skey: string
  st: SwState
  rows: Entry[]
  favs: string[]
  /** keepOrder: tenants keep the model's order. */
  keepOrder: boolean
  mode: 'pick' | 'all'
  noun: string
  cols: number
  lineOf: (e: Entry, w: number) => string
  open: (id: string) => void
  star: (id: string) => void
  move: (id: string, dir: -1 | 1) => void
  close: () => void
}

const rowStyleOf = (style: string): 'text' | 'gopher' | 'loz' => (style === 'gopher' ? 'gopher' : style === 'agile' || style === 'kanban' ? 'loz' : 'text')

/** One page of rows: a star (its own target), the line (the whole line is the target), pager, letter jumps. */
const listPage = (ui: any, $: any, o: PageIn) => {
  const { Box, Text, Button } = ui
  const st = { ...o.st, view: o.mode }
  const list = listFor(o.rows, st, o.favs, o.keepOrder)
  const pg = paged(list.rows, o.st.page)
  const setPage = (n: number) => void setSw($, o.skey, { page: Math.max(0, Math.min(pg.pages - 1, n)) })
  const w = Math.max(30, Math.min(100, o.cols - 8))
  const favSet = new Set(o.favs)
  const rows: any[] = []
  pg.rows.forEach((e, i) => {
    const g = pg.page * PAGE + i
    if (list.needs > 0 && g === 0) rows.push(<Text key="hdr-needs" bold color="error">{`NEEDS YOU ${list.needs}`}</Text>)
    if (list.needs > 0 && g === list.needs && list.rows.length > list.needs) rows.push(<Text key="hdr-rest" dimColor>{`OTHERS ${list.rows.length - list.needs}`}</Text>)
    const starred = favSet.has(e.id)
    rows.push(
      <Box key={`line-${e.id}`} alignItems="center" gap={1}>
        {tipped(ui, `star-${e.id}`, <Button key={`star-${e.id}`} plain label={starred ? '★' : '☆'} dimColor={!starred} onPress={() => o.star(e.id)} />, starred ? 'Remove from favorites' : 'Add to favorites')}
        {o.st.fav && starred ? [
          <Button key={`fav-left-${e.id}`} plain dimColor label={'◂'} onPress={() => o.move(e.id, -1)} />,
          <Button key={`fav-right-${e.id}`} plain dimColor label={'▸'} onPress={() => o.move(e.id, 1)} />,
        ] : null}
        <Button key={`row-${e.id}`} plain label={o.lineOf(e, w)} onPress={() => o.open(e.id)} />
      </Box>,
    )
  })
  const pager = group(ui, 'pager', [
    tipped(ui, 'pg-first', <Button key="pg-first" plain label={'⇤'} dimColor={pg.page === 0} onPress={() => setPage(0)} />, 'First page'),
    tipped(ui, 'pg-prev', <Button key="pg-prev" plain label={'◂'} dimColor={pg.page === 0} onPress={() => setPage(pg.page - 1)} />, 'Previous page'),
    <Text key="pg-label" dimColor>{` ${pagerLabel(pg)} `}</Text>,
    tipped(ui, 'pg-next', <Button key="pg-next" plain label={'▸'} dimColor={pg.page >= pg.pages - 1} onPress={() => setPage(pg.page + 1)} />, 'Next page'),
    tipped(ui, 'pg-last', <Button key="pg-last" plain label={'⇥'} dimColor={pg.page >= pg.pages - 1} onPress={() => setPage(pg.pages - 1)} />, 'Last page'),
  ])
  const letters = group(ui, 'letters', [
    ...RANGES.map(r => tipped(ui, `rg-${r.key}`, <Button key={`rg-${r.key}`} plain label={r.label} variant={o.st.range === r.key ? 'primary' : undefined} dimColor={o.st.range !== r.key} onPress={() => void setSw($, o.skey, { range: o.st.range === r.key ? undefined : r.key, fav: false, page: 0 })} />, `${o.noun}s starting ${r.label}`)),
  ])
  const total = o.rows.length
  return (
    <Box key="page" flexDirection="column" gap={1}>
      <Box alignItems="center" gap={2} flexWrap="wrap">
        <Text bold>{o.st.fav ? `Favorites · ${list.rows.length}` : o.mode === 'all' ? `All ${o.noun}s · ${total}` : `Pick a ${o.noun} · ${total}`}</Text>
        {tipped(ui, 'fav-filter', <Button key="fav-filter" plain label={`★ ${o.favs.length}`} variant={o.st.fav ? 'primary' : undefined} dimColor={!o.st.fav} onPress={() => void setSw($, o.skey, { fav: !o.st.fav, range: undefined, page: 0 })} />, o.st.fav ? 'Show all' : 'Only favorites')}
        {tipped(ui, 'pick-close', <Button key="pick-close" plain label={'✕ close'} dimColor onPress={o.close} />, 'Back to the page')}
      </Box>
      <Box alignItems="center" gap={2} flexWrap="wrap">
        {letters}
        {o.st.range !== undefined ? tipped(ui, 'rg-clear', <Button key="rg-clear" plain label={'✕'} onPress={() => void setSw($, o.skey, { range: undefined, page: 0 })} />, 'All letters') : null}
      </Box>
      {rows.length === 0 ? <Text dimColor>{o.st.fav ? 'No favorites yet. Press ☆ on a line.' : `No ${o.noun}s${o.st.range ? ' in that range' : ''}.`}</Text> : <Box flexDirection="column">{rows}</Box>}
      {pager}
    </Box>
  )
}

export const register: Register = (on, options) => {
  opts = options as Record<string, unknown>
  on('session.start', async ($, e, next) => {
    // a model-callable way to open the panes (the same as /mw-jobs or /mw-news [id]); it only opens a pane
    try {
      await $.tool.register({
        name: 'mw_panes_open',
        description: 'Open the mw jobs pane or the mw news pane on a registered job (the same as /mw-jobs [id] or /mw-news [id]). Read-only apart from showing the pane.',
        inputSchema: { type: 'object', properties: { pane: { type: 'string', enum: ['jobs', 'news'] }, job: { type: 'string', description: 'a job id from the registry; default the job open in the pane' } }, required: ['pane'], additionalProperties: false },
        isDeferred: false,
      })
    } catch {
      // the tool is a convenience: no registration, no tool
    }
    sessionCwd = typeof e?.cwd === 'string' ? e.cwd : ''
    scanCache = undefined
    mwProbed = false
    homeCache = undefined
    await replayTranscript($)
    await $.command.register({ name: 'mw-watch', description: 'Open the mw jobs pane; it finds your boards on its own. /mw-watch scan [DIR] looks for more.' })
    await $.command.register({ name: 'mw-jobs', description: 'mw jobs pane: /mw-jobs [id | release | ask | review | style | size | log | mute | every …]' })
    await $.command.register({ name: 'mw-news', description: 'mw news pane: /mw-news [id]' })
    await $.command.register({ name: 'mw-configure', description: 'mw settings: refresh, style, zoom, fit, alerts, fanfare, status, discovery (the mw configure keys)' })
    await $.command.register({ name: 'mw-fanfare', description: 'mw fanfare: /mw-fanfare [next | on | off | use NAME | test [milestone|finale] | stop]' })
    // a new session starts from nothing: what survives a reload is in $.store
    await update($, chosen, () => false)
    await update($, favorites, () => ({ jobs: [], tenants: {} }))
    await update($, tenantSel, () => ({}))
    await update($, sw, () => ({}))
    try {
      const saved = await $.store.get('refreshEvery')
      if (typeof saved === 'string' && EVERY_ALL.includes(saved)) {
        await update($, refreshEvery, () => saved)
      }
      const savedSeen = await $.store.get('seen')
      if (savedSeen !== null && typeof savedSeen === 'object') await update($, seen, () => savedSeen as Record<string, MwJobsSeen>)
      const savedChat = await $.store.get('inChat')
      if (savedChat !== null && typeof savedChat === 'object') await update($, inChat, () => savedChat as Record<string, Record<string, number>>)
      const savedKnown = await $.store.get('relKnown')
      if (savedKnown !== null && typeof savedKnown === 'object') await update($, relKnown, () => savedKnown as Record<string, number>)
      const savedRelSeen = await $.store.get('relSeen')
      if (savedRelSeen !== null && typeof savedRelSeen === 'object') await update($, newsView, () => Object.fromEntries(Object.entries(savedRelSeen as Record<string, number>).map(([k, v]) => [k, { seen: v }])))
      const savedPage = await $.store.get('newsPage')
      if (savedPage !== null && typeof savedPage === 'object') await update($, newsView, (m: Record<string, NewsViewState>) => ({ ...m, ...Object.fromEntries(Object.entries(savedPage as Record<string, unknown>).filter(([, v]) => typeof v === 'number').map(([k, v]) => [k, { ...(m[k] ?? {}), page: v as number }])) }))
      const savedExp = await $.store.get('expanded')
      if (savedExp !== null && typeof savedExp === 'object') await update($, expanded, () => savedExp as Record<string, Record<string, number>>)
      const savedMute = await $.store.get('muted')
      if (typeof savedMute === 'boolean') await update($, muted, () => savedMute)
      const savedAl = await $.store.get('alerted')
      if (savedAl !== null && typeof savedAl === 'object') await update($, alerted, () => savedAl as Record<string, string[]>)
      const savedNs = await $.store.get('newsSeen')
      if (savedNs !== null && typeof savedNs === 'object') await update($, newsSeen, () => savedNs as Record<string, string>)
      const savedStep = await $.store.get('fontStep')
      if (typeof savedStep === 'number') await update($, fontStep, () => Math.max(-STEP_MAX, Math.min(STEP_MAX, savedStep)))
      const savedBar = await $.store.get('barStyle')
      if (savedBar === 'normal' || savedBar === 'blocks' || savedBar === 'gopher' || savedBar === 'agile' || savedBar === 'kanban') await update($, barStyle, () => savedBar)
      if (savedBar === 'thin') await update($, barStyle, () => 'blocks')
      if (savedBar === 'board' || savedBar === 'j') await update($, barStyle, () => 'kanban')
      const savedNews = await $.store.get('newsStyle')
      if (savedNews === 'normal' || savedNews === 'blocks' || savedNews === 'gopher' || savedNews === 'agile' || savedNews === 'kanban') await update($, newsStyle, () => savedNews)
      if (savedNews === 'board' || savedNews === 'j') await update($, newsStyle, () => 'kanban')
      const savedFill = await $.store.get('blocksFill')
      if (savedFill !== null && typeof savedFill === 'object') await update($, blocksFill, () => ({ jobs: (savedFill as { jobs?: unknown }).jobs === true, news: (savedFill as { news?: unknown }).news === true }))
      const savedStall = await $.store.get('stalledMin')
      if (typeof savedStall === 'number') await update($, stalledMin, () => savedStall)
      const sel = await $.store.get('selectedJob')
      if (typeof sel === 'string') {
        await update($, selected, () => sel)
        await update($, chosen, () => true)
      }
      const savedAlerts = parseAlertRows(await $.store.get('alertRows'))
      await update($, alertRows, () => savedAlerts)
      const savedFav = await $.store.get('favorites')
      if (savedFav !== null && typeof savedFav === 'object') await update($, favorites, () => parseFavorites(savedFav))
      const savedTs = await $.store.get('tenantSel')
      if (savedTs !== null && typeof savedTs === 'object') await update($, tenantSel, () => Object.fromEntries(Object.entries(savedTs as Record<string, unknown>).filter(([, v]) => typeof v === 'string')) as Record<string, string>)
    } catch {
      // defaults
    }
    // the per-user mw keys win over the store (the store is their cache); an older mw keeps the store, with a hint
    await initSettings($)
    await loadRegistry($)
    // the chat bar shows at once, from the registry or the boards discovery found ("loading" until the refresh lands)
    await renderStatus($)
    void (async () => openMain($, false, await cfgGet($, 'pane.open')))().catch(() => undefined)
    await arm($)
    await refreshAll($)

    return next(e)
  })

  // a submitted filled question marks its items "in chat"; the text is passed through untouched
  on('prompt.submit', async ($, e, next) => {
    try {
      const text = String((e as { text?: string }).text ?? '')
      await markChat($, text)
      if (chatKeys(text).length > 0) {
        // the question is on its way: refresh both panes now, and keep looking for a while
        void requestRefresh($)
        void startChatPoll($)
      }
    } catch {
      // never block a prompt
    }
    return next(e)
  })

  // a turn that ends while something is in chat: the answer may have changed the board
  on('turn.complete', async ($, e, next) => {
    try {
      const marks = (await read($, inChat)) as Record<string, Record<string, number>>
      if (Object.values(marks).some(m => Object.keys(m).length > 0)) void requestRefresh($)
    } catch {
      // never block a turn
    }
    return next(e)
  })

  // every tool call (the main loop's and its agents'): observed after it ran, never changed
  on('tool.call', async ($: any, e: any, next: any) => {
    const r = await next(e)
    await watchUse($, e)
    return r
  })
  on('tool.call', { tool: 'mcp__mw-jobs__mw_panes_open' as never }, async ($: any, e: any) => {
    const input = e as { pane?: unknown; job?: unknown }
    if (input.pane !== 'jobs' && input.pane !== 'news') return { deny: 'pane must be "jobs" or "news"' }
    if (input.job !== undefined && typeof input.job !== 'string') return { deny: 'job must be a job id string' }
    // a cold board discovery can take minutes (`mw boards ~`): never hold the model's call for it. Wait a few seconds, then answer.
    const loading = loadRegistry($)
    loading.catch(() => undefined)
    const reg: MwJobsRegistry | undefined = await Promise.race([loading, $.clock.sleep(PANES_OPEN_WAIT_MS).then(() => undefined)])
    if (reg === undefined) return { result: 'The mw jobs pane is still looking for your boards. Call mw_panes_open again in a few seconds.' }
    if (input.job !== undefined && !reg.jobs.some(j => j.id === input.job)) return { deny: `unknown job "${String(input.job).slice(0, 60)}" (${idList(reg.jobs.map(j => j.id)) || 'none registered'})` }
    if (reg.jobs.length === 0) return { deny: 'no jobs registered' }
    const r = (await runCommand($, input.pane === 'news' ? `news${input.job ? ` ${input.job}` : ''}` : (input.job ?? ''), undefined)) as { text?: string }
    return { result: r.text ?? `${input.pane} pane opened` }
  })

  const argsOf = (e: unknown) => String((e as { args?: string }).args ?? '').trim()
  // /mw-watch: the zero-setup entry (opens the jobs pane); `scan [DIR]` looks for more boards
  on('command.run', { command: 'mw-watch' }, async ($, e) => {
    const a = argsOf(e)
    const m = /^scan(?:\s+(.+))?$/.exec(a)
    if (m) return scanCommand($, m[1]?.trim())
    return runCommand($, a, undefined)
  })
  // /mw-jobs [id] and its subcommands; /mw-news [id]
  on('command.run', { command: 'mw-jobs' }, async ($, e) => {
    const a = argsOf(e)
    const first = a.split(/\s+/)[0] ?? ''
    if (a === '' || MW_SUBS.has(first) || /^(stalled\s+\d+|bigger|smaller|reset)$/.test(a)) return MW_SUBS.has(first) ? runMw($, a) : runCommand($, a, undefined)
    const reg = await loadRegistry($)
    return reg.jobs.some(j => j.id === a) ? runCommand($, a, undefined) : runMw($, a)
  })
  on('command.run', { command: 'mw-news' }, async ($, e) => runMw($, `news ${argsOf(e)}`.trim()))
  on('command.run', { command: 'mw-fanfare' }, async ($, e) => fanfareCommand($, argsOf(e)))

  // the news pane closed: it is not open any more, the refresh stops loading its news (T8)
  on('ui.close', async ($, e, next) => {
    if ((e as { id?: string }).id === NEWS_PANE) newsOpen = false
    return next(e)
  })

  on('ui.render', { component: 'Pane', requestId: DETAIL_PANE }, async ($, e) => guardDraw('detail', $, e, async () => {
    const ui = $.ui.resolve(e) as any
    const dd = (await read($, detail)) as MwJobsDetail
    const snap = dd.cur === null ? undefined : (((await read($, snapshots)) as Record<string, MwJobsSnapshot>)[dd.cur.jobId])
    const dm = dd.cur === null ? undefined : (((await read($, newsModel)) as Record<string, NewsEntry>)[dd.cur.jobId])?.draft
    return detailTree(ui, $, dd, (await read($, detailData)) as Record<string, { text?: string; why?: string }>, snap === undefined ? new Map() : idIndexOf(snap, dm))
  }))
  on('ui.render', { component: 'Pane', requestId: SETTINGS_PANE }, async ($, e) => guardDraw('settings', $, e, async () => {
    const ui = $.ui.resolve(e) as any
    const st = (await read($, settings)) as CfgState
    const reg: MwJobsRegistry = await read($, registry)
    const sel: string = await read($, selected)
    const job = reg.jobs.find(j => j.id === sel) ?? reg.jobs[0]
    const fans = (await read($, fanfare)) as Record<string, MwJobsFanfare | null>
    return settingsTree(ui, $, st, job ? fans[job.id] : undefined, job)
  }))
  on('command.run', { command: 'mw-configure' }, async ($) => {
    await openSettings($)
    return { text: 'Opened the mw settings pane.' }
  })
  on('ui.render', { component: 'Pane', requestId: NEWS_PANE }, async ($, e) => guardDraw('news', $, e, async () => {
    const { Box, Text, Link, Button, Markdown, Svg, Code } = $.ui.resolve(e) as any
    const reg: MwJobsRegistry = await read($, registry)
    const wanted: string = (await read($, newsJob)) || (await read($, selected))
    const job = reg.jobs.find(j => j.id === wanted) ?? reg.jobs[0]
    if (job === undefined) {
      return (
        <Box flexDirection="column">
          <Box alignItems="center" gap={2} flexWrap="wrap">
            <Text bold>mw news</Text>
            <Text key="sep-nav" dimColor>{'\u00a6'}</Text>
            <Button key="back-jobs" plain label={JOBS_LABEL} onPress={() => void backToJobs($, '')} />
          </Box>
          <Text dimColor>no jobs registered</Text>
          <Text dimColor>{`Registry: ${registryPath()}`}</Text>
          <Text dimColor>Boards are found on their own; or list jobs in the registry file.</Text>
        </Box>
      )
    }
    // read before anything below starts a load (a load's write must land after this drawing is dispatched)
    const fanBtn = await fanfareButton($, job)
    const mentry = ((await read($, newsModel)) as Record<string, NewsEntry>)[job.id]
    if (mentry === undefined && !modelProbed.has(job.id)) {
      modelProbed.add(job.id)
      void loadModels($, job)
    }
    const mm = mentry !== undefined && (mentry.draft !== undefined || mentry.nums.length > 0)
    const nview = (((await read($, newsView)) as Record<string, NewsViewState>)[job.id] ?? {}) as NewsViewState
    const entry = (await read($, news))[job.id] as { text: string | null; truncated: boolean } | undefined
    const body =
      entry === undefined
        ? 'Loading…'
        : entry.text === null
          ? NEWS_MISSING
          : entry.truncated
            ? `${entry.text}\n\n*… truncated (first 9,500 characters)*`
            : entry.text
    const wantedStyle: string = await read($, newsStyle)
    // without a news model there is no bulletin and no release notes: gopher and board draw as blocks
    const oldStyle = wantedStyle === 'gopher' || wantedStyle === 'agile' || wantedStyle === 'kanban' ? 'blocks' : wantedStyle
    const noModelWhy = mm ? undefined : modelWhy.get(job.id)
    const modelNote = mm ? undefined : noModelNotice(wantedStyle, noModelWhy)
    const noModelTip = noModelWhy === undefined || /format|unknown|usage|invalid|unrecognized/i.test(noModelWhy) ? 'needs the newer mw (no news model)' : `needs the news model: ${noModelWhy}`
    const every: string = await read($, refreshEvery)
    const step: number = await read($, fontStep)
    const canSvg = e.surface !== 'terminal'
    const given = Number((e.props as { bodyColumns?: number }).bodyColumns)
    const hasGiven = Number.isFinite(given) && given > 0
    const baseWidth = Number.isFinite(given) && given > 0 ? Math.max(300, Math.min(520, given * 8)) : 420
    const paneW = Number.isFinite(given) && given > 0 ? given * 8 : 420
    const isMissing = entry !== undefined && entry.text === null
    const home: string = (await homeOf($))
    // mw's own render (text for blocks and the terminal, md for normal); status.md only when mw fails
    lastTerminal = !canSvg
    lastPaneW = paneW
    const kind = 'md' as 'text' | 'md' // mw 0.3.0: `news --format text` is the 70-column bulletin whatever --width says, so blocks is drawn here from the md/json
    const fillNews = ((await read($, blocksFill)) as { jobs: boolean; news: boolean }).news
    setMonoMax(fillNews ? FILL_MAX : COLS)
    lastBodyCols = hasGiven ? given : undefined
    const colsNow = !canSvg ? termCols(lastBodyCols, fillNews) : monoCols(paneW, svgSizeFor(step) * 0.6)
    const outEntry = ((await read($, newsOut)) as Record<string, { ok: boolean; text: string }>)[outKey(job.id, kind, colsNow)]
    if (outEntry === undefined && !mm) void runNewsOut($, job, kind, colsNow)
    const mwOk = outEntry?.ok === true
    const noteFallback = outEntry !== undefined && !outEntry.ok
    const textLines = mwOk && kind === 'text' ? stripTextHead(newsTextLines(outEntry!.text)) : undefined
    const mdText = kind === 'md' && mwOk ? outEntry!.text : isMissing ? NEWS_MISSING : entry?.truncated ? `${entry.text}\n\n… truncated (first 9,500 characters)` : body
    const mono = oldStyle === 'blocks'
    const chunks = canSvg && !textLines ? chunkMd(kind === 'md' && mwOk ? stripMdHead(parseMd(mdText)) : parseMd(mdText), job.news) : []
    // blocks: one mono Svg for the whole body (links drawn accent + underlined), plus one Links row
    const flat = chunks.flatMap(c => ('svg' in c ? c.svg : [c.link]))
    const urls = new Map<string, string>()
    for (const b of flat) if (b.kind === 'p' || b.kind === 'li') for (const r of b.runs) if (r.href && !urls.has(r.href)) urls.set(r.href, r.text === r.href ? shortUrl(r.href) : r.text)
    const noteText = 'mw news text unavailable — showing status.md'
    const noteInSvg = noteFallback && mono && canSvg
    const parts: typeof chunks = mono && canSvg ? [{ svg: noteInSvg ? [{ kind: 'notice' as const, tone: 'amber', text: noteText }, ...flat] : flat, sec: '' }] : chunks
    const lastIdx = parts.length - 1
    if (textLines) for (const [u, l] of newsTextUrls(textLines)) urls.set(u, l)
    const pieceProps = (cls: string): any => {
      const o: any = {}
      if (cls.includes('red')) o.color = 'error'
      else if (cls.includes('amber')) o.color = 'warning'
      else if (cls.includes('green')) o.color = 'success'
      else if (cls.includes('acc')) o.color = 'suggestion'
      else if (cls.includes('dim') || cls.includes('trk')) o.dimColor = true
      if (cls.includes('w')) o.bold = true
      return o
    }
    const body2: any[] = textLines
      ? canSvg
        ? [(() => { const sv = buildTextSvg(textLines, step, paneW); return <Svg key="mwtext" source={sv.source} alt={sv.alt} width={sv.width} /> })()]
        : textLines.map((l, i) => (
            <Box key={`tl${i}`}>
              {l === '' ? <Text> </Text> : tokenizeNewsLine(l).map((pc, j) => <Text key={`p${j}`} {...pieceProps(pc.cls)}>{pc.text}</Text>)}
            </Box>
          ))
      : canSvg
        ? mdNodes({ Box, Text, Link }, noteInSvg ? [{ kind: 'notice' as const, tone: 'amber', text: noteText }, ...flat] : flat, { key: 'md', mono })
        : chunks.map((part, i) => {
      const b = ('link' in part ? [part.link] : part.svg)[0]
      return <Text key={`pl${i}`}>{b && 'runs' in b ? b.runs.map(r => r.text).join('') : ''}</Text>
    })
    if (urls.size > 0 && ((mono && canSvg) || textLines)) {
      body2.push(
        <Box key="links" gap={1}>
          <Text dimColor>Links</Text>
          {[...urls].map(([href, label]) => <Link key={`lk-${href}`} href={href}>{label}</Link>)}
        </Box>,
      )
    }
    const allSnaps = (await read($, snapshots)) as Record<string, MwJobsSnapshot>
    const nowMs: number = await $.clock.now()
    const nsnap0 = expireChat(allSnaps[job.id] as MwJobsSnapshot | undefined, nowMs)
    // many jobs and tenants: the bars, the tenant filter, the summary pages
    const allModels = (await read($, newsModel)) as Record<string, NewsEntry>
    const swAll = (await read($, sw)) as Record<string, SwState>
    const favAll = (await read($, favorites)) as Favorites
    const chosenFlag = (await read($, chosen)) as boolean
    const jobEntries = reg.jobs.map((j: MwJobsJob) => jobEntry(j, allSnaps[j.id], allModels[j.id]))
    const tenantRowsAll = mm ? tenantEntries(job, nsnap0, nowMs) : []
    const tsel0 = (((await read($, tenantSel)) as Record<string, string>)[job.id] ?? '') as string
    const tsel = tsel0 !== '' && tenantRowsAll.some(t => t.id === tsel0) ? tsel0 : ''
    const nsnap = viewSnapshot(nsnap0, mentry, tsel, nowMs)
    const mentryV = mentry !== undefined ? viewEntry(mentry, tsel) : undefined
    const barCols = fitCols(hasGiven ? given : undefined, svgSizeFor(step))
    const jv0 = swOf(swAll, jobsKey('news'))
    const jobView = jv0.view ?? (reg.jobs.length > 1 && !chosenFlag ? 'all' : undefined)
    const tv = swOf(swAll, tenantsKey('news', job.id))
    const tenantView = jobView === undefined ? tv.view : undefined
    const totals = jobView !== undefined ? totalsOf(jobEntries) : tenantView !== undefined ? totalsOf(tenantRowsAll) : countsOf(nsnap)
    const nAn = nsnap ? analyse(nsnap, nowMs, nowMs, (await read($, stalledMin)) * 60_000) : undefined
    const newsAsk: MwJobsItem[] = nAn ? [...nAn.needs, ...nAn.review] : []
    const mnodes = mm ? await modelNewsNodes($, { Box, Text, Button, Link, Svg, Markdown, Code }, job, { entry: mentryV!, view: nview, canSvg, paneW, given: Number.isFinite(given) && given > 0 ? given : undefined, step, style: wantedStyle }) : undefined
    // title and description come from the job (the same header as the jobs pane); the body drops mw's own copies
    const newsDesc = (((await read($, jobDesc)) as Record<string, string>)[job.id] || job.description || '').replace(/\s+/g, ' ')
    const descCols = canSvg ? Math.max(20, Math.floor(paneW / 8)) : COLS
    // a summary page (⌂) or picker (☰) replaces the news: jobs first, else this job's tenants
    let pageNode: any
    if (jobView !== undefined || tenantView !== undefined) {
      const rs = rowStyleOf(wantedStyle)
      const isJobs = jobView !== undefined
      const mode = (isJobs ? jobView : tenantView) as 'pick' | 'all'
      const skey = isJobs ? jobsKey('news') : tenantsKey('news', job.id)
      pageNode = listPage({ Box, Text, Button }, $, {
        skey,
        st: isJobs ? { ...jv0, view: jobView } : tv,
        rows: isJobs ? jobEntries : tenantRowsAll,
        favs: isJobs ? favAll.jobs : (favAll.tenants[job.id] ?? []),
        keepOrder: !isJobs,
        mode,
        noun: isJobs ? 'job' : 'tenant',
        cols: barCols,
        lineOf: (e, w) => (mode === 'all' ? (isJobs ? newsLine(e, w, rs === 'gopher' ? 'gopher' : 'text') : summaryLine(e, w, rs)) : pickLine(e, w, isJobs ? hhmmMs(e.last) : `${abbr(e.done)}/${abbr(e.total)}`)),
        open: id => void (isJobs ? openJobIn($, 'news', id) : (async () => { await setSw($, skey, { view: undefined }); await setTenant($, job.id, id) })()),
        star: id => void (isJobs ? starJob($, id) : starTenant($, job.id, id)),
        move: (id, dir) => void (isJobs ? moveJobFav($, id, dir) : moveTenantFav($, job.id, id, dir)),
        close: () => void (async () => { await update($, chosen, () => true); await setSw($, skey, { view: undefined }) })(),
      })
    }
    return (
      <Box flexDirection="column" gap={1}>
        <Box flexDirection="column">
          <Box alignItems="center" gap={2} flexWrap="wrap">
            <Text bold wrap="truncate-end">{job.title}</Text>
            <Text key="sep-nav" dimColor>{'\u00a6'}</Text>
            <Button key="back-jobs" plain label={JOBS_LABEL} onPress={() => void backToJobs($, job.id)} />
          </Box>
          {reg.jobs.length > 1 && <Box alignItems="center" gap={2} flexWrap="wrap">{jobBar({ Box, Text, Button }, $, { pane: 'news', entries: jobEntries, current: job.id, favs: favAll.jobs, cols: barCols, reserve: 0, swAll })}</Box>}
          {(tenantRowsAll.length > 1 || tsel !== '') && (
            <Box alignItems="center" gap={2} flexWrap="wrap">
              {tenantBar({ Box, Text, Button }, $, { pane: 'news', job, entries: tenantRowsAll, current: tsel, favs: favAll.tenants[job.id] ?? [], cols: barCols, swAll })}
              {tenantChip({ Box, Text, Button }, $, job.id, tsel)}
            </Box>
          )}
          {newsDesc !== '' && wrapDesc(newsDesc, descCols).map((l, i) => <Text key={`nd${i}`} dimColor>{l}</Text>)}
          {controlsRow({ Box, Text, Button, Link }, $, {
            job,
            refresh: { key: 'news-refresh', onPress: () => void (async () => { await loadModels($, job); await loadNews($, job); await loadNewsOut($, job); await markNewsSeen($, job.id) })() },
            every,
            wanted: wantedStyle,
            pane: 'news' as const,
            ...(await readMutedPopup($)),
            needs: totals.needs,
            review: totals.review,
            hasPage: /^https:\/\//.test(job.news),
            canOpen: newsStatusPath(lastJobShow[job.id], job.board, homeCache ?? '') !== undefined,
            canSvg,
            legend: e.surface === 'mobile',
            unavailable: mm ? undefined : { gopher: noModelTip, agile: noModelTip, kanban: noModelTip },
            fill: fillNews,
            fanfare: fanBtn,
            scan: reg.source === 'scan' ? { tip: reg.hint ?? 'Scan for mw boards', onPress: () => void rescan($) } : undefined,
          })}
        </Box>
        {pageNode !== undefined ? pageNode : null}
        {pageNode !== undefined ? null : modelNote !== undefined && <Text key="model-note" color="warning">{modelNote}</Text>}
        {noteFallback && !noteInSvg && !mm && <Text dimColor>{noteText}</Text>}
        {pageNode !== undefined ? null : mnodes !== undefined ? <Box key="model-news" flexDirection="column" gap={1}>{mnodes}</Box> : canSvg || textLines ? <Box flexDirection="column">{body2}</Box> : <Markdown text={body} dimColor={isMissing} />}
      </Box>
    )
  }))

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => guardDraw('jobs', $, e, async () => {
    const { Box, Text, Link, Button, Svg, Code } = $.ui.resolve(e) as any
    const reg = await read($, registry)
    // Text `size`, `weight` and `mono` are refused on the terminal and desktop tables of this build:
    // sz() and mo add nothing and wt is plain bold
    const wt: any = { bold: true }
    const mo: any = {}
    const sz = (_v: string): any => ({})

    if (reg.jobs.length === 0 && reg.error === null && reg.loaded !== true) {
      return (
        <Box flexDirection="column">
          <Text bold>mw jobs</Text>
          <Text dimColor>Loading…</Text>
        </Box>
      )
    }
    if (reg.jobs.length === 0) {
      return (
        <Box flexDirection="column">
          <Box key="empty-row" gap={1}>
            <Text bold>mw jobs</Text>
            <Text key="sep-scan" dimColor>{'\u00a6'}</Text>
            {tipped({ Box, Text }, 'scan', <Button key="scan" plain label="Scan" dimColor onPress={() => void rescan($)} />, 'Scan for mw boards (up from this folder, then ~)')}
          </Box>
          <Text dimColor>{reg.error ?? 'no jobs registered'}</Text>
          {reg.hint ? <Text key="scan-hint" dimColor>{reg.hint}</Text> : null}
          <Text dimColor>{`Registry: ${registryPath()}`}</Text>
          <Text dimColor>Boards are found on their own; or list jobs in the registry file.</Text>
        </Box>
      )
    }

    const sel = await read($, selected)
    const job = reg.jobs.find(j => j.id === sel) ?? reg.jobs[0]!
    const fanBtn = await fanfareButton($, job)
    const snaps = await read($, snapshots)
    const stale = (await read($, staleSince))[job.id]
    const every = await read($, refreshEvery)
    const isBusy = (await read($, busy)).includes(job.id)
    const cursors = await read($, seen)
    const stallMin = await read($, stalledMin)
    const now: number = await $.clock.now()
    const snap0 = expireChat(snaps[job.id] as MwJobsSnapshot | undefined, await $.clock.now())
    const newsModels = (await read($, newsModel)) as Record<string, NewsEntry>
    const tsel0 = (((await read($, tenantSel)) as Record<string, string>)[job.id] ?? '') as string
    const nowJ: number = await $.clock.now()
    const tenantRowsAll = tenantEntries(job, snap0, nowJ)
    const tsel = tsel0 !== '' && tenantRowsAll.some(t => t.id === tsel0) ? tsel0 : ''
    const snap = viewSnapshot(snap0, newsModels[job.id], tsel, nowJ)
    const given = Number((e.props as { bodyColumns?: number }).bodyColumns)
    const hasWidth = Number.isFinite(given) && given > 0
    // Text has no font size here, so the +/-/# buttons change density
    const wanted: string = await read($, barStyle)
    // columns from 50 body columns, stacked below
    const rowsLayout = Number((e.props as { bodyColumns?: number }).bodyColumns) >= 50 ? 'columns' : 'stacked'
    const canSvg = e.surface !== 'terminal'
    const fillJobs = ((await read($, blocksFill)) as { jobs: boolean; news: boolean }).jobs
    setMonoMax(fillJobs ? FILL_MAX : COLS)
    // off the Svg surfaces normal and board draw as blocks (the board's columns need Box layout over an Svg surface)
    const flowWanted = wanted === 'agile' || wanted === 'kanban'
    const flowNoModel = flowWanted && newsModels[job.id]?.draft === undefined
    const flowNote = flowNoModel ? noModelNotice(wanted, modelWhy.get(job.id)) : undefined
    const style: string = (wanted === 'normal' && !canSvg) || flowNoModel ? 'blocks' : wanted
    const step: number = await read($, fontStep)
    const density = step < 0 ? 'compact' : step > 0 ? 'roomy' : 'default'
    const columns = hasWidth ? Math.max(20, Math.min(style === 'blocks' ? (fillJobs ? FILL_MAX : COLS) : 100, given)) : 40

    const unseenNews = await newsUnseen($, job.id)
    // many jobs and tenants: entries, favorites, the bars and the summary pages
    const swAll = (await read($, sw)) as Record<string, SwState>
    const favAll = (await read($, favorites)) as Favorites
    const chosenFlag = (await read($, chosen)) as boolean
    const entries = reg.jobs.map((j: MwJobsJob) => jobEntry(j, snaps[j.id], newsModels[j.id]))
    const barCols = fitCols(hasWidth ? given : undefined, svgSizeFor(step))
    const jv0 = swOf(swAll, jobsKey('jobs'))
    const jobView = jv0.view ?? (reg.jobs.length > 1 && !chosenFlag ? 'all' : undefined)
    const tv = swOf(swAll, tenantsKey('jobs', job.id))
    const tenantView = jobView === undefined ? tv.view : undefined
    const pageRows = jobView !== undefined ? entries : tenantRowsAll
    const totals = jobView !== undefined ? totalsOf(entries) : tenantView !== undefined ? totalsOf(tenantRowsAll) : countsOf(snap)
    const descText = (((await read($, jobDesc)) as Record<string, string>)[job.id] || job.description || '').replace(/\s+/g, ' ')
    const header = (
      <Box flexDirection="column">
        <Box alignItems="center" gap={2} flexWrap="wrap">
          {reg.jobs.length < 2 && <Text {...wt} wrap="truncate-end">{job.title}</Text>}
          {jobBar({ Box, Text, Button }, $, { pane: 'jobs', entries, current: job.id, favs: favAll.jobs, cols: barCols, reserve: 14, swAll })}
          {isBusy && <Text dimColor>refreshing…</Text>}
          <Text key="sep-nav" dimColor>{'\u00a6'}</Text>
          <Button key="news" plain label={NEWS_LABEL} onPress={() => void openNews($, job.id)} />
        </Box>
        {(tenantRowsAll.length > 1 || tsel !== '') && (
          <Box alignItems="center" gap={2} flexWrap="wrap">
            {tenantBar({ Box, Text, Button }, $, { pane: 'jobs', job, entries: tenantRowsAll, current: tsel, favs: favAll.tenants[job.id] ?? [], cols: barCols, swAll })}
            {tenantChip({ Box, Text, Button }, $, job.id, tsel)}
          </Box>
        )}
        {flowNote !== undefined && <Text color="warning">{flowNote}</Text>}
        {job.tenants.length === 0 && snap0?.model === undefined && <Text dimColor>no tenants set</Text>}
        {snap !== undefined && snap.items.length === 0 && snap.waves.length === 0 && snap.model === undefined && job.tenants.length > 0 && <Text dimColor>No items yet</Text>}
        {snap !== undefined && snap.model !== undefined && snap.model.counts.needs + snap.model.counts.review + snap.model.counts.blocked + snap.model.counts.doing + snap.model.total === 0 && <Text dimColor>No items yet</Text>}
        {snap !== undefined && stale !== undefined && <Text color="warning">{`mw unavailable since ${hhmm(stale)}${jobWhy.get(job.id) ? ` \u2014 ${jobWhy.get(job.id)}` : ''} (showing the last poll)`}</Text>}
        {descText !== '' && wrapDesc(descText, columns).map((l, i) => <Text key={`jd${i}`} dimColor>{l}</Text>)}
        {controlsRow({ Box, Text, Button, Link }, $, {
          job,
          refresh: { key: 'refresh', onPress: () => void refreshSelected($) },
          every,
          wanted,
          pane: 'jobs' as const,
          ...(await readMutedPopup($)),
          needs: totals.needs,
          review: totals.review,
          hasPage: /^https:\/\//.test(job.news),
          canOpen: newsStatusPath(lastJobShow[job.id], job.board, homeCache ?? '') !== undefined,
          canSvg,
          legend: e.surface === 'mobile',
          fill: fillJobs,
          fanfare: fanBtn,
          scan: reg.source === 'scan' ? { tip: reg.hint ?? 'Scan for mw boards', onPress: () => void rescan($) } : undefined,
        })}
      </Box>
    )
    const separator = e.surface !== 'terminal' ? null : <Text dimColor wrap="truncate-end">{'─'.repeat(columns)}</Text>

    // a summary page (⌂) or picker (☰) replaces the content: jobs first, else this job's tenants
    if (jobView !== undefined || tenantView !== undefined) {
      const rs = rowStyleOf(wanted)
      const isJobs = jobView !== undefined
      const mode = (isJobs ? jobView : tenantView) as 'pick' | 'all'
      const skey = isJobs ? jobsKey('jobs') : tenantsKey('jobs', job.id)
      const closePage = () => void (async () => {
        await update($, chosen, () => true)
        await setSw($, skey, { view: undefined })
      })()
      return (
        <Box flexDirection="column" gap={1}>
          {header}
          {listPage({ Box, Text, Button }, $, {
            skey,
            st: isJobs ? { ...jv0, view: jobView } : tv,
            rows: pageRows,
            favs: isJobs ? favAll.jobs : (favAll.tenants[job.id] ?? []),
            keepOrder: !isJobs,
            mode,
            noun: isJobs ? 'job' : 'tenant',
            cols: barCols,
            lineOf: (e, w) => (mode === 'all' ? summaryLine(e, w, rs) : pickLine(e, w, isJobs ? hhmmMs(e.last) : `${abbr(e.done)}/${abbr(e.total)}`)),
            open: id => void (isJobs ? openJobIn($, 'jobs', id) : (async () => { await setSw($, skey, { view: undefined }); await setTenant($, job.id, id) })()),
            star: id => void (isJobs ? starJob($, id) : starTenant($, job.id, id)),
            move: (id, dir) => void (isJobs ? moveJobFav($, id, dir) : moveTenantFav($, job.id, id, dir)),
            close: closePage,
          })}
        </Box>
      )
    }

    if (snap === undefined) {
      return (
        <Box flexDirection="column">
          {header}
          {!canSvg && separator}
          <Text dimColor>{stale === undefined ? 'Loading…' : `mw unavailable since ${hhmm(stale)}${jobWhy.get(job.id) ? ` \u2014 ${jobWhy.get(job.id)}` : ''}`}</Text>
        </Box>
      )
    }

    const cursor = cursors[job.id] as MwJobsSeen | undefined
    // metrics (any job's own measures): one view model for every style; from a model the deltas are against the last poll
    const prevSnaps = (await read($, previous)) as Record<string, MwJobsSnapshot>
    const mviewsAll = metricViews(snap.metrics, snap.model !== undefined ? metricsBase(prevSnaps[job.id]) : cursor?.metrics)
    const seenTs = cursor?.ts ?? now
    const a0 = analyse(snap, now, seenTs, stallMin * 60_000)
    const brief = snap.model
    const head: Headline = brief ? modelHeadline(a0, brief, countsOf(snap)) : headline(a0)
    const labels = tsel !== '' ? tenantLabels([tsel]) : tenantLabels(job.tenants.length > 0 ? job.tenants : [...new Set(snap.waves.map(w => w.id.split(':')[0] ?? ''))])
    const wavesAll = snap.waves.filter(w => !isPlaceholder(w))
    // open milestones only, nearest to done first, at most 8; the rest as "+N open · ✓N closed"
    const openAll = wavesAll.filter(w => w.state !== 'done')
    const openSorted = brief ? openAll : [...openAll].sort((x, y) => y.percent - x.percent)
    const openCount = brief ? brief.openMilestones : openSorted.length
    const closedCount = brief ? brief.closedMilestones : wavesAll.length - openAll.length
    // fill on lifts every cap of the pane (and mw's, with --full); an mw without --full keeps its caps and says so
    const fullJobs = fillJobs && !fullUnsupported.has(job.id)
    const shownWaves = fullJobs ? openSorted : openSorted.slice(0, capN(openCount, MILES_MAX))
    const tailOf = (shown: number) => [openCount - shown > 0 ? `+${openCount - shown} open` : '', closedCount > 0 ? `\u2713${closedCount} closed` : ''].filter(Boolean).join(' \u00b7 ')
    const readyCount = snap.items.filter(i => i.state === 'ready').length
    const events: MwJobsEvent[] = []
    // each group: first 10, "+N more" expands by 10 up to 50
    const exp = (((await read($, expanded)) as Record<string, Record<string, number>>)[job.id] ?? {}) as Record<string, number>
    const all = <T,>(l: T[], extra: number) => ({ rows: l, hidden: extra, more: false, shown: l.length })
    const cap = fullJobs
      ? { needs: all(a0.needs, brief?.more.needs ?? 0), review: all(a0.review, brief?.more.review ?? 0), blocked: all(a0.blocked, brief?.more.blocked ?? 0), doing: all(a0.doing, brief?.more.doing ?? 0) }
      : {
          needs: capList(a0.needs, exp.needs, brief?.more.needs ?? 0),
          review: capList(a0.review, exp.review, brief?.more.review ?? 0),
          blocked: capList(a0.blocked, exp.blocked, brief?.more.blocked ?? 0),
          doing: capList(a0.doing, exp.doing, brief?.more.doing ?? 0),
        }
    // one bar length for every wave, style and layout: a single function of the body width
    const barLen = barLength(hasWidth ? given : undefined)
    // one line only where name, bar and count fit; blocks always take two
    const twoLine = rowsLayout === 'stacked'
    const itemAge = (i: MwJobsItem) => {
      const t = Date.parse(i.updated)
      return Number.isNaN(t) ? '' : ageText(now - t)
    }
    const label = (text: string, color?: string) => (
      <Text {...sz("caption")} {...wt} color={color} dimColor={color === undefined}>{text.toUpperCase()}</Text>
    )
    const pid = (tenant: string, id: string) => (labels[tenant] ? `${labels[tenant]}·${id}` : id)
    const idx = idIndexOf(snap, (((await read($, newsModel)) as Record<string, NewsEntry>)[job.id])?.draft)
    const lt = (text: string, key: string, dim = false) => linkedText({ Box, Text, Button }, $, job.id, text, idx, key, 'jobs', { dimColor: dim })
    // an item row's name: its id as a link, then the title with any other board ids linked
    const itemName = (i: MwJobsItem, key: string) => (
      <Box key={`iname-${key}`} gap={1} overflow="hidden">
        <Button key={`iid-${key}`} plain label={pid(i.tenant, i.id)} onPress={() => void openDetail($, { kind: 'item', jobId: job.id, tenant: i.tenant, id: i.id }, 'jobs')} />
        {lt(i.title, `ititle-${key}`)}
      </Box>
    )
    const dot = (key: string) => (a0.changed.has(key) ? <Text color="suggestion">●</Text> : <Text> </Text>)
    // one grid for the whole pane: name | status column (as wide as a bar) | right column
    const draw = (trim: number): { node: any; size: number } => {
    const gone = (k: (typeof TRIM_ORDER)[number]) => TRIM_ORDER.indexOf(k) < trim
    const a: Analysis = { ...a0, needs: cap.needs.rows, review: cap.review.rows, blocked: cap.blocked.rows, doing: gone('progress') ? [] : cap.doing.rows }
    const mviews = mviewsAll.map(sec => (fullJobs ? { ...sec, rows: gone('metrics') ? [] : sec.rows, more: gone('metrics') ? sec.rows.length : 0 } : { ...sec, rows: gone('metrics') ? [] : sec.rows.slice(0, capN(sec.rows.length, METRIC_ROWS_MAX)), more: gone('metrics') ? sec.rows.length : Math.max(0, sec.rows.length - capN(sec.rows.length, METRIC_ROWS_MAX)) }))
    const closedShown = gone('milestones') ? [] : shownWaves
    const waves = closedShown
    const foldedText = tailOf(closedShown.length)
    const trimNote = trim > 0 ? '(trimmed to fit)' : ''
    const moreRow = groupMoreRow({ Box, Text, Button }, $, job.id, cap)
    const waveRows = closedShown.map(w => ({
      w,
      name: `${w.state === 'done' ? '✓ ' : ''}${`${waveName(w, labels)} ${w.title.split(' ').slice(1).join(' ')}`.trim()}`,
    }))
    const nameLens = [
      ...a.needs.map(i => `! ${i.title}`.length),
      ...a.review.map(i => `${pid(i.tenant, i.id)} ${i.title}`.length),
      ...a.doing.map(d => `${pid(d.item.tenant, d.item.id)} ${d.item.title}`.length),
      ...waveRows.map(r => r.name.length),
      ...mviews.flatMap(sec => sec.rows.map(r => r.name.length)),
    ]
    const rightLens = [
      3,
      2,
      ...closedShown.map(w => w.members.length),
      ...a.needs.map(i => itemAge(i).length),
      ...a.review.map(i => itemAge(i).length),
      ...a.doing.map(d => (d.ageMs === undefined ? 0 : ageText(d.ageMs).length)),
    ]
    // a metric row's count is one line: `good/total` (or the value) then the deltas; its width is the widest row's, so it never wraps
    const metricCount = (r: MetricRowView) => (r.ratio ? `${abbr(r.ratio.a)}/${abbr(r.ratio.b)}` : (r.valueText ?? ''))
    const cmax = Math.max(0, ...mviews.flatMap(sec => sec.rows.map(r => metricCount(r).length + r.delta.reduce((n, d) => n + d.text.length + 1, 0))))
    rightLens.push(cmax)
    const rightW = Math.max(...rightLens)
    // done / total as fixed Boxes: digits of a proportional font never line up as a plain string
    const doneWd = Math.max(1, ...closedShown.map(w => String(memberParts(w).done).length)) + 1
    const totalWt = Math.max(1, ...closedShown.map(w => String(memberParts(w).total).length)) + 1
    const countW = doneWd + 1 + totalWt
    const FIG = '\u2007'
    const countCell = (w: MwJobsWave) => {
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
    // blocks is the cell grid on every surface (Text in fixed-width cells, colours as Text props); normal is proportional
    const isMono = e.surface === 'terminal' || style === 'blocks'

    type Cell = { text?: string; color?: string; dim?: boolean; node?: unknown }
    const gridRow = (key: string, name: string, nameColor: string | undefined, status: Cell | undefined, right: unknown, marker: unknown, open?: () => void, minRight?: number, nameOverride?: unknown) => {
      const nameNode = nameOverride !== undefined ? (nameOverride as any) : open !== undefined ? <Button key={`open-${key}`} plain label={name} onPress={open} /> : <Text color={nameColor} wrap="truncate-end">{name}</Text>
      const statusNode = status === undefined ? null : status.node !== undefined ? (status.node as any) : (
        <Text color={status.color} dimColor={status.dim ?? status.color === undefined} wrap="wrap">{status.text}</Text>
      )
      const rightBox = (k?: string) => (
        <Box key={k} width={Math.max(minRight ?? 0, isMono ? Math.max(rightW, countW) : Math.max(6, countW))} flexShrink={0} flexGrow={0} justifyContent="flex-end">
          {right as any}
        </Box>
      )
      if (isCols) {
        return (
          <Box key={`row-${key}`} height={1} flexShrink={0} overflow="hidden">
            <Box key={`name-${key}`} width={isMono ? nameW : '38%'} flexShrink={0} flexGrow={0} overflow="hidden">
              {nameNode}
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
              {nameNode}
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
    const noteLine = (text: string, key = text) => (density === 'compact' || text === '' ? null : <Box key={`note-${key}`}><Text dimColor>{'  '}</Text>{lt(text, `nl-${key}`, true)}</Box>)
    const barCell = (w: MwJobsWave, isNew: boolean): Cell => {
      if (isMono) {
        const filled = Math.round((Math.max(0, Math.min(100, w.percent)) / 100) * barLen)
        return { node: meterBar({ Box, Text }, `bar-${w.id}`, [{ n: filled, color: isNew || w.percent >= 100 ? 'success' : 'suggestion' }], barLen, e.surface === 'terminal') }
      }
      if (canSvg) {
        const eb = epicBarSvg(w.percent, 96)
        return { node: <Svg key={`bar-${w.id}`} source={eb.source} alt={`${w.percent}%`} width={eb.width} /> }
      }
      return { text: '' }
    }
    const chipParts: string[] = []
    if (a.doneNew > 0) chipParts.push(`+${a.doneNew} done`)
    if (a.blockedNew > 0) chipParts.push(`+${a.blockedNew} blocked`)

    const summary = summaryParts(a, head, readyCount)

    const svgSize = svgSizeFor(step)
    if (style === 'gopher') {
      const gsize = svgSizeFor(step)
      const gcols = gopherCols(hasWidth ? given * 8 : undefined, gsize, canSvg, hasWidth ? given : undefined)
      if (gcols < GOPHER_MIN) {
        return { node: <Text color="warning">{`too narrow for gopher (needs ${GOPHER_MIN} columns)`}</Text>, size: 100 }
      }
      const stateOf = (i: MwJobsItem, fallback: string) => itemAge(i) || fallback
      const rowOf = (i: MwJobsItem, fallback: string) => ({ tenant: i.tenant, id: i.id, title: i.title, ticket: i.ticket, state: stateOf(i, fallback) })
      const view: NowView = {
        title: job.title,
        time: hhmm(snap.updatedAt),
        url: job.news || `gopher://local/1/now/${job.id}`,
        counts: brief ? countsOf(snap) : { needs: a0.needs.length, review: a0.review.length, blocked: a0.blocked.length, doing: a0.doing.length },
        groups: [
          { type: '!', title: 'NEEDS YOU', rows: a.needs.map(i => rowOf(i, 'needs you')), hidden: cap.needs.hidden },
          { type: '?', title: 'REVIEW', rows: a.review.map(i => rowOf(i, 'review')), hidden: cap.review.hidden },
          { type: '-', title: 'BLOCKED', rows: a.blocked.map(i => rowOf(i, 'blocked')), hidden: cap.blocked.hidden },
          { type: '+', title: 'IN PROGRESS', rows: a.doing.map(d => rowOf(d.item, 'doing')), hidden: cap.doing.hidden },
        ],
        miles: closedShown.map(w => waveToMile(w, {})),
        tail: foldedText,
        metrics: mviews,
        metricsNotice: snap.metricsNotice,
        notes: [...(stale !== undefined ? [`stale since ${hhmm(stale)}`] : []), ...(fillJobs && fullUnsupported.has(job.id) ? [FULL_HINT] : [])],
        footer: [`updated ${hhmm(snap.updatedAt)} - auto ${every}`, ...(trimNote ? [trimNote] : [])],
      }
      const lines = withTenantHidden(tsel !== '', () => nowSheet(view, gcols))
      return {
        node: (
          <Box flexDirection="column">
            {header}
            <Box key="gopher" flexDirection="column">
              {gopherNodes({ Box, Text, Button, Svg, Code }, lines, { canSvg, cols: gcols, size: gsize, key: 'now', ask: (tn, id) => void askInChat($, job, tn, id) })}
              {idsRow({ Box, Text, Button }, $, job.id, lines, idx, 'now', 'jobs')}
            </Box>
            {moreRow}
          </Box>
        ),
        size: canSvg ? buildBulletinSvg(lines, gsize, gcols).source.length : lines.reduce((n, l) => n + l.t.length + 1, 0),
      }
    }

    if (style === 'kanban' || style === 'agile') {
      const full = newsModels[job.id]?.draft
      const m = full === undefined ? undefined : tsel !== '' ? filterModel(full, tsel) : full
      if (m === undefined) {
        return { node: <Box flexDirection="column">{header}<Text color="warning">{noModelNotice(style, modelWhy.get(job.id)) ?? 'no news model'}</Text></Box>, size: 100 }
      }
      const noCap = fillJobs && !fullUnsupported.has(job.id)
      const doc = style === 'agile' ? agileJobs(m, noCap ? Number.MAX_SAFE_INTEGER : undefined) : kanbanJobs(m, noCap ? Number.MAX_SAFE_INTEGER : undefined)
      const ageOfS = (since: string) => {
        const t = Date.parse(since)
        return Number.isNaN(t) ? '' : ageText(now - t)
      }
      const node = withTenantHidden(tsel !== '', () => flowNodes({ Box, Text, Button, Svg, Link }, $, job, doc, { cols: style === 'agile' ? 1 : flowCols(hasWidth ? given : undefined), ageOf: ageOfS, keyPrefix: `j${style}`, ask: true, width: hasWidth ? given : undefined }))
      const est = flowText(doc).join('\n').length
      return {
        node: (
          <Box flexDirection="column">
            {header}
            {stale !== undefined && <Text color="warning">{`stale since ${hhmm(stale)}`}</Text>}
            {node}
            {trimNote && <Text dimColor>{trimNote}</Text>}
          </Box>
        ),
        size: est,
      }
    }


    const node = (
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

        {a.needs.length > 0 && (
          <Box flexDirection="column">
            {label('Needs you', 'error')}
            {a.needs.map(i => {
              const k = `${i.tenant}:${i.id}`
              return (
                <Box flexDirection="column">
                  {gridRow(`need-${k}`, `! ${i.title}`, undefined, { text: 'needs you', color: 'error' }, <Text dimColor>{itemAge(i)}</Text>, dot(k), undefined, undefined, <Box key={`iname-need-${k}`} gap={1} overflow="hidden"><Text color="error">!</Text>{lt(i.title, `ititle-need-${k}`)}</Box>)}
                  {noteLine([i.tenant, i.id, i.note].filter(Boolean).join(' · '), `need-${k}`)}
                </Box>
              )
            })}
          </Box>
        )}

        {a.review.length > 0 && (
          <Box flexDirection="column">
            {label('Review', 'warning')}
            {a.review.map(i => {
              const k = `${i.tenant}:${i.id}`
              return (
                <Box flexDirection="column">
                  {gridRow(`rev-${k}`, `${pid(i.tenant, i.id)} ${i.title}`, undefined, { text: 'awaiting review', color: 'warning' }, <Text dimColor>{itemAge(i)}</Text>, dot(k), undefined, undefined, itemName(i, `rev-${k}`))}
                  {noteLine(i.note === 'awaiting review' ? '' : i.note, `rev-${k}`)}
                </Box>
              )
            })}
          </Box>
        )}

        {a.blocked.length > 0 && (
          <Box flexDirection="column">
            {label('Blocked', 'error')}
            {a.blocked.map(i => {
              const k = `${i.tenant}:${i.id}`
              return (
                <Box flexDirection="column">
                  {gridRow(`blk-${k}`, `${pid(i.tenant, i.id)} ${i.title}`, undefined, { text: 'blocked', color: 'error' }, <Text dimColor>{itemAge(i)}</Text>, dot(k), undefined, undefined, itemName(i, `blk-${k}`))}
                  {noteLine(i.note, `blk-${k}`)}
                </Box>
              )
            })}
          </Box>
        )}

        {a.chat.length > 0 && (
          <Box flexDirection="column">
            {label('In chat')}
            {a.chat.map(i => {
              const k = `${i.tenant}:${i.id}`
              return gridRow(`chat-${k}`, `${pid(i.tenant, i.id)} ${i.title}`, undefined, { text: 'in chat' }, <Text dimColor>{itemAge(i)}</Text>, dot(k), undefined, undefined, itemName(i, `chat-${k}`))
            })}
          </Box>
        )}

        {a.doing.length > 0 && (
          <Box flexDirection="column">
            {label('In progress')}
            {a.doing.map(d => {
              const k = `${d.item.tenant}:${d.item.id}`
              const age = d.ageMs === undefined ? '' : ageText(d.ageMs)
              return gridRow(
                `do-${k}`,
                `${pid(d.item.tenant, d.item.id)} ${d.item.title}`,
                undefined,
                d.isStalled ? { text: 'stalled', color: 'warning' } : isCols ? { text: '' } : undefined,
                <Text color={d.isStalled ? 'warning' : undefined} dimColor={!d.isStalled}>{age}</Text>,
                dot(k),
                undefined,
                undefined,
                itemName(d.item, `do-${k}`),
              )
            })}
          </Box>
        )}

        {(waves.length > 0 || foldedText !== '') && (
          <Box flexDirection="column">
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
              return gridRow(`wav-${w.id}`, name, undefined, status, countCell(w), marker, () => void openDetail($, { kind: 'wave', jobId: job.id, tenant: w.id.split(':')[0] ?? '', id: w.id }, 'jobs'))
            })}
            {foldedText !== '' && (openCount - closedShown.length > 0 ? <Button key="miles-more" plain dimColor label={foldedText} onPress={() => void setFill($, 'jobs', true)} /> : <Text dimColor wrap="truncate-end" {...sz("footnote")}>{foldedText}</Text>)}
          </Box>
        )}

        {snap.metricsNotice && <Text dimColor>{snap.metricsNotice}</Text>}
        {density !== 'compact' &&
          mviews.map(sec => (
            <Box flexDirection="column">
              {label(sec.title)}
              {sec.rows.map(r => {
                const tcol = (t: string) => (t === 'green' ? 'success' : t === 'red' ? 'error' : t === 'amber' ? 'warning' : undefined)
                // narrow pane: the bar shrinks, the count never does
                const blen = Math.max(4, Math.min(isMono ? barLen : 10, hasWidth ? columns - nameW - cmax - 5 : 99))
                // coloured segments (cumulative rounding, so the parts and the track always add up to the bar)
                const segs: Array<{ n: number; tone: string }> = []
                let used = 0
                let acc = 0
                if (r.showBar) {
                  for (const pt of r.parts) {
                    acc += pt.f
                    const upto = Math.min(blen, Math.round(acc * blen))
                    const n = Math.max(0, upto - used)
                    used += n
                    segs.push({ n, tone: pt.tone })
                  }
                }
                const barNode = r.showBar ? meterBar({ Box, Text }, `mbar-${sec.title}-${r.name}`, segs.map(sg => ({ n: sg.n, color: tcol(sg.tone) ?? 'suggestion' })), blen, e.surface === 'terminal') : undefined
                const dim = r.barNote
                const countStr = metricCount(r)
                const used0 = countStr.length + r.delta.reduce((n, d) => n + d.text.length + 1, 0)
                const arrows = (
                  <Text key={`mcount-${sec.title}-${r.name}`} wrap="truncate-end">
                    {' '.repeat(Math.max(0, cmax - used0))}
                    {r.ratio ? <Text dimColor>{countStr}</Text> : <Text color={tcol(r.valueTone)}>{countStr}</Text>}
                    {r.delta.map(d => <Text color={tcol(d.tone)}>{` ${d.text}`}</Text>)}
                  </Text>
                )
                return gridRow(`met-${sec.title}-${r.name}`, r.name, undefined, barNode ? { node: barNode } : { text: dim, dim: true }, arrows, <Text>{'  '}</Text>, undefined, cmax)
              })}
              {(sec.more ?? 0) > 0 && <Button key={`rows-more-${sec.title}`} plain dimColor label={`+${sec.more} more rows`} onPress={() => void setFill($, 'jobs', true)} />}
              {(sec.note || sec.legend.length > 0) && <Text dimColor wrap="wrap">{[sec.legend.map(l => l.name).join(' · '), sec.note].filter(Boolean).join(' — ')}</Text>}
            </Box>
          ))}

        {events.length > 0 && (
          <Box flexDirection="column">
            {label('Last events')}
            {events.map(ev => (
              <Box gap={1}>
                <Text {...mo} dimColor {...sz("footnote")}>{isoHhmm(ev.ts)}</Text>
                <Text dimColor wrap="truncate-end" {...sz("footnote")}>{`${ev.tenant} ${ev.item} ${ev.action}${ev.note ? ` — ${ev.note.slice(0, 60)}` : ''}`}</Text>
              </Box>
            ))}
          </Box>
        )}

        {askRow({ Box, Text, Button }, $, job, [...a.needs, ...a.review, ...a.blocked])}
        {moreRow}
        <Box flexDirection="column">
          {separator}
          <Box>
            <Text dimColor {...sz("footnote")}>{`updated ${hhmm(snap.updatedAt)} · auto ${every}${trimNote ? ` · ${trimNote}` : ''}`}</Text>
          </Box>
        </Box>
      </Box>
    )
    // the terminal has no Svg: the budget counts the characters the rows would print
    const rowChars = (i: MwJobsItem) => i.title.length + i.note.length + i.id.length + 40
    const size =
      [...a.needs, ...a.review, ...a.blocked, ...a.doing.map(d => d.item)].reduce((n, i) => n + rowChars(i), 0) +
      waveRows.reduce((n, r) => n + r.name.length + 40, 0) +
      mviews.reduce((n, sec) => n + sec.title.length + (sec.note?.length ?? 0) + 120 + sec.rows.reduce((m2, r) => m2 + r.name.length + 80, 0), 0)
    return { node, size }
    }
    // build it, and give up the lowest-priority sections one by one while it stays above the budget (never refused)
    const limitFor = TEXT_BUDGET
    let drawn = draw(0)
    for (let t = 1; !fullJobs && t <= TRIM_ORDER.length && drawn.size > limitFor; t++) drawn = draw(t)
    return drawn.node
  }))
}
