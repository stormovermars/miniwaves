// Stress test for the jobs and news panes: real `mw` output captured from the stress board
// (1000 waves, ~3000 items, 250 attention items, 2000+ events) fed through mocked process.run.
// It measures and prints; it only fails on a crash. Results go to stdout as `@@STRESS {json}` and
// `@@SVG {json}` lines; gen_stress.py (phase extract) parses them into $SCR/svg, $SCR/png and the tables.
import { expect, mock, test } from 'claude-code/testing'

import { askGroupText, statusSegment } from './register'
import { D } from './fixtures/stress/data'

declare const console: { log: (...a: unknown[]) => void }
import INVALID from './fixtures/stress/invalid'
import LIST0 from './fixtures/stress/list0'
import LIST1 from './fixtures/stress/list1'

const SVG_MAX = 131072
const MD_MAX = 10000
const PANE = {
  plugin: 'mw-jobs',
  component: 'Pane',
  requestId: 'mw-jobs',
  props: { title: 'Jobs', isFocused: false, bodyColumns: 80, placement: 'dock', scroll: { offset: 0, bodyRows: 30 }, view: {} },
} as const
const NEWS_PANE = { ...PANE, requestId: 'mw-news', props: { ...PANE.props, title: 'mw news' } } as const

const DESC300 = ('Synthetic 300-character job description with unicode and emoji ✅ 日本語. ' + 'lorem ipsum dolor sit amet '.repeat(20)).slice(0, 300)
const BASE = Date.parse('2026-10-22T10:00:00')

const REG = (metrics: boolean) => ({
  jobs: [
    {
      id: 'stress', title: 'Stress', board: '~/apps/stress', tenants: ['a', 'b', 'c'], news: 'https://example.com/news/stress', summary: '',
      description: DESC300, coordinator: 'c', desk: 'd', alerts: { sound: true, notify: true },
      ...(metrics ? { metrics: 'docs/news/metrics.json' } : {}),
    },
  ],
})

const FULL_LIST = { items: [...LIST0, ...LIST1] as Array<Record<string, unknown>>, waves: D.listWaves as unknown[] }
const FULL_JSON = JSON.stringify(FULL_LIST)
const COLD_LIST = JSON.stringify({
  ...FULL_LIST,
  items: FULL_LIST.items.map(i => (i.state === 'blocked' ? { ...i, state: 'doing', note: '' } : i)),
})
const FULL_REVIEW = D.review
const COLD_REVIEW = JSON.stringify({ needs: [] })
const jobShow = JSON.stringify({ ...JSON.parse(D.jobShow), description: DESC300 })

const WAVES_ALL = JSON.parse(D.waveList) as Array<Record<string, any>>
const subsets = new Map<number, { waves: string; list: string; review: string; items: any[]; wavesArr: any[] }>()
/** The first N waves of the stress board, with their members and the asks that belong to them (all items without a wave stay). */
const subset = (n: number) => {
  let r = subsets.get(n)
  if (r) return r
  const keep = new Set(WAVES_ALL.slice(0, n).map(w => String(w.id)))
  const items = (FULL_LIST.items as any[]).filter(i => !i.wave || keep.has(`${i.tenant}:${i.wave}`))
  const ids = new Set(items.map(i => `${i.tenant}:${i.id}`))
  const needs = (JSON.parse(D.review).needs as any[]).filter(a => ids.has(`${a.tenant}:${a.item}`))
  r = { waves: JSON.stringify(WAVES_ALL.slice(0, n)), list: JSON.stringify({ items, waves: [] }), review: JSON.stringify({ needs }), items, wavesArr: WAVES_ALL.slice(0, n) }
  subsets.set(n, r)
  return r
}

const textFor = (argv: string[]) => {
  const w = Number(argv[argv.indexOf('--width') + 1] || 80)
  return w >= 140 ? D.newsText160 : w >= 100 ? D.newsText120 : D.newsText80
}

