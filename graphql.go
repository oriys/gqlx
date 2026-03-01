package gqlx

// Execute is the top-level function to execute a GraphQL query.
func Execute(params ExecuteParams) *Result {
	schema := params.Schema
	if schema == nil {
		return &Result{Errors: []*GraphQLError{{Message: "Must provide schema"}}}
	}

	executor := NewExecutor(schema)
	return executor.Execute(params)
}

// Do is a convenience function that parses, validates, and executes a GraphQL query.
func Do(schema *Schema, query string, variables map[string]interface{}, operationName string) *Result {
	if schema == nil {
		return &Result{Errors: []*GraphQLError{{Message: "Must provide schema"}}}
	}

	doc, err := Parse(query)
	if err != nil {
		return &Result{Errors: []*GraphQLError{FormatError(err)}}
	}

	validationErrors := Validate(schema, doc)
	if len(validationErrors) > 0 {
		return &Result{Errors: validationErrors}
	}

	return Execute(ExecuteParams{
		Schema:        schema,
		Document:      doc,
		Variables:     variables,
		OperationName: operationName,
	})
}
