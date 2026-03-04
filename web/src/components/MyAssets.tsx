import { useState, useEffect, useCallback, useRef } from 'react'
import Card from './ui/Card'
import Button from './ui/Button'
import FormError from './ui/FormError'
import { apiClient, type Asset, type Project } from '../lib/api'

function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

type FileTypeFilter = '' | 'image' | 'video' | 'pdf'

export default function MyAssets({ projectId }: { projectId: number }) {
  const [assets, setAssets] = useState<Asset[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [searchQuery, setSearchQuery] = useState('')
  const [fileTypeFilter, setFileTypeFilter] = useState<FileTypeFilter>('')
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editingValue, setEditingValue] = useState('')
  const [savingAlt, setSavingAlt] = useState(false)
  const [deletingId, setDeletingId] = useState<number | null>(null)
  const [confirmDeleteId, setConfirmDeleteId] = useState<number | null>(null)
  const [shareMenuId, setShareMenuId] = useState<number | null>(null)
  const [otherProjects, setOtherProjects] = useState<Project[]>([])
  const debounceRef = useRef<ReturnType<typeof setTimeout>>()
  const shareMenuRef = useRef<HTMLDivElement>(null)

  const loadAssets = useCallback(async (q: string, type: FileTypeFilter) => {
    try {
      setLoading(true)
      setError('')
      const data = await apiClient.getAssets(projectId, {
        q: q || undefined,
        type: type || undefined,
        limit: 50,
      })
      setAssets(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load assets')
    } finally {
      setLoading(false)
    }
  }, [projectId])

  useEffect(() => {
    loadAssets(searchQuery, fileTypeFilter)
  }, [fileTypeFilter, loadAssets]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    apiClient.getProjects().then(projects => {
      setOtherProjects(projects.filter(p => p.id !== projectId))
    }).catch(() => {})
  }, [projectId])

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (shareMenuRef.current && !shareMenuRef.current.contains(e.target as Node)) {
        setShareMenuId(null)
      }
    }
    if (shareMenuId !== null) document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [shareMenuId])

  const handleSearchChange = (value: string) => {
    setSearchQuery(value)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      loadAssets(value, fileTypeFilter)
    }, 300)
  }

  const handleStartEdit = (asset: Asset) => {
    setEditingId(asset.id)
    setEditingValue(asset.alt_name || '')
  }

  const handleCancelEdit = () => {
    setEditingId(null)
    setEditingValue('')
  }

  const handleSaveAlt = async (id: number) => {
    try {
      setSavingAlt(true)
      setError('')
      await apiClient.updateAttachment(id, { alt_name: editingValue })
      setAssets(prev => prev.map(a => a.id === id ? { ...a, alt_name: editingValue } : a))
      setEditingId(null)
      setEditingValue('')
      setSuccess('Alt text updated')
      setTimeout(() => setSuccess(''), 3000)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update alt text')
    } finally {
      setSavingAlt(false)
    }
  }

  const handleShareAsset = async (assetId: number, targetProjectId: number) => {
    try {
      await apiClient.shareAttachment(assetId, targetProjectId)
      setShareMenuId(null)
      setSuccess('Asset shared successfully')
      setTimeout(() => setSuccess(''), 3000)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to share asset')
    }
  }

  const handleUnshareAsset = async (assetId: number) => {
    try {
      await apiClient.unshareAttachment(assetId, projectId)
      setAssets(prev => prev.filter(a => a.id !== assetId))
      setSuccess('Asset removed from this project')
      setTimeout(() => setSuccess(''), 3000)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to remove asset')
    }
  }

  const handleDelete = async (id: number) => {
    try {
      setDeletingId(id)
      setError('')
      await apiClient.deleteAttachment(id)
      setAssets(prev => prev.filter(a => a.id !== id))
      setConfirmDeleteId(null)
      setSuccess('File deleted')
      setTimeout(() => setSuccess(''), 3000)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete file')
    } finally {
      setDeletingId(null)
    }
  }

  const renderThumbnail = (asset: Asset) => {
    if (asset.file_type === 'image') {
      return (
        <img
          src={asset.cloudinary_url}
          alt={asset.alt_name || asset.filename}
          className="w-full h-36 object-cover rounded-t-lg"
          loading="lazy"
        />
      )
    }
    if (asset.file_type === 'video') {
      return (
        <div className="w-full h-36 bg-dark-bg-primary rounded-t-lg flex items-center justify-center">
          <svg className="w-12 h-12 text-purple-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
          </svg>
        </div>
      )
    }
    // PDF or other
    return (
      <div className="w-full h-36 bg-dark-bg-primary rounded-t-lg flex items-center justify-center">
        <svg className="w-12 h-12 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z" />
        </svg>
      </div>
    )
  }

  const filterButtons: { label: string; value: FileTypeFilter }[] = [
    { label: 'All', value: '' },
    { label: 'Images', value: 'image' },
    { label: 'Videos', value: 'video' },
    { label: 'PDFs', value: 'pdf' },
  ]

  return (
    <Card className="shadow-md">
      <div className="p-6 sm:p-8 flex items-start gap-4">
        <div className="flex-shrink-0 w-10 h-10 bg-teal-500/10 rounded-lg flex items-center justify-center">
          <svg className="w-6 h-6 text-teal-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
          </svg>
        </div>
        <div className="flex-1 min-w-0">
          <h2 className="text-xl font-semibold text-dark-text-primary mb-1">My Assets</h2>
          <p className="text-sm text-dark-text-secondary mb-6">Manage files uploaded across your projects</p>

          {success && (
            <div className="mb-4 p-4 bg-success-500/10 border-l-4 border-success-400 rounded-r-lg">
              <div className="flex items-center">
                <svg className="w-5 h-5 text-success-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                </svg>
                <span className="text-success-300 font-medium">{success}</span>
              </div>
            </div>
          )}

          {error && <FormError message={error} className="mb-4" />}

          {/* Search and Filter */}
          <div className="flex flex-col sm:flex-row gap-3 mb-6">
            <div className="relative flex-1">
              <svg className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-dark-text-tertiary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
              </svg>
              <input
                type="text"
                placeholder="Search by name..."
                value={searchQuery}
                onChange={e => handleSearchChange(e.target.value)}
                className="w-full pl-10 pr-4 py-2 bg-dark-bg-primary border border-dark-border-subtle rounded-lg text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:ring-1 focus:ring-primary-500/50 focus:border-primary-500/50 text-sm"
              />
            </div>
            <div className="flex gap-1 bg-dark-bg-primary border border-dark-border-subtle rounded-lg p-1">
              {filterButtons.map(btn => (
                <button
                  key={btn.value}
                  onClick={() => setFileTypeFilter(btn.value)}
                  className={`px-3 py-1.5 text-xs font-medium rounded-md transition-colors ${
                    fileTypeFilter === btn.value
                      ? 'bg-teal-500/20 text-teal-300'
                      : 'text-dark-text-tertiary hover:text-dark-text-secondary'
                  }`}
                >
                  {btn.label}
                </button>
              ))}
            </div>
          </div>

          {/* Asset Grid */}
          {loading ? (
            <div className="flex items-center justify-center py-12">
              <svg className="animate-spin h-6 w-6 text-teal-400" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
              </svg>
              <span className="ml-3 text-dark-text-secondary text-sm">Loading assets...</span>
            </div>
          ) : assets.length === 0 ? (
            <div className="text-center py-12">
              <svg className="mx-auto w-12 h-12 text-dark-text-tertiary mb-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
              </svg>
              <p className="text-dark-text-secondary text-sm">
                {searchQuery || fileTypeFilter ? 'No files match your filters' : 'No files uploaded yet'}
              </p>
              <p className="text-dark-text-tertiary text-xs mt-1">Upload files to tasks to see them here</p>
            </div>
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
              {assets.map(asset => (
                <div key={asset.id} className="bg-dark-bg-primary border border-dark-border-subtle rounded-lg overflow-hidden group hover:border-dark-border-medium transition-colors">
                  {/* Thumbnail */}
                  <a href={asset.cloudinary_url} target="_blank" rel="noopener noreferrer" className="block">
                    {renderThumbnail(asset)}
                  </a>

                  {/* Info */}
                  <div className="p-3 space-y-2">
                    {/* Alt name / edit */}
                    {editingId === asset.id ? (
                      <div className="flex gap-2">
                        <input
                          type="text"
                          value={editingValue}
                          onChange={e => setEditingValue(e.target.value)}
                          className="flex-1 px-2 py-1 bg-dark-bg-elevated border border-dark-border-medium rounded text-sm text-dark-text-primary focus:outline-none focus:ring-1 focus:ring-teal-500/50"
                          placeholder="Alt text..."
                          autoFocus
                          onKeyDown={e => {
                            if (e.key === 'Enter') handleSaveAlt(asset.id)
                            if (e.key === 'Escape') handleCancelEdit()
                          }}
                        />
                        <Button size="sm" onClick={() => handleSaveAlt(asset.id)} loading={savingAlt} className="!px-2 !py-1">
                          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                          </svg>
                        </Button>
                        <Button size="sm" variant="ghost" onClick={handleCancelEdit} className="!px-2 !py-1">
                          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                          </svg>
                        </Button>
                      </div>
                    ) : (
                      <div className="flex items-start justify-between gap-2">
                        <div className="min-w-0">
                          <p className="text-sm font-medium text-dark-text-primary truncate" title={asset.alt_name || asset.filename}>
                            {asset.alt_name || asset.filename}
                          </p>
                          {asset.alt_name && asset.alt_name !== asset.filename && (
                            <p className="text-xs text-dark-text-tertiary truncate" title={asset.filename}>{asset.filename}</p>
                          )}
                        </div>
                        <div className="flex flex-col items-end gap-1">
                          {asset.is_shared && (
                            <span className="flex-shrink-0 text-xs text-amber-400 bg-amber-500/10 px-2 py-0.5 rounded">shared</span>
                          )}
                          {!asset.is_owner && !asset.is_shared && (
                            <span className="flex-shrink-0 text-xs text-dark-text-tertiary bg-dark-bg-elevated px-2 py-0.5 rounded">View only</span>
                          )}
                        </div>
                      </div>
                    )}

                    {/* Meta */}
                    <div className="flex items-center gap-2 text-xs text-dark-text-tertiary">
                      <span>{formatFileSize(asset.file_size)}</span>
                      <span>&middot;</span>
                      <span>{formatDate(asset.created_at)}</span>
                      {asset.user_name && (
                        <>
                          <span>&middot;</span>
                          <span className="truncate">{asset.user_name}</span>
                        </>
                      )}
                    </div>

                    {/* Actions */}
                    {editingId !== asset.id && (
                      <div className="flex gap-2 pt-1 items-center">
                        {asset.is_shared ? (
                          <button
                            onClick={() => handleUnshareAsset(asset.id)}
                            className="text-xs text-amber-400 hover:text-amber-300 transition-colors"
                            title="Remove from this project"
                          >
                            Remove
                          </button>
                        ) : asset.is_owner ? (
                          <>
                            <button
                              onClick={() => handleStartEdit(asset)}
                              className="text-xs text-dark-text-tertiary hover:text-teal-400 transition-colors"
                              title="Edit alt text"
                            >
                              Edit
                            </button>
                            <div className="relative" ref={shareMenuId === asset.id ? shareMenuRef : undefined}>
                              <button
                                onClick={() => setShareMenuId(shareMenuId === asset.id ? null : asset.id)}
                                className="text-xs text-dark-text-tertiary hover:text-primary-400 transition-colors"
                                title="Share to another project"
                              >
                                Share
                              </button>
                              {shareMenuId === asset.id && (
                                <div className="absolute left-0 bottom-6 z-10 w-48 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg shadow-lg py-1">
                                  {otherProjects.length === 0 ? (
                                    <p className="px-3 py-2 text-xs text-dark-text-tertiary">No other projects</p>
                                  ) : (
                                    otherProjects.map(p => (
                                      <button
                                        key={p.id}
                                        onClick={() => handleShareAsset(asset.id, p.id!)}
                                        className="w-full text-left px-3 py-2 text-sm text-dark-text-primary hover:bg-dark-bg-tertiary transition-colors truncate"
                                      >
                                        {p.name}
                                      </button>
                                    ))
                                  )}
                                </div>
                              )}
                            </div>
                            {confirmDeleteId === asset.id ? (
                              <div className="flex items-center gap-2">
                                <span className="text-xs text-danger-400">Delete?</span>
                                <button
                                  onClick={() => handleDelete(asset.id)}
                                  disabled={deletingId === asset.id}
                                  className="text-xs text-danger-400 hover:text-danger-300 font-medium transition-colors disabled:opacity-50"
                                >
                                  {deletingId === asset.id ? 'Deleting...' : 'Yes'}
                                </button>
                                <button
                                  onClick={() => setConfirmDeleteId(null)}
                                  className="text-xs text-dark-text-tertiary hover:text-dark-text-secondary transition-colors"
                                >
                                  No
                                </button>
                              </div>
                            ) : (
                              <button
                                onClick={() => setConfirmDeleteId(asset.id)}
                                className="text-xs text-dark-text-tertiary hover:text-danger-400 transition-colors"
                                title="Delete file"
                              >
                                Delete
                              </button>
                            )}
                          </>
                        ) : null}
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </Card>
  )
}
