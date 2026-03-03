import { createContext, useContext, useState, useEffect, useCallback, useRef, ReactNode } from 'react'
import { apiClient, ProjectInvitation } from '../lib/api'

interface NotificationContextType {
  count: number
  invitations: ProjectInvitation[]
  refreshInvitations: () => Promise<void>
  acceptInvitation: (id: number) => Promise<void>
  rejectInvitation: (id: number) => Promise<void>
}

const NotificationContext = createContext<NotificationContextType | undefined>(undefined)

// Derive WebSocket base URL from the same logic as API_BASE_URL
function getWSBase(): string {
  const apiBase = (import.meta.env.VITE_API_URL as string | undefined) ||
    (import.meta.env.PROD ? '' : 'http://localhost:8080')
  if (!apiBase) {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${proto}//${window.location.host}`
  }
  return apiBase.replace(/^http/, 'ws')
}

export function NotificationProvider({ children }: { children: ReactNode }) {
  const [invitations, setInvitations] = useState<ProjectInvitation[]>([])
  const mountedRef = useRef(true)

  const refreshInvitations = useCallback(async () => {
    try {
      const data = await apiClient.getMyProjectInvitations()
      if (mountedRef.current) {
        setInvitations(data.filter(inv => inv.status === 'pending'))
      }
    } catch {
      // ignore — will retry on next poll or WS event
    }
  }, [])

  // WebSocket connection with reconnect
  useEffect(() => {
    mountedRef.current = true
    let ws: WebSocket | null = null
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let delay = 1000

    const connect = () => {
      if (!mountedRef.current) return
      const token = localStorage.getItem('auth_token')
      if (!token) return

      ws = new WebSocket(`${getWSBase()}/api/ws/user?token=${encodeURIComponent(token)}`)

      ws.onopen = () => {
        delay = 1000
      }

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data as string) as { type: string }
          if (msg.type === 'project_invitation') {
            refreshInvitations()
          } else if (msg.type === 'project_membership') {
            window.dispatchEvent(new CustomEvent('project-membership-changed'))
            refreshInvitations()
          }
        } catch {
          // ignore malformed messages
        }
      }

      ws.onclose = () => {
        ws = null
        if (!mountedRef.current) return
        reconnectTimer = setTimeout(() => {
          delay = Math.min(delay * 2, 30000)
          connect()
        }, delay)
      }

      ws.onerror = () => {
        ws?.close()
      }
    }

    refreshInvitations()
    connect()

    // Polling fallback every 60s
    const pollInterval = setInterval(refreshInvitations, 60_000)

    return () => {
      mountedRef.current = false
      clearInterval(pollInterval)
      if (reconnectTimer) clearTimeout(reconnectTimer)
      if (ws) {
        ws.onclose = null
        ws.close()
      }
    }
  }, [refreshInvitations])

  const acceptInvitation = useCallback(async (id: number) => {
    await apiClient.acceptProjectInvitation(id)
    window.dispatchEvent(new CustomEvent('project-membership-changed'))
    await refreshInvitations()
  }, [refreshInvitations])

  const rejectInvitation = useCallback(async (id: number) => {
    await apiClient.rejectProjectInvitation(id)
    await refreshInvitations()
  }, [refreshInvitations])

  return (
    <NotificationContext.Provider value={{
      count: invitations.length,
      invitations,
      refreshInvitations,
      acceptInvitation,
      rejectInvitation,
    }}>
      {children}
    </NotificationContext.Provider>
  )
}

export function useNotifications() {
  const context = useContext(NotificationContext)
  if (!context) throw new Error('useNotifications must be used within NotificationProvider')
  return context
}
