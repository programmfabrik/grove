import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import type { Diagnostics, Prefs, Program } from '../types'
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
  const [desktop, setDesktop] = useState(false)
  const [d, setD] = useState<Diagnostics | null>(null)
  const [prefs, setPrefs] = useState<Prefs | null>(null)
  const [programs, setPrograms] = useState<Program[]>([])
  const [login, setLogin] = useState<{ on: boolean; possible: boolean } | null>(null)
  const [theme, setThemeState] = useState<ThemePref>(getThemePref)
  const [err, setErr] = useState('')

  const load = useCallback(() => {
    api.diagnostics().then(setD).catch((e) => setErr(String(e.message || e)))
    api.prefs().then(setPrefs).catch(() => {})
    api.programs().then((r) => setPrograms(r.programs)).catch(() => {})
    api.loginItem().then(setLogin).catch(() => setLogin(null))
    api.version().then((v) => setDesktop(!!v.desktop)).catch(() => {})
  }, [])
  useEffect(load, [load])

  // `on` is what it IS, so the new stored negative is exactly that: on now
  // means off next, which means the no_ flag becomes true.
    const put = (patch: Prefs) => {
    const next = { ...(prefs ?? {}), ...patch }
    setPrefs(next)
    api.setPrefs(next).then(() => load()).catch((e) => setErr(String(e.message || e)))
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

          <h3 className="set-h">Programs</h3>
          <p className="set-what dim set-lead">
            grove does not bring its own. These are the ones it found here; anything it did not
            recognise is one <b>Choose an application…</b> away, because no list of programs is ever
            complete.
          </p>
          <Pick
            label="Browser"
            hint="Where a link to GitHub opens."
            kind="browser"
            programs={programs}
            value={prefs?.browser ?? ''}
            onPick={(id) => put({ browser: id })}
            onBrowse={() => api.chooseProgram('browser').then((r) => !r.cancelled && load())}
          />
          <Pick
            label="Terminal"
            hint="Opened at the checkout, from the button in its head."
            kind="terminal"
            programs={programs}
            value={prefs?.terminal ?? ''}
            onPick={(id) => put({ terminal: id })}
            onBrowse={() => api.chooseProgram('terminal').then((r) => !r.cancelled && load())}
          />
          <Pick
            label="Editor"
            hint="Right-click a file in the diff to open it — at the line you were reading, where the editor can be told one."
            kind="editor"
            programs={programs}
            value={prefs?.editor ?? ''}
            onPick={(id) => put({ editor: id })}
            onBrowse={() => api.chooseProgram('editor').then((r) => !r.cancelled && load())}
          />

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
            <Switch on={!prefs?.no_checks} onFlip={(on) => put({ no_checks: on })} />
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
            <Switch on={!prefs?.no_auto_fetch} onFlip={(on) => put({ no_auto_fetch: on })} />
          </div>

          <div className="set-row">
            <div className="set-row-text">
              <div className="set-row-name">Re-scan every</div>
              <p className="set-what dim">
                How stale a repository's worktrees may be before the next look at them redoes the
                scan. Each one costs a handful of git calls, which is why this is not a second.
              </p>
            </div>
            <select
              className="set-select"
              value={String(prefs?.refresh_seconds ?? 0)}
              onChange={(e) => put({ refresh_seconds: Number(e.target.value) })}
            >
              <option value="0">as started ({d.refresh_seconds}s)</option>
              <option value="5">5 seconds</option>
              <option value="10">10 seconds</option>
              <option value="20">20 seconds</option>
              <option value="60">1 minute</option>
              <option value="300">5 minutes</option>
            </select>
          </div>

          <h3 className="set-h">Telling you things</h3>
          <div className="set-row">
            <div className="set-row-text">
              <div className="set-row-name">Say when checks finish</div>
              <p className="set-what dim">
                A notification when a run that was going turns green or red, so an hour of checks is
                not an hour of looking at the column. Once per run, and nothing else is ever
                announced.
              </p>
            </div>
            <Switch on={!prefs?.no_notify} onFlip={(on) => put({ no_notify: on })} />
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
            <Switch on={!prefs?.no_update_check} onFlip={(on) => put({ no_update_check: on })} />
          </div>

          {login?.possible && (
            <div className="set-row">
              <div className="set-row-text">
                <div className="set-row-name">Start Grove at login</div>
                <p className="set-what dim">
                  Writes a LaunchAgent in your own Library. Switching this off deletes it again —
                  nothing is left behind in the login sequence.
                </p>
              </div>
              <Switch
                on={!!login.on}
                onFlip={(on) =>
                  api
                    .setLoginItem(!on)
                    .then((r) => setLogin({ on: r.on, possible: true }))
                    .catch((e) => setErr(String(e.message || e)))
                }
              />
            </div>
          )}

          <h3 className="set-h">Where grove is looking</h3>
          <div className="set-row">
            <div className="set-row-text">
              <div className="set-row-name mono set-dir-name">{d.dir}</div>
              {!!prefs?.recent?.length && (
                <div className="set-recent">
                  {prefs.recent
                    .filter((r) => r !== d.dir)
                    .map((r) => (
                      <button key={r} className="set-recent-row mono" onClick={() => api.useFolder(r).then(load)}>
                        {r}
                      </button>
                    ))}
                </div>
              )}
            </div>
            <button
              className="btn-ghost"
              onClick={() => api.chooseFolder().catch((e) => setErr(String(e.message || e)))}
            >
              Choose…
            </button>
          </div>

          {/* the read-only facts live in About now, under the application
              menu. A browser tab has no menu bar to put an About in, so it
              keeps them here rather than losing them. */}
          {!desktop && (
            <>
              <h3 className="set-h">This grove</h3>
              <div className="set-facts">
                <div>
                  <span className="dim">version</span> <span className="mono">{d.version}</span>
                </div>
                <div>
                  <span className="dim">platform</span> <span className="mono">{d.platform}</span>
                </div>
                <div>
                  <span className="dim">git</span> <span className="mono">{d.tools[0]?.version}</span>{' '}
                  <span className="mono set-path">{d.tools[0]?.path}</span>
                </div>
              </div>
            </>
          )}
        </>
      )}
    </>
  )
}

// One program of a kind, or whatever the machine would do on its own.
function Pick({
  label,
  hint,
  kind,
  programs,
  value,
  onPick,
  onBrowse,
}: {
  label: string
  hint: string
  kind: string
  programs: Program[]
  value: string
  onPick: (id: string) => void
  onBrowse: () => void
}) {
  const mine = programs.filter((p) => p.kind === kind)
  const chosen = mine.find((p) => p.id === value)
  return (
    <div className="set-row">
      <div className="set-row-text">
        <div className="set-row-name">{label}</div>
        <p className="set-what dim">{hint}</p>
        {chosen && (
          <p className="set-sub dim">
            <span className="mono set-path">{chosen.path}</span>
            {kind === 'editor' && !chosen.line && (
              <span className="set-tag set-tag-warn">opens the file, not the line</span>
            )}
          </p>
        )}
      </div>
      <select
        className="set-select"
        value={value}
        onChange={(e) => (e.target.value === '\u0000browse' ? onBrowse() : onPick(e.target.value))}
      >
        <option value="">System default</option>
        {mine.map((p) => (
          <option key={p.id} value={p.id}>
            {p.name}
          </option>
        ))}
        <option value={'\u0000browse'}>Choose an application…</option>
      </select>
    </div>
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
