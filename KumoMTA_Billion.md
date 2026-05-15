# KumoMTA + BillionMail Multi-Tenant Architecture

Date: 2026-05-12

Status: Planning and architecture report only. This document does not implement any code, Docker, schema, deployment, or configuration changes.

## Executive Verdict

Use KumoMTA as a separate outbound MTA service for production. Since you already have KumoMTA running on a DigitalOcean droplet, the best next step is to integrate BillionMail with that KumoMTA node over a private, authenticated HTTP injection path, then grow it into a dedicated KumoMTA cluster as tenant volume increases.

Do not integrate KumoMTA source code into the BillionMail codebase. KumoMTA is its own Rust/Lua MTA runtime, queueing system, policy engine, spool system, and operational surface. Vendoring or embedding it into the Go/React BillionMail app would make upgrades, security patches, deployment, and incident isolation harder without giving a real product benefit.

Do not run production KumoMTA inside the same BillionMail Docker Compose stack that currently runs Postgres, Redis, Postfix, Dovecot, Rspamd, Roundcube, and the core app. That is acceptable only for local development, demos, or staging. For a product you will sell to multiple tenants, outbound delivery needs its own data plane, disk/spool isolation, IP reputation boundary, and monitoring.

## Current BillionMail Architecture

Based on the current repository, BillionMail is built as a self-hosted mail and campaign platform:

- `docker-compose.yml` runs Postgres, Redis, Rspamd, Dovecot, Postfix, Roundcube webmail, and `core-billionmail`.
- Postfix exposes SMTP/SMTPS/submission ports and is the current outbound MTA.
- Dovecot and Roundcube support mailbox access and webmail.
- The Go core service manages domains, mailboxes, contacts, templates, campaigns, API sending, tracking, and background timers.
- Campaign sending and API sending currently render mail in the core service and send it through Postfix using SMTP.
- Mail analytics are heavily tied to Postfix log parsing and Postfix message IDs.

Current simplified architecture:

```text
                         +----------------------+
                         | Browser / API Client |
                         +----------+-----------+
                                    |
                                    v
                         +----------------------+
                         | BillionMail Core     |
                         | UI/API/jobs/tracking |
                         +----+------------+----+
                              |            |
                              |            |
                    +---------v---+    +---v---------+
                    | PostgreSQL  |    | Redis       |
                    +-------------+    +-------------+
                              |
                              |
                              v
                         +----------------------+
                         | Postfix              |
                         | current outbound MTA |
                         +----------+-----------+
                                    |
                                    v
                         +----------------------+
                         | Recipient MX servers |
                         +----------------------+

                +----------------+       +----------------+
                | Dovecot IMAP   |<----->| Roundcube      |
                | mailbox access |       | webmail        |
                +----------------+       +----------------+
```

## Why BillionMail Is Currently Single-Tenant

BillionMail has user accounts and RBAC, but the product data model is not tenant-scoped yet. This means it is multi-user, not truly multi-tenant.

Evidence from the current codebase:

- `core/internal/service/database_initialization/rbac.go` creates `account`, `role`, `permission`, `account_role`, and `role_permission`.
- Business tables do not carry a `tenant_id` or `workspace_id`.
- Global mail-service tables include `domain`, `mailbox`, `alias`, `bm_bcc`, `bm_relay`, and `alias_domain`.
- Global marketing/API tables include `bm_contact_groups`, `bm_contacts`, `email_templates`, `email_tasks`, `recipient_info`, `unsubscribe_records`, `api_templates`, `api_mail_logs`, `api_ip_whitelist`, `bm_tags`, and `bm_contact_tags`.
- Mail statistics tables are global and Postfix-oriented: `mailstat_message_ids`, `mailstat_senders`, `mailstat_send_mails`, `mailstat_deferred_mails`, `mailstat_opened`, `mailstat_clicked`, and `mailstat_complaints`.
- Campaign sender code processes `email_tasks` globally and sends through `mail_service.NewEmailSenderWithLocal(...)`.
- API sender code processes `api_mail_logs` globally and groups by sender address, not tenant.

The result is that one deployment has one shared pool of domains, mailboxes, contacts, templates, campaigns, API keys, unsubscribe records, and analytics. That is not safe for a SaaS product where different customers configure their own sender domains, contacts, API keys, and sending limits.

## Target Product Model

The commercial product should separate responsibilities:

- BillionMail is the control plane.
- KumoMTA is the outbound delivery data plane.
- Postfix/Dovecot/Roundcube can remain for inbound mailbox and webmail functions unless you later decide to replace them.

Control plane responsibilities:

