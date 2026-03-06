/**
 * useLocalTasks Hook
 * React hook for managing tasks with server-side fetching
 */

import { useState, useEffect } from 'react'
import { api, type Task, type UpdateTaskRequest } from '../lib/api'

export function useLocalTasks(projectId: number) {
  const [tasks, setTasks] = useState<Task[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const fetchTasks = async () => {
      try {
        setLoading(true)
        const serverTasks = await api.getTasks(projectId)
        setTasks(serverTasks)
        setLoading(false)
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load tasks')
        setLoading(false)
      }
    }
    fetchTasks()
  }, [projectId])

  // Real-time task updates via WebSocket events
  useEffect(() => {
    const handleCreated = (e: Event) => {
      const task = (e as CustomEvent<Task>).detail
      if (task.project_id !== projectId) return
      setTasks(prev => prev.some(t => t.id === task.id) ? prev : [task, ...prev])
    }
    const handleUpdated = (e: Event) => {
      const task = (e as CustomEvent<Task>).detail
      if (task.project_id !== projectId) return
      setTasks(prev => prev.map(t => t.id === task.id ? task : t))
    }
    const handleDeleted = (e: Event) => {
      const { id, project_id } = (e as CustomEvent<{ id: number; project_id: number }>).detail
      if (project_id !== projectId) return
      setTasks(prev => prev.filter(t => t.id !== id))
    }
    window.addEventListener('task_created', handleCreated)
    window.addEventListener('task_updated', handleUpdated)
    window.addEventListener('task_deleted', handleDeleted)
    return () => {
      window.removeEventListener('task_created', handleCreated)
      window.removeEventListener('task_updated', handleUpdated)
      window.removeEventListener('task_deleted', handleDeleted)
    }
  }, [projectId])

  const createTask = async (data: {
    title: string
    description?: string
    status?: 'todo' | 'in_progress' | 'done'
    swim_lane_id?: number
    priority?: 'low' | 'medium' | 'high' | 'urgent'
    assignee_id?: number
    due_date?: string
  }) => {
    const newTask = await api.createTask(projectId, data)
    setTasks(prev => prev.some(t => t.id === newTask.id) ? prev : [newTask, ...prev])
  }

  const updateTask = async (taskId: number, updates: UpdateTaskRequest) => {
    const updatedTask = await api.updateTask(taskId, updates)
    setTasks(prev => prev.map(t => t.id === taskId ? updatedTask : t))
  }

  const deleteTask = async (taskId: number) => {
    await api.deleteTask(taskId)
    setTasks(prev => prev.filter(t => t.id !== taskId))
  }

  const updateTaskStatus = async (
    taskId: number,
    newStatus: 'todo' | 'in_progress' | 'done'
  ) => {
    await updateTask(taskId, { status: newStatus })
  }

  return {
    tasks,
    loading,
    error,
    createTask,
    updateTask,
    deleteTask,
    updateTaskStatus,
  }
}
