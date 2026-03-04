import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'

// Projects welcome screen
export default function Projects() {
  const navigate = useNavigate()

  useEffect(() => {
    const lastProject = localStorage.getItem('taskai_last_project')
    if (lastProject) {
      navigate(`/app/projects/${lastProject}`, { replace: true })
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="flex items-center justify-center h-full bg-dark-bg-base">
      <div className="text-center max-w-lg px-6">
        <div className="w-16 h-16 bg-gradient-to-br from-primary-500/20 to-primary-600/10 rounded-xl flex items-center justify-center mx-auto mb-6 border border-primary-500/20">
          <svg
            className="w-8 h-8 text-primary-400"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={1.5}
              d="M13 10V3L4 14h7v7l9-11h-7z"
            />
          </svg>
        </div>
        <h2 className="text-2xl font-semibold text-dark-text-primary mb-3 tracking-tight">
          Welcome to TaskAI
        </h2>
        <p className="text-sm text-dark-text-tertiary mb-8 leading-relaxed">
          Select a project from the sidebar to view its tasks, or create a new
          project to get started.
        </p>
        <div className="flex flex-col gap-3 text-xs text-dark-text-tertiary">
          <div className="flex items-center gap-3">
            <div className="w-5 h-5 rounded-full bg-primary-500/10 flex items-center justify-center flex-shrink-0">
              <svg className="w-3 h-3 text-primary-400" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
              </svg>
            </div>
            <span>Organize your work into projects</span>
          </div>
          <div className="flex items-center gap-3">
            <div className="w-5 h-5 rounded-full bg-primary-500/10 flex items-center justify-center flex-shrink-0">
              <svg className="w-3 h-3 text-primary-400" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
              </svg>
            </div>
            <span>Track tasks with customizable swim lanes</span>
          </div>
          <div className="flex items-center gap-3">
            <div className="w-5 h-5 rounded-full bg-primary-500/10 flex items-center justify-center flex-shrink-0">
              <svg className="w-3 h-3 text-primary-400" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
              </svg>
            </div>
            <span>Stay productive and ship faster</span>
          </div>
        </div>
      </div>
    </div>
  )
}
