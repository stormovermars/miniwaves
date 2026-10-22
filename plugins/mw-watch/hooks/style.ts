import type { TidewatchColor, TidewatchColorName, TidewatchSectionId, TidewatchStyle } from '../types'

export type Style = TidewatchStyle

export const COLOR_NAMES: TidewatchColorName[] = ['fg', 'dim', 'track', 'needs', 'waiting', 'progress', 'new', 'pass', 'fail']
export const SECTION_IDS: TidewatchSectionId[] = ['needs', 'review', 'progress', 'milestones', 'harness', 'events']

const allShown = (): Record<TidewatchSectionId, boolean> => ({
  needs: true,
  review: true,
  progress: true,
  milestones: true,
  harness: true,
  events: true,
})

/** The built-in `normal` style: sans, 6px rounded bars. styles/normal.json mirrors it. */
export const NORMAL: Style = {
  font: 'sans',
  sizes: { headline: 18, body: 13, note: 12, label: 11 },
  rowHeight: 22,
  columns: { name: 150, count: 56 },
  bar: { kind: 'rect', height: 6, radius: 3 },
  colors: {
    fg: { light: '#18181B', dark: '#F4F4F5' },
    dim: { light: '#5B5B66', dark: '#A1A1AA' },
    track: { light: '#E4E4E7', dark: '#3F3F46' },
    needs: { light: '#B91C1C', dark: '#F87171' },
    waiting: { light: '#A16207', dark: '#FBBF24' },
    progress: { light: '#18181B', dark: '#F4F4F5' },
    new: { light: '#2563EB', dark: '#60A5FA' },
    pass: { light: '#15803D', dark: '#4ADE80' },
    fail: { light: '#B91C1C', dark: '#F87171' },
  },
  sections: { order: [...SECTION_IDS], show: allShown() },
  scale: [0.85, 1, 1.15],
}

/** The built-in `blocks` style: monospace, block-glyph bars. styles/blocks.json mirrors it. */
export const BLOCKS: Style = {
  font: 'mono',
  sizes: { headline: 12, body: 12, note: 12, label: 12 },
  rowHeight: 19.2,
  columns: { name: 22, count: 6 },
  bar: { kind: 'glyphs', fill: '█', rest: '░' },
  colors: {
    fg: { light: '#18181B', dark: '#F4F4F5' },
    dim: { light: '#5B5B66', dark: '#A1A1AA' },
    track: { light: '#D4D4D8', dark: '#3F3F46' },
    needs: { light: '#B91C1C', dark: '#F87171' },
    waiting: { light: '#A16207', dark: '#FBBF24' },
    progress: { light: '#18181B', dark: '#F4F4F5' },
    new: { light: '#15803D', dark: '#4ADE80' },
    pass: { light: '#15803D', dark: '#4ADE80' },
    fail: { light: '#B91C1C', dark: '#F87171' },
  },
  sections: { order: [...SECTION_IDS], show: allShown() },
  scale: [0.92, 1, 1.17],
}

export const BUILTIN: Record<string, Style> = { normal: NORMAL, blocks: BLOCKS }
export const BUILTIN_NAMES = ['normal', 'blocks']

/** A style name: lowercase letters, digits and hyphens, 1 to 32. */
export const isStyleName = (n: string) => /^[a-z0-9-]{1,32}$/.test(n)

const clone = <T,>(v: T): T => JSON.parse(JSON.stringify(v)) as T
const isObj = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === 'object' && !Array.isArray(v)
const HEX = /^#[0-9a-fA-F]{3,8}$/

/**
 * Merges a user style (parsed JSON) over `base`. Unknown keys and bad values are listed in `issues` and
 * ignored; the result is always a complete, valid style. Never throws.
 */
