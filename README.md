# Olu

**A graph-enhanced REST API prototyping server**

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8.svg)](https://golang.org/)

> **v0.9.0** - API stable, 200+ tests passing. See [MANUAL.md](MANUAL.md) for complete documentation.

## What is Olu?

Olu is a REST API server that automatically maintains a **graph representation** of your entity relationships. Define your data schema, create entities with references, and get graph traversal capabilities for free.

**Perfect for:** API prototyping, content management, social networks, organizational structures, knowledge bases.

> Olu is a Go port of [rserv](https://github.com/ha1tch/rserv) (Python). Both share the same API design.

## Quick Start

```bash
# Build and run
git clone https://github.com/ha1tch/olu.git && cd olu
make build && ./bin/olu

# Or with Docker
docker run -p 9090:9090 ghcr.io/ha1tch/olu:latest
```

Create an entity:
```bash
curl -X POST http://localhost:9090/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{"name": "Alice", "email": "alice@example.com"}'
```

Create with reference:
```bash
curl -X POST http://localhost:9090/api/v1/posts \
  -H "Content-Type: application/json" \
  -d '{"title": "Hello", "author": {"type": "REF", "entity": "users", "id": 1}}'
```

Query the graph:
```bash
curl "http://localhost:9090/api/v1/graph/node/users:1/neighbors"
```

## Key Features

| Feature | Description |
|---------|-------------|
| **Automatic Graph** | REF objects create graph edges automatically |
| **Dual Storage** | JSONFile (dev) or SQLite (prod) |
| **Query Languages** | OQL (SQL-like) and Sulpher (graph paths) |
| **Full-text Search** | SQLite FTS5 integration |
| **Authentication** | JWT and API key support |
| **Rate Limiting** | Per-IP and per-key limiting |
| **Metrics** | Prometheus `/metrics` endpoint |
| **Multi-tenant** | Path-based tenant isolation |

## API Overview

| Endpoint | Description |
|----------|-------------|
| `POST /api/v1/{entity}` | Create entity |
| `GET /api/v1/{entity}/{id}` | Get entity (with embedded refs) |
| `GET /api/v1/{entity}` | List entities (paginated) |
| `PUT /api/v1/{entity}/{id}` | Update entity |
| `PATCH /api/v1/{entity}/{id}` | Partial update |
| `DELETE /api/v1/{entity}/{id}` | Delete entity |
| `GET /api/v1/graph/shortestPath` | Find shortest path |
| `POST /api/v1/oql/query` | Run OQL query |
| `POST /api/v1/sulpher/query` | Run graph query |
| `GET /api/v1/search?q=term` | Full-text search |
| `GET /metrics` | Prometheus metrics |

## Configuration

Key environment variables:

```bash
# Storage
STORAGE_TYPE=sqlite          # jsonfile or sqlite
DB_PATH=olu.db              # SQLite path
FULLTEXT_ENABLED=true       # Enable FTS5

# Auth
AUTH_TYPE=jwt               # none, jwt, or apikey
JWT_SECRET=your-secret      # For JWT
API_KEYS=key1,key2          # For API keys

# Rate Limiting
RATE_LIMIT_ENABLED=true
RATE_LIMIT_RATE=100         # Requests per window
RATE_LIMIT_WINDOW=60        # Window in seconds

# Graph
GRAPH_CYCLE_DETECTION=warn  # warn, error, or ignore
REF_EMBED_DEPTH=3           # Auto-resolve ref depth
```

See [MANUAL.md](MANUAL.md) for all options.

## Query Examples

**OQL (SQL-like):**
```bash
curl -X POST http://localhost:9090/api/v1/oql/query \
  -d '{"query": "SELECT * FROM users WHERE age > 25 ORDER BY name LIMIT 10"}'
```

**Sulpher (Graph paths):**
```bash
curl -X POST http://localhost:9090/api/v1/sulpher/query \
  -d '{"query": "users:1 -[*1..3]-> posts"}'
```

## Testing

```bash
make test        # Quick tests
make test-full   # Full suite with stress tests
make bench       # Benchmarks
```

## Project Structure

```
olu/
├── cmd/olu/          # Server entry point
├── pkg/
│   ├── server/       # HTTP handlers
│   ├── storage/      # JSONFile & SQLite backends
│   ├── graph/        # Graph operations
│   ├── oql/          # OQL query engine
│   ├── sulpher/      # Sulpher query engine
│   ├── middleware/   # Auth, rate limiting, metrics
│   └── cache/        # Memory & Redis cache
└── docs/             # Additional documentation
```

## Roadmap

**Completed:** Dual storage, graph tracking, OQL/Sulpher, FTS5, auth, rate limiting, metrics, 200+ tests, CI/CD

**Planned:** Batch operations, documentation site, production deployment guide

## License

Apache 2.0 - See [LICENSE](LICENSE)

---

**[Full Documentation →](MANUAL.md)**
