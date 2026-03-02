import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useAuth } from '../state/AuthContext'
import { api, type EmailProviderResponse } from '../lib/api'
import { version as frontendVersion } from '../lib/version'

// API URL with fallback for production (empty string = relative URL)
const API_URL = import.meta.env.VITE_API_URL || ''

// Types from backend
interface UserWithStats {
  id: number
  email: string
  is_admin: boolean
  created_at: string
  login_count: number
  last_login_at?: string | null
  last_login_ip?: string | null
  failed_attempts: number
  invite_count: number
}

interface UserActivity {
  id: number
  user_id: number
  activity_type: string
  ip_address?: string | null
  user_agent?: string | null
  created_at: string
}

type AdminTab = 'users' | 'email' | 'backup' | 'system'

interface VersionInfo {
  version: string
  git_commit: string
  build_time: string
  go_version: string
  platform: string
  server_time: string
  db_version: number
  environment: string
  db_driver: string
}

export default function Admin() {
  const { user } = useAuth()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()

  // Read tab from URL or default to 'users'
  const tabFromUrl = (searchParams.get('tab') as AdminTab) || 'users'
  const [activeTab, setActiveTab] = useState<AdminTab>(tabFromUrl)

  // User management state
  const [users, setUsers] = useState<UserWithStats[]>([])
  const [expandedUserId, setExpandedUserId] = useState<number | null>(null)
  const [activities, setActivities] = useState<Record<number, UserActivity[]>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [activityLoading, setActivityLoading] = useState<number | null>(null)
  const [editingInvites, setEditingInvites] = useState<Record<number, number>>({})
  const [savingInvites, setSavingInvites] = useState<number | null>(null)

  // Email provider state
  const [emailProvider, setEmailProvider] = useState<EmailProviderResponse | null>(null)
  const [emailApiKey, setEmailApiKey] = useState('')
  const [emailSenderEmail, setEmailSenderEmail] = useState('')
  const [emailSenderName, setEmailSenderName] = useState('')
  const [emailError, setEmailError] = useState('')
  const [emailSuccess, setEmailSuccess] = useState('')
  const [isSavingEmail, setIsSavingEmail] = useState(false)
  const [isDeletingEmail, setIsDeletingEmail] = useState(false)
  const [isTestingEmail, setIsTestingEmail] = useState(false)

  // Backup/restore state
  const [isExporting, setIsExporting] = useState(false)
  const [isImporting, setIsImporting] = useState(false)
  const [backupStatus, setBackupStatus] = useState('')
  const [backupError, setBackupError] = useState('')

  // System/version state
  const [versionInfo, setVersionInfo] = useState<VersionInfo | null>(null)
  const [versionLoading, setVersionLoading] = useState(false)

  // Sync activeTab with URL
  useEffect(() => {
    const urlTab = searchParams.get('tab') as AdminTab
    if (urlTab && urlTab !== activeTab) {
      setActiveTab(urlTab)
    }
  }, [searchParams])

  // Update URL when tab changes
  const handleTabChange = (tab: AdminTab) => {
    setActiveTab(tab)
    setSearchParams({ tab })
  }

  useEffect(() => {
    if (!user?.is_admin) {
      navigate('/app')
      return
    }
    loadUsers()
    loadEmailProvider()
    loadVersionInfo()
  }, [user, navigate])

  // === User Management ===
  const loadUsers = async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await api.getUsers()
      setUsers(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load users')
    } finally {
      setLoading(false)
    }
  }

  const loadUserActivity = async (userId: number) => {
    if (activities[userId]) return
    try {
      setActivityLoading(userId)
      const data = await api.getUserActivity(userId)
      setActivities(prev => ({ ...prev, [userId]: data }))
    } catch {
      setActivities(prev => ({ ...prev, [userId]: [] }))
    } finally {
      setActivityLoading(null)
    }
  }

  const toggleExpanded = (userId: number) => {
    if (expandedUserId === userId) {
      setExpandedUserId(null)
    } else {
      setExpandedUserId(userId)
      loadUserActivity(userId)
    }
  }

  const toggleAdminStatus = async (e: React.MouseEvent, userId: number, currentStatus: boolean) => {
    e.stopPropagation()
    try {
      await api.updateUserAdmin(userId, !currentStatus)
      await loadUsers()
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to update admin status')
    }
  }

  const handleSaveInvites = async (e: React.MouseEvent, userId: number) => {
    e.stopPropagation()
    const count = editingInvites[userId]
    if (count === undefined) return
    try {
      setSavingInvites(userId)
      await api.adminBoostInvites(userId, count)
      await loadUsers()
      setEditingInvites(prev => {
        const next = { ...prev }
        delete next[userId]
        return next
      })
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to update invites')
    } finally {
      setSavingInvites(null)
    }
  }

  // === Email Provider ===
  const loadEmailProvider = async () => {
    try {
      const data = await api.getEmailProvider()
      setEmailProvider(data)
      if (data) {
        setEmailSenderEmail(data.sender_email)
        setEmailSenderName(data.sender_name)
      }
    } catch {
      // ignore — no provider configured
    }
  }

  const handleSaveEmailProvider = async (e: React.FormEvent) => {
    e.preventDefault()
    setEmailError('')
    setEmailSuccess('')

    if (!emailApiKey || !emailSenderEmail || !emailSenderName) {
      setEmailError('All fields are required')
      return
    }

    setIsSavingEmail(true)
    try {
      const data = await api.saveEmailProvider({
        api_key: emailApiKey,
        sender_email: emailSenderEmail,
        sender_name: emailSenderName,
      })
      setEmailProvider(data)
      setEmailApiKey('')
      setEmailSuccess(data.status === 'connected' ? 'Email provider saved and connected' : 'Email provider saved but connection test failed')
    } catch (err) {
      setEmailError(err instanceof Error ? err.message : 'Failed to save email provider')
    } finally {
      setIsSavingEmail(false)
    }
  }

  const handleDeleteEmailProvider = async () => {
    if (!confirm('Remove email provider configuration?')) return
    setEmailError('')
    setEmailSuccess('')
    setIsDeletingEmail(true)
    try {
      await api.deleteEmailProvider()
      setEmailProvider(null)
      setEmailApiKey('')
      setEmailSenderEmail('')
      setEmailSenderName('')
      setEmailSuccess('Email provider removed')
    } catch (err) {
      setEmailError(err instanceof Error ? err.message : 'Failed to delete email provider')
    } finally {
      setIsDeletingEmail(false)
    }
  }

  const handleTestEmailProvider = async () => {
    setEmailError('')
    setEmailSuccess('')
    setIsTestingEmail(true)
    try {
      const data = await api.testEmailProvider()
      setEmailProvider(data)
      setEmailSuccess(data.status === 'connected' ? 'Connection successful' : `Connection test failed: ${data.last_error}`)
    } catch (err) {
      setEmailError(err instanceof Error ? err.message : 'Failed to test connection')
    } finally {
      setIsTestingEmail(false)
    }
  }

  // === System/Version ===
  const loadVersionInfo = async () => {
    try {
      setVersionLoading(true)
      const data = await api.getVersion()
      setVersionInfo(data)
    } catch (err) {
      // non-critical load failure
    } finally {
      setVersionLoading(false)
    }
  }

  // === Formatting ===
  const formatDate = (dateStr: string) => {
    const date = new Date(dateStr)
    return date.toLocaleString('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
      hour: 'numeric',
      minute: '2-digit',
    })
  }

  const formatShortDate = (dateStr: string) => {
    const date = new Date(dateStr)
    return date.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    })
  }

  const getActivityBadgeClass = (type: string) => {
    switch (type) {
      case 'login':
        return 'text-emerald-400 bg-emerald-500/10 border-emerald-500/30'
      case 'failed_login':
        return 'text-red-400 bg-red-500/10 border-red-500/30'
      case 'logout':
        return 'text-gray-400 bg-gray-500/10 border-gray-500/30'
      default:
        return 'text-blue-400 bg-blue-500/10 border-blue-500/30'
    }
  }

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'connected':
        return 'text-emerald-400 bg-emerald-500/10 border-emerald-500/30'
      case 'error':
        return 'text-red-400 bg-red-500/10 border-red-500/30'
      case 'suspended':
        return 'text-orange-400 bg-orange-500/10 border-orange-500/30'
      default:
        return 'text-gray-400 bg-gray-500/10 border-gray-500/30'
    }
  }

  if (loading) {
    return (
      <div className="p-8 bg-dark-bg-primary min-h-screen">
        <div className="animate-pulse space-y-4 max-w-4xl mx-auto">
          <div className="h-8 bg-dark-bg-secondary rounded w-1/3"></div>
          <div className="h-16 bg-dark-bg-secondary rounded"></div>
          <div className="h-16 bg-dark-bg-secondary rounded"></div>
          <div className="h-16 bg-dark-bg-secondary rounded"></div>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="p-8 bg-dark-bg-primary min-h-screen">
        <div className="max-w-4xl mx-auto">
          <div className="bg-danger-500/10 border border-danger-500/30 text-danger-300 px-4 py-3 rounded">
            {error}
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="h-full flex flex-col bg-dark-bg-primary">
      {/* Header with tabs */}
      <div className="bg-dark-bg-secondary border-b border-dark-border-subtle px-8 py-6">
        <div className="max-w-4xl mx-auto">
          <h1 className="text-2xl font-bold text-dark-text-primary">Admin Dashboard</h1>
          <div className="mt-4 flex gap-1">
            <button
              onClick={() => handleTabChange('users')}
              className={`px-4 py-2 text-sm font-medium rounded-lg transition-colors ${
                activeTab === 'users'
                  ? 'bg-primary-500/10 text-primary-400 border border-primary-500/30'
                  : 'text-dark-text-secondary hover:text-dark-text-primary hover:bg-dark-bg-tertiary/30'
              }`}
            >
              Users ({users.length})
            </button>
            <button
              onClick={() => handleTabChange('email')}
              className={`px-4 py-2 text-sm font-medium rounded-lg transition-colors flex items-center gap-2 ${
                activeTab === 'email'
                  ? 'bg-primary-500/10 text-primary-400 border border-primary-500/30'
                  : 'text-dark-text-secondary hover:text-dark-text-primary hover:bg-dark-bg-tertiary/30'
              }`}
            >
              Email Provider
              {emailProvider && (
                <span className={`w-2 h-2 rounded-full ${
                  emailProvider.status === 'connected' ? 'bg-emerald-400' :
                  emailProvider.status === 'error' ? 'bg-red-400' :
                  emailProvider.status === 'suspended' ? 'bg-orange-400' : 'bg-gray-400'
                }`} />
              )}
            </button>
            <button
              onClick={() => handleTabChange('backup')}
              className={`px-4 py-2 text-sm font-medium rounded-lg transition-colors ${
                activeTab === 'backup'
                  ? 'bg-primary-500/10 text-primary-400 border border-primary-500/30'
                  : 'text-dark-text-secondary hover:text-dark-text-primary hover:bg-dark-bg-tertiary/30'
              }`}
            >
              Backup & Restore
            </button>
            <button
              onClick={() => handleTabChange('system')}
              className={`px-4 py-2 text-sm font-medium rounded-lg transition-colors ${
                activeTab === 'system'
                  ? 'bg-primary-500/10 text-primary-400 border border-primary-500/30'
                  : 'text-dark-text-secondary hover:text-dark-text-primary hover:bg-dark-bg-tertiary/30'
              }`}
            >
              System
            </button>
          </div>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-8">
        <div className="max-w-4xl mx-auto">
          {/* Users Tab */}
          {activeTab === 'users' && (
            <div className="space-y-2">
              {users.map((u) => (
                <div key={u.id} className="bg-dark-bg-secondary rounded-lg border border-dark-border-subtle overflow-hidden">
                  {/* User Row */}
                  <button
                    onClick={() => toggleExpanded(u.id)}
                    className="w-full px-5 py-4 flex items-center gap-4 hover:bg-dark-bg-tertiary/20 transition-colors text-left"
                  >
                    <svg
                      className={`w-4 h-4 text-dark-text-tertiary transition-transform flex-shrink-0 ${expandedUserId === u.id ? 'rotate-90' : ''}`}
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                    </svg>

                    <div className="flex-1 min-w-0">
                      <span className="text-sm font-medium text-dark-text-primary truncate block">{u.email}</span>
                      <span className="text-xs text-dark-text-tertiary">Joined {formatShortDate(u.created_at)}</span>
                    </div>

                    <div className="hidden sm:flex items-center gap-1.5 text-xs text-dark-text-secondary" title="Invites remaining">
                      <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                      </svg>
                      {u.is_admin ? '∞' : u.invite_count}
                    </div>

                    <div
                      className="flex items-center gap-2 flex-shrink-0"
                      onClick={(e) => toggleAdminStatus(e, u.id, u.is_admin)}
                      role="switch"
                      aria-checked={u.is_admin}
                      aria-label={`Admin status for ${u.email}`}
                    >
                      <span className="text-xs text-dark-text-secondary hidden sm:inline">Admin</span>
                      <div className={`relative w-9 h-5 rounded-full transition-colors ${u.is_admin ? 'bg-purple-500' : 'bg-dark-bg-tertiary'}`}>
                        <div className={`absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white transition-transform ${u.is_admin ? 'translate-x-4' : ''}`} />
                      </div>
                    </div>
                  </button>

                  {/* Expanded Panel */}
                  {expandedUserId === u.id && (
                    <div className="border-t border-dark-border-subtle bg-dark-bg-primary/50 px-5 py-4 space-y-5">
                      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
                        <div className="bg-dark-bg-secondary rounded-lg p-3 border border-dark-border-subtle">
                          <p className="text-xs text-dark-text-tertiary">Logins</p>
                          <p className="text-lg font-semibold text-dark-text-primary">{u.login_count}</p>
                        </div>
                        <div className="bg-dark-bg-secondary rounded-lg p-3 border border-dark-border-subtle">
                          <p className="text-xs text-dark-text-tertiary">Failed Attempts</p>
                          <p className={`text-lg font-semibold ${u.failed_attempts > 0 ? 'text-red-400' : 'text-dark-text-primary'}`}>
                            {u.failed_attempts}
                          </p>
                        </div>
                        <div className="bg-dark-bg-secondary rounded-lg p-3 border border-dark-border-subtle">
                          <p className="text-xs text-dark-text-tertiary">Last IP</p>
                          <p className="text-sm font-mono text-dark-text-primary truncate">{u.last_login_ip || 'N/A'}</p>
                        </div>
                        <div className="bg-dark-bg-secondary rounded-lg p-3 border border-dark-border-subtle">
                          <p className="text-xs text-dark-text-tertiary">Last Login</p>
                          <p className="text-sm text-dark-text-primary">{u.last_login_at ? formatShortDate(u.last_login_at) : 'Never'}</p>
                        </div>
                      </div>

                      <div className="flex items-center gap-3 bg-dark-bg-secondary rounded-lg p-3 border border-dark-border-subtle">
                        <label className="text-sm text-dark-text-secondary flex-shrink-0">Invite count:</label>
                        {u.is_admin ? (
                          <span className="text-sm text-purple-400 font-medium">Unlimited (admin)</span>
                        ) : (
                          <>
                            <input
                              type="number"
                              min={0}
                              value={editingInvites[u.id] !== undefined ? editingInvites[u.id] : u.invite_count}
                              onChange={(e) => setEditingInvites(prev => ({ ...prev, [u.id]: parseInt(e.target.value) || 0 }))}
                              onClick={(e) => e.stopPropagation()}
                              className="w-20 px-2 py-1 text-sm bg-dark-bg-primary border border-dark-border-subtle rounded text-dark-text-primary focus:outline-none focus:border-primary-500"
                            />
                            {editingInvites[u.id] !== undefined && editingInvites[u.id] !== u.invite_count && (
                              <button
                                onClick={(e) => handleSaveInvites(e, u.id)}
                                disabled={savingInvites === u.id}
                                className="px-3 py-1 text-xs font-medium bg-primary-500 text-white rounded hover:bg-primary-600 transition-colors disabled:opacity-50"
                              >
                                {savingInvites === u.id ? 'Saving...' : 'Save'}
                              </button>
                            )}
                          </>
                        )}
                      </div>

                      <div>
                        <h3 className="text-sm font-medium text-dark-text-secondary mb-2">Recent Activity</h3>
                        {activityLoading === u.id ? (
                          <div className="text-center py-6">
                            <div className="inline-block animate-spin rounded-full h-5 w-5 border-b-2 border-primary-500"></div>
                          </div>
                        ) : !activities[u.id] || activities[u.id].length === 0 ? (
                          <p className="text-sm text-dark-text-tertiary py-3">No activity recorded</p>
                        ) : (
                          <div className="space-y-1.5 max-h-64 overflow-y-auto">
                            {activities[u.id].map((a) => (
                              <div key={a.id} className="flex items-center gap-3 py-1.5 px-3 rounded bg-dark-bg-secondary border border-dark-border-subtle text-sm">
                                <span className={`inline-flex px-2 py-0.5 rounded text-xs font-medium border ${getActivityBadgeClass(a.activity_type)}`}>
                                  {a.activity_type.replace('_', ' ')}
                                </span>
                                <span className="text-dark-text-tertiary text-xs flex-1">
                                  {a.ip_address && <span className="font-mono">{a.ip_address}</span>}
                                </span>
                                <span className="text-xs text-dark-text-tertiary flex-shrink-0">{formatDate(a.created_at)}</span>
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}

          {/* Email Provider Tab */}
          {activeTab === 'email' && (
            <div className="bg-dark-bg-secondary rounded-lg border border-dark-border-subtle p-6 space-y-6">
              <div>
                <h2 className="text-lg font-semibold text-dark-text-primary">Email Provider (Brevo)</h2>
                <p className="text-sm text-dark-text-secondary mt-1">
                  Configure Brevo to send invite emails and project notifications. The connection is checked daily.
                </p>
              </div>

              {emailSuccess && (
                <div className="p-3 bg-emerald-500/10 border border-emerald-500/30 text-emerald-400 text-sm rounded-lg">
                  {emailSuccess}
                </div>
              )}
              {emailError && (
                <div className="p-3 bg-red-500/10 border border-red-500/30 text-red-400 text-sm rounded-lg">
                  {emailError}
                </div>
              )}

              {/* Current status */}
              {emailProvider && (
                <div className="flex items-center justify-between bg-dark-bg-primary rounded-lg p-4 border border-dark-border-subtle">
                  <div className="flex items-center gap-3">
                    <span className={`inline-flex px-2.5 py-1 rounded text-xs font-medium border ${getStatusBadge(emailProvider.status)}`}>
                      {emailProvider.status}
                    </span>
                    <div className="text-sm">
                      <span className="text-dark-text-primary">{emailProvider.sender_name}</span>
                      <span className="text-dark-text-tertiary"> &lt;{emailProvider.sender_email}&gt;</span>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={handleTestEmailProvider}
                      disabled={isTestingEmail}
                      className="px-3 py-1.5 text-xs font-medium bg-dark-bg-secondary text-dark-text-primary border border-dark-border-subtle rounded hover:bg-dark-bg-tertiary/50 transition-colors disabled:opacity-50"
                    >
                      {isTestingEmail ? 'Testing...' : 'Test Connection'}
                    </button>
                    <button
                      onClick={handleDeleteEmailProvider}
                      disabled={isDeletingEmail}
                      className="px-3 py-1.5 text-xs font-medium text-red-400 border border-red-500/30 rounded hover:bg-red-500/10 transition-colors disabled:opacity-50"
                    >
                      {isDeletingEmail ? 'Removing...' : 'Remove'}
                    </button>
                  </div>
                </div>
              )}

              {/* Status details */}
              {emailProvider && (emailProvider.last_checked_at || emailProvider.last_error) && (
                <div className="text-xs text-dark-text-tertiary space-y-1">
                  {emailProvider.last_checked_at && (
                    <p>Last checked: {formatDate(emailProvider.last_checked_at)}</p>
                  )}
                  {emailProvider.last_error && (
                    <p className="text-red-400">Last error: {emailProvider.last_error}</p>
                  )}
                </div>
              )}

              {/* Config form */}
              <form onSubmit={handleSaveEmailProvider} className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-dark-text-secondary mb-1">
                    API Key {emailProvider && <span className="text-dark-text-tertiary">(current: {emailProvider.api_key})</span>}
                  </label>
                  <input
                    type="password"
                    value={emailApiKey}
                    onChange={(e) => setEmailApiKey(e.target.value)}
                    placeholder={emailProvider ? 'Enter new key to update' : 'xkeysib-...'}
                    className="w-full px-3 py-2 text-sm bg-dark-bg-primary border border-dark-border-subtle rounded-lg text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:border-primary-500"
                    required={!emailProvider}
                  />
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-dark-text-secondary mb-1">Sender Name</label>
                    <input
                      type="text"
                      value={emailSenderName}
                      onChange={(e) => setEmailSenderName(e.target.value)}
                      placeholder="TaskAI"
                      className="w-full px-3 py-2 text-sm bg-dark-bg-primary border border-dark-border-subtle rounded-lg text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:border-primary-500"
                      required
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-dark-text-secondary mb-1">Sender Email</label>
                    <input
                      type="email"
                      value={emailSenderEmail}
                      onChange={(e) => setEmailSenderEmail(e.target.value)}
                      placeholder="noreply@yourdomain.com"
                      className="w-full px-3 py-2 text-sm bg-dark-bg-primary border border-dark-border-subtle rounded-lg text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:border-primary-500"
                      required
                    />
                  </div>
                </div>
                <button
                  type="submit"
                  disabled={isSavingEmail}
                  className="px-4 py-2 text-sm font-medium bg-primary-500 text-white rounded-lg hover:bg-primary-600 transition-colors disabled:opacity-50"
                >
                  {isSavingEmail ? 'Saving...' : emailProvider ? 'Update Provider' : 'Save Provider'}
                </button>
              </form>
            </div>
          )}

          {/* Backup & Restore Tab */}
          {activeTab === 'backup' && (
            <div className="space-y-6">
              {/* Export Section */}
              <div className="bg-dark-bg-secondary rounded-lg border border-dark-border-subtle p-6">
                <div className="flex items-start justify-between mb-4">
                  <div>
                    <h2 className="text-lg font-semibold text-dark-text-primary mb-2">Export Data</h2>
                    <p className="text-sm text-dark-text-secondary">
                      Download a complete backup of all database data including migration version.
                    </p>
                  </div>
                  <button
                    onClick={async () => {
                      setIsExporting(true)
                      setBackupError('')
                      setBackupStatus('')
                      try {
                        const response = await fetch(`${API_URL}/api/admin/backup/export`, {
                          method: 'GET',
                          headers: {
                            'Authorization': `Bearer ${localStorage.getItem('auth_token')}`
                          }
                        })
                        if (!response.ok) {
                          const error = await response.json()
                          throw new Error(error.error || 'Export failed')
                        }

                        const blob = await response.blob()
                        const url = window.URL.createObjectURL(blob)
                        const a = document.createElement('a')
                        a.href = url
                        a.download = `taskai-backup-${new Date().toISOString().split('T')[0]}.json`
                        document.body.appendChild(a)
                        a.click()
                        window.URL.revokeObjectURL(url)
                        document.body.removeChild(a)

                        setBackupStatus('✅ Export completed successfully')
                      } catch (err: unknown) {
                        setBackupError(err instanceof Error ? err.message : 'Export failed')
                      } finally {
                        setIsExporting(false)
                      }
                    }}
                    disabled={isExporting}
                    className="px-4 py-2 text-sm font-medium bg-primary-500 text-white rounded-lg hover:bg-primary-600 transition-colors disabled:opacity-50 flex items-center gap-2"
                  >
                    {isExporting ? (
                      <>
                        <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white"></div>
                        Exporting...
                      </>
                    ) : (
                      <>
                        <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                        </svg>
                        Export Data
                      </>
                    )}
                  </button>
                </div>

                <div className="mt-4 p-4 bg-dark-bg-tertiary rounded border border-dark-border-subtle">
                  <h3 className="text-sm font-medium text-dark-text-primary mb-2">Export includes:</h3>
                  <ul className="text-sm text-dark-text-secondary space-y-1">
                    <li>• All users, teams, and members</li>
                    <li>• All projects, tasks, and comments</li>
                    <li>• Tags, sprints, and swim lanes</li>
                    <li>• User activity and invitations</li>
                    <li>• API keys and provider settings</li>
                    <li>• Migration version for compatibility check</li>
                  </ul>
                </div>
              </div>

              {/* Import Section */}
              <div className="bg-dark-bg-secondary rounded-lg border border-dark-border-subtle p-6">
                <div className="mb-4">
                  <h2 className="text-lg font-semibold text-dark-text-primary mb-2">Import Data</h2>
                  <p className="text-sm text-dark-text-secondary mb-4">
                    Restore data from a backup file. The migration version must match your current database.
                  </p>
                  <div className="p-4 bg-orange-500/10 border border-orange-500/30 rounded-lg">
                    <p className="text-sm text-orange-400 font-medium">⚠️ Warning</p>
                    <p className="text-sm text-orange-300 mt-1">
                      Importing will replace existing data with the backup. Make sure to export current data first.
                    </p>
                  </div>
                </div>

                <input
                  type="file"
                  accept=".json"
                  id="backup-file-input"
                  className="hidden"
                  onChange={async (e) => {
                    const file = e.target.files?.[0]
                    if (!file) return

                    setIsImporting(true)
                    setBackupError('')
                    setBackupStatus('')

                    try {
                      const fileContent = await file.text()

                      const response = await fetch(`${API_URL}/api/admin/backup/import`, {
                        method: 'POST',
                        headers: {
                          'Authorization': `Bearer ${localStorage.getItem('auth_token')}`,
                          'Content-Type': 'application/json'
                        },
                        body: fileContent
                      })

                      if (!response.ok) {
                        const error = await response.json()
                        throw new Error(error.error || 'Import failed')
                      }

                      const result = await response.json()
                      setBackupStatus(`✅ Import completed: ${result.rows} rows imported`)

                      // Reload page after successful import
                      setTimeout(() => window.location.reload(), 2000)
                    } catch (err: unknown) {
                      setBackupError(err instanceof Error ? err.message : 'Import failed')
                    } finally {
                      setIsImporting(false)
                      e.target.value = '' // Reset input
                    }
                  }}
                />

                <button
                  onClick={() => document.getElementById('backup-file-input')?.click()}
                  disabled={isImporting}
                  className="px-4 py-2 text-sm font-medium bg-orange-500 text-white rounded-lg hover:bg-orange-600 transition-colors disabled:opacity-50 flex items-center gap-2"
                >
                  {isImporting ? (
                    <>
                      <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white"></div>
                      Importing...
                    </>
                  ) : (
                    <>
                      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
                      </svg>
                      Choose Backup File
                    </>
                  )}
                </button>
              </div>

              {/* Status Messages */}
              {backupStatus && (
                <div className="p-4 bg-emerald-500/10 border border-emerald-500/30 rounded-lg">
                  <p className="text-sm text-emerald-400">{backupStatus}</p>
                </div>
              )}

              {backupError && (
                <div className="p-4 bg-red-500/10 border border-red-500/30 rounded-lg">
                  <p className="text-sm text-red-400">{backupError}</p>
                </div>
              )}
            </div>
          )}

          {/* System Tab */}
          {activeTab === 'system' && (
            <div className="space-y-6">
              {/* Version Info Card */}
              <div className="bg-dark-bg-secondary rounded-lg border border-dark-border-subtle p-6">
                <div className="flex items-center justify-between mb-6">
                  <h2 className="text-lg font-semibold text-dark-text-primary">System Information</h2>
                  <button
                    onClick={loadVersionInfo}
                    disabled={versionLoading}
                    className="px-3 py-1.5 text-xs font-medium bg-dark-bg-tertiary text-dark-text-primary border border-dark-border-subtle rounded hover:bg-dark-bg-tertiary/70 transition-colors disabled:opacity-50"
                  >
                    {versionLoading ? 'Refreshing...' : 'Refresh'}
                  </button>
                </div>

                {versionInfo ? (
                  <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
                    {/* Backend Version */}
                    <div className="space-y-3">
                      <h3 className="text-sm font-medium text-dark-text-secondary uppercase tracking-wide">Backend API</h3>
                      <div className="space-y-2 text-sm">
                        <div className="flex justify-between">
                          <span className="text-dark-text-tertiary">Version</span>
                          <span className="text-dark-text-primary font-mono font-semibold">{versionInfo.version}</span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-dark-text-tertiary">Git Commit</span>
                          <span className="text-dark-text-primary font-mono text-xs">
                            {versionInfo.git_commit.substring(0, 8)}
                          </span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-dark-text-tertiary">Build Time</span>
                          <span className="text-dark-text-primary text-xs">{versionInfo.build_time}</span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-dark-text-tertiary">Go Version</span>
                          <span className="text-dark-text-primary font-mono text-xs">{versionInfo.go_version}</span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-dark-text-tertiary">Platform</span>
                          <span className="text-dark-text-primary font-mono text-xs">{versionInfo.platform}</span>
                        </div>
                      </div>
                    </div>

                    {/* Frontend Version */}
                    <div className="space-y-3">
                      <h3 className="text-sm font-medium text-dark-text-secondary uppercase tracking-wide">Frontend Web</h3>
                      <div className="space-y-2 text-sm">
                        <div className="flex justify-between">
                          <span className="text-dark-text-tertiary">Version</span>
                          <span className="text-dark-text-primary font-mono font-semibold">{frontendVersion.version}</span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-dark-text-tertiary">Git Commit</span>
                          <span className="text-dark-text-primary font-mono text-xs">
                            {frontendVersion.gitCommit.substring(0, 8)}
                          </span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-dark-text-tertiary">Build Time</span>
                          <span className="text-dark-text-primary text-xs">{frontendVersion.buildTime}</span>
                        </div>
                      </div>
                    </div>

                    {/* Database & Environment */}
                    <div className="space-y-3">
                      <h3 className="text-sm font-medium text-dark-text-secondary uppercase tracking-wide">Database & Environment</h3>
                      <div className="space-y-2 text-sm">
                        <div className="flex justify-between">
                          <span className="text-dark-text-tertiary">DB Migration Version</span>
                          <span className="text-dark-text-primary font-mono font-semibold text-primary-400">
                            {versionInfo.db_version} migrations
                          </span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-dark-text-tertiary">Database Type</span>
                          <span className={`inline-flex px-2 py-0.5 rounded text-xs font-medium ${
                            versionInfo.db_driver === 'postgres'
                              ? 'bg-green-500/10 text-green-400 border border-green-500/30'
                              : 'bg-gray-500/10 text-gray-400 border border-gray-500/30'
                          }`}>
                            {versionInfo.db_driver === 'postgres' ? 'PostgreSQL' : 'SQLite'}
                          </span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-dark-text-tertiary">Environment</span>
                          <span className={`inline-flex px-2 py-0.5 rounded text-xs font-medium ${
                            versionInfo.environment === 'production'
                              ? 'bg-red-500/10 text-red-400 border border-red-500/30'
                              : versionInfo.environment === 'staging'
                              ? 'bg-yellow-500/10 text-yellow-400 border border-yellow-500/30'
                              : 'bg-blue-500/10 text-blue-400 border border-blue-500/30'
                          }`}>
                            {versionInfo.environment}
                          </span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-dark-text-tertiary">Server Time</span>
                          <span className="text-dark-text-primary text-xs">
                            {new Date(versionInfo.server_time).toLocaleString()}
                          </span>
                        </div>
                      </div>
                    </div>
                  </div>
                ) : (
                  <div className="text-center py-8 text-dark-text-tertiary">
                    {versionLoading ? 'Loading version information...' : 'Failed to load version information'}
                  </div>
                )}
              </div>

              {/* Deployment Info */}
              <div className="bg-dark-bg-secondary rounded-lg border border-dark-border-subtle p-6">
                <h2 className="text-lg font-semibold text-dark-text-primary mb-4">Deployment</h2>
                <div className="space-y-2 text-sm text-dark-text-tertiary">
                  <p>• Deployments are automated via GitHub Actions</p>
                  <p>• Push to main → Staging deployment via webhook</p>
                  <p>• Promote to production: <code className="px-1.5 py-0.5 bg-dark-bg-primary rounded text-xs font-mono text-dark-text-primary">gh workflow run deploy-production.yml -f ref="main"</code></p>
                  <p>• DB migrations run automatically on container startup</p>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
