package gqlx

import "fmt"

// Validate validates a document against a schema.
func Validate(schema *Schema, doc *Document) []*GraphQLError {
	ctx := &validationContext{
		schema:    schema,
		doc:       doc,
		fragments: make(map[string]*FragmentDefinition),
	}

	// Collect fragments
	for _, def := range doc.Definitions {
		if frag, ok := def.(*FragmentDefinition); ok {
			ctx.fragments[frag.Name] = frag
		}
	}

	var errors []*GraphQLError

	// Run all validation rules
	errors = append(errors, ctx.validateExecutableDefinitions()...)
	errors = append(errors, ctx.validateUniqueOperationNames()...)
	errors = append(errors, ctx.validateLoneAnonymousOperation()...)
	errors = append(errors, ctx.validateKnownTypeNames()...)
	errors = append(errors, ctx.validateFragmentsOnCompositeTypes()...)
	errors = append(errors, ctx.validateUniqueFragmentNames()...)
	errors = append(errors, ctx.validateKnownFragmentNames()...)
	errors = append(errors, ctx.validateNoUnusedFragments()...)
	errors = append(errors, ctx.validateNoFragmentCycles()...)
	errors = append(errors, ctx.validateFieldsOnCorrectType()...)
	errors = append(errors, ctx.validateScalarLeafs()...)
	errors = append(errors, ctx.validateKnownArgumentNames()...)
	errors = append(errors, ctx.validateUniqueArgumentNames()...)
	errors = append(errors, ctx.validateProvidedRequiredArguments()...)
	errors = append(errors, ctx.validateUniqueVariableNames()...)
	errors = append(errors, ctx.validateNoUndefinedVariables()...)
	errors = append(errors, ctx.validateNoUnusedVariables()...)
	errors = append(errors, ctx.validateKnownDirectives()...)
	errors = append(errors, ctx.validateUniqueDirectivesPerLocation()...)
	errors = append(errors, ctx.validateVariablesAreInputTypes()...)

	return errors
}

type validationContext struct {
	schema    *Schema
	doc       *Document
	fragments map[string]*FragmentDefinition
}

func (ctx *validationContext) validateExecutableDefinitions() []*GraphQLError {
	// All definitions should be operations or fragments (already enforced by parser)
	return nil
}

func (ctx *validationContext) validateUniqueOperationNames() []*GraphQLError {
	var errors []*GraphQLError
	seen := make(map[string]bool)
	for _, def := range ctx.doc.Definitions {
		if op, ok := def.(*OperationDefinition); ok && op.Name != "" {
			if seen[op.Name] {
				errors = append(errors, NewValidationError(
					fmt.Sprintf("There can be only one operation named \"%s\".", op.Name),
					op.Loc))
			}
			seen[op.Name] = true
		}
	}
	return errors
}

func (ctx *validationContext) validateLoneAnonymousOperation() []*GraphQLError {
	var opCount int
	var hasAnonymous bool
	var anonLoc Location
	for _, def := range ctx.doc.Definitions {
		if op, ok := def.(*OperationDefinition); ok {
			opCount++
			if op.Name == "" {
				hasAnonymous = true
				anonLoc = op.Loc
			}
		}
	}
	if hasAnonymous && opCount > 1 {
		return []*GraphQLError{NewValidationError(
			"This anonymous operation must be the only defined operation.", anonLoc)}
	}
	return nil
}

func (ctx *validationContext) validateKnownTypeNames() []*GraphQLError {
	var errors []*GraphQLError
	for _, def := range ctx.doc.Definitions {
		if frag, ok := def.(*FragmentDefinition); ok {
			if ctx.schema.Type(frag.TypeCondition) == nil {
				errors = append(errors, NewValidationError(
					fmt.Sprintf("Unknown type \"%s\".", frag.TypeCondition),
					frag.Loc))
			}
		}
	}
	// Check inline fragments
	for _, def := range ctx.doc.Definitions {
		var selections []Selection
		switch d := def.(type) {
		case *OperationDefinition:
			selections = d.SelectionSet
		case *FragmentDefinition:
			selections = d.SelectionSet
		}
		errors = append(errors, ctx.checkInlineFragmentTypes(selections)...)
	}
	return errors
}

