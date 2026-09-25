import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import { Popover, PopoverButton, PopoverPanel } from '@headlessui/react'

export type SelectValue = string | number

export interface SelectOption<T extends SelectValue = string> {
  value: T
  label: string
  /** Secondary line under the label (also matched by the search box). */
  description?: string
  /** Small badge on the right, e.g. "Current". */
  hint?: string
  disabled?: boolean
  /** Tooltip, e.g. why an option is disabled. */
  title?: string
  /** Nesting level, for tree-shaped lists (e.g. wiki pages). */
  indent?: number
}

type Size = 'sm' | 'md'
type Variant = 'form' | 'inline' | 'ghost'

export interface SelectProps<T extends SelectValue> {
  value: T | null | undefined
  onChange: (value: T) => void
  options: SelectOption<T>[]
  /** Shown on the trigger when no option matches `value`. */
  placeholder?: string
  disabled?: boolean
  /** Lets a `<label htmlFor>` name the trigger. */
  id?: string
  'aria-label'?: string
  /**
   * Shows a filter box above the options. `'auto'` (the default) turns it on
   * for lists long enough that scanning them by eye gets slow.
   */
  searchable?: boolean | 'auto'
  searchPlaceholder?: string
  size?: Size
  /** `form` = bordered field, `inline` = borderless until hover, `ghost` = text only. */
  variant?: Variant
  /** Classes for the wrapper, e.g. `w-full` or `flex-1`. */
  className?: string
  /** Classes appended to the trigger button. */
  buttonClassName?: string
  /** Overrides how the chosen option is shown on the trigger. */
  renderValue?: (option: SelectOption<T> | undefined) => ReactNode
  emptyText?: string
}

// Static class names so Tailwind keeps them (wiki trees nest up to 6 levels).
const INDENT_CLASSES = ['', 'w-3', 'w-6', 'w-9', 'w-12', 'w-16', 'w-20']

/** Lists at least this long get a search box when `searchable` is `'auto'`. */
export const AUTO_SEARCH_THRESHOLD = 8

const triggerBase =
  'group inline-flex w-full items-center justify-between gap-2 text-left outline-none transition-colors disabled:cursor-not-allowed disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-primary-500/60 data-[open]:ring-2 data-[open]:ring-primary-500/40'

const triggerVariant: Record<Variant, string> = {
  form: 'rounded-lg border border-dark-border-subtle bg-dark-bg-primary text-dark-text-primary hover:border-dark-border-strong',
  inline: 'rounded-md border border-transparent bg-transparent text-dark-text-primary hover:border-dark-border-subtle hover:bg-dark-bg-tertiary/50',
  ghost: 'rounded-md bg-transparent text-dark-text-primary',
}

const triggerSize: Record<Size, string> = {
  sm: 'px-2 py-1 text-xs',
  md: 'px-3 py-2 text-sm',
}

function nextEnabled<T extends SelectValue>(list: SelectOption<T>[], from: number, step: 1 | -1): number {
  if (list.length === 0) return -1
  for (let i = 1; i <= list.length; i++) {
    const idx = (from + step * i + list.length * 2) % list.length
    if (!list[idx].disabled) return idx
  }
  return -1
}

function filterOptions<T extends SelectValue>(list: SelectOption<T>[], query: string): SelectOption<T>[] {
  const q = query.trim().toLowerCase()
  if (!q) return list
  return list.filter((o) => o.label.toLowerCase().includes(q) || o.description?.toLowerCase().includes(q))
}

function firstEnabled<T extends SelectValue>(list: SelectOption<T>[], fromEnd = false): number {
  const order = fromEnd ? [...list.keys()].reverse() : [...list.keys()]
  return order.find((i) => !list[i].disabled) ?? -1
}

/**
 * The app's dropdown. Replaces native `<select>` so menus match the design
 * system in dark and light themes. Follows the WAI-ARIA listbox pattern:
 * arrow keys, Home/End, Enter/Space, Escape, and type-ahead (or a search box
 * for long lists).
 */
