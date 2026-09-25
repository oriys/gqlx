package gqlx

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
)

// errNonNullViolation signals that a non-null field completed to null. It
// propagates up to the nearest nullable parent, which itself resolves to null,
// exactly as required by the GraphQL specification.
var errNonNullViolation = errors.New("non-null field resolved to null")

// Executor executes GraphQL operations.
type Executor struct {
	schema *Schema
}

// NewExecutor creates a new executor for the given schema.
func NewExecutor(schema *Schema) *Executor {
	return &Executor{schema: schema}
}

// ExecuteParams holds the parameters for executing a GraphQL operation.
type ExecuteParams struct {
	Schema        *Schema
	Document      *Document
	RootValue     interface{}
	Variables     map[string]interface{}
	OperationName string
	MaxDepth      int  // 0 means no limit
	Concurrent    bool // resolve sibling query fields concurrently
}

// Execute executes a GraphQL document.
func (e *Executor) Execute(params ExecuteParams) *Result {
	doc := params.Document
	schema := params.Schema
	if schema == nil {
		schema = e.schema
	}
	if schema == nil {
		return &Result{Errors: []*GraphQLError{{Message: "Must provide schema"}}}
	}
	if doc == nil {
		return &Result{Errors: []*GraphQLError{{Message: "Must provide document"}}}
	}

	operation, fragments, err := findOperation(doc, params.OperationName)
	if err != nil {
		return &Result{Errors: []*GraphQLError{err}}
	}

	coercedVars, varErrors := coerceOperationVariables(schema, operation, params.Variables)
	if len(varErrors) > 0 {
		return &Result{Errors: varErrors}
	}

	// Get root type
	var rootType *ObjectType
	switch operation.Operation {
	case OperationQuery:
		rootType = schema.QueryType
	case OperationMutation:
		rootType = schema.MutationType
	case OperationSubscription:
		rootType = schema.SubscriptionType
	}

	if rootType == nil {
		return &Result{Errors: []*GraphQLError{{Message: fmt.Sprintf("Schema is not configured for %ss.", operation.Operation)}}}
	}

	ctx := newExecutionContext(schema, operation, fragments, coercedVars, params.MaxDepth)
	ctx.concurrent = params.Concurrent

	var data interface{}
	var execErr error
	if operation.Operation == OperationMutation {
		// Mutations must be executed serially per the GraphQL specification.
		data, execErr = ctx.executeFieldsSerially(rootType, params.RootValue, operation.SelectionSet, []interface{}{})
	} else {
		data, execErr = ctx.executeFields(rootType, params.RootValue, operation.SelectionSet, []interface{}{})
	}
	// A non-null root field that resolved to null turns the whole data entry null.
	if execErr != nil {
		data = nil
	}

	return &Result{Data: data, Errors: ctx.Errors()}
}

// findOperation locates the operation to execute and collects fragment definitions.
func findOperation(doc *Document, operationName string) (*OperationDefinition, map[string]*FragmentDefinition, *GraphQLError) {
	var operation *OperationDefinition
	fragments := make(map[string]*FragmentDefinition)

	for _, def := range doc.Definitions {
		switch d := def.(type) {
		case *OperationDefinition:
			if operationName == "" {
				if operation != nil {
					return nil, nil, &GraphQLError{Message: "Must provide operation name if query contains multiple operations."}
				}
				operation = d
			} else if d.Name == operationName {
				operation = d
			}
		case *FragmentDefinition:
			fragments[d.Name] = d
		}
	}

	if operation == nil {
		if operationName != "" {
			return nil, nil, &GraphQLError{Message: fmt.Sprintf("Unknown operation named \"%s\".", operationName)}
		}
		return nil, nil, &GraphQLError{Message: "Must provide an operation."}
	}
	return operation, fragments, nil
}