- Tenants, users, roles, invitations, and permissions.
- Tenant domains, DNS checks, DKIM/SPF/DMARC setup guidance.
- Tenant contacts, groups, tags, templates, campaigns, API keys, and suppression lists.
- Billing plans, quotas, abuse controls, and per-tenant limits.
- Message rendering, personalization, unsubscribe links, open/click tracking links.
- Injection into KumoMTA.
- Delivery event ingestion from KumoMTA.
- Analytics, suppression, bounce handling, complaint handling, and reporting.

Data plane responsibilities:

- Accept rendered messages from BillionMail.
- Queue mail by tenant, campaign, and destination domain.
- Select egress pools and sending IPs.
- Apply traffic shaping by destination provider, tenant, campaign, and source IP.
- Retry transient failures.
- Expire old messages.
- DKIM sign outgoing mail.
- Emit delivery, bounce, transient failure, expiration, and complaint events.

Target high-level architecture:

```text
                           CONTROL PLANE

 +-------------------+     +----------------------------+
 | Tenant User       |     | Admin / Operator           |
 | browser / API     |     | monitoring / abuse review  |
 +---------+---------+     +-------------+--------------+
           |                             |
           v                             v
 +------------------------------------------------------+
 | BillionMail UI/API/Core                              |
 | - tenant auth and RBAC                               |
 | - domains, contacts, templates, campaigns            |
 | - quotas, billing, suppression, tracking             |
 | - message rendering and personalization              |
 +------------+---------------------+-------------------+
              |                     |
              v                     v
 +-------------------------+   +-------------------------+
 | PostgreSQL              |   | Redis                   |
 | tenant-scoped data      |   | jobs, locks, counters   |
 +-------------------------+   +-------------------------+
              |
              | rendered RFC 5322 message + tenant metadata
              v

                            DATA PLANE

 +------------------------------------------------------+
 | KumoMTA HTTP Injection Endpoint                      |
 | private network or TLS + token/HMAC auth             |
 +-------------------------+----------------------------+
                           |
                           v
 +------------------------------------------------------+
 | KumoMTA Scheduled Queues                             |
 | queue = campaign:tenant@destination-domain           |
 +-------------------------+----------------------------+
                           |
                           v
 +--------------------+    +-----------------------------+
 | Egress Pool        |--->| Egress Source IP / EHLO     |
 | tenant mapped      |    | DKIM signing / shaping      |
 +--------------------+    +--------------+--------------+
                                           |
                                           v
                                +------------------------+
                                | Recipient MX servers   |
                                +------------------------+
                                           |
                                           v
 +------------------------------------------------------+
 | KumoMTA log hooks / webhooks                         |
 | delivery, bounce, deferral, expiration, complaint     |
 +-------------------------+----------------------------+
                           |
                           v
 +------------------------------------------------------+
 | BillionMail Event Receiver                           |
 | analytics, status updates, suppression, billing usage |
 +------------------------------------------------------+
```

## KumoMTA Capabilities Relevant To This Product

KumoMTA is a strong fit because it is designed as a high-performance outbound MTA. Its public GitHub repository describes it as an open-source MTA for high-performance outbound sending, similar in class to enterprise MTAs such as Momentum, PowerMTA, and Halon.

The most important KumoMTA features for BillionMail are:

- HTTP injection: BillionMail can submit generated messages to KumoMTA through an HTTP listener instead of pretending KumoMTA is just another SMTP relay.
- Queue metadata: KumoMTA can form queue names from `campaign`, `tenant`, and destination domain, such as `campaign:tenant@domain`.
- Egress sources: sending IPs can be represented as named sources with `source_address` and `ehlo_domain`.
- Egress pools: one or more egress sources can be grouped into a pool, including weighted pools for warmup.
- Queue config policy: KumoMTA policy can map each tenant to a pool.
- Traffic shaping: KumoMTA supports connection limits, message rate limits, messages per connection, retry behavior, TLS options, and provider-specific shaping rules.
- Traffic Shaping Automation: KumoMTA can adjust shaping behavior based on recipient provider responses.
- DKIM signing: KumoMTA has a DKIM signing helper and can sign mail during SMTP or HTTP-generated message events.
- Webhooks/log hooks: KumoMTA can publish log events to an HTTP endpoint and queue those webhook events durably.
- Spooling: KumoMTA has a dedicated spool model and recommends separate storage for performance-sensitive deployments.
- Docker packaging: KumoMTA publishes official images to GitHub Container Registry, which is useful for running KumoMTA as its own service.

## Recommended Integration Pattern

### 1. Keep BillionMail as the source of truth

BillionMail should own:

