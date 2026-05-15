# KumoMTA Implementation Review

Date: 2026-05-15

Scope: review of the KumoMTA/BillionMail phase 0-9 implementation against `agent_kumo_billion.md` and `PRD.md`.

## Findings And Fixes

| Severity | Finding | Fix | Verification |
| --- | --- | --- | --- |
| High | KumoMTA config, test, status, metrics, preview, and deploy handlers relied on route/RBAC behavior instead of explicitly enforcing operator-only access. | Added operator checks in the Kumo controller for all operator-only endpoints while keeping `/api/kumo/events` webhook-auth only and `/api/kumo/runtime` tenant-filtered for tenant users. | `go test ./...` |
| High | Kumo health/test/metrics requests supported bearer auth only, so HMAC-configured Kumo endpoints could fail health checks even though injection worked. | Added HMAC request signing to the Kumo health/test/metrics HTTP helper using the same timestamp/body signature convention as injection and webhooks. | `go test ./internal/service/kumo`; HMAC regression test |
| High | Send API queue locking used `SETEX` without NX ownership, so multiple workers could overwrite each other's lock and process the same queue concurrently. | Replaced it with an owner-token Redis `SET ... EX ... NX` lock plus Lua-based safe renew and release. | `go test ./internal/service/batch_mail`; lock hook regression tests |
| High | Frontend Axios request interceptors run in reverse registration order, so auth/tenant header checks saw already-prefixed `/api/...` URLs and could add auth headers to public routes or tenant headers to tenant-control routes. | Normalized request paths before whitelist checks and added regression coverage for public, tenant-control, and tenant-scoped API requests. | `pnpm test src/api/index.test.ts`; `pnpm test` |
| High | Public Send API contact-group enrollment did not verify that a configured group belonged to the tenant resolved from the API key. | Added tenant ownership validation before adding a recipient to an API template group for both single and batch Send API requests. | `go test ./internal/controller/batch_mail` |
| High | Kumo webhook event correlation and mutation paths could accept conflicting tenant metadata and then update rows by global IDs. | Tenant-scoped injection/recipient/API-log lookups, added tenant conflict detection, scoped row mutations/suppression/campaign counter refreshes, and treated mismatches as orphaned diagnostics. | `go test ./internal/service/kumo`; `go test -race ./internal/service/kumo` |
| High | Send API overview/template stats could double-count Kumo rows because legacy `status=2` rows were counted as Postfix sends and then Kumo queued counts were added again. | Excluded Kumo engine rows from Postfix legacy queries and kept Kumo totals based on `injection_status` and webhook-driven `delivery_status`. | `go test ./internal/controller/batch_mail` |
| High | Campaign analytics stat-chart dashboard did not include Kumo lifecycle counters even though the frontend analytics page uses that endpoint. | Merged Kumo queued/delivered/deferred/bounced/expired/complained counters into the stat-chart dashboard and recomputed rates from final delivery status. | `go test ./internal/controller/batch_mail`; `pnpm test`; `pnpm build` |
| Medium | Runtime backpressure was acquired after RFC822 and JSON payload generation, limiting only the HTTP call instead of the full injection workload. | Acquired the runtime slot before RFC822/payload generation and added a regression hook proving backpressure runs before message building. | `go test ./internal/service/outbound`; targeted race test |
| Medium | Generated Kumo webhook policy always used the deployment-specific `mail.gitdate.ink` URL. | Policy generation now uses the configured BillionMail base/reverse-proxy URL when available and emits a warning when falling back to `https://mail.gitdate.ink/api/kumo/events`. | `go test ./internal/service/kumo`; policy regression tests |
| Medium | Raw webhook duplicate helper was dead code and the raw webhook Redis marker had no reader. | Removed the unused raw webhook duplicate path; event-level Redis/DB idempotency remains the authoritative dedupe layer. | `go test ./internal/service/kumo` |
| Medium | `kumo_message_injections` had a global `(message_id, recipient)` uniqueness constraint, which was stricter than tenant-owned message state. | Migration now drops the global constraint and adds tenant-aware uniqueness plus tenant queue/status indexes. | `go test ./...` |
| Medium | Send API worker result updates used only the global log ID even though queued logs now carry tenant IDs. | Scoped worker result updates by `tenant_id` when present while preserving legacy log rows with no tenant. | `go test ./internal/service/batch_mail` |

## Verification

- `cd core && go vet ./...`
- `cd core && go test ./internal/service/kumo ./internal/service/outbound ./internal/service/batch_mail ./internal/controller/batch_mail ./internal/controller/kumo`
- `cd core && go test -race ./internal/service/kumo ./internal/service/outbound ./internal/service/batch_mail`
- `cd core && go test ./...`
- `cd core/frontend && pnpm test`
- `cd core/frontend && pnpm build`

All commands passed.

## Notes

- Kumo HTTP `2xx` remains queued/accepted only. Final delivery continues to come from KumoMTA webhook events.
- Postfix compatibility remains intact for local/mailbox/system/fallback flows.
- This review did not mutate DigitalOcean infrastructure or KumoMTA droplets.
