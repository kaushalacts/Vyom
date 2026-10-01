package models

import (
	"errors"
	"fmt"
	"strings"
)

// ValidationError marks a caller/config mistake that cannot be retried.
//
// It lives in pkg/models so that both the configuration loader and the API
// layer can translate it into a 400 response without importing internal
// packages.
type ValidationError struct {
	// Field is the dotted path of the offending value, e.g. "policy.images.quality".
	Field string
	// Detail explains what is wrong in human terms.
	Detail string
	// Value is the rejected value, redacted by the caller when sensitive.
	Value string
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.Detail
	}
	if e.Value == "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Detail)
	}
	return fmt.Sprintf("%s: %s (got %q)", e.Field, e.Detail, e.Value)
}

func errValidation(format string, args ...any) error {
	return &ValidationError{Field: "value", Detail: fmt.Sprintf(format, args...)}
}

// NewValidationError builds a ValidationError for the given field.
func NewValidationError(field, detail string, args ...any) error {
	if len(args) > 0 {
		detail = fmt.Sprintf(detail, args...)
	}
	return &ValidationError{Field: field, Detail: detail}
}

// ValidationErrors aggregates multiple configuration problems so that a
// single `vyom doctor` run can report all of them instead of one per restart.
type ValidationErrors struct {
	Errors []*ValidationError
}

// Add appends an error.
func (v *ValidationErrors) Add(err error) {
	if err == nil {
		return
	}
	var ve *ValidationError
	if errors.As(err, &ve) {
		v.Errors = append(v.Errors, ve)
		return
	}
	v.Errors = append(v.Errors, &ValidationError{Detail: err.Error()})
}

// Addf appends a formatted validation error for a field.
func (v *ValidationErrors) Addf(field, format string, args ...any) {
	v.Errors = append(v.Errors, &ValidationError{Field: field, Detail: fmt.Sprintf(format, args...)})
}

// Empty reports whether no errors were collected.
func (v *ValidationErrors) Empty() bool { return v == nil || len(v.Errors) == 0 }

// Err returns nil when empty, a single error, or the aggregate itself.
//
// The aggregate implements Unwrap() []error (Go 1.20 multi-error unwrapping),
// so errors.As(err, &validationError) still resolves to the first field-level
// problem when several were collected.
func (v *ValidationErrors) Err() error {
	switch {
	case v.Empty():
		return nil
	case len(v.Errors) == 1:
		return v.Errors[0]
	default:
		return v
	}
}

// Unwrap exposes every collected error to errors.Is/errors.As.
func (v *ValidationErrors) Unwrap() []error {
	if v == nil || len(v.Errors) == 0 {
		return nil
	}
	out := make([]error, 0, len(v.Errors))
	for _, e := range v.Errors {
		out = append(out, e)
	}
	return out
}

// Error implements error.
func (v *ValidationErrors) Error() string {
	if v.Empty() {
		return "no validation errors"
	}
	msgs := make([]string, 0, len(v.Errors))
	for _, e := range v.Errors {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "; ")
}

// IsValidationError reports whether err is (or wraps) a ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
