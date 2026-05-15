# PRD: Deployed KumoMTA Integration for BillionMail

Date: 2026-05-12
Last updated: 2026-05-14

Status: Product and engineering requirements only. This document describes future implementation work. It does not implement code, schema, Docker, deployment, or configuration changes.

## 1. Executive Summary

BillionMail should integrate with the already deployed KumoMTA droplet as an external outbound delivery engine. The integration should use authenticated HTTP injection, KumoMTA webhook/log events, metrics polling, and generated KumoMTA policy artifacts. Postfix should remain available for mailbox, local, system, and compatibility flows. The KumoMTA source code should not be embedded into this repository.

The commercial SaaS model should be B2B workspaces. Each paying customer organization is a tenant with its own admins, members, domains, API keys, contacts, templates, campaigns, suppression lists, quotas, sending profiles, analytics, and KumoMTA pool assignments. A login account may belong to multiple tenant workspaces, but all business data access must be scoped by a server-validated tenant context.

The first production-grade version should keep BillionMail as the control plane and use KumoMTA as the delivery data plane:

- BillionMail owns tenants, users, domains, contacts, templates, campaigns, API keys, suppression, quotas, rendering, tracking, and analytics.
- KumoMTA owns high-volume outbound queueing, egress source selection, egress pools, traffic shaping, retries, DKIM signing, and delivery event publication.
- Campaign and Send API workflows should move from local Postfix SMTP submission to KumoMTA HTTP injection for high-volume outbound mail.
- BillionMail must stop treating "submitted to the local sender" as "delivered". KumoMTA acceptance means `queued` or `accepted`; final delivery state comes from KumoMTA webhook events.
- Real B2B workspace multi-tenancy must be introduced before selling this as a hosted SaaS product. Current accounts/RBAC are not enough because business tables are global and not tenant-scoped.

## 2. Problem Statement

The product goal is to sell BillionMail to multiple customer organizations. Each organization gets a tenant workspace where its admins configure sender domains, sender identities, API keys, contacts, campaigns, team members, and delivery limits. Some tenants may need to send up to 1 million emails per day.

The current codebase can send campaigns and API-triggered emails, but it is centered on local Postfix SMTP and global business data. That is not sufficient for a commercial multi-tenant high-volume sender because:

- Tenant data is not isolated at the database query level.
- Sending identity, domain ownership, API logs, contacts, campaigns, suppression, and analytics are global.
- Current delivery analytics are Postfix-log oriented.
- Current send status conflates SMTP send completion with delivery outcome.
- Local Postfix is not designed in this codebase as the high-volume multi-tenant outbound data plane.
- KumoMTA already exists as a deployed service and should be used for the outbound data plane instead of being embedded into BillionMail.

## 3. Goals

1. Define the product and engineering requirements for integrating the deployed KumoMTA droplet with BillionMail.
2. Specify the target outbound architecture for high-volume campaign and API sending.
3. Specify the multi-tenant model required before commercial sale.
4. Define API routes, backend interfaces, database direction, frontend screens, state management, caching, latency, reliability, and tests.
5. Preserve existing Postfix/Dovecot/Roundcube flows for mailbox and local/system mail.
6. Provide ASCII mock screens that fit the current Vue + Naive UI admin product style.

## 4. Non-Goals

- Do not implement the integration in this document.
- Do not remove Postfix, Dovecot, Rspamd, or Roundcube.
- Do not vendor or embed the KumoMTA source code into the BillionMail repository.
- Do not run production KumoMTA inside the same BillionMail Docker Compose stack.
- Do not rely on SMTP relay as the long-term native integration path.
- Do not introduce tenant switching before tenant isolation is designed and enforced end to end.
- Do not mark KumoMTA accepted messages as delivered before final events arrive.

## 5. Current-State Codebase Analysis

### 5.1 Runtime Architecture

The current deployment in `docker-compose.yml` runs:

- `pgsql-billionmail`
- `redis-billionmail`
- `rspamd-billionmail`
- `dovecot-billionmail`
- `postfix-billionmail`
- `webmail-billionmail`
- `core-billionmail`

Postfix exposes SMTP, SMTPS, and submission ports. Dovecot and Roundcube support mailbox and webmail flows. The Go core service manages the admin UI, APIs, background jobs, domains, contacts, campaigns, templates, API sending, tracking, and log aggregation.

Current simplified architecture:

```text
+---------------------+        +----------------------------+
| Browser / API user  |------->| BillionMail Core           |
+---------------------+        | Go / GoFrame API and jobs  |
                               +-------------+--------------+
                                             |
                    +------------------------+------------------------+
                    |                                                 |
                    v                                                 v
          +------------------+                              +------------------+
          | PostgreSQL       |                              | Redis            |
          | global data      |                              | jobs and locks   |
          +------------------+                              +------------------+
                    |
                    v
          +------------------+
          | Postfix          |
          | local outbound   |
          +--------+---------+
                   |
                   v
          +------------------+
          | Recipient MX     |
          +------------------+

          +------------------+       +------------------+
          | Dovecot          |<----->| Roundcube        |
          | mailbox access   |       | webmail          |
          +------------------+       +------------------+
```

### 5.2 Backend Stack

The backend is Go with GoFrame. Relevant current areas:

- SMTP sender: `core/internal/service/mail_service/sending.go`
- Campaign worker: `core/internal/service/batch_mail/task_executor.go`
- Send API worker: `core/internal/service/batch_mail/api_mail_send.go`
- Send API public controller: `core/internal/controller/batch_mail/batch_mail_v1_api_mail_send.go`
- Database initialization: `core/internal/service/database_initialization/*.go`
- Auth/RBAC middleware: `core/internal/service/rbac/*.go`, `core/internal/service/middlewares/rbac.go`
- Postfix log/stat aggregation: `core/internal/service/maillog_stat/*.go`

### 5.3 Frontend Stack

The frontend is Vue with Naive UI, Pinia, Axios, and static route modules.

Relevant current areas:

- Routes: `core/frontend/src/router/router.ts`
- Settings routes: `core/frontend/src/router/modules/settings.ts`
- API client wrapper: `core/frontend/src/api/index.ts`
- Existing API module pattern: `core/frontend/src/api/modules/*.ts`
- Send API page: `core/frontend/src/views/api/index.vue`
- Campaign task pages: `core/frontend/src/views/market/task/*.vue`
- Send Queue page: `core/frontend/src/views/settings/send-queue/index.vue`
- SMTP page: `core/frontend/src/views/smtp/index.vue`

Frontend conventions to preserve:

- Use Naive UI cards, tables, forms, modals, tags, tooltips, tabs, and buttons.
- Keep settings screens under route tabs when the workflow is operational settings.
- Keep dense operational pages table-first.
- Use the existing Axios wrapper and endpoint module pattern.
- Add a tenant Pinia store only when tenant switching is actually implemented.

### 5.4 Current Campaign Sending Flow

Campaign sending is currently driven by `ProcessEmailTasks`:

```text
email_tasks where task_process in (0,1), pause = 0, start_time <= now
        |
        v
TaskExecutor
        |
        v
recipient_info rows selected and marked in process
        |
        v
render personalized content
        |
        v
mail_service.NewEmailSenderWithLocal(addresser)
        |
        v
SMTP to local Postfix
        |
        v
recipient_info.is_sent updated
```

Current concern: `recipient_info.is_sent` is used as a processing/sent flag. It does not model KumoMTA injection, queue acceptance, delivery, bounce, deferral, expiration, or complaint as distinct states.

### 5.5 Current Send API Flow

The public Send API controller records API mail into `api_mail_logs`. A background worker processes pending logs:

```text
POST /api/batch_mail/api/send
        |
        v
record api_mail_logs status = 0
        |
        v
ProcessApiMailQueueWithLock via Redis lock
        |
        v
ProcessApiMailQueue
        |
        v
group pending logs by addresser
        |
        v
render content from api_templates + email_templates + contact data
        |
        v
sendApiMailWithSender using mail_service.EmailSender
        |
        v
api_mail_logs.status = 2 or 3
```

Current concern: `api_mail_logs.status = 2` means SMTP submission succeeded, not final recipient delivery. With KumoMTA this must become a queued/injected state, and delivery must be event-driven.

### 5.6 Current Database Tenancy Gap

The current codebase has accounts and RBAC, but product/business data is not tenant-scoped.

Global or customer-owned tables include:

- `domain`
- `mailbox`
- `alias`
- `alias_domain`
- `bm_bcc`
- `bm_relay`
- `bm_relay_config`
- `bm_relay_domain_mapping`
- `bm_domain_smtp_transport`
- `bm_multi_ip_domain`
- `bm_sender_ip_warmup`
- `bm_sender_ip_mail_provider`
- `bm_campaign_warmup`
- `bm_contact_groups`
- `bm_contacts`
- `bm_tags`
- `bm_contact_tags`
- `email_templates`
- `email_tasks`
- `recipient_info`
- `unsubscribe_records`
- `abnormal_recipient`
- `api_templates`
- `api_mail_logs`
- `api_ip_whitelist`
- `mailstat_*`
- `bm_operation_logs`
- tenant-specific `bm_options`

This means the current product is multi-user, not multi-tenant. A SaaS version requires tenant-scoped tables, tenant-aware queries, tenant-bound API keys, tenant-aware quotas, and tenant-specific sender policy.

## 6. KumoMTA Research Conclusions

### 6.1 HTTP Injection Is The Correct Native Integration

KumoMTA exposes HTTP injection through `/api/inject/v1`. BillionMail owns message rendering, personalization, unsubscribe links, tracking links, and Message-ID generation, so it can inject fully rendered RFC 5322 messages over HTTP and pass structured metadata.

Use HTTP injection instead of SMTP relay for the native integration because:

- BillionMail can send tenant/campaign/recipient/API-log metadata deterministically.
- HTTP request/response status can be handled cleanly in Go.
- Auth, TLS, timeout, retry, and idempotency controls are easier to enforce.
- It avoids pretending KumoMTA is just an external SMTP relay when the codebase controls the sender path.
- It allows future use of KumoMTA features such as deferred generation only after operational validation.

SMTP relay can remain useful for quick testing or compatibility, but it should not be the primary product architecture.

### 6.2 Queue Naming And Metadata

Use KumoMTA queue metadata convention:

```text
campaign:tenant@domain
```

Recommended mapping:

- `campaign`: `campaign_<email_tasks.id>` for campaigns, `api_<api_templates.id>` for Send API, or `system` for internal mail.
- `tenant`: stable tenant slug or ID, such as `tenant_42`.
- `domain`: recipient destination domain, such as `gmail.com`.

Example:

```text
campaign_981:tenant_42@gmail.com
api_17:tenant_42@yahoo.com
```

This lets KumoMTA queue behavior, metrics, and traffic shaping reflect tenant and campaign boundaries.

### 6.3 Egress Sources And Pools

KumoMTA egress sources should represent sending IPs and EHLO names. Egress pools should represent tenant-facing sending profiles, such as:

- shared pool
- dedicated IP pool
- warmup pool
- high-reputation pool
- restricted/recovery pool

BillionMail should store tenant-to-pool assignments and generate KumoMTA policy artifacts. KumoMTA should apply the actual source/pool selection at delivery time.

### 6.4 Traffic Shaping And TSA

KumoMTA traffic shaping should be the primary mechanism for provider-specific outbound control. BillionMail should keep product-level quotas and campaign throttles, while KumoMTA handles delivery-plane throttles such as:

- connection limits
- message rate limits
- messages per connection
- retry intervals
- TLS behavior
- destination-provider rules
- Traffic Shaping Automation response handling

This avoids pushing provider-specific SMTP behavior into BillionMail's campaign worker.

### 6.5 DKIM Signing

BillionMail currently manages domain DNS and Rspamd DKIM for the local stack. For KumoMTA-sent mail, KumoMTA should sign outbound mail using its DKIM helper/config so the signature is applied at the actual outbound MTA.

Product requirement:

- BillionMail remains the UI/source of truth for sender domain verification and selector/key metadata.
- KumoMTA receives or references the signing key material required to sign for that sender domain.
- Domain DNS readiness must clearly show whether the domain is ready for KumoMTA sending.
- If both Postfix/Rspamd and KumoMTA are active, the UI must clarify which engine signs which flow.

