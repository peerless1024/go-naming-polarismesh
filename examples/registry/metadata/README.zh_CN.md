[English](README.md) | 中文

# 注册 metadata

本示例只聚焦注册侧 metadata：

- `RegisterMetadataProvider` 动态值；
- 与 YAML 静态 metadata 合并；
- 注册实例 version；
- Agent 保留 metadata 冲突时 fail-fast。

```text
registry/metadata/
├── provider/main.go
├── provider/trpc_go.yaml
├── provider/trpc_go-conflict.yaml
├── verify.sh
├── cleanup.sh
├── test.md
└── test.zh_CN.md
```

## 运行

动态 metadata 模式：

```bash
cd examples/registry/metadata/provider

POLARIS_SERVER=<polaris-host> go run . \
  -conf trpc_go.yaml \
  -metadata-mode dynamic
```

冲突模式预期在注册前失败：

```bash
POLARIS_SERVER=<polaris-host> go run . \
  -conf trpc_go-conflict.yaml \
  -metadata-mode conflict \
  -port 28272
```

预期错误包含 `reserved agent key`。

## 真实 E2E

```bash
./examples/registry/metadata/verify.sh --polaris-server <polaris-host>
./examples/registry/metadata/cleanup.sh
./examples/registry/metadata/cleanup.sh --delete-service --polaris-server <polaris-host>
```

验证会保留动态 Provider。默认 cleanup 停止进程并删除本地产物；
`--delete-service` 才会删除远端服务。
