import { useCallback, useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent, type ReactNode } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, type Memory, type MemoryCompactReport, type MemoryKind, type MemoryUsage } from '../lib/api'
import { useAuth } from '../state/AuthContext'
import Button from '../components/ui/Button'
import TextInput from '../components/ui/TextInput'
import Select from '../components/ui/Select'
import FormError from '../components/ui/FormError'

const kinds: MemoryKind[] = ['fact', 'decision', 'gotcha', 'runbook', 'handoff']
const SEARCH_DEBOUNCE_MS = 200

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

const formatDate = (iso?: string) => (iso ? new Date(iso).toLocaleString() : '')

// The words the server searches for (same rule as memoryQueryWords): letters, digits and _, at most 12.
const queryWords = (q: string) => [...new Set(q.toLowerCase().match(/[\p{L}\p{N}_]+/gu) ?? [])].slice(0, 12)

const escapeRe = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

// Highlight marks every occurrence of the query words, partial words included, as search matches them.
function Highlight({ text, words }: { text: string; words: string[] }) {
  if (words.length === 0) return <>{text}</>
  const re = new RegExp(`(${[...words].sort((a, b) => b.length - a.length).map(escapeRe).join('|')})`, 'giu')
  return (
    <>
      {text.split(re).map((part, i) =>
        i % 2 === 1 ? <mark key={i} className="bg-primary-500/25 text-dark-text-primary rounded-sm">{part}</mark> : part
      )}
    </>
  )
}

// snippet shows the body around its first match, so a hit deep in a long memory is visible in the list.
function snippet(body: string, words: string[], size = 220): string {
  const lower = body.toLowerCase()
  const at = Math.min(...words.map(w => lower.indexOf(w)).filter(i => i >= 0), Infinity)
  const start = Number.isFinite(at) && at > 60 ? at - 60 : 0
  const cut = body.slice(start, start + size)
  return `${start > 0 ? '…' : ''}${cut}${start + size < body.length ? '…' : ''}`
}

const isTyping = (el: EventTarget | null) =>
  el instanceof HTMLElement && (el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName))

