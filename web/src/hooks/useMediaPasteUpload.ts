import { useCallback, useRef, useState } from 'react'
import { mediaFilesFrom, mediaMarkdown, uploadMedia, type UploadTarget } from '../lib/upload'

interface Options {
  /** Where uploads are attached; null disables paste/drop uploading. */
  target: UploadTarget | null
  /** The textarea's current markdown. */
  value: string
  setValue: (value: string) => void
  onError: (message: string) => void
  /** Called after each successful upload, e.g. to refresh an attachment list. */
  onUploaded?: () => void
}

let placeholderSeq = 0

/**
 * Lets a markdown textarea accept pasted or dropped screenshots and files:
 * each one is uploaded and its markdown inserted where the cursor was, with a
 * visible placeholder while the upload runs.
 */
export function useMediaPasteUpload({ target, value, setValue, onError, onUploaded }: Options) {
  const valueRef = useRef(value)
  valueRef.current = value
  const [uploadingCount, setUploadingCount] = useState(0)

  const apply = useCallback((next: string) => {
    valueRef.current = next
    setValue(next)
  }, [setValue])

  const uploadInto = useCallback(async (el: HTMLTextAreaElement, files: File[]) => {
    if (!target || files.length === 0) return

    const placeholders = files.map((f) => `![Uploading ${f.name}…](uploading-${++placeholderSeq})`)
    const current = valueRef.current
    const start = el.selectionStart ?? current.length
    const end = el.selectionEnd ?? current.length
    const inserted = placeholders.join('\n')
    apply(current.slice(0, start) + inserted + current.slice(end))

    setUploadingCount((n) => n + files.length)
    // Sequential on purpose: the server numbers attachments as they are saved.
    for (let i = 0; i < files.length; i++) {
      const file = files[i]
      const placeholder = placeholders[i]
      try {
        const media = await uploadMedia(target, file)
        apply(valueRef.current.replace(placeholder, mediaMarkdown(media, file)))
        onUploaded?.()
      } catch (err: unknown) {
        apply(valueRef.current.replace(`${placeholder}\n`, '').replace(placeholder, ''))
        onError(err instanceof Error ? err.message : `Failed to upload ${file.name}`)
      } finally {
        setUploadingCount((n) => n - 1)
      }
    }
  }, [target, apply, onError, onUploaded])

  const onPaste = useCallback((e: React.ClipboardEvent<HTMLTextAreaElement>) => {
    if (!target) return
    // Rich copies (text plus an image) keep their normal text paste.
    if (e.clipboardData.types.includes('text/plain')) return
    const files = mediaFilesFrom(e.clipboardData)
    if (files.length === 0) return
    e.preventDefault()
    void uploadInto(e.currentTarget, files)
  }, [target, uploadInto])

  const onDrop = useCallback((e: React.DragEvent<HTMLTextAreaElement>) => {
    if (!target) return
    const files = mediaFilesFrom(e.dataTransfer)
    if (files.length === 0) return
    e.preventDefault()
    void uploadInto(e.currentTarget, files)
  }, [target, uploadInto])

  const onDragOver = useCallback((e: React.DragEvent<HTMLTextAreaElement>) => {
    if (target && e.dataTransfer.types.includes('Files')) e.preventDefault()
  }, [target])

  return { onPaste, onDrop, onDragOver, isUploading: uploadingCount > 0, uploadInto }
}