### 6.6 Webhooks And Event Ingestion

KumoMTA log hooks/webhooks should publish delivery events back to BillionMail. BillionMail should ingest events, normalize them, deduplicate them, update message state, update analytics, update suppression, and update tenant usage.

Events to support:

- injected/accepted where available
- delivered
- deferred/transient failure
- bounced/permanent failure
- expired
- complaint/feedback loop where available
- rejection/policy failure where available

Webhook events must include enough metadata to correlate back to:

- tenant
- campaign task
- recipient row
- API log row
- sender domain
- Kumo queue
- BillionMail Message-ID

### 6.7 Metrics

KumoMTA metrics should be polled by BillionMail in the background and cached for the UI. Metrics failures should not block sending.

Metrics should support:

- Kumo node health
- injection success/failure rate
- queue depth by tenant/campaign/domain
- delivery/defer/bounce rate
- egress pool/source utilization
- webhook lag
- event ingestion lag

### 6.8 Integration References

The Mautic, Ongage, and EmailElement integration docs are useful references, but they mostly demonstrate external application connector workflows. For BillionMail, native HTTP injection is better because this codebase owns the campaign and API send paths.

## 7. Target Architecture

### 7.1 High-Level Architecture

```text
                              CONTROL PLANE

+-------------------+       +----------------------------------------+
| Tenant user       |------>| BillionMail UI/API                     |
| browser or API    |       | auth, tenants, domains, campaigns      |
+-------------------+       +--------------------+-------------------+
                                                |
                      +-------------------------+-------------------------+
                      |                                                   |
                      v                                                   v
            +---------------------+                           +---------------------+
            | PostgreSQL          |                           | Redis               |
            | tenant-scoped data  |                           | locks, quotas, jobs |
            +---------------------+                           +---------------------+
                      |
                      | rendered message + tenant metadata
                      v

                               DATA PLANE

            +-----------------------------------------------+
            | Deployed KumoMTA Droplet / Cluster            |
            | HTTP injection /api/inject/v1                 |
            +----------------------+------------------------+
                                   |
                                   v
            +-----------------------------------------------+
            | Scheduled queues                              |
            | campaign:tenant@destination-domain            |
            +----------------------+------------------------+
                                   |
                                   v
            +--------------------+     +--------------------+
            | Egress pool        |---->| Egress source IP   |
            | tenant mapped      |     | EHLO + DKIM        |
            +--------------------+     +---------+----------+
                                                |
                                                v
                                      +--------------------+
                                      | Recipient MX       |
                                      +---------+----------+
                                                |
                                                v
            +-----------------------------------------------+
            | KumoMTA webhooks/log hooks                    |
            | delivered, bounced, deferred, expired         |
            +----------------------+------------------------+
                                   |
                                   v
            +-----------------------------------------------+
            | BillionMail event receiver                    |
            | status, analytics, suppression, billing usage |
            +-----------------------------------------------+
```

### 7.2 Responsibility Split

```text
+------------------------------+------------------------------+
| BillionMail                  | KumoMTA                      |
+------------------------------+------------------------------+
| tenants and users            | outbound queues              |
| RBAC and tenant membership   | egress source selection      |
| contacts and lists           | egress pools                 |
| templates and rendering      | provider traffic shaping     |
| campaign scheduling          | retry and expiration         |
| API keys and API logs        | DKIM signing                 |
| suppression and unsubscribe  | delivery event emission      |
| quotas and billing usage     | queue and node metrics       |
| analytics UI                 | SMTP delivery to MX servers  |
+------------------------------+------------------------------+
```

### 7.3 Campaign Send Flow With KumoMTA

```text
Campaign worker
      |
      v
Load tenant-scoped email_tasks and recipient_info
      |
      v
Check tenant quota, suppression, sender profile, domain readiness
      |
      v
Render one recipient message
      |
      v
Generate or preserve BillionMail Message-ID
      |
      v
POST to KumoMTA /api/inject/v1
      |
      +-- HTTP 2xx accepted --> recipient_info.injection_status = queued
      |
      +-- HTTP/network error --> keep retryable, backoff, do not mark delivered
      |
      v
KumoMTA queue campaign:tenant@domain
      |
      v
Recipient MX delivery attempt
      |
      v
KumoMTA webhook event
      |
      v
BillionMail event receiver updates delivery_status and analytics
```

### 7.4 Send API Flow With KumoMTA

```text
POST /api/batch_mail/api/send
      |
      v
Resolve API key to tenant server-side
      |
      v
Validate tenant quota, sender profile, addresser, recipient, IP whitelist
      |
      v
Insert api_mail_logs status = pending
      |
      v
API worker renders message
      |
      v
POST to KumoMTA /api/inject/v1
      |
      v
api_mail_logs.injection_status = queued
api_mail_logs.delivery_status = pending
      |
      v
KumoMTA webhook updates final status
```

### 7.5 Webhook Event Flow

```text
KumoMTA log hook / webhook
      |
      v
POST /api/kumo/events
      |
      v
Verify HMAC or token
      |
      v
Normalize event
      |
      v
Idempotency check by provider_event_id or event hash
      |
      v
Find message by X-BM-* headers and Message-ID
      |
      v
Update message delivery_status
      |
      v
Update task/API/template/domain/provider analytics
      |
      v
Update suppression, bounce, complaint, usage counters
```

### 7.6 GoDaddy DNS Requirements For gitdate.ink

`gitdate.ink` is managed in GoDaddy DNS. Current public DNS verification shows:

- Nameservers are `ns43.domaincontrol.com` and `ns44.domaincontrol.com`, so GoDaddy DNS is authoritative.
- `email.gitdate.ink` resolves to `192.241.130.241`, the KumoMTA droplet public IP.
- `mail.gitdate.ink` resolves to `159.89.33.85`, the BillionMail app droplet public IP.
- `gitdate.ink` currently resolves to `216.150.1.1`.
- `www.gitdate.ink` currently points through Vercel DNS.
- `gitdate.ink` MX currently points to Microsoft 365: `gitdate-ink.mail.protection.outlook.com`.
- Root SPF remains Microsoft-only: `v=spf1 include:spf.protection.outlook.com -all`.
- `email.gitdate.ink` has subdomain SPF: `v=spf1 ip4:192.241.130.241 ~all`.
- `s1._domainkey.email.gitdate.ink` has a published DKIM TXT record for the `email.gitdate.ink` subdomain.
- Root DMARC is published as `v=DMARC1; p=quarantine; adkim=r; aspf=r; rua=mailto:info@gitdate.ink;`.
- `_dmarc.email.gitdate.ink` is published as `v=DMARC1; p=none; rua=mailto:info@gitdate.ink`.

Current deployment-specific records:

| Type | Name | Value | Purpose |
| --- | --- | --- | --- |
| A | `mail` | `159.89.33.85` | BillionMail app, tracking URLs, and Kumo webhook receiver |
| A | `email` | `192.241.130.241` | KumoMTA control and HTTP injection endpoint |
| MX | `@` | `gitdate-ink.mail.protection.outlook.com` | Microsoft 365 inbound mail |
| TXT | `@` | `v=spf1 include:spf.protection.outlook.com -all` | Microsoft 365 root-domain SPF |
| TXT | `email` | `v=spf1 ip4:192.241.130.241 ~all` | SPF for mail using the `email.gitdate.ink` subdomain |
| TXT | `s1._domainkey.email` | published RSA DKIM key | DKIM selector `s1` for `email.gitdate.ink` |
| TXT | `_dmarc` | `v=DMARC1; p=quarantine; adkim=r; aspf=r; rua=mailto:info@gitdate.ink;` | Root-domain DMARC policy |
| TXT | `_dmarc.email` | `v=DMARC1; p=none; rua=mailto:info@gitdate.ink` | Subdomain DMARC policy for `email.gitdate.ink` |

The GoDaddy zone also contains Microsoft 365 autodiscover/Teams records, Vercel records, DigitalOcean app records, ACME challenge TXT records, and GoDaddy `_domainconnect`. These are outside the KumoMTA integration path and should be left unchanged unless the owning service is intentionally migrated.

GoDaddy verification and remaining configuration steps:

1. Sign in to GoDaddy.
2. Open **Domain Portfolio**.
3. Select `gitdate.ink`.
4. Open **DNS** or **Edit DNS**.
5. Confirm the BillionMail app record exists:
   - Type: `A`
   - Name: `mail`
   - Value: `159.89.33.85`
   - TTL: `600 seconds`
6. Confirm the KumoMTA control record exists:
   - Type: `A`
   - Name: `email`
   - Value: `192.241.130.241`
   - TTL: `1 Hour`
7. Confirm Microsoft 365 inbound mail remains unchanged:
   - Type: `MX`
   - Name: `@`
   - Value: `gitdate-ink.mail.protection.outlook.com`
   - Priority: `0`
8. Do not replace the root SPF blindly. Current root SPF is Microsoft-only. Production root-domain outbound SPF must later merge Microsoft plus the real sending egress if `gitdate.ink` itself sends outbound mail:

```text
Microsoft only:
v=spf1 include:spf.protection.outlook.com -all

Microsoft plus a future provider:
v=spf1 include:spf.protection.outlook.com include:PROVIDER_SPF_VALUE -all

Microsoft plus a future allowed egress IP:
v=spf1 include:spf.protection.outlook.com ip4:REAL_EGRESS_IP -all
```

9. Keep the `email.gitdate.ink` SPF/DKIM/DMARC records for subdomain-level Kumo testing, but do not treat `ip4:192.241.130.241` as production direct-to-MX readiness while the DigitalOcean SMTP block remains unresolved.
10. Verify DNS after propagation:

```bash
dig +short A mail.gitdate.ink
# expected: 159.89.33.85

dig +short A email.gitdate.ink
# expected: 192.241.130.241

dig +short MX gitdate.ink
# expected unless intentionally changed: 0 gitdate-ink.mail.protection.outlook.com.

dig +short TXT email.gitdate.ink
# expected: "v=spf1 ip4:192.241.130.241 ~all"

dig +short TXT s1._domainkey.email.gitdate.ink
# expected: DKIM TXT record beginning with "v=DKIM1; k=rsa; p="

dig +short TXT _dmarc.gitdate.ink
# expected: "v=DMARC1; p=quarantine; adkim=r; aspf=r; rua=mailto:info@gitdate.ink;"

dig +short TXT _dmarc.email.gitdate.ink
# expected: "v=DMARC1; p=none; rua=mailto:info@gitdate.ink"
```

11. Since `mail.gitdate.ink` now resolves, configure BillionMail:
    - `reverse_proxy_domain = https://mail.gitdate.ink`
    - reverse proxy forwards `Host`, `X-Real-IP`, `X-Forwarded-For`, and `X-Forwarded-Proto`
12. Configure KumoMTA webhooks to call:

```text
https://mail.gitdate.ink/api/kumo/events
```

DNS warning: pointing `email.gitdate.ink` at KumoMTA and publishing SPF/DKIM/DMARC for the `email` subdomain makes the Kumo control/sender identity discoverable and testable, but it does not solve DigitalOcean's outbound SMTP block. SPF, PTR, and EHLO must align with the actual outbound egress IP. If using external KumoProxy, an external Kumo node, provider HTTP API routing, or provider SMTP submission, the DNS readiness checks must use that real egress source rather than assuming `192.241.130.241` is the production sending IP.

### 7.7 Gitdate Droplet Connectivity And Delivery Egress

Current droplet topology:

```text
+--------------------------------------------------------------------------------+
| GoDaddy DNS: gitdate.ink                                                        |
| NS: ns43.domaincontrol.com / ns44.domaincontrol.com                             |
+--------------------------------------------------------------------------------+
           |                                                |
           v                                                v
+-------------------------------+              +-------------------------------+
| mail.gitdate.ink              |              | email.gitdate.ink             |
| BillionMail app droplet       |              | KumoMTA control droplet       |
| Public IP: 159.89.33.85       |              | Public IP: 192.241.130.241    |
| Private IP: 10.108.0.3        |              | Private IP: 10.116.0.2        |
| Region/VPC: nyc3/default-nyc3 |              | Region/VPC: nyc1/default-nyc1 |
+-------------------------------+              +-------------------------------+
           |                                                ^
           | Kumo webhooks                                  |
           +<-----------------------------------------------+
           | https://mail.gitdate.ink/api/kumo/events       |
           |
           | authenticated HTTP injection
           v
+-------------------------------+
| KumoMTA /api/inject/v1        |
| queues, policy, DKIM, metrics |
+---------------+---------------+
                |
                | delivery egress must leave DigitalOcean
                v
+-------------------------------+
| External allowed egress path  |
| KumoProxy, external Kumo node,|
| provider HTTP API, or 2525    |
+---------------+---------------+
                |
                v
+-------------------------------+
| Recipient MX / provider API   |
+-------------------------------+
```

