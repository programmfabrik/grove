import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from './api'
import type { Checkout, PanelTab, Repo, State } from './types'
import { RepoList } from './components/RepoList'
import { WorktreeList } from './components/WorktreeList'
import { Sidebar } from './components/Sidebar'
import { Logo } from './components/ui'
import { fmtAgo } from './lib/format'
import { clamp, Splitter, useStoredWidth } from './components/Splitter'
import { PaneHead, PaneRail, useFolded } from './components/Pane'
import { getThemePref, setThemePref, type ThemePref } from './lib/theme'
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
  const [repo, setRepo] = useState('')
  const [state, setState] = useState<State | null>(null)
  const [checkout, setCheckout] = useState('')
  const [tab, setTab] = useState<PanelTab>(wanted.current.tab === 'worktree' ? 'worktree' : 'diff')
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
  const [q, setQ] = useState('')
  const [theme, setTheme] = useState<ThemePref>(getThemePref())
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

  const checkouts = state?.checkouts ?? []
  const shown = useMemo(() => {
    const needle = q.trim().toLowerCase()
    if (!needle) return checkouts
    return checkouts.filter((c) =>
      [c.name, c.branch, c.ticket, c.head.subject]
        .filter(Boolean)
        .some((v) => String(v).toLowerCase().includes(needle)),
    )
  }, [checkouts, q])

  const current: Checkout | undefined = checkouts.find((c) => c.name === checkout)

  // the fragment mirrors the view: repo, worktree, tab and — on the diff tab —
  // what the viewer is showing
  const repoName = repos?.find((r) => r.path === repo)?.name
  useEffect(() => {
    if (!repoName) return
    writeUrl({ repo: repoName, wt: checkout, tab, ...(tab === 'diff' ? diffSel : {}) })
  }, [repoName, checkout, tab, diffSel])

  return (
    <div className="app">
      <header className="topbar">
        <div className="brand">
          <Logo />
          <div>
            <div className="brand-title">grove</div>
            <div className="brand-sub mono" title={dir}>
              {dir || '…'}
            </div>
          </div>
        </div>

        <div className="topbar-tools">
          <input
            className="search"
            placeholder="filter worktrees…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
          <button className="btn-ghost" onClick={refresh} disabled={refreshing}>
            {refreshing ? 'Refreshing…' : 'Refresh'}
          </button>
          <div className="seg">
            {(['light', 'dark', 'system'] as ThemePref[]).map((t) => (
              <button
                key={t}
                className={theme === t ? 'active' : ''}
                onClick={() => {
                  setThemePref(t)
                  setTheme(t)
                }}
              >
                {t === 'light' ? '☀' : t === 'dark' ? '☾' : 'auto'}
              </button>
            ))}
          </div>
        </div>
      </header>

      {(error || state?.git_error) && <div className="error error-bar">{error || state?.git_error}</div>}

      <div className="panes">
        {reposFold ? (
          <PaneRail label="repos" onExpand={() => setReposFold(false)} />
        ) : (
          <>
            <div className="pane" style={{ width: reposW }}>
              <PaneHead label="repos" meta={repos?.length ?? ''} onCollapse={() => setReposFold(true)} />
              <div className="pane-body">
                <RepoList repos={repos} sel={repo} onPick={setRepo} />
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
                meta={state ? `${shown.length}${shown.length !== checkouts.length ? `/${checkouts.length}` : ''}` : ''}
                onCollapse={() => setTreesFold(true)}
              />
          <div className="pane-body">
            {state ? (
              <WorktreeList
                checkouts={shown}
                base={state.base}
                sel={checkout}
                onPick={(n) => {
                  setCheckout(n)
                  setDiffRepo(undefined)
                }}
              />
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
            <Sidebar c={current} base={state?.base ?? ''} repo={diffRepo} tab={tab} onTab={setTab} onDiffSel={setDiffSel} />
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
