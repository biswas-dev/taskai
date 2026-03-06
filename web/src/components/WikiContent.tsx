import { useCallback, useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, WikiPage, WikiAnnotation, AnnotationColor, AnnotationComment } from '../lib/api'
import WikiEditor from './WikiEditor'
import WikiAnnotationSidebar from './WikiAnnotationSidebar'

interface WikiContentProps {
  projectId: string
}

export default function WikiContent({ projectId }: WikiContentProps) {
  const [searchParams, setSearchParams] = useSearchParams()
  const selectedPageId = searchParams.get('page')
  const annotationParam = searchParams.get('annotation')

  const [pages, setPages] = useState<WikiPage[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [newPageTitle, setNewPageTitle] = useState('')
  const [showNewPageInput, setShowNewPageInput] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')

  const [annotations, setAnnotations] = useState<WikiAnnotation[]>([])
  const [selectedAnnotationId, setSelectedAnnotationId] = useState<number | null>(null)
  const [showAnnotationSidebar, setShowAnnotationSidebar] = useState(false)
  const [pinnedAnnotations, setPinnedAnnotations] = useState(false)
  const [showResolved, setShowResolved] = useState(false)

  useEffect(() => {
    if (projectId) loadPages()
  }, [projectId]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (selectedPageId) {
      api.listWikiAnnotations(Number(selectedPageId))
        .then(data => setAnnotations(data))
        .catch(() => setAnnotations([]))
      setSelectedAnnotationId(null)
    } else {
      setAnnotations([])
    }
  }, [selectedPageId])

  // Deep-link to annotation from ?annotation=X (e.g. notification clicks)
  useEffect(() => {
    if (!annotationParam || !annotations.length) return
    const id = Number(annotationParam)
    if (!id) return
    setSelectedAnnotationId(id)
    setShowAnnotationSidebar(true)
  }, [annotationParam, annotations])

  const loadPages = async () => {
    try {
      setLoading(true)
      setError(null)
      const pagesData = await api.getWikiPages(Number(projectId))
      setPages(pagesData.sort((a, b) => b.updated_at.localeCompare(a.updated_at)))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load wiki pages')
    } finally {
      setLoading(false)
    }
  }

  const handleCreatePage = async () => {
    if (!newPageTitle.trim() || !projectId) return
    try {
      setCreating(true)
      const newPage = await api.createWikiPage(Number(projectId), newPageTitle.trim())
      setPages([newPage, ...pages])
      setNewPageTitle('')
      setShowNewPageInput(false)
      setSearchParams(prev => {
        const next = new URLSearchParams(prev)
        next.set('page', String(newPage.id))
        return next
      })
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to create page')
    } finally {
      setCreating(false)
    }
  }

  const handleDeletePage = async (pageId: number) => {
    if (!confirm('Are you sure you want to delete this wiki page?')) return
    try {
      await api.deleteWikiPage(pageId)
      setPages(pages.filter(p => p.id !== pageId))
      if (selectedPageId === String(pageId)) {
        setSearchParams(prev => {
          const next = new URLSearchParams(prev)
          next.delete('page')
          return next
        })
      }
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to delete page')
    }
  }

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
  const filteredPages = searchQuery
    ? pages.filter(p => p.title.toLowerCase().includes(searchQuery.toLowerCase()))
    : pages

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
            className="w-full px-3 py-2 bg-dark-bg-primary border border-dark-border-subtle rounded text-sm text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:border-dark-accent-primary"
          />
        </div>

        <div className="p-4 border-b border-dark-border-subtle">
          {!showNewPageInput ? (
            <button
              onClick={() => setShowNewPageInput(true)}
              className="w-full px-3 py-2 bg-dark-accent-primary text-white rounded hover:bg-dark-accent-primary/90 transition-colors text-sm font-medium"
            >
              + New Page
            </button>
          ) : (
            <div className="space-y-2">
              <input
                type="text"
                placeholder="Page title..."
                value={newPageTitle}
                onChange={(e) => setNewPageTitle(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleCreatePage()}
                autoFocus
                className="w-full px-3 py-2 bg-dark-bg-primary border border-dark-border-subtle rounded text-sm text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:border-dark-accent-primary"
              />
              <div className="flex gap-2">
                <button
                  onClick={handleCreatePage}
                  disabled={creating || !newPageTitle.trim()}
                  className="flex-1 px-3 py-1.5 bg-dark-accent-primary text-white rounded hover:bg-dark-accent-primary/90 transition-colors text-sm disabled:opacity-50"
                >
                  {creating ? 'Creating...' : 'Create'}
                </button>
                <button
                  onClick={() => {
                    setShowNewPageInput(false)
                    setNewPageTitle('')
                  }}
                  className="flex-1 px-3 py-1.5 bg-dark-bg-tertiary text-dark-text-secondary rounded hover:bg-dark-bg-tertiary/80 transition-colors text-sm"
                >
                  Cancel
                </button>
              </div>
            </div>
          )}
        </div>

        <div className="flex-1 overflow-y-auto">
          {loading ? (
            <div className="p-4 text-center text-dark-text-tertiary text-sm">Loading...</div>
          ) : error ? (
            <div className="p-4 text-center text-red-400 text-sm">{error}</div>
          ) : filteredPages.length === 0 ? (
            <div className="p-4 text-center text-dark-text-tertiary text-sm">
              {searchQuery ? 'No matching pages' : 'No pages yet'}
            </div>
          ) : (
            <div className="py-2">
              {filteredPages.map((page) => (
                <div
                  key={page.id}
                  className={`px-4 py-2 cursor-pointer hover:bg-dark-bg-tertiary transition-colors group ${
                    selectedPageId === String(page.id) ? 'bg-dark-bg-tertiary border-l-2 border-dark-accent-primary' : ''
                  }`}
                  onClick={() => setSearchParams(prev => {
                    const next = new URLSearchParams(prev)
                    next.set('page', String(page.id))
                    return next
                  })}
                >
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-dark-text-primary truncate">{page.title}</span>
                    <button
                      onClick={(e) => {
                        e.stopPropagation()
                        handleDeletePage(page.id)
                      }}
                      className="opacity-0 group-hover:opacity-100 p-1 hover:bg-red-500/20 rounded transition-opacity"
                      title="Delete page"
                    >
                      <svg className="w-4 h-4 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                      </svg>
                    </button>
                  </div>
                  <div className="text-xs text-dark-text-tertiary mt-1">
                    Updated {new Date(page.updated_at).toLocaleDateString()}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Main content */}
      <div className="flex-1 flex overflow-hidden relative">
        <div className="flex-1 flex flex-col overflow-hidden">
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
            className="absolute right-0 top-1/2 -translate-y-1/2 z-10 p-1.5 bg-dark-bg-secondary border border-r-0 border-dark-border-subtle rounded-l-lg text-dark-text-tertiary hover:text-primary-400 hover:bg-primary-500/10 transition-colors"
            title="Show annotations"
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
            </svg>
          </button>
        )}

        {/* Annotation sidebar with pin/close controls */}
        {selectedPage && (pinnedAnnotations || showAnnotationSidebar) && (
          <div className="flex flex-col border-l border-dark-border-subtle w-80 flex-shrink-0">
            <div className="flex items-center justify-between px-3 py-2 border-b border-dark-border-subtle bg-dark-bg-secondary">
              <span className="text-xs font-semibold text-dark-text-secondary uppercase tracking-wide">Annotations</span>
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