func (ctx *validationContext) checkInlineFragmentTypes(selections []Selection) []*GraphQLError {
	var errors []*GraphQLError
	for _, sel := range selections {
		switch s := sel.(type) {
		case *InlineFragment:
			if s.TypeCondition != "" && ctx.schema.Type(s.TypeCondition) == nil {
				errors = append(errors, NewValidationError(
					fmt.Sprintf("Unknown type \"%s\".", s.TypeCondition),
					s.Loc))
			}
			errors = append(errors, ctx.checkInlineFragmentTypes(s.SelectionSet)...)
		case *Field:
			errors = append(errors, ctx.checkInlineFragmentTypes(s.SelectionSet)...)
		}
	}
	return errors
}

func (ctx *validationContext) validateFragmentsOnCompositeTypes() []*GraphQLError {
	var errors []*GraphQLError
	for _, def := range ctx.doc.Definitions {
		if frag, ok := def.(*FragmentDefinition); ok {
			t := ctx.schema.Type(frag.TypeCondition)
			if t != nil && !IsCompositeType(t) {
				errors = append(errors, NewValidationError(
					fmt.Sprintf("Fragment \"%s\" cannot condition on non composite type \"%s\".", frag.Name, frag.TypeCondition),
					frag.Loc))
			}
		}
	}
	return errors
}

func (ctx *validationContext) validateUniqueFragmentNames() []*GraphQLError {
	var errors []*GraphQLError
	seen := make(map[string]bool)
	for _, def := range ctx.doc.Definitions {
		if frag, ok := def.(*FragmentDefinition); ok {
			if seen[frag.Name] {
				errors = append(errors, NewValidationError(
					fmt.Sprintf("There can be only one fragment named \"%s\".", frag.Name),
					frag.Loc))
			}
			seen[frag.Name] = true
		}
	}
	return errors
}

func (ctx *validationContext) validateKnownFragmentNames() []*GraphQLError {
	var errors []*GraphQLError
	for _, def := range ctx.doc.Definitions {
		switch d := def.(type) {
		case *OperationDefinition:
			errors = append(errors, ctx.checkFragmentSpreads(d.SelectionSet)...)
		case *FragmentDefinition:
			errors = append(errors, ctx.checkFragmentSpreads(d.SelectionSet)...)
		}
	}
	return errors
}

func (ctx *validationContext) checkFragmentSpreads(selections []Selection) []*GraphQLError {
	var errors []*GraphQLError
	for _, sel := range selections {
		switch s := sel.(type) {
		case *FragmentSpread:
			if _, ok := ctx.fragments[s.Name]; !ok {
				errors = append(errors, NewValidationError(
					fmt.Sprintf("Unknown fragment \"%s\".", s.Name),
					s.Loc))
			}
		case *InlineFragment:
			errors = append(errors, ctx.checkFragmentSpreads(s.SelectionSet)...)
		case *Field:
			errors = append(errors, ctx.checkFragmentSpreads(s.SelectionSet)...)
		}
	}
	return errors
}

func (ctx *validationContext) validateNoUnusedFragments() []*GraphQLError {
	var errors []*GraphQLError
	used := make(map[string]bool)

	for _, def := range ctx.doc.Definitions {
		if op, ok := def.(*OperationDefinition); ok {
			ctx.collectUsedFragments(op.SelectionSet, used)
		}
	}

	for _, def := range ctx.doc.Definitions {
		if frag, ok := def.(*FragmentDefinition); ok {
			if !used[frag.Name] {
				errors = append(errors, NewValidationError(
					fmt.Sprintf("Fragment \"%s\" is never used.", frag.Name),
					frag.Loc))
			}
		}
	}
	return errors
}

