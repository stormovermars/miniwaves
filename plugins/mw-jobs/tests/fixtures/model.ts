// Hand-written news models (docs/specs/news-model.md, version 1) for the mw-jobs tests.
// Lists are { items, more }: mw cuts a list at its cap and says how many it left out.

type It = { tenant: string; id: string; title: string; note?: string; since?: string; by?: string; done?: number; total?: number }
const L = (items: It[], more = 0) => ({ items, more })

const job = { id: 'zeta', title: 'Zeta consolidation', description: 'Consolidate ZetaSheet + ZetaDoc.', url: '' }

const METRICS = {
  updated: '2026-10-22T09:00:00',
  sections: [
    {
      title: 'Corpus parity',
      kind: 'split',
      legend: { good: 'pass', warn: 'partial', bad: 'fail', none: 'untested' },
      rows: [
        { name: 'sheet formulas', good: 40, warn: 10, bad: 5, none: 13, delta: { good: 2, bad: -1 } },
        { name: 'doc import', good: 20, warn: 0, bad: 3, none: 0, delta: { good: -1 } },
      ],
    },
    { title: 'Burndown', kind: 'progress', legend: { good: 'done', warn: 'x', bad: 'y', none: 'z' }, rows: [{ name: 'zetasheet', done: 5, total: 68, delta: { done: 2 } }, { name: 'zetadoc', done: 0, total: 0 }] },
    { title: 'Speed', kind: 'value', legend: { good: 'a', warn: 'b', bad: 'c', none: 'd' }, rows: [{ name: 'p95 ms', value: 120, unit: 'ms', target: 100, better: 'lower', delta: { value: -8 } }] },
  ],
}

/** The draft since release 6: the small fixture. */
export const SMALL_DRAFT = JSON.stringify({
  version: 1,
  job,
  release: { number: null, at: '2026-10-22T10:00:00', since: 6 },
  now: {
    counts: { needs: 2, review: 1, blocked: 1, doing: 2, ready: 3, done: 31, total: 68 },
    needs: L([
      { tenant: 'zetadoc', id: 'TSK-021', title: 'Pick the default image format', note: 'PNG or WebP for inline images?', since: '2026-10-22T09:30:00', by: 'zetadoc-session' },
      { tenant: 'zetasheet', id: 'TSK-044', title: 'Decide the rounding rule', note: 'banker or half-up', since: '2026-10-22T09:45:00', by: 'zetasheet-session' },
    ]),
    review: L([{ tenant: 'zetadoc', id: 'TSK-013', title: 'Inline images on iOS', note: 'awaiting review', since: '2026-10-22T08:00:00', by: 'x' }]),
    blocked: L([{ tenant: 'zetasheet', id: 'BUG-009', title: 'Tab accepts completion', note: 'needs the corpus build', since: '2026-10-21T18:00:00', by: 'y' }]),
    doing: L([
      { tenant: 'zetasheet', id: 'TSK-050', title: 'Column fill from formula', since: '2026-10-22T09:55:00', by: 'a' },
      { tenant: 'zetadoc', id: 'TSK-051', title: 'Heading numbering', since: '2026-10-22T09:20:00', by: 'b' },
    ]),
    milestones: { open: L([
      { tenant: 'zetasheet', id: 'WAV-002', title: 'Formulas', done: 40, total: 50 },
      { tenant: 'zetadoc', id: 'WAV-003', title: 'Import', done: 5, total: 20 },
    ]), closed_count: 1 },
  },
  happened: {
    closed_milestones: L([{ tenant: 'zetasheet', id: 'WAV-001', title: 'Corpus' }]),
    done: L([{ tenant: 'zetadoc', id: 'TSK-013', title: 'Inline images on iOS with a very long title that has to be cut by the bulletin to fit the line' }, { tenant: 'zetasheet', id: 'TSK-030', title: 'Sort' }]),
    accepted: L([{ tenant: 'zetasheet', id: 'TSK-031', title: 'Filter' }]),
    sent_back: L([{ tenant: 'zetadoc', id: 'TSK-040', title: 'Tables' }]),
    answers: L([{ tenant: 'zetasheet', id: 'TSK-033', title: 'Locale', by: 'Sir' }]),
    decisions: L([{ tenant: 'zetadoc', id: 'TSK-034', title: 'Use CommonMark' }]),
    new_items: L([{ tenant: 'zetadoc', id: 'TSK-060', title: 'Footnotes' }]),
    counts: { closed_milestones: 1, done: 2, accepted: 1, sent_back: 1, answers: 1, decisions: 1, new_items: 1 },
  },
  next: {
    ready: L([{ tenant: 'zetasheet', id: 'TSK-070', title: 'Pivot tables' }, { tenant: 'zetadoc', id: 'TSK-071', title: 'Comments' }, { tenant: 'zetadoc', id: 'TSK-072', title: 'Track changes' }]),
    next_milestones: L([{ tenant: 'zetasheet', id: 'WAV-002', title: 'Formulas', done: 40, total: 50 }]),
  },
  metrics: METRICS,
  summary: '',
})

