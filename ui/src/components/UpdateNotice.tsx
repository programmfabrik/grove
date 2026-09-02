import type { Update } from '../types'

// A newer release exists. Grove says so and then gets out of the way: it
// downloads nothing and installs nothing, because replacing a running program
// on somebody's machine is not a thing a dashboard should do while they are
// reading a diff.
//
// A Homebrew install is not offered a file. Downloading one beside the managed
// copy would leave two, with brew still describing the one that is no longer
// current — so that case gets the command instead, on a click, to run where
// commands belong.
export function UpdateNotice({ update }: { update: Update }) {
  const { latest, url, notes_url, homebrew } = update
  if (homebrew) {
    const cmd = 'brew upgrade --cask grove'
    return (
      <button
        className="update-pill"
        title={`${latest} is out — copy the upgrade command`}
        onClick={() => navigator.clipboard?.writeText(cmd)}
      >
        <span className="update-dot" />
        {latest} · copy <code>{cmd}</code>
      </button>
    )
  }
  return (
    <a
      className="update-pill"
      href={url || notes_url}
      target="_blank"
      rel="noreferrer"
      title={`${latest} is out — download the file that replaces this build`}
    >
      <span className="update-dot" />
      {latest} available
    </a>
  )
}
