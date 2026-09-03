import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from './api'
import type { Checkout, Checks, Prefs, Repo, State, Update } from './types'
import { RepoList } from './components/RepoList'
import { WorktreeList } from './components/WorktreeList'
import { Sidebar } from './components/Sidebar'
import { Logo } from './components/ui'
import { UpdateNotice } from './components/UpdateNotice'
import { Settings } from './components/Settings'
import { ChecksDialog } from './components/ChecksLine'
import { fmtAgo } from './lib/format'
import { clamp, Splitter, useStoredWidth } from './components/Splitter'
import { PaneFilter, PaneHead, PaneRail, useFolded, useStoredFlag } from './components/Pane'
import { matchesFields, parseTerms } from './lib/filter'
import { readUrl, writeUrl } from './lib/urlstate'

const POLL_MS = 4000
const REPO_POLL_MS = 30000

// Three panes, left to right: the repositories in the start directory, the
// worktrees of the one selected, and the viewer for the worktree selected —
// its diff or everything else known about it. Each pane narrows what the next
// one shows, and both separators are draggable.
export default function App() {
  // what the fragment asked for at load: consumed once per pane, then the
  // user's clicks own the state and the fragment follows them
  const wanted = useRef(readUrl())
  const [repos, setRepos] = useState<Repo[] | null>(null)
  const [dir, setDir] = useState('')
  // asked once, on load. The server does the checking on its own schedule and
  // answers from what it last learned, so this never waits on the network.
  const [update, setUpdate] = useState<Update | null>(null)
  // what GitHub says about each checkout's pushed commit, asked for the
  // selected repository only and left alone otherwise
  const [checks, setChecks] = useState<Record<string, Checks>>({})
  const [showSettings, setShowSettings] = useState(false)
  const [showChecks, setShowChecks] = useState<string | null>(null)
  // which of the offers are switched on. The settings live in another window,
  // so a change there arrives through localStorage — the value is only a
  // nudge; the answer still comes from the server.
  const [prefs, setPrefs] = useState<Prefs | null>(null)
  useEffect(() => {
    const read = () => api.prefs().then(setPrefs).catch(() => {})
    read()
    const heard = (e: StorageEvent) => {
      if (e.key === null || e.key === 'grove_prefs_rev') read()
    }
    window.addEventListener('storage', heard)
    return () => window.removeEventListener('storage', heard)
  }, [])
  useEffect(() => {
    api.version().then(setUpdate).catch(() => {})
  }, [])
  const [repo, setRepo] = useState('')
  const [state, setState] = useState<State | null>(null)
  const [checkout, setCheckout] = useState('')
  // The diff tab's own selection, reported up so the fragment has one writer.
  // Seeded FROM the fragment: App writes it as soon as it knows the repo, which
  // happens before the viewer has mounted and read it — starting empty would
  // erase a deep link's scope and file before anything could restore them.
  const [diffSel, setDiffSel] = useState<{ sub?: string; scope?: string; file?: string }>(() => ({
    sub: wanted.current.sub,
    scope: wanted.current.scope,
    file: wanted.current.file,
  }))
  const [diffRepo, setDiffRepo] = useState<string | undefined>(undefined)
  const [error, setError] = useState('')
  // each pane filters its own list, from a slideout under its header that
  // stays open or closed the way it was left; closing one clears its text
  const [reposQ, setReposQ] = useState('')
  const [treesQ, setTreesQ] = useState('')
  const [reposFilter, setReposFilter] = useStoredFlag('grove_repos_filter')
  const [treesFilter, setTreesFilter] = useStoredFlag('grove_trees_filter')
  const toggleReposFilter = () => {
    if (reposFilter) setReposQ('')
    setReposFilter(!reposFilter)
  }
  const toggleTreesFilter = () => {
    if (treesFilter) setTreesQ('')
    setTreesFilter(!treesFilter)
  }
  const [refreshing, setRefreshing] = useState(false)

  const [reposW, setReposW, resetReposW] = useStoredWidth('grove_repos_w', () => 210)
  const [treesW, setTreesW, resetTreesW] = useStoredWidth('grove_trees_w', () => 300)
  const [reposFold, setReposFold] = useFolded('grove_repos_fold')
  const [treesFold, setTreesFold] = useFolded('grove_trees_fold')

  // the repo list, and the first selection: the top repo, which by the sort is
  // the one most worked in
  const loadRepos = useCallback(async () => {
    try {
      const r = await api.repos()
      setRepos(r.repos)
      setDir(r.dir)
      setRepo((cur) => {
        if (cur) return cur
        const askedName = wanted.current.repo
        const asked = askedName ? r.repos.find((x) => x.name === askedName) : undefined
        return (asked ?? r.repos[0])?.path || ''
      })
      setError('')
    } catch (e) {
      setError(String(e))
    }
  }, [])

  useEffect(() => {
    loadRepos()
    const t = setInterval(loadRepos, REPO_POLL_MS)
    return () => clearInterval(t)
  }, [loadRepos])

  const loadState = useCallback(async () => {
    if (!repo) return
    try {
      const s = await api.state(repo)
      setState(s)
      setError('')
    } catch (e) {
      setError(String(e))
    }
  }, [repo])

  useEffect(() => {
    loadState()
    const t = setInterval(loadState, POLL_MS)
    return () => clearInterval(t)
  }, [loadState])

  // switching repo selects its main checkout — the one that is always there
  const wantMain = useRef(false)
  useEffect(() => {
    wantMain.current = true
    setState(null)
  }, [repo])
  useEffect(() => {
    if (!wantMain.current || !state?.checkouts.length) return
    wantMain.current = false
    const askedName = wanted.current.wt
    const asked = askedName ? state.checkouts.find((c) => c.name === askedName) : undefined
    setCheckout((asked ?? state.checkouts.find((c) => c.is_main) ?? state.checkouts[0]).name)
    setDiffRepo(undefined)
    wanted.current = { ...wanted.current, wt: undefined } // consumed
  }, [state])

  const refresh = async () => {
    setRefreshing(true)
    try {
      await Promise.all([loadRepos(), loadState()])
    } finally {
      setRefreshing(false)
    }
  }

  // what each checkout's checks were last time, so a run that FINISHES can be
  // told from one that was already finished when grove started looking
  const wasChecking = useRef<Record<string, string>>({})
  useEffect(() => {
    if (!repo) return
    let stop = false
    const ask = () =>
      api
        .checks(repo)
        .then((r) => {
          if (stop) return
          const now = r.checks || {}
          for (const [name, c] of Object.entries(now)) {
            const before = wasChecking.current[name]
            // only the transition, and only once: a green branch says green on
            // its own, and being told about it every thirty seconds is not news
            if (before === 'pending' && (c.state === 'success' || c.state === 'failure')) {
              api
                .notify(
                  `${name}: checks ${c.state === 'success' ? 'passed' : 'failed'}`,
                  `${c.total} check${c.total === 1 ? '' : 's'} on ${c.sha?.slice(0, 8) ?? 'the pushed commit'}`,
                )
                .catch(() => {})
            }
            wasChecking.current[name] = c.state
          }
          setChecks(now)
        })
        .catch(() => {})
    ask()
    // a run that is still going is worth asking about again; one that has
    // finished is not going to change
    const id = setInterval(ask, 30000)
    return () => {
      stop = true
      clearInterval(id)
    }
  }, [repo])

  const checkouts = state?.checkouts ?? []
  const treeTerms = useMemo(() => parseTerms(treesQ), [treesQ])
  const shown = useMemo(
    () =>
      treeTerms.length
        ? checkouts.filter((c) => matchesFields(treeTerms, [c.name, c.branch, c.head.subject]))
        : checkouts,
    [checkouts, treeTerms],
  )
  const repoTerms = useMemo(() => parseTerms(reposQ), [reposQ])
  const shownRepos = useMemo(
    () => (repos && repoTerms.length ? repos.filter((r) => matchesFields(repoTerms, [r.name, r.branch])) : repos),
    [repos, repoTerms],
  )

  const current: Checkout | undefined = checkouts.find((c) => c.name === checkout)

  // the fragment mirrors the view: repo, worktree, and what the viewer shows
  const repoName = repos?.find((r) => r.path === repo)?.name
  useEffect(() => {
    if (!repoName) return
    writeUrl({ repo: repoName, wt: checkout, ...diffSel })
  }, [repoName, checkout, diffSel])

  return (
    <div className="app">
      <header className="topbar">
        <div className="brand">
          <Logo />
          <div className="brand-text">
            <div className="brand-title">grove</div>
            <div className="brand-sub mono" title={dir}>
              {dir || '…'}
            </div>
          </div>
        </div>

        <div className="topbar-tools">
          {update?.available && <UpdateNotice update={update} />}
          {/* the window has this in its own menu, under cmd-comma, where it
              belongs; a browser tab has no menu bar to put it in */}
          {!update?.desktop && (
            <button className="btn-ghost" onClick={() => setShowSettings(true)} title="what grove is standing on">
              Settings
            </button>
          )}
          <button className="btn-ghost" onClick={refresh} disabled={refreshing}>
            {refreshing ? 'Refreshing…' : 'Refresh'}
          </button>
        </div>
      </header>

      {showSettings && <Settings onClose={() => setShowSettings(false)} />}

      {showChecks && checks[showChecks] && (
        <ChecksDialog
          name={showChecks}
          checks={checks[showChecks]}
          desktop={!!update?.desktop}
          onClose={() => setShowChecks(null)}
        />
      )}

      {(error || state?.git_error) && <div className="error error-bar">{error || state?.git_error}</div>}

      <div className="panes">
        {reposFold ? (
          <PaneRail label="repos" onExpand={() => setReposFold(false)} />
        ) : (
          <>
            <div className="pane" style={{ width: reposW }}>
              <PaneHead
                label="repos"
                meta={
                  repos && shownRepos
                    ? shownRepos.length !== repos.length
                      ? `${shownRepos.length} / ${repos.length}`
                      : repos.length
                    : ''
                }
                onCollapse={() => setReposFold(true)}
                filter={{ open: reposFilter, active: repoTerms.length > 0, onToggle: toggleReposFilter }}
              />
              {reposFilter && <PaneFilter value={reposQ} onChange={setReposQ} placeholder="filter repos…" />}
              <div className="pane-body">
                <RepoList repos={shownRepos} sel={repo} terms={repoTerms} filtering={repoTerms.length > 0} onPick={setRepo} />
              </div>
            </div>

            <Splitter onMove={(x) => setReposW(clamp(x, 150, 420))} onReset={resetReposW} />
          </>
        )}

        {treesFold ? (
          <PaneRail label="worktrees" onExpand={() => setTreesFold(false)} />
        ) : (
          <>
            <div className="pane" style={{ width: treesW }}>
              <PaneHead
                label="worktrees"
                meta={state ? (shown.length !== checkouts.length ? `${shown.length} / ${checkouts.length}` : checkouts.length) : ''}
                onCollapse={() => setTreesFold(true)}
                filter={{ open: treesFilter, active: treeTerms.length > 0, onToggle: toggleTreesFilter }}
              />
              {treesFilter && <PaneFilter value={treesQ} onChange={setTreesQ} placeholder="filter worktrees…" />}
          <div className="pane-body">
            {state ? (
              <WorktreeList
                checkouts={shown}
                base={state.base}
                sel={checkout}
                terms={treeTerms}
                checks={checks}
                onChecks={setShowChecks}
                onPick={(n) => {
                  setCheckout(n)
                  setDiffRepo(undefined)
                }}
              />
            ) : repos && !repos.length ? (
              // there are no repositories at all, and the repos column already
              // says so. A second column insisting it is loading something is
              // just a window that looks broken.
              null
            ) : (
              <div className="empty small">loading…</div>
            )}
          </div>
            </div>

            <Splitter
              onMove={(x) => setTreesW(clamp(x - (reposFold ? 24 : reposW + 7), 200, 560))}
              onReset={resetTreesW}
            />
          </>
        )}

        <div className="pane pane-grow">
          {current ? (
            <Sidebar
              c={current}
              base={state?.base ?? ''}
              repo={diffRepo}
              terminal={prefs?.terminal !== 'off'}
              editor={prefs?.editor !== 'off'}
              onDiffSel={setDiffSel}
            />
          ) : (
            <div className="empty">select a worktree</div>
          )}
        </div>
      </div>

      <footer className="foot">
        <span>git scanned {fmtAgo(state?.git_at)}</span>
        <span className="dim">
          comparing against <span className="mono">{state?.base ?? '…'}</span>
        </span>
      </footer>
    </div>
  )
}
