// Import generated types from OpenAPI spec
import type { components, operations } from './api.types'

// Re-export types for convenience
export type User = components['schemas']['User']
export type AuthResponse = components['schemas']['AuthResponse']
export type SignupRequest = components['schemas']['SignupRequest']
export type LoginRequest = components['schemas']['LoginRequest']
export type Project = components['schemas']['Project']
export type Task = components['schemas']['Task'] & { task_number?: number }
export type ApiError = components['schemas']['Error']
// Types with required fields for commonly used API responses
export interface TaskComment {
  id: number
  task_id: number
  user_id: number
  user_name?: string | null
  comment: string
  created_at: string
  updated_at: string
}

export interface Sprint {
  id: number
  user_id: number
  name: string
  start_date?: string
  end_date?: string
  goal: string
  status: string
  created_at: string
  updated_at?: string
}

export interface Tag {
  id: number
  user_id: number
  name: string
  color: string
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

export interface WikiPage {
  id: number
  project_id: number
  title: string
  slug: string
  created_by: number
  created_at: string
  updated_at: string
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
  github_last_sync: string | null
}

export interface UserWithStats {
  id: number
  email: string
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
  due_date?: string | null
  sprint_id?: number | null
  priority?: 'low' | 'medium' | 'high' | 'urgent'
  assignee_id?: number | null
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

  // Project settings - GitHub
  async getProjectGitHub(projectId: number): Promise<ProjectGitHubSettings> {
    return this.request<ProjectGitHubSettings>(`/api/projects/${projectId}/github`)
  }

  async updateProjectGitHub(projectId: number, data: Partial<ProjectGitHubSettings>): Promise<ProjectGitHubSettings> {
    return this.request<ProjectGitHubSettings>(`/api/projects/${projectId}/github`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    })
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

  // Sprint endpoints
  async getSprints(): Promise<Sprint[]> {
    return this.request<Sprint[]>('/api/sprints')
  }

  async createSprint(data: Partial<Sprint>): Promise<Sprint> {
    return this.request<Sprint>('/api/sprints', {
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

  // Tag endpoints
  async getTags(): Promise<Tag[]> {
    return this.request<Tag[]>('/api/tags')
  }

  async createTag(data: Partial<Tag>): Promise<Tag> {
    return this.request<Tag>('/api/tags', {
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

  // Image library
  async getImages(query?: string): Promise<Attachment[]> {
    const params = query ? `?q=${encodeURIComponent(query)}` : ''
    return this.request<Attachment[]>(`/api/images${params}`)
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
  async getAssets(params?: { q?: string; type?: string; limit?: number; offset?: number }): Promise<Asset[]> {
    const searchParams = new URLSearchParams()
    if (params?.q) searchParams.set('q', params.q)
    if (params?.type) searchParams.set('type', params.type)
    if (params?.limit) searchParams.set('limit', String(params.limit))
    if (params?.offset) searchParams.set('offset', String(params.offset))
    const qs = searchParams.toString()
    return this.request<Asset[]>(`/api/assets${qs ? '?' + qs : ''}`)
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

  async updateWikiPageContent(pageId: number, content: string): Promise<{ page_id: number; content: string; updated_at: string }> {
    return this.request<{ page_id: number; content: string; updated_at: string }>(`/api/wiki/pages/${pageId}/content`, {
      method: 'PUT',
      body: JSON.stringify({ content }),
    })
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