func (ctx *validationContext) collectUsedFragments(selections []Selection, used map[string]bool) {
	for _, sel := range selections {
		switch s := sel.(type) {
		case *FragmentSpread:
			if !used[s.Name] {
				used[s.Name] = true
				if frag, ok := ctx.fragments[s.Name]; ok {
					ctx.collectUsedFragments(frag.SelectionSet, used)
				}
			}
		case *InlineFragment:
			ctx.collectUsedFragments(s.SelectionSet, used)
		case *Field:
			ctx.collectUsedFragments(s.SelectionSet, used)
		}
	}
}

func (ctx *validationContext) validateNoFragmentCycles() []*GraphQLError {
	var errors []*GraphQLError
	visited := make(map[string]bool)

	for name := range ctx.fragments {
		if !visited[name] {
			path := make(map[string]bool)
			if ctx.detectFragmentCycle(name, path, visited) {
				errors = append(errors, NewValidationError(
					fmt.Sprintf("Cannot spread fragment \"%s\" within itself.", name),
					ctx.fragments[name].Loc))
			}
		}
	}
	return errors
}

func (ctx *validationContext) detectFragmentCycle(name string, path map[string]bool, visited map[string]bool) bool {
	if path[name] {
		return true
	}
	if visited[name] {
		return false
	}

	path[name] = true
	visited[name] = true

	frag, ok := ctx.fragments[name]
	if !ok {
		return false
	}

	spreads := ctx.getFragmentSpreads(frag.SelectionSet)
	for _, spread := range spreads {
		if ctx.detectFragmentCycle(spread, path, visited) {
			return true
		}
	}

	delete(path, name)
	return false
}

func (ctx *validationContext) getFragmentSpreads(selections []Selection) []string {
	var spreads []string
	for _, sel := range selections {
		switch s := sel.(type) {
		case *FragmentSpread:
			spreads = append(spreads, s.Name)
		case *InlineFragment:
			spreads = append(spreads, ctx.getFragmentSpreads(s.SelectionSet)...)
		case *Field:
			spreads = append(spreads, ctx.getFragmentSpreads(s.SelectionSet)...)
		}
	}
	return spreads
}

func (ctx *validationContext) validateFieldsOnCorrectType() []*GraphQLError {
	var errors []*GraphQLError
	for _, def := range ctx.doc.Definitions {
		switch d := def.(type) {
		case *OperationDefinition:
			parentType := ctx.getOperationType(d)
			if parentType != nil {
				errors = append(errors, ctx.checkFields(d.SelectionSet, parentType)...)
			}
		case *FragmentDefinition:
			parentType := ctx.schema.Type(d.TypeCondition)
			if parentType != nil {
				errors = append(errors, ctx.checkFields(d.SelectionSet, parentType)...)
			}
		}
	}
	return errors
}

func (ctx *validationContext) getOperationType(op *OperationDefinition) GraphQLType {
	switch op.Operation {
	case OperationQuery:
		if ctx.schema.QueryType != nil {
			return ctx.schema.QueryType
		}
	case OperationMutation:
		if ctx.schema.MutationType != nil {
			return ctx.schema.MutationType
		}
	case OperationSubscription:
		if ctx.schema.SubscriptionType != nil {
			return ctx.schema.SubscriptionType
		}
	}
	return nil
}

func (ctx *validationContext) checkFields(selections []Selection, parentType GraphQLType) []*GraphQLError {
	return ctx.checkFieldsRecursive(selections, parentType, make(map[string]bool))
}

