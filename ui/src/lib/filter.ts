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

// matchesFields is the same rule over a row with several texts — a name, a
// branch, a subject: every term has to hit one of them, whichever.
export function matchesFields(terms: string[], fields: (string | undefined)[]): boolean {
  const lows = fields.filter((f): f is string => !!f).map((f) => f.toLowerCase())
  return terms.every((t) => lows.some((f) => f.includes(t)))
}

// tokenRuns cuts `text` into runs, each owned by the term that hit it or by
// none, so a label can colour every term differently. Terms overlap — "rel"
// and "release" hit the same letters — and the earlier term keeps the letters
// it reached first, so no letter is claimed twice.
export function tokenRuns(text: string, terms: string[]): { text: string; term: number }[] {
  const low = text.toLowerCase()
  const owner = new Int16Array(text.length).fill(-1)
  terms.forEach((t, ti) => {
    if (!t) return
    for (let i = low.indexOf(t); i !== -1; i = low.indexOf(t, i + 1)) {
      for (let k = i; k < i + t.length; k++) if (owner[k] === -1) owner[k] = ti
    }
  })
  const out: { text: string; term: number }[] = []
  let from = 0
  for (let i = 1; i <= text.length; i++) {
    if (i === text.length || owner[i] !== owner[from]) {
      out.push({ text: text.slice(from, i), term: owner[from] })
      from = i
    }
  }
  return out
}
