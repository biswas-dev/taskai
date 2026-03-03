import { useEffect, useState, useCallback, useRef } from 'react'
import { WikiPage, apiClient } from '../lib/api'
import SearchSelect from './ui/SearchSelect'
import ImagePickerModal from './ImagePickerModal'
import * as Y from 'yjs'
import { WebsocketProvider } from 'y-websocket'
import {
  unescapeHtml,
  buildImageMarkup,
  findImagesInContent,
  detectImageSize,
  findDrawsInContent,
  mapYjsStatus,
  findDrawShortcodeAtPosition,
  shouldSaveContent,
  getSaveStatusColor,
  getSaveStatusText,
  getSaveStatusTextColor,
  buildDrawShortcode,
  insertMarkupAtCursor,
  clearSavedStatus,
  insertAtCursorPure,
  insertLinePure,
  escapeRegExp,
  fetchDrawings,
  createDrawing,
  renameDrawing,
  deleteDrawing,
  deleteDrawings,
  fetchPreview,
} from './WikiEditor.helpers'
import type { ImageInfo, DrawInfo, DrawItem, SyncState, SaveStatus } from './WikiEditor.helpers'

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
  const { newContent, cursorStart, cursorEnd } = insertAtCursorPure(
    content, textarea.selectionStart, textarea.selectionEnd, before, after,
  )
  setContent(newContent)
  syncToYjs(newContent)
  if (isDirtyRef) isDirtyRef.current = true

  setTimeout(() => {
    textarea.focus()
    textarea.selectionStart = cursorStart
    textarea.selectionEnd = cursorEnd
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
  const { newContent, cursorStart } = insertLinePure(content, textarea.selectionStart, prefix)
  setContent(newContent)
  syncToYjs(newContent)
  if (isDirtyRef) isDirtyRef.current = true

  setTimeout(() => {
    textarea.focus()
    textarea.selectionStart = textarea.selectionEnd = cursorStart
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
  btn.dataset.size = sz
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
    const isActive = btn.dataset.size === activeSize
    btn.classList.toggle('active', isActive)
    btn.style.background = isActive ? 'rgba(99,102,241,0.8)' : 'transparent'
    btn.style.color = isActive ? '#fff' : 'rgba(255,255,255,0.7)'
  })
}

function initDrawEmbeds(
  container: HTMLElement | null,
  contentRef?: React.MutableRefObject<string>,
  updateContent?: (newContent: string) => void,
) {
  if (!container) return
  const embeds = container.querySelectorAll('.godraw-embed:not(.godraw-preview-init)')
  embeds.forEach((div) => {
    let src = (div as HTMLElement).dataset.src
    const w = (div as HTMLElement).dataset.width || '100%'
    const h = (div as HTMLElement).dataset.height || '520px'
    const zoom = (div as HTMLElement).dataset.zoom
    if (!src) return

    const drawIdMatch = /\/([a-zA-Z0-9_-]+?)(?:\/edit)?$/.exec(src)
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
        const scRe = new RegExp(String.raw`\[draw:` + escapeRegExp(drawId) + '(?::edit)?(?::([sml]))?')
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
          const re = new RegExp(String.raw`\[draw:` + escapeRegExp(drawId) + String.raw`(?::edit)?(?::[sml])?(?::z[^\]]+)?\]`)
          const m = re.exec(contentRef.current)
          if (!m) return
          const zoomMatch = /:z([^\]]+)/.exec(m[0])
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
    const wrapTarget = img.closest<HTMLElement>('figure') || img
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
        const figRe = new RegExp(String.raw`<figure[^>]*>[\s\S]*?` + escapeRegExp(imgUrl) + String.raw`[\s\S]*?</figure>`)
        const figMatch = figRe.exec(content)
        if (figMatch) {
          const altM = /alt="([^"]*)"/.exec(figMatch[0])
          const capM = /<figcaption>([\s\S]*?)<\/figcaption>/.exec(figMatch[0])
          const alt = unescapeHtml(altM?.[1] || '')
          const cap = unescapeHtml(capM?.[1] || '')
          updateContent(content.replace(figMatch[0], buildImageMarkup(imgUrl, alt, cap, sz)))
        } else {
          // Try markdown ![alt](url)
          const mdRe = new RegExp(String.raw`!\[([^\]]*)\]\(` + escapeRegExp(imgUrl) + String.raw`\)`)
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

// ── Draw API helpers ─────────────────────────────────────────
// Moved to WikiEditor.helpers.ts: fetchDrawings, createDrawing,
// renameDrawing, deleteDrawing, deleteDrawings

async function uploadSingleFile(file: File, pageId: number): Promise<{ url: string; publicId: string; altName: string }> {
  const sig = await apiClient.getUploadSignature({ pageId })
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
  const altName = file.name.replace(/\.[^.]+$/, '').replaceAll(/[-_]/g, ' ')

  await apiClient.createWikiPageAttachment(pageId, {
    filename: file.name,
    alt_name: altName,
    file_type: 'image',
    content_type: file.type,
    file_size: file.size,
    cloudinary_url: uploadData.secure_url,
    cloudinary_public_id: uploadData.public_id,
  })

  return { url: uploadData.secure_url, publicId: uploadData.public_id, altName }
}

// ── Server-side preview fetcher ──────────────────────────────────
// fetchPreview moved to WikiEditor.helpers.ts

async function abortAndFetchPreview(
  abortRef: React.MutableRefObject<AbortController | null>,
  markdown: string,
): Promise<string | null> {
  if (abortRef.current) abortRef.current.abort()
  const controller = new AbortController()
  abortRef.current = controller
  try {
    return await fetchPreview(PREVIEW_ENDPOINT, markdown, controller.signal)
  } catch {
    return null
  }
}

// ── Custom hooks (extract complexity from component) ──────────────

function useGlobalKeyboard(isFullscreen: boolean, setIsFullscreen: React.Dispatch<React.SetStateAction<boolean>>) {
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
    globalThis.addEventListener('keydown', handler)
    return () => globalThis.removeEventListener('keydown', handler)
  }, [isFullscreen, setIsFullscreen])
}

