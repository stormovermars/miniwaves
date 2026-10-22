import { expect, mock, test } from 'claude-code/testing'

const PANE = {
  plugin: 'mw-watch',
  component: 'Pane',
  requestId: 'mw-watch',
  props: {
    title: 'mw-watch',
    isFocused: false,
    bodyColumns: 80,
    placement: 'dock',
    scroll: { offset: 0, bodyRows: 30 },
    view: {},
  },
} as const

const MW = '/usr/local/bin/mw'
const REG = {
  jobs: {
    alpha: { title: 'Alpha job', board: '/boards/alpha', tenants: ['alpha-core', 'alpha-web'], news: 'https://example.com/news', newsFile: '', summary: '' },
    demo: { title: 'Demo job', board: '/boards/demo', tenants: ['demo'], news: '', summary: '' },
  },
}

const wave = (id: string, title: string, state: string, members: string, percent: number) => ({ id, title, state, members, percent, blocked: 0 })
const BASE = Date.parse('2026-10-22T10:00:00')
const iso = (ms: number) => new Date(ms).toISOString()
const item = (tenant: string, id: string, state: string, note = '', updated = iso(BASE - 600_000), type = 'task', title = 't') => ({ tenant, id, type, state, title, note, updated })
const ok = (out: unknown) => ({ value: { exitCode: 0, stdout: JSON.stringify(out), stderr: '', isStdoutTruncated: false, isStderrTruncated: false } })
const raw = (stdout: string, exitCode = 0) => ({ value: { exitCode, stdout, stderr: '', isStdoutTruncated: false, isStderrTruncated: false } })

type RigOptions = { registry?: unknown; hasMw?: boolean; files?: Record<string, string>; store?: Record<string, unknown>; oldStore?: unknown; messages?: unknown[]; extra?: (argv: string[], cwd: string) => unknown }

const rig = ($: any, on: any, opts: RigOptions = {}) => {
  const { registry = REG, hasMw = true, files = {}, store = {}, oldStore, messages = [], extra } = opts
  const state = {
    round: 0,
    alphaCalls: 0,
    toasts: [] as string[],
    opened: [] as string[],
    focused: [] as unknown[],
    runs: [] as string[][],
    statuses: [] as unknown[],
    sounds: [] as string[],
    argv0: new Set<string>(),
    files: { ...files } as Record<string, string>,
    store: new Map<string, unknown>(Object.entries(registry === null ? store : { registry, ...store })),
  }
  const clock = mock.clock(on, { now: BASE })
  on('store.get', (_: any, e: any) => ({ value: state.store.get(e.key) }))
  on('store.set', (_: any, e: any) => { state.store.set(e.key, e.value); return { value: undefined } })
  on('command.register', () => ({ value: { command: 'x' } }))
  on('ui.open', (_: any, e: any) => { state.opened.push(`${e.id}|${e.title}`); state.focused.push(e.focus); return { value: { isPlaced: true } } })
  on('session.start', (_: any, e: any) => ({ cwd: e.cwd }))
  on('session.messages', () => ({ value: messages }))
  on('session.root', () => ({ value: '/work' }))
  on('tool.call', { tool: 'Bash' }, () => ({ result: { stdout: '', stderr: '', interrupted: false }, text: '' }))
  on('tool.call', { tool: 'Read' }, () => ({ result: { type: 'text' }, text: '' }))
  on('tool.call', { tool: 'Write' }, () => ({ result: { type: 'text' }, text: '' }))
  on('fs.stat', (_: any, e: any) => ({ value: { realPath: String(e.path).replace('/link/', '/real/') } }))
  on('env.get', (_: any, e: any) => ({ value: e.name === 'HOME' ? '/home/me' : undefined }))
  on('fs.exists', (_: any, e: any) => ({ value: (hasMw && e.path === MW) || (String(e.path).endsWith('/.miniwaves.json') && !String(e.path).includes('/nope/') && !String(e.path).includes('/scanroot/')) }))
  on('fs.read', (_: any, e: any) => (state.files[e.path] !== undefined ? { value: state.files[e.path] } : { deny: 'ENOENT' }))
  on('fs.write', (_: any, e: any) => { state.files[e.path] = e.text; return { value: undefined } })
  on('ui.toast', (_: any, e: any) => { state.toasts.push(e.text); return { value: undefined } })
  on('ui.status', (_: any, e: any) => { state.statuses.push(e.text); return { value: undefined } })
  on('audio.play', (_: any, e: any) => { state.sounds.push(JSON.stringify(e)); return { value: undefined } })
  on('process.run', (_: any, e: any) => {
    const cmd = e.argv.join(' ')
    if (e.argv[0] === '/bin/sh' && String(e.argv[2]).includes('tidewatch_')) return oldStore === undefined ? raw('', 1) : raw(JSON.stringify(oldStore))
    if (e.argv[0] === '/bin/sh') return hasMw ? raw(`${MW}\n`) : raw('', 1)
    state.argv0.add(String(e.argv[0]))
    state.runs.push([...e.argv, `@${String(e.init?.cwd)}`])
    const hit = extra?.(e.argv.slice(1), String(e.init?.cwd))
    if (hit !== undefined) return hit
    const cwd = String(e.init?.cwd)
    if (cwd.endsWith('/nope')) return raw('', 2)
    const demo = cwd.endsWith('/demo')
    const closed = state.round > 0
    if (!demo && cmd.endsWith('wave list -A -j')) state.alphaCalls += 1
    let out: unknown = []
    if (cmd.endsWith('wave list -A -j')) {
      out = demo
        ? [wave('demo:WAV-001', 'D1 Only', closed ? 'done' : 'ready', closed ? '2/2' : '1/2', closed ? 100 : 50)]
        : [
            wave('alpha-core:WAV-002', 'C2 Harness', closed ? 'done' : 'ready', closed ? '6/6' : '5/6', closed ? 100 : 83),
            wave('alpha-web:WAV-004', 'M4 Consolidation', 'ready', '0/16', 0),
            wave('alpha-web:WAV-002', 'M2 Corpus', 'done', '0/0', 0),
            wave('alpha-web:WAV-005', 'M5 Verification and release', 'ready', '0/0', 0),
            wave('other:WAV-009', 'X9 Foreign', 'ready', '1/9', 11),
          ]
    } else if (cmd.endsWith('list -A -j -v') || cmd.endsWith('list -A -j')) {
      out = { items: demo ? [item('demo', 'D-1', closed ? 'done' : 'doing')] : [
            item('alpha-web', 'A-1', 'doing', '', iso(BASE - 2 * 3_600_000), 'task', 'Fix adapters -t alpha-core'),
            item('alpha-web', 'STR-1', 'doing', '', iso(BASE - 5 * 3_600_000), 'story', 'A story'),
            item('alpha-web', 'A-2', 'blocked', 'needs a decision'),
            item('alpha-web', 'A-3', 'blocked', 'awaiting review'),
            item('other', 'Z-1', 'doing'),
            ...(closed ? [item('alpha-web', 'A-9', 'done'), item('alpha-web', 'A-8', 'blocked', 'stuck')] : []),
          ], waves: [] }
    } else if (cmd.endsWith('log -A -n 40 -j')) {
      out = [{ ts: iso(BASE - 3_600_000), tenant: demo ? 'demo' : 'alpha-web', item: 'A-1', action: 'start', by: '', note: '' }]
      if (closed && !demo) {
        out = [
          { ts: iso(BASE + 30_000), tenant: 'alpha-core', item: 'WAV-002', action: 'done', by: '', note: '' },
          { ts: iso(BASE + 30_000), tenant: 'alpha-web', item: 'A-9', action: 'done', by: '', note: '' },
          ...(out as object[]),
        ]
      }
    }
    return ok(out)
  })
  return { state, clock }
}

const cmd = ($: any, args: string) => $.command.run({ command: 'mw-watch', args, origin: { kind: 'user' }, presentation: {} } as never)
const start = ($: any) => $.session.start({ cwd: '/work', surface: 'desktop', isInteractive: true })
const svgOf = async (u: any) => String((await u.find({ type: 'Svg' }))?.props.source ?? '')