The two current droplets are in different DigitalOcean regions and VPCs. BillionMail is in NYC3 on `default-nyc3`; KumoMTA is in NYC1 on `default-nyc1`. They are not privately connected unless VPC peering is created or one droplet is moved/rebuilt into the same region/VPC.

KumoMTA injection path requirements:

- Preferred path: create VPC peering between `default-nyc3` and `default-nyc1`, then allow BillionMail private IP `10.108.0.3` to reach KumoMTA private IP `10.116.0.2` on the Kumo HTTP listener.
- Acceptable path: expose `https://email.gitdate.ink` with TLS, authentication, and source allowlisting for BillionMail public IP `159.89.33.85`.
- Never expose `/api/inject/v1`, Kumo admin APIs, or metrics publicly without strong authentication and network allowlisting.

Production delivery egress requirements:

- DigitalOcean blocks SMTP ports `25`, `465`, and `587` on Droplets by default.
- Therefore the KumoMTA DigitalOcean droplet must not be treated as the final direct-to-MX production egress node.
- For B2B production sending, KumoMTA must route delivery through one of:
  - external KumoProxy/SOCKS5 hosted on infrastructure that permits outbound SMTP
  - external KumoMTA egress node hosted where outbound TCP 25 is permitted
  - provider HTTP API routing from KumoMTA policy
  - provider SMTP submission on a supported non-blocked port such as `2525`, where the provider supports it
- Reverse proxy, DNS, VPC peering, or port forwarding cannot bypass DigitalOcean's SMTP restriction and must not be documented as a production workaround.

Tenant sending profiles must expose the selected egress mode:

```text
external_kumoproxy
external_kumo_node
provider_http_api
provider_smtp_2525
```

Tenant/domain readiness must verify:

- `mail.gitdate.ink` app/tracking/webhook DNS
- `email.gitdate.ink` Kumo injection/control DNS
- webhook reachability from KumoMTA to BillionMail
- Kumo injection reachability from BillionMail to KumoMTA
- DigitalOcean SMTP block status for any DO-hosted node
- real outbound egress SPF, DKIM, DMARC, PTR, and EHLO alignment

## 8. Product Requirements

### 8.1 Operator KumoMTA Configuration

An operator must be able to configure the deployed KumoMTA connection:

- Base URL for the KumoMTA HTTP injection endpoint.
- Auth token or HMAC secret.
- Webhook secret.
- TLS verification mode.
- Optional custom CA bundle reference.
- Metrics endpoint URL.
- Request timeout.
- Retry policy.
- Health check behavior.
- Default sending engine: Postfix, KumoMTA, or hybrid by workflow.
- Default Kumo egress pool.
- Whether KumoMTA is enabled for campaigns, Send API, or both.

### 8.2 KumoMTA Health And Metrics

An operator must see:

- connection status
- last successful health check
- last failed health check
- injection endpoint latency
- metrics endpoint latency
- webhook receiver status
- webhook lag
- queued messages
- active queues
- defer rate
- bounce rate
- event ingestion rate

### 8.3 Tenant Sending Profiles

A tenant/admin must be able to assign sender domains to sending profiles.

A sending profile should include:

- profile name
- tenant ID
- sender domains
- default From domain
- Kumo egress pool
- DKIM selector
- traffic class or policy name
- daily quota
- hourly quota
- warmup mode
- suppression behavior
- default tracking domain where applicable

### 8.4 Domain Readiness For KumoMTA

The domain UI must show whether a domain is ready for KumoMTA sending:

- SPF includes the required sending source or include mechanism.
- DKIM selector for KumoMTA is published and matches active key material.
- DMARC exists and is valid enough for the plan.
- MX and mailbox readiness remain separate from outbound KumoMTA readiness.
- PTR/rDNS and EHLO alignment is shown for dedicated IP plans.
- The UI clearly differentiates local Postfix/Rspamd signing from KumoMTA signing.
- For `gitdate.ink`, `mail.gitdate.ink` currently resolves to `159.89.33.85` and may be used for app, tracking, and webhook URLs after reverse proxy health passes.
- For `gitdate.ink`, `email.gitdate.ink` currently resolves to `192.241.130.241` and may be used as the KumoMTA control/injection DNS name after TLS, auth, and source allowlisting are configured.
- For `email.gitdate.ink`, SPF, DKIM selector `s1`, and subdomain DMARC are currently published, but production readiness must still validate the real outbound egress path.
- The production readiness check must validate the actual outbound egress IP/provider, not only the DigitalOcean KumoMTA control droplet, because DigitalOcean blocks SMTP delivery ports.
- The existing Microsoft 365 MX/SPF records must remain intact unless inbound mail is intentionally migrated away from Microsoft 365.

### 8.5 Campaign Sending

Campaign create/edit must let the user select or inherit a sending profile.

Rules:

- If no Kumo profile is assigned, the campaign uses the tenant default.
- If KumoMTA is disabled for campaigns, preserve current Postfix behavior.
- If KumoMTA is enabled, campaign sending uses `KumoHTTPMailer`.
- KumoMTA accepted response sets queued/injected status, not delivered.
- Final campaign analytics come from KumoMTA events plus existing open/click tracking.

### 8.6 Send API

Send API templates must show the delivery engine:

- Postfix/local
- KumoMTA
- inherited from tenant default

API keys must resolve tenant server-side. The caller should not be trusted to choose an arbitrary tenant.

API response requirements:

- Return accepted/queued status for async KumoMTA injection.
- Return BillionMail message ID or API log ID for tracking.
- Do not return delivered unless a final delivery event has occurred.
- Expose webhook health and recent event state in the Send API detail page.

### 8.7 Multi-Tenancy

Before commercial SaaS launch:

- Every customer-owned table must be tenant-scoped.
- Every business query must filter by tenant.
- Every write must attach tenant ID server-side.
- Every API key must belong to exactly one tenant.
- Every sender domain must belong to one tenant unless explicitly shared by operator policy.
- Tenant A must never read, update, delete, send from, or see analytics for Tenant B.
- UI tenant switching should be implemented only after backend enforcement exists.
- The primary product model is B2B workspaces: one tenant per paying customer organization.
- A login account may belong to multiple tenants, but the active tenant context must be validated on every request.
- Platform operators may see cross-tenant operational metadata, but tenant users must never see another tenant's customer data, recipients, campaigns, API logs, or message bodies.

### 8.8 How Multi-Tenancy Works When Selling BillionMail

The sellable product should behave like a B2B workspace SaaS application. The tenant boundary is the customer organization, not an individual user and not a KumoMTA queue. KumoMTA should receive tenant metadata for queueing and delivery, but BillionMail remains the source of truth for tenant identity, access, quotas, domains, and billing.

#### 8.8.1 Tenant Model

Core identity terms:

- `tenant`: a paying customer organization/workspace, such as "Acme Marketing".
- `account`: a login identity, such as `alex@agency.com`, which may belong to one or more tenants.
- `tenant_member`: the membership record connecting an account to a tenant with a tenant role.
- `tenant_owner`: the customer-side administrator who manages billing, members, domains, API keys, and sending profiles.
- `tenant_admin`: a customer-side administrator with operational access but not necessarily billing ownership.
- `tenant_marketer`: a customer user who manages contacts, templates, and campaigns.
- `tenant_developer`: a customer user who manages API keys, API templates, and integration diagnostics.
- `operator`: the BillionMail platform administrator who manages all tenants, plans, abuse controls, KumoMTA pools, and system health.

Tenant context rules:

- UI tenant selection is a convenience only; it is not a security boundary.
- Browser requests may include `X-Tenant-ID` after tenant switching exists, but the backend must verify membership before using it.
- Public Send API requests must resolve tenant from the API key server-side.
- Background workers must load the tenant from the queued campaign/API record, not from process-global state.
- KumoMTA events must map back to tenant using stored message injection rows and `X-BM-*` metadata.

#### 8.8.2 Ownership Matrix

| Object | Tenant-owned? | Visibility | KumoMTA relationship |
| --- | --- | --- | --- |
| Account login | No | Account owner and operator | None |
| Tenant/workspace | Yes | Tenant members and operator | Used in queue metadata |
| Tenant members | Yes | Tenant owner/admin and operator | None |
| Domains | Yes | Tenant members with permission | Kumo signing and routing input |
| Mailboxes | Yes | Tenant members with permission | Postfix/Dovecot flow unless outbound profile uses Kumo |
| Contacts/groups/tags | Yes | Tenant members with marketing permission | Render/input only, never stored in Kumo |
| Templates | Yes | Tenant members with marketing/API permission | Rendered by BillionMail before injection |
| Campaigns | Yes | Tenant marketers/admins | `campaign_<id>:tenant_<id>@domain` queues |
| API keys/templates | Yes | Tenant developers/admins | `api_<id>:tenant_<id>@domain` queues |
| Send API logs | Yes | Tenant developers/admins | Updated from Kumo events |
| Suppression lists | Yes | Tenant admins/marketers | Checked before Kumo injection |
| Quotas/usage | Yes | Tenant owner/admin and operator | Enforced before injection |
| Sending profiles | Yes | Tenant admin and operator | Maps tenant domains to Kumo pools |
| Kumo egress pools | Platform-owned | Operator; tenant sees assigned pool name/status | Delivery-plane source selection |
| Kumo delivery events | Tenant-owned after correlation | Tenant members see their own events; operator sees all | Ingested from Kumo webhooks |
| Aggregate system metrics | Platform-owned | Operator | Derived from Kumo metrics |

#### 8.8.3 Tenant Data Flow

```text
Login account
      |
      v
GET /api/tenants -> memberships
      |
      v
Select active tenant
      |
      v
Backend verifies account is tenant_member
      |
      v
All UI/API queries include server-validated tenant_id
      |
      v
Campaign/API worker loads tenant-owned send record
      |
      v
Quota + suppression + domain + profile checks
      |
      v
KumoMTA HTTP injection with X-BM-Tenant-ID
      |
      v
Kumo queue and delivery event
      |
      v
BillionMail updates only that tenant's records
```

#### 8.8.4 Use Case: Operator Creates A Tenant And Assigns Kumo Pool

Scenario: A platform operator provisions a new paying customer, assigns a plan, sets the daily quota, and chooses either a shared or dedicated KumoMTA pool.

Required behavior:

- Operator can create tenant without creating domains or campaigns yet.
- Tenant starts in `pending_setup` or `active` depending on billing/approval policy.
- Default quota is copied from plan but can be overridden by operator.
- Default Kumo pool is assigned at tenant or sending-profile level.
- Tenant users cannot change platform-owned Kumo pool sources.

```text
+--------------------------------------------------------------------------------+
| Operator / Tenants                                      [Create tenant]         |
+--------------------------------------------------------------------------------+
| Tenant             Plan          Status        Quota/day   Kumo pool      Risk  |
| Acme Marketing     Growth        Active        1,000,000   shared-a       Low   |
| Northwind Labs     Starter       Pending setup 50,000      shared-a       -     |
| Bluebird Agency    Enterprise    Suspended     2,000,000   dedicated-17   High  |
+--------------------------------------------------------------------------------+
| Create tenant                                                                    |
| Organization name     [ Acme Marketing                                      ]    |
| Owner email           [ owner@acme.example                                  ]    |
| Plan                  [ Growth v ]                                               |
| Daily quota           [ 1000000 ]                                                |
| Monthly quota         [ 30000000 ]                                               |
| Default Kumo pool     [ shared-a v ]                                             |
| Tenant status         (o) Active   ( ) Pending setup                             |
| Actions               [Cancel] [Create tenant]                                  |
+--------------------------------------------------------------------------------+
```