const rig = ($: any, on: any, o: { metrics?: string; hot?: boolean; n?: number } = {}) => {
  const state = {
    hot: o.hot ?? true, toasts: [] as string[], notifies: [] as string[], statuses: [] as string[], sounds: [] as string[], fills: [] as string[],
    store: new Map<string, unknown>(), files: new Map<string, string>(), runs: [] as string[], opens: [] as string[], procMs: [] as number[],
  }
  const ok = (stdout: string) => ({ value: { exitCode: 0, stdout, stderr: '', isStdoutTruncated: false, isStderrTruncated: false } })
  const clock = mock.clock(on, { now: BASE })
  on('store.get', (_: any, e: any) => ({ value: state.store.get(e.key) }))
  on('store.set', (_: any, e: any) => { state.store.set(e.key, e.value); return { value: undefined } })
  on('command.register', () => ({ value: { command: 'x' } }))
  on('ui.open', (_: any, e: any) => { state.opens.push(e.id); return { value: { isPlaced: true } } })
  on('prompt.fill', (_: any, e: any) => { state.fills.push(String(e.text)); return { isFilled: true } })
  on('prompt.submit', () => ({ drop: 'test' }))
  on('ui.close', () => ({ value: undefined }))
  on('session.start', (_: any, e: any) => ({ cwd: e.cwd }))
  on('env.get', () => ({ value: '/home' }))
  on('fs.write', (_: any, e: any) => { state.files.set(String(e.path), (state.files.get(String(e.path)) ?? '') + String(e.text)); return { value: undefined } })
  on('fs.read', (_: any, e: any) => {
    const p = String(e.path)
    if (state.files.has(p)) return { value: state.files.get(p) }
    if (p.endsWith('/.mw/jobs.json')) return { value: JSON.stringify(REG(o.metrics !== undefined)) }
    if (p.endsWith('/docs/news/metrics.json') && o.metrics !== undefined) return { value: o.metrics }
    if (p.endsWith('/docs/news/status.md')) return { value: D.newsMd }
    return { deny: 'ENOENT' }
  })
  on('fs.stat', (_: any, e: any) => ({ value: { kind: 'file', size: 1, mtimeMs: 0, isLink: false, realPath: e.path } }))
  on('ui.log', () => ({ value: undefined }))
  on('ui.toast', (_: any, e: any) => { state.toasts.push(String(e.text)); return { value: undefined } })
  on('classic.Notification', (_: any, e: any) => { state.notifies.push(String(e.message ?? e.text ?? '')); return {} })
  on('ui.status', (_: any, e: any) => { state.statuses.push(String(e.text)); return { value: undefined } })
  on('audio.play', (_: any, e: any) => { state.sounds.push(JSON.stringify(e)); return { value: undefined } })
  on('process.run', (_: any, e: any) => {
    const cmd = e.argv.join(' ')
    state.runs.push(cmd)
    if (e.argv[0] === 'osascript') return ok('')
    if (cmd.startsWith('mw log') && e.argv.includes('--since')) return ok(JSON.stringify({ events: [], next: 'C1' }))
    if (cmd.startsWith('mw wave list')) return ok(o.n === undefined ? D.waveList : subset(o.n).waves)
    if (cmd.startsWith('mw list')) return ok(o.n !== undefined ? subset(o.n).list : state.hot ? FULL_JSON : COLD_LIST)
    if (cmd.startsWith('mw log')) return ok(D.log40)
    if (cmd.startsWith('mw review')) {
      // mw reads one tenant per call: bare = the default tenant (none of the job's); -t SLUG = that tenant's asks
      const ti = e.argv.indexOf('-t')
      const src = JSON.parse(o.n !== undefined ? subset(o.n).review : state.hot ? FULL_REVIEW : COLD_REVIEW) as { needs: Array<{ tenant: string }> }
      return ok(JSON.stringify({ needs: ti < 0 ? [] : src.needs.filter(a => a.tenant === e.argv[ti + 1]) }))
    }
    if (cmd.startsWith('mw jobs show')) return ok(jobShow)
    if (cmd.startsWith('mw news')) return ok(e.argv.includes('text') ? textFor(e.argv) : D.newsMd)
    return ok('[]')
  })
  return { state, clock }
}

const JOB = REG(false).jobs[0] as any
const snapOf = (items: any[], waves: any[], asks: any[]) => ({ waves, items, events: [], updatedAt: 0, asks }) as any
const fullStatus = () => statusSegment(JOB, snapOf(FULL_LIST.items as any[], WAVES_ALL, JSON.parse(D.review).needs), undefined)

const emit = (tag: 'STRESS' | 'SVG', o: unknown) => console.log(`@@${tag} ${JSON.stringify(o)}`)
const now = () => performance.now()
const r1 = (n: number) => Math.round(n * 10) / 10