test('two jobs: status, switching, alerts once, refresh, timer', async ($, on) => {
  const { state, clock } = rig($, on)
  await start($)
  await cmd($, 'style blocks')

  expect(state.toasts.length).toBe(0)
  // the bottom bar belongs to mw-jobs: this plugin draws no status
  expect(state.statuses).toEqual([])
  // mw is always reached by its absolute path, never as a bare name
  expect([...state.argv0]).toEqual([MW])

  for (const surface of ['terminal', 'desktop'] as const) {
    const ui = await $.ui.mount({ ...PANE, surface })
    // blocks off the terminal: the whole body is one monospace Svg (headline, sections, rows), tabs stay real
    if (surface !== 'terminal') {
      const src = await svgOf(ui)
      expect(src).toContain('! 1 need you')
      expect(src.indexOf('NEEDS YOU')).toBeGreaterThan(-1)
      expect(src.indexOf('NEEDS YOU')).toBeLessThan(src.indexOf('REVIEW'))
      expect(src.indexOf('REVIEW')).toBeLessThan(src.indexOf('IN PROGRESS'))
      expect(src.indexOf('IN PROGRESS')).toBeLessThan(src.indexOf('MILESTONES'))
      expect(src.indexOf('MILESTONES')).toBeLessThan(src.indexOf('LAST EVENTS'))
      expect(src).toContain('C2 Harness')
      expect(src).toContain('5/6 ')
      expect(src).toContain('W-M5 Verification')
      expect(src).toContain('not started')
      expect(src).toContain('stalled')
      expect(src).toContain('W·A-1')
      expect(src).not.toContain('X9')
      expect(src).not.toContain('A story')
      expect(src).not.toContain(' -t ')
      expect(await ui.find({ key: 'job-alpha' })).toBeDefined()
      await ui.press({ key: 'job-demo' })
      expect(await svgOf(ui)).toContain('D1 Only')
      await ui.press({ key: 'job-alpha' })
      await ui.unmount()
      continue
    }
    expect(await ui.find({ type: 'Text', text: /^C2 Harness/ })).toBeDefined()
    expect(await ui.find({ type: 'Text', text: /X9/ })).toBeUndefined()
    expect(await ui.find({ type: 'Text', text: /W-M2/ })).toBeUndefined()
    const head = await ui.find({ type: 'Text', text: /^! 1 need you/ })
    expect(head?.props.color).toBe('error')
    expect(await ui.find({ type: 'Text', text: /^◐ 1 awaiting review · ⏱ 1 stalled/ })).toBeDefined()
    expect(await ui.find({ type: 'Box', key: 'total-alpha-core:WAV-002' } as never)).toBeDefined()
    // no fact twice: the headline says needs, the summary adds the rest
    expect(await ui.find({ type: 'Text', text: /^! 1 need you.*need you/ })).toBeUndefined()
    // the ` -t slug` tail is gone, stories are not stalled or in progress, ids carry the tenant label
    expect(await ui.find({ type: 'Text', text: / -t / })).toBeUndefined()
    expect(await ui.find({ type: 'Text', text: /A story/ })).toBeUndefined()
    expect(await ui.find({ type: 'Text', text: /W·A-1/ })).toBeDefined()
    expect(await ui.find({ type: 'Text', text: /^1 in progress/ })).toBeUndefined()
    // 0/0 waves are one dim line
    expect(await ui.find({ type: 'Text', text: /W-M5 Verification and release/ })).toBeDefined()
    expect((await ui.find({ type: 'Text', text: /^not started/ }))?.props.dimColor).toBe(true)
    // tabs only for 2+ jobs (two here)
    expect(await ui.find({ key: 'job-alpha' })).toBeDefined()
    const texts = (await ui.findAll({ type: 'Text' })).map(t => t.text)
    const at = (re: RegExp) => texts.findIndex(t => re.test(t))
    expect(at(/^NEEDS YOU/)).toBeGreaterThan(-1)
    expect(at(/^NEEDS YOU/)).toBeLessThan(at(/^REVIEW/))
    expect(at(/^REVIEW/)).toBeLessThan(at(/^IN PROGRESS/))
    expect(at(/^IN PROGRESS/)).toBeLessThan(at(/^MILESTONES/))
    expect(at(/^MILESTONES/)).toBeLessThan(at(/^LAST EVENTS/))
    const stalled = await ui.find({ type: 'Text', text: /^stalled$/ })
    expect(stalled?.props.color).toBe('warning')
    expect(((await ui.findAll({ type: 'Text', text: /●/ })).filter(d => d.props.color !== undefined))).toHaveLength(0)
    await ui.press({ key: 'job-demo' })
    expect(await ui.find({ type: 'Text', text: /^D1 Only/ })).toBeDefined()
    await ui.press({ key: 'job-alpha' })
    await ui.unmount()
  }

  // both jobs close/finish on the next timer tick
  state.round = 1
  await clock.advance(60_000)
  const toasts = state.toasts.join('\n')
  expect(toasts).toContain('[alpha] Milestone closed: C2')
  expect(toasts).toContain('[demo] Milestone closed: D1')
  // one alert per event (two milestones closed, one item blocked), kept in the store; only the person-needing one pings.
  // (this build of the engine has no ui.notify event to hook, so the store is where a test sees what was sent)
  const keys = state.store.get('notified') as string[]
  expect(keys.length).toBe(3)
  expect(keys.some(k => k.includes('|blocked|alpha-web:A-8|'))).toBe(true)
  expect(state.sounds.length).toBe(1)
  expect(state.sounds[0]).toContain('assets/Ping.aiff')
  // a later tick finds nothing new: nothing is sent twice
  await clock.advance(60_000)
  expect((state.store.get('notified') as string[]).length).toBe(3)
  expect(state.sounds.length).toBe(1)

  // new since the seen cursor: a dot and a delta chip in the Svg, until Refresh is pressed
  const ui = await $.ui.mount({ ...PANE, surface: 'desktop' })
  expect(await svgOf(ui)).toContain('class="new"> ●')
  expect(await svgOf(ui)).toContain('+1 done')
  const before = state.alphaCalls
  await ui.press({ key: 'refresh' })
  expect(state.alphaCalls).toBe(before + 1)
  expect(await svgOf(ui)).not.toContain('class="new"> ●')
  expect(await svgOf(ui)).not.toContain('since')
  await ui.unmount()

  // /mw-watch demo selects demo, a bare /mw-watch opens the pane
  await cmd($, 'demo')
  const ui2 = await $.ui.mount({ ...PANE, surface: 'desktop' })
  expect(await svgOf(ui2)).toContain('✓ 1 closed')
  await ui2.unmount()
  await cmd($, 'alpha')
  const ui3 = await $.ui.mount({ ...PANE, surface: 'desktop' })
  const s3 = await svgOf(ui3)
  expect(s3).toContain('need you')
  expect(s3).toContain('auto 1m')
  await ui3.unmount()

  // mute silences sound and notifications; Off stops the timer
  await cmd($, 'every off')
  await cmd($, 'mute')
  const stopped = state.alphaCalls
  await clock.advance(600_000)
  expect(state.alphaCalls).toBe(stopped)
})

test('an empty registry shows the how-to, no toast', async ($, on) => {
  const { state } = rig($, on, { registry: null, extra: argv => (argv[0] === 'boards' ? ok({ root: argv[1], boards: [], truncated: false, reason: '' }) : undefined) })
  await start($)
  for (const surface of ['terminal', 'desktop'] as const) {
    const ui = await $.ui.mount({ ...PANE, surface })
    expect(await ui.find({ type: 'Text', text: /No boards yet\. Run \/mw-watch to look for them/ })).toBeDefined()
    expect(await ui.find({ type: 'Text', text: /mw command was not found/ })).toBeUndefined()
    expect(await ui.find({ type: 'Svg' })).toBeUndefined()
    await ui.unmount()
  }
  expect(state.toasts.length).toBe(0)
  expect((await cmd($, '')).text).toContain('no boards found')
})

