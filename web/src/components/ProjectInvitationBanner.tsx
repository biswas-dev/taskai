import { useState, useRef, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useNotifications } from '../state/NotificationContext'

export default function NotificationBell() {
  const { count, invitations, acceptInvitation, rejectInvitation } = useNotifications()
  const [isOpen, setIsOpen] = useState(false)
  const [actionLoading, setActionLoading] = useState<number | null>(null)
  const dropdownRef = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()

  // Close dropdown on outside click
  useEffect(() => {
    if (!isOpen) return
    const handler = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setIsOpen(false)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [isOpen])

  const handleAccept = async (id: number) => {
    setActionLoading(id)
    try {
      await acceptInvitation(id)
    } finally {
      setActionLoading(null)
    }
  }

  const handleReject = async (id: number) => {
    setActionLoading(id)
    try {
      await rejectInvitation(id)
    } finally {
      setActionLoading(null)
    }
  }

  return (
    <div className="relative" ref={dropdownRef}>
      <button
        onClick={() => setIsOpen(v => !v)}
        className="relative p-1.5 text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary rounded-md transition-colors"
        aria-label={count > 0 ? `${count} pending project invitation${count !== 1 ? 's' : ''}` : 'Notifications'}
        title={count > 0 ? `${count} pending project invitation${count !== 1 ? 's' : ''}` : 'Notifications'}
      >
        <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
        </svg>
        {count > 0 && (
          <span className="absolute -top-0.5 -right-0.5 min-w-[16px] h-4 px-0.5 bg-danger-500 text-white text-[10px] font-bold rounded-full flex items-center justify-center leading-none">
            {count > 9 ? '9+' : count}
          </span>
        )}
      </button>

      {isOpen && (
        <div className="absolute right-0 mt-1 w-80 bg-dark-bg-primary border border-dark-border-subtle rounded-lg shadow-xl z-50">
          <div className="px-4 py-3 border-b border-dark-border-subtle flex items-center justify-between">
            <h3 className="text-sm font-semibold text-dark-text-primary">Project Invitations</h3>
            <button
              onClick={() => { setIsOpen(false); navigate('/app/settings') }}
              className="text-xs text-primary-400 hover:text-primary-300 transition-colors"
            >
              View all
            </button>
          </div>

          {invitations.length === 0 ? (
            <div className="px-4 py-6 text-center text-sm text-dark-text-tertiary">
              No pending invitations
            </div>
          ) : (
            <div className="max-h-72 overflow-y-auto divide-y divide-dark-border-subtle">
              {invitations.map(inv => (
                <div key={inv.id} className="px-4 py-3">
                  <p className="text-sm font-medium text-dark-text-primary truncate">
                    {inv.project_name ?? 'Unknown project'}
                  </p>
                  <p className="text-xs text-dark-text-tertiary mt-0.5">
                    Invited by {inv.inviter_name ?? 'someone'} · <span className="capitalize">{inv.role}</span>
                  </p>
                  <div className="flex gap-2 mt-2">
                    <button
                      onClick={() => handleAccept(inv.id)}
                      disabled={actionLoading === inv.id}
                      className="flex-1 py-1 px-2 text-xs font-medium bg-primary-500 hover:bg-primary-600 disabled:opacity-50 text-white rounded transition-colors"
                    >
                      {actionLoading === inv.id ? 'Joining...' : 'Join'}
                    </button>
                    <button
                      onClick={() => handleReject(inv.id)}
                      disabled={actionLoading === inv.id}
                      className="flex-1 py-1 px-2 text-xs font-medium bg-dark-bg-tertiary hover:bg-dark-bg-elevated disabled:opacity-50 text-dark-text-secondary rounded border border-dark-border-subtle transition-colors"
                    >
                      Decline
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
