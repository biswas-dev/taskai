import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import Memories from './Memories'
import { api, type Memory, type MemoryUsage } from '../lib/api'

vi.mock('../state/AuthContext', () => ({
  useAuth: () => ({ user: { id: 1 } }),
}))

vi.mock('../lib/api', () => ({
  api: {
    getMemories: vi.fn(),
    getMemoryUsage: vi.fn(),
    deleteMemory: vi.fn(),
    compactMemories: vi.fn(),
  },
}))

const mocked = vi.mocked(api)
const mine: Memory = {
  id: 4, owner_id: 1, kind: 'gotcha', title: 'Partial index needs WHERE', body: 'ON CONFLICT must repeat it',
  scope: { repo: 'biswas-dev/taskai' }, author: { agent: 'claude-code', host: 'mbp' }, confidence: 'high',
  status: 'active', size_bytes: 1700, created_at: '2026-11-01T00:00:00Z', updated_at: '2026-11-01T00:00:00Z',
}
const shared: Memory = { ...mine, id: 5, owner_id: 2, title: 'Staging is ARM64' }
const usage: MemoryUsage = { used_bytes: 900 * 1024 * 1024, quota_bytes: 1024 * 1024 * 1024, percent: 87.9, count: 2,
  active_count: 2, high_water_percent: 80, warning: 'memory storage is 88% full' }

describe('Memories', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocked.getMemoryUsage.mockResolvedValue(usage)
    mocked.getMemories.mockResolvedValue([mine, shared])
  })

  it('shows usage with its warning, and only offers delete on your own memories', async () => {
    render(<Memories />)
    expect(await screen.findByText('Partial index needs WHERE')).toBeInTheDocument()
    expect(screen.getByRole('progressbar', { name: 'Memory storage used' })).toHaveAttribute('aria-valuenow', '88')
    expect(screen.getByText(/88% full/)).toBeInTheDocument()
    expect(screen.getByText('900 MB of 1.0 GB (87.9%)')).toBeInTheDocument()
    expect(screen.getByText('shared with you')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /Delete memory/ })).toHaveLength(1)
  })

  it('searches with the query and repo', async () => {
    render(<Memories />)
    await screen.findByText('Partial index needs WHERE')
    await userEvent.type(screen.getByLabelText('Search'), 'partial index')
    await userEvent.type(screen.getByLabelText('Repository'), 'biswas-dev/taskai')
    await userEvent.click(screen.getByRole('button', { name: 'Search' }))
    expect(mocked.getMemories).toHaveBeenLastCalledWith({ query: 'partial index', repo: 'biswas-dev/taskai', kind: '' })
  })

  it('deletes after confirming', async () => {
    mocked.deleteMemory.mockResolvedValue()
    render(<Memories />)
    await userEvent.click(await screen.findByRole('button', { name: 'Delete memory Partial index needs WHERE' }))
    const item = screen.getByText('Partial index needs WHERE').closest('li') as HTMLElement
    await userEvent.click(within(item).getByRole('button', { name: 'Delete' }))
    expect(mocked.deleteMemory).toHaveBeenCalledWith(4)
    expect(screen.queryByText('Partial index needs WHERE')).not.toBeInTheDocument()
  })

  it('compacts and reports what it freed', async () => {
    mocked.compactMemories.mockResolvedValue({ superseded_removed: 2, expired_removed: 1, duplicates_merged: 3,
      low_confidence_summarised: 0, summaries_created: 0, bytes_before: 10000, bytes_after: 4000, freed_bytes: 6000 })
    render(<Memories />)
    await screen.findByText('Partial index needs WHERE')
    await userEvent.click(screen.getByRole('button', { name: 'Compact' }))
    expect(await screen.findByText(/Compaction freed 5.9 KB/)).toBeInTheDocument()
  })
})
