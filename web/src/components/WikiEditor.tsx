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
  isDirtyRef?: React.MutableRefObject<boolean>,
) {
  const start = textarea.selectionStart
  const end = textarea.selectionEnd
  const selected = content.substring(start, end)
  const insert = before + (selected || 'text') + after
  const newContent = content.substring(0, start) + insert + content.substring(end)
  setContent(newContent)
  syncToYjs(newContent)
  if (isDirtyRef) isDirtyRef.current = true

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
  isDirtyRef?: React.MutableRefObject<boolean>,
) {
  const start = textarea.selectionStart
  // Find beginning of current line
  const lineStart = content.lastIndexOf('\n', start - 1) + 1
  const insert = prefix
  const newContent = content.substring(0, lineStart) + insert + content.substring(lineStart)
  setContent(newContent)
  syncToYjs(newContent)
  if (isDirtyRef) isDirtyRef.current = true

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

// ── Draw embed init — replicate embed.js behavior for innerHTML previews ──

const drawSizeMap: Record<string, { width: string; height: string }> = {
  s: { width: '50%', height: '300px' },
  m: { width: '100%', height: '520px' },
  l: { width: '100%', height: '720px' },
}

const overlayBarStyle: Record<string, string> = {
  position: 'absolute', top: '8px', right: '8px',
  display: 'flex', alignItems: 'center', gap: '2px',
  padding: '3px', borderRadius: '6px',
  background: 'rgba(0,0,0,0.6)', backdropFilter: 'blur(4px)',
  opacity: '0', transition: 'opacity 0.2s', zIndex: '5',
}

function makeSizeBtn(sz: string, isActive: boolean): HTMLButtonElement {
  const btn = document.createElement('button')
  btn.type = 'button'
  btn.textContent = sz.toUpperCase()
  btn.setAttribute('data-size', sz)
  Object.assign(btn.style, {
    padding: '3px 8px', border: 'none', borderRadius: '4px',
    background: isActive ? 'rgba(99,102,241,0.8)' : 'transparent',
    color: isActive ? '#fff' : 'rgba(255,255,255,0.7)',
    fontSize: '11px', fontWeight: '600', cursor: 'pointer',
    transition: 'background 0.15s, color 0.15s',
  })
  btn.addEventListener('mouseenter', () => { if (!btn.classList.contains('active')) { btn.style.background = 'rgba(255,255,255,0.15)'; btn.style.color = '#fff' } })
  btn.addEventListener('mouseleave', () => { if (!btn.classList.contains('active')) { btn.style.background = 'transparent'; btn.style.color = 'rgba(255,255,255,0.7)' } })
  if (isActive) btn.classList.add('active')
  return btn
}

function updateSizeBtnStates(overlay: HTMLElement, activeSize: string) {
  overlay.querySelectorAll('button[data-size]').forEach((b) => {
    const btn = b as HTMLButtonElement
    const isActive = btn.getAttribute('data-size') === activeSize
    btn.classList.toggle('active', isActive)
    btn.style.background = isActive ? 'rgba(99,102,241,0.8)' : 'transparent'
    btn.style.color = isActive ? '#fff' : 'rgba(255,255,255,0.7)'
  })
}

function escapeRegExp(str: string) { return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') }

function initDrawEmbeds(
  container: HTMLElement | null,
  contentRef?: React.MutableRefObject<string>,
  updateContent?: (newContent: string) => void,
) {
  if (!container) return
  const embeds = container.querySelectorAll('.godraw-embed:not(.godraw-preview-init)')
  embeds.forEach((div) => {
    let src = div.getAttribute('data-src')
    const w = div.getAttribute('data-width') || '100%'
    const h = div.getAttribute('data-height') || '520px'
    const zoom = div.getAttribute('data-zoom')
    if (!src) return

    const drawIdMatch = src.match(/\/([a-zA-Z0-9_-]+?)(?:\/edit)?$/)
    const drawId = drawIdMatch ? drawIdMatch[1] : null

    src = src.replace(/\/edit$/, '')
    if (zoom) {
      src += (src.includes('?') ? '&' : '?') + 'zoom=' + encodeURIComponent(zoom)
    }

    const wrapper = document.createElement('div')
    wrapper.style.position = 'relative'
    wrapper.style.display = 'inline-block'
    wrapper.style.width = w

    const iframe = document.createElement('iframe')
    iframe.src = src
    iframe.style.width = '100%'
    iframe.style.height = h
    iframe.style.border = 'none'
    iframe.style.borderRadius = '8px'
    iframe.setAttribute('loading', 'lazy')
    wrapper.appendChild(iframe)

    if (drawId) {
      // Detect current size from shortcode
      let currentSize = 'm'
      if (contentRef) {
        const scRe = new RegExp('\\[draw:' + escapeRegExp(drawId) + '(?::edit)?(?::([sml]))?')
        const scMatch = scRe.exec(contentRef.current)
        if (scMatch?.[1]) currentSize = scMatch[1]
      }

      const overlay = document.createElement('div')
      Object.assign(overlay.style, overlayBarStyle)

      // Size buttons S/M/L
      for (const sz of ['s', 'm', 'l']) {
        const btn = makeSizeBtn(sz, sz === currentSize)
        btn.addEventListener('click', (e) => {
          e.preventDefault(); e.stopPropagation()
          if (!contentRef || !updateContent) return
          const re = new RegExp('\\[draw:' + escapeRegExp(drawId) + '(?::edit)?(?::[sml])?(?::z[^\\]]+)?\\]')
          const m = re.exec(contentRef.current)
          if (!m) return
          const zoomMatch = m[0].match(/:z([^\]]+)/)
          const zoomTag = zoomMatch ? ':z' + zoomMatch[1] : ''
          const sizeTag = sz === 'm' ? '' : ':' + sz
          const newSC = `[draw:${drawId}:edit${sizeTag}${zoomTag}]`
          const newContent = contentRef.current.substring(0, m.index) + newSC + contentRef.current.substring(m.index + m[0].length)
          updateContent(newContent)
          // Update visual
          const dims = drawSizeMap[sz] || drawSizeMap.m
          wrapper.style.width = dims.width
          iframe.style.height = dims.height
          updateSizeBtnStates(overlay, sz)
        })
        overlay.appendChild(btn)
      }

      // Separator + Edit button
      const sep = document.createElement('span')
      Object.assign(sep.style, { width: '1px', height: '16px', background: 'rgba(255,255,255,0.3)', margin: '0 2px' })
      overlay.appendChild(sep)

      const editBtn = document.createElement('button')
      editBtn.type = 'button'
      editBtn.innerHTML = '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 3a2.828 2.828 0 114 4L7.5 20.5 2 22l1.5-5.5L17 3z"/></svg> Edit'
      Object.assign(editBtn.style, {
        display: 'inline-flex', alignItems: 'center', gap: '3px',
        padding: '3px 8px', border: 'none', borderRadius: '4px',
        background: 'transparent', color: 'rgba(255,255,255,0.7)',
        fontSize: '11px', fontWeight: '500', cursor: 'pointer',
        transition: 'background 0.15s, color 0.15s',
      })
      editBtn.addEventListener('mouseenter', () => { editBtn.style.background = 'rgba(99,102,241,0.6)'; editBtn.style.color = '#fff' })
      editBtn.addEventListener('mouseleave', () => { editBtn.style.background = 'transparent'; editBtn.style.color = 'rgba(255,255,255,0.7)' })
      editBtn.addEventListener('click', (e) => {
        e.preventDefault(); e.stopPropagation()
        window.open(`/draw/${drawId}/edit`, '_blank')
      })
      overlay.appendChild(editBtn)

      wrapper.addEventListener('mouseenter', () => { overlay.style.opacity = '1' })
      wrapper.addEventListener('mouseleave', () => { overlay.style.opacity = '0' })
      wrapper.appendChild(overlay)
    }

    div.innerHTML = ''
    div.appendChild(wrapper)
    div.classList.add('godraw-preview-init')
  })
}

// ── Image edit overlays — add S/M/L size controls to images in preview ──

function buildImageMarkup(url: string, alt: string, caption: string, size: string): string {
  const sizeStyles: Record<string, string> = {
    s: 'max-width:50%;height:auto;',
    m: 'max-width:75%;height:auto;',
    l: 'width:100%;height:auto;max-width:100%;',
  }
  const imgStyle = sizeStyles[size] || sizeStyles.m
  const escHtml = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

  if (size === 'l' && !caption) {
    return `![${alt}](${url})`
  }
  return `<figure style="text-align:center;margin:1.5rem 0"><a href="${url}" data-lightbox="article-images" data-title="${escHtml(alt)}"><img src="${url}" alt="${escHtml(alt)}" style="${imgStyle}"/></a>${caption ? `<figcaption>${escHtml(caption)}</figcaption>` : ''}</figure>`
}

function addImageEditOverlays(
  container: HTMLElement | null,
  contentRef: React.MutableRefObject<string>,
  updateContent: (newContent: string) => void,
) {
  if (!container) return
  const images = container.querySelectorAll('img:not(.gw-preview-img-init)')
  images.forEach((imgEl) => {
    const img = imgEl as HTMLImageElement
    // Skip images inside draw embeds
    if (img.closest('.godraw-embed') || img.closest('[style*="inline-block"]')) return
    const imgUrl = img.getAttribute('src')
    if (!imgUrl) return

    // Detect current size
    let currentSize = 'l'
    const styleStr = img.getAttribute('style') || ''
    if (/max-width:\s*50%/.test(styleStr)) currentSize = 's'
    else if (/max-width:\s*75%/.test(styleStr)) currentSize = 'm'

    // Wrap the image (or its <figure> parent)
    const wrapTarget = (img.closest('figure') || img) as HTMLElement
    const wrapper = document.createElement('div')
    wrapper.style.position = 'relative'
    wrapper.style.display = 'inline-block'
    wrapTarget.parentNode?.insertBefore(wrapper, wrapTarget)
    wrapper.appendChild(wrapTarget)

    const overlay = document.createElement('div')
    Object.assign(overlay.style, overlayBarStyle)

    for (const sz of ['s', 'm', 'l']) {
      const btn = makeSizeBtn(sz, sz === currentSize)
      btn.addEventListener('click', (e) => {
        e.preventDefault(); e.stopPropagation()
        const content = contentRef.current

        // Try <figure> containing this URL
        const figRe = new RegExp('<figure[^>]*>[\\s\\S]*?' + escapeRegExp(imgUrl) + '[\\s\\S]*?<\\/figure>')
        const figMatch = figRe.exec(content)
        if (figMatch) {
          const altM = figMatch[0].match(/alt="([^"]*)"/)
          const capM = figMatch[0].match(/<figcaption>([\s\S]*?)<\/figcaption>/)
          const alt = (altM?.[1] || '').replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"')
          const cap = (capM?.[1] || '').replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"')
          updateContent(content.replace(figMatch[0], buildImageMarkup(imgUrl, alt, cap, sz)))
        } else {
          // Try markdown ![alt](url)
          const mdRe = new RegExp('!\\[([^\\]]*)\\]\\(' + escapeRegExp(imgUrl) + '\\)')
          const mdMatch = mdRe.exec(content)
          if (mdMatch) {
            updateContent(content.replace(mdMatch[0], buildImageMarkup(imgUrl, mdMatch[1], '', sz)))
          }
        }

        // Update visual
        const sizeStyles: Record<string, string> = {
          s: 'max-width:50%;height:auto;',
          m: 'max-width:75%;height:auto;',
          l: 'width:100%;height:auto;max-width:100%;',
        }
        img.setAttribute('style', sizeStyles[sz] || sizeStyles.m)
        updateSizeBtnStates(overlay, sz)
      })
      overlay.appendChild(btn)
    }

    wrapper.appendChild(overlay)
    wrapper.addEventListener('mouseenter', () => { overlay.style.opacity = '1' })
    wrapper.addEventListener('mouseleave', () => { overlay.style.opacity = '0' })
    img.classList.add('gw-preview-img-init')
  })
}

