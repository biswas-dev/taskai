import { useState, useEffect, useMemo, useRef } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useAuth } from '../state/AuthContext'
import Card from '../components/ui/Card'
import Button from '../components/ui/Button'
import TextInput from '../components/ui/TextInput'
import FormError from '../components/ui/FormError'
import SearchSelect from '../components/ui/SearchSelect'
import { apiClient, type SwimLane, type Project, type ProjectInvitation, type GitHubPreviewResponse, type GitHubUserMatch, type GitHubRepo, type GitHubStatusMatch, type GitHubProgressEvent, type GitHubMilestone, type GitHubLabel } from '../lib/api'

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
  github_push_enabled: boolean
  github_last_sync: string | null
  github_token_set: boolean
  github_login: string | null
}

// ── GitHub-style filter bar ───────────────────────────────────────────────────
type FilterCategory = 'milestone' | 'assignee' | 'label' | 'state'

interface GitHubFilterBarProps {
  milestones: GitHubMilestone[]
  assignees: GitHubUserMatch[]
  labels: GitHubLabel[]
  filterMilestone: number | undefined
  filterAssignee: string
  filterLabels: string[]
  filterState: 'all' | 'open' | 'closed'
  onChange: (patch: {
    milestone?: number | undefined
    assignee?: string
    labels?: string[]
    state?: 'all' | 'open' | 'closed'
  }) => void
}

