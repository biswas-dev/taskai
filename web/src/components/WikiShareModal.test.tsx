import type { ReactElement } from 'react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import WikiShareModal from './WikiShareModal'
import { DialogProvider } from '../state/DialogContext'
import type { WikiPage } from '../lib/api'

vi.mock('./ui/SearchSelect', () => ({
  default: ({ options, onChange, placeholder }: {
    options: { value: string; label: string }[]
    onChange: (value: string) => void
    placeholder?: string
  }) => (
    <select aria-label="Add person" value="" onChange={(e) => onChange(e.target.value)}>
      <option value="">{placeholder}</option>
      {options.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
    </select>
  ),
}))

const mocks = vi.hoisted(() => ({
  getWikiSharing: vi.fn(),
  updateWikiSharing: vi.fn(),
  createWikiPublicLink: vi.fn(),
  deleteWikiPublicLink: vi.fn(),
  getProjectMembers: vi.fn(),
}))

vi.mock('../lib/api', () => ({ apiClient: mocks, api: mocks }))

const page: WikiPage = {
  id: 7,
  project_id: 3,
  title: 'Roadmap',
  slug: 'roadmap',
  parent_id: null,
  position: 0,
  created_by: 10,
  created_at: '2026-09-01T00:00:00Z',
  updated_at: '2026-09-01T00:00:00Z',
  visibility: 'project',
  is_public: false,
}

const members = [
  { id: 1, user_id: 10, email: 'author@test.com', role: 'owner', granted_by: 10, granted_at: '' },
  { id: 2, user_id: 20, email: 'bob@test.com', user_name: 'Bob', role: 'member', granted_by: 10, granted_at: '' },
  { id: 3, user_id: 30, email: 'cara@test.com', role: 'member', granted_by: 10, granted_at: '' },
]

const managed = { visibility: 'project', can_manage: true, created_by: 10, shared_with: [] }

function renderModal(ui: ReactElement) {
  return render(<DialogProvider>{ui}</DialogProvider>)
}

describe('WikiShareModal', () => {
  const onClose = vi.fn()
  const onChanged = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getProjectMembers.mockResolvedValue(members)
  })

  it('saves a protected page with the chosen people', async () => {
    const user = userEvent.setup()
    mocks.getWikiSharing.mockResolvedValue(managed)
    mocks.updateWikiSharing.mockResolvedValue({ ...managed, visibility: 'restricted' })
    renderModal(<WikiShareModal page={page} projectId={3} onClose={onClose} onChanged={onChanged} />)

    await user.click(await screen.findByRole('radio', { name: /Protected/ }))
    const picker = await screen.findByRole('combobox', { name: 'Add person' })
    // The page author is never offered: they always have access.
    expect(within(picker).queryByRole('option', { name: 'author@test.com' })).not.toBeInTheDocument()
    await user.selectOptions(picker, '20')

    expect(screen.getByRole('list', { name: 'People with access' })).toHaveTextContent('Bob')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(mocks.updateWikiSharing).toHaveBeenCalledWith(7, { visibility: 'restricted', user_ids: [20] }))
    expect(onChanged).toHaveBeenCalledWith({ visibility: 'restricted' })
    expect(onClose).toHaveBeenCalled()
  })

  it('shows save errors inline', async () => {
    const user = userEvent.setup()
    mocks.getWikiSharing.mockResolvedValue(managed)
    mocks.updateWikiSharing.mockRejectedValue(new Error('user is not a project member'))
    renderModal(<WikiShareModal page={page} projectId={3} onClose={onClose} onChanged={onChanged} />)

    await user.click(await screen.findByRole('button', { name: 'Save' }))
    expect(await screen.findByText('user is not a project member')).toBeInTheDocument()
    expect(onClose).not.toHaveBeenCalled()
  })

  it('is read-only for people who cannot manage sharing', async () => {
    mocks.getWikiSharing.mockResolvedValue({
      visibility: 'restricted',
      can_manage: false,
      created_by: 10,
      shared_with: [{ user_id: 20, email: 'bob@test.com', user_name: 'Bob' }],
    })
    renderModal(<WikiShareModal page={page} projectId={3} onClose={onClose} onChanged={onChanged} />)

    expect(await screen.findByText(/Only the page author or project owner can change sharing/)).toBeInTheDocument()
    expect(screen.getByText('Bob')).toBeInTheDocument()
    expect(screen.queryByRole('radio')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Create public link' })).not.toBeInTheDocument()
    expect(mocks.getProjectMembers).not.toHaveBeenCalled()
  })

  it('creates a public link and shows its URL', async () => {
    const user = userEvent.setup()
    mocks.getWikiSharing.mockResolvedValue(managed)
    mocks.createWikiPublicLink.mockResolvedValue({ public_token: 'tok123' })
    renderModal(<WikiShareModal page={page} projectId={3} onClose={onClose} onChanged={onChanged} />)

    await user.click(await screen.findByRole('button', { name: 'Create public link' }))

    expect(await screen.findByRole('textbox', { name: 'Public link' })).toHaveValue(`${window.location.origin}/share/wiki/tok123`)
    expect(mocks.createWikiPublicLink).toHaveBeenCalledWith(7)
    expect(onChanged).toHaveBeenCalledWith({ is_public: true })
  })

  it('revokes the public link after confirmation', async () => {
    const user = userEvent.setup()
    mocks.getWikiSharing.mockResolvedValue({ ...managed, public_token: 'tok123' })
    mocks.deleteWikiPublicLink.mockResolvedValue(undefined)
    renderModal(<WikiShareModal page={{ ...page, is_public: true }} projectId={3} onClose={onClose} onChanged={onChanged} />)

    await user.click(await screen.findByRole('button', { name: 'Revoke link' }))
    const confirm = await screen.findByRole('alertdialog')
    expect(mocks.deleteWikiPublicLink).not.toHaveBeenCalled()
    await user.click(within(confirm).getByRole('button', { name: 'Revoke' }))

    await waitFor(() => expect(mocks.deleteWikiPublicLink).toHaveBeenCalledWith(7))
    expect(await screen.findByRole('button', { name: 'Create public link' })).toBeInTheDocument()
    expect(onChanged).toHaveBeenCalledWith({ is_public: false })
    expect(onClose).not.toHaveBeenCalled()
  })

  it('shows a load error with a retry', async () => {
    mocks.getWikiSharing.mockRejectedValue(new Error('boom'))
    renderModal(<WikiShareModal page={page} projectId={3} onClose={onClose} onChanged={onChanged} />)
    expect(await screen.findByText('boom')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument()
  })

  it('closes on Escape', async () => {
    const user = userEvent.setup()
    mocks.getWikiSharing.mockResolvedValue(managed)
    renderModal(<WikiShareModal page={page} projectId={3} onClose={onClose} onChanged={onChanged} />)
    await screen.findByRole('radio', { name: /Everyone/ })
    await user.keyboard('{Escape}')
    expect(onClose).toHaveBeenCalled()
  })
})
