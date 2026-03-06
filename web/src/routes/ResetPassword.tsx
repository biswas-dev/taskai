import { useState, FormEvent } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api } from '../lib/api'
import Card, { CardHeader, CardBody } from '../components/ui/Card'
import TextInput from '../components/ui/TextInput'
import Button from '../components/ui/Button'

export default function ResetPassword() {
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') ?? ''
  const navigate = useNavigate()

  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [loading, setLoading] = useState(false)
  const [done, setDone] = useState(false)
  const [error, setError] = useState('')

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')

    if (!token) {
      setError('Invalid reset link — please request a new one.')
      return
    }
    if (password.length < 8) {
      setError('Password must be at least 8 characters')
      return
    }
    if (password !== confirm) {
      setError('Passwords do not match')
      return
    }

    setLoading(true)
    try {
      await api.resetPassword(token, password)
      setDone(true)
      setTimeout(() => navigate('/login'), 3000)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to reset password')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-dark-bg-base to-dark-bg-primary px-4 relative">
      <Link
        to="/login"
        className="absolute top-6 left-6 text-sm text-dark-text-tertiary hover:text-dark-text-primary flex items-center gap-2 transition-colors duration-150"
      >
        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" />
        </svg>
        Back to sign in
      </Link>

      <Card className="max-w-md w-full">
        <CardHeader>
          <div className="text-center">
            <img src="/logo.svg" alt="TaskAI" className="mx-auto h-16 w-16 mb-4" />
            <h2 className="text-xl font-semibold text-dark-text-primary tracking-tight">
              Set new password
            </h2>
            <p className="mt-2 text-xs text-dark-text-tertiary">
              {done ? 'Redirecting to sign in…' : 'Choose a strong password'}
            </p>
          </div>
        </CardHeader>

        <CardBody>
          {done ? (
            <div className="p-4 bg-green-500/10 border border-green-500/30 rounded-lg text-center">
              <svg className="w-8 h-8 text-green-400 mx-auto mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <p className="text-sm text-green-300 font-medium">Password reset successfully</p>
              <p className="text-xs text-dark-text-tertiary mt-1">Redirecting to sign in…</p>
            </div>
          ) : !token ? (
            <div className="text-center space-y-4">
              <p className="text-sm text-danger-300">Invalid or missing reset token.</p>
              <Link to="/forgot-password" className="text-sm text-primary-400 hover:text-primary-300">
                Request a new reset link
              </Link>
            </div>
          ) : (
            <form className="space-y-5" onSubmit={handleSubmit}>
              {error && (
                <div className="p-3 bg-danger-500/10 border border-danger-500/30 rounded-lg text-sm text-danger-300">
                  {error}
                </div>
              )}
              <TextInput
                id="password"
                name="password"
                type="password"
                label="New password"
                autoComplete="new-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="At least 8 characters"
                disabled={loading}
              />
              <TextInput
                id="confirm"
                name="confirm"
                type="password"
                label="Confirm password"
                autoComplete="new-password"
                required
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                placeholder="Repeat your new password"
                disabled={loading}
                error={confirm && confirm !== password ? 'Passwords do not match' : undefined}
              />
              <Button type="submit" variant="primary" fullWidth loading={loading}>
                Reset password
              </Button>
            </form>
          )}
        </CardBody>
      </Card>
    </div>
  )
}
