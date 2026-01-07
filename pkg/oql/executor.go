package oql

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ha1tch/tsqlparser/ast"
	"github.com/ha1tch/olu/pkg/storage"
)

// Executor executes OQL queries against storage
type Executor struct {
	store           storage.Store
	aggregator      *Aggregator
	schemaValidator SchemaValidator
}

// NewExecutor creates a new executor
func NewExecutor(store storage.Store, sv SchemaValidator) *Executor {
	return &Executor{
		store:           store,
		aggregator:      NewAggregator(),
		schemaValidator: sv,
	}
}

// Execute executes a validated AST statement
func (e *Executor) Execute(ctx context.Context, stmt ast.Statement) (*Result, error) {
	switch s := stmt.(type) {
	case *ast.SelectStatement:
		return e.executeSelect(ctx, s)
	case *ast.InsertStatement:
		return e.executeInsert(ctx, s)
	case *ast.UpdateStatement:
		return e.executeUpdate(ctx, s)
	case *ast.DeleteStatement:
		return e.executeDelete(ctx, s)
	default:
		return nil, fmt.Errorf("unsupported statement: %T", stmt)
	}
}

func (e *Executor) executeSelect(ctx context.Context, s *ast.SelectStatement) (*Result, error) {
	startTime := time.Now()

	// Extract entity name
	entity := extractEntityFromSelect(s)

	// 1. Scan all records
	records, err := e.store.List(ctx, entity)
	if err != nil {
		return nil, fmt.Errorf("failed to read entity '%s': %w", entity, err)
	}
	scanned := len(records)

	// 2. Apply WHERE filter
	if s.Where != nil {
		records = e.filterRecords(records, s.Where)
	}

	// 3. GROUP BY + aggregates
	if len(s.GroupBy) > 0 || hasAggregates(s.Columns) {
		records = e.aggregator.Aggregate(records, s.Columns, s.GroupBy, s.Having)
	}

	// 4. ORDER BY
	if len(s.OrderBy) > 0 {
		records = OrderBy(records, s.OrderBy)
	}

	// 5. DISTINCT
	if s.Distinct {
		records = e.distinctRecords(records, s.Columns)
	}

	// 6. TOP (limit)
	if s.Top != nil {
		records = ApplyTop(records, s.Top)
	}

	// 7. Project columns
	rows := e.projectColumns(records, s.Columns)

	return NewSelectResult(rows, scanned, time.Since(startTime)), nil
}

func (e *Executor) executeInsert(ctx context.Context, s *ast.InsertStatement) (*Result, error) {
	startTime := time.Now()

	entity := normalizeEntityName(s.Table.String())
	inserted := 0

	// Extract column names
	var columns []string
	for _, col := range s.Columns {
		columns = append(columns, col.Value)
	}

	// Insert each row
	for i, row := range s.Values {
		record := make(map[string]interface{})

		if len(columns) > 0 {
			// Named columns
			for j, val := range row {
				if j < len(columns) {
					record[columns[j]] = evalLiteral(val)
				}
			}
		} else {
			// Positional - would need schema to know column names
			// For now, just number them
			for j, val := range row {
				record[fmt.Sprintf("col%d", j)] = evalLiteral(val)
			}
		}

		// Validate against schema if validator is configured
		if e.schemaValidator != nil {
			if valid, errors := e.schemaValidator.Validate(entity, record); !valid {
				return nil, fmt.Errorf("validation failed at row %d: %v", i+1, errors)
			}
		}

		if _, err := e.store.Create(ctx, entity, record); err != nil {
			return nil, fmt.Errorf("insert failed at row %d: %w", i+1, err)
		}
		inserted++
	}

	return NewMutationResult(ResultInsert, inserted, time.Since(startTime)), nil
}

func (e *Executor) executeUpdate(ctx context.Context, s *ast.UpdateStatement) (*Result, error) {
	startTime := time.Now()

	entity := normalizeEntityName(s.Table.String())

	// Find matching records
	records, err := e.store.List(ctx, entity)
	if err != nil {
		return nil, fmt.Errorf("failed to read entity '%s': %w", entity, err)
	}

	// Filter by WHERE
	records = e.filterRecords(records, s.Where)

	updated := 0
	for _, rec := range records {
		id := extractID(rec)
		if id == 0 {
			continue
		}

		// Apply SET clauses
		for _, set := range s.SetClauses {
			colName := extractSetColumn(set)
			rec[colName] = evalLiteralWithRecord(set.Value, rec)
		}

		// Validate against schema if validator is configured
		if e.schemaValidator != nil {
			if valid, errors := e.schemaValidator.Validate(entity, rec); !valid {
				return nil, fmt.Errorf("validation failed for id %d: %v", id, errors)
			}
		}

		if err := e.store.Update(ctx, entity, id, rec); err != nil {
			return nil, fmt.Errorf("update failed for id %d: %w", id, err)
		}
		updated++
	}

	return NewMutationResult(ResultUpdate, updated, time.Since(startTime)), nil
}

