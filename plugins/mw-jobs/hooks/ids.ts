// Ids in text: item ids (TSK-057, BUG-12, S·A-9), milestone ids (WAV-002, M4 when it is a wave's label). Pure.

export type IdRef = { kind: 'item' | 'wave'; tenant: string; id: string }
export type IdIndex = Map<string, IdRef>
export type IdPart = { text: string; ref?: IdRef }

/** An id as written in text: PREFIX-123, the short `S·A-9` form (tenant label, middle dot), or a wave label like M4. */
const ID_RE = /(?:[A-Za-z0-9]{1,8}·)?(?:\b[A-Z][A-Z0-9]{0,5}-\d{1,6}\b|\b[A-Z]{1,3}\d{1,3}\b)/g

/**
 * The ids that exist on a board. An item id maps to its first tenant; a wave id (`tenant:WAV-1`) to its tenant; a wave's label (the first
 * word of its title, like M4) to the wave when exactly one wave has it. An id that is not here stays plain text.
 */
export const buildIdIndex = (items: Array<{ tenant: string; id: string }>, waves: Array<{ id: string; title?: string }>): IdIndex => {
  const index: IdIndex = new Map()
  for (const i of items) if (!index.has(i.id)) index.set(i.id, { kind: 'item', tenant: i.tenant, id: i.id })
  const labels = new Map<string, IdRef | null>()
  for (const w of waves) {
    const c = w.id.indexOf(':')
    const tenant = c >= 0 ? w.id.slice(0, c) : ''
    const bare = c >= 0 ? w.id.slice(c + 1) : w.id
    if (!index.has(bare)) index.set(bare, { kind: 'wave', tenant, id: w.id })
    const tok = (w.title ?? '').split(' ')[0] ?? ''
    if (/^[A-Z]{1,3}\d{1,3}$/.test(tok)) labels.set(tok, labels.has(tok) ? null : { kind: 'wave', tenant, id: w.id })
  }
  for (const [tok, ref] of labels) if (ref !== null && !index.has(tok)) index.set(tok, ref)
  return index
}

/** The text cut into plain parts and linked ids (only ids in the index are linked). */
export const splitIds = (text: string, index: IdIndex): IdPart[] => {
  const out: IdPart[] = []
  let last = 0
  for (const m of text.matchAll(ID_RE)) {
    const at = m.index ?? 0
    const token = m[0]
    const bare = token.includes('·') ? token.slice(token.indexOf('·') + 1) : token
    const ref = index.get(bare)
    if (ref === undefined) continue
    if (at > last) out.push({ text: text.slice(last, at) })
    out.push({ text: token, ref })
    last = at + token.length
  }
  if (last < text.length) out.push({ text: text.slice(last) })
  return out
}
