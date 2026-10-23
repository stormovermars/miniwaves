/** A tenant of a job as the model's now.tenants lists them (attention order): counts like the job's own. */
export type TenantBrief = { slug: string; name: string; draft: number; needs: number; review: number; blocked: number; doing: number; ready: number; done: number; total: number; last?: number }
export type MItem = { tenant: string; id: string; title: string; note: string; since: string; by: string; done: number; total: number; ticket: string; ticket_url: string; milestone: string; milestone_title: string; state: string; needs: boolean }
/* MItem.ticket holds the model's ticket_label (what the tracker calls it: MOB-142, #123, AB#123, sc-123); ticket_url is the link mw built, '' without one. */
export type MList = { items: MItem[]; more: number }
/** The open wave as mw's model gives it (now.sprint): its members with states todo, doing, review, blocked, done. */
export type MSprint = { kind: string; tenant: string; id: string; title: string; done: number; total: number; items: MItem[]; more: number }
export type MDelta = Record<string, Record<string, number>>
export type Model = {
  version: 1
  job: { id: string; title: string; description: string; url: string }
  /** since.number is the release compared against (`previous`), since.at the previous release's time; changes is mw's own count of events in the window. */
  release: { number: number | null; at: string; since: { number: number | null; at: string }; changes?: number }
  now: {
    counts: Record<string, number>
    needs: MList
    review: MList
    blocked: MList
    doing: MList
    milestones: MList
    /** Drafts plus ready items, in queue order, with mw's count (now.backlog). */
    backlog: MList & { count: number }
    /** The most recent done items, each with its milestone (now.recent_done). */
    recentDone: MList
    sprint: MSprint | null
    /** False when the model has no now.backlog (an older mw): Agile and Kanban say so and dim. */
    backlogKnown?: boolean
    closed: number
    /** Per-tenant counts in the model's order (absent from an older mw). */
    tenants: TenantBrief[]
    /** Tenant rows the model left out (its cap), so the job has tenants.length + tenantsMore. */
    tenantsMore: number
  }
  happened: {
    closed_milestones: MList
    done: MList
    accepted: MList
    sent_back: MList
    answers: MList
    decisions: MList
    new_items: MList
    moved_to_review: MList
    moved_to_doing: MList
    counts: Record<string, number>
  }
  next: { ready: MList; next_milestones: MList }
  metrics: MwMetrics | { error: string } | null
  /** Set on a model filtered to one tenant: its slug (the rows then drop the tenant prefix). */
  only?: string
  /** Row deltas from the model, by `section title/row name`: { field: change }. */
  deltas: MDelta
  summary: string
}

