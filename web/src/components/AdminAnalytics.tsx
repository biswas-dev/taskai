import { useEffect, useState, useCallback } from 'react'
import {
  api,
  type AnalyticsOverview,
  type AnalyticsUserRow,
  type AnalyticsUserDetail,
  type AnalyticsAPIKeyUsage,
} from '../lib/api'

// ── Helpers ─────────────────────────────────────────────────────────────────

function formatDate(dateStr: string) {
  const d = new Date(dateStr)
  return d.toLocaleDateString('en-US', { year: 'numeric', month: '2-digit', day: '2-digit' }) +
    ' ' + d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' })
}

function relativeTime(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime()
  const secs = Math.floor(diff / 1000)
  if (secs < 60) return 'just now'
  const mins = Math.floor(secs / 60)
  if (mins < 60) return `${mins}m ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs}h ago`
  const days = Math.floor(hrs / 24)
  return `${days}d ago`
}

function formatMinutes(mins: number): string {
  if (mins < 1) return '<1m'
  if (mins < 60) return `${Math.round(mins)}m`
  const h = Math.floor(mins / 60)
  const m = Math.round(mins % 60)
  return m > 0 ? `${h}h ${m}m` : `${h}h`
}

// ── Types ───────────────────────────────────────────────────────────────────

interface Props {
  users: { id: number; email: string; name?: string; first_name?: string; last_name?: string }[]
}

type Period = 7 | 30 | 90
type SortKey = 'login_count' | 'page_view_count' | 'api_request_count' | 'tasks_created' | 'total_session_minutes'

// ── Component ───────────────────────────────────────────────────────────────

