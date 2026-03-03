import { lazy, Suspense } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useLocation, useNavigate } from 'react-router-dom'
import { AuthProvider, useAuth } from './state/AuthContext'
import { SyncProvider } from './state/SyncContext'
import ProtectedRoute from './components/ProtectedRoute'
import Landing from './routes/Landing'
import Login from './routes/Login'
import Signup from './routes/Signup'
import Dashboard from './routes/Dashboard'

// Lazy-loaded route components (code-split per route)
const Projects = lazy(() => import('./routes/Projects'))
const ProjectDetail = lazy(() => import('./routes/ProjectDetail'))
const ProjectSettings = lazy(() => import('./routes/ProjectSettings'))
const TaskDetail = lazy(() => import('./routes/TaskDetail'))
const Sprints = lazy(() => import('./routes/Sprints'))
const Tags = lazy(() => import('./routes/Tags'))
const Admin = lazy(() => import('./routes/Admin'))
const Settings = lazy(() => import('./routes/Settings'))
const Assets = lazy(() => import('./routes/Assets'))
const AcceptTeamInvite = lazy(() => import('./routes/AcceptTeamInvite'))
const Wiki = lazy(() => import('./routes/Wiki'))

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
        <Route path="/accept-invite" element={<AcceptTeamInvite />} />

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
          <Route path="projects/:projectId/wiki" element={<Wiki />} />
          <Route path="projects/:projectId/settings" element={<ProjectSettings />} />
          <Route path="projects/:projectId/tasks/:taskNumber" element={<TaskDetail />} />
          <Route path="projects/:projectId/sprints" element={<Sprints />} />
          <Route path="projects/:projectId/tags" element={<Tags />} />
          <Route path="projects/:projectId/assets" element={<Assets />} />
          <Route path="admin" element={<Admin />} />
          <Route path="settings" element={<Settings />} />
        </Route>

        {/* Catch-all redirect to landing */}
        <Route path="*" element={<Navigate to="/" replace />} />
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
    <AuthProvider>
      <SyncProvider>
        <BrowserRouter>
          <AppRoutes />
        </BrowserRouter>
      </SyncProvider>
    </AuthProvider>
  )
}

export default App