function useAutoSave(
  pageId: number,
  autoSaveEnabled: boolean,
  saveNow: () => Promise<void>,
  isDirtyRef: React.MutableRefObject<boolean>,
  contentRef: React.MutableRefObject<string>,
  lastSavedContentRef: React.MutableRefObject<string>,
) {
  // Autosave interval (10 seconds)
  useEffect(() => {
    if (!autoSaveEnabled) return

    const saveToServer = async () => {
      if (!shouldSaveContent(isDirtyRef.current, contentRef.current, lastSavedContentRef.current)) return
      await saveNow()
    }

    const interval = setInterval(saveToServer, 10_000)

    // Save on unmount — refs are stable objects, safe to read .current in cleanup
    return () => {
      clearInterval(interval)
      // eslint-disable-next-line react-hooks/exhaustive-deps
      if (shouldSaveContent(isDirtyRef.current, contentRef.current, lastSavedContentRef.current)) {
        // eslint-disable-next-line react-hooks/exhaustive-deps
        apiClient.updateWikiPageContent(pageId, contentRef.current).catch(() => {})
      }
    }
  }, [pageId, autoSaveEnabled, saveNow, isDirtyRef, contentRef, lastSavedContentRef])

  // Save on beforeunload
  useEffect(() => {
    const handler = () => {
      if (!shouldSaveContent(isDirtyRef.current, contentRef.current, lastSavedContentRef.current)) return
      const blob = new Blob([JSON.stringify({ content: contentRef.current })], { type: 'application/json' })
      const API_BASE_FOR_SAVE = import.meta.env.VITE_API_URL || (import.meta.env.PROD ? '' : 'http://localhost:8080')
      navigator.sendBeacon(
        `${API_BASE_FOR_SAVE}/api/wiki/pages/${pageId}/content`,
        blob
      )
    }
    globalThis.addEventListener('beforeunload', handler)
    return () => globalThis.removeEventListener('beforeunload', handler)
  }, [pageId, isDirtyRef, contentRef, lastSavedContentRef])
}

interface ImageDropOpts {
  pageId: number
  isFullscreen: boolean
  content: string
  setContent: (c: string) => void
  syncToYjs: (c: string) => void
  isDirtyRef: React.MutableRefObject<boolean>
  textareaRef: React.RefObject<HTMLTextAreaElement | null>
  fsTextareaRef: React.RefObject<HTMLTextAreaElement | null>
}

function useImageDrop(opts: ImageDropOpts) {
  const { pageId, isFullscreen, content, setContent, syncToYjs, isDirtyRef, textareaRef, fsTextareaRef } = opts
  const dragCounterRef = useRef(0)
  const [isDragOver, setIsDragOver] = useState(false)
  const [isDropUploading, setIsDropUploading] = useState(false)

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
        const { url, altName } = await uploadSingleFile(file, pageId)
        const markup = `![${altName}](${url})`
        const start = textarea?.selectionStart ?? content.length
        const end = textarea?.selectionEnd ?? content.length
        const newContent = content.substring(0, start) + markup + '\n' + content.substring(end)
        setContent(newContent)
        syncToYjs(newContent)
        isDirtyRef.current = true
      }
    } catch (err) {
      console.error('Drop upload failed:', err)
    } finally {
      setIsDropUploading(false)
    }
  }, [isFullscreen, content, setContent, syncToYjs, pageId, isDirtyRef, textareaRef, fsTextareaRef])

  return { isDragOver, isDropUploading, handleDragEnter, handleDragOver, handleDragLeave, handleDrop }
}