export default function Select<T extends SelectValue>(props: Readonly<SelectProps<T>>) {
  const {
    value,
    options,
    placeholder = 'Select…',
    disabled = false,
    id,
    size = 'md',
    variant = 'form',
    className = '',
    buttonClassName = '',
    renderValue,
  } = props

  const selected = options.find((o) => o.value === value)

  return (
    <Popover className={`relative ${className}`}>
      {({ open }) => (
        <>
          <PopoverButton
            id={id}
            disabled={disabled}
            aria-haspopup="listbox"
            aria-label={props['aria-label']}
            className={`${triggerBase} ${triggerVariant[variant]} ${triggerSize[size]} ${buttonClassName}`}
            onKeyDown={(e: KeyboardEvent<HTMLButtonElement>) => {
              if (!open && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
                e.preventDefault()
                e.currentTarget.click()
              }
            }}
          >
            <span className={`min-w-0 truncate ${selected ? '' : 'text-dark-text-tertiary'}`}>
              {renderValue ? renderValue(selected) : (selected?.label ?? placeholder)}
            </span>
            <svg
              aria-hidden="true"
              className={`${size === 'sm' ? 'h-3.5 w-3.5' : 'h-4 w-4'} flex-shrink-0 text-dark-text-tertiary transition-transform group-data-[open]:rotate-180`}
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
            </svg>
          </PopoverButton>
          <PopoverPanel
            anchor={{ to: 'bottom start', gap: 4, padding: 8 }}
            className="z-[60] min-w-[var(--button-width)] max-w-[min(28rem,calc(100vw-1rem))] overflow-hidden rounded-lg border border-dark-border-medium bg-dark-bg-elevated shadow-xl shadow-black/20 focus:outline-none"
          >
            {({ close }) => <SelectPanel {...props} selected={selected} onDone={() => close()} />}
          </PopoverPanel>
        </>
      )}
    </Popover>
  )
}

interface PanelProps<T extends SelectValue> extends SelectProps<T> {
  selected: SelectOption<T> | undefined
  onDone: () => void
}

