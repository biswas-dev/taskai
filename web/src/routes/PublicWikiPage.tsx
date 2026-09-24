import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { apiClient, type PublicWikiPage as PublicWikiPageData } from '../lib/api'

type PageState =
  | { kind: 'loading' }
  | { kind: 'ready'; page: PublicWikiPageData }
  | { kind: 'inactive' }

function formatDate(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' })
}

/**
 * Read-only view of a wiki page shared through a public link. Needs no
 * sign-in; the HTML is sanitized by the server before it is sent.
 */
export default function PublicWikiPage() {
  const { token = '' } = useParams<{ token: string }>()
  const [state, setState] = useState<PageState>({ kind: 'loading' })

  useEffect(() => {
    let cancelled = false
    setState({ kind: 'loading' })
    if (!token) {
      setState({ kind: 'inactive' })
      return
    }
    apiClient.getPublicWikiPage(token)
      .then((page) => { if (!cancelled) setState({ kind: 'ready', page }) })
      .catch(() => { if (!cancelled) setState({ kind: 'inactive' }) })
    return () => { cancelled = true }
  }, [token])

  return (
    <div className="min-h-screen flex flex-col bg-dark-bg-base text-dark-text-primary">
      <main className="flex-1 w-full max-w-3xl mx-auto px-4 sm:px-6 py-10">
        {state.kind === 'loading' && (
          <div className="flex justify-center py-24" role="status" aria-live="polite">
            <span className="text-sm text-dark-text-tertiary">Loading page…</span>
          </div>
        )}

        {state.kind === 'inactive' && (
          <div className="text-center py-24">
            <h1 className="text-xl font-semibold text-dark-text-primary mb-2">This link is no longer active</h1>
            <p className="text-sm text-dark-text-secondary">
              The page may have been unshared or removed. Ask the person who sent it for a new link.
            </p>
          </div>
        )}

        {state.kind === 'ready' && (
          <article>
            <header className="mb-8 pb-6 border-b border-dark-border-subtle">
              <p className="text-xs uppercase tracking-wide text-dark-text-tertiary mb-2">{state.page.project_name}</p>
              <h1 className="text-3xl font-bold text-dark-text-primary tracking-tight">{state.page.title}</h1>
              {formatDate(state.page.updated_at) && (
                <p className="mt-2 text-sm text-dark-text-tertiary">Updated {formatDate(state.page.updated_at)}</p>
              )}
            </header>
            <div
              className="prose prose-invert max-w-none"
              data-testid="public-wiki-content"
              dangerouslySetInnerHTML={{ __html: state.page.html }}
            />
          </article>
        )}
      </main>

      <footer className="py-6 text-center text-xs text-dark-text-tertiary border-t border-dark-border-subtle">
        Shared from{' '}
        <Link to="/" className="text-primary-400 hover:text-primary-300">TaskAI</Link>
      </footer>
    </div>
  )
}