test('without an mw binary the pane says how to get it and nothing runs', async ($, on) => {
  const { state } = rig($, on, { registry: null, hasMw: false })
  await start($)
  const ui = await $.ui.mount({ ...PANE, surface: 'desktop' })
  expect(await ui.find({ type: 'Text', text: /mw command was not found/ })).toBeDefined()
  await ui.unmount()
  expect((await cmd($, 'add /boards/fresh')).text).toContain('mw command was not found')
  expect(state.argv0.size).toBe(0)
})

const flat = (el: unknown): string => {
  if (typeof el === 'string') return el
  const e = el as { text?: string; children?: unknown[] }
  if (Array.isArray(e.children) && e.children.length > 0) return e.children.map(flat).join('')
  return e.text ?? ''
}

test('styles: normal and blocks; blocks is cell-counted on the terminal and a shared mono Svg elsewhere', async ($, on) => {
  const { state } = rig($, on)
  await start($)
  const mount = (surface: 'terminal' | 'desktop', props: Record<string, unknown> = {}) =>
    $.ui.mount({ ...PANE, props: { ...PANE.props, ...props }, surface } as never)

  // default is normal: one Svg on desktop with the headline in it
  const d = await mount('desktop')
  const svg = await d.find({ type: 'Svg' })
  expect(svg).toBeDefined()
  expect(String(svg?.props.source)).toContain('! 1 need you')
  expect(String(svg?.props.source)).toContain('not started')
  expect(String(svg?.props.source)).not.toContain(' -t ')
  await d.unmount()

  // the terminal has no Svg: every style is drawn in cells, the button still names the style
  const t = await mount('terminal')
  expect(await t.find({ type: 'Svg' })).toBeUndefined()
  expect((await t.find({ key: 'style' }))?.props.label).toBe('Style: normal')
  await t.unmount()

  await cmd($, 'style blocks')
  expect(state.store.get('style')).toBe('blocks')

  // terminal: monospace, every bar the one length barLength(width), only block glyphs
  const glyphs = async (props: Record<string, unknown>, re: RegExp) => {
    const ui = await mount('terminal', props)
    const bars = (await ui.findAll({ type: 'Box' })).filter(b => b.key?.startsWith('bar-'))
    const lens = bars.map(b => (flat(b).match(re) ?? []).length)
    await ui.unmount()
    return lens
  }
  const expected: [number | undefined, number][] = [[80, 40], [40, 20], [30, 15], [8, 10], [undefined, 20]]
  for (const [cols, want] of expected) {
    const lens = await glyphs({ bodyColumns: cols }, /[█░]/g)
    expect(lens.length).toBeGreaterThan(1)
    expect(new Set(lens).size).toBe(1)
    expect(lens[0]).toBe(want)
    expect((await glyphs({ bodyColumns: cols }, /[━─]/g))[0]).toBe(0)
  }

  // terminal grid: one shared name, track and right column; every row the same fixed height
  const tu = await mount('terminal', { bodyColumns: 80 })
  const tb = await tu.findAll({ type: 'Box' })
  const uniq = (prefix: string, prop: string) => new Set(tb.filter(b => b.key?.startsWith(prefix)).map(b => String(b.props[prop])))
  expect(uniq('name-', 'width').size).toBe(1)
  expect(uniq('status-', 'width').size).toBe(1)
  expect(uniq('right-', 'width').size).toBe(1)
  const allRows = tb.filter(b => b.key?.startsWith('row-'))
  expect(allRows.length).toBeGreaterThan(5)
  expect(new Set(allRows.map(r => `${r.props.height}/${r.props.flexShrink}/${r.props.overflow}`)).size).toBe(1)
  for (const b of tb.filter(x => x.key?.startsWith('bar-') || x.key?.startsWith('status-'))) expect(flat(b)).not.toContain('…')
  // the count is three fixed Boxes: same widths on every row, digits padded with figure spaces
  const counts = tb.filter(b => b.key?.startsWith('count-'))
  expect(new Set(counts.map(b => b.props.width)).size).toBe(1)
  expect(new Set(tb.filter(b => b.key?.startsWith('done-')).map(b => b.props.width)).size).toBe(1)
  expect(new Set(tb.filter(b => b.key?.startsWith('total-')).map(b => b.props.width)).size).toBe(1)
  await tu.unmount()
  // below 50 columns the rows stack (no name column), two lines for every wave row
  const nu = await mount('terminal', { bodyColumns: 40 })
  const narrow = await nu.findAll({ type: 'Box' })
  expect(narrow.filter(b => b.key?.startsWith('name-'))).toHaveLength(0)
  const narrowRows = narrow.filter(b => b.key?.startsWith('row-wav-'))
  expect(new Set(narrowRows.map(r => r.props.height)).size).toBe(1)
  expect(narrowRows[0]!.props.height).toBe(2)
  await nu.unmount()

  // desktop blocks: the whole body is one monospace Svg: one font, one size, 22ch name, 20ch bar, 19px rows
  const bu = await mount('desktop', { bodyColumns: 80 })
  const svgs = await bu.findAll({ type: 'Svg' })
  expect(svgs.length).toBe(1)
  const src = String(svgs[0]!.props.source)
  expect(src).toContain('ui-monospace')
  expect(src).not.toContain('-apple-system')
  expect(src).not.toMatch(/font-size:(?!12px)/)
  expect(src).toContain('█')
  expect(src).not.toMatch(/[━─…]/)
  const lens = [...src.matchAll(/textLength="([\d.]+)"/g)].map(m => m[1])
  expect(lens.length).toBeGreaterThan(1)
  expect(new Set(lens).size).toBe(1)
  expect(lens[0]).toBe(String(20 * 12 * 0.6))
  const barXs = [...src.matchAll(/<text x="([\d.]+)" y="[\d.]+" class="m" textLength/g)].map(m => m[1])
  const notStartedX = /<text x="([\d.]+)" y="[\d.]+" class="m" xml:space="preserve"><tspan class="dim">not started/.exec(src)?.[1]
  expect(new Set(barXs).size).toBe(1)
  expect(notStartedX).toBe(barXs[0])
  const ys = [...src.matchAll(/<text x="8" y="([\d.]+)"/g)].map(m => Number(m[1]))
  const diffs = ys.slice(1).map((y, i) => y - ys[i]!)
  expect(ys.length).toBeGreaterThan(8)
  expect(Math.min(...diffs)).toBe(19)
  const barYs = [...src.matchAll(/y="([\d.]+)" class="m" textLength/g)].map(m => Number(m[1]))
  expect(barYs.slice(1).every((y, i) => y - barYs[i]! === 19)).toBe(true)
  expect(src).toContain('> 5/6 <')
  expect(src).toContain('> 0/16<')
  expect(src).toContain('> 0/0 <')
  expect(await bu.find({ type: 'Box', key: 'bar-alpha-core:WAV-002' } as never)).toBeUndefined()
  await bu.unmount()

  // -/#/+ set the real Svg font size: 11, 12 and 14px, with row height 1.6x
  const sizeAt = async (step: string, style: string) => {
    await cmd($, 'reset')
    await cmd($, step)
    await cmd($, `style ${style}`)
    const u = await mount('desktop', { bodyColumns: 80 })
    const source = await svgOf(u)
    await u.unmount()
    return source
  }
  const blocksAt = async (step: string) => {
    const source = await sizeAt(step, 'blocks')
    return { px: /\.m\{font-family:[^;]*;font-size:(\d+)px\}/.exec(source)?.[1], source }
  }
  expect((await blocksAt('smaller')).px).toBe('11')
  expect((await blocksAt('reset')).px).toBe('12')
  const big = await blocksAt('bigger')
  expect(big.px).toBe('14')
  const ysBig = [...big.source.matchAll(/<text x="8" y="([\d.]+)"/g)].map(m => Number(m[1]))
  expect(Math.min(...ysBig.slice(1).map((y, i) => y - ysBig[i]!))).toBe(22)
  // normal: one sans font, four sizes (18/13/12/11) scaled 0.85 / 1 / 1.15 per step, with the row height
  const sizesOf = (source: string) => [...new Set([...source.matchAll(/font-size="([\d.]+)"/g)].map(m => Number(m[1])))].sort((x, y) => x - y)
  const near = (a: number[], b: number[]) => a.length === b.length && a.every((x, i) => Math.abs(x - b[i]!) < 0.02)
  const base = [11, 12, 13, 18]
  for (const [step, k] of [['smaller', 0.85], ['reset', 1], ['bigger', 1.15]] as const) {
    const source = await sizeAt(step, 'normal')
    expect(near(sizesOf(source), base.map(x => Math.round(x * k * 100) / 100))).toBe(true)
    expect(source).not.toContain('ui-monospace')
    const rowHs = [...source.matchAll(/<svg x="[\d.]+" y="([\d.]+)" width="[\d.]+" height="([\d.]+)" overflow="hidden">/g)].map(m => Number(m[2]))
    expect(rowHs.length).toBeGreaterThan(3)
    expect(new Set(rowHs.map(h => h.toFixed(2))).size).toBe(1)
    expect(Math.abs(rowHs[0]! - 22 * k)).toBeLessThan(0.02)
  }
  const normalSrc = await sizeAt('reset', 'normal')
  // three-column grid: every bar starts and ends at the same x, 6px high, rounded
  const trackRects = [...normalSrc.matchAll(/<rect x="([\d.]+)" y="[\d.]+" width="([\d.]+)" height="6" rx="3" class="track"\/>/g)]
  expect(trackRects.length).toBeGreaterThan(1)
  expect(new Set(trackRects.map(m => `${m[1]}/${m[2]}`)).size).toBe(1)
  expect(normalSrc).toMatch(/class="dim"[^>]*>not started</)
  expect(normalSrc).toContain('5/6 ')
  expect(normalSrc).toContain('0/16')
  await cmd($, 'reset')
  await cmd($, 'style blocks')

  // the button cycles normal -> blocks -> normal
  await cmd($, 'style normal')
  const ui = await mount('desktop')
  await ui.press({ key: 'style' })
  expect(state.store.get('style')).toBe('blocks')
  await ui.press({ key: 'style' })
  expect(state.store.get('style')).toBe('normal')
  await ui.unmount()

  // density: - is compact (no meta line), # resets, commands persist
  await cmd($, 'style blocks')
  const meta = async () => {
    const u = await mount('desktop', { bodyColumns: 80 })
    const m = (await svgOf(u)).includes('alpha-web · A-2') ? true : undefined
    await u.unmount()
    return m
  }
  expect(await meta()).toBeDefined()
  await cmd($, 'smaller')
  expect(state.store.get('fontStep')).toBe(-1)
  expect(await meta()).toBeUndefined()
  await cmd($, 'reset')
  expect(state.store.get('fontStep')).toBe(0)
  await cmd($, 'bigger')
  expect(state.store.get('fontStep')).toBe(1)
})