#### 8.8.5 Use Case: Tenant Owner Onboards Workspace

Scenario: The tenant owner signs in for the first time and sees an onboarding checklist that guides setup before high-volume sending is allowed.

Required behavior:

- Owner sees only their tenant workspace.
- Checklist is tenant-scoped.
- Sending remains blocked until minimum readiness checks pass.
- KumoMTA readiness is shown separately from mailbox/webmail readiness.

```text
+--------------------------------------------------------------------------------+
| Acme Marketing                                            [Tenant: Acme v]      |
+--------------------------------------------------------------------------------+
| Setup checklist                                      Sending status: Blocked     |
+--------------------------------------------------------------------------------+
| [ ] Add sender domain             Required before campaigns                      |
| [ ] Verify SPF/DKIM/DMARC         Required before KumoMTA outbound               |
| [ ] Create sending profile        Required before campaign/API sending           |
| [ ] Invite team members           Optional                                       |
| [ ] Create first contact group    Optional                                       |
| [ ] Create API key                Optional for developers                        |
+--------------------------------------------------------------------------------+
| Tenant plan                                                                      |
| Plan: Growth              Daily quota: 1,000,000       Kumo pool: shared-a       |
| Used today: 0             Remaining: 1,000,000         Status: Pending setup     |
+--------------------------------------------------------------------------------+
```

#### 8.8.6 Use Case: Tenant Admin Invites Members And Assigns Roles

Scenario: A tenant admin invites teammates and assigns role-based access inside the workspace.

Required behavior:

- Invitations are tenant-scoped.
- A single account can accept invitations to multiple tenants.
- Roles control feature access inside the active tenant only.
- Removing a member revokes access to that tenant without deleting the account.

```text
+--------------------------------------------------------------------------------+
| Acme Marketing / Team                                      [Invite member]      |
+--------------------------------------------------------------------------------+
| Members                                                                         |
| Name              Email                    Role              Status             |
| Priya Shah        priya@acme.example       Owner             Active             |
| Mateo Ruiz        mateo@acme.example       Marketer          Active             |
| Dana Lee          dana@acme.example        Developer         Invited            |
+--------------------------------------------------------------------------------+
| Invite member                                                                    |
| Email                 [ dana@acme.example                                    ]   |
| Role                  [ Developer v ]                                            |
| Permissions preview   API keys, API templates, Send API logs, delivery events    |
| Actions               [Cancel] [Send invitation]                                |
+--------------------------------------------------------------------------------+
| Role guide                                                                       |
| Owner       Billing, members, domains, sending profiles, all campaigns/API       |
| Admin       Domains, profiles, members, campaigns/API, analytics                 |
| Marketer    Contacts, templates, campaigns, campaign analytics                   |
| Developer   API keys, API templates, API logs, webhook/API diagnostics           |
+--------------------------------------------------------------------------------+
```

#### 8.8.7 Use Case: Tenant Admin Verifies Domain For KumoMTA

Scenario: A tenant admin adds a sender domain and verifies DNS readiness for KumoMTA outbound.

Required behavior:

- Domain ownership belongs to one tenant unless operator explicitly marks it shared.
- DNS checks show SPF, DKIM, DMARC, MX, and KumoMTA readiness.
- DKIM selector/key state is tied to the tenant domain and Kumo signing policy.
- Another tenant cannot add the same domain unless the operator resolves ownership.

```text
+--------------------------------------------------------------------------------+
| Acme Marketing / Domains / example.com                    [Recheck DNS]         |
+--------------------------------------------------------------------------------+
| Ownership                                                                         |
| Tenant               Acme Marketing                                               |
| Domain status        Verified                                                     |
| Outbound engine      KumoMTA                                                       |
| Sending profile      Marketing default                                            |
+--------------------------------------------------------------------------------+
| DNS readiness                                                                     |
| Record      Host                  Expected value                    Status        |
| SPF         example.com           include:kumo.shared-a.example     Pass          |
| DKIM        bm1._domainkey        p=MIIB...                         Pass          |
| DMARC       _dmarc                v=DMARC1; p=none                  Pass          |
| MX          example.com           mailbox host                      Optional      |
| PTR/EHLO    shared-a source       shared-a.kumo.example             Pass          |
+--------------------------------------------------------------------------------+
| KumoMTA readiness                                                                 |
| Kumo pool           shared-a                                                     |
| DKIM deploy state   Active in generated Kumo policy                              |
| Queue pattern       campaign_or_api:tenant_42@recipient-domain                   |
| Result              Ready for campaign and Send API outbound                     |
+--------------------------------------------------------------------------------+
```

#### 8.8.8 Use Case: Tenant Admin Creates Sending Profile

Scenario: A tenant admin creates a sending profile that maps verified domains to a KumoMTA egress pool and tenant quota.

Required behavior:

- Tenant can choose only domains owned by that tenant.
- Tenant sees pool assignment but cannot see other tenants on the pool.
- Operator can restrict which pools a tenant may use.
- Profile cannot be active until at least one domain is Kumo-ready.

```text
+--------------------------------------------------------------------------------+
| Acme Marketing / Sending Profiles                         [Create profile]      |
+--------------------------------------------------------------------------------+
| Profiles                                                                         |
| Name                  Domains              Kumo pool      Quota/day   Status     |
| Marketing default     example.com          shared-a       1,000,000   Ready      |
| Product launches      news.example.com     warmup-a       100,000     Warming    |
+--------------------------------------------------------------------------------+
| Profile editor                                                                    |
| Name                  [ Marketing default                                   ]     |
| Sender domains        [example.com x] [news.example.com x]                       |
| Default From domain   [ example.com v ]                                          |
| Kumo egress pool      [ shared-a v ]                                             |
| Daily profile quota   [ 1000000 ]                                                |
| Hourly profile quota  [ 50000 ]                                                  |
| Warmup mode           [ ] Enabled                                                |
| Status                Ready                                                      |
| Actions               [Cancel] [Save profile]                                   |
+--------------------------------------------------------------------------------+
```

#### 8.8.9 Use Case: Tenant Marketer Sends Campaign With Tenant Quota

Scenario: A marketer creates a campaign and selects the tenant sending profile. BillionMail checks tenant quota, suppression, domain readiness, and KumoMTA health before allowing the send.

Required behavior:

- Marketer sees only tenant-owned contact groups, templates, and sending profiles.
- Quota availability is calculated for the active tenant.
- KumoMTA acceptance updates `queued`; delivery updates arrive later from events.
- Campaign analytics are tenant-scoped.

```text
+--------------------------------------------------------------------------------+
| Acme Marketing / Campaigns / Spring Launch                 [Schedule send]      |
+--------------------------------------------------------------------------------+
| Content | Recipients | Delivery | Schedule | Review                              |
+--------------------------------------------------------------------------------+
| Delivery                                                                         |
| Sending profile       [ Marketing default v ]                                    |
| From address          [ Rahul <news@example.com> v ]                             |
| Engine                KumoMTA                                                    |
| Kumo pool             shared-a                                                   |
| Queue pattern         campaign_981:tenant_42@recipient-domain                    |
+--------------------------------------------------------------------------------+
| Tenant checks                                                                    |
| Recipients selected   245,000                                                    |
| Tenant quota left     842,120                                                    |
| Suppressed skipped    1,284                                                      |
| Domain readiness      Ready                                                      |
| Webhook health        Healthy                                                    |
| Result                Ready to schedule                                          |
+--------------------------------------------------------------------------------+
```

#### 8.8.10 Use Case: Tenant Developer Uses Send API

Scenario: A developer creates a tenant API key, sends transactional mail, and views KumoMTA injection/delivery state.

Required behavior:

- API key belongs to exactly one tenant.
- API key scopes limit what the key can do.
- API key resolves tenant server-side; caller cannot inject a different tenant ID.
- Send API logs show `queued` separately from `delivered`.

```text
+--------------------------------------------------------------------------------+
| Acme Marketing / Send API / Password Reset                    [Create key]      |
+--------------------------------------------------------------------------------+
| API template                                                                      |
| Template name        Password Reset                                              |
| Engine               KumoMTA                                                     |
| Sending profile      Transactional                                               |
| API key tenant       Acme Marketing                                              |
| Scopes               send:mail, read:logs                                        |
+--------------------------------------------------------------------------------+
| Recent sends                                                                      |
| Time       Recipient             Injection    Delivery     Message ID            |
| 12:01:11   user@gmail.com        queued       delivered    <...@example.com>     |
| 12:01:09   user@yahoo.com        queued       deferred     <...@example.com>     |
| 12:00:58   user@corp.com         retrying     pending      <...@example.com>     |
+--------------------------------------------------------------------------------+
| Integration note                                                                  |
| Tenant is resolved from this API key. Do not send X-Tenant-ID from your app.      |
+--------------------------------------------------------------------------------+
```

#### 8.8.11 Use Case: Multi-Tenant User Switches Workspace

Scenario: A consultant belongs to two customer tenants and switches workspace context.

Required behavior:

- Workspace switch updates active tenant context only after backend confirms membership.
- UI clears tenant-scoped caches after switching.
- URLs or stale tabs cannot reveal previous tenant data after switch.
- Background polling uses the active tenant context.

```text
+--------------------------------------------------------------------------------+
| BillionMail                                      Tenant: [Acme Marketing v]      |
+--------------------------------------------------------------------------------+
| Switch workspace                                                                 |
| Current account        consultant@example.com                                    |
| Available tenants                                                               |
| (o) Acme Marketing      Role: Admin       Plan: Growth       Status: Active      |
| ( ) Northwind Labs      Role: Marketer    Plan: Starter      Status: Active      |
+--------------------------------------------------------------------------------+
| After switch behavior                                                            |
| [x] Clear contacts/templates/campaign cache                                      |
| [x] Reload navigation permissions                                                |
| [x] Reload tenant quota and sending profiles                                     |
| [x] Use selected tenant for subsequent API calls after server validation         |
| Actions                [Cancel] [Switch workspace]                              |
+--------------------------------------------------------------------------------+
```

#### 8.8.12 Use Case: Tenant Hits Quota Or Abuse Threshold

Scenario: A tenant exhausts daily quota or crosses bounce/complaint limits. Sending should stop for that tenant without impacting other tenants.

Required behavior:

- Only the affected tenant is blocked or throttled.
- Existing queued KumoMTA messages may continue or pause according to operator policy.
- UI explains the tenant-level block.
- Operators can override, raise quota, or suspend tenant.

```text
+--------------------------------------------------------------------------------+
| Acme Marketing / Delivery Status                                                |
+--------------------------------------------------------------------------------+
| Sending blocked                                                                  |
| Reason              Daily quota exhausted                                        |
| Used today          1,000,000 of 1,000,000                                       |
| Campaigns paused    3                                                           |
| API sends blocked   Yes                                                         |
| Kumo queues         Existing queued mail continues under current policy          |
+--------------------------------------------------------------------------------+
| Recovery actions                                                                 |
| Tenant owner        Upgrade plan or wait until quota resets                      |
| Operator            [Increase quota] [Throttle tenant] [Suspend tenant]          |
+--------------------------------------------------------------------------------+
| Tenant isolation                                                                 |
| Other tenants on shared-a continue sending. Shared pool health remains normal.   |
+--------------------------------------------------------------------------------+
```

#### 8.8.13 Use Case: Operator Reviews Tenants And Suspends Abuse

Scenario: A platform operator monitors all tenants, sees one tenant with high complaint rate, and suspends that tenant without exposing recipient-level data to other tenants.

Required behavior:

- Operator can see cross-tenant operational metrics.
- Operator can inspect tenant-specific queue and abuse summary.
- Tenant suspension prevents new injection for that tenant.
- Other tenants on the same KumoMTA pool continue sending unless pool-level health requires throttling.

```text
+--------------------------------------------------------------------------------+
| Operator / Tenant Risk                                      [Refresh]           |
+--------------------------------------------------------------------------------+
| Tenant              Queue     Bounce   Complaint   Pool          Action          |
| Acme Marketing      18,402    1.2%     0.01%       shared-a      Review          |
| Bluebird Agency     92,118    8.9%     0.42%       shared-a      Suspend         |
| Northwind Labs      1,224     0.8%     0.00%       dedicated-17  Review          |
+--------------------------------------------------------------------------------+
| Tenant detail: Bluebird Agency                                                  |
| Status              Active                                                       |
| Risk                High complaint rate                                          |
| New injection       Allowed                                                      |
| Existing queues     92,118 messages                                              |
| Pool impact         shared-a healthy, other tenants unaffected                   |
| Actions             [Throttle] [Suspend tenant] [Export audit summary]           |
+--------------------------------------------------------------------------------+
```

