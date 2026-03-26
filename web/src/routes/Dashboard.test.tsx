import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import Dashboard from './Dashboard'

// Mock child components
vi.mock('../components/Sidebar', () => ({
  default: (props: Record<string, unknown>) => (
    <div data-testid="sidebar" data-open={String(props.isOpen)} />
  ),
}))
vi.mock('../components/ProjectModal', () => ({
  default: () => <div data-testid="project-modal" />,
}))
vi.mock('../components/SyncStatus', () => ({
  default: () => <div data-testid="sync-status">Synced</div>,
}))
vi.mock('../components/CommandPalette', () => ({
  default: () => <div data-testid="command-palette" />,
  searchShortcutLabel: '⌘K',
}))

// Mock react-router-dom
const mockNavigate = vi.fn()
vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useLocation: () => ({ pathname: '/app', search: '', hash: '', state: null, key: 'default' }),
  Outlet: () => <div data-testid="outlet" />,
}))

// Mock useAuth
const mockLogout = vi.fn()
let mockAuthState = {
  user: { email: 'test@example.com', is_admin: false } as {
    email: string
    is_admin: boolean
  } | null,
  logout: mockLogout,
}

vi.mock('../state/AuthContext', () => ({
  useAuth: () => mockAuthState,
}))

describe('Dashboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthState = {
      user: { email: 'test@example.com', is_admin: false },
      logout: mockLogout,
    }
  })

  it('renders header with TaskAI branding', () => {
    render(<Dashboard />)

    expect(screen.getByText('TaskAI')).toBeInTheDocument()
    expect(screen.getByAltText('TaskAI')).toBeInTheDocument()
  })

  // User email, avatar, and logout are now in the Sidebar component
  // and covered by Sidebar.test.tsx

  it('opens sidebar on mobile hamburger click', async () => {
    const user = userEvent.setup()
    render(<Dashboard />)

    const hamburger = screen.getByLabelText('Open sidebar')
    await user.click(hamburger)

    await waitFor(() => {
      const sidebar = screen.getByTestId('sidebar')
      expect(sidebar).toHaveAttribute('data-open', 'true')
    })
  })

  it('renders Sidebar component', () => {
    render(<Dashboard />)

    expect(screen.getByTestId('sidebar')).toBeInTheDocument()
  })

  it('renders CommandPalette component', () => {
    render(<Dashboard />)

    expect(screen.getByTestId('command-palette')).toBeInTheDocument()
  })

  it('renders ProjectModal component', () => {
    render(<Dashboard />)

    expect(screen.getByTestId('project-modal')).toBeInTheDocument()
  })

  it('renders SyncStatus component', () => {
    render(<Dashboard />)

    expect(screen.getByTestId('sync-status')).toBeInTheDocument()
  })

  it('renders the Outlet for nested routes', () => {
    render(<Dashboard />)

    expect(screen.getByTestId('outlet')).toBeInTheDocument()
  })

  it('sidebar starts closed', () => {
    render(<Dashboard />)

    const sidebar = screen.getByTestId('sidebar')
    expect(sidebar).toHaveAttribute('data-open', 'false')
  })
})
