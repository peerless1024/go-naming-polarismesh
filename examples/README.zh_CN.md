[English](README.md) | 中文

# 示例

| 示例 | 说明 |
| --- | --- |
| [helloworld](./helloworld) | 标准 proto、生成桩、Server/Client 和 Polaris YAML |
| [loadbalancer](./loadbalancer) | WR、hash、ring hash 和 maglev 的可观测断言 |
| [circuitbreaker](./circuitbreaker) | INSTANCE、SERVICE、METHOD 和 HalfOpen 恢复 |
| [registry/metadata](./registry/metadata) | 动态 metadata 合并、version 和保留 key 冲突 |
| [route/metadata](./route/metadata) | destination metadata 和 callee environment 路由 |
| [route/rule](./route/rule) | source service 到 destination label 的规则路由 |
| [multiContext](./multiContext) | 命名 Selector 和 SDKContext 隔离 |

真实 E2E 必须通过 `--polaris-server` 或 `POLARIS_SERVER` 提供 Polaris 地址，
并使用共享命名空间 `go-naming-polarismesh-e2e`。
Provider 会从连接 Polaris 的 TCP socket 获取本端 IP，不使用固定回环地址。
验证结束后会保留进程、日志和远端资源供排查，清理需要单独执行。测试用例和预期结果见
[registry/metadata/test.zh_CN.md](./registry/metadata/test.zh_CN.md)。

如果本示例进程仍在运行，或目标服务仍有注册实例，verify 会在启动前失败。异常中断后
应先使用 `cleanup.sh --delete-service` 清理，再重新验证。

依次运行全部示例，包括 HelloWorld 和六组功能示例：

```bash
./examples/verify_all.sh --polaris-server <polaris-server-host>
```

验证结束后，停止全部示例进程并清理本地产物：

```bash
./examples/cleanup.sh
```

默认不会删除远端服务和规则。需要显式清理时：

```bash
./examples/cleanup.sh --delete-service \
  --polaris-server <polaris-server-host>
```

第一次使用时，请优先阅读源码形式的
[helloworld](./helloworld/README.zh_CN.md)。`verify.sh` 只是对同一运行链路的
自动化验证，清理由独立 cleanup 脚本负责。
