import { useState } from 'react'
import type { ScopeRepo } from '../types'
import { useScrollToActive } from '../lib/scroll'
import { commitState } from '../lib/commit'
import { matchesFields } from '../lib/filter'
import { Hits } from './Hits'

// The scope list: what the diff shows, per repo. A worktree's changes have
// several honest readings — against the base branch, against what is pushed,
// the index halves, one commit — and this column is the choice between them.
// Repos are the section headlines, because each has its own branch, its own
// upstream and its own commits.

const kindLabel: Record<string, string> = {
  range: '',
  staged: 'index vs HEAD',
  unstaged: 'worktree vs index',
  commit: '',
}

export function ScopeList({
  repos,
  sel,
  unmergedOnly,
  terms,
  onPick,
}: {
  repos: ScopeRepo[] | null
  sel: { repo: string; scope: string } | null
  // drop the commits the base branch already has — the grey ones — so a
  // branch reads as its own work and not the history it sits on
  unmergedOnly: boolean
  // the column's text filter, over a row's subject, sha, author and hint
  terms: string[]
  onPick: (repo: string, scope: string) => void
}) {
  const box = useScrollToActive('.sc-active', `${sel?.repo}:${sel?.scope}`)
  // repo sections fold away: a checkout with submodules lists several repos'
  // worth of commits and only one of them is usually the one being worked on
  const [folded, setFolded] = useState<Set<string>>(new Set())
  const fold = (name: string) =>
    setFolded((prev) => {
      const next = new Set(prev)
      next.has(name) ? next.delete(name) : next.add(name)
      return next
    })
  if (!repos) return <div className="sb-scopes"><div className="empty small">loading…</div></div>

  return (
    <div className="sb-scopes" ref={box}>
      {repos.map((r) => {
        const shown = r.scopes.filter(
          (s) =>
            (!unmergedOnly || s.kind !== 'commit' || !s.merged) &&
            matchesFields(terms, [s.label, s.sha, s.author, s.hint]),
        )
        const landed = unmergedOnly ? r.scopes.filter((s) => s.kind === 'commit' && s.merged).length : 0
        return (
        <div key={r.name} className="sc-repo">
          <div
            className="sc-head"
            title={[r.branch, r.upstream && `tracks ${r.upstream}`].filter(Boolean).join(' · ')}
            onClick={() => fold(r.name)}
          >
            <svg className={folded.has(r.name) ? 'sc-chev' : 'sc-chev sc-chev-open'} viewBox="0 0 16 16" aria-hidden="true">
              <path d="M6 3.5L10.5 8L6 12.5" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
            <span className="sc-head-name">{r.name}</span>
            <span className="sc-head-branch mono">{r.branch || 'detached'}</span>
          </div>

          {!folded.has(r.name) && shown.map((s) => {
            const active = sel?.repo === r.name && sel?.scope === s.id
            const commit = s.kind === 'commit'
            const state = commit ? commitState(s, r.base) : null
            return (
              <div
                key={s.id}
                className={`sc-row${active ? ' sc-active' : ''}${s.files ? '' : ' sc-empty'}${commit ? ' sc-commit' : ''}`}
                onClick={() => onPick(r.name, s.id)}
                title={[s.label, s.hint || kindLabel[s.kind], s.date, state?.title].filter(Boolean).join(' · ')}
              >
                <div className="sc-line">
                  {state && <span className={state.cls} title={state.title} />}
                  <span className="sc-label">
                    <Hits text={s.label} terms={terms} />
                  </span>
                  {!!(s.added || s.deleted) && (
                    <span className="sc-stat">
                      {!!s.added && <span className="plus">+{s.added}</span>}
                      {!!s.deleted && <span className="minus">−{s.deleted}</span>}
                    </span>
                  )}
                </div>
                <div className="sc-sub dim">
                  {commit ? (
                    <>
                      <span className="mono">
                        <Hits text={s.sha || ''} terms={terms} />
                      </span>{' '}
                      · {s.date}
                    </>
                  ) : (
                    <>
                      {s.files} file{s.files === 1 ? '' : 's'}
                      {s.hint && (
                        <>
                          {' '}
                          · <Hits text={s.hint} terms={terms} />
                        </>
                      )}
                    </>
                  )}
                </div>
              </div>
            )
          })}
          {!folded.has(r.name) && landed > 0 && (
            <div className="sc-hidden dim">{landed} landed · hidden</div>
          )}
        </div>
        )
      })}
    </div>
  )
}
