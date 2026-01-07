package server

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ha1tch/olu/pkg/graph"
	"github.com/ha1tch/olu/pkg/models"
	"github.com/ha1tch/olu/pkg/version"
)

// handlePatch partially updates an entity
func (s *Server) handlePatch(w http.ResponseWriter, r *http.Request) {
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

	var patchData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&patchData); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Get existing entity
	existing, err := s.storage.Get(r.Context(), entity, id)
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

	// Verify tenant access if in tenant-scoped route
	tenantID := getTenantID(r.Context())
	if tenantID != "" {
		if !matchesTenant(existing, tenantID) {
			s.writeError(w, http.StatusNotFound,
				fmt.Sprintf("Resource of entity %s with id %d not found", entity, id))
			return
		}
	}

	// Handle null behavior
	updatedFields := []string{}
	for key, value := range patchData {
		// Skip id and tenant_id - these cannot be changed
		if key == "id" || key == "tenant_id" {
			continue
		}
		if value == nil && s.config.PatchNullBehavior == "delete" {
			delete(existing, key)
		} else {
			existing[key] = value
		}
		updatedFields = append(updatedFields, key)
	}

	// Validate merged data
	if valid, errors := s.validator.Validate(entity, existing); !valid {
		s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":   "Validation failed",
			"details": errors,
		})
		return
	}

	// Update
	if err := s.storage.Update(r.Context(), entity, id, existing); err != nil {
		s.logger.Error().Err(err).Msg("Failed to patch entity")
		s.writeError(w, http.StatusInternalServerError, "Failed to patch entity")
		return
	}

	// Update graph
	if s.config.GraphEnabled {
		if err := s.graph.UpdateFromEntity(entity, id, existing); err != nil {
			s.logger.Error().Err(err).Msg("Failed to update graph")
		}
		if s.persister != nil {
			s.persister.MarkDirty()
		}
	}

	s.invalidateCache(entity)
	s.logger.Info().Str("entity", entity).Int("id", id).Msg("Patched entity")

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":        fmt.Sprintf("%s with id %d patched successfully", entity, id),
		"updated_fields": updatedFields,
	})
}

// handleDelete deletes an entity
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
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

	// Check if entity exists
	if !s.storage.Exists(r.Context(), entity, id) {
		s.writeError(w, http.StatusNotFound,
			fmt.Sprintf("Resource of entity %s with id %d not found", entity, id))
		return
	}

	// Verify tenant access if in tenant-scoped route
	if tenantID := getTenantID(r.Context()); tenantID != "" {
		existing, err := s.storage.Get(r.Context(), entity, id)
		if err != nil || !matchesTenant(existing, tenantID) {
			s.writeError(w, http.StatusNotFound,
				fmt.Sprintf("Resource of entity %s with id %d not found", entity, id))
			return
		}
	}

	// Handle cascading delete
	deletedRefs := []string{fmt.Sprintf("%s:%d", entity, id)}
	if s.config.CascadingDelete {
		refs, err := s.cascadeDelete(r.Context(), entity, id)
		if err != nil {
			s.logger.Error().Err(err).Msg("Cascade delete failed")
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		deletedRefs = refs
	} else {
		// Simple delete
		if err := s.storage.Delete(r.Context(), entity, id); err != nil {
			s.logger.Error().Err(err).Msg("Failed to delete entity")
			s.writeError(w, http.StatusInternalServerError, "Failed to delete entity")
			return
		}

		// Update graph
		if s.config.GraphEnabled {
			nodeID := fmt.Sprintf("%s:%d", entity, id)
			if err := s.graph.RemoveNode(nodeID); err != nil {
				s.logger.Error().Err(err).Msg("Failed to remove from graph")
			}
			if s.persister != nil {
				s.persister.MarkDirty()
			}
		}
	}

	s.invalidateCache(entity)
	s.logger.Info().Str("entity", entity).Int("id", id).Msg("Deleted entity")

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":          fmt.Sprintf("%s with id %d deleted successfully", entity, id),
		"cascaded_deletes": deletedRefs,
	})
}

