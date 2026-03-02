import { useEffect, useState } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { api, WikiPage, Project } from '../lib/api'
import WikiEditor from '../components/WikiEditor'

export default function Wiki() {
  const { projectId } = useParams<{ projectId: string }>()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const selectedPageId = searchParams.get('page')

  const [project, setProject] = useState<Project | null>(null)
  const [pages, setPages] = useState<WikiPage[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [newPageTitle, setNewPageTitle] = useState('')
  const [showNewPageInput, setShowNewPageInput] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')

  useEffect(() => {
    if (projectId) {
      loadProject()
      loadPages()
    }
  }, [projectId])

  const loadProject = async () => {
    try {
      const proj = await api.getProject(Number(projectId))
      setProject(proj)
    } catch (err) {
      // non-critical load failure
    }
  }

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
      // Select the new page
      setSearchParams({ page: String(newPage.id) })
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
        setSearchParams({})
      }
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to delete page')
    }
  }

  const selectedPage = pages.find(p => p.id === Number(selectedPageId))

  const filteredPages = searchQuery
    ? pages.filter(p => p.title.toLowerCase().includes(searchQuery.toLowerCase()))
    : pages

  return (
    <div className="flex flex-col h-full">
      {/* Project Header */}
      <div className="bg-dark-bg-secondary border-b border-dark-border-subtle">
        {/* Top bar with project info */}
        <div className="px-6 py-4 flex items-start justify-between gap-4">
          <div className="flex-1 min-w-0">
            <h1 className="text-xl font-semibold text-dark-text-primary truncate">
              {project?.name}
            </h1>
            {project?.description && (
              <p className="mt-1 text-sm text-dark-text-tertiary line-clamp-1">{project.description}</p>
            )}
          </div>
        </div>

        {/* Navigation tabs */}
        <div className="px-6 flex items-end justify-between border-t border-dark-border-subtle/50">
          <div className="flex items-center gap-1">
            <button
              onClick={() => navigate(`/app/projects/${projectId}`)}
              className="px-4 py-3 text-sm font-medium text-dark-text-secondary hover:text-dark-text-primary transition-colors"
            >
              Board
            </button>
            <button
              onClick={() => navigate(`/app/projects/${projectId}/wiki`)}
              className="relative px-4 py-3 text-sm font-medium text-primary-400 transition-colors"
            >
              Wiki
              <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-primary-500"></div>
            </button>
            <button
              onClick={() => navigate(`/app/projects/${projectId}/settings`)}
              className="px-4 py-3 text-sm font-medium text-dark-text-secondary hover:text-dark-text-primary transition-colors"
            >
              Settings
            </button>
          </div>
          <div className="py-3"></div>
        </div>
      </div>

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
                  onClick={() => setSearchParams({ page: String(page.id) })}
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
      <div className="flex-1 flex flex-col">
        {selectedPage ? (
          <WikiEditor key={selectedPage.id} page={selectedPage} />
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
      </div>
    </div>
  )
}
