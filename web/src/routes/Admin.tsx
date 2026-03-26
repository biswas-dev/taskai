import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useAuth } from '../state/AuthContext'
import { api, type EmailProviderResponse, type AdminInvitation, type BackupStatus, type BackupSettings, type BackupRecord, type BackupFolder } from '../lib/api'
import { version as frontendVersion } from '../lib/version'
import AdminAnalytics from '../components/AdminAnalytics'

// API URL with fallback for production (empty string = relative URL)
const API_URL = import.meta.env.VITE_API_URL || ''

// ─── Schedule / cron helpers ─────────────────────────────────────────────────
type ScheduleFreq = 'hourly' | 'daily' | 'weekly' | 'monthly' | 'custom'
const DOW_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

function scheduleToCron(freq: ScheduleFreq, hour: number, minute: number, dow: number, dom: number, custom: string): string {
  const h = String(hour).padStart(2, '0')
  const m = String(minute).padStart(2, '0')
  switch (freq) {
    case 'hourly':  return `${m} * * * *`
    case 'daily':   return `${m} ${h} * * *`
    case 'weekly':  return `${m} ${h} * * ${dow}`
    case 'monthly': return `${m} ${h} ${dom} * *`
    case 'custom':  return custom
  }
}

function cronToSchedule(cron: string): { freq: ScheduleFreq; hour: number; minute: number; dow: number; dom: number } {
  const parts = (cron || '0 3 * * *').trim().split(/\s+/)
  if (parts.length !== 5) return { freq: 'custom', hour: 3, minute: 0, dow: 1, dom: 1 }
  const [min, hr, dom, , dow] = parts
  if (hr === '*' && dom === '*' && dow === '*') return { freq: 'hourly', hour: 0, minute: Number(min) || 0, dow: 1, dom: 1 }
  if (dom === '*' && dow === '*') return { freq: 'daily', hour: Number(hr) || 3, minute: Number(min) || 0, dow: 1, dom: 1 }
  if (dom === '*' && dow !== '*') return { freq: 'weekly', hour: Number(hr) || 3, minute: Number(min) || 0, dow: Number(dow) || 1, dom: 1 }
  if (dom !== '*' && dow === '*') return { freq: 'monthly', hour: Number(hr) || 3, minute: Number(min) || 0, dow: 1, dom: Number(dom) || 1 }
  return { freq: 'custom', hour: 3, minute: 0, dow: 1, dom: 1 }
}

function scheduleDesc(freq: ScheduleFreq, hour: number, minute: number, dow: number, dom: number): string {
  const t = `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')} UTC`
  switch (freq) {
    case 'hourly':  return `Every hour at :${String(minute).padStart(2, '0')}`
    case 'daily':   return `Every day at ${t}`
    case 'weekly':  return `Every ${DOW_LABELS[dow]} at ${t}`
    case 'monthly': return `On the ${dom}${dom === 1 ? 'st' : dom === 2 ? 'nd' : dom === 3 ? 'rd' : 'th'} of each month at ${t}`
    case 'custom':  return 'Custom schedule'
  }
}

const MONTH_NAMES = ['January','February','March','April','May','June','July','August','September','October','November','December']

function buildCalendarGrid(year: number, month: number): (number | null)[] {
  const firstDay = new Date(year, month, 1).getDay()
  const daysInMonth = new Date(year, month + 1, 0).getDate()
  const cells: (number | null)[] = []
  for (let i = 0; i < firstDay; i++) cells.push(null)
  for (let d = 1; d <= daysInMonth; d++) cells.push(d)
  while (cells.length % 7 !== 0) cells.push(null)
  return cells
}

function getCronScheduledDays(cron: string, year: number, month: number): Set<number> {
  const s = new Set<number>()
  const parts = cron.trim().split(/\s+/)
  if (parts.length !== 5) return s
  const [, hr, dom, , dow] = parts
  const daysInMonth = new Date(year, month + 1, 0).getDate()
  for (let d = 1; d <= daysInMonth; d++) {
    const date = new Date(year, month, d)
    const matchDow = dow === '*' || date.getDay() === Number(dow)
    const matchDom = dom === '*' || date.getDate() === Number(dom)
    if (matchDow && matchDom) {
      if (hr === '*') { s.add(d) } else { s.add(d) }
    }
  }
  return s
}

function getRetentionTier(backupDate: string, now: Date, fullDays: number, alternateDays: number, weeklyDays: number): 'full' | 'alternate' | 'weekly' | 'expired' {
  const ageDays = Math.floor((now.getTime() - new Date(backupDate).getTime()) / (1000 * 60 * 60 * 24))
  if (ageDays < fullDays) return 'full'
  if (ageDays < alternateDays) return 'alternate'
  if (ageDays < weeklyDays) return 'weekly'
  return 'expired'
}

function relativeTime(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime()
  const secs = Math.floor(diff / 1000)
  if (secs < 60) return 'just now'
  const mins = Math.floor(secs / 60)
  if (mins < 60) return `${mins}m ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs}h ago`
  const days = Math.floor(hrs / 24)
  return `${days}d ago`
}

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
  linked_providers: string[]
}

interface UserActivity {
  id: number
  user_id: number
  activity_type: string
  ip_address?: string | null
  user_agent?: string | null
  created_at: string
}

type AdminTab = 'users' | 'invitations' | 'email' | 'backup' | 'system' | 'analytics'

