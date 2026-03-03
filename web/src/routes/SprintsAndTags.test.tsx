import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import SprintsAndTags from './SprintsAndTags'

const mockNavigate = vi.fn()
vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: () => ({ projectId: '1' }),
}))

vi.mock('../components/ui/FormError', () => ({
  default: ({ message }: { message: string }) => message ? <div role="alert">{message}</div> : null,
}))

const mocks = vi.hoisted(() => ({
  getSprints: vi.fn(),
  createSprint: vi.fn(),
  updateSprint: vi.fn(),
  deleteSprint: vi.fn(),
  getTags: vi.fn(),
  createTag: vi.fn(),
  updateTag: vi.fn(),
  deleteTag: vi.fn(),
}))

vi.mock('../lib/api', () => ({
  apiClient: mocks,
}))

const sprints = [
  {
    id: 1,
    name: 'Sprint Alpha',
    goal: 'Ship MVP',
    start_date: '2024-01-01',
    end_date: '2024-01-14',
    status: 'active',
    created_at: '2024-01-01T00:00:00Z',
  },
]

const tags = [
  { id: 1, name: 'critical', color: '#DC2626', created_at: '2024-01-01T00:00:00Z' },
]

describe('SprintsAndTags', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getSprints.mockResolvedValue(sprints)
    mocks.getTags.mockResolvedValue(tags)
  })

  it('renders page heading', async () => {
    render(<SprintsAndTags />)
    await waitFor(() => {
      expect(screen.getByText('Sprints & Tags')).toBeInTheDocument()
    })
  })

  it('displays both sprints and tags sections', async () => {
    render(<SprintsAndTags />)
    await waitFor(() => {
      expect(screen.getByText('Sprint Alpha')).toBeInTheDocument()
      expect(screen.getByText('critical')).toBeInTheDocument()
    })
  })

  it('shows sprint goal and status', async () => {
    render(<SprintsAndTags />)
    await waitFor(() => {
      expect(screen.getByText('Ship MVP')).toBeInTheDocument()
      expect(screen.getByText('active')).toBeInTheDocument()
    })
  })

  it('shows empty states when no data', async () => {
    mocks.getSprints.mockResolvedValue([])
    mocks.getTags.mockResolvedValue([])
    render(<SprintsAndTags />)
    await waitFor(() => {
      expect(screen.getByText('No sprints yet')).toBeInTheDocument()
      expect(screen.getByText('No tags yet')).toBeInTheDocument()
    })
  })

  it('creates a sprint from the combined page', async () => {
    mocks.createSprint.mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<SprintsAndTags />)
    await waitFor(() => {
      expect(screen.getByText('Sprint Alpha')).toBeInTheDocument()
    })

    const newSprintBtns = screen.getAllByRole('button').filter(b => b.textContent?.includes('New Sprint'))
    await user.click(newSprintBtns[0])

    const nameInput = screen.getByPlaceholderText('Sprint 1')
    await user.type(nameInput, 'Sprint Beta')
    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(mocks.createSprint).toHaveBeenCalledWith(
        1,
        expect.objectContaining({ name: 'Sprint Beta' })
      )
    })
  })

  it('creates a tag from the combined page', async () => {
    mocks.createTag.mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<SprintsAndTags />)
    await waitFor(() => {
      expect(screen.getByText('critical')).toBeInTheDocument()
    })

    const newTagBtns = screen.getAllByRole('button').filter(b => b.textContent?.includes('New Tag'))
    await user.click(newTagBtns[0])

    const nameInput = screen.getByPlaceholderText('bug, feature, urgent...')
    await user.type(nameInput, 'low-priority')
    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(mocks.createTag).toHaveBeenCalledWith(
        1,
        expect.objectContaining({ name: 'low-priority' })
      )
    })
  })

  it('validates sprint name before creating', async () => {
    const user = userEvent.setup()
    render(<SprintsAndTags />)
    await waitFor(() => {
      expect(screen.getByText('Sprint Alpha')).toBeInTheDocument()
    })

    const newSprintBtns = screen.getAllByRole('button').filter(b => b.textContent?.includes('New Sprint'))
    await user.click(newSprintBtns[0])

    // Type whitespace to bypass HTML required attribute, triggering custom validation
    const nameInput = screen.getByPlaceholderText('Sprint 1')
    await user.type(nameInput, '   ')
    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(screen.getByText('Sprint name is required')).toBeInTheDocument()
    })
  })

  it('validates tag name before creating', async () => {
    const user = userEvent.setup()
    render(<SprintsAndTags />)
    await waitFor(() => {
      expect(screen.getByText('critical')).toBeInTheDocument()
    })

    const newTagBtns = screen.getAllByRole('button').filter(b => b.textContent?.includes('New Tag'))
    await user.click(newTagBtns[0])

    // Type whitespace to bypass HTML required attribute, triggering custom validation
    const nameInput = screen.getByPlaceholderText('bug, feature, urgent...')
    await user.type(nameInput, '   ')
    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(screen.getByText('Tag name is required')).toBeInTheDocument()
    })
  })

  it('deletes a sprint with confirmation', async () => {
    mocks.deleteSprint.mockResolvedValue(undefined)
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const user = userEvent.setup()
    render(<SprintsAndTags />)
    await waitFor(() => {
      expect(screen.getByText('Sprint Alpha')).toBeInTheDocument()
    })

    const deleteButtons = screen.getAllByTitle('Delete')
    await user.click(deleteButtons[0])
    expect(mocks.deleteSprint).toHaveBeenCalledWith(1)
  })

  it('navigates back on Back button', async () => {
    const user = userEvent.setup()
    render(<SprintsAndTags />)
    await waitFor(() => {
      expect(screen.getByText('Back')).toBeInTheDocument()
    })
    await user.click(screen.getByText('Back'))
    expect(mockNavigate).toHaveBeenCalledWith('/app')
  })
})
