package database_initialization

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

func init() {
	registerHandler(func() {
		ctx := context.Background()
		if err := initTenantTables(ctx); err != nil {
			g.Log().Error(ctx, "Failed to initialize tenant tables:", err)
			return
		}
		if err := addTenantColumns(ctx); err != nil {
			g.Log().Error(ctx, "Failed to initialize tenant columns:", err)
			return
		}
		if err := backfillDefaultTenant(ctx); err != nil {
			g.Log().Error(ctx, "Failed to backfill default tenant:", err)
			return
		}
	})
}

func initTenantTables(ctx context.Context) error {
	sqlList := []string{
		`CREATE TABLE IF NOT EXISTS tenant_plans (
			id BIGSERIAL PRIMARY KEY,
			name VARCHAR(120) NOT NULL UNIQUE,
			daily_send_limit BIGINT NOT NULL DEFAULT 0,
			monthly_send_limit BIGINT NOT NULL DEFAULT 0,
			max_domains INTEGER NOT NULL DEFAULT 0,
			max_contacts INTEGER NOT NULL DEFAULT 0,
			dedicated_ip_allowed SMALLINT NOT NULL DEFAULT 0,
			create_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			update_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())
		)`,
		`CREATE TABLE IF NOT EXISTS tenants (
			id BIGSERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			slug VARCHAR(255) NOT NULL UNIQUE,
			status VARCHAR(32) NOT NULL DEFAULT 'active',
			plan_id BIGINT NOT NULL DEFAULT 0,
			daily_quota BIGINT NOT NULL DEFAULT 0,
			monthly_quota BIGINT NOT NULL DEFAULT 0,
			default_kumo_pool VARCHAR(255) NOT NULL DEFAULT '',
			sending_status VARCHAR(32) NOT NULL DEFAULT 'active',
			sending_block_reason TEXT NOT NULL DEFAULT '',
			sending_suspended_at INTEGER NOT NULL DEFAULT 0,
			sending_throttle_until INTEGER NOT NULL DEFAULT 0,
			operator_kill_switch SMALLINT NOT NULL DEFAULT 0,
			bounce_threshold_per_mille INTEGER NOT NULL DEFAULT 100,
			complaint_threshold_per_mille INTEGER NOT NULL DEFAULT 1,
			create_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			update_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())
		)`,
		`CREATE TABLE IF NOT EXISTS tenant_members (
			id BIGSERIAL PRIMARY KEY,
			tenant_id BIGINT NOT NULL,
			account_id BIGINT NOT NULL,
			role VARCHAR(32) NOT NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'active',
			create_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			update_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			UNIQUE (tenant_id, account_id)
		)`,
		`CREATE TABLE IF NOT EXISTS tenant_invitations (
			id BIGSERIAL PRIMARY KEY,
			tenant_id BIGINT NOT NULL,
			email VARCHAR(255) NOT NULL,
			role VARCHAR(32) NOT NULL,
			token_hash VARCHAR(255) NOT NULL,
			invited_by_account_id BIGINT NOT NULL DEFAULT 0,
			status VARCHAR(32) NOT NULL DEFAULT 'pending',
			expires_at INTEGER NOT NULL DEFAULT 0,
			accepted_at INTEGER NOT NULL DEFAULT 0,
			create_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			update_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())
		)`,
		`CREATE TABLE IF NOT EXISTS tenant_usage_daily (
			id BIGSERIAL PRIMARY KEY,
			tenant_id BIGINT NOT NULL,
			date DATE NOT NULL,
			queued_count BIGINT NOT NULL DEFAULT 0,
			delivered_count BIGINT NOT NULL DEFAULT 0,
			bounced_count BIGINT NOT NULL DEFAULT 0,
			complained_count BIGINT NOT NULL DEFAULT 0,
			api_count BIGINT NOT NULL DEFAULT 0,
			campaign_count BIGINT NOT NULL DEFAULT 0,
			create_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			update_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			UNIQUE (tenant_id, date)
		)`,
		`CREATE TABLE IF NOT EXISTS tenant_suppression_list (
			id BIGSERIAL PRIMARY KEY,
			tenant_id BIGINT NOT NULL,
			email VARCHAR(255) NOT NULL,
			reason VARCHAR(64) NOT NULL DEFAULT '',
			source VARCHAR(64) NOT NULL DEFAULT '',
			create_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			update_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			UNIQUE (tenant_id, email)
		)`,
		`CREATE TABLE IF NOT EXISTS tenant_api_keys (
			id BIGSERIAL PRIMARY KEY,
			tenant_id BIGINT NOT NULL,
			api_key_hash VARCHAR(255) NOT NULL UNIQUE,
			name VARCHAR(255) NOT NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'active',
			scopes TEXT NOT NULL DEFAULT '',
			last_used_at INTEGER NOT NULL DEFAULT 0,
			expires_at INTEGER NOT NULL DEFAULT 0,
			create_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			update_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())
		)`,
		`CREATE TABLE IF NOT EXISTS kumo_nodes (
			id BIGSERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL UNIQUE,
			base_url TEXT NOT NULL DEFAULT '',
			metrics_url TEXT NOT NULL DEFAULT '',
			status VARCHAR(32) NOT NULL DEFAULT 'unknown',
			last_ok_at INTEGER NOT NULL DEFAULT 0,
			last_error_at INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			create_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			update_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())
		)`,
		`CREATE TABLE IF NOT EXISTS kumo_egress_sources (
			id BIGSERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL UNIQUE,
			source_address VARCHAR(128) NOT NULL DEFAULT '',
			ehlo_domain VARCHAR(255) NOT NULL DEFAULT '',
			node_id BIGINT NOT NULL DEFAULT 0,
			status VARCHAR(32) NOT NULL DEFAULT 'active',
			warmup_status VARCHAR(32) NOT NULL DEFAULT '',
			create_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			update_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())
		)`,
		`CREATE TABLE IF NOT EXISTS kumo_egress_pools (
			id BIGSERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			status VARCHAR(32) NOT NULL DEFAULT 'active',
			create_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			update_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())
		)`,
		`CREATE TABLE IF NOT EXISTS kumo_egress_pool_sources (
			id BIGSERIAL PRIMARY KEY,
			pool_id BIGINT NOT NULL,
			source_id BIGINT NOT NULL,
			weight INTEGER NOT NULL DEFAULT 100,
			status VARCHAR(32) NOT NULL DEFAULT 'active',
			UNIQUE (pool_id, source_id)
		)`,
		`CREATE TABLE IF NOT EXISTS tenant_allowed_kumo_pools (
			id BIGSERIAL PRIMARY KEY,
			tenant_id BIGINT NOT NULL,
			pool_id BIGINT NOT NULL,
			create_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			update_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			UNIQUE (tenant_id, pool_id)
		)`,
		`CREATE TABLE IF NOT EXISTS tenant_sending_profiles (
			id BIGSERIAL PRIMARY KEY,
			tenant_id BIGINT NOT NULL,
			name VARCHAR(255) NOT NULL,
			default_from_domain_id BIGINT NOT NULL DEFAULT 0,
			default_from_domain VARCHAR(255) NOT NULL DEFAULT '',
			kumo_pool_id BIGINT NOT NULL DEFAULT 0,
			kumo_pool_name VARCHAR(255) NOT NULL DEFAULT '',
			egress_mode VARCHAR(64) NOT NULL DEFAULT 'external_kumoproxy',
			egress_provider VARCHAR(128) NOT NULL DEFAULT '',
			dkim_selector VARCHAR(128) NOT NULL DEFAULT '',
			daily_quota BIGINT NOT NULL DEFAULT 0,
			hourly_quota BIGINT NOT NULL DEFAULT 0,
			warmup_enabled SMALLINT NOT NULL DEFAULT 0,
			status VARCHAR(32) NOT NULL DEFAULT 'draft',
			paused_reason TEXT NOT NULL DEFAULT '',
			throttle_until INTEGER NOT NULL DEFAULT 0,
			suspended_at INTEGER NOT NULL DEFAULT 0,
			operator_kill_switch SMALLINT NOT NULL DEFAULT 0,
			bounce_threshold_per_mille INTEGER NOT NULL DEFAULT 100,
			complaint_threshold_per_mille INTEGER NOT NULL DEFAULT 1,
			create_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			update_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			UNIQUE (tenant_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS tenant_sending_profile_domains (
			id BIGSERIAL PRIMARY KEY,
			profile_id BIGINT NOT NULL,
			domain_id BIGINT NOT NULL,
			domain_name VARCHAR(255) NOT NULL DEFAULT '',
			create_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			update_time INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
			UNIQUE (profile_id, domain_name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_members_account ON tenant_members(account_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_members_tenant ON tenant_members(tenant_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_usage_daily_tenant_date ON tenant_usage_daily(tenant_id, date)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_invitations_tenant_email ON tenant_invitations(tenant_id, email)`,
		`CREATE INDEX IF NOT EXISTS idx_kumo_egress_pool_sources_pool ON kumo_egress_pool_sources(pool_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_kumo_egress_pool_sources_source ON kumo_egress_pool_sources(source_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_allowed_kumo_pools_tenant ON tenant_allowed_kumo_pools(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_sending_profiles_tenant ON tenant_sending_profiles(tenant_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_sending_profile_domains_profile ON tenant_sending_profile_domains(profile_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_sending_profile_domains_domain ON tenant_sending_profile_domains(domain_id)`,
	}
	for _, sql := range sqlList {
		if _, err := g.DB().Exec(ctx, sql); err != nil {
			return err
		}
	}
	return nil
}

