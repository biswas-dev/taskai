import { useState, useEffect, useRef } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import Card from '../components/ui/Card'
import Button from '../components/ui/Button'
import TextInput from '../components/ui/TextInput'
import FormError from '../components/ui/FormError'
import SearchSelect from '../components/ui/SearchSelect'
import { apiClient, type Project } from '../lib/api'

interface Sprint {
  id: number
  name: string
  goal: string
  start_date?: string
  end_date?: string
  status: string
  is_shared?: boolean
  created_at: string
}

export default function Sprints() {
  const navigate = useNavigate()
  const { projectId } = useParams<{ projectId: string }>()
  const projectIdNum = Number(projectId)
  const [sprints, setSprints] = useState<Sprint[]>([])
  const [showForm, setShowForm] = useState(false)
  const [formData, setFormData] = useState<{
    name: string
    goal: string
    start_date: string
    end_date: string
    status: 'planned' | 'active' | 'completed'
  }>({
    name: '',
    goal: '',
    start_date: '',
    end_date: '',
    status: 'planned',
  })
  const [editingId, setEditingId] = useState<number | null>(null)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [shareMenuId, setShareMenuId] = useState<number | null>(null)
  const [otherProjects, setOtherProjects] = useState<Project[]>([])
  const shareMenuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (projectIdNum) loadSprints()
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

  const loadSprints = async () => {
    try {
      const data = await apiClient.getSprints(projectIdNum)
      setSprints(data)
    } catch (error: unknown) {
      console.error('Failed to load sprints:', error)
    }
  }

  const handleShare = async (sprintId: number, targetProjectId: number) => {
    try {
      await apiClient.shareSprint(sprintId, targetProjectId)
      setShareMenuId(null)
      setSuccess('Sprint shared successfully')
    } catch (error: unknown) {
      setError(error instanceof Error ? error.message : 'Failed to share sprint')
    }
  }

  const handleUnshare = async (sprintId: number) => {
    try {
      await apiClient.unshareSprint(sprintId, projectIdNum)
      setSuccess('Sprint removed from this project')
      loadSprints()
    } catch (error: unknown) {
      setError(error instanceof Error ? error.message : 'Failed to remove sprint')
    }
  }

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setSuccess('')

    if (!formData.name.trim()) {
      setError('Sprint name is required')
      return
    }

    try {
      if (editingId) {
        await apiClient.updateSprint(editingId, formData)
        setSuccess('Sprint updated successfully')
      } else {
        await apiClient.createSprint(projectIdNum, formData)
        setSuccess('Sprint created successfully')
      }

      setShowForm(false)
      setEditingId(null)
      setFormData({ name: '', goal: '', start_date: '', end_date: '', status: 'planned' })
      loadSprints()
    } catch (error: unknown) {
      setError(error instanceof Error ? error.message : 'Failed to save sprint')
    }
  }

  const handleEdit = (sprint: Sprint) => {
    setEditingId(sprint.id)
    setFormData({
      name: sprint.name,
      goal: sprint.goal || '',
      start_date: sprint.start_date || '',
      end_date: sprint.end_date || '',
      status: sprint.status as 'planned' | 'active' | 'completed',
    })
    setShowForm(true)
  }

  const handleDelete = async (id: number) => {
    if (!confirm('Are you sure you want to delete this sprint?')) {
      return
    }

    try {
      await apiClient.deleteSprint(id)
      setSuccess('Sprint deleted successfully')
      loadSprints()
    } catch (error: unknown) {
      setError(error instanceof Error ? error.message : 'Failed to delete sprint')
    }
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'active':
        return 'bg-success-500/10 text-success-400 border-success-500/30'
      case 'planned':
        return 'bg-primary-500/10 text-primary-400 border-primary-500/30'
      case 'completed':
        return 'bg-dark-bg-tertiary text-dark-text-tertiary border-dark-border-subtle'
      default:
        return 'bg-dark-bg-tertiary text-dark-text-tertiary border-dark-border-subtle'
    }
  }

  return (
    <div className="min-h-screen bg-dark-bg-primary py-8">
      <div className="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="mb-8">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-3xl font-bold text-dark-text-primary">Sprints</h1>
              <p className="text-dark-text-secondary mt-1">Organize tasks into time-boxed iterations</p>
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
                <div className="w-10 h-10 bg-purple-500/10 rounded-lg flex items-center justify-center">
                  <svg className="w-6 h-6 text-purple-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                  </svg>
                </div>
                <h2 className="text-xl font-semibold text-dark-text-primary">Manage Sprints</h2>
              </div>
              <Button
                onClick={() => {
                  setShowForm(true)
                  setEditingId(null)
                  setFormData({ name: '', goal: '', start_date: '', end_date: '', status: 'planned' })
                }}
                size="sm"
              >
                <svg className="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                </svg>
                New Sprint
              </Button>
            </div>

            {success && (
              <div className="mb-4 p-3 bg-success-500/10 border-l-4 border-success-400 rounded-r text-success-400 text-sm">
                {success}
              </div>
            )}

            {error && <FormError message={error} className="mb-4" />}

            {showForm && (
              <form onSubmit={handleSave} className="mb-6 p-4 bg-purple-500/5 border border-purple-500/30 rounded-lg">
                <h3 className="font-semibold text-dark-text-primary mb-4">
                  {editingId ? 'Edit Sprint' : 'New Sprint'}
                </h3>

                <div className="space-y-3">
                  <TextInput
                    label="Sprint Name"
                    value={formData.name}
                    onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                    placeholder="Sprint 1"
                    required
                  />

                  <div>
                    <label className="block text-sm font-medium text-dark-text-primary mb-1">Goal (Optional)</label>
                    <textarea
                      value={formData.goal}
                      onChange={(e) => setFormData({ ...formData, goal: e.target.value })}
                      placeholder="What do you want to achieve?"
                      rows={2}
                      className="w-full px-3 py-2 border border-dark-border-subtle bg-dark-bg-primary text-dark-text-primary placeholder-dark-text-tertiary rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 outline-none"
                    />
                  </div>

                  <div className="grid grid-cols-2 gap-3">
                    <TextInput
                      label="Start Date"
                      type="date"
                      value={formData.start_date}
                      onChange={(e) => setFormData({ ...formData, start_date: e.target.value })}
                    />

                    <TextInput
                      label="End Date"
                      type="date"
                      value={formData.end_date}
                      onChange={(e) => setFormData({ ...formData, end_date: e.target.value })}
                    />
                  </div>

                  <div>
                    <label className="block text-sm font-medium text-dark-text-primary mb-1">Status</label>
                    <SearchSelect
                      value={formData.status}
                      onChange={(v) => setFormData({ ...formData, status: v as 'planned' | 'active' | 'completed' })}
                      options={[
                        { value: 'planned', label: 'Planned' },
                        { value: 'active', label: 'Active' },
                        { value: 'completed', label: 'Completed' },
                      ]}
                    />
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

            <div className="space-y-3">
              {sprints.length === 0 ? (
                <div className="text-center py-8 text-dark-text-tertiary">
                  <svg className="w-12 h-12 mx-auto mb-3 text-dark-text-tertiary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                  </svg>
                  <p>No sprints yet</p>
                  <p className="text-sm mt-1">Create your first sprint to get started</p>
                </div>
              ) : (
                sprints.map((sprint) => (
                  <div
                    key={sprint.id}
                    className="p-4 bg-dark-bg-primary border border-dark-border-subtle rounded-lg hover:border-dark-border-medium transition-colors"
                  >
                    <div className="flex items-start justify-between">
                      <div className="flex-1">
                        <div className="flex items-center gap-2 mb-1">
                          <h3 className="font-semibold text-dark-text-primary">{sprint.name}</h3>
                          <span className={`px-2 py-0.5 text-xs font-medium rounded-full border ${getStatusColor(sprint.status)}`}>
                            {sprint.status}
                          </span>
                          {sprint.is_shared && (
                            <span className="px-2 py-0.5 text-xs font-medium rounded-full border bg-amber-500/10 text-amber-400 border-amber-500/30">
                              shared
                            </span>
                          )}
                        </div>
                        {sprint.goal && <p className="text-sm text-dark-text-secondary mb-2">{sprint.goal}</p>}
                        {(sprint.start_date || sprint.end_date) && (
                          <p className="text-xs text-dark-text-tertiary">
                            {sprint.start_date && new Date(sprint.start_date).toLocaleDateString()}
                            {sprint.start_date && sprint.end_date && ' - '}
                            {sprint.end_date && new Date(sprint.end_date).toLocaleDateString()}
                          </p>
                        )}
                      </div>
                      <div className="flex gap-1 relative" ref={shareMenuId === sprint.id ? shareMenuRef : undefined}>
                        {sprint.is_shared ? (
                          <button
                            onClick={() => handleUnshare(sprint.id)}
                            className="p-1.5 text-amber-400 hover:bg-amber-500/10 rounded transition-colors"
                            title="Remove from this project"
                          >
                            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
                            </svg>
                          </button>
                        ) : (
                          <>
                            <button
                              onClick={() => setShareMenuId(shareMenuId === sprint.id ? null : sprint.id)}
                              className="p-1.5 text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary rounded transition-colors"
                              title="Share to another project"
                            >
                              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8.684 13.342C8.886 12.938 9 12.482 9 12c0-.482-.114-.938-.316-1.342m0 2.684a3 3 0 110-2.684m0 2.684l6.632 3.316m-6.632-6l6.632-3.316m0 0a3 3 0 105.367-2.684 3 3 0 00-5.367 2.684zm0 9.316a3 3 0 105.368 2.684 3 3 0 00-5.368-2.684z" />
                              </svg>
                            </button>
                            {shareMenuId === sprint.id && (
                              <div className="absolute right-0 top-8 z-10 w-48 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg shadow-lg py-1">
                                {otherProjects.length === 0 ? (
                                  <p className="px-3 py-2 text-xs text-dark-text-tertiary">No other projects</p>
                                ) : (
                                  otherProjects.map(p => (
                                    <button
                                      key={p.id}
                                      onClick={() => handleShare(sprint.id, p.id!)}
                                      className="w-full text-left px-3 py-2 text-sm text-dark-text-primary hover:bg-dark-bg-tertiary transition-colors truncate"
                                    >
                                      {p.name}
                                    </button>
                                  ))
                                )}
                              </div>
                            )}
                            <button
                              onClick={() => handleEdit(sprint)}
                              className="p-1.5 text-primary-400 hover:bg-primary-500/10 rounded transition-colors"
                              title="Edit"
                            >
                              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                              </svg>
                            </button>
                            <button
                              onClick={() => handleDelete(sprint.id)}
                              className="p-1.5 text-danger-400 hover:bg-danger-500/10 rounded transition-colors"
                              title="Delete"
                            >
                              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                              </svg>
                            </button>
                          </>
                        )}
                      </div>
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>
        </Card>
      </div>
    </div>
  )
}