test('one job shows its title instead of tabs', async ($, on) => {
  rig($, on, { registry: { jobs: { alpha: REG.jobs.alpha } } })
  await start($)
  const ui = await $.ui.mount({ ...PANE, surface: 'desktop' })
  expect(await ui.find({ type: 'Text', text: /^Alpha job$/ })).toBeDefined()
  expect(await ui.find({ key: 'job-alpha' })).toBeUndefined()
  await ui.unmount()
})

// ---- the built-ins draw exactly what the mod drew before styles became tokens ----

const strip = (s: string) => s.replace(/<style>[\s\S]*?<\/style>/, '').replace(/ class="[^"]*"/g, '')
const fnv = (s: string) => {
  let h = 0x811c9dc5
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 0x01000193) >>> 0
  }
  return h.toString(16)
}
// hash and length of the Svg with classes and the style block stripped, taken from the mod before the refactor
const BEFORE: Record<string, [string, number]> = {
  'normal/smaller': ['f4a8a0bd', 3199],
  'normal/reset': ['1642f3e9', 3203],
  'normal/bigger': ['60153fcd', 3399],
  'blocks/smaller': ['1b493c92', 2376],
  'blocks/reset': ['fdbcccf', 2553],
  'blocks/bigger': ['3e3ca6a4', 2475],
}

test('built-in styles render the same Svg as before the refactor', async ($, on) => {
  rig($, on)
  await start($)
  for (const style of ['normal', 'blocks']) {
    for (const step of ['smaller', 'reset', 'bigger']) {
      await cmd($, 'reset')
      await cmd($, step)
      await cmd($, `style ${style}`)
      const u = await $.ui.mount({ ...PANE, surface: 'desktop' })
      const src = await svgOf(u)
      await u.unmount()
      expect([fnv(strip(src)), strip(src).length]).toEqual(BEFORE[`${style}/${step}`])
      const css = /<style>([\s\S]*?)<\/style>/.exec(src)?.[1] ?? ''
      // the same palette as before, light and dark
      for (const c of ['#18181B', '#5B5B66', '#B91C1C', '#A16207', '#15803D']) expect(css).toContain(c)
      for (const c of ['#F4F4F5', '#A1A1AA', '#F87171', '#FBBF24', '#4ADE80']) expect(css).toContain(c)
      expect(css).toContain(style === 'normal' ? '--track:#E4E4E7' : '--track:#D4D4D8')
      expect(css).toContain(style === 'normal' ? '--new:#2563EB' : '--new:#15803D')
    }
  }
  // key attributes of the default drawings
  await cmd($, 'reset')
  await cmd($, 'style normal')
  const n = await $.ui.mount({ ...PANE, surface: 'desktop' })
  const ns = await svgOf(n)
  await n.unmount()
  expect(ns).toContain('width="520"')
  expect(ns).toContain('height="6" rx="3" class="track"')
  expect([...new Set([...ns.matchAll(/font-size="([\d.]+)"/g)].map(m => m[1]))].sort()).toEqual(['11', '12', '13', '18'])
  await cmd($, 'style blocks')
  const b = await $.ui.mount({ ...PANE, surface: 'desktop' })
  const bs = await svgOf(b)
  await b.unmount()
  expect(bs).toContain('font-size:12px')
  expect(bs).toContain('textLength="144"')
  expect(bs).toContain('█')
})

// ---- user styles ----

const USER = (extra: unknown) => ({ userStyles: { mine: extra } })

