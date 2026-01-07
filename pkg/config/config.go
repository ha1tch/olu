package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds application configuration
type Config struct {
	// Server configuration
	Host string
	Port int

	// Storage configuration
	StorageType string // "jsonfile" or "sqlite"
	BaseDir     string
	SchemaDir   string
	Schema      string
	DBPath      string // SQLite database path

	// Cache configuration
	CacheType string // "memory" or "redis"
	CacheTTL  int    // seconds
	RedisHost string
	RedisPort int
	CacheSize int

	// Graph configuration
	GraphEnabled        bool
	GraphMode           string // "indexed" or "disabled"
	GraphDataFile       string
	GraphIndexFile      string
	GraphQueryTTL       int
	GraphResultTTL      int
	GraphCycleDetection string // "warn", "error", "ignore"

	// Full-text search
	FullTextEnabled bool

	// Query configuration
	MaxQueryDepth   int
	MaxEmbedDepth   int
	RefEmbedDepth   int
	DefaultPageSize int

	// Entity configuration
	PatchNullBehavior string // "store" or "delete"
	MaxEntitySize     int    // bytes

	// Cascade delete configuration
	CascadingDelete     bool
	MaxCascadeDeletions int
	MaxCascadeWork      int

	// Debug
	Debug      bool
	DebugLocks bool

	// Authentication
	AuthType         string   // "none", "jwt", "apikey"
	JWTSecret        string   // Secret for JWT validation
	JWTIssuer        string   // Expected issuer claim
	APIKeys          []string // Valid API keys (comma-separated in env)
	AuthExcludePaths []string // Paths excluded from auth (e.g., /health)

	// Rate limiting
	RateLimitEnabled bool
	RateLimitRate    int // Requests per window
	RateLimitWindow  int // Window in seconds
	RateLimitByIP    bool
	RateLimitByKey   bool // Rate limit by API key or JWT subject

	// Metrics
	MetricsEnabled bool

	// Multi-tenancy
	// TenantMode controls tenant isolation behaviour:
	//   "none"   - No tenant features (default)
	//   "path"   - Tenant routes available, non-tenant routes also work
	//   "strict" - All entity requests require tenant context; non-tenant routes return 403
	TenantMode string
}

