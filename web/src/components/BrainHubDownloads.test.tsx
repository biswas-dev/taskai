import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'

// The release is cached per page load, so each test loads a fresh module.
let BrainHubDownloads: (typeof import('./BrainHubDownloads'))['default']
beforeEach(async () => {
  vi.resetModules()
  BrainHubDownloads = (await import('./BrainHubDownloads')).default
})

const asset = (name: string) => ({ name, browser_download_url: `https://example.test/${name}` })
const releases = [
  { tag_name: 'hub-v0.1.18', draft: false, prerelease: false, assets: [asset('hub-linux-amd64')] },
  { tag_name: 'client-v0.2.0', draft: true, prerelease: false, assets: [] },
  { tag_name: 'client-v0.1.99', draft: false, prerelease: true, assets: [] },
  {
    tag_name: 'client-v0.1.17',
    draft: false,
    prerelease: false,
    assets: ['BrainHub-mac-0.1.17.zip', 'Brain.Hub_0.1.17_x64-setup.exe', 'Brain.Hub_0.1.17_x64_en-US.msi', 'Brain.Hub_0.1.17_amd64.AppImage', 'Brain.Hub_0.1.17_amd64.deb', 'Brain.Hub-0.1.17-1.x86_64.rpm'].map(asset),
  },
]

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('BrainHubDownloads', () => {
  it('links each system to its asset in the newest published client release (not a hub, draft or prerelease)', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(releases) })
    vi.stubGlobal('fetch', fetchMock)
    render(<BrainHubDownloads />)
    await waitFor(() => expect(screen.getByText(/Desktop app version 0\.1\.17\./)).toBeInTheDocument())
    expect(fetchMock.mock.calls[0][0]).toContain('per_page=100')
    expect(screen.getByRole('link', { name: 'Download for macOS' })).toHaveAttribute('href', 'https://example.test/BrainHub-mac-0.1.17.zip')
    expect(screen.getByRole('link', { name: 'Download for Windows' })).toHaveAttribute('href', 'https://example.test/Brain.Hub_0.1.17_x64-setup.exe')
    expect(screen.getByRole('link', { name: 'Download for Linux' })).toHaveAttribute('href', 'https://example.test/Brain.Hub_0.1.17_amd64.AppImage')
    expect(screen.getByRole('link', { name: '.msi' })).toHaveAttribute('href', 'https://example.test/Brain.Hub_0.1.17_x64_en-US.msi')
    expect(screen.getByRole('link', { name: '.deb' })).toHaveAttribute('href', 'https://example.test/Brain.Hub_0.1.17_amd64.deb')
    expect(screen.getByRole('link', { name: '.rpm' })).toHaveAttribute('href', 'https://example.test/Brain.Hub-0.1.17-1.x86_64.rpm')
  })

  it('calls GitHub once for many mounts', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(releases) })
    vi.stubGlobal('fetch', fetchMock)
    render(<><BrainHubDownloads /><BrainHubDownloads /></>)
    await waitFor(() => expect(screen.getAllByText(/Desktop app version 0\.1\.17\./)).toHaveLength(2))
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('falls back to the release page when GitHub does not answer', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')))
    render(<BrainHubDownloads />)
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('could not be found'))
    expect(screen.getByRole('link', { name: 'Download for macOS' })).toHaveAttribute('href', 'https://github.com/anchoo2kewl/llm-brain/releases')
  })

  it('shows the fallback on a non-OK answer, then tries again on the next mount', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: false, status: 403, json: () => Promise.resolve({}) })
      .mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(releases) })
    vi.stubGlobal('fetch', fetchMock)
    const first = render(<BrainHubDownloads />)
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('could not be found'))
    first.unmount()
    render(<BrainHubDownloads />)
    await waitFor(() => expect(screen.getByRole('link', { name: 'Download for macOS' })).toHaveAttribute('href', 'https://example.test/BrainHub-mac-0.1.17.zip'))
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('falls back when there is no published client release', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve([releases[0]]) }))
    render(<BrainHubDownloads />)
    await waitFor(() => expect(screen.getByRole('status')).toBeInTheDocument())
    expect(screen.getByRole('link', { name: 'Download for Linux' })).toHaveAttribute('href', 'https://github.com/anchoo2kewl/llm-brain/releases')
  })
})
