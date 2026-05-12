import { describe, expect, it } from 'vitest'
import { escapeHtml, sanitizeHtml } from './sanitize'

describe('sanitize utilities', () => {
	it('removes executable HTML while keeping safe markup', () => {
		const result = sanitizeHtml('<p>Hello</p><img src=x onerror=alert(1)><script>alert(1)</script>')
		expect(result).toContain('<p>Hello</p>')
		expect(result).not.toContain('onerror')
		expect(result).not.toContain('<script>')
	})

	it('escapes text for code rendering fallbacks', () => {
		expect(escapeHtml(`<span data-x="1">A&B</span>`)).toBe(
			'&lt;span data-x=&quot;1&quot;&gt;A&amp;B&lt;/span&gt;'
		)
	})
})
