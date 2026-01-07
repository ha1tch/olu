package config

import (
	"os"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.Host != "0.0.0.0" {
		t.Errorf("Expected Host '0.0.0.0', got '%s'", cfg.Host)
	}
	if cfg.Port != 9090 {
		t.Errorf("Expected Port 9090, got %d", cfg.Port)
	}
	if cfg.StorageType != "jsonfile" {
		t.Errorf("Expected StorageType 'jsonfile', got '%s'", cfg.StorageType)
	}
	if cfg.CacheType != "memory" {
		t.Errorf("Expected CacheType 'memory', got '%s'", cfg.CacheType)
	}
	if cfg.CacheTTL != 300 {
		t.Errorf("Expected CacheTTL 300, got %d", cfg.CacheTTL)
	}
	if !cfg.GraphEnabled {
		t.Error("Expected GraphEnabled true")
	}
	if cfg.GraphMode != "indexed" {
		t.Errorf("Expected GraphMode 'indexed', got '%s'", cfg.GraphMode)
	}
	if cfg.GraphCycleDetection != "warn" {
		t.Errorf("Expected GraphCycleDetection 'warn', got '%s'", cfg.GraphCycleDetection)
	}
	if cfg.AuthType != "none" {
		t.Errorf("Expected AuthType 'none', got '%s'", cfg.AuthType)
	}
	if cfg.RateLimitEnabled {
		t.Error("Expected RateLimitEnabled false")
	}
	if !cfg.MetricsEnabled {
		t.Error("Expected MetricsEnabled true")
	}
}

func TestLoadFromEnv_Server(t *testing.T) {
	cfg := Default()

	os.Setenv("HOST", "127.0.0.1")
	os.Setenv("PORT", "8080")
	defer os.Unsetenv("HOST")
	defer os.Unsetenv("PORT")

	LoadFromEnv(cfg)

	if cfg.Host != "127.0.0.1" {
		t.Errorf("Expected Host '127.0.0.1', got '%s'", cfg.Host)
	}
	if cfg.Port != 8080 {
		t.Errorf("Expected Port 8080, got %d", cfg.Port)
	}
}

func TestLoadFromEnv_Storage(t *testing.T) {
	cfg := Default()

	os.Setenv("STORAGE_TYPE", "sqlite")
	os.Setenv("DB_PATH", "/tmp/test.db")
	os.Setenv("BASE_DIR", "/data")
	os.Setenv("SCHEMA_NAME", "myschema")
	defer os.Unsetenv("STORAGE_TYPE")
	defer os.Unsetenv("DB_PATH")
	defer os.Unsetenv("BASE_DIR")
	defer os.Unsetenv("SCHEMA_NAME")

	LoadFromEnv(cfg)

	if cfg.StorageType != "sqlite" {
		t.Errorf("Expected StorageType 'sqlite', got '%s'", cfg.StorageType)
	}
	if cfg.DBPath != "/tmp/test.db" {
		t.Errorf("Expected DBPath '/tmp/test.db', got '%s'", cfg.DBPath)
	}
	if cfg.BaseDir != "/data" {
		t.Errorf("Expected BaseDir '/data', got '%s'", cfg.BaseDir)
	}
	if cfg.Schema != "myschema" {
		t.Errorf("Expected Schema 'myschema', got '%s'", cfg.Schema)
	}
}

func TestLoadFromEnv_Cache(t *testing.T) {
	cfg := Default()

	os.Setenv("CACHE_TYPE", "redis")
	os.Setenv("CACHE_TTL", "600")
	os.Setenv("REDIS_HOST", "redis.local")
	os.Setenv("REDIS_PORT", "6380")
	defer os.Unsetenv("CACHE_TYPE")
	defer os.Unsetenv("CACHE_TTL")
	defer os.Unsetenv("REDIS_HOST")
	defer os.Unsetenv("REDIS_PORT")

	LoadFromEnv(cfg)

	if cfg.CacheType != "redis" {
		t.Errorf("Expected CacheType 'redis', got '%s'", cfg.CacheType)
	}
	if cfg.CacheTTL != 600 {
		t.Errorf("Expected CacheTTL 600, got %d", cfg.CacheTTL)
	}
	if cfg.RedisHost != "redis.local" {
		t.Errorf("Expected RedisHost 'redis.local', got '%s'", cfg.RedisHost)
	}
	if cfg.RedisPort != 6380 {
		t.Errorf("Expected RedisPort 6380, got %d", cfg.RedisPort)
	}
}

