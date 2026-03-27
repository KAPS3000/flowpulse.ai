# FlowPulse — Product Requirements Document & Software Functional Specification

**Version:** 1.0  
**Date:** March 26, 2026  
**Author:** Product & Engineering  
**Status:** Draft  

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Problem Statement](#2-problem-statement)
3. [Product Vision & Goals](#3-product-vision--goals)
4. [Target Users & Personas](#4-target-users--personas)
5. [Use Cases](#5-use-cases)
6. [System Design](#6-system-design)
    - 6.1 [Design Principles](#61-design-principles)
    - 6.2 [High-Level Architecture](#62-high-level-architecture)
    - 6.3 [Component Deep Dive](#63-component-deep-dive)
    - 6.4 [Data Flow & Lifecycle](#64-data-flow--lifecycle)
    - 6.5 [Networking & Service Communication](#65-networking--service-communication)
    - 6.6 [Storage Architecture](#66-storage-architecture)
    - 6.7 [Real-Time Event Architecture](#67-real-time-event-architecture)
    - 6.8 [Scaling Strategy](#68-scaling-strategy)
    - 6.9 [Failure Modes & Resilience](#69-failure-modes--resilience)
    - 6.10 [Security Architecture](#610-security-architecture)
    - 6.11 [Capacity Planning & Sizing](#611-capacity-planning--sizing)
    - 6.12 [Technology Choices & Rationale](#612-technology-choices--rationale)
7. [Functional Requirements](#7-functional-requirements)
8. [Data Model & Schema](#8-data-model--schema)
9. [API Specification](#9-api-specification)
10. [eBPF Instrumentation Specification](#10-ebpf-instrumentation-specification)
11. [Dashboard & UI Specification](#11-dashboard--ui-specification)
12. [Deployment Specification](#12-deployment-specification)
13. [Algorithms & Metrics Computation](#13-algorithms--metrics-computation)
14. [Non-Functional Requirements](#14-non-functional-requirements)
15. [Known Gaps & Roadmap](#15-known-gaps--roadmap)
16. [Glossary](#16-glossary)

---

## 1. Executive Summary

FlowPulse is a real-time network observability platform purpose-built for large-scale distributed ML training clusters. It uses eBPF-based kernel instrumentation to capture east-west network flows, CPU scheduling behavior, and InfiniBand/RoCEv2 RDMA traffic at the node level — then correlates, aggregates, and visualizes this data to surface training-specific efficiency metrics such as straggler detection, communication bubble ratio, gradient synchronization overhead, and network saturation.

The system is designed for deployment on GPU datacenter infrastructure where hundreds to thousands of nodes participate in collective communication (NCCL AllReduce, AllGather, ReduceScatter) over high-speed InfiniBand fabrics. FlowPulse answers the question: **"Why is my distributed training job slow, and which nodes or network paths are responsible?"**

### Key Differentiators

- **Kernel-level visibility** via eBPF — zero application instrumentation required
- **RDMA-aware** — parses RoCEv2 BTH headers for QP-level telemetry
- **Training-specific metrics** — straggler score, bubble ratio, gradient sync overhead — not generic network monitoring
- **Sub-second latency** — NATS JetStream event bus + WebSocket push to live dashboard
- **Multi-tenant** — tenant isolation via JWT claims and cgroup-to-tenant mapping

---

## 2. Problem Statement

### Context

Distributed ML training at scale (thousands of GPUs across hundreds of nodes) suffers from tail-latency problems where a single slow node ("straggler") or degraded network link can block an entire collective operation, causing all other nodes to wait. These "bubbles" in the training pipeline can reduce GPU utilization from >90% to <60% without any visible error.

### Challenges

| Challenge | Current State |
|-----------|---------------|
| **Invisible stragglers** | Training frameworks (PyTorch, JAX) report aggregate throughput but do not isolate per-node network contribution |
| **Network blind spots** | Standard monitoring (Prometheus, Grafana) captures interface-level counters but misses per-flow, per-QP, and per-collective behavior |
| **RDMA opacity** | InfiniBand error counters exist in sysfs but are not correlated with specific training collectives or flow patterns |
| **Root-cause latency** | By the time operators notice degraded training throughput, the causal event (link flap, ECN storm, PFC pause) has passed without telemetry |
| **Multi-tenant blindness** | Shared clusters run multiple training jobs; per-tenant visibility requires flow-level attribution, not just interface stats |

### Impact

A 5% reduction in network efficiency for a 1,000-GPU training run costs approximately $50K/day in wasted compute (at $2/GPU-hour). Early detection of straggler nodes or saturated links can save millions in large-scale training campaigns.

---

## 3. Product Vision & Goals

### Vision

FlowPulse provides the "MRI for your training cluster network" — a deep, real-time, always-on view of how data moves between GPUs during distributed training, surfacing the exact nodes, links, and collective operations that limit training throughput.

### Goals (v1.0)

| # | Goal | Success Metric |
|---|------|----------------|
| G1 | Capture all east-west TCP/UDP flows at kernel level with <1% CPU overhead per node | Measured via `perf stat` under synthetic NCCL load |
| G2 | Detect straggler nodes within 10 seconds of onset | Straggler score deviation >20 within 2 polling cycles |
| G3 | Provide per-tenant flow visibility on shared clusters | Distinct flow tables per tenant_id in ClickHouse |
| G4 | Surface 5 training-specific metrics on a live dashboard | All metric cards updating via WebSocket within 5s |
| G5 | Support clusters of 64–2,048 nodes | Aggregator handles 100K flows/sec sustained |
| G6 | Deploy as a DaemonSet on Kubernetes with zero application changes | Helm chart installs end-to-end in <5 minutes |

---

## 4. Target Users & Personas

### Persona 1: ML Infrastructure Engineer ("Infra Eng")

- **Role:** Manages the GPU cluster, network fabric, and training job scheduling
- **Needs:** Per-node and per-link health visibility; rapid root-cause analysis when training throughput degrades
- **Uses:** Topology view, straggler leaderboard, flow table with RDMA details, alert panel

### Persona 2: ML Training Engineer ("ML Eng")

- **Role:** Develops and runs distributed training jobs
- **Needs:** Understanding whether training slowdowns are caused by model code or infrastructure
- **Uses:** Training metrics dashboard (bubble ratio, gradient sync overhead), collective timeline

### Persona 3: Platform/SRE Team Lead ("SRE Lead")

- **Role:** Oversees cluster reliability and cost efficiency
- **Needs:** Trend analysis, multi-tenant utilization comparison, capacity planning data
- **Uses:** Historical metrics, per-tenant flow aggregation, alert rules and notification

---

## 5. Use Cases

### UC-1: Real-Time Straggler Detection

**Trigger:** A 128-node AllReduce operation has 1 node with a degraded InfiniBand link.

**Flow:**
1. eBPF agent on each node captures per-flow byte counts and RDMA QP telemetry every 2 seconds.
2. Aggregator correlates forward/reverse flows and computes per-node IB utilization deviation.
3. Straggler score exceeds threshold; flagged in training metrics and published via NATS.
4. Dashboard highlights the straggler node in the topology view and leaderboard within 5 seconds.

**Outcome:** Infra engineer identifies the degraded node and drains it before the next training step.

### UC-2: Communication Bubble Analysis

**Trigger:** Training throughput drops 15% with no code changes.

**Flow:**
1. Agent captures CPU scheduling data: context switches, softirq time breakdown (NET_TX/NET_RX).
2. Aggregator computes bubble ratio (softirq time as proportion of active CPU time).
3. High bubble ratio (>40%) indicates GPUs are idle waiting for network collectives.
4. Correlated with network saturation index to distinguish "slow network" from "inefficient collective".

**Outcome:** ML engineer identifies that the job's AllGather step is bottlenecked by asymmetric payload sizes.

### UC-3: Multi-Tenant Flow Isolation

**Trigger:** Two training jobs on a shared cluster suspect network interference.

**Flow:**
1. Agent tags each flow with a tenant_id derived from cgroup path or configuration.
2. ClickHouse stores flows partitioned by (tenant_id, date).
3. Dashboard filters by tenant; each tenant sees only their own flows.
4. Network saturation index computed independently per tenant.

**Outcome:** SRE confirms that tenant A's AllReduce traffic is not interfering with tenant B's.

### UC-4: Post-Incident Forensics

**Trigger:** A training run failed at 3 AM; need to understand what happened.

**Flow:**
1. ClickHouse retains 30 days of flow data and 7 days of node metrics.
2. Operator queries historical flows filtered by time range, node, and protocol.
3. Training metrics timeline shows the exact moment straggler score spiked.
4. RDMA retransmission and ECN mark counts identify the degraded path.

**Outcome:** Root cause identified as a specific switch port generating excessive ECN marks.

---

## 6. System Design

### 6.1 Design Principles

| Principle | Rationale | How Applied |
|-----------|-----------|-------------|
| **Zero instrumentation** | ML training code must not be modified; changes risk correctness and slow iteration cycles | All telemetry is captured via eBPF at the kernel level — applications are unaware |
| **Observe, never interfere** | Monitoring must never impact training throughput or correctness | TC programs return TC_ACT_OK (passthrough); BPF maps use LRU eviction rather than blocking |
| **Separate hot and warm paths** | Real-time dashboards need sub-second latency; historical queries need columnar analytics — different access patterns | NATS JetStream for hot-path fan-out; ClickHouse for warm-path analytics; never block one on the other |
| **Push from the edge** | Agents should push telemetry rather than be polled; reduces coordination and scales linearly | Agents batch and HTTP POST to aggregator; no central poller |
| **Tenant as a first-class dimension** | Shared GPU clusters host multiple training jobs; every data path must be tenant-scoped | Flows tagged at capture; ClickHouse partitioned by tenant; API enforces tenant from JWT; NATS subjects namespaced |
| **Graceful degradation** | Component failure should reduce fidelity, not cause outages | Agent continues if flow_tracker fails; aggregator drops batches on backpressure; dashboard falls back to REST polling if WebSocket disconnects |
| **Immutable infrastructure** | Containers should be stateless and replaceable; state lives in purpose-built stores | Go binaries in distroless images; ClickHouse for durable state; NATS for ephemeral events |

### 6.2 High-Level Architecture

```
                            ┌─────────────────────────────────────┐
                            │         CONTROL PLANE               │
                            │  ┌───────────────────────────────┐  │
                            │  │  Kubernetes API Server         │  │
                            │  │  - DaemonSet scheduling        │  │
                            │  │  - RBAC / ServiceAccounts      │  │
                            │  │  - ConfigMaps / Secrets        │  │
                            │  └───────────────────────────────┘  │
                            └─────────────────────────────────────┘

 ╔═══════════════════════════════════════════════════════════════════════════╗
 ║  DATA PLANE                                                              ║
 ║                                                                          ║
 ║  ┌──────────┐ ┌──────────┐ ┌──────────┐       ┌──────────┐              ║
 ║  │  Node 1  │ │  Node 2  │ │  Node 3  │  ...  │  Node N  │              ║
 ║  │ ┌──────┐ │ │ ┌──────┐ │ │ ┌──────┐ │       │ ┌──────┐ │              ║
 ║  │ │eBPF  │ │ │ │eBPF  │ │ │ │eBPF  │ │       │ │eBPF  │ │              ║
 ║  │ │Agent │ │ │ │Agent │ │ │ │Agent │ │       │ │Agent │ │              ║
 ║  │ └──┬───┘ │ │ └──┬───┘ │ │ └──┬───┘ │       │ └──┬───┘ │              ║
 ║  └────┼─────┘ └────┼─────┘ └────┼─────┘       └────┼─────┘              ║
 ║       │            │            │                   │                     ║
 ║       └────────────┴─────┬──────┴───────────────────┘                    ║
 ║                          │ HTTP POST (FlowBatch + NodeMetrics)            ║
 ║                          ▼                                                ║
 ║  ┌─────────────────────────────────────────────────────────────────────┐  ║
 ║  │                    AGGREGATION TIER                                  │  ║
 ║  │                                                                     │  ║
 ║  │  ┌─────────────────┐    ┌─────────────────┐    (future: sharded)   │  ║
 ║  │  │  Aggregator-0   │    │  Aggregator-1   │                        │  ║
 ║  │  │  ┌───────────┐  │    │  ┌───────────┐  │                        │  ║
 ║  │  │  │ Correlator│  │    │  │ Correlator│  │                        │  ║
 ║  │  │  │ Metrics   │  │    │  │ Metrics   │  │                        │  ║
 ║  │  │  │ Computer  │  │    │  │ Computer  │  │                        │  ║
 ║  │  │  └─────┬─────┘  │    │  └─────┬─────┘  │                        │  ║
 ║  │  └────────┼────────┘    └────────┼────────┘                        │  ║
 ║  │           │   write              │   write                          │  ║
 ║  │           ▼                      ▼                                  │  ║
 ║  │  ┌──────────────────────────────────────────────────────────────┐   │  ║
 ║  │  │                    STORAGE TIER                               │   │  ║
 ║  │  │                                                               │   │  ║
 ║  │  │  ┌──────────────┐  ┌───────────────┐  ┌─────────────────┐   │   │  ║
 ║  │  │  │  ClickHouse  │  │     NATS      │  │     Redis       │   │   │  ║
 ║  │  │  │  (columnar)  │  │  (JetStream)  │  │   (cache/state) │   │   │  ║
 ║  │  │  │              │  │               │  │   (future)      │   │   │  ║
 ║  │  │  │  flows       │  │  .flows       │  │                 │   │   │  ║
 ║  │  │  │  node_metrics│  │  .metrics     │  │                 │   │   │  ║
 ║  │  │  │  training_   │  │  .training    │  │                 │   │   │  ║
 ║  │  │  │    metrics   │  │               │  │                 │   │   │  ║
 ║  │  │  │  metrics_1m  │  │               │  │                 │   │   │  ║
 ║  │  │  │    (MV)      │  │               │  │                 │   │   │  ║
 ║  │  │  └──────┬───────┘  └───────┬───────┘  └─────────────────┘   │   │  ║
 ║  │  └─────────┼──────────────────┼─────────────────────────────────┘   │  ║
 ║  └────────────┼──────────────────┼─────────────────────────────────────┘  ║
 ║               │ query            │ subscribe                              ║
 ║               ▼                  ▼                                        ║
 ║  ┌─────────────────────────────────────────────────────────────────────┐  ║
 ║  │                      SERVING TIER                                   │  ║
 ║  │                                                                     │  ║
 ║  │  ┌─────────────────────────┐    ┌────────────────────────────────┐  │  ║
 ║  │  │  API Server (×2+)      │    │  Web Dashboard (×2+)           │  │  ║
 ║  │  │  ┌───────────────────┐ │    │  ┌────────────────────────┐   │  │  ║
 ║  │  │  │ REST: /api/v1/*   │ │    │  │ Next.js SSR/SSG        │   │  │  ║
 ║  │  │  │ WS:   /ws         │ │◄───│  │ Zustand state store    │   │  │  ║
 ║  │  │  │ Auth: JWT + CORS  │ │    │  │ WebSocket client       │   │  │  ║
 ║  │  │  └───────────────────┘ │    │  └────────────────────────┘   │  │  ║
 ║  │  └─────────────────────────┘    └────────────────────────────────┘  │  ║
 ║  └─────────────────────────────────────────────────────────────────────┘  ║
 ║                          ▲                                                ║
 ╚══════════════════════════╪════════════════════════════════════════════════╝
                            │ HTTPS / WSS
                            │
                     ┌──────┴──────┐
                     │   Browser   │
                     │  (Operator) │
                     └─────────────┘
```

### 6.3 Component Deep Dive

#### 6.3.1 eBPF Agent (Per-Node)

The agent is the edge telemetry collector. It runs one instance per node as a Kubernetes DaemonSet (or standalone on bare metal). It operates in two domains: kernel space (eBPF programs) and user space (Go binary).

**Kernel Space — eBPF Programs**

```
┌─────────────────── Linux Kernel ───────────────────────────────────────┐
│                                                                         │
│  TC Subsystem                    Tracepoint Subsystem                   │
│  ┌────────────────────────┐      ┌─────────────────────────────────┐   │
│  │  flow_tracker.o        │      │  cpu_sched.o                    │   │
│  │                        │      │                                 │   │
│  │  ingress ──► parse ──► │      │  sched_switch ──► cpu_stats    │   │
│  │              ETH/IP/   │      │  softirq_entry ──► start_ts    │   │
│  │              TCP/UDP   │      │  softirq_exit  ──► softirq_    │   │
│  │              RoCEv2    │      │                     stats       │   │
│  │  egress  ──► parse ──► │      │                                 │   │
│  │                    │   │      │                    │            │   │
│  │          ┌─────────▼─┐ │      │       ┌───────────▼──────────┐ │   │
│  │          │flow_table │ │      │       │ cpu_stats            │ │   │
│  │          │LRU hash   │ │      │       │ percpu_hash          │ │   │
│  │          │100K entries│ │      │       │                      │ │   │
│  │          └───────────┘ │      │       │ softirq_stats        │ │   │
│  │          ┌───────────┐ │      │       │ percpu_hash          │ │   │
│  │          │flow_events│ │      │       └──────────────────────┘ │   │
│  │          │ringbuf    │ │      │       ┌──────────────────────┐ │   │
│  │          │256KB      │ │      │       │ cpu_events ringbuf   │ │   │
│  │          └───────────┘ │      │       └──────────────────────┘ │   │
│  └────────────────────────┘      └─────────────────────────────────┘   │
│                                                                         │
│  Future: ib_verbs.o (kprobe: ib_post_send/recv, ib_poll_cq)           │
└─────────────────────────────────────────────────────────────────────────┘
          │                                │
          │ BPF syscall (map read)         │ BPF syscall (map iterate)
          ▼                                ▼
┌─────────────────── User Space (Go) ─────────────────────────────────────┐
│                                                                         │
│  ┌────────────────┐  ┌────────────────┐  ┌─────────────────────────┐   │
│  │  Flow Poller   │  │  CPU Poller    │  │  IB Sysfs Collector     │   │
│  │  (2s interval) │  │  (2s interval) │  │  (reads /sys/class/     │   │
│  │                │  │                │  │   infiniband/*)          │   │
│  │  - Iterate     │  │  - Iterate     │  │                         │   │
│  │    flow_table  │  │    cpu_stats   │  │  - TX/RX bytes/pkts     │   │
│  │  - Read ring   │  │  - Iterate     │  │  - Error counters       │   │
│  │    buffer      │  │    softirq_    │  │  - Link utilization     │   │
│  │  - Update      │  │    stats       │  │                         │   │
│  │    in-memory   │  │  - Aggregate   │  │                         │   │
│  │    flow cache  │  │    percpu      │  │                         │   │
│  └───────┬────────┘  └───────┬────────┘  └────────────┬────────────┘   │
│          │                   │                         │                │
│          ▼                   ▼                         ▼                │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    Flush / Send Pipeline                         │   │
│  │                                                                  │   │
│  │  FlowBatch (every 2s)              NodeMetrics (every 2s)       │   │
│  │  ┌──────────────────────┐          ┌─────────────────────┐      │   │
│  │  │ node_id: "gpu-node-1"│          │ node_id, tenant_id  │      │   │
│  │  │ tenant_id: "team-a"  │          │ cpu_metrics[]:      │      │   │
│  │  │ flows[]: 500 max/    │          │   core utilization   │      │   │
│  │  │   batch              │          │   context switches   │      │   │
│  │  │ collected_at: now()  │          │   softirq %         │      │   │
│  │  └──────────┬───────────┘          │ ib_metrics:          │      │   │
│  │             │                      │   tx/rx bytes        │      │   │
│  │             │                      │   link utilization   │      │   │
│  │             │                      └──────────┬──────────┘      │   │
│  │             │                                 │                  │   │
│  │             ▼                                 ▼                  │   │
│  │       HTTP POST to aggregator:9092                               │   │
│  │       /api/v1/ingest/flows    /api/v1/ingest/metrics            │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  Health Server (:8081)                                          │   │
│  │  GET /healthz → {"status":"ok"}                                 │   │
│  │  GET /metrics → flowpulse_agent_flows_tracked (Prometheus)      │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

**In-Memory Flow Cache:** The agent maintains a local flow cache (`flow_table.go`) keyed by 5-tuple. This serves as a staging buffer between BPF map reads and aggregator sends. Flows are drained in batches (max 500 per HTTP request). Stale flows (no packets for `flow_timeout/2`) are evicted to prevent unbounded memory growth.

**Concurrency Model:** The agent uses `errgroup` to run 6 goroutines concurrently: flow map poller, ring buffer reader, CPU map poller, flush loop, eviction loop, and health server. All share a single context for coordinated shutdown.

#### 6.3.2 Aggregator (Stateful Processing)

The aggregator is the brain of the pipeline. It receives raw telemetry, correlates it into meaningful structures, computes derived metrics, and fans out to both durable storage and real-time consumers.

```
                    HTTP Ingest (:9092)
                          │
              ┌───────────┴───────────┐
              │                       │
              ▼                       ▼
     ┌─────────────────┐    ┌─────────────────┐
     │  flowCh (4096)  │    │  metricCh (1024)│
     │  buffered chan   │    │  buffered chan   │
     └────────┬────────┘    └────────┬────────┘
              │                      │
              ▼                      ▼
     ┌─────────────────┐    ┌─────────────────┐
     │  processFlows   │    │ processMetrics  │
     │                 │    │                 │
     │  For each flow: │    │  Store latest   │
     │  correlator.    │    │  per node_id    │
     │    Ingest()     │    │                 │
     │                 │    │  Batch & write  │
     │  Publish to     │    │  to ClickHouse  │
     │  NATS .flows    │    │  every 5s       │
     └────────┬────────┘    │                 │
              │             │  Publish to     │
              │             │  NATS .metrics  │
              │             └────────┬────────┘
              │                      │
              ▼                      ▼
     ┌─────────────────────────────────────────┐
     │           Flow Correlator               │
     │                                         │
     │  In-memory map[canonicalKey]*Correlated  │
     │                                         │
     │  Canonical key = sorted(srcIP,dstIP) +  │
     │    sorted(srcPort,dstPort) + protocol   │
     │                                         │
     │  Forward + Reverse flows merged into    │
     │  CorrelatedFlow with TotalBytes/Pkts    │
     └────────────────┬────────────────────────┘
                      │ DrainAll() every 5s
                      ▼
     ┌────────────────────────────────────────────┐
     │              flushLoop (5s)                │
     │                                            │
     │  correlated flows ──► ClickHouse.flows     │
     │  (batch INSERT)                            │
     └────────────────────────────────────────────┘

     ┌────────────────────────────────────────────┐
     │         computeMetricsLoop (5s)            │
     │                                            │
     │  nodeMetrics + correlatedFlows             │
     │        │                                   │
     │        ▼                                   │
     │  MetricsComputer.ComputeTrainingMetrics()  │
     │        │                                   │
     │        ├──► StragglerScore                 │
     │        ├──► BubbleRatio                    │
     │        ├──► GradientSyncOverhead           │
     │        ├──► NetworkSaturation              │
     │        └──► ImbalanceScore                 │
     │                                            │
     │  Result ──► ClickHouse.training_metrics    │
     │         ──► NATS .training                 │
     └────────────────────────────────────────────┘
```

**Flow Correlation Algorithm:**
1. On ingest, each unidirectional flow is keyed by a canonical 5-tuple: `min(srcIP, dstIP), max(srcIP, dstIP), min(srcPort, dstPort), max(srcPort, dstPort), protocol`.
2. If a CorrelatedFlow exists for that key, the new flow is merged as the Forward or Reverse direction.
3. On flush, all correlated flows are drained, written to ClickHouse, and the map is cleared.

#### 6.3.3 API Server (Stateless Serving)

```
             Browser Requests
                   │
                   ▼
     ┌─────────────────────────────┐
     │     Chi Router (Go)         │
     │                             │
     │  Global Middleware:         │
     │  ├── RequestID              │
     │  ├── RealIP                 │
     │  ├── Recoverer              │
     │  ├── Timeout (30s)          │
     │  └── CORS (Allow-Origin: *)│
     │                             │
     │  Public Routes:             │
     │  ├── GET  /healthz          │
     │  └── POST /api/v1/auth/     │
     │          token              │
     │                             │
     │  Protected Routes:          │
     │  (JWTAuth middleware)       │
     │  ├── GET /api/v1/flows      │──► ClickHouse Reader
     │  ├── GET /api/v1/metrics/   │──► ClickHouse Reader
     │  │       training           │
     │  └── GET /api/v1/topology   │──► ClickHouse Reader
     │                             │
     │  WebSocket:                 │
     │  └── GET /ws?tenant_id=     │──► NATS Subscribe
     │          ↕ JSON frames      │        ↓
     │          to/from browser    │    WSGateway
     └─────────────────────────────┘    (fan-out)
```

**Request Flow (REST):**
1. Incoming request → CORS headers set → JWT extracted from `Authorization: Bearer` header.
2. JWT validated (HS256, not expired) → `tenant_id` extracted from claims and injected into context.
3. Handler queries ClickHouse with `WHERE tenant_id = ?` — tenant isolation enforced at the query level.
4. Response serialized as JSON and returned.

**WebSocket Lifecycle:**
1. Client connects to `/ws?tenant_id=local-dev`.
2. Server upgrades to WebSocket, creates NATS subscriptions for `flowpulse.local-dev.{flows,metrics,training}`.
3. On each NATS message, the server wraps it as `{"subject": "...", "data": {...}}` and writes to the WebSocket.
4. On client disconnect, NATS subscriptions are drained and cleaned up.

#### 6.3.4 Web Dashboard (Client-Side)

```
┌─────────────────────── Browser ──────────────────────────────────┐
│                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │  Next.js App                                                │ │
│  │                                                              │ │
│  │  ┌──────────────┐    ┌─────────────────────────────────┐    │ │
│  │  │  useData     │    │  useWebSocket('local-dev')      │    │ │
│  │  │  Loader      │    │                                 │    │ │
│  │  │              │    │  WSManager                      │    │ │
│  │  │  REST poll   │    │  ├─ connect()                   │    │ │
│  │  │  every 3s    │    │  ├─ subscribe(callback)         │    │ │
│  │  │              │    │  ├─ reconnect (exp backoff)     │    │ │
│  │  │  1. getFlows │    │  └─ 1s → 2s → 4s → ... → 30s  │    │ │
│  │  │  2. getTopo  │    │                                 │    │ │
│  │  │  3. getTrain │    │  On message:                    │    │ │
│  │  │  4. getAlert │    │  ├─ .flows    → addFlow()       │    │ │
│  │  │  5. getSumm  │    │  ├─ .training → setMetrics()   │    │ │
│  │  │              │    │  ├─ .metrics  → setNodes()      │    │ │
│  │  └──────┬───────┘    │  └─ .alert    → addAlert()     │    │ │
│  │         │            └──────────┬──────────────────────┘    │ │
│  │         │                       │                            │ │
│  │         ▼                       ▼                            │ │
│  │  ┌──────────────────────────────────────────────────────┐   │ │
│  │  │                Zustand Store                          │   │ │
│  │  │                                                      │   │ │
│  │  │  flows: Flow[]           (cap: 10,000)               │   │ │
│  │  │  totalFlowCount: number                              │   │ │
│  │  │  trainingMetrics: TrainingMetrics                    │   │ │
│  │  │  topologyNodes: TopologyNode[]                       │   │ │
│  │  │  alerts: Alert[]                                     │   │ │
│  │  │  alertSummary: AlertSummary                          │   │ │
│  │  │  isConnected: boolean                                │   │ │
│  │  └──────────────────────────┬───────────────────────────┘   │ │
│  │                             │ React re-render               │ │
│  │                             ▼                               │ │
│  │  ┌──────────────────────────────────────────────────────┐   │ │
│  │  │  Page Components                                     │   │ │
│  │  │  ┌────────────┐ ┌──────────┐ ┌──────────────────┐   │   │ │
│  │  │  │  Overview  │ │ FlowTable│ │ TopologyView     │   │   │ │
│  │  │  │  (cards)   │ │ (table)  │ │ (grid + heatmap) │   │   │ │
│  │  │  └────────────┘ └──────────┘ └──────────────────┘   │   │ │
│  │  │  ┌────────────────────────┐ ┌────────────────────┐  │   │ │
│  │  │  │  TrainingDashboard    │ │ AlertPanel          │  │   │ │
│  │  │  │  ├─ StragglerBoard   │ │ (severity, ack,     │  │   │ │
│  │  │  │  ├─ BandwidthGauges  │ │  resolve actions)   │  │   │ │
│  │  │  │  └─ CollectiveTimeln │ │                      │  │   │ │
│  │  │  └────────────────────────┘ └────────────────────┘  │   │ │
│  │  └──────────────────────────────────────────────────────┘   │ │
│  └─────────────────────────────────────────────────────────────┘ │
└───────────────────────────────────────────────────────────────────┘
```

### 6.4 Data Flow & Lifecycle

#### 6.4.1 End-to-End Packet Journey

```
 Packet arrives at eth0 on GPU Node
         │
    ①    ▼  TC ingress hook (kernel, <1μs)
         flow_tracker.o: parse ETH→IP→TCP/UDP→optional RoCEv2 BTH
         Update flow_table LRU hash (atomic increment packets/bytes)
         If new flow: push event to flow_events ringbuf
         Return TC_ACT_OK (packet continues unmodified)
         │
    ②    ▼  Agent userspace poll (every 2s)
         Iterate flow_table → copy to in-memory flow cache
         Read flow_events ringbuf → log new flow detections
         Iterate cpu_stats/softirq_stats (percpu) → aggregate per-core
         Read /sys/class/infiniband/* → IB port metrics
         │
    ③    ▼  Agent flush (every 2s)
         Drain flow cache → split into batches of 500
         HTTP POST /api/v1/ingest/flows → aggregator:9092  (JSON, ~50KB/batch)
         HTTP POST /api/v1/ingest/metrics → aggregator:9092 (JSON, ~2KB)
         │
    ④    ▼  Aggregator ingest (immediate)
         Deserialize FlowBatch → push to flowCh (buffered 4096)
         Deserialize NodeMetrics → push to metricCh (buffered 1024)
         │
    ⑤    ▼  Aggregator processFlows (continuous)
         For each flow: correlator.Ingest(flow) — merge forward/reverse
         Marshal batch → NATS Publish to flowpulse.{tenant}.flows
         │
    ⑥    ▼  Aggregator flushLoop (every 5s)
         correlator.DrainAll() → batch INSERT into ClickHouse flows table
         │
    ⑦    ▼  Aggregator computeMetricsLoop (every 5s)
         Read nodeMetrics map + drain correlated flows
         Compute 5 training metrics
         INSERT into ClickHouse training_metrics
         NATS Publish to flowpulse.{tenant}.training
         │
    ⑧    ▼  API Server WebSocket Gateway (immediate on NATS msg)
         Subscribe to flowpulse.{tenant}.* subjects
         Wrap as {"subject": "...", "data": {...}}
         Write JSON frame to each connected WebSocket
         │
    ⑨    ▼  Browser (immediate on WS message)
         Parse message → dispatch to Zustand store action
         React re-renders affected components
```

#### 6.4.2 Latency Budget

| Step | Operation | P50 Latency | P99 Latency | Bottleneck |
|------|-----------|-------------|-------------|------------|
| ① | TC BPF program execution | <1μs | <5μs | BPF instruction limit (1M) |
| ② | Map iteration (100K entries) | ~20ms | ~50ms | Kernel lock contention under high flow churn |
| ③ | HTTP POST (500 flows) | ~10ms | ~50ms | Network RTT + JSON serialization |
| ④ | Channel enqueue | <1μs | <10μs | Channel buffer full → drop (backpressure) |
| ⑤ | Flow correlation | ~5ms | ~20ms | Map lookups, O(1) per flow |
| ⑥ | ClickHouse batch insert | ~50ms | ~200ms | Disk I/O, MergeTree merge |
| ⑦ | Training metrics compute | ~2ms | ~10ms | Math operations on node array |
| ⑧ | NATS → WS fan-out | ~5ms | ~20ms | NATS internal queue + WS write |
| ⑨ | Browser render | ~10ms | ~50ms | React reconciliation |
| **Total** | **Kernel → Dashboard** | **~2.1s** | **~7.4s** | **Dominated by poll interval (2s) + flush interval (5s)** |

#### 6.4.3 Data Retention & Lifecycle

```
                    Hot              Warm              Cold
                  (seconds)         (days)           (months)
                     │                │                 │
  NATS JetStream ◄───┤                │                 │
  (memory, 5min TTL) │                │                 │
                     │                │                 │
  ClickHouse ────────┼────────────────┤                 │
  flows              │   30 day TTL   │                 │
  node_metrics       │   7 day TTL    │                 │
  training_metrics   │   30 day TTL   │                 │
  metrics_1m (MV)    │   (no TTL — rollup survives)     │
                     │                │                 │
  Future: S3/GCS ────┼────────────────┼─────────────────┤
  (Parquet export)   │                │   archival      │
```

### 6.5 Networking & Service Communication

#### 6.5.1 Port Map

| Component | Port | Protocol | Direction | Purpose |
|-----------|------|----------|-----------|---------|
| Agent | 8081 | HTTP | Inbound | Health checks and Prometheus metrics |
| Aggregator | 9091 | TCP (gRPC) | Inbound | Reserved for future gRPC ingest |
| Aggregator | 9092 | HTTP | Inbound | Flow and metric ingest from agents |
| API Server | 8080 | HTTP | Inbound | REST API and WebSocket for dashboard |
| API Server | 9090 | TCP (gRPC) | Inbound | Reserved for future gRPC API |
| ClickHouse | 9000 | TCP (native) | Internal | Binary protocol for Go client |
| ClickHouse | 8123 | HTTP | Internal | HTTP interface for debugging |
| NATS | 4222 | TCP | Internal | Client connections |
| NATS | 8222 | HTTP | Internal | Monitoring/healthcheck |
| Redis | 6379 | TCP | Internal | Future caching layer |
| Web | 3000 | HTTP | Inbound | Dashboard for browsers |

#### 6.5.2 Service Dependencies

```
                    ┌──────────┐
                    │  Agent   │
                    └────┬─────┘
                         │ depends on (runtime)
                         ▼
                  ┌──────────────┐
                  │  Aggregator  │
                  └──┬─────┬────┘
                     │     │ depends on (startup)
              ┌──────┘     └──────┐
              ▼                   ▼
        ┌───────────┐      ┌───────────┐
        │ClickHouse │      │   NATS    │
        └─────┬─────┘      └─────┬─────┘
              │                  │  depends on (startup)
              │           ┌──────┘
              ▼           ▼
        ┌─────────────────────┐
        │    API Server       │───► Redis (future, optional)
        └──────────┬──────────┘
                   │ depends on (runtime, optional)
                   ▼
             ┌───────────┐
             │    Web    │
             └───────────┘
```

**Startup order:** ClickHouse → NATS → Redis → Aggregator → Server → Web → Agents. Docker Compose enforces this via `depends_on` with health checks.

#### 6.5.3 Kubernetes Network Policies

The Helm chart implements a **default-deny** network policy strategy with explicit allow rules:

| Source | Destination | Ports | Justification |
|--------|-------------|-------|---------------|
| Agent (any node) | Aggregator | 9092 | Flow and metric ingest |
| Aggregator | ClickHouse | 9000 | Write flows, metrics, training data |
| Aggregator | NATS | 4222 | Publish real-time events |
| API Server | ClickHouse | 9000 | Read queries |
| API Server | NATS | 4222 | WebSocket gateway subscriptions |
| API Server | Redis | 6379 | Future caching |
| Web | API Server | 8080 | SSR API calls |
| Ingress | Web | 3000 | External dashboard access |
| Ingress | API Server | 8080 | External API access |

### 6.6 Storage Architecture

#### 6.6.1 ClickHouse Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Engine | MergeTree family | Optimized for append-heavy, read-heavy analytical workloads |
| Partitioning | `(tenant_id, toDate(timestamp))` | Enables partition pruning on the two most common WHERE clauses; efficient TTL-based deletion |
| Sort order (flows) | `(tenant_id, flow_id, timestamp)` | Fast point lookups by flow_id within a tenant; range scans by time |
| Sort order (metrics) | `(tenant_id, node_id, timestamp)` | Fast per-node time series queries for topology |
| DateTime precision | DateTime64(3) — millisecond | eBPF timestamps are nanosecond but millisecond is sufficient for analysis; avoids uint64 overflow in TTL |
| TTL implementation | `toDateTime(timestamp) + INTERVAL` | Cast required because DateTime64 cannot be used directly in TTL expressions |
| Rollup | SummingMergeTree materialized view | Automatic 1-minute aggregation for long-term capacity planning without manual ETL |

#### 6.6.2 Write Path Optimization

```
Agent sends 500 flows per batch, ~20 batches/node/flush
  → Aggregator correlates into ~300 CorrelatedFlows (forward+reverse merge)
    → Single PrepareBatch → batch.Append (300 rows) → batch.Send
      → ClickHouse native protocol batch insert (single network round trip)
        → MergeTree data part written to disk
          → Background merge consolidates parts (ClickHouse internal)
```

**Key optimizations:**
- Batch inserts (not individual INSERTs) — amortizes network and disk I/O
- Native protocol (port 9000, not HTTP 8123) — binary encoding, lower overhead
- Correlation reduces write volume by ~40% (2 unidirectional → 1 bidirectional)
- TTL-based deletion avoids manual cleanup jobs

#### 6.6.3 Read Path Optimization

```
Dashboard request: GET /api/v1/flows?limit=100&sort_by=bytes&sort_order=desc
  → ClickHouse query:
    SELECT ... FROM flowpulse.flows
    WHERE tenant_id = 'local-dev'        ← partition pruning (eliminates other tenants)
    ORDER BY bytes DESC                  ← may require full scan within partition
    LIMIT 100 OFFSET 0                   ← early termination
  → Two queries: COUNT(*) for total, then data query
```

**Optimization opportunities (future):**
- Secondary indices on `src_ip`, `dst_ip` for filtered queries
- Pre-aggregated materialized views for common dashboard queries
- Redis caching for frequently-accessed topology data

### 6.7 Real-Time Event Architecture

#### 6.7.1 NATS JetStream Configuration

| Setting | Value | Rationale |
|---------|-------|-----------|
| Stream name | `flowpulse` | Single stream with wildcard subjects |
| Subjects | `flowpulse.>` | Tenant-scoped: `flowpulse.{tenant}.flows/metrics/training` |
| Storage | Memory | Real-time events are ephemeral; ClickHouse is the durable store |
| Max age | 5 minutes | Prevents unbounded memory growth if consumers lag |
| Replicas | 1 (dev) / 3 (prod) | Trade availability vs. resource cost |

#### 6.7.2 WebSocket Fan-Out Pattern

```
                         NATS JetStream
                              │
              ┌───────────────┼───────────────┐
              │               │               │
              ▼               ▼               ▼
     flowpulse.teamA.flows  .teamA.metrics  .teamA.training
              │               │               │
              └───────┬───────┘               │
                      │                       │
              ┌───────▼───────────────────────▼──────┐
              │        API Server: WSGateway          │
              │                                       │
              │  Per-connection goroutine:            │
              │  ┌───────────────────────────────┐   │
              │  │  NATS subscriber for tenant   │   │
              │  │     ↓                         │   │
              │  │  JSON marshal                 │   │
              │  │     ↓                         │   │
              │  │  ws.WriteMessage(TextMessage)  │   │
              │  └───────────────────────────────┘   │
              │                                       │
              │  × N concurrent browser connections   │
              └───────────────────────────────────────┘
                          │
              ┌───────────┼───────────┐
              ▼           ▼           ▼
          Browser 1   Browser 2   Browser 3
```

Each WebSocket connection gets its own NATS subscription. This is a **per-tenant fan-out**: all browsers viewing the same tenant see the same events. The pattern scales horizontally — adding API server replicas distributes WebSocket connections.

### 6.8 Scaling Strategy

#### 6.8.1 Horizontal Scaling Dimensions

```
                    Agents              Aggregators         API Servers
                  (DaemonSet)          (StatefulSet)       (Deployment)
                      │                     │                    │
Scale trigger:   Node count            Ingest rate         WebSocket
                                       (flows/sec)         connections
                      │                     │                    │
Scaling:         Automatic             Manual /             HPA on
                 (1 per node)          config-based         CPU / conn
                      │                     │                    │
Current:         1 per node            1 (single)           2+ replicas
                      │                     │                    │
Target:          1–10,000              1–8 (sharded)        2–20
```

#### 6.8.2 Aggregator Sharding (v2.0 Design)

For clusters larger than ~500 nodes, a single aggregator becomes a bottleneck. The sharding strategy uses a consistent hash ring:

```
     Agent on Node X sends FlowBatch
              │
              ▼
     Hash(tenant_id + node_id) → ring position
              │
              ▼
     Route to Aggregator shard owning that range
              │
     ┌────────┼────────┐────────┐
     ▼        ▼        ▼        ▼
   Agg-0    Agg-1    Agg-2    Agg-3
  (0-63)   (64-127) (128-191)(192-255)

   Each shard:
   - Correlates only its assigned flows
   - Writes to shared ClickHouse
   - Publishes to shared NATS
   - Computes training metrics for its nodes
     (cross-shard aggregation via NATS query)
```

**Design constraints:**
- Flow correlation requires both forward and reverse flows on the same shard → hash by canonical 5-tuple, not node_id
- Training metrics computation needs all nodes → either a coordinator shard or a final aggregation step
- `pkg/aggregator/hashring.go` provides the consistent hash ring implementation (built, not yet wired)

#### 6.8.3 ClickHouse Scaling

| Scale Level | Configuration | Capacity |
|-------------|---------------|----------|
| Single node | 1 server, local SSD | ~100M flows/day, 7-day window |
| Replicated | 2-node ReplicatedMergeTree | HA, ~200M flows/day |
| Sharded + Replicated | 2 shards × 2 replicas (Distributed table) | ~1B flows/day |

### 6.9 Failure Modes & Resilience

#### 6.9.1 Failure Matrix

| Component Fails | Impact | Recovery | Data Loss |
|-----------------|--------|----------|-----------|
| **Agent crash** | No new telemetry from that node; existing eBPF programs remain in kernel | Agent restarts (DaemonSet); re-loads BPF programs; re-attaches TC filters | Flows during downtime are lost (kernel flow_table continues accumulating; data recovered on restart if LRU hasn't evicted) |
| **Agent → Aggregator network** | Flows queue in agent; dropped when buffer full | Retry with backoff; log drop count | Dropped batches not recoverable; gap in ClickHouse |
| **Aggregator crash** | In-flight flows in correlation cache lost; no new writes to CH/NATS | StatefulSet restart; new correlation cache starts empty | Uncorrelated flows lost; ~5s gap in training metrics |
| **ClickHouse down** | Aggregator log errors but continue processing; NATS events still flow | ClickHouse restart; aggregator reconnects automatically | Flows during outage not persisted; real-time dashboard still works via NATS |
| **NATS down** | No real-time WebSocket updates; aggregator logs publish failures | NATS restart; WSGateway reconnects; clients reconnect | Real-time events lost; ClickHouse data unaffected; dashboard falls back to REST polling |
| **API Server crash** | REST and WS unavailable; dashboard shows stale data | Deployment restarts replica; other replicas serve traffic | No data loss; transient unavailability |
| **Web crash** | Dashboard inaccessible | Deployment restarts; static assets served from new pod | No data loss |
| **All storage down** | System is blind but training workloads unaffected | Restart storage; aggregator/server reconnect | Gap in historical data |

#### 6.9.2 Backpressure Handling

```
 Overload scenario: 2,000 nodes sending 10,000 flows/s each
                            │
                            ▼
 ┌─ Agent ─────────────────────────────────────────┐
 │  Flow cache: if cache > max_flows, LRU eviction │
 │  HTTP send: if aggregator returns 5xx or timeout │
 │    → log warning, drop batch, continue           │
 │  Health: agent always stays up                   │
 └─────────────────────────────────────────────────┘
                            │
                            ▼
 ┌─ Aggregator ────────────────────────────────────┐
 │  flowCh buffer: 4,096 batches                   │
 │  If full → drop incoming batch (log warning)     │
 │  metricCh buffer: 1,024 entries                 │
 │  If full → drop incoming metrics                 │
 │  ClickHouse write timeout → log error, skip      │
 │  NATS publish fail → log warning, continue       │
 └─────────────────────────────────────────────────┘
                            │
                            ▼
 ┌─ NATS ──────────────────────────────────────────┐
 │  MaxAge: 5min → old messages auto-expire         │
 │  Memory storage → bounded by stream config       │
 │  Slow consumer → messages dropped per policy     │
 └─────────────────────────────────────────────────┘
```

**Design philosophy:** Every stage has a finite buffer. When a buffer is full, the system drops the oldest/least-critical data and logs a warning. The system never blocks the data plane — an overloaded monitoring pipeline must not affect the training workload.

#### 6.9.3 Graceful Shutdown Sequence

```
 SIGTERM received
       │
       ▼
 1. Stop accepting new HTTP requests (agent/aggregator)
 2. Drain in-flight batches (flush remaining flows)
 3. Final ClickHouse batch.Send()
 4. Close NATS connection (drain pending publishes)
 5. GracefulStop gRPC server (drain RPCs)
 6. Close ClickHouse connection
 7. Exit
```

### 6.10 Security Architecture

#### 6.10.1 Trust Boundaries

```
 ┌─────────────────────────────────────────────────────────────────┐
 │  TRUSTED ZONE (Cluster Internal)                                │
 │                                                                 │
 │  Agent ←→ Aggregator ←→ ClickHouse ←→ API Server ←→ NATS      │
 │  (no TLS within cluster; mTLS recommended for production)       │
 │                                                                 │
 │  Trust model: any pod in the namespace can reach any service    │
 │  Mitigation: NetworkPolicy restricts pod-to-pod communication  │
 └─────────────────────────────────────────────────────────────────┘
                          │
                    ┌─────┴─────┐
                    │  BOUNDARY  │  Ingress controller
                    │  (TLS      │  terminates HTTPS/WSS
                    │  termination)
                    └─────┬─────┘
                          │
 ┌────────────────────────┴────────────────────────────────────────┐
 │  UNTRUSTED ZONE (External)                                      │
 │                                                                 │
 │  Browser ──► Dashboard (:3000)                                  │
 │  Browser ──► API (:8080) with JWT Bearer token                  │
 │  Browser ──► WS (:8080/ws) with tenant_id query param           │
 └─────────────────────────────────────────────────────────────────┘
```

#### 6.10.2 Authentication & Authorization Flow

```
 Browser                    API Server                 ClickHouse
    │                           │                          │
    │  POST /api/v1/auth/token  │                          │
    │  ?tenant_id=team-a        │                          │
    │ ─────────────────────────►│                          │
    │                           │  Sign JWT (HS256)        │
    │  {"token": "eyJ..."}      │  claims: user_id,        │
    │ ◄─────────────────────────│  tenant_id, role, exp    │
    │                           │                          │
    │  GET /api/v1/flows        │                          │
    │  Authorization: Bearer eyJ│                          │
    │ ─────────────────────────►│                          │
    │                           │  Validate JWT            │
    │                           │  Extract tenant_id       │
    │                           │                          │
    │                           │  SELECT ... WHERE        │
    │                           │  tenant_id = 'team-a'    │
    │                           │ ─────────────────────────►
    │                           │                          │
    │                           │  ◄─── results ───────────│
    │  {"flows": [...]}         │                          │
    │ ◄─────────────────────────│                          │
```

#### 6.10.3 Agent Privilege Model

| Capability | Required | Reason |
|------------|----------|--------|
| `CAP_BPF` | Yes | Load and attach eBPF programs |
| `CAP_NET_ADMIN` | Yes | Attach TC filters to network interfaces |
| `CAP_SYS_ADMIN` | Yes | Access `/sys/kernel/debug/tracing` for tracepoints |
| `hostNetwork` | Yes | See all node traffic (not just pod traffic) |
| `hostPID` | Yes | Correlate cgroup IDs to tenant processes |
| `privileged` | Yes (DaemonSet) | Simplified capability granting; can be narrowed with securityContext |

**Risk mitigation:** Agent binary is a static Go binary with no shell, no package manager (distroless image). The eBPF programs are read-only (passthrough — TC_ACT_OK). The agent has no write access to the filesystem beyond `/sys/fs/bpf` (for program pinning).

### 6.11 Capacity Planning & Sizing

#### 6.11.1 Data Volume Estimates

| Cluster Size | Flows/Node/s | Total Flows/s | Flow Record Size | ClickHouse Write Rate | Daily Storage |
|--------------|-------------|---------------|------------------|----------------------|---------------|
| 64 nodes | 500 | 32,000 | ~200B | 6.4 MB/s | ~550 GB |
| 256 nodes | 500 | 128,000 | ~200B | 25.6 MB/s | ~2.2 TB |
| 1,024 nodes | 500 | 512,000 | ~200B | 102 MB/s | ~8.8 TB |
| 2,048 nodes | 500 | 1,024,000 | ~200B | 205 MB/s | ~17.6 TB |

**Note:** ClickHouse compression ratio for this workload is typically 5–10×, so actual disk usage is 5–20% of raw data volume. With 30-day TTL and 10× compression:

| Cluster Size | 30-Day Disk (compressed) |
|-------------|--------------------------|
| 64 nodes | ~1.7 TB |
| 256 nodes | ~6.6 TB |
| 1,024 nodes | ~26 TB |
| 2,048 nodes | ~53 TB |

#### 6.11.2 Resource Sizing Guide

| Component | CPU | Memory | Disk | Per |
|-----------|-----|--------|------|-----|
| Agent | 0.1–0.5 cores | 50–200 MB | — | Node |
| Aggregator | 1–4 cores | 1–4 GB | — | Instance (1 per 500 nodes) |
| API Server | 0.5–2 cores | 256 MB–1 GB | — | Instance |
| Web | 0.2–0.5 cores | 128–256 MB | — | Instance |
| ClickHouse | 4–16 cores | 8–32 GB | SSD (see above) | Instance |
| NATS | 0.5–2 cores | 512 MB–2 GB | — | Instance |
| Redis | 0.2–0.5 cores | 256 MB–1 GB | — | Instance |

#### 6.11.3 Sizing Example: 256-Node Cluster

| Component | Instances | CPU Total | Memory Total | Disk |
|-----------|-----------|-----------|--------------|------|
| Agent | 256 | 128 cores (0.5 each) | 51 GB | — |
| Aggregator | 1 | 4 cores | 4 GB | — |
| API Server | 2 | 2 cores | 1 GB | — |
| Web | 2 | 0.5 cores | 256 MB | — |
| ClickHouse | 1 | 8 cores | 16 GB | 2 TB SSD |
| NATS | 1 | 1 core | 1 GB | — |
| **Total infra** | **263** | **~144 cores** | **~74 GB** | **2 TB** |

Agent overhead per GPU node: 0.5 core + 200 MB — typically <1% of an 8-GPU node with 128 cores and 1 TB RAM.

### 6.12 Technology Choices & Rationale

| Technology | Role | Why This | Alternatives Considered |
|------------|------|----------|------------------------|
| **Go** | Agent, aggregator, API server | Low-latency, small binaries, excellent concurrency (goroutines), strong eBPF library ecosystem (cilium/ebpf) | Rust (higher dev cost, fewer eBPF libraries), Python (too slow for agent), C++ (no need for this complexity) |
| **eBPF** | Kernel instrumentation | Zero-copy telemetry at wire speed; no kernel module needed; safe (verifier); hot-patchable | Kernel modules (dangerous, hard to maintain), DPDK (requires dedicated NIC), netflow/sFlow (too coarse, no RDMA) |
| **ClickHouse** | Analytics storage | Columnar compression (10×), fast aggregation, SQL interface, MergeTree TTL, materialized views, proven at petabyte scale | TimescaleDB (row-oriented, slower for analytics), Elasticsearch (expensive, not columnar), InfluxDB (limited SQL, no joins) |
| **NATS JetStream** | Event bus | Lightweight, embeddable, at-most-once delivery sufficient for real-time, wildcard subjects for tenant routing | Kafka (overkill for this use case, heavy ops burden), Redis Streams (limited routing), RabbitMQ (heavier, AMQP complexity) |
| **Next.js** | Dashboard | React ecosystem, SSR for fast initial load, file-based routing, built-in optimization | Vue/Nuxt (smaller ecosystem), Svelte (fewer component libraries), plain React (no SSR/SSG) |
| **Zustand** | Client state | Minimal boilerplate, excellent React integration, no providers/context needed | Redux (too much boilerplate), MobX (complex), React Context (re-render issues at scale) |
| **JWT (HS256)** | Authentication | Stateless, simple, well-understood; symmetric key sufficient for single-cluster deployment | OAuth2/OIDC (future, for multi-cluster), mTLS (complex for browser clients), API keys (no tenant claims) |
| **Chi** | HTTP router | Lightweight, idiomatic Go, middleware-friendly, no reflection magic | Gin (heavier, framework-ish), gorilla/mux (maintenance concerns), stdlib net/http (missing middleware pattern) |
| **Docker / K8s** | Deployment | Industry standard; DaemonSet perfect for per-node agents; StatefulSet for aggregator state | Nomad (less ecosystem), bare-metal (no orchestration), systemd (no cluster management) |
| **Lima** | macOS eBPF dev | Native Apple Virtualization.framework, fast virtiofs mounts, ARM64 Ubuntu with BTF support | Docker Desktop (no eBPF in LinuxKit), Vagrant (slower, x86 emulation), UTM (manual setup) |

---

## 7. Functional Requirements

### FR-1: Network Flow Capture

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-1.1 | Capture all IPv4 TCP and UDP flows traversing monitored interfaces using eBPF TC programs | P0 |
| FR-1.2 | Extract 5-tuple (src_ip, dst_ip, src_port, dst_port, protocol) for each flow | P0 |
| FR-1.3 | Track per-flow packet count, byte count, first_seen, and last_seen timestamps | P0 |
| FR-1.4 | Detect and parse RoCEv2 (UDP port 4791) BTH headers to extract QP number, ECN marks, and message count | P1 |
| FR-1.5 | Classify flow direction as ingress or egress relative to the node | P0 |
| FR-1.6 | Use LRU hash map with configurable max_flows (default 100,000) to prevent unbounded memory growth | P0 |
| FR-1.7 | Emit new-flow events via BPF ring buffer for low-latency detection | P1 |
| FR-1.8 | Support configurable interface list for TC program attachment | P0 |

### FR-2: CPU & Scheduling Telemetry

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-2.1 | Instrument sched_switch tracepoint to measure per-CPU on-CPU and off-CPU time | P0 |
| FR-2.2 | Count voluntary and involuntary context switches per CPU | P0 |
| FR-2.3 | Instrument softirq_entry/exit tracepoints to measure NET_TX and NET_RX softirq duration | P0 |
| FR-2.4 | Aggregate per-CPU values across all CPUs when reading from percpu_hash maps | P0 |
| FR-2.5 | Read and report per-CPU utilization as a percentage | P0 |

### FR-3: InfiniBand / RDMA Telemetry

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-3.1 | Read IB port counters from sysfs (/sys/class/infiniband) for TX/RX bytes, packets, and errors | P1 |
| FR-3.2 | Instrument ib_post_send and ib_post_recv kernel functions via kprobe for per-QP operation counts | P2 |
| FR-3.3 | Instrument ib_poll_cq for completion latency measurement | P2 |
| FR-3.4 | Emit IB verb events via ring buffer for real-time QP activity monitoring | P2 |

### FR-4: Flow Aggregation & Correlation

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-4.1 | Receive flow batches from multiple agents via HTTP ingest endpoint | P0 |
| FR-4.2 | Correlate forward and reverse flows into bidirectional CorrelatedFlow records using canonical 5-tuple key | P0 |
| FR-4.3 | Aggregate total bytes and packets across both directions | P0 |
| FR-4.4 | Persist correlated flows to ClickHouse in batches with configurable flush interval | P0 |
| FR-4.5 | Publish real-time flow batches to NATS JetStream for WebSocket fan-out | P0 |
| FR-4.6 | Evict stale flows from the correlation cache after configurable timeout | P1 |

### FR-5: Training Metrics Computation

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-5.1 | Compute straggler score based on IB link utilization deviation from median across nodes | P0 |
| FR-5.2 | Compute bubble ratio as the proportion of softirq time to total active CPU time | P0 |
| FR-5.3 | Compute gradient synchronization overhead as kernel CPU time relative to total active time | P0 |
| FR-5.4 | Compute network saturation index as actual throughput divided by theoretical peak bandwidth | P0 |
| FR-5.5 | Compute flow imbalance score as the coefficient of variation of per-flow byte counts | P0 |
| FR-5.6 | Persist computed training metrics to ClickHouse every 5 seconds | P0 |
| FR-5.7 | Publish training metrics to NATS for real-time dashboard updates | P0 |
| FR-5.8 | Identify and list top-10 straggler nodes by deviation | P1 |

### FR-6: Node Metrics Persistence

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-6.1 | Receive node metrics (CPU, IB) from agents via HTTP ingest | P0 |
| FR-6.2 | Persist node metrics to ClickHouse with averaged CPU values and IB counters | P0 |
| FR-6.3 | Maintain a 1-minute rollup materialized view for long-term trend analysis | P1 |
| FR-6.4 | Publish node metrics to NATS for real-time topology updates | P0 |

### FR-7: REST API

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-7.1 | Provide JWT-protected REST endpoints for flows, training metrics, and topology | P0 |
| FR-7.2 | Support query parameters for filtering (node_id, protocol, time range) and pagination (limit, offset, sort) | P0 |
| FR-7.3 | Provide a development token endpoint for local testing without external auth | P1 |
| FR-7.4 | Return flow data with nested key structure matching frontend expectations | P0 |
| FR-7.5 | Enforce tenant isolation via JWT claims — users see only their tenant's data | P0 |

### FR-8: Real-Time WebSocket

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-8.1 | Provide WebSocket endpoint that subscribes to tenant-specific NATS subjects | P0 |
| FR-8.2 | Fan out flow, metric, and training events to connected browser clients | P0 |
| FR-8.3 | Support tenant_id as query parameter for subscription scoping | P0 |
| FR-8.4 | Deliver events as JSON envelopes with subject and data fields | P0 |

### FR-9: Dashboard

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-9.1 | Display overview cards for key training metrics (straggler, bubble, saturation, imbalance, flow count, node count) | P0 |
| FR-9.2 | Display a sortable, searchable flow table with IP addresses, ports, protocol, byte/packet counts | P0 |
| FR-9.3 | Display a topology view showing node health and utilization | P0 |
| FR-9.4 | Display a training-specific dashboard with straggler leaderboard and bandwidth gauges | P0 |
| FR-9.5 | Auto-refresh data every 3 seconds via REST polling + real-time WebSocket updates | P0 |
| FR-9.6 | Show live connection status indicator | P1 |

### FR-10: Multi-Tenancy

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-10.1 | Each agent tags flows and metrics with a configured tenant_id | P0 |
| FR-10.2 | ClickHouse tables are partitioned by tenant_id | P0 |
| FR-10.3 | API enforces tenant_id from JWT claims on all queries | P0 |
| FR-10.4 | NATS subjects are namespaced by tenant_id | P0 |

---

## 8. Data Model & Schema

### 8.1 Domain Objects

#### Flow

| Field | Type | Description |
|-------|------|-------------|
| flow_id | string (UUID) | Unique identifier generated at correlation |
| tenant_id | string | Owning tenant |
| node_id | string | Originating agent node |
| key.src_ip | uint32 | Source IP (network byte order, stored as integer) |
| key.dst_ip | uint32 | Destination IP |
| key.src_port | uint16 | Source port |
| key.dst_port | uint16 | Destination port |
| key.protocol | uint8 | IP protocol (6=TCP, 17=UDP) |
| direction | uint8 | 0=unknown, 1=ingress, 2=egress |
| packets | uint64 | Packet count |
| bytes | uint64 | Byte count |
| first_seen | DateTime64(3) | First packet timestamp |
| last_seen | DateTime64(3) | Last packet timestamp |
| rdma.qp_number | uint32 | RDMA Queue Pair number (0 if not RDMA) |
| rdma.dest_qp | uint32 | Destination QP |
| rdma.rdma_msg_rate | uint64 | RDMA messages per interval |
| rdma.retransmissions | uint64 | RDMA retransmit count |
| rdma.ecn_marks | uint64 | ECN-marked packet count |
| rdma.cnp_count | uint64 | Congestion Notification Packets received |

#### NodeMetrics

| Field | Type | Description |
|-------|------|-------------|
| node_id | string | Node identifier |
| tenant_id | string | Owning tenant |
| cpu_metrics[] | array | Per-core CPU telemetry |
| cpu_metrics[].core_id | uint32 | Logical core number |
| cpu_metrics[].utilization | float64 | CPU utilization percentage |
| cpu_metrics[].kernel_pct | float64 | Kernel mode percentage |
| cpu_metrics[].user_pct | float64 | User mode percentage |
| cpu_metrics[].softirq_pct | float64 | SoftIRQ percentage (NET_TX/RX) |
| cpu_metrics[].context_switches | uint64 | Total context switches |
| ib_metrics.tx_bytes | uint64 | IB transmit bytes |
| ib_metrics.rx_bytes | uint64 | IB receive bytes |
| ib_metrics.link_utilization_pct | float64 | IB link utilization |
| ib_metrics.port_rcv_errors | uint64 | IB receive errors |
| timestamp | DateTime64(3) | Collection timestamp |

#### TrainingMetrics

| Field | Type | Description |
|-------|------|-------------|
| tenant_id | string | Tenant identifier |
| straggler_score | float64 | 0–100: how much the slowest node deviates from median (see §13) |
| bubble_ratio | float64 | 0–100: percentage of CPU time in softirq vs. active (network wait) |
| gradient_sync_overhead_pct | float64 | 0–100: kernel CPU time as fraction of total active |
| network_saturation_index | float64 | 0–100: actual throughput vs. theoretical peak |
| imbalance_score | float64 | 0–100: coefficient of variation of per-flow byte counts |
| stragglers[] | array | Top-10 straggler nodes by deviation |
| timestamp | DateTime64(3) | Computation timestamp |

### 8.2 ClickHouse Schema

#### Table: `flows`
- **Engine:** MergeTree
- **Partition:** `(tenant_id, toDate(timestamp))`
- **Order:** `(tenant_id, flow_id, timestamp)`
- **TTL:** 30 days
- **Columns:** tenant_id, flow_id, node_id, src_ip, dst_ip, src_port, dst_port, protocol, direction, packets, bytes, first_seen, last_seen, rdma_qp, rdma_dest_qp, rdma_msg_rate, rdma_retransmits, rdma_ecn_marks, rdma_cnp_count, total_bytes, total_pkts, timestamp

#### Table: `node_metrics`
- **Engine:** MergeTree
- **Partition:** `(tenant_id, toDate(timestamp))`
- **Order:** `(tenant_id, node_id, timestamp)`
- **TTL:** 7 days
- **Columns:** tenant_id, node_id, cpu_utilization, cpu_kernel_pct, cpu_softirq_pct, context_switches, ib_tx_bytes, ib_rx_bytes, ib_link_util_pct, ib_errors, timestamp

#### Table: `training_metrics`
- **Engine:** MergeTree
- **Partition:** `(tenant_id, toDate(timestamp))`
- **Order:** `(tenant_id, timestamp)`
- **TTL:** 30 days
- **Columns:** tenant_id, straggler_score, bubble_ratio, gradient_sync_overhead_pct, network_saturation_index, imbalance_score, timestamp

#### Materialized View: `metrics_1m`
- **Engine:** SummingMergeTree
- **Source:** `node_metrics`
- **Granularity:** 1-minute windows
- **Aggregates:** avg/max CPU, avg IB util, sum TX/RX bytes, sum context switches, sample count
- **Purpose:** Long-term trend analysis and capacity planning dashboards

---

## 9. API Specification

### 9.1 Authentication

| Aspect | Detail |
|--------|--------|
| Scheme | Bearer JWT (HS256) |
| Secret | Loaded from `FLOWPULSE_JWT_SECRET` env var; dev fallback: `flowpulse-dev-secret-change-in-production` |
| Claims | `user_id` (string), `tenant_id` (string, required), `role` (admin/operator/viewer), `iat`, `exp` |
| Token lifetime | Configurable, default 24 hours |

### 9.2 Endpoints

#### Public (No Auth)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Returns `{"status":"ok"}` — used for readiness/liveness probes |
| POST | `/api/v1/auth/token?tenant_id={id}` | Issues a dev JWT for the specified tenant (defaults to `local-dev`) |

#### Protected (JWT Required)

| Method | Path | Query Parameters | Response |
|--------|------|------------------|----------|
| GET | `/api/v1/flows` | `node_id`, `limit` (1–10000, default 100), `offset`, `protocol`, `start_time`, `end_time`, `sort_by` (bytes/packets/timestamp/total_bytes/first_seen/last_seen), `sort_order` (asc/desc) | `{ "flows": Flow[], "total_count": number }` |
| GET | `/api/v1/metrics/training` | `window` (1m/5m/1h, default 5m) | `TrainingMetrics` object |
| GET | `/api/v1/topology` | — | `{ "nodes": TopologyNode[] }` where TopologyNode = `{ node_id, cpu_avg, ib_util_pct, tx_bytes, rx_bytes, status }` |

#### WebSocket

| Path | Query Parameters | Behavior |
|------|------------------|----------|
| `/ws` (configurable) | `tenant_id` (required) | Upgrades to WebSocket; subscribes to NATS subjects `flowpulse.{tenant}.flows`, `.metrics`, `.training`; sends JSON `{ "subject": string, "data": object }` |

#### Aggregator Ingest (Internal)

| Method | Path | Port | Body | Description |
|--------|------|------|------|-------------|
| POST | `/api/v1/ingest/flows` | 9092 | `FlowBatch` JSON | Agent sends batched flows |
| POST | `/api/v1/ingest/metrics` | 9092 | `NodeMetrics` JSON | Agent sends node metrics |
| GET | `/healthz` | 9092 | — | Aggregator health |

### 9.3 Error Responses

| HTTP Status | Condition |
|-------------|-----------|
| 401 | Missing or invalid JWT |
| 403 | Valid JWT but missing tenant_id claim |
| 400 | Malformed request body or invalid query parameters |
| 500 | Internal error (ClickHouse query failure, etc.) |

---

## 10. eBPF Instrumentation Specification

### 10.1 flow_tracker.c — Network Flow Capture

| Aspect | Detail |
|--------|--------|
| **Program type** | TC classifier (sched_cls) |
| **Attach point** | TC ingress and egress on configured network interfaces |
| **Sections** | `tc/ingress` (flowpulse_ingress), `tc/egress` (flowpulse_egress) |
| **Maps** | `flow_table` (LRU hash, max 100K entries), `pkt_counter` (percpu_array), `flow_events` (ringbuf, 256KB) |
| **Packet parsing** | Uses `bpf_skb_load_bytes()` for verifier-safe access; parses Ethernet → IPv4 → TCP/UDP headers |
| **RDMA detection** | Checks for UDP dst_port 4791 (RoCEv2); parses BTH header for dest_qp, flags (ECN bit 0x80) |
| **Flow direction** | Determined by program section: ingress=1, egress=2 |
| **Action** | Always returns TC_ACT_OK (passthrough — does not modify or drop packets) |

### 10.2 cpu_sched.c — CPU Scheduling & SoftIRQ

| Aspect | Detail |
|--------|--------|
| **Program type** | Tracepoint |
| **Attach points** | `sched/sched_switch`, `irq/softirq_entry`, `irq/softirq_exit` |
| **Maps** | `cpu_stats` (percpu_hash), `softirq_start` (percpu_array), `softirq_stats` (percpu_hash), `cpu_events` (ringbuf) |
| **sched_switch** | Records on_cpu_ns and off_cpu_ns per CPU; counts voluntary (prev_state != 0) and involuntary switches |
| **softirq** | Tracks entry timestamp; on exit, accumulates total_ns and net_rx_ns/net_tx_ns (vec 3=NET_TX, vec 4=NET_RX) |

### 10.3 ib_verbs.c — InfiniBand Verb Tracing (Future)

| Aspect | Detail |
|--------|--------|
| **Program type** | kprobe/kretprobe |
| **Attach points** | `ib_post_send`, `ib_post_recv`, `ib_poll_cq` |
| **Maps** | `ib_qp_stats` (hash), `pending_sends` (hash by wr_id), `ib_events` (ringbuf) |
| **Status** | Built but not loaded by the agent (roadmap item) |

### 10.4 Resource Limits

| Resource | Budget |
|----------|--------|
| CPU overhead per node | <1% under 100Gbps line rate |
| Memory (BPF maps) | ~16MB (100K flow entries × 160B each) |
| Ring buffer | 256KB per program (flow_events, cpu_events) |
| Kernel version requirement | 5.8+ (BTF, ringbuf, TC BPF, tracepoints) |

---

## 11. Dashboard & UI Specification

### 11.1 Technology

- **Framework:** Next.js 14 (React, TypeScript)
- **State management:** Zustand
- **Real-time:** Native WebSocket with exponential backoff reconnect
- **Data fetching:** REST polling every 3 seconds + WebSocket push
- **Styling:** Tailwind CSS

### 11.2 Pages

#### Dashboard (/)

| Section | Data Source | Content |
|---------|------------|---------|
| Overview cards | REST `/api/v1/metrics/training` + local state | Straggler score, Bubble ratio, Network saturation, Imbalance, Total flows, Node count, Connection status |
| Flow table | REST `/api/v1/flows` + WS `.flows` | Recent flows with columns: Source IP, Dest IP, Source Port, Dest Port, Protocol, Packets, Bytes, Duration |

#### Flows (/flows)

Full-page flow table with all query parameters: sorting, filtering by protocol, search.

#### Topology (/topology)

| Component | Data Source | Content |
|-----------|------------|---------|
| TopologyView | REST `/api/v1/topology` + WS `.metrics` | Node grid/graph with color-coded health status |
| TopologyHeatmap | Same | Heatmap visualization of CPU and IB utilization across nodes |

#### Training (/training)

| Component | Data Source | Content |
|-----------|------------|---------|
| TrainingDashboard | REST `/api/v1/metrics/training` + WS `.training` | All 5 metric gauges with trend indicators |
| StragglerLeaderboard | Embedded in training metrics | Top straggler nodes ranked by deviation |
| BandwidthGauges | Node metrics | Per-node and aggregate bandwidth utilization |
| CollectiveTimeline | WS events | Timeline visualization of collective operations |

#### Alerts (/alerts)

| Component | Status | Content |
|-----------|--------|---------|
| AlertPanel | Frontend implemented; backend not yet wired | Alert list with severity, category, affected nodes, acknowledgment actions |

### 11.3 Real-Time Update Flow

```
Browser loads page
  → useDataLoader: REST poll (3s interval) fills Zustand store
  → useWebSocket: connects to /ws?tenant_id=local-dev
       ↓ on message
       ├─ subject ends with .flows    → addRealtimeFlow (append, cap at 10K)
       ├─ subject ends with .training → setTrainingMetrics
       ├─ subject ends with .metrics  → setTopologyNodes + setConnected(true)
       └─ subject ends with .alert*   → addRealtimeAlert / updateAlert
```

---

## 12. Deployment Specification

### 12.1 Docker Compose (Local Development)

| Service | Image | Ports | Dependencies |
|---------|-------|-------|-------------|
| clickhouse | clickhouse/clickhouse-server:24-alpine | 8123, 9000 | — |
| nats | nats:2-alpine | 4222, 8222 | — |
| redis | redis:7-alpine | 6379 | — |
| aggregator | flowpulse-aggregator (built) | 9091, 9092 | clickhouse, nats |
| server | flowpulse-server (built) | 8080, 9090 | clickhouse, nats, redis |
| web | flowpulse-web (built) | 3000 | server |

**Agent:** Runs natively in a Lima VM (or any Linux host with eBPF support) pointing to the aggregator's HTTP ingest port.

### 12.2 Kubernetes (Production)

| Component | K8s Resource | Replicas | Special Requirements |
|-----------|-------------|----------|---------------------|
| Agent | DaemonSet | 1 per node | `privileged: true`, `hostNetwork: true`, `hostPID: true`; mounts `/sys/fs/bpf`, `/sys/kernel/debug`; node selector: `flowpulse.io/monitor=true` |
| Aggregator | StatefulSet | 1–3 | Needs ClickHouse and NATS connectivity |
| Server | Deployment | 2+ | Needs ClickHouse, NATS, Redis connectivity |
| Web | Deployment | 2+ | Needs server connectivity |
| ClickHouse | StatefulSet (or external) | 1+ | 20Gi PVC minimum |
| NATS | StatefulSet (or external) | 1+ | JetStream enabled |
| Redis | StatefulSet (or external) | 1 | Optional (for future caching) |

**Helm chart** provides: namespace creation, ServiceAccounts, RBAC (ClusterRole for agent, Role for server), Secrets, ConfigMaps, PodDisruptionBudgets, NetworkPolicies (default-deny + explicit allow), optional Ingress.

### 12.3 Lima VM (macOS eBPF Development)

| Aspect | Detail |
|--------|--------|
| VMType | vz (Apple Virtualization.framework) |
| OS | Ubuntu 24.04 server (arm64) |
| Resources | 4 CPU, 4GB RAM, 20GB disk |
| Mount | `~/flowpulse` → `/home/{user}/flowpulse` (virtiofs, writable) |
| Provisioned tools | build-essential, clang, llvm, libbpf-dev, linux-headers, Go 1.22, Node.js 20 |
| Network | VM reaches Mac host at 192.168.5.2 (default gateway) |

---

## 13. Algorithms & Metrics Computation

### 13.1 Straggler Score

**Purpose:** Detects nodes whose network behavior deviates from the cluster median, indicating they may be bottlenecking collective operations.

**Algorithm:**
1. Collect `IBMetrics.LinkUtilizationPct` from all reporting nodes for the current tenant.
2. Sort values and compute the **median**.
3. Compute the **absolute deviation** of each node from the median.
4. The straggler score is `(max_deviation / median) × 100`.
5. Nodes with deviation > 20 percentage points are flagged as stragglers (sorted by deviation, capped at 10).

**Interpretation:**
- 0–10: Healthy, balanced utilization
- 10–30: Mild imbalance, investigate if persistent
- 30+: Significant straggler, likely impacting training throughput

### 13.2 Bubble Ratio

**Purpose:** Measures the fraction of CPU time spent in network softirq processing vs. productive (user) work. High values indicate GPUs are waiting for network collectives.

**Algorithm:**
1. Sum `SoftIRQPct` and `UserPct` across all cores of all reporting nodes.
2. Bubble ratio = `totalSoftIRQ / (totalSoftIRQ + totalUser) × 100`.

**Interpretation:**
- 0–20%: Normal — network overhead is a small fraction
- 20–50%: Elevated — collectives may be poorly sized or scheduled
- 50%+: Severe — network is a primary bottleneck (or training is compute-light)

### 13.3 Gradient Synchronization Overhead

**Purpose:** Measures kernel CPU time as a proxy for time spent in NCCL collective operations (kernel-mode network stack processing).

**Algorithm:**
1. Sum `KernelPct` and `Utilization` across all cores.
2. Overhead = `totalKernel / totalActive × 100`.

**Interpretation:**
- 0–10%: Normal kernel activity
- 10–25%: Elevated — may indicate inefficient collective scheduling
- 25%+: High — investigate NCCL configuration and network stack tuning

### 13.4 Network Saturation Index

**Purpose:** Measures actual aggregate throughput as a percentage of theoretical peak bandwidth.

**Algorithm:**
1. Sum `TotalBytes` across all correlated flows in the measurement window.
2. Compute actual bytes per second: `totalBytes / windowSeconds`.
3. Saturation = `(actualBps / peakBandwidthBps) × 100`.
4. Default peak: 50 GB/s (400 Gbps NDR InfiniBand).

**Interpretation:**
- 0–30%: Under-utilized — check if training is compute-bound or flows are not captured
- 30–70%: Healthy utilization
- 70–90%: High utilization — approaching saturation
- 90%+: Saturated — likely causing congestion and ECN marks

### 13.5 Imbalance Score

**Purpose:** Measures how evenly traffic is distributed across flows. In ideal all-to-all collectives, all flows should carry similar volumes.

**Algorithm:**
1. Collect `TotalBytes` from each correlated flow in the window.
2. Compute mean and standard deviation.
3. Imbalance = `(stddev / mean) × 100` (coefficient of variation).

**Interpretation:**
- 0–15%: Well-balanced traffic
- 15–40%: Moderate imbalance — may be expected for mixed collective types
- 40%+: Significant imbalance — investigate asymmetric communication patterns

---

## 14. Non-Functional Requirements

### 14.1 Performance

| Requirement | Target |
|-------------|--------|
| Agent CPU overhead | <1% per node at 100 Gbps line rate |
| Agent memory | <50 MB resident (excluding BPF maps) |
| Aggregator throughput | 100,000 flows/second sustained ingest |
| ClickHouse write latency | <100ms per batch (5-second flush) |
| API query latency (p99) | <500ms for flow queries with 10M rows |
| WebSocket event latency | <100ms from NATS publish to browser |
| Dashboard render | <2 seconds initial load; <100ms incremental updates |

### 14.2 Scalability

| Dimension | v1.0 Target | Design Ceiling |
|-----------|-------------|----------------|
| Nodes | 64–2,048 | 10,000+ (with sharded aggregator) |
| Flows per node | 10,000 concurrent | 100,000 (with LRU eviction) |
| Tenants | 1–10 | 100+ (partition per tenant) |
| Retention | 30 days flows, 7 days node metrics | Configurable via TTL |

### 14.3 Reliability

| Requirement | Implementation |
|-------------|----------------|
| Agent crash isolation | Agent failure does not affect node workloads (eBPF programs remain in kernel, agent restart re-attaches) |
| Aggregator restart | Flows in transit are lost (acceptable); ClickHouse data persists |
| ClickHouse persistence | PVC-backed volumes with TTL-based cleanup |
| NATS durability | JetStream with memory storage, 5-minute max age (real-time events are ephemeral) |

### 14.4 Security

| Requirement | Implementation |
|-------------|----------------|
| API authentication | JWT bearer tokens (HS256); token endpoint for dev; production should integrate with IdP |
| Tenant isolation | All queries filtered by tenant_id from JWT claims |
| Agent privilege | Runs as root/privileged for eBPF; host-network for full flow visibility |
| Network policies | Default-deny in Kubernetes; explicit allow rules per component pair |
| Secrets | JWT secret via env var; ClickHouse/NATS credentials via Kubernetes secrets |

### 14.5 Observability

| Requirement | Implementation |
|-------------|----------------|
| Agent health | HTTP /healthz on port 8081; Prometheus metrics (flowpulse_agent_flows_tracked) |
| Aggregator health | HTTP /healthz on port 9092; structured JSON logs (zerolog) |
| Server health | HTTP /healthz on port 8080 |
| Logging | Structured JSON (zerolog) with millisecond timestamps; log level configurable |

---

## 15. Known Gaps & Roadmap

### 15.1 Current Gaps (v1.0)

| Gap | Description | Impact | Priority |
|-----|-------------|--------|----------|
| **Alerts system** | Frontend has alerts UI and types; backend has no alert endpoints, rules engine, or notification system | Operators cannot set thresholds or receive notifications | P1 |
| **ib_verbs eBPF** | Program is built but not loaded by the agent | Per-QP latency and RDMA verb-level telemetry is not available | P2 |
| **TC auto-attach** | Agent loads TC programs but does not attach them to interfaces; requires manual `tc filter` commands | Deployment friction; flows not captured until manually attached | P0 (fix) |
| **Tenant CRUD API** | Handlers exist but routes not registered | Cannot manage tenants via API | P2 |
| **Redis usage** | In compose and config but not connected in API server | No caching layer for frequent queries | P2 |
| **KernelPct/UserPct** | Agent does not populate these CPU metric fields from eBPF data | Gradient sync overhead metric may read as 0 | P1 (fix) |
| **gRPC transport** | Server and listener exist but no protobuf services registered | Documented as gRPC but actually HTTP | P2 (either implement or remove) |
| **Collective tagging** | `CollectiveTagger` and `StragglerDetector` modules exist but are not wired into the aggregator pipeline | Advanced training-aware analysis unavailable | P2 |

### 15.2 Roadmap

| Phase | Focus | Key Deliverables |
|-------|-------|-----------------|
| **v1.1** | Operational readiness | TC auto-attach in agent; alerts backend (rules, evaluation, email/webhook notification); fix KernelPct/UserPct; tenant API routes |
| **v1.2** | Advanced analytics | ib_verbs loading; collective tagging (AllReduce/AllGather detection); multi-signal straggler detector; per-QP latency dashboard |
| **v2.0** | Scale & enterprise | Sharded aggregator (consistent hash ring); gRPC transport with backpressure; OIDC/SAML authentication; role-based dashboard views; ClickHouse cluster support; Grafana plugin |

---

## 16. Glossary

| Term | Definition |
|------|------------|
| **AllReduce** | Collective operation where all nodes contribute a tensor and receive the sum; the most common operation in data-parallel training |
| **BTH** | Base Transport Header — the RDMA header in RoCEv2 packets, containing QP numbers and opcode |
| **Bubble** | Idle time in the training pipeline where GPUs wait for network collectives to complete |
| **CNP** | Congestion Notification Packet — sent by a receiver to throttle a sender when ECN is triggered |
| **CorrelatedFlow** | A bidirectional flow created by merging the forward (A→B) and reverse (B→A) unidirectional flows |
| **DaemonSet** | Kubernetes resource that runs one pod per node, used for the eBPF agent |
| **eBPF** | Extended Berkeley Packet Filter — a Linux kernel technology for safe, programmable instrumentation |
| **ECN** | Explicit Congestion Notification — a mechanism where switches mark packets instead of dropping them |
| **InfiniBand (IB)** | High-speed interconnect fabric used in GPU clusters, typically 200–400 Gbps per port |
| **NATS JetStream** | Persistent messaging layer within NATS, used for reliable event streaming |
| **NCCL** | NVIDIA Collective Communications Library — handles GPU-to-GPU communication in distributed training |
| **NDR** | Next Data Rate — InfiniBand speed standard at 400 Gbps per port |
| **QP** | Queue Pair — the fundamental RDMA connection abstraction, analogous to a socket |
| **RDMA** | Remote Direct Memory Access — zero-copy network transfer bypassing the kernel, used by NCCL over InfiniBand |
| **RoCEv2** | RDMA over Converged Ethernet v2 — RDMA transport encapsulated in UDP (port 4791) |
| **SoftIRQ** | Software interrupt in the Linux kernel; NET_TX and NET_RX softirqs handle network packet processing |
| **Straggler** | A node that performs significantly slower than its peers, blocking collective operations |
| **TC** | Traffic Control — Linux kernel subsystem for packet classification and queuing; used to attach eBPF programs to network interfaces |
| **Tenant** | An isolated organizational unit (team or training job) within a shared cluster |
