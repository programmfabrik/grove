import { useCallback, useState } from 'react'

// Every column left of the diff can be pushed out of the way, because the diff
// is what the window is for: fold the columns you are not using and the code
// gets the room. A folded column stays on screen as a rail — a 24px strip with
// its name — so it is one click back, and never a pane you have to remember
// existed.

export function PaneHead({
  label,
  meta,
  onCollapse,
}: {
  label: string
  meta?: React.ReactNode
  onCollapse: () => void
}) {
  return (
    <div className="pane-head">
      <span>{label}</span>
      <span className="pane-head-right">
        {meta ? <span className="dim">{meta}</span> : null}
        <button className="pane-fold" onClick={onCollapse} title={`collapse ${label}`} aria-label={`collapse ${label}`}>
          ‹
        </button>
      </span>
    </div>
  )
}

export function PaneRail({ label, onExpand }: { label: string; onExpand: () => void }) {
  return (
    <div className="pane-rail" onClick={onExpand} title={`show ${label}`} role="button" tabIndex={0}>
      <span className="pane-fold">›</span>
      <span className="pane-rail-label">{label}</span>
    </div>
  )
}

// useFolded remembers a folded column across reloads, like useStoredWidth does
// for a dragged one — the layout you left is the layout you come back to.
export function useFolded(key: string): [boolean, (v: boolean) => void] {
  const [folded, setFolded] = useState(() => localStorage.getItem(key) === '1')
  const set = useCallback(
    (v: boolean) => {
      setFolded(v)
      if (v) localStorage.setItem(key, '1')
      else localStorage.removeItem(key)
    },
    [key],
  )
  return [folded, set]
}
