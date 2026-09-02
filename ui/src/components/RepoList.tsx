import type { Repo } from '../types'
import { fmtAgo } from '../lib/format'
import { useScrollToActive } from '../lib/scroll'
import { Hits } from './Hits'

// Pane one: the repositories in the start directory. Sorted by how much is
// checked out and then by when the repo was last touched — a repo with a dozen
// worktrees is where the work is, and among the single-checkout ones the
// recently used are the ones worth seeing first.
export function RepoList({
  repos,
  sel,
  terms,
  filtering,
  onPick,
}: {
  repos: Repo[] | null
  sel: string
  terms: string[]
  filtering: boolean
  onPick: (path: string) => void
}) {
  const box = useScrollToActive('.rp-active', sel)
  if (!repos) return <div className="empty small">loading…</div>
  return (
    <div className="rp-list" ref={box}>
      {repos.map((r) => (
        <div
          key={r.path}
          className={`rp-row${r.path === sel ? ' rp-active' : ''}`}
          onClick={() => onPick(r.path)}
          title={r.path}
        >
          <div className="rp-line">
            <span className="rp-name mono">
              <Hits text={r.name} terms={terms} />
            </span>
            {r.worktrees > 1 && <span className="rp-count" title={`${r.worktrees} checkouts`}>{r.worktrees}</span>}
            {r.dirty && <span className="rp-dirty" title="uncommitted changes" />}
          </div>
          <div className="rp-sub dim">
            <Hits text={r.branch || 'detached'} terms={terms} />
            {r.last_used && <> · {fmtAgo(r.last_used)}</>}
          </div>
        </div>
      ))}
      {!repos.length &&
        (filtering ? (
          <div className="empty small">no repository matches</div>
        ) : (
          // Not the same thing at all, and the difference is what somebody
          // opening grove for the first time needs to read: there is nothing
          // here, and here is how you point it somewhere there is something.
          <div className="empty small">
            <p>No git repositories in this directory.</p>
            <p className="dim">
              Choose one that holds your checkouts — <b>File → Open Folder…</b>, or start
              <code> grove</code> in it.
            </p>
          </div>
        ))}
    </div>
  )
}
