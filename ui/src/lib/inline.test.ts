// Run by Node's own test runner, which strips the types itself — no bundler,
// no dependencies: `npm test`. Kept out of tsconfig for the same reason there
// is no @types/node to check it against.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { changedPart, escapeHtml, markHtml, pairMarks } from './inline.ts'

test('a removed trailing space is marked and drawn — the line from fylr3', () => {
  const was = '- **%fylr:uuid%**",TRUE,,FALSE,"Syötä … jakamisdialogissa. '
  const now = '- **%fylr:uuid%**",TRUE,,FALSE,"Syötä … jakamisdialogissa.'
  const pair = changedPart(was, now)
  assert.ok(pair)
  const [removed, added] = pair
  assert.equal(was.slice(removed.from, removed.to), ' ')
  assert.equal(added.to - added.from, 0, 'nothing to mark on the side that lost it')
  assert.ok(markHtml(escapeHtml(was), removed, was.length)!.endsWith('<span class="dl-chg"><span class="ws">·</span></span>'))
})

test('a one-word edit marks the word, a rewrite marks nothing', () => {
  const [a, b] = changedPart('return foo(bar)', 'return foo(baz)')!
  assert.deepEqual(['return foo(bar)'.slice(a.from, a.to), 'return foo(baz)'.slice(b.from, b.to)], ['r', 'z'])
  assert.equal(changedPart('completely one thing', 'another line entirely'), null)
})

test('invisible characters are drawn, the rare ones named', () => {
  const [cr] = changedPart('x = 1\r', 'x = 1')!
  assert.ok(markHtml(escapeHtml('x = 1\r'), cr, 6)!.includes('␍'))
  const [tab, spaces] = changedPart('\tgo()', '    go()')!
  assert.ok(markHtml('\tgo()', tab, 5)!.includes('→'))
  assert.equal(markHtml('    go()', spaces, 8)!.split('·').length - 1, 4)
  const [nbsp] = changedPart('a b', 'a b')!
  assert.ok(markHtml('a b', nbsp, 3)!.includes('title="U+00A0 no-break space"'))
})

test('the mark never straddles the highlighter’s own spans', () => {
  const html = '<span class="hljs-keyword">return</span> <span class="hljs-title">foo</span>(bar) '
  const text = 'return foo(bar) '
  const [m] = changedPart(text, 'return foo(bar)')!
  const out = markHtml(html, m, text.length)!
  assert.equal((out.match(/<span/g) || []).length, (out.match(/<\/span>/g) || []).length)
  assert.equal(
    markHtml('ab<span class="x">cd</span>ef', { from: 1, to: 5 }, 6),
    'a<span class="dl-chg">b</span><span class="x"><span class="dl-chg">cd</span></span><span class="dl-chg">e</span>f',
  )
})

test('an entity is one character, and markup that does not add up is refused', () => {
  assert.equal(markHtml(escapeHtml('a<b'), { from: 1, to: 2 }, 3), 'a<span class="dl-chg">&#60;</span>b')
  assert.equal(markHtml('abc', { from: 0, to: 1 }, 5), null)
})

test('an emoji is never cut in half', () => {
  const [m] = changedPart('x 😀', 'x 😃')!
  assert.equal('x 😀'.slice(m.from, m.to), '😀')
})

test('pairing stays inside one replacement', () => {
  const lines = [
    { kind: 'ctx', text: 'keep' },
    { kind: 'del', text: 'one ' },
    { kind: 'del', text: 'two\t' },
    { kind: 'add', text: 'one' },
    { kind: 'add', text: 'two' },
    { kind: 'ctx', text: 'keep' },
    { kind: 'add', text: 'new line' },
  ]
  assert.deepEqual([...pairMarks(lines).keys()].sort(), [1, 2, 3, 4])
})

// gofmt realigning a struct after a new, longer field: every old field must
// still find its new self, and the new field is not an edit of anything
test('a line inserted among edits does not shift the pairing', () => {
  const marks = pairMarks([
    { kind: 'del', text: '\tLimit        int' },
    { kind: 'del', text: '\tAdminEmail   []string' },
    { kind: 'del', text: '\tTagfilterSets OAIPMHSets' },
    { kind: 'add', text: '\tLimit                int' },
    { kind: 'add', text: '\tAdminEmail           []string' },
    { kind: 'add', text: '\tMetadataFormatEasydb bool' },
    { kind: 'add', text: '\tTagfilterSets        OAIPMHSets' },
  ])
  assert.deepEqual([...marks.keys()].sort(), [0, 1, 2, 3, 4, 6])
})

test('a removal with no counterpart takes no addition with it', () => {
  const marks = pairMarks([
    { kind: 'del', text: 'a completely different line' },
    { kind: 'del', text: 'x := 1 ' },
    { kind: 'add', text: 'x := 1' },
  ])
  assert.deepEqual([...marks.keys()].sort(), [1, 2])
})
