<template>
	<div class="kumo-page app-card">
		<div class="kumo-header">
			<div class="header-title">
				<i class="i-mdi:email-fast-outline text-6"></i>
				<span>KumoMTA Connection</span>
				<n-tag :type="statusTagType" size="small">{{ statusLabel }}</n-tag>
			</div>
			<n-flex class="kumo-actions">
				<n-button :loading="loading" @click="refreshAll">Refresh</n-button>
				<n-button :loading="testing" type="primary" ghost @click="onTest">Test</n-button>
				<n-button :loading="saving" type="primary" @click="onSave">Save changes</n-button>
			</n-flex>
		</div>

		<div class="section-layout">
			<aside class="section-menu" aria-label="KumoMTA settings sections">
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
				<section v-show="activeSection === 'connection'" class="kumo-section">
					<n-grid :cols="4" :x-gap="16" :y-gap="12" responsive="screen">
						<n-grid-item>
							<n-statistic label="Inject latency" :value="status.inject_latency_ms || 0">
								<template #suffix>ms</template>
							</n-statistic>
						</n-grid-item>
						<n-grid-item>
							<n-statistic label="Metrics latency" :value="status.metrics_latency_ms || 0">
								<template #suffix>ms</template>
							</n-statistic>
						</n-grid-item>
						<n-grid-item>
							<n-statistic label="Metric lines" :value="metrics.metric_lines || 0" />
						</n-grid-item>
						<n-grid-item>
							<n-statistic label="Webhook lag" :value="status.webhook_lag_seconds || 0">
								<template #suffix>s</template>
							</n-statistic>
						</n-grid-item>
					</n-grid>
				</section>

				<section v-show="activeSection === 'configuration'" class="kumo-section">
					<div class="section-heading">
						<div class="section-title">Configuration</div>
					</div>
					<n-form :model="form" label-placement="left" label-width="150" require-mark-placement="right-hanging">
						<n-grid :cols="2" :x-gap="20" responsive="screen">
							<n-form-item-gi :span="2" label="Enabled">
								<n-space vertical>
									<n-checkbox v-model:checked="form.enabled">
										Use KumoMTA for outbound campaign/API sending
									</n-checkbox>
									<n-space>
										<n-checkbox v-model:checked="form.campaigns_enabled">Campaign sending</n-checkbox>
										<n-checkbox v-model:checked="form.api_enabled">Send API</n-checkbox>
									</n-space>
								</n-space>
							</n-form-item-gi>

							<n-form-item-gi label="Base URL">
								<n-input v-model:value="form.base_url" placeholder="https://email.gitdate.ink" />
							</n-form-item-gi>
							<n-form-item-gi label="Injection path">
								<n-input v-model:value="form.inject_path" placeholder="/api/inject/v1" />
							</n-form-item-gi>
							<n-form-item-gi label="Metrics URL">
								<n-input v-model:value="form.metrics_url" placeholder="https://email.gitdate.ink/metrics" />
							</n-form-item-gi>
							<n-form-item-gi label="Default pool">
								<n-input v-model:value="form.default_pool" placeholder="shared-default" />
							</n-form-item-gi>
							<n-form-item-gi label="Auth mode">
								<n-select v-model:value="form.auth_mode" :options="authModeOptions" />
							</n-form-item-gi>
							<n-form-item-gi label="Request timeout">
								<n-input-number v-model:value="form.timeout_ms" :min="100" :max="60000" :step="500" class="w-full">
									<template #suffix>ms</template>
								</n-input-number>
							</n-form-item-gi>
							<n-form-item-gi label="TLS verification">
								<n-switch v-model:value="form.tls_verify" />
							</n-form-item-gi>
							<n-form-item-gi label="Auth secret">
								<n-input
									v-model:value="form.auth_secret"
									type="password"
									show-password-on="click"
									:placeholder="config.has_auth_secret ? 'Configured; leave blank to keep' : 'Bearer token or HMAC secret'" />
							</n-form-item-gi>
							<n-form-item-gi label="Webhook secret">
								<n-input
									v-model:value="form.webhook_secret"
									type="password"
									show-password-on="click"
									:placeholder="config.has_webhook_secret ? 'Configured; leave blank to keep' : 'Webhook token or HMAC secret'" />
							</n-form-item-gi>
						</n-grid>
					</n-form>
				</section>

				<section v-show="activeSection === 'runtime'" class="kumo-section">
					<div class="section-heading">
						<div class="section-title">Runtime Controls</div>
					</div>
					<n-grid :cols="4" :x-gap="16" :y-gap="12" responsive="screen">
						<n-grid-item>
							<n-statistic label="Injection attempts" :value="runtime.injection.attempts || 0" />
						</n-grid-item>
						<n-grid-item>
							<n-statistic label="Injection failures" :value="runtime.injection.failures || 0" />
						</n-grid-item>
						<n-grid-item>
							<n-statistic label="In flight" :value="runtime.injection.in_flight_global || 0" />
						</n-grid-item>
						<n-grid-item>
							<n-statistic label="Circuit rejects" :value="runtime.injection.circuit_rejected || 0" />
						</n-grid-item>
					</n-grid>

					<n-alert v-if="runtime.collection_warning" type="warning" :show-icon="true" :bordered="false" class="mt-16px">
						{{ runtime.collection_warning }}
					</n-alert>
					<n-alert v-if="runtime.release_readiness.blockers.length" type="warning" :show-icon="true" :bordered="false" class="mt-16px">
						<div v-for="blocker in runtime.release_readiness.blockers" :key="blocker">{{ blocker }}</div>
					</n-alert>

					<n-grid :cols="2" :x-gap="16" :y-gap="16" responsive="screen" class="mt-16px">
						<n-grid-item>
							<div class="runtime-section-title">Alerts</div>
							<n-data-table :columns="alertColumns" :data="runtime.alerts" :pagination="{ pageSize: 5 }" />
						</n-grid-item>
						<n-grid-item>
							<div class="runtime-section-title">Queue Age</div>
							<n-data-table :columns="queueColumns" :data="runtime.queues" :pagination="{ pageSize: 5 }" />
						</n-grid-item>
						<n-grid-item>
							<div class="runtime-section-title">Tenant Risk</div>
							<n-data-table :columns="riskColumns" :data="runtime.tenant_risk" :pagination="{ pageSize: 5 }" />
						</n-grid-item>
						<n-grid-item>
							<div class="runtime-section-title">Webhook Health</div>
							<n-descriptions :column="2" size="small" bordered>
								<n-descriptions-item label="Accepted">{{ runtime.webhook.accepted }}</n-descriptions-item>
								<n-descriptions-item label="Duplicates">{{ runtime.webhook.duplicates }}</n-descriptions-item>
								<n-descriptions-item label="Security failures">{{ runtime.webhook.security_failures }}</n-descriptions-item>
								<n-descriptions-item label="Redis dedupe hits">{{ runtime.webhook.redis_dedupe_hits }}</n-descriptions-item>
							</n-descriptions>
						</n-grid-item>
					</n-grid>
				</section>

				<section v-show="activeSection === 'policy'" class="kumo-section">
					<div class="section-heading">
						<div class="section-title">KumoMTA Policy Preview</div>
						<n-flex class="section-actions">
							<n-button :loading="previewing" @click="onPreview">Preview config</n-button>
							<n-button type="primary" ghost :loading="deploying" :disabled="!preview.version" @click="onDryRun">
								Dry run deploy
							</n-button>
						</n-flex>
					</div>

					<n-space vertical :size="12">
						<n-alert type="info" :show-icon="true" :bordered="false">
							Manual deployment remains the v1 workflow. Dry run validates and records the selected preview without mutating KumoMTA.
						</n-alert>
						<n-alert v-if="preview.validation_errors.length" type="error" :show-icon="true" :bordered="false">
							<div v-for="error in preview.validation_errors" :key="error">{{ error }}</div>
						</n-alert>
						<n-alert v-if="preview.warnings.length" type="warning" :show-icon="true" :bordered="false">
							<div v-for="warning in preview.warnings" :key="warning">{{ warning }}</div>
						</n-alert>
						<n-descriptions v-if="preview.version" :column="3" size="small" bordered>
							<n-descriptions-item label="Version">{{ preview.version }}</n-descriptions-item>
							<n-descriptions-item label="Files">{{ preview.files.length }}</n-descriptions-item>
							<n-descriptions-item label="Generated">{{ preview.generated_at ? formatTime(preview.generated_at) : '-' }}</n-descriptions-item>
						</n-descriptions>
						<n-data-table v-if="preview.files.length" :columns="policyFileColumns" :data="preview.files" :pagination="false" />
						<n-form-item v-if="preview.files.length" label="Artifact">
							<n-select v-model:value="selectedPolicyPath" :options="policyFileOptions" class="w-360px max-w-full" />
						</n-form-item>
						<n-input
							v-if="selectedPolicyFile"
							:value="selectedPolicyFile.content"
							type="textarea"
							readonly
							:autosize="{ minRows: 12, maxRows: 24 }" />
						<n-alert v-if="deployResult.message" type="success" :show-icon="true" :bordered="false">
							{{ deployResult.message }}
						</n-alert>
					</n-space>
				</section>

				<section v-show="activeSection === 'checks'" class="kumo-section">
					<div class="section-heading">
						<div class="section-title">Recent checks</div>
					</div>
					<n-data-table :columns="checkColumns" :data="checkRows" :pagination="false" />
				</section>
			</main>
		</div>
	</div>
