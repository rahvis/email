import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import CommonSettings from './index.vue'

const mockGetCommonConfig = vi.fn()
const mockReset = vi.fn()

vi.mock('@/views/settings/common/store', () => ({
	useSettingsStore: () => ({
		getCommonConfig: mockGetCommonConfig,
		reset: mockReset,
	}),
}))

vi.mock('vue-i18n', async importOriginal => {
	const actual = await importOriginal<typeof import('vue-i18n')>()
	return {
		...actual,
		useI18n: () => ({
			t: (key: string) =>
				({
					'layout.menu.common': 'Common Settings',
					'settings.common.security.title': 'Security',
					'settings.common.network.title': 'Network',
					'settings.common.system.title': 'System',
					'settings.common.notifySettings.title': 'Notifications',
				})[key] || key,
		}),
	}
})

vi.mock('./components/SecuritySettings.vue', () => ({
	default: { template: '<div>Security Panel</div>' },
}))

vi.mock('./components/NetworkSettings.vue', () => ({
	default: { template: '<div>Network Panel</div>' },
}))

vi.mock('./components/SystemSettings.vue', () => ({
	default: { template: '<div>System Panel</div>' },
}))

vi.mock('./components/Notify/NotifySettings.vue', () => ({
	default: { template: '<div>Notifications Panel</div>' },
}))

describe('Common settings page', () => {
	beforeEach(() => {
		vi.clearAllMocks()
	})

	it('renders full-width section menu layout', () => {
		const wrapper = mount(CommonSettings)

		expect(wrapper.find('.common-settings-page').classes()).toContain('app-card')
		expect(wrapper.text()).toContain('Common Settings')
		expect(wrapper.text()).toContain('Security')
		expect(wrapper.text()).toContain('Network')
		expect(wrapper.text()).toContain('System')
		expect(wrapper.text()).toContain('Notifications')
		expect(wrapper.text()).toContain('Security Panel')
		expect(mockGetCommonConfig).toHaveBeenCalled()
	})

	it('switches active common settings section', async () => {
		const wrapper = mount(CommonSettings)

		const networkButton = wrapper
			.findAll('button.section-menu-item')
			.find(button => button.text().includes('Network'))

		expect(networkButton).toBeTruthy()
		await networkButton!.trigger('click')

		expect(networkButton!.classes()).toContain('is-active')
		expect(wrapper.text()).toContain('Network Panel')
	})
})
