import type { Scope } from '../types'

// A commit carries one dot, not two flags. How far it has travelled is a
// ladder — nobody else has it, a remote has it, the base branch has it — and
// each rung is a weaker claim on it: what is still yours to rewrite, what is
// still yours to land, what is history. The strongest true rung wins, so a
// commit that is neither pushed nor in the base reads `unpushed`: that is the
// live fact about it, and "not in main" follows from it anyway.
export function commitState(s: Scope, base?: string): { cls: string; tagCls: string; title: string; tag: string } {
  if (!s.pushed) {
    return { cls: 'sc-dot sc-unpushed', tagCls: 'ch-tag ch-tag-unpushed', title: 'not pushed yet', tag: 'unpushed' }
  }
  if (!s.merged) {
    const where = base || 'the base branch'
    return {
      cls: 'sc-dot sc-unmerged',
      tagCls: 'ch-tag ch-tag-unmerged',
      title: `pushed · not merged into ${where}`,
      tag: `not in ${where}`,
    }
  }
  return { cls: 'sc-dot sc-pushed', tagCls: '', title: 'pushed', tag: '' }
}

// landedWhy explains a count that contradicts itself: a branch N commits ahead
// of a base that already holds every line of them. That is what a squash merge
// leaves — the commits are not ancestors of the base, so nothing git counts can
// see it, and the branch reads as N commits of unmerged work for as long as it
// exists. Empty when there is nothing to explain.
export function landedWhy(c: { ahead: number; landed?: boolean }, base: string): string {
  if (!c.landed || !c.ahead) return ''
  return (
    `${base} already holds every change these ${c.ahead} commits carry.\n\n` +
    `They arrived by squash merge or cherry-pick, so they are not ancestors of ` +
    `${base} and it still counts them as ahead. There is nothing here ${base} lacks.`
  )
}