- tenant records
- users and roles
- domains and DNS verification status
- DKIM key metadata
- sending profiles
- API keys
- contacts and lists
- templates and campaigns
- unsubscribe and suppression records
- daily/monthly sending quotas
- billing state
- audit logs

KumoMTA should not become the customer database. It should receive enough metadata to route, sign, queue, and report on messages.

### 2. Add a real tenant boundary before selling this as SaaS

Minimum tenant tables:

```text
tenants
tenant_members
tenant_roles or tenant_member_roles
tenant_plans
tenant_usage_daily
tenant_api_keys
tenant_sending_profiles
tenant_egress_pools
tenant_suppression_list
```

Existing tables that need tenant scoping:

```text
domain
mailbox
alias
alias_domain
bm_bcc
bm_relay
bm_relay_config
bm_relay_domain_mapping
bm_domain_smtp_transport
bm_multi_ip_domain
bm_sender_ip_warmup
bm_sender_ip_mail_provider
bm_campaign_warmup
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
mailstat_* tables or replacement mail event tables
bm_operation_logs
bm_options where the option is tenant-specific
```

Examples of tenant-scoped uniqueness:

```text
domain:              unique (tenant_id, domain)
mailbox:             unique (tenant_id, username)
contact group:       unique (tenant_id, name)
contact:             unique (tenant_id, group_id, email)
email template:      unique (tenant_id, temp_name)
api key:             globally unique api_key, also indexed by tenant_id
unsubscribe record:  indexed by tenant_id, email, group_id, task_id
```

Every request should resolve an active tenant context:

```text
Session login:
  account_id -> selected tenant_id -> permission check -> tenant-scoped queries

API send:
  api_key -> tenant_id -> template -> sender domain -> quota check -> Kumo injection
```

### 3. Introduce an outbound sender abstraction

Today the campaign/API code directly uses the SMTP sender path that connects to Postfix. The future design should introduce an outbound sender abstraction:

```text
OutboundMailer
  |
  +-- PostfixSMTPMailer
  |     - current behavior
  |     - useful for system messages, local mailbox tests, and fallback
  |
  +-- KumoHTTPMailer
        - production campaign/API outbound
        - injects rendered RFC 5322 messages into KumoMTA
        - sends tenant/campaign/recipient metadata
```

This lets BillionMail keep current Postfix flows where useful while moving high-volume outbound to KumoMTA.

### 4. Use HTTP injection for campaign/API sending

HTTP injection is preferred over SMTP injection for this product because:

- BillionMail can pass structured metadata cleanly.
- It avoids per-message SMTP authentication overhead between core and MTA.
- It gives a clear service-to-service API boundary.
- It is easier to secure with private networking, TLS, allowlists, and shared tokens/HMAC.
- It maps better to queued SaaS workloads than connecting as a mailbox user to Postfix.

Each injected message should include metadata equivalent to:

```text
tenant_id
tenant_slug
campaign_id or api_template_id
email_task_id
recipient_info_id or api_mail_log_id
sender_domain
sender_email
message_id
track_open
track_click
unsubscribe_enabled
egress_pool_hint
plan_or_quota_class
```

Recommended custom headers for troubleshooting and event correlation:

```text
X-BM-Tenant-ID: <opaque tenant uuid>
X-BM-Campaign-ID: <campaign id or uuid>
X-BM-Task-ID: <task id or uuid>
X-BM-Recipient-ID: <recipient id or uuid>
X-BM-Api-Log-ID: <api log id or uuid>
X-BM-Message-ID: <internal message id>
```

Use opaque UUIDs for externally visible values where possible. Do not expose sequential tenant IDs in public tracking links, unsubscribe links, or headers if that creates enumeration risk.

### 5. Let KumoMTA own queueing and delivery attempts

After KumoMTA accepts a message, BillionMail should treat the message as "queued" or "accepted by MTA", not "delivered".

Final state should come from KumoMTA events:

```text
BillionMail status       Source
-------------------      -------------------------------
pending                  BillionMail queue before injection
queued                   KumoMTA accepted injection
delivered                KumoMTA delivery event
deferred                 KumoMTA transient failure event
bounced                  KumoMTA bounce/permanent failure event
expired                  KumoMTA queue expiration event
complained               feedback loop / complaint event
suppressed               BillionMail suppression policy
```

### 6. Use KumoMTA webhooks for analytics

KumoMTA webhooks/log hooks should post delivery events back to BillionMail:

```text
KumoMTA event
  |
  v
POST /internal/kumo/events
  |
  v
Verify HMAC/token/source IP
  |
  v
Normalize event
  |
  +--> update recipient_info / api_mail_logs
  +--> insert mail_events
  +--> update campaign aggregate counters
  +--> update tenant usage counters
  +--> update suppression list on hard bounce/complaint
```

