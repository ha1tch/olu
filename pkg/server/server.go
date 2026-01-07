package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/ha1tch/olu/pkg/cache"
	"github.com/ha1tch/olu/pkg/config"
	"github.com/ha1tch/olu/pkg/graph"
	oluMiddleware "github.com/ha1tch/olu/pkg/middleware"
	"github.com/ha1tch/olu/pkg/models"
	"github.com/ha1tch/olu/pkg/oql"
	"github.com/ha1tch/olu/pkg/storage"
	"github.com/ha1tch/olu/pkg/sulpher"
	"github.com/ha1tch/olu/pkg/validation"
	"github.com/ha1tch/olu/pkg/version"
	"github.com/rs/zerolog"
)

// Context key type for tenant isolation
type contextKey string

const tenantContextKey contextKey = "tenant_id"

// Server represents the HTTP server
type Server struct {
	config      *config.Config
	storage     storage.Store
	cache       cache.Cache
	graph       graph.Graph
	persister   *graph.AdaptivePersister
	validator   validation.Validator
	sulpherJobs *sulpher.JobManager
	oqlJobs     *oql.JobManager
	rateLimiter *oluMiddleware.RateLimiter
	metrics     *oluMiddleware.Metrics
	logger      zerolog.Logger
	router      *chi.Mux
}

