import type { DiffFile } from '../types'
import { marks, matches } from '../lib/filter'

// The file tree in the diff sidebar's left column. The flat file list the
// server sends is turned into one tree per sub-repo; a chain of directories
// with a single child collapses into one row ("internal/server/api"), which is
// what keeps a 77-file diff readable.

export type Node = {
  key: string // repo + ':' + path, unique across repos
  label: string
  repo: string
  file?: DiffFile // set on leaves
  children: Node[]
  added: number
  deleted: number
}

export function buildTree(files: DiffFile[]): Node[] {
  const roots: Node[] = []
  const byRepo = new Map<string, Node>()

  for (const f of files) {
    let repoNode = byRepo.get(f.repo)
    if (!repoNode) {
      repoNode = { key: `${f.repo}:`, label: f.repo, repo: f.repo, children: [], added: 0, deleted: 0 }
      byRepo.set(f.repo, repoNode)
      roots.push(repoNode)
    }
    const segs = f.path.split('/')
    let cur = repoNode
    let acc = ''
    segs.forEach((seg, i) => {
      acc = acc ? `${acc}/${seg}` : seg
      const leaf = i === segs.length - 1
      let next = cur.children.find((c) => c.label === seg && !c.file === !leaf)
      if (!next) {
        next = {
          key: `${f.repo}:${acc}`,
          label: seg,
          repo: f.repo,
          children: [],
          added: 0,
          deleted: 0,
          file: leaf ? f : undefined,
        }
        cur.children.push(next)
      }
      cur = next
    })
  }
  roots.forEach(finish)
  return roots
}

// finish collapses single-child directory chains and sums the stats upwards,
// so a directory row carries the totals of everything under it.
function finish(n: Node) {
  if (n.file) {
    n.added = n.file.added
    n.deleted = n.file.deleted
    return
  }
  // "a" → "a/b" → "a/b/c" as long as the only child is another directory
  while (n.children.length === 1 && !n.children[0].file) {
    const only = n.children[0]
    n.label = `${n.label}/${only.label}`
    n.key = only.key
    n.children = only.children
  }
  n.children.forEach(finish)
  n.children.sort((a, b) => {
    if (!a.file !== !b.file) return a.file ? 1 : -1 // directories first
    return a.label.localeCompare(b.label)
  })
  n.added = n.children.reduce((s, c) => s + c.added, 0)
  n.deleted = n.children.reduce((s, c) => s + c.deleted, 0)
}

// firstLeaf is the file a freshly opened panel selects: the first one in TREE
// order, so the highlighted row sits at the top of the tree instead of
// somewhere the user has to scroll to find.
export function firstLeaf(nodes: Node[]): DiffFile | undefined {
  for (const n of nodes) {
    if (n.file) return n.file
    const deeper = firstLeaf(n.children)
    if (deeper) return deeper
  }
  return undefined
}

// filterTree keeps the leaves whose path matches every term and the
// directories on the way to them, dropping the rest. The stats are re-summed
// from what survived: a directory row still claiming +4307 while one file of
// it is on screen would be reading the wrong number off the right row.
export function filterTree(nodes: Node[], terms: string[]): Node[] {
  if (!terms.length) return nodes
  const keep = (n: Node): Node | null => {
    if (n.file) return matches(n.file.path, terms) ? n : null
    const children = n.children.map(keep).filter((c): c is Node => c !== null)
    if (!children.length) return null
    return {
      ...n,
      children,
      added: children.reduce((sum, c) => sum + c.added, 0),
      deleted: children.reduce((sum, c) => sum + c.deleted, 0),
    }
  }
  return nodes.map(keep).filter((n): n is Node => n !== null)
}

// Label marks the part of the row the filter matched. A collapsed directory
// chain is one label ("internal/server/api"), so the highlight has to work on
// arbitrary text rather than on a single path segment.
function Label({ text, terms }: { text: string; terms: string[] }) {
  const hits = terms.length ? marks(text, terms) : []
  if (!hits.length) return <span className="tw-label">{text}</span>
  const out: React.ReactNode[] = []
  let at = 0
  hits.forEach(([start, end], i) => {
    if (start > at) out.push(text.slice(at, start))
    out.push(
      <mark key={i} className="tw-hit">
        {text.slice(start, end)}
      </mark>,
    )
    at = end
  })
  if (at < text.length) out.push(text.slice(at))
  return <span className="tw-label">{out}</span>
}

// Tree icons. Inline SVG on currentColor: crisp at any zoom, themed by the
// row's colour, and no font or asset to load. 16px viewBox, drawn on the
// half-pixel so the strokes land sharp at 15px.
function Chevron({ open }: { open: boolean }) {
  return (
    <svg className={open ? 'tw-chev tw-chev-open' : 'tw-chev'} viewBox="0 0 16 16" aria-hidden="true">
      <path d="M6 3.5L10.5 8L6 12.5" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

// A repo root is not a directory — it is another checkout — so it gets the
// branch glyph rather than a folder.
function RepoIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <g fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
        <path d="M4.5 3.5v9" />
        <path d="M4.5 6.5h4a2 2 0 0 1 2 2v1" />
      </g>
      <circle cx="4.5" cy="2.75" r="1.5" fill="currentColor" />
      <circle cx="10.5" cy="10.75" r="1.5" fill="currentColor" />
    </svg>
  )
}

