<template>
	<n-layout-sider
		class="app-sidebar"
		:class="{ 'app-sidebar--collapsed': isCollapse }"
		collapse-mode="width"
		:collapsed="isCollapse"
		:width="232"
		:collapsed-width="72"
		:content-style="{
			display: 'flex',
			flexDirection: 'column',
			height: '100%',
			overflow: 'hidden',
		}">
		<!-- 应用标志和名称 -->
		<div class="app-logo" :class="{ collapse: isCollapse }">
			<a href="/">
				<img class="icon" src="@/assets/images/logo-white.png"></img>
				<span v-show="!isCollapse" class="app-name">{{ BRAND.name }}</span>
			</a>
		</div>

		<!-- 导航菜单 -->
		<div class="nav-section">
			<n-menu
				:value="activeMenuKey"
				:collapsed="isCollapse"
				:collapsed-width="72"
				:options="menuOptions"
				:root-indent="14"
				@update:value="handleUpdateMenu">
			</n-menu>
		</div>
		<!-- 退出登录 -->
		<div class="footer-section">
			<n-menu
				value=""
				:collapsed="isCollapse"
				:collapsed-width="72"
				:options="logoutOptions"
				:root-indent="14"
				@update:value="handleUpdateMenu">
			</n-menu>
		</div>
	</n-layout-sider>
</template>

<script lang="tsx" setup>
import { VNodeChild } from 'vue'
import { MenuOption } from 'naive-ui'
import { storeToRefs } from 'pinia'
import { RouterLink } from 'vue-router'
import { useMenuStore, useGlobalStore, useUserStore } from '@/store'
import { menuList } from '@/router/router'
import { BRAND } from '@/config/brand'

const { t } = useI18n()

const route = useRoute()

const menuStore = useMenuStore()
const userStore = useUserStore()
const globalStore = useGlobalStore()

const { isCollapse } = storeToRefs(globalStore)

// 当前菜单名称
const activeMenuKey = computed(() => {
	return String(route.meta?.key || '')
})

// 路由菜单
const routerMenus = computed(() => {
	return menuStore.menuList.filter(route => route.meta && !route.meta.hidden)
})

// 导航菜单选项
const menuOptions = computed(() => {
	return routerMenus.value.map(route => {
		const name = String(route.children?.[0]?.name || '')
		const key = String(route.meta?.key || '')
		const titleKey = String(route.meta?.titleKey || '')
		const title = titleKey ? t(titleKey) : String(route.meta?.title || '')
		return {
			key,
			label: () => renderLabel(name, title),
			icon: () => renderIcon(key),
		}
	})
})

const logoutOptions = ref<MenuOption[]>([
	{
		key: 'logout',
		label: () => <span class="sidebar-link sidebar-link--action">{t('layout.menu.logout')}</span>,
		icon: () => renderIcon('logout'),
	},
])

const renderLabel = (name: string, title: string) => {
	return (
		<RouterLink class="sidebar-link" to={{ name }} aria-label={title} title={title}>
			<span class="sidebar-label-text">{title}</span>
		</RouterLink>
	)
}

const iconMap: Record<string, VNodeChild> = {
	overview: <i class="i-mdi-web"></i>,
	market: <i class="i-mdi-email-fast-outline"></i>,
	api: <i class="i-mdi-chart-line"></i>,
	contacts: <i class="i-mdi-user-multiple-outline"></i>,
	sequences: <i class="i-mdi-email-sync-outline"></i>,
	leads: <i class="i-mdi-account-search-outline"></i>,
	enrichment: <i class="i-mdi-database-search-outline"></i>,
	domain: <i class="i-mdi-web"></i>,
	mailbox: <i class="i-custom:mailbox"></i>,
	smtp: <i class="i-custom:smtp"></i>,
	settings: <i class="i-mdi-settings-outline"></i>,
	template: <i class="i-mdi-settings-outline"></i>,
	logs: <i class="i-icon-park-outline:log"></i>,
	'video-outreach': <i class="i-mdi-video-outline"></i>,
	logout: <i class="i-mdi-logout"></i>,
}

const renderIcon = (key: string) => {
	return iconMap[key]
}