// tenantMiddleware extracts tenant_id from URL and stores in context
func (s *Server) tenantMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenant_id")
		if tenantID == "" {
			s.writeError(w, http.StatusBadRequest, "tenant_id required")
			return
		}
		ctx := context.WithValue(r.Context(), tenantContextKey, tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// tenantStrictMiddleware enforces tenant context in strict mode.
// It rejects requests to entity routes that don't have tenant context.
// Used when TenantMode is "strict".
func (s *Server) tenantStrictMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this is an entity operation path (not system paths)
		path := r.URL.Path
		
		// Allow system endpoints
		if path == "/health" || path == "/version" || path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		
		// Allow non-entity API endpoints (graph, oql, schema, export, search)
		if strings.Contains(path, "/graph/") ||
			strings.Contains(path, "/oql/") ||
			strings.Contains(path, "/schema/") ||
			strings.Contains(path, "/export") ||
			strings.Contains(path, "/search") {
			next.ServeHTTP(w, r)
			return
		}
		
		// For entity routes, require tenant context
		if strings.HasPrefix(path, "/api/v1/") && !strings.Contains(path, "/tenant/") {
			s.writeError(w, http.StatusForbidden,
				"Tenant context required. Use /api/v1/tenant/{tenant_id}/... routes")
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// getTenantID returns tenant_id from context, or empty string if not set
func getTenantID(ctx context.Context) string {
	if v := ctx.Value(tenantContextKey); v != nil {
		return v.(string)
	}
	return ""
}

// New creates a new server instance
func New(
	cfg *config.Config,
	store storage.Store,
	cache cache.Cache,
	g graph.Graph,
	persister *graph.AdaptivePersister,
	validator validation.Validator,
	logger zerolog.Logger,
) *Server {
	s := &Server{
		config:    cfg,
		storage:   store,
		cache:     cache,
		graph:     g,
		persister: persister,
		validator: validator,
		logger:    logger,
		router:    chi.NewRouter(),
	}

	// Initialize rate limiter if enabled
	if cfg.RateLimitEnabled {
		s.rateLimiter = oluMiddleware.NewRateLimiter(cfg)
	}

	// Initialize metrics if enabled
	if cfg.MetricsEnabled {
		s.metrics = oluMiddleware.NewMetrics()
	}

	// Initialize Sulpher query engine if graph is enabled
	if cfg.GraphEnabled {
		if ig, ok := g.(*graph.IndexedGraph); ok {
			executor := sulpher.NewExecutor(ig, cfg.MaxQueryDepth)
			s.sulpherJobs = sulpher.NewJobManager(executor, time.Duration(cfg.GraphQueryTTL)*time.Second)
		}
	}

	// Initialize OQL query engine with schema validation
	oqlEngine := oql.NewEngineWithSchemaValidator(store, cfg.SchemaDir, validator)
	s.oqlJobs = oql.NewJobManager(oqlEngine, time.Duration(cfg.GraphQueryTTL)*time.Second)

	s.setupRoutes()
	return s
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Timeout(60 * time.Second))

	// Metrics middleware (before auth so we capture all requests)
	if s.metrics != nil {
		s.router.Use(oluMiddleware.MetricsMiddleware(s.metrics))
	}

	// Authentication middleware (applied to all routes, checks exclusions internally)
	s.router.Use(oluMiddleware.AuthMiddleware(s.config))

	// Rate limiting middleware
	if s.rateLimiter != nil {
		s.router.Use(oluMiddleware.RateLimitMiddleware(s.config, s.rateLimiter))
	}

	// Tenant strict mode middleware
	if s.config.TenantMode == "strict" {
		s.router.Use(s.tenantStrictMiddleware)
	}

	// Health check and metrics
	s.router.Get("/health", s.handleHealth)
	s.router.Get("/version", s.handleVersion)
	s.router.Get("/metrics", s.handleMetrics)

	// API routes
	s.router.Route("/api/v1", func(r chi.Router) {
		// Entity CRUD operations (non-tenant)
		r.Post("/{entity}", s.handleCreate)
		r.Get("/{entity}", s.handleList)
		r.Get("/{entity}/{id}", s.handleGet)
		r.Put("/{entity}/{id}", s.handleUpdate)
		r.Patch("/{entity}/{id}", s.handlePatch)
		r.Delete("/{entity}/{id}", s.handleDelete)
		r.Post("/{entity}/save/{id}", s.handleSave)

		// Tenant-scoped routes
		r.Route("/tenant/{tenant_id}", func(tr chi.Router) {
			tr.Use(s.tenantMiddleware)
			tr.Post("/{entity}", s.handleCreate)
			tr.Get("/{entity}", s.handleList)
			tr.Get("/{entity}/{id}", s.handleGet)
			tr.Put("/{entity}/{id}", s.handleUpdate)
			tr.Patch("/{entity}/{id}", s.handlePatch)
			tr.Delete("/{entity}/{id}", s.handleDelete)
			tr.Post("/{entity}/save/{id}", s.handleSave)
		})

		// Graph operations
		if s.config.GraphEnabled {
			// Existing endpoints
			r.Post("/graph/path", s.handleGraphPath)
			r.Post("/graph/neighbors", s.handleGraphNeighbors)
			r.Get("/graph/stats", s.handleGraphStats)

			// New rserv-compatible endpoints
			r.Get("/graph/nodes/{node_id}", s.handleGraphNodeInfo)
			r.Get("/graph/nodes/{node_id}/degree", s.handleGraphNodeDegree)
			r.Get("/graph/{node_id}/in", s.handleGraphIncoming)
			r.Get("/graph/{node_id}/out", s.handleGraphOutgoing)
			r.Post("/graph/shortestPath", s.handleGraphShortestPath)
			r.Post("/graph/pathExists", s.handleGraphPathExists)
			r.Post("/graph/commonNeighbors", s.handleGraphCommonNeighbors)
			r.Post("/graph/nodes/search", s.handleGraphNodeSearch)

			// Sulpher query language endpoints
			r.Post("/graph/query", s.handleSulpherQuery)
			r.Post("/graph/query/async", s.handleSulpherQueryAsync)
			r.Get("/graph/query/{query_id}", s.handleSulpherQueryStatus)
			r.Get("/graph/query/{query_id}/result", s.handleSulpherQueryResult)
		}

		// OQL (SQL) query language endpoints
		r.Post("/oql/query", s.handleOQLQuery)
		r.Post("/oql/query/async", s.handleOQLQueryAsync)
		r.Get("/oql/query/{query_id}", s.handleOQLQueryStatus)
		r.Get("/oql/query/{query_id}/result", s.handleOQLQueryResult)

		// Schema operations
		r.Post("/schema/{entity}", s.handleCreateSchema)
		r.Get("/schema/{entity}", s.handleGetSchema)

		// Export operations
		r.Get("/export", s.handleExport)

		// Search operations
		r.Get("/search", s.handleFullTextSearch)
	})
}

// Start starts the HTTP server
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	s.logger.Info().Str("addr", addr).Msg("Starting server")
	return http.ListenAndServe(addr, s.router)
}

