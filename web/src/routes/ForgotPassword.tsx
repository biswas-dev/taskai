import { useState, FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../lib/api'
import Card, { CardHeader, CardBody } from '../components/ui/Card'
import TextInput from '../components/ui/TextInput'
import Button from '../components/ui/Button'

export default function ForgotPassword() {
  const [email, setEmail] = useState('')
  const [loading, setLoading] = useState(false)
  const [sent, setSent] = useState(false)
  const [error, setError] = useState('')

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    if (!email.trim()) {
      setError('Email is required')
      return
    }
    setLoading(true)
    try {
      await api.forgotPassword(email.trim())
      setSent(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong')
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
              Forgot your password?
            </h2>
            <p className="mt-2 text-xs text-dark-text-tertiary">
              {sent ? 'Check your inbox' : "Enter your email and we'll send you a reset link"}
            </p>
          </div>
        </CardHeader>

        <CardBody>
          {sent ? (
            <div className="space-y-4">
              <div className="p-4 bg-green-500/10 border border-green-500/30 rounded-lg text-center">
                <svg className="w-8 h-8 text-green-400 mx-auto mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                </svg>
                <p className="text-sm text-green-300 font-medium">Reset link sent</p>
                <p className="text-xs text-dark-text-tertiary mt-1">
                  If <strong className="text-dark-text-secondary">{email}</strong> is registered, you'll receive a reset link shortly.
                </p>
              </div>
              <p className="text-xs text-center text-dark-text-tertiary">
                Didn't receive it?{' '}
                <button
                  onClick={() => setSent(false)}
                  className="text-primary-400 hover:text-primary-300 transition-colors"
                >
                  Try again
                </button>
              </p>
            </div>
          ) : (
            <form className="space-y-6" onSubmit={handleSubmit}>
              {error && (
                <div className="p-3 bg-danger-500/10 border border-danger-500/30 rounded-lg text-sm text-danger-300">
                  {error}
                </div>
              )}
              <TextInput
                id="email"
                name="email"
                type="email"
                label="Email address"
                autoComplete="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="you@example.com"
                disabled={loading}
              />
              <Button type="submit" variant="primary" fullWidth loading={loading}>
                Send reset link
              </Button>
              <div className="text-sm text-center">
                <span className="text-dark-text-quaternary">Remember your password? </span>
                <Link to="/login" className="font-medium text-primary-400 hover:text-primary-300 transition-colors">
                  Sign in
                </Link>
              </div>
            </form>
          )}
        </CardBody>
      </Card>
    </div>
  )
}