function SelectPanel<T extends SelectValue>({
  options,
  onChange,
  selected,
  onDone,
  searchable = 'auto',
  searchPlaceholder = 'Search…',
  emptyText = 'No matches',
  size = 'md',
  'aria-label': ariaLabel,
  id,
}: Readonly<PanelProps<T>>) {
  const baseId = useId()
  const listId = `${baseId}-list`
  const optionId = (i: number) => `${baseId}-opt-${i}`

  const showSearch = searchable === 'auto' ? options.length >= AUTO_SEARCH_THRESHOLD : searchable
  const [query, setQuery] = useState('')

  const visible = useMemo(() => filterOptions(options, query), [options, query])

  const [active, setActive] = useState(() => {
    const idx = options.findIndex((o) => o === selected)
    return idx >= 0 && !options[idx].disabled ? idx : firstEnabled(options)
  })

  const searchRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLUListElement>(null)
  const typeahead = useRef({ text: '', at: 0 })

  // Move focus into the panel so the keyboard drives the list straight away.
  useEffect(() => {
    if (showSearch) searchRef.current?.focus()
    else listRef.current?.focus()
  }, [showSearch])

  // Keep the active option on screen while arrowing through a long list.
  useEffect(() => {
    if (active < 0) return
    document.getElementById(optionId(active))?.scrollIntoView?.({ block: 'nearest' })
    // optionId is derived from a stable id; re-run only when the index moves.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active])

  const choose = (opt: SelectOption<T> | undefined) => {
    if (!opt || opt.disabled) return
    // Like a native select, only report a change when the value actually changes.
    if (opt.value !== selected?.value) onChange(opt.value)
    onDone()
  }

  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        setActive((i) => nextEnabled(visible, i, 1))
        return
      case 'ArrowUp':
        e.preventDefault()
        setActive((i) => nextEnabled(visible, i < 0 ? 0 : i, -1))
        return
      case 'Home':
        if (showSearch) return
        e.preventDefault()
        setActive(firstEnabled(visible))
        return
      case 'End':
        if (showSearch) return
        e.preventDefault()
        setActive(firstEnabled(visible, true))
        return
      case 'Enter':
        e.preventDefault()
        choose(visible[active])
        return
      case ' ':
        if (showSearch) return
        e.preventDefault()
        choose(visible[active])
        return
      case 'Tab':
        onDone()
        return
    }
    // Type-ahead: jump to the first option starting with what was typed.
    if (!showSearch && e.key.length === 1 && !e.metaKey && !e.ctrlKey && !e.altKey) {
      const now = Date.now()
      const buf = now - typeahead.current.at > 700 ? e.key : typeahead.current.text + e.key
      typeahead.current = { text: buf, at: now }
      const hit = visible.findIndex((o) => !o.disabled && o.label.toLowerCase().startsWith(buf.toLowerCase()))
      if (hit >= 0) setActive(hit)
    }
  }

  const optionPad = size === 'sm' ? 'px-2.5 py-1.5 text-xs' : 'px-3 py-2 text-sm'

  return (
    <div className="flex max-h-[min(20rem,var(--anchor-max-height,20rem))] flex-col">
      {showSearch && (
        <div className="border-b border-dark-border-subtle p-1.5">
          <input
            ref={searchRef}
            type="text"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
              setActive(firstEnabled(filterOptions(options, e.target.value)))
            }}
            onKeyDown={onKeyDown}
            placeholder={searchPlaceholder}
            aria-label={searchPlaceholder}
            aria-controls={listId}
            aria-activedescendant={active >= 0 ? optionId(active) : undefined}
            role="combobox"
            aria-expanded="true"
            aria-autocomplete="list"
            className="w-full rounded-md border border-dark-border-subtle bg-dark-bg-primary px-2.5 py-1.5 text-sm text-dark-text-primary placeholder:text-dark-text-quaternary outline-none focus:border-primary-500 focus:ring-1 focus:ring-primary-500/40"
          />
        </div>
      )}
      <ul
        ref={listRef}
        id={listId}
        role="listbox"
        tabIndex={showSearch ? -1 : 0}
        aria-label={ariaLabel}
        aria-labelledby={ariaLabel || !id ? undefined : id}
        aria-activedescendant={!showSearch && active >= 0 ? optionId(active) : undefined}
        onKeyDown={showSearch ? undefined : onKeyDown}
        className="min-h-0 flex-1 overflow-y-auto py-1 outline-none"
      >
        {visible.length === 0 && (
          <li role="presentation" className={`${optionPad} text-dark-text-tertiary`}>{emptyText}</li>
        )}
        {visible.map((opt, i) => {
          const isSelected = opt.value === selected?.value
          const isActive = i === active
          return (
            <li
              key={String(opt.value)}
              id={optionId(i)}
              role="option"
              aria-selected={isSelected}
              aria-disabled={opt.disabled || undefined}
              title={opt.title}
              onMouseEnter={() => !opt.disabled && setActive(i)}
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => choose(opt)}
              className={`flex cursor-pointer select-none items-center gap-2 ${optionPad} ${
                opt.disabled
                  ? 'cursor-not-allowed text-dark-text-quaternary'
                  : isActive
                    ? 'bg-primary-500/15 text-dark-text-primary'
                    : 'text-dark-text-secondary'
              }`}
            >
              {opt.indent ? <span aria-hidden="true" className={`flex-shrink-0 ${INDENT_CLASSES[Math.min(opt.indent, INDENT_CLASSES.length - 1)]}`} /> : null}
              <span className="min-w-0 flex-1">
                <span className={`block truncate ${isSelected ? 'font-medium text-dark-text-primary' : ''}`}>{opt.label}</span>
                {opt.description && (
                  <span className="block truncate text-xs text-dark-text-tertiary">{opt.description}</span>
                )}
              </span>
              {opt.hint && (
                <span className="flex-shrink-0 rounded-full bg-dark-bg-tertiary px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-dark-text-tertiary">
                  {opt.hint}
                </span>
              )}
              <svg
                aria-hidden="true"
                className={`h-4 w-4 flex-shrink-0 text-primary-400 ${isSelected ? '' : 'invisible'}`}
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
              </svg>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
