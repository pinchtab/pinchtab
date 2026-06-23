# Implementations

This section covers implementation-focused documents: how specific subsystems work in practice, what tradeoffs they make, and how the current code is structured.

Use these pages when you want lower-level detail than the architecture overview, but do not need full API reference material.

- [Static Fetch (Lite Engine)](./lite-engine.md) — Chrome-free DOM capture using Gost-DOM (the lightweight HTTP+DOM path used by the `ghost-chrome` provider before escalating to Chrome; "lite engine" is deprecated terminology — see [terminology](../architecture/terminology.md))
- [Managed Bridge vs Managed Direct CDP](./managed-bridge-vs-managed-direct-cdp.md)
- [Chrome Profile Lock Recovery](./chrome-profile-lock-recovery.md)
- [Chrome Files](./chrome-files.md)
- [Parallel Tab Execution](./parallel-tab-execution.md)
- [Docker Local Testing](./docker-local-testing.md)
