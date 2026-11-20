import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import Meetings, { toLocalInput } from './Meetings'
import { api, type Meeting } from '../lib/api'

vi.mock('../lib/api', () => ({
  api: {
    getMeetings: vi.fn(),
    getProjectMembers: vi.fn(),
    createMeeting: vi.fn(),
    updateMeeting: vi.fn(),
    deleteMeeting: vi.fn(),
  },
}))

const mocked = vi.mocked(api)
const members = [
  { id: 1, user_id: 1, email: 'a@x.com', name: 'Anshuman', role: 'owner', granted_by: 1, granted_at: '' },
  { id: 2, user_id: 7, email: 'n@x.com', name: 'Nishant', role: 'member', granted_by: 1, granted_at: '' },
]
const meeting: Meeting = {
  id: 5, project_id: 12, title: 'Superteam kickoff',
  starts_at: '2026-11-20T18:30:00Z', ends_at: '2026-11-20T19:00:00Z',
  agenda: '1. Intros', minutes: '', attendees: [{ user_id: 1, name: 'Anshuman', email: 'a@x.com' }],
  created_at: '', updated_at: '',
}

describe('Meetings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocked.getProjectMembers.mockResolvedValue(members)
  })

  it('shows the empty state and creates a meeting with every member preselected', async () => {
    mocked.getMeetings.mockResolvedValue([])
    mocked.createMeeting.mockResolvedValue({ ...meeting, id: 9 })
    render(<Meetings projectId={12} />)

    expect(await screen.findByText(/No meetings yet/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'New meeting' }))
    expect(screen.getByRole('checkbox', { name: 'Anshuman' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'Nishant' })).toBeChecked()

    await userEvent.type(screen.getByLabelText(/^Title/), 'Weekly sync')
    await userEvent.click(screen.getByRole('button', { name: 'Create meeting' }))
    expect(mocked.createMeeting).toHaveBeenCalledWith(12, expect.objectContaining({ title: 'Weekly sync', attendee_ids: [1, 7] }))
  })

  it('opens the latest meeting and saves minutes', async () => {
    mocked.getMeetings.mockResolvedValue([meeting])
    mocked.updateMeeting.mockResolvedValue({ ...meeting, minutes: 'Agreed on plan' })
    render(<Meetings projectId={12} />)

    expect(await screen.findByDisplayValue('Superteam kickoff')).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: 'Nishant' })).not.toBeChecked()
    await userEvent.type(screen.getByLabelText('Minutes (Markdown)'), 'Agreed on plan')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(mocked.updateMeeting).toHaveBeenCalledWith(5, expect.objectContaining({ minutes: 'Agreed on plan' }))
    expect(mocked.updateMeeting.mock.calls[0][1]).not.toHaveProperty('attendee_ids')
  })

  it('needs a second click to delete', async () => {
    mocked.getMeetings.mockResolvedValue([meeting])
    mocked.deleteMeeting.mockResolvedValue()
    render(<Meetings projectId={12} />)

    await userEvent.click(await screen.findByRole('button', { name: 'Delete' }))
    expect(mocked.deleteMeeting).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: 'Confirm delete' }))
    expect(mocked.deleteMeeting).toHaveBeenCalledWith(5)
  })

  it('round-trips local datetime input values', () => {
    const local = toLocalInput('2026-11-20T18:30:00Z')
    expect(new Date(local).toISOString()).toBe('2026-11-20T18:30:00.000Z')
  })
})
