import { expect, test } from 'claude-code/testing'

import { attentionSort, cleanFavorites, countsText, fitCols, fitMid, inRange, listFor, moveIn, newsLine, paged, parseFavorites, pickLine, planBar, pollPlan, rrSlice, summaryLine, tabWidth, toggleIn } from './switcher'
import type { Entry, Tab } from './switcher'
import { abbr, cellWidth, cutCells, graphemes, safeSlice, INVISIBLE } from './rows'
import { cleanBoard } from './register'

const entry = (id: string, needs = 0, review = 0, blocked = 0, last = 0): Entry => ({ id, title: `Title of ${id}`, needs, review, blocked, doing: 0, done: 3, total: 10, last })
const tabs = (n: number, label = (i: number) => `job${i}`): Tab[] => Array.from({ length: n }, (_, i) => ({ id: label(i), label: label(i), needs: 0, review: 0 }))

test('attention order: needs, review, blocked, last change, id', () => {
  const rows = [entry('b', 0, 0, 0, 5), entry('a', 0, 0, 0, 9), entry('c', 1), entry('d', 1, 2), entry('e', 0, 3), entry('f', 0, 0, 2)]
  expect(attentionSort(rows).map(r => r.id)).toEqual(['d', 'c', 'e', 'f', 'a', 'b'])
  expect(attentionSort([entry('b'), entry('a')]).map(r => r.id)).toEqual(['a', 'b'])
})

test('the top bar: 3 items that fit are all tabs; 12 fit at 1200 px but not at 320 px; 1000 never do', () => {
  const three = planBar({ tabs: tabs(3), current: 'job0', favs: [], cols: fitCols(320 / 8, 12), lead: 4 })
  expect(three.mode).toBe('all')
  const twelve = tabs(12)
  expect(planBar({ tabs: twelve, current: 'job0', favs: [], cols: fitCols(1200 / 8, 12), lead: 4 }).mode).toBe('all')
  const narrow = planBar({ tabs: twelve, current: 'job3', favs: [], cols: fitCols(320 / 8, 12), lead: 4 })
  expect(narrow.mode).toBe('compact')
  if (narrow.mode === 'compact') {
    // the automatic quick tabs: at most 3, never the current one, and the group fits the width
    expect(narrow.tabs.length).toBeLessThanOrEqual(3)
    expect(narrow.tabs.map(t => t.id)).not.toContain('job3')
    expect(narrow.count).toBe(12)
    expect(narrow.plus).toBe(0)
  }
  const big = planBar({ tabs: tabs(1000, i => `t${String(i).padStart(4, '0')}`), current: 't0007', favs: [], cols: fitCols(1200 / 8, 12), lead: 10 })
  expect(big.mode).toBe('compact')
  // the size step changes the fit: bigger text, fewer columns
  expect(fitCols(40, 14)).toBeLessThan(fitCols(40, 12))
})

test('favorites replace the automatic tabs, fit the width and overflow into +N', () => {
  const t = tabs(40)
  const favs = ['job5', 'job9', 'job12', 'job20', 'job30', 'job31']
  const p = planBar({ tabs: t, current: 'job1', favs, cols: 40, lead: 4 })
  expect(p.mode).toBe('compact')
  if (p.mode === 'compact') {
    expect(p.tabs.map(x => x.id)).toEqual(favs.slice(0, p.tabs.length))
    expect(p.plus).toBe(favs.length - p.tabs.length)
    expect(p.plus).toBeGreaterThan(0)
    const used = 4 + p.tabs.reduce((n, x) => n + tabWidth(x), 0)
    expect(used).toBeLessThanOrEqual(40)
  }
  const wide = planBar({ tabs: t, current: 'job1', favs, cols: 150, lead: 4 })
  expect(wide.mode).toBe('compact')
  if (wide.mode === 'compact') {
    expect(wide.plus).toBe(0)
    expect(wide.tabs.map(x => x.id)).toEqual(favs)
  }
  // a favorite that no longer exists is not a tab
  const gone = planBar({ tabs: t, current: 'job1', favs: ['nope', 'job5'], cols: 80, lead: 4 })
  if (gone.mode === 'compact') expect(gone.tabs.map(x => x.id)).toEqual(['job5'])
})

