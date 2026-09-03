import type { Checkout, Checks } from '../types'
import { useScrollToActive } from '../lib/scroll'
import { Hits } from './Hits'

// Pane two: the worktrees of the selected repo. Most repos have exactly one
// checkout and this is a single row; a repo with two dozen is the case worth
// designing for — name, branch, and how far each is from the base.
export function WorktreeList({
  checkouts,
  base,
  sel,
  terms,
  checks,
  onPick,
}: {
  checkouts: Checkout[]
  base: string
  sel: string
  terms: string[]
  checks?: Record<string, Checks>
  onPick: (name: string) => void
}) {
  const box = useScrollToActive('.wl-active', sel)
  return (
    <div className="wl-list" ref={box}>
      {checkouts.map((c) => (
        <div
          key={c.name}
          className={`wl-row${c.name === sel ? ' wl-active' : ''}`}
          onClick={() => onPick(c.name)}
          title={c.path}
        >
          <div className="wl-line">
            <span className="wl-name mono">
              <Hits text={c.name} terms={terms} />
            </span>
            <CheckDot checks={checks?.[c.name]} />
            <span className="gitstat" title={`${c.ahead} ahead of ${base}, ${c.behind} behind, ${c.dirty} uncommitted`}>
              {/* the glyph is a label, not a digit: it gets its own breathing
                  room rather than sitting flush against the count */}
              <span className={c.ahead ? 'ahead on' : 'ahead'}>
                <i>↑</i>
                {c.ahead}
              </span>
              <span className={c.behind ? 'behind on' : 'behind'}>
                <i>↓</i>
                {c.behind}
              </span>
              <span className={c.dirty ? 'dirty on' : 'dirty'}>
                <i>●</i>
                {c.dirty}
              </span>
            </span>
          </div>
          <div className="wl-sub">
            <span className="wl-branch dim" title={c.branch}>
              <Hits text={c.detached ? 'detached' : c.branch} terms={terms} />
            </span>
          </div>
        </div>
      ))}
      {!checkouts.length && <div className="empty small">no worktrees</div>}
    </div>
  )
}

// Whether GitHub is testing what this checkout pushed.
//
// It reports on the commit the REMOTE has, not the one on disk: "is what I
// pushed passing" is the question, and a commit nobody has seen is not being
// tested by anybody. Nothing is drawn when grove could not ask — an empty
// space is honest, and a grey dot would claim GitHub had answered.
export function CheckDot({ checks }: { checks?: Checks }) {
  if (!checks || checks.state === 'none') return null
  const word =
    checks.state === 'success' ? 'passing' : checks.state === 'failure' ? 'failing' : 'running'
  const detail = (checks.runs ?? [])
    .map((r) => `${r.conclusion || r.status}  ${r.name}`)
    .join('\n')
  return (
    <span
      className={'ci ci-' + checks.state}
      title={`GitHub checks on the pushed commit: ${word}\n\n${detail}`}
      aria-label={`checks ${word}`}
    />
  )
}
