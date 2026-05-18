# mailbox

Mailbox 业务服务。

本仓库负责邮箱账号、邮件文件夹、邮件搜索/读取、邮件状态变更，以及 provider adapter 的内部实现。

## 当前实现

- Go module：`github.com/byte-v-forge/mailbox`
- 公共契约 adapter：`internal/adapters/grpc`
- 核心应用服务：`internal/app`
- 领域模型和端口：`internal/core`
- 内存 store：`internal/app`
- provider 私有契约：
  - `proto/byte/v/forge/mailbox/internal/v1/mailbox_internal.proto`
  - `proto/byte/v/forge/mailbox/providers/imap/v1/imap.proto`
  - `proto/byte/v/forge/mailbox/providers/jmap/v1/jmap.proto`

## 契约边界

- 公共 mailbox 能力定义在 `contracts/mailbox`。
- 本仓只放业务内部契约和 provider 私有模型。
- `EmailAccount` 表示邮箱账号；`MailFolder` 表示 INBOX、Archive、Trash、Junk 等容器。
- `Mailbox` 不作为账号模型名称使用，避免和 IMAP/JMAP 的 mailbox/folder 概念冲突。
- OAuth token、cookie、IMAP 密码、provider raw response、验证码提取规则和注册业务流程不进入公共契约。

## 生成

```sh
sh scripts/generate-proto.sh
```

脚本会先生成 `../contracts` 的本地 Go 代码，再生成本仓内部 proto。
`gen/` 下的生成物是本地构建产物，不提交到仓库。

## 测试

```sh
go test ./...
go vet ./...
```

## 贡献与安全

- 贡献规则见 `CONTRIBUTING.md`。
- 安全报告规则见 `SECURITY.md`。
- 本仓使用 Apache-2.0 许可证。

## 尚未实现

- PostgreSQL 持久化 adapter。
- 运行时配置加载和 secret 解析。
- 进程入口和 gRPC server bootstrap。
- 真实 IMAP/JMAP/Gmail/Graph provider adapter。
