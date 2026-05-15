package kumo

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	runtimeRedisTTLSeconds = 48 * 60 * 60
	maxRuntimeAlerts       = 100

	alertSeverityInfo     = "info"
	alertSeverityWarning  = "warning"
	alertSeverityCritical = "critical"
)

var (
	defaultRuntimeLimits = RuntimeLimits{
		GlobalConcurrency:       200,
		TenantConcurrency:       50,
		ProfilePerSecond:        100,
		CircuitFailureThreshold: 5,
		CircuitWindowSeconds:    120,
		CircuitOpenSeconds:      60,
		QueueAgeAlertSeconds:    15 * 60,
		WebhookIdleSeconds:      5 * 60,
		SignatureFailureWindow:  5 * 60,
		SignatureFailureSpike:   10,
	}

	runtimeMetricsMu sync.RWMutex
	runtimeMetrics   = runtimeMetricsState{
		injectionBuckets: map[string]*injectionBucketState{},
		webhookTypes:     map[string]int64{},
		alerts:           []RuntimeAlert{},
	}
)

type InjectionControlInput struct {
	TenantID         int64
	SendingProfileID int64
	Engine           string
	Queue            string
	RecipientDomain  string
}

type InjectionSlot struct {
	TenantID         int64
	SendingProfileID int64
	GlobalKey        string
	TenantKey        string
	AcquiredGlobal   bool
	AcquiredTenant   bool
	Released         bool
}

type runtimeMetricsState struct {
	attempts             int64
	successes            int64
	failures             int64
	rateLimited          int64
	backpressureRejected int64
	circuitRejected      int64
	latencyTotalMS       int64
	inFlightGlobal       int64
	openCircuitCount     int64

	webhookAccepted           int64
	webhookDuplicates         int64
	webhookFailed             int64
	webhookSecurityFailures   int64
	webhookProcessingFailures int64
	webhookOrphaned           int64
	webhookRedisDedupeHits    int64
	webhookLagTotalMS         int64
	webhookLagSamples         int64

	injectionBuckets map[string]*injectionBucketState
	webhookTypes     map[string]int64
	alerts           []RuntimeAlert
}

type injectionBucketState struct {
	tenantID         int64
	sendingProfileID int64
	engine           string
	attempts         int64
	successes        int64
	failures         int64
	latencyTotalMS   int64
}

func AcquireInjectionSlot(ctx context.Context, input InjectionControlInput) (*InjectionSlot, error) {
	input = normalizeInjectionControlInput(input)
	if err := checkCircuit(ctx, input); err != nil {
		recordRuntimeReject("circuit", input, err.Error())
		return nil, err
	}

	limits := CurrentRuntimeLimits()
	slot := &InjectionSlot{
		TenantID:         input.TenantID,
		SendingProfileID: input.SendingProfileID,
		GlobalKey:        "kumo:inject:inflight:global",
		TenantKey:        fmt.Sprintf("kumo:inject:inflight:tenant:%d", input.TenantID),
	}

	global, err := redisIncrWithTTL(ctx, slot.GlobalKey, 5*60)
	if err != nil {
		return nil, err
	}
	slot.AcquiredGlobal = true
	if reason := EvaluateInjectionWindow(global, 0, limits.GlobalConcurrency, 0); reason != "" {
		ReleaseInjectionSlot(ctx, slot)
		recordRuntimeReject("backpressure", input, reason)
		return nil, gerror.New(reason)
	}

	tenant, err := redisIncrWithTTL(ctx, slot.TenantKey, 5*60)
	if err != nil {
		ReleaseInjectionSlot(ctx, slot)
		return nil, err
	}
	slot.AcquiredTenant = true
	if reason := EvaluateInjectionWindow(global, tenant, limits.GlobalConcurrency, limits.TenantConcurrency); reason != "" {
		ReleaseInjectionSlot(ctx, slot)
		recordRuntimeReject("backpressure", input, reason)
		return nil, gerror.New(reason)
	}

	rateKey := fmt.Sprintf("kumo:inject:rate:tenant:%d:profile:%d:%d", input.TenantID, input.SendingProfileID, time.Now().Unix())
	rate, err := redisIncrWithTTL(ctx, rateKey, 3)
	if err != nil {
		ReleaseInjectionSlot(ctx, slot)
		return nil, err
	}
	if limits.ProfilePerSecond > 0 && rate > limits.ProfilePerSecond {
		ReleaseInjectionSlot(ctx, slot)
		recordRuntimeReject("rate_limit", input, fmt.Sprintf("profile injection rate exceeded: %d of %d per second", rate, limits.ProfilePerSecond))
		return nil, gerror.New("profile injection rate exceeded")
	}

	runtimeMetricsMu.Lock()
	runtimeMetrics.inFlightGlobal++
	runtimeMetricsMu.Unlock()

	return slot, nil
}