function GitHubFilterBar({
  milestones, assignees, labels,
  filterMilestone, filterAssignee, filterLabels, filterState,
  onChange,
}: GitHubFilterBarProps) {
  const [open, setOpen] = useState(false)
  const [category, setCategory] = useState<FilterCategory | null>(null)
  const [search, setSearch] = useState('')
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false); setCategory(null); setSearch('')
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  const CATEGORIES: { id: FilterCategory; label: string; icon: React.ReactNode }[] = [
    {
      id: 'milestone', label: 'Milestone',
      icon: <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M3 3l9 9m0 0l9-9M12 12v9M3 3v6h6" /></svg>,
    },
    {
      id: 'assignee', label: 'Assignee',
      icon: <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg>,
    },
    {
      id: 'label', label: 'Label',
      icon: <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A2 2 0 013 12V7a4 4 0 014-4z" /></svg>,
    },
    {
      id: 'state', label: 'State',
      icon: <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><circle cx="12" cy="12" r="9" strokeWidth={1.5} /><path strokeLinecap="round" strokeWidth={1.5} d="M12 8v4m0 4h.01" /></svg>,
    },
  ]

  // Active filter chips
  const chips: { key: FilterCategory; label: string }[] = []
  if (filterMilestone) chips.push({ key: 'milestone', label: `milestone:"${milestones.find(m => m.number === filterMilestone)?.title ?? filterMilestone}"` })
  if (filterAssignee) chips.push({ key: 'assignee', label: filterAssignee === 'none' ? 'no assignee' : `assignee:"${filterAssignee}"` })
  if (filterLabels.length > 0) chips.push({ key: 'label', label: `label:"${filterLabels[0]}"` })
  if (filterState !== 'all') chips.push({ key: 'state', label: `is:${filterState}` })

  const removeChip = (key: FilterCategory) => {
    if (key === 'milestone') onChange({ milestone: undefined })
    if (key === 'assignee') onChange({ assignee: '' })
    if (key === 'label') onChange({ labels: [] })
    if (key === 'state') onChange({ state: 'all' })
  }

  const selectOption = (cat: FilterCategory, value: string) => {
    if (cat === 'milestone') onChange({ milestone: value ? Number(value) : undefined })
    if (cat === 'assignee') onChange({ assignee: value })
    if (cat === 'label') onChange({ labels: value ? [value] : [] })
    if (cat === 'state') onChange({ state: value as 'all' | 'open' | 'closed' })
    setOpen(false); setCategory(null); setSearch('')
  }

  const q = search.toLowerCase()
  let options: { value: string; label: string; sub?: string; color?: string }[] = []
  if (category === 'milestone') {
    options = milestones
      .filter(m => !q || m.title.toLowerCase().includes(q))
      .map(m => ({ value: String(m.number), label: m.title, sub: m.state === 'closed' ? 'closed' : undefined }))
  } else if (category === 'assignee') {
    const base = [{ value: 'none', label: 'No assignee' }]
    const users = assignees
      .filter(u => !q || u.login.toLowerCase().includes(q) || (u.name ?? '').toLowerCase().includes(q))
      .map(u => ({ value: u.login, label: u.login, sub: u.name || undefined }))
    options = [...base, ...users]
  } else if (category === 'label') {
    options = labels
      .filter(l => !q || l.name.toLowerCase().includes(q))
      .map(l => ({ value: l.name, label: l.name, color: l.color ? '#' + l.color : undefined }))
  } else if (category === 'state') {
    options = [
      { value: 'open', label: 'Open issues' },
      { value: 'closed', label: 'Closed issues' },
    ]
  }

  const activeValue = (cat: FilterCategory) => {
    if (cat === 'milestone') return filterMilestone ? milestones.find(m => m.number === filterMilestone)?.title : undefined
    if (cat === 'assignee') return filterAssignee || undefined
    if (cat === 'label') return filterLabels[0]
    if (cat === 'state') return filterState !== 'all' ? filterState : undefined
  }

  return (
    <div ref={ref} className="relative">
      {/* Bar */}
      <button
        type="button"
        onClick={() => { setOpen(v => !v); setCategory(null); setSearch('') }}
        className="w-full flex items-center gap-2 px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-left hover:border-[#484f58] transition-colors focus:outline-none focus:border-primary-500"
      >
        <svg className="w-4 h-4 text-[#8b949e] flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
        <div className="flex flex-wrap gap-1.5 flex-1 min-h-[20px]">
          {chips.map(c => (
            <span key={c.key} className="inline-flex items-center gap-1 px-2 py-0.5 bg-[#1f6feb]/20 border border-[#1f6feb]/40 text-[#79c0ff] rounded text-xs font-mono whitespace-nowrap">
              {c.label}
              <span
                role="button"
                onClick={e => { e.stopPropagation(); removeChip(c.key) }}
                className="ml-0.5 text-[#8b949e] hover:text-white cursor-pointer leading-none"
              >×</span>
            </span>
          ))}
          {chips.length === 0 && <span className="text-sm text-[#8b949e]">Filter issues to import…</span>}
        </div>
      </button>

      {/* Dropdown */}
      {open && (
        <div className="absolute top-full left-0 right-0 mt-1 bg-[#161b22] border border-[#30363d] rounded-lg shadow-2xl z-50 overflow-hidden text-sm">
          {!category ? (
            <>
              <div className="px-3 py-2 text-xs text-[#8b949e] font-semibold border-b border-[#30363d]">Filter by</div>
              {CATEGORIES.map(cat => (
                <button
                  key={cat.id}
                  type="button"
                  onClick={() => { setCategory(cat.id); setSearch('') }}
                  className="w-full flex items-center gap-3 px-3 py-2.5 text-[#c9d1d9] hover:bg-[#1f6feb]/10 transition-colors"
                >
                  <span className="text-[#8b949e] w-4">{cat.icon}</span>
                  <span className="flex-1 text-left">{cat.label}</span>
                  {activeValue(cat.id) && (
                    <span className="text-xs text-[#79c0ff] font-mono truncate max-w-[120px]">{activeValue(cat.id)}</span>
                  )}
                  <svg className="w-3 h-3 text-[#8b949e]" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" /></svg>
                </button>
              ))}
            </>
          ) : (
            <>
              <div className="flex items-center gap-2 px-3 py-2 border-b border-[#30363d]">
                <button type="button" onClick={() => { setCategory(null); setSearch('') }} className="text-[#8b949e] hover:text-[#c9d1d9] transition-colors">
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" /></svg>
                </button>
                <span className="text-xs text-[#8b949e] font-semibold uppercase tracking-wide">Filter by {category}</span>
              </div>
              {category !== 'state' && (
                <div className="px-3 py-2 border-b border-[#30363d]">
                  <input
                    autoFocus
                    type="text"
                    placeholder={`Search ${category}s…`}
                    value={search}
                    onChange={e => setSearch(e.target.value)}
                    className="w-full text-sm bg-transparent text-[#c9d1d9] placeholder-[#8b949e] outline-none"
                  />
                </div>
              )}
              <div className="max-h-52 overflow-y-auto">
                {options.length === 0 ? (
                  <div className="px-3 py-3 text-[#8b949e]">No results</div>
                ) : options.map(opt => {
                  const isActive =
                    (category === 'milestone' && filterMilestone === Number(opt.value)) ||
                    (category === 'assignee' && filterAssignee === opt.value) ||
                    (category === 'label' && filterLabels.includes(opt.value)) ||
                    (category === 'state' && filterState === opt.value)
                  return (
                    <button
                      key={opt.value}
                      type="button"
                      onClick={() => selectOption(category, opt.value)}
                      className={`w-full flex items-center gap-3 px-3 py-2 hover:bg-[#1f6feb]/10 transition-colors ${isActive ? 'text-[#79c0ff]' : 'text-[#c9d1d9]'}`}
                    >
                      <span className={`w-3.5 h-3.5 rounded-full border-2 flex-shrink-0 transition-colors ${isActive ? 'bg-[#1f6feb] border-[#1f6feb]' : 'border-[#484f58]'}`} />
                      {opt.color && <span className="w-3 h-3 rounded-full flex-shrink-0" style={{ backgroundColor: opt.color.startsWith('#') ? opt.color : '#' + opt.color }} />}
                      <span className="flex-1 text-left">{opt.label}</span>
                      {opt.sub && <span className="text-xs text-[#8b949e]">{opt.sub}</span>}
                    </button>
                  )
                })}
              </div>
            </>
          )}
        </div>
      )}
    </div>
  )
}
// ─────────────────────────────────────────────────────────────────────────────

