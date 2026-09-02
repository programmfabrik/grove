import { useState } from 'react'
import type { DiffFile } from '../types'

// "Binary files … differ" is a dead end for the file types a browser renders
// itself. For those the viewer shows the file: before and after side by side
// where both exist, one pane where the change added or removed it.
//
// Which side exists follows from the status — a new file has no before, a
// deleted one no after — but a rename or a fork-point edge can still 404, so
// each pane also hides itself if the blob does not load.

const blobURL = (name: string, repo: string, scope: string, f: DiffFile, side: 'before' | 'after') =>
  `api/blob?name=${encodeURIComponent(name)}&repo=${encodeURIComponent(repo)}&scope=${encodeURIComponent(scope)}` +
  `&file=${encodeURIComponent(f.path)}&side=${side}`

function Media({ kind, src, onError }: { kind: string; src: string; onError: () => void }) {
  switch (kind) {
    case 'video':
      return <video className="pv-media" src={src} controls preload="metadata" onError={onError} />
    case 'audio':
      return <audio className="pv-audio" src={src} controls preload="metadata" onError={onError} />
    case 'pdf':
      return <iframe className="pv-pdf" src={src} title={src} />
    default:
      return <img className="pv-media" src={src} alt="" onError={onError} />
  }
}

function Pane({ label, kind, src }: { label: string; kind: string; src: string }) {
  const [failed, setFailed] = useState(false)
  if (failed) return null
  return (
    <figure className="pv-pane">
      <figcaption className="pv-label">{label}</figcaption>
      <Media kind={kind} src={src} onError={() => setFailed(true)} />
    </figure>
  )
}

export function Preview({
  name,
  repo,
  scope,
  file,
}: {
  name: string
  repo: string
  scope: string
  file: DiffFile
}) {
  const kind = file.preview!
  const added = file.status === 'new' || file.status === 'added' || file.untracked
  const removed = file.status === 'deleted'

  return (
    <div className={`pv${added || removed ? ' pv-single' : ''}`}>
      {!added && (
        <Pane label={removed ? 'deleted' : 'before'} kind={kind} src={blobURL(name, repo, scope, file, 'before')} />
      )}
      {!removed && (
        <Pane label={added ? 'added' : 'after'} kind={kind} src={blobURL(name, repo, scope, file, 'after')} />
      )}
    </div>
  )
}
