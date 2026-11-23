import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, within, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom'
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
  scope: { repo: 'biswas-dev/taskai', issue: 50, files: ['api/db.go'] },
  author: { agent: 'claude-code', provider: 'claude', host: 'mbp', session: 's-1' }, confidence: 'high',
  status: 'active', size_bytes: 1700, created_at: '2026-11-01T00:00:00Z', updated_at: '2026-11-02T00:00:00Z',
}
const shared: Memory = { ...mine, id: 5, owner_id: 2, title: 'Staging is ARM64', body: 'Both staging and UAT are aarch64' }
const usage: MemoryUsage = { used_bytes: 900 * 1024 * 1024, quota_bytes: 1024 * 1024 * 1024, percent: 87.9, count: 2,
  active_count: 2, high_water_percent: 80, warning: 'memory storage is 88% full' }

let location = ''
function Where() {
  const l = useLocation()
  const navigate = useNavigate()
  location = l.search
  return (
    <>
      <button onClick={() => navigate('/app/memories?q=index&kind=gotcha')}>open link</button>
      <button onClick={() => navigate(-1)}>back</button>
    </>
  )
}

function renderAt(url = '/app/memories') {
  return render(
    <MemoryRouter initialEntries={[url]}>
      <Memories />
      <Where />
    </MemoryRouter>
  )
}

const searchCalls = () => mocked.getMemories.mock.calls.filter(([f]) => f.query)
const results = () => screen.getByRole('listbox', { name: 'Memories' })

