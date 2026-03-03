import { useState } from 'react'
import { Outlet, useNavigate } from 'react-router-dom'
import { useAuth } from '../state/AuthContext'
import Sidebar from '../components/Sidebar'
import ProjectModal from '../components/ProjectModal'
import SyncStatus from '../components/SyncStatus'
import CommandPalette, { searchShortcutLabel } from '../components/CommandPalette'
import ProjectInvitationBanner from '../components/ProjectInvitationBanner'
import { Project } from '../lib/api'

export default function Dashboard() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const [isProjectModalOpen, setIsProjectModalOpen] = useState(false)
  const [sidebarOpen, setSidebarOpen] = useState(false)

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
    <div className="min-h-screen bg-dark-bg-base flex flex-col">
      {/* Header */}
      <header className="bg-dark-bg-primary/80 backdrop-blur-lg border-b border-dark-border-subtle sticky top-0 z-10">
        <div className="flex items-center justify-between h-14 px-4 md:px-6">
          <div className="flex items-center gap-2.5">
            {/* Hamburger - mobile only */}
            <button
              onClick={() => setSidebarOpen(true)}
              className="md:hidden p-1.5 -ml-1.5 text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary rounded-md transition-colors"
              aria-label="Open sidebar"
            >
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
              </svg>
            </button>
            <img
              src="/logo.svg"
              alt="TaskAI"
              className="w-5 h-5"
            />
            <h1 className="text-sm font-semibold text-dark-text-primary tracking-tight">TaskAI</h1>
          </div>

          <div className="flex items-center gap-2 md:gap-4">
            <SyncStatus />
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
            <div className="flex items-center gap-2">
              <div className="w-7 h-7 bg-primary-500/10 border border-primary-500/20 rounded-full flex items-center justify-center">
                <span className="text-xs font-medium text-primary-400">
                  {user?.email?.charAt(0).toUpperCase()}
                </span>
              </div>
              <span className="text-xs text-dark-text-tertiary hidden md:inline">{user?.email}</span>
            </div>
            <button
              onClick={handleLogout}
              className="inline-flex items-center gap-1.5 px-2.5 py-1.5 text-xs font-medium text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary rounded-md transition-all duration-150"
              title="Logout"
            >
              <svg
                className="w-3.5 h-3.5"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"
                />
              </svg>
              <span className="hidden md:inline">Logout</span>
            </button>
          </div>
        </div>
      </header>

      {/* Project invitation banner */}
      <ProjectInvitationBanner />

      {/* Main Layout */}
      <div className="flex flex-1 overflow-hidden">
        {/* Sidebar */}
        <Sidebar
          onCreateProject={() => setIsProjectModalOpen(true)}
          isOpen={sidebarOpen}
          onClose={() => setSidebarOpen(false)}
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
  )
}