func (e *Executor) executeDelete(ctx context.Context, s *ast.DeleteStatement) (*Result, error) {
	startTime := time.Now()

	entity := normalizeEntityName(extractDeleteEntity(s))

	// Find matching records
	records, err := e.store.List(ctx, entity)
	if err != nil {
		return nil, fmt.Errorf("failed to read entity '%s': %w", entity, err)
	}

	// Filter by WHERE
	records = e.filterRecords(records, s.Where)

	deleted := 0
	for _, rec := range records {
		id := extractID(rec)
		if id == 0 {
			continue
		}

		if err := e.store.Delete(ctx, entity, id); err != nil {
			return nil, fmt.Errorf("delete failed for id %d: %w", id, err)
		}
		deleted++
	}

	return NewMutationResult(ResultDelete, deleted, time.Since(startTime)), nil
}

// filterRecords filters records by a WHERE expression
func (e *Executor) filterRecords(records []map[string]interface{}, where ast.Expression) []map[string]interface{} {
	var filtered []map[string]interface{}
	for _, rec := range records {
		if e.evalCondition(rec, where) {
			filtered = append(filtered, rec)
		}
	}
	return filtered
}

// evalCondition evaluates a WHERE condition
func (e *Executor) evalCondition(rec map[string]interface{}, expr ast.Expression) bool {
	switch ex := expr.(type) {
	case *ast.InfixExpression:
		switch ex.Operator {
		case "AND":
			return e.evalCondition(rec, ex.Left) && e.evalCondition(rec, ex.Right)
		case "OR":
			return e.evalCondition(rec, ex.Left) || e.evalCondition(rec, ex.Right)
		default:
			left := e.evalExpr(rec, ex.Left)
			right := e.evalExpr(rec, ex.Right)
			return evalComparison(left, ex.Operator, right)
		}

	case *ast.PrefixExpression:
		if ex.Operator == "NOT" {
			return !e.evalCondition(rec, ex.Right)
		}

	case *ast.IsNullExpression:
		val := e.evalExpr(rec, ex.Expr)
		isNull := val == nil
		return isNull != ex.Not

	case *ast.BetweenExpression:
		val := e.evalExpr(rec, ex.Expr)
		low := e.evalExpr(rec, ex.Low)
		high := e.evalExpr(rec, ex.High)
		inRange := compareValues(val, low) >= 0 && compareValues(val, high) <= 0
		return inRange != ex.Not

	case *ast.InExpression:
		val := e.evalExpr(rec, ex.Expr)
		found := false
		for _, item := range ex.Values {
			if compareValues(val, e.evalExpr(rec, item)) == 0 {
				found = true
				break
			}
		}
		return found != ex.Not

	case *ast.LikeExpression:
		val := fmt.Sprintf("%v", e.evalExpr(rec, ex.Expr))
		pattern := fmt.Sprintf("%v", e.evalExpr(rec, ex.Pattern))
		matches := matchLike(val, pattern)
		return matches != ex.Not
	}

	return true // Default to true for unsupported expressions
}

// evalExpr evaluates an expression to a value
func (e *Executor) evalExpr(rec map[string]interface{}, expr ast.Expression) interface{} {
	switch ex := expr.(type) {
	case *ast.Identifier:
		// Handle TRUE/FALSE as booleans
		upper := strings.ToUpper(ex.Value)
		if upper == "TRUE" {
			return true
		}
		if upper == "FALSE" {
			return false
		}
		return getFieldValue(rec, ex.Value)
	case *ast.QualifiedIdentifier:
		return getFieldValue(rec, ex.String())
	case *ast.IntegerLiteral:
		return ex.Value
	case *ast.FloatLiteral:
		return ex.Value
	case *ast.StringLiteral:
		return ex.Value
	case *ast.NullLiteral:
		return nil
	case *ast.InfixExpression:
		// Arithmetic
		left := e.evalExpr(rec, ex.Left)
		right := e.evalExpr(rec, ex.Right)
		return evalArithmetic(left, ex.Operator, right)
	}
	return nil
}

