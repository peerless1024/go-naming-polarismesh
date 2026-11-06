English | [Chinese](README.zh_CN.md)

# Rule Route

Creates a Polaris v2 routing rule that sends one source service to providers
labeled `route=canary`.

```bash
./examples/route/rule/verify.sh --polaris-server <polaris-server-host>
```

The test proves both source service and destination label matching. Another
caller falls back to normal load balancing.

Some Polaris deployments reject rule deletion with `403002`; the script
disables stale rules before creating a new one.
