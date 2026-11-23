import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { api, type Memory, type MemoryCompactReport, type MemoryFilter, type MemoryKind, type MemoryUsage } from '../lib/api'
import { useAuth } from '../state/AuthContext'
import Button from '../components/ui/Button'
import TextInput from '../components/ui/TextInput'
import Select from '../components/ui/Select'
import FormError from '../components/ui/FormError'

const kinds: MemoryKind[] = ['fact', 'decision', 'gotcha', 'runbook', 'handoff']

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let v = n / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v < 10 ? 1 : 0)} ${units[i]}`
}

const scopeLabel = (m: Memory) => {
  const { repo, issue, pr, folder } = m.scope
  if (repo && issue) return `${repo}#${issue}`
  if (repo && pr) return `${repo} PR ${pr}`
  return repo || folder || ''
}

export default function Memories() {
  const { user } = useAuth()
  const [filter, setFilter] = useState<MemoryFilter>({ query: '', repo: '', kind: '' })
  const [applied, setApplied] = useState<MemoryFilter>(filter)
  const [memories, setMemories] = useState<Memory[]>([])
  const [usage, setUsage] = useState<MemoryUsage | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [confirmId, setConfirmId] = useState<number | null>(null)
  const [compacting, setCompacting] = useState(false)
  const [report, setReport] = useState<MemoryCompactReport | null>(null)
  const latest = useRef(0) // only the newest request may set results

  const load = useCallback(async () => {
    const request = ++latest.current
    try {
      setLoading(true)
      setError('')
      const [ms, u] = await Promise.all([api.getMemories(applied), api.getMemoryUsage()])
      if (request !== latest.current) return
      setMemories(ms)
      setUsage(u)
    } catch (err) {
      if (request === latest.current) setError(err instanceof Error ? err.message : 'Failed to load memories')
    } finally {
      if (request === latest.current) setLoading(false)
    }
  }, [applied])

  useEffect(() => {
    void load()
  }, [load])

  const search = (e: FormEvent) => {
    e.preventDefault()
    setApplied({ ...filter })
  }

  const remove = async (id: number) => {
    const before = memories
    setMemories(ms => ms.filter(m => m.id !== id))
    setConfirmId(null)
    try {
      await api.deleteMemory(id)
    } catch (err) {
      setMemories(before)
      setError(err instanceof Error ? err.message : 'Failed to delete memory')
      return
    }
    // A failed usage refresh leaves the bar stale; the delete itself succeeded.
    api.getMemoryUsage().then(setUsage, () => undefined)
  }

  const compact = async () => {
    try {
      setCompacting(true)
      setError('')
      setReport(await api.compactMemories())
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to compact memories')
    } finally {
      setCompacting(false)
    }
  }

  const percent = Math.min(100, usage?.percent ?? 0)

  return (
    <div className="max-w-4xl mx-auto px-6 py-8 space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-dark-text-primary">Agent memories</h1>
          <p className="text-sm text-dark-text-tertiary mt-1">
            Lessons your agents and you keep, searchable by every agent with your API keys and by people you share them with.
          </p>
        </div>
        <Button variant="secondary" size="sm" onClick={() => void compact()} loading={compacting}>
          Compact
        </Button>
      </div>

      {usage && (
        <section aria-label="Storage" className="space-y-1.5">
          <div className="flex justify-between text-xs text-dark-text-tertiary">
            <span>{usage.active_count} active of {usage.count} memories</span>
            <span>{formatBytes(usage.used_bytes)} of {formatBytes(usage.quota_bytes)} ({usage.percent.toFixed(1)}%)</span>
          </div>
          <div
            role="progressbar"
            aria-label="Memory storage used"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={Math.round(percent)}
            className="h-2 rounded-full bg-dark-bg-tertiary overflow-hidden"
          >
            <div
              className={`h-full ${usage.warning ? 'bg-warning-500' : 'bg-primary-500'}`}
              style={{ width: `${percent}%` }}
            />
          </div>
          {usage.warning && <p className="text-xs text-warning-400" role="status">{usage.warning}</p>}
        </section>
      )}

      {report && (
        <p className="text-sm text-dark-text-secondary" role="status">
          Compaction freed {formatBytes(report.freed_bytes)}: {report.superseded_removed} superseded, {report.expired_removed} expired,{' '}
          {report.duplicates_merged} duplicates and {report.low_confidence_summarised} low-confidence notes merged.
        </p>
      )}

      <form onSubmit={search} className="grid grid-cols-1 sm:grid-cols-[1fr_12rem_9rem_auto] gap-3 items-end">
        <TextInput id="memory-query" label="Search" placeholder="What do you want to know?" value={filter.query}
          onChange={e => setFilter({ ...filter, query: e.target.value })} />
        <TextInput id="memory-repo" label="Repository" placeholder="owner/name" value={filter.repo}
          onChange={e => setFilter({ ...filter, repo: e.target.value })} />
        <div>
          <label htmlFor="memory-kind" className="block text-xs font-medium text-dark-text-tertiary mb-1.5">Kind</label>
          <Select<string> id="memory-kind" value={filter.kind ?? ''} onChange={v => setFilter({ ...filter, kind: v as MemoryKind | '' })}
            options={[{ value: '', label: 'Any kind' }, ...kinds.map(k => ({ value: k, label: k }))]} />
        </div>
        <Button type="submit">Search</Button>
      </form>

      <FormError message={error} />

      {loading ? (
        <p className="text-sm text-dark-text-tertiary" role="status">Loading memories…</p>
      ) : memories.length === 0 ? (
        <p className="text-sm text-dark-text-tertiary">
          {applied.query || applied.repo || applied.kind ? 'No memories match.' : 'No memories yet. Agents save them with write_memory.'}
        </p>
      ) : (
        <ul className="space-y-3">
          {memories.map(m => (
            <li key={m.id} className="rounded-lg border border-dark-border-subtle bg-dark-bg-secondary p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2 text-xs text-dark-text-tertiary">
                    <span className="rounded px-1.5 py-0.5 bg-dark-bg-tertiary text-dark-text-secondary">{m.kind}</span>
                    <span>{scopeLabel(m)}</span>
                    {m.confidence !== 'medium' && <span>{m.confidence} confidence</span>}
                    {user && m.owner_id !== user.id && <span>shared with you</span>}
                  </div>
                  <h2 className="mt-1 text-sm font-medium text-dark-text-primary">{m.title}</h2>
                </div>
                {user && m.owner_id === user.id && (
                  confirmId === m.id ? (
                    <div className="flex gap-2 shrink-0">
                      <Button variant="danger" size="sm" onClick={() => void remove(m.id)}>Delete</Button>
                      <Button variant="ghost" size="sm" onClick={() => setConfirmId(null)}>Cancel</Button>
                    </div>
                  ) : (
                    <Button variant="ghost" size="sm" aria-label={`Delete memory ${m.title}`} onClick={() => setConfirmId(m.id)}>
                      Delete
                    </Button>
                  )
                )}
              </div>
              <p className="mt-2 text-sm text-dark-text-secondary whitespace-pre-wrap break-words">{m.body}</p>
              <p className="mt-2 text-xs text-dark-text-quaternary">
                {[m.author.agent, m.author.host].filter(Boolean).join(' on ')} · {new Date(m.updated_at).toLocaleDateString()}
                {m.scope.files?.length ? ` · ${m.scope.files.join(', ')}` : ''}
              </p>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
