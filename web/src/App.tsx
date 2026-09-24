import { lazy, Suspense, useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { AuthProvider, useAuth } from './state/AuthContext'
import { SyncProvider } from './state/SyncContext'
import { ThemeProvider } from './state/ThemeContext'
import { DialogProvider } from './state/DialogContext'
import ProtectedRoute from './components/ProtectedRoute'
import Landing from './routes/Landing'
import Login from './routes/Login'
import Signup from './routes/Signup'
import ForgotPassword from './routes/ForgotPassword'
import ResetPassword from './routes/ResetPassword'
import Dashboard from './routes/Dashboard'
import OAuthCallback from './routes/OAuthCallback'

// Lazy-loaded route components (code-split per route)
const Projects = lazy(() => import('./routes/Projects'))
const ProjectDetail = lazy(() => import('./routes/ProjectDetail'))
const TaskDetail = lazy(() => import('./routes/TaskDetail'))
const Sprints = lazy(() => import('./routes/Sprints'))
const Tags = lazy(() => import('./routes/Tags'))
const Admin = lazy(() => import('./routes/Admin'))
const Settings = lazy(() => import('./routes/Settings'))
const Assets = lazy(() => import('./routes/Assets'))
const AcceptTeamInvite = lazy(() => import('./routes/AcceptTeamInvite'))
const KnowledgeGraphPage = lazy(() => import('./routes/KnowledgeGraphPage'))
const UserProfile = lazy(() => import('./routes/UserProfile'))
const PublicWikiPage = lazy(() => import('./routes/PublicWikiPage'))

function HomeRoute() {
  const { user } = useAuth()
  if (user) return <Navigate to="/app" replace />
  return <Landing />
}

function RouteSpinner() {
  return (
    <div className="flex items-center justify-center py-20">
      <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-400" />
    </div>
  )
}

function WikiRedirect() {
  const { projectId } = useParams()
  const [searchParams] = useSearchParams()
  const page = searchParams.get('page')
  const annotation = searchParams.get('annotation')
  let target = `/app/projects/${projectId}?tab=wiki`
  if (page) target += `&page=${page}`
  if (annotation) target += `&annotation=${annotation}`
  return <Navigate to={target} replace />
}

function SettingsRedirect() {
  const { projectId } = useParams()
  const [searchParams] = useSearchParams()
  const github = searchParams.get('github')
  const target = github
    ? `/app/projects/${projectId}?tab=settings&github=${github}`
    : `/app/projects/${projectId}?tab=settings`
  return <Navigate to={target} replace />
}

// Renders for unknown paths with a robots noindex so crawlers treat them as
// intentionally unindexed rather than as soft 404s (GSC 2026-08-16 report).
function NotFoundRoute() {
  useEffect(() => {
    const meta = document.createElement('meta')
    meta.name = 'robots'
    meta.content = 'noindex'
    document.head.appendChild(meta)
    return () => {
      document.head.removeChild(meta)
    }
  }, [])
  return (
    <div className="min-h-screen flex flex-col items-center justify-center gap-3 text-center p-8">
      <h1 className="text-3xl font-semibold">Page not found</h1>
      <p className="text-neutral-500">
        That page does not exist.{' '}
        <a href="/" className="underline">
          Back to TaskAI
        </a>
      </p>
    </div>
  )
}

function AppRoutes() {
  const location = useLocation()
  const bgLocation = (location.state as { backgroundLocation?: Location })?.backgroundLocation

  return (
    <Suspense fallback={<RouteSpinner />}>
      <Routes location={bgLocation || location}>
        {/* Public routes */}
        <Route path="/" element={<HomeRoute />} />
        <Route path="/login" element={<Login />} />
        <Route path="/signup" element={<Signup />} />
        <Route path="/forgot-password" element={<ForgotPassword />} />
        <Route path="/reset-password" element={<ResetPassword />} />
        <Route path="/oauth/callback" element={<OAuthCallback />} />
        <Route path="/accept-invite" element={<AcceptTeamInvite />} />
        <Route path="/share/wiki/:token" element={<PublicWikiPage />} />

        {/* Protected routes */}
        <Route
          path="/app"
          element={
            <ProtectedRoute>
              <Dashboard />
            </ProtectedRoute>
          }
        >
          <Route index element={<Projects />} />
          <Route path="projects/:projectId" element={<ProjectDetail />} />
          <Route path="projects/:projectId/wiki" element={<WikiRedirect />} />
          <Route path="projects/:projectId/settings" element={<SettingsRedirect />} />
          <Route path="projects/:projectId/tasks/:taskNumber" element={<TaskDetail />} />
          <Route path="projects/:projectId/sprints" element={<Sprints />} />
          <Route path="projects/:projectId/tags" element={<Tags />} />
          <Route path="projects/:projectId/assets" element={<Assets />} />
          <Route path="projects/:projectId/graph" element={<KnowledgeGraphPage />} />
          <Route path="users/:userId" element={<UserProfile />} />
          <Route path="admin" element={<Admin />} />
          <Route path="settings" element={<Settings />} />
        </Route>

        {/* Catch-all: a real 404, not a redirect. Redirecting unknown URLs
            to the landing page made every phantom URL render homepage
            content, which Google flags as a soft 404; the injected robots
            noindex marks these as intentionally unindexed instead. */}
        <Route path="*" element={<NotFoundRoute />} />
      </Routes>

      {/* Task detail modal overlay when opened from project board */}
      {bgLocation && (
        <Routes>
          <Route
            path="/app/projects/:projectId/tasks/:taskNumber"
            element={<TaskDetailModal />}
          />
        </Routes>
      )}
    </Suspense>
  )
}

function TaskDetailModal() {
  const navigate = useNavigate()

  const handleClose = () => {
    navigate(-1)
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/60 backdrop-blur-sm overflow-y-auto"
      onClick={handleClose}
    >
      <div
        className="w-full max-w-5xl my-0 md:my-8 mx-0 md:mx-4 bg-dark-bg-primary md:rounded-xl overflow-hidden shadow-2xl border-0 md:border border-dark-border-subtle min-h-screen md:min-h-0"
        onClick={(e) => e.stopPropagation()}
      >
        <TaskDetail isModal onClose={handleClose} />
      </div>
    </div>
  )
}

function App() {
  return (
    <ThemeProvider>
      <DialogProvider>
        <AuthProvider>
          <SyncProvider>
            <BrowserRouter>
              <AppRoutes />
            </BrowserRouter>
          </SyncProvider>
        </AuthProvider>
      </DialogProvider>
    </ThemeProvider>
  )
}

export default App
