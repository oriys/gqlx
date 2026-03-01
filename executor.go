package gqlx

import (
	"fmt"
	"reflect"
	"sort"
)

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
	Schema         *Schema
	Document       *Document
	RootValue      interface{}
	Variables      map[string]interface{}
	OperationName  string
}

// Execute executes a GraphQL document.
func (e *Executor) Execute(params ExecuteParams) *Result {
	doc := params.Document
	schema := params.Schema
	if schema == nil {
		schema = e.schema
	}

	// Find the operation
	var operation *OperationDefinition
	fragments := make(map[string]*FragmentDefinition)

	for _, def := range doc.Definitions {
		switch d := def.(type) {
		case *OperationDefinition:
			if params.OperationName == "" {
				if operation != nil {
					return &Result{Errors: []*GraphQLError{{Message: "Must provide operation name if query contains multiple operations."}}}
				}
				operation = d
			} else if d.Name == params.OperationName {
				operation = d
			}
		case *FragmentDefinition:
			fragments[d.Name] = d
		}
	}

	if operation == nil {
		if params.OperationName != "" {
			return &Result{Errors: []*GraphQLError{{Message: fmt.Sprintf("Unknown operation named \"%s\".", params.OperationName)}}}
		}
		return &Result{Errors: []*GraphQLError{{Message: "Must provide an operation."}}}
	}

	// Coerce variables
	variables := params.Variables
	if variables == nil {
		variables = make(map[string]interface{})
	}
	coercedVars, varErrors := CoerceVariableValues(schema, operation.VariableDefinitions, variables)
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

	ctx := &executionContext{
		schema:    schema,
		fragments: fragments,
		variables: coercedVars,
		errors:    nil,
		operation: operation,
	}

	rootValue := params.RootValue

	var data interface{}
	if operation.Operation == OperationMutation {
		data = ctx.executeFieldsSerially(rootType, rootValue, operation.SelectionSet, []interface{}{})
	} else {
		data = ctx.executeFields(rootType, rootValue, operation.SelectionSet, []interface{}{})
	}

	return &Result{Data: data, Errors: ctx.errors}
}

type executionContext struct {
	schema    *Schema
	fragments map[string]*FragmentDefinition
	variables map[string]interface{}
	errors    []*GraphQLError
	operation *OperationDefinition
}

func (ctx *executionContext) executeFields(parentType *ObjectType, source interface{}, selections []Selection, path []interface{}) map[string]interface{} {
	groupedFields := ctx.collectFields(parentType, selections, nil)
	result := make(map[string]interface{})

	for _, entry := range groupedFields {
		responseName := entry.key
		fieldNodes := entry.fields
		fieldPath := append(append([]interface{}{}, path...), responseName)
		result[responseName] = ctx.resolveField(parentType, source, fieldNodes, fieldPath)
	}

	return result
}

func (ctx *executionContext) executeFieldsSerially(parentType *ObjectType, source interface{}, selections []Selection, path []interface{}) map[string]interface{} {
	// For mutations, fields are executed serially (same as executeFields in Go since we're single-threaded)
	return ctx.executeFields(parentType, source, selections, path)
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

func (ctx *executionContext) resolveField(parentType *ObjectType, source interface{}, fieldNodes []*Field, path []interface{}) interface{} {
	fieldNode := fieldNodes[0]
	fieldName := fieldNode.Name

	// Handle introspection fields
	if fieldName == "__typename" {
		return parentType.Name_
	}
	if fieldName == "__schema" && parentType == ctx.schema.QueryType {
		return ctx.resolveIntrospectionSchema(fieldNode, path)
	}
	if fieldName == "__type" && parentType == ctx.schema.QueryType {
		args, _ := CoerceArgumentValues(
			ArgumentMap{"name": {Name_: "name", Type: NewNonNull(StringScalar)}},
			fieldNode.Arguments, ctx.variables)
		typeName, _ := args["name"].(string)
		t := ctx.schema.Type(typeName)
		if t == nil {
			return nil
		}
		return ctx.resolveIntrospectionType(t, fieldNode, path)
	}

	fieldDef, ok := parentType.Fields_[fieldName]
	if !ok {
		return nil
	}

	// Resolve arguments
	args, err := CoerceArgumentValues(fieldDef.Args, fieldNode.Arguments, ctx.variables)
	if err != nil {
		ctx.addError(err.Error(), fieldNode.Loc, path)
		return nil
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
			return ctx.handleNullPropagation(fieldDef.Type)
		}
	} else {
		result = resolveFieldValue(source, fieldName)
	}

	return ctx.completeValue(fieldDef.Type, fieldNodes, result, path)
}

