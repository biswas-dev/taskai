import type { WikiPage } from '../lib/api'

type IconKind = 'lock' | 'globe' | 'people'

const PATHS: Record<IconKind, string> = {
  lock: 'M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z',
  globe: 'M12 21a9 9 0 100-18 9 9 0 000 18zm0 0c2.485 0 4.5-4.03 4.5-9S14.485 3 12 3 7.5 7.03 7.5 12s2.015 9 4.5 9zM3.6 9h16.8M3.6 15h16.8',
  people: 'M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z',
}

/** A small outline icon used to signal who can see a wiki page. */
export function WikiIcon({ kind, className = 'w-4 h-4' }: Readonly<{ kind: IconKind; className?: string }>) {
  return (
    <svg className={className} fill="none" stroke="currentColor" strokeWidth={1.8} viewBox="0 0 24 24" aria-hidden="true">
      <path strokeLinecap="round" strokeLinejoin="round" d={PATHS[kind]} />
    </svg>
  )
}

/** Lock / globe markers shown next to a page title. Renders nothing for ordinary pages. */
export function WikiVisibilityBadges({ page }: Readonly<{ page: Pick<WikiPage, 'visibility' | 'is_public'> }>) {
  const restricted = page.visibility === 'restricted'
  if (!restricted && !page.is_public) return null
  return (
    <span className="shrink-0 flex items-center gap-0.5 text-dark-text-tertiary">
      {restricted && (
        <span role="img" aria-label="Protected" title="Protected">
          <WikiIcon kind="lock" className="w-3 h-3" />
        </span>
      )}
      {page.is_public && (
        <span role="img" aria-label="Public link" title="Public link">
          <WikiIcon kind="globe" className="w-3 h-3" />
        </span>
      )}
    </span>
  )
}