The event receiver must be idempotent. KumoMTA can retry webhooks, and network failures can cause duplicate delivery. Use a unique event ID or a compound uniqueness key from Kumo event fields plus BillionMail message correlation fields.

## Multi-Tenant Egress Model

KumoMTA should be configured around tenants, pools, and sources.

Example model:

```text
Tenant
  id: tenant-a
  plan: enterprise
  domains:
    - example-a.com
  sending profile:
    default_pool: dedicated-a
    daily_quota: 1000000
    hourly_quota: 60000

Tenant
  id: tenant-b
  plan: starter
  domains:
    - example-b.com
  sending profile:
    default_pool: shared-warmup
    daily_quota: 100000
    hourly_quota: 5000
```

Egress mapping:

```text
                         +--------------------------+
                         | BillionMail tenants      |
                         +------------+-------------+
                                      |
                                      v
                         +--------------------------+
                         | tenant_egress_pools     |
                         +------------+-------------+
                                      |
          +---------------------------+---------------------------+
          |                           |                           |
          v                           v                           v
 +------------------+        +------------------+        +------------------+
 | dedicated-a      |        | shared-warmup    |        | dedicated-c      |
 | tenant A only    |        | many small users |        | tenant C only    |
 +--------+---------+        +--------+---------+        +--------+---------+
          |                           |                           |
          v                           v                           v
 +------------------+        +------------------+        +------------------+
 | ip-a-1           |        | ip-shared-1      |        | ip-c-1           |
 | ip-a-2           |        | ip-shared-2      |        | ip-c-2           |
 +------------------+        +------------------+        | ip-c-3           |
                                                       +------------------+
```

Queue naming:

```text
campaign-101:tenant-a@gmail.com
campaign-101:tenant-a@yahoo.com
campaign-204:tenant-b@outlook.com
api:tenant-c@example.net
```

This matters because the operational unit is not just "send 1 million emails per day". The real operational unit is:

```text
tenant + campaign + destination provider + egress source + time window
```

Gmail, Yahoo, Outlook, corporate domains, and regional mailbox providers should each have independent shaping behavior.

## 1 Million Emails Per Day Per User

One million emails per day sounds like a single number, but delivery systems care about rate, reputation, provider mix, and bursts.

Basic math:

```text
1,000,000 / 24 hours = 41,666 emails/hour
1,000,000 / 86,400 seconds = 11.57 emails/second
```

Average rate is not the hard part. The hard parts are:

- cold sender domain reputation
- cold IP reputation
- uneven campaign bursts
- Gmail/Yahoo/Outlook provider limits
- spam complaints
- bounce rate
- unsubscribe rate
- content quality
- DNS alignment
- DKIM/SPF/DMARC correctness
- per-tenant abuse
- noisy-neighbor impact in shared pools

No new tenant should be allowed to send 1 million emails per day immediately on a cold domain or cold IP. The product needs:

- domain verification
- SPF/DKIM/DMARC validation
- bounce/complaint thresholds
- warmup schedules
- tenant-specific daily/hourly quotas
- provider-specific shaping
- suppression enforcement
- abuse review and suspension controls
- dedicated IP pools for high-volume tenants

Recommended policy:

```text
Starter tenant:
  shared pool
  strict daily cap
  strict provider shaping
  no immediate 1M/day access

Growth tenant:
  shared or semi-dedicated pool
  increased cap after reputation checks
  warmup required

Enterprise/high-volume tenant:
  dedicated domain review
  dedicated IP pool
  warmup plan
  signed sending agreement
  separate complaint/bounce thresholds
  possible isolated KumoMTA node if volume is large enough
```

## Deployment Options

### Option A: Separate KumoMTA DigitalOcean droplet

This is the recommended near-term production design since KumoMTA is already deployed and running.

Architecture:

```text
 +------------------------------+
 | BillionMail app droplet      |
 |                              |
 | core / UI / Postgres / Redis |
 | Postfix / Dovecot / webmail  |
 +---------------+--------------+
                 |
                 | private network or TLS
                 | HTTP injection + config deploy + webhooks
                 v
 +------------------------------+
 | KumoMTA droplet              |
 |                              |
 | HTTP listener                |
 | queues                       |
 | spool                        |
 | DKIM signing                 |
 | traffic shaping              |
 | source IPs / egress pools    |
 +---------------+--------------+
                 |
                 | outbound SMTP
                 v
 +------------------------------+
 | Recipient MX servers         |
 +------------------------------+
```

