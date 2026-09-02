import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import type { RemoteRepo, RemoteResult } from '../types'
import { fmtAgo } from '../lib/format'

// Push, and the two ways out when it cannot.
//
// The button is only ever enabled when the server says so, and the server
// decides again after the fetch it does on the way — so what is on screen can
// go stale without becoming a lie. When the remote has moved, push greys out
// and the two honest answers take its place: rebase onto it, or merge it.
//
// The select is which repositories to act on, because a checkout with
// submodules is several of them with separate remotes. Submodules are pushed
// before the parent: pushing a submodule is what makes the parent's pointer to
// it followable, so the other order publishes exactly the state grove refuses
// to publish.

type Action = 'fetch' | 'push' | 'rebase' | 'merge'

export function RemoteBar({ name, onChanged }: { name: string; onChanged: () => void }) {
  const [repos, setRepos] = useState<RemoteRepo[] | null>(null)
  const [repoPath, setRepoPath] = useState('')
  const [auto, setAuto] = useState(false)
  const [autoOpen, setAutoOpen] = useState(false)
  const [picked, setPicked] = useState<string[] | null>(null) // null = all
  const [busy, setBusy] = useState<Action | null>(null)
  const [results, setResults] = useState<RemoteResult[] | null>(null)
  const [open, setOpen] = useState(false)

  const load = useCallback(() => {
    api
      .remote(name)
      .then((s) => {
        setRepos(s.repos)
        setRepoPath(s.repo || '')
        setAuto(!!s.auto_fetch)
      })
      .catch(() => setRepos(null))
  }, [name])

  useEffect(() => {
    setRepos(null)
    setPicked(null)
    setResults(null)
    load()
  }, [name, load])

  if (!repos || !repos.length) return null

  // A submodule is normally detached — that is what a submodule IS, a commit
  // rather than a branch — so it is not something to push and not something to
  // complain about. It starts unselected and says nothing.
  const actionable = repos.filter((r) => !r.detached).map((r) => r.name)
  const selected = picked ?? actionable
  const chosen = repos.filter((r) => selected.includes(r.name))
  const parent = repos[0]

  // what the selection can do, as the server sees it
  const canPush = chosen.some((r) => r.can_push)
  const behind = chosen.filter((r) => r.behind > 0)
  // only obstacles worth acting on: being detached is a standing fact, not
  // something the reader can do anything about
  const blocked = chosen.filter((r) => !r.can_push && r.blocked && !r.detached)
  const ahead = chosen.reduce((n, r) => n + r.ahead, 0)
  const stale = parent.fetched_at

  const act = (action: Action) => {
    setBusy(action)
    setResults(null)
    api
      .remoteAct({ name, repos: selected, action })
      .then((res) => {
        setRepos(res.repos)
        setResults(res.results)
        onChanged()
      })
      .catch((e) => setResults([{ repo: '', ok: false, detail: String(e.message || e) }]))
      .finally(() => setBusy(null))
  }

  const toggle = (n: string) =>
    setPicked((p) => {
      const cur = p ?? actionable
      return cur.includes(n) ? cur.filter((x) => x !== n) : [...cur, n]
    })

  return (
    <div className="rb">
      <div className="rb-row">
        <button
          className="btn-ghost rb-push"
          disabled={!canPush || busy !== null}
          onClick={() => act('push')}
          title={
            canPush
              ? `Fetch, then push ${ahead} commit${ahead === 1 ? '' : 's'}`
              : blocked.map((r) => `${r.name}: ${r.blocked}`).join('\n') || 'nothing to push'
          }
        >
          {busy === 'push' ? 'Pushing…' : `Push${ahead ? ` ${ahead}` : ''}`}
        </button>

        {/* the remote moved: rebase or merge are the only honest answers */}
        {behind.length > 0 && (
          <>
            <button className="btn-ghost" disabled={busy !== null} onClick={() => act('rebase')}
              title={behind.map((r) => `${r.name}: ${r.behind} behind ${r.upstream}`).join('\n')}>
              {busy === 'rebase' ? 'Rebasing…' : 'Fetch & rebase'}
            </button>
            <button className="btn-ghost" disabled={busy !== null} onClick={() => act('merge')}>
              {busy === 'merge' ? 'Merging…' : 'Fetch & merge'}
            </button>
          </>
        )}

        <span className="rb-split-btn">
          <button className="btn-ghost" disabled={busy !== null} onClick={() => act('fetch')}
            title={stale ? `last fetched ${fmtAgo(stale)}` : 'never fetched'}>
            {busy === 'fetch' ? 'Fetching…' : 'Fetch'}
          </button>
          <button
            className={auto ? 'btn-ghost rb-caret on' : 'btn-ghost rb-caret'}
            onClick={() => setAutoOpen((o) => !o)}
            title={auto ? 'fetching on its own every 5 minutes' : 'fetch on a timer?'}
          >
            {auto ? '⟳' : '▾'}
          </button>
        </span>

        {/* which repositories: only worth a control when there is more than one */}
        {repos.length > 1 && (
          <button className={open ? 'btn-ghost on' : 'btn-ghost'} onClick={() => setOpen((o) => !o)}
            title="which repositories to act on">
            {selected.length === repos.length
              ? 'all repos'
              : `${selected.length} of ${repos.length}`}{' '}
            ▾
          </button>
        )}
        {stale && <span className="rb-stale dim" title={stale}>fetched {fmtAgo(stale)}</span>}
      </div>

      {autoOpen && (
        <div className="rb-pick">
          <label className="rb-pick-row" title={repoPath}>
            <input
              type="checkbox"
              checked={auto}
              disabled={!repoPath}
              onChange={(e) => {
                const on = e.target.checked
                setAuto(on)
                api.autoFetch(repoPath, on).catch(() => setAuto(!on))
              }}
            />
            <span>fetch every 5 minutes</span>
          </label>
          {/* said plainly: this is grove reaching out on its own, and it is the
              whole repository rather than this one checkout */}
          <div className="rb-hint dim">
            the whole repository — every worktree and their submodules
          </div>
        </div>
      )}

      {open && repos.length > 1 && (
        <div className="rb-pick">
          {repos.map((r) => (
            <label
              key={r.name}
              className={r.detached ? 'rb-pick-row rb-detached' : 'rb-pick-row'}
              title={r.blocked || ''}
            >
              <input type="checkbox" checked={selected.includes(r.name)} onChange={() => toggle(r.name)} />
              <span className="mono">{r.name}</span>
              <span className="dim">{r.detached ? 'detached' : r.branch}</span>
              <span className="rb-num">
                {r.ahead > 0 && <span className="on ahead">↑ {r.ahead}</span>}
                {r.behind > 0 && <span className="on behind">↓ {r.behind}</span>}
              </span>
              {r.gitlink_unknown && (
                <span className="rb-warn" title={`${r.gitlink} is on no remote branch`}>unpushed id</span>
              )}
            </label>
          ))}
        </div>
      )}

      {blocked.length > 0 && !busy && (
        <div className="rb-note dim">
          {blocked.map((r) => (
            <div key={r.name}>
              <span className="mono">{r.name}</span> — {r.blocked}
            </div>
          ))}
        </div>
      )}

      {results && (
        <div className="rb-note">
          {results.map((r, i) => (
            <div key={i} className={r.ok ? 'rb-ok' : 'rb-bad'}>
              {r.repo && <span className="mono">{r.repo}</span>} {r.detail}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
