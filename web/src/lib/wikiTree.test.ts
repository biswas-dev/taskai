import { describe, it, expect } from 'vitest'
import type { WikiPage } from './api'
import {
  buildWikiTree,
  canAddWikiChild,
  canMoveWikiPage,
  getWikiAncestors,
  getWikiDescendantIds,
  getWikiDepth,
  getWikiSubtreeHeight,
  WIKI_MAX_DEPTH,
} from './wikiTree'

function page(id: number, title: string, parent_id: number | null, position = 0): WikiPage {
  return {
    id,
    project_id: 1,
    title,
    slug: title.toLowerCase(),
    parent_id,
    position,
    created_by: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }
}

// A(1) > B(2) > C(3); D(4) top-level; E(5) under A after B
const pages = [
  page(1, 'A', null, 1),
  page(2, 'B', 1, 0),
  page(3, 'C', 2),
  page(4, 'D', null, 0),
  page(5, 'E', 1, 1),
]

describe('buildWikiTree', () => {
  it('nests children under parents ordered by position', () => {
    const tree = buildWikiTree(pages)
    expect(tree.map(n => n.page.title)).toEqual(['D', 'A'])
    const a = tree[1]
    expect(a.children.map(n => n.page.title)).toEqual(['B', 'E'])
    expect(a.children[0].children[0].page.title).toBe('C')
    expect(a.children[0].children[0].depth).toBe(3)
  })

  it('promotes pages whose parent is missing to the top level', () => {
    const tree = buildWikiTree(pages.filter(p => p.title !== 'B'))
    expect(tree.map(n => n.page.title)).toEqual(['C', 'D', 'A'])
  })

  it('falls back to title ordering when positions tie', () => {
    const tree = buildWikiTree([page(1, 'Zeta', null), page(2, 'Alpha', null)])
    expect(tree.map(n => n.page.title)).toEqual(['Alpha', 'Zeta'])
  })
})

describe('ancestors, descendants and depth', () => {
  it('returns ancestors from root to parent', () => {
    expect(getWikiAncestors(pages, 3).map(p => p.title)).toEqual(['A', 'B'])
    expect(getWikiAncestors(pages, 1)).toEqual([])
  })

  it('collects every descendant', () => {
    expect([...getWikiDescendantIds(pages, 1)].sort()).toEqual([2, 3, 5])
    expect(getWikiDescendantIds(pages, 4).size).toBe(0)
  })

  it('computes depth and subtree height', () => {
    expect(getWikiDepth(pages, 3)).toBe(3)
    expect(getWikiSubtreeHeight(pages, 1)).toBe(3)
    expect(getWikiSubtreeHeight(pages, 3)).toBe(1)
  })
})

describe('canMoveWikiPage', () => {
  it('allows moving to the top level and under unrelated pages', () => {
    expect(canMoveWikiPage(pages, 3, null)).toEqual({ ok: true })
    expect(canMoveWikiPage(pages, 3, 4)).toEqual({ ok: true })
  })

  it('rejects no-op, self, and descendant targets', () => {
    expect(canMoveWikiPage(pages, 2, 1).ok).toBe(false)
    expect(canMoveWikiPage(pages, 1, 1).ok).toBe(false)
    expect(canMoveWikiPage(pages, 1, 3).ok).toBe(false)
  })

  it('rejects moves that would exceed the depth limit', () => {
    // Build a chain of MAX_DEPTH - 1 pages, then try to hang a 2-high subtree on the end.
    const chain: WikiPage[] = []
    for (let i = 1; i < WIKI_MAX_DEPTH; i++) chain.push(page(100 + i, `L${i}`, i === 1 ? null : 100 + i - 1))
    const all = [...chain, page(200, 'X', null), page(201, 'Y', 200)]
    const last = chain[chain.length - 1].id
    const result = canMoveWikiPage(all, 200, last)
    expect(result.ok).toBe(false)
    // Y alone (height 1) fits exactly at depth MAX_DEPTH.
    expect(canMoveWikiPage(all, 201, last)).toEqual({ ok: true })
  })
})

describe('canAddWikiChild', () => {
  it('blocks children at the maximum depth', () => {
    const chain: WikiPage[] = []
    for (let i = 1; i <= WIKI_MAX_DEPTH; i++) chain.push(page(i, `L${i}`, i === 1 ? null : i - 1))
    expect(canAddWikiChild(chain, WIKI_MAX_DEPTH - 1)).toBe(true)
    expect(canAddWikiChild(chain, WIKI_MAX_DEPTH)).toBe(false)
  })
})
