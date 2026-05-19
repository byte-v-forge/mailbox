# cloudflare-email-relay

Cloudflare Email Routing Worker that forwards normalized inbound email events to the mailbox webhook.

## Runtime config

- `MAILBOX_WEBHOOK_URL`: mailbox webhook endpoint.
- `MAILBOX_WEBHOOK_TOKEN`: secret sent as `X-Webhook-Token`.
- `WEBHOOK_FAIL_OPEN`: when `true`, accepts the email even if webhook forwarding fails.
- `TELEGRAM_BOT_TOKEN`: optional secret; enables Telegram notifications when paired with `TELEGRAM_CHAT_ID`.
- `TELEGRAM_CHAT_ID`: optional secret; enables Telegram notifications when paired with `TELEGRAM_BOT_TOKEN`.

Telegram delivery is best-effort and does not affect mailbox webhook delivery.

## Run

```sh
npm install
npm run deploy
```

Set secrets:

```sh
wrangler secret put MAILBOX_WEBHOOK_TOKEN
wrangler secret put TELEGRAM_BOT_TOKEN
wrangler secret put TELEGRAM_CHAT_ID
```
