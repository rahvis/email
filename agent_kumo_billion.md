# Agent Runbook: KumoMTA Integration For BillionMail

Date: 2026-05-14

Source of truth: `PRD.md`

Status: Implementation runbook only. This file does not implement code, schema, Docker, deployment, or configuration changes.

## 1. Purpose

This runbook is for an implementation agent working on the deployed KumoMTA integration for BillionMail. It translates `PRD.md` into an ordered, phase-gated execution plan.

The agent must complete each phase fully before starting the next phase. Completion means:

- implementation is phase-scoped
- code review is performed
- bugs found during review are fixed
- unit, integration, frontend, security, and operational tests relevant to the phase pass
- regressions in existing Postfix, Dovecot, Rspamd, Roundcube, and BillionMail flows are checked
- the phase gate is explicitly satisfied

Do not use this file as permission to skip `PRD.md`. Read `PRD.md` before each phase and treat it as authoritative if any detail differs.

## 2. Non-Negotiable Architecture Decisions

- KumoMTA stays external to the BillionMail repository.
- Do not vendor, copy, or embed KumoMTA source code into BillionMail.
- Do not run production KumoMTA inside the existing BillionMail Docker Compose stack.
- Native integration uses authenticated HTTP injection to KumoMTA `/api/inject/v1`.
- KumoMTA HTTP `2xx` acceptance means `queued` or `accepted`, not `delivered`.
- Final delivery state comes from KumoMTA webhook/log events.
- BillionMail remains the control plane for tenants, users, domains, contacts, templates, campaigns, API keys, suppression, quotas, rendering, tracking, analytics, and policy generation.
- KumoMTA remains the delivery data plane for outbound queueing, egress selection, traffic shaping, retries, DKIM signing, and delivery events.
- Postfix remains for mailbox, local SMTP, system mail, compatibility, low-volume tests, and explicitly configured fallback.
- DigitalOcean-hosted KumoMTA must not be treated as production direct-to-MX egress while outbound SMTP ports are blocked.
- Production delivery must use an allowed egress path: external KumoProxy/SOCKS5, external KumoMTA egress node, provider HTTP API, or provider SMTP submission on a permitted port such as `2525`.
- Multi-tenant SaaS launch is blocked until tenant isolation is enforced end to end.

## 3. Agent Operating Rules

Before each phase:

- Re-read the relevant sections of `PRD.md`.
- Inspect the current repo state before editing.
- Identify existing user changes and do not revert unrelated work.
- Keep changes narrow and aligned with existing GoFrame, Vue, Naive UI, Pinia, Axios, PostgreSQL, and Redis patterns.
- Produce an implementation checklist for the phase.

During each phase:

- Preserve existing Postfix, Dovecot, Rspamd, Roundcube, mailbox, webmail, and local/system mail behavior.
- Avoid unrelated refactors.
- Do not log secrets, auth tokens, DKIM private keys, webhook secrets, full RFC 5322 message bodies, or recipient lists by default.
- Sanitize request and response logs.
- Use server-validated tenant context; never trust a UI-sent or caller-sent tenant ID without membership/API-key validation.
- Keep metrics polling and UI reads decoupled from message injection.
- Fail closed for Kumo sending if required config, tenant profile, domain readiness, or auth state cannot be loaded.
- Do not expose Kumo injection, admin, or metrics endpoints publicly without TLS, strong authentication, and source allowlisting.
- Do not document reverse proxy, DNS, VPC peering, or port forwarding as a workaround for DigitalOcean SMTP blocking.

Before marking a phase complete:

- Review the code and configuration changes made in the phase.
- Fix bugs found during review.
- Run relevant tests.
- Confirm old behavior still works where the phase should not change it.
- Record remaining risks and follow-up work for later phases.

## 4. Global Implementation Constraints

### 4.1 Security

- Store Kumo auth and webhook secrets according to the existing secret-management convention.
- Never return raw secrets from APIs.
- Add `/api/kumo/events` to any JWT skip list only after HMAC/token validation middleware is active.
- Enforce request body size limits on webhook ingestion.
- Use timestamp tolerance for HMAC signatures if the signature scheme includes timestamps.
- Reject missing or invalid webhook signatures.
- Store raw webhook event payloads safely for audit/debug without leaking secrets in normal logs.