func TestLoadFromEnv_Graph(t *testing.T) {
	cfg := Default()

	os.Setenv("RSERV_GRAPH", "disabled")
	os.Setenv("GRAPH_CYCLE_DETECTION", "error")
	defer os.Unsetenv("RSERV_GRAPH")
	defer os.Unsetenv("GRAPH_CYCLE_DETECTION")

	LoadFromEnv(cfg)

	if cfg.GraphMode != "disabled" {
		t.Errorf("Expected GraphMode 'disabled', got '%s'", cfg.GraphMode)
	}
	if cfg.GraphEnabled {
		t.Error("Expected GraphEnabled false when mode is disabled")
	}
	if cfg.GraphCycleDetection != "error" {
		t.Errorf("Expected GraphCycleDetection 'error', got '%s'", cfg.GraphCycleDetection)
	}
}

func TestLoadFromEnv_Features(t *testing.T) {
	cfg := Default()

	os.Setenv("FULLTEXT_ENABLED", "true")
	os.Setenv("CASCADING_DELETE", "yes")
	os.Setenv("REF_EMBED_DEPTH", "5")
	os.Setenv("MAX_ENTITY_SIZE", "2097152")
	os.Setenv("PATCH_NULL", "delete")
	defer os.Unsetenv("FULLTEXT_ENABLED")
	defer os.Unsetenv("CASCADING_DELETE")
	defer os.Unsetenv("REF_EMBED_DEPTH")
	defer os.Unsetenv("MAX_ENTITY_SIZE")
	defer os.Unsetenv("PATCH_NULL")

	LoadFromEnv(cfg)

	if !cfg.FullTextEnabled {
		t.Error("Expected FullTextEnabled true")
	}
	if !cfg.CascadingDelete {
		t.Error("Expected CascadingDelete true")
	}
	if cfg.RefEmbedDepth != 5 {
		t.Errorf("Expected RefEmbedDepth 5, got %d", cfg.RefEmbedDepth)
	}
	if cfg.MaxEntitySize != 2097152 {
		t.Errorf("Expected MaxEntitySize 2097152, got %d", cfg.MaxEntitySize)
	}
	if cfg.PatchNullBehavior != "delete" {
		t.Errorf("Expected PatchNullBehavior 'delete', got '%s'", cfg.PatchNullBehavior)
	}
}

func TestLoadFromEnv_Auth(t *testing.T) {
	cfg := Default()

	os.Setenv("AUTH_TYPE", "jwt")
	os.Setenv("JWT_SECRET", "my-secret-key")
	os.Setenv("JWT_ISSUER", "my-app")
	defer os.Unsetenv("AUTH_TYPE")
	defer os.Unsetenv("JWT_SECRET")
	defer os.Unsetenv("JWT_ISSUER")

	LoadFromEnv(cfg)

	if cfg.AuthType != "jwt" {
		t.Errorf("Expected AuthType 'jwt', got '%s'", cfg.AuthType)
	}
	if cfg.JWTSecret != "my-secret-key" {
		t.Errorf("Expected JWTSecret 'my-secret-key', got '%s'", cfg.JWTSecret)
	}
	if cfg.JWTIssuer != "my-app" {
		t.Errorf("Expected JWTIssuer 'my-app', got '%s'", cfg.JWTIssuer)
	}
}

func TestLoadFromEnv_APIKeys(t *testing.T) {
	cfg := Default()

	os.Setenv("AUTH_TYPE", "apikey")
	os.Setenv("API_KEYS", "key1, key2, key3")
	defer os.Unsetenv("AUTH_TYPE")
	defer os.Unsetenv("API_KEYS")

	LoadFromEnv(cfg)

	if cfg.AuthType != "apikey" {
		t.Errorf("Expected AuthType 'apikey', got '%s'", cfg.AuthType)
	}
	if len(cfg.APIKeys) != 3 {
		t.Errorf("Expected 3 API keys, got %d", len(cfg.APIKeys))
	}
	// Check trimming
	if cfg.APIKeys[1] != "key2" {
		t.Errorf("Expected APIKeys[1] 'key2', got '%s'", cfg.APIKeys[1])
	}
}

