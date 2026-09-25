import type { ReactElement } from 'react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import TeamsPanel from './TeamsPanel'
import { DialogProvider } from '../state/DialogContext'
import { pickOption } from '../test/select'

vi.mock('./ui/FormError', () => ({
  default: ({ message }: { message: string }) => (message ? <div role="alert">{message}</div> : null),
}))

const mocks = vi.hoisted(() => ({
  listTeams: vi.fn(),
  getMyInvitations: vi.fn(),
  getTeamMembers: vi.fn(),
  getTeamSentInvitations: vi.fn(),
  createTeam: vi.fn(),
  moveTeamMember: vi.fn(),
  leaveTeam: vi.fn(),
  searchTeamUsers: vi.fn(),
  addTeamMember: vi.fn(),
  inviteTeamMember: vi.fn(),
}))

vi.mock('../lib/api', () => ({ apiClient: mocks }))

const elastio = { id: 1, name: 'Elastio', owner_id: 10, role: 'owner', is_owner: true, is_home: true, member_count: 2, project_count: 4 }
const intelliviz = { id: 2, name: 'Intelliviz', owner_id: 10, role: 'owner', is_owner: true, is_home: false, member_count: 3, project_count: 1 }

const membersByTeam: Record<number, unknown[]> = {
  1: [
    { id: 11, user_id: 10, email: 'me@test.com', role: 'owner' },
    { id: 12, user_id: 20, email: 'bob@elastio.test', role: 'member' },
  ],
  2: [
    { id: 21, user_id: 10, email: 'me@test.com', role: 'owner' },
    { id: 22, user_id: 30, email: 'nakul@intelliviz.test', user_name: 'Nakul Mehra', role: 'admin' },
  ],
}

const renderWithDialogs = (ui: ReactElement) => render(<DialogProvider>{ui}</DialogProvider>)

describe('TeamsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mocks.listTeams.mockResolvedValue([elastio, intelliviz])
    mocks.getMyInvitations.mockResolvedValue([])
    mocks.getTeamSentInvitations.mockResolvedValue([])
    mocks.getTeamMembers.mockImplementation(async (teamId: number) => membersByTeam[teamId] ?? [])
    mocks.searchTeamUsers.mockResolvedValue([])
  })

  it('opens on the primary team and only shows that team\'s members', async () => {
    renderWithDialogs(<TeamsPanel />)
    expect(await screen.findByText('bob@elastio.test')).toBeInTheDocument()
    expect(mocks.getTeamMembers).toHaveBeenCalledWith(1)
    expect(screen.queryByText('nakul@intelliviz.test')).not.toBeInTheDocument()
  })

  it('switches teams and loads the selected roster', async () => {
    const user = userEvent.setup()
    renderWithDialogs(<TeamsPanel />)
    await screen.findByText('bob@elastio.test')

    await user.click(screen.getByRole('button', { name: /Intelliviz/ }))

    expect(await screen.findByText('nakul@intelliviz.test')).toBeInTheDocument()
    expect(screen.queryByText('bob@elastio.test')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Intelliviz/ })).toHaveAttribute('aria-pressed', 'true')
  })

  it('creates a new team and selects it', async () => {
    const user = userEvent.setup()
    const acme = { ...intelliviz, id: 3, name: 'Acme', member_count: 1, project_count: 0 }
    mocks.createTeam.mockResolvedValue({ id: 3, name: 'Acme' })
    renderWithDialogs(<TeamsPanel />)
    await screen.findByText('bob@elastio.test')

    mocks.listTeams.mockResolvedValue([elastio, intelliviz, acme])
    await user.click(screen.getByRole('button', { name: '+ New team' }))
    await user.type(screen.getByLabelText('New team name'), 'Acme')
    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => expect(mocks.createTeam).toHaveBeenCalledWith('Acme'))
    await waitFor(() => expect(mocks.getTeamMembers).toHaveBeenCalledWith(3))
    expect(screen.getByRole('button', { name: /Acme/ })).toHaveAttribute('aria-pressed', 'true')
  })

  it('moves a member to another team', async () => {
    const user = userEvent.setup()
    mocks.moveTeamMember.mockResolvedValue({ message: 'member moved' })
    renderWithDialogs(<TeamsPanel />)
    await screen.findByText('bob@elastio.test')

    await pickOption(user, screen.getByLabelText(/Move bob@elastio.test to another team/), 'Intelliviz')

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Move bob@elastio.test from Elastio to Intelliviz? Their project access is not changed.')).toBeInTheDocument()
    expect(mocks.moveTeamMember).not.toHaveBeenCalled()
    await user.click(within(dialog).getByRole('button', { name: 'Move' }))

    await waitFor(() => expect(mocks.moveTeamMember).toHaveBeenCalledWith(1, 12, 2))
    expect(await screen.findByText('bob@elastio.test moved to Intelliviz')).toBeInTheDocument()
  })

  it('lets a non-owner leave but not manage the team', async () => {
    const user = userEvent.setup()
    mocks.listTeams.mockResolvedValue([
      { ...intelliviz, owner_id: 99, role: 'member', is_owner: false, is_home: false },
      { ...elastio, id: 5, name: 'Mine', is_home: true },
    ])
    mocks.leaveTeam.mockResolvedValue({ message: 'left team' })
    renderWithDialogs(<TeamsPanel />)

    const group = await screen.findByRole('group', { name: 'Select a team' })
    await user.click(within(group).getByRole('button', { name: /Intelliviz/ }))

    expect(await screen.findByText('nakul@intelliviz.test')).toBeInTheDocument()
    expect(screen.queryByLabelText(/Add someone to/)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Remove' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Leave team' }))
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Leave' }))
    await waitFor(() => expect(mocks.leaveTeam).toHaveBeenCalledWith(2))
  })
})
