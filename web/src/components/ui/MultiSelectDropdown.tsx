import { useState, useRef, useEffect, useMemo } from 'react'

export interface MultiSelectOption {
  value: string
  label: string
  description?: string
  color?: string
}

interface MultiSelectDropdownProps {
  values: string[]
  onChange: (values: string[]) => void
  options: MultiSelectOption[]
  title?: string
  placeholder?: string
  filterPlaceholder?: string
  disabled?: boolean
}

export default function MultiSelectDropdown({
  values,
  onChange,
  options,
  title = 'Select items',
  placeholder = 'None',
  filterPlaceholder = 'Filter…',
  disabled = false,
}: Readonly<MultiSelectDropdownProps>) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const ref = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  const selectedSet = useMemo(() => new Set(values), [values])

  const filtered = useMemo(() => {
    if (!query) return options
    const q = query.toLowerCase()
    return options.filter(
      (o) => o.label.toLowerCase().includes(q) || o.description?.toLowerCase().includes(q)
    )
  }, [query, options])

  const selected = filtered.filter((o) => selectedSet.has(o.value))
  const unselected = filtered.filter((o) => !selectedSet.has(o.value))

  // All selected options (unfiltered) for display in trigger
  const selectedOptions = options.filter((o) => selectedSet.has(o.value))

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
        setQuery('')
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  useEffect(() => {
    if (open) {
      setTimeout(() => inputRef.current?.focus(), 50)
    }
  }, [open])

  const toggle = (value: string) => {
    if (selectedSet.has(value)) {
      onChange(values.filter((v) => v !== value))
    } else {
      onChange([...values, value])
    }
  }

  return (
    <div ref={ref} className="relative w-full">
      {/* Trigger */}
      <button
        type="button"
        disabled={disabled}
        onClick={() => { if (!disabled) setOpen((v) => !v) }}
        className="w-full flex items-center gap-1.5 px-3 py-1.5 rounded-md text-left text-sm hover:bg-dark-bg-tertiary/50 transition-colors focus:outline-none disabled:opacity-50 disabled:cursor-not-allowed min-h-[32px]"
      >
        {selectedOptions.length === 0 ? (
          <span className="text-dark-text-tertiary">{placeholder}</span>
        ) : (
          <div className="flex flex-wrap gap-1">
            {selectedOptions.map((o) => (
              <span
                key={o.value}
                className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-xs font-medium border border-dark-border-subtle"
                style={
                  o.color
                    ? { backgroundColor: o.color + '22', color: o.color, borderColor: o.color + '44' }
                    : undefined
                }
              >
                {o.color && <span className="w-1.5 h-1.5 rounded-full flex-shrink-0" style={{ backgroundColor: o.color }} />}
                {o.label}
              </span>
            ))}
          </div>
        )}
      </button>

      {/* Dropdown */}
      {open && (
        <div className="absolute top-full left-0 right-0 mt-1 bg-dark-bg-elevated border border-dark-border-subtle rounded-xl shadow-2xl z-50 overflow-hidden" style={{ minWidth: '220px' }}>
          {/* Header */}
          <div className="px-3 pt-3 pb-2 border-b border-dark-border-subtle">
            <p className="text-xs font-semibold text-dark-text-primary mb-2">{title}</p>
            {/* Search */}
            <div className="flex items-center gap-2 px-2.5 py-1.5 bg-dark-bg-secondary border border-dark-border-subtle rounded-lg focus-within:border-primary-500 focus-within:ring-1 focus-within:ring-primary-500/30 transition-colors">
              <svg className="w-3.5 h-3.5 text-dark-text-tertiary flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
              </svg>
              <input
                ref={inputRef}
                type="text"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={filterPlaceholder}
                className="flex-1 text-sm bg-transparent text-dark-text-primary placeholder-dark-text-tertiary outline-none"
              />
              {query && (
                <button type="button" onClick={() => setQuery('')} className="text-dark-text-tertiary hover:text-dark-text-secondary transition-colors">
                  <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              )}
            </div>
          </div>

          <div className="max-h-56 overflow-y-auto">
            {/* Selected group */}
            {selected.length > 0 && (
              <>
                <div className="px-3 pt-2.5 pb-1">
                  <span className="text-[10px] font-semibold text-dark-text-tertiary uppercase tracking-wider">Selected</span>
                </div>
                {selected.map((opt) => (
                  <OptionRow key={opt.value} opt={opt} checked={true} onToggle={toggle} />
                ))}
              </>
            )}

            {/* Unselected group */}
            {unselected.length > 0 && (
              <>
                {selected.length > 0 && (
                  <div className="px-3 pt-2.5 pb-1">
                    <span className="text-[10px] font-semibold text-dark-text-tertiary uppercase tracking-wider">Available</span>
                  </div>
                )}
                {unselected.map((opt) => (
                  <OptionRow key={opt.value} opt={opt} checked={false} onToggle={toggle} />
                ))}
              </>
            )}

            {filtered.length === 0 && (
              <div className="px-3 py-4 text-sm text-dark-text-tertiary text-center">No results</div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function OptionRow({
  opt,
  checked,
  onToggle,
}: {
  opt: MultiSelectOption
  checked: boolean
  onToggle: (v: string) => void
}) {
  return (
    <button
      type="button"
      onClick={() => onToggle(opt.value)}
      className={`w-full flex items-center gap-2.5 px-3 py-2 text-left transition-colors hover:bg-dark-bg-tertiary/60 ${
        checked ? 'bg-primary-500/8' : ''
      }`}
    >
      {/* Checkbox */}
      <span
        className={`w-4 h-4 rounded flex-shrink-0 flex items-center justify-center border transition-colors ${
          checked
            ? 'bg-primary-500 border-primary-500'
            : 'bg-transparent border-dark-border-default hover:border-dark-border-strong'
        }`}
      >
        {checked && (
          <svg className="w-2.5 h-2.5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
          </svg>
        )}
      </span>

      {/* Color dot */}
      {opt.color && (
        <span className="w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: opt.color }} />
      )}

      {/* Text */}
      <div className="flex-1 min-w-0">
        <span className={`block text-sm truncate ${checked ? 'text-dark-text-primary font-medium' : 'text-dark-text-secondary'}`}>
          {opt.label}
        </span>
        {opt.description && (
          <span className="block text-xs text-dark-text-tertiary truncate">{opt.description}</span>
        )}
      </div>
    </button>
  )
}
