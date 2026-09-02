// Rendered markdown: GitHub-flavoured through marked, fenced code through the
// same highlighter as the diff, and everything through DOMPurify before it
// touches the page — the file comes out of somebody's repository and this
// page has a write endpoint. In its own chunk, loaded when a markdown file
// is first switched to rendered.
import { Marked, type Tokens } from 'marked'
import DOMPurify from 'dompurify'
import hljs from './hljs'

const escape = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

const marked = new Marked({
  gfm: true,
  renderer: {
    code({ text, lang }: Tokens.Code) {
      const language = lang && hljs.getLanguage(lang) ? lang : undefined
      const html = language ? hljs.highlight(text, { language, ignoreIllegals: true }).value : escape(text)
      return `<pre><code class="hljs">${html}</code></pre>\n`
    },
  },
})

// A leading YAML block between two `---` lines is metadata, not a heading
// (markdown would read the text before the second rule as one): it is shown
// as it is, small and dim, ahead of the body.
function splitFrontMatter(text: string): { front: string; body: string } {
  const m = /^---\r?\n([\s\S]*?)\r?\n---\r?\n?/.exec(text)
  return m ? { front: m[1], body: text.slice(m[0].length) } : { front: '', body: text }
}

// renderMarkdown returns sanitized HTML. resolve maps a relative image path
// to a URL the page can load — an image next to the README lives in the
// repository, at the same revision as the text that shows it.
export function renderMarkdown(text: string, resolve: (rel: string) => string | undefined): string {
  const { front, body } = splitFrontMatter(text)
  const raw = (front ? `<pre class="md-front">${escape(front)}</pre>\n` : '') + (marked.parse(body, { async: false }) as string)
  const frag = DOMPurify.sanitize(raw, { RETURN_DOM_FRAGMENT: true })
  frag.querySelectorAll('img').forEach((img) => {
    const src = img.getAttribute('src') || ''
    img.setAttribute('data-src', src) // the reference as written: what the diff compares
    if (/^([a-z][a-z0-9+.-]*:|\/|#)/i.test(src)) return
    const url = resolve(src)
    if (url) img.setAttribute('src', url)
  })
  frag.querySelectorAll('a').forEach((a) => {
    if (/^https?:/i.test(a.getAttribute('href') || '')) {
      a.setAttribute('target', '_blank')
      a.setAttribute('rel', 'noreferrer')
    }
  })
  const div = document.createElement('div')
  div.appendChild(frag)
  return div.innerHTML
}

// resolvePath joins a relative reference onto the directory of the file that
// made it, the way a browser would, and refuses to climb out of the checkout.
export function resolvePath(fromFile: string, rel: string): string | undefined {
  const parts = fromFile.split('/').slice(0, -1)
  for (const seg of rel.split('?')[0].split('#')[0].split('/')) {
    if (seg === '' || seg === '.') continue
    if (seg === '..') {
      if (!parts.length) return undefined
      parts.pop()
      continue
    }
    parts.push(seg)
  }
  return parts.join('/')
}

// ── the rendered diff ────────────────────────────────────────────────────
// Two rendered pages, coloured by what changed between them. The blocks —
// paragraphs, headings, list items, table rows, code — are matched by their
// text with a longest common subsequence. A block only one side has is tinted
// whole; a removed block facing an added one is matched again word by word,
// so a reworded sentence shows the words that moved and not the paragraph.

type Op = { type: 'eq' | 'del' | 'ins'; i: number; j: number }

export function diffRendered(
  beforeHtml: string,
  afterHtml: string,
): { before: string; after: string; changes: number } {
  const parse = (html: string) => {
    const div = document.createElement('div')
    div.innerHTML = html
    return div
  }
  const a = parse(beforeHtml)
  const b = parse(afterHtml)
  const ua = units(a)
  const ub = units(b)
  if (ua.length * ub.length > 4_000_000) return { before: beforeHtml, after: afterHtml, changes: 0 }
  const ops = lcsOps(ua.map(unitKey), ub.map(unitKey))
  let i = 0
  let group = 0 // each run of changes is one stop for the viewer's ↑ ↓
  while (i < ops.length) {
    if (ops[i].type === 'eq') {
      i++
      continue
    }
    // a run of changes between two matches: the removals and the additions
    // face each other in order, and each facing pair is compared by word
    const dels: Element[] = []
    const inss: Element[] = []
    for (; i < ops.length && ops[i].type !== 'eq'; i++) {
      if (ops[i].type === 'del') dels.push(ua[ops[i].i])
      else inss.push(ub[ops[i].j])
    }
    const pairs = Math.min(dels.length, inss.length)
    for (let k = 0; k < pairs; k++) diffWords(dels[k], inss[k])
    for (let k = pairs; k < dels.length; k++) dels[k].classList.add('md-b-del')
    for (let k = pairs; k < inss.length; k++) inss[k].classList.add('md-b-add')
    for (const el of [...dels, ...inss]) el.setAttribute('data-chg', String(group))
    group++
  }
  return { before: a.innerHTML, after: b.innerHTML, changes: group }
}

// units are the blocks compared: the top-level elements, with lists and
// tables opened to their items and rows so one changed item does not paint
// the whole list.
function units(root: Element): Element[] {
  const out: Element[] = []
  for (const el of Array.from(root.children)) {
    if (el.tagName === 'UL' || el.tagName === 'OL') out.push(...Array.from(el.children))
    else if (el.tagName === 'TABLE') out.push(...Array.from(el.querySelectorAll('tr')))
    else out.push(el)
  }
  return out
}

// a block's identity for the match: its tag, its text, and the images it
// shows — by the reference as written, since the resolved URL names a side
const unitKey = (el: Element) =>
  el.tagName +
  ':' +
  (el.textContent || '').replace(/\s+/g, ' ').trim() +
  Array.from(el.querySelectorAll('img'))
    .map((img) => '|' + (img.getAttribute('data-src') || ''))
    .join('')

// lcsOps aligns two sequences. Plain dynamic programming: the inputs are the
// blocks of one document or the words of one paragraph, and both are small.
function lcsOps(a: string[], b: string[]): Op[] {
  const n = a.length
  const m = b.length
  const w = m + 1
  const dp = new Uint16Array((n + 1) * w)
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i * w + j] = a[i] === b[j] ? dp[(i + 1) * w + j + 1] + 1 : Math.max(dp[(i + 1) * w + j], dp[i * w + j + 1])
    }
  }
  const ops: Op[] = []
  let i = 0
  let j = 0
  while (i < n && j < m) {
    if (a[i] === b[j]) ops.push({ type: 'eq', i: i++, j: j++ })
    else if (dp[(i + 1) * w + j] >= dp[i * w + j + 1]) ops.push({ type: 'del', i: i++, j })
    else ops.push({ type: 'ins', i, j: j++ })
  }
  while (i < n) ops.push({ type: 'del', i: i++, j })
  while (j < m) ops.push({ type: 'ins', i, j: j++ })
  return ops
}

