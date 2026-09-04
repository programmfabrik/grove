import type { Checks, DiffFile, Diagnostics, JobState, Prefs, Program, RemoteResult, RemoteState, Repo, ScopeRepo, State, Update } from './types'

async function j<T>(r: Response): Promise<T> {
  if (!r.ok) {
    const e = (await r.json().catch(() => ({}))) as { error?: string }
    throw new Error(e.error || r.statusText)
  }
  return r.json() as Promise<T>
}

export const api = {
  repos: (): Promise<{ dir: string; repos: Repo[] }> =>
    fetch('api/repos').then((r) => j<{ dir: string; repos: Repo[] }>(r)),
  state: (repo: string): Promise<State> =>
    fetch(`api/state?repo=${encodeURIComponent(repo)}`).then((r) => j<State>(r)),
  // the diff sidebar: what can be diffed, then a scope's files, then one file
  scopes: (name: string): Promise<{ name: string; repos: ScopeRepo[] }> =>
    fetch(`api/scopes?name=${encodeURIComponent(name)}`).then((r) =>
      j<{ name: string; repos: ScopeRepo[] }>(r),
    ),
  diffFiles: (name: string, repo: string, scope: string, ignoreComments?: boolean): Promise<{ files: DiffFile[] }> =>
    fetch(
      `api/diff?name=${encodeURIComponent(name)}&repo=${encodeURIComponent(repo)}&scope=${encodeURIComponent(scope)}` +
        (ignoreComments ? '&ignore_comments=1' : ''),
    ).then((r) => j<{ files: DiffFile[] }>(r)),
  diffText: (
    name: string,
    repo: string,
    scope: string,
    file: string,
    untracked?: boolean,
    ignoreComments?: boolean,
  ): Promise<{ diff: string; total: number; truncated?: boolean }> =>
    fetch(
      `api/diff?name=${encodeURIComponent(name)}&repo=${encodeURIComponent(repo)}&scope=${encodeURIComponent(scope)}` +
        `&file=${encodeURIComponent(file)}${untracked ? '&untracked=1' : ''}` +
        (ignoreComments ? '&ignore_comments=1' : ''),
    ).then((r) => j<{ diff: string; total: number; truncated?: boolean }>(r)),
  // the one write: unstage or discard, on paths the caller can see
  revert: (body: {
    name: string
    repo: string
    action: 'unstage' | 'discard'
    paths: string[]
    untracked: string[]
  }): Promise<{ reverted: number }> =>
    fetch('api/revert', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }).then((r) => j<{ reverted: number }>(r)),
  // a whole file as one side of the scope sees it — what the syntax colouring
  // and the rendered markdown read, since a diff is not a file
  fileText: (
    name: string,
    repo: string,
    scope: string,
    file: string,
    side: 'before' | 'after',
  ): Promise<{ lines: string[]; total: number }> =>
    fetch(
      `api/lines?name=${encodeURIComponent(name)}&repo=${encodeURIComponent(repo)}&scope=${encodeURIComponent(scope)}` +
        `&file=${encodeURIComponent(file)}&from=1&to=0&side=${side}`,
    ).then((r) => j<{ lines: string[]; total: number }>(r)),
  // the hunk expanders: the unchanged lines a diff leaves out, read at the
  // scope's own revision
  fileLines: (
    name: string,
    repo: string,
    scope: string,
    file: string,
    from: number,
    to: number,
  ): Promise<{ lines: string[]; from: number; total: number }> =>
    fetch(
      `api/lines?name=${encodeURIComponent(name)}&repo=${encodeURIComponent(repo)}&scope=${encodeURIComponent(scope)}` +
        `&file=${encodeURIComponent(file)}&from=${from}&to=${to}`,
    ).then((r) => j<{ lines: string[]; from: number; total: number }>(r)),
  // whether GitHub is testing what each checkout pushed, keyed by checkout
  checks: (repo: string): Promise<{ checks: Record<string, Checks>; error?: string; note?: string }> =>
    fetch(`api/checks?repo=${encodeURIComponent(repo)}`).then((r) =>
      j<{ checks: Record<string, Checks>; error?: string; note?: string }>(r),
    ),
  diagnostics: (): Promise<Diagnostics> => fetch('api/diagnostics').then((r) => j<Diagnostics>(r)),
  prefs: (): Promise<Prefs> => fetch('api/settings').then((r) => j<Prefs>(r)),
  programs: (): Promise<{ programs: Program[]; chosen: Record<string, string> }> =>
    fetch('api/programs').then((r) => j<{ programs: Program[]; chosen: Record<string, string> }>(r)),
  // open a checkout in a terminal, or a file in an editor, at a line
  launch: (body: {
    kind: 'terminal' | 'editor'
    name: string
    repo?: string
    file?: string
    line?: number
  }): Promise<{ opened: string }> =>
    fetch('api/launch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }).then((r) => j<{ opened: string }>(r)),
  notify: (title: string, body: string): Promise<unknown> =>
    fetch('api/notify', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title, body }),
    }).then((r) => j<unknown>(r)),
  // point at any application on the disk, since no catalogue is complete
  chooseProgram: (kind: string): Promise<{ id?: string; name?: string; cancelled?: boolean }> =>
    fetch('api/programs/choose', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ kind }),
    }).then((r) => j<{ id?: string; name?: string; cancelled?: boolean }>(r)),
  chooseFolder: (): Promise<{ asked: boolean }> =>
    fetch('api/folder/choose', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' }).then(
      (r) => j<{ asked: boolean }>(r),
    ),
  useFolder: (dir: string): Promise<{ dir: string }> =>
    fetch('api/folder/use', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dir }),
    }).then((r) => j<{ dir: string }>(r)),
  loginItem: (): Promise<{ on: boolean; possible: boolean }> =>
    fetch('api/loginitem').then((r) => j<{ on: boolean; possible: boolean }>(r)),
  setLoginItem: (on: boolean): Promise<{ on: boolean }> =>
    fetch('api/loginitem', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ on }),
    }).then((r) => j<{ on: boolean }>(r)),
  setPrefs: (p: Prefs): Promise<Prefs> =>
    fetch('api/settings', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(p),
    }).then((r) => j<Prefs>(r)),
  // a window is not a browser: it has no tabs and no address bar, so a link
  // outward is handed to the browser you are already signed in to
  open: (url: string): Promise<{ opened: string }> =>
    fetch('api/open', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url }),
    }).then((r) => j<{ opened: string }>(r)),
  // a second window on the same page, at the view the fragment names. Only
  // the app has windows to open; a browser opens its own and never asks.
  window: (frag: string, title: string): Promise<{ opened: boolean }> =>
    fetch('api/window', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ frag, title }),
    }).then((r) => j<{ opened: boolean }>(r)),
  // what is running, and whether anything newer was published
  version: (): Promise<Update> => fetch('api/version').then((r) => j<Update>(r)),
  // where every repository under a checkout stands with its remote. A read:
  // it never fetches, so it is as fresh as the last fetch and says when.
  remote: (name: string): Promise<RemoteState> =>
    fetch(`api/remote?name=${encodeURIComponent(name)}`).then((r) => j<RemoteState>(r)),
  // fetch, push, rebase or merge. Always fetches first, always every
  // repository under the checkout, because whether the parent may push
  // depends on what the submodules' remotes hold.
  // a push or a pull, watched while it runs: start it, then read the
  // transcript as it is written
  run: (body: {
    name: string
    repos: string[]
    action: 'push' | 'rebase' | 'merge' | 'ff'
    remote?: string
  }): Promise<{ job: string }> =>
    fetch('api/run', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }).then((r) => j<{ job: string }>(r)),
  job: (id: string, after: number): Promise<JobState> =>
    fetch(`api/run?job=${encodeURIComponent(id)}&after=${after}`).then((r) => j<JobState>(r)),
  remoteAct: (body: {
    name: string
    repos: string[]
    action: 'fetch' | 'push' | 'rebase' | 'merge' | 'ff'
    remote?: string
  }): Promise<{ results: RemoteResult[]; repos: RemoteState['repos'] }> =>
    fetch('api/remote', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }).then((r) => j<{ results: RemoteResult[]; repos: RemoteState['repos'] }>(r)),
}
