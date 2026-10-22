export type TidewatchJob = {
  id: string
  title: string
  /** Absolute path of the folder holding the mw board. */
  board: string
  /** Tenants of the board that belong to the job (empty: all of them). */
  tenants: string[]
  /** Optional news page URL. */
  news: string
  /** News Markdown file read by the news pane (relative to the board; empty: docs/news/status.md). */
  newsFile: string
  /** Optional Markdown file (relative to the board) with harness lines. */
  summary: string
  /** Short job id on its board ('' for a board shown as one job); used in pane titles. */
  name?: string
  /** Id of the registered board this job came from (the job's own id when not split). */
  boardId?: string
}
export type TidewatchRegistry = { jobs: TidewatchJob[]; error: string | null }
export type TidewatchWave = {
  id: string
  title: string
  state: string
  members: string
  percent: number
  blocked: number
}
export type TidewatchItem = {
  tenant: string
  id: string
  type: string
  state: string
  title: string
  note: string
  /** ISO time of the item's last update ('' when unknown). */
  updated: string
}
/** An open ask from `mw review -j` (`needs`). */
export type TidewatchAsk = { tenant: string; item: string; question: string; at: string }
export type TidewatchEvent = { ts: string; tenant: string; item: string; action: string; note: string }
export type TidewatchHarness = { name: string; line: string; pass: number; fail: number; skip: number }
/** What the person has looked at: the cursor in epoch ms and the harness numbers then. */
export type TidewatchSeen = { ts: number; harness: Record<string, { pass: number; fail: number }> }
export type TidewatchSnapshot = {
  waves: TidewatchWave[]
  items: TidewatchItem[]
  events: TidewatchEvent[]
  harness: TidewatchHarness[]
  /** Open asks from `review -j` `needs`; undefined when that mw has no such array (blocked-note heuristic then). */
  asks?: TidewatchAsk[]
  updatedAt: number
}
/** The fanfare footer's state: the per-user value wins over the board's. */
export type TidewatchFanfare = { enabled: boolean; use: string; sets: string[] }

/** A colour for light and dark, each a #rgb, #rrggbb or #rrggbbaa string. */
export type TidewatchColor = { light: string; dark: string }
export type TidewatchColorName = 'fg' | 'dim' | 'track' | 'needs' | 'waiting' | 'progress' | 'new' | 'pass' | 'fail'
export type TidewatchSectionId = 'needs' | 'review' | 'progress' | 'milestones' | 'harness' | 'events'
/** A style: tokens only, never code. */
export type TidewatchStyle = {
  font: 'sans' | 'mono'
  sizes: { headline: number; body: number; note: number; label: number }
  rowHeight: number
  columns: { name: number; count: number }
  bar:
    | { kind: 'rect'; height: number; radius: number }
    | { kind: 'glyphs'; fill: string; rest: string }
  colors: Record<TidewatchColorName, TidewatchColor>
  sections: { order: TidewatchSectionId[]; show: Record<TidewatchSectionId, boolean> }
  scale: [number, number, number]
}

declare module 'claude-code' {
  interface PluginState {
    'mw-watch': {
      /** The registry as last read. */
      registry: TidewatchRegistry
      /** Last good snapshot per job id. */
      snapshots: Record<string, TidewatchSnapshot>
      /** The snapshot before it, per job id, for change detection. */
      previous: Record<string, TidewatchSnapshot>
      /** Epoch ms of the first failed refresh since the last good one, per job id. */
      staleSince: Record<string, number>
      /** The job shown in the pane (also kept in $.store). */
      selected: string
      /** Sound and notifications off (toasts stay). */
      muted: boolean
      /** Auto-refresh period: off, 30s, 1m or 5m (also kept in $.store). */
      refreshEvery: string
      /** The seen cursor per job id (also kept in $.store): rows changed after it are marked new. */
      seen: Record<string, TidewatchSeen>
      /** Pane size step, -1..1, 0 default; picks the style's scale factor (also kept in $.store). */
      fontStep: number
      /** Name of the chosen style: a built-in (normal, blocks) or a user file; also kept in $.store. */
      style: string
      /** Resolved styles by name (built-ins and user styles loaded so far). */
      styles: Record<string, TidewatchStyle>
      /** The last problem text reported as a toast per style name, so a bad token is reported once. */
      styleIssues: Record<string, string>
      /** Names of the styles on offer: built-ins first, then the user's. */
      styleNames: string[]
      /** The news file as last read per job id: its path, text (cut to 9,500 characters) and whether it was missing. */
      news: Record<string, { path: string; text: string; isCut: boolean; isMissing: boolean }>
      /** A doing item with no activity for this many minutes counts as stalled (default 45). */
      stalledMin: number
      /** Ids of the jobs being refreshed now. */
      busy: string[]
      /** The fanfare footer state per board folder; absent when that mw lacks fanfare sets or user show. */
      fanfare: Record<string, TidewatchFanfare>
    }
  }
}
