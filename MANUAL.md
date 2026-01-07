# Olu Manual

Complete reference documentation for Olu v0.9.0.

## Table of Contents

1. [Installation](#installation)
2. [Configuration](#configuration)
3. [API Reference](#api-reference)
4. [Query Languages](#query-languages)
5. [Authentication](#authentication)
6. [Rate Limiting](#rate-limiting)
7. [Metrics & Monitoring](#metrics--monitoring)
8. [Storage Backends](#storage-backends)
9. [Graph Features](#graph-features)
10. [Testing & Benchmarks](#testing--benchmarks)
11. [Deployment](#deployment)

---

## Installation

### From Source

```bash
git clone https://github.com/ha1tch/olu.git
cd olu
make build
./bin/olu
```

### Using Go Install

```bash
go install github.com/ha1tch/olu/cmd/olu@latest
```

### Docker

```bash
docker pull ghcr.io/ha1tch/olu:latest
docker run -p 9090:9090 -v $(pwd)/data:/data ghcr.io/ha1tch/olu:latest
```

### Build Options

```bash
make build          # Build binary
make build-all      # Build for all platforms (18 OS/arch)
make docker-build   # Build Docker image
make install        # Install to $GOPATH/bin
```

---

## Configuration

All configuration is via environment variables.

### Server

| Variable | Default | Description |
|----------|---------|-------------|
| `HOST` | `0.0.0.0` | Server bind address |
| `PORT` | `9090` | Server port |

### Storage

| Variable | Default | Description |
|----------|---------|-------------|
| `STORAGE_TYPE` | `jsonfile` | Backend: `jsonfile` or `sqlite` |
| `BASE_DIR` | `data` | Base directory for JSONFile storage |
| `DB_PATH` | `olu.db` | SQLite database path |
| `SCHEMA_NAME` | `default` | Schema/namespace name |

### Cache

| Variable | Default | Description |
|----------|---------|-------------|
| `CACHE_TYPE` | `memory` | Cache type: `memory` or `redis` |
| `CACHE_TTL` | `300` | Cache TTL in seconds |
| `REDIS_HOST` | `localhost` | Redis host (if using redis cache) |
| `REDIS_PORT` | `6379` | Redis port |

### Graph

| Variable | Default | Description |
|----------|---------|-------------|
| `RSERV_GRAPH` | `indexed` | Graph mode: `indexed` or `disabled` |
| `GRAPH_CYCLE_DETECTION` | `warn` | Cycle handling: `warn`, `error`, `ignore` |

### Features

| Variable | Default | Description |
|----------|---------|-------------|
| `FULLTEXT_ENABLED` | `false` | Enable FTS5 full-text search (SQLite only) |
| `CASCADING_DELETE` | `false` | Delete referencing entities on delete |
| `REF_EMBED_DEPTH` | `3` | Default reference embedding depth |
| `MAX_EMBED_DEPTH` | `10` | Maximum allowed embed depth |
| `MAX_ENTITY_SIZE` | `1048576` | Maximum entity size in bytes |
| `PATCH_NULL` | `store` | Null handling in PATCH: `store` or `delete` |

### Authentication

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_TYPE` | `none` | Auth type: `none`, `jwt`, `apikey` |
| `JWT_SECRET` | | Secret key for JWT validation |
| `JWT_ISSUER` | | Expected JWT issuer claim |
| `API_KEYS` | | Comma-separated list of valid API keys |

### Rate Limiting

| Variable | Default | Description |
|----------|---------|-------------|
| `RATE_LIMIT_ENABLED` | `false` | Enable rate limiting |
| `RATE_LIMIT_RATE` | `100` | Requests per window |
| `RATE_LIMIT_WINDOW` | `60` | Window duration in seconds |
| `RATE_LIMIT_BY_IP` | `true` | Rate limit by client IP |
| `RATE_LIMIT_BY_KEY` | `false` | Rate limit by auth key/subject |

### Observability

| Variable | Default | Description |
|----------|---------|-------------|
| `METRICS_ENABLED` | `true` | Enable Prometheus metrics |
| `DEBUG` | `false` | Enable debug logging |

---

## API Reference

### Entity Operations

#### Create Entity
```http
POST /api/v1/{entity}
Content-Type: application/json

{"name": "Alice", "email": "alice@example.com"}
```

Response: `201 Created`
```json
{"id": 1, "name": "Alice", "email": "alice@example.com"}
```

#### Get Entity
```http
GET /api/v1/{entity}/{id}
```

Query parameters:
- `embed=false` - Disable reference embedding
- `embed_depth=N` - Override default embed depth

#### List Entities
```http
GET /api/v1/{entity}?page=1&per_page=20
```

Response includes pagination:
```json
{
  "data": [...],
  "pagination": {
    "page": 1,
    "per_page": 20,
    "total_items": 100,
    "total_pages": 5
  }
}
```

#### Update Entity (Full)
```http
PUT /api/v1/{entity}/{id}
Content-Type: application/json

{"name": "Alice Smith", "email": "alice.smith@example.com"}
```

#### Update Entity (Partial)
```http
PATCH /api/v1/{entity}/{id}
Content-Type: application/json

{"email": "newemail@example.com"}
```

#### Delete Entity
```http
DELETE /api/v1/{entity}/{id}
```

#### Save Entity (Upsert with ID)
```http
POST /api/v1/{entity}/{id}/save
Content-Type: application/json

{"name": "Bob", "email": "bob@example.com"}
```

### Multi-Tenant Operations

All entity operations support tenant isolation via URL prefix:

```http
GET /api/v1/tenant/{tenant_id}/{entity}
POST /api/v1/tenant/{tenant_id}/{entity}
GET /api/v1/tenant/{tenant_id}/{entity}/{id}
```

### Graph Operations

#### Shortest Path
```http
GET /api/v1/graph/shortestPath?from={entity}:{id}&to={entity}:{id}
```

#### Path Exists
```http
GET /api/v1/graph/pathExists?from={entity}:{id}&to={entity}:{id}
```

#### Common Neighbors
```http
GET /api/v1/graph/commonNeighbors?node1={entity}:{id}&node2={entity}:{id}
```

#### Node Information
```http
GET /api/v1/graph/node/{entity}:{id}
GET /api/v1/graph/node/{entity}:{id}/degree
GET /api/v1/graph/node/{entity}:{id}/neighbors?direction=out
```

### Search Operations

#### Full-Text Search (SQLite only)
```http
GET /api/v1/search?q={query}&entity={entity}
```

#### Field Search
```http
GET /api/v1/{entity}/search?field={field}&query={value}&match={type}
```

Match types: `exact`, `contains`, `prefix`, `suffix`

### Export Operations

```http
GET /api/v1/export
```

Returns a ZIP archive containing:
- `manifest.json` - Export metadata
- `entities.db` or `data/` - Entity data
- `graph.json` - Graph structure
- `graph.data`, `graph.index` - Binary graph files

### Schema Operations

```http
GET /api/v1/schema
GET /api/v1/schema/{entity}
POST /api/v1/schema/{entity}
```

### System Operations

```http
GET /health        # Health check
GET /version       # Version info
GET /metrics       # Prometheus metrics
```

---

## Query Languages

### OQL (Olu Query Language)

SQL-like query language for entities.

```http
POST /api/v1/oql/query
Content-Type: application/json

{"query": "SELECT * FROM users WHERE age > 25 ORDER BY name LIMIT 10"}
```

#### Supported Features

- `SELECT` with field selection and `*`
- `WHERE` with operators: `=`, `!=`, `>`, `<`, `>=`, `<=`, `LIKE`, `IN`
- `ORDER BY` with `ASC`/`DESC`
- `LIMIT` and `OFFSET`
- `GROUP BY` with aggregates: `COUNT`, `SUM`, `AVG`, `MIN`, `MAX`

#### Examples

```sql
-- Basic query
SELECT name, email FROM users WHERE status = 'active'

-- Aggregation
SELECT department, COUNT(*) as count, AVG(salary) as avg_salary
FROM employees
GROUP BY department

-- Pattern matching
SELECT * FROM products WHERE name LIKE '%widget%'

-- Sorting and pagination
SELECT * FROM orders ORDER BY created_at DESC LIMIT 20 OFFSET 40
```

#### Async Queries

For long-running queries:

```http
POST /api/v1/oql/async
Content-Type: application/json

{"query": "SELECT * FROM large_table"}
```

Returns job ID:
```json
{"job_id": "abc123", "status": "pending"}
```

Check status:
```http
GET /api/v1/oql/job/{job_id}
```

### Sulpher (Graph Query Language)

Path-based query language for graph traversal.

```http
POST /api/v1/sulpher/query
Content-Type: application/json

{"query": "users:1 -[*1..3]-> posts"}
```

#### Syntax

```
source -[edge_spec]-> target
```

Edge specifications:
- `*` - Any edge type
- `*1..3` - 1 to 3 hops
- `manages` - Specific edge type

#### Examples

```
-- Direct connections
users:1 -> posts

-- Multi-hop paths
users:1 -[*1..5]-> users

-- Bidirectional
users:1 <-> users:2
```

---

## Authentication

### JWT Authentication

```bash
export AUTH_TYPE=jwt
export JWT_SECRET=your-secret-key-min-32-chars
export JWT_ISSUER=your-app  # Optional
```

Request with JWT:
```http
GET /api/v1/users
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

JWT requirements:
- Algorithm: HS256
- Claims: `sub` (subject), `exp` (expiration)
- Optional: `iss` (issuer), `nbf` (not before)

### API Key Authentication

```bash
export AUTH_TYPE=apikey
export API_KEYS=key1,key2,key3
```

Request with API key:
```http
GET /api/v1/users
X-API-Key: key1
```

Or:
```http
GET /api/v1/users
Authorization: ApiKey key1
```

### Excluded Paths

By default, these paths don't require authentication:
- `/health`
- `/version`
- `/metrics`

---

## Rate Limiting

Enable rate limiting to protect your API:

```bash
export RATE_LIMIT_ENABLED=true
export RATE_LIMIT_RATE=100      # requests
export RATE_LIMIT_WINDOW=60     # seconds
export RATE_LIMIT_BY_IP=true
```

### Response Headers

All responses include rate limit headers:

```http
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1704067200
```

### Rate Limited Response

When limit exceeded:
```http
HTTP/1.1 429 Too Many Requests
Retry-After: 45

{"error": "Too Many Requests", "message": "Rate limit exceeded", "retry_after": 45}
```

---

## Metrics & Monitoring

### Prometheus Endpoint

```http
GET /metrics
```

### Available Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `olu_uptime_seconds` | gauge | Server uptime |
| `olu_requests_total` | counter | Total HTTP requests |
| `olu_requests_by_status_total` | counter | Requests by status code |
| `olu_request_errors_total` | counter | Total 4xx/5xx responses |
| `olu_active_requests` | gauge | Current in-flight requests |
| `olu_request_duration_seconds_bucket` | histogram | Request latency distribution |
| `olu_entity_operations_total` | counter | CRUD operations by type |
| `olu_cache_total` | counter | Cache hits/misses |
| `olu_queries_total` | counter | Query operations by type |

### JSON Format

```http
GET /metrics
Accept: application/json
```

### Prometheus Configuration

```yaml
scrape_configs:
  - job_name: 'olu'
    static_configs:
      - targets: ['localhost:9090']
```

---

## Storage Backends

### JSONFile Storage

Human-readable storage using JSON files.

```bash
export STORAGE_TYPE=jsonfile
export BASE_DIR=data
export SCHEMA_NAME=myapp
```

Directory structure:
```
data/
└── myapp/
    ├── users/
    │   ├── 1.json
    │   ├── 2.json
    │   └── ...
    ├── posts/
    │   └── ...
    └── _schemas/
        └── users.json
```

**Advantages:**
- Human-readable
- Easy debugging
- Git-friendly

**Limitations:**
- Slower at scale
- No ACID guarantees
- No full-text search

### SQLite Storage

Production-ready storage with ACID guarantees.

```bash
export STORAGE_TYPE=sqlite
export DB_PATH=olu.db
export FULLTEXT_ENABLED=true
```

**Advantages:**
- ACID transactions
- Better performance
- Full-text search support
- Single-file database

**Migration:**

```bash
./bin/olu-migrate --from jsonfile --to sqlite \
  --source-dir ./data/myapp \
  --target-db ./olu.db
```

---

## Graph Features

### Reference Format

Create relationships using REF objects:

```json
{
  "name": "Alice",
  "manager": {
    "type": "REF",
    "entity": "users",
    "id": 42
  },
  "department": {
    "type": "REF",
    "entity": "departments",
    "id": 5
  }
}
```

### Automatic Graph Sync

References are automatically:
- Added to graph on entity creation
- Updated when entity is modified
- Removed when entity is deleted

### Cycle Detection

Configure cycle handling:

```bash
export GRAPH_CYCLE_DETECTION=warn   # Log warning, allow
export GRAPH_CYCLE_DETECTION=error  # Reject edge creation
export GRAPH_CYCLE_DETECTION=ignore # Allow silently
```

### Reference Embedding

Fetch entities with references resolved:

```http
GET /api/v1/users/1
```

Returns:
```json
{
  "id": 1,
  "name": "Alice",
  "manager": {
    "id": 42,
    "name": "Bob",
    "manager": {
      "id": 10,
      "name": "Carol"
    }
  }
}
```

Control embedding:
```http
GET /api/v1/users/1?embed=false
GET /api/v1/users/1?embed_depth=1
```

### Cascading Deletes

When enabled, deleting an entity also deletes entities that reference it:

```bash
export CASCADING_DELETE=true
```

---

## Testing & Benchmarks

### Running Tests

```bash
make test           # Quick tests
make test-v         # Verbose output
make test-race      # With race detector
make test-full      # Full suite + stress tests
make coverage       # With coverage report
```

### Package Tests

```bash
make test-storage   # Storage tests
make test-sqlite    # SQLite-specific
make test-server    # HTTP server tests
make test-graph     # Graph operations
make test-oql       # OQL parser/executor
make test-sulpher   # Sulpher queries
```

### Benchmarks

```bash
make bench          # All benchmarks
make bench-storage  # Storage benchmarks
make bench-server   # HTTP benchmarks
```

### Stress Tests

```bash
make stress         # 10k record stress test
make stress-race    # With race detector
```

---

## Deployment

### Docker Compose

```yaml
version: '3.8'
services:
  olu:
    image: ghcr.io/ha1tch/olu:latest
    ports:
      - "9090:9090"
    environment:
      - STORAGE_TYPE=sqlite
      - DB_PATH=/data/olu.db
      - AUTH_TYPE=apikey
      - API_KEYS=${API_KEYS}
      - RATE_LIMIT_ENABLED=true
    volumes:
      - olu-data:/data
    restart: unless-stopped

volumes:
  olu-data:
```

### Reverse Proxy (nginx)

```nginx
upstream olu {
    server 127.0.0.1:9090;
}

server {
    listen 443 ssl;
    server_name api.example.com;

    ssl_certificate /etc/ssl/certs/api.crt;
    ssl_certificate_key /etc/ssl/private/api.key;

    location / {
        proxy_pass http://olu;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

### Health Checks

```bash
# Simple health check
curl -f http://localhost:9090/health

# Kubernetes liveness probe
livenessProbe:
  httpGet:
    path: /health
    port: 9090
  initialDelaySeconds: 5
  periodSeconds: 10
```

### Backup

```bash
# SQLite
sqlite3 olu.db ".backup backup.db"

# Or use export endpoint
curl http://localhost:9090/api/v1/export > backup.zip
```

---

## Implementation Notes

### Cache Backends

Olu supports two cache backends:

#### Memory Cache (Default)

Simple in-process LRU cache. Good for development and single-instance deployments.

```bash
export CACHE_TYPE=memory
export CACHE_TTL=300
```

**Characteristics:**
- Fast (no network overhead)
- Not shared between instances
- Lost on restart
- Uses global TTL (per-item TTL not supported)

#### Redis Cache

Production-grade distributed cache. Use when running multiple instances or when you need per-item TTL control.

```bash
export CACHE_TYPE=redis
export CACHE_TTL=300
export REDIS_HOST=localhost
export REDIS_PORT=6379
```

**Characteristics:**
- Shared across all olu instances
- Survives restarts (if Redis persistence enabled)
- Supports per-item TTL
- Network latency on every operation
- Requires Redis infrastructure

#### When to Use Redis

- Running multiple olu instances behind a load balancer
- Need cache to survive restarts
- Need per-item TTL control
- Want to inspect cache contents via redis-cli

For single-instance deployments or development, the memory cache is simpler and faster.

### OQL Entity Discovery

OQL validates that entity types exist before executing queries. Entity discovery happens automatically:

1. On startup, OQL scans the schema directory for entity folders
2. When a query references an unknown entity, OQL automatically rescans the directory
3. If the entity still doesn't exist, the query fails with "entity does not exist"

This means **newly created entity types are recognised automatically** without server restart or manual refresh. The first query against a new entity type may incur a small overhead for the rescan.

### Multi-Tenancy

Olu supports path-based tenant isolation via `/api/v1/tenant/{tenant_id}/...` routes.

#### Tenant Modes

| Mode | Behaviour |
|------|-----------|
| `none` | Default. No tenant features enabled. |
| `path` | Tenant routes available. Non-tenant routes also work. |
| `strict` | All entity requests require tenant context. Non-tenant entity routes return 403. |

Configure via environment:
```bash
export TENANT_MODE=strict
```

#### Security Model

**Important:** This is **application-level isolation**, not security isolation.

In `path` mode:
- Tenant data is tagged with `tenant_id` field
- Queries filter by tenant automatically on tenant routes
- Non-tenant routes can still access all data

In `strict` mode:
- Non-tenant entity routes are blocked (403 Forbidden)
- System routes (`/health`, `/metrics`, etc.) remain accessible
- Graph, OQL, schema, and export routes remain accessible

**For true security isolation between hostile tenants:**
- Use separate Olu instances per tenant
- Use separate databases per tenant
- Implement authentication that maps users to tenants

#### Example Usage

```bash
# Create entity in tenant "acme"
curl -X POST http://localhost:9090/api/v1/tenant/acme/users \
  -H "Content-Type: application/json" \
  -d '{"name": "Alice"}'

# List only acme's users
curl http://localhost:9090/api/v1/tenant/acme/users

# In strict mode, this returns 403:
curl http://localhost:9090/api/v1/users
# {"error": "Tenant context required. Use /api/v1/tenant/{tenant_id}/... routes"}
```

---

## Troubleshooting

### Common Issues

**"Database is locked"**
- SQLite concurrent write issue
- Solution: Ensure single writer or use WAL mode

**"Entity not found" after creation**
- Cache may be stale
- Solution: Check cache TTL or disable caching for debugging

**Graph queries return empty**
- Graph may not be initialized
- Check: `RSERV_GRAPH=indexed`

**Rate limiting too aggressive**
- Adjust `RATE_LIMIT_RATE` and `RATE_LIMIT_WINDOW`
- Consider `RATE_LIMIT_BY_KEY=true` for authenticated clients

### Debug Mode

```bash
export DEBUG=true
```

Enables verbose logging including:
- Request/response details
- Query execution
- Graph operations

---

## License

Apache 2.0 - See [LICENSE](LICENSE)