test('a user style overriding colors.needs and bar.kind changes the output', async ($, on) => {
  const { state } = rig($, on, { store: USER({ colors: { needs: { light: '#112233', dark: '#445566' } }, bar: { kind: 'glyphs', fill: '#', rest: '.' } }) })
  await start($)
  const draw = async (name: string) => {
    await cmd($, `style ${name}`)
    const u = await $.ui.mount({ ...PANE, surface: 'desktop' })
    const s = await svgOf(u)
    await u.unmount()
    return s
  }
  const normal = await draw('normal')
  const mine = await draw('mine')
  expect(mine).not.toBe(normal)
  // the colour: only the user style carries it
  expect(mine).toContain('--needs:#112233')
  expect(mine).toContain('--needs:#445566')
  expect(normal).not.toContain('#112233')
  // the bar kind: normal draws rounded rects over a track, mine draws glyph text and no rect track
  expect(normal).toContain('class="track"/>')
  expect(mine).not.toMatch(/height="6" rx="3" class="track"/)
  expect(mine).toContain('lengthAdjust="spacingAndGlyphs"')
  expect(mine).toMatch(/<tspan class="\w+">#+<\/tspan>/)
  // everything it did not mention is normal's
  expect(mine).toContain('--fg:#18181B')
  expect(state.toasts.length).toBe(0)
  // the terminal draws cell bars from the style's glyphs
  const t = await $.ui.mount({ ...PANE, surface: 'terminal' })
  const bars = (await t.findAll({ type: 'Box' })).filter(x => x.key?.startsWith('bar-')).map(x => flat(x))
  expect(bars.length).toBeGreaterThan(0)
  expect(bars.every(x => /^#*\.*$/.test(x))).toBe(true)
  await t.unmount()
})

test('a bad token is ignored with one toast', async ($, on) => {
  const bad = {
    font: 'comic',
    colors: { needs: { light: 'red', dark: '#445566' }, glow: { light: '#fff', dark: '#000' } },
    bar: { kind: 'glyphs', fill: '<', rest: 'ab' },
    sizes: { body: 'big' },
    rowHeight: 9000,
    scale: [1, 1],
    wat: true,
  }
  const { state } = rig($, on, { store: USER(bad) })
  await start($)
  await cmd($, 'style mine')
  for (let i = 0; i < 3; i++) {
    const u = await $.ui.mount({ ...PANE, surface: 'desktop' })
    const src = await svgOf(u)
    await u.unmount()
    // nothing broke: the good value stays, the bad ones fell back to normal's
    expect(src).toContain('--needs:#445566')
    expect(src).toContain('--needs:#B91C1C')
    expect(src).not.toContain('red')
    expect(src).not.toContain('<<')
  }
  await cmd($, 'bigger')
  await cmd($, 'reset')
  const toasts = state.toasts.filter(t => t.includes('mw-watch style "mine"'))
  expect(toasts.length).toBe(1)
  expect(toasts[0]).toContain('font')
  expect(toasts[0]).toContain('more')
  // fixing the file clears it, breaking it again reports again
  state.store.set('userStyles', { mine: {} })
  await cmd($, 'every 1m')
  const ui = await $.ui.mount({ ...PANE, surface: 'desktop' })
  await ui.press({ key: 'refresh' })
  await ui.unmount()
  expect(state.toasts.filter(t => t.includes('mw-watch style')).length).toBe(1)
})

test('style names, new, import, export and list', async ($, on) => {
  const { state } = rig($, on, { files: { '/styles/loud.json': JSON.stringify({ colors: { new: { light: '#ff0000', dark: '#ff8800' } } }), '/styles/Bad_Name.json': '{}' } })
  await start($)
  expect((await cmd($, 'style list')).text).toContain('normal, blocks')
  expect((await cmd($, 'style new quiet')).text).toContain('copy of normal')
  expect((await cmd($, 'style new quiet')).text).toContain('already exists')
  expect((await cmd($, 'style new normal')).text).toContain('already exists')
  expect((await cmd($, 'style new Bad_Name')).text).toContain('usage')
  expect((await cmd($, 'style new ../x')).text).toContain('usage')
  expect(((state.store.get('userStyles') as Record<string, { font: string }>).quiet)?.font).toBe('sans')
  expect((await cmd($, 'style import /styles/loud.json')).text).toContain('imported style "loud"')
  expect((await cmd($, 'style import /styles/Bad_Name.json')).text).toContain('not a usable style name')
  expect((await cmd($, 'style import /styles/missing.json')).text).toContain('could not read')
  expect((await cmd($, 'style import ~/loud.json')).text).toContain('absolute')
  expect((await cmd($, 'style list')).text).toContain('normal, blocks, loud, quiet')
  expect((await cmd($, 'style loud')).text).toBe('mw-watch: style loud.')
  expect(state.store.get('style')).toBe('loud')
  expect((await cmd($, 'style nonesuch')).text).toContain('unknown style')
  expect((await cmd($, 'style export blocks /out/blocks.json')).text).toContain('/out/blocks.json')
  expect(JSON.parse(state.files['/out/blocks.json']!).bar.kind).toBe('glyphs')
  expect((await cmd($, 'style export loud /out/loud.json')).text).toContain('written')
  expect(JSON.parse(state.files['/out/loud.json']!).colors.new.light).toBe('#ff0000')
  expect(state.toasts.length).toBe(0)
})

// ---- the registry ----

test('add and remove boards in the registry', async ($, on) => {
  const { state } = rig($, on, { registry: null })
  await start($)
  const jobs = () => (state.store.get('registry') as { jobs: Record<string, { board: string; tenants: string[]; title: string }> } | undefined)?.jobs ?? {}

  expect((await cmd($, 'add')).text).toContain('usage')
  // a folder with no .miniwaves.json is scanned instead; the fake mw has no `boards` there
  expect((await cmd($, 'add /boards/nope')).text).toContain('update mw for auto-discovery')
  expect(Object.keys(jobs())).toEqual([])

  // the id is the folder name, the tenants come from the board's items
  expect((await cmd($, 'add /boards/fresh/')).text).toBe('mw-watch: added fresh (2 tenants).')
  expect(jobs().fresh).toEqual({ title: 'fresh', board: '/boards/fresh', tenants: ['alpha-web', 'other'], news: '', newsFile: '', summary: '' })
  // a relative path is taken from the session folder, stored absolute
  expect((await cmd($, 'add rel/demo')).text).toContain('added demo')
  expect(jobs().demo!.board).toBe('/work/rel/demo')
  expect(jobs().demo!.tenants).toEqual(['demo'])
  expect([...state.argv0]).toEqual([MW])
  // the same folder again updates; the same name elsewhere is refused
  expect((await cmd($, 'add /boards/fresh')).text).toContain('updated fresh')
  expect((await cmd($, 'add /elsewhere/demo')).text).toContain('already used by /work/rel/demo')

  const ui = await $.ui.mount({ ...PANE, surface: 'desktop' })
  expect(await ui.find({ key: 'job-fresh' })).toBeDefined()
  expect(await ui.find({ key: 'job-demo' })).toBeDefined()
  await ui.unmount()

  expect((await cmd($, 'remove ghost')).text).toContain('unknown job "ghost"')
  expect((await cmd($, 'remove demo')).text).toBe('mw-watch: removed demo.')
  expect(Object.keys(jobs())).toEqual(['fresh'])
  expect((await cmd($, 'remove fresh')).text).toBe('mw-watch: removed fresh.')
  const empty = await $.ui.mount({ ...PANE, surface: 'desktop' })
  expect(await empty.find({ type: 'Text', text: /No boards yet/ })).toBeDefined()
  await empty.unmount()
})

// ---- the news pane ----

const NEWS_PANE = { ...PANE, requestId: 'mw-watch-news' } as const
const NEWS_FILE = '/boards/alpha/docs/news/status.md'

test('the news pane renders the news Markdown, capped, with the news link', async ($, on) => {
  const long = `# Status\n\n${'word '.repeat(4000)}`
  const { state } = rig($, on, { files: { [NEWS_FILE]: '# Status\n\nAll *good*.' } })
  await start($)
  // the jobs pane is titled for the job it shows
  expect(state.opened.at(0)).toBe('mw-watch|mw jobs · alpha')

  // opens on request only: nothing opened a news pane yet; the command does, for the named job
  expect(state.opened.some(o => o.startsWith('mw-watch-news'))).toBe(false)
  expect((await cmd($, 'news')).text).toContain('news pane opened for alpha')
  expect(state.opened.at(-1)).toBe('mw-watch-news|mw news · alpha')
  expect((await cmd($, 'news demo')).text).toContain('opened for demo')
  expect(state.opened.at(-1)).toBe('mw-watch-news|mw news · demo')
  expect((await cmd($, 'news ghost')).text).toContain('unknown job "ghost"')
  await cmd($, 'alpha')

  for (const surface of ['terminal', 'desktop'] as const) {
    await cmd($, 'news alpha')
    const ui = await $.ui.mount({ ...NEWS_PANE, surface })
    expect(await ui.find({ type: 'Text', text: /^mw news · alpha$/ })).toBeDefined()
    const md = await ui.find({ type: 'Markdown' })
    expect(String(md?.props.text)).toBe('# Status\n\nAll *good*.')
    expect(await ui.find({ type: 'Link' })).toBeDefined()
    expect(await ui.find({ type: 'Text', text: /Truncated/ })).toBeUndefined()
    await ui.unmount()
  }

  // a long file is cut at 9,500 characters, with a note
  state.files[NEWS_FILE] = long
  const ui = await $.ui.mount({ ...NEWS_PANE, surface: 'desktop' })
  await ui.press({ key: 'refresh-news' })
  expect(String((await ui.find({ type: 'Markdown' }))?.props.text).length).toBe(9500)
  expect(await ui.find({ type: 'Text', text: /Truncated: showing the first 9500/ })).toBeDefined()
  await ui.unmount()

  // the News button in the jobs pane opens it
  const before = state.opened.length
  const jobs = await $.ui.mount({ ...PANE, surface: 'desktop' })
  await jobs.press({ key: 'news' })
  await jobs.unmount()
  expect(state.opened.length).toBe(before + 1)
  expect(state.opened.at(-1)).toBe('mw-watch-news|mw news · alpha')
  // it only reads a file: no process beyond mw's board calls ran for it
  expect([...state.argv0]).toEqual([MW])
})

test('a missing news file shows a short how-to', async ($, on) => {
  rig($, on)
  await start($)
  await cmd($, 'news')
  const ui = await $.ui.mount({ ...NEWS_PANE, surface: 'desktop' })
  expect(await ui.find({ type: 'Text', text: /No news page yet/ })).toBeDefined()
  expect(await ui.find({ type: 'Text', text: new RegExp('/boards/alpha/docs/news/status.md') })).toBeDefined()
  expect(await ui.find({ type: 'Markdown' })).toBeUndefined()
  await ui.unmount()
})

const JOBS = [
  { id: 'alpha-job', title: 'Alpha job', tenants: ['alpha-core'], 'news.out': 'docs/news/', 'news.url': 'https://example.com/a', summary: '', pane: 'on' },
  { id: 'web', title: 'Web job', tenants: ['alpha-web'], 'news.out': 'out/web.md', 'news.url': '', summary: '', pane: 'on' },
  { id: 'hidden', title: 'Hidden', tenants: ['alpha-web'], 'news.out': 'docs/news/', 'news.url': '', summary: '', pane: 'off' },
]
const jobsExtra = (argv: string[], cwd: string) =>
  argv.join(' ') === 'jobs show -j' ? (cwd.endsWith('/alpha') ? ok(JOBS) : ok([])) : undefined

test('jobs: one tab per job with pane on, a board with no jobs stays one job', async ($, on) => {
  const { state } = rig($, on, { extra: jobsExtra, files: { '/boards/alpha/docs/news/status.md': '# alpha news', '/boards/alpha/out/web.md': '# web news' } })
  await start($)
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await ui.find({ key: 'job-alpha/alpha-job' })).toBeDefined()
  expect(await ui.find({ key: 'job-alpha/web' })).toBeDefined()
  expect(await ui.find({ key: 'job-alpha/hidden' })).toBeUndefined()
  expect(await ui.find({ key: 'job-demo' })).toBeDefined()
  expect(await ui.find({ type: 'Text', text: /^Fanfare/ })).toBeUndefined()
  // the job's tenants only: web has alpha-web, not alpha-core
  await ui.press({ key: 'job-alpha/web' })
  expect(await ui.find({ type: 'Text', text: /^C2 Harness/ })).toBeUndefined()
  expect(await ui.find({ type: 'Text', text: /M4 Consolidation/ })).toBeDefined()
  expect(await ui.find({ type: 'Link' })).toBeUndefined()
  await ui.press({ key: 'job-alpha/alpha-job' })
  expect(await ui.find({ type: 'Link' })).toBeDefined()
  await ui.unmount()
  // news pane: titled by the job id, read from news.out/status.md, brought to the front
  await cmd($, 'news alpha-job')
  expect(state.opened.at(-1)).toBe('mw-watch-news|mw news · alpha-job')
  expect(state.focused.at(-1)).toBe(true)
  expect(state.opened.some(o => o === 'mw-watch|mw jobs · alpha-job' || o === 'mw-watch|mw jobs · alpha')).toBe(true)
  const news = await $.ui.mount({ ...PANE, requestId: 'mw-watch-news', surface: 'terminal' })
  expect(await news.find({ type: 'Markdown' })).toBeDefined()
  expect(String((await news.find({ type: 'Markdown' }))?.props.text)).toBe('# alpha news')
  await news.unmount()
  await cmd($, 'news web')
  const web = await $.ui.mount({ ...PANE, requestId: 'mw-watch-news', surface: 'terminal' })
  expect(String((await web.find({ type: 'Markdown' }))?.props.text)).toBe('# web news')
  await web.unmount()
})

const FAN = { config: { enabled: false }, enabled: false, use: 'all', sets: { calm: ['rocky'], loud: ['die-hard'] } }
const fanExtra = (user0: Record<string, unknown> = { 'fanfare.enabled': null, 'fanfare.use': '' }, fan: unknown = FAN) => {
  // `mw user set` persists, like the real mw: the next `user show -j` reads what was set
  const user = { ...user0 }
  return (argv: string[]) => {
    const c = argv.join(' ')
    if (c === 'fanfare -j') return ok(fan)
    if (c === 'user show -j') return ok(user)
    if (c.startsWith('user set ')) {
      for (const kv of argv.slice(2)) {
        const [k = '', v = ''] = kv.split('=')
        user[k] = v === 'true' ? true : v === 'false' ? false : v
      }
      return raw('')
    }
    if (c.startsWith('fanfare test')) return raw('')
    return undefined
  }
}
const fanRuns = (state: { runs: string[][] }) => state.runs.filter(r => r[1] === 'user' || r[1] === 'fanfare').map(r => r.slice(1, -1).join(' ')).filter(r => /^user set|^fanfare test/.test(r))

test('fanfare: top-row button cycles off -> all -> sets -> off; there is no Test button', async ($, on) => {
  const { state } = rig($, on, { extra: fanExtra({ 'fanfare.enabled': true, 'fanfare.use': 'calm' }) })
  await start($)
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await ui.find({ key: 'fanfare' })).toBeDefined()
  expect(await ui.find({ type: 'Button', label: 'Fanfare: calm' } as never)).toBeDefined()
  expect(await ui.find({ key: 'fanfare-toggle' })).toBeUndefined()
  expect(await ui.find({ key: 'fanfare-set' })).toBeUndefined()
  expect(await ui.find({ key: 'fanfare-test' })).toBeUndefined()
  // no Select, no Input
  expect(await ui.find({ type: 'Select' })).toBeUndefined()
  await ui.press({ key: 'fanfare' })
  await cmd($, 'fanfare test')
  expect(fanRuns(state)).toEqual(['user set fanfare.use=loud', 'fanfare test milestone'])
  // argv[0] absolute, cwd is the board
  expect(state.runs.filter(r => r[1] === 'user' && r[2] === 'set').every(r => r[0] === MW && r.at(-1) === '@/boards/alpha')).toBe(true)
  await ui.unmount()
})

test('fanfare cycle with sets [calm, loud]: off, all, calm, loud, off', async ($, on) => {
  const { state } = rig($, on, { extra: fanExtra() })
  await start($)
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await ui.find({ type: 'Button', label: 'Fanfare: off' } as never)).toBeDefined()
  for (const _ of [1, 2, 3, 4]) await ui.press({ key: 'fanfare' })
  expect(fanRuns(state)).toEqual([
    'user set fanfare.enabled=true fanfare.use=all',
    'user set fanfare.use=calm',
    'user set fanfare.use=loud',
    'user set fanfare.enabled=false',
  ])
  await ui.unmount()
})

