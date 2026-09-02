import type { Checkout } from '../types'
import { useScrollToActive } from '../lib/scroll'

// Pane two: the worktrees of the selected repo. Most repos have exactly one
// checkout and this is a single row; a repo with two dozen is the case worth
// designing for — name, branch, ticket, and how far each is from the base.
export function WorktreeList({
  checkouts,
  base,
  sel,
  onPick,
}: {
  checkouts: Checkout[]
  base: string
  sel: string
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
            <span className="wl-name mono">{c.name}</span>
            <span className="gitstat" title={`${c.ahead} ahead of ${base}, ${c.behind} behind, ${c.dirty} uncommitted`}>
              <span className={c.ahead ? 'ahead on' : 'ahead'}>↑{c.ahead}</span>
              <span className={c.behind ? 'behind on' : 'behind'}>↓{c.behind}</span>
              <span className={c.dirty ? 'dirty on' : 'dirty'}>●{c.dirty}</span>
            </span>
          </div>
          <div className="wl-sub">
            <span className="wl-branch dim" title={c.branch}>
              {c.detached ? 'detached' : c.branch}
            </span>
            {c.ticket && <span className="wl-ticket mono">#{c.ticket}</span>}
          </div>
        </div>
      ))}
      {!checkouts.length && <div className="empty small">no worktrees</div>}
    </div>
  )
}
