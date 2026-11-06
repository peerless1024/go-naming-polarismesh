[English](README.md) | 中文

# 熔断

INSTANCE、SERVICE、METHOD 和 HalfOpen 的独立真实服务用例。

```bash
./examples/circuitbreaker/verify.sh --polaris-server <polaris-server-host>
```

用例：

1. 终止 provider-b，验证 INSTANCE failover；
2. 重启 provider-b，验证 HalfOpen 恢复；
3. SERVICE 规则达到阈值后拒绝整个服务；
4. METHOD 规则只拒绝指定 method。

脚本会创建 caller-specific 规则，并在清理时删除或禁用。

E2E client 设置了 `WithShouldErrReportToSelector`，因此复用 TCP 连接时出现的
`RetClientReadFrameErr` 会作为下游失败上报。业务应用也可以在 client 配置中
使用等价的 `report_any_err_to_selector: true`。
