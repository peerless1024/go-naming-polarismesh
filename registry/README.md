# Service Registry Plugin

## How to use
```go
import _ "trpc.group/trpc-go/trpc-naming-polarismesh/registry"
```

## A complete Configuration
```yaml
plugins:  # tRPC-Go plugin configuration.
  registry:  # Registry has its own plugin type.
    polarismesh:  # This Registry is based on polaris mesh.
      register_self: true  # Whether to register, default as false.
      heartbeat_interval: 3000  # The interval to report heartbeat, must be provided.
      debug: true  # Whether to enable the debug log of polaris mesh sdk, default as false.
      instance_location:  # The location of the registered instance.
        region: China
        zone: Guangdong
        campus: Shenzhen
      service:
        - name:  trpc.test.helloworld.Greeter1  # The name of service.
          namespace: namespace-test1  # The namespace of your service.
          version: v1.2.3  # Optional instance version.
          # (Optional) Used to heartbeat or unregister.
          # When register_self is true, this config has no effect, the plugin will use returned instance_id of register to overwrite config.
          # if register_self is false, instance_id cannot be missing.
          instance_id: yyyyyyyyyyyyyyyy
          bind_address: eth1:8080  # Specify the listening address of the service.
          weight: 100  # Default weight is 100.
          metadata:  # Custom metadata when registering.
            # Enable set (both this line and the next line need to be set to fully enable set).
            internal-enable-set: Y
            internal-set-name: xx.yy.sz  # Set service set name.
            key1: val1  # For other metadata, etc., please refer to polaris mesh related documents.
            key2: val2
```

## Dynamic Instance Metadata

Use `RegisterMetadataProvider` when instance metadata is produced at runtime, such as an AgentCard.
The provider is keyed by the service name and is called by every registration attempt. The result is
merged over the static `metadata` configuration; a provider error fails registration before a
heartbeat starts.

```go
import (
    "fmt"

    trpc "trpc.group/trpc-go/trpc-go"
    polarisregistry "trpc.group/trpc-go/trpc-naming-polarismesh/registry"
)

func prepareAgentMetadata(service string) error {
    provider := polarisregistry.MetadataProviderFunc(func() (map[string]string, error) {
        cardJSON := `{"name":"customer-agent","version":"v1.2.3","url":"http://127.0.0.1:8000/a2a"}`
        return map[string]string{
            "ai-agent-name":  "customer-agent",
            "a2a.agent.card": cardJSON,
            "a2a.transport":  "JSONRPC",
            "a2a.path":       "/a2a",
        }, nil
    })
    return polarisregistry.RegisterMetadataProvider(service, provider)
}

func main() {
    s := trpc.NewServer()
    service := "customer-agent"
    if err := prepareAgentMetadata(service); err != nil {
        panic(err)
    }
    defer polarisregistry.UnregisterMetadataProvider(service)

    if err := s.Serve(); err != nil {
        panic(err)
    }
}
```

The provider must be registered before `Serve` triggers `Registry.Register`. Do not add `ai-agent-name`,
`a2a.agent.card`, `a2a.transport`, or `a2a.path` to the static metadata while a provider is registered;
the plugin rejects this configuration to avoid two competing sources. Registration accepts at most 64
metadata entries; keys must be non-empty and values must not exceed 65535 Unicode characters.

## Reusing the Registry SDK Context

`FromFramework` returns the registry created by the tRPC-Go plugin. Reuse its `SDKContext` for
registry-side extensions instead of creating a second Polaris SDK context.

```go
r, ok := polarisregistry.FromFramework("customer-agent")
if !ok {
    return fmt.Errorf("polaris registry is not initialized")
}
sdkCtx := r.SDKContext()
instanceVersion := r.Version()
// Build extension APIs from sdkCtx and pass sdkCtx to the adapter.
```

Use this shutdown order: stop accepting new requests, let tRPC deregister the instance, call
`UnregisterMetadataProvider`, then release adapter-owned resources. The provider is not cached, so a
later registration gets the latest value returned by `Metadata`.