func ReleaseInjectionSlot(ctx context.Context, slot *InjectionSlot) {
	if slot == nil || slot.Released {
		return
	}
	if slot.AcquiredTenant && slot.TenantKey != "" {
		_, _ = g.Redis().Decr(ctx, slot.TenantKey)
	}
	if slot.AcquiredGlobal && slot.GlobalKey != "" {
		_, _ = g.Redis().Decr(ctx, slot.GlobalKey)
	}
	slot.Released = true

	runtimeMetricsMu.Lock()
	if runtimeMetrics.inFlightGlobal > 0 {
		runtimeMetrics.inFlightGlobal--
	}
	runtimeMetricsMu.Unlock()
}

func RecordInjectionAttempt(ctx context.Context, input InjectionControlInput) {
	input = normalizeInjectionControlInput(input)
	runtimeMetricsMu.Lock()
	runtimeMetrics.attempts++
	bucket := getInjectionBucketLocked(input)
	bucket.attempts++
	runtimeMetricsMu.Unlock()

	redisMetricIncr(ctx, fmt.Sprintf("kumo:metrics:inject:attempts:tenant:%d:profile:%d:engine:%s", input.TenantID, input.SendingProfileID, input.Engine))
}

func RecordInjectionResult(ctx context.Context, input InjectionControlInput, success bool, latencyMS int64, err error) {
	input = normalizeInjectionControlInput(input)
	if latencyMS < 0 {
		latencyMS = 0
	}

	runtimeMetricsMu.Lock()
	bucket := getInjectionBucketLocked(input)
	if success {
		runtimeMetrics.successes++
		bucket.successes++
	} else {
		runtimeMetrics.failures++
		bucket.failures++
	}
	runtimeMetrics.latencyTotalMS += latencyMS
	bucket.latencyTotalMS += latencyMS
	runtimeMetricsMu.Unlock()

	status := "failure"
	if success {
		status = "success"
	}
	redisMetricIncr(ctx, fmt.Sprintf("kumo:metrics:inject:%s:tenant:%d:profile:%d:engine:%s", status, input.TenantID, input.SendingProfileID, input.Engine))
	if success {
		_ = redisDel(ctx, failureKey(input))
		return
	}

	failures, redisErr := redisIncrWithTTL(ctx, failureKey(input), CurrentRuntimeLimits().CircuitWindowSeconds)
	if redisErr != nil {
		g.Log().Warningf(ctx, "KumoMTA circuit failure counter unavailable: tenant_id=%d profile_id=%d error=%s", input.TenantID, input.SendingProfileID, sanitizeLogText(redisErr.Error()))
		return
	}
	if ShouldOpenCircuit(failures, CurrentRuntimeLimits().CircuitFailureThreshold) {
		_ = redisSetEX(ctx, circuitKey(input), "1", CurrentRuntimeLimits().CircuitOpenSeconds)
		runtimeMetricsMu.Lock()
		runtimeMetrics.openCircuitCount++
		runtimeMetricsMu.Unlock()
		RecordRuntimeAlert(RuntimeAlert{
			Type:      "kumo_circuit_open",
			Severity:  alertSeverityCritical,
			TenantID:  input.TenantID,
			ProfileID: input.SendingProfileID,
			Queue:     input.Queue,
			Message:   fmt.Sprintf("KumoMTA injection circuit opened after %d repeated failures", failures),
		})
	}
	if err != nil {
		g.Log().Warningf(ctx, "KumoMTA injection failure metric recorded: tenant_id=%d profile_id=%d queue=%s latency_ms=%d error=%s",
			input.TenantID, input.SendingProfileID, input.Queue, latencyMS, sanitizeLogText(err.Error()))
	}
}