test('pages of 20, letter jumps, needs first in the picker, favorites in starred order', () => {
  const rows = Array.from({ length: 1000 }, (_, i) => entry(`${'abcdefghijklmnopqrstuvwxyz0123456789'[i % 36]}${String(i).padStart(4, '0')}`, i < 3 ? 1 : 0))
  const pk = listFor(rows, { view: 'pick', page: 0 }, [], true)
  expect(pk.needs).toBe(3)
  expect(pk.rows).toHaveLength(1000)
  expect(paged(pk.rows, 0).rows).toHaveLength(20)
  expect(paged(pk.rows, 49).rows).toHaveLength(20)
  expect(paged(pk.rows, 99).page).toBe(49)
  expect(paged(pk.rows, 1).page).toBe(1)
  const af = listFor(rows, { view: 'pick', page: 0, range: 'a-f' }, [], true).rows
  expect(af.every(r => inRange(r.id, 'a-f'))).toBe(true)
  expect(af.map(r => r.id)).toEqual([...af.map(r => r.id)].sort())
  const digits = listFor(rows, { view: 'pick', page: 0, range: '0-9' }, [], true).rows
  expect(digits.length).toBeGreaterThan(200)
  expect(digits.every(r => /^[0-9]/.test(r.id))).toBe(true)
  const total = ['a-f', 'g-l', 'm-r', 's-z', '0-9'].reduce((n, k) => n + listFor(rows, { view: 'pick', page: 0, range: k }, [], true).rows.length, 0)
  expect(total).toBe(1000)
  const favs = listFor(rows, { view: 'pick', page: 0, fav: true }, [rows[9]!.id, rows[2]!.id], true).rows
  expect(favs.map(r => r.id)).toEqual([rows[9]!.id, rows[2]!.id])
})

test('favorites: toggle, move, store shape, and a deleted job or tenant is dropped quietly', () => {
  expect(toggleIn(['a'], 'b')).toEqual(['a', 'b'])
  expect(toggleIn(['a', 'b'], 'a')).toEqual(['b'])
  expect(moveIn(['a', 'b', 'c'], 'b', -1)).toEqual(['b', 'a', 'c'])
  expect(moveIn(['a', 'b', 'c'], 'a', -1)).toEqual(['a', 'b', 'c'])
  const f = parseFavorites({ jobs: ['x', 'y', 'x', 5], tenants: { x: ['t1', 't2'], gone: ['t'], y: [] } })
  expect(f).toEqual({ jobs: ['x', 'y'], tenants: { x: ['t1', 't2'], gone: ['t'] } })
  expect(parseFavorites('junk')).toEqual({ jobs: [], tenants: {} })
  expect(cleanFavorites(f, ['x'], jid => (jid === 'x' ? ['t2'] : undefined))).toEqual({ jobs: ['x'], tenants: { x: ['t2'] } })
  expect(cleanFavorites(f, ['x', 'y'], () => undefined)).toEqual({ jobs: ['x', 'y'], tenants: { x: ['t1', 't2'] } })
})

