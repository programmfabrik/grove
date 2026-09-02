import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '../api'
import type { DiffFile } from '../types'
import { Preview } from './Preview'

// One file's diff: its own text, its own expanded context, its own preview.
// Extracted from the viewer so a selection of many files is simply many of
// these — each fetching only when it is actually open.

type Line = {
  kind: 'add' | 'del' | 'ctx' | 'hunk' | 'meta'
  text: string
  old?: number
  new?: number
  // hunk lines only: the unchanged stretch above this hunk that the diff left
  // out, and how far the before-numbering trails the after-numbering in it
  gapFrom?: number
  gapTo?: number
  offset?: number
}

// how many hidden lines one click on an expander reveals
const EXPAND_LINES = 20

// parseDiff turns unified diff text into rendered lines, carrying the old and
// new line numbers from each hunk header so the gutters can show them.
export function parseDiff(text: string): Line[] {
  const out: Line[] = []
  let oldNo = 0
  let newNo = 0
  let lastNew = 0 // the last after-line the diff has shown so far
  for (const raw of text.split('\n')) {
    if (raw.startsWith('@@')) {
      const m = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(raw)
      oldNo = m ? Number(m[1]) : 0
      newNo = m ? Number(m[2]) : 0
      out.push({
        kind: 'hunk',
        text: raw,
        gapFrom: lastNew + 1, // 1 for the first hunk: the head of the file
        gapTo: newNo - 1,
        offset: newNo - oldNo,
      })
      continue
    }
    // the git header repeats what the tree and the head already say
    if (
      raw.startsWith('diff --git') ||
      raw.startsWith('index ') ||
      raw.startsWith('--- ') ||
      raw.startsWith('+++ ')
    ) {
      continue
    }
    if (
      raw.startsWith('new file') ||
      raw.startsWith('deleted file') ||
      raw.startsWith('similarity index') ||
      raw.startsWith('rename ') ||
      raw.startsWith('old mode') ||
      raw.startsWith('new mode') ||
      raw.startsWith('Binary files')
    ) {
      out.push({ kind: 'meta', text: raw })
      continue
    }
    if (raw.startsWith('+')) out.push({ kind: 'add', text: raw.slice(1), new: (lastNew = newNo++) })
    else if (raw.startsWith('-')) out.push({ kind: 'del', text: raw.slice(1), old: oldNo++ })
    else if (raw.startsWith('\\')) out.push({ kind: 'meta', text: raw })
    else out.push({ kind: 'ctx', text: raw.slice(1), old: oldNo++, new: (lastNew = newNo++) })
  }
  // a trailing empty context line is just the text ending in a newline
  if (out.length && out[out.length - 1].kind === 'ctx' && out[out.length - 1].text === '') out.pop()
  return out
}

