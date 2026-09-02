import { useCallback, useState } from 'react'

// A draggable vertical separator. It reports the pointer's x while dragging
// and lets the caller decide what that means for its pane, so the same handle
// works for a pane that grows to the left (the sidebar) and one that grows to
// the right (the file tree).
//
// The window listeners are attached in the pointerdown handler itself, not in
// an effect: an effect only runs after the next render, which drops the first
// frames of the drag.
export function Splitter({ onMove, onReset }: { onMove: (clientX: number) => void; onReset?: () => void }) {
  const [dragging, setDragging] = useState(false)

  const start = (e: React.PointerEvent) => {
    e.preventDefault()
    setDragging(true)
    const move = (ev: PointerEvent) => {
      ev.preventDefault() // a drag must not select the text it passes over
      onMove(ev.clientX)
    }
    const stop = () => {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', stop)
      // the cursor has to survive leaving the 7px handle mid-drag
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
      setDragging(false)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', stop)
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
  }

  return (
    <div
      className={dragging ? 'splitter dragging' : 'splitter'}
      onPointerDown={start}
      onDoubleClick={onReset}
      title="drag to resize · double-click to reset"
      role="separator"
      aria-orientation="vertical"
    />
  )
}

// useStoredWidth keeps a pane's width across reloads — a layout you dragged
// into place should still be there tomorrow.
export function useStoredWidth(key: string, fallback: () => number) {
  const [width, setWidth] = useState(() => {
    const stored = Number(localStorage.getItem(key))
    return stored > 0 ? stored : fallback()
  })
  const set = useCallback(
    (w: number) => {
      setWidth(w)
      localStorage.setItem(key, String(Math.round(w)))
    },
    [key],
  )
  const reset = useCallback(() => {
    localStorage.removeItem(key)
    setWidth(fallback())
  }, [key, fallback])
  return [width, set, reset] as const
}

export const clamp = (v: number, lo: number, hi: number) => Math.min(Math.max(v, lo), hi)
