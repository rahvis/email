import { instance } from '@/api'

export interface TenantContext {
	tenant_id: number
	tenant_name: string
	tenant_slug: string
	role: string
	permissions: string[]
	plan: string
	daily_quota: number
	daily_used: number
	status: string
	is_operator: boolean
	sending_status: string
	sending_block_reason: string
	sending_throttle_until: number
}

export interface TenantMembership {
	tenant_id: number
	name: string
	slug: string
	role: string
	status: string
	plan: string
}

export interface TenantCreateParams {
	name: string
	owner_email?: string
	plan_id?: number
	daily_quota?: number
	monthly_quota?: number
	default_kumo_pool?: string
	status?: string
}

export interface ReadinessCheck {
	name: string
	ready: boolean
	message: string
}

export interface DomainReadiness {
	domain: string
	ready: boolean
	checks: ReadinessCheck[]
}

export interface ProfileReadiness {
	ready: boolean
	checks: ReadinessCheck[]
	domains: DomainReadiness[]
}

export interface SendingProfile {
	id: number
	tenant_id: number
	name: string
	default_from_domain: string
	kumo_pool_id: number
	kumo_pool_name: string
	egress_mode: string
	egress_provider: string
	dkim_selector: string
	daily_quota: number
	hourly_quota: number
	warmup_enabled: boolean
	status: string
	paused_reason: string
	throttle_until: number
	operator_kill_switch: boolean
	bounce_threshold_per_mille: number
	complaint_threshold_per_mille: number
	domains: string[]
	create_time: number
	update_time: number
}

export interface SendingProfileParams {
	profile_id?: number
	name: string
	sender_domains: string[]
	default_from_domain?: string
	kumo_pool_id?: number
	kumo_pool?: string
	egress_mode: string
	egress_provider?: string
	dkim_selector: string
	daily_quota?: number
	hourly_quota?: number
	warmup_enabled?: boolean
	status?: string
	bounce_threshold_per_mille?: number
	complaint_threshold_per_mille?: number
}

export interface SendingControlParams {
	profile_id?: number
	status: string
	reason?: string
	throttle_until?: number
	operator_kill_switch?: boolean
}

export const getCurrentTenant = () => {
	return instance.get<TenantContext>('/tenants/current')
}

export const getTenants = () => {
	return instance.get<{ tenants: TenantMembership[] }>('/tenants')
}

export const switchTenant = (tenant_id: number) => {
	return instance.post<TenantContext>('/tenants/switch', { tenant_id })
}

export const createTenant = (params: TenantCreateParams) => {
	return instance.post('/tenants', params, {
		fetchOptions: {
			successMessage: true,
		},
	})
}

export const getSendingProfiles = (tenantID: number) => {
	return instance.get<{ profiles: SendingProfile[] }>(`/tenants/${tenantID}/sending-profiles`)
}

export const saveSendingProfile = (tenantID: number, params: SendingProfileParams) => {
	return instance.post<{ profile: SendingProfile; readiness: ProfileReadiness }>(`/tenants/${tenantID}/sending-profile`, params, {
		fetchOptions: {
			successMessage: true,
		},
	})
}

export const updateSendingControl = (tenantID: number, params: SendingControlParams) => {
	return instance.post(`/tenants/${tenantID}/sending-control`, params, {
		fetchOptions: {
			successMessage: true,
		},
	})
}
