import { useEffect, useRef, useState } from 'react'
import { api } from '../api'
import type { JobLine, RemoteRepo, RemoteResult } from '../types'

// A push or a pull, while it happens.
//
// These take seconds and sometimes tens of them — a fetch of four repositories
// over a real remote measured eight — and a button that says "Pulling…" for
// that long is indistinguishable from one that has hung. So the commands are
// shown as they run.
//
// It is a transcript rather than a spinner on purpose. Everything grove does
// to a repository is a git command somebody could have typed, and showing
// which ones, in order, is the difference between a tool you trust with your
// work and one you hope about. When it goes wrong the transcript IS the
// explanation, already on screen.
export function RunLog({
  name,
  repo,
  action,
  remote,
  onDone,
  onClose,
}: {
  name: string
  repo: string
  action: 'push' | 'rebase' | 'merge' | 'ff'
  remote?: string
  onDone: (results: RemoteResult[], repos?: RemoteRepo[]) => void
  onClose: () => void
}) {
  const [lines, setLines] = useState<JobLine[]>([])
  const [done, setDone] = useState(false)
  const [failed, setFailed] = useState<RemoteResult[]>([])
  const box = useRef<HTMLDivElement>(null)
  // onDone is written fresh by the parent on every render, so it must not be a
  // dependency: the effect would re-run, its cleanup would stop the polling,
  // and the re-run would find the job already started and do nothing. The
  // transcript then froze mid-command while the work carried on without it.
  const finished = useRef(onDone)
  finished.current = onDone

  useEffect(() => {
    let stop = false
    let timer = 0
    ;(async () => {
      try {
        const { job } = await api.run({ name, repos: [repo], action, remote })
        let after = 0
        const poll = async () => {
          if (stop) return
          try {
            const s = await api.job(job, after)
            after = s.next
            if (s.lines.length) setLines((l) => [...l, ...s.lines])
            if (s.done) {
              setDone(true)
              const bad = (s.results ?? []).filter((r) => !r.ok)
              setFailed(bad)
              finished.current(s.results ?? [], s.repos)
              return
            }
          } catch {
            // the job is gone, or the server went away mid-run
            setDone(true)
            return
          }
          timer = window.setTimeout(poll, 250)
        }
        poll()
      } catch (e) {
        setLines([{ kind: 'err', text: String((e as Error).message || e) }])
        setDone(true)
      }
    })()
    return () => {
      stop = true
      clearTimeout(timer)
    }
  }, [name, repo, action, remote])

  // follow the tail, the way a terminal does
  useEffect(() => {
    if (box.current) box.current.scrollTop = box.current.scrollHeight
  }, [lines])

  const title = done
    ? failed.length
      ? 'That did not go through'
      : 'Done'
    : { push: 'Pushing', rebase: 'Rebasing', merge: 'Merging', ff: 'Fast-forwarding' }[action] + '…'

  return (
    <div className="modal-backdrop" onClick={done ? onClose : undefined}>
      <div className="modal modal-wide" onClick={(e) => e.stopPropagation()}>
        <h2 className="modal-title">
          {title} <span className="mono run-of">{repo}</span>
        </h2>
        <div className="run" ref={box}>
          {lines.map((l, i) => (
            <div key={i} className={'run-' + l.kind}>
              {l.kind === 'cmd' && <span className="run-prompt">{l.dir} $</span>}
              {l.kind === 'ok' && <span className="run-mark">✓</span>}
              {l.kind === 'err' && <span className="run-mark">✕</span>}
              <span className="run-text">{l.text}</span>
            </div>
          ))}
          {!done && <div className="run-note run-live">…</div>}
        </div>
        {done &&
          failed.map((r, i) => (
            <div key={i} className="run-why">
              {r.why || r.detail}
            </div>
          ))}
        {done && !failed.length && (
          <p className="dim run-safe">Nothing else was touched.</p>
        )}
        {done && failed.length > 0 && (
          <p className="dim run-safe">
            Nothing was left half-done: grove undoes each step it cannot finish, so the repository is
            where it was before you pressed the button.
          </p>
        )}
        <div className="modal-actions">
          <button className="btn-ghost" onClick={onClose} disabled={!done}>
            {done ? 'Close' : 'Working…'}
          </button>
        </div>
      </div>
    </div>
  )
}