// handleSave saves an entity with a specific ID
func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
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

	// Check if already exists
	if s.storage.Exists(r.Context(), entity, id) {
		s.writeError(w, http.StatusConflict,
			fmt.Sprintf("Resource of entity %s with id %d already exists", entity, id))
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

	// Validate
	data["id"] = id
	if valid, errors := s.validator.Validate(entity, data); !valid {
		s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":   "Validation failed",
			"details": errors,
		})
		return
	}

	// Save
	if err := s.storage.Save(r.Context(), entity, id, data); err != nil {
		s.logger.Error().Err(err).Msg("Failed to save entity")
		s.writeError(w, http.StatusInternalServerError, "Failed to save entity")
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
	s.logger.Info().Str("entity", entity).Int("id", id).Msg("Saved entity")

	s.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"message": fmt.Sprintf("Resource of entity %s saved successfully with id %d", entity, id),
	})
}

// handleGraphPath finds a path between two nodes
func (s *Server) handleGraphPath(w http.ResponseWriter, r *http.Request) {
	if !s.config.GraphEnabled {
		s.writeError(w, http.StatusNotImplemented, "Graph operations are disabled")
		return
	}

	var req struct {
		From     string `json:"from"`
		To       string `json:"to"`
		MaxDepth int    `json:"max_depth"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if req.MaxDepth <= 0 {
		req.MaxDepth = s.config.MaxQueryDepth
	}

	path, err := s.graph.FindPath(req.From, req.To, req.MaxDepth)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"from":   req.From,
		"to":     req.To,
		"path":   path,
		"length": len(path) - 1,
	})
}

// handleGraphNeighbors gets neighbors of a node
func (s *Server) handleGraphNeighbors(w http.ResponseWriter, r *http.Request) {
	if !s.config.GraphEnabled {
		s.writeError(w, http.StatusNotImplemented, "Graph operations are disabled")
		return
	}

	var req struct {
		NodeID    string `json:"node_id"`
		Direction string `json:"direction"` // "out", "in", or "both"
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if req.Direction == "" {
		req.Direction = "out"
	}

	result := make(map[string]interface{})

	if req.Direction == "out" || req.Direction == "both" {
		neighbors, err := s.graph.GetNeighbors(req.NodeID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		result["outgoing"] = neighbors
	}

	if req.Direction == "in" || req.Direction == "both" {
		incoming, err := s.graph.GetIncomingEdges(req.NodeID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		result["incoming"] = incoming
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"neighbors": result,
	})
}

// handleGraphStats returns graph statistics
func (s *Server) handleGraphStats(w http.ResponseWriter, r *http.Request) {
	if !s.config.GraphEnabled {
		s.writeError(w, http.StatusNotImplemented, "Graph operations are disabled")
		return
	}

	nodeCount := 0
	edgeCount := 0

	if ig, ok := s.graph.(*graph.IndexedGraph); ok {
		nodeCount = ig.NodeCount()
		edgeCount = ig.EdgeCount()
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"node_count": nodeCount,
		"edge_count": edgeCount,
		"has_cycle":  s.graph.HasCycle(),
	})
}

// handleGraphNodeInfo returns detailed info about a specific node
// GET /api/v1/graph/nodes/{node_id}
func (s *Server) handleGraphNodeInfo(w http.ResponseWriter, r *http.Request) {
	if !s.config.GraphEnabled {
		s.writeError(w, http.StatusNotImplemented, "Graph operations are disabled")
		return
	}

	nodeID := chi.URLParam(r, "node_id")
	if nodeID == "" {
		s.writeError(w, http.StatusBadRequest, "node_id required")
		return
	}

	ig, ok := s.graph.(*graph.IndexedGraph)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "Graph type not supported")
		return
	}

	info, err := ig.GetNodeInfo(nodeID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, info)
}

// handleGraphNodeDegree returns degree counts for a node
// GET /api/v1/graph/nodes/{node_id}/degree
func (s *Server) handleGraphNodeDegree(w http.ResponseWriter, r *http.Request) {
	if !s.config.GraphEnabled {
		s.writeError(w, http.StatusNotImplemented, "Graph operations are disabled")
		return
	}

	nodeID := chi.URLParam(r, "node_id")
	if nodeID == "" {
		s.writeError(w, http.StatusBadRequest, "node_id required")
		return
	}

	ig, ok := s.graph.(*graph.IndexedGraph)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "Graph type not supported")
		return
	}

	degree, err := ig.GetDegree(nodeID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"node_id": nodeID,
		"degree":  degree,
	})
}

// handleGraphIncoming returns incoming edges to a node
// GET /api/v1/graph/{node_id}/in
func (s *Server) handleGraphIncoming(w http.ResponseWriter, r *http.Request) {
	if !s.config.GraphEnabled {
		s.writeError(w, http.StatusNotImplemented, "Graph operations are disabled")
		return
	}

	nodeID := chi.URLParam(r, "node_id")
	if nodeID == "" {
		s.writeError(w, http.StatusBadRequest, "node_id required")
		return
	}

	incoming, err := s.graph.GetIncomingEdges(nodeID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Format as array of edge objects for rserv compatibility
	edges := make([]map[string]string, 0, len(incoming))
	for source, relationship := range incoming {
		edges = append(edges, map[string]string{
			"source":       source,
			"target":       nodeID,
			"relationship": relationship,
		})
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"node_id": nodeID,
		"edges":   edges,
		"count":   len(edges),
	})
}

// handleGraphOutgoing returns outgoing edges from a node
// GET /api/v1/graph/{node_id}/out
func (s *Server) handleGraphOutgoing(w http.ResponseWriter, r *http.Request) {
	if !s.config.GraphEnabled {
		s.writeError(w, http.StatusNotImplemented, "Graph operations are disabled")
		return
	}

	nodeID := chi.URLParam(r, "node_id")
	if nodeID == "" {
		s.writeError(w, http.StatusBadRequest, "node_id required")
		return
	}

	outgoing, err := s.graph.GetNeighbors(nodeID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Format as array of edge objects for rserv compatibility
	edges := make([]map[string]string, 0, len(outgoing))
	for target, relationship := range outgoing {
		edges = append(edges, map[string]string{
			"source":       nodeID,
			"target":       target,
			"relationship": relationship,
		})
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"node_id": nodeID,
		"edges":   edges,
		"count":   len(edges),
	})
}

// handleGraphShortestPath finds shortest path between two nodes
// POST /api/v1/graph/shortestPath
func (s *Server) handleGraphShortestPath(w http.ResponseWriter, r *http.Request) {
	if !s.config.GraphEnabled {
		s.writeError(w, http.StatusNotImplemented, "Graph operations are disabled")
		return
	}

	var req struct {
		From     string `json:"from"`
		To       string `json:"to"`
		MaxDepth int    `json:"max_depth"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if req.From == "" || req.To == "" {
		s.writeError(w, http.StatusBadRequest, "from and to are required")
		return
	}

	if req.MaxDepth <= 0 {
		req.MaxDepth = s.config.MaxQueryDepth
	}

	path, err := s.graph.FindPath(req.From, req.To, req.MaxDepth)
	if err != nil {
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"from":   req.From,
			"to":     req.To,
			"exists": false,
			"path":   nil,
			"length": 0,
		})
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"from":   req.From,
		"to":     req.To,
		"exists": true,
		"path":   path,
		"length": len(path) - 1,
	})
}