### 4.2 State And Delivery Semantics

Use separate state concepts:

```text
------------------+--------------------------------------------------+
| State group      | Meaning                                          |
+------------------+--------------------------------------------------+
| injection_status | BillionMail rendering/injection/Kumo acceptance  |
| delivery_status  | recipient delivery result from Kumo events       |
+------------------+--------------------------------------------------+
```

Required injection states:

```text
pending
rendering
injecting
queued
retrying
failed
cancelled
```

Required delivery states:

```text
pending
delivered
deferred
bounced
expired
complained
suppressed
unknown
```

Do not overload existing `is_sent` or legacy status fields to mean final delivery.

### 4.3 Kumo Queue And Header Standards

Queue names:

```text
campaign_<campaign_or_task_id>:tenant_<tenant_id>@<recipient-domain>
api_<api_template_id>:tenant_<tenant_id>@<recipient-domain>
```

Campaign headers:

```text
X-BM-Tenant-ID: <tenant id>
X-BM-Campaign-ID: <campaign id>
X-BM-Task-ID: <task id>
X-BM-Recipient-ID: <recipient_info id>
X-BM-Api-Log-ID: 0
X-BM-Message-ID: <message id>
X-BM-Sending-Profile-ID: <profile id>
X-BM-Engine: kumomta
```

Send API headers:

```text
X-BM-Tenant-ID: <tenant id>
X-BM-Api-ID: <api template id>
X-BM-Api-Log-ID: <api_mail_logs id>
X-BM-Message-ID: <message id>
X-BM-Sending-Profile-ID: <profile id>
X-BM-Engine: kumomta
```

### 4.4 Gitdate Infrastructure Facts

Current DNS facts from `PRD.md`:

```text
mail.gitdate.ink  -> 159.89.33.85
email.gitdate.ink -> 192.241.130.241
MX gitdate.ink    -> gitdate-ink.mail.protection.outlook.com, priority 0
root SPF          -> v=spf1 include:spf.protection.outlook.com -all
email SPF         -> v=spf1 ip4:192.241.130.241 ~all
email DKIM        -> s1._domainkey.email.gitdate.ink TXT exists
email DMARC       -> _dmarc.email.gitdate.ink TXT exists
```

Use `https://mail.gitdate.ink/api/kumo/events` for Kumo webhooks after reverse proxy health passes and webhook authentication is enabled.

Preferred Kumo injection path:

```text
BillionMail private IP 10.108.0.3
    -> VPC/private routing
KumoMTA private IP 10.116.0.2
```

Acceptable Kumo injection path:

```text
BillionMail public IP 159.89.33.85
    -> HTTPS + TLS verification + strong auth + source allowlisting
https://email.gitdate.ink
```

Production delivery warning:

```text
DigitalOcean blocks outbound SMTP ports 25, 465, and 587 on Droplets by default.
Do not mark direct-to-MX delivery from the DigitalOcean Kumo droplet as production-ready
until an allowed external egress path is configured and verified.
```

## 5. Phase 0: Baseline Review And Operational Verification

Goal: establish current repo, DNS, reverse proxy, Kumo, egress, and risk facts before code changes.

### Tasks

- Inspect current backend send paths:
  - campaign worker
  - Send API worker
  - SMTP sender service
  - mail log/stat aggregation
- Inspect current frontend shape:
  - settings route modules
  - API modules
  - Send API page
  - campaign pages
  - Send Queue and SMTP pages
- Inspect database initialization for:
  - mail service tables
  - batch mail tables
  - options/config storage
  - RBAC/account tables
- Inspect auth/RBAC middleware and identify where operator/admin checks should be attached.
- Inspect Redis usage for locks, queues, counters, and rate limits.
- Verify DNS:
  - `dig +short A mail.gitdate.ink`
  - `dig +short A email.gitdate.ink`
  - `dig +short MX gitdate.ink`
  - `dig +short TXT email.gitdate.ink`
  - `dig +short TXT s1._domainkey.email.gitdate.ink`
  - `dig +short TXT _dmarc.email.gitdate.ink`
- Verify reverse proxy health for `https://mail.gitdate.ink/api/languages/get`.
- Verify KumoMTA injection reachability path:
  - private VPC path if configured
  - public HTTPS path only if TLS, auth, and source allowlisting are configured