func RecordWebhookIngestMetrics(ctx context.Context, events []NormalizedDeliveryEvent, result *WebhookIngestResult) {
	if result == nil {
		return
	}
	now := time.Now().Unix()
	runtimeMetricsMu.Lock()
	runtimeMetrics.webhookAccepted += int64(result.Accepted)
	runtimeMetrics.webhookDuplicates += int64(result.Duplicates)
	runtimeMetrics.webhookFailed += int64(result.Failed)
	for _, event := range events {
		eventType := strings.ToLower(strings.TrimSpace(event.EventType))
		if eventType == "" {
			eventType = "unknown"
		}
		runtimeMetrics.webhookTypes[eventType]++
		if event.OccurredAt > 0 {
			lagMS := (now - event.OccurredAt) * 1000
			if lagMS >= 0 {
				runtimeMetrics.webhookLagTotalMS += lagMS
				runtimeMetrics.webhookLagSamples++
			}
		}
	}
	runtimeMetricsMu.Unlock()

	for _, event := range events {
		redisMetricIncr(ctx, "kumo:metrics:webhook:event_type:"+strings.ToLower(strings.TrimSpace(event.EventType)))
	}
}

func RecordWebhookSecurityFailure(ctx context.Context, reason string) {
	runtimeMetricsMu.Lock()
	runtimeMetrics.webhookSecurityFailures++
	runtimeMetricsMu.Unlock()
	count, _ := redisIncrWithTTL(ctx, "kumo:metrics:webhook:signature_failures", CurrentRuntimeLimits().SignatureFailureWindow)
	g.Log().Warningf(ctx, "KumoMTA webhook verification failed: reason=%s", sanitizeLogText(reason))
	if count >= CurrentRuntimeLimits().SignatureFailureSpike {
		RecordRuntimeAlert(RuntimeAlert{
			Type:     "webhook_signature_failures",
			Severity: alertSeverityCritical,
			Message:  fmt.Sprintf("KumoMTA webhook signature failures reached %d in the current window", count),
		})
	}
}

func RecordWebhookProcessingFailure(ctx context.Context, reason string) {
	runtimeMetricsMu.Lock()
	runtimeMetrics.webhookProcessingFailures++
	runtimeMetricsMu.Unlock()
	redisMetricIncr(ctx, "kumo:metrics:webhook:processing_failures")
	g.Log().Warningf(ctx, "KumoMTA webhook processing failed: reason=%s", sanitizeLogText(reason))
}

func RecordWebhookRedisDedupeHit(ctx context.Context) {
	runtimeMetricsMu.Lock()
	runtimeMetrics.webhookRedisDedupeHits++
	runtimeMetricsMu.Unlock()
	redisMetricIncr(ctx, "kumo:metrics:webhook:redis_dedupe_hits")
}

func RecordOrphanedDeliveryEvent(ctx context.Context, event NormalizedDeliveryEvent) {
	runtimeMetricsMu.Lock()
	runtimeMetrics.webhookOrphaned++
	runtimeMetricsMu.Unlock()
	RecordRuntimeAlert(RuntimeAlert{
		Type:     "orphaned_delivery_event",
		Severity: alertSeverityWarning,
		TenantID: event.TenantID,
		Queue:    event.QueueName,
		Message:  "KumoMTA delivery event could not be matched to a campaign recipient or API log",
	})
	g.Log().Warningf(ctx, "KumoMTA orphaned event stored: event_hash=%s tenant_id=%d queue=%s message_id=%s",
		event.EventHash, event.TenantID, event.QueueName, sanitizeLogText(event.MessageID))
}

func RecordQuotaRejection(ctx context.Context, tenantID, profileID int64, reason string) {
	RecordRuntimeAlert(RuntimeAlert{
		Type:      "tenant_quota_rejected",
		Severity:  alertSeverityWarning,
		TenantID:  tenantID,
		ProfileID: profileID,
		Message:   nonEmptyString(reason, "tenant or profile quota rejected KumoMTA injection"),
	})
	g.Log().Warningf(ctx, "KumoMTA quota guard rejected send: tenant_id=%d profile_id=%d reason=%s", tenantID, profileID, sanitizeLogText(reason))
}

func RecordControlEvent(ctx context.Context, tenantID, profileID int64, status, reason string) {
	RecordRuntimeAlert(RuntimeAlert{
		Type:      "tenant_profile_control",
		Severity:  alertSeverityInfo,
		TenantID:  tenantID,
		ProfileID: profileID,
		Message:   fmt.Sprintf("KumoMTA sending control changed to %s: %s", strings.TrimSpace(status), nonEmptyString(reason, "no reason provided")),
	})
	g.Log().Infof(ctx, "KumoMTA sending control changed: tenant_id=%d profile_id=%d status=%s reason=%s", tenantID, profileID, strings.TrimSpace(status), sanitizeLogText(reason))
}

