English | [Chinese](README.zh_CN.md)

# Registry Metadata

This example focuses only on registration-side metadata:

- dynamic `RegisterMetadataProvider` values;
- merge with static YAML metadata;
- registered instance version;
- reserved Agent metadata conflict fail-fast.

```text
registry/metadata/
|-- provider/main.go
|-- provider/trpc_go.yaml
|-- provider/trpc_go-conflict.yaml
|-- verify.sh
|-- cleanup.sh
|-- test.md
`-- test.zh_CN.md
```

## Run

Dynamic metadata mode:

```bash
cd examples/registry/metadata/provider

POLARIS_SERVER=<polaris-host> go run . \
  -conf trpc_go.yaml \
  -metadata-mode dynamic
```

Conflict mode is expected to fail before registration:

```bash
POLARIS_SERVER=<polaris-host> go run . \
  -conf trpc_go-conflict.yaml \
  -metadata-mode conflict \
  -port 28272
```

The expected error contains `reserved agent key`.

## Real E2E

```bash
./examples/registry/metadata/verify.sh --polaris-server <polaris-host>
./examples/registry/metadata/cleanup.sh
./examples/registry/metadata/cleanup.sh --delete-service --polaris-server <polaris-host>
```

Verification retains the dynamic provider. Default cleanup stops processes and
removes local artifacts; `--delete-service` removes the remote service.
