import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, WikiAnnotation, AnnotationComment, AnnotationColor } from '../lib/api'
import { useAuth } from '../state/AuthContext'
import MentionTextarea from './MentionTextarea'

const COLOR_LABELS: Record<AnnotationColor, string> = {
  yellow: 'Note',
  blue: 'Question',
  green: 'Approved',
  red: 'Issue',
}

const COLOR_CLASSES: Record<AnnotationColor, string> = {
  yellow: 'bg-yellow-400/20 border-yellow-400/50 text-yellow-300',
  blue: 'bg-blue-400/20 border-blue-400/50 text-blue-300',
  green: 'bg-green-400/20 border-green-400/50 text-green-300',
  red: 'bg-red-400/20 border-red-400/50 text-red-300',
}

const DOT_CLASSES: Record<AnnotationColor, string> = {
  yellow: 'bg-yellow-400',
  blue: 'bg-blue-400',
  green: 'bg-green-400',
  red: 'bg-red-400',
}

interface WikiAnnotationSidebarProps {
  annotations: WikiAnnotation[]
  selectedAnnotationId: number | null
  showResolved: boolean
  projectId: number
  onAnnotationSelect: (id: number | null) => void
  onAnnotationUpdate: (annotation: WikiAnnotation) => void
  onAnnotationDelete: (annotationId: number) => void
  onCommentCreate: (annotationId: number, comment: AnnotationComment) => void
  onCommentUpdate: (comment: AnnotationComment) => void
  onCommentDelete: (annotationId: number, commentId: number) => void
  onToggleShowResolved: () => void
}

