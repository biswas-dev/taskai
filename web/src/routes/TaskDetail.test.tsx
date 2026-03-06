import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import TaskDetail from './TaskDetail'

const mockNavigate = vi.fn()
vi.mock('react-router-dom', () => ({
  useParams: () => ({ projectId: '7', taskNumber: '1' }),
  useNavigate: () => mockNavigate,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  Link: ({ children, to, className, onClick }: any) => <a href={to} className={className} onClick={onClick}>{children}</a>,
}))

// Mock ReactMarkdown
vi.mock('react-markdown', () => ({
  default: ({ children }: { children: string }) => <div>{children}</div>,
}))

vi.mock('remark-gfm', () => ({
  default: () => null,
}))

const mocks = vi.hoisted(() => ({
  getTasks: vi.fn(),
  getTaskByNumber: vi.fn(),
  updateTask: vi.fn(),
  getSprints: vi.fn(),
  getSwimLanes: vi.fn(),
  getTaskComments: vi.fn(),
  createTaskComment: vi.fn(),
  getProjectMembers: vi.fn(),
  getTaskAttachments: vi.fn(),
  createTaskAttachment: vi.fn(),
  deleteTaskAttachment: vi.fn(),
  getUploadSignature: vi.fn(),
  updateAttachment: vi.fn(),
  getProjectGitHub: vi.fn(),
  getProjectTags: vi.fn(),
}))

vi.mock('../lib/api', () => ({
  apiClient: mocks,
}))

const task1 = {
  id: 1,
  project_id: 7,
  task_number: 1,
  title: 'Fix bug in login',
  description: 'The login form crashes on empty submit',
  status: 'in_progress',
  priority: 'high',
  swim_lane_id: 2,
  assignee_id: null,
  sprint_id: null,
  due_date: '2024-02-01',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-15T00:00:00Z',
}

describe('TaskDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getTaskByNumber.mockResolvedValue(task1)
    mocks.getSprints.mockResolvedValue([])
    mocks.getSwimLanes.mockResolvedValue([
      { id: 1, project_id: 7, name: 'To Do', color: '#6B7280', position: 0, status_category: 'todo' },
      { id: 2, project_id: 7, name: 'In Progress', color: '#3B82F6', position: 1, status_category: 'in_progress' },
    ])
    mocks.getTaskComments.mockResolvedValue([])
    mocks.getProjectMembers.mockResolvedValue([])
    mocks.getTaskAttachments.mockResolvedValue([])
    mocks.getProjectGitHub.mockResolvedValue({ github_owner: '', github_repo_name: '', github_token_set: false, github_branch: 'main', github_sync_enabled: false, github_last_sync: null, github_login: null })
    mocks.getProjectTags.mockResolvedValue([])
  })

  it('shows loading state initially', () => {
    mocks.getTaskByNumber.mockReturnValue(new Promise(() => {}))
    render(<TaskDetail />)
    expect(screen.queryByText('Fix bug in login')).not.toBeInTheDocument()
  })

  it('displays task title after loading', async () => {
    render(<TaskDetail />)
    await waitFor(() => {
      expect(screen.getByText('Fix bug in login')).toBeInTheDocument()
    })
  })

  it('displays task description', async () => {
    render(<TaskDetail />)
    await waitFor(() => {
      expect(screen.getByText('The login form crashes on empty submit')).toBeInTheDocument()
    })
  })

  it('displays task status badge', async () => {
    render(<TaskDetail />)
    await waitFor(() => {
      expect(screen.getByText('Fix bug in login')).toBeInTheDocument()
    })
    // Status shown as badge span in header
    const statusBadges = screen.getAllByText('In Progress')
    expect(statusBadges.length).toBeGreaterThanOrEqual(1)
  })

  it('displays task priority badge', async () => {
    render(<TaskDetail />)
    await waitFor(() => {
      expect(screen.getByText('Fix bug in login')).toBeInTheDocument()
    })
    // Priority shown as badge span in header (lowercase value)
    expect(screen.getByText('high')).toBeInTheDocument()
  })

  it('shows error when task not found', async () => {
    mocks.getTaskByNumber.mockRejectedValue(new Error('Task not found'))
    render(<TaskDetail />)
    await waitFor(() => {
      expect(screen.getByText('Task not found')).toBeInTheDocument()
    })
  })

  it('shows error on API failure', async () => {
    mocks.getTaskByNumber.mockRejectedValue(new Error('Network error'))
    render(<TaskDetail />)
    await waitFor(() => {
      expect(screen.getByText('Network error')).toBeInTheDocument()
    })
  })

  it('renders comments section', async () => {
    mocks.getTaskComments.mockResolvedValue([
      {
        id: 1,
        task_id: 1,
        user_id: 1,
        user_name: 'Alice',
        comment: 'Working on this now',
        created_at: '2024-01-10T00:00:00Z',
        updated_at: '2024-01-10T00:00:00Z',
      },
    ])

    render(<TaskDetail />)
    await waitFor(() => {
      expect(screen.getByText('Working on this now')).toBeInTheDocument()
    })
  })

  it('renders attachments section', async () => {
    mocks.getTaskAttachments.mockResolvedValue([
      {
        id: 1,
        task_id: 1,
        filename: 'screenshot.png',
        alt_name: 'Bug screenshot',
        file_type: 'image',
        content_type: 'image/png',
        file_size: 204800,
        cloudinary_url: 'https://res.cloudinary.com/test/image/upload/screenshot.png',
        cloudinary_public_id: 'test/screenshot',
        created_at: '2024-01-10T00:00:00Z',
      },
    ])

    render(<TaskDetail />)
    await waitFor(() => {
      expect(screen.getByText('Bug screenshot')).toBeInTheDocument()
    })
  })

  it('renders swim lane selector in sidebar', async () => {
    render(<TaskDetail />)
    await waitFor(() => {
      expect(screen.getByText('Fix bug in login')).toBeInTheDocument()
    })
    // Swim lane shown via InlineSelect (select element)
    expect(screen.getByDisplayValue('In Progress')).toBeInTheDocument()
  })
})
