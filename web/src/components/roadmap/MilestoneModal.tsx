import { useState, FormEvent, useEffect } from 'react'
import { api, type Milestone, type CreateMilestoneRequest, type UpdateMilestoneRequest } from '../../lib/api'
import TextInput from '../ui/TextInput'
import Button from '../ui/Button'
import Select from '../ui/Select'

const MILESTONE_COLORS = [
  '#5e6ad2', '#6366f1', '#8b5cf6', '#a855f7',
  '#ec4899', '#ef4444', '#f97316', '#eab308',
  '#22c55e', '#14b8a6', '#06b6d4', '#3b82f6',
]

interface MilestoneModalProps {
  isOpen: boolean
  onClose: () => void
  onSaved: (milestone: Milestone) => void
  projectId: number
  milestone?: Milestone | null
}

export default function MilestoneModal({
  isOpen,
  onClose,
  onSaved,
  projectId,
  milestone,
}: MilestoneModalProps) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [color, setColor] = useState('#5e6ad2')
  const [targetDate, setTargetDate] = useState('')
  const [status, setStatus] = useState<'active' | 'completed' | 'cancelled'>('active')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const isEditing = !!milestone

  useEffect(() => {
    if (milestone) {
      setName(milestone.name)
      setDescription(milestone.description || '')
      setColor(milestone.color)
      setTargetDate(milestone.target_date || '')
      setStatus(milestone.status)
    } else {
      setName('')
      setDescription('')
      setColor('#5e6ad2')
      setTargetDate('')
      setStatus('active')
    }
    setError(null)
  }, [milestone, isOpen])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError(null)

    if (!name.trim()) {
      setError('Milestone name is required')
      return
    }

    try {
      setLoading(true)
      let saved: Milestone

      if (isEditing && milestone) {
        const data: UpdateMilestoneRequest = {
          name: name.trim(),
          description: description.trim(),
          color,
          target_date: targetDate || '',
          status,
        }
        saved = await api.updateMilestone(milestone.id, data)
      } else {
        const data: CreateMilestoneRequest = {
          name: name.trim(),
          description: description.trim() || undefined,
          color,
          target_date: targetDate || undefined,
          status,
        }
        saved = await api.createMilestone(projectId, data)
      }

      onSaved(saved)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save milestone')
    } finally {
      setLoading(false)
    }
  }

  const handleClose = () => {
    if (!loading) onClose()
  }

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 z-50 overflow-y-auto">
      <div
        className="fixed inset-0 bg-black bg-opacity-75 transition-opacity"
        onClick={handleClose}
      />
      <div className="flex min-h-full items-center justify-center p-4">
        <div className="relative bg-dark-bg-secondary rounded-lg shadow-xl max-w-md w-full border border-dark-border-subtle">
          <div className="flex items-center justify-between p-6 border-b border-dark-border-subtle">
            <h3 className="text-xl font-semibold text-dark-text-primary">
              {isEditing ? 'Edit Milestone' : 'Create Milestone'}
            </h3>
            <button
              onClick={handleClose}
              disabled={loading}
              className="text-dark-text-tertiary hover:text-dark-text-primary transition-colors disabled:opacity-50"
              aria-label="Close modal"
            >
              <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>

          <form onSubmit={handleSubmit}>
            <div className="p-6 space-y-4">
              {error && (
                <div className="bg-danger-500/10 border border-danger-500/30 text-danger-400 px-4 py-3 rounded" role="alert">
                  <p className="text-sm">{error}</p>
                </div>
              )}

              <TextInput
                id="milestone-name"
                name="name"
                type="text"
                label="Name"
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="v2.0 Launch"
                disabled={loading}
                autoFocus
              />

              <div className="w-full">
                <label htmlFor="milestone-description" className="block text-sm font-medium text-dark-text-primary mb-1">
                  Description <span className="text-dark-text-tertiary font-normal">(optional)</span>
                </label>
                <textarea
                  id="milestone-description"
                  rows={2}
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="What does this milestone deliver?"
                  disabled={loading}
                  className="w-full px-3 py-2 border border-dark-border-subtle bg-dark-bg-primary text-dark-text-primary placeholder-dark-text-tertiary rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500 transition-all duration-200 disabled:opacity-50 resize-none"
                />
              </div>

              <div className="w-full">
                <label className="block text-sm font-medium text-dark-text-primary mb-2">Color</label>
                <div className="flex flex-wrap gap-2">
                  {MILESTONE_COLORS.map((c) => (
                    <button
                      key={c}
                      type="button"
                      onClick={() => setColor(c)}
                      className={`w-7 h-7 rounded-full transition-all ${color === c ? 'ring-2 ring-offset-2 ring-offset-dark-bg-secondary ring-white scale-110' : 'hover:scale-105'}`}
                      style={{ backgroundColor: c }}
                      aria-label={`Select color ${c}`}
                    />
                  ))}
                </div>
              </div>

              <TextInput
                id="milestone-target-date"
                name="target_date"
                type="date"
                label="Target Date"
                value={targetDate}
                onChange={(e) => setTargetDate(e.target.value)}
                disabled={loading}
              />

              {isEditing && (
                <div className="w-full">
                  <label htmlFor="milestone-status" className="block text-sm font-medium text-dark-text-primary mb-1">Status</label>
                  <Select<'active' | 'completed' | 'cancelled'>
                    id="milestone-status"
                    className="w-full"
                    value={status}
                    onChange={setStatus}
                    disabled={loading}
                    options={[
                      { value: 'active', label: 'Active' },
                      { value: 'completed', label: 'Completed' },
                      { value: 'cancelled', label: 'Cancelled' },
                    ]}
                  />
                </div>
              )}
            </div>

            <div className="flex justify-end gap-3 p-6 border-t border-dark-border-subtle">
              <Button type="button" variant="secondary" onClick={handleClose} disabled={loading}>
                Cancel
              </Button>
              <Button type="submit" disabled={loading}>
                {loading ? 'Saving...' : isEditing ? 'Update' : 'Create'}
              </Button>
            </div>
          </form>
        </div>
      </div>
    </div>
  )
}