// coerceOperationVariables applies variable default values and coercion.
func coerceOperationVariables(schema *Schema, operation *OperationDefinition, variables map[string]interface{}) (map[string]interface{}, []*GraphQLError) {
	if variables == nil {
		variables = make(map[string]interface{})
	}
	return CoerceVariableValues(schema, operation.VariableDefinitions, variables)
}

type executionContext struct {
	schema     *Schema
	fragments  map[string]*FragmentDefinition
	variables  map[string]interface{}
	errors     []*GraphQLError
	operation  *OperationDefinition
	maxDepth   int
	concurrent bool
	mu         sync.Mutex
}

func newExecutionContext(schema *Schema, operation *OperationDefinition, fragments map[string]*FragmentDefinition, variables map[string]interface{}, maxDepth int) *executionContext {
	return &executionContext{
		schema:    schema,
		operation: operation,
		fragments: fragments,
		variables: variables,
		maxDepth:  maxDepth,
	}
}

// Errors returns a snapshot of the accumulated execution errors.
func (ctx *executionContext) Errors() []*GraphQLError {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.errors
}

func (ctx *executionContext) executeFields(parentType *ObjectType, source interface{}, selections []Selection, path []interface{}) (map[string]interface{}, error) {
	groupedFields := ctx.collectFields(parentType, selections, nil)
	if ctx.concurrent {
		return ctx.executeFieldsConcurrently(parentType, source, groupedFields, path)
	}
	return ctx.executeGroupedFields(parentType, source, groupedFields, path)
}

func (ctx *executionContext) executeFieldsSerially(parentType *ObjectType, source interface{}, selections []Selection, path []interface{}) (map[string]interface{}, error) {
	groupedFields := ctx.collectFields(parentType, selections, nil)
	return ctx.executeGroupedFields(parentType, source, groupedFields, path)
}

// executeGroupedFields resolves each field in order. If any non-null field
// resolves to null, the enclosing object itself resolves to null.
func (ctx *executionContext) executeGroupedFields(parentType *ObjectType, source interface{}, groupedFields []fieldEntry, path []interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(groupedFields))
	nonNullViolated := false

	for _, entry := range groupedFields {
		fieldPath := append(append([]interface{}{}, path...), entry.key)
		value, err := ctx.resolveField(parentType, source, entry.fields, fieldPath)
		if err != nil {
			nonNullViolated = true
			continue
		}
		result[entry.key] = value
	}

	if nonNullViolated {
		return nil, errNonNullViolation
	}
	return result, nil
}

// executeFieldsConcurrently resolves sibling fields in parallel. It is opt-in
// via ExecuteParams.Concurrent and is not used for mutations.
func (ctx *executionContext) executeFieldsConcurrently(parentType *ObjectType, source interface{}, groupedFields []fieldEntry, path []interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(groupedFields))
	values := make([]interface{}, len(groupedFields))
	failed := make([]bool, len(groupedFields))

	var wg sync.WaitGroup
	for i, entry := range groupedFields {
		wg.Add(1)
		go func(i int, entry fieldEntry) {
			defer wg.Done()
			fieldPath := append(append([]interface{}{}, path...), entry.key)
			value, err := ctx.resolveField(parentType, source, entry.fields, fieldPath)
			if err != nil {
				failed[i] = true
				return
			}
			values[i] = value
		}(i, entry)
	}
	wg.Wait()

	nonNullViolated := false
	for i, entry := range groupedFields {
		if failed[i] {
			nonNullViolated = true
			continue
		}
		result[entry.key] = values[i]
	}

	if nonNullViolated {
		return nil, errNonNullViolation
	}
	return result, nil
}

type fieldEntry struct {
	key    string
	fields []*Field
}