func addTenantColumns(ctx context.Context) error {
	tenantColumns := []struct {
		name         string
		columnType   string
		defaultValue string
		notNull      bool
	}{
		{"sending_status", "VARCHAR(32)", "'active'", true},
		{"sending_block_reason", "TEXT", "''", true},
		{"sending_suspended_at", "INTEGER", "0", true},
		{"sending_throttle_until", "INTEGER", "0", true},
		{"operator_kill_switch", "SMALLINT", "0", true},
		{"bounce_threshold_per_mille", "INTEGER", "100", true},
		{"complaint_threshold_per_mille", "INTEGER", "1", true},
	}
	for _, column := range tenantColumns {
		if err := AddColumnIfNotExists("tenants", column.name, column.columnType, column.defaultValue, column.notNull); err != nil {
			return err
		}
	}
	if checkTableExists(ctx, "tenant_sending_profiles") {
		if err := AddColumnIfNotExists("tenant_sending_profiles", "default_from_domain", "VARCHAR(255)", "''", true); err != nil {
			return err
		}
	}
	if checkTableExists(ctx, "tenant_sending_profile_domains") {
		if err := AddColumnIfNotExists("tenant_sending_profile_domains", "domain_name", "VARCHAR(255)", "''", true); err != nil {
			return err
		}
	}

	for _, table := range []string{
		"domain",
		"mailbox",
		"alias",
		"alias_domain",
		"bm_bcc",
		"bm_relay",
		"bm_relay_config",
		"bm_relay_domain_mapping",
		"bm_domain_smtp_transport",
		"bm_multi_ip_domain",
		"bm_sender_ip_warmup",
		"bm_sender_ip_mail_provider",
		"bm_campaign_warmup",
		"bm_contact_groups",
		"bm_contacts",
		"bm_tags",
		"bm_contact_tags",
		"email_templates",
		"email_tasks",
		"recipient_info",
		"unsubscribe_records",
		"abnormal_recipient",
		"api_templates",
		"api_mail_logs",
		"api_ip_whitelist",
		"mailstat_send_mails",
		"mailstat_receive_mails",
		"mailstat_message_ids",
		"mailstat_opened",
		"mailstat_clicked",
		"bm_operation_logs",
	} {
		if !checkTableExists(ctx, table) {
			continue
		}
		if err := AddColumnIfNotExists(table, "tenant_id", "BIGINT", "0", true); err != nil {
			return err
		}
		if _, err := g.DB().Exec(ctx, "CREATE INDEX IF NOT EXISTS idx_"+table+"_tenant_id ON "+table+"(tenant_id)"); err != nil {
			return err
		}
	}
	return nil
}

