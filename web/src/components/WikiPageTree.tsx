import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent, DragEvent as ReactDragEvent } from 'react'
import type { WikiPage } from '../lib/api'
import {
  buildWikiTree,
  canAddWikiChild,
  canMoveWikiPage,
  getWikiAncestors,
  getWikiDepth,
  getWikiDescendantIds,
  WIKI_MAX_DEPTH,
  type WikiTreeNode,
} from '../lib/wikiTree'

interface WikiPageTreeProps {
  projectId: number
  pages: WikiPage[]
  selectedPageId: number | null
  searchQuery: string
  onSelect: (pageId: number) => void
  onCreate: (title: string, parentId: number | null) => Promise<WikiPage | null>
  onMove: (pageId: number, parentId: number | null) => Promise<void>
  onDelete: (pageId: number) => void
}

/** Left padding per depth level. Static strings so Tailwind can see them. */
const DEPTH_PADDING = ['pl-2', 'pl-2', 'pl-6', 'pl-10', 'pl-14', 'pl-[4.5rem]', 'pl-[5.5rem]']

/** Horizontal offset of the vertical guide line drawn under a node's chevron, per depth. */
const DEPTH_GUIDE = ['left-[17px]', 'left-[17px]', 'left-[33px]', 'left-[49px]', 'left-[65px]', 'left-[81px]', 'left-[97px]']

function depthPadding(depth: number): string {
  return DEPTH_PADDING[Math.min(depth, DEPTH_PADDING.length - 1)]
}

function depthGuide(depth: number): string {
  return DEPTH_GUIDE[Math.min(depth, DEPTH_GUIDE.length - 1)]
}

function storageKey(projectId: number): string {
  return `taskai.wiki.tree.expanded.${projectId}`
}

function loadExpanded(projectId: number): Set<number> {
  try {
    const raw = localStorage.getItem(storageKey(projectId))
    if (!raw) return new Set()
    const parsed: unknown = JSON.parse(raw)
    return Array.isArray(parsed) ? new Set(parsed.filter((v): v is number => typeof v === 'number')) : new Set()
  } catch {
    return new Set()
  }
}

function saveExpanded(projectId: number, expanded: Set<number>): void {
  try {
    localStorage.setItem(storageKey(projectId), JSON.stringify([...expanded]))
  } catch {
    // Storage may be unavailable; expansion state is a convenience only.
  }
}

/** Flatten the visible portion of the tree (respecting collapsed nodes) for keyboard navigation. */
function visibleNodes(nodes: WikiTreeNode[], expanded: Set<number>): WikiTreeNode[] {
  const out: WikiTreeNode[] = []
  const walk = (list: WikiTreeNode[]) => {
    for (const n of list) {
      out.push(n)
      if (n.children.length && expanded.has(n.page.id)) walk(n.children)
    }
  }
  walk(nodes)
  return out
}

