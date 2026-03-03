import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import Tags from './Tags'

const mockNavigate = vi.fn()
vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: () => ({ projectId: '1' }),
}))

vi.mock('../components/ui/FormError', () => ({
  default: ({ message }: { message: string }) => message ? <div role="alert">{message}</div> : null,
}))

const mocks = vi.hoisted(() => ({
  getTags: vi.fn(),
  createTag: vi.fn(),
  updateTag: vi.fn(),
  deleteTag: vi.fn(),
  getProjects: vi.fn().mockResolvedValue([]),
  shareTag: vi.fn(),
  unshareTag: vi.fn(),
}))

vi.mock('../lib/api', () => ({
  apiClient: mocks,
}))

const tags = [
  { id: 1, name: 'bug', color: '#EF4444', created_at: '2024-01-01T00:00:00Z' },
  { id: 2, name: 'feature', color: '#3B82F6', created_at: '2024-01-02T00:00:00Z' },
]

describe('Tags', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getTags.mockResolvedValue(tags)
  })

  it('renders heading and description', async () => {
    render(<Tags />)
    await waitFor(() => {
      expect(screen.getByRole('heading', { level: 1, name: 'Tags' })).toBeInTheDocument()
      expect(screen.getByText('Label and categorize your tasks')).toBeInTheDocument()
    })
  })

  it('displays tags after loading', async () => {
    render(<Tags />)
    await waitFor(() => {
      expect(screen.getByText('bug')).toBeInTheDocument()
      expect(screen.getByText('feature')).toBeInTheDocument()
    })
  })

  it('shows empty state when no tags', async () => {
    mocks.getTags.mockResolvedValue([])
    render(<Tags />)
    await waitFor(() => {
      expect(screen.getByText('No tags yet')).toBeInTheDocument()
    })
  })

  it('opens create form on New Tag click', async () => {
    const user = userEvent.setup()
    render(<Tags />)
    await waitFor(() => {
      expect(screen.getByText('bug')).toBeInTheDocument()
    })

    const btn = screen.getAllByRole('button').find(b => b.textContent?.includes('New Tag'))!
    await user.click(btn)
    expect(screen.getByRole('heading', { level: 3, name: /New Tag/ })).toBeInTheDocument()
  })

  it('validates tag name on submit', async () => {
    const user = userEvent.setup()
    render(<Tags />)
    await waitFor(() => {
      expect(screen.getByText('bug')).toBeInTheDocument()
    })

    const btn = screen.getAllByRole('button').find(b => b.textContent?.includes('New Tag'))!
    await user.click(btn)
    // Type whitespace to bypass HTML required attribute, triggering custom validation
    const nameInput = screen.getByPlaceholderText('bug, feature, urgent...')
    await user.type(nameInput, '   ')
    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(screen.getByText('Tag name is required')).toBeInTheDocument()
    })
    expect(mocks.createTag).not.toHaveBeenCalled()
  })

  it('creates a tag successfully', async () => {
    mocks.createTag.mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<Tags />)
    await waitFor(() => {
      expect(screen.getByText('bug')).toBeInTheDocument()
    })

    const btn = screen.getAllByRole('button').find(b => b.textContent?.includes('New Tag'))!
    await user.click(btn)
    const nameInput = screen.getByPlaceholderText('bug, feature, urgent...')
    await user.type(nameInput, 'urgent')
    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(mocks.createTag).toHaveBeenCalledWith(
        1,
        expect.objectContaining({ name: 'urgent' })
      )
    })
  })

  it('opens edit form with pre-filled data', async () => {
    const user = userEvent.setup()
    render(<Tags />)
    await waitFor(() => {
      expect(screen.getByText('bug')).toBeInTheDocument()
    })

    const editButtons = screen.getAllByTitle('Edit')
    await user.click(editButtons[0])

    expect(screen.getByRole('heading', { level: 3, name: /Edit Tag/ })).toBeInTheDocument()
    expect(screen.getByDisplayValue('bug')).toBeInTheDocument()
  })

  it('updates a tag', async () => {
    mocks.updateTag.mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<Tags />)
    await waitFor(() => {
      expect(screen.getByText('bug')).toBeInTheDocument()
    })

    const editButtons = screen.getAllByTitle('Edit')
    await user.click(editButtons[0])
    await user.click(screen.getByRole('button', { name: 'Update' }))

    await waitFor(() => {
      expect(mocks.updateTag).toHaveBeenCalledWith(1, expect.any(Object))
    })
  })

  it('deletes a tag with confirmation', async () => {
    mocks.deleteTag.mockResolvedValue(undefined)
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const user = userEvent.setup()
    render(<Tags />)
    await waitFor(() => {
      expect(screen.getByText('bug')).toBeInTheDocument()
    })

    const deleteButtons = screen.getAllByTitle('Delete')
    await user.click(deleteButtons[0])

    expect(mocks.deleteTag).toHaveBeenCalledWith(1)
  })

  it('shows error on failed create', async () => {
    mocks.createTag.mockRejectedValue(new Error('Create failed'))
    const user = userEvent.setup()
    render(<Tags />)
    await waitFor(() => {
      expect(screen.getByText('bug')).toBeInTheDocument()
    })

    const btn = screen.getAllByRole('button').find(b => b.textContent?.includes('New Tag'))!
    await user.click(btn)
    const nameInput = screen.getByPlaceholderText('bug, feature, urgent...')
    await user.type(nameInput, 'urgent')
    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(screen.getByText('Create failed')).toBeInTheDocument()
    })
  })

  it('navigates back on Back button click', async () => {
    const user = userEvent.setup()
    render(<Tags />)
    await waitFor(() => {
      expect(screen.getByText('bug')).toBeInTheDocument()
    })
    await user.click(screen.getByText('Back'))
    expect(mockNavigate).toHaveBeenCalledWith('/app/projects/1')
  })

  it('cancel button hides the form', async () => {
    const user = userEvent.setup()
    render(<Tags />)
    await waitFor(() => {
      expect(screen.getByText('bug')).toBeInTheDocument()
    })

    const btn = screen.getAllByRole('button').find(b => b.textContent?.includes('New Tag'))!
    await user.click(btn)
    expect(screen.getByRole('heading', { level: 3, name: /New Tag/ })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('heading', { level: 3, name: /New Tag/ })).not.toBeInTheDocument()
  })
})
