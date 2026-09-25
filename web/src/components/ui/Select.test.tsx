import { describe, it, expect, vi } from 'vitest'
import { useState } from 'react'
import { render, screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import Select, { AUTO_SEARCH_THRESHOLD, type SelectOption } from './Select'

const roles: SelectOption[] = [
  { value: 'viewer', label: 'Viewer' },
  { value: 'member', label: 'Member' },
  { value: 'editor', label: 'Editor', disabled: true, title: 'Not allowed' },
  { value: 'owner', label: 'Owner', hint: 'Current' },
]

function Controlled<T extends string | number>(props: { initial: T; options: SelectOption<T>[]; onChange?: (v: T) => void; searchable?: boolean }) {
  const [value, setValue] = useState<T>(props.initial)
  return (
    <>
      <label htmlFor="sel">Role</label>
      <Select
        id="sel"
        value={value}
        options={props.options}
        searchable={props.searchable}
        onChange={(v) => {
          setValue(v)
          props.onChange?.(v)
        }}
      />
    </>
  )
}

describe('Select', () => {
  it('shows the selected label on a labelled trigger, not a native select', () => {
    const { container } = render(<Controlled initial="member" options={roles} />)
    const trigger = screen.getByLabelText('Role')
    expect(trigger.tagName).toBe('BUTTON')
    expect(trigger).toHaveTextContent('Member')
    expect(trigger).toHaveAttribute('aria-haspopup', 'listbox')
    expect(container.querySelector('select')).toBeNull()
  })

  it('shows the placeholder when nothing matches the value', () => {
    render(<Select value="" onChange={() => {}} options={roles} placeholder="Pick a role" />)
    expect(screen.getByRole('button')).toHaveTextContent('Pick a role')
  })

  it('opens a listbox with the selected option marked and hints rendered', async () => {
    const user = userEvent.setup()
    render(<Controlled initial="owner" options={roles} />)
    await user.click(screen.getByLabelText('Role'))

    const listbox = await screen.findByRole('listbox')
    const options = within(listbox).getAllByRole('option')
    expect(options).toHaveLength(4)
    expect(within(listbox).getByRole('option', { name: /Owner/ })).toHaveAttribute('aria-selected', 'true')
    expect(within(listbox).getByRole('option', { name: /Viewer/ })).toHaveAttribute('aria-selected', 'false')
    expect(within(listbox).getByText('Current')).toBeInTheDocument()
  })

  it('selects with the mouse and closes', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<Controlled initial="viewer" options={roles} onChange={onChange} />)
    await user.click(screen.getByLabelText('Role'))
    await user.click(await screen.findByRole('option', { name: 'Member' }))

    expect(onChange).toHaveBeenCalledWith('member')
    await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument())
    expect(screen.getByLabelText('Role')).toHaveTextContent('Member')
  })

  it('does not fire onChange when the current value is picked again', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<Controlled initial="viewer" options={roles} onChange={onChange} />)
    await user.click(screen.getByLabelText('Role'))
    await user.click(await screen.findByRole('option', { name: 'Viewer' }))
    expect(onChange).not.toHaveBeenCalled()
  })

  it('ignores disabled options', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<Controlled initial="viewer" options={roles} onChange={onChange} />)
    await user.click(screen.getByLabelText('Role'))
    const editor = await screen.findByRole('option', { name: 'Editor' })
    expect(editor).toHaveAttribute('aria-disabled', 'true')
    await user.click(editor)
    expect(onChange).not.toHaveBeenCalled()
  })

  it('is fully keyboard operable and skips disabled options', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<Controlled initial="viewer" options={roles} onChange={onChange} />)
    screen.getByLabelText('Role').focus()

    await user.keyboard('{ArrowDown}')
    const listbox = await screen.findByRole('listbox')
    await waitFor(() => expect(listbox).toHaveFocus())
    // Active starts on the selected option (Viewer); down twice skips disabled Editor.
    await user.keyboard('{ArrowDown}{ArrowDown}')
    const active = listbox.getAttribute('aria-activedescendant')
    expect(document.getElementById(active!)).toHaveTextContent('Owner')
    await user.keyboard('{Enter}')
    expect(onChange).toHaveBeenCalledWith('owner')
  })

  it('supports Home/End and type-ahead', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<Controlled initial="member" options={roles} onChange={onChange} />)
    await user.click(screen.getByLabelText('Role'))
    const listbox = await screen.findByRole('listbox')

    await user.keyboard('{End}')
    expect(document.getElementById(listbox.getAttribute('aria-activedescendant')!)).toHaveTextContent('Owner')
    await user.keyboard('{Home}')
    expect(document.getElementById(listbox.getAttribute('aria-activedescendant')!)).toHaveTextContent('Viewer')
    await user.keyboard('o')
    await user.keyboard(' ')
    expect(onChange).toHaveBeenCalledWith('owner')
  })

  it('closes on Escape without changing the value', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<Controlled initial="member" options={roles} onChange={onChange} />)
    await user.click(screen.getByLabelText('Role'))
    await screen.findByRole('listbox')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument())
    expect(onChange).not.toHaveBeenCalled()
  })

  it('does not open when disabled', async () => {
    const user = userEvent.setup()
    render(<Select value="member" onChange={() => {}} options={roles} disabled aria-label="Role" />)
    const trigger = screen.getByRole('button', { name: 'Role' })
    expect(trigger).toBeDisabled()
    await user.click(trigger)
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
  })

  it('keeps number values as numbers', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const hours = Array.from({ length: 3 }, (_, h) => ({ value: h, label: `${h}:00` }))
    render(<Controlled initial={0} options={hours} onChange={onChange} />)
    await user.click(screen.getByLabelText('Role'))
    await user.click(await screen.findByRole('option', { name: '2:00' }))
    expect(onChange).toHaveBeenCalledWith(2)
  })

  describe('search box', () => {
    const teams = Array.from({ length: AUTO_SEARCH_THRESHOLD }, (_, i) => ({
      value: i + 1,
      label: ['Adhiraj', 'AIAgentLens', 'Biswas', 'Elastio', 'Intellivizz', 'TickrAPI', 'Velvet Hour', 'Zeta'][i],
      description: i === 3 ? 'elastio.com' : undefined,
    }))

    it('appears automatically for long lists and filters by label or description', async () => {
      const user = userEvent.setup()
      const onChange = vi.fn()
      render(<Controlled initial={1} options={teams} onChange={onChange} />)
      await user.click(screen.getByLabelText('Role'))

      const search = await screen.findByRole('combobox', { name: 'Search…' })
      await waitFor(() => expect(search).toHaveFocus())
      await user.type(search, 'elastio.com')
      const listbox = screen.getByRole('listbox')
      expect(within(listbox).getAllByRole('option')).toHaveLength(1)

      await user.keyboard('{Enter}')
      expect(onChange).toHaveBeenCalledWith(4)
    })

    it('shows an empty state when nothing matches', async () => {
      const user = userEvent.setup()
      render(<Controlled initial={1} options={teams} />)
      await user.click(screen.getByLabelText('Role'))
      await user.type(await screen.findByRole('combobox'), 'zzz')
      expect(screen.getByText('No matches')).toBeInTheDocument()
    })

    it('stays off for short lists unless asked for', async () => {
      const user = userEvent.setup()
      const { unmount } = render(<Controlled initial="viewer" options={roles} />)
      await user.click(screen.getByLabelText('Role'))
      await screen.findByRole('listbox')
      expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
      unmount()

      render(<Controlled initial="viewer" options={roles} searchable />)
      await user.click(screen.getByLabelText('Role'))
      expect(await screen.findByRole('combobox')).toBeInTheDocument()
    })
  })
})
