import { useState, useRef, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useNotifications } from '../state/NotificationContext'

export default function NotificationBell() {
  const {
    count, invitations, notifications, unreadNotifCount,
    acceptInvitation, rejectInvitation, markNotificationRead, markAllNotificationsRead,
  } = useNotifications()
  const [isOpen, setIsOpen] = useState(false)
  const [tab, setTab] = useState<'notifications' | 'invitations'>('notifications')
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
    try { await acceptInvitation(id) } finally { setActionLoading(null) }
  }

  const handleReject = async (id: number) => {
    setActionLoading(id)
    try { await rejectInvitation(id) } finally { setActionLoading(null) }
  }

  const handleNotifClick = async (notifId: number, link: string) => {
    await markNotificationRead(notifId)
    setIsOpen(false)
    navigate(link)
  }

  const notifTypeIcon = (type: string) => {
    switch (type) {
      case 'mention': return '💬'
      case 'task_comment': return '📋'
      case 'annotation_comment': return '✏️'
      case 'reply': return '↩️'
      default: return '🔔'
    }
  }

  return (
    <div className="relative" ref={dropdownRef}>
      <button
        onClick={() => setIsOpen(v => !v)}
        className="relative p-1.5 text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary rounded-md transition-colors"
        aria-label={count > 0 ? `${count} unread notification${count !== 1 ? 's' : ''}` : 'Notifications'}
        title={count > 0 ? `${count} unread notification${count !== 1 ? 's' : ''}` : 'Notifications'}
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
          {/* Tabs */}
          <div className="flex border-b border-dark-border-subtle">
            <button
              onClick={() => setTab('notifications')}
              className={`flex-1 px-4 py-2.5 text-xs font-semibold transition-colors ${
                tab === 'notifications'
                  ? 'text-primary-400 border-b-2 border-primary-400'
                  : 'text-dark-text-tertiary hover:text-dark-text-secondary'
              }`}
            >
              Notifications {unreadNotifCount > 0 && <span className="ml-1 bg-primary-500/20 text-primary-400 text-[10px] px-1 rounded-full">{unreadNotifCount}</span>}
            </button>
            <button
              onClick={() => setTab('invitations')}
              className={`flex-1 px-4 py-2.5 text-xs font-semibold transition-colors ${
                tab === 'invitations'
                  ? 'text-primary-400 border-b-2 border-primary-400'
                  : 'text-dark-text-tertiary hover:text-dark-text-secondary'
              }`}
            >
              Invitations {invitations.length > 0 && <span className="ml-1 bg-danger-500/20 text-danger-400 text-[10px] px-1 rounded-full">{invitations.length}</span>}
            </button>
          </div>

          {tab === 'notifications' && (
            <>
              {unreadNotifCount > 0 && (
                <div className="px-4 py-2 border-b border-dark-border-subtle flex justify-end">
                  <button
                    onClick={markAllNotificationsRead}
                    className="text-xs text-primary-400 hover:text-primary-300 transition-colors"
                  >
                    Mark all read
                  </button>
                </div>
              )}
              {notifications.length === 0 ? (
                <div className="px-4 py-6 text-center text-sm text-dark-text-tertiary">
                  No notifications
                </div>
              ) : (
                <div className="max-h-72 overflow-y-auto divide-y divide-dark-border-subtle">
                  {notifications.map(n => (
                    <button
                      key={n.id}
                      onClick={() => handleNotifClick(n.id, n.link)}
                      className={`w-full text-left px-4 py-3 hover:bg-dark-bg-secondary transition-colors ${!n.read_at ? 'bg-primary-500/5' : ''}`}
                    >
                      <div className="flex items-start gap-2">
                        <span className="text-base mt-0.5 shrink-0">{notifTypeIcon(n.type)}</span>
                        <div className="min-w-0">
                          <p className={`text-xs leading-snug ${!n.read_at ? 'text-dark-text-primary font-medium' : 'text-dark-text-secondary'}`}>
                            {n.message}
                          </p>
                          <p className="text-[10px] text-dark-text-tertiary mt-0.5">
                            {n.project_name} · {new Date(n.created_at).toLocaleDateString()}
                          </p>
                        </div>
                        {!n.read_at && (
                          <span className="w-1.5 h-1.5 rounded-full bg-primary-400 shrink-0 mt-1.5" />
                        )}
                      </div>
                    </button>
                  ))}
                </div>
              )}
            </>
          )}

          {tab === 'invitations' && (
            <>
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
              <div className="px-4 py-2 border-t border-dark-border-subtle">
                <button
                  onClick={() => { setIsOpen(false); navigate('/app/settings') }}
                  className="text-xs text-primary-400 hover:text-primary-300 transition-colors"
                >
                  View all in Settings
                </button>
              </div>
            </>
          )}
        </div>
      )}
    </div>
  )
}