- Verify webhook reachability from KumoMTA to `https://mail.gitdate.ink/api/kumo/events`.
- Verify production egress strategy:
  - external KumoProxy/SOCKS5
  - external KumoMTA node
  - provider HTTP API
  - provider SMTP on allowed port such as `2525`
- Mark DigitalOcean direct-to-MX as blocked/not production-ready unless explicitly proven otherwise.

### Review Checklist

- Repo findings are grounded in actual files, not assumptions.
- Current Postfix and Send API behavior is documented.
- DNS facts match `PRD.md`.
- Kumo injection path and webhook path are separately documented.
- Production egress plan is explicit.

### Tests

- DNS commands return expected records.
- Reverse proxy health endpoint returns a valid BillionMail response.
- Kumo endpoint health/test call is performed only with safe auth and approved network path.
- No code or schema changes are made in this phase.

### Phase Gate

Do not start Phase 1 until the agent has produced:

- baseline findings
- risks
- exact implementation deltas
- verified or blocked operational dependencies

## 6. Phase 1: KumoMTA Configuration, Health, Metrics, And Webhook Shell

Goal: add operator-managed KumoMTA configuration, status, metrics cache, and authenticated webhook shell.

### Tasks

- Add durable config storage for:
  - enabled
  - campaigns enabled
  - Send API enabled
  - base URL
  - injection path
  - metrics URL
  - TLS verification mode
  - auth mode
  - auth secret presence
  - webhook secret presence
  - timeout
  - default pool
- Store secrets protected by the existing convention.
- Add config cache:
  - TTL between 30 and 120 seconds
  - immediate invalidation on config update
  - fail closed for Kumo sending if cache/config load fails
- Implement operator/admin APIs:
  - `GET /api/kumo/config`
  - `POST /api/kumo/config`
  - `POST /api/kumo/test_connection`
  - `GET /api/kumo/status`
  - `GET /api/kumo/metrics`
- Implement metrics polling:
  - background poller stores cached snapshots
  - failures store last error and last success time
  - sending does not depend on metrics availability
- Add webhook shell:
  - `POST /api/kumo/events`
  - HMAC/token validation
  - body size limits
  - duplicate/idempotency foundation
  - sanitized raw event storage
  - no mutation of campaign/API delivery state until Phase 3
- Add frontend API module:
  - `core/frontend/src/api/modules/kumo.ts`
- Add operator UI under `Settings > KumoMTA`.
- Do not overload the existing SMTP page.

### Review Checklist

- Raw secrets are never returned from APIs.
- Raw secrets and full message bodies are not logged.
- Webhook route is not publicly mutable without auth.
- Metrics failure does not block config save, health display, or sending path.
- UI follows existing Vue + Naive UI patterns.

### Tests

- Config API returns `has_auth_secret` and `has_webhook_secret`, never secret values.
- Config update validates URL shape and timeout bounds.
- Cache invalidates after config update.
- Test connection handles success, TLS failure, auth failure, timeout, and metrics failure.
- Webhook rejects missing/invalid signatures.
- Webhook accepts valid signed empty/test payload.
- Kumo settings UI renders disconnected, connected, and error states.

### Phase Gate

Proceed only after:

- unit tests pass
- API tests pass
- frontend state tests pass
- manual UI review confirms disconnected/connected/error behavior
- code review bugs are fixed

## 7. Phase 2: Backend Outbound Mailer Abstraction

Goal: decouple campaign/API workers from direct local SMTP submission and add a Kumo HTTP mailer implementation.

### Tasks

- Add `OutboundMailer` interface.
- Add `OutboundMessage` and `OutboundResult` types.
- Wrap current SMTP behavior as `PostfixSMTPMailer`.
- Add `KumoHTTPMailer`.
- Use a long-lived Go HTTP client with:
  - timeout
  - keep-alives
  - TLS verification behavior from config
  - sanitized request/response logging
- Implement HTTP status handling:
  - `2xx`: accepted/queued
  - `400/422`: permanent message/config validation failure
  - `401/403`: Kumo auth/config failure; pause or block Kumo sending
  - `429`: retryable backpressure
  - `5xx`: retryable service failure
  - network timeout: retryable service failure