// Stop stops the server and cleans up resources
func (s *Server) Stop() {
	if s.rateLimiter != nil {
		s.rateLimiter.Stop()
	}
}

// Handler returns the HTTP handler (useful for testing)
func (s *Server) Handler() http.Handler {
	return s.router
}

// handleHealth returns server health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"version": version.Version,
	})
}

// handleVersion returns server version
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"version": version.Version,
	})
}

// handleMetrics returns Prometheus-format metrics
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if s.metrics == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Metrics not enabled")
		return
	}

	// Check Accept header for format preference
	accept := r.Header.Get("Accept")
	if accept == "application/json" {
		// Return JSON format
		snapshot := s.metrics.GetSnapshot()
		s.writeJSON(w, http.StatusOK, snapshot)
		return
	}

	// Default to Prometheus format
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(s.metrics.PrometheusFormat()))
}

// handleCreate creates a new entity
func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	entity := chi.URLParam(r, "entity")
	if err := validateEntityName(entity); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Inject tenant_id if in tenant-scoped route
	if tenantID := getTenantID(r.Context()); tenantID != "" {
		data["tenant_id"] = tenantID
	}

	// Validate against schema
	if valid, errors := s.validator.Validate(entity, data); !valid {
		s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":   "Validation failed",
			"details": errors,
		})
		return
	}

	// Check size limit
	jsonData, _ := json.Marshal(data)
	if len(jsonData) > s.config.MaxEntitySize {
		s.writeError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("Entity too large: %d bytes (max: %d)",
				len(jsonData), s.config.MaxEntitySize))
		return
	}

	// Create entity
	id, err := s.storage.Create(r.Context(), entity, data)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to create entity")
		s.writeError(w, http.StatusInternalServerError, "Failed to create entity")
		return
	}

	// Update graph if enabled
	if s.config.GraphEnabled {
		data["id"] = id
		if err := s.graph.UpdateFromEntity(entity, id, data); err != nil {
			s.logger.Error().Err(err).Msg("Failed to update graph")
		}
		if s.persister != nil {
			s.persister.MarkDirty()
		}
	}

	// Invalidate cache
	s.invalidateCache(entity)

	s.logger.Info().Str("entity", entity).Int("id", id).Msg("Created entity")

	s.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"message": fmt.Sprintf("Resource of entity %s created successfully", entity),
		"id":      id,
	})
}

// handleList lists all entities of a type
func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	entity := chi.URLParam(r, "entity")
	if err := validateEntityName(entity); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Get pagination params
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = s.config.DefaultPageSize
		if perPage < 1 {
			perPage = 10 // Fallback default
		}
	}

	// Extract filter params (exclude pagination and system params)
	filters := extractFilters(r.URL.Query())

	// Add tenant_id filter if in tenant-scoped route
	if tenantID := getTenantID(r.Context()); tenantID != "" {
		filters["tenant_id"] = tenantID
	}

	// Build cache key including filters
	cacheKey := buildListCacheKey(entity, page, perPage, filters)
	if cached, err := s.cache.Get(r.Context(), cacheKey); err == nil {
		s.writeJSON(w, http.StatusOK, cached)
		return
	}

	// Get all entities
	entities, err := s.storage.List(r.Context(), entity)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to list entities")
		s.writeError(w, http.StatusInternalServerError, "Failed to list entities")
		return
	}

	// Apply filters
	if len(filters) > 0 {
		entities = applyFilters(entities, filters)
	}

	// Apply pagination
	totalItems := len(entities)
	totalPages := (totalItems + perPage - 1) / perPage

	start := (page - 1) * perPage
	end := start + perPage
	if end > totalItems {
		end = totalItems
	}

	var pageData []map[string]interface{}
	if start < totalItems {
		pageData = entities[start:end]
	} else {
		pageData = []map[string]interface{}{}
	}

	// Embed references if explicitly requested (not default for lists due to performance)
	embedParam := r.URL.Query().Get("embed")
	if embedParam != "false" && embedParam != "0" {
		embedDepth := 0
		if depthParam := r.URL.Query().Get("embed_depth"); depthParam != "" {
			if parsed, err := strconv.Atoi(depthParam); err == nil && parsed > 0 {
				embedDepth = parsed
			}
		}
		if embedDepth > s.config.MaxEmbedDepth {
			embedDepth = s.config.MaxEmbedDepth
		}
		if embedDepth > 0 {
			for i, item := range pageData {
				pageData[i] = s.embedReferences(r.Context(), item, embedDepth)
			}
		}
	}

	response := models.PagedResponse{
		Data: pageData,
	}
	response.Pagination.Page = page
	response.Pagination.PerPage = perPage
	response.Pagination.TotalItems = totalItems
	response.Pagination.TotalPages = totalPages

	// Cache result
	_ = s.cache.Set(r.Context(), cacheKey, response, time.Duration(s.config.CacheTTL)*time.Second)

	s.writeJSON(w, http.StatusOK, response)
}