test('fanfare cycle with no sets: off, all, off; /mw-watch fanfare next does the same', async ($, on) => {
  const { state } = rig($, on, { extra: fanExtra(undefined, { config: { enabled: false }, enabled: false, use: 'all', sets: {} }) })
  await start($)
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  await ui.press({ key: 'fanfare' })
  await ui.press({ key: 'fanfare' })
  await cmd($, 'fanfare next')
  expect(fanRuns(state)).toEqual(['user set fanfare.enabled=true fanfare.use=all', 'user set fanfare.enabled=false', 'user set fanfare.enabled=true fanfare.use=all'])
  await ui.unmount()
})

test('/mw-fanfare with no arguments shows the state and opens the pane; `next` cycles', async ($, on) => {
  const { state } = rig($, on, { extra: fanExtra() })
  await start($)
  const bare = await $.command.run({ command: 'mw-fanfare', args: '', origin: { kind: 'user' }, presentation: {} } as never)
  expect(bare.text).toContain('mw-fanfare: off, set all; sets: all, calm, loud')
  expect(state.opened.some(o => o.startsWith('mw-fanfare|'))).toBe(true)
  const ui = await $.ui.mount({ ...PANE, requestId: 'mw-fanfare', surface: 'terminal' })
  expect(await ui.find({ key: 'fanfare' })).toBeDefined()
  expect(await ui.find({ key: 'fanfare-test' })).toBeUndefined()
  await ui.press({ key: 'fanfare' })
  const next = await $.command.run({ command: 'mw-fanfare', args: 'next', origin: { kind: 'user' }, presentation: {} } as never)
  expect(next.text).toBe('mw-watch: fanfare calm.')
  expect(fanRuns(state)).toEqual(['user set fanfare.enabled=true fanfare.use=all', 'user set fanfare.use=calm'])
  await ui.unmount()
})

