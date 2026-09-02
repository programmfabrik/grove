// The view lives in the URL fragment, so F5 comes back to where you were and a
// link to "that file in that scope of that worktree" is copyable.
//
// One writer: App owns the fragment and writes it whole from its state, and the
// panes read their opening values from it once at mount. Nobody merges, so no
// two writers can race a half-updated fragment into place.
//
//   #repo=myrepo&wt=myrepo3&tab=diff&sub=myrepo&scope=base&file=cmd/main.go

export type UrlState = {
  repo?: string // repository NAME, not its path — shorter and stable
  wt?: string // worktree (checkout) name
  tab?: string // diff | worktree
  sub?: string // sub-repo inside the checkout
  scope?: string
  file?: string
}

const KEYS: (keyof UrlState)[] = ['repo', 'wt', 'tab', 'sub', 'scope', 'file']

export function readUrl(): UrlState {
  const p = new URLSearchParams(location.hash.replace(/^#/, ''))
  const out: UrlState = {}
  for (const k of KEYS) {
    const v = p.get(k)
    if (v) out[k] = v
  }
  return out
}

// writeUrl replaces the fragment. replaceState, not pushState: selecting a file
// is navigation within one view, and a back button that walks every click of a
// session is worse than none.
export function writeUrl(s: UrlState) {
  const p = new URLSearchParams()
  for (const k of KEYS) {
    const v = s[k]
    if (v) p.set(k, v)
  }
  const hash = p.toString()
  const url = `${location.pathname}${location.search}${hash ? '#' + hash : ''}`
  if (url !== location.pathname + location.search + location.hash) {
    history.replaceState(null, '', url)
  }
}
