import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

const mockGetCurrentTenant = vi.fn()
const mockGetTenants = vi.fn()
const mockSwitchTenant = vi.fn()

vi.mock('@/api/modules/tenants', () => ({
	getCurrentTenant: (...args: any[]) => mockGetCurrentTenant(...args),
	getTenants: (...args: any[]) => mockGetTenants(...args),
	switchTenant: (...args: any[]) => mockSwitchTenant(...args),
}))

import useTenantStore from './tenant'

describe('TenantStore', () => {
	beforeEach(() => {
		setActivePinia(createPinia())
		vi.clearAllMocks()
	})

	it('bootstraps current tenant and membership list', async () => {
		mockGetCurrentTenant.mockResolvedValue({
			tenant_id: 42,
			tenant_name: 'Acme',
			role: 'admin',
		})
		mockGetTenants.mockResolvedValue({
			tenants: [{ tenant_id: 42, name: 'Acme', role: 'admin', status: 'active' }],
		})

		const store = useTenantStore()
		await store.bootstrap()

		expect(store.currentTenantID).toBe(42)
		expect(store.currentTenantName).toBe('Acme')
		expect(store.currentRole).toBe('admin')
		expect(store.tenants).toHaveLength(1)
	})

	it('switches tenant through backend validation', async () => {
		mockSwitchTenant.mockResolvedValue({ tenant_id: 88, tenant_name: 'Northwind', role: 'marketer' })
		mockGetTenants.mockResolvedValue({ tenants: [] })

		const store = useTenantStore()
		await store.switchTenant(88)

		expect(mockSwitchTenant).toHaveBeenCalledWith(88)
		expect(store.currentTenantID).toBe(88)
		expect(store.currentRole).toBe('marketer')
	})

	it('resets tenant-scoped state', () => {
		const store = useTenantStore()
		store.current = { tenant_id: 42, tenant_name: 'Acme' } as any
		store.tenants = [{ tenant_id: 42, name: 'Acme' } as any]

		store.reset()

		expect(store.current).toBeNull()
		expect(store.tenants).toEqual([])
	})
})
