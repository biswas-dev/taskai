import { useState, useEffect, useCallback } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useAuth } from '../state/AuthContext'
import { apiClient, TokenInvitationInfo } from '../lib/api'
import Card, { CardHeader, CardBody } from '../components/ui/Card'
import Button from '../components/ui/Button'

type PageState =
  | { kind: 'loading' }
  | { kind: 'info'; invitation: TokenInvitationInfo }
  | { kind: 'accepting' }
  | { kind: 'accepted'; teamName: string }
  | { kind: 'error'; message: string }

export default function AcceptTeamInvite() {
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') || ''
  const { user } = useAuth()
  const navigate = useNavigate()
  const [state, setState] = useState<PageState>({ kind: 'loading' })

  // Fetch invitation info
  useEffect(() => {
    if (!token) {
      setState({ kind: 'error', message: 'No invitation token provided.' })
      return
    }

    apiClient.getInvitationByToken(token)
      .then((info) => setState({ kind: 'info', invitation: info }))
      .catch((err) => {
        const message = err instanceof Error ? err.message : 'Invalid or expired invitation link.'
        setState({ kind: 'error', message })
      })
  }, [token])

  // Auto-accept when user is logged in and invitation info is loaded
  const acceptInvitation = useCallback(async () => {
    if (!token) return
    setState({ kind: 'accepting' })
    try {
      await apiClient.acceptInvitationByToken(token)
      const teamName = state.kind === 'info' ? state.invitation.team_name : 'the team'
      setState({ kind: 'accepted', teamName })
      setTimeout(() => navigate('/app', { replace: true }), 2000)
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to accept invitation.'
      setState({ kind: 'error', message })
    }
  }, [token, navigate, state])

  useEffect(() => {
    if (state.kind === 'info' && user) {
      acceptInvitation()
    }
  }, [state.kind, user, acceptInvitation])

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-dark-bg-base to-dark-bg-primary px-4 relative">
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
            <img src="/logo.svg" alt="TaskAI" className="mx-auto h-16 w-16 mb-4" />
            <h2 className="text-xl font-semibold text-dark-text-primary tracking-tight">
              Team Invitation
            </h2>
          </div>
        </CardHeader>

        <CardBody>
          {state.kind === 'loading' && (
            <div className="text-center py-8">
              <div className="animate-spin rounded-full h-8 w-8 border-2 border-primary-400 border-t-transparent mx-auto mb-4" />
              <p className="text-dark-text-tertiary text-sm">Loading invitation...</p>
            </div>
          )}

          {state.kind === 'accepting' && (
            <div className="text-center py-8">
              <div className="animate-spin rounded-full h-8 w-8 border-2 border-primary-400 border-t-transparent mx-auto mb-4" />
              <p className="text-dark-text-tertiary text-sm">Joining team...</p>
            </div>
          )}

          {state.kind === 'accepted' && (
            <div className="text-center py-8">
              <div className="w-12 h-12 bg-success-500/10 rounded-full flex items-center justify-center mx-auto mb-4">
                <svg className="w-6 h-6 text-success-400" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                </svg>
              </div>
              <h3 className="text-lg font-semibold text-dark-text-primary mb-2">You're in!</h3>
              <p className="text-dark-text-tertiary text-sm">
                You've joined <strong className="text-dark-text-secondary">{state.teamName}</strong>. Redirecting...
              </p>
            </div>
          )}

          {state.kind === 'error' && (
            <div className="text-center py-8">
              <div className="w-12 h-12 bg-danger-500/10 rounded-full flex items-center justify-center mx-auto mb-4">
                <svg className="w-6 h-6 text-danger-400" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                </svg>
              </div>
              <h3 className="text-lg font-semibold text-dark-text-primary mb-2">Unable to accept</h3>
              <p className="text-dark-text-tertiary text-sm mb-6">{state.message}</p>
              <Link to="/app">
                <Button variant="secondary">Go to Dashboard</Button>
              </Link>
            </div>
          )}

          {state.kind === 'info' && !user && (
            <div className="py-4">
              <div className="p-4 bg-primary-500/5 border border-primary-500/20 rounded-lg mb-6">
                <p className="text-dark-text-secondary text-sm">
                  <strong>{state.invitation.inviter_name}</strong> invited you to join{' '}
                  <strong>{state.invitation.team_name}</strong>
                </p>
              </div>

              <div className="space-y-3">
                {state.invitation.requires_signup ? (
                  <Link
                    to={`/signup?code=${encodeURIComponent(state.invitation.invite_code || '')}&email=${encodeURIComponent(state.invitation.invitee_email || '')}&redirect=${encodeURIComponent(`/accept-invite?token=${token}`)}`}
                    className="block"
                  >
                    <Button variant="primary" fullWidth>
                      Create Account &amp; Join
                    </Button>
                  </Link>
                ) : (
                  <Link to={`/login?redirect=${encodeURIComponent(`/accept-invite?token=${token}`)}`} className="block">
                    <Button variant="primary" fullWidth>
                      Sign in to Accept
                    </Button>
                  </Link>
                )}
              </div>
            </div>
          )}
        </CardBody>
      </Card>
    </div>
  )
}