</template>

<script lang="tsx" setup>
import { DataTableColumns, NTag } from 'naive-ui'
import { formatTime, Message } from '@/utils'
import {
	getKumoConfig,
	getKumoMetrics,
	getKumoRuntime,
	getKumoStatus,
	previewKumoConfig,
	saveKumoConfig,
	testKumoConnection,
	deployKumoConfig,
	type KumoConfigPreview,
	type KumoConfig,
	type KumoDeployResult,
	type KumoPolicyFile,
	type KumoMetrics,
	type KumoRuntimeAlert,
	type KumoRuntimeQueue,
	type KumoRuntimeSnapshot,
	type KumoRuntimeTenantRisk,
	type KumoStatus,
	type KumoTestResult,
} from '@/api/modules/kumo'

interface CheckRow {
	name: string
	result: string
	latency: number
	detail: string
}

const defaultForm = (): KumoConfig => ({
	enabled: false,
	campaigns_enabled: false,
	api_enabled: false,
	base_url: '',
	inject_path: '/api/inject/v1',
	metrics_url: '',
	tls_verify: true,
	auth_mode: 'bearer',
	has_auth_secret: false,
	has_webhook_secret: false,
	auth_secret: '',
	webhook_secret: '',
	timeout_ms: 5000,
	default_pool: '',
})

