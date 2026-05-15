<template>
	<n-layout class="app-layout w-full h-full" has-sider>
		<sidebar></sidebar>
		<n-layout class="app-content" :content-style="{ position: 'relative', paddingTop: '56px' }" @scroll="handleScroll">
			<app-header :top="scrollTop"></app-header>
			<div v-if="sendingBlocked" class="tenant-send-banner">
				<n-alert type="warning" :show-icon="true" :bordered="false">
					Tenant sending blocked: {{ sendingBlockReason }}
				</n-alert>
			</div>
			<app-main></app-main>
		</n-layout>
	</n-layout>
</template>

<script lang="ts" setup>
import { Sidebar, AppHeader, AppMain } from './components'
import { useTenantStore } from '@/store'

const scrollTop = ref(0)
const tenantStore = useTenantStore()
const sendingBlocked = computed(() => {
	const status = tenantStore.current?.sending_status || 'active'
	return status !== 'active'
})
const sendingBlockReason = computed(() => {
	return tenantStore.current?.sending_block_reason || tenantStore.current?.sending_status || 'blocked'
})

const handleScroll = (e: Event) => {
	scrollTop.value = (e.target as HTMLElement).scrollTop || 0
}
</script>

<style lang="scss" scoped>
.app-layout {
	background: var(--color-bg-2);
}

.app-content {
	background:
		linear-gradient(180deg, rgba(255, 255, 255, 0.72), rgba(255, 255, 255, 0) 220px),
		var(--color-bg-2);
}

.tenant-send-banner {
	padding: 12px 16px 0;
}
</style>
