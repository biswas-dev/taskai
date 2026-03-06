import { useState, useEffect, useMemo, useRef, useCallback } from 'react'
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

type FilterType = 'all' | 'task_comment' | 'task_created' | 'wiki_page' | 'wiki_edit' | 'annotation_created' | 'annotation_comment'

const TYPE_FILTERS: { value: FilterType; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'task_comment', label: 'Comments' },
  { value: 'task_created', label: 'Tasks' },
  { value: 'wiki_page', label: 'Wiki pages' },
  { value: 'wiki_edit', label: 'Wiki edits' },
  { value: 'annotation_created', label: 'Annotations' },
  { value: 'annotation_comment', label: 'Ann. comments' },
]

function getDateGroup(date: Date): string {
  const now = new Date()
  const diffDays = (now.getTime() - date.getTime()) / (1000 * 60 * 60 * 24)
  if (diffDays < 1) return 'Today'
  if (diffDays < 7) return 'This week'
  return 'Earlier'
}

function ActivityItem({ item }: { item: UserProfileActivity }) {
  return (
    <Link
      to={item.link}
      className="flex items-start gap-3 px-4 py-3 hover:bg-dark-bg-tertiary rounded-lg transition-colors group"
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

export default function UserProfile() {
  const { userId } = useParams<{ userId: string }>()
  const navigate = useNavigate()
  const [profile, setProfile] = useState<UserProfileType | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [search, setSearch] = useState('')
  const [typeFilter, setTypeFilter] = useState<FilterType>('all')
  const [projectFilter, setProjectFilter] = useState<string>('all')

  // Lazy load state
  const [allActivity, setAllActivity] = useState<UserProfileActivity[]>([])
  const [hasMore, setHasMore] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)
  const sentinelRef = useRef<HTMLDivElement>(null)
  const observerRef = useRef<IntersectionObserver | null>(null)

  useEffect(() => {
    if (!userId) return
    setLoading(true)
    setError('')
    setSearch('')
    setTypeFilter('all')
    setProjectFilter('all')
    setAllActivity([])
    setHasMore(false)
    apiClient.getUserProfile(parseInt(userId, 10))
      .then(p => {
        setProfile(p)
        setAllActivity(p.recent_activity)
        setHasMore(p.has_more)
      })
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load profile'))
      .finally(() => setLoading(false))
  }, [userId])

  const loadMore = useCallback(() => {
    if (!userId || loadingMore || !hasMore || allActivity.length === 0) return
    const cursor = allActivity[allActivity.length - 1].created_at
    setLoadingMore(true)
    apiClient.getUserActivityFeed(parseInt(userId, 10), cursor)
      .then(page => {
        setAllActivity(prev => [...prev, ...page.items])
        setHasMore(page.has_more)
      })
      .catch(() => { /* silently ignore — sentinel will retry on next intersection */ })
      .finally(() => setLoadingMore(false))
  }, [userId, loadingMore, hasMore, allActivity])

  // Set up IntersectionObserver on sentinel
  useEffect(() => {
    if (observerRef.current) observerRef.current.disconnect()
    observerRef.current = new IntersectionObserver(entries => {
      if (entries[0].isIntersecting) loadMore()
    }, { threshold: 0.1 })
    if (sentinelRef.current) observerRef.current.observe(sentinelRef.current)
    return () => observerRef.current?.disconnect()
  }, [loadMore])

  const { user } = profile ?? { user: null }

  // Derive unique projects from all activity
  const projects = useMemo(() => {
    const map = new Map<string, string>()
    for (const a of allActivity) {
      map.set(String(a.project_id), a.project_name)
    }
    return Array.from(map.entries()).map(([id, name]) => ({ id, name }))
  }, [allActivity])

  // Apply filters
  const filtered = useMemo(() => {
    const q = search.toLowerCase().trim()
    return allActivity.filter(a => {
      if (typeFilter !== 'all' && a.type !== typeFilter) return false
      if (projectFilter !== 'all' && String(a.project_id) !== projectFilter) return false
      if (q && !a.entity_title.toLowerCase().includes(q) && !a.project_name.toLowerCase().includes(q)) return false
      return true
    })
  }, [allActivity, search, typeFilter, projectFilter])

  // Group filtered activities by date
  const groups = useMemo(() => {
    const groupMap = new Map<string, UserProfileActivity[]>()
    for (const item of filtered) {
      const label = getDateGroup(new Date(item.created_at))
      if (!groupMap.has(label)) groupMap.set(label, [])
      groupMap.get(label)!.push(item)
    }
    return ['Today', 'This week', 'Earlier']
      .filter(l => groupMap.has(l))
      .map(label => ({ label, items: groupMap.get(label)! }))
  }, [filtered])

  // Activity count summary (total, not filtered)
  const summary = useMemo(() => {
    const counts: Record<string, number> = {}
    for (const a of allActivity) counts[a.type] = (counts[a.type] ?? 0) + 1
    const parts: string[] = []
    if (counts.task_comment) parts.push(`${counts.task_comment} comment${counts.task_comment !== 1 ? 's' : ''}`)
    if (counts.task_created) parts.push(`${counts.task_created} task${counts.task_created !== 1 ? 's' : ''} created`)
    if (counts.wiki_page) parts.push(`${counts.wiki_page} wiki page${counts.wiki_page !== 1 ? 's' : ''}`)
    if (counts.annotation_created) parts.push(`${counts.annotation_created} annotation${counts.annotation_created !== 1 ? 's' : ''}`)
    if (counts.annotation_comment) parts.push(`${counts.annotation_comment} ann. comment${counts.annotation_comment !== 1 ? 's' : ''}`)
    if (counts.wiki_edit) parts.push(`${counts.wiki_edit} wiki edit${counts.wiki_edit !== 1 ? 's' : ''}`)
    return parts.join(' · ')
  }, [allActivity])

  const filtersActive = typeFilter !== 'all' || projectFilter !== 'all' || search.trim() !== ''

  if (loading) {
    return (
      <div className="flex items-center justify-center py-20">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-400" />
      </div>
    )
  }

  if (error || !profile || !user) {
    return (
      <div className="max-w-2xl mx-auto px-4 py-8">
        <button onClick={() => navigate(-1)} className="text-sm text-dark-text-tertiary hover:text-dark-text-secondary mb-4 flex items-center gap-1">
          ← Back
        </button>
        <p className="text-danger-400">{error || 'User not found'}</p>
      </div>
    )
  }

  const displayName = user.name
    ?? (user.first_name || user.last_name ? [user.first_name, user.last_name].filter(Boolean).join(' ') : null)
    ?? user.email

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
            {(user.first_name || user.last_name) && user.name && user.name !== [user.first_name, user.last_name].filter(Boolean).join(' ') && (
              <p className="text-sm text-dark-text-tertiary">{[user.first_name, user.last_name].filter(Boolean).join(' ')}</p>
            )}
            <p className="text-sm text-dark-text-tertiary">{user.email}</p>
            {user.joined_at && (
              <p className="text-xs text-dark-text-tertiary mt-1">
                Joined {new Date(user.joined_at).toLocaleDateString(undefined, { year: 'numeric', month: 'long' })}
              </p>
            )}
            {summary && <p className="text-xs text-dark-text-tertiary mt-0.5">{summary}</p>}
          </div>
        </div>
      </div>

      {/* Activity feed */}
      <div className="bg-dark-bg-secondary border border-dark-border-subtle rounded-xl">
        <div className="px-4 py-3 border-b border-dark-border-subtle space-y-3">
          <div className="flex items-center justify-between gap-3">
            <h2 className="text-sm font-semibold text-dark-text-primary shrink-0">Recent Activity</h2>
            {/* Search */}
            <input
              type="text"
              placeholder="Search…"
              value={search}
              onChange={e => setSearch(e.target.value)}
              className="flex-1 min-w-0 px-3 py-1.5 bg-dark-bg-primary border border-dark-border-subtle rounded text-xs text-dark-text-primary placeholder-dark-text-tertiary/60 focus:outline-none focus:border-primary-500"
            />
            {/* Project filter */}
            {projects.length > 1 && (
              <select
                value={projectFilter}
                onChange={e => setProjectFilter(e.target.value)}
                className="shrink-0 px-2 py-1.5 bg-dark-bg-primary border border-dark-border-subtle rounded text-xs text-dark-text-primary focus:outline-none focus:border-primary-500"
              >
                <option value="all">All projects</option>
                {projects.map(p => (
                  <option key={p.id} value={p.id}>{p.name}</option>
                ))}
              </select>
            )}
          </div>
          {/* Type filter chips */}
          {allActivity.length > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {TYPE_FILTERS.filter(f => f.value === 'all' || allActivity.some(a => a.type === f.value)).map(f => (
                <button
                  key={f.value}
                  onClick={() => setTypeFilter(f.value)}
                  className={`px-2.5 py-1 rounded-full text-xs font-medium transition-colors ${
                    typeFilter === f.value
                      ? 'bg-primary-500/20 text-primary-400 border border-primary-500/40'
                      : 'bg-dark-bg-primary text-dark-text-tertiary border border-dark-border-subtle hover:text-dark-text-secondary'
                  }`}
                >
                  {f.label}
                </button>
              ))}
            </div>
          )}
        </div>

        {filtered.length === 0 ? (
          <p className="px-4 py-6 text-sm text-center text-dark-text-tertiary">
            {filtersActive ? 'No activity matches your filters' : 'No recent activity'}
          </p>
        ) : (
          <div className="py-2">
            {filtersActive && (
              <p className="px-4 pb-1 text-xs text-dark-text-tertiary">
                {filtered.length} result{filtered.length !== 1 ? 's' : ''}
                {' '}<button onClick={() => { setSearch(''); setTypeFilter('all'); setProjectFilter('all') }} className="text-primary-400 hover:underline">clear</button>
              </p>
            )}
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

        {/* Sentinel for infinite scroll */}
        <div ref={sentinelRef} className="h-1" />

        {loadingMore && (
          <div className="flex justify-center py-4">
            <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-primary-400" />
          </div>
        )}
      </div>
    </div>
  )
}
