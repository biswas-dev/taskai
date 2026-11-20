import { useCallback, useEffect, useState } from 'react'
import { api, type Meeting, type MeetingInput, type ProjectMember } from '../lib/api'
import Button from './ui/Button'
import TextInput from './ui/TextInput'
import FormError from './ui/FormError'

interface MeetingsProps {
  projectId: number
}

interface Draft {
  title: string
  startsAt: string
  endsAt: string
  agenda: string
  minutes: string
  attendeeIds: number[]
}

// datetime-local inputs work in the viewer's local time, without a zone suffix.
export function toLocalInput(iso: string): string {
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function draftFrom(m: Meeting): Draft {
  return {
    title: m.title,
    startsAt: toLocalInput(m.starts_at),
    endsAt: toLocalInput(m.ends_at),
    agenda: m.agenda,
    minutes: m.minutes,
    attendeeIds: m.attendees.map(a => a.user_id),
  }
}

function newDraft(members: ProjectMember[]): Draft {
  const start = new Date()
  start.setMinutes(start.getMinutes() < 30 ? 0 : 30, 0, 0)
  const end = new Date(start.getTime() + 30 * 60 * 1000)
  return {
    title: '',
    startsAt: toLocalInput(start.toISOString()),
    endsAt: toLocalInput(end.toISOString()),
    agenda: '',
    minutes: '',
    attendeeIds: members.map(m => m.user_id),
  }
}

const formatWhen = (m: Meeting) => {
  const start = new Date(m.starts_at)
  const end = new Date(m.ends_at)
  const time: Intl.DateTimeFormatOptions = { hour: 'numeric', minute: '2-digit' }
  return `${start.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })} · ${start.toLocaleTimeString(undefined, time)}–${end.toLocaleTimeString(undefined, time)}`
}

const textareaClass =
  'w-full rounded-md bg-dark-bg-primary border border-dark-border-subtle px-3 py-2 text-sm text-dark-text-primary font-mono focus:outline-none focus:ring-2 focus:ring-primary-500'

export default function Meetings({ projectId }: MeetingsProps) {
  const [meetings, setMeetings] = useState<Meeting[]>([])
  const [members, setMembers] = useState<ProjectMember[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [selectedId, setSelectedId] = useState<number | 'new' | null>(null)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)

  const load = useCallback(async () => {
    try {
      setLoading(true)
      setLoadError(null)
      const [ms, mem] = await Promise.all([api.getMeetings(projectId), api.getProjectMembers(projectId)])
      setMeetings(ms)
      setMembers(mem)
      if (ms.length > 0) {
        setSelectedId(ms[0].id)
        setDraft(draftFrom(ms[0]))
      }
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : 'Failed to load meetings')
    } finally {
      setLoading(false)
    }
  }, [projectId])

  useEffect(() => {
    void load()
  }, [load])

  const select = (m: Meeting) => {
    setSelectedId(m.id)
    setDraft(draftFrom(m))
    setSaveError(null)
    setConfirmDelete(false)
  }

  const startNew = () => {
    setSelectedId('new')
    setDraft(newDraft(members))
    setSaveError(null)
  }

  const toggleAttendee = (userId: number) => {
    if (!draft) return
    const ids = draft.attendeeIds.includes(userId)
      ? draft.attendeeIds.filter(id => id !== userId)
      : [...draft.attendeeIds, userId]
    setDraft({ ...draft, attendeeIds: ids })
  }

  const save = async () => {
    if (!draft || selectedId === null) return
    const input: MeetingInput = {
      title: draft.title.trim(),
      starts_at: new Date(draft.startsAt).toISOString(),
      ends_at: new Date(draft.endsAt).toISOString(),
      agenda: draft.agenda,
      minutes: draft.minutes,
    }
    // Only resend attendees when they changed, so a member leaving the project doesn't block saving minutes.
    const original = meetings.find(m => m.id === selectedId)?.attendees.map(a => a.user_id) ?? []
    const changed = selectedId === 'new' || original.length !== draft.attendeeIds.length ||
      original.some(id => !draft.attendeeIds.includes(id))
    if (changed) input.attendee_ids = draft.attendeeIds
    try {
      setSaving(true)
      setSaveError(null)
      const saved = selectedId === 'new'
        ? await api.createMeeting(projectId, input)
        : await api.updateMeeting(selectedId, input)
      setMeetings(prev => {
        const rest = prev.filter(m => m.id !== saved.id)
        return [saved, ...rest].sort((a, b) => b.starts_at.localeCompare(a.starts_at))
      })
      select(saved)
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save meeting')
    } finally {
      setSaving(false)
    }
  }

  const remove = async () => {
    if (typeof selectedId !== 'number') return
    try {
      setSaving(true)
      await api.deleteMeeting(selectedId)
      const rest = meetings.filter(m => m.id !== selectedId)
      setMeetings(rest)
      if (rest.length > 0) select(rest[0])
      else {
        setSelectedId(null)
        setDraft(null)
      }
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to delete meeting')
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="flex-1 flex items-center justify-center" role="status" aria-label="Loading meetings">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500" />
      </div>
    )
  }
  if (loadError) {
    return (
      <div className="flex-1 p-6">
        <FormError message={loadError} />
        <Button variant="secondary" size="sm" className="mt-3" onClick={() => void load()}>Retry</Button>
      </div>
    )
  }

  return (
    <div className="flex-1 flex min-h-0">
      <aside className="w-72 border-r border-dark-border-subtle flex flex-col" aria-label="Meetings">
        <div className="p-3 flex items-center justify-between border-b border-dark-border-subtle">
          <h2 className="text-sm font-semibold text-dark-text-primary">Meetings</h2>
          <Button size="sm" onClick={startNew}>New meeting</Button>
        </div>
        {meetings.length === 0 ? (
          <p className="p-4 text-xs text-dark-text-tertiary">No meetings yet. Record your first one.</p>
        ) : (
          <ul className="overflow-y-auto flex-1">
            {meetings.map(m => (
              <li key={m.id}>
                <button
                  onClick={() => select(m)}
                  aria-current={selectedId === m.id ? 'true' : undefined}
                  className={`w-full text-left px-3 py-2.5 border-b border-dark-border-subtle transition-colors ${selectedId === m.id ? 'bg-dark-bg-tertiary' : 'hover:bg-dark-bg-secondary'}`}
                >
                  <p className="text-sm font-medium text-dark-text-primary truncate">{m.title}</p>
                  <p className="text-xs text-dark-text-tertiary mt-0.5">{formatWhen(m)}</p>
                  <p className="text-xs text-dark-text-quaternary mt-0.5">{m.attendees.length} attendees</p>
                </button>
              </li>
            ))}
          </ul>
        )}
      </aside>

      <section className="flex-1 overflow-y-auto p-6">
        {!draft ? (
          <p className="text-sm text-dark-text-tertiary">Select a meeting or create a new one.</p>
        ) : (
          <form
            className="max-w-3xl space-y-5"
            onSubmit={e => {
              e.preventDefault()
              void save()
            }}
          >
            <TextInput id="meeting-title" label="Title" value={draft.title} required maxLength={200}
              onChange={e => setDraft({ ...draft, title: e.target.value })} />
            <div className="grid grid-cols-2 gap-4">
              <TextInput id="meeting-start" type="datetime-local" label="Starts" value={draft.startsAt} required
                onChange={e => setDraft({ ...draft, startsAt: e.target.value })} />
              <TextInput id="meeting-end" type="datetime-local" label="Ends" value={draft.endsAt} required
                onChange={e => setDraft({ ...draft, endsAt: e.target.value })} />
            </div>

            <fieldset>
              <legend className="block text-xs font-medium text-dark-text-tertiary mb-1.5">Attendees</legend>
              <div className="flex flex-wrap gap-2">
                {members.map(m => (
                  <label key={m.user_id} className="flex items-center gap-2 rounded-md border border-dark-border-subtle px-2.5 py-1.5 text-xs text-dark-text-secondary cursor-pointer">
                    <input type="checkbox" checked={draft.attendeeIds.includes(m.user_id)} onChange={() => toggleAttendee(m.user_id)} />
                    {m.name || m.user_name || m.email}
                  </label>
                ))}
              </div>
            </fieldset>

            <div>
              <label htmlFor="meeting-agenda" className="block text-xs font-medium text-dark-text-tertiary mb-1.5">Agenda (Markdown)</label>
              <textarea id="meeting-agenda" rows={6} className={textareaClass} value={draft.agenda}
                onChange={e => setDraft({ ...draft, agenda: e.target.value })} />
            </div>
            <div>
              <label htmlFor="meeting-minutes" className="block text-xs font-medium text-dark-text-tertiary mb-1.5">Minutes (Markdown)</label>
              <textarea id="meeting-minutes" rows={12} className={textareaClass} value={draft.minutes}
                onChange={e => setDraft({ ...draft, minutes: e.target.value })} />
            </div>

            {saveError && <FormError message={saveError} />}
            <div className="flex items-center gap-3">
              <Button type="submit" disabled={saving}>{saving ? 'Saving…' : selectedId === 'new' ? 'Create meeting' : 'Save'}</Button>
              {typeof selectedId === 'number' && (
                <Button type="button" variant="danger" disabled={saving}
                  onClick={() => (confirmDelete ? void remove() : setConfirmDelete(true))}>
                  {confirmDelete ? 'Confirm delete' : 'Delete'}
                </Button>
              )}
            </div>
          </form>
        )}
      </section>
    </div>
  )
}
