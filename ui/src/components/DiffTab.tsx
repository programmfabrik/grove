import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '../api'
import type { Checkout, DiffFile, ScopeRepo } from '../types'
import { buildTree, filterTree, firstLeaf, leavesOf, TreeRows, visibleLeaves, type Node } from './DiffTree'
import { ScopeList } from './ScopeList'
import { FileDiff } from './FileDiff'
import { isMarkdown, Markdown } from './Markdown'
import { clamp, Splitter, useStoredWidth } from './Splitter'
import { PaneFilter, PaneHead, PaneRail, useFolded, useStoredFlag } from './Pane'
import { ScopeHead } from './ScopeHead'
import { readUrl } from '../lib/urlstate'
import { parseTerms } from '../lib/filter'
import { useScrollToActive } from '../lib/scroll'

// The diff tab, three columns: WHAT to diff (the scope list, per repo), WHICH
// files (the tree), and the diffs themselves.
//
// More than one file can be selected — cmd-click to add, shift-click for a
// range, and a click on a directory takes everything under it. The viewer then
// shows one collapsible section per file.

const VIEW_POLL_MS = 5000

// Beyond this many files a multi-selection opens collapsed: fetching sixty
// diffs nobody asked to read would make selecting a directory feel broken.
const AUTO_OPEN_MAX = 8

// how many selected paths the URL fragment carries before it keeps just one
const URL_FILES_MAX = 6

export const originLabel: Record<string, string> = {
  branch: 'committed on this branch',
  working: 'uncommitted',
  both: 'committed + uncommitted',
  staged: 'staged',
  merged: 'committed · the base branch already has this change',
}

const key = (f: DiffFile) => `${f.repo}:${f.path}`

const NOTHING_FOLDED: Set<string> = new Set()

function sameFiles(a: DiffFile[] | null, b: DiffFile[]): boolean {
  if (!a || a.length !== b.length) return false
  return a.every((f, i) => {
    const o = b[i]
    return f.path === o.path && f.repo === o.repo && f.added === o.added && f.deleted === o.deleted && f.origin === o.origin
  })
}