// ── Draw browser hook ─────────────────────────────────────────────

interface ContentOpts {
  content: string
  setContent: (c: string) => void
  syncToYjs: (c: string) => void
  isDirtyRef: React.MutableRefObject<boolean>
  isFullscreen: boolean
  textareaRef: React.RefObject<HTMLTextAreaElement | null>
  fsTextareaRef: React.RefObject<HTMLTextAreaElement | null>
  projectId: number
}

function useDrawBrowser(opts: ContentOpts) {
  const { content, setContent, syncToYjs, isDirtyRef, isFullscreen, textareaRef, fsTextareaRef, projectId } = opts

  const [showDrawBrowser, setShowDrawBrowser] = useState(false)
  const [drawList, setDrawList] = useState<DrawItem[]>([])
  const [drawLoading, setDrawLoading] = useState(false)
  const [editDrawList, setEditDrawList] = useState<DrawInfo[] | null>(null)
  const [selectedEditDraw, setSelectedEditDraw] = useState<DrawInfo | null>(null)
  const [editDrawSize, setEditDrawSize] = useState('m')
  const [editDrawZoom, setEditDrawZoom] = useState('fit')

  const loadDrawings = useCallback(async () => {
    setDrawLoading(true)
    const drawings = await fetchDrawings(projectId)
    setDrawList(drawings)
    setDrawLoading(false)
  }, [projectId])

  const handleDraw = useCallback(() => {
    setShowDrawBrowser(true)
    loadDrawings()
  }, [loadDrawings])

  const handleDrawInsert = useCallback((id: string, size: string, zoom: string) => {
    const textarea = isFullscreen ? fsTextareaRef.current : textareaRef.current
    const markup = '\n' + buildDrawShortcode(id, size, zoom) + '\n'
    const { newContent, focusPos } = insertMarkupAtCursor(textarea, content, markup)
    setContent(newContent)
    syncToYjs(newContent)
    isDirtyRef.current = true
    if (focusPos !== null && textarea) {
      setTimeout(() => { textarea.focus(); textarea.selectionStart = textarea.selectionEnd = focusPos }, 0)
    }
    setShowDrawBrowser(false)
  }, [isFullscreen, content, setContent, syncToYjs, isDirtyRef, textareaRef, fsTextareaRef])

  const handleDrawRename = useCallback(async (id: string, title: string) => {
    const ok = await renameDrawing(id, title)
    if (ok) setDrawList(prev => prev.map(d => d.id === id ? { ...d, title } : d))
  }, [])

  const handleDrawDelete = useCallback(async (id: string) => {
    const ok = await deleteDrawing(id)
    if (!ok) { alert('Failed to delete drawing'); return }
    setDrawList(prev => prev.filter(d => d.id !== id))
  }, [])

  const handleDrawNew = useCallback(async () => {
    const created = await createDrawing(projectId)
    if (created) loadDrawings()
  }, [loadDrawings, projectId])

  const handleDrawDeleteUnused = useCallback(async (unusedIds: string[]) => {
    await deleteDrawings(unusedIds)
    loadDrawings()
  }, [loadDrawings])

  const handleEditDraw = useCallback(() => {
    const draws = findDrawsInContent(content)
    if (draws.length === 0) {
      alert('No draw shortcodes found in content')
      return
    }
    setEditDrawList(draws)
    setSelectedEditDraw(null)
  }, [content])

  const selectDrawForEdit = (draw: DrawInfo) => {
    setSelectedEditDraw(draw)
    setEditDrawSize(draw.size)
    setEditDrawZoom(draw.zoom)
  }

  const saveEditDraw = useCallback(() => {
    if (!selectedEditDraw) return
    const newShortcode = buildDrawShortcode(selectedEditDraw.id, editDrawSize, editDrawZoom)
    const newContent = content.replace(selectedEditDraw.shortcode, newShortcode)
    setContent(newContent)
    syncToYjs(newContent)
    isDirtyRef.current = true
    setEditDrawList(null)
    setSelectedEditDraw(null)
  }, [selectedEditDraw, editDrawSize, editDrawZoom, content, setContent, syncToYjs, isDirtyRef])

  return {
    showDrawBrowser, drawList, drawLoading,
    editDrawList, selectedEditDraw, editDrawSize, editDrawZoom,
    handleDraw, handleDrawInsert, handleDrawRename, handleDrawDelete,
    handleDrawNew, handleDrawDeleteUnused, handleEditDraw, selectDrawForEdit, saveEditDraw,
    setEditDrawSize, setEditDrawZoom,
    closeDrawBrowser: useCallback(() => setShowDrawBrowser(false), []),
    closeEditDraw: useCallback(() => { setEditDrawList(null); setSelectedEditDraw(null) }, []),
  }
}