interface VersionInfoRaw {
  backend: { version: string; git_commit: string; build_time: string; go_version: string; platform: string }
  runtime: { hostname: string; pid: number; port: number; uptime_seconds: number; started_at: string }
  resources: { memory_alloc_mb: number; heap_inuse_mb: number; goroutines: number }
  database: { type: string; migration_version: number; environment?: string }
  container?: { memory_usage_mb: number; cpu_usage_ns: number }
}

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
  uptime_seconds: number
  goroutines: number
  memory_mb: number
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

  // Backup subtab (also reads ?subtab= from URL so OAuth callback can land here directly)
  type BackupSubTab = 'manual' | 'automated'
  const subtabFromUrl = searchParams.get('subtab') as BackupSubTab | null
  const [backupSubTab, setBackupSubTab] = useState<BackupSubTab>(
    subtabFromUrl === 'automated' ? 'automated' : 'manual'
  )

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
  const [autoBackupEnabled, setAutoBackupEnabled] = useState(false)
  const [isAutoBackupAvailable, setIsAutoBackupAvailable] = useState(false)
  const [isLoadingAutoBackup, setIsLoadingAutoBackup] = useState(false)
  const [isTriggeringAutoBackup, setIsTriggeringAutoBackup] = useState(false)
  const [isSavingAutoBackupSettings, setIsSavingAutoBackupSettings] = useState(false)
  const [isDisconnectingAutoBackup, setIsDisconnectingAutoBackup] = useState(false)
  const [autoBackupError, setAutoBackupError] = useState('')
  const [autoBackupSuccess, setAutoBackupSuccess] = useState('')
  // Visual schedule builder
  const [schedFreq, setSchedFreq] = useState<ScheduleFreq>('daily')
  const [schedHour, setSchedHour] = useState(3)
  const [schedMinute, setSchedMinute] = useState(0)
  const [schedDayOfWeek, setSchedDayOfWeek] = useState(1)
  const [schedDayOfMonth, setSchedDayOfMonth] = useState(1)
  const [schedCustomCron, setSchedCustomCron] = useState('0 3 * * *')
  // Folder browser
  const [folderBrowserOpen, setFolderBrowserOpen] = useState(false)
  const [folderStack, setFolderStack] = useState<{ id: string; name: string }[]>([])
  const [folderList, setFolderList] = useState<BackupFolder[]>([])
  const [isFolderLoading, setIsFolderLoading] = useState(false)
  const [newFolderName, setNewFolderName] = useState('')
  const [isCreatingFolder, setIsCreatingFolder] = useState(false)
  const [selectedFolderId, setSelectedFolderId] = useState('')
  const [selectedFolderName, setSelectedFolderName] = useState('')
  const [isDownloadingRecord, setIsDownloadingRecord] = useState<string | null>(null)
  // Retention settings
  const [retFullDays, setRetFullDays] = useState(30)
  const [retAlternateDays, setRetAlternateDays] = useState(60)
  const [retWeeklyDays, setRetWeeklyDays] = useState(365)
  // Folder search
  const [folderSearch, setFolderSearch] = useState('')
  // Calendar navigation
  const [calYear, setCalYear] = useState(() => new Date().getFullYear())
  const [calMonth, setCalMonth] = useState(() => new Date().getMonth())

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
  }, [activeTab, backupSubTab])

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
        api.listBackupHistory(20),
      ])
      setAutoBackupStatus(status)
      setAutoBackupSettings(settings)
      setAutoBackupHistory(history)
      setAutoBackupEnabled(settings.enabled)
      // Initialise visual schedule builder from stored cron
      const cron = settings.cron_expression || '0 3 * * *'
      const parsed = cronToSchedule(cron)
      setSchedFreq(parsed.freq)
      setSchedHour(parsed.hour)
      setSchedMinute(parsed.minute)
      setSchedDayOfWeek(parsed.dow)
      setSchedDayOfMonth(parsed.dom)
      setSchedCustomCron(cron)
      // Initialise retention
      setRetFullDays(settings.retention?.FullDays || 30)
      setRetAlternateDays(settings.retention?.AlternateDays || 60)
      setRetWeeklyDays(settings.retention?.WeeklyDays || 365)
      // Initialise folder selection
      setSelectedFolderId(settings.folder_id || '')
      setSelectedFolderName(settings.folder_id ? `Folder: ${settings.folder_id}` : '')
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

  const handleSaveAutoBackupSettings = async () => {
    setIsSavingAutoBackupSettings(true)
    setAutoBackupError('')
    setAutoBackupSuccess('')
    try {
      const cron = scheduleToCron(schedFreq, schedHour, schedMinute, schedDayOfWeek, schedDayOfMonth, schedCustomCron)
      const updated = await api.updateBackupSettings({
        enabled: autoBackupEnabled,
        cron_expression: cron,
        folder_id: selectedFolderId,
        retention: { FullDays: retFullDays, AlternateDays: retAlternateDays, WeeklyDays: retWeeklyDays },
      })
      setAutoBackupSettings(updated)
      setAutoBackupSuccess('Settings saved')
      setTimeout(() => setAutoBackupSuccess(''), 3000)
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

  const handleDownloadRecord = async (record: BackupRecord) => {
    setIsDownloadingRecord(record.id)
    try {
      await api.downloadBackupRecord(record.id, record.filename || `backup-${record.id}.tar.gz`)
    } catch (err) {
      setAutoBackupError(err instanceof Error ? err.message : 'Download failed')
    } finally {
      setIsDownloadingRecord(null)
    }
  }

  const openFolderBrowser = async () => {
    setFolderBrowserOpen(true)
    setFolderStack([])
    setNewFolderName('')
    await loadFolders('')
  }

  const loadFolders = async (parentId: string) => {
    setIsFolderLoading(true)
    try {
      const folders = await api.listBackupFolders(parentId)
      setFolderList(folders || [])
    } catch {
      setFolderList([])
    } finally {
      setIsFolderLoading(false)
    }
  }

  const handleFolderClick = (folder: BackupFolder) => {
    setFolderStack(prev => [...prev, { id: folder.id, name: folder.name }])
    loadFolders(folder.id)
  }

  const handleFolderBreadcrumb = (idx: number) => {
    const newStack = folderStack.slice(0, idx + 1)
    setFolderStack(newStack)
    loadFolders(newStack[newStack.length - 1]?.id ?? '')
  }

  const handleFolderBreadcrumbRoot = () => {
    setFolderStack([])
    loadFolders('')
  }

  const handleSelectFolder = (id: string, name: string) => {
    setSelectedFolderId(id)
    setSelectedFolderName(name)
    setFolderBrowserOpen(false)
  }

  const handleCreateFolder = async () => {
    if (!newFolderName.trim()) return
    setIsCreatingFolder(true)
    try {
      const parentId = folderStack[folderStack.length - 1]?.id ?? ''
      const folder = await api.createBackupFolder(newFolderName.trim(), parentId)
      setFolderList(prev => [...prev, folder])
      setNewFolderName('')
    } catch (err) {
      setAutoBackupError(err instanceof Error ? err.message : 'Failed to create folder')
    } finally {
      setIsCreatingFolder(false)
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
      const raw = await api.getVersion() as unknown as VersionInfoRaw
      setVersionInfo({
        version: raw.backend.version,
        git_commit: raw.backend.git_commit,
        build_time: raw.backend.build_time,
        go_version: raw.backend.go_version,
        platform: raw.backend.platform,
        server_time: raw.runtime.started_at,
        db_version: raw.database.migration_version,
        environment: raw.database.environment || (raw.backend.version === 'dev' ? 'development' : 'production'),
        db_driver: raw.database.type,
        uptime_seconds: raw.runtime.uptime_seconds,
        goroutines: raw.resources.goroutines,
        memory_mb: raw.resources.memory_alloc_mb,
      })
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
            <button
              onClick={() => handleTabChange('analytics')}
              className={`px-4 py-2 text-sm font-medium rounded-lg transition-colors ${
                activeTab === 'analytics'
                  ? 'bg-primary-500/10 text-primary-400 border border-primary-500/30'
                  : 'text-dark-text-secondary hover:text-dark-text-primary hover:bg-dark-bg-tertiary/30'
              }`}
            >
              Analytics
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

                      {/* Auth providers */}
                      <div className="bg-dark-bg-secondary rounded-lg p-3 border border-dark-border-subtle">
                        <p className="text-xs text-dark-text-tertiary mb-2">Auth Methods</p>
                        <div className="flex flex-wrap gap-2">
                          {(u.linked_providers ?? []).length === 0 ? (
                            <span className="text-xs text-dark-text-tertiary">None recorded</span>
                          ) : (u.linked_providers ?? []).map(p => (
                            <span key={p} className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium ${
                              p === 'password' ? 'bg-blue-500/20 text-blue-300 border border-blue-500/30' :
                              p === 'google'   ? 'bg-red-500/20 text-red-300 border border-red-500/30' :
                              p === 'github'   ? 'bg-gray-500/20 text-gray-300 border border-gray-500/30' :
                                                 'bg-purple-500/20 text-purple-300 border border-purple-500/30'
                            }`}>
                              {p === 'password' ? '🔑' : p === 'google' ? '🔴' : p === 'github' ? '🐙' : '🔗'} {p}
                            </span>
                          ))}
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
                  <button key={sub} onClick={() => { setBackupSubTab(sub); setSearchParams({ tab: 'backup', subtab: sub }) }}
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
                <div className="space-y-4">
                  {isLoadingAutoBackup && (
                    <div className="animate-pulse space-y-4">
                      <div className="h-28 bg-dark-bg-secondary rounded-xl"></div>
                      <div className="h-40 bg-dark-bg-secondary rounded-xl"></div>
                    </div>
                  )}

                  {!isLoadingAutoBackup && !isAutoBackupAvailable && (
                    <div className="bg-dark-bg-secondary rounded-xl border border-dark-border-subtle p-6">
                      <p className="text-sm text-dark-text-tertiary">
                        Google Drive backup is not configured. Set{' '}
                        <code className="font-mono text-xs bg-dark-bg-tertiary px-1 py-0.5 rounded">GOOGLE_CLIENT_ID</code>{' '}
                        and ensure PostgreSQL is in use.
                      </p>
                    </div>
                  )}

                  {!isLoadingAutoBackup && isAutoBackupAvailable && (
                    <>
                      {/* Global alerts */}
                      {autoBackupSuccess && (
                        <div className="p-3 bg-emerald-500/10 border border-emerald-500/30 rounded-lg text-sm text-emerald-400">{autoBackupSuccess}</div>
                      )}
                      {autoBackupError && (
                        <div className="p-3 bg-red-500/10 border border-red-500/30 rounded-lg text-sm text-red-400 flex items-center justify-between">
                          <span>{autoBackupError}</span>
                          <button onClick={() => setAutoBackupError('')} className="ml-3 text-red-400/60 hover:text-red-400">✕</button>
                        </div>
                      )}

                      {/* Card 1 — Connection */}
                      <div className="bg-dark-bg-secondary rounded-xl border border-dark-border-subtle p-5">
                        <div className="flex items-center gap-3 mb-4">
                          <div className="w-9 h-9 rounded-lg bg-teal-500/15 flex items-center justify-center flex-shrink-0">
                            <svg className="w-4.5 h-4.5 text-teal-400" viewBox="0 0 24 24" fill="currentColor" style={{ width: '18px', height: '18px' }}>
                              <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" />
                              <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" />
                              <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" />
                              <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" />
                            </svg>
                          </div>
                          <div className="flex-1 min-w-0">
                            <h3 className="text-sm font-semibold text-dark-text-primary">Google Drive</h3>
                            {autoBackupStatus?.provider_connected ? (
                              <p className="text-xs text-emerald-400 flex items-center gap-1">
                                <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 inline-block"></span>
                                Connected{autoBackupStatus.connected_email ? ` · ${autoBackupStatus.connected_email}` : ''}
                              </p>
                            ) : (
                              <p className="text-xs text-dark-text-tertiary">Not connected</p>
                            )}
                          </div>
                          {autoBackupStatus?.provider_connected ? (
                            <div className="flex items-center gap-2">
                              <button
                                onClick={handleTriggerAutoBackup}
                                disabled={isTriggeringAutoBackup || !!autoBackupStatus?.running}
                                className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-teal-600 hover:bg-teal-500 disabled:opacity-50 text-white rounded-lg text-xs font-medium transition-colors"
                              >
                                {isTriggeringAutoBackup || autoBackupStatus?.running ? (
                                  <><div className="w-3 h-3 rounded-full border border-white border-t-transparent animate-spin"></div>Running…</>
                                ) : (
                                  <><svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" /><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>Run Now</>
                                )}
                              </button>
                              <button
                                onClick={handleDisconnectAutoBackup}
                                disabled={isDisconnectingAutoBackup}
                                className="px-3 py-1.5 text-xs font-medium text-dark-text-secondary hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-colors"
                              >
                                Disconnect
                              </button>
                            </div>
                          ) : (
                            <a
                              href="/api/admin/backup/oauth/start"
                              className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-teal-600 hover:bg-teal-500 text-white rounded-lg text-xs font-medium transition-colors"
                            >
                              <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" /></svg>
                              Connect
                            </a>
                          )}
                        </div>
                        {autoBackupStatus?.next_run && (
                          <p className="text-xs text-dark-text-tertiary border-t border-dark-border-subtle pt-3">
                            Next scheduled run: {new Date(autoBackupStatus.next_run).toLocaleString()}
                          </p>
                        )}
                        {autoBackupStatus?.last_backup && (
                          <p className="text-xs text-dark-text-tertiary mt-1">
                            Last backup: {relativeTime(autoBackupStatus.last_backup.started_at)} ·{' '}
                            <span className={autoBackupStatus.last_backup.status === 'success' ? 'text-emerald-400' : 'text-red-400'}>
                              {autoBackupStatus.last_backup.status}
                            </span>
                            {autoBackupStatus.last_backup.size_bytes > 0 && ` · ${formatBytes(autoBackupStatus.last_backup.size_bytes)}`}
                          </p>
                        )}
                      </div>

                      {/* Cards 2 & 3 — Schedule + Storage (only when connected) */}
                      {autoBackupStatus?.provider_connected && (
                        <>
                          {/* Card 2 — Schedule */}
                          <div className="bg-dark-bg-secondary rounded-xl border border-dark-border-subtle p-5">
                            <div className="flex items-center gap-3 mb-4">
                              <div className="w-9 h-9 rounded-lg bg-violet-500/15 flex items-center justify-center flex-shrink-0">
                                <svg className="w-4 h-4 text-violet-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                              </div>
                              <div className="flex-1">
                                <h3 className="text-sm font-semibold text-dark-text-primary">Schedule</h3>
                                <p className="text-xs text-dark-text-tertiary">{scheduleDesc(schedFreq, schedHour, schedMinute, schedDayOfWeek, schedDayOfMonth)}</p>
                              </div>
                              <div className="flex items-center gap-2">
                                <span className="text-xs text-dark-text-secondary">Enabled</span>
                                <label className="relative inline-flex items-center cursor-pointer">
                                  <input type="checkbox" checked={autoBackupEnabled} onChange={e => setAutoBackupEnabled(e.target.checked)} className="sr-only peer" />
                                  <div className="w-8 h-4 bg-dark-bg-tertiary rounded-full peer peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-0.5 after:left-[2px] after:bg-white after:rounded-full after:h-3 after:w-3 after:transition-all peer-checked:bg-teal-600"></div>
                                </label>
                              </div>
                            </div>

                            {/* Frequency pills */}
                            <div className="flex flex-wrap gap-2 mb-4">
                              {(['hourly', 'daily', 'weekly', 'monthly', 'custom'] as ScheduleFreq[]).map(f => (
                                <button
                                  key={f}
                                  onClick={() => setSchedFreq(f)}
                                  className={`px-3 py-1 rounded-full text-xs font-medium transition-colors capitalize ${
                                    schedFreq === f
                                      ? 'bg-violet-500/20 text-violet-300 border border-violet-500/40'
                                      : 'bg-dark-bg-tertiary text-dark-text-secondary hover:text-dark-text-primary border border-dark-border-subtle'
                                  }`}
                                >
                                  {f}
                                </button>
                              ))}
                            </div>

                            {/* Time / day pickers */}
                            {schedFreq !== 'custom' && (
                              <div className="flex flex-wrap gap-3 mb-4">
                                {schedFreq !== 'hourly' && (
                                  <div>
                                    <label className="block text-xs text-dark-text-tertiary mb-1">Hour (UTC)</label>
                                    <select
                                      value={schedHour}
                                      onChange={e => setSchedHour(Number(e.target.value))}
                                      className="px-2 py-1.5 bg-dark-bg-primary border border-dark-border-subtle rounded text-sm text-dark-text-primary focus:outline-none focus:border-dark-accent-primary"
                                    >
                                      {Array.from({ length: 24 }, (_, i) => (
                                        <option key={i} value={i}>{String(i).padStart(2, '0')}:00</option>
                                      ))}
                                    </select>
                                  </div>
                                )}
                                <div>
                                  <label className="block text-xs text-dark-text-tertiary mb-1">Minute</label>
                                  <select
                                    value={schedMinute}
                                    onChange={e => setSchedMinute(Number(e.target.value))}
                                    className="px-2 py-1.5 bg-dark-bg-primary border border-dark-border-subtle rounded text-sm text-dark-text-primary focus:outline-none focus:border-dark-accent-primary"
                                  >
                                    {[0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55].map(m => (
                                      <option key={m} value={m}>:{String(m).padStart(2, '0')}</option>
                                    ))}
                                  </select>
                                </div>
                                {schedFreq === 'weekly' && (
                                  <div>
                                    <label className="block text-xs text-dark-text-tertiary mb-1">Day of week</label>
                                    <select
                                      value={schedDayOfWeek}
                                      onChange={e => setSchedDayOfWeek(Number(e.target.value))}
                                      className="px-2 py-1.5 bg-dark-bg-primary border border-dark-border-subtle rounded text-sm text-dark-text-primary focus:outline-none focus:border-dark-accent-primary"
                                    >
                                      {DOW_LABELS.map((d, i) => <option key={i} value={i}>{d}</option>)}
                                    </select>
                                  </div>
                                )}
                                {schedFreq === 'monthly' && (
                                  <div>
                                    <label className="block text-xs text-dark-text-tertiary mb-1">Day of month</label>
                                    <select
                                      value={schedDayOfMonth}
                                      onChange={e => setSchedDayOfMonth(Number(e.target.value))}
                                      className="px-2 py-1.5 bg-dark-bg-primary border border-dark-border-subtle rounded text-sm text-dark-text-primary focus:outline-none focus:border-dark-accent-primary"
                                    >
                                      {Array.from({ length: 28 }, (_, i) => i + 1).map(d => <option key={d} value={d}>{d}</option>)}
                                    </select>
                                  </div>
                                )}
                              </div>
                            )}

                            {schedFreq === 'custom' && (
                              <div className="mb-4">
                                <label className="block text-xs text-dark-text-tertiary mb-1">Cron expression</label>
                                <input
                                  type="text"
                                  value={schedCustomCron}
                                  onChange={e => setSchedCustomCron(e.target.value)}
                                  placeholder="0 3 * * *"
                                  className="w-full max-w-xs px-3 py-1.5 bg-dark-bg-primary border border-dark-border-subtle rounded text-sm text-dark-text-primary font-mono focus:outline-none focus:border-dark-accent-primary"
                                />
                              </div>
                            )}

                            {/* Cron preview */}
                            <p className="text-xs text-dark-text-tertiary font-mono mb-5">
                              {scheduleToCron(schedFreq, schedHour, schedMinute, schedDayOfWeek, schedDayOfMonth, schedCustomCron)}
                            </p>

                            {/* Retention */}
                            <div className="border-t border-dark-border-subtle pt-4">
                              <p className="text-xs font-medium text-dark-text-secondary mb-3">Retention Policy</p>
                              <div className="grid grid-cols-3 gap-4 mb-3">
                                {([
                                  { label: 'Daily (full)', key: 'full', value: retFullDays, setter: setRetFullDays, color: 'text-teal-400', max: retAlternateDays - 1 },
                                  { label: 'Alternate days', key: 'alt', value: retAlternateDays, setter: setRetAlternateDays, color: 'text-amber-400', max: retWeeklyDays - 1 },
                                  { label: 'Weekly', key: 'weekly', value: retWeeklyDays, setter: setRetWeeklyDays, color: 'text-blue-400', max: 3650 },
                                ] as const).map(({ label, key, value, setter, color, max }) => (
                                  <div key={key}>
                                    <label className={`block text-xs font-medium ${color} mb-1`}>{label}</label>
                                    <div className="flex items-center gap-1.5">
                                      <input
                                        type="number"
                                        min={1}
                                        max={max}
                                        value={value}
                                        onChange={e => setter(Math.max(1, Math.min(max, Number(e.target.value))))}
                                        className="w-16 px-2 py-1 bg-dark-bg-primary border border-dark-border-subtle rounded text-sm text-dark-text-primary text-center focus:outline-none focus:border-dark-accent-primary"
                                      />
                                      <span className="text-xs text-dark-text-tertiary">days</span>
                                    </div>
                                  </div>
                                ))}
                              </div>
                              {/* Retention timeline bar */}
                              <div className="relative h-3 rounded-full overflow-hidden flex">
                                <div
                                  className="bg-teal-500/60 h-full"
                                  style={{ width: `${(retFullDays / retWeeklyDays) * 100}%` }}
                                  title={`Daily: 0–${retFullDays}d`}
                                />
                                <div
                                  className="bg-amber-500/60 h-full"
                                  style={{ width: `${((retAlternateDays - retFullDays) / retWeeklyDays) * 100}%` }}
                                  title={`Alternate: ${retFullDays}–${retAlternateDays}d`}
                                />
                                <div
                                  className="bg-blue-500/60 h-full flex-1"
                                  title={`Weekly: ${retAlternateDays}–${retWeeklyDays}d`}
                                />
                              </div>
                              <div className="flex justify-between text-xs text-dark-text-tertiary mt-1">
                                <span>0</span>
                                <span className="text-teal-400">{retFullDays}d</span>
                                <span className="text-amber-400">{retAlternateDays}d</span>
                                <span className="text-blue-400">{retWeeklyDays}d</span>
                              </div>
                              <p className="text-xs text-dark-text-tertiary mt-2">
                                Keep every backup for {retFullDays} days, then every other day until day {retAlternateDays}, then weekly until day {retWeeklyDays}.
                              </p>
                            </div>
                          </div>

                          {/* Card 3 — Storage / Folder */}
                          <div className="bg-dark-bg-secondary rounded-xl border border-dark-border-subtle p-5">
                            <div className="flex items-center gap-3 mb-4">
                              <div className="w-9 h-9 rounded-lg bg-amber-500/15 flex items-center justify-center flex-shrink-0">
                                <svg className="w-4 h-4 text-amber-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" /></svg>
                              </div>
                              <div className="flex-1">
                                <h3 className="text-sm font-semibold text-dark-text-primary">Storage Folder</h3>
                                <p className="text-xs text-dark-text-tertiary">
                                  {selectedFolderName || (selectedFolderId ? `ID: ${selectedFolderId}` : 'Google Drive root')}
                                </p>
                              </div>
                              <button
                                onClick={openFolderBrowser}
                                className="px-3 py-1.5 text-xs font-medium bg-dark-bg-tertiary hover:bg-dark-bg-primary border border-dark-border-subtle text-dark-text-secondary hover:text-dark-text-primary rounded-lg transition-colors"
                              >
                                Browse…
                              </button>
                              {selectedFolderId && (
                                <button
                                  onClick={() => { setSelectedFolderId(''); setSelectedFolderName('') }}
                                  className="px-3 py-1.5 text-xs font-medium text-dark-text-tertiary hover:text-dark-text-secondary rounded-lg transition-colors"
                                >
                                  Use root
                                </button>
                              )}
                            </div>

                            {/* Folder browser inline */}
                            {folderBrowserOpen && (
                              <div className="border border-dark-border-subtle rounded-lg overflow-hidden mb-4">
                                {/* Breadcrumb */}
                                <div className="flex items-center gap-1 px-3 py-2 bg-dark-bg-tertiary border-b border-dark-border-subtle text-xs text-dark-text-secondary overflow-x-auto">
                                  <button onClick={handleFolderBreadcrumbRoot} className="hover:text-dark-text-primary transition-colors shrink-0">Root</button>
                                  {folderStack.map((f, i) => (
                                    <span key={f.id} className="flex items-center gap-1 shrink-0">
                                      <span className="text-dark-text-tertiary">/</span>
                                      <button onClick={() => handleFolderBreadcrumb(i)} className="hover:text-dark-text-primary transition-colors">{f.name}</button>
                                    </span>
                                  ))}
                                </div>

                                {/* Search */}
                                <div className="px-3 py-2 border-b border-dark-border-subtle">
                                  <input
                                    type="text"
                                    value={folderSearch}
                                    onChange={e => setFolderSearch(e.target.value)}
                                    placeholder="Search folders…"
                                    className="w-full px-2 py-1 text-xs bg-dark-bg-primary border border-dark-border-subtle rounded text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:border-dark-accent-primary"
                                  />
                                </div>

                                {/* Folder list */}
                                <div className="max-h-48 overflow-y-auto">
                                  {isFolderLoading ? (
                                    <div className="p-4 text-center text-xs text-dark-text-tertiary">Loading…</div>
                                  ) : folderList.filter(f => f.name.toLowerCase().includes(folderSearch.toLowerCase())).length === 0 ? (
                                    <div className="p-4 text-center text-xs text-dark-text-tertiary">{folderSearch ? 'No matches' : 'No folders here'}</div>
                                  ) : (
                                    folderList.filter(f => f.name.toLowerCase().includes(folderSearch.toLowerCase())).map(folder => (
                                      <div key={folder.id} className="flex items-center px-3 py-2 hover:bg-dark-bg-tertiary/50 border-b border-dark-border-subtle last:border-0 group">
                                        <svg className="w-3.5 h-3.5 text-amber-400 mr-2 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" /></svg>
                                        <span className="flex-1 text-xs text-dark-text-primary truncate">{folder.name}</span>
                                        <button
                                          onClick={() => handleSelectFolder(folder.id, folder.name)}
                                          className="opacity-0 group-hover:opacity-100 px-2 py-0.5 text-xs bg-teal-600 text-white rounded transition-all mr-1"
                                        >
                                          Use
                                        </button>
                                        <button
                                          onClick={() => handleFolderClick(folder)}
                                          className="opacity-0 group-hover:opacity-100 px-2 py-0.5 text-xs bg-dark-bg-primary border border-dark-border-subtle text-dark-text-secondary rounded transition-all"
                                        >
                                          Open
                                        </button>
                                      </div>
                                    ))
                                  )}
                                </div>

                                {/* New folder + actions */}
                                <div className="flex items-center gap-2 px-3 py-2 border-t border-dark-border-subtle bg-dark-bg-tertiary/50">
                                  <input
                                    type="text"
                                    value={newFolderName}
                                    onChange={e => setNewFolderName(e.target.value)}
                                    onKeyDown={e => e.key === 'Enter' && handleCreateFolder()}
                                    placeholder="New folder name…"
                                    className="flex-1 px-2 py-1 text-xs bg-dark-bg-primary border border-dark-border-subtle rounded text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:border-dark-accent-primary"
                                  />
                                  <button
                                    onClick={handleCreateFolder}
                                    disabled={isCreatingFolder || !newFolderName.trim()}
                                    className="px-2 py-1 text-xs bg-teal-600 hover:bg-teal-500 disabled:opacity-40 text-white rounded transition-colors"
                                  >
                                    Create
                                  </button>
                                  <button
                                    onClick={() => {
                                      const currentId = folderStack[folderStack.length - 1]?.id ?? ''
                                      const currentName = folderStack[folderStack.length - 1]?.name ?? 'Root'
                                      handleSelectFolder(currentId, currentName === 'Root' ? '' : currentName)
                                    }}
                                    className="px-2 py-1 text-xs bg-dark-bg-primary border border-dark-border-subtle text-dark-text-secondary hover:text-dark-text-primary rounded transition-colors"
                                  >
                                    Use current
                                  </button>
                                </div>
                              </div>
                            )}
                          </div>

                          {/* Save settings button */}
                          <div className="flex items-center gap-3">
                            <button
                              onClick={handleSaveAutoBackupSettings}
                              disabled={isSavingAutoBackupSettings}
                              className="px-4 py-2 bg-teal-600 hover:bg-teal-500 disabled:opacity-50 text-white rounded-lg text-sm font-medium transition-colors"
                            >
                              {isSavingAutoBackupSettings ? 'Saving…' : 'Save Settings'}
                            </button>
                            {autoBackupSettings && (
                              <p className="text-xs text-dark-text-tertiary">
                                Last saved {relativeTime(autoBackupSettings.updated_at)}
                              </p>
                            )}
                          </div>
                        </>
                      )}

                      {/* Card 4 — History */}
                      <div className="bg-dark-bg-secondary rounded-xl border border-dark-border-subtle overflow-hidden">
                        <div className="flex items-center gap-3 px-5 py-4 border-b border-dark-border-subtle">
                          <div className="w-9 h-9 rounded-lg bg-blue-500/15 flex items-center justify-center flex-shrink-0">
                            <svg className="w-4 h-4 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" /></svg>
                          </div>
                          <h3 className="text-sm font-semibold text-dark-text-primary flex-1">Backup History</h3>
                          <button
                            onClick={loadAutomatedBackup}
                            className="text-xs text-dark-text-tertiary hover:text-dark-text-secondary transition-colors"
                          >
                            Refresh
                          </button>
                        </div>

                        {autoBackupHistory.length === 0 ? (
                          <div className="px-5 py-8 text-center">
                            <p className="text-sm text-dark-text-tertiary">No backups yet.</p>
                            {autoBackupStatus?.provider_connected && (
                              <p className="text-xs text-dark-text-tertiary mt-1">Run your first backup using the button above.</p>
                            )}
                          </div>
                        ) : (
                          <div className="divide-y divide-dark-border-subtle">
                            {autoBackupHistory.map(record => (
                              <div key={record.id} className="flex items-center gap-3 px-5 py-3 hover:bg-dark-bg-tertiary/30 transition-colors">
                                {/* Status dot */}
                                <span className={`w-2 h-2 rounded-full flex-shrink-0 ${
                                  record.status === 'success' ? 'bg-emerald-400' :
                                  record.status === 'failed' ? 'bg-red-400' : 'bg-amber-400 animate-pulse'
                                }`}></span>
                                {/* Filename + meta */}
                                <div className="flex-1 min-w-0">
                                  <p className="text-xs font-medium text-dark-text-primary truncate">{record.filename || `backup-${record.id.slice(0, 8)}`}</p>
                                  <p className="text-xs text-dark-text-tertiary">
                                    {relativeTime(record.started_at)} ·{' '}
                                    {record.size_bytes > 0 ? formatBytes(record.size_bytes) : '—'} ·{' '}
                                    {record.triggered_by}
                                    {record.status === 'failed' && record.error_message && (
                                      <span className="text-red-400 ml-1">· {record.error_message}</span>
                                    )}
                                  </p>
                                </div>
                                {/* Actions */}
                                <div className="flex items-center gap-1 flex-shrink-0">
                                  {record.status === 'success' && (
                                    <button
                                      onClick={() => handleDownloadRecord(record)}
                                      disabled={isDownloadingRecord === record.id}
                                      title="Download backup"
                                      className="p-1.5 rounded text-dark-text-tertiary hover:text-teal-400 hover:bg-teal-500/10 transition-colors disabled:opacity-50"
                                    >
                                      {isDownloadingRecord === record.id ? (
                                        <div className="w-3.5 h-3.5 rounded-full border border-current border-t-transparent animate-spin"></div>
                                      ) : (
                                        <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" /></svg>
                                      )}
                                    </button>
                                  )}
                                  <button
                                    onClick={() => handleDeleteAutoBackupRecord(record.id)}
                                    title="Delete record"
                                    className="p-1.5 rounded text-dark-text-tertiary hover:text-red-400 hover:bg-red-500/10 transition-colors"
                                  >
                                    <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                                  </button>
                                </div>
                              </div>
                            ))}
                          </div>
                        )}
                      </div>

                      {/* Card 5 — Backup Calendar */}
                      <div className="bg-dark-bg-secondary rounded-xl border border-dark-border-subtle overflow-hidden">
                        <div className="flex items-center gap-3 px-5 py-4 border-b border-dark-border-subtle">
                          <div className="w-9 h-9 rounded-lg bg-purple-500/15 flex items-center justify-center flex-shrink-0">
                            <svg className="w-4 h-4 text-purple-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" /></svg>
                          </div>
                          <h3 className="text-sm font-semibold text-dark-text-primary flex-1">Backup Calendar</h3>
                          <div className="flex items-center gap-1">
                            <button
                              onClick={() => {
                                const d = new Date(calYear, calMonth - 1, 1)
                                setCalYear(d.getFullYear())
                                setCalMonth(d.getMonth())
                              }}
                              className="p-1.5 rounded text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary transition-colors"
                            >
                              <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" /></svg>
                            </button>
                            <span className="text-xs font-medium text-dark-text-primary min-w-[110px] text-center">
                              {MONTH_NAMES[calMonth]} {calYear}
                            </span>
                            <button
                              onClick={() => {
                                const d = new Date(calYear, calMonth + 1, 1)
                                setCalYear(d.getFullYear())
                                setCalMonth(d.getMonth())
                              }}
                              className="p-1.5 rounded text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary transition-colors"
                            >
                              <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" /></svg>
                            </button>
                            <button
                              onClick={() => { setCalYear(new Date().getFullYear()); setCalMonth(new Date().getMonth()) }}
                              className="ml-1 px-2 py-1 text-xs text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary border border-dark-border-subtle rounded transition-colors"
                            >
                              Today
                            </button>
                          </div>
                        </div>

                        <div className="p-4">
                          {/* Day-of-week headers */}
                          <div className="grid grid-cols-7 mb-1">
                            {['Sun','Mon','Tue','Wed','Thu','Fri','Sat'].map(d => (
                              <div key={d} className="text-center text-xs font-medium text-dark-text-tertiary py-1">{d}</div>
                            ))}
                          </div>

                          {/* Calendar grid */}
                          {(() => {
                            const now = new Date()
                            const grid = buildCalendarGrid(calYear, calMonth)
                            const cronExpr = schedFreq === 'custom' ? schedCustomCron : scheduleToCron(schedFreq, schedHour, schedMinute, schedDayOfWeek, schedDayOfMonth, schedCustomCron)
                            const scheduledDays = autoBackupSettings?.enabled ? getCronScheduledDays(cronExpr, calYear, calMonth) : new Set<number>()
                            const today = now.getFullYear() === calYear && now.getMonth() === calMonth ? now.getDate() : -1

                            // Build a map: day → record for that day (prefer success over failed)
                            const dayRecordMap = new Map<number, typeof autoBackupHistory[0]>()
                            for (const rec of autoBackupHistory) {
                              const d = new Date(rec.started_at)
                              if (d.getFullYear() === calYear && d.getMonth() === calMonth) {
                                const day = d.getDate()
                                const existing = dayRecordMap.get(day)
                                if (!existing || (rec.status === 'success' && existing.status !== 'success')) {
                                  dayRecordMap.set(day, rec)
                                }
                              }
                            }

                            return (
                              <div className="grid grid-cols-7 gap-0.5">
                                {grid.map((day, idx) => {
                                  if (day === null) {
                                    return <div key={`empty-${idx}`} className="aspect-square" />
                                  }
                                  const rec = dayRecordMap.get(day)
                                  const isFuture = new Date(calYear, calMonth, day) > now
                                  const isToday = day === today
                                  const isScheduled = scheduledDays.has(day)

                                  let bgClass = ''
                                  let dotColor = ''
                                  let title = ''

                                  if (rec) {
                                    if (rec.status === 'success') {
                                      const tier = getRetentionTier(rec.started_at, now, retFullDays, retAlternateDays, retWeeklyDays)
                                      if (tier === 'full') { bgClass = 'bg-teal-500/20 hover:bg-teal-500/30'; dotColor = 'bg-teal-400'; title = 'Daily backup (full retention)' }
                                      else if (tier === 'alternate') { bgClass = 'bg-amber-500/20 hover:bg-amber-500/30'; dotColor = 'bg-amber-400'; title = 'Alternate-day retention' }
                                      else if (tier === 'weekly') { bgClass = 'bg-blue-500/20 hover:bg-blue-500/30'; dotColor = 'bg-blue-400'; title = 'Weekly retention' }
                                      else { bgClass = 'bg-red-500/10 hover:bg-red-500/15'; dotColor = 'bg-red-400'; title = 'Expired (will be pruned)' }
                                    } else if (rec.status === 'failed') {
                                      bgClass = 'bg-red-500/15 hover:bg-red-500/25'
                                      dotColor = 'bg-red-400'
                                      title = `Failed: ${rec.error_message || 'unknown error'}`
                                    }
                                  }

                                  return (
                                    <div
                                      key={day}
                                      title={title || (isFuture && isScheduled ? 'Scheduled' : '')}
                                      className={`aspect-square rounded flex flex-col items-center justify-center relative cursor-default transition-colors ${bgClass || (isScheduled && isFuture ? 'bg-dark-bg-tertiary/40' : '')}`}
                                    >
                                      <span className={`text-xs font-medium ${
                                        isToday ? 'text-primary-400 font-bold' :
                                        rec ? 'text-dark-text-primary' :
                                        isFuture ? 'text-dark-text-tertiary' :
                                        'text-dark-text-secondary'
                                      }`}>
                                        {day}
                                      </span>
                                      {/* Dot for past backup */}
                                      {rec && <span className={`w-1 h-1 rounded-full ${dotColor} mt-0.5`} />}
                                      {/* Ring for future scheduled */}
                                      {!rec && isScheduled && isFuture && (
                                        <span className="w-1 h-1 rounded-full border border-dark-text-tertiary mt-0.5 opacity-50" />
                                      )}
                                      {/* Today indicator */}
                                      {isToday && (
                                        <span className="absolute bottom-0.5 left-1/2 -translate-x-1/2 w-1 h-1 rounded-full bg-primary-400" />
                                      )}
                                    </div>
                                  )
                                })}
                              </div>
                            )
                          })()}

                          {/* Legend */}
                          <div className="flex flex-wrap items-center gap-3 mt-4 pt-3 border-t border-dark-border-subtle">
                            <span className="text-xs text-dark-text-tertiary font-medium">Legend:</span>
                            {[
                              { color: 'bg-teal-400', label: `Daily (0–${retFullDays}d)` },
                              { color: 'bg-amber-400', label: `Alternate (${retFullDays}–${retAlternateDays}d)` },
                              { color: 'bg-blue-400', label: `Weekly (${retAlternateDays}–${retWeeklyDays}d)` },
                              { color: 'bg-red-400', label: 'Expired' },
                            ].map(({ color, label }) => (
                              <div key={label} className="flex items-center gap-1.5">
                                <span className={`w-2 h-2 rounded-full ${color}`} />
                                <span className="text-xs text-dark-text-secondary">{label}</span>
                              </div>
                            ))}
                            <div className="flex items-center gap-1.5">
                              <span className="w-2 h-2 rounded-full border border-dark-text-tertiary opacity-50" />
                              <span className="text-xs text-dark-text-secondary">Scheduled</span>
                            </div>
                          </div>
                        </div>
                      </div>
                    </>
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

          {/* Analytics Tab */}
          {activeTab === 'analytics' && (
            <AdminAnalytics users={users} />
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
