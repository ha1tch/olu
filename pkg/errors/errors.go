// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package errors defines structured API error codes for olu.
//
// Every error returned to HTTP clients includes a stable, machine-readable
// code in the format OLU-SSNNN where SS is a two-letter category and NNN
// is a three-digit sequence number. Client code should switch on the code
// field, not the human-readable message.
package errors

import "fmt"

// Code is a machine-readable error identifier.
type Code string

// APIError pairs a stable code with a human-readable message and an
// HTTP status hint. The status is a suggestion — handlers may override
// it if context requires a different HTTP status.
type APIError struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"status"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// New creates an APIError with a formatted message.
func New(code Code, status int, format string, args ...interface{}) *APIError {
	return &APIError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Status:  status,
	}
}

// Wrap creates an APIError wrapping an underlying error's message.
func Wrap(code Code, status int, err error) *APIError {
	return &APIError{
		Code:    code,
		Message: err.Error(),
		Status:  status,
	}
}

// ---------------------------------------------------------------------------
// Storage errors (ST)
// ---------------------------------------------------------------------------

const (
	ErrEntityNotFound   Code = "OLU-ST001"
	ErrEntityExists     Code = "OLU-ST002"
	ErrInvalidEntity    Code = "OLU-ST003"
	ErrInvalidID        Code = "OLU-ST004"
	ErrVersionConflict  Code = "OLU-ST005"
	ErrStorageFailed    Code = "OLU-ST006"
	ErrEntityTooLarge   Code = "OLU-ST007"
	ErrSchemaNotFound   Code = "OLU-ST008"
	ErrSchemaLoadFailed Code = "OLU-ST009"
)

// ---------------------------------------------------------------------------
// Graph errors (GR)
// ---------------------------------------------------------------------------

const (
	ErrCycleDetected    Code = "OLU-GR001"
	ErrGraphDisabled      Code = "OLU-GR002"
	ErrGraphUnsupported   Code = "OLU-GR003"
	ErrGraphFailed        Code = "OLU-GR004"
	ErrGraphVisitedLimit  Code = "OLU-GR005"
	ErrGraphResultLimit   Code = "OLU-GR006"
)

// ---------------------------------------------------------------------------
// Query errors — OQL and Sulpher (QL)
// ---------------------------------------------------------------------------

const (
	ErrQuerySyntax       Code = "OLU-QL001"
	ErrQueryDepthExceeded Code = "OLU-QL002"
	ErrQueryNotFound     Code = "OLU-QL003"
	ErrQueryFailed       Code = "OLU-QL004"
	ErrQueryRequired     Code = "OLU-QL005"
	ErrQueryIDRequired   Code = "OLU-QL006"
	ErrQueryEngineNotInit Code = "OLU-QL007"
	ErrQueryTimeout      Code = "OLU-QL008"
	ErrQueryRowLimit     Code = "OLU-QL009"
	ErrQueryScanLimit    Code = "OLU-QL010"
	ErrQueryResponseSize Code = "OLU-QL011"
	ErrSearchFailed      Code = "OLU-QL012"
	ErrSearchDisabled    Code = "OLU-QL013"
)

// ---------------------------------------------------------------------------
// Validation errors (VL)
// ---------------------------------------------------------------------------

const (
	ErrValidationFailed Code = "OLU-VL001"
	ErrInvalidJSON      Code = "OLU-VL002"
	ErrMissingParam     Code = "OLU-VL003"
)

// ---------------------------------------------------------------------------
// Authentication and authorization errors (AU)
// ---------------------------------------------------------------------------

const (
	ErrUnauthorized   Code = "OLU-AU001"
	ErrInvalidToken   Code = "OLU-AU002"
	ErrForbidden      Code = "OLU-AU003"
)

// ---------------------------------------------------------------------------
// Rate limiting errors (RL)
// ---------------------------------------------------------------------------

const (
	ErrRateLimited Code = "OLU-RL001"
)

// ---------------------------------------------------------------------------
// Tenant errors (TN)
// ---------------------------------------------------------------------------

const (
	ErrTenantNotFound Code = "OLU-TN001"
	ErrTenantRequired Code = "OLU-TN002"
)

// ---------------------------------------------------------------------------
// Configuration errors (CF)
// ---------------------------------------------------------------------------

const (
	ErrConfigInvalid Code = "OLU-CF001"
)

// ---------------------------------------------------------------------------
// Timeseries errors (TS)
// ---------------------------------------------------------------------------

const (
	ErrTSNotAvailable     Code = "OLU-TS001" // wrong tenant mode
	ErrTSNotEnabled       Code = "OLU-TS002" // feature flag off
	ErrTSNotProvisioned   Code = "OLU-TS003" // tenant not provisioned for TS
	ErrTSInvalidTrigger   Code = "OLU-TS004"
	ErrTSInvalidTimestamp Code = "OLU-TS005"
	ErrTSBatchTooLarge    Code = "OLU-TS006"
	ErrTSMissingField     Code = "OLU-TS007"
	ErrTSInvalidAggFunc   Code = "OLU-TS008"
	ErrTSInvalidAggField  Code = "OLU-TS009"
	ErrTSInvalidInterval  Code = "OLU-TS010"
	ErrTSRangeTooWide     Code = "OLU-TS011"
	ErrTSLimitExceeded    Code = "OLU-TS012"
	ErrTSInternal         Code = "OLU-TS013"
	ErrTSRetentionFailed  Code = "OLU-TS014"
	ErrTSProvisionFailed  Code = "OLU-TS015"
)
