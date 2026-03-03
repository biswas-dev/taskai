import { useState, FormEvent, useEffect } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useAuth } from '../state/AuthContext'
import { validateSignupForm } from '../lib/validation'
import { apiClient } from '../lib/api'
import Card, { CardHeader, CardBody } from '../components/ui/Card'
import TextInput from '../components/ui/TextInput'
import Button from '../components/ui/Button'
import FormError from '../components/ui/FormError'

export default function Signup() {
  const [searchParams] = useSearchParams()
  const inviteCodeFromURL = searchParams.get('code') || ''
  const emailFromURL = searchParams.get('email') || ''
  const redirectTo = searchParams.get('redirect')

  const [firstName, setFirstName] = useState('')
  const [lastName, setLastName] = useState('')
  const [email, setEmail] = useState(emailFromURL)
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [inviteCode, setInviteCode] = useState(inviteCodeFromURL)
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [touched, setTouched] = useState<Record<string, boolean>>({})
  const { signup, error, loading, clearError, user } = useAuth()
  const navigate = useNavigate()

  // Invite validation state
  const [inviteValid, setInviteValid] = useState<boolean | null>(null)
  const [inviterName, setInviterName] = useState('')
  const [inviteMessage, setInviteMessage] = useState('')
  const [validatingInvite, setValidatingInvite] = useState(false)

  // Redirect if already logged in
  useEffect(() => {
    if (user) {
      navigate(redirectTo || '/app', { replace: true })
    }
  }, [user, navigate, redirectTo])

  // Validate invite code from URL on mount
  useEffect(() => {
    if (inviteCodeFromURL) {
      validateInviteCode(inviteCodeFromURL)
    }
  }, [inviteCodeFromURL])

  const validateInviteCode = async (code: string) => {
    if (!code.trim()) {
      setInviteValid(null)
      setInviterName('')
      setInviteMessage('')
      return
    }

    setValidatingInvite(true)
    try {
      const result = await apiClient.validateInvite(code.trim())
      setInviteValid(result.valid)
      setInviterName(result.inviter_name || '')
      setInviteMessage(result.message || '')
    } catch {
      setInviteValid(false)
      setInviteMessage('Failed to validate invite code')
    } finally {
      setValidatingInvite(false)
    }
  }

  const handleBlur = (field: string) => {
    setTouched({ ...touched, [field]: true })

    // Validate on blur
    const validation = validateSignupForm(email, password, confirmPassword)
    setFieldErrors(validation.errors)
  }

  const handleInviteBlur = () => {
    if (inviteCode.trim()) {
      validateInviteCode(inviteCode)
    }
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    clearError()

    // Mark all fields as touched
    setTouched({ email: true, password: true, confirmPassword: true })

    // Validate form
    const validation = validateSignupForm(email, password, confirmPassword)
    setFieldErrors(validation.errors)

    if (!validation.isValid) {
      return
    }

    if (!firstName.trim()) {
      setFieldErrors(prev => ({ ...prev, firstName: 'First name is required' }))
      return
    }

    if (!inviteCode.trim()) {
      setFieldErrors(prev => ({ ...prev, inviteCode: 'Invite code is required' }))
      return
    }

    try {
      await signup({ email, password, invite_code: inviteCode.trim(), first_name: firstName.trim(), last_name: lastName.trim() })
      // AuthContext will update user, useEffect will redirect
    } catch {
      // Error is handled by AuthContext
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-dark-bg-base to-dark-bg-primary px-4 relative">
      {/* Back to home */}
      <Link
        to="/"
        className="absolute top-6 left-6 text-sm text-dark-text-tertiary hover:text-dark-text-primary flex items-center gap-2 transition-colors duration-150"
      >
        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" />
        </svg>
        Back
      </Link>

      <Card className="max-w-md w-full">
        <CardHeader>
          <div className="text-center">
            <img
              src="/logo.svg"
              alt="TaskAI"
              className="mx-auto h-16 w-16 mb-4"
            />
            <h2 className="text-xl font-semibold text-dark-text-primary tracking-tight">
              Create your account
            </h2>
            <p className="mt-2 text-xs text-dark-text-tertiary">
              TaskAI is invite-only. You need a referral to create an account.
            </p>
          </div>
        </CardHeader>

        <CardBody>
          {/* Invite status banner */}
          {inviteValid === true && (
            <div className="mb-4 p-3 bg-success-500/10 border border-success-500/30 rounded-lg">
              <div className="flex items-center gap-2">
                <svg className="w-4 h-4 text-success-400 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                </svg>
                <span className="text-sm text-success-300">
                  Valid invite{inviterName ? ` from ${inviterName}` : ''}
                </span>
              </div>
            </div>
          )}

          {inviteValid === false && inviteMessage && (
            <div className="mb-4 p-3 bg-danger-500/10 border border-danger-500/30 rounded-lg">
              <div className="flex items-center gap-2">
                <svg className="w-4 h-4 text-danger-400 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                </svg>
                <span className="text-sm text-danger-300">{inviteMessage}</span>
              </div>
            </div>
          )}

          <form className="space-y-6" onSubmit={handleSubmit}>
            <FormError message={error || ''} />

            <div className="space-y-4">
              {/* Invite code field */}
              <div>
                <TextInput
                  id="invite-code"
                  name="invite-code"
                  type="text"
                  label="Invite Code"
                  required
                  value={inviteCode}
                  onChange={(e) => {
                    setInviteCode(e.target.value)
                    setInviteValid(null)
                  }}
                  onBlur={handleInviteBlur}
                  error={fieldErrors.inviteCode}
                  placeholder="Paste your invite code"
                  disabled={loading}
                  helpText={validatingInvite ? 'Validating...' : undefined}
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <TextInput
                  id="first-name"
                  name="first-name"
                  type="text"
                  label="First Name"
                  autoComplete="given-name"
                  required
                  value={firstName}
                  onChange={(e) => setFirstName(e.target.value)}
                  error={fieldErrors.firstName}
                  placeholder="First"
                  disabled={loading}
                />
                <TextInput
                  id="last-name"
                  name="last-name"
                  type="text"
                  label="Last Name"
                  autoComplete="family-name"
                  value={lastName}
                  onChange={(e) => setLastName(e.target.value)}
                  placeholder="Last"
                  disabled={loading}
                />
              </div>

              <TextInput
                id="email"
                name="email"
                type="email"
                label="Email address"
                autoComplete="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                onBlur={() => handleBlur('email')}
                error={touched.email ? fieldErrors.email : undefined}
                placeholder="you@example.com"
                disabled={loading}
              />

              <TextInput
                id="password"
                name="password"
                type="password"
                label="Password"
                autoComplete="new-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                onBlur={() => handleBlur('password')}
                error={touched.password ? fieldErrors.password : undefined}
                helpText="Must be at least 8 characters with a letter and number"
                placeholder="••••••••"
                disabled={loading}
              />

              <TextInput
                id="confirm-password"
                name="confirm-password"
                type="password"
                label="Confirm Password"
                autoComplete="new-password"
                required
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                onBlur={() => handleBlur('confirmPassword')}
                error={touched.confirmPassword ? fieldErrors.confirmPassword : undefined}
                placeholder="••••••••"
                disabled={loading}
              />
            </div>

            <Button
              type="submit"
              variant="primary"
              fullWidth
              loading={loading}
            >
              Create account
            </Button>

            <div className="text-sm text-center">
              <span className="text-dark-text-quaternary">Already have an account? </span>
              <Link to="/login" className="font-medium text-primary-400 hover:text-primary-300 transition-colors">
                Sign in
              </Link>
            </div>
          </form>

          {/* Referral info */}
          {!inviteCodeFromURL && (
            <div className="mt-6 p-4 bg-dark-bg-primary border border-dark-border-subtle rounded-lg">
              <div className="flex gap-3">
                <svg className="w-5 h-5 text-primary-400 flex-shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd" />
                </svg>
                <div className="text-xs text-dark-text-tertiary">
                  <p className="font-medium mb-1 text-dark-text-secondary">How do I get an invite?</p>
                  <p>Ask an existing TaskAI user to send you an invite link from their account settings. Each user can invite a limited number of friends.</p>
                </div>
              </div>
            </div>
          )}
        </CardBody>
      </Card>
    </div>
  )
}