const measureMount = async ($: any, state: ReturnType<typeof rig>['state'], pane: typeof PANE | typeof NEWS_PANE, surface: 'desktop' | 'terminal', style: string, width: number) => {
  const which = pane === PANE ? 'jobs' : 'news'
  const name = `${which}-${style}-${surface}-w${width}`
  const rec: Record<string, unknown> = { kind: 'pane', name, pane: which, style, surface, width }
  const t0 = now()
  let ui: any
  try {
    ui = await $.ui.mount({ ...pane, props: { ...pane.props, bodyColumns: width }, surface })
  } catch (e) {
    rec.mountError = String(e).slice(0, 300)
    emit('STRESS', rec)
    return
  }
  rec.renderMs = r1(now() - t0)
  let all: any[] = []
  try {
    all = await ui.findAll({})
  } catch (e) {
    rec.findAllError = String(e).slice(0, 200)
  }
  const byType: Record<string, number> = {}
  for (const el of all) byType[el.type] = (byType[el.type] ?? 0) + 1
  rec.elements = all.length
  rec.byType = byType
  const svgs = all.filter(el => el.type === 'Svg')
  rec.svgs = svgs.map((s, i) => {
    const src = String(s.props.source ?? '')
    const sname = `${name}-${i}`
    emit('SVG', { name: sname, source: src })
    return { name: sname, bytes: src.length, overCap: src.length > SVG_MAX, width: s.props.width, height: s.props.height, altLen: String(s.props.alt ?? '').length }
  })
  const mds = all.filter(el => el.type === 'Markdown')
  rec.markdown = mds.map(m => ({ len: String(m.props.text ?? m.text ?? '').length, overCap: String(m.props.text ?? m.text ?? '').length > MD_MAX }))
  const texts = all.filter(el => el.type === 'Text').map(el => String(el.text ?? ''))
  rec.textCount = texts.length
  rec.textMaxLen = texts.reduce((m, t) => Math.max(m, t.length), 0)
  rec.textOver200 = texts.filter(t => t.length > 200).length
  const buttons = all.filter(el => el.type === 'Button')
  rec.buttons = buttons.length
  rec.askLabels = { needs: (await ui.find({ key: 'ask-needs' }))?.props.label, review: (await ui.find({ key: 'ask-review' }))?.props.label }
  rec.statusLen = state.statuses.length > 0 ? state.statuses[state.statuses.length - 1]!.length : -1
  // metrics section visible anywhere?
  const hay = svgs.map(s => String(s.props.source)).join('\n') + texts.join('\n')
  rec.showsMetrics = /Section 0|metrics file invalid|Harness/.test(hay) && /Section 0|metrics file invalid/.test(hay)
  rec.metricsInvalidNotice = /metrics file invalid/.test(hay)
  await ui.unmount()
  emit('STRESS', rec)
}

for (const surface of ['desktop', 'terminal'] as const) {
  for (const style of ['normal', 'blocks'] as const) {
    test(`stress panes: ${style} on ${surface}`, async ($, on) => {
      const { state } = rig($, on)
      const t0 = now()
      await $.session.start({ cwd: '/', surface, isInteractive: true })
      emit('STRESS', { kind: 'session-start', style, surface, ms: r1(now() - t0), statusShown: state.statuses[state.statuses.length - 1], statusWouldBeLen: fullStatus().length, statusWouldBeHead: fullStatus().slice(0, 240), toasts: state.toasts.length, notifies: state.notifies.length })
      await $.command.run({ command: 'mw-jobs', args: `style ${style}`, origin: { kind: 'user' }, presentation: {} } as never)
      for (const width of [40, 80, 160]) {
        await measureMount($, state, PANE, surface, style, width)
        await measureMount($, state, NEWS_PANE, surface, style, width)
      }
      expect(true).toBe(true)
    })
  }
}

