import { useState, useMemo } from 'react'
import { Combobox } from '@headlessui/react'

export interface SearchSelectOption {
  value: string
  label: string
  description?: string
}

interface SearchSelectProps {
  value: string
  onChange: (value: string) => void
  options: SearchSelectOption[]
  placeholder?: string
  disabled?: boolean
  variant?: 'inline' | 'form'
}

export default function SearchSelect({
  value,
  onChange,
  options,
  placeholder = 'Select...',
  disabled = false,
  variant = 'form',
}: Readonly<SearchSelectProps>) {
  const [query, setQuery] = useState('')

  const filtered = useMemo(() => {
    if (!query) return options
    const q = query.toLowerCase()
    return options.filter(
      (o) => o.label.toLowerCase().includes(q) || o.description?.toLowerCase().includes(q)
    )
  }, [query, options])

  const selected = options.find((o) => o.value === value)

  const isInline = variant === 'inline'

  return (
    <Combobox
      value={value}
      onChange={(v: string | null) => {
        if (v !== null) onChange(v)
        setQuery('')
      }}
      disabled={disabled}
    >
      <div className="relative">
        <div className="relative">
          <Combobox.Input
            className={
              isInline
                ? 'w-full bg-transparent cursor-pointer text-sm text-dark-text-primary hover:bg-dark-bg-tertiary/50 pl-3 pr-7 py-1.5 rounded-md border border-transparent hover:border-dark-border-subtle focus:border-primary-500 focus:ring-1 focus:ring-primary-500/30 outline-none transition-colors disabled:opacity-50 disabled:cursor-not-allowed'
                : 'w-full px-3 py-2 bg-dark-bg-secondary border border-dark-border-subtle text-dark-text-primary rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 outline-none transition-colors disabled:opacity-50 disabled:cursor-not-allowed'
            }
            displayValue={() => selected?.label ?? ''}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={placeholder}
          />
          <Combobox.Button className="absolute inset-y-0 right-0 flex items-center pr-2">
            <svg
              className={`${isInline ? 'w-3.5 h-3.5' : 'w-4 h-4'} text-dark-text-tertiary`}
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
            </svg>
          </Combobox.Button>
        </div>

        <Combobox.Options className="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg bg-dark-bg-elevated border border-dark-border-subtle shadow-lg py-1 text-sm focus:outline-none">
          {filtered.length === 0 ? (
            <div className="px-3 py-2 text-dark-text-tertiary">No results</div>
          ) : (
            filtered.map((opt) => (
              <Combobox.Option
                key={opt.value}
                value={opt.value}
                className={({ active }: { active: boolean }) =>
                  `cursor-pointer select-none px-3 py-1.5 ${
                    active ? 'bg-dark-bg-tertiary text-dark-text-primary' : 'text-dark-text-secondary'
                  }`
                }
              >
                {({ selected: isSelected }) => (
                  <div className="flex items-center justify-between">
                    <div className="min-w-0">
                      <span className={`block truncate ${isSelected ? 'font-medium text-dark-text-primary' : ''}`}>
                        {opt.label}
                      </span>
                      {opt.description && (
                        <span className="block truncate text-xs text-dark-text-quaternary">
                          {opt.description}
                        </span>
                      )}
                    </div>
                    {isSelected && (
                      <svg className="w-4 h-4 text-primary-400 flex-shrink-0 ml-2" fill="currentColor" viewBox="0 0 20 20">
                        <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
                      </svg>
                    )}
                  </div>
                )}
              </Combobox.Option>
            ))
          )}
        </Combobox.Options>
      </div>
    </Combobox>
  )
}