Pros:

- Better isolation between application workloads and outbound MTA workloads.
- Kumo spool and logs do not compete with Postgres, Redis, Dovecot, Roundcube, or app logs.
- Easier to reason about disk pressure, queue pressure, and MTA restarts.
- Easier to scale KumoMTA independently from BillionMail.
- Cleaner IP reputation model because Kumo nodes can map directly to outbound IPs and PTR/EHLO naming.
- Smaller blast radius if KumoMTA has a queue spike or delivery incident.
- Easier to add more KumoMTA nodes later.
- Better security boundary between customer web/API traffic and outbound MTA operations.
- More realistic production pattern for a commercial multi-tenant sender.

Cons:

- More infrastructure to operate.
- Needs secure service-to-service networking.
- Needs config deployment from BillionMail to KumoMTA.
- Needs webhook/event receiver reliability.
- Slight extra latency between app and MTA, though this is usually irrelevant for bulk mail.
- Requires monitoring on both the app node and Kumo node.

Important DigitalOcean note:

DigitalOcean documentation currently says SMTP ports `25`, `465`, and `587` are blocked on Droplets by default. If your KumoMTA droplet is delivering directly to recipient MX servers, you must verify that outbound SMTP is actually allowed for that droplet/account or use a provider/environment that permits legitimate outbound MTA operation. Without outbound TCP 25, KumoMTA can accept messages but cannot deliver direct-to-MX.

### Option B: KumoMTA as a Docker container inside the current BillionMail Compose stack

This is acceptable for development and staging, but not for production multi-tenant SaaS.

Architecture:

```text
 +---------------------------------------------------------+
 | One BillionMail host / one Docker Compose stack         |
 |                                                         |
 | +----------+  +-------+  +---------+  +---------------+ |
 | | Core     |  | Redis |  | Postgres|  | KumoMTA       | |
 | +----+-----+  +-------+  +---------+  | spool/queues  | |
 |      |                                  +-------+-------+ |
 |      | HTTP injection                           |         |
 |      +------------------------------------------+         |
 |                                                         |
 | +----------+  +---------+  +----------+                  |
 | | Postfix  |  | Dovecot |  | Roundcube|                  |
 | +----------+  +---------+  +----------+                  |
 +---------------------------------------------------------+
                              |
                              v
                    +---------------------+
                    | Recipient MX        |
                    +---------------------+
```

Pros:

- Easiest local development setup.
- One deployment bundle.
- Good for proof-of-concept testing.
- Easy to test BillionMail-to-Kumo HTTP injection without a second host.
- Can use official KumoMTA container images.

Cons:

- Kumo spool disk I/O competes with Postgres, Dovecot mailbox storage, Postfix spool, Rspamd, and logs.
- Harder to scale the MTA independently.
- A queue spike can affect the UI/API/database/mailbox services.
- MTA restarts and config reloads happen inside the same operational boundary as the app.
- Outbound IP, PTR, EHLO, and source address management is harder when nested in the app stack.
- Noisy-neighbor risk is higher.
- Security blast radius is larger.
- Not appropriate for many tenants each expecting high-volume delivery.

Recommended use:

```text
Local development: yes
Demo environment: yes
Staging: yes, if volume is low
Production SaaS: no
```

### Option C: Integrate KumoMTA source code into the BillionMail codebase

This is not recommended.

Pros:

- Theoretical ability to deeply customize KumoMTA internals.
- One repository could contain everything.

Cons:

- BillionMail is a Go/React application; KumoMTA is a Rust/Lua MTA with its own lifecycle.
- You would inherit KumoMTA build complexity, release cadence, and security patching.
- Upstream KumoMTA upgrades become harder.
- Debugging mail delivery incidents becomes harder because the MTA is no longer a clean operational unit.
- Source-level integration does not improve tenant isolation.
- Source-level integration does not solve IP reputation, DNS, traffic shaping, queueing, or abuse.
- It blurs control plane and data plane responsibilities.
- It makes production deployment more fragile.

Recommended use:

```text
Do not vendor KumoMTA source into BillionMail.
Run KumoMTA as an external service.
Use official packages or official container images on dedicated MTA hosts.
```

## Recommended Production Architecture

Near-term:

```text
 +------------------------------------------------------+
 | BillionMail production node                          |
 |                                                      |
 | - UI/API/core                                        |
 | - Postgres                                           |
 | - Redis                                              |
 | - Postfix for local/inbound mailbox paths            |
 | - Dovecot/Roundcube for mailbox/webmail              |
 +-------------------------+----------------------------+
                           |
                           | private HTTP injection
                           | TLS + token/HMAC
                           v
 +------------------------------------------------------+
 | KumoMTA production node                              |
 |                                                      |
 | - HTTP listener                                      |
 | - tenant/campaign/domain queues                      |
 | - egress pools and source IPs                        |
 | - DKIM signing                                       |
 | - traffic shaping                                    |
 | - webhooks back to BillionMail                       |
 +-------------------------+----------------------------+
                           |
                           v
 +------------------------------------------------------+
 | Recipient mailbox providers                          |
 | Gmail / Yahoo / Outlook / corporate MX / others      |
 +------------------------------------------------------+
```

Growth architecture:

```text
                         +----------------------+
                         | BillionMail Core     |
                         +----------+-----------+
                                    |
                  +-----------------+-----------------+
                  |                                   |
                  v                                   v
        +-------------------+               +-------------------+
        | KumoMTA node 1    |               | KumoMTA node 2    |
        | pool shared       |               | pool enterprise   |
        +---------+---------+               +---------+---------+
                  |                                   |
                  v                                   v
        +-------------------+               +-------------------+
        | IPs shared/warmup |               | IPs dedicated     |
        +-------------------+               +-------------------+
                  |                                   |
                  +-----------------+-----------------+
                                    |
                                    v
                         +----------------------+
                         | Recipient MX servers |
                         +----------------------+
```

Larger SaaS architecture:

```text
 +-------------------+       +----------------------+
 | Load balanced     |       | Admin/ops dashboard  |
 | BillionMail API   |       +----------+-----------+
 +---------+---------+                  |
           |                            |
           v                            v
 +--------------------------------------------------+
 | Shared services                                  |
 | Postgres HA / Redis / object storage / metrics   |
 +-----------------------+--------------------------+
                         |
                         | injection routing
                         v
 +--------------------------------------------------+
 | KumoMTA fleet                                    |
 |                                                  |
 | node group: shared-low-volume                    |
 | node group: warmup                               |
 | node group: enterprise-dedicated                 |
 | node group: transactional/API                    |
 +-----------------------+--------------------------+
                         |
                         v
 +--------------------------------------------------+
 | Recipient providers                              |
 +--------------------------------------------------+
```

## Data Flow Details

### Campaign send flow

```text
1. Tenant creates campaign in BillionMail.
2. BillionMail verifies tenant owns the sender domain.
3. BillionMail validates SPF/DKIM/DMARC status.
4. BillionMail checks tenant quota and suppression policies.
5. Background worker selects pending recipients for that tenant/task.
6. BillionMail renders personalized HTML/text.
7. BillionMail adds tracking pixel, tracked links, unsubscribe links, and headers.
8. BillionMail injects the message into KumoMTA over HTTP.
9. KumoMTA assigns tenant/campaign/domain queue metadata.
10. KumoMTA selects the tenant egress pool.
11. KumoMTA applies provider-specific traffic shaping.
12. KumoMTA signs with the right DKIM key.
13. KumoMTA delivers to recipient MX.
14. KumoMTA posts events back to BillionMail.
15. BillionMail updates analytics and recipient state.
```

### API send flow

```text
1. Tenant calls BillionMail API with tenant API key.
2. BillionMail resolves api_key -> tenant_id -> api_template_id.
3. BillionMail checks IP whitelist, plan, quota, sender domain, and suppression.
4. BillionMail records pending api_mail_log.
5. Worker renders template and personalization.
6. Worker injects to KumoMTA.
7. KumoMTA queues, signs, shapes, and delivers.
8. Webhook updates api_mail_log and analytics.
```

### Event flow

```text
KumoMTA Delivery/Bounce/Deferral/Complaint
  |
  v
BillionMail /internal/kumo/events
  |
  +-- verify auth
  +-- dedupe event
  +-- resolve tenant/message/recipient
  +-- update raw mail_events
  +-- update recipient_info or api_mail_logs
  +-- update campaign counters
  +-- update tenant usage
  +-- update suppression on hard bounce/complaint
```

## Configuration Ownership

Recommended rule: BillionMail owns customer intent; KumoMTA owns delivery execution.

BillionMail should generate or publish:

```text
tenants.toml/json
tenant_to_pool.toml/json
sources.toml
dkim_data.toml
shaping overrides
webhook endpoint config
```

KumoMTA should consume these through:

- versioned config files deployed to the Kumo node
- a controlled config deployment command
- a reload or config epoch bump
- eventually, a config service or GitOps-style deployment

Avoid connecting KumoMTA directly to the BillionMail production database in v1. A direct DB dependency couples the MTA runtime to app schema changes and makes delivery incidents harder to isolate.

