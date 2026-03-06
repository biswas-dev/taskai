import { useState, useEffect } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { apiClient, UserProfile as UserProfileType, UserProfileActivity } from '../lib/api'

const activityIcon: Record<string, string> = {
  task_comment: '💬',
  wiki_page: '📄',
  annotation_comment: '💭',
  task_created: '✅',
  annotation_created: '📌',
  wiki_edit: '✏️',
}

const activityLabel: Record<string, string> = {
  task_comment: 'Commented on task',
  wiki_page: 'Created wiki page',
  annotation_comment: 'Commented on annotation',
  task_created: 'Created task',
  annotation_created: 'Created annotation',
  wiki_edit: 'Edited wiki page',
}

function getDateGroup(date: Date): string {
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffDays = diffMs / (1000 * 60 * 60 * 24)
  if (diffDays < 1) return 'Today'
  if (diffDays < 7) return 'This week'
  return 'Earlier'
}

function ActivityItem({ item }: { item: UserProfileActivity }) {
  return (
    <Link
      to={item.link}
      className="flex items-start gap-3 px-4 py-3 hover:bg-dark-bg-secondary rounded-lg transition-colors group"
    >
      <span className="text-base mt-0.5 shrink-0">{activityIcon[item.type] ?? '🔔'}</span>
      <div className="min-w-0 flex-1">
        <p className="text-xs text-dark-text-tertiary">{activityLabel[item.type] ?? item.type}</p>
        <p className="text-sm text-dark-text-primary font-medium truncate group-hover:text-primary-400 transition-colors">
          {item.entity_title}
        </p>
        <p className="text-xs text-dark-text-tertiary mt-0.5">
          {item.project_name} · {new Date(item.created_at).toLocaleDateString()}
        </p>
      </div>
    </Link>
  )
}

function ActivitySummary({ activities }: { activities: UserProfileActivity[] }) {
  const counts: Record<string, number> = {}
  for (const a of activities) {
    counts[a.type] = (counts[a.type] ?? 0) + 1
  }
  const parts: string[] = []
  if (counts.task_comment) parts.push(`${counts.task_comment} comment${counts.task_comment !== 1 ? 's' : ''}`)
  if (counts.task_created) parts.push(`${counts.task_created} task${counts.task_created !== 1 ? 's' : ''} created`)
  if (counts.wiki_page) parts.push(`${counts.wiki_page} wiki page${counts.wiki_page !== 1 ? 's' : ''}`)
  if (counts.annotation_created) parts.push(`${counts.annotation_created} annotation${counts.annotation_created !== 1 ? 's' : ''}`)
  if (counts.annotation_comment) parts.push(`${counts.annotation_comment} annotation comment${counts.annotation_comment !== 1 ? 's' : ''}`)
  if (counts.wiki_edit) parts.push(`${counts.wiki_edit} wiki edit${counts.wiki_edit !== 1 ? 's' : ''}`)
  if (!parts.length) return null
  return (
    <p className="text-xs text-dark-text-tertiary mt-1">{parts.join(' · ')}</p>
  )
}

export default function UserProfile() {
  const { userId } = useParams<{ userId: string }>()
  const navigate = useNavigate()
  const [profile, setProfile] = useState<UserProfileType | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!userId) return
    setLoading(true)
    setError('')
    apiClient.getUserProfile(parseInt(userId, 10))
      .then(setProfile)
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load profile'))
      .finally(() => setLoading(false))
  }, [userId])

  if (loading) {
    return (
      <div className="flex items-center justify-center py-20">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-400" />
      </div>
    )
  }

  if (error || !profile) {
    return (
      <div className="max-w-2xl mx-auto px-4 py-8">
        <button onClick={() => navigate(-1)} className="text-sm text-dark-text-tertiary hover:text-dark-text-secondary mb-4 flex items-center gap-1">
          ← Back
        </button>
        <p className="text-danger-400">{error || 'User not found'}</p>
      </div>
    )
  }

  const { user, recent_activity } = profile
  const displayName = user.name ?? user.user_name ?? user.email

  // Group activities by date
  const groups: { label: string; items: UserProfileActivity[] }[] = []
  const groupMap = new Map<string, UserProfileActivity[]>()
  const groupOrder = ['Today', 'This week', 'Earlier']
  for (const item of recent_activity) {
    const label = getDateGroup(new Date(item.created_at))
    if (!groupMap.has(label)) groupMap.set(label, [])
    groupMap.get(label)!.push(item)
  }
  for (const label of groupOrder) {
    const items = groupMap.get(label)
    if (items?.length) groups.push({ label, items })
  }

  return (
    <div className="max-w-2xl mx-auto px-4 py-8">
      <button onClick={() => navigate(-1)} className="text-sm text-dark-text-tertiary hover:text-dark-text-secondary mb-6 flex items-center gap-1">
        ← Back
      </button>

      {/* Profile header */}
      <div className="bg-dark-bg-secondary border border-dark-border-subtle rounded-xl p-6 mb-6">
        <div className="flex items-center gap-4">
          <div className="w-14 h-14 rounded-full bg-primary-500/20 flex items-center justify-center text-2xl font-bold text-primary-400 shrink-0">
            {displayName.charAt(0).toUpperCase()}
          </div>
          <div>
            <h1 className="text-xl font-semibold text-dark-text-primary">{displayName}</h1>
            {user.user_name && user.name && (
              <p className="text-sm text-dark-text-tertiary">@{user.user_name}</p>
            )}
            <p className="text-sm text-dark-text-tertiary">{user.email}</p>
            {user.joined_at && (
              <p className="text-xs text-dark-text-tertiary mt-1">
                Joined {new Date(user.joined_at).toLocaleDateString(undefined, { year: 'numeric', month: 'long' })}
              </p>
            )}
            <ActivitySummary activities={recent_activity} />
          </div>
        </div>
      </div>

      {/* Activity feed */}
      <div className="bg-dark-bg-secondary border border-dark-border-subtle rounded-xl">
        <div className="px-4 py-3 border-b border-dark-border-subtle">
          <h2 className="text-sm font-semibold text-dark-text-primary">Recent Activity</h2>
        </div>
        {recent_activity.length === 0 ? (
          <p className="px-4 py-6 text-sm text-center text-dark-text-tertiary">No recent activity</p>
        ) : (
          <div className="py-2">
            {groups.map(group => (
              <div key={group.label}>
                <p className="px-4 pt-3 pb-1 text-xs font-semibold text-dark-text-tertiary uppercase tracking-wide">
                  {group.label}
                </p>
                {group.items.map((item, idx) => (
                  <ActivityItem key={`${item.type}-${item.entity_id}-${idx}`} item={item} />
                ))}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
