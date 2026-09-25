import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import WikiPageTree from './WikiPageTree'
import type { WikiPage } from '../lib/api'

function page(id: number, title: string, parent_id: number | null, position = 0): WikiPage {
  return {
    id,
    project_id: 7,
    title,
    slug: title.toLowerCase().replace(/\s+/g, '-'),
    parent_id,
    position,
    created_by: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }
}

// Getting Started(1) > Install(2) > Linux(3); Reference(4) top-level
const pages = [
  page(1, 'Getting Started', null, 0),
  page(2, 'Install', 1, 0),
  page(3, 'Linux', 2, 0),
  page(4, 'Reference', null, 1),
]

function renderTree(overrides: Partial<Parameters<typeof WikiPageTree>[0]> = {}) {
  const props = {
    projectId: 7,
    pages,
    selectedPageId: null as number | null,
    searchQuery: '',
    onSelect: vi.fn(),
    onCreate: vi.fn().mockResolvedValue(page(99, 'New', null)),
    onMove: vi.fn().mockResolvedValue(undefined),
    onDelete: vi.fn(),
    ...overrides,
  }
  return { ...render(<WikiPageTree {...props} />), props }
}

describe('WikiPageTree', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('renders top-level pages collapsed by default', () => {
    renderTree()
    const tree = screen.getByRole('tree', { name: 'Wiki pages' })
    expect(within(tree).getByText('Getting Started')).toBeInTheDocument()
    expect(within(tree).getByText('Reference')).toBeInTheDocument()
    expect(within(tree).queryByText('Install')).not.toBeInTheDocument()
  })

  it('expands a node when its chevron is clicked and reveals nested levels', async () => {
    const user = userEvent.setup()
    renderTree()
    await user.click(screen.getByRole('button', { name: 'Expand Getting Started' }))
    expect(screen.getByText('Install')).toBeInTheDocument()
    expect(screen.getByRole('treeitem', { name: /Install/ })).toHaveAttribute('aria-level', '2')

    await user.click(screen.getByRole('button', { name: 'Expand Install' }))
    expect(screen.getByRole('treeitem', { name: /Linux/ })).toHaveAttribute('aria-level', '3')
  })

  it('auto-expands ancestors of the selected page', () => {
    renderTree({ selectedPageId: 3 })
    expect(screen.getByText('Linux')).toBeInTheDocument()
    expect(screen.getByRole('treeitem', { name: /Linux/ })).toHaveAttribute('aria-selected', 'true')
  })

  it('selects a page on click', async () => {
    const user = userEvent.setup()
    const { props } = renderTree()
    await user.click(screen.getByText('Reference'))
    expect(props.onSelect).toHaveBeenCalledWith(4)
  })

  it('creates a sub-page under the chosen parent', async () => {
    const user = userEvent.setup()
    const { props } = renderTree()
    await user.click(screen.getByRole('button', { name: 'Add sub-page under Reference' }))
    const input = screen.getByRole('textbox', { name: 'New sub-page title' })
    await user.type(input, 'Glossary{Enter}')
    expect(props.onCreate).toHaveBeenCalledWith('Glossary', 4)
  })

  it('creates a top-level page from the toolbar', async () => {
    const user = userEvent.setup()
    const { props } = renderTree()
    await user.click(screen.getByRole('button', { name: 'New top-level page' }))
    await user.type(screen.getByRole('textbox', { name: 'New page title' }), 'Roadmap{Enter}')
    expect(props.onCreate).toHaveBeenCalledWith('Roadmap', null)
  })

  it('disables adding a sub-page at the maximum depth', () => {
    const deep: WikiPage[] = []
    for (let i = 1; i <= 6; i++) deep.push(page(i, `L${i}`, i === 1 ? null : i - 1))
    renderTree({ pages: deep, selectedPageId: 6 })
    expect(screen.getByRole('button', { name: /Cannot add sub-page/ })).toBeDisabled()
  })

  it('moves a page to the top level from its menu', async () => {
    const user = userEvent.setup()
    const { props } = renderTree({ selectedPageId: 2 })
    await user.click(screen.getByRole('button', { name: 'More actions for Install' }))
    await user.click(screen.getByRole('menuitem', { name: 'Move to top level' }))
    expect(props.onMove).toHaveBeenCalledWith(2, null)
  })

  it('opens the move dialog and blocks moving under a descendant', async () => {
    const user = userEvent.setup()
    const { props } = renderTree({ selectedPageId: 1 })
    await user.click(screen.getByRole('button', { name: 'More actions for Getting Started' }))
    await user.click(screen.getByRole('menuitem', { name: 'Move to…' }))
    const dialog = screen.getByRole('dialog')
    await user.click(within(dialog).getByLabelText('New parent'))
    const listbox = await screen.findByRole('listbox')
    // Descendants of the moved page are not offered at all.
    expect(within(listbox).queryByRole('option', { name: /Install/ })).not.toBeInTheDocument()
    await user.click(within(listbox).getByRole('option', { name: 'Reference' }))
    await user.click(within(dialog).getByRole('button', { name: 'Move' }))
    expect(props.onMove).toHaveBeenCalledWith(1, 4)
  })

  it('asks to delete via the menu', async () => {
    const user = userEvent.setup()
    const { props } = renderTree()
    await user.click(screen.getByRole('button', { name: 'More actions for Reference' }))
    await user.click(screen.getByRole('menuitem', { name: 'Delete' }))
    expect(props.onDelete).toHaveBeenCalledWith(4)
  })

  it('shows a flat result list with ancestor paths while searching', () => {
    renderTree({ searchQuery: 'lin' })
    expect(screen.getByRole('listbox', { name: 'Search results' })).toBeInTheDocument()
    expect(screen.getByText('Linux')).toBeInTheDocument()
    expect(screen.getByText('Getting Started / Install')).toBeInTheDocument()
    expect(screen.queryByText('Reference')).not.toBeInTheDocument()
  })

  it('supports arrow-key navigation between visible nodes', async () => {
    const user = userEvent.setup()
    renderTree({ selectedPageId: 1 })
    const first = screen.getByRole('treeitem', { name: /Getting Started/ })
    first.focus()
    await user.keyboard('{ArrowRight}')
    expect(screen.getByText('Install')).toBeInTheDocument()
    await user.keyboard('{ArrowDown}')
    expect(screen.getByRole('treeitem', { name: /Install/ })).toHaveFocus()
    await user.keyboard('{ArrowLeft}')
    expect(first).toHaveFocus()
  })
})