func (ctx *executionContext) collectFields(parentType *ObjectType, selections []Selection, visitedFragments map[string]bool) []fieldEntry {
	if visitedFragments == nil {
		visitedFragments = make(map[string]bool)
	}

	var entries []fieldEntry
	entryMap := make(map[string]int) // responseName -> index in entries

	for _, sel := range selections {
		// Check directives
		if ctx.shouldSkip(sel) {
			continue
		}

		switch s := sel.(type) {
		case *Field:
			responseName := s.Name
			if s.Alias != "" {
				responseName = s.Alias
			}
			if idx, ok := entryMap[responseName]; ok {
				entries[idx].fields = append(entries[idx].fields, s)
			} else {
				entryMap[responseName] = len(entries)
				entries = append(entries, fieldEntry{key: responseName, fields: []*Field{s}})
			}

		case *FragmentSpread:
			if visitedFragments[s.Name] {
				continue
			}
			visitedFragments[s.Name] = true
			frag, ok := ctx.fragments[s.Name]
			if !ok {
				continue
			}
			if !ctx.doesFragmentApply(frag.TypeCondition, parentType) {
				continue
			}
			fragEntries := ctx.collectFields(parentType, frag.SelectionSet, visitedFragments)
			for _, entry := range fragEntries {
				if idx, ok := entryMap[entry.key]; ok {
					entries[idx].fields = append(entries[idx].fields, entry.fields...)
				} else {
					entryMap[entry.key] = len(entries)
					entries = append(entries, entry)
				}
			}

		case *InlineFragment:
			if s.TypeCondition != "" && !ctx.doesFragmentApply(s.TypeCondition, parentType) {
				continue
			}
			fragEntries := ctx.collectFields(parentType, s.SelectionSet, visitedFragments)
			for _, entry := range fragEntries {
				if idx, ok := entryMap[entry.key]; ok {
					entries[idx].fields = append(entries[idx].fields, entry.fields...)
				} else {
					entryMap[entry.key] = len(entries)
					entries = append(entries, entry)
				}
			}
		}
	}

	return entries
}

func (ctx *executionContext) shouldSkip(sel Selection) bool {
	var directives []*Directive
	switch s := sel.(type) {
	case *Field:
		directives = s.Directives
	case *FragmentSpread:
		directives = s.Directives
	case *InlineFragment:
		directives = s.Directives
	}

	for _, d := range directives {
		if d.Name == "skip" {
			args, _ := CoerceArgumentValues(SkipDirective.Args, d.Arguments, ctx.variables)
			if ifVal, ok := args["if"].(bool); ok && ifVal {
				return true
			}
		}
		if d.Name == "include" {
			args, _ := CoerceArgumentValues(IncludeDirective.Args, d.Arguments, ctx.variables)
			if ifVal, ok := args["if"].(bool); ok && !ifVal {
				return true
			}
		}
	}
	return false
}

func (ctx *executionContext) doesFragmentApply(typeName string, objectType *ObjectType) bool {
	if typeName == objectType.Name_ {
		return true
	}
	t := ctx.schema.Type(typeName)
	if t == nil {
		return false
	}
	return ctx.schema.IsPossibleType(t, objectType)
}

