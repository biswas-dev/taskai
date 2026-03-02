import { useEffect, useState } from 'react'
import { useParams, useNavigate, useLocation } from 'react-router-dom'
import { DndContext, DragEndEvent, DragOverlay, DragStartEvent, PointerSensor, useSensor, useSensors } from '@dnd-kit/core'
import { api, Project, Task, type SwimLane } from '../lib/api'
import { useLocalTasks } from '../hooks/useLocalTasks'

export default function ProjectDetail() {
  const { projectId } = useParams<{ projectId: string }>()
  const navigate = useNavigate()
  const [project, setProject] = useState<Project | null>(null)
  const [loadingProject, setLoadingProject] = useState(true)
  const [projectError, setProjectError] = useState<string | null>(null)
  const [swimLanes, setSwimLanes] = useState<SwimLane[]>([])
  const [loadingSwimLanes, setLoadingSwimLanes] = useState(true)

  // Use local-first tasks hook
  const {
    tasks,
    loading: loadingTasks,
    error: tasksError,
    createTask,
    updateTask,
  } = useLocalTasks(Number(projectId))

  // New task modal state
  const [showNewTaskModal, setShowNewTaskModal] = useState(false)
  const [newTaskTitle, setNewTaskTitle] = useState('')
  const [newTaskDescription, setNewTaskDescription] = useState('')
  const [newTaskDueDate, setNewTaskDueDate] = useState('')
  const [creating, setCreating] = useState(false)

  // Drag and drop state
  const [activeTask, setActiveTask] = useState<Task | null>(null)

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    })
  )

  // Load project metadata and swim lanes (tasks are handled by useLocalTasks hook)
  useEffect(() => {
    if (projectId) {
      loadProject()
      loadSwimLanes()
    }
  }, [projectId])

  const loadProject = async () => {
    try {
      setLoadingProject(true)
      setProjectError(null)
      const projectData = await api.getProject(Number(projectId))
      setProject(projectData)
    } catch (err) {
      setProjectError(err instanceof Error ? err.message : 'Failed to load project')
    } finally {
      setLoadingProject(false)
    }
  }

  const loadSwimLanes = async () => {
    try {
      setLoadingSwimLanes(true)
      const lanes = await api.getSwimLanes(Number(projectId))
      setSwimLanes(lanes.sort((a, b) => a.position - b.position))
    } catch (err) {
      // non-critical load failure — use defaults below
      // Fallback to default swim lanes if fetch fails
      setSwimLanes([
        { id: 0, project_id: Number(projectId), name: 'To Do', color: '#6B7280', position: 0, status_category: 'todo', created_at: '', updated_at: '' },
        { id: 1, project_id: Number(projectId), name: 'In Progress', color: '#3B82F6', position: 1, status_category: 'in_progress', created_at: '', updated_at: '' },
        { id: 2, project_id: Number(projectId), name: 'Done', color: '#10B981', position: 2, status_category: 'done', created_at: '', updated_at: '' },
      ])
    } finally {
      setLoadingSwimLanes(false)
    }
  }

  const handleCreateTask = async () => {
    if (!newTaskTitle.trim() || !projectId) return

    try {
      setCreating(true)
      // Optimistic create - updates UI instantly and syncs in background
      await createTask({
        title: newTaskTitle.trim(),
        description: newTaskDescription.trim() || undefined,
        status: 'todo',
        swim_lane_id: swimLanes.length > 0 ? swimLanes[0].id : undefined,
        due_date: newTaskDueDate || undefined,
      })
      setShowNewTaskModal(false)
      setNewTaskTitle('')
      setNewTaskDescription('')
      setNewTaskDueDate('')
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to create task')
    } finally {
      setCreating(false)
    }
  }

  const handleDragStart = (event: DragStartEvent) => {
    const task = tasks.find(t => t.id === event.active.id)
    setActiveTask(task || null)
  }

  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event
    setActiveTask(null)

    if (!over) return

    const taskId = active.id as number
    const newSwimLaneId = Number(over.id) // Convert string ID to number

    const task = tasks.find(t => t.id === taskId)
    if (!task || task.swim_lane_id === newSwimLaneId) return

    // Find the swim lane to get the status mapping
    const swimLane = swimLanes.find(l => l.id === newSwimLaneId)
    if (!swimLane) return

    try {
      // Backend auto-syncs status from swim lane's status_category
      await updateTask(taskId, {
        swim_lane_id: newSwimLaneId,
      })
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to update task status')
    }
  }

  if (loadingProject || loadingTasks || loadingSwimLanes) {
    return (
      <div className="p-6 bg-dark-bg-base">
        <div className="animate-pulse space-y-3">
          <div className="h-6 bg-dark-bg-tertiary rounded w-1/3"></div>
          <div className="h-3 bg-dark-bg-secondary rounded w-1/2"></div>
          <div className="space-y-2 mt-6">
            <div className="h-16 bg-dark-bg-secondary rounded"></div>
            <div className="h-16 bg-dark-bg-secondary rounded"></div>
            <div className="h-16 bg-dark-bg-secondary rounded"></div>
          </div>
        </div>
      </div>
    )
  }

  if (projectError || tasksError) {
    return (
      <div className="p-6 bg-dark-bg-base">
        <div className="bg-danger-500/10 border border-danger-500/20 text-danger-400 px-4 py-3 rounded text-sm">
          {projectError || tasksError}
        </div>
      </div>
    )
  }

  // Group tasks by swim lane
  const tasksBySwimLane = swimLanes.reduce((acc, lane) => {
    acc[lane.id] = tasks.filter((t) => t.swim_lane_id === lane.id)
    return acc
  }, {} as Record<number, Task[]>)

  return (
    <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
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
            <button
              onClick={() => setShowNewTaskModal(true)}
              className="flex-shrink-0 inline-flex items-center gap-2 px-4 py-2 bg-primary-500 hover:bg-primary-600 text-white text-sm font-medium rounded-lg transition-colors shadow-sm"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
              </svg>
              New Task
            </button>
          </div>

          {/* Navigation tabs and stats */}
          <div className="px-6 flex items-end justify-between border-t border-dark-border-subtle/50">
            <div className="flex items-center gap-1">
              <button
                onClick={() => navigate(`/app/projects/${projectId}`)}
                className="relative px-4 py-3 text-sm font-medium text-primary-400 transition-colors"
              >
                Board
                <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-primary-500"></div>
              </button>
              <button
                onClick={() => navigate(`/app/projects/${projectId}/wiki`)}
                className="px-4 py-3 text-sm font-medium text-dark-text-secondary hover:text-dark-text-primary transition-colors"
              >
                Wiki
              </button>
              <button
                onClick={() => navigate(`/app/projects/${projectId}/settings`)}
                className="px-4 py-3 text-sm font-medium text-dark-text-secondary hover:text-dark-text-primary transition-colors"
              >
                Settings
              </button>
            </div>

            {/* Task Stats */}
            <div className="flex items-center gap-4 py-3">
              {swimLanes.map((lane) => (
                <div key={lane.id} className="flex items-center gap-2">
                  <div
                    className="w-2.5 h-2.5 rounded-full"
                    style={{ backgroundColor: lane.color }}
                  ></div>
                  <span className="text-xs font-medium text-dark-text-secondary">
                    <span className="text-dark-text-primary">{tasksBySwimLane[lane.id]?.length || 0}</span> {lane.name}
                  </span>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Tasks Board */}
        <div className="flex-1 overflow-y-auto overflow-x-auto p-4 md:p-6 bg-dark-bg-base">
          {tasks.length === 0 ? (
            <div className="flex items-center justify-center h-64">
              <div className="text-center">
                <svg
                  className="mx-auto h-10 w-10 text-dark-text-tertiary"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"
                  />
                </svg>
                <h3 className="mt-2 text-sm font-medium text-dark-text-primary">No tasks</h3>
                <p className="mt-1 text-xs text-dark-text-secondary">
                  Get started by creating a new task.
                </p>
              </div>
            </div>
          ) : (
            <div className="flex gap-4 overflow-x-auto pb-2 md:grid md:overflow-x-visible" style={{ gridTemplateColumns: `repeat(${swimLanes.length}, minmax(0, 1fr))` }}>
              {swimLanes.map((lane) => (
                <TaskColumn
                  key={lane.id}
                  id={lane.id.toString()}
                  title={lane.name}
                  count={tasksBySwimLane[lane.id]?.length || 0}
                  tasks={tasksBySwimLane[lane.id] || []}
                  color={lane.color}
                  projectId={projectId || ''}
                />
              ))}
            </div>
          )}
        </div>

        {/* Drag Overlay */}
        <DragOverlay>
          {activeTask ? (
            <TaskCard
              task={activeTask}
              projectId={projectId || ''}
              isDragging
            />
          ) : null}
        </DragOverlay>

        {/* New Task Modal */}
        {showNewTaskModal && (
          <div className="fixed inset-0 bg-black/70 flex items-center justify-center p-4 z-50">
            <div className="bg-dark-bg-elevated rounded-xl shadow-linear-xl max-w-2xl w-full p-6 border border-dark-border-subtle max-h-[90vh] overflow-y-auto">
              <h2 className="text-lg font-semibold text-dark-text-primary mb-5">Create New Task</h2>

              <div className="space-y-4">
                <div>
                  <label htmlFor="task-title" className="block text-sm font-medium text-dark-text-secondary mb-2">
                    Title *
                  </label>
                  <input
                    id="task-title"
                    type="text"
                    value={newTaskTitle}
                    onChange={(e) => setNewTaskTitle(e.target.value)}
                    className="w-full px-3 py-2 text-sm bg-dark-bg-secondary border border-dark-border-subtle text-dark-text-primary rounded-md focus:outline-none focus:ring-1 focus:ring-primary-500 focus:border-primary-500"
                    placeholder="Enter task title"
                    autoFocus
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && newTaskTitle.trim()) {
                        handleCreateTask()
                      }
                    }}
                  />
                </div>

                <div>
                  <label htmlFor="task-description" className="block text-sm font-medium text-dark-text-secondary mb-2">
                    Description
                  </label>
                  <textarea
                    id="task-description"
                    value={newTaskDescription}
                    onChange={(e) => setNewTaskDescription(e.target.value)}
                    rows={12}
                    className="w-full px-3 py-2 text-sm bg-dark-bg-secondary border border-dark-border-subtle text-dark-text-primary rounded-md focus:outline-none focus:ring-1 focus:ring-primary-500 focus:border-primary-500 resize-y"
                    placeholder="Enter task description (optional)"
                  />
                </div>

                <div>
                  <label htmlFor="task-due-date" className="block text-xs font-medium text-dark-text-secondary mb-1">
                    Due Date
                  </label>
                  <input
                    id="task-due-date"
                    type="date"
                    value={newTaskDueDate}
                    onChange={(e) => setNewTaskDueDate(e.target.value)}
                    className="w-full px-3 py-2 text-sm bg-dark-bg-secondary border border-dark-border-subtle text-dark-text-primary rounded-md focus:outline-none focus:ring-1 focus:ring-primary-500 focus:border-primary-500"
                  />
                </div>
              </div>

              <div className="flex gap-3 mt-6">
                <button
                  onClick={() => {
                    setShowNewTaskModal(false)
                    setNewTaskTitle('')
                    setNewTaskDescription('')
                    setNewTaskDueDate('')
                  }}
                  className="flex-1 px-4 py-2 text-sm border border-dark-border-subtle text-dark-text-secondary rounded-md hover:bg-dark-bg-secondary transition-colors duration-150"
                  disabled={creating}
                >
                  Cancel
                </button>
                <button
                  onClick={handleCreateTask}
                  disabled={!newTaskTitle.trim() || creating}
                  className="flex-1 px-4 py-2 text-sm bg-primary-500 text-white rounded-md hover:bg-primary-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors duration-150"
                >
                  {creating ? 'Creating...' : 'Create Task'}
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </DndContext>
  )
}

