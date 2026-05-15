import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import KumoSettings from './index.vue'

const mockGetKumoConfig = vi.fn()
const mockGetKumoStatus = vi.fn()
const mockGetKumoMetrics = vi.fn()
const mockGetKumoRuntime = vi.fn()
const mockSaveKumoConfig = vi.fn()
const mockTestKumoConnection = vi.fn()
const mockPreviewKumoConfig = vi.fn()
const mockDeployKumoConfig = vi.fn()

vi.mock('@/api/modules/kumo', () => ({
	getKumoConfig: () => mockGetKumoConfig(),
	getKumoStatus: () => mockGetKumoStatus(),
	getKumoMetrics: () => mockGetKumoMetrics(),
	getKumoRuntime: () => mockGetKumoRuntime(),
	saveKumoConfig: (params: unknown) => mockSaveKumoConfig(params),
	testKumoConnection: (params: unknown) => mockTestKumoConnection(params),
	previewKumoConfig: () => mockPreviewKumoConfig(),
	deployKumoConfig: (params: unknown) => mockDeployKumoConfig(params),
}))

vi.mock('@/utils', () => ({
	formatTime: (time: number) => `time:${time}`,
	Message: {
		success: vi.fn(),
		error: vi.fn(),
		loading: vi.fn(() => ({ close: vi.fn() })),
	},
}))

const baseConfig = {
	enabled: true,
	campaigns_enabled: true,
	api_enabled: true,
	base_url: 'https://email.example.com',
	inject_path: '/api/inject/v1',
	metrics_url: 'https://email.example.com/metrics',
	tls_verify: true,
	auth_mode: 'bearer',
	has_auth_secret: true,
	has_webhook_secret: true,
	timeout_ms: 5000,
	default_pool: 'shared-default',
}

const baseMetrics = {
	snapshot_at: 1,
	queues: [],
	nodes: [],
	raw_bytes: 120,
	metric_lines: 12,
}

const baseRuntime = {
	snapshot_at: 1,
	injection: {
		attempts: 12,
		successes: 10,
		failures: 2,
		rate_limited: 0,
		backpressure_rejected: 0,
		circuit_rejected: 0,
		avg_latency_ms: 42,
		in_flight_global: 1,
		open_circuit_count: 0,
	},
	webhook: {
		accepted: 10,
		duplicates: 1,
		failed: 0,
		security_failures: 0,
		processing_failures: 0,
		orphaned: 0,
		redis_dedupe_hits: 1,
		avg_ingestion_lag_ms: 120,
	},
	alerts: [],
	queues: [],
	tenant_risk: [],
	release_readiness: {
		ready: true,
		checks: [],
		blockers: [],
		rollback: [],
		generated_at: 1,
	},
}

const stubs = {
	NCard: { template: '<section><slot name="header" /><slot /></section>' },
	NTag: { template: '<span><slot /></span>' },
	NFlex: { template: '<div><slot /></div>' },
	NGrid: { template: '<div><slot /></div>' },
	NGridItem: { template: '<div><slot /></div>' },
	NStatistic: { template: '<div><slot name="label" />{{ label }}{{ value }}<slot name="suffix" /></div>', props: ['label', 'value'] },
	NForm: { template: '<form><slot /></form>' },
	NFormItemGi: { template: '<div><slot /></div>' },
	NFormItem: { template: '<div><slot /></div>' },
	NCheckbox: { template: '<label><slot /></label>' },
	NSpace: { template: '<div><slot /></div>' },
	NAlert: { template: '<div><slot /></div>' },
	NDescriptions: { template: '<div><slot /></div>' },
	NDescriptionsItem: { template: '<div><slot /></div>' },
	NInput: { template: '<input />' },
	NInputNumber: { template: '<input />' },
	NSelect: { template: '<div />' },
	NSwitch: { template: '<button />' },
	NButton: { template: '<button><slot /></button>' },
	NDataTable: { template: '<div />' },
}

describe('KumoMTA settings page', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mockGetKumoConfig.mockResolvedValue(baseConfig)
		mockGetKumoMetrics.mockResolvedValue(baseMetrics)
		mockGetKumoRuntime.mockResolvedValue(baseRuntime)
	})

	it('renders connected state', async () => {
		mockGetKumoStatus.mockResolvedValue({
			connected: true,
			last_ok_at: 1,
			last_error_at: 0,
			last_error: '',
			inject_latency_ms: 34,
			metrics_latency_ms: 72,
			webhook_last_seen_at: 1,
			webhook_lag_seconds: 3,
		})

		const wrapper = mount(KumoSettings, { global: { stubs } })
		await flushPromises()

		expect(wrapper.text()).toContain('KumoMTA Connection')
		expect(wrapper.text()).toContain('Connected')
		expect(wrapper.text()).toContain('Configuration')
		expect(wrapper.text()).toContain('Policy Preview')
		expect(wrapper.find('.kumo-page').classes()).toContain('app-card')
		expect(mockGetKumoConfig).toHaveBeenCalled()
		expect(mockGetKumoStatus).toHaveBeenCalled()
		expect(mockGetKumoMetrics).toHaveBeenCalled()
		expect(mockGetKumoRuntime).toHaveBeenCalled()
	})

	it('switches between KumoMTA section menu items', async () => {
		mockGetKumoStatus.mockResolvedValue({
			connected: true,
			last_ok_at: 1,
			last_error_at: 0,
			last_error: '',
			inject_latency_ms: 34,
			metrics_latency_ms: 72,
			webhook_last_seen_at: 1,
			webhook_lag_seconds: 3,
		})

		const wrapper = mount(KumoSettings, { global: { stubs } })
		await flushPromises()

		const configurationButton = wrapper
			.findAll('button.section-menu-item')
			.find(button => button.text().includes('Configuration'))

		expect(configurationButton).toBeTruthy()
		await configurationButton!.trigger('click')

		expect(configurationButton!.classes()).toContain('is-active')
	})

	it('renders disconnected state', async () => {
		mockGetKumoStatus.mockResolvedValue({
			connected: false,
			last_ok_at: 0,
			last_error_at: 0,
			last_error: '',
			inject_latency_ms: 0,
			metrics_latency_ms: 0,
			webhook_last_seen_at: 0,
			webhook_lag_seconds: 0,
		})

		const wrapper = mount(KumoSettings, { global: { stubs } })
		await flushPromises()

		expect(wrapper.text()).toContain('Disconnected')
	})

	it('renders error state', async () => {
		mockGetKumoStatus.mockResolvedValue({
			connected: false,
			last_ok_at: 0,
			last_error_at: 1,
			last_error: 'authentication failed',
			inject_latency_ms: 10,
			metrics_latency_ms: 0,
			webhook_last_seen_at: 0,
			webhook_lag_seconds: 0,
		})

		const wrapper = mount(KumoSettings, { global: { stubs } })
		await flushPromises()

		expect(wrapper.text()).toContain('Error')
	})
})
