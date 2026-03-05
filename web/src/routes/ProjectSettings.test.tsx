import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import ProjectSettings from './ProjectSettings'

const mockNavigate = vi.fn()
vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: () => ({ projectId: '42' }),
}))

vi.mock('../components/ui/FormError', () => ({
  default: ({ message }: { message: string }) => message ? <div role="alert">{message}</div> : null,
}))

const mocks = vi.hoisted(() => ({
  getProject: vi.fn(),
  getProjectMembers: vi.fn(),
  getTeamMembers: vi.fn(),
  addProjectMember: vi.fn(),
  updateProjectMember: vi.fn(),
  removeProjectMember: vi.fn(),
  getProjectGitHub: vi.fn(),
  updateProjectGitHub: vi.fn(),
  getSwimLanes: vi.fn(),
  createSwimLane: vi.fn(),
  updateSwimLane: vi.fn(),
  deleteSwimLane: vi.fn(),
  getStorageUsage: vi.fn(),
  getProjectInvitations: vi.fn(),
  githubGetMappings: vi.fn(),
  githubSaveMappings: vi.fn(),
}))

vi.mock('../lib/api', () => ({
  apiClient: mocks,
}))

vi.mock('../state/AuthContext', () => ({
  useAuth: () => ({ user: { id: 10, email: 'alice@test.com' } }),
}))

const members = [
  {
    id: 1,
    user_id: 10,
    email: 'alice@test.com',
    role: 'owner',
    granted_by: 1,
    granted_at: '2024-01-01T00:00:00Z',
  },
]

const teamMembers = [
  { id: 1, user_id: 10, email: 'alice@test.com', role: 'admin' },
  { id: 2, user_id: 20, email: 'bob@test.com', role: 'member' },
]

const swimLanes = [
  { id: 1, project_id: 42, name: 'To Do', color: '#6B7280', position: 0, status_category: 'todo' as const, created_at: '', updated_at: '' },
  { id: 2, project_id: 42, name: 'In Progress', color: '#3B82F6', position: 1, status_category: 'in_progress' as const, created_at: '', updated_at: '' },
  { id: 3, project_id: 42, name: 'Done', color: '#10B981', position: 2, status_category: 'done' as const, created_at: '', updated_at: '' },
]

