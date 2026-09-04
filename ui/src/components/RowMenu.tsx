import { useDismiss, useMenuAt } from '../lib/menu'
import { openInWindow } from '../lib/window'
import type { UrlState } from '../lib/urlstate'

// The right-click on a row of any of the three columns. A repository, a
// worktree and a scope are each a whole view of their own — the fragment can
// name all three — so each of them can be put in a window and left there while
// the work goes on somewhere else. Comparing two branches, or watching one
// checkout's diff while committing in another, is two windows or it is nothing.

export type RowMenuState = {
  x: number
  y: number
  label: string // what the row is, as its head
  view: UrlState // where the new window opens
  title: string // and what its title bar says
}

export function RowMenu({ menu, onClose }: { menu: RowMenuState; onClose: () => void }) {
  useDismiss(onClose)
  const { ref, at } = useMenuAt(menu.x, menu.y)
  return (
    <div
      ref={ref}
      className="ctx"
      style={at}
      onClick={(e) => e.stopPropagation()}
      onContextMenu={(e) => e.preventDefault()}
    >
      <div className="ctx-head">{menu.label}</div>
      <button
        className="ctx-item"
        onClick={() => {
          openInWindow(menu.view, menu.title)
          onClose()
        }}
      >
        Open in new window
      </button>
    </div>
  )
}

// onRowMenu is the handler every row shares: swallow the browser's own menu,
// and open ours where the pointer is.
export function onRowMenu(open: (m: RowMenuState) => void, m: Omit<RowMenuState, 'x' | 'y'>) {
  return (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    open({ ...m, x: e.clientX, y: e.clientY })
  }
}
