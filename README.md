# grove

One page over a directory full of git repositories: every checkout, every
worktree, and what each of them changed, at http://localhost.

![grove: repos, worktrees, and the diff of one commit](docs/grove.png)

Three panes, each narrowing what the next one shows:

1. **repos** — every repository in the start directory, sorted by how much is
   checked out and then by when it was last worked in
2. **worktrees** — the checkouts of the selected repo: branch, distance from
   the base branch, uncommitted count
3. **viewer** — the selected worktree: what is known about it in the head, its
   diff below

grove reads git, and it never starts, stops or reconfigures anything. There are
two exceptions and they are both deliberate: the diff tree's context menu
writes — see [Unstage and discard](#unstage-and-discard) — and once a day it
asks GitHub whether there is a newer release, which is the only thing it does
over the network. See [Updates](#updates).

## Install

The app, on macOS:

```sh
brew tap programmfabrik/grove https://github.com/programmfabrik/grove
brew trust --cask programmfabrik/grove/grove
brew install --cask grove
xattr -dr com.apple.quarantine /Applications/Grove.app
```

Neither of the last two lines is optional. Homebrew refuses to load a cask from
a third-party tap until you say you trust it. And Homebrew quarantines what it
downloads exactly as a browser does — it does **not** avoid Gatekeeper — so
without the `xattr` line the first launch is a dialog saying macOS "could not
verify Grove.app is free of malware", whose buttons on current macOS are
**Done** and **Move to Bin**. There is no "Open Anyway" in it.

That is the price of an unsigned build, and it says nothing about the program.
Notarising it is the actual fix and is not done yet.

…or a download for macOS or Windows from
[the releases](https://github.com/programmfabrik/grove/releases), which the
[download page](https://programmfabrik.github.io/grove/) will pick for you —
including what to do about the warning an unsigned build earns on first run.

The command, anywhere Go runs:

```sh
go install github.com/programmfabrik/grove@latest
cd ~/src && grove          # http://localhost
```

macOS, Windows and Linux — CI builds and tests all three on every push.

It needs `git` on the path, nothing else, and says so and stops when it is
missing rather than serving a dashboard of empty panes. On Windows it also
looks where Git for Windows installs itself, since a program started from an
icon does not always inherit the PATH entry that puts git there.

Started with no arguments it takes the working directory, or its repo's parent
when started inside a checkout — so `cd myrepo && grove` lists everything next
to myrepo, not myrepo alone. `-dir` overrides.

**Where it listens.** Port 80 is what makes it `http://localhost`, and macOS
lets an unprivileged process bind it. Linux keeps everything under 1024 for
root and on Windows http.sys may already hold it, so when the default cannot
be had grove moves to `127.0.0.1:7433` and prints where it went. An explicit
`-addr` is taken literally: you named a port, and quietly using a different one
would be a lie about where the dashboard is.

It binds the loopback interface and not the wildcard. grove reads every
repository on the machine and has one endpoint that deletes untracked files, so
a dashboard the rest of the network can reach is not a default anybody asked
for. `-addr :80` restores it for whoever wants it.

| flag | default | |
| --- | --- | --- |
| `-addr` | `127.0.0.1:80`, or `127.0.0.1:7433` when that is taken | listen address |
| `-dir` | the working directory, or its repo's parent | the directory holding the repositories |
| `-base` | the branch of the main checkout | what ahead/behind and the branch diff compare against |
| `-refresh` | `20s` | how often the worktrees are re-scanned |
| `-open` | off | open the browser once it listens |

## Grove, the app

The same dashboard, in a window of its own rather than a browser tab. The
command you type is `grove`; the application is **Grove**, a proper noun like
every other name in a menu bar — which is also why the binary is capitalised,
since an unbundled program is named in the menu bar by its executable.

```sh
make app          # bin/Grove.app on macOS, bin/app/Grove elsewhere
make app-windows  # bin/app/Grove.exe, cross-built from anywhere
open bin/Grove.app
```

The desktop build lands in `bin/app/` and the command stays in `bin/`, because
`Grove` and `grove` are the same file on a case-insensitive filesystem — which
macOS is by default, and so are its CI runners. Built beside each other they
overwrite each other in silence, and what ships is whichever ran first.

On macOS the executable is wrapped in a bundle, because a bare executable is
not an application there: launched from a shell it dies with that shell, it
has no icon, and the menu bar names it after the file. `Grove.icns` is
committed rather than converted during the build — the same bargain `ui/dist`
strikes, one less tool to have installed.

macOS and Windows. Linux gets the command above and a browser for now — its
webview wants cgo and a webkitgtk whose version differs per distribution, and
that is a worse trade than a tab.

The window loads `http://127.0.0.1:PORT` rather than being served through the
webview's own asset scheme, and that is the whole design. The page then sits on
an ordinary HTTP origin, so everything it already does keeps working: range
requests, which is how `<video>` in a diff seeks; a secure context, which is
where `navigator.clipboard` lives; ordinary caching. An asset scheme would have
put all three in doubt. The cost is that page and window share no JavaScript
bridge — less of a loss than it sounds, because the page already talks to Go
over HTTP, and everything native is driven from the Go side.

The port is a stable one (7433, or the next free after it) rather than whatever
the OS offers. localStorage is keyed by origin, and it is where the pane widths,
the folds and the theme are kept, so a port that moved every launch would hand
back an empty dashboard each time.

**File → Open Folder…** points it at another directory of repositories, which
is what an app needs and a command does not: a command is started in the
directory it is meant to show, and an app is started from an icon — in `/`,
where there is nothing to show and never will be. So the app writes the
directory down (`settings.go`, in Application Support / `%AppData%` /
`$XDG_CONFIG_HOME`) and opens there next time. The working directory still
wins when it actually holds repositories, which is what a launch from a
terminal means, and an explicit `-dir` beats both without being recorded — it
was a one-off instruction rather than a change of mind.

**The first run has nothing to remember**, and the answer is to ask rather than
to guess. Grove opens the folder chooser, pointed at wherever checkouts
actually seem to be: `~/src`, `~/Projects`, `~/code`, `~/go/src/github.com` and
a few more, in order, the first one that *holds* a repository rather than the
first name that exists — and the home directory if none of them do, since that
is a better place to start looking than the root of the disk. It suggests; it
does not decide. Opening somewhere unexpected without being asked is its own
kind of rude, and a window that opens on `/`, finds nothing and says nothing is
the worst first impression grove could make.

Cancel the dialog and the window says what it found and what to do about it,
rather than showing an empty column that claims to be loading.

Both binaries hold the same server and the same dashboard; only the front door
differs (`front_cli.go`, `front_desktop.go`). The default build has no window
code in it at all — no cgo, no webview, not one Wails package — so `go install`
still produces the plain command.

## What it discovers, and how

| shown | where it comes from |
| --- | --- |
| the repo list | the top level of the start directory, entries with a `.git`; each resolved through `git rev-parse --git-common-dir` so a linked worktree is not listed as a repo of its own |
| a repo's worktrees | `git worktree list --porcelain` on the repo — which reports them wherever they live, a sibling directory of worktrees included. The main checkout comes first, the rest in natural order of their names (`wt2` before `wt10`) |
| last used | the newest of the last commit and `.git/HEAD`'s mtime. **Not** `.git`'s mtime: reading a repo touches that, so a dashboard that scans every repo would make them all look freshly used |
| branch | `git symbolic-ref HEAD` |
| ahead / behind / dirty | `git rev-list --left-right --count <base>...HEAD`, `git status --porcelain -uall` |
| the base branch | the branch checked out in the repo's main checkout — see below |
| last commit | `git log -1` |
| a checkout's submodules | `.gitmodules`, followed into nested submodules; a submodule directory without a `.git` was never initialised and is skipped |

Scans are lazy and per repo: with dozens of repositories in the directory,
refreshing all of them on a timer would be waste, so a repo is scanned when a
request finds its cache stale (`-refresh`).

Every pane refreshes itself, the viewer included — commit or edit in a terminal
and the diff follows without a click. A refresh never disturbs what you are
doing: the selection survives, state is replaced only where the data actually
differs (so identical diff text never re-renders under the cursor), and each
list scrolls its active row into view once per *selection*, not once per
refresh.

### The base branch

Nothing compares against a hardcoded `main`. The base is **the branch checked
out in the main repo** — the checkout that holds the real `.git`. Put a release
branch there and every ahead/behind number and every diff follows it, with no
flag to remember. It falls back to the remote's default branch
(`origin/HEAD`) and then to `main` for a checkout that has neither, and `-base`
overrides it.

A submodule is a repository with a history of its own, so its section compares
against its own remote's default branch instead.

## The viewer

The third pane. Its head holds what is known about the checkout — name,
branch, distance from the base, path, head commit — and the diff sits under
it. Every vertical separator in the window is
draggable and each width is remembered across reloads; double-click a
separator to put it back where it started.

Every column left of the diff — repos, worktrees, scope, files — also folds,
through the `‹` in its header. A folded column stays on screen as a 24px rail
with its name, so it is one click back rather than a pane you have to remember
existed, and the fold is remembered across reloads like the widths are. Fold
all four and the diff has the whole window.

### diff

Three columns: **what** to diff, **which** file, and the diff.

The scope column is the left one, and it is per repo — each has its own
branch, its own upstream and its own commits, so the repos are its section
headlines (`scope.go`). A checkout that carries submodules is several
repositories, and each checked-out one, nested ones included, gets a section
of its own, named after its directory.

| scope | what it shows |
| --- | --- |
| `vs <base>` | everything the checkout holds that the base branch does not, committed and uncommitted together. Omitted in a checkout that *is* the base branch — it would compare to itself. |
| `vs <upstream>` | the same against the branch's upstream: what is not pushed yet plus what is not committed yet. Omitted for a branch with no upstream. |
| `staged` | the index against HEAD |
| `unstaged` | the worktree against the index, untracked files included |
| one commit | that commit alone, with a dot for how far it has travelled |

A commit's dot is a ladder, not two independent flags — each rung is a weaker
claim on the commit, so the strongest true one wins and one dot says it all.
The scope header, which has room the list row does not, spells the state out
next to the sha:

| dot | meaning |
| --- | --- |
| amber, `unpushed` | no remote has it yet — it is still yours to rewrite |
| blue, `not in <base>` | pushed, but the base branch does not have it: the branch you are still working on, told apart from the history it sits on |
| grey | it has landed in the base branch and is just history now |

![the scope column: a branch two commits ahead, one of them pushed, one not, the rest history](docs/scope.png)

Merged-ness is `git cherry <base> HEAD`, not ancestry: landing work
elsewhere rewrites shas, and a commit cherry-picked or rebased onto the base
keeps its patch while losing its identity — ancestry would call it unmerged
forever. cherry compares patch-ids, so it recognises the copy. Two edges stay:
commits with byte-identical diffs share a patch-id and one can vouch for the
other, and a squash merge produces a patch no single branch commit matches, so
a squashed branch keeps showing as `not in <base>`.

**unmerged only**, in the scope column's filter, drops the grey commits —
the ones the base branch already has — so a branch on top of a long main reads as
its own four commits rather than the twenty on screen. The range and index
scopes stay, and a section says how many commits it hid. Remembered, like
`ignore comments`.

Over the file tree and the diff — the two columns that show the scope, not the
one that offers alternatives — stands its header: for a commit its subject,
sha, author and date, with the message body one click below (`▸ message`); for
a range or the index the scope and what it holds. The window's own head above
stays on the checkout: name, branch, distance from the base, path and head
commit. The selected
file's name is not up there — a selection can be sixty files, and each already
carries its own header in the viewer.

**ignore comments**, in the same filter, drops changes whose lines are all comments —
git does the work through `-I<regex>`, so a hunk that only rewords a comment
disappears, and a file with nothing else left disappears with it. Note that
`--name-status` does *not* honour `-I` while `--numstat` does, which is why the
file list is filtered against the stats. Block-comment continuation lines
(`  * more text`) are deliberately not matched: they are indistinguishable from
a Markdown bullet, and hiding a real change would be worse than showing a
comment. The setting is remembered.

Every scope carries its file count and `+`/`−`, so the column doubles as the
overview: whether there is anything staged, how big the branch is, which
commits are still yours to rewrite.

Within the two range scopes, committed and uncommitted work are not separate
views: a file is listed once and marked with where its change lives, because
"what is in this worktree" is the question and whether a line is committed yet
is a detail of it.

| marker | meaning |
| --- | --- |
| green dot | committed on this branch |
| amber dot | uncommitted |
| both dots | committed, and modified since |
| grey dot | committed, and the base branch already has the change |

A file can be in the range and on the base branch at the same time. The range
is measured from the fork point, so a change the base picked up through
another commit — one lifted into a review commit, say — keeps the file in the
range, while `git cherry` keeps the commit `not in <base>` because it compares
whole commits. So `vs <base>` asks the content instead: `git merge-tree`
merges the checkout onto the base in memory, and a committed file that merge
would not change on the base is already there. It gets the grey dot, and
**unmerged only** hides it along with the landed commits. Uncommitted work is
never on the base and is never marked.

Every column has a **filter** folded under its header — click the header to
open it, click again to close it, and whether it is open is remembered like
the widths and the folds are. Repos filter by name and branch, worktrees by
name, branch and last subject, the scope column by subject, sha and
author (and it holds the two switches above), the files column by path. Each is
a filter, not a search: it narrows the list already on screen, so it answers as
fast as you can type and never changes what the scope holds. Closing a filter
clears its text; the switches keep their setting.

![two filters open: "main" on the worktrees, "ui list" on the files, each term in its colour](docs/filters.png)

The query is whitespace-separated
terms, all of which must match, in any order, case-insensitively, anywhere in
the path — order-independent because a path is remembered in pieces, and
`yml workflows` is the same thought as `workflows release`. Matching the whole
path rather than the file name means a term can name a directory, and one that
does keeps everything under it without a rule of its own.

The tree opens itself down to every match. Folds are kept in two sets for it:
the tree's own, and the ones made while a filter is on. Both halves have to
hold at once — a fold left over from before must not hide what you just
searched for, and a fold you make while reading the results is a deliberate act
that has to stick. One set cannot do both, because a filtered tree is nothing
but ancestors of matches: a rule that keeps matches visible would undo every
fold you made inside it. So a filter starts with nothing folded, folds made
while it is on go to their own set, and clearing it hands back the tree you
left. The matched letters are marked in every list, each term in a colour of
its own — the colour the term wears in the filter input, so the eye can tell
which word found what. None of them is the accent: the selected row is already
accent-tinted, and a hit marked in accent would vanish exactly where it is
being looked for. Directory rows re-sum their
`+`/`−` over what survived, so a row never reports a total for files that are
not under it any more. The head counts `shown / total`, and `esc` or the `×`
clears.

The filter narrows the tree, never the selection: the diffs you are reading
stay open while you look for the next file. It survives a change of scope —
following one file from commit to commit is the point — and resets when you
change checkout.

A diff shows three lines of context and hides the rest, which is the wrong
amount the moment the question is what the code around a change looks like. So
each hunk header is an **expander**: `↑N` pulls in the next 20 of the N hidden
lines above it, repeatable until the gap closes, and a `↓` below the last hunk
walks to the end of the file. The lines come from `/api/lines`, which reads any
range of the file at the scope's own revision — the same resolution the media
preview uses, so an expanded line and a preview never disagree about which
revision is on screen. Revealed lines carry both line numbers and sit on a
quieter background than the diff itself. Once a gap is fully revealed its
header row goes away, so the lines read as the contiguous stretch they are.

**Syntax colours.** A diff is not a file — a block comment can open in one
hunk and close in the next, and a tokenizer fed one hunk at a time gets that
wrong — so each side of a file is highlighted whole, at the scope's own
revision, and every diff line looks its colours up by line number: a removed
line on the before side, everything else on the after side, the expanded
context included. highlight.js does the colouring in the browser, loaded on
first use in a chunk of its own, with a fixed set of languages chosen by file
name (`ui/src/lib/highlight.ts`); a file outside the set stays plain, and so
does a file over 400 KB. The palette follows the two GitHub palettes the diff
colours already come from, one per theme.

**Markdown, raw or rendered.** A `.md` file's section header carries a
`raw | rendered` switch. Rendered shows before and after side by side, the
same two panes the image preview uses, one pane where the change added or
removed the file; GitHub-flavoured through marked, fenced code through the
same highlighter, and everything through DOMPurify before it touches the page
— the file comes out of somebody's repository and this page has a write
endpoint. A relative image resolves through the blob endpoint at the pane's
own side, so a screenshot the change added shows on the right and not on the
left. The last choice is the default for the next file; a file switched on its
own keeps its choice for the session.

The two rendered pages are compared as well. Their blocks — paragraphs,
headings, list items, table rows, code — are matched by text; a block only one
side has is tinted whole, and a removed block facing an added one is matched
again word by word, so a reworded sentence shows the words that moved and not
the paragraph, in the line diff's own green and red. YAML front matter is
shown as metadata, small and dim, rather than as the heading markdown would
make of it.

A rendered page is the whole file, not the neighbourhood of a change, so the
changes can sit anywhere in it: a strip over the two panes counts them and
walks them, `↑` and `↓`, wrapping at either end. The stop flashes so the eye
finds it, and since a change can stand on both sides at once, the pane scrolls
to whichever of the pair sits higher. An image the browser cannot fetch — a
badge from a private repository, a path that resolves on one side only —
reads as its own alt text in a dashed box rather than as a broken icon.

![a README rendered before and after, side by side](docs/markdown.png)

The same marker heads the diff itself. One git command covers every range
case: the fork point (`git merge-base <base> HEAD`) against the working tree —
so a file that is partly committed and partly not reads as one diff. Untracked
files are in no commit and no index, so they are diffed against `/dev/null` and
read as all additions.

Every scope resolves to one `scopeSpec`, and the file list, the per-file diff
and the media preview all read that struct — a new scope is a case in
`resolveScope` and nothing else.

**Files a browser can render, it renders.** A diff of a jpg is "Binary files …
differ", which tells you nothing, and a repository full of test fixtures
changes images as often as code. So images, SVG, PDF, video and audio show as
the file itself — before and after side by side where both exist, one pane
where the change added or removed it (`preview.go`, `Preview.tsx`). An SVG gets
both: the rendered image and its text diff underneath. Both sides resolve
through the same scope, so "before" means the fork point in a range, HEAD for
the index, and the commit's parent for a commit. The bytes come from
`/api/blob`, which serves the working tree from disk (so `<video>` can seek
through range requests) and any other revision out of the object store via
`git show`. It refuses any path outside the checkout and any extension not on
the renderable list — it is a preview endpoint, not a file reader.

**Selecting more than one file.** cmd/ctrl-click adds or removes, shift-click
takes a range in render order, and a click on a directory takes everything
under it (its chevron folds instead). The viewer then shows one collapsible
section per file, headed the same way the scope column heads its repos. Past
eight files the sections open collapsed and fetch their diff when opened —
selecting a directory of sixty files must not fire sixty diffs nobody asked to
read. The repo sections in the scope column fold the same way.

The file tree is the middle column. Rows carry inline SVG icons on
`currentColor` — a branch glyph for the repo root, open/closed folders, a
document for files — so they theme with the row and need no asset. Every row
carries its `+`/`−` counts, summed upwards, so a directory says how much
changed beneath it before it is opened. Directory chains with a single child
collapse into one row, which is what keeps a 135-file tree readable; a scope
opens on its first file in tree order. One file's diff is capped at 400 KB.

### Unstage and discard

Right-click in the tree. The actions follow git's own model, the way GitHub
Desktop presents it, and apply to the whole selection:

| the file is | offered | what it runs |
| --- | --- | --- |
| staged | **Unstage** | `git restore --staged` — the index goes back to HEAD, the working tree is untouched |
| uncommitted | **Discard changes** | `git restore --worktree` — the working tree goes back to the index |
| uncommitted **and** untracked | **Discard changes** | the file is *deleted*: it is in no commit and no index, so git cannot bring it back |
| committed only | nothing | undoing that is a rebase or a revert commit, not a file operation |

A mixed selection offers each action for the files it applies to. Every action
goes through a dialog that names the files first, and spells out the deletions
separately from the restores. `/api/revert` is the only endpoint that writes,
it takes only paths inside the checkout, and it never touches a commit.

## The URL

The view lives in the URL fragment, so a reload comes back to where you were
and the address bar is a copyable link to "that file, in that scope, of that
worktree":

```
#repo=myrepo&wt=myrepo3&sub=myrepo&scope=commit:81b535fb&file=cmd/main.go
```

The app owns the fragment and writes it whole from its state; each pane reads
its opening value from it once and then the clicks own the state. A value that
no longer exists — a deleted worktree, a rebased commit — is dropped for the
pane's own default rather than leaving the page empty. Every list scrolls its
restored row into view, with `block: 'nearest'` so a row already on screen is
left where it is and clicking never yanks the list.

## Pushing

The checkout's head carries **Push**, and the two honest answers for when it
cannot.

Push always fetches first — and fetches *every* repository under the checkout,
not only the one being pushed, because whether the parent may push depends on
what the submodules' remotes hold and that cannot be answered with stale refs.
Then it decides again, against what it just learned rather than against
whatever the page was showing when you clicked. It refuses four cases:

| | |
| --- | --- |
| detached HEAD | there is no branch to push |
| the remote has moved | pushing would fail anyway — **Fetch & rebase** and **Fetch & merge** appear in its place |
| nothing to push | being ahead by nothing is a no-op, not an error |
| an unknown submodule commit | see below |

Never `--force`, never a lease, never a branch it invented: it pushes the
branch that is checked out to the upstream it already has, and `--set-upstream`
only when there is none. A rebase or merge that would conflict stops and is
left to a terminal, which is where a conflict belongs.

**The unknown submodule commit** is the one worth having. A checkout carrying
submodules is several repositories, and the parent records a specific commit
for each. If that commit is on no remote branch of the submodule, pushing the
parent publishes a pointer nobody else can follow: they fetch the parent, git
asks the submodule's remote for that id, and nobody has it. So the parent is
refused until the submodule is pushed — and the submodule itself is never
refused for this, because pushing it is the cure. Selecting both pushes the
submodule first for the same reason.

**One repository at a time.** The caret beside each button chooses which, and
the button says which branch it would move — `Push main`. A checkout carrying
submodules is several repositories with separate remotes and separate
standings, and "push these three" is one button hiding three different answers
to whether it is even allowed. The submodules nest, too, and the flat list does
not show it: `easydb-library` is declared by `easydb-webfrontend`, which is
declared by `fylr`, and only `easydb-webfrontend` records a commit for it — so
that is the repository an unknown id stops. Where a repository has more than
one remote, the same popover chooses it.

**Pull asks how**, in a dialog rather than behind a caret, because the three
ways in are not interchangeable and their names do not say how they differ.
Each is explained in a sentence, only the possible ones are offered — a
fast-forward is not offered when you have commits of your own on top — and one
is marked *suggested* when the situation has a right answer:

| where you stand | what grove suggests |
| --- | --- |
| nothing of yours on top | **fast-forward** — one right answer, no history rewritten, no merge commit for work that does not exist |
| your commits on top | **rebase**, while they are still yours alone. Merge is right once anybody else has them, and grove cannot tell which, so it says so rather than pretending |

**Uncommitted work does not stop any of it.** A merge or a fast-forward only
fails when what is coming in touches a file you have edited, and git says so
plainly when it does — being told "commit first" while holding an edit to an
unrelated file is a wrong answer. A rebase genuinely does want a clean tree, so
grove gets one: the changes are stashed, the rebase runs, the stash is popped
back on top, and the dialog says as much before you agree to it.

A stash cannot clean a submodule sitting at a different commit than the parent
records: git stashes the recorded pointer and leaves the submodule's own
checkout alone, so the tree stays modified and a rebase will not start. That is
not a reason to refuse either. The submodules are moved to what their parents
record, the rebase runs, and each goes back exactly where it was — on the
branch it was on if it was on one. Nothing is lost by the trip: every commit
stays in its own repository's object store.

**At every depth, outermost first.** A submodule of a submodule is out of
position only relative to the commit *its* parent is on, so moving the outer
one changes what the inner one should be. Moving only the top level is not
enough and moving bottom-up is wrong: fylr's `easydb-webfrontend` came back at
the commit fylr records and still reported ` M coffeescript-ui`, because
nothing had told `coffeescript-ui` where the *new* `easydb-webfrontend` wanted
it. `git submodule update --checkout --recursive` does it in the right order,
and the restore afterwards runs outermost-first for the same reason.

The one submodule grove will not move is one holding uncommitted work of its
own. Moving it would mean overwriting somebody's edits to get a rebase through,
and no rebase is worth that, so it is named and the rebase is not started.

That sequence has three steps that can fail, and a half-finished one is worse
than not having started — a branch rebased under you and a stash to reconcile
by hand is not a state anybody asked to be in. So every step is followed by the
one that undoes it: a rebase that stops is aborted and the changes handed back;
a stash that will not reapply undoes the rebase too, putting HEAD and the
working tree exactly where they were. There are two outcomes and no third. The
one thing that is never done is discarding the changes — if they will not come
back cleanly they stay in the stash and the message says so.

**A push or a pull happens in front of you.** They take seconds and sometimes
tens of them, and a button reading "Pulling…" for ten of them is
indistinguishable from one that has hung — so a dialog shows each git command
as it runs and what it said. It is a transcript rather than a spinner on
purpose: everything grove does to a repository is a command somebody could have
typed, and showing which ones, in order, is the difference between a tool you
trust with your work and one you hope about.

**When something does not go through**, the transcript is already the
explanation, and grove's own reading of it is added underneath rather than
under the buttons — a line of red in the head pushes the layout around and still has
room for none of what happened. The dialog carries grove's explanation where it
recognises the failure, and git's own words either way, because git explains
itself better than any paraphrase and the person reading has to act on it. A
failure that carried nothing but `exit status 1` was a bug of exactly this
kind: `cmd.Output()` keeps stderr on the error and throws it away everywhere
else, so the reason git gave never reached the screen.

**Fetching happens on its own** while a checkout is open, which is what lets
any of these numbers mean anything. Every five seconds is the aim and not
always the outcome: a round over four repositories on a real remote measured
eight seconds here, and something that takes eight seconds cannot happen every
five — it would run back to back for as long as the window is open, thousands
of connections an hour to somebody else's server. So the next round waits twice
however long the last one took. Quick remotes get the five seconds; slow ones
settle at spending a third of the time fetching and the rest leaving the remote
alone. A window nobody is looking at fetches nothing.

## Is what you pushed passing?

The **branch name** carries the colour: green if GitHub's checks on the pushed
commit passed, amber while they run, red if any failed. The branch takes it
rather than a dot beside it, because the branch is what the state is *about* —
a coloured dot on its own says a state exists without saying whose. The worst
state wins, since the colour has room for one word and a green one beside a
failed job is the lie a dashboard exists to prevent.

The right of the same line says **when, and how long**: `running 21m 12s` while
they go, `17h ago · 1h 7m` once they are done — because green is worth knowing
and green four days ago is worth knowing something else about. A dot sits at
the end of it for the colour-blind case and for a glance down the column.

Clicking that text opens every run behind it, each with its own state and how
long it took, and **Open in GitHub** goes to the commit's checks page. A window
is not a browser — no tabs, no address bar — so in the app the link is handed
to the browser you are already signed in to rather than opened in a webview
with no way out of it.

It asks about the commit the **remote** has, not the one on disk — "is what I
pushed being tested" is the question, and a commit nobody has seen is not being
tested by anybody. A checkout with no upstream has no dot, and neither does one
whose commit GitHub has never run anything for: nothing drawn means nothing to
say, which is honest, where a grey dot would claim GitHub had answered.

This is the one thing grove does not read from git, because git does not know
it — a check run lives on GitHub and nowhere else. It never asks you for a
credential. It looks for one you already have, in this order:

| | |
| --- | --- |
| `gh auth token` | whatever `gh` is signed in as |
| `GITHUB_TOKEN`, `GH_TOKEN` | if either is set |
| the git credential helper | what it already holds for github.com |

If none of them answers, the column stays empty and **Settings** says so along
with what would fix it. Nothing is ever written, no scope is requested, and the
only calls made are for commits that are already pushed.

## Settings

**⌘,** in the app, under Grove in the menu bar — where a Mac user looks for it,
and where no button in a web toolbar will make them look instead. It opens a
window of its own rather than a panel inside the dashboard. Elsewhere it is
under File, on Ctrl-,. In a browser there is no menu bar to put it in, so the
top bar keeps a button.

The contents are still the page. There is no native form toolkit here, and
pretending otherwise would mean two settings screens to keep in step; what is
the operating system's is the window, the menu item and the shortcut, which is
what "where do I find settings" is actually asking about.

Three of the things grove does reach off this machine — asking GitHub about
your checks, keeping the open checkout's remotes current, and looking for a
newer grove — and each is a switch here. A tool that quietly talks to the
network is a thing to be able to say no to, and a flag you have to restart with
is not saying no, it is starting again. The switches take effect where they are
flipped.

The theme lives here too, and follows into the other window: both are one
origin, so a change in either is a change in both.

And what grove is standing on. It runs other programs, and when one is absent a
feature quietly does not happen — the check column stays empty and nobody is
told whether that means "nothing is running" or "I could not ask". The GitHub
row says which, in one place rather than two: whether `gh` is installed and at
what version, whether a credential was found and where from, and — when there
is none — the exact lines to type, `brew install gh` and `gh auth login` on a
Mac, with GITHUB_TOKEN and the git credential helper named as the alternatives.
A credential that IS found is proved by using it, since a token that is present
and refused looks exactly like one that works until something asks.

## What the loopback port is, and is not

grove binds the loopback interface, so nothing outside the machine can reach
it. That keeps it off the network and does not keep it to your browser: every
page in every tab can make requests to `127.0.0.1`, and `/api/revert` deletes
files. Two attacks followed from that, and both worked:

A page on any site could POST to `/api/revert`. Cross-origin JSON would have
been stopped by the CORS preflight, but `Content-Type: text/plain` makes the
request "simple" and no preflight is asked for — the browser refuses to show
the attacker the *response* and sends the request anyway, which is all that is
needed when the effect is a deletion.

And DNS rebinding: a name the attacker controls, answering first with their own
address and then with `127.0.0.1`, makes their page same-origin with the
dashboard and lets it read every repository on the machine. The `Host` header
is the only thing that tells the two apart, and grove served any.

So `guard.go` asks three questions of every request, in order:

| | |
| --- | --- |
| the `Host` | must be a loopback name — a name that resolved here came through DNS, which loopback cannot do |
| `Sec-Fetch-Site` and `Origin` | set by the browser and unforgeable by the page: a request that says it came from another site is refused. A loopback origin on another port is the vite dev server and is allowed |
| a write | must carry a per-launch secret, in a `SameSite=Strict` cookie no cross-site context is ever handed, and must be `application/json`, which is not a content type a form can send |

`-addr :80` binds the wildcard deliberately, and then the `Host` and `Origin` of
a legitimate request are whatever address the browser used, so those two checks
are dropped rather than refusing the thing the flag exists to allow. The secret
and the content type still stand.

None of this defends against another program running as you. It cannot: that
program can read the repositories directly, and grove's endpoints are the long
way round. What it defends is the browser, which runs code from strangers all
day long.

## Updates

A tool downloaded once is a tool that stays at the version you downloaded.
Homebrew can upgrade what it installed and nothing can upgrade a zip, so once a
day grove asks GitHub what the latest release is, and says so in the top bar if
it is newer than what is running.

It sends nothing. It is an unauthenticated GET of a public URL with no
identifier attached — GitHub sees an address and a user agent, exactly as it
would for anyone opening the releases page. Nothing is downloaded and nothing is
installed: the answer is a link to the one file that replaces this build, chosen
for the platform and for whether this is the app or the command. An install
Homebrew manages is offered `brew upgrade --cask grove` instead, because a
download beside the managed copy would leave two of them with brew describing
the wrong one.

`-no-update-check` turns it off, and then grove makes no network request at
all. A build from a working tree carries no version, so it never checks and
never nags.

Replacing a running program while somebody is reading a diff is not something a
dashboard should do, so grove does not: it points at the file and gets out of
the way.

## Development

```sh
make build        # rebuild ui/dist if the UI sources changed, then bin/grove
make run          # …and start it (ADDR=127.0.0.1:8000 to pick the address)
make app          # the desktop build instead: bin/Grove
make test         # go vet and the tests
```

`ui/dist` is committed, so `go install` and a plain `go build` work without
node. Change something under `ui/` and `make ui` rebuilds the bundle — decided
by a content hash over the sources, so it is a no-op when nothing changed —
and the rebuilt `ui/dist` goes into the same commit as the change. CI builds
the UI from source and warns when the committed bundle differs.

The tests need git on the path and build real repositories in a temp
directory: `go test ./...` covers the path spellings, the natural sort, the two
path guards that stand between the browser and git, and one pass over every
endpoint the dashboard calls — against a checkout with a committed, a staged
and an untracked file, since an untracked file is diffed against `/dev/null`,
a name Windows does not have and git special-cases anyway.

For UI work run the Go side and vite side by side; vite proxies `/api` to
`localhost:80` (`ui/vite.config.ts`):

```sh
./bin/grove &
cd ui && npm run dev
```

| file | |
| --- | --- |
| `main.go` | flags, the per-repo caches, `/api/repos`, `/api/state` |
| `repos.go` | the repo scan, the sort, and what "last used" means |
| `discover.go` | worktree discovery and the per-checkout git facts |
| `scope.go` | `/api/scopes` — what can be diffed, per repo |
| `diff.go` | `/api/diff` — the submodules a checkout spans, a scope's changed files, and one file's diff |
| `lines.go` | `/api/lines` — the unchanged lines the hunk expanders pull in, and whole files for the colouring and the rendering |
| `preview.go` | `/api/blob` — bytes of a renderable file, at either revision |
| `revert.go` | `/api/revert` — unstage and discard |
| `remote.go` | `/api/remote` — where each repository stands with its remote, and push, fetch, rebase, merge |
| `job.go` | `/api/run` — a push or a pull as a transcript the page can watch while it runs |
| `checks.go` | `/api/checks` — whether GitHub is testing what each checkout pushed |
| `diagnostics.go` | `/api/diagnostics` — what grove is standing on, and what it is missing |
| `paths.go` | the one spelling a path is kept in — see below |
| `front_cli.go` | the default front door: serve it and say where it is |
| `front_desktop.go` | `-tags desktop` — the window, its menu and the folder dialog |
| `settings.go` | the one thing the app remembers and the command never needs: where to look |
| `packaging/macos/` | the bundle's Info.plist and icon |
| `ui.go` | the embedded SPA |
| `ui/` | vite + React 18 + TypeScript; highlight.js, marked and DOMPurify in lazy chunks |
| `ui/src/components/Sidebar.tsx` | the checkout's head, and the diff under it |
| `ui/src/components/DiffTab.tsx` | file selection and diff rendering |
| `ui/src/components/DiffTree.tsx` | the flat file list turned into a per-repo tree |
| `ui/src/components/FileDiff.tsx` | one file's diff, its expanders and its preview |
| `ui/src/components/Markdown.tsx` | the rendered reading of a markdown file, before and after |
| `ui/src/lib/highlight.ts`, `lib/md.ts` | the lazily loaded highlighter and markdown renderer |

### Paths

A path reaches grove from two directions and they do not spell it the same
way. git prints forward slashes on every platform, so a worktree on Windows
comes back as `C:/Users/x/src/repo` while Go builds `C:\Users\x\src\repo`;
and git reports the *physical* path, so a repository reached through a symlink
comes out of `git worktree list` at its real location while grove still holds
the link. macOS puts `/tmp` behind such a link, and a `~/src` on a second
volume is a common enough layout.

Neither is cosmetic. The main checkout is recognised by comparing the worktree
path git printed against the repo path grove built, so either mismatch makes a
repository look as though it had no main checkout — and lists it twice, once
under each spelling. So a path is normalised the moment it enters, from either
side, and everything downstream compares normalised paths (`paths.go`).

grove grew up inside [fylr](https://github.com/programmfabrik/fylr) as a tool
over its two dozen worktrees, and moved out once nothing in it was fylr's any
more.

## License

MIT — see [LICENSE](LICENSE).
