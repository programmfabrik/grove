import { useEffect, useRef } from 'react'

// After a reload restores a deep selection, the selected row is usually below
// the fold — a repo 40 rows down, a file in a 135-file tree. Each list scrolls
// its own active row into view once the row exists.
//
// `block: 'nearest'` is what makes this safe to run on every selection change:
// a row already on screen is left exactly where it is, so clicking around
// never yanks the list.
//
// The key must change when either the selection OR the data does — on a
// restore the selection is set as the data arrives, and the effect has to run
// on the render where the row is finally in the DOM.
export function useScrollToActive<T extends HTMLElement = HTMLDivElement>(selector: string, key: unknown) {
  const ref = useRef<T>(null)
  // once per selection: a background refresh changes the key (the row count
  // moves) without changing what is selected, and scrolling then would yank a
  // list the user had deliberately scrolled elsewhere
  const done = useRef<unknown>(null)
  useEffect(() => {
    const el = ref.current?.querySelector(selector)
    if (!el || done.current === key) return
    done.current = key
    el.scrollIntoView({ block: 'nearest' })
  }, [selector, key])
  return ref
}