function FolderIcon({ open }: { open: boolean }) {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      {open ? (
        <g fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round">
          <path d="M2 12.5V4a.5.5 0 0 1 .5-.5h3.4l1.4 1.6h4.8a.5.5 0 0 1 .5.5v1.4" />
          <path d="M2 12.5l1.8-4.6a.5.5 0 0 1 .47-.32h10a.35.35 0 0 1 .33.47L13 12.5z" />
        </g>
      ) : (
        <path
          d="M2 12.4V3.9a.5.5 0 0 1 .5-.5h3.3L7.2 5h6a.5.5 0 0 1 .5.5v6.9a.5.5 0 0 1-.5.5H2.5a.5.5 0 0 1-.5-.5z"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.4"
          strokeLinejoin="round"
        />
      )}
    </svg>
  )
}

function FileIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <g fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round">
        <path d="M4 2.4h5L12.4 6v7.6a.4.4 0 0 1-.4.4H4a.4.4 0 0 1-.4-.4V2.8a.4.4 0 0 1 .4-.4z" />
        <path d="M8.9 2.5v3.2h3.3" />
      </g>
    </svg>
  )
}

function Stat({ n }: { n: Node }) {
  if (n.file?.untracked && !n.added && !n.deleted) return <span className="tw-stat dim">new</span>
  if (!n.added && !n.deleted) return null
  return (
    <span className="tw-stat">
      {!!n.added && <span className="plus">+{n.added}</span>}
      {!!n.deleted && <span className="minus">−{n.deleted}</span>}
    </span>
  )
}

// Where the file's change lives, as a dot per source: committed on the branch,
// uncommitted, or both. This is the only place the distinction is drawn — the
// tree is one list of everything the checkout holds.
function Origin({ origin, merged }: { origin: string; merged?: boolean }) {
  // a committed change the base branch already holds is history, like a
  // landed commit, and takes that dot's grey
  if (merged) return <span className="tw-origin" title={originTitle.merged}><i className="od od-merged" /></span>
  return (
    <span className="tw-origin" title={originTitle[origin] ?? origin}>
      {(origin === 'branch' || origin === 'both') && <i className="od od-branch" />}
      {(origin === 'working' || origin === 'both') && <i className="od od-working" />}
    </span>
  )
}

const originTitle: Record<string, string> = {
  merged: 'committed · the base branch already has this change',
  branch: 'committed on this branch',
  working: 'uncommitted',
  both: 'committed on this branch, and modified since',
}

// leavesOf is every file under a node — a directory click selects all of them.
export function leavesOf(n: Node): DiffFile[] {
  if (n.file) return [n.file]
  return n.children.flatMap(leavesOf)
}

// visibleLeaves is the files in RENDER order, skipping collapsed directories:
// what a shift-click range has to walk.
export function visibleLeaves(nodes: Node[], collapsed: Set<string>): Node[] {
  const out: Node[] = []
  const walk = (list: Node[]) => {
    for (const n of list) {
      if (n.file) {
        out.push(n)
        continue
      }
      if (!collapsed.has(n.key)) walk(n.children)
    }
  }
  walk(nodes)
  return out
}

export function TreeRows({
  nodes,
  depth,
  sel,
  collapsed,
  terms,
  onPick,
  onToggle,
  onContext,
}: {
  nodes: Node[]
  depth: number
  sel: Set<string>
  collapsed: Set<string>
  // the filter's terms, for highlighting; the pruning already happened
  terms: string[]
  onPick: (n: Node, e: React.MouseEvent) => void
  onToggle: (key: string) => void
  onContext: (n: Node, e: React.MouseEvent) => void
}) {
  return (
    <>
      {nodes.map((n) => {
        const dir = !n.file
        const open = dir && !collapsed.has(n.key)
        // a directory counts as selected when everything under it is
        const leaves = dir ? leavesOf(n) : []
        const active = dir
          ? leaves.length > 0 && leaves.every((f) => sel.has(`${f.repo}:${f.path}`))
          : sel.has(n.key)
        return (
          <div key={n.key}>
            <div
              className={`tw-row${dir ? ' tw-dir' : ''}${active ? ' tw-active' : ''}${depth === 0 ? ' tw-repo' : ''}`}
              style={{ paddingLeft: 5 + depth * 11 }}
              title={n.file ? n.file.path : n.label}
              // clicking a directory SELECTS everything under it — the diffs of
              // a whole subtree are the point. Folding is the chevron's job.
              onClick={(e) => onPick(n, e)}
              onContextMenu={(e) => onContext(n, e)}
            >
              <span
                className={dir ? 'tw-caret tw-caret-hit' : 'tw-caret'}
                // the row selects everything under it, so the chevron has to
                // keep its click to itself: folding a directory must not also
                // load the diffs of every file in it
                onClick={
                  dir
                    ? (e) => {
                        e.stopPropagation()
                        onToggle(n.key)
                      }
                    : undefined
                }
                title={dir ? (open ? 'fold' : 'unfold') : undefined}
              >
                {dir && <Chevron open={open} />}
              </span>
              <span className={`tw-icon${depth === 0 ? ' tw-icon-repo' : ''}`}>
                {depth === 0 ? <RepoIcon /> : dir ? <FolderIcon open={open} /> : <FileIcon />}
              </span>
              <Label text={n.label} terms={terms} />
              {n.file && <Origin origin={n.file.origin} merged={n.file.merged} />}
              <Stat n={n} />
            </div>
            {open && (
              <TreeRows
                nodes={n.children}
                depth={depth + 1}
                sel={sel}
                collapsed={collapsed}
                terms={terms}
                onPick={onPick}
                onToggle={onToggle}
                onContext={onContext}
              />
            )}
          </div>
        )
      })}
    </>
  )
}
