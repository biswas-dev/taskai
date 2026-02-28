import { useEffect, useState, useCallback, useRef } from 'react'
import { WikiPage, apiClient } from '../lib/api'
import ImagePickerModal from './ImagePickerModal'
import * as Y from 'yjs'
import { WebsocketProvider } from 'y-websocket'

// Use relative URL in production, or VITE_API_URL for dev override
const API_BASE = import.meta.env.VITE_API_URL || (import.meta.env.PROD ? '' : 'http://localhost:8080')
const PREVIEW_ENDPOINT = `${API_BASE}/api/wiki/preview`
const PREVIEW_DEBOUNCE_MS = 350

interface WikiEditorProps {
  page: WikiPage
}

// ── Toolbar helpers ──────────────────────────────────────────────

function insertAtCursor(
  textarea: HTMLTextAreaElement,
  before: string,
  after: string,
  content: string,
  setContent: (c: string) => void,
  syncToYjs: (c: string) => void,
) {
  const start = textarea.selectionStart
  const end = textarea.selectionEnd
  const selected = content.substring(start, end)
  const insert = before + (selected || 'text') + after
  const newContent = content.substring(0, start) + insert + content.substring(end)
  setContent(newContent)
  syncToYjs(newContent)

  const cursorPos = selected ? start + before.length + selected.length + after.length : start + before.length
  setTimeout(() => {
    textarea.focus()
    textarea.selectionStart = selected ? cursorPos : start + before.length
    textarea.selectionEnd = selected ? cursorPos : start + before.length + (selected || 'text').length
  }, 0)
}

function insertLine(
  textarea: HTMLTextAreaElement,
  prefix: string,
  content: string,
  setContent: (c: string) => void,
  syncToYjs: (c: string) => void,
) {
  const start = textarea.selectionStart
  // Find beginning of current line
  const lineStart = content.lastIndexOf('\n', start - 1) + 1
  const insert = prefix
  const newContent = content.substring(0, lineStart) + insert + content.substring(lineStart)
  setContent(newContent)
  syncToYjs(newContent)

  setTimeout(() => {
    textarea.focus()
    textarea.selectionStart = textarea.selectionEnd = start + insert.length
  }, 0)
}

// ── Toolbar button definitions ───────────────────────────────────

interface ToolbarAction {
  label: string
  icon: string
  action: 'wrap' | 'line' | 'insert'
  before?: string
  after?: string
  prefix?: string
  text?: string
  title: string
}

const toolbarActions: ToolbarAction[] = [
  { label: 'B', icon: 'B', action: 'wrap', before: '**', after: '**', title: 'Bold' },
  { label: 'I', icon: 'I', action: 'wrap', before: '*', after: '*', title: 'Italic' },
  { label: 'H2', icon: 'H2', action: 'line', prefix: '## ', title: 'Heading 2' },
  { label: 'H3', icon: 'H3', action: 'line', prefix: '### ', title: 'Heading 3' },
  { label: 'UL', icon: '\u2022', action: 'line', prefix: '- ', title: 'Bullet list' },
  { label: 'OL', icon: '1.', action: 'line', prefix: '1. ', title: 'Numbered list' },
  { label: 'BQ', icon: '\u201C', action: 'line', prefix: '> ', title: 'Blockquote' },
  { label: 'HR', icon: '\u2014', action: 'insert', text: '\n---\n', title: 'Horizontal rule' },
  { label: 'Code', icon: '< >', action: 'wrap', before: '`', after: '`', title: 'Inline code' },
  { label: 'Link', icon: '\uD83D\uDD17', action: 'wrap', before: '[', after: '](url)', title: 'Link' },
]

// ── Server-side preview fetcher ──────────────────────────────────

async function fetchPreview(markdown: string, signal?: AbortSignal): Promise<string> {
  const token = localStorage.getItem('auth_token')
  const resp = await fetch(PREVIEW_ENDPOINT, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify({ content: markdown }),
    signal,
  })
  if (!resp.ok) throw new Error(`Preview failed: ${resp.status}`)
  const data = await resp.json()
  return data.html
}

// ── Component ────────────────────────────────────────────────────

