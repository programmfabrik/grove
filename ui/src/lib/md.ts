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

// renderMarkdown returns sanitized HTML. resolve maps a relative image path
// to a URL the page can load — an image next to the README lives in the
// repository, at the same revision as the text that shows it.
export function renderMarkdown(text: string, resolve: (rel: string) => string | undefined): string {
  const raw = marked.parse(text, { async: false }) as string
  const frag = DOMPurify.sanitize(raw, { RETURN_DOM_FRAGMENT: true })
  frag.querySelectorAll('img').forEach((img) => {
    const src = img.getAttribute('src') || ''
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
