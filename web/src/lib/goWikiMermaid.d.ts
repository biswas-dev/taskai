// Type declarations for window.GoWikiMermaid (loaded from /mermaid-init.js)

interface GoWikiMermaidAPI {
  /** Render all unprocessed <div class="mermaid"> elements within rootEl. */
  run(rootEl: HTMLElement | null): void
}

declare global {
  interface Window {
    GoWikiMermaid?: GoWikiMermaidAPI
  }
}

export {}
