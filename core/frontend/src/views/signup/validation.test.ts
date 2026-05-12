import { describe, expect, it } from 'vitest'

import { isValidSignupEmail, isValidSignupUsername, validateSignupForm } from './validation'

describe('signup validation', () => {
	it('accepts a valid signup form', () => {
		expect(
			validateSignupForm({
				username: 'ping_admin',
				email: 'admin@example.com',
				password: 'password123',
				confirm_password: 'password123',
			})
		).toEqual([])
	})

	it('rejects invalid usernames', () => {
		expect(isValidSignupUsername('abc')).toBe(false)
		expect(isValidSignupUsername('ping-admin')).toBe(false)
		expect(isValidSignupUsername('ping_admin')).toBe(true)
	})

	it('rejects invalid email addresses', () => {
		expect(isValidSignupEmail('admin')).toBe(false)
		expect(isValidSignupEmail('admin@example.com')).toBe(true)
	})

	it('rejects short or mismatched passwords', () => {
		expect(
			validateSignupForm({
				username: 'ping_admin',
				email: 'admin@example.com',
				password: 'short',
				confirm_password: 'different',
			})
		).toHaveLength(2)
	})
})