func RecordRuntimeAlert(alert RuntimeAlert) {
	alert.Type = strings.TrimSpace(alert.Type)
	if alert.Type == "" {
		alert.Type = "runtime"
	}
	if alert.Severity == "" {
		alert.Severity = alertSeverityWarning
	}
	if alert.CreatedAt == 0 {
		alert.CreatedAt = time.Now().Unix()
	}
	alert.ID = runtimeAlertID(alert)

	runtimeMetricsMu.Lock()
	defer runtimeMetricsMu.Unlock()
	for i, existing := range runtimeMetrics.alerts {
		if existing.ID == alert.ID {
			alert.CreatedAt = existing.CreatedAt
			runtimeMetrics.alerts[i] = alert
			return
		}
	}
	runtimeMetrics.alerts = append([]RuntimeAlert{alert}, runtimeMetrics.alerts...)
	if len(runtimeMetrics.alerts) > maxRuntimeAlerts {
		runtimeMetrics.alerts = runtimeMetrics.alerts[:maxRuntimeAlerts]
	}
}

func GetRuntimeSnapshot(ctx context.Context, tenantID int64, operatorView bool) (*RuntimeSnapshot, error) {
	now := time.Now().Unix()
	if operatorView {
		tenantID = 0
	}
	injection, webhook, alerts := runtimeMetricsSnapshot()
	snapshot := &RuntimeSnapshot{
		SnapshotAt:       now,
		Limits:           CurrentRuntimeLimits(),
		Status:           GetStatus(),
		Injection:        injection,
		Webhook:          webhook,
		Alerts:           filterAlerts(alerts, tenantID, operatorView),
		Queues:           []QueueRuntimeMetric{},
		Nodes:            []NodeRuntimeMetric{},
		Pools:            []PoolRuntimeMetric{},
		TenantRisk:       []TenantRiskMetric{},
		ReleaseReadiness: BuildReleaseReadiness(now),
		OperatorView:     operatorView,
		FilteredTenantID: tenantID,
	}

	var warnings []string
	if queues, err := loadQueueRuntimeMetrics(ctx, tenantID); err != nil {
		warnings = append(warnings, "queue metrics: "+sanitizeLogText(err.Error()))
	} else {
		snapshot.Queues = queues
		snapshot.Alerts = append(snapshot.Alerts, queueAgeAlerts(queues, CurrentRuntimeLimits().QueueAgeAlertSeconds, now)...)
	}
	if nodes, err := loadNodeRuntimeMetrics(ctx); err != nil {
		warnings = append(warnings, "node metrics: "+sanitizeLogText(err.Error()))
	} else {
		snapshot.Nodes = nodes
	}
	if pools, err := loadPoolRuntimeMetrics(ctx, tenantID); err != nil {
		warnings = append(warnings, "pool metrics: "+sanitizeLogText(err.Error()))
	} else {
		snapshot.Pools = pools
	}
	if risks, err := loadTenantRiskMetrics(ctx, tenantID, operatorView); err != nil {
		warnings = append(warnings, "tenant risk metrics: "+sanitizeLogText(err.Error()))
	} else {
		snapshot.TenantRisk = risks
		snapshot.Alerts = append(snapshot.Alerts, riskAlerts(risks)...)
	}

	snapshot.Alerts = append(snapshot.Alerts, statusAlerts(snapshot.Status, snapshot.Injection.Attempts, now)...)
	sortAlerts(snapshot.Alerts)
	if len(warnings) > 0 {
		snapshot.CollectionWarning = strings.Join(warnings, "; ")
	}
	return snapshot, nil
}

func CurrentRuntimeLimits() RuntimeLimits {
	return defaultRuntimeLimits
}

func EvaluateInjectionWindow(globalInFlight, tenantInFlight, globalLimit, tenantLimit int64) string {
	if globalLimit > 0 && globalInFlight > globalLimit {
		return fmt.Sprintf("global KumoMTA injection concurrency exceeded: %d of %d", globalInFlight, globalLimit)
	}
	if tenantLimit > 0 && tenantInFlight > tenantLimit {
		return fmt.Sprintf("tenant KumoMTA injection concurrency exceeded: %d of %d", tenantInFlight, tenantLimit)
	}
	return ""
}

