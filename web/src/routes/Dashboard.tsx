import { useEffect, useState } from 'react'
import { Outlet, useNavigate } from 'react-router-dom'
import { useAuth } from '../state/AuthContext'
import { useTheme } from '../state/ThemeContext'
import Sidebar from '../components/Sidebar'
import ProjectModal from '../components/ProjectModal'
import SyncStatus from '../components/SyncStatus'
import CommandPalette, { searchShortcutLabel } from '../components/CommandPalette'
import NotificationBell from '../components/ProjectInvitationBanner'
import { NotificationProvider } from '../state/NotificationContext'
import { Project } from '../lib/api'

export default function Dashboard() {
  const { user, logout } = useAuth()
  const { theme, toggleTheme } = useTheme()
  const navigate = useNavigate()
  const [isProjectModalOpen, setIsProjectModalOpen] = useState(false)
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [sidebarPinned, setSidebarPinned] = useState(false)

  // Load sidebar pin preference from localStorage once user is known
  useEffect(() => {
    if (!user?.id) return
    const key = `sidebar-pinned-${user.id}`
    setSidebarPinned(localStorage.getItem(key) === '1')
  }, [user?.id])

  const handleTogglePin = () => {
    if (!user?.id) return
    const key = `sidebar-pinned-${user.id}`
    setSidebarPinned(pinned => {
      const next = !pinned
      localStorage.setItem(key, next ? '1' : '0')
      if (next) setSidebarOpen(false)
      return next
    })
  }

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  const handleProjectCreated = (project: Project) => {
    // Add project to sidebar via window callback
    const w = window as Window & { __addProject?: (project: Project) => void }
    if (w.__addProject) {
      w.__addProject(project)
    }
    // Navigate to the new project
    navigate(`/app/projects/${project.id}`)
  }

  return (
    <NotificationProvider>
      <div className="h-screen bg-dark-bg-base flex flex-col overflow-hidden">
        {/* Header */}
        <header className="bg-dark-bg-primary/80 backdrop-blur-lg border-b border-dark-border-subtle sticky top-0 z-10">
          <div className="flex items-center justify-between h-14 px-4 md:px-6">
            <div className="flex items-center gap-2.5">
              {/* Hamburger — visible when sidebar is not pinned */}
              {!sidebarPinned && (
                <button
                  onClick={() => setSidebarOpen(true)}
                  className="p-1.5 -ml-1.5 text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary rounded-md transition-colors"
                  aria-label="Open sidebar"
                >
                  <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
                  </svg>
                </button>
              )}
              <img
                src="/logo.svg"
                alt="TaskAI"
                className="w-5 h-5"
              />
              <h1 className="text-sm font-semibold text-dark-text-primary tracking-tight">TaskAI</h1>
            </div>

            <div className="flex items-center gap-2 md:gap-4">
              <SyncStatus />
              <NotificationBell />
              {/* Theme toggle */}
              <button
                onClick={toggleTheme}
                className="p-1.5 text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary rounded-md transition-colors"
                aria-label={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
                title={theme === 'dark' ? 'Light mode' : 'Dark mode'}
              >
                {theme === 'dark' ? (
                  /* Sun icon */
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364-6.364l-.707.707M6.343 17.657l-.707.707M17.657 17.657l-.707-.707M6.343 6.343l-.707-.707M12 8a4 4 0 100 8 4 4 0 000-8z" />
                  </svg>
                ) : (
                  /* Moon icon */
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
                  </svg>
                )}
              </button>
              <button
                onClick={() => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', metaKey: true }))}
                className="hidden md:inline-flex items-center gap-2 px-3 py-1.5 text-xs text-dark-text-quaternary hover:text-dark-text-tertiary bg-dark-bg-secondary hover:bg-dark-bg-tertiary border border-dark-border-subtle rounded-md transition-all duration-150 cursor-pointer"
                aria-label={`Search (${searchShortcutLabel})`}
              >
                <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                </svg>
                <span>Search...</span>
                <kbd className="px-1.5 py-0.5 text-[10px] font-semibold bg-dark-bg-tertiary rounded border border-dark-border-subtle">{searchShortcutLabel}</kbd>
              </button>
            </div>
          </div>
        </header>

        {/* Left-edge hover trigger — opens sidebar when mouse hugs the left side */}
        {!sidebarPinned && (
          <div
            className="fixed left-0 top-0 bottom-0 w-2 z-30"
            onMouseEnter={() => setSidebarOpen(true)}
          />
        )}

        {/* Main Layout */}
        <div className="flex flex-1 overflow-hidden">
          {/* Sidebar */}
          <Sidebar
            onCreateProject={() => setIsProjectModalOpen(true)}
            onLogout={handleLogout}
            isOpen={sidebarOpen}
            onClose={() => setSidebarOpen(false)}
            isPinned={sidebarPinned}
            onTogglePin={handleTogglePin}
          />

          {/* Main Content */}
          <main className="flex-1 overflow-y-auto bg-dark-bg-base">
            <Outlet />
          </main>
        </div>

        {/* Project Modal */}
        <ProjectModal
          isOpen={isProjectModalOpen}
          onClose={() => setIsProjectModalOpen(false)}
          onProjectCreated={handleProjectCreated}
        />

        {/* Command Palette (Cmd+K) */}
        <CommandPalette />
      </div>
    </NotificationProvider>
  )
}