const form = reactive<KumoConfig>(defaultForm())
const config = reactive<KumoConfig>(defaultForm())
const status = reactive<KumoStatus>({
	connected: false,
	last_ok_at: 0,
	last_error_at: 0,
	last_error: '',
	inject_latency_ms: 0,
	metrics_latency_ms: 0,
	webhook_last_seen_at: 0,
	webhook_lag_seconds: 0,
})
const metrics = reactive<KumoMetrics>({
	snapshot_at: 0,
	queues: [],
	nodes: [],
	raw_bytes: 0,
	metric_lines: 0,
})
const defaultRuntime = (): KumoRuntimeSnapshot => ({
	snapshot_at: 0,
	injection: {
		attempts: 0,
		successes: 0,
		failures: 0,
		rate_limited: 0,
		backpressure_rejected: 0,
		circuit_rejected: 0,
		avg_latency_ms: 0,
		in_flight_global: 0,
		open_circuit_count: 0,
	},
	webhook: {
		accepted: 0,
		duplicates: 0,
		failed: 0,
		security_failures: 0,
		processing_failures: 0,
		orphaned: 0,
		redis_dedupe_hits: 0,
		avg_ingestion_lag_ms: 0,
	},
	alerts: [],
	queues: [],
	tenant_risk: [],
	release_readiness: {
		ready: false,
		checks: [],
		blockers: [],
		rollback: [],
		generated_at: 0,
	},
})
const runtime = reactive<KumoRuntimeSnapshot>(defaultRuntime())

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const previewing = ref(false)
const deploying = ref(false)
const lastTest = ref<KumoTestResult | null>(null)
const preview = reactive<KumoConfigPreview>({
	version: '',
	generated_at: 0,
	generated_by: 0,
	files: [],
	warnings: [],
	validation_errors: [],
})
const deployResult = reactive<KumoDeployResult>({
	version: '',
	status: '',
	dry_run: true,
	deployed_at: 0,
	rollback_version: '',
	message: '',
	warnings: [],
	validation_errors: [],
})
const selectedPolicyPath = ref('')