func ShouldOpenCircuit(failureCount, threshold int64) bool {
	return threshold > 0 && failureCount >= threshold
}

func QueueAgeAlert(queue QueueRuntimeMetric, thresholdSeconds, now int64) *RuntimeAlert {
	if thresholdSeconds <= 0 || queue.PendingFinalEvents <= 0 || queue.OldestAcceptedAt <= 0 {
		return nil
	}
	age := now - queue.OldestAcceptedAt
	if age < thresholdSeconds {
		return nil
	}
	return &RuntimeAlert{
		Type:      "queue_age_exceeded",
		Severity:  alertSeverityWarning,
		TenantID:  queue.TenantID,
		Queue:     queue.Queue,
		CreatedAt: now,
		Message:   fmt.Sprintf("KumoMTA queue %s has pending final events older than %d seconds", queue.Queue, thresholdSeconds),
	}
}

func TenantRiskLevel(queued, bounced, complained int64) string {
	if queued <= 0 {
		return "none"
	}
	bouncePermille := bounced * 1000 / queued
	complaintPPM := complained * 1000000 / queued
	switch {
	case complaintPPM >= 1000 || bouncePermille >= 100:
		return "high"
	case complaintPPM >= 300 || bouncePermille >= 50:
		return "medium"
	default:
		return "low"
	}
}

func BuildReleaseReadiness(now int64) ReleaseReadiness {
	status := GetStatus()
	cfgReady := false
	if cfg, err := LoadConfig(context.Background()); err == nil && cfg != nil && cfg.Enabled && cfg.BaseURL != "" {
		cfgReady = true
	}
	checks := []string{
		"Postfix/Dovecot/Roundcube compatibility remains available for local mailbox/system flows.",
		"KumoMTA production direct-to-MX from DigitalOcean remains blocked unless an allowed external egress path is verified.",
		"Managed deploy remains dry-run/manual-preview first unless validation and rollback metadata are present.",
	}
	blockers := []string{}
	if !cfgReady {
		blockers = append(blockers, "KumoMTA configuration is disabled or missing a base URL.")
	}
	if !status.Connected {
		blockers = append(blockers, "KumoMTA health check is not connected.")
	}
	if status.WebhookLastSeenAt == 0 {
		blockers = append(blockers, "KumoMTA webhook receiver has not accepted events.")
	}
	return ReleaseReadiness{
		Ready:       len(blockers) == 0,
		Checks:      checks,
		Blockers:    blockers,
		GeneratedAt: now,
		Rollback: []string{
			"Set KumoMTA campaigns_enabled/api_enabled to false to stop new high-volume Kumo injection.",
			"Keep Postfix local/system flows online; do not route high-volume fallback unless explicitly approved.",
			"Pause affected tenant/profile via tenant sending controls when failures are tenant-scoped.",
			"Revert to the last validated Kumo policy preview before any managed deploy is enabled.",
		},
	}
}

func checkCircuit(ctx context.Context, input InjectionControlInput) error {
	val, err := g.Redis().Get(ctx, circuitKey(input))
	if err != nil {
		return err
	}
	if !val.IsNil() && strings.TrimSpace(val.String()) != "" {
		return gerror.New("KumoMTA injection circuit is open for this tenant/profile")
	}
	return nil
}

func recordRuntimeReject(kind string, input InjectionControlInput, reason string) {
	runtimeMetricsMu.Lock()
	switch kind {
	case "rate_limit":
		runtimeMetrics.rateLimited++
	case "circuit":
		runtimeMetrics.circuitRejected++
	default:
		runtimeMetrics.backpressureRejected++
	}
	runtimeMetricsMu.Unlock()
	RecordRuntimeAlert(RuntimeAlert{
		Type:      "kumo_injection_" + kind,
		Severity:  alertSeverityWarning,
		TenantID:  input.TenantID,
		ProfileID: input.SendingProfileID,
		Queue:     input.Queue,
		Message:   reason,
	})
}