/** A frozen release (number 6, 2026-10-20). */
export const SMALL_RELEASE = (n = 6, at = '2026-10-20T10:00:00') =>
  JSON.stringify({
    version: 1,
    job,
    release: { number: n, at, since: n - 1 },
    now: { counts: { needs: 1, review: 0, blocked: 0, doing: 1, ready: 2, done: 20, total: 68 }, needs: L([{ tenant: 'zetadoc', id: 'TSK-021', title: 'Pick the default image format', note: 'PNG or WebP?', since: at, by: 'z' }]), review: L([]), blocked: L([]), doing: L([{ tenant: 'zetasheet', id: 'TSK-050', title: 'Column fill', since: at, by: 'a' }]), milestones: { open: L([]), closed_count: 0 } },
    happened: { closed_milestones: L([]), done: L([{ tenant: 'zetadoc', id: 'TSK-010', title: 'Paste' }, { tenant: 'zetadoc', id: 'TSK-011', title: 'Undo' }]), accepted: L([]), sent_back: L([]), answers: L([]), decisions: L([]), new_items: L([]), counts: { done: 2 } },
    next: { ready: L([]), next_milestones: L([]) },
    metrics: null,
    summary: `Release ${n}: paste and undo are done.`,
  })

const word = (i: number) => ['alpha', 'bravo', 'charlie', 'delta', 'echo', 'foxtrot', 'golf', 'hotel'][i % 8]!
const many = (prefix: string, n: number, tenant = 'zetasheet', long = false): It[] =>
  Array.from({ length: n }, (_, i) => ({
    tenant: i % 2 === 0 ? tenant : 'zetadoc',
    id: `${prefix}-${String(i + 1).padStart(3, '0')}`,
    title: long && i % 7 === 0 ? `${word(i)} `.repeat(40).slice(0, 200) : `${word(i)} ${word(i + 3)} item ${i + 1}`,
    note: `note ${i + 1}`,
    since: `2026-10-22T09:${String(i % 60).padStart(2, '0')}:00`,
    by: 'session',
    done: i % 30,
    total: 30,
  }))

/** The stress draft: every list at its cap with `more`, 1000 milestones (200 shown), 100 blocked and 100 review. */
export const STRESS_DRAFT = JSON.stringify({
  version: 1,
  job: { ...job, title: 'Stress job with a long title that goes on and on for a while', description: 'x'.repeat(200) },
  release: { number: null, at: '2026-10-22T10:00:00', since: { number: 29, at: '2026-10-21T10:00:00' } },
  now: {
    counts: { needs: 120, review: 100, blocked: 100, doing: 80, ready: 400, done: 2000, total: 3000 },
    needs: L(many('NEED', 50, 'zetasheet', true), 70),
    review: L(many('REV', 50), 50),
    blocked: L(many('BLK', 50), 50),
    doing: L(many('DO', 50), 30),
    milestones: { open: L(many('WAV', 200), 800), closed_count: 0 },
  },
  happened: {
    closed_milestones: L(many('WAV', 100), 40),
    done: L(many('DONE', 100), 500),
    accepted: L(many('ACC', 100), 20),
    sent_back: L(many('BACK', 100), 5),
    answers: L(many('ANS', 100), 9),
    decisions: L(many('DEC', 100), 9),
    new_items: L(many('NEW', 100), 90),
    counts: { closed_milestones: 140, done: 600, accepted: 120, sent_back: 105, answers: 109, decisions: 109, new_items: 190 },
  },
  next: { ready: L(many('RDY', 10), 390), next_milestones: L(many('WAV', 5), 995) },
  metrics: {
    updated: '2026-10-22T09:00:00',
    sections: [0, 1, 2].map(s => ({
      title: `Section ${s}`,
      kind: 'split',
      legend: { good: 'pass', warn: 'partial', bad: 'fail', none: 'untested' },
      rows: Array.from({ length: 40 }, (_, i) => ({ name: `row ${s}-${i} with a long name`, good: i, warn: 3, bad: i % 5, none: 40 - i, delta: { good: i % 3 - 1 } })),
    })),
  },
  summary: '',
})

