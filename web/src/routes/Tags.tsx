import { useState, useEffect, useRef } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import Card from '../components/ui/Card'
import Button from '../components/ui/Button'
import TextInput from '../components/ui/TextInput'
import FormError from '../components/ui/FormError'
import { apiClient, type Project } from '../lib/api'

interface Tag {
  id: number
  name: string
  color: string
  is_shared?: boolean
  created_at: string
}

export default function Tags() {
  const navigate = useNavigate()
  const { projectId } = useParams<{ projectId: string }>()
  const projectIdNum = Number(projectId)
  const [tags, setTags] = useState<Tag[]>([])
  const [showForm, setShowForm] = useState(false)
  const [formData, setFormData] = useState({
    name: '',
    color: '#3B82F6',
  })
  const [editingId, setEditingId] = useState<number | null>(null)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [shareMenuId, setShareMenuId] = useState<number | null>(null)
  const [otherProjects, setOtherProjects] = useState<Project[]>([])
  const shareMenuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (projectIdNum) loadTags()
  }, [projectIdNum]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    apiClient.getProjects().then(projects => {
      setOtherProjects(projects.filter(p => p.id !== projectIdNum))
    }).catch(() => {})
  }, [projectIdNum])

  // Close share menu on outside click
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (shareMenuRef.current && !shareMenuRef.current.contains(e.target as Node)) {
        setShareMenuId(null)
      }
    }
    if (shareMenuId !== null) document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [shareMenuId])

  const loadTags = async () => {
    try {
      const data = await apiClient.getTags(projectIdNum)
      setTags(data)
    } catch (error: unknown) {
      console.error('Failed to load tags:', error)
    }
  }

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setSuccess('')

    if (!formData.name.trim()) {
      setError('Tag name is required')
      return
    }

    try {
      if (editingId) {
        await apiClient.updateTag(editingId, formData)
        setSuccess('Tag updated successfully')
      } else {
        await apiClient.createTag(projectIdNum, formData)
        setSuccess('Tag created successfully')
      }

      setShowForm(false)
      setEditingId(null)
      setFormData({ name: '', color: '#3B82F6' })
      loadTags()
    } catch (error: unknown) {
      setError(error instanceof Error ? error.message : 'Failed to save tag')
    }
  }

  const handleEdit = (tag: Tag) => {
    setEditingId(tag.id)
    setFormData({
      name: tag.name,
      color: tag.color,
    })
    setShowForm(true)
  }

  const handleDelete = async (id: number) => {
    if (!confirm('Are you sure you want to delete this tag?')) {
      return
    }

    try {
      await apiClient.deleteTag(id)
      setSuccess('Tag deleted successfully')
      loadTags()
    } catch (error: unknown) {
      setError(error instanceof Error ? error.message : 'Failed to delete tag')
    }
  }

  const handleShare = async (tagId: number, targetProjectId: number) => {
    try {
      await apiClient.shareTag(tagId, targetProjectId)
      setShareMenuId(null)
      setSuccess('Tag shared successfully')
    } catch (error: unknown) {
      setError(error instanceof Error ? error.message : 'Failed to share tag')
    }
  }

  const handleUnshare = async (tagId: number) => {
    try {
      await apiClient.unshareTag(tagId, projectIdNum)
      setSuccess('Tag removed from this project')
      loadTags()
    } catch (error: unknown) {
      setError(error instanceof Error ? error.message : 'Failed to remove tag')
    }
  }

  return (
    <div className="min-h-screen bg-dark-bg-primary py-8">
      <div className="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="mb-8">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-3xl font-bold text-dark-text-primary">Tags</h1>
              <p className="text-dark-text-secondary mt-1">Label and categorize your tasks</p>
            </div>
            <Button onClick={() => navigate(projectId ? `/app/projects/${projectId}` : '/app')} variant="secondary">
              <svg className="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" />
              </svg>
              Back
            </Button>
          </div>
        </div>

        <Card className="shadow-md">
          <div className="p-6 sm:p-8">
            <div className="flex items-center justify-between mb-6">
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 bg-indigo-500/10 rounded-lg flex items-center justify-center">
                  <svg className="w-6 h-6 text-indigo-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z" />
                  </svg>
                </div>
                <h2 className="text-xl font-semibold text-dark-text-primary">Manage Tags</h2>
              </div>
              <Button
                onClick={() => {
                  setShowForm(true)
                  setEditingId(null)
                  setFormData({ name: '', color: '#3B82F6' })
                }}
                size="sm"
              >
                <svg className="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                </svg>
                New Tag
              </Button>
            </div>

            {success && (
              <div className="mb-4 p-3 bg-success-500/10 border-l-4 border-success-400 rounded-r text-success-400 text-sm">
                {success}
              </div>
            )}

            {error && <FormError message={error} className="mb-4" />}

            {showForm && (
              <form onSubmit={handleSave} className="mb-6 p-4 bg-indigo-500/5 border border-indigo-500/30 rounded-lg">
                <h3 className="font-semibold text-dark-text-primary mb-4">
                  {editingId ? 'Edit Tag' : 'New Tag'}
                </h3>

                <div className="space-y-3">
                  <TextInput
                    label="Tag Name"
                    value={formData.name}
                    onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                    placeholder="bug, feature, urgent..."
                    required
                  />

                  <div>
                    <label className="block text-sm font-medium text-dark-text-primary mb-1">Color</label>
                    <div className="flex gap-2">
                      <input
                        type="color"
                        value={formData.color}
                        onChange={(e) => setFormData({ ...formData, color: e.target.value })}
                        className="h-10 w-20 rounded border border-dark-border-subtle bg-dark-bg-primary cursor-pointer"
                      />
                      <input
                        type="text"
                        value={formData.color}
                        onChange={(e) => setFormData({ ...formData, color: e.target.value })}
                        className="flex-1 px-3 py-2 border border-dark-border-subtle bg-dark-bg-primary text-dark-text-primary placeholder-dark-text-tertiary rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 outline-none font-mono text-sm"
                        placeholder="#3B82F6"
                      />
                    </div>
                  </div>
                </div>

                <div className="flex gap-2 mt-4">
                  <Button type="submit" size="sm">
                    {editingId ? 'Update' : 'Create'}
                  </Button>
                  <Button
                    type="button"
                    variant="secondary"
                    size="sm"
                    onClick={() => {
                      setShowForm(false)
                      setEditingId(null)
                    }}
                  >
                    Cancel
                  </Button>
                </div>
              </form>
            )}

            <div className="space-y-2">
              {tags.length === 0 ? (
                <div className="text-center py-8 text-dark-text-tertiary">
                  <svg className="w-12 h-12 mx-auto mb-3 text-dark-text-tertiary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z" />
                  </svg>
                  <p>No tags yet</p>
                  <p className="text-sm mt-1">Create tags to organize your tasks</p>
                </div>
              ) : (
                <div className="flex flex-wrap gap-2">
                  {tags.map((tag) => (
                    <div
                      key={tag.id}
                      className="group relative inline-flex items-center gap-2 px-3 py-1.5 rounded-full border border-dark-border-subtle hover:border-dark-bg-tertiary/50 transition-colors"
                      style={{ backgroundColor: tag.color + '20' }}
                    >
                      <div
                        className="w-3 h-3 rounded-full"
                        style={{ backgroundColor: tag.color }}
                      />
                      <span className="text-sm font-medium text-dark-text-primary">{tag.name}</span>
                      {tag.is_shared && (
                        <span className="text-xs text-amber-400 font-medium">shared</span>
                      )}
                      <div className="flex gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
                        {tag.is_shared ? (
                          <button
                            onClick={() => handleUnshare(tag.id)}
                            className="p-1 text-amber-400 hover:bg-amber-500/10 rounded transition-colors"
                            title="Remove from this project"
                          >
                            <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                            </svg>
                          </button>
                        ) : (
                          <>
                            <div className="relative" ref={shareMenuId === tag.id ? shareMenuRef : undefined}>
                              <button
                                onClick={() => setShareMenuId(shareMenuId === tag.id ? null : tag.id)}
                                className="p-1 text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary rounded transition-colors"
                                title="Share to another project"
                              >
                                <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8.684 13.342C8.886 12.938 9 12.482 9 12c0-.482-.114-.938-.316-1.342m0 2.684a3 3 0 110-2.684m0 2.684l6.632 3.316m-6.632-6l6.632-3.316m0 0a3 3 0 105.367-2.684 3 3 0 00-5.367 2.684zm0 9.316a3 3 0 105.368 2.684 3 3 0 00-5.368-2.684z" />
                                </svg>
                              </button>
                              {shareMenuId === tag.id && (
                                <div className="absolute right-0 top-6 z-10 w-48 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg shadow-lg py-1">
                                  {otherProjects.length === 0 ? (
                                    <p className="px-3 py-2 text-xs text-dark-text-tertiary">No other projects</p>
                                  ) : (
                                    otherProjects.map(p => (
                                      <button
                                        key={p.id}
                                        onClick={() => handleShare(tag.id, p.id!)}
                                        className="w-full text-left px-3 py-2 text-sm text-dark-text-primary hover:bg-dark-bg-tertiary transition-colors truncate"
                                      >
                                        {p.name}
                                      </button>
                                    ))
                                  )}
                                </div>
                              )}
                            </div>
                            <button
                              onClick={() => handleEdit(tag)}
                              className="p-1 text-primary-400 hover:bg-primary-500/10 rounded transition-colors"
                              title="Edit"
                            >
                              <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                              </svg>
                            </button>
                            <button
                              onClick={() => handleDelete(tag.id)}
                              className="p-1 text-danger-400 hover:bg-danger-500/10 rounded transition-colors"
                              title="Delete"
                            >
                              <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                              </svg>
                            </button>
                          </>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </Card>
      </div>
    </div>
  )
}
