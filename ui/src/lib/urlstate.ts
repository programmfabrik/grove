// The view lives in the URL fragment, so F5 comes back to where you were and a
// link to "that file in that scope of that worktree" is copyable.
//
// One writer: App owns the fragment and writes it whole from its state, and the
// panes read their opening values from it once at mount. Nobody merges, so no
// two writers can race a half-updated fragment into place.
//
//   #repo=myrepo&wt=myrepo3&sub=myrepo&scope=base&file=cmd/main.go

export type UrlState = {
  repo?: string // repository NAME, not its path — shorter and stable
  wt?: string // worktree (checkout) name
  sub?: string // sub-repo inside the checkout
  scope?: string
  file?: string
}

const KEYS: (keyof UrlState)[] = ['repo', 'wt', 'sub', 'scope', 'file']

export function readUrl(): UrlState {
  const p = new URLSearchParams(location.hash.replace(/^#/, ''))
  const out: UrlState = {}
  for (const k of KEYS) {
    const v = p.get(k)
    if (v) out[k] = v
  }
  return out
}

// fragmentFor renders a view as the fragment that names it. The keys it does
// not know are dropped, so a state built by spreading another one over a
// partial cannot smuggle anything through.
export function fragmentFor(s: UrlState): string {
  const p = new URLSearchParams()
  for (const k of KEYS) {
    const v = s[k]
    if (v) p.set(k, v)
  }
  return p.toString()
}

// writeUrl replaces the fragment. replaceState, not pushState: selecting a file
// is navigation within one view, and a back button that walks every click of a
// session is worse than none.
export function writeUrl(s: UrlState) {
  const hash = fragmentFor(s)
  const url = `${location.pathname}${location.search}${hash ? '#' + hash : ''}`
  if (url !== location.pathname + location.search + location.hash) {
    history.replaceState(null, '', url)
  }
}
