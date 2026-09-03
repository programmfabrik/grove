import type { DiffFile, RemoteResult, RemoteState, Repo, ScopeRepo, State, Update } from './types'

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
  // what is running, and whether anything newer was published
  version: (): Promise<Update> => fetch('api/version').then((r) => j<Update>(r)),
  // where every repository under a checkout stands with its remote. A read:
  // it never fetches, so it is as fresh as the last fetch and says when.
  remote: (name: string): Promise<RemoteState> =>
    fetch(`api/remote?name=${encodeURIComponent(name)}`).then((r) => j<RemoteState>(r)),
  // fetch, push, rebase or merge. Always fetches first, always every
  // repository under the checkout, because whether the parent may push
  // depends on what the submodules' remotes hold.
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