// every attention item appears at once: a quiet snapshot first (blocked items still "doing", no asks), then the full board on the next tick
for (const mode of ['notify', 'toast+notify', 'toast'] as const) {
  test(`stress alerts: 250 attention items at once, alertNeeds=${mode}`, { options: { alertNeeds: mode, alertMilestone: mode, alertNew: mode, alertDone: mode } }, async ($, on) => {
    const { state, clock } = rig($, on, { hot: false })
    await $.session.start({ cwd: '/', surface: 'desktop', isInteractive: true })
    const base = { toasts: state.toasts.length, notifies: state.notifies.length, sounds: state.sounds.length }
    state.hot = true
    await clock.advance(60_000)
    let d: any
    let mountError = ''
    try {
      d = await $.ui.mount({ ...PANE, surface: 'desktop' } as never)
    } catch (e) {
      mountError = String(e).slice(0, 400)
    }
    const needs = d ? await d.find({ key: 'ask-needs' }) : undefined
    const review = d ? await d.find({ key: 'ask-review' }) : undefined
    const logText = [...state.files.entries()].filter(([p]) => p.endsWith('alerts.log')).map(([, v]) => v).join('')
    const delivered: Record<string, number> = {}
    for (const l of logText.split('\n').filter(Boolean)) {
      const k = `${l.split('  ')[1] ?? '?'}/${(l.match(/->\s*(\S+)\s*$/) ?? [])[1] ?? '?'}`
      delivered[k] = (delivered[k] ?? 0) + 1
    }
    const rec: Record<string, unknown> = {
      kind: 'alerts', mode, mountError, toastCount: state.toasts.length - base.toasts, notifyCount: state.notifies.length - base.notifies, sounds: state.sounds.length - base.sounds,
      toastLens: state.toasts.map(t => t.length), notifyLens: state.notifies.map(t => t.length), toastSample: state.toasts.slice(0, 2), notifySample: state.notifies.slice(0, 3),
      needsLabel: needs?.props.label, reviewLabel: review?.props.label,
      statusShown: state.statuses[state.statuses.length - 1],
      statusWouldBe: fullStatus().length,
      delivered,
      alertsLogBytes: [...state.files.entries()].filter(([p]) => p.endsWith('alerts.log')).reduce((n, [, v]) => n + v.length, 0),
      alertsLogLines: [...state.files.entries()].filter(([p]) => p.endsWith('alerts.log')).reduce((n, [, v]) => n + v.split('\n').filter(Boolean).length, 0),
    }
    if (mode === 'notify') {
      // the filled !N and ◐N questions: through the pane when it draws, and straight from askGroupText on the same data
      if (d) {
        for (const key of ['ask-needs', 'ask-review']) {
          const before = state.fills.length
          await d.press({ key })
          const t = state.fills[state.fills.length - 1] ?? ''
          rec[`fill_${key}`] = { filled: state.fills.length > before, chars: t.length, lines: t.split('\n').length, head: t.slice(0, 300) }
          await clock.advance(2500)
        }
      }
      const asks = (JSON.parse(D.review).needs as Array<Record<string, string>>)
      const askKeys = new Set(asks.map(a => `${a.tenant}:${a.item}`))
      const isRev = (i: Record<string, unknown>) => i.state === 'blocked' && /awaiting review/i.test(String(i.note))
      const its = FULL_LIST.items as never as any[]
      const needItems = its.filter(i => i.state !== 'done' && !isRev(i) && (i.state === 'blocked' || askKeys.has(`${i.tenant}:${i.id}`)))
      const revItems = its.filter(isRev)
      const snap = { waves: [], items: its, events: [], updatedAt: 0, asks } as any
      const nt = askGroupText('needs', needItems, snap)
      const rt = askGroupText('review', revItems, snap)
      rec.direct = { needItems: needItems.length, reviewItems: revItems.length, needsChars: nt.length, needsLines: nt.split('\n').length, reviewChars: rt.length, reviewLines: rt.split('\n').length, needsHead: nt.slice(0, 400), reviewHead: rt.slice(0, 400) }
    }
    if (d) await d.unmount()
    emit('STRESS', rec)
    expect(true).toBe(true)
  })
}

