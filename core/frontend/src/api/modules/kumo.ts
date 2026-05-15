import { instance } from '@/api'

export interface KumoConfig {
	enabled: boolean
	campaigns_enabled: boolean
	api_enabled: boolean
	base_url: string
	inject_path: string
	metrics_url: string
	tls_verify: boolean
	auth_mode: string
	has_auth_secret?: boolean
	has_webhook_secret?: boolean
	auth_secret?: string
	webhook_secret?: string
	timeout_ms: number
	default_pool: string
	updated_at?: number
}

export interface KumoEndpointCheck {
	ok: boolean
	status_code: number
	latency_ms: number
	message: string
}

export interface KumoTestResult {
	ok: boolean
	health_ms: number
	metrics_ms: number
	message: string
	inject: KumoEndpointCheck
	metrics: KumoEndpointCheck
}

export interface KumoStatus {
	connected: boolean
	last_ok_at: number
	last_error_at: number
	last_error: string
	inject_latency_ms: number
	metrics_latency_ms: number
	webhook_last_seen_at: number
	webhook_lag_seconds: number
}

export interface KumoMetrics {
	snapshot_at: number
	queues: unknown[]
	nodes: unknown[]
	raw_bytes: number
	metric_lines: number
	last_error?: string
	last_error_at?: number
	last_successful_at?: number
}

export interface KumoRuntimeAlert {
	id: string
	type: string
	severity: string
	message: string
	tenant_id?: number
	profile_id?: number
	queue?: string
	created_at: number
}

export interface KumoRuntimeQueue {
	queue: string
	tenant_id: number
	campaign_id: number
	api_id: number
	domain: string
	queued: number
	deferred: number
	bounced: number
	complained: number
	oldest_age_seconds: number
	pending_final_events: number
}

export interface KumoRuntimeTenantRisk {
	tenant_id: number
	tenant_name?: string
	queued: number
	delivered: number
	bounced: number
	complained: number
	bounce_per_mille: number
	complaint_ppm: number
	risk: string
}

export interface KumoRuntimeSnapshot {
	snapshot_at: number
	injection: {
		attempts: number
		successes: number
		failures: number
		rate_limited: number
		backpressure_rejected: number
		circuit_rejected: number
		avg_latency_ms: number
		in_flight_global: number
		open_circuit_count: number
	}
	webhook: {
		accepted: number
		duplicates: number
		failed: number
		security_failures: number
		processing_failures: number
		orphaned: number
		redis_dedupe_hits: number
		avg_ingestion_lag_ms: number
	}
	alerts: KumoRuntimeAlert[]
	queues: KumoRuntimeQueue[]
	tenant_risk: KumoRuntimeTenantRisk[]
	release_readiness: {
		ready: boolean
		checks: string[]
		blockers: string[]
		rollback: string[]
		generated_at: number
	}
	collection_warning?: string
}

export interface KumoPolicyFile {
	path: string
	content: string
	sha256: string
}

export interface KumoConfigPreview {
	version: string
	generated_at: number
	generated_by: number
	files: KumoPolicyFile[]
	warnings: string[]
	validation_errors: string[]
}

export interface KumoDeployResult {
	version: string
	status: string
	dry_run: boolean
	deployed_at: number
	rollback_version: string
	message: string
	warnings: string[]
	validation_errors: string[]
}

export const getKumoConfig = () => {
	return instance.get<KumoConfig>('/kumo/config')
}

export const saveKumoConfig = (params: KumoConfig) => {
	return instance.post<KumoConfig>('/kumo/config', params, {
		fetchOptions: {
			successMessage: true,
		},
	})
}

export const testKumoConnection = (params: Partial<KumoConfig>) => {
	return instance.post<KumoTestResult>('/kumo/test_connection', params)
}

export const getKumoStatus = () => {
	return instance.get<KumoStatus>('/kumo/status')
}

export const getKumoMetrics = () => {
	return instance.get<KumoMetrics>('/kumo/metrics')
}

export const getKumoRuntime = () => {
	return instance.get<KumoRuntimeSnapshot>('/kumo/runtime')
}

export const previewKumoConfig = () => {
	return instance.post<KumoConfigPreview>('/kumo/config/preview', {})
}

export const deployKumoConfig = (params: { version?: string; dry_run: boolean }) => {
	return instance.post<KumoDeployResult>('/kumo/config/deploy', params, {
		fetchOptions: {
			successMessage: true,
		},
	})
}
