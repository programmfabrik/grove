import { useState, type ReactNode } from 'react'
import { copy } from '../lib/format'

export function Logo() {
  return (
    <svg className="logo" viewBox="0 0 32 32" width="26" height="26" aria-hidden="true">
      <rect width="32" height="32" rx="7" fill="#ff5724" />
      <g stroke="#fff" strokeWidth="2.4" strokeLinecap="round" fill="none">
        <path d="M10 8v16" />
        <path d="M10 13h7a3 3 0 0 1 3 3v1" />
        <path d="M10 19h7" />
      </g>
      <g fill="#fff">
        <circle cx="10" cy="8" r="2.6" />
        <circle cx="22" cy="17" r="2.6" />
      </g>
    </svg>
  )
}

// Copy renders a value with a click-to-copy affordance — the detail panel is
// mostly paths and commands that want to end up in a terminal.
export function Copy({ text, children, title }: { text: string; children?: ReactNode; title?: string }) {
  const [done, setDone] = useState(false)
  return (
    <button
      className={done ? 'copy copied' : 'copy'}
      title={title ?? 'Copy'}
      onClick={async (e) => {
        e.stopPropagation()
        if (await copy(text)) {
          setDone(true)
          setTimeout(() => setDone(false), 1200)
        }
      }}
    >
      <span className="copy-text">{children ?? text}</span>
      <span className="copy-icon">{done ? '✓' : '⧉'}</span>
    </button>
  )
}
