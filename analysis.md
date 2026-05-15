# KumoMTA Egress Strategy on DigitalOcean — Analysis & Verdict

Date: 2026-05-14
Author: Implementation review for BillionMail (`gitdate.ink`)
Inputs: [test.md](test.md), [agent_kumo_billion.md](agent_kumo_billion.md), KumoMTA docs, DigitalOcean docs, vendor docs.

---

## 1. The Problem In One Paragraph

BillionMail's KumoMTA droplet sits on DigitalOcean (`159.89.33.85` / `192.241.130.241`). DigitalOcean blocks **outbound TCP 25, 465, and 587 on every Droplet by default**, including traffic routed through Reserved IPs. KumoMTA's actual job — opening an SMTP session to the recipient's MX server — happens on port 25. So with the current topology, KumoMTA can be reached, configured, injected into, and will accept HTTP `2xx` for messages, but the messages will never leave the droplet. The integration is "UI-deployed but not delivery-ready," exactly what [test.md](test.md) and `agent_kumo_billion.md` already flag.

This document inventories every realistic egress path, scores each one against your stack, and gives a recommendation.

---

## 2. Confirmed Facts (don't re-litigate these)

| Fact | Source |
|---|---|
| DO blocks outbound 25/465/587 on all Droplets, including Reserved IPs | [DigitalOcean: Why is SMTP blocked?](https://docs.digitalocean.com/support/why-is-smtp-blocked/) |
| DO support "may" lift the block; in practice approval is rare and recent reports suggest it is now effectively impossible for new accounts | [DO community](https://www.digitalocean.com/community/questions/why-is-digitalocean-blocking-outbound-port-25-on-my-droplet), [Mail-in-a-Box thread](https://discourse.mailinabox.email/t/digitalocean-smtp-port-25-is-now-blocked-for-all-new-accounts/9346) |
| KumoMTA's recommended egress pattern is a SOCKS5 proxy (KumoProxy) running on a separate, port-25-capable host | [KumoMTA: Proxy](https://docs.kumomta.com/userguide/operation/proxy/), [KumoProxy](https://docs.kumomta.com/userguide/operation/kumo-proxy/) |
| KumoMTA also supports HAProxy v2 PROXY protocol, but the docs explicitly say "SOCKS5 should be utilized when possible" because HAProxy as a forward proxy gives no error feedback to the MTA | [KumoMTA proxy docs](https://docs.kumomta.com/userguide/operation/proxy/) |
| KumoMTA can use any third-party SMTP relay as a smart-host with SMTP AUTH on 587 or 2525 | [KumoMTA: Outbound SMTP AUTH](https://docs.kumomta.com/userguide/operation/outbound_auth/) |
| Hetzner, OVH, Vultr and Linode all *also* block 25 by default but allow case-by-case unblock — generally with much higher approval rates than DO | [Hetzner](https://j11g.com/2022/01/03/bypassing-hetzner-mail-port-block-port-25-465/), [OVH](https://support.us.ovhcloud.com/hc/en-us/articles/16100926574995-OVHcloud-AntiSpam-Best-practices-and-unblocking-an-IP), [Vultr](https://docs.vultr.com/support/products/compute/why-is-smtp-blocked) |
| KumoMTA recommends a clustered architecture where *MTA nodes share configuration* and *egress is proxied* — exactly so you can keep the MTA wherever and put the IPs elsewhere | [KumoMTA: Deployment Architecture](https://docs.kumomta.com/userguide/clustering/deployment/) |
| [agent_kumo_billion.md](agent_kumo_billion.md) §2 already mandates: "Production delivery must use an allowed egress path: external KumoProxy/SOCKS5, external KumoMTA egress node, provider HTTP API, or provider SMTP submission on a permitted port such as 2525." | Your own PRD |

Translation: this is a known, common scenario. The KumoMTA project literally ships a solution for it (KumoProxy).

---

## 3. Approaches — Full Inventory

I evaluated **eight** distinct approaches. They are ordered from "least change to current topology" to "most change."

### Approach A — Ask DigitalOcean to unblock port 25/587 on the droplet

**How:** Open a Networking & Security ticket. Justify use case, list SPF/DKIM/DMARC, explain volumes, confirm no marketing spam, and so on.

**Pros**
- Zero architectural change.
- If approved, KumoMTA can sit where it is and deliver direct-to-MX.

**Cons**
- DO approval rate for port 25 unblock has dropped to near-zero for new accounts in 2025–2026. Public reports show DO routinely closing tickets with no unblock.
- Even on approval, you're stuck using **DO IP space**, which has a well-known low sender reputation for bulk mail. Recipient MTAs (Gmail/Outlook/Yahoo) score DO ranges aggressively.
- No fallback if DO later reinstates the block (they have done this before with no notice).
- You're betting the entire production launch on a one-way ticket decision you don't control.

**Verdict:** Do not rely on this. It is acceptable to *file* the ticket, but assume `no` and design around it.

---

### Approach B — External KumoProxy (SOCKS5) on a port-25-allowed host

**How:**
1. Rent a small box on a provider that allows outbound 25 (Hetzner / OVH / Vultr-with-unblock / bare-metal). 1 vCPU / 1 GB is enough for tens of millions of messages/month — KumoProxy is a thin TCP-relay.
2. Install KumoMTA package on it just to get the binary at `/opt/kumomta/sbin/proxy-server`. Run `proxy-server --listen <ip:port>`. Systemd unit, `Restart=always`.
3. On the DO KumoMTA droplet, define every sending IP as a separate `egress_source`:
   ```lua
   kumo.make_egress_source {
     name = 'gitdate-ip-1',
     socks5_proxy_server      = '<proxy-host>:5000',
     socks5_proxy_source_address = '<public-ip-on-proxy-host>',
     ehlo_domain = 'mail.gitdate.ink',
   }
   ```
4. Group sources into an `egress_pool`, point your tenant `tenant_sending_profiles.kumo_pool` at it.

**Pros**
- This is the **KumoMTA-recommended pattern**. Docs literally say: "We recommend the use of a proxy for egress; this allows each node to have an identical configuration while still being able to use an appropriate source IP address."
- Sending IP reputation is **the proxy host's IP**, not DO's. You can plumb multiple IPs on the proxy NIC and rotate them. Phase 7 of your PRD (sending profiles, dedicated IPs, warmup) becomes possible.
- DO droplet remains as the BillionMail + KumoMTA control plane — minimal disruption to your existing topology.
- KumoMTA gets actionable SMTP-level errors from the proxy (the whole reason SOCKS5 is preferred over HAProxy).
- Scales horizontally: add more KumoProxy hosts with different IP plumbing as your dedicated-pool tenants grow.

**Cons**
- New infrastructure piece to operate. You now own the proxy host(s) — patching, monitoring, IP-warming.
- Latency: every SMTP connection adds one extra hop. Negligible at <500 emails/sec; relevant only at very high volume.
- Proxy host needs its own SPF / rDNS / PTR alignment for the sending IPs. PRD Phase 7 already calls this out.
- KumoProxy is documented as tested on Ubuntu 24 only — slight Ops cost.
- One known issue ([KumoCorp/kumomta#153](https://github.com/KumoCorp/kumomta/issues/153)): early KumoProxy versions only delivered from the first plumbed IP. Resolved in recent releases, but pin to a known-good version and load-test.

**Verdict:** Strong default for a serious sender. Highest ceiling, modest operational cost.

---

### Approach C — External KumoMTA *egress node* (move the MTA, not just a proxy)

**How:** Run a second KumoMTA install on a port-25-allowed host. BillionMail on DO injects to it over HTTPS just like today (it doesn't care where Kumo lives). The remote node owns the IPs, the queues, the shaping, the DKIM signing, the webhook emission.

**Pros**
- Operationally simpler than B: no separate proxy daemon, one fewer process class, one fewer protocol (SOCKS5).
- Cleanly maps to PRD §4.4 "Acceptable Kumo injection path: BillionMail public IP → HTTPS + TLS verification + strong auth + source allowlisting → https://email.gitdate.ink".
- If you ever leave DO, you don't have to disentangle a proxy abstraction.
- All sending state, shaping data, TSA daemon, and Redis throttles live where the IPs live, which is the KumoMTA-recommended layout for single-node deployments.

**Cons**
- You now move the *whole* MTA off DO. Bigger box than the proxy approach. More config to redeploy.
- If you want multi-IP / multi-region down the line, you eventually end up at Approach B anyway (proxy is the better long-term shape for horizontal scaling).
- The DO KumoMTA droplet (`192.241.130.241`) becomes dead weight — you'd retire it.

**Verdict:** Best choice **if you want to retire the DO Kumo droplet** and you're not yet at a scale that demands multiple IPs. It is functionally equivalent to "host KumoMTA somewhere sane in the first place."

---

### Approach D — External SMTP relay (smart-host) on port 2525 or 587

Providers: Amazon SES, SendGrid, Mailgun, Postmark, SparkPost, Brevo, Resend.

**How:** Configure KumoMTA with `smtp_auth_plain_username` / `smtp_auth_plain_password` and a `shaping.toml` entry pointing the destination MX/host to the provider on port 2525 (port 2525 is *not* blocked by DO, since it's not a standard SMTP port).

```lua
kumo.on('get_egress_path_config', function(routing_domain, source, site_name)
  return kumo.make_egress_path {
    enable_tls = 'Required',
    smtp_auth_plain_username = '<api-user>',
    smtp_auth_plain_password = { key_data = '<api-key>' },
  }
end)
```

```toml
# shaping.toml
["smtp-relay.sendinblue.com"]
mx_rollup = false
connection_limit = 50
```

**Pros**
- Fastest path to "real production delivery." Hours, not days.
- DO droplet stays put. No new infra at all.
- Provider handles MX direct-to-mailbox, IP warming, suppression list freshness, FBL feedback.
- Best deliverability *out of the box* — you inherit SES's or Mailgun's IP reputation.
- Costs scale linearly; free tiers exist (SES ~$0.10/1000 emails, Mailgun ~$15/mo entry, SendGrid free tier 100/day).

**Cons**
- **You lose the central reason to run KumoMTA in the first place.** All the value props — dedicated IPs, traffic shaping, granular per-domain throttling, FBL parsing, your own bounce classification, multi-tenant pool isolation — are now handled by someone else's MTA.
- KumoMTA collapses to "a queue with retries that calls another MTA." A simple SMTP client + queue would do the same job.
- Costs grow per-message forever. At 10M emails/month, SES is ~$1,000/mo, Mailgun is ~$3,000+/mo. KumoProxy on a $20 Hetzner box is $20/mo.
- Bounce/complaint events come back via the **provider's** webhook format, not KumoMTA's. You'd need to either (a) wire the provider's webhook directly into BillionMail's `/api/kumo/events` (forking your event ingestion), or (b) accept that webhook-driven final delivery state is fed by the provider, not Kumo. Your PRD Phase 3 schema (`kumo_delivery_events`) was designed for KumoMTA's emission shape — it would need adapters.
- Per-tenant IP isolation is provider-dependent (SES dedicated IPs cost $24.95/mo each; Mailgun dedicated IPs $59/mo each). Multi-tenant SaaS dedicated-IP economics are very different here than on bare metal.

**Verdict:** Good as a **launch fallback** so you can ship to paying customers next week without solving the IP problem. Bad as a strategic destination — you spent the implementation effort to integrate KumoMTA; this approach throws away the part you paid for.

---

### Approach E — Provider HTTP API (SES HTTP, SendGrid Web API, Postmark, etc.)

**How:** Bypass KumoMTA entirely for the affected tenant/profile. BillionMail's `OutboundMailer` interface (Phase 2 of your PRD already mandates this abstraction) gets a third implementation: `SESHTTPMailer`, `MailgunHTTPMailer`, etc.

**Pros**
- Even faster than D for very low volume (no SMTP at all).
- Best provider latency (HTTPS push is lower-overhead than SMTP submission).
- Provider-native idempotency, batch endpoints, and templating.
- Same `OutboundMailer` plug-in shape your PRD already specifies.

**Cons**
- All the cons of Approach D, **plus** you don't even get the SMTP-shaped abstraction. Per-recipient pricing.
- The KumoMTA box becomes mostly useless for those tenants — you're routing around it entirely. KumoMTA is reduced to handling only the (currently-zero) tenants you can deliver direct-to-MX for.
- KumoMTA events / queues / webhooks are not exercised on this path, so your delivery telemetry is bifurcated — half from Kumo (for tenants that go via Kumo), half from each provider's webhook (for tenants on Approach E).

**Verdict:** Niche. Useful if you have a small "transactional only" tier where you want to charge customers a flat fee and never warm an IP. Avoid as a general strategy.

---

### Approach F — HAProxy v2 PROXY protocol egress

**How:** Run HAProxy on a port-25-allowed external host. KumoMTA uses `ha_proxy_server` + `ha_proxy_source_address` instead of `socks5_proxy_server`.

**Pros**
- Same architectural benefit as Approach B (egress IP lives off DO).
- If you already operate HAProxy elsewhere, you can reuse expertise/tooling.

**Cons**
- KumoMTA docs explicitly say **prefer SOCKS5**. HAProxy as a forward proxy can't tell the MTA *why* a connection failed — KumoMTA only sees timeouts where SOCKS5 would surface a 4xx/5xx code. This breaks your bounce-classification logic.
- More TCP/PROXY-protocol surface area to misconfigure.

**Verdict:** Don't pick this unless you already run HAProxy at scale and have a real reason. SOCKS5 (Approach B) wins on the same problem.

---

### Approach G — WireGuard / IPsec tunnel from DO to a port-25-allowed host

**How:** Drop a tunnel between the DO droplet and a small remote box. Use `ip rule` + `ip route` to send TCP/25 outbound traffic through the tunnel.

**Pros**
- Transparent — no application changes, no SOCKS5 client code path.
- Cheap to stand up.

**Cons**
- **DO TOS exposure.** This is functionally a way to evade the SMTP block. DO's policy says traffic through Reserved IPs is also blocked; routing port 25 traffic through a tunnel that lands on a non-DO box is exactly what they're trying to prevent. If they detect it (which they can — sustained tunneled traffic with port-25 payload is fingerprintable), they may suspend the droplet.
- No per-IP egress control — you're tunneling, not selecting from a pool of source IPs. You lose the multi-IP value of Approaches B/C.
- KumoMTA traffic shaping still thinks it's connecting from DO IPs, so retry/backoff heuristics may misfire.

**Verdict:** Hard no. Worse than B in every dimension and adds policy risk.

---

### Approach H — Move everything off DigitalOcean

**How:** Migrate BillionMail + KumoMTA + Postfix/Dovecot/Rspamd stack to Hetzner or OVH dedicated, where port 25 is unblockable by support ticket.

**Pros**
- Cleanest. No proxies, no relays, no second hop. Just open SMTP from a box you own.
- Bare-metal IPs on OVH/Hetzner have decent baseline reputation when warmed properly.
- Cheaper at scale (a Hetzner AX41 dedicated is roughly the price of a mid-size DO droplet but with 2 IPs and no port block).

**Cons**
- Big lift. Disrupts everything else that runs on DO for `gitdate.ink` (BillionMail UI, Postfix, Dovecot, Rspamd, Roundcube, webhooks).
- You replicate the risk: Hetzner *also* defaults port 25 to blocked and unblocks per-IP on request — approval is usual but not instant.
- Doesn't solve "want lots of dedicated IPs" — you still want Approach B's KumoProxy on top once you scale.

**Verdict:** Right answer if you were starting over today. As a migration mid-project, the cost-to-benefit is poor *unless* you have other reasons to leave DO.

---

## 4. Side-by-Side Scorecard

| | A: ask DO | B: KumoProxy | C: ext Kumo node | D: smart-host | E: provider HTTP API | F: HAProxy | G: tunnel | H: leave DO |
|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|
| Likely to succeed | ❌ low | ✅ high | ✅ high | ✅ high | ✅ high | ✅ ok | ⚠️ TOS risk | ✅ high |
| Time to "first send" | days–weeks (or never) | 1–2 days | 1–2 days | hours | hours | 1–2 days | hours | 1–2 weeks |
| Preserves KumoMTA value | ✅ | ✅ | ✅ | ❌ | ❌❌ | ✅ | ⚠️ | ✅ |
| Multi-IP / dedicated pools | ⚠️ DO IPs only | ✅✅ | ⚠️ single-host scale | ⚠️ provider-priced | ⚠️ provider-priced | ✅ | ❌ | ⚠️ single-host scale |
| Per-tenant isolation (PRD §6) | ⚠️ | ✅✅ | ✅ | ⚠️ | ❌ | ✅ | ❌ | ✅ |
| Marginal cost @ 10M/mo | $0 if approved | ~$20–80 (proxy box) | ~$50–150 | ~$1k–3k+ | ~$1k–5k+ | ~$50 | ~$10 | ~$100–200 |
| Ops complexity | low | medium | medium | low | low | medium | high (and fragile) | high (migration) |
| Match to your PRD §2/§4.4 explicit list | not listed | ✅ "external KumoProxy/SOCKS5" | ✅ "external Kumo node" | ✅ "provider SMTP submission on port 2525" | ✅ "provider HTTP API" | partial (HAProxy variant of B) | not listed | not listed |
| Strategic upgrade path | dead-end | scales horizontally | upgrades to B | locked into provider | locked into provider | inferior to B | dead-end + risky | upgrades to B |

---

## 5. Mapping To Your PRD Phases

Your PRD already commits to:

> "DigitalOcean-hosted KumoMTA must not be treated as production direct-to-MX egress while outbound SMTP ports are blocked."
> "Production delivery must use an allowed egress path: external KumoProxy/SOCKS5, external KumoMTA egress node, provider HTTP API, or provider SMTP submission on a permitted port such as 2525."

So the question is not *whether* — it's *which one for v1, and what's the upgrade path*.

The PRD's existing structure already accommodates this:

- **Phase 2** introduces an `OutboundMailer` interface (`PostfixSMTPMailer`, `KumoHTTPMailer`). A `RelayHTTPMailer` (Approach E) is a third trivial implementation of the same interface if/when you want it.
- **Phase 7** introduces `kumo_egress_sources`, `kumo_egress_pools`, `kumo_egress_pool_sources`, and per-tenant `egress_mode` + `egress_provider`. **Approach B fits this schema natively** — each KumoProxy host with N plumbed IPs becomes N `kumo_egress_sources` rows in a pool.
- **Phase 8** generates KumoMTA policy from this state. Approach B requires only one extra Lua block in the generator (`socks5_proxy_server` + `socks5_proxy_source_address`). Approach D requires a `make_egress_path` AUTH block per relay provider.

This is all already built for. You're not adding architecture, you're picking which row of `tenant_sending_profiles.egress_mode` is the v1 default.

---

## 6. Recommended Path (Verdict)

**Short answer:** Approach D for week-1 launch, Approach B as the strategic destination, run them concurrently.

### Phase α (this week — unblock paying-customer onboarding)

Pick **Amazon SES SMTP on port 2525** as the v1 default `egress_mode = "provider_smtp"`:

1. Verify your `email.gitdate.ink` sender domain in SES.
2. Add SES DKIM CNAMEs (they replace your current `s1._domainkey` for SES-sent mail; keep `s1._domainkey` for KumoMTA-direct path later).
3. Configure KumoMTA with a `make_egress_path` providing SES credentials over 2525 (DO does not block 2525).
4. In BillionMail, create a `tenant_sending_profile` with `egress_mode = provider_smtp`, `egress_provider = ses`, pointing all tenants at it by default.
5. Wire SES's SNS/webhook bounces back into `/api/kumo/events` via a small adapter, OR accept Kumo's own bounce parsing from the relay response (SES will return SMTP-level reject codes).

You can have real production sending in 24–48 hours, billed at SES rates ($0.10 / 1000), with the rest of the platform untouched.

### Phase β (2–6 weeks — own your reputation, unlock multi-IP)

Stand up **KumoProxy on an external host** (Hetzner CX22 / Vultr High-Frequency / OVH VLE) and migrate tenants who need dedicated IPs to `egress_mode = "kumo_proxy"`:

1. Provision a small box on Hetzner. File the port-25 unblock request on day 0 (Hetzner usually approves within 24h after first invoice).
2. Plumb 2–4 public IPs on the NIC. Set rDNS / PTR for each.
3. Publish per-IP SPF (`v=spf1 ip4:<proxy-ip-1> ip4:<proxy-ip-2> -all`), DKIM (`s1._domainkey.email.gitdate.ink`), DMARC.
4. Install KumoMTA package on the proxy host purely to get `/opt/kumomta/sbin/proxy-server`. Run via systemd.
5. Add the proxy IPs as `kumo_egress_sources`, group into pools, wire to high-volume / dedicated tenants via their `tenant_sending_profile.kumo_pool`.
6. Warm the IPs over 2–3 weeks per Phase 7 of the PRD.

This keeps the DO KumoMTA droplet as the control plane and webhook receiver, while the actual SMTP sessions originate from your own clean, port-25-enabled IPs.

### Why this combo, and not just one of them

| Risk | How this combo handles it |
|---|---|
| "We need to send mail before we've fixed infra" | Phase α delivers in 48h |
| "SES decides we're too marketing-heavy and throttles us" | Phase β provides a dedicated escape hatch |
| "A tenant demands a dedicated IP" | Phase β provides it; SES dedicated IPs cost $24.95/mo each anyway |
| "We outgrow shared IPs" | Phase β scales horizontally — add more KumoProxy hosts |
| "We lose KumoMTA's value props" | Phase β puts them back; Phase α only routes through SES, doesn't replace Kumo |

This is also exactly the layered model the KumoMTA team themselves describe in their [GA announcement](https://kumomta.com/blog/announcing-general-availability-of-kumomta): start with a smart-host relay to derisk launch, migrate to KumoMTA-direct as you scale the reputation system.

### What NOT to do

- **Don't bet on Approach A.** It's fine to file the ticket, but do not block the launch on DO's response.
- **Don't use Approach G (tunnel).** TOS risk. Worse than B at the same job.
- **Don't pick Approach F (HAProxy).** KumoMTA docs are explicit: prefer SOCKS5.
- **Don't move the entire stack off DO (Approach H) just to fix this**, unless you have other reasons to migrate. Approach B gives you 90% of H's benefits for 10% of the effort.

---

## 7. Concrete Next Actions (this sprint)

1. **Today** — confirm SES eligibility (already have a domain verified for `email.gitdate.ink`? if not, do that).
2. **Today** — file Hetzner / OVH order and port-25 unblock request (kicks off the clock).
3. **Day 1** — add `tenant_sending_profile.egress_mode` enum: `kumo_direct | kumo_proxy | provider_smtp | provider_http | postfix_local`. PRD already implies this; make it explicit.
4. **Day 2–3** — implement Approach D wiring (SES smart-host config + Phase 3 event adapter for SES bounces).
5. **Day 2–3** — Phase 1 of agent runbook (`/api/kumo/config`, `/api/kumo/test_connection`, webhook shell) — independent of egress choice.
6. **Day 4–7** — UAT campaign send through SES via KumoMTA. Verify webhook health, bounce ingestion, suppression updates.
7. **Week 2–3** — once Hetzner/OVH box is ready, stand up KumoProxy. Migrate one internal tenant to `kumo_proxy` egress as a soak test.
8. **Week 4+** — promote `kumo_proxy` to default for new high-volume tenants; keep `provider_smtp` as the fallback profile.

Throughout: maintain the rule from [agent_kumo_billion.md](agent_kumo_billion.md) §2 — direct-to-MX from DO stays officially blocked until the egress path is proven on Hetzner/SES.

---

## 8. Open Questions To Resolve Before Coding

These belong in your next standup, not in code yet:

1. **Provider choice for Approach D:** Amazon SES (cheapest, highest deliverability, requires AWS account + sandbox exit) vs Mailgun (more turnkey, higher per-msg cost) vs Postmark (best transactional reputation, expensive for bulk). Tenant mix should drive this.
2. **Host choice for Approach B:** Hetzner Cloud (CX22 ~€4/mo, port 25 unblock by ticket post-payment) vs OVH (VPS Value, port 25 open by default in many regions) vs bare-metal (overkill for v1).
3. **DKIM key strategy:** does each tenant get a per-tenant DKIM selector, or is `s1._domainkey.email.gitdate.ink` the shared signer? PRD Phase 7 leaves this open.
4. **Webhook split:** when a tenant is on Approach D (SES), the bounce/delivery events come from SES, not Kumo. Do we (a) translate them into a Kumo-event-shaped payload on a small adapter so `/api/kumo/events` stays the single source of truth, or (b) accept two ingest paths? Recommend (a) for schema simplicity.

---

## 9. Sources

- [DigitalOcean — Why is SMTP blocked?](https://docs.digitalocean.com/support/why-is-smtp-blocked/)
- [DigitalOcean community — outbound SMTP block](https://www.digitalocean.com/community/questions/why-is-digitalocean-blocking-outbound-port-25-on-my-droplet)
- [Mail-in-a-Box — DO blocks SMTP for all new accounts](https://discourse.mailinabox.email/t/digitalocean-smtp-port-25-is-now-blocked-for-all-new-accounts/9346)
- [KumoMTA — Routing Messages Via Proxy Servers](https://docs.kumomta.com/userguide/operation/proxy/)
- [KumoMTA — KumoProxy SOCKS5 Server](https://docs.kumomta.com/userguide/operation/kumo-proxy/)
- [KumoMTA — make_egress_source reference](https://docs.kumomta.com/reference/kumo/make_egress_source/)
- [KumoMTA — make_egress_pool reference](https://docs.kumomta.com/reference/kumo/make_egress_pool/)
- [KumoMTA — Delivering Messages Using SMTP Auth](https://docs.kumomta.com/userguide/operation/outbound_auth/)
- [KumoMTA — Clustering / Deployment Architecture](https://docs.kumomta.com/userguide/clustering/deployment/)
- [KumoMTA — Configuring Sending IPs](https://docs.kumomta.com/userguide/configuration/sendingips/)
- [KumoMTA — Announcing GA + First Production Deployment](https://kumomta.com/blog/announcing-general-availability-of-kumomta)
- [KumoCorp/kumomta#153 — KumoProxy first-IP-only bug](https://github.com/KumoCorp/kumomta/issues/153)
- [KumoCorp/kumomta#45 — original SOCKS5/KumoProxy issue](https://github.com/KumoCorp/kumomta/issues/45)
- [Hetzner mail port 25 unblock procedure](https://j11g.com/2022/01/03/bypassing-hetzner-mail-port-block-port-25-465/)
- [OVHcloud AntiSpam best practices](https://support.us.ovhcloud.com/hc/en-us/articles/16100926574995-OVHcloud-AntiSpam-Best-practices-and-unblocking-an-IP)
- [Vultr — Why Is SMTP Blocked?](https://docs.vultr.com/support/products/compute/why-is-smtp-blocked)
- [Captain DNS — Port 25 blocked by provider](https://www.captaindns.com/en/blog/port-25-blocked-diagnosis-solutions)
- [Postfix smart-host / SES / Mailgun / SendGrid relay configuration](https://www.cyberciti.biz/faq/how-to-configure-postfix-relayhost-smarthost-to-send-email-using-an-external-smptd/)
- [AWS SES — SMTP Relay docs](https://docs.aws.amazon.com/ses/latest/dg/eb-relay.html)
- [BillionMail PRD: `agent_kumo_billion.md`](agent_kumo_billion.md)
- [BillionMail integration smoke-test: `test.md`](test.md)
