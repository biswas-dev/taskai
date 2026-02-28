import { useState, useEffect, useRef, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import Button from '../components/ui/Button'
import ImagePickerModal from '../components/ImagePickerModal'
import { apiClient, Task, type SwimLane, type Sprint, type ProjectMember, type Attachment, type TaskComment } from '../lib/api'

interface TaskDetailProps {
  isModal?: boolean
  onClose?: () => void
}

export default function TaskDetail({ isModal, onClose }: TaskDetailProps) {
  const { projectId, taskNumber } = useParams()
  const navigate = useNavigate()
  const [task, setTask] = useState<Task | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  // Inline editing
  const [editingField, setEditingField] = useState<string | null>(null)
  const [editValue, setEditValue] = useState('')
  const titleRef = useRef<HTMLInputElement>(null)
  const descRef = useRef<HTMLTextAreaElement>(null)
  const commentRef = useRef<HTMLTextAreaElement>(null)

  // Reference data
  const [sprints, setSprints] = useState<Sprint[]>([])
  const [swimLanes, setSwimLanes] = useState<SwimLane[]>([])
  const [members, setMembers] = useState<ProjectMember[]>([])

  // Attachments
  const [attachments, setAttachments] = useState<Attachment[]>([])
  const [uploading, setUploading] = useState(false)
  const [editingAltId, setEditingAltId] = useState<number | null>(null)
  const [editingAltValue, setEditingAltValue] = useState('')
  const [pendingFile, setPendingFile] = useState<File | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  // Comments
  const [comments, setComments] = useState<TaskComment[]>([])
  const [newComment, setNewComment] = useState('')
  const [postingComment, setPostingComment] = useState(false)

  // Image picker
  const [imagePickerTarget, setImagePickerTarget] = useState<'description' | 'comment' | null>(null)

  // Confirm modal
  const [confirmAction, setConfirmAction] = useState<{ message: string; onConfirm: () => void } | null>(null)

  useEffect(() => {
    loadTask()
    loadSprints()
    loadSwimLanes()
    loadMembers()
  }, [projectId, taskNumber])

  useEffect(() => {
    if (task?.id) {
      loadComments(task.id)
      loadAttachments(task.id)
    }
  }, [task?.id])

  useEffect(() => {
    if (editingField === 'title') titleRef.current?.focus()
    if (editingField === 'description') {
      const el = descRef.current
      if (el) {
        el.focus()
        el.setSelectionRange(el.value.length, el.value.length)
      }
    }
  }, [editingField])

  const loadTask = async () => {
    try {
      setLoading(true)
      const found = await apiClient.getTaskByNumber(Number(projectId), Number(taskNumber))
      setTask(found)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to load task')
    } finally {
      setLoading(false)
    }
  }

  const loadSprints = async () => {
    try { setSprints(await apiClient.getSprints()) } catch { /* ignore */ }
  }

  const loadSwimLanes = async () => {
    try {
      const lanes = await apiClient.getSwimLanes(Number(projectId))
      setSwimLanes(lanes.sort((a, b) => a.position - b.position))
    } catch { /* ignore */ }
  }

  const loadComments = async (id?: number) => {
    const taskId = id ?? task?.id
    if (!taskId) return
    try { setComments(await apiClient.getTaskComments(taskId)) } catch { /* ignore */ }
  }

  const loadMembers = async () => {
    try { setMembers(await apiClient.getProjectMembers(Number(projectId))) } catch { /* ignore */ }
  }

  const loadAttachments = async (id?: number) => {
    const taskId = id ?? task?.id
    if (!taskId) return
    try { setAttachments(await apiClient.getTaskAttachments(taskId)) } catch { /* ignore */ }
  }

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return

    const allowedTypes = ['image/', 'video/', 'application/pdf']
    if (!allowedTypes.some(t => file.type.startsWith(t))) {
      setError('Only images, videos, and PDFs are allowed')
      e.target.value = ''
      return
    }

    setPendingFile(file)
    e.target.value = ''
  }

  const handleConfirmUpload = async (file: File, altText: string) => {
    try {
      setUploading(true)
      const sig = await apiClient.getUploadSignature({ taskId: task!.id! })

      const formData = new FormData()
      formData.append('file', file)
      formData.append('api_key', sig.api_key)
      formData.append('timestamp', String(sig.timestamp))
      formData.append('signature', sig.signature)
      formData.append('folder', sig.folder)
      formData.append('public_id', sig.public_id)

      const uploadRes = await fetch(
        `https://api.cloudinary.com/v1_1/${sig.cloud_name}/auto/upload`,
        { method: 'POST', body: formData }
      )

      if (!uploadRes.ok) throw new Error('Upload to Cloudinary failed')
      const uploadData = await uploadRes.json()

      let fileType = 'image'
      if (file.type.startsWith('video/')) fileType = 'video'
      else if (file.type === 'application/pdf') fileType = 'pdf'

      await apiClient.createTaskAttachment(task!.id!, {
        filename: file.name,
        alt_name: altText,
        file_type: fileType,
        content_type: file.type,
        file_size: file.size,
        cloudinary_url: uploadData.secure_url,
        cloudinary_public_id: uploadData.public_id,
      })

      await loadAttachments()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to upload file')
    } finally {
      setUploading(false)
      setPendingFile(null)
    }
  }

  const handleDeleteAttachment = (attachmentId: number) => {
    setConfirmAction({
      message: 'Are you sure you want to delete this attachment? This cannot be undone.',
      onConfirm: async () => {
        try {
          await apiClient.deleteTaskAttachment(task!.id!, attachmentId)
          await loadAttachments()
        } catch (err: unknown) {
          setError(err instanceof Error ? err.message : 'Failed to delete attachment')
        }
        setConfirmAction(null)
      },
    })
  }

  const handleSaveAltName = async (attachmentId: number) => {
    try {
      await apiClient.updateAttachment(attachmentId, { alt_name: editingAltValue })
      setEditingAltId(null)
      await loadAttachments()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to update alt name')
    }
  }

  const insertImageMarkdown = useCallback((alt: string, url: string) => {
    const markdown = `![${alt}](${url})`

    if (imagePickerTarget === 'description' && descRef.current) {
      const textarea = descRef.current
      const start = textarea.selectionStart
      const end = textarea.selectionEnd
      const text = editValue
      const newText = text.substring(0, start) + markdown + text.substring(end)
      setEditValue(newText)
      setTimeout(() => {
        textarea.focus()
        const cursorPos = start + markdown.length
        textarea.setSelectionRange(cursorPos, cursorPos)
      }, 0)
    } else if (imagePickerTarget === 'comment' && commentRef.current) {
      const textarea = commentRef.current
      const start = textarea.selectionStart
      const end = textarea.selectionEnd
      const newText = newComment.substring(0, start) + markdown + newComment.substring(end)
      setNewComment(newText)
      setTimeout(() => {
        textarea.focus()
        const cursorPos = start + markdown.length
        textarea.setSelectionRange(cursorPos, cursorPos)
      }, 0)
    }

    setImagePickerTarget(null)
  }, [imagePickerTarget, editValue, newComment])

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const saveField = async (field: string, value: any) => {
    if (!task) return
    try {
      setSaving(true)
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const update: Record<string, any> = {}

      switch (field) {
        case 'title':
          if (!value?.trim()) return
          update.title = value.trim()
          break
        case 'description':
          update.description = value?.trim() || ''
          break
        case 'swim_lane_id': {
          // Backend auto-syncs status from swim lane's status_category
          update.swim_lane_id = Number(value)
          break
        }
        case 'priority':
          update.priority = value
          break
        case 'sprint_id':
          update.sprint_id = value ? parseInt(value) : null
          break
        case 'assignee_id':
          update.assignee_id = value ? parseInt(value) : null
          break
        case 'due_date':
          update.due_date = value || null
          break
        case 'estimated_hours':
          update.estimated_hours = value ? parseFloat(value) : 0
          break
        case 'actual_hours':
          update.actual_hours = value ? parseFloat(value) : 0
          break
      }

      await apiClient.updateTask(task.id!, update)
      await loadTask()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to update')
    } finally {
      setSaving(false)
      setEditingField(null)
    }
  }

  const startEdit = (field: string, currentValue: string) => {
    setEditingField(field)
    setEditValue(currentValue)
  }

  const cancelEdit = () => {
    setEditingField(null)
    setEditValue('')
  }

  const handleKeyDown = (e: React.KeyboardEvent, field: string) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      saveField(field, editValue)
    } else if (e.key === 'Escape') {
      cancelEdit()
    }
  }

  const handlePostComment = async () => {
    if (!newComment.trim()) return
    try {
      setPostingComment(true)
      await apiClient.createTaskComment(task!.id!, newComment.trim())
      setNewComment('')
      await loadComments()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to post comment')
    } finally {
      setPostingComment(false)
    }
  }

  const handleDelete = () => {
    if (!task) return
    setConfirmAction({
      message: 'Are you sure you want to delete this task? This action cannot be undone.',
      onConfirm: async () => {
        try {
          await apiClient.deleteTask(task.id!)
          setConfirmAction(null)
          handleClose()
        } catch (err: unknown) {
          setError(err instanceof Error ? err.message : 'Failed to delete task')
          setConfirmAction(null)
        }
      },
    })
  }

  const handleClose = () => {
    if (onClose) onClose()
    else navigate(`/app/projects/${projectId}`)
  }

  const getPriorityColor = (priority: string) => {
    switch (priority) {
      case 'urgent': return 'bg-danger-500/10 text-danger-400 border-danger-500/30'
      case 'high': return 'bg-warning-500/10 text-warning-400 border-warning-500/30'
      case 'medium': return 'bg-yellow-500/10 text-yellow-400 border-yellow-500/30'
      case 'low': return 'bg-success-500/10 text-success-400 border-success-500/30'
      default: return 'bg-dark-bg-tertiary text-dark-text-tertiary border-dark-border-subtle'
    }
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'done': return 'bg-success-500/10 text-success-400 border-success-500/30'
      case 'in_progress': return 'bg-primary-500/10 text-primary-400 border-primary-500/30'
      case 'todo': return 'bg-dark-bg-tertiary text-dark-text-tertiary border-dark-border-subtle'
      default: return 'bg-dark-bg-tertiary text-dark-text-tertiary border-dark-border-subtle'
    }
  }

  const getStatusLabel = (status: string) => {
    switch (status) {
      case 'done': return 'Done'
      case 'in_progress': return 'In Progress'
      case 'todo': return 'To Do'
      default: return status
    }
  }

  if (loading) {
    return (
      <div className={`${isModal ? 'p-8' : 'min-h-screen bg-dark-bg-primary'} flex items-center justify-center`}>
        <div className="text-dark-text-secondary">Loading task...</div>
      </div>
    )
  }

  if (error && !task) {
    return (
      <div className={`${isModal ? 'p-8' : 'min-h-screen bg-dark-bg-primary py-8'}`}>
        <div className={`${isModal ? '' : 'max-w-7xl mx-auto px-4 sm:px-6 lg:px-8'}`}>
          <div className="bg-dark-bg-secondary border border-dark-border-subtle rounded-lg p-8 text-center">
            <p className="text-danger-400 mb-4">{error}</p>
            <Button onClick={handleClose}>
              {isModal ? 'Close' : 'Back to Project'}
            </Button>
          </div>
        </div>
      </div>
    )
  }

  if (!task) return null

  const containerClass = isModal ? '' : 'min-h-screen bg-dark-bg-primary'
  const innerClass = isModal ? 'px-6' : 'max-w-7xl mx-auto px-4 sm:px-6 lg:px-8'

  return (
    <div className={containerClass}>
      {/* Header */}
      <div className="border-b border-dark-border-subtle bg-dark-bg-secondary sticky top-0 z-10">
        <div className={`${innerClass} py-4`}>
          <div className="flex items-center justify-between mb-3">
            <button
              onClick={handleClose}
              className="inline-flex items-center text-sm text-dark-text-secondary hover:text-dark-text-primary transition-colors"
            >
              {isModal ? (
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              ) : (
                <>
                  <svg className="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" />
                  </svg>
                  Back to project
                </>
              )}
            </button>
            <button
              onClick={handleDelete}
              className="p-2 text-dark-text-tertiary hover:text-danger-400 hover:bg-danger-500/10 rounded-lg transition-colors"
              title="Delete task"
            >
              <svg className="w-4.5 h-4.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
              </svg>
            </button>
          </div>

          {/* Task number */}
          {task.task_number && (
            <span className="text-sm font-mono text-dark-text-tertiary mb-1">#{task.task_number}</span>
          )}

          {/* Title - inline editable */}
          {editingField === 'title' ? (
            <input
              ref={titleRef}
              type="text"
              value={editValue}
              onChange={(e) => setEditValue(e.target.value)}
              onBlur={() => saveField('title', editValue)}
              onKeyDown={(e) => handleKeyDown(e, 'title')}
              className="w-full text-2xl font-bold px-2 py-1 -ml-2 border border-dark-border-subtle bg-dark-bg-primary text-dark-text-primary rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 outline-none"
            />
          ) : (
            <h1
              onClick={() => startEdit('title', task.title || '')}
              className="text-2xl font-bold text-dark-text-primary cursor-text hover:bg-dark-bg-tertiary/50 px-2 py-1 -ml-2 rounded-lg transition-colors"
              title="Click to edit title"
            >
              {task.title}
            </h1>
          )}

          {/* Status badges */}
          <div className="flex flex-wrap gap-2 mt-2">
            {task.status && (
              <span className={`px-2.5 py-1 text-xs font-semibold rounded-full border ${getStatusColor(task.status)}`}>
                {getStatusLabel(task.status)}
              </span>
            )}
            {task.priority && (
              <span className={`px-2.5 py-1 text-xs font-semibold rounded-full border ${getPriorityColor(task.priority)}`}>
                {task.priority}
              </span>
            )}
            {task.tags && task.tags.length > 0 && task.tags.map((tag) => (
              <span
                key={tag.id}
                className="px-2.5 py-1 text-xs font-semibold rounded-full border border-dark-border-subtle"
                style={{ backgroundColor: tag.color + '20', color: tag.color }}
              >
                {tag.name}
              </span>
            ))}
            {saving && (
              <span className="px-2.5 py-1 text-xs text-dark-text-tertiary animate-pulse">
                Saving...
              </span>
            )}
          </div>
        </div>
      </div>

      {/* Main Content */}
      <div className={`${innerClass} py-6`}>
        <div className="flex flex-col md:flex-row gap-6">
          {/* Left Column - Description & Comments */}
          <div className="flex-1 min-w-0 space-y-6">
            {/* Description */}
            <div className="bg-dark-bg-secondary border border-dark-border-subtle rounded-lg p-6">
              <div className="flex items-center justify-between mb-3">
                <h2 className="text-sm font-semibold text-dark-text-primary">Description</h2>
                {editingField !== 'description' && (
                  <button
                    onClick={() => startEdit('description', task.description || '')}
                    className="p-1.5 text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary rounded-md transition-colors"
                    title="Edit description"
                  >
                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
                    </svg>
                  </button>
                )}
              </div>

              {editingField === 'description' ? (
                <div>
                  {/* Toolbar */}
                  <div className="flex items-center gap-1 mb-1">
                    <button
                      onClick={() => setImagePickerTarget('description')}
                      className="inline-flex items-center gap-1 px-2 py-1 text-xs text-dark-text-tertiary hover:text-primary-400 hover:bg-primary-500/10 rounded transition-colors"
                      title="Insert image"
                    >
                      <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
                      </svg>
                      Insert image
                    </button>
                  </div>
                  <textarea
                    ref={descRef}
                    value={editValue}
                    onChange={(e) => setEditValue(e.target.value)}
                    rows={12}
                    className="w-full px-3 py-2 border border-dark-border-subtle bg-dark-bg-primary text-dark-text-primary rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 outline-none font-mono text-sm placeholder-dark-text-tertiary"
                    placeholder="Add a description in markdown format..."
                    onKeyDown={(e) => {
                      if (e.key === 'Escape') cancelEdit()
                      if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                        e.preventDefault()
                        saveField('description', editValue)
                      }
                    }}
                  />
                  <div className="flex items-center gap-2 mt-2">
                    <Button size="sm" onClick={() => saveField('description', editValue)} disabled={saving}>
                      {saving ? 'Saving...' : 'Save'}
                    </Button>
                    <Button size="sm" variant="secondary" onClick={cancelEdit}>
                      Cancel
                    </Button>
                    <span className="text-xs text-dark-text-tertiary ml-auto">
                      Markdown supported &middot; {navigator.platform.includes('Mac') ? 'Cmd' : 'Ctrl'}+Enter to save
                    </span>
                  </div>
                </div>
              ) : task.description ? (
                <div className="prose prose-sm max-w-none prose-headings:text-dark-text-primary prose-p:text-dark-text-secondary prose-a:text-primary-400 prose-code:text-primary-400 prose-code:bg-primary-500/10 prose-code:px-1 prose-code:py-0.5 prose-code:rounded prose-pre:bg-dark-bg-primary prose-pre:border prose-pre:border-dark-border-subtle prose-strong:text-dark-text-primary prose-li:text-dark-text-secondary prose-img:rounded-lg prose-img:border prose-img:border-dark-border-subtle">
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>
                    {task.description}
                  </ReactMarkdown>
                </div>
              ) : (
                <p
                  className="text-sm text-dark-text-tertiary italic cursor-pointer hover:text-dark-text-secondary transition-colors"
                  onClick={() => startEdit('description', '')}
                >
                  Click to add a description...
                </p>
              )}
            </div>

            {/* Attachments Section */}
            <div className="bg-dark-bg-secondary border border-dark-border-subtle rounded-lg p-6">
              <div className="flex items-center justify-between mb-4">
                <h2 className="text-sm font-semibold text-dark-text-primary">
                  Attachments {attachments.length > 0 && `(${attachments.length})`}
                </h2>
                <label className={`inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-md cursor-pointer transition-colors ${
                  uploading
                    ? 'bg-dark-bg-tertiary text-dark-text-tertiary cursor-not-allowed'
                    : 'bg-primary-500/10 text-primary-400 hover:bg-primary-500/20'
                }`}>
                  <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
                  </svg>
                  {uploading ? 'Uploading...' : 'Upload'}
                  <input
                    ref={fileInputRef}
                    type="file"
                    className="hidden"
                    accept="image/*,video/*,.pdf"
                    onChange={handleFileUpload}
                    disabled={uploading}
                  />
                </label>
              </div>

              {attachments.length === 0 ? (
                <p className="text-sm text-dark-text-tertiary italic">No attachments</p>
              ) : (
                <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
                  {attachments.map((att: Attachment) => (
                    <div key={att.id} className="group relative border border-dark-border-subtle rounded-lg overflow-hidden bg-dark-bg-primary">
                      {att.file_type === 'image' ? (
                        <a href={att.cloudinary_url} target="_blank" rel="noopener noreferrer">
                          <img
                            src={att.cloudinary_url}
                            alt={att.alt_name || att.filename}
                            className="w-full h-24 object-cover"
                          />
                        </a>
                      ) : att.file_type === 'video' ? (
                        <a href={att.cloudinary_url} target="_blank" rel="noopener noreferrer" className="flex items-center justify-center h-24 bg-dark-bg-tertiary">
                          <svg className="w-8 h-8 text-dark-text-tertiary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                          </svg>
                        </a>
                      ) : (
                        <a href={att.cloudinary_url} target="_blank" rel="noopener noreferrer" className="flex items-center justify-center h-24 bg-dark-bg-tertiary">
                          <svg className="w-8 h-8 text-dark-text-tertiary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z" />
                          </svg>
                        </a>
                      )}
                      <div className="p-2">
                        {editingAltId === att.id ? (
                          <input
                            type="text"
                            value={editingAltValue}
                            onChange={(e) => setEditingAltValue(e.target.value)}
                            onBlur={() => handleSaveAltName(att.id)}
                            onKeyDown={(e) => {
                              if (e.key === 'Enter') handleSaveAltName(att.id)
                              if (e.key === 'Escape') setEditingAltId(null)
                            }}
                            className="w-full text-xs bg-dark-bg-tertiary border border-dark-border-subtle text-dark-text-primary rounded px-1.5 py-0.5 outline-none focus:ring-1 focus:ring-primary-500"
                            autoFocus
                            placeholder="Alt name..."
                          />
                        ) : (
                          <p
                            className="text-xs text-dark-text-primary truncate cursor-pointer hover:text-primary-400 transition-colors"
                            title={`Click to edit alt name: ${att.alt_name || att.filename}`}
                            onClick={() => {
                              setEditingAltId(att.id)
                              setEditingAltValue(att.alt_name || '')
                            }}
                          >
                            {att.alt_name || att.filename}
                          </p>
                        )}
                        <p className="text-[10px] text-dark-text-tertiary">
                          {(att.file_size / 1024 / 1024).toFixed(1)} MB
                        </p>
                      </div>
                      <button
                        onClick={() => handleDeleteAttachment(att.id)}
                        className="absolute top-1 right-1 p-1 bg-dark-bg-primary/80 rounded-md opacity-0 group-hover:opacity-100 transition-opacity text-danger-400 hover:bg-danger-500/10"
                        title="Delete attachment"
                      >
                        <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                        </svg>
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* Comments Section */}
            <div className="bg-dark-bg-secondary border border-dark-border-subtle rounded-lg p-6">
              <h2 className="text-sm font-semibold text-dark-text-primary mb-4">
                Comments {comments.length > 0 && `(${comments.length})`}
              </h2>

              <div className="mb-4">
                {/* Comment toolbar */}
                <div className="flex items-center gap-1 mb-1">
                  <button
                    onClick={() => setImagePickerTarget('comment')}
                    className="inline-flex items-center gap-1 px-2 py-1 text-xs text-dark-text-tertiary hover:text-primary-400 hover:bg-primary-500/10 rounded transition-colors"
                    title="Insert image"
                  >
                    <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
                    </svg>
                    Insert image
                  </button>
                </div>
                <textarea
                  ref={commentRef}
                  value={newComment}
                  onChange={(e) => setNewComment(e.target.value)}
                  rows={3}
                  className="w-full px-3 py-2 border border-dark-border-subtle bg-dark-bg-primary text-dark-text-primary rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 outline-none text-sm placeholder-dark-text-tertiary"
                  placeholder="Add a comment..."
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey) && newComment.trim()) {
                      e.preventDefault()
                      handlePostComment()
                    }
                  }}
                />
                <div className="flex justify-between items-center mt-2">
                  <span className="text-xs text-dark-text-tertiary">
                    {navigator.platform.includes('Mac') ? 'Cmd' : 'Ctrl'}+Enter to post
                  </span>
                  <Button
                    onClick={handlePostComment}
                    size="sm"
                    disabled={!newComment.trim() || postingComment}
                  >
                    {postingComment ? 'Posting...' : 'Post Comment'}
                  </Button>
                </div>
              </div>

              <div className="space-y-4">
                {comments.length === 0 ? (
                  <p className="text-sm text-dark-text-tertiary italic">No comments yet</p>
                ) : (
                  comments.map((comment) => (
                    <div key={comment.id} className="border-t border-dark-border-subtle pt-4 first:border-t-0 first:pt-0">
                      <div className="flex items-start gap-3">
                        <div className="w-8 h-8 rounded-full bg-primary-500/10 flex items-center justify-center flex-shrink-0">
                          <span className="text-xs font-medium text-primary-400">
                            {(comment.user_name || 'U').charAt(0).toUpperCase()}
                          </span>
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 mb-1">
                            <span className="text-sm font-medium text-dark-text-primary">
                              {comment.user_name || `User ${comment.user_id}`}
                            </span>
                            <span className="text-xs text-dark-text-tertiary">
                              {new Date(comment.created_at).toLocaleString()}
                            </span>
                          </div>
                          <div className="text-sm text-dark-text-secondary prose prose-sm max-w-none prose-img:rounded-lg prose-img:max-h-64 prose-img:border prose-img:border-dark-border-subtle prose-a:text-primary-400">
                            <ReactMarkdown remarkPlugins={[remarkGfm]}>
                              {comment.comment}
                            </ReactMarkdown>
                          </div>
                        </div>
                      </div>
                    </div>
                  ))
                )}
              </div>
            </div>
          </div>

          {/* Right Column - Sidebar */}
          <div className="w-full md:w-72 flex-shrink-0">
            <div className="bg-dark-bg-secondary border border-dark-border-subtle rounded-lg divide-y divide-dark-border-subtle">
              {/* Swim Lane */}
              <SidebarField label="Swim Lane">
                <InlineSelect
                  value={String(task.swim_lane_id ?? '')}
                  onChange={(v) => saveField('swim_lane_id', v)}
                  options={swimLanes.map(l => ({ value: String(l.id), label: l.name }))}
                />
              </SidebarField>

              {/* Priority */}
              <SidebarField label="Priority">
                <InlineSelect
                  value={task.priority || 'medium'}
                  onChange={(v) => saveField('priority', v)}
                  options={[
                    { value: 'low', label: 'Low' },
                    { value: 'medium', label: 'Medium' },
                    { value: 'high', label: 'High' },
                    { value: 'urgent', label: 'Urgent' },
                  ]}
                />
              </SidebarField>

              {/* Sprint */}
              <SidebarField label="Sprint">
                <InlineSelect
                  value={task.sprint_id?.toString() || ''}
                  onChange={(v) => saveField('sprint_id', v)}
                  options={[
                    { value: '', label: 'No sprint' },
                    ...sprints.map(s => ({ value: String(s.id), label: s.name })),
                  ]}
                />
              </SidebarField>

              {/* Assignee */}
              <SidebarField label="Assignee">
                {members.length > 0 ? (
                  <InlineSelect
                    value={task.assignee_id?.toString() || ''}
                    onChange={(v) => saveField('assignee_id', v)}
                    options={[
                      { value: '', label: 'Unassigned' },
                      ...members.map(m => ({ value: String(m.user_id || m.id), label: m.user_name || m.email || `User ${m.user_id || m.id}` })),
                    ]}
                  />
                ) : (
                  <div className="flex items-center gap-2 px-3 py-1.5">
                    <div className="w-5 h-5 rounded-full bg-primary-500/10 flex items-center justify-center flex-shrink-0">
                      <svg className="w-3 h-3 text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
                      </svg>
                    </div>
                    <span className="text-sm text-dark-text-primary">
                      {task.assignee_name || (task.assignee_id ? `User ${task.assignee_id}` : 'Unassigned')}
                    </span>
                  </div>
                )}
              </SidebarField>

              {/* Due Date */}
              <SidebarField label="Due Date">
                {editingField === 'due_date' ? (
                  <input
                    type="date"
                    value={editValue}
                    onChange={(e) => setEditValue(e.target.value)}
                    onBlur={() => saveField('due_date', editValue)}
                    onKeyDown={(e) => handleKeyDown(e, 'due_date')}
                    className="w-full text-sm bg-dark-bg-primary border border-dark-border-subtle text-dark-text-primary rounded-md px-3 py-1.5 focus:ring-1 focus:ring-primary-500 focus:border-primary-500 outline-none"
                    autoFocus
                  />
                ) : (
                  <button
                    onClick={() => startEdit('due_date', task.due_date?.split('T')[0] || '')}
                    className="text-sm text-dark-text-primary hover:bg-dark-bg-tertiary/50 px-3 py-1.5 rounded-md w-full text-left transition-colors"
                  >
                    {task.due_date ? new Date(task.due_date).toLocaleDateString() : 'None'}
                  </button>
                )}
              </SidebarField>

              {/* Estimated Hours */}
              <SidebarField label="Estimated Hours">
                {editingField === 'estimated_hours' ? (
                  <input
                    type="number"
                    step="0.5"
                    value={editValue}
                    onChange={(e) => setEditValue(e.target.value)}
                    onBlur={() => saveField('estimated_hours', editValue)}
                    onKeyDown={(e) => handleKeyDown(e, 'estimated_hours')}
                    className="w-full text-sm bg-dark-bg-primary border border-dark-border-subtle text-dark-text-primary rounded-md px-3 py-1.5 focus:ring-1 focus:ring-primary-500 focus:border-primary-500 outline-none"
                    autoFocus
                  />
                ) : (
                  <button
                    onClick={() => startEdit('estimated_hours', String(task.estimated_hours ?? 0))}
                    className="text-sm text-dark-text-primary hover:bg-dark-bg-tertiary/50 px-3 py-1.5 rounded-md w-full text-left transition-colors"
                  >
                    {task.estimated_hours ?? 0}h
                  </button>
                )}
              </SidebarField>

              {/* Actual Hours */}
              <SidebarField label="Actual Hours">
                {editingField === 'actual_hours' ? (
                  <input
                    type="number"
                    step="0.5"
                    value={editValue}
                    onChange={(e) => setEditValue(e.target.value)}
                    onBlur={() => saveField('actual_hours', editValue)}
                    onKeyDown={(e) => handleKeyDown(e, 'actual_hours')}
                    className="w-full text-sm bg-dark-bg-primary border border-dark-border-subtle text-dark-text-primary rounded-md px-3 py-1.5 focus:ring-1 focus:ring-primary-500 focus:border-primary-500 outline-none"
                    autoFocus
                  />
                ) : (
                  <button
                    onClick={() => startEdit('actual_hours', String(task.actual_hours ?? 0))}
                    className="text-sm text-dark-text-primary hover:bg-dark-bg-tertiary/50 px-3 py-1.5 rounded-md w-full text-left transition-colors"
                  >
                    {task.actual_hours ?? 0}h
                  </button>
                )}
              </SidebarField>

              {/* Tags */}
              {task.tags && task.tags.length > 0 && (
                <SidebarField label="Tags">
                  <div className="flex flex-wrap gap-1.5 px-3 py-1.5">
                    {task.tags.map((tag) => (
                      <span
                        key={tag.id}
                        className="inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium rounded-md border border-dark-border-subtle"
                        style={{ backgroundColor: tag.color + '20', color: tag.color }}
                      >
                        <div className="w-1.5 h-1.5 rounded-full" style={{ backgroundColor: tag.color }} />
                        {tag.name}
                      </span>
                    ))}
                  </div>
                </SidebarField>
              )}
            </div>

            {/* Timestamps */}
            <div className="mt-4 space-y-2 text-xs text-dark-text-tertiary px-1">
              {task.created_at && (
                <div>
                  Created <span className="text-dark-text-secondary">{new Date(task.created_at).toLocaleDateString()}</span>
                </div>
              )}
              {task.updated_at && (
                <div>
                  Updated <span className="text-dark-text-secondary">{new Date(task.updated_at).toLocaleDateString()}</span>
                </div>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Error notification */}
      {error && task && (
        <div className="fixed bottom-4 right-4 bg-danger-500/10 border border-danger-500/30 text-danger-400 px-4 py-3 rounded-lg shadow-lg z-50">
          <div className="flex items-center gap-2">
            <span>{error}</span>
            <button onClick={() => setError('')} className="text-danger-400 hover:text-danger-300">
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>
      )}

      {/* Image Picker Modal */}
      {imagePickerTarget && (
        <ImagePickerModal
          onSelect={insertImageMarkdown}
          onClose={() => setImagePickerTarget(null)}
          taskId={task!.id!}
          onUploadComplete={loadAttachments}
        />
      )}

      {/* Alt Text Modal */}
      {pendingFile && (
        <AltTextModal
          file={pendingFile}
          uploading={uploading}
          onConfirm={(altText) => handleConfirmUpload(pendingFile, altText)}
          onCancel={() => setPendingFile(null)}
        />
      )}

      {/* Confirm Modal */}
      {confirmAction && (
        <ConfirmModal
          message={confirmAction.message}
          onConfirm={confirmAction.onConfirm}
          onCancel={() => setConfirmAction(null)}
        />
      )}
    </div>
  )
}

