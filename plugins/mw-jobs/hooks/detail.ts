// The detail panel's data: `mw show ID -t T -j` (an item) and `mw wave show ID -t T -j` (a milestone), read only, parsed here. Pure.

export type DLog = { at: string; action: string; by: string; note: string }
export type DItem = {
  id: string
  title: string
  kind: string
  state: string
  tenant: string
  wave: string
  owner: string
  /** The latest block reason (blocked), the done proof (done), or the last note. */
  reason: { label: string; text: string } | null
  evidence: string[]
  ticket: { label: string; url: string } | null
  body: string
  acceptance: string
  children: { id: string; kind: string; state: string; title: string }[]
  /** Newest first. */
  log: DLog[]
}
export type DWaveItem = { id: string; kind: string; state: string; title: string }
export type DWave = { id: string; title: string; tenant: string; state: string; done: number; total: number; pct: number; groups: { state: string; items: DWaveItem[] }[] }

const s = (v: unknown): string => (typeof v === 'string' ? v : typeof v === 'number' ? String(v) : '')
const obj = (text: string | undefined): Record<string, unknown> | undefined => {
  if (text === undefined) return undefined
  try {
    const v = JSON.parse(text)
    return v !== null && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : undefined
  } catch {
    return undefined
  }
}
const list = (v: unknown): Record<string, unknown>[] => (Array.isArray(v) ? v.filter(x => x !== null && typeof x === 'object') : []) as Record<string, unknown>[]

export const LOG_SHOWN = 8

/** `mw show -j` as the panel's item, or undefined when it is not that JSON. */
export const parseItemDetail = (text: string | undefined, tenant: string): DItem | undefined => {
  const v = obj(text)
  if (v === undefined || s(v.id) === '') return undefined
  const events = list(v.events).map(e => ({ at: s(e.at), action: s(e.action), by: s(e.by), note: s(e.note) }))
  const state = s(v.state)
  const lastOf = (action: string) => [...events].reverse().find(e => e.action === action)
  const note = s(v.note) || [...(Array.isArray(v.notes) ? v.notes : [])].map(n => (typeof n === 'string' ? n : s((n as Record<string, unknown>)?.text))).filter(Boolean).at(-1) || ''
  let reason: DItem['reason'] = null
  if (state === 'blocked') reason = { label: 'Blocked', text: lastOf('block')?.note || s(v.block_reason) || note || '(no reason given)' }
  else if (state === 'done') reason = { label: 'Done proof', text: lastOf('done')?.note || note || '(no note)' }
  else if (note !== '') reason = { label: 'Note', text: note }
  const owner = s(v.by) || s(v.session) || lastOf('start')?.by || lastOf('claim')?.by || ''
  const tl = s(v.ticket_label) || s(v.ticket)
  const tu = s(v.ticket_url) || s(v.ticketUrl)
  return {
    id: s(v.id),
    title: s(v.title),
    kind: s(v.type) || s(v.kind),
    state,
    tenant: s(v.tenant) || tenant,
    wave: s(v.wave) || s(v.milestone),
    owner,
    reason,
    evidence: (Array.isArray(v.evidence) ? v.evidence : []).map(s).filter(Boolean),
    ticket: tl !== '' || tu !== '' ? { label: tl || tu, url: tu } : null,
    body: s(v.body),
    acceptance: s(v.acceptance),
    children: list(v.children).map(c => ({ id: s(c.id), kind: s(c.type), state: s(c.state), title: s(c.title) })),
    log: events.reverse().slice(0, LOG_SHOWN),
  }
}

/** The order of the state groups in a milestone panel: what needs a look first, done last. */
export const STATE_ORDER = ['blocked', 'needs', 'review', 'doing', 'ready', 'inbox', 'draft', 'done']

/** `mw wave show -j` as the panel's milestone: done/total from members ("6/6"), items grouped by state. */
export const parseWaveDetail = (text: string | undefined, tenant: string): DWave | undefined => {
  const v = obj(text)
  if (v === undefined || s(v.id) === '') return undefined
  const items = list(v.member_items ?? v.items).map(i => ({ id: s(i.id), kind: s(i.type), state: s(i.state), title: s(i.title) }))
  const m = /^(\d+)\s*\/\s*(\d+)$/.exec(s(v.members))
  const done = m ? Number(m[1]) : items.filter(i => i.state === 'done').length
  const total = m ? Number(m[2]) : items.length
  const states = [...new Set(items.map(i => i.state))].sort((a, b) => (STATE_ORDER.indexOf(a) < 0 ? 99 : STATE_ORDER.indexOf(a)) - (STATE_ORDER.indexOf(b) < 0 ? 99 : STATE_ORDER.indexOf(b)) || a.localeCompare(b))
  return {
    id: s(v.id),
    title: s(v.title),
    tenant: s(v.tenant) || tenant,
    state: s(v.state),
    done,
    total,
    pct: typeof v.percent === 'number' ? v.percent : total === 0 ? 0 : Math.round((done / total) * 100),
    groups: states.map(st => ({ state: st, items: items.filter(i => i.state === st) })),
  }
}
