[English](README.md) | 中文

# 规则路由

通过 Polaris v2 routing rule 将指定 source service 路由到
`route=canary` 的 provider。

```bash
./examples/route/rule/verify.sh --polaris-server <polaris-server-host>
```

测试同时验证 source service 和 destination label；其他 caller 会正常回退到普通
负载均衡。

部分 Polaris 部署会以 `403002` 拒绝删除规则；脚本会先禁用同名旧规则再创建。