- Generate queue names using the standard format.
- Add required `X-BM-*` headers.
- Add engine selection by workflow and future tenant/profile setting.
- Keep Postfix/local behavior unchanged when selected or when Kumo is disabled.
- Send one rendered recipient per HTTP injection in v1.
- Do not use Kumo deferred generation in v1.

### Review Checklist

- Existing mail rendering path is preserved.
- Existing Postfix path is wrapped, not rewritten unnecessarily.
- Kumo accepted result never maps to delivered.
- Retry/permanent error classification is explicit.
- Logs are useful but sanitized.

### Tests

- Postfix/local send behavior still passes existing tests.
- Mock Kumo verifies payload body shape.
- Mock Kumo verifies queue name.
- Mock Kumo verifies required headers.
- HTTP status mapping tests cover `2xx`, `400`, `422`, `401`, `403`, `429`, `5xx`, and timeout.
- Message-ID is preserved.

### Phase Gate

Proceed only after:

- Postfix regression tests pass
- Kumo mock tests pass
- retry classification tests pass
- code review bugs are fixed

## 8. Phase 3: Message Lifecycle, Injection State, And Event Ingestion

Goal: introduce explicit injection and delivery state, durable Kumo injection records, and event-driven final status updates.

### Tasks

- Add state fields to campaign recipient records:
  - engine
  - injection_status
  - delivery_status
  - kumo_queue
  - provider queue/injection ID
  - last delivery event time
  - last delivery response
  - attempt count
  - next retry time
- Add equivalent fields to API mail logs.
- Add `kumo_message_injections`.
- Add `kumo_delivery_events`.
- Implement event verification, normalization, idempotent storage, and application.
- Support single-event and batched webhook payloads.
- Extract correlation from:
  - `X-BM-*` headers
  - Message-ID
  - queue name
  - stored injection rows
- Deduplicate by provider event ID or stable event hash.
- Store orphaned events for diagnostics.
- Apply safe state transitions:
  - delivered
  - deferred
  - bounced
  - expired
  - complained
  - suppressed
  - unknown
- Update analytics, suppression, and usage counters from events.
- Do not update final delivery status from injection success alone.

### Review Checklist

- Idempotency exists in Redis fast path and DB durable path where practical.
- Duplicate events cannot double count analytics.
- Out-of-order events keep event history and do not corrupt final state.
- Unknown messages are stored and visible for diagnostics.
- Partial batch failure accepts valid events and reports failures.

### Tests

- HMAC/token verification.
- Single event normalization.
- Batch event normalization.
- Duplicate event handling.
- Stable hash generation when provider event ID is missing.
- Delivered event updates delivery status.
- Bounce event updates analytics and suppression.
- Deferred event does not become delivered.
- Orphan event is stored safely.
- Partial batch reports accepted, duplicate, and failed counts.

### Phase Gate

Proceed only after:

- duplicate events do not double count
- out-of-order events are handled
- unknown messages are stored safely
- review bugs are fixed
- tests pass

## 9. Phase 4: Campaign Sending Through KumoMTA

Goal: move Kumo-enabled campaigns from local Postfix submission to Kumo HTTP injection while preserving local/Postfix mode.

### Tasks

- Add campaign delivery engine selection:
  - tenant default
  - KumoMTA
  - local Postfix
- Add campaign sending profile selection or tenant default inheritance.
- Check before scheduling/sending:
  - tenant quota
  - profile quota
  - suppression
  - sender domain readiness
  - Kumo health/config
  - webhook health
- Inject one rendered recipient message per HTTP request.
- Set injection status:
  - pending
  - rendering
  - injecting
  - queued
  - retrying
  - failed
  - cancelled
- Keep delivery status pending until Kumo events arrive.
- Keep existing open/click tracking.
- Update campaign analytics to show:
  - queued
  - delivered
  - deferred
  - bounced
  - expired
  - complained
- Preserve Postfix behavior when selected or when Kumo campaigns are disabled.

### Review Checklist

- Campaign worker does not treat injection as delivery.
- Retryable errors remain retryable with backoff.
- Permanent failures are recorded clearly.
- Suppressed recipients are not injected.
- Campaign UI remains dense and operational, matching existing Naive UI style.

### Tests

