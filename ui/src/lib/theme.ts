// Theme preference (light / dark / system), stored per browser. The CSS knows
// only two palettes (:root and :root[data-theme='dark']); "system" is resolved
// here and re-resolved live when the OS scheme changes. index.html stamps the
// attribute pre-paint with the same key so there is no light flash.

export type ThemePref = 'light' | 'dark' | 'system'

const KEY = 'grove_theme'

export function getThemePref(): ThemePref {
  const v = localStorage.getItem(KEY)
  return v === 'light' || v === 'dark' ? v : 'system'
}

const media = window.matchMedia('(prefers-color-scheme: dark)')

function apply(pref: ThemePref) {
  const resolved = pref === 'system' ? (media.matches ? 'dark' : 'light') : pref
  document.documentElement.dataset.theme = resolved
}

export function setThemePref(pref: ThemePref) {
  if (pref === 'system') localStorage.removeItem(KEY)
  else localStorage.setItem(KEY, pref)
  apply(pref)
}

export function initTheme() {
  apply(getThemePref())
  media.addEventListener('change', () => {
    if (getThemePref() === 'system') apply('system')
  })
  // There are two windows now — the dashboard and the settings window — on one
  // origin, so a change made in either belongs in both. localStorage tells the
  // OTHER window and never the one that made the change, which is why apply()
  // is called directly there and only listened for here.
  window.addEventListener('storage', (e) => {
    if (e.key === null || e.key === KEY) apply(getThemePref())
  })
}
