import { describe, it, expect, beforeEach, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  getUploadSignature: vi.fn(),
  createTaskAttachment: vi.fn(),
  createWikiPageAttachment: vi.fn(),
}))
vi.mock('./api', () => ({ apiClient: mocks }))

import { mediaFilesFrom, uploadMedia, mediaMarkdown } from './upload'

function dataTransfer(files: File[]): DataTransfer {
  return {
    items: files.map((f) => ({ kind: 'file', type: f.type, getAsFile: () => f })),
    files,
    types: ['Files'],
  } as unknown as DataTransfer
}

const signature = {
  signature: 'sig', timestamp: 1, cloud_name: 'demo', api_key: 'key', folder: 'taskai/p', public_id: '7_01',
}

describe('mediaFilesFrom', () => {
  it('names clipboard screenshots and drops unsupported files', () => {
    const pasted = new File(['x'], 'image.png', { type: 'image/png' })
    const text = new File(['x'], 'notes.txt', { type: 'text/plain' })
    const pdf = new File(['x'], 'spec.pdf', { type: 'application/pdf' })

    const files = mediaFilesFrom(dataTransfer([pasted, text, pdf]))

    expect(files.map((f) => f.type)).toEqual(['image/png', 'application/pdf'])
    expect(files[0].name).toMatch(/^Screenshot \d{4}-\d{2}-\d{2} \d{2}\.\d{2}\.\d{2}\.png$/)
    expect(files[1].name).toBe('spec.pdf')
  })

  it('returns nothing for an empty transfer', () => {
    expect(mediaFilesFrom(null)).toEqual([])
  })
})

describe('uploadMedia', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getUploadSignature.mockResolvedValue(signature)
    mocks.createTaskAttachment.mockResolvedValue({})
    mocks.createWikiPageAttachment.mockResolvedValue({})
  })

  it('uploads to Cloudinary and records a task attachment', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ secure_url: 'https://cdn/x.png', public_id: 'taskai/p/7_01' }), { status: 200 }),
    )
    vi.stubGlobal('fetch', fetchMock)
    const file = new File(['x'], 'bug-report.png', { type: 'image/png' })

    const media = await uploadMedia({ taskId: 7 }, file)

    expect(fetchMock).toHaveBeenCalledWith('https://api.cloudinary.com/v1_1/demo/auto/upload', expect.objectContaining({ method: 'POST' }))
    expect(mocks.createTaskAttachment).toHaveBeenCalledWith(7, expect.objectContaining({
      filename: 'bug-report.png', alt_name: 'bug report', file_type: 'image', cloudinary_public_id: 'taskai/p/7_01',
    }))
    expect(mediaMarkdown(media, file)).toBe('![bug report](https://cdn/x.png)')
    vi.unstubAllGlobals()
  })

  it("surfaces Cloudinary's reason when it rejects the file", async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: { message: 'File size too large. Got 12000000. Maximum is 10485760.' } }), { status: 400 }),
    ))
    const file = new File(['x'], 'big.png', { type: 'image/png' })

    await expect(uploadMedia({ pageId: 3 }, file)).rejects.toThrow('Cloudinary rejected "big.png": File size too large')
    expect(mocks.createWikiPageAttachment).not.toHaveBeenCalled()
    vi.unstubAllGlobals()
  })

  it('rejects unsupported file types before contacting anyone', async () => {
    await expect(uploadMedia({ taskId: 1 }, new File(['x'], 'a.zip', { type: 'application/zip' }))).rejects.toThrow('Only images')
    expect(mocks.getUploadSignature).not.toHaveBeenCalled()
  })
})
