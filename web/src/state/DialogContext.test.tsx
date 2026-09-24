import { useState } from 'react'
import { describe, it, expect } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DialogProvider, useDialog } from './DialogContext'

function ConfirmHarness() {
  const dialog = useDialog()
  const [result, setResult] = useState('pending')
  const ask = async () => {
    const ok = await dialog.confirm({ title: 'Delete thing?', message: 'It will be gone.', confirmLabel: 'Delete', danger: true })
    setResult(ok ? 'confirmed' : 'cancelled')
  }
  return (
    <>
      <button type="button" onClick={ask}>Ask</button>
      <output>{result}</output>
    </>
  )
}

function NotifyHarness() {
  const dialog = useDialog()
  return (
    <button type="button" onClick={() => dialog.notify('Something broke', 'error')}>
      Fail
    </button>
  )
}

const renderConfirm = () =>
  render(
    <DialogProvider>
      <ConfirmHarness />
    </DialogProvider>,
  )

describe('DialogProvider', () => {
  it('resolves confirm to true when the confirm button is clicked', async () => {
    const user = userEvent.setup()
    renderConfirm()

    await user.click(screen.getByRole('button', { name: 'Ask' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Delete thing?')).toBeInTheDocument()
    expect(within(dialog).getByText('It will be gone.')).toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

    expect(await screen.findByText('confirmed')).toBeInTheDocument()
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  })

  it('resolves confirm to false when Cancel is clicked', async () => {
    const user = userEvent.setup()
    renderConfirm()

    await user.click(screen.getByRole('button', { name: 'Ask' }))
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Cancel' }))

    expect(await screen.findByText('cancelled')).toBeInTheDocument()
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  })

  it('resolves confirm to false when Escape is pressed', async () => {
    const user = userEvent.setup()
    renderConfirm()

    await user.click(screen.getByRole('button', { name: 'Ask' }))
    await screen.findByRole('alertdialog')
    await user.keyboard('{Escape}')

    expect(await screen.findByText('cancelled')).toBeInTheDocument()
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  })

  it('shows an error toast with role alert for notify', async () => {
    const user = userEvent.setup()
    render(
      <DialogProvider>
        <NotifyHarness />
      </DialogProvider>,
    )

    await user.click(screen.getByRole('button', { name: 'Fail' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Something broke')
  })
})
