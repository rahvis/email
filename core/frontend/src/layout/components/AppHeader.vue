<template>
	<n-layout-header ref="headerRef" :style="{ top: `${top}px` }">
		<div class="header-left">
			<n-button class="icon-btn" :bordered="false" @click="handleCollapse">
				<i class="icon" :class="isCollapse ? 'i-mdi-menu-close' : 'i-mdi-menu-open'"></i>
			</n-button>
			<n-button type="primary" secondary class="header-pill" @click="handleGoIssues">
				{{ t('layout.header.submit') }}
				<i class="i-mdi:arrow-right ml-1px"></i>
			</n-button>
		</div>

		<div class="header-right">
			<n-button class="icon-btn" :bordered="false" @click="handleSetTheme">
				<i class="icon" :class="theme === 'light' ? 'i-ri-sun-line' : 'i-ri-moon-line'"></i>
			</n-button>
			<n-dropdown
				v-if="langOptions.length > 0"
				size="large"
				:options="langOptions"
				@select="handleLangAction">
				<n-button class="icon-btn" :bordered="false">
					<i class="icon i-mdi-language text-20px"></i>
				</n-button>
			</n-dropdown>
			<TenantSwitcher />
			<InstanceSwitcher />
			<n-dropdown size="large" :options="userOptions" @select="handleUserAction">
				<n-button class="icon-btn" :bordered="false">
					<i class="icon i-mdi-user-outline"></i>
				</n-button>
			</n-dropdown>
			<n-button type="primary" text class="header-version" @click="handleGoVersion">
				{{ version }}
			</n-button>
		</div>
	</n-layout-header>
</template>

<script lang="ts" setup>
import { storeToRefs } from 'pinia'
import { DropdownOption } from 'naive-ui'
import { useUserStore, useGlobalStore, useThemeStore } from '@/store'
import InstanceSwitcher from './InstanceSwitcher.vue'
import TenantSwitcher from './TenantSwitcher.vue'
import { getVersionInfo } from '@/api/modules/settings'
import { isObject } from '@/utils'
import { BRAND } from '@/config/brand'

defineProps({
	top: {
		type: Number,
		default: 0,
	},
})

const { t } = useI18n()

const version = ref('--')

const userStore = useUserStore()

const globalStore = useGlobalStore()
const { isCollapse, langList } = storeToRefs(globalStore)

const themeStore = useThemeStore()
const { theme } = storeToRefs(themeStore)

const handleCollapse = () => {
	globalStore.setCollapse()
}

const handleGoIssues = () => {
	window.open(BRAND.issuesUrl)
}

const langOptions = ref<DropdownOption[]>([])

const userOptions = ref<DropdownOption[]>([
	{
		label: t('layout.menu.logout'),
		key: 'logout',
	},
])

const handleSetTheme = () => {
	themeStore.setTheme(theme.value === 'dark' ? 'light' : 'dark')
}

const handleLangAction = async (key: string) => {
	await globalStore.setLang(key)
	window.location.reload()
}

const handleUserAction = (key: string) => {
	switch (key) {
		case 'logout':
			userStore.logout()
			break
	}
}

const handleGoVersion = () => {
	window.open(BRAND.releasesUrl)
}

const getLangOptions = async () => {
	langOptions.value = langList.value.map(item => {
		return {
			label: item.cn,
			key: item.name,
		}
	})
}

const getVersion = async () => {
	const res = await getVersionInfo()
	if (isObject<{ version: string }>(res)) {
		version.value = res.version ? `v${res.version}` : '--'
	}
}

onMounted(() => {
	getVersion()
	getLangOptions()
})
</script>

<style lang="scss" scoped>
.n-layout-header {
	position: absolute;
	top: 0;
	left: 0;
	right: 0;
	display: flex;
	justify-content: space-between;
	align-items: center;
	height: 56px;
	padding: 0 24px 0 16px;
	border-bottom: 1px solid var(--color-border-1);
	background: rgba(255, 255, 255, 0.84);
	box-shadow: var(--shadow-level-1);
	backdrop-filter: blur(18px);
	z-index: 1000;
}

.header-left,
.header-right {
	display: flex;
	align-items: center;
	gap: 8px;
}

.header-item {
	color: var(--color-text-4);
	font-size: 14px;
	text-align: center;
	font-weight: 600;
}

.icon-btn {
	--n-width: 42px;
	--n-height: 42px;
	--n-padding: 0;
	--n-font-size: 21px;
	--n-text-color: var(--color-text-4);
	--n-ripple-color: none;
}

.header-pill {
	--n-height: 34px;
}

.header-version {
	font-size: 14px;
	font-weight: 400;
}
</style>
