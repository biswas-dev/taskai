import { apiClient } from './api'

export type UploadTarget = { taskId: number } | { pageId: number }

export interface UploadedMedia {
  url: string
  publicId: string
  altName: string
}

const ALLOWED_PREFIXES = ['image/', 'video/']
const ALLOWED_TYPES = ['application/pdf']

export function isUploadableFile(file: File): boolean {
  return ALLOWED_PREFIXES.some((p) => file.type.startsWith(p)) || ALLOWED_TYPES.includes(file.type)
}

export function fileKind(file: File): 'image' | 'video' | 'pdf' {
  if (file.type.startsWith('video/')) return 'video'
  if (file.type === 'application/pdf') return 'pdf'
  return 'image'
}

export function defaultAltText(file: File): string {
  return file.name.replace(/\.[^.]+$/, '').replaceAll(/[-_]/g, ' ').trim() || 'Image'
}

function timestampLabel(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}.${pad(date.getMinutes())}.${pad(date.getSeconds())}`
}

/**
 * Pulls uploadable files out of a paste or drop. Clipboard screenshots arrive
 * as a nameless "image.png", so they get a readable timestamped name.
 */
export function mediaFilesFrom(data: DataTransfer | null): File[] {
  if (!data) return []
  const files: File[] = []
  const fromItems = Array.from(data.items ?? [])
    .filter((item) => item.kind === 'file')
    .map((item) => item.getAsFile())
    .filter((f): f is File => f !== null)
  const source = fromItems.length > 0 ? fromItems : Array.from(data.files ?? [])
  for (const file of source) {
    if (!isUploadableFile(file)) continue
    if (!file.name || /^image\.(png|jpe?g|gif|webp)$/i.test(file.name)) {
      const ext = file.type.split('/')[1] || 'png'
      files.push(new File([file], `Screenshot ${timestampLabel(new Date())}.${ext}`, { type: file.type }))
    } else {
      files.push(file)
    }
  }
  return files
}

async function cloudinaryError(res: Response): Promise<string> {
  try {
    const body = await res.json()
    const message = body?.error?.message
    if (typeof message === 'string' && message) return message
  } catch {
    // Fall through to the status-based message.
  }
  return `HTTP ${res.status}`
}

/**
 * Uploads a file to the uploader's Cloudinary account and records it as an
 * attachment of the task or wiki page. Errors carry Cloudinary's own reason
 * (file too large, account restrictions, ...) so users can act on them.
 */
export async function uploadMedia(target: UploadTarget, file: File, altName = defaultAltText(file)): Promise<UploadedMedia> {
  if (!isUploadableFile(file)) {
    throw new Error('Only images, videos, and PDFs can be uploaded')
  }

  const sig = await apiClient.getUploadSignature(target)

  const formData = new FormData()
  formData.append('file', file)
  formData.append('api_key', sig.api_key)
  formData.append('timestamp', String(sig.timestamp))
  formData.append('signature', sig.signature)
  formData.append('folder', sig.folder)
  formData.append('public_id', sig.public_id)

  let res: Response
  try {
    res = await fetch(`https://api.cloudinary.com/v1_1/${sig.cloud_name}/auto/upload`, { method: 'POST', body: formData })
  } catch {
    throw new Error('Could not reach Cloudinary. Check your connection and try again.')
  }
  if (!res.ok) {
    throw new Error(`Cloudinary rejected "${file.name}": ${await cloudinaryError(res)}`)
  }
  const uploaded: { secure_url: string; public_id: string } = await res.json()

  const attachment = {
    filename: file.name,
    alt_name: altName,
    file_type: fileKind(file),
    content_type: file.type,
    file_size: file.size,
    cloudinary_url: uploaded.secure_url,
    cloudinary_public_id: uploaded.public_id,
  }
  if ('taskId' in target) {
    await apiClient.createTaskAttachment(target.taskId, attachment)
  } else {
    await apiClient.createWikiPageAttachment(target.pageId, attachment)
  }

  return { url: uploaded.secure_url, publicId: uploaded.public_id, altName }
}

export function mediaMarkdown(media: UploadedMedia, file: File): string {
  const kind = fileKind(file)
  if (kind === 'image') return `![${media.altName}](${media.url})`
  return `[${media.altName}](${media.url})`
}
