# authzcache stress test

Measures the ceiling of the edge container itself — max RPS and memory growth — independent
of the Descope backend.

The `loadtest` build tag swaps the remote Descope client for an in-process fake
(`internal/services/remote/loadtest_on.go`): `Check` returns `allowed:true, direct:true` after a
random 10-50ms delay, and `GetModified` polling always reports no changes. Results are **direct
only**, so they land in `directRelationCache` and are never wiped by the indirect-purge path —
the cache and RSS grow monotonically with unique keys.

The fake is behind a build tag, not an env flag, because an env flag that makes an authz cache
answer "allowed" for everything is an authz-bypass switch sitting in the production image. Under
`!loadtest` the fake code does not exist in the binary.

## Before you run: size the Docker VM

Everything here happens inside Docker Desktop's Linux VM, and both containers plus anything else
you have running share it. Check what you actually have:

```bash
docker info --format '{{.NCPU}} cpus, {{.MemTotal}} bytes'
```

A stock 4-CPU / 8 GB VM is not enough: authzcache pinned to 2 CPUs and k6 to 4 already oversubscribe
it, and 5M cache entries at the measured ~1.7 KB each will not fit. Raise Docker Desktop to at least 10 CPUs and
24 GB (Settings -> Resources), and **stop the `euw1` dev-env stack while measuring** — ~45 idling
service containers still burn enough CPU to move the knee.

## Run

```bash
./build.sh                                    # build authzcache:loadtest
docker compose up -d authzcache
./smoke.sh                                    # miss ~10-50ms, hit <2ms, allowed+direct
SCENARIO=smoke VERIFY_BODIES=true docker compose --profile k6 run --rm k6
SCENARIO=ramp docker compose --profile k6 run --rm k6
docker compose down
```