interface ProjectSettingsProps {
  embedded?: boolean
  projectIdOverride?: number
}

export default function ProjectSettings({ embedded, projectIdOverride }: ProjectSettingsProps = {}) {
  const navigate = useNavigate()
  const { projectId: projectIdParam } = useParams<{ projectId: string }>()
  const projectId = projectIdOverride || parseInt(projectIdParam || '0')
  const { user } = useAuth()

  // Project state
  const [project, setProject] = useState<Project | null>(null)

  // Members state
  const [members, setMembers] = useState<ProjectMember[]>([])
  const isOwnerOrAdmin = useMemo(
    () => members.some(m => m.user_id === user?.id && (m.role === 'owner' || m.role === 'admin')),
    [members, user]
  )
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
    github_push_enabled: false,
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
  const [previewProgress, setPreviewProgress] = useState<GitHubProgressEvent | null>(null)
  const [isPulling, setIsPulling] = useState(false)
  const [isSyncing, setIsSyncing] = useState(false)
  const [isPushingAll, setIsPushingAll] = useState(false)
  const [pushAllProgress, setPushAllProgress] = useState<GitHubProgressEvent | null>(null)
  const [importError, setImportError] = useState('')
  const [importSuccess, setImportSuccess] = useState('')
  const [importProgress, setImportProgress] = useState<GitHubProgressEvent | null>(null)
  const [userAssignments, setUserAssignments] = useState<Record<string, number>>({})
  const [statusAssignments, setStatusAssignments] = useState<Record<string, number>>({})
  const [isSavingMappings, setIsSavingMappings] = useState(false)
  const [mappingsSaved, setMappingsSaved] = useState(false)
  const [pullSprints, setPullSprints] = useState(true)
  const [pullTags, setPullTags] = useState(true)
  const [pullTasks, setPullTasks] = useState(true)
  const [pullComments, setPullComments] = useState(true)
  // Import filters
  const [filterMilestone, setFilterMilestone] = useState<number | undefined>(undefined)
  const [filterAssignee, setFilterAssignee] = useState('')
  const [filterLabels, setFilterLabels] = useState<string[]>([])
  const [filterState, setFilterState] = useState<'all' | 'open' | 'closed'>('all')

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
      const url = new URL(window.location.href)
      url.searchParams.delete('github')
      window.history.replaceState({}, '', url.toString())
    } else if (params.get('github') === 'error') {
      setGithubError('GitHub connection failed. Please try again.')
      const url = new URL(window.location.href)
      url.searchParams.delete('github')
      window.history.replaceState({}, '', url.toString())
    }
  }, [projectId]) // eslint-disable-line react-hooks/exhaustive-deps

  // Load saved GitHub mappings from DB on mount
  useEffect(() => {
    apiClient.githubGetMappings(projectId)
      .then(({ status_mappings, user_mappings }) => {
        if (Object.keys(status_mappings).length > 0) setStatusAssignments(status_mappings)
        if (Object.keys(user_mappings).length > 0) setUserAssignments(user_mappings)
      })
      .catch(() => { /* no saved mappings yet */ })
  }, [projectId])

  // Restore saved import filters for this project (filters stay in localStorage — they're UI state)
  useEffect(() => {
    try {
      const saved = localStorage.getItem(`taskai_gh_filter_${projectId}`)
      if (saved) {
        const f = JSON.parse(saved)
        if (f.milestone !== undefined) setFilterMilestone(f.milestone)
        if (f.assignee !== undefined) setFilterAssignee(f.assignee)
        if (f.labels !== undefined) setFilterLabels(f.labels)
        if (f.state !== undefined) setFilterState(f.state)
      }
    } catch { /* ignore */ }
  }, [projectId])

  // Persist import filters whenever they change
  useEffect(() => {
    localStorage.setItem(`taskai_gh_filter_${projectId}`, JSON.stringify({
      milestone: filterMilestone,
      assignee: filterAssignee,
      labels: filterLabels,
      state: filterState,
    }))
  }, [projectId, filterMilestone, filterAssignee, filterLabels, filterState])

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
    setPreviewProgress(null)
    try {
      const preview = await apiClient.githubPreview(projectId, undefined, (evt) => setPreviewProgress(evt))
      setGithubPreview(preview)
      // Initialize user assignments from auto-matched users
      const assignments: Record<string, number> = {}
      for (const u of preview.github_users) {
        assignments[u.login] = u.matched_user_id ?? 0
      }
      setUserAssignments(assignments)
      // Initialize status assignments from auto-matched statuses,
      // then overlay any previously saved DB mappings (DB values take priority)
      const statusInit: Record<string, number> = {}
      for (const s of (preview.statuses ?? [])) {
        statusInit[s.key] = s.matched_lane_id ?? 0
      }
      // Merge saved DB mappings on top
      Object.assign(statusInit, statusAssignments)
      setStatusAssignments(statusInit)
    } catch (error: unknown) {
      setImportError(error instanceof Error ? error.message : 'Failed to fetch GitHub preview')
    } finally {
      setIsPreviewing(false)
      setPreviewProgress(null)
    }
  }

  const handleImportFromGitHub = async () => {
    setImportError('')
    setImportSuccess('')
    setImportProgress(null)
    setIsPulling(true)
    try {
      const filter = (filterMilestone || filterAssignee || filterLabels.length || filterState !== 'all')
        ? {
            milestone_number: filterMilestone,
            assignee: filterAssignee || undefined,
            labels: filterLabels.length ? filterLabels : undefined,
            state: filterState !== 'all' ? filterState : undefined,
          }
        : undefined
      const result = await apiClient.githubPull(
        projectId,
        {
          pull_sprints: pullSprints,
          pull_tags: pullTags,
          pull_tasks: pullTasks,
          pull_comments: pullComments,
          user_assignments: userAssignments,
          status_assignments: statusAssignments,
          filter,
        },
        (evt) => setImportProgress(evt)
      )
      const parts = [`${result.created_sprints} sprints`, `${result.created_tags} tags`, `${result.created_tasks} tasks (${result.skipped_tasks} skipped)`]
      if (result.created_comments > 0) parts.push(`${result.created_comments} comments`)
      setImportSuccess(`Imported: ${parts.join(', ')}`)
      setImportProgress(null)
      loadGitHubSettings()
    } catch (error: unknown) {
      setImportError(error instanceof Error ? error.message : 'Import failed')
      setImportProgress(null)
    } finally {
      setIsPulling(false)
    }
  }

  const handleSaveMappings = async () => {
    setIsSavingMappings(true)
    setMappingsSaved(false)
    try {
      await apiClient.githubSaveMappings(projectId, statusAssignments, userAssignments)
      setMappingsSaved(true)
      setTimeout(() => setMappingsSaved(false), 3000)
    } catch {
      // non-critical
    } finally {
      setIsSavingMappings(false)
    }
  }

  const handleSyncNow = async () => {
    setImportError('')
    setImportSuccess('')
    setImportProgress(null)
    setIsSyncing(true)
    try {
      const result = await apiClient.githubSync(
        projectId,
        statusAssignments,
        userAssignments,
        (evt) => setImportProgress(evt)
      )
      setImportSuccess(`Synced: ${result.created_tasks} new tasks, updated existing`)
      setImportProgress(null)
      loadGitHubSettings()
    } catch (error: unknown) {
      setImportError(error instanceof Error ? error.message : 'Sync failed')
      setImportProgress(null)
    } finally {
      setIsSyncing(false)
    }
  }

  const handlePushAllToGitHub = async () => {
    setImportError('')
    setImportSuccess('')
    setPushAllProgress(null)
    setIsPushingAll(true)
    try {
      const result = await apiClient.githubPushAll(
        projectId,
        (evt) => setPushAllProgress(evt)
      )
      setImportSuccess(`Pushed ${result.created_tasks ?? 0} new tasks to GitHub`)
      setPushAllProgress(null)
    } catch (error: unknown) {
      setImportError(error instanceof Error ? error.message : 'Push failed')
      setPushAllProgress(null)
    } finally {
      setIsPushingAll(false)
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
        github_push_enabled: githubSettings.github_push_enabled,
      })
      setGithubSuccess('GitHub settings saved successfully')
    } catch (error: unknown) {
      setGithubError(error instanceof Error ? error.message : 'Failed to save GitHub settings')
    } finally {
      setIsSavingGitHub(false)
    }
  }

  return (
    <div className={embedded ? 'flex flex-col flex-1 overflow-hidden' : 'h-full flex flex-col bg-dark-bg-base'}>
      {!embedded && (
      <>
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
      </>
      )}

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

                  <div className="flex items-center gap-3 p-4 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                    <input
                      type="checkbox"
                      id="push-enabled"
                      checked={githubSettings.github_push_enabled}
                      onChange={(e) => setGithubSettings({ ...githubSettings, github_push_enabled: e.target.checked })}
                      className="w-4 h-4 text-primary-600 border-dark-border-subtle rounded focus:ring-2 focus:ring-primary-500"
                    />
                    <label htmlFor="push-enabled" className="flex-1">
                      <span className="font-medium text-dark-text-primary">Push changes to GitHub</span>
                      <p className="text-sm text-dark-text-secondary mt-0.5">Send new comments and task status changes back to GitHub</p>
                    </label>
                  </div>

                  <Button type="submit" disabled={isSavingGitHub}>
                    {isSavingGitHub ? 'Saving...' : 'Save Settings'}
                  </Button>
                </form>
              )}

              {/* Import Section — only owners and admins can import/sync */}
              {isOwnerOrAdmin && githubSettings.github_owner && githubSettings.github_repo_name && (
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
                    <div className="space-y-2">
                      <Button onClick={handleFetchPreview} disabled={isPreviewing} variant="secondary">
                        {isPreviewing ? 'Fetching...' : 'Fetch Preview'}
                      </Button>
                      {isPreviewing && (
                        <div className="space-y-1">
                          <div className="text-xs text-dark-text-secondary">
                            {previewProgress?.message ?? 'Connecting...'}
                          </div>
                          <div className="w-full bg-dark-bg-secondary rounded-full h-1 overflow-hidden">
                            <div className="h-1 bg-blue-500 rounded-full animate-pulse w-full" />
                          </div>
                        </div>
                      )}
                    </div>
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
                                  {members.map(m => (
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

                      {/* Unified status → swim lane mapping */}
                      {(githubPreview.statuses ?? []).length > 0 && (
                        <div>
                          <h4 className="text-sm font-medium text-dark-text-primary mb-2">Map GitHub statuses to swim lanes</h4>
                          <div className="space-y-2">
                            {(githubPreview.statuses ?? []).map((st: GitHubStatusMatch) => (
                              <div key={st.key} className="flex items-center gap-3 p-3 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                                <div className={`w-2 h-2 rounded-full flex-shrink-0 ${
                                  st.source === 'project_column' ? 'bg-purple-400' :
                                  st.source === 'label' ? 'bg-yellow-400' :
                                  st.key === 'open' ? 'bg-green-400' : 'bg-gray-400'
                                }`} />
                                <div className="flex-1 min-w-0">
                                  <span className="text-sm font-medium text-dark-text-primary">{st.label}</span>
                                  {st.source === 'project_column' && (
                                    <span className="ml-2 text-xs text-dark-text-tertiary">Projects V2</span>
                                  )}
                                  {st.source === 'label' && (
                                    <span className="ml-2 text-xs text-dark-text-tertiary">label</span>
                                  )}
                                  {st.issue_count > 0 && (
                                    <span className="ml-2 text-xs text-dark-text-tertiary">{st.issue_count} issues</span>
                                  )}
                                </div>
                                <select
                                  value={statusAssignments[st.key] ?? 0}
                                  onChange={(e) => setStatusAssignments(prev => ({ ...prev, [st.key]: Number(e.target.value) }))}
                                  className="text-sm bg-dark-bg-primary border border-dark-border-subtle text-dark-text-primary rounded-md px-2 py-1 focus:outline-none focus:ring-1 focus:ring-primary-500"
                                >
                                  <option value={0}>Default (by category)</option>
                                  {swimLanes.map(l => (
                                    <option key={l.id} value={l.id}>{l.name}</option>
                                  ))}
                                </select>
                              </div>
                            ))}
                          </div>
                        </div>
                      )}

                      {/* Save mappings */}
                      {(githubPreview.github_users.length > 0 || (githubPreview.statuses ?? []).length > 0) && (
                        <div className="flex items-center gap-3">
                          <button
                            onClick={handleSaveMappings}
                            disabled={isSavingMappings}
                            className="px-3 py-1.5 text-xs font-medium bg-primary-600 hover:bg-primary-500 disabled:opacity-50 text-white rounded-lg transition-colors"
                          >
                            {isSavingMappings ? 'Saving...' : 'Save Mappings'}
                          </button>
                          {mappingsSaved && (
                            <span className="text-xs text-success-400">Mappings saved — will be used for future syncs</span>
                          )}
                        </div>
                      )}

                      {/* Filters */}
                      <GitHubFilterBar
                        milestones={githubPreview.milestones ?? []}
                        assignees={githubPreview.github_users ?? []}
                        labels={githubPreview.labels ?? []}
                        filterMilestone={filterMilestone}
                        filterAssignee={filterAssignee}
                        filterLabels={filterLabels}
                        filterState={filterState}
                        onChange={patch => {
                          if ('milestone' in patch) setFilterMilestone(patch.milestone)
                          if ('assignee' in patch) setFilterAssignee(patch.assignee ?? '')
                          if ('labels' in patch) setFilterLabels(patch.labels ?? [])
                          if ('state' in patch) setFilterState(patch.state ?? 'all')
                        }}
                      />

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
                        <label className="flex items-center gap-2 text-sm text-dark-text-primary cursor-pointer">
                          <input type="checkbox" checked={pullComments} onChange={e => setPullComments(e.target.checked)}
                            className="w-4 h-4 text-primary-600 border-dark-border-subtle rounded focus:ring-primary-500" />
                          Import Comments
                        </label>
                      </div>

                      {/* Progress indicator */}
                      {importProgress && (
                        <div className="p-3 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                          <div className="flex items-center justify-between text-sm mb-1.5">
                            <span className="text-dark-text-primary font-medium">{importProgress.message}</span>
                            {importProgress.total > 0 && (
                              <span className="text-dark-text-tertiary text-xs">{importProgress.current}/{importProgress.total}</span>
                            )}
                          </div>
                          {importProgress.total > 0 && (
                            <div className="w-full bg-dark-bg-primary rounded-full h-1.5">
                              <div
                                className="bg-primary-500 h-1.5 rounded-full transition-all duration-300"
                                style={{ width: `${Math.round((importProgress.current / importProgress.total) * 100)}%` }}
                              />
                            </div>
                          )}
                        </div>
                      )}

                      <div className="flex items-center gap-3">
                        <Button onClick={handleImportFromGitHub} disabled={isPulling || (!pullSprints && !pullTags && !pullTasks)}>
                          {isPulling ? 'Importing...' : 'Import from GitHub'}
                        </Button>
                      </div>
                    </div>
                  )}

                  {/* Persistent Sync Mappings — visible even without Preview */}
                  {!githubPreview && (Object.keys(statusAssignments).length > 0 || Object.keys(userAssignments).length > 0) && (
                    <div className="mt-4 pt-4 border-t border-dark-border-subtle space-y-4">
                      <div className="flex items-center justify-between">
                        <div>
                          <h4 className="text-sm font-medium text-dark-text-primary">Sync Mappings</h4>
                          <p className="text-xs text-dark-text-tertiary mt-0.5">Saved mappings used when syncing from GitHub</p>
                        </div>
                        <div className="flex items-center gap-3">
                          <button
                            onClick={handleSaveMappings}
                            disabled={isSavingMappings}
                            className="px-3 py-1.5 text-xs font-medium bg-primary-600 hover:bg-primary-500 disabled:opacity-50 text-white rounded-lg transition-colors"
                          >
                            {isSavingMappings ? 'Saving...' : 'Save Mappings'}
                          </button>
                          {mappingsSaved && (
                            <span className="text-xs text-success-400">Saved!</span>
                          )}
                        </div>
                      </div>
                      {Object.keys(statusAssignments).length > 0 && (
                        <div>
                          <h5 className="text-xs font-medium text-dark-text-secondary uppercase tracking-wide mb-2">GitHub Status → Swim Lane</h5>
                          <div className="space-y-2">
                            {Object.entries(statusAssignments).map(([key, laneId]) => (
                              <div key={key} className="flex items-center gap-3 p-2 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                                <span className="flex-1 text-sm text-dark-text-primary font-mono">{key}</span>
                                <select
                                  value={laneId ?? 0}
                                  onChange={(e) => setStatusAssignments(prev => ({ ...prev, [key]: Number(e.target.value) }))}
                                  className="text-sm bg-dark-bg-primary border border-dark-border-subtle text-dark-text-primary rounded-md px-2 py-1 focus:outline-none focus:ring-1 focus:ring-primary-500"
                                >
                                  <option value={0}>Default (by category)</option>
                                  {swimLanes.map(l => (
                                    <option key={l.id} value={l.id}>{l.name}</option>
                                  ))}
                                </select>
                              </div>
                            ))}
                          </div>
                        </div>
                      )}
                      {Object.keys(userAssignments).length > 0 && (
                        <div>
                          <h5 className="text-xs font-medium text-dark-text-secondary uppercase tracking-wide mb-2">GitHub User → TaskAI User</h5>
                          <div className="space-y-2">
                            {Object.entries(userAssignments).map(([login, userId]) => (
                              <div key={login} className="flex items-center gap-3 p-2 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                                <span className="flex-1 text-sm text-dark-text-primary font-mono">@{login}</span>
                                <select
                                  value={userId ?? 0}
                                  onChange={(e) => setUserAssignments(prev => ({ ...prev, [login]: Number(e.target.value) }))}
                                  className="text-sm bg-dark-bg-primary border border-dark-border-subtle text-dark-text-primary rounded-md px-2 py-1 focus:outline-none focus:ring-1 focus:ring-primary-500"
                                >
                                  <option value={0}>Unassigned</option>
                                  {members.map(u => (
                                    <option key={u.user_id} value={u.user_id}>{u.name || u.email}</option>
                                  ))}
                                </select>
                              </div>
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
                  )}

                  {/* Sync Now (shown after first sync, only for owners/admins) */}
                  {githubSettings.github_last_sync && (
                    <div className="mt-4 pt-4 border-t border-dark-border-subtle space-y-3">
                      <div className="flex items-center gap-4">
                        <span className="text-sm text-dark-text-secondary">
                          Last synced: {new Date(githubSettings.github_last_sync).toLocaleString()}
                        </span>
                        <Button onClick={handleSyncNow} disabled={isSyncing} variant="secondary" size="sm">
                          {isSyncing ? 'Syncing...' : 'Sync Now'}
                        </Button>
                      </div>
                      {isSyncing && importProgress && (
                        <div className="p-3 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                          <div className="flex items-center justify-between text-sm mb-1.5">
                            <span className="text-dark-text-primary font-medium">{importProgress.message}</span>
                            {importProgress.total > 0 && (
                              <span className="text-dark-text-tertiary text-xs">{importProgress.current}/{importProgress.total}</span>
                            )}
                          </div>
                          {importProgress.total > 0 && (
                            <div className="w-full bg-dark-bg-primary rounded-full h-1.5">
                              <div
                                className="bg-primary-500 h-1.5 rounded-full transition-all duration-300"
                                style={{ width: `${Math.round((importProgress.current / importProgress.total) * 100)}%` }}
                              />
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  )}

                  {/* Push All New Issues to GitHub */}
                  {githubSettings.github_token_set && (isOwnerOrAdmin) && (
                    <div className="mt-4 pt-4 border-t border-dark-border-subtle space-y-3">
                      <div>
                        <h4 className="text-sm font-medium text-dark-text-primary mb-1">Push New Issues to GitHub</h4>
                        <p className="text-xs text-dark-text-tertiary mb-3">
                          Create GitHub issues for all TaskAI tasks that haven't been linked to GitHub yet.
                        </p>
                        <Button onClick={handlePushAllToGitHub} disabled={isPushingAll} variant="secondary" size="sm">
                          {isPushingAll ? 'Pushing...' : 'Push All New Issues to GitHub'}
                        </Button>
                      </div>
                      {isPushingAll && pushAllProgress && (
                        <div className="p-3 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                          <div className="flex items-center justify-between text-sm mb-1.5">
                            <span className="text-dark-text-primary font-medium">{pushAllProgress.message}</span>
                            {pushAllProgress.total > 0 && (
                              <span className="text-dark-text-tertiary text-xs">{pushAllProgress.current}/{pushAllProgress.total}</span>
                            )}
                          </div>
                          {pushAllProgress.total > 0 && (
                            <div className="w-full bg-dark-bg-primary rounded-full h-1.5">
                              <div
                                className="bg-primary-500 h-1.5 rounded-full transition-all duration-300"
                                style={{ width: `${Math.round((pushAllProgress.current / pushAllProgress.total) * 100)}%` }}
                              />
                            </div>
                          )}
                        </div>
                      )}
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