func runtimeMetricsSnapshot() (InjectionRuntimeMetrics, WebhookRuntimeMetrics, []RuntimeAlert) {
	runtimeMetricsMu.RLock()
	defer runtimeMetricsMu.RUnlock()

	injection := InjectionRuntimeMetrics{
		Attempts:              runtimeMetrics.attempts,
		Successes:             runtimeMetrics.successes,
		Failures:              runtimeMetrics.failures,
		RateLimited:           runtimeMetrics.rateLimited,
		BackpressureRejected:  runtimeMetrics.backpressureRejected,
		CircuitRejected:       runtimeMetrics.circuitRejected,
		InFlightGlobal:        runtimeMetrics.inFlightGlobal,
		OpenCircuitCount:      runtimeMetrics.openCircuitCount,
		ByTenantProfileEngine: make([]InjectionMetricBucket, 0, len(runtimeMetrics.injectionBuckets)),
	}
	if runtimeMetrics.attempts > 0 {
		injection.AvgLatencyMS = runtimeMetrics.latencyTotalMS / runtimeMetrics.attempts
	}
	for _, bucket := range runtimeMetrics.injectionBuckets {
		item := InjectionMetricBucket{
			TenantID:         bucket.tenantID,
			SendingProfileID: bucket.sendingProfileID,
			Engine:           bucket.engine,
			Attempts:         bucket.attempts,
			Successes:        bucket.successes,
			Failures:         bucket.failures,
		}
		if bucket.attempts > 0 {
			item.AvgLatencyMS = bucket.latencyTotalMS / bucket.attempts
		}
		injection.ByTenantProfileEngine = append(injection.ByTenantProfileEngine, item)
	}
	sort.Slice(injection.ByTenantProfileEngine, func(i, j int) bool {
		return injection.ByTenantProfileEngine[i].Attempts > injection.ByTenantProfileEngine[j].Attempts
	})

	webhook := WebhookRuntimeMetrics{
		Accepted:           runtimeMetrics.webhookAccepted,
		Duplicates:         runtimeMetrics.webhookDuplicates,
		Failed:             runtimeMetrics.webhookFailed,
		SecurityFailures:   runtimeMetrics.webhookSecurityFailures,
		ProcessingFailures: runtimeMetrics.webhookProcessingFailures,
		Orphaned:           runtimeMetrics.webhookOrphaned,
		RedisDedupeHits:    runtimeMetrics.webhookRedisDedupeHits,
		ByType:             make([]WebhookBucket, 0, len(runtimeMetrics.webhookTypes)),
	}
	if runtimeMetrics.webhookLagSamples > 0 {
		webhook.AvgIngestionLagMS = runtimeMetrics.webhookLagTotalMS / runtimeMetrics.webhookLagSamples
	}
	for eventType, count := range runtimeMetrics.webhookTypes {
		webhook.ByType = append(webhook.ByType, WebhookBucket{EventType: eventType, Count: count})
	}
	sort.Slice(webhook.ByType, func(i, j int) bool { return webhook.ByType[i].Count > webhook.ByType[j].Count })

	alerts := append([]RuntimeAlert{}, runtimeMetrics.alerts...)
	return injection, webhook, alerts
}

func getInjectionBucketLocked(input InjectionControlInput) *injectionBucketState {
	key := fmt.Sprintf("%d:%d:%s", input.TenantID, input.SendingProfileID, input.Engine)
	bucket := runtimeMetrics.injectionBuckets[key]
	if bucket == nil {
		bucket = &injectionBucketState{
			tenantID:         input.TenantID,
			sendingProfileID: input.SendingProfileID,
			engine:           input.Engine,
		}
		runtimeMetrics.injectionBuckets[key] = bucket
	}
	return bucket
}

func loadQueueRuntimeMetrics(ctx context.Context, tenantID int64) ([]QueueRuntimeMetric, error) {
	rows := make([]QueueRuntimeMetric, 0)
	model := g.DB().Model("kumo_message_injections").Ctx(ctx).
		Fields(`
			queue_name AS queue,
			tenant_id,
			MAX(campaign_id) AS campaign_id,
			MAX(api_id) AS api_id,
			recipient_domain AS domain,
			SUM(CASE WHEN injection_status = 'queued' THEN 1 ELSE 0 END) AS queued,
			SUM(CASE WHEN delivery_status = 'deferred' THEN 1 ELSE 0 END) AS deferred,
			SUM(CASE WHEN delivery_status = 'bounced' THEN 1 ELSE 0 END) AS bounced,
			SUM(CASE WHEN delivery_status = 'complained' THEN 1 ELSE 0 END) AS complained,
			COALESCE(MIN(CASE WHEN injection_status = 'queued' AND final_event_at = 0 AND accepted_at > 0 THEN accepted_at ELSE NULL END), 0) AS oldest_accepted_at,
			SUM(CASE WHEN final_event_at > 0 THEN 1 ELSE 0 END) AS finalized,
			SUM(CASE WHEN injection_status = 'queued' AND final_event_at = 0 THEN 1 ELSE 0 END) AS pending_final_events
		`).
		Where("created_at >= ?", time.Now().Add(-24*time.Hour).Unix()).
		Group("queue_name, tenant_id, recipient_domain").
		OrderDesc("pending_final_events").
		OrderAsc("oldest_accepted_at").
		Limit(100)
	if tenantID > 0 {
		model = model.Where("tenant_id", tenantID)
	}
	if err := model.Scan(&rows); err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	for i := range rows {
		if rows[i].OldestAcceptedAt > 0 {
			rows[i].OldestAgeSeconds = now - rows[i].OldestAcceptedAt
		}
	}
	return rows, nil
}

