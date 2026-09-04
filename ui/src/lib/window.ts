import { api } from '../api'
import { fragmentFor, readUrl, type UrlState } from './urlstate'

// The same page, at a different place in it, in a window of its own. The view
// already lives in the fragment — that is what makes a deep link work — so a
// second window is nothing more than the same URL with a different one.

// Which front door served this page. The app's window has no tabs and no
// address bar, and its webview ignores window.open; a browser has no native
// windows to ask for. So the two cases are genuinely different code, and this
// is the one fact that tells them apart. App learns it from api.version() on
// mount, which is long before anybody can right-click anything.
let native = false

export function setNativeWindows(on: boolean) {
  native = on
}

// where returns the view a row stands for: what the row knows, over the parts
// of the current view it belongs to. A worktree row knows its own name and not
// which repository it is a worktree of; that is the column it is sitting in.
export function where(part: UrlState): UrlState {
  return { ...readUrl(), ...part }
}

export function openInWindow(view: UrlState, title: string) {
  const frag = fragmentFor(view)
  if (!native) {
    // named, so asking for the same view twice raises the window that already
    // has it rather than stacking another one behind it
    window.open(`${location.pathname}${location.search}#${frag}`, `grove:${frag}`)
    return
  }
  api.window(frag, title).catch(() => {})
}