func (ctx *executionContext) resolveField(parentType *ObjectType, source interface{}, fieldNodes []*Field, path []interface{}) (interface{}, error) {
	fieldNode := fieldNodes[0]
	fieldName := fieldNode.Name

	// Handle introspection fields
	if fieldName == "__typename" {
		return parentType.Name_, nil
	}
	if fieldName == "__schema" && parentType == ctx.schema.QueryType {
		return ctx.resolveIntrospectionSchema(fieldNode, path), nil
	}
	if fieldName == "__type" && parentType == ctx.schema.QueryType {
		args, _ := CoerceArgumentValues(
			ArgumentMap{"name": {Name_: "name", Type: NewNonNull(StringScalar)}},
			fieldNode.Arguments, ctx.variables)
		typeName, _ := args["name"].(string)
		t := ctx.schema.Type(typeName)
		if t == nil {
			return nil, nil
		}
		return ctx.resolveIntrospectionType(t, fieldNode, path), nil
	}

	fieldDef, ok := parentType.Fields_[fieldName]
	if !ok {
		return nil, nil
	}

	// Resolve arguments
	args, err := CoerceArgumentValues(fieldDef.Args, fieldNode.Arguments, ctx.variables)
	if err != nil {
		ctx.addError(err.Error(), fieldNode.Loc, path)
		return nil, ctx.fieldError(fieldDef.Type)
	}

	// Resolve field value
	var result interface{}
	if fieldDef.Resolve != nil {
		resolveParams := ResolveParams{
			Source: source,
			Args:   args,
			Info: ResolveInfo{
				FieldName:  fieldName,
				ReturnType: fieldDef.Type,
				ParentType: parentType,
				Path:       path,
				Schema:     ctx.schema,
				Operation:  ctx.operation,
				Fragments:  ctx.fragments,
				Variables:  ctx.variables,
			},
		}
		result, err = fieldDef.Resolve(resolveParams)
		if err != nil {
			ctx.addError(err.Error(), fieldNode.Loc, path)
			return nil, ctx.fieldError(fieldDef.Type)
		}
	} else {
		result = resolveFieldValue(source, fieldName)
	}

	completed, cerr := ctx.completeValue(fieldDef.Type, fieldNodes, result, path)
	if cerr != nil {
		return nil, ctx.fieldError(fieldDef.Type)
	}
	return completed, nil
}

// fieldError decides whether a completion failure propagates past the current
// field. A nullable field swallows the failure and resolves to null; a non-null
// field propagates it to the parent so that the parent resolves to null.
func (ctx *executionContext) fieldError(typ GraphQLType) error {
	if _, isNonNull := typ.(*NonNullOfType); isNonNull {
		return errNonNullViolation
	}
	return nil
}

// completeValue turns a resolved value into a response value according to its
// type. It returns errNonNullViolation when a non-null position completed to
// null; the caller at a nullable boundary converts that into a JSON null.
func (ctx *executionContext) completeValue(typ GraphQLType, fieldNodes []*Field, result interface{}, path []interface{}) (interface{}, error) {
	// NonNull
	if nn, ok := typ.(*NonNullOfType); ok {
		completed, err := ctx.completeValue(nn.OfType, fieldNodes, result, path)
		if err != nil {
			// A nested non-null violation already recorded its error; propagate it.
			return nil, err
		}
		if completed == nil {
			ctx.addError(
				"Cannot return null for non-nullable field.",
				fieldNodes[0].Loc, path)
			return nil, errNonNullViolation
		}
		return completed, nil
	}

	if result == nil {
		return nil, nil
	}

	// List
	if list, ok := typ.(*ListOfType); ok {
		return ctx.completeListValue(list, fieldNodes, result, path)
	}

	// Leaf
	if IsLeafType(typ) {
		return ctx.completeLeafValue(typ, fieldNodes, result, path)
	}

	// Abstract
	if IsAbstractType(typ) {
		return ctx.completeAbstractValue(typ, fieldNodes, result, path)
	}

	// Object
	if obj, ok := typ.(*ObjectType); ok {
		return ctx.completeObjectValue(obj, fieldNodes, result, path)
	}

	return nil, nil
}

func (ctx *executionContext) completeListValue(listType *ListOfType, fieldNodes []*Field, result interface{}, path []interface{}) (interface{}, error) {
	rv := reflect.ValueOf(result)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		ctx.addError("Expected iterable, did not find one.", fieldNodes[0].Loc, path)
		return nil, errNonNullViolation
	}

	// If elements are non-null, a failing element nulls the whole list.
	// If elements are nullable, a failing element becomes null and others survive.
	_, itemNonNull := listType.OfType.(*NonNullOfType)
	itemNullable := !itemNonNull
	items := make([]interface{}, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		itemPath := append(append([]interface{}{}, path...), i)
		item, err := ctx.completeValue(listType.OfType, fieldNodes, rv.Index(i).Interface(), itemPath)
		if err != nil {
			if itemNullable {
				items[i] = nil
				continue
			}
			return nil, err
		}
		items[i] = item
	}
	return items, nil
}

