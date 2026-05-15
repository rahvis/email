import { describe, it, expect, vi, beforeEach } from 'vitest'

const mockGet = vi.fn()
const mockPost = vi.fn()

vi.mock('@/api', () => ({
	instance: {
		get: (...args: any[]) => mockGet(...args),
		post: (...args: any[]) => mockPost(...args),
	},
}))

import {
	createTenant,
	getCurrentTenant,
	getSendingProfiles,
	getTenants,
	saveSendingProfile,
	switchTenant,
	updateSendingControl,
} from './tenants'

describe('tenants API module', () => {
	beforeEach(() => {
		vi.clearAllMocks()
	})

	it('getCurrentTenant calls GET /tenants/current', () => {
		getCurrentTenant()
		expect(mockGet).toHaveBeenCalledWith('/tenants/current')
	})

	it('getTenants calls GET /tenants', () => {
		getTenants()
		expect(mockGet).toHaveBeenCalledWith('/tenants')
	})

	it('switchTenant calls POST /tenants/switch', () => {
		switchTenant(42)
		expect(mockPost).toHaveBeenCalledWith('/tenants/switch', { tenant_id: 42 })
	})

	it('createTenant calls POST /tenants with success message', () => {
		const params = { name: 'Acme', owner_email: 'owner@example.com' }
		createTenant(params)
		expect(mockPost).toHaveBeenCalledWith('/tenants', params, expect.objectContaining({
			fetchOptions: expect.objectContaining({ successMessage: true }),
		}))
	})

	it('getSendingProfiles calls tenant profile list endpoint', () => {
		getSendingProfiles(42)
		expect(mockGet).toHaveBeenCalledWith('/tenants/42/sending-profiles')
	})

	it('saveSendingProfile posts profile params with success message', () => {
		const params = {
			name: 'Marketing',
			sender_domains: ['example.com'],
			egress_mode: 'external_kumoproxy',
			dkim_selector: 'bm1',
		}
		saveSendingProfile(42, params)
		expect(mockPost).toHaveBeenCalledWith('/tenants/42/sending-profile', params, expect.objectContaining({
			fetchOptions: expect.objectContaining({ successMessage: true }),
		}))
	})

	it('updateSendingControl posts tenant control params with success message', () => {
		const params = { status: 'paused', reason: 'quota exceeded' }
		updateSendingControl(42, params)
		expect(mockPost).toHaveBeenCalledWith('/tenants/42/sending-control', params, expect.objectContaining({
			fetchOptions: expect.objectContaining({ successMessage: true }),
		}))
	})
})
