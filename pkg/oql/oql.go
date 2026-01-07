// Package oql provides SQL-compatible query language for olu.
//
// OQL supports a subset of T-SQL syntax for querying and mutating data:
//
//   - SELECT with aggregates (COUNT, SUM, AVG, MIN, MAX)
//   - GROUP BY, HAVING, ORDER BY, TOP
//   - INSERT with VALUES
//   - UPDATE with WHERE (required)
//   - DELETE with WHERE (required)
//
// JOINs are not supported as relationships are handled by the graph layer.
package oql

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ha1tch/tsqlparser"
	"github.com/ha1tch/tsqlparser/ast"
	"github.com/ha1tch/olu/pkg/storage"
)

// SchemaValidator validates entity data against schemas
type SchemaValidator interface {
	Validate(entity string, data map[string]interface{}) (bool, []string)
}

// Engine is the main OQL query engine
type Engine struct {
	store           storage.Store
	validator       *Validator
	executor        *Executor
	schemaValidator SchemaValidator
	mu              sync.RWMutex
}

// NewEngine creates a new OQL engine
func NewEngine(store storage.Store, schemaDir string) *Engine {
	return &Engine{
		store:     store,
		validator: NewValidator(schemaDir),
		executor:  NewExecutor(store, nil),
	}
}

// NewEngineWithSchemaValidator creates an OQL engine with schema validation
func NewEngineWithSchemaValidator(store storage.Store, schemaDir string, sv SchemaValidator) *Engine {
	return &Engine{
		store:           store,
		validator:       NewValidator(schemaDir),
		executor:        NewExecutor(store, sv),
		schemaValidator: sv,
	}
}

// Execute parses, validates, and executes an OQL query
func (e *Engine) Execute(ctx context.Context, sql string) (*Result, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. Parse
	stmt, err := e.parse(sql)
	if err != nil {
		return nil, err
	}

	// 2. Validate
	if err := e.validator.Validate(stmt); err != nil {
		return nil, err
	}

	// 3. Execute
	return e.executor.Execute(ctx, stmt)
}

// parse parses a SQL string into an AST statement
func (e *Engine) parse(sql string) (ast.Statement, error) {
	program, errs := tsqlparser.Parse(sql)
	if len(errs) > 0 {
		return nil, fmt.Errorf("parse error: %v", errs[0])
	}

	if program == nil || len(program.Statements) == 0 {
		return nil, fmt.Errorf("empty query")
	}

	if len(program.Statements) > 1 {
		return nil, fmt.Errorf("only single statements are supported")
	}

	return program.Statements[0], nil
}

// RefreshSchema reloads the entity list from disk
func (e *Engine) RefreshSchema() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.validator.RefreshEntities()
}

// Job represents an async OQL query job
type Job struct {
	ID        string
	Query     string
	Status    JobStatus
	Result    *Result
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// JobStatus represents the status of a job
type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
)

// JobManager manages async OQL query jobs
type JobManager struct {
	engine  *Engine
	jobs    map[string]*Job
	mu      sync.RWMutex
	ttl     time.Duration
	closeCh chan struct{}
}

// NewJobManager creates a new job manager
func NewJobManager(engine *Engine, ttl time.Duration) *JobManager {
	jm := &JobManager{
		engine:  engine,
		jobs:    make(map[string]*Job),
		ttl:     ttl,
		closeCh: make(chan struct{}),
	}
	go jm.cleanupLoop()
	return jm
}

// Submit submits a query for async execution
func (jm *JobManager) Submit(query string) string {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	id := generateJobID()
	job := &Job{
		ID:        id,
		Query:     query,
		Status:    JobPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	jm.jobs[id] = job

	// Execute in background
	go jm.executeJob(job)

	return id
}

// ExecuteSync executes a query synchronously
func (jm *JobManager) ExecuteSync(ctx context.Context, query string) (*Result, error) {
	return jm.engine.Execute(ctx, query)
}

// GetJob returns a job by ID
func (jm *JobManager) GetJob(id string) *Job {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	return jm.jobs[id]
}

// GetJobResult returns the result of a completed job
func (jm *JobManager) GetJobResult(id string) (*Result, error) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

	job, exists := jm.jobs[id]
	if !exists {
		return nil, fmt.Errorf("job not found: %s", id)
	}

	if job.Status == JobPending || job.Status == JobRunning {
		return nil, fmt.Errorf("job not completed")
	}

	if job.Status == JobFailed {
		return nil, fmt.Errorf("job failed: %s", job.Error)
	}

	return job.Result, nil
}

func (jm *JobManager) executeJob(job *Job) {
	jm.mu.Lock()
	job.Status = JobRunning
	job.UpdatedAt = time.Now()
	jm.mu.Unlock()

	ctx := context.Background()
	result, err := jm.engine.Execute(ctx, job.Query)

	jm.mu.Lock()
	defer jm.mu.Unlock()

	if err != nil {
		job.Status = JobFailed
		job.Error = err.Error()
	} else {
		job.Status = JobCompleted
		job.Result = result
	}
	job.UpdatedAt = time.Now()
}

func (jm *JobManager) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			jm.cleanup()
		case <-jm.closeCh:
			return
		}
	}
}

func (jm *JobManager) cleanup() {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	cutoff := time.Now().Add(-jm.ttl)
	for id, job := range jm.jobs {
		if job.UpdatedAt.Before(cutoff) {
			delete(jm.jobs, id)
		}
	}
}

// Close stops the job manager
func (jm *JobManager) Close() {
	close(jm.closeCh)
}

// generateJobID creates a unique job ID
func generateJobID() string {
	return fmt.Sprintf("oql_%d", time.Now().UnixNano())
}
