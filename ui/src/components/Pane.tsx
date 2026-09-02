import { useCallback, useRef, useState, type ReactNode } from 'react'
import { TOKEN_COLOURS } from './Hits'

// Every column left of the diff can be pushed out of the way, because the diff
// is what the window is for: fold the columns you are not using and the code
// gets the room. A folded column stays on screen as a rail — a 24px strip with
// its name — so it is one click back, and never a pane you have to remember
// existed.
//
// Each column also carries its own filter, folded under its header: a click on
// the header opens it, another closes it, and whether it is open is remembered
// the way the widths and the folds are.

export type PaneFilterState = {
  open: boolean
  active?: boolean // the filter is narrowing the list right now
  onToggle: () => void
}

export function PaneHead({
  label,
  meta,
  onCollapse,
  filter,
}: {
  label: string
  meta?: ReactNode
  onCollapse: () => void
  filter?: PaneFilterState
}) {
  return (
    <div
      className={filter ? 'pane-head pane-head-filterable' : 'pane-head'}
      onClick={filter?.onToggle}
      title={filter ? (filter.open ? `hide the ${label} filter` : `filter ${label}`) : undefined}
    >
      <span className="pane-head-label">
        {label}
        {filter && (
          <svg
            className={filter.open || filter.active ? 'pane-head-search on' : 'pane-head-search'}
            viewBox="0 0 16 16"
            aria-hidden="true"
          >
            <circle cx="6.5" cy="6.5" r="4.5" fill="none" stroke="currentColor" strokeWidth="1.7" />
            <path d="M10 10l4 4" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" />
          </svg>
        )}
      </span>
      <span className="pane-head-right">
        {meta ? <span className="dim">{meta}</span> : null}
        <button
          className="pane-fold"
          onClick={(e) => {
            e.stopPropagation() // folding the column is not toggling its filter
            onCollapse()
          }}
          title={`collapse ${label}`}
          aria-label={`collapse ${label}`}
        >
          ‹
        </button>
      </span>
    </div>
  )
}

// PaneFilter is the slideout under a header: a text filter, and whatever
// switches narrow the same list. A filter, not a search — it narrows what is
// already on screen and never asks the server anything.
//
// The terms are shown in colour, one per term, the same colours the lists
// mark their hits in. An input cannot colour its own text, so a mirror of the
// value sits under a transparent one and scrolls with it.
export function PaneFilter({
  value,
  onChange,
  placeholder,
  children,
}: {
  value: string
  onChange: (v: string) => void
  placeholder: string
  children?: ReactNode
}) {
  const mirror = useRef<HTMLDivElement>(null)
  let n = 0
  const tokens = value.split(/(\s+)/).map((part, i) =>
    /^\s*$/.test(part) ? (
      part
    ) : (
      <span key={i} className={`tok tok-${n++ % TOKEN_COLOURS}`}>
        {part}
      </span>
    ),
  )
  return (
    <div className="pane-filter">
      <div className="pane-filter-row">
        <div className="pane-filter-box">
          <input
            className="pane-filter-input"
            autoFocus
            placeholder={placeholder}
            value={value}
            spellCheck={false}
            onChange={(e) => onChange(e.target.value)}
            onScroll={(e) => {
              if (mirror.current) mirror.current.scrollLeft = e.currentTarget.scrollLeft
            }}
            onKeyDown={(e) => {
              if (e.key === 'Escape') onChange('')
            }}
          />
          <div className="pane-filter-mirror" aria-hidden="true" ref={mirror}>
            {tokens}
          </div>
        </div>
        {!!value && (
          <button className="pane-filter-clear" onClick={() => onChange('')} title="clear (esc)">
            ×
          </button>
        )}
      </div>
      {children && <div className="pane-filter-opts">{children}</div>}
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

// useStoredFlag remembers a yes/no across reloads — a folded column, an open
// filter — like useStoredWidth does for a dragged separator: the layout you
// left is the layout you come back to.
export function useStoredFlag(key: string): [boolean, (v: boolean) => void] {
  const [on, setOn] = useState(() => localStorage.getItem(key) === '1')
  const set = useCallback(
    (v: boolean) => {
      setOn(v)
      if (v) localStorage.setItem(key, '1')
      else localStorage.removeItem(key)
    },
    [key],
  )
  return [on, set]
}

export const useFolded = useStoredFlag
