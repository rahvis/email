import { defineStore } from 'pinia'
import {
	getCurrentTenant,
	getTenants,
	switchTenant as switchTenantApi,
	type TenantContext,
	type TenantMembership,
} from '@/api/modules/tenants'

export default defineStore(
	'TenantStore',
	() => {
		const current = ref<TenantContext | null>(null)
		const tenants = ref<TenantMembership[]>([])
		const loading = ref(false)

		const currentTenantID = computed(() => current.value?.tenant_id || 0)
		const currentTenantName = computed(() => current.value?.tenant_name || '')
		const currentRole = computed(() => current.value?.role || '')

		const loadCurrent = async () => {
			const res = await getCurrentTenant()
			current.value = res as TenantContext
			return current.value
		}

		const loadTenants = async () => {
			const res = await getTenants()
			tenants.value = Array.isArray(res?.tenants) ? res.tenants : []
			return tenants.value
		}

		const bootstrap = async () => {
			loading.value = true
			try {
				await Promise.all([loadCurrent(), loadTenants()])
			} finally {
				loading.value = false
			}
		}

		const switchTenant = async (tenantID: number) => {
			const res = await switchTenantApi(tenantID)
			current.value = res as TenantContext
			await loadTenants()
			return current.value
		}

		const reset = () => {
			current.value = null
			tenants.value = []
			loading.value = false
		}

		return {
			current,
			tenants,
			loading,
			currentTenantID,
			currentTenantName,
			currentRole,
			loadCurrent,
			loadTenants,
			bootstrap,
			switchTenant,
			reset,
		}
	},
	{
		persist: {
			pick: ['current'],
		},
	}
)