// Helper components
import { useDroppable } from '@dnd-kit/core'
import { useDraggable } from '@dnd-kit/core'

function TaskColumn({ id, title, count, tasks, color, projectId }: {
  id: string
  title: string
  count: number
  tasks: Task[]
  color: string
  projectId: string
}) {
  const { setNodeRef, isOver } = useDroppable({ id })

  return (
    <div ref={setNodeRef} className={`min-h-[200px] min-w-[280px] flex-shrink-0 md:min-w-0 md:flex-shrink ${isOver ? 'bg-dark-bg-tertiary/20 ring-1 ring-primary-500/30 rounded-md' : ''}`}>
      <h3 className="text-[11px] uppercase tracking-wide font-semibold text-dark-text-quaternary mb-3 flex items-center gap-2">
        <div
          className="w-1.5 h-1.5 rounded-full"
          style={{ backgroundColor: color }}
        ></div>
        {title} ({count})
      </h3>
      <div className="space-y-2">
        {tasks.map((task) => (
          <DraggableTask
            key={task.id}
            task={task}
            projectId={projectId || ''}
          />
        ))}
      </div>
    </div>
  )
}

function DraggableTask({ task, projectId }: {
  task: Task
  projectId: string
}) {
  const navigate = useNavigate()
  const location = useLocation()
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: task.id as number,
  })

  const style = transform ? {
    transform: `translate3d(${transform.x}px, ${transform.y}px, 0)`,
    opacity: isDragging ? 0.5 : 1,
  } : undefined

  const handleClick = () => {
    const taskIdentifier = task.task_number || task.id
    navigate(`/app/projects/${projectId}/tasks/${taskIdentifier}`, {
      state: { backgroundLocation: location },
    })
  }

  return (
    <div
      ref={setNodeRef}
      style={style}
      {...listeners}
      {...attributes}
      onClick={handleClick}
    >
      <TaskCard
        task={task}
        projectId={projectId || ''}
        isDragging={isDragging}
      />
    </div>
  )
}

function TaskCard({ task, isDragging }: {
  task: Task
  projectId?: string
  isDragging?: boolean
}) {
  return (
    <div
      className={`bg-dark-bg-primary border border-dark-border-subtle rounded-lg p-3 hover:border-dark-border-medium hover:shadow-linear-sm transition-all duration-150 cursor-pointer ${
        isDragging ? 'shadow-linear-lg rotate-1' : ''
      } ${task.status === 'done' ? 'opacity-60' : ''}`}
    >
      <div className="flex items-center gap-2">
        {task.task_number && <span className="text-xs font-mono text-dark-text-tertiary">#{task.task_number}</span>}
        <h4 className="text-sm font-medium text-dark-text-primary hover:text-primary-400 transition-colors">{task.title}</h4>
      </div>
      {task.assignee_id && (
        <div className="flex items-center gap-1.5 text-xs text-dark-text-tertiary mt-2">
          <div className="w-4 h-4 rounded-full bg-primary-500/10 flex items-center justify-center">
            <svg className="w-2.5 h-2.5 text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
            </svg>
          </div>
          <span>{task.assignee_name || `User ${task.assignee_id}`}</span>
        </div>
      )}
    </div>
  )
}
