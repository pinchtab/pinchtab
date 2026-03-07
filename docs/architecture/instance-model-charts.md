# Instance Model Charts

This page collects the visual mental model for Pinchtab instances.

It complements:
- [Architecture](pinchtab-architecture.md)
- [Managed Bridge vs Managed Direct-CDP](managed-bridge-vs-managed-direct-cdp.md)

## Chart 1: Current High-Level Model

```mermaid
flowchart LR
    U["User / Agent"] --> S["Pinchtab Server"]

    S --> I1["Managed Instance"]
    I1 --> B["Bridge process"]
    B --> C["Chrome"]
    C --> T["Tabs"]

    S -. attach .-> E["External Chrome"]
    E --> T2["Tabs"]
```

## Chart 2: Instance Kinds

```mermaid
flowchart TD
    I["Instance"] --> K{"Kind"}

    K --> A["attached-external"]
    K --> B["managed-bridge"]
    K --> C["managed-direct-cdp"]
    K --> D["adopted / handover"]

    A --> A1["Pinchtab did not launch browser"]
    B --> B1["Pinchtab launched bridge process"]
    C --> C1["Pinchtab launched browser directly"]
    D --> D1["Browser was created elsewhere, then handed to server"]
```

## Chart 3: Communication Paths

```mermaid
flowchart LR
    S["Pinchtab Server"]

    S -->|HTTP| B["Bridge instance"]
    B -->|CDP| C1["Chrome"]

    S -->|CDP| C2["Direct-CDP instance"]

    S -->|CDP| C3["Attached external Chrome"]
```

## Chart 4: Lifecycle Ownership

```mermaid
flowchart TD
    M["Instance lifecycle"] --> L1["Launch by Pinchtab"]
    M --> L2["Attach existing browser"]
    M --> L3["Adopt then manage"]
    M --> L4["Create then hand over"]

    L1 --> X1["Pinchtab owns start/stop"]
    L2 --> X2["Pinchtab routes, but may not own process"]
    L3 --> X3["Pinchtab takes over ownership after attach"]
    L4 --> X4["Pinchtab creates browser, then another controller owns it"]
```

## Chart 5: Recommended Taxonomy

```mermaid
flowchart TD
    S["Pinchtab Server / Control Plane"]

    S --> R["Instance Registry"]

    R --> I1["managed-bridge"]
    R --> I2["managed-direct-cdp"]
    R --> I3["attached-external"]

    I1 --> P1["Transport: HTTP -> bridge -> CDP"]
    I2 --> P2["Transport: direct CDP"]
    I3 --> P3["Transport: direct CDP"]

    P1 --> T1["Tabs"]
    P2 --> T2["Tabs"]
    P3 --> T3["Tabs"]
```

## Recommended Reading Of These Charts

The cleanest interpretation is:

- `source` answers who introduced the instance
- `runtime` answers how Pinchtab reaches the browser
- `ownership` answers who controls lifecycle

For the current architecture, the useful combinations are:

- `managed + bridge + pinchtab`
- `attached + direct-cdp + external`

And the main future option is:

- `managed + direct-cdp + pinchtab`