func (ctx *executionContext) completeLeafValue(typ GraphQLType, fieldNodes []*Field, result interface{}, path []interface{}) (interface{}, error) {
	switch t := typ.(type) {
	case *ScalarType:
		val, err := t.Serialize(result)
		if err != nil {
			ctx.addError(err.Error(), fieldNodes[0].Loc, path)
			return nil, errNonNullViolation
		}
		return val, nil
	case *EnumType:
		s := fmt.Sprintf("%v", result)
		for _, ev := range t.Values {
			val := ev.Name_
			if ev.Value != nil {
				val = fmt.Sprintf("%v", ev.Value)
			}
			if val == s || ev.Name_ == s {
				return ev.Name_, nil
			}
		}
		ctx.addError(fmt.Sprintf("Enum \"%s\" cannot represent value: %v", t.Name_, result), fieldNodes[0].Loc, path)
		return nil, errNonNullViolation
	}
	return nil, nil
}

func (ctx *executionContext) completeAbstractValue(abstractType GraphQLType, fieldNodes []*Field, result interface{}, path []interface{}) (interface{}, error) {
	var objectType *ObjectType

	switch at := abstractType.(type) {
	case *InterfaceType:
		if at.ResolveType != nil {
			info := ResolveInfo{Schema: ctx.schema}
			objectType = at.ResolveType(result, info)
		}
	case *UnionType:
		if at.ResolveType != nil {
			info := ResolveInfo{Schema: ctx.schema}
			objectType = at.ResolveType(result, info)
		}
	}

	if objectType == nil {
		// Try IsTypeOf
		possibleTypes := GetPossibleTypes(ctx.schema, abstractType)
		for _, pt := range possibleTypes {
			if pt.IsTypeOf != nil && pt.IsTypeOf(result) {
				objectType = pt
				break
			}
		}
	}

	if objectType == nil {
		ctx.addError(
			fmt.Sprintf("Abstract type \"%s\" must resolve to an Object type at runtime.", abstractType.TypeName()),
			fieldNodes[0].Loc, path)
		return nil, errNonNullViolation
	}

	return ctx.completeObjectValue(objectType, fieldNodes, result, path)
}

func (ctx *executionContext) completeObjectValue(objectType *ObjectType, fieldNodes []*Field, result interface{}, path []interface{}) (interface{}, error) {
	// Enforce max depth: count only string elements in path (skip list indices)
	if ctx.maxDepth > 0 {
		depth := 0
		for _, p := range path {
			if _, ok := p.(string); ok {
				depth++
			}
		}
		if depth > ctx.maxDepth {
			ctx.addError(
				fmt.Sprintf("Query depth %d exceeds maximum allowed depth of %d.", depth, ctx.maxDepth),
				fieldNodes[0].Loc, path)
			return nil, errNonNullViolation
		}
	}

	// Merge sub-selection sets
	var subSelections []Selection
	for _, fieldNode := range fieldNodes {
		subSelections = append(subSelections, fieldNode.SelectionSet...)
	}

	if len(subSelections) == 0 {
		return result, nil
	}

	return ctx.executeFields(objectType, result, subSelections, path)
}

func (ctx *executionContext) addError(message string, loc Location, path []interface{}) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.errors = append(ctx.errors, &GraphQLError{
		Message:   message,
		Locations: []ErrorLocation{{Line: loc.Line, Column: loc.Col}},
		Path:      path,
	})
}

// Introspection resolution helpers

func (ctx *executionContext) resolveIntrospectionSchema(fieldNode *Field, path []interface{}) interface{} {
	schemaObj := buildIntrospectionSchema(ctx.schema)
	if fieldNode.SelectionSet == nil {
		return schemaObj
	}
	return ctx.resolveIntrospectionObject(schemaObj, fieldNode.SelectionSet, path)
}