export default function WikiAnnotationSidebar({
  annotations,
  selectedAnnotationId,
  showResolved,
  projectId,
  onAnnotationSelect,
  onAnnotationUpdate,
  onAnnotationDelete,
  onCommentCreate,
  onCommentUpdate,
  onCommentDelete,
  onToggleShowResolved,
}: Readonly<WikiAnnotationSidebarProps>) {
  const { user } = useAuth()

  const visible = showResolved ? annotations : annotations.filter(a => !a.resolved)

  return (
    <div className="w-72 border-l border-dark-border-subtle bg-dark-bg-secondary flex flex-col h-full">
      {/* Header */}
      <div className="px-4 py-3 border-b border-dark-border-subtle flex items-center justify-between flex-shrink-0">
        <h3 className="text-sm font-semibold text-dark-text-primary">
          Annotations
          {annotations.length > 0 && (
            <span className="ml-2 text-xs font-normal text-dark-text-tertiary">
              ({visible.length}{showResolved ? '' : ` of ${annotations.length}`})
            </span>
          )}
        </h3>
        <button
          onClick={onToggleShowResolved}
          className={`text-xs px-2 py-1 rounded transition-colors ${
            showResolved
              ? 'bg-green-500/15 text-green-400 hover:bg-green-500/25'
              : 'bg-dark-bg-tertiary text-dark-text-tertiary hover:text-dark-text-secondary'
          }`}
          title={showResolved ? 'Hide resolved' : 'Show resolved'}
        >
          {showResolved ? 'Hide resolved' : 'Show resolved'}
        </button>
      </div>

      {/* List */}
      <div className="flex-1 overflow-y-auto">
        {visible.length === 0 ? (
          <div className="p-6 text-center text-dark-text-tertiary text-sm">
            <svg className="w-10 h-10 mx-auto mb-3 opacity-30" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
            </svg>
            Select text in the preview to annotate
          </div>
        ) : (
          <div className="py-2">
            {visible.map(annotation => (
              <AnnotationCard
                key={annotation.id}
                annotation={annotation}
                isSelected={selectedAnnotationId === annotation.id}
                currentUserId={user?.id ?? 0}
                projectId={projectId}
                onSelect={() => onAnnotationSelect(
                  selectedAnnotationId === annotation.id ? null : annotation.id
                )}
                onUpdate={onAnnotationUpdate}
                onDelete={onAnnotationDelete}
                onCommentCreate={onCommentCreate}
                onCommentUpdate={onCommentUpdate}
                onCommentDelete={onCommentDelete}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

// ── AnnotationCard ────────────────────────────────────────────────────────

interface AnnotationCardProps {
  annotation: WikiAnnotation
  isSelected: boolean
  currentUserId: number
  projectId: number
  onSelect: () => void
  onUpdate: (annotation: WikiAnnotation) => void
  onDelete: (annotationId: number) => void
  onCommentCreate: (annotationId: number, comment: AnnotationComment) => void
  onCommentUpdate: (comment: AnnotationComment) => void
  onCommentDelete: (annotationId: number, commentId: number) => void
}

function AnnotationCard({
  annotation,
  isSelected,
  currentUserId,
  projectId,
  onSelect,
  onUpdate,
  onDelete,
  onCommentCreate,
  onCommentUpdate,
  onCommentDelete,
}: Readonly<AnnotationCardProps>) {
  const [replyText, setReplyText] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [replyTo, setReplyTo] = useState<number | null>(null) // parent comment id

  const handleResolveToggle = async () => {
    try {
      const updated = await api.updateWikiAnnotation(annotation.id, { resolved: !annotation.resolved })
      onUpdate(updated)
    } catch {
      // silent fail
    }
  }

  const handleDelete = async () => {
    if (!confirm('Delete this annotation and all its comments?')) return
    try {
      await api.deleteWikiAnnotation(annotation.id)
      onDelete(annotation.id)
    } catch {
      // silent fail
    }
  }

  const handleSubmitReply = async () => {
    if (!replyText.trim() || submitting) return
    setSubmitting(true)
    try {
      const comment = await api.createAnnotationComment(annotation.id, {
        content: replyText.trim(),
        parent_comment_id: replyTo ?? undefined,
      })
      onCommentCreate(annotation.id, comment)
      setReplyText('')
      setReplyTo(null)
    } catch {
      // silent fail
    } finally {
      setSubmitting(false)
    }
  }

  const rootComments = annotation.comments.filter(c => !c.parent_comment_id)
  const replies = (parentId: number) => annotation.comments.filter(c => c.parent_comment_id === parentId)

  return (
    <div
      className={`mx-2 mb-2 rounded-lg border transition-colors cursor-pointer ${
        isSelected
          ? `border-l-2 ${COLOR_CLASSES[annotation.color]} shadow-sm`
          : 'border-dark-border-subtle hover:border-dark-border-subtle/80 bg-dark-bg-primary/30'
      } ${annotation.resolved ? 'opacity-60' : ''}`}
      onClick={onSelect}
    >
      {/* Annotation header */}
      <div className="px-3 pt-3 pb-2">
        <div className="flex items-start gap-2">
          <div className={`w-2.5 h-2.5 rounded-full mt-1 flex-shrink-0 ${DOT_CLASSES[annotation.color]}`} />
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-1.5 mb-1">
              <span className={`text-xs font-medium px-1.5 py-0.5 rounded border ${COLOR_CLASSES[annotation.color]}`}>
                {COLOR_LABELS[annotation.color]}
              </span>
              {annotation.resolved && (
                <span className="text-xs text-green-400 font-medium">Resolved</span>
              )}
            </div>
            <blockquote className="text-xs text-dark-text-secondary italic line-clamp-2 border-l-2 border-dark-border-subtle pl-2">
              "{annotation.selected_text}"
            </blockquote>
            <div className="flex items-center gap-2 mt-1.5 text-xs text-dark-text-tertiary">
              <Link
                to={`/app/users/${annotation.author_id}`}
                className="hover:text-primary-400 transition-colors"
                onClick={(e) => e.stopPropagation()}
              >
                {annotation.author_name ?? 'Unknown'}
              </Link>
              <span>·</span>
              <span>{new Date(annotation.created_at).toLocaleDateString()}</span>
            </div>
          </div>
          {currentUserId === annotation.author_id && (
            <div className="flex items-center gap-1 flex-shrink-0">
              <button
                onClick={(e) => { e.stopPropagation(); handleResolveToggle() }}
                className={`p-1 rounded transition-colors ${annotation.resolved ? 'text-green-400 hover:text-green-300' : 'text-dark-text-tertiary hover:text-green-400'}`}
                title={annotation.resolved ? 'Unresolve' : 'Resolve'}
              >
                <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                </svg>
              </button>
              <button
                onClick={(e) => { e.stopPropagation(); handleDelete() }}
                className="p-1 rounded text-dark-text-tertiary hover:text-red-400 transition-colors"
                title="Delete annotation"
              >
                <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                </svg>
              </button>
            </div>
          )}
        </div>
      </div>

      {/* Comments (only when expanded/selected) */}
      {isSelected && (
        <div
          className="border-t border-dark-border-subtle/50 px-3 py-2 space-y-2"
          onClick={(e) => e.stopPropagation()}
        >
          {rootComments.map(comment => (
            <CommentThread
              key={comment.id}
              comment={comment}
              replies={replies(comment.id)}
              currentUserId={currentUserId}
              onReplyClick={(id) => setReplyTo(prev => prev === id ? null : id)}
              activeReplyId={replyTo}
              onUpdate={onCommentUpdate}
              onDelete={(commentId) => onCommentDelete(annotation.id, commentId)}
            />
          ))}

          {/* Reply input */}
          <div className="pt-1">
            {replyTo !== null && (
              <div className="text-xs text-dark-text-tertiary mb-1 flex items-center gap-1">
                Replying to comment
                <button onClick={() => setReplyTo(null)} className="text-primary-400 hover:underline">cancel</button>
              </div>
            )}
            <div className="flex gap-2">
              <MentionTextarea
                value={replyText}
                onChange={setReplyText}
                projectId={projectId}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault()
                    handleSubmitReply()
                  }
                }}
                placeholder="Add a comment... (@ to mention)"
                rows={2}
                className="flex-1 px-2 py-1.5 bg-dark-bg-primary border border-dark-border-subtle rounded text-xs text-dark-text-primary placeholder-dark-text-tertiary/50 focus:outline-none focus:border-primary-500 resize-none"
              />
              <button
                onClick={handleSubmitReply}
                disabled={!replyText.trim() || submitting}
                className="px-2 py-1.5 bg-primary-600 text-white rounded text-xs font-medium hover:bg-primary-500 disabled:opacity-40 transition-colors self-end"
              >
                Send
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ── CommentThread ────────────────────────────────────────────────────────

interface CommentThreadProps {
  comment: AnnotationComment
  replies: AnnotationComment[]
  currentUserId: number
  onReplyClick: (commentId: number) => void
  activeReplyId: number | null
  onUpdate: (comment: AnnotationComment) => void
  onDelete: (commentId: number) => void
}

function CommentThread({
  comment,
  replies,
  currentUserId,
  onReplyClick,
  activeReplyId,
  onUpdate,
  onDelete,
}: Readonly<CommentThreadProps>) {
  const [editing, setEditing] = useState(false)
  const [editText, setEditText] = useState(comment.content)
  const [saving, setSaving] = useState(false)

  const handleSaveEdit = async () => {
    if (!editText.trim() || saving) return
    setSaving(true)
    try {
      const updated = await api.updateAnnotationComment(comment.id, { content: editText.trim() })
      onUpdate(updated)
      setEditing(false)
    } catch {
      // silent fail
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!confirm('Delete this comment?')) return
    try {
      await api.deleteAnnotationComment(comment.id)
      onDelete(comment.id)
    } catch {
      // silent fail
    }
  }

  return (
    <div className={`${comment.resolved ? 'opacity-50' : ''}`}>
      <div className="flex items-start gap-2 text-xs">
        <div className="w-5 h-5 rounded-full bg-dark-bg-tertiary flex items-center justify-center text-dark-text-tertiary flex-shrink-0 mt-0.5 text-[10px] font-bold">
          {(comment.author_name ?? '?')[0].toUpperCase()}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-1.5 mb-0.5">
            <Link
              to={`/app/users/${comment.author_id}`}
              className="font-medium text-dark-text-primary hover:text-primary-400 transition-colors"
            >
              {comment.author_name ?? 'Unknown'}
            </Link>
            <span className="text-dark-text-tertiary">{new Date(comment.created_at).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}</span>
            {comment.resolved && <span className="text-green-400 text-[10px]">resolved</span>}
          </div>
          {editing ? (
            <div className="flex gap-1.5 mt-1">
              <textarea
                value={editText}
                onChange={(e) => setEditText(e.target.value)}
                rows={2}
                autoFocus
                className="flex-1 px-2 py-1 bg-dark-bg-primary border border-primary-500 rounded text-xs text-dark-text-primary focus:outline-none resize-none"
              />
              <div className="flex flex-col gap-1">
                <button
                  onClick={handleSaveEdit}
                  disabled={saving}
                  className="px-2 py-1 bg-primary-600 text-white rounded text-[10px] hover:bg-primary-500 disabled:opacity-40"
                >
                  Save
                </button>
                <button
                  onClick={() => { setEditing(false); setEditText(comment.content) }}
                  className="px-2 py-1 bg-dark-bg-tertiary text-dark-text-secondary rounded text-[10px] hover:bg-dark-bg-tertiary/80"
                >
                  Cancel
                </button>
              </div>
            </div>
          ) : (
            <p className="text-dark-text-secondary whitespace-pre-wrap break-words">{comment.content}</p>
          )}
          <div className="flex items-center gap-2 mt-1">
            <button
              onClick={() => onReplyClick(comment.id)}
              className={`text-[10px] transition-colors ${activeReplyId === comment.id ? 'text-primary-400' : 'text-dark-text-tertiary hover:text-dark-text-secondary'}`}
            >
              Reply
            </button>
            {currentUserId === comment.author_id && !editing && (
              <>
                <button
                  onClick={() => setEditing(true)}
                  className="text-[10px] text-dark-text-tertiary hover:text-dark-text-secondary transition-colors"
                >
                  Edit
                </button>
                <button
                  onClick={handleDelete}
                  className="text-[10px] text-dark-text-tertiary hover:text-red-400 transition-colors"
                >
                  Delete
                </button>
              </>
            )}
          </div>
        </div>
      </div>

      {/* Nested replies */}
      {replies.length > 0 && (
        <div className="ml-7 mt-1.5 space-y-1.5 border-l border-dark-border-subtle/50 pl-2">
          {replies.map(reply => (
            <CommentThread
              key={reply.id}
              comment={reply}
              replies={[]} // only one level of nesting for MVP
              currentUserId={currentUserId}
              onReplyClick={onReplyClick}
              activeReplyId={activeReplyId}
              onUpdate={onUpdate}
              onDelete={onDelete}
            />
          ))}
        </div>
      )}
    </div>
  )
}
