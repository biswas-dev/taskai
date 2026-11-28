import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import BrainHub from './BrainHub'

let pathname = '/brain-hub'
vi.mock('react-router-dom', () => ({
  Link: ({ to, children, ...props }: { to: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={to} {...props}>{children}</a>
  ),
  useLocation: () => ({ pathname }),
}))

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, json: () => Promise.resolve({}) }))
})
afterEach(() => {
  vi.unstubAllGlobals()
  pathname = '/brain-hub'
})

describe('BrainHub', () => {
  it('says Brain Hub is MIT open source and TaskAI is proprietary', () => {
    render(<BrainHub />)
    expect(screen.getByText(/open source under the MIT license/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'MIT license' })).toHaveAttribute('href', 'https://github.com/anchoo2kewl/llm-brain/blob/main/LICENSE')
    expect(screen.getByText('TaskAI is a proprietary service. It is not open source.')).toBeInTheDocument()
    expect(screen.queryByText(/private/i)).toBeNull()
  })

  it('has the downloads and the features', () => {
    render(<BrainHub />)
    expect(screen.getByRole('link', { name: 'Download for Windows' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 3, name: /Encrypted sync through TaskAI \(in progress\)/ })).toBeInTheDocument()
  })

  it('shows its own top bar on the public page only', () => {
    const { unmount } = render(<BrainHub />)
    expect(screen.getByRole('navigation')).toBeInTheDocument()
    unmount()
    pathname = '/app/brain-hub'
    render(<BrainHub />)
    expect(screen.queryByRole('navigation')).toBeNull()
  })
})