export const mergeStyle = (base: Style, input: unknown): { style: Style; issues: string[] } => {
  const out = clone(base)
  const issues: string[] = []
  if (!isObj(input)) return { style: out, issues: ['the file is not a JSON object'] }

  const num = (path: string, v: unknown, lo: number, hi: number): number | undefined => {
    if (typeof v === 'number' && Number.isFinite(v)) {
      if (v >= lo && v <= hi) return v
      issues.push(`${path}: ${v} clamped to ${lo}..${hi}`)
      return Math.max(lo, Math.min(hi, v))
    }
    issues.push(`${path}: ${JSON.stringify(v)} is not a finite number`)
    return undefined
  }
  const group = (path: string, v: unknown, keys: readonly string[]): Record<string, unknown> | undefined => {
    if (!isObj(v)) {
      issues.push(`${path}: expected an object`)
      return undefined
    }
    const ok: Record<string, unknown> = {}
    for (const k of Object.keys(v)) {
      if (keys.includes(k)) ok[k] = v[k]
      else issues.push(`${path}.${k}: unknown key`)
    }
    return ok
  }

  for (const key of Object.keys(input)) {
    const v = input[key]
    if (key === 'font') {
      if (v === 'sans' || v === 'mono') out.font = v
      else issues.push(`font: ${JSON.stringify(v)} is not "sans" or "mono"`)
    } else if (key === 'sizes') {
      const g = group('sizes', v, ['headline', 'body', 'note', 'label'])
      for (const k of Object.keys(g ?? {}) as (keyof Style['sizes'])[]) {
        const n = num(`sizes.${k}`, g![k], 6, 40)
        if (n !== undefined) out.sizes[k] = n
      }
    } else if (key === 'rowHeight') {
      const n = num('rowHeight', v, 8, 60)
      if (n !== undefined) out.rowHeight = n
    } else if (key === 'columns') {
      const g = group('columns', v, ['name', 'count'])
      for (const k of Object.keys(g ?? {}) as (keyof Style['columns'])[]) {
        const n = num(`columns.${k}`, g![k], k === 'name' ? 4 : 2, 400)
        if (n !== undefined) out.columns[k] = n
      }
    } else if (key === 'bar') {
      if (!isObj(v)) {
        issues.push('bar: expected an object')
        continue
      }
      const kind = v.kind === undefined ? out.bar.kind : v.kind
      if (kind !== 'rect' && kind !== 'glyphs') {
        issues.push(`bar.kind: ${JSON.stringify(v.kind)} is not "rect" or "glyphs"`)
        continue
      }
      const keys = kind === 'rect' ? ['kind', 'height', 'radius'] : ['kind', 'fill', 'rest']
      for (const k of Object.keys(v)) if (!keys.includes(k)) issues.push(`bar.${k}: unknown key for kind "${kind}"`)
      if (kind === 'rect') {
        const cur = out.bar.kind === 'rect' ? out.bar : { height: 6, radius: 3 }
        const height = v.height === undefined ? cur.height : num('bar.height', v.height, 1, 40)
        const radius = v.radius === undefined ? cur.radius : num('bar.radius', v.radius, 0, 20)
        out.bar = { kind: 'rect', height: height ?? cur.height, radius: radius ?? cur.radius }
      } else {
        const cur = out.bar.kind === 'glyphs' ? out.bar : { fill: '█', rest: '░' }
        const glyph = (name: string, g: unknown, fallback: string) => {
          if (g === undefined) return fallback
          if (typeof g === 'string' && [...g].length === 1 && !'<>&"\''.includes(g)) return g
          issues.push(`bar.${name}: ${JSON.stringify(g)} is not one character (and not one of <>&"')`)
          return fallback
        }
        out.bar = { kind: 'glyphs', fill: glyph('fill', v.fill, cur.fill), rest: glyph('rest', v.rest, cur.rest) }
      }
    } else if (key === 'colors') {
      const g = group('colors', v, COLOR_NAMES)
      for (const name of Object.keys(g ?? {}) as TidewatchColorName[]) {
        const pair = group(`colors.${name}`, g![name], ['light', 'dark'])
        for (const mode of Object.keys(pair ?? {}) as (keyof TidewatchColor)[]) {
          const c = pair![mode]
          if (typeof c === 'string' && HEX.test(c)) out.colors[name][mode] = c
          else issues.push(`colors.${name}.${mode}: ${JSON.stringify(c)} is not a #hex color`)
        }
      }
    } else if (key === 'sections') {
      const g = group('sections', v, ['order', 'show'])
      if (g?.order !== undefined) {
        const list = g.order
        if (Array.isArray(list) && list.every(x => SECTION_IDS.includes(x as TidewatchSectionId))) {
          const ids = [...new Set(list as TidewatchSectionId[])]
          out.sections.order = [...ids, ...SECTION_IDS.filter(s => !ids.includes(s))]
        } else issues.push(`sections.order: expected a list of ${SECTION_IDS.join(', ')}`)
      }
      if (g?.show !== undefined) {
        const s = group('sections.show', g.show, SECTION_IDS)
        for (const id of Object.keys(s ?? {}) as TidewatchSectionId[]) {
          if (typeof s![id] === 'boolean') out.sections.show[id] = s![id] as boolean
          else issues.push(`sections.show.${id}: expected true or false`)
        }
      }
    } else if (key === 'scale') {
      if (Array.isArray(v) && v.length === 3 && v.every(n => typeof n === 'number' && Number.isFinite(n))) {
        const c = (n: number) => Math.max(0.5, Math.min(2, n))
        out.scale = [c(v[0]), c(v[1]), c(v[2])]
        if (v.some(n => n < 0.5 || n > 2)) issues.push('scale: values clamped to 0.5..2')
      } else issues.push('scale: expected three finite numbers (smaller, default, bigger)')
    } else issues.push(`${key}: unknown key`)
  }
  return { style: out, issues }
}

/** -/#/+ pick the style's three scale factors. */
export const scaleFor = (s: Style, step: number) => (step < 0 ? s.scale[0] : step > 0 ? s.scale[2] : s.scale[1])

const cssVars = (mode: 'light' | 'dark', s: Style) => {
  const c = s.colors
  const pick = (n: TidewatchColorName) => c[n][mode]
  return `--fg:${pick('fg')};--dim:${pick('dim')};--track:${pick('track')};--needs:${pick('needs')};--wait:${pick('waiting')};--prog:${pick('progress')};--new:${pick('new')};--pass:${pick('pass')};--fail:${pick('fail')}`
}

/** The colour variables and classes both Svg builders share, light and dark. */
export const colorCss = (s: Style) =>
  `:root{${cssVars('light', s)}}\n@media (prefers-color-scheme:dark){:root{${cssVars('dark', s)}}}\n` +
  '.fg{fill:var(--fg)}.dim{fill:var(--dim)}.needs{fill:var(--needs)}.wait{fill:var(--wait)}.pass{fill:var(--pass)}.fail{fill:var(--fail)}.new{fill:var(--new)}.track{fill:var(--track)}.prog{fill:var(--prog)}'

/** Whether a colour of a user style differs from `normal`: only then does the terminal use its raw value. */
export const isCustomColor = (name: string, s: Style, c: TidewatchColorName) => {
  if (BUILTIN[name] !== undefined) return false
  return s.colors[c].dark !== NORMAL.colors[c].dark || s.colors[c].light !== NORMAL.colors[c].light
}