export default function AdminAnalytics({ users }: Props) {
  const [days, setDays] = useState<Period>(30)
  const [overview, setOverview] = useState<AnalyticsOverview | null>(null)
  const [userRows, setUserRows] = useState<AnalyticsUserRow[]>([])
  const [apiKeys, setApiKeys] = useState<AnalyticsAPIKeyUsage[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // User detail drill-down
  const [selectedUserId, setSelectedUserId] = useState<number | null>(null)
  const [userDetail, setUserDetail] = useState<AnalyticsUserDetail | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)

  // Sort
  const [sortKey, setSortKey] = useState<SortKey>('login_count')
  const [sortDesc, setSortDesc] = useState(true)

  const loadData = useCallback(async (period: Period) => {
    setLoading(true)
    setError(null)
    try {
      const [ov, ur, ak] = await Promise.all([
        api.getAnalyticsOverview(period),
        api.getAnalyticsUsers(period),
        api.getAnalyticsAPIKeys(period),
      ])
      setOverview(ov)
      setUserRows(ur)
      setApiKeys(ak)
    } catch (err) {
      // If endpoints don't exist yet (not deployed), show empty state instead of error
      const msg = err instanceof Error ? err.message : ''
      if (msg.includes('404') || msg.includes('Not Found')) {
        setOverview({
          period_days: period, active_users: 0, total_logins: 0, total_page_views: 0,
          total_api_requests: 0, tasks_created: 0, wiki_pages_created: 0, wiki_edits: 0,
          comments_added: 0, avg_session_duration_minutes: 0, daily_active_users: [],
        })
        setUserRows([])
        setApiKeys([])
        setError('Analytics endpoints not deployed yet — showing empty state. Deploy migration 063 to enable.')
      } else {
        setError(msg || 'Failed to load analytics')
      }
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { loadData(days) }, [days, loadData])

  const loadUserDetail = useCallback(async (userId: number) => {
    if (selectedUserId === userId) {
      setSelectedUserId(null)
      setUserDetail(null)
      return
    }
    setSelectedUserId(userId)
    setDetailLoading(true)
    try {
      const detail = await api.getAnalyticsUserDetail(userId, days)
      setUserDetail(detail)
    } catch {
      setUserDetail(null)
    } finally {
      setDetailLoading(false)
    }
  }, [selectedUserId, days])

  // Sort user rows
  const sortedUsers = [...userRows].sort((a, b) => {
    const av = a[sortKey]
    const bv = b[sortKey]
    return sortDesc ? (bv as number) - (av as number) : (av as number) - (bv as number)
  })

  const handleSort = (key: SortKey) => {
    if (sortKey === key) {
      setSortDesc(!sortDesc)
    } else {
      setSortKey(key)
      setSortDesc(true)
    }
  }

  const sortArrow = (key: SortKey) =>
    sortKey === key ? (sortDesc ? ' \u25BC' : ' \u25B2') : ''

  // ── Render ──────────────────────────────────────────────────────────────

  if (loading && !overview) {
    return (
      <div className="flex items-center justify-center py-20">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500" />
      </div>
    )
  }

  if (error && !overview) {
    return (
      <div className="p-4 bg-red-500/10 border border-red-500/30 rounded-lg text-red-400 text-sm">
        {error}
      </div>
    )
  }

  if (!overview) return null

  // Chart max for scaling
  const chartMax = Math.max(...overview.daily_active_users.map(d => d.count), 1)

  return (
    <div className="space-y-6">
      {/* Period selector */}
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-dark-text-primary">User Engagement Analytics</h2>
        <div className="flex gap-1">
          {([7, 30, 90] as Period[]).map(p => (
            <button
              key={p}
              onClick={() => setDays(p)}
              className={`px-3 py-1.5 text-xs font-medium rounded-lg transition-colors ${
                days === p
                  ? 'bg-primary-500/10 text-primary-400 border border-primary-500/30'
                  : 'text-dark-text-secondary hover:text-dark-text-primary hover:bg-dark-bg-tertiary/30'
              }`}
            >
              {p}d
            </button>
          ))}
        </div>
      </div>

      {/* Warning banner if endpoints not deployed */}
      {error && (
        <div className="p-3 bg-yellow-500/10 border border-yellow-500/30 rounded-lg text-yellow-400 text-xs">
          {error}
        </div>
      )}

      {/* Summary cards */}
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-3">
        <StatCard label="Active Users" value={overview.active_users} />
        <StatCard label="Total Logins" value={overview.total_logins} />
        <StatCard label="Tasks Created" value={overview.tasks_created} />
        <StatCard label="Wiki Edits" value={overview.wiki_edits + overview.wiki_pages_created} />
        <StatCard label="API Requests" value={overview.total_api_requests} />
        <StatCard label="Avg Session" value={formatMinutes(overview.avg_session_duration_minutes)} />
      </div>

      {/* Daily Active Users chart */}
      {overview.daily_active_users.length > 0 && (
        <div className="bg-dark-bg-secondary rounded-lg border border-dark-border-subtle p-4">
          <h3 className="text-sm font-medium text-dark-text-secondary mb-3">Daily Active Users</h3>
          <div className="flex items-end gap-[2px] h-32">
            {overview.daily_active_users.map(d => {
              const pct = (d.count / chartMax) * 100
              return (
                <div
                  key={d.date}
                  className="flex-1 group relative"
                  title={`${d.date}: ${d.count} users`}
                >
                  <div
                    className="w-full bg-primary-500/60 rounded-t hover:bg-primary-400/80 transition-colors"
                    style={{ height: `${Math.max(pct, 2)}%` }}
                  />
                  <div className="absolute bottom-full mb-1 left-1/2 -translate-x-1/2 hidden group-hover:block bg-dark-bg-tertiary text-dark-text-primary text-xs px-2 py-1 rounded whitespace-nowrap z-10">
                    {d.date}: {d.count}
                  </div>
                </div>
              )
            })}
          </div>
          <div className="flex justify-between mt-1 text-[10px] text-dark-text-tertiary">
            <span>{overview.daily_active_users[0]?.date}</span>
            <span>{overview.daily_active_users[overview.daily_active_users.length - 1]?.date}</span>
          </div>
        </div>
      )}

      {/* User filter */}
      <div className="bg-dark-bg-secondary rounded-lg border border-dark-border-subtle p-4">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-medium text-dark-text-secondary">User Engagement</h3>
          <select
            className="bg-dark-bg-tertiary text-dark-text-primary text-sm rounded-lg border border-dark-border-subtle px-3 py-1.5"
            value={selectedUserId ?? ''}
            onChange={(e) => {
              const id = e.target.value ? Number(e.target.value) : null
              if (id) loadUserDetail(id)
              else { setSelectedUserId(null); setUserDetail(null) }
            }}
          >
            <option value="">All users</option>
            {users.map(u => (
              <option key={u.id} value={u.id}>
                {u.first_name && u.last_name ? `${u.first_name} ${u.last_name}` : u.name || u.email}
              </option>
            ))}
          </select>
        </div>

        {/* User table */}
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-dark-text-tertiary text-xs border-b border-dark-border-subtle">
                <th className="text-left py-2 pr-3 font-medium">User</th>
                <th className="text-right py-2 px-2 font-medium cursor-pointer hover:text-dark-text-primary" onClick={() => handleSort('login_count')}>
                  Logins{sortArrow('login_count')}
                </th>
                <th className="text-right py-2 px-2 font-medium cursor-pointer hover:text-dark-text-primary" onClick={() => handleSort('page_view_count')}>
                  Pages{sortArrow('page_view_count')}
                </th>
                <th className="text-right py-2 px-2 font-medium cursor-pointer hover:text-dark-text-primary" onClick={() => handleSort('api_request_count')}>
                  API{sortArrow('api_request_count')}
                </th>
                <th className="text-right py-2 px-2 font-medium cursor-pointer hover:text-dark-text-primary" onClick={() => handleSort('tasks_created')}>
                  Tasks{sortArrow('tasks_created')}
                </th>
                <th className="text-right py-2 px-2 font-medium cursor-pointer hover:text-dark-text-primary" onClick={() => handleSort('total_session_minutes')}>
                  Time{sortArrow('total_session_minutes')}
                </th>
                <th className="text-right py-2 pl-2 font-medium">Last Active</th>
              </tr>
            </thead>
            <tbody>
              {sortedUsers.map(u => (
                <tr
                  key={u.user_id}
                  onClick={() => loadUserDetail(u.user_id)}
                  className={`border-b border-dark-border-subtle/50 cursor-pointer transition-colors ${
                    selectedUserId === u.user_id
                      ? 'bg-primary-500/5'
                      : 'hover:bg-dark-bg-tertiary/30'
                  }`}
                >
                  <td className="py-2 pr-3">
                    <div className="text-dark-text-primary font-medium">{u.name}</div>
                    <div className="text-dark-text-tertiary text-xs">{u.email}</div>
                  </td>
                  <td className="text-right py-2 px-2 text-dark-text-secondary">{u.login_count}</td>
                  <td className="text-right py-2 px-2 text-dark-text-secondary">{u.page_view_count}</td>
                  <td className="text-right py-2 px-2 text-dark-text-secondary">{u.api_request_count}</td>
                  <td className="text-right py-2 px-2 text-dark-text-secondary">{u.tasks_created}</td>
                  <td className="text-right py-2 px-2 text-dark-text-secondary">{formatMinutes(u.total_session_minutes)}</td>
                  <td className="text-right py-2 pl-2 text-dark-text-tertiary text-xs">
                    {u.last_active_at ? relativeTime(u.last_active_at) : '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* User detail panel */}
      {selectedUserId && (
        <div className="bg-dark-bg-secondary rounded-lg border border-primary-500/30 p-4 space-y-4">
          {detailLoading ? (
            <div className="flex items-center justify-center py-8">
              <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-primary-500" />
            </div>
          ) : userDetail ? (
            <>
              <div className="flex items-center justify-between">
                <h3 className="text-sm font-semibold text-dark-text-primary">
                  {userDetail.user.name} — Deep Dive
                </h3>
                <button
                  onClick={() => { setSelectedUserId(null); setUserDetail(null) }}
                  className="text-dark-text-tertiary hover:text-dark-text-primary text-xs"
                >
                  Close
                </button>
              </div>

              {/* Summary row */}
              <div className="grid grid-cols-3 md:grid-cols-6 gap-2 text-center">
                <MiniStat label="Logins" value={userDetail.user.login_count} />
                <MiniStat label="Page Views" value={userDetail.user.page_view_count} />
                <MiniStat label="API Requests" value={userDetail.user.api_request_count} />
                <MiniStat label="Tasks" value={userDetail.user.tasks_created} />
                <MiniStat label="Comments" value={userDetail.user.comments_added} />
                <MiniStat label="Time" value={formatMinutes(userDetail.user.total_session_minutes)} />
              </div>

              {/* Recent logins */}
              {userDetail.recent_logins.length > 0 && (
                <DetailSection title="Recent Logins">
                  <div className="space-y-1 max-h-40 overflow-y-auto">
                    {userDetail.recent_logins.map(l => (
                      <div key={l.id} className="flex items-center gap-3 py-1 px-2 rounded bg-dark-bg-primary text-xs">
                        <span className={`inline-flex px-1.5 py-0.5 rounded font-medium border ${
                          l.activity_type === 'login' ? 'text-emerald-400 bg-emerald-500/10 border-emerald-500/30' :
                          l.activity_type === 'failed_login' ? 'text-red-400 bg-red-500/10 border-red-500/30' :
                          'text-gray-400 bg-gray-500/10 border-gray-500/30'
                        }`}>
                          {l.activity_type.replace('_', ' ')}
                        </span>
                        {l.ip_address && <span className="font-mono text-dark-text-tertiary">{l.ip_address}</span>}
                        <span className="ml-auto text-dark-text-tertiary">{formatDate(l.created_at)}</span>
                      </div>
                    ))}
                  </div>
                </DetailSection>
              )}

              {/* Recent page views */}
              {userDetail.recent_page_views.length > 0 && (
                <DetailSection title="Recent Page Views">
                  <div className="space-y-1 max-h-48 overflow-y-auto">
                    {userDetail.recent_page_views.map(pv => (
                      <div key={pv.id} className="flex items-center gap-3 py-1 px-2 rounded bg-dark-bg-primary text-xs">
                        <span className="text-dark-text-primary font-mono truncate flex-1">{pv.path}</span>
                        {pv.duration_ms != null && (
                          <span className="text-dark-text-tertiary">{formatMinutes(pv.duration_ms / 60000)}</span>
                        )}
                        <span className="text-dark-text-tertiary shrink-0">{formatDate(pv.created_at)}</span>
                      </div>
                    ))}
                  </div>
                </DetailSection>
              )}

              {/* Recent project activity */}
              {userDetail.recent_activity.length > 0 && (
                <DetailSection title="Recent Activity">
                  <div className="space-y-1 max-h-48 overflow-y-auto">
                    {userDetail.recent_activity.map(a => (
                      <div key={a.id} className="flex items-center gap-3 py-1 px-2 rounded bg-dark-bg-primary text-xs">
                        <span className="inline-flex px-1.5 py-0.5 rounded font-medium text-blue-400 bg-blue-500/10 border border-blue-500/30">
                          {a.action.replace(/_/g, ' ')}
                        </span>
                        <span className="text-dark-text-secondary truncate flex-1">
                          {a.entity_title || `${a.entity_type} #${a.entity_id}`}
                        </span>
                        <span className="text-dark-text-tertiary shrink-0">{formatDate(a.created_at)}</span>
                      </div>
                    ))}
                  </div>
                </DetailSection>
              )}

              {/* API key usage */}
              {userDetail.api_keys.length > 0 && (
                <DetailSection title="API Keys">
                  <div className="space-y-2">
                    {userDetail.api_keys.map(k => (
                      <div key={k.api_key_id} className="p-2 rounded bg-dark-bg-primary border border-dark-border-subtle">
                        <div className="flex items-center justify-between mb-1">
                          <span className="text-dark-text-primary text-xs font-medium">{k.key_name}</span>
                          <span className="text-dark-text-tertiary text-xs font-mono">{k.key_prefix}...</span>
                        </div>
                        <div className="flex items-center gap-3 text-xs text-dark-text-tertiary">
                          <span>{k.request_count} requests</span>
                          {k.last_used_at && <span>Last: {relativeTime(k.last_used_at)}</span>}
                        </div>
                        {k.top_paths.length > 0 && (
                          <div className="mt-1 space-y-0.5">
                            {k.top_paths.slice(0, 5).map((p, i) => (
                              <div key={i} className="flex items-center gap-2 text-[11px]">
                                <span className="font-mono text-dark-text-tertiary w-10">{p.method}</span>
                                <span className="font-mono text-dark-text-secondary truncate flex-1">{p.path}</span>
                                <span className="text-dark-text-tertiary">{p.count}x</span>
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                </DetailSection>
              )}
            </>
          ) : (
            <p className="text-sm text-dark-text-tertiary py-4 text-center">No data available</p>
          )}
        </div>
      )}

      {/* API Key / MCP usage section */}
      {apiKeys.length > 0 && (
        <div className="bg-dark-bg-secondary rounded-lg border border-dark-border-subtle p-4">
          <h3 className="text-sm font-medium text-dark-text-secondary mb-3">API Key / MCP Usage</h3>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-dark-text-tertiary text-xs border-b border-dark-border-subtle">
                  <th className="text-left py-2 pr-3 font-medium">Key</th>
                  <th className="text-left py-2 px-2 font-medium">Owner</th>
                  <th className="text-right py-2 px-2 font-medium">Requests</th>
                  <th className="text-left py-2 px-2 font-medium">Top Paths</th>
                  <th className="text-right py-2 pl-2 font-medium">Last Used</th>
                </tr>
              </thead>
              <tbody>
                {apiKeys.map(k => (
                  <tr key={k.api_key_id} className="border-b border-dark-border-subtle/50">
                    <td className="py-2 pr-3">
                      <div className="text-dark-text-primary font-medium text-xs">{k.key_name}</div>
                      <div className="text-dark-text-tertiary text-[11px] font-mono">{k.key_prefix}...</div>
                    </td>
                    <td className="py-2 px-2 text-dark-text-secondary text-xs">{k.user_email}</td>
                    <td className="text-right py-2 px-2 text-dark-text-secondary">{k.request_count}</td>
                    <td className="py-2 px-2">
                      {k.top_paths.slice(0, 3).map((p, i) => (
                        <div key={i} className="text-[11px] font-mono text-dark-text-tertiary">
                          {p.method} {p.path} ({p.count})
                        </div>
                      ))}
                    </td>
                    <td className="text-right py-2 pl-2 text-dark-text-tertiary text-xs">
                      {k.last_used_at ? relativeTime(k.last_used_at) : '—'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Extra stats */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
        <StatCard label="Page Views" value={overview.total_page_views} />
        <StatCard label="Comments" value={overview.comments_added} />
        <StatCard label="Wiki Pages" value={overview.wiki_pages_created} />
        <StatCard label="Total API Reqs" value={overview.total_api_requests} />
      </div>
    </div>
  )
}

// ── Sub-components ──────────────────────────────────────────────────────────

function StatCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="bg-dark-bg-secondary rounded-lg border border-dark-border-subtle p-3 text-center">
      <div className="text-xl font-bold text-dark-text-primary">{value}</div>
      <div className="text-[11px] text-dark-text-tertiary mt-0.5">{label}</div>
    </div>
  )
}

function MiniStat({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="p-2 rounded bg-dark-bg-primary">
      <div className="text-sm font-bold text-dark-text-primary">{value}</div>
      <div className="text-[10px] text-dark-text-tertiary">{label}</div>
    </div>
  )
}

function DetailSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <h4 className="text-xs font-medium text-dark-text-tertiary mb-1.5">{title}</h4>
      {children}
    </div>
  )
}
