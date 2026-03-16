# System Charts

This page collects the main high-level charts for the current PinchTab architecture.

## Chart 1: Product Shape

```mermaid
flowchart TD
    U["Agent / CLI / Tool"] --> S["PinchTab Server"]

    S --> D["Dashboard + Config + Profiles API"]
    S --> O["Orchestrator + Strategy Layer"]

    O --> M1["Managed Instance"]
    O --> M2["Managed Instance"]

    M1 --> B1["pinchtab bridge"]
    M2 --> B2["pinchtab bridge"]

    B1 --> C1["Chrome"]
    B2 --> C2["Chrome"]

    C1 --> T1["Tabs"]
    C2 --> T2["Tabs"]

    S -. "advanced attach path" .-> E["Registered External Chrome"]
```

This is the default system shape today:

- agents talk to the server over HTTP
- the server manages profiles, instances, and routing
- managed instances are bridge-backed
- attach exists as an advanced external-browser registration path

## Chart 2: Primary Usage Path

```mermaid
flowchart LR
    I["Install PinchTab"] --> R["Run: pinchtab server"]
    R --> L["Local server on localhost:9867"]
    L --> A["Agent / CLI sends HTTP requests"]
    A --> W["Browser work happens through PinchTab"]
```

This is the normal mental model for users. Most users should think about `pinchtab server`, not `pinchtab bridge`.

## Chart 3: Runtime Shapes

```mermaid
flowchart LR
    subgraph S1["Server Mode"]
        C1["Client"] --> P1["pinchtab server"]
        P1 --> B1["pinchtab bridge"]
        B1 --> CH1["Chrome"]
        CH1 --> T1["Tabs"]
    end

    subgraph S2["Bridge Mode"]
        C2["Client"] --> B2["pinchtab bridge"]
        B2 --> CH2["Chrome"]
        CH2 --> T2["Tabs"]
    end
```

Meaning:

- **server mode** is the multi-instance control-plane path
- **bridge mode** is the single-instance browser runtime

## Chart 4: Current Request Paths

```mermaid
flowchart TD
    R["HTTP Request"] --> M["Auth + Middleware"]
    M --> T{"Route Type"}

    T -->|Direct browser route| X["Tab / Instance Resolution"]
    T -->|Task route, when enabled| Q["Scheduler"]
    T -->|Attach route| A["Attach Policy Check"]

    Q --> X
    X --> H["Bridge Handler"]
    H --> P["Handler-Level Policy Checks"]
    P --> C["Chrome via CDP"]
    C --> O["JSON / Text / PDF / Image Response"]

    A --> AR["Register External Instance"]
```

Important details:

- auth and shared middleware run at the HTTP layer
- attach policy is enforced on the attach route in the server
- IDPI and similar browser-facing checks run in handlers such as navigation, text, and snapshot
- tab-scoped routes are resolved to the owning instance before execution
- the scheduler is optional, server-only, and applies to `/tasks`
- bridge handlers perform the actual browser work
