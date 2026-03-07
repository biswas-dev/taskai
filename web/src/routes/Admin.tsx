import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useAuth } from '../state/AuthContext'
import { api, type EmailProviderResponse, type AdminInvitation, type BackupStatus, type BackupSettings, type BackupRecord } from '../lib/api'
import { version as frontendVersion } from '../lib/version'

// API URL with fallback for production (empty string = relative URL)
const API_URL = import.meta.env.VITE_API_URL || ''

// Types from backend
interface UserWithStats {
  id: number
  email: string
  name?: string
  first_name?: string
  last_name?: string
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

type AdminTab = 'users' | 'invitations' | 'email' | 'backup' | 'system'

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
  const [deletingUserId, setDeletingUserId] = useState<number | null>(null)
  const [deleteConfirm, setDeleteConfirm] = useState<{ id: number; email: string } | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  // Name editing state (per user)
  const [editingName, setEditingName] = useState<Record<number, { firstName: string; lastName: string }>>({})
  const [savingName, setSavingName] = useState<number | null>(null)
  const [nameSuccess, setNameSuccess] = useState<Record<number, string>>({})

  // Password reset state (per user)
  const [resetPasswordModal, setResetPasswordModal] = useState<{ id: number; email: string } | null>(null)
  const [resetPasswordValue, setResetPasswordValue] = useState('')
  const [resetPasswordLoading, setResetPasswordLoading] = useState(false)
  const [resetPasswordError, setResetPasswordError] = useState('')
  const [resetPasswordSuccess, setResetPasswordSuccess] = useState('')

  // Invitations state
  const [invitations, setInvitations] = useState<AdminInvitation[]>([])
  const [invitationsLoading, setInvitationsLoading] = useState(false)
  const [invitationsError, setInvitationsError] = useState('')
  const [invStatusFilter, setInvStatusFilter] = useState('pending')
  const [invTypeFilter, setInvTypeFilter] = useState('')
  const [resolvingId, setResolvingId] = useState<string | null>(null) // "team-123" or "project-456"

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

  // Backup subtab
  type BackupSubTab = 'manual' | 'automated'
  const [backupSubTab, setBackupSubTab] = useState<BackupSubTab>('manual')

  // Copy-from-env state
  const [copySourceUrl, setCopySourceUrl] = useState('')
  const [copySourceApiKey, setCopySourceApiKey] = useState('')
  const [isCopying, setIsCopying] = useState(false)
  const [copyStatus, setCopyStatus] = useState('')
  const [copyError, setCopyError] = useState('')

  // Automated (Google Drive) backup state
  const [autoBackupStatus, setAutoBackupStatus] = useState<BackupStatus | null>(null)
  const [autoBackupSettings, setAutoBackupSettings] = useState<BackupSettings | null>(null)
  const [autoBackupHistory, setAutoBackupHistory] = useState<BackupRecord[]>([])
  const [autoBackupCron, setAutoBackupCron] = useState('0 3 * * *')
  const [autoBackupFolderId, setAutoBackupFolderId] = useState('')
  const [autoBackupEnabled, setAutoBackupEnabled] = useState(false)
  const [isAutoBackupAvailable, setIsAutoBackupAvailable] = useState(false)
  const [isLoadingAutoBackup, setIsLoadingAutoBackup] = useState(false)
  const [isTriggeringAutoBackup, setIsTriggeringAutoBackup] = useState(false)
  const [isSavingAutoBackupSettings, setIsSavingAutoBackupSettings] = useState(false)
  const [isDisconnectingAutoBackup, setIsDisconnectingAutoBackup] = useState(false)
  const [autoBackupError, setAutoBackupError] = useState('')
  const [autoBackupSuccess, setAutoBackupSuccess] = useState('')

  // System/version state
  const [versionInfo, setVersionInfo] = useState<VersionInfo | null>(null)
  const [versionLoading, setVersionLoading] = useState(false)

  // Sync activeTab with URL
  useEffect(() => {
    const urlTab = searchParams.get('tab') as AdminTab
    if (urlTab && urlTab !== activeTab) {
      setActiveTab(urlTab)
    }
  }, [searchParams, activeTab])

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

  // Load invitations when the tab becomes active or filters change
  useEffect(() => {
    if (activeTab === 'invitations') {
      loadInvitations(invStatusFilter, invTypeFilter)
    }
  }, [activeTab, invStatusFilter, invTypeFilter])