export type MwJobsAlerts = { sound?: boolean; notify?: boolean; push?: boolean; email?: boolean }
export type MwJobsJob = {
  id: string
  title: string
  board: string
  tenants: string[]
  news: string
  /** Path of the job's metrics.json, relative to the board (the registry's own key; `mw jobs show` may carry it too). */
  metrics?: string
  /** One line about the job; the fallback when `mw jobs show ID -j` has no description. */
  description?: string
  coordinator: string
  desk: string
  alerts: MwJobsAlerts
  /** Set when the job came from `mw boards` discovery, not the registry: 'job' (an mw job of that board) or 'tenant' (a board with no jobs, one row per tenant). */
  scanned?: 'job' | 'tenant'
  /** The board's name, for a job found by scan. */
  boardName?: string
  /** A discovered job whose board this session used. */
  thisSession?: boolean
}
/** Fanfare state for one board: on or off, the set in use ('all' or a set name), the set names in `mw fanfare -j` order. */
export type MwJobsFanfare = { enabled: boolean; use: string; sets: string[]; playingFinale?: boolean }
/** One detail panel: an item or a milestone (wave) of a job's tenant. */
export type MwJobsDetailRef = { kind: 'item' | 'wave'; jobId: string; tenant: string; id: string }
export type MwJobsDetail = { cur: MwJobsDetailRef | null; trail: MwJobsDetailRef[]; from: 'jobs' | 'news' }
/** A board the session used, as `mw boards DIR --up --depth 0 -j` resolved it. */
export type MwJobsUsedBoard = { dir: string; name: string; tenants: { slug: string; name: string; archived: boolean }[] }
/** The jobs the panes show: the registry file, or (when it is empty) the boards `mw boards` found; `hint` is one line about how they were found. */
export type MwJobsRegistry = { jobs: MwJobsJob[]; error: string | null; loaded?: boolean; source?: 'registry' | 'scan'; hint?: string | null }
export type MwJobsWave = {
  id: string
  title: string
  state: string
  members: string
  percent: number
  blocked: number
}
export type MwJobsItem = {
  tenant: string
  id: string
  type: string
  state: string
  title: string
  note: string
  /** ISO time of the item's last update ('' when unknown). */
  updated: string
  /** The item's Jira key, and the link mw built for it (never built here). */
  ticket?: string
  ticketUrl?: string
}
export type MwJobsEvent = { ts: string; tenant: string; item: string; action: string; note: string; by?: string }
export type MwMetricRow = { name: string; good?: number; warn?: number; bad?: number; none?: number; done?: number; total?: number; value?: number; unit?: string; target?: number; better?: 'higher' | 'lower' }
export type MwMetricSection = { title: string; kind: 'split' | 'progress' | 'value'; legend: { good: string; warn: string; bad: string; none: string }; note?: string; rows: MwMetricRow[] }
export type MwMetrics = { updated: string; sections: MwMetricSection[] }
/** What the person has looked at: the cursor in epoch ms and the harness numbers then. */
/** The seen cursor: ms, and per `section/row` the measures then ([primary, bad]). */
export type MwJobsSeen = { ts: number; metrics: Record<string, number[]> }
/** An open ask from `mw review -j` needs: closed by `mw answer`. */
export type MwJobsAsk = { tenant: string; item: string; question: string; by: string; at: string }
/** What the jobs pane keeps from a news model beyond the items: headline counts, hidden rows and the milestone tail. */
export type ModelBrief = {
  counts: { needs: number; review: number; blocked: number; doing: number }
  more: { needs: number; review: number; blocked: number; doing: number }
  openMilestones: number
  closedMilestones: number
  lastChange?: number
  /** done / total items of the job, and its tenants (the model's order). */
  done: number
  total: number
  tenants: TenantBrief[]
  tenantsMore: number
  /** Ready items (the model's count, else the listed ones). */
  ready?: number
}
export type MwJobsCfgKey = { key: string; value: string; default: string; source: string; group: string; choices: string[] }
export type MwJobsSettings = { ok: boolean | null; keys: MwJobsCfgKey[]; hint: string; err: Record<string, string> }
export type MwJobsAlertRow = { id: string; at: number; job: string; type: string; text: string; ref?: string; link?: { kind: 'item' | 'wave'; tenant: string; id: string } }
export type MwJobsSnapshot = {
  waves: MwJobsWave[]
  items: MwJobsItem[]
  events: MwJobsEvent[]
  /** The job's metrics.json, parsed and limit-checked; absent with no file or an invalid one. */
  metrics?: MwMetrics
  /** One dim line when the metrics file is invalid or newer than this reader. */
  metricsNotice?: string
  updatedAt: number
  /** Open asks (mw review -j needs); needs you means these plus blocked items. */
  asks?: MwJobsAsk[]
  /** Items being answered in chat now (`tenant:id` → ms): out of the needs/review counts for up to 30 minutes. */
  inChat?: Record<string, number>
  /** Set when the snapshot was built from a news model: needs are open asks only, blocked is its own group, no events. */
  model?: ModelBrief
}

