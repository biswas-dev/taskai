import { Link, useLocation } from 'react-router-dom'
import BrainHubDownloads from '../components/BrainHubDownloads'
import BrainHubFeatures from '../components/BrainHubFeatures'

const REPO = 'https://github.com/anchoo2kewl/llm-brain'
const linkClass = 'text-primary-400 hover:text-primary-300 underline underline-offset-2'

export default function BrainHub() {
  const inApp = useLocation().pathname.startsWith('/app')

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
            Brain Hub (llm-brain) is a hub for your coding agents. It runs Claude Code, Codex and Antigravity on every machine that you own. With TaskAI, your agents share one memory, one tracker and one record of work.
          </p>
        </header>

        <section aria-labelledby="brain-hub-download" className="mt-10 p-5 sm:p-6 rounded-xl bg-dark-bg-primary border border-dark-border-subtle">
          <h2 id="brain-hub-download" className="text-xl font-semibold">Download the desktop app</h2>
          <p className="mt-2 mb-5 text-sm text-dark-text-tertiary">
            The app connects to your hub. To install the hub itself, read{' '}
            <a className={linkClass} href={`${REPO}/blob/main/docs/getting-started.md`} target="_blank" rel="noopener noreferrer">Getting started</a>.
          </p>
          <BrainHubDownloads />
        </section>

        <section aria-labelledby="brain-hub-why" className="mt-12">
          <h2 id="brain-hub-why" className="text-2xl font-semibold mb-6">How Brain Hub and TaskAI make development faster</h2>
          <BrainHubFeatures />
        </section>

        <section aria-labelledby="brain-hub-connect" className="mt-12">
          <h2 id="brain-hub-connect" className="text-2xl font-semibold">Connect Brain Hub to TaskAI</h2>
          <ol className="mt-4 list-decimal pl-5 space-y-2 text-dark-text-secondary">
            <li>Install the hub and the desktop app.</li>
            <li>In TaskAI, open Settings and make an API key.</li>
            <li>In the hub, open Settings and add the key. You can also run <code className="px-1 rounded bg-dark-bg-tertiary text-sm">hub memory key set</code>.</li>
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
