import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import type { RemoteRepo, RemoteResult } from '../types'

// Push and Pull, in the checkout's head.
//
// Both are decided by the server and re-decided after the fetch it does on the
// way, so what is on screen can go stale without becoming a lie. Neither ever
// forces anything: grove does not rewrite history, yours included.
//
// One repository at a time. A checkout carrying submodules is several
// repositories with separate remotes and separate standings — "push these
// three" is one button hiding three different answers to whether it is even
// allowed, and the one that matters is usually the one it hid.
//
// The checkout's repositories are fetched while it is open, which is what lets
// the buttons mean something — "behind" read from refs nobody has updated
// since Tuesday is not information. Every five seconds is the aim and not
// always the outcome: a cycle over four repositories on a real remote measured
// eight seconds here, and something that takes eight seconds cannot happen
// every five. The next round waits twice however long the last one took, so
// quick remotes get the five seconds asked for and slow ones settle at
// spending a third of the time fetching and the rest leaving the remote alone.

type Action = 'push' | 'rebase' | 'merge' | 'ff'
const FETCH_EVERY = 5000

type Way = { mode: Exclude<Action, 'push'>; label: string; what: string }

// What each way in actually does, in a sentence, because the difference
// matters and the names do not say it.
const WAYS: Way[] = [
  {
    mode: 'ff',
    label: 'Fast-forward',
    what: 'Move your branch onto theirs. Nothing is rewritten and no merge commit is made — possible only when you have no commits of your own on top.',
  },
  {
    mode: 'rebase',
    label: 'Rebase',
    what: 'Replay your commits on top of theirs, so the branch stays a straight line. Your commits get new ids, which is fine while nobody else has them.',
  },
  {
    mode: 'merge',
    label: 'Merge',
    what: 'Bring theirs in beside yours and tie the two together with a merge commit. Nothing is rewritten, so this is the answer once others have your commits.',
  },
]