/** 30 releases (1..30) for navigation: number, date and a summary each; `latest` is 30. */
export const releases30 = (count = 30): Record<number, string> => {
  const out: Record<number, string> = {}
  for (let n = 1; n <= count; n++) {
    const day = String(((n - 1) % 28) + 1).padStart(2, '0')
    const month = n > 28 ? '11' : '10'
    out[n] = SMALL_RELEASE(n, `2026-${month}-${day}T10:00:00`)
  }
  return out
}

/** The slug of tenant i: t0000 ... (letters and digits mixed so the letter groups have something in each). */
export const tenantSlug = (i: number): string => `${'abcdefghijklmnopqrstuvwxyz0123456789'[i % 36]}${String(i).padStart(4, '0')}`

/**
 * A draft whose job has `n` tenants in the model's order (attention order): the first `needy` tenants have needs
 * (tenant i has 1 + i % 3), then a few with review. Items exist for those tenants; the totals are the sums.
 * Items carry tickets of every kind on some rows, and a tenant has a name.
 */
export const tenantsDraft = (n: number, needy = 3, jobId = 'zeta'): string => {
  const d = JSON.parse(SMALL_DRAFT)
  d.job.id = jobId
  const rows: unknown[] = []
  const needs: It[] = []
  const review: It[] = []
  let tn = 0
  let tr = 0
  let td = 0
  let tt = 0
  for (let i = 0; i < n; i++) {
    const nd = i < needy ? 1 + (i % 3) : 0
    const rv = i >= needy && i < needy + 4 ? 1 : 0
    const dn = i % 7
    const total = 10 + (i % 5)
    rows.push({ slug: tenantSlug(i), name: `Tenant ${i}`, counts: { needs: nd, review: rv, blocked: 0, doing: dn, ready: 1, done: i % total, total }, last_change: `2026-10-22T09:${String(i % 60).padStart(2, '0')}:00` })
    tn += nd
    tr += rv
    td += i % total
    tt += total
    for (let k = 0; k < nd; k++) needs.push({ tenant: tenantSlug(i), id: `Q-${i}-${k}`, title: `Question ${i}.${k}`, note: 'q', since: '2026-10-22T09:30:00', by: 'x' })
    if (rv > 0) review.push({ tenant: tenantSlug(i), id: `R-${i}`, title: `Review ${i}`, note: 'awaiting review', since: '2026-10-22T09:00:00', by: 'x' })
  }
  d.now.tenants = { items: rows.slice(0, 2000), more: Math.max(0, n - 2000) }
  d.now.needs = L(needs)
  d.now.review = L(review)
  d.now.blocked = L([])
  d.now.doing = L([])
  d.now.milestones = { open: L([]), closed_count: 0 }
  d.now.counts = { needs: tn, review: tr, blocked: 0, doing: 0, ready: n, done: td, total: tt }
  d.happened.done = L([])
  d.happened.closed_milestones = L([])
  d.happened.accepted = L([])
  d.happened.sent_back = L([])
  d.happened.answers = L([])
  d.happened.decisions = L([])
  d.happened.new_items = L([])
  d.happened.counts = {}
  d.next = { ready: L([]), next_milestones: L([]) }
  return JSON.stringify(d)
}

/** A registry of `n` jobs j000.., each with one tenant; `needs[i]` open needs for job i. */
export const manyJobs = (n: number) => ({
  jobs: Array.from({ length: n }, (_, i) => ({ id: `j${String(i).padStart(3, '0')}`, title: `Job number ${i}`, board: `~/apps/j${i}`, tenants: [`jt${i}`], news: '', summary: '', coordinator: 'c', desk: 'd', alerts: { sound: false, notify: false } })),
})