func (ctx *validationContext) checkFieldsRecursive(selections []Selection, parentType GraphQLType, visitedFragments map[string]bool) []*GraphQLError {
	var errors []*GraphQLError
	fields := GetFields(parentType)

	for _, sel := range selections {
		switch s := sel.(type) {
		case *Field:
			if s.Name == "__typename" || s.Name == "__schema" || s.Name == "__type" {
				if s.SelectionSet != nil {
					var fieldType GraphQLType
					switch s.Name {
					case "__schema":
						fieldType = introspectionSchemaType()
					case "__type":
						fieldType = introspectionTypeType()
					}
					if fieldType != nil {
						errors = append(errors, ctx.checkFieldsRecursive(s.SelectionSet, fieldType, visitedFragments)...)
					}
				}
				continue
			}
			if fields == nil {
				errors = append(errors, NewValidationError(
					fmt.Sprintf("Cannot query field \"%s\" on type \"%s\".", s.Name, parentType.TypeName()),
					s.Loc))
				continue
			}
			fieldDef, ok := fields[s.Name]
			if !ok {
				errors = append(errors, NewValidationError(
					fmt.Sprintf("Cannot query field \"%s\" on type \"%s\".", s.Name, parentType.TypeName()),
					s.Loc))
				continue
			}
			if s.SelectionSet != nil {
				innerType := UnwrapType(fieldDef.Type)
				errors = append(errors, ctx.checkFieldsRecursive(s.SelectionSet, innerType, visitedFragments)...)
			}
		case *InlineFragment:
			typeName := s.TypeCondition
			if typeName == "" {
				errors = append(errors, ctx.checkFieldsRecursive(s.SelectionSet, parentType, visitedFragments)...)
			} else {
				t := ctx.schema.Type(typeName)
				if t != nil {
					errors = append(errors, ctx.checkFieldsRecursive(s.SelectionSet, t, visitedFragments)...)
				}
			}
		case *FragmentSpread:
			if visitedFragments[s.Name] {
				continue
			}
			visitedFragments[s.Name] = true
			if frag, ok := ctx.fragments[s.Name]; ok {
				t := ctx.schema.Type(frag.TypeCondition)
				if t != nil {
					errors = append(errors, ctx.checkFieldsRecursive(frag.SelectionSet, t, visitedFragments)...)
				}
			}
		}
	}
	return errors
}

func (ctx *validationContext) validateScalarLeafs() []*GraphQLError {
	var errors []*GraphQLError
	for _, def := range ctx.doc.Definitions {
		switch d := def.(type) {
		case *OperationDefinition:
			parentType := ctx.getOperationType(d)
			if parentType != nil {
				errors = append(errors, ctx.checkScalarLeafs(d.SelectionSet, parentType, make(map[string]bool))...)
			}
		case *FragmentDefinition:
			parentType := ctx.schema.Type(d.TypeCondition)
			if parentType != nil {
				errors = append(errors, ctx.checkScalarLeafs(d.SelectionSet, parentType, make(map[string]bool))...)
			}
		}
	}
	return errors
}

func (ctx *validationContext) checkScalarLeafs(selections []Selection, parentType GraphQLType, vf map[string]bool) []*GraphQLError {
	var errors []*GraphQLError
	fields := GetFields(parentType)

	for _, sel := range selections {
		switch s := sel.(type) {
		case *Field:
			if s.Name == "__typename" {
				continue
			}
			if s.Name == "__schema" || s.Name == "__type" {
				continue
			}
			if fields == nil {
				continue
			}
			fieldDef, ok := fields[s.Name]
			if !ok {
				continue
			}
			fieldType := UnwrapType(fieldDef.Type)
			if IsLeafType(fieldType) {
				if s.SelectionSet != nil && len(s.SelectionSet) > 0 {
					errors = append(errors, NewValidationError(
						fmt.Sprintf("Field \"%s\" must not have a selection since type \"%s\" has no subfields.", s.Name, fieldType),
						s.Loc))
				}
			} else {
				if s.SelectionSet == nil || len(s.SelectionSet) == 0 {
					errors = append(errors, NewValidationError(
						fmt.Sprintf("Field \"%s\" of type \"%s\" must have a selection of subfields.", s.Name, fieldType),
						s.Loc))
				} else {
					errors = append(errors, ctx.checkScalarLeafs(s.SelectionSet, fieldType, vf)...)
				}
			}
		case *InlineFragment:
			typeName := s.TypeCondition
			if typeName == "" {
				errors = append(errors, ctx.checkScalarLeafs(s.SelectionSet, parentType, vf)...)
			} else {
				t := ctx.schema.Type(typeName)
				if t != nil {
					errors = append(errors, ctx.checkScalarLeafs(s.SelectionSet, t, vf)...)
				}
			}
		case *FragmentSpread:
			if vf[s.Name] {
				continue
			}
			vf[s.Name] = true
			if frag, ok := ctx.fragments[s.Name]; ok {
				t := ctx.schema.Type(frag.TypeCondition)
				if t != nil {
					errors = append(errors, ctx.checkScalarLeafs(frag.SelectionSet, t, vf)...)
				}
			}
		}
	}
	return errors
}

