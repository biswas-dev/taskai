import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

// Mock all route components
vi.mock('./routes/Landing', () => ({ default: () => <div data-testid="landing">Landing</div> }))
vi.mock('./routes/Login', () => ({ default: () => <div data-testid="login">Login</div> }))
vi.mock('./routes/Signup', () => ({ default: () => <div data-testid="signup">Signup</div> }))
vi.mock('./routes/Dashboard', () => ({ default: () => <div data-testid="dashboard">Dashboard</div> }))
vi.mock('./routes/Projects', () => ({ default: () => <div data-testid="projects">Projects</div> }))
vi.mock('./routes/ProjectDetail', () => ({ default: () => <div data-testid="project-detail">ProjectDetail</div> }))
vi.mock('./routes/ProjectSettings', () => ({ default: () => <div data-testid="project-settings">ProjectSettings</div> }))
vi.mock('./routes/TaskDetail', () => ({ default: (props: Record<string, unknown>) => <div data-testid="task-detail" data-modal={props.isModal}>TaskDetail</div> }))
vi.mock('./routes/Sprints', () => ({ default: () => <div data-testid="sprints">Sprints</div> }))
vi.mock('./routes/Tags', () => ({ default: () => <div data-testid="tags">Tags</div> }))
vi.mock('./routes/Admin', () => ({ default: () => <div data-testid="admin">Admin</div> }))
vi.mock('./routes/Settings', () => ({ default: () => <div data-testid="settings">Settings</div> }))
vi.mock('./routes/Assets', () => ({ default: () => <div data-testid="assets">Assets</div> }))
vi.mock('./routes/AcceptTeamInvite', () => ({ default: () => <div data-testid="accept-invite">AcceptTeamInvite</div> }))
vi.mock('./routes/Wiki', () => ({ default: () => <div data-testid="wiki">Wiki</div> }))

// Mock ProtectedRoute to just render children
vi.mock('./components/ProtectedRoute', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

// Mock AuthProvider and SyncProvider
vi.mock('./state/AuthContext', () => ({
  AuthProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  useAuth: () => ({ user: null, loading: false }),
}))
vi.mock('./state/SyncContext', () => ({
  SyncProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

import App from './App'

describe('App', () => {
  it('renders without crashing', () => {
    render(<App />)
    // The landing page is the default route
    expect(screen.getByTestId('landing')).toBeInTheDocument()
  })

  it('renders landing page at root path', () => {
    window.history.pushState({}, '', '/')
    render(<App />)
    expect(screen.getByTestId('landing')).toBeInTheDocument()
  })
})