func (ctx *executionContext) resolveIntrospectionType(t GraphQLType, fieldNode *Field, path []interface{}) interface{} {
	b := newIntrospectionBuilder()
	typeObj := b.buildTypeObj(t)
	if fieldNode.SelectionSet == nil {
		return typeObj
	}
	return ctx.resolveIntrospectionObject(typeObj, fieldNode.SelectionSet, path)
}

func (ctx *executionContext) resolveIntrospectionObject(obj map[string]interface{}, selections []Selection, path []interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for _, sel := range selections {
		if ctx.shouldSkip(sel) {
			continue
		}
		switch s := sel.(type) {
		case *Field:
			responseName := s.Name
			if s.Alias != "" {
				responseName = s.Alias
			}
			if s.Name == "__typename" {
				result[responseName] = obj["__typename"]
				continue
			}
			val := obj[s.Name]
			if s.SelectionSet != nil && val != nil {
				switch v := val.(type) {
				case map[string]interface{}:
					result[responseName] = ctx.resolveIntrospectionObject(v, s.SelectionSet, append(path, responseName))
				case []interface{}:
					items := make([]interface{}, len(v))
					for i, item := range v {
						if m, ok := item.(map[string]interface{}); ok {
							items[i] = ctx.resolveIntrospectionObject(m, s.SelectionSet, append(path, responseName, i))
						} else {
							items[i] = item
						}
					}
					result[responseName] = items
				default:
					result[responseName] = val
				}
			} else {
				result[responseName] = val
			}
		case *InlineFragment:
			subResult := ctx.resolveIntrospectionObject(obj, s.SelectionSet, path)
			for k, v := range subResult {
				result[k] = v
			}
		case *FragmentSpread:
			if frag, ok := ctx.fragments[s.Name]; ok {
				subResult := ctx.resolveIntrospectionObject(obj, frag.SelectionSet, path)
				for k, v := range subResult {
					result[k] = v
				}
			}
		}
	}

	return result
}

// introspectionBuilder avoids infinite recursion when building introspection data for recursive types.
type introspectionBuilder struct {
	visited map[GraphQLType]bool
}

func newIntrospectionBuilder() *introspectionBuilder {
	return &introspectionBuilder{visited: make(map[GraphQLType]bool)}
}

// buildIntrospectionSchema creates the introspection data for __schema
func buildIntrospectionSchema(schema *Schema) map[string]interface{} {
	b := newIntrospectionBuilder()
	result := map[string]interface{}{
		"__typename": "__Schema",
	}

	// types
	var types []interface{}
	typeNames := make([]string, 0, len(schema.typeMap))
	for name := range schema.typeMap {
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)
	for _, name := range typeNames {
		types = append(types, b.buildTypeObj(schema.typeMap[name]))
	}
	result["types"] = types

	// queryType
	result["queryType"] = b.buildTypeObj(schema.QueryType)

	// mutationType
	if schema.MutationType != nil {
		result["mutationType"] = b.buildTypeObj(schema.MutationType)
	} else {
		result["mutationType"] = nil
	}

	// subscriptionType
	if schema.SubscriptionType != nil {
		result["subscriptionType"] = b.buildTypeObj(schema.SubscriptionType)
	} else {
		result["subscriptionType"] = nil
	}

	// directives
	var directives []interface{}
	for _, d := range schema.Directives_ {
		directives = append(directives, b.buildDirective(d))
	}
	result["directives"] = directives

	return result
}