func backfillDefaultTenant(ctx context.Context) error {
	now := time.Now().Unix()
	planID, err := ensureDefaultTenantPlan(ctx, now)
	if err != nil {
		return err
	}
	tenantID, err := ensureDefaultTenant(ctx, planID, now)
	if err != nil {
		return err
	}
	if err := backfillTenantMembers(ctx, tenantID, now); err != nil {
		return err
	}
	if err := backfillTenantOwnedRows(ctx, tenantID); err != nil {
		return err
	}
	return nil
}

func ensureDefaultTenantPlan(ctx context.Context, now int64) (int64, error) {
	value, err := g.DB().Model("tenant_plans").Ctx(ctx).Where("name", "Default").Value("id")
	if err == nil && value != nil {
		return value.Int64(), nil
	}
	result, err := g.DB().Model("tenant_plans").Ctx(ctx).InsertIgnore(g.Map{
		"name":                 "Default",
		"daily_send_limit":     0,
		"monthly_send_limit":   0,
		"max_domains":          0,
		"max_contacts":         0,
		"dedicated_ip_allowed": 0,
		"create_time":          now,
		"update_time":          now,
	})
	if err != nil {
		return 0, err
	}
	id, _ := result.LastInsertId()
	if id > 0 {
		return id, nil
	}
	value, err = g.DB().Model("tenant_plans").Ctx(ctx).Where("name", "Default").Value("id")
	if err != nil {
		return 0, err
	}
	return value.Int64(), nil
}

