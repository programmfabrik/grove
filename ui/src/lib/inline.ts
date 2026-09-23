// Where a replaced line actually changed.
//
// A diff says a line went and another came, and leaves the reader to spot the
// difference. Usually that is a word, and easy. Sometimes it is a trailing
// space, a tab where spaces were, a Windows line ending, a no-break space
// pasted in from a document — and then the two lines look identical and the
// diff looks broken.
//
// So a removed line is paired with the added line that replaced it, the part
// between their common start and their common end is marked, and inside that
// part, and only there, whitespace is drawn. Everywhere else it stays
// whitespace: a file full of dots is unreadable, and nothing outside the mark
// changed anyway.

// Mark is [from, to) in UTF-16 units of a line's text.
export type Mark = { from: number; to: number }

type DiffLine = { kind: string; text: string }

// how far down the added run a removed line looks for the line that replaced it
const LOOKAHEAD = 16

// pairMarks finds, for every replaced line, the part of it that changed — keyed
// by the line's index. A run of removed lines followed at once by a run of added
// lines is one replacement, and pairing never reaches across a context line, so
// a line is never compared with an unrelated one that merely sits nearby.
//
// Within a replacement the lines are matched in order, but not strictly one to
// one: the removed line takes the first added line it is recognisably an edit
// of, a little way ahead. Matching the i-th with the i-th went wrong the moment
// a line was INSERTED among the edits — add one field to a struct, gofmt
// realigns the rest, and every field after the new one was compared with its
// neighbour, found to be a rewrite, and left unmarked.
export function pairMarks(lines: DiffLine[]): Map<number, Mark> {
  const marks = new Map<number, Mark>()
  let i = 0
  while (i < lines.length) {
    if (lines[i].kind !== 'del') {
      i++
      continue
    }
    const dels: number[] = []
    while (i < lines.length && lines[i].kind === 'del') dels.push(i++)
    const adds: number[] = []
    while (i < lines.length && lines[i].kind === 'add') adds.push(i++)
    let next = 0 // the first added line not yet taken
    for (const d of dels) {
      for (let k = next; k < Math.min(adds.length, next + LOOKAHEAD); k++) {
        const pair = changedPart(lines[d].text, lines[adds[k]].text)
        if (!pair) continue
        marks.set(d, pair[0])
        marks.set(adds[k], pair[1])
        next = k + 1
        break
      }
      // a removed line with no edit of it ahead is simply gone, and takes no
      // added line with it
    }
  }
  return marks
}

// changedPart is what lies between the longest common start and the longest
// common end of two lines. A pair that shares less than half of the longer line
// is a rewrite, not an edit: it is already all red and all green, and marking
// most of it a second time would say nothing.
export function changedPart(a: string, b: string): [Mark, Mark] | null {
  const shortest = Math.min(a.length, b.length)
  let start = 0
  while (start < shortest && a.charCodeAt(start) === b.charCodeAt(start)) start++
  let end = 0
  while (end < shortest - start && a.charCodeAt(a.length - 1 - end) === b.charCodeAt(b.length - 1 - end)) end++
  if ((start + end) * 2 < Math.max(a.length, b.length)) return null
  // never cut a character in two: an emoji is two code units, and a change in
  // its second half is a change to the whole of it
  if (start > 0 && isHigh(a.charCodeAt(start - 1))) start--
  if (end > 0 && isLow(a.charCodeAt(a.length - end))) end--
  return [
    { from: start, to: a.length - end },
    { from: start, to: b.length - end },
  ]
}

const isHigh = (c: number) => c >= 0xd800 && c <= 0xdbff
const isLow = (c: number) => c >= 0xdc00 && c <= 0xdfff

// What an invisible character looks like when it is the thing that changed.
// The first two are the conventions every editor uses. The rest are rare enough
// to deserve their name on hover, since "a space that is not a space" is not
// something anybody guesses.
const named: Record<string, [string, string?]> = {
  ' ': ['·'],
  '\t': ['→', 'tab'],
  '\r': ['␍', 'carriage return — a Windows line ending'],
  '\u00a0': ['⍽', 'U+00A0 no-break space'],
  '\u200b': ['◦', 'U+200B zero-width space'],
  '\ufeff': ['◦', 'U+FEFF byte-order mark'],
}
// every other space and invisible formatting character in the tables
const otherInvisible = /[\u2000-\u200f\u2028-\u202f\u205f-\u2064\u3000]/

function glyph(ch: string): string | null {
  const g = named[ch]
  if (g) return g[1] ? `<span class="ws" title="${g[1]}">${g[0]}</span>` : `<span class="ws">${g[0]}</span>`
  if (otherInvisible.test(ch)) {
    const code = ch.charCodeAt(0).toString(16).toUpperCase().padStart(4, '0')
    return `<span class="ws" title="U+${code}">◦</span>`
  }
  return null
}

export function escapeHtml(text: string): string {
  return text.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`)
}

// markHtml marks [from, to) of a line's TEXT inside its HTML — either the
// highlighter's markup or plain escaped text — and draws the whitespace inside
// the mark. It walks the markup the way splitLines does: a tag passes through,
// an entity counts as the one character it stands for. The mark is closed in
// front of every tag it meets and reopened behind it, so it never straddles the
// highlighter's own spans and the result is always well nested.
//
// Null when the markup's text is not the line's text — the highlighted file and
// the diff are read separately, and a line that does not add up is shown plain
// with the mark rather than coloured with the mark in the wrong place.
export function markHtml(html: string, mark: Mark, length: number): string | null {
  let out = ''
  let n = 0 // text characters seen
  let open = false
  const inside = () => n >= mark.from && n < mark.to
  const openIf = () => {
    if (!open && inside()) {
      out += '<span class="dl-chg">'
      open = true
    }
  }
  const close = () => {
    if (open) {
      out += '</span>'
      open = false
    }
  }
  let i = 0
  while (i < html.length) {
    const c = html[i]
    if (c === '<') {
      const gt = html.indexOf('>', i)
      if (gt < 0) return null
      close()
      out += html.slice(i, gt + 1)
      i = gt + 1
      continue
    }
    let ch = c
    if (c === '&') {
      const semi = html.indexOf(';', i)
      if (semi < 0) return null
      ch = html.slice(i, semi + 1)
    } else if (isHigh(c.charCodeAt(0)) && i + 1 < html.length) {
      ch = html.slice(i, i + 2) // one character, two code units
    }
    if (inside()) {
      openIf()
      out += (ch.length === 1 && glyph(ch)) || ch
    } else {
      close()
      out += ch
    }
    i += ch.length
    n += ch[0] === '&' && ch.length > 1 ? 1 : ch.length
    if (n >= mark.to) close()
  }
  close()
  return n === length ? out : null
}
