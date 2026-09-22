/**
 * Sync Context
 * React context for managing sync state
 * Note: RxDB local-first support was removed. This context provides a
 * lightweight server refresh bus for views that need Google Docs-style freshness.
 */

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, ReactNode } from 'react'
import { useAuth } from './AuthContext'

interface SyncState {
  status: 'idle' | 'syncing' | 'synced' | 'error' | 'offline'
  lastSyncTime: number | null
  error: string | null
  pendingOperations: number
}

type SyncTask = () => Promise<void> | void

interface SyncContextValue {
  db: null
  syncService: null
  syncState: SyncState
  isInitialized: boolean
  initializeSync: () => Promise<void>
  destroySync: () => Promise<void>
  triggerSync: () => Promise<void>
  registerSyncTask: (id: string, task: SyncTask) => () => void
}

const SYNC_INTERVAL_MS = 15_000

const SyncContext = createContext<SyncContextValue | undefined>(undefined)

export function SyncProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth()
  const syncTasksRef = useRef<Map<string, SyncTask>>(new Map())
  const isSyncingRef = useRef(false)
  const [syncState, setSyncState] = useState<SyncState>({
    status: 'idle',
    lastSyncTime: null,
    error: null,
    pendingOperations: 0,
  })
  const [isInitialized, setIsInitialized] = useState(false)

  const initializeSync = useCallback(async () => {
    if (!user?.id || isInitialized) return
    setIsInitialized(true)
    setSyncState({
      status: 'synced',
      lastSyncTime: Date.now(),
      error: null,
      pendingOperations: 0,
    })
  }, [isInitialized, user?.id])

  const destroySync = useCallback(async () => {
    syncTasksRef.current.clear()
    setIsInitialized(false)
    setSyncState({
      status: 'idle',
      lastSyncTime: null,
      error: null,
      pendingOperations: 0,
    })
  }, [])

  const registerSyncTask = useCallback((id: string, task: SyncTask) => {
    syncTasksRef.current.set(id, task)
    return () => {
      syncTasksRef.current.delete(id)
    }
  }, [])

  const triggerSync = useCallback(async () => {
    if (!user?.id) return

    if (typeof navigator !== 'undefined' && !navigator.onLine) {
      setSyncState(prev => ({
        ...prev,
        status: 'offline',
        error: 'You are offline. Changes will refresh when the connection returns.',
        pendingOperations: 0,
      }))
      return
    }

    if (isSyncingRef.current) return
    isSyncingRef.current = true

    const tasks = Array.from(syncTasksRef.current.values())
    setSyncState(prev => ({
      ...prev,
      status: 'syncing',
      error: null,
      pendingOperations: tasks.length,
    }))

    try {
      const results = await Promise.allSettled(tasks.map(task => task()))
      const rejected = results.find((result): result is PromiseRejectedResult => result.status === 'rejected')
      if (rejected) {
        throw rejected.reason
      }
      setSyncState({
        status: 'synced',
        lastSyncTime: Date.now(),
        error: null,
        pendingOperations: 0,
      })
    } catch (err) {
      setSyncState(prev => ({
        ...prev,
        status: 'error',
        error: err instanceof Error ? err.message : 'Failed to refresh latest changes',
        pendingOperations: 0,
      }))
    } finally {
      isSyncingRef.current = false
    }
  }, [user?.id])

  useEffect(() => {
    if (user && !isInitialized) {
      initializeSync()
    } else if (!user && isInitialized) {
      destroySync()
    }
  }, [destroySync, initializeSync, isInitialized, user])

  useEffect(() => {
    if (!user?.id || !isInitialized) return

    const refreshIfActive = () => {
      if (typeof document !== 'undefined' && document.hidden) return
      void triggerSync()
    }

    const interval = window.setInterval(refreshIfActive, SYNC_INTERVAL_MS)
    window.addEventListener('focus', refreshIfActive)
    window.addEventListener('online', refreshIfActive)
    window.addEventListener('offline', refreshIfActive)
    document.addEventListener('visibilitychange', refreshIfActive)

    return () => {
      window.clearInterval(interval)
      window.removeEventListener('focus', refreshIfActive)
      window.removeEventListener('online', refreshIfActive)
      window.removeEventListener('offline', refreshIfActive)
      document.removeEventListener('visibilitychange', refreshIfActive)
    }
  }, [isInitialized, triggerSync, user?.id])

  // Memoized: the 15s heartbeat flips syncState twice per tick, and a fresh
  // context object would re-render every consumer of useSync() each time.
  const value: SyncContextValue = useMemo(() => ({
    db: null,
    syncService: null,
    syncState,
    isInitialized,
    initializeSync,
    destroySync,
    triggerSync,
    registerSyncTask,
  }), [destroySync, initializeSync, isInitialized, registerSyncTask, syncState, triggerSync])

  return <SyncContext.Provider value={value}>{children}</SyncContext.Provider>
}

// eslint-disable-next-line react-refresh/only-export-components
export function useSync() {
  const context = useContext(SyncContext)
  if (context === undefined) {
    throw new Error('useSync must be used within a SyncProvider')
  }
  return context
}