// ── Image edit hook ──────────────────────────────────────────────

function useImageEdit(opts: Pick<ContentOpts, 'content' | 'setContent' | 'syncToYjs' | 'isDirtyRef'>) {
  const { content, setContent, syncToYjs, isDirtyRef } = opts

  const [editImgList, setEditImgList] = useState<ImageInfo[] | null>(null)
  const [selectedEditImg, setSelectedEditImg] = useState<ImageInfo | null>(null)
  const [editAlt, setEditAlt] = useState('')
  const [editCaption, setEditCaption] = useState('')
  const [editSize, setEditSize] = useState('m')

  const handleEditImg = useCallback(() => {
    const images = findImagesInContent(content)
    if (images.length === 0) {
      alert('No images found in content')
      return
    }
    setEditImgList(images)
    setSelectedEditImg(null)
  }, [content])

  const selectImgForEdit = (img: ImageInfo) => {
    setSelectedEditImg(img)
    setEditAlt(img.alt)
    setEditCaption(img.caption)
    setEditSize(detectImageSize(img.html))
  }

  const saveEditImg = useCallback(() => {
    if (!selectedEditImg) return
    const newMarkup = buildImageMarkup(selectedEditImg.url, editAlt, editCaption, editSize)
    const newContent = content.replace(selectedEditImg.html, newMarkup)
    setContent(newContent)
    syncToYjs(newContent)
    isDirtyRef.current = true
    setEditImgList(null)
    setSelectedEditImg(null)
  }, [selectedEditImg, editAlt, editCaption, editSize, content, setContent, syncToYjs, isDirtyRef])

  return {
    editImgList, selectedEditImg, editAlt, editCaption, editSize,
    handleEditImg, selectImgForEdit, saveEditImg,
    setEditAlt, setEditCaption, setEditSize,
    deselectImg: useCallback(() => setSelectedEditImg(null), []),
    closeEditImg: useCallback(() => { setEditImgList(null); setSelectedEditImg(null) }, []),
  }
}

// ── Small sub-components to reduce cognitive complexity ──────────

