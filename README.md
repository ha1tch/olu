# Olu

**A graph-enhanced REST API prototyping server**

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Version](https://img.shields.io/badge/version-0.7.0-orange.svg)](https://github.com/ha1tch/olu)
[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8.svg)](https://golang.org/)

> **Pre-Release Software**: Olu is currently in active development (v0.7.0). Not recommended for production use yet.

## What is Olu?

Olu is a REST API prototyping server that automatically maintains a **graph representation** of your entity relationships. Define your data schema, create entities with references, and get graph traversal capabilities for free.

> **Note:** Olu is a Go implementation of the [rserv](https://github.com/ha1tch/rserv) project, which is API-complete and written in Python. If you prefer Python, or you wish to understand more of the API better check out rserv!

**Perfect for:**
- Rapid API prototyping with relationship tracking
- Content management systems with complex hierarchies
- Social networks (users, posts, comments, follows)
- Organizational structures (employees, departments, managers)
- Knowledge bases and wikis with interconnected articles

## Key Features

### Automatic Graph Relationships
Define references between entities and Olu automatically builds and maintains a graph:

```json
{
  "name": "Alice",
  "email": "alice@example.com",
  "manager": {
    "type": "REF",
    "entity": "users",
    "id": 42
  }
}
```

Then query the graph:
- Find paths between entities
- Get all neighbors (incoming/outgoing)
- Detect cycles
- Traverse relationships

### Dual Storage Backends

**JSONFile (Development)**
```bash
export STORAGE_TYPE=jsonfile
./olu
```
- Human-readable JSON files
- Easy debugging and inspection
- Git-friendly test data
- Perfect for development

**SQLite (Production)**
```bash
export STORAGE_TYPE=sqlite
export DB_PATH=olu.db
./olu
```
- ACID transactions
- Efficient queries
- Automatic graph synchronization
- Production-ready

**Switch between them with zero code changes.**

### Performance Features
- **Caching**: In-memory LRU or Redis
- **Pagination**: Built-in page/per_page support
- **Reference Embedding**: Fetch nested relationships in one request
- **Concurrent Operations**: Thread-safe with proper locking

### Schema Validation
Define JSON schemas for your entities:

```json
{
  "type": "object",
  "required": ["name", "email"],
  "properties": {
    "name": {"type": "string", "minLength": 1},
    "email": {"type": "string"},
    "age": {"type": "number", "minimum": 0}
  }
}
```

## Quick Start

### Installation

```bash
# Clone the repository
git clone https://github.com/ha1tch/olu.git
cd olu

# Install dependencies
make deps

# Build
make build

# Run
./olu
```

The server starts on `http://localhost:9090` by default.

### Your First API

**1. Create an entity:**
```bash
curl -X POST http://localhost:9090/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Alice Smith",
    "email": "alice@example.com",
    "age": 30
  }'

# Response: {"message": "Resource of entity users created successfully", "id": 1}
```

**2. Get the entity:**
```bash
curl http://localhost:9090/api/v1/users/1
```

**3. Create a related entity:**
```bash
curl -X POST http://localhost:9090/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Bob Johnson", 
    "email": "bob@example.com",
    "manager": {
      "type": "REF",
      "entity": "users",
      "id": 1
    }
  }'
```

**4. Query the graph:**
```bash
# Find path between users
curl -X POST http://localhost:9090/api/v1/graph/path \
  -H "Content-Type: application/json" \
  -d '{"from": "users:2", "to": "users:1", "max_depth": 10}'

# Get neighbors
curl -X POST http://localhost:9090/api/v1/graph/neighbors \
  -H "Content-Type: application/json" \
  -d '{"node_id": "users:1", "direction": "both"}'
```

## Development Workflow

### Using JSONFile Storage (Recommended for Development)

```bash
# Start server with JSONFile backend
export STORAGE_TYPE=jsonfile
./olu
```

Your data is stored in human-readable files:
```bash
data/
  default/
    users/
      1.json          # {"id": 1, "name": "Alice", ...}
      2.json          # {"id": 2, "name": "Bob", ...}
      _next_id.json   # {"next_id": 3}
  graph.data          # users:2:users:1:manager
```

**Inspect your data:**
```bash
# Read an entity
cat data/default/users/1.json | jq

# Find all references to user 1
grep "users:1" data/graph.data

# Check ID sequence
cat data/default/users/_next_id.json
```

**Edit data manually:**
```bash
# Fix bad data during development
vim data/default/users/1.json
```

### Migrating to SQLite for Production

```bash
# Build migration tool
make build-migrate

# Migrate JSONFile data to SQLite
./olu-migrate ./data/default ./production.db

# Run with SQLite
export STORAGE_TYPE=sqlite
export DB_PATH=./production.db
./olu
```

## API Reference

### Entity Operations

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/{entity}` | Create entity |
| `GET` | `/api/v1/{entity}` | List entities (paginated) |
| `GET` | `/api/v1/{entity}/{id}` | Get entity by ID |
| `PUT` | `/api/v1/{entity}/{id}` | Update entity (replace) |
| `PATCH` | `/api/v1/{entity}/{id}` | Patch entity (partial update) |
| `DELETE` | `/api/v1/{entity}/{id}` | Delete entity |
| `POST` | `/api/v1/{entity}/save/{id}` | Save entity with specific ID |

### Graph Operations

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/graph/path` | Find path between nodes |
| `POST` | `/api/v1/graph/neighbors` | Get node neighbors |
| `GET` | `/api/v1/graph/stats` | Get graph statistics |

### Schema Operations

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/schema/{entity}` | Create/update schema |
| `GET` | `/api/v1/schema/{entity}` | Get schema |

### System Operations

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/version` | Get version |

## Configuration

Configure via environment variables:

### Server
```bash
HOST=0.0.0.0              # Server host (default: 0.0.0.0)
PORT=9090                 # Server port (default: 9090)
```

### Storage
```bash
STORAGE_TYPE=jsonfile     # Storage backend: jsonfile|sqlite
DB_PATH=olu.db           # SQLite database path
BASE_DIR=data            # Base directory for JSONFile storage
SCHEMA_NAME=default      # Schema name
```

### Cache
```bash
CACHE_TYPE=memory        # Cache type: memory|redis
CACHE_TTL=300           # Cache TTL in seconds
REDIS_HOST=localhost    # Redis host
REDIS_PORT=6379         # Redis port
```

### Graph
```bash
RSERV_GRAPH=indexed      # Graph mode: indexed|disabled
GRAPH_CYCLE_DETECTION=warn  # Cycle detection: warn|error|ignore
```

### Features
```bash
FULLTEXT_ENABLED=false   # Enable full-text search
CASCADING_DELETE=false   # Enable cascading deletes
REF_EMBED_DEPTH=3       # Default reference embedding depth
MAX_ENTITY_SIZE=1048576 # Max entity size in bytes (1MB)
PATCH_NULL=store        # Null behavior in PATCH: store|delete
```

## Advanced Features

### Reference Embedding

Fetch entities with their references resolved:

```bash
curl "http://localhost:9090/api/v1/users/2?embed_depth=1"
```

Response:
```json
{
  "id": 2,
  "name": "Bob Johnson",
  "manager": {
    "id": 1,
    "name": "Alice Smith",
    "email": "alice@example.com"
  }
}
```

### Pagination

```bash
curl "http://localhost:9090/api/v1/users?page=1&per_page=20"
```

Response:
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

### Cascading Deletes

Enable cascading deletes to automatically remove dependent entities:

```bash
export CASCADING_DELETE=true
./olu
```

When you delete an entity, all entities referencing it will also be deleted.

### Partial Updates (PATCH)

Update only specific fields:

```bash
curl -X PATCH http://localhost:9090/api/v1/users/1 \
  -H "Content-Type: application/json" \
  -d '{"age": 31}'
```

Control null behavior:
- `PATCH_NULL=store`: `{"email": null}` sets email to null
- `PATCH_NULL=delete`: `{"email": null}` removes the email field

## Testing

```bash
# Run all tests
make test

# Run with coverage
make coverage

# Run benchmarks
make benchmark

# Run specific test suite
make test-unit        # Storage layer tests
make test-integration # Server tests
make test-sqlite      # SQLite-specific tests

# Run with race detector
make test-race
```

## Benchmarks

Run benchmarks to measure performance:

```bash
# Quick benchmarks
make benchmark

# Extended benchmarks (5s each)
make benchmark-long

# Specific benchmark
make benchmark-Create
make benchmark-GraphPath
```

## Docker Support

```bash
# Build Docker image
make docker-build

# Run container
make docker-run

# Or use docker-compose
docker-compose up
```

## Project Structure

```
olu/
├── cmd/
│   ├── olu/              # Main application
│   └── olu-migrate/      # Migration tool
├── pkg/
│   ├── cache/            # Cache implementations (memory, Redis)
│   ├── config/           # Configuration management
│   ├── graph/            # Graph data structure and operations
│   ├── models/           # Data models and types
│   ├── server/           # HTTP server and handlers
│   ├── storage/          # Storage backends (JSONFile, SQLite)
│   └── validation/       # JSON schema validation
├── schema/               # Example schemas
├── data/                 # Data directory (JSONFile storage)
├── Makefile             # Build and test commands
└── docker-compose.yml   # Docker configuration
```

## Examples

See the `test_api.sh` script for comprehensive API examples:

```bash
./test_api.sh
```

This script demonstrates:
- Creating entities
- Creating entities with references
- Updating and patching
- Graph queries (paths, neighbors)
- Pagination
- Reference embedding
- Deletion operations

## Architecture Highlights

### Storage Abstraction
Both backends implement the same `Store` interface:
```go
type Store interface {
    Create(ctx context.Context, entity string, data map[string]interface{}) (int, error)
    Get(ctx context.Context, entity string, id int) (map[string]interface{}, error)
    Update(ctx context.Context, entity string, id int, data map[string]interface{}) error
    // ... more methods
}
```

### Automatic Graph Synchronization
References are automatically extracted and added to the graph:
- **JSONFile**: Graph maintained in separate `graph.data` file
- **SQLite**: Graph stored in `graph_edges` table with transactional consistency

### Pluggable Components
- **Storage**: JSONFile, SQLite (PostgreSQL, MongoDB possible)
- **Cache**: Memory, Redis
- **Validation**: JSON Schema (extensible)

## Roadmap to 1.0

- [ ] Complete JSON Schema validation (patterns, nested objects)
- [ ] Batch operations (BatchCreate, BatchDelete)
- [ ] Full-text search with SQLite FTS5
- [ ] Authentication middleware
- [ ] Query language for complex filters
- [ ] Metrics and observability (Prometheus)
- [ ] Comprehensive documentation site
- [ ] Performance optimizations
- [ ] Production deployment guide

## Known Limitations

- **Single-node only**: No clustering or distributed support
- **Graph in-memory**: Graph must fit in RAM
- **No authentication**: Add your own auth middleware
- **Limited JSON Schema**: Doesn't support all JSON Schema features
- **Sequential IDs**: IDs are sequential integers (reveals entity count)

## FAQ

**Q: What's the relationship to rserv?**  
A: Olu is a Go port of [rserv](https://github.com/ha1tch/rserv), which is the original API-complete Python implementation. Both share the same API design and concepts.

**Q: Why both JSONFile and SQLite?**  
A: JSONFile is perfect for development - you can `cat` your data files and see exactly what's stored. SQLite provides ACID guarantees and better performance for production.

**Q: Can I use this in production?**  
A: Not yet. This is v0.7.0 pre-release. The API is stable but tests are not at this time.

**Q: How does graph performance scale?**  
A: The in-memory graph is fast but limited by RAM. For very large graphs (millions of nodes), consider specialized graph databases.

**Q: Can I add custom authentication?**  
A: The server uses `chi` router, so you can wrap handlers with middleware. It's on our roadmap to provide that as a ready-made solution in the future.

**Q: What about database migrations?**  
A: Olu is schema-free by design. Entities are JSON blobs, so schema evolution is natural.


## License

Apache License 2.0 - see [LICENSE](LICENSE) file for details.

## Author

**ha1tch** - [h@ual.fi](mailto:h@ual.fi)

Repository: [https://github.com/ha1tch/olu](https://github.com/ha1tch/olu)