func (ctx *validationContext) validateKnownArgumentNames() []*GraphQLError {
	var errors []*GraphQLError
	for _, def := range ctx.doc.Definitions {
		switch d := def.(type) {
		case *OperationDefinition:
			parentType := ctx.getOperationType(d)
			if parentType != nil {
				errors = append(errors, ctx.checkArgNames(d.SelectionSet, parentType, make(map[string]bool))...)
			}
		}
	}
	return errors
}

func (ctx *validationContext) checkArgNames(selections []Selection, parentType GraphQLType, vf map[string]bool) []*GraphQLError {
	var errors []*GraphQLError
	fields := GetFields(parentType)

	for _, sel := range selections {
		switch s := sel.(type) {
		case *Field:
			if s.Name == "__typename" || s.Name == "__schema" || s.Name == "__type" {
				continue
			}
			if fields == nil {
				continue
			}
			fieldDef, ok := fields[s.Name]
			if !ok {
				continue
			}
			for _, arg := range s.Arguments {
				if fieldDef.Args == nil {
					errors = append(errors, NewValidationError(
						fmt.Sprintf("Unknown argument \"%s\" on field \"%s.%s\".", arg.Name, parentType.TypeName(), s.Name),
						arg.Loc))
					continue
				}
				if _, ok := fieldDef.Args[arg.Name]; !ok {
					errors = append(errors, NewValidationError(
						fmt.Sprintf("Unknown argument \"%s\" on field \"%s.%s\".", arg.Name, parentType.TypeName(), s.Name),
						arg.Loc))
				}
			}
			if s.SelectionSet != nil {
				innerType := UnwrapType(fieldDef.Type)
				errors = append(errors, ctx.checkArgNames(s.SelectionSet, innerType, vf)...)
			}
		case *InlineFragment:
			t := parentType
			if s.TypeCondition != "" {
				if resolved := ctx.schema.Type(s.TypeCondition); resolved != nil {
					t = resolved
				}
			}
			errors = append(errors, ctx.checkArgNames(s.SelectionSet, t, vf)...)
		case *FragmentSpread:
			if vf[s.Name] {
				continue
			}
			vf[s.Name] = true
			if frag, ok := ctx.fragments[s.Name]; ok {
				if t := ctx.schema.Type(frag.TypeCondition); t != nil {
					errors = append(errors, ctx.checkArgNames(frag.SelectionSet, t, vf)...)
				}
			}
		}
	}
	return errors
}

func (ctx *validationContext) validateUniqueArgumentNames() []*GraphQLError {
	var errors []*GraphQLError
	for _, def := range ctx.doc.Definitions {
		switch d := def.(type) {
		case *OperationDefinition:
			errors = append(errors, ctx.checkUniqueArgs(d.SelectionSet)...)
		case *FragmentDefinition:
			errors = append(errors, ctx.checkUniqueArgs(d.SelectionSet)...)
		}
	}
	return errors
}