function EditImageModal({ editImgList, selectedEditImg, editAlt, editCaption, editSize, onSelectImg, onAltChange, onCaptionChange, onSizeChange, onSave, onDeselect, onClose }: Readonly<{
  editImgList: ImageInfo[] | null
  selectedEditImg: ImageInfo | null
  editAlt: string
  editCaption: string
  editSize: string
  onSelectImg: (img: ImageInfo) => void
  onAltChange: (v: string) => void
  onCaptionChange: (v: string) => void
  onSizeChange: (v: string) => void
  onSave: () => void
  onDeselect: () => void
  onClose: () => void
}>) {
  if (!editImgList) return null
  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center">
      <button type="button" className="absolute inset-0 bg-black/60 backdrop-blur-sm border-0" onClick={onClose} aria-label="Close dialog" />
      <dialog open className="relative z-[1] w-full max-w-2xl mx-4 m-0 p-0 bg-dark-bg-secondary rounded-xl border border-dark-border-subtle shadow-2xl overflow-hidden">
        <div className="flex items-center justify-between px-6 py-4 border-b border-dark-border-subtle">
          <h3 className="text-lg font-semibold text-dark-text-primary">Edit Image</h3>
          <button onClick={onClose} className="text-dark-text-tertiary hover:text-dark-text-primary transition-colors">
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {selectedEditImg ? (
          <div className="p-6">
            <div className="flex gap-4">
              <div className="w-40 h-40 flex-shrink-0 rounded-lg overflow-hidden bg-dark-bg-tertiary border border-dark-border-subtle">
                <img src={selectedEditImg.url} alt={editAlt} className="w-full h-full object-cover" />
              </div>
              <div className="flex-1 space-y-4">
                <div>
                  <label htmlFor="edit-img-alt" className="block text-xs font-medium text-dark-text-secondary mb-1">Alt text</label>
                  <input id="edit-img-alt" type="text" value={editAlt} onChange={e => onAltChange(e.target.value)} className="w-full px-3 py-2 rounded-lg bg-dark-bg-tertiary border border-dark-border-subtle text-sm text-dark-text-primary focus:outline-none focus:border-primary-500" placeholder="Describe this image..." />
                </div>
                <div>
                  <label htmlFor="edit-img-caption" className="block text-xs font-medium text-dark-text-secondary mb-1">Caption</label>
                  <input id="edit-img-caption" type="text" value={editCaption} onChange={e => onCaptionChange(e.target.value)} className="w-full px-3 py-2 rounded-lg bg-dark-bg-tertiary border border-dark-border-subtle text-sm text-dark-text-primary focus:outline-none focus:border-primary-500" placeholder="Optional caption..." />
                </div>
                <fieldset className="border-0 p-0 m-0 min-w-0">
                  <legend className="block text-xs font-medium text-dark-text-secondary mb-1">Size</legend>
                  <div className="flex gap-1.5">
                    {([['s', 'Small'], ['m', 'Medium'], ['l', 'Large']] as const).map(([val, label]) => (
                      <button key={val} type="button" onClick={() => onSizeChange(val)} className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${editSize === val ? 'bg-primary-500 text-white' : 'bg-dark-bg-tertiary text-dark-text-secondary hover:bg-dark-bg-tertiary/80'}`}>
                        {label}
                      </button>
                    ))}
                  </div>
                </fieldset>
              </div>
            </div>
            <div className="flex items-center justify-end gap-3 mt-6">
              <button onClick={onDeselect} className="px-4 py-2 rounded-lg text-sm font-medium text-dark-text-secondary hover:text-dark-text-primary transition-colors">Back</button>
              <button onClick={onSave} className="px-4 py-2 rounded-lg text-sm font-medium bg-primary-600 text-white hover:bg-primary-500 transition-colors">Save</button>
            </div>
          </div>
        ) : (
          <div className="p-6">
            <p className="text-sm text-dark-text-secondary mb-4">Select an image to edit:</p>
            <div className="grid grid-cols-3 gap-3 max-h-80 overflow-y-auto">
              {editImgList.map((img) => (
                <button key={img.url} onClick={() => onSelectImg(img)} className="group relative aspect-square rounded-lg overflow-hidden border-2 border-dark-border-subtle hover:border-primary-500 transition-colors bg-dark-bg-tertiary">
                  <img src={img.url} alt={img.alt} className="w-full h-full object-cover" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} />
                  <div className="absolute inset-x-0 bottom-0 bg-black/70 px-2 py-1">
                    <p className="text-xs text-dark-text-secondary truncate">{img.alt || 'No alt text'}</p>
                  </div>
                </button>
              ))}
            </div>
          </div>
        )}
      </dialog>
    </div>
  )
}

function DropOverlay({ isDragOver, isDropUploading }: Readonly<{ isDragOver: boolean; isDropUploading: boolean }>) {
  if (!isDragOver && !isDropUploading) return null
  return (
    <div className="absolute inset-0 bg-primary-500/10 border-2 border-dashed border-primary-500 rounded-lg flex flex-col items-center justify-center gap-2 pointer-events-none z-10">
      <svg className="w-12 h-12 text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
      </svg>
      <span className="text-sm font-semibold text-primary-400">
        {isDropUploading ? 'Uploading...' : 'Drop images here'}
      </span>
    </div>
  )
}

function PreviewContent({ previewHTML, content, previewRef }: Readonly<{
  previewHTML: string
  content: string
  previewRef: React.Ref<HTMLDivElement>
}>) {
  if (previewHTML) {
    return (
      <div
        ref={previewRef}
        className="prose prose-invert max-w-none"
        dangerouslySetInnerHTML={{ __html: previewHTML }}
      />
    )
  }
  if (content.trim()) {
    return (
      <div className="flex items-center justify-center h-full text-dark-text-tertiary">
        <p className="text-sm">Loading preview...</p>
      </div>
    )
  }
  return (
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
  )
}

// ── Component ────────────────────────────────────────────────────

export default function WikiEditor({ page }: Readonly<WikiEditorProps>) {
  const [content, setContent] = useState('')
  const [isPreview, setIsPreview] = useState(true)
  const [previewHTML, setPreviewHTML] = useState('')
  const [syncState, setSyncState] = useState<SyncState>('connecting')
  const [showImagePicker, setShowImagePicker] = useState(false)
  const [saveStatus, setSaveStatus] = useState<SaveStatus>('idle')
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
  const dividerRef = useRef<HTMLButtonElement>(null)
  const fsContainerRef = useRef<HTMLDivElement>(null)
  const previewRef = useRef<HTMLDivElement>(null)
  const fsPreviewRef = useRef<HTMLDivElement>(null)
  const [fsSplitPct, setFsSplitPct] = useState(50)

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
      setTimeout(() => setSaveStatus(clearSavedStatus), 3000)
    } catch {
      setSaveStatus('error')
    } finally {
      isSavingRef.current = false
    }
  }, [page.id])

  useAutoSave(page.id, autoSaveEnabled, saveNow, isDirtyRef, contentRef, lastSavedContentRef)

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
      setSyncState(mapYjsStatus(status))
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

    switch (action.action) {
      case 'wrap':
        insertAtCursor(textarea, action.before!, action.after!, content, setContent, syncToYjs, isDirtyRef)
        break
      case 'line':
        insertLine(textarea, action.prefix!, content, setContent, syncToYjs, isDirtyRef)
        break
      case 'insert': {
        const start = textarea.selectionStart
        const newContent = content.substring(0, start) + action.text! + content.substring(start)
        setContent(newContent)
        syncToYjs(newContent)
        isDirtyRef.current = true
        setTimeout(() => {
          textarea.focus()
          textarea.selectionStart = textarea.selectionEnd = start + action.text!.length
        }, 0)
        break
      }
    }
  }, [content, syncToYjs, isFullscreen])

  // ── Server-side preview (inline mode) ────────────────────────

  const loadPreview = useCallback(async (markdown: string) => {
    const html = await abortAndFetchPreview(abortRef, markdown)
    if (html !== null) setPreviewHTML(html)
  }, [])

  useEffect(() => {
    if (isPreview) loadPreview(content)
  }, [isPreview, content, loadPreview])

  // ── Fullscreen live preview (debounced) ──────────────────────

  const scheduleFsPreview = useCallback((markdown: string) => {
    if (previewTimerRef.current) clearTimeout(previewTimerRef.current)
    previewTimerRef.current = setTimeout(async () => {
      const html = await abortAndFetchPreview(abortRef, markdown)
      if (html !== null) setFsPreviewHTML(html)
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
    switch (e.key) {
      case 'Tab': {
        e.preventDefault()
        const textarea = e.currentTarget
        const start = textarea.selectionStart
        const end = textarea.selectionEnd
        const newContent = content.substring(0, start) + '  ' + content.substring(end)
        setContent(newContent)
        syncToYjs(newContent)
        isDirtyRef.current = true
        setTimeout(() => { textarea.selectionStart = textarea.selectionEnd = start + 2 }, 0)
        break
      }
      case 'F11':
        e.preventDefault()
        setIsFullscreen(prev => !prev)
        break
      case 'Escape':
        if (isFullscreen) { e.preventDefault(); setIsFullscreen(false) }
        break
    }
  }, [content, syncToYjs, isFullscreen])

  // ── Double-click draw shortcode → open editor ─────────────

  const handleDoubleClick = useCallback((e: React.MouseEvent<HTMLTextAreaElement>) => {
    const drawId = findDrawShortcodeAtPosition(e.currentTarget.value, e.currentTarget.selectionStart)
    if (drawId) window.open(`/draw/${drawId}/edit`, '_blank')
  }, [])

  useGlobalKeyboard(isFullscreen, setIsFullscreen)

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
    const markup = buildImageMarkup(url, alt, caption || '', size || 'm')
    const textarea = isFullscreen ? fsTextareaRef.current : textareaRef.current
    const { newContent, focusPos } = insertMarkupAtCursor(textarea, content, markup)
    setContent(newContent)
    syncToYjs(newContent)
    isDirtyRef.current = true
    if (focusPos !== null && textarea) {
      setTimeout(() => { textarea.selectionStart = textarea.selectionEnd = focusPos; textarea.focus() }, 0)
    }
    setShowImagePicker(false)
  }

  const { isDragOver, isDropUploading, handleDragEnter, handleDragOver, handleDragLeave, handleDrop } =
    useImageDrop({ pageId: page.id, isFullscreen, content, setContent, syncToYjs, isDirtyRef, textareaRef, fsTextareaRef })

  const {
    editImgList, selectedEditImg, editAlt, editCaption, editSize,
    handleEditImg, selectImgForEdit, saveEditImg,
    setEditAlt, setEditCaption, setEditSize, deselectImg, closeEditImg,
  } = useImageEdit({ content, setContent, syncToYjs, isDirtyRef })

  const {
    showDrawBrowser, drawList, drawLoading,
    editDrawList, selectedEditDraw, editDrawSize, editDrawZoom,
    handleDraw, handleDrawInsert, handleDrawRename, handleDrawDelete,
    handleDrawNew, handleDrawDeleteUnused, handleEditDraw, selectDrawForEdit, saveEditDraw,
    setEditDrawSize, setEditDrawZoom, closeDrawBrowser, closeEditDraw,
  } = useDrawBrowser({ content, setContent, syncToYjs, isDirtyRef, isFullscreen, textareaRef, fsTextareaRef, projectId: page.project_id })

  // ── Toolbar JSX ──────────────────────────────────────────────

  const renderToolbar = (compact?: boolean) => (
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

  // ── Fullscreen overlay ───────────────────────────────────────

  const fullscreenContent = isFullscreen && (
    <div className="fixed inset-0 z-50 bg-dark-bg-primary flex flex-col">
      {/* Top bar */}
      <div className="flex items-center justify-between border-b border-dark-border-subtle bg-dark-bg-secondary px-4 py-2">
        <div className="flex items-center gap-4">
          <span className="text-sm font-semibold text-dark-text-primary truncate max-w-xs">
            {page.title}
          </span>
          {renderToolbar(true)}
        </div>
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2">
            <div className={`w-2 h-2 rounded-full ${getSaveStatusColor(saveStatus, syncState)}`} />
            <span className="text-xs text-dark-text-tertiary">
              {getSaveStatusText(saveStatus, autoSaveEnabled)}
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
        <section
          aria-label="Editor with drop support"
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
          <DropOverlay isDragOver={isDragOver} isDropUploading={isDropUploading} />
        </section>

        {/* Divider */}
        <button
          type="button"
          ref={dividerRef}
          onMouseDown={handleDividerMouseDown}
          className="w-1.5 bg-dark-border-subtle hover:bg-dark-accent-primary/50 cursor-col-resize transition-colors flex-shrink-0 border-0 p-0"
          aria-label="Resize panels"
        />

        {/* Right: live preview */}
        <div style={{ width: `${100 - fsSplitPct}%` }} className="overflow-y-auto p-4">
          <PreviewContent previewHTML={fsPreviewHTML} content={content} previewRef={fsPreviewRef} />
        </div>
      </div>
    </div>
  )

  // ── Render ───────────────────────────────────────────────────

  return (
    <>
      {fullscreenContent}

      {!isFullscreen && (
        <div className="flex flex-col h-full">
          {/* Header */}
          <div className="border-b border-dark-border-subtle bg-dark-bg-secondary px-6 py-4">
            <div className="flex items-center justify-between">
              <div>
                <h1 className="text-2xl font-semibold text-dark-text-primary">{page.title}</h1>
                <div className="flex items-center gap-3 mt-2">
                  <div className="flex items-center gap-2">
                    <div className={`w-2 h-2 rounded-full ${getSaveStatusColor(saveStatus, syncState)}`} />
                    <span className="text-sm text-dark-text-tertiary">
                      {getSaveStatusText(saveStatus, autoSaveEnabled)}
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
              {renderToolbar()}
            </div>
          )}

          {/* Content */}
          <div className="flex-1 overflow-hidden flex flex-col">
            {isPreview ? (
              <div className="h-full overflow-y-auto px-6 py-4">
                <PreviewContent previewHTML={previewHTML} content={content} previewRef={previewRef} />
              </div>
            ) : (
              <>
                <section
                  aria-label="Editor with drop support"
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
                  <DropOverlay isDragOver={isDragOver} isDropUploading={isDropUploading} />
                </section>
                {/* Visible drop zone */}
                <section
                  aria-label="Drop zone for images"
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
                </section>
              </>
            )}
          </div>

          {/* Footer helper */}
          {!isPreview && (
            <div className="border-t border-dark-border-subtle bg-dark-bg-secondary px-6 py-2">
              <div className="flex items-center gap-4 text-xs text-dark-text-tertiary">
                <span>Markdown supported</span>
                <span>&bull;</span>
                <span className={getSaveStatusTextColor(saveStatus)}>
                  {getSaveStatusText(saveStatus, autoSaveEnabled)}
                </span>
                <span>&bull;</span>
                <span>Tab to indent</span>
                <span>&bull;</span>
                <span>F11 fullscreen</span>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Shared modals — rendered once regardless of fullscreen state */}
      {showImagePicker && (
        <ImagePickerModal
          onSelect={insertImageMarkdown}
          onClose={() => setShowImagePicker(false)}
          projectId={page.project_id}
          wikiPageId={page.id}
          onUploadComplete={() => {}}
        />
      )}

      <EditImageModal
        editImgList={editImgList}
        selectedEditImg={selectedEditImg}
        editAlt={editAlt}
        editCaption={editCaption}
        editSize={editSize}
        onSelectImg={selectImgForEdit}
        onAltChange={setEditAlt}
        onCaptionChange={setEditCaption}
        onSizeChange={setEditSize}
        onSave={saveEditImg}
        onDeselect={deselectImg}
        onClose={closeEditImg}
      />

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
          onClose={closeDrawBrowser}
        />
      )}

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
          onClose={closeEditDraw}
        />
      )}
    </>
  )
}

// ── DrawCard sub-component ───────────────────────────────────────

function DrawCard({ drawing, isUsed, onInsert, onRename, onDelete }: Readonly<{
  drawing: DrawItem
  isUsed: boolean
  onInsert: (id: string, size: string, zoom: string) => void
  onRename: (id: string, title: string) => void
  onDelete: (id: string) => void
}>) {
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

  return (
    <div
      className={`border rounded-xl p-4 flex flex-col gap-1.5 transition-colors ${isUsed ? 'border-green-500/30 hover:border-green-500/50' : 'border-dark-border-subtle hover:border-primary-500/50'}`}
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
      <div className="flex gap-1.5 mt-1 items-center" role="toolbar" onClick={e => e.stopPropagation()} onKeyDown={e => e.stopPropagation()}>
        <SearchSelect
          variant="inline"
          value={size}
          onChange={setSize}
          options={[
            { value: 's', label: 'S' },
            { value: 'm', label: 'M' },
            { value: 'l', label: 'L' },
          ]}
        />
        <SearchSelect
          variant="inline"
          value={zoom}
          onChange={setZoom}
          options={[
            { value: 'fit', label: 'fit' },
            { value: '50%', label: '50%' },
            { value: '100%', label: '100%' },
            { value: '150%', label: '150%' },
            { value: '200%', label: '200%' },
          ]}
        />
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
      <button
        onClick={() => onInsert(drawing.id, size, zoom)}
        className="mt-1.5 w-full py-1.5 rounded-lg text-xs font-medium bg-primary-500/10 text-primary-400 hover:bg-primary-500/20 transition-colors"
      >
        Insert
      </button>
    </div>
  )
}

// ── DrawBrowserModal ─────────────────────────────────────────────

function DrawBrowserModal({ drawings, loading, editorContent, onInsert, onRename, onDelete, onDeleteUnused, onNew, onClose }: Readonly<{
  drawings: DrawItem[]
  loading: boolean
  editorContent: string
  onInsert: (id: string, size: string, zoom: string) => void
  onRename: (id: string, title: string) => void
  onDelete: (id: string) => void
  onDeleteUnused: (ids: string[]) => void
  onNew: () => void
  onClose: () => void
}>) {
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
    <div className="fixed inset-0 z-[60] flex items-center justify-center">
      <button
        type="button"
        className="absolute inset-0 bg-black/60 backdrop-blur-sm border-0"
        onClick={onClose}
        aria-label="Close dialog"
      />
      <dialog
        open
        className="relative z-[1] w-full max-w-2xl mx-4 m-0 p-0 bg-dark-bg-secondary rounded-xl border border-dark-border-subtle shadow-2xl overflow-hidden flex flex-col max-h-[80vh]"
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
          {loading && (
            <p className="text-center text-sm text-dark-text-tertiary py-8">Loading drawings...</p>
          )}
          {!loading && drawings.length === 0 && (
            <p className="text-center text-sm text-dark-text-tertiary py-8">No drawings yet. Create one above!</p>
          )}
          {!loading && drawings.length > 0 && (
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
          <div className="absolute inset-0 z-10 flex items-center justify-center">
            <button
              type="button"
              className="absolute inset-0 bg-black/45 backdrop-blur-sm rounded-xl border-0"
              onClick={() => setConfirmAction(null)}
              aria-label="Close confirmation"
            />
            <dialog
              open
              className="relative z-[1] m-0 p-6 bg-dark-bg-secondary rounded-xl border border-dark-border-subtle shadow-2xl max-w-[340px] text-center"
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
            </dialog>
          </div>
        )}
      </dialog>
    </div>
  )
}

// ── EditDrawModal ────────────────────────────────────────────────

function EditDrawModal({ draws, selectedDraw, editSize, editZoom, onSelect, onSizeChange, onZoomChange, onSave, onClose }: Readonly<{
  draws: DrawInfo[]
  selectedDraw: DrawInfo | null
  editSize: string
  editZoom: string
  onSelect: (draw: DrawInfo) => void
  onSizeChange: (size: string) => void
  onZoomChange: (zoom: string) => void
  onSave: () => void
  onClose: () => void
}>) {
  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center">
      <button
        type="button"
        className="absolute inset-0 bg-black/60 backdrop-blur-sm border-0"
        onClick={onClose}
        aria-label="Close dialog"
      />
      <dialog
        open
        className="relative z-[1] w-full max-w-lg mx-4 m-0 p-0 bg-dark-bg-secondary rounded-xl border border-dark-border-subtle shadow-2xl overflow-hidden"
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

        {selectedDraw ? null : (
          /* Draw shortcode list */
          <div className="p-6">
            <p className="text-sm text-dark-text-secondary mb-4">Select a draw shortcode to edit:</p>
            <div className="space-y-2 max-h-80 overflow-y-auto">
              {draws.map((draw) => (
                <button
                  key={draw.shortcode}
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
        )}
        {selectedDraw && (
          /* Edit form */
          <div className="p-6">
            <div className="mb-4 px-3 py-2 rounded-lg bg-dark-bg-tertiary border border-dark-border-subtle">
              <span className="text-xs font-mono text-dark-text-secondary">{selectedDraw.shortcode}</span>
            </div>

            <div className="space-y-4">
              {/* Size */}
              <fieldset className="border-0 p-0 m-0 min-w-0">
                <legend className="block text-xs font-medium text-dark-text-secondary mb-1.5">Size</legend>
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
              </fieldset>

              {/* Zoom */}
              <fieldset className="border-0 p-0 m-0 min-w-0">
                <legend className="block text-xs font-medium text-dark-text-secondary mb-1.5">Zoom</legend>
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
              </fieldset>
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
      </dialog>
    </div>
  )
}
