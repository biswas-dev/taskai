/**
 * Command Palette (Cmd+K / Ctrl+K)
 * Linear-style command palette for quick navigation and actions
 */

import { useState, useEffect, useMemo, useRef, useCallback, Fragment } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { Dialog, Transition, Combobox } from '@headlessui/react'
import { useAuth } from '../state/AuthContext'
import { api } from '../lib/api'
import type { SearchTaskResult, GlobalSearchWikiResult } from '../lib/api'

// Detect Mac vs Windows/Linux for keyboard shortcut display
const isMac = typeof navigator !== 'undefined' && /Mac|iPod|iPhone|iPad/.test(navigator.userAgent)
// eslint-disable-next-line react-refresh/only-export-components
export const searchShortcutLabel = isMac ? '⌘K' : 'Ctrl+K'

interface Command {
  id: string
  name: string
  description?: string
  snippet?: string
  icon: string
  action: () => void
  category: 'navigation' | 'actions' | 'tasks' | 'wiki'
  keywords?: string[]
}

export default function CommandPalette() {
  const [isOpen, setIsOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [taskResults, setTaskResults] = useState<SearchTaskResult[]>([])
  const [wikiResults, setWikiResults] = useState<GlobalSearchWikiResult[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const { logout } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const abortControllerRef = useRef<AbortController | null>(null)
  const debounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Keyboard shortcut to open/close palette
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Cmd+K (Mac) or Ctrl+K (Windows/Linux)
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        setIsOpen(open => !open)
      }
      // Escape to close
      if (e.key === 'Escape') {
        setIsOpen(false)
      }
    }

    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [])

  // Reset state when closed
  useEffect(() => {
    if (!isOpen) {
      setQuery('')
      setTaskResults([])
      setWikiResults([])
      setIsLoading(false)
      if (abortControllerRef.current) {
        abortControllerRef.current.abort()
        abortControllerRef.current = null
      }
      if (debounceTimerRef.current) {
        clearTimeout(debounceTimerRef.current)
        debounceTimerRef.current = null
      }
    }
  }, [isOpen])

  // Extract project ID from current URL if on a project page
  const currentProjectId = useMemo(() => {
    const match = location.pathname.match(/\/app\/projects\/(\d+)/)
    return match ? Number(match[1]) : undefined
  }, [location.pathname])

  // Debounced search
  const performSearch = useCallback(async (searchQuery: string) => {
    // Cancel any in-flight request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
    }

    if (searchQuery.length < 2) {
      setTaskResults([])
      setWikiResults([])
      setIsLoading(false)
      return
    }

    const controller = new AbortController()
    abortControllerRef.current = controller
    setIsLoading(true)

    try {
      const results = await api.globalSearch(searchQuery, currentProjectId, undefined, 10, controller.signal)
      // Only update if this request wasn't aborted
      if (!controller.signal.aborted) {
        setTaskResults(results.tasks)
        setWikiResults(results.wiki)
        setIsLoading(false)
      }
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        return // Ignore aborted requests
      }
      if (!controller.signal.aborted) {
        setTaskResults([])
        setWikiResults([])
        setIsLoading(false)
      }
    }
  }, [currentProjectId])

  // Handle query change with debounce
  const handleQueryChange = useCallback((value: string) => {
    setQuery(value)

    if (debounceTimerRef.current) {
      clearTimeout(debounceTimerRef.current)
    }

    debounceTimerRef.current = setTimeout(() => {
      performSearch(value)
    }, 150)
  }, [performSearch])

  // Define available commands
  const staticCommands: Command[] = useMemo(() => [
    // Navigation
    {
      id: 'nav-projects',
      name: 'Go to Projects',
      description: 'View all projects',
      icon: '📁',
      category: 'navigation',
      keywords: ['projects', 'list'],
      action: () => {
        navigate('/app')
        setIsOpen(false)
      },
    },
    {
      id: 'nav-cycles',
      name: 'Go to Cycles',
      description: 'View sprint cycles',
      icon: '🔄',
      category: 'navigation',
      keywords: ['sprints', 'cycles', 'iterations'],
      action: () => {
        const pid = localStorage.getItem('taskai_last_project')
        navigate(pid ? `/app/projects/${pid}/sprints` : '/app')
        setIsOpen(false)
      },
    },
    {
      id: 'nav-tags',
      name: 'Go to Tags',
      description: 'Manage labels and tags',
      icon: '🏷️',
      category: 'navigation',
      keywords: ['tags', 'labels'],
      action: () => {
        const pid = localStorage.getItem('taskai_last_project')
        navigate(pid ? `/app/projects/${pid}/tags` : '/app')
        setIsOpen(false)
      },
    },
    {
      id: 'nav-settings',
      name: 'Go to Settings',
      description: 'Account and preferences',
      icon: '⚙️',
      category: 'navigation',
      keywords: ['settings', 'preferences', 'config'],
      action: () => {
        navigate('/app/settings')
        setIsOpen(false)
      },
    },
    // Actions
    {
      id: 'action-logout',
      name: 'Logout',
      description: 'Sign out of your account',
      icon: '🚪',
      category: 'actions',
      keywords: ['logout', 'signout', 'exit'],
      action: () => {
        logout()
        navigate('/login')
        setIsOpen(false)
      },
    },
  ], [navigate, logout])

  // Convert search results to commands
  // Capture query at memo time so wiki highlight URLs have the correct value
  const searchCommands: Command[] = useMemo(() => {
    const currentQuery = query
    const commands: Command[] = []

    taskResults.forEach(task => {
      let statusIcon = '📝'
      if (task.status === 'done') statusIcon = '✅'
      else if (task.status === 'in_progress') statusIcon = '🔄'
      const prioritySuffix = task.priority === 'medium' ? '' : ' · ' + task.priority
      const ghSuffix = task.github_issue_number ? ` · GH#${task.github_issue_number}` : ''
      commands.push({
        id: `task-${task.id}`,
        name: `#${task.task_number} ${task.title}`,
        description: task.project_name + ' · ' + task.status.replace('_', ' ') + prioritySuffix + ghSuffix,
        icon: statusIcon,
        category: 'tasks',
        action: () => {
          navigate(`/app/projects/${task.project_id}/tasks/${task.task_number}`)
          setIsOpen(false)
        },
      })
    })

    wikiResults.forEach(wiki => {
      commands.push({
        id: `wiki-${wiki.page_id}-${wiki.snippet?.slice(0, 20) ?? ''}`,
        name: wiki.page_title,
        description: wiki.project_name + (wiki.headings_path ? ' · ' + wiki.headings_path : ''),
        snippet: wiki.snippet || undefined,
        icon: '📄',
        category: 'wiki',
        action: () => {
          navigate(`/app/projects/${wiki.project_id}?tab=wiki&page=${wiki.page_id}&highlight=${encodeURIComponent(currentQuery)}`)
          setIsOpen(false)
        },
      })
    })

    return commands
  }, [taskResults, wikiResults, navigate, query])

  // Filter static commands based on query
  const filteredStaticCommands = useMemo(() => {
    if (!query) return staticCommands

    const lowerQuery = query.toLowerCase()
    return staticCommands.filter(cmd => {
      if (cmd.name.toLowerCase().includes(lowerQuery)) return true
      if (cmd.description?.toLowerCase().includes(lowerQuery)) return true
      if (cmd.keywords?.some(k => k.includes(lowerQuery))) return true
      return false
    })
  }, [query, staticCommands])

  // Group all commands
  const allCommands = useMemo(() => [...filteredStaticCommands, ...searchCommands], [filteredStaticCommands, searchCommands])

  const groupedCommands = useMemo(() => {
    const groups: Record<string, Command[]> = {
      tasks: [],
      wiki: [],
      navigation: [],
      actions: [],
    }

    allCommands.forEach(cmd => {
      if (groups[cmd.category]) {
        groups[cmd.category].push(cmd)
      }
    })

    return groups
  }, [allCommands])

  const categoryLabels: Record<string, string> = {
    tasks: 'Tasks',
    wiki: 'Wiki Pages',
    navigation: 'Navigation',
    actions: 'Actions',
  }

  const hasResults = allCommands.length > 0
  const hasSearchQuery = query.length >= 2

  return (
    <Transition.Root show={isOpen} as={Fragment} afterLeave={() => setQuery('')}>
      <Dialog className="relative z-50" onClose={setIsOpen}>
        {/* Backdrop */}
        <Transition.Child
          as={Fragment}
          enter="ease-out duration-200"
          enterFrom="opacity-0"
          enterTo="opacity-100"
          leave="ease-in duration-150"
          leaveFrom="opacity-100"
          leaveTo="opacity-0"
        >
          <div className="fixed inset-0 bg-black bg-opacity-40 backdrop-blur-sm transition-opacity" />
        </Transition.Child>

        <div className="fixed inset-0 z-10 overflow-y-auto p-4 sm:p-6 md:p-20">
          <Transition.Child
            as={Fragment}
            enter="ease-out duration-200"
            enterFrom="opacity-0 scale-95"
            enterTo="opacity-100 scale-100"
            leave="ease-in duration-150"
            leaveFrom="opacity-100 scale-100"
            leaveTo="opacity-0 scale-95"
          >
            <Dialog.Panel className="mx-auto max-w-2xl transform rounded-xl bg-dark-bg-elevated shadow-linear-xl ring-1 ring-dark-border-medium transition-all">
              <Combobox onChange={(command: Command | null) => command?.action()}>
                {/* Search input */}
                <div className="relative">
                  <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-4">
                    {isLoading ? (
                      <svg className="animate-spin h-5 w-5 text-dark-text-quaternary" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                        <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                        <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
                      </svg>
                    ) : (
                      <svg className="h-5 w-5 text-dark-text-quaternary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                      </svg>
                    )}
                  </div>
                  <Combobox.Input
                    className="h-14 w-full border-0 bg-transparent pl-12 pr-4 text-dark-text-primary placeholder-dark-text-quaternary focus:ring-0 text-base"
                    placeholder={currentProjectId ? "Search tasks & wiki in this project..." : "Search tasks, wiki pages, or type a command..."}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => handleQueryChange(e.target.value)}
                    autoFocus
                  />
                  <div className="absolute inset-y-0 right-0 flex items-center pr-4">
                    <kbd className="hidden sm:inline-block px-2 py-1 text-xs font-semibold text-dark-text-quaternary bg-dark-bg-secondary rounded border border-dark-border-subtle">
                      Esc
                    </kbd>
                  </div>
                </div>

                {/* Results */}
                {hasResults && (
                  <Combobox.Options
                    static
                    className="max-h-96 scroll-py-2 overflow-y-auto border-t border-dark-border-subtle"
                  >
                    {Object.entries(groupedCommands).map(([category, commands]) => {
                      if (commands.length === 0) return null

                      return (
                        <div key={category} className="p-2">
                          <div className="px-3 py-2 text-[11px] font-semibold text-dark-text-quaternary uppercase tracking-wider">
                            {categoryLabels[category] || category}
                          </div>
                          {commands.map((command) => (
                            <Combobox.Option
                              key={command.id}
                              value={command}
                              className={({ active }: { active: boolean }) =>
                                `flex cursor-pointer select-none items-center rounded-md px-3 py-2 ${
                                  active ? 'bg-dark-bg-tertiary text-dark-text-primary' : 'text-dark-text-secondary'
                                }`
                              }
                            >
                              {({ active }: { active: boolean }) => (
                                <>
                                  <span className="mr-3 text-xl flex-shrink-0">{command.icon}</span>
                                  <div className="flex-1 min-w-0">
                                    <p className={`text-sm font-medium truncate ${active ? 'text-dark-text-primary' : 'text-dark-text-secondary'}`}>
                                      {command.name}
                                    </p>
                                    {command.description && (
                                      <p className={`text-xs truncate ${active ? 'text-dark-text-tertiary' : 'text-dark-text-quaternary'}`}>
                                        {command.description}
                                      </p>
                                    )}
                                    {command.snippet && (
                                      <p className={`text-xs truncate mt-0.5 ${active ? 'text-dark-text-quaternary' : 'text-dark-text-quaternary/70'}`}>
                                        {command.snippet}
                                      </p>
                                    )}
                                  </div>
                                  <span className={`ml-3 text-xs flex-shrink-0 ${active ? 'text-dark-text-tertiary' : 'text-dark-text-quaternary'}`}>
                                    ↵
                                  </span>
                                </>
                              )}
                            </Combobox.Option>
                          ))}
                        </div>
                      )
                    })}
                  </Combobox.Options>
                )}

                {/* Loading state for search */}
                {isLoading && hasSearchQuery && searchCommands.length === 0 && (
                  <div className="border-t border-dark-border-subtle px-6 py-8 text-center">
                    <svg className="animate-spin h-5 w-5 text-dark-text-quaternary mx-auto mb-2" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
                    </svg>
                    <p className="text-sm text-dark-text-tertiary">Searching...</p>
                  </div>
                )}

                {/* Empty state */}
                {query && !isLoading && !hasResults && (
                  <div className="border-t border-dark-border-subtle px-6 py-14 text-center">
                    <p className="text-sm text-dark-text-tertiary">No results found for "{query}"</p>
                  </div>
                )}

                {/* Footer hint */}
                {!query && (
                  <div className="border-t border-dark-border-subtle px-4 py-3 text-xs text-dark-text-quaternary bg-dark-bg-secondary">
                    <div className="flex items-center justify-between">
                      <span>
                        Type to search • Use <kbd className="px-1.5 py-0.5 bg-dark-bg-tertiary rounded border border-dark-border-subtle text-dark-text-tertiary">↑↓</kbd> to navigate
                      </span>
                      <span>
                        <kbd className="px-1.5 py-0.5 bg-dark-bg-tertiary rounded border border-dark-border-subtle text-dark-text-tertiary">{searchShortcutLabel}</kbd> to toggle
                      </span>
                    </div>
                  </div>
                )}
              </Combobox>
            </Dialog.Panel>
          </Transition.Child>
        </div>
      </Dialog>
    </Transition.Root>
  )
}