- Kumo-enabled campaign injects into mocked KumoMTA.
- Kumo `2xx` sets injection status queued and delivery status pending.
- Kumo timeout sets retrying and does not mark delivered.
- Kumo `400/422` sets failed with sanitized error.
- Delivered webhook updates campaign recipient status.
- Bounce webhook updates analytics and suppression.
- Postfix/local campaign path still works.

### Phase Gate

Proceed only after:

- mocked Kumo campaign run queues messages
- Kumo events update final delivery state
- retryable errors do not mark delivered
- Postfix campaign regression passes
- review bugs are fixed

## 10. Phase 5: Send API Through KumoMTA

Goal: move Kumo-enabled Send API messages to Kumo HTTP injection with tenant-bound API keys and queued/final delivery state separation.

### Tasks

- Resolve tenant from API key server-side.
- Do not trust caller-sent tenant headers.
- Scope API templates, logs, IP whitelist, sender profile, and quota to the resolved tenant.
- Add Send API engine selection:
  - inherited
  - KumoMTA
  - Postfix/local
- Queue API mail with explicit injection/delivery states.
- Return accepted/queued state plus BillionMail message ID or API log ID.
- Do not return delivered unless a final delivery event has already occurred.
- Update API logs from Kumo webhooks.
- Show Kumo state and webhook diagnostics on Send API UI.
- Preserve existing Send API behavior when Postfix/local is selected.

### Review Checklist

- API key belongs to exactly one tenant.
- API key hash storage is preserved or introduced before plaintext storage.
- Tenant quotas apply server-side.
- Forged tenant headers are ignored or rejected.
- API responses are clear about queued vs delivered.

### Tests

- Send API returns queued for Kumo `2xx`.
- Webhook later updates delivered/bounced/deferred status.
- Forged tenant header cannot switch tenant.
- API key for Tenant A cannot send from Tenant B domain/template/profile.
- IP whitelist remains tenant/API-key scoped.
- Postfix/local Send API path still works when selected.

### Phase Gate

Proceed only after:

- Send API queued response works
- webhook updates final status
- API-key tenant isolation is tested
- Postfix Send API regression passes
- review bugs are fixed

## 11. Phase 6: Multi-Tenant Foundation And Hardening

Goal: enforce B2B workspace tenancy before SaaS launch.

### Tasks

- Add core tenant tables:
  - tenants
  - tenant_members
  - tenant_invitations
  - tenant_plans
  - tenant_usage_daily
  - tenant_suppression_list
  - tenant_api_keys
- Backfill existing single-tenant installs into a default tenant.
- Make existing accounts members of the default tenant using current role mapping.
- Add `tenant_id` to all customer-owned tables listed in `PRD.md`.
- Update business queries to require tenant scope.
- Update writes to attach tenant ID server-side.
- Add membership validation for UI/API tenant context.
- Add API key to tenant resolution for public Send API.
- Add tenant roles:
  - owner
  - admin
  - marketer
  - developer
  - operator
- Add tenant APIs:
  - `GET /api/tenants/current`
  - `GET /api/tenants`
  - `POST /api/tenants/switch`
  - `POST /api/tenants`
- Add tenant switcher only after backend enforcement exists.
- Clear tenant-scoped frontend caches after switching.
- Ensure URLs or stale tabs cannot reveal previous tenant data.

### Review Checklist

- Every business query touched in this phase includes tenant scope.
- Direct natural-key uniqueness is reviewed for tenant-aware constraints.
- Domain ownership remains globally unique by default unless operator policy explicitly allows sharing.
- Tenant users cannot see platform-wide Kumo pool membership or other tenant names.
- Operators can see cross-tenant operational metadata without exposing unnecessary recipient-level data.

### Tests

- Default tenant backfill.
- Tenant membership checks.
- Tenant role permission checks.
- Tenant A cannot read Tenant B contacts, domains, campaigns, API logs, analytics, templates, suppression, or message bodies.
- Tenant A cannot send from Tenant B domain.
- Multi-tenant account can switch only to active memberships.
- Stale tenant context is rejected after switch.
- Public Send API key resolves correct tenant even with forged headers.

### Phase Gate

Proceed only after:

- cross-tenant access denial tests pass
- tenant switch cache behavior works
- default tenant migration is tested
- review bugs are fixed

## 12. Phase 7: Sending Profiles, Domain Readiness, Quotas, And Abuse Controls

