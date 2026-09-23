import { api } from '../api'
import type { Checks } from '../types'
import { fmtAgo, fmtDur } from '../lib/format'

// What GitHub is doing about the commit this checkout pushed, on the line the
// branch name is already on.
//
// The branch name takes the colour, because that is the thing the state is
// ABOUT — a coloured dot on its own says a state exists without saying whose.
// The right of the line says when, and how long, which is the question after
// the colour: green is worth knowing, green four days ago is worth knowing
// something else about.

// checksHere says whether the checks ran on what is checked out. They are asked
// for the branch's PUSHED tip — the only commit a remote can have run anything
// on — and the moment a commit is made on top of it, or the branch falls behind
// its remote, that is a different commit from the one in the worktree. The
// result is still true; it is just not about this checkout, and a green branch
// name with thirty-eight untested commits under it is the one thing a CI colour
// must never say. head is the checkout's abbreviated hash, a prefix of the sha.
export function checksHere(checks: Checks | undefined, head: string): boolean {
  return !!checks?.sha && !!head && checks.sha.startsWith(head)
}

export function checkClass(checks: Checks | undefined, head: string): string {
  if (!checks || checks.state === 'none' || !checksHere(checks, head)) return ''
  return ' ck-' + checks.state
}

// summary is the few words at the right: how long it has been running, or when
// it finished and how long it took.
export function checkSummary(checks: Checks): string {
  const start = checks.started ? Date.parse(checks.started) : NaN
  const end = checks.finished ? Date.parse(checks.finished) : NaN
  if (checks.state === 'pending') {
    return isFinite(start) ? `running ${fmtDur(Date.now() - start)}` : 'running'
  }
  const took = isFinite(start) && isFinite(end) ? fmtDur(end - start) : ''
  const when = checks.finished ? fmtAgo(checks.finished) : ''
  return [when, took].filter(Boolean).join(' · ')
}

export function ChecksLine({ checks, head, onOpen }: { checks?: Checks; head: string; onOpen: () => void }) {
  if (!checks || checks.state === 'none') return null
  const passed = (checks.runs ?? []).filter((r) => r.status === 'completed').length
  // still worth showing — when the pushed commit was last tested is a fact —
  // but grey, and saying which commit it was, since it is not this one
  const here = checksHere(checks, head)
  return (
    <button
      className="ck-line"
      onClick={(e) => {
        e.stopPropagation() // the row underneath selects a worktree
        onOpen()
      }}
      title={
        here
          ? `${passed} of ${checks.total} checks finished — click for all of them`
          : `These ran on ${(checks.sha ?? '').slice(0, 9)}, the commit the remote has — not on ${head}, which is checked out here`
      }
    >
      <span className="ck-when">{checkSummary(checks)}</span>
      <span className={'ci ' + (here ? 'ci-' + checks.state : 'ci-elsewhere')} />
    </button>
  )
}

// The runs behind the colour, and the way out to GitHub.
export function ChecksDialog({
  name,
  checks,
  head,
  desktop,
  onClose,
}: {
  name: string
  checks: Checks
  head: string
  desktop: boolean
  onClose: () => void
}) {
  const here = checksHere(checks, head)
  const go = (url?: string) => {
    if (!url) return
    // a window has no tabs to open one in; the browser you are signed in to does
    if (desktop) api.open(url).catch(() => {})
    else window.open(url, '_blank', 'noreferrer')
  }
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal modal-wide" onClick={(e) => e.stopPropagation()}>
        <h2 className="modal-title">
          Checks on <span className="mono">{name}</span>
        </h2>
        <div className="modal-body">
          <p className="dim ck-head">
            <span className={'ci ' + (here ? 'ci-' + checks.state : 'ci-elsewhere')} />
            <span>
              {checkSummary(checks)} · <span className="mono">{(checks.sha ?? '').slice(0, 8)}</span>, the
              commit the remote has
            </span>
          </p>
          {!here && head && (
            <p className="ck-elsewhere">
              Checked out in {name} is <span className="mono">{head}</span>, which is not the commit these ran
              on, so none of this is a verdict on it.
            </p>
          )}
          <div className="ck-runs">
            {(checks.runs ?? []).map((r, i) => {
              const start = r.started_at ? Date.parse(r.started_at) : NaN
              const end = r.completed_at ? Date.parse(r.completed_at) : Date.now()
              const took = isFinite(start) ? fmtDur(end - start) : ''
              const state =
                r.status !== 'completed' ? 'pending' : ok(r.conclusion) ? 'success' : 'failure'
              return (
                <button key={i} className="ck-run" onClick={() => go(r.url)} title={r.url || ''}>
                  <span className={'ci ci-' + state} />
                  <span className="ck-run-name">{r.name}</span>
                  <span className="ck-run-state dim">{r.conclusion || r.status.replace('_', ' ')}</span>
                  <span className="ck-run-took dim mono">{took}</span>
                </button>
              )
            })}
            {!checks.runs?.length && <p className="dim">GitHub has the commit and nothing has run for it.</p>}
          </div>
        </div>
        <div className="modal-actions">
          <button className="btn-ghost" onClick={onClose}>
            Close
          </button>
          <button className="btn-ghost rb-go" onClick={() => go(checks.url)} disabled={!checks.url}>
            Open in GitHub
          </button>
        </div>
      </div>
    </div>
  )
}

// what GitHub counts as not-a-failure
function ok(conclusion?: string) {
  return conclusion === 'success' || conclusion === 'neutral' || conclusion === 'skipped'
}