### 8.9 Postfix Compatibility

Postfix remains available for:

- mailbox/webmail local SMTP flows
- system alerts
- compatibility fallback
- low-volume local tests
- workflows explicitly configured to use local delivery

KumoMTA should be the high-volume outbound engine for commercial campaign and API sending.

## 9. Public APIs To Add

All routes below are requirements for the future implementation. They do not exist in this document.

### 9.1 `GET /api/kumo/config`

Purpose: Return stored KumoMTA connection and feature configuration.

Auth: Operator/admin only.

Response:

```json
{
  "enabled": true,
  "campaigns_enabled": true,
  "api_enabled": true,
  "base_url": "https://kumo.example.com:8000",
  "inject_path": "/api/inject/v1",
  "metrics_url": "https://kumo.example.com:8000/metrics",
  "tls_verify": true,
  "auth_mode": "bearer",
  "has_auth_secret": true,
  "has_webhook_secret": true,
  "timeout_ms": 5000,
  "default_pool": "shared-default",
  "updated_at": 1778600000
}
```

Never return raw secrets.

### 9.2 `POST /api/kumo/config`

Purpose: Store KumoMTA configuration.

Auth: Operator/admin only.

Request:

```json
{
  "enabled": true,
  "campaigns_enabled": true,
  "api_enabled": true,
  "base_url": "https://kumo.example.com:8000",
  "inject_path": "/api/inject/v1",
  "metrics_url": "https://kumo.example.com:8000/metrics",
  "tls_verify": true,
  "auth_mode": "bearer",
  "auth_secret": "new-secret-or-empty-to-keep",
  "webhook_secret": "new-secret-or-empty-to-keep",
  "timeout_ms": 5000,
  "default_pool": "shared-default"
}
```

Behavior:

- Validate URL shape.
- Validate timeout bounds.
- Store secrets encrypted or protected by existing secret-management convention.
- Invalidate Kumo config cache after update.
- Audit the change.

### 9.3 `POST /api/kumo/test_connection`

Purpose: Test injection endpoint, metrics endpoint, TLS behavior, and auth.

Auth: Operator/admin only.

Request:

```json
{
  "base_url": "https://kumo.example.com:8000",
  "metrics_url": "https://kumo.example.com:8000/metrics",
  "auth_mode": "bearer",
  "auth_secret": "optional-test-secret",
  "tls_verify": true
}
```

Response:

```json
{
  "ok": true,
  "health_ms": 42,
  "metrics_ms": 88,
  "message": "KumoMTA reachable"
}
```

### 9.4 `GET /api/kumo/status`

Purpose: Return cached connection status and operational status.

Auth: Operator/admin only.

Response:

```json
{
  "connected": true,
  "last_ok_at": 1778600000,
  "last_error_at": 0,
  "last_error": "",
  "inject_latency_ms": 34,
  "metrics_latency_ms": 72,
  "webhook_last_seen_at": 1778600010,
  "webhook_lag_seconds": 3
}
```

### 9.5 `GET /api/kumo/metrics`

Purpose: Return cached KumoMTA metrics snapshots for UI dashboards.

Auth: Operator/admin only. Tenant users may receive tenant-filtered metrics later.

Response:

```json
{
  "snapshot_at": 1778600000,
  "queues": [
    {
      "queue": "campaign_981:tenant_42@gmail.com",
      "tenant_id": 42,
      "campaign_id": 981,
      "domain": "gmail.com",
      "ready": 1200,
      "scheduled": 300,
      "deferred": 80
    }
  ],
  "nodes": [
    {
      "name": "kumo-do-1",
      "healthy": true,
      "inject_rps": 220,
      "delivery_rps": 190
    }
  ]
}
```

### 9.6 `GET /api/kumo/pools`

Purpose: Return known KumoMTA egress pools and current tenant assignments.

Auth: Operator/admin only. Tenant users can receive allowed profiles later.

Response:

```json
{
  "pools": [
    {
      "id": 1,
      "name": "shared-default",
      "description": "Default shared pool",
      "sources": ["do-ip-1", "do-ip-2"],
      "tenant_count": 18,
      "enabled": true
    }
  ]
}
```

### 9.7 `POST /api/kumo/pools`

Purpose: Create or update known egress pool metadata in BillionMail and optionally generate KumoMTA config preview.

Auth: Operator/admin only.

Request:

```json
{
  "name": "shared-default",
  "description": "Default shared pool",
  "sources": ["do-ip-1", "do-ip-2"],
  "enabled": true
}
```

### 9.8 `POST /api/kumo/config/preview`

Purpose: Generate KumoMTA policy artifacts from BillionMail state without deploying them.

Auth: Operator/admin only.

Response:

```json
{
  "version": "2026-05-12T12:00:00Z",
  "files": [
    {
      "path": "policy/tenant_pools.lua",
      "content": "-- generated preview"
    },
    {
      "path": "policy/dkim.lua",
      "content": "-- generated preview"
    }
  ],
  "warnings": [
    "tenant_42 has no verified DKIM selector"
  ]
}
```

### 9.9 `POST /api/kumo/config/deploy`

Purpose: Deploy generated KumoMTA policy artifacts to the already deployed KumoMTA node or cluster.

Auth: Operator/admin only.

Requirements:

- Support dry run and deploy modes.
- Create config version records.
- Preserve rollback metadata.
- Return deployment status.
- Do not deploy if validation fails.

### 9.10 `POST /api/kumo/events`

Purpose: Receive KumoMTA webhook/log events.

Auth: JWT-bypassed but protected by HMAC or static token.

Middleware requirements:

- Add `/api/kumo/events` to JWT skip list only after HMAC/token validation middleware exists.
- Validate timestamp tolerance if HMAC includes timestamp.
- Reject missing or invalid signatures.
- Enforce request body size limits.
- Record raw event safely for audit/debug.

Request shape should support both single events and batches:

```json
{
  "events": [
    {
      "event_id": "kumo-event-abc",
      "event_type": "delivered",
      "timestamp": 1778600000,
      "message_id": "<1778600000.abc@example.com>",
      "recipient": "user@gmail.com",
      "sender": "news@example.com",
      "queue": "campaign_981:tenant_42@gmail.com",
      "headers": {
        "X-BM-Tenant-ID": "42",
        "X-BM-Campaign-ID": "981",
        "X-BM-Recipient-ID": "12345",
        "X-BM-Message-ID": "<1778600000.abc@example.com>"
      },
      "response": "250 2.0.0 OK",
      "remote_mx": "gmail-smtp-in.l.google.com"
    }
  ]
}
```

Response:

```json
{
  "accepted": 1,
  "duplicates": 0,
  "failed": 0
}
```

## 10. Future Tenant APIs

### 10.1 `GET /api/tenants/current`

Purpose: Return current server-validated tenant context, tenant plan, quotas, role, and permissions.

Response:

```json
{
  "tenant_id": 42,
  "tenant_name": "Acme Marketing",
  "tenant_slug": "acme-marketing",
  "role": "admin",
  "permissions": ["domains:write", "campaigns:write", "api_keys:read"],
  "plan": "Growth",
  "daily_quota": 1000000,
  "daily_used": 157880,
  "status": "active"
}
```

### 10.2 `POST /api/tenants/switch`

Purpose: Switch active tenant for UI sessions after backend tenant membership enforcement exists.

Request:

```json
{
  "tenant_id": 42
}
```

Behavior:

- Verify the authenticated account is an active `tenant_member`.
- Store active tenant context in the session/JWT refresh flow or return a renewed tenant-bound token.
- Clear or invalidate tenant-scoped UI/API caches after switch.
- Return 403 if the account is not a member of the requested tenant.

### 10.3 `GET /api/tenants`

Purpose: Return tenant workspaces the current account can access.

Response:

```json
{
  "tenants": [
    {
      "tenant_id": 42,
      "name": "Acme Marketing",
      "slug": "acme-marketing",
      "role": "admin",
      "status": "active",
      "plan": "Growth"
    },
    {
      "tenant_id": 88,
      "name": "Northwind Labs",
      "slug": "northwind-labs",
      "role": "marketer",
      "status": "active",
      "plan": "Starter"
    }
  ]
}
```

### 10.4 `POST /api/tenants`

Purpose: Create a tenant/workspace. Operator-created tenants may assign plan, quota, status, and default Kumo pool. Self-serve tenant creation should use default plan and require domain setup before sending.

Request:

```json
{
  "name": "Acme Marketing",
  "owner_email": "owner@acme.example",
  "plan_id": 2,
  "daily_quota": 1000000,
  "monthly_quota": 30000000,
  "default_kumo_pool": "shared-a",
  "status": "pending_setup"
}
```

### 10.5 `POST /api/tenants/:id/sending-profile`

Purpose: Create or update tenant sending profile and Kumo pool assignment.

Request:

```json
{
  "name": "Marketing default",
  "sender_domain_ids": [10, 11],
  "default_from_domain_id": 10,
  "kumo_pool": "shared-default",
  "dkim_selector": "bm1",
  "daily_quota": 1000000,
  "hourly_quota": 50000,
  "warmup_enabled": true
}
```

Behavior:

- Verify caller can administer the target tenant.
- Verify every `sender_domain_id` belongs to the target tenant and is Kumo-ready before activating the profile.
- Verify requested Kumo pool is allowed for the tenant by operator policy.
- Return profile readiness details so the UI can show why sending is blocked.

## 11. Backend Interfaces

### 11.1 OutboundMailer

Add an abstraction so campaign/API workers no longer depend directly on `mail_service.EmailSender`.

```go
type OutboundMailer interface {
    Send(ctx context.Context, req OutboundMessage) (*OutboundResult, error)
}

type OutboundMessage struct {
    TenantID        int64
    CampaignID      int64
    TaskID          int64
    RecipientID     int64
    APILogID        int64
    FromEmail       string
    FromName        string
    Recipient       string
    Subject         string
    RFC822          []byte
    MessageID       string
    SenderDomain    string
    DestinationDomain string
    SendingProfileID int64
    Metadata        map[string]string
}

type OutboundResult struct {
    Engine          string
    MessageID       string
    InjectionStatus string
    ProviderQueueID string
    QueueName       string
    AcceptedAt      int64
    RawResponse     string
}
```

Implementations:

- `PostfixSMTPMailer`: wraps current `mail_service.EmailSender` behavior.
- `KumoHTTPMailer`: sends HTTP injection requests to KumoMTA.

### 11.2 KumoClient

```go
type KumoClient interface {
    Inject(ctx context.Context, req KumoInjectRequest) (*KumoInjectResponse, error)
    Health(ctx context.Context) (*KumoHealth, error)
    Metrics(ctx context.Context) (*KumoMetricsSnapshot, error)
    QueueSummary(ctx context.Context) (*KumoQueueSummary, error)
    PreviewConfig(ctx context.Context, state KumoPolicyState) (*KumoConfigPreview, error)
    DeployConfig(ctx context.Context, version string) (*KumoDeployResult, error)
}
```

Requirements:

- Use a long-lived Go HTTP client with timeouts and keep-alives.
- Keep request timeout small enough to avoid blocking campaign workers.
- Treat HTTP 2xx as accepted/queued, not delivered.
- Treat 429/5xx/network errors as retryable unless policy says otherwise.
- Treat 4xx validation/auth errors as configuration or message failures.
- Log sanitized request/response metadata.
- Never log secrets or full message bodies by default.

### 11.3 KumoEventNormalizer

```go
type KumoEventNormalizer interface {
    Verify(ctx context.Context, request *http.Request, body []byte) error
    Normalize(ctx context.Context, body []byte) ([]NormalizedDeliveryEvent, error)
    StoreIdempotently(ctx context.Context, events []NormalizedDeliveryEvent) (*EventIngestResult, error)
    Apply(ctx context.Context, events []NormalizedDeliveryEvent) error
}
```