func (ctx *executionContext) completeValue(typ GraphQLType, fieldNodes []*Field, result interface{}, path []interface{}) interface{} {
	// NonNull
	if nn, ok := typ.(*NonNullOfType); ok {
		completed := ctx.completeValue(nn.OfType, fieldNodes, result, path)
		if completed == nil {
			ctx.addError(
				fmt.Sprintf("Cannot return null for non-nullable field."),
				fieldNodes[0].Loc, path)
			return nil
		}
		return completed
	}

	if result == nil {
		return nil
	}

	// List
	if list, ok := typ.(*ListOfType); ok {
		return ctx.completeListValue(list, fieldNodes, result, path)
	}

	// Leaf
	if IsLeafType(typ) {
		return ctx.completeLeafValue(typ, result)
	}

	// Abstract
	if IsAbstractType(typ) {
		return ctx.completeAbstractValue(typ, fieldNodes, result, path)
	}

	// Object
	if obj, ok := typ.(*ObjectType); ok {
		return ctx.completeObjectValue(obj, fieldNodes, result, path)
	}

	return nil
}

func (ctx *executionContext) completeListValue(listType *ListOfType, fieldNodes []*Field, result interface{}, path []interface{}) interface{} {
	rv := reflect.ValueOf(result)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		ctx.addError("Expected iterable, did not find one.", fieldNodes[0].Loc, path)
		return nil
	}

	items := make([]interface{}, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		itemPath := append(append([]interface{}{}, path...), i)
		items[i] = ctx.completeValue(listType.OfType, fieldNodes, rv.Index(i).Interface(), itemPath)
	}
	return items
}

func (ctx *executionContext) completeLeafValue(typ GraphQLType, result interface{}) interface{} {
	switch t := typ.(type) {
	case *ScalarType:
		val, err := t.Serialize(result)
		if err != nil {
			return nil
		}
		return val
	case *EnumType:
		s := fmt.Sprintf("%v", result)
		for _, ev := range t.Values {
			val := ev.Name_
			if ev.Value != nil {
				val = fmt.Sprintf("%v", ev.Value)
			}
			if val == s || ev.Name_ == s {
				return ev.Name_
			}
		}
		return nil
	}
	return nil
}

func (ctx *executionContext) completeAbstractValue(abstractType GraphQLType, fieldNodes []*Field, result interface{}, path []interface{}) interface{} {
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
		return nil
	}

	return ctx.completeObjectValue(objectType, fieldNodes, result, path)
}

func (ctx *executionContext) completeObjectValue(objectType *ObjectType, fieldNodes []*Field, result interface{}, path []interface{}) interface{} {
	// Merge sub-selection sets
	var subSelections []Selection
	for _, fieldNode := range fieldNodes {
		subSelections = append(subSelections, fieldNode.SelectionSet...)
	}

	if len(subSelections) == 0 {
		return result
	}

	return ctx.executeFields(objectType, result, subSelections, path)
}

func (ctx *executionContext) handleNullPropagation(typ GraphQLType) interface{} {
	if _, ok := typ.(*NonNullOfType); ok {
		return nil
	}
	return nil
}

func (ctx *executionContext) addError(message string, loc Location, path []interface{}) {
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
	b2 := newIntrospectionBuilder()
	result["queryType"] = b2.buildTypeObj(schema.QueryType)

	// mutationType
	if schema.MutationType != nil {
		b3 := newIntrospectionBuilder()
		result["mutationType"] = b3.buildTypeObj(schema.MutationType)
	} else {
		result["mutationType"] = nil
	}

	// subscriptionType
	if schema.SubscriptionType != nil {
		b4 := newIntrospectionBuilder()
		result["subscriptionType"] = b4.buildTypeObj(schema.SubscriptionType)
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
			"__typename": "__Type",
			"kind":       typeKindString(t),
			"name":       t.TypeName(),
			"description": nil,
			"fields": nil, "inputFields": nil, "interfaces": nil,
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
