import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import Assets from './Assets'

vi.mock('react-router-dom', () => ({
  useParams: () => ({ projectId: '1' }),
}))

vi.mock('../components/MyAssets', () => ({
  default: ({ projectId }: { projectId: number }) => <div data-testid="my-assets" data-project-id={projectId}>MyAssets</div>,
}))

describe('Assets', () => {
  it('renders MyAssets component', () => {
    render(<Assets />)
    expect(screen.getByTestId('my-assets')).toBeInTheDocument()
    expect(screen.getByText('MyAssets')).toBeInTheDocument()
  })
})
