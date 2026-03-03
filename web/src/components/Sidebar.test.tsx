import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import Sidebar from './Sidebar'

// Mock react-router-dom
const mockNavigate = vi.fn()
vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: () => ({}),
  useLocation: () => ({ pathname: '/app' }),
}))

// Mock useAuth
let mockUser: { email: string; is_admin: boolean } | null = {
  email: 'test@example.com',
  is_admin: false,
}
vi.mock('../state/AuthContext', () => ({
  useAuth: () => ({ user: mockUser }),
}))

// Mock the API module
vi.mock('../lib/api', () => ({
  api: {
    getProjects: vi.fn(),
  },
}))

import { api } from '../lib/api'

const mockedGetProjects = vi.mocked(api.getProjects)

describe('Sidebar', () => {
  const defaultProps = {
    onCreateProject: vi.fn(),
    isOpen: false,
    onClose: vi.fn(),
    isPinned: true, // render static sidebar so tests can see content
    onTogglePin: vi.fn(),
  }

  beforeEach(() => {
    vi.clearAllMocks()
    mockUser = { email: 'test@example.com', is_admin: false }
  })

  it('shows loading skeleton initially', () => {
    mockedGetProjects.mockReturnValue(new Promise(() => {})) // never resolves
    render(<Sidebar {...defaultProps} />)

    // Loading state renders animated pulse placeholders
    const pulseDivs = document.querySelectorAll('.animate-pulse')
    expect(pulseDivs.length).toBeGreaterThan(0)
  })

  it('renders project list after loading', async () => {
    const projects = [
      { id: 1, name: 'Project Alpha', description: 'First project', owner_id: 1, created_at: '2024-01-01T00:00:00Z' },
      { id: 2, name: 'Project Beta', description: '', owner_id: 1, created_at: '2024-01-02T00:00:00Z' },
    ]
    mockedGetProjects.mockResolvedValue(projects)

    render(<Sidebar {...defaultProps} />)

    await waitFor(() => {
      expect(screen.getByText('Project Alpha')).toBeInTheDocument()
      expect(screen.getByText('Project Beta')).toBeInTheDocument()
    })
    expect(screen.getByText('First project')).toBeInTheDocument()
  })

  it('shows empty state when no projects', async () => {
    mockedGetProjects.mockResolvedValue([])

    render(<Sidebar {...defaultProps} />)

    await waitFor(() => {
      expect(screen.getByText('No projects yet')).toBeInTheDocument()
    })
    expect(screen.getByText('Create your first project')).toBeInTheDocument()
  })

  it('shows error when API fails', async () => {
    mockedGetProjects.mockRejectedValue(new Error('Network error'))

    render(<Sidebar {...defaultProps} />)

    await waitFor(() => {
      expect(screen.getByText('Network error')).toBeInTheDocument()
    })
  })

  it('calls onCreateProject when "New Project" button clicked', async () => {
    const user = userEvent.setup()
    mockedGetProjects.mockResolvedValue([])

    render(<Sidebar {...defaultProps} />)

    await waitFor(() => {
      expect(screen.getByText('New Project')).toBeInTheDocument()
    })

    await user.click(screen.getByText('New Project'))

    expect(defaultProps.onCreateProject).toHaveBeenCalledOnce()
  })

  it('shows admin link for admin users', async () => {
    mockUser = { email: 'admin@example.com', is_admin: true }
    mockedGetProjects.mockResolvedValue([])

    render(<Sidebar {...defaultProps} />)

    await waitFor(() => {
      expect(screen.getByText('Admin')).toBeInTheDocument()
    })
  })

  it('does not show admin link for non-admin users', async () => {
    mockUser = { email: 'user@example.com', is_admin: false }
    mockedGetProjects.mockResolvedValue([])

    render(<Sidebar {...defaultProps} />)

    await waitFor(() => {
      expect(screen.getByText('No projects yet')).toBeInTheDocument()
    })

    expect(screen.queryByText('Admin')).not.toBeInTheDocument()
  })

  it('navigation items are rendered', async () => {
    mockedGetProjects.mockResolvedValue([])

    render(<Sidebar {...defaultProps} />)

    await waitFor(() => {
      expect(screen.getByText('Sprints')).toBeInTheDocument()
    })
    expect(screen.getByText('Tags')).toBeInTheDocument()
    expect(screen.getByText('Assets')).toBeInTheDocument()
    expect(screen.getByText('Settings')).toBeInTheDocument()
  })

  it('navigates to project on click', async () => {
    const user = userEvent.setup()
    const projects = [
      { id: 42, name: 'Clickable Project', owner_id: 1, created_at: '2024-01-01T00:00:00Z' },
    ]
    mockedGetProjects.mockResolvedValue(projects)

    render(<Sidebar {...defaultProps} />)

    await waitFor(() => {
      expect(screen.getByText('Clickable Project')).toBeInTheDocument()
    })

    await user.click(screen.getByText('Clickable Project'))

    expect(mockNavigate).toHaveBeenCalledWith('/app/projects/42')
  })

  it('navigates on nav item click', async () => {
    const user = userEvent.setup()
    mockedGetProjects.mockResolvedValue([])

    render(<Sidebar {...defaultProps} />)

    await waitFor(() => {
      expect(screen.getByText('Settings')).toBeInTheDocument()
    })

    await user.click(screen.getByText('Settings'))

    expect(mockNavigate).toHaveBeenCalledWith('/app/settings')
  })

  it('shows generic error for non-Error API failures', async () => {
    mockedGetProjects.mockRejectedValue('something broke')

    render(<Sidebar {...defaultProps} />)

    await waitFor(() => {
      expect(screen.getByText('Failed to load projects')).toBeInTheDocument()
    })
  })
})
