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