export default function WikiEditor({ page }: WikiEditorProps) {
  const [content, setContent] = useState('')
  const [isPreview, setIsPreview] = useState(false)
  const [previewHTML, setPreviewHTML] = useState('')
  const [syncState, setSyncState] = useState<'connecting' | 'connected' | 'disconnected'>('connecting')
  const [lastSaved, setLastSaved] = useState<Date | null>(null)
  const [showImagePicker, setShowImagePicker] = useState(false)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [fsPreviewHTML, setFsPreviewHTML] = useState('')

  const ydocRef = useRef<Y.Doc | null>(null)
  const providerRef = useRef<WebsocketProvider | null>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const fsTextareaRef = useRef<HTMLTextAreaElement>(null)
  const previewTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const abortRef = useRef<AbortController | null>(null)
  const dividerRef = useRef<HTMLDivElement>(null)
  const fsContainerRef = useRef<HTMLDivElement>(null)
  const [fsSplitPct, setFsSplitPct] = useState(50)
  const [isDragOver, setIsDragOver] = useState(false)
  const [isDropUploading, setIsDropUploading] = useState(false)
  const dragCounterRef = useRef(0)
  const [editImgList, setEditImgList] = useState<Array<{ html: string; url: string; alt: string; caption: string; index: number }> | null>(null)
  const [selectedEditImg, setSelectedEditImg] = useState<{ html: string; url: string; alt: string; caption: string; index: number } | null>(null)
  const [editAlt, setEditAlt] = useState('')
  const [editCaption, setEditCaption] = useState('')

  // ── Yjs setup ────────────────────────────────────────────────

  useEffect(() => {
    const ydoc = new Y.Doc()
    ydocRef.current = ydoc
    const ytext = ydoc.getText('content')

    const token = localStorage.getItem('token')
    if (!token) {
      console.error('No auth token found')
      setSyncState('disconnected')
      return
    }

    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsHost = window.location.host
    const wsUrl = `${wsProtocol}//${wsHost}/api/wiki/collab`

    const provider = new WebsocketProvider(wsUrl, `wiki-page-${page.id}`, ydoc, {
      params: { token },
      connect: true,
    })
    providerRef.current = provider

    provider.on('status', ({ status }: { status: string }) => {
      if (status === 'connected') {
        setSyncState('connected')
        setLastSaved(new Date())
      } else if (status === 'disconnected') {
        setSyncState('disconnected')
      } else {
        setSyncState('connecting')
      }
    })

    const updateContent = () => setContent(ytext.toString())
    ytext.observe(updateContent)
    updateContent()

    return () => {
      ytext.unobserve(updateContent)
      provider.destroy()
      ydoc.destroy()
    }
  }, [page.id])

  // ── Yjs sync helper ──────────────────────────────────────────

  const syncToYjs = useCallback((newContent: string) => {
    if (!ydocRef.current) return
    const ytext = ydocRef.current.getText('content')
    const currentContent = ytext.toString()
    if (newContent !== currentContent) {
      ydocRef.current.transact(() => {
        ytext.delete(0, currentContent.length)
        ytext.insert(0, newContent)
      })
    }
  }, [])

  const handleTextareaChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const newContent = e.target.value
    setContent(newContent)
    syncToYjs(newContent)
  }

  // ── Toolbar action dispatcher ────────────────────────────────

  const execToolbarAction = useCallback((action: ToolbarAction) => {
    const textarea = isFullscreen ? fsTextareaRef.current : textareaRef.current
    if (!textarea) return

    if (action.action === 'wrap') {
      insertAtCursor(textarea, action.before!, action.after!, content, setContent, syncToYjs)
    } else if (action.action === 'line') {
      insertLine(textarea, action.prefix!, content, setContent, syncToYjs)
    } else if (action.action === 'insert') {
      const start = textarea.selectionStart
      const newContent = content.substring(0, start) + action.text! + content.substring(start)
      setContent(newContent)
      syncToYjs(newContent)
      setTimeout(() => {
        textarea.focus()
        textarea.selectionStart = textarea.selectionEnd = start + action.text!.length
      }, 0)
    }
  }, [content, syncToYjs, isFullscreen])

  // ── Server-side preview (inline mode) ────────────────────────

  const loadPreview = useCallback(async (markdown: string) => {
    if (abortRef.current) abortRef.current.abort()
    const controller = new AbortController()
    abortRef.current = controller
    try {
      const html = await fetchPreview(markdown, controller.signal)
      setPreviewHTML(html)
    } catch {
      // aborted or network error — ignore
    }
  }, [])

  useEffect(() => {
    if (isPreview) loadPreview(content)
  }, [isPreview, content, loadPreview])

  // ── Fullscreen live preview (debounced) ──────────────────────

  const scheduleFsPreview = useCallback((markdown: string) => {
    if (previewTimerRef.current) clearTimeout(previewTimerRef.current)
    previewTimerRef.current = setTimeout(async () => {
      if (abortRef.current) abortRef.current.abort()
      const controller = new AbortController()
      abortRef.current = controller
      try {
        const html = await fetchPreview(markdown, controller.signal)
        setFsPreviewHTML(html)
      } catch {
        // ignore
      }
    }, PREVIEW_DEBOUNCE_MS)
  }, [])

  // When fullscreen opens, render preview immediately; schedule on content change
  useEffect(() => {
    if (isFullscreen) scheduleFsPreview(content)
  }, [isFullscreen, content, scheduleFsPreview])

  // ── Keyboard shortcuts ───────────────────────────────────────

  const handleKeyDown = useCallback((e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    // Tab → 2-space indent
    if (e.key === 'Tab') {
      e.preventDefault()
      const textarea = e.currentTarget
      const start = textarea.selectionStart
      const end = textarea.selectionEnd
      const newContent = content.substring(0, start) + '  ' + content.substring(end)
      setContent(newContent)
      syncToYjs(newContent)
      setTimeout(() => {
        textarea.selectionStart = textarea.selectionEnd = start + 2
      }, 0)
      return
    }

    // F11 → toggle fullscreen
    if (e.key === 'F11') {
      e.preventDefault()
      setIsFullscreen(prev => !prev)
      return
    }

    // Escape → exit fullscreen
    if (e.key === 'Escape' && isFullscreen) {
      e.preventDefault()
      setIsFullscreen(false)
      return
    }
  }, [content, syncToYjs, isFullscreen])

  // Global F11 listener (when focus is not on textarea)
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'F11') {
        e.preventDefault()
        setIsFullscreen(prev => !prev)
      }
      if (e.key === 'Escape' && isFullscreen) {
        e.preventDefault()
        setIsFullscreen(false)
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [isFullscreen])

  // ── Fullscreen divider resize ────────────────────────────────

  const handleDividerMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    const container = fsContainerRef.current
    if (!container) return

    const onMove = (ev: MouseEvent) => {
      const rect = container.getBoundingClientRect()
      const pct = ((ev.clientX - rect.left) / rect.width) * 100
      setFsSplitPct(Math.max(20, Math.min(80, pct)))
    }
    const onUp = () => {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
    }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
  }, [])

  // ── Image picker ─────────────────────────────────────────────

  const insertImageMarkdown = (alt: string, url: string, caption?: string) => {
    const textarea = isFullscreen ? fsTextareaRef.current : textareaRef.current
    const escHtml = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
    let markup: string
    if (caption) {
      markup = `<figure style="text-align:center;margin:1.5rem 0"><a href="${url}" data-lightbox="article-images" data-title="${escHtml(alt)}"><img src="${url}" alt="${escHtml(alt)}" style="width:66%;height:auto;max-width:100%"/></a><figcaption>${escHtml(caption)}</figcaption></figure>\n`
    } else {
      markup = `![${alt}](${url})`
    }

    if (textarea) {
      const start = textarea.selectionStart
      const end = textarea.selectionEnd
      const newContent = content.substring(0, start) + markup + content.substring(end)
      setContent(newContent)
      syncToYjs(newContent)
      setTimeout(() => {
        textarea.selectionStart = textarea.selectionEnd = start + markup.length
        textarea.focus()
      }, 0)
    } else {
      const newContent = content + (content.endsWith('\n') ? '' : '\n') + markup + '\n'
      setContent(newContent)
      syncToYjs(newContent)
    }
    setShowImagePicker(false)
  }

  // ── Drag & drop image upload ─────────────────────────────────

  const handleDragEnter = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    dragCounterRef.current++
    if (e.dataTransfer.types.includes('Files')) setIsDragOver(true)
  }, [])

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'
  }, [])

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    dragCounterRef.current--
    if (dragCounterRef.current <= 0) {
      dragCounterRef.current = 0
      setIsDragOver(false)
    }
  }, [])

  const handleDrop = useCallback(async (e: React.DragEvent) => {
    e.preventDefault()
    dragCounterRef.current = 0
    setIsDragOver(false)

    const files = Array.from(e.dataTransfer.files).filter(f => f.type.startsWith('image/'))
    if (files.length === 0) return

    setIsDropUploading(true)
    const textarea = isFullscreen ? fsTextareaRef.current : textareaRef.current

    try {
      for (const file of files) {
        const sig = await apiClient.getUploadSignature({ pageId: page.id })

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
        if (!uploadRes.ok) throw new Error('Upload failed')
        const uploadData = await uploadRes.json()

        const altName = file.name.replace(/\.[^.]+$/, '').replace(/[-_]/g, ' ')

        await apiClient.createWikiPageAttachment(page.id, {
          filename: file.name,
          alt_name: altName,
          file_type: 'image',
          content_type: file.type,
          file_size: file.size,
          cloudinary_url: uploadData.secure_url,
          cloudinary_public_id: uploadData.public_id,
        })

        // Insert into editor
        const markup = `![${altName}](${uploadData.secure_url})`
        const start = textarea?.selectionStart ?? content.length
        const end = textarea?.selectionEnd ?? content.length
        const newContent = content.substring(0, start) + markup + '\n' + content.substring(end)
        setContent(newContent)
        syncToYjs(newContent)
      }
    } catch (err) {
      console.error('Drop upload failed:', err)
    } finally {
      setIsDropUploading(false)
    }
  }, [isFullscreen, content, syncToYjs, page.id])

  // ── Status helpers ───────────────────────────────────────────

  const getSyncStatusColor = () => {
    switch (syncState) {
      case 'connected': return 'bg-green-500'
      case 'connecting': return 'bg-yellow-500'
      case 'disconnected': return 'bg-red-500'
    }
  }
  const getSyncStatusText = () => {
    switch (syncState) {
      case 'connected': return 'Connected'
      case 'connecting': return 'Connecting...'
      case 'disconnected': return 'Disconnected'
    }
  }

  // ── Edit existing image ─────────────────────────────────────

  const handleEditImg = useCallback(() => {
    const images: Array<{ html: string; url: string; alt: string; caption: string; index: number }> = []

    // Match <figure> blocks with <img>
    const figureRegex = /<figure[^>]*>[\s\S]*?<img\s[^>]*src="([^"]+)"[^>]*?(?:alt="([^"]*)")?[\s\S]*?(?:<figcaption>([\s\S]*?)<\/figcaption>)?[\s\S]*?<\/figure>/g
    let match
    while ((match = figureRegex.exec(content)) !== null) {
      // Also try to extract alt from img if it wasn't in the first capture
      let alt = match[2] || ''
      if (!alt) {
        const altMatch = match[0].match(/alt="([^"]*)"/)
        if (altMatch) alt = altMatch[1]
      }
      images.push({
        html: match[0],
        url: match[1],
        alt: alt.replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"'),
        caption: (match[3] || '').replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"'),
        index: match.index,
      })
    }

    // Match markdown images ![alt](url)
    const mdRegex = /!\[([^\]]*)\]\(([^)]+)\)/g
    while ((match = mdRegex.exec(content)) !== null) {
      // Skip if this position is already inside a <figure> we found
      const pos = match.index
      const insideFigure = images.some(img => pos >= img.index && pos < img.index + img.html.length)
      if (insideFigure) continue

      images.push({
        html: match[0],
        url: match[2],
        alt: match[1],
        caption: '',
        index: match.index,
      })
    }

    if (images.length === 0) {
      alert('No images found in content')
      return
    }

    // Sort by position in content
    images.sort((a, b) => a.index - b.index)
    setEditImgList(images)
    setSelectedEditImg(null)
  }, [content])

  const selectImgForEdit = (img: typeof editImgList extends Array<infer T> | null ? T : never) => {
    setSelectedEditImg(img)
    setEditAlt(img.alt)
    setEditCaption(img.caption)
  }

  const saveEditImg = useCallback(() => {
    if (!selectedEditImg) return

    const escHtml = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
    let newMarkup: string

    if (editCaption || selectedEditImg.html.startsWith('<figure')) {
      // Build <figure> block
      newMarkup = `<figure style="text-align:center;margin:1.5rem 0"><a href="${selectedEditImg.url}" data-lightbox="article-images" data-title="${escHtml(editAlt)}"><img src="${selectedEditImg.url}" alt="${escHtml(editAlt)}" style="width:66%;height:auto;max-width:100%"/></a>${editCaption ? `<figcaption>${escHtml(editCaption)}</figcaption>` : ''}</figure>`
    } else {
      // Plain markdown image
      newMarkup = `![${editAlt}](${selectedEditImg.url})`
    }

    const newContent = content.replace(selectedEditImg.html, newMarkup)
    setContent(newContent)
    syncToYjs(newContent)
    setEditImgList(null)
    setSelectedEditImg(null)
  }, [selectedEditImg, editAlt, editCaption, content, syncToYjs])

  // ── Draw handler ────────────────────────────────────────────

  const [isDrawCreating, setIsDrawCreating] = useState(false)

  const handleDraw = useCallback(async () => {
    if (isDrawCreating) return
    setIsDrawCreating(true)
    try {
      const res = await fetch('/draw/api/new', { method: 'POST' })
      const data = await res.json()
      if (data && data.id) {
        const textarea = isFullscreen ? fsTextareaRef.current : textareaRef.current
        const markup = `\n[draw:${data.id}:edit]\n`
        if (textarea) {
          const start = textarea.selectionStart
          const end = textarea.selectionEnd
          const newContent = content.substring(0, start) + markup + content.substring(end)
          setContent(newContent)
          syncToYjs(newContent)
          setTimeout(() => {
            textarea.focus()
            textarea.selectionStart = textarea.selectionEnd = start + markup.length
          }, 0)
        } else {
          const newContent = content + markup
          setContent(newContent)
          syncToYjs(newContent)
        }
      }
    } catch (err) {
      console.error('Failed to create drawing:', err)
    } finally {
      setIsDrawCreating(false)
    }
  }, [isDrawCreating, isFullscreen, content, setContent, syncToYjs])

  // ── Toolbar JSX ──────────────────────────────────────────────

  const Toolbar = ({ compact }: { compact?: boolean }) => (
    <div className={`flex items-center gap-1 ${compact ? '' : 'flex-wrap'}`}>
      {toolbarActions.map((action) => (
        <button
          key={action.label}
          onClick={() => execToolbarAction(action)}
          className="px-2 py-1 rounded text-xs font-mono font-medium transition-colors bg-dark-bg-tertiary text-dark-text-secondary hover:bg-dark-bg-tertiary/80 hover:text-dark-text-primary"
          title={action.title}
        >
          {action.icon}
        </button>
      ))}
      <button
        onClick={() => setShowImagePicker(true)}
        className="px-2 py-1 rounded text-xs font-medium transition-colors bg-dark-bg-tertiary text-dark-text-secondary hover:bg-dark-bg-tertiary/80 hover:text-dark-text-primary flex items-center gap-1"
        title="Insert image"
      >
        <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
        </svg>
      </button>
      <button
        onClick={handleEditImg}
        className="px-2 py-1 rounded text-xs font-medium transition-colors bg-dark-bg-tertiary text-dark-text-secondary hover:bg-dark-bg-tertiary/80 hover:text-dark-text-primary flex items-center gap-1"
        title="Edit image in content"
      >
        <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
        </svg>
      </button>
      <button
        onClick={handleDraw}
        disabled={isDrawCreating}
        className="px-2 py-1 rounded text-xs font-medium transition-colors bg-dark-bg-tertiary text-dark-text-secondary hover:bg-dark-bg-tertiary/80 hover:text-dark-text-primary flex items-center gap-1 disabled:opacity-50"
        title="Insert a drawing canvas"
      >
        <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
          <path d="M12 19l7-7 3 3-7 7-3-3z" />
          <path d="M18 13l-1.5-7.5L2 2l3.5 14.5L13 18l5-5z" />
          <path d="M2 2l7.586 7.586" />
          <circle cx="11" cy="11" r="2" />
        </svg>
        Draw
      </button>
    </div>
  )

  // ── Fullscreen overlay ───────────────────────────────────────

  if (isFullscreen) {
    return (
      <>
        <div className="fixed inset-0 z-50 bg-dark-bg-primary flex flex-col">
          {/* Top bar */}
          <div className="flex items-center justify-between border-b border-dark-border-subtle bg-dark-bg-secondary px-4 py-2">
            <div className="flex items-center gap-4">
              <span className="text-sm font-semibold text-dark-text-primary truncate max-w-xs">
                {page.title}
              </span>
              <Toolbar compact />
            </div>
            <div className="flex items-center gap-3">
              <div className="flex items-center gap-2">
                <div className={`w-2 h-2 rounded-full ${getSyncStatusColor()}`} />
                <span className="text-xs text-dark-text-tertiary">{getSyncStatusText()}</span>
              </div>
              <button
                onClick={() => setIsFullscreen(false)}
                className="px-3 py-1.5 rounded text-xs font-medium transition-colors bg-dark-bg-tertiary text-dark-text-secondary hover:bg-red-500/20 hover:text-red-400"
                title="Exit fullscreen (Esc)"
              >
                Exit
              </button>
            </div>
          </div>

          {/* Split panes */}
          <div ref={fsContainerRef} className="flex flex-1 overflow-hidden">
            {/* Left: editor */}
            <div
              style={{ width: `${fsSplitPct}%` }}
              className="flex flex-col overflow-hidden relative"
              onDragEnter={handleDragEnter}
              onDragOver={handleDragOver}
              onDragLeave={handleDragLeave}
              onDrop={handleDrop}
            >
              <textarea
                ref={fsTextareaRef}
                value={content}
                onChange={handleTextareaChange}
                onKeyDown={handleKeyDown}
                className="flex-1 w-full bg-dark-bg-primary text-dark-text-primary resize-none focus:outline-none font-mono text-sm p-4"
                spellCheck={false}
                placeholder="Start writing in Markdown..."
              />
              {(isDragOver || isDropUploading) && (
                <div className="absolute inset-0 bg-primary-500/10 border-2 border-dashed border-primary-500 rounded-lg flex flex-col items-center justify-center gap-2 pointer-events-none z-10">
                  <svg className="w-12 h-12 text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
                  </svg>
                  <span className="text-sm font-semibold text-primary-400">
                    {isDropUploading ? 'Uploading...' : 'Drop images here'}
                  </span>
                </div>
              )}
            </div>

            {/* Divider */}
            <div
              ref={dividerRef}
              onMouseDown={handleDividerMouseDown}
              className="w-1.5 bg-dark-border-subtle hover:bg-dark-accent-primary/50 cursor-col-resize transition-colors flex-shrink-0"
            />

            {/* Right: live preview */}
            <div style={{ width: `${100 - fsSplitPct}%` }} className="overflow-y-auto p-4">
              {fsPreviewHTML ? (
                <div
                  className="prose prose-invert max-w-none"
                  dangerouslySetInnerHTML={{ __html: fsPreviewHTML }}
                />
              ) : (
                <div className="flex items-center justify-center h-full text-dark-text-tertiary">
                  <p className="text-sm">Preview will appear here...</p>
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Image picker in fullscreen */}
        {showImagePicker && (
          <ImagePickerModal
            onSelect={insertImageMarkdown}
            onClose={() => setShowImagePicker(false)}
            wikiPageId={page.id}
            onUploadComplete={() => {}}
          />
        )}
      </>
    )
  }

  // ── Normal (non-fullscreen) view ─────────────────────────────

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="border-b border-dark-border-subtle bg-dark-bg-secondary px-6 py-4">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-semibold text-dark-text-primary">{page.title}</h1>
            <div className="flex items-center gap-3 mt-2">
              <div className="flex items-center gap-2">
                <div className={`w-2 h-2 rounded-full ${getSyncStatusColor()}`} />
                <span className="text-sm text-dark-text-tertiary">{getSyncStatusText()}</span>
              </div>
              {lastSaved && (
                <>
                  <span className="text-dark-text-tertiary">&bull;</span>
                  <span className="text-sm text-dark-text-tertiary">
                    Last saved {lastSaved.toLocaleTimeString()}
                  </span>
                </>
              )}
            </div>
          </div>

          <div className="flex items-center gap-2">
            <button
              onClick={() => setIsPreview(false)}
              className={`px-4 py-2 rounded text-sm font-medium transition-colors ${
                !isPreview
                  ? 'bg-dark-accent-primary text-white'
                  : 'bg-dark-bg-tertiary text-dark-text-secondary hover:bg-dark-bg-tertiary/80'
              }`}
            >
              Edit
            </button>
            <button
              onClick={() => setIsPreview(true)}
              className={`px-4 py-2 rounded text-sm font-medium transition-colors ${
                isPreview
                  ? 'bg-dark-accent-primary text-white'
                  : 'bg-dark-bg-tertiary text-dark-text-secondary hover:bg-dark-bg-tertiary/80'
              }`}
            >
              Preview
            </button>
            <button
              onClick={() => setIsFullscreen(true)}
              className="px-3 py-2 rounded text-sm font-medium transition-colors bg-dark-bg-tertiary text-dark-text-secondary hover:bg-dark-bg-tertiary/80"
              title="Fullscreen editor (F11)"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 8V4m0 0h4M4 4l5 5m11-1V4m0 0h-4m4 0l-5 5M4 16v4m0 0h4m-4 0l5-5m11 5l-5-5m5 5v-4m0 4h-4" />
              </svg>
            </button>
          </div>
        </div>
      </div>

      {/* Toolbar (edit mode only) */}
      {!isPreview && (
        <div className="border-b border-dark-border-subtle bg-dark-bg-secondary px-6 py-2">
          <Toolbar />
        </div>
      )}

      {/* Content */}
      <div className="flex-1 overflow-hidden flex flex-col">
        {isPreview ? (
          <div className="h-full overflow-y-auto px-6 py-4">
            {previewHTML ? (
              <div
                className="prose prose-invert max-w-none"
                dangerouslySetInnerHTML={{ __html: previewHTML }}
              />
            ) : content.trim() ? (
              <div className="flex items-center justify-center h-full text-dark-text-tertiary">
                <p className="text-sm">Loading preview...</p>
              </div>
            ) : (
              <div className="flex items-center justify-center h-full text-dark-text-tertiary">
                <div className="text-center">
                  <svg className="w-16 h-16 mx-auto mb-4 text-dark-text-tertiary/50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1} d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                  </svg>
                  <p className="text-lg">No content to preview</p>
                  <p className="text-sm mt-2">Switch to Edit mode to start writing</p>
                </div>
              </div>
            )}
          </div>
        ) : (
          <>
            <div
              className="relative flex-1 min-h-0"
              onDragEnter={handleDragEnter}
              onDragOver={handleDragOver}
              onDragLeave={handleDragLeave}
              onDrop={handleDrop}
            >
              <textarea
                ref={textareaRef}
                value={content}
                onChange={handleTextareaChange}
                onKeyDown={handleKeyDown}
                placeholder="Start writing in Markdown...

# Heading 1
## Heading 2

**bold** *italic* `code`

- List item
- List item

[Link text](https://example.com)

```code block```"
                className="w-full h-full px-6 py-4 bg-dark-bg-primary text-dark-text-primary resize-none focus:outline-none font-mono text-sm placeholder-dark-text-tertiary/50"
                spellCheck={false}
              />
              {(isDragOver || isDropUploading) && (
                <div className="absolute inset-0 bg-primary-500/10 border-2 border-dashed border-primary-500 rounded-lg flex flex-col items-center justify-center gap-2 pointer-events-none z-10">
                  <svg className="w-12 h-12 text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
                  </svg>
                  <span className="text-sm font-semibold text-primary-400">
                    {isDropUploading ? 'Uploading...' : 'Drop images here'}
                  </span>
                </div>
              )}
            </div>
            {/* Visible drop zone */}
            <div
              className={`mx-6 mb-4 mt-2 border-2 border-dashed rounded-lg p-4 flex items-center justify-center gap-3 transition-colors ${
                isDragOver
                  ? 'border-primary-500 bg-primary-500/5'
                  : 'border-dark-border-subtle hover:border-primary-500/50'
              }`}
              onDragEnter={handleDragEnter}
              onDragOver={handleDragOver}
              onDragLeave={handleDragLeave}
              onDrop={handleDrop}
            >
              <svg className="w-8 h-8 text-dark-text-tertiary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
              </svg>
              <div>
                <div className="text-sm text-dark-text-secondary">
                  Drop images here or{' '}
                  <button
                    onClick={() => setShowImagePicker(true)}
                    className="text-primary-400 hover:underline font-medium"
                  >
                    browse
                  </button>
                </div>
                <div className="text-xs text-dark-text-tertiary">Supports JPG, PNG, GIF</div>
              </div>
            </div>
          </>
        )}
      </div>

      {/* Footer helper */}
      {!isPreview && (
        <div className="border-t border-dark-border-subtle bg-dark-bg-secondary px-6 py-2">
          <div className="flex items-center gap-4 text-xs text-dark-text-tertiary">
            <span>Markdown supported</span>
            <span>&bull;</span>
            <span>Changes sync automatically</span>
            <span>&bull;</span>
            <span>Tab to indent</span>
            <span>&bull;</span>
            <span>F11 fullscreen</span>
          </div>
        </div>
      )}

      {/* Image Picker Modal */}
      {showImagePicker && (
        <ImagePickerModal
          onSelect={insertImageMarkdown}
          onClose={() => setShowImagePicker(false)}
          wikiPageId={page.id}
          onUploadComplete={() => {}}
        />
      )}

      {/* Edit Image Modal */}
      {editImgList && (
        <div
          className="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 backdrop-blur-sm"
          onClick={() => { setEditImgList(null); setSelectedEditImg(null) }}
        >
          <div
            className="w-full max-w-2xl mx-4 bg-dark-bg-secondary rounded-xl border border-dark-border-subtle shadow-2xl overflow-hidden"
            onClick={e => e.stopPropagation()}
          >
            {/* Header */}
            <div className="flex items-center justify-between px-6 py-4 border-b border-dark-border-subtle">
              <h3 className="text-lg font-semibold text-dark-text-primary">Edit Image</h3>
              <button
                onClick={() => { setEditImgList(null); setSelectedEditImg(null) }}
                className="text-dark-text-tertiary hover:text-dark-text-primary transition-colors"
              >
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>

            {!selectedEditImg ? (
              /* Image grid picker */
              <div className="p-6">
                <p className="text-sm text-dark-text-secondary mb-4">Select an image to edit:</p>
                <div className="grid grid-cols-3 gap-3 max-h-80 overflow-y-auto">
                  {editImgList.map((img, i) => (
                    <button
                      key={i}
                      onClick={() => selectImgForEdit(img)}
                      className="group relative aspect-square rounded-lg overflow-hidden border-2 border-dark-border-subtle hover:border-primary-500 transition-colors bg-dark-bg-tertiary"
                    >
                      <img
                        src={img.url}
                        alt={img.alt}
                        className="w-full h-full object-cover"
                        onError={(e) => {
                          (e.target as HTMLImageElement).style.display = 'none'
                        }}
                      />
                      <div className="absolute inset-x-0 bottom-0 bg-black/70 px-2 py-1">
                        <p className="text-xs text-dark-text-secondary truncate">{img.alt || 'No alt text'}</p>
                      </div>
                    </button>
                  ))}
                </div>
              </div>
            ) : (
              /* Edit form */
              <div className="p-6">
                <div className="flex gap-4">
                  {/* Preview */}
                  <div className="w-40 h-40 flex-shrink-0 rounded-lg overflow-hidden bg-dark-bg-tertiary border border-dark-border-subtle">
                    <img
                      src={selectedEditImg.url}
                      alt={editAlt}
                      className="w-full h-full object-cover"
                    />
                  </div>
                  {/* Fields */}
                  <div className="flex-1 space-y-4">
                    <div>
                      <label className="block text-xs font-medium text-dark-text-secondary mb-1">Alt text</label>
                      <input
                        type="text"
                        value={editAlt}
                        onChange={e => setEditAlt(e.target.value)}
                        className="w-full px-3 py-2 rounded-lg bg-dark-bg-tertiary border border-dark-border-subtle text-sm text-dark-text-primary focus:outline-none focus:border-primary-500"
                        placeholder="Describe this image..."
                      />
                    </div>
                    <div>
                      <label className="block text-xs font-medium text-dark-text-secondary mb-1">Caption</label>
                      <input
                        type="text"
                        value={editCaption}
                        onChange={e => setEditCaption(e.target.value)}
                        className="w-full px-3 py-2 rounded-lg bg-dark-bg-tertiary border border-dark-border-subtle text-sm text-dark-text-primary focus:outline-none focus:border-primary-500"
                        placeholder="Optional caption..."
                      />
                    </div>
                  </div>
                </div>
                {/* Actions */}
                <div className="flex items-center justify-end gap-3 mt-6">
                  <button
                    onClick={() => setSelectedEditImg(null)}
                    className="px-4 py-2 rounded-lg text-sm font-medium text-dark-text-secondary hover:text-dark-text-primary transition-colors"
                  >
                    Back
                  </button>
                  <button
                    onClick={saveEditImg}
                    className="px-4 py-2 rounded-lg text-sm font-medium bg-primary-600 text-white hover:bg-primary-500 transition-colors"
                  >
                    Save
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
