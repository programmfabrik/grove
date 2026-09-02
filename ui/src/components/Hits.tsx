import { tokenRuns } from '../lib/filter'

// how many colours the filter terms cycle through — the same cycle in the
// filter input and in the lists it narrows
export const TOKEN_COLOURS = 6

// Hits marks where the filter's terms hit `text`, each term in its own colour
// — the colour the term wears in the filter input — so the eye can tell which
// word found what.
export function Hits({ text, terms }: { text: string; terms: string[] }) {
  if (!terms.length) return <>{text}</>
  return (
    <>
      {tokenRuns(text, terms).map((r, i) =>
        r.term < 0 ? (
          r.text
        ) : (
          <mark key={i} className={`hit hit-${r.term % TOKEN_COLOURS}`}>
            {r.text}
          </mark>
        ),
      )}
    </>
  )
}