const authModeOptions = [
	{ label: 'Bearer token', value: 'bearer' },
	{ label: 'HMAC', value: 'hmac' },
	{ label: 'None', value: 'none' },
]

const sectionItems = [
	{ key: 'connection', label: 'Connection', icon: 'i-mdi:connection' },
	{ key: 'configuration', label: 'Configuration', icon: 'i-mdi:tune-variant' },
	{ key: 'runtime', label: 'Runtime', icon: 'i-mdi:chart-timeline-variant' },
	{ key: 'policy', label: 'Policy Preview', icon: 'i-mdi:file-code-outline' },
	{ key: 'checks', label: 'Recent Checks', icon: 'i-mdi:clipboard-check-outline' },
] as const

type KumoSectionKey = (typeof sectionItems)[number]['key']

const activeSection = ref<KumoSectionKey>('connection')

const statusTagType = computed(() => {
	if (status.connected) return 'success'
	if (status.last_error) return 'error'
	return 'default'
})

const statusLabel = computed(() => {
	if (status.connected) return 'Connected'
	if (status.last_error) return 'Error'
	return 'Disconnected'
})

const checkRows = computed<CheckRow[]>(() => [
	{
		name: 'Inject endpoint',
		result: lastTest.value?.inject?.ok || status.connected ? 'OK' : 'Not connected',
		latency: lastTest.value?.inject?.latency_ms || status.inject_latency_ms || 0,
		detail: lastTest.value?.inject?.message || status.last_error || '-',
	},
	{
		name: 'Metrics endpoint',
		result: lastTest.value?.metrics?.ok || metrics.last_successful_at ? 'OK' : 'Not connected',
		latency: lastTest.value?.metrics?.latency_ms || status.metrics_latency_ms || 0,
		detail: lastTest.value?.metrics?.message || metrics.last_error || '-',
	},
	{
		name: 'Webhook receiver',
		result: status.webhook_last_seen_at ? 'Receiving events' : 'No events',
		latency: 0,
		detail: status.webhook_last_seen_at ? formatTime(status.webhook_last_seen_at) : '-',
	},
])

const checkColumns: DataTableColumns<CheckRow> = [
	{ key: 'name', title: 'Check' },
	{
		key: 'result',
		title: 'Result',
		render: row => (
			<NTag size="small" type={row.result === 'OK' || row.result === 'Receiving events' ? 'success' : 'default'}>
				{row.result}
			</NTag>
		),
	},
	{
		key: 'latency',
		title: 'Latency',
		render: row => (row.latency ? `${row.latency}ms` : '-'),
	},
	{ key: 'detail', title: 'Detail', ellipsis: { tooltip: true } },
]

const policyFileColumns: DataTableColumns<KumoPolicyFile> = [
	{ key: 'path', title: 'Path' },
	{ key: 'sha256', title: 'SHA-256', ellipsis: { tooltip: true } },
]

const alertColumns: DataTableColumns<KumoRuntimeAlert> = [
	{
		key: 'severity',
		title: 'Severity',
		render: row => (
			<NTag size="small" type={row.severity === 'critical' ? 'error' : row.severity === 'warning' ? 'warning' : 'default'}>
				{row.severity}
			</NTag>
		),
	},
	{ key: 'type', title: 'Type', ellipsis: { tooltip: true } },
	{ key: 'message', title: 'Message', ellipsis: { tooltip: true } },
]

