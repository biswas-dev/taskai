// GoWikiMermaid — mermaid.js rendering helper for go-wiki.
// mermaid is ~3.5 MB of JavaScript, and most pages in the app contain no
// diagrams at all, so it is fetched lazily the first time a diagram actually
// shows up rather than on every page load.
// Usage: window.GoWikiMermaid.run(rootElement) after injecting wiki HTML into the DOM.
;(function () {
  'use strict'

  var MERMAID_SRC = 'https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.min.js'

  var _configured = false
  var _loading = null
  var _pending = []

  function configure() {
    if (_configured || typeof window.mermaid === 'undefined') return
    window.mermaid.initialize({ startOnLoad: false, theme: 'dark' })
    _configured = true
  }

  // load resolves once window.mermaid is available. Repeat calls share one request.
  function load() {
    if (typeof window.mermaid !== 'undefined') return Promise.resolve()
    if (_loading) return _loading
    _loading = new Promise(function (resolve, reject) {
      var existing = document.querySelector('script[data-gowiki-mermaid]')
      if (existing) {
        existing.addEventListener('load', function () { resolve() })
        existing.addEventListener('error', reject)
        return
      }
      var s = document.createElement('script')
      s.src = MERMAID_SRC
      s.async = true
      s.setAttribute('data-gowiki-mermaid', '')
      s.onload = function () { resolve() }
      s.onerror = reject
      document.head.appendChild(s)
    })
    return _loading
  }

  function render(rootEl) {
    configure()
    var nodes = Array.from(rootEl.querySelectorAll('div.mermaid:not([data-processed])'))
    if (!nodes.length) return
    window.mermaid.run({ nodes: nodes })
  }

  // run renders any unprocessed mermaid diagrams within rootEl.
  // Safe to call repeatedly — mermaid marks processed elements with data-processed.
  function run(rootEl) {
    if (!rootEl) return
    if (!rootEl.querySelector('div.mermaid:not([data-processed])')) return

    if (typeof window.mermaid !== 'undefined') {
      render(rootEl)
      return
    }

    // Diagrams queued while the library downloads are rendered once it lands.
    // The element may have been replaced by then, so re-check it is still attached.
    _pending.push(rootEl)
    load().then(function () {
      var queued = _pending
      _pending = []
      queued.forEach(function (el) {
        if (el.isConnected) render(el)
      })
    }).catch(function () {
      _pending = []
    })
  }

  window.GoWikiMermaid = { run: run }
})()
