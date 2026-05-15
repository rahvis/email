export interface Api {
	id: number
	api_key: string
	api_name: string
	template_id: number
	tenant_id: number
	subject: string
	addresser: string
	full_name: string
	server_addresser: string
	unsubscribe: number
	track_open: number
	track_click: number
	active: number
	create_time: number
	update_time: number
	expire_time: number
	send_count: number
	success_count: number
	fail_count: number
	open_rate: number
	click_rate: number
	delivery_rate: number
	bounce_rate: number
	group_id: number
	ip_whitelist: string[]
	delivery_engine: 'tenant_default' | 'kumomta' | 'postfix' | 'local' | 'smtp' | 'kumo'
	sending_profile_id: number
	queued_count: number
	delivered_count: number
	deferred_count: number
	bounced_count: number
	expired_count: number
	complained_count: number
	webhook_last_seen_at: number
	webhook_lag_seconds: number
	webhook_healthy: boolean
}

export interface ApiParams {
	page: number
	page_size: number
	keyword: string
	active: number
	time_range: [number, number]
}

export interface OverviewStats {
	total_send: number
	avg_delivery_rate: number
	avg_open_rate: number
	avg_click_rate: number
	avg_bounce_rate: number
	avg_unsub_rate: number
	total_unsubscribe: number
}
