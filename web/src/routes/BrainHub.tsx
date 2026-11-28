import { useEffect, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'

const REPO = 'https://github.com/anchoo2kewl/llm-brain'
const RELEASES = `${REPO}/releases`
const RELEASES_API = 'https://api.github.com/repos/anchoo2kewl/llm-brain/releases?per_page=30'

interface GitHubRelease {
  tag_name: string
  draft: boolean
  prerelease: boolean
  assets: { name: string; browser_download_url: string }[]
}

interface Download {
  system: string
  primary: { label: string; match: (name: string) => boolean }
  others: { label: string; match: (name: string) => boolean }[]
}

// Asset names carry the version (docs/desktop-app.md in llm-brain), so the page reads the newest client-v release.
const downloads: Download[] = [
  { system: 'macOS', primary: { label: 'Download for macOS', match: n => n.startsWith('BrainHub-mac-') && n.endsWith('.zip') }, others: [] },
  {
    system: 'Windows',
    primary: { label: 'Download for Windows', match: n => n.endsWith('_x64-setup.exe') },
    others: [{ label: '.msi', match: n => n.endsWith('.msi') }],
  },
  {
    system: 'Linux',
    primary: { label: 'Download for Linux', match: n => n.endsWith('.AppImage') },
    others: [
      { label: '.deb', match: n => n.endsWith('.deb') },
      { label: '.rpm', match: n => n.endsWith('.rpm') },
    ],
  },
]

function latestClientRelease(releases: GitHubRelease[]): GitHubRelease | undefined {
  return releases.find(r => r.tag_name.startsWith('client-v') && !r.draft && !r.prerelease)
}

const features = [
  {
    title: 'Shared agent memory',
    body: 'Agents save lessons, decisions and gotchas. Brain Hub sends each memory to TaskAI. Agents on your other machines, and you in the TaskAI app, can search it.',
  },
  {
    title: 'Every session on every machine',
    body: 'The hub shows each Claude Code, Codex and Gemini session from each machine that you own. Search every message. Open live terminals in the browser.',
  },
  {
    title: 'Issues to merged PRs',
    body: 'The fleet takes the issues that are assigned to you, does the work and opens PRs. It reviews the PRs that ask for you. A second LLM cross-reviews each change.',
  },
  {
    title: 'Tracker and activity',
    body: 'Agents read and update TaskAI tasks, comments and wiki pages through the TaskAI MCP server. The activity log of each project shows what each agent did.',
  },
  {
    title: 'Encrypted sync (in progress)',
    body: 'TaskAI now has a storage API for encrypted objects. Brain Hub will use it to sync skills, rules and settings between your machines. TaskAI keeps only encrypted bytes and cannot read them.',
  },
]

const linkClass = 'text-primary-400 hover:text-primary-300 underline underline-offset-2'

export default function BrainHub() {
  const inApp = useLocation().pathname.startsWith('/app')
  const [release, setRelease] = useState<GitHubRelease | undefined>()
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    const ctrl = new AbortController()
    fetch(RELEASES_API, { signal: ctrl.signal, headers: { Accept: 'application/vnd.github+json' } })
      .then(r => (r.ok ? r.json() : Promise.reject(new Error(`GitHub ${r.status}`))))
      .then((data: GitHubRelease[]) => {
        const latest = latestClientRelease(Array.isArray(data) ? data : [])
        if (latest) setRelease(latest)
        else setFailed(true)
      })
      .catch((err: unknown) => {
        if (!(err instanceof DOMException && err.name === 'AbortError')) setFailed(true)
      })
    return () => ctrl.abort()
  }, [])

  // Until the release loads (or if GitHub does not answer), each button opens the release page.
  const assetUrl = (match: (name: string) => boolean) => release?.assets.find(a => match(a.name))?.browser_download_url

  return (
    <div className={inApp ? 'text-dark-text-primary' : 'min-h-screen bg-dark-bg-base text-dark-text-primary'}>
      {!inApp && (
        <nav className="border-b border-dark-border-subtle">
          <div className="max-w-5xl mx-auto px-4 sm:px-6 h-16 flex items-center justify-between">
            <Link to="/" className="flex items-center gap-2.5">
              <img src="/logo.svg" alt="TaskAI" className="w-7 h-7" />
              <span className="text-base font-semibold tracking-tight">TaskAI</span>
            </Link>
            <Link to="/signup" className="text-sm text-dark-text-tertiary hover:text-dark-text-primary transition-colors">
              Get started free
            </Link>
          </div>
        </nav>
      )}

      <main className="max-w-5xl mx-auto px-4 sm:px-6 py-10 md:py-16">
        <header className="max-w-3xl">
          <p className="text-sm font-medium text-primary-400">Add-on for TaskAI</p>
          <h1 className="mt-2 text-3xl md:text-5xl font-bold tracking-tight">Brain Hub</h1>
          <p className="mt-4 text-lg text-dark-text-secondary leading-relaxed">
            Brain Hub (llm-brain) is a hub for your coding agents. It works with TaskAI, so your agents share one memory, one tracker and one record of work on all of your machines.
          </p>
        </header>

        <section aria-labelledby="brain-hub-download" className="mt-10 p-5 sm:p-6 rounded-xl bg-dark-bg-primary border border-dark-border-subtle">
          <h2 id="brain-hub-download" className="text-xl font-semibold">Download the desktop app</h2>
          <p className="mt-2 text-sm text-dark-text-tertiary">
            {release ? `Version ${release.tag_name.replace(/^client-v/, '')}. ` : ''}
            The app connects to your hub. To install the hub itself, read{' '}
            <a className={linkClass} href={`${REPO}/blob/main/docs/getting-started.md`} target="_blank" rel="noopener noreferrer">Getting started</a>.
          </p>
          <div className="mt-5 grid grid-cols-1 sm:grid-cols-3 gap-4">
            {downloads.map(d => (
              <div key={d.system} className="flex flex-col gap-2">
                <a
                  href={assetUrl(d.primary.match) ?? RELEASES}
                  className="inline-flex items-center justify-center px-4 py-3 text-sm font-medium text-white bg-primary-500 hover:bg-primary-600 rounded-md transition-colors"
                  rel="noopener noreferrer"
                >
                  {d.primary.label}
                </a>
                {d.others.length > 0 && (
                  <p className="text-xs text-dark-text-tertiary text-center">
                    Also:{' '}
                    {d.others.map((o, i) => (
                      <span key={o.label}>
                        {i > 0 && ', '}
                        <a className={linkClass} href={assetUrl(o.match) ?? RELEASES} rel="noopener noreferrer">{o.label}</a>
                      </span>
                    ))}
                  </p>
                )}
              </div>
            ))}
          </div>
          {failed && (
            <p role="status" className="mt-4 text-sm text-dark-text-tertiary">
              The latest version could not be found. The buttons open the{' '}
              <a className={linkClass} href={RELEASES}>release page</a>.
            </p>
          )}
          <p className="mt-4 text-xs text-dark-text-quaternary">
            The builds are not signed. macOS and Windows can show a warning at the first start. Read the{' '}
            <a className={linkClass} href={`${REPO}/blob/main/docs/desktop-app.md`} target="_blank" rel="noopener noreferrer">install notes</a>.
          </p>
        </section>

        <section aria-labelledby="brain-hub-why" className="mt-12">
          <h2 id="brain-hub-why" className="text-2xl font-semibold">How Brain Hub and TaskAI make development faster</h2>
          <div className="mt-6 grid grid-cols-1 md:grid-cols-2 gap-4">
            {features.map(f => (
              <div key={f.title} className="p-5 rounded-xl bg-dark-bg-primary border border-dark-border-subtle">
                <h3 className="font-semibold">{f.title}</h3>
                <p className="mt-2 text-sm text-dark-text-tertiary leading-relaxed">{f.body}</p>
              </div>
            ))}
          </div>
        </section>

        <section aria-labelledby="brain-hub-connect" className="mt-12">
          <h2 id="brain-hub-connect" className="text-2xl font-semibold">Connect Brain Hub to TaskAI</h2>
          <ol className="mt-4 list-decimal pl-5 space-y-2 text-dark-text-secondary">
            <li>Install the hub and the desktop app.</li>
            <li>In TaskAI, open Settings and make an API key.</li>
            <li>
              Put the key in <code className="px-1 rounded bg-dark-bg-tertiary text-sm">~/.config/llm-brain/taskai.env</code> as{' '}
              <code className="px-1 rounded bg-dark-bg-tertiary text-sm">TASKAI_API_KEY</code>.
            </li>
          </ol>
        </section>

        <section aria-labelledby="brain-hub-license" className="mt-12 p-5 sm:p-6 rounded-xl border border-dark-border-medium">
          <h2 id="brain-hub-license" className="text-xl font-semibold">License</h2>
          <p className="mt-2 text-dark-text-secondary leading-relaxed">
            Brain Hub (llm-brain) is open source under the MIT license. Read the{' '}
            <a className={linkClass} href={REPO} target="_blank" rel="noopener noreferrer">source code</a> and the{' '}
            <a className={linkClass} href={`${REPO}/blob/main/LICENSE`} target="_blank" rel="noopener noreferrer">MIT license</a>.
          </p>
          <p className="mt-2 text-dark-text-secondary leading-relaxed">
            TaskAI is a proprietary service. It is not open source.
          </p>
        </section>
      </main>
    </div>
  )
}
