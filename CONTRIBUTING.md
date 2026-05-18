# 贡献指南

## 边界

本仓只承载 mailbox 业务能力、provider adapter、业务内部 proto 和 mailbox 服务实现。

以下内容不进入本仓：

- 公共 mailbox 契约源文件；
- 其他业务域代码；
- 真实邮箱账号、真实邮件内容或真实 provider 凭据；
- 生成代码。

## 开发流程

1. 公共能力先修改 `contracts/mailbox`，再同步 `contracts-go`。
2. provider 私有 shape 放在 `proto/byte/v/forge/mailbox/providers/<provider>/v1/`。
3. 业务内部模型放在 `proto/byte/v/forge/mailbox/internal/v1/`。
4. 外部 provider 调用必须设置超时，并按 provider 文档实现状态和错误映射。

## 验证

```sh
sh scripts/generate-proto.sh
go test ./...
go vet ./...
```

`gen/` 是本地生成目录，不提交。
