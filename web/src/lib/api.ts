// Import generated types from OpenAPI spec
import type { components, operations } from './api.types'

// Re-export types for convenience
export type User = components['schemas']['User']
export type AuthResponse = components['schemas']['AuthResponse']
export type SignupRequest = components['schemas']['SignupRequest']
export type LoginRequest = components['schemas']['LoginRequest']
export type Project = components['schemas']['Project']
export type Task = components['schemas']['Task'] & { task_number?: number; github_issue_number?: number | null; start_date?: string | null; github_reactions?: GitHubReaction[] }
export type ApiError = components['schemas']['Error']

export interface GitHubReaction {
  reaction: string
  count: number
  user_reacted?: boolean
}

// Types with required fields for commonly used API responses
export interface TaskComment {
  id: number
  task_id: number
  user_id: number
  user_name?: string | null
  comment: string
  created_at: string
  updated_at: string
  github_reactions?: GitHubReaction[]
}

export interface Sprint {
  id: number
  user_id: number
  name: string
  start_date?: string
  end_date?: string
  goal: string
  status: string
  is_shared?: boolean
  created_at: string
  updated_at?: string
}

export interface Tag {
  id: number
  user_id: number
  name: string
  color: string
  is_shared?: boolean
  created_at: string
}

export interface Attachment {
  id: number
  task_id: number
  filename: string
  alt_name?: string
  file_type: string
  content_type: string
  file_size: number
  cloudinary_url: string
  cloudinary_public_id: string
  created_at: string
}

export interface WikiPageAttachment {
  id: number
  wiki_page_id: number
  project_id: number
  user_id: number
  filename: string
  alt_name?: string
  file_type: string
  content_type: string
  file_size: number
  cloudinary_url: string
  cloudinary_public_id: string
  created_at: string
  user_name?: string | null
}

export interface Invite {
  id: number
  code: string
  inviter_id: number
  invitee_id?: number
  invitee_name?: string
  used_at?: string
  expires_at?: string
  created_at: string
}

export interface ProjectMember {
  id: number
  user_id: number
  email: string
  name?: string
  user_name?: string
  role: string
  granted_by: number
  granted_at: string
}

export interface ProjectInvitation {
  id: number
  project_id: number
  project_name?: string
  inviter_id: number
  inviter_name?: string
  invitee_user_id: number
  invitee_name?: string
  invitee_email?: string
  role: string
  status: string
  invited_at: string
  responded_at?: string
  last_sent_at: string
  can_resend: boolean
}

export interface ProjectDrawing {
  id: number
  project_id: number
  draw_id: string
  created_by: number
  created_at: string
}

export interface WikiPage {
  id: number
  project_id: number
  title: string
  slug: string
  created_by: number
  creator_name?: string
  updated_by?: number
  updater_name?: string
  created_at: string
  updated_at: string
}

export interface AppNotification {
  id: number
  sender_id?: number | null
  sender_name?: string | null
  type: string
  entity_type: string
  entity_id: number
  project_id: number
  project_name?: string | null
  message: string
  link: string
  read_at?: string | null
  created_at: string
}

export interface UserProfileActivity {
  type: string
  entity_id: number
  entity_title: string
  project_id: number
  project_name: string
  link: string
  created_at: string
}

export interface UserProfile {
  user: {
    id: number
    name?: string | null
    first_name?: string | null
    last_name?: string | null
    email: string
    joined_at?: string | null
  }
  recent_activity: UserProfileActivity[]
  has_more: boolean
}

export interface UserActivityPage {
  items: UserProfileActivity[]
  has_more: boolean
}

export type AnnotationColor = 'yellow' | 'blue' | 'green' | 'red'

export interface AnnotationComment {
  id: number
  annotation_id: number
  author_id: number
  author_name?: string | null
  parent_comment_id?: number | null
  content: string
  resolved: boolean
  created_at: string
  updated_at: string
}

export interface WikiAnnotation {
  id: number
  wiki_page_id: number
  author_id: number
  author_name?: string | null
  start_offset: number
  end_offset: number
  selected_text: string
  color: AnnotationColor
  resolved: boolean
  created_at: string
  comments: AnnotationComment[]
}

export interface WikiPageVersion {
  id: number
  wiki_page_id: number
  version_number: number
  content_hash: string
  created_by: number
  creator_name?: string
  created_at: string
}

export interface WikiPageVersionWithContent extends WikiPageVersion {
  content: string
}

export interface WikiSearchResult {
  page_id: number
  page_title: string
  page_slug: string
  block_id: number
  block_type: string
  headings_path: string
  snippet: string
  rank?: number
}

export interface SearchTaskResult {
  id: number
  project_id: number
  project_name: string
  task_number: number
  title: string
  snippet: string
  status: string
  priority: string
}

export interface GlobalSearchWikiResult {
  page_id: number
  page_title: string
  page_slug: string
  project_id: number
  project_name: string
  snippet: string
  headings_path?: string
}

export interface GlobalSearchResponse {
  tasks: SearchTaskResult[]
  wiki: GlobalSearchWikiResult[]
}

export interface MessageResponse {
  message: string
}

export interface ProjectGitHubSettings {
  github_repo_url: string
  github_owner: string
  github_repo_name: string
  github_branch: string
  github_sync_enabled: boolean
  github_push_enabled: boolean
  github_last_sync: string | null
  github_token_set: boolean
  github_login: string | null
  github_project_url: string   // optional explicit GitHub Projects V2 URL
  github_sync_interval: string // 'daily','weekly','monthly', '' = disabled
  github_sync_hour: number     // 0-23
  github_sync_day: number      // 0-6 for weekly (0=Sun), 1-28 for monthly
}

export interface GitHubRepo {
  id: number
  full_name: string
  name: string
  owner: string
  default_branch: string
  private: boolean
  html_url: string
}

export interface GitHubUserMatch {
  login: string
  name: string
  matched_user_id: number | null
  matched_name: string
}

export interface GitHubStatusMatch {
  key: string
  label: string
  source: 'issue_state' | 'project_column' | 'label'
  issue_count: number
  matched_lane_id: number | null
  matched_name: string
}

export interface GitHubMilestone {
  number: number
  title: string
  state: string
  due_on: string
}

export interface GitHubLabel {
  name: string
  color: string
}

export interface GitHubPreviewResponse {
  milestone_count: number
  label_count: number
  issue_count: number
  github_users: GitHubUserMatch[]
  statuses: GitHubStatusMatch[]
  milestones: GitHubMilestone[]
  labels: GitHubLabel[]
}

