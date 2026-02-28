# Optimization & Scaling

## Overview

Pinchtab multi-instance architecture is production-ready. Phase 7 focuses on optimizing performance, establishing baseline metrics, and validating system behavior under high load.

## Goals

- Establish performance baselines (latency, memory, throughput)
- Optimize critical paths (connection pooling, caching, batch operations)
- Validate scalability (100+ concurrent instances)
- Document resource requirements and limits

---

## Section 1: Benchmarking & Metrics

### Objectives

Capture baseline performance metrics across single and multi-instance scenarios.

### Tasks

#### 1.1 Request Latency Baseline

- [ ] **Measure endpoint latency (p50, p95, p99)**
  - `/navigate` - first request (Chrome init) vs subsequent
  - `/snapshot` - element retrieval time
  - `/actions` (click, type, etc.) - per action cost
  - `/tabs` aggregate - response time with N instances

- [ ] **Tools & Methods**
  - Use `ab` (Apache Bench) for load testing
  - Use `vegeta` (Go HTTP load testing) for sustained load
  - Capture detailed timing with custom harness
  - Document environment: CPU, RAM, disk speed

- [ ] **Acceptance Criteria**
  - Single instance: `/navigate` < 2s (first request)
  - Single instance: `/navigate` < 200ms (subsequent)
  - 10 instances: `/tabs` aggregate < 500ms
  - All operations p99 < 5s

#### 1.2 Memory per Instance

- [ ] **Measure baseline memory footprint**
  - Idle Chrome process (headless)
  - Idle Chrome process (headed)
  - Memory after 10 navigations
  - Memory after 100 navigations
  - Memory leak detection over 1 hour

- [ ] **Tools & Methods**
  - Use `pmap` or `/proc/[pid]/status` on Linux
  - Use `Instruments.app` or `Activity Monitor` on macOS
  - Track RSS (resident set size) and VSZ (virtual size)
  - Profile heap with `pprof`

- [ ] **Acceptance Criteria**
  - Idle instance: < 150 MB RSS
  - 10 instances: < 2 GB total RSS
  - No memory growth > 5 MB/hour idle
  - No memory growth > 20 MB/100 navigations

#### 1.3 Concurrent Instance Limits

- [ ] **Identify breaking points**
  - How many instances can run concurrently? (CPU bound? File descriptor bound? Memory?)
  - How many concurrent requests per instance?
  - Orchestrator overhead as instance count grows

- [ ] **Tools & Methods**
  - Create instances in loop, monitor system metrics
  - Run concurrent navigation requests across instances
  - Use `ulimit` and `/proc/sys/fs/file-max` to check limits
  - Profile orchestrator CPU usage

- [ ] **Acceptance Criteria**
  - 100 instances should be sustainable with 16GB RAM
  - Orchestrator should handle 1000 concurrent requests
  - Port allocation should handle allocation/release in < 10ms
  - No file descriptor leaks (connections cleaned up properly)

---

## Section 2: Optimizations & Improvements

### Objectives

Implement targeted performance improvements based on bottleneck analysis.

### Tasks

#### 2.1 Connection Pooling

- [ ] **HTTP connection reuse**
  - Orchestrator to instances: Keep-Alive connections
  - Bridge handlers: Connection pools for concurrent requests
  - Measure impact on latency (target: 10-20% improvement)

- [ ] **Implementation Strategy**
  - Update `proxyToInstance()` to use persistent HTTP client
  - Configure http.Client with max idle connections, timeouts
  - Consider connection limits per instance

- [ ] **Testing**
  - Baseline: 100 sequential requests to single endpoint
  - After: 100 sequential requests (measure improvement)
  - Concurrent: 50 concurrent requests, measure connection reuse

- [ ] **Acceptance Criteria**
  - Connection pool should reduce latency by 10-20%
  - No connection leaks (verify with `ss -tna | grep ESTABLISHED`)
  - Graceful connection cleanup on instance shutdown

#### 2.2 Cache Improvements

- [ ] **Query result caching**
  - Cache `/tabs` aggregate results (5-10s TTL)
  - Cache `/instances` listing (2-5s TTL)
  - Cache profile data (1-5m TTL)

- [ ] **Implementation Strategy**
  - Add simple in-memory cache with sync.RWMutex
  - Invalidate cache on instance create/delete/update
  - Log cache hits/misses for monitoring

- [ ] **Testing**
  - Measure reduction in aggregation time
  - Verify cache invalidation on changes
  - Test with 50+ instances

- [ ] **Acceptance Criteria**
  - `/tabs` response time with cache < 50ms (vs 500ms without)
  - Cache eviction doesn't break consistency
  - Stale reads acceptable for 5-10s window

#### 2.3 Batch Navigation Support

