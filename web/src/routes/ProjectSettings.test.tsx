import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import ProjectSettings from './ProjectSettings'
import { DialogProvider } from '../state/DialogContext'
import { pickOption } from '../test/select'

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
  getCollaborators: vi.fn(),
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
  listTeams: vi.fn(),
  updateProject: vi.fn(),
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

const collaborators = [
  { user_id: 20, email: 'bob@test.com', team_id: 1, team_name: 'Intelliviz' },
]

const teams = [
  { id: 1, name: 'Intelliviz', owner_id: 10, role: 'owner', is_owner: true, is_home: false, member_count: 3, project_count: 1 },
  { id: 2, name: 'Elastio', owner_id: 10, role: 'owner', is_owner: true, is_home: true, member_count: 40, project_count: 5 },
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
      owner_id: 10,
      created_by: 1,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
    })
    mocks.getProjectMembers.mockResolvedValue(members)
    mocks.getCollaborators.mockResolvedValue(collaborators)
    mocks.getProjectGitHub.mockResolvedValue({
      github_repo_url: '',
      github_owner: '',
      github_repo_name: '',
      github_branch: 'main',
      github_sync_enabled: false,
      github_last_sync: null,
      github_token_set: false,
      github_login: null,
      github_project_url: '',
    })
    mocks.getSwimLanes.mockResolvedValue(swimLanes)
    mocks.getStorageUsage.mockResolvedValue([])
    mocks.getProjectInvitations.mockResolvedValue([])
    mocks.githubGetMappings.mockResolvedValue({ status_mappings: {}, user_mappings: {} })
    mocks.listTeams.mockResolvedValue(teams)
  })

  describe('Teams', () => {
    const renderWithDialogs = () => render(<DialogProvider><ProjectSettings /></DialogProvider>)

    it("only offers members of the project's team, never the user's other teams", async () => {
      const user = userEvent.setup()
      mocks.getCollaborators.mockResolvedValue([
        { user_id: 30, email: 'thiva@tickrapi.test', team_id: 2, team_name: 'TickrAPI' },
        { user_id: 20, email: 'nakul@intelliviz.test', team_id: 1, team_name: 'Intelliviz' },
      ])
      render(<ProjectSettings />)

      const trigger = await screen.findByLabelText(/Team Member/)
      await waitFor(() => expect(trigger).toHaveTextContent('Select a member of Intelliviz...'))
      await user.click(trigger)

      const listbox = await screen.findByRole('listbox')
      expect(within(listbox).getAllByRole('option')).toHaveLength(1)
      expect(within(listbox).getByText('nakul@intelliviz.test')).toBeInTheDocument()
      expect(within(listbox).queryByText('thiva@tickrapi.test')).not.toBeInTheDocument()
    })

    it("offers the owner exactly the teams they belong to, owner or member, with the current one marked", async () => {
      const user = userEvent.setup()
      mocks.listTeams.mockResolvedValue([
        ...teams,
        { id: 3, name: 'TickrAPI', owner_id: 99, role: 'member', is_owner: false, is_home: false, member_count: 4, project_count: 2 },
      ])
      renderWithDialogs()

      const trigger = await screen.findByLabelText('Move to')
      expect(trigger.tagName).toBe('BUTTON')
      await waitFor(() => expect(trigger).toHaveTextContent('Intelliviz'))
      await user.click(trigger)

      const listbox = await screen.findByRole('listbox')
      const options = within(listbox).getAllByRole('option')
      expect(options.map((o) => o.querySelector('span span')?.textContent)).toEqual(['Intelliviz', 'Elastio', 'TickrAPI'])
      const current = within(listbox).getByRole('option', { name: /Intelliviz/ })
      expect(current).toHaveAttribute('aria-selected', 'true')
      expect(current).toHaveTextContent('Current')
      expect(within(listbox).getByRole('option', { name: /TickrAPI/ })).toHaveTextContent('You are a member')
    })

    it('asks for confirmation before moving, and does nothing when cancelled', async () => {
      const user = userEvent.setup()
      renderWithDialogs()

      const trigger = await screen.findByLabelText('Move to')
      await waitFor(() => expect(trigger).toHaveTextContent('Intelliviz'))
      await pickOption(user, trigger, /Elastio/)

      const dialog = await screen.findByRole('alertdialog')
      expect(within(dialog).getByText('Move "Test Project" to Elastio?')).toBeInTheDocument()
      expect(within(dialog).getByText(/Only members of/)).toHaveTextContent('Only members of Elastio can be invited from now on.')
      expect(within(dialog).getByText(/already a member keeps their access/)).toBeInTheDocument()
      await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))

      await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
      expect(mocks.updateProject).not.toHaveBeenCalled()
    })

    it('moves the project once the owner confirms', async () => {
      const user = userEvent.setup()
      mocks.updateProject.mockResolvedValue({ id: 42, name: 'Test Project', owner_id: 10, team_id: 2, created_at: '', updated_at: '' })
      renderWithDialogs()

      const trigger = await screen.findByLabelText('Move to')
      await waitFor(() => expect(trigger).toHaveTextContent('Intelliviz'))
      await pickOption(user, trigger, /Elastio/)
      await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Move project' }))

      await waitFor(() => {
        expect(mocks.updateProject).toHaveBeenCalledWith(42, { team_id: 2 })
      })
      expect(await screen.findByText(/Project moved to Elastio/)).toBeInTheDocument()
    })

    it('lets a co-owner with the Owner role move the project, listing their own teams', async () => {
      const user = userEvent.setup()
      mocks.getProject.mockResolvedValue({
        id: 42, name: 'Test Project', team_id: 1, owner_id: 77, created_at: '', updated_at: '',
      })
      mocks.listTeams.mockResolvedValue([
        teams[0],
        { id: 3, name: 'TickrAPI', owner_id: 99, role: 'member', is_owner: false, is_home: false, member_count: 4, project_count: 2 },
      ])
      mocks.updateProject.mockResolvedValue({ id: 42, name: 'Test Project', owner_id: 77, team_id: 3, created_at: '', updated_at: '' })
      renderWithDialogs()

      const trigger = await screen.findByLabelText('Move to')
      await waitFor(() => expect(trigger).toHaveTextContent('Intelliviz'))
      await user.click(trigger)
      const listbox = await screen.findByRole('listbox')
      expect(within(listbox).getAllByRole('option')).toHaveLength(2)
      await user.click(within(listbox).getByRole('option', { name: /TickrAPI/ }))
      await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Move project' }))

      await waitFor(() => expect(mocks.updateProject).toHaveBeenCalledWith(42, { team_id: 3 }))
    })

    it.each(['editor', 'member', 'viewer'])('shows the team read-only to a %s', async (role) => {
      mocks.getProject.mockResolvedValue({
        id: 42, name: 'Test Project', team_id: 1, owner_id: 77, created_at: '', updated_at: '',
      })
      mocks.getProjectMembers.mockResolvedValue([{ ...members[0], role }])
      renderWithDialogs()

      expect(await screen.findByText('Intelliviz')).toBeInTheDocument()
      await waitFor(() => expect(mocks.listTeams).toHaveBeenCalled())
      // Members have loaded, so the role check has run.
      expect(await screen.findByText('alice@test.com')).toBeInTheDocument()
      expect(screen.queryByLabelText('Move to')).not.toBeInTheDocument()
      expect(screen.queryByText('Move to')).not.toBeInTheDocument()
    })

    it('uses the modern select for invite role and member roles', async () => {
      const user = userEvent.setup()
      mocks.updateProjectMember.mockResolvedValue({})
      const { container } = render(<ProjectSettings />)

      const role = await screen.findByLabelText(/^Role/)
      expect(role).toHaveTextContent('Member')
      const memberRole = await screen.findByRole('button', { name: 'Role for alice@test.com' })
      expect(memberRole).toHaveTextContent('Owner')
      await pickOption(user, memberRole, 'Editor')

      await waitFor(() => expect(mocks.updateProjectMember).toHaveBeenCalledWith(42, 1, { role: 'editor' }))
      expect(container.querySelector('select')).toBeNull()
    })
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