export function FileDiff({
  name,
  repo,
  scope,
  file,
  ignoreComments,
  poll,
}: {
  name: string
  repo: string
  scope: string
  file: DiffFile
  ignoreComments: boolean
  poll: number // changes on every background tick
}) {
  const [text, setText] = useState('')
  const [total, setTotal] = useState(0)
  const [truncated, setTruncated] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  // expanded context, per hunk; -1 is the stretch below the last hunk
  const [expanded, setExpanded] = useState<Record<number, { from: number; lines: string[] }>>({})

  const load = useCallback(async () => {
    const r = await api.diffText(name, repo, scope, file.path, file.untracked, ignoreComments)
    setText((prev) => (prev === r.diff ? prev : r.diff)) // identical text: no re-render
    setTotal(r.total)
    setTruncated(!!r.truncated)
  }, [name, repo, scope, file.path, file.untracked, ignoreComments])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setExpanded({})
    load()
      .catch((e) => !cancelled && setError(String(e)))
      .finally(() => !cancelled && setLoading(false))
    return () => {
      cancelled = true
    }
  }, [load])

  // the background refresh: same fetch, no spinner, no reset of what is open
  const first = useRef(true)
  useEffect(() => {
    if (first.current) {
      first.current = false
      return
    }
    load().catch(() => {})
  }, [poll, load])

  const lines = useMemo(() => parseDiff(text), [text])

  const expand = async (idx: number, gapFrom: number, gapTo: number) => {
    const already = expanded[idx]
    const to = already ? already.from - 1 : gapTo
    if (to < gapFrom) return
    const from = Math.max(gapFrom, to - EXPAND_LINES + 1)
    const r = await api.fileLines(name, repo, scope, file.path, from, to)
    setTotal(r.total)
    setExpanded((prev) => {
      const old = prev[idx]
      return { ...prev, [idx]: { from: r.from, lines: old ? [...r.lines, ...old.lines] : r.lines } }
    })
  }

  const lastNew = [...lines].reverse().find((l) => l.new !== undefined)?.new ?? 0
  const tail = expanded[-1]
  const tailTo = tail ? tail.from + tail.lines.length - 1 : lastNew
  const tailLeft = total ? total - tailTo : 0

  const expandTail = async () => {
    const start = tail ? tail.from + tail.lines.length : lastNew + 1
    const r = await api.fileLines(name, repo, scope, file.path, start, start + EXPAND_LINES - 1)
    setTotal(r.total)
    if (!r.lines.length) return
    setExpanded((prev) => {
      const old = prev[-1]
      return { ...prev, [-1]: { from: old ? old.from : r.from, lines: old ? [...old.lines, ...r.lines] : r.lines } }
    })
  }

  if (error) return <div className="error">{error}</div>
  if (loading && !lines.length && !file.preview) return <div className="empty small">loading…</div>

  return (
    <>
      {file.preview && <Preview name={name} repo={repo} scope={scope} file={file} />}
      <div className="diff mono">
        {lines.map((l, i) => {
          if (l.kind !== 'hunk') {
            return (
              <div key={i} className={`dl dl-${l.kind}`}>
                <span className="dl-no">{l.old ?? ''}</span>
                <span className="dl-no">{l.new ?? ''}</span>
                <span className="dl-sign">{l.kind === 'add' ? '+' : l.kind === 'del' ? '−' : ' '}</span>
                <span className="dl-text">{l.text || ' '}</span>
              </div>
            )
          }
          // a hunk header IS the expander: what it hides is the stretch of
          // unchanged file between the previous hunk and this one
          const shown = expanded[i]
          const from = l.gapFrom ?? 1
          const top = shown ? shown.from - 1 : (l.gapTo ?? 0)
          const left = top - from + 1
          return (
            <div key={i}>
              {/* the row is the fold: once nothing is folded any more, the
                  lines above and below are one contiguous stretch of the
                  file and a seam between them would only mislead */}
              {left > 0 && (
                <div className="dl dl-hunk">
                  <span className="dl-expand">
                    <button
                      className="ex-btn"
                      title={`show ${Math.min(left, EXPAND_LINES)} of ${left} hidden lines`}
                      onClick={() => expand(i, from, l.gapTo ?? 0)}
                    >
                      ↑{left}
                    </button>
                  </span>
                  <span className="dl-sign"> </span>
                  <span className="dl-text">{l.text}</span>
                </div>
              )}
              {shown?.lines.map((t, k) => (
                <div key={k} className="dl dl-ctx dl-context">
                  <span className="dl-no">{shown.from + k - (l.offset ?? 0)}</span>
                  <span className="dl-no">{shown.from + k}</span>
                  <span className="dl-sign"> </span>
                  <span className="dl-text">{t || ' '}</span>
                </div>
              ))}
            </div>
          )
        })}

        {truncated && <div className="empty">— diff truncated —</div>}

        {/* below the last hunk there is often more file — but not always, and
            the line count from the diff response says which */}
        {!!lines.length && (
          <>
            {tail?.lines.map((t, k) => (
              <div key={k} className="dl dl-ctx dl-context">
                <span className="dl-no" />
                <span className="dl-no">{tail.from + k}</span>
                <span className="dl-sign"> </span>
                <span className="dl-text">{t || ' '}</span>
              </div>
            ))}
            {total > 0 && tailTo < total && (
              <div className="dl dl-hunk">
                <span className="dl-expand">
                  <button
                    className="ex-btn"
                    title={`show ${Math.min(tailLeft, EXPAND_LINES)} of ${tailLeft} lines below`}
                    onClick={expandTail}
                  >
                    ↓{tailLeft}
                  </button>
                </span>
                <span className="dl-sign"> </span>
                <span className="dl-text dim">more</span>
              </div>
            )}
          </>
        )}
      </div>
    </>
  )
}
