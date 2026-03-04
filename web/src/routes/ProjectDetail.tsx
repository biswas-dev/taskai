import { useEffect, useState, useRef, useMemo } from 'react'
import { useParams, useNavigate, useLocation } from 'react-router-dom'
import { DndContext, DragEndEvent, DragOverlay, DragStartEvent, PointerSensor, useSensor, useSensors } from '@dnd-kit/core'
import { api, Project, Task, type SwimLane, type Sprint, type Tag } from '../lib/api'
import { useLocalTasks } from '../hooks/useLocalTasks'

// ── Board filter bar (GitHub-style) ──────────────────────────────────────────
type BoardFilterKey = 'sprint' | 'assignee' | 'priority' | 'label'

interface BoardFilterBarProps {
  sprints: Sprint[]
  assignees: { id: number; name: string }[]
  tags: Tag[]
  sprintId: number | null
  assigneeId: number | null
  priority: string
  tagId: number | null
  onChange: (patch: {
    sprintId?: number | null
    assigneeId?: number | null
    priority?: string
    tagId?: number | null
  }) => void
}

const PRIORITY_OPTIONS = [
  { value: 'urgent', label: 'Urgent', color: '#ef4444' },
  { value: 'high',   label: 'High',   color: '#f97316' },
  { value: 'medium', label: 'Medium', color: '#eab308' },
  { value: 'low',    label: 'Low',    color: '#6b7280' },
]

