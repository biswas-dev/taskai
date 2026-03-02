import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import Card from '../components/ui/Card'
import Button from '../components/ui/Button'
import TextInput from '../components/ui/TextInput'
import FormError from '../components/ui/FormError'
import { apiClient } from '../lib/api'

interface Sprint {
  id: number
  name: string
  goal: string
  start_date?: string
  end_date?: string
  status: string
  created_at: string
}

interface Tag {
  id: number
  name: string
  color: string
  created_at: string
}

export default function SprintsAndTags() {
  const navigate = useNavigate()

  // Sprints state
  const [sprints, setSprints] = useState<Sprint[]>([])
  const [showSprintForm, setShowSprintForm] = useState(false)
  const [sprintFormData, setSprintFormData] = useState<{
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
  const [editingSprintId, setEditingSprintId] = useState<number | null>(null)
  const [sprintError, setSprintError] = useState('')
  const [sprintSuccess, setSprintSuccess] = useState('')

  // Tags state
  const [tags, setTags] = useState<Tag[]>([])
  const [showTagForm, setShowTagForm] = useState(false)
  const [tagFormData, setTagFormData] = useState({
    name: '',
    color: '#3B82F6',
  })
  const [editingTagId, setEditingTagId] = useState<number | null>(null)
  const [tagError, setTagError] = useState('')
  const [tagSuccess, setTagSuccess] = useState('')

  useEffect(() => {
    loadSprints()
    loadTags()
  }, [])

  const loadSprints = async () => {
    try {
      const data = await apiClient.getSprints()
      setSprints(data)
    } catch (error: unknown) {
      // non-critical load failure
    }
  }

  const loadTags = async () => {
    try {
      const data = await apiClient.getTags()
      setTags(data)
    } catch (error: unknown) {
      // non-critical load failure
    }
  }

  const handleSaveSprint = async (e: React.FormEvent) => {
    e.preventDefault()
    setSprintError('')
    setSprintSuccess('')

    if (!sprintFormData.name.trim()) {
      setSprintError('Sprint name is required')
      return
    }

    try {
      if (editingSprintId) {
        await apiClient.updateSprint(editingSprintId, sprintFormData)
        setSprintSuccess('Sprint updated successfully')
      } else {
        await apiClient.createSprint(sprintFormData)
        setSprintSuccess('Sprint created successfully')
      }

      setShowSprintForm(false)
      setEditingSprintId(null)
      setSprintFormData({ name: '', goal: '', start_date: '', end_date: '', status: 'planned' })
      loadSprints()
    } catch (error: unknown) {
      setSprintError(error instanceof Error ? error.message : 'Failed to save sprint')
    }
  }

  const handleEditSprint = (sprint: Sprint) => {
    setEditingSprintId(sprint.id)
    setSprintFormData({
      name: sprint.name,
      goal: sprint.goal || '',
      start_date: sprint.start_date || '',
      end_date: sprint.end_date || '',
      status: sprint.status as 'planned' | 'active' | 'completed',
    })
    setShowSprintForm(true)
  }

  const handleDeleteSprint = async (id: number) => {
    if (!confirm('Are you sure you want to delete this sprint?')) {
      return
    }

    try {
      await apiClient.deleteSprint(id)
      setSprintSuccess('Sprint deleted successfully')
      loadSprints()
    } catch (error: unknown) {
      setSprintError(error instanceof Error ? error.message : 'Failed to delete sprint')
    }
  }

  const handleSaveTag = async (e: React.FormEvent) => {
    e.preventDefault()
    setTagError('')
    setTagSuccess('')

    if (!tagFormData.name.trim()) {
      setTagError('Tag name is required')
      return
    }

    try {
      if (editingTagId) {
        await apiClient.updateTag(editingTagId, tagFormData)
        setTagSuccess('Tag updated successfully')
      } else {
        await apiClient.createTag(tagFormData)
        setTagSuccess('Tag created successfully')
      }

      setShowTagForm(false)
      setEditingTagId(null)
      setTagFormData({ name: '', color: '#3B82F6' })
      loadTags()
    } catch (error: unknown) {
      setTagError(error instanceof Error ? error.message : 'Failed to save tag')
    }
  }

  const handleEditTag = (tag: Tag) => {
    setEditingTagId(tag.id)
    setTagFormData({
      name: tag.name,
      color: tag.color,
    })
    setShowTagForm(true)
  }

  const handleDeleteTag = async (id: number) => {
    if (!confirm('Are you sure you want to delete this tag?')) {
      return
    }

    try {
      await apiClient.deleteTag(id)
      setTagSuccess('Tag deleted successfully')
      loadTags()
    } catch (error: unknown) {
      setTagError(error instanceof Error ? error.message : 'Failed to delete tag')
    }
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'active':
        return 'bg-green-100 text-green-800 border-green-200'
      case 'planned':
        return 'bg-blue-100 text-blue-800 border-blue-200'
      case 'completed':
        return 'bg-gray-100 text-gray-800 border-gray-200'
      default:
        return 'bg-gray-100 text-gray-800 border-gray-200'
    }
  }

  return (
    <div className="min-h-screen bg-gray-50 py-8">
      <div className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8">
        {/* Header */}
        <div className="mb-8">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-3xl font-bold text-gray-900">Sprints & Tags</h1>
              <p className="text-gray-600 mt-1">Manage your sprints and tags across all projects</p>
            </div>
            <Button onClick={() => navigate('/app')} variant="secondary">
              <svg className="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" />
              </svg>
              Back
            </Button>
          </div>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Sprints Section */}
          <Card className="shadow-md">
            <div className="p-6 sm:p-8">
              <div className="flex items-center justify-between mb-6">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 bg-purple-100 rounded-lg flex items-center justify-center">
                    <svg className="w-6 h-6 text-purple-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                    </svg>
                  </div>
                  <div>
                    <h2 className="text-xl font-semibold text-gray-900">Sprints</h2>
                    <p className="text-sm text-gray-600">Organize tasks into time-boxed iterations</p>
                  </div>
                </div>
                <Button
                  onClick={() => {
                    setShowSprintForm(true)
                    setEditingSprintId(null)
                    setSprintFormData({ name: '', goal: '', start_date: '', end_date: '', status: 'planned' })
                  }}
                  size="sm"
                >
                  <svg className="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                  </svg>
                  New Sprint
                </Button>
              </div>

              {sprintSuccess && (
                <div className="mb-4 p-3 bg-green-50 border-l-4 border-green-400 rounded-r text-green-800 text-sm">
                  {sprintSuccess}
                </div>
              )}

              {sprintError && <FormError message={sprintError} className="mb-4" />}

              {/* Sprint Form */}
              {showSprintForm && (
                <form onSubmit={handleSaveSprint} className="mb-6 p-4 bg-purple-50 border border-purple-200 rounded-lg">
                  <h3 className="font-semibold text-gray-900 mb-4">
                    {editingSprintId ? 'Edit Sprint' : 'New Sprint'}
                  </h3>

                  <div className="space-y-3">
                    <TextInput
                      label="Sprint Name"
                      value={sprintFormData.name}
                      onChange={(e) => setSprintFormData({ ...sprintFormData, name: e.target.value })}
                      placeholder="Sprint 1"
                      required
                    />

                    <div>
                      <label className="block text-sm font-medium text-gray-700 mb-1">Goal (Optional)</label>
                      <textarea
                        value={sprintFormData.goal}
                        onChange={(e) => setSprintFormData({ ...sprintFormData, goal: e.target.value })}
                        placeholder="What do you want to achieve?"
                        rows={2}
                        className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 outline-none"
                      />
                    </div>

                    <div className="grid grid-cols-2 gap-3">
                      <TextInput
                        label="Start Date"
                        type="date"
                        value={sprintFormData.start_date}
                        onChange={(e) => setSprintFormData({ ...sprintFormData, start_date: e.target.value })}
                      />

                      <TextInput
                        label="End Date"
                        type="date"
                        value={sprintFormData.end_date}
                        onChange={(e) => setSprintFormData({ ...sprintFormData, end_date: e.target.value })}
                      />
                    </div>

                    <div>
                      <label className="block text-sm font-medium text-gray-700 mb-1">Status</label>
                      <select
                        value={sprintFormData.status}
                        onChange={(e) => setSprintFormData({ ...sprintFormData, status: e.target.value as 'planned' | 'active' | 'completed' })}
                        className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 outline-none"
                      >
                        <option value="planned">Planned</option>
                        <option value="active">Active</option>
                        <option value="completed">Completed</option>
                      </select>
                    </div>
                  </div>

                  <div className="flex gap-2 mt-4">
                    <Button type="submit" size="sm">
                      {editingSprintId ? 'Update' : 'Create'}
                    </Button>
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      onClick={() => {
                        setShowSprintForm(false)
                        setEditingSprintId(null)
                      }}
                    >
                      Cancel
                    </Button>
                  </div>
                </form>
              )}

              {/* Sprints List */}
              <div className="space-y-3">
                {sprints.length === 0 ? (
                  <div className="text-center py-8 text-gray-500">
                    <svg className="w-12 h-12 mx-auto mb-3 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                    </svg>
                    <p>No sprints yet</p>
                    <p className="text-sm mt-1">Create your first sprint to get started</p>
                  </div>
                ) : (
                  sprints.map((sprint) => (
                    <div
                      key={sprint.id}
                      className="p-4 bg-white border border-gray-200 rounded-lg hover:border-gray-300 transition-colors"
                    >
                      <div className="flex items-start justify-between">
                        <div className="flex-1">
                          <div className="flex items-center gap-2 mb-1">
                            <h3 className="font-semibold text-gray-900">{sprint.name}</h3>
                            <span className={`px-2 py-0.5 text-xs font-medium rounded-full border ${getStatusColor(sprint.status)}`}>
                              {sprint.status}
                            </span>
                          </div>
                          {sprint.goal && <p className="text-sm text-gray-600 mb-2">{sprint.goal}</p>}
                          {(sprint.start_date || sprint.end_date) && (
                            <p className="text-xs text-gray-500">
                              {sprint.start_date && new Date(sprint.start_date).toLocaleDateString()}
                              {sprint.start_date && sprint.end_date && ' - '}
                              {sprint.end_date && new Date(sprint.end_date).toLocaleDateString()}
                            </p>
                          )}
                        </div>
                        <div className="flex gap-1">
                          <button
                            onClick={() => handleEditSprint(sprint)}
                            className="p-1.5 text-blue-600 hover:bg-blue-50 rounded transition-colors"
                            title="Edit"
                          >
                            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                            </svg>
                          </button>
                          <button
                            onClick={() => handleDeleteSprint(sprint.id)}
                            className="p-1.5 text-red-600 hover:bg-red-50 rounded transition-colors"
                            title="Delete"
                          >
                            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                            </svg>
                          </button>
                        </div>
                      </div>
                    </div>
                  ))
                )}
              </div>
            </div>
          </Card>

          {/* Tags Section */}
          <Card className="shadow-md">
            <div className="p-6 sm:p-8">
              <div className="flex items-center justify-between mb-6">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 bg-indigo-100 rounded-lg flex items-center justify-center">
                    <svg className="w-6 h-6 text-indigo-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z" />
                    </svg>
                  </div>
                  <div>
                    <h2 className="text-xl font-semibold text-gray-900">Tags</h2>
                    <p className="text-sm text-gray-600">Label and categorize your tasks</p>
                  </div>
                </div>
                <Button
                  onClick={() => {
                    setShowTagForm(true)
                    setEditingTagId(null)
                    setTagFormData({ name: '', color: '#3B82F6' })
                  }}
                  size="sm"
                >
                  <svg className="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                  </svg>
                  New Tag
                </Button>
              </div>

              {tagSuccess && (
                <div className="mb-4 p-3 bg-green-50 border-l-4 border-green-400 rounded-r text-green-800 text-sm">
                  {tagSuccess}
                </div>
              )}

              {tagError && <FormError message={tagError} className="mb-4" />}

              {/* Tag Form */}
              {showTagForm && (
                <form onSubmit={handleSaveTag} className="mb-6 p-4 bg-indigo-50 border border-indigo-200 rounded-lg">
                  <h3 className="font-semibold text-gray-900 mb-4">
                    {editingTagId ? 'Edit Tag' : 'New Tag'}
                  </h3>

                  <div className="space-y-3">
                    <TextInput
                      label="Tag Name"
                      value={tagFormData.name}
                      onChange={(e) => setTagFormData({ ...tagFormData, name: e.target.value })}
                      placeholder="bug, feature, urgent..."
                      required
                    />

                    <div>
                      <label className="block text-sm font-medium text-gray-700 mb-1">Color</label>
                      <div className="flex gap-2">
                        <input
                          type="color"
                          value={tagFormData.color}
                          onChange={(e) => setTagFormData({ ...tagFormData, color: e.target.value })}
                          className="h-10 w-20 rounded border border-gray-300 cursor-pointer"
                        />
                        <input
                          type="text"
                          value={tagFormData.color}
                          onChange={(e) => setTagFormData({ ...tagFormData, color: e.target.value })}
                          className="flex-1 px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 outline-none font-mono text-sm"
                          placeholder="#3B82F6"
                        />
                      </div>
                    </div>
                  </div>

                  <div className="flex gap-2 mt-4">
                    <Button type="submit" size="sm">
                      {editingTagId ? 'Update' : 'Create'}
                    </Button>
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      onClick={() => {
                        setShowTagForm(false)
                        setEditingTagId(null)
                      }}
                    >
                      Cancel
                    </Button>
                  </div>
                </form>
              )}

              {/* Tags List */}
              <div className="space-y-2">
                {tags.length === 0 ? (
                  <div className="text-center py-8 text-gray-500">
                    <svg className="w-12 h-12 mx-auto mb-3 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
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
                        className="group inline-flex items-center gap-2 px-3 py-1.5 rounded-full border border-gray-200 hover:border-gray-300 transition-colors"
                        style={{ backgroundColor: tag.color + '20' }}
                      >
                        <div
                          className="w-3 h-3 rounded-full"
                          style={{ backgroundColor: tag.color }}
                        />
                        <span className="text-sm font-medium text-gray-900">{tag.name}</span>
                        <div className="flex gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
                          <button
                            onClick={() => handleEditTag(tag)}
                            className="p-1 text-blue-600 hover:bg-blue-50 rounded transition-colors"
                            title="Edit"
                          >
                            <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                            </svg>
                          </button>
                          <button
                            onClick={() => handleDeleteTag(tag.id)}
                            className="p-1 text-red-600 hover:bg-red-50 rounded transition-colors"
                            title="Delete"
                          >
                            <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                            </svg>
                          </button>
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
    </div>
  )
}
