English | [Chinese](README.zh_CN.md)

# Circuit Breaker

Independent real-service cases for INSTANCE, SERVICE, METHOD and HalfOpen.

```bash
./examples/circuitbreaker/verify.sh --polaris-server <polaris-server-host>
```

Cases:

1. kill provider-b and verify INSTANCE failover;
2. restart provider-b and verify HalfOpen recovery;
3. SERVICE rule rejects the whole service after the threshold;
4. METHOD rule rejects only the configured method.

The script creates caller-specific rules and removes or disables them during
cleanup.

The E2E client sets `WithShouldErrReportToSelector` so `RetClientReadFrameErr`
from a reused TCP connection is reported as a downstream failure. Applications
can use the equivalent `report_any_err_to_selector: true` client setting.
