import { describe, it, expect, beforeEach, vi } from 'vitest'
import { useState } from 'react'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'

const mocks = vi.hoisted(() => ({ uploadMedia: vi.fn() }))
vi.mock('../lib/upload', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/upload')>()
  return { ...actual, uploadMedia: mocks.uploadMedia }
})

import { useMediaPasteUpload } from './useMediaPasteUpload'

function Editor({ onError, target = { taskId: 5 } }: { onError: (m: string) => void; target?: { taskId: number } | null }) {
  const [value, setValue] = useState('Before  after')
  const paste = useMediaPasteUpload({ target, value, setValue, onError })
  return (
    <>
      <textarea aria-label="editor" value={value} onChange={(e) => setValue(e.target.value)} onPaste={paste.onPaste} />
      <span>{paste.isUploading ? 'uploading' : 'idle'}</span>
    </>
  )
}

function pasteImage(el: HTMLElement, types: string[] = ['Files']) {
  const file = new File(['x'], 'image.png', { type: 'image/png' })
  fireEvent.paste(el, {
    clipboardData: {
      types,
      items: [{ kind: 'file', type: 'image/png', getAsFile: () => file }],
      files: [file],
    },
  })
}

describe('useMediaPasteUpload', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('inserts the uploaded screenshot at the cursor', async () => {
    let finish: (v: { url: string; publicId: string; altName: string }) => void = () => {}
    mocks.uploadMedia.mockReturnValue(new Promise((r) => { finish = r }))
    render(<Editor onError={vi.fn()} />)
    const editor = screen.getByLabelText('editor') as HTMLTextAreaElement
    editor.setSelectionRange(7, 7)

    pasteImage(editor)

    await waitFor(() => expect(editor.value).toMatch(/^Before !\[Uploading Screenshot .+…\]\(uploading-\d+\) after$/))
    expect(screen.getByText('uploading')).toBeInTheDocument()

    finish({ url: 'https://cdn/s.png', publicId: 'p', altName: 'Screenshot' })
    await waitFor(() => expect(editor.value).toBe('Before ![Screenshot](https://cdn/s.png) after'))
    expect(screen.getByText('idle')).toBeInTheDocument()
  })

  it('removes the placeholder and reports the error when the upload fails', async () => {
    mocks.uploadMedia.mockRejectedValue(new Error('Cloudinary rejected "x.png": File size too large'))
    const onError = vi.fn()
    render(<Editor onError={onError} />)
    const editor = screen.getByLabelText('editor') as HTMLTextAreaElement
    editor.setSelectionRange(7, 7)

    pasteImage(editor)

    await waitFor(() => expect(onError).toHaveBeenCalledWith('Cloudinary rejected "x.png": File size too large'))
    expect(editor.value).toBe('Before  after')
  })

  it('leaves normal text pastes alone', () => {
    render(<Editor onError={vi.fn()} />)
    pasteImage(screen.getByLabelText('editor'), ['text/plain', 'Files'])
    expect(mocks.uploadMedia).not.toHaveBeenCalled()
  })
})
