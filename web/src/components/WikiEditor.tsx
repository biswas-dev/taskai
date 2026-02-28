import { useEffect, useState, useCallback, useRef } from 'react'
import { WikiPage } from '../lib/api'
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

  const insertImageMarkdown = (alt: string, url: string) => {
    const textarea = isFullscreen ? fsTextareaRef.current : textareaRef.current
    const markdown = `![${alt}](${url})`

    if (textarea) {
      const start = textarea.selectionStart
      const end = textarea.selectionEnd
      const newContent = content.substring(0, start) + markdown + content.substring(end)
      setContent(newContent)
      syncToYjs(newContent)
      setTimeout(() => {
        textarea.selectionStart = textarea.selectionEnd = start + markdown.length
        textarea.focus()
      }, 0)
    } else {
      const newContent = content + (content.endsWith('\n') ? '' : '\n') + markdown + '\n'
      setContent(newContent)
      syncToYjs(newContent)
    }
    setShowImagePicker(false)
  }

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
            <div style={{ width: `${fsSplitPct}%` }} className="flex flex-col overflow-hidden">
              <textarea
                ref={fsTextareaRef}
                value={content}
                onChange={handleTextareaChange}
                onKeyDown={handleKeyDown}
                className="flex-1 w-full bg-dark-bg-primary text-dark-text-primary resize-none focus:outline-none font-mono text-sm p-4"
                spellCheck={false}
                placeholder="Start writing in Markdown..."
              />
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
      <div className="flex-1 overflow-hidden">
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
    </div>
  )
}
