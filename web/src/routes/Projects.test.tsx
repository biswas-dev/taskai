import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import Projects from './Projects'

const mockNavigate = vi.fn()
vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
}))

describe('Projects', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })

  it('renders "Welcome to TaskAI" heading', () => {
    render(<Projects />)
    expect(screen.getByText('Welcome to TaskAI')).toBeInTheDocument()
  })

  it('renders descriptive text', () => {
    render(<Projects />)
    expect(
      screen.getByText(/Select a project from the sidebar to view its tasks/)
    ).toBeInTheDocument()
  })

  it('renders feature list items', () => {
    render(<Projects />)
    expect(screen.getByText('Organize your work into projects')).toBeInTheDocument()
    expect(screen.getByText('Track tasks with customizable swim lanes')).toBeInTheDocument()
    expect(screen.getByText('Stay productive and ship faster')).toBeInTheDocument()
  })
})
