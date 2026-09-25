import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'
import postcss from 'postcss'
import tailwindcss, { type Config } from 'tailwindcss'
import { describe, expect, it } from 'vitest'

// Wiki pages (editor preview and public links) render server HTML inside
// `prose prose-invert`. Inline code arrives as <code>x</code>; the stylesheet
// must not add backticks around it.
async function proseCSS(): Promise<string> {
  // The config is plain JS with no type declarations, so load it dynamically.
  const configURL = pathToFileURL(resolve(__dirname, '../../tailwind.config.js')).href
  const { default: tailwindConfig } = (await import(/* @vite-ignore */ configURL)) as { default: Config }
  const config: Config = {
    ...tailwindConfig,
    content: [{ raw: '<div class="prose prose-invert p-4"><p><strong>Keep:</strong> the <code>x</code></p></div>', extension: 'html' }],
  }
  const result = await postcss([tailwindcss(config)]).process('@tailwind components; @tailwind utilities;', { from: undefined })
  return result.css
}

describe('wiki prose inline code', () => {
  it('does not wrap inline code in backtick pseudo-elements', async () => {
    const css = await proseCSS()
    expect(css).toContain('.prose')
    expect(css).not.toMatch(/content:\s*"`"/)
    expect(css).toMatch(/:where\(code\)[^{]*::before\s*\{\s*content:\s*none/)
    expect(css).toMatch(/:where\(code\)[^{]*::after\s*\{\s*content:\s*none/)
  })
})