type Tok = { text: string; node: Text; start: number; end: number; space: boolean }

// diffWords marks, inside a facing pair of blocks, the words only one side
// has. Tokens are words and the whitespace between them, each inside a single
// text node, so a marked token wraps cleanly however the inline markup runs.
function diffWords(a: Element, b: Element) {
  a.classList.add('md-b-chg')
  b.classList.add('md-b-chg')
  const ta = tokens(a)
  const tb = tokens(b)
  const wa = ta.filter((t) => !t.space)
  const wb = tb.filter((t) => !t.space)
  if (wa.length * wb.length > 1_000_000) return
  const ia = ta.map((t, i) => i).filter((i) => !ta[i].space) // token index of each word
  const ib = tb.map((t, i) => i).filter((i) => !tb[i].space)
  const markA = new Set<number>()
  const markB = new Set<number>()
  for (const op of lcsOps(wa.map((t) => t.text), wb.map((t) => t.text))) {
    if (op.type === 'del') markA.add(ia[op.i])
    if (op.type === 'ins') markB.add(ib[op.j])
  }
  wrap(ta, markA, 'del')
  wrap(tb, markB, 'ins')
}

function tokens(el: Element): Tok[] {
  const out: Tok[] = []
  const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT)
  for (let n = walker.nextNode() as Text | null; n; n = walker.nextNode() as Text | null) {
    const re = /\s+|\S+/g
    let m: RegExpExecArray | null
    while ((m = re.exec(n.data))) {
      out.push({ text: m[0], node: n, start: m.index, end: m.index + m[0].length, space: /^\s+$/.test(m[0]) })
    }
  }
  return out
}

function wrap(toks: Tok[], marked: Set<number>, tag: 'ins' | 'del') {
  // whitespace between two marked words is marked too, so a run reads as one
  toks.forEach((t, i) => {
    if (t.space && marked.has(i - 1) && marked.has(i + 1)) marked.add(i)
  })
  // wrapped from the end: splitting a text node leaves the offsets before the
  // split where they were
  for (let i = toks.length - 1; i >= 0; i--) {
    if (!marked.has(i)) continue
    const t = toks[i]
    const range = document.createRange()
    range.setStart(t.node, t.start)
    range.setEnd(t.node, t.end)
    const el = document.createElement(tag)
    el.className = tag === 'ins' ? 'md-w-add' : 'md-w-del'
    range.surroundContents(el)
  }
}