// metrics.json through the registry's `metrics` key: max, invalid, bad numbers. The jobs pane runs on the first 25 waves (the full board is refused, see the scale test); news on the full board.
for (const [label, body] of [['max', D.metricsMax], ['invalid', INVALID], ['badnumbers', D.metricsBadNumbers]] as const) {
  test(`stress metrics: ${label}`, async ($, on) => {
    const { state } = rig($, on, { metrics: body, n: 25 })
    await $.session.start({ cwd: '/', surface: 'desktop', isInteractive: true })
    await $.command.run({ command: 'mw-jobs', args: 'style normal', origin: { kind: 'user' }, presentation: {} } as never)
    const rec: Record<string, unknown> = { kind: 'metrics', label, statuses: state.statuses.length }
    for (const [which, pane] of [['jobs', PANE], ['news', NEWS_PANE]] as const) {
      const t0 = now()
      try {
        const u = await $.ui.mount({ ...pane, surface: 'desktop' } as never)
        const src = String((await u.find({ type: 'Svg' }))?.props.source ?? '')
        rec[which] = { ms: r1(now() - t0), svgBytes: src.length, hasMetrics: /Section [0-7]/.test(src), invalidNotice: (src.match(/metrics file invalid[^<]*/) ?? [])[0] ?? null, newerNotice: /newer metrics/.test(src) }
        emit('SVG', { name: `metrics-${label}-${which}`, source: src })
        await u.unmount()
      } catch (e) {
        rec[which] = { mountError: String(e).slice(0, 300) }
      }
    }
    emit('STRESS', rec)
    expect(true).toBe(true)
  })
}

// how the jobs pane scales: the first N waves of the board, N = 10 .. 1000; where does a surface refuse it?
for (const surface of ['desktop', 'terminal'] as const) {
  for (const style of ['normal', 'blocks'] as const) {
    for (const n of [10, 25, 50, 100, 150, 200, 300, 400, 550, 700, 850, 1000]) {
      test(`stress scale: jobs pane, ${style} on ${surface}, ${n} waves`, async ($, on) => {
        rig($, on, { n })
        const t0 = now()
        const rec: Record<string, unknown> = { kind: 'scale', style, surface, n, items: subset(n).items.length }
        try {
          await $.session.start({ cwd: '/', surface, isInteractive: true })
          await $.command.run({ command: 'mw-jobs', args: `style ${style}`, origin: { kind: 'user' }, presentation: {} } as never)
          const ui = await $.ui.mount({ ...PANE, surface } as never)
          rec.renderMs = r1(now() - t0)
          const svgs = (await ui.findAll({ type: 'Svg' })).map((x: any) => String(x.props.source))
          rec.svgBytes = svgs.map(x => x.length)
          rec.elements = (await ui.findAll({})).length
          rec.textChars = (await ui.findAll({ type: 'Text' })).reduce((m: number, x: any) => m + String(x.text ?? '').length, 0)
          if (n === 25 || n === 100) svgs.forEach((x, i) => emit('SVG', { name: `jobs-scale-${style}-${surface}-n${n}-${i}`, source: x }))
          await ui.unmount()
        } catch (e) {
          rec.refused = String(e).split('\n')[0]!.slice(0, 200)
        }
        const snap = subset(n)
        rec.statusLen = statusSegment(JOB, snapOf(snap.items, snap.wavesArr, JSON.parse(snap.review).needs), undefined).length
        emit('STRESS', rec)
        expect(true).toBe(true)
      })
    }
  }
}

// metrics rows needed to push the jobs pane past the Svg cap: 8 sections x R rows, 10 waves of the board
for (const rows of [1, 5, 10, 20, 30, 40]) {
  test(`stress metrics size: 8 sections x ${rows} rows`, async ($, on) => {
    const m = JSON.parse(D.metricsMax)
    m.sections = m.sections.map((sec: any) => ({ ...sec, rows: sec.rows.slice(0, rows) }))
    rig($, on, { metrics: JSON.stringify(m), n: 10 })
    await $.session.start({ cwd: '/', surface: 'desktop', isInteractive: true })
    await $.command.run({ command: 'mw-jobs', args: 'style normal', origin: { kind: 'user' }, presentation: {} } as never)
    const rec: Record<string, unknown> = { kind: 'metricsrows', rows }
    try {
      const u = await $.ui.mount({ ...PANE, surface: 'desktop' } as never)
      const src = String((await u.find({ type: 'Svg' }))?.props.source ?? '')
      rec.svgBytes = src.length
      rec.hasMetrics = /Section 0/.test(src)
      if (rows === 5 || rows === 20) emit('SVG', { name: `metrics-rows${rows}-jobs`, source: src })
      await u.unmount()
    } catch (e) {
      rec.refused = String(e).split('\n')[0]!.slice(0, 160)
    }
    emit('STRESS', rec)
    expect(true).toBe(true)
  })
}
