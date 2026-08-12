package ecp

import (
	"errors"
	"fmt"
)

type ErrorKind string

const (
	KindUsage     ErrorKind = "USAGE"
	KindBlocked   ErrorKind = "BLOCKED"
	KindIntegrity ErrorKind = "INTEGRITY"
	KindRuntime   ErrorKind = "RUNTIME"
	KindNotFound  ErrorKind = "NOT_FOUND"
	KindConflict  ErrorKind = "CONFLICT"
)

type ECPError struct {
	Kind    ErrorKind
	Code    string
	Message string
	Cause   error
}

func (e *ECPError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *ECPError) Unwrap() error { return e.Cause }

func newError(kind ErrorKind, code, message string, cause error) error {
	return &ECPError{Kind: kind, Code: code, Message: message, Cause: cause}
}

func isErrorCodeValue(err error, code string) bool {
	var typed *ECPError
	return errors.As(err, &typed) && typed.Code == code
}