Responsibilities:

- Verify HMAC/token.
- Parse single or batched KumoMTA webhook payloads.
- Extract `X-BM-*` correlation headers.
- Deduplicate by Kumo event ID or stable hash.
- Map Kumo event types to BillionMail statuses.
- Update campaign/API records.
- Update suppression and usage counters.

## 12. Kumo HTTP Injection Requirements

### 12.1 Payload Model

The v1 implementation should send one recipient per HTTP injection to preserve personalization, tracking, and deterministic event correlation.

Conceptual request:

```json
{
  "envelope_sender": "bounce+tenant42@example.com",
  "recipients": ["user@gmail.com"],
  "content": "base64-or-raw-rfc822-message-content"
}
```

The exact encoding must follow the KumoMTA HTTP injection schema during implementation.

### 12.2 Required BillionMail Headers

Every KumoMTA-injected message should include:

```text
X-BM-Tenant-ID: 42
X-BM-Campaign-ID: 981
X-BM-Task-ID: 981
X-BM-Recipient-ID: 12345
X-BM-Api-Log-ID: 0
X-BM-Message-ID: <1778600000.abc@example.com>
X-BM-Sending-Profile-ID: 7
X-BM-Engine: kumomta
```

For Send API messages:

```text
X-BM-Tenant-ID: 42
X-BM-Api-ID: 17
X-BM-Api-Log-ID: 77441
X-BM-Message-ID: <1778600000.def@example.com>
X-BM-Sending-Profile-ID: 7
X-BM-Engine: kumomta
```

KumoMTA webhook configuration must return these headers or equivalent metadata in log events.

### 12.3 Queue Mapping

Queue name should be derived as:

```text
{campaign_or_api}:{tenant}@{destination_domain}
```

Examples:

```text
campaign_981:tenant_42@gmail.com
api_17:tenant_42@yahoo.com
```

### 12.4 Deferred Generation

Do not use KumoMTA deferred generation in v1. Consider it only after:

- KumoMTA version behavior is validated.
- Event correlation remains deterministic.
- Template rendering responsibilities are clearly split.
- Operational metrics prove a need for it.

In v1, BillionMail renders every personalized message before injection.

## 13. Message Lifecycle And Status Model

### 13.1 Required States

Add explicit status fields instead of overloading `is_sent`.

Injection status:

```text
pending
rendering
injecting
queued
retrying
failed
cancelled
```

Delivery status:

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

### 13.2 Campaign State Transition

```text
recipient_info row created
      |
      v
injection_status = pending
delivery_status = pending
      |
      v
worker renders message
      |
      v
injection_status = injecting
      |
      +-- Kumo accepted --> injection_status = queued
      |
      +-- retryable error --> injection_status = retrying
      |
      +-- permanent error --> injection_status = failed
      |
      v
Kumo event received
      |
      +-- delivered --> delivery_status = delivered
      +-- deferred  --> delivery_status = deferred
      +-- bounced   --> delivery_status = bounced
      +-- expired   --> delivery_status = expired
      +-- complaint --> delivery_status = complained
```

### 13.3 API Mail State Transition

```text
api_mail_logs row created
      |
      v
status = pending
injection_status = pending
delivery_status = pending
      |
      v
worker injects to KumoMTA
      |
      +-- accepted --> status = queued, injection_status = queued
      +-- failed   --> status = failed or retrying
      |
      v
webhook updates final delivery_status
```

## 14. Database Direction

### 14.1 Tenancy Tables

Add:

```text
tenants
  id
  name
  slug
  status
  plan_id
  created_at
  updated_at

tenant_members
  id
  tenant_id
  account_id
  role
  status
  created_at
  updated_at

tenant_invitations
  id
  tenant_id
  email
  role
  token_hash
  invited_by_account_id
  status
  expires_at
  accepted_at
  created_at
  updated_at

tenant_plans
  id
  name
  daily_send_limit
  monthly_send_limit
  max_domains
  max_contacts
  dedicated_ip_allowed
  created_at
  updated_at

tenant_usage_daily
  id
  tenant_id
  date
  queued_count
  delivered_count
  bounced_count
  complained_count
  api_count
  campaign_count
  created_at
  updated_at

tenant_suppression_list
  id
  tenant_id
  email
  reason
  source
  created_at
  updated_at

tenant_api_keys
  id
  tenant_id
  api_key_hash
  name
  status
  scopes
  last_used_at
  expires_at
  created_at
  updated_at
```

### 14.2 Tenant-Scoped Existing Tables

Add `tenant_id` to all customer-owned tables. At minimum:

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

Migration requirement:

- Existing single-tenant installations should create a default tenant and backfill all existing rows.
- The default tenant should be named from the current installation hostname or `Default Workspace` when no better value exists.
- Existing accounts should become members of the default tenant with an owner/admin role according to current global role.
- Unique constraints must become tenant-aware.
- Example: `UNIQUE(group_id, email)` may remain because `group_id` is tenant-owned, but direct natural-key constraints like `UNIQUE(api_key)` and domain constraints need tenant-aware review.
- Domain ownership should be unique globally by default to prevent two tenants from sending from the same domain without operator approval.
- Tenant-owned names should become unique per tenant where appropriate, for example template name, contact group name, tag name, and sending profile name.

### 14.3 Kumo-Specific Tables

Add:

```text
kumo_nodes
  id
  name
  base_url
  metrics_url
  status
  last_ok_at
  last_error_at
  last_error
  created_at
  updated_at

kumo_egress_sources
  id
  name
  source_address
  ehlo_domain
  node_id
  status
  warmup_status
  created_at
  updated_at

kumo_egress_pools
  id
  name
  description
  status
  created_at
  updated_at

kumo_egress_pool_sources
  id
  pool_id
  source_id
  weight
  status

tenant_sending_profiles
  id
  tenant_id
  name
  default_from_domain_id
  kumo_pool_id
  egress_mode
  egress_provider
  dkim_selector
  daily_quota
  hourly_quota
  warmup_enabled
  status
  created_at
  updated_at

tenant_sending_profile_domains
  id
  profile_id
  domain_id

kumo_message_injections
  id
  tenant_id
  message_id
  recipient
  recipient_domain
  campaign_id
  task_id
  recipient_info_id
  api_id
  api_log_id
  sending_profile_id
  queue_name
  injection_status
  delivery_status
  attempt_count
  next_retry_at
  accepted_at
  final_event_at
  last_error
  created_at
  updated_at

kumo_delivery_events
  id
  tenant_id
  provider_event_id
  event_hash
  event_type
  message_id
  recipient
  queue_name
  campaign_id
  recipient_info_id
  api_log_id
  raw_event
  occurred_at
  ingested_at

kumo_config_versions
  id
  version
  generated_by_account_id
  status
  preview
  deployed_at
  error
  created_at
```

### 14.4 Status Field Additions

For `recipient_info`:

```text
engine
injection_status
delivery_status
kumo_queue
kumo_injection_id or provider_queue_id
last_delivery_event_at
last_delivery_response
attempt_count
next_retry_at
```

For `api_mail_logs`:

```text
engine
tenant_id
injection_status
delivery_status
kumo_queue
kumo_injection_id or provider_queue_id
last_delivery_event_at
last_delivery_response
attempt_count
next_retry_at
```

## 15. KumoMTA Policy Artifact Requirements

BillionMail should generate and version KumoMTA policy artifacts from database state.

Artifacts may include:

- tenant to pool mapping
- egress source definitions
- egress pool definitions
- DKIM domain/selector/key mapping
- traffic class mapping
- webhook/log hook configuration
- headers/log parameters needed for `X-BM-*` correlation

Deployment options:

1. Manual export in v1:
   - Operator previews generated config.
   - Operator applies it on the KumoMTA droplet.
   - Lowest risk for first production rollout.

2. Managed deploy in v2:
   - BillionMail pushes config to the KumoMTA droplet through a controlled deployment channel.
   - Requires validation, rollback, audit logs, and service reload coordination.

The PRD recommendation is manual preview first, managed deploy later.

## 16. Frontend Requirements

### 16.1 New API Module

Add future module:

```text
core/frontend/src/api/modules/kumo.ts
```

It should wrap:

- `GET /api/kumo/config`
- `POST /api/kumo/config`
- `POST /api/kumo/test_connection`
- `GET /api/kumo/status`
- `GET /api/kumo/metrics`
- `GET /api/kumo/pools`
- `POST /api/kumo/pools`
- `POST /api/kumo/config/preview`
- `POST /api/kumo/config/deploy`

### 16.2 Navigation

Recommended v1 placement:

- Add `Settings > KumoMTA` because this starts as operator configuration.
- Add a top-level `Delivery` route later when queue metrics, pool management, and tenant operations become central daily workflows.

Do not overload the existing `SMTP` page. SMTP is currently external SMTP relay/provider oriented. KumoMTA should be presented as the outbound delivery engine.

### 16.3 State Management

Use local component state and API calls for Kumo settings in v1.

Add a `tenant` Pinia store only when tenant switching is implemented. The store should hold:

- current tenant ID
- available tenants
- tenant permissions
- selected tenant sending profile
- tenant plan and quota snapshot
- tenant onboarding/readiness state

The Axios wrapper should add `X-Tenant-ID` only after backend membership validation is complete.

### 16.4 Tenant Switcher And Tenant Settings

The primary tenant control should be a compact workspace switcher in the app header or sidebar header. It should show the active tenant name and role, then open a switcher modal listing only tenants returned by `GET /api/tenants`.

Tenant settings should be separate from platform operator settings:

- Tenant settings: members, invitations, domains, API keys, sending profiles, quotas, suppression, billing/plan summary.
- Operator settings: all tenants, plans, abuse controls, KumoMTA nodes, Kumo pools, generated Kumo policy, global metrics.
- Tenant users should not see platform-wide Kumo pool membership or other tenant names in shared-pool operational views.
- Operator views may show cross-tenant queue health, risk, and pool utilization, but should avoid exposing recipient-level message data unless needed for abuse investigation.

### 16.5 Tenant-Scoped Dashboard Behavior

When a user switches tenant:

- clear tenant-scoped frontend caches for contacts, campaigns, templates, Send API logs, analytics, and sending profiles
- reload permissions before rendering navigation
- reload quota/readiness banners
- restart dashboard polling using the server-validated tenant context
- prevent stale detail pages from showing previous tenant records after a switch

## 17. ASCII Mock Screens

These mockups are conceptual and should be implemented with existing Naive UI patterns.

### 17.1 Settings > KumoMTA

```text
+--------------------------------------------------------------------------------+
| Settings                                                                       |
| Common | Service | BCC | Forward | AI Model | Send Queue | KumoMTA             |
+--------------------------------------------------------------------------------+
| KumoMTA Connection                                      [Test] [Save changes]   |
+--------------------------------------------------------------------------------+
| Status: Connected            Last check: 12:01:22 PM      Inject latency: 34ms  |
| Webhook: Receiving events    Last event: 4s ago           Metrics: Healthy      |
+--------------------------------------------------------------------------------+
| Enabled              [x] Use KumoMTA for outbound campaign/API sending          |
| Campaign sending     [x] Enabled                                                |
| Send API             [x] Enabled                                                |
| Default engine       (o) KumoMTA  ( ) Postfix  ( ) Hybrid                       |
+--------------------------------------------------------------------------------+
| Base URL             https://kumo.example.com:8000                              |
| Injection path       /api/inject/v1                                             |
| Metrics URL          https://kumo.example.com:8000/metrics                      |
| TLS verification     [x] Verify TLS certificate                                 |
| Auth mode            [Bearer token v]                                           |
| Auth secret          **************                    [Rotate]                 |
| Webhook secret       **************                    [Rotate]                 |
| Request timeout      [ 5000 ] ms                                                |
| Default pool         [ shared-default v ]                                       |
+--------------------------------------------------------------------------------+
| Recent checks                                                                  |
| Time                 Check             Result          Latency      Detail       |
| 12:01:22             Inject endpoint   OK              34ms         200          |
| 12:01:22             Metrics endpoint  OK              72ms         snapshot     |
| 11:59:01             Webhook secret    OK              -            verified     |
+--------------------------------------------------------------------------------+
```

### 17.2 KumoMTA Nodes, Metrics, And Queues

