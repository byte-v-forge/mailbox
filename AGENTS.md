# AGENTS.md

- 本仓库只承载 mailbox 业务能力、provider adapter、业务内部契约和 mailbox 服务实现。
- 公共契约来自 `contracts`；provider 私有 shape 留在本仓库内部 proto。
- 后端优先使用 Go，按 Clean Code、DI 和面向抽象设计组织代码。
- 生成物不提交到仓库；提交前确认 `gen/` 和其他可再生成产物没有进入暂存区。
- proto 变更后必须运行生成命令、格式化和 Go 测试。