// handleGraphPathExists checks if a path exists between two nodes
// POST /api/v1/graph/pathExists
func (s *Server) handleGraphPathExists(w http.ResponseWriter, r *http.Request) {
	if !s.config.GraphEnabled {
		s.writeError(w, http.StatusNotImplemented, "Graph operations are disabled")
		return
	}

	var req struct {
		From     string `json:"from"`
		To       string `json:"to"`
		MaxDepth int    `json:"max_depth"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if req.From == "" || req.To == "" {
		s.writeError(w, http.StatusBadRequest, "from and to are required")
		return
	}

	if req.MaxDepth <= 0 {
		req.MaxDepth = s.config.MaxQueryDepth
	}

	ig, ok := s.graph.(*graph.IndexedGraph)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "Graph type not supported")
		return
	}

	exists, length, err := ig.PathExists(req.From, req.To, req.MaxDepth)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"from":   req.From,
		"to":     req.To,
		"exists": exists,
		"length": length,
	})
}

// handleGraphCommonNeighbors finds common neighbors between two nodes
// POST /api/v1/graph/commonNeighbors
func (s *Server) handleGraphCommonNeighbors(w http.ResponseWriter, r *http.Request) {
	if !s.config.GraphEnabled {
		s.writeError(w, http.StatusNotImplemented, "Graph operations are disabled")
		return
	}

	var req struct {
		NodeA string `json:"node_a"`
		NodeB string `json:"node_b"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if req.NodeA == "" || req.NodeB == "" {
		s.writeError(w, http.StatusBadRequest, "node_a and node_b are required")
		return
	}

	ig, ok := s.graph.(*graph.IndexedGraph)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "Graph type not supported")
		return
	}

	common, err := ig.CommonNeighbors(req.NodeA, req.NodeB)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if common == nil {
		common = []string{}
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"node_a": req.NodeA,
		"node_b": req.NodeB,
		"common": common,
		"count":  len(common),
	})
}