```text
+--------------------------------------------------------------------------------+
| Delivery / KumoMTA Dashboard                         [Refresh] [Export]         |
+--------------------------------------------------------------------------------+
| Inject RPS  220 | Delivery RPS 190 | Queued 18,402 | Deferred 812 | Bounce 1.2% |
+--------------------------------------------------------------------------------+
| Nodes                                                                          |
| Name        Status    Inject ms   Metrics ms   Last OK       Last Error         |
| kumo-do-1   Healthy   34          72           12:01:22      -                  |
+--------------------------------------------------------------------------------+
| Queue summary                                                                  |
| Queue                              Ready   Scheduled   Deferred   Oldest        |
| campaign_981:tenant_42@gmail.com   1200    300         80         00:04:12      |
| api_17:tenant_42@yahoo.com          180     20          4          00:01:09      |
+--------------------------------------------------------------------------------+
| Egress pools                                                                    |
| Pool              Sources            Tenants    Status      Utilization          |
| shared-default    do-ip-1,do-ip-2    18         Active      62%                  |
| warmup-a          do-ip-3            4          Warming     28%                  |
+--------------------------------------------------------------------------------+
```

### 17.3 Tenant Sending Profile

```text
+--------------------------------------------------------------------------------+
| Tenant: Acme Marketing                         [Create profile] [Save]          |
+--------------------------------------------------------------------------------+
| Sending profiles                                                               |
| Profile            Domains                 Kumo pool        Quota/day  Status   |
| Marketing default  example.com, mail.io     shared-default   1,000,000  Ready    |
| Warmup             offers.example.com       warmup-a         50,000     Warming  |
+--------------------------------------------------------------------------------+
| Profile details                                                                |
| Name                 Marketing default                                           |
| Default From domain  example.com                                                |
| Sender domains       [example.com] [mail.io]                                    |
| Kumo egress pool     [shared-default v]                                         |
| DKIM selector        bm1                                                        |
| Daily quota          1000000                                                    |
| Hourly quota         50000                                                      |
| Warmup mode          [ ] Enabled                                                |
+--------------------------------------------------------------------------------+
| Domain readiness                                                               |
| Domain           SPF       DKIM      DMARC     rDNS/EHLO     Result             |
| example.com      Pass      Pass      Pass      Pass          Ready              |
| mail.io          Pass      Missing   Pass      Shared        Needs DKIM         |
+--------------------------------------------------------------------------------+
```

### 17.4 Domain DNS/DKIM/SPF Readiness For KumoMTA

```text
+--------------------------------------------------------------------------------+
| Domain: example.com                                      [Recheck DNS]          |
+--------------------------------------------------------------------------------+
| Readiness                                                                         |
| Local mailbox: Ready        Postfix/Rspamd signing: Ready                         |
| KumoMTA outbound: Needs attention                                                 |
+--------------------------------------------------------------------------------+
| Record       Host                    Expected value                 Status        |
| SPF          example.com             v=spf1 include:... -all        Pass          |
| DKIM Kumo    bm1._domainkey          p=MIIB...                      Missing       |
| DMARC        _dmarc                  v=DMARC1; p=none               Pass          |
| PTR/EHLO     sending IP              mail.example.com               Pass          |
+--------------------------------------------------------------------------------+
| KumoMTA signing                                                                  |
| Selector        bm1                                                              |
| Egress pool     shared-default                                                   |
| Key status      Generated in BillionMail, not deployed to KumoMTA                 |
| Action          [Preview Kumo config] [Mark deployed]                             |
+--------------------------------------------------------------------------------+
```

### 17.5 Campaign Create/Edit Sending Profile Section

```text
+--------------------------------------------------------------------------------+
| Campaign: Spring Launch                                                          |
+--------------------------------------------------------------------------------+
| Content | Recipients | Delivery | Schedule | Review                              |
+--------------------------------------------------------------------------------+
| Delivery engine                                                                  |
| (o) Tenant default: KumoMTA                                                       |
| ( ) Local Postfix                                                                |
+--------------------------------------------------------------------------------+
| Sending profile       [Marketing default v]                                      |
| From address          Rahul <news@example.com>                                   |
| Kumo egress pool      shared-default                                             |
| Queue pattern         campaign_981:tenant_42@recipient-domain                    |
| Daily quota left      842,120 of 1,000,000                                       |
| Domain readiness      Ready                                                      |
+--------------------------------------------------------------------------------+
| Safety checks                                                                     |
| [x] Suppression list checked                                                      |
| [x] DKIM ready for KumoMTA                                                        |
| [x] Webhook receiver healthy                                                      |
| [x] Tenant quota available                                                        |
+--------------------------------------------------------------------------------+
```

### 17.6 Campaign Analytics With Kumo Events

```text
+--------------------------------------------------------------------------------+
| Campaign Analytics: Spring Launch                         [Refresh] [Export]     |
+--------------------------------------------------------------------------------+
| Queued 98,400 | Delivered 91,220 | Deferred 4,810 | Bounced 2,110 | Complaints 8 |
+--------------------------------------------------------------------------------+
| Delivery timeline                                                                |
| 12:00  queued ########################                                           |
| 12:10  delivered ###################                                             |
| 12:20  deferred ####                                                           |
+--------------------------------------------------------------------------------+
| Provider performance                                                             |
| Provider     Queued   Delivered   Deferred   Bounced   Avg delay   Pool          |
| Gmail        42,000   39,900      1,400      690       00:02:12    shared-default|
| Yahoo        18,200   16,700      900        560       00:04:40    shared-default|
+--------------------------------------------------------------------------------+
| Event stream                                                                     |
| Time       Recipient             Event       Response                  Queue      |
| 12:21:01   u1@gmail.com          delivered   250 2.0.0 OK              campaign.. |
| 12:21:04   u2@yahoo.com          deferred    421 rate limited          campaign.. |
+--------------------------------------------------------------------------------+
```

### 17.7 Send API Detail Page

```text
+--------------------------------------------------------------------------------+
| Send API: Password Reset                                      [Test] [Edit]      |
+--------------------------------------------------------------------------------+
| Delivery engine       KumoMTA                                                     |
| Sending profile       Transactional default                                       |
| Kumo pool             transactional-dedicated                                     |
| Webhook health        Healthy, last event 3s ago                                  |
| Queue pattern         api_17:tenant_42@recipient-domain                           |
+--------------------------------------------------------------------------------+
| Recent API sends                                                                  |
| Time       Recipient          Injection      Delivery     Message ID              |
| 12:01:11   user@gmail.com     queued         delivered    <...@example.com>       |
| 12:01:09   user@yahoo.com     queued         deferred     <...@example.com>       |
| 12:00:58   user@corp.com      retrying       pending      <...@example.com>       |
+--------------------------------------------------------------------------------+
| Webhook diagnostics                                                              |
| Last accepted event: 12:01:14                                                     |
| Duplicates ignored: 128                                                           |
| Signature failures: 0                                                             |
+--------------------------------------------------------------------------------+
```

## 18. Caching, Latency, And Backpressure

### 18.1 Configuration Cache

KumoMTA connection config and tenant sending profiles should be cached in-process for a short TTL.

Requirements:

- TTL: 30 to 120 seconds depending on implementation.
- Invalidate immediately on admin update.
- Never cache raw secrets in logs.
- If cache load fails, fail closed for Kumo sending unless an explicit fallback policy exists.

### 18.2 Metrics Cache

Poll KumoMTA metrics in a background job.

Requirements:

- UI reads cached snapshots from BillionMail.
- Metrics endpoint failure must not block campaign/API sending.
- Store last successful snapshot time and last error.
- Retain short history for charts.

### 18.3 Redis Usage

Use Redis for:

- webhook event idempotency fast path
- per-tenant quota counters
- short-lived injection backpressure counters
- distributed locks for workers
- rate-limiting public API requests

### 18.4 Latency Targets

Suggested targets:

- Kumo config read from cache: under 10ms.
- Kumo injection request: timeout 5s default, configurable.
- Webhook verification and enqueue/store: under 250ms for normal single/batch payloads.
- Metrics UI load from cached snapshot: under 500ms.

## 19. Reliability And Failure Handling

### 19.1 KumoMTA Unavailable

If KumoMTA is unavailable:

- Do not mark messages delivered.
- Keep campaign/API messages pending or retrying.
- Use exponential backoff.
- Surface operator alert.
- Apply circuit breaker per tenant/profile if failures continue.
- Optional fallback to Postfix should require explicit operator configuration and should be disabled by default for high-volume campaigns.

### 19.2 Injection Errors

```text
HTTP 2xx: queued/accepted
HTTP 400/422: permanent message/config validation error
HTTP 401/403: Kumo auth/config error, pause Kumo sending
HTTP 429: retryable backpressure
HTTP 5xx: retryable service failure
network timeout: retryable service failure
```

### 19.3 Webhook Errors

- Invalid signature: reject and count security failure.
- Duplicate event: return success but ignore duplicate mutation.
- Unknown message: store event as orphaned and expose diagnostics.
- Out-of-order event: apply state transition rules and keep event history.
- Partial batch failure: accept valid events and report failures.

### 19.4 Backpressure

BillionMail should avoid overwhelming KumoMTA:

- Limit concurrent Kumo injection workers.
- Apply per-tenant queue limits.
- Apply per-profile injection rate limits.
- Pause tenants exceeding abuse thresholds.
- Keep KumoMTA traffic shaping responsible for destination-provider behavior.

## 20. Security Requirements

### 20.1 KumoMTA Endpoint Security

- Prefer private network connectivity between BillionMail and KumoMTA.
- Use TLS when crossing public networks.
- Use bearer token or HMAC auth for injection.
- Use HMAC or token auth for webhook receiver.
- Rotate secrets from the UI.
- Store secrets encrypted or protected according to the chosen secret storage pattern.
- Never print secrets in logs.

### 20.2 Tenant Isolation

- Resolve tenant server-side from session or API key.
- UI-sent `X-Tenant-ID` is only a selection hint and must be verified.
- Every business query must include tenant scope.
- Add tenant ownership checks before domain, mailbox, contact, campaign, template, API key, and analytics reads/writes.
- Add tests for cross-tenant access denial.

### 20.3 API Key Behavior

- API keys belong to exactly one tenant.
- Store API key hashes, not plaintext keys.
- Restrict API key scopes.
- Apply tenant quotas server-side.
- IP whitelist remains tenant/API-key scoped.

### 20.4 Abuse Controls

Before commercial launch:

- tenant approval state
- domain verification state
- bounce/complaint thresholds
- suppression enforcement
- per-tenant and per-profile quotas
- operator kill switch per tenant
- audit logs for sending profile and Kumo config changes

## 21. Observability Requirements

### 21.1 Logs

Log structured events for:

- Kumo config changes
- Kumo health check failures
- injection attempts and sanitized failures
- webhook verification failures
- webhook processing failures
- orphaned delivery events
- quota rejections
- tenant/profile pause/resume

### 21.2 Metrics

Track:

- injection attempts by tenant/profile/engine
- injection success/failure/latency
- webhook events by type
- event ingestion lag
- queued messages by queue/domain/tenant
- deferred/bounced/complained counts
- Kumo node health
- Redis idempotency hit rate

### 21.3 Alerts

Operator alerts:

- Kumo unreachable
- webhook no events for active traffic
- signature failures spike
- bounce/complaint threshold exceeded
- queue age exceeds threshold
- tenant quota exceeded
- config deploy failed

## 22. Rollout Plan

### Phase 0: Manual Operational Verification

- Confirm BillionMail server can reach deployed KumoMTA injection endpoint.
- Confirm the DigitalOcean SMTP block status and choose an allowed production egress path before any direct-to-MX launch.
- Confirm DNS, rDNS, SPF, DKIM, and DMARC requirements for at least one sending domain or subdomain, including `email.gitdate.ink` selector `s1` if that subdomain is used.
- Confirm webhook URL is reachable from KumoMTA.

### Phase 1: KumoMTA Settings And Health

- Add config storage.
- Add settings UI.
- Add health checks.
- Add metrics polling cache.
- Add webhook receiver with signature verification.

### Phase 2: Backend Mailer Abstraction

