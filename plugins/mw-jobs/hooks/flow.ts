// Agile and Kanban views as plain data: one document per view, drawn as selectable Text by the pane.
import type { MItem, Model } from '../types'

export type FRow = { tenant: string; id: string; title: string; ticket: string; ticketUrl: string; since: string; flag: string; ask: boolean; note: string }
export type FSection = { key: string; title: string; count: number; rows: FRow[]; rest: number; tone?: string; sub?: string }
export type FDoc = { notice?: string; dim: boolean; head?: { title: string; done: number; total: number; pct: number }; sections: FSection[] }

const key = (i: { tenant: string; id: string }) => `${i.tenant}:${i.id}`
const BACKLOG_NOTICE = 'Backlog needs the newer mw (no now.backlog in the model)'

/** - blocked (as in gopher), ! needs you; mw marks asks on now.needs and the item state on the sprint. */
const rowOf = (i: MItem, m: Model, ask = false): FRow => {
  const needs = i.needs || m.now.needs.items.some(n => key(n) === key(i))
  const blocked = i.state === 'blocked' || m.now.blocked.items.some(n => key(n) === key(i))
  return {
    tenant: i.tenant,
    id: i.id,
    title: i.title,
    ticket: i.ticket,
    ticketUrl: i.ticket_url,
    since: i.since,
    flag: `${blocked ? '-' : ''}${needs ? '!' : ''}`,
    ask: ask || needs,
    note: i.note,
  }
}

const cut = (items: MItem[], m: Model, top: number, total: number, ask = false): { rows: FRow[]; rest: number } => {
  const shown = items.slice(0, top)
  return { rows: shown.map(i => rowOf(i, m, ask)), rest: Math.max(0, total - shown.length) }
}

const dedupe = (items: MItem[]): MItem[] => {
  const seen = new Set<string>()
  return items.filter(i => (seen.has(key(i)) ? false : (seen.add(key(i)), true)))
}

const sec = (k: string, title: string, items: MItem[], m: Model, o: { top?: number; count?: number; tone?: string; sub?: string; ask?: boolean } = {}): FSection => {
  const count = o.count ?? items.length
  const c = cut(items, m, o.top ?? 10, count, o.ask)
  return { key: k, title, count, rows: c.rows, rest: c.rest, tone: o.tone, sub: o.sub }
}

const backlogOf = (m: Model) => {
  const items = m.now.backlog.items
  const ready = items.filter(i => i.state === 'ready')
  const drafts = items.filter(i => i.state !== 'ready')
  return { ready, drafts, count: m.now.backlog.count }
}

const unknownBacklog = (m: Model) => m.now.backlogKnown === false

/** Jobs, Agile: the sprint (To do / In progress / Review / Done) and the backlog (ready first, then drafts). */
export const agileJobs = (m: Model, top = 10): FDoc => {
  const sp = m.now.sprint
  const dim = unknownBacklog(m)
  const notices = [dim ? BACKLOG_NOTICE : '', sp === null ? 'No open sprint' : ''].filter(Boolean)
  const by = (st: string[]) => (sp ? sp.items.filter(i => st.includes(i.state)) : [])
  const sections: FSection[] = []
  if (sp !== null) {
    sections.push(sec('todo', 'To do', by(['todo', 'ready', 'open']), m, { top }))
    const prog = dedupe([...by(['doing', 'blocked']), ...m.now.needs.items.filter(n => sp.items.some(i => key(i) === key(n)))])
    sections.push(sec('doing', 'In progress', prog, m, { top, tone: 'warning' }))
    sections.push(sec('review', 'Review', by(['review']), m, { top, tone: 'warning' }))
    sections.push(sec('done', 'Done', by(['done']), m, { top, tone: 'success' }))
  }
  const b = backlogOf(m)
  const ordered = [...b.ready, ...b.drafts]
  sections.push(sec('backlog', 'Backlog', ordered, m, { top, count: dim ? 0 : Math.max(b.count, ordered.length), sub: dim ? undefined : `${m.now.counts.ready ?? b.ready.length} ready · ${m.now.counts.draft ?? b.drafts.length} draft` }))
  return {
    notice: notices.length > 0 ? notices.join(' · ') : undefined,
    dim,
    head: sp ? { title: `${sp.id} ${sp.title}`.trim(), done: sp.done, total: sp.total, pct: sp.total > 0 ? Math.round((sp.done / sp.total) * 100) : 0 } : undefined,
    sections,
  }
}

