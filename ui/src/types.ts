// Mirrors the Go payload (discover.go, repos.go, main.go).

export type Commit = {
  hash: string
  subject: string
  author: string
  date: string
}

export type Checkout = {
  name: string
  path: string
  is_main: boolean
  branch: string
  detached: boolean
  head: Commit
  ahead: number
  behind: number
  dirty: number
}

// where a changed file's change lives. "staged" only occurs in the staged
// scope, where every file is by definition in the index.
export type DiffOrigin = 'branch' | 'working' | 'both' | 'staged'

// one entry in the scope list: WHAT the diff shows for a repo
export type Scope = {
  id: string
  label: string
  kind: 'range' | 'staged' | 'unstaged' | 'commit'
  hint?: string
  sha?: string
  pushed?: boolean
  // the base branch already contains this commit
  merged?: boolean
  date?: string
  author?: string
  // a commit's message below the subject — what the header expands
  body?: string
  files: number
  added: number
  deleted: number
}

// a repo's section in the scope list — the headline and its scopes
export type ScopeRepo = {
  name: string
  branch?: string
  base?: string
  upstream?: string
  scopes: Scope[]
}

export type DiffFile = {
  repo: string
  path: string
  status: string
  origin: DiffOrigin
  // set when the browser can render the file itself: image | pdf | video | audio
  preview?: string
  added: number
  deleted: number
  untracked?: boolean
  // range scopes: the branch compared against already holds this change
  merged?: boolean
}

// a repository in the start directory — pane one
export type Repo = {
  name: string
  path: string
  branch?: string
  worktrees: number
  dirty?: boolean
  last_used?: string
}

export type State = {
  repo: string // path
  name: string
  checkouts: Checkout[]
  base: string
  git_at: string
  git_error?: string
}

// what /api/version says: what is running and, if the check is on, what has
// been released since
export type Update = {
  version: string
  latest?: string
  available?: boolean
  url?: string
  notes_url?: string
  homebrew?: boolean
}

// One repository's standing with its remote — the checkout, or a submodule
// under it. See remote.go.
export type RemoteRepo = {
  name: string
  branch?: string
  detached?: boolean
  upstream?: string
  remote?: string
  remotes?: string[]
  ahead: number
  behind: number
  dirty: number
  gitlink?: string
  gitlink_unknown?: boolean
  can_push: boolean
  blocked?: string
  can_pull: boolean
  pull_blocked?: string
  pull_mode?: 'ff' | 'rebase' | ''
  fetched_at?: string
}
export type RemoteState = { name: string; repo?: string; repos: RemoteRepo[] }
export type RemoteResult = { repo: string; ok: boolean; detail?: string }