func (ctx *validationContext) checkUniqueArgs(selections []Selection) []*GraphQLError {
	var errors []*GraphQLError
	for _, sel := range selections {
		switch s := sel.(type) {
		case *Field:
			seen := make(map[string]bool)
			for _, arg := range s.Arguments {
				if seen[arg.Name] {
					errors = append(errors, NewValidationError(
						fmt.Sprintf("There can be only one argument named \"%s\".", arg.Name),
						arg.Loc))
				}
				seen[arg.Name] = true
			}
			errors = append(errors, ctx.checkUniqueArgs(s.SelectionSet)...)
		case *InlineFragment:
			errors = append(errors, ctx.checkUniqueArgs(s.SelectionSet)...)
		case *FragmentSpread:
			// directives args handled separately
		}
	}
	return errors
}

func (ctx *validationContext) validateProvidedRequiredArguments() []*GraphQLError {
	var errors []*GraphQLError
	for _, def := range ctx.doc.Definitions {
		switch d := def.(type) {
		case *OperationDefinition:
			parentType := ctx.getOperationType(d)
			if parentType != nil {
				errors = append(errors, ctx.checkRequiredArgs(d.SelectionSet, parentType, make(map[string]bool))...)
			}
		}
	}
	return errors
}

func (ctx *validationContext) checkRequiredArgs(selections []Selection, parentType GraphQLType, vf map[string]bool) []*GraphQLError {
	var errors []*GraphQLError
	fields := GetFields(parentType)

	for _, sel := range selections {
		switch s := sel.(type) {
		case *Field:
			if s.Name == "__typename" || s.Name == "__schema" || s.Name == "__type" {
				continue
			}
			if fields == nil {
				continue
			}
			fieldDef, ok := fields[s.Name]
			if !ok {
				continue
			}
			provided := make(map[string]bool)
			for _, arg := range s.Arguments {
				provided[arg.Name] = true
			}
			for argName, argDef := range fieldDef.Args {
				if _, isNonNull := argDef.Type.(*NonNullOfType); isNonNull {
					if argDef.DefaultValue == nil && !provided[argName] {
						errors = append(errors, NewValidationError(
							fmt.Sprintf("Field \"%s\" argument \"%s\" of type \"%s\" is required, but it was not provided.", s.Name, argName, argDef.Type),
							s.Loc))
					}
				}
			}
			if s.SelectionSet != nil {
				innerType := UnwrapType(fieldDef.Type)
				errors = append(errors, ctx.checkRequiredArgs(s.SelectionSet, innerType, vf)...)
			}
		case *InlineFragment:
			t := parentType
			if s.TypeCondition != "" {
				if resolved := ctx.schema.Type(s.TypeCondition); resolved != nil {
					t = resolved
				}
			}
			errors = append(errors, ctx.checkRequiredArgs(s.SelectionSet, t, vf)...)
		case *FragmentSpread:
			if vf[s.Name] {
				continue
			}
			vf[s.Name] = true
			if frag, ok := ctx.fragments[s.Name]; ok {
				if t := ctx.schema.Type(frag.TypeCondition); t != nil {
					errors = append(errors, ctx.checkRequiredArgs(frag.SelectionSet, t, vf)...)
				}
			}
		}
	}
	return errors
}

func (ctx *validationContext) validateUniqueVariableNames() []*GraphQLError {
	var errors []*GraphQLError
	for _, def := range ctx.doc.Definitions {
		if op, ok := def.(*OperationDefinition); ok {
			seen := make(map[string]bool)
			for _, v := range op.VariableDefinitions {
				if seen[v.Variable] {
					errors = append(errors, NewValidationError(
						fmt.Sprintf("There can be only one variable named \"$%s\".", v.Variable),
						v.Loc))
				}
				seen[v.Variable] = true
			}
		}
	}
	return errors
}

