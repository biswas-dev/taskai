import { useState, FormEvent, useEffect } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useAuth } from '../state/AuthContext'
import { validateLoginForm } from '../lib/validation'
import Card, { CardHeader, CardBody } from '../components/ui/Card'
import TextInput from '../components/ui/TextInput'
import Button from '../components/ui/Button'
import FormError from '../components/ui/FormError'

export default function Login() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [touched, setTouched] = useState<Record<string, boolean>>({})
  const { login, error, loading, clearError, user } = useAuth()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const redirectTo = searchParams.get('redirect')
  const oauthError = searchParams.get('oauth_error')

  // Redirect if already logged in
  useEffect(() => {
    if (user) {
      navigate(redirectTo || '/app', { replace: true })
    }
  }, [user, navigate, redirectTo])

  const handleBlur = (field: string) => {
    setTouched({ ...touched, [field]: true })

    // Validate on blur
    const validation = validateLoginForm(email, password)
    setFieldErrors(validation.errors)
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    clearError()

    // Mark all fields as touched
    setTouched({ email: true, password: true })

    // Validate form
    const validation = validateLoginForm(email, password)
    setFieldErrors(validation.errors)

    if (!validation.isValid) {
      return
    }

    try {
      await login({ email, password })
      // AuthContext will update user, useEffect will redirect
    } catch (err) {
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
              Sign in to TaskAI
            </h2>
            <p className="mt-2 text-xs text-dark-text-tertiary">
              Welcome back
            </p>
          </div>
        </CardHeader>

        <CardBody>
          {oauthError && (
            <div className="mb-4 p-3 bg-danger-500/10 border border-danger-500/30 rounded-lg">
              <div className="flex items-center gap-2">
                <svg className="w-4 h-4 text-danger-400 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                </svg>
                <span className="text-sm text-danger-300">{oauthError}</span>
              </div>
            </div>
          )}

          <form className="space-y-6" onSubmit={handleSubmit}>
            <FormError message={error || ''} />

            <div className="space-y-4">
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

              <div>
                <TextInput
                  id="password"
                  name="password"
                  type="password"
                  label="Password"
                  autoComplete="current-password"
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  onBlur={() => handleBlur('password')}
                  error={touched.password ? fieldErrors.password : undefined}
                  placeholder="••••••••"
                  disabled={loading}
                />
                <div className="mt-1.5 text-right">
                  <Link to="/forgot-password" className="text-xs text-dark-text-tertiary hover:text-primary-400 transition-colors">
                    Forgot password?
                  </Link>
                </div>
              </div>
            </div>

            <Button
              type="submit"
              variant="primary"
              fullWidth
              loading={loading}
            >
              Sign in
            </Button>

            <div className="text-sm text-center">
              <span className="text-dark-text-quaternary">Don't have an account? </span>
              <Link to="/signup" className="font-medium text-primary-400 hover:text-primary-300 transition-colors">
                Sign up
              </Link>
            </div>
          </form>

          <div className="mt-6">
            <div className="relative">
              <div className="absolute inset-0 flex items-center">
                <div className="w-full border-t border-dark-border-subtle" />
              </div>
              <div className="relative flex justify-center text-xs">
                <span className="px-2 bg-dark-bg-secondary text-dark-text-quaternary">or continue with</span>
              </div>
            </div>

            <div className="mt-4 flex flex-col gap-3">
              <a
                href="/api/auth/google"
                className="flex items-center justify-center gap-3 px-4 py-2.5 rounded-lg border border-dark-border-subtle bg-dark-bg-primary hover:bg-dark-bg-tertiary transition-colors text-sm text-dark-text-secondary"
              >
                <svg className="w-4 h-4" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" fill="#4285F4"/>
                  <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/>
                  <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" fill="#FBBC05"/>
                  <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/>
                </svg>
                Sign in with Google
              </a>

              <a
                href="/api/auth/github/login"
                className="flex items-center justify-center gap-3 px-4 py-2.5 rounded-lg border border-dark-border-subtle bg-dark-bg-primary hover:bg-dark-bg-tertiary transition-colors text-sm text-dark-text-secondary"
              >
                <svg className="w-4 h-4" viewBox="0 0 24 24" aria-hidden="true" fill="currentColor">
                  <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0 1 12 6.844a9.59 9.59 0 0 1 2.504.337c1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.02 10.02 0 0 0 22 12.017C22 6.484 17.522 2 12 2z" />
                </svg>
                Sign in with GitHub
              </a>
            </div>
          </div>
        </CardBody>
      </Card>
    </div>
  )
}
