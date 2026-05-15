package database_initialization

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
)

func init() {
	registerHandler(func() {
		sqlList := []string{
			`CREATE TABLE IF NOT EXISTS kumo_webhook_events (
				id SERIAL PRIMARY KEY,
				event_hash VARCHAR(64) NOT NULL,
				event_id VARCHAR(255) NOT NULL DEFAULT '',
				raw_event JSONB NOT NULL DEFAULT '{}'::jsonb,
				body_size INTEGER NOT NULL DEFAULT 0,
				remote_ip VARCHAR(64) NOT NULL DEFAULT '',
				received_at INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
				UNIQUE(event_hash)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_webhook_events_event_id ON kumo_webhook_events(event_id)`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_webhook_events_received_at ON kumo_webhook_events(received_at)`,
			`CREATE TABLE IF NOT EXISTS kumo_message_injections (
				id SERIAL PRIMARY KEY,
				tenant_id INTEGER NOT NULL DEFAULT 0,
				message_id TEXT NOT NULL DEFAULT '',
				recipient VARCHAR(320) NOT NULL DEFAULT '',
				recipient_domain VARCHAR(255) NOT NULL DEFAULT '',
				campaign_id INTEGER NOT NULL DEFAULT 0,
				task_id INTEGER NOT NULL DEFAULT 0,
				recipient_info_id INTEGER NOT NULL DEFAULT 0,
				api_id INTEGER NOT NULL DEFAULT 0,
				api_log_id INTEGER NOT NULL DEFAULT 0,
				sending_profile_id INTEGER NOT NULL DEFAULT 0,
				queue_name TEXT NOT NULL DEFAULT '',
				injection_status VARCHAR(32) NOT NULL DEFAULT 'pending',
				delivery_status VARCHAR(32) NOT NULL DEFAULT 'pending',
				attempt_count INTEGER NOT NULL DEFAULT 0,
				next_retry_at INTEGER NOT NULL DEFAULT 0,
				accepted_at INTEGER NOT NULL DEFAULT 0,
				final_event_at INTEGER NOT NULL DEFAULT 0,
				last_error TEXT NOT NULL DEFAULT '',
				created_at INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
				updated_at INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
				UNIQUE(message_id, recipient)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_message_injections_tenant_id ON kumo_message_injections(tenant_id)`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_message_injections_message_id ON kumo_message_injections(message_id)`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_message_injections_recipient_info_id ON kumo_message_injections(recipient_info_id)`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_message_injections_api_log_id ON kumo_message_injections(api_log_id)`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_message_injections_status ON kumo_message_injections(injection_status, delivery_status)`,
			`ALTER TABLE kumo_message_injections DROP CONSTRAINT IF EXISTS kumo_message_injections_message_id_recipient_key`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_kumo_message_injections_tenant_message_recipient ON kumo_message_injections(tenant_id, message_id, recipient)`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_message_injections_tenant_queue ON kumo_message_injections(tenant_id, queue_name)`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_message_injections_tenant_status ON kumo_message_injections(tenant_id, injection_status, delivery_status)`,
			`CREATE TABLE IF NOT EXISTS kumo_delivery_events (
				id SERIAL PRIMARY KEY,
				tenant_id INTEGER NOT NULL DEFAULT 0,
				provider_event_id VARCHAR(255) NOT NULL DEFAULT '',
				event_hash VARCHAR(64) NOT NULL,
				event_type VARCHAR(64) NOT NULL DEFAULT '',
				delivery_status VARCHAR(32) NOT NULL DEFAULT 'unknown',
				message_id TEXT NOT NULL DEFAULT '',
				recipient VARCHAR(320) NOT NULL DEFAULT '',
				queue_name TEXT NOT NULL DEFAULT '',
				campaign_id INTEGER NOT NULL DEFAULT 0,
				task_id INTEGER NOT NULL DEFAULT 0,
				recipient_info_id INTEGER NOT NULL DEFAULT 0,
				api_id INTEGER NOT NULL DEFAULT 0,
				api_log_id INTEGER NOT NULL DEFAULT 0,
				response TEXT NOT NULL DEFAULT '',
				remote_mx TEXT NOT NULL DEFAULT '',
				raw_event JSONB NOT NULL DEFAULT '{}'::jsonb,
				orphaned BOOLEAN NOT NULL DEFAULT false,
				occurred_at INTEGER NOT NULL DEFAULT 0,
				ingested_at INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
				UNIQUE(event_hash)
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_kumo_delivery_events_provider_event_id ON kumo_delivery_events(provider_event_id) WHERE provider_event_id <> ''`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_delivery_events_message_id ON kumo_delivery_events(message_id)`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_delivery_events_recipient_info_id ON kumo_delivery_events(recipient_info_id)`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_delivery_events_api_log_id ON kumo_delivery_events(api_log_id)`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_delivery_events_orphaned ON kumo_delivery_events(orphaned)`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_delivery_events_occurred_at ON kumo_delivery_events(occurred_at)`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_delivery_events_tenant_occurred_at ON kumo_delivery_events(tenant_id, occurred_at)`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_delivery_events_tenant_status ON kumo_delivery_events(tenant_id, delivery_status, occurred_at)`,
			`CREATE TABLE IF NOT EXISTS kumo_config_versions (
				id SERIAL PRIMARY KEY,
				version VARCHAR(64) NOT NULL UNIQUE,
				generated_by_account_id INTEGER NOT NULL DEFAULT 0,
				status VARCHAR(32) NOT NULL DEFAULT 'preview',
				preview JSONB NOT NULL DEFAULT '{}'::jsonb,
				deployed_at INTEGER NOT NULL DEFAULT 0,
				error TEXT NOT NULL DEFAULT '',
				rollback_version VARCHAR(64) NOT NULL DEFAULT '',
				dry_run SMALLINT NOT NULL DEFAULT 0,
				create_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
				update_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())
			)`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_config_versions_status ON kumo_config_versions(status, create_time)`,
			`CREATE INDEX IF NOT EXISTS idx_kumo_config_versions_generated_by ON kumo_config_versions(generated_by_account_id)`,
		}

		for _, sql := range sqlList {
			if _, err := g.DB().Exec(context.Background(), sql); err != nil {
				g.Log().Error(context.Background(), "Failed to execute KumoMTA SQL:", err, sql)
				return
			}
		}

		g.Log().Info(context.Background(), "KumoMTA tables initialized successfully")
	})
}
