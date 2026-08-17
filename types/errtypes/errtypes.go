// Package errtypes contains custom error types
package errtypes

import (
	"fmt"
	"strings"
)

const (
	UnknownLoomKeyErrMsg = "unknown loom key"
	InvalidModelNameErrMsg = "invalid model name"
)

// TODO: This should have a structured response from the API
type UnknownLoomKey struct {
	Key string
}

func (e *UnknownLoomKey) Error() string {
	return fmt.Sprintf("unauthorized: %s %q", UnknownLoomKeyErrMsg, strings.TrimSpace(e.Key))
}
