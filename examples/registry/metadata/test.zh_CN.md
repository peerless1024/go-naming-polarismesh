[English](test.md) | 中文

# 注册 metadata 测试

本文档与 [verify.sh](verify.sh) 保持同步。

### [case 1.1 dynamic metadata registration]

操作：启动一个启用 `RegisterMetadataProvider` 的 Provider，并通过 Polaris
OpenAPI 查询实例。

预期：

- 恰好注册一个实例；
- 实例 host 非空，port 为 `28271`；
- 实例 version 为 `v1.2.3`；
- 保留静态 metadata `registry.static=provider-yaml`；
- 包含动态 metadata `registry.runtime=dynamic` 和 `registry.version=v1.2.3`；
- 包含动态 AI metadata `ai-agent-name`、`a2a.agent.card`、`a2a.transport`
  和 `a2a.path`。

### [case 1.2 reserved metadata key conflict]

操作：启动第二个 Provider，其静态 metadata 包含
`ai-agent-name=static-agent`，同时 metadata provider 返回
`ai-agent-name=dynamic-registry-agent`。

预期：注册在创建第二个实例前失败，日志包含 `reserved agent key`。

## 清理

`verify.sh` 会保留动态 Provider。默认 cleanup 停止进程并删除本地产物；增加
`--delete-service --polaris-server` 才会删除远端服务。
