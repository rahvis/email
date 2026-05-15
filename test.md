That screen means BillionMail has the KumoMTA UI deployed, but KumoMTA is not configured yet. Test in this order.

**1. Verify Kumo Is Reachable**
From your machine first:

```bash
curl -i https://email.gitdate.ink/api/inject/v1
curl -i https://email.gitdate.ink/metrics
```

Expected:

- `/api/inject/v1`: `401`, `403`, `405`, or `2xx` means reachable.
- Timeout/DNS/TLS error means fix Kumo listener, reverse proxy, firewall, or TLS first.
- `/metrics`: should return Prometheus-style metrics if exposed.

Do not treat this as delivery-ready. It only proves HTTP reachability.

**2. Fill This Screen**
Use:

```text
Enabled: unchecked first
Base URL: https://email.gitdate.ink
Injection path: /api/inject/v1
Metrics URL: https://email.gitdate.ink/metrics
Auth mode: Bearer token or HMAC, matching Kumo/reverse-proxy config
Auth secret: the Kumo injection auth secret
Webhook secret: the secret Kumo will use when calling BillionMail
TLS verification: on if email.gitdate.ink has a valid cert
Default pool: shared-default or your real Kumo pool name
Request timeout: 5000
```

Click **Test** before enabling sending.

**3. Interpret Test Results**

- `KumoMTA reachable`: connection/auth/path are okay.
- `authentication or source allowlist failed`: auth secret is wrong or Kumo/reverse proxy is blocking `159.89.33.85`.
- `unexpected HTTP status 404`: injection path is wrong.
- timeout/TLS error: DNS, firewall, reverse proxy, cert, or Kumo listener problem.

After Test passes, check:

- `Use KumoMTA for outbound campaign/API sending`
- `Send API`
- optionally `Campaign sending`

Then click **Save changes**, then **Refresh**.

**4. Test Webhook Receiver Manually**
Use the webhook secret you saved:

```bash
curl -k -X POST https://mail.gitdate.ink/api/kumo/events \
  -H 'Content-Type: application/json' \
  -H 'X-BM-Kumo-Token: YOUR_WEBHOOK_SECRET' \
  -d '{
    "events": [
      {
        "event_id": "manual-test-1",
        "event_type": "delivered",
        "timestamp": 1778600000,
        "message_id": "<manual-test@example.com>",
        "recipient": "test@example.com",
        "sender": "news@gitdate.ink",
        "queue": "manual:tenant_1@example.com",
        "response": "250 2.0.0 manual test"
      }
    ]
  }'
```

Expected:

- response shows accepted event
- Webhook Health `Accepted` increments
- if message does not exist, it may be stored as orphaned, which is fine for this test.

**5. Preview Kumo Policy**
Click **Preview config**.

Check generated files for:

- webhook URL: `https://mail.gitdate.ink/api/kumo/events`
- tenant/pool mapping
- DKIM policy placeholders/warnings
- no secrets printed

Click **Dry run deploy** only to validate/record the preview. This does not mutate KumoMTA.

**6. Configure Kumo To Send Webhooks**
On the Kumo side, configure log/webhook delivery to:

```text
https://mail.gitdate.ink/api/kumo/events
```

Use one supported auth method:

- `Authorization: Bearer YOUR_WEBHOOK_SECRET`
- or `X-BM-Kumo-Token: YOUR_WEBHOOK_SECRET`
- or HMAC headers:
  - `X-BM-Kumo-Timestamp`
  - `X-BM-Kumo-Signature`

**7. Test Send API Path**
In BillionMail:

1. Create/choose an email template.
2. Create/choose a Send API template.
3. Set delivery engine to `KumoMTA` or tenant default.
4. Use a sender domain/address that matches the API template.
5. Copy the API key.

Then send:

```bash
curl -k -X POST https://mail.gitdate.ink/api/batch_mail/api/send \
  -H 'Content-Type: application/json' \
  -H 'X-API-Key: YOUR_API_KEY' \
  -d '{
    "recipient": "your-test-inbox@example.com",
    "attribs": {
      "name": "Test User"
    }
  }'
```

Expected response:

- `accepted: true`
- `status: queued`
- `engine: kumomta`
- `injection_status: pending` initially, then worker changes it to `queued`
- `delivery_status: pending`

In the KumoMTA screen, Runtime Controls should show injection attempts.

**8. Confirm Event-Driven Delivery**
After Kumo sends a webhook, the Send API log should move:

```text
injection_status: queued
delivery_status: delivered / deferred / bounced / expired / complained
```

Important: Kumo HTTP `2xx` only means queued. It does not mean delivered.

**9. Do Not Test Production Direct-To-MX From DO**
Your Kumo droplet is on DigitalOcean. Treat direct-to-MX production sending as blocked unless you have verified outbound SMTP is allowed.

For real production delivery, configure one of:

- external KumoProxy/SOCKS5
- external Kumo egress node
- provider HTTP API route
- provider SMTP submission on allowed port like `2525`

Until then, test only:

- Kumo HTTP injection
- queue creation
- webhook ingestion
- lifecycle updates in BillionMail.