// Default returns the default configuration
func Default() *Config {
	return &Config{
		Host:                "0.0.0.0",
		Port:                9090,
		StorageType:         "jsonfile",
		BaseDir:             "data",
		SchemaDir:           "schema",
		Schema:              "default",
		DBPath:              "olu.db",
		CacheType:           "memory",
		CacheTTL:            300,
		CacheSize:           1024,
		RedisHost:           "localhost",
		RedisPort:           6379,
		GraphEnabled:        true,
		GraphMode:           "indexed",
		GraphDataFile:       "graph.data",
		GraphIndexFile:      "graph.index",
		GraphQueryTTL:       86400,
		GraphResultTTL:      3600,
		GraphCycleDetection: "warn",
		FullTextEnabled:     false,
		MaxQueryDepth:       10,
		MaxEmbedDepth:       10,
		RefEmbedDepth:       3,
		DefaultPageSize:     10,
		PatchNullBehavior:   "store",
		MaxEntitySize:       1048576, // 1MB
		CascadingDelete:     false,
		MaxCascadeDeletions: 10000,
		MaxCascadeWork:      100000,
		Debug:               false,
		DebugLocks:          false,
		AuthType:            "none",
		JWTSecret:           "",
		JWTIssuer:           "",
		APIKeys:             []string{},
		AuthExcludePaths:    []string{"/health", "/version", "/metrics"},
		RateLimitEnabled:    false,
		RateLimitRate:       100,
		RateLimitWindow:     60,
		RateLimitByIP:       true,
		RateLimitByKey:      false,
		MetricsEnabled:      true,
		TenantMode:          "none",
	}
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv(cfg *Config) {
	if val := os.Getenv("HOST"); val != "" {
		cfg.Host = val
	}
	if val := os.Getenv("PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			cfg.Port = port
		}
	}
	if val := os.Getenv("STORAGE_TYPE"); val != "" {
		cfg.StorageType = val
	}
	if val := os.Getenv("DB_PATH"); val != "" {
		cfg.DBPath = val
	}
	if val := os.Getenv("BASE_DIR"); val != "" {
		cfg.BaseDir = val
	}
	if val := os.Getenv("SCHEMA_DIR"); val != "" {
		cfg.SchemaDir = val
	}
	if val := os.Getenv("SCHEMA_NAME"); val != "" {
		cfg.Schema = val
	}
	if val := os.Getenv("CACHE_TYPE"); val != "" {
		cfg.CacheType = val
	}
	if val := os.Getenv("CACHE_TTL"); val != "" {
		if ttl, err := strconv.Atoi(val); err == nil {
			cfg.CacheTTL = ttl
		}
	}
	if val := os.Getenv("REDIS_HOST"); val != "" {
		cfg.RedisHost = val
	}
	if val := os.Getenv("REDIS_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			cfg.RedisPort = port
		}
	}
	if val := os.Getenv("RSERV_GRAPH"); val != "" {
		cfg.GraphMode = val
		cfg.GraphEnabled = val != "disabled"
	}
	if val := os.Getenv("GRAPH_CYCLE_DETECTION"); val != "" {
		cfg.GraphCycleDetection = val
	}
	if val := os.Getenv("FULLTEXT_ENABLED"); val != "" {
		cfg.FullTextEnabled = parseBool(val)
	}
	if val := os.Getenv("CASCADING_DELETE"); val != "" {
		cfg.CascadingDelete = parseBool(val)
	}
	if val := os.Getenv("DEBUG"); val != "" {
		cfg.Debug = parseBool(val)
	}
	if val := os.Getenv("DEBUG_LOCKS"); val != "" {
		cfg.DebugLocks = parseBool(val)
	}
	if val := os.Getenv("REF_EMBED_DEPTH"); val != "" {
		if depth, err := strconv.Atoi(val); err == nil {
			cfg.RefEmbedDepth = depth
		}
	}
	if val := os.Getenv("MAX_ENTITY_SIZE"); val != "" {
		if size, err := strconv.Atoi(val); err == nil {
			cfg.MaxEntitySize = size
		}
	}
	if val := os.Getenv("PATCH_NULL"); val != "" {
		cfg.PatchNullBehavior = val
	}

	// Authentication settings
	if val := os.Getenv("AUTH_TYPE"); val != "" {
		cfg.AuthType = val
	}
	if val := os.Getenv("JWT_SECRET"); val != "" {
		cfg.JWTSecret = val
	}
	if val := os.Getenv("JWT_ISSUER"); val != "" {
		cfg.JWTIssuer = val
	}
	if val := os.Getenv("API_KEYS"); val != "" {
		cfg.APIKeys = strings.Split(val, ",")
		for i := range cfg.APIKeys {
			cfg.APIKeys[i] = strings.TrimSpace(cfg.APIKeys[i])
		}
	}

	// Rate limiting settings
	if val := os.Getenv("RATE_LIMIT_ENABLED"); val != "" {
		cfg.RateLimitEnabled = parseBool(val)
	}
	if val := os.Getenv("RATE_LIMIT_RATE"); val != "" {
		if rate, err := strconv.Atoi(val); err == nil {
			cfg.RateLimitRate = rate
		}
	}
	if val := os.Getenv("RATE_LIMIT_WINDOW"); val != "" {
		if window, err := strconv.Atoi(val); err == nil {
			cfg.RateLimitWindow = window
		}
	}
	if val := os.Getenv("RATE_LIMIT_BY_IP"); val != "" {
		cfg.RateLimitByIP = parseBool(val)
	}
	if val := os.Getenv("RATE_LIMIT_BY_KEY"); val != "" {
		cfg.RateLimitByKey = parseBool(val)
	}
	if val := os.Getenv("METRICS_ENABLED"); val != "" {
		cfg.MetricsEnabled = parseBool(val)
	}

	// Tenant mode
	if val := os.Getenv("TENANT_MODE"); val != "" {
		cfg.TenantMode = val
	}
}

func parseBool(val string) bool {
	val = strings.ToLower(val)
	return val == "true" || val == "1" || val == "yes"
}
