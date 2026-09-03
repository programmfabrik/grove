// fmtAgo renders an RFC3339 stamp as a compact age ("3m ago", "2d ago").
export function fmtAgo(ts?: string): string {
  if (!ts) return '—'
  const t = new Date(ts).getTime()
  if (isNaN(t)) return ts
  const s = Math.max(0, Math.floor((Date.now() - t) / 1000))
  if (s < 45) return 'just now'
  const units: [number, string][] = [
    [365 * 86400, 'y'],
    [30 * 86400, 'mo'],
    [86400, 'd'],
    [3600, 'h'],
    [60, 'm'],
  ]
  for (const [span, label] of units) {
    if (s >= span) return `${Math.floor(s / span)}${label} ago`
  }
  return `${s}s ago`
}

export function fmtDateTime(ts?: string): string {
  if (!ts) return '—'
  const d = new Date(ts)
  if (isNaN(d.getTime())) return ts
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

// copy puts text on the clipboard, resolving false when the browser refuses
// (a non-secure origin without the async clipboard API).
export async function copy(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}

// fmtDur is how long something took, in the units a person would say it in.
export function fmtDur(ms: number): string {
  if (!isFinite(ms) || ms < 0) return ''
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return s % 60 ? `${m}m ${s % 60}s` : `${m}m`
  const h = Math.floor(m / 60)
  return m % 60 ? `${h}h ${m % 60}m` : `${h}h`
}