test('fanfare commands', async ($, on) => {
  const { state } = rig($, on, { extra: fanExtra() })
  await start($)
  await cmd($, 'fanfare on')
  await cmd($, 'fanfare off')
  await cmd($, 'fanfare use calm')
  expect(String((await cmd($, 'fanfare use nope')).text)).toContain('unknown set')
  await cmd($, 'fanfare test')
  expect(fanRuns(state)).toEqual(['user set fanfare.enabled=true', 'user set fanfare.enabled=false', 'user set fanfare.use=calm', 'fanfare test milestone'])
})

const oldMwChecks = async ($: any, on: any, extra: (argv: string[]) => unknown) => {
  const { state } = rig($, on, { extra })
  await start($)
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await ui.find({ key: 'fanfare' })).toBeUndefined()
  expect(await ui.find({ key: 'fanfare-test' })).toBeUndefined()
  expect(String((await cmd($, 'fanfare on')).text)).toContain('no fanfare')
  expect(fanRuns(state)).toEqual([])
  await ui.unmount()
}

test('fanfare footer is hidden when user show fails', async ($, on) => {
  await oldMwChecks($, on, argv => (argv.join(' ') === 'user show -j' ? raw('', 1) : argv.join(' ') === 'fanfare -j' ? ok(FAN) : undefined))
})

test('fanfare footer is hidden when fanfare -j lacks sets', async ($, on) => {
  await oldMwChecks($, on, argv => (argv.join(' ') === 'fanfare -j' ? ok({ config: { enabled: false } }) : argv.join(' ') === 'user show -j' ? ok({}) : undefined))
})

test('needs come from review -j asks when present, else the blocked-note heuristic', async ($, on) => {
  const asks = { needs: [{ tenant: 'alpha-web', item: 'A-3', question: 'Which format?', by: 'bot', at: iso(BASE - 600_000) }] }
  rig($, on, { extra: argv => (argv.join(' ') === 'review -j' ? ok(asks) : undefined) })
  await start($)
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await ui.find({ type: 'Text', text: /^! Which format\?/ })).toBeDefined()
  expect(await ui.find({ type: 'Text', text: /needs a decision/ })).toBeUndefined()
  expect(await ui.find({ type: 'Text', text: /^! 1 need you/ })).toBeDefined()
  await ui.unmount()
})

// ---- the rename from tidewatch ----

test('/tidewatch still works as an alias and says it is deprecated', async ($, on) => {
  rig($, on)
  await start($)
  const r = await $.command.run({ command: 'tidewatch', args: 'demo', origin: { kind: 'user' }, presentation: {} } as never)
  expect(r.text).toContain('`/tidewatch` is now `/mw-watch` (the old name goes in the next release).')
  expect((await cmd($, 'demo')).text).not.toContain('/tidewatch')
})

test('the old tidewatch store is imported once when the registry is empty', async ($, on) => {
  const { state } = rig($, on, { registry: null, oldStore: { registry: REG, style: 'blocks', fontStep: 1 } })
  await start($)
  expect(Object.keys((state.store.get('registry') as { jobs: object }).jobs)).toEqual(['alpha', 'demo'])
  expect(state.store.get('style')).toBe('blocks')
  expect(state.store.get('fontStep')).toBe(1)
})

test('a non-empty registry is never overwritten by the old store', async ($, on) => {
  const { state } = rig($, on, { oldStore: { registry: { jobs: { old: { board: '/boards/old', tenants: [] } } } } })
  await start($)
  expect(Object.keys((state.store.get('registry') as { jobs: object }).jobs)).toEqual(['alpha', 'demo'])
})

// ---- discovery: bare /mw-watch is the whole setup ----

const FOUND = (dir: string, name: string, tenants: string[], error = '') => ({ dir, path: `${dir}/.miniwaves.json`, name, schema: 9, tenants: tenants.map(t => ({ slug: t, name: t, archived: t === 'old' })), ...(error ? { error } : {}) })
const report = (root: string, boards: unknown[]) => ({ root, boards, truncated: false, reason: '' })
const boardRuns = (state: any) => state.runs.filter((r: string[]) => r[1] === 'boards').map((r: string[]) => r.slice(1).join(' '))

test('session start with an empty registry discovers the session folder board; no status line is drawn', async ($, on) => {
  const { state } = rig($, on, {
    registry: null,
    extra: argv => (argv[0] === 'boards' ? ok(report('/work', [FOUND('/work/proj', 'P', ['a', 'old', 'b']), FOUND('/work/bad', 'B', ['x'], 'unreadable')])) : undefined),
  })
  await start($)
  // found at session.start: no /mw-watch add, no ~ scan
  expect(boardRuns(state)).toEqual(['boards /work --up --depth 3 -j @/work'])
  expect(state.statuses).toEqual([])
  expect((await cmd($, '')).text).toBe('mw-watch pane opened.')
  const jobs = (state.store.get('registry') as { jobs: Record<string, { board: string; tenants: string[] }> }).jobs
  expect(jobs.proj).toMatchObject({ board: '/work/proj', tenants: ['a', 'b'] })
  expect(Object.keys(jobs)).toEqual(['proj'])
  expect(state.opened.at(-1)).toContain('mw-watch|')
})

test('nothing near the session folder: the home folder is scanned', async ($, on) => {
  const { state } = rig($, on, {
    registry: null,
    extra: argv => (argv[0] === 'boards' ? ok(report(argv[1]!, argv[1] === '/home/me' ? [FOUND('/home/me/apps/x', 'X', ['x'])] : [])) : undefined),
  })
  await start($)
  expect((await cmd($, '')).text).toContain('found 1 board (x)')
  // session start looks at the session folder only (cheap); the bare command goes on to the home folder
  expect(boardRuns(state)).toEqual(['boards /work --up --depth 3 -j @/work', 'boards /work --up --depth 3 -j @/work', 'boards /home/me --depth 4 -j @/home/me'])
})

test('an mw without `boards` falls back with a toast', async ($, on) => {
  const { state } = rig($, on, { registry: null, extra: argv => (argv[0] === 'boards' ? raw('', 2) : undefined) })
  await start($)
  expect((await cmd($, '')).text).toContain('update mw for auto-discovery')
  expect(state.toasts.some(t => t.includes('update mw for auto-discovery (mw update)'))).toBe(true)
})

test('scan lists boards not added yet; Add and Add all register them', async ($, on) => {
  const { state } = rig($, on, {
    extra: argv => (argv[0] === 'boards' ? ok(report('/scanroot', [FOUND('/boards/alpha', 'A', ['a']), FOUND('/scanroot/n1', 'N1', ['n']), FOUND('/scanroot/n2', 'N2', ['m'])])) : undefined),
  })
  await start($)
  const r = await cmd($, 'scan /scanroot')
  expect(r.text).toContain('found 2 boards not added yet: /scanroot/n1, /scanroot/n2')
  expect(boardRuns(state)).toEqual(['boards /scanroot --depth 4 -j @/scanroot'])
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await ui.find({ key: 'add-all' })).toBeDefined()
  await ui.press({ key: 'add-/scanroot/n1' })
  expect(Object.keys((state.store.get('registry') as { jobs: object }).jobs)).toEqual(['alpha', 'demo', 'n1'])
  await ui.unmount()
})

