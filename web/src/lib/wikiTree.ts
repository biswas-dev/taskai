import type { WikiPage } from './api'

/** Maximum nesting depth for wiki pages (a top-level page is depth 1). */
export const WIKI_MAX_DEPTH = 6

export interface WikiTreeNode {
  page: WikiPage
  depth: number
  children: WikiTreeNode[]
}

/** How sibling pages are ordered in the sidebar tree. */
export type WikiSortMode = 'created_desc' | 'created_asc' | 'updated_desc' | 'title_asc' | 'manual'

export const WIKI_SORT_MODES: { value: WikiSortMode; label: string; hint: string }[] = [
  { value: 'created_desc', label: 'Newest first', hint: 'Date created, newest at the top' },
  { value: 'created_asc', label: 'Oldest first', hint: 'Date created, oldest at the top' },
  { value: 'updated_desc', label: 'Recently updated', hint: 'Date last edited, most recent at the top' },
  { value: 'title_asc', label: 'Title A–Z', hint: 'Alphabetical by page title' },
  { value: 'manual', label: 'Manual order', hint: 'The order pages were arranged in' },
]

export const DEFAULT_WIKI_SORT: WikiSortMode = 'created_desc'

/** Author of a page: the person who created it, or the agent that wrote it. */
export function wikiPageAuthor(page: WikiPage): string {
  return page.agent_name || page.creator_name || 'Unknown'
}

/** The distinct authors across a page list, alphabetically. */
export function wikiPageAuthors(pages: WikiPage[]): string[] {
  return [...new Set(pages.map(wikiPageAuthor))].sort((a, b) => a.localeCompare(b))
}

function manualOrder(a: WikiPage, b: WikiPage): number {
  return a.position - b.position || a.title.localeCompare(b.title)
}

/**
 * Comparator for the given sort mode. Every mode falls back to the manual order
 * so pages with identical timestamps keep a stable, predictable position.
 */
export function wikiSortComparator(mode: WikiSortMode): (a: WikiPage, b: WikiPage) => number {
  switch (mode) {
    case 'created_desc':
      return (a, b) => b.created_at.localeCompare(a.created_at) || manualOrder(a, b)
    case 'created_asc':
      return (a, b) => a.created_at.localeCompare(b.created_at) || manualOrder(a, b)
    case 'updated_desc':
      return (a, b) => b.updated_at.localeCompare(a.updated_at) || manualOrder(a, b)
    case 'title_asc':
      return (a, b) => a.title.localeCompare(b.title) || manualOrder(a, b)
    case 'manual':
    default:
      return manualOrder
  }
}

/**
 * Build a forest from a flat page list. Pages whose parent is missing from the
 * list (e.g. filtered out) are promoted to the top level so nothing is hidden.
 * Siblings are ordered by `sort`, which defaults to the manual arrangement.
 */
export function buildWikiTree(pages: WikiPage[], sort: WikiSortMode = 'manual'): WikiTreeNode[] {
  const bySiblingOrder = wikiSortComparator(sort)
  const ids = new Set(pages.map(p => p.id))
  const childrenOf = new Map<number | null, WikiPage[]>()
  for (const page of pages) {
    const key = page.parent_id !== null && ids.has(page.parent_id) ? page.parent_id : null
    const list = childrenOf.get(key) ?? []
    list.push(page)
    childrenOf.set(key, list)
  }

  const visited = new Set<number>()
  const build = (parentId: number | null, depth: number): WikiTreeNode[] =>
    (childrenOf.get(parentId) ?? [])
      .filter(p => !visited.has(p.id))
      .sort(bySiblingOrder)
      .map(page => {
        visited.add(page.id)
        return { page, depth, children: build(page.id, depth + 1) }
      })

  return build(null, 1)
}

/** Ancestors of a page, ordered from the top-level page down to the direct parent. */
export function getWikiAncestors(pages: WikiPage[], pageId: number): WikiPage[] {
  const byId = new Map(pages.map(p => [p.id, p]))
  const chain: WikiPage[] = []
  let current = byId.get(pageId)?.parent_id ?? null
  while (current !== null && !chain.some(p => p.id === current)) {
    const parent = byId.get(current)
    if (!parent) break
    chain.unshift(parent)
    current = parent.parent_id
  }
  return chain
}

/** IDs of every page beneath the given page (not including the page itself). */
export function getWikiDescendantIds(pages: WikiPage[], pageId: number): Set<number> {
  const result = new Set<number>()
  const stack = [pageId]
  while (stack.length) {
    const id = stack.pop() as number
    for (const p of pages) {
      if (p.parent_id === id && !result.has(p.id)) {
        result.add(p.id)
        stack.push(p.id)
      }
    }
  }
  return result
}

/** 1-based depth of a page. */
export function getWikiDepth(pages: WikiPage[], pageId: number): number {
  return getWikiAncestors(pages, pageId).length + 1
}

/** Number of levels in the subtree rooted at the page (a leaf is 1). */
export function getWikiSubtreeHeight(pages: WikiPage[], pageId: number): number {
  let height = 1
  for (const p of pages) {
    if (p.parent_id === pageId) {
      height = Math.max(height, getWikiSubtreeHeight(pages, p.id) + 1)
    }
  }
  return height
}

export type WikiMoveCheck = { ok: true } | { ok: false; reason: string }

/**
 * Mirror of the server-side hierarchy validation so the UI can give instant
 * feedback before making a request.
 */
export function canMoveWikiPage(pages: WikiPage[], pageId: number, newParentId: number | null): WikiMoveCheck {
  const page = pages.find(p => p.id === pageId)
  if (!page) return { ok: false, reason: 'Page not found' }
  if (newParentId === page.parent_id) return { ok: false, reason: 'Page is already there' }
  if (newParentId === null) return { ok: true }
  if (newParentId === pageId) return { ok: false, reason: 'A page cannot be nested under itself' }
  if (getWikiDescendantIds(pages, pageId).has(newParentId)) {
    return { ok: false, reason: 'A page cannot be nested under one of its own sub-pages' }
  }
  if (!pages.some(p => p.id === newParentId)) return { ok: false, reason: 'Parent page not found' }
  const resultingDepth = getWikiDepth(pages, newParentId) + getWikiSubtreeHeight(pages, pageId)
  if (resultingDepth > WIKI_MAX_DEPTH) {
    return { ok: false, reason: `Pages can be nested at most ${WIKI_MAX_DEPTH} levels deep` }
  }
  return { ok: true }
}

/** Whether a new child page may be created under the given page. */
export function canAddWikiChild(pages: WikiPage[], parentId: number): boolean {
  return getWikiDepth(pages, parentId) < WIKI_MAX_DEPTH
}