// handleGraphNodeSearch searches for nodes by entity type
// POST /api/v1/graph/nodes/search
func (s *Server) handleGraphNodeSearch(w http.ResponseWriter, r *http.Request) {
	if !s.config.GraphEnabled {
		s.writeError(w, http.StatusNotImplemented, "Graph operations are disabled")
		return
	}

	var req struct {
		Entity string `json:"entity"`
		Limit  int    `json:"limit"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	ig, ok := s.graph.(*graph.IndexedGraph)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "Graph type not supported")
		return
	}

	var nodes []string
	if req.Entity != "" {
		nodes = ig.GetNodesByType(req.Entity)
	} else {
		nodes = ig.GetAllNodes()
	}

	// Apply limit
	if req.Limit > 0 && len(nodes) > req.Limit {
		nodes = nodes[:req.Limit]
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"nodes": nodes,
		"count": len(nodes),
	})
}

// Sulpher Query Handlers

// handleSulpherQuery executes a Sulpher query synchronously
// POST /api/v1/graph/query
func (s *Server) handleSulpherQuery(w http.ResponseWriter, r *http.Request) {
	if !s.config.GraphEnabled {
		s.writeError(w, http.StatusNotImplemented, "Graph operations are disabled")
		return
	}

	if s.sulpherJobs == nil {
		s.writeError(w, http.StatusNotImplemented, "Sulpher query engine not initialized")
		return
	}

	var req struct {
		Query    string `json:"query"`
		MaxDepth int    `json:"max_depth"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if req.Query == "" {
		s.writeError(w, http.StatusBadRequest, "Query is required")
		return
	}

	if req.MaxDepth <= 0 {
		req.MaxDepth = s.config.MaxQueryDepth
	}

	result, err := s.sulpherJobs.ExecuteSync(req.Query, req.MaxDepth)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "completed",
		"result": result.Data,
		"stats": map[string]interface{}{
			"nodes_traversed":   result.Stats.NodesTraversed,
			"paths_found":       result.Stats.PathsFound,
			"execution_time_ms": result.Stats.ExecutionTime.Milliseconds(),
		},
	})
}

