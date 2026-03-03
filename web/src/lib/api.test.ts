import { describe, it, expect, beforeEach, vi } from 'vitest'

// We need to mock fetch before importing the API client
const mockFetch = vi.fn()
global.fetch = mockFetch

// Mock localStorage
const localStorageMock = {
  getItem: vi.fn(),
  setItem: vi.fn(),
  removeItem: vi.fn(),
  clear: vi.fn(),
}
Object.defineProperty(global, 'localStorage', { value: localStorageMock })

// Dynamic import type
type ApiModule = typeof import('./api')
let apiClient: ApiModule['apiClient']

beforeEach(async () => {
  vi.resetModules()
  mockFetch.mockReset()
  localStorageMock.getItem.mockReturnValue('test-token')
  localStorageMock.setItem.mockClear()
  localStorageMock.removeItem.mockClear()

  // Re-import to get fresh instance
  const mod: ApiModule = await import('./api')
  apiClient = mod.apiClient
})

function mockResponse(data: unknown, status = 200) {
  mockFetch.mockResolvedValueOnce({
    ok: status >= 200 && status < 300,
    status,
    headers: {
      get: (name: string) => name === 'content-type' ? 'application/json' : null,
    },
    json: () => Promise.resolve(data),
  })
}

function mock204() {
  mockFetch.mockResolvedValueOnce({
    ok: true,
    status: 204,
    headers: {
      get: (name: string) => name === 'content-type' ? 'application/json' : null,
    },
    json: () => Promise.resolve({}),
  })
}

function mockErrorResponse(error: string, status = 400) {
  mockFetch.mockResolvedValueOnce({
    ok: false,
    status,
    headers: {
      get: (name: string) => name === 'content-type' ? 'application/json' : null,
    },
    json: () => Promise.resolve({ error }),
  })
}

