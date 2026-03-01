package gqlx

import (
	"testing"
)

func testSchema() *Schema {
	humanType := &ObjectType{
		Name_: "Human",
		Fields_: FieldMap{
			"name":   {Type: StringScalar},
			"age":    {Type: IntScalar},
			"height": {Type: FloatScalar},
		},
	}

	droidType := &ObjectType{
		Name_: "Droid",
		Fields_: FieldMap{
			"name":       {Type: StringScalar},
			"primaryFn":  {Type: StringScalar},
		},
	}

	characterIface := &InterfaceType{
		Name_: "Character",
		Fields_: FieldMap{
			"name": {Type: StringScalar},
		},
	}

	humanType.Interfaces_ = []*InterfaceType{characterIface}
	droidType.Interfaces_ = []*InterfaceType{characterIface}

	searchUnion := &UnionType{
		Name_: "SearchResult",
		Types: []*ObjectType{humanType, droidType},
	}

	colorEnum := &EnumType{
		Name_: "Color",
		Values: []*EnumValueDefinition{
			{Name_: "RED"},
			{Name_: "GREEN"},
			{Name_: "BLUE"},
		},
	}

	queryType := &ObjectType{
		Name_: "Query",
		Fields_: FieldMap{
			"hero": {
				Type: humanType,
				Args: ArgumentMap{
					"id": {Name_: "id", Type: IDScalar},
				},
			},
			"search": {Type: NewList(searchUnion)},
			"color":  {Type: colorEnum},
			"name":   {Type: StringScalar},
		},
	}

	mutationType := &ObjectType{
		Name_: "Mutation",
		Fields_: FieldMap{
			"addHero": {
				Type: humanType,
				Args: ArgumentMap{
					"name": {Name_: "name", Type: NewNonNull(StringScalar)},
				},
			},
		},
	}

	schema, _ := NewSchema(SchemaConfig{
		Query:    queryType,
		Mutation: mutationType,
		Types:    []GraphQLType{humanType, droidType},
	})
	return schema
}

func TestValidateValidQuery(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`{ hero { name age } }`)
	errs := Validate(schema, doc)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", FormatErrors(errs))
	}
}

func TestValidateFieldsOnCorrectType(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`{ hero { unknownField } }`)
	errs := Validate(schema, doc)
	if len(errs) == 0 {
		t.Error("expected error for unknown field")
	}
}

func TestValidateScalarLeafs(t *testing.T) {
	schema := testSchema()

	// Scalar with subselection
	doc, _ := Parse(`{ hero { name { sub } } }`)
	errs := Validate(schema, doc)
	if len(errs) == 0 {
		t.Error("expected error for scalar with subselection")
	}

	// Object without subselection
	doc, _ = Parse(`{ hero }`)
	errs = Validate(schema, doc)
	if len(errs) == 0 {
		t.Error("expected error for object without subselection")
	}
}

func TestValidateUniqueOperationNames(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`
		query A { hero { name } }
		query A { hero { age } }
	`)
	errs := Validate(schema, doc)
	if len(errs) == 0 {
		t.Error("expected error for duplicate operation names")
	}
}

func TestValidateLoneAnonymousOperation(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`
		{ hero { name } }
		query B { hero { age } }
	`)
	errs := Validate(schema, doc)
	if len(errs) == 0 {
		t.Error("expected error for anonymous + named operations")
	}
}

func TestValidateKnownTypeNames(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`
		{ hero { name } }
		fragment f on UnknownType { name }
	`)
	errs := Validate(schema, doc)
	found := false
	for _, e := range errs {
		if e.Message == `Unknown type "UnknownType".` {
			found = true
		}
	}
	if !found {
		t.Error("expected error for unknown type name")
	}
}

func TestValidateFragmentsOnCompositeTypes(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`
		{ hero { ...f } }
		fragment f on String { name }
	`)
	errs := Validate(schema, doc)
	found := false
	for _, e := range errs {
		if e.Message == `Fragment "f" cannot condition on non composite type "String".` {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error for fragment on non-composite, got: %v", FormatErrors(errs))
	}
}

func TestValidateUniqueFragmentNames(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`
		{ hero { ...f } }
		fragment f on Human { name }
		fragment f on Human { age }
	`)
	errs := Validate(schema, doc)
	if len(errs) == 0 {
		t.Error("expected error for duplicate fragment names")
	}
}

