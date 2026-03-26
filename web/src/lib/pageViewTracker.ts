import { useEffect, useRef } from 'react'
import { useLocation } from 'react-router-dom'
import { api } from './api'

function getSessionId(): string {
  let id = sessionStorage.getItem('taskai_session_id')
  if (!id) {
    id = crypto.randomUUID()
    sessionStorage.setItem('taskai_session_id', id)
  }
  return id
}

/**
 * Tracks page views and time-on-page for analytics.
 * Must be called inside a Router context and only for authenticated users.
 */
export function usePageViewTracker() {
  const location = useLocation()
  const prevPath = useRef<string | null>(null)
  const enteredAt = useRef<number>(Date.now())
  const sessionId = useRef(getSessionId())

  useEffect(() => {
    const currentPath = location.pathname

    // On navigation, send the *previous* page's data
    if (prevPath.current && prevPath.current !== currentPath) {
      const durationMs = Date.now() - enteredAt.current

      // Skip pages visited for less than 1 second (rapid navigation / redirects)
      if (durationMs >= 1000) {
        sendBeacon(prevPath.current, currentPath, sessionId.current, durationMs)
      }
    }

    prevPath.current = currentPath
    enteredAt.current = Date.now()
  }, [location.pathname])

  // Send final page view on tab close
  useEffect(() => {
    const handleUnload = () => {
      if (prevPath.current) {
        const durationMs = Date.now() - enteredAt.current
        if (durationMs >= 1000) {
          sendBeacon(prevPath.current, '', sessionId.current, durationMs)
        }
      }
    }

    window.addEventListener('beforeunload', handleUnload)
    return () => window.removeEventListener('beforeunload', handleUnload)
  }, [])
}

function sendBeacon(path: string, referrer: string, sessionId: string, durationMs: number) {
  // Use fire-and-forget fetch with keepalive for reliability on page unload
  try {
    api.trackPageView({ path, referrer, session_id: sessionId, duration_ms: durationMs }).catch(() => {
      // Silently ignore errors — analytics should never break the app
    })
  } catch {
    // Ignore
  }
}