- [ ] **Bulk navigation endpoint**
  - `POST /instances/{id}/batch-navigate` - navigate multiple URLs
  - `POST /batch-navigate` - navigate across instances

- [ ] **Implementation Strategy**
  - Accept array of URLs
  - Return array of tab IDs
  - Consider sequential vs parallel (with concurrency limits)

- [ ] **Testing**
  - Navigate 50 URLs on single instance
  - Measure latency improvement vs sequential calls
  - Verify all tabs created correctly

- [ ] **Acceptance Criteria**
  - 50-URL batch < 20s (vs 50 * 200ms = 10s sequential + overhead)
  - All tabs successfully created
  - Error handling for partial failures

---

## Section 3: Scaling Tests & Validation

### Objectives

Validate system behavior under realistic production load scenarios.

### Tasks

#### 3.1 Stress Test: 100+ Concurrent Instances

- [ ] **Test scenario**
  - Create 100 instances in rapid sequence
  - Verify all reach "running" status
  - Verify port allocation doesn't fail
  - Verify Chrome initialization completes for all

- [ ] **Monitoring**
  - Track system metrics: CPU, RAM, file descriptors, disk I/O
  - Monitor orchestrator response times
  - Log any errors or warnings

- [ ] **Implementation**
  - Build test script: Create 100 instances, pause, verify all running
  - Run 3x and average results
  - Document peak resource usage

- [ ] **Acceptance Criteria**
  - All 100 instances successfully created
  - No port allocation conflicts
  - CPU usage < 80% during test
  - RAM usage < 12 GB
  - All instances responsive after test

#### 3.2 Multi-Agent Concurrent Load

- [ ] **Test scenario**
  - Simulate 5-10 agents, each using 2-3 instances
  - Each agent navigates different URLs concurrently
  - Run for 5 minutes
  - Measure latency, errors, resource usage

- [ ] **Monitoring**
  - Track latency distribution (p50, p95, p99)
  - Monitor for race conditions or conflicts
  - Verify instance isolation (no data leakage)

- [ ] **Implementation**
  - Build multi-agent test harness
  - Use goroutines to simulate agents
  - Log actions and timing

- [ ] **Acceptance Criteria**
  - p99 latency < 5s for all operations
  - 0 errors from isolation violations
  - CPU usage stable (not growing over time)
  - No connection leaks

#### 3.3 Resource Limits Testing

- [ ] **Test orchestrator limits**
  - What happens at 1000 instances? (Should gracefully degrade or error)
  - What happens with port range exhaustion? (Clear error, not crash)
  - What happens with memory pressure? (Can't create more instances)

- [ ] **Test instance limits**
  - How many tabs per instance? (Chrome limit, usually 500+)
  - What happens over 500 tabs? (Error or performance degradation)
  - How many concurrent requests per instance?

- [ ] **Implementation**
  - Build test for each limit
  - Document expected behavior and error messages
  - Verify graceful degradation

- [ ] **Acceptance Criteria**
  - All limit conditions handled gracefully (no crashes)
  - Clear error messages to users
  - Documentation of limits
  - Possible auto-scaling recommendations

---

## Success Metrics

| Metric | Current | Target | Priority |
|--------|---------|--------|----------|
| `/navigate` latency (p99) | TBD | < 5s | High |
| Instance memory (idle) | TBD | < 150MB | High |
| 100 instances creation time | TBD | < 2 min | High |
| Cache hit ratio | N/A | > 80% | Medium |
| Connection reuse efficiency | N/A | > 90% | Medium |
| Error rate under load | TBD | < 0.1% | High |

---

## Timeline Estimate

- **Benchmarking (3.1)**: 2-3 hours
- **Connection pooling (2.1)**: 1-2 hours
- **Cache improvements (2.2)**: 1-2 hours
- **Batch navigation (2.3)**: 1-2 hours
- **Stress testing (3.1-3.3)**: 2-3 hours
- **Documentation & analysis**: 1-2 hours

**Total: 9-14 hours**

---

## Deliverables

- [ ] `docs/performance-benchmarks.md` - Baseline metrics and analysis
- [ ] `docs/performance-tuning.md` - Optimization recommendations
- [ ] `scripts/benchmark.sh` - Automated benchmarking harness
- [ ] `scripts/stress-test.sh` - 100+ instance stress test
- [ ] `scripts/multi-agent-load.sh` - Concurrent agent simulation
- [ ] Updated `TESTING.md` with performance test procedures
- [ ] Performance dashboard or metrics export (optional)

---

## Notes

- Focus on realistic production scenarios, not synthetic micro-benchmarks
- Profile actual bottlenecks before optimizing (avoid premature optimization)
- Document all findings, even if results are "good enough"
- Consider infrastructure costs (more instances = more resource usage)
- Plan for monitoring/observability from the start
