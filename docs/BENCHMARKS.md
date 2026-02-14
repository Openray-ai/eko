# Benchmarking Suite

Comprehensive Go benchmark and memory profiling suite covering every component in the tokenization pipeline.

## Quick Start

```bash
# Run all micro-benchmarks
make bench

# Run memory ceiling tests
make bench-memory

# Save results for later comparison
make bench-save
```

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make bench` | Run all micro-benchmarks with `-benchmem -count=5` |
| `make bench-save` | Run and save timestamped results to `benchmarks/` |
| `make bench-compare` | Compare `benchmarks/baseline.txt` vs `benchmarks/current.txt` via benchstat |
| `make bench-profile-cpu` | Generate CPU profile and open in pprof |
| `make bench-profile-mem` | Generate memory profile and open in pprof |
| `make bench-memory` | Run memory ceiling tests (`TestMemory*`) |
| `make bench-ceiling` | Run 512MB boundary probe (`TestMemoryCeiling*`) |

## Benchmark Files

### Micro-benchmarks (fast, run on every change)

| File | Package | What it measures |
|------|---------|------------------|
| `internal/core/tokenizer/vault_bench_test.go` | tokenizer | VaultManager GetOrCreate (new/cache/scale/concurrent), Cleanup sweep, Store, GetToken, ReverseTokens |
| `internal/core/tokenizer/tokenizer_bench_test.go` | tokenizer | Per-generator (email, phone, BVN, account, CC, IBAN, default), cache hit, emailSubTokens, concurrent generation |
| `internal/core/detector/detector_bench_test.go` | detector | By input size (100B-1MB), violation density (0-100), pattern count (1-50), deduplication, concurrent |
| `internal/core/tokenizer/resolver_bench_test.go` | tokenizer | By token count (1-1000), by body size (1KB-1MB), empty/no-token fast paths |
| `internal/core/sanitizer/sanitizer_bench_test.go` | sanitizer | Full pipeline: clean text, single email, mixed PII, dense PII (50 emails), by input size, concurrent |

### Memory and stress tests (longer-running, separate)

| File | Package | What it measures |
|------|---------|------------------|
| `benchmarks/memory_test.go` | benchmarks | Vault growth, token growth, cleanup recovery, vault matrix, 512MB ceiling probe, concurrent sessions, growing vault, large payload |

## Baseline Results (Apple M2 Pro, 10 cores)

### Vault Manager

| Operation | Latency | Allocs | Notes |
|-----------|---------|--------|-------|
| GetOrCreate (new) | 528 ns | 6 | Map allocation for new vault |
| GetOrCreate (cache hit) | 107 ns | 0 | Flat from 10 to 10K vaults — O(1) confirmed |
| GetOrCreate (256 goroutines) | 284 ns | 0 | Low mutex contention |
| Cleanup (10K vaults, 50% expired) | 459 us | 0 | Linear sweep |
| Vault.Store | 49 ns | 0 | Map insert |
| Vault.GetToken | 14 ns | 0 | Map read |
| ReverseTokens (10K tokens) | 349 us | 34 | Full map copy on every call |

### Token Generation

| Generator | Latency | Allocs | Notes |
|-----------|---------|--------|-------|
| Email | 1.94 us | 22 | Most expensive (sub-tokens + string manipulation) |
| Phone (Nigerian) | 1.31 us | 16 | |
| Phone (Kenyan) | 1.37 us | 16 | |
| Numeric (BVN) | 1.11 us | 14 | Simplest numeric |
| Numeric (Account) | 1.14 us | 14 | |
| Credit Card | 1.17 us | 14 | |
| IBAN | 1.24 us | 14 | |
| Default | 1.23 us | 15 | |
| **Cache hit** | **23 ns** | **0** | Map lookup only |
| **Credential rejection** | **2.5 ns** | **0** | Type check only |

### Detection

| Input Size | Latency | Throughput | Notes |
|------------|---------|------------|-------|
| 100 B | 20 us | 5 MB/s | Goroutine overhead dominates |
| 1 KB | 51 us | 19 MB/s | |
| 10 KB | 278 us | 36 MB/s | |
| 100 KB | 3.75 ms | 27 MB/s | |
| 1 MB | 34.6 ms | 29 MB/s | Regex-bound |

### Resolver

Replaced the O(n\*m) `strings.ReplaceAll` loop with a single-pass `strings.NewReplacer` (Aho-Corasick trie). This also eliminated a cascading replacement bug where replacement text containing a shorter token would be re-matched by later passes.

#### Before (O(n\*m) strings.ReplaceAll loop)

| Tokens | Body Size | Latency | Throughput | Allocs | Notes |
|--------|-----------|---------|------------|--------|-------|
| 1 | 10 KB | 18 us | 543 MB/s | 8 | |
| 10 | 10 KB | 128 us | 80 MB/s | 26 | |
| 50 | 10 KB | 769 us | 14 MB/s | 106 | |
| 100 | 10 KB | 1.75 ms | 6.9 MB/s | 205 | |
| 500 | 10 KB | 16.1 ms | 1.3 MB/s | 1006 | **Latency wall** |
| 1000 | 10 KB | 51.5 ms | 0.64 MB/s | 2011 | Unacceptable for response path |
| 10 | 1 MB | 11.9 ms | 84 MB/s | 27 | Body size scales linearly |

#### After (single-pass strings.NewReplacer)

| Tokens | Body Size | Latency | Throughput | Allocs | Speedup |
|--------|-----------|---------|------------|--------|---------|
| 1 | 10 KB | 24 us | 421 MB/s | 18 | 0.8x (trie build overhead) |
| 10 | 10 KB | 26 us | 389 MB/s | 38 | **4.9x** |
| 50 | 10 KB | 42 us | 258 MB/s | 130 | **18x** |
| 100 | 10 KB | 54 us | 222 MB/s | 240 | **32x** |
| 500 | 10 KB | 172 us | 124 MB/s | 1126 | **94x** |
| 1000 | 10 KB | 329 us | 100 MB/s | 2230 | **157x** |
| 10 | 1 MB | 2.2 ms | 448 MB/s | 38 | **5.4x** |

The scaling is now dominated by body size (O(m)), not token count. At 1000 tokens: **51ms down to 329us**.

### Memory Footprint

| Configuration | Bytes/Token | Notes |
|---------------|-------------|-------|
| 10K vaults, 10 tokens each | ~333 | Vault overhead amortized |
| 1 vault, 100K tokens | ~222 | Steady state per-token cost |
| Max tokens at 512MB (~400MB usable) | ~1.2M (with vaults) | Theoretical ceiling |

## Key Findings

1. **Resolver bottleneck fixed.** The original O(n\*m) `strings.ReplaceAll` loop was the scalability wall (51ms at 1000 tokens). Replaced with `strings.NewReplacer` which builds an Aho-Corasick trie for single-pass O(m) resolution. Result: 157x faster at 1000 tokens (329us). Also eliminated a cascading replacement bug.

2. **Default limits are unreachable.** 10K vaults x 100K tokens/vault = 1 billion theoretical tokens, but at ~333 bytes/token the 512MB container OOMs at ~1.2M total tokens. The defaults should be lowered to match the memory reality.

3. **Vault lookups are excellent.** O(1) map access with 0 allocs on cache hit. No degradation up to 10K vaults. Concurrent access at 256 goroutines adds only ~170 ns of mutex overhead.

4. **Token generation is allocation-heavy.** Email tokenization allocates 22 objects per call. For high-density PII inputs (50+ emails), pooling `[]rune` buffers or pre-sizing maps could reduce GC pressure.

5. **ReverseTokens copies the full map on every resolve.** At 10K tokens this costs 349 us and 656 KB per call. Now that the resolver itself is O(m), this map copy is the next optimization target for sessions with many tokens.

6. **Detection goroutine overhead.** On small inputs (<1 KB), goroutine spawn/join cost likely exceeds the regex work itself. Pattern count scaling is sub-linear (1 pattern = 211 us, 50 patterns = 1.23 ms).

## Comparing Results Over Time

```bash
# Step 1: Create a baseline
make bench-save
cp benchmarks/bench-YYYYMMDD-HHMMSS.txt benchmarks/baseline.txt

# Step 2: Make changes, then create current
make bench-save
cp benchmarks/bench-YYYYMMDD-HHMMSS.txt benchmarks/current.txt

# Step 3: Compare (requires golang.org/x/perf/cmd/benchstat)
make bench-compare
```

Install benchstat:
```bash
go install golang.org/x/perf/cmd/benchstat@latest
```

## Profiling

```bash
# CPU profile — opens pprof web UI on :8081
make bench-profile-cpu

# Memory profile — opens pprof web UI on :8081
make bench-profile-mem
```

From within pprof, useful commands:
- `top20` — hottest functions
- `web` — call graph visualization
- `list FunctionName` — annotated source
