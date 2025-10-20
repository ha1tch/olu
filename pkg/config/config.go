package config

import (
	"os"
	"strconv"
	"strings"
)

const Version = "0.7.0"

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
	CacheType      string // "memory" or "redis"
	CacheTTL       int    // seconds
	RedisHost      string
	RedisPort      int
	CacheSize      int

	// Graph configuration
	GraphEnabled       bool
	GraphMode          string // "indexed" or "disabled"
	GraphDataFile      string
	GraphIndexFile     string
	GraphQueryTTL      int
	GraphResultTTL     int
	GraphCycleDetection string // "warn", "error", "ignore"

	// Full-text search
	FullTextEnabled bool

	// Query configuration
	MaxQueryDepth     int
	MaxEmbedDepth     int
	RefEmbedDepth     int
	DefaultPageSize   int

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
}

func parseBool(val string) bool {
	val = strings.ToLower(val)
	return val == "true" || val == "1" || val == "yes"
}
