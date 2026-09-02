import type { Checkout } from '../types'
import { Copy, Field } from './ui'
import { fmtDateTime } from '../lib/format'

// Everything known about one checkout that does not fit the list row: its
// full path, its branch spelled out against the base, and the head commit.
export function WorktreeTab({ c, base }: { c: Checkout; base: string }) {
  return (
    <div className="wt">
      <div className="wt-grid">
        <Field label="Path">
          <Copy text={c.path} title="Copy path">
            <span className="mono">{c.path}</span>
          </Copy>
        </Field>
        <Field label="Branch">
          <span className="mono">{c.detached ? 'detached' : c.branch}</span>
          {c.ticket && <span className="dim"> · #{c.ticket}</span>}
          <div className="dim small">
            {c.ahead} ahead of {base} · {c.behind} behind · {c.dirty} uncommitted
          </div>
        </Field>
        <Field label="Head">
          <span className="mono">{c.head.hash}</span>
          <div className="dim small">
            {c.head.author} · {fmtDateTime(c.head.date)}
          </div>
          <div className="small">{c.head.subject}</div>
        </Field>
      </div>
    </div>
  )
}