Goal: add tenant sending profiles and enforce domain, quota, egress, and abuse readiness.

### Tasks

- Add tenant sending profile tables:
  - tenant_sending_profiles
  - tenant_sending_profile_domains
- Add Kumo egress metadata tables:
  - kumo_nodes
  - kumo_egress_sources
  - kumo_egress_pools
  - kumo_egress_pool_sources
- Implement sending profile fields:
  - tenant ID
  - name
  - sender domains
  - default From domain
  - Kumo pool
  - egress mode
  - egress provider
  - DKIM selector
  - daily quota
  - hourly quota
  - warmup enabled
  - status
- Add `POST /api/tenants/:id/sending-profile`.
- Enforce that sender domains belong to the tenant.
- Enforce that requested Kumo pool is allowed for the tenant by operator policy.
- Add domain readiness checks:
  - SPF
  - DKIM
  - DMARC
  - PTR/rDNS
  - EHLO
  - mailbox/MX readiness separately from Kumo outbound readiness
  - webhook reachability
  - Kumo injection reachability
  - DigitalOcean SMTP block status
  - real outbound egress source readiness
- Enforce quota using Redis counters and DB usage records.
- Add tenant/profile pause, resume, throttle, suspend, and operator kill switch.
- Add bounce/complaint thresholds.
- Add tenant-visible quota and abuse-block banners.

### Review Checklist

- Tenant users can select only their own domains.
- Tenants cannot see other tenants assigned to shared pools.
- Quota enforcement is per tenant/profile and does not globally block unrelated tenants.
- DNS readiness validates real egress source, not just `192.241.130.241`.
- Microsoft 365 MX/root SPF remains intact unless inbound mail is intentionally migrated.

### Tests

- Sending profile cannot activate without a Kumo-ready domain.
- Tenant cannot attach another tenant's domain.
- Tenant cannot choose an unallowed Kumo pool.
- Daily quota exhaustion blocks only affected tenant.
- Abuse threshold pauses only affected tenant.
- Other tenants on shared pool continue sending.
- Gitdate DNS checks validate `mail`, `email`, SPF, DKIM selector `s1`, and DMARC records.
- Direct-to-MX from DigitalOcean remains not production-ready until allowed egress is verified.

### Phase Gate

Proceed only after:

- quota exhaustion blocks only the affected tenant
- domain readiness validates real egress path
- abuse controls work per tenant
- review bugs are fixed

## 13. Phase 8: KumoMTA Policy Preview And Manual Deployment Workflow

Goal: generate deterministic KumoMTA policy artifacts from BillionMail state and support manual deployment first.

### Tasks

- Generate versioned KumoMTA policy previews for:
  - tenant-to-pool mapping
  - egress source definitions
  - egress pool definitions
  - DKIM domain/selector/key mapping
  - traffic class mapping
  - webhook/log hook configuration
  - correlation headers/log parameters
- Add config version records:
  - version
  - generated by account ID
  - status
  - preview
  - deployed at
  - error
  - created at
- Implement `POST /api/kumo/config/preview`.
- Add preview UI for operators.
- Keep managed deployment disabled or dry-run only until manual workflow is stable.
- Implement `POST /api/kumo/config/deploy` only after:
  - validation exists
  - rollback metadata exists
  - audit logging exists
  - service reload behavior is defined
- Do not deploy if validation fails.

### Review Checklist

- Preview output is deterministic for the same DB state.
- Preview never exposes secrets in normal UI/logs.
- DKIM private key handling follows secret rules.
- Deployment path supports dry run.
- Manual deployment remains the recommended v1 workflow.

### Tests

- Preview generation with no tenants.
- Preview generation with shared pool tenant.
- Preview generation with dedicated pool tenant.
- Warning when tenant lacks verified DKIM selector.
- Validation failure blocks deploy.
- Dry run does not mutate remote KumoMTA.
- Audit log records preview/deploy attempts.

### Phase Gate

Proceed only after:

- preview output is deterministic
- validation catches bad config
- secrets are protected
- dry run is safe
- review bugs are fixed

## 14. Phase 9: Scale, Performance, Observability, And Release Readiness

Goal: harden the system for high-volume multi-tenant sending and production operations.

### Tasks

