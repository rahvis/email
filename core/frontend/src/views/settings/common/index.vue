<template>
	<div class="common-settings-page app-card">
		<div class="common-settings-header">
			<div class="header-title">
				<i class="i-mdi:cog-outline text-6"></i>
				<span>{{ t('layout.menu.common') }}</span>
			</div>
		</div>

		<div class="section-layout">
			<aside class="section-menu" aria-label="Common settings sections">
				<button
					v-for="section in sectionItems"
					:key="section.key"
					type="button"
					class="section-menu-item"
					:class="{ 'is-active': activeSection === section.key }"
					@click="activeSection = section.key">
					<i :class="section.icon"></i>
					<span>{{ section.label }}</span>
				</button>
			</aside>

			<main class="section-panel">
				<component :is="activeComponent" />
			</main>
		</div>
	</div>
</template>

<script lang="ts" setup>
import { useSettingsStore } from '@/views/settings/common/store'

import SecuritySettings from './components/SecuritySettings.vue'
import NetworkSettings from './components/NetworkSettings.vue'
import SystemSettings from './components/SystemSettings.vue'
import NotifySettings from './components/Notify/NotifySettings.vue'

const { t } = useI18n()

const settingsStore = useSettingsStore()

settingsStore.getCommonConfig()

const sectionComponents = {
	security: SecuritySettings,
	network: NetworkSettings,
	system: SystemSettings,
	notifications: NotifySettings,
}

type SectionKey = keyof typeof sectionComponents

const activeSection = ref<SectionKey>('security')

const sectionItems = computed(() => [
	{
		key: 'security' as const,
		label: t('settings.common.security.title'),
		icon: 'i-mdi-shield-account',
	},
	{
		key: 'network' as const,
		label: t('settings.common.network.title'),
		icon: 'i-mdi-network',
	},
	{
		key: 'system' as const,
		label: t('settings.common.system.title'),
		icon: 'i-mdi-cog',
	},
	{
		key: 'notifications' as const,
		label: t('settings.common.notifySettings.title'),
		icon: 'i-mdi:notification-settings-outline',
	},
])

const activeComponent = computed(() => sectionComponents[activeSection.value])

onBeforeUnmount(() => {
	settingsStore.reset()
})
</script>

<style lang="scss" scoped>
@use '@/styles/index.scss' as base;

.common-settings-page {
	width: 100%;
	padding: var(--space-xl);
	box-sizing: border-box;
}

.common-settings-header {
	padding-bottom: var(--space-lg);
	margin-bottom: var(--space-lg);
	border-bottom: 1px solid var(--color-border-2);
}

.header-title {
	@include base.row-flex-start;
	gap: var(--space-sm);
	align-items: center;
	font-size: 16px;
	font-weight: 700;
	color: var(--color-text-1);
}

.section-layout {
	display: grid;
	grid-template-columns: 230px minmax(0, 1fr);
	min-height: 560px;
}

.section-menu {
	display: flex;
	flex-direction: column;
	padding-right: var(--space-lg);
	border-right: 1px solid var(--color-border-2);
}

.section-menu-item {
	display: flex;
	align-items: center;
	gap: var(--space-sm);
	width: 100%;
	min-height: 48px;
	padding: 0 var(--space-md);
	border: 0;
	border-radius: var(--radius-md);
	background: transparent;
	color: var(--color-text-2);
	font-size: 14px;
	font-weight: 600;
	text-align: left;
	cursor: pointer;
	transition:
		background 0.2s ease,
		color 0.2s ease;

	i {
		flex: 0 0 auto;
		font-size: 22px;
	}

	span {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	&:hover,
	&.is-active {
		background: var(--color-primary-2);
		color: var(--color-primary-1);
	}
}

.section-panel {
	min-width: 0;
	padding-left: var(--space-xl);
}

.section-panel :deep(.n-card) {
	margin-bottom: 0 !important;
	background: transparent;
	box-shadow: none;
}

.section-panel :deep(.n-card-header) {
	padding-top: 0;
	padding-left: 0;
	padding-right: 0;
	text-align: left;
}

.section-panel :deep(.n-card-header__main) {
	display: flex;
	justify-content: flex-start;
	text-align: left;
}

.section-panel :deep(.n-card-header .flex.items-center) {
	justify-content: flex-start;
	text-align: left;
}

.section-panel :deep(.n-card-header .n-icon) {
	display: none;
}

@media (max-width: 900px) {
	.section-layout {
		grid-template-columns: 1fr;
		min-height: auto;
	}

	.section-menu {
		flex-direction: row;
		gap: var(--space-sm);
		padding-right: 0;
		padding-bottom: var(--space-md);
		margin-bottom: var(--space-lg);
		overflow-x: auto;
		border-right: 0;
		border-bottom: 1px solid var(--color-border-2);
	}

	.section-menu-item {
		width: auto;
		min-width: 160px;
	}

	.section-panel {
		padding-left: 0;
	}
}
</style>