describe('ProjectSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getProject.mockResolvedValue({
      id: 42,
      name: 'Test Project',
      description: 'Test project description',
      team_id: 1,
      created_by: 1,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
    })
    mocks.getProjectMembers.mockResolvedValue(members)
    mocks.getTeamMembers.mockResolvedValue(teamMembers)
    mocks.getProjectGitHub.mockResolvedValue({
      github_repo_url: '',
      github_owner: '',
      github_repo_name: '',
      github_branch: 'main',
      github_sync_enabled: false,
      github_last_sync: null,
      github_token_set: false,
      github_login: null,
    })
    mocks.getSwimLanes.mockResolvedValue(swimLanes)
    mocks.getStorageUsage.mockResolvedValue([])
    mocks.getProjectInvitations.mockResolvedValue([])
    mocks.githubGetMappings.mockResolvedValue({ status_mappings: {}, user_mappings: {} })
  })

  it('renders page heading', async () => {
    render(<ProjectSettings />)
    await waitFor(() => {
      expect(screen.getByText('Test Project')).toBeInTheDocument()
    })
  })

  it('navigates back to project on back button', async () => {
    const user = userEvent.setup()
    render(<ProjectSettings />)
    await waitFor(() => {
      expect(screen.getByText('Test Project')).toBeInTheDocument()
    })
    // Click the back button (first button with "Settings" text, which has the arrow icon)
    const buttons = screen.getAllByText('Settings')
    await user.click(buttons[0])
    expect(mockNavigate).toHaveBeenCalledWith('/app/projects/42')
  })

  describe('Team Members', () => {
    it('displays current members', async () => {
      render(<ProjectSettings />)
      await waitFor(() => {
        expect(screen.getByText('alice@test.com')).toBeInTheDocument()
        expect(screen.getByText('Current Members (1)')).toBeInTheDocument()
      })
    })

    it('shows no members state', async () => {
      mocks.getProjectMembers.mockResolvedValue([])
      render(<ProjectSettings />)
      await waitFor(() => {
        expect(screen.getByText('No members added yet')).toBeInTheDocument()
      })
    })
  })

  describe('Swim Lanes', () => {
    it('displays current swim lanes', async () => {
      render(<ProjectSettings />)
      await waitFor(() => {
        expect(screen.getAllByText('To Do').length).toBeGreaterThanOrEqual(1)
        expect(screen.getAllByText('In Progress').length).toBeGreaterThanOrEqual(1)
        expect(screen.getAllByText('Done').length).toBeGreaterThanOrEqual(1)
        expect(screen.getByText('Current Swim Lanes (3)')).toBeInTheDocument()
      })
    })

    it('creates a new swim lane', async () => {
      mocks.createSwimLane.mockResolvedValue(undefined)
      const user = userEvent.setup()
      render(<ProjectSettings />)

      await waitFor(() => {
        expect(screen.getByText('Add New Swim Lane')).toBeInTheDocument()
      })

      const nameInput = screen.getByPlaceholderText('e.g., In Review, Testing')
      await user.type(nameInput, 'Review')
      await user.click(screen.getByText('Add Swim Lane'))

      await waitFor(() => {
        expect(mocks.createSwimLane).toHaveBeenCalledWith(42, {
          name: 'Review',
          color: '#6B7280',
          position: 3,
          status_category: 'todo',
        })
      })
    })

    it('disables Add Swim Lane button when name is empty', async () => {
      render(<ProjectSettings />)

      await waitFor(() => {
        expect(screen.getByText('Add Swim Lane')).toBeInTheDocument()
      })

      // Button should be disabled when no name is entered
      const addBtn = screen.getByText('Add Swim Lane').closest('button')!
      expect(addBtn).toBeDisabled()
    })

    it('prevents creating more than 6 swim lanes', async () => {
      const sixLanes = Array.from({ length: 6 }, (_, i) => ({
        id: i + 1,
        project_id: 42,
        name: `Lane ${i + 1}`,
        color: '#6B7280',
        position: i,
        created_at: '',
        updated_at: '',
      }))
      mocks.getSwimLanes.mockResolvedValue(sixLanes)

      render(<ProjectSettings />)

      await waitFor(() => {
        expect(screen.getByText('Current Swim Lanes (6)')).toBeInTheDocument()
      })

      expect(screen.queryByText('Add New Swim Lane')).not.toBeInTheDocument()
    })

    it('prevents deleting when only 2 lanes remain', async () => {
      const twoLanes = swimLanes.slice(0, 2)
      mocks.getSwimLanes.mockResolvedValue(twoLanes)

      render(<ProjectSettings />)

      await waitFor(() => {
        expect(screen.getByText('Current Swim Lanes (2)')).toBeInTheDocument()
      })

      const deleteButtons = screen.getAllByTitle('Delete')
      deleteButtons.forEach(btn => {
        expect(btn).toBeDisabled()
      })
    })

    it('edits a swim lane', async () => {
      const user = userEvent.setup()
      render(<ProjectSettings />)

      await waitFor(() => {
        expect(screen.getByText('To Do')).toBeInTheDocument()
      })

      const editButtons = screen.getAllByTitle('Edit')
      await user.click(editButtons[0])

      const editInputs = screen.getAllByDisplayValue('To Do')
      expect(editInputs.length).toBeGreaterThanOrEqual(1)
      expect(screen.getByText('Save')).toBeInTheDocument()
    })
  })

  describe('Storage Usage', () => {
    it('shows empty storage state', async () => {
      render(<ProjectSettings />)
      await waitFor(() => {
        expect(screen.getByText('No files uploaded yet')).toBeInTheDocument()
      })
    })

    it('displays storage usage data', async () => {
      mocks.getStorageUsage.mockResolvedValue([
        { user_id: 1, user_name: 'Alice', file_count: 5, total_size: 1048576 },
      ])

      render(<ProjectSettings />)
      await waitFor(() => {
        expect(screen.getByText('Alice')).toBeInTheDocument()
      }, { timeout: 3000 })
      // "1.0 MB" appears in both total summary and per-user row
      const mbTexts = screen.getAllByText('1.0 MB')
      expect(mbTexts.length).toBeGreaterThanOrEqual(1)
    })
  })

  describe('GitHub Integration', () => {
    it('renders GitHub settings form', async () => {
      render(<ProjectSettings />)
      await waitFor(() => {
        expect(screen.getByText('GitHub Integration')).toBeInTheDocument()
        expect(screen.getByText('Connect with GitHub')).toBeInTheDocument()
      })
    })
  })
})