describe('Memories', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocked.getMemoryUsage.mockResolvedValue(usage)
    mocked.getMemories.mockResolvedValue([mine, shared])
  })

  it('shows usage with its warning, the result count and latency', async () => {
    renderAt()
    expect(await screen.findByText('Partial index needs WHERE')).toBeInTheDocument()
    expect(await screen.findByRole('progressbar', { name: 'Memory storage used' })).toHaveAttribute('aria-valuenow', '88')
    expect(screen.getByText(/88% full/)).toBeInTheDocument()
    expect(screen.getByText('900 MB of 1.0 GB (87.9%)')).toBeInTheDocument()
    expect(screen.getByText('shared with you')).toBeInTheDocument()
    expect(screen.getByText(/^2 results · \d+ ms$/)).toBeInTheDocument()
  })

  it('searches as you type, once per pause, and puts the search in the URL', async () => {
    renderAt()
    await screen.findByText('Partial index needs WHERE')
    await userEvent.type(screen.getByLabelText('Search'), 'partial index')
    await userEvent.type(screen.getByLabelText('Repository'), 'biswas-dev/taskai')
    await waitFor(() => expect(mocked.getMemories).toHaveBeenLastCalledWith(
      { query: 'partial index', repo: 'biswas-dev/taskai', kind: '' }, expect.any(AbortSignal)))
    // Keystrokes inside the debounce window do not each fire a request.
    expect(searchCalls().length).toBeLessThanOrEqual(2)
    expect(new URLSearchParams(location).get('q')).toBe('partial index')
    expect(new URLSearchParams(location).get('repo')).toBe('biswas-dev/taskai')
  })

  it('runs the search a shared URL describes', async () => {
    renderAt('/app/memories?q=arm&kind=gotcha&repo=biswas-dev/taskai')
    expect(screen.getByLabelText('Search')).toHaveValue('arm')
    await waitFor(() => expect(mocked.getMemories).toHaveBeenCalledWith(
      { query: 'arm', repo: 'biswas-dev/taskai', kind: 'gotcha' }, expect.any(AbortSignal)))
    expect(mocked.getMemories).toHaveBeenCalledTimes(1)
  })

  it('follows URL changes it did not make, like back and forward', async () => {
    renderAt('/app/memories?q=arm')
    await waitFor(() => expect(mocked.getMemories).toHaveBeenCalledTimes(1))
    await userEvent.click(screen.getByRole('button', { name: 'open link' }))
    expect(screen.getByLabelText('Search')).toHaveValue('index')
    await waitFor(() => expect(mocked.getMemories).toHaveBeenLastCalledWith(
      { query: 'index', repo: '', kind: 'gotcha' }, expect.any(AbortSignal)))
    await userEvent.click(screen.getByRole('button', { name: 'back' }))
    expect(screen.getByLabelText('Search')).toHaveValue('arm')
    await waitFor(() => expect(mocked.getMemories).toHaveBeenLastCalledWith(
      { query: 'arm', repo: '', kind: '' }, expect.any(AbortSignal)))
    await new Promise(r => setTimeout(r, 300)) // past the debounce: the inputs must not write the old search back
    expect(new URLSearchParams(location).get('q')).toBe('arm')
    expect(new URLSearchParams(location).get('kind')).toBeNull()
  })

  it('applies a kind filter at once and records it in the URL', async () => {
    renderAt()
    await screen.findByText('Partial index needs WHERE')
    await userEvent.click(screen.getByLabelText('Kind'))
    await userEvent.click(screen.getByRole('option', { name: 'decision' }))
    await waitFor(() => expect(mocked.getMemories).toHaveBeenLastCalledWith(
      { query: '', repo: '', kind: 'decision' }, expect.any(AbortSignal)))
    expect(new URLSearchParams(location).get('kind')).toBe('decision')
  })

  it('keeps earlier results visible while a search runs, and an older answer never replaces a newer one', async () => {
    let resolveOld: (ms: Memory[]) => void = () => undefined
    let resolveNew: (ms: Memory[]) => void = () => undefined
    renderAt()
    await screen.findByText('Partial index needs WHERE')
    mocked.getMemories.mockReturnValueOnce(new Promise(r => { resolveOld = r }))
    await userEvent.type(screen.getByLabelText('Search'), 'part')
    await waitFor(() => expect(searchCalls()).toHaveLength(1))
    const oldSignal = searchCalls()[0][1] as AbortSignal
    expect(screen.getByText('Partial index needs WHERE')).toBeInTheDocument()
    expect(screen.getByText('Searching…')).toBeInTheDocument()

    mocked.getMemories.mockReturnValueOnce(new Promise(r => { resolveNew = r }))
    await userEvent.type(screen.getByLabelText('Search'), 'ial arm')
    await waitFor(() => expect(searchCalls()).toHaveLength(2))
    expect(oldSignal.aborted).toBe(true)
    await act(async () => resolveNew([shared]))
    await waitFor(() => expect(within(results()).getAllByRole('option').map(o => o.querySelector('h2')?.textContent)).toEqual(['Staging is ARM64']))
    await act(async () => resolveOld([mine]))
    expect(within(results()).getAllByRole('option').map(o => o.querySelector('h2')?.textContent)).toEqual(['Staging is ARM64'])
    expect(screen.getByText(/^1 result · \d+ ms$/)).toBeInTheDocument()
  })

  it('highlights the matched words, partial words included', async () => {
    renderAt('/app/memories?q=parti')
    await waitFor(() => expect(results()).toBeInTheDocument())
    const title = within(results()).getAllByRole('option')[0].querySelector('h2') as HTMLElement
    expect(title).toHaveTextContent('Partial index needs WHERE')
    expect(within(title).getByText('Parti').tagName).toBe('MARK')
  })

  it('focuses search with / and Ctrl+K, moves with the arrows and opens a memory with Enter', async () => {
    renderAt()
    await screen.findByText('Partial index needs WHERE')
    const input = screen.getByLabelText('Search')
    input.blur()
    await userEvent.keyboard('/')
    expect(input).toHaveFocus()
    expect(input).toHaveValue('')
    input.blur()
    const palette = vi.fn()
    document.addEventListener('keydown', palette)
    await userEvent.keyboard('{Control>}k{/Control}')
    expect(input).toHaveFocus()
    expect(palette).not.toHaveBeenCalledWith(expect.objectContaining({ key: 'k' })) // the app's command palette stays shut
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', metaKey: true })) // the header's search button
    expect(palette).toHaveBeenCalledWith(expect.objectContaining({ key: 'k' }))
    document.removeEventListener('keydown', palette)

    const options = within(results()).getAllByRole('option')
    expect(options[0]).toHaveAttribute('aria-selected', 'true')
    await userEvent.keyboard('{ArrowDown}')
    expect(options[1]).toHaveAttribute('aria-selected', 'true')
    expect(input).toHaveAttribute('aria-activedescendant', 'memory-5')
    await userEvent.keyboard('{ArrowDown}{ArrowUp}{ArrowUp}')
    expect(options[0]).toHaveAttribute('aria-selected', 'true')
    await userEvent.keyboard('{Enter}')

    const dialog = screen.getByRole('dialog', { name: 'Partial index needs WHERE' })
    expect(within(dialog).getByText('ON CONFLICT must repeat it')).toBeInTheDocument()
    expect(within(dialog).getByText('biswas-dev/taskai')).toBeInTheDocument()
    expect(within(dialog).getByText('#50')).toBeInTheDocument()
    expect(within(dialog).getByText('api/db.go')).toBeInTheDocument()
    expect(within(dialog).getByText('claude-code · claude')).toBeInTheDocument()
    expect(within(dialog).getByText('mbp')).toBeInTheDocument()
    expect(within(dialog).getByText('Created')).toBeInTheDocument()
    await userEvent.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(input).toHaveFocus()
  })

  it('offers delete only on your own memories, after confirming', async () => {
    mocked.deleteMemory.mockResolvedValue()
    renderAt()
    await userEvent.click(await screen.findByText('Staging is ARM64'))
    expect(within(screen.getByRole('dialog')).queryByRole('button', { name: 'Delete memory' })).not.toBeInTheDocument()
    await userEvent.keyboard('{Escape}')

    await userEvent.click(screen.getByText('Partial index needs WHERE'))
    mocked.getMemoryUsage.mockRejectedValue(new Error('down'))
    await userEvent.click(screen.getByRole('button', { name: 'Delete memory' }))
    await userEvent.click(screen.getByRole('button', { name: 'Delete' }))
    expect(mocked.deleteMemory).toHaveBeenCalledWith(4)
    expect(screen.queryByText('Partial index needs WHERE')).not.toBeInTheDocument()
    expect(screen.queryByText('down')).not.toBeInTheDocument()
  })

  it('keeps a failed delete visible after the reload that restores the memory', async () => {
    mocked.deleteMemory.mockRejectedValue(new Error('delete failed'))
    renderAt()
    await userEvent.click(await screen.findByText('Partial index needs WHERE'))
    await userEvent.click(screen.getByRole('button', { name: 'Delete memory' }))
    await userEvent.click(screen.getByRole('button', { name: 'Delete' }))
    expect(await screen.findByText('delete failed')).toBeInTheDocument()
    await waitFor(() => expect(mocked.getMemories).toHaveBeenCalledTimes(2))
    expect(await screen.findByText('Partial index needs WHERE')).toBeInTheDocument()
    expect(screen.getByText('delete failed')).toBeInTheDocument()
  })

  it('stays on a result when the selected last one is deleted', async () => {
    mocked.deleteMemory.mockResolvedValue()
    mocked.getMemories.mockResolvedValue([shared, mine])
    renderAt()
    await screen.findByText('Partial index needs WHERE')
    await userEvent.click(screen.getByLabelText('Search'))
    await userEvent.keyboard('{ArrowDown}{Enter}')
    await userEvent.click(screen.getByRole('button', { name: 'Delete memory' }))
    await userEvent.click(screen.getByRole('button', { name: 'Delete' }))
    expect(within(results()).getByRole('option')).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByLabelText('Search')).toHaveAttribute('aria-activedescendant', 'memory-5')
  })

  it('does not open a result of the previous query while a new one is pending', async () => {
    renderAt()
    await screen.findByText('Partial index needs WHERE')
    await userEvent.type(screen.getByLabelText('Search'), 'staging{Enter}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('keeps Tab inside the open memory', async () => {
    renderAt()
    await userEvent.click(await screen.findByText('Partial index needs WHERE'))
    const dialog = screen.getByRole('dialog')
    for (let i = 0; i < 4; i++) {
      await userEvent.tab()
      expect(dialog).toContainElement(document.activeElement as HTMLElement)
    }
    await userEvent.tab({ shift: true })
    expect(dialog).toContainElement(document.activeElement as HTMLElement)
  })

  it('shows a long body around its first match, with ellipses', async () => {
    const long = { ...mine, body: `${'filler '.repeat(30)}the zebrafish heater ${'tail '.repeat(60)}` }
    mocked.getMemories.mockResolvedValue([long])
    renderAt('/app/memories?q=zebra')
    await waitFor(() => expect(within(results()).getByText('zebra').tagName).toBe('MARK'))
    const text = within(results()).getByText('zebra').closest('p')?.textContent ?? ''
    expect(text.startsWith('…')).toBe(true)
    expect(text.endsWith('…')).toBe(true)
    expect(text.length).toBeLessThan(230)
  })

  it('compacts and reports what it freed', async () => {
    mocked.compactMemories.mockResolvedValue({ superseded_removed: 2, expired_removed: 1, duplicates_merged: 3,
      low_confidence_summarised: 0, summaries_created: 0, bytes_before: 10000, bytes_after: 4000, freed_bytes: 6000 })
    renderAt()
    await screen.findByText('Partial index needs WHERE')
    await userEvent.click(screen.getByRole('button', { name: 'Compact' }))
    expect(await screen.findByText(/Compaction freed 5.9 KB/)).toBeInTheDocument()
  })
})
