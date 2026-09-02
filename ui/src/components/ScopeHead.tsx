import { useState } from 'react'
import type { Scope } from '../types'
import { commitState } from '../lib/commit'

// The header over the file tree and the diff: WHAT is being read, not which
// file is open — the file has its own header in the viewer, and this one has
// to survive a multi-file selection. For a commit that is its subject, with
// the message body one click below; for a range or the index it is the scope
// and what it holds.
//
// It sits over the two right columns only. The scope column is a list of
// these things, and a header over a list of alternatives says nothing.
export function ScopeHead({ scope, repo, base }: { scope?: Scope; repo?: string; base?: string }) {
  const [open, setOpen] = useState(false)
  if (!scope) return null
  const commit = scope.kind === 'commit'
  const state = commit ? commitState(scope, base) : null
  return (
    <div className="ch">
      <div className="ch-top">
        <span className="ch-title" title={scope.label}>
          {scope.label}
        </span>
        {commit && !!scope.body && (
          <button className="ch-more" onClick={() => setOpen((v) => !v)}>
            {open ? '▾ message' : '▸ message'}
          </button>
        )}
      </div>
      <div className="ch-meta dim">
        {commit ? (
          <>
            <span className={state!.cls} title={state!.title} />
            <span className="mono">{scope.sha}</span>
            {/* the header has the room the list row does not, so the state
                that matters is a word here and a colour there */}
            {!!state!.tag && <span className={state!.tagCls}>{state!.tag}</span>}
            {scope.author && <span>· {scope.author}</span>}
            {scope.date && <span>· {scope.date}</span>}
          </>
        ) : (
          scope.hint && <span className="mono">{scope.hint}</span>
        )}
        {repo && <span>· {repo}</span>}
        <span>
          · {scope.files} {scope.files === 1 ? 'file' : 'files'}
        </span>
        {!!scope.added && <span className="plus">+{scope.added}</span>}
        {!!scope.deleted && <span className="minus">−{scope.deleted}</span>}
      </div>
      {open && !!scope.body && <pre className="ch-body">{scope.body}</pre>}
    </div>
  )
}