export function DiffTab({
  c,
  repo,
  onSel,
  reverted,
  onContext,
}: {
  c: Checkout
  repo?: string
  reverted: number
  onSel: (s: { sub?: string; scope?: string; file?: string }) => void
  // right-click on the tree: the shell owns the menu and the confirm dialog
  onContext: (e: React.MouseEvent, files: DiffFile[], repoName: string, scope: string) => void
}) {
  const wanted = useRef(readUrl())
  const [repos, setRepos] = useState<ScopeRepo[] | null>(null)
  const [sel, setSel] = useState<{ name: string; repo: string; scope: string } | null>(null)
  const [files, setFiles] = useState<DiffFile[] | null>(null)
  const [picked, setPicked] = useState<string[]>([])
  const [anchor, setAnchor] = useState<string>('')
  const [openSections, setOpenSections] = useState<Set<string>>(new Set())
  const [error, setError] = useState('')
  const [poll, setPoll] = useState(0)
  const [filter, setFilter] = useState('')
  // "ignore comments" changes what the diff asks git for (-I) and so narrows
  // every file of the scope at once, which is why it sits with the scope.
  // Remembered, because a reviewer who wants comments out wants them out all
  // session.
  const [ignoreComments, setIgnoreComments] = useState(() => localStorage.getItem('grove_ignore_comments') === '1')
  const toggleIgnore = (v: boolean) => {
    setIgnoreComments(v)
    localStorage.setItem('grove_ignore_comments', v ? '1' : '0')
  }
  // each column's filter folds under its header and stays the way it was
  // left; closing one clears its text, the switches keep their setting
  const [scopeQ, setScopeQ] = useState('')
  const [scopeFilter, setScopeFilter] = useStoredFlag('grove_scope_filter')
  const [filesFilter, setFilesFilter] = useStoredFlag('grove_files_filter')
  const toggleScopeFilter = () => {
    if (scopeFilter) setScopeQ('')
    setScopeFilter(!scopeFilter)
  }
  const toggleFilesFilter = () => {
    if (filesFilter) setFilter('')
    setFilesFilter(!filesFilter)
  }
  const scopeTerms = useMemo(() => parseTerms(scopeQ), [scopeQ])
  // the scope list can drop the commits the base branch already has: of
  // twenty commits listed, the four that are the branch are the ones looked
  // for. Remembered, like the other reading preferences.
  const [unmergedOnly, setUnmergedOnly] = useState(() => localStorage.getItem('grove_unmerged_only') === '1')
  const toggleUnmerged = (v: boolean) => {
    setUnmergedOnly(v)
    localStorage.setItem('grove_unmerged_only', v ? '1' : '0')
  }
  // A markdown file reads as its diff or rendered, before and after side by
  // side. The last choice is the default for the next file; a file switched
  // on its own keeps its own choice for the session.
  const [mdDefault, setMdDefault] = useState(() => localStorage.getItem('grove_md_rendered') === '1')
  const [mdChoice, setMdChoice] = useState<Record<string, boolean>>({})
  const rendered = (k: string) => mdChoice[k] ?? mdDefault
  const setRendered = (k: string, v: boolean) => {
    setMdChoice((prev) => ({ ...prev, [k]: v }))
    setMdDefault(v)
    localStorage.setItem('grove_md_rendered', v ? '1' : '0')
  }

  // Which checkout is on screen RIGHT NOW, for the loads below to check
  // against when they come back. Every one of them is a round trip, and
  // clicking down a column of worktrees starts several — the one that answers
  // last wins, and it is not always the one that was asked last. That is how a
  // checkout ends up showing another checkout's scope list, with a scope it
  // does not have selected: nothing failed, the answers simply landed out of
  // order. Kept in a ref rather than a dependency because the point is to read
  // it at the END, after the await, not to capture it at the start.
  const showing = useRef(c.name)
  showing.current = c.name

  const loadScopes = useCallback(
    async (initial: boolean) => {
      const r = await api.scopes(c.name)
      if (showing.current !== c.name) return // answered for a checkout since left
      setRepos(r.repos)
      if (!initial) return
      const askedSub = wanted.current.sub
      const asked = askedSub ? r.repos.find((x) => x.name === askedSub) : undefined
      const want = asked ?? r.repos.find((x) => x.name === repo) ?? r.repos[0]
      if (!want?.scopes.length) return
      const askedId = wanted.current.scope
      const askedScope = askedId ? want.scopes.find((s) => s.id === askedId) : undefined
      const first = askedScope ?? want.scopes.find((s) => s.files > 0) ?? want.scopes[0]
      setSel({ name: c.name, repo: want.name, scope: first.id })
      wanted.current = { ...wanted.current, sub: undefined, scope: undefined }
    },
    [c.name, repo],
  )

  useEffect(() => {
    let cancelled = false
    setRepos(null)
    setSel(null)
    setFiles(null)
    setPicked([])
    setError('')
    // the filter belongs to the diff being read, so a new checkout starts
    // clean — but switching scope keeps it, which is how you follow one file
    // from commit to commit
    setFilter('')
    setScopeQ('')
    loadScopes(true).catch((e) => !cancelled && setError(String(e)))
    return () => {
      cancelled = true
    }
  }, [loadScopes])

  const loadFiles = useCallback(async () => {
    if (!sel || sel.name !== c.name) return // a stale scope from the row before
    const r = await api.diffFiles(c.name, sel.repo, sel.scope, ignoreComments)
    if (showing.current !== c.name) return // and one that was current when asked
    const all = r.files || []
    setFiles((prev) => (sameFiles(prev, all) ? prev : all))
    setPicked((prev) => {
      const alive = prev.filter((k) => all.some((f) => key(f) === k))
      // keep what is open — and keep the *identity* when nothing dropped out,
      // so a background refresh is not a selection change to everything below
      if (alive.length) return alive.length === prev.length ? prev : alive
      const askedPath = wanted.current.file
      const askedFiles = askedPath ? askedPath.split(',').map((p) => all.find((f) => f.path === p)) : []
      wanted.current = { ...wanted.current, file: undefined }
      const found = askedFiles.filter(Boolean) as DiffFile[]
      if (found.length) return found.map(key)
      // the file opened on arrival is one that still needs reading, so it is
      // taken from the same list the tree will show: a file the base branch
      // already has is not a change, and a scope where every file has landed
      // opens nothing at all
      const pool =
        unmergedOnly || all.some((f) => f.merged) ? all.filter((f) => !f.merged) : all
      const first = firstLeaf(buildTree(pool))
      return first ? [key(first)] : []
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- `reverted` is a
    // trigger, not an input: a revert changed the tree this list describes
  }, [c.name, sel, ignoreComments, reverted, unmergedOnly])

  useEffect(() => {
    let cancelled = false
    loadScopes(false).catch(() => {}) // the scope counts moved too
    // clearing on success as well as setting on failure: an error box that
    // only a checkout switch can dismiss outlives its cause, and the cause is
    // usually a moment that has already passed
    loadFiles().then(
      () => !cancelled && setError(''),
      (e) => !cancelled && setError(String(e)),
    )
    return () => {
      cancelled = true
    }
  }, [loadFiles])

  // …and both on a timer, without touching the selection
  useEffect(() => {
    const t = setInterval(() => {
      loadScopes(false).catch(() => {})
      loadFiles().catch(() => {})
      setPoll((n) => n + 1)
    }, VIEW_POLL_MS)
    return () => clearInterval(t)
  }, [loadScopes, loadFiles])

  // Files the base branch already holds are not changes to read. They are in
  // this range because the range is measured from the FORK POINT — which is
  // where a squash merge leaves it, however much of the branch the base has
  // taken since — so the moment any of the work lands, the list stops being a
  // list of things to review and becomes mostly history.
  //
  // So the ones that have landed are not listed, and how many is said in words
  // above what remains. Not gated on ALL of them having landed: thirty of
  // thirty-one is the ordinary case a week after a merge, and it is the one
  // where a list of thirty-one is least worth reading. Nothing is lost — the
  // button puts them back.
  const landed = useMemo(() => (files || []).filter((f) => f.merged).length, [files])
  const [showLanded, setShowLanded] = useState(false)
  useEffect(() => setShowLanded(false), [c.name, sel?.repo, sel?.scope])
  const hideLanded = landed > 0 && !showLanded

  // "unmerged only" reaches the tree too: a committed file whose change the
  // base branch already holds is history, like a landed commit
  const listed = useMemo(
    () => (unmergedOnly || hideLanded ? (files || []).filter((f) => !f.merged) : files || []),
    [files, unmergedOnly, hideLanded],
  )
  const allLanded = !!files?.length && landed === files.length
  const full = useMemo(() => buildTree(listed), [listed])
  const terms = useMemo(() => parseTerms(filter), [filter])
  const tree = useMemo(() => filterTree(full, terms), [full, terms])
  const narrowed = terms.length > 0 || listed.length !== (files?.length ?? 0)
  const shown = useMemo(() => (terms.length ? visibleLeaves(tree, NOTHING_FOLDED).length : listed.length), [terms, tree, listed])

  // Folds live in two sets: the tree's own, and the ones made while a filter
  // is on. Both halves have to be true at once — a fold left from before must
  // not hide what you just searched for, and a fold you make while reading the
  // results is a deliberate act that has to hold. One set cannot do both: a
  // filtered tree is nothing BUT ancestors of matches, so "keep matches
  // visible" would immediately undo every fold you made. So a filter starts
  // with nothing folded, folds while it is on go to their own set, and
  // clearing it hands back the tree you left.
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())
  const [filterFolds, setFilterFolds] = useState<Set<string>>(new Set())
  const filtering = terms.length > 0
  // Shift is about what is INSIDE a directory, never the directory itself,
  // which stays open through all of it — folding it as well would hide the
  // very thing the click was asking to see.
  //
  //   open, something inside it open   →  shut everything inside
  //   open, everything inside shut     →  open everything inside
  //   shut                             →  open it and everything inside
  //
  // So one shift-click on a repo turns a tree of hundreds into a list of its
  // top folders, and the next one puts it all back. A directory with no
  // directories in it has no inside to speak of, and shift is an ordinary fold
  // there rather than a click that does nothing.
  const toggle = (n: Node, e: React.MouseEvent) => {
    const below = (x: Node): string[] =>
      x.children.filter((c) => !c.file).flatMap((c) => [c.key, ...below(c)])
    ;(filtering ? setFilterFolds : setCollapsed)((prev) => {
      const next = new Set(prev)
      const shut = prev.has(n.key)
      const inside = e.shiftKey ? below(n) : []
      if (!inside.length) {
        shut ? next.delete(n.key) : next.add(n.key)
        return next
      }
      next.delete(n.key) // whichever way this goes, you are left looking into it
      if (shut || inside.every((k) => prev.has(k))) inside.forEach((k) => next.delete(k))
      else inside.forEach((k) => next.add(k))
      return next
    })
  }

  // each filtering session starts fresh: the folds of the last one belong to a
  // result set that is gone
  useEffect(() => {
    if (!filtering) setFilterFolds(new Set())
  }, [filtering])

  const folds = filtering ? filterFolds : collapsed
  const pickedSet = useMemo(() => new Set(picked), [picked])
  // A file can only be read at the scope it was listed in, and `sel` belongs to
  // the checkout it was chosen in. For one render after switching checkouts it
  // is still the OLD one while c.name is already the new — and that is long
  // enough, because the viewer below is handed c.name and sel.scope together.
  // A file that exists in both checkouts stays picked across the switch, so it
  // gets asked for as "this checkout, at that checkout's scope": a 400, and an
  // error box that outlives the render that caused it. loadFiles has always
  // guarded this pairing; the render never did.
  const ready = !!sel && sel.name === c.name
  const selected = useMemo(
    () => (ready ? (files || []).filter((f) => pickedSet.has(key(f))) : []),
    [ready, files, pickedSet],
  )
  const scopeRepo = repos?.find((r) => r.name === sel?.repo)
  const scope = scopeRepo?.scopes.find((s) => s.id === sel?.scope)

  // A multi-selection opens its sections only while it is small. Keyed on the
  // selection's content, never its array identity: a refresh that re-picks the
  // same files must not fold the sections the user just opened.
  const pickedKey = useMemo(() => selected.map(key).sort().join('\n'), [selected])
  useEffect(() => {
    const keys = pickedKey ? pickedKey.split('\n') : []
    setOpenSections(new Set(keys.length <= AUTO_OPEN_MAX ? keys : []))
  }, [pickedKey])

  // Where a shift-range measures FROM. It stays where the last plain or
  // cmd-click put it, which is the part that makes a range re-drawable: shift,
  // look, shift again somewhere else, and you get a range from the same place
  // rather than one grown from wherever the last one happened to end.
  //
  // A selection nobody clicked for counts too. Landing on a scope opens a file,
  // and shift-clicking four rows down has to mean the four rows — being handed
  // the fourth on its own is the answer to a question nobody asked.
  //
  // An anchor from a tree that is gone is no anchor: switching scope replaces
  // every key, and folding a directory can take the anchor off screen.
  const rangeStart = (order: string[]) => {
    for (const k of [anchor, picked[0]]) if (k && order.includes(k)) return k
    return ''
  }

  const pick = (n: Node, e: React.MouseEvent) => {
    const leaves = leavesOf(n).map(key)
    if (!leaves.length) return
    const order = visibleLeaves(tree, folds).map((x) => x.key)
    const add = e.metaKey || e.ctrlKey

    if (e.shiftKey) {
      const a = order.indexOf(rangeStart(order))
      // a directory reaches to whichever of its ends is further from the
      // anchor, so shift-clicking a folder takes the whole of it either way
      const ends = [order.indexOf(leaves[0]), order.indexOf(leaves[leaves.length - 1])].filter((i) => i >= 0)
      if (a >= 0 && ends.length) {
        const b = ends.reduce((far, i) => (Math.abs(i - a) > Math.abs(far - a) ? i : far), ends[0])
        const range = order.slice(Math.min(a, b), Math.max(a, b) + 1)
        // shift with cmd adds the range to the selection instead of replacing
        // it, which is how a second run of files is added to a first
        setPicked((prev) => (add ? [...new Set([...prev, ...range])] : range))
        return // and the anchor does not move: that is the whole point of one
      }
      // nothing to measure from — fall through and behave like a plain click
    }

    if (add) {
      // add or remove, so a selection can be assembled file by file
      setPicked((prev) => {
        const set = new Set(prev)
        const allIn = leaves.every((k) => set.has(k))
        leaves.forEach((k) => (allIn ? set.delete(k) : set.add(k)))
        return [...set]
      })
    } else {
      setPicked(leaves)
    }
    setAnchor(n.file ? n.key : leaves[0])
  }

  const splitRef = useRef<HTMLDivElement>(null)
  const [scopeW, setScopeW, resetScopeW] = useStoredWidth('grove_scope_w', () => 220)
  const [treeW, setTreeW, resetTreeW] = useStoredWidth('grove_tree_w', () => 290)
  const [scopeFold, setScopeFold] = useFolded('grove_scope_fold')
  const [treeFold, setTreeFold] = useFolded('grove_tree_fold')
  const treeBox = useScrollToActive('.tw-active', picked[0] || '')

  useEffect(() => {
    // A whole directory can be selected, and putting sixty paths in the
    // address bar makes it unreadable and unshareable. Past a handful the
    // fragment keeps the first file only — enough to land in the right place.
    const paths = selected.map((f) => f.path)
    onSel({
      sub: sel?.repo,
      scope: sel?.scope,
      file: (paths.length > URL_FILES_MAX ? paths.slice(0, 1) : paths).join(',') || undefined,
    })
  }, [sel, selected, onSel])

  return (
    <div className="sb-split" ref={splitRef}>
      {scopeFold ? (
        <PaneRail label="scope" onExpand={() => setScopeFold(false)} />
      ) : (
        <>
          <div className="pane" style={{ width: scopeW }}>
            <PaneHead
              label="scope"
              onCollapse={() => setScopeFold(true)}
              filter={{
                open: scopeFilter,
                active: scopeTerms.length > 0 || unmergedOnly || ignoreComments,
                onToggle: toggleScopeFilter,
              }}
            />
            {scopeFilter && (
              <PaneFilter value={scopeQ} onChange={setScopeQ} placeholder="filter commits…">
                <label
                  className="toggle pane-toggle"
                  title="only what the base branch does not have yet: the amber and blue commits, and the files it lacks"
                >
                  <input type="checkbox" checked={unmergedOnly} onChange={(e) => toggleUnmerged(e.target.checked)} />
                  unmerged only
                </label>
                <label className="toggle pane-toggle" title="hide changes whose lines are all comments (git -I)">
                  <input type="checkbox" checked={ignoreComments} onChange={(e) => toggleIgnore(e.target.checked)} />
                  ignore comments
                </label>
              </PaneFilter>
            )}
            <div className="pane-body">
              <ScopeList
                repos={repos}
                sel={sel}
                unmergedOnly={unmergedOnly}
                terms={scopeTerms}
                onPick={(r, s) => setSel({ name: c.name, repo: r, scope: s })}
              />
            </div>
          </div>

          <Splitter
            onMove={(x) => setScopeW(clamp(x - (splitRef.current?.getBoundingClientRect().left ?? 0), 150, 460))}
            onReset={resetScopeW}
          />
        </>
      )}

      <div className="sb-right">
        {/* keyed: an expanded message belongs to ITS commit, not to the
            header — switching scope must not carry the fold state along */}
        <ScopeHead key={scope?.id} scope={scope} repo={sel?.repo} base={scopeRepo?.base} />
        <div className="sb-right-row">
      {treeFold ? (
        <PaneRail label="files" onExpand={() => setTreeFold(false)} />
      ) : (
        <>
        <div className="pane" style={{ width: treeW }}>
          <PaneHead
            label="files"
            meta={narrowed ? `${shown} / ${files?.length ?? 0}` : files?.length ?? ''}
            onCollapse={() => setTreeFold(true)}
            filter={{ open: filesFilter, active: terms.length > 0, onToggle: toggleFilesFilter }}
          />
          {filesFilter && <PaneFilter value={filter} onChange={setFilter} placeholder="filter files…" />}
          {landed > 0 && (
            <div className="landed-note">
              <p>
                {allLanded ? (
                  <>
                    All {landed} file{landed === 1 ? ' is' : 's are'} already in{' '}
                    <b>{scopeRepo?.base || 'the base branch'}</b> — this branch has nothing it lacks.
                  </>
                ) : (
                  <>
                    {landed} of {files!.length} files are already in{' '}
                    <b>{scopeRepo?.base || 'the base branch'}</b>
                    {showLanded
                      ? '.'
                      : ` — the other ${files!.length - landed} ${files!.length - landed === 1 ? 'is' : 'are'} below.`}
                  </>
                )}
              </p>
              <button className="btn-ghost" onClick={() => setShowLanded((v) => !v)}>
                {showLanded ? `Hide the ${landed} again` : `Show the ${landed} anyway`}
              </button>
            </div>
          )}
          <div className="sb-tree" ref={treeBox}>
        {files === null && <div className="empty small">loading…</div>}
        {!!terms.length && !shown && files?.length ? (
          <div className="empty small">
            no file matches
            <div className="mono" style={{ marginTop: 4 }}>
              {filter.trim()}
            </div>
            <button className="btn-ghost" style={{ marginTop: 10 }} onClick={() => setFilter('')}>
              clear
            </button>
          </div>
        ) : null}
        <TreeRows
          nodes={tree}
          depth={0}
          sel={pickedSet}
          collapsed={folds}
          terms={terms}
          onPick={pick}
          onToggle={toggle}
          onContext={(n, e) => {
            e.preventDefault() // no browser menu over the tree, whatever we do next
            const leaves = leavesOf(n)
            // right-clicking outside the selection acts on what was clicked
            const target = leaves.every((f) => pickedSet.has(key(f))) ? selected : leaves
            if (!target.length || !sel) return
            if (!leaves.every((f) => pickedSet.has(key(f)))) setPicked(leaves.map(key))
            onContext(e, target, sel.repo, sel.scope)
          }}
        />
          </div>
        </div>

        <Splitter
          onMove={(x) =>
            setTreeW(
              clamp(
                x - (splitRef.current?.getBoundingClientRect().left ?? 0) - (scopeFold ? 24 : scopeW + 7),
                140,
                720,
              ),
            )
          }
          onReset={resetTreeW}
        />
        </>
      )}

      <div className="sb-body">
        {error && <div className="error">{error}</div>}
        {!error && files?.length === 0 && (
          <div className="empty">
            nothing in this scope
            <div className="small" style={{ marginTop: 6 }}>
              {scope?.label}
              {ignoreComments && ' · comments ignored'}
            </div>
          </div>
        )}
        {!error && hideLanded && !listed.length && (
          <div className="empty">
            nothing to review
            <div className="small" style={{ marginTop: 6 }}>
              {scopeRepo?.base || 'the base branch'} already has every change this branch made
            </div>
          </div>
        )}

        {selected.map((f) => {
          const k = key(f)
          const open = openSections.has(k)
          const single = selected.length === 1
          return (
            <section key={k} className="fs">
              {/* one header per file, the same shape as the repo headlines in
                  the scope list — and the thing that folds the section */}
              <div
                className={`fs-head${open ? '' : ' fs-closed'}`}
                onClick={() =>
                  setOpenSections((prev) => {
                    const next = new Set(prev)
                    next.has(k) ? next.delete(k) : next.add(k)
                    return next
                  })
                }
                // a header is a file too: right-click it like its tree row
                onContextMenu={(e) => {
                  e.preventDefault()
                  if (sel) onContext(e, [f], sel.repo, sel.scope)
                }}
                title={f.path}
              >
                <span className="fs-caret">{open ? '▾' : '▸'}</span>
                <span
                  className={`od od-${f.merged ? 'merged' : f.origin === 'branch' ? 'branch' : 'working'}`}
                  title={f.merged ? originLabel.merged : originLabel[f.origin] ?? f.origin}
                />
                <span className="fs-path mono">{f.path}</span>
                {isMarkdown(f.path) && (
                  <span className="seg seg-mini" onClick={(e) => e.stopPropagation()}>
                    <button className={rendered(k) ? '' : 'active'} onClick={() => setRendered(k, false)}>
                      raw
                    </button>
                    <button className={rendered(k) ? 'active' : ''} onClick={() => setRendered(k, true)}>
                      rendered
                    </button>
                  </span>
                )}
                <span className="fs-stat">
                  {!!f.added && <span className="plus">+{f.added}</span>}
                  {!!f.deleted && <span className="minus">−{f.deleted}</span>}
                </span>
              </div>
              {open && sel && (
                <div className={single ? '' : 'fs-body'}>
                  {isMarkdown(f.path) && rendered(k) ? (
                    <Markdown name={c.name} repo={sel.repo} scope={sel.scope} file={f} poll={poll} />
                  ) : (
                    <FileDiff
                      name={c.name}
                      repo={sel.repo}
                      scope={sel.scope}
                      file={f}
                      ignoreComments={ignoreComments}
                      poll={poll}
                    />
                  )}
                </div>
              )}
            </section>
          )
        })}
      </div>
        </div>
      </div>
    </div>
  )
}