/* Image Picker Modal - imported from shared component */

/* Alt Text Modal */

function AltTextModal({ file, uploading, onConfirm, onCancel }: {
  file: File
  uploading: boolean
  onConfirm: (altText: string) => void
  onCancel: () => void
}) {
  const defaultAlt = file.name.replace(/\.[^.]+$/, '').replace(/[-_]/g, ' ')
  const [altText, setAltText] = useState(defaultAlt)
  const [previewUrl, setPreviewUrl] = useState<string | null>(null)

  useEffect(() => {
    if (file.type.startsWith('image/')) {
      const url = URL.createObjectURL(file)
      setPreviewUrl(url)
      return () => URL.revokeObjectURL(url)
    }
  }, [file])

  const isValid = altText.trim().length >= 3

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={onCancel}
    >
      <div
        className="w-full max-w-md mx-4 bg-dark-bg-secondary rounded-xl border border-dark-border-subtle shadow-2xl overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="px-6 pt-6 pb-4">
          <h3 className="text-sm font-semibold text-dark-text-primary mb-4">Add Description for Upload</h3>

          {/* Preview */}
          {previewUrl && (
            <div className="mb-4 flex justify-center">
              <img
                src={previewUrl}
                alt="Preview"
                className="max-h-40 rounded-lg border border-dark-border-subtle object-contain"
              />
            </div>
          )}

          <p className="text-xs text-dark-text-tertiary mb-3 truncate">
            File: {file.name} ({(file.size / 1024 / 1024).toFixed(1)} MB)
          </p>

          <div>
            <label className="block text-sm font-medium text-dark-text-primary mb-1">
              Alt text / description <span className="text-danger-400">*</span>
            </label>
            <input
              type="text"
              value={altText}
              onChange={(e) => setAltText(e.target.value)}
              className="w-full px-3 py-2 text-sm bg-dark-bg-primary border border-dark-border-subtle text-dark-text-primary rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 outline-none placeholder-dark-text-tertiary"
              placeholder="Describe this file..."
              autoFocus
              onKeyDown={(e) => {
                if (e.key === 'Enter' && isValid && !uploading) onConfirm(altText.trim())
                if (e.key === 'Escape') onCancel()
              }}
            />
            {altText.trim().length > 0 && altText.trim().length < 3 && (
              <p className="text-xs text-danger-400 mt-1">At least 3 characters required</p>
            )}
          </div>
        </div>

        <div className="flex justify-end gap-2 px-6 py-4 border-t border-dark-border-subtle bg-dark-bg-primary/50">
          <button
            onClick={onCancel}
            disabled={uploading}
            className="px-4 py-2 text-sm font-medium text-dark-text-secondary bg-dark-bg-tertiary hover:bg-dark-bg-tertiary/80 rounded-lg transition-colors disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            onClick={() => onConfirm(altText.trim())}
            disabled={!isValid || uploading}
            className="px-4 py-2 text-sm font-medium text-white bg-primary-500 hover:bg-primary-600 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {uploading ? 'Uploading...' : 'Upload'}
          </button>
        </div>
      </div>
    </div>
  )
}