func loadNodeRuntimeMetrics(ctx context.Context) ([]NodeRuntimeMetric, error) {
	rows := make([]NodeRuntimeMetric, 0)
	if err := g.DB().Model("kumo_nodes").Ctx(ctx).
		Fields("id, name, status, last_ok_at, last_error_at, last_error").
		Order("id ASC").
		Scan(&rows); err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Healthy = strings.EqualFold(rows[i].Status, "healthy") || strings.EqualFold(rows[i].Status, "active") || rows[i].LastOKAt > rows[i].LastErrorAt
	}
	return rows, nil
}

func loadPoolRuntimeMetrics(ctx context.Context, tenantID int64) ([]PoolRuntimeMetric, error) {
	rows := make([]PoolRuntimeMetric, 0)
	model := g.DB().Model("kumo_egress_pools p").Ctx(ctx).
		LeftJoin("kumo_egress_pool_sources eps", "eps.pool_id = p.id").
		LeftJoin("tenant_sending_profiles tsp", "tsp.kumo_pool_id = p.id").
		Fields("p.id, p.name, p.status, COUNT(DISTINCT eps.source_id) AS source_count, COUNT(DISTINCT tsp.tenant_id) AS tenant_count").
		Group("p.id, p.name, p.status").
		Order("p.name ASC")
	if tenantID > 0 {
		model = model.Where("tsp.tenant_id", tenantID)
	}
	if err := model.Scan(&rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func loadTenantRiskMetrics(ctx context.Context, tenantID int64, operatorView bool) ([]TenantRiskMetric, error) {
	rows := make([]TenantRiskMetric, 0)
	model := g.DB().Model("tenant_usage_daily tud").Ctx(ctx).
		LeftJoin("tenants t", "t.id = tud.tenant_id").
		Fields("tud.tenant_id, COALESCE(t.name, '') AS tenant_name, tud.queued_count AS queued, tud.delivered_count AS delivered, tud.bounced_count AS bounced, tud.complained_count AS complained").
		Where("tud.date", time.Now().Format("2006-01-02")).
		OrderDesc("tud.queued_count").
		Limit(100)
	if tenantID > 0 {
		model = model.Where("tud.tenant_id", tenantID)
	}
	if err := model.Scan(&rows); err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].Queued > 0 {
			rows[i].BouncePermille = rows[i].Bounced * 1000 / rows[i].Queued
			rows[i].ComplaintPPM = rows[i].Complained * 1000000 / rows[i].Queued
		}
		rows[i].Risk = TenantRiskLevel(rows[i].Queued, rows[i].Bounced, rows[i].Complained)
		if !operatorView {
			rows[i].TenantName = ""
		}
	}
	return rows, nil
}

func queueAgeAlerts(queues []QueueRuntimeMetric, threshold, now int64) []RuntimeAlert {
	alerts := make([]RuntimeAlert, 0)
	for _, queue := range queues {
		if alert := QueueAgeAlert(queue, threshold, now); alert != nil {
			alerts = append(alerts, *alert)
		}
	}
	return alerts
}

func riskAlerts(risks []TenantRiskMetric) []RuntimeAlert {
	alerts := make([]RuntimeAlert, 0)
	for _, risk := range risks {
		if risk.Risk != "high" {
			continue
		}
		alerts = append(alerts, RuntimeAlert{
			Type:      "tenant_abuse_threshold",
			Severity:  alertSeverityCritical,
			TenantID:  risk.TenantID,
			Message:   fmt.Sprintf("Tenant %d has high bounce/complaint risk", risk.TenantID),
			CreatedAt: time.Now().Unix(),
		})
	}
	return alerts
}