// ── Draw browser types ──────────────────────────────────────────

interface DrawItem {
  id: string
  title: string
  updated_at: string
}

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
  const [isPreview, setIsPreview] = useState(true)
  const [previewHTML, setPreviewHTML] = useState('')
  const [syncState, setSyncState] = useState<'connecting' | 'connected' | 'disconnected'>('connecting')
  const [showImagePicker, setShowImagePicker] = useState(false)
  const [saveStatus, setSaveStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const [autoSaveEnabled, setAutoSaveEnabled] = useState(true)
  const isDirtyRef = useRef(false)
  const contentRef = useRef('')
  const lastSavedContentRef = useRef('')
  const isSavingRef = useRef(false)
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
  const previewRef = useRef<HTMLDivElement>(null)
  const fsPreviewRef = useRef<HTMLDivElement>(null)
  const [fsSplitPct, setFsSplitPct] = useState(50)
  const [isDragOver, setIsDragOver] = useState(false)
  const [isDropUploading, setIsDropUploading] = useState(false)
  const dragCounterRef = useRef(0)
  const [editImgList, setEditImgList] = useState<Array<{ html: string; url: string; alt: string; caption: string; index: number }> | null>(null)
  const [selectedEditImg, setSelectedEditImg] = useState<{ html: string; url: string; alt: string; caption: string; index: number } | null>(null)
  const [editAlt, setEditAlt] = useState('')
  const [editCaption, setEditCaption] = useState('')
  const [editSize, setEditSize] = useState('m')
  const [showDrawBrowser, setShowDrawBrowser] = useState(false)
  const [drawList, setDrawList] = useState<DrawItem[]>([])
  const [drawLoading, setDrawLoading] = useState(false)
  const [editDrawList, setEditDrawList] = useState<Array<{ shortcode: string; id: string; size: string; zoom: string; index: number }> | null>(null)
  const [selectedEditDraw, setSelectedEditDraw] = useState<{ shortcode: string; id: string; size: string; zoom: string; index: number } | null>(null)
  const [editDrawSize, setEditDrawSize] = useState('m')
  const [editDrawZoom, setEditDrawZoom] = useState('fit')

  // ── Keep contentRef in sync ──────────────────────────────────
  useEffect(() => { contentRef.current = content }, [content])

  // ── Load content from REST on mount ────────────────────────
  useEffect(() => {
    let cancelled = false
    apiClient.getWikiPageContent(page.id).then(res => {
      if (cancelled) return
      if (res.content) {
        setContent(res.content)
        lastSavedContentRef.current = res.content
      }
    }).catch(() => {
      // ignore — will fall back to Yjs or empty
    })
    return () => { cancelled = true }
  }, [page.id])

  // ── Manual save function ─────────────────────────────────────
  const saveNow = useCallback(async () => {
    const current = contentRef.current
    if (isSavingRef.current || current === lastSavedContentRef.current) return

    isSavingRef.current = true
    setSaveStatus('saving')
    try {
      await apiClient.updateWikiPageContent(page.id, current)
      lastSavedContentRef.current = current
      isDirtyRef.current = false
      setSaveStatus('saved')
      setTimeout(() => setSaveStatus(prev => prev === 'saved' ? 'idle' : prev), 3000)
    } catch {
      setSaveStatus('error')
    } finally {
      isSavingRef.current = false
    }
  }, [page.id])

  // ── Autosave interval (10 seconds) ─────────────────────────
  useEffect(() => {
    if (!autoSaveEnabled) return

    const saveToServer = async () => {
      const current = contentRef.current
      if (!isDirtyRef.current || current === lastSavedContentRef.current) return
      await saveNow()
    }

    const interval = setInterval(saveToServer, 10_000)

    // Save on unmount
    return () => {
      clearInterval(interval)
      const current = contentRef.current
      if (isDirtyRef.current && current !== lastSavedContentRef.current) {
        apiClient.updateWikiPageContent(page.id, current).catch(() => {})
      }
    }
  }, [page.id, autoSaveEnabled, saveNow])

  // ── Save on beforeunload ───────────────────────────────────
  useEffect(() => {
    const handler = () => {
      const current = contentRef.current
      if (isDirtyRef.current && current !== lastSavedContentRef.current) {
        const blob = new Blob([JSON.stringify({ content: current })], { type: 'application/json' })
        const token = localStorage.getItem('auth_token')
        const API_BASE_FOR_SAVE = import.meta.env.VITE_API_URL || (import.meta.env.PROD ? '' : 'http://localhost:8080')
        navigator.sendBeacon(
          `${API_BASE_FOR_SAVE}/api/wiki/pages/${page.id}/content`,
          blob
        )
        // Note: sendBeacon doesn't support custom headers (no auth), so this is best-effort.
        // The interval save is the primary mechanism.
        void token // suppress unused warning
      }
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [page.id])

  // ── Yjs setup ────────────────────────────────────────────────

  useEffect(() => {
    const ydoc = new Y.Doc()
    ydocRef.current = ydoc
    const ytext = ydoc.getText('content')

    const token = localStorage.getItem('token')
    if (!token) {
      // no auth token — cannot connect WebSocket
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
    isDirtyRef.current = true
  }

  // ── Toolbar action dispatcher ────────────────────────────────

  const execToolbarAction = useCallback((action: ToolbarAction) => {
    const textarea = isFullscreen ? fsTextareaRef.current : textareaRef.current
    if (!textarea) return

    if (action.action === 'wrap') {
      insertAtCursor(textarea, action.before!, action.after!, content, setContent, syncToYjs, isDirtyRef)
    } else if (action.action === 'line') {
      insertLine(textarea, action.prefix!, content, setContent, syncToYjs, isDirtyRef)
    } else if (action.action === 'insert') {
      const start = textarea.selectionStart
      const newContent = content.substring(0, start) + action.text! + content.substring(start)
      setContent(newContent)
      syncToYjs(newContent)
      isDirtyRef.current = true
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

  // ── Init draw embeds in previews ──────────────────────────────

  const previewUpdateContent = useCallback((newContent: string) => {
    setContent(newContent)
    syncToYjs(newContent)
    isDirtyRef.current = true
  }, [syncToYjs])

  useEffect(() => {
    if (isPreview && previewHTML) {
      const t = setTimeout(() => {
        initDrawEmbeds(previewRef.current, contentRef, previewUpdateContent)
        addImageEditOverlays(previewRef.current, contentRef, previewUpdateContent)
      }, 50)
      return () => clearTimeout(t)
    }
  }, [isPreview, previewHTML, previewUpdateContent])

  useEffect(() => {
    if (isFullscreen && fsPreviewHTML) {
      const t = setTimeout(() => {
        initDrawEmbeds(fsPreviewRef.current, contentRef, previewUpdateContent)
        addImageEditOverlays(fsPreviewRef.current, contentRef, previewUpdateContent)
      }, 50)
      return () => clearTimeout(t)
    }
  }, [isFullscreen, fsPreviewHTML, previewUpdateContent])

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
      isDirtyRef.current = true
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

  // ── Double-click draw shortcode → open editor ─────────────

  const handleDoubleClick = useCallback((e: React.MouseEvent<HTMLTextAreaElement>) => {
    const textarea = e.currentTarget
    const pos = textarea.selectionStart
    const val = textarea.value
    const re = /\[draw:([a-zA-Z0-9_-]+)(?::edit)?\]/g
    let match
    while ((match = re.exec(val)) !== null) {
      if (pos >= match.index && pos <= match.index + match[0].length) {
        window.open(`/draw/${match[1]}/edit`, '_blank')
        return
      }
    }
  }, [])

  // ── Edit existing draw shortcode ─────────────────────────────

  const handleEditDraw = useCallback(() => {
    const draws: Array<{ shortcode: string; id: string; size: string; zoom: string; index: number }> = []
    const re = /\[draw:([a-zA-Z0-9_-]+)(?::edit)?(?::([sml]))?(?::z([^\]]+))?\]/g
    let match
    while ((match = re.exec(content)) !== null) {
      draws.push({
        shortcode: match[0],
        id: match[1],
        size: match[2] || 'm',
        zoom: match[3] || 'fit',
        index: match.index,
      })
    }
    if (draws.length === 0) {
      alert('No draw shortcodes found in content')
      return
    }
    draws.sort((a, b) => a.index - b.index)
    setEditDrawList(draws)
    setSelectedEditDraw(null)
  }, [content])

  const selectDrawForEdit = (draw: typeof editDrawList extends Array<infer T> | null ? T : never) => {
    setSelectedEditDraw(draw)
    setEditDrawSize(draw.size)
    setEditDrawZoom(draw.zoom)
  }

  const saveEditDraw = useCallback(() => {
    if (!selectedEditDraw) return
    const sizeTag = editDrawSize === 'm' ? '' : ':' + editDrawSize
    const zoomTag = editDrawZoom === 'fit' ? '' : ':z' + editDrawZoom
    const newShortcode = `[draw:${selectedEditDraw.id}:edit${sizeTag}${zoomTag}]`
    const newContent = content.replace(selectedEditDraw.shortcode, newShortcode)
    setContent(newContent)
    syncToYjs(newContent)
    isDirtyRef.current = true
    setEditDrawList(null)
    setSelectedEditDraw(null)
  }, [selectedEditDraw, editDrawSize, editDrawZoom, content, syncToYjs])

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

  const insertImageMarkdown = (alt: string, url: string, caption?: string, size?: string) => {
    const textarea = isFullscreen ? fsTextareaRef.current : textareaRef.current
    const escHtml = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
    const sz = size || 'm'
    const sizeStyles: Record<string, string> = {
      s: 'max-width:50%;height:auto;',
      m: 'max-width:75%;height:auto;',
      l: 'width:100%;height:auto;max-width:100%;',
    }
    const imgStyle = sizeStyles[sz] || sizeStyles.m
    let markup: string
    if (sz === 'l' && !caption) {
      markup = `![${alt}](${url})`
    } else {
      markup = `<figure style="text-align:center;margin:1.5rem 0"><a href="${url}" data-lightbox="article-images" data-title="${escHtml(alt)}"><img src="${url}" alt="${escHtml(alt)}" style="${imgStyle}"/></a>${caption ? `<figcaption>${escHtml(caption)}</figcaption>` : ''}</figure>\n`
    }

    if (textarea) {
      const start = textarea.selectionStart
      const end = textarea.selectionEnd
      const newContent = content.substring(0, start) + markup + content.substring(end)
      setContent(newContent)
      syncToYjs(newContent)
      isDirtyRef.current = true
      setTimeout(() => {
        textarea.selectionStart = textarea.selectionEnd = start + markup.length
        textarea.focus()
      }, 0)
    } else {
      const newContent = content + (content.endsWith('\n') ? '' : '\n') + markup + '\n'
      setContent(newContent)
      syncToYjs(newContent)
      isDirtyRef.current = true
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
        isDirtyRef.current = true
      }
    } catch (err) {
      // drop upload failed — error state handled by UI
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
    // Detect size from existing HTML
    if (img.html.startsWith('<figure')) {
      if (/max-width:\s*50%/.test(img.html)) setEditSize('s')
      else if (/max-width:\s*75%/.test(img.html)) setEditSize('m')
      else setEditSize('l')
    } else {
      setEditSize('l') // plain markdown = large
    }
  }

  const saveEditImg = useCallback(() => {
    if (!selectedEditImg) return

    const escHtml = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
    const sizeStyles: Record<string, string> = {
      s: 'max-width:50%;height:auto;',
      m: 'max-width:75%;height:auto;',
      l: 'width:100%;height:auto;max-width:100%;',
    }
    const imgStyle = sizeStyles[editSize] || sizeStyles.m
    let newMarkup: string

    if (editSize === 'l' && !editCaption) {
      // Large + no caption → plain markdown
      newMarkup = `![${editAlt}](${selectedEditImg.url})`
    } else {
      // Use <figure>
      newMarkup = `<figure style="text-align:center;margin:1.5rem 0"><a href="${selectedEditImg.url}" data-lightbox="article-images" data-title="${escHtml(editAlt)}"><img src="${selectedEditImg.url}" alt="${escHtml(editAlt)}" style="${imgStyle}"/></a>${editCaption ? `<figcaption>${escHtml(editCaption)}</figcaption>` : ''}</figure>`
    }

    const newContent = content.replace(selectedEditImg.html, newMarkup)
    setContent(newContent)
    syncToYjs(newContent)
    isDirtyRef.current = true
    setEditImgList(null)
    setSelectedEditImg(null)
  }, [selectedEditImg, editAlt, editCaption, editSize, content, syncToYjs])

  // ── Draw browser handlers ────────────────────────────────────

  const loadDrawings = useCallback(async () => {
    setDrawLoading(true)
    try {
      const res = await fetch('/draw/api/list')
      const data = await res.json()
      setDrawList(data.drawings || [])
    } catch {
      setDrawList([])
    } finally {
      setDrawLoading(false)
    }
  }, [])

  const handleDraw = useCallback(() => {
    setShowDrawBrowser(true)
    loadDrawings()
  }, [loadDrawings])

  const handleDrawInsert = useCallback((id: string, size: string, zoom: string) => {
    const textarea = isFullscreen ? fsTextareaRef.current : textareaRef.current
    const sizeTag = size === 'm' ? '' : ':' + size
    const zoomTag = zoom === 'fit' ? '' : ':z' + zoom
    const markup = `\n[draw:${id}:edit${sizeTag}${zoomTag}]\n`
    if (textarea) {
      const start = textarea.selectionStart
      const end = textarea.selectionEnd
      const newContent = content.substring(0, start) + markup + content.substring(end)
      setContent(newContent)
      syncToYjs(newContent)
      isDirtyRef.current = true
      setTimeout(() => {
        textarea.focus()
        textarea.selectionStart = textarea.selectionEnd = start + markup.length
      }, 0)
    } else {
      const newContent = content + markup
      setContent(newContent)
      syncToYjs(newContent)
      isDirtyRef.current = true
    }
    setShowDrawBrowser(false)
  }, [isFullscreen, content, syncToYjs])

  const handleDrawRename = useCallback(async (id: string, title: string) => {
    try {
      await fetch(`/draw/api/${id}/rename`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title }),
      })
      setDrawList(prev => prev.map(d => d.id === id ? { ...d, title } : d))
    } catch {
      // ignore
    }
  }, [])

  const handleDrawDelete = useCallback(async (id: string) => {
    try {
      await fetch(`/draw/api/${id}/delete`, { method: 'POST' })
      setDrawList(prev => prev.filter(d => d.id !== id))
    } catch {
      alert('Failed to delete drawing')
    }
  }, [])

  const handleDrawNew = useCallback(async () => {
    try {
      const res = await fetch('/draw/api/new', { method: 'POST' })
      const data = await res.json()
      if (data && data.id) {
        const editUrl = data.edit_url || `/draw/${data.id}/edit`
        window.open(editUrl, '_blank')
        loadDrawings()
      }
    } catch (err) {
      // drawing creation failed — silently swallowed
    }
  }, [loadDrawings])

  const handleDrawDeleteUnused = useCallback(async (unusedIds: string[]) => {
    try {
      await Promise.all(unusedIds.map(id => fetch(`/draw/api/${id}/delete`, { method: 'POST' })))
      loadDrawings()
    } catch {
      alert('Some deletions failed')
      loadDrawings()
    }
  }, [loadDrawings])

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
        className="px-2 py-1 rounded text-xs font-medium transition-colors bg-dark-bg-tertiary text-dark-text-secondary hover:bg-dark-bg-tertiary/80 hover:text-dark-text-primary flex items-center gap-1"
        title="Browse drawings"
      >
        <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
          <path d="M12 19l7-7 3 3-7 7-3-3z" />
          <path d="M18 13l-1.5-7.5L2 2l3.5 14.5L13 18l5-5z" />
          <path d="M2 2l7.586 7.586" />
          <circle cx="11" cy="11" r="2" />
        </svg>
        Draw
      </button>
      <button
        onClick={handleEditDraw}
        className="px-2 py-1 rounded text-xs font-medium transition-colors bg-dark-bg-tertiary text-dark-text-secondary hover:bg-dark-bg-tertiary/80 hover:text-dark-text-primary flex items-center gap-1"
        title="Edit existing draw shortcode"
      >
        <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24">
          <path d="M12 19l7-7 3 3-7 7-3-3z" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
          <path d="M18 13l-1.5-7.5L2 2l3.5 14.5L13 18l5-5z" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
          <path d="M15.5 6.5l2 2" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" />
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
                <div className={`w-2 h-2 rounded-full ${
                  saveStatus === 'saving' ? 'bg-yellow-500' :
                  saveStatus === 'saved' ? 'bg-green-500' :
                  saveStatus === 'error' ? 'bg-red-500' :
                  getSyncStatusColor()
                }`} />
                <span className="text-xs text-dark-text-tertiary">
                  {saveStatus === 'saving' ? 'Saving...' :
                   saveStatus === 'saved' ? 'Saved' :
                   saveStatus === 'error' ? 'Save failed' :
                   autoSaveEnabled ? 'Autosave on' : 'Autosave off'}
                </span>
              </div>
              <button
                onClick={() => setAutoSaveEnabled(prev => !prev)}
                className={`px-2 py-1 rounded text-xs font-medium transition-colors ${
                  autoSaveEnabled
                    ? 'bg-green-500/15 text-green-400 hover:bg-green-500/25'
                    : 'bg-dark-bg-tertiary text-dark-text-tertiary hover:bg-dark-bg-tertiary/80'
                }`}
                title={autoSaveEnabled ? 'Disable autosave' : 'Enable autosave'}
              >
                {autoSaveEnabled ? 'Auto' : 'Manual'}
              </button>
              <button
                onClick={saveNow}
                disabled={saveStatus === 'saving'}
                className="px-2.5 py-1 rounded text-xs font-medium transition-colors bg-primary-600 text-white hover:bg-primary-500 disabled:opacity-50"
                title="Save now"
              >
                Save
              </button>
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
                onDoubleClick={handleDoubleClick}
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
                  ref={fsPreviewRef}
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

        {/* Draw browser in fullscreen */}
        {showDrawBrowser && (
          <DrawBrowserModal
            drawings={drawList}
            loading={drawLoading}
            editorContent={content}
            onInsert={handleDrawInsert}
            onRename={handleDrawRename}
            onDelete={handleDrawDelete}
            onDeleteUnused={handleDrawDeleteUnused}
            onNew={handleDrawNew}
            onClose={() => setShowDrawBrowser(false)}
          />
        )}

        {/* Edit Draw modal in fullscreen */}
        {editDrawList && (
          <EditDrawModal
            draws={editDrawList}
            selectedDraw={selectedEditDraw}
            editSize={editDrawSize}
            editZoom={editDrawZoom}
            onSelect={selectDrawForEdit}
            onSizeChange={setEditDrawSize}
            onZoomChange={setEditDrawZoom}
            onSave={saveEditDraw}
            onClose={() => { setEditDrawList(null); setSelectedEditDraw(null) }}
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
                <div className={`w-2 h-2 rounded-full ${
                  saveStatus === 'saving' ? 'bg-yellow-500' :
                  saveStatus === 'saved' ? 'bg-green-500' :
                  saveStatus === 'error' ? 'bg-red-500' :
                  getSyncStatusColor()
                }`} />
                <span className="text-sm text-dark-text-tertiary">
                  {saveStatus === 'saving' ? 'Saving...' :
                   saveStatus === 'saved' ? 'Saved' :
                   saveStatus === 'error' ? 'Save failed' :
                   autoSaveEnabled ? 'Autosave on' : 'Autosave off'}
                </span>
              </div>
              <button
                onClick={() => setAutoSaveEnabled(prev => !prev)}
                className={`px-2 py-0.5 rounded text-xs font-medium transition-colors ${
                  autoSaveEnabled
                    ? 'bg-green-500/15 text-green-400 hover:bg-green-500/25'
                    : 'bg-dark-bg-tertiary text-dark-text-tertiary hover:bg-dark-bg-tertiary/80'
                }`}
                title={autoSaveEnabled ? 'Disable autosave' : 'Enable autosave'}
              >
                {autoSaveEnabled ? 'Auto' : 'Manual'}
              </button>
              <button
                onClick={saveNow}
                disabled={saveStatus === 'saving'}
                className="px-2.5 py-0.5 rounded text-xs font-medium transition-colors bg-primary-600 text-white hover:bg-primary-500 disabled:opacity-50"
                title="Save now"
              >
                Save
              </button>
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
                ref={previewRef}
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
                onDoubleClick={handleDoubleClick}
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
            <span className={
              saveStatus === 'saving' ? 'text-yellow-400' :
              saveStatus === 'saved' ? 'text-green-400' :
              saveStatus === 'error' ? 'text-red-400' :
              ''
            }>
              {saveStatus === 'saving' ? 'Saving...' :
               saveStatus === 'saved' ? 'Saved' :
               saveStatus === 'error' ? 'Save failed' :
               autoSaveEnabled ? 'Autosave on' : 'Autosave off'}
            </span>
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
                    <div>
                      <label className="block text-xs font-medium text-dark-text-secondary mb-1">Size</label>
                      <div className="flex gap-1.5">
                        {([['s', 'Small'], ['m', 'Medium'], ['l', 'Large']] as const).map(([val, label]) => (
                          <button
                            key={val}
                            type="button"
                            onClick={() => setEditSize(val)}
                            className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
                              editSize === val
                                ? 'bg-primary-500 text-white'
                                : 'bg-dark-bg-tertiary text-dark-text-secondary hover:bg-dark-bg-tertiary/80'
                            }`}
                          >
                            {label}
                          </button>
                        ))}
                      </div>
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

      {/* Draw Browser Modal */}
      {showDrawBrowser && (
        <DrawBrowserModal
          drawings={drawList}
          loading={drawLoading}
          editorContent={content}
          onInsert={handleDrawInsert}
          onRename={handleDrawRename}
          onDelete={handleDrawDelete}
          onDeleteUnused={handleDrawDeleteUnused}
          onNew={handleDrawNew}
          onClose={() => setShowDrawBrowser(false)}
        />
      )}

      {/* Edit Draw Modal */}
      {editDrawList && (
        <EditDrawModal
          draws={editDrawList}
          selectedDraw={selectedEditDraw}
          editSize={editDrawSize}
          editZoom={editDrawZoom}
          onSelect={selectDrawForEdit}
          onSizeChange={setEditDrawSize}
          onZoomChange={setEditDrawZoom}
          onSave={saveEditDraw}
          onClose={() => { setEditDrawList(null); setSelectedEditDraw(null) }}
        />
      )}
    </div>
  )
}

// ── DrawCard sub-component ───────────────────────────────────────

function DrawCard({ drawing, isUsed, onInsert, onRename, onDelete }: {
  drawing: DrawItem
  isUsed: boolean
  onInsert: (id: string, size: string, zoom: string) => void
  onRename: (id: string, title: string) => void
  onDelete: (id: string) => void
}) {
  const [editing, setEditing] = useState(false)
  const [title, setTitle] = useState(drawing.title || 'Untitled')
  const [size, setSize] = useState('m')
  const [zoom, setZoom] = useState('fit')
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (editing && inputRef.current) inputRef.current.focus()
  }, [editing])

  const saveTitle = () => {
    const trimmed = title.trim()
    if (trimmed && trimmed !== drawing.title) {
      onRename(drawing.id, trimmed)
    } else {
      setTitle(drawing.title || 'Untitled')
    }
    setEditing(false)
  }

  const formattedDate = (() => {
    try {
      return new Date(drawing.updated_at).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })
    } catch { return drawing.updated_at }
  })()

  const handleCardClick = (e: React.MouseEvent) => {
    const target = e.target as HTMLElement
    const tag = target.tagName
    if (tag === 'SELECT' || tag === 'OPTION' || tag === 'BUTTON' || tag === 'INPUT' || tag === 'SVG' || tag === 'PATH' || tag === 'POLYLINE') return
    if (target.closest('select') || target.closest('button') || target.closest('input')) return
    onInsert(drawing.id, size, zoom)
  }

  return (
    <div
      onClick={handleCardClick}
      className={`border rounded-xl p-4 flex flex-col gap-1.5 cursor-pointer transition-colors hover:bg-dark-bg-tertiary/50 ${isUsed ? 'border-green-500/30 hover:border-green-500/50' : 'border-dark-border-subtle hover:border-primary-500/50'}`}
    >
      <div className="flex items-center gap-1.5">
        {editing ? (
          <input
            ref={inputRef}
            value={title}
            onChange={e => setTitle(e.target.value)}
            onBlur={saveTitle}
            onKeyDown={e => {
              if (e.key === 'Enter') saveTitle()
              if (e.key === 'Escape') { setTitle(drawing.title || 'Untitled'); setEditing(false) }
            }}
            className="text-sm font-semibold text-dark-text-primary bg-dark-bg-tertiary border border-primary-500 rounded px-2 py-1 focus:outline-none flex-1 min-w-0"
          />
        ) : (
          <button
            onClick={(e) => { e.stopPropagation(); setEditing(true) }}
            className="text-sm font-semibold text-dark-text-primary text-left truncate hover:text-primary-400 transition-colors flex-1 min-w-0"
            title="Click to rename"
          >
            {drawing.title || 'Untitled'}
          </button>
        )}
        {isUsed && (
          <span className="text-[10px] font-semibold text-green-400 bg-green-500/10 px-1.5 py-0.5 rounded flex-shrink-0">in use</span>
        )}
      </div>
      <span className="text-xs text-dark-text-tertiary">{formattedDate}</span>
      <div className="flex gap-1.5 mt-1 items-center">
        <select
          value={size}
          onChange={e => setSize(e.target.value)}
          onClick={e => e.stopPropagation()}
          className="px-1.5 py-0.5 rounded border border-dark-border-subtle bg-dark-bg-tertiary text-[11px] font-medium text-dark-text-secondary focus:outline-none focus:border-primary-500"
          title="Size"
        >
          <option value="s">S</option>
          <option value="m">M</option>
          <option value="l">L</option>
        </select>
        <select
          value={zoom}
          onChange={e => setZoom(e.target.value)}
          onClick={e => e.stopPropagation()}
          className="px-1.5 py-0.5 rounded border border-dark-border-subtle bg-dark-bg-tertiary text-[11px] font-medium text-dark-text-secondary focus:outline-none focus:border-primary-500"
          title="Zoom"
        >
          <option value="fit">fit</option>
          <option value="50%">50%</option>
          <option value="100%">100%</option>
          <option value="150%">150%</option>
          <option value="200%">200%</option>
        </select>
        <button
          onClick={(e) => { e.stopPropagation(); window.open(`/draw/${drawing.id}/edit`, '_blank') }}
          className="w-[26px] h-[26px] rounded flex items-center justify-center bg-dark-bg-tertiary text-dark-text-secondary hover:bg-dark-bg-tertiary/80 hover:text-dark-text-primary transition-colors"
          title="Edit drawing"
        >
          <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
            <path d="M17 3a2.828 2.828 0 114 4L7.5 20.5 2 22l1.5-5.5L17 3z"/>
          </svg>
        </button>
        <button
          onClick={(e) => { e.stopPropagation(); onDelete(drawing.id) }}
          className="w-[26px] h-[26px] rounded flex items-center justify-center bg-red-500/15 text-red-400 hover:bg-red-500/25 transition-colors"
          title="Delete drawing"
        >
          <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
            <polyline points="3 6 5 6 21 6"/>
            <path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2"/>
          </svg>
        </button>
      </div>
    </div>
  )
}

// ── DrawBrowserModal ─────────────────────────────────────────────

function DrawBrowserModal({ drawings, loading, editorContent, onInsert, onRename, onDelete, onDeleteUnused, onNew, onClose }: {
  drawings: DrawItem[]
  loading: boolean
  editorContent: string
  onInsert: (id: string, size: string, zoom: string) => void
  onRename: (id: string, title: string) => void
  onDelete: (id: string) => void
  onDeleteUnused: (ids: string[]) => void
  onNew: () => void
  onClose: () => void
}) {
  const [confirmAction, setConfirmAction] = useState<{ message: string; onConfirm: () => void } | null>(null)

  const usedIds = new Set<string>()
  const re = /\[draw:([a-zA-Z0-9_-]+)/g
  let m
  while ((m = re.exec(editorContent)) !== null) usedIds.add(m[1])
  const unusedDrawings = drawings.filter(d => !usedIds.has(d.id))

  const handleDelete = (id: string) => {
    const title = drawings.find(d => d.id === id)?.title || 'Untitled'
    setConfirmAction({
      message: `Delete "${title}"?`,
      onConfirm: () => { onDelete(id); setConfirmAction(null) },
    })
  }

  const handleDeleteUnused = () => {
    const count = unusedDrawings.length
    setConfirmAction({
      message: `Delete ${count} unused drawing${count > 1 ? 's' : ''}? This cannot be undone.`,
      onConfirm: () => { onDeleteUnused(unusedDrawings.map(d => d.id)); setConfirmAction(null) },
    })
  }

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        className="relative w-full max-w-2xl mx-4 bg-dark-bg-secondary rounded-xl border border-dark-border-subtle shadow-2xl overflow-hidden flex flex-col max-h-[80vh]"
        onClick={e => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-dark-border-subtle">
          <h3 className="text-lg font-semibold text-dark-text-primary">Drawings</h3>
          <div className="flex items-center gap-2">
            <button
              onClick={onNew}
              className="px-3 py-1.5 rounded-lg text-xs font-semibold bg-primary-600 text-white hover:bg-primary-500 transition-colors"
            >
              + New Drawing
            </button>
            <button
              onClick={onClose}
              className="text-dark-text-tertiary hover:text-dark-text-primary transition-colors"
            >
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>

        {/* Body */}
        <div className="overflow-y-auto p-6">
          {loading ? (
            <p className="text-center text-sm text-dark-text-tertiary py-8">Loading drawings...</p>
          ) : drawings.length === 0 ? (
            <p className="text-center text-sm text-dark-text-tertiary py-8">No drawings yet. Create one above!</p>
          ) : (
            <>
              {unusedDrawings.length > 0 && (
                <div className="flex items-center justify-between gap-2 mb-3 px-3 py-2.5 rounded-lg bg-red-500/10 border border-red-500/20">
                  <span className="text-xs text-red-300">{unusedDrawings.length} unused drawing{unusedDrawings.length > 1 ? 's' : ''}</span>
                  <button
                    onClick={handleDeleteUnused}
                    className="px-2.5 py-1 rounded text-xs font-medium bg-red-500/15 text-red-400 hover:bg-red-500/25 transition-colors"
                  >
                    Delete all unused
                  </button>
                </div>
              )}
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
                {drawings.map(d => (
                  <DrawCard
                    key={d.id}
                    drawing={d}
                    isUsed={usedIds.has(d.id)}
                    onInsert={onInsert}
                    onRename={onRename}
                    onDelete={handleDelete}
                  />
                ))}
              </div>
            </>
          )}
        </div>

        {/* Confirmation dialog overlay */}
        {confirmAction && (
          <div
            className="absolute inset-0 z-10 flex items-center justify-center bg-black/45 backdrop-blur-sm rounded-xl"
            onClick={() => setConfirmAction(null)}
          >
            <div
              className="bg-dark-bg-secondary rounded-xl border border-dark-border-subtle shadow-2xl p-6 max-w-[340px] text-center"
              onClick={e => e.stopPropagation()}
            >
              <p className="text-sm font-medium text-dark-text-primary mb-4">{confirmAction.message}</p>
              <div className="flex gap-2 justify-center">
                <button
                  onClick={() => setConfirmAction(null)}
                  className="px-3 py-1.5 rounded-lg text-xs font-medium border border-dark-border-subtle text-dark-text-secondary hover:bg-dark-bg-tertiary transition-colors"
                >
                  Cancel
                </button>
                <button
                  onClick={confirmAction.onConfirm}
                  className="px-3 py-1.5 rounded-lg text-xs font-semibold bg-red-600 text-white hover:bg-red-500 transition-colors"
                >
                  Delete
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

// ── EditDrawModal ────────────────────────────────────────────────

function EditDrawModal({ draws, selectedDraw, editSize, editZoom, onSelect, onSizeChange, onZoomChange, onSave, onClose }: {
  draws: Array<{ shortcode: string; id: string; size: string; zoom: string; index: number }>
  selectedDraw: { shortcode: string; id: string; size: string; zoom: string; index: number } | null
  editSize: string
  editZoom: string
  onSelect: (draw: { shortcode: string; id: string; size: string; zoom: string; index: number }) => void
  onSizeChange: (size: string) => void
  onZoomChange: (zoom: string) => void
  onSave: () => void
  onClose: () => void
}) {
  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        className="w-full max-w-lg mx-4 bg-dark-bg-secondary rounded-xl border border-dark-border-subtle shadow-2xl overflow-hidden"
        onClick={e => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-dark-border-subtle">
          <h3 className="text-lg font-semibold text-dark-text-primary">Edit Draw Shortcode</h3>
          <button
            onClick={onClose}
            className="text-dark-text-tertiary hover:text-dark-text-primary transition-colors"
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {!selectedDraw ? (
          /* Draw shortcode list */
          <div className="p-6">
            <p className="text-sm text-dark-text-secondary mb-4">Select a draw shortcode to edit:</p>
            <div className="space-y-2 max-h-80 overflow-y-auto">
              {draws.map((draw, i) => (
                <button
                  key={i}
                  onClick={() => onSelect(draw)}
                  className="w-full text-left px-4 py-3 rounded-lg border border-dark-border-subtle hover:border-primary-500/50 hover:bg-dark-bg-tertiary/50 transition-colors"
                >
                  <div className="text-sm font-mono text-dark-text-primary truncate">{draw.shortcode}</div>
                  <div className="text-xs text-dark-text-tertiary mt-1">
                    ID: {draw.id} &bull; Size: {draw.size.toUpperCase()} &bull; Zoom: {draw.zoom}
                  </div>
                </button>
              ))}
            </div>
          </div>
        ) : (
          /* Edit form */
          <div className="p-6">
            <div className="mb-4 px-3 py-2 rounded-lg bg-dark-bg-tertiary border border-dark-border-subtle">
              <span className="text-xs font-mono text-dark-text-secondary">{selectedDraw.shortcode}</span>
            </div>

            <div className="space-y-4">
              {/* Size */}
              <div>
                <label className="block text-xs font-medium text-dark-text-secondary mb-1.5">Size</label>
                <div className="flex gap-1.5">
                  {([['s', 'Small'], ['m', 'Medium'], ['l', 'Large']] as const).map(([val, label]) => (
                    <button
                      key={val}
                      type="button"
                      onClick={() => onSizeChange(val)}
                      className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
                        editSize === val
                          ? 'bg-primary-500 text-white'
                          : 'bg-dark-bg-tertiary text-dark-text-secondary hover:bg-dark-bg-tertiary/80'
                      }`}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              </div>

              {/* Zoom */}
              <div>
                <label className="block text-xs font-medium text-dark-text-secondary mb-1.5">Zoom</label>
                <div className="flex gap-1.5 flex-wrap">
                  {(['fit', '50%', '100%', '150%', '200%'] as const).map((val) => (
                    <button
                      key={val}
                      type="button"
                      onClick={() => onZoomChange(val)}
                      className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
                        editZoom === val
                          ? 'bg-primary-500 text-white'
                          : 'bg-dark-bg-tertiary text-dark-text-secondary hover:bg-dark-bg-tertiary/80'
                      }`}
                    >
                      {val}
                    </button>
                  ))}
                </div>
              </div>
            </div>

            {/* Actions */}
            <div className="flex items-center justify-between mt-6">
              <a
                href={`/draw/${selectedDraw.id}/edit`}
                target="_blank"
                rel="noopener noreferrer"
                className="text-xs text-primary-400 hover:text-primary-300 hover:underline transition-colors"
              >
                Open Editor
              </a>
              <div className="flex items-center gap-3">
                <button
                  onClick={() => onSelect(null as unknown as typeof selectedDraw)}
                  className="px-4 py-2 rounded-lg text-sm font-medium text-dark-text-secondary hover:text-dark-text-primary transition-colors"
                >
                  Back
                </button>
                <button
                  onClick={onSave}
                  className="px-4 py-2 rounded-lg text-sm font-medium bg-primary-600 text-white hover:bg-primary-500 transition-colors"
                >
                  Update
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