/** The draft of job i of manyJobs: `needs` open asks, `review` reviews, one tenant. */
export const jobDraft = (i: number, needs = 0, review = 0): string => {
  const d = JSON.parse(SMALL_DRAFT)
  d.job.id = `j${String(i).padStart(3, '0')}`
  d.job.title = `Job number ${i}`
  const tenant = `jt${i}`
  d.now.needs = L(Array.from({ length: needs }, (_, k) => ({ tenant, id: `N-${k}`, title: `ask ${k}`, note: 'q', since: '2026-10-22T09:30:00', by: 'x' })))
  d.now.review = L(Array.from({ length: review }, (_, k) => ({ tenant, id: `R-${k}`, title: `rev ${k}`, note: 'awaiting review', since: '2026-10-22T09:00:00', by: 'x' })))
  d.now.blocked = L([])
  d.now.doing = L([])
  d.now.milestones = { open: L([]), closed_count: 0 }
  d.now.counts = { needs, review, blocked: 0, doing: 0, ready: 0, done: i % 10, total: 10 }
  d.now.tenants = [{ slug: tenant, name: `Tenant of ${i}`, counts: { needs, review, blocked: 0, doing: 0, ready: 0, done: i % 10, total: 10 } }]
  d.happened.done = L([])
  d.happened.closed_milestones = L([])
  d.happened.accepted = L([])
  d.happened.sent_back = L([])
  d.happened.answers = L([])
  d.happened.decisions = L([])
  d.happened.new_items = L([])
  d.happened.counts = {}
  d.next = { ready: L([]), next_milestones: L([]) }
  return JSON.stringify(d)
}