## Security Requirements

Minimum controls:

- KumoMTA HTTP injection endpoint must not be public without strong authentication.
- Prefer private networking between BillionMail and KumoMTA.
- Use TLS for injection if traffic crosses any untrusted network.
- Use token or HMAC authentication for injection.
- Use token or HMAC authentication for webhooks.
- Allowlist BillionMail source IPs on KumoMTA.
- Allowlist KumoMTA source IPs on BillionMail webhook endpoint.
- Do not expose sequential tenant IDs in public URLs or headers.
- Sign unsubscribe/tracking URLs.
- Enforce tenant ownership before allowing sender domain use.
- Enforce suppression before injection.
- Enforce quotas before injection.
- Add abuse suspension controls at tenant and campaign level.

Security boundary diagram:

```text
Public Internet
      |
      v
 +----------------------+
 | BillionMail public   |
 | UI/API/tracking      |
 +----------+-----------+
            |
            | private/authenticated
            v
 +----------------------+
 | KumoMTA injection    |
 | not public to users  |
 +----------+-----------+
            |
            | outbound SMTP
            v
 +----------------------+
 | Recipient MX         |
 +----------------------+

KumoMTA webhook -> BillionMail internal endpoint
must be authenticated and idempotent.
```

## Operational Requirements

For production, monitor at least:

- Kumo queue depth by tenant/campaign/domain.
- Kumo ready queue depth.
- Kumo deferred queue depth.
- spool disk free bytes and inodes.
- injection accept/reject rates.
- delivery/bounce/deferral rates.
- provider-level throttling.
- per-tenant daily/hourly send rate.
- per-tenant complaint rate.
- per-tenant hard bounce rate.
- webhook delivery failures.
- BillionMail event ingestion lag.
- Postgres write volume from event ingestion.

Backpressure rules:

```text
If Kumo injection fails:
  keep BillionMail message pending
  retry with exponential backoff
  do not mark as sent

If Kumo queue depth for tenant is too high:
  slow BillionMail injection for that tenant
  keep campaign active but throttled

If tenant bounce/complaint threshold is exceeded:
  pause tenant campaign
  stop injection
  alert operator

If webhook ingestion is down:
  Kumo webhook queue should retry
  BillionMail should alert on event lag
```

## Migration/Roadmap

This roadmap is planning only. It describes future work and does not change the current codebase.

### Phase 0: KumoMTA droplet validation

- Confirm outbound TCP 25 works from the KumoMTA droplet.
- Confirm PTR/rDNS for each outbound IP.
- Confirm EHLO hostnames align with PTR records.
- Confirm SPF includes the KumoMTA outbound IPs.
- Confirm DKIM keys can be generated and DNS records published.
- Confirm DMARC alignment for tenant domains.
- Confirm KumoMTA spool disk has enough capacity and inode headroom.
- Confirm KumoMTA metrics are accessible.
- Confirm logs/webhooks can reach BillionMail.

### Phase 1: Tenant foundation

- Add tenant tables and tenant membership.
- Migrate all existing data into a default tenant.
- Add tenant context middleware.
- Add tenant-scoped query patterns.
- Add tenant-aware API keys.
- Add tenant-scoped uniqueness constraints.
- Add tenant-aware operation logs.
- Add tests proving tenant isolation.

### Phase 2: Sending abstraction

- Add `OutboundMailer` design.
- Keep existing Postfix SMTP sender as one implementation.
- Add Kumo HTTP injection implementation.
- Route campaign/API sending through Kumo for selected tenants.
- Keep Postfix for local mailbox/system paths.
- Add retry/backoff when Kumo injection is unavailable.

### Phase 3: Kumo event ingestion

- Add authenticated internal endpoint for Kumo events.
- Store raw normalized events in a tenant-scoped event table.
- Update campaign/API statuses from Kumo events.
- Update suppression lists from bounces and complaints.
- Make event ingestion idempotent.
- Add event lag monitoring.

### Phase 4: Kumo config generation

- Generate tenant-to-pool mappings from BillionMail settings.
- Generate `sources.toml` from configured sending IPs.
- Generate DKIM config from tenant domains/selectors.
- Generate shaping overrides for pools/providers.
- Deploy config to KumoMTA with versioning and rollback.

### Phase 5: SaaS hardening

- Per-tenant daily/hourly quotas.
- Per-provider throttles.
- Warmup workflows.
- Abuse detection and suspension.
- Billing usage counters.
- Dedicated IP pool assignment.
- Operator review tools.
- Multiple KumoMTA nodes.
- Disaster recovery and queue/spool runbooks.

## Database Direction

