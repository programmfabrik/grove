import { useState } from 'react'
import { api } from '../api'
import type { Checkout, DiffFile, PanelTab } from '../types'
import { DiffTab } from './DiffTab'
import { WorktreeTab } from './WorktreeTab'
import { ContextMenu, RevertDialog, type MenuState, type PendingRevert } from './RevertMenu'

// The sidebar is the one detail surface: a row click opens it instead of
// expanding underneath, and its tabs hold what used to be two separate places
// — the diff and everything else known about the checkout.
export function Sidebar({
  c,
  base,
  repo,
  tab,
  onTab,
  onDiffSel,
}: {
  c: Checkout
  base: string
  repo?: string
  tab: PanelTab
  onTab: (t: PanelTab) => void
  onDiffSel: (s: { sub?: string; scope?: string; file?: string }) => void
}) {
  // "ignore comments" belongs to the head but changes what the diff tab asks
  // for, so the state lives here and rides down as a prop. Remembered, because
  // a reviewer who wants comments out usually wants them out all session.
  const [ignoreComments, setIgnoreComments] = useState(
    () => localStorage.getItem('grove_ignore_comments') === '1',
  )
  const toggleIgnore = (v: boolean) => {
    setIgnoreComments(v)
    localStorage.setItem('grove_ignore_comments', v ? '1' : '0')
  }

  // the tree's context menu and the confirmation in front of a revert live
  // here: they overlay the whole viewer, not one column of it
  const [menu, setMenu] = useState<MenuState | null>(null)
  const [pending, setPending] = useState<PendingRevert | null>(null)
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState('')
  // bumped after a successful revert: the tree it changed must not wait for
  // the next poll to notice
  const [reverted, setReverted] = useState(0)

  const diff = tab === 'diff'
  return (
    <>
      <div className="sb-head">
        {/* the checkout, on both tabs. What the diff shows has its own header
            over the columns that show it — this one never moved under it. */}
        <div className="sb-pick">
          <div className="sb-title mono">{c.name}</div>
          <div className="sb-sub dim">
            <span className="mono">{c.branch || 'detached'}</span>
            {c.ticket && <> · #{c.ticket}</>}
            {!!c.dirty && <> · {c.dirty} uncommitted</>}
          </div>
        </div>
        <div className="sb-modes">
          {diff && (
            <label className="toggle" title="hide changes whose lines are all comments (git -I)">
              <input
                type="checkbox"
                checked={ignoreComments}
                onChange={(e) => toggleIgnore(e.target.checked)}
              />
              ignore comments
            </label>
          )}
          <div className="seg">
            <button className={diff ? 'active' : ''} onClick={() => onTab('diff')}>
              diff
            </button>
            <button className={!diff ? 'active' : ''} onClick={() => onTab('worktree')}>
              worktree
            </button>
          </div>
        </div>
      </div>

      {diff ? (
        <DiffTab
          c={c}
          repo={repo}
          ignoreComments={ignoreComments}
          onSel={onDiffSel}
          reverted={reverted}
          onContext={(e, files, repoName, scope) => {
            e.preventDefault()
            setMenu({ x: e.clientX, y: e.clientY, files, repo: repoName, scope })
          }}
        />
      ) : (
        <WorktreeTab c={c} base={base} />
      )}

      {menu && (
        <ContextMenu
          menu={menu}
          onClose={() => setMenu(null)}
          onPick={(p) => {
            setMenu(null)
            setFailed('')
            setPending(p)
          }}
        />
      )}
      {pending && (
        <RevertDialog
          pending={pending}
          busy={busy}
          onCancel={() => setPending(null)}
          onConfirm={async () => {
            setBusy(true)
            try {
              await api.revert({
                name: c.name,
                repo: pending.repo,
                action: pending.action,
                paths: pending.files.filter((f: DiffFile) => !f.untracked).map((f: DiffFile) => f.path),
                untracked: pending.files.filter((f: DiffFile) => f.untracked).map((f: DiffFile) => f.path),
              })
              setPending(null)
              setReverted((n) => n + 1)
            } catch (err) {
              setFailed(String(err))
              setPending(null)
            } finally {
              setBusy(false)
            }
          }}
        />
      )}
      {failed && <div className="error error-bar">{failed}</div>}
    </>
  )
}
