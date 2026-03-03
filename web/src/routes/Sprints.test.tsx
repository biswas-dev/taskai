import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import Sprints from './Sprints'

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
}))

vi.mock('../lib/api', () => ({
  apiClient: mocks,
}))

const sprints = [
  {
    id: 1,
    name: 'Sprint 1',
    goal: 'Complete auth module',
    start_date: '2024-01-15',
    end_date: '2024-01-29',
    status: 'active',
    created_at: '2024-01-01T00:00:00Z',
  },
  {
    id: 2,
    name: 'Sprint 2',
    goal: '',
    start_date: '',
    end_date: '',
    status: 'planned',
    created_at: '2024-01-01T00:00:00Z',
  },
]

describe('Sprints', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getSprints.mockResolvedValue(sprints)
  })

  it('renders heading and description', async () => {
    render(<Sprints />)
    await waitFor(() => {
      expect(screen.getByRole('heading', { level: 1, name: 'Sprints' })).toBeInTheDocument()
      expect(screen.getByText('Organize tasks into time-boxed iterations')).toBeInTheDocument()
    })
  })

  it('displays sprints after loading', async () => {
    render(<Sprints />)
    await waitFor(() => {
      expect(screen.getByText('Sprint 1')).toBeInTheDocument()
      expect(screen.getByText('Sprint 2')).toBeInTheDocument()
    })
  })

  it('shows sprint goal when present', async () => {
    render(<Sprints />)
    await waitFor(() => {
      expect(screen.getByText('Complete auth module')).toBeInTheDocument()
    })
  })

  it('shows status badges', async () => {
    render(<Sprints />)
    await waitFor(() => {
      expect(screen.getByText('active')).toBeInTheDocument()
      expect(screen.getByText('planned')).toBeInTheDocument()
    })
  })

  it('shows empty state when no sprints', async () => {
    mocks.getSprints.mockResolvedValue([])
    render(<Sprints />)
    await waitFor(() => {
      expect(screen.getByText('No sprints yet')).toBeInTheDocument()
    })
  })

  it('opens create form on New Sprint click', async () => {
    const user = userEvent.setup()
    render(<Sprints />)
    await waitFor(() => {
      expect(screen.getByText('Sprint 1')).toBeInTheDocument()
    })

    const btn = screen.getAllByRole('button').find(b => b.textContent?.includes('New Sprint'))!
    await user.click(btn)
    expect(screen.getByRole('heading', { level: 3, name: /New Sprint/ })).toBeInTheDocument()
  })

  it('validates sprint name on submit', async () => {
    const user = userEvent.setup()
    render(<Sprints />)
    await waitFor(() => {
      expect(screen.getByText('Sprint 1')).toBeInTheDocument()
    })

    const btn = screen.getAllByRole('button').find(b => b.textContent?.includes('New Sprint'))!
    await user.click(btn)

    // Type whitespace to bypass HTML required attribute, then submit
    const nameInput = screen.getByPlaceholderText('Sprint 1')
    await user.type(nameInput, '   ')
    const submitBtn = screen.getByRole('button', { name: 'Create' })
    await user.click(submitBtn)

    await waitFor(() => {
      expect(screen.getByText('Sprint name is required')).toBeInTheDocument()
    })
    expect(mocks.createSprint).not.toHaveBeenCalled()
  })

  it('creates a sprint successfully', async () => {
    mocks.createSprint.mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<Sprints />)
    await waitFor(() => {
      expect(screen.getByText('Sprint 1')).toBeInTheDocument()
    })

    const btn = screen.getAllByRole('button').find(b => b.textContent?.includes('New Sprint'))!
    await user.click(btn)

    const nameInput = screen.getByPlaceholderText('Sprint 1')
    await user.type(nameInput, 'Sprint 3')

    const submitBtn = screen.getByRole('button', { name: 'Create' })
    await user.click(submitBtn)

    await waitFor(() => {
      expect(mocks.createSprint).toHaveBeenCalledWith(
        1,
        expect.objectContaining({ name: 'Sprint 3' })
      )
    })
  })

  it('opens edit form with pre-filled data', async () => {
    const user = userEvent.setup()
    render(<Sprints />)
    await waitFor(() => {
      expect(screen.getByText('Sprint 1')).toBeInTheDocument()
    })

    const editButtons = screen.getAllByTitle('Edit')
    await user.click(editButtons[0])

    expect(screen.getByRole('heading', { level: 3, name: /Edit Sprint/ })).toBeInTheDocument()
    expect(screen.getByDisplayValue('Sprint 1')).toBeInTheDocument()
  })

  it('updates a sprint', async () => {
    mocks.updateSprint.mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<Sprints />)
    await waitFor(() => {
      expect(screen.getByText('Sprint 1')).toBeInTheDocument()
    })

    const editButtons = screen.getAllByTitle('Edit')
    await user.click(editButtons[0])
    await user.click(screen.getByRole('button', { name: 'Update' }))

    await waitFor(() => {
      expect(mocks.updateSprint).toHaveBeenCalledWith(1, expect.any(Object))
    })
  })

  it('deletes a sprint with confirmation', async () => {
    mocks.deleteSprint.mockResolvedValue(undefined)
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const user = userEvent.setup()
    render(<Sprints />)
    await waitFor(() => {
      expect(screen.getByText('Sprint 1')).toBeInTheDocument()
    })

    const deleteButtons = screen.getAllByTitle('Delete')
    await user.click(deleteButtons[0])

    expect(mocks.deleteSprint).toHaveBeenCalledWith(1)
  })

  it('does not delete when confirmation is cancelled', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false)

    const user = userEvent.setup()
    render(<Sprints />)
    await waitFor(() => {
      expect(screen.getByText('Sprint 1')).toBeInTheDocument()
    })

    const deleteButtons = screen.getAllByTitle('Delete')
    await user.click(deleteButtons[0])

    expect(mocks.deleteSprint).not.toHaveBeenCalled()
  })

  it('shows error on failed save', async () => {
    mocks.createSprint.mockRejectedValue(new Error('Save failed'))
    const user = userEvent.setup()
    render(<Sprints />)
    await waitFor(() => {
      expect(screen.getByText('Sprint 1')).toBeInTheDocument()
    })

    const btn = screen.getAllByRole('button').find(b => b.textContent?.includes('New Sprint'))!
    await user.click(btn)
    const nameInput = screen.getByPlaceholderText('Sprint 1')
    await user.type(nameInput, 'Sprint 3')
    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(screen.getByText('Save failed')).toBeInTheDocument()
    })
  })

  it('navigates back on Back button click', async () => {
    const user = userEvent.setup()
    render(<Sprints />)
    await waitFor(() => {
      expect(screen.getByText('Sprint 1')).toBeInTheDocument()
    })
    await user.click(screen.getByText('Back'))
    expect(mockNavigate).toHaveBeenCalledWith('/app/projects/1')
  })

  it('cancel button hides the form', async () => {
    const user = userEvent.setup()
    render(<Sprints />)
    await waitFor(() => {
      expect(screen.getByText('Sprint 1')).toBeInTheDocument()
    })

    const btn = screen.getAllByRole('button').find(b => b.textContent?.includes('New Sprint'))!
    await user.click(btn)
    expect(screen.getByRole('heading', { level: 3, name: /New Sprint/ })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('heading', { level: 3, name: /New Sprint/ })).not.toBeInTheDocument()
  })
})
