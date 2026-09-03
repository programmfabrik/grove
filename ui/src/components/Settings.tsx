import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import type { Diagnostics, Prefs } from '../types'
import { getThemePref, setThemePref, type ThemePref } from '../lib/theme'

// What grove is doing, and the parts of it you can turn off.
//
// Three of the things grove does reach off this machine — asking GitHub about
// your checks, keeping the open checkout's remotes current, and looking for a
// newer grove. Each is here as a switch, because "a tool that quietly talks to
// the network" is a thing to be able to say no to, and a flag you have to
// restart with is not saying no, it is starting again.
//
// Everything else here is what grove is standing on: the programs it runs,
// found or not, and what goes without them.
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
  const [prefs, setPrefs] = useState<Prefs | null>(null)
  const [theme, setThemeState] = useState<ThemePref>(getThemePref)
  const [err, setErr] = useState('')

  const load = useCallback(() => {
    api.diagnostics().then(setD).catch((e) => setErr(String(e.message || e)))
    api.prefs().then(setPrefs).catch(() => {})
  }, [])
  useEffect(load, [load])

  // `on` is what it IS, so the new stored negative is exactly that: on now
  // means off next, which means the no_ flag becomes true.
  const flip = (key: keyof Prefs, on: boolean) => {
    const next = { ...(prefs ?? {}), [key]: on }
    setPrefs(next)
    api
      .setPrefs(next)
      .then(() => load()) // the diagnostics change with them
      .catch((e) => setErr(String(e.message || e)))
  }

  const pickTheme = (t: ThemePref) => {
    setThemeState(t)
    setThemePref(t) // applies here; the other window hears it through storage
  }

  return (
    <>
      {err && <div className="set-bad">{err}</div>}
      {!d && !err && <p className="dim">Looking…</p>}
      {d && (
        <>
          <h3 className="set-h">Appearance</h3>
          <div className="set-row">
            <div className="set-row-text">
              <div className="set-row-name">Theme</div>
              <p className="set-what dim">
                System follows what the rest of the machine is doing, and changes with it.
              </p>
            </div>
            <div className="seg set-seg">
              {(['light', 'dark', 'system'] as ThemePref[]).map((t) => (
                <button key={t} className={theme === t ? 'active' : ''} onClick={() => pickTheme(t)}>
                  {t}
                </button>
              ))}
            </div>
          </div>

          <h3 className="set-h">GitHub checks</h3>
          <div className="set-row">
            <div className="set-row-text">
              <div className="set-row-name">
                Ask GitHub whether what you pushed is passing
                {d.github.working && !prefs?.no_checks && <span className="set-ok-tag">working</span>}
              </div>
              <p className="set-what dim">{d.github.detail}</p>
              {d.github.fix?.length ? (
                <div className="set-fix">
                  {d.github.fix.map((f, i) => {
                    const [cmd, ...rest] = f.split('  — ')
                    return (
                      <div key={i} className="set-fix-row">
                        <code>{cmd}</code>
                        {rest.length > 0 && <span className="dim">{rest.join(' — ')}</span>}
                      </div>
                    )
                  })}
                </div>
              ) : null}
              {d.github.alternative && <p className="set-what dim">{d.github.alternative}</p>}
              <p className="set-sub dim">
                <span className={d.github.tool.found ? 'set-ok' : 'set-no'}>
                  {d.github.tool.found ? '✓' : '✕'}
                </span>{' '}
                <span className="mono">gh</span>{' '}
                {d.github.tool.found ? (
                  <>
                    <span className="mono">{d.github.tool.version}</span>{' '}
                    <span className="mono set-path">{d.github.tool.path}</span>
                  </>
                ) : (
                  'not installed'
                )}
              </p>
            </div>
            <Switch on={!prefs?.no_checks} onFlip={(on) => flip('no_checks', on)} />
          </div>

          <h3 className="set-h">While a checkout is open</h3>
          <div className="set-row">
            <div className="set-row-text">
              <div className="set-row-name">Keep its remotes current</div>
              <p className="set-what dim">
                Fetches the open checkout and its submodules every few seconds, so ahead and behind
                mean something. Off, the numbers are as fresh as your last fetch and Push still
                fetches before it decides.
              </p>
            </div>
            <Switch on={!prefs?.no_auto_fetch} onFlip={(on) => flip('no_auto_fetch', on)} />
          </div>

          <h3 className="set-h">Updates</h3>
          <div className="set-row">
            <div className="set-row-text">
              <div className="set-row-name">Look for a newer grove</div>
              <p className="set-what dim">
                Once a day, an unauthenticated read of the public releases page. Nothing is sent and
                nothing is installed — the answer is a link.
              </p>
            </div>
            <Switch on={!prefs?.no_update_check} onFlip={(on) => flip('no_update_check', on)} />
          </div>

          <h3 className="set-h">This grove</h3>
          <div className="set-facts">
            <div>
              <span className="dim">version</span> <span className="mono">{d.version}</span>
            </div>
            <div>
              <span className="dim">platform</span> <span className="mono">{d.platform}</span>
            </div>
            <div>
              <span className="dim">git</span>{' '}
              <span className="mono">{d.tools[0]?.version}</span>{' '}
              <span className="mono set-path">{d.tools[0]?.path}</span>
            </div>
            <div className="set-dir">
              <span className="dim">watching</span> <span className="mono">{d.dir}</span>
            </div>
          </div>
          <p className="set-what dim">
            File → Open Folder… points grove at a different directory of repositories.
          </p>
        </>
      )}
    </>
  )
}

function Switch({ on, onFlip }: { on: boolean; onFlip: (on: boolean) => void }) {
  return (
    <button
      role="switch"
      aria-checked={on}
      className={on ? 'sw sw-on' : 'sw'}
      onClick={() => onFlip(on)}
      title={on ? 'on' : 'off'}
    >
      <span className="sw-knob" />
    </button>
  )
}
