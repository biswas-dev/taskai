import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import PublicWikiPage from './PublicWikiPage'

const mocks = vi.hoisted(() => ({ getPublicWikiPage: vi.fn() }))

vi.mock('../lib/api', () => ({ apiClient: mocks, api: mocks }))

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/share/wiki/:token" element={<PublicWikiPage />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('PublicWikiPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the shared page', async () => {
    mocks.getPublicWikiPage.mockResolvedValue({
      title: 'Launch plan',
      html: '<p>Ship it on <strong>Friday</strong></p>',
      project_name: 'Apollo',
      updated_at: '2026-09-20T12:00:00Z',
    })
    renderAt('/share/wiki/abc123')

    expect(await screen.findByRole('heading', { name: 'Launch plan' })).toBeInTheDocument()
    expect(mocks.getPublicWikiPage).toHaveBeenCalledWith('abc123')
    expect(screen.getByText('Apollo')).toBeInTheDocument()
    expect(screen.getByText(/^Updated /)).toBeInTheDocument()
    expect(screen.getByText('Friday').tagName).toBe('STRONG')
    expect(screen.getByText(/Shared from/)).toBeInTheDocument()
  })

  it('says the link is inactive when the fetch fails', async () => {
    mocks.getPublicWikiPage.mockRejectedValue(new Error('not found'))
    renderAt('/share/wiki/gone')
    expect(await screen.findByText('This link is no longer active')).toBeInTheDocument()
  })
})
