import { useState, useEffect, useRef } from 'react'
import { apiClient, type Attachment } from '../lib/api'

interface ImagePickerModalProps {
  onSelect: (alt: string, url: string, caption?: string) => void
  onClose: () => void
  taskId?: number
  wikiPageId?: number
  onUploadComplete: () => void
}

export default function ImagePickerModal({ onSelect, onClose, taskId, wikiPageId, onUploadComplete }: ImagePickerModalProps) {
  const [images, setImages] = useState<Attachment[]>([])
  const [searchQuery, setSearchQuery] = useState('')
  const [loading, setLoading] = useState(true)
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState('')
  const searchTimeout = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [pendingUploadFile, setPendingUploadFile] = useState<File | null>(null)
  const [uploadAltText, setUploadAltText] = useState('')
  const [uploadCaption, setUploadCaption] = useState('')
  const [browseCaption, setBrowseCaption] = useState('')
  const [pendingBrowseImage, setPendingBrowseImage] = useState<{ alt: string; url: string } | null>(null)

  useEffect(() => {
    loadImages()
  }, [])

  const loadImages = async (query?: string) => {
    try {
      setLoading(true)
      const result = await apiClient.getImages(query)
      setImages(result)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to load images')
    } finally {
      setLoading(false)
    }
  }

  const handleSearch = (value: string) => {
    setSearchQuery(value)
    if (searchTimeout.current) clearTimeout(searchTimeout.current)
    searchTimeout.current = setTimeout(() => {
      loadImages(value || undefined)
    }, 300)
  }

  const handleSelectFile = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file || !file.type.startsWith('image/')) {
      setError('Only images can be inserted')
      return
    }
    const defaultAlt = file.name.replace(/\.[^.]+$/, '').replace(/[-_]/g, ' ')
    setPendingUploadFile(file)
    setUploadAltText(defaultAlt)
    e.target.value = ''
  }

  const handleConfirmUploadAndInsert = async () => {
    if (!pendingUploadFile || uploadAltText.trim().length < 3) return

    try {
      setUploading(true)
      const sig = await apiClient.getUploadSignature(
        taskId ? { taskId } : { pageId: wikiPageId }
      )

      const formData = new FormData()
      formData.append('file', pendingUploadFile)
      formData.append('api_key', sig.api_key)
      formData.append('timestamp', String(sig.timestamp))
      formData.append('signature', sig.signature)
      formData.append('folder', sig.folder)
      formData.append('public_id', sig.public_id)

      const uploadRes = await fetch(
        `https://api.cloudinary.com/v1_1/${sig.cloud_name}/auto/upload`,
        { method: 'POST', body: formData }
      )

      if (!uploadRes.ok) throw new Error('Upload to Cloudinary failed')
      const uploadData = await uploadRes.json()

      const altName = uploadAltText.trim()

      if (taskId) {
        await apiClient.createTaskAttachment(taskId, {
          filename: pendingUploadFile.name,
          alt_name: altName,
          file_type: 'image',
          content_type: pendingUploadFile.type,
          file_size: pendingUploadFile.size,
          cloudinary_url: uploadData.secure_url,
          cloudinary_public_id: uploadData.public_id,
        })
      } else if (wikiPageId) {
        await apiClient.createWikiPageAttachment(wikiPageId, {
          filename: pendingUploadFile.name,
          alt_name: altName,
          file_type: 'image',
          content_type: pendingUploadFile.type,
          file_size: pendingUploadFile.size,
          cloudinary_url: uploadData.secure_url,
          cloudinary_public_id: uploadData.public_id,
        })
      }

      onUploadComplete()
      onSelect(altName, uploadData.secure_url, uploadCaption.trim() || undefined)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to upload')
    } finally {
      setUploading(false)
      setPendingUploadFile(null)
      setUploadAltText('')
      setUploadCaption('')
    }
  }

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        className="w-full max-w-2xl mx-4 bg-dark-bg-secondary rounded-xl border border-dark-border-subtle shadow-2xl overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-dark-border-subtle">
          <h3 className="text-sm font-semibold text-dark-text-primary">Insert Image</h3>
          <button
            onClick={onClose}
            className="p-1 text-dark-text-tertiary hover:text-dark-text-primary rounded transition-colors"
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Search + Upload */}
        <div className="px-5 py-3 border-b border-dark-border-subtle flex gap-2">
          <div className="relative flex-1">
            <svg className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-dark-text-tertiary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => handleSearch(e.target.value)}
              placeholder="Search images by name..."
              className="w-full pl-9 pr-3 py-2 text-sm bg-dark-bg-primary border border-dark-border-subtle text-dark-text-primary rounded-lg focus:ring-1 focus:ring-primary-500 focus:border-primary-500 outline-none placeholder-dark-text-tertiary"
              autoFocus
            />
          </div>
          <label className={`inline-flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg cursor-pointer transition-colors ${
            uploading
              ? 'bg-dark-bg-tertiary text-dark-text-tertiary cursor-not-allowed'
              : 'bg-primary-500/10 text-primary-400 hover:bg-primary-500/20'
          }`}>
            <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
            </svg>
            {uploading ? 'Uploading...' : 'Upload new'}
            <input
              type="file"
              className="hidden"
              accept="image/*"
              onChange={handleSelectFile}
              disabled={uploading}
            />
          </label>
        </div>

        {/* Alt text form for pending upload */}
        {pendingUploadFile && (
          <div className="px-5 py-3 border-b border-dark-border-subtle bg-dark-bg-primary/50">
            <div className="flex items-start gap-3">
              <img
                src={URL.createObjectURL(pendingUploadFile)}
                alt="Preview"
                className="w-16 h-16 rounded-lg border border-dark-border-subtle object-cover flex-shrink-0"
              />
              <div className="flex-1 min-w-0">
                <p className="text-xs text-dark-text-tertiary truncate mb-1">{pendingUploadFile.name}</p>
                <input
                  type="text"
                  value={uploadAltText}
                  onChange={(e) => setUploadAltText(e.target.value)}
                  placeholder="Describe this image..."
                  className="w-full px-2.5 py-1.5 text-sm bg-dark-bg-secondary border border-dark-border-subtle text-dark-text-primary rounded-md focus:ring-1 focus:ring-primary-500 focus:border-primary-500 outline-none placeholder-dark-text-tertiary"
                  autoFocus
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && uploadAltText.trim().length >= 3 && !uploading) handleConfirmUploadAndInsert()
                    if (e.key === 'Escape') { setPendingUploadFile(null); setUploadAltText(''); setUploadCaption('') }
                  }}
                />
                {uploadAltText.trim().length > 0 && uploadAltText.trim().length < 3 && (
                  <p className="text-xs text-danger-400 mt-0.5">At least 3 characters</p>
                )}
                <input
                  type="text"
                  value={uploadCaption}
                  onChange={(e) => setUploadCaption(e.target.value)}
                  placeholder="Caption (optional figcaption)"
                  className="w-full mt-1 px-2.5 py-1.5 text-sm bg-dark-bg-secondary border border-dark-border-subtle text-dark-text-primary rounded-md focus:ring-1 focus:ring-primary-500 focus:border-primary-500 outline-none placeholder-dark-text-tertiary"
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && uploadAltText.trim().length >= 3 && !uploading) handleConfirmUploadAndInsert()
                    if (e.key === 'Escape') { setPendingUploadFile(null); setUploadAltText(''); setUploadCaption('') }
                  }}
                />
              </div>
              <div className="flex gap-1.5 flex-shrink-0">
                <button
                  onClick={() => { setPendingUploadFile(null); setUploadAltText(''); setUploadCaption('') }}
                  disabled={uploading}
                  className="px-2.5 py-1.5 text-xs font-medium text-dark-text-secondary bg-dark-bg-tertiary hover:bg-dark-bg-tertiary/80 rounded-md transition-colors disabled:opacity-50"
                >
                  Cancel
                </button>
                <button
                  onClick={handleConfirmUploadAndInsert}
                  disabled={uploadAltText.trim().length < 3 || uploading}
                  className="px-2.5 py-1.5 text-xs font-medium text-white bg-primary-500 hover:bg-primary-600 rounded-md transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {uploading ? 'Uploading...' : 'Insert'}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Image Grid */}
        <div className="px-5 py-4 max-h-80 overflow-y-auto">
          {error && (
            <p className="text-sm text-danger-400 mb-3">{error}</p>
          )}
          {loading ? (
            <div className="flex items-center justify-center py-8">
              <span className="text-sm text-dark-text-tertiary animate-pulse">Loading images...</span>
            </div>
          ) : images.length === 0 ? (
            <div className="text-center py-8">
              <svg className="w-10 h-10 mx-auto text-dark-text-tertiary mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
              </svg>
              <p className="text-sm text-dark-text-tertiary">
                {searchQuery ? 'No images match your search' : 'No images yet. Upload one to get started.'}
              </p>
            </div>
          ) : (
            <>
            <div className="grid grid-cols-3 sm:grid-cols-4 gap-2">
              {images.map((img: Attachment) => (
                <button
                  key={img.id}
                  onClick={() => setPendingBrowseImage({ alt: img.alt_name || img.filename, url: img.cloudinary_url })}
                  className={`group relative border rounded-lg overflow-hidden bg-dark-bg-primary hover:border-primary-500 hover:ring-1 hover:ring-primary-500/30 transition-all ${
                    pendingBrowseImage?.url === img.cloudinary_url ? 'border-primary-500 ring-1 ring-primary-500/30' : 'border-dark-border-subtle'
                  }`}
                  title={img.alt_name || img.filename}
                >
                  <img
                    src={img.cloudinary_url}
                    alt={img.alt_name || img.filename}
                    className="w-full h-20 object-cover"
                  />
                  <div className="absolute inset-0 bg-black/0 group-hover:bg-black/30 transition-colors flex items-center justify-center">
                    <svg className="w-5 h-5 text-white opacity-0 group-hover:opacity-100 transition-opacity" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                    </svg>
                  </div>
                  <div className="px-1.5 py-1">
                    <p className="text-[10px] text-dark-text-secondary truncate">
                      {img.alt_name || img.filename}
                    </p>
                  </div>
                </button>
              ))}
            </div>

            {/* Caption input for selected browse image */}
            {pendingBrowseImage && (
              <div className="mt-3 p-3 rounded-lg border border-dark-border-subtle bg-dark-bg-primary/50">
                <div className="flex items-center gap-3">
                  <img
                    src={pendingBrowseImage.url}
                    alt={pendingBrowseImage.alt}
                    className="w-12 h-12 rounded border border-dark-border-subtle object-cover flex-shrink-0"
                  />
                  <div className="flex-1 min-w-0">
                    <p className="text-xs text-dark-text-secondary truncate mb-1">{pendingBrowseImage.alt}</p>
                    <input
                      type="text"
                      value={browseCaption}
                      onChange={(e) => setBrowseCaption(e.target.value)}
                      placeholder="Caption (optional figcaption)"
                      className="w-full px-2.5 py-1.5 text-sm bg-dark-bg-secondary border border-dark-border-subtle text-dark-text-primary rounded-md focus:ring-1 focus:ring-primary-500 focus:border-primary-500 outline-none placeholder-dark-text-tertiary"
                      autoFocus
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          onSelect(pendingBrowseImage.alt, pendingBrowseImage.url, browseCaption.trim() || undefined)
                          setPendingBrowseImage(null)
                          setBrowseCaption('')
                        }
                        if (e.key === 'Escape') { setPendingBrowseImage(null); setBrowseCaption('') }
                      }}
                    />
                  </div>
                  <div className="flex gap-1.5 flex-shrink-0">
                    <button
                      onClick={() => { setPendingBrowseImage(null); setBrowseCaption('') }}
                      className="px-2.5 py-1.5 text-xs font-medium text-dark-text-secondary bg-dark-bg-tertiary hover:bg-dark-bg-tertiary/80 rounded-md transition-colors"
                    >
                      Cancel
                    </button>
                    <button
                      onClick={() => {
                        onSelect(pendingBrowseImage.alt, pendingBrowseImage.url, browseCaption.trim() || undefined)
                        setPendingBrowseImage(null)
                        setBrowseCaption('')
                      }}
                      className="px-2.5 py-1.5 text-xs font-medium text-white bg-primary-500 hover:bg-primary-600 rounded-md transition-colors"
                    >
                      Insert
                    </button>
                  </div>
                </div>
              </div>
            )}
            </>
          )}
        </div>
      </div>
    </div>
  )
}
