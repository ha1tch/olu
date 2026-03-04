# Timeseries v0.3 Progress

Design reference: `docs/TIMESERIES_DESIGN_V3.md`

## Status: Complete (v0.9.4)

Phases 1 and 2 are production-ready. Phases 3–5 remain deferred.

---

## Phase 1 — Core storage layer ✓

- [x] `types.go` — `Event`, `RangeQuery`, `LatestQuery`, `AggregateQuery`,
       `TimelineConfig`, `StoreConfig`, `Store` interface, `Manager` interface
- [x] `codec.go` — variable-length `[tid:2][d0..dN][ts:8]` key layout;
       flags+numerics+payload value encoding; `incrementKey`
- [x] `registry.go` — per-tenant JSON registry, atomic tmp+rename writes,
       dims immutability across sessions, per-timeline and store-level retention
- [x] `store.go` — `PebbleStore`: `DefineTimeline`, `UpdateTimeline`, `Append`,
       `AppendBatch`, `QueryRange`, `Latest`, `Aggregate`, `Purge`,
       `DefaultRetentionDays`, `SetDefaultRetentionDays`, `Stats`, `TimelineStats`,
       `RangeAggregate`, `RangeSum`, `RangeAvg`, `RangeMin`, `RangeMax`, `RangeCount`
- [x] `manager.go` — `DefaultManager`: lazy store open, tenant dir scan on start,
       idempotent `Provision`
- [x] `retention.go` — `RetentionWorker` goroutine with clean start/stop lifecycle

## Phase 2 — HTTP API layer ✓

- [x] `pkg/server/ts_handlers.go` — full handler set (see MANUAL.md for endpoint list)
- [x] `pkg/server/server.go` — route registration
- [x] `pkg/config/config.go` — 14 `OLU_TS_*` env vars, defaults, validation

## Phase 1 test suite ✓

- [x] `store_test.go` (15) — correctness, codec, purge, aggregate, scan-limit
- [x] `codec_property_test.go` (14) — key/value property tests, edge cases
- [x] `registry_persist_test.go` (6) — durability, dims immutability, atomicity
- [x] `concurrent_test.go` (5) — parallel access, `-race`
- [x] `ts_stress_test.go` (4) — bulk append, concurrent workers, mixed workload

## Phase 2 test suite ✓

- [x] `pkg/server/ts_e2e_test.go` (9) — full HTTP lifecycle, multi-tenant isolation
- [x] `pkg/server/ts_error_paths_test.go` (21) — all OLU-TS error codes
- [x] `pkg/server/ts_guardrail_test.go` (8) — all six backend limits
- [x] `pkg/config/config_test.go` (+3) — guardrail env vars, defaults, validation

## Bugs fixed post-implementation

1. `purgeTimeline` false `break` on first non-expired event → `continue`
2. `QueryRange` partial-prefix leaking out-of-range events → Go-side time filter
3. `Aggregate` partial-prefix — same fix
4. `DefaultRetentionDays`/`SetDefaultRetentionDays` missing from `Store` interface → added
5. Six backend query limits (config → handler) — all previously missing

## Known weaknesses (deferred to future items)

See plan in session notes. Numbered items 1–7:

1. `AppendBatch` hardcoded `max 5000` error message (config limit not forwarded to store)
2. `recordFirstWrite` acquires write lock on every Append after first write
3. `TotalEvents` counter can go negative after crash+purge (cosmetic; clamp to zero in `TimelineStats`)
4. `Latest` From/To: inverted bounds not validated (silent empty result instead of error)
5. Two doc gaps closed this session; `config_test.go` `TestDefault_TimeseriesGuardrails` missing `TSMaxAggregateBuckets`
6. Benchmark baseline: single-field sugar functions and `RangeAggregate` are within noise (777–834 ns/op on 2,500 events). No perf reason to maintain separate scan paths.

## Deferred phases

- **Phase 3** — IoT adapter (`pkg/iot/`)
- **Phase 4** — Migration tooling
- **Phase 5** — Extended benchmarks
