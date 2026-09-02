// Syntax colours for the diff. A diff is not a file — a block comment can open
// in one hunk and close in the next, and a tokenizer fed one hunk at a time
// gets that wrong — so each SIDE of a file is highlighted whole, at the
// scope's own revision, and a diff line looks its markup up by line number.
// The highlighter loads on first use (lib/hljs.ts), with a fixed set of
// languages chosen by file name; a file outside the set stays plain.

const byExt: Record<string, string> = {
  go: 'go',
  ts: 'typescript', tsx: 'typescript', mts: 'typescript', cts: 'typescript',
  js: 'javascript', jsx: 'javascript', mjs: 'javascript', cjs: 'javascript',
  css: 'css', scss: 'scss',
  html: 'xml', htm: 'xml', xml: 'xml', svg: 'xml', xsl: 'xml', xslt: 'xml',
  json: 'json', yml: 'yaml', yaml: 'yaml',
  md: 'markdown', markdown: 'markdown',
  sh: 'bash', bash: 'bash', zsh: 'bash',
  py: 'python', rb: 'ruby', php: 'php', rs: 'rust', java: 'java',
  c: 'c', h: 'c', cpp: 'cpp', cc: 'cpp', hpp: 'cpp', hh: 'cpp',
  sql: 'sql', coffee: 'coffeescript',
  ini: 'ini', toml: 'ini', cfg: 'ini', conf: 'ini',
  diff: 'diff', patch: 'diff',
  mk: 'makefile',
}
const byName: Record<string, string> = {
  Makefile: 'makefile', GNUmakefile: 'makefile', makefile: 'makefile',
  Dockerfile: 'dockerfile', Containerfile: 'dockerfile',
  '.bashrc': 'bash', '.zshrc': 'bash', '.profile': 'bash',
}

// languageOf names the highlighter's language for a path, or nothing.
export function languageOf(path: string): string | undefined {
  const base = path.slice(path.lastIndexOf('/') + 1)
  if (byName[base]) return byName[base]
  if (base.startsWith('Dockerfile.')) return 'dockerfile'
  const dot = base.lastIndexOf('.')
  return dot > 0 ? byExt[base.slice(dot + 1).toLowerCase()] : undefined
}

let loading: Promise<typeof import('./hljs').default> | undefined
export const loadHljs = () => (loading ??= import('./hljs').then((m) => m.default))

// highlightLines colours a whole text and hands back one HTML string per
// line, safe to set as innerHTML: highlight.js escapes the text and emits
// nothing but its own spans.
export async function highlightLines(text: string, language: string): Promise<string[]> {
  const hljs = await loadHljs()
  return splitLines(hljs.highlight(text, { language, ignoreIllegals: true }).value)
}

// splitLines cuts highlighted HTML at newlines. A span may run across a line
// break — a multi-line string, a block comment — so every span still open at
// the break is closed there and reopened on the next line.
export function splitLines(html: string): string[] {
  const out: string[] = []
  const open: string[] = []
  let line = ''
  let i = 0
  while (i < html.length) {
    const lt = html.indexOf('<', i)
    const nl = html.indexOf('\n', i)
    if (nl >= 0 && (lt < 0 || nl < lt)) {
      out.push(line + html.slice(i, nl) + '</span>'.repeat(open.length))
      line = open.join('')
      i = nl + 1
      continue
    }
    if (lt < 0) {
      line += html.slice(i)
      break
    }
    line += html.slice(i, lt)
    const gt = html.indexOf('>', lt)
    const tag = html.slice(lt, gt + 1)
    if (tag.startsWith('</')) open.pop()
    else open.push(tag)
    line += tag
    i = gt + 1
  }
  out.push(line + '</span>'.repeat(open.length))
  return out
}
