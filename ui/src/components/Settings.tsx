import { useEffect, useState } from 'react'
import { api } from '../api'
import type { Diagnostics } from '../types'

// What grove is standing on, and what it is missing.
//
// grove runs other programs, and when one is absent a feature quietly does not
// happen — the check column stays empty, and nobody is told whether that means
// "nothing is running" or "I could not ask". This says which, for each of
// them, with what it is for and what goes without it.
// SettingsPage is the window of its own that cmd-comma opens. The same body,
// without a dialog around it, because a window that draws its own modal inside
// itself is a web page wearing a window.
export function SettingsPage() {
  return (
    <div className="set-page">
      <h1 className="set-page-title">Settings</h1>
      <SettingsBody />
    </div>
  )
}

export function Settings({ onClose }: { onClose: () => void }) {
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal modal-wide" onClick={(e) => e.stopPropagation()}>
        <h2 className="modal-title">Settings</h2>
        <div className="modal-body">
          <SettingsBody />
        </div>
        <div className="modal-actions">
          <button className="btn-ghost" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </div>
  )
}

function SettingsBody() {
  const [d, setD] = useState<Diagnostics | null>(null)
  const [err, setErr] = useState('')

  useEffect(() => {
    api
      .diagnostics()
      .then(setD)
      .catch((e) => setErr(String(e.message || e)))
  }, [])

  return (
    <>
          {err && <div className="set-bad">{err}</div>}
          {!d && !err && <p className="dim">Looking…</p>}
          {d && (
            <>
              <div className="set-facts">
                <div>
                  <span className="dim">grove</span> <span className="mono">{d.version}</span>
                </div>
                <div>
                  <span className="dim">platform</span> <span className="mono">{d.platform}</span>
                </div>
                <div className="set-dir">
                  <span className="dim">watching</span> <span className="mono">{d.dir}</span>
                </div>
              </div>

              <h3 className="set-h">Programs grove runs</h3>
              {d.tools.map((t) => (
                <div key={t.name} className={t.found ? 'set-tool' : 'set-tool set-tool-gone'}>
                  <div className="set-tool-head">
                    <span className={t.found ? 'set-mark set-ok' : 'set-mark set-no'}>
                      {t.found ? '✓' : '✕'}
                    </span>
                    <span className="mono set-name">{t.name}</span>
                    {t.version && <span className="mono dim">{t.version}</span>}
                    {t.required && <span className="set-tag">required</span>}
                    {!t.found && !t.required && <span className="set-tag set-tag-warn">not found</span>}
                  </div>
                  {t.path && <div className="set-path mono dim">{t.path}</div>}
                  <p className="set-what dim">{t.needed}</p>
                  {!t.found && t.missing && <p className="set-lost">Without it: {t.missing}</p>}
                </div>
              ))}

              <h3 className="set-h">GitHub checks</h3>
              <div className={d.github.working ? 'set-tool' : 'set-tool set-tool-gone'}>
                <div className="set-tool-head">
                  <span className={d.github.working ? 'set-mark set-ok' : 'set-mark set-no'}>
                    {d.github.working ? '✓' : '✕'}
                  </span>
                  <span className="set-name">
                    {d.github.working ? 'Working' : 'Not available'}
                  </span>
                  {d.github.from && <span className="mono dim">{d.github.from}</span>}
                </div>
                <p className="set-what dim">{d.github.detail}</p>
              </div>
            </>
          )}
    </>
  )
}
