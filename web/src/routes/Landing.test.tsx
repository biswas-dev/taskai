import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import Landing from './Landing'

vi.mock('react-router-dom', () => ({
  Link: ({ to, children, ...props }: { to: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={to} {...props}>{children}</a>
  ),
}))

describe('Landing', () => {
  it('leads with the project management hero as the only h1, first on the page', () => {
    render(<Landing />)
    const h1s = screen.getAllByRole('heading', { level: 1 })
    expect(h1s).toHaveLength(1)
    expect(h1s[0]).toHaveTextContent('Project management built for AI agents')
    const first = document.querySelector('h1, h2')
    expect(first).toBe(h1s[0])
  })

  it('shows memories below the hero as an h2 section, with a protocol link and no quick start', () => {
    render(<Landing />)
    const memories = screen.getByRole('region', { name: 'One memory for every AI you use' })
    expect(memories).toHaveAttribute('id', 'memories')
    expect(screen.getByRole('heading', { level: 2, name: 'One memory for every AI you use' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Read the memory protocol/ })).toHaveAttribute('href', expect.stringContaining('docs/memory-protocol.md'))
    expect(screen.getByRole('link', { name: 'Memories' })).toHaveAttribute('href', '#memories')
    expect(screen.queryByText(/Quick start/i)).toBeNull()
    expect(screen.queryByText(/claude mcp add/)).toBeNull()
    expect(screen.queryByText(/curl /)).toBeNull()
  })

  it('has no pill labels, glow blobs or gradient text', () => {
    const { container } = render(<Landing />)
    expect(container.querySelector('.animate-pulse')).toBeNull()
    expect(container.querySelector('.bg-clip-text')).toBeNull()
    expect(container.querySelector('[class*="blur-["]')).toBeNull()
    expect(screen.queryByText(/^New ·/)).toBeNull()
    // The facts the old pills held are still on the page, as text.
    expect(screen.getByText(/The MCP server is built in/)).toBeInTheDocument()
  })

  it('keeps heading levels in order (no level is skipped)', () => {
    const { container } = render(<Landing />)
    const levels = [...container.querySelectorAll('h1, h2, h3, h4, h5, h6')].map(h => Number(h.tagName[1]))
    levels.reduce((prev, cur) => {
      expect(cur).toBeLessThanOrEqual(prev + 1)
      return cur
    })
  })

  it('shows how memories work with the rest of TaskAI, and leads the features with a Memories card', () => {
    render(<Landing />)
    const connect = screen.getByRole('region', { name: 'Works with everything in TaskAI' })
    for (const title of ['Tasks, issues and PRs', 'GitHub sync', 'Wiki and knowledge graph', 'One MCP server, one API key', 'Teams and projects']) {
      expect(connect).toHaveTextContent(title)
    }
    const card = screen.getByRole('link', { name: /^Memories One store of lessons/ })
    expect(card).toHaveAttribute('href', '#memories')
    const featureHeadings = [...document.querySelectorAll('#features h3')].map(h => h.textContent)
    expect(featureHeadings[0]).toBe('Memories')
  })

  it('renders "Sign in" and "Get started free" navigation links', () => {
    render(<Landing />)
    const signInLinks = screen.getAllByText('Sign in')
    expect(signInLinks.length).toBeGreaterThanOrEqual(1)

    const getStartedLinks = screen.getAllByText('Get started free')
    expect(getStartedLinks.length).toBeGreaterThanOrEqual(1)
  })

  it('renders feature sections', () => {
    render(<Landing />)
    expect(screen.getByText('Kanban boards')).toBeInTheDocument()
    expect(screen.getByText('Collaborative wiki')).toBeInTheDocument()
    expect(screen.getByText('Sprint planning')).toBeInTheDocument()
    expect(screen.getByText('Knowledge graph')).toBeInTheDocument()
  })

  it('renders the AI agents section', () => {
    render(<Landing />)
    expect(screen.getByText(/Let your AI agent/)).toBeInTheDocument()
    expect(screen.getByText(/built-in MCP \(Model Context Protocol\) server/)).toBeInTheDocument()
  })

  it('renders the footer', () => {
    render(<Landing />)
    expect(screen.getByText('Project management for AI-native teams.')).toBeInTheDocument()
  })

  it('has links to /signup', () => {
    render(<Landing />)
    const signupLinks = screen.getAllByRole('link').filter(
      (link) => link.getAttribute('href') === '/signup'
    )
    expect(signupLinks.length).toBeGreaterThanOrEqual(1)
  })

  it('has links to /login', () => {
    render(<Landing />)
    const loginLinks = screen.getAllByRole('link').filter(
      (link) => link.getAttribute('href') === '/login'
    )
    expect(loginLinks.length).toBeGreaterThanOrEqual(1)
  })
})