// buildTypeRef builds a minimal type reference (name + kind + wrappers) without recursing into fields.
func (b *introspectionBuilder) buildTypeRef(t GraphQLType) map[string]interface{} {
	if t == nil {
		return nil
	}
	switch typ := t.(type) {
	case *ListOfType:
		return map[string]interface{}{
			"__typename": "__Type", "kind": "LIST", "name": nil,
			"ofType": b.buildTypeRef(typ.OfType),
		}
	case *NonNullOfType:
		return map[string]interface{}{
			"__typename": "__Type", "kind": "NON_NULL", "name": nil,
			"ofType": b.buildTypeRef(typ.OfType),
		}
	default:
		return b.buildTypeObj(t)
	}
}

func (b *introspectionBuilder) buildTypeObj(t GraphQLType) map[string]interface{} {
	if t == nil {
		return nil
	}

	// Handle wrappers first (no cycle issue)
	switch typ := t.(type) {
	case *ListOfType:
		obj := map[string]interface{}{
			"__typename": "__Type", "kind": "LIST", "name": nil, "description": nil,
			"fields": nil, "inputFields": nil, "interfaces": nil,
			"enumValues": nil, "possibleTypes": nil,
			"ofType": b.buildTypeRef(typ.OfType),
		}
		return obj
	case *NonNullOfType:
		obj := map[string]interface{}{
			"__typename": "__Type", "kind": "NON_NULL", "name": nil, "description": nil,
			"fields": nil, "inputFields": nil, "interfaces": nil,
			"enumValues": nil, "possibleTypes": nil,
			"ofType": b.buildTypeRef(typ.OfType),
		}
		return obj
	}

	// Check visited for named types to prevent infinite recursion
	if b.visited[t] {
		// Return a minimal reference
		return map[string]interface{}{
			"__typename":  "__Type",
			"kind":        typeKindString(t),
			"name":        t.TypeName(),
			"description": nil,
			"fields":      nil, "inputFields": nil, "interfaces": nil,
			"enumValues": nil, "possibleTypes": nil, "ofType": nil,
		}
	}
	b.visited[t] = true

	obj := map[string]interface{}{}

	switch typ := t.(type) {
	case *ScalarType:
		obj["__typename"] = "__Type"
		obj["kind"] = "SCALAR"
		obj["name"] = typ.Name_
		obj["description"] = typ.Description
		obj["fields"] = nil
		obj["inputFields"] = nil
		obj["interfaces"] = nil
		obj["enumValues"] = nil
		obj["possibleTypes"] = nil
		obj["ofType"] = nil

	case *ObjectType:
		obj["__typename"] = "__Type"
		obj["kind"] = "OBJECT"
		obj["name"] = typ.Name_
		obj["description"] = typ.Description
		obj["fields"] = b.buildFields(typ.Fields_, false)
		obj["inputFields"] = nil
		var interfaces []interface{}
		for _, iface := range typ.Interfaces_ {
			interfaces = append(interfaces, b.buildTypeObj(iface))
		}
		obj["interfaces"] = interfaces
		obj["enumValues"] = nil
		obj["possibleTypes"] = nil
		obj["ofType"] = nil

	case *InterfaceType:
		obj["__typename"] = "__Type"
		obj["kind"] = "INTERFACE"
		obj["name"] = typ.Name_
		obj["description"] = typ.Description
		obj["fields"] = b.buildFields(typ.Fields_, false)
		obj["inputFields"] = nil
		obj["interfaces"] = nil
		obj["enumValues"] = nil
		obj["possibleTypes"] = nil
		obj["ofType"] = nil

	case *UnionType:
		obj["__typename"] = "__Type"
		obj["kind"] = "UNION"
		obj["name"] = typ.Name_
		obj["description"] = typ.Description
		obj["fields"] = nil
		obj["inputFields"] = nil
		obj["interfaces"] = nil
		obj["enumValues"] = nil
		var possibleTypes []interface{}
		for _, pt := range typ.Types {
			possibleTypes = append(possibleTypes, b.buildTypeObj(pt))
		}
		obj["possibleTypes"] = possibleTypes
		obj["ofType"] = nil

	case *EnumType:
		obj["__typename"] = "__Type"
		obj["kind"] = "ENUM"
		obj["name"] = typ.Name_
		obj["description"] = typ.Description
		obj["fields"] = nil
		obj["inputFields"] = nil
		obj["interfaces"] = nil
		var enumValues []interface{}
		for _, ev := range typ.Values {
			enumValues = append(enumValues, map[string]interface{}{
				"__typename":        "__EnumValue",
				"name":              ev.Name_,
				"description":       ev.Description,
				"isDeprecated":      ev.DeprecationReason != "",
				"deprecationReason": ev.DeprecationReason,
			})
		}
		obj["enumValues"] = enumValues
		obj["possibleTypes"] = nil
		obj["ofType"] = nil

	case *InputObjectType:
		obj["__typename"] = "__Type"
		obj["kind"] = "INPUT_OBJECT"
		obj["name"] = typ.Name_
		obj["description"] = typ.Description
		obj["fields"] = nil
		var inputFields []interface{}
		for name, field := range typ.Fields_ {
			inputFields = append(inputFields, map[string]interface{}{
				"__typename":   "__InputValue",
				"name":         name,
				"description":  field.Description,
				"type":         b.buildTypeRef(field.Type),
				"defaultValue": formatDefaultValue(field.DefaultValue),
			})
		}
		obj["inputFields"] = inputFields
		obj["interfaces"] = nil
		obj["enumValues"] = nil
		obj["possibleTypes"] = nil
		obj["ofType"] = nil
	}

	return obj
}