test('add PATH with no .miniwaves.json is a scan root', async ($, on) => {
  const { state } = rig($, on, { extra: argv => (argv[0] === 'boards' ? ok(report('/scanroot/x', [])) : undefined) })
  await start($)
  expect((await cmd($, 'add /scanroot/x')).text).toContain('no new boards under /scanroot/x')
  expect(boardRuns(state)).toEqual(['boards /scanroot/x --depth 4 -j @/scanroot/x'])
})

test('hide toggles a tenant of a board, never the last one', async ($, on) => {
  const { state } = rig($, on)
  await start($)
  expect((await cmd($, 'hide alpha alpha-web')).text).toContain('alpha-web hidden on alpha')
  expect((state.store.get('registry') as { jobs: Record<string, { hidden?: string[] }> }).jobs.alpha!.hidden).toEqual(['alpha-web'])
  expect((await cmd($, 'hide alpha alpha-core')).text).toContain('keep at least one')
  expect((await cmd($, 'hide alpha alpha-web')).text).toContain('alpha-web shown on alpha')
})

test('/mw-fanfare test says why it failed, in one line', async ($, on) => {
  const { state } = rig($, on, {
    extra: argv => (argv.join(' ') === 'fanfare test milestone' ? { value: { exitCode: 1, stdout: '', stderr: 'noise\nno player found\n', isStdoutTruncated: false, isStderrTruncated: false } } : fanExtra()(argv)),
  })
  await start($)
  const r = await $.command.run({ command: 'mw-fanfare', args: 'test', origin: { kind: 'user' }, presentation: {} } as never)
  expect(r.text).toBe('mw-watch: fanfare test failed: no player found')
  expect(state.toasts.at(-1)).toBe('mw-watch: fanfare test failed: no player found')
})

// ---- tier 1: every board the session used, listed first ----

const BOARDS_BY_DIR = (argv: string[]) => {
  if (argv[0] !== 'boards') return undefined
  const dir = argv[1]!
  const map: Record<string, unknown[]> = {
    '/work/a/sub': [FOUND('/work/a', 'A', ['a'])],
    '/work/b': [FOUND('/work/b', 'B', ['b'])],
    '/link/c': [FOUND('/link/c', 'C', ['c'])],
    '/real/c': [FOUND('/real/c', 'C', ['c'])],
    '/wt/d': [FOUND('/wt/d', 'D', ['d'])],
    '/work': [FOUND('/work/cwdboard', 'W', ['w'])],
  }
  return ok(report(dir, argv.includes('--up') && argv.includes('0') ? (map[dir] ?? []) : dir === '/work' ? map['/work']! : []))
}

test('every board the session used is found first, deduped by real path, marked this session', async ($, on) => {
  const { state } = rig($, on, {
    registry: null,
    messages: [
      { role: 'assistant', text: '', toolUses: [{ tool: 'Bash', input: { command: 'cd /work/a/sub && mw list' } }] },
      { role: 'assistant', text: '', toolUses: [{ tool: 'Edit', input: { file_path: '/work/b/.miniwaves.json' } }, { tool: 'Bash', input: { command: 'ls /nowhere' } }] },
    ],
    extra: BOARDS_BY_DIR,
  })
  await start($)
  // live calls, one from a subagent, one through a symlink to a board already seen
  await $.tool.call({ tool: 'Bash', command: 'mw -C /link/c ls', agentId: 'sub1' } as never)
  await $.tool.call({ tool: 'Bash', command: 'mw -C /real/c ls' } as never)
  await $.tool.call({ tool: 'Write', file_path: '/wt/d/.miniwaves/x' } as never)
  // session start already registered the cheap finds; empty the registry so the bare command discovers again
  for (const id of ['a', 'b', 'cwdboard']) await cmd($, `remove ${id}`)
  const text = (await cmd($, '')).text
  expect(text).toBe('mw-watch: found 5 boards (a [this session], b [this session], c [this session], d [this session], cwdboard).')
  expect(Object.keys((state.store.get('registry') as { jobs: object }).jobs)).toEqual(['a', 'b', 'c', 'd', 'cwdboard'])
  expect(boardRuns(state).every((r: string) => !r.includes('/home/me'))).toBe(true)
})

test('the tool.call hook only observes: the call goes through unchanged', async ($, on) => {
  rig($, on, { extra: BOARDS_BY_DIR })
  await start($)
  const input = { tool: 'Bash', command: 'cd /work/b && mw list' }
  const out = await $.tool.call(input as never)
  expect(JSON.stringify(out)).not.toContain('deny')
  expect(input).toEqual({ tool: 'Bash', command: 'cd /work/b && mw list' })
})

// ---- per-user settings (`mw user get -j`) ----

const userGet = (vals: Record<string, string>) => ({ keys: Object.entries(vals).map(([key, value]) => ({ key, value, default: value, source: 'user', group: key.split('.')[0], choices: [] })) })
const withUser = (vals: Record<string, string> | 'fail', more?: (argv: string[]) => unknown) => (argv: string[]) => {
  if (argv.join(' ') === 'user get -j') return vals === 'fail' ? raw('', 2) : ok(userGet(vals))
  return more?.(argv)
}

test('user settings: discover.home=false means no home scan', async ($, on) => {
  const { state } = rig($, on, { registry: null, extra: withUser({ 'discover.home': 'false' }, argv => (argv[0] === 'boards' ? ok(report(argv[1]!, [])) : undefined)) })
  await start($)
  await cmd($, '')
  expect(boardRuns(state).some((r: string) => r.includes('/home/me'))).toBe(false)
})

test('user settings: discover.depth is passed through', async ($, on) => {
  const { state } = rig($, on, { registry: null, extra: withUser({ 'discover.depth': '2' }, argv => (argv[0] === 'boards' ? ok(report(argv[1]!, [])) : undefined)) })
  await start($)
  await cmd($, '')
  expect(boardRuns(state)).toContain('boards /home/me --depth 2 -j @/home/me')
})

test('user settings: an older mw (user get fails) keeps the defaults', async ($, on) => {
  const { state } = rig($, on, { registry: null, extra: withUser('fail', argv => (argv[0] === 'boards' ? ok(report(argv[1]!, [])) : undefined)) })
  await start($)
  await cmd($, '')
  expect(boardRuns(state)).toContain('boards /home/me --depth 4 -j @/home/me')
  expect(state.toasts.length).toBe(0)
})

const alertRun = async ($: any, on: any, vals: Record<string, string>) => {
  const { state, clock } = rig($, on, { extra: withUser(vals) })
  await start($)
  await cmd($, 'every 1m')
  state.round = 1
  await clock.advance(60_000)
  return state
}

test('user settings: alerts.enabled=false suppresses sound and toasts', async ($, on) => {
  const state = await alertRun($, on, { 'alerts.enabled': 'false' })
  expect(state.sounds.length).toBe(0)
  expect(state.toasts.length).toBe(0)
})

test('user settings: alerts.sound=false and alerts.kinds filter', async ($, on) => {
  expect((await alertRun($, on, { 'alerts.sound': 'false' })).sounds.length).toBe(0)
})

test('user settings: alerts.kinds without blocked/ask stays silent', async ($, on) => {
  expect((await alertRun($, on, { 'alerts.kinds': 'done,release' })).sounds.length).toBe(0)
})

test('user settings: pane.refresh=off means no poll timer; a saved override wins', async ($, on) => {
  const { state, clock } = rig($, on, { extra: withUser({ 'pane.refresh': 'off' }) })
  await start($)
  const calls = state.alphaCalls
  await clock.advance(600_000)
  expect(state.alphaCalls).toBe(calls)
  await cmd($, 'every 30s')
  await clock.advance(30_000)
  expect(state.alphaCalls).toBeGreaterThan(calls)
})

test('user settings: pane.refresh=15s seeds the timer', async ($, on) => {
  const { state, clock } = rig($, on, { extra: withUser({ 'pane.refresh': '15s' }) })
  await start($)
  const calls = state.alphaCalls
  await clock.advance(15_000)
  expect(state.alphaCalls).toBeGreaterThan(calls)
})