export function RemoteBar({ name, onChanged }: { name: string; onChanged: () => void }) {
  const [repos, setRepos] = useState<RemoteRepo[] | null>(null)
  const [pick, setPick] = useState<string | null>(null) // repo name; null = the checkout's own
  const [remote, setRemote] = useState<string | null>(null)
  const [busy, setBusy] = useState<Action | null>(null)
  const [results, setResults] = useState<RemoteResult[] | null>(null)
  const [menu, setMenu] = useState<'push' | 'pull' | null>(null)
  const [asking, setAsking] = useState(false) // the how-to-pull dialog

  const load = useCallback(
    () =>
      api
        .remote(name)
        .then((s) => setRepos(s.repos))
        .catch(() => {}),
    [name],
  )

  useEffect(() => {
    setRepos(null)
    setPick(null)
    setRemote(null)
    setResults(null)
    setAsking(false)
    load()
  }, [name, load])

  useEffect(() => {
    let stopped = false
    let timer = 0
    const round = async () => {
      if (stopped) return
      let wait = FETCH_EVERY
      // nothing is worth fetching for a window nobody is looking at
      if (!document.hidden) {
        const started = performance.now()
        try {
          const res = await api.remoteAct({ name, repos: [], action: 'fetch' })
          if (!stopped) setRepos(res.repos)
        } catch {
          // offline, or a remote that wants a password: say nothing and carry
          // on showing what was last known
        }
        wait = Math.max(FETCH_EVERY, (performance.now() - started) * 2)
      }
      if (!stopped) timer = window.setTimeout(round, wait)
    }
    timer = window.setTimeout(round, FETCH_EVERY)
    return () => {
      stopped = true
      clearTimeout(timer)
    }
  }, [name])

  useEffect(() => {
    if (!menu) return
    const close = (e: MouseEvent) => {
      if (!(e.target as HTMLElement).closest('.rb')) setMenu(null)
    }
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [menu])

  if (!repos || !repos.length) return null

  const repo = repos.find((r) => r.name === pick) ?? repos[0]
  const remotes = repo.remotes ?? []
  const pushTo = remote ?? repo.remote

  // which ways in are actually available here, and which one grove will stand
  // behind. Nothing of ours on top is the only case with one right answer.
  const offered = repo.can_pull
    ? WAYS.filter((w) => w.mode !== 'ff' || repo.ahead === 0)
    : []
  const advised = repo.ahead === 0 ? 'ff' : repo.pull_mode || ''

  const act = (action: Action) => {
    setBusy(action)
    setResults(null)
    setMenu(null)
    setAsking(false)
    api
      .remoteAct({ name, repos: [repo.name], action, remote: remote ?? undefined })
      .then((res) => {
        setRepos(res.repos)
        setResults(res.results)
        onChanged()
      })
      .catch((e) => setResults([{ repo: '', ok: false, detail: String(e.message || e) }]))
      .finally(() => setBusy(null))
  }

  const which = repo.detached ? 'detached' : repo.branch ?? ''

  return (
    <div className="rb">
      <div className="rb-row">
        <span className="rb-split">
          <button
            className="btn-ghost rb-go"
            disabled={!repo.can_push || busy !== null}
            onClick={() => act('push')}
            title={repo.can_push ? `Fetch, then push ${repo.name} to ${pushTo}` : repo.blocked || 'nothing to push'}
          >
            {busy === 'push' ? 'Pushing…' : `Push ${which}`}
            {repo.ahead > 0 && <span className="rb-n"> {repo.ahead}</span>}
          </button>
          <button
            className={menu === 'push' ? 'btn-ghost rb-caret on' : 'btn-ghost rb-caret'}
            onClick={() => setMenu((m) => (m === 'push' ? null : 'push'))}
            title="which repository, and where"
          >
            ▾
          </button>
          {menu === 'push' && (
            <div className="rb-pop">
              <RepoPicker repos={repos} pick={repo.name} onPick={(n) => { setPick(n); setRemote(null) }} />
              {remotes.length > 1 && (
                <>
                  <div className="rb-sep" />
                  <div className="rb-pop-title">remote</div>
                  {remotes.map((rm) => (
                    <label key={rm} className="rb-pop-row">
                      <input type="radio" name="rb-remote" checked={pushTo === rm} onChange={() => setRemote(rm)} />
                      <span className="mono">{rm}</span>
                    </label>
                  ))}
                </>
              )}
            </div>
          )}
        </span>

        <span className="rb-split">
          <button
            className="btn-ghost rb-go"
            disabled={!repo.can_pull || busy !== null}
            onClick={() => setAsking(true)}
            title={repo.can_pull ? `Bring in ${repo.behind} from ${repo.upstream}` : repo.pull_blocked || 'already up to date'}
          >
            {busy && busy !== 'push' ? 'Pulling…' : `Pull ${which}`}
            {repo.behind > 0 && <span className="rb-n"> {repo.behind}</span>}
          </button>
          <button
            className={menu === 'pull' ? 'btn-ghost rb-caret on' : 'btn-ghost rb-caret'}
            onClick={() => setMenu((m) => (m === 'pull' ? null : 'pull'))}
            title="which repository"
          >
            ▾
          </button>
          {menu === 'pull' && (
            <div className="rb-pop">
              <RepoPicker repos={repos} pick={repo.name} onPick={(n) => { setPick(n); setRemote(null) }} />
            </div>
          )}
        </span>
      </div>

      {!repo.can_push && repo.blocked && !repo.detached && !busy && (
        <div className="rb-note dim">
          <span className="mono">{repo.name}</span> — {repo.blocked}
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

      {asking && (
        <HowToPull repo={repo} offered={offered} advised={advised} onCancel={() => setAsking(false)} onPick={act} />
      )}
    </div>
  )
}

// The three ways in are not interchangeable and their names do not say how
// they differ, so the choice is made in front of the reader rather than behind
// a caret — with what each one does, and which one grove will stand behind
// when the situation has one right answer.
function HowToPull({
  repo,
  offered,
  advised,
  onCancel,
  onPick,
}: {
  repo: RemoteRepo
  offered: Way[]
  advised: string
  onCancel: () => void
  onPick: (a: Action) => void
}) {
  const [mode, setMode] = useState<Action>((advised || offered[0]?.mode || 'merge') as Action)
  return (
    <div className="modal-backdrop" onClick={onCancel}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h2 className="modal-title">
          Pull {repo.behind} into <span className="mono">{repo.name}</span>
        </h2>
        <div className="modal-body">
          <p className="dim">
            <span className="mono">{repo.upstream}</span> has {repo.behind} commit
            {repo.behind === 1 ? '' : 's'} you do not
            {repo.ahead > 0 && <>, and you have {repo.ahead} it does not</>}.
          </p>
          {repo.ahead === 0 ? (
            <p className="dim">
              Nothing of yours sits on top, so there is one right answer and no history to rewrite.
            </p>
          ) : (
            <p className="dim">
              Your {repo.ahead} commit{repo.ahead === 1 ? ' sits' : 's sit'} on top, so this is a judgement:
              rebase while they are still yours alone, merge once anybody else has them. grove cannot
              tell which, so it does not pretend to.
            </p>
          )}
          <div className="rb-ways">
            {offered.map((w) => (
              <label key={w.mode} className={mode === w.mode ? 'rb-way-row on' : 'rb-way-row'}>
                <input type="radio" name="rb-how" checked={mode === w.mode} onChange={() => setMode(w.mode)} />
                <span className="rb-way-head">
                  {w.label}
                  {w.mode === advised && <span className="rb-advised">suggested</span>}
                </span>
                <span className="rb-way-what dim">{w.what}</span>
              </label>
            ))}
          </div>
        </div>
        <div className="modal-actions">
          <button className="btn-ghost" onClick={onCancel}>
            Cancel
          </button>
          <button className="btn-ghost rb-go" onClick={() => onPick(mode)} disabled={!offered.length}>
            {WAYS.find((w) => w.mode === mode)?.label ?? 'Pull'}
          </button>
        </div>
      </div>
    </div>
  )
}

function RepoPicker({
  repos,
  pick,
  onPick,
}: {
  repos: RemoteRepo[]
  pick: string
  onPick: (n: string) => void
}) {
  return (
    <>
      <div className="rb-pop-title">repository</div>
      {repos.map((r) => (
        <label
          key={r.name}
          className={r.detached ? 'rb-pop-row rb-detached' : 'rb-pop-row'}
          title={r.blocked || r.pull_blocked || ''}
        >
          <input type="radio" name="rb-repo" checked={pick === r.name} onChange={() => onPick(r.name)} />
          <span className="mono">{r.name}</span>
          <span className="dim">{r.detached ? 'detached' : r.branch}</span>
          <span className="rb-num">
            {r.ahead > 0 && <span className="ahead">↑ {r.ahead}</span>}
            {r.behind > 0 && <span className="behind">↓ {r.behind}</span>}
          </span>
          {r.gitlink_unknown && (
            <span className="rb-warn" title={`${r.gitlink} is on no remote branch`}>
              unpushed id
            </span>
          )}
        </label>
      ))}
    </>
  )
}
