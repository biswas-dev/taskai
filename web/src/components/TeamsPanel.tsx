import { useCallback, useEffect, useState } from 'react'
import Card from './ui/Card'
import Button from './ui/Button'
import FormError from './ui/FormError'
import { useDialog } from '../state/DialogContext'
import {
  apiClient,
  type SentInvitation,
  type TeamInvitation,
  type TeamMember,
  type TeamSummary,
  type UserSearchResult,
} from '../lib/api'
import Select from './ui/Select'

const SELECTED_TEAM_KEY = 'taskai.settings.selectedTeamId'

function readStoredTeamId(): number | null {
  try {
    const raw = localStorage.getItem(SELECTED_TEAM_KEY)
    return raw ? Number(raw) : null
  } catch {
    return null
  }
}

function storeTeamId(id: number) {
  try {
    localStorage.setItem(SELECTED_TEAM_KEY, String(id))
  } catch {
    // Storage is a convenience only.
  }
}

const isManager = (role?: string) => role === 'owner' || role === 'admin'
const errorMessage = (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback)

export default function TeamsPanel() {
  const dialog = useDialog()
  const [teams, setTeams] = useState<TeamSummary[]>([])
  const [selectedTeamId, setSelectedTeamId] = useState<number | null>(null)
  const [members, setMembers] = useState<TeamMember[]>([])
  const [sentInvitations, setSentInvitations] = useState<SentInvitation[]>([])
  const [invitations, setInvitations] = useState<TeamInvitation[]>([])
  const [isLoadingTeams, setIsLoadingTeams] = useState(true)
  const [isLoadingMembers, setIsLoadingMembers] = useState(false)
  const [teamError, setTeamError] = useState('')
  const [teamSuccess, setTeamSuccess] = useState('')

  const [isCreatingTeam, setIsCreatingTeam] = useState(false)
  const [newTeamName, setNewTeamName] = useState('')
  const [isSavingNewTeam, setIsSavingNewTeam] = useState(false)

  const [isEditingTeamName, setIsEditingTeamName] = useState(false)
  const [editTeamName, setEditTeamName] = useState('')
  const [isSavingTeamName, setIsSavingTeamName] = useState(false)

  const [inviteEmail, setInviteEmail] = useState('')
  const [isInviting, setIsInviting] = useState(false)
  const [searchResults, setSearchResults] = useState<UserSearchResult[]>([])
  const [isSearching, setIsSearching] = useState(false)
  const [selectedUser, setSelectedUser] = useState<UserSearchResult | null>(null)
  const [showSearchDropdown, setShowSearchDropdown] = useState(false)

  const [busyMemberId, setBusyMemberId] = useState<number | null>(null)
  const [isRespondingToInvitation, setIsRespondingToInvitation] = useState<number | null>(null)
  const [isChangingTeam, setIsChangingTeam] = useState(false)

  const selectedTeam = teams.find((t) => t.id === selectedTeamId) ?? null
  const canManage = isManager(selectedTeam?.role)
  const moveTargets = teams.filter((t) => t.id !== selectedTeamId && isManager(t.role))

  const loadTeams = useCallback(async (preferTeamId?: number) => {
    try {
      const [teamList, invitationList] = await Promise.all([
        apiClient.listTeams(),
        apiClient.getMyInvitations(),
      ])
      setTeams(teamList)
      setInvitations(invitationList)
      setSelectedTeamId((current) => {
        const candidates = [preferTeamId, current, readStoredTeamId()]
        for (const id of candidates) {
          if (id && teamList.some((t) => t.id === id)) return id
        }
        return teamList.find((t) => t.is_home)?.id ?? teamList[0]?.id ?? null
      })
    } catch (error: unknown) {
      setTeamError(errorMessage(error, 'Failed to load teams'))
    } finally {
      setIsLoadingTeams(false)
    }
  }, [])

  const loadMembers = useCallback(async (teamId: number) => {
    setIsLoadingMembers(true)
    try {
      const [memberList, sent] = await Promise.all([
        apiClient.getTeamMembers(teamId),
        apiClient.getTeamSentInvitations(teamId),
      ])
      setMembers(memberList)
      setSentInvitations(sent)
    } catch (error: unknown) {
      setTeamError(errorMessage(error, 'Failed to load team members'))
    } finally {
      setIsLoadingMembers(false)
    }
  }, [])

  useEffect(() => {
    loadTeams()
  }, [loadTeams])

  useEffect(() => {
    if (selectedTeamId == null) return
    storeTeamId(selectedTeamId)
    setIsEditingTeamName(false)
    setInviteEmail('')
    setSelectedUser(null)
    loadMembers(selectedTeamId)
  }, [selectedTeamId, loadMembers])

  const refresh = async () => {
    await loadTeams()
    if (selectedTeamId != null) await loadMembers(selectedTeamId)
  }

  const resetMessages = () => {
    setTeamError('')
    setTeamSuccess('')
  }

  const handleCreateTeam = async (e: React.FormEvent) => {
    e.preventDefault()
    resetMessages()
    const name = newTeamName.trim()
    if (!name) {
      setTeamError('Team name is required')
      return
    }
    setIsSavingNewTeam(true)
    try {
      const created = await apiClient.createTeam(name)
      setNewTeamName('')
      setIsCreatingTeam(false)
      setTeamSuccess(`Team "${created.name}" created. Add people to it below.`)
      await loadTeams(created.id)
    } catch (error: unknown) {
      setTeamError(errorMessage(error, 'Failed to create team'))
    } finally {
      setIsSavingNewTeam(false)
    }
  }

  const handleSaveTeamName = async () => {
    if (!selectedTeam) return
    const trimmed = editTeamName.trim()
    if (!trimmed) {
      setTeamError('Team name is required')
      return
    }
    resetMessages()
    setIsSavingTeamName(true)
    try {
      await apiClient.updateTeam(trimmed, selectedTeam.id)
      setIsEditingTeamName(false)
      setTeamSuccess('Team name updated')
      await loadTeams()
    } catch (error: unknown) {
      setTeamError(errorMessage(error, 'Failed to update team name'))
    } finally {
      setIsSavingTeamName(false)
    }
  }

  const handleDeleteTeam = async () => {
    if (!selectedTeam) return
    if (!(await dialog.confirm({ title: 'Delete team?', message: `Delete the team "${selectedTeam.name}"? Its members lose access to the team, but keep any project access they were given.`, confirmLabel: 'Delete', danger: true }))) return
    resetMessages()
    setIsChangingTeam(true)
    try {
      await apiClient.deleteTeam(selectedTeam.id)
      setTeamSuccess(`Team "${selectedTeam.name}" deleted`)
      setSelectedTeamId(null)
      await loadTeams()
    } catch (error: unknown) {
      setTeamError(errorMessage(error, 'Failed to delete team'))
    } finally {
      setIsChangingTeam(false)
    }
  }

  const handleLeaveTeam = async () => {
    if (!selectedTeam) return
    if (!(await dialog.confirm({ title: 'Leave team?', message: `Leave "${selectedTeam.name}"? You will stop seeing its members.`, confirmLabel: 'Leave', danger: true }))) return
    resetMessages()
    setIsChangingTeam(true)
    try {
      await apiClient.leaveTeam(selectedTeam.id)
      setTeamSuccess(`You left "${selectedTeam.name}"`)
      setSelectedTeamId(null)
      await loadTeams()
    } catch (error: unknown) {
      setTeamError(errorMessage(error, 'Failed to leave team'))
    } finally {
      setIsChangingTeam(false)
    }
  }

  const handleRemoveMember = async (member: TeamMember) => {
    if (!selectedTeam) return
    const label = member.user_name || member.email
    if (!(await dialog.confirm({ title: 'Remove member?', message: `Remove ${label} from ${selectedTeam.name}?`, confirmLabel: 'Remove', danger: true }))) return
    resetMessages()
    setBusyMemberId(member.id)
    try {
      await apiClient.removeTeamMember(member.id, selectedTeam.id)
      setTeamSuccess(`${label} removed from ${selectedTeam.name}`)
      await refresh()
    } catch (error: unknown) {
      setTeamError(errorMessage(error, 'Failed to remove member'))
    } finally {
      setBusyMemberId(null)
    }
  }

  const handleMoveMember = async (member: TeamMember, targetTeamId: number) => {
    if (!selectedTeam) return
    const target = teams.find((t) => t.id === targetTeamId)
    if (!target) return
    const label = member.user_name || member.email
    if (!(await dialog.confirm({ title: 'Move member?', message: `Move ${label} from ${selectedTeam.name} to ${target.name}? Their project access is not changed.`, confirmLabel: 'Move' }))) return
    resetMessages()
    setBusyMemberId(member.id)
    try {
      await apiClient.moveTeamMember(selectedTeam.id, member.id, targetTeamId)
      setTeamSuccess(`${label} moved to ${target.name}`)
      await refresh()
    } catch (error: unknown) {
      setTeamError(errorMessage(error, 'Failed to move member'))
    } finally {
      setBusyMemberId(null)
    }
  }

  const handleInvitationResponse = async (invitationId: number, accept: boolean) => {
    resetMessages()
    setIsRespondingToInvitation(invitationId)
    try {
      if (accept) {
        await apiClient.acceptInvitation(invitationId)
        setTeamSuccess('Invitation accepted')
      } else {
        await apiClient.rejectInvitation(invitationId)
        setTeamSuccess('Invitation declined')
      }
      await loadTeams()
    } catch (error: unknown) {
      setTeamError(errorMessage(error, 'Failed to respond to invitation'))
    } finally {
      setIsRespondingToInvitation(null)
    }
  }

  const handleSelectUser = (u: UserSearchResult) => {
    setSelectedUser(u)
    setInviteEmail(u.name ? `${u.name} (${u.email})` : u.email)
    setShowSearchDropdown(false)
    setSearchResults([])
  }

  const handleInviteOrAdd = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!selectedTeam) return
    resetMessages()
    setIsInviting(true)
    try {
      if (selectedUser) {
        await apiClient.addTeamMember(selectedUser.id, selectedTeam.id)
        setTeamSuccess(`${selectedUser.name || selectedUser.email} added to ${selectedTeam.name}`)
      } else {
        const email = inviteEmail.trim()
        if (!email) {
          setTeamError('Email is required')
          return
        }
        await apiClient.inviteTeamMember(email, selectedTeam.id)
        setTeamSuccess(`${email} invited to ${selectedTeam.name}`)
      }
      setInviteEmail('')
      setSelectedUser(null)
      await refresh()
    } catch (error: unknown) {
      setTeamError(errorMessage(error, 'Failed to add member'))
    } finally {
      setIsInviting(false)
    }
  }

  // Debounced search for people to add to the selected team
  useEffect(() => {
    if (selectedUser || selectedTeamId == null) return
    const query = inviteEmail.trim()
    if (query.length < 2) {
      setSearchResults([])
      setShowSearchDropdown(false)
      return
    }
    let cancelled = false
    const timer = setTimeout(async () => {
      setIsSearching(true)
      try {
        const results = await apiClient.searchTeamUsers(query, selectedTeamId)
        if (cancelled) return
        setSearchResults(results)
        setShowSearchDropdown(results.length > 0)
      } catch {
        if (cancelled) return
        setSearchResults([])
        setShowSearchDropdown(false)
      } finally {
        if (!cancelled) setIsSearching(false)
      }
    }, 300)
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [inviteEmail, selectedUser, selectedTeamId])

  return (
    <Card className="shadow-md">
      <div className="p-6 sm:p-8 flex items-start gap-4">
        <div className="flex-shrink-0 w-10 h-10 bg-green-500/10 rounded-lg flex items-center justify-center">
          <svg className="w-6 h-6 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
          </svg>
        </div>
        <div className="flex-1 min-w-0">
          <h2 className="text-xl font-semibold text-dark-text-primary mb-1">Teams</h2>
          <p className="text-sm text-dark-text-secondary mb-6">
            Keep each company or client in its own team. People only see members of the teams they belong to.
          </p>

          {teamSuccess && (
            <div role="status" className="mb-4 p-4 bg-success-500/10 border-l-4 border-success-400 rounded-r-lg">
              <span className="text-success-300 font-medium">{teamSuccess}</span>
            </div>
          )}
          {teamError && <FormError message={teamError} className="mb-4" />}

          {/* Pending invitations to join someone else's team */}
          {invitations.length > 0 && (
            <div className="mb-6">
              <h3 className="text-sm font-semibold text-dark-text-primary mb-3">Pending Invitations</h3>
              <div className="space-y-2">
                {invitations.map((invitation) => (
                  <div key={invitation.id} className="p-4 bg-primary-500/10 border border-primary-500/30 rounded-lg flex items-center justify-between gap-3">
                    <div>
                      <p className="font-medium text-dark-text-primary">Invitation to join {invitation.team_name}</p>
                      {invitation.inviter_name && (
                        <p className="text-sm text-dark-text-secondary">From: {invitation.inviter_name}</p>
                      )}
                    </div>
                    <div className="flex gap-2">
                      <Button size="sm" onClick={() => handleInvitationResponse(invitation.id, true)} disabled={isRespondingToInvitation === invitation.id}>
                        {isRespondingToInvitation === invitation.id ? 'Accepting...' : 'Accept'}
                      </Button>
                      <Button size="sm" variant="secondary" onClick={() => handleInvitationResponse(invitation.id, false)} disabled={isRespondingToInvitation === invitation.id}>
                        Decline
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Team switcher */}
          <div className="mb-6">
            <div className="flex items-center justify-between mb-2">
              <h3 className="text-sm font-semibold text-dark-text-primary">Your teams</h3>
              {!isCreatingTeam && (
                <Button size="sm" variant="secondary" onClick={() => { resetMessages(); setIsCreatingTeam(true) }}>
                  + New team
                </Button>
              )}
            </div>

            {isCreatingTeam && (
              <form onSubmit={handleCreateTeam} className="mb-3 flex gap-2 items-center">
                <label htmlFor="new-team-name" className="sr-only">New team name</label>
                <input
                  id="new-team-name"
                  type="text"
                  value={newTeamName}
                  onChange={(e) => setNewTeamName(e.target.value)}
                  maxLength={100}
                  placeholder="e.g. Intelliviz"
                  autoFocus
                  className="flex-1 px-3 py-2 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:ring-2 focus:ring-primary-500"
                />
                <Button type="submit" size="sm" disabled={isSavingNewTeam || !newTeamName.trim()}>
                  {isSavingNewTeam ? 'Creating...' : 'Create'}
                </Button>
                <Button type="button" size="sm" variant="secondary" onClick={() => { setIsCreatingTeam(false); setNewTeamName('') }}>
                  Cancel
                </Button>
              </form>
            )}

            {isLoadingTeams ? (
              <p className="text-sm text-dark-text-tertiary">Loading teams...</p>
            ) : teams.length === 0 ? (
              <p className="text-sm text-dark-text-tertiary">You are not in any team yet.</p>
            ) : (
              <div className="flex flex-wrap gap-2" role="group" aria-label="Select a team">
                {teams.map((t) => {
                  const active = t.id === selectedTeamId
                  return (
                    <button
                      key={t.id}
                      type="button"
                      aria-pressed={active}
                      onClick={() => { resetMessages(); setSelectedTeamId(t.id) }}
                      className={`px-3 py-2 rounded-lg border text-left transition-colors ${
                        active
                          ? 'border-primary-500 bg-primary-500/10 text-dark-text-primary'
                          : 'border-dark-border-subtle bg-dark-bg-secondary text-dark-text-secondary hover:text-dark-text-primary'
                      }`}
                    >
                      <span className="block text-sm font-medium">{t.name}</span>
                      <span className="block text-xs text-dark-text-tertiary capitalize">
                        {t.role} · {t.member_count} {t.member_count === 1 ? 'member' : 'members'}
                      </span>
                    </button>
                  )
                })}
              </div>
            )}
          </div>

          {selectedTeam && (
            <>
              {/* Selected team header */}
              <div className="mb-6 p-4 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                <div className="flex items-center justify-between gap-3 flex-wrap">
                  <div className="flex-1 min-w-0">
                    {isEditingTeamName ? (
                      <div className="flex items-center gap-2">
                        <label htmlFor="edit-team-name" className="sr-only">Team name</label>
                        <input
                          id="edit-team-name"
                          type="text"
                          value={editTeamName}
                          onChange={(e) => setEditTeamName(e.target.value)}
                          maxLength={100}
                          autoFocus
                          onKeyDown={(e) => {
                            if (e.key === 'Enter') handleSaveTeamName()
                            if (e.key === 'Escape') setIsEditingTeamName(false)
                          }}
                          className="px-3 py-1.5 bg-dark-bg-primary border border-dark-border-subtle rounded-lg text-dark-text-primary text-lg font-semibold focus:outline-none focus:ring-2 focus:ring-primary-500"
                        />
                        <Button size="sm" onClick={handleSaveTeamName} disabled={isSavingTeamName}>
                          {isSavingTeamName ? 'Saving...' : 'Save'}
                        </Button>
                        <Button size="sm" variant="secondary" onClick={() => setIsEditingTeamName(false)} disabled={isSavingTeamName}>
                          Cancel
                        </Button>
                      </div>
                    ) : (
                      <div className="flex items-center gap-2">
                        <p className="text-lg font-semibold text-dark-text-primary truncate">{selectedTeam.name}</p>
                        {selectedTeam.is_home && (
                          <span className="px-2 py-0.5 text-xs font-medium bg-dark-bg-tertiary text-dark-text-secondary rounded">Primary</span>
                        )}
                        {canManage && (
                          <button
                            type="button"
                            onClick={() => { setEditTeamName(selectedTeam.name); setIsEditingTeamName(true) }}
                            className="p-1 text-dark-text-tertiary hover:text-dark-text-primary transition-colors rounded"
                            aria-label="Edit team name"
                            title="Edit team name"
                          >
                            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
                            </svg>
                          </button>
                        )}
                      </div>
                    )}
                    <p className="text-xs text-dark-text-tertiary mt-0.5">
                      {selectedTeam.member_count} {selectedTeam.member_count === 1 ? 'member' : 'members'} · {selectedTeam.project_count} {selectedTeam.project_count === 1 ? 'project' : 'projects'}
                    </p>
                  </div>
                  <div className="flex gap-2">
                    {selectedTeam.is_owner && !selectedTeam.is_home && (
                      <Button
                        size="sm"
                        variant="danger"
                        onClick={handleDeleteTeam}
                        disabled={isChangingTeam || selectedTeam.project_count > 0}
                        title={selectedTeam.project_count > 0 ? 'Move this team\'s projects to another team first' : undefined}
                      >
                        Delete team
                      </Button>
                    )}
                    {!selectedTeam.is_owner && (
                      <Button size="sm" variant="secondary" onClick={handleLeaveTeam} disabled={isChangingTeam}>
                        Leave team
                      </Button>
                    )}
                  </div>
                </div>
              </div>

              {/* Add / invite */}
              {canManage && (
                <form onSubmit={handleInviteOrAdd} className="mb-6">
                  <label htmlFor="team-invite-input" className="block text-sm font-medium text-dark-text-secondary mb-1.5">
                    Add someone to {selectedTeam.name}
                  </label>
                  <div className="flex gap-3 items-start">
                    <div className="flex-1 relative">
                      <input
                        id="team-invite-input"
                        type="text"
                        value={inviteEmail}
                        onChange={(e) => { setInviteEmail(e.target.value); setSelectedUser(null) }}
                        placeholder="Name of a colleague, or anyone's email address"
                        autoComplete="off"
                        className="w-full px-3 py-2 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                      />
                      {showSearchDropdown && searchResults.length > 0 && (
                        <div role="listbox" className="absolute z-10 mt-1 w-full bg-dark-bg-secondary border border-dark-border-subtle rounded-lg shadow-lg max-h-48 overflow-y-auto">
                          {searchResults.map((u) => (
                            <button
                              key={u.id}
                              type="button"
                              role="option"
                              aria-selected={false}
                              onClick={() => handleSelectUser(u)}
                              className="w-full px-3 py-2 text-left hover:bg-dark-bg-tertiary transition-colors"
                            >
                              <p className="text-sm font-medium text-dark-text-primary">{u.name || u.email}</p>
                              {u.name && <p className="text-xs text-dark-text-tertiary">{u.email}</p>}
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                    <Button type="submit" disabled={isInviting || !inviteEmail.trim()}>
                      {isInviting ? 'Adding...' : selectedUser ? 'Add to team' : 'Add / invite'}
                    </Button>
                  </div>
                  {selectedUser ? (
                    <p className="mt-1 text-xs text-primary-400">Will add {selectedUser.name || selectedUser.email} to {selectedTeam.name}</p>
                  ) : (
                    <p className="mt-1 text-xs text-dark-text-tertiary">
                      {isSearching
                        ? 'Searching...'
                        : 'Existing TaskAI users are added right away; new people get an email invitation to sign up.'}
                    </p>
                  )}
                </form>
              )}

              {/* Members */}
              <div>
                <h3 className="text-sm font-semibold text-dark-text-primary mb-3">Members of {selectedTeam.name}</h3>
                {isLoadingMembers ? (
                  <p className="text-sm text-dark-text-tertiary">Loading members...</p>
                ) : members.length === 0 ? (
                  <p className="text-sm text-dark-text-tertiary py-4 text-center">No members yet</p>
                ) : (
                  <ul className="space-y-3">
                    {members.map((member) => (
                      <li key={member.id} className="p-4 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                        <div className="flex items-center justify-between gap-3 flex-wrap">
                          <div className="min-w-0">
                            <div className="flex items-center gap-2">
                              <h4 className="font-medium text-dark-text-primary truncate">{member.user_name || member.email}</h4>
                              {member.role === 'owner' && (
                                <span className="px-2 py-0.5 text-xs font-medium bg-primary-500/10 text-primary-400 rounded">Owner</span>
                              )}
                              {member.role === 'admin' && (
                                <span className="px-2 py-0.5 text-xs font-medium bg-purple-500/10 text-purple-400 rounded">Admin</span>
                              )}
                            </div>
                            {member.user_name && (
                              <p className="text-sm text-dark-text-tertiary truncate">{member.email}</p>
                            )}
                          </div>
                          {canManage && member.role !== 'owner' && (
                            <div className="flex items-center gap-2">
                              {moveTargets.length > 0 && (
                                <>
                                  <label htmlFor={`move-member-${member.id}`} className="sr-only">
                                    Move {member.user_name || member.email} to another team
                                  </label>
                                  <Select<number>
                                    id={`move-member-${member.id}`}
                                    className="w-40"
                                    buttonClassName="py-1.5"
                                    value={null}
                                    placeholder="Move to…"
                                    disabled={busyMemberId === member.id}
                                    onChange={(target) => handleMoveMember(member, target)}
                                    searchPlaceholder="Search teams…"
                                    options={moveTargets.map((t) => ({ value: t.id, label: t.name }))}
                                  />
                                </>
                              )}
                              <Button
                                size="sm"
                                variant="danger"
                                onClick={() => handleRemoveMember(member)}
                                disabled={busyMemberId === member.id}
                              >
                                {busyMemberId === member.id ? 'Working...' : 'Remove'}
                              </Button>
                            </div>
                          )}
                        </div>
                      </li>
                    ))}
                  </ul>
                )}
              </div>

              {canManage && sentInvitations.length > 0 && (
                <div className="mt-6">
                  <h3 className="text-sm font-semibold text-dark-text-primary mb-3">Invited, waiting for sign-up</h3>
                  <ul className="space-y-2">
                    {sentInvitations.map((inv) => (
                      <li key={inv.id} className="p-3 bg-amber-500/5 border border-amber-500/20 rounded-lg flex items-center justify-between">
                        <span className="text-sm text-dark-text-primary">{inv.invitee_email}</span>
                        <span className="text-xs text-dark-text-tertiary">{new Date(inv.created_at).toLocaleDateString()}</span>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </>
          )}

          <div className="mt-6 bg-green-500/10 border border-green-500/30 rounded-lg p-4 text-sm text-dark-text-secondary">
            <p className="font-medium mb-1 text-dark-text-primary">How access works</p>
            <ol className="list-decimal list-inside space-y-1">
              <li>Create a team for each company or client you work with, and add its people.</li>
              <li>Members only see people in the teams they belong to, never your other teams.</li>
              <li>Being on a team gives no project access by itself: invite people from a project's Settings → Members, and they accept.</li>
            </ol>
          </div>
        </div>
      </div>
    </Card>
  )
}
