package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ha1tch/olu/pkg/graph"
	"github.com/ha1tch/olu/pkg/models"
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
	
	// Handle null behavior
	updatedFields := []string{}
	for key, value := range patchData {
		if key != "id" {
			if value == nil && s.config.PatchNullBehavior == "delete" {
				delete(existing, key)
			} else {
				existing[key] = value
			}
			updatedFields = append(updatedFields, key)
		}
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
		_ = s.graph.Save(s.config.GraphDataFile)
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
			_ = s.graph.Save(s.config.GraphDataFile)
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
		_ = s.graph.Save(s.config.GraphDataFile)
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
	json.NewEncoder(w).Encode(data)
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
	
	s.cache.DeletePattern(ctx, entity)
}

func (s *Server) embedReferences(ctx context.Context, data map[string]interface{}, depth int) map[string]interface{} {
	if depth <= 0 {
		return data
	}
	
	result := make(map[string]interface{})
	for k, v := range data {
		if ref, isRef := models.IsReference(v); isRef {
			// Fetch the referenced entity
			if refData, err := s.storage.Get(ctx, ref.Entity, ref.ID); err == nil {
				// Recursively embed
				result[k] = s.embedReferences(ctx, refData, depth-1)
			} else {
				result[k] = v
			}
		} else {
			result[k] = v
		}
	}
	
	return result
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
			s.graph.RemoveNode(nodeID)
		}
	}
	
	if s.config.GraphEnabled {
		s.graph.Save(s.config.GraphDataFile)
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
