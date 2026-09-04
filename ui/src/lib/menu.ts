import { useEffect, useLayoutEffect, useRef, useState } from 'react'

// What every context menu in grove needs and neither of them should own: a way
// to be dismissed, and a way to fit on the screen.

// useDismiss closes the menu on the next click anywhere, on another right-click,
// and on Escape.
export function useDismiss(onClose: () => void) {
  useEffect(() => {
    const away = () => onClose()
    const esc = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    // Registered a tick late, on purpose. React flushes a discrete event's
    // state update while that same event is still bubbling, so listeners added
    // straight away would see the very right-click that opened the menu and
    // close it again — the menu would never appear at all.
    const t = setTimeout(() => {
      window.addEventListener('click', away)
      window.addEventListener('contextmenu', away)
    }, 0)
    window.addEventListener('keydown', esc)
    return () => {
      clearTimeout(t)
      window.removeEventListener('click', away)
      window.removeEventListener('contextmenu', away)
      window.removeEventListener('keydown', esc)
    }
  }, [onClose])
}

// useMenuAt places the menu at the pointer, and then pulls it back onto the
// screen. A menu is position: fixed, so whatever hangs off the edge is simply
// gone — grove is a window, not a page somebody can scroll to reach it — and
// the last row of a full column is exactly where you right-click.
const MARGIN = 8

export function useMenuAt(x: number, y: number) {
  const ref = useRef<HTMLDivElement>(null)
  const [at, setAt] = useState({ left: x, top: y })
  // layout, not effect: the menu must never be painted at the wrong place first
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const { width, height } = el.getBoundingClientRect()
    setAt({
      left: Math.max(MARGIN, Math.min(x, window.innerWidth - width - MARGIN)),
      top: Math.max(MARGIN, Math.min(y, window.innerHeight - height - MARGIN)),
    })
  }, [x, y])
  return { ref, at }
}