/** A model exactly as `mw news --job demo --format json` printed it (built from the audit-fixes worktree): two tenants, one wave, a blocked, a doing, a ready and drafts. */
export const REAL_DRAFT = "{\"version\":1,\"job\":{\"id\":\"demo\",\"title\":\"demo\",\"description\":\"\",\"url\":\"\"},\"release\":{\"number\":null,\"at\":\"2026-10-22T14:35:20.161884Z\",\"since\":\"\",\"previous\":null,\"changes\":5},\"now\":{\"counts\":{\"draft\":2,\"needs\":0,\"review\":0,\"blocked\":1,\"doing\":1,\"ready\":1,\"done\":0,\"total\":5},\"tenants\":[{\"slug\":\"alpha\",\"name\":\"Alpha\",\"counts\":{\"draft\":1,\"needs\":0,\"review\":0,\"blocked\":1,\"doing\":1,\"ready\":1,\"done\":0,\"total\":4},\"last_change\":\"2026-10-22T14:35:20.161884Z\"},{\"slug\":\"beta\",\"name\":\"Beta\",\"counts\":{\"draft\":1,\"needs\":0,\"review\":0,\"blocked\":0,\"doing\":0,\"ready\":0,\"done\":0,\"total\":1},\"last_change\":\"2026-10-22T14:31:37.44372Z\"}],\"backlog\":{\"items\":[{\"tenant\":\"alpha\",\"id\":\"TSK-002\",\"title\":\"T-ready\",\"note\":\"\",\"since\":\"2026-10-22T14:35:20.0736Z\",\"by\":\"\",\"state\":\"ready\"},{\"tenant\":\"beta\",\"id\":\"TSK-001\",\"title\":\"Second thing\",\"note\":\"\",\"since\":\"2026-10-22T14:31:37.44372Z\",\"by\":\"\",\"state\":\"draft\"},{\"tenant\":\"alpha\",\"id\":\"TSK-004\",\"title\":\"T-two\",\"note\":\"\",\"since\":\"2026-10-22T14:35:14.206776Z\",\"by\":\"\",\"state\":\"draft\"}],\"count\":3},\"recent_done\":[],\"sprint\":{\"kind\":\"wave\",\"tenant\":\"alpha\",\"id\":\"WAV-001\",\"title\":\"W1 Sprint one\",\"done\":0,\"total\":3,\"items\":[{\"tenant\":\"alpha\",\"id\":\"TSK-003\",\"title\":\"T-doing\",\"note\":\"\",\"since\":\"2026-10-22T14:35:20.109812Z\",\"by\":\"\",\"state\":\"doing\"},{\"tenant\":\"alpha\",\"id\":\"TSK-001\",\"title\":\"First\",\"note\":\"\",\"since\":\"2026-10-22T14:35:20.161884Z\",\"by\":\"\",\"state\":\"blocked\"},{\"tenant\":\"alpha\",\"id\":\"TSK-002\",\"title\":\"T-ready\",\"note\":\"\",\"since\":\"2026-10-22T14:35:20.0736Z\",\"by\":\"\",\"state\":\"todo\"}],\"more\":0},\"needs\":[],\"review\":[],\"blocked\":[{\"tenant\":\"alpha\",\"id\":\"TSK-001\",\"title\":\"First\",\"note\":\"needs a decision\",\"since\":\"2026-10-22T14:35:20.161884Z\",\"by\":\"s2\"}],\"doing\":[{\"tenant\":\"alpha\",\"id\":\"TSK-003\",\"title\":\"T-doing\",\"note\":\"\",\"since\":\"2026-10-22T14:35:20.109812Z\",\"by\":\"s1\"}],\"milestones\":{\"open\":[{\"tenant\":\"alpha\",\"id\":\"WAV-001\",\"title\":\"W1 Sprint one\",\"done\":0,\"total\":3}],\"closed_count\":0},\"caps\":{\"backlog\":50,\"blocked\":50,\"doing\":50,\"milestones\":200,\"needs\":50,\"recent_done\":10,\"review\":50,\"sprint\":50,\"tenants\":2000},\"more\":{\"backlog\":0,\"blocked\":0,\"doing\":0,\"milestones\":0,\"needs\":0,\"review\":0,\"tenants\":0}},\"happened\":{\"closed_milestones\":[],\"done\":[],\"accepted\":[],\"sent_back\":[],\"answers\":[],\"decisions\":[],\"new_items\":[{\"tenant\":\"alpha\",\"id\":\"TSK-004\",\"title\":\"T-two\",\"note\":\"\",\"since\":\"2026-10-22T14:35:14.206776Z\",\"by\":\"\"},{\"tenant\":\"alpha\",\"id\":\"TSK-003\",\"title\":\"T-doing\",\"note\":\"\",\"since\":\"2026-10-22T14:35:14.188857Z\",\"by\":\"\"},{\"tenant\":\"alpha\",\"id\":\"TSK-002\",\"title\":\"T-ready\",\"note\":\"\",\"since\":\"2026-10-22T14:35:14.165133Z\",\"by\":\"\"},{\"tenant\":\"beta\",\"id\":\"TSK-001\",\"title\":\"Second thing\",\"note\":\"\",\"since\":\"2026-10-22T14:31:37.44372Z\",\"by\":\"\"},{\"tenant\":\"alpha\",\"id\":\"TSK-001\",\"title\":\"First\",\"note\":\"\",\"since\":\"2026-10-22T14:31:37.295589Z\",\"by\":\"\"}],\"moved_to_review\":[],\"moved_to_doing\":[{\"tenant\":\"alpha\",\"id\":\"TSK-001\",\"title\":\"First\",\"note\":\"\",\"since\":\"2026-10-22T14:35:20.144395Z\",\"by\":\"s2\"},{\"tenant\":\"alpha\",\"id\":\"TSK-003\",\"title\":\"T-doing\",\"note\":\"\",\"since\":\"2026-10-22T14:35:20.109812Z\",\"by\":\"s1\"}],\"counts\":{\"accepted\":0,\"answers\":0,\"closed_milestones\":0,\"decisions\":0,\"done\":0,\"moved_to_doing\":2,\"moved_to_review\":0,\"new_items\":5,\"sent_back\":0},\"caps\":{\"accepted\":100,\"answers\":100,\"closed_milestones\":100,\"decisions\":100,\"done\":100,\"moved_to_doing\":100,\"moved_to_review\":100,\"new_items\":100,\"sent_back\":100},\"more\":{\"accepted\":0,\"answers\":0,\"closed_milestones\":0,\"decisions\":0,\"done\":0,\"moved_to_doing\":0,\"moved_to_review\":0,\"new_items\":0,\"sent_back\":0}},\"next\":{\"ready\":[{\"tenant\":\"alpha\",\"id\":\"TSK-002\",\"title\":\"T-ready\",\"note\":\"\",\"since\":\"2026-10-22T14:35:20.0736Z\",\"by\":\"\"}],\"next_milestones\":[{\"tenant\":\"alpha\",\"id\":\"WAV-001\",\"title\":\"W1 Sprint one\",\"done\":0,\"total\":3}],\"caps\":{\"next_milestones\":5,\"ready\":10},\"more\":{\"next_milestones\":0,\"ready\":0}},\"metrics\":null,\"summary\":\"\"}"
