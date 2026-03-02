import '@testing-library/jest-dom'

// Polyfill ResizeObserver for Headless UI in jsdom
globalThis.ResizeObserver ??= class ResizeObserver {
  // No-op stubs required for jsdom environment
  observe() { /* no-op */ }
  unobserve() { /* no-op */ }
  disconnect() { /* no-op */ }
}

// Mock import.meta.env for tests
Object.defineProperty(import.meta, 'env', {
  value: {
    VITE_API_URL: 'http://localhost:8080',
    PROD: false,
    DEV: true,
    MODE: 'test',
  },
})
