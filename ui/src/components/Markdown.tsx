import { useEffect, useState } from 'react'
import { api } from '../api'
import type { DiffFile } from '../types'

// The rendered reading of a markdown file: before and after side by side, the
// same two panes the image preview uses, one pane where the change added or
// removed the file. Each pane reads the whole file as its side of the scope
// sees it and renders it (lib/md); a relative image resolves through the blob
// endpoint at that same side, so a screenshot the change added shows on the
// right and not on the left.

export const isMarkdown = (path: string) => /\.(md|markdown)$/i.test(path)

const blobURL = (name: string, repo: string, scope: string, path: string, side: 'before' | 'after') =>
  `api/blob?name=${encodeURIComponent(name)}&repo=${encodeURIComponent(repo)}&scope=${encodeURIComponent(scope)}` +
  `&file=${encodeURIComponent(path)}&side=${side}`

function Pane({
  label,
  side,
  name,
  repo,
  scope,
  file,
  poll,
}: {
  label: string
  side: 'before' | 'after'
  name: string
  repo: string
  scope: string
  file: DiffFile
  poll: number
}) {
  const [html, setHtml] = useState<string | null>(null)
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const [md, r] = await Promise.all([import('../lib/md'), api.fileText(name, repo, scope, file.path, side)])
        if (cancelled) return
        const out = md.renderMarkdown(r.lines.join('\n'), (rel) => {
          const path = md.resolvePath(file.path, rel)
          return path ? blobURL(name, repo, scope, path, side) : undefined
        })
        setHtml((prev) => (prev === out ? prev : out)) // identical text: no re-render
      } catch {
        if (!cancelled) setFailed(true)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [name, repo, scope, file.path, side, poll])
  if (failed) return null
  return (
    <figure className="pv-pane">
      <figcaption className="pv-label">{label}</figcaption>
      {html === null ? (
        <div className="empty small">rendering…</div>
      ) : (
        <div className="md" dangerouslySetInnerHTML={{ __html: html }} />
      )}
    </figure>
  )
}

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
  const pane = (label: string, side: 'before' | 'after') => (
    <Pane label={label} side={side} name={name} repo={repo} scope={scope} file={file} poll={poll} />
  )
  return (
    <div className={`pv${added || removed ? ' pv-single' : ''}`}>
      {!added && pane(removed ? 'deleted' : 'before', 'before')}
      {!removed && pane(added ? 'added' : 'after', 'after')}
    </div>
  )
}
