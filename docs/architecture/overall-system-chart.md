# Overall System Chart

This page gives the full mental model of Pinchtab in one place.

It combines:
- the primary user path
- the server/bridge runtime split
- managed vs attached instances
- current routing and execution
- current security layer and future scheduling layer

## Chart 1: Overall Product Shape

```mermaid
flowchart TD
    U["User / Agent / Tool"] --> S["Pinchtab Server"]

    S --> D["Dashboard / Profiles / Instance API"]
    S --> R["Routing / Orchestrator"]
    S --> X["Shorthand Browser API"]

    R --> M1["Managed Instance"]
    R --> A1["Attached Instance"]

    M1 --> B["pinchtab bridge"]
    B --> C1["Chrome"]
    C1 --> T1["Tabs"]

    A1 --> C2["External Chrome"]
    C2 --> T2["Tabs"]
```

## Chart 2: Primary Usage Path

```mermaid
flowchart LR
    I["Install Pinchtab"] --> P["Run: pinchtab"]
    P --> L["Local server on localhost:9867"]
    L --> A["Agent / Tool connects over HTTP"]
    A --> W["Browser work happens through Pinchtab"]
```

This is the primary user journey.

The normal user should think:
- install Pinchtab
- run `pinchtab`
- point the client at `http://localhost:9867`

They should not need to think about `pinchtab bridge` directly.

## Chart 3: Runtime Types

```mermaid
flowchart TD
    I["Instance"] --> S{"source"}

    S --> M["managed"]
    S --> A["attached"]

    M --> R1{"runtime"}
    A --> R2{"runtime"}

    R1 --> B["bridge"]
    R1 --> D["direct-cdp"]

    R2 --> D2["direct-cdp"]
```

Interpretation:
- `source` = who introduced the instance
- `runtime` = how the server reaches the browser

## Chart 4: Current And Future Execution Layers

```mermaid
flowchart LR
    U["Agent Request"] --> P["Policy / Security Layer"]
    P --> Q["Task Scheduling Layer"]
    Q --> O["Orchestrator / Routing"]
    O --> E["Bridge Executor or Direct CDP"]
    E --> C["Chrome Tab"]
```

Meaning:
- **Policy / Security** decides whether a request should be admitted or sanitized
- **Task Scheduling** decides when and where admitted work should run
- **Orchestrator / Routing** decides which instance/tab path is used
- **Executor** performs the actual tab work

Today:
- execution exists
- routing exists
- security exists as a real IDPI defense layer
- scheduling is still mostly direct execution plus concurrency control

## Chart 4A: Current Security Layer

```mermaid
flowchart TD
    R["Request"] --> N{"Navigation?"}
    N -->|Yes| D["IDPI domain policy"]
    N -->|No| X["Continue"]

    D --> D2{"Allowed?"}
    D2 -->|No, strict| B["Block"]
    D2 -->|No, warn| W["Add warning header"]
    D2 -->|Yes| X

    X --> O{"Output content?"}
    O -->|Text or Snapshot| S["IDPI content scan"]
    O -->|Other| E["Execute / Return"]

    S --> S2{"Threat?"}
    S2 -->|No| E
    S2 -->|Yes, strict| B2["Block response"]
    S2 -->|Yes, warn| W2["Add warning metadata"]

    W2 --> WR{"Wrap text?"}
    WR -->|Yes| WT["Wrap as untrusted_web_content"]
    WR -->|No| E
    WT --> E
```

Current implementation shape:

- navigation checks happen before the tab opens or re-navigates
- content scanning happens on `/text` and `/snapshot`
- text wrapping is applied to `/text` output when enabled
- strict mode blocks, warn mode annotates

## Chart 5: Current Execution Model

```mermaid
flowchart TD
    H["HTTP Request"] --> O["Orchestrator / Server"]
    O --> T["Target Tab"]
    T --> L["Per-tab lock"]
    L --> G["Global concurrency limit"]
    G --> E["Execute CDP action"]
```

This is the current model already present in the product:
- per-tab sequential execution
- bounded cross-tab parallelism
- direct request execution

That means the current architecture already has:

- a **security layer** for admission and output safety
- an **execution layer** for concurrency correctness

What it does not yet have as a first-class subsystem is:

- a true **task scheduling layer** with queued work, fairness, and dispatch policy

## Chart 6: Recommended Future Model

```mermaid
flowchart TD
    H["HTTP Request"] --> A["Admission"]
    A --> B{"Allowed?"}
    B -->|No| X["Reject"]
    B -->|Yes| Q["Queue"]
    Q --> S["Scheduler"]
    S --> R["Instance / Tab selection"]
    R --> E["Executor"]
    E --> C["Chrome"]
```

This is the cleaner model for multi-agent and parallel execution:
- requests become tasks
- tasks are admitted or rejected
- admitted tasks are queued
- the scheduler chooses what runs next
- existing executors still enforce per-tab correctness

## Reading Guide

Use the charts like this:

- **Chart 1** for the product overview
- **Chart 2** for onboarding and default usage
- **Chart 3** for instance taxonomy
- **Chart 4** for control-plane layering
- **Chart 5** for how execution works today
- **Chart 6** for the likely future scheduling architecture

## Related Docs

- [Architecture](pinchtab-architecture.md)
- [Instance Model Charts](instance-model-charts.md)
- [Managed Bridge vs Managed Direct-CDP](managed-bridge-vs-managed-direct-cdp.md)
- [Expert Guide: Attach](../guides/expert-attach.md)
- [Expert Guide: Multi-Instance Strategies](../guides/expert-strategies.md)