func (ctx *validationContext) validateNoUndefinedVariables() []*GraphQLError {
	var errors []*GraphQLError
	for _, def := range ctx.doc.Definitions {
		if op, ok := def.(*OperationDefinition); ok {
			defined := make(map[string]bool)
			for _, v := range op.VariableDefinitions {
				defined[v.Variable] = true
			}
			used := ctx.collectUsedVariables(op.SelectionSet, make(map[string]bool))
			for _, v := range used {
				if !defined[v.name] {
					errors = append(errors, NewValidationError(
						fmt.Sprintf("Variable \"$%s\" is not defined.", v.name),
						v.loc))
				}
			}
		}
	}
	return errors
}

type variableUsage struct {
	name string
	loc  Location
}

func (ctx *validationContext) collectUsedVariables(selections []Selection, vf map[string]bool) []variableUsage {
	var usages []variableUsage
	for _, sel := range selections {
		switch s := sel.(type) {
		case *Field:
			for _, arg := range s.Arguments {
				usages = append(usages, ctx.extractVariables(arg.Value)...)
			}
			usages = append(usages, ctx.collectUsedVariables(s.SelectionSet, vf)...)
		case *InlineFragment:
			usages = append(usages, ctx.collectUsedVariables(s.SelectionSet, vf)...)
		case *FragmentSpread:
			if vf[s.Name] {
				continue
			}
			vf[s.Name] = true
			if frag, ok := ctx.fragments[s.Name]; ok {
				usages = append(usages, ctx.collectUsedVariables(frag.SelectionSet, vf)...)
			}
		}
	}
	return usages
}

func (ctx *validationContext) extractVariables(val Value) []variableUsage {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case *VariableValue:
		return []variableUsage{{name: v.Name, loc: v.Loc}}
	case *ListValue:
		var usages []variableUsage
		for _, item := range v.Values {
			usages = append(usages, ctx.extractVariables(item)...)
		}
		return usages
	case *ObjectValue:
		var usages []variableUsage
		for _, f := range v.Fields {
			usages = append(usages, ctx.extractVariables(f.Value)...)
		}
		return usages
	}
	return nil
}

func (ctx *validationContext) validateNoUnusedVariables() []*GraphQLError {
	var errors []*GraphQLError
	for _, def := range ctx.doc.Definitions {
		if op, ok := def.(*OperationDefinition); ok {
			usedVars := make(map[string]bool)
			usages := ctx.collectUsedVariables(op.SelectionSet, make(map[string]bool))
			for _, u := range usages {
				usedVars[u.name] = true
			}
			for _, v := range op.VariableDefinitions {
				if !usedVars[v.Variable] {
					errors = append(errors, NewValidationError(
						fmt.Sprintf("Variable \"$%s\" is never used.", v.Variable),
						v.Loc))
				}
			}
		}
	}
	return errors
}

func (ctx *validationContext) validateKnownDirectives() []*GraphQLError {
	var errors []*GraphQLError
	for _, def := range ctx.doc.Definitions {
		switch d := def.(type) {
		case *OperationDefinition:
			for _, dir := range d.Directives {
				if ctx.schema.Directive(dir.Name) == nil {
					errors = append(errors, NewValidationError(
						fmt.Sprintf("Unknown directive \"@%s\".", dir.Name),
						dir.Loc))
				}
			}
			errors = append(errors, ctx.checkDirectivesInSelections(d.SelectionSet)...)
		case *FragmentDefinition:
			for _, dir := range d.Directives {
				if ctx.schema.Directive(dir.Name) == nil {
					errors = append(errors, NewValidationError(
						fmt.Sprintf("Unknown directive \"@%s\".", dir.Name),
						dir.Loc))
				}
			}
			errors = append(errors, ctx.checkDirectivesInSelections(d.SelectionSet)...)
		}
	}
	return errors
}

