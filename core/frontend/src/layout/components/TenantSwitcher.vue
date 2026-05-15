<template>
	<n-dropdown
		size="large"
		trigger="click"
		:options="dropdownOptions"
		:disabled="tenantStore.loading || tenantStore.tenants.length === 0"
		@select="handleSelect">
		<n-button class="tenant-button" :loading="tenantStore.loading" secondary>
			<i class="i-mdi-domain text-16px"></i>
			<span class="tenant-name">{{ tenantLabel }}</span>
			<n-tag v-if="tenantStore.currentRole" size="small" :bordered="false">
				{{ tenantStore.currentRole }}
			</n-tag>
		</n-button>
	</n-dropdown>
</template>

<script lang="ts" setup>
import { DropdownOption, useMessage } from 'naive-ui'
import { clearPendingRequests } from '@/api'
import router from '@/router'
import { useTenantStore } from '@/store'

const tenantStore = useTenantStore()
const message = useMessage()

const tenantLabel = computed(() => tenantStore.currentTenantName || 'Workspace')

const dropdownOptions = computed<DropdownOption[]>(() => {
	return tenantStore.tenants.map(tenant => ({
		label: `${tenant.name} (${tenant.role})`,
		key: tenant.tenant_id,
		disabled: tenant.tenant_id === tenantStore.currentTenantID,
		icon:
			tenant.tenant_id === tenantStore.currentTenantID
				? () => h('i', { class: 'i-mdi-check text-14px' })
				: undefined,
	}))
})

const handleSelect = async (key: string | number) => {
	const tenantID = Number(key)
	if (!tenantID || tenantID === tenantStore.currentTenantID) {
		return
	}
	clearPendingRequests()
	await tenantStore.switchTenant(tenantID)
	sessionStorage.removeItem('bm-contact-filters')
	sessionStorage.removeItem('bm-campaign-filters')
	sessionStorage.removeItem('bm-template-filters')
	message.success('Workspace switched')
	await router.replace('/overview')
}

onMounted(() => {
	if (!tenantStore.currentTenantID) {
		tenantStore.bootstrap().catch(() => tenantStore.reset())
	} else if (tenantStore.tenants.length === 0) {
		tenantStore.loadTenants().catch(() => undefined)
	}
})
</script>

<style lang="scss" scoped>
.tenant-button {
	--n-height: 34px;
	max-width: 260px;
	padding: 0 10px;
}

.tenant-name {
	min-width: 0;
	max-width: 140px;
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
	font-size: 13px;
}
</style>
