import { describe, it, expect, vi, beforeEach } from 'vitest'

const mockGet = vi.fn()
const mockPost = vi.fn()

vi.mock('@/api', () => ({
	instance: {
		get: (...args: any[]) => mockGet(...args),
		post: (...args: any[]) => mockPost(...args),
	},
}))

import {
	deployKumoConfig,
	getKumoConfig,
	getKumoMetrics,
	getKumoRuntime,
	getKumoStatus,
	previewKumoConfig,
	saveKumoConfig,
	testKumoConnection,
	type KumoConfig,
} from './kumo'

describe('kumo API module', () => {
	beforeEach(() => {
		vi.clearAllMocks()
	})

	it('getKumoConfig calls GET /kumo/config', () => {
		getKumoConfig()
		expect(mockGet).toHaveBeenCalledWith('/kumo/config')
	})

	it('saveKumoConfig calls POST /kumo/config', () => {
		const params: KumoConfig = {
			enabled: true,
			campaigns_enabled: true,
			api_enabled: true,
			base_url: 'https://email.example.com',
			inject_path: '/api/inject/v1',
			metrics_url: 'https://email.example.com/metrics',
			tls_verify: true,
			auth_mode: 'bearer',
			timeout_ms: 5000,
			default_pool: 'shared-default',
		}
		saveKumoConfig(params)
		expect(mockPost).toHaveBeenCalledWith('/kumo/config', params, expect.any(Object))
	})

	it('testKumoConnection calls POST /kumo/test_connection', () => {
		const params = { base_url: 'https://email.example.com', timeout_ms: 5000 }
		testKumoConnection(params)
		expect(mockPost).toHaveBeenCalledWith('/kumo/test_connection', params)
	})

	it('getKumoStatus calls GET /kumo/status', () => {
		getKumoStatus()
		expect(mockGet).toHaveBeenCalledWith('/kumo/status')
	})

	it('getKumoMetrics calls GET /kumo/metrics', () => {
		getKumoMetrics()
		expect(mockGet).toHaveBeenCalledWith('/kumo/metrics')
	})

	it('getKumoRuntime calls GET /kumo/runtime', () => {
		getKumoRuntime()
		expect(mockGet).toHaveBeenCalledWith('/kumo/runtime')
	})

	it('previewKumoConfig calls POST /kumo/config/preview', () => {
		previewKumoConfig()
		expect(mockPost).toHaveBeenCalledWith('/kumo/config/preview', {})
	})

	it('deployKumoConfig calls POST /kumo/config/deploy', () => {
		const params = { version: 'policy-abc', dry_run: true }
		deployKumoConfig(params)
		expect(mockPost).toHaveBeenCalledWith('/kumo/config/deploy', params, expect.objectContaining({
			fetchOptions: expect.objectContaining({ successMessage: true }),
		}))
	})
})
