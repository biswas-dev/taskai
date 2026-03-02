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
    setTasks(prev => [newTask, ...prev])
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
