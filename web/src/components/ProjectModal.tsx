import { useState, useEffect, FormEvent } from 'react'
import { api, Project, type TeamSummary } from '../lib/api'
import TextInput from './ui/TextInput'
import Button from './ui/Button'
import Select from './ui/Select'

interface ProjectModalProps {
  isOpen: boolean
  onClose: () => void
  onProjectCreated: (project: Project) => void
}

export default function ProjectModal({
  isOpen,
  onClose,
  onProjectCreated,
}: ProjectModalProps) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [teams, setTeams] = useState<TeamSummary[]>([])
  const [teamId, setTeamId] = useState<number | null>(null)

  // People in several teams choose where the project lives; everyone else
  // gets their primary team without seeing a picker.
  useEffect(() => {
    if (!isOpen) return
    let cancelled = false
    const load = async () => {
      try {
        const list = await api.listTeams()
        if (cancelled) return
        setTeams(list)
        setTeamId((current) => current ?? list.find((t) => t.is_home)?.id ?? list[0]?.id ?? null)
      } catch {
        // Without the list the server falls back to the primary team.
      }
    }
    load()
    return () => {
      cancelled = true
    }
  }, [isOpen])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError(null)

    if (!name.trim()) {
      setError('Project name is required')
      return
    }

    try {
      setLoading(true)
      const project = await api.createProject({
        name: name.trim(),
        description: description.trim() || undefined,
        ...(teams.length > 1 && teamId ? { team_id: teamId } : {}),
      })
      onProjectCreated(project)
      // Reset form
      setName('')
      setDescription('')
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create project')
    } finally {
      setLoading(false)
    }
  }

  const handleClose = () => {
    if (!loading) {
      setName('')
      setDescription('')
      setError(null)
      onClose()
    }
  }

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 z-50 overflow-y-auto">
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black bg-opacity-75 transition-opacity"
        onClick={handleClose}
      ></div>

      {/* Modal */}
      <div className="flex min-h-full items-center justify-center p-4">
        <div className="relative bg-dark-bg-secondary rounded-lg shadow-xl max-w-md w-full border border-dark-border-subtle">
          {/* Header */}
          <div className="flex items-center justify-between p-6 border-b border-dark-border-subtle">
            <h3 className="text-xl font-semibold text-dark-text-primary">
              Create New Project
            </h3>
            <button
              onClick={handleClose}
              disabled={loading}
              className="text-dark-text-tertiary hover:text-dark-text-primary transition-colors disabled:opacity-50"
              aria-label="Close modal"
            >
              <svg
                className="w-6 h-6"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M6 18L18 6M6 6l12 12"
                />
              </svg>
            </button>
          </div>

          {/* Body */}
          <form onSubmit={handleSubmit}>
            <div className="p-6 space-y-4">
              {error && (
                <div
                  className="bg-danger-500/10 border border-danger-500/30 text-danger-400 px-4 py-3 rounded"
                  role="alert"
                >
                  <p className="text-sm">{error}</p>
                </div>
              )}

              <TextInput
                id="project-name"
                name="name"
                type="text"
                label="Project Name"
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="My Awesome Project"
                disabled={loading}
                autoFocus
              />

              <div className="w-full">
                <label
                  htmlFor="project-description"
                  className="block text-sm font-medium text-dark-text-primary mb-1"
                >
                  Description{' '}
                  <span className="text-dark-text-tertiary font-normal">(optional)</span>
                </label>
                <textarea
                  id="project-description"
                  name="description"
                  rows={3}
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="What is this project about?"
                  disabled={loading}
                  className="w-full px-3 py-2 border border-dark-border-subtle bg-dark-bg-primary text-dark-text-primary placeholder-dark-text-tertiary rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500 transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed resize-none"
                />
              </div>

              {teams.length > 1 && (
                <div className="w-full">
                  <label htmlFor="project-team" className="block text-sm font-medium text-dark-text-primary mb-1">
                    Team
                  </label>
                  <Select
                    id="project-team"
                    className="w-full"
                    value={teamId ?? null}
                    onChange={setTeamId}
                    disabled={loading}
                    placeholder="Select a team"
                    searchPlaceholder="Search your teams…"
                    options={teams.map((t) => ({ value: t.id, label: t.name }))}
                  />
                  <p className="mt-1 text-xs text-dark-text-tertiary">
                    Members of this team are suggested first when you share the project.
                  </p>
                </div>
              )}
            </div>

            {/* Footer */}
            <div className="flex items-center justify-end gap-3 p-6 border-t border-dark-border-subtle bg-dark-bg-primary rounded-b-lg">
              <Button
                type="button"
                variant="outline"
                onClick={handleClose}
                disabled={loading}
              >
                Cancel
              </Button>
              <Button type="submit" variant="primary" loading={loading}>
                Create Project
              </Button>
            </div>
          </form>
        </div>
      </div>
    </div>
  )
}
