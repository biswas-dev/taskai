import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import type { Task, Milestone, Sprint } from '../../lib/api'
import Select from '../ui/Select'

interface RoadmapListProps {
  milestones: Milestone[]
  tasks: Task[]
  sprints: Sprint[]
  projectId: number
}

type SortField = 'task_number' | 'title' | 'milestone' | 'sprint' | 'assignee' | 'priority' | 'due_date' | 'status'
type SortDir = 'asc' | 'desc'

const PRIORITY_ORDER: Record<string, number> = { urgent: 0, high: 1, medium: 2, low: 3 }
const PRIORITY_COLORS: Record<string, string> = { urgent: '#ef4444', high: '#f97316', medium: '#eab308', low: '#6b7280' }

export default function RoadmapList({ milestones, tasks, sprints, projectId }: RoadmapListProps) {
  const navigate = useNavigate()
  const [sortField, setSortField] = useState<SortField>('task_number')
  const [sortDir, setSortDir] = useState<SortDir>('asc')
  const [filterMilestone, setFilterMilestone] = useState<number | ''>('')
  const [filterSprint, setFilterSprint] = useState<number | ''>('')

  const milestoneMap = useMemo(() => {
    const map: Record<number, Milestone> = {}
    milestones.forEach(m => { map[m.id] = m })
    return map
  }, [milestones])

  const sortedTasks = useMemo(() => {
    let filtered = [...tasks]

    if (filterMilestone !== '') {
      filtered = filtered.filter(t => (t as Task & { milestone_id?: number }).milestone_id === filterMilestone)
    }
    if (filterSprint !== '') {
      filtered = filtered.filter(t => (t as Task & { sprint_id?: number }).sprint_id === filterSprint)
    }

    filtered.sort((a, b) => {
      let cmp = 0
      const aExt = a as Task & { milestone_id?: number; milestone_name?: string; sprint_id?: number; sprint_name?: string }
      const bExt = b as Task & { milestone_id?: number; milestone_name?: string; sprint_id?: number; sprint_name?: string }

      switch (sortField) {
        case 'task_number':
          cmp = (a.task_number || 0) - (b.task_number || 0)
          break
        case 'title':
          cmp = (a.title || '').localeCompare(b.title || '')
          break
        case 'milestone':
          cmp = (aExt.milestone_name || 'zzz').localeCompare(bExt.milestone_name || 'zzz')
          break
        case 'sprint':
          cmp = (aExt.sprint_name || 'zzz').localeCompare(bExt.sprint_name || 'zzz')
          break
        case 'assignee':
          cmp = (a.assignees?.[0]?.user_name || 'zzz').localeCompare(b.assignees?.[0]?.user_name || 'zzz')
          break
        case 'priority':
          cmp = (PRIORITY_ORDER[a.priority || 'medium'] ?? 9) - (PRIORITY_ORDER[b.priority || 'medium'] ?? 9)
          break
        case 'due_date':
          cmp = (a.due_date || '9999').localeCompare(b.due_date || '9999')
          break
        case 'status':
          cmp = (a.status || '').localeCompare(b.status || '')
          break
      }
      return sortDir === 'asc' ? cmp : -cmp
    })

    return filtered
  }, [tasks, sortField, sortDir, filterMilestone, filterSprint])

  const toggleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDir(d => d === 'asc' ? 'desc' : 'asc')
    } else {
      setSortField(field)
      setSortDir('asc')
    }
  }

  const SortHeader = ({ field, children }: { field: SortField; children: React.ReactNode }) => (
    <th
      className="px-3 py-2 text-left text-xs font-medium text-dark-text-tertiary uppercase tracking-wider cursor-pointer hover:text-dark-text-primary transition-colors select-none"
      onClick={() => toggleSort(field)}
    >
      <div className="flex items-center gap-1">
        {children}
        {sortField === field && (
          <svg className={`w-3 h-3 ${sortDir === 'desc' ? 'rotate-180' : ''}`} fill="currentColor" viewBox="0 0 20 20">
            <path fillRule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clipRule="evenodd" />
          </svg>
        )}
      </div>
    </th>
  )

  return (
    <div className="flex flex-col h-full">
      {/* Filters */}
      <div className="flex items-center gap-3 px-4 py-2 border-b border-dark-border-subtle">
        <Select<number | ''>
          aria-label="Filter by milestone"
          size="sm"
          className="w-40"
          value={filterMilestone}
          onChange={setFilterMilestone}
          options={[{ value: '', label: 'All milestones' }, ...milestones.map(m => ({ value: m.id, label: m.name }))]}
        />
        <Select<number | ''>
          aria-label="Filter by sprint"
          size="sm"
          className="w-40"
          value={filterSprint}
          onChange={setFilterSprint}
          options={[{ value: '', label: 'All sprints' }, ...sprints.map(s => ({ value: s.id, label: s.name }))]}
        />
        <span className="ml-auto text-xs text-dark-text-quaternary">{sortedTasks.length} tasks</span>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        <table className="w-full">
          <thead className="sticky top-0 bg-dark-bg-secondary z-10">
            <tr className="border-b border-dark-border-subtle">
              <SortHeader field="task_number">#</SortHeader>
              <SortHeader field="title">Title</SortHeader>
              <SortHeader field="milestone">Milestone</SortHeader>
              <SortHeader field="sprint">Sprint</SortHeader>
              <SortHeader field="assignee">Assignee</SortHeader>
              <SortHeader field="priority">Priority</SortHeader>
              <SortHeader field="due_date">Due Date</SortHeader>
              <SortHeader field="status">Status</SortHeader>
            </tr>
          </thead>
          <tbody>
            {sortedTasks.map(task => {
              const ext = task as Task & { milestone_id?: number; milestone_name?: string; sprint_id?: number; sprint_name?: string }
              const milestone = ext.milestone_id ? milestoneMap[ext.milestone_id] : null
              const isDone = (task.status || '') === 'done'
              const overdue = task.due_date && new Date(task.due_date) < new Date() && !isDone

              return (
                <tr
                  key={task.id}
                  className={`border-b border-dark-border-subtle/30 hover:bg-dark-bg-tertiary/20 cursor-pointer transition-colors ${isDone ? 'opacity-50' : ''}`}
                  onClick={() => task.task_number && navigate(`/app/projects/${projectId}/tasks/${task.task_number}`)}
                >
                  <td className="px-3 py-2 text-xs text-dark-text-tertiary w-14">{task.task_number}</td>
                  <td className="px-3 py-2">
                    <span className={`text-sm ${isDone ? 'text-dark-text-quaternary line-through' : 'text-dark-text-primary'}`}>
                      {task.title}
                    </span>
                  </td>
                  <td className="px-3 py-2">
                    {milestone ? (
                      <div className="flex items-center gap-1.5">
                        <div className="w-2 h-2 rounded-full" style={{ backgroundColor: milestone.color }} />
                        <span className="text-xs text-dark-text-secondary truncate max-w-[120px]">{milestone.name}</span>
                      </div>
                    ) : (
                      <span className="text-xs text-dark-text-quaternary">-</span>
                    )}
                  </td>
                  <td className="px-3 py-2 text-xs text-dark-text-secondary truncate max-w-[100px]">
                    {ext.sprint_name || '-'}
                  </td>
                  <td className="px-3 py-2 text-xs text-dark-text-secondary truncate max-w-[100px]">
                    {task.assignees?.map(a => a.user_name).join(', ') || '-'}
                  </td>
                  <td className="px-3 py-2">
                    <div className="flex items-center gap-1.5">
                      <div className="w-1.5 h-1.5 rounded-full" style={{ backgroundColor: PRIORITY_COLORS[task.priority || 'medium'] || '#6b7280' }} />
                      <span className="text-xs text-dark-text-secondary capitalize">{task.priority}</span>
                    </div>
                  </td>
                  <td className={`px-3 py-2 text-xs ${overdue ? 'text-danger-400' : 'text-dark-text-secondary'}`}>
                    {task.due_date ? new Date(task.due_date).toLocaleDateString('en-US', { month: 'short', day: 'numeric' }) : '-'}
                  </td>
                  <td className="px-3 py-2">
                    <span className={`text-xs px-1.5 py-0.5 rounded ${
                      isDone ? 'bg-success-500/10 text-success-400' :
                      task.status === 'in_progress' ? 'bg-primary-500/10 text-primary-400' :
                      'bg-dark-bg-tertiary text-dark-text-tertiary'
                    }`}>
                      {task.status === 'in_progress' ? 'In Progress' : task.status === 'done' ? 'Done' : 'To Do'}
                    </span>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
        {sortedTasks.length === 0 && (
          <div className="text-center py-12 text-sm text-dark-text-quaternary">
            No tasks match the current filters
          </div>
        )}
      </div>
    </div>
  )
}