export interface GitHubImportFilter {
  milestone_number?: number
  assignee?: string
  labels?: string[]
  state?: 'all' | 'open' | 'closed'
}

export interface GitHubPullRequest {
  token?: string
  pull_sprints: boolean
  pull_tags: boolean
  pull_tasks: boolean
  pull_comments: boolean
  user_assignments: Record<string, number>
  status_assignments: Record<string, number>
  filter?: GitHubImportFilter
  force_full_sync?: boolean
}

export interface GitHubPullResponse {
  created_sprints: number
  created_tags: number
  created_tasks: number
  updated_tasks: number
  skipped_tasks: number
  created_comments: number
}

export interface GitHubSyncLog {
  id: number
  project_id: number
  started_at: string
  completed_at?: string
  status: 'running' | 'success' | 'failed'
  triggered_by: 'manual' | 'auto'
  created_tasks: number
  updated_tasks: number
  created_comments: number
  skipped_tasks: number
  error_message?: string
}

export interface GitHubPushTaskResponse {
  issue_number: number
  html_url: string
}

export interface GitHubProgressEvent {
  type: 'progress'
  stage: string
  message: string
  current: number
  total: number
}

export interface UserWithStats {
  id: number
  email: string
  name?: string
  first_name?: string
  last_name?: string
  is_admin: boolean
  created_at: string
  login_count: number
  last_login_at?: string | null
  last_login_ip?: string | null
  failed_attempts: number
  invite_count: number
}

export interface UserActivity {
  id: number
  user_id: number
  activity_type: string
  ip_address?: string | null
  user_agent?: string | null
  created_at: string
}

export interface AdminInvitation {
  id: number
  type: 'team' | 'project'
  status: string
  inviter_name: string
  inviter_email: string
  invitee_name: string
  invitee_email: string
  invitee_id: number | null
  context: string
  role?: string
  created_at: string
}

export interface APIKey {
  id: number
  name: string
  key_prefix: string
  created_at: string
  last_used_at?: string | null
  expires_at?: string | null
}

export interface CreateAPIKeyResponse {
  key: string
  name: string
}

export interface Team {
  id: number
  name: string
}

export interface TeamMember {
  id: number
  user_id: number
  user_name?: string
  email: string
  role: string
}

export interface TeamInvitation {
  id: number
  team_name: string
  inviter_name?: string
}

export interface SentInvitation {
  id: number
  invitee_email: string
  status: string
  created_at: string
}

export interface UserSearchResult {
  id: number
  email: string
  name?: string
}

export interface TeamMembership {
  team_id: number
  team_name: string
  owner_id: number
  role: string
  joined_at: string
}

export interface TokenInvitationInfo {
  invitation_id: number
  team_name: string
  inviter_name: string
  invitee_email: string
  status: string
  requires_signup: boolean
  invite_code?: string
}

export interface StorageUsageItem {
  user_id: number
  user_name: string
  file_count: number
  total_size: number
}

// Request types (not in OpenAPI spec yet, so define them)
export interface CreateProjectRequest {
  name: string
  description?: string
}

export interface UpdateProjectRequest {
  name?: string
  description?: string
}

export interface CreateTaskRequest {
  title: string
  description?: string
  status?: 'todo' | 'in_progress' | 'done'
  swim_lane_id?: number
  due_date?: string
  sprint_id?: number
  priority?: 'low' | 'medium' | 'high' | 'urgent'
  assignee_id?: number
  estimated_hours?: number
  actual_hours?: number
  tag_ids?: number[]
}

export interface UpdateTaskRequest {
  title?: string
  description?: string
  status?: 'todo' | 'in_progress' | 'done'
  swim_lane_id?: number | null
  start_date?: string | null
  due_date?: string | null
  sprint_id?: number | null
  priority?: 'low' | 'medium' | 'high' | 'urgent'
  assignee_id?: number | null
  assignee_ids?: number[]
  estimated_hours?: number | null
  actual_hours?: number | null
  tag_ids?: number[]
}

export interface SwimLane {
  id: number
  project_id: number
  name: string
  color: string
  position: number
  status_category: 'todo' | 'in_progress' | 'done'
  created_at: string
  updated_at: string
}

export interface CreateSwimLaneRequest {
  name: string
  color: string
  position: number
  status_category: 'todo' | 'in_progress' | 'done'
}

export interface UpdateSwimLaneRequest {
  name?: string
  color?: string
  position?: number
  status_category?: 'todo' | 'in_progress' | 'done'
}

// Helper types for API responses (using available operations)
type SignupResponse = operations['signup']['responses']['201']['content']['application/json']
type LoginResponse = operations['login']['responses']['200']['content']['application/json']
type GetCurrentUserResponse = operations['getCurrentUser']['responses']['200']['content']['application/json']

export interface EmailProviderResponse {
  id: number
  provider: string
  api_key: string
  sender_email: string
  sender_name: string
  status: 'unknown' | 'connected' | 'error' | 'suspended'
  last_checked_at: string | null
  last_error: string
  consecutive_failures: number
  created_at: string
  updated_at: string
}

export interface CloudinaryCredentialResponse {
  id: number
  user_id: number
  cloud_name: string
  api_key: string
  max_file_size_mb: number
  status: 'unknown' | 'connected' | 'error' | 'suspended'
  last_checked_at: string | null
  last_error: string
  consecutive_failures: number
  created_at: string
  updated_at: string
}

export interface FigmaCredentialsStatus {
  configured: boolean
}

export interface FigmaEmbedInfo {
  embed_url: string
  name?: string
  thumbnail_url?: string
  configured: boolean
}

export interface BackupStatus {
  running: boolean
  enabled: boolean
  provider_connected: boolean
  provider_name?: string
  connected_email?: string
  next_run?: string
  last_backup?: {
    id: string
    status: string
    filename: string
    size_bytes: number
    started_at: string
    finished_at?: string
  }
}

export interface BackupSettings {
  enabled: boolean
  cron_expression: string
  folder_id: string
  provider_name: string
  retention: {
    full_days: number
    alternate_days: number
    weekly_days: number
  }
  updated_at: string
}

export interface BackupRecord {
  id: string
  status: 'running' | 'success' | 'failed'
  triggered_by: string
  filename: string
  size_bytes: number
  provider_name: string
  file_id: string
  file_url: string
  error_message: string
  started_at: string
  finished_at?: string
}

