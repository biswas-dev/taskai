import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import ProjectModal from './ProjectModal'

// Mock the API module
vi.mock('../lib/api', () => ({
  api: {
    createProject: vi.fn(),
    listTeams: vi.fn().mockResolvedValue([]),
  },
}))

// Import after mock so we get the mocked version
import { api } from '../lib/api'
import { pickOption } from '../test/select'

const mockedCreateProject = vi.mocked(api.createProject)

describe('ProjectModal', () => {
  const defaultProps = {
    isOpen: true,
    onClose: vi.fn(),
    onProjectCreated: vi.fn(),
  }

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns null when isOpen is false', () => {
    const { container } = render(
      <ProjectModal {...defaultProps} isOpen={false} />
    )

    expect(container.innerHTML).toBe('')
  })

  it('renders modal with form when isOpen is true', () => {
    render(<ProjectModal {...defaultProps} />)

    expect(screen.getByText('Create New Project')).toBeInTheDocument()
    expect(screen.getByLabelText(/project name/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/description/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /create project/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /cancel/i })).toBeInTheDocument()
  })

  it('shows validation error when submitting whitespace-only name', async () => {
    const user = userEvent.setup()
    render(<ProjectModal {...defaultProps} />)

    // Type a space to bypass the HTML required attribute, but fail the trim check
    await user.type(screen.getByLabelText(/project name/i), ' ')
    await user.click(screen.getByRole('button', { name: /create project/i }))

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent('Project name is required')
    })
    expect(mockedCreateProject).not.toHaveBeenCalled()
  })

  it('calls api.createProject on valid submit', async () => {
    const user = userEvent.setup()
    const mockProject = { id: 1, name: 'Test Project', owner_id: 1, created_at: '2024-01-01T00:00:00Z' }
    mockedCreateProject.mockResolvedValue(mockProject)

    render(<ProjectModal {...defaultProps} />)

    await user.type(screen.getByLabelText(/project name/i), 'Test Project')
    await user.type(screen.getByLabelText(/description/i), 'A test description')
    await user.click(screen.getByRole('button', { name: /create project/i }))

    await waitFor(() => {
      expect(mockedCreateProject).toHaveBeenCalledWith({
        name: 'Test Project',
        description: 'A test description',
      })
    })
  })

  it('calls onProjectCreated and onClose after successful creation', async () => {
    const user = userEvent.setup()
    const mockProject = { id: 1, name: 'New Project', owner_id: 1, created_at: '2024-01-01T00:00:00Z' }
    mockedCreateProject.mockResolvedValue(mockProject)

    render(<ProjectModal {...defaultProps} />)

    await user.type(screen.getByLabelText(/project name/i), 'New Project')
    await user.click(screen.getByRole('button', { name: /create project/i }))

    await waitFor(() => {
      expect(defaultProps.onProjectCreated).toHaveBeenCalledWith(mockProject)
    })
    expect(defaultProps.onClose).toHaveBeenCalled()
  })

  it('shows error on API failure', async () => {
    const user = userEvent.setup()
    mockedCreateProject.mockRejectedValue(new Error('Server error'))

    render(<ProjectModal {...defaultProps} />)

    await user.type(screen.getByLabelText(/project name/i), 'Failing Project')
    await user.click(screen.getByRole('button', { name: /create project/i }))

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent('Server error')
    })
    expect(defaultProps.onProjectCreated).not.toHaveBeenCalled()
    expect(defaultProps.onClose).not.toHaveBeenCalled()
  })

  it('shows generic error when API throws non-Error object', async () => {
    const user = userEvent.setup()
    mockedCreateProject.mockRejectedValue('unknown error')

    render(<ProjectModal {...defaultProps} />)

    await user.type(screen.getByLabelText(/project name/i), 'Some Project')
    await user.click(screen.getByRole('button', { name: /create project/i }))

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent('Failed to create project')
    })
  })

  it('close button calls onClose', async () => {
    const user = userEvent.setup()
    render(<ProjectModal {...defaultProps} />)

    await user.click(screen.getByLabelText(/close modal/i))

    expect(defaultProps.onClose).toHaveBeenCalled()
  })

  it('clicking backdrop calls onClose', async () => {
    const user = userEvent.setup()
    render(<ProjectModal {...defaultProps} />)

    // The backdrop is the first div inside the fixed container
    const backdrop = screen.getByText('Create New Project')
      .closest('.fixed.inset-0.z-50')!
      .querySelector('.fixed.inset-0.bg-black')!

    await user.click(backdrop)

    expect(defaultProps.onClose).toHaveBeenCalled()
  })

  it('cancel button calls onClose', async () => {
    const user = userEvent.setup()
    render(<ProjectModal {...defaultProps} />)

    await user.click(screen.getByRole('button', { name: /cancel/i }))

    expect(defaultProps.onClose).toHaveBeenCalled()
  })

  it('trims whitespace from name before validation', async () => {
    const user = userEvent.setup()
    render(<ProjectModal {...defaultProps} />)

    await user.type(screen.getByLabelText(/project name/i), '   ')
    await user.click(screen.getByRole('button', { name: /create project/i }))

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent('Project name is required')
    })
    expect(mockedCreateProject).not.toHaveBeenCalled()
  })

  it('sends undefined description when description is empty', async () => {
    const user = userEvent.setup()
    const mockProject = { id: 1, name: 'No Desc', owner_id: 1, created_at: '2024-01-01T00:00:00Z' }
    mockedCreateProject.mockResolvedValue(mockProject)

    render(<ProjectModal {...defaultProps} />)

    await user.type(screen.getByLabelText(/project name/i), 'No Desc')
    await user.click(screen.getByRole('button', { name: /create project/i }))

    await waitFor(() => {
      expect(mockedCreateProject).toHaveBeenCalledWith({
        name: 'No Desc',
        description: undefined,
      })
    })
  })

  it('lets people in several teams pick the project team', async () => {
    const user = userEvent.setup()
    vi.mocked(api.listTeams).mockResolvedValueOnce([
      { id: 1, name: 'Elastio', owner_id: 1, role: 'owner', is_owner: true, is_home: true, member_count: 5, project_count: 3 },
      { id: 2, name: 'Intelliviz', owner_id: 1, role: 'owner', is_owner: true, is_home: false, member_count: 3, project_count: 0 },
    ])
    mockedCreateProject.mockResolvedValue({ id: 9, name: 'Dashboard', owner_id: 1, created_at: '2024-01-01T00:00:00Z' })

    render(<ProjectModal {...defaultProps} />)

    const teamSelect = await screen.findByLabelText('Team')
    await waitFor(() => expect(teamSelect).toHaveTextContent('Elastio'))
    await pickOption(user, teamSelect, 'Intelliviz')
    await user.type(screen.getByLabelText(/project name/i), 'Dashboard')
    await user.click(screen.getByRole('button', { name: /create project/i }))

    await waitFor(() => {
      expect(mockedCreateProject).toHaveBeenCalledWith({ name: 'Dashboard', description: undefined, team_id: 2 })
    })
  })

  it('hides the team picker for single-team users', async () => {
    render(<ProjectModal {...defaultProps} />)
    await waitFor(() => expect(api.listTeams).toHaveBeenCalled())
    expect(screen.queryByLabelText('Team')).not.toBeInTheDocument()
  })
})