// handleSulpherQueryAsync submits a Sulpher query for async execution
// POST /api/v1/graph/query/async
func (s *Server) handleSulpherQueryAsync(w http.ResponseWriter, r *http.Request) {
	if !s.config.GraphEnabled {
		s.writeError(w, http.StatusNotImplemented, "Graph operations are disabled")
		return
	}

	if s.sulpherJobs == nil {
		s.writeError(w, http.StatusNotImplemented, "Sulpher query engine not initialized")
		return
	}

	var req struct {
		Query    string `json:"query"`
		MaxDepth int    `json:"max_depth"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if req.Query == "" {
		s.writeError(w, http.StatusBadRequest, "Query is required")
		return
	}

	if req.MaxDepth <= 0 {
		req.MaxDepth = s.config.MaxQueryDepth
	}

	job, err := s.sulpherJobs.Submit(req.Query, req.MaxDepth)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"query_id":   job.ID,
		"status":     job.Status,
		"created_at": job.CreatedAt,
	})
}

// handleSulpherQueryStatus gets the status of an async query
// GET /api/v1/graph/query/{query_id}
func (s *Server) handleSulpherQueryStatus(w http.ResponseWriter, r *http.Request) {
	if !s.config.GraphEnabled {
		s.writeError(w, http.StatusNotImplemented, "Graph operations are disabled")
		return
	}

	if s.sulpherJobs == nil {
		s.writeError(w, http.StatusNotImplemented, "Sulpher query engine not initialized")
		return
	}

	queryID := chi.URLParam(r, "query_id")
	if queryID == "" {
		s.writeError(w, http.StatusBadRequest, "query_id required")
		return
	}

	job, exists := s.sulpherJobs.GetJob(queryID)
	if !exists {
		s.writeError(w, http.StatusNotFound, "Query not found")
		return
	}

	response := map[string]interface{}{
		"query_id":   job.ID,
		"query":      job.Query,
		"status":     job.Status,
		"created_at": job.CreatedAt,
	}

	if job.StartedAt != nil {
		response["started_at"] = job.StartedAt
	}
	if job.EndedAt != nil {
		response["ended_at"] = job.EndedAt
	}
	if job.Error != "" {
		response["error"] = job.Error
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleSulpherQueryResult gets the result of a completed async query
// GET /api/v1/graph/query/{query_id}/result
func (s *Server) handleSulpherQueryResult(w http.ResponseWriter, r *http.Request) {
	if !s.config.GraphEnabled {
		s.writeError(w, http.StatusNotImplemented, "Graph operations are disabled")
		return
	}

	if s.sulpherJobs == nil {
		s.writeError(w, http.StatusNotImplemented, "Sulpher query engine not initialized")
		return
	}

	queryID := chi.URLParam(r, "query_id")
	if queryID == "" {
		s.writeError(w, http.StatusBadRequest, "query_id required")
		return
	}

	job, exists := s.sulpherJobs.GetJob(queryID)
	if !exists {
		s.writeError(w, http.StatusNotFound, "Query not found")
		return
	}

	if job.Status == "pending" || job.Status == "running" {
		s.writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"query_id": job.ID,
			"status":   job.Status,
			"message":  "Query is still processing",
		})
		return
	}

	if job.Status == "failed" {
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"query_id": job.ID,
			"status":   job.Status,
			"error":    job.Error,
		})
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"query_id": job.ID,
		"status":   job.Status,
		"result":   job.Result.Data,
		"stats": map[string]interface{}{
			"nodes_traversed":   job.Result.Stats.NodesTraversed,
			"paths_found":       job.Result.Stats.PathsFound,
			"execution_time_ms": job.Result.Stats.ExecutionTime.Milliseconds(),
		},
	})
}

// =============================================================================
// OQL (SQL) Query Handlers
// =============================================================================

// handleOQLQuery executes an OQL query synchronously
// POST /api/v1/oql/query
func (s *Server) handleOQLQuery(w http.ResponseWriter, r *http.Request) {
	if s.oqlJobs == nil {
		s.writeError(w, http.StatusNotImplemented, "OQL query engine not initialized")
		return
	}

	var req struct {
		Query string `json:"query"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if req.Query == "" {
		s.writeError(w, http.StatusBadRequest, "Query is required")
		return
	}

	result, err := s.oqlJobs.ExecuteSync(r.Context(), req.Query)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "completed",
		"data":   result.Rows,
		"stats": map[string]interface{}{
			"rows_scanned":      result.Stats.RowsScanned,
			"rows_returned":     result.Stats.RowsReturned,
			"rows_affected":     result.Stats.RowsAffected,
			"execution_time_ms": result.Stats.ExecutionTime.Milliseconds(),
		},
	})
}

