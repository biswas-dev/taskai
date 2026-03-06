import { useState, useRef, useEffect, useCallback, forwardRef, TextareaHTMLAttributes } from 'react'
import { apiClient, ProjectMember } from '../lib/api'

interface MentionTextareaProps extends Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, 'value' | 'onChange'> {
  value: string
  onChange: (value: string) => void
  projectId: number
}

const MentionTextarea = forwardRef<HTMLTextAreaElement, MentionTextareaProps>(function MentionTextarea(
  { value, onChange, projectId, ...rest },
  forwardedRef,
) {
  const internalRef = useRef<HTMLTextAreaElement>(null)
  const [members, setMembers] = useState<ProjectMember[]>([])
  const [showMenu, setShowMenu] = useState(false)
  const [menuFilter, setMenuFilter] = useState('')
  const [menuIndex, setMenuIndex] = useState(0)
  const [atPos, setAtPos] = useState(-1)
  const menuRef = useRef<HTMLDivElement>(null)

  // Resolve the actual textarea element via whichever ref is available
  const getTA = (): HTMLTextAreaElement | null => {
    if (forwardedRef && typeof forwardedRef === 'object') return forwardedRef.current
    return internalRef.current
  }

  useEffect(() => {
    apiClient.getProjectMembers(projectId).then(setMembers).catch(() => {})
  }, [projectId])

  const filteredMembers = members.filter(m => {
    const name = (m.user_name ?? m.name ?? m.email ?? '').toLowerCase()
    return name.includes(menuFilter.toLowerCase())
  }).slice(0, 6)

  const handleChange = useCallback((e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const val = e.target.value
    onChange(val)
    const pos = e.target.selectionStart ?? val.length
    const before = val.slice(0, pos)
    const match = before.match(/@([\w.-]*)$/)
    if (match) {
      setAtPos(pos - match[0].length)
      setMenuFilter(match[1])
      setMenuIndex(0)
      setShowMenu(true)
    } else {
      setShowMenu(false)
    }
  }, [onChange])

  const insertMention = useCallback((member: ProjectMember) => {
    if (atPos < 0) return
    const username = member.user_name ?? member.name ?? member.email ?? ''
    const ta = getTA()
    const before = value.slice(0, atPos)
    const after = value.slice(ta?.selectionStart ?? value.length)
    onChange(before + '@' + username + ' ' + after)
    setShowMenu(false)
    setTimeout(() => {
      const t = getTA()
      if (t) {
        t.focus()
        const cp = atPos + username.length + 2
        t.setSelectionRange(cp, cp)
      }
    }, 0)
  }, [atPos, value, onChange]) // eslint-disable-line react-hooks/exhaustive-deps

  const handleKeyDown = useCallback((e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (!showMenu || filteredMembers.length === 0) return
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setMenuIndex(i => Math.min(i + 1, filteredMembers.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setMenuIndex(i => Math.max(i - 1, 0))
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault()
      insertMention(filteredMembers[menuIndex])
    } else if (e.key === 'Escape') {
      setShowMenu(false)
    }
  }, [showMenu, filteredMembers, menuIndex, insertMention])

  // Close on outside click
  useEffect(() => {
    if (!showMenu) return
    const handler = (e: MouseEvent) => {
      const ta = getTA()
      if (menuRef.current && !menuRef.current.contains(e.target as Node) &&
          ta && !ta.contains(e.target as Node)) {
        setShowMenu(false)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [showMenu]) // eslint-disable-line react-hooks/exhaustive-deps

  const displayName = (m: ProjectMember) => m.user_name ?? m.name ?? m.email ?? ''

  return (
    <div className="relative">
      <textarea
        ref={forwardedRef ?? internalRef}
        value={value}
        onChange={handleChange}
        onKeyDown={handleKeyDown}
        {...rest}
      />
      {showMenu && filteredMembers.length > 0 && (
        <div
          ref={menuRef}
          className="absolute left-0 bottom-full mb-1 w-56 bg-dark-bg-elevated border border-dark-border-subtle rounded-lg shadow-xl z-50 overflow-hidden"
          role="listbox"
          aria-label="Mention suggestions"
        >
          {filteredMembers.map((m, i) => (
            <button
              key={m.id}
              type="button"
              role="option"
              aria-selected={i === menuIndex}
              onClick={() => insertMention(m)}
              onMouseEnter={() => setMenuIndex(i)}
              className={`w-full text-left px-3 py-2 text-sm transition-colors flex items-center gap-2 ${
                i === menuIndex
                  ? 'bg-primary-500/20 text-primary-300'
                  : 'text-dark-text-secondary hover:bg-dark-bg-tertiary'
              }`}
            >
              <span className="w-6 h-6 rounded-full bg-primary-500/20 flex items-center justify-center text-xs font-bold text-primary-400 shrink-0">
                {displayName(m).charAt(0).toUpperCase()}
              </span>
              <span className="truncate">{displayName(m)}</span>
              {m.role && (
                <span className="text-[10px] text-dark-text-tertiary ml-auto shrink-0 capitalize">
                  {m.role}
                </span>
              )}
            </button>
          ))}
        </div>
      )}
    </div>
  )
})

export default MentionTextarea
