# Changelog

All notable changes to olu are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).


## [0.9.4] - 2026-03-03

### Added

- **RangeAggregate — single-pass all-fields aggregate** — New `RangeAggregate`
  method on `Store` computing count, sum, avg, min, max for all seven numeric
  fields simultaneously in one Pebble iterator pass. New types:
  - `RangeAllQuery` — query shape for `RangeAggregate`; no `NumField` (covers
    all fields; distinct from `RangeNumQuery` to prevent silent ignored-field bugs)
  - `RangeAggregateResult` — `Count uint64`, `Sums/Avgs/Mins/Maxs [7]float64`,
    `Fields [7]bool` (which fields were present in at least one event)

- **RangeSum, RangeAvg, RangeMin, RangeMax, RangeCount** — Single-field
  convenience functions retained as syntax sugar. All now delegate to
  `RangeAggregate` internally; `rangeNumIter` removed. Benchmark data confirms
  the delegation costs nothing: all six functions are within measurement noise
  of each other on a 2,500-event dataset (776–834 ns/op, ~300 KB/op, ~10K
  allocs/op). The Pebble scan dominates; accumulating 7×4 float64s vs 1×2
  is immeasurable.

- **Range aggregate benchmarks** — `BenchmarkRangeSum`, `BenchmarkRangeAvg`,
  `BenchmarkRangeMin`, `BenchmarkRangeMax`, `BenchmarkRangeCount`,
  `BenchmarkRangeAggregate` in `range_agg_test.go`. Seed volume: 2,500 events,
  all seven fields populated. Baseline reading (Xeon 8581C, 2.10 GHz):
  single-field 777–834 ns/op, RangeAggregate 829 ns/op.

### Changed

- **Sugar functions delegate to RangeAggregate** — `RangeSum`, `RangeAvg`,
  `RangeMin`, `RangeMax`, `RangeCount` are now one-scan functions via
  `RangeAggregate`. `NumField` validation (0–6) is still performed before
  delegation. `RangeCount` returns the count of events carrying the specific
  field (`Fields[i]` true), not total event count.

### Fixed

- **`pkg/timeseries/store.go` line 712** — `fmt.Errorf` format string had one
  `%d` verb but two arguments (`len(q.Dims)`, `cfg.Dims`); corrected to
  `"ts: query dims %d out of range 1–%d (OLU-TS007)"`. Build failure in the
  `timeseries` package on all prior releases.

## [0.9.3] - 2026-03-03

### Added

- **Timeseries v0.3 implementation complete** — Full rewrite of the
  timeseries subsystem on a generic multi-dimensional key layout. Phase 1
  (storage layer) and Phase 2 (HTTP API layer) are production-ready.

  Storage layer (`pkg/timeseries/`):
  - `codec.go` — variable-length `[tid:2][d0..dN][ts:8]` big-endian key
    layout; compact flags+numerics+payload value encoding; `incrementKey`
    for exclusive upper-bound generation
  - `registry.go` — per-tenant JSON registry with atomic tmp+rename writes;
    dims immutability enforced across sessions; per-timeline and store-level
    retention both persisted
  - `store.go` — `PebbleStore` implementing the `Store` interface:
    `DefineTimeline`, `UpdateTimeline`, `Append`, `AppendBatch`,
    `QueryRange`, `Latest`, `Aggregate`, `Purge`, `Stats`, `TimelineStats`,
    `DefaultRetentionDays`, `SetDefaultRetentionDays`
  - `manager.go` — `DefaultManager` with lazy store open, tenant dir
    scanning on startup, idempotent `Provision`
  - `retention.go` — `RetentionWorker` goroutine with configurable interval
    and clean stop/wait lifecycle

  HTTP API layer (`pkg/server/ts_handlers.go`):
  - Timeline management: `POST /ts/provision`, `POST /ts/timelines`,
    `GET /ts/timelines`, `GET /ts/timelines/{id}`, `PATCH /ts/timelines/{id}`
  - Write: `POST /ts/events` (single), `POST /ts/events/batch`
  - Read: `GET /ts/events` (range query), `GET /ts/events/latest`
  - Aggregation: `POST /ts/aggregate` (scalar and time-bucketed)
  - Management: `GET /ts/retention`, `PATCH /ts/retention`, `GET /ts/stats`,
    `GET /ts/timelines/{id}/stats`
  - All routes tenant-scoped under `/api/v1/tenant/{id}/ts/`