Do not try to retrofit multi-tenancy only at the UI level. It must be enforced in the database access layer and API layer.

Recommended event model:

```text
mail_events
  id
  tenant_id
  message_id
  provider_message_id
  campaign_id
  task_id
  recipient_id
  api_log_id
  sender
  recipient
  event_type
  event_time
  provider
  mx_host
  response_code
  response_text
  raw_event_json
  created_at
```

Recommended status model:

```text
recipient_info
  tenant_id
  task_id
  recipient
  message_id
  injection_status
  delivery_status
  last_event_at
  last_error

api_mail_logs
  tenant_id
  api_id
  recipient
  message_id
  injection_status
  delivery_status
  last_event_at
  last_error
```

This is cleaner than continuing to make everything look like a Postfix message ID pipeline after KumoMTA is introduced.

## KumoMTA Policy Direction

KumoMTA policy should be generated from BillionMail tenant settings but stay operationally simple.

Illustrative model:

```text
Tenant settings in BillionMail:
  tenant-a -> pool dedicated-a
  tenant-b -> pool shared-warmup
  tenant-c -> pool dedicated-c

KumoMTA policy:
  on message received:
    read tenant metadata
    read campaign metadata
    set queue metadata

  on get_queue_config:
    map tenant -> egress pool

  on get_egress_pool:
    return pool sources

  on get_egress_source:
    return source_address and ehlo_domain

  on message generated:
    DKIM sign using sender domain selector/key

  on log event:
    publish webhook to BillionMail
```

## Testing And Acceptance Criteria

Tenant isolation:

- Tenant A cannot list Tenant B domains.
- Tenant A cannot use Tenant B sender domain.
- Tenant A cannot access Tenant B contacts, templates, campaigns, API keys, logs, or unsubscribe records.
- Tenant A tracking and unsubscribe links cannot mutate Tenant B data.

Kumo injection:

- One campaign message can be injected to KumoMTA.
- Batch injection preserves BillionMail message IDs.
- KumoMTA queue metadata includes tenant and campaign identifiers.
- Injection failure leaves the message retryable, not marked delivered.

Kumo events:

- Delivery event updates the correct tenant/task/recipient.
- Hard bounce updates delivery status and suppression.
- Transient failure updates deferred status without suppressing.
- Complaint updates suppression and campaign analytics.
- Duplicate webhook event is ignored or safely idempotent.

Load:

- Queue at least 10,000 messages across multiple tenants.
- Verify tenant queue separation.
- Verify per-tenant rate caps.
- Verify provider-level shaping.
- Verify webhook ingestion keeps up.

Operations:

- KumoMTA restart does not lose accepted queued mail.
- BillionMail event receiver downtime causes webhook retry, not permanent event loss.
- Spool disk pressure triggers alerts.
- Tenant abuse threshold pauses sending.

## Final Recommendation

For the product you described - customers configuring their own email-related settings and potentially sending 1 million emails per day - the right architecture is:

```text
BillionMail SaaS control plane
  +
KumoMTA dedicated outbound data plane
  +
tenant-scoped database model
  +
per-tenant quotas, pools, DKIM, suppression, and analytics
```

Use your existing KumoMTA DigitalOcean droplet as the first external MTA integration target only after confirming outbound SMTP is permitted and deliverability DNS is correct. Then move toward a dedicated KumoMTA node pool or cluster as volume grows.

The production decision is:

```text
Recommended:
  BillionMail app stack and KumoMTA run separately.

Allowed for dev/staging:
  KumoMTA container inside the BillionMail compose environment.

Not recommended:
  Vendoring or embedding KumoMTA source code into BillionMail.
```

## Sources

- KumoMTA GitHub repository: https://github.com/KumoCorp/kumomta
- KumoMTA Docker installation: https://docs.kumomta.com/userguide/installation/docker/
- KumoMTA HTTP injection: https://docs.kumomta.com/userguide/operation/httpinjection/
- KumoMTA queues: https://docs.kumomta.com/reference/queues/
- KumoMTA sending IPs and egress pools: https://docs.kumomta.com/userguide/configuration/sendingips/
- KumoMTA traffic shaping: https://docs.kumomta.com/userguide/configuration/trafficshaping/
- KumoMTA DKIM signing: https://docs.kumomta.com/userguide/configuration/dkim/
- KumoMTA webhooks/log hooks: https://docs.kumomta.com/userguide/operation/webhooks/
- KumoMTA spooling: https://docs.kumomta.com/userguide/configuration/spool/
- DigitalOcean SMTP blocking policy: https://docs.digitalocean.com/support/why-is-smtp-blocked/
