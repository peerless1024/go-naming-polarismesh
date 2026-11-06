English | [Chinese](test.zh_CN.md)

# Registry Metadata Test

This document is kept in sync with [verify.sh](verify.sh).

### [case 1.1 dynamic metadata registration]

Operation: start one provider with `RegisterMetadataProvider` enabled and query
its instance through the Polaris OpenAPI.

Expected:

- exactly one instance is registered;
- instance host is non-empty and port is `28271`;
- instance version is `v1.2.3`;
- static metadata `registry.static=provider-yaml` is retained;
- dynamic metadata `registry.runtime=dynamic` and `registry.version=v1.2.3`
  are present;
- dynamic AI metadata `ai-agent-name`, `a2a.agent.card`, `a2a.transport` and
  `a2a.path` is present.

### [case 1.2 reserved metadata key conflict]

Operation: start a second provider whose static metadata contains
`ai-agent-name=static-agent` while the metadata provider returns
`ai-agent-name=dynamic-registry-agent`.

Expected: registration fails before the second instance is created; the log
contains `reserved agent key`.

## Cleanup

`verify.sh` retains the dynamic provider. Default cleanup stops it and removes
local artifacts. Add `--delete-service` with `--polaris-server` to delete the
remote service.