- **Timeseries backend query limits** — Six new config fields enforced at
  the handler layer on every read and aggregate operation:
  - `OLU_TS_QUERY_TIMEOUT` (default 30s) — context deadline per read
  - `OLU_TS_MAX_QUERY_EVENTS` (default 10000) — caps returned events
  - `OLU_TS_MAX_SCAN_EVENTS` (default 500000) — aborts scan mid-flight
  - `OLU_TS_MAX_RANGE_DAYS` (default 366) — caps From→To window
  - `OLU_TS_MAX_BATCH_SIZE` (default 5000) — caps batch append size
  - `OLU_TS_MAX_RESPONSE_BYTES` (default 10 MB) — caps JSON response size

- **Timeseries test suite** — 78 new tests across 7 files:
  - `pkg/timeseries/store_test.go` (15) — store correctness, codec round-trips,
    purge, aggregation, scan-limit
  - `pkg/timeseries/codec_property_test.go` (14) — key/value property tests
    across full dims×value matrix and edge cases
  - `pkg/timeseries/registry_persist_test.go` (6) — close/reopen durability,
    dims immutability across sessions, atomic tmp-file write
  - `pkg/timeseries/concurrent_test.go` (5) — parallel appends, append+query,
    idempotent define, purge+append; all under `-race`
  - `pkg/timeseries/ts_stress_test.go` (4) — 5k bulk, 10-worker concurrent,
    100 bulk queries, mixed append+purge; skipped in `-short`
  - `pkg/server/ts_e2e_test.go` (9) — full HTTP lifecycle, multi-tenant
    isolation, partial-prefix queries, batch atomicity, ordering, aggregate
  - `pkg/server/ts_error_paths_test.go` (21) — every OLU-TS error branch
  - `pkg/server/ts_guardrail_test.go` (8) — all six backend limits enforced
  - `pkg/config/config_test.go` (+3) — guardrail env vars, defaults,
    conditional validation for `TimeseriesEnabled`

### Fixed

- **Timeseries purge false break** — `purgeTimeline` was stopping on the
  first non-expired event encountered, silently skipping all events after a
  gap. Fixed to `continue`, scanning the full key space.
