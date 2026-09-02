import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../api'
import type { DiffFile } from '../types'

// The rendered reading of a markdown file: before and after side by side, the
// same two panes the image preview uses, one pane where the change added or
// removed the file. Both sides are read whole as their side of the scope sees
// them, rendered (lib/md), and — where both exist — compared: a block only
// one side has is tinted, a reworded one shows its changed words, in the line
// diff's own green and red. A relative image resolves through the blob
// endpoint at the pane's own side, so a screenshot the change added shows on
// the right and not on the left.

export const isMarkdown = (path: string) => /\.(md|markdown)$/i.test(path)

const blobURL = (name: string, repo: string, scope: string, path: string, side: 'before' | 'after') =>
  `api/blob?name=${encodeURIComponent(name)}&repo=${encodeURIComponent(repo)}&scope=${encodeURIComponent(scope)}` +
  `&file=${encodeURIComponent(path)}&side=${side}`

type Sides = { before?: string; after?: string; changes: number }

export function Markdown({
  name,
  repo,
  scope,
  file,
  poll,
}: {
  name: string
  repo: string
  scope: string
  file: DiffFile
  poll: number
}) {
  const added = file.status === 'new' || file.status === 'added' || file.untracked
  const removed = file.status === 'deleted'
  const [html, setHtml] = useState<Sides | null>(null)
  const [failed, setFailed] = useState(false)
  const box = useRef<HTMLDivElement>(null)
  const [at, setAt] = useState(-1) // which change the ↑ ↓ last went to

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const md = await import('../lib/md')
        const side = async (s: 'before' | 'after') => {
          try {
            const r = await api.fileText(name, repo, scope, file.path, s)
            return md.renderMarkdown(r.lines.join('\n'), (rel) => {
              const path = md.resolvePath(file.path, rel)
              return path ? blobURL(name, repo, scope, path, s) : undefined
            })
          } catch {
            return undefined // no file on that side
          }
        }
        const [before, after] = await Promise.all([added ? undefined : side('before'), removed ? undefined : side('after')])
        if (cancelled) return
        if (before === undefined && after === undefined) {
          setFailed(true)
          return
        }
        const out: Sides =
          before !== undefined && after !== undefined ? md.diffRendered(before, after) : { before, after, changes: 0 }
        // identical pages: no re-render under the reader
        setHtml((prev) => (prev && prev.before === out.before && prev.after === out.after ? prev : out))
      } catch {
        if (!cancelled) setFailed(true)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [name, repo, scope, file.path, added, removed, poll])

  // An image the browser cannot fetch — a badge from a private repository, a
  // path that only resolves on the other side — leaves a broken icon that says
  // nothing. Marked as failed, it reads as its own alt text instead. The
  // listeners go on after each render, since the HTML is set as a string.
  useEffect(() => {
    const root = box.current
    if (!root || !html) return
    const imgs = Array.from(root.querySelectorAll('img'))
    const fail = (img: HTMLImageElement) => img.classList.add('md-img-broken')
    for (const img of imgs) {
      if (img.complete && img.naturalWidth === 0) fail(img)
      const onError = () => fail(img)
      img.addEventListener('error', onError)
      ;(img as HTMLImageElement & { _off?: () => void })._off = () =>
        img.removeEventListener('error', onError)
    }
    return () => imgs.forEach((i) => (i as HTMLImageElement & { _off?: () => void })._off?.())
  }, [html])

  // ↑ ↓ walk the changes. A change is one run of blocks and can stand on both
  // sides at once; the two panes scroll as one, so the stop is whichever of
  // the pair sits higher — its counterpart is then beside or just below it.
  const goto = useCallback(
    (n: number) => {
      const root = box.current
      if (!root || !html?.changes) return
      const next = ((n % html.changes) + html.changes) % html.changes
      const pair = Array.from(root.querySelectorAll<HTMLElement>(`[data-chg="${next}"]`))
      if (!pair.length) return
      const top = pair.reduce((a, b) => (a.getBoundingClientRect().top <= b.getBoundingClientRect().top ? a : b))
      top.scrollIntoView({ block: 'center', behavior: 'smooth' })
      pair.forEach((el) => {
        el.classList.remove('md-b-at')
        void el.offsetWidth // restart the flash when the same stop is asked for twice
        el.classList.add('md-b-at')
      })
      setAt(next)
    },
    [html],
  )

  if (failed) return <div className="empty small">cannot render this file</div>
  const single = added || removed
  const pane = (label: string, body?: string) => (
    <figure className="pv-pane">
      <figcaption className="pv-label">{label}</figcaption>
      {html === null ? (
        <div className="empty small">rendering…</div>
      ) : (
        <div className="md" dangerouslySetInnerHTML={{ __html: body ?? '' }} />
      )}
    </figure>
  )
  return (
    <div className={`pv${single ? ' pv-single' : ''}`} ref={box}>
      {!!html?.changes && (
        <div className="md-nav">
          <span className="dim">
            {at < 0 ? `${html.changes} change${html.changes === 1 ? '' : 's'}` : `change ${at + 1} of ${html.changes}`}
          </span>
          <button className="ex-btn" onClick={() => goto(at < 0 ? html.changes - 1 : at - 1)} title="previous change">
            ↑
          </button>
          <button className="ex-btn" onClick={() => goto(at + 1)} title="next change">
            ↓
          </button>
        </div>
      )}
      {!added && pane(removed ? 'deleted' : 'before', html?.before)}
      {!removed && pane(added ? 'added' : 'after', html?.after)}
    </div>
  )
}