export interface Asset {
  id: number
  task_id: number
  project_id: number
  user_id: number
  filename: string
  alt_name: string
  file_type: string
  content_type: string
  file_size: number
  cloudinary_url: string
  cloudinary_public_id: string
  created_at: string
  user_name?: string
  is_owner: boolean
  is_shared?: boolean
}

export interface GraphNode {
  id: number
  project_id: number
  entity_type: 'wiki' | 'task'
  entity_id: number
  entity_number?: number | null
  title: string
  created_at: string
  updated_at: string
}

export interface GraphEdge {
  id: number
  source_node_id: number
  target_node_id: number
  relation_type: string
  created_at: string
}

export interface GraphData {
  nodes: GraphNode[]
  edges: GraphEdge[]
}

// API Client Configuration
// Use relative URL in production (served behind nginx proxy)
// or VITE_API_URL for development override
const API_BASE_URL = import.meta.env.VITE_API_URL || (
  import.meta.env.PROD ? '' : 'http://localhost:8080'
)

class ApiClient {
  private baseURL: string
  private token: string | null = null

  constructor(baseURL: string) {
    this.baseURL = baseURL
    // Load token from localStorage on initialization
    this.token = localStorage.getItem('auth_token')
  }

  setToken(token: string | null) {
    this.token = token
    if (token) {
      localStorage.setItem('auth_token', token)
    } else {
      localStorage.removeItem('auth_token')
    }
  }

  getToken(): string | null {
    return this.token
  }

