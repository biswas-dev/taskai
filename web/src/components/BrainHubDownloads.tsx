import { useEffect, useState } from 'react'

const BRAIN_HUB_REPO = 'https://github.com/anchoo2kewl/llm-brain'
const RELEASES = `${BRAIN_HUB_REPO}/releases`
// 100 releases, so newer hub-v releases cannot push the newest client-v release off the page.
const RELEASES_API = 'https://api.github.com/repos/anchoo2kewl/llm-brain/releases?per_page=100'

interface GitHubRelease {
  tag_name: string
  draft: boolean
  prerelease: boolean
  assets: { name: string; browser_download_url: string }[]
}

type Match = (name: string) => boolean

// Asset names carry the version (docs/desktop-app.md in llm-brain), so the newest client-v release is read at run time.
const downloads: { system: string; primary: Match; others: { label: string; match: Match }[] }[] = [
  { system: 'macOS', primary: n => n.startsWith('BrainHub-mac-') && n.endsWith('.zip'), others: [] },
  { system: 'Windows', primary: n => n.endsWith('_x64-setup.exe'), others: [{ label: '.msi', match: n => n.endsWith('.msi') }] },
  {
    system: 'Linux',
    primary: n => n.endsWith('.AppImage'),
    others: [
      { label: '.deb', match: n => n.endsWith('.deb') },
      { label: '.rpm', match: n => n.endsWith('.rpm') },
    ],
  },
]

const linkClass = 'text-primary-400 hover:text-primary-300 underline underline-offset-2'

// One GitHub call per page load, shared by every mount (GitHub allows 60 unauthenticated calls per hour per IP).
let latestRelease: Promise<GitHubRelease | undefined> | undefined
function loadLatestRelease(): Promise<GitHubRelease | undefined> {
  latestRelease ??= fetch(RELEASES_API, { headers: { Accept: 'application/vnd.github+json' } })
    .then(r => (r.ok ? r.json() : Promise.reject(new Error(`GitHub ${r.status}`))))
    .then((data: GitHubRelease[]) => (Array.isArray(data) ? data : []).find(r => r.tag_name.startsWith('client-v') && !r.draft && !r.prerelease))
  latestRelease.catch(() => {
    latestRelease = undefined // a failed call can be tried again on the next mount
  })
  return latestRelease
}

export default function BrainHubDownloads() {
  const [release, setRelease] = useState<GitHubRelease | undefined>()
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let live = true
    loadLatestRelease()
      .then(latest => {
        if (!live) return
        if (latest) setRelease(latest)
        else setFailed(true)
      })
      .catch(() => {
        if (live) setFailed(true)
      })
    return () => {
      live = false
    }
  }, [])

  // Until the release loads (or if GitHub does not answer), each button opens the release page.
  const assetUrl = (match: Match) => release?.assets.find(a => match(a.name))?.browser_download_url ?? RELEASES

  return (
    <div>
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        {downloads.map(d => (
          <div key={d.system} className="flex flex-col gap-2">
            <a
              href={assetUrl(d.primary)}
              className="inline-flex items-center justify-center px-4 py-3 text-sm font-medium text-white bg-primary-500 hover:bg-primary-600 rounded-md transition-colors"
              rel="noopener noreferrer"
            >
              Download for {d.system}
            </a>
            {d.others.length > 0 && (
              <p className="text-xs text-dark-text-tertiary text-center">
                Also:{' '}
                {d.others.map((o, i) => (
                  <span key={o.label}>
                    {i > 0 && ', '}
                    <a className={linkClass} href={assetUrl(o.match)} rel="noopener noreferrer">{o.label}</a>
                  </span>
                ))}
              </p>
            )}
          </div>
        ))}
      </div>
      <p className="mt-4 text-xs text-dark-text-tertiary" aria-live="polite">
        {release && `Desktop app version ${release.tag_name.replace(/^client-v/, '')}. `}
        {failed && (
          <span role="status">
            The latest version could not be found. The buttons open the <a className={linkClass} href={RELEASES}>release page</a>.{' '}
          </span>
        )}
        The builds are not signed. macOS and Windows can show a warning at the first start. Read the{' '}
        <a className={linkClass} href={`${BRAIN_HUB_REPO}/blob/main/docs/desktop-app.md`} target="_blank" rel="noopener noreferrer">install notes</a>.
      </p>
    </div>
  )
}
