package errors

import (
	stdErrors "errors"
	"fmt"
)

var ErrNotImplemented = stdErrors.New("not implemented")

type NotFoundError struct {
	Resource string
	Name     string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s '%s' not found", e.Resource, e.Name)
}

type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("validation error: %s", e.Msg)
	}
	return fmt.Sprintf("validation error: field %s %s", e.Field, e.Msg)
}

type OperationError struct {
	Op  string
	Err error
}

func (e *OperationError) Error() string {
	if e.Err == nil {
		return e.Op
	}
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *OperationError) Unwrap() error {
	return e.Err
}
