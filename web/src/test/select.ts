import { screen, within } from '@testing-library/react'
import type { UserEvent } from '@testing-library/user-event'

/** Opens a `Select` and clicks the option with the given accessible name. */
export async function pickOption(user: UserEvent, trigger: HTMLElement, name: string | RegExp) {
  await user.click(trigger)
  const listbox = await screen.findByRole('listbox')
  await user.click(within(listbox).getByRole('option', { name }))
}