func TestLoadFromEnv_RateLimit(t *testing.T) {
	cfg := Default()

	os.Setenv("RATE_LIMIT_ENABLED", "true")
	os.Setenv("RATE_LIMIT_RATE", "50")
	os.Setenv("RATE_LIMIT_WINDOW", "30")
	os.Setenv("RATE_LIMIT_BY_IP", "false")
	os.Setenv("RATE_LIMIT_BY_KEY", "true")
	defer os.Unsetenv("RATE_LIMIT_ENABLED")
	defer os.Unsetenv("RATE_LIMIT_RATE")
	defer os.Unsetenv("RATE_LIMIT_WINDOW")
	defer os.Unsetenv("RATE_LIMIT_BY_IP")
	defer os.Unsetenv("RATE_LIMIT_BY_KEY")

	LoadFromEnv(cfg)

	if !cfg.RateLimitEnabled {
		t.Error("Expected RateLimitEnabled true")
	}
	if cfg.RateLimitRate != 50 {
		t.Errorf("Expected RateLimitRate 50, got %d", cfg.RateLimitRate)
	}
	if cfg.RateLimitWindow != 30 {
		t.Errorf("Expected RateLimitWindow 30, got %d", cfg.RateLimitWindow)
	}
	if cfg.RateLimitByIP {
		t.Error("Expected RateLimitByIP false")
	}
	if !cfg.RateLimitByKey {
		t.Error("Expected RateLimitByKey true")
	}
}

func TestLoadFromEnv_Metrics(t *testing.T) {
	cfg := Default()

	os.Setenv("METRICS_ENABLED", "false")
	defer os.Unsetenv("METRICS_ENABLED")

	LoadFromEnv(cfg)

	if cfg.MetricsEnabled {
		t.Error("Expected MetricsEnabled false")
	}
}

func TestLoadFromEnv_Debug(t *testing.T) {
	cfg := Default()

	os.Setenv("DEBUG", "1")
	os.Setenv("DEBUG_LOCKS", "true")
	defer os.Unsetenv("DEBUG")
	defer os.Unsetenv("DEBUG_LOCKS")

	LoadFromEnv(cfg)

	if !cfg.Debug {
		t.Error("Expected Debug true")
	}
	if !cfg.DebugLocks {
		t.Error("Expected DebugLocks true")
	}
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"1", true},
		{"yes", true},
		{"Yes", true},
		{"false", false},
		{"0", false},
		{"no", false},
		{"", false},
		{"invalid", false},
	}

	for _, tc := range tests {
		result := parseBool(tc.input)
		if result != tc.expected {
			t.Errorf("parseBool(%q) = %v, expected %v", tc.input, result, tc.expected)
		}
	}
}

func TestLoadFromEnv_InvalidPort(t *testing.T) {
	cfg := Default()
	originalPort := cfg.Port

	os.Setenv("PORT", "invalid")
	defer os.Unsetenv("PORT")

	LoadFromEnv(cfg)

	// Should keep default on invalid input
	if cfg.Port != originalPort {
		t.Errorf("Expected Port to remain %d on invalid input, got %d", originalPort, cfg.Port)
	}
}

func TestLoadFromEnv_TenantMode(t *testing.T) {
	cfg := Default()

	os.Setenv("TENANT_MODE", "strict")
	defer os.Unsetenv("TENANT_MODE")

	LoadFromEnv(cfg)

	if cfg.TenantMode != "strict" {
		t.Errorf("Expected TenantMode 'strict', got '%s'", cfg.TenantMode)
	}
}

func TestDefault_TenantMode(t *testing.T) {
	cfg := Default()

	if cfg.TenantMode != "none" {
		t.Errorf("Expected default TenantMode 'none', got '%s'", cfg.TenantMode)
	}
}