func TestValidateKnownFragmentNames(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`{ hero { ...unknownFrag } }`)
	errs := Validate(schema, doc)
	found := false
	for _, e := range errs {
		if e.Message == `Unknown fragment "unknownFrag".` {
			found = true
		}
	}
	if !found {
		t.Error("expected error for unknown fragment name")
	}
}

func TestValidateNoUnusedFragments(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`
		{ hero { name } }
		fragment unused on Human { age }
	`)
	errs := Validate(schema, doc)
	found := false
	for _, e := range errs {
		if e.Message == `Fragment "unused" is never used.` {
			found = true
		}
	}
	if !found {
		t.Error("expected error for unused fragment")
	}
}

func TestValidateNoFragmentCycles(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`
		{ hero { ...a } }
		fragment a on Human { ...b }
		fragment b on Human { ...a }
	`)
	errs := Validate(schema, doc)
	if len(errs) == 0 {
		t.Error("expected error for fragment cycle")
	}
}

func TestValidateKnownArgumentNames(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`{ hero(unknownArg: 1) { name } }`)
	errs := Validate(schema, doc)
	found := false
	for _, e := range errs {
		if e.Message == `Unknown argument "unknownArg" on field "Query.hero".` {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error for unknown argument, got: %v", FormatErrors(errs))
	}
}

func TestValidateUniqueArgumentNames(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`{ hero(id: 1, id: 2) { name } }`)
	errs := Validate(schema, doc)
	if len(errs) == 0 {
		t.Error("expected error for duplicate arguments")
	}
}

func TestValidateProvidedRequiredArguments(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`mutation { addHero { name } }`)
	errs := Validate(schema, doc)
	if len(errs) == 0 {
		t.Error("expected error for missing required argument")
	}
}

func TestValidateUniqueVariableNames(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`query($x: Int, $x: Int) { hero(id: $x) { name } }`)
	errs := Validate(schema, doc)
	if len(errs) == 0 {
		t.Error("expected error for duplicate variables")
	}
}

func TestValidateNoUndefinedVariables(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`{ hero(id: $undeclared) { name } }`)
	errs := Validate(schema, doc)
	found := false
	for _, e := range errs {
		if e.Message == `Variable "$undeclared" is not defined.` {
			found = true
		}
	}
	if !found {
		t.Error("expected error for undefined variable")
	}
}

func TestValidateNoUnusedVariables(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`query($x: Int) { hero { name } }`)
	errs := Validate(schema, doc)
	found := false
	for _, e := range errs {
		if e.Message == `Variable "$x" is never used.` {
			found = true
		}
	}
	if !found {
		t.Error("expected error for unused variable")
	}
}

func TestValidateKnownDirectives(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`{ hero @unknown { name } }`)
	errs := Validate(schema, doc)
	found := false
	for _, e := range errs {
		if e.Message == `Unknown directive "@unknown".` {
			found = true
		}
	}
	if !found {
		t.Error("expected error for unknown directive")
	}
}

func TestValidateUniqueDirectivesPerLocation(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`{ hero @skip(if: true) @skip(if: false) { name } }`)
	errs := Validate(schema, doc)
	if len(errs) == 0 {
		t.Error("expected error for duplicate directives")
	}
}

func TestValidateVariablesAreInputTypes(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`query($x: Human) { hero(id: $x) { name } }`)
	errs := Validate(schema, doc)
	found := false
	for _, e := range errs {
		if e.Message == `Variable "$x" cannot be non-input type "Human".` {
			found = true
		}
	}
	if !found {
		t.Error("expected error for non-input variable type")
	}
}

func TestValidateInlineFragmentUnknownType(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`{ hero { ... on UnknownType { name } } }`)
	errs := Validate(schema, doc)
	found := false
	for _, e := range errs {
		if e.Message == `Unknown type "UnknownType".` {
			found = true
		}
	}
	if !found {
		t.Error("expected error for unknown type in inline fragment")
	}
}