function BoardFilterBar({ sprints, assignees, tags, sprintId, assigneeId, priority, tagId, onChange }: BoardFilterBarProps) {
  const [open, setOpen] = useState(false)
  const [category, setCategory] = useState<BoardFilterKey | null>(null)
  const [search, setSearch] = useState('')
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const h = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false); setCategory(null); setSearch('')
      }
    }
    document.addEventListener('mousedown', h)
    return () => document.removeEventListener('mousedown', h)
  }, [])

  const CATEGORIES: { id: BoardFilterKey; label: string; icon: React.ReactNode }[] = [
    { id: 'sprint',   label: 'Sprint',   icon: <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" /></svg> },
    { id: 'assignee', label: 'Assignee', icon: <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg> },
    { id: 'priority', label: 'Priority', icon: <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M3 4h13M3 8h9m-9 4h6m4 0l4-4m0 0l4 4m-4-4v12" /></svg> },
    { id: 'label',    label: 'Label',    icon: <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A2 2 0 013 12V7a4 4 0 014-4z" /></svg> },
  ]

  // Active chips
  const chips: { key: BoardFilterKey; label: string }[] = []
  if (sprintId)   chips.push({ key: 'sprint',   label: `sprint:"${sprints.find(s => s.id === sprintId)?.name ?? sprintId}"` })
  if (assigneeId) chips.push({ key: 'assignee', label: `assignee:"${assignees.find(a => a.id === assigneeId)?.name ?? assigneeId}"` })
  if (priority)   chips.push({ key: 'priority', label: `priority:${priority}` })
  if (tagId)      chips.push({ key: 'label',    label: `label:"${tags.find(t => t.id === tagId)?.name ?? tagId}"` })

  const removeChip = (key: BoardFilterKey) => {
    if (key === 'sprint')   onChange({ sprintId: null })
    if (key === 'assignee') onChange({ assigneeId: null })
    if (key === 'priority') onChange({ priority: '' })
    if (key === 'label')    onChange({ tagId: null })
  }

  const q = search.toLowerCase()

  let options: { value: string; label: string; sub?: string; color?: string }[] = []
  if (category === 'sprint') {
    options = sprints
      .filter(s => !q || s.name.toLowerCase().includes(q))
      .map(s => ({ value: String(s.id), label: s.name, sub: s.status === 'completed' ? 'completed' : s.status === 'active' ? 'active' : undefined }))
  } else if (category === 'assignee') {
    options = [
      { value: 'none', label: 'No assignee' },
      ...assignees
        .filter(a => !q || a.name.toLowerCase().includes(q))
        .map(a => ({ value: String(a.id), label: a.name }))
    ]
  } else if (category === 'priority') {
    options = PRIORITY_OPTIONS.map(p => ({ value: p.value, label: p.label, color: p.color }))
  } else if (category === 'label') {
    options = tags
      .filter(t => !q || t.name.toLowerCase().includes(q))
      .map(t => ({ value: String(t.id), label: t.name, color: t.color }))
  }

  const activeVal = (key: BoardFilterKey) => {
    if (key === 'sprint')   return sprintId ? sprints.find(s => s.id === sprintId)?.name : undefined
    if (key === 'assignee') return assigneeId ? assignees.find(a => a.id === assigneeId)?.name : undefined
    if (key === 'priority') return priority || undefined
    if (key === 'label')    return tagId ? tags.find(t => t.id === tagId)?.name : undefined
  }

  const selectOption = (cat: BoardFilterKey, value: string) => {
    if (cat === 'sprint')   onChange({ sprintId: value && value !== 'none' ? Number(value) : null })
    if (cat === 'assignee') onChange({ assigneeId: value && value !== 'none' ? Number(value) : null })
    if (cat === 'priority') onChange({ priority: value })
    if (cat === 'label')    onChange({ tagId: value ? Number(value) : null })
    setOpen(false); setCategory(null); setSearch('')
  }

  const hasFilters = chips.length > 0

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => { setOpen(v => !v); setCategory(null); setSearch('') }}
        className={`flex items-center gap-2 px-3 py-1.5 rounded-lg border text-sm transition-colors focus:outline-none ${
          hasFilters
            ? 'bg-[#1f6feb]/10 border-[#1f6feb]/40 text-[#79c0ff]'
            : 'bg-transparent border-[#30363d] text-[#8b949e] hover:border-[#484f58] hover:text-[#c9d1d9]'
        }`}
      >
        <svg className="w-3.5 h-3.5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2a1 1 0 01-.293.707L13 13.414V19a1 1 0 01-.553.894l-4 2A1 1 0 017 21v-7.586L3.293 6.707A1 1 0 013 6V4z" />
        </svg>
        {hasFilters ? (
          <div className="flex items-center gap-1.5 flex-wrap">
            {chips.map(c => (
              <span key={c.key} className="inline-flex items-center gap-1 font-mono text-xs">
                {c.label}
                <span
                  role="button"
                  onClick={e => { e.stopPropagation(); removeChip(c.key) }}
                  className="text-[#8b949e] hover:text-white cursor-pointer"
                >×</span>
              </span>
            ))}
          </div>
        ) : (
          <span>Filter</span>
        )}
      </button>

      {open && (
        <div className="absolute top-full left-0 mt-1 w-64 bg-[#161b22] border border-[#30363d] rounded-lg shadow-2xl z-50 overflow-hidden text-sm">
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
                  <span className="text-[#8b949e] w-4 flex-shrink-0">{cat.icon}</span>
                  <span className="flex-1 text-left">{cat.label}</span>
                  {activeVal(cat.id) && <span className="text-xs text-[#79c0ff] font-mono truncate max-w-[80px]">{activeVal(cat.id)}</span>}
                  <svg className="w-3 h-3 text-[#8b949e] flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" /></svg>
                </button>
              ))}
              {hasFilters && (
                <div className="border-t border-[#30363d] px-3 py-2">
                  <button
                    type="button"
                    onClick={() => { onChange({ sprintId: null, assigneeId: null, priority: '', tagId: null }); setOpen(false) }}
                    className="text-xs text-[#f85149] hover:text-red-400"
                  >
                    Clear all filters
                  </button>
                </div>
              )}
            </>
          ) : (
            <>
              <div className="flex items-center gap-2 px-3 py-2 border-b border-[#30363d]">
                <button type="button" onClick={() => { setCategory(null); setSearch('') }} className="text-[#8b949e] hover:text-[#c9d1d9]">
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" /></svg>
                </button>
                <span className="text-xs text-[#8b949e] font-semibold uppercase tracking-wide">Filter by {category}</span>
              </div>
              {category !== 'priority' && (
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
                    (category === 'sprint'   && sprintId   === Number(opt.value)) ||
                    (category === 'assignee' && assigneeId === Number(opt.value)) ||
                    (category === 'priority' && priority   === opt.value) ||
                    (category === 'label'    && tagId      === Number(opt.value))
                  return (
                    <button
                      key={opt.value}
                      type="button"
                      onClick={() => selectOption(category, opt.value)}
                      className={`w-full flex items-center gap-3 px-3 py-2 hover:bg-[#1f6feb]/10 transition-colors ${isActive ? 'text-[#79c0ff]' : 'text-[#c9d1d9]'}`}
                    >
                      <span className={`w-3 h-3 rounded-full border-2 flex-shrink-0 ${isActive ? 'bg-[#1f6feb] border-[#1f6feb]' : 'border-[#484f58]'}`} />
                      {opt.color && <span className="w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: opt.color }} />}
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

export default function ProjectDetail() {
  const { projectId } = useParams<{ projectId: string }>()
  const navigate = useNavigate()
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

  // Load project metadata, swim lanes, sprints and tags
  useEffect(() => {
    if (projectId) {
      loadProject()
      loadSwimLanes()
      api.getSprints(Number(projectId)).then(setSprints).catch(() => {})
      api.getTags(Number(projectId)).then(setTags).catch(() => {})
    }
  }, [projectId]) // eslint-disable-line react-hooks/exhaustive-deps

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
      } else {
        setFilterSprint(null)
        setFilterAssignee(null)
        setFilterPriority('')
        setFilterTag(null)
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
    }))
  }, [projectId, filterSprint, filterAssignee, filterPriority, filterTag])

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
      console.error('Failed to load swim lanes:', err)
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

  // Derive unique assignees from loaded tasks (must be before early returns)
  const uniqueAssignees = useMemo(() => {
    const map = new Map<number, string>()
    tasks.forEach(t => {
      if (t.assignee_id && !map.has(t.assignee_id)) {
        map.set(t.assignee_id, t.assignee_name || `User ${t.assignee_id}`)
      }
    })
    return Array.from(map.entries()).map(([id, name]) => ({ id, name }))
  }, [tasks])

  // Apply board filters (must be before early returns)
  const filteredTasks = useMemo(() => tasks.filter(t => {
    if (filterSprint   !== null && t.sprint_id !== filterSprint) return false
    if (filterAssignee !== null && t.assignee_id !== filterAssignee) return false
    if (filterPriority && t.priority !== filterPriority) return false
    if (filterTag      !== null && !t.tags?.some(tag => tag.id === filterTag)) return false
    return true
  }), [tasks, filterSprint, filterAssignee, filterPriority, filterTag])

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

            {/* Filter bar + Task Stats */}
            <div className="flex items-center gap-4 py-3">
              <BoardFilterBar
                sprints={sprints}
                assignees={uniqueAssignees}
                tags={tags}
                sprintId={filterSprint}
                assigneeId={filterAssignee}
                priority={filterPriority}
                tagId={filterTag}
                onChange={patch => {
                  if ('sprintId'   in patch) setFilterSprint(patch.sprintId ?? null)
                  if ('assigneeId' in patch) setFilterAssignee(patch.assigneeId ?? null)
                  if ('priority'   in patch) setFilterPriority(patch.priority ?? '')
                  if ('tagId'      in patch) setFilterTag(patch.tagId ?? null)
                }}
              />
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