/** Jobs, Kanban: Backlog (drafts) · Ready · In progress · Review · Done (recent). */
export const kanbanJobs = (m: Model, top = 8): FDoc => {
  const dim = unknownBacklog(m)
  const b = backlogOf(m)
  const prog = dedupe([...m.now.needs.items, ...m.now.doing.items, ...m.now.blocked.items])
  const progCount = prog.length + m.now.needs.more + m.now.doing.more + m.now.blocked.more
  return {
    notice: dim ? BACKLOG_NOTICE : undefined,
    dim,
    sections: [
      sec('backlog', 'Backlog', b.drafts, m, { top, count: dim ? 0 : m.now.counts.draft ?? b.drafts.length }),
      sec('ready', 'Ready', b.ready.length > 0 ? b.ready : m.next.ready.items, m, { top, count: m.now.counts.ready ?? b.ready.length, tone: 'suggestion' }),
      sec('doing', 'In progress', prog, m, { top, count: progCount, tone: 'warning' }),
      sec('review', 'Review', m.now.review.items, m, { top, count: m.now.counts.review ?? m.now.review.items.length, tone: 'warning', ask: true }),
      sec('done', 'Done', m.now.recentDone.items, m, { top, count: m.now.recentDone.items.length + m.now.recentDone.more, tone: 'success' }),
    ],
  }
}

/** News, Agile: the sprint review. Done grouped by milestone, carry-over, backlog. */
export const agileNews = (m: Model, top = 10): FDoc => {
  const dim = unknownBacklog(m)
  const groups = new Map<string, MItem[]>()
  for (const i of m.happened.done.items) {
    const g = i.milestone !== '' ? `${i.milestone} ${i.milestone_title}`.trim() : 'No milestone'
    groups.set(g, [...(groups.get(g) ?? []), i])
  }
  const sections: FSection[] = [...groups.entries()].map(([g, items], n) => sec(`done-${n}`, `Done · ${g}`, items, m, { top, tone: 'success' }))
  if (sections.length === 0) sections.push(sec('done', 'Done', [], m, { tone: 'success' }))
  const carry = m.now.sprint ? m.now.sprint.items.filter(i => i.state !== 'done') : []
  sections.push(sec('carry', 'Carry-over', carry, m, { top, tone: 'warning' }))
  const b = backlogOf(m)
  sections.push(sec('backlog', 'Backlog', [...b.ready, ...b.drafts], m, { top, count: dim ? 0 : Math.max(b.count, b.ready.length + b.drafts.length) }))
  return { notice: dim ? BACKLOG_NOTICE : undefined, dim, sections }
}

/** News, Kanban: the flow of the window. Moved to Done / Review / In progress, added to Backlog. */
export const kanbanNews = (m: Model, top = 8): FDoc => {
  const h = m.happened
  const c = (l: { items: MItem[]; more: number }) => l.items.length + l.more
  return {
    dim: false,
    sections: [
      sec('added', 'Added to Backlog', h.new_items.items, m, { top, count: c(h.new_items) }),
      sec('doing', 'Moved to In progress', h.moved_to_doing.items, m, { top, count: c(h.moved_to_doing), tone: 'warning' }),
      sec('review', 'Moved to Review', h.moved_to_review.items, m, { top, count: c(h.moved_to_review), tone: 'warning' }),
      sec('done', 'Moved to Done', h.done.items, m, { top, count: c(h.done), tone: 'success' }),
    ],
  }
}

/** Plain text of a document, one line per row: what a selection or a copy of the pane gives. */
export const flowText = (d: FDoc): string[] => [
  ...(d.notice ? [d.notice] : []),
  ...d.sections.flatMap(s => [`${s.title.toUpperCase()} ${s.count}`, ...s.rows.map(r => `  ${r.flag}${r.flag ? ' ' : ''}${r.ticket || `${r.tenant}:${r.id}`} ${r.title}`), ...(s.rest > 0 ? [`  +${s.rest} more`] : [])]),
]
