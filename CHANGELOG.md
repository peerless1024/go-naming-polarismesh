# Change Log

## v0.3.0 - 2026-11-04

### Added

- Add per-service dynamic metadata providers for instance registration.
- Merge dynamic metadata over static metadata with size and reserved-key validation.
- Add optional service instance `version` configuration.
- Expose the registration-side Polaris SDKContext and lookup through `FromFramework`.
- Add runnable real E2E provider/client examples for registration, discovery, dynamic metadata,
  load balancing, circuit-breaker failover and graceful deregistration.
- Add a canonical HelloWorld project plus feature-focused examples for load-balancer
  algorithms, INSTANCE/SERVICE/METHOD circuit breaking, metadata routing, rule routing and
  multi-context selector isolation.

### Changed

- Upgrade polaris-go to the AI Registry development baseline and regenerate mocks for the SDKContext
  and composite circuit-breaker APIs.
- Align circuit-breaker defaults with polaris-go's composite circuit breaker. Legacy bucket-count
  settings remain accepted but are no longer applied by the upstream SDK.

### Fixed

- Include half-open instances in load-balancing candidates and retry only after probe quota is denied.
- Preserve recover-all routing when the remaining instances are Open.
- Carry the caller service through selection and circuit-breaker reporting.
- Reject legacy single-strategy circuit-breaker chains instead of silently enabling both composite triggers.