// handleOQLQueryAsync submits an OQL query for async execution
// POST /api/v1/oql/query/async
func (s *Server) handleOQLQueryAsync(w http.ResponseWriter, r *http.Request) {
	if s.oqlJobs == nil {
		s.writeError(w, http.StatusNotImplemented, "OQL query engine not initialized")
		return
	}

	var req struct {
		Query string `json:"query"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if req.Query == "" {
		s.writeError(w, http.StatusBadRequest, "Query is required")
		return
	}

	queryID := s.oqlJobs.Submit(req.Query)

	s.writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"query_id": queryID,
		"status":   "pending",
	})
}

// handleOQLQueryStatus gets the status of an async OQL query
// GET /api/v1/oql/query/{query_id}
func (s *Server) handleOQLQueryStatus(w http.ResponseWriter, r *http.Request) {
	if s.oqlJobs == nil {
		s.writeError(w, http.StatusNotImplemented, "OQL query engine not initialized")
		return
	}

	queryID := chi.URLParam(r, "query_id")
	if queryID == "" {
		s.writeError(w, http.StatusBadRequest, "query_id required")
		return
	}

	job := s.oqlJobs.GetJob(queryID)
	if job == nil {
		s.writeError(w, http.StatusNotFound, "Query not found")
		return
	}

	response := map[string]interface{}{
		"query_id":   job.ID,
		"query":      job.Query,
		"status":     job.Status,
		"created_at": job.CreatedAt,
		"updated_at": job.UpdatedAt,
	}

	if job.Error != "" {
		response["error"] = job.Error
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleOQLQueryResult gets the result of a completed async OQL query
// GET /api/v1/oql/query/{query_id}/result
func (s *Server) handleOQLQueryResult(w http.ResponseWriter, r *http.Request) {
	if s.oqlJobs == nil {
		s.writeError(w, http.StatusNotImplemented, "OQL query engine not initialized")
		return
	}

	queryID := chi.URLParam(r, "query_id")
	if queryID == "" {
		s.writeError(w, http.StatusBadRequest, "query_id required")
		return
	}

	job := s.oqlJobs.GetJob(queryID)
	if job == nil {
		s.writeError(w, http.StatusNotFound, "Query not found")
		return
	}

	if job.Status == "pending" || job.Status == "running" {
		s.writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"query_id": job.ID,
			"status":   job.Status,
			"message":  "Query is still processing",
		})
		return
	}

	if job.Status == "failed" {
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"query_id": job.ID,
			"status":   job.Status,
			"error":    job.Error,
		})
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"query_id": job.ID,
		"status":   job.Status,
		"data":     job.Result.Rows,
		"stats": map[string]interface{}{
			"rows_scanned":      job.Result.Stats.RowsScanned,
			"rows_returned":     job.Result.Stats.RowsReturned,
			"rows_affected":     job.Result.Stats.RowsAffected,
			"execution_time_ms": job.Result.Stats.ExecutionTime.Milliseconds(),
		},
	})
}

// handleCreateSchema creates or updates a schema
func (s *Server) handleCreateSchema(w http.ResponseWriter, r *http.Request) {
	entity := chi.URLParam(r, "entity")

	if err := validateEntityName(entity); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var schema map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&schema); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if err := s.validator.LoadSchema(entity, schema); err != nil {
		s.logger.Error().Err(err).Msg("Failed to load schema")
		s.writeError(w, http.StatusInternalServerError, "Failed to load schema")
		return
	}

	s.logger.Info().Str("entity", entity).Msg("Created/updated schema")

	s.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"message": fmt.Sprintf("Schema for %s created/updated successfully", entity),
	})
}

// handleGetSchema retrieves a schema
func (s *Server) handleGetSchema(w http.ResponseWriter, r *http.Request) {
	entity := chi.URLParam(r, "entity")

	if err := validateEntityName(entity); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if !s.validator.HasSchema(entity) {
		s.writeError(w, http.StatusNotFound, fmt.Sprintf("No schema found for %s", entity))
		return
	}

	schema, err := s.validator.GetSchema(entity)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to retrieve schema")
		return
	}

	s.writeJSON(w, http.StatusOK, schema)
}

// Helper functions

func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, models.ErrorResponse{
		Error: struct {
			Message string `json:"message"`
			Status  int    `json:"status"`
		}{
			Message: message,
			Status:  status,
		},
	})
}

func (s *Server) invalidateCache(entity string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = s.cache.DeletePattern(ctx, entity)
}

// handleFullTextSearch performs full-text search across entities
func (s *Server) handleFullTextSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		s.writeError(w, http.StatusBadRequest, "Missing 'q' query parameter")
		return
	}

	entity := r.URL.Query().Get("entity")

	// Check if full-text search is enabled
	if !s.config.FullTextEnabled {
		s.writeError(w, http.StatusServiceUnavailable, "Full-text search is not enabled")
		return
	}

	results, err := s.storage.FullTextSearch(r.Context(), query, entity)
	if err != nil {
		s.logger.Error().Err(err).Str("query", query).Msg("Full-text search failed")
		s.writeError(w, http.StatusInternalServerError, "Search failed")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   query,
		"entity":  entity,
		"count":   len(results),
		"results": results,
	})
}

func (s *Server) embedReferences(ctx context.Context, data map[string]interface{}, depth int) map[string]interface{} {
	if depth <= 0 {
		return data
	}

	result := make(map[string]interface{})
	for k, v := range data {
		result[k] = s.embedValue(ctx, v, depth)
	}

	return result
}

// embedValue recursively embeds references in any value type
func (s *Server) embedValue(ctx context.Context, v interface{}, depth int) interface{} {
	if depth <= 0 {
		return v
	}

	// Check if it's a REF
	if ref, isRef := models.IsReference(v); isRef {
		if refData, err := s.storage.Get(ctx, ref.Entity, ref.ID); err == nil {
			return s.embedReferences(ctx, refData, depth-1)
		}
		return v
	}

	// Check if it's a map (nested object)
	if m, ok := v.(map[string]interface{}); ok {
		result := make(map[string]interface{})
		for mk, mv := range m {
			result[mk] = s.embedValue(ctx, mv, depth)
		}
		return result
	}

	// Check if it's an array
	if arr, ok := v.([]interface{}); ok {
		result := make([]interface{}, len(arr))
		for i, av := range arr {
			result[i] = s.embedValue(ctx, av, depth)
		}
		return result
	}

	// Scalar value, return as-is
	return v
}

func (s *Server) cascadeDelete(ctx context.Context, entity string, id int) ([]string, error) {
	// This is a simplified cascade delete
	// In production, you'd want more sophisticated logic

	deletedRefs := []string{}
	toCheck := []struct {
		entity string
		id     int
	}{{entity, id}}

	checked := make(map[string]bool)

	for len(toCheck) > 0 && len(deletedRefs) < s.config.MaxCascadeDeletions {
		current := toCheck[0]
		toCheck = toCheck[1:]

		key := fmt.Sprintf("%s:%d", current.entity, current.id)
		if checked[key] {
			continue
		}
		checked[key] = true
		deletedRefs = append(deletedRefs, key)

		// Find referencing entities
		// This would require scanning all entities - simplified here

		// Delete the entity
		if err := s.storage.Delete(ctx, current.entity, current.id); err != nil {
			s.logger.Error().Err(err).Str("entity", current.entity).Int("id", current.id).
				Msg("Failed to delete during cascade")
		}

		// Remove from graph
		if s.config.GraphEnabled {
			nodeID := fmt.Sprintf("%s:%d", current.entity, current.id)
			_ = s.graph.RemoveNode(nodeID)
		}
	}

	if s.config.GraphEnabled && s.persister != nil {
		s.persister.MarkDirty()
	}

	return deletedRefs, nil
}

func validateEntityName(entity string) error {
	if entity == "" {
		return fmt.Errorf("entity name cannot be empty")
	}

	matched, _ := regexp.MatchString(`^[a-zA-Z][a-zA-Z0-9_]*$`, entity)
	if !matched {
		return fmt.Errorf("invalid entity name: must start with a letter and contain only letters, numbers, and underscores")
	}

	return nil
}

// handleExport exports all data as a zip archive
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	// Generate timestamp for filename
	timestamp := time.Now().UTC().Format("2006-01-02T150405Z")
	filename := fmt.Sprintf("olu-export-%s.zip", timestamp)

	// Set headers for zip download
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	// Create zip writer directly to response
	zw := zip.NewWriter(w)
	defer zw.Close()

	// Create manifest
	manifest := map[string]interface{}{
		"version":      version.Version,
		"exported_at":  time.Now().UTC().Format(time.RFC3339),
		"storage_type": s.config.StorageType,
		"graph_enabled": s.config.GraphEnabled,
	}

	// Add SQLite database if using sqlite storage
	if s.config.StorageType == "sqlite" {
		dbPath := s.config.DBPath
		if err := s.addFileToZip(zw, dbPath, "entities.db"); err != nil {
			s.logger.Error().Err(err).Str("path", dbPath).Msg("Failed to add database to export")
			// Can't write error response since we've started writing the zip
			return
		}
		manifest["entities_file"] = "entities.db"
	} else {
		// For jsonfile storage, add the data directory
		dataDir := filepath.Join(s.config.BaseDir, s.config.Schema)
		if err := s.addDirToZip(zw, dataDir, "data"); err != nil {
			s.logger.Error().Err(err).Str("path", dataDir).Msg("Failed to add data directory to export")
			return
		}
		manifest["entities_dir"] = "data"
	}

	// Add graph files if enabled
	if s.config.GraphEnabled {
		graphFiles := []struct {
			src  string
			dest string
		}{
			{s.config.GraphDataFile, "graph.data"},
			{s.config.GraphIndexFile, "graph.index"},
		}

		for _, gf := range graphFiles {
			if _, err := os.Stat(gf.src); err == nil {
				if err := s.addFileToZip(zw, gf.src, gf.dest); err != nil {
					s.logger.Error().Err(err).Str("path", gf.src).Msg("Failed to add graph file to export")
				} else {
					if manifest["graph_files"] == nil {
						manifest["graph_files"] = []string{}
					}
					manifest["graph_files"] = append(manifest["graph_files"].([]string), gf.dest)
				}
			}
		}

		// Also export graph as JSON for easier analysis
		if s.graph != nil {
			if err := s.addGraphJSONToZip(zw); err != nil {
				s.logger.Error().Err(err).Msg("Failed to add graph JSON to export")
			} else {
				manifest["graph_json"] = "graph.json"
			}
		}
	}

	// Write manifest
	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	mw, err := zw.Create("manifest.json")
	if err == nil {
		_, _ = mw.Write(manifestBytes)
	}

	s.logger.Info().Str("filename", filename).Msg("Export completed")
}

// addFileToZip adds a file to the zip archive
func (s *Server) addFileToZip(zw *zip.Writer, srcPath, destName string) error {
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = destName
	header.Method = zip.Deflate

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, file)
	return err
}

// addDirToZip recursively adds a directory to the zip archive
func (s *Server) addDirToZip(zw *zip.Writer, srcDir, destDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden files and directories
		if strings.HasPrefix(info.Name(), ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(destDir, relPath)

		if info.IsDir() {
			// Create directory entry
			_, err := zw.Create(destPath + "/")
			return err
		}

		// Add file
		return s.addFileToZip(zw, path, destPath)
	})
}

// addGraphJSONToZip exports graph data as JSON
func (s *Server) addGraphJSONToZip(zw *zip.Writer) error {
	// Try to use IndexedGraph's Save method if available
	if ig, ok := s.graph.(*graph.IndexedGraph); ok {
		// Get graph statistics
		nodeCount := ig.NodeCount()
		edgeCount := ig.EdgeCount()
		allNodes := ig.GetAllNodes()

		// Build graph data structure
		graphData := map[string]interface{}{
			"exported_at": time.Now().UTC().Format(time.RFC3339),
			"stats": map[string]int{
				"nodes": nodeCount,
				"edges": edgeCount,
			},
			"nodes": []map[string]interface{}{},
			"edges": []map[string]interface{}{},
		}

		nodes := []map[string]interface{}{}
		edges := []map[string]interface{}{}

		// Export all nodes and their edges
		for _, nodeID := range allNodes {
			nodes = append(nodes, map[string]interface{}{
				"id": nodeID,
			})

			// Get outgoing edges (neighbors)
			neighbors, err := ig.GetNeighbors(nodeID)
			if err == nil {
				for target, rel := range neighbors {
					edges = append(edges, map[string]interface{}{
						"from":         nodeID,
						"to":           target,
						"relationship": rel,
					})
				}
			}
		}

		graphData["nodes"] = nodes
		graphData["edges"] = edges

		jsonBytes, err := json.MarshalIndent(graphData, "", "  ")
		if err != nil {
			return err
		}

		writer, err := zw.Create("graph.json")
		if err != nil {
			return err
		}

		_, err = writer.Write(jsonBytes)
		return err
	}

	// Fallback: minimal export for non-IndexedGraph implementations
	graphData := map[string]interface{}{
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"note":        "Graph export not available for this graph implementation",
	}

	jsonBytes, err := json.MarshalIndent(graphData, "", "  ")
	if err != nil {
		return err
	}

	writer, err := zw.Create("graph.json")
	if err != nil {
		return err
	}

	_, err = writer.Write(jsonBytes)
	return err
}