func (ctx *validationContext) checkDirectivesInSelections(selections []Selection) []*GraphQLError {
	var errors []*GraphQLError
	for _, sel := range selections {
		switch s := sel.(type) {
		case *Field:
			for _, dir := range s.Directives {
				if ctx.schema.Directive(dir.Name) == nil {
					errors = append(errors, NewValidationError(
						fmt.Sprintf("Unknown directive \"@%s\".", dir.Name),
						dir.Loc))
				}
			}
			errors = append(errors, ctx.checkDirectivesInSelections(s.SelectionSet)...)
		case *InlineFragment:
			for _, dir := range s.Directives {
				if ctx.schema.Directive(dir.Name) == nil {
					errors = append(errors, NewValidationError(
						fmt.Sprintf("Unknown directive \"@%s\".", dir.Name),
						dir.Loc))
				}
			}
			errors = append(errors, ctx.checkDirectivesInSelections(s.SelectionSet)...)
		case *FragmentSpread:
			for _, dir := range s.Directives {
				if ctx.schema.Directive(dir.Name) == nil {
					errors = append(errors, NewValidationError(
						fmt.Sprintf("Unknown directive \"@%s\".", dir.Name),
						dir.Loc))
				}
			}
		}
	}
	return errors
}

func (ctx *validationContext) validateUniqueDirectivesPerLocation() []*GraphQLError {
	var errors []*GraphQLError
	for _, def := range ctx.doc.Definitions {
		switch d := def.(type) {
		case *OperationDefinition:
			errors = append(errors, checkUniqueDirectives(d.Directives)...)
			errors = append(errors, ctx.checkUniqueDirectivesInSelections(d.SelectionSet)...)
		case *FragmentDefinition:
			errors = append(errors, checkUniqueDirectives(d.Directives)...)
			errors = append(errors, ctx.checkUniqueDirectivesInSelections(d.SelectionSet)...)
		}
	}
	return errors
}

func (ctx *validationContext) checkUniqueDirectivesInSelections(selections []Selection) []*GraphQLError {
	var errors []*GraphQLError
	for _, sel := range selections {
		switch s := sel.(type) {
		case *Field:
			errors = append(errors, checkUniqueDirectives(s.Directives)...)
			errors = append(errors, ctx.checkUniqueDirectivesInSelections(s.SelectionSet)...)
		case *InlineFragment:
			errors = append(errors, checkUniqueDirectives(s.Directives)...)
			errors = append(errors, ctx.checkUniqueDirectivesInSelections(s.SelectionSet)...)
		case *FragmentSpread:
			errors = append(errors, checkUniqueDirectives(s.Directives)...)
		}
	}
	return errors
}

func checkUniqueDirectives(directives []*Directive) []*GraphQLError {
	var errors []*GraphQLError
	seen := make(map[string]bool)
	for _, d := range directives {
		if seen[d.Name] {
			errors = append(errors, NewValidationError(
				fmt.Sprintf("The directive \"@%s\" can only be used once at this location.", d.Name),
				d.Loc))
		}
		seen[d.Name] = true
	}
	return errors
}

func (ctx *validationContext) validateVariablesAreInputTypes() []*GraphQLError {
	var errors []*GraphQLError
	for _, def := range ctx.doc.Definitions {
		if op, ok := def.(*OperationDefinition); ok {
			for _, v := range op.VariableDefinitions {
				t := resolveASTType(ctx.schema, v.Type)
				if t != nil && !IsInputType(t) {
					errors = append(errors, NewValidationError(
						fmt.Sprintf("Variable \"$%s\" cannot be non-input type \"%s\".", v.Variable, v.Type),
						v.Loc))
				}
			}
		}
	}
	return errors
}

// Helper for introspection type references used in validation
func introspectionSchemaType() GraphQLType {
	// Returns nil - introspection fields are handled specially during validation
	return nil
}

func introspectionTypeType() GraphQLType {
	return nil
}