- Add Kumo injection concurrency limits.
- Add per-tenant backpressure counters.
- Add per-profile injection rate limits.
- Add circuit breakers for repeated Kumo failures.
- Add queue age alerts.
- Add structured logs for:
  - Kumo config changes
  - health failures
  - injection attempts
  - sanitized injection failures
  - webhook verification failures
  - webhook processing failures
  - orphaned delivery events
  - quota rejections
  - tenant/profile pause/resume
- Add metrics for:
  - injection attempts by tenant/profile/engine
  - injection success/failure/latency
  - webhook events by type
  - event ingestion lag
  - queued messages by queue/domain/tenant
  - deferred/bounced/complained counts
  - Kumo node health
  - Redis idempotency hit rate
- Add alerts for:
  - Kumo unreachable
  - no webhook events during active traffic
  - signature failure spike
  - bounce/complaint threshold exceeded
  - queue age threshold exceeded
  - tenant quota exceeded
  - config deploy failed
- Add dashboards:
  - Kumo nodes
  - queues
  - pools
  - tenant risk
  - webhook health
- Validate latency targets:
  - config cache read under 10ms
  - Kumo injection timeout default 5s
  - webhook verification/store under 250ms for normal payloads
  - metrics UI load under 500ms from cached snapshot
- Execute release readiness review.
- Prepare rollback plan.

### Review Checklist

- Backpressure protects KumoMTA and BillionMail workers.
- Metrics failures do not block sending.
- Alerts are actionable and not overly noisy.
- Dashboards do not leak tenant data to other tenants.
- Rollback plan restores safe Postfix/local or paused state without false delivery claims.

### Tests

- Load test injection worker concurrency.
- Load test webhook ingestion.
- Metrics endpoint failure does not block injection.
- Kumo outage triggers retry/backoff/circuit breaker.
- Tenant quota and profile rate limits enforce correctly under concurrency.
- Signature failure spike increments alert metric.
- Queue age alert triggers.
- Dashboard data respects operator vs tenant visibility.
- Rollback procedure is documented and tested in staging.

### Phase Gate

Release only after:

- load tests pass
- failure tests pass
- security review passes
- tenant isolation tests pass
- rollback plan is validated
- direct-to-MX from DigitalOcean is still blocked unless an allowed egress path is verified

## 15. Cross-Phase Test Matrix

Run the relevant subset after each phase and the full matrix before release.

### Unit Tests

- Kumo injection payload generation.
- Kumo response parsing.
- HTTP status to retry/permanent mapping.
- Message-ID preservation.
- Required `X-BM-*` header generation.
- Queue name generation.
- HMAC/token webhook verification.
- Event normalization.
- Event idempotency.
- State transition rules.
- Tenant membership enforcement.
- API key to tenant resolution.
- Active tenant context validation.
- Tenant-scoped unique constraints.
- Default tenant backfill.
- Tenant role permission checks.
- Sending profile selection.

### Integration Tests

- Campaign send path injects into mocked KumoMTA.
- Send API path returns queued and later updates from webhook.
- Kumo unavailable keeps messages pending/retryable.
- Kumo auth failure pauses or blocks Kumo sending.
- Metrics endpoint failure does not block sending.
- Duplicate webhook event does not double count.
- Orphan webhook event is stored for diagnostics.
- Postfix/local sending still works when selected.
- Tenant A cannot access Tenant B data.
- Tenant A cannot send from Tenant B sender domain.
- Public Send API key resolves correct tenant.
- Quota exhaustion blocks only affected tenant.
- Operator suspension blocks only affected tenant.
- External egress mode delivers and returns events.

### Frontend Tests

- `kumo.ts` calls correct endpoints.
- Kumo settings screen renders disconnected, connected, and error states.
- Test connection action handles success and failure.
- Campaign create/edit shows delivery engine and sending profile.
- Campaign analytics show Kumo event statuses.
- Send API detail shows Kumo engine and webhook health.
- Dashboard tables render queues and pools.
- Tenant switcher renders only available workspaces.
- Tenant switch clears tenant-scoped state.
- Tenant onboarding renders blocked, partial, and ready states.
- Tenant team screen applies roles.
- Quota and abuse banners render correctly.
- Operator tenant-risk view does not expose recipient lists to tenant users.

### Security Tests

