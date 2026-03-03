// ── Pure helper functions extracted from WikiEditor.tsx ──────────────

// ── Types ────────────────────────────────────────────────────────────

export type ImageInfo = { html: string; url: string; alt: string; caption: string; index: number }
export type DrawInfo = { shortcode: string; id: string; size: string; zoom: string; index: number }
export type SyncState = 'connecting' | 'connected' | 'disconnected'
export type SaveStatus = 'idle' | 'saving' | 'saved' | 'error'

// ── HTML entity helpers ──────────────────────────────────────────────

export function escapeHtml(s: string): string {
  return s.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;')
}

export function unescapeHtml(s: string): string {
  return s.replaceAll('&amp;', '&').replaceAll('&lt;', '<').replaceAll('&gt;', '>').replaceAll('&quot;', '"')
}

// ── Image markup builder ─────────────────────────────────────────────

export function buildImageMarkup(url: string, alt: string, caption: string, size: string): string {
  const sizeStyles: Record<string, string> = {
    s: 'max-width:50%;height:auto;',
    m: 'max-width:75%;height:auto;',
    l: 'width:100%;height:auto;max-width:100%;',
  }
  const imgStyle = sizeStyles[size] || sizeStyles.m

  if (size === 'l' && !caption) {
    return `![${alt}](${url})`
  }
  const captionHtml = caption ? '<figcaption>' + escapeHtml(caption) + '</figcaption>' : ''
  return `<figure style="text-align:center;margin:1.5rem 0"><a href="${url}" data-lightbox="article-images" data-title="${escapeHtml(alt)}"><img src="${url}" alt="${escapeHtml(alt)}" style="${imgStyle}"/></a>${captionHtml}</figure>`
}

// ── Image detection helpers ──────────────────────────────────────────

export function findImagesInContent(content: string): ImageInfo[] {
  const images: ImageInfo[] = []

  const figureRegex = /<figure[^>]*>[\s\S]*?<\/figure>/g
  const imgSrcRegex = /src="([^"]+)"/
  const imgAltRegex = /alt="([^"]*)"/
  const captionRegex = /<figcaption>([\s\S]*?)<\/figcaption>/
  let match
  while ((match = figureRegex.exec(content)) !== null) {
    const figHtml = match[0]
    const srcMatch = imgSrcRegex.exec(figHtml)
    if (!srcMatch) continue
    const altMatch = imgAltRegex.exec(figHtml)
    const capMatch = captionRegex.exec(figHtml)
    images.push({
      html: figHtml,
      url: srcMatch[1],
      alt: unescapeHtml(altMatch?.[1] || ''),
      caption: unescapeHtml(capMatch?.[1] || ''),
      index: match.index,
    })
  }

  const mdRegex = /!\[([^\]]*)\]\(([^)]+)\)/g
  while ((match = mdRegex.exec(content)) !== null) {
    const pos = match.index
    const insideFigure = images.some(img => pos >= img.index && pos < img.index + img.html.length)
    if (insideFigure) continue
    images.push({ html: match[0], url: match[2], alt: match[1], caption: '', index: match.index })
  }

  images.sort((a, b) => a.index - b.index)
  return images
}

export function detectImageSize(html: string): string {
  if (!html.startsWith('<figure')) return 'l'
  if (/max-width:\s*50%/.test(html)) return 's'
  if (/max-width:\s*75%/.test(html)) return 'm'
  return 'l'
}

// ── Draw shortcode helpers ───────────────────────────────────────────

export function findDrawsInContent(content: string): DrawInfo[] {
  const draws: DrawInfo[] = []
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
  draws.sort((a, b) => a.index - b.index)
  return draws
}

export function mapYjsStatus(status: string): SyncState {
  if (status === 'connected') return 'connected'
  if (status === 'disconnected') return 'disconnected'
  return 'connecting'
}

export function findDrawShortcodeAtPosition(text: string, pos: number): string | null {
  const re = /\[draw:([a-zA-Z0-9_-]+)(?::edit)?\]/g
  let match
  while ((match = re.exec(text)) !== null) {
    if (pos >= match.index && pos <= match.index + match[0].length) {
      return match[1]
    }
  }
  return null
}

export function shouldSaveContent(isDirty: boolean, current: string, lastSaved: string): boolean {
  return isDirty && current !== lastSaved
}

// ── Status display helpers ───────────────────────────────────────────

export function getSyncStatusColor(syncState: SyncState): string {
  switch (syncState) {
    case 'connected': return 'bg-green-500'
    case 'connecting': return 'bg-yellow-500'
    case 'disconnected': return 'bg-red-500'
  }
}

export function getSaveStatusColor(saveStatus: SaveStatus, syncState: SyncState): string {
  switch (saveStatus) {
    case 'saving': return 'bg-yellow-500'
    case 'saved': return 'bg-green-500'
    case 'error': return 'bg-red-500'
    default: return getSyncStatusColor(syncState)
  }
}

export function getSaveStatusText(saveStatus: SaveStatus, autoSaveEnabled: boolean): string {
  switch (saveStatus) {
    case 'saving': return 'Saving...'
    case 'saved': return 'Saved'
    case 'error': return 'Save failed'
    default: return autoSaveEnabled ? 'Autosave on' : 'Autosave off'
  }
}

export function getSaveStatusTextColor(saveStatus: SaveStatus): string {
  switch (saveStatus) {
    case 'saving': return 'text-yellow-400'
    case 'saved': return 'text-green-400'
    case 'error': return 'text-red-400'
    default: return 'text-dark-text-tertiary'
  }
}

