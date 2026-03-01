package gqlx

import (
	"testing"
)

func TestGraphQLError(t *testing.T) {
	err := NewGraphQLError("test error", []ErrorLocation{{Line: 1, Column: 2}}, []interface{}{"field"})
	if err.Error() != "test error" {
		t.Errorf("Error() = %q", err.Error())
	}
	if len(err.Locations) != 1 || err.Locations[0].Line != 1 {
		t.Error("location mismatch")
	}
	if len(err.Path) != 1 || err.Path[0] != "field" {
		t.Error("path mismatch")
	}
}

func TestValidationError(t *testing.T) {
	err := NewValidationError("validation failed", Location{Line: 3, Col: 5})
	if err.Message != "validation failed" {
		t.Errorf("message = %q", err.Message)
	}
	if err.Locations[0].Line != 3 || err.Locations[0].Column != 5 {
		t.Error("location mismatch")
	}
}

func TestFormatError(t *testing.T) {
	gqlErr := &GraphQLError{Message: "gql error"}
	if FormatError(gqlErr) != gqlErr {
		t.Error("FormatError should return same GraphQLError")
	}

	stdErr := FormatError(gqlErr)
	if stdErr.Message != "gql error" {
		t.Errorf("message = %q", stdErr.Message)
	}
}

func TestResultHasErrors(t *testing.T) {
	r := &Result{}
	if r.HasErrors() {
		t.Error("empty result should not have errors")
	}
	r.Errors = []*GraphQLError{{Message: "err"}}
	if !r.HasErrors() {
		t.Error("result with errors should have errors")
	}
}

func TestFormatErrors(t *testing.T) {
	if FormatErrors(nil) != "" {
		t.Error("nil errors should return empty string")
	}
	errs := []*GraphQLError{
		{Message: "error 1"},
		{Message: "error 2"},
	}
	s := FormatErrors(errs)
	if s != "- error 1\n- error 2" {
		t.Errorf("FormatErrors = %q", s)
	}
}