func TestValidateKnownDirectivesOnFragments(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`
		{ hero { ...f @unknown } }
		fragment f on Human @unknown { name }
	`)
	errs := Validate(schema, doc)
	found := 0
	for _, e := range errs {
		if e.Message == `Unknown directive "@unknown".` {
			found++
		}
	}
	if found < 2 {
		t.Errorf("expected at least 2 errors for unknown directive on fragments, got %d", found)
	}
}

func TestValidateDirectiveOnOperation(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`query @unknown { hero { name } }`)
	errs := Validate(schema, doc)
	found := false
	for _, e := range errs {
		if e.Message == `Unknown directive "@unknown".` {
			found = true
		}
	}
	if !found {
		t.Error("expected error for unknown directive on operation")
	}
}

func TestValidateInlineFragmentDirective(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`{ hero { ... on Human @unknown { name } } }`)
	errs := Validate(schema, doc)
	found := false
	for _, e := range errs {
		if e.Message == `Unknown directive "@unknown".` {
			found = true
		}
	}
	if !found {
		t.Error("expected error for unknown directive on inline fragment")
	}
}

func TestValidateNestedFieldCheck(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`{ hero { name } search { unknownOnSearchResult } }`)
	errs := Validate(schema, doc)
	// search returns [SearchResult] which is a union - no fields
	if len(errs) == 0 {
		t.Error("expected error for field on union type")
	}
}

func TestValidateFragmentUsedViaSpread(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`
		{ hero { ...f } }
		fragment f on Human { name }
	`)
	errs := Validate(schema, doc)
	for _, e := range errs {
		if e.Message == `Fragment "f" is never used.` {
			t.Error("fragment f should be considered used")
		}
	}
}

func TestValidateVariableInListValue(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`query($x: ID!) { hero(id: $x) { name } }`)
	errs := Validate(schema, doc)
	if len(errs) > 0 {
		t.Errorf("unexpected validation errors: %v", FormatErrors(errs))
	}
}

func TestValidateInlineFragmentWithoutTypeCond(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`{ hero { ... { name } } }`)
	errs := Validate(schema, doc)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", FormatErrors(errs))
	}
}

func TestValidateScalarLeafsInInlineFragment(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`{ hero { ... on Human { name { sub } } } }`)
	errs := Validate(schema, doc)
	if len(errs) == 0 {
		t.Error("expected scalar leaf error in inline fragment")
	}
}

func TestValidateFieldsViaFragmentSpread(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`
		{ hero { ...f } }
		fragment f on Human { unknownField }
	`)
	errs := Validate(schema, doc)
	found := false
	for _, e := range errs {
		if e.Message == `Cannot query field "unknownField" on type "Human".` {
			found = true
		}
	}
	if !found {
		t.Error("expected error for unknown field via fragment spread")
	}
}

func TestValidateUniqueDirectivesOnInlineFragment(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`{ hero { ... on Human @skip(if: true) @skip(if: false) { name } } }`)
	errs := Validate(schema, doc)
	if len(errs) == 0 {
		t.Error("expected error for duplicate directives on inline fragment")
	}
}

func TestValidateUniqueDirectivesOnFragmentSpread(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`
		{ hero { ...f @skip(if: true) @skip(if: false) } }
		fragment f on Human { name }
	`)
	errs := Validate(schema, doc)
	if len(errs) == 0 {
		t.Error("expected error for duplicate directives on fragment spread")
	}
}

func TestValidateUniqueDirectivesOnFragmentDef(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`
		{ hero { ...f } }
		fragment f on Human @skip(if: true) @skip(if: false) { name }
	`)
	errs := Validate(schema, doc)
	if len(errs) == 0 {
		t.Error("expected error for duplicate directives on fragment definition")
	}
}

func TestValidateArgsViaInlineFragment(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`{ hero { ... on Human { name } } }`)
	errs := Validate(schema, doc)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", FormatErrors(errs))
	}
}

func TestValidateRequiredArgsViaInlineFragment(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`mutation { ... on Mutation { addHero { name } } }`)
	errs := Validate(schema, doc)
	if len(errs) == 0 {
		t.Error("expected error for missing required args via inline fragment")
	}
}

func TestValidateVariableInObjectValue(t *testing.T) {
	schema := testSchema()
	doc, _ := Parse(`query($x: ID!) { hero(id: $x) { name } }`)
	errs := Validate(schema, doc)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", FormatErrors(errs))
	}
}