declare module 'claude-code' {
  interface PluginState {
    'mw-jobs': {
      /** The registry as last read. */
      registry: MwJobsRegistry
      /** Last good snapshot per job id. */
      snapshots: Record<string, MwJobsSnapshot>
      /** The snapshot before it, per job id, for change detection. */
      previous: Record<string, MwJobsSnapshot>
      /** Epoch ms of the first failed refresh since the last good one, per job id. */
      staleSince: Record<string, number>
      /** The job shown in the pane (also kept in $.store). */
      selected: string
      /** Sound and OS notifications off (toasts stay). */
      muted: boolean
      popup: boolean
      /** Auto-refresh period: off, 30s, 1m or 5m (also kept in $.store). */
      refreshEvery: string
      /** The seen cursor per job id (also kept in $.store): rows changed after it are marked new. */
      seen: Record<string, MwJobsSeen>
      /** Pane density step, -2..3, 0 default (also kept in $.store). Text has no font size, so below 0 is compact and above 0 roomy. */
      fontStep: number
      /** Pane style: normal (one SVG, desktop/vscode/mobile only) or blocks (█░ bars; a monospace SVG off the terminal); also kept in $.store, a stored thin means blocks. Default normal; the terminal shows blocks for normal. */
      barStyle: string
      /** The news pane's own style (normal, blocks or gopher), also kept in $.store. */
      newsStyle: string
      /** The job whose news pane (docs/news/status.md) is shown. */
      newsJob: string
      /** docs/news/status.md per job id: the text (null when the file is missing), cut at 64 KB. */
      news: Record<string, { text: string | null; truncated: boolean; hash: string }>
      /** The news content hash last seen per job id (the Updated line ignored); also kept in $.store. A different hash marks the news tab with a dot. */
      newsSeen: Record<string, string>
      /** mw's own news render per job + kind + columns (`id|text|80`, `id|md`): ok and its stdout; a failed run is cached as ok false. Not persisted. */
      newsOut: Record<string, { ok: boolean; text: string }>
      /** The news model per job id: the draft (mw news --job ID --format json), the releases read so far, and the archive's numbers. Not persisted. */
      newsModel: Record<string, { draft?: Model; draftError?: string; stale?: boolean; tdraft?: { slug: string; model: Model }; releases: Record<number, Model>; bad: Record<number, string>; nums: number[]; latest: number; dir: string }>
      /** Where the reader is in each job's news: page 0 is the draft, N a release; undefined means the newest page. seen = the newest release the reader has seen (a higher latest puts a dot on the list and on latest). */
      newsView: Record<string, { page?: number; list?: boolean; listPage?: number; seen?: number; confirm?: boolean }>
      /** The newest release number already alerted per job id (also kept in $.store). */
      relKnown: Record<string, number>
      /** Jobs pane groups shown in full instead of the first 10, per job id. */
      expanded: Record<string, Record<string, number>>
      /** The one-line description per job id, from `mw jobs show ID -j` (cached; refresh and timer only). Not persisted. */
      jobDesc: Record<string, string>
      /** Alert lines already raised per job id (also kept in $.store): a change alerts once, whichever pane, timer or reload sees it. */
      alerted: Record<string, string[]>
      /** Per job: items whose question was sent to the chat, with the send time (also kept in $.store). */
      inChat: Record<string, Record<string, number>>
      /** A doing item with no activity for this many minutes counts as stalled (default 45). */
      stalledMin: number
      /** Ids of the jobs being refreshed now. */
      busy: string[]
      /** Blocks fill option (↔) per pane: on = the grid takes the pane width up to 160 columns, off = 80 (also kept in $.store as blocksFill). */
      blocksFill: { jobs: boolean; news: boolean }
      /** Ids of the jobs whose release is being cut now (the Release button is off meanwhile). */
      releasing: string[]
      /** The summary pages and pickers: per `pane|jobs` or `pane|tenants|jobId`. Not persisted. */
      sw: Record<string, { view?: 'pick' | 'all'; page: number; range?: string; fav?: boolean }>
      /** Favorites, in the order they were starred (also kept in $.store as `favorites`). */
      favorites: { jobs: string[]; tenants: Record<string, string[]> }
      /** The tenant chosen per job id ('' or absent = all); also kept in $.store. */
      tenantSel: Record<string, string>
      /** True once a job was chosen (or stored): until then more than one job opens on the summary page. */
      chosen: boolean
      /** The fanfare as mw reads it per job id (per-user value over the board's); null when this mw has no fanfare keys. */
      fanfare: Record<string, MwJobsFanfare | null>
      /** Boards this session used (an mw command run in them, a .miniwaves file read or written), resolved to their roots, oldest first. */
      usedBoards: MwJobsUsedBoard[]
      /** The detail pane: what it shows, the panels to go back to, and the pane it was opened from. */
      detail: MwJobsDetail
      /** What `mw show` / `mw wave show` returned per panel key (job|kind|tenant|id): the raw JSON text, or why it failed. */
      detailData: Record<string, { text?: string; why?: string }>
      /** Alerts shown as rows at the top of the jobs and news panes until dismissed, newest first. */
      alertRows: MwJobsAlertRow[]
      /** The per-user mw keys (`mw user get -j`); ok false with an older mw. */
      settings: MwJobsSettings
    }
  }
}
