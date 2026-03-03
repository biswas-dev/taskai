import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import AcceptTeamInvite from './AcceptTeamInvite'

// Mock react-router-dom
const mockNavigate = vi.fn()
let mockSearchParams = new URLSearchParams()
vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useSearchParams: () => [mockSearchParams],
  Link: ({ to, children }: { to: string; children: React.ReactNode }) => <a href={to}>{children}</a>,
}))

// Must use vi.hoisted to define mocks that are referenced in vi.mock factories
const mocks = vi.hoisted(() => ({
  getInvitationByToken: vi.fn(),
  acceptInvitationByToken: vi.fn(),
}))

vi.mock('../lib/api', () => ({
  apiClient: mocks,
}))

// Mock useAuth
let mockUser: { id: number; email: string; is_admin: boolean; created_at: string } | null = null

vi.mock('../state/AuthContext', () => ({
  useAuth: () => ({ user: mockUser }),
}))

describe('AcceptTeamInvite', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockSearchParams = new URLSearchParams()
    mockUser = null
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows loading state initially', () => {
    mockSearchParams = new URLSearchParams('token=abc123')
    mocks.getInvitationByToken.mockReturnValue(new Promise(() => {})) // never resolves

    render(<AcceptTeamInvite />)

    expect(screen.getByText('Loading invitation...')).toBeInTheDocument()
    expect(screen.getByText('Team Invitation')).toBeInTheDocument()
  })

  it('shows error when no token provided', async () => {
    mockSearchParams = new URLSearchParams()

    render(<AcceptTeamInvite />)

    await waitFor(() => {
      expect(screen.getByText('Unable to accept')).toBeInTheDocument()
    })
    expect(screen.getByText('No invitation token provided.')).toBeInTheDocument()
    expect(screen.getByText('Go to Dashboard')).toBeInTheDocument()
    expect(mocks.getInvitationByToken).not.toHaveBeenCalled()
  })

  it('shows invitation info for unauthenticated users (requires sign in)', async () => {
    mockSearchParams = new URLSearchParams('token=valid-token')
    mocks.getInvitationByToken.mockResolvedValue({
      invitation_id: 1,
      team_name: 'Acme Corp',
      inviter_name: 'Alice',
      invitee_email: 'bob@example.com',
      status: 'pending',
      requires_signup: false,
    })

    render(<AcceptTeamInvite />)

    await waitFor(() => {
      expect(screen.getByText(/invited you to join/)).toBeInTheDocument()
    })
    expect(screen.getByText('Alice')).toBeInTheDocument()
    expect(screen.getByText('Acme Corp')).toBeInTheDocument()
    expect(screen.getByText('Sign in to Accept')).toBeInTheDocument()

    // Verify sign-in link encodes the redirect properly
    const signInLink = screen.getByText('Sign in to Accept').closest('a')
    expect(signInLink).toHaveAttribute('href', expect.stringContaining('/login?redirect='))
    expect(signInLink).toHaveAttribute('href', expect.stringContaining('valid-token'))

    // Existing user: no signup button shown
    expect(screen.queryByText('Create Account & Join')).not.toBeInTheDocument()
  })

  it('shows signup button when requires_signup is true', async () => {
    mockSearchParams = new URLSearchParams('token=signup-token')
    mocks.getInvitationByToken.mockResolvedValue({
      invitation_id: 2,
      team_name: 'Startup Inc',
      inviter_name: 'Carol',
      invitee_email: 'dave@example.com',
      status: 'pending',
      requires_signup: true,
      invite_code: 'INV-CODE-123',
    })

    render(<AcceptTeamInvite />)

    await waitFor(() => {
      expect(screen.getByText('Create Account & Join')).toBeInTheDocument()
    })

    // Verify signup link encodes invite code, email and redirect
    const signupLink = screen.getByText('Create Account & Join').closest('a')
    expect(signupLink).toHaveAttribute('href', expect.stringContaining('/signup?code='))
    expect(signupLink).toHaveAttribute('href', expect.stringContaining('INV-CODE-123'))
    expect(signupLink).toHaveAttribute('href', expect.stringContaining('email='))
    expect(signupLink).toHaveAttribute('href', expect.stringContaining('dave%40example.com'))
    expect(signupLink).toHaveAttribute('href', expect.stringContaining('redirect='))
    expect(signupLink).toHaveAttribute('href', expect.stringContaining('signup-token'))

    // New user: no sign-in button shown
    expect(screen.queryByText('Sign in to Accept')).not.toBeInTheDocument()
  })

  it('auto-accepts when user is logged in', async () => {
    mockSearchParams = new URLSearchParams('token=auto-token')
    mockUser = { id: 1, email: 'user@example.com', is_admin: false, created_at: '2024-01-01T00:00:00Z' }

    mocks.getInvitationByToken.mockResolvedValue({
      invitation_id: 3,
      team_name: 'Dev Team',
      inviter_name: 'Eve',
      invitee_email: 'user@example.com',
      status: 'pending',
      requires_signup: false,
    })
    mocks.acceptInvitationByToken.mockResolvedValue({ message: 'Accepted' })

    render(<AcceptTeamInvite />)

    // Should auto-accept the invitation
    await waitFor(() => {
      expect(mocks.acceptInvitationByToken).toHaveBeenCalledWith('auto-token')
    })

    // Should transition to accepted state
    await waitFor(() => {
      expect(screen.getByText("You're in!")).toBeInTheDocument()
    })
    expect(screen.getByText(/Dev Team/)).toBeInTheDocument()
    expect(screen.getByText(/Redirecting.../)).toBeInTheDocument()
  })

  it('navigates to /app after successful acceptance', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })

    mockSearchParams = new URLSearchParams('token=nav-token')
    mockUser = { id: 1, email: 'user@example.com', is_admin: false, created_at: '2024-01-01T00:00:00Z' }

    mocks.getInvitationByToken.mockResolvedValue({
      invitation_id: 3,
      team_name: 'Dev Team',
      inviter_name: 'Eve',
      invitee_email: 'user@example.com',
      status: 'pending',
      requires_signup: false,
    })
    mocks.acceptInvitationByToken.mockResolvedValue({ message: 'Accepted' })

    render(<AcceptTeamInvite />)

    await waitFor(() => {
      expect(screen.getByText("You're in!")).toBeInTheDocument()
    })

    // Should navigate after 2s timeout
    vi.advanceTimersByTime(2000)
    expect(mockNavigate).toHaveBeenCalledWith('/app', { replace: true })

    vi.useRealTimers()
  })

  it('shows success/accepted state with correct team name', async () => {
    mockSearchParams = new URLSearchParams('token=success-token')
    mockUser = { id: 2, email: 'member@example.com', is_admin: false, created_at: '2024-01-01T00:00:00Z' }

    mocks.getInvitationByToken.mockResolvedValue({
      invitation_id: 4,
      team_name: 'Marketing Team',
      inviter_name: 'Frank',
      invitee_email: 'member@example.com',
      status: 'pending',
      requires_signup: false,
    })
    mocks.acceptInvitationByToken.mockResolvedValue({ message: 'Accepted' })

    render(<AcceptTeamInvite />)

    await waitFor(() => {
      expect(screen.getByText("You're in!")).toBeInTheDocument()
    })

    // Verify the team name appears in the success message
    expect(screen.getByText((_, el) => {
      return el?.textContent === "You've joined Marketing Team. Redirecting..." || false
    })).toBeInTheDocument()
  })

  it('shows error on API failure (token lookup)', async () => {
    mockSearchParams = new URLSearchParams('token=bad-token')
    mocks.getInvitationByToken.mockRejectedValue(new Error('Invitation has expired'))

    render(<AcceptTeamInvite />)

    await waitFor(() => {
      expect(screen.getByText('Unable to accept')).toBeInTheDocument()
    })
    expect(screen.getByText('Invitation has expired')).toBeInTheDocument()
    expect(screen.getByText('Go to Dashboard')).toBeInTheDocument()

    // Dashboard link should point to /app
    const dashboardLink = screen.getByText('Go to Dashboard').closest('a')
    expect(dashboardLink).toHaveAttribute('href', '/app')
  })

  it('shows fallback error message when token lookup throws non-Error', async () => {
    mockSearchParams = new URLSearchParams('token=bad-token')
    mocks.getInvitationByToken.mockRejectedValue('something unexpected')

    render(<AcceptTeamInvite />)

    await waitFor(() => {
      expect(screen.getByText('Unable to accept')).toBeInTheDocument()
    })
    expect(screen.getByText('Invalid or expired invitation link.')).toBeInTheDocument()
  })

  it('shows error on accept failure', async () => {
    mockSearchParams = new URLSearchParams('token=fail-accept')
    mockUser = { id: 3, email: 'fail@example.com', is_admin: false, created_at: '2024-01-01T00:00:00Z' }

    mocks.getInvitationByToken.mockResolvedValue({
      invitation_id: 5,
      team_name: 'Failing Team',
      inviter_name: 'Grace',
      invitee_email: 'fail@example.com',
      status: 'pending',
      requires_signup: false,
    })
    mocks.acceptInvitationByToken.mockRejectedValue(new Error('You are already a member of this team'))

    render(<AcceptTeamInvite />)

    await waitFor(() => {
      expect(screen.getByText('Unable to accept')).toBeInTheDocument()
    })
    expect(screen.getByText('You are already a member of this team')).toBeInTheDocument()
    expect(mocks.acceptInvitationByToken).toHaveBeenCalledWith('fail-accept')
  })

  it('shows fallback error message when accept throws non-Error', async () => {
    mockSearchParams = new URLSearchParams('token=fail-accept')
    mockUser = { id: 4, email: 'fail2@example.com', is_admin: false, created_at: '2024-01-01T00:00:00Z' }

    mocks.getInvitationByToken.mockResolvedValue({
      invitation_id: 6,
      team_name: 'Another Team',
      inviter_name: 'Hank',
      invitee_email: 'fail2@example.com',
      status: 'pending',
      requires_signup: false,
    })
    mocks.acceptInvitationByToken.mockRejectedValue(42)

    render(<AcceptTeamInvite />)

    await waitFor(() => {
      expect(screen.getByText('Unable to accept')).toBeInTheDocument()
    })
    expect(screen.getByText('Failed to accept invitation.')).toBeInTheDocument()
  })

  it('renders a back link to the home page', () => {
    mockSearchParams = new URLSearchParams('token=any')
    mocks.getInvitationByToken.mockReturnValue(new Promise(() => {}))

    render(<AcceptTeamInvite />)

    const backLink = screen.getByText('Back').closest('a')
    expect(backLink).toHaveAttribute('href', '/')
  })
})
