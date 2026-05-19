# cloudflare-email-relay

Cloudflare Email Routing Worker that forwards normalized inbound email events to the mailbox webhook.

## Runtime config

- `MAILBOX_WEBHOOK_URL`: mailbox webhook endpoint.
- `MAILBOX_WEBHOOK_TOKEN`: secret sent as `X-Webhook-Token`.
- `WEBHOOK_FAIL_OPEN`: when `true`, accepts the email even if webhook forwarding fails.

## Run

```sh
npm install
npm run deploy
```
