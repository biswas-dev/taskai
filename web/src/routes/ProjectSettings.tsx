import { useState, useEffect } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import Card from '../components/ui/Card'
import Button from '../components/ui/Button'
import TextInput from '../components/ui/TextInput'
import FormError from '../components/ui/FormError'
import SearchSelect from '../components/ui/SearchSelect'
import { apiClient, type SwimLane, type Project, type ProjectInvitation, type GitHubPreviewResponse, type GitHubUserMatch, type GitHubRepo } from '../lib/api'

interface ProjectMember {
  id: number
  user_id: number
  email: string
  name?: string
  role: string
  granted_by: number
  granted_at: string
}

interface TeamMember {
  id: number
  user_id: number
  email: string
  name?: string
  role: string
}

interface GitHubSettings {
  github_repo_url: string
  github_owner: string
  github_repo_name: string
  github_branch: string
  github_sync_enabled: boolean
  github_last_sync: string | null
  github_token_set: boolean
  github_login: string | null
}

export default function ProjectSettings() {
  const navigate = useNavigate()
  const { projectId: projectIdParam } = useParams<{ projectId: string }>()
  const projectId = parseInt(projectIdParam || '0')

  // Project state
  const [project, setProject] = useState<Project | null>(null)

  // Members state
  const [members, setMembers] = useState<ProjectMember[]>([])
  const [invitations, setInvitations] = useState<ProjectInvitation[]>([])
  const [teamMembers, setTeamMembers] = useState<TeamMember[]>([])
  const [selectedUserId, setSelectedUserId] = useState('')
  const [newMemberRole, setNewMemberRole] = useState('member')
  const [memberError, setMemberError] = useState('')
  const [memberSuccess, setMemberSuccess] = useState('')
  const [isAddingMember, setIsAddingMember] = useState(false)

  // GitHub state
  const [githubSettings, setGithubSettings] = useState<GitHubSettings>({
    github_repo_url: '',
    github_owner: '',
    github_repo_name: '',
    github_branch: 'main',
    github_sync_enabled: false,
    github_last_sync: null,
    github_token_set: false,
    github_login: null,
  })
  const [githubError, setGithubError] = useState('')
  const [githubSuccess, setGithubSuccess] = useState('')
  const [isSavingGitHub, setIsSavingGitHub] = useState(false)
  const [isConnectingGitHub, setIsConnectingGitHub] = useState(false)
  const [isDisconnectingGitHub, setIsDisconnectingGitHub] = useState(false)
  const [githubRepos, setGithubRepos] = useState<GitHubRepo[]>([])
  const [isLoadingRepos, setIsLoadingRepos] = useState(false)
  const [selectedRepoFullName, setSelectedRepoFullName] = useState('')
  const [repoSearchQuery, setRepoSearchQuery] = useState('')

  // GitHub import state
  const [githubPreview, setGithubPreview] = useState<GitHubPreviewResponse | null>(null)
  const [isPreviewing, setIsPreviewing] = useState(false)
  const [isPulling, setIsPulling] = useState(false)
  const [isSyncing, setIsSyncing] = useState(false)
  const [importError, setImportError] = useState('')
  const [importSuccess, setImportSuccess] = useState('')
  const [userAssignments, setUserAssignments] = useState<Record<string, number>>({})
  const [pullSprints, setPullSprints] = useState(true)
  const [pullTags, setPullTags] = useState(true)
  const [pullTasks, setPullTasks] = useState(true)
  const [projectMembers, setProjectMembers] = useState<{ user_id: number; email: string; name?: string }[]>([])

  // Storage usage state
  const [storageUsage, setStorageUsage] = useState<{ user_id: number; user_name: string; file_count: number; total_size: number }[]>([])
  const [loadingStorage, setLoadingStorage] = useState(true)

  // Swim lanes state
  const [swimLanes, setSwimLanes] = useState<SwimLane[]>([])
  const [editingLane, setEditingLane] = useState<number | null>(null)
  const [editLaneName, setEditLaneName] = useState('')
  const [editLaneColor, setEditLaneColor] = useState('')
  const [newLaneName, setNewLaneName] = useState('')
  const [newLaneColor, setNewLaneColor] = useState('#6B7280')
  const [newLaneStatusCategory, setNewLaneStatusCategory] = useState<'todo' | 'in_progress' | 'done'>('todo')
  const [swimLaneError, setSwimLaneError] = useState('')
  const [swimLaneSuccess, setSwimLaneSuccess] = useState('')

  useEffect(() => {
    loadProject()
    loadMembers()
    loadInvitations()
    loadTeamMembers()
    loadGitHubSettings()
    loadSwimLanes()
    loadStorageUsage()

    // Detect ?github=connected from OAuth callback
    const params = new URLSearchParams(window.location.search)
    if (params.get('github') === 'connected') {
      setGithubSuccess('GitHub connected successfully!')
      window.history.replaceState({}, '', window.location.pathname)
    } else if (params.get('github') === 'error') {
      setGithubError('GitHub connection failed. Please try again.')
      window.history.replaceState({}, '', window.location.pathname)
    }
  }, [projectId]) // eslint-disable-line react-hooks/exhaustive-deps

  const loadProject = async () => {
    try {
      const proj = await apiClient.getProject(projectId)
      setProject(proj)
    } catch (error: unknown) {
      console.error('Failed to load data:', error)
    }
  }

  const loadMembers = async () => {
    try {
      const data = await apiClient.getProjectMembers(projectId)
      setMembers(data)
      setProjectMembers(data.map(m => ({ user_id: m.user_id, email: m.email, name: m.name })))
    } catch (error: unknown) {
      console.error('Failed to load data:', error)
    }
  }

  const loadInvitations = async () => {
    try {
      const data = await apiClient.getProjectInvitations(projectId)
      setInvitations(data)
    } catch {
      // Not owner/admin — ignore
    }
  }

  const loadTeamMembers = async () => {
    try {
      const data = await apiClient.getTeamMembers()
      setTeamMembers(data)
    } catch (error: unknown) {
      console.error('Failed to load data:', error)
    }
  }

  const loadGitHubSettings = async () => {
    try {
      const data = await apiClient.getProjectGitHub(projectId)
      setGithubSettings(data)
      if (data.github_token_set) {
        loadGitHubRepos()
        if (data.github_owner) setSelectedRepoFullName(`${data.github_owner}/${data.github_repo_name}`)
      }
    } catch (error: unknown) {
      console.error('Failed to load data:', error)
    }
  }

  const loadGitHubRepos = async () => {
    setIsLoadingRepos(true)
    try {
      const repos = await apiClient.githubListRepos(projectId)
      setGithubRepos(repos)
    } catch {
      // Silently fail — token may have expired
    } finally {
      setIsLoadingRepos(false)
    }
  }

  const handleConnectGitHub = async () => {
    setIsConnectingGitHub(true)
    setGithubError('')
    try {
      const { auth_url } = await apiClient.githubOAuthInit(projectId)
      window.location.href = auth_url
    } catch (error: unknown) {
      setGithubError(error instanceof Error ? error.message : 'Failed to initiate GitHub connection')
      setIsConnectingGitHub(false)
    }
  }

  const handleDisconnectGitHub = async () => {
    setIsDisconnectingGitHub(true)
    setGithubError('')
    try {
      await apiClient.githubDisconnect(projectId)
      setGithubSettings(prev => ({ ...prev, github_token_set: false, github_login: null, github_owner: '', github_repo_name: '', github_repo_url: '' }))
      setGithubRepos([])
      setSelectedRepoFullName('')
      setGithubSuccess('')
    } catch (error: unknown) {
      setGithubError(error instanceof Error ? error.message : 'Failed to disconnect GitHub')
    } finally {
      setIsDisconnectingGitHub(false)
    }
  }

  const handleRepoSelect = async (fullName: string) => {
    setSelectedRepoFullName(fullName)
    const repo = githubRepos.find(r => r.full_name === fullName)
    if (!repo) return

    const updated = {
      github_owner: repo.owner,
      github_repo_name: repo.name,
      github_repo_url: repo.html_url,
      github_branch: repo.default_branch || 'main',
    }
    setGithubSettings(prev => ({ ...prev, ...updated }))

    // Auto-save so backend has owner/repo for preview/import
    try {
      await apiClient.updateProjectGitHub(projectId, updated)
    } catch {
      // Non-fatal — user can still save manually
    }
  }

  const handleFetchPreview = async () => {
    setImportError('')
    setImportSuccess('')
    setIsPreviewing(true)
    try {
      const preview = await apiClient.githubPreview(projectId)
      setGithubPreview(preview)
      // Initialize user assignments from auto-matched users
      const assignments: Record<string, number> = {}
      for (const u of preview.github_users) {
        assignments[u.login] = u.matched_user_id ?? 0
      }
      setUserAssignments(assignments)
    } catch (error: unknown) {
      setImportError(error instanceof Error ? error.message : 'Failed to fetch GitHub preview')
    } finally {
      setIsPreviewing(false)
    }
  }

  const handleImportFromGitHub = async () => {
    setImportError('')
    setImportSuccess('')
    setIsPulling(true)
    try {
      const result = await apiClient.githubPull(projectId, {
        pull_sprints: pullSprints,
        pull_tags: pullTags,
        pull_tasks: pullTasks,
        user_assignments: userAssignments,
      })
      setImportSuccess(`Imported: ${result.created_sprints} sprints, ${result.created_tags} tags, ${result.created_tasks} tasks (${result.skipped_tasks} skipped)`)
      loadGitHubSettings()
    } catch (error: unknown) {
      setImportError(error instanceof Error ? error.message : 'Import failed')
    } finally {
      setIsPulling(false)
    }
  }

  const handleSyncNow = async () => {
    setImportError('')
    setImportSuccess('')
    setIsSyncing(true)
    try {
      const result = await apiClient.githubSync(projectId)
      setImportSuccess(`Synced: ${result.created_tasks} new tasks, updated existing`)
      loadGitHubSettings()
    } catch (error: unknown) {
      setImportError(error instanceof Error ? error.message : 'Sync failed')
    } finally {
      setIsSyncing(false)
    }
  }

  const loadSwimLanes = async () => {
    try {
      const data = await apiClient.getSwimLanes(projectId)
      setSwimLanes(data)
    } catch (error: unknown) {
      console.error('Failed to load data:', error)
    }
  }

  const loadStorageUsage = async () => {
    try {
      setLoadingStorage(true)
      const data = await apiClient.getStorageUsage(projectId)
      setStorageUsage(data || [])
    } catch (error) {
      console.error('Failed to load storage usage:', error)
    } finally {
      setLoadingStorage(false)
    }
  }

  const formatBytes = (bytes: number): string => {
    if (bytes === 0) return '0 B'
    const units = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(1024))
    return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
  }

  const handleAddSwimLane = async () => {
    setSwimLaneError('')
    setSwimLaneSuccess('')

    if (!newLaneName.trim()) {
      setSwimLaneError('Swim lane name is required')
      return
    }

    if (swimLanes.length >= 6) {
      setSwimLaneError('Maximum 6 swim lanes allowed per project')
      return
    }

    try {
      await apiClient.createSwimLane(projectId, {
        name: newLaneName.trim(),
        color: newLaneColor,
        position: swimLanes.length,
        status_category: newLaneStatusCategory,
      })
      setSwimLaneSuccess('Swim lane created successfully')
      setNewLaneName('')
      setNewLaneColor('#6B7280')
      setNewLaneStatusCategory('todo')
      loadSwimLanes()
    } catch (error: unknown) {
      setSwimLaneError(error instanceof Error ? error.message : 'Failed to create swim lane')
    }
  }

  const handleUpdateSwimLane = async (laneId: number) => {
    setSwimLaneError('')
    setSwimLaneSuccess('')

    if (!editLaneName.trim()) {
      setSwimLaneError('Swim lane name is required')
      return
    }

    try {
      await apiClient.updateSwimLane(laneId, {
        name: editLaneName.trim(),
        color: editLaneColor,
      })
      setSwimLaneSuccess('Swim lane updated successfully')
      setEditingLane(null)
      loadSwimLanes()
    } catch (error: unknown) {
      setSwimLaneError(error instanceof Error ? error.message : 'Failed to update swim lane')
    }
  }

  const handleDeleteSwimLane = async (laneId: number) => {
    if (!confirm('Are you sure you want to delete this swim lane? Tasks using this swim lane will need to be reassigned.')) {
      return
    }

    if (swimLanes.length <= 2) {
      setSwimLaneError('Minimum 2 swim lanes required per project')
      return
    }

    try {
      await apiClient.deleteSwimLane(laneId)
      setSwimLaneSuccess('Swim lane deleted successfully')
      loadSwimLanes()
    } catch (error: unknown) {
      setSwimLaneError(error instanceof Error ? error.message : 'Failed to delete swim lane')
    }
  }

  const handleMoveSwimLane = async (laneId: number, direction: 'up' | 'down') => {
    const currentIndex = swimLanes.findIndex(l => l.id === laneId)
    if (currentIndex === -1) return
    if (direction === 'up' && currentIndex === 0) return
    if (direction === 'down' && currentIndex === swimLanes.length - 1) return

    const newIndex = direction === 'up' ? currentIndex - 1 : currentIndex + 1
    const swappedLane = swimLanes[newIndex]

    try {
      // Update positions sequentially to avoid UNIQUE constraint violation
      // Step 1: Move first lane to temporary high position
      await apiClient.updateSwimLane(laneId, { position: 999 })
      // Step 2: Move swapped lane to the target position
      await apiClient.updateSwimLane(swappedLane.id, { position: currentIndex })
      // Step 3: Move first lane to its final position
      await apiClient.updateSwimLane(laneId, { position: newIndex })
      loadSwimLanes()
    } catch (error: unknown) {
      setSwimLaneError(error instanceof Error ? error.message : 'Failed to reorder swim lanes')
    }
  }

  const handleInviteMember = async (e: React.FormEvent) => {
    e.preventDefault()
    setMemberError('')
    setMemberSuccess('')

    if (!selectedUserId) {
      setMemberError('Please select a team member')
      return
    }

    setIsAddingMember(true)

    try {
      await apiClient.inviteProjectMember(projectId, {
        user_id: parseInt(selectedUserId),
        role: newMemberRole,
      })

      setMemberSuccess('Invitation sent')
      setSelectedUserId('')
      setNewMemberRole('member')
      loadInvitations()
    } catch (error: unknown) {
      setMemberError(error instanceof Error ? error.message : 'Failed to send invitation')
    } finally {
      setIsAddingMember(false)
    }
  }

  const handleWithdrawInvitation = async (invId: number) => {
    try {
      await apiClient.withdrawProjectInvitation(invId)
      setMemberSuccess('Invitation withdrawn')
      loadInvitations()
    } catch (error: unknown) {
      setMemberError(error instanceof Error ? error.message : 'Failed to withdraw invitation')
    }
  }

  const handleResendInvitation = async (invId: number) => {
    try {
      await apiClient.resendProjectInvitation(invId)
      setMemberSuccess('Invitation resent')
      loadInvitations()
    } catch (error: unknown) {
      setMemberError(error instanceof Error ? error.message : 'Failed to resend invitation')
    }
  }

  // Filter team members that aren't already project members and don't have a pending invitation
  const availableTeamMembers = teamMembers.filter(
    tm => !members.some(pm => pm.user_id === tm.user_id) &&
          !invitations.some(inv => inv.invitee_user_id === tm.user_id && inv.status === 'pending')
  )

  const handleUpdateMemberRole = async (memberId: number, role: string) => {
    try {
      await apiClient.updateProjectMember(projectId, memberId, { role })
      setMemberSuccess('Member role updated successfully')
      loadMembers()
    } catch (error: unknown) {
      setMemberError(error instanceof Error ? error.message : 'Failed to update member role')
    }
  }

  const handleRemoveMember = async (memberId: number) => {
    if (!confirm('Are you sure you want to remove this member?')) {
      return
    }

    try {
      await apiClient.removeProjectMember(projectId, memberId)
      setMemberSuccess('Member removed successfully')
      loadMembers()
    } catch (error: unknown) {
      setMemberError(error instanceof Error ? error.message : 'Failed to remove member')
    }
  }

  const handleSaveGitHub = async (e: React.FormEvent) => {
    e.preventDefault()
    setGithubError('')
    setGithubSuccess('')

    setIsSavingGitHub(true)

    try {
      await apiClient.updateProjectGitHub(projectId, {
        github_repo_url: githubSettings.github_repo_url,
        github_owner: githubSettings.github_owner,
        github_repo_name: githubSettings.github_repo_name,
        github_branch: githubSettings.github_branch,
        github_sync_enabled: githubSettings.github_sync_enabled,
      })
      setGithubSuccess('GitHub settings saved successfully')
    } catch (error: unknown) {
      setGithubError(error instanceof Error ? error.message : 'Failed to save GitHub settings')
    } finally {
      setIsSavingGitHub(false)
    }
  }

  return (
    <div className="h-full flex flex-col bg-dark-bg-base">
      {/* Project Header */}
      <div className="bg-dark-bg-secondary border-b border-dark-border-subtle">
        {/* Top bar with project info and actions */}
        <div className="px-6 py-4 flex items-start justify-between gap-4">
          <div className="flex-1 min-w-0">
            <h1 className="text-xl font-semibold text-dark-text-primary truncate">
              {project?.name}
            </h1>
            {project?.description && (
              <p className="mt-1 text-sm text-dark-text-tertiary line-clamp-1">{project.description}</p>
            )}
          </div>
          <Button onClick={() => navigate(`/app/projects/${projectId}`)} variant="secondary" className="flex-shrink-0">
            <svg className="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" />
            </svg>
            Settings
          </Button>
        </div>

        {/* Navigation tabs */}
        <div className="px-6 flex items-end justify-between border-t border-dark-border-subtle/50">
          <div className="flex items-center gap-1">
            <button
              onClick={() => navigate(`/app/projects/${projectId}`)}
              className="px-4 py-3 text-sm font-medium text-dark-text-secondary hover:text-dark-text-primary transition-colors"
            >
              Board
            </button>
            <button
              onClick={() => navigate(`/app/projects/${projectId}/wiki`)}
              className="px-4 py-3 text-sm font-medium text-dark-text-secondary hover:text-dark-text-primary transition-colors"
            >
              Wiki
            </button>
            <button
              onClick={() => navigate(`/app/projects/${projectId}/settings`)}
              className="relative px-4 py-3 text-sm font-medium text-primary-400 transition-colors"
            >
              Settings
              <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-primary-500"></div>
            </button>
          </div>
          <div className="py-3"></div>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto bg-dark-bg-primary py-8">
        <div className="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8">

        <div className="space-y-6">
          {/* Team Members Section */}
          <Card className="shadow-md">
            <div className="p-6 sm:p-8">
              <div className="flex items-start gap-4 mb-6">
                <div className="flex-shrink-0 w-10 h-10 bg-primary-500/10 rounded-lg flex items-center justify-center">
                  <svg className="w-6 h-6 text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z" />
                  </svg>
                </div>
                <div className="flex-1">
                  <h2 className="text-xl font-semibold text-dark-text-primary mb-1">Team Members</h2>
                  <p className="text-sm text-dark-text-secondary">Share this project with other users</p>
                </div>
              </div>

              {memberSuccess && (
                <div className="mb-4 p-4 bg-success-500/10 border-l-4 border-success-500/30 rounded-r-lg">
                  <div className="flex items-center">
                    <svg className="w-5 h-5 text-success-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                      <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                    </svg>
                    <span className="text-success-300 font-medium">{memberSuccess}</span>
                  </div>
                </div>
              )}

              {memberError && <FormError message={memberError} className="mb-4" />}

              {/* Add Member Form */}
              <form onSubmit={handleInviteMember} className="mb-6 p-4 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                <h3 className="font-semibold text-dark-text-primary mb-4">Invite to Project</h3>
                <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                  <div className="md:col-span-2">
                    <label className="block text-sm font-medium text-dark-text-primary mb-1">
                      Team Member <span className="text-danger-400">*</span>
                    </label>
                    <SearchSelect
                      value={selectedUserId}
                      onChange={setSelectedUserId}
                      placeholder="Select a team member..."
                      options={availableTeamMembers.map(member => ({
                        value: String(member.user_id),
                        label: member.name || member.email,
                        description: member.name ? member.email : undefined,
                      }))}
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-dark-text-primary mb-1">
                      Role <span className="text-danger-400">*</span>
                    </label>
                    <SearchSelect
                      value={newMemberRole}
                      onChange={setNewMemberRole}
                      options={[
                        { value: 'viewer', label: 'Viewer' },
                        { value: 'member', label: 'Member' },
                        { value: 'editor', label: 'Editor' },
                        { value: 'owner', label: 'Owner' },
                      ]}
                    />
                  </div>
                </div>
                <div className="mt-4">
                  <Button type="submit" disabled={isAddingMember || availableTeamMembers.length === 0} size="sm">
                    {isAddingMember ? 'Sending...' : 'Send Invite'}
                  </Button>
                  {availableTeamMembers.length === 0 && (
                    <p className="text-sm text-dark-text-tertiary mt-2">All team members already have access or a pending invitation</p>
                  )}
                </div>
              </form>

              {/* Members List */}
              <div>
                <h3 className="font-semibold text-dark-text-primary mb-3">Current Members ({members.length})</h3>
                {members.length === 0 ? (
                  <div className="text-center py-8 text-dark-text-tertiary">
                    <svg className="w-12 h-12 mx-auto mb-3 text-dark-text-tertiary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
                    </svg>
                    <p>No members added yet</p>
                    <p className="text-sm mt-1">Add registered users to collaborate on this project</p>
                  </div>
                ) : (
                  <div className="space-y-2">
                    {members.map((member) => (
                      <div
                        key={member.id}
                        className="flex items-center justify-between p-4 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg hover:border-dark-border-subtle transition-colors"
                      >
                        <div className="flex items-center gap-3">
                          <div className="w-10 h-10 bg-gradient-to-br from-indigo-500 to-purple-600 rounded-full flex items-center justify-center text-white font-semibold">
                            {member.email.charAt(0).toUpperCase()}
                          </div>
                          <div>
                            <p className="font-medium text-dark-text-primary">{member.email}</p>
                            <p className="text-xs text-dark-text-tertiary">Added {new Date(member.granted_at).toLocaleDateString()}</p>
                          </div>
                        </div>
                        <div className="flex items-center gap-3">
                          <SearchSelect
                            variant="inline"
                            value={member.role}
                            onChange={(v) => handleUpdateMemberRole(member.id, v)}
                            options={[
                              { value: 'viewer', label: 'Viewer' },
                              { value: 'member', label: 'Member' },
                              { value: 'editor', label: 'Editor' },
                              { value: 'owner', label: 'Owner' },
                            ]}
                          />
                          <button
                            onClick={() => handleRemoveMember(member.id)}
                            className="p-2 text-danger-300 hover:bg-danger-500/10 rounded-lg transition-colors"
                            title="Remove member"
                          >
                            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                            </svg>
                          </button>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* Pending Invitations */}
              {invitations.filter(inv => ['pending', 'rejected', 'withdrawn'].includes(inv.status)).length > 0 && (
                <div className="mt-6">
                  <h3 className="font-semibold text-dark-text-primary mb-3">Invitations</h3>
                  <div className="space-y-2">
                    {invitations.filter(inv => ['pending', 'rejected', 'withdrawn'].includes(inv.status)).map((inv) => (
                      <div key={inv.id} className="flex items-center justify-between p-4 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                        <div className="flex items-center gap-3">
                          <div className="w-10 h-10 bg-gradient-to-br from-indigo-500/30 to-purple-600/30 rounded-full flex items-center justify-center text-white font-semibold text-sm">
                            {(inv.invitee_email || '?').charAt(0).toUpperCase()}
                          </div>
                          <div>
                            <p className="font-medium text-dark-text-primary">{inv.invitee_name || inv.invitee_email}</p>
                            <p className="text-xs text-dark-text-tertiary">Invited {new Date(inv.invited_at).toLocaleDateString()} · {inv.role}</p>
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          {inv.status === 'pending' && (
                            <>
                              <span className="px-2 py-1 text-xs font-medium bg-warning-500/10 text-warning-400 border border-warning-500/20 rounded-full">Pending</span>
                              <button
                                onClick={() => handleResendInvitation(inv.id)}
                                disabled={!inv.can_resend}
                                title={inv.can_resend ? 'Resend invitation' : 'Can resend in 2 days'}
                                className="px-2 py-1 text-xs text-dark-text-tertiary hover:text-dark-text-primary disabled:opacity-40 disabled:cursor-not-allowed border border-dark-border-subtle rounded transition-colors"
                              >
                                Resend
                              </button>
                              <button
                                onClick={() => handleWithdrawInvitation(inv.id)}
                                className="px-2 py-1 text-xs text-danger-400 hover:text-danger-300 border border-danger-500/20 rounded transition-colors"
                              >
                                Withdraw
                              </button>
                            </>
                          )}
                          {inv.status === 'rejected' && (
                            <span className="px-2 py-1 text-xs font-medium bg-dark-bg-tertiary text-dark-text-tertiary border border-dark-border-subtle rounded-full">Rejected</span>
                          )}
                          {inv.status === 'withdrawn' && (
                            <span className="px-2 py-1 text-xs font-medium bg-dark-bg-tertiary text-dark-text-tertiary border border-dark-border-subtle rounded-full">Withdrawn</span>
                          )}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Role Descriptions */}
              <div className="mt-6 p-4 bg-primary-500/10 border border-primary-500/30 rounded-lg">
                <h4 className="font-semibold text-primary-300 mb-2 text-sm">Role Permissions</h4>
                <ul className="text-sm text-primary-300 space-y-1">
                  <li><strong>Viewer:</strong> Can view project and tasks</li>
                  <li><strong>Editor:</strong> Can view, create, and edit tasks</li>
                  <li><strong>Admin:</strong> Full access including managing members and settings</li>
                </ul>
              </div>
            </div>
          </Card>

          {/* Swim Lanes Section */}
          <Card className="shadow-md">
            <div className="p-6 sm:p-8">
              <div className="flex items-start gap-4 mb-6">
                <div className="flex-shrink-0 w-10 h-10 bg-primary-500/10 rounded-lg flex items-center justify-center">
                  <svg className="w-6 h-6 text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 17V7m0 10a2 2 0 01-2 2H5a2 2 0 01-2-2V7a2 2 0 012-2h2a2 2 0 012 2m0 10a2 2 0 002 2h2a2 2 0 002-2M9 7a2 2 0 012-2h2a2 2 0 012 2m0 10V7m0 10a2 2 0 002 2h2a2 2 0 002-2V7a2 2 0 00-2-2h-2a2 2 0 00-2 2" />
                  </svg>
                </div>
                <div className="flex-1">
                  <h2 className="text-xl font-semibold text-dark-text-primary mb-1">Swim Lanes</h2>
                  <p className="text-sm text-dark-text-secondary">Customize the columns on your Kanban board (min: 2, max: 6)</p>
                </div>
              </div>

              {swimLaneSuccess && (
                <div className="mb-4 p-4 bg-success-500/10 border-l-4 border-success-500/30 rounded-r-lg">
                  <div className="flex items-center">
                    <svg className="w-5 h-5 text-success-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                      <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                    </svg>
                    <span className="text-success-300 font-medium">{swimLaneSuccess}</span>
                  </div>
                </div>
              )}

              {swimLaneError && <FormError message={swimLaneError} className="mb-4" />}

              {/* Add Swim Lane Form */}
              {swimLanes.length < 6 && (
                <div className="mb-6 p-4 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                  <h3 className="font-semibold text-dark-text-primary mb-4">Add New Swim Lane</h3>
                  <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                    <div className="md:col-span-2">
                      <label className="block text-sm font-medium text-dark-text-primary mb-1">
                        Name <span className="text-danger-400">*</span>
                      </label>
                      <input
                        type="text"
                        value={newLaneName}
                        onChange={(e) => setNewLaneName(e.target.value)}
                        placeholder="e.g., In Review, Testing"
                        className="w-full px-3 py-2 bg-dark-bg-secondary border border-dark-border-subtle text-dark-text-primary rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 outline-none transition-colors"
                        maxLength={50}
                      />
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-dark-text-primary mb-1">
                        Color
                      </label>
                      <input
                        type="color"
                        value={newLaneColor}
                        onChange={(e) => setNewLaneColor(e.target.value)}
                        className="w-full h-[42px] px-2 py-1 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 outline-none transition-colors cursor-pointer"
                      />
                    </div>
                  </div>
                  <div className="mt-3">
                    <label className="block text-sm font-medium text-dark-text-primary mb-1">
                      Status Category <span className="text-danger-400">*</span>
                    </label>
                    <SearchSelect
                      value={newLaneStatusCategory}
                      onChange={(v) => setNewLaneStatusCategory(v as 'todo' | 'in_progress' | 'done')}
                      options={[
                        { value: 'todo', label: 'To Do' },
                        { value: 'in_progress', label: 'In Progress' },
                        { value: 'done', label: 'Done' },
                      ]}
                    />
                  </div>
                  <div className="mt-4">
                    <Button onClick={handleAddSwimLane} disabled={!newLaneName.trim()} size="sm">
                      Add Swim Lane
                    </Button>
                  </div>
                </div>
              )}

              {/* Swim Lanes List */}
              <div>
                <h3 className="font-semibold text-dark-text-primary mb-3">Current Swim Lanes ({swimLanes.length})</h3>
                <div className="space-y-2">
                  {swimLanes.map((lane, index) => (
                    <div
                      key={lane.id}
                      className="flex items-center gap-3 p-4 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg hover:border-dark-border-subtle transition-colors"
                    >
                      {/* Color indicator */}
                      <div
                        className="w-4 h-4 rounded-full flex-shrink-0"
                        style={{ backgroundColor: lane.color }}
                      />

                      {editingLane === lane.id ? (
                        <>
                          <input
                            type="text"
                            value={editLaneName}
                            onChange={(e) => setEditLaneName(e.target.value)}
                            className="flex-1 px-3 py-1 bg-dark-bg-primary border border-dark-border-subtle text-dark-text-primary rounded-md focus:ring-1 focus:ring-primary-500 focus:border-primary-500 outline-none text-sm"
                            maxLength={50}
                          />
                          <input
                            type="color"
                            value={editLaneColor}
                            onChange={(e) => setEditLaneColor(e.target.value)}
                            className="w-12 h-8 px-1 bg-dark-bg-primary border border-dark-border-subtle rounded-md cursor-pointer"
                          />
                          <button
                            onClick={() => handleUpdateSwimLane(lane.id)}
                            className="px-3 py-1 bg-primary-500 hover:bg-primary-600 text-white text-sm rounded-md transition-colors"
                          >
                            Save
                          </button>
                          <button
                            onClick={() => setEditingLane(null)}
                            className="px-3 py-1 bg-dark-bg-secondary hover:bg-dark-bg-tertiary text-dark-text-secondary text-sm rounded-md transition-colors"
                          >
                            Cancel
                          </button>
                        </>
                      ) : (
                        <>
                          <div className="flex-1">
                            <span className="font-medium text-dark-text-primary">{lane.name}</span>
                            <span className="ml-2 text-xs text-dark-text-tertiary">Position: {lane.position + 1}</span>
                          </div>
                          <div className="flex items-center gap-1">
                            {/* Move up */}
                            <button
                              onClick={() => handleMoveSwimLane(lane.id, 'up')}
                              disabled={index === 0}
                              className="p-1.5 text-dark-text-secondary hover:text-dark-text-primary hover:bg-dark-bg-secondary rounded transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
                              title="Move up"
                            >
                              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 15l7-7 7 7" />
                              </svg>
                            </button>
                            {/* Move down */}
                            <button
                              onClick={() => handleMoveSwimLane(lane.id, 'down')}
                              disabled={index === swimLanes.length - 1}
                              className="p-1.5 text-dark-text-secondary hover:text-dark-text-primary hover:bg-dark-bg-secondary rounded transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
                              title="Move down"
                            >
                              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                              </svg>
                            </button>
                            {/* Edit */}
                            <button
                              onClick={() => {
                                setEditingLane(lane.id)
                                setEditLaneName(lane.name)
                                setEditLaneColor(lane.color)
                              }}
                              className="p-1.5 text-primary-400 hover:text-primary-300 hover:bg-primary-500/10 rounded transition-colors"
                              title="Edit"
                            >
                              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                              </svg>
                            </button>
                            {/* Delete */}
                            <button
                              onClick={() => handleDeleteSwimLane(lane.id)}
                              disabled={swimLanes.length <= 2}
                              className="p-1.5 text-danger-400 hover:text-danger-300 hover:bg-danger-500/10 rounded transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
                              title="Delete"
                            >
                              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                              </svg>
                            </button>
                          </div>
                        </>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </Card>

          {/* Storage Usage Section */}
          <Card className="shadow-md">
            <div className="p-6 sm:p-8">
              <div className="flex items-start gap-4 mb-6">
                <div className="flex-shrink-0 w-10 h-10 bg-orange-500/10 rounded-lg flex items-center justify-center">
                  <svg className="w-6 h-6 text-orange-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4m0 5c0 2.21-3.582 4-8 4s-8-1.79-8-4" />
                  </svg>
                </div>
                <div className="flex-1">
                  <h2 className="text-xl font-semibold text-dark-text-primary mb-1">Storage Usage</h2>
                  <p className="text-sm text-dark-text-secondary">Track file uploads and storage per team member</p>
                </div>
              </div>

              {loadingStorage ? (
                <div className="animate-pulse space-y-3">
                  <div className="h-16 bg-dark-bg-tertiary/30 rounded-lg"></div>
                  <div className="h-10 bg-dark-bg-tertiary/30 rounded-lg"></div>
                </div>
              ) : storageUsage.length === 0 ? (
                <div className="text-center py-8 text-dark-text-tertiary">
                  <svg className="w-12 h-12 mx-auto mb-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4m0 5c0 2.21-3.582 4-8 4s-8-1.79-8-4" />
                  </svg>
                  <p>No files uploaded yet</p>
                </div>
              ) : (
                <>
                  {/* Total project storage */}
                  <div className="mb-6 p-4 bg-dark-bg-primary border border-dark-border-subtle rounded-lg">
                    <div className="flex items-center justify-between">
                      <div>
                        <p className="text-sm text-dark-text-secondary">Total Project Storage</p>
                        <p className="text-2xl font-bold text-dark-text-primary">
                          {formatBytes(storageUsage.reduce((sum, u) => sum + u.total_size, 0))}
                        </p>
                      </div>
                      <div className="text-right">
                        <p className="text-sm text-dark-text-secondary">Total Files</p>
                        <p className="text-2xl font-bold text-dark-text-primary">
                          {storageUsage.reduce((sum, u) => sum + u.file_count, 0)}
                        </p>
                      </div>
                    </div>
                  </div>

                  {/* Per-user table */}
                  <div>
                    <h3 className="font-semibold text-dark-text-primary mb-3">Usage by Member</h3>
                    <div className="space-y-2">
                      {storageUsage.map((usage) => {
                        const maxSize = Math.max(...storageUsage.map(u => u.total_size))
                        const barWidth = maxSize > 0 ? (usage.total_size / maxSize) * 100 : 0
                        return (
                          <div
                            key={usage.user_id}
                            className="p-4 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg"
                          >
                            <div className="flex items-center justify-between mb-2">
                              <div className="flex items-center gap-3">
                                <div className="w-8 h-8 bg-primary-500/10 rounded-full flex items-center justify-center">
                                  <span className="text-xs font-medium text-primary-400">
                                    {(usage.user_name || 'U').charAt(0).toUpperCase()}
                                  </span>
                                </div>
                                <span className="font-medium text-dark-text-primary text-sm">
                                  {usage.user_name || `User ${usage.user_id}`}
                                </span>
                              </div>
                              <div className="text-right">
                                <span className="text-sm font-medium text-dark-text-primary">{formatBytes(usage.total_size)}</span>
                                <span className="text-xs text-dark-text-tertiary ml-2">{usage.file_count} file{usage.file_count !== 1 ? 's' : ''}</span>
                              </div>
                            </div>
                            <div className="h-1.5 bg-dark-bg-primary rounded-full overflow-hidden">
                              <div
                                className="h-full bg-primary-500 rounded-full transition-all duration-300"
                                style={{ width: `${barWidth}%` }}
                              />
                            </div>
                          </div>
                        )
                      })}
                    </div>
                  </div>
                </>
              )}
            </div>
          </Card>

          {/* GitHub Integration Section */}
          <Card className="shadow-md">
            <div className="p-6 sm:p-8">
              <div className="flex items-start gap-4 mb-6">
                <div className="flex-shrink-0 w-10 h-10 bg-gray-900 rounded-lg flex items-center justify-center">
                  <svg className="w-6 h-6 text-white" fill="currentColor" viewBox="0 0 24 24">
                    <path fillRule="evenodd" d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" clipRule="evenodd" />
                  </svg>
                </div>
                <div className="flex-1">
                  <h2 className="text-xl font-semibold text-dark-text-primary mb-1">GitHub Integration</h2>
                  <p className="text-sm text-dark-text-secondary">Connect this project to a GitHub repository</p>
                </div>
              </div>

              {githubSuccess && (
                <div className="mb-4 p-4 bg-success-500/10 border-l-4 border-success-500/30 rounded-r-lg">
                  <div className="flex items-center">
                    <svg className="w-5 h-5 text-success-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                      <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                    </svg>
                    <span className="text-success-300 font-medium">{githubSuccess}</span>
                  </div>
                </div>
              )}

              {githubError && <FormError message={githubError} className="mb-4" />}

              {!githubSettings.github_token_set ? (
                /* --- Not connected --- */
                <div className="py-4">
                  <p className="text-sm text-dark-text-secondary mb-4">Connect this project to your GitHub account to pick a repository.</p>
                  <Button onClick={handleConnectGitHub} disabled={isConnectingGitHub}>
                    {isConnectingGitHub ? 'Redirecting...' : 'Connect with GitHub'}
                  </Button>
                </div>
              ) : (
                /* --- Connected --- */
                <form onSubmit={handleSaveGitHub} className="space-y-4">
                  {/* Connected as badge + disconnect */}
                  <div className="flex items-center justify-between p-3 bg-success-500/10 border border-success-500/20 rounded-lg">
                    <div className="flex items-center gap-2">
                      <svg className="w-4 h-4 text-success-400" fill="currentColor" viewBox="0 0 20 20">
                        <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                      </svg>
                      <span className="text-sm font-medium text-success-300">
                        Connected{githubSettings.github_login ? ` as @${githubSettings.github_login}` : ''}
                      </span>
                    </div>
                    <button
                      type="button"
                      onClick={handleDisconnectGitHub}
                      disabled={isDisconnectingGitHub}
                      className="text-xs text-dark-text-tertiary hover:text-error-400 transition-colors"
                    >
                      {isDisconnectingGitHub ? 'Disconnecting...' : 'Disconnect'}
                    </button>
                  </div>

                  {/* Repository picker */}
                  <div>
                    <label className="block text-sm font-medium text-dark-text-primary mb-1">Repository</label>
                    {isLoadingRepos ? (
                      <div className="text-sm text-dark-text-tertiary py-2">Loading repositories...</div>
                    ) : (
                      <>
                        <input
                          type="text"
                          value={repoSearchQuery}
                          onChange={(e) => setRepoSearchQuery(e.target.value)}
                          placeholder="Search or select a repository..."
                          className="w-full px-3 py-2 mb-1 bg-dark-bg-secondary border border-dark-border-subtle text-dark-text-primary rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 outline-none transition-colors text-sm"
                        />
                        <select
                          value={selectedRepoFullName}
                          onChange={(e) => handleRepoSelect(e.target.value)}
                          className="w-full px-3 py-2 bg-dark-bg-secondary border border-dark-border-subtle text-dark-text-primary rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 outline-none transition-colors text-sm"
                          size={Math.min(8, githubRepos.filter(r => !repoSearchQuery || r.full_name.toLowerCase().includes(repoSearchQuery.toLowerCase())).length + 1)}
                        >
                          <option value="">— Select a repository —</option>
                          {githubRepos
                            .filter(r => !repoSearchQuery || r.full_name.toLowerCase().includes(repoSearchQuery.toLowerCase()))
                            .map(r => (
                              <option key={r.id} value={r.full_name}>
                                {r.full_name}{r.private ? ' 🔒' : ''}
                              </option>
                            ))}
                        </select>
                      </>
                    )}
                  </div>

                  <TextInput
                    label="Branch"
                    type="text"
                    value={githubSettings.github_branch}
                    onChange={(e) => setGithubSettings({ ...githubSettings, github_branch: e.target.value })}
                    placeholder="main"
                    helpText="The default branch to track (e.g., main, master, develop)"
                  />

                  <div className="flex items-center gap-3 p-4 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                    <input
                      type="checkbox"
                      id="sync-enabled"
                      checked={githubSettings.github_sync_enabled}
                      onChange={(e) => setGithubSettings({ ...githubSettings, github_sync_enabled: e.target.checked })}
                      className="w-4 h-4 text-primary-600 border-dark-border-subtle rounded focus:ring-2 focus:ring-primary-500"
                    />
                    <label htmlFor="sync-enabled" className="flex-1">
                      <span className="font-medium text-dark-text-primary">Enable GitHub Sync</span>
                      <p className="text-sm text-dark-text-secondary mt-0.5">Automatically sync tasks with GitHub issues</p>
                    </label>
                  </div>

                  <Button type="submit" disabled={isSavingGitHub}>
                    {isSavingGitHub ? 'Saving...' : 'Save Settings'}
                  </Button>
                </form>
              )}

              {/* Import Section — shown when owner + repo are configured */}
              {githubSettings.github_owner && githubSettings.github_repo_name && (
                <div className="mt-8 pt-6 border-t border-dark-border-subtle">
                  <h3 className="text-lg font-semibold text-dark-text-primary mb-1">Import from GitHub</h3>
                  <p className="text-sm text-dark-text-secondary mb-4">
                    Pull milestones → sprints, labels → tags, and issues → tasks into this project.
                  </p>

                  {importSuccess && (
                    <div className="mb-4 p-4 bg-success-500/10 border-l-4 border-success-500/30 rounded-r-lg">
                      <div className="flex items-center">
                        <svg className="w-5 h-5 text-success-400 mr-2 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                          <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                        </svg>
                        <span className="text-success-300 font-medium">{importSuccess}</span>
                      </div>
                    </div>
                  )}

                  {importError && <FormError message={importError} className="mb-4" />}

                  {!githubPreview ? (
                    <Button onClick={handleFetchPreview} disabled={isPreviewing} variant="secondary">
                      {isPreviewing ? 'Fetching...' : 'Fetch Preview'}
                    </Button>
                  ) : (
                    <div className="space-y-4">
                      {/* Counts */}
                      <div className="flex flex-wrap gap-3">
                        <div className="px-3 py-2 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg text-sm">
                          <span className="font-semibold text-dark-text-primary">{githubPreview.milestone_count}</span>
                          <span className="text-dark-text-secondary ml-1">milestones</span>
                        </div>
                        <div className="px-3 py-2 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg text-sm">
                          <span className="font-semibold text-dark-text-primary">{githubPreview.label_count}</span>
                          <span className="text-dark-text-secondary ml-1">labels</span>
                        </div>
                        <div className="px-3 py-2 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg text-sm">
                          <span className="font-semibold text-dark-text-primary">{githubPreview.issue_count}</span>
                          <span className="text-dark-text-secondary ml-1">issues</span>
                        </div>
                        <button
                          onClick={() => { setGithubPreview(null); setImportError(''); setImportSuccess('') }}
                          className="px-3 py-2 text-xs text-dark-text-tertiary hover:text-dark-text-secondary border border-dark-border-subtle rounded-lg transition-colors"
                        >
                          Refresh
                        </button>
                      </div>

                      {/* User mapping */}
                      {githubPreview.github_users.length > 0 && (
                        <div>
                          <h4 className="text-sm font-medium text-dark-text-primary mb-2">Map GitHub users to TaskAI members</h4>
                          <div className="space-y-2">
                            {githubPreview.github_users.map((gu: GitHubUserMatch) => (
                              <div key={gu.login} className="flex items-center gap-3 p-3 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                                <div className="w-7 h-7 bg-gray-700 rounded-full flex items-center justify-center text-xs font-medium text-gray-300 flex-shrink-0">
                                  {gu.login.charAt(0).toUpperCase()}
                                </div>
                                <div className="flex-1 min-w-0">
                                  <span className="text-sm font-medium text-dark-text-primary">{gu.login}</span>
                                  {gu.name && <span className="text-xs text-dark-text-tertiary ml-1">({gu.name})</span>}
                                </div>
                                <select
                                  value={userAssignments[gu.login] ?? 0}
                                  onChange={(e) => setUserAssignments(prev => ({ ...prev, [gu.login]: Number(e.target.value) }))}
                                  className="text-sm bg-dark-bg-primary border border-dark-border-subtle text-dark-text-primary rounded-md px-2 py-1 focus:outline-none focus:ring-1 focus:ring-primary-500"
                                >
                                  <option value={0}>Unassigned</option>
                                  {projectMembers.map(m => (
                                    <option key={m.user_id} value={m.user_id}>
                                      {m.name || m.email}
                                    </option>
                                  ))}
                                </select>
                              </div>
                            ))}
                          </div>
                        </div>
                      )}

                      {/* Options */}
                      <div className="flex flex-wrap gap-4">
                        <label className="flex items-center gap-2 text-sm text-dark-text-primary cursor-pointer">
                          <input type="checkbox" checked={pullSprints} onChange={e => setPullSprints(e.target.checked)}
                            className="w-4 h-4 text-primary-600 border-dark-border-subtle rounded focus:ring-primary-500" />
                          Import Sprints (milestones)
                        </label>
                        <label className="flex items-center gap-2 text-sm text-dark-text-primary cursor-pointer">
                          <input type="checkbox" checked={pullTags} onChange={e => setPullTags(e.target.checked)}
                            className="w-4 h-4 text-primary-600 border-dark-border-subtle rounded focus:ring-primary-500" />
                          Import Tags (labels)
                        </label>
                        <label className="flex items-center gap-2 text-sm text-dark-text-primary cursor-pointer">
                          <input type="checkbox" checked={pullTasks} onChange={e => setPullTasks(e.target.checked)}
                            className="w-4 h-4 text-primary-600 border-dark-border-subtle rounded focus:ring-primary-500" />
                          Import Tasks (issues)
                        </label>
                      </div>

                      <div className="flex items-center gap-3">
                        <Button onClick={handleImportFromGitHub} disabled={isPulling || (!pullSprints && !pullTags && !pullTasks)}>
                          {isPulling ? 'Importing...' : 'Import from GitHub'}
                        </Button>
                      </div>
                    </div>
                  )}

                  {/* Sync Now (shown after first sync) */}
                  {githubSettings.github_last_sync && (
                    <div className="mt-4 pt-4 border-t border-dark-border-subtle flex items-center gap-4">
                      <span className="text-sm text-dark-text-secondary">
                        Last synced: {new Date(githubSettings.github_last_sync).toLocaleString()}
                      </span>
                      <Button onClick={handleSyncNow} disabled={isSyncing} variant="secondary" size="sm">
                        {isSyncing ? 'Syncing...' : 'Sync Now'}
                      </Button>
                    </div>
                  )}
                </div>
              )}
            </div>
          </Card>
        </div>
        </div>
      </div>
    </div>
  )
}