export default function WikiPageTree({
  projectId,
  pages,
  selectedPageId,
  searchQuery,
  onSelect,
  onCreate,
  onMove,
  onDelete,
}: Readonly<WikiPageTreeProps>) {
  const tree = useMemo(() => buildWikiTree(pages), [pages])
  const [expanded, setExpanded] = useState<Set<number>>(() => loadExpanded(projectId))
  const [menuFor, setMenuFor] = useState<number | null>(null)
  const [moveFor, setMoveFor] = useState<number | null>(null)
  // `undefined` = not creating; `null` = creating at top level; number = under that page.
  const [creatingUnder, setCreatingUnder] = useState<number | null | undefined>(undefined)
  const [newTitle, setNewTitle] = useState('')
  const [creating, setCreating] = useState(false)
  const [draggingId, setDraggingId] = useState<number | null>(null)
  const [dropTarget, setDropTarget] = useState<{ id: number | null; ok: boolean; reason?: string } | null>(null)
  const [moving, setMoving] = useState(false)
  const [focusedId, setFocusedId] = useState<number | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  // Persist expansion per project.
  useEffect(() => {
    saveExpanded(projectId, expanded)
  }, [projectId, expanded])

  // Reveal the selected page by expanding all of its ancestors.
  useEffect(() => {
    if (selectedPageId === null) return
    const ancestors = getWikiAncestors(pages, selectedPageId)
    if (!ancestors.length) return
    setExpanded(prev => {
      if (ancestors.every(a => prev.has(a.id))) return prev
      const next = new Set(prev)
      ancestors.forEach(a => next.add(a.id))
      return next
    })
  }, [pages, selectedPageId])

  // Close any open menu when clicking elsewhere.
  useEffect(() => {
    if (menuFor === null) return
    const close = (e: MouseEvent) => {
      if (!containerRef.current?.contains(e.target as Node)) setMenuFor(null)
    }
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [menuFor])

  const toggle = useCallback((id: number) => {
    setExpanded(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const expandAll = useCallback(() => {
    setExpanded(new Set(pages.filter(p => pages.some(c => c.parent_id === p.id)).map(p => p.id)))
  }, [pages])

  const collapseAll = useCallback(() => setExpanded(new Set()), [])

  const startCreate = useCallback((parentId: number | null) => {
    setMenuFor(null)
    setNewTitle('')
    setCreatingUnder(parentId)
    if (parentId !== null) setExpanded(prev => (prev.has(parentId) ? prev : new Set(prev).add(parentId)))
  }, [])

  const submitCreate = useCallback(async () => {
    const title = newTitle.trim()
    if (!title || creatingUnder === undefined) return
    setCreating(true)
    try {
      const created = await onCreate(title, creatingUnder)
      if (created) {
        setCreatingUnder(undefined)
        setNewTitle('')
      }
    } finally {
      setCreating(false)
    }
  }, [creatingUnder, newTitle, onCreate])

  const cancelCreate = useCallback(() => {
    setCreatingUnder(undefined)
    setNewTitle('')
  }, [])

  const performMove = useCallback(
    async (pageId: number, parentId: number | null) => {
      const check = canMoveWikiPage(pages, pageId, parentId)
      if (!check.ok) return
      setMoving(true)
      try {
        await onMove(pageId, parentId)
        if (parentId !== null) setExpanded(prev => (prev.has(parentId) ? prev : new Set(prev).add(parentId)))
      } finally {
        setMoving(false)
        setMoveFor(null)
      }
    },
    [onMove, pages],
  )

  // ── Drag and drop ────────────────────────────────────────────
  const handleDragStart = (e: ReactDragEvent, pageId: number) => {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', String(pageId))
    setDraggingId(pageId)
    setMenuFor(null)
  }

  const handleDragOver = (e: ReactDragEvent, targetId: number | null) => {
    if (draggingId === null) return
    e.preventDefault()
    const check = canMoveWikiPage(pages, draggingId, targetId)
    e.dataTransfer.dropEffect = check.ok ? 'move' : 'none'
    if (dropTarget?.id !== targetId || dropTarget.ok !== check.ok) {
      setDropTarget({ id: targetId, ok: check.ok, reason: check.ok ? undefined : check.reason })
    }
  }

  const handleDrop = async (e: ReactDragEvent, targetId: number | null) => {
    e.preventDefault()
    const id = draggingId
    setDraggingId(null)
    setDropTarget(null)
    if (id === null) return
    await performMove(id, targetId)
  }

  const handleDragEnd = () => {
    setDraggingId(null)
    setDropTarget(null)
  }

  // ── Keyboard navigation ──────────────────────────────────────
  const handleKeyDown = (e: ReactKeyboardEvent, node: WikiTreeNode) => {
    const flat = visibleNodes(tree, expanded)
    const idx = flat.findIndex(n => n.page.id === node.page.id)
    const focusNode = (n: WikiTreeNode | undefined) => {
      if (!n) return
      setFocusedId(n.page.id)
      containerRef.current?.querySelector<HTMLElement>(`[data-page-id="${n.page.id}"]`)?.focus()
    }
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        focusNode(flat[idx + 1])
        break
      case 'ArrowUp':
        e.preventDefault()
        focusNode(flat[idx - 1])
        break
      case 'ArrowRight':
        e.preventDefault()
        if (node.children.length && !expanded.has(node.page.id)) toggle(node.page.id)
        else if (node.children.length) focusNode(node.children[0])
        break
      case 'ArrowLeft':
        e.preventDefault()
        if (node.children.length && expanded.has(node.page.id)) toggle(node.page.id)
        else if (node.page.parent_id !== null) focusNode(flat.find(n => n.page.id === node.page.parent_id))
        break
      case 'Enter':
      case ' ':
        e.preventDefault()
        onSelect(node.page.id)
        break
      case 'Delete':
      case 'Backspace':
        if (e.metaKey || e.ctrlKey) {
          e.preventDefault()
          onDelete(node.page.id)
        }
        break
      default:
        break
    }
  }

  // ── Search mode: flat list of matches with their path ────────
  const query = searchQuery.trim().toLowerCase()
  if (query) {
    const matches = pages.filter(p => p.title.toLowerCase().includes(query))
    if (!matches.length) {
      return <div className="p-4 text-center text-dark-text-tertiary text-sm">No matching pages</div>
    }
    return (
      <ul className="py-2" role="listbox" aria-label="Search results">
        {matches.map(page => {
          const path = getWikiAncestors(pages, page.id).map(a => a.title)
          const selected = selectedPageId === page.id
          return (
            <li key={page.id} role="option" aria-selected={selected}>
              <button
                type="button"
                onClick={() => onSelect(page.id)}
                className={`w-full text-left px-4 py-2 hover:bg-dark-bg-tertiary transition-colors ${
                  selected ? 'bg-dark-bg-tertiary border-l-2 border-primary-500' : 'border-l-2 border-transparent'
                }`}
              >
                <span className="block text-sm text-dark-text-primary truncate">{page.title}</span>
                {path.length > 0 && (
                  <span className="block text-[11px] text-dark-text-tertiary truncate">{path.join(' / ')}</span>
                )}
              </button>
            </li>
          )
        })}
      </ul>
    )
  }

  const hasAnyChildren = pages.some(p => p.parent_id !== null)

  const renderCreateRow = (parentId: number | null, depth: number) => (
    <li key={`new-${parentId ?? 'root'}`} className={`${depthPadding(depth)} pr-3 py-1`}>
      <div className="flex items-center gap-1">
        <input
          type="text"
          value={newTitle}
          placeholder={parentId === null ? 'New page title…' : 'New sub-page title…'}
          aria-label={parentId === null ? 'New page title' : 'New sub-page title'}
          autoFocus
          onChange={e => setNewTitle(e.target.value)}
          onKeyDown={e => {
            if (e.key === 'Enter') void submitCreate()
            if (e.key === 'Escape') cancelCreate()
          }}
          className="flex-1 min-w-0 px-2 py-1 bg-dark-bg-primary border border-primary-500 rounded text-sm text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none"
        />
        <button
          type="button"
          onClick={() => void submitCreate()}
          disabled={creating || !newTitle.trim()}
          className="px-2 py-1 text-xs bg-primary-600 text-white rounded hover:bg-primary-500 disabled:opacity-50"
        >
          {creating ? '…' : 'Add'}
        </button>
        <button
          type="button"
          onClick={cancelCreate}
          aria-label="Cancel"
          className="px-1.5 py-1 text-xs text-dark-text-tertiary hover:text-dark-text-primary"
        >
          ✕
        </button>
      </div>
    </li>
  )

  const renderNode = (node: WikiTreeNode): JSX.Element => {
    const { page, depth, children } = node
    const isExpanded = expanded.has(page.id)
    const isSelected = selectedPageId === page.id
    const isDragging = draggingId === page.id
    const isDropTarget = dropTarget?.id === page.id
    const canAddChild = canAddWikiChild(pages, page.id)
    const showChildren = children.length > 0 && isExpanded

    return (
      <li key={page.id} role="none">
        <div
          role="treeitem"
          aria-level={depth}
          aria-expanded={children.length ? isExpanded : undefined}
          aria-selected={isSelected}
          data-page-id={page.id}
          tabIndex={focusedId === page.id || (focusedId === null && isSelected) ? 0 : -1}
          draggable
          onDragStart={e => handleDragStart(e, page.id)}
          onDragOver={e => handleDragOver(e, page.id)}
          onDragLeave={() => {
            if (dropTarget?.id === page.id) setDropTarget(null)
          }}
          onDrop={e => void handleDrop(e, page.id)}
          onDragEnd={handleDragEnd}
          onClick={() => onSelect(page.id)}
          onKeyDown={e => handleKeyDown(e, node)}
          onFocus={() => setFocusedId(page.id)}
          title={isDropTarget && !dropTarget.ok ? dropTarget.reason : page.title}
          className={`group relative flex items-center gap-1 ${depthPadding(depth)} pr-2 py-1.5 cursor-pointer select-none transition-colors outline-none focus-visible:ring-1 focus-visible:ring-primary-500 ${
            isSelected
              ? 'bg-dark-bg-tertiary border-l-2 border-primary-500'
              : 'border-l-2 border-transparent hover:bg-dark-bg-tertiary/70'
          } ${isDragging ? 'opacity-40' : ''} ${
            isDropTarget ? (dropTarget.ok ? 'ring-1 ring-inset ring-primary-500 bg-primary-500/10' : 'ring-1 ring-inset ring-red-500/60') : ''
          }`}
        >
          {/* Expand / collapse chevron */}
          {children.length > 0 ? (
            <button
              type="button"
              aria-label={isExpanded ? `Collapse ${page.title}` : `Expand ${page.title}`}
              onClick={e => {
                e.stopPropagation()
                toggle(page.id)
              }}
              className="shrink-0 w-5 h-5 flex items-center justify-center rounded text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-primary/60"
            >
              <svg
                className={`w-3 h-3 transition-transform ${isExpanded ? 'rotate-90' : ''}`}
                fill="none"
                stroke="currentColor"
                strokeWidth={2.5}
                viewBox="0 0 24 24"
              >
                <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
              </svg>
            </button>
          ) : (
            <span className="shrink-0 w-5 h-5 flex items-center justify-center text-dark-text-quaternary" aria-hidden="true">
              <svg className="w-3 h-3" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
              </svg>
            </span>
          )}

          <span className={`flex-1 min-w-0 text-sm truncate ${isSelected ? 'text-dark-text-primary font-medium' : 'text-dark-text-primary'}`}>
            {page.title}
          </span>

          {children.length > 0 && !isExpanded && (
            <span className="shrink-0 text-[10px] text-dark-text-quaternary tabular-nums group-hover:hidden">{children.length}</span>
          )}

          {/* Hover actions */}
          <span className="shrink-0 hidden group-hover:flex group-focus-within:flex items-center gap-0.5">
            <button
              type="button"
              aria-label={canAddChild ? `Add sub-page under ${page.title}` : `Cannot add sub-page: maximum depth of ${WIKI_MAX_DEPTH} reached`}
              title={canAddChild ? 'Add sub-page' : `Maximum depth of ${WIKI_MAX_DEPTH} levels reached`}
              disabled={!canAddChild}
              onClick={e => {
                e.stopPropagation()
                startCreate(page.id)
              }}
              className="w-5 h-5 flex items-center justify-center rounded text-dark-text-tertiary hover:text-primary-400 hover:bg-dark-bg-primary/60 disabled:opacity-30 disabled:cursor-not-allowed"
            >
              <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
              </svg>
            </button>
            <button
              type="button"
              aria-label={`More actions for ${page.title}`}
              aria-haspopup="menu"
              aria-expanded={menuFor === page.id}
              onClick={e => {
                e.stopPropagation()
                setMenuFor(prev => (prev === page.id ? null : page.id))
              }}
              className="w-5 h-5 flex items-center justify-center rounded text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-primary/60"
            >
              <svg className="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 24 24">
                <circle cx="5" cy="12" r="1.8" />
                <circle cx="12" cy="12" r="1.8" />
                <circle cx="19" cy="12" r="1.8" />
              </svg>
            </button>
          </span>

          {menuFor === page.id && (
            <div
              role="menu"
              aria-label={`Actions for ${page.title}`}
              className="absolute right-2 top-full z-20 mt-0.5 w-40 rounded-md border border-dark-border-medium bg-dark-bg-elevated shadow-lg py-1 text-sm"
              onClick={e => e.stopPropagation()}
            >
              <button
                type="button"
                role="menuitem"
                onClick={() => {
                  setMenuFor(null)
                  setMoveFor(page.id)
                }}
                className="w-full text-left px-3 py-1.5 text-dark-text-secondary hover:bg-dark-bg-tertiary hover:text-dark-text-primary"
              >
                Move to…
              </button>
              {page.parent_id !== null && (
                <button
                  type="button"
                  role="menuitem"
                  onClick={() => {
                    setMenuFor(null)
                    void performMove(page.id, null)
                  }}
                  className="w-full text-left px-3 py-1.5 text-dark-text-secondary hover:bg-dark-bg-tertiary hover:text-dark-text-primary"
                >
                  Move to top level
                </button>
              )}
              <div className="my-1 border-t border-dark-border-subtle" />
              <button
                type="button"
                role="menuitem"
                onClick={() => {
                  setMenuFor(null)
                  onDelete(page.id)
                }}
                className="w-full text-left px-3 py-1.5 text-red-400 hover:bg-red-500/10"
              >
                Delete
              </button>
            </div>
          )}
        </div>

        {(showChildren || creatingUnder === page.id) && (
          <ul role="group" className="relative">
            {/* Vertical guide line for nested levels */}
            <span
              aria-hidden="true"
              className={`pointer-events-none absolute top-0 bottom-0 w-px bg-dark-border-medium ${depthGuide(depth)}`}
            />
            {showChildren && children.map(renderNode)}
            {creatingUnder === page.id && renderCreateRow(page.id, depth + 1)}
          </ul>
        )}
      </li>
    )
  }

  return (
    <div ref={containerRef} className="flex flex-col h-full">
      {/* Tree toolbar */}
      <div className="flex items-center justify-between px-3 py-1.5 border-b border-dark-border-subtle/60">
        <span className="text-[11px] uppercase tracking-wide text-dark-text-tertiary">Pages</span>
        <div className="flex items-center gap-1">
          {hasAnyChildren && (
            <>
              <button
                type="button"
                onClick={expandAll}
                title="Expand all"
                aria-label="Expand all pages"
                className="w-6 h-6 flex items-center justify-center rounded text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary"
              >
                <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M4 8V4h4M20 8V4h-4M4 16v4h4M20 16v4h-4" />
                </svg>
              </button>
              <button
                type="button"
                onClick={collapseAll}
                title="Collapse all"
                aria-label="Collapse all pages"
                className="w-6 h-6 flex items-center justify-center rounded text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary"
              >
                <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M8 4v4H4M16 4v4h4M8 20v-4H4M16 20v-4h4" />
                </svg>
              </button>
            </>
          )}
          <button
            type="button"
            onClick={() => startCreate(null)}
            title="New top-level page"
            aria-label="New top-level page"
            className="w-6 h-6 flex items-center justify-center rounded text-primary-400 hover:bg-primary-500/15"
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
            </svg>
          </button>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto">
        {tree.length === 0 && creatingUnder === undefined ? (
          <div className="p-4 text-center text-dark-text-tertiary text-sm">
            <p>No pages yet</p>
            <button
              type="button"
              onClick={() => startCreate(null)}
              className="mt-2 text-primary-400 hover:text-primary-300 text-xs font-medium"
            >
              Create your first page
            </button>
          </div>
        ) : (
          <ul role="tree" aria-label="Wiki pages" className="py-1">
            {creatingUnder === null && renderCreateRow(null, 1)}
            {tree.map(renderNode)}
          </ul>
        )}

        {/* Root drop zone: visible only while dragging */}
        {draggingId !== null && (
          <div
            onDragOver={e => handleDragOver(e, null)}
            onDragLeave={() => {
              if (dropTarget?.id === null) setDropTarget(null)
            }}
            onDrop={e => void handleDrop(e, null)}
            className={`mx-3 my-2 rounded border border-dashed px-3 py-3 text-center text-xs transition-colors ${
              dropTarget?.id === null
                ? dropTarget.ok
                  ? 'border-primary-500 bg-primary-500/10 text-primary-400'
                  : 'border-red-500/60 text-red-400'
                : 'border-dark-border-medium text-dark-text-tertiary'
            }`}
          >
            {dropTarget?.id === null && !dropTarget.ok ? dropTarget.reason : 'Drop here to move to top level'}
          </div>
        )}
      </div>

      {moveFor !== null && (
        <MoveDialog
          pages={pages}
          pageId={moveFor}
          busy={moving}
          onCancel={() => setMoveFor(null)}
          onConfirm={parentId => void performMove(moveFor, parentId)}
        />
      )}
    </div>
  )
}

// ── Move dialog ────────────────────────────────────────────────

interface MoveDialogProps {
  pages: WikiPage[]
  pageId: number
  busy: boolean
  onCancel: () => void
  onConfirm: (parentId: number | null) => void
}

function MoveDialog({ pages, pageId, busy, onCancel, onConfirm }: Readonly<MoveDialogProps>) {
  const page = pages.find(p => p.id === pageId)
  const [target, setTarget] = useState<string>('')
  const excluded = useMemo(() => {
    const ids = getWikiDescendantIds(pages, pageId)
    ids.add(pageId)
    return ids
  }, [pages, pageId])

  // Present candidates in tree order with indentation so the choice reads like the sidebar.
  const options = useMemo(() => {
    const out: { id: number; label: string; disabled: boolean; reason?: string }[] = []
    const walk = (nodes: WikiTreeNode[]) => {
      for (const n of nodes) {
        if (!excluded.has(n.page.id)) {
          const check = canMoveWikiPage(pages, pageId, n.page.id)
          out.push({
            id: n.page.id,
            label: `${'  '.repeat(n.depth - 1)}${n.page.title}`,
            disabled: !check.ok,
            reason: check.ok ? undefined : check.reason,
          })
        }
        walk(n.children)
      }
    }
    walk(buildWikiTree(pages))
    return out
  }, [excluded, pageId, pages])

  const parsedTarget: number | null = target === '' ? null : Number(target)
  const check = canMoveWikiPage(pages, pageId, parsedTarget)
  const currentDepth = getWikiDepth(pages, pageId)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onCancel()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onCancel])

  if (!page) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      role="presentation"
      onMouseDown={e => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="wiki-move-title"
        className="w-full max-w-sm rounded-lg border border-dark-border-medium bg-dark-bg-elevated shadow-xl"
      >
        <div className="px-4 pt-4 pb-2">
          <h2 id="wiki-move-title" className="text-sm font-semibold text-dark-text-primary">
            Move “{page.title}”
          </h2>
          <p className="mt-1 text-xs text-dark-text-tertiary">
            Currently at level {currentDepth} of {WIKI_MAX_DEPTH}. Choose a new parent page.
          </p>
        </div>
        <div className="px-4 pb-3">
          <label htmlFor="wiki-move-target" className="block text-xs text-dark-text-secondary mb-1">
            New parent
          </label>
          <select
            id="wiki-move-target"
            value={target}
            onChange={e => setTarget(e.target.value)}
            className="w-full px-3 py-2 bg-dark-bg-primary border border-dark-border-subtle rounded text-sm text-dark-text-primary focus:outline-none focus:border-primary-500"
          >
            <option value="">— Top level —</option>
            {options.map(o => (
              <option key={o.id} value={o.id} disabled={o.disabled} title={o.reason}>
                {o.label}
                {o.disabled ? ' (not allowed)' : ''}
              </option>
            ))}
          </select>
          {!check.ok && (
            <p className="mt-2 text-xs text-red-400" role="alert">
              {check.reason}
            </p>
          )}
        </div>
        <div className="flex justify-end gap-2 px-4 py-3 border-t border-dark-border-subtle">
          <button
            type="button"
            onClick={onCancel}
            className="px-3 py-1.5 text-sm text-dark-text-secondary rounded hover:bg-dark-bg-tertiary"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => onConfirm(parsedTarget)}
            disabled={busy || !check.ok}
            className="px-3 py-1.5 text-sm bg-primary-600 text-white rounded hover:bg-primary-500 disabled:opacity-50"
          >
            {busy ? 'Moving…' : 'Move'}
          </button>
        </div>
      </div>
    </div>
  )
}
