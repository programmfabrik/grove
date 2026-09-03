import { useState } from 'react'
import { api } from '../api'
import type { Checkout, DiffFile } from '../types'
import { DiffTab } from './DiffTab'
import { Copy } from './ui'
import { fmtDateTime } from '../lib/format'
import { ContextMenu, RevertDialog, type MenuState, type PendingRevert } from './RevertMenu'
import { RemoteBar } from './RemoteBar'

// The sidebar is the one detail surface. Its head is everything known about
// the checkout — name, branch, distance from the base, path, head commit —
// and the diff sits under it. What the diff shows has its own header over the
// columns that show it; this one never moves under it.
export function Sidebar({
  c,
  base,
  repo,
  terminal = true,
  editor = true,
  onDiffSel,
}: {
  c: Checkout
  base: string
  repo?: string
  // an offer somebody has turned off is not a disabled button, it is no
  // button: a control that is only ever declined is clutter with a tooltip
  terminal?: boolean
  editor?: boolean
  onDiffSel: (s: { sub?: string; scope?: string; file?: string }) => void
}) {
  // the tree's context menu and the confirmation in front of a revert live
  // here: they overlay the whole viewer, not one column of it
  const [menu, setMenu] = useState<MenuState | null>(null)
  const [pending, setPending] = useState<PendingRevert | null>(null)
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState('')
  // bumped after a successful revert: the tree it changed must not wait for
  // the next poll to notice
  const [reverted, setReverted] = useState(0)

  return (
    <>
      <div className="sb-head">
        <div className="sb-pick">
          <div className="sb-title">
            <span className="mono">{c.name}</span>
            <span className="sb-branch mono dim">{c.detached ? 'detached' : c.branch}</span>
          </div>
          <div className="sb-sub dim">
            <span className={c.ahead ? 'on ahead' : ''}>
              {c.ahead} ahead of {base}
            </span>
            {' · '}
            <span className={c.behind ? 'on behind' : ''}>{c.behind} behind</span>
            {' · '}
            <span className={c.dirty ? 'on dirty' : ''}>{c.dirty} uncommitted</span>
          </div>
        </div>
        <div className="sb-tools">
          {/* the two verbs grove was missing: you look at a worktree and then
              want to be in it */}
          {terminal && (
            <button
              className="btn-ghost"
              title="Open this checkout in your terminal"
              onClick={() => api.launch({ kind: 'terminal', name: c.name }).catch(() => {})}
            >
              Terminal
            </button>
          )}
          {editor && (
            <button
              className="btn-ghost"
              title="Open this checkout in your editor"
              onClick={() => api.launch({ kind: 'editor', name: c.name }).catch(() => {})}
            >
              Editor
            </button>
          )}
          <RemoteBar name={c.name} onChanged={() => setReverted((n) => n + 1)} />
        </div>
        <div className="sb-facts dim">
          <Copy text={c.path} title="Copy path">
            <span className="mono">{c.path}</span>
          </Copy>
          <div className="sb-fact" title={c.head.subject}>
            <span className="mono">{c.head.hash}</span> · {c.head.author} · {fmtDateTime(c.head.date)} ·{' '}
            {c.head.subject}
          </div>
        </div>
      </div>

      <DiffTab
        c={c}
        repo={repo}
        onSel={onDiffSel}
        reverted={reverted}
        onContext={(e, files, repoName, scope) => {
          e.preventDefault()
          setMenu({ x: e.clientX, y: e.clientY, files, repo: repoName, scope })
        }}
      />

      {menu && (
        <ContextMenu
          onEdit={
            editor
              ? (f) =>
                  api
                    .launch({ kind: 'editor', name: c.name, repo: menu.repo, file: f.path })
                    .catch(() => {})
              : undefined
          }
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