Results land in `out/` (`report.html` from k6's built-in web dashboard, `summary.json`).

## Scenarios

| `SCENARIO` | Executor | Purpose |
|---|---|---|
| `smoke` | constant-arrival-rate, 500 RPS, 30s | correctness: 50/50 split, allowed+direct |
| `fill` | constant-arrival-rate, `NEW_RATIO=1.0` | grow the cache to `FILL_ENTRIES` |
| `ramp` | ramping-arrival-rate, step-and-hold to 40k | find the max sustained RPS |

Run `fill` first to reach a cache size, then `ramp` against that warm cache. Ramping RPS and
cache size at the same time gives two confounded variables and an uninterpretable curve.

## Knobs

Container (compose env): `AC_CPUS` (2), `AC_MEM` (4g), `AC_DIRECT_CACHE_SIZE` (5,000,000),
`AC_GOGC` (100), `AC_GOMEMLIMIT` (3500MiB).

k6: `NEW_RATIO` (0.5, forced to 1 for `fill`), `TUPLES_PER_REQ` (1), `RESOURCE_CARDINALITY` /
`TARGET_CARDINALITY` (0 = unbounded), `FILL_ENTRIES`, `FILL_RPS`, `ID_OFFSET`, `SMOKE_RPS`,
`SMOKE_DURATION`, `VERIFY_BODIES`.

**`ID_OFFSET` matters across runs.** Keys are derived from the iteration counter, which restarts at
0 every k6 run. A second run against a warm cache would re-request keys the first run already
cached, and its `miss` tag would be a lie. After a `fill` of N entries, run `ramp` with
`ID_OFFSET=N`.

Cardinality shapes the memory cost per entry. `addDirectRelation` writes four structures per key:
the LRU plus `directKeyComponents`, `directResourcesIndex` and `directTargetsIndex`. The default
(both unbounded) is the worst case — a million single-entry nested maps. Set
`RESOURCE_CARDINALITY=1000` for the realistic "few resources, many users" fan-out. Capping both
is rejected: the cache key is `resource:target:relation`, so distinct ids would collapse onto the
same key and the cache would stop growing.

## Why these k6 parameters

**`ramping-arrival-rate`, not `ramping-vus`.** Closed-model executors self-throttle: when the
server slows, VUs send fewer requests, so you measure the system's own pace and never see the
cliff. The open model pushes a fixed rate regardless of response time. Saturation then shows up
as rising latency *and* as `dropped_iterations`, which is the unambiguous "you exceeded capacity"
signal. Watch that metric above all others.

**Step-and-hold stages.** A continuous ramp smears the knee across time and makes percentiles
unattributable. Flat 2-minute holds give a clean RPS -> p95 table with hundreds of thousands of
samples behind each p99.

**VU sizing.** Little's law: in-flight = RPS x mean latency. The 50/50 mix averages
`0.5 x ~0.5ms + 0.5 x 30ms ~= 15ms`, so 40k RPS needs ~600 in-flight. `preAllocatedVUs: 800`
(allocated up front so k6 pays no allocation cost mid-ramp), `maxVUs: 4000` for 3-5x tail
headroom. If k6 reports allocating VUs mid-run, numbers around that moment are suspect.

**Hit and miss are tagged separately.** This is the single most important part of the setup. The
miss path carries a synthetic 10-50ms floor, so a blended p95 is pinned near 50ms regardless of
how the edge behaves. `http_req_duration{path:hit}` is the edge's true serving cost;
`http_req_duration{path:miss}` minus ~30ms is the edge's overhead on the write path.

**`discardResponseBodies: true` for capacity runs.** JSON parsing in k6's JS VM is the top reason
the load generator becomes the bottleneck. `VERIFY_BODIES=true` turns it off for correctness runs.

**k6 runs in a container on the bridge.** On macOS a published port traverses Docker Desktop's
userland proxy, which caps around a few thousand RPS and adds jitter — you would be measuring the
proxy. Only pprof is published.

**Container pins are mandatory.** Unpinned CPU/memory makes results irreproducible. `LOG_LEVEL=error`
is not optional either: `controllers/controller.go` logs an Info line per request, and at 20k RPS
that alone dominates the profile. The compose file also sets `logging: driver: none` so the
Docker json-file writer is not a second ceiling.

**Sanity check the generator.** If `iteration_duration` climbs while `http_req_duration` stays
flat, k6 is saturated, not authzcache. Keep the k6 container under ~70% of its CPU allocation.

## Test matrix

Per data point: `fill` to N entries (record RSS via `docker stats` and
`curl localhost:6060/debug/pprof/heap`), then `ramp` and record the last step with
`dropped_iterations` ~ 0 and `http_req_failed` < 1%, plus `http_req_duration{path:hit}` p95/p99.

Repeat for N = 100k, 500k, 1M, 2M, 5M. Output: **max RPS vs cache entries** and **RSS vs entries**.

`ramp` keeps inserting while it runs (20k RPS adds 10k entries/s), so for the larger N values
also run a control with `NEW_RATIO=0.1` to separate "cache is big" from "cache is growing fast".

**Eviction control.** Re-run at N = 1M with `AC_DIRECT_CACHE_SIZE=1000000` (the product default) so
the LRU sits at its cap and every insert triggers `removeIndexOnCacheEviction` under the write
lock. Comparing against the uncapped 1M run isolates the eviction cost a real customer at cap pays.

## First measurements

Caveat: taken on a 4-CPU / 8 GB Docker VM with the `euw1` dev-env stack co-resident and k6 sharing
the same CPUs. Treat the RPS as a lower bound; the *profiles* are the useful part, since the
proportions hold regardless of how much CPU there is.

**Lock contention** (`/debug/pprof/mutex`, 4000 RPS, 50/50, ~150k entries) — the write lock
dominates, not the LRU read lock:

| Site | Share of lock delay |
|---|---|
| `UpdateCacheWithChecks` (`pc.mutex.Lock()`, miss path) | 47% |
| `middlewares.ProjectIDParser` | 10% |
| `CheckRelations` (RLock + LRU `Get`) | 8% |

**CPU** (`/debug/pprof/profile`) — GC is ~35-40% of CPU (`scanSpan` 21%, `tryDeferToSpanScan` 16%,
`scanObjectSmall` 11%, `mallocgc` 8%), against 18% in network syscalls. Because GC scan cost scales
with live heap, **throughput falls as the cache grows** — which is exactly the curve this harness
is built to plot. `AC_GOGC` is the lever for quantifying how much of the ceiling is GC.

**Memory** (`/debug/pprof/heap`, inuse_space, ~150k entries, default unbounded cardinality) —
~1.7 KB live per entry, roughly 3.4x the 500 B the high-level design assumes. The breakdown says
why: the two secondary indexes cost ~6x the cache itself.

| Allocation site | Share of live heap |
|---|---|
| `addKeyToDirectResourceIndex` | 35% |
| `addKeyToDirectTargetIndex` | 33% |
| the LRU itself (`insertValue`, `Add`, `key`) | 11% |

At the 1M-entry product default that extrapolates to ~1.7 GB live and ~3.4 GB RSS at `GOGC=100`.
Worth re-running with `RESOURCE_CARDINALITY=1000` — the indexes should collapse, since they stop
allocating a nested map per key.

## Reproducing the profiles

While holding the last good step:

```bash
go tool pprof -http=: http://localhost:6060/debug/pprof/profile?seconds=30   # CPU
go tool pprof -http=: http://localhost:6060/debug/pprof/mutex                # contention
go tool pprof -http=: http://localhost:6060/debug/pprof/heap                 # memory per entry
```

`SetMutexProfileFraction(5)` and `SetBlockProfileRate` are already on in the loadtest build.