  // Load automated backup when subtab becomes active
  useEffect(() => {
    if (activeTab === 'backup' && backupSubTab === 'automated') {
      loadAutomatedBackup()
    }
  }, [activeTab, backupSubTab]) // eslint-disable-line react-hooks/exhaustive-deps

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
      setDeleteError(err instanceof Error ? err.message : 'Failed to update admin status')
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
      setDeleteError(err instanceof Error ? err.message : 'Failed to update invites')
    } finally {
      setSavingInvites(null)
    }
  }

  const handleDeleteUser = (e: React.MouseEvent, userId: number, email: string) => {
    e.stopPropagation()
    setDeleteConfirm({ id: userId, email })
  }

  const confirmDeleteUser = async () => {
    if (!deleteConfirm) return
    const { id: userId } = deleteConfirm
    setDeleteConfirm(null)
    setDeleteError(null)
    try {
      setDeletingUserId(userId)
      await api.deleteUser(userId)
      setUsers(prev => prev.filter(u => u.id !== userId))
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Failed to delete user')
    } finally {
      setDeletingUserId(null)
    }
  }

  // === Name editing ===
  const startEditName = (e: React.MouseEvent, u: UserWithStats) => {
    e.stopPropagation()
    setEditingName(prev => ({ ...prev, [u.id]: { firstName: u.first_name ?? '', lastName: u.last_name ?? '' } }))
  }

  const handleSaveName = async (e: React.MouseEvent, userId: number) => {
    e.stopPropagation()
    const data = editingName[userId]
    if (!data) return
    setSavingName(userId)
    try {
      const updated = await api.adminUpdateUserProfile(userId, { first_name: data.firstName, last_name: data.lastName })
      setUsers(prev => prev.map(u => u.id === userId ? { ...u, first_name: updated.first_name, last_name: updated.last_name, name: updated.name } : u))
      setEditingName(prev => { const next = { ...prev }; delete next[userId]; return next })
      setNameSuccess(prev => ({ ...prev, [userId]: 'Name updated' }))
      setTimeout(() => setNameSuccess(prev => { const n = { ...prev }; delete n[userId]; return n }), 3000)
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Failed to update name')
    } finally {
      setSavingName(null)
    }
  }

  // === Password reset ===
  const handleAdminResetPassword = async (sendEmail: boolean) => {
    if (!resetPasswordModal) return
    if (!sendEmail && !resetPasswordValue) {
      setResetPasswordError('Enter a password or choose to send email')
      return
    }
    setResetPasswordLoading(true)
    setResetPasswordError('')
    try {
      const result = await api.adminResetPassword(resetPasswordModal.id, {
        send_email: sendEmail,
        password: sendEmail ? undefined : resetPasswordValue,
      })
      setResetPasswordSuccess(result.message)
      setResetPasswordValue('')
    } catch (err) {
      setResetPasswordError(err instanceof Error ? err.message : 'Failed to reset password')
    } finally {
      setResetPasswordLoading(false)
    }
  }

  // === Invitations ===
  const loadInvitations = async (status: string, type: string) => {
    setInvitationsLoading(true)
    setInvitationsError('')
    try {
      const data = await api.adminGetInvitations({
        status: status || undefined,
        type: type || undefined,
      })
      setInvitations(data)
    } catch (err) {
      setInvitationsError(err instanceof Error ? err.message : 'Failed to load invitations')
    } finally {
      setInvitationsLoading(false)
    }
  }

  const handleResolveInvitation = async (inv: AdminInvitation, action: 'accept' | 'reject') => {
    const key = `${inv.type}-${inv.id}`
    setResolvingId(key)
    try {
      if (inv.type === 'team') {
        await api.adminResolveTeamInvitation(inv.id, action)
      } else {
        await api.adminResolveProjectInvitation(inv.id, action)
      }
      await loadInvitations(invStatusFilter, invTypeFilter)
    } catch (err) {
      setInvitationsError(err instanceof Error ? err.message : 'Failed to resolve invitation')
    } finally {
      setResolvingId(null)
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

  // === Copy from environment ===
  const handleCopyFromEnv = async () => {
    if (!copySourceUrl || !copySourceApiKey) {
      setCopyError('Source URL and API key are required')
      return
    }
    if (!confirm('This will overwrite ALL data in this environment with data from the source. Are you sure?')) return
    setIsCopying(true)
    setCopyError('')
    setCopyStatus('')
    try {
      const result = await api.copyFromEnv(copySourceUrl, copySourceApiKey)
      setCopyStatus(`✅ ${result.message} — ${result.rows} rows imported (v${result.version})`)
      setTimeout(() => globalThis.location.reload(), 2000)
    } catch (err) {
      setCopyError(err instanceof Error ? err.message : 'Copy failed')
    } finally {
      setIsCopying(false)
    }
  }

  // === Automated (Google Drive) backup ===
  const loadAutomatedBackup = async () => {
    setIsLoadingAutoBackup(true)
    try {
      const [status, settings, history] = await Promise.all([
        api.getBackupStatus(),
        api.getBackupSettings(),
        api.listBackupHistory(10),
      ])
      setAutoBackupStatus(status)
      setAutoBackupSettings(settings)
      setAutoBackupHistory(history)
      setAutoBackupEnabled(settings.enabled)
      setAutoBackupCron(settings.cron_expression || '0 3 * * *')
      setAutoBackupFolderId(settings.folder_id || '')
      setIsAutoBackupAvailable(true)
    } catch {
      setIsAutoBackupAvailable(false)
    } finally {
      setIsLoadingAutoBackup(false)
    }
  }

  const handleTriggerAutoBackup = async () => {
    setIsTriggeringAutoBackup(true)
    setAutoBackupError('')
    setAutoBackupSuccess('')
    try {
      await api.triggerBackup()
      setAutoBackupSuccess('Backup started — check history in a moment')
      setTimeout(() => loadAutomatedBackup(), 3000)
    } catch (err) {
      setAutoBackupError(err instanceof Error ? err.message : 'Failed to trigger backup')
    } finally {
      setIsTriggeringAutoBackup(false)
    }
  }

  const handleSaveAutoBackupSettings = async (e: React.FormEvent) => {
    e.preventDefault()
    setIsSavingAutoBackupSettings(true)
    setAutoBackupError('')
    setAutoBackupSuccess('')
    try {
      const updated = await api.updateBackupSettings({
        enabled: autoBackupEnabled,
        cron_expression: autoBackupCron,
        folder_id: autoBackupFolderId,
      })
      setAutoBackupSettings(updated)
      setAutoBackupSuccess('Backup settings saved')
    } catch (err) {
      setAutoBackupError(err instanceof Error ? err.message : 'Failed to save settings')
    } finally {
      setIsSavingAutoBackupSettings(false)
    }
  }

  const handleDisconnectAutoBackup = async () => {
    if (!confirm('Disconnect Google Drive? Scheduled backups will be disabled.')) return
    setIsDisconnectingAutoBackup(true)
    setAutoBackupError('')
    try {
      await api.disconnectBackup()
      setAutoBackupSuccess('Google Drive disconnected')
      loadAutomatedBackup()
    } catch (err) {
      setAutoBackupError(err instanceof Error ? err.message : 'Failed to disconnect')
    } finally {
      setIsDisconnectingAutoBackup(false)
    }
  }

  const handleDeleteAutoBackupRecord = async (id: string) => {
    if (!confirm('Delete this backup record and its remote file?')) return
    try {
      await api.deleteBackupRecord(id)
      setAutoBackupHistory(prev => prev.filter(r => r.id !== id))
    } catch (err) {
      setAutoBackupError(err instanceof Error ? err.message : 'Failed to delete record')
    }
  }

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`
  }

  // === System/Version ===
  const loadVersionInfo = async () => {
    try {
      setVersionLoading(true)
      const data = await api.getVersion()
      setVersionInfo(data)
    } catch (err) {
      console.error('Failed to load version info:', err)
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
              onClick={() => handleTabChange('invitations')}
              className={`px-4 py-2 text-sm font-medium rounded-lg transition-colors ${
                activeTab === 'invitations'
                  ? 'bg-primary-500/10 text-primary-400 border border-primary-500/30'
                  : 'text-dark-text-secondary hover:text-dark-text-primary hover:bg-dark-bg-tertiary/30'
              }`}
            >
              Invitations
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
              {deleteError && (
                <div className="flex items-center justify-between p-3 bg-red-500/10 border border-red-500/30 rounded-lg">
                  <span className="text-sm text-red-400">{deleteError}</span>
                  <button onClick={() => setDeleteError(null)} className="text-red-400 hover:text-red-300 ml-3">
                    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                </div>
              )}
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
                      {u.name && u.name !== u.email && (
                        <span className="text-sm font-medium text-dark-text-primary truncate block">{u.name}</span>
                      )}
                      <span className={`truncate block ${u.name && u.name !== u.email ? 'text-xs text-dark-text-tertiary' : 'text-sm font-medium text-dark-text-primary'}`}>{u.email}</span>
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

                    {u.id !== user?.id && (
                      <button
                        onClick={(e) => handleDeleteUser(e, u.id, u.email)}
                        disabled={deletingUserId === u.id}
                        title="Delete user"
                        className="p-1.5 text-dark-text-tertiary hover:text-red-400 hover:bg-red-500/10 rounded transition-colors disabled:opacity-50 flex-shrink-0"
                      >
                        {deletingUserId === u.id ? (
                          <div className="w-4 h-4 animate-spin rounded-full border-b-2 border-red-400" />
                        ) : (
                          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                          </svg>
                        )}
                      </button>
                    )}
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

                      {/* Name editing */}
                      <div className="bg-dark-bg-secondary rounded-lg p-3 border border-dark-border-subtle space-y-2">
                        <div className="flex items-center justify-between">
                          <label className="text-sm text-dark-text-secondary">Display name</label>
                          {!editingName[u.id] && (
                            <button
                              onClick={(e) => startEditName(e, u)}
                              className="text-xs text-primary-400 hover:text-primary-300 transition-colors"
                            >
                              {u.name && u.name !== u.email ? 'Edit' : 'Set name'}
                            </button>
                          )}
                        </div>
                        {editingName[u.id] ? (
                          <div className="flex items-center gap-2" onClick={(e) => e.stopPropagation()}>
                            <input
                              type="text"
                              placeholder="First"
                              value={editingName[u.id].firstName}
                              onChange={(e) => setEditingName(prev => ({ ...prev, [u.id]: { ...prev[u.id], firstName: e.target.value } }))}
                              className="flex-1 px-2 py-1 text-sm bg-dark-bg-primary border border-dark-border-subtle rounded text-dark-text-primary focus:outline-none focus:border-primary-500"
                            />
                            <input
                              type="text"
                              placeholder="Last"
                              value={editingName[u.id].lastName}
                              onChange={(e) => setEditingName(prev => ({ ...prev, [u.id]: { ...prev[u.id], lastName: e.target.value } }))}
                              className="flex-1 px-2 py-1 text-sm bg-dark-bg-primary border border-dark-border-subtle rounded text-dark-text-primary focus:outline-none focus:border-primary-500"
                            />
                            <button
                              onClick={(e) => handleSaveName(e, u.id)}
                              disabled={savingName === u.id}
                              className="px-3 py-1 text-xs font-medium bg-primary-500 text-white rounded hover:bg-primary-600 transition-colors disabled:opacity-50"
                            >
                              {savingName === u.id ? 'Saving…' : 'Save'}
                            </button>
                            <button
                              onClick={(e) => { e.stopPropagation(); setEditingName(prev => { const n = { ...prev }; delete n[u.id]; return n }) }}
                              className="px-2 py-1 text-xs text-dark-text-tertiary hover:text-dark-text-primary rounded border border-dark-border-subtle transition-colors"
                            >
                              Cancel
                            </button>
                          </div>
                        ) : (
                          <p className="text-sm text-dark-text-primary">
                            {nameSuccess[u.id] ? (
                              <span className="text-green-400 text-xs">{nameSuccess[u.id]}</span>
                            ) : u.name && u.name !== u.email ? u.name : (
                              <span className="text-dark-text-tertiary italic">Not set (showing email)</span>
                            )}
                          </p>
                        )}
                      </div>

                      {/* Password reset */}
                      <div className="bg-dark-bg-secondary rounded-lg p-3 border border-dark-border-subtle space-y-2">
                        <div className="flex items-center justify-between">
                          <label className="text-sm text-dark-text-secondary">Password reset</label>
                          <button
                            onClick={(e) => { e.stopPropagation(); setResetPasswordModal({ id: u.id, email: u.email }); setResetPasswordValue(''); setResetPasswordError(''); setResetPasswordSuccess('') }}
                            className="text-xs text-amber-400 hover:text-amber-300 transition-colors"
                          >
                            Reset password
                          </button>
                        </div>
                        <p className="text-xs text-dark-text-tertiary">Send a reset link to the user or set a password directly.</p>
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

          {/* Invitations Tab */}
          {activeTab === 'invitations' && (
            <div className="space-y-4">
              {/* Filters */}
              <div className="flex flex-wrap gap-3 items-center">
                <div className="flex items-center gap-2">
                  <label className="text-xs text-dark-text-tertiary">Status</label>
                  <select
                    value={invStatusFilter}
                    onChange={(e) => setInvStatusFilter(e.target.value)}
                    className="text-sm bg-dark-bg-secondary border border-dark-border-subtle rounded-lg px-3 py-1.5 text-dark-text-primary focus:outline-none focus:border-primary-500"
                  >
                    <option value="">All</option>
                    <option value="pending">Pending</option>
                    <option value="accepted">Accepted</option>
                    <option value="rejected">Rejected</option>
                    <option value="cancelled">Cancelled</option>
                    <option value="withdrawn">Withdrawn</option>
                  </select>
                </div>
                <div className="flex items-center gap-2">
                  <label className="text-xs text-dark-text-tertiary">Type</label>
                  <select
                    value={invTypeFilter}
                    onChange={(e) => setInvTypeFilter(e.target.value)}
                    className="text-sm bg-dark-bg-secondary border border-dark-border-subtle rounded-lg px-3 py-1.5 text-dark-text-primary focus:outline-none focus:border-primary-500"
                  >
                    <option value="">All</option>
                    <option value="team">Team</option>
                    <option value="project">Project</option>
                  </select>
                </div>
                <button
                  onClick={() => loadInvitations(invStatusFilter, invTypeFilter)}
                  disabled={invitationsLoading}
                  className="text-xs px-3 py-1.5 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg text-dark-text-secondary hover:text-dark-text-primary transition-colors disabled:opacity-50"
                >
                  {invitationsLoading ? 'Loading…' : 'Refresh'}
                </button>
              </div>

              {invitationsError && (
                <div className="p-3 bg-red-500/10 border border-red-500/30 rounded-lg text-sm text-red-400">
                  {invitationsError}
                </div>
              )}

              {invitationsLoading ? (
                <div className="space-y-2">
                  {[1,2,3].map(i => (
                    <div key={i} className="h-16 bg-dark-bg-secondary rounded-lg animate-pulse" />
                  ))}
                </div>
              ) : invitations.length === 0 ? (
                <div className="text-center py-16 text-dark-text-tertiary text-sm">
                  No invitations found
                </div>
              ) : (
                <div className="space-y-2">
                  {invitations.map((inv) => {
                    const key = `${inv.type}-${inv.id}`
                    const isResolving = resolvingId === key
                    const isPending = inv.status === 'pending'
                    return (
                      <div key={key} className="bg-dark-bg-secondary border border-dark-border-subtle rounded-lg px-5 py-4">
                        <div className="flex items-start gap-4">
                          <div className="flex-1 min-w-0 space-y-1">
                            <div className="flex items-center gap-2 flex-wrap">
                              <span className={`inline-flex px-2 py-0.5 rounded text-xs font-medium border ${
                                inv.type === 'team'
                                  ? 'bg-blue-500/10 text-blue-400 border-blue-500/30'
                                  : 'bg-violet-500/10 text-violet-400 border-violet-500/30'
                              }`}>
                                {inv.type}
                              </span>
                              <span className={`inline-flex px-2 py-0.5 rounded text-xs font-medium border ${
                                inv.status === 'pending' ? 'bg-yellow-500/10 text-yellow-400 border-yellow-500/30' :
                                inv.status === 'accepted' ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/30' :
                                inv.status === 'rejected' ? 'bg-red-500/10 text-red-400 border-red-500/30' :
                                'bg-gray-500/10 text-gray-400 border-gray-500/30'
                              }`}>
                                {inv.status}
                              </span>
                              {inv.role && (
                                <span className="text-xs text-dark-text-tertiary">role: {inv.role}</span>
                              )}
                            </div>
                            <div className="text-sm text-dark-text-primary font-medium truncate">
                              {inv.context}
                            </div>
                            <div className="text-xs text-dark-text-secondary">
                              <span className="text-dark-text-tertiary">From: </span>
                              <span>{inv.inviter_name}</span>
                              <span className="text-dark-text-tertiary mx-1">→</span>
                              <span>{inv.invitee_name}</span>
                              {inv.invitee_email !== inv.invitee_name && (
                                <span className="text-dark-text-tertiary ml-1">({inv.invitee_email})</span>
                              )}
                            </div>
                            <div className="text-xs text-dark-text-tertiary">
                              {new Date(inv.created_at).toLocaleString()}
                            </div>
                          </div>

                          {isPending && (
                            <div className="flex items-center gap-2 flex-shrink-0">
                              <button
                                onClick={() => handleResolveInvitation(inv, 'accept')}
                                disabled={isResolving || (inv.type === 'team' && inv.invitee_id === null)}
                                title={inv.type === 'team' && inv.invitee_id === null ? 'Cannot accept: user has not registered yet' : 'Force accept'}
                                className="px-3 py-1.5 text-xs font-medium bg-emerald-500/10 text-emerald-400 border border-emerald-500/30 rounded-lg hover:bg-emerald-500/20 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                              >
                                {isResolving ? '…' : 'Accept'}
                              </button>
                              <button
                                onClick={() => handleResolveInvitation(inv, 'reject')}
                                disabled={isResolving}
                                className="px-3 py-1.5 text-xs font-medium bg-red-500/10 text-red-400 border border-red-500/30 rounded-lg hover:bg-red-500/20 transition-colors disabled:opacity-40"
                              >
                                {isResolving ? '…' : 'Reject'}
                              </button>
                            </div>
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          )}

          {/* Backup & Restore Tab */}
          {activeTab === 'backup' && (
            <div className="space-y-6">
              {/* Subtab switcher */}
              <div className="flex border-b border-dark-border-subtle">
                {(['manual', 'automated'] as const).map(sub => (
                  <button key={sub} onClick={() => setBackupSubTab(sub)}
                    className={`px-5 py-2.5 text-sm font-medium border-b-2 transition-colors ${
                      backupSubTab === sub
                        ? 'border-primary-500 text-primary-400'
                        : 'border-transparent text-dark-text-secondary hover:text-dark-text-primary'
                    }`}>
                    {sub === 'manual' ? 'Manual' : 'Automated (Google Drive)'}
                  </button>
                ))}
              </div>

              {/* Manual subtab */}
              {backupSubTab === 'manual' && (
                <div className="space-y-6">
                  {/* Copy from environment — hidden on production */}
                  {versionInfo?.environment !== 'production' && (
                    <div className="bg-dark-bg-secondary rounded-lg border border-dark-border-subtle p-6">
                      <h2 className="text-lg font-semibold text-dark-text-primary mb-1">Copy from Environment</h2>
                      <p className="text-sm text-dark-text-secondary mb-4">
                        Overwrite this environment's database with data from another environment.
                      </p>
                      <div className="p-4 bg-orange-500/10 border border-orange-500/30 rounded-lg mb-4">
                        <p className="text-sm text-orange-400 font-medium">⚠️ Warning</p>
                        <p className="text-sm text-orange-300 mt-1">
                          This will overwrite <strong>all data</strong> in this environment with data from the source. This action cannot be undone.
                        </p>
                      </div>
                      <div className="space-y-4">
                        <div>
                          <label className="block text-sm font-medium text-dark-text-secondary mb-1">Source Environment URL</label>
                          <div className="flex flex-wrap gap-2 mb-2">
                            {versionInfo?.environment === 'staging' && (
                              <>
                                <button type="button" onClick={() => setCopySourceUrl('https://taskai.cc')}
                                  className={`px-3 py-1.5 text-xs rounded border transition-colors ${copySourceUrl === 'https://taskai.cc' ? 'border-primary-500 text-primary-400 bg-primary-500/10' : 'border-dark-border-subtle text-dark-text-secondary hover:text-dark-text-primary'}`}>
                                  Production
                                </button>
                                <button type="button" onClick={() => setCopySourceUrl('https://uat.taskai.cc')}
                                  className={`px-3 py-1.5 text-xs rounded border transition-colors ${copySourceUrl === 'https://uat.taskai.cc' ? 'border-primary-500 text-primary-400 bg-primary-500/10' : 'border-dark-border-subtle text-dark-text-secondary hover:text-dark-text-primary'}`}>
                                  UAT
                                </button>
                              </>
                            )}
                            {versionInfo?.environment === 'uat' && (
                              <>
                                <button type="button" onClick={() => setCopySourceUrl('https://taskai.cc')}
                                  className={`px-3 py-1.5 text-xs rounded border transition-colors ${copySourceUrl === 'https://taskai.cc' ? 'border-primary-500 text-primary-400 bg-primary-500/10' : 'border-dark-border-subtle text-dark-text-secondary hover:text-dark-text-primary'}`}>
                                  Production
                                </button>
                                <button type="button" onClick={() => setCopySourceUrl('https://staging.taskai.cc')}
                                  className={`px-3 py-1.5 text-xs rounded border transition-colors ${copySourceUrl === 'https://staging.taskai.cc' ? 'border-primary-500 text-primary-400 bg-primary-500/10' : 'border-dark-border-subtle text-dark-text-secondary hover:text-dark-text-primary'}`}>
                                  Staging
                                </button>
                              </>
                            )}
                          </div>
                          <input
                            type="text"
                            value={copySourceUrl}
                            onChange={e => setCopySourceUrl(e.target.value)}
                            placeholder="https://taskai.cc"
                            className="w-full px-3 py-2 bg-dark-bg-primary border border-dark-border-subtle rounded text-sm text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:border-dark-accent-primary"
                          />
                        </div>
                        <div>
                          <label className="block text-sm font-medium text-dark-text-secondary mb-1">Source API Key</label>
                          <input
                            type="password"
                            value={copySourceApiKey}
                            onChange={e => setCopySourceApiKey(e.target.value)}
                            placeholder="tai_..."
                            className="w-full px-3 py-2 bg-dark-bg-primary border border-dark-border-subtle rounded text-sm text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:border-dark-accent-primary"
                          />
                          <p className="mt-1 text-xs text-dark-text-tertiary">Use an admin API key from the source environment</p>
                        </div>
                        <button
                          onClick={handleCopyFromEnv}
                          disabled={isCopying || !copySourceUrl || !copySourceApiKey}
                          className="px-4 py-2 text-sm font-medium bg-orange-600 text-white rounded-lg hover:bg-orange-500 transition-colors disabled:opacity-50 flex items-center gap-2"
                        >
                          {isCopying ? (
                            <><div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white"></div>Copying...</>
                          ) : 'Copy Database'}
                        </button>
                        {copyStatus && <div className="p-3 bg-emerald-500/10 border border-emerald-500/30 rounded text-sm text-emerald-400">{copyStatus}</div>}
                        {copyError && <div className="p-3 bg-red-500/10 border border-red-500/30 rounded text-sm text-red-400">{copyError}</div>}
                      </div>
                    </div>
                  )}

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
                            const url = globalThis.URL.createObjectURL(blob)
                            const a = document.createElement('a')
                            a.href = url
                            a.download = `taskai-backup-${new Date().toISOString().split('T')[0]}.json`
                            document.body.appendChild(a)
                            a.click()
                            globalThis.URL.revokeObjectURL(url)
                            a.remove()

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
                          setTimeout(() => globalThis.location.reload(), 2000)
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

              {/* Automated subtab — Google Drive backup */}
              {backupSubTab === 'automated' && (
                <div className="space-y-6">
                  {isLoadingAutoBackup && (
                    <div className="animate-pulse space-y-4">
                      <div className="h-32 bg-dark-bg-secondary rounded-lg"></div>
                    </div>
                  )}
                  {!isLoadingAutoBackup && !isAutoBackupAvailable && (
                    <div className="bg-dark-bg-secondary rounded-lg border border-dark-border-subtle p-6">
                      <p className="text-sm text-dark-text-tertiary">Google Drive backup is not configured on this server. Set <code className="font-mono text-xs bg-dark-bg-tertiary px-1 rounded">GOOGLE_CLIENT_ID</code> and ensure the database is PostgreSQL.</p>
                    </div>
                  )}
                  {!isLoadingAutoBackup && isAutoBackupAvailable && (
                    <div className="bg-dark-bg-secondary rounded-lg border border-dark-border-subtle p-6">
                      <div className="flex items-center gap-3 mb-1">
                        <h2 className="text-lg font-semibold text-dark-text-primary">Google Drive Backup</h2>
                        {autoBackupStatus?.provider_connected ? (
                          <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium bg-green-500/15 text-green-400 border border-green-500/30">
                            <span className="w-1.5 h-1.5 rounded-full bg-green-400"></span>
                            Connected{autoBackupStatus.connected_email ? ` · ${autoBackupStatus.connected_email}` : ''}
                          </span>
                        ) : (
                          <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium bg-dark-bg-tertiary text-dark-text-secondary border border-dark-border-subtle">
                            Not connected
                          </span>
                        )}
                      </div>
                      <p className="text-sm text-dark-text-secondary mb-6">
                        Scheduled database backups to Google Drive with automatic retention policies
                      </p>

                      {autoBackupSuccess && (
                        <div className="mb-4 p-3 bg-green-500/10 border border-green-500/30 rounded text-sm text-green-400">{autoBackupSuccess}</div>
                      )}
                      {autoBackupError && (
                        <div className="mb-4 p-3 bg-red-500/10 border border-red-500/30 rounded text-sm text-red-400">{autoBackupError}</div>
                      )}

                      {/* Connect / action row */}
                      <div className="flex flex-wrap items-center gap-3 mb-6">
                        {!autoBackupStatus?.provider_connected ? (
                          <a
                            href="/api/admin/backup/oauth/start"
                            className="inline-flex items-center gap-2 px-4 py-2 bg-green-600 hover:bg-green-500 text-white rounded text-sm font-medium transition-colors"
                          >
                            <svg className="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
                              <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" />
                              <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" />
                              <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" />
                              <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" />
                            </svg>
                            Connect Google Drive
                          </a>
                        ) : (
                          <>
                            <button
                              onClick={handleTriggerAutoBackup}
                              disabled={isTriggeringAutoBackup || autoBackupStatus?.running}
                              className="px-4 py-2 bg-green-600 hover:bg-green-500 disabled:opacity-50 text-white rounded text-sm font-medium transition-colors"
                            >
                              {isTriggeringAutoBackup || autoBackupStatus?.running ? 'Backing up…' : 'Run Backup Now'}
                            </button>
                            <button
                              onClick={handleDisconnectAutoBackup}
                              disabled={isDisconnectingAutoBackup}
                              className="px-4 py-2 bg-dark-bg-tertiary hover:bg-red-500/20 text-dark-text-secondary hover:text-red-400 rounded text-sm font-medium transition-colors"
                            >
                              {isDisconnectingAutoBackup ? 'Disconnecting…' : 'Disconnect'}
                            </button>
                          </>
                        )}
                        {autoBackupStatus?.next_run && (
                          <span className="text-xs text-dark-text-tertiary">
                            Next: {new Date(autoBackupStatus.next_run).toLocaleString()}
                          </span>
                        )}
                      </div>

                      {/* Settings form — only when connected */}
                      {autoBackupStatus?.provider_connected && (
                        <form onSubmit={handleSaveAutoBackupSettings} className="space-y-4 mb-6">
                          <div className="flex items-center gap-3">
                            <label className="relative inline-flex items-center cursor-pointer">
                              <input
                                type="checkbox"
                                checked={autoBackupEnabled}
                                onChange={e => setAutoBackupEnabled(e.target.checked)}
                                className="sr-only peer"
                              />
                              <div className="w-9 h-5 bg-dark-bg-tertiary peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-0.5 after:left-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-green-600"></div>
                            </label>
                            <span className="text-sm text-dark-text-primary">Enable scheduled backups</span>
                          </div>

                          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                            <div>
                              <label className="block text-sm font-medium text-dark-text-secondary mb-1">Schedule (cron)</label>
                              <input
                                type="text"
                                value={autoBackupCron}
                                onChange={e => setAutoBackupCron(e.target.value)}
                                placeholder="0 3 * * *"
                                className="w-full px-3 py-2 bg-dark-bg-primary border border-dark-border-subtle rounded text-sm text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:border-dark-accent-primary font-mono"
                              />
                              <p className="mt-1 text-xs text-dark-text-tertiary">Default: daily at 3 AM UTC</p>
                            </div>
                            <div>
                              <label className="block text-sm font-medium text-dark-text-secondary mb-1">Google Drive Folder ID <span className="text-dark-text-tertiary">(optional)</span></label>
                              <input
                                type="text"
                                value={autoBackupFolderId}
                                onChange={e => setAutoBackupFolderId(e.target.value)}
                                placeholder="Leave blank for root"
                                className="w-full px-3 py-2 bg-dark-bg-primary border border-dark-border-subtle rounded text-sm text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:border-dark-accent-primary"
                              />
                            </div>
                          </div>

                          {autoBackupSettings && (
                            <div className="text-xs text-dark-text-tertiary">
                              Retention: keep all for {autoBackupSettings.retention.full_days}d, every-other-day for {autoBackupSettings.retention.alternate_days}d, weekly for {autoBackupSettings.retention.weekly_days}d
                            </div>
                          )}

                          <button
                            type="submit"
                            disabled={isSavingAutoBackupSettings}
                            className="px-4 py-2 bg-dark-accent-primary hover:bg-dark-accent-primary/80 disabled:opacity-50 text-white rounded text-sm font-medium transition-colors"
                          >
                            {isSavingAutoBackupSettings ? 'Saving…' : 'Save Settings'}
                          </button>
                        </form>
                      )}

                      {/* Backup history */}
                      {autoBackupHistory.length > 0 && (
                        <div>
                          <h3 className="text-sm font-medium text-dark-text-secondary mb-3">Recent Backups</h3>
                          <div className="rounded border border-dark-border-subtle overflow-hidden">
                            <table className="w-full text-sm">
                              <thead className="bg-dark-bg-tertiary">
                                <tr>
                                  <th className="px-3 py-2 text-left text-xs font-medium text-dark-text-tertiary">Status</th>
                                  <th className="px-3 py-2 text-left text-xs font-medium text-dark-text-tertiary">Started</th>
                                  <th className="px-3 py-2 text-left text-xs font-medium text-dark-text-tertiary">Size</th>
                                  <th className="px-3 py-2 text-left text-xs font-medium text-dark-text-tertiary">Trigger</th>
                                  <th className="px-3 py-2"></th>
                                </tr>
                              </thead>
                              <tbody className="divide-y divide-dark-border-subtle">
                                {autoBackupHistory.map(record => (
                                  <tr key={record.id} className="hover:bg-dark-bg-tertiary/50">
                                    <td className="px-3 py-2">
                                      <span className={`inline-flex items-center gap-1 text-xs font-medium ${
                                        record.status === 'success' ? 'text-green-400' :
                                        record.status === 'failed' ? 'text-red-400' : 'text-yellow-400'
                                      }`}>
                                        {record.status === 'success' ? '✓' : record.status === 'failed' ? '✗' : '…'}
                                        {record.status}
                                      </span>
                                    </td>
                                    <td className="px-3 py-2 text-dark-text-secondary text-xs">
                                      {new Date(record.started_at).toLocaleString()}
                                    </td>
                                    <td className="px-3 py-2 text-dark-text-secondary text-xs">
                                      {record.size_bytes > 0 ? formatBytes(record.size_bytes) : '—'}
                                    </td>
                                    <td className="px-3 py-2 text-dark-text-tertiary text-xs">{record.triggered_by}</td>
                                    <td className="px-3 py-2 text-right">
                                      <button
                                        onClick={() => handleDeleteAutoBackupRecord(record.id)}
                                        className="text-dark-text-tertiary hover:text-red-400 transition-colors p-1"
                                        title="Delete record"
                                      >
                                        <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                                        </svg>
                                      </button>
                                    </td>
                                  </tr>
                                ))}
                              </tbody>
                            </table>
                          </div>
                        </div>
                      )}

                      {!isLoadingAutoBackup && autoBackupStatus?.provider_connected && autoBackupHistory.length === 0 && (
                        <p className="text-sm text-dark-text-tertiary">No backups yet. Run your first backup manually above.</p>
                      )}
                    </div>
                  )}
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

      {/* Password Reset Modal */}
      {resetPasswordModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
          <div className="bg-dark-bg-secondary border border-dark-border-subtle rounded-xl shadow-2xl p-6 max-w-md w-full mx-4 space-y-4">
            <h3 className="text-base font-semibold text-dark-text-primary">Reset password for {resetPasswordModal.email}</h3>
            {resetPasswordSuccess ? (
              <div className="p-3 bg-green-500/10 border border-green-500/30 rounded-lg text-sm text-green-300">{resetPasswordSuccess}</div>
            ) : (
              <>
                {resetPasswordError && (
                  <div className="p-3 bg-red-500/10 border border-red-500/30 rounded-lg text-sm text-red-300">{resetPasswordError}</div>
                )}
                <div className="space-y-3">
                  <div>
                    <label className="text-xs text-dark-text-secondary block mb-1.5">Set password directly</label>
                    <input
                      type="password"
                      placeholder="New password (min 8 chars)"
                      value={resetPasswordValue}
                      onChange={(e) => setResetPasswordValue(e.target.value)}
                      className="w-full px-3 py-2 text-sm bg-dark-bg-primary border border-dark-border-subtle rounded text-dark-text-primary focus:outline-none focus:border-primary-500"
                    />
                  </div>
                  <button
                    onClick={() => handleAdminResetPassword(false)}
                    disabled={resetPasswordLoading || !resetPasswordValue}
                    className="w-full px-4 py-2 text-sm font-medium bg-primary-500 text-white rounded-lg hover:bg-primary-600 transition-colors disabled:opacity-50"
                  >
                    {resetPasswordLoading ? 'Setting…' : 'Set password'}
                  </button>
                  <div className="relative flex items-center">
                    <div className="flex-1 border-t border-dark-border-subtle" />
                    <span className="px-3 text-xs text-dark-text-tertiary">or</span>
                    <div className="flex-1 border-t border-dark-border-subtle" />
                  </div>
                  <button
                    onClick={() => handleAdminResetPassword(true)}
                    disabled={resetPasswordLoading}
                    className="w-full px-4 py-2 text-sm font-medium bg-dark-bg-tertiary text-dark-text-secondary border border-dark-border-subtle rounded-lg hover:text-dark-text-primary hover:bg-dark-bg-tertiary/80 transition-colors disabled:opacity-50"
                  >
                    {resetPasswordLoading ? 'Sending…' : 'Send reset email to user'}
                  </button>
                </div>
              </>
            )}
            <div className="flex justify-end">
              <button
                onClick={() => { setResetPasswordModal(null); setResetPasswordSuccess(''); setResetPasswordError(''); setResetPasswordValue('') }}
                className="px-4 py-2 text-sm font-medium text-dark-text-secondary bg-dark-bg-tertiary/50 border border-dark-border-subtle rounded-lg hover:text-dark-text-primary transition-colors"
              >
                {resetPasswordSuccess ? 'Close' : 'Cancel'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delete User Confirmation Modal */}
      {deleteConfirm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
          <div className="bg-dark-bg-secondary border border-dark-border-subtle rounded-xl shadow-2xl p-6 max-w-md w-full mx-4">
            <div className="flex items-start gap-4">
              <div className="flex-shrink-0 w-10 h-10 rounded-full bg-red-500/10 border border-red-500/30 flex items-center justify-center">
                <svg className="w-5 h-5 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
              </div>
              <div className="flex-1">
                <h3 className="text-base font-semibold text-dark-text-primary">Delete user account</h3>
                <p className="mt-1 text-sm text-dark-text-secondary">
                  Are you sure you want to delete <span className="font-medium text-dark-text-primary">{deleteConfirm.email}</span>?
                </p>
                <ul className="mt-3 space-y-1 text-xs text-dark-text-tertiary">
                  <li className="flex items-center gap-1.5"><span className="text-emerald-400">✓</span> Invite history is preserved</li>
                  <li className="flex items-center gap-1.5"><span className="text-emerald-400">✓</span> Email freed up — user can be re-invited</li>
                  <li className="flex items-center gap-1.5"><span className="text-red-400">✗</span> Login access permanently revoked</li>
                </ul>
              </div>
            </div>
            <div className="mt-5 flex gap-3 justify-end">
              <button
                onClick={() => setDeleteConfirm(null)}
                className="px-4 py-2 text-sm font-medium text-dark-text-secondary bg-dark-bg-tertiary/50 border border-dark-border-subtle rounded-lg hover:text-dark-text-primary hover:bg-dark-bg-tertiary transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={confirmDeleteUser}
                className="px-4 py-2 text-sm font-medium text-white bg-red-600 rounded-lg hover:bg-red-700 transition-colors"
              >
                Delete User
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