// ── Draw shortcode builder ───────────────────────────────────────────

export function buildDrawShortcode(id: string, size: string, zoom: string): string {
  const sizeTag = size === 'm' ? '' : ':' + size
  const zoomTag = zoom === 'fit' ? '' : ':z' + zoom
  return `[draw:${id}:edit${sizeTag}${zoomTag}]`
}

// ── Cursor/markup helpers ────────────────────────────────────────────

export function insertMarkupAtCursor(
  textarea: HTMLTextAreaElement | null,
  content: string,
  markup: string,
): { newContent: string; focusPos: number | null } {
  if (textarea) {
    const start = textarea.selectionStart
    const end = textarea.selectionEnd
    return { newContent: content.substring(0, start) + markup + content.substring(end), focusPos: start + markup.length }
  }
  const sep = content.endsWith('\n') ? '' : '\n'
  return { newContent: content + sep + markup + '\n', focusPos: null }
}

// ── Save status helpers ──────────────────────────────────────────────

export function clearSavedStatus(prev: SaveStatus): SaveStatus {
  return prev === 'saved' ? 'idle' : prev
}

// ── Pure cursor-insertion helpers ────────────────────────────────────

export interface CursorResult {
  newContent: string
  cursorStart: number
  cursorEnd: number
}

/**
 * Pure version of insertAtCursor — wraps selection (or placeholder "text")
 * with before/after strings and returns new content + cursor positions.
 */
export function insertAtCursorPure(
  content: string,
  selectionStart: number,
  selectionEnd: number,
  before: string,
  after: string,
): CursorResult {
  const selected = content.substring(selectionStart, selectionEnd)
  const insert = before + (selected || 'text') + after
  const newContent = content.substring(0, selectionStart) + insert + content.substring(selectionEnd)

  if (selected) {
    const cursorPos = selectionStart + before.length + selected.length + after.length
    return { newContent, cursorStart: cursorPos, cursorEnd: cursorPos }
  }
  return {
    newContent,
    cursorStart: selectionStart + before.length,
    cursorEnd: selectionStart + before.length + 'text'.length,
  }
}

/**
 * Pure version of insertLine — prepends prefix to the current line
 * and returns new content + cursor position.
 */
export function insertLinePure(
  content: string,
  cursorPosition: number,
  prefix: string,
): CursorResult {
  const lineStart = content.lastIndexOf('\n', cursorPosition - 1) + 1
  const newContent = content.substring(0, lineStart) + prefix + content.substring(lineStart)
  const newCursor = cursorPosition + prefix.length
  return { newContent, cursorStart: newCursor, cursorEnd: newCursor }
}

// ── Regex escape helper ─────────────────────────────────────────────

export function escapeRegExp(str: string): string {
  return str.replaceAll(/[.*+?^${}()|[\]\\]/g, String.raw`\$&`)
}

// ── Draw API types & helpers ────────────────────────────────────────

export interface DrawItem {
  id: string
  title: string
  updated_at: string
}

export async function fetchDrawings(projectId: number): Promise<DrawItem[]> {
  try {
    const token = localStorage.getItem('auth_token')
    // Get draw IDs registered to this project
    const regRes = await fetch(`/api/projects/${projectId}/drawings`, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
    if (!regRes.ok) return []
    const registered: { draw_id: string }[] = await regRes.json()
    if (registered.length === 0) return []

    // Fetch full drawing list from go-draw and filter to registered IDs
    const listRes = await fetch('/draw/api/list')
    if (!listRes.ok) return []
    const data = await listRes.json()
    const allDrawings: DrawItem[] = data.drawings || []
    const registeredIds = new Set(registered.map(r => r.draw_id))
    return allDrawings.filter(d => registeredIds.has(d.id))
  } catch {
    return []
  }
}

export async function createDrawing(projectId: number): Promise<string | null> {
  try {
    const res = await fetch('/draw/api/new', { method: 'POST' })
    const data = await res.json()
    if (data?.id) {
      // Register the new drawing with this project
      const token = localStorage.getItem('auth_token')
      await fetch(`/api/projects/${projectId}/drawings`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        body: JSON.stringify({ draw_id: data.id }),
      })
      const editUrl = data.edit_url || `/draw/${data.id}/edit`
      window.open(editUrl, '_blank')
      return data.id
    }
  } catch (err) {
    console.error('Failed to create drawing:', err)
  }
  return null
}

export async function renameDrawing(id: string, title: string): Promise<boolean> {
  try {
    await fetch(`/draw/api/${id}/rename`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title }),
    })
    return true
  } catch {
    return false
  }
}

export async function deleteDrawing(id: string): Promise<boolean> {
  try {
    await fetch(`/draw/api/${id}/delete`, { method: 'POST' })
    return true
  } catch {
    return false
  }
}

export async function deleteDrawings(ids: string[]): Promise<boolean> {
  try {
    await Promise.all(ids.map(id => fetch(`/draw/api/${id}/delete`, { method: 'POST' })))
    return true
  } catch {
    return false
  }
}

// ── Server-side preview fetcher ─────────────────────────────────────

export async function fetchPreview(
  previewEndpoint: string,
  markdown: string,
  signal?: AbortSignal,
): Promise<string> {
  const token = localStorage.getItem('auth_token')
  const resp = await fetch(previewEndpoint, {
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
