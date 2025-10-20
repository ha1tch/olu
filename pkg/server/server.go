package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/ha1tch/olu/pkg/cache"
	"github.com/ha1tch/olu/pkg/config"
	"github.com/ha1tch/olu/pkg/graph"
	"github.com/ha1tch/olu/pkg/models"
	"github.com/ha1tch/olu/pkg/storage"
	"github.com/ha1tch/olu/pkg/validation"
)

// Server represents the HTTP server
type Server struct {
	config    *config.Config
	storage   storage.Store
	cache     cache.Cache
	graph     graph.Graph
	validator validation.Validator
	logger    zerolog.Logger
	router    *chi.Mux
}

// New creates a new server instance
func New(
	cfg *config.Config,
	store storage.Store,
	cache cache.Cache,
	graph graph.Graph,
	validator validation.Validator,
	logger zerolog.Logger,
) *Server {
	s := &Server{
		config:    cfg,
		storage:   store,
		cache:     cache,
		graph:     graph,
		validator: validator,
		logger:    logger,
		router:    chi.NewRouter(),
	}
	
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
	
	// Health check
	s.router.Get("/health", s.handleHealth)
	s.router.Get("/version", s.handleVersion)
	
	// API routes
	s.router.Route("/api/v1", func(r chi.Router) {
		// Entity CRUD operations
		r.Post("/{entity}", s.handleCreate)
		r.Get("/{entity}", s.handleList)
		r.Get("/{entity}/{id}", s.handleGet)
		r.Put("/{entity}/{id}", s.handleUpdate)
		r.Patch("/{entity}/{id}", s.handlePatch)
		r.Delete("/{entity}/{id}", s.handleDelete)
		r.Post("/{entity}/save/{id}", s.handleSave)
		
		// Graph operations
		if s.config.GraphEnabled {
			r.Post("/graph/path", s.handleGraphPath)
			r.Post("/graph/neighbors", s.handleGraphNeighbors)
			r.Get("/graph/stats", s.handleGraphStats)
		}
		
		// Schema operations
		r.Post("/schema/{entity}", s.handleCreateSchema)
		r.Get("/schema/{entity}", s.handleGetSchema)
	})
}

// Start starts the HTTP server
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	s.logger.Info().Str("addr", addr).Msg("Starting server")
	return http.ListenAndServe(addr, s.router)
}

// Handler returns the HTTP handler (useful for testing)
func (s *Server) Handler() http.Handler {
	return s.router
}

// handleHealth returns server health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"version": config.Version,
	})
}

// handleVersion returns server version
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"version": config.Version,
	})
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
		
		// Save graph
		if err := s.graph.Save(s.config.GraphDataFile); err != nil {
			s.logger.Error().Err(err).Msg("Failed to save graph")
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
	
	// Check cache
	cacheKey := fmt.Sprintf("%s:list:%d:%d", entity, page, perPage)
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
	
	// Check cache
	cacheKey := fmt.Sprintf("%s:%d", entity, id)
	if cached, err := s.cache.Get(r.Context(), cacheKey); err == nil {
		s.writeJSON(w, http.StatusOK, cached)
		return
	}
	
	// Get entity
	data, err := s.storage.Get(r.Context(), entity, id)
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
	
	// Embed references if requested
	embedDepth, _ := strconv.Atoi(r.URL.Query().Get("embed_depth"))
	if embedDepth > 0 && embedDepth <= s.config.MaxEmbedDepth {
		data = s.embedReferences(r.Context(), data, embedDepth)
	}
	
	// Cache result
	_ = s.cache.Set(r.Context(), cacheKey, data, time.Duration(s.config.CacheTTL)*time.Second)
	
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
		_ = s.graph.Save(s.config.GraphDataFile)
	}
	
	s.invalidateCache(entity)
	s.logger.Info().Str("entity", entity).Int("id", id).Msg("Updated entity")
	
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("Resource of entity %s with id %d updated successfully", entity, id),
	})
}

// Continued in next part...