export default function Memories() {
  const { user } = useAuth()
  // The URL holds the applied search (q, repo, kind), so it can be shared, bookmarked and walked with back/forward.
  const [params, setParams] = useSearchParams()
  const q = (params.get('q') ?? '').trim()
  const r = (params.get('repo') ?? '').trim()
  const urlKind = params.get('kind') ?? ''
  const kind: MemoryKind | '' = kinds.includes(urlKind as MemoryKind) ? (urlKind as MemoryKind) : ''
  const [query, setQuery] = useState(q)
  const [repo, setRepo] = useState(r)
  const applied = useRef({ q, r, kind }) // the search the URL holds, as this page last wrote or was reset to
  const draft = useRef({ q, r }) // the inputs as typed, read when the debounce fires
  const timer = useRef<ReturnType<typeof setTimeout>>()
  const setParamsNow = useRef(setParams)
  setParamsNow.current = setParams // react-router 6 hands a functional update the params of the render it came from

  const [memories, setMemories] = useState<Memory[]>([])
  const [loaded, setLoaded] = useState(false)
  const [searching, setSearching] = useState(true)
  const [took, setTook] = useState<number | null>(null)
  const [searched, setSearched] = useState('') // the query the shown results answer, for highlighting
  const [selected, setActive] = useState(0)
  const [open, setOpen] = useState<Memory | null>(null)
  const [reload, setReload] = useState(0)
  const [usage, setUsage] = useState<MemoryUsage | null>(null)
  const [error, setError] = useState('') // the search's; a newer search clears it
  const [actionError, setActionError] = useState('') // a delete's or compaction's; survives the reload after it
  const [compacting, setCompacting] = useState(false)
  const [report, setReport] = useState<MemoryCompactReport | null>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  const active = Math.min(selected, Math.max(0, memories.length - 1)) // stays on a result when the list shrinks
  // Results are stale while typing waits for the debounce or a search is in flight.
  const stale = searching || query.trim() !== q || repo.trim() !== r

  // Typing applies the search after a pause, by writing it to the URL.
  const type = (field: 'q' | 'r', value: string) => {
    draft.current = { ...draft.current, [field]: value }
    if (field === 'q') setQuery(value)
    else setRepo(value)
    clearTimeout(timer.current)
    timer.current = setTimeout(() => {
      applied.current = { ...applied.current, q: draft.current.q.trim(), r: draft.current.r.trim() }
      setParamsNow.current(prev => {
        const next = new URLSearchParams(prev)
        for (const [key, value] of [['q', applied.current.q], ['repo', applied.current.r]] as const) {
          if (value) next.set(key, value)
          else next.delete(key)
        }
        return next
      }, { replace: true })
    }, SEARCH_DEBOUNCE_MS)
  }
  useEffect(() => () => clearTimeout(timer.current), [])

  // A URL change the inputs did not make (back/forward, a pasted link) drops any pending edit and shows in both inputs.
  useEffect(() => {
    const a = applied.current
    if (q !== a.q || r !== a.r || kind !== a.kind) {
      clearTimeout(timer.current)
      draft.current = { q, r }
      setQuery(q)
      setRepo(r)
    }
    applied.current = { q, r, kind }
  }, [q, r, kind])

  const chooseKind = (v: string) => {
    applied.current = { ...applied.current, kind: kinds.includes(v as MemoryKind) ? (v as MemoryKind) : '' } // keeps a pending edit
    setParams(prev => {
      const next = new URLSearchParams(prev)
      if (v) next.set('kind', v)
      else next.delete('kind')
      return next
    }, { replace: true })
  }

  // A delete's or compaction's error belongs to the search it happened in.
  useEffect(() => setActionError(''), [q, r, kind])

  useEffect(() => {
    const ctrl = new AbortController()
    const started = performance.now()
    setSearching(true)
    api.getMemories({ query: q.trim(), repo: r.trim(), kind }, ctrl.signal).then(
      ms => {
        if (ctrl.signal.aborted) return // a newer search owns the results
        setMemories(ms)
        setTook(Math.round(performance.now() - started))
        setSearched(q)
        setActive(0)
        setError('')
        setLoaded(true)
        setSearching(false)
      },
      err => {
        if (ctrl.signal.aborted) return
        setError(err instanceof Error ? err.message : 'Failed to load memories')
        setLoaded(true)
        setSearching(false)
      }
    )
    return () => ctrl.abort()
  }, [q, r, kind, reload])

  const refreshUsage = useCallback(() => {
    api.getMemoryUsage().then(setUsage, () => undefined) // a stale bar is not worth an error
  }, [])
  useEffect(refreshUsage, [refreshUsage])

  // '/' or Cmd/Ctrl+K focuses search from anywhere on the page. A pressed Cmd/Ctrl+K stops here, before the app's
  // command palette (a document listener) sees it. The header's search button dispatches its event at the document
  // itself, not at an element, and still opens the palette.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (open) return
      const cmdK = (e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k' && e.target !== document
      if (cmdK || (e.key === '/' && !isTyping(e.target))) {
        e.preventDefault()
        e.stopPropagation()
        searchRef.current?.focus()
        searchRef.current?.select()
      }
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [open])

  useEffect(() => {
    const id = memories[active]?.id
    if (id !== undefined) document.getElementById(`memory-${id}`)?.scrollIntoView?.({ block: 'nearest' })
  }, [active, memories])

  const onSearchKey = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      const step = e.key === 'ArrowDown' ? 1 : -1
      setActive(Math.max(0, Math.min(memories.length - 1, active + step)))
    } else if (e.key === 'Enter' && memories[active] && !stale) { // never open a result of the previous query
      e.preventDefault()
      setOpen(memories[active])
    }
  }

  const close = useCallback(() => {
    setOpen(null)
    searchRef.current?.focus()
  }, [])

  const remove = async (id: number) => {
    setActionError('')
    setMemories(ms => ms.filter(m => m.id !== id))
    close()
    try {
      await api.deleteMemory(id)
    } catch (err) {
      setReload(n => n + 1) // reload rather than restore a snapshot newer searches may have moved past
      setActionError(err instanceof Error ? err.message : 'Failed to delete memory')
      return
    }
    refreshUsage()
  }

  const compact = async () => {
    try {
      setCompacting(true)
      setActionError('')
      setReport(await api.compactMemories())
      setReload(n => n + 1)
      refreshUsage()
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to compact memories')
    } finally {
      setCompacting(false)
    }
  }

  const words = queryWords(searched)
  const percent = Math.min(100, usage?.percent ?? 0)
  const filtered = Boolean(q.trim() || r.trim() || kind)
  const activeId = memories[active] ? `memory-${memories[active].id}` : undefined

  return (
    <div className="max-w-4xl mx-auto px-4 sm:px-6 py-6 sm:py-8 space-y-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-dark-text-primary">Memories</h1>
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
          <div className="flex justify-between gap-3 text-xs text-dark-text-tertiary">
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
            <div className={`h-full ${usage.warning ? 'bg-warning-500' : 'bg-primary-500'}`} style={{ width: `${percent}%` }} />
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

      <div role="search" className="grid grid-cols-1 sm:grid-cols-[1fr_12rem_9rem] gap-3 items-end">
        <TextInput
          ref={searchRef}
          id="memory-query"
          label="Search"
          type="search"
          placeholder="Search memories  ( / or ⌘K )"
          autoComplete="off"
          value={query}
          onChange={e => type('q', e.target.value)}
          onKeyDown={onSearchKey}
          role="combobox"
          aria-expanded={memories.length > 0}
          aria-controls="memory-results"
          aria-activedescendant={activeId}
          aria-autocomplete="list"
        />
        <TextInput id="memory-repo" label="Repository" placeholder="owner/name" autoComplete="off" value={repo}
          onChange={e => type('r', e.target.value)} />
        <div>
          <label htmlFor="memory-kind" className="block text-xs font-medium text-dark-text-tertiary mb-1.5">Kind</label>
          <Select<string> id="memory-kind" value={kind}
            onChange={chooseKind}
            options={[{ value: '', label: 'Any kind' }, ...kinds.map(k => ({ value: k, label: k }))]} />
        </div>
      </div>

      <div className="flex items-center justify-between text-xs text-dark-text-quaternary min-h-[1rem]">
        <span role="status" aria-live="polite">
          {loaded && `${memories.length} ${memories.length === 1 ? 'result' : 'results'}${took !== null ? ` · ${took} ms` : ''}`}
        </span>
        {searching && <span>{loaded ? 'Searching…' : 'Loading memories…'}</span>}
      </div>

      <FormError message={error || actionError} />

      {loaded && memories.length === 0 && !searching && (
        <p className="text-sm text-dark-text-tertiary">
          {filtered ? 'No memories match.' : 'No memories yet. Agents save them with write_memory.'}
        </p>
      )}

      {memories.length > 0 && (
        <ul id="memory-results" role="listbox" aria-label="Memories" className={`space-y-2 transition-opacity ${searching ? 'opacity-70' : ''}`}>
          {memories.map((m, i) => (
            <li
              key={m.id}
              id={`memory-${m.id}`}
              role="option"
              aria-selected={i === active}
              onClick={() => { setActive(i); setOpen(m) }}
              onMouseMove={() => { if (i !== active) setActive(i) }}
              className={`cursor-pointer rounded-lg border p-3 sm:p-4 ${
                i === active ? 'border-primary-500/50 bg-dark-bg-tertiary' : 'border-dark-border-subtle bg-dark-bg-secondary'
              }`}
            >
              <div className="flex flex-wrap items-center gap-2 text-xs text-dark-text-tertiary">
                <span className="rounded px-1.5 py-0.5 bg-dark-bg-tertiary text-dark-text-secondary">{m.kind}</span>
                <span className="truncate">{scopeLabel(m)}</span>
                {m.confidence !== 'medium' && <span>{m.confidence} confidence</span>}
                {user && m.owner_id !== user.id && <span>shared with you</span>}
                <span className="ml-auto">{new Date(m.updated_at).toLocaleDateString()}</span>
              </div>
              <h2 className="mt-1 text-sm font-medium text-dark-text-primary"><Highlight text={m.title} words={words} /></h2>
              <p className="mt-1 text-sm text-dark-text-secondary break-words line-clamp-2">
                <Highlight text={snippet(m.body, words)} words={words} />
              </p>
            </li>
          ))}
        </ul>
      )}

      {open && (
        <MemoryDetail memory={open} words={words} canDelete={Boolean(user && open.owner_id === user.id)}
          onClose={close} onDelete={() => void remove(open.id)} />
      )}
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  if (children === '' || children === undefined || children === null) return null
  return (
    <div className="min-w-0">
      <dt className="text-dark-text-quaternary">{label}</dt>
      <dd className="text-dark-text-secondary break-words">{children}</dd>
    </div>
  )
}