- Introduce `OutboundMailer`.
- Keep `PostfixSMTPMailer` as current behavior.
- Add `KumoHTTPMailer`.
- Add engine selection logic by workflow and tenant/profile.

### Phase 3: Campaign KumoMTA Injection

- Add campaign sending profile selection.
- Inject campaign messages into KumoMTA.
- Store injection state separately from delivery state.
- Keep existing open/click tracking.
- Update campaign analytics from Kumo events.

### Phase 4: Send API KumoMTA Injection

- Resolve API keys to tenant.
- Queue API mail with Kumo injection state.
- Return queued state from API.
- Update API logs from webhooks.
- Show Kumo state in Send API UI.

### Phase 5: Multi-Tenant Hardening

- Add tenant tables.
- Backfill default tenant for existing installs.
- Add `tenant_id` to customer-owned tables.
- Update all business queries.
- Add tenant membership checks.
- Add tenant-aware frontend selection.

### Phase 6: KumoMTA Policy Management

- Generate tenant/pool/source/DKIM policy artifacts.
- Add preview UI.
- Add config versioning.
- Add managed deployment only after manual workflow is stable.

### Phase 7: Scale And Abuse Automation

- Add per-tenant quotas and billing usage.
- Add bounce/complaint automation.
- Add backpressure automation.
- Add Kumo cluster support.
- Add dedicated IP and warmup product flows.

## 23. Test Plan

### 23.1 Unit Tests

- Kumo injection payload generation.
- Kumo response parsing.
- HTTP status to retry/permanent error mapping.
- Message-ID preservation.
- Required `X-BM-*` header generation.
- HMAC/token webhook verification.
- Event normalization.
- Event idempotency.
- Status transition rules.
- Tenant membership enforcement.
- API key to tenant resolution.
- Active tenant context validation from session/JWT and `X-Tenant-ID`.
- Tenant-scoped unique constraint behavior for domains, templates, contacts, API keys, and sending profiles.
- Default tenant backfill mapping for existing single-tenant installs.
- Tenant role permission checks for owner, admin, marketer, developer, and operator.
- Sending profile selection.

### 23.2 Integration Tests

- Campaign send path injects into mocked KumoMTA.
- API send path returns queued state and later updates from webhook.
- Kumo unavailable keeps messages pending/retryable.
- Kumo auth failure pauses Kumo sending and surfaces operator error.
- Metrics endpoint failures do not block sending.
- Webhook duplicate event does not double count analytics.
- Webhook orphaned event is stored for diagnostics.
- Postfix/local sending still works when selected.
- Tenant A cannot query, update, delete, send from, or view analytics for Tenant B records through UI APIs.
- Multi-tenant account can switch from Tenant A to Tenant B only when membership is active.
- Public Send API key resolves the correct tenant even if a caller sends a forged tenant header.
- Tenant quota exhaustion blocks only that tenant while other tenants on the same Kumo pool keep sending.
- Operator suspension prevents new Kumo injection for the suspended tenant and leaves unrelated tenants unaffected.
- DNS validation confirms `mail.gitdate.ink` resolves to `159.89.33.85` before saving `https://mail.gitdate.ink` as the reverse proxy domain.
- DNS validation confirms `email.gitdate.ink` resolves to `192.241.130.241` before using it as the KumoMTA control/injection domain.
- DNS validation confirms `email.gitdate.ink` has SPF, DKIM selector `s1`, and `_dmarc.email.gitdate.ink` records before using that subdomain for Kumo-signed test mail.
- KumoMTA webhook delivery to `https://mail.gitdate.ink/api/kumo/events` succeeds with webhook auth enabled.
- KumoMTA injection endpoint is reachable through the selected private VPC path or authenticated public HTTPS path.
- Direct-to-MX delivery from the DigitalOcean KumoMTA droplet is marked blocked/not production-ready until an allowed external egress path is configured.
- External egress mode can deliver through KumoProxy, external Kumo node, provider HTTP API, or provider submission port and return events to BillionMail.
- SPF/DKIM/DMARC readiness points to the real sending egress source, not just the Kumo control droplet.

### 23.3 Frontend Tests

- `kumo.ts` API module calls correct endpoints.
- Kumo settings screen renders disconnected state.
- Kumo settings screen renders connected state.
- Kumo settings screen renders error state.
- Test connection action handles success and failure.
- Campaign create/edit shows delivery engine and sending profile.
- Send API detail shows Kumo delivery engine and webhook health.
- Dashboard tables render queue and pool data.
- Tenant switcher renders only available workspaces and clears tenant-scoped state after switch.
- Tenant onboarding checklist renders blocked, partial, and ready states.
- Tenant team screen renders owner/admin/marketer/developer role differences.
- Tenant quota and abuse-block banners render on campaign and Send API screens.
- Operator tenant-risk view shows cross-tenant operational metrics without exposing another tenant's recipient list to tenant users.

### 23.4 Acceptance Scenarios

1. Existing Postfix/local sending still works.
2. Operator configures KumoMTA endpoint and sees healthy status.
3. Kumo-enabled campaign queues mail through KumoMTA.
4. Kumo webhook delivered event updates campaign recipient delivery status.
5. Kumo webhook bounce event updates analytics and suppression logic.
6. Send API returns queued state for KumoMTA messages.
7. Send API detail later shows delivered/bounced from webhook event.
8. Tenant A cannot access Tenant B contacts, domains, campaigns, API logs, or analytics after tenancy is introduced.
9. Queue and event metrics show useful operational state.
10. Kumo metrics failure does not prevent message injection.
11. Operator can create a B2B tenant, assign plan/quota, and assign a Kumo pool.
12. Tenant owner can complete onboarding from domain setup to sending profile readiness.
13. Tenant admin can invite members and role permissions apply inside that tenant only.
14. A user who belongs to multiple tenants can switch workspace context without leaking stale data.
15. Tenant developer can create a tenant API key and see Kumo queued/delivered states for that tenant only.
16. Quota or abuse blocking pauses only the affected tenant and does not stop unrelated tenants on the same pool.
17. GoDaddy DNS has `mail.gitdate.ink -> 159.89.33.85`, `email.gitdate.ink -> 192.241.130.241`, Microsoft 365 MX intact, and SPF/DKIM/DMARC records for `email.gitdate.ink`.
18. BillionMail reverse proxy health passes at `https://mail.gitdate.ink/api/languages/get`.
19. KumoMTA webhook receiver is reachable at `https://mail.gitdate.ink/api/kumo/events` with valid webhook authentication.
20. Kumo HTTP injection is reachable from BillionMail through VPC peering/private routing or public HTTPS with source allowlisting.
21. Direct-to-MX sending from the DigitalOcean KumoMTA droplet is treated as blocked by policy and is not marked production-ready.
22. A configured external egress path can deliver mail and return delivery/bounce events to BillionMail.
23. Tenant DNS readiness validates SPF/DKIM/DMARC/PTR/EHLO for the real egress IP or provider, not only `192.241.130.241`.

## 24. Key Product Decisions

### 24.1 Verdict: External KumoMTA

Use the already deployed KumoMTA droplet as an external outbound MTA service. Later, scale it into a KumoMTA cluster.

Do not embed KumoMTA source code into BillionMail because:

- It is a separate Rust/Lua MTA runtime.
- It has its own spool, queue, policy, metrics, and operational lifecycle.
- Embedding would make upgrades and security patches harder.
- It would couple web app deploys to delivery-plane risk.
- It does not improve tenant isolation or deliverability.

### 24.2 Verdict: HTTP Injection First

Use `/api/inject/v1` HTTP injection as the native integration path.

SMTP relay is acceptable for:

- smoke testing
- compatibility mode
- transitional fallback

But HTTP injection is the production path because BillionMail owns rendering and metadata.

### 24.3 Verdict: Manual Config Preview Before Managed Deploy

Start with generated config preview and manual KumoMTA deployment. Add managed deploy only after:

- config generation is stable
- validation is strong
- rollback is tested
- operator audit logging exists

## 25. Risks And Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Tenant isolation incomplete | Data leak, commercial blocker | Add tenant IDs, query guards, tests, default tenant backfill |
| KumoMTA unavailable | Sending stalls | Retry/backoff, circuit breakers, operator alerts |
| Webhook events missing | Analytics wrong | Webhook health, lag alerts, orphan diagnostics, metrics comparison |
| Duplicate webhook events | Double counting | Idempotency by event ID/hash in Redis and DB |
| Kumo accepted treated as delivered | False analytics | Separate injection and delivery status |
| DKIM split-brain | Authentication failures | Clear domain readiness and source of signing truth |
| DO SMTP limitations | Kumo cannot deliver direct-to-MX | Treat DO-hosted Kumo as control/queue node only and require external allowed egress for production |
| Overwhelming KumoMTA | Queue growth/failures | Backpressure, concurrency limits, per-tenant quotas |
| Managed config deploy breaks Kumo | Delivery outage | Start manual, add preview, validation, rollback |

## 26. Open Questions

1. What exact KumoMTA auth mode is configured on the deployed droplet today?
2. Is the KumoMTA droplet reachable over private networking, public TLS, or both?
3. Which external egress strategy will production use for outbound delivery: KumoProxy, external Kumo node, provider HTTP API, or provider SMTP on a non-blocked port?
4. What KumoMTA version is deployed?
5. What webhook payload shape is currently configured or preferred?
6. Should KumoMTA sign all outbound campaign/API mail, or should some flows remain Rspamd/Postfix signed?
7. Will tenants have shared IP pools, dedicated IP pools, or both at launch?
8. What billing plan/quota model should drive per-tenant sending limits?
9. Should current single-tenant installs be migrated automatically to a default tenant?
10. What is the first target workflow: campaigns, Send API, or both?

## 27. Source Links

Official KumoMTA documentation:

- KumoMTA docs home: https://docs.kumomta.com/
- HTTP injection: https://docs.kumomta.com/userguide/operation/httpinjection/
- HTTP injection schema: https://docs.kumomta.com/reference/http/kumod/schemas/InjectV1Request/
- Queues, sources, and pools: https://docs.kumomta.com/reference/queues/
- Sending IPs: https://docs.kumomta.com/userguide/configuration/sendingips/
- Traffic shaping: https://docs.kumomta.com/userguide/configuration/trafficshaping/
- DKIM signing: https://docs.kumomta.com/userguide/configuration/dkim/
- Webhooks: https://docs.kumomta.com/userguide/operation/webhooks/
- Metrics: https://docs.kumomta.com/reference/http/metrics/
- Routing via proxy/KumoProxy: https://docs.kumomta.com/userguide/operation/proxy/
- Routing via HTTP provider API: https://docs.kumomta.com/userguide/policy/http/
- Mautic integration: https://docs.kumomta.com/userguide/integrations/mautic/
- EmailElement integration: https://docs.kumomta.com/userguide/integrations/emailelement/
- Ongage integration: https://docs.kumomta.com/userguide/integrations/ongage/

DNS and infrastructure references:

- GoDaddy DNS record management: https://www.godaddy.com/help/manage-dns-records-680
- DigitalOcean SMTP block: https://docs.digitalocean.com/support/why-is-smtp-blocked/
- DigitalOcean VPC cross-region peering limits: https://docs.digitalocean.com/products/networking/vpc/details/limits/

Local planning source:

- `KumoMTA_Billion.md`
- `docs/REVERSE_PROXY.md`

Codebase areas referenced:

- `docker-compose.yml`
- `core/internal/service/mail_service/sending.go`
- `core/internal/service/batch_mail/task_executor.go`
- `core/internal/service/batch_mail/api_mail_send.go`
- `core/internal/controller/batch_mail/batch_mail_v1_api_mail_send.go`
- `core/internal/service/database_initialization/batch_mail.go`
- `core/internal/service/database_initialization/mail_serv.go`
- `core/internal/service/database_initialization/rbac.go`
- `core/internal/service/database_initialization/options.go`
- `core/internal/service/maillog_stat/`
- `core/frontend/src/router/router.ts`
- `core/frontend/src/router/modules/settings.ts`
- `core/frontend/src/api/index.ts`
- `core/frontend/src/api/modules/`
- `core/frontend/src/views/settings/send-queue/index.vue`
- `core/frontend/src/views/smtp/index.vue`
- `core/frontend/src/views/api/index.vue`
- `core/frontend/src/views/market/task/`