func statusAlerts(status Status, attempts int64, now int64) []RuntimeAlert {
	alerts := make([]RuntimeAlert, 0)
	if !status.Connected && status.LastError != "" {
		alerts = append(alerts, RuntimeAlert{
			Type:      "kumo_unreachable",
			Severity:  alertSeverityCritical,
			Message:   "KumoMTA health check is failing: " + status.LastError,
			CreatedAt: now,
		})
	}
	if attempts > 0 && (status.WebhookLastSeenAt == 0 || now-status.WebhookLastSeenAt >= CurrentRuntimeLimits().WebhookIdleSeconds) {
		alerts = append(alerts, RuntimeAlert{
			Type:      "webhook_idle",
			Severity:  alertSeverityWarning,
			Message:   "KumoMTA webhook has no recent events while injection traffic exists",
			CreatedAt: now,
		})
	}
	return alerts
}

func filterAlerts(alerts []RuntimeAlert, tenantID int64, operatorView bool) []RuntimeAlert {
	out := make([]RuntimeAlert, 0, len(alerts))
	for _, alert := range alerts {
		if !operatorView && alert.TenantID > 0 && alert.TenantID != tenantID {
			continue
		}
		out = append(out, alert)
	}
	return out
}

func sortAlerts(alerts []RuntimeAlert) {
	sort.SliceStable(alerts, func(i, j int) bool {
		if alerts[i].Severity == alerts[j].Severity {
			return alerts[i].CreatedAt > alerts[j].CreatedAt
		}
		return severityRank(alerts[i].Severity) > severityRank(alerts[j].Severity)
	})
}

func severityRank(severity string) int {
	switch severity {
	case alertSeverityCritical:
		return 3
	case alertSeverityWarning:
		return 2
	default:
		return 1
	}
}

func runtimeAlertID(alert RuntimeAlert) string {
	return fmt.Sprintf("%s:%d:%d:%s", alert.Type, alert.TenantID, alert.ProfileID, alert.Queue)
}

func normalizeInjectionControlInput(input InjectionControlInput) InjectionControlInput {
	input.Engine = strings.TrimSpace(input.Engine)
	if input.Engine == "" {
		input.Engine = "kumomta"
	}
	input.Queue = strings.TrimSpace(input.Queue)
	input.RecipientDomain = strings.ToLower(strings.TrimSpace(input.RecipientDomain))
	return input
}

func failureKey(input InjectionControlInput) string {
	return fmt.Sprintf("kumo:circuit:failures:tenant:%d:profile:%d", input.TenantID, input.SendingProfileID)
}

func circuitKey(input InjectionControlInput) string {
	return fmt.Sprintf("kumo:circuit:open:tenant:%d:profile:%d", input.TenantID, input.SendingProfileID)
}

func redisIncrWithTTL(ctx context.Context, key string, ttlSeconds int64) (int64, error) {
	next, err := g.Redis().Incr(ctx, key)
	if err != nil {
		return 0, err
	}
	if ttlSeconds > 0 {
		_, _ = g.Redis().Expire(ctx, key, ttlSeconds)
	}
	return next, nil
}

func redisMetricIncr(ctx context.Context, key string) {
	if key == "" {
		return
	}
	if _, err := redisIncrWithTTL(ctx, key, runtimeRedisTTLSeconds); err != nil {
		g.Log().Debugf(ctx, "failed to increment KumoMTA runtime metric %s: %v", key, err)
	}
}

func redisSetEX(ctx context.Context, key, value string, ttlSeconds int64) error {
	if ttlSeconds <= 0 {
		ttlSeconds = 60
	}
	return g.Redis().SetEX(ctx, key, value, ttlSeconds)
}

func redisDel(ctx context.Context, key string) error {
	_, err := g.Redis().Del(ctx, key)
	return err
}

func nonEmptyString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func resetRuntimeMetricsForTesting() {
	runtimeMetricsMu.Lock()
	defer runtimeMetricsMu.Unlock()
	runtimeMetrics = runtimeMetricsState{
		injectionBuckets: map[string]*injectionBucketState{},
		webhookTypes:     map[string]int64{},
		alerts:           []RuntimeAlert{},
	}
}
