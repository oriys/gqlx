package gqlx

import "fmt"

// GraphQLError represents an error in a GraphQL response.
type GraphQLError struct {
	Message   string                 `json:"message"`
	Locations []ErrorLocation        `json:"locations,omitempty"`
	Path      []interface{}          `json:"path,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

func (e *GraphQLError) Error() string {
	return e.Message
}

// ErrorLocation represents a location in a source document where an error occurred.
type ErrorLocation struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// NewGraphQLError creates a new GraphQL error.
func NewGraphQLError(message string, locations []ErrorLocation, path []interface{}) *GraphQLError {
	return &GraphQLError{
		Message:   message,
		Locations: locations,
		Path:      path,
	}
}

// NewValidationError creates a new validation error.
func NewValidationError(message string, loc Location) *GraphQLError {
	return &GraphQLError{
		Message:   message,
		Locations: []ErrorLocation{{Line: loc.Line, Column: loc.Col}},
	}
}

// Result represents the result of a GraphQL execution.
type Result struct {
	Data   interface{}     `json:"data,omitempty"`
	Errors []*GraphQLError `json:"errors,omitempty"`
}

// HasErrors returns true if the result contains errors.
func (r *Result) HasErrors() bool {
	return len(r.Errors) > 0
}

// FormatError formats an error as a GraphQL error.
func FormatError(err error) *GraphQLError {
	if gqlErr, ok := err.(*GraphQLError); ok {
		return gqlErr
	}
	return &GraphQLError{Message: err.Error()}
}

// FormatErrors formats multiple errors as a string.
func FormatErrors(errs []*GraphQLError) string {
	if len(errs) == 0 {
		return ""
	}
	s := ""
	for i, err := range errs {
		if i > 0 {
			s += "\n"
		}
		s += fmt.Sprintf("- %s", err.Message)
	}
	return s
}