- Secrets are not returned from APIs.
- Secrets are not logged.
- Webhook rejects invalid signatures.
- Forged tenant headers are ignored or rejected.
- Tenant access checks are enforced on reads and writes.
- Public Kumo endpoints require TLS/auth/allowlisting.
- Request body limits are enforced.
- Raw event storage is safe for diagnostics.

### Operational Tests

- `mail.gitdate.ink` resolves to `159.89.33.85`.
- `email.gitdate.ink` resolves to `192.241.130.241`.
- Microsoft 365 MX remains intact.
- `email.gitdate.ink` SPF exists.
- `s1._domainkey.email.gitdate.ink` DKIM exists.
- `_dmarc.email.gitdate.ink` DMARC exists.
- Reverse proxy health passes at `https://mail.gitdate.ink/api/languages/get`.
- Kumo webhook receiver is reachable with valid auth.
- Kumo injection endpoint is reachable through selected private or secured public path.
- DigitalOcean direct-to-MX remains blocked/not production-ready until a valid egress path is configured.
- Real outbound egress SPF/DKIM/DMARC/PTR/EHLO alignment is verified.

## 16. Public API Checklist

Implement these APIs in the phases where they belong:

```text
GET  /api/kumo/config
POST /api/kumo/config
POST /api/kumo/test_connection
GET  /api/kumo/status
GET  /api/kumo/metrics
GET  /api/kumo/pools
POST /api/kumo/pools
POST /api/kumo/config/preview
POST /api/kumo/config/deploy
POST /api/kumo/events

GET  /api/tenants/current
GET  /api/tenants
POST /api/tenants/switch
POST /api/tenants
POST /api/tenants/:id/sending-profile
```

General API rules:

- Operator-only APIs must enforce operator/admin authorization.
- Tenant APIs must validate membership and role.
- Public Send API must resolve tenant from API key.
- Webhook API bypasses JWT only after HMAC/token validation exists.
- Never return raw secrets.
- Use clear queued vs delivered response language.

## 17. Data Model Checklist

Create or update tables according to `PRD.md`.

Tenant foundation:

```text
tenants
tenant_members
tenant_invitations
tenant_plans
tenant_usage_daily
tenant_suppression_list
tenant_api_keys
```

Kumo metadata:

```text
kumo_nodes
kumo_egress_sources
kumo_egress_pools
kumo_egress_pool_sources
tenant_sending_profiles
tenant_sending_profile_domains
kumo_message_injections
kumo_delivery_events
kumo_config_versions
```

Tenant-scope existing customer-owned tables at minimum:

```text
domain
mailbox
alias
alias_domain
bm_bcc
bm_relay_config
bm_relay_domain_mapping
bm_domain_smtp_transport
bm_contact_groups
bm_contacts
bm_tags
bm_contact_tags
email_templates
email_tasks
recipient_info
unsubscribe_records
abnormal_recipient
api_templates
api_mail_logs
api_ip_whitelist
mailstat_* or replacement event tables
bm_operation_logs
```

Migration rules:

- Create a default tenant for existing installs.
- Backfill existing rows to the default tenant.
- Add existing accounts as default tenant members.
- Review unique constraints for tenant awareness.
- Keep domain ownership globally unique by default unless operator policy explicitly allows sharing.

## 18. Completion Definition

The KumoMTA integration is not release-ready until all of the following are true:

- Existing Postfix/local sending still works.
- Operator can configure KumoMTA and see healthy status.
- Kumo-enabled campaign queues mail through KumoMTA.
- Kumo delivered/bounce/defer events update campaign and API records.
- Send API returns queued state and later shows final event state.
- Tenant isolation is enforced across contacts, domains, campaigns, templates, API logs, analytics, suppression, quotas, and sending profiles.
- Tenant quota or abuse block affects only that tenant.
- Queue and event metrics are operationally useful.
- Kumo metrics failure does not block injection.
- Gitdate DNS and reverse proxy checks pass.
- Kumo webhook receiver is authenticated and reachable.
- Kumo injection path is private or secured public HTTPS with allowlisting.
- Direct-to-MX from the DigitalOcean Kumo droplet is blocked/not production-ready unless allowed egress is verified.
- External egress can deliver mail and return events.
- Real egress SPF/DKIM/DMARC/PTR/EHLO alignment is verified.
- Security review and rollback plan are complete.