test('rows: one line per job or tenant with the same counts, cut to the width, dot leaders', () => {
  const e = { ...entry('mw', 2, 1, 3), doing: 4, last: Date.parse('2026-10-22T14:02:00') }
  const line = summaryLine(e, 70)
  expect(line).toHaveLength(70)
  expect(line.startsWith('!2  ◐1  ⛔3  ▶4  ')).toBe(true)
  expect(line).toContain('mw')
  expect(line).toContain('3/10 [##......]')
  expect(line.trimEnd().endsWith('14:02') || line.includes('14:02')).toBe(true)
  expect(summaryLine(e, 70, 'gopher').startsWith('1 !2  ?1  -3  +4  ')).toBe(true)
  expect(summaryLine(e, 70, 'loz')).toContain('[!2][◐1][⛔3][▶4]')
  expect(countsText(e)).toContain('!2')
  // long titles and keys are cut with ~, columns stay aligned
  const long = { ...e, id: 'a-very-long-tenant-slug', title: 'x'.repeat(200) }
  const l2 = summaryLine(long, 70)
  expect(l2).toHaveLength(70)
  expect(l2).toContain('~')
  expect(pickLine(e, 60, '14:02')).toHaveLength(60)
  expect(newsLine({ ...e, news: 'release 7 (2026-10-22) · draft: 3 changes' }, 90)).toContain('release 7 (2026-10-22) · draft: 3 changes  !2')
  expect(fitMid('ab ', 'title', ' r', 20)).toHaveLength(20)
})

test('polling: the current and quick-tab jobs every poll, the others round-robin at most 5, none starves', () => {
  const ids = Array.from({ length: 100 }, (_, i) => `j${i}`)
  const polled = new Map<string, number>()
  const seen = new Set<string>()
  let seq = 0
  for (let poll = 0; poll < 25; poll++) {
    const plan = pollPlan(ids, ['j7', 'j8'], polled)
    expect(plan.length).toBeLessThanOrEqual(2 + 5)
    expect(plan).toContain('j7')
    seq += 1
    for (const id of plan) {
      polled.set(id, seq)
      seen.add(id)
    }
  }
  expect(seen.size).toBe(100)
  expect(pollPlan(['a', 'b'], ['a'], new Map())).toEqual(['a', 'b'])
  const rr = rrSlice(Array.from({ length: 25 }, (_, i) => i), 20, 10)
  expect(rr.slice).toEqual([20, 21, 22, 23, 24, 0, 1, 2, 3, 4])
  expect(rr.cursor).toBe(5)
  expect(rrSlice([1, 2, 3], 2, 10).slice).toEqual([1, 2, 3])
})

test('S7: counts that round up to the next unit use it', () => {
  expect(abbr(999_950)).toBe('1M')
  expect(abbr(999_949)).toBe('999.9k')
  expect(abbr(12_345)).toBe('12.3k')
  expect(abbr(999_950_000)).toBe('1B')
  expect(abbr(9_999)).toBe('9999')
  expect(abbr(-999_950)).toBe('-1M')
  expect(abbr(1e308)).toBe('1e308')
})

test('S4: text is measured and cut by grapheme and display width, and a surrogate pair is never split', () => {
  expect(cellWidth('abc')).toBe(3)
  expect(cellWidth('\u65e5\u672c\u8a9e')).toBe(6)
  expect(cellWidth('e\u0301')).toBe(1)
  expect(cellWidth('\u{1F600}')).toBe(2)
  expect(cutCells('\u65e5\u672c\u8a9e', 5)).toBe('\u65e5\u672c')
  expect(cutCells('a\u{1F600}b', 2)).toBe('a')
  expect(cutCells('a\u{1F600}b', 3)).toBe('a\u{1F600}')
  // a cut never ends in half a pair or between a letter and its accent
  for (let n = 0; n < 8; n++) {
    const out = safeSlice('x\u{1F600}\u{1F468}\u200d\u{1F469}e\u0301y', n)
    expect(out.length === 0 || !/[\ud800-\udbff]$/.test(out)).toBe(true)
    expect(graphemes(out).join('')).toBe(out)
  }
})

test('S5: bidi controls and zero-width characters are stripped from board text', () => {
  const dirty = 'ok\u202eevil\u202c\u200bhid\u2066den\u2069\ufeff'
  expect(cleanBoard(dirty)).toBe('okevilhidden')
  expect(INVISIBLE.test('a\u200fb')).toBe(true)
  INVISIBLE.lastIndex = 0
  expect(cleanBoard('a\u200b\u200d\u200eb')).toBe('ab')
})
