import { useEffect, useState, useMemo, useCallback, lazy, Suspense } from 'react'
import { useParams, useNavigate, useLocation, useSearchParams, Link } from 'react-router-dom'
import { DndContext, DragEndEvent, DragOverlay, DragStartEvent, PointerSensor, useSensor, useSensors } from '@dnd-kit/core'
import { api, Project, Task, type SwimLane, type Sprint, type Tag } from '../lib/api'
import { useLocalTasks } from '../hooks/useLocalTasks'
import { useSync } from '../state/SyncContext'
import { REACTION_EMOJI, REACTION_ORDER } from '../lib/reactionUtils'
import BoardFilterBar, { applyBoardFilters } from '../components/board/BoardFilterBar'
import Select from '../components/ui/Select'
import { useDialog } from '../state/DialogContext'

const WikiContent = lazy(() => import('../components/WikiContent'))
const ProjectSettings = lazy(() => import('./ProjectSettings'))
const Roadmap = lazy(() => import('./Roadmap'))


export default function ProjectDetail() {
  const dialog = useDialog()
  const { projectId } = useParams<{ projectId: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const activeTab = searchParams.get('tab') || 'board'
  const { registerSyncTask } = useSync()
  const [project, setProject] = useState<Project | null>(null)
  const [loadingProject, setLoadingProject] = useState(true)
  const [projectError, setProjectError] = useState<string | null>(null)
  const [swimLanes, setSwimLanes] = useState<SwimLane[]>([])
  const [loadingSwimLanes, setLoadingSwimLanes] = useState(true)
  const [sprints, setSprints] = useState<Sprint[]>([])
  const [tags, setTags] = useState<Tag[]>([])

  // Board filters (persisted to localStorage per project)
  const [filterSprint, setFilterSprint] = useState<number | null>(null)
  const [filterAssignee, setFilterAssignee] = useState<number | null>(null)
  const [filterPriority, setFilterPriority] = useState('')
  const [filterTag, setFilterTag] = useState<number | null>(null)
  const [filterTaskIds, setFilterTaskIds] = useState<number[]>([])

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

  // Mobile: selected swim lane (tab picker)
  const [mobileLane, setMobileLane] = useState<number | null>(null)

  // Drag and drop state
  const [activeTask, setActiveTask] = useState<Task | null>(null)

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    })
  )

  // Default mobile lane to first swim lane once loaded
  useEffect(() => {
    if (swimLanes.length > 0 && mobileLane === null) {
      setMobileLane(swimLanes[0].id)
    }
  }, [swimLanes, mobileLane])

  // Restore filters from localStorage when projectId changes; track last visited project
  useEffect(() => {
    if (!projectId) return
    localStorage.setItem('taskai_last_project', projectId)
    try {
      const raw = localStorage.getItem(`taskai_filters_${projectId}`)
      if (raw) {
        const s = JSON.parse(raw)
        setFilterSprint(s.sprint ?? null)
        setFilterAssignee(s.assignee ?? null)
        setFilterPriority(s.priority ?? '')
        setFilterTag(s.tag ?? null)
        setFilterTaskIds(Array.isArray(s.taskIds) ? s.taskIds : [])
      } else {
        setFilterSprint(null)
        setFilterAssignee(null)
        setFilterPriority('')
        setFilterTag(null)
        setFilterTaskIds([])
      }
    } catch { /* ignore */ }
  }, [projectId])

  // Persist filters to localStorage when they change
  useEffect(() => {
    if (!projectId) return
    localStorage.setItem(`taskai_filters_${projectId}`, JSON.stringify({
      sprint: filterSprint,
      assignee: filterAssignee,
      priority: filterPriority,
      tag: filterTag,
      taskIds: filterTaskIds,
    }))
  }, [projectId, filterSprint, filterAssignee, filterPriority, filterTag, filterTaskIds])

  // Keyboard shortcuts for project board
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement
      const isInput = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT' || target.isContentEditable

      // Cmd/Ctrl+N to create new task
      if ((e.metaKey || e.ctrlKey) && e.key === 'n') {
        e.preventDefault()
        setShowNewTaskModal(true)
        return
      }

      if (isInput) return

      // '/' to focus search
      if (e.key === '/') {
        e.preventDefault()
        const searchInput = document.querySelector<HTMLInputElement>('input[placeholder*="Search"]')
        searchInput?.focus()
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [])

  const loadProject = useCallback(async (silent = false) => {
    if (!projectId) return
    try {
      if (!silent) setLoadingProject(true)
      setProjectError(null)
      const projectData = await api.getProject(Number(projectId))
      setProject(projectData)
    } catch (err) {
      if (silent) throw err
      setProjectError(err instanceof Error ? err.message : 'Failed to load project')
    } finally {
      if (!silent) setLoadingProject(false)
    }
  }, [projectId])

  const loadSwimLanes = useCallback(async (silent = false) => {
    if (!projectId) return
    try {
      if (!silent) setLoadingSwimLanes(true)
      const lanes = await api.getSwimLanes(Number(projectId))
      setSwimLanes(lanes.sort((a, b) => a.position - b.position))
    } catch (err) {
      if (silent) throw err
      // Fallback to default swim lanes if fetch fails
      setSwimLanes([
        { id: 0, project_id: Number(projectId), name: 'To Do', color: '#6B7280', position: 0, status_category: 'todo', created_at: '', updated_at: '' },
        { id: 1, project_id: Number(projectId), name: 'In Progress', color: '#3B82F6', position: 1, status_category: 'in_progress', created_at: '', updated_at: '' },
        { id: 2, project_id: Number(projectId), name: 'Done', color: '#10B981', position: 2, status_category: 'done', created_at: '', updated_at: '' },
      ])
    } finally {
      if (!silent) setLoadingSwimLanes(false)
    }
  }, [projectId])

  const refreshProjectData = useCallback(async (silent = false) => {
    if (!projectId) return
    const projectNumber = Number(projectId)
    const [sprintData, tagData] = await Promise.all([
      api.getSprints(projectNumber).catch(() => [] as Sprint[]),
      api.getTags(projectNumber).catch(() => [] as Tag[]),
      loadProject(silent),
      loadSwimLanes(silent),
    ])
    setSprints(sprintData)
    setTags(tagData)
  }, [loadProject, loadSwimLanes, projectId])

  // Load project metadata, swim lanes, sprints and tags
  useEffect(() => {
    if (!projectId) return
    setMobileLane(null)
    void refreshProjectData()
  }, [projectId, refreshProjectData])

  useEffect(() => {
    if (!projectId) return
    return registerSyncTask(`project:${projectId}:metadata`, () => refreshProjectData(true))
  }, [projectId, refreshProjectData, registerSyncTask])

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
      dialog.notify(err instanceof Error ? err.message : 'Failed to create task', 'error')
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
      dialog.notify(err instanceof Error ? err.message : 'Failed to update task status', 'error')
    }
  }

  // Derive unique assignees from loaded tasks (must be before early returns)
  const uniqueAssignees = useMemo(() => {
    const map = new Map<number, string>()
    tasks.forEach(t => {
      if (t.assignees?.length) {
        t.assignees.forEach(a => {
          if (a.user_id != null && !map.has(a.user_id)) map.set(a.user_id, a.user_name ?? `User ${a.user_id}`)
        })
      } else if (t.assignee_id && !map.has(t.assignee_id)) {
        map.set(t.assignee_id, t.assignee_name || `User ${t.assignee_id}`)
      }
    })
    return Array.from(map.entries()).map(([id, name]) => ({ id, name }))
  }, [tasks])

  // Apply board filters (must be before early returns)
  const filteredTasks = useMemo(
    () => applyBoardFilters(tasks, {
      sprintId: filterSprint,
      assigneeId: filterAssignee,
      priority: filterPriority,
      tagId: filterTag,
      taskIds: filterTaskIds,
    }),
    [tasks, filterSprint, filterAssignee, filterPriority, filterTag, filterTaskIds]
  )

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

  // Group FILTERED tasks by swim lane
  const tasksBySwimLane = swimLanes.reduce((acc, lane) => {
    acc[lane.id] = filteredTasks.filter((t) => t.swim_lane_id === lane.id)
    return acc
  }, {} as Record<number, Task[]>)

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
            <div className="flex items-center gap-2 flex-shrink-0">
              <button
                onClick={() => setSearchParams({ tab: 'settings' })}
                title="Settings"
                className={`p-2 rounded-lg transition-colors ${activeTab === 'settings' ? 'text-primary-400 bg-dark-bg-tertiary' : 'text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary'}`}
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                </svg>
              </button>
              <button
                onClick={() => setShowNewTaskModal(true)}
                className="inline-flex items-center gap-2 px-4 py-2 bg-primary-500 hover:bg-primary-600 text-white text-sm font-medium rounded-lg transition-colors shadow-sm"
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                </svg>
                New Task
              </button>
            </div>
          </div>

          {/* Navigation tabs and stats */}
          <div className="px-6 flex items-end justify-between border-t border-dark-border-subtle/50">
            <div className="flex items-center gap-1">
              {/* Board icon tab */}
              <button
                onClick={() => setSearchParams({})}
                title="Board"
                className={`relative p-3 transition-colors ${activeTab === 'board' ? 'text-primary-400' : 'text-dark-text-secondary hover:text-dark-text-primary'}`}
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 17V7m0 10a2 2 0 01-2 2H5a2 2 0 01-2-2V7a2 2 0 012-2h2a2 2 0 012 2m0 10a2 2 0 002 2h2a2 2 0 002-2M9 7a2 2 0 012-2h2a2 2 0 012 2m0 10V7m0 10a2 2 0 002 2h2a2 2 0 002-2V7a2 2 0 00-2-2h-2a2 2 0 00-2 2" />
                </svg>
                {activeTab === 'board' && <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-primary-500" />}
              </button>
              {/* Wiki icon tab */}
              <button
                onClick={() => setSearchParams({ tab: 'wiki' })}
                title="Wiki"
                className={`relative p-3 transition-colors ${activeTab === 'wiki' ? 'text-primary-400' : 'text-dark-text-secondary hover:text-dark-text-primary'}`}
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
                </svg>
                {activeTab === 'wiki' && <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-primary-500" />}
              </button>
              {/* Roadmap icon tab */}
              <button
                onClick={() => setSearchParams({ tab: 'roadmap' })}
                title="Roadmap"
                className={`relative p-3 transition-colors ${activeTab === 'roadmap' ? 'text-primary-400' : 'text-dark-text-secondary hover:text-dark-text-primary'}`}
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
                </svg>
                {activeTab === 'roadmap' && <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-primary-500" />}
              </button>
            </div>

            {/* Filter bar + Task Stats (visible on board, invisible but space-reserving on other tabs) */}
            <div className={`flex items-center gap-4 py-3 ${activeTab === 'board' ? '' : 'invisible'}`}>
              <BoardFilterBar
                sprints={sprints}
                assignees={uniqueAssignees}
                tags={tags}
                sprintId={filterSprint}
                assigneeId={filterAssignee}
                priority={filterPriority}
                tagId={filterTag}
                taskIds={filterTaskIds}
                onChange={patch => {
                  if ('sprintId'   in patch) setFilterSprint(patch.sprintId ?? null)
                  if ('assigneeId' in patch) setFilterAssignee(patch.assigneeId ?? null)
                  if ('priority'   in patch) setFilterPriority(patch.priority ?? '')
                  if ('tagId'      in patch) setFilterTag(patch.tagId ?? null)
                  if ('taskIds'    in patch) setFilterTaskIds(patch.taskIds ?? [])
                }}
              />
              {/* Lane stats — hidden on mobile (shown in lane tab bar instead) */}
              <div className="hidden md:flex items-center gap-4">
                <div className="w-px h-4 bg-dark-border-subtle" />
                {swimLanes.map((lane) => (
                  <div key={lane.id} className="flex items-center gap-2">
                    <div className="w-2.5 h-2.5 rounded-full" style={{ backgroundColor: lane.color }} />
                    <span className="text-xs font-medium text-dark-text-secondary">
                      <span className="text-dark-text-primary">{tasksBySwimLane[lane.id]?.length || 0}</span> {lane.name}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>

        {/* Tab Content */}
        {activeTab === 'board' && (
          <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
            {/* Tasks Board */}
            <div className="flex-1 overflow-y-auto overflow-x-hidden p-4 md:p-6 bg-dark-bg-base">
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
                <>
                  {/* Mobile: swim lane dropdown */}
                  {mobileLane !== null && (
                    <div className="md:hidden mb-3 relative">
                      <div className="flex items-center gap-2 px-3 py-2.5 bg-dark-bg-elevated border border-dark-border-medium rounded-lg shadow-linear-sm">
                        <span
                          className="w-2.5 h-2.5 rounded-full flex-shrink-0"
                          style={{ backgroundColor: swimLanes.find(l => l.id === mobileLane)?.color }}
                        />
                        <Select
                          aria-label="Select swim lane"
                          variant="ghost"
                          className="flex-1"
                          buttonClassName="px-0 py-0 font-medium"
                          value={mobileLane}
                          onChange={setMobileLane}
                          options={swimLanes.map(lane => ({
                            value: lane.id,
                            label: `${lane.name} (${tasksBySwimLane[lane.id]?.length || 0})`,
                          }))}
                        />
                      </div>
                    </div>
                  )}

                  {/* Mobile: single lane view */}
                  {mobileLane !== null && (
                    <div className="md:hidden">
                      {swimLanes
                        .filter(lane => lane.id === mobileLane)
                        .map(lane => (
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

                  {/* Desktop: all lanes as grid */}
                  <div className="hidden md:grid gap-4" style={{ gridTemplateColumns: `repeat(${swimLanes.length}, minmax(0, 1fr))` }}>
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
                </>
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

          </DndContext>
        )}

        {activeTab === 'wiki' && (
          <Suspense fallback={<div className="flex-1 flex items-center justify-center"><div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500" /></div>}>
            <WikiContent projectId={projectId!} />
          </Suspense>
        )}

        {activeTab === 'roadmap' && (
          <Suspense fallback={<div className="flex-1 flex items-center justify-center"><div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500" /></div>}>
            <Roadmap projectId={Number(projectId)} tasks={tasks} />
          </Suspense>
        )}

        {activeTab === 'settings' && (
          <Suspense fallback={<div className="flex-1 flex items-center justify-center"><div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500" /></div>}>
            <ProjectSettings embedded projectIdOverride={Number(projectId)} />
          </Suspense>
        )}

        {/* New Task Modal (available on all tabs) */}
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
      {(() => {
        const assignees: { id: number; name: string }[] = task.assignees?.length
          ? task.assignees.filter(a => a.user_id != null).map(a => ({ id: a.user_id as number, name: a.user_name ?? `User ${a.user_id}` }))
          : task.assignee_id
            ? [{ id: task.assignee_id, name: task.assignee_name ?? `User ${task.assignee_id}` }]
            : []
        if (!assignees.length) return null
        return (
          <div className="flex items-center gap-1.5 text-xs text-dark-text-tertiary mt-2">
            <div className="w-4 h-4 rounded-full bg-primary-500/10 flex items-center justify-center flex-shrink-0">
              <svg className="w-2.5 h-2.5 text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
              </svg>
            </div>
            <span>
              {assignees.map((a, i) => (
                <span key={a.id}>
                  {i > 0 && ', '}
                  <Link
                    to={`/app/users/${a.id}`}
                    className="hover:text-primary-400 transition-colors"
                    onClick={(e) => e.stopPropagation()}
                  >
                    {a.name}
                  </Link>
                </span>
              ))}
            </span>
          </div>
        )
      })()}
      {task.github_reactions && task.github_reactions.length > 0 && (
        <div className="flex flex-wrap gap-1 mt-1.5">
          {REACTION_ORDER
            .filter(r => (task.github_reactions ?? []).find(gr => gr.reaction === r && gr.count > 0))
            .map(r => {
              const gr = task.github_reactions!.find(g => g.reaction === r)!
              return (
                <span key={r}
                  className="inline-flex items-center gap-0.5 text-xs bg-dark-bg-secondary border border-dark-border-subtle rounded-full px-1.5 py-0.5 text-dark-text-tertiary">
                  {REACTION_EMOJI[r]} {gr.count}
                </span>
              )
            })}
        </div>
      )}
    </div>
  )
}
