# mailbox

Mailbox 领域仓，承载邮箱账号、Outlook provider、邮箱注册/OAuth MQ 编排和收件能力。

## 目录

- `services/mailbox-api`：Mailbox 领域 gRPC API 和唯一服务进程，内置 Outlook/Cloudflare provider adapter、邮箱注册/OAuth MQ worker、收件、webhook 和邮件信号解析能力。
- `Dockerfile`：部署入口，只构建并启动 mailbox API 一个进程。
- `workers/cloudflare-email-relay`：Cloudflare Email Routing Worker，将 CF 入站邮件转发到 mailbox webhook。
- `proto/email.proto`：邮件读取服务契约。
- `proto/mailbox_register.proto`：邮箱注册与 OAuth 编排模型。
- `proto/mailbox_service.proto`：Mailbox 领域 API 契约。

跨仓公开 mailbox 建模统一引用 `common-lib/proto/byte/v/forge/contracts/mailbox/v1/mailbox.proto`。
`mailbox` 仓内部 `email.proto` 可以保存 password、refresh/access token 等拥有方细节；
对外收件结果、provider capability 和 domain 投影只暴露 common-lib 公共模型与 `credential_state`。

## 生成

```sh
sh scripts/generate-proto.sh
```

生成物用于远程构建和部署验证，位于仓库忽略路径。

## 配置

`services/mailbox-api` 直接内置 Outlook 和 Cloudflare provider adapter，并通过 `MAILBOX_PG_DSN` 维护邮箱、邮件和操作状态投影。

Outlook 注册和 OAuth 通过 mailbox 内置 MQ worker 编排：`RegisterMailbox` / `RunMailboxOAuth` 只创建 operation 并发布 `mailbox.registration.operation_requested` / `mailbox.oauth.operation_requested` 到 `platform-nats`，由 mailbox worker 消费后调用 `browser-automation` 执行并更新 operation 投影。claim owner、lease、attempt count、OAuth limit/only_missing 只保存在 mailbox 内部表中，不进入 dashboard/API 对外 `MailboxOperation` 模型。

Outlook 注册/OAuth 浏览器 profile 通过 `BROWSER_AUTOMATION_ADDR`、`OUTLOOK_REGISTER_AUTOMATION_PROXY_REF`、`OUTLOOK_REGISTER_AUTOMATION_LOCALE` 和 `OUTLOOK_REGISTER_AUTOMATION_TIMEZONE` 配置。

Outlook 邮件读取使用 Microsoft Graph Go SDK，默认读取当前 OAuth 用户的 messages，并用 `Prefer: outlook.body-content-type="text"` 请求文本正文；只有显式覆盖 `OUTLOOK_GRAPH_MESSAGES_URL` 时才走兼容 REST adapter。

Cloudflare 邮件是主动推送链路：Email Routing Worker 收到邮件后把标准化事件 POST 到 `/webhooks/email/cloudflare`，mailbox 服务使用 `MAILBOX_WEBHOOK_HTTP_ADDR` 开启 HTTP webhook，并只通过 `X-Webhook-Token` 读取 `MAILBOX_WEBHOOK_TOKEN` 校验转发请求。Outlook Graph webhook 使用 `/webhooks/email/microsoft-graph`，验证 URL 必须带同一个 token，POST 通知的 `clientState` 也必须等于同一个 token。Cloudflare 域名池来自 Cloudflare API：`MAILBOX_CLOUDFLARE_API_TOKEN` 读取 `MAILBOX_CLOUDFLARE_EMAIL_CONFIG_FILE` 中声明的 zones，并从 Email Routing catch-all 规则与 Cloudflare MX DNS 记录推导可用邮箱域名；token 限制到目标 zone，并授予 `Zone Read`、`DNS Read` 和 `Email Routing Rules Read` 即可。Cloudflare 地址不需要手动导入，邮件到达后按 recipient 自动形成虚拟邮箱并按 domain 分组展示。需要公网入口时在 deploy 的 `ingress.webhook` 暴露 mailbox webhook，或使用受管 HTTPS 隧道把该入口映射到公网域名；业务代码不管理公网隧道。

邮件保留策略按 provider 独立执行 FIFO：Outlook 使用 `MAILBOX_OUTLOOK_MAX_MESSAGES_PER_MAILBOX` 限定每个邮箱的最大邮件数，Cloudflare 使用 `MAILBOX_CLOUDFLARE_MAX_MESSAGES_PER_DOMAIN` 限定每个 domain 的最大邮件数。超过上限时删除最早邮件及对应 seen 记录，默认分别为 `100` 和 `500`。

邮件内容会先落库，再通过 mailbox 通用解析器生成 `EmailSignal`。通用解析器只识别验证码等可复用邮件信号；业务状态判断由业务服务通过 webhook 或查询读取邮件后自行完成。

`CACHE_REDIS_URL` 是 mailbox 运行时协调依赖：新入库邮件按邮箱写入业务 Redis 近期热缓存，`WaitForMailboxEmail` 只读近期缓存/PostgreSQL 投影；需要 Outlook provider 拉取时先发布 `mailbox.email.poll_requested` 到 `platform-nats`，由 mailbox poll worker 消费执行。UI 实时刷新通过 HotStream/NATS Core 的非持久化通知触发前端重新查询；`MAILBOX_INBOX_LOCK_KEY_PREFIX` 用于跨副本抓取锁和 Outlook webhook refresh 锁。Redis 仅作为热点读取与协调层，不作为邮件领域状态真源。

邮件入库会在同一 DB transaction 写入 `mailbox_platform_event_outbox`，再由 outbox worker 发布公共 `mailbox.email.received` / `mailbox.email.signal.received` 事件，避免邮件已落库但 NATS 临时失败导致下游投影丢失。`FetchMailboxInboxes` 只创建 operation 并发布 `mailbox.inbox.fetch_requested`，由 fetch worker 异步更新 operation 投影。mailbox 注册/OAuth operation、入站 poll/fetch 和公共事件 outbox 都依赖 `PLATFORM_NATS_URL`。业务服务需要消费邮箱事件时应订阅 platform events 并在自身服务内维护幂等投影，不再通过 mailbox outbound HTTP webhook 旁路投递。

## 检查

```sh
cd ../deploy
./scripts/deploy-remote.sh mailbox
```

业务构建、镜像构建和部署验证统一在远程宿主机执行，本机只做源码编辑和调度。
