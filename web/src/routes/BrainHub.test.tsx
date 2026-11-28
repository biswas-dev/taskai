import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import BrainHub from './BrainHub'

let pathname = '/brain-hub'
vi.mock('react-router-dom', () => ({
  Link: ({ to, children, ...props }: { to: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={to} {...props}>{children}</a>
  ),
  useLocation: () => ({ pathname }),
}))

const asset = (name: string) => ({ name, browser_download_url: `https://example.test/${name}` })
const releases = [
  { tag_name: 'hub-v0.1.18', draft: false, prerelease: false, assets: [asset('hub-linux-amd64')] },
  { tag_name: 'client-v0.2.0', draft: true, prerelease: false, assets: [] },
  { tag_name: 'client-v0.1.99', draft: false, prerelease: true, assets: [] },
  {
    tag_name: 'client-v0.1.17',
    draft: false,
    prerelease: false,
    assets: [
      asset('BrainHub-mac-0.1.17.zip'),
      asset('Brain.Hub_0.1.17_x64-setup.exe'),
      asset('Brain.Hub_0.1.17_x64_en-US.msi'),
      asset('Brain.Hub_0.1.17_amd64.AppImage'),
      asset('Brain.Hub_0.1.17_amd64.deb'),
      asset('Brain.Hub-0.1.17-1.x86_64.rpm'),
    ],
  },
]

afterEach(() => {
  vi.unstubAllGlobals()
  pathname = '/brain-hub'
})

describe('BrainHub', () => {
  it('links each system to its asset in the newest published client release (not a hub, draft or prerelease)', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(releases) }))
    render(<BrainHub />)
    await waitFor(() => expect(screen.getByText(/Version 0\.1\.17\./)).toBeInTheDocument())
    expect(screen.getByRole('link', { name: 'Download for macOS' })).toHaveAttribute('href', 'https://example.test/BrainHub-mac-0.1.17.zip')
    expect(screen.getByRole('link', { name: 'Download for Windows' })).toHaveAttribute('href', 'https://example.test/Brain.Hub_0.1.17_x64-setup.exe')
    expect(screen.getByRole('link', { name: 'Download for Linux' })).toHaveAttribute('href', 'https://example.test/Brain.Hub_0.1.17_amd64.AppImage')
    expect(screen.getByRole('link', { name: '.msi' })).toHaveAttribute('href', 'https://example.test/Brain.Hub_0.1.17_x64_en-US.msi')
    expect(screen.getByRole('link', { name: '.deb' })).toHaveAttribute('href', 'https://example.test/Brain.Hub_0.1.17_amd64.deb')
    expect(screen.getByRole('link', { name: '.rpm' })).toHaveAttribute('href', 'https://example.test/Brain.Hub-0.1.17-1.x86_64.rpm')
  })

  it('falls back to the release page when GitHub does not answer', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')))
    render(<BrainHub />)
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('could not be found'))
    expect(screen.getByRole('link', { name: 'Download for macOS' })).toHaveAttribute('href', 'https://github.com/anchoo2kewl/llm-brain/releases')
  })

  it('says Brain Hub is MIT open source and TaskAI is proprietary', () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, json: () => Promise.resolve({}) }))
    render(<BrainHub />)
    expect(screen.getByText(/open source under the MIT license/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'MIT license' })).toHaveAttribute('href', 'https://github.com/anchoo2kewl/llm-brain/blob/main/LICENSE')
    expect(screen.getByText('TaskAI is a proprietary service. It is not open source.')).toBeInTheDocument()
    expect(screen.getByText(/Encrypted sync \(in progress\)/)).toBeInTheDocument()
  })

  it('shows its own top bar on the public page only', () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, json: () => Promise.resolve({}) }))
    const { unmount } = render(<BrainHub />)
    expect(screen.getByRole('navigation')).toBeInTheDocument()
    unmount()
    pathname = '/app/brain-hub'
    render(<BrainHub />)
    expect(screen.queryByRole('navigation')).toBeNull()
  })
})
