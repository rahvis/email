# KumoMTA Phase 9 Release Readiness

This document is the operator release gate for the BillionMail KumoMTA integration scale phase. It does not replace `PRD.md` or `agent_kumo_billion.md`; it records the checks that must pass before enabling KumoMTA for production campaign or Send API traffic.

## Required Checks

- KumoMTA remains external; no KumoMTA source is vendored into BillionMail.
- Kumo HTTP `2xx` acceptance is treated as `queued`, never final delivery.
- Final delivery, bounce, defer, expiration, and complaint state comes from KumoMTA webhook/log events.
- Postfix, Dovecot, and Roundcube local/system/mailbox flows remain available.
- Kumo injection endpoints are reachable only through private routing or public HTTPS with TLS, authentication, and source allowlisting.
- Kumo metrics failures do not block injection.
- Webhook signature failures are rejected and counted.
- Tenant/profile quota exhaustion blocks only that tenant/profile.
- DigitalOcean-hosted KumoMTA is not marked production direct-to-MX ready while SMTP egress ports are blocked.
- Production egress uses KumoProxy, an external Kumo node, provider HTTP API, or provider SMTP submission on a permitted port such as `2525`.

## Latency Targets

- Config cache read: under 10ms in-process.
- Kumo HTTP injection timeout: 5s default unless operator changes it.
- Webhook verification and enqueue/store path: under 250ms for normal payloads.
- Metrics dashboard load: under 500ms from BillionMail cache/runtime snapshot.

## Failure Tests

- Kumo outage opens retry/backoff behavior and does not mark mail delivered.
- Repeated tenant/profile injection failures open the circuit breaker.
- Per-profile rate limits reject excess injection as retryable backpressure.
- Queue age alerts fire when queued messages have no final event beyond the configured threshold.
- Webhook duplicate events do not double count analytics.
- Signature failure spikes create an operator alert.
- Tenant risk alerts fire for high bounce or complaint ratios.

## Rollback Plan

1. Disable new Kumo campaign/API injection by setting KumoMTA campaign/API flags off in `Settings > KumoMTA`.
2. Pause only the affected tenant or sending profile when the incident is tenant-scoped.
3. Keep Postfix local/system/mailbox mail online; do not move high-volume traffic to Postfix unless explicitly approved.
4. Revert Kumo policy manually to the last validated preview. Managed deploy remains disabled until validation, rollback, and audit logs are proven in staging.
5. Preserve Kumo webhook ingestion during rollback so already queued messages can continue to update final delivery state.

## Release Sign-Off

Before launch, attach the test run output for:

- Go unit/integration tests.
- Frontend build and test suite.
- Mocked Kumo injection, metrics, and webhook tests.
- Load test summary for injection concurrency and webhook ingestion.
- Security review summary covering secrets, webhook auth, public endpoint exposure, and tenant isolation.