func ensureDefaultTenant(ctx context.Context, planID int64, now int64) (int64, error) {
	value, err := g.DB().Model("tenants").Ctx(ctx).Where("slug", "default-workspace").Value("id")
	if err == nil && value != nil {
		return value.Int64(), nil
	}
	result, err := g.DB().Model("tenants").Ctx(ctx).InsertIgnore(g.Map{
		"name":          "Default Workspace",
		"slug":          "default-workspace",
		"status":        "active",
		"plan_id":       planID,
		"daily_quota":   0,
		"monthly_quota": 0,
		"create_time":   now,
		"update_time":   now,
	})
	if err != nil {
		return 0, err
	}
	id, _ := result.LastInsertId()
	if id > 0 {
		return id, nil
	}
	value, err = g.DB().Model("tenants").Ctx(ctx).Where("slug", "default-workspace").Value("id")
	if err != nil {
		return 0, err
	}
	return value.Int64(), nil
}

func backfillTenantMembers(ctx context.Context, tenantID int64, now int64) error {
	var accounts []struct {
		AccountId int64 `json:"account_id"`
	}
	if err := g.DB().Model("account").Ctx(ctx).Fields("account_id").Scan(&accounts); err != nil {
		return err
	}
	for _, account := range accounts {
		role := "admin"
		adminCount, err := g.DB().Model("account_role ar").
			Ctx(ctx).
			LeftJoin("role r", "r.role_id = ar.role_id").
			Where("ar.account_id", account.AccountId).
			Where("r.role_name", "admin").
			Count()
		if err != nil {
			return err
		}
		if adminCount > 0 {
			role = "owner"
		}
		if _, err = g.DB().Model("tenant_members").Ctx(ctx).InsertIgnore(g.Map{
			"tenant_id":   tenantID,
			"account_id":  account.AccountId,
			"role":        role,
			"status":      "active",
			"create_time": now,
			"update_time": now,
		}); err != nil {
			return err
		}
	}
	return nil
}

func backfillTenantOwnedRows(ctx context.Context, tenantID int64) error {
	for _, table := range []string{
		"domain",
		"mailbox",
		"alias",
		"alias_domain",
		"bm_bcc",
		"bm_relay",
		"bm_relay_config",
		"bm_relay_domain_mapping",
		"bm_domain_smtp_transport",
		"bm_multi_ip_domain",
		"bm_sender_ip_warmup",
		"bm_sender_ip_mail_provider",
		"bm_campaign_warmup",
		"bm_contact_groups",
		"bm_contacts",
		"bm_tags",
		"bm_contact_tags",
		"email_templates",
		"email_tasks",
		"recipient_info",
		"unsubscribe_records",
		"abnormal_recipient",
		"api_templates",
		"api_mail_logs",
		"api_ip_whitelist",
		"mailstat_send_mails",
		"mailstat_receive_mails",
		"mailstat_message_ids",
		"mailstat_opened",
		"mailstat_clicked",
		"bm_operation_logs",
	} {
		if !checkTableExists(ctx, table) {
			continue
		}
		if _, err := g.DB().Exec(ctx, "UPDATE "+table+" SET tenant_id = ? WHERE tenant_id = 0 OR tenant_id IS NULL", tenantID); err != nil {
			return err
		}
	}

	relationalBackfillSQL := []string{
		`UPDATE recipient_info ri SET tenant_id = et.tenant_id FROM email_tasks et WHERE ri.task_id = et.id AND (ri.tenant_id = 0 OR ri.tenant_id IS NULL)`,
		`UPDATE bm_contacts c SET tenant_id = g.tenant_id FROM bm_contact_groups g WHERE c.group_id = g.id AND (c.tenant_id = 0 OR c.tenant_id IS NULL)`,
		`UPDATE bm_tags t SET tenant_id = g.tenant_id FROM bm_contact_groups g WHERE t.group_id = g.id AND (t.tenant_id = 0 OR t.tenant_id IS NULL)`,
		`UPDATE bm_contact_tags ct SET tenant_id = c.tenant_id FROM bm_contacts c WHERE ct.contact_id = c.id AND (ct.tenant_id = 0 OR ct.tenant_id IS NULL)`,
		`UPDATE api_mail_logs aml SET tenant_id = at.tenant_id FROM api_templates at WHERE aml.api_id = at.id AND (aml.tenant_id = 0 OR aml.tenant_id IS NULL)`,
		`UPDATE api_ip_whitelist aiw SET tenant_id = at.tenant_id FROM api_templates at WHERE aiw.api_id = at.id AND (aiw.tenant_id = 0 OR aiw.tenant_id IS NULL)`,
	}
	for _, sql := range relationalBackfillSQL {
		if _, err := g.DB().Exec(ctx, sql); err != nil {
			return err
		}
	}
	return nil
}