// distinctRecords removes duplicate rows
func (e *Executor) distinctRecords(records []map[string]interface{}, columns []ast.SelectColumn) []map[string]interface{} {
	seen := make(map[string]bool)
	var unique []map[string]interface{}

	for _, rec := range records {
		key := e.buildDistinctKey(rec, columns)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, rec)
		}
	}
	return unique
}

func (e *Executor) buildDistinctKey(rec map[string]interface{}, columns []ast.SelectColumn) string {
	var parts []string
	for _, col := range columns {
		val := e.evalExpr(rec, col.Expression)
		parts = append(parts, fmt.Sprintf("%v", val))
	}
	return strings.Join(parts, "|")
}

// projectColumns projects selected columns from records
func (e *Executor) projectColumns(records []map[string]interface{}, columns []ast.SelectColumn) []map[string]interface{} {
	// Check for SELECT *
	for _, col := range columns {
		if col.AllColumns {
			return records // Return all columns
		}
	}

	var projected []map[string]interface{}
	for _, rec := range records {
		row := make(map[string]interface{})
		for _, col := range columns {
			alias := columnAlias(col)

			// If already computed (e.g., aggregate), use directly
			if val, ok := rec[alias]; ok {
				row[alias] = val
				continue
			}

			// Otherwise evaluate expression
			row[alias] = e.evalExpr(rec, col.Expression)
		}
		projected = append(projected, row)
	}
	return projected
}

// Helper functions

func extractEntityFromSelect(s *ast.SelectStatement) string {
	if s.From == nil || len(s.From.Tables) == 0 {
		return ""
	}
	if tn, ok := s.From.Tables[0].(*ast.TableName); ok {
		return normalizeEntityName(tn.Name.String())
	}
	return ""
}

func extractID(rec map[string]interface{}) int {
	if id, ok := rec["id"].(float64); ok {
		return int(id)
	}
	if id, ok := rec["id"].(int); ok {
		return id
	}
	if id, ok := rec["id"].(int64); ok {
		return int(id)
	}
	return 0
}

func extractSetColumn(set *ast.SetClause) string {
	if set.Column != nil {
		return set.Column.String()
	}
	return ""
}

func evalLiteral(expr ast.Expression) interface{} {
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return e.Value
	case *ast.FloatLiteral:
		return e.Value
	case *ast.StringLiteral:
		return e.Value
	case *ast.Identifier:
		// Handle TRUE/FALSE as booleans
		upper := strings.ToUpper(e.Value)
		if upper == "TRUE" {
			return true
		}
		if upper == "FALSE" {
			return false
		}
		return e.Value
	case *ast.NullLiteral:
		return nil
	default:
		return expr.String()
	}
}

func evalLiteralWithRecord(expr ast.Expression, rec map[string]interface{}) interface{} {
	switch e := expr.(type) {
	case *ast.Identifier:
		return getFieldValue(rec, e.Value)
	case *ast.QualifiedIdentifier:
		return getFieldValue(rec, e.String())
	default:
		return evalLiteral(expr)
	}
}

func evalComparison(left interface{}, op string, right interface{}) bool {
	cmp := compareValues(left, right)
	switch op {
	case "=":
		return cmp == 0
	case "!=", "<>":
		return cmp != 0
	case "<":
		return cmp < 0
	case ">":
		return cmp > 0
	case "<=":
		return cmp <= 0
	case ">=":
		return cmp >= 0
	}
	return false
}

func evalArithmetic(left interface{}, op string, right interface{}) interface{} {
	l := toFloat(left)
	r := toFloat(right)
	switch op {
	case "+":
		return l + r
	case "-":
		return l - r
	case "*":
		return l * r
	case "/":
		if r == 0 {
			return nil
		}
		return l / r
	case "%":
		if r == 0 {
			return nil
		}
		return float64(int(l) % int(r))
	}
	return nil
}

// matchLike implements SQL LIKE pattern matching
// % = any characters, _ = single character
func matchLike(value, pattern string) bool {
	// Convert SQL LIKE pattern to simple matching
	// This is a simplified implementation
	pattern = strings.ToLower(pattern)
	value = strings.ToLower(value)

	// Handle % wildcards
	if strings.HasPrefix(pattern, "%") && strings.HasSuffix(pattern, "%") {
		return strings.Contains(value, pattern[1:len(pattern)-1])
	}
	if strings.HasPrefix(pattern, "%") {
		return strings.HasSuffix(value, pattern[1:])
	}
	if strings.HasSuffix(pattern, "%") {
		return strings.HasPrefix(value, pattern[:len(pattern)-1])
	}

	// Exact match (with _ handling could be added)
	return value == pattern
}
