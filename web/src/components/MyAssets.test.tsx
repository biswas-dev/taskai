import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import MyAssets from './MyAssets'
import { apiClient, type Asset } from '../lib/api'

// Mock the API client
vi.mock('../lib/api', () => {
  const mockClient = {
    getAssets: vi.fn(),
    updateAttachment: vi.fn(),
    deleteAttachment: vi.fn(),
  }
  return {
    apiClient: mockClient,
    default: mockClient,
  }
})

const mockedApi = vi.mocked(apiClient)

function makeAsset(overrides: Partial<Asset> = {}): Asset {
  return {
    id: 1,
    task_id: 1,
    project_id: 1,
    user_id: 1,
    filename: 'test-file.jpg',
    alt_name: 'Test Image',
    file_type: 'image',
    content_type: 'image/jpeg',
    file_size: 1024000,
    cloudinary_url: 'https://res.cloudinary.com/test/image/upload/test.jpg',
    cloudinary_public_id: 'test/test',
    created_at: '2024-01-15T10:00:00Z',
    user_name: 'Test User',
    is_owner: true,
    ...overrides,
  }
}

describe('MyAssets', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders loading state initially', () => {
    mockedApi.getAssets.mockReturnValue(new Promise(() => {})) // never resolves
    render(<MyAssets projectId={1} />)
    expect(screen.getByText('Loading assets...')).toBeInTheDocument()
  })

  it('renders asset grid after data loads', async () => {
    const assets = [
      makeAsset({ id: 1, filename: 'photo1.jpg', alt_name: 'Sunset Photo' }),
      makeAsset({ id: 2, filename: 'photo2.jpg', alt_name: 'Mountain View' }),
    ]
    mockedApi.getAssets.mockResolvedValue(assets)

    render(<MyAssets projectId={1} />)

    await waitFor(() => {
      expect(screen.getByText('Sunset Photo')).toBeInTheDocument()
      expect(screen.getByText('Mountain View')).toBeInTheDocument()
    })
  })

  it('shows empty state when no files', async () => {
    mockedApi.getAssets.mockResolvedValue([])

    render(<MyAssets projectId={1} />)

    await waitFor(() => {
      expect(screen.getByText('No files uploaded yet')).toBeInTheDocument()
    })
  })

  it('shows filter message when empty with active filters', async () => {
    mockedApi.getAssets.mockResolvedValue([])

    render(<MyAssets projectId={1} />)

    await waitFor(() => {
      expect(screen.getByText('No files uploaded yet')).toBeInTheDocument()
    })

    // Click the Images filter
    const user = userEvent.setup()
    await user.click(screen.getByText('Images'))

    // API gets called again with type filter
    mockedApi.getAssets.mockResolvedValue([])

    await waitFor(() => {
      expect(screen.getByText('No files match your filters')).toBeInTheDocument()
    })
  })

  it('search input triggers reload after debounce', async () => {
    mockedApi.getAssets.mockResolvedValue([])

    render(<MyAssets projectId={1} />)

    await waitFor(() => {
      expect(screen.getByPlaceholderText('Search by name...')).toBeInTheDocument()
    })

    const user = userEvent.setup()
    const searchInput = screen.getByPlaceholderText('Search by name...')
    await user.type(searchInput, 'sunset')

    // Wait for debounce
    await waitFor(() => {
      // Initial call + debounced call
      expect(mockedApi.getAssets).toHaveBeenCalledWith(
        1,
        expect.objectContaining({ q: 'sunset' })
      )
    }, { timeout: 1000 })
  })

  it('file type filter buttons work', async () => {
    mockedApi.getAssets.mockResolvedValue([])

    render(<MyAssets projectId={1} />)

    await waitFor(() => {
      expect(screen.getByText('Images')).toBeInTheDocument()
    })

    const user = userEvent.setup()
    await user.click(screen.getByText('Videos'))

    await waitFor(() => {
      expect(mockedApi.getAssets).toHaveBeenCalledWith(
        1,
        expect.objectContaining({ type: 'video' })
      )
    })
  })

  it('edit button shows inline input for owned assets', async () => {
    const asset = makeAsset({ is_owner: true, alt_name: 'My Photo' })
    mockedApi.getAssets.mockResolvedValue([asset])

    render(<MyAssets projectId={1} />)

    await waitFor(() => {
      expect(screen.getByText('My Photo')).toBeInTheDocument()
    })

    const user = userEvent.setup()
    await user.click(screen.getByTitle('Edit alt text'))

    // Should now show an input field
    expect(screen.getByPlaceholderText('Alt text...')).toBeInTheDocument()
  })

  it('delete shows confirmation then removes on confirm', async () => {
    const asset = makeAsset({ id: 42, is_owner: true })
    mockedApi.getAssets.mockResolvedValue([asset])
    mockedApi.deleteAttachment.mockResolvedValue(undefined)

    render(<MyAssets projectId={1} />)

    await waitFor(() => {
      expect(screen.getByTitle('Delete file')).toBeInTheDocument()
    })

    const user = userEvent.setup()

    // Click delete - should show confirmation
    await user.click(screen.getByTitle('Delete file'))
    expect(screen.getByText('Delete?')).toBeInTheDocument()

    // Confirm deletion
    await user.click(screen.getByText('Yes'))

    await waitFor(() => {
      expect(mockedApi.deleteAttachment).toHaveBeenCalledWith(42)
    })
  })

  it('shows View only badge for non-owned assets', async () => {
    const asset = makeAsset({ is_owner: false })
    mockedApi.getAssets.mockResolvedValue([asset])

    render(<MyAssets projectId={1} />)

    await waitFor(() => {
      expect(screen.getByText('View only')).toBeInTheDocument()
    })
  })

  it('hides Edit and Delete buttons for non-owned assets', async () => {
    const asset = makeAsset({ is_owner: false })
    mockedApi.getAssets.mockResolvedValue([asset])

    render(<MyAssets projectId={1} />)

    await waitFor(() => {
      expect(screen.getByText('View only')).toBeInTheDocument()
    })

    expect(screen.queryByTitle('Edit alt text')).not.toBeInTheDocument()
    expect(screen.queryByTitle('Delete file')).not.toBeInTheDocument()
  })

  it('displays file size and date correctly', async () => {
    const asset = makeAsset({
      file_size: 1048576, // 1 MB
      created_at: '2024-01-15T10:00:00Z',
    })
    mockedApi.getAssets.mockResolvedValue([asset])

    render(<MyAssets projectId={1} />)

    await waitFor(() => {
      expect(screen.getByText('1 MB')).toBeInTheDocument()
      expect(screen.getByText('Jan 15, 2024')).toBeInTheDocument()
    })
  })

  it('shows error message on API failure', async () => {
    mockedApi.getAssets.mockRejectedValue(new Error('Network error'))

    render(<MyAssets projectId={1} />)

    await waitFor(() => {
      expect(screen.getByText('Network error')).toBeInTheDocument()
    })
  })
})