// handleGet retrieves a single entity
func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	entity := chi.URLParam(r, "entity")
	idStr := chi.URLParam(r, "id")

	if err := validateEntityName(entity); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil || id < 0 {
		s.writeError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	// Check cache for raw entity data
	cacheKey := fmt.Sprintf("%s:%d", entity, id)
	var data map[string]interface{}

	if cached, err := s.cache.Get(r.Context(), cacheKey); err == nil {
		// Cache hit - use cached data
		if cachedData, ok := cached.(map[string]interface{}); ok {
			// Make a copy to avoid mutating cached data during embedding
			data = make(map[string]interface{}, len(cachedData))
			for k, v := range cachedData {
				data[k] = v
			}
		}
	}

	if data == nil {
		// Cache miss - fetch from storage
		var err error
		data, err = s.storage.Get(r.Context(), entity, id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				s.writeError(w, http.StatusNotFound,
					fmt.Sprintf("Resource of entity %s with id %d not found", entity, id))
				return
			}
			s.logger.Error().Err(err).Msg("Failed to get entity")
			s.writeError(w, http.StatusInternalServerError, "Failed to get entity")
			return
		}

		// Cache the raw data
		_ = s.cache.Set(r.Context(), cacheKey, data, time.Duration(s.config.CacheTTL)*time.Second)
	}

	// Verify tenant access if in tenant-scoped route
	if tenantID := getTenantID(r.Context()); tenantID != "" {
		if !matchesTenant(data, tenantID) {
			s.writeError(w, http.StatusNotFound,
				fmt.Sprintf("Resource of entity %s with id %d not found", entity, id))
			return
		}
	}

	// Embed references
	// Use RefEmbedDepth as default, allow override via query param
	// Use embed=false to disable entirely
	embedParam := r.URL.Query().Get("embed")
	if embedParam == "false" || embedParam == "0" {
		// Embedding explicitly disabled
	} else {
		embedDepth := s.config.RefEmbedDepth
		if depthParam := r.URL.Query().Get("embed_depth"); depthParam != "" {
			if parsed, err := strconv.Atoi(depthParam); err == nil && parsed >= 0 {
				embedDepth = parsed
			}
		}
		if embedDepth > s.config.MaxEmbedDepth {
			embedDepth = s.config.MaxEmbedDepth
		}
		if embedDepth > 0 {
			data = s.embedReferences(r.Context(), data, embedDepth)
		}
	}

	s.writeJSON(w, http.StatusOK, data)
}

