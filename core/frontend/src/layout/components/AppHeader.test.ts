import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'

const mockUserStore = vi.hoisted(() => ({
	logout: vi.fn(),
}))

const mockGlobalStore = vi.hoisted(() => ({
	isCollapse: false,
	langList: [],
	setCollapse: vi.fn(),
	setLang: vi.fn(),
}))

const mockThemeStore = vi.hoisted(() => ({
	theme: 'light',
	setTheme: vi.fn(),
}))

vi.mock('pinia', () => ({
	storeToRefs: (store: Record<string, unknown>) => ({
		isCollapse: { value: store.isCollapse },
		langList: { value: store.langList },
		theme: { value: store.theme },
	}),
}))

vi.mock('@/store', () => ({
	default: { install: vi.fn() },
	useUserStore: () => mockUserStore,
	useGlobalStore: () => mockGlobalStore,
	useThemeStore: () => mockThemeStore,
}))

vi.mock('vue-i18n', () => ({
	createI18n: () => ({
		install: vi.fn(),
		global: { locale: { value: 'en' } },
	}),
	useI18n: () => ({
		t: (key: string) =>
			({
				'layout.menu.logout': 'Logout',
				'layout.header.submit': 'Submit a Request',
			})[key] || key,
	}),
}))

vi.mock('./TenantSwitcher.vue', () => ({
	default: { name: 'TenantSwitcher', template: '<div>TenantSwitcher</div>' },
}))

vi.mock('./InstanceSwitcher.vue', () => ({
	default: { name: 'InstanceSwitcher', template: '<div>InstanceSwitcher</div>' },
}))

import AppHeader from './AppHeader.vue'

const stubs = {
	NLayoutHeader: { template: '<header><slot /></header>' },
	NButton: { template: '<button><slot /></button>' },
	NDropdown: { template: '<div><slot /></div>' },
}

describe('AppHeader', () => {
	beforeEach(() => {
		vi.clearAllMocks()
	})

	it('removes submit request and version controls while keeping core header controls', () => {
		const wrapper = mount(AppHeader, {
			props: { top: 0 },
			global: { stubs },
		})

		expect(wrapper.text()).not.toContain('Submit a Request')
		expect(wrapper.find('.header-pill').exists()).toBe(false)
		expect(wrapper.find('.header-version').exists()).toBe(false)
		expect(wrapper.find('.header-left .icon-btn').exists()).toBe(true)
		expect(wrapper.text()).toContain('TenantSwitcher')
		expect(wrapper.text()).toContain('InstanceSwitcher')
	})
})
