# 🌐 PortPulse

A high-performance Cloud-Native API Gateway written in Go that acts as the unified edge entrypoint for distributed microservices, connecting 32-shard lock-striped rate limiting, Layer-4 reverse proxy load balancing, and asynchronous distributed tracing.

---

## 🏗️ Architecture

PortPulse unifies three foundational systems infrastructure components into an end-to-end distributed microservice platform:

```
                       [ Incoming Client Traffic ]
                                   │
                                   ▼
                       ┌───────────────────────┐
                       │   PortPulse Gateway   │ (HTTP :8080)
                       └───────────┬───────────┘
                                   │
          ┌────────────────────────┼────────────────────────┐
          │ 1. Rate-Limit Check    │ 2. Reverse Proxy       │ 3. Async Telemetry
          ▼                        ▼                        ▼
┌───────────────────┐    ┌───────────────────┐    ┌───────────────────┐
│     ByteCache     │    │     GoBalance     │    │    TracePulse     │
│ (Raw TCP :6379)   │    │  (TCP Proxy :8000)│    │  (gRPC :50051)    │
│                   │    │                   │    │                   │
│ 32-Shard Striped  │    │ Layer-4 TCP Load  │    │ Non-blocking gRPC │
│ In-Memory KV with │    │ Balancer routing  │    │ Span ingestion    │
│ Parallel TTL Evict│    │ to backend pools  │    │ collector         │
│ (56M+ reads/sec)  │    │ (26 ns/op, 0 alloc│    │ (240M+ spans/sec) │
└───────────────────┘    └───────────────────┘    └───────────────────┘
```

---

## ✨ Engineering Highlights

* **Multi-Tier Rate Limiting Architecture:**
  * **L1 In-Memory Sharding (`shared_limiter.go`):** 32 independent partitions indexed via FNV-1a hashing and bitwise masking (`hash & 31`), eliminating mutex lock contention across multi-core systems.
  * **L2 Distributed Cache (`limiter.go`):** Out-of-process TCP communication with ByteCache featuring automated **fail-open resilience** to safeguard uptime during downstream cache network partitions.
* **32KB Buffer Pool Recycling (`proxy.go`):** Custom `sync.Pool` implementing `httputil.BufferPool`, eliminating heap thrashing by recycling 32KB copy buffers across reverse-proxy streams. Slashed per-request memory allocation by **84.3%**.
* **Distributed Observability (`tracer.go`):** Non-blocking gRPC client intercepting HTTP requests, computing end-to-end request latency, and asynchronously shipping Protocol Buffer spans to TracePulse.
* **100% Race-Free Concurrency:** Fully verified under the Go race detector (`go test -race`).

---

## 📊 Benchmark Results

Benchmarked on an **Intel® Core™ Ultra 5 125H (18 cores)** under Linux/WSL:

### 1. 32-Shard Parallel Rate Limiter (`BenchmarkShardedRateLimiter`)
```text
BenchmarkShardedRateLimiter-18   7,953,666 ops   143.1 ns/op   0 B/op   0 allocs/op
```
* Sustains **~8 Million parallel rate-limiting decisions per second** across 18 concurrent CPU threads with **zero heap memory allocations**.

### 2. Reverse Proxy Throughput & Buffer Pool Optimization (`BenchmarkPortPulse`)
| Configuration | Operations | Latency | Memory / Op | Allocs / Op |
| :--- | :--- | :--- | :--- | :--- |
| **Standard Unpooled Proxy** | 16,695 ops | 71.7 µs/op | 38,629 B/op | 58 allocs/op |
| **With 32KB `sync.Pool` BufferPool** | **21,289 ops** | **57.1 µs/op** | **6,073 B/op** | **58 allocs/op** |
| **Net Optimization Gain** | **+27.5% Throughput** | **~15 µs Faster** | **-84.3% Memory Drop** | **Zero Buffer Churn** |

---

## 🚀 Getting Started

### 1. Clone & Build
```bash
git clone https://github.com/PrimeFold/portpulse.git
cd portpulse
go build -v .
```

### 2. Run the Gateway
```bash
./portpulse
```

### 3. Run Test Suite with Race Detector
```bash
go test -v -race .
```

### 4. Run Performance Benchmarks
```bash
go test -bench=. -benchmem -run=^$ .
```