// handleUpdate updates an entire entity
func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	entity := chi.URLParam(r, "entity")
	idStr := chi.URLParam(r, "id")

	if err := validateEntityName(entity); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil || id < 0 {
		s.writeError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Verify tenant access and inject tenant_id if in tenant-scoped route
	if tenantID := getTenantID(r.Context()); tenantID != "" {
		existing, err := s.storage.Get(r.Context(), entity, id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				s.writeError(w, http.StatusNotFound,
					fmt.Sprintf("Resource of entity %s with id %d not found", entity, id))
				return
			}
			s.logger.Error().Err(err).Msg("Failed to get entity for tenant check")
			s.writeError(w, http.StatusInternalServerError, "Failed to update entity")
			return
		}
		if !matchesTenant(existing, tenantID) {
			s.writeError(w, http.StatusNotFound,
				fmt.Sprintf("Resource of entity %s with id %d not found", entity, id))
			return
		}
		data["tenant_id"] = tenantID
	}

	// Validate
	data["id"] = id
	if valid, errors := s.validator.Validate(entity, data); !valid {
		s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":   "Validation failed",
			"details": errors,
		})
		return
	}

	// Update
	if err := s.storage.Update(r.Context(), entity, id, data); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.writeError(w, http.StatusNotFound,
				fmt.Sprintf("Resource of entity %s with id %d not found", entity, id))
			return
		}
		s.logger.Error().Err(err).Msg("Failed to update entity")
		s.writeError(w, http.StatusInternalServerError, "Failed to update entity")
		return
	}

	// Update graph
	if s.config.GraphEnabled {
		if err := s.graph.UpdateFromEntity(entity, id, data); err != nil {
			s.logger.Error().Err(err).Msg("Failed to update graph")
		}
		if s.persister != nil {
			s.persister.MarkDirty()
		}
	}

	s.invalidateCache(entity)
	s.logger.Info().Str("entity", entity).Int("id", id).Msg("Updated entity")

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("Resource of entity %s with id %d updated successfully", entity, id),
	})
}

// Continued in next part...

// reservedQueryParams are params that should not be treated as filters
var reservedQueryParams = map[string]bool{
	"page":        true,
	"per_page":    true,
	"embed_depth": true,
	"sort":        true,
	"order":       true,
}

// extractFilters extracts filter parameters from query string
func extractFilters(query url.Values) map[string]string {
	filters := make(map[string]string)
	for key, values := range query {
		if reservedQueryParams[key] {
			continue
		}
		if len(values) > 0 && values[0] != "" {
			filters[key] = values[0]
		}
	}
	return filters
}

// buildListCacheKey creates a deterministic cache key including filters
func buildListCacheKey(entity string, page, perPage int, filters map[string]string) string {
	if len(filters) == 0 {
		return fmt.Sprintf("%s:list:%d:%d", entity, page, perPage)
	}

	// Sort filter keys for deterministic cache key
	keys := make([]string, 0, len(filters))
	for k := range filters {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var filterParts []string
	for _, k := range keys {
		filterParts = append(filterParts, fmt.Sprintf("%s=%s", k, filters[k]))
	}

	return fmt.Sprintf("%s:list:%d:%d:%s", entity, page, perPage, strings.Join(filterParts, ","))
}

// applyFilters filters entities by matching field values
func applyFilters(entities []map[string]interface{}, filters map[string]string) []map[string]interface{} {
	if len(filters) == 0 {
		return entities
	}

	var result []map[string]interface{}
	for _, entity := range entities {
		if matchesFilters(entity, filters) {
			result = append(result, entity)
		}
	}
	return result
}

// matchesFilters checks if an entity matches all filter criteria
func matchesFilters(entity map[string]interface{}, filters map[string]string) bool {
	for field, expected := range filters {
		value, exists := entity[field]
		if !exists {
			return false
		}

		// Convert value to string for comparison
		var actual string
		switch v := value.(type) {
		case string:
			actual = v
		case float64:
			// Handle both integer and float JSON numbers
			if v == float64(int(v)) {
				actual = strconv.Itoa(int(v))
			} else {
				actual = strconv.FormatFloat(v, 'f', -1, 64)
			}
		case int:
			actual = strconv.Itoa(v)
		case bool:
			actual = strconv.FormatBool(v)
		default:
			actual = fmt.Sprintf("%v", v)
		}

		if actual != expected {
			return false
		}
	}
	return true
}

// matchesTenant checks if an entity belongs to the specified tenant
func matchesTenant(entity map[string]interface{}, tenantID string) bool {
	value, exists := entity["tenant_id"]
	if !exists {
		return false
	}

	// Convert value to string for comparison
	var actual string
	switch v := value.(type) {
	case string:
		actual = v
	case float64:
		if v == float64(int(v)) {
			actual = strconv.Itoa(int(v))
		} else {
			actual = strconv.FormatFloat(v, 'f', -1, 64)
		}
	case int:
		actual = strconv.Itoa(v)
	default:
		actual = fmt.Sprintf("%v", v)
	}

	return actual == tenantID
}
