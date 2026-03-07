import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import Card from '../components/ui/Card'
import Button from '../components/ui/Button'
import TextInput from '../components/ui/TextInput'
import FormError from '../components/ui/FormError'
import SearchSelect from '../components/ui/SearchSelect'
import { apiClient, type CloudinaryCredentialResponse, type APIKey, type Team, type TeamMember, type TeamInvitation, type TeamMembership, type SentInvitation, type UserSearchResult, type Invite, type ProjectInvitation } from '../lib/api'
import type { FigmaCredentialsStatus, BackupStatus, BackupSettings, BackupRecord } from '../lib/api'

export default function Settings() {
  const navigate = useNavigate()

  // Profile state
  const [profileFirstName, setProfileFirstName] = useState('')
  const [profileLastName, setProfileLastName] = useState('')
  const [profileError, setProfileError] = useState('')
  const [profileSuccess, setProfileSuccess] = useState('')
  const [isSavingProfile, setIsSavingProfile] = useState(false)

  // Password change state
  const [hasPassword, setHasPassword] = useState(true) // false = OAuth-only user
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [passwordError, setPasswordError] = useState('')
  const [passwordSuccess, setPasswordSuccess] = useState('')
  const [isChangingPassword, setIsChangingPassword] = useState(false)

  // 2FA state
  const [twoFAEnabled, setTwoFAEnabled] = useState(false)
  const [twoFASecret, setTwoFASecret] = useState('')
  const [qrCodeURL, setQrCodeURL] = useState('')
  const [verificationCode, setVerificationCode] = useState('')
  const [backupCodes, setBackupCodes] = useState<string[]>([])
  const [twoFAError, setTwoFAError] = useState('')
  const [twoFASuccess, setTwoFASuccess] = useState('')
  const [isSettingUp2FA, setIsSettingUp2FA] = useState(false)
  const [isEnabling2FA, setIsEnabling2FA] = useState(false)
  const [showBackupCodes, setShowBackupCodes] = useState(false)
  const [disablePassword, setDisablePassword] = useState('')
  const [isDisabling2FA, setIsDisabling2FA] = useState(false)

  // API Keys state
  const [apiKeys, setApiKeys] = useState<APIKey[]>([])
  const [newKeyName, setNewKeyName] = useState('')
  const [newKeyExpires, setNewKeyExpires] = useState<number | undefined>(90)
  const [createdKey, setCreatedKey] = useState<{ key: string; name: string } | null>(null)
  const [apiKeyError, setApiKeyError] = useState('')
  const [apiKeySuccess, setApiKeySuccess] = useState('')
  const [copiedSnippet, setCopiedSnippet] = useState<string | null>(null)
  const [isCreatingKey, setIsCreatingKey] = useState(false)
  const [isDeletingKey, setIsDeletingKey] = useState<number | null>(null)

  // Cloudinary state
  const [cloudName, setCloudName] = useState('')
  const [cloudAPIKey, setCloudAPIKey] = useState('')
  const [cloudAPISecret, setCloudAPISecret] = useState('')
  const [cloudMaxSize, setCloudMaxSize] = useState(10)
  const [hasCloudinaryCredentials, setHasCloudinaryCredentials] = useState(false)
  const [cloudinaryError, setCloudinaryError] = useState('')
  const [cloudinarySuccess, setCloudinarySuccess] = useState('')
  const [isSavingCloudinary, setIsSavingCloudinary] = useState(false)
  const [isDeletingCloudinary, setIsDeletingCloudinary] = useState(false)
  const [cloudinaryStatus, setCloudinaryStatus] = useState<'unknown' | 'connected' | 'error' | 'suspended'>('unknown')
  const [cloudinaryLastChecked, setCloudinaryLastChecked] = useState<string | null>(null)
  const [cloudinaryLastError, setCloudinaryLastError] = useState('')
  const [cloudinaryConsecutiveFailures, setCloudinaryConsecutiveFailures] = useState(0)
  const [isTestingCloudinary, setIsTestingCloudinary] = useState(false)

  // Figma state
  const [figmaToken, setFigmaToken] = useState('')
  const [hasFigmaCredentials, setHasFigmaCredentials] = useState(false)
  const [figmaError, setFigmaError] = useState('')
  const [figmaSuccess, setFigmaSuccess] = useState('')
  const [isSavingFigma, setIsSavingFigma] = useState(false)
  const [isDeletingFigma, setIsDeletingFigma] = useState(false)

  // Google Backup state
  const [backupStatus, setBackupStatus] = useState<BackupStatus | null>(null)
  const [backupSettings, setBackupSettings] = useState<BackupSettings | null>(null)
  const [backupHistory, setBackupHistory] = useState<BackupRecord[]>([])
  const [backupCronExpr, setBackupCronExpr] = useState('0 3 * * *')
  const [backupFolderId, setBackupFolderId] = useState('')
  const [backupEnabled, setBackupEnabled] = useState(false)
  const [isBackupAvailable, setIsBackupAvailable] = useState(false)
  const [isLoadingBackup, setIsLoadingBackup] = useState(false)
  const [isTriggeringBackup, setIsTriggeringBackup] = useState(false)
  const [isSavingBackupSettings, setIsSavingBackupSettings] = useState(false)
  const [isDisconnectingBackup, setIsDisconnectingBackup] = useState(false)
  const [backupError, setBackupError] = useState('')
  const [backupSuccess, setBackupSuccess] = useState('')

  // Team Management state
  const [team, setTeam] = useState<Team | null>(null)
  const [teamMembers, setTeamMembers] = useState<TeamMember[]>([])
  const [invitations, setInvitations] = useState<TeamInvitation[]>([])
  const [inviteEmail, setInviteEmail] = useState('')
  const [teamError, setTeamError] = useState('')
  const [teamSuccess, setTeamSuccess] = useState('')
  const [isInviting, setIsInviting] = useState(false)
  const [isRemovingMember, setIsRemovingMember] = useState<number | null>(null)
  const [isRespondingToInvitation, setIsRespondingToInvitation] = useState<number | null>(null)
  const [isEditingTeamName, setIsEditingTeamName] = useState(false)
  const [editTeamName, setEditTeamName] = useState('')
  const [isSavingTeamName, setIsSavingTeamName] = useState(false)
  const [sentInvitations, setSentInvitations] = useState<SentInvitation[]>([])
  const [searchResults, setSearchResults] = useState<UserSearchResult[]>([])
  const [isSearching, setIsSearching] = useState(false)
  const [selectedUser, setSelectedUser] = useState<UserSearchResult | null>(null)
  const [showSearchDropdown, setShowSearchDropdown] = useState(false)
  const [otherTeams, setOtherTeams] = useState<TeamMembership[]>([])

  // Project invitations (received)
  const [projectInvitations, setProjectInvitations] = useState<ProjectInvitation[]>([])
  const [projectInviteError, setProjectInviteError] = useState('')
  const [projectInviteSuccess, setProjectInviteSuccess] = useState('')
  const [isRespondingToProjectInvite, setIsRespondingToProjectInvite] = useState<number | null>(null)

  // Invite system state
  const [myInvites, setMyInvites] = useState<Invite[]>([])
  const [myInviteCount, setMyInviteCount] = useState(0)
  const [isUserAdmin, setIsUserAdmin] = useState(false)
  const [isCreatingInvite, setIsCreatingInvite] = useState(false)
  const [inviteError, setInviteError] = useState('')
  const [inviteSuccess, setInviteSuccess] = useState('')
  const [newInviteCode, setNewInviteCode] = useState('')
  const [inviteRecipientEmail, setInviteRecipientEmail] = useState('')

  useEffect(() => {
    loadProfile()
    load2FAStatus()
    loadAPIKeys()
    loadTeamData()
    loadCloudinaryCredentials()
    loadFigmaCredentials()
    loadBackupData()
    loadInvites()
    loadProjectInvitations()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const loadProfile = async () => {
    try {
      const me = await apiClient.getCurrentUser()
      setProfileFirstName(me.first_name || '')
      setProfileLastName(me.last_name || '')
      setHasPassword(me.has_password !== false) // default true; false only when explicitly set
    } catch {
      // non-critical load failure
    }
  }

  const handleProfileSave = async (e: React.FormEvent) => {
    e.preventDefault()
    setProfileError('')
    setProfileSuccess('')
    setIsSavingProfile(true)
    try {
      await apiClient.updateProfile({ first_name: profileFirstName.trim(), last_name: profileLastName.trim() })
      setProfileSuccess('Profile updated successfully')
    } catch (error: unknown) {
      setProfileError(error instanceof Error ? error.message : 'Failed to update profile')
    } finally {
      setIsSavingProfile(false)
    }
  }

  const load2FAStatus = async () => {
    try {
      const status = await apiClient.get2FAStatus()
      setTwoFAEnabled(status.enabled)
    } catch (error) {
      console.error('Failed to load data:', error)
    }
  }

  const handlePasswordChange = async (e: React.FormEvent) => {
    e.preventDefault()
    setPasswordError('')
    setPasswordSuccess('')

    if (newPassword !== confirmPassword) {
      setPasswordError('Passwords do not match')
      return
    }

    if (newPassword.length < 8) {
      setPasswordError('Password must be at least 8 characters')
      return
    }

    setIsChangingPassword(true)

    try {
      await apiClient.changePassword({
        current_password: hasPassword ? currentPassword : '',
        new_password: newPassword,
      })

      setPasswordSuccess(hasPassword ? 'Password changed successfully' : 'Password set successfully')
      setHasPassword(true)
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
    } catch (error: unknown) {
      setPasswordError(error instanceof Error ? error.message : 'Failed to change password')
    } finally {
      setIsChangingPassword(false)
    }
  }

  const handleSetup2FA = async () => {
    setTwoFAError('')
    setIsSettingUp2FA(true)

    try {
      const response = await apiClient.setup2FA()
      setTwoFASecret(response.secret)
      setQrCodeURL(response.qr_code_url)
      setShowBackupCodes(false)
    } catch (error: unknown) {
      setTwoFAError(error instanceof Error ? error.message : 'Failed to setup 2FA')
    } finally {
      setIsSettingUp2FA(false)
    }
  }

  const handleEnable2FA = async (e: React.FormEvent) => {
    e.preventDefault()
    setTwoFAError('')
    setTwoFASuccess('')

    if (!verificationCode || verificationCode.length !== 6) {
      setTwoFAError('Please enter a 6-digit verification code')
      return
    }

    setIsEnabling2FA(true)

    try {
      const response = await apiClient.enable2FA({ code: verificationCode })
      setBackupCodes(response.backup_codes)
      setShowBackupCodes(true)
      setTwoFAEnabled(true)
      setTwoFASuccess('2FA enabled successfully! Save your backup codes.')
      setVerificationCode('')
      setQrCodeURL('')
      setTwoFASecret('')
    } catch (error: unknown) {
      setTwoFAError(error instanceof Error ? error.message : 'Invalid verification code')
    } finally {
      setIsEnabling2FA(false)
    }
  }

  const handleDisable2FA = async (e: React.FormEvent) => {
    e.preventDefault()
    setTwoFAError('')
    setTwoFASuccess('')

    if (!disablePassword) {
      setTwoFAError('Password is required to disable 2FA')
      return
    }

    setIsDisabling2FA(true)

    try {
      await apiClient.disable2FA({ password: disablePassword })
      setTwoFAEnabled(false)
      setTwoFASuccess('2FA disabled successfully')
      setDisablePassword('')
      setBackupCodes([])
      setShowBackupCodes(false)
    } catch (error: unknown) {
      setTwoFAError(error instanceof Error ? error.message : 'Failed to disable 2FA')
    } finally {
      setIsDisabling2FA(false)
    }
  }

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text)
  }

  const copyAllBackupCodes = () => {
    const allCodes = backupCodes.join('\n')
    copyToClipboard(allCodes)
    setTwoFASuccess('All backup codes copied to clipboard')
  }

  const copySecret = () => {
    copyToClipboard(twoFASecret)
    setTwoFASuccess('Secret key copied to clipboard')
  }

  const applyCloudinaryHealth = (cred: CloudinaryCredentialResponse) => {
    setCloudinaryStatus(cred.status)
    setCloudinaryLastChecked(cred.last_checked_at)
    setCloudinaryLastError(cred.last_error)
    setCloudinaryConsecutiveFailures(cred.consecutive_failures)
  }

  const loadCloudinaryCredentials = async () => {
    try {
      const cred = await apiClient.getCloudinaryCredential()
      if (cred) {
        setCloudName(cred.cloud_name)
        setCloudAPIKey(cred.api_key)
        setCloudMaxSize(cred.max_file_size_mb || 10)
        setHasCloudinaryCredentials(true)
        applyCloudinaryHealth(cred)

        // Auto-test if last check was > 24h ago and not suspended
        if (cred.status !== 'suspended') {
          const lastCheck = cred.last_checked_at ? new Date(cred.last_checked_at).getTime() : 0
          const dayAgo = Date.now() - 24 * 60 * 60 * 1000
          if (lastCheck < dayAgo) {
            handleTestCloudinary()
          }
        }
      }
    } catch (error) {
      console.error('Failed to load data:', error)
    }
  }

  const loadFigmaCredentials = async () => {
    try {
      const status: FigmaCredentialsStatus = await apiClient.getFigmaCredentials()
      setHasFigmaCredentials(status.configured)
    } catch {
      // ignore
    }
  }

  const handleSaveFigma = async (e: React.FormEvent) => {
    e.preventDefault()
    setFigmaError('')
    setFigmaSuccess('')
    const token = figmaToken.trim()
    if (!token) {
      setFigmaError('Personal access token is required')
      return
    }
    if (!token.startsWith('figd_')) {
      setFigmaError('Token must start with figd_')
      return
    }
    setIsSavingFigma(true)
    try {
      await apiClient.saveFigmaCredentials(token)
      setHasFigmaCredentials(true)
      setFigmaToken('')
      setFigmaSuccess('Figma token saved successfully')
    } catch (error: unknown) {
      setFigmaError(error instanceof Error ? error.message : 'Failed to save token')
    } finally {
      setIsSavingFigma(false)
    }
  }

  const handleDeleteFigma = async () => {
    if (!confirm('Are you sure you want to remove your Figma token?')) return
    setIsDeletingFigma(true)
    setFigmaError('')
    setFigmaSuccess('')
    try {
      await apiClient.deleteFigmaCredentials()
      setHasFigmaCredentials(false)
      setFigmaToken('')
      setFigmaSuccess('Figma token removed')
    } catch (error: unknown) {
      setFigmaError(error instanceof Error ? error.message : 'Failed to remove token')
    } finally {
      setIsDeletingFigma(false)
    }
  }

  const loadBackupData = async () => {
    setIsLoadingBackup(true)
    try {
      const [status, settings, history] = await Promise.all([
        apiClient.getBackupStatus(),
        apiClient.getBackupSettings(),
        apiClient.listBackupHistory(10),
      ])
      setBackupStatus(status)
      setBackupSettings(settings)
      setBackupHistory(history)
      setBackupEnabled(settings.enabled)
      setBackupCronExpr(settings.cron_expression || '0 3 * * *')
      setBackupFolderId(settings.folder_id || '')
      setIsBackupAvailable(true)
    } catch {
      // Backup not configured — hide the section silently
      setIsBackupAvailable(false)
    } finally {
      setIsLoadingBackup(false)
    }
  }

  const handleSaveBackupSettings = async (e: React.FormEvent) => {
    e.preventDefault()
    setIsSavingBackupSettings(true)
    setBackupError('')
    setBackupSuccess('')
    try {
      const updated = await apiClient.updateBackupSettings({
        enabled: backupEnabled,
        cron_expression: backupCronExpr,
        folder_id: backupFolderId,
      })
      setBackupSettings(updated)
      setBackupSuccess('Backup settings saved')
    } catch (error: unknown) {
      setBackupError(error instanceof Error ? error.message : 'Failed to save settings')
    } finally {
      setIsSavingBackupSettings(false)
    }
  }

  const handleTriggerBackup = async () => {
    setIsTriggeringBackup(true)
    setBackupError('')
    setBackupSuccess('')
    try {
      await apiClient.triggerBackup()
      setBackupSuccess('Backup started — check history in a moment')
      setTimeout(() => loadBackupData(), 3000)
    } catch (error: unknown) {
      setBackupError(error instanceof Error ? error.message : 'Failed to trigger backup')
    } finally {
      setIsTriggeringBackup(false)
    }
  }

  const handleDisconnectBackup = async () => {
    if (!confirm('Disconnect Google Drive? Scheduled backups will be disabled.')) return
    setIsDisconnectingBackup(true)
    setBackupError('')
    try {
      await apiClient.disconnectBackup()
      setBackupSuccess('Google Drive disconnected')
      loadBackupData()
    } catch (error: unknown) {
      setBackupError(error instanceof Error ? error.message : 'Failed to disconnect')
    } finally {
      setIsDisconnectingBackup(false)
    }
  }

  const handleDeleteBackupRecord = async (id: string) => {
    if (!confirm('Delete this backup record and its remote file?')) return
    try {
      await apiClient.deleteBackupRecord(id)
      setBackupHistory(prev => prev.filter(r => r.id !== id))
    } catch (error: unknown) {
      setBackupError(error instanceof Error ? error.message : 'Failed to delete record')
    }
  }

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`
  }

  const handleSaveCloudinary = async (e: React.FormEvent) => {
    e.preventDefault()
    setCloudinaryError('')
    setCloudinarySuccess('')

    if (!cloudName.trim() || !cloudAPIKey.trim() || !cloudAPISecret.trim()) {
      setCloudinaryError('All fields are required')
      return
    }

    setIsSavingCloudinary(true)
    try {
      const cred = await apiClient.saveCloudinaryCredential({
        cloud_name: cloudName.trim(),
        api_key: cloudAPIKey.trim(),
        api_secret: cloudAPISecret.trim(),
        max_file_size_mb: cloudMaxSize,
      })
      setHasCloudinaryCredentials(true)
      setCloudAPISecret('')
      applyCloudinaryHealth(cred)

      if (cred.status === 'connected') {
        setCloudinarySuccess('Credentials saved and connection verified')
      } else {
        setCloudinarySuccess('Credentials saved')
        setCloudinaryError(cred.last_error || 'Connection test failed')
      }
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : 'Failed to save credentials'
      setCloudinaryError(message)
    } finally {
      setIsSavingCloudinary(false)
    }
  }

  const handleTestCloudinary = async () => {
    setIsTestingCloudinary(true)
    setCloudinaryError('')
    setCloudinarySuccess('')
    try {
      const cred = await apiClient.testCloudinaryConnection()
      applyCloudinaryHealth(cred)
      if (cred.status === 'connected') {
        setCloudinarySuccess('Connection verified')
      } else {
        setCloudinaryError(cred.last_error || 'Connection test failed')
      }
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : 'Failed to test connection'
      setCloudinaryError(message)
    } finally {
      setIsTestingCloudinary(false)
    }
  }

  const handleDeleteCloudinary = async () => {
    if (!confirm('Are you sure you want to remove your Cloudinary credentials?')) return

    setIsDeletingCloudinary(true)
    setCloudinaryError('')
    setCloudinarySuccess('')
    try {
      await apiClient.deleteCloudinaryCredential()
      setCloudName('')
      setCloudAPIKey('')
      setCloudAPISecret('')
      setCloudMaxSize(10)
      setHasCloudinaryCredentials(false)
      setCloudinaryStatus('unknown')
      setCloudinaryLastChecked(null)
      setCloudinaryLastError('')
      setCloudinaryConsecutiveFailures(0)
      setCloudinarySuccess('Cloudinary credentials removed')
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : 'Failed to remove credentials'
      setCloudinaryError(message)
    } finally {
      setIsDeletingCloudinary(false)
    }
  }

  const loadInvites = async () => {
    try {
      const data = await apiClient.getInvites()
      setMyInvites(data.invites || [])
      setMyInviteCount(data.invite_count)
      setIsUserAdmin(data.is_admin)
    } catch (error) {
      console.error('Failed to load data:', error)
    }
  }

  const loadProjectInvitations = async () => {
    try {
      const data = await apiClient.getMyProjectInvitations()
      setProjectInvitations(data)
    } catch {
      // non-critical
    }
  }

  const handleAcceptProjectInvite = async (invId: number) => {
    setProjectInviteError('')
    setProjectInviteSuccess('')
    setIsRespondingToProjectInvite(invId)
    try {
      await apiClient.acceptProjectInvitation(invId)
      setProjectInviteSuccess('You have joined the project!')
      loadProjectInvitations()
    } catch (error: unknown) {
      setProjectInviteError(error instanceof Error ? error.message : 'Failed to accept invitation')
    } finally {
      setIsRespondingToProjectInvite(null)
    }
  }

  const handleRejectProjectInvite = async (invId: number) => {
    setProjectInviteError('')
    setProjectInviteSuccess('')
    setIsRespondingToProjectInvite(invId)
    try {
      await apiClient.rejectProjectInvitation(invId)
      setProjectInviteSuccess('Invitation declined')
      loadProjectInvitations()
    } catch (error: unknown) {
      setProjectInviteError(error instanceof Error ? error.message : 'Failed to decline invitation')
    } finally {
      setIsRespondingToProjectInvite(null)
    }
  }

  const handleCreateInviteCode = async () => {
    setInviteError('')
    setInviteSuccess('')
    setNewInviteCode('')
    setIsCreatingInvite(true)

    try {
      const email = inviteRecipientEmail.trim() || undefined
      const result = await apiClient.createInvite(email)
      setNewInviteCode(result.code)
      if (result.email_sent && email) {
        setInviteSuccess(`Invite created and email sent to ${email}!`)
      } else {
        setInviteSuccess('Invite created! Share the link below.')
      }
      setInviteRecipientEmail('')
      await loadInvites()
    } catch (error: unknown) {
      setInviteError(error instanceof Error ? error.message : 'Failed to create invite')
    } finally {
      setIsCreatingInvite(false)
    }
  }

  const copyInviteLink = (code: string) => {
    const url = `${window.location.origin}/signup?code=${code}`
    navigator.clipboard.writeText(url)
    setInviteSuccess('Invite link copied to clipboard')
  }

  const loadAPIKeys = async () => {
    try {
      const keys = await apiClient.getAPIKeys()
      setApiKeys(keys)
    } catch (error) {
      console.error('Failed to load data:', error)
    }
  }

  const handleCreateAPIKey = async (e: React.FormEvent) => {
    e.preventDefault()
    setApiKeyError('')
    setApiKeySuccess('')

    if (!newKeyName.trim()) {
      setApiKeyError('Key name is required')
      return
    }

    setIsCreatingKey(true)

    try {
      const response = await apiClient.createAPIKey({
        name: newKeyName,
        expires_in: newKeyExpires,
      })

      setCreatedKey({ key: response.key, name: response.name })
      setApiKeySuccess('API key created successfully')
      setNewKeyName('')
      setNewKeyExpires(90)
      await loadAPIKeys()
    } catch (error: unknown) {
      setApiKeyError(error instanceof Error ? error.message : 'Failed to create API key')
    } finally {
      setIsCreatingKey(false)
    }
  }

  const handleDeleteAPIKey = async (id: number, name: string) => {
    if (!confirm(`Are you sure you want to delete the API key "${name}"?`)) {
      return
    }

    setIsDeletingKey(id)
    setApiKeyError('')
    setApiKeySuccess('')

    try {
      await apiClient.deleteAPIKey(id)
      setApiKeySuccess('API key deleted successfully')
      await loadAPIKeys()
    } catch (error: unknown) {
      setApiKeyError(error instanceof Error ? error.message : 'Failed to delete API key')
    } finally {
      setIsDeletingKey(null)
    }
  }

  const copyAPIKey = (key: string) => {
    copyToClipboard(key)
    setApiKeySuccess('API key copied to clipboard')
  }

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString()
  }

  const loadTeamData = async () => {
    try {
      const [teamData, membersData, invitationsData, sentInvData, otherTeamsData] = await Promise.all([
        apiClient.getMyTeam(),
        apiClient.getTeamMembers(),
        apiClient.getMyInvitations(),
        apiClient.getTeamSentInvitations(),
        apiClient.getMyTeamMemberships(),
      ])
      setTeam(teamData)
      setTeamMembers(membersData)
      setInvitations(invitationsData)
      setSentInvitations(sentInvData)
      setOtherTeams(otherTeamsData)
    } catch (error) {
      console.error('Failed to load data:', error)
    }
  }

  const handleRemoveMember = async (memberId: number, memberName: string) => {
    if (!confirm(`Are you sure you want to remove ${memberName} from the team?`)) {
      return
    }

    setIsRemovingMember(memberId)
    setTeamError('')
    setTeamSuccess('')

    try {
      await apiClient.removeTeamMember(memberId)
      setTeamSuccess('Member removed successfully')
      await loadTeamData()
    } catch (error: unknown) {
      setTeamError(error instanceof Error ? error.message : 'Failed to remove member')
    } finally {
      setIsRemovingMember(null)
    }
  }

  const handleAcceptInvitation = async (invitationId: number) => {
    setIsRespondingToInvitation(invitationId)
    setTeamError('')
    setTeamSuccess('')

    try {
      await apiClient.acceptInvitation(invitationId)
      setTeamSuccess('Invitation accepted! Reloading team data...')
      await loadTeamData()
    } catch (error: unknown) {
      setTeamError(error instanceof Error ? error.message : 'Failed to accept invitation')
    } finally {
      setIsRespondingToInvitation(null)
    }
  }

  const handleRejectInvitation = async (invitationId: number) => {
    setIsRespondingToInvitation(invitationId)
    setTeamError('')
    setTeamSuccess('')

    try {
      await apiClient.rejectInvitation(invitationId)
      setTeamSuccess('Invitation rejected')
      await loadTeamData()
    } catch (error: unknown) {
      setTeamError(error instanceof Error ? error.message : 'Failed to reject invitation')
    } finally {
      setIsRespondingToInvitation(null)
    }
  }

  const handleSaveTeamName = async () => {
    const trimmed = editTeamName.trim()
    if (!trimmed) {
      setTeamError('Team name is required')
      return
    }
    setIsSavingTeamName(true)
    setTeamError('')
    setTeamSuccess('')

    try {
      const updated = await apiClient.updateTeam(trimmed)
      setTeam(updated)
      setIsEditingTeamName(false)
      setTeamSuccess('Team name updated')
    } catch (error: unknown) {
      setTeamError(error instanceof Error ? error.message : 'Failed to update team name')
    } finally {
      setIsSavingTeamName(false)
    }
  }

  const handleSearchUsers = async (query: string) => {
    setInviteEmail(query)
    setSelectedUser(null)

    if (query.trim().length < 2) {
      setSearchResults([])
      setShowSearchDropdown(false)
      return
    }

    setIsSearching(true)
    try {
      const results = await apiClient.searchTeamUsers(query.trim())
      setSearchResults(results)
      setShowSearchDropdown(results.length > 0)
    } catch {
      setSearchResults([])
      setShowSearchDropdown(false)
    } finally {
      setIsSearching(false)
    }
  }

  const handleSelectUser = (u: UserSearchResult) => {
    setSelectedUser(u)
    setInviteEmail(u.name ? `${u.name} (${u.email})` : u.email)
    setShowSearchDropdown(false)
    setSearchResults([])
  }

  const handleInviteOrAdd = async (e: React.FormEvent) => {
    e.preventDefault()
    setTeamError('')
    setTeamSuccess('')

    if (selectedUser) {
      // Direct add existing user
      setIsInviting(true)
      try {
        await apiClient.addTeamMember(selectedUser.id)
        setTeamSuccess(`${selectedUser.name || selectedUser.email} added to the team`)
        setInviteEmail('')
        setSelectedUser(null)
        await loadTeamData()
      } catch (error: unknown) {
        setTeamError(error instanceof Error ? error.message : 'Failed to add member')
      } finally {
        setIsInviting(false)
      }
    } else {
      // Fall back to email invite
      const email = inviteEmail.trim()
      if (!email) {
        setTeamError('Email is required')
        return
      }
      setIsInviting(true)
      try {
        await apiClient.inviteTeamMember(email)
        setTeamSuccess(`Invitation sent to ${email}`)
        setInviteEmail('')
        await loadTeamData()
      } catch (error: unknown) {
        setTeamError(error instanceof Error ? error.message : 'Failed to send invitation')
      } finally {
        setIsInviting(false)
      }
    }
  }

  // Debounce user search
  useEffect(() => {
    if (selectedUser) return
    const query = inviteEmail.trim()
    if (query.length < 2) {
      setSearchResults([])
      setShowSearchDropdown(false)
      return
    }
    const timer = setTimeout(() => {
      handleSearchUsers(inviteEmail)
    }, 300)
    return () => clearTimeout(timer)
  }, [inviteEmail, selectedUser])

  return (
    <div className="min-h-screen bg-dark-bg-primary py-8">
      <div className="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8">
        {/* Header */}
        <div className="mb-8">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-3xl font-bold text-dark-text-primary">Account Settings</h1>
              <p className="text-dark-text-secondary mt-1">Manage your security and authentication preferences</p>
            </div>
            <Button onClick={() => navigate('/app')} variant="secondary">
              <svg className="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" />
              </svg>
              Back
            </Button>
          </div>
        </div>

        <div className="space-y-6">
          {/* Profile Section */}
          <Card className="shadow-md">
            <div className="p-6 sm:p-8 flex items-start gap-4">
              <div className="flex-shrink-0 w-10 h-10 bg-primary-500/10 rounded-lg flex items-center justify-center">
                <svg className="w-6 h-6 text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
                </svg>
              </div>
              <div className="flex-1">
                <h2 className="text-xl font-semibold text-dark-text-primary mb-1">Profile</h2>
                <p className="text-sm text-dark-text-secondary mb-6">Update your display name</p>

                {profileSuccess && (
                  <div className="mb-4 p-4 bg-success-500/10 border-l-4 border-success-400 rounded-r-lg">
                    <div className="flex items-center">
                      <svg className="w-5 h-5 text-success-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                        <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                      </svg>
                      <span className="text-sm text-success-300">{profileSuccess}</span>
                    </div>
                  </div>
                )}

                <FormError message={profileError} className="mb-4" />

                <form onSubmit={handleProfileSave} className="space-y-4 max-w-md">
                  <div className="grid grid-cols-2 gap-3">
                    <TextInput
                      label="First Name"
                      value={profileFirstName}
                      onChange={(e) => setProfileFirstName(e.target.value)}
                      placeholder="First"
                    />
                    <TextInput
                      label="Last Name"
                      value={profileLastName}
                      onChange={(e) => setProfileLastName(e.target.value)}
                      placeholder="Last"
                    />
                  </div>

                  <Button type="submit" disabled={isSavingProfile} size="sm">
                    {isSavingProfile ? 'Saving...' : 'Save Profile'}
                  </Button>
                </form>
              </div>
            </div>
          </Card>

          {/* Password Change Section */}
          <Card className="shadow-md">
            <div className="p-6 sm:p-8 flex items-start gap-4">
              <div className="flex-shrink-0 w-10 h-10 bg-primary-500/10 rounded-lg flex items-center justify-center">
                <svg className="w-6 h-6 text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
                </svg>
              </div>
              <div className="flex-1">
                <h2 className="text-xl font-semibold text-dark-text-primary mb-1">
                  {hasPassword ? 'Change Password' : 'Set a Password'}
                </h2>
                <p className="text-sm text-dark-text-secondary mb-6">
                  {hasPassword
                    ? 'Update your password to keep your account secure'
                    : 'Your account uses social login. You can set a password to also sign in with email.'}
                </p>

                {passwordSuccess && (
                  <div className="mb-4 p-4 bg-success-500/10 border-l-4 border-success-400 rounded-r-lg">
                    <div className="flex items-center">
                      <svg className="w-5 h-5 text-success-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                        <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                      </svg>
                      <span className="text-success-300 font-medium">{passwordSuccess}</span>
                    </div>
                  </div>
                )}

                <form onSubmit={handlePasswordChange} className="space-y-4">
                  <TextInput
                    label="Current Password"
                    type="password"
                    value={currentPassword}
                    onChange={(e) => setCurrentPassword(e.target.value)}
                    required={hasPassword}
                    disabled={!hasPassword}
                    autoComplete="current-password"
                    helpText={!hasPassword ? 'Not set — your account uses social login' : undefined}
                  />

                  <TextInput
                    label="New Password"
                    type="password"
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                    required
                    autoComplete="new-password"
                    helpText="Must be at least 8 characters with a letter and number"
                  />

                  <TextInput
                    label="Confirm New Password"
                    type="password"
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    required
                    autoComplete="new-password"
                  />

                  {passwordError && <FormError message={passwordError} />}

                  <Button
                    type="submit"
                    disabled={isChangingPassword}
                    className="w-full sm:w-auto"
                  >
                    {isChangingPassword
                      ? (hasPassword ? 'Changing Password...' : 'Setting Password...')
                      : (hasPassword ? 'Change Password' : 'Set Password')}
                  </Button>
                </form>
              </div>
            </div>
          </Card>

          {/* Two-Factor Authentication Section */}
          <Card className="shadow-md">
            <div className="p-6 sm:p-8 flex items-start gap-4">
              <div className="flex-shrink-0 w-10 h-10 bg-purple-500/10 rounded-lg flex items-center justify-center">
                <svg className="w-6 h-6 text-purple-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
                </svg>
              </div>
              <div className="flex-1">
                <h2 className="text-xl font-semibold text-dark-text-primary mb-1">Two-Factor Authentication</h2>
                <p className="text-sm text-dark-text-secondary mb-6">Add an extra layer of security to your account</p>

                {/* Status Badge */}
                <div className="mb-6 p-4 bg-dark-bg-primary border border-dark-border-subtle rounded-lg flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <div className={`w-3 h-3 rounded-full ${twoFAEnabled ? 'bg-success-500' : 'bg-dark-text-tertiary'}`}></div>
                    <div>
                      <p className="font-medium text-dark-text-primary">Status</p>
                      <p className="text-sm text-dark-text-secondary">{twoFAEnabled ? 'Active' : 'Not configured'}</p>
                    </div>
                  </div>
                  <span className={`px-3 py-1 rounded-full text-sm font-medium ${
                    twoFAEnabled
                      ? 'bg-success-500/10 text-success-400'
                      : 'bg-dark-bg-secondary text-dark-text-tertiary'
                  }`}>
                    {twoFAEnabled ? '✓ Enabled' : 'Disabled'}
                  </span>
                </div>

                {twoFASuccess && (
                  <div className="mb-4 p-4 bg-success-500/10 border-l-4 border-success-400 rounded-r-lg">
                    <div className="flex items-center">
                      <svg className="w-5 h-5 text-success-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                        <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                      </svg>
                      <span className="text-success-300 font-medium">{twoFASuccess}</span>
                    </div>
                  </div>
                )}

                {twoFAError && <FormError message={twoFAError} className="mb-4" />}

                {/* Enable 2FA Flow */}
                {!twoFAEnabled && !qrCodeURL && (
                  <div>
                    <div className="bg-primary-500/10 border border-primary-500/30 rounded-lg p-4 mb-4">
                      <div className="flex gap-3">
                        <svg className="w-5 h-5 text-primary-400 flex-shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
                          <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd" />
                        </svg>
                        <div className="text-sm text-dark-text-secondary">
                          <p className="font-medium mb-1 text-dark-text-primary">How it works</p>
                          <p>Two-factor authentication adds an extra security layer. You'll need your password and a verification code from your phone to sign in.</p>
                        </div>
                      </div>
                    </div>
                    <Button onClick={handleSetup2FA} disabled={isSettingUp2FA}>
                      {isSettingUp2FA ? 'Setting up...' : 'Enable 2FA'}
                    </Button>
                  </div>
                )}

                {/* QR Code Display */}
                {qrCodeURL && !twoFAEnabled && (
                  <div className="space-y-4">
                    <div className="bg-dark-bg-primary border-2 border-dark-border-subtle rounded-xl p-6">
                      <div className="flex items-center gap-2 mb-4">
                        <div className="w-8 h-8 bg-purple-500/10 rounded-full flex items-center justify-center text-purple-400 font-bold">1</div>
                        <h3 className="font-semibold text-dark-text-primary">Scan QR Code</h3>
                      </div>
                      <p className="text-sm text-dark-text-secondary mb-4">
                        Open your authenticator app (Google Authenticator, Authy, 1Password, etc.) and scan this code
                      </p>
                      <div className="flex flex-col items-center gap-4">
                        <div className="bg-white p-4 rounded-lg border-2 border-dark-border-subtle shadow-sm">
                          <img src={qrCodeURL} alt="2FA QR Code" className="w-48 h-48" />
                        </div>

                        <div className="w-full bg-dark-bg-secondary p-4 rounded-lg border border-dark-border-subtle">
                          <p className="text-xs font-medium text-dark-text-secondary mb-2">Manual Entry Key:</p>
                          <div className="flex items-center gap-2">
                            <code className="flex-1 text-sm font-mono bg-dark-bg-primary text-dark-text-primary px-3 py-2 rounded border border-dark-border-subtle break-all">
                              {twoFASecret}
                            </code>
                            <Button size="sm" variant="secondary" onClick={copySecret}>
                              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                              </svg>
                            </Button>
                          </div>
                        </div>
                      </div>
                    </div>

                    <form onSubmit={handleEnable2FA} className="bg-dark-bg-primary border-2 border-dark-border-subtle rounded-xl p-6">
                      <div className="flex items-center gap-2 mb-4">
                        <div className="w-8 h-8 bg-purple-500/10 rounded-full flex items-center justify-center text-purple-400 font-bold">2</div>
                        <h3 className="font-semibold text-dark-text-primary">Enter Verification Code</h3>
                      </div>
                      <p className="text-sm text-dark-text-secondary mb-4">
                        Enter the 6-digit code shown in your authenticator app
                      </p>
                      <div className="flex gap-3">
                        <input
                          type="text"
                          value={verificationCode}
                          onChange={(e) => setVerificationCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                          placeholder="000000"
                          maxLength={6}
                          pattern="\d{6}"
                          required
                          className="flex-1 text-center text-3xl font-mono tracking-widest px-4 py-3 border-2 border-dark-border-subtle bg-dark-bg-secondary text-dark-text-primary rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-500/20 outline-none transition-colors placeholder-dark-text-tertiary"
                        />
                        <Button type="submit" disabled={isEnabling2FA || verificationCode.length !== 6}>
                          {isEnabling2FA ? 'Verifying...' : 'Verify & Enable'}
                        </Button>
                      </div>
                    </form>
                  </div>
                )}

                {/* Backup Codes Display */}
                {showBackupCodes && backupCodes.length > 0 && (
                  <div className="border-2 border-yellow-500/30 bg-yellow-500/10 rounded-xl p-6">
                    <div className="flex items-start gap-3 mb-4">
                      <svg className="w-6 h-6 text-yellow-400 flex-shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
                        <path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
                      </svg>
                      <div>
                        <h3 className="font-bold text-yellow-300 mb-1">Save Your Backup Codes</h3>
                        <p className="text-sm text-yellow-400/90">
                          Store these codes in a safe place. You can use them to access your account if you lose your device. Each code can only be used once.
                        </p>
                      </div>
                    </div>

                    <div className="bg-dark-bg-primary p-5 rounded-lg border-2 border-yellow-500/30 mb-4">
                      <div className="grid grid-cols-2 gap-3">
                        {backupCodes.map((code, index) => (
                          <div key={index} className="flex items-center justify-between p-3 bg-dark-bg-secondary rounded-lg border border-dark-border-subtle">
                            <span className="font-mono text-sm font-medium text-dark-text-primary">{code}</span>
                            <button
                              onClick={() => {
                                copyToClipboard(code)
                                setTwoFASuccess(`Code ${index + 1} copied`)
                              }}
                              className="text-primary-400 hover:text-primary-300 text-xs font-medium px-2 py-1 rounded hover:bg-primary-500/10 transition-colors"
                            >
                              Copy
                            </button>
                          </div>
                        ))}
                      </div>
                    </div>

                    <Button onClick={copyAllBackupCodes} variant="secondary" className="w-full">
                      <svg className="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                      </svg>
                      Copy All Codes
                    </Button>
                  </div>
                )}

                {/* Disable 2FA */}
                {twoFAEnabled && !showBackupCodes && (
                  <form onSubmit={handleDisable2FA} className="space-y-4">
                    <div className="bg-danger-500/10 border-l-4 border-danger-400 rounded-r-lg p-4">
                      <div className="flex items-start gap-3">
                        <svg className="w-5 h-5 text-danger-400 flex-shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
                          <path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
                        </svg>
                        <div className="flex-1">
                          <h3 className="font-semibold text-danger-300 mb-1">Disable Two-Factor Authentication</h3>
                          <p className="text-sm text-danger-400/90 mb-4">
                            This will make your account less secure. Enter your password to confirm.
                          </p>

                          <TextInput
                            label="Password"
                            type="password"
                            value={disablePassword}
                            onChange={(e) => setDisablePassword(e.target.value)}
                            required
                            autoComplete="current-password"
                          />

                          <Button
                            type="submit"
                            variant="danger"
                            disabled={isDisabling2FA}
                            className="w-full mt-4"
                          >
                            {isDisabling2FA ? 'Disabling...' : 'Disable 2FA'}
                          </Button>
                        </div>
                      </div>
                    </div>
                  </form>
                )}
              </div>
            </div>
          </Card>

          {/* Cloudinary Section */}
          <Card className="shadow-md">
            <div className="p-6 sm:p-8 flex items-start gap-4">
              <div className="flex-shrink-0 w-10 h-10 bg-orange-500/10 rounded-lg flex items-center justify-center">
                <svg className="w-6 h-6 text-orange-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
                </svg>
              </div>
              <div className="flex-1">
                <h2 className="text-xl font-semibold text-dark-text-primary mb-1">Cloudinary Storage</h2>
                <p className="text-sm text-dark-text-secondary mb-6">
                  Connect your Cloudinary account to upload images, videos, and PDFs to tasks
                </p>

                {cloudinarySuccess && (
                  <div className="mb-4 p-4 bg-success-500/10 border-l-4 border-success-400 rounded-r-lg">
                    <div className="flex items-center">
                      <svg className="w-5 h-5 text-success-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                        <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                      </svg>
                      <span className="text-success-300 font-medium">{cloudinarySuccess}</span>
                    </div>
                  </div>
                )}

                {cloudinaryError && <FormError message={cloudinaryError} className="mb-4" />}

                {hasCloudinaryCredentials && (
                  <div className="mb-6 p-4 bg-dark-bg-primary border border-dark-border-subtle rounded-lg space-y-3">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-3">
                        <div className={`w-3 h-3 rounded-full ${
                          cloudinaryStatus === 'connected' ? 'bg-success-500' :
                          cloudinaryStatus === 'error' ? 'bg-danger-500' :
                          cloudinaryStatus === 'suspended' ? 'bg-yellow-500' :
                          'bg-dark-text-tertiary'
                        }`}></div>
                        <div>
                          <p className="font-medium text-dark-text-primary">
                            {cloudinaryStatus === 'connected' ? 'Connected' :
                             cloudinaryStatus === 'error' ? 'Connection Error' :
                             cloudinaryStatus === 'suspended' ? 'Suspended' :
                             'Unknown'}
                          </p>
                          <p className="text-sm text-dark-text-secondary">
                            Cloud: {cloudName} &middot; Max file size: {cloudMaxSize}MB
                            {cloudinaryLastChecked && (
                              <> &middot; Checked: {new Date(cloudinaryLastChecked).toLocaleString()}</>
                            )}
                          </p>
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={handleTestCloudinary}
                          disabled={isTestingCloudinary}
                        >
                          {isTestingCloudinary ? 'Testing...' : 'Test Connection'}
                        </Button>
                        <Button
                          size="sm"
                          variant="danger"
                          onClick={handleDeleteCloudinary}
                          disabled={isDeletingCloudinary}
                        >
                          {isDeletingCloudinary ? 'Removing...' : 'Remove'}
                        </Button>
                      </div>
                    </div>
                    {cloudinaryLastError && cloudinaryStatus !== 'connected' && (
                      <div className="text-sm text-danger-400 bg-danger-500/10 px-3 py-2 rounded">
                        {cloudinaryLastError}
                        {cloudinaryConsecutiveFailures >= 5 && (
                          <span className="block mt-1 text-yellow-400">
                            Auto-checks suspended after {cloudinaryConsecutiveFailures} consecutive failures. Use "Test Connection" to retry.
                          </span>
                        )}
                      </div>
                    )}
                  </div>
                )}

                <form onSubmit={handleSaveCloudinary} className="space-y-4">
                  <TextInput
                    label="Cloud Name"
                    value={cloudName}
                    onChange={(e) => setCloudName(e.target.value)}
                    placeholder="e.g. drfaxtpug"
                    helpText="Found in Cloudinary Console → Settings → API Keys. It's the short identifier after the @ in your CLOUDINARY_URL."
                    required
                  />

                  <TextInput
                    label="API Key"
                    value={cloudAPIKey}
                    onChange={(e) => setCloudAPIKey(e.target.value)}
                    placeholder="e.g. 587593568128219"
                    helpText="The numeric key from your Cloudinary API Keys page (console.cloudinary.com → Settings → API Keys)."
                    required
                  />

                  <TextInput
                    label={hasCloudinaryCredentials ? 'API Secret (enter to update)' : 'API Secret'}
                    type="password"
                    value={cloudAPISecret}
                    onChange={(e) => setCloudAPISecret(e.target.value)}
                    placeholder={hasCloudinaryCredentials ? '••••••••••••' : 'Enter your API secret'}
                    helpText="Click the eye icon next to your API key on the Cloudinary API Keys page to reveal it."
                    required={!hasCloudinaryCredentials}
                  />

                  <div>
                    <label className="block text-sm font-medium text-dark-text-primary mb-2">
                      Max File Size (MB)
                    </label>
                    <SearchSelect
                      value={String(cloudMaxSize)}
                      onChange={(v) => setCloudMaxSize(Number.parseInt(v))}
                      options={[
                        { value: '5', label: '5 MB' },
                        { value: '10', label: '10 MB' },
                        { value: '25', label: '25 MB' },
                        { value: '50', label: '50 MB' },
                        { value: '100', label: '100 MB' },
                      ]}
                    />
                  </div>

                  <Button type="submit" disabled={isSavingCloudinary}>
                    {isSavingCloudinary ? 'Saving...' : hasCloudinaryCredentials ? 'Update Credentials' : 'Save Credentials'}
                  </Button>
                </form>

                <div className="mt-6 bg-orange-500/10 border border-orange-500/30 rounded-lg p-4">
                  <div className="flex gap-3">
                    <svg className="w-5 h-5 text-orange-400 flex-shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
                      <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd" />
                    </svg>
                    <div className="text-sm text-dark-text-secondary">
                      <p className="font-medium mb-1 text-dark-text-primary">How file storage works</p>
                      <p className="mb-2">Files are uploaded directly to your Cloudinary account. In team projects, each member uses their own Cloudinary quota. Storage usage is tracked per user per project.</p>
                      <p>Find your credentials at <a href="https://console.cloudinary.com/settings/api-keys" target="_blank" rel="noopener noreferrer" className="text-orange-400 underline hover:text-orange-300">Cloudinary Console &rarr; Settings &rarr; API Keys</a>. Your cloud name, API key, and secret are all on that page.</p>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </Card>

          {/* Figma Section */}
          <Card className="shadow-md">
            <div className="p-6 sm:p-8 flex items-start gap-4">
              <div className="flex-shrink-0 w-10 h-10 bg-purple-500/10 rounded-lg flex items-center justify-center">
                <svg width="20" height="20" viewBox="0 0 38 57" fill="none" aria-hidden="true">
                  <path d="M19 28.5c0-2.674 1.054-5.24 2.929-7.115C23.804 19.51 26.37 18.457 29.043 18.457s5.239 1.053 7.115 2.928c1.875 1.875 2.929 4.441 2.929 7.115s-1.054 5.24-2.929 7.115C34.282 37.49 31.717 38.543 29.043 38.543s-5.239-1.053-7.114-2.928C20.054 33.74 19 31.174 19 28.5z" fill="#1ABCFE"/>
                  <path d="M-1.087 47.543c0-2.674 1.053-5.24 2.929-7.115C3.717 38.553 6.283 37.5 8.956 37.5H19v10.043c0 2.674-1.053 5.24-2.929 7.115C14.196 56.533 11.63 57.587 8.957 57.587s-5.239-1.054-7.115-2.929C-.033 52.782-1.087 50.217-1.087 47.543z" fill="#0ACF83"/>
                  <path d="M19 .413V18.457h10.043c2.674 0 5.239-1.053 7.115-2.929 1.875-1.875 2.929-4.44 2.929-7.114S38.033 3.174 36.158 1.298C34.282-.577 31.717-1.63 29.043-1.63H19V.413z" fill="#FF7262"/>
                  <path d="M-1.087 8.413c0 2.674 1.053 5.24 2.929 7.115C3.717 17.403 6.283 18.457 8.956 18.457H19V-1.587H8.956c-2.673 0-5.239 1.054-7.114 2.929C-.033 3.217-1.087 5.74-1.087 8.413z" fill="#F24E1E"/>
                  <path d="M-1.087 28.5c0 2.674 1.053 5.24 2.929 7.115C3.717 37.49 6.283 38.543 8.956 38.543H19V18.457H8.956c-2.673 0-5.239 1.053-7.114 2.928C-.033 23.261-1.087 25.826-1.087 28.5z" fill="#A259FF"/>
                </svg>
              </div>
              <div className="flex-1">
                <h2 className="text-xl font-semibold text-dark-text-primary mb-1">Figma Integration</h2>
                <p className="text-sm text-dark-text-secondary mb-6">
                  Connect your Figma account to embed design previews in wiki pages and task descriptions
                </p>

                {figmaSuccess && (
                  <div className="mb-4 p-4 bg-success-500/10 border-l-4 border-success-400 rounded-r-lg">
                    <div className="flex items-center">
                      <svg className="w-5 h-5 text-success-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                        <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                      </svg>
                      <p className="text-sm text-success-400">{figmaSuccess}</p>
                    </div>
                  </div>
                )}

                {figmaError && (
                  <div className="mb-4 p-4 bg-red-500/10 border-l-4 border-red-400 rounded-r-lg">
                    <p className="text-sm text-red-400">{figmaError}</p>
                  </div>
                )}

                {hasFigmaCredentials && (
                  <div className="mb-6 p-4 bg-dark-bg-primary border border-dark-border-subtle rounded-lg flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <div className="w-3 h-3 rounded-full bg-success-500" />
                      <span className="text-sm text-dark-text-primary font-medium">Connected</span>
                    </div>
                    <button
                      onClick={handleDeleteFigma}
                      disabled={isDeletingFigma}
                      className="px-3 py-1.5 rounded text-xs font-medium bg-red-500/15 text-red-400 hover:bg-red-500/25 transition-colors disabled:opacity-50"
                    >
                      {isDeletingFigma ? 'Removing...' : 'Disconnect'}
                    </button>
                  </div>
                )}

                <form onSubmit={handleSaveFigma} className="space-y-4">
                  <TextInput
                    label={hasFigmaCredentials ? 'Personal Access Token (enter to update)' : 'Personal Access Token'}
                    type="password"
                    value={figmaToken}
                    onChange={(e) => setFigmaToken(e.target.value)}
                    placeholder={hasFigmaCredentials ? '••••••••••••' : 'figd_...'}
                    helpText="Get your token at figma.com → Account Settings → Personal access tokens. Token must start with figd_."
                    required={!hasFigmaCredentials}
                  />
                  <Button type="submit" disabled={isSavingFigma}>
                    {isSavingFigma ? 'Saving...' : hasFigmaCredentials ? 'Update Token' : 'Save Token'}
                  </Button>
                </form>
              </div>
            </div>
          </Card>

          {/* Google Backup Section */}
          {isBackupAvailable && (
            <Card className="shadow-md" id="google-backup">
              <div className="p-6 sm:p-8 flex items-start gap-4">
                <div className="flex-shrink-0 w-10 h-10 bg-green-500/10 rounded-lg flex items-center justify-center">
                  <svg className="w-6 h-6 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M9 19l3 3m0 0l3-3m-3 3V10" />
                  </svg>
                </div>
                <div className="flex-1">
                  <div className="flex items-center gap-3 mb-1">
                    <h2 className="text-xl font-semibold text-dark-text-primary">Google Drive Backup</h2>
                    {backupStatus?.provider_connected ? (
                      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium bg-green-500/15 text-green-400 border border-green-500/30">
                        <span className="w-1.5 h-1.5 rounded-full bg-green-400"></span>
                        Connected{backupStatus.connected_email ? ` · ${backupStatus.connected_email}` : ''}
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

                  {backupSuccess && (
                    <div className="mb-4 p-3 bg-green-500/10 border border-green-500/30 rounded text-sm text-green-400">{backupSuccess}</div>
                  )}
                  {backupError && (
                    <div className="mb-4 p-3 bg-red-500/10 border border-red-500/30 rounded text-sm text-red-400">{backupError}</div>
                  )}

                  {/* Connect / status row */}
                  <div className="flex flex-wrap items-center gap-3 mb-6">
                    {!backupStatus?.provider_connected ? (
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
                          onClick={handleTriggerBackup}
                          disabled={isTriggeringBackup || backupStatus?.running}
                          className="px-4 py-2 bg-green-600 hover:bg-green-500 disabled:opacity-50 text-white rounded text-sm font-medium transition-colors"
                        >
                          {isTriggeringBackup || backupStatus?.running ? 'Backing up…' : 'Run Backup Now'}
                        </button>
                        <button
                          onClick={handleDisconnectBackup}
                          disabled={isDisconnectingBackup}
                          className="px-4 py-2 bg-dark-bg-tertiary hover:bg-red-500/20 text-dark-text-secondary hover:text-red-400 rounded text-sm font-medium transition-colors"
                        >
                          {isDisconnectingBackup ? 'Disconnecting…' : 'Disconnect'}
                        </button>
                      </>
                    )}
                    {backupStatus?.next_run && (
                      <span className="text-xs text-dark-text-tertiary">
                        Next: {new Date(backupStatus.next_run).toLocaleString()}
                      </span>
                    )}
                  </div>

                  {/* Settings form — only when connected */}
                  {backupStatus?.provider_connected && (
                    <form onSubmit={handleSaveBackupSettings} className="space-y-4 mb-6">
                      <div className="flex items-center gap-3">
                        <label className="relative inline-flex items-center cursor-pointer">
                          <input
                            type="checkbox"
                            checked={backupEnabled}
                            onChange={e => setBackupEnabled(e.target.checked)}
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
                            value={backupCronExpr}
                            onChange={e => setBackupCronExpr(e.target.value)}
                            placeholder="0 3 * * *"
                            className="w-full px-3 py-2 bg-dark-bg-primary border border-dark-border-subtle rounded text-sm text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:border-dark-accent-primary font-mono"
                          />
                          <p className="mt-1 text-xs text-dark-text-tertiary">Default: daily at 3 AM UTC</p>
                        </div>
                        <div>
                          <label className="block text-sm font-medium text-dark-text-secondary mb-1">Google Drive Folder ID <span className="text-dark-text-tertiary">(optional)</span></label>
                          <input
                            type="text"
                            value={backupFolderId}
                            onChange={e => setBackupFolderId(e.target.value)}
                            placeholder="Leave blank for root"
                            className="w-full px-3 py-2 bg-dark-bg-primary border border-dark-border-subtle rounded text-sm text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:border-dark-accent-primary"
                          />
                        </div>
                      </div>

                      {backupSettings && (
                        <div className="text-xs text-dark-text-tertiary">
                          Retention: keep all for {backupSettings.retention.full_days}d, every-other-day for {backupSettings.retention.alternate_days}d, weekly for {backupSettings.retention.weekly_days}d
                        </div>
                      )}

                      <button
                        type="submit"
                        disabled={isSavingBackupSettings}
                        className="px-4 py-2 bg-dark-accent-primary hover:bg-dark-accent-primary/80 disabled:opacity-50 text-white rounded text-sm font-medium transition-colors"
                      >
                        {isSavingBackupSettings ? 'Saving…' : 'Save Settings'}
                      </button>
                    </form>
                  )}

                  {/* Backup history */}
                  {backupHistory.length > 0 && (
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
                            {backupHistory.map(record => (
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
                                    onClick={() => handleDeleteBackupRecord(record.id)}
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

                  {!isLoadingBackup && backupStatus?.provider_connected && backupHistory.length === 0 && (
                    <p className="text-sm text-dark-text-tertiary">No backups yet. Run your first backup manually above.</p>
                  )}
                </div>
              </div>
            </Card>
          )}

          {/* API Keys Section */}
          <Card className="shadow-md" id="api-keys">
            <div className="p-6 sm:p-8 flex items-start gap-4">
              <div className="flex-shrink-0 w-10 h-10 bg-blue-500/10 rounded-lg flex items-center justify-center">
                <svg className="w-6 h-6 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
                </svg>
              </div>
              <div className="flex-1">
                <h2 className="text-xl font-semibold text-dark-text-primary mb-1">API Keys</h2>
                <p className="text-sm text-dark-text-secondary mb-6">Create and manage API keys for programmatic access</p>

                {apiKeySuccess && (
                  <div className="mb-4 p-4 bg-success-500/10 border-l-4 border-success-400 rounded-r-lg">
                    <div className="flex items-center">
                      <svg className="w-5 h-5 text-success-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                        <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                      </svg>
                      <span className="text-success-300 font-medium">{apiKeySuccess}</span>
                    </div>
                  </div>
                )}

                {apiKeyError && <FormError message={apiKeyError} className="mb-4" />}

                {/* Newly Created Key Display */}
                {createdKey && (
                  <div className="mb-6 border-2 border-yellow-500/30 bg-yellow-500/10 rounded-xl p-6">
                    <div className="flex items-start gap-3 mb-4">
                      <svg className="w-6 h-6 text-yellow-400 flex-shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
                        <path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
                      </svg>
                      <div>
                        <h3 className="font-bold text-yellow-300 mb-1">Save Your API Key</h3>
                        <p className="text-sm text-yellow-400/90">
                          This is the only time you'll see the full key. Copy it now and store it securely.
                        </p>
                      </div>
                    </div>

                    <div className="bg-dark-bg-primary p-5 rounded-lg border-2 border-yellow-500/30 mb-4">
                      <p className="text-xs font-medium text-dark-text-secondary mb-2">Key Name: {createdKey.name}</p>
                      <div className="flex items-center gap-2">
                        <code className="flex-1 text-sm font-mono bg-dark-bg-secondary text-dark-text-primary px-3 py-2 rounded border border-dark-border-subtle break-all">
                          {createdKey.key}
                        </code>
                        <Button size="sm" variant="secondary" onClick={() => copyAPIKey(createdKey.key)}>
                          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                          </svg>
                        </Button>
                      </div>
                    </div>

                    <Button onClick={() => setCreatedKey(null)} variant="secondary" className="w-full">
                      I've saved my key
                    </Button>
                  </div>
                )}

                {/* Create New Key Form */}
                <form onSubmit={handleCreateAPIKey} className="mb-6 space-y-4">
                  <TextInput
                    label="Key Name"
                    value={newKeyName}
                    onChange={(e) => setNewKeyName(e.target.value)}
                    placeholder="e.g., CI/CD Pipeline, Mobile App"
                    required
                  />

                  <div>
                    <label className="block text-sm font-medium text-dark-text-primary mb-2">
                      Expiration
                    </label>
                    <select
                      value={newKeyExpires === undefined ? '' : String(newKeyExpires)}
                      onChange={(e) => setNewKeyExpires(e.target.value ? Number.parseInt(e.target.value) : undefined)}
                      className="w-full px-3 py-2 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg text-sm text-dark-text-primary focus:outline-none focus:ring-2 focus:ring-primary-500/50 focus:border-primary-500/50"
                    >
                      <option value="30">30 days</option>
                      <option value="90">90 days</option>
                      <option value="180">180 days</option>
                      <option value="365">1 year</option>
                      <option value="">Never expires</option>
                    </select>
                  </div>

                  <Button type="submit" disabled={isCreatingKey}>
                    {isCreatingKey ? 'Creating...' : 'Create API Key'}
                  </Button>
                </form>

                {/* Existing Keys List */}
                <div>
                  <h3 className="text-sm font-semibold text-dark-text-primary mb-3">Active API Keys</h3>
                  {apiKeys.length === 0 ? (
                    <div className="text-center py-8 text-dark-text-tertiary">
                      <svg className="w-12 h-12 mx-auto mb-2 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
                      </svg>
                      <p className="text-sm">No API keys created yet</p>
                    </div>
                  ) : (
                    <div className="space-y-3">
                      {apiKeys.map((key) => (
                        <div key={key.id} className="p-4 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                          <div className="flex items-center justify-between">
                            <div className="flex-1">
                              <div className="flex items-center gap-3">
                                <h4 className="font-medium text-dark-text-primary">{key.name}</h4>
                                <code className="text-xs font-mono bg-dark-bg-primary text-dark-text-secondary px-2 py-1 rounded border border-dark-border-subtle">
                                  {key.key_prefix}...
                                </code>
                              </div>
                              <div className="mt-1 flex items-center gap-4 text-xs text-dark-text-tertiary">
                                <span>Created: {formatDate(key.created_at)}</span>
                                {key.last_used_at && <span>Last used: {formatDate(key.last_used_at)}</span>}
                                {key.expires_at && <span>Expires: {formatDate(key.expires_at)}</span>}
                              </div>
                            </div>
                            <Button
                              size="sm"
                              variant="danger"
                              onClick={() => handleDeleteAPIKey(key.id, key.name)}
                              disabled={isDeletingKey === key.id}
                            >
                              {isDeletingKey === key.id ? 'Deleting...' : 'Delete'}
                            </Button>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                <div className="mt-6 bg-blue-500/10 border border-blue-500/30 rounded-lg p-4">
                  <div className="flex gap-3">
                    <svg className="w-5 h-5 text-blue-400 flex-shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
                      <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd" />
                    </svg>
                    <div className="text-sm text-dark-text-secondary">
                      <p className="font-medium mb-1 text-dark-text-primary">Using API keys</p>
                      <p className="mb-2">Include your API key in the Authorization header:</p>
                      <code className="block bg-dark-bg-secondary text-dark-text-primary px-3 py-2 rounded border border-dark-border-subtle font-mono text-xs">
                        Authorization: ApiKey YOUR_API_KEY
                      </code>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </Card>

          {/* API & Integrations Section */}
          <Card className="shadow-md">
            <div className="p-6 sm:p-8 flex items-start gap-4">
              <div className="flex-shrink-0 w-10 h-10 bg-cyan-500/10 rounded-lg flex items-center justify-center">
                <svg className="w-6 h-6 text-cyan-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
                </svg>
              </div>
              <div className="flex-1">
                <h2 className="text-xl font-semibold text-dark-text-primary mb-1">API & Integrations</h2>
                <p className="text-sm text-dark-text-secondary mb-6">
                  Connect to TaskAI via REST API or MCP (Model Context Protocol) for AI assistants
                </p>

                {copiedSnippet && (
                  <div className="mb-4 p-3 bg-success-500/10 border-l-4 border-success-400 rounded-r-lg">
                    <div className="flex items-center">
                      <svg className="w-4 h-4 text-success-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                        <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                      </svg>
                      <span className="text-success-300 text-sm font-medium">Copied to clipboard</span>
                    </div>
                  </div>
                )}

                {/* OpenAPI Section */}
                <div className="mb-6">
                  <h3 className="text-sm font-semibold text-dark-text-primary mb-3">OpenAPI / REST</h3>
                  <div className="space-y-3">
                    <div className="flex items-center justify-between p-3 bg-dark-bg-primary border border-dark-border-subtle rounded-lg">
                      <div>
                        <p className="text-xs font-medium text-dark-text-tertiary">OpenAPI Spec</p>
                        <code className="text-sm text-dark-text-primary font-mono">{window.location.origin}/api/openapi</code>
                      </div>
                      <div className="flex items-center gap-1.5">
                        <button
                          onClick={() => { copyToClipboard(`${window.location.origin}/api/openapi`); setCopiedSnippet('spec'); setTimeout(() => setCopiedSnippet(null), 2000) }}
                          className="p-1.5 text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary rounded transition-colors"
                          title="Copy URL"
                        >
                          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                          </svg>
                        </button>
                        <a
                          href={`${window.location.origin}/api/openapi`}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="p-1.5 text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary rounded transition-colors"
                          title="Open in new tab"
                        >
                          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                          </svg>
                        </a>
                      </div>
                    </div>
                    <div className="flex items-center justify-between p-3 bg-dark-bg-primary border border-dark-border-subtle rounded-lg">
                      <div>
                        <p className="text-xs font-medium text-dark-text-tertiary">API Docs</p>
                        <code className="text-sm text-dark-text-primary font-mono">{window.location.origin}/api/docs</code>
                      </div>
                      <div className="flex items-center gap-1.5">
                        <button
                          onClick={() => { copyToClipboard(`${window.location.origin}/api/docs`); setCopiedSnippet('docs'); setTimeout(() => setCopiedSnippet(null), 2000) }}
                          className="p-1.5 text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary rounded transition-colors"
                          title="Copy URL"
                        >
                          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                          </svg>
                        </button>
                        <a
                          href={`${window.location.origin}/api/docs`}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="p-1.5 text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary rounded transition-colors"
                          title="Open in new tab"
                        >
                          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                          </svg>
                        </a>
                      </div>
                    </div>
                  </div>

                  {/* curl example */}
                  <div className="mt-3">
                    <div className="flex items-center justify-between mb-1">
                      <p className="text-xs font-medium text-dark-text-tertiary">Example: curl</p>
                      <button
                        onClick={() => {
                          copyToClipboard(`curl -H "Authorization: ApiKey YOUR_API_KEY" ${window.location.origin}/api/projects`)
                          setCopiedSnippet('curl')
                          setTimeout(() => setCopiedSnippet(null), 2000)
                        }}
                        className="text-xs text-dark-text-tertiary hover:text-dark-text-primary transition-colors"
                      >
                        {copiedSnippet === 'curl' ? 'Copied!' : 'Copy'}
                      </button>
                    </div>
                    <pre className="bg-dark-bg-primary border border-dark-border-subtle rounded-lg p-3 overflow-x-auto">
                      <code className="text-xs font-mono text-dark-text-secondary">
{`curl -H "Authorization: ApiKey YOUR_API_KEY" \\
  ${window.location.origin}/api/projects`}
                      </code>
                    </pre>
                  </div>

                  {/* Python example */}
                  <div className="mt-3">
                    <div className="flex items-center justify-between mb-1">
                      <p className="text-xs font-medium text-dark-text-tertiary">Example: Python</p>
                      <button
                        onClick={() => {
                          copyToClipboard(`import requests\n\nheaders = {"Authorization": "ApiKey YOUR_API_KEY"}\nresponse = requests.get("${window.location.origin}/api/projects", headers=headers)\nprint(response.json())`)
                          setCopiedSnippet('python')
                          setTimeout(() => setCopiedSnippet(null), 2000)
                        }}
                        className="text-xs text-dark-text-tertiary hover:text-dark-text-primary transition-colors"
                      >
                        {copiedSnippet === 'python' ? 'Copied!' : 'Copy'}
                      </button>
                    </div>
                    <pre className="bg-dark-bg-primary border border-dark-border-subtle rounded-lg p-3 overflow-x-auto">
                      <code className="text-xs font-mono text-dark-text-secondary">
{`import requests

headers = {"Authorization": "ApiKey YOUR_API_KEY"}
response = requests.get(
    "${window.location.origin}/api/projects",
    headers=headers
)
print(response.json())`}
                      </code>
                    </pre>
                  </div>
                </div>

                {/* MCP Section */}
                <div className="mb-4">
                  <h3 className="text-sm font-semibold text-dark-text-primary mb-3">MCP (Model Context Protocol)</h3>
                  <p className="text-sm text-dark-text-secondary mb-3">
                    Connect AI assistants like Claude to TaskAI using the MCP protocol.
                  </p>

                  <div className="flex items-center justify-between p-3 bg-dark-bg-primary border border-dark-border-subtle rounded-lg mb-3">
                    <div>
                      <p className="text-xs font-medium text-dark-text-tertiary">MCP Endpoint</p>
                      <code className="text-sm text-dark-text-primary font-mono">
                        {(() => {
                          const hostname = window.location.hostname
                          if (hostname === 'taskai.cc') return 'https://mcp.taskai.cc/mcp'
                          if (hostname === 'staging.taskai.cc') return 'https://mcp.staging.taskai.cc/mcp'
                          return `${window.location.origin}/mcp`
                        })()}
                      </code>
                    </div>
                    <button
                      onClick={() => {
                        const hostname = window.location.hostname
                        let mcpUrl = `${window.location.origin}/mcp`
                        if (hostname === 'taskai.cc') mcpUrl = 'https://mcp.taskai.cc/mcp'
                        if (hostname === 'staging.taskai.cc') mcpUrl = 'https://mcp.staging.taskai.cc/mcp'
                        copyToClipboard(mcpUrl)
                        setCopiedSnippet('mcp-url')
                        setTimeout(() => setCopiedSnippet(null), 2000)
                      }}
                      className="p-1.5 text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary rounded transition-colors"
                      title="Copy URL"
                    >
                      <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                      </svg>
                    </button>
                  </div>

                  {/* Available tools */}
                  <div className="mb-3">
                    <p className="text-xs font-medium text-dark-text-tertiary mb-2">Available Tools (8)</p>
                    <div className="flex flex-wrap gap-1.5">
                      {['get_me', 'list_projects', 'get_project', 'list_tasks', 'create_task', 'update_task', 'list_comments', 'add_comment'].map(tool => (
                        <span key={tool} className="px-2 py-0.5 text-xs font-mono bg-cyan-500/10 text-cyan-400 border border-cyan-500/20 rounded">
                          {tool}
                        </span>
                      ))}
                    </div>
                  </div>

                  {/* Claude Code config */}
                  <div className="mt-3">
                    <div className="flex items-center justify-between mb-1">
                      <p className="text-xs font-medium text-dark-text-tertiary">Claude Code MCP Config</p>
                      <button
                        onClick={() => {
                          const hostname = window.location.hostname
                          let mcpUrl = `${window.location.origin}/mcp`
                          if (hostname === 'taskai.cc') mcpUrl = 'https://mcp.taskai.cc/mcp'
                          if (hostname === 'staging.taskai.cc') mcpUrl = 'https://mcp.staging.taskai.cc/mcp'
                          copyToClipboard(JSON.stringify({
                            mcpServers: {
                              taskai: {
                                type: "streamable-http",
                                url: mcpUrl,
                                headers: { "X-API-Key": "YOUR_API_KEY" }
                              }
                            }
                          }, null, 2))
                          setCopiedSnippet('mcp-config')
                          setTimeout(() => setCopiedSnippet(null), 2000)
                        }}
                        className="text-xs text-dark-text-tertiary hover:text-dark-text-primary transition-colors"
                      >
                        {copiedSnippet === 'mcp-config' ? 'Copied!' : 'Copy'}
                      </button>
                    </div>
                    <pre className="bg-dark-bg-primary border border-dark-border-subtle rounded-lg p-3 overflow-x-auto">
                      <code className="text-xs font-mono text-dark-text-secondary">
{JSON.stringify({
  mcpServers: {
    taskai: {
      type: "streamable-http",
      url: (() => {
        const hostname = window.location.hostname
        if (hostname === 'taskai.cc') return 'https://mcp.taskai.cc/mcp'
        if (hostname === 'staging.taskai.cc') return 'https://mcp.staging.taskai.cc/mcp'
        return `${window.location.origin}/mcp`
      })(),
      headers: { "X-API-Key": "YOUR_API_KEY" }
    }
  }
}, null, 2)}
                      </code>
                    </pre>
                  </div>
                </div>

                <div className="bg-cyan-500/10 border border-cyan-500/30 rounded-lg p-4">
                  <div className="flex gap-3">
                    <svg className="w-5 h-5 text-cyan-400 flex-shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
                      <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd" />
                    </svg>
                    <div className="text-sm text-dark-text-secondary">
                      <p className="font-medium mb-1 text-dark-text-primary">Authentication</p>
                      <p className="mb-2">
                        All API requests require an API key. Create one in the{' '}
                        <a href="#api-keys" className="text-cyan-400 underline hover:text-cyan-300">API Keys section above</a>.
                      </p>
                      <p>
                        REST API: <code className="bg-dark-bg-primary px-1.5 py-0.5 rounded text-xs font-mono text-dark-text-primary">Authorization: ApiKey YOUR_API_KEY</code>
                        <br />
                        MCP: <code className="bg-dark-bg-primary px-1.5 py-0.5 rounded text-xs font-mono text-dark-text-primary">X-API-Key: YOUR_API_KEY</code>
                      </p>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </Card>

          {/* Invites Section */}
          <Card className="shadow-md">
            <div className="p-6 sm:p-8 flex items-start gap-4">
              <div className="flex-shrink-0 w-10 h-10 bg-indigo-500/10 rounded-lg flex items-center justify-center">
                <svg className="w-6 h-6 text-indigo-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                </svg>
              </div>
              <div className="flex-1">
                <h2 className="text-xl font-semibold text-dark-text-primary mb-1">Invite Friends</h2>
                <p className="text-sm text-dark-text-secondary mb-6">
                  TaskAI is invite-only. Share invite links with friends to let them join.
                </p>

                {inviteSuccess && (
                  <div className="mb-4 p-4 bg-success-500/10 border-l-4 border-success-400 rounded-r-lg">
                    <div className="flex items-center">
                      <svg className="w-5 h-5 text-success-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                        <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                      </svg>
                      <span className="text-success-300 font-medium">{inviteSuccess}</span>
                    </div>
                  </div>
                )}

                {inviteError && <FormError message={inviteError} className="mb-4" />}

                {/* Invite count status */}
                <div className="mb-6 p-4 bg-dark-bg-primary border border-dark-border-subtle rounded-lg space-y-3">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <div className={`w-3 h-3 rounded-full ${(isUserAdmin || myInviteCount > 0) ? 'bg-success-500' : 'bg-dark-text-tertiary'}`}></div>
                      <div>
                        <p className="font-medium text-dark-text-primary">Invites Available</p>
                        <p className="text-sm text-dark-text-secondary">
                          {isUserAdmin ? 'Unlimited (admin)' : `${myInviteCount} remaining`}
                        </p>
                      </div>
                    </div>
                    <Button
                      onClick={handleCreateInviteCode}
                      disabled={isCreatingInvite || (!isUserAdmin && myInviteCount <= 0)}
                      size="sm"
                    >
                      {isCreatingInvite ? 'Creating...' : 'Create Invite'}
                    </Button>
                  </div>
                  <div>
                    <input
                      type="email"
                      value={inviteRecipientEmail}
                      onChange={(e) => setInviteRecipientEmail(e.target.value)}
                      placeholder="Recipient email (optional — sends invite automatically)"
                      className="w-full px-3 py-2 text-sm bg-dark-bg-secondary border border-dark-border-subtle rounded-lg text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:border-primary-500"
                    />
                  </div>
                </div>

                {/* Newly created invite link */}
                {newInviteCode && (
                  <div className="mb-6 border-2 border-indigo-500/30 bg-indigo-500/10 rounded-xl p-6">
                    <div className="flex items-start gap-3 mb-4">
                      <svg className="w-6 h-6 text-indigo-400 flex-shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
                        <path d="M15 8a3 3 0 10-2.977-2.63l-4.94 2.47a3 3 0 100 4.319l4.94 2.47a3 3 0 10.895-1.789l-4.94-2.47a3.027 3.027 0 000-.74l4.94-2.47C13.456 7.68 14.19 8 15 8z" />
                      </svg>
                      <div>
                        <h3 className="font-bold text-indigo-300 mb-1">Share This Link</h3>
                        <p className="text-sm text-indigo-400/90">
                          Send this link to your friend. It expires in 7 days.
                        </p>
                      </div>
                    </div>

                    <div className="bg-dark-bg-primary p-4 rounded-lg border-2 border-indigo-500/30 mb-4">
                      <div className="flex items-center gap-2">
                        <code className="flex-1 text-sm font-mono bg-dark-bg-secondary text-dark-text-primary px-3 py-2 rounded border border-dark-border-subtle break-all">
                          {window.location.origin}/signup?code={newInviteCode}
                        </code>
                        <Button size="sm" variant="secondary" onClick={() => copyInviteLink(newInviteCode)}>
                          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                          </svg>
                        </Button>
                      </div>
                    </div>

                    <Button onClick={() => setNewInviteCode('')} variant="secondary" className="w-full">
                      Done
                    </Button>
                  </div>
                )}

                {/* Existing invites list */}
                <div>
                  <h3 className="text-sm font-semibold text-dark-text-primary mb-3">Your Invites</h3>
                  {myInvites.length === 0 ? (
                    <div className="text-center py-8 text-dark-text-tertiary">
                      <svg className="w-12 h-12 mx-auto mb-2 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                      </svg>
                      <p className="text-sm">No invites created yet</p>
                    </div>
                  ) : (
                    <div className="space-y-3">
                      {myInvites.map((inv: Invite) => (
                        <div key={inv.id} className="p-4 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                          <div className="flex items-center justify-between">
                            <div className="flex-1 min-w-0">
                              <div className="flex items-center gap-2">
                                <code className="text-xs font-mono bg-dark-bg-primary text-dark-text-secondary px-2 py-1 rounded border border-dark-border-subtle">
                                  {inv.code.substring(0, 12)}...
                                </code>
                                {inv.used_at ? (
                                  <span className="px-2 py-0.5 text-xs font-medium bg-success-500/10 text-success-400 rounded">
                                    Used
                                  </span>
                                ) : (
                                  <span className="px-2 py-0.5 text-xs font-medium bg-primary-500/10 text-primary-400 rounded">
                                    Available
                                  </span>
                                )}
                              </div>
                              <div className="mt-1 flex items-center gap-4 text-xs text-dark-text-tertiary">
                                <span>Created: {formatDate(inv.created_at)}</span>
                                {inv.used_at && inv.invitee_name && (
                                  <span>Used by: {inv.invitee_name}</span>
                                )}
                                {inv.expires_at && !inv.used_at && (
                                  <span>Expires: {formatDate(inv.expires_at)}</span>
                                )}
                              </div>
                            </div>
                            {!inv.used_at && (
                              <Button
                                size="sm"
                                variant="secondary"
                                onClick={() => copyInviteLink(inv.code)}
                              >
                                Copy Link
                              </Button>
                            )}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                <div className="mt-6 bg-indigo-500/10 border border-indigo-500/30 rounded-lg p-4">
                  <div className="flex gap-3">
                    <svg className="w-5 h-5 text-indigo-400 flex-shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
                      <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd" />
                    </svg>
                    <div className="text-sm text-dark-text-secondary">
                      <p className="font-medium mb-1 text-dark-text-primary">About invites</p>
                      <p>Each invite link can be used once and expires after 7 days. You start with 3 invites. Share them wisely!</p>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </Card>

          {/* Project Invitations Section */}
          <Card className="shadow-md" id="project-invitations">
            <div className="p-6 sm:p-8">
              <div className="flex items-start gap-4 mb-6">
                <div className="flex-shrink-0 w-10 h-10 bg-primary-500/10 rounded-lg flex items-center justify-center">
                  <svg className="w-5 h-5 text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
                  </svg>
                </div>
                <div className="flex-1">
                  <div className="flex items-center gap-3">
                    <h2 className="text-xl font-semibold text-dark-text-primary">Project Invitations</h2>
                    {projectInvitations.length > 0 && (
                      <span className="px-2 py-0.5 text-xs font-bold bg-danger-500 text-white rounded-full">
                        {projectInvitations.length}
                      </span>
                    )}
                  </div>
                  <p className="text-sm text-dark-text-secondary mt-1">
                    Accept or decline project access invitations from your teammates
                  </p>
                </div>
              </div>

              {projectInviteSuccess && (
                <div className="mb-4 p-3 bg-success-500/10 border border-success-500/30 rounded-lg text-sm text-success-300 flex items-center gap-2">
                  <svg className="w-4 h-4 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                    <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                  </svg>
                  {projectInviteSuccess}
                </div>
              )}
              {projectInviteError && (
                <div className="mb-4 p-3 bg-danger-500/10 border border-danger-500/30 rounded-lg text-sm text-danger-300">
                  {projectInviteError}
                </div>
              )}

              {projectInvitations.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-10 text-center">
                  <div className="w-12 h-12 bg-dark-bg-secondary rounded-full flex items-center justify-center mb-3">
                    <svg className="w-6 h-6 text-dark-text-quaternary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
                    </svg>
                  </div>
                  <p className="text-sm text-dark-text-tertiary">No pending project invitations</p>
                  <p className="text-xs text-dark-text-quaternary mt-1">When a project owner invites you, it will appear here</p>
                </div>
              ) : (
                <div className="space-y-3">
                  {projectInvitations.map((inv) => (
                    <div key={inv.id} className="flex items-center justify-between gap-4 p-4 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg hover:border-primary-500/30 transition-colors">
                      <div className="flex items-center gap-3 min-w-0">
                        <div className="w-10 h-10 bg-gradient-to-br from-indigo-500/20 to-purple-600/20 rounded-full flex items-center justify-center flex-shrink-0">
                          <svg className="w-5 h-5 text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
                          </svg>
                        </div>
                        <div className="min-w-0">
                          <p className="font-semibold text-dark-text-primary truncate">{inv.project_name}</p>
                          <p className="text-xs text-dark-text-tertiary mt-0.5">
                            Invited by <span className="text-dark-text-secondary">{inv.inviter_name}</span>
                            {' · '}
                            <span className="capitalize px-1.5 py-0.5 bg-dark-bg-tertiary rounded text-dark-text-secondary text-[11px]">{inv.role}</span>
                          </p>
                        </div>
                      </div>
                      <div className="flex items-center gap-2 flex-shrink-0">
                        <Button
                          size="sm"
                          onClick={() => handleAcceptProjectInvite(inv.id)}
                          disabled={isRespondingToProjectInvite === inv.id}
                        >
                          {isRespondingToProjectInvite === inv.id ? 'Joining...' : 'Join Project'}
                        </Button>
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() => handleRejectProjectInvite(inv.id)}
                          disabled={isRespondingToProjectInvite === inv.id}
                        >
                          Decline
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </Card>

          {/* Team Management Section */}
          <Card className="shadow-md">
            <div className="p-6 sm:p-8 flex items-start gap-4">
              <div className="flex-shrink-0 w-10 h-10 bg-green-500/10 rounded-lg flex items-center justify-center">
                <svg className="w-6 h-6 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
                </svg>
              </div>
              <div className="flex-1">
                <h2 className="text-xl font-semibold text-dark-text-primary mb-1">Team Management</h2>
                <p className="text-sm text-dark-text-secondary mb-6">Invite team members and manage access to your projects</p>

                {teamSuccess && (
                  <div className="mb-4 p-4 bg-success-500/10 border-l-4 border-success-400 rounded-r-lg">
                    <div className="flex items-center">
                      <svg className="w-5 h-5 text-success-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                        <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                      </svg>
                      <span className="text-success-300 font-medium">{teamSuccess}</span>
                    </div>
                  </div>
                )}

                {teamError && <FormError message={teamError} className="mb-4" />}

                {/* Team Info */}
                {team && (
                  <div className="mb-6 p-4 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                    <div className="flex items-center justify-between">
                      <div className="flex-1">
                        <p className="text-sm text-dark-text-secondary">Your Team</p>
                        {isEditingTeamName ? (
                          <div className="flex items-center gap-2 mt-1">
                            <input
                              type="text"
                              value={editTeamName}
                              onChange={(e) => setEditTeamName(e.target.value)}
                              maxLength={100}
                              className="px-3 py-1.5 bg-dark-bg-primary border border-dark-border-subtle rounded-lg text-dark-text-primary text-lg font-semibold focus:outline-none focus:ring-2 focus:ring-primary-500"
                              autoFocus
                              onKeyDown={(e) => {
                                if (e.key === 'Enter') handleSaveTeamName()
                                if (e.key === 'Escape') setIsEditingTeamName(false)
                              }}
                            />
                            <Button size="sm" onClick={handleSaveTeamName} disabled={isSavingTeamName}>
                              {isSavingTeamName ? 'Saving...' : 'Save'}
                            </Button>
                            <Button size="sm" variant="secondary" onClick={() => setIsEditingTeamName(false)} disabled={isSavingTeamName}>
                              Cancel
                            </Button>
                          </div>
                        ) : (
                          <div className="flex items-center gap-2">
                            <p className="text-lg font-semibold text-dark-text-primary">{team.name}</p>
                            <button
                              onClick={() => { setEditTeamName(team.name); setIsEditingTeamName(true) }}
                              className="p-1 text-dark-text-tertiary hover:text-dark-text-primary transition-colors rounded"
                              title="Edit team name"
                            >
                              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
                              </svg>
                            </button>
                          </div>
                        )}
                      </div>
                      <div className="text-right">
                        <p className="text-sm text-dark-text-secondary">Members</p>
                        <p className="text-lg font-semibold text-dark-text-primary">{teamMembers.length}</p>
                      </div>
                    </div>
                  </div>
                )}

                {/* Pending Invitations */}
                {invitations.length > 0 && (
                  <div className="mb-6">
                    <h3 className="text-sm font-semibold text-dark-text-primary mb-3">Pending Invitations</h3>
                    <div className="space-y-2">
                      {invitations.map((invitation: TeamInvitation) => (
                        <div key={invitation.id} className="p-4 bg-primary-500/10 border border-primary-500/30 rounded-lg">
                          <div className="flex items-center justify-between">
                            <div>
                              <p className="font-medium text-dark-text-primary">
                                Invitation to join {invitation.team_name}
                              </p>
                              {invitation.inviter_name && (
                                <p className="text-sm text-dark-text-secondary">
                                  From: {invitation.inviter_name}
                                </p>
                              )}
                            </div>
                            <div className="flex gap-2">
                              <Button
                                size="sm"
                                onClick={() => handleAcceptInvitation(invitation.id)}
                                disabled={isRespondingToInvitation === invitation.id}
                              >
                                {isRespondingToInvitation === invitation.id ? 'Accepting...' : 'Accept'}
                              </Button>
                              <Button
                                size="sm"
                                variant="secondary"
                                onClick={() => handleRejectInvitation(invitation.id)}
                                disabled={isRespondingToInvitation === invitation.id}
                              >
                                Decline
                              </Button>
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Invite / Add Member Form */}
                <form onSubmit={handleInviteOrAdd} className="mb-6">
                  <label className="block text-sm font-medium text-dark-text-secondary mb-1.5">Add or Invite Team Member</label>
                  <div className="relative">
                    <div className="flex gap-3 items-start">
                      <div className="flex-1 relative">
                        <input
                          type="text"
                          value={inviteEmail}
                          onChange={(e) => { setInviteEmail(e.target.value); setSelectedUser(null) }}
                          placeholder="Search by name or email..."
                          className="w-full px-3 py-2 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg text-dark-text-primary placeholder-dark-text-tertiary focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                        />
                        {isSearching && (
                          <div className="absolute right-3 top-2.5">
                            <svg className="w-4 h-4 animate-spin text-dark-text-tertiary" fill="none" viewBox="0 0 24 24">
                              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                            </svg>
                          </div>
                        )}
                        {showSearchDropdown && searchResults.length > 0 && (
                          <div className="absolute z-10 mt-1 w-full bg-dark-bg-secondary border border-dark-border-subtle rounded-lg shadow-lg max-h-48 overflow-y-auto">
                            {searchResults.map((u) => (
                              <button
                                key={u.id}
                                type="button"
                                onClick={() => handleSelectUser(u)}
                                className="w-full px-3 py-2 text-left hover:bg-dark-bg-tertiary transition-colors first:rounded-t-lg last:rounded-b-lg"
                              >
                                <p className="text-sm font-medium text-dark-text-primary">{u.name || u.email}</p>
                                {u.name && <p className="text-xs text-dark-text-tertiary">{u.email}</p>}
                              </button>
                            ))}
                          </div>
                        )}
                      </div>
                      <Button type="submit" disabled={isInviting || !inviteEmail.trim()}>
                        {isInviting ? 'Adding...' : selectedUser ? 'Add to Team' : 'Send Invite'}
                      </Button>
                    </div>
                    {selectedUser && (
                      <p className="mt-1 text-xs text-primary-400">Will add {selectedUser.name || selectedUser.email} directly to the team</p>
                    )}
                    {!selectedUser && inviteEmail.trim() && !isSearching && searchResults.length === 0 && inviteEmail.trim().length >= 2 && (
                      <p className="mt-1 text-xs text-dark-text-tertiary">No matching users found — will send an email invitation</p>
                    )}
                  </div>
                </form>

                {/* Team Members List */}
                <div>
                  <h3 className="text-sm font-semibold text-dark-text-primary mb-3">Team Members</h3>
                  {teamMembers.length === 0 ? (
                    <div className="text-center py-8 text-dark-text-tertiary">
                      <svg className="w-12 h-12 mx-auto mb-2 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
                      </svg>
                      <p className="text-sm">No team members yet</p>
                    </div>
                  ) : (
                    <div className="space-y-3">
                      {teamMembers.map((member: TeamMember) => (
                        <div key={member.id} className="p-4 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                          <div className="flex items-center justify-between">
                            <div className="flex-1">
                              <div className="flex items-center gap-3">
                                <div className="flex-1">
                                  <div className="flex items-center gap-2">
                                    <h4 className="font-medium text-dark-text-primary">
                                      {member.user_name || member.email}
                                    </h4>
                                    {member.role === 'owner' && (
                                      <span className="px-2 py-0.5 text-xs font-medium bg-primary-500/10 text-primary-400 rounded">
                                        Owner
                                      </span>
                                    )}
                                    {member.role === 'admin' && (
                                      <span className="px-2 py-0.5 text-xs font-medium bg-purple-500/10 text-purple-400 rounded">
                                        Admin
                                      </span>
                                    )}
                                  </div>
                                  <p className="text-sm text-dark-text-tertiary">{member.email}</p>
                                </div>
                              </div>
                            </div>
                            {member.role !== 'owner' && (
                              <Button
                                size="sm"
                                variant="danger"
                                onClick={() => handleRemoveMember(member.id, member.user_name || member.email)}
                                disabled={isRemovingMember === member.id}
                              >
                                {isRemovingMember === member.id ? 'Removing...' : 'Remove'}
                              </Button>
                            )}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                {/* Sent Invitations (Pending) */}
                {sentInvitations.length > 0 && (
                  <div className="mt-6">
                    <h3 className="text-sm font-semibold text-dark-text-primary mb-3">Pending Sent Invitations</h3>
                    <div className="space-y-2">
                      {sentInvitations.map((inv) => (
                        <div key={inv.id} className="p-3 bg-amber-500/5 border border-amber-500/20 rounded-lg">
                          <div className="flex items-center justify-between">
                            <div className="flex items-center gap-2">
                              <span className="text-sm text-dark-text-primary">{inv.invitee_email}</span>
                              <span className="px-2 py-0.5 text-xs font-medium bg-amber-500/10 text-amber-400 rounded">
                                Invited
                              </span>
                            </div>
                            <span className="text-xs text-dark-text-tertiary">
                              {new Date(inv.created_at).toLocaleDateString()}
                            </span>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                <div className="mt-6 bg-green-500/10 border border-green-500/30 rounded-lg p-4">
                  <div className="flex gap-3">
                    <svg className="w-5 h-5 text-green-400 flex-shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
                      <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd" />
                    </svg>
                    <div className="text-sm text-dark-text-secondary">
                      <p className="font-medium mb-1 text-dark-text-primary">About team access</p>
                      <p>Team members can view and edit all projects, tasks, sprints, and tags shared with the team. Only the team owner can invite or remove members.</p>
                    </div>
                  </div>
                </div>

                {/* Other Teams (member of, but not owner) */}
                {otherTeams.length > 0 && (
                  <div className="mt-8 pt-8 border-t border-dark-border-subtle">
                    <h3 className="text-sm font-semibold text-dark-text-primary mb-1">Other Teams</h3>
                    <p className="text-xs text-dark-text-tertiary mb-4">Teams you've joined as a member</p>
                    <div className="space-y-3">
                      {otherTeams.map((membership) => (
                        <div key={membership.team_id} className="p-4 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg">
                          <div className="flex items-center justify-between">
                            <div>
                              <p className="font-medium text-dark-text-primary">{membership.team_name}</p>
                              <p className="text-xs text-dark-text-tertiary mt-0.5">
                                Joined {new Date(membership.joined_at).toLocaleDateString()}
                              </p>
                            </div>
                            <span className="px-2 py-0.5 text-xs font-medium bg-dark-bg-tertiary text-dark-text-secondary rounded capitalize">
                              {membership.role}
                            </span>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            </div>
          </Card>
        </div>
      </div>
    </div>
  )
}