function MemoryDetail({ memory: m, words, canDelete, onClose, onDelete }: {
  memory: Memory; words: string[]; canDelete: boolean; onClose: () => void; onDelete: () => void
}) {
  const [confirming, setConfirming] = useState(false)
  const dialogRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    dialogRef.current?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
      if (e.key !== 'Tab' || !dialogRef.current) return
      // Keep Tab inside the dialog: wrap from the last control to the first and back.
      const items = [...dialogRef.current.querySelectorAll<HTMLElement>('button:not([disabled]), a[href]')]
      if (items.length === 0) return
      const [first, last] = [items[0], items[items.length - 1]]
      const inside = dialogRef.current.contains(document.activeElement)
      if (e.shiftKey && (!inside || document.activeElement === first || document.activeElement === dialogRef.current)) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && (!inside || document.activeElement === last)) {
        e.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <div className="fixed inset-0 z-50 flex items-stretch sm:items-center justify-center bg-black/60 sm:p-6" onClick={onClose}>
      <div
        ref={dialogRef}
        tabIndex={-1}
        role="dialog"
        aria-modal="true"
        aria-labelledby="memory-detail-title"
        onClick={e => e.stopPropagation()}
        className="w-full sm:max-w-2xl max-h-full overflow-y-auto bg-dark-bg-primary sm:rounded-xl border border-dark-border-subtle p-5 space-y-4 focus:outline-none"
      >
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="text-xs text-dark-text-tertiary">{m.kind} · {m.status} · {m.confidence} confidence</p>
            <h2 id="memory-detail-title" className="mt-1 text-base font-semibold text-dark-text-primary break-words">
              <Highlight text={m.title} words={words} />
            </h2>
          </div>
          <Button variant="ghost" size="sm" onClick={onClose} aria-label="Close memory">Close</Button>
        </div>
        <p className="text-sm text-dark-text-secondary whitespace-pre-wrap break-words"><Highlight text={m.body} words={words} /></p>
        <dl className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-xs">
          <Field label="Repository">{m.scope.repo}</Field>
          <Field label="Issue">{m.scope.issue ? `#${m.scope.issue}` : ''}</Field>
          <Field label="Pull request">{m.scope.pr ? `#${m.scope.pr}` : ''}</Field>
          <Field label="Folder">{m.scope.folder}</Field>
          <Field label="Files">{m.scope.files?.join(', ')}</Field>
          <Field label="Agent">{[m.author.agent, m.author.provider].filter(Boolean).join(' · ')}</Field>
          <Field label="Host">{m.author.host}</Field>
          <Field label="Session">{m.author.session}</Field>
          <Field label="Created">{formatDate(m.created_at)}</Field>
          <Field label="Updated">{formatDate(m.updated_at)}</Field>
          <Field label="Expires">{formatDate(m.expires_at)}</Field>
        </dl>
        {canDelete && (
          <div className="flex justify-end gap-2 pt-2 border-t border-dark-border-subtle">
            {confirming ? (
              <>
                <Button variant="danger" size="sm" onClick={onDelete}>Delete</Button>
                <Button variant="ghost" size="sm" onClick={() => setConfirming(false)}>Cancel</Button>
              </>
            ) : (
              <Button variant="ghost" size="sm" onClick={() => setConfirming(true)}>Delete memory</Button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