func typeKindString(t GraphQLType) string {
	switch t.(type) {
	case *ScalarType:
		return "SCALAR"
	case *ObjectType:
		return "OBJECT"
	case *InterfaceType:
		return "INTERFACE"
	case *UnionType:
		return "UNION"
	case *EnumType:
		return "ENUM"
	case *InputObjectType:
		return "INPUT_OBJECT"
	case *ListOfType:
		return "LIST"
	case *NonNullOfType:
		return "NON_NULL"
	}
	return ""
}

func (b *introspectionBuilder) buildFields(fields FieldMap, includeDeprecated bool) []interface{} {
	if fields == nil {
		return nil
	}
	var result []interface{}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		field := fields[name]
		if !includeDeprecated && field.DeprecationReason != "" {
			continue
		}
		var args []interface{}
		for argName, arg := range field.Args {
			args = append(args, map[string]interface{}{
				"__typename":   "__InputValue",
				"name":         argName,
				"description":  arg.Description,
				"type":         b.buildTypeRef(arg.Type),
				"defaultValue": formatDefaultValue(arg.DefaultValue),
			})
		}
		result = append(result, map[string]interface{}{
			"__typename":        "__Field",
			"name":              name,
			"description":       field.Description,
			"args":              args,
			"type":              b.buildTypeRef(field.Type),
			"isDeprecated":      field.DeprecationReason != "",
			"deprecationReason": field.DeprecationReason,
		})
	}
	return result
}

func (b *introspectionBuilder) buildDirective(d *DirectiveDefinition) map[string]interface{} {
	var args []interface{}
	for name, arg := range d.Args {
		args = append(args, map[string]interface{}{
			"__typename":   "__InputValue",
			"name":         name,
			"description":  arg.Description,
			"type":         b.buildTypeRef(arg.Type),
			"defaultValue": formatDefaultValue(arg.DefaultValue),
		})
	}
	var locations []interface{}
	for _, loc := range d.Locations {
		locations = append(locations, string(loc))
	}
	return map[string]interface{}{
		"__typename":   "__Directive",
		"name":         d.Name_,
		"description":  d.Description,
		"locations":    locations,
		"args":         args,
		"isRepeatable": d.IsRepeatable,
	}
}

func formatDefaultValue(val interface{}) interface{} {
	if val == nil {
		return nil
	}
	return fmt.Sprintf("%v", val)
}
