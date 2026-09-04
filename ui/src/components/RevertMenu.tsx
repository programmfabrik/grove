import type { DiffFile } from '../types'
import { useDismiss, useMenuAt } from '../lib/menu'

// The tree's context menu, and the confirmation in front of the one thing
// grove can destroy. The offered actions follow git's own model, the way
// GitHub Desktop presents it:
//
//   staged            → Unstage            (the index goes back to HEAD)
//   uncommitted       → Discard changes    (the working tree goes back to the
//                                           index; an untracked file has no
//                                           index entry, so it is deleted)
//   committed only    → nothing            (undoing that is a rebase, not a
//                                           file operation)
//
// A mixed selection offers each action for the files it applies to, and the
// dialog names those files before anything happens.

export type MenuState = { x: number; y: number; files: DiffFile[]; repo: string; scope: string }
export type PendingRevert = { action: 'unstage' | 'discard'; files: DiffFile[]; repo: string }

export function stageable(files: DiffFile[]): DiffFile[] {
  return files.filter((f) => f.origin === 'staged')
}

export function discardable(files: DiffFile[]): DiffFile[] {
  return files.filter((f) => f.origin === 'working' || f.origin === 'both')
}

export function ContextMenu({
  menu,
  onPick,
  onEdit,
  onClose,
}: {
  menu: MenuState
  onPick: (p: PendingRevert) => void
  onEdit?: (f: DiffFile) => void
  onClose: () => void
}) {
  useDismiss(onClose)
  const { ref, at } = useMenuAt(menu.x, menu.y)

  const toUnstage = stageable(menu.files)
  const toDiscard = discardable(menu.files)
  const n = (list: DiffFile[]) => (list.length === 1 ? '1 file' : `${list.length} files`)

  return (
    <div
      ref={ref}
      className="ctx"
      style={at}
      onClick={(e) => e.stopPropagation()}
      onContextMenu={(e) => e.preventDefault()}
    >
      <div className="ctx-head">{menu.files.length === 1 ? menu.files[0].path.split('/').pop() : n(menu.files)}</div>
      {/* the verb grove was missing: you read a change and then want to be in
          the file. One file at a time, since an editor opening sixty is not a
          thing anybody meant to ask for — and not here at all when there is no
          editor to open it in. */}
      {onEdit && (
        <>
          <button
            className="ctx-item"
            disabled={menu.files.length !== 1}
            onClick={() => onEdit(menu.files[0])}
          >
            Open in editor
            {menu.files.length > 1 && <span className="dim"> · one at a time</span>}
          </button>
          <div className="ctx-sep" />
        </>
      )}
      <button
        className="ctx-item"
        disabled={!toUnstage.length}
        onClick={() => onPick({ action: 'unstage', files: toUnstage, repo: menu.repo })}
      >
        Unstage
        {!!toUnstage.length && menu.files.length !== toUnstage.length && <span className="dim"> · {n(toUnstage)}</span>}
      </button>
      <button
        className="ctx-item ctx-danger"
        disabled={!toDiscard.length}
        onClick={() => onPick({ action: 'discard', files: toDiscard, repo: menu.repo })}
      >
        Discard changes
        {!!toDiscard.length && menu.files.length !== toDiscard.length && <span className="dim"> · {n(toDiscard)}</span>}
      </button>
      {!toUnstage.length && !toDiscard.length && (
        <div className="ctx-note dim">committed — undo it with a rebase, not here</div>
      )}
    </div>
  )
}

export function RevertDialog({
  pending,
  busy,
  onConfirm,
  onCancel,
}: {
  pending: PendingRevert
  busy: boolean
  onConfirm: () => void
  onCancel: () => void
}) {
  const deletes = pending.action === 'discard' ? pending.files.filter((f) => f.untracked) : []
  const restores = pending.files.filter((f) => !deletes.includes(f))
  return (
    <div className="modal-backdrop" onClick={onCancel}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h2 className="modal-title">{pending.action === 'unstage' ? 'Unstage' : 'Discard changes'}</h2>
        <div className="modal-body">
          {pending.action === 'unstage' ? (
            <p>
              {restores.length === 1 ? 'This file goes' : `These ${restores.length} files go`} back to what HEAD has in
              the index. The working tree is not touched.
            </p>
          ) : (
            <>
              {!!restores.length && (
                <p>
                  {restores.length === 1 ? 'This file loses' : `These ${restores.length} files lose`} every uncommitted
                  change — the working tree goes back to the index.
                </p>
              )}
              {!!deletes.length && (
                <p className="modal-warn">
                  {deletes.length === 1 ? '1 untracked file is DELETED' : `${deletes.length} untracked files are DELETED`}
                  . They are in no commit and no index, so git cannot bring them back.
                </p>
              )}
            </>
          )}
          <ul className="modal-list mono">
            {pending.files.slice(0, 12).map((f) => (
              <li key={f.path} className={deletes.includes(f) ? 'minus' : undefined}>
                {f.path}
                {deletes.includes(f) && ' — delete'}
              </li>
            ))}
            {pending.files.length > 12 && <li className="dim">…and {pending.files.length - 12} more</li>}
          </ul>
        </div>
        <div className="modal-actions">
          <button className="btn-ghost" onClick={onCancel} disabled={busy}>
            Cancel
          </button>
          <button
            className={pending.action === 'discard' ? 'btn-danger' : 'btn'}
            onClick={onConfirm}
            disabled={busy}
            autoFocus
          >
            {busy ? '…' : pending.action === 'unstage' ? 'Unstage' : 'Discard'}
          </button>
        </div>
      </div>
    </div>
  )
}
