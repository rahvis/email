import { describe, it, expect, vi, beforeEach } from 'vitest'

const mockGet = vi.fn()
const mockPost = vi.fn()

vi.mock('@/api', () => ({
	instance: {
		get: (...args: any[]) => mockGet(...args),
		post: (...args: any[]) => mockPost(...args),
	},
}))

vi.mock('@/i18n', () => ({
	default: {
		global: { t: (key: string) => key },
	},
}))

import { login, signup, logout, refreshToken, getValidateCode } from './user'

describe('user API', () => {
	beforeEach(() => {
		vi.clearAllMocks()
	})

	it('login calls POST /login with credentials', () => {
		const params = { username: 'admin', password: 'pass' }
		login(params)
		expect(mockPost).toHaveBeenCalledWith(
			'/login',
			params,
			expect.objectContaining({
				fetchOptions: expect.objectContaining({ successMessage: true }),
			})
		)
	})

	it('signup calls POST /signup with account details', () => {
		const params = {
			username: 'ping_admin',
			email: 'admin@example.com',
			password: 'password123',
			confirm_password: 'password123',
		}
		signup(params)
		expect(mockPost).toHaveBeenCalledWith(
			'/signup',
			params,
			expect.objectContaining({
				fetchOptions: expect.objectContaining({ successMessage: true }),
			})
		)
	})

	it('logout calls POST /logout with refresh token when available', () => {
		logout('refresh-token')
		expect(mockPost).toHaveBeenCalledWith(
			'/logout',
			{ refreshToken: 'refresh-token' },
			expect.objectContaining({
				fetchOptions: expect.objectContaining({
					loading: 'user.api.loading.logout',
					successMessage: true,
				}),
			})
		)
	})

	it('refreshToken calls POST /refresh-token with refresh token body', () => {
		refreshToken('refresh-token')
		expect(mockPost).toHaveBeenCalledWith('/refresh-token', { refreshToken: 'refresh-token' })
	})

	it('getValidateCode calls GET /get_validate_code', () => {
		getValidateCode()
		expect(mockGet).toHaveBeenCalledWith('/get_validate_code')
	})
})
