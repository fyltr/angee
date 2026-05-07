// Package service implements the core business logic for the angee operator.
// Both HTTP handlers and MCP tools delegate to methods on Platform.
package service

import "errors"

// ServiceError carries an HTTP-appropriate status code from the business logic layer.
type ServiceError struct {
	Status  int
	Message string
}

func (e *ServiceError) Error() string { return e.Message }

// NotFound returns a 404 error.
func NotFound(msg string) error { return &ServiceError{404, msg} }

// BadRequest returns a 400 error.
func BadRequest(msg string) error { return &ServiceError{400, msg} }

// Conflict returns a 409 error.
func Conflict(msg string) error { return &ServiceError{409, msg} }

// ErrorStatus extracts the HTTP status code from an error.
// ServiceError returns its Status; all other errors default to 500.
func ErrorStatus(err error) int {
	var se *ServiceError
	if errors.As(err, &se) {
		return se.Status
	}
	return 500
}
