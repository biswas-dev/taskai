import { useState, useEffect, useRef } from 'react'
import { Link } from 'react-router-dom'
import { apiClient } from '../lib/api'

const DISMISSED_KEY = 'project_invitation_banner_dismissed_count'

export default function ProjectInvitationBanner() {
  const [count, setCount] = useState(0)
  const [dismissed, setDismissed] = useState(false)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const fetchCount = async () => {
    try {
      const { count: c } = await apiClient.getMyProjectInvitationCount()
      setCount(prev => {
        // Re-show banner if count increased since last dismiss
        if (c > prev) {
          setDismissed(false)
          sessionStorage.removeItem(DISMISSED_KEY)
        }
        return c
      })
    } catch {
      // ignore
    }
  }

  useEffect(() => {
    // Check if previously dismissed for this count
    const dismissedCountStr = sessionStorage.getItem(DISMISSED_KEY)
    if (dismissedCountStr) {
      setDismissed(true)
    }

    fetchCount()
    intervalRef.current = setInterval(fetchCount, 60_000)
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current)
    }
  }, [])

  const handleDismiss = () => {
    setDismissed(true)
    sessionStorage.setItem(DISMISSED_KEY, String(count))
  }

  if (count === 0 || dismissed) return null

  return (
    <div className="bg-primary-500/10 border-b border-primary-500/20 px-4 py-2 flex items-center justify-between gap-4">
      <div className="flex items-center gap-2 text-sm text-primary-300">
        <svg className="w-4 h-4 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
          <path d="M10 2a6 6 0 00-6 6v3.586l-.707.707A1 1 0 004 14h12a1 1 0 00.707-1.707L16 11.586V8a6 6 0 00-6-6zM10 18a3 3 0 01-3-3h6a3 3 0 01-3 3z" />
        </svg>
        <span>
          You have <strong>{count}</strong> pending project invitation{count !== 1 ? 's' : ''}.{' '}
          <Link to="/app/settings" className="underline hover:text-primary-200 transition-colors">
            View &amp; respond
          </Link>
        </span>
      </div>
      <button
        onClick={handleDismiss}
        className="flex-shrink-0 p-1 text-primary-400 hover:text-primary-200 transition-colors"
        aria-label="Dismiss"
      >
        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>
  )
}
