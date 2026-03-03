import {
  escapeHtml,
  unescapeHtml,
  buildImageMarkup,
  findImagesInContent,
  detectImageSize,
  findDrawsInContent,
  mapYjsStatus,
  findDrawShortcodeAtPosition,
  shouldSaveContent,
  getSyncStatusColor,
  getSaveStatusColor,
  getSaveStatusText,
  getSaveStatusTextColor,
  buildDrawShortcode,
  insertMarkupAtCursor,
  clearSavedStatus,
  insertAtCursorPure,
  insertLinePure,
  escapeRegExp,
  fetchDrawings,
  createDrawing,
  renameDrawing,
  deleteDrawing,
  deleteDrawings,
  fetchPreview,
} from './WikiEditor.helpers'

describe('WikiEditor helpers', () => {
  // ── escapeHtml / unescapeHtml ─────────────────────────────────

  describe('escapeHtml', () => {
    it('escapes ampersands', () => {
      expect(escapeHtml('a & b')).toBe('a &amp; b')
    })

    it('escapes angle brackets', () => {
      expect(escapeHtml('<div>')).toBe('&lt;div&gt;')
    })

    it('escapes double quotes', () => {
      expect(escapeHtml('"hello"')).toBe('&quot;hello&quot;')
    })

    it('escapes all special chars together', () => {
      expect(escapeHtml('<a href="x">&')).toBe('&lt;a href=&quot;x&quot;&gt;&amp;')
    })

    it('returns plain text unchanged', () => {
      expect(escapeHtml('hello world')).toBe('hello world')
    })
  })

  describe('unescapeHtml', () => {
    it('unescapes ampersands', () => {
      expect(unescapeHtml('a &amp; b')).toBe('a & b')
    })

    it('unescapes angle brackets', () => {
      expect(unescapeHtml('&lt;div&gt;')).toBe('<div>')
    })

    it('unescapes double quotes', () => {
      expect(unescapeHtml('&quot;hello&quot;')).toBe('"hello"')
    })

    it('is the inverse of escapeHtml', () => {
      const original = '<a href="test">&foo'
      expect(unescapeHtml(escapeHtml(original))).toBe(original)
    })
  })

  // ── buildImageMarkup ──────────────────────────────────────────

  describe('buildImageMarkup', () => {
    it('returns markdown for large size with no caption', () => {
      const result = buildImageMarkup('http://img.com/a.png', 'My Image', '', 'l')
      expect(result).toBe('![My Image](http://img.com/a.png)')
    })

    it('returns figure HTML for large size with caption', () => {
      const result = buildImageMarkup('http://img.com/a.png', 'Alt', 'Caption text', 'l')
      expect(result).toContain('<figure')
      expect(result).toContain('<figcaption>Caption text</figcaption>')
      expect(result).toContain('width:100%')
    })

    it('returns figure HTML for medium size', () => {
      const result = buildImageMarkup('http://img.com/a.png', 'Alt', '', 'm')
      expect(result).toContain('<figure')
      expect(result).toContain('max-width:75%')
    })

    it('returns figure HTML for small size', () => {
      const result = buildImageMarkup('http://img.com/a.png', 'Alt', '', 's')
      expect(result).toContain('<figure')
      expect(result).toContain('max-width:50%')
    })

    it('escapes alt text in HTML output', () => {
      const result = buildImageMarkup('http://img.com/a.png', '<script>', '', 'm')
      expect(result).toContain('&lt;script&gt;')
      expect(result).not.toContain('<script>')
    })

    it('escapes caption in HTML output', () => {
      const result = buildImageMarkup('http://img.com/a.png', 'Alt', '<b>bold</b>', 'm')
      expect(result).toContain('&lt;b&gt;bold&lt;/b&gt;')
    })

    it('includes lightbox data attributes', () => {
      const result = buildImageMarkup('http://img.com/a.png', 'Alt', '', 'm')
      expect(result).toContain('data-lightbox="article-images"')
      expect(result).toContain('data-title="Alt"')
    })
  })

  // ── findImagesInContent ───────────────────────────────────────

  describe('findImagesInContent', () => {
    it('finds markdown images', () => {
      const content = 'Hello ![Alt text](http://img.com/pic.png) world'
      const images = findImagesInContent(content)
      expect(images).toHaveLength(1)
      expect(images[0].url).toBe('http://img.com/pic.png')
      expect(images[0].alt).toBe('Alt text')
      expect(images[0].caption).toBe('')
    })

    it('finds figure elements', () => {
      const content = '<figure style="text-align:center"><a href="url"><img src="http://img.com/pic.png" alt="My Alt" style="max-width:75%"/></a><figcaption>Caption</figcaption></figure>'
      const images = findImagesInContent(content)
      expect(images).toHaveLength(1)
      expect(images[0].url).toBe('http://img.com/pic.png')
      expect(images[0].alt).toBe('My Alt')
      expect(images[0].caption).toBe('Caption')
    })

    it('finds multiple images', () => {
      const content = '![First](http://a.com/1.png)\n\n![Second](http://a.com/2.png)'
      const images = findImagesInContent(content)
      expect(images).toHaveLength(2)
      expect(images[0].alt).toBe('First')
      expect(images[1].alt).toBe('Second')
    })

    it('returns empty array for no images', () => {
      expect(findImagesInContent('Just some text')).toHaveLength(0)
    })

    it('does not double-count images inside figures', () => {
      const content = '<figure><a href="url"><img src="http://img.com/pic.png" alt="Alt"/></a></figure>'
      const images = findImagesInContent(content)
      expect(images).toHaveLength(1)
    })

    it('sorts images by position', () => {
      const content = '![B](http://b.png) some text ![A](http://a.png)'
      const images = findImagesInContent(content)
      expect(images[0].alt).toBe('B')
      expect(images[1].alt).toBe('A')
    })
  })

  // ── detectImageSize ───────────────────────────────────────────

  describe('detectImageSize', () => {
    it('returns l for plain markdown', () => {
      expect(detectImageSize('![Alt](url)')).toBe('l')
    })

    it('returns s for 50% width figure', () => {
      expect(detectImageSize('<figure><img style="max-width: 50%; height:auto;"/></figure>')).toBe('s')
    })

    it('returns m for 75% width figure', () => {
      expect(detectImageSize('<figure><img style="max-width: 75%; height:auto;"/></figure>')).toBe('m')
    })

    it('returns l for figure without size styles', () => {
      expect(detectImageSize('<figure><img style="width:100%"/></figure>')).toBe('l')
    })
  })

  // ── findDrawsInContent ────────────────────────────────────────

  describe('findDrawsInContent', () => {
    it('finds basic draw shortcode', () => {
      const draws = findDrawsInContent('Some text [draw:abc123:edit] more text')
      expect(draws).toHaveLength(1)
      expect(draws[0].id).toBe('abc123')
      expect(draws[0].size).toBe('m')
      expect(draws[0].zoom).toBe('fit')
    })

    it('finds draw shortcode with size', () => {
      const draws = findDrawsInContent('[draw:abc:edit:s]')
      expect(draws).toHaveLength(1)
      expect(draws[0].size).toBe('s')
    })

    it('finds draw shortcode with zoom', () => {
      const draws = findDrawsInContent('[draw:abc:edit:m:z150%]')
      expect(draws).toHaveLength(1)
      expect(draws[0].zoom).toBe('150%')
    })

    it('finds multiple draws', () => {
      const content = '[draw:a:edit]\n[draw:b:edit:l]\n[draw:c:edit:s:z200%]'
      const draws = findDrawsInContent(content)
      expect(draws).toHaveLength(3)
      expect(draws[0].id).toBe('a')
      expect(draws[1].id).toBe('b')
      expect(draws[2].id).toBe('c')
    })

    it('returns empty for no draws', () => {
      expect(findDrawsInContent('No draws here')).toHaveLength(0)
    })
  })

  // ── mapYjsStatus ──────────────────────────────────────────────

  describe('mapYjsStatus', () => {
    it('maps connected', () => {
      expect(mapYjsStatus('connected')).toBe('connected')
    })

    it('maps disconnected', () => {
      expect(mapYjsStatus('disconnected')).toBe('disconnected')
    })

    it('maps connecting', () => {
      expect(mapYjsStatus('connecting')).toBe('connecting')
    })

    it('maps unknown to connecting', () => {
      expect(mapYjsStatus('whatever')).toBe('connecting')
    })
  })

  // ── findDrawShortcodeAtPosition ───────────────────────────────

  describe('findDrawShortcodeAtPosition', () => {
    it('returns draw id when cursor is inside shortcode', () => {
      const text = 'Hello [draw:abc123:edit] world'
      expect(findDrawShortcodeAtPosition(text, 10)).toBe('abc123')
    })

    it('returns null when cursor is outside shortcode', () => {
      const text = 'Hello [draw:abc123:edit] world'
      expect(findDrawShortcodeAtPosition(text, 2)).toBeNull()
    })

    it('returns null for no shortcodes', () => {
      expect(findDrawShortcodeAtPosition('no draws', 3)).toBeNull()
    })

    it('handles cursor at start of shortcode', () => {
      const text = '[draw:abc:edit]'
      expect(findDrawShortcodeAtPosition(text, 0)).toBe('abc')
    })

    it('handles cursor at end of shortcode', () => {
      const text = '[draw:abc:edit]'
      expect(findDrawShortcodeAtPosition(text, 15)).toBe('abc')
    })
  })

  // ── shouldSaveContent ─────────────────────────────────────────

  describe('shouldSaveContent', () => {
    it('returns true when dirty and content changed', () => {
      expect(shouldSaveContent(true, 'new', 'old')).toBe(true)
    })

    it('returns false when not dirty', () => {
      expect(shouldSaveContent(false, 'new', 'old')).toBe(false)
    })

    it('returns false when content unchanged', () => {
      expect(shouldSaveContent(true, 'same', 'same')).toBe(false)
    })

    it('returns false when not dirty and content unchanged', () => {
      expect(shouldSaveContent(false, 'same', 'same')).toBe(false)
    })
  })

  // ── Status color/text helpers ─────────────────────────────────

  describe('getSyncStatusColor', () => {
    it('returns green for connected', () => {
      expect(getSyncStatusColor('connected')).toBe('bg-green-500')
    })

    it('returns yellow for connecting', () => {
      expect(getSyncStatusColor('connecting')).toBe('bg-yellow-500')
    })

    it('returns red for disconnected', () => {
      expect(getSyncStatusColor('disconnected')).toBe('bg-red-500')
    })
  })

  describe('getSaveStatusColor', () => {
    it('returns yellow for saving', () => {
      expect(getSaveStatusColor('saving', 'connected')).toBe('bg-yellow-500')
    })

    it('returns green for saved', () => {
      expect(getSaveStatusColor('saved', 'connected')).toBe('bg-green-500')
    })

    it('returns red for error', () => {
      expect(getSaveStatusColor('error', 'connected')).toBe('bg-red-500')
    })

    it('falls back to sync color for idle', () => {
      expect(getSaveStatusColor('idle', 'connected')).toBe('bg-green-500')
      expect(getSaveStatusColor('idle', 'disconnected')).toBe('bg-red-500')
    })
  })

  describe('getSaveStatusText', () => {
    it('returns Saving... for saving', () => {
      expect(getSaveStatusText('saving', true)).toBe('Saving...')
    })

    it('returns Saved for saved', () => {
      expect(getSaveStatusText('saved', true)).toBe('Saved')
    })

    it('returns Save failed for error', () => {
      expect(getSaveStatusText('error', true)).toBe('Save failed')
    })

    it('returns autosave state for idle', () => {
      expect(getSaveStatusText('idle', true)).toBe('Autosave on')
      expect(getSaveStatusText('idle', false)).toBe('Autosave off')
    })
  })

  describe('getSaveStatusTextColor', () => {
    it('returns yellow for saving', () => {
      expect(getSaveStatusTextColor('saving')).toBe('text-yellow-400')
    })

    it('returns green for saved', () => {
      expect(getSaveStatusTextColor('saved')).toBe('text-green-400')
    })

    it('returns red for error', () => {
      expect(getSaveStatusTextColor('error')).toBe('text-red-400')
    })

    it('returns tertiary for idle', () => {
      expect(getSaveStatusTextColor('idle')).toBe('text-dark-text-tertiary')
    })
  })

  // ── buildDrawShortcode ────────────────────────────────────────

  describe('buildDrawShortcode', () => {
    it('builds default shortcode (m size, fit zoom)', () => {
      expect(buildDrawShortcode('abc', 'm', 'fit')).toBe('[draw:abc:edit]')
    })

    it('includes size when not m', () => {
      expect(buildDrawShortcode('abc', 's', 'fit')).toBe('[draw:abc:edit:s]')
      expect(buildDrawShortcode('abc', 'l', 'fit')).toBe('[draw:abc:edit:l]')
    })

    it('includes zoom when not fit', () => {
      expect(buildDrawShortcode('abc', 'm', '150%')).toBe('[draw:abc:edit:z150%]')
    })

    it('includes both size and zoom', () => {
      expect(buildDrawShortcode('abc', 'l', '200%')).toBe('[draw:abc:edit:l:z200%]')
    })
  })

  // ── insertMarkupAtCursor ──────────────────────────────────────

  describe('insertMarkupAtCursor', () => {
    it('inserts at cursor when textarea is provided', () => {
      const textarea = {
        selectionStart: 5,
        selectionEnd: 5,
      } as HTMLTextAreaElement
      const result = insertMarkupAtCursor(textarea, 'Hello world', '**bold**')
      expect(result.newContent).toBe('Hello**bold** world')
      expect(result.focusPos).toBe(13)
    })

    it('replaces selection when range is selected', () => {
      const textarea = {
        selectionStart: 5,
        selectionEnd: 11,
      } as HTMLTextAreaElement
      const result = insertMarkupAtCursor(textarea, 'Hello world!', '**bold**')
      expect(result.newContent).toBe('Hello**bold**!')
      expect(result.focusPos).toBe(13)
    })

    it('appends with newline when no textarea', () => {
      const result = insertMarkupAtCursor(null, 'Hello', 'markup')
      expect(result.newContent).toBe('Hello\nmarkup\n')
      expect(result.focusPos).toBeNull()
    })

    it('skips extra newline when content ends with one', () => {
      const result = insertMarkupAtCursor(null, 'Hello\n', 'markup')
      expect(result.newContent).toBe('Hello\nmarkup\n')
    })
  })

  // ── clearSavedStatus ──────────────────────────────────────────

  describe('clearSavedStatus', () => {
    it('clears saved to idle', () => {
      expect(clearSavedStatus('saved')).toBe('idle')
    })

    it('keeps saving as is', () => {
      expect(clearSavedStatus('saving')).toBe('saving')
    })

    it('keeps error as is', () => {
      expect(clearSavedStatus('error')).toBe('error')
    })

    it('keeps idle as idle', () => {
      expect(clearSavedStatus('idle')).toBe('idle')
    })
  })

  // ── insertAtCursorPure ──────────────────────────────────────

  describe('insertAtCursorPure', () => {
    it('wraps selected text with before/after', () => {
      const result = insertAtCursorPure('Hello world', 6, 11, '**', '**')
      expect(result.newContent).toBe('Hello **world**')
      expect(result.cursorStart).toBe(15)
      expect(result.cursorEnd).toBe(15)
    })

    it('inserts placeholder "text" when nothing is selected', () => {
      const result = insertAtCursorPure('Hello world', 5, 5, '**', '**')
      // selected = '' (empty), insert = '**' + 'text' + '**' = '**text**'
      // newContent = 'Hello' + '**text**' + ' world' = 'Hello**text** world'
      expect(result.newContent).toBe('Hello**text** world')
      // cursorStart = 5 + 2 = 7, cursorEnd = 5 + 2 + 4 = 11
      expect(result.cursorStart).toBe(7)
      expect(result.cursorEnd).toBe(11)
    })

    it('replaces selection range with wrapped text', () => {
      const result = insertAtCursorPure('abcdef', 2, 4, '[', '](url)')
      // selected = 'cd', insert = '[cd](url)'
      // newContent = 'ab' + '[cd](url)' + 'ef' = 'ab[cd](url)ef'
      expect(result.newContent).toBe('ab[cd](url)ef')
      // cursorPos = 2 + 1 + 2 + 6 = 11
      expect(result.cursorStart).toBe(11)
      expect(result.cursorEnd).toBe(11)
    })

    it('handles cursor at start of content', () => {
      const result = insertAtCursorPure('Hello', 0, 0, '*', '*')
      expect(result.newContent).toBe('*text*Hello')
      expect(result.cursorStart).toBe(1)
      expect(result.cursorEnd).toBe(5)
    })

    it('handles cursor at end of content', () => {
      const result = insertAtCursorPure('Hello', 5, 5, '`', '`')
      expect(result.newContent).toBe('Hello`text`')
      expect(result.cursorStart).toBe(6)
      expect(result.cursorEnd).toBe(10)
    })

    it('wraps entire content when all selected', () => {
      const result = insertAtCursorPure('abc', 0, 3, '**', '**')
      expect(result.newContent).toBe('**abc**')
      expect(result.cursorStart).toBe(7)
      expect(result.cursorEnd).toBe(7)
    })

    it('handles empty content with no selection', () => {
      const result = insertAtCursorPure('', 0, 0, '> ', '')
      expect(result.newContent).toBe('> text')
      expect(result.cursorStart).toBe(2)
      expect(result.cursorEnd).toBe(6)
    })
  })

  // ── insertLinePure ──────────────────────────────────────────

  describe('insertLinePure', () => {
    it('prepends prefix to line at cursor', () => {
      const result = insertLinePure('Hello world', 6, '## ')
      // lineStart = lastIndexOf('\n', 5) + 1 = -1 + 1 = 0
      // newContent = '' + '## ' + 'Hello world' = '## Hello world'
      expect(result.newContent).toBe('## Hello world')
      expect(result.cursorStart).toBe(9) // 6 + 3
      expect(result.cursorEnd).toBe(9)
    })

    it('prepends prefix to the correct line in multi-line content', () => {
      const content = 'Line one\nLine two\nLine three'
      // Cursor at position 12 = in 'Line two' (after 'Lin')
      const result = insertLinePure(content, 12, '- ')
      // lineStart = lastIndexOf('\n', 11) + 1 = 8 + 1 = 9
      // newContent = 'Line one\n' + '- ' + 'Line two\nLine three'
      expect(result.newContent).toBe('Line one\n- Line two\nLine three')
      expect(result.cursorStart).toBe(14) // 12 + 2
    })

    it('prepends prefix to first line', () => {
      const content = 'Hello\nWorld'
      const result = insertLinePure(content, 3, '> ')
      expect(result.newContent).toBe('> Hello\nWorld')
      expect(result.cursorStart).toBe(5) // 3 + 2
    })

    it('handles cursor at the start of a line', () => {
      const content = 'Line one\nLine two'
      const result = insertLinePure(content, 9, '1. ')
      // lineStart = lastIndexOf('\n', 8) + 1 = 8 + 1 = 9
      expect(result.newContent).toBe('Line one\n1. Line two')
      expect(result.cursorStart).toBe(12)
    })

    it('handles empty content', () => {
      const result = insertLinePure('', 0, '## ')
      expect(result.newContent).toBe('## ')
      expect(result.cursorStart).toBe(3)
    })

    it('handles cursor at end of last line', () => {
      const content = 'First\nSecond'
      const result = insertLinePure(content, 12, '- ')
      expect(result.newContent).toBe('First\n- Second')
      expect(result.cursorStart).toBe(14)
    })
  })

  // ── escapeRegExp ────────────────────────────────────────────

  describe('escapeRegExp', () => {
    it('escapes dots', () => {
      expect(escapeRegExp('a.b')).toBe('a\\.b')
    })

    it('escapes asterisks', () => {
      expect(escapeRegExp('a*b')).toBe('a\\*b')
    })

    it('escapes plus signs', () => {
      expect(escapeRegExp('a+b')).toBe('a\\+b')
    })

    it('escapes question marks', () => {
      expect(escapeRegExp('a?b')).toBe('a\\?b')
    })

    it('escapes caret and dollar', () => {
      expect(escapeRegExp('^a$')).toBe('\\^a\\$')
    })

    it('escapes curly braces', () => {
      expect(escapeRegExp('a{2}')).toBe('a\\{2\\}')
    })

    it('escapes parentheses', () => {
      expect(escapeRegExp('(a|b)')).toBe('\\(a\\|b\\)')
    })

    it('escapes square brackets', () => {
      expect(escapeRegExp('[abc]')).toBe('\\[abc\\]')
    })

    it('escapes backslashes', () => {
      expect(escapeRegExp('a\\b')).toBe('a\\\\b')
    })

    it('returns plain text unchanged', () => {
      expect(escapeRegExp('hello world')).toBe('hello world')
    })

    it('escapes multiple special chars together', () => {
      expect(escapeRegExp('[draw:abc]')).toBe('\\[draw:abc\\]')
    })

    it('produces a valid regex from escaped string', () => {
      const input = 'file.name (copy) [v2].txt'
      const re = new RegExp(escapeRegExp(input))
      expect(re.test(input)).toBe(true)
      expect(re.test('filexname')).toBe(false)
    })
  })

  // ── fetchPreview ────────────────────────────────────────────

  describe('fetchPreview', () => {
    const originalFetch = globalThis.fetch
    const originalLocalStorage = globalThis.localStorage

    beforeEach(() => {
      // Mock localStorage
      Object.defineProperty(globalThis, 'localStorage', {
        value: {
          getItem: vi.fn().mockReturnValue('test-token'),
          setItem: vi.fn(),
          removeItem: vi.fn(),
        },
        writable: true,
        configurable: true,
      })
    })

    afterEach(() => {
      globalThis.fetch = originalFetch
      Object.defineProperty(globalThis, 'localStorage', {
        value: originalLocalStorage,
        writable: true,
        configurable: true,
      })
    })

    it('sends POST with correct headers and body', async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ html: '<p>Hello</p>' }),
      })
      globalThis.fetch = mockFetch

      await fetchPreview('/api/wiki/preview', '# Hello')

      expect(mockFetch).toHaveBeenCalledWith('/api/wiki/preview', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: 'Bearer test-token',
        },
        body: JSON.stringify({ content: '# Hello' }),
        signal: undefined,
      })
    })

    it('returns HTML from response', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ html: '<p>Result</p>' }),
      })

      const result = await fetchPreview('/api/wiki/preview', '# Test')
      expect(result).toBe('<p>Result</p>')
    })

    it('throws on non-ok response', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
      })

      await expect(fetchPreview('/api/wiki/preview', 'test')).rejects.toThrow('Preview failed: 500')
    })

    it('omits Authorization header when no token', async () => {
      (localStorage.getItem as ReturnType<typeof vi.fn>).mockReturnValue(null)
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ html: '' }),
      })
      globalThis.fetch = mockFetch

      await fetchPreview('/api/wiki/preview', 'test')

      const headers = mockFetch.mock.calls[0][1].headers
      expect(headers).not.toHaveProperty('Authorization')
    })

    it('passes abort signal through', async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ html: '' }),
      })
      globalThis.fetch = mockFetch
      const controller = new AbortController()

      await fetchPreview('/api/wiki/preview', 'test', controller.signal)

      expect(mockFetch.mock.calls[0][1].signal).toBe(controller.signal)
    })
  })

  // ── Draw API helpers ────────────────────────────────────────

  describe('fetchDrawings', () => {
    const originalFetch = globalThis.fetch

    afterEach(() => {
      globalThis.fetch = originalFetch
    })

    it('returns drawings filtered to registered project IDs', async () => {
      const registered = [{ draw_id: 'abc' }]
      const allDrawings = [
        { id: 'abc', title: 'Test', updated_at: '2024-01-01' },
        { id: 'other', title: 'Other', updated_at: '2024-01-02' },
      ]
      globalThis.fetch = vi.fn()
        .mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(registered) })
        .mockResolvedValueOnce({ ok: true, json: () => Promise.resolve({ drawings: allDrawings }) })

      const result = await fetchDrawings(1)
      expect(result).toEqual([{ id: 'abc', title: 'Test', updated_at: '2024-01-01' }])
    })

    it('returns empty array when no drawings registered for project', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve([]) })

      const result = await fetchDrawings(1)
      expect(result).toEqual([])
    })

    it('returns empty array on fetch error', async () => {
      globalThis.fetch = vi.fn().mockRejectedValue(new Error('Network error'))

      const result = await fetchDrawings(1)
      expect(result).toEqual([])
    })
  })

  describe('createDrawing', () => {
    const originalFetch = globalThis.fetch
    const originalOpen = globalThis.open

    beforeEach(() => {
      globalThis.open = vi.fn()
    })

    afterEach(() => {
      globalThis.fetch = originalFetch
      globalThis.open = originalOpen
    })

    it('creates a drawing and opens edit URL', async () => {
      globalThis.fetch = vi.fn()
        .mockResolvedValueOnce({ json: () => Promise.resolve({ id: 'new123', edit_url: '/draw/new123/edit' }) })
        .mockResolvedValueOnce({ ok: true })

      const result = await createDrawing(1)
      expect(result).toBe('new123')
      expect(globalThis.open).toHaveBeenCalledWith('/draw/new123/edit', '_blank')
    })

    it('falls back to default edit URL when edit_url not provided', async () => {
      globalThis.fetch = vi.fn()
        .mockResolvedValueOnce({ json: () => Promise.resolve({ id: 'xyz' }) })
        .mockResolvedValueOnce({ ok: true })

      const result = await createDrawing(1)
      expect(result).toBe('xyz')
      expect(globalThis.open).toHaveBeenCalledWith('/draw/xyz/edit', '_blank')
    })

    it('returns null on fetch error', async () => {
      globalThis.fetch = vi.fn().mockRejectedValue(new Error('Network error'))

      const result = await createDrawing(1)
      expect(result).toBeNull()
    })

    it('returns null when response has no id', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        json: () => Promise.resolve({}),
      })

      const result = await createDrawing(1)
      expect(result).toBeNull()
    })
  })

  describe('renameDrawing', () => {
    const originalFetch = globalThis.fetch

    afterEach(() => {
      globalThis.fetch = originalFetch
    })

    it('sends rename request and returns true', async () => {
      const mockFetch = vi.fn().mockResolvedValue({})
      globalThis.fetch = mockFetch

      const result = await renameDrawing('abc', 'New Title')
      expect(result).toBe(true)
      expect(mockFetch).toHaveBeenCalledWith('/draw/api/abc/rename', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: 'New Title' }),
      })
    })

    it('returns false on error', async () => {
      globalThis.fetch = vi.fn().mockRejectedValue(new Error('fail'))

      const result = await renameDrawing('abc', 'title')
      expect(result).toBe(false)
    })
  })

  describe('deleteDrawing', () => {
    const originalFetch = globalThis.fetch

    afterEach(() => {
      globalThis.fetch = originalFetch
    })

    it('sends delete request and returns true', async () => {
      const mockFetch = vi.fn().mockResolvedValue({})
      globalThis.fetch = mockFetch

      const result = await deleteDrawing('abc')
      expect(result).toBe(true)
      expect(mockFetch).toHaveBeenCalledWith('/draw/api/abc/delete', { method: 'POST' })
    })

    it('returns false on error', async () => {
      globalThis.fetch = vi.fn().mockRejectedValue(new Error('fail'))

      const result = await deleteDrawing('abc')
      expect(result).toBe(false)
    })
  })

  describe('deleteDrawings', () => {
    const originalFetch = globalThis.fetch

    afterEach(() => {
      globalThis.fetch = originalFetch
    })

    it('deletes multiple drawings in parallel', async () => {
      const mockFetch = vi.fn().mockResolvedValue({})
      globalThis.fetch = mockFetch

      const result = await deleteDrawings(['a', 'b', 'c'])
      expect(result).toBe(true)
      expect(mockFetch).toHaveBeenCalledTimes(3)
      expect(mockFetch).toHaveBeenCalledWith('/draw/api/a/delete', { method: 'POST' })
      expect(mockFetch).toHaveBeenCalledWith('/draw/api/b/delete', { method: 'POST' })
      expect(mockFetch).toHaveBeenCalledWith('/draw/api/c/delete', { method: 'POST' })
    })

    it('returns false if any delete fails', async () => {
      globalThis.fetch = vi.fn().mockRejectedValue(new Error('fail'))

      const result = await deleteDrawings(['a', 'b'])
      expect(result).toBe(false)
    })

    it('handles empty array', async () => {
      const mockFetch = vi.fn().mockResolvedValue({})
      globalThis.fetch = mockFetch

      const result = await deleteDrawings([])
      expect(result).toBe(true)
      expect(mockFetch).not.toHaveBeenCalled()
    })
  })
})