describe('WikiPageTree sort and filter', () => {
  // Newest → oldest by creation: Newer, Middle, Older.
  const authored: WikiPage[] = [
    { ...page(10, 'Middle', null, 0), created_at: '2026-02-01T00:00:00Z', creator_name: 'Ada' },
    { ...page(11, 'Newer', null, 1), created_at: '2026-03-01T00:00:00Z', creator_name: 'Zoe' },
    { ...page(12, 'Older', null, 2), created_at: '2026-01-01T00:00:00Z', creator_name: 'Ada' },
  ]

  // The menu stays open while options are picked, so only toggle it when closed.
  const openMenu = async (user: ReturnType<typeof userEvent.setup>) => {
    if (!screen.queryByRole('dialog', { name: 'Sort and filter pages' })) {
      await user.click(screen.getByRole('button', { name: /Sort and filter pages/ }))
    }
    return screen.getByRole('dialog', { name: 'Sort and filter pages' })
  }

  const treeTitles = () =>
    within(screen.getByRole('tree', { name: 'Wiki pages' }))
      .getAllByRole('treeitem')
      .map(el => el.getAttribute('data-page-id'))

  beforeEach(() => {
    localStorage.clear()
  })

  it('shows newest-created pages first by default', () => {
    renderTree({ pages: authored })
    expect(treeTitles()).toEqual(['11', '10', '12'])
  })

  it('re-sorts when a different order is picked', async () => {
    const user = userEvent.setup()
    renderTree({ pages: authored })
    const menu = await openMenu(user)
    await user.click(within(menu).getByRole('button', { name: /Oldest first/ }))
    expect(treeTitles()).toEqual(['12', '10', '11'])
  })

  it('filters the tree down to one author, with a way back', async () => {
    const user = userEvent.setup()
    renderTree({ pages: authored })
    const menu = await openMenu(user)
    await user.click(within(menu).getByRole('button', { name: /^Zoe/ }))
    expect(treeTitles()).toEqual(['11'])

    await user.click(within(await openMenu(user)).getByRole('button', { name: /Everyone/ }))
    expect(treeTitles()).toEqual(['11', '10', '12'])
  })

  it('offers a way out when the author filter matches nothing', async () => {
    const user = userEvent.setup()
    const { rerender, props } = renderTree({ pages: authored })
    await user.click(within(await openMenu(user)).getByRole('button', { name: /^Zoe/ }))

    // Zoe's only page is gone; the filter must not leave a dead-end empty tree.
    const withoutZoe = authored.filter(pg => pg.creator_name !== 'Zoe')
    rerender(<WikiPageTree {...props} pages={withoutZoe} />)
    expect(treeTitles()).toEqual(['10', '12'])
  })

  it('remembers the chosen order across remounts', async () => {
    const user = userEvent.setup()
    const { unmount } = renderTree({ pages: authored })
    await user.click(within(await openMenu(user)).getByRole('button', { name: /Title A–Z/ }))
    unmount()

    renderTree({ pages: authored })
    expect(treeTitles()).toEqual(['10', '11', '12'])
  })
})
