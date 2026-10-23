// TSK-071: evidence as expandable thumbnails in the detail panel. Pure helpers; the reads (ls, sips) run in register.tsx.

export const IMG_EXT = /\.(png|jpe?g|webp|gif)$/i
export const THUMB_MAX = 8
/** Pixels on the long edge of a thumbnail and of the expanded picture. */
export const THUMB_PX = 200
export const BIG_PX = 720
/** An Svg above 131072 characters is refused by the host; the picture must leave room for the markup around it. */
export const IMG_SVG_CAP = 120_000

export const isImagePath = (p: string): boolean => IMG_EXT.test(p.trim())

/** Whether an evidence entry could be a path (not a note such as "PLAN.md 8558"): absolute, ~, a Windows drive, or ending in an image extension. */
export const looksLikePath = (e: string): boolean => /^(~|\/|[A-Za-z]:[\\/])/.test(e.trim()) || isImagePath(e)

const join = (dir: string, name: string) => `${dir.replace(/[\\/]+$/, '')}/${name}`

/** The image files a `ls -1p -- path` output names: the path itself when it is an image file (ls echoes it), else the image files of the folder (sub-folders skipped), sorted, at most `max`. */
export const imagesFromLs = (path: string, out: string, max = THUMB_MAX): string[] => {
  const lines = out.split(/\r?\n/).map(l => l.trim()).filter(Boolean)
  if (lines.length === 1 && lines[0] === path) return isImagePath(path) ? [path] : []
  return lines.filter(n => !n.endsWith('/') && isImagePath(n)).sort((a, b) => a.localeCompare(b)).slice(0, max).map(n => join(path, n))
}

/** The paths to look at for an item: every path-like evidence entry and the item's evidence folder (when it holds files), without repeats. */
export const evidenceRoots = (evidence: string[], evidenceDir: string, evidenceFiles: number): string[] => {
  const out: string[] = []
  for (const e of evidence) if (looksLikePath(e) && !out.includes(e.trim())) out.push(e.trim())
  if (evidenceDir !== '' && evidenceFiles > 0 && !out.includes(evidenceDir)) out.push(evidenceDir)
  return out
}

/** The entries that are not shown as thumbnails stay text: everything that is not a path to an image or to a folder whose images were found. */
export const textEvidence = (evidence: string[], images: Record<string, string[]>): string[] => evidence.filter(e => !(looksLikePath(e) && (images[e.trim()]?.length ?? 0) > 0))

/** A thumbnail's cache key: path plus modified time, so an edited file is read again. */
export const thumbKey = (path: string, mtime: string | number, px: number) => `${path}|${mtime}|${px}`

export type ImgDecision = 'svg' | 'link'
/** Whether a base64 picture goes into an Svg or falls back to a link: it must fit under the Svg cap with its markup. */
export const imgDecision = (b64: string | undefined): ImgDecision => (b64 !== undefined && b64.length > 0 && b64.length <= IMG_SVG_CAP ? 'svg' : 'link')

/** One click: the clicked picture opens, the open one closes; one is open at a time. */
export const toggleOpen = (open: string, path: string): string => (open === path ? '' : path)

/** The Svg of one picture inside a w x h box (meet: the picture keeps its shape). */
export const imageSvg = (b64: string, w: number, h: number): { source: string; width: number; height: number } => ({
  source: `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}"><image href="data:image/jpeg;base64,${b64}" x="0" y="0" width="${w}" height="${h}" preserveAspectRatio="xMidYMid meet"/></svg>`,
  width: w,
  height: h,
})

/** argv that prints a picture as one line of base64 JPEG with the long edge at `px` (macOS sips; nothing else runs). The path is an argument, never part of the script. */
export const thumbArgv = (path: string, px: number): string[] => [
  '/bin/sh',
  '-c',
  't="/tmp/mwj-thumb-$$.jpg"; /usr/bin/sips -s format jpeg -s formatOptions 60 -Z "$2" "$1" --out "$t" >/dev/null 2>&1 && /usr/bin/base64 -i "$t" | /usr/bin/tr -d "\\n"; rc=$?; rm -f "$t"; exit $rc',
  'sh',
  path,
  String(px),
]

/** The viewer for an expanded picture that is too big for an Svg: the OS default app (plain `open`, not the text editor). */
export const imageOpenerArgv = (os: 'windows' | 'mac' | 'linux', path: string): string[] =>
  os === 'windows' ? ['cmd', '/c', 'start', '', path] : os === 'linux' ? ['xdg-open', path] : ['open', path]
