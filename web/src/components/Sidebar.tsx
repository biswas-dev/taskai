import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams, useLocation } from 'react-router-dom'
import { api, Project } from '../lib/api'
import { useAuth } from '../state/AuthContext'

interface SidebarProps {
  onCreateProject: () => void
  onLogout: () => void
  isOpen?: boolean
  onClose?: () => void
  isPinned?: boolean
  onTogglePin?: () => void
}

// A team is the company; its projects (Engineering, Sales, ...) are listed under it in API order.
function groupByCompany(projects: Project[]): [string, Project[]][] {
  const groups = new Map<string, Project[]>()
  for (const p of projects) {
    const company = p.team_name || 'Projects'
    groups.set(company, [...(groups.get(company) ?? []), p])
  }
  return [...groups]
}

export default function Sidebar({ onCreateProject, onLogout, isOpen, onClose, isPinned, onTogglePin }: SidebarProps) {
  const [projects, setProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [profileMenuOpen, setProfileMenuOpen] = useState(false)
  const profileMenuRef = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()
  const location = useLocation()
  const { user } = useAuth()
  const { projectId: urlProjectId } = useParams<{ projectId: string }>()

  // Remember last selected project so nav items stay active on Settings/Admin pages
  const [lastProjectId, setLastProjectId] = useState<string | null>(
    () => localStorage.getItem('taskai_last_project')
  )
  useEffect(() => {
    if (urlProjectId) {
      setLastProjectId(urlProjectId)
      // Note: ProjectDetail also sets this key; keep in sync
      localStorage.setItem('taskai_last_project', urlProjectId)
    }
  }, [urlProjectId])

  const selectedProjectId = urlProjectId || lastProjectId

  useEffect(() => {
    loadProjects()
  }, [])

  // Reload projects when a project membership changes (invitation accepted)
  useEffect(() => {
    const handler = () => loadProjects()
    window.addEventListener('project-membership-changed', handler)
    return () => window.removeEventListener('project-membership-changed', handler)
  }, [])

  // Close profile menu on click outside
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (profileMenuRef.current && !profileMenuRef.current.contains(e.target as Node)) {
        setProfileMenuOpen(false)
      }
    }
    if (profileMenuOpen) {
      document.addEventListener('mousedown', handleClickOutside)
      return () => document.removeEventListener('mousedown', handleClickOutside)
    }
  }, [profileMenuOpen])

  const loadProjects = async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await api.getProjects()
      setProjects(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load projects')
    } finally {
      setLoading(false)
    }
  }

  const handleProjectClick = (projectId: number | undefined) => {
    if (projectId) {
      navigate(`/app/projects/${projectId}`)
      onClose?.()
    }
  }

  const addProject = (project: Project) => {
    // Create responses omit team_name; borrow it from a sibling so the project lands under its company.
    setProjects(prev => [{ ...project, team_name: project.team_name ?? prev.find(p => p.team_id === project.team_id)?.team_name }, ...prev])
  }

  // Expose addProject method to parent via window callback
  useEffect(() => {
    const w = window as Window & { __addProject?: (project: Project) => void }
    w.__addProject = addProject
    return () => {
      delete w.__addProject
    }
  }, [projects])

  const sprintsPath = selectedProjectId ? `/app/projects/${selectedProjectId}/sprints` : null
  const tagsPath = selectedProjectId ? `/app/projects/${selectedProjectId}/tags` : null
  const assetsPath = selectedProjectId ? `/app/projects/${selectedProjectId}/assets` : null
  const graphPath = selectedProjectId ? `/app/projects/${selectedProjectId}/graph` : null

  const navItems = [
    {
      label: 'Memories',
      path: '/app/memories',
      icon: (
        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z" />
        </svg>
      ),
    },
    {
      label: 'Sprints',
      path: sprintsPath,
      icon: (
        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M13 10V3L4 14h7v7l9-11h-7z" />
        </svg>
      ),
    },
    {
      label: 'Tags',
      path: tagsPath,
      icon: (
        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z" />
        </svg>
      ),
    },
    {
      label: 'Assets',
      path: assetsPath,
      icon: (
        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
        </svg>
      ),
    },
    {
      label: 'Graph',
      path: graphPath,
      icon: (
        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M4 6a2 2 0 100 4 2 2 0 000-4zM20 6a2 2 0 100 4 2 2 0 000-4zM12 16a2 2 0 100 4 2 2 0 000-4zM6 8l6 6M18 8l-6 6" />
        </svg>
      ),
    },
  ]

  const sidebarContent = (
    <div className="w-64 bg-dark-bg-primary border-r border-dark-border-subtle flex flex-col h-full">
      {/* Header row: close/pin buttons */}
      <div className="flex items-center justify-between px-4 pt-3 pb-1">
        <span className="text-[10px] font-semibold text-dark-text-quaternary uppercase tracking-wider">Menu</span>
        <div className="flex items-center gap-1">
          {/* Pin/unpin button */}
          {onTogglePin && (
            <button
              onClick={onTogglePin}
              className="p-1.5 text-dark-text-quaternary hover:text-dark-text-primary hover:bg-dark-bg-tertiary rounded-md transition-colors"
              aria-label={isPinned ? 'Unpin sidebar' : 'Pin sidebar'}
              title={isPinned ? 'Unpin sidebar' : 'Pin sidebar'}
            >
              {isPinned ? (
                // Filled thumbtack — pinned
                <svg className="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M16 12V4h1V2H7v2h1v8l-2 2v2h5.2v6h1.6v-6H18v-2l-2-2z" />
                </svg>
              ) : (
                // Outline thumbtack — not pinned (same shape, stroked)
                <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round">
                  <path d="M16 12V4h1V2H7v2h1v8l-2 2v2h5.2v6h1.6v-6H18v-2l-2-2z" />
                </svg>
              )}
            </button>
          )}
          {/* Close button — only shown in overlay mode */}
          {!isPinned && (
            <button
              onClick={onClose}
              className="p-1.5 text-dark-text-tertiary hover:text-dark-text-primary hover:bg-dark-bg-tertiary rounded-md transition-colors"
              aria-label="Close sidebar"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          )}
        </div>
      </div>

      {/* New Project Button */}
      <div className="p-4 border-b border-dark-border-subtle">
        <button
          onClick={onCreateProject}
          className="w-full bg-primary-500 hover:bg-primary-600 text-white font-medium py-2 px-3 rounded-md transition-all duration-150 flex items-center justify-center gap-2 text-sm shadow-linear-sm"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          New Project
        </button>
      </div>

      {/* Navigation */}
      <div className="p-4 border-b border-dark-border-subtle">
        <p className="text-[10px] uppercase tracking-wider text-dark-text-quaternary mb-2 px-3 font-medium">Navigation</p>
        <div className="space-y-0.5">
          {navItems.map((item) => {
            const isActive = item.path !== null && location.pathname === item.path
            const isDisabled = item.path === null
            return (
              <button
                key={item.label}
                onClick={() => { if (item.path) { navigate(item.path); onClose?.() } }}
                disabled={isDisabled}
                title={isDisabled ? 'Select a project first' : undefined}
                className={`w-full font-medium py-2.5 md:py-1.5 px-3 rounded-md transition-all duration-150 flex items-center gap-2.5 text-sm ${
                  isDisabled
                    ? 'text-dark-text-quaternary opacity-50 cursor-not-allowed'
                    : isActive
                    ? 'bg-dark-bg-tertiary text-dark-text-primary'
                    : 'text-dark-text-tertiary hover:bg-dark-bg-secondary hover:text-dark-text-primary'
                }`}
              >
                {item.icon}
                {item.label}
              </button>
            )
          })}
        </div>
      </div>

      {/* Projects List */}
      <div className="flex-1 overflow-y-auto p-4">
        {(loading || projects.length === 0) && (
          <p className="text-[10px] uppercase tracking-wider text-dark-text-quaternary mb-2 px-3 font-medium">Projects</p>
        )}

        {loading && (
          <div className="animate-pulse space-y-2 px-1">
            <div className="h-7 bg-dark-bg-tertiary/40 rounded-md"></div>
            <div className="h-7 bg-dark-bg-tertiary/30 rounded-md"></div>
            <div className="h-7 bg-dark-bg-tertiary/30 rounded-md"></div>
          </div>
        )}

        {error && (
          <div className="bg-danger-500/10 border border-danger-500/20 text-danger-400 px-3 py-2 rounded-md text-xs">
            {error}
          </div>
        )}

        {!loading && projects.length === 0 && !error && (
          <div className="text-center py-8">
            <svg
              className="mx-auto h-8 w-8 text-dark-text-quaternary"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={1.5}
                d="M9 13h6m-3-3v6m5 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
              />
            </svg>
            <p className="mt-2 text-xs text-dark-text-tertiary">No projects yet</p>
            <p className="text-xs text-dark-text-quaternary mt-1">
              Create your first project
            </p>
          </div>
        )}

        {!loading && (
          <div className="space-y-4">
            {groupByCompany(projects).map(([company, companyProjects]) => (
              <section key={company} aria-label={company} className="space-y-0.5">
                <h2 className="text-[10px] uppercase tracking-wider text-dark-text-quaternary mb-2 px-3 font-medium truncate">{company}</h2>
                {companyProjects.map((project) => {
                  const isSelected = selectedProjectId === String(project.id)
                  return (
                    <button
                      key={project.id}
                      onClick={() => handleProjectClick(project.id)}
                      className={`w-full text-left py-2 px-3 rounded-md transition-all duration-150 ${
                        isSelected
                          ? 'bg-dark-bg-tertiary text-dark-text-primary'
                          : 'text-dark-text-tertiary hover:bg-dark-bg-secondary hover:text-dark-text-primary'
                      }`}
                    >
                      <h3
                        className={`font-medium text-xs truncate ${
                          isSelected ? 'text-dark-text-primary' : ''
                        }`}
                      >
                        {project.name}
                      </h3>
                      {project.description && (
                        <p className="text-xs text-dark-text-quaternary truncate mt-0.5">
                          {project.description}
                        </p>
                      )}
                    </button>
                  )
                })}
              </section>
            ))}
          </div>
        )}
      </div>

      {/* User Profile Section */}
      <div className="relative border-t border-dark-border-subtle p-3" ref={profileMenuRef}>
        {/* Profile Menu (opens upward) */}
        {profileMenuOpen && (
          <div className="absolute bottom-full left-2 right-2 mb-1 bg-dark-bg-elevated border border-dark-border-subtle rounded-lg shadow-linear-lg overflow-hidden">
            <div className="py-1">
              <button
                onClick={() => { navigate('/app/settings'); setProfileMenuOpen(false); onClose?.() }}
                className={`w-full text-left px-3 py-2 text-sm flex items-center gap-2.5 ${
                  location.pathname === '/app/settings'
                    ? 'bg-dark-bg-tertiary text-dark-text-primary'
                    : 'text-dark-text-secondary hover:bg-dark-bg-tertiary hover:text-dark-text-primary'
                } transition-colors`}
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                </svg>
                Settings
              </button>

              {user?.is_admin && (
                <button
                  onClick={() => { navigate('/app/admin'); setProfileMenuOpen(false); onClose?.() }}
                  className={`w-full text-left px-3 py-2 text-sm flex items-center gap-2.5 ${
                    location.pathname === '/app/admin'
                      ? 'bg-dark-bg-tertiary text-dark-text-primary'
                      : 'text-dark-text-secondary hover:bg-dark-bg-tertiary hover:text-dark-text-primary'
                  } transition-colors`}
                >
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z" />
                  </svg>
                  Admin
                </button>
              )}

              <div className="border-t border-dark-border-subtle my-1" />

              <button
                onClick={() => { setProfileMenuOpen(false); onLogout() }}
                className="w-full text-left px-3 py-2 text-sm flex items-center gap-2.5 text-dark-text-secondary hover:bg-dark-bg-tertiary hover:text-dark-text-primary transition-colors"
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
                </svg>
                Sign out
              </button>
            </div>
          </div>
        )}

        {/* Profile trigger button */}
        <button
          onClick={() => setProfileMenuOpen(!profileMenuOpen)}
          className="w-full flex items-center gap-2.5 px-2 py-2 rounded-md hover:bg-dark-bg-secondary transition-colors"
          aria-label="User menu"
        >
          <div className="w-8 h-8 bg-primary-500/15 border border-primary-500/25 rounded-full flex items-center justify-center flex-shrink-0">
            <span className="text-xs font-semibold text-primary-400">
              {user?.email?.charAt(0).toUpperCase()}
            </span>
          </div>
          <div className="flex-1 min-w-0 text-left">
            <p className="text-xs font-medium text-dark-text-primary truncate">
              {user?.first_name && user?.last_name
                ? `${user.first_name} ${user.last_name}`
                : user?.email}
            </p>
            {user?.first_name && (
              <p className="text-[11px] text-dark-text-quaternary truncate">{user.email}</p>
            )}
          </div>
          <svg className={`w-4 h-4 text-dark-text-quaternary transition-transform ${profileMenuOpen ? 'rotate-180' : ''}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 15l7-7 7 7" />
          </svg>
        </button>
      </div>
    </div>
  )

  return (
    <>
      {/* Static sidebar — always visible when pinned */}
      {isPinned && (
        <div className="flex-shrink-0">
          {sidebarContent}
        </div>
      )}

      {/* Overlay sidebar — shown when open and not pinned (all screen sizes) */}
      {!isPinned && isOpen && (
        <div className="fixed inset-0 z-40">
          {/* Backdrop */}
          <div
            className="fixed inset-0 bg-black/60 backdrop-blur-sm"
            onClick={onClose}
          />
          {/* Sidebar panel */}
          <div className="fixed inset-y-0 left-0 z-50">
            {sidebarContent}
          </div>
        </div>
      )}
    </>
  )
}
