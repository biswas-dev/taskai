import { useCallback, useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, WikiPage, WikiAnnotation, AnnotationColor, AnnotationComment } from '../lib/api'
import WikiEditor from './WikiEditor'
import WikiAnnotationSidebar from './WikiAnnotationSidebar'
import WikiPageTree from './WikiPageTree'
import { getWikiAncestors, getWikiDepth, WIKI_MAX_DEPTH } from '../lib/wikiTree'
import { useSync } from '../state/SyncContext'
import { useDialog } from '../state/DialogContext'

interface WikiContentProps {
  projectId: string
}

export default function WikiContent({ projectId }: WikiContentProps) {
  const dialog = useDialog()
  const { registerSyncTask } = useSync()
  const [searchParams, setSearchParams] = useSearchParams()
  const selectedPageId = searchParams.get('page')
  const annotationParam = searchParams.get('annotation')
  const highlightTerm = searchParams.get('highlight')

  const [pages, setPages] = useState<WikiPage[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [searchQuery, setSearchQuery] = useState('')

  const [annotations, setAnnotations] = useState<WikiAnnotation[]>([])
  const [selectedAnnotationId, setSelectedAnnotationId] = useState<number | null>(null)
  const [showAnnotationSidebar, setShowAnnotationSidebar] = useState(false)
  const [pinnedAnnotations, setPinnedAnnotations] = useState(false)
  const [showResolved, setShowResolved] = useState(false)

  // Deep-link to annotation from ?annotation=X (e.g. notification clicks)
  useEffect(() => {
    if (!annotationParam || !annotations.length) return
    const id = Number(annotationParam)
    if (!id) return
    setSelectedAnnotationId(id)
    setShowAnnotationSidebar(true)
  }, [annotationParam, annotations])

  // Highlight search term from ?highlight=X (from command palette wiki search)
  useEffect(() => {
    if (!highlightTerm || !selectedPageId) return
    // Wait for wiki content to render
    const timer = setTimeout(() => {
      const container = document.querySelector('[data-wiki-content]') || document.querySelector('.ProseMirror') || document.querySelector('.prose')
      if (!container) return
      const walker = document.createTreeWalker(container, NodeFilter.SHOW_TEXT)
      const regex = new RegExp(`(${highlightTerm.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi')
      const textNodes: Text[] = []
      let node: Node | null
      while ((node = walker.nextNode())) {
        const text = node.textContent || ''
        if (regex.test(text)) { regex.lastIndex = 0; textNodes.push(node as Text) }
      }
      // Wrap matches in <mark> elements
      let firstMark: HTMLElement | null = null
      for (const textNode of textNodes) {
        const parts = (textNode.textContent || '').split(regex)
        const frag = document.createDocumentFragment()
        for (const part of parts) {
          if (regex.test(part)) {
            const mark = document.createElement('mark')
            mark.textContent = part
            mark.style.cssText = 'background: #facc15; color: #1a1a2e; padding: 1px 2px; border-radius: 2px; transition: background 2s ease;'
            frag.appendChild(mark)
            if (!firstMark) firstMark = mark
            regex.lastIndex = 0
          } else {
            frag.appendChild(document.createTextNode(part))
          }
        }
        textNode.parentNode?.replaceChild(frag, textNode)
      }
      // Scroll first match into view
      if (firstMark) firstMark.scrollIntoView({ behavior: 'smooth', block: 'center' })
      // Fade out highlights after 4 seconds
      setTimeout(() => {
        container.querySelectorAll('mark').forEach((m: Element) => {
          (m as HTMLElement).style.background = 'transparent'
        })
      }, 4000)
      // Remove highlight param from URL
      const newParams = new URLSearchParams(searchParams)
      newParams.delete('highlight')
      setSearchParams(newParams, { replace: true })
    }, 800)
    return () => clearTimeout(timer)
  }, [highlightTerm, selectedPageId]) // eslint-disable-line react-hooks/exhaustive-deps

  const loadPages = useCallback(async (silent = false) => {
    try {
      if (!silent) setLoading(true)
      setError(null)
      const pagesData = await api.getWikiPages(Number(projectId))
      setPages(pagesData)
    } catch (err) {
      if (silent) throw err
      setError(err instanceof Error ? err.message : 'Failed to load wiki pages')
    } finally {
      if (!silent) setLoading(false)
    }
  }, [projectId])

  const loadAnnotations = useCallback(async (silent = false) => {
    if (!selectedPageId) {
      setAnnotations([])
      return
    }

    try {
      const data = await api.listWikiAnnotations(Number(selectedPageId))
      setAnnotations(data)
    } catch (err) {
      if (silent) throw err
      setAnnotations([])
    }
  }, [selectedPageId])

  useEffect(() => {
    if (projectId) void loadPages()
  }, [loadPages, projectId])

  useEffect(() => {
    void loadAnnotations()
    setSelectedAnnotationId(null)
  }, [loadAnnotations])

  useEffect(() => {
    return registerSyncTask(`project:${projectId}:wiki`, async () => {
      await loadPages(true)
      await loadAnnotations(true)
    })
  }, [loadAnnotations, loadPages, projectId, registerSyncTask])

  const selectPage = useCallback((pageId: number) => {
    setSearchParams(prev => {
      const next = new URLSearchParams(prev)
      next.set('page', String(pageId))
      next.delete('annotation')
      return next
    })
  }, [setSearchParams])

  const clearSelectedPage = useCallback(() => {
    setSearchParams(prev => {
      const next = new URLSearchParams(prev)
      next.delete('page')
      next.delete('annotation')
      return next
    })
  }, [setSearchParams])

  const handleCreatePage = useCallback(async (title: string, parentId: number | null): Promise<WikiPage | null> => {
    if (!title.trim() || !projectId) return null
    try {
      const newPage = await api.createWikiPage(Number(projectId), title.trim(), parentId)
      setPages(prev => [...prev, newPage])
      selectPage(newPage.id)
      return newPage
    } catch (err) {
      dialog.notify(err instanceof Error ? err.message : 'Failed to create page', 'error')
      return null
    }
  }, [dialog, projectId, selectPage])

  const handlePageUpdate = useCallback((updated: WikiPage) => {
    setPages(prev => prev.map(p => (p.id === updated.id ? { ...p, ...updated } : p)))
  }, [])

  const handleMovePage = useCallback(async (pageId: number, parentId: number | null) => {
    // Optimistic: reflect the move immediately, then reconcile with the server's response.
    const snapshot = pages
    setPages(prev => prev.map(p => (p.id === pageId ? { ...p, parent_id: parentId } : p)))
    try {
      const updated = await api.updateWikiPage(pageId, { parent_id: parentId })
      setPages(prev => prev.map(p => (p.id === pageId ? { ...p, ...updated } : p)))
    } catch (err) {
      setPages(snapshot)
      dialog.notify(err instanceof Error ? err.message : 'Failed to move page', 'error')
    }
  }, [dialog, pages])

  const handleDeletePage = useCallback(async (pageId: number) => {
    const page = pages.find(p => p.id === pageId)
    const childCount = pages.filter(p => p.parent_id === pageId).length
    const message = childCount > 0
      ? `Delete "${page?.title ?? 'this page'}"? Its ${childCount} sub-page${childCount === 1 ? '' : 's'} will be kept and moved up one level.`
      : `"${page?.title ?? 'this page'}" will be permanently deleted.`
    if (!(await dialog.confirm({ title: 'Delete page?', message, confirmLabel: 'Delete', danger: true }))) return
    try {
      await api.deleteWikiPage(pageId)
      const grandparent = page?.parent_id ?? null
      setPages(prev => prev
        .filter(p => p.id !== pageId)
        .map(p => (p.parent_id === pageId ? { ...p, parent_id: grandparent } : p)))
      if (selectedPageId === String(pageId)) clearSelectedPage()
    } catch (err) {
      dialog.notify(err instanceof Error ? err.message : 'Failed to delete page', 'error')
    }
  }, [clearSelectedPage, dialog, pages, selectedPageId])

  const handleAnnotationCreate = useCallback(async (info: {
    startOffset: number; endOffset: number; selectedText: string; color: AnnotationColor
  }) => {
    if (!selectedPageId) return
    try {
      const annotation = await api.createWikiAnnotation(Number(selectedPageId), {
        start_offset: info.startOffset,
        end_offset: info.endOffset,
        selected_text: info.selectedText,
        color: info.color,
      })
      setAnnotations(prev => [...prev, annotation])
      setSelectedAnnotationId(annotation.id)
      setShowAnnotationSidebar(true)
    } catch { /* ignore */ }
  }, [selectedPageId])

  const handleAnnotationClick = useCallback((annotationId: number) => {
    setSelectedAnnotationId(prev => prev === annotationId ? null : annotationId)
    setShowAnnotationSidebar(true)
  }, [])

  const handleAnnotationUpdate = useCallback((updated: WikiAnnotation) => {
    setAnnotations(prev => prev.map(a => a.id === updated.id ? updated : a))
  }, [])

  const handleAnnotationDelete = useCallback((annotationId: number) => {
    setAnnotations(prev => prev.filter(a => a.id !== annotationId))
    if (selectedAnnotationId === annotationId) setSelectedAnnotationId(null)
  }, [selectedAnnotationId])

  const handleCommentCreate = useCallback((annotationId: number, comment: AnnotationComment) => {
    setAnnotations(prev => prev.map(a =>
      a.id === annotationId ? { ...a, comments: [...a.comments, comment] } : a
    ))
  }, [])

  const handleCommentUpdate = useCallback((updated: AnnotationComment) => {
    setAnnotations(prev => prev.map(a =>
      a.id === updated.annotation_id
        ? { ...a, comments: a.comments.map(c => c.id === updated.id ? updated : c) }
        : a
    ))
  }, [])

  const handleCommentDelete = useCallback((annotationId: number, commentId: number) => {
    setAnnotations(prev => prev.map(a =>
      a.id === annotationId
        ? { ...a, comments: a.comments.filter(c => c.id !== commentId) }
        : a
    ))
  }, [])

  const selectedPage = pages.find(p => p.id === Number(selectedPageId))
  const annotationTotal = annotations.length
  const unresolvedAnnotationTotal = annotations.filter(a => !a.resolved).length
  const hasAnnotationComments = annotations.some(a => a.comments.length > 0)
  const ancestors = selectedPage ? getWikiAncestors(pages, selectedPage.id) : []
  const childPages = selectedPage
    ? pages.filter(p => p.parent_id === selectedPage.id).sort((a, b) => a.position - b.position || a.title.localeCompare(b.title))
    : []
  const selectedDepth = selectedPage ? getWikiDepth(pages, selectedPage.id) : 0

  return (
    <div className="flex flex-1 overflow-hidden">
      {/* Sidebar */}
      <div className="w-64 border-r border-dark-border-subtle bg-dark-bg-secondary flex flex-col">
        <div className="p-4 border-b border-dark-border-subtle">
          <input
            type="text"
            placeholder="Search pages..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full px-3 py-2 bg-dark-bg-primary border border-dark-border-subtle rounded text-sm text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:border-primary-500"
          />
        </div>

        {loading ? (
          <div className="p-4 text-center text-dark-text-tertiary text-sm">Loading...</div>
        ) : error ? (
          <div className="p-4 text-center text-red-400 text-sm">{error}</div>
        ) : (
          <WikiPageTree
            projectId={Number(projectId)}
            pages={pages}
            selectedPageId={selectedPageId ? Number(selectedPageId) : null}
            searchQuery={searchQuery}
            onSelect={selectPage}
            onCreate={handleCreatePage}
            onMove={handleMovePage}
            onDelete={handleDeletePage}
          />
        )}
      </div>

      {/* Main content */}
      <div className="flex-1 flex overflow-hidden relative">
        <div className="flex-1 flex flex-col overflow-hidden">
        {selectedPage && (
          <>
            {/* Breadcrumbs */}
            <nav
              aria-label="Page location"
              className="flex items-center justify-between gap-3 px-6 py-2 border-b border-dark-border-subtle/60 bg-dark-bg-secondary/40 text-xs"
            >
              <ol className="flex items-center gap-1 min-w-0 overflow-hidden">
                <li className="shrink-0">
                  <button
                    type="button"
                    onClick={clearSelectedPage}
                    className="text-dark-text-tertiary hover:text-dark-text-primary transition-colors"
                  >
                    Wiki
                  </button>
                </li>
                {ancestors.map(a => (
                  <li key={a.id} className="flex items-center gap-1 min-w-0">
                    <span className="text-dark-text-quaternary" aria-hidden="true">/</span>
                    <button
                      type="button"
                      onClick={() => selectPage(a.id)}
                      className="truncate max-w-[12rem] text-dark-text-tertiary hover:text-dark-text-primary transition-colors"
                    >
                      {a.title}
                    </button>
                  </li>
                ))}
                <li className="flex items-center gap-1 min-w-0" aria-current="page">
                  <span className="text-dark-text-quaternary" aria-hidden="true">/</span>
                  <span className="truncate max-w-[16rem] text-dark-text-primary font-medium">{selectedPage.title}</span>
                </li>
              </ol>
              <span className="shrink-0 text-dark-text-quaternary tabular-nums" title="Nesting level">
                Level {selectedDepth}/{WIKI_MAX_DEPTH}
              </span>
            </nav>

            {childPages.length > 0 && (
              <div className="flex items-center gap-2 px-6 py-1.5 border-b border-dark-border-subtle/60 text-xs overflow-x-auto">
                <span className="shrink-0 text-dark-text-tertiary">Sub-pages:</span>
                {childPages.map(c => (
                  <button
                    key={c.id}
                    type="button"
                    onClick={() => selectPage(c.id)}
                    className="shrink-0 px-2 py-0.5 rounded-full bg-dark-bg-tertiary text-dark-text-secondary hover:text-dark-text-primary hover:bg-dark-bg-tertiary/80 transition-colors max-w-[14rem] truncate"
                  >
                    {c.title}
                  </button>
                ))}
              </div>
            )}
          </>
        )}
        {selectedPage ? (
          <WikiEditor
            key={selectedPage.id}
            page={selectedPage}
            annotations={annotations}
            selectedAnnotationId={selectedAnnotationId}
            showAnnotationHighlights={pinnedAnnotations || showAnnotationSidebar}
            onAnnotationCreate={handleAnnotationCreate}
            onAnnotationClick={handleAnnotationClick}
            onAnnotationUpdate={handleAnnotationUpdate}
            onAnnotationDelete={handleAnnotationDelete}
            onCommentCreate={handleCommentCreate}
            onCommentUpdate={handleCommentUpdate}
            onCommentDelete={handleCommentDelete}
            showResolved={showResolved}
            onToggleShowResolved={() => setShowResolved(v => !v)}
            onPageUpdate={handlePageUpdate}
          />
        ) : (
          <div className="flex-1 flex items-center justify-center text-dark-text-tertiary">
            <div className="text-center">
              <svg className="w-16 h-16 mx-auto mb-4 text-dark-text-tertiary/50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
              </svg>
              <p className="text-lg">Select a page or create a new one</p>
            </div>
          </div>
        )}
        </div>

        {/* Pen icon: reveal sidebar when not visible */}
        {selectedPage && !pinnedAnnotations && !showAnnotationSidebar && (
          <button
            onClick={() => setShowAnnotationSidebar(true)}
            className={`absolute right-3 top-1/2 -translate-y-1/2 z-10 flex items-center gap-2 rounded-full border px-3 py-2 text-xs font-semibold shadow-linear-lg transition-all ${
              unresolvedAnnotationTotal > 0
                ? 'border-red-500/35 bg-red-500/10 text-red-500 shadow-[0_0_26px_rgba(239,68,68,0.32)] hover:bg-red-500/15'
                : annotationTotal > 0
                  ? 'border-amber-500/35 bg-amber-500/10 text-amber-500 shadow-[0_0_20px_rgba(245,158,11,0.22)] hover:bg-amber-500/15'
                  : 'border-dark-border-subtle bg-dark-bg-elevated text-dark-text-tertiary hover:text-primary-400 hover:bg-primary-500/10'
            }`}
            title="Show annotations"
          >
            <svg className="w-4 h-4" fill={hasAnnotationComments ? 'currentColor' : 'none'} stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d={unresolvedAnnotationTotal > 0 ? 'M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z' : 'M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z'} />
            </svg>
            {annotationTotal > 0 && <span>{unresolvedAnnotationTotal}/{annotationTotal}</span>}
          </button>
        )}

        {/* Annotation sidebar with pin/close controls */}
        {selectedPage && (pinnedAnnotations || showAnnotationSidebar) && (
          <div className="flex flex-col border-l border-dark-border-subtle w-80 flex-shrink-0">
            <div className="flex items-center justify-between px-3 py-2 border-b border-dark-border-subtle bg-dark-bg-secondary">
              <span className="text-xs font-semibold text-dark-text-secondary uppercase tracking-wide">
                Annotations
                {annotationTotal > 0 && (
                  <span className={`ml-2 rounded-full px-2 py-0.5 text-[10px] font-bold ${
                    unresolvedAnnotationTotal > 0 ? 'bg-red-500/10 text-red-500' : 'bg-amber-500/10 text-amber-500'
                  }`}>
                    {unresolvedAnnotationTotal}/{annotationTotal}
                  </span>
                )}
              </span>
              <div className="flex items-center gap-1">
                <button
                  onClick={() => setPinnedAnnotations(v => !v)}
                  className={`p-1 rounded transition-colors ${pinnedAnnotations ? 'text-primary-400 bg-primary-500/10' : 'text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary'}`}
                  title={pinnedAnnotations ? 'Unpin sidebar' : 'Pin sidebar'}
                >
                  <svg className="w-3.5 h-3.5" fill={pinnedAnnotations ? 'currentColor' : 'none'} stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z" />
                  </svg>
                </button>
                {!pinnedAnnotations && (
                  <button
                    onClick={() => setShowAnnotationSidebar(false)}
                    className="p-1 rounded text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary transition-colors"
                    title="Hide annotations"
                  >
                    <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                )}
              </div>
            </div>
            <div className="flex-1 overflow-hidden">
              <WikiAnnotationSidebar
                annotations={annotations}
                selectedAnnotationId={selectedAnnotationId}
                showResolved={showResolved}
                projectId={Number(projectId)}
                onAnnotationSelect={setSelectedAnnotationId}
                onAnnotationUpdate={handleAnnotationUpdate}
                onAnnotationDelete={handleAnnotationDelete}
                onCommentCreate={handleCommentCreate}
                onCommentUpdate={handleCommentUpdate}
                onCommentDelete={handleCommentDelete}
                onToggleShowResolved={() => setShowResolved(v => !v)}
              />
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
