// The files column's filter. It is a filter, not a search: it narrows the tree
// that is already on screen rather than asking the server anything, so it
// answers instantly and never changes what the scope holds.

// A query is whitespace-separated terms, all of which must match, in any
// order, case-insensitively, anywhere in the path. Order-independent because a
// path is remembered in pieces: "yml workflows" and "workflows release" are
// the same thought, and neither is the order the path is written in.
export function parseTerms(q: string): string[] {
  return q.toLowerCase().split(/\s+/).filter(Boolean)
}

// Matched against the whole path, not the file name, so a term can name a
// directory ("workflows") as readily as a file — and a directory that matches
// keeps everything under it without needing a rule of its own. The repo is not
// part of it: a diff list is one repo's, so its name would match every row.
export function matches(path: string, terms: string[]): boolean {
  const low = path.toLowerCase()
  return terms.every((t) => low.includes(t))
}

// marks is where the terms hit `text`, as merged [start, end) ranges. Merged
// because terms overlap — "rel" and "release" both hit the same letters, and a
// highlight drawn twice over one letter comes out as nested markup instead of
// one mark.
export function marks(text: string, terms: string[]): [number, number][] {
  const low = text.toLowerCase()
  const hits: [number, number][] = []
  for (const t of terms) {
    // from i+1, not i+t.length: a term can overlap its own next occurrence
    for (let i = low.indexOf(t); i !== -1; i = low.indexOf(t, i + 1)) hits.push([i, i + t.length])
  }
  hits.sort((a, b) => a[0] - b[0] || a[1] - b[1])
  const out: [number, number][] = []
  for (const [start, end] of hits) {
    const last = out[out.length - 1]
    if (last && start <= last[1]) last[1] = Math.max(last[1], end)
    else out.push([start, end])
  }
  return out
}
