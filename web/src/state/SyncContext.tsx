/**
 * Sync Context
 * React context for managing sync state
 * Note: RxDB local-first support was removed. This context now only provides
 * server-only sync state for downstream consumers.
 */

import { createContext, useContext, useEffect, useState, ReactNode } from 'react'
import { useAuth } from './AuthContext'

interface SyncState {
  status: 'idle' | 'syncing' | 'synced' | 'error' | 'offline'
  lastSyncTime: number | null
  error: string | null
  pendingOperations: number
}

interface SyncContextValue {
  db: null
  syncService: null
  syncState: SyncState
  isInitialized: boolean
  initializeSync: () => Promise<void>
  destroySync: () => Promise<void>
  triggerSync: () => Promise<void>
}

const SyncContext = createContext<SyncContextValue | undefined>(undefined)

export function SyncProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth()
  const [syncState, setSyncState] = useState<SyncState>({
    status: 'idle',
    lastSyncTime: null,
    error: null,
    pendingOperations: 0,
  })
  const [isInitialized, setIsInitialized] = useState(false)

  const initializeSync = async () => {
    if (!user?.id || isInitialized) return
    setIsInitialized(true)
    setSyncState({
      status: 'synced',
      lastSyncTime: Date.now(),
      error: null,
      pendingOperations: 0,
    })
  }

  const destroySync = async () => {
    setIsInitialized(false)
    setSyncState({
      status: 'idle',
      lastSyncTime: null,
      error: null,
      pendingOperations: 0,
    })
  }

  const triggerSync = async () => {
    // No-op in server-only mode
  }

  useEffect(() => {
    if (user && !isInitialized) {
      initializeSync()
    } else if (!user && isInitialized) {
      destroySync()
    }
  }, [user?.id])

  const value: SyncContextValue = {
    db: null,
    syncService: null,
    syncState,
    isInitialized,
    initializeSync,
    destroySync,
    triggerSync,
  }

  return <SyncContext.Provider value={value}>{children}</SyncContext.Provider>
}

export function useSync() {
  const context = useContext(SyncContext)
  if (context === undefined) {
    throw new Error('useSync must be used within a SyncProvider')
  }
  return context
}
