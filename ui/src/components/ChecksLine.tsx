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

export function checkClass(checks?: Checks): string {
  if (!checks || checks.state === 'none') return ''
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

export function ChecksLine({ checks, onOpen }: { checks?: Checks; onOpen: () => void }) {
  if (!checks || checks.state === 'none') return null
  const passed = (checks.runs ?? []).filter((r) => r.status === 'completed').length
  return (
    <button
      className="ck-line"
      onClick={(e) => {
        e.stopPropagation() // the row underneath selects a worktree
        onOpen()
      }}
      title={`${passed} of ${checks.total} checks finished — click for all of them`}
    >
      <span className="ck-when">{checkSummary(checks)}</span>
      <span className={'ci ci-' + checks.state} />
    </button>
  )
}

// The runs behind the colour, and the way out to GitHub.
export function ChecksDialog({
  name,
  checks,
  desktop,
  onClose,
}: {
  name: string
  checks: Checks
  desktop: boolean
  onClose: () => void
}) {
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
            <span className={'ci ci-' + checks.state} />
            <span>
              {checkSummary(checks)} · <span className="mono">{(checks.sha ?? '').slice(0, 8)}</span>, the
              commit the remote has
            </span>
          </p>
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
