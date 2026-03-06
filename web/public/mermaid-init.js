// GoWikiMermaid — mermaid.js rendering helper for go-wiki.
// Requires mermaid.js (https://mermaid.js.org/) to be loaded before this script.
// Usage: window.GoWikiMermaid.run(rootElement) after injecting wiki HTML into the DOM.
;(function () {
  'use strict'

  var _configured = false

  function configure() {
    if (_configured || typeof window.mermaid === 'undefined') return
    window.mermaid.initialize({ startOnLoad: false, theme: 'dark' })
    _configured = true
  }

  // run renders any unprocessed mermaid diagrams within rootEl.
  // Safe to call repeatedly — mermaid marks processed elements with data-processed.
  function run(rootEl) {
    if (!rootEl || typeof window.mermaid === 'undefined') return
    configure()
    var nodes = Array.from(rootEl.querySelectorAll('div.mermaid:not([data-processed])'))
    if (!nodes.length) return
    window.mermaid.run({ nodes: nodes })
  }

  window.GoWikiMermaid = { run: run }
})()
