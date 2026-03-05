import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import ProjectDetail from './ProjectDetail'

const mockNavigate = vi.fn()
const mockSetSearchParams = vi.fn()
vi.mock('react-router-dom', () => ({
  useParams: () => ({ projectId: '7' }),
  useNavigate: () => mockNavigate,
  useLocation: () => ({ pathname: '/app/projects/7' }),
  useSearchParams: () => [new URLSearchParams(), mockSetSearchParams],
}))

// Mock DnD kit with all needed exports
vi.mock('@dnd-kit/core', () => ({
  DndContext: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DragOverlay: () => null,
  useSensor: vi.fn(),
  useSensors: () => [],
  useDroppable: () => ({ setNodeRef: vi.fn(), isOver: false }),
  useDraggable: () => ({ attributes: {}, listeners: {}, setNodeRef: vi.fn(), transform: null, isDragging: false }),
  PointerSensor: vi.fn(),
}))

// Mock useLocalTasks hook
const hookState = vi.hoisted(() => ({
  createTask: vi.fn(),
  updateTask: vi.fn(),
  tasks: [] as Array<{
    id: number
    project_id: number
    title: string
    status: string
    swim_lane_id: number
    created_at: string
    updated_at: string
  }>,
  loading: false,
  error: null as string | null,
}))

vi.mock('../hooks/useLocalTasks', () => ({
  useLocalTasks: () => hookState,
}))

const apiMocks = vi.hoisted(() => ({
  getProject: vi.fn(),
  getSwimLanes: vi.fn(),
  getSprints: vi.fn(),
  getTags: vi.fn(),
}))

vi.mock('../lib/api', () => ({
  api: apiMocks,
}))

const project = {
  id: 7,
  name: 'Test Project',
  description: 'A project for testing',
  owner_id: 1,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

const swimLanes = [
  { id: 1, project_id: 7, name: 'To Do', color: '#6B7280', position: 0, status_category: 'todo' as const, created_at: '', updated_at: '' },
  { id: 2, project_id: 7, name: 'In Progress', color: '#3B82F6', position: 1, status_category: 'in_progress' as const, created_at: '', updated_at: '' },
  { id: 3, project_id: 7, name: 'Done', color: '#10B981', position: 2, status_category: 'done' as const, created_at: '', updated_at: '' },
]

describe('ProjectDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.getProject.mockResolvedValue(project)
    apiMocks.getSwimLanes.mockResolvedValue(swimLanes)
    apiMocks.getSprints.mockResolvedValue([])
    apiMocks.getTags.mockResolvedValue([])
    hookState.tasks = [
      {
        id: 1,
        project_id: 7,
        title: 'Build login page',
        status: 'todo',
        swim_lane_id: 1,
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
      {
        id: 2,
        project_id: 7,
        title: 'Write tests',
        status: 'in_progress',
        swim_lane_id: 2,
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
    ]
    hookState.loading = false
    hookState.error = null
    hookState.createTask.mockReset()
    hookState.updateTask.mockReset()
  })

  it('renders project name and description', async () => {
    render(<ProjectDetail />)
    await waitFor(() => {
      expect(screen.getByText('Test Project')).toBeInTheDocument()
      expect(screen.getByText('A project for testing')).toBeInTheDocument()
    })
  })

  it('renders swim lane columns', async () => {
    render(<ProjectDetail />)
    await waitFor(() => {
      // Swim lane headings include task count, may match multiple elements
      expect(screen.getAllByText(/To Do/).length).toBeGreaterThanOrEqual(1)
      expect(screen.getAllByText(/In Progress/).length).toBeGreaterThanOrEqual(1)
      expect(screen.getAllByText(/Done/).length).toBeGreaterThanOrEqual(1)
    })
  })

  it('renders tasks in their swim lanes', async () => {
    render(<ProjectDetail />)
    await waitFor(() => {
      expect(screen.getAllByText('Build login page').length).toBeGreaterThanOrEqual(1)
      expect(screen.getAllByText('Write tests').length).toBeGreaterThanOrEqual(1)
    })
  })

  it('shows error state on project load failure', async () => {
    apiMocks.getProject.mockRejectedValue(new Error('Not found'))
    hookState.error = 'Not found'
    render(<ProjectDetail />)
    await waitFor(() => {
      expect(screen.getByText('Not found')).toBeInTheDocument()
    })
  })

  it('opens new task modal on New Task click', async () => {
    const user = userEvent.setup()
    render(<ProjectDetail />)
    await waitFor(() => {
      expect(screen.getByText('Test Project')).toBeInTheDocument()
    })

    await user.click(screen.getByText('New Task'))
    expect(screen.getByPlaceholderText('Enter task title')).toBeInTheDocument()
  })

  it('creates a new task', async () => {
    hookState.createTask.mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<ProjectDetail />)
    await waitFor(() => {
      expect(screen.getByText('Test Project')).toBeInTheDocument()
    })

    await user.click(screen.getByText('New Task'))
    const titleInput = screen.getByPlaceholderText('Enter task title')
    await user.type(titleInput, 'New feature')
    await user.click(screen.getByText('Create Task'))

    await waitFor(() => {
      expect(hookState.createTask).toHaveBeenCalledWith(
        expect.objectContaining({ title: 'New feature', status: 'todo' })
      )
    })
  })

  it('does not create task with empty title', async () => {
    const user = userEvent.setup()
    render(<ProjectDetail />)
    await waitFor(() => {
      expect(screen.getByText('Test Project')).toBeInTheDocument()
    })

    await user.click(screen.getByText('New Task'))
    // Create Task button should be disabled when title is empty
    const createBtn = screen.getByText('Create Task')
    expect(createBtn).toBeDisabled()
  })

  it('switches to settings tab on settings click', async () => {
    const user = userEvent.setup()
    render(<ProjectDetail />)
    await waitFor(() => {
      expect(screen.getByText('Test Project')).toBeInTheDocument()
    })

    const settingsButton = screen.getByTitle('Settings')
    await user.click(settingsButton)
    expect(mockSetSearchParams).toHaveBeenCalledWith({ tab: 'settings' })
  })

  it('falls back to default swim lanes when API fails', async () => {
    apiMocks.getSwimLanes.mockRejectedValue(new Error('Network error'))
    render(<ProjectDetail />)
    await waitFor(() => {
      // Default swim lanes should still render
      expect(screen.getAllByText(/To Do/).length).toBeGreaterThanOrEqual(1)
      expect(screen.getAllByText(/In Progress/).length).toBeGreaterThanOrEqual(1)
      expect(screen.getAllByText(/Done/).length).toBeGreaterThanOrEqual(1)
    })
  })
})