- **QueryRange and Aggregate partial-prefix time leakage** — Partial-prefix
  scans (fewer dims than the timeline's declared count) leaked events outside
  the `From`/`To` window because the Pebble key bounds spanned multiple
  series that are not time-ordered relative to each other. Added a Go-side
  time filter for all partial-prefix queries.

### Changed

- **Timeseries batch max configurable** — `AppendBatch` limit raised from a
  hardcoded 5000 to the server-configured `TSMaxBatchSize` (default 5000).
  Tests updated accordingly.

## [0.9.2-rc1] - 2026-03-02

### Changed

- **Timeseries design v0.3** — Complete redesign of the timeseries
  subsystem from domain-specific (asset_id, sensor_id, trigger_type
  hardcoded in keys) to a generic multi-timeline store. Key changes:
  variable-length `[timeline_id:2][dims][ts:8]` key layout with uint64
  dimensions (1–5 per timeline); per-timeline retention replacing the
  single store-level policy; secondary index removed; domain-specific
  value fields replaced with generic numeric fields (up to 7 float64)
  plus a caller-defined opaque payload. Documented in
  `docs/TIMESERIES_DESIGN_V3.md`.
- **Timeseries doc consolidation** — `docs/TIMESERIES_DESIGN.md`,
  `docs/TIMESERIES_DESIGN_V2.md`, and `docs/TIMESERIES_IMPL_PLAN.md`
  retired. `docs/TIMESERIES_DESIGN_V3.md` is the single authoritative
  reference.
- **Storage estimate corrected** — Effective bytes/event updated from
  ~52 bytes (v0.1 estimate based on old value encoding) to ~30 bytes
  (v0.3 generic encoding with Zstd compression) in MANUAL.md.

## [0.9.2] - 2026-03-02

### Added

- **Hardware-aware query complexity gating** — EXPLAIN-based cost
  estimation for adapted full push-down. Complex queries (non-covering
  aggregates, temp B-tree sorts) are gated against hardware-specific
  thresholds. Three preset profiles (VPS, dedicated, bare-metal) and a
  runtime `CalibrateProfile()` function. New files:
  `complexity_estimator.go`, `complexity_profiles.go`,
  `complexity_planner.go`. 8 test functions / 26 subtests including
  result-correctness tests that verify both paths produce identical
  output.
- **Complexity benchmarks** — `BenchmarkComplexity_Generate`,
  `BenchmarkComplexity_Execute`, `BenchmarkComplexity_Full`,
  `BenchmarkComplexity_GoPath`, `BenchmarkComplexity_SQLPlan` in
  `complexity_bench_test.go`. Validated on Apple M1 (calibrated profile:
  blob=127, nonCovering=142, tempBTree1=161, tempBTree2=219) and
  container amd64 (VPS preset correctly gates regressions).
- **JOIN push-down exploration** — Design analysis documented in
  `QUERY_OPTIMISATION_PROGRESS.md`: relationship metadata requirements,
  backend-compatibility tension (three options evaluated), entity
  combination matrix (adapted-adapted, adapted-blob, blob-blob), and
  graph-layer boundary definition. No implementation planned for v0.9.x.

### Changed

- **Query planner doc v2.0** — `QUERY_PLANNER.md` updated: removed stale
  section 13 entries (GROUP BY/DISTINCT push-down, startup calibration)
  that are now implemented; added JOIN push-down entry.
- **Progress tracker** — Phase A4 added to `QUERY_OPTIMISATION_PROGRESS.md`
  with dependency graph, test count (1744), and full JOIN exploration
  section.

### Fixed

- **Tautological aggregate tests** — `aggEnv.run()` was passing the raw
  `env.store` to `ExecuteWithStore`, bypassing the `nonAggStore` wrapper.
  Both "Go path" and "push-down path" were effectively running push-down,
  making the comparison meaningless. The `WithWhere` and `WithWhereEquality`
  tests now correctly exercise the Go-side filtering pipeline.

### Changed

- **Golden database test infrastructure** — Converted 112 independent test
  functions (each copying the golden DB and creating a fresh env) into 4
  table-driven parent tests with shared environments. Read-only query tests
  now share a single store per group.
- **Fast blob entity seeding** — Rewrote `seedEquivalence` to use raw SQL
  in a single transaction with a prepared statement instead of 2000+
  individual `store.Create` calls. Seeding time: 3.7s to 0.4s.
- **SQLite PRAGMA tuning** — `synchronous=OFF` and `journal_mode=MEMORY`
  during golden database seeding (ephemeral file, durability irrelevant).
- **Total test suite speedup** — Previously-slow test groups reduced from
  7-11s each (frequently timing out at 15s) to 2.5-2.8s each.


## [0.9.0-rc20] - 2026-03-01

### Added

- **Schema evolution:** Automatic migration when JSON Schemas change
  after initial adapted table registration. `DiffAdaptedSpecs` computes
  a migration plan (added/dropped columns, index changes, type
  conflicts). `MigrateAdaptedTable` executes the plan in a single
  transaction: ALTER TABLE ADD/DROP COLUMN, index updates, metadata
  sync. Dropped column data is preserved in the `_extra` overflow
  column. Incompatible type changes and loss-of-additionalProperties
  are rejected with clear errors. 12 new tests (7 diff unit, 5
  migration integration).

### Changed

- **RegisterAdaptedTable:** Now calls `MigrateAdaptedTable` on schema
  hash mismatch instead of returning a hard error. Compatible changes
  (add/drop columns) proceed automatically; incompatible changes
  (type changes) still error.


- **B3 (columnar executor) deferred:** Cost-benefit analysis concluded
  that B4's inline predicate filtering already delivers the primary
  allocation win. Columnar rewrite would touch ~30% of the codebase
  for ~15-20% throughput gain on the Go fallback path only. Deferred
  with full design notes in QUERY_OPTIMISATION_PROGRESS.md.


## [0.9.0-rc19] - 2026-03-01

### Added

- **Prepared statement cache (A3):** LRU cache (`StmtCache`) for
  `*sql.Stmt` objects with generation-based eviction. Wired into
  `SQLiteStore` for `CountEntities`, `QueryWithPlan`,
  `AggregateQuery`, `ListWithFields`, and `QueryWithFields`.
  Default capacity 256 entries. 10 new tests.

- **Predicate push-down during tokenisation (B4):** Inline predicate
  evaluation in the jsonic token walk. New types: `FieldPredicate`,
  `PredicateSet`, `PredicateOp` (Eq/Neq/Lt/Lte/Gt/Gte/In/Like).
  `FilterExtractFromTokens` performs single-pass field extraction +
  predicate evaluation — rows that fail predicates never allocate a
  `map[string]interface{}`.

- **Predicate compiler:** `CompilePredicates()` decomposes OQL WHERE
  AST into a `jsonic.PredicateSet` (pushable AND-combined simple
  comparisons) plus a residual expression for unpushable terms (OR,
  NOT, IS NULL, BETWEEN, functions, subqueries).

- **FilterableStore interface:** Extension of `FieldQueryable` with
  `ListWithFieldsAndFilter` for inline predicate filtering during
  blob reads. SQLite implementation wired.

- **Executor B4 integration:** Go-path fallback now checks for
  `FilterableStore`, compiles WHERE predicates, and passes them into
  the tokenisation loop. Residual WHERE terms still evaluated in Go.

- **B4 test coverage:** 19 jsonic predicate/filter tests, 12
  predicate compiler tests.

### Changed

- **A-track complete:** All adapted-table optimisation phases (A1, A2,
  A3) are done.

- **B-track nearly complete:** B1, B2, B4 done. Only B3 (columnar
  executor) remains.


## [0.9.0-rc18] - 2026-03-01

### Added

- **PushFull planner decision:** New `PushFull` variant in
  `PushDecision` enum. The planner now detects adapted entities and
  returns `PushFull` or `PushAggregate` directly, skipping the
  `CountEntities` database round-trip entirely for adapted tables.

- **FieldQueryable interface:** Optional `FieldQueryable` interface
  (`ListWithFields`, `QueryWithFields`) for selective field
  extraction from blob entities. SQLite implementation uses jsonic
  tokenisation with atom-based key matching — extracts only requested
  fields without deserialising full JSON objects.

- **Executor FieldQueryable integration:** When the OQL query's
  SELECT list names specific fields (not `SELECT *`), the executor
  routes through `FieldQueryable` for both the Go-path fallback
  (`ListWithFields`) and the WHERE push-down path
  (`QueryWithFields`).

- **B2 test coverage:** 13 storage-level tests (type preservation,
  nested objects, arrays, null handling, long field names, missing
  fields, empty-fields fallback, comparative oracle vs `List` and
  `QueryWithPlan`). 7 executor E2E tests (basic select, WHERE,
  ORDER BY, TOP, push-down vs Go-path comparison, SELECT * bypass).

### Changed

- **Executor dispatch refactored:** Replaced cascading
  `fullPushed`/`aggregatePushed` boolean flags with a clean
  `switch` on `plan.pushed()` checks and a single `fetched` flag.
  Three strategies with graceful fallback: PushFull, PushAggregate,
  blob push-down, Go-path.

- **Planner adapted-entity fast path:** `Plan()` checks
  `AggregateQueryable.IsAdaptedEntity()` before calling
  `CountEntities`. For adapted entities, push-down is always
  beneficial regardless of row count.

### Fixed

- **ListWithFields empty-fields fallback:** Passing `nil` or empty
  field list to `ListWithFields` now correctly delegates to `List`
  (full deserialisation) instead of returning rows with zero fields.


## [0.9.0-rc11] - 2026-02-22

### Added

- **Adapted tables:** Schema-adapted table layouts that generate
  optimised SQLite tables from JSON Schema definitions instead of
  storing entities as JSON blobs. Benchmarks show 2.6x–124x speedup
  for common query patterns.

- **StorageDialect interface:** Backend-agnostic abstraction for
  adapted table operations. SQLite implementation provided;
  PostgreSQL can be added without changing the core layer.

- **queryfy v0.3.0 integration:** Replaced the internal
  `JSONSchemaValidator` with queryfy-based validation. Schema
  introspection via `SchemaBrowser` provides field metadata (type,
  format, precision, scale) used by the adapted table layer.

- **Decimal type support:** Fixed-point decimal fields with exact
  arithmetic, no floating-point approximation at any stage.

  - Schema declaration: `type: "string"`, `format: "decimal"` with
    `decimalPrecision` and `decimalScale` metadata.
  - Wire format: JSON strings (not numbers) to preserve exactness.
  - Validation: parse, precision bounds, scale bounds — rejects
    rather than silently truncates.
  - Storage (SQLite): scaled integer — value × 10^scale stored as
    `INTEGER`. Correct ordering, range queries, and indexing across
    the full signed range. Maximum 18 digits of precision (int64).
  - Storage (PostgreSQL, future): native `NUMERIC(p,s)`.
  - OQL aggregation: `SUM`, `AVG`, `MIN`, `MAX` use
    `shopspring/decimal` for exact Go-side arithmetic on SQLite.
    PostgreSQL will use native SQL aggregation.
  - Documentation: design doc (`docs/DECIMAL_TYPE_DESIGN.md`) and
    user guide (`docs/DECIMAL_TYPES.md`).

- **Adapted CRUD operations:** `adaptedCreate`, `adaptedUpdate`,
  `adaptedGet`, `adaptedList`, `adaptedGetInTx` with column-level
  partitioning (`PartitionData`), reassembly (`ReassembleData`), and
  decimal normalisation/denormalisation on write and read paths.

- **Adapted table registry:** `AdaptedRegistry` tracks which entities
  have adapted table specs, used by OQL executor for decimal-aware
  aggregation dispatch.

- **OQL decimal aggregation:** `Aggregator` detects decimal fields
  via `AdaptedRegistry` and dispatches to `DecimalAggregates` map
  (`shopspring/decimal`-based `SUM`, `AVG`, `MIN`, `MAX`) instead of
  float64 aggregation.

### Changed

- **Validation pipeline:** Validation now delegates to queryfy's
  compiled `ObjectSchema` with transform pipeline. Decimal fields are
  wrapped in a transform closure that captures precision and scale
  from field metadata.

### Dependencies

- Added `shopspring/decimal` v1.4.0 for exact decimal arithmetic.
- Updated `queryfy` to v0.3.0 for schema introspection and
  validation.


## Release candidate history

| RC | Date | Description |
|---|---|---|
| rc1 | 2026-02 | Initial 0.9.0 candidate. No detailed record survives. |
| rc2 | 2026-02 | No detailed record survives. |
| rc3 | 2026-02 | Query guardrail tests (scan limit, row limit, response size, timeout). Backup/restore drill test. Sulpher context cancellation, `MaxVisitedNodes`/`MaxResults` replacing hardcoded BFS limits. Slow query logging (>5 s) for OQL and Sulpher. Bug fixes: `ExecuteWithStore` not copying `QueryLimits`; export handler not checkpointing WAL. |
| rc4 | 2026-02-22 | Sentinel errors replace string matching for query limit violations (`oql.ErrScanLimit`, `oql.ErrResultLimit`, `sulpher.ErrVisitedNodeLimit`, `sulpher.ErrResultLimit`). Dedicated graph error codes OLU-GR005, OLU-GR006. Sulpher guardrail tests. 698 tests passing. Baseline for adapted tables work. |
| rc5 | 2026-02-22 | Adapted tables Phase 1: metadata layer (`AdaptedTableSpec`, `ColumnDef`, `ColumnType`), `StorageDialect` interface, SQLite implementation, adapted CRUD operations, registry. Schema introspection via `SchemaBrowser` (queryfy v0.3.0). |
| rc6 | 2026-02-22 | queryfy validation delegation: `JSONSchemaValidator` replaced with queryfy transform pipeline. 36 validation tests. |
| rc7 | 2026-02-22 | Decimal type design document (`docs/DECIMAL_TYPE_DESIGN.md`). |
| rc8 | 2026-02-22 | Decimal storage layer: `NormaliseDecimal`/`DenormaliseDecimal` on `StorageDialect`, adapted CRUD wiring, `shopspring/decimal` v1.4.0 dependency. 269 storage tests. |
| rc9 | 2026-02-22 | Decimal OQL aggregation: `DecimalAggregates` (SUM/AVG/MIN/MAX with exact arithmetic), executor integration via `AdaptedRegistry`. User documentation (`docs/DECIMAL_TYPES.md`). 285 OQL tests. Signed decimal design (N/P text prefix, unsigned implementation). |
| rc10 | 2026-02-22 | Simplified signed decimal text storage (N/P prefix, straight digits, accepted reversed intra-negative sort). |
| rc11 | 2026-02-22 | Switched to scaled integer storage. Decimal values stored as int64 (value x 10^scale) in INTEGER columns. Correct ordering for full signed range. Both docs updated. Changelog added. |
| rc12 | 2026-02-23 | `StorageDialect` interface expanded: `SupportsNativeDecimalAggregation()`, `ColumnType` signature formalised with precision/scale parameters. |
| rc13 | 2026-02-23 | Full adapted CRUD wiring. All Store operations branch on `AdaptedRegistry.IsAdapted()`. Auto-registration on schema load and startup sync. REST list handler routes adapted entities away from blob push-down. Read/write split: separate writer (1 conn) and reader (NumCPU conns, `query_only=ON`) pools. 16 CRUD tests, comparative benchmarks, E2E HTTP test. |
| rc14-15 | 2026-02-23 | Aggregate push-down for adapted tables. `AggregateQueryable` interface, `GenerateAggregateSQL` for native GROUP BY + aggregates against adapted columns. Three-tier executor dispatch. Decimal denormalisation from scaled integers. `hasScalarFunctions` guard. |
| rc16 | 2026-02-23 | Query optimisation roadmap phases 0 (abstraction cleanup), B1 (jsonic tokeniser), A1 (full SELECT push-down for adapted tables). 44 comparative push-down tests. Decimal MIN/MAX bugfix. |
| rc17 | 2026-02-23 | Checkpoint. 1645 tests, 16 packages, all passing. Roadmap document updated. |
| rc18 | 2026-03-01 | Phase A2: planner integration. `PushFull` decision type; planner owns adapted-entity routing (skips `CountEntities`); executor dispatch refactored from cascading booleans to strategy switch. Phase B2: `FieldQueryable` interface wired into executor (`ListWithFields`, `QueryWithFields`); jsonic selective extraction on blob reads; empty-fields fallback bug fixed; 20 new tests (13 storage, 7 executor E2E). |
| rc19 | 2026-03-01 | Phase A3: prepared statement cache (LRU, generation-based eviction, 10 tests). Phase B4: predicate push-down during tokenisation — inline filtering in jsonic token walk, predicate compiler (AST → PredicateSet + residual), FilterableStore interface, executor wiring. 31 new tests. 1706 total. |
| rc20 | 2026-03-01 | Schema evolution: automatic adapted table migration on schema change. DiffAdaptedSpecs, MigrateAdaptedTable (transactional ADD/DROP COLUMN, _extra data preservation, index sync). Type change rejection. 12 new tests. 1718 total. |
