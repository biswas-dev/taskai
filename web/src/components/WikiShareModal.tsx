import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  apiClient,
  type ProjectMember,
  type WikiPage,
  type WikiPageVisibility,
  type WikiSharing,
} from '../lib/api'
import { useDialog } from '../state/DialogContext'
import FormError from './ui/FormError'
import SearchSelect from './ui/SearchSelect'
import { WikiIcon } from './WikiVisibilityIcon'

type SharedPerson = WikiSharing['shared_with'][number]

interface WikiShareModalProps {
  page: WikiPage
  projectId: number
  onClose: () => void
  onChanged: (updated: Partial<WikiPage>) => void
}

function personLabel(person: { email: string; user_name?: string; name?: string }): string {
  return person.user_name || person.name || person.email
}

function errorMessage(err: unknown, fallback: string): string {
  return err instanceof Error && err.message ? err.message : fallback
}

function publicWikiUrl(token: string): string {
  return `${window.location.origin}/share/wiki/${token}`
}

const VISIBILITY_OPTIONS: { value: WikiPageVisibility; label: string; hint: string }[] = [
  { value: 'project', label: 'Everyone in this project', hint: 'All project members can view and edit.' },
  { value: 'restricted', label: 'Protected — only people you choose', hint: 'Hidden from everyone else in the project.' },
]