const handleUpdateMenu = (key: string) => {
	if (key === 'logout') {
		userStore.logout()
	}
	if (key === 'webmail') {
		const route = routerMenus.value.find(item => item.meta?.key === 'webmail')
		if (route) {
			const href = String(route.meta?.href)
			window.open(href)
		}
	}
}

onMounted(() => {
	menuStore.setMenuList(menuList)
})
</script>

<style lang="scss" scoped>
.app-sidebar {
	background: linear-gradient(180deg, #1c1e54 0%, #191c4d 100%);
	border-right: 1px solid rgba(255, 255, 255, 0.08);
	box-shadow: rgba(0, 0, 0, 0.18) 0 8px 24px;
	z-index: 1010;
}

.app-logo {
	display: flex;
	min-height: 72px;
	padding: 16px 18px;
	border-bottom: 1px solid rgba(255, 255, 255, 0.08);
	transition: all 0.3s ease;

	&.collapse {
		justify-content: center;
		padding: 16px 0;
	}

	a {
		display: flex;
		align-items: center;
		width: 100%;
		min-width: 0;
		gap: 12px;
		color: inherit;
		text-decoration: none;
	}

	.icon {
		flex: 0 0 38px;
		width: 38px;
		height: 38px;
		object-fit: contain;
	}
	
	.app-name {
		overflow: hidden;
		font-size: 20px;
		font-weight: 500;
		letter-spacing: 0;
		line-height: 1.2;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: #fff;
	}
}

.nav-section {
	flex: 1;
	min-height: 0;
	padding: 8px 0;
	overflow: auto;

	&::-webkit-scrollbar {
		width: 6px;
	}

	&::-webkit-scrollbar-thumb {
		border-radius: var(--radius-pill);
		background: rgba(255, 255, 255, 0.16);
	}
}

.footer-section {
	border-top: 1px solid rgba(255, 255, 255, 0.08);
	transition: all 0.3s ease;
}

.n-menu {
	--n-item-height: 44px;
	--n-font-size: 14px;
	padding: 8px 12px;

	:deep(.n-menu-item) {
		margin-top: 0;
		margin-bottom: 4px;

		&:last-of-type {
			margin-bottom: 0;
		}

		.n-menu-item-content {
			min-width: 0;
			padding-right: 12px;
			border-radius: var(--radius-md);
			line-height: 22px;
			transition:
				background 0.18s ease,
				color 0.18s ease;
		}

		.n-menu-item-content::before {
			left: 0;
			right: 0;
			border-radius: var(--radius-md);
		}

		.n-menu-item-content:not(.n-menu-item-content--disabled):hover::before {
			background: rgba(255, 255, 255, 0.08);
		}

		.n-menu-item-content--selected::before {
			background: linear-gradient(90deg, rgba(102, 94, 253, 0.95), rgba(83, 58, 253, 0.78));
			box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.12);
		}

		.n-menu-item-content--selected .n-menu-item-content-header {
			font-weight: 500;
		}

		.n-menu-item-content__icon {
			flex: 0 0 20px;
			width: 20px;
			height: 20px;
			margin-right: 12px;
			font-size: 20px;
		}

		.n-menu-item-content__icon i {
			width: 20px;
			height: 20px;
		}

		.n-menu-item-content-header {
			min-width: 0;
			text-align: left;
		}
	}
}

.sidebar-link {
	display: flex;
	align-items: center;
	justify-content: flex-start;
	width: 100%;
	min-width: 0;
	height: 100%;
	color: inherit;
	text-align: left;
	text-decoration: none;
}

.sidebar-link--action {
	cursor: pointer;
}

.sidebar-label-text {
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
}

.footer-section .n-menu {
	padding-top: 10px;
	padding-bottom: 14px;
}

.app-sidebar--collapsed {
	.app-logo {
		min-height: 68px;

		.icon {
			flex-basis: 34px;
			width: 34px;
			height: 34px;
		}
	}

	.n-menu {
		padding-right: 10px;
		padding-left: 10px;

		:deep(.n-menu-item-content) {
			padding-right: 0;
			padding-left: 0;
			justify-content: center;
		}

		:deep(.n-menu-item-content__icon) {
			margin-right: 0;
		}
	}
}
</style>