  private async request<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const url = `${this.baseURL}${endpoint}`

    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    }

    // Add existing headers
    if (options.headers) {
      Object.assign(headers, options.headers)
    }

    // Add authorization header if token exists
    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`
    }

    const config: RequestInit = {
      ...options,
      headers,
    }

    try {
      const response = await fetch(url, config)

      // Handle non-JSON responses (like 204 No Content)
      if (response.status === 204) {
        return {} as T
      }

      // Check if response is JSON before attempting to parse
      const contentType = response.headers.get('content-type')
      const isJson = contentType?.includes('application/json')

      if (!isJson) {
        // Server returned non-JSON (likely HTML error page) — consume body
        await response.text()
        throw new Error(
          response.ok
            ? 'Server returned unexpected response format'
            : `Server error (${response.status}): ${response.statusText}`
        )
      }

      const data = await response.json()

      if (!response.ok) {
        // Handle API errors
        const error = data as ApiError
        throw new Error(error.error || `HTTP ${response.status}`)
      }

      return data as T
    } catch (error) {
      if (error instanceof Error) {
        throw error
      }
      throw new Error('An unexpected error occurred')
    }
  }

  // Auth endpoints
  async signup(data: SignupRequest & { invite_code?: string }): Promise<SignupResponse> {
    const response = await this.request<SignupResponse>('/api/auth/signup', {
      method: 'POST',
      body: JSON.stringify(data),
    })
    if (response.token) {
      this.setToken(response.token)
    }
    return response
  }

  async login(data: LoginRequest): Promise<LoginResponse> {
    const response = await this.request<LoginResponse>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify(data),
    })
    if (response.token) {
      this.setToken(response.token)
    }
    return response
  }

  logout(): void {
    this.setToken(null)
  }

  async getCurrentUser(): Promise<GetCurrentUserResponse> {
    return this.request<GetCurrentUserResponse>('/api/me')
  }

  async updateProfile(data: { first_name: string; last_name: string }): Promise<GetCurrentUserResponse> {
    return this.request<GetCurrentUserResponse>('/api/me', {
      method: 'PATCH',
      body: JSON.stringify(data),
    })
  }

  // Health check
  async healthCheck(): Promise<{ status: string; database?: string }> {
    return this.request('/healthz')
  }

  // Project endpoints
  async getProjects(): Promise<Project[]> {
    return this.request<Project[]>('/api/projects')
  }

  async getProject(id: number): Promise<Project> {
    return this.request<Project>('/api/projects/' + id)
  }

  async createProject(data: CreateProjectRequest): Promise<Project> {
    return this.request<Project>('/api/projects', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async updateProject(id: number, data: UpdateProjectRequest): Promise<Project> {
    return this.request<Project>('/api/projects/' + id, {
      method: 'PATCH',
      body: JSON.stringify(data),
    })
  }

  async deleteProject(id: number): Promise<void> {
    return this.request<void>('/api/projects/' + id, {
      method: 'DELETE',
    })
  }

  // Task endpoints
  async getTasks(projectId: number): Promise<Task[]> {
    return this.request<Task[]>('/api/projects/' + projectId + '/tasks')
  }

  async createTask(projectId: number, data: CreateTaskRequest): Promise<Task> {
    return this.request<Task>('/api/projects/' + projectId + '/tasks', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async updateTask(id: number, data: UpdateTaskRequest): Promise<Task> {
    return this.request<Task>('/api/tasks/' + id, {
      method: 'PATCH',
      body: JSON.stringify(data),
    })
  }

  async getTaskByNumber(projectId: number, taskNumber: number): Promise<Task> {
    return this.request<Task>(`/api/projects/${projectId}/tasks/${taskNumber}`)
  }

  async deleteTask(id: number): Promise<void> {
    return this.request<void>('/api/tasks/' + id, {
      method: 'DELETE',
    })
  }

  // Task comments endpoints
  async getTaskComments(taskId: number): Promise<TaskComment[]> {
    return this.request<TaskComment[]>(`/api/tasks/${taskId}/comments`)
  }

  async createTaskComment(taskId: number, comment: string): Promise<TaskComment> {
    return this.request<TaskComment>(`/api/tasks/${taskId}/comments`, {
      method: 'POST',
      body: JSON.stringify({ comment }),
    })
  }

  async toggleReaction(taskId: number, reaction: string, commentId?: number): Promise<GitHubReaction> {
    return this.request<GitHubReaction>(`/api/tasks/${taskId}/reactions`, {
      method: 'POST',
      body: JSON.stringify({ reaction, comment_id: commentId ?? 0 }),
    })
  }

  // Project settings - Members
  async getProjectMembers(projectId: number): Promise<ProjectMember[]> {
    return this.request<ProjectMember[]>(`/api/projects/${projectId}/members`)
  }

  async addProjectMember(projectId: number, data: { email: string; role: string }): Promise<MessageResponse> {
    return this.request<MessageResponse>(`/api/projects/${projectId}/members`, {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async updateProjectMember(projectId: number, memberId: number, data: { role: string }): Promise<MessageResponse> {
    return this.request<MessageResponse>(`/api/projects/${projectId}/members/${memberId}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    })
  }

  async removeProjectMember(projectId: number, memberId: number): Promise<void> {
    return this.request<void>(`/api/projects/${projectId}/members/${memberId}`, {
      method: 'DELETE',
    })
  }

  // Project invitations
  async inviteProjectMember(projectId: number, data: { user_id: number; role: string }): Promise<{ message: string; invitation_id: number }> {
    return this.request(`/api/projects/${projectId}/invitations`, {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async getProjectInvitations(projectId: number): Promise<ProjectInvitation[]> {
    return this.request<ProjectInvitation[]>(`/api/projects/${projectId}/invitations`)
  }

  async acceptProjectInvitation(invitationId: number): Promise<void> {
    return this.request<void>(`/api/project-invitations/${invitationId}/accept`, { method: 'POST' })
  }

  async rejectProjectInvitation(invitationId: number): Promise<void> {
    return this.request<void>(`/api/project-invitations/${invitationId}/reject`, { method: 'POST' })
  }

  async withdrawProjectInvitation(invitationId: number): Promise<void> {
    return this.request<void>(`/api/project-invitations/${invitationId}`, { method: 'DELETE' })
  }

  async resendProjectInvitation(invitationId: number): Promise<void> {
    return this.request<void>(`/api/project-invitations/${invitationId}/resend`, { method: 'POST' })
  }

  async getMyProjectInvitations(): Promise<ProjectInvitation[]> {
    return this.request<ProjectInvitation[]>('/api/my/project-invitations')
  }

  async getMyProjectInvitationCount(): Promise<{ count: number }> {
    return this.request<{ count: number }>('/api/my/project-invitations/count')
  }

  // Project settings - GitHub
  async getProjectGitHub(projectId: number): Promise<ProjectGitHubSettings> {
    return this.request<ProjectGitHubSettings>(`/api/projects/${projectId}/github`)
  }

  async updateProjectGitHub(projectId: number, data: Partial<ProjectGitHubSettings> & { github_token?: string }): Promise<MessageResponse> {
    return this.request<MessageResponse>(`/api/projects/${projectId}/github`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    })
  }

  async githubPreview(
    projectId: number,
    token?: string,
    onProgress?: (event: GitHubProgressEvent) => void
  ): Promise<GitHubPreviewResponse> {
    return this.streamGitHub<GitHubPreviewResponse>(
      `/api/projects/${projectId}/github/preview`,
      { token: token || '' },
      onProgress ?? (() => {})
    )
  }

  // Stream a GitHub SSE endpoint and report progress events.
  private async streamGitHub<T = GitHubPullResponse>(
    endpoint: string,
    data: object,
    onProgress: (event: GitHubProgressEvent) => void
  ): Promise<T> {
    const url = `${this.baseURL}${endpoint}`
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (this.token) headers['Authorization'] = `Bearer ${this.token}`

    const response = await fetch(url, { method: 'POST', headers, body: JSON.stringify(data) })

    if (!response.ok || !response.body) {
      const text = await response.text()
      throw new Error(`HTTP ${response.status}: ${text}`)
    }

    const reader = response.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''

    // Accumulate chunked data sent as dedicated events before `done`
    let collectedMilestones: unknown[] | null = null
    let collectedLabels: unknown[] | null = null

    // eslint-disable-next-line no-constant-condition
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() ?? ''
      for (const line of lines) {
        if (!line.startsWith('data: ')) continue
        const json = line.slice(6).trim()
        if (!json) continue
        let event: Record<string, unknown>
        try { event = JSON.parse(json) } catch { continue }
        if (event.type === 'done') {
          const result = event.result as Record<string, unknown>
          // Merge milestones/labels streamed as dedicated events into the result
          if (collectedMilestones !== null) result.milestones = collectedMilestones
          if (collectedLabels !== null) result.labels = collectedLabels
          return result as T
        }
        if (event.type === 'milestones') { collectedMilestones = event.items as unknown[]; continue }
        if (event.type === 'labels') { collectedLabels = event.items as unknown[]; continue }
        if (event.type === 'error') throw new Error(event.message as string)
        if (event.type === 'progress') onProgress(event as unknown as GitHubProgressEvent)
      }
    }
    throw new Error('Stream ended without result')
  }

  async githubPull(
    projectId: number,
    data: GitHubPullRequest,
    onProgress?: (event: GitHubProgressEvent) => void
  ): Promise<GitHubPullResponse> {
    return this.streamGitHub(
      `/api/projects/${projectId}/github/pull`,
      data,
      onProgress ?? (() => {})
    )
  }

  async githubSync(
    projectId: number,
    statusAssignments?: Record<string, number>,
    userAssignments?: Record<string, number>,
    stateFilter?: 'open' | 'closed' | 'all',
    onProgress?: (event: GitHubProgressEvent) => void,
    forceFullSync?: boolean
  ): Promise<GitHubPullResponse> {
    const filter = stateFilter && stateFilter !== 'all' ? { state: stateFilter } : undefined
    return this.streamGitHub(
      `/api/projects/${projectId}/github/sync`,
      {
        pull_sprints: true,
        pull_tags: true,
        pull_tasks: true,
        pull_comments: true,
        user_assignments: userAssignments ?? {},
        status_assignments: statusAssignments ?? {},
        filter,
        force_full_sync: forceFullSync,
      },
      onProgress ?? (() => {})
    )
  }

  async githubOAuthInit(projectId: number): Promise<{ auth_url: string }> {
    return this.request<{ auth_url: string }>(`/api/projects/${projectId}/github/oauth-init`, {
      method: 'POST',
    })
  }

  async githubListRepos(projectId: number): Promise<GitHubRepo[]> {
    return this.request<GitHubRepo[]>(`/api/projects/${projectId}/github/repos`)
  }

  async githubDisconnect(projectId: number): Promise<void> {
    await this.request<void>(`/api/projects/${projectId}/github/token`, {
      method: 'DELETE',
    })
  }

  async githubGetMappings(projectId: number): Promise<{ status_mappings: Record<string, number>; user_mappings: Record<string, number> }> {
    return this.request(`/api/projects/${projectId}/github/mappings`)
  }

  async githubSaveMappings(projectId: number, statusMappings: Record<string, number>, userMappings: Record<string, number>): Promise<void> {
    await this.request(`/api/projects/${projectId}/github/mappings`, {
      method: 'PUT',
      body: JSON.stringify({ status_mappings: statusMappings, user_mappings: userMappings }),
    })
  }

  async githubGetSyncLogs(projectId: number): Promise<GitHubSyncLog[]> {
    return this.request<GitHubSyncLog[]>(`/api/projects/${projectId}/github/sync-logs`)
  }

  async githubPushTask(taskId: number): Promise<GitHubPushTaskResponse> {
    return this.request<GitHubPushTaskResponse>(`/api/tasks/${taskId}/github/push`, {
      method: 'POST',
    })
  }

  async githubPushAll(
    projectId: number,
    onProgress?: (event: GitHubProgressEvent) => void
  ): Promise<GitHubPullResponse> {
    return this.streamGitHub(
      `/api/projects/${projectId}/github/push-all`,
      {},
      onProgress ?? (() => {})
    )
  }

  // Admin endpoints
  async getUsers(): Promise<UserWithStats[]> {
    return this.request<UserWithStats[]>('/api/admin/users')
  }

  async getUserActivity(userId: number): Promise<UserActivity[]> {
    return this.request<UserActivity[]>(`/api/admin/users/${userId}/activity`)
  }

  async updateUserAdmin(userId: number, isAdmin: boolean): Promise<MessageResponse> {
    return this.request<MessageResponse>(`/api/admin/users/${userId}/admin`, {
      method: 'PATCH',
      body: JSON.stringify({ is_admin: isAdmin }),
    })
  }

  async deleteUser(userId: number): Promise<{ id: number; deleted: boolean }> {
    return this.request<{ id: number; deleted: boolean }>(`/api/admin/users/${userId}`, {
      method: 'DELETE',
    })
  }

  async adminUpdateUserProfile(userId: number, data: { first_name: string; last_name: string }): Promise<{ id: number; first_name: string; last_name: string; name: string }> {
    return this.request(`/api/admin/users/${userId}/profile`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    })
  }

  async adminResetPassword(userId: number, data: { send_email: boolean; password?: string }): Promise<{ message: string }> {
    return this.request<{ message: string }>(`/api/admin/users/${userId}/reset-password`, {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async adminGetInvitations(params?: { status?: string; type?: string }): Promise<AdminInvitation[]> {
    const q = new URLSearchParams()
    if (params?.status) q.set('status', params.status)
    if (params?.type) q.set('type', params.type)
    const qs = q.toString()
    return this.request<AdminInvitation[]>(`/api/admin/invitations${qs ? `?${qs}` : ''}`)
  }

  async adminResolveTeamInvitation(id: number, action: 'accept' | 'reject'): Promise<{ message: string }> {
    return this.request<{ message: string }>(`/api/admin/team-invitations/${id}/resolve`, {
      method: 'POST',
      body: JSON.stringify({ action }),
    })
  }

  async adminResolveProjectInvitation(id: number, action: 'accept' | 'reject'): Promise<{ message: string }> {
    return this.request<{ message: string }>(`/api/admin/project-invitations/${id}/resolve`, {
      method: 'POST',
      body: JSON.stringify({ action }),
    })
  }

  async forgotPassword(email: string): Promise<{ message: string }> {
    return this.request<{ message: string }>('/api/auth/forgot-password', {
      method: 'POST',
      body: JSON.stringify({ email }),
    })
  }

  async resetPassword(token: string, password: string): Promise<{ message: string }> {
    return this.request<{ message: string }>('/api/auth/reset-password', {
      method: 'POST',
      body: JSON.stringify({ token, password }),
    })
  }

  // Security/Settings endpoints
  async changePassword(data: { current_password: string; new_password: string }): Promise<{ message: string }> {
    return this.request<{ message: string }>('/api/settings/password', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async get2FAStatus(): Promise<{ enabled: boolean }> {
    return this.request<{ enabled: boolean }>('/api/settings/2fa/status')
  }

  async setup2FA(): Promise<{ secret: string; qr_code_url: string; qr_code_svg: string }> {
    return this.request<{ secret: string; qr_code_url: string; qr_code_svg: string }>('/api/settings/2fa/setup', {
      method: 'POST',
    })
  }

  async enable2FA(data: { code: string }): Promise<{ backup_codes: string[] }> {
    return this.request<{ backup_codes: string[] }>('/api/settings/2fa/enable', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async disable2FA(data: { password: string }): Promise<{ message: string }> {
    return this.request<{ message: string }>('/api/settings/2fa/disable', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  // Sprint endpoints (project-scoped)
  async getSprints(projectId: number): Promise<Sprint[]> {
    return this.request<Sprint[]>(`/api/projects/${projectId}/sprints`)
  }

  async createSprint(projectId: number, data: Partial<Sprint>): Promise<Sprint> {
    return this.request<Sprint>(`/api/projects/${projectId}/sprints`, {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async updateSprint(id: number, data: Partial<Sprint>): Promise<Sprint> {
    return this.request<Sprint>(`/api/sprints/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    })
  }

  async deleteSprint(id: number): Promise<void> {
    return this.request<void>(`/api/sprints/${id}`, {
      method: 'DELETE',
    })
  }

  // Tag endpoints (project-scoped)
  async getTags(projectId: number): Promise<Tag[]> {
    return this.request<Tag[]>(`/api/projects/${projectId}/tags`)
  }

  async createTag(projectId: number, data: Partial<Tag>): Promise<Tag> {
    return this.request<Tag>(`/api/projects/${projectId}/tags`, {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async updateTag(id: number, data: Partial<Tag>): Promise<Tag> {
    return this.request<Tag>(`/api/tags/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    })
  }

  async deleteTag(id: number): Promise<void> {
    return this.request<void>(`/api/tags/${id}`, {
      method: 'DELETE',
    })
  }

  // Cross-project sharing
  async shareSprint(sprintId: number, projectId: number): Promise<void> {
    return this.request<void>(`/api/sprints/${sprintId}/share`, {
      method: 'POST',
      body: JSON.stringify({ project_id: projectId }),
    })
  }

  async unshareSprint(sprintId: number, projectId: number): Promise<void> {
    return this.request<void>(`/api/sprints/${sprintId}/share/${projectId}`, {
      method: 'DELETE',
    })
  }

  async shareTag(tagId: number, projectId: number): Promise<void> {
    return this.request<void>(`/api/tags/${tagId}/share`, {
      method: 'POST',
      body: JSON.stringify({ project_id: projectId }),
    })
  }

  async unshareTag(tagId: number, projectId: number): Promise<void> {
    return this.request<void>(`/api/tags/${tagId}/share/${projectId}`, {
      method: 'DELETE',
    })
  }

  async shareAttachment(attachmentId: number, projectId: number): Promise<void> {
    return this.request<void>(`/api/attachments/${attachmentId}/share`, {
      method: 'POST',
      body: JSON.stringify({ project_id: projectId }),
    })
  }

  async unshareAttachment(attachmentId: number, projectId: number): Promise<void> {
    return this.request<void>(`/api/attachments/${attachmentId}/share/${projectId}`, {
      method: 'DELETE',
    })
  }

  // API key endpoints
  async getAPIKeys(): Promise<APIKey[]> {
    return this.request<APIKey[]>('/api/api-keys')
  }

  async createAPIKey(data: { name: string; expires_in?: number }): Promise<CreateAPIKeyResponse> {
    return this.request<CreateAPIKeyResponse>('/api/api-keys', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async deleteAPIKey(id: number): Promise<void> {
    return this.request<void>(`/api/api-keys/${id}`, {
      method: 'DELETE',
    })
  }

  // Team endpoints
  async getMyTeam(): Promise<Team> {
    return this.request<Team>('/api/team')
  }

  async getTeamMembers(): Promise<TeamMember[]> {
    return this.request<TeamMember[]>('/api/team/members')
  }

  async inviteTeamMember(email: string): Promise<void> {
    return this.request<void>('/api/team/invite', {
      method: 'POST',
      body: JSON.stringify({ email }),
    })
  }

  async removeTeamMember(memberId: number): Promise<void> {
    return this.request<void>(`/api/team/members/${memberId}`, {
      method: 'DELETE',
    })
  }

  async getMyInvitations(): Promise<TeamInvitation[]> {
    return this.request<TeamInvitation[]>('/api/team/invitations')
  }

  async getMyTeamMemberships(): Promise<TeamMembership[]> {
    return this.request<TeamMembership[]>('/api/team/memberships')
  }

  async acceptInvitation(invitationId: number): Promise<void> {
    return this.request<void>(`/api/team/invitations/${invitationId}/accept`, {
      method: 'POST',
    })
  }

  async rejectInvitation(invitationId: number): Promise<void> {
    return this.request<void>(`/api/team/invitations/${invitationId}/reject`, {
      method: 'POST',
    })
  }

  async getInvitationByToken(token: string): Promise<TokenInvitationInfo> {
    return this.request<TokenInvitationInfo>(`/api/team/invitations/by-token?token=${encodeURIComponent(token)}`)
  }

  async acceptInvitationByToken(token: string): Promise<{ message: string }> {
    return this.request<{ message: string }>('/api/team/invitations/accept-by-token', {
      method: 'POST',
      body: JSON.stringify({ token }),
    })
  }

  async updateTeam(name: string): Promise<Team> {
    return this.request<Team>('/api/team', {
      method: 'PATCH',
      body: JSON.stringify({ name }),
    })
  }

  async getTeamSentInvitations(): Promise<SentInvitation[]> {
    return this.request<SentInvitation[]>('/api/team/invitations/sent')
  }

  async searchTeamUsers(query: string): Promise<UserSearchResult[]> {
    return this.request<UserSearchResult[]>(`/api/team/users/search?q=${encodeURIComponent(query)}`)
  }

  async addTeamMember(userId: number): Promise<{ message: string }> {
    return this.request<{ message: string }>('/api/team/members', {
      method: 'POST',
      body: JSON.stringify({ user_id: userId }),
    })
  }

  // Cloudinary endpoints
  async getCloudinaryCredential(): Promise<CloudinaryCredentialResponse | null> {
    const data = await this.request<CloudinaryCredentialResponse>('/api/settings/cloudinary')
    // Backend returns {} when no credentials exist
    if (!data || !data.cloud_name) return null
    return data
  }

  async saveCloudinaryCredential(data: { cloud_name: string; api_key: string; api_secret: string; max_file_size_mb?: number }): Promise<CloudinaryCredentialResponse> {
    return this.request<CloudinaryCredentialResponse>('/api/settings/cloudinary', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async deleteCloudinaryCredential(): Promise<void> {
    return this.request<void>('/api/settings/cloudinary', {
      method: 'DELETE',
    })
  }

  async testCloudinaryConnection(): Promise<CloudinaryCredentialResponse> {
    return this.request<CloudinaryCredentialResponse>('/api/settings/cloudinary/test', {
      method: 'POST',
    })
  }

  async getUploadSignature(opts: { taskId?: number; pageId?: number }): Promise<{ signature: string; timestamp: number; cloud_name: string; api_key: string; folder: string; public_id: string }> {
    const params = opts.taskId ? `task_id=${opts.taskId}` : `page_id=${opts.pageId}`
    return this.request(`/api/settings/cloudinary/signature?${params}`)
  }

  // Figma endpoints
  async getFigmaCredentials(): Promise<FigmaCredentialsStatus> {
    return this.request<FigmaCredentialsStatus>('/api/user/figma-credentials')
  }

  async saveFigmaCredentials(accessToken: string): Promise<FigmaCredentialsStatus> {
    return this.request<FigmaCredentialsStatus>('/api/user/figma-credentials', {
      method: 'POST',
      body: JSON.stringify({ access_token: accessToken }),
    })
  }

  async deleteFigmaCredentials(): Promise<FigmaCredentialsStatus> {
    return this.request<FigmaCredentialsStatus>('/api/user/figma-credentials', {
      method: 'DELETE',
    })
  }

  async getFigmaEmbed(figmaUrl: string): Promise<FigmaEmbedInfo> {
    return this.request<FigmaEmbedInfo>(`/api/figma/embed?url=${encodeURIComponent(figmaUrl)}`)
  }

  // Backup endpoints (go-backup / Google Drive)
  async getBackupStatus(): Promise<BackupStatus> {
    return this.request<BackupStatus>('/api/admin/backup/status')
  }

  async getBackupSettings(): Promise<BackupSettings> {
    return this.request<BackupSettings>('/api/admin/backup/settings')
  }

  async updateBackupSettings(settings: { enabled?: boolean; cron_expression?: string; folder_id?: string }): Promise<BackupSettings> {
    return this.request<BackupSettings>('/api/admin/backup/settings', {
      method: 'PUT',
      body: JSON.stringify(settings),
    })
  }

  async triggerBackup(): Promise<BackupRecord> {
    return this.request<BackupRecord>('/api/admin/backup/trigger', { method: 'POST' })
  }

  async listBackupHistory(limit = 20): Promise<BackupRecord[]> {
    return this.request<BackupRecord[]>(`/api/admin/backup/history?limit=${limit}`)
  }

  async deleteBackupRecord(id: string): Promise<void> {
    return this.request<void>(`/api/admin/backup/history/${id}`, { method: 'DELETE' })
  }

  async disconnectBackup(): Promise<void> {
    return this.request<void>('/api/admin/backup/oauth/disconnect', { method: 'DELETE' })
  }

  // Task attachment endpoints
  async getTaskAttachments(taskId: number): Promise<Attachment[]> {
    return this.request<Attachment[]>(`/api/tasks/${taskId}/attachments`)
  }

  async createTaskAttachment(taskId: number, data: {
    filename: string; alt_name?: string; file_type: string; content_type: string;
    file_size: number; cloudinary_url: string; cloudinary_public_id: string;
  }): Promise<Attachment> {
    return this.request<Attachment>(`/api/tasks/${taskId}/attachments`, {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async deleteTaskAttachment(taskId: number, attachmentId: number): Promise<void> {
    return this.request<void>(`/api/tasks/${taskId}/attachments/${attachmentId}`, {
      method: 'DELETE',
    })
  }

  // Wiki page attachment endpoints
  async getWikiPageAttachments(pageId: number): Promise<WikiPageAttachment[]> {
    return this.request<WikiPageAttachment[]>(`/api/wiki/pages/${pageId}/attachments`)
  }

  async createWikiPageAttachment(pageId: number, data: {
    filename: string; alt_name?: string; file_type: string; content_type: string;
    file_size: number; cloudinary_url: string; cloudinary_public_id: string;
  }): Promise<WikiPageAttachment> {
    return this.request<WikiPageAttachment>(`/api/wiki/pages/${pageId}/attachments`, {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async deleteWikiPageAttachment(attachmentId: number): Promise<void> {
    return this.request<void>(`/api/wiki/attachments/${attachmentId}`, {
      method: 'DELETE',
    })
  }

  // Storage usage
  async getStorageUsage(projectId: number): Promise<StorageUsageItem[]> {
    return this.request<StorageUsageItem[]>(`/api/projects/${projectId}/storage`)
  }

  // Image library — scoped to a project
  async getImages(projectId: number, query?: string): Promise<Attachment[]> {
    const params = new URLSearchParams({ project_id: String(projectId) })
    if (query) params.set('q', query)
    return this.request<Attachment[]>(`/api/images?${params.toString()}`)
  }

  // Project drawings (go-draw isolation)
  async getProjectDrawings(projectId: number): Promise<ProjectDrawing[]> {
    return this.request<ProjectDrawing[]>(`/api/projects/${projectId}/drawings`)
  }

  async registerProjectDrawing(projectId: number, drawId: string): Promise<void> {
    return this.request<void>(`/api/projects/${projectId}/drawings`, {
      method: 'POST',
      body: JSON.stringify({ draw_id: drawId }),
    })
  }

  // Update attachment
  async updateAttachment(id: number, data: { alt_name?: string }): Promise<Attachment> {
    return this.request<Attachment>(`/api/attachments/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    })
  }

  // Delete attachment (standalone, not task-scoped)
  async deleteAttachment(id: number): Promise<void> {
    return this.request<void>(`/api/attachments/${id}`, {
      method: 'DELETE',
    })
  }

  // Asset management
  async getAssets(projectId: number, params?: { q?: string; type?: string; limit?: number; offset?: number }): Promise<Asset[]> {
    const searchParams = new URLSearchParams()
    searchParams.set('project_id', String(projectId))
    if (params?.q) searchParams.set('q', params.q)
    if (params?.type) searchParams.set('type', params.type)
    if (params?.limit) searchParams.set('limit', String(params.limit))
    if (params?.offset) searchParams.set('offset', String(params.offset))
    return this.request<Asset[]>(`/api/assets?${searchParams.toString()}`)
  }

  // Invite endpoints
  async getInvites(): Promise<{ invites: Invite[]; invite_count: number; is_admin: boolean }> {
    return this.request('/api/invites')
  }

  async createInvite(email?: string): Promise<{ code: string; expires_at: string; email_sent: boolean }> {
    return this.request('/api/invites', {
      method: 'POST',
      body: JSON.stringify(email ? { email } : {}),
    })
  }

  async validateInvite(code: string): Promise<{ valid: boolean; inviter_name?: string; message?: string }> {
    return this.request(`/api/invites/validate?code=${encodeURIComponent(code)}`)
  }

  async adminBoostInvites(userId: number, inviteCount: number): Promise<MessageResponse> {
    return this.request(`/api/admin/users/${userId}/invites`, {
      method: 'PATCH',
      body: JSON.stringify({ invite_count: inviteCount }),
    })
  }

  // Email provider endpoints (admin only)
  async getEmailProvider(): Promise<EmailProviderResponse | null> {
    const data = await this.request<EmailProviderResponse>('/api/admin/settings/email')
    if (!data || !data.sender_email) return null
    return data
  }

  async saveEmailProvider(data: { api_key: string; sender_email: string; sender_name: string }): Promise<EmailProviderResponse> {
    return this.request<EmailProviderResponse>('/api/admin/settings/email', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async deleteEmailProvider(): Promise<void> {
    return this.request<void>('/api/admin/settings/email', {
      method: 'DELETE',
    })
  }

  async testEmailProvider(): Promise<EmailProviderResponse> {
    return this.request<EmailProviderResponse>('/api/admin/settings/email/test', {
      method: 'POST',
    })
  }

  // Swim lane endpoints
  async getSwimLanes(projectId: number): Promise<SwimLane[]> {
    return this.request<SwimLane[]>(`/api/projects/${projectId}/swim-lanes`)
  }

  async createSwimLane(projectId: number, data: CreateSwimLaneRequest): Promise<SwimLane> {
    return this.request<SwimLane>(`/api/projects/${projectId}/swim-lanes`, {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async updateSwimLane(swimLaneId: number, data: UpdateSwimLaneRequest): Promise<SwimLane> {
    return this.request<SwimLane>(`/api/swim-lanes/${swimLaneId}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    })
  }

  async deleteSwimLane(swimLaneId: number): Promise<void> {
    return this.request<void>(`/api/swim-lanes/${swimLaneId}`, {
      method: 'DELETE',
    })
  }

  // Wiki endpoints
  async getWikiPages(projectId: number): Promise<WikiPage[]> {
    return this.request<WikiPage[]>(`/api/projects/${projectId}/wiki/pages`)
  }

  async createWikiPage(projectId: number, title: string): Promise<WikiPage> {
    return this.request<WikiPage>(`/api/projects/${projectId}/wiki/pages`, {
      method: 'POST',
      body: JSON.stringify({ title }),
    })
  }

  async getWikiPage(pageId: number): Promise<WikiPage> {
    return this.request<WikiPage>(`/api/wiki/pages/${pageId}`)
  }

  async updateWikiPage(pageId: number, data: { title?: string }): Promise<WikiPage> {
    return this.request<WikiPage>(`/api/wiki/pages/${pageId}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    })
  }

  async deleteWikiPage(pageId: number): Promise<void> {
    return this.request<void>(`/api/wiki/pages/${pageId}`, {
      method: 'DELETE',
    })
  }

  async getWikiPageContent(pageId: number): Promise<{ page_id: number; content: string; updated_at: string }> {
    return this.request<{ page_id: number; content: string; updated_at: string }>(`/api/wiki/pages/${pageId}/content`)
  }

  async updateWikiPageContent(pageId: number, content: string, manualSave = false): Promise<{ page_id: number; content: string; updated_at: string }> {
    return this.request<{ page_id: number; content: string; updated_at: string }>(`/api/wiki/pages/${pageId}/content`, {
      method: 'PUT',
      body: JSON.stringify({ content, manual_save: manualSave }),
    })
  }

  async getWikiPageVersions(pageId: number): Promise<WikiPageVersion[]> {
    return this.request<WikiPageVersion[]>(`/api/wiki/pages/${pageId}/versions`)
  }

  async getWikiPageVersion(pageId: number, versionNumber: number): Promise<WikiPageVersionWithContent> {
    return this.request<WikiPageVersionWithContent>(`/api/wiki/pages/${pageId}/versions/${versionNumber}`)
  }

  async restoreWikiPageVersion(pageId: number, versionNumber: number): Promise<{ page_id: number; content: string; updated_at: string }> {
    return this.request<{ page_id: number; content: string; updated_at: string }>(
      `/api/wiki/pages/${pageId}/versions/${versionNumber}/restore`,
      { method: 'POST' },
    )
  }

  async searchWiki(query: string, projectId?: number, limit?: number): Promise<{ results: WikiSearchResult[]; total: number }> {
    return this.request('/api/wiki/search', {
      method: 'POST',
      body: JSON.stringify({ query, project_id: projectId, limit }),
    })
  }

  async globalSearch(query: string, projectId?: number, types?: string[], limit?: number, signal?: AbortSignal): Promise<GlobalSearchResponse> {
    return this.request<GlobalSearchResponse>('/api/search', {
      method: 'POST',
      body: JSON.stringify({
        query,
        project_id: projectId,
        types,
        limit,
      }),
      signal,
    })
  }

  // Knowledge Graph endpoints
  async getProjectGraph(projectId: number): Promise<GraphData> {
    return this.request<GraphData>(`/api/projects/${projectId}/graph`)
  }

  // Wiki annotation endpoints
  async listWikiAnnotations(pageId: number): Promise<WikiAnnotation[]> {
    return this.request<WikiAnnotation[]>(`/api/wiki/pages/${pageId}/annotations`)
  }

  async createWikiAnnotation(
    pageId: number,
    data: { start_offset: number; end_offset: number; selected_text: string; color: AnnotationColor; comment?: string }
  ): Promise<WikiAnnotation> {
    return this.request<WikiAnnotation>(`/api/wiki/pages/${pageId}/annotations`, {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async updateWikiAnnotation(
    annotationId: number,
    data: { color?: AnnotationColor; resolved?: boolean }
  ): Promise<WikiAnnotation> {
    return this.request<WikiAnnotation>(`/api/wiki/annotations/${annotationId}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    })
  }

  async deleteWikiAnnotation(annotationId: number): Promise<void> {
    return this.request<void>(`/api/wiki/annotations/${annotationId}`, { method: 'DELETE' })
  }

  async createAnnotationComment(
    annotationId: number,
    data: { content: string; parent_comment_id?: number }
  ): Promise<AnnotationComment> {
    return this.request<AnnotationComment>(`/api/wiki/annotations/${annotationId}/comments`, {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async updateAnnotationComment(
    commentId: number,
    data: { content?: string; resolved?: boolean }
  ): Promise<AnnotationComment> {
    return this.request<AnnotationComment>(`/api/wiki/annotation-comments/${commentId}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    })
  }

  async deleteAnnotationComment(commentId: number): Promise<void> {
    return this.request<void>(`/api/wiki/annotation-comments/${commentId}`, { method: 'DELETE' })
  }

  // Notification endpoints
  async getNotifications(): Promise<AppNotification[]> {
    return this.request<AppNotification[]>('/api/notifications')
  }

  async getNotificationCount(): Promise<{ count: number }> {
    return this.request<{ count: number }>('/api/notifications/count')
  }

  async markNotificationsRead(ids: number[]): Promise<void> {
    return this.request<void>('/api/notifications/mark-read', {
      method: 'POST',
      body: JSON.stringify({ ids }),
    })
  }

  async markAllNotificationsRead(): Promise<void> {
    return this.request<void>('/api/notifications/mark-all-read', { method: 'POST' })
  }

  // User profile
  async getUserProfile(userId: number): Promise<UserProfile> {
    return this.request<UserProfile>(`/api/users/${userId}/profile`)
  }

  async getUserActivityFeed(userId: number, before?: string): Promise<UserActivityPage> {
    const qs = before ? `?before=${encodeURIComponent(before)}` : ''
    return this.request<UserActivityPage>(`/api/users/${userId}/activity${qs}`)
  }

  // Version endpoint
  async getVersion(): Promise<{
    version: string
    git_commit: string
    build_time: string
    go_version: string
    platform: string
    server_time: string
    db_version: number
    environment: string
    db_driver: string
  }> {
    return this.request('/api/version')
  }
}

// Export a singleton instance
export const api = new ApiClient(API_BASE_URL)
export const apiClient = api // Alias for consistency