/* Confirm Modal */

function ConfirmModal({ message, onConfirm, onCancel }: {
  message: string
  onConfirm: () => void
  onCancel: () => void
}) {
  const [loading, setLoading] = useState(false)

  const handleConfirm = async () => {
    setLoading(true)
    await onConfirm()
  }

  return (
    <div
      className="fixed inset-0 z-[70] flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={onCancel}
    >
      <div
        className="w-full max-w-sm mx-4 bg-dark-bg-secondary rounded-xl border border-dark-border-subtle shadow-2xl overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="px-6 pt-6 pb-4">
          <div className="flex items-start gap-3">
            <div className="flex-shrink-0 w-10 h-10 rounded-full bg-danger-500/10 flex items-center justify-center">
              <svg className="w-5 h-5 text-danger-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
              </svg>
            </div>
            <div>
              <h3 className="text-sm font-semibold text-dark-text-primary mb-1">Confirm Delete</h3>
              <p className="text-sm text-dark-text-secondary">{message}</p>
            </div>
          </div>
        </div>
        <div className="flex justify-end gap-2 px-6 py-4 border-t border-dark-border-subtle bg-dark-bg-primary/50">
          <button
            onClick={onCancel}
            disabled={loading}
            className="px-4 py-2 text-sm font-medium text-dark-text-secondary bg-dark-bg-tertiary hover:bg-dark-bg-tertiary/80 rounded-lg transition-colors disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            onClick={handleConfirm}
            disabled={loading}
            className="px-4 py-2 text-sm font-medium text-white bg-danger-500 hover:bg-danger-600 rounded-lg transition-colors disabled:opacity-50"
          >
            {loading ? 'Deleting...' : 'Delete'}
          </button>
        </div>
      </div>
    </div>
  )
}

/* Sidebar helper components */

function SidebarField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="px-4 py-3">
      <label className="block text-[11px] font-medium text-dark-text-tertiary uppercase tracking-wide mb-1">
        {label}
      </label>
      {children}
    </div>
  )
}

function InlineSelect({ value, onChange, options }: {
  value: string
  onChange: (value: string) => void
  options: { value: string; label: string }[]
}) {
  return (
    <div className="relative">
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="w-full appearance-none bg-transparent cursor-pointer text-sm text-dark-text-primary hover:bg-dark-bg-tertiary/50 pl-3 pr-7 py-1.5 rounded-md border border-transparent hover:border-dark-border-subtle focus:border-primary-500 focus:ring-1 focus:ring-primary-500/30 outline-none transition-colors"
      >
        {options.map(opt => (
          <option key={opt.value} value={opt.value} className="bg-dark-bg-secondary text-dark-text-primary">
            {opt.label}
          </option>
        ))}
      </select>
      <svg className="absolute right-2 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-dark-text-tertiary pointer-events-none" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
      </svg>
    </div>
  )
}