const queueColumns: DataTableColumns<KumoRuntimeQueue> = [
	{ key: 'queue', title: 'Queue', ellipsis: { tooltip: true } },
	{ key: 'pending_final_events', title: 'Pending' },
	{
		key: 'oldest_age_seconds',
		title: 'Oldest',
		render: row => `${row.oldest_age_seconds || 0}s`,
	},
]

const riskColumns: DataTableColumns<KumoRuntimeTenantRisk> = [
	{
		key: 'risk',
		title: 'Risk',
		render: row => (
			<NTag size="small" type={row.risk === 'high' ? 'error' : row.risk === 'medium' ? 'warning' : 'success'}>
				{row.risk}
			</NTag>
		),
	},
	{
		key: 'tenant',
		title: 'Tenant',
		render: row => row.tenant_name || `Tenant ${row.tenant_id}`,
	},
	{ key: 'queued', title: 'Queued' },
	{ key: 'complaint_ppm', title: 'Complaint ppm' },
]

const policyFileOptions = computed(() =>
	preview.files.map(file => ({
		label: file.path,
		value: file.path,
	}))
)

const selectedPolicyFile = computed(() => {
	return preview.files.find(file => file.path === selectedPolicyPath.value) || preview.files[0]
})

const applyConfig = (data: KumoConfig) => {
	Object.assign(config, data)
	Object.assign(form, data, {
		auth_secret: '',
		webhook_secret: '',
	})
}

const refreshAll = async () => {
	loading.value = true
	try {
		const [configData, statusData, metricsData, runtimeData] = await Promise.all([
			getKumoConfig(),
			getKumoStatus(),
			getKumoMetrics(),
			getKumoRuntime(),
		])
		applyConfig(configData)
		Object.assign(status, statusData)
		Object.assign(metrics, metricsData)
		Object.assign(runtime, runtimeData)
	} finally {
		loading.value = false
	}
}

const onSave = async () => {
	saving.value = true
	try {
		const saved = await saveKumoConfig({ ...form })
		applyConfig(saved)
		await refreshAll()
	} finally {
		saving.value = false
	}
}

const onTest = async () => {
	testing.value = true
	try {
		lastTest.value = await testKumoConnection({ ...form })
		if (lastTest.value.ok) {
			Message.success(lastTest.value.message)
		}
	} finally {
		testing.value = false
	}
}

const onPreview = async () => {
	previewing.value = true
	try {
		const data = await previewKumoConfig()
		Object.assign(preview, data)
		selectedPolicyPath.value = data.files[0]?.path || ''
		deployResult.message = ''
	} finally {
		previewing.value = false
	}
}

const onDryRun = async () => {
	if (!preview.version) return
	deploying.value = true
	try {
		const data = await deployKumoConfig({ version: preview.version, dry_run: true })
		Object.assign(deployResult, data)
	} finally {
		deploying.value = false
	}
}

onMounted(refreshAll)
</script>

<style lang="scss" scoped>
@use '@/styles/index.scss' as base;

.kumo-page {
	width: 100%;
	padding: var(--space-xl);
	box-sizing: border-box;
}

.kumo-header {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: var(--space-md);
	padding-bottom: var(--space-lg);
	margin-bottom: var(--space-lg);
	border-bottom: 1px solid var(--color-border-2);
}

.header-title {
	@include base.row-flex-start;
	flex-wrap: wrap;
	gap: var(--space-sm);
	align-items: center;
	min-width: 0;
	font-size: 16px;
	font-weight: 700;
	color: var(--color-text-1);
}

.kumo-actions {
	flex: 0 0 auto;
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

.kumo-section {
	min-width: 0;
}

.section-heading {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: var(--space-md);
	margin-bottom: var(--space-lg);
}

.section-title,
.runtime-section-title {
	font-weight: 600;
	color: var(--color-text-1);
}

.section-actions {
	flex: 0 0 auto;
}

.runtime-section-title {
	margin-bottom: 8px;
}

@media (max-width: 900px) {
	.kumo-header {
		align-items: flex-start;
		flex-direction: column;
	}

	.kumo-actions,
	.section-actions {
		width: 100%;
	}

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

	.section-heading {
		align-items: flex-start;
		flex-direction: column;
	}
}
</style>