describe('ApiClient', () => {
  // --- Auth ---
  describe('signup', () => {
    it('sends POST with credentials and stores token', async () => {
      mockResponse({ token: 'new-token', user: { id: 1, email: 'u@e.com' } }, 201)
      const result = await apiClient.signup({ email: 'u@e.com', password: 'pass123' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/auth/signup'),
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ email: 'u@e.com', password: 'pass123' }),
        })
      )
      expect(result.token).toBe('new-token')
      expect(localStorageMock.setItem).toHaveBeenCalledWith('auth_token', 'new-token')
    })
  })

  describe('login', () => {
    it('sends POST with credentials and stores token', async () => {
      mockResponse({ token: 'login-token', user: { id: 1, email: 'u@e.com' } })
      const result = await apiClient.login({ email: 'u@e.com', password: 'pass' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/auth/login'),
        expect.objectContaining({ method: 'POST' })
      )
      expect(result.token).toBe('login-token')
      expect(localStorageMock.setItem).toHaveBeenCalledWith('auth_token', 'login-token')
    })
  })

  describe('logout', () => {
    it('clears token from localStorage', () => {
      apiClient.logout()
      expect(localStorageMock.removeItem).toHaveBeenCalledWith('auth_token')
    })
  })

  describe('getCurrentUser', () => {
    it('sends GET to /api/me', async () => {
      mockResponse({ id: 1, email: 'u@e.com' })
      const user = await apiClient.getCurrentUser()

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/me'),
        expect.any(Object)
      )
      expect(user.email).toBe('u@e.com')
    })
  })

  // --- Projects ---
  describe('getProjects', () => {
    it('sends GET to /api/projects', async () => {
      mockResponse([{ id: 1, name: 'Project 1' }])
      const projects = await apiClient.getProjects()

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/projects'),
        expect.any(Object)
      )
      expect(projects).toHaveLength(1)
    })
  })

  describe('getProject', () => {
    it('sends GET to /api/projects/:id', async () => {
      mockResponse({ id: 5, name: 'My Project' })
      const project = await apiClient.getProject(5)

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/projects/5')
      expect(project.name).toBe('My Project')
    })
  })

  describe('createProject', () => {
    it('sends POST with project data', async () => {
      mockResponse({ id: 1, name: 'New' })
      await apiClient.createProject({ name: 'New', description: 'desc' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/projects'),
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ name: 'New', description: 'desc' }),
        })
      )
    })
  })

  describe('updateProject', () => {
    it('sends PATCH with update data', async () => {
      mockResponse({ id: 1, name: 'Updated' })
      await apiClient.updateProject(1, { name: 'Updated' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/projects/1'),
        expect.objectContaining({
          method: 'PATCH',
          body: JSON.stringify({ name: 'Updated' }),
        })
      )
    })
  })

  describe('deleteProject', () => {
    it('sends DELETE to correct path', async () => {
      mock204()
      await apiClient.deleteProject(3)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/projects/3'),
        expect.objectContaining({ method: 'DELETE' })
      )
    })
  })

  // --- Tasks ---
  describe('getTasks', () => {
    it('sends GET to /api/projects/:id/tasks', async () => {
      mockResponse([{ id: 1, title: 'Task 1' }])
      const tasks = await apiClient.getTasks(10)

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/projects/10/tasks')
      expect(tasks).toHaveLength(1)
    })
  })

  describe('createTask', () => {
    it('sends POST with task data', async () => {
      mockResponse({ id: 1, title: 'New Task' })
      await apiClient.createTask(10, { title: 'New Task', status: 'todo' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/projects/10/tasks'),
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ title: 'New Task', status: 'todo' }),
        })
      )
    })
  })

  describe('updateTask', () => {
    it('sends PATCH with task update data', async () => {
      mockResponse({ id: 5, title: 'Updated' })
      await apiClient.updateTask(5, { title: 'Updated', status: 'done' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/tasks/5'),
        expect.objectContaining({
          method: 'PATCH',
          body: JSON.stringify({ title: 'Updated', status: 'done' }),
        })
      )
    })
  })

  describe('deleteTask', () => {
    it('sends DELETE to /api/tasks/:id', async () => {
      mock204()
      await apiClient.deleteTask(7)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/tasks/7'),
        expect.objectContaining({ method: 'DELETE' })
      )
    })
  })

  // --- Task Comments ---
  describe('getTaskComments', () => {
    it('sends GET to correct path', async () => {
      mockResponse([{ id: 1, comment: 'Hello' }])
      await apiClient.getTaskComments(3)

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/tasks/3/comments')
    })
  })

  describe('createTaskComment', () => {
    it('sends POST with comment', async () => {
      mockResponse({ id: 1, comment: 'Nice work' })
      await apiClient.createTaskComment(3, 'Nice work')

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/tasks/3/comments'),
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ comment: 'Nice work' }),
        })
      )
    })
  })

  // --- Sprints ---
  describe('getSprints', () => {
    it('sends GET to /api/projects/:id/sprints', async () => {
      mockResponse([])
      await apiClient.getSprints(42)

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/projects/42/sprints')
    })
  })

  describe('createSprint', () => {
    it('sends POST with sprint data', async () => {
      mockResponse({ id: 1, name: 'Sprint 1' })
      await apiClient.createSprint(42, { name: 'Sprint 1', status: 'active' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/projects/42/sprints'),
        expect.objectContaining({ method: 'POST' })
      )
    })
  })

  describe('updateSprint', () => {
    it('sends PATCH to /api/sprints/:id', async () => {
      mockResponse({ id: 1, name: 'Updated' })
      await apiClient.updateSprint(1, { name: 'Updated' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/sprints/1'),
        expect.objectContaining({ method: 'PATCH' })
      )
    })
  })

  describe('deleteSprint', () => {
    it('sends DELETE to /api/sprints/:id', async () => {
      mock204()
      await apiClient.deleteSprint(2)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/sprints/2'),
        expect.objectContaining({ method: 'DELETE' })
      )
    })
  })

  // --- Tags ---
  describe('getTags', () => {
    it('sends GET to /api/projects/:id/tags', async () => {
      mockResponse([])
      await apiClient.getTags(42)

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/projects/42/tags')
    })
  })

  describe('createTag', () => {
    it('sends POST with tag data', async () => {
      mockResponse({ id: 1, name: 'bug', color: '#FF0000' })
      await apiClient.createTag(42, { name: 'bug', color: '#FF0000' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/projects/42/tags'),
        expect.objectContaining({ method: 'POST' })
      )
    })
  })

  describe('updateTag', () => {
    it('sends PATCH to /api/tags/:id', async () => {
      mockResponse({ id: 1, name: 'updated' })
      await apiClient.updateTag(1, { name: 'updated' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/tags/1'),
        expect.objectContaining({ method: 'PATCH' })
      )
    })
  })

  describe('deleteTag', () => {
    it('sends DELETE to /api/tags/:id', async () => {
      mock204()
      await apiClient.deleteTag(3)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/tags/3'),
        expect.objectContaining({ method: 'DELETE' })
      )
    })
  })

  // --- API Keys ---
  describe('getAPIKeys', () => {
    it('sends GET to /api/api-keys', async () => {
      mockResponse([])
      await apiClient.getAPIKeys()

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/api-keys')
    })
  })

  describe('createAPIKey', () => {
    it('sends POST with name', async () => {
      mockResponse({ id: 1, name: 'My Key', key: 'abc123' })
      await apiClient.createAPIKey({ name: 'My Key' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/api-keys'),
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ name: 'My Key' }),
        })
      )
    })
  })

  describe('deleteAPIKey', () => {
    it('sends DELETE to /api/api-keys/:id', async () => {
      mock204()
      await apiClient.deleteAPIKey(5)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/api-keys/5'),
        expect.objectContaining({ method: 'DELETE' })
      )
    })
  })

  // --- Team ---
  describe('getMyTeam', () => {
    it('sends GET to /api/team', async () => {
      mockResponse({ id: 1, name: 'My Team' })
      await apiClient.getMyTeam()

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/team')
    })
  })

  describe('getTeamMembers', () => {
    it('sends GET to /api/team/members', async () => {
      mockResponse([])
      await apiClient.getTeamMembers()

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/team/members')
    })
  })

  describe('inviteTeamMember', () => {
    it('sends POST with email', async () => {
      mock204()
      await apiClient.inviteTeamMember('new@team.com')

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/team/invite'),
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ email: 'new@team.com' }),
        })
      )
    })
  })

  describe('removeTeamMember', () => {
    it('sends DELETE to /api/team/members/:id', async () => {
      mock204()
      await apiClient.removeTeamMember(5)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/team/members/5'),
        expect.objectContaining({ method: 'DELETE' })
      )
    })
  })

  describe('acceptInvitation', () => {
    it('sends POST to accept endpoint', async () => {
      mock204()
      await apiClient.acceptInvitation(10)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/team/invitations/10/accept'),
        expect.objectContaining({ method: 'POST' })
      )
    })
  })

  describe('rejectInvitation', () => {
    it('sends POST to reject endpoint', async () => {
      mock204()
      await apiClient.rejectInvitation(10)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/team/invitations/10/reject'),
        expect.objectContaining({ method: 'POST' })
      )
    })
  })

  // --- Security ---
  describe('changePassword', () => {
    it('sends POST with password data', async () => {
      mockResponse({ message: 'Password changed' })
      await apiClient.changePassword({ current_password: 'old', new_password: 'new1234' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/settings/password'),
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ current_password: 'old', new_password: 'new1234' }),
        })
      )
    })
  })

  describe('get2FAStatus', () => {
    it('sends GET to 2fa status', async () => {
      mockResponse({ enabled: false })
      const result = await apiClient.get2FAStatus()

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/settings/2fa/status')
      expect(result.enabled).toBe(false)
    })
  })

  describe('setup2FA', () => {
    it('sends POST to 2fa setup', async () => {
      mockResponse({ secret: 'TOTP123', qr_code_url: 'url', qr_code_svg: '<svg>' })
      await apiClient.setup2FA()

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/settings/2fa/setup'),
        expect.objectContaining({ method: 'POST' })
      )
    })
  })

  describe('enable2FA', () => {
    it('sends POST with code', async () => {
      mockResponse({ backup_codes: ['code1', 'code2'] })
      const result = await apiClient.enable2FA({ code: '123456' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/settings/2fa/enable'),
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ code: '123456' }),
        })
      )
      expect(result.backup_codes).toHaveLength(2)
    })
  })

  describe('disable2FA', () => {
    it('sends POST with password', async () => {
      mockResponse({ message: '2FA disabled' })
      await apiClient.disable2FA({ password: 'mypassword' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/settings/2fa/disable'),
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ password: 'mypassword' }),
        })
      )
    })
  })

  // --- Swim Lanes ---
  describe('getSwimLanes', () => {
    it('sends GET to /api/projects/:id/swim-lanes', async () => {
      mockResponse([])
      await apiClient.getSwimLanes(5)

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/projects/5/swim-lanes')
    })
  })

  describe('createSwimLane', () => {
    it('sends POST with swim lane data', async () => {
      mockResponse({ id: 1, name: 'In Progress', color: '#00F', position: 1, status_category: 'in_progress' })
      await apiClient.createSwimLane(5, { name: 'In Progress', color: '#00F', position: 1, status_category: 'in_progress' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/projects/5/swim-lanes'),
        expect.objectContaining({ method: 'POST' })
      )
    })
  })

  describe('updateSwimLane', () => {
    it('sends PATCH to /api/swim-lanes/:id', async () => {
      mockResponse({ id: 3, name: 'Updated' })
      await apiClient.updateSwimLane(3, { name: 'Updated' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/swim-lanes/3'),
        expect.objectContaining({ method: 'PATCH' })
      )
    })
  })

  describe('deleteSwimLane', () => {
    it('sends DELETE to /api/swim-lanes/:id', async () => {
      mock204()
      await apiClient.deleteSwimLane(3)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/swim-lanes/3'),
        expect.objectContaining({ method: 'DELETE' })
      )
    })
  })

  // --- Cloudinary ---
  describe('getCloudinaryCredential', () => {
    it('returns credentials when present', async () => {
      mockResponse({ cloud_name: 'mycloud', api_key: 'key123' })
      const cred = await apiClient.getCloudinaryCredential()

      expect(cred).not.toBeNull()
      expect(cred!.cloud_name).toBe('mycloud')
    })

    it('returns null when no credentials exist', async () => {
      mockResponse({})
      const cred = await apiClient.getCloudinaryCredential()

      expect(cred).toBeNull()
    })
  })

  describe('saveCloudinaryCredential', () => {
    it('sends POST with credential data', async () => {
      mockResponse({ id: 1, cloud_name: 'mycloud' })
      await apiClient.saveCloudinaryCredential({
        cloud_name: 'mycloud',
        api_key: 'key',
        api_secret: 'secret',
      })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/settings/cloudinary'),
        expect.objectContaining({ method: 'POST' })
      )
    })
  })

  describe('deleteCloudinaryCredential', () => {
    it('sends DELETE', async () => {
      mock204()
      await apiClient.deleteCloudinaryCredential()

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/settings/cloudinary'),
        expect.objectContaining({ method: 'DELETE' })
      )
    })
  })

  // --- Invites ---
  describe('getInvites', () => {
    it('sends GET to /api/invites', async () => {
      mockResponse({ invites: [], invite_count: 3, is_admin: false })
      const result = await apiClient.getInvites()

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/invites')
      expect(result.invite_count).toBe(3)
    })
  })

  describe('createInvite', () => {
    it('sends POST to /api/invites', async () => {
      mockResponse({ code: 'abc123', expires_at: '2026-03-01' })
      const result = await apiClient.createInvite()

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/invites'),
        expect.objectContaining({ method: 'POST' })
      )
      expect(result.code).toBe('abc123')
    })
  })

  describe('validateInvite', () => {
    it('sends GET with code param', async () => {
      mockResponse({ valid: true, inviter_name: 'Alice' })
      const result = await apiClient.validateInvite('test-code')

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/invites/validate?code=test-code')
      expect(result.valid).toBe(true)
    })
  })

  // --- Project Members ---
  describe('getProjectMembers', () => {
    it('sends GET to /api/projects/:id/members', async () => {
      mockResponse([])
      await apiClient.getProjectMembers(5)

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/projects/5/members')
    })
  })

  describe('addProjectMember', () => {
    it('sends POST with member data', async () => {
      mockResponse({ id: 1 })
      await apiClient.addProjectMember(5, { email: 'new@member.com', role: 'editor' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/projects/5/members'),
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ email: 'new@member.com', role: 'editor' }),
        })
      )
    })
  })

  describe('removeProjectMember', () => {
    it('sends DELETE to /api/projects/:id/members/:memberId', async () => {
      mock204()
      await apiClient.removeProjectMember(5, 3)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/projects/5/members/3'),
        expect.objectContaining({ method: 'DELETE' })
      )
    })
  })

  // --- Admin ---
  describe('getUsers', () => {
    it('sends GET to /api/admin/users', async () => {
      mockResponse([])
      await apiClient.getUsers()

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/admin/users')
    })
  })

  describe('updateUserAdmin', () => {
    it('sends PATCH with admin flag', async () => {
      mockResponse({ id: 1, is_admin: true })
      await apiClient.updateUserAdmin(1, true)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/admin/users/1/admin'),
        expect.objectContaining({
          method: 'PATCH',
          body: JSON.stringify({ is_admin: true }),
        })
      )
    })
  })

  // --- Task Attachments ---
  describe('getTaskAttachments', () => {
    it('sends GET to /api/tasks/:taskId/attachments', async () => {
      mockResponse([])
      await apiClient.getTaskAttachments(5)

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/tasks/5/attachments')
    })
  })

  describe('deleteTaskAttachment', () => {
    it('sends DELETE to /api/tasks/:taskId/attachments/:id', async () => {
      mock204()
      await apiClient.deleteTaskAttachment(5, 3)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/tasks/5/attachments/3'),
        expect.objectContaining({ method: 'DELETE' })
      )
    })
  })

  // --- Assets ---
  describe('getAssets', () => {
    it('calls correct URL with project_id', async () => {
      mockResponse([])
      await apiClient.getAssets(42)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/assets'),
        expect.any(Object)
      )
      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('project_id=42')
    })

    it('passes query params correctly', async () => {
      mockResponse([])
      await apiClient.getAssets(42, { q: 'photo', type: 'image', limit: 20 })

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('q=photo')
      expect(url).toContain('type=image')
      expect(url).toContain('limit=20')
    })

    it('URL encodes search query', async () => {
      mockResponse([])
      await apiClient.getAssets(42, { q: 'hello world' })

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('q=hello+world')
    })

    it('passes offset param', async () => {
      mockResponse([])
      await apiClient.getAssets(42, { offset: 10 })

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('offset=10')
    })
  })

  describe('deleteAttachment', () => {
    it('sends DELETE to correct path', async () => {
      mock204()
      await apiClient.deleteAttachment(5)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/attachments/5'),
        expect.objectContaining({ method: 'DELETE' })
      )
    })
  })

  describe('updateAttachment', () => {
    it('sends PATCH with alt_name data', async () => {
      mockResponse({ id: 5, alt_name: 'new name' })
      await apiClient.updateAttachment(5, { alt_name: 'new name' })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/attachments/5'),
        expect.objectContaining({
          method: 'PATCH',
          body: JSON.stringify({ alt_name: 'new name' }),
        })
      )
    })
  })

  describe('getStorageUsage', () => {
    it('calls correct project storage URL', async () => {
      mockResponse([{ user_id: 1, file_count: 5, total_size: 1024 }])
      await apiClient.getStorageUsage(42)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/projects/42/storage'),
        expect.any(Object)
      )
    })
  })

  describe('getImages', () => {
    it('calls /api/images with project_id', async () => {
      mockResponse([])
      await apiClient.getImages(42)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/images'),
        expect.any(Object)
      )
      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('project_id=42')
    })

    it('passes search query alongside project_id', async () => {
      mockResponse([])
      await apiClient.getImages(42, 'sunset')

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('project_id=42')
      expect(url).toContain('q=sunset')
    })
  })

  // --- Health ---
  describe('healthCheck', () => {
    it('sends GET to /healthz', async () => {
      mockResponse({ status: 'ok', database: 'connected' })
      const result = await apiClient.healthCheck()

      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/healthz')
      expect(result.status).toBe('ok')
    })
  })

  // --- Error handling ---
  describe('error handling', () => {
    it('throws on non-ok response', async () => {
      mockErrorResponse('Not authorized', 401)
      await expect(apiClient.getAssets(1)).rejects.toThrow('Not authorized')
    })

    it('throws generic error for 500 errors', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        headers: {
          get: (name: string) => name === 'content-type' ? 'application/json' : null,
        },
        json: () => Promise.resolve({ error: 'Internal server error' }),
      })
      await expect(apiClient.getAssets(1)).rejects.toThrow('Internal server error')
    })

    it('handles 204 No Content responses', async () => {
      mock204()
      const result = await apiClient.deleteProject(1)
      expect(result).toEqual({})
    })
  })

  // --- Authorization ---
  describe('authorization', () => {
    it('includes Bearer token in requests', async () => {
      mockResponse([])
      await apiClient.getAssets(1)

      const config = mockFetch.mock.calls[0][1] as RequestInit
      expect((config.headers as Record<string, string>)['Authorization']).toContain('Bearer')
    })

    it('sets Content-Type to application/json', async () => {
      mockResponse([])
      await apiClient.getAssets(1)

      const config = mockFetch.mock.calls[0][1] as RequestInit
      expect((config.headers as Record<string, string>)['Content-Type']).toBe('application/json')
    })
  })

  // --- Token management ---
  describe('token management', () => {
    it('setToken stores in localStorage', () => {
      apiClient.setToken('new-token')
      expect(localStorageMock.setItem).toHaveBeenCalledWith('auth_token', 'new-token')
    })

    it('setToken(null) removes from localStorage', () => {
      apiClient.setToken(null)
      expect(localStorageMock.removeItem).toHaveBeenCalledWith('auth_token')
    })

    it('getToken returns current token', () => {
      // Token was loaded from localStorage mock returning 'test-token'
      expect(apiClient.getToken()).toBe('test-token')
    })
  })
})