export default function WikiShareModal({ page, projectId, onClose, onChanged }: Readonly<WikiShareModalProps>) {
  const dialog = useDialog()
  const [sharing, setSharing] = useState<WikiSharing | null>(null)
  const [loadError, setLoadError] = useState('')
  const [members, setMembers] = useState<ProjectMember[]>([])
  const [membersError, setMembersError] = useState('')
  const [visibility, setVisibility] = useState<WikiPageVisibility>(page.visibility ?? 'project')
  const [people, setPeople] = useState<SharedPerson[]>([])
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [publicToken, setPublicToken] = useState<string | null>(null)
  const [linkBusy, setLinkBusy] = useState(false)
  const [linkError, setLinkError] = useState('')
  const [copied, setCopied] = useState(false)
  const confirmingRef = useRef(false)

  const load = useCallback(async () => {
    setLoadError('')
    setSharing(null)
    try {
      const data = await apiClient.getWikiSharing(page.id)
      setSharing(data)
      setVisibility(data.visibility)
      setPeople(data.shared_with)
      setPublicToken(data.public_token ?? null)
      if (data.can_manage) {
        try {
          setMembers(await apiClient.getProjectMembers(projectId))
        } catch (err) {
          setMembersError(errorMessage(err, 'Could not load project members'))
        }
      }
    } catch (err) {
      setLoadError(errorMessage(err, 'Could not load sharing settings'))
    }
  }, [page.id, projectId])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !e.defaultPrevented && !confirmingRef.current) onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  const memberOptions = useMemo(() => {
    const taken = new Set(people.map((p) => p.user_id))
    if (sharing) taken.add(sharing.created_by)
    return members
      .filter((m) => !taken.has(m.user_id))
      .map((m) => ({ value: String(m.user_id), label: personLabel(m), description: m.email }))
  }, [members, people, sharing])

  const addPerson = (value: string) => {
    const member = members.find((m) => String(m.user_id) === value)
    if (!member) return
    setPeople((list) => [...list, { user_id: member.user_id, email: member.email, user_name: member.user_name || member.name }])
  }

  const removePerson = (userId: number) => {
    setPeople((list) => list.filter((p) => p.user_id !== userId))
  }

  const save = async () => {
    setSaving(true)
    setSaveError('')
    try {
      const updated = await apiClient.updateWikiSharing(page.id, {
        visibility,
        user_ids: visibility === 'restricted' ? people.map((p) => p.user_id) : [],
      })
      onChanged({ visibility: updated.visibility })
      dialog.notify('Sharing updated', 'success')
      onClose()
    } catch (err) {
      setSaveError(errorMessage(err, 'Could not save sharing settings'))
    } finally {
      setSaving(false)
    }
  }

  const createLink = async () => {
    setLinkBusy(true)
    setLinkError('')
    try {
      const { public_token } = await apiClient.createWikiPublicLink(page.id)
      setPublicToken(public_token)
      onChanged({ is_public: true })
    } catch (err) {
      setLinkError(errorMessage(err, 'Could not create a public link'))
    } finally {
      setLinkBusy(false)
    }
  }

  const revokeLink = async () => {
    confirmingRef.current = true
    const ok = await dialog.confirm({
      title: 'Revoke public link?',
      message: 'The link will stop working for anyone who has it.',
      confirmLabel: 'Revoke',
      danger: true,
    })
    confirmingRef.current = false
    if (!ok) return
    setLinkBusy(true)
    setLinkError('')
    try {
      await apiClient.deleteWikiPublicLink(page.id)
      setPublicToken(null)
      setCopied(false)
      onChanged({ is_public: false })
    } catch (err) {
      setLinkError(errorMessage(err, 'Could not revoke the link'))
    } finally {
      setLinkBusy(false)
    }
  }

  const copyLink = async () => {
    if (!publicToken) return
    try {
      await navigator.clipboard.writeText(publicWikiUrl(publicToken))
      setCopied(true)
    } catch {
      setLinkError('Could not copy. Select the link and copy it manually.')
    }
  }

  const canManage = sharing?.can_manage ?? false

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
      onClick={onClose}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="wiki-share-title"
        className="w-full max-w-lg bg-dark-bg-secondary rounded-xl border border-dark-border-subtle shadow-2xl overflow-hidden max-h-[90vh] flex flex-col"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start justify-between gap-4 px-6 pt-6 pb-4">
          <h3 id="wiki-share-title" className="text-base font-semibold text-dark-text-primary break-words">
            Share “{page.title}”
          </h3>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="text-dark-text-tertiary hover:text-dark-text-primary"
          >
            ×
          </button>
        </div>

        <div className="px-6 pb-6 overflow-y-auto space-y-5">
          {!sharing && !loadError && (
            <p role="status" className="text-sm text-dark-text-tertiary">Loading sharing settings…</p>
          )}

          {loadError && (
            <div className="space-y-3">
              <FormError message={loadError} />
              <button
                type="button"
                onClick={() => void load()}
                className="px-3 py-1.5 text-sm font-medium text-dark-text-secondary bg-dark-bg-tertiary hover:bg-dark-bg-tertiary/80 rounded-lg"
              >
                Try again
              </button>
            </div>
          )}

          {sharing && !canManage && (
            <ReadOnlyView visibility={sharing.visibility} people={sharing.shared_with} />
          )}

          {sharing && canManage && (
            <>
              <fieldset>
                <legend className="text-sm font-medium text-dark-text-primary mb-2">Who can see this page</legend>
                <div className="space-y-2">
                  {VISIBILITY_OPTIONS.map((opt) => (
                    <label
                      key={opt.value}
                      className={`flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-colors ${
                        visibility === opt.value
                          ? 'border-primary-500/60 bg-primary-500/10'
                          : 'border-dark-border-subtle hover:bg-dark-bg-tertiary/50'
                      }`}
                    >
                      <input
                        type="radio"
                        name="wiki-visibility"
                        value={opt.value}
                        checked={visibility === opt.value}
                        onChange={() => setVisibility(opt.value)}
                        className="mt-0.5 accent-primary-500"
                      />
                      <span>
                        <span className="block text-sm text-dark-text-primary">{opt.label}</span>
                        <span className="block text-xs text-dark-text-tertiary">{opt.hint}</span>
                      </span>
                    </label>
                  ))}
                </div>
                <p className="mt-2 text-xs text-dark-text-tertiary">
                  The page author and the project owner always have access.
                </p>
              </fieldset>

              {visibility === 'restricted' && (
                <div className="space-y-3">
                  <h4 className="text-sm font-medium text-dark-text-primary">People with access</h4>
                  {membersError ? (
                    <FormError message={membersError} />
                  ) : (
                    <SearchSelect
                      value=""
                      onChange={addPerson}
                      options={memberOptions}
                      placeholder={memberOptions.length ? 'Add a project member…' : 'No more members to add'}
                      disabled={memberOptions.length === 0}
                    />
                  )}
                  {people.length === 0 ? (
                    <p className="text-xs text-dark-text-tertiary">Nobody added yet. Only the author and project owner can see it.</p>
                  ) : (
                    <ul className="space-y-1" aria-label="People with access">
                      {people.map((p) => (
                        <li
                          key={p.user_id}
                          className="flex items-center justify-between gap-2 px-3 py-1.5 rounded-lg bg-dark-bg-tertiary/60"
                        >
                          <span className="min-w-0">
                            <span className="block text-sm text-dark-text-primary truncate">{personLabel(p)}</span>
                            {p.user_name && <span className="block text-xs text-dark-text-tertiary truncate">{p.email}</span>}
                          </span>
                          <button
                            type="button"
                            onClick={() => removePerson(p.user_id)}
                            aria-label={`Remove ${personLabel(p)}`}
                            className="shrink-0 text-dark-text-tertiary hover:text-danger-400"
                          >
                            ×
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              )}

              <FormError message={saveError} />

              <section aria-labelledby="wiki-public-link-title" className="pt-4 border-t border-dark-border-subtle">
                <h4 id="wiki-public-link-title" className="flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide text-dark-text-tertiary mb-2">
                  <WikiIcon kind="globe" className="w-3.5 h-3.5" />
                  Public link
                </h4>
                <p className="text-xs text-dark-text-tertiary mb-2">
                  Anyone with the link can read this page without signing in.
                </p>
                {publicToken ? (
                  <div className="space-y-2">
                    <div className="flex items-center gap-2">
                      <input
                        type="text"
                        readOnly
                        value={publicWikiUrl(publicToken)}
                        aria-label="Public link"
                        onFocus={(e) => e.currentTarget.select()}
                        className="flex-1 min-w-0 px-2 py-1.5 text-xs bg-dark-bg-primary border border-dark-border-subtle text-dark-text-secondary rounded-lg outline-none focus:border-primary-500"
                      />
                      <button
                        type="button"
                        onClick={() => void copyLink()}
                        className="shrink-0 px-3 py-1.5 text-xs font-medium text-dark-text-secondary bg-dark-bg-tertiary hover:bg-dark-bg-tertiary/80 rounded-lg"
                      >
                        {copied ? 'Copied' : 'Copy'}
                      </button>
                    </div>
                    <button
                      type="button"
                      onClick={() => void revokeLink()}
                      disabled={linkBusy}
                      className="text-xs font-medium text-danger-400 hover:text-danger-300 disabled:opacity-50"
                    >
                      Revoke link
                    </button>
                  </div>
                ) : (
                  <button
                    type="button"
                    onClick={() => void createLink()}
                    disabled={linkBusy}
                    className="px-3 py-1.5 text-xs font-medium text-dark-text-secondary bg-dark-bg-tertiary hover:bg-dark-bg-tertiary/80 rounded-lg disabled:opacity-50"
                  >
                    {linkBusy ? 'Creating…' : 'Create public link'}
                  </button>
                )}
                <FormError message={linkError} className="mt-2" />
              </section>
            </>
          )}
        </div>

        <div className="flex justify-end gap-2 px-6 py-4 border-t border-dark-border-subtle bg-dark-bg-primary/50">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 text-sm font-medium text-dark-text-secondary bg-dark-bg-tertiary hover:bg-dark-bg-tertiary/80 rounded-lg transition-colors"
          >
            {canManage ? 'Cancel' : 'Close'}
          </button>
          {canManage && (
            <button
              type="button"
              onClick={() => void save()}
              disabled={saving}
              className="px-4 py-2 text-sm font-medium text-white bg-primary-500 hover:bg-primary-600 rounded-lg transition-colors disabled:opacity-50"
            >
              {saving ? 'Saving…' : 'Save'}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

function ReadOnlyView({ visibility, people }: Readonly<{ visibility: WikiPageVisibility; people: SharedPerson[] }>) {
  return (
    <div className="space-y-3">
      <p className="text-xs text-dark-text-tertiary">Only the page author or project owner can change sharing.</p>
      <div className="flex items-center gap-2 text-sm text-dark-text-primary">
        <WikiIcon kind={visibility === 'restricted' ? 'lock' : 'people'} />
        {visibility === 'restricted' ? 'Protected — only chosen people' : 'Everyone in this project'}
      </div>
      {visibility === 'restricted' && (
        people.length === 0 ? (
          <p className="text-xs text-dark-text-tertiary">Shared with nobody else.</p>
        ) : (
          <ul className="space-y-1" aria-label="People with access">
            {people.map((p) => (
              <li key={p.user_id} className="text-sm text-dark-text-secondary">
                {personLabel(p)}
              </li>
            ))}
          </ul>
        )
      )}
    </div>
  )
}
